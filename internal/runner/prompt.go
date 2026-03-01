// internal/runner/prompt.go
package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

	return content, nil
}

type PromptVars struct {
	DocsPath         string
	IterationContext string
	TaskOverride     string
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
