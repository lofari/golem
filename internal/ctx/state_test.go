// internal/ctx/state_test.go
package ctx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadWriteState(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ctx"), 0755)

	original := State{
		Project: Project{Name: "test", DocsPath: "docs/"},
		Status:  Status{Phase: "building"},
		Tasks: []Task{
			{Name: "task1", Status: "done"},
			{Name: "task2", Status: "todo"},
		},
		Decisions: []Decision{{What: "use Go", Why: "fast", When: "2026-03-01"}},
		Pitfalls:  []Pitfall{{What: "watch out for X"}},
	}

	if err := WriteState(dir, original); err != nil {
		t.Fatalf("WriteState: %v", err)
	}

	got, err := ReadState(dir)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}

	if got.Project.Name != "test" {
		t.Errorf("Project.Name = %q, want %q", got.Project.Name, "test")
	}
	if len(got.Tasks) != 2 {
		t.Errorf("len(Tasks) = %d, want 2", len(got.Tasks))
	}
	if got.Tasks[0].Status != "done" {
		t.Errorf("Tasks[0].Status = %q, want %q", got.Tasks[0].Status, "done")
	}
}

func TestValidateState(t *testing.T) {
	tests := []struct {
		name    string
		state   State
		wantErr bool
	}{
		{
			name:    "valid state",
			state:   State{Project: Project{Name: "test"}, Status: Status{Phase: "building"}},
			wantErr: false,
		},
		{
			name:    "missing project name",
			state:   State{Status: Status{Phase: "building"}},
			wantErr: true,
		},
		{
			name:    "invalid phase",
			state:   State{Project: Project{Name: "test"}, Status: Status{Phase: "invalid"}},
			wantErr: true,
		},
		{
			name: "invalid task status",
			state: State{
				Project: Project{Name: "test"},
				Tasks:   []Task{{Name: "t", Status: "invalid"}},
			},
			wantErr: true,
		},
		{
			name: "blocked without reason",
			state: State{
				Project: Project{Name: "test"},
				Tasks:   []Task{{Name: "t", Status: "blocked"}},
			},
			wantErr: true,
		},
		{
			name: "blocked with reason",
			state: State{
				Project: Project{Name: "test"},
				Tasks:   []Task{{Name: "t", Status: "blocked", BlockedReason: "waiting"}},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateState(tt.state)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateState() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNormalizeTaskStatuses(t *testing.T) {
	tasks := []Task{
		{Name: "a", Status: "complete"},
		{Name: "b", Status: "completed"},
		{Name: "c", Status: "finished"},
		{Name: "d", Status: "pending"},
		{Name: "e", Status: "in_progress"},
		{Name: "f", Status: "wip"},
		{Name: "g", Status: "stuck"},
		{Name: "h", Status: "done"},   // already canonical
		{Name: "i", Status: "todo"},   // already canonical
		{Name: "j", Status: "Custom"}, // unknown — left as-is
	}

	fixed := NormalizeTaskStatuses(tasks)

	want := map[string]string{
		"a": "done", "b": "done", "c": "done",
		"d": "todo", "e": "in-progress", "f": "in-progress",
		"g": "blocked", "h": "done", "i": "todo", "j": "Custom",
	}
	for _, task := range tasks {
		if task.Status != want[task.Name] {
			t.Errorf("task %q: got %q, want %q", task.Name, task.Status, want[task.Name])
		}
	}
	if fixed != 7 {
		t.Errorf("fixed = %d, want 7", fixed)
	}
}

func TestNormalizePhase(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		changed bool
	}{
		{"review", "polishing", true},
		{"reviewing", "polishing", true},
		{"build", "building", true},
		{"development", "building", true},
		{"fix", "fixing", true},
		{"debugging", "fixing", true},
		{"plan", "planning", true},
		{"building", "building", false},  // already canonical
		{"unknown", "unknown", false},    // not in aliases
	}
	for _, tt := range tests {
		got, changed := NormalizePhase(tt.input)
		if got != tt.want || changed != tt.changed {
			t.Errorf("NormalizePhase(%q) = (%q, %v), want (%q, %v)", tt.input, got, changed, tt.want, tt.changed)
		}
	}
}

func TestReadStateNormalizesStatuses(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ctx"), 0755)

	yaml := `project:
  name: test
tasks:
  - name: t1
    status: completed
  - name: t2
    status: in_progress
`
	os.WriteFile(filepath.Join(dir, ".ctx", "state.yaml"), []byte(yaml), 0644)

	state, err := ReadState(dir)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if state.Tasks[0].Status != "done" {
		t.Errorf("task t1 status = %q, want %q", state.Tasks[0].Status, "done")
	}
	if state.Tasks[1].Status != "in-progress" {
		t.Errorf("task t2 status = %q, want %q", state.Tasks[1].Status, "in-progress")
	}
}

func TestRemainingTasks(t *testing.T) {
	s := State{
		Tasks: []Task{
			{Name: "a", Status: "done"},
			{Name: "b", Status: "todo"},
			{Name: "c", Status: "in-progress"},
			{Name: "d", Status: "done"},
		},
	}
	if got := s.RemainingTasks(); got != 2 {
		t.Errorf("RemainingTasks() = %d, want 2", got)
	}
}

func TestFindTask(t *testing.T) {
	s := State{
		Tasks: []Task{
			{Name: "first", Status: "todo"},
			{Name: "second", Status: "done"},
		},
	}
	task := s.FindTask("second")
	if task == nil {
		t.Fatal("FindTask returned nil")
	}
	if task.Status != "done" {
		t.Errorf("task.Status = %q, want %q", task.Status, "done")
	}
	if s.FindTask("nonexistent") != nil {
		t.Error("FindTask should return nil for nonexistent task")
	}
}
