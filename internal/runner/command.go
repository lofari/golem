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
	Verbose      bool
	PluginDirs   []string  // local plugin directories passed via --plugin-dir
	OutputWriter io.Writer // stdout destination; defaults to os.Stdout
	ErrWriter    io.Writer // stderr destination; defaults to os.Stderr
}

func (c *ClaudeRunner) Run(ctx context.Context, dir string, prompt string, maxTurns int, model string) (string, error) {
	args := []string{"-p", prompt, "--max-turns", fmt.Sprintf("%d", maxTurns), "--dangerously-skip-permissions"}
	if model != "" {
		args = append(args, "--model", model)
	}
	if c.Verbose {
		args = append(args, "--verbose")
	}
	for _, dir := range c.PluginDirs {
		args = append(args, "--plugin-dir", dir)
	}

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout := c.OutputWriter
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := c.ErrWriter
	if stderr == nil {
		stderr = os.Stderr
	}

	cmd.Stderr = stderr

	var outputBuf strings.Builder
	cmd.Stdout = io.MultiWriter(stdout, &outputBuf)

	if err := cmd.Run(); err != nil {
		// If killed by context cancellation, return what we have
		if ctx.Err() != nil {
			return outputBuf.String(), fmt.Errorf("interrupted: %w", ctx.Err())
		}
		return outputBuf.String(), fmt.Errorf("claude exited with error: %w", err)
	}

	return outputBuf.String(), nil
}
