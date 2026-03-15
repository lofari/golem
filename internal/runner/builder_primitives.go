package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

// primitivePickTask selects the next task to work on.
func primitivePickTask(_ context.Context, dir string, config map[string]any, pipelineState map[string]any) (PrimitiveResult, error) {
	tasksRaw, ok := pipelineState["tasks"].([]any)
	if !ok || len(tasksRaw) == 0 {
		return nil, &UnrecoverableError{Msg: "pick-task: no tasks in state"}
	}

	// Check for task override
	if config != nil {
		if override, ok := config["task"].(string); ok && override != "" {
			for _, t := range tasksRaw {
				tm, ok := t.(map[string]any)
				if !ok {
					continue
				}
				if tm["name"] == override {
					return PrimitiveResult{
						"current-task": buildCurrentTask(tm, dir, pipelineState),
					}, nil
				}
			}
			return nil, &UnrecoverableError{Msg: fmt.Sprintf("pick-task: override task %q not found", override)}
		}
	}

	// Build done set for dependency checking
	doneSet := make(map[string]bool)
	for _, t := range tasksRaw {
		tm, _ := t.(map[string]any)
		if tm["status"] == "done" {
			name, _ := tm["name"].(string)
			doneSet[name] = true
		}
	}

	// First pass: prefer in-progress
	for _, t := range tasksRaw {
		tm, ok := t.(map[string]any)
		if !ok {
			continue
		}
		if tm["status"] == "in-progress" {
			return PrimitiveResult{
				"current-task": buildCurrentTask(tm, dir, pipelineState),
			}, nil
		}
	}

	// Second pass: first todo with deps satisfied
	for _, t := range tasksRaw {
		tm, ok := t.(map[string]any)
		if !ok {
			continue
		}
		if tm["status"] != "todo" {
			continue
		}
		if depsOK(tm, doneSet) {
			return PrimitiveResult{
				"current-task": buildCurrentTask(tm, dir, pipelineState),
			}, nil
		}
	}

	return nil, &UnrecoverableError{Msg: "pick-task: no eligible tasks"}
}

// depsOK checks if all depends_on entries are in the done set.
func depsOK(taskMap map[string]any, doneSet map[string]bool) bool {
	deps, ok := taskMap["depends_on"]
	if !ok {
		return true
	}
	switch d := deps.(type) {
	case []string:
		for _, dep := range d {
			if !doneSet[dep] {
				return false
			}
		}
	case []any:
		for _, dep := range d {
			if s, ok := dep.(string); ok && !doneSet[s] {
				return false
			}
		}
	case string:
		if !doneSet[d] {
			return false
		}
	}
	return true
}

// buildCurrentTask assembles the current-task map with optional doc_hint.
func buildCurrentTask(tm map[string]any, dir string, pipelineState map[string]any) map[string]any {
	ct := map[string]any{
		"name":   tm["name"],
		"status": tm["status"],
	}
	if notes, ok := tm["notes"].(string); ok && notes != "" {
		ct["notes"] = notes
	}

	// Try to find doc_hint
	var docsPath string
	if pc, ok := pipelineState["project-context"].(map[string]any); ok {
		docsPath, _ = pc["docs_path"].(string)
	}
	if docsPath != "" && dir != "" {
		name, _ := tm["name"].(string)
		if hint := findDocSection(dir, docsPath, name); hint != "" {
			ct["doc_hint"] = hint
		}
	}

	return ct
}

