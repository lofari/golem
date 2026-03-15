package runner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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
	// Context integration
	MCPEnabled bool
	LSPEnabled bool
	GraphPath  string // defaults to ".ctx/graph.db" if empty
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
	var suffix [3]byte
	rand.Read(suffix[:])
	runID := "run-" + ts + "-" + hex.EncodeToString(suffix[:])

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
	if err := os.Remove(currentLink); err != nil && !os.IsNotExist(err) {
		log.Printf("warning: could not remove symlink %s: %v", currentLink, err)
	}
	if err := os.Symlink(e.runDir, currentLink); err != nil {
		log.Printf("warning: could not create symlink %s: %v", currentLink, err)
	}
	defer os.Remove(currentLink)

	var err error
	e.logFile, err = os.Create(filepath.Join(e.runDir, "log.json"))
	if err != nil {
		return nil, fmt.Errorf("create log: %w", err)
	}
	defer e.logFile.Close()

	// Context integration
	injectProjectContext(e.cfg.Dir, e.state)

	if e.cfg.MCPEnabled {
		if cr, ok := e.cfg.Runner.(*ClaudeRunner); ok {
			if err := setupMCP(e.cfg.Dir, cr, e.cfg.LSPEnabled); err != nil {
				log.Printf("golem: warning: MCP setup failed: %v", err)
			}
		}
	}

	// Graph sync
	if err := syncGraph(e.cfg.Dir, e.cfg.GraphPath); err != nil {
		log.Printf("golem: warning: graph sync: %v", err)
	}

	// Execution collector
	collectorCleanup := setupCollector(e.cfg.Dir, e.cfg.GraphPath, e.cfg.Runner, 5)
	var pipelineStatus string
	if collectorCleanup != nil {
		defer func() {
			collectorCleanup(pipelineStatus)
		}()
	}

	e.saveState()
	e.emit(EngineEvent{Type: "pipeline-start", Agent: e.cfg.AgentName, Goal: e.cfg.Goal, RunID: e.RunID})

	start := time.Now()

	for _, node := range e.cfg.Blueprint.pipeline.Nodes {
		if err := e.execNode(ctx, node); err != nil {
			pipelineStatus = "failed"
			e.emit(EngineEvent{Type: "pipeline-end", Status: "error", Duration: time.Since(start).Milliseconds(), RunID: e.RunID})
			return e.state, err
		}
	}

	pipelineStatus = "completed"
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
			log.Printf("warning: event channel full, dropping event type=%s step=%s", ev.Type, ev.Step)
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

func (e *Engine) detectCodeChanges(ctx context.Context) {
	cmd := exec.CommandContext(ctx, "git", "diff", "--name-only", "HEAD")
	cmd.Dir = e.cfg.Dir
	out, err := cmd.Output()
	if err != nil {
		return
	}
	files := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(files) == 1 && files[0] == "" {
		files = nil
	}
	statCmd := exec.CommandContext(ctx, "git", "diff", "--stat", "HEAD")
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
	err := e.runStep(ctx, step)
	if err != nil {
		return e.handleError(ctx, step, err)
	}
	return nil
}

