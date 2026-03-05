// internal/runner/prompt.go
package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	golemctx "github.com/lofari/golem/internal/ctx"
)

// RenderPrompt reads a prompt template from disk and replaces template variables.
func RenderPrompt(dir string, templateFile string, vars PromptVars) (string, error) {
	tmplPath := filepath.Join(dir, ".ctx", templateFile)
	data, err := os.ReadFile(tmplPath)
	if err != nil {
		return "", fmt.Errorf("reading prompt template %s: %w", templateFile, err)
	}

	content := string(data)
	content = strings.ReplaceAll(content, "{{DOCS_PATH}}", vars.DocsPath)
	content = strings.ReplaceAll(content, "{{ITERATION_CONTEXT}}", vars.IterationContext)
	content = strings.ReplaceAll(content, "{{TASK_OVERRIDE}}", vars.TaskOverride)
	content = strings.ReplaceAll(content, "{{INJECTED_CONTEXT}}", vars.InjectedContext)

	// Append review context if there are pending review tasks
	if vars.ReviewContext != "" {
		content += "\n" + vars.ReviewContext
	}

	return content, nil
}

type PromptVars struct {
	DocsPath         string
	IterationContext string
	TaskOverride     string
	ReviewContext    string
	InjectedContext  string
}

// BuildIterationContext generates the iteration context string.
func BuildIterationContext(iteration, maxIterations, tasksRemaining int) string {
	ctx := fmt.Sprintf("You are on iteration %d of %d. There are %d tasks remaining.", iteration, maxIterations, tasksRemaining)
	if float64(iteration)/float64(maxIterations) > 0.7 {
		ctx += "\nIf you are running low on iterations, prioritize finishing in-progress tasks cleanly over starting new ones."
	}
	return ctx
}

// BuildTaskOverride generates the task override string for --task flag.
func BuildTaskOverride(taskName string) string {
	if taskName == "" {
		return ""
	}
	return fmt.Sprintf("IMPORTANT: You MUST work on the following task this iteration: %q\nDo not pick a different task.\n", taskName)
}

// BuildReviewContext generates prompt context for pending [review] tasks.
// This is appended to the prompt so Claude knows how to handle review tasks
// regardless of the user's prompt template version.
func BuildReviewContext(tasks []golemctx.Task) string {
	var reviewTasks []golemctx.Task
	for _, t := range tasks {
		if strings.HasPrefix(t.Name, "[review]") && t.Status != "done" {
			reviewTasks = append(reviewTasks, t)
		}
	}
	if len(reviewTasks) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Review Tasks\n")
	b.WriteString("The following `[review]` tasks were added by a code review pass.\n")
	b.WriteString("These are real implementation tasks — do NOT just mark them as done.\n")
	b.WriteString("Read the task `notes` for what needs fixing, investigate the issue in the codebase, and implement the fix.\n")
	b.WriteString("Do NOT look for a `## Task` section in the implementation doc for review tasks — the `notes` field has all the context you need.\n\n")
	for _, t := range reviewTasks {
		b.WriteString(fmt.Sprintf("- **%s** (status: %s)\n", t.Name, t.Status))
		if t.Notes != "" {
			b.WriteString(fmt.Sprintf("  Notes: %s\n", t.Notes))
		}
	}
	return b.String()
}