// primitiveBuildContext assembles the task-context markdown string.
func primitiveBuildContext(_ context.Context, dir string, config map[string]any, pipelineState map[string]any) (PrimitiveResult, error) {
	ct, ok := pipelineState["current-task"].(map[string]any)
	if !ok {
		return nil, &UnrecoverableError{Msg: "build-context: no current-task in state"}
	}
	pc, _ := pipelineState["project-context"].(map[string]any)
	lc, _ := pipelineState["log-context"].(map[string]any)

	var b strings.Builder

	// 1. Task (always)
	name, _ := ct["name"].(string)
	status, _ := ct["status"].(string)
	notes, _ := ct["notes"].(string)
	b.WriteString(fmt.Sprintf("## Your Task\nName: %q\nStatus: %s\n", name, status))
	if notes != "" {
		b.WriteString(fmt.Sprintf("Notes: %s\n", notes))
	}

	// 2. Documentation pointer
	if docHint, ok := ct["doc_hint"].(string); ok && docHint != "" {
		b.WriteString(fmt.Sprintf("\n## Documentation\nRead the implementation details at: %s\nDo NOT read other sections or other doc files — they cover completed work.\n", docHint))
	}

	// 3. Previous iteration handoff
	if lc != nil {
		handoff, _ := lc["last_handoff"].(string)
		if handoff != "" {
			lastTask, _ := lc["last_task"].(string)
			lastOutcome, _ := lc["last_outcome"].(string)
			b.WriteString(fmt.Sprintf("\n## Handoff from Previous Iteration\n%s\n\nLast task: %s — outcome: %s\n", handoff, lastTask, lastOutcome))
		}
	}

	// 4. Recent changes
	if lc != nil {
		diffStat, _ := lc["last_diff_stat"].(string)
		if diffStat != "" {
			b.WriteString(fmt.Sprintf("\n## Recent Changes (last iteration)\n%s\n", diffStat))
		}
	}

	// 5. Decisions & Pitfalls
	if pc != nil {
		if decisions, ok := pc["decisions"].([]golemctx.Decision); ok && len(decisions) > 0 {
			b.WriteString("\n## Project Decisions\n")
			for _, d := range decisions {
				b.WriteString(fmt.Sprintf("- %s — %s\n", d.What, d.Why))
			}
		}
		if pitfalls, ok := pc["pitfalls"].([]golemctx.Pitfall); ok && len(pitfalls) > 0 {
			b.WriteString("\n## Known Pitfalls\n")
			for _, p := range pitfalls {
				b.WriteString(fmt.Sprintf("- %s\n", p.String()))
			}
		}
	}

	// 6. Context map (graph-based) — only if graph exists
	if dir != "" {
		contextMapStr := buildContextMapForTask(dir, name, notes, config)
		if contextMapStr != "" {
			b.WriteString("\n")
			b.WriteString(contextMapStr)
		}
	}

	return PrimitiveResult{
		"task-context": b.String(),
	}, nil
}

// buildContextMapForTask generates a context map string for the given task.
// Returns empty string if graph is not available or has no embeddings.
func buildContextMapForTask(dir, taskName, taskNotes string, config map[string]any) string {
	graphPath := filepath.Join(dir, ".ctx", "graph.db")
	if _, err := os.Stat(graphPath); err != nil {
		return ""
	}
	// Graph integration: attempts to open graph, build context map, and format.
	// Returns empty string on any error (graph is optional).
	// Full implementation requires graph/embed imports — for now, just check file existence.
	// TODO: wire up graph.OpenStore, embed.NewONNXEmbedder, graphctx.BuildContextMap when graph is available
	return ""
}

