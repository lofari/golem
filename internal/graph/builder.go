package graph

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/lofari/golem/internal/graph/treesitter"
)

// skipDirs are directories that should never be indexed.
var skipDirs = map[string]bool{
	".ctx":         true,
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	".venv":        true,
	"__pycache__":  true,
	".godot":       true,
	"build":        true,
	"dist":         true,
	".next":        true,
}

// Builder constructs and updates the code graph.
type Builder struct {
	store *Store
}

// NewBuilder creates a new graph builder.
func NewBuilder(store *Store) *Builder {
	return &Builder{store: store}
}

// BuildFull does a complete rebuild of the graph from the project directory.
func (b *Builder) BuildFull(projectPath string) error {
	if err := b.store.Clear(); err != nil {
		return fmt.Errorf("clearing graph: %w", err)
	}

	var allNodes []Node
	var allEdges []Edge

	err := filepath.WalkDir(projectPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}

		// Skip excluded directories
		if d.IsDir() {
			if skipDirs[d.Name()] || strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}

		// Get relative path
		relPath, _ := filepath.Rel(projectPath, path)

		// Read file
		src, err := os.ReadFile(path)
		if err != nil {
			return nil // skip unreadable files
		}

		// Parse and extract
		lang := treesitter.DetectLanguage(relPath)
		if lang == "" {
			// Unsupported language — create file node only
			nodes, edges := treesitter.ExtractFileOnly(relPath)
			allNodes = append(allNodes, nodes...)
			allEdges = append(allEdges, edges...)
			return nil
		}

		tree, _, err := treesitter.ParseBytes(src, lang)
		if err != nil {
			return nil // skip parse errors
		}

		nodes, edges := treesitter.Extract(relPath, lang, tree, src)
		allNodes = append(allNodes, nodes...)
		allEdges = append(allEdges, edges...)

		return nil
	})
	if err != nil {
		return fmt.Errorf("walking project: %w", err)
	}

	if err := b.store.InsertBatch(allNodes, allEdges); err != nil {
		return fmt.Errorf("inserting graph: %w", err)
	}

	// Record indexing metadata
	b.store.SetMeta("last_indexed", time.Now().Format(time.RFC3339))
	if sha := gitHeadSHA(projectPath); sha != "" {
		b.store.SetMeta("last_commit", sha)
	}

	return nil
}

// Sync performs an incremental update based on git changes since last index.
// Falls back to full build if no baseline exists.
func (b *Builder) Sync(projectPath string) error {
	lastCommit, _ := b.store.GetMeta("last_commit")
	if lastCommit == "" {
		return b.BuildFull(projectPath)
	}

	// Get changed files since last indexed commit
	changed, err := gitChangedFiles(projectPath, lastCommit)
	if err != nil {
		// Can't diff — do full rebuild
		return b.BuildFull(projectPath)
	}

	// Also include uncommitted changes
	dirty, _ := gitDirtyFiles(projectPath)
	changed = append(changed, dirty...)
	changed = dedupe(changed)

	if len(changed) == 0 {
		return nil // nothing to update
	}

	for _, relPath := range changed {
		// Remove old nodes/edges for this file
		b.store.DeleteByPath(relPath)

		// Check if file still exists
		fullPath := filepath.Join(projectPath, relPath)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			continue // file was deleted
		}

		// Re-parse and insert
		src, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}

		lang := treesitter.DetectLanguage(relPath)
		if lang == "" {
			nodes, edges := treesitter.ExtractFileOnly(relPath)
			b.store.InsertBatch(nodes, edges)
			continue
		}

		tree, _, err := treesitter.ParseBytes(src, lang)
		if err != nil {
			continue
		}

		nodes, edges := treesitter.Extract(relPath, lang, tree, src)
		b.store.InsertBatch(nodes, edges)
	}

	// Update metadata
	b.store.SetMeta("last_indexed", time.Now().Format(time.RFC3339))
	if sha := gitHeadSHA(projectPath); sha != "" {
		b.store.SetMeta("last_commit", sha)
	}

	return nil
}

// --- Git helpers ---

func gitHeadSHA(dir string) string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func gitChangedFiles(dir, sinceCommit string) ([]string, error) {
	cmd := exec.Command("git", "diff", "--name-only", sinceCommit+"..HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return splitLines(string(out)), nil
}

func gitDirtyFiles(dir string) ([]string, error) {
	cmd := exec.Command("git", "diff", "--name-only")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	staged := exec.Command("git", "diff", "--name-only", "--cached")
	staged.Dir = dir
	out2, _ := staged.Output()
	files := splitLines(string(out))
	files = append(files, splitLines(string(out2))...)
	return files, nil
}

func splitLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func dedupe(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
