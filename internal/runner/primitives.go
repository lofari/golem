package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// PrimitiveResult is a map of key-value pairs written to pipeline state.
type PrimitiveResult map[string]any

// primitiveGitSetup creates a new branch for the agent run.
func primitiveGitSetup(ctx context.Context, dir string, agentName string, config map[string]any) (PrimitiveResult, error) {
	base, err := gitCurrentBranch(dir)
	if err != nil {
		base = "main"
	}

	ts := time.Now().Format("20060102-150405")
	branch := fmt.Sprintf("golem/%s-%s", agentName, ts)

	for i := 1; branchExists(dir, branch); i++ {
		branch = fmt.Sprintf("golem/%s-%s-%d", agentName, ts, i)
	}

	if err := gitRun(ctx, dir, "checkout", "-b", branch); err != nil {
		return nil, fmt.Errorf("git-setup: create branch: %w", err)
	}

	return PrimitiveResult{
		"branch": branch,
		"base":   base,
	}, nil
}

func gitCurrentBranch(dir string) (string, error) {
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func branchExists(dir, branch string) bool {
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	cmd.Dir = dir
	return cmd.Run() == nil
}

func gitRun(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %s", err, out)
	}
	return nil
}

// primitiveLint runs the configured lint command.
func primitiveLint(ctx context.Context, dir string, config map[string]any) (PrimitiveResult, error) {
	lintCmd, _ := config["lint-cmd"].(string)
	if lintCmd == "" {
		return PrimitiveResult{"status": "skipped", "reason": "no lint-cmd"}, nil
	}

	fixCmd, _ := config["lint-fix-cmd"].(string)
	autofixApplied := false
	if fixCmd != "" {
		runShellCmd(ctx, dir, fixCmd, 30*time.Second)
		autofixApplied = true
	}

	out, err := runShellCmd(ctx, dir, lintCmd, 30*time.Second)
	if err != nil {
		if isCommandNotFound(err) {
			return nil, &UnrecoverableError{Msg: fmt.Sprintf("lint command not found: %s", lintCmd)}
		}
		if isTimeout(err) {
			return nil, &TransientError{Msg: "lint timeout"}
		}
		result := PrimitiveResult{"status": "fail", "output": out}
		if autofixApplied {
			result["autofix-applied"] = true
		}
		return result, nil
	}
	return PrimitiveResult{"status": "pass", "output": out}, nil
}

// primitiveRunTests runs the configured test command.
func primitiveRunTests(ctx context.Context, dir string, config map[string]any) (PrimitiveResult, error) {
	testCmd, _ := config["test-cmd"].(string)
	if testCmd == "" {
		return PrimitiveResult{"status": "skipped"}, nil
	}

	timeout := 5 * time.Minute
	if t, ok := config["test-timeout"].(string); ok {
		if d, err := time.ParseDuration(t); err == nil {
			timeout = d
		}
	}

	start := time.Now()
	out, err := runShellCmd(ctx, dir, testCmd, timeout)
	durationMs := time.Since(start).Milliseconds()

	if err != nil {
		if isCommandNotFound(err) {
			return nil, &UnrecoverableError{Msg: fmt.Sprintf("test command not found: %s", testCmd)}
		}
		if isTimeout(err) {
			return nil, &TransientError{Msg: "test timeout"}
		}
		return PrimitiveResult{"status": "fail", "output": out, "duration-ms": durationMs}, nil
	}
	return PrimitiveResult{"status": "pass", "output": out, "duration-ms": durationMs}, nil
}

