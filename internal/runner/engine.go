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
	if node.Step != nil {
		return e.execStep(ctx, node.Step)
	}
	if node.ControlFlow != nil {
		return e.execControlFlow(ctx, node.ControlFlow)
	}
	return nil
}

func (e *Engine) execStep(ctx context.Context, step *Step) error {
	e.emit(EngineEvent{Type: "step-start", Step: step.Name, StepType: step.Type})
	start := time.Now()

	var err error
	switch step.Type {
	case StepTypeAgentic:
		err = e.execAgenticStep(ctx, step)
	case StepTypeBuiltin:
		err = e.execBuiltinStep(ctx, step)
	case StepTypeShell:
		err = e.execShellStep(ctx, step)
	default:
		err = fmt.Errorf("unknown step type: %s", step.Type)
	}

	status := "success"
	if err != nil {
		status = "error"
	}
	e.emit(EngineEvent{Type: "step-end", Step: step.Name, Status: status, Duration: time.Since(start).Milliseconds()})

	if err != nil {
		return e.handleError(ctx, step, err)
	}
	e.saveState()
	return nil
}

func (e *Engine) execAgenticStep(ctx context.Context, step *Step) error {
	tmpl, err := e.loadPromptTemplate(step)
	if err != nil {
		return err
	}
	prompt, err := RenderStepPrompt(tmpl, step.Reads, step.OptionalReads, e.state, e.cfg.Config, e.cfg.AgentName, e.RunID)
	if err != nil {
		return err
	}

	tools := step.Tools
	if len(tools) == 0 {
		tools = defaultTools[step.Name]
	}

	maxTurns := e.resolveMaxTurns(step)
	timeout := e.resolveTimeout(step)

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	_, err = e.cfg.Runner.RunWithTools(ctx, e.cfg.Dir, prompt, maxTurns, e.resolveModel(step), tools)
	if err != nil {
		return &TransientError{Msg: fmt.Sprintf("agentic step %s: %v", step.Name, err)}
	}

	if err := e.readSessionOutput(step); err != nil {
		return err
	}

	if sliceContains(step.Writes, "code") {
		e.detectCodeChanges()
	}

	return nil
}

func (e *Engine) execBuiltinStep(ctx context.Context, step *Step) error {
	var result PrimitiveResult
	var err error

	switch step.Name {
	case "git-setup":
		result, err = primitiveGitSetup(ctx, e.cfg.Dir, e.cfg.AgentName, e.cfg.Config)
	case "lint":
		result, err = primitiveLint(ctx, e.cfg.Dir, e.cfg.Config)
	case "run-tests":
		result, err = primitiveRunTests(ctx, e.cfg.Dir, e.cfg.Config)
	case "ci-tests":
		result, err = primitiveCITests(ctx, e.cfg.Dir, e.cfg.Config, e.state)
	case "create-pr":
		result, err = primitiveCreatePR(ctx, e.cfg.Dir, e.cfg.Config, e.state)
	default:
		return fmt.Errorf("unknown builtin primitive: %s", step.Name)
	}

	if err != nil {
		return err
	}
	for k, v := range result {
		e.state[k] = v
	}
	return nil
}

func (e *Engine) execShellStep(ctx context.Context, step *Step) error {
	timeout := 5 * time.Minute
	if step.Timeout != "" {
		if d, err := time.ParseDuration(step.Timeout); err == nil {
			timeout = d
		}
	}

	out, err := runShellCmd(ctx, e.cfg.Dir, step.Command, timeout)

	result := PrimitiveResult{"output": out}
	if err != nil {
		result["status"] = "fail"
		if step.StepErrors != nil && step.StepErrors.NonZero == "halt" {
			for k, v := range result {
				e.state[k] = v
			}
			return &UnrecoverableError{Msg: fmt.Sprintf("shell step %q failed: %v", step.Name, err)}
		}
		return &TransientError{Msg: fmt.Sprintf("shell step %q failed: %v", step.Name, err)}
	}
	result["status"] = "pass"

	for _, key := range step.Writes {
		e.state[key] = result
	}
	return nil
}

func (e *Engine) execControlFlow(ctx context.Context, cf *ControlFlowNode) error {
	switch cf.Type {
	case ControlWhile:
		return e.execWhile(ctx, cf)
	case ControlWhen:
		return e.execWhen(ctx, cf)
	case ControlIf:
		return e.execIf(ctx, cf)
	default:
		return fmt.Errorf("unknown control flow type: %s", cf.Type)
	}
}

func (e *Engine) execWhile(ctx context.Context, cf *ControlFlowNode) error {
	for i := 0; i < cf.Max; i++ {
		if !EvalPredicate(cf.Predicate, e.state, e.cfg.Config) {
			e.emit(EngineEvent{Type: "loop-exit", Predicate: cf.Predicate, Reason: "false"})
			return nil
		}
		e.emit(EngineEvent{Type: "loop-enter", Predicate: cf.Predicate, Iteration: i + 1, Max: cf.Max})

		for _, ref := range cf.StepRefs {
			step := e.cfg.Blueprint.pipeline.StepDefs[ref]
			if step == nil {
				for idx := range cf.InlineSteps {
					if cf.InlineSteps[idx].Name == ref {
						step = &cf.InlineSteps[idx]
						break
					}
				}
			}
			if step == nil {
				return fmt.Errorf("while loop: step %q not found", ref)
			}
			if err := e.execStep(ctx, step); err != nil {
				return err
			}
		}
	}
	e.emit(EngineEvent{Type: "loop-exit", Predicate: cf.Predicate, Reason: "max"})
	return nil
}

func (e *Engine) execWhen(ctx context.Context, cf *ControlFlowNode) error {
	if !EvalPredicate(cf.Predicate, e.state, e.cfg.Config) {
		e.emit(EngineEvent{Type: "conditional-skip", Predicate: cf.Predicate})
		return nil
	}
	for _, ref := range cf.StepRefs {
		step := e.cfg.Blueprint.pipeline.StepDefs[ref]
		if step == nil {
			for idx := range cf.InlineSteps {
				if cf.InlineSteps[idx].Name == ref {
					step = &cf.InlineSteps[idx]
					break
				}
			}
		}
		if step == nil {
			return fmt.Errorf("when block: step %q not found", ref)
		}
		if err := e.execStep(ctx, step); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) execIf(ctx context.Context, cf *ControlFlowNode) error {
	refs := cf.ElseRefs
	if EvalPredicate(cf.Predicate, e.state, e.cfg.Config) {
		refs = cf.ThenRefs
	}
	for _, ref := range refs {
		step := e.cfg.Blueprint.pipeline.StepDefs[ref]
		if step == nil {
			for idx := range cf.InlineSteps {
				if cf.InlineSteps[idx].Name == ref {
					step = &cf.InlineSteps[idx]
					break
				}
			}
		}
		if step == nil {
			return fmt.Errorf("if block: step %q not found", ref)
		}
		if err := e.execStep(ctx, step); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) handleError(ctx context.Context, step *Step, err error) error {
	// Stub — will be implemented in Task 15.
	return err
}

// Keep the compiler happy for imports used in later tasks.
var _ = errors.As
