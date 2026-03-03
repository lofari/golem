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
	StreamJSON   bool      // use --output-format stream-json and parse output for TUI
	PluginDirs   []string  // local plugin directories passed via --plugin-dir
	OutputWriter io.Writer // display destination; defaults to os.Stdout
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
	if c.StreamJSON {
		args = append(args, "--output-format", "stream-json")
	}
	for _, d := range c.PluginDirs {
		args = append(args, "--plugin-dir", d)
	}

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	display := c.OutputWriter
	if display == nil {
		display = os.Stdout
	}
	stderr := c.ErrWriter
	if stderr == nil {
		stderr = os.Stderr
	}

	cmd.Stderr = stderr

	if c.StreamJSON {
		return c.runStreamJSON(ctx, cmd, display, dir)
	}

	// Default: text mode — pipe stdout directly
	var outputBuf strings.Builder
	cmd.Stdout = io.MultiWriter(display, &outputBuf)

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return outputBuf.String(), fmt.Errorf("interrupted: %w", ctx.Err())
		}
		return outputBuf.String(), fmt.Errorf("claude exited with error: %w", err)
	}

	return outputBuf.String(), nil
}

func (c *ClaudeRunner) runStreamJSON(ctx context.Context, cmd *exec.Cmd, display io.Writer, dir string) (string, error) {
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("creating stdout pipe: %w", err)
	}

	parser := NewStreamParser(display)
	parser.EnableDebugLog(dir)
	defer parser.Close()

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("starting claude: %w", err)
	}

	// Parse stream in foreground — blocks until stdout closes
	parser.Parse(stdoutPipe)

	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return parser.Text(), fmt.Errorf("interrupted: %w", ctx.Err())
		}
		return parser.Text(), fmt.Errorf("claude exited with error: %w", err)
	}

	return parser.Text(), nil
}