func runShellCmd(ctx context.Context, dir, command string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// runGHCmd runs a gh CLI command without shell interpolation.
func runGHCmd(ctx context.Context, dir string, timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// buildGHPRArgs constructs the argument list for gh pr create.
func buildGHPRArgs(title, body, base, branch string) []string {
	return []string{"pr", "create", "--title", title, "--body", body, "--base", base, "--head", branch}
}

// Error types for primitive failures.

// TransientError represents a temporary failure that may be retried.
type TransientError struct{ Msg string }

func (e *TransientError) Error() string { return e.Msg }

// UnrecoverableError represents a permanent failure that should stop the pipeline.
type UnrecoverableError struct{ Msg string }

func (e *UnrecoverableError) Error() string { return e.Msg }

// MalformedOutputError represents invalid output from a Claude session.
type MalformedOutputError struct{ Msg string }

func (e *MalformedOutputError) Error() string { return e.Msg }

func isCommandNotFound(err error) bool {
	if errors.Is(err, exec.ErrNotFound) {
		return true
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode() == 127
	}
	return false
}

func isTimeout(err error) bool {
	return errors.Is(err, context.DeadlineExceeded)
}

// primitiveCITests pushes branch and monitors CI workflow.
func primitiveCITests(ctx context.Context, dir string, config map[string]any, state map[string]any) (PrimitiveResult, error) {
	// Check gh is available
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, &UnrecoverableError{Msg: "gh CLI not found — install from https://cli.github.com"}
	}

	branch, _ := state["branch"].(string)
	if branch == "" {
		return nil, &UnrecoverableError{Msg: "ci-tests: no branch in state"}
	}

	// Push branch
	if err := gitRun(ctx, dir, "push", "--force-with-lease", "origin", branch); err != nil {
		return nil, &TransientError{Msg: fmt.Sprintf("ci-tests: push failed: %v", err)}
	}

	// Poll for workflow run (simplified: check once after short delay)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(5 * time.Second):
	}

	out, err := runGHCmd(ctx, dir, 30*time.Second, "run", "list", "--branch", branch, "--limit", "1", "--json", "status,conclusion,databaseId")
	if err != nil {
		return nil, &TransientError{Msg: fmt.Sprintf("ci-tests: gh run list failed: %v", err)}
	}

	// Parse result
	var runs []map[string]any
	if jsonErr := json.Unmarshal([]byte(out), &runs); jsonErr != nil || len(runs) == 0 {
		return PrimitiveResult{"status": "skipped", "reason": "no CI runs found"}, nil
	}

	conclusion, _ := runs[0]["conclusion"].(string)
	status := "pass"
	if conclusion == "failure" {
		status = "fail"
	}

	return PrimitiveResult{
		"status":     status,
		"conclusion": conclusion,
		"output":     out,
	}, nil
}

// primitiveCreatePR creates a GitHub PR.
func primitiveCreatePR(ctx context.Context, dir string, config map[string]any, state map[string]any) (PrimitiveResult, error) {
	code, _ := state["code"].(map[string]any)
	files, _ := code["files"].([]string)
	if len(files) == 0 {
		// Also check []any
		filesAny, _ := code["files"].([]any)
		if len(filesAny) == 0 {
			return PrimitiveResult{"status": "skipped", "reason": "no changes"}, nil
		}
	}

	branch, _ := state["branch"].(string)
	base, _ := state["base"].(string)
	goal, _ := state["goal"].(string)

	if branch == "" || base == "" {
		return nil, &UnrecoverableError{Msg: "create-pr: missing branch or base in state"}
	}

	// Push
	if err := gitRun(ctx, dir, "push", "--force-with-lease", "origin", branch); err != nil {
		return nil, &TransientError{Msg: fmt.Sprintf("create-pr: push failed: %v", err)}
	}

	title := generatePRTitle(goal)
	body := buildPRBody(state)

	args := buildGHPRArgs(title, body, base, branch)
	out, err := runGHCmd(ctx, dir, 60*time.Second, args...)
	if err != nil {
		return nil, &TransientError{Msg: fmt.Sprintf("create-pr: gh pr create failed: %v\n%s", err, out)}
	}

	return PrimitiveResult{
		"status": "created",
		"url":    strings.TrimSpace(out),
		"title":  title,
	}, nil
}

func generatePRTitle(goal string) string {
	if len(goal) <= 70 {
		return goal
	}
	// Try word boundary
	truncated := goal[:70]
	lastSpace := strings.LastIndex(truncated, " ")
	if lastSpace > 0 {
		return goal[:lastSpace] + "..."
	}
	return goal[:67] + "..."
}

func buildPRBody(state map[string]any) string {
	var sb strings.Builder
	goal, _ := state["goal"].(string)
	sb.WriteString("## Goal\n\n")
	sb.WriteString(goal)
	sb.WriteString("\n\n")

	if plan, ok := state["plan"]; ok {
		planJSON, _ := json.MarshalIndent(plan, "", "  ")
		sb.WriteString("## Plan\n\n```json\n")
		sb.Write(planJSON)
		sb.WriteString("\n```\n\n")
	}

	if code, ok := state["code"].(map[string]any); ok {
		if diffStat, ok := code["diff-stat"].(string); ok && diffStat != "" {
			sb.WriteString("## Changes\n\n```\n")
			sb.WriteString(diffStat)
			sb.WriteString("\n```\n\n")
		}
	}

	sb.WriteString("---\n*Generated by golem*\n")
	return sb.String()
}