// runStep executes a step without error handling (used by retry logic to avoid re-entrancy).
func (e *Engine) runStep(ctx context.Context, step *Step) error {
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

	if err == nil {
		e.saveState()
	}
	return err
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
		e.detectCodeChanges(ctx)
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

	// Store results: if step declares writes, store result under those keys.
	// Primitives like git-setup write engine-managed keys (branch, base) directly.
	if len(step.Writes) > 0 {
		for _, key := range step.Writes {
			if reservedKeys[key] {
				// Reserved keys (branch, base) come from the result directly
				if val, ok := result[key]; ok {
					e.state[key] = val
				}
			} else {
				e.state[key] = map[string]any(result)
			}
		}
	} else {
		// No declared writes — store flat (e.g., git-setup writes branch/base)
		for k, v := range result {
			e.state[k] = v
		}
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

		if err := e.execSubNodes(ctx, cf.SubNodes, cf.StepRefs, cf.InlineSteps, "while loop"); err != nil {
			return err
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
	return e.execSubNodes(ctx, cf.SubNodes, cf.StepRefs, cf.InlineSteps, "when block")
}

func (e *Engine) execIf(ctx context.Context, cf *ControlFlowNode) error {
	if EvalPredicate(cf.Predicate, e.state, e.cfg.Config) {
		return e.execSubNodes(ctx, cf.ThenNodes, cf.ThenRefs, cf.InlineSteps, "if block")
	}
	return e.execSubNodes(ctx, cf.ElseNodes, cf.ElseRefs, cf.InlineSteps, "if block")
}

// execSubNodes runs a list of sub-nodes. If SubNodes is populated (from parsed YAML),
// it dispatches each node directly. Otherwise falls back to StepRefs lookup.
func (e *Engine) execSubNodes(ctx context.Context, nodes []PipelineNode, refs []string, inlineSteps []Step, label string) error {
	if len(nodes) > 0 {
		for i, node := range nodes {
			if node.ControlFlow != nil {
				if err := e.execControlFlow(ctx, node.ControlFlow); err != nil {
					return err
				}
				continue
			}
			if node.Step != nil {
				if err := e.execStep(ctx, node.Step); err != nil {
					return err
				}
				continue
			}
			// Placeholder node — resolve via ref
			if i < len(refs) && refs[i] != "" {
				step := e.resolveStepRef(refs[i], inlineSteps)
				if step == nil {
					return fmt.Errorf("%s: step %q not found", label, refs[i])
				}
				if err := e.execStep(ctx, step); err != nil {
					return err
				}
			}
		}
		return nil
	}
	// Fallback: use refs directly (for programmatically constructed pipelines)
	for _, ref := range refs {
		if ref == "" {
			continue
		}
		step := e.resolveStepRef(ref, inlineSteps)
		if step == nil {
			return fmt.Errorf("%s: step %q not found", label, ref)
		}
		if err := e.execStep(ctx, step); err != nil {
			return err
		}
	}
	return nil
}

// resolveStepRef looks up a step by name in pipeline StepDefs, then in inline steps.
func (e *Engine) resolveStepRef(ref string, inlineSteps []Step) *Step {
	if step := e.cfg.Blueprint.pipeline.StepDefs[ref]; step != nil {
		return step
	}
	for idx := range inlineSteps {
		if inlineSteps[idx].Name == ref {
			return &inlineSteps[idx]
		}
	}
	return nil
}

func (e *Engine) handleError(ctx context.Context, step *Step, err error) error {
	var transient *TransientError
	var unrecoverable *UnrecoverableError
	var malformed *MalformedOutputError

	switch {
	case errors.As(err, &unrecoverable):
		e.emit(EngineEvent{Type: "error-occurred", Step: step.Name, ErrorType: "unrecoverable", Action: "halt"})
		return err

	case errors.As(err, &malformed):
		handler := e.cfg.Blueprint.Errors.MalformedOutput
		if handler.Action == "" {
			handler.Action = "halt"
		}
		return e.handleMalformedOutput(ctx, step, malformed, handler)

	case errors.As(err, &transient):
		handler := e.cfg.Blueprint.Errors.Transient
		if handler.Action == "" {
			handler.Action = "halt"
		}
		return e.handleTransient(ctx, step, handler)

	default:
		handler := e.cfg.Blueprint.Errors.Transient
		if handler.Action == "" {
			handler.Action = "halt"
		}
		return e.handleTransient(ctx, step, handler)
	}
}

func (e *Engine) handleTransient(ctx context.Context, step *Step, handler ErrorHandler) error {
	if handler.Action != "retry" || handler.Max <= 0 {
		return fmt.Errorf("step %q failed (transient, no retry configured)", step.Name)
	}
	for attempt := 1; attempt <= handler.Max; attempt++ {
		e.emit(EngineEvent{Type: "error-retry", Step: step.Name, ErrorType: "transient", Attempt: attempt, Action: "retry"})
		if err := e.runStep(ctx, step); err == nil {
			return nil
		}
	}
	return fmt.Errorf("step %q failed after %d retries", step.Name, handler.Max)
}

func (e *Engine) handleMalformedOutput(ctx context.Context, step *Step, malformed *MalformedOutputError, handler ErrorHandler) error {
	if handler.Action != "re-run" || handler.Max <= 0 {
		return fmt.Errorf("step %q: malformed output: %s", step.Name, malformed.Msg)
	}
	for attempt := 1; attempt <= handler.Max; attempt++ {
		e.emit(EngineEvent{Type: "error-retry", Step: step.Name, ErrorType: "malformed-output", Attempt: attempt, Action: "re-run"})
		if handler.Hint != "" {
			e.state["_hint"] = handler.Hint
		}
		if err := e.runStep(ctx, step); err == nil {
			delete(e.state, "_hint")
			return nil
		}
	}
	delete(e.state, "_hint")
	return fmt.Errorf("step %q: malformed output after %d re-runs: %s", step.Name, handler.Max, malformed.Msg)
}
