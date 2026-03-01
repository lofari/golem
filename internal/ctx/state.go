// internal/ctx/state.go
package ctx

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type State struct {
	Project   Project    `yaml:"project"`
	Status    Status     `yaml:"status"`
	Decisions []Decision `yaml:"decisions"`
	Locked    []Lock     `yaml:"locked"`
	Tasks     []Task     `yaml:"tasks"`
	Pitfalls  []string   `yaml:"pitfalls"`
}

type Project struct {
	Name     string `yaml:"name"`
	Summary  string `yaml:"summary"`
	Stack    string `yaml:"stack"`
	DocsPath string `yaml:"docs_path"`
}

type Status struct {
	CurrentFocus string `yaml:"current_focus"`
	Phase        string `yaml:"phase"`
	LastSession  string `yaml:"last_session"`
}

type Decision struct {
	What string `yaml:"what"`
	Why  string `yaml:"why"`
	When string `yaml:"when"`
}

type Lock struct {
	Path string `yaml:"path"`
	Note string `yaml:"note"`
}

type Task struct {
	Name          string `yaml:"name"`
	Status        string `yaml:"status"`
	Notes         string `yaml:"notes,omitempty"`
	DependsOn     string `yaml:"depends_on,omitempty"`
	BlockedReason string `yaml:"blocked_reason,omitempty"`
}

var validPhases = map[string]bool{
	"planning": true, "building": true, "fixing": true, "polishing": true,
}

var validTaskStatuses = map[string]bool{
	"todo": true, "in-progress": true, "done": true, "blocked": true,
}

func StatePath(dir string) string {
	return filepath.Join(dir, ".ctx", "state.yaml")
}

func ReadState(dir string) (State, error) {
	data, err := os.ReadFile(StatePath(dir))
	if err != nil {
		return State{}, fmt.Errorf("reading state.yaml: %w", err)
	}
	var s State
	if err := yaml.Unmarshal(data, &s); err != nil {
		return State{}, fmt.Errorf("parsing state.yaml: %w", err)
	}
	return s, nil
}

func WriteState(dir string, s State) error {
	data, err := yaml.Marshal(&s)
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}
	return os.WriteFile(StatePath(dir), data, 0644)
}

func ValidateState(s State) error {
	var errs []string
	if s.Project.Name == "" {
		errs = append(errs, "project.name is required")
	}
	if s.Status.Phase != "" && !validPhases[s.Status.Phase] {
		errs = append(errs, fmt.Sprintf("invalid phase %q (must be planning|building|fixing|polishing)", s.Status.Phase))
	}
	for i, t := range s.Tasks {
		if !validTaskStatuses[t.Status] {
			errs = append(errs, fmt.Sprintf("task[%d] %q has invalid status %q", i, t.Name, t.Status))
		}
		if t.Status == "blocked" && t.BlockedReason == "" {
			errs = append(errs, fmt.Sprintf("task[%d] %q is blocked but has no blocked_reason", i, t.Name))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("state validation failed:\n  %s", strings.Join(errs, "\n  "))
	}
	return nil
}

// RemainingTasks returns count of tasks not in "done" status.
func (s State) RemainingTasks() int {
	count := 0
	for _, t := range s.Tasks {
		if t.Status != "done" {
			count++
		}
	}
	return count
}

// FindTask returns a pointer to the task with the given name, or nil.
func (s *State) FindTask(name string) *Task {
	for i := range s.Tasks {
		if s.Tasks[i].Name == name {
			return &s.Tasks[i]
		}
	}
	return nil
}
