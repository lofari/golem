# Golem Improvements Design

Five targeted improvements to the golem CLI: model selection, testability, gitignore, decoupling, and signal handling.

## 1. `--model` flag on root command

Add `--model` as a persistent flag on `rootCmd`. Accepted values: `sonnet`, `opus`, `haiku`. Default: empty (claude picks its own default). The flag threads through `BuilderConfig.Model`, `RunReview`'s parameters, and `spawnClaude`'s args. `cmd/plan.go` reads it from `rootCmd` and passes it to `exec.Command("claude", "--model", model)`.

## 2. `CommandRunner` interface for testability

Extract `spawnClaude` into a `CommandRunner` interface:

```go
type CommandRunner interface {
    Run(ctx context.Context, dir string, prompt string, maxTurns int, model string) (string, error)
}
```

`ClaudeRunner` is the production implementation. `BuilderConfig` and `RunReview` accept a `CommandRunner` field. Tests inject a mock that returns canned output. This also enables signal handling (improvement 5) since the runner owns the `exec.Cmd` lifecycle.

## 3. `.gitignore`

Add `/golem` to a `.gitignore` at the project root.

## 4. Remove `runReview` cross-file coupling

`cmd/run.go` currently calls `runReview()` defined in `cmd/review.go`. Change `cmd/run.go` to call `runner.RunReview()` directly. The `runReview` helper stays in `review.go` as a private convenience for that command only — but `run.go` no longer depends on it.

## 5. Signal handling with process groups

In `ClaudeRunner.Run`:
- Set `cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}` so claude gets its own process group.
- Accept a `context.Context`. On context cancellation (from SIGINT/SIGTERM), forward the signal to the child process group via `syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)`.
- The builder loop uses `signal.NotifyContext` to create a cancellable context. On signal, it finishes the current iteration's post-validation then exits gracefully.
