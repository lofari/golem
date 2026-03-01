// internal/runner/prompt_test.go
package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderPrompt(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ctx"), 0755)

	tmpl := "Docs at {{DOCS_PATH}}.\n{{ITERATION_CONTEXT}}\n{{TASK_OVERRIDE}}"
	os.WriteFile(filepath.Join(dir, ".ctx", "prompt.md"), []byte(tmpl), 0644)

	result, err := RenderPrompt(dir, "prompt.md", PromptVars{
		DocsPath:         "docs/plans",
		IterationContext: "Iteration 3 of 10. 5 tasks remaining.",
		TaskOverride:     "",
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "Docs at docs/plans") {
		t.Error("DOCS_PATH not replaced")
	}
	if !strings.Contains(result, "Iteration 3 of 10") {
		t.Error("ITERATION_CONTEXT not replaced")
	}
	// TaskOverride empty => replaced with empty string
	if strings.Contains(result, "{{TASK_OVERRIDE}}") {
		t.Error("TASK_OVERRIDE not replaced")
	}
}

func TestBuildIterationContext(t *testing.T) {
	// Not low on iterations
	ctx := BuildIterationContext(3, 20, 8)
	if !strings.Contains(ctx, "iteration 3 of 20") {
		t.Error("missing iteration info")
	}
	if strings.Contains(ctx, "running low") {
		t.Error("should not warn when not low")
	}

	// Low on iterations (>70%)
	ctx = BuildIterationContext(15, 20, 3)
	if !strings.Contains(ctx, "running low") {
		t.Error("should warn when low on iterations")
	}
}

func TestBuildTaskOverride(t *testing.T) {
	if got := BuildTaskOverride(""); got != "" {
		t.Errorf("empty task should produce empty string, got %q", got)
	}

	got := BuildTaskOverride("fix auth")
	if !strings.Contains(got, "fix auth") {
		t.Error("task name not in override")
	}
	if !strings.Contains(got, "MUST work on") {
		t.Error("missing MUST directive")
	}
}
