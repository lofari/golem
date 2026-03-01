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
		Pitfalls:  []string{"watch out for X"},
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
