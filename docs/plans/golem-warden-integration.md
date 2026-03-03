# Golem + Warden Integration

## Context

Golem spawns `claude -p` with `--dangerously-skip-permissions`, giving the agent unrestricted host access. Warden is a sandbox CLI that wraps commands in Docker containers. We want Golem to optionally wrap its Claude invocations with `warden run` so the agent runs inside a sandbox.

**Auth model:** Claude Code with a subscription authenticates via OAuth tokens stored in `~/.claude/`. No API key is needed — the `~/.claude/` directory must be mounted read-only into the container.

**Warden has a `claude` feature** that installs Claude Code CLI inside the container via npm.

## What the wrapped command looks like

Currently Golem runs:
```
claude -p <prompt> --max-turns 50 --dangerously-skip-permissions [--model X] [--verbose] [--output-format stream-json] [--plugin-dir D]
```

With sandbox enabled, this becomes:
```
warden run \
  --network \
  --tools claude \
  --mount ~/.claude:ro \
  --mount <project-dir>:rw \
  [--mount <plugin-dir>:ro ...] \
  -- \
  claude -p <prompt> --max-turns 50 --dangerously-skip-permissions [--model X] [--verbose] [--output-format stream-json] [--plugin-dir D]
```

Key points:
- `--network` is required — Claude Code must reach Anthropic's API
- `--tools claude` installs Claude Code CLI inside the container
- `~/.claude` is mounted read-only for OAuth credentials
- The project directory is mounted read-write (agent modifies code)
- Plugin directories are mounted read-only
- Everything after `--` is passed through verbatim to Claude inside the container

## Changes

### 1. Add `Sandbox` field to `ClaudeRunner`

**File:** `internal/runner/command.go`

Add `Sandbox bool` to the `ClaudeRunner` struct.

In `Run()`, when `Sandbox` is true, wrap the command with warden:

```go
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
	// ... rest unchanged
```

Add a helper method:

```go
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
```

### 2. Add `--sandbox` flag to `golem run`

**File:** `cmd/run.go`

Add flag:
```go
runCmd.Flags().Bool("sandbox", false, "run Claude inside a warden sandbox container")
```

Read it and pass to ClaudeRunner:
```go
sandbox, _ := cmd.Flags().GetBool("sandbox")

claudeRunner := &runner.ClaudeRunner{
	Verbose:    verbose,
	PluginDirs: pluginDirs,
	Sandbox:    sandbox,
}
```

Do this for both `runWithoutTUI()` and `runWithTUI()` — the sandbox flag must be threaded through to both paths.

### 3. Add `--sandbox` flag to `golem review`

**File:** `cmd/review.go`

Same pattern:
```go
sandbox, _ := cmd.Flags().GetBool("sandbox")

// in the runner construction:
&runner.ClaudeRunner{Verbose: verbose, PluginDirs: pluginDirs, Sandbox: sandbox}
```

Add flag:
```go
reviewCmd.Flags().Bool("sandbox", false, "run Claude inside a warden sandbox container")
```

### 4. Tests

**File:** `internal/runner/command_test.go`

Test `buildCommand` with sandbox off (returns `claude`, original args) and sandbox on (returns `warden`, wrapped args with mounts).

## Files to modify

| File | Change |
|------|--------|
| `internal/runner/command.go` | Add `Sandbox` field, `buildCommand()` helper |
| `cmd/run.go` | Add `--sandbox` flag, thread to both TUI and non-TUI paths |
| `cmd/review.go` | Add `--sandbox` flag |
| `internal/runner/command_test.go` | Test buildCommand with/without sandbox |

## Verification

1. `go test ./...` — all tests pass
2. `go vet ./...` — clean
3. `golem run --sandbox --dry-run` — verify rendered prompt still looks correct (dry-run happens before Claude invocation, so sandbox doesn't affect it)
