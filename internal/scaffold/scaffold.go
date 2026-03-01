// internal/scaffold/scaffold.go
package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lofari/golem/internal/ctx"
	"github.com/lofari/golem/templates"
)

type InitOptions struct {
	Name     string
	Stack    string
	DocsPath string // default "docs/"
}

type InitResult struct {
	Created []string
	Skipped []string
	Updated []string
}

func Init(dir string, opts InitOptions) (InitResult, error) {
	var result InitResult

	if opts.DocsPath == "" {
		opts.DocsPath = "docs/"
	}

	// Create .ctx/ directory
	ctxDir := filepath.Join(dir, ".ctx")
	if err := os.MkdirAll(ctxDir, 0755); err != nil {
		return result, fmt.Errorf("creating .ctx/: %w", err)
	}

	// Write template files (skip if they exist)
	templateFiles := map[string]string{
		"state.yaml":       "state.yaml",
		"log.yaml":         "log.yaml",
		"prompt.md":        "prompt.md",
		"review-prompt.md": "review-prompt.md",
	}

	for destName, tmplName := range templateFiles {
		destPath := filepath.Join(ctxDir, destName)
		if _, err := os.Stat(destPath); err == nil {
			result.Skipped = append(result.Skipped, ".ctx/"+destName)
			continue
		}

		data, err := templates.FS.ReadFile(tmplName)
		if err != nil {
			return result, fmt.Errorf("reading template %s: %w", tmplName, err)
		}

		if err := os.WriteFile(destPath, data, 0644); err != nil {
			return result, fmt.Errorf("writing %s: %w", destPath, err)
		}
		result.Created = append(result.Created, ".ctx/"+destName)
	}

	// Pre-fill state.yaml with options
	state, err := ctx.ReadState(dir)
	if err != nil {
		return result, fmt.Errorf("reading state for pre-fill: %w", err)
	}
	if opts.Name != "" {
		state.Project.Name = opts.Name
	}
	if opts.Stack != "" {
		state.Project.Stack = opts.Stack
	}
	state.Project.DocsPath = opts.DocsPath
	if err := ctx.WriteState(dir, state); err != nil {
		return result, fmt.Errorf("writing state: %w", err)
	}

	// Inject CLAUDE.md section
	action, err := injectClaudeMD(dir)
	if err != nil {
		return result, fmt.Errorf("injecting CLAUDE.md: %w", err)
	}
	result.Updated = append(result.Updated, "CLAUDE.md ("+action+")")

	return result, nil
}

func injectClaudeMD(dir string) (string, error) {
	section, err := templates.FS.ReadFile("claude.md")
	if err != nil {
		return "", fmt.Errorf("reading claude.md template: %w", err)
	}
	sectionStr := string(section)

	claudePath := filepath.Join(dir, "CLAUDE.md")
	data, err := os.ReadFile(claudePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Create new file with just the section
			return "created", os.WriteFile(claudePath, section, 0644)
		}
		return "", err
	}

	content := string(data)
	startMarker := "<!-- golem:start -->"
	endMarker := "<!-- golem:end -->"

	startIdx := strings.Index(content, startMarker)
	endIdx := strings.Index(content, endMarker)

	if startIdx >= 0 && endIdx >= 0 {
		// Replace between markers (inclusive)
		newContent := content[:startIdx] + sectionStr + content[endIdx+len(endMarker):]
		return "updated", os.WriteFile(claudePath, []byte(newContent), 0644)
	}

	// Append to existing file
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += "\n" + sectionStr
	return "appended", os.WriteFile(claudePath, []byte(content), 0644)
}

// CtxExists checks if the .ctx/ directory exists.
func CtxExists(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".ctx"))
	return err == nil && info.IsDir()
}
