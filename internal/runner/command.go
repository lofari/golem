// internal/runner/command.go
package runner

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
	Sandbox      bool      // run Claude inside a warden sandbox container
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

	cmdName, cmdArgs := c.buildCommand(dir, args)
	cmd := exec.CommandContext(ctx, cmdName, cmdArgs...)
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

func (c *ClaudeRunner) buildCommand(dir string, claudeArgs []string) (string, []string) {
	if !c.Sandbox {
		return "claude", claudeArgs
	}

	homeDir, _ := os.UserHomeDir()
	wardenArgs := []string{
		"run",
		"--network",
		"--tools", "claude",
		"--mount", homeDir + "/.claude:ro",
		"--mount", dir + ":rw",
	}
	for _, d := range c.PluginDirs {
		abs, err := filepath.Abs(d)
		if err == nil {
			d = abs
		}
		wardenArgs = append(wardenArgs, "--mount", d+":ro")
	}
	wardenArgs = append(wardenArgs, "--")
	wardenArgs = append(wardenArgs, "claude")
	wardenArgs = append(wardenArgs, claudeArgs...)
	return "warden", wardenArgs
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
