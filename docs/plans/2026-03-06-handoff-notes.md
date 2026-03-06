# Handoff Notes Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Let each iteration write a freeform handoff note that the next iteration receives automatically, reducing orientation overhead.

**Architecture:** Add a `Handoff` field to `Session` in `log.yaml`. The `log_session` MCP tool accepts it. The builder loop reads the last session's handoff and injects it into `{{INJECTED_CONTEXT}}` alongside strategy context.

**Tech Stack:** Go, stdlib testing

**Design doc:** `docs/plans/2026-03-06-handoff-notes-design.md`

---

## Task 1: Add Handoff Field to Session Struct

Add `Handoff` to the `Session` struct and verify it round-trips through YAML.

**Files:**
- Modify: `internal/ctx/log.go:16-25`
- Modify: `internal/ctx/log_test.go`

**Step 1: Write the failing test**

Add to `internal/ctx/log_test.go`:

```go
func TestHandoffRoundTrip(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ctx"), 0755)

	original := Log{
		Sessions: []Session{
			{
				Iteration: 1,
				Task:      "auth",
				Outcome:   "partial",
				Summary:   "started OAuth2",
				Handoff:   "OAuth2 provider configured. Next: implement callback handler in auth.go:45. Watch out — redirect URI must match exactly.",
			},
		},
	}

	if err := WriteLog(dir, original); err != nil {
		t.Fatalf("WriteLog: %v", err)
	}

	got, err := ReadLog(dir)
	if err != nil {
		t.Fatalf("ReadLog: %v", err)
	}

	if got.Sessions[0].Handoff != original.Sessions[0].Handoff {
		t.Errorf("Handoff = %q, want %q", got.Sessions[0].Handoff, original.Sessions[0].Handoff)
	}
}

func TestHandoffOmittedWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ctx"), 0755)

	l := Log{Sessions: []Session{{Iteration: 1, Task: "x", Outcome: "done", Summary: "done"}}}
	if err := WriteLog(dir, l); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, ".ctx", "log.yaml"))
	if strings.Contains(string(data), "handoff") {
		t.Error("empty handoff should be omitted from YAML")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/ctx/ -run TestHandoff -v`
Expected: FAIL — `Session` has no `Handoff` field

**Step 3: Write the implementation**

In `internal/ctx/log.go`, add the `Handoff` field to `Session`:

```go
type Session struct {
	Iteration     int      `yaml:"iteration"`
	Timestamp     string   `yaml:"timestamp"`
	Task          string   `yaml:"task"`
	Outcome       string   `yaml:"outcome"`
	Summary       string   `yaml:"summary"`
	Handoff       string   `yaml:"handoff,omitempty"`
	FilesChanged  []string `yaml:"files_changed,omitempty"`
	DecisionsMade []string `yaml:"decisions_made,omitempty"`
	PitfallsFound []string `yaml:"pitfalls_found,omitempty"`
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/ctx/ -run TestHandoff -v`
Expected: PASS

**Step 5: Run all ctx tests**

Run: `go test ./internal/ctx/ -v`
Expected: All pass (existing tests unaffected)

**Step 6: Commit**

```bash
git add internal/ctx/log.go internal/ctx/log_test.go
git commit -m "feat(ctx): add Handoff field to Session struct"
```

---

## Task 2: Accept Handoff in log_session MCP Tool

Wire the `handoff` parameter into the `log_session` MCP tool.

**Files:**
- Modify: `internal/mcp/tools.go:317-361`
- Modify: `internal/mcp/tools_test.go`

**Step 1: Write the failing test**

Add to `internal/mcp/tools_test.go`:

```go
func TestHandleLogSession_WithHandoff(t *testing.T) {
	dir := setupTestDir(t)
	gs := NewServer(dir)

	result, err := gs.handleLogSession(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"task":    "auth",
				"outcome": "partial",
				"summary": "started OAuth2 flow",
				"handoff": "Provider configured. Next: implement callback in auth.go:45.",
			},
		},
	})
	if err != nil {
		t.Fatalf("handleLogSession: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %v", result.Content)
	}

	log, _ := golemctx.ReadLog(dir)
	if log.Sessions[0].Handoff != "Provider configured. Next: implement callback in auth.go:45." {
		t.Errorf("handoff = %q", log.Sessions[0].Handoff)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/mcp/ -run TestHandleLogSession_WithHandoff -v`
Expected: FAIL — handoff not read from arguments

**Step 3: Write the implementation**

In `internal/mcp/tools.go`, update `logSessionTool()` to add `handoff` to the schema properties:

```go
"handoff": map[string]string{"type": "string", "description": "Handoff note for the next iteration — what you worked on, where you left off, what to do next, and any gotchas"},
```

In `handleLogSession()`, read the handoff and set it on the session:

