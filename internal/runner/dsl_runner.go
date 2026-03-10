package runner

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"time"
)

// DSLRunner spawns golem-dsl as a subprocess and streams its NDJSON events.
type DSLRunner struct {
	DSLCommand string
	Agent      string
	Goal       string
	StateDir   string
	AgentOpts  map[string]interface{}
	Events     chan<- Event
	MaxIter    int
}

func (r *DSLRunner) buildArgs() []string {
	args := []string{r.DSLCommand, "run", r.Agent, "--goal", r.Goal, "--state-dir", r.StateDir}
	for k, v := range r.AgentOpts {
		args = append(args, "--opt", fmt.Sprintf("%s=%v", k, v))
	}
	return args
}

// CheckBinary verifies the DSL binary is available on PATH.
func (r *DSLRunner) CheckBinary() error {
	_, err := exec.LookPath(r.DSLCommand)
	if err != nil {
		return fmt.Errorf("golem-dsl binary not found: %s\nInstall it or set dsl-command in config", r.DSLCommand)
	}
	return nil
}

// Run spawns golem-dsl and streams events until it exits.
func (r *DSLRunner) Run(ctx context.Context) (*BuilderResult, error) {
	startTime := time.Now()
	args := r.buildArgs()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = r.StateDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start golem-dsl: %w", err)
	}

	var lastEvent DSLEvent
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		evt, err := ParseDSLEvent(scanner.Text())
		if err != nil {
			continue // skip non-JSON lines
		}
		lastEvent = evt
		if r.Events != nil {
			r.Events <- MapDSLEvent(evt, r.MaxIter)
		}
	}

	if err := cmd.Wait(); err != nil {
		return &BuilderResult{Halted: true, HaltReason: err.Error(), Duration: time.Since(startTime)}, fmt.Errorf("golem-dsl exited: %w", err)
	}

	result := &BuilderResult{
		Duration:   time.Since(startTime),
		Iterations: lastEvent.TotalSteps,
		Completed:  lastEvent.Outcome == "complete",
		Halted:     lastEvent.Outcome != "" && lastEvent.Outcome != "complete",
	}
	if result.Halted {
		result.HaltReason = lastEvent.Outcome
	}
	return result, nil
}
