package graph

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/lofari/golem/internal/graph/lsp"
	"github.com/lofari/golem/internal/graph/markdown"
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
	store        *Store
	historyDepth int
	lspManager   *lsp.Manager
}

// NewBuilder creates a new graph builder. An optional historyDepth controls
// how many git commits are indexed (default 500).
func NewBuilder(store *Store, historyDepth ...int) *Builder {
	depth := 500
	if len(historyDepth) > 0 && historyDepth[0] > 0 {
		depth = historyDepth[0]
	}
	return &Builder{store: store, historyDepth: depth}
}

// WithLSP sets the LSP manager for enhanced extraction.
func (b *Builder) WithLSP(mgr *lsp.Manager) {
	b.lspManager = mgr
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

		// Try LSP extraction first
		if b.extractWithLSP(projectPath, relPath, &allNodes, &allEdges) {
			return nil
		}

		// Tree-sitter fallback
		src, err := os.ReadFile(path)
		if err != nil {
			return nil // skip unreadable files
		}

		lang := treesitter.DetectLanguage(relPath)
		if lang == "" {
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

	// Index documentation
	if err := b.indexDocs(projectPath, allNodes); err != nil {
		return fmt.Errorf("index docs: %w", err)
	}

	// Index git history (best-effort — project may not be a git repo)
	hb := NewHistoryBuilder(b.store, b.historyDepth)
	if err := hb.Build(projectPath); err != nil {
		// Non-fatal: history is optional (project may not be a git repo)
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

	hasChangedDocs := false
	for _, relPath := range changed {
		// Remove old nodes/edges for this file
		b.store.DeleteByPath(relPath)

		if strings.HasSuffix(relPath, ".md") {
			hasChangedDocs = true
		}

		// Check if file still exists
		fullPath := filepath.Join(projectPath, relPath)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			continue // file was deleted
		}

		// Try LSP extraction first
		var nodes []Node
		var edges []Edge
		if b.extractWithLSP(projectPath, relPath, &nodes, &edges) {
			b.store.InsertBatch(nodes, edges)
			continue
		}

		// Tree-sitter fallback
		src, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}

		lang := treesitter.DetectLanguage(relPath)
		if lang == "" {
			n, e := treesitter.ExtractFileOnly(relPath)
			b.store.InsertBatch(n, e)
			continue
		}

		tree, _, err := treesitter.ParseBytes(src, lang)
		if err != nil {
			continue
		}

		nodes, edges = treesitter.Extract(relPath, lang, tree, src)
		b.store.InsertBatch(nodes, edges)
	}

	// Re-index docs if any markdown files changed
	if hasChangedDocs {
		var allCodeNodes []Node
		for _, typ := range []string{"function", "method", "type"} {
			nodes, _ := b.store.NodesByType(typ)
			allCodeNodes = append(allCodeNodes, nodes...)
		}
		if err := b.indexDocs(projectPath, allCodeNodes); err != nil {
			return fmt.Errorf("re-index docs: %w", err)
		}
	}

	// Sync git history (best-effort — project may not be a git repo)
	hb := NewHistoryBuilder(b.store, b.historyDepth)
	if err := hb.Sync(projectPath); err != nil {
		// Non-fatal: history is optional (project may not be a git repo)
	}

	// Update metadata
	b.store.SetMeta("last_indexed", time.Now().Format(time.RFC3339))
	if sha := gitHeadSHA(projectPath); sha != "" {
		b.store.SetMeta("last_commit", sha)
	}

	return nil
}

// indexDocs walks markdown files, parses them, and creates document/section nodes with edges.
func (b *Builder) indexDocs(projectPath string, existingNodes []Node) error {
	// Build name->node lookup for code linking
	nameIndex := map[string][]Node{}
	for _, n := range existingNodes {
		if n.Type == "function" || n.Type == "method" || n.Type == "type" {
			nameIndex[n.Name] = append(nameIndex[n.Name], n)
		}
	}

	var docNodes []Node
	var docEdges []Edge

	err := filepath.WalkDir(projectPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skipDirs[d.Name()] || strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}

		relPath, _ := filepath.Rel(projectPath, path)
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		sections, err := markdown.ParseMarkdown(relPath, content)
		if err != nil {
			return nil
		}
		if len(sections) == 0 {
			return nil
		}

		// Create document node
		docID := fmt.Sprintf("doc:%s", relPath)
		docNodes = append(docNodes, Node{
			ID:   docID,
			Type: "document",
			Name: filepath.Base(relPath),
			Path: relPath,
			Line: 1,
		})

		for _, sec := range sections {
			secID := fmt.Sprintf("sec:%s:%s", relPath, sec.Heading)
			docNodes = append(docNodes, Node{
				ID:   secID,
				Type: "section",
				Name: sec.Heading,
				Path: relPath,
				Line: sec.Line,
			})

			// CONTAINS edge: document -> section
			docEdges = append(docEdges, Edge{From: docID, To: secID, Type: "CONTAINS"})

			// REFERENCES edges: section -> code symbols found in backtick refs
			for _, ref := range sec.Refs {
				if targets, ok := nameIndex[ref]; ok {
					for _, target := range targets {
						docEdges = append(docEdges, Edge{From: secID, To: target.ID, Type: "REFERENCES"})
					}
				}
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	if len(docNodes) > 0 {
		return b.store.InsertBatch(docNodes, docEdges)
	}
	return nil
}

// extractWithLSP tries to extract nodes/edges using LSP. Returns true if successful.
func (b *Builder) extractWithLSP(projectPath, relPath string, nodes *[]Node, edges *[]Edge) bool {
	if b.lspManager == nil {
		return false
	}
	cfg := lsp.ConfigForExt(filepath.Ext(relPath))
	if cfg == nil {
		return false
	}
	client := b.lspManager.ClientFor(cfg.Language)
	if client == nil {
		return false
	}
	n, e, err := lsp.Extract(client, projectPath, relPath)
	if err != nil {
		return false
	}
	*nodes = append(*nodes, n...)
	*edges = append(*edges, e...)
	return true
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