```go
handoff := getStr(args, "handoff")
```

And in the Session literal, add:

```go
Handoff:      handoff,
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/mcp/ -run TestHandleLogSession_WithHandoff -v`
Expected: PASS

**Step 5: Run all MCP tests**

Run: `go test ./internal/mcp/ -v`
Expected: All pass

**Step 6: Commit**

```bash
git add internal/mcp/tools.go internal/mcp/tools_test.go
git commit -m "feat(mcp): accept handoff parameter in log_session tool"
```

---

## Task 3: Inject Handoff into Next Iteration's Prompt

Read the last session's handoff from the log and prepend it to `injectedContext` in the builder loop.

**Files:**
- Modify: `internal/runner/prompt.go`
- Modify: `internal/runner/prompt_test.go`
- Modify: `internal/runner/builder.go:46-52`

**Step 1: Write the failing test**

Add to `internal/runner/prompt_test.go`:

```go
func TestBuildHandoffContext(t *testing.T) {
	if got := BuildHandoffContext(""); got != "" {
		t.Errorf("empty handoff should produce empty string, got %q", got)
	}

	got := BuildHandoffContext("Auth provider configured. Next: callback handler.")
	if !strings.Contains(got, "## Handoff from Previous Iteration") {
		t.Error("missing handoff header")
	}
	if !strings.Contains(got, "Auth provider configured") {
		t.Error("missing handoff content")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/runner/ -run TestBuildHandoffContext -v`
Expected: FAIL — function doesn't exist

**Step 3: Write the implementation**

Add to `internal/runner/prompt.go`:

```go
// BuildHandoffContext wraps a handoff note from the previous iteration.
func BuildHandoffContext(handoff string) string {
	if handoff == "" {
		return ""
	}
	return fmt.Sprintf("## Handoff from Previous Iteration\n%s\n", handoff)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/runner/ -run TestBuildHandoffContext -v`
Expected: PASS

**Step 5: Wire into builder loop**

In `internal/runner/builder.go`, after reading the log at the start of the loop (around line 50), build the initial handoff context. Then inside the loop, after strategy evaluation sets `injectedContext`, also read the latest handoff.

Replace the initial `var injectedContext string` block (lines 50-51) with:

```go
var injectedContext string

// Seed handoff from last session (if any) for first iteration
log, _ := golemctx.ReadLog(cfg.Dir)
if last := lastLogSession(log); last != nil {
	injectedContext = BuildHandoffContext(last.Handoff)
}
```

After the strategy switch block (around line 277), add handoff injection for subsequent iterations:

```go
// Inject handoff from the iteration that just ran
log, _ = golemctx.ReadLog(cfg.Dir)
if last := lastLogSession(log); last != nil && last.Handoff != "" {
	handoffCtx := BuildHandoffContext(last.Handoff)
	if injectedContext == "" {
		injectedContext = handoffCtx
	} else {
		injectedContext = handoffCtx + "\n" + injectedContext
	}
}
```

**Step 6: Run all runner tests**

Run: `go test ./internal/runner/ -v`
Expected: All pass

**Step 7: Commit**

```bash
git add internal/runner/prompt.go internal/runner/prompt_test.go internal/runner/builder.go
git commit -m "feat(runner): inject handoff notes from previous iteration into prompt"
```

---

## Task 4: Update Prompt Template

Add handoff instructions to the agent's end-of-session protocol.

**Files:**
- Modify: `templates/prompt.md`

**Step 1: Update the prompt template**

In `templates/prompt.md`, add a new step between the existing end-of-session steps. After step 6 (`set_status`) and before step 7 (`log_session`), the `log_session` call already exists — update the `log_session` instruction to mention handoff.

Replace the current `log_session` line:

```
7. Call `log_session` with task name, outcome (done|partial|blocked|unproductive), summary, and files_changed.
```

With:

```
7. Call `log_session` with task name, outcome (done|partial|blocked|unproductive), summary, files_changed, and a `handoff` note. The handoff should tell the next iteration: what you worked on, where you left off, what to do next, and any gotchas.
```

**Step 2: Verify the template renders**

Run: `go test ./internal/runner/ -run TestRenderPrompt -v`
Expected: PASS (template variables still substitute correctly)

**Step 3: Commit**

```bash
git add templates/prompt.md
git commit -m "feat(templates): add handoff note instruction to end-of-session protocol"
```

---

## Task 5: Full Integration Verification

Verify all tests pass and the full build succeeds.

**Step 1: Run all tests**

Run: `go test ./... -v`
Expected: All pass

**Step 2: Build**

Run: `go build ./...`
Expected: Builds cleanly

**Step 3: Commit (if any fixes needed)**

Only commit if Task 5 required fixes. Otherwise, no commit needed.