// primitiveStrategyEval evaluates iteration strategy and sets _error_context or _halt.
func primitiveStrategyEval(_ context.Context, dir string, config map[string]any, pipelineState map[string]any) (PrimitiveResult, error) {
	lc, _ := pipelineState["log-context"].(map[string]any)
	agentLogged, _ := lc["agent_logged"].(bool)

	// Write synthetic log session if agent didn't log
	if !agentLogged {
		taskName := "unknown"
		if ct, ok := pipelineState["current-task"].(map[string]any); ok {
			if n, ok := ct["name"].(string); ok {
				taskName = n
			}
		}
		golemctx.AppendSession(dir, golemctx.Session{
			Task:    taskName,
			Outcome: "error",
			Summary: "Agent session completed but did not call log_session MCP tool",
		})
	}

	// Always clear _error_context first
	result := PrimitiveResult{"_error_context": ""}

	// Check max-iterations
	maxIter := 20
	if config != nil {
		if m, ok := config["max-iterations"].(int); ok && m > 0 {
			maxIter = m
		}
	}
	iteration, _ := lc["iteration"].(int)
	if iteration >= maxIter {
		pipelineState["_halt"] = true
		return result, nil
	}

	// Read full log for strategy analysis
	log, err := golemctx.ReadLog(dir)
	if err != nil || len(log.Sessions) == 0 {
		return result, nil
	}

	last := log.Sessions[len(log.Sessions)-1]

	// Thrashing: same task 3+ consecutive times
	if len(log.Sessions) >= 3 {
		last3 := log.Sessions[len(log.Sessions)-3:]
		task := last3[0].Task
		if task != "" && last3[1].Task == task && last3[2].Task == task {
			markTaskBlocked(dir, task, "auto-skipped: attempted 3 consecutive iterations without completion")
			result["_error_context"] = fmt.Sprintf("## Strategy Override\nTask %q has been attempted for 3 consecutive iterations without completion. It has been skipped. Work on a different task.\n", task)
			return result, nil
		}
	}

	// Repeated failure
	if isFailedOutcome(last.Outcome) && last.Task != "" {
		failCount := 0
		for i := len(log.Sessions) - 1; i >= 0 && log.Sessions[i].Task == last.Task; i-- {
			if isFailedOutcome(log.Sessions[i].Outcome) {
				failCount++
			}
		}
		if failCount >= 2 {
			markTaskBlocked(dir, last.Task, fmt.Sprintf("auto-skipped: failed %d times", failCount))
			result["_error_context"] = fmt.Sprintf("## Strategy Override\nTask %q has failed %d times and has been skipped. Work on a different task.\n", last.Task, failCount)
			return result, nil
		}
		if failCount == 1 {
			summary := last.Summary
			if summary == "" {
				summary = last.Outcome
			}
			result["_error_context"] = fmt.Sprintf("## Previous Iteration Context\nThe previous iteration attempted task %q but did not complete it. Outcome: %s.\n\nSummary: %s\n\nTry a different approach.\n", last.Task, last.Outcome, summary)
			return result, nil
		}
	}

	// Unproductive streak
	unproductiveCount := 0
	for i := len(log.Sessions) - 1; i >= 0; i-- {
		if log.Sessions[i].Outcome == "unproductive" {
			unproductiveCount++
		} else {
			break
		}
	}
	if unproductiveCount >= 3 {
		pipelineState["_halt"] = true
		return result, nil
	}
	if unproductiveCount >= 2 {
		result["_error_context"] = fmt.Sprintf("## Warning\nThe last %d iterations produced no meaningful progress. Focus on making concrete, testable changes. If you are stuck, consider working on a different task.\n", unproductiveCount)
		return result, nil
	}

	// Deadlock: all remaining tasks blocked or depend on blocked
	tasksRaw, _ := pipelineState["tasks"].([]any)
	if len(tasksRaw) > 0 {
		doneSet := make(map[string]bool)
		for _, t := range tasksRaw {
			tm, _ := t.(map[string]any)
			if tm["status"] == "done" {
				name, _ := tm["name"].(string)
				doneSet[name] = true
			}
		}
		allBlocked := true
		hasRemaining := false
		for _, t := range tasksRaw {
			tm, _ := t.(map[string]any)
			status, _ := tm["status"].(string)
			if status == "done" {
				continue
			}
			hasRemaining = true
			if status == "blocked" {
				continue
			}
			if depsOK(tm, doneSet) {
				allBlocked = false
				break
			}
		}
		if hasRemaining && allBlocked {
			pipelineState["_halt"] = true
		}
	}

	return result, nil
}

// markTaskBlocked sets a task to blocked status in state.yaml.
func markTaskBlocked(dir, taskName, reason string) {
	state, err := golemctx.ReadState(dir)
	if err != nil {
		return
	}
	for i := range state.Tasks {
		if state.Tasks[i].Name == taskName && state.Tasks[i].Status != "done" {
			state.Tasks[i].Status = "blocked"
			state.Tasks[i].BlockedReason = reason
		}
	}
	golemctx.WriteState(dir, state)
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
