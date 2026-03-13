package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/lofari/golem/templates"
)

// EngineConfig holds configuration for a blueprint engine run.
type EngineConfig struct {
	Dir       string
	AgentName string
	Goal      string
	Blueprint *Blueprint
	Config    map[string]any
	Runner    CommandRunner
	Model     string
	Events    chan<- EngineEvent
	Verbose   bool
}

// EngineEvent represents a structured event emitted during engine execution.
type EngineEvent struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Step      string    `json:"step,omitempty"`
	StepType  string    `json:"step-type,omitempty"`
	Status    string    `json:"status,omitempty"`
	Duration  int64     `json:"duration-ms,omitempty"`
	Agent     string    `json:"agent,omitempty"`
	Goal      string    `json:"goal,omitempty"`
	RunID     string    `json:"run-id,omitempty"`
	Line      string    `json:"line,omitempty"`
	Predicate string    `json:"predicate,omitempty"`
	Iteration int       `json:"iteration,omitempty"`
	Max       int       `json:"max,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	ErrorType string    `json:"error-type,omitempty"`
	Action    string    `json:"action,omitempty"`
	Attempt   int       `json:"attempt,omitempty"`
}

// Engine executes a blueprint pipeline with state management.
type Engine struct {
	RunID    string
	cfg      EngineConfig
	state    map[string]any
	runDir   string
	stateVer int
	logFile  *os.File
}

// NewEngine creates a new engine instance with an initial state.
func NewEngine(cfg EngineConfig) *Engine {
	ts := time.Now().Format("20060102-150405")
	runID := "run-" + ts

	e := &Engine{
		RunID: runID,
		cfg:   cfg,
		state: map[string]any{"goal": cfg.Goal},
	}
	return e
}

// State returns the current pipeline state.
func (e *Engine) State() map[string]any {
	return e.state
}

// Run executes the blueprint pipeline to completion.
func (e *Engine) Run(ctx context.Context) (map[string]any, error) {
	e.runDir = filepath.Join(e.cfg.Dir, ".ctx", "runs", e.RunID)
	if err := os.MkdirAll(filepath.Join(e.runDir, "sessions"), 0755); err != nil {
		return nil, fmt.Errorf("create run dir: %w", err)
	}

	currentLink := filepath.Join(e.cfg.Dir, ".ctx", "runs", "current")
	os.Remove(currentLink)
	os.Symlink(e.runDir, currentLink)
	defer os.Remove(currentLink)

	var err error
	e.logFile, err = os.Create(filepath.Join(e.runDir, "log.json"))
	if err != nil {
		return nil, fmt.Errorf("create log: %w", err)
	}
	defer e.logFile.Close()

	e.saveState()
	e.emit(EngineEvent{Type: "pipeline-start", Agent: e.cfg.AgentName, Goal: e.cfg.Goal, RunID: e.RunID})

	start := time.Now()

	for _, node := range e.cfg.Blueprint.pipeline.Nodes {
		if err := e.execNode(ctx, node); err != nil {
			e.emit(EngineEvent{Type: "pipeline-end", Status: "error", Duration: time.Since(start).Milliseconds(), RunID: e.RunID})
			return e.state, err
		}
	}

	e.emit(EngineEvent{Type: "pipeline-end", Status: "success", Duration: time.Since(start).Milliseconds(), RunID: e.RunID})
	return e.state, nil
}

func (e *Engine) emit(ev EngineEvent) {
	ev.Timestamp = time.Now()
	if ev.RunID == "" {
		ev.RunID = e.RunID
	}
	if e.logFile != nil {
		data, _ := json.Marshal(ev)
		e.logFile.Write(data)
		e.logFile.Write([]byte("\n"))
	}
	if e.cfg.Events != nil {
		select {
		case e.cfg.Events <- ev:
		default:
		}
	}
}

func (e *Engine) saveState() {
	if e.runDir == "" {
		return
	}
	e.stateVer++
	data, _ := json.MarshalIndent(e.state, "", "  ")
	os.WriteFile(filepath.Join(e.runDir, fmt.Sprintf("state-%03d.json", e.stateVer)), data, 0644)
	os.WriteFile(filepath.Join(e.runDir, "state.json"), data, 0644)
}

func (e *Engine) loadPromptTemplate(step *Step) (string, error) {
	if step.Prompt != "" {
		return step.Prompt, nil
	}
	data, err := templates.FS.ReadFile("prompts/" + step.Name + ".md")
	if err != nil {
		return "", fmt.Errorf("no prompt template for step %q: inline prompt not set and templates/prompts/%s.md not found", step.Name, step.Name)
	}
	return string(data), nil
}

func (e *Engine) resolveMaxTurns(step *Step) int {
	if step.MaxTurns > 0 {
		return step.MaxTurns
	}
	if d, ok := stepDefaults[step.Name]; ok {
		return d.MaxTurns
	}
	return defaultStepMaxTurns
}

func (e *Engine) resolveTimeout(step *Step) time.Duration {
	if step.Timeout != "" {
		if d, err := time.ParseDuration(step.Timeout); err == nil {
			return d
		}
	}
	if d, ok := stepDefaults[step.Name]; ok {
		return d.Timeout
	}
	return defaultStepTimeout
}

func (e *Engine) resolveModel(step *Step) string {
	if step.Model != "" {
		return step.Model
	}
	return e.cfg.Model
}

func (e *Engine) readSessionOutput(step *Step) error {
	path := filepath.Join(e.cfg.Dir, "session-output.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &MalformedOutputError{Msg: fmt.Sprintf("step %q did not write session-output.json", step.Name)}
		}
		return &TransientError{Msg: fmt.Sprintf("reading session-output.json: %v", err)}
	}
	defer os.Remove(path)

	var output map[string]any
	if err := json.Unmarshal(data, &output); err != nil {
		return &MalformedOutputError{Msg: fmt.Sprintf("invalid JSON in session-output.json: %v", err)}
	}

	for _, key := range step.Writes {
		if reservedKeys[key] {
			continue
		}
		if val, ok := output[key]; ok {
			e.state[key] = val
		}
	}
	return nil
}

func (e *Engine) detectCodeChanges() {
	cmd := exec.CommandContext(context.Background(), "git", "diff", "--name-only", "HEAD")
	cmd.Dir = e.cfg.Dir
	out, err := cmd.Output()
	if err != nil {
		return
	}
	files := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(files) == 1 && files[0] == "" {
		files = nil
	}
	statCmd := exec.CommandContext(context.Background(), "git", "diff", "--stat", "HEAD")
	statCmd.Dir = e.cfg.Dir
	statOut, _ := statCmd.Output()

	e.state["code"] = map[string]any{
		"files":     files,
		"diff-stat": strings.TrimSpace(string(statOut)),
	}
}

func (e *Engine) execNode(ctx context.Context, node PipelineNode) error {
	// Stub — will be implemented in Task 13.
	return fmt.Errorf("execNode not yet implemented")
}

// Keep the compiler happy for imports used in later tasks.
var _ = errors.As
