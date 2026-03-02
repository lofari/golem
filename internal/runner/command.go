// internal/runner/command.go
package runner

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// CommandRunner abstracts the execution of a Claude CLI session.
type CommandRunner interface {
	Run(ctx context.Context, dir string, prompt string, maxTurns int, model string) (string, error)
}

// ClaudeRunner is the production implementation that spawns `claude -p`.
type ClaudeRunner struct {
	Verbose bool
}

func (c *ClaudeRunner) Run(ctx context.Context, dir string, prompt string, maxTurns int, model string) (string, error) {
	args := []string{"-p", prompt, "--max-turns", fmt.Sprintf("%d", maxTurns)}
	if model != "" {
		args = append(args, "--model", model)
	}
	if c.Verbose {
		args = append(args, "--verbose")
	}

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var outputBuf strings.Builder
	cmd.Stdout = io.MultiWriter(os.Stdout, &outputBuf)

	if err := cmd.Run(); err != nil {
		// If killed by context cancellation, return what we have
		if ctx.Err() != nil {
			return outputBuf.String(), fmt.Errorf("interrupted: %w", ctx.Err())
		}
		return outputBuf.String(), fmt.Errorf("claude exited with error: %w", err)
	}

	return outputBuf.String(), nil
}
