package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	golemctx "github.com/lofari/golem/internal/ctx"
)

// readProjectState reads state.yaml and log.yaml and returns a PrimitiveResult
// with project-context, tasks, and log-context keys.
func readProjectState(dir string, pipelineState map[string]any) (PrimitiveResult, error) {
	state, err := golemctx.ReadState(dir)
	if err != nil {
		return nil, &UnrecoverableError{Msg: fmt.Sprintf("reading state: %v", err)}
	}

	log, err := golemctx.ReadLog(dir)
	if err != nil {
		return nil, &UnrecoverableError{Msg: fmt.Sprintf("reading log: %v", err)}
	}

	projectContext := map[string]any{
		"decisions":     state.Decisions,
		"pitfalls":      state.Pitfalls,
		"phase":         state.Status.Phase,
		"current_focus": state.Status.CurrentFocus,
		"docs_path":     state.Project.DocsPath,
	}

	tasks := make([]any, len(state.Tasks))
	for i, t := range state.Tasks {
		tm := map[string]any{
			"name":   t.Name,
			"status": t.Status,
		}
		if t.Notes != "" {
			tm["notes"] = t.Notes
		}
		if !t.DependsOn.IsEmpty() {
			tm["depends_on"] = []string(t.DependsOn)
		}
		if t.BlockedReason != "" {
			tm["blocked_reason"] = t.BlockedReason
		}
		tasks[i] = tm
	}

	logContext := map[string]any{
		"iteration": len(log.Sessions),
	}
	if len(log.Sessions) > 0 {
		last := log.Sessions[len(log.Sessions)-1]
		logContext["last_task"] = last.Task
		logContext["last_outcome"] = last.Outcome
		logContext["last_handoff"] = last.Handoff
	}

	// Compute diff stat from _head_before if available
	if headBefore, ok := pipelineState["_head_before"].(string); ok && headBefore != "" {
		diffStat := gitDiffStat(dir, headBefore)
		if diffStat != "" {
			logContext["last_diff_stat"] = diffStat
		}
	}

	return PrimitiveResult{
		"project-context": projectContext,
		"tasks":           tasks,
		"log-context":     logContext,
	}, nil
}

// primitiveInitState runs once before the loop. Saves snapshot, records HEAD.
func primitiveInitState(_ context.Context, dir string, _ map[string]any, pipelineState map[string]any) (PrimitiveResult, error) {
	if err := SaveSnapshot(dir, 0); err != nil {
		fmt.Fprintf(os.Stderr, "golem: warning: could not save snapshot: %v\n", err)
	}

	head := gitHead(dir)
	pipelineState["_head_before"] = head
	pipelineState["_sync_initialized"] = true

	result, err := readProjectState(dir, pipelineState)
	if err != nil {
		return nil, err
	}

	if lc, ok := result["log-context"].(map[string]any); ok {
		pipelineState["_last_log_iteration"] = lc["iteration"]
	}

	return result, nil
}

// primitiveSyncState runs inside the loop. Validates state, detects agent logging.
func primitiveSyncState(_ context.Context, dir string, _ map[string]any, pipelineState map[string]any) (PrimitiveResult, error) {
	state, err := golemctx.ReadState(dir)
	if err != nil {
		restored, restoreErr := RestoreLatestSnapshot(dir)
		if !restored || restoreErr != nil {
			return nil, &UnrecoverableError{Msg: fmt.Sprintf("sync-state: state unreadable, no snapshot: %v", err)}
		}
		state, err = golemctx.ReadState(dir)
		if err != nil {
			return nil, &UnrecoverableError{Msg: fmt.Sprintf("sync-state: state still unreadable after restore: %v", err)}
		}
	}

	// Auto-repair
	repaired := false
	if state.Status.Phase != "" {
		if _, ok := golemctx.ValidPhases()[state.Status.Phase]; !ok {
			state.Status.Phase = "building"
			repaired = true
		}
	}
	for i := range state.Tasks {
		if _, ok := golemctx.ValidTaskStatuses()[state.Tasks[i].Status]; !ok {
			state.Tasks[i].Status = "todo"
			repaired = true
		}
		if state.Tasks[i].Status == "blocked" && state.Tasks[i].BlockedReason == "" {
			state.Tasks[i].BlockedReason = "no reason provided by agent"
			repaired = true
		}
	}
	if repaired {
		golemctx.WriteState(dir, state)
	}

	result, err := readProjectState(dir, pipelineState)
	if err != nil {
		return nil, err
	}

	// Detect if agent logged
	lc, _ := result["log-context"].(map[string]any)
	prevIteration, _ := pipelineState["_last_log_iteration"].(int)
	currentIteration, _ := lc["iteration"].(int)
	lc["agent_logged"] = currentIteration > prevIteration

	// Update tracking
	pipelineState["_last_log_iteration"] = currentIteration
	pipelineState["_head_before"] = gitHead(dir)

	return result, nil
}

// gitHead returns the current HEAD commit hash.
func gitHead(dir string) string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// gitDiffStat returns `git diff --stat` between two refs.
func gitDiffStat(dir, from string) string {
	cmd := exec.Command("git", "diff", "--stat", from+"..HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
