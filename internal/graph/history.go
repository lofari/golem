package graph

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// coChangedMinCount is the minimum number of commits two files must appear
// together in before a CO_CHANGED edge is created.
const coChangedMinCount = 3

// defaultHistoryDepth is the default maximum number of commits to index.
const defaultHistoryDepth = 500

// HistoryBuilder indexes git history into the graph store.
type HistoryBuilder struct {
	store *Store
	depth int
}

// NewHistoryBuilder creates a new HistoryBuilder.
func NewHistoryBuilder(store *Store, depth int) *HistoryBuilder {
	if depth <= 0 {
		depth = defaultHistoryDepth
	}
	return &HistoryBuilder{store: store, depth: depth}
}

// parsedCommit holds data parsed from a single git log entry.
type parsedCommit struct {
	sha         string
	authorEmail string
	authorName  string
	timestamp   int64
	subject     string
	files       []string
}

// Build performs a full history build: clears existing history data, parses
// git log output, and inserts commits, authors, and edges into the store.
func (h *HistoryBuilder) Build(projectPath string) error {
	if err := h.store.DeleteHistory(); err != nil {
		return fmt.Errorf("clearing history: %w", err)
	}

	args := []string{
		"--format=%H%n%ae%n%an%n%at%n%s",
		"-n", strconv.Itoa(h.depth),
		"--name-only",
	}
	output, err := gitLog(projectPath, args...)
	if err != nil {
		return fmt.Errorf("running git log: %w", err)
	}

	parsed := parseGitLog(output)
	if len(parsed) == 0 {
		return nil
	}

	if err := h.insertParsed(parsed); err != nil {
		return err
	}

	// Compute CO_CHANGED from the parsed data
	if err := h.computeCoChangedFromParsed(parsed); err != nil {
		return err
	}

	// Set metadata
	if err := h.store.SetMeta("history_last_sha", parsed[0].sha); err != nil {
		return err
	}
	return h.store.SetMeta("history_depth", strconv.Itoa(h.depth))
}

// Sync performs an incremental history update. If no prior build exists,
// it falls back to a full Build.
func (h *HistoryBuilder) Sync(projectPath string) error {
	lastSHA, err := h.store.GetMeta("history_last_sha")
	if err != nil {
		return fmt.Errorf("reading history_last_sha: %w", err)
	}
	if lastSHA == "" {
		return h.Build(projectPath)
	}

	output, err := gitLog(projectPath,
		"--format=%H%n%ae%n%an%n%at%n%s",
		"--name-only",
		lastSHA+"..HEAD",
	)
	if err != nil {
		// SHA may no longer exist (e.g. after rebase); fall back to full build
		return h.Build(projectPath)
	}

	parsed := parseGitLog(output)
	if len(parsed) == 0 {
		return nil
	}

	// Insert new commits, authors, MODIFIES, AUTHORED_BY
	if err := h.insertParsed(parsed); err != nil {
		return err
	}

	// Recompute CO_CHANGED from ALL MODIFIES edges in the database
	if err := h.recomputeCoChangedFromDB(); err != nil {
		return err
	}

	// Update last SHA to the most recent commit
	return h.store.SetMeta("history_last_sha", parsed[0].sha)
}

// insertParsed inserts commits, authors, MODIFIES, and AUTHORED_BY edges
// for the given parsed commits.
func (h *HistoryBuilder) insertParsed(parsed []parsedCommit) error {
	// Build commit and author slices
	commits := make([]Commit, len(parsed))
	authorMap := make(map[string]string) // email -> latest name
	for i, p := range parsed {
		commits[i] = Commit{
			SHA:         p.sha,
			Message:     p.subject,
			AuthorEmail: p.authorEmail,
			Timestamp:   p.timestamp,
		}
		// Keep the first occurrence (most recent commit) for author name
		if _, exists := authorMap[p.authorEmail]; !exists {
			authorMap[p.authorEmail] = p.authorName
		}
	}

	if err := h.store.InsertCommitBatch(commits); err != nil {
		return fmt.Errorf("inserting commits: %w", err)
	}

	// Build authors slice
	authors := make([]Author, 0, len(authorMap))
	for email, name := range authorMap {
		authors = append(authors, Author{Email: email, Name: name})
	}
	if err := h.store.InsertAuthorBatch(authors); err != nil {
		return fmt.Errorf("inserting authors: %w", err)
	}

	// Build MODIFIES and AUTHORED_BY edges
	var edges []Edge
	for _, p := range parsed {
		commitID := "commit:" + p.sha
		authorID := "author:" + p.authorEmail

		// AUTHORED_BY
		edges = append(edges, Edge{From: commitID, To: authorID, Type: "AUTHORED_BY"})

		// MODIFIES
		for _, file := range p.files {
			edges = append(edges, Edge{From: commitID, To: "file:" + file, Type: "MODIFIES"})
		}
	}

	if err := h.store.InsertBatch(nil, edges); err != nil {
		return fmt.Errorf("inserting history edges: %w", err)
	}

	return nil
}

