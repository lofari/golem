package mcp

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	golemctx "github.com/lofari/golem/internal/ctx"
)

// flock acquires an exclusive file lock. Returns unlock function.
func flock(path string) (func(), error) {
	f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}

func (gs *GolemServer) withStateLock(fn func(golemctx.State) (golemctx.State, error)) error {
	unlock, err := flock(golemctx.StatePath(gs.dir))
	if err != nil {
		return fmt.Errorf("acquiring state lock: %w", err)
	}
	defer unlock()

	state, err := golemctx.ReadState(gs.dir)
	if err != nil {
		return err
	}
	updated, err := fn(state)
	if err != nil {
		return err
	}
	return golemctx.WriteState(gs.dir, updated)
}

func (gs *GolemServer) withLogLock(fn func(golemctx.Log) (golemctx.Log, error)) error {
	unlock, err := flock(golemctx.LogPath(gs.dir))
	if err != nil {
		return fmt.Errorf("acquiring log lock: %w", err)
	}
	defer unlock()

	log, err := golemctx.ReadLog(gs.dir)
	if err != nil {
		return err
	}
	updated, err := fn(log)
	if err != nil {
		return err
	}
	return golemctx.WriteLog(gs.dir, updated)
}

func getStr(args map[string]any, key string) string {
	v, _ := args[key].(string)
	return v
}

func getStrSlice(args map[string]any, key string) []string {
	raw, ok := args[key].([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// --- Tool Definitions ---

func markTaskTool() mcp.Tool {
	return mcp.Tool{
		Name:        "mark_task",
		Description: "Update a task's status and notes in state.yaml. Valid statuses: todo, in-progress, done, blocked. If status is 'blocked', blocked_reason is required.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"name":           map[string]string{"type": "string", "description": "Task name (must match exactly)"},
				"status":         map[string]string{"type": "string", "description": "New status: todo, in-progress, done, or blocked"},
				"notes":          map[string]string{"type": "string", "description": "Optional notes to set on the task"},
				"blocked_reason": map[string]string{"type": "string", "description": "Required when status is 'blocked'"},
			},
			Required: []string{"name", "status"},
		},
	}
}

func (gs *GolemServer) handleMarkTask(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	name := getStr(args, "name")
	status := getStr(args, "status")
	notes := getStr(args, "notes")
	blockedReason := getStr(args, "blocked_reason")

	if !golemctx.ValidTaskStatuses()[status] {
		return mcp.NewToolResultError(fmt.Sprintf("invalid status %q — must be one of: todo, in-progress, done, blocked", status)), nil
	}
	if status == "blocked" && blockedReason == "" {
		return mcp.NewToolResultError("blocked_reason is required when status is 'blocked'"), nil
	}

	err := gs.withStateLock(func(s golemctx.State) (golemctx.State, error) {
		task := s.FindTask(name)
		if task == nil {
			return s, fmt.Errorf("task %q not found", name)
		}
		task.Status = status
		if notes != "" {
			task.Notes = notes
		}
		if blockedReason != "" {
			task.BlockedReason = blockedReason
		}
		return s, nil
	})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("task %q marked as %s", name, status)), nil
}

func setPhaseTool() mcp.Tool {
	return mcp.Tool{
		Name:        "set_phase",
		Description: "Set the project phase in state.yaml. Valid phases: planning, building, fixing, polishing.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"phase": map[string]string{"type": "string", "description": "New phase: planning, building, fixing, or polishing"},
			},
			Required: []string{"phase"},
		},
	}
}

func (gs *GolemServer) handleSetPhase(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	phase := getStr(req.GetArguments(), "phase")

	if !golemctx.ValidPhases()[phase] {
		return mcp.NewToolResultError(fmt.Sprintf("invalid phase %q — must be one of: planning, building, fixing, polishing", phase)), nil
	}

	err := gs.withStateLock(func(s golemctx.State) (golemctx.State, error) {
		s.Status.Phase = phase
		return s, nil
	})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("phase set to %q", phase)), nil
}