// computeCoChangedFromParsed computes CO_CHANGED edges from in-memory parsed commits.
func (h *HistoryBuilder) computeCoChangedFromParsed(parsed []parsedCommit) error {
	// Count file pair co-occurrences
	pairCount := make(map[[2]string]int)
	for _, p := range parsed {
		if len(p.files) < 2 {
			continue
		}
		// Sort-free canonical pair: use lexicographic comparison
		for i := 0; i < len(p.files); i++ {
			for j := i + 1; j < len(p.files); j++ {
				a, b := p.files[i], p.files[j]
				if a > b {
					a, b = b, a
				}
				pairCount[[2]string{a, b}]++
			}
		}
	}

	for pair, count := range pairCount {
		if count >= coChangedMinCount {
			from := "file:" + pair[0]
			to := "file:" + pair[1]
			if err := h.store.InsertEdgeWithWeight(from, to, "CO_CHANGED", count); err != nil {
				return fmt.Errorf("inserting CO_CHANGED edge: %w", err)
			}
		}
	}
	return nil
}

// recomputeCoChangedFromDB drops all CO_CHANGED edges and recomputes them
// from ALL MODIFIES edges in the database.
func (h *HistoryBuilder) recomputeCoChangedFromDB() error {
	// Delete existing CO_CHANGED edges
	_, err := h.store.db.Exec("DELETE FROM edges WHERE type = 'CO_CHANGED'")
	if err != nil {
		return fmt.Errorf("deleting CO_CHANGED edges: %w", err)
	}

	// Query all MODIFIES edges, grouped by commit
	rows, err := h.store.db.Query(
		"SELECT from_node, to_node FROM edges WHERE type = 'MODIFIES' ORDER BY from_node",
	)
	if err != nil {
		return fmt.Errorf("querying MODIFIES edges: %w", err)
	}
	defer rows.Close()

	// Group files by commit
	commitFiles := make(map[string][]string)
	for rows.Next() {
		var from, to string
		if err := rows.Scan(&from, &to); err != nil {
			return err
		}
		// from is "commit:<sha>", to is "file:<path>"
		file := strings.TrimPrefix(to, "file:")
		commitFiles[from] = append(commitFiles[from], file)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// Count pair co-occurrences
	pairCount := make(map[[2]string]int)
	for _, files := range commitFiles {
		if len(files) < 2 {
			continue
		}
		for i := 0; i < len(files); i++ {
			for j := i + 1; j < len(files); j++ {
				a, b := files[i], files[j]
				if a > b {
					a, b = b, a
				}
				pairCount[[2]string{a, b}]++
			}
		}
	}

	for pair, count := range pairCount {
		if count >= coChangedMinCount {
			from := "file:" + pair[0]
			to := "file:" + pair[1]
			if err := h.store.InsertEdgeWithWeight(from, to, "CO_CHANGED", count); err != nil {
				return fmt.Errorf("inserting CO_CHANGED edge: %w", err)
			}
		}
	}
	return nil
}

// gitLog runs `git log` with the given arguments in the specified directory.
func gitLog(dir string, args ...string) (string, error) {
	cmdArgs := append([]string{"log"}, args...)
	cmd := exec.Command("git", cmdArgs...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git log: %w: %s", err, stderr.String())
	}
	return string(out), nil
}

// isSHA returns true if the string looks like a full git SHA (40 hex chars).
func isSHA(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// parseGitLog parses the output of git log with format '%H%n%ae%n%an%n%at%n%s'
// and --name-only into parsedCommit structs.
//
// The output format per commit is:
//
//	SHA (40 hex chars)
//	author email
//	author name
//	unix timestamp
//	subject
//	<blank line>       ← inserted by --name-only between header and files
//	file1
//	file2
//	...                ← next commit SHA follows immediately (no blank separator)
func parseGitLog(output string) []parsedCommit {
	output = strings.TrimSpace(output)
	if output == "" {
		return nil
	}

	var result []parsedCommit
	lines := strings.Split(output, "\n")

	i := 0
	for i < len(lines) {
		// Skip blank lines
		for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
			i++
		}
		if i >= len(lines) {
			break
		}

		// We need at least 5 lines: SHA, email, name, timestamp, subject
		if i+4 >= len(lines) {
			break
		}

		sha := strings.TrimSpace(lines[i])
		if !isSHA(sha) {
			i++
			continue
		}
		email := strings.TrimSpace(lines[i+1])
		name := strings.TrimSpace(lines[i+2])
		tsStr := strings.TrimSpace(lines[i+3])
		subject := strings.TrimSpace(lines[i+4])
		i += 5

		ts, err := strconv.ParseInt(tsStr, 10, 64)
		if err != nil {
			continue
		}

		// Skip blank line(s) between header and file list (inserted by --name-only)
		for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
			i++
		}

		// Collect file paths until we hit a SHA line (next commit) or end
		var files []string
		for i < len(lines) {
			line := strings.TrimSpace(lines[i])
			if line == "" || isSHA(line) {
				break
			}
			files = append(files, line)
			i++
		}

		result = append(result, parsedCommit{
			sha:         sha,
			authorEmail: email,
			authorName:  name,
			timestamp:   ts,
			subject:     subject,
			files:       files,
		})
	}

	return result
}