func addDecisionTool() mcp.Tool {
	return mcp.Tool{
		Name:        "add_decision",
		Description: "Append an architectural decision to state.yaml.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"what": map[string]string{"type": "string", "description": "What was decided"},
				"why":  map[string]string{"type": "string", "description": "Why this decision was made"},
			},
			Required: []string{"what", "why"},
		},
	}
}

func (gs *GolemServer) handleAddDecision(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	what := getStr(args, "what")
	why := getStr(args, "why")

	err := gs.withStateLock(func(s golemctx.State) (golemctx.State, error) {
		s.Decisions = append(s.Decisions, golemctx.Decision{
			What: what,
			Why:  why,
			When: time.Now().Format("2006-01-02"),
		})
		return s, nil
	})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("decision added: %s", what)), nil
}

func addPitfallTool() mcp.Tool {
	return mcp.Tool{
		Name:        "add_pitfall",
		Description: "Record a pitfall/lesson learned in state.yaml.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"what": map[string]string{"type": "string", "description": "What went wrong or could go wrong"},
				"fix":  map[string]string{"type": "string", "description": "How to fix or avoid it"},
			},
			Required: []string{"what"},
		},
	}
}

func (gs *GolemServer) handleAddPitfall(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	what := getStr(args, "what")
	fix := getStr(args, "fix")

	err := gs.withStateLock(func(s golemctx.State) (golemctx.State, error) {
		s.Pitfalls = append(s.Pitfalls, golemctx.Pitfall{What: what, Fix: fix})
		return s, nil
	})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("pitfall added: %s", what)), nil
}

func addLockedTool() mcp.Tool {
	return mcp.Tool{
		Name:        "add_locked",
		Description: "Lock a file path so it won't be modified by future iterations.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"path": map[string]string{"type": "string", "description": "File or directory path to lock"},
				"note": map[string]string{"type": "string", "description": "Why this path is locked"},
			},
			Required: []string{"path"},
		},
	}
}

func (gs *GolemServer) handleAddLocked(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	path := getStr(args, "path")
	note := getStr(args, "note")

	err := gs.withStateLock(func(s golemctx.State) (golemctx.State, error) {
		s.Locked = append(s.Locked, golemctx.Lock{Path: path, Note: note})
		return s, nil
	})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("locked: %s", path)), nil
}

func logSessionTool() mcp.Tool {
	return mcp.Tool{
		Name:        "log_session",
		Description: "Append a session entry to log.yaml. Call this at the end of each iteration.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"task":          map[string]string{"type": "string", "description": "Task name worked on"},
				"outcome":       map[string]string{"type": "string", "description": "Outcome: done, partial, blocked, or unproductive"},
				"summary":       map[string]string{"type": "string", "description": "Brief summary of work done"},
				"files_changed": map[string]any{"type": "array", "items": map[string]string{"type": "string"}, "description": "List of files modified"},
			},
			Required: []string{"task", "outcome", "summary"},
		},
	}
}

func (gs *GolemServer) handleLogSession(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	task := getStr(args, "task")
	outcome := getStr(args, "outcome")
	summary := getStr(args, "summary")
	filesChanged := getStrSlice(args, "files_changed")

	validOutcomes := map[string]bool{"done": true, "partial": true, "blocked": true, "unproductive": true}
	if !validOutcomes[outcome] {
		return mcp.NewToolResultError(fmt.Sprintf("invalid outcome %q — must be one of: done, partial, blocked, unproductive", outcome)), nil
	}

	err := gs.withLogLock(func(l golemctx.Log) (golemctx.Log, error) {
		iteration := len(l.Sessions) + 1
		l.Sessions = append(l.Sessions, golemctx.Session{
			Iteration:    iteration,
			Timestamp:    time.Now().Format(time.RFC3339),
			Task:         task,
			Outcome:      outcome,
			Summary:      summary,
			FilesChanged: filesChanged,
		})
		return l, nil
	})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("session logged: %s — %s", task, outcome)), nil
}
