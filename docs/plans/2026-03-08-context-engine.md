# Context Engine Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a context engine that automatically analyzes the current task and knowledge graph to produce a ranked "context map" of relevant symbols, injected into each iteration's prompt.

**Architecture:** New `internal/graph/context/` package with a multi-signal ranking pipeline (semantic + structural + co-change + recency + failures). Called by the builder loop after graph sync, output rendered into prompt via `{{CONTEXT_MAP}}` placeholder.

**Tech Stack:** Go stdlib, existing graph store/embedder, sqlite-vec for semantic search.

---

### Task 1: Core types and Format method

**Files:**
- Create: `internal/graph/context/context.go`
- Create: `internal/graph/context/context_test.go`

**Step 1: Write the failing test**

```go
// internal/graph/context/context_test.go
package context

import (
	"strings"
	"testing"
)

func TestContextMap_Format_Empty(t *testing.T) {
	cm := &ContextMap{Task: "fix login"}
	got := cm.Format()
	if got != "" {
		t.Errorf("expected empty string for no symbols, got %q", got)
	}
}

func TestContextMap_Format(t *testing.T) {
	cm := &ContextMap{
		Task: "fix login",
		Symbols: []SymbolEntry{
			{
				Name:      "ValidateCredentials",
				Kind:      "function",
				Path:      "auth/login.go",
				Line:      45,
				Relations: []string{"calls CheckPassword", "called by LoginHandler"},
			},
			{
				Name:      "SessionMiddleware",
				Kind:      "method",
				Path:      "middleware/session.go",
				Line:      12,
				Relations: []string{"calls ValidateToken"},
			},
		},
	}
	got := cm.Format()

	if !strings.Contains(got, "## Relevant Context") {
		t.Error("missing header")
	}
	if !strings.Contains(got, "`ValidateCredentials` function (auth/login.go:45)") {
		t.Error("missing first symbol")
	}
	if !strings.Contains(got, "calls CheckPassword, called by LoginHandler") {
		t.Error("missing relations for first symbol")
	}
	if !strings.Contains(got, "`SessionMiddleware` method (middleware/session.go:12)") {
		t.Error("missing second symbol")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/graph/context/ -run TestContextMap_Format -v`
Expected: FAIL — package does not exist yet.

**Step 3: Write minimal implementation**

```go
// internal/graph/context/context.go
package context

import (
	"fmt"
	"strings"
)

// ContextMap holds ranked symbols relevant to a task.
type ContextMap struct {
	Task    string
	Symbols []SymbolEntry
}

// SymbolEntry is a single relevant symbol with location and relationships.
type SymbolEntry struct {
	Name      string
	Kind      string   // function, method, type, file
	Path      string
	Line      int
	Score     float64  // internal ranking score
	Relations []string // e.g. "calls CheckPassword"
}

// Format renders the context map as a markdown section for prompt injection.
// Returns empty string if there are no symbols.
func (cm *ContextMap) Format() string {
	if cm == nil || len(cm.Symbols) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Relevant Context\n\n")
	b.WriteString("The following symbols are relevant to your current task. Start here.\n\n")

	for _, s := range cm.Symbols {
		b.WriteString(fmt.Sprintf("- `%s` %s (%s:%d)", s.Name, s.Kind, s.Path, s.Line))
		if len(s.Relations) > 0 {
			b.WriteString(" — ")
			b.WriteString(strings.Join(s.Relations, ", "))
		}
		b.WriteString("\n")
	}

	return b.String()
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/graph/context/ -run TestContextMap_Format -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/graph/context/context.go internal/graph/context/context_test.go
git commit -m "feat(context): add ContextMap types and Format method"
```

---

### Task 2: Semantic search stage

**Files:**
- Create: `internal/graph/context/engine.go`
- Modify: `internal/graph/context/context_test.go`

**Step 1: Write the failing test**

Add to `context_test.go`:

```go
func TestSemanticCandidates(t *testing.T) {
	store := setupTestStore(t) // helper that creates in-memory graph.db
	embedder := &fakeEmbedder{} // returns deterministic vectors

	// Insert nodes with embeddings
	nodes := []graph.Node{
		{ID: "func:auth/login.go:ValidateCredentials", Type: "function", Name: "ValidateCredentials", Path: "auth/login.go", Line: 45},
		{ID: "func:auth/crypto.go:CheckPassword", Type: "function", Name: "CheckPassword", Path: "auth/crypto.go", Line: 30},
		{ID: "func:util/logger.go:LogInfo", Type: "function", Name: "LogInfo", Path: "util/logger.go", Line: 10},
	}
	store.InsertBatch(nodes, nil)

	// Embed with known vectors — ValidateCredentials is closest to query
	store.InsertEmbedding(nodes[0].ID, embedder.vectorFor("validate credentials login"))
	store.InsertEmbedding(nodes[1].ID, embedder.vectorFor("check password hash"))
	store.InsertEmbedding(nodes[2].ID, embedder.vectorFor("log info message"))

	candidates, err := semanticCandidates(store, embedder, "fix login validation", 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) == 0 {
		t.Fatal("expected candidates")
	}
	// First candidate should be the most semantically similar
	if candidates[0].Node.Name != "ValidateCredentials" {
		t.Errorf("expected ValidateCredentials first, got %s", candidates[0].Node.Name)
	}
}
```

Test helpers (add to `context_test.go`):

```go
import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lofari/golem/internal/graph"
)

func setupTestStore(t *testing.T) *graph.Store {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "graph.db")
	store, err := graph.OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// fakeEmbedder returns deterministic vectors for testing.
type fakeEmbedder struct{}

func (f *fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	vecs := make([][]float32, len(texts))
	for i, text := range texts {
		vecs[i] = f.vectorFor(text)
	}
	return vecs, nil
}

func (f *fakeEmbedder) Dimensions() int { return 384 }
func (f *fakeEmbedder) Close() error    { return nil }

// vectorFor produces a deterministic 384-dim vector based on text content.
// Similar texts produce similar vectors via simple character frequency hashing.
func (f *fakeEmbedder) vectorFor(text string) []float32 {
	vec := make([]float32, 384)
	for i, c := range text {
		idx := (int(c) + i) % 384
		vec[idx] += 1.0
	}
	// Normalize
	var norm float32
	for _, v := range vec {
		norm += v * v
	}
	if norm > 0 {
		norm = float32(1.0 / float64(norm))
		for i := range vec {
			vec[i] *= norm
		}
	}
	return vec
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/graph/context/ -run TestSemanticCandidates -v`
Expected: FAIL — `semanticCandidates` undefined.

**Step 3: Write minimal implementation**

```go
// internal/graph/context/engine.go
package context

import (
	gocontext "context"
	"fmt"

	"github.com/lofari/golem/internal/graph"
	"github.com/lofari/golem/internal/graph/embed"
)

// candidate is an internal scored symbol during ranking.
type candidate struct {
	Node  graph.Node
	Score float64
}

// semanticCandidates finds nodes semantically similar to the task text.
func semanticCandidates(store *graph.Store, embedder embed.Embedder, taskText string, limit int) ([]candidate, error) {
	count, err := store.EmbeddingCount()
	if err != nil || count == 0 {
		return nil, nil // no embeddings — graceful degradation
	}

	vecs, err := embedder.Embed(gocontext.Background(), []string{taskText})
	if err != nil {
		return nil, fmt.Errorf("embedding task text: %w", err)
	}

	similar, err := store.SearchSimilar(vecs[0], limit)
	if err != nil {
		return nil, fmt.Errorf("searching similar: %w", err)
	}

	var candidates []candidate
	for _, r := range similar {
		node, err := store.NodeByID(r.NodeID)
		if err != nil || node == nil {
			continue
		}
		score := float64(1.0 - r.Distance)
		if score < 0 {
			score = 0
		}
		candidates = append(candidates, candidate{
			Node:  *node,
			Score: score,
		})
	}

	return candidates, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/graph/context/ -run TestSemanticCandidates -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/graph/context/engine.go internal/graph/context/context_test.go
git commit -m "feat(context): add semantic search stage for context engine"
```

---

### Task 3: Structural expansion stage

**Files:**
- Modify: `internal/graph/context/engine.go`
- Modify: `internal/graph/context/context_test.go`

**Step 1: Write the failing test**

Add to `context_test.go`:

```go
func TestStructuralExpansion(t *testing.T) {
	store := setupTestStore(t)

	// Insert nodes
	nodes := []graph.Node{
		{ID: "func:a.go:Foo", Type: "function", Name: "Foo", Path: "a.go", Line: 10},
		{ID: "func:b.go:Bar", Type: "function", Name: "Bar", Path: "b.go", Line: 20},
		{ID: "func:c.go:Baz", Type: "function", Name: "Baz", Path: "c.go", Line: 30},
	}
	edges := []graph.Edge{
		{From: "func:a.go:Foo", To: "func:b.go:Bar", Type: "CALLS"},
		{From: "func:b.go:Bar", To: "func:c.go:Baz", Type: "CALLS"},
	}
	store.InsertBatch(nodes, edges)

	// Start with Foo as a semantic candidate
	seeds := []candidate{
		{Node: nodes[0], Score: 0.9},
	}

	expanded := structuralExpansion(store, seeds)

	// Should include Bar (1-hop from Foo) with reduced score
	found := false
	for _, c := range expanded {
		if c.Node.Name == "Bar" {
			found = true
			expected := 0.9 * structuralDecay
			if c.Score < expected-0.01 || c.Score > expected+0.01 {
				t.Errorf("Bar score = %f, want ~%f", c.Score, expected)
			}
		}
	}
	if !found {
		t.Error("expected Bar in expanded candidates")
	}

	// Should NOT include Baz (2 hops away, we only do 1-hop)
	for _, c := range expanded {
		if c.Node.Name == "Baz" {
			t.Error("did not expect Baz (2 hops away)")
		}
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/graph/context/ -run TestStructuralExpansion -v`
Expected: FAIL — `structuralExpansion` undefined.

**Step 3: Write minimal implementation**

Add to `engine.go`:

```go
const structuralDecay = 0.7

// structuralExpansion adds 1-hop callers/callees of seed candidates.
func structuralExpansion(store *graph.Store, seeds []candidate) []candidate {
	seen := make(map[string]bool)
	for _, s := range seeds {
		seen[s.Node.ID] = true
	}

	var expanded []candidate
	for _, seed := range seeds {
		// Outgoing edges (callees, dependencies)
		outEdges, _ := store.EdgesFrom(seed.Node.ID)
		for _, e := range outEdges {
			if seen[e.To] {
				continue
			}
			seen[e.To] = true
			if node, _ := store.NodeByID(e.To); node != nil {
				expanded = append(expanded, candidate{
					Node:  *node,
					Score: seed.Score * structuralDecay,
				})
			}
		}

		// Incoming edges (callers, dependents)
		inEdges, _ := store.EdgesTo(seed.Node.ID)
		for _, e := range inEdges {
			if seen[e.From] {
				continue
			}
			seen[e.From] = true
			if node, _ := store.NodeByID(e.From); node != nil {
				expanded = append(expanded, candidate{
					Node:  *node,
					Score: seed.Score * structuralDecay,
				})
			}
		}
	}

	return expanded
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/graph/context/ -run TestStructuralExpansion -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/graph/context/engine.go internal/graph/context/context_test.go
git commit -m "feat(context): add structural expansion stage"
```

---

### Task 4: Co-change, recency, and failure boost stages

**Files:**
- Modify: `internal/graph/context/engine.go`
- Modify: `internal/graph/context/context_test.go`

**Step 1: Write the failing tests**

Add to `context_test.go`:

```go
func TestCoChangeBoost(t *testing.T) {
	store := setupTestStore(t)

	nodes := []graph.Node{
		{ID: "file:a.go", Type: "file", Name: "a.go", Path: "a.go", Line: 0},
		{ID: "file:b.go", Type: "file", Name: "b.go", Path: "b.go", Line: 0},
	}
	store.InsertBatch(nodes, nil)
	store.InsertEdgeWithWeight("file:a.go", "file:b.go", "CO_CHANGED", 5)

	candidates := []candidate{
		{Node: graph.Node{ID: "func:a.go:Foo", Type: "function", Name: "Foo", Path: "a.go", Line: 10}, Score: 0.5},
		{Node: graph.Node{ID: "func:b.go:Bar", Type: "function", Name: "Bar", Path: "b.go", Line: 20}, Score: 0.5},
	}

	boosted := coChangeBoost(store, candidates)

	// Both should be boosted because their files co-change
	for _, c := range boosted {
		if c.Score <= 0.5 {
			t.Errorf("%s score should be boosted above 0.5, got %f", c.Node.Name, c.Score)
		}
	}
}

func TestRecencyBoost(t *testing.T) {
	store := setupTestStore(t)

	// Insert a file node and a recent commit that modifies it
	nodes := []graph.Node{
		{ID: "file:a.go", Type: "file", Name: "a.go", Path: "a.go", Line: 0},
	}
	store.InsertBatch(nodes, nil)
	store.InsertCommitBatch([]graph.Commit{
		{SHA: "abc123", Message: "fix a.go", AuthorEmail: "dev@test.com", Timestamp: 9999999999, Additions: 10, Deletions: 5},
	})
	store.InsertBatch(nil, []graph.Edge{
		{From: "commit:abc123", To: "file:a.go", Type: "MODIFIES"},
	})

	candidates := []candidate{
		{Node: graph.Node{ID: "func:a.go:Foo", Type: "function", Name: "Foo", Path: "a.go", Line: 10}, Score: 0.5},
	}

	boosted := recencyBoost(store, candidates, 10)
	if boosted[0].Score <= 0.5 {
		t.Errorf("expected recency boost, got %f", boosted[0].Score)
	}
}

func TestFailureBoost(t *testing.T) {
	store := setupTestStore(t)

	// Insert a failed test result linked to a file
	store.InsertExecution("session-1", 1000, "failed")
	store.InsertTestResult("test:session-1:TestFoo", "session-1", "TestFoo", false, 100, "assertion failed")

	candidates := []candidate{
		{Node: graph.Node{ID: "func:a.go:Foo", Type: "function", Name: "Foo", Path: "a.go", Line: 10}, Score: 0.5},
	}

	boosted := failureBoost(store, candidates)
	if boosted[0].Score <= 0.5 {
		t.Errorf("expected failure boost, got %f", boosted[0].Score)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/graph/context/ -run "TestCoChangeBoost|TestRecencyBoost|TestFailureBoost" -v`
Expected: FAIL — functions undefined.

**Step 3: Write minimal implementation**

Add to `engine.go`:

```go
import "strings"

const (
	coChangeBoostPerLink = 0.1
	recencyBoostMax      = 0.15
	failureBoostValue    = 0.2
)

// coChangeBoost boosts candidates whose files co-change with other candidates' files.
func coChangeBoost(store *graph.Store, candidates []candidate) []candidate {
	// Collect all file paths in candidate set
	filePaths := make(map[string]bool)
	for _, c := range candidates {
		if c.Node.Path != "" {
			filePaths[c.Node.Path] = true
		}
	}

	for i, c := range candidates {
		if c.Node.Path == "" {
			continue
		}
		coChanged, err := store.QueryCoChanged(c.Node.Path, 1)
		if err != nil {
			continue
		}
		for _, cc := range coChanged {
			if filePaths[cc.File] {
				candidates[i].Score += coChangeBoostPerLink
			}
		}
	}

	return candidates
}

// recencyBoost boosts candidates whose files were modified in recent commits.
func recencyBoost(store *graph.Store, candidates []candidate, recentN int) []candidate {
	for i, c := range candidates {
		if c.Node.Path == "" {
			continue
		}
		commits, err := store.QueryCommitsByFile(c.Node.Path, recentN)
		if err != nil || len(commits) == 0 {
			continue
		}
		// Boost decays by position: most recent commit = full boost
		boost := recencyBoostMax * (1.0 - float64(0)/float64(recentN))
		candidates[i].Score += boost
	}

	return candidates
}

// failureBoost boosts candidates that match recent test failures.
func failureBoost(store *graph.Store, candidates []candidate) []candidate {
	// Get latest execution session
	latest, err := store.LatestExecution()
	if err != nil || latest == nil {
		return candidates
	}

	// Get failed tests from latest session
	failedTests, err := store.QueryTestResults(latest.SessionID, "failed")
	if err != nil {
		return candidates
	}

	failedNames := make(map[string]bool)
	for _, t := range failedTests {
		// Store lowercase test name for fuzzy matching
		failedNames[strings.ToLower(t.Name)] = true
	}

	for i, c := range candidates {
		// Check if any failed test name contains the candidate's name
		candidateLower := strings.ToLower(c.Node.Name)
		for name := range failedNames {
			if strings.Contains(name, candidateLower) || strings.Contains(candidateLower, name) {
				candidates[i].Score += failureBoostValue
				break
			}
		}
	}

	return candidates
}
```

Note: The test for `failureBoost` uses `store.InsertExecution` and `store.InsertTestResult` which are helper methods on `Store` already used by the execution collector. If they are not directly exposed, the test should use the execution collector's SQL inserts directly. Check `internal/graph/store_execution.go` for the exact insert methods available — if `InsertExecution` doesn't exist as a public method, add a test helper that inserts directly via SQL.

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/graph/context/ -run "TestCoChangeBoost|TestRecencyBoost|TestFailureBoost" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/graph/context/engine.go internal/graph/context/context_test.go
git commit -m "feat(context): add co-change, recency, and failure boost stages"
```

---

### Task 5: BuildContextMap orchestrator and deduplication

**Files:**
- Modify: `internal/graph/context/engine.go`
- Modify: `internal/graph/context/context_test.go`

**Step 1: Write the failing tests**

Add to `context_test.go`:

```go
func TestBuildContextMap_NoEmbeddings(t *testing.T) {
	store := setupTestStore(t)
	embedder := &fakeEmbedder{}

	task := "fix login"
	cm, err := BuildContextMap(store, embedder, task, 15)
	if err != nil {
		t.Fatal(err)
	}
	if len(cm.Symbols) != 0 {
		t.Errorf("expected no symbols when no embeddings, got %d", len(cm.Symbols))
	}
}

func TestBuildContextMap_Deduplication(t *testing.T) {
	store := setupTestStore(t)
	embedder := &fakeEmbedder{}

	// Insert a node with embedding
	nodes := []graph.Node{
		{ID: "func:a.go:Foo", Type: "function", Name: "Foo", Path: "a.go", Line: 10},
		{ID: "func:b.go:Bar", Type: "function", Name: "Bar", Path: "b.go", Line: 20},
	}
	edges := []graph.Edge{
		{From: "func:a.go:Foo", To: "func:b.go:Bar", Type: "CALLS"},
	}
	store.InsertBatch(nodes, edges)
	store.InsertEmbedding(nodes[0].ID, embedder.vectorFor("foo function"))
	store.InsertEmbedding(nodes[1].ID, embedder.vectorFor("bar function"))

	cm, err := BuildContextMap(store, embedder, "foo bar", 15)
	if err != nil {
		t.Fatal(err)
	}

	// Each symbol should appear at most once
	seen := make(map[string]int)
	for _, s := range cm.Symbols {
		seen[s.Name]++
	}
	for name, count := range seen {
		if count > 1 {
			t.Errorf("symbol %s appears %d times, expected 1", name, count)
		}
	}
}

func TestBuildContextMap_Limit(t *testing.T) {
	store := setupTestStore(t)
	embedder := &fakeEmbedder{}

	// Insert many nodes
	var nodes []graph.Node
	for i := 0; i < 20; i++ {
		name := fmt.Sprintf("Func%d", i)
		id := fmt.Sprintf("func:f%d.go:%s", i, name)
		path := fmt.Sprintf("f%d.go", i)
		nodes = append(nodes, graph.Node{ID: id, Type: "function", Name: name, Path: path, Line: i + 1})
	}
	store.InsertBatch(nodes, nil)
	for _, n := range nodes {
		store.InsertEmbedding(n.ID, embedder.vectorFor(n.Name))
	}

	cm, err := BuildContextMap(store, embedder, "some task", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(cm.Symbols) > 5 {
		t.Errorf("expected at most 5 symbols, got %d", len(cm.Symbols))
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/graph/context/ -run "TestBuildContextMap" -v`
Expected: FAIL — `BuildContextMap` undefined.

**Step 3: Write minimal implementation**

Add to `engine.go`:

```go
import "sort"

const (
	semanticSearchLimit = 30
	recentCommitWindow  = 10
)

// BuildContextMap produces a ranked context map for the given task.
// Returns an empty map (not an error) when embeddings are unavailable.
func BuildContextMap(store *graph.Store, embedder embed.Embedder, taskText string, limit int) (*ContextMap, error) {
	cm := &ContextMap{Task: taskText}

	if limit < 1 {
		limit = 15
	}

	// Stage 1: Semantic search
	seeds, err := semanticCandidates(store, embedder, taskText, semanticSearchLimit)
	if err != nil {
		return cm, fmt.Errorf("semantic search: %w", err)
	}
	if len(seeds) == 0 {
		return cm, nil
	}

	// Stage 2: Structural expansion
	expanded := structuralExpansion(store, seeds)

	// Merge seeds + expanded
	all := append(seeds, expanded...)

	// Stage 3: Co-change boost
	all = coChangeBoost(store, all)

	// Stage 4: Recency boost
	all = recencyBoost(store, all, recentCommitWindow)

	// Stage 5: Failure boost
	all = failureBoost(store, all)

	// Deduplicate: keep highest score per node ID
	best := make(map[string]candidate)
	for _, c := range all {
		if existing, ok := best[c.Node.ID]; !ok || c.Score > existing.Score {
			best[c.Node.ID] = c
		}
	}

	// Convert to slice and sort
	deduped := make([]candidate, 0, len(best))
	for _, c := range best {
		deduped = append(deduped, c)
	}
	sort.Slice(deduped, func(i, j int) bool {
		return deduped[i].Score > deduped[j].Score
	})

	// Limit
	if len(deduped) > limit {
		deduped = deduped[:limit]
	}

	// Build relations and convert to SymbolEntries
	for _, c := range deduped {
		relations := buildRelations(store, c.Node)
		cm.Symbols = append(cm.Symbols, SymbolEntry{
			Name:      c.Node.Name,
			Kind:      c.Node.Type,
			Path:      c.Node.Path,
			Line:      c.Node.Line,
			Score:     c.Score,
			Relations: relations,
		})
	}

	return cm, nil
}

// buildRelations produces human-readable relation strings for a node.
func buildRelations(store *graph.Store, node graph.Node) []string {
	var relations []string

	outEdges, _ := store.EdgesFrom(node.ID)
	for _, e := range outEdges {
		if e.Type == "CALLS" {
			if target, _ := store.NodeByID(e.To); target != nil {
				relations = append(relations, "calls "+target.Name)
			}
		}
	}

	inEdges, _ := store.EdgesTo(node.ID)
	for _, e := range inEdges {
		if e.Type == "CALLS" {
			if source, _ := store.NodeByID(e.From); source != nil {
				relations = append(relations, "called by "+source.Name)
			}
		}
	}

	return relations
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/graph/context/ -run TestBuildContextMap -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/graph/context/engine.go internal/graph/context/context_test.go
git commit -m "feat(context): add BuildContextMap orchestrator with deduplication"
```

---

### Task 6: Config fields for context map

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go` (if exists)

**Step 1: Write the failing test**

Add to `internal/config/config_test.go` (or create it):

```go
func TestDefaults_ContextMap(t *testing.T) {
	cfg := Defaults()
	if !cfg.ContextMap {
		t.Error("expected ContextMap to default to true")
	}
	if cfg.ContextMapLimit != 15 {
		t.Errorf("expected ContextMapLimit to default to 15, got %d", cfg.ContextMapLimit)
	}
}

func TestGetValue_ContextMap(t *testing.T) {
	cfg := Defaults()
	v, err := GetValue(cfg, "context-map")
	if err != nil {
		t.Fatal(err)
	}
	if v != "true" {
		t.Errorf("expected 'true', got %q", v)
	}

	v, err = GetValue(cfg, "context-map-limit")
	if err != nil {
		t.Fatal(err)
	}
	if v != "15" {
		t.Errorf("expected '15', got %q", v)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run "TestDefaults_ContextMap|TestGetValue_ContextMap" -v`
Expected: FAIL — fields don't exist.

**Step 3: Write minimal implementation**

Add to `Config` struct in `internal/config/config.go`:

```go
ContextMap      bool `yaml:"context-map" json:"context-map"`
ContextMapLimit int  `yaml:"context-map-limit" json:"context-map-limit"`
```

Update `Defaults()`:

```go
ContextMap:      true,
ContextMapLimit: 15,
```

Add to `configLayer`:

```go
ContextMap      *bool `yaml:"context-map"`
ContextMapLimit *int  `yaml:"context-map-limit"`
```

Add to `merge()`:

```go
if layer.ContextMap != nil {
    base.ContextMap = *layer.ContextMap
}
if layer.ContextMapLimit != nil {
    base.ContextMapLimit = *layer.ContextMapLimit
}
```

Add to `GetValue()`:

```go
case "context-map":
    return strconv.FormatBool(cfg.ContextMap), nil
case "context-map-limit":
    return strconv.Itoa(cfg.ContextMapLimit), nil
```

Add to `PrintConfig()`:

```go
fmt.Fprintf(w, "context-map: %v\n", cfg.ContextMap)
fmt.Fprintf(w, "context-map-limit: %d\n", cfg.ContextMapLimit)
```

Add to `Keys()`:

```go
{"context-map", "enable context map injection (true/false)"},
{"context-map-limit", "max symbols in context map (default 15)"},
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -run "TestDefaults_ContextMap|TestGetValue_ContextMap" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): add context-map and context-map-limit settings"
```

---

### Task 7: Prompt template integration

**Files:**
- Modify: `internal/runner/prompt.go`
- Modify: `templates/prompt.md`
- Modify: `internal/runner/prompt_test.go` (if exists, otherwise `internal/runner/builder_test.go`)

**Step 1: Write the failing test**

Add to `internal/runner/prompt_test.go` (or the appropriate test file):

```go
func TestRenderPrompt_ContextMap(t *testing.T) {
	dir := t.TempDir()
	ctxDir := filepath.Join(dir, ".ctx")
	os.MkdirAll(ctxDir, 0755)

	tmpl := "Before\n{{CONTEXT_MAP}}\nAfter"
	os.WriteFile(filepath.Join(ctxDir, "prompt.md"), []byte(tmpl), 0644)

	vars := PromptVars{
		ContextMap: "## Relevant Context\n\n- `Foo` function (a.go:10)\n",
	}

	got, err := RenderPrompt(dir, "prompt.md", vars)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "## Relevant Context") {
		t.Error("context map not injected")
	}
	if !strings.Contains(got, "Before") || !strings.Contains(got, "After") {
		t.Error("template content lost")
	}
}

func TestRenderPrompt_EmptyContextMap(t *testing.T) {
	dir := t.TempDir()
	ctxDir := filepath.Join(dir, ".ctx")
	os.MkdirAll(ctxDir, 0755)

	tmpl := "Before\n{{CONTEXT_MAP}}\nAfter"
	os.WriteFile(filepath.Join(ctxDir, "prompt.md"), []byte(tmpl), 0644)

	vars := PromptVars{ContextMap: ""}
	got, err := RenderPrompt(dir, "prompt.md", vars)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "Relevant Context") {
		t.Error("empty context map should not produce output")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/runner/ -run "TestRenderPrompt_ContextMap" -v`
Expected: FAIL — `ContextMap` field not in `PromptVars`.

**Step 3: Write minimal implementation**

Add `ContextMap string` field to `PromptVars` in `internal/runner/prompt.go`:

```go
type PromptVars struct {
	DocsPath         string
	IterationContext string
	TaskOverride     string
	ReviewContext    string
	InjectedContext  string
	ContextMap       string
}
```

Add replacement in `RenderPrompt`:

```go
content = strings.ReplaceAll(content, "{{CONTEXT_MAP}}", vars.ContextMap)
```

Add `{{CONTEXT_MAP}}` to `templates/prompt.md` after `{{INJECTED_CONTEXT}}`:

```
{{INJECTED_CONTEXT}}

{{CONTEXT_MAP}}

## Start of Session
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/runner/ -run "TestRenderPrompt_ContextMap" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/runner/prompt.go templates/prompt.md
git commit -m "feat(runner): add CONTEXT_MAP placeholder to prompt rendering"
```

---

### Task 8: Builder loop integration

**Files:**
- Modify: `internal/runner/builder.go`
- Modify: `cmd/code.go`
- Modify: `cmd/helpers.go`

**Step 1: Write the failing test**

This is an integration point — the builder loop needs to call `BuildContextMap` and pass it to `PromptVars`. Since the builder loop is complex, test this by verifying the wiring compiles and the config flows through.

Add to `internal/runner/builder_test.go` (or create test file):

```go
func TestBuilderConfig_ContextMapFields(t *testing.T) {
	cfg := BuilderConfig{
		ContextMap:      true,
		ContextMapLimit: 15,
	}
	if !cfg.ContextMap {
		t.Error("expected ContextMap true")
	}
	if cfg.ContextMapLimit != 15 {
		t.Errorf("expected ContextMapLimit 15, got %d", cfg.ContextMapLimit)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/runner/ -run TestBuilderConfig_ContextMapFields -v`
Expected: FAIL — fields not in `BuilderConfig`.

**Step 3: Write minimal implementation**

Add to `BuilderConfig` in `internal/runner/builder.go`:

```go
ContextMap      bool // enable context map injection
ContextMapLimit int  // max symbols (default 15)
```

In the iteration loop of `RunBuilderLoop`, after graph sync (line ~100) and before prompt rendering (line ~180), add context map generation. Find the section where `PromptVars` is constructed and add:

```go
// Build context map if enabled and embeddings exist
var contextMapStr string
if cfg.ContextMap {
	graphPath := filepath.Join(cfg.Dir, ".ctx", "graph.db")
	if _, statErr := os.Stat(graphPath); statErr == nil {
		if gStore, gErr := graph.OpenStore(graphPath); gErr == nil {
			if eCount, _ := gStore.EmbeddingCount(); eCount > 0 {
				modelDir, mErr := embed.EnsureModel(embed.DefaultModelID, embed.DefaultModelDir())
				if mErr == nil {
					if embedder, oErr := embed.NewONNXEmbedder(modelDir); oErr == nil {
						// Determine task text
						taskText := cfg.TaskOverride
						if taskText == "" {
							for _, task := range state.Tasks {
								if task.Status == "in-progress" || task.Status == "todo" {
									taskText = task.Name
									if task.Notes != "" {
										taskText += " " + task.Notes
									}
									break
								}
							}
						}
						if taskText != "" {
							limit := cfg.ContextMapLimit
							if limit < 1 {
								limit = 15
							}
							cm, cmErr := graphctx.BuildContextMap(gStore, embedder, taskText, limit)
							if cmErr == nil {
								contextMapStr = cm.Format()
							} else {
								fmt.Fprintf(os.Stderr, "golem: warning: context map failed: %v\n", cmErr)
							}
						}
						embedder.Close()
					}
				}
			}
			gStore.Close()
		}
	}
}
```

Add `contextMapStr` to the `PromptVars`:

```go
prompt, err := RenderPrompt(cfg.Dir, templateFile, PromptVars{
    DocsPath:         state.Project.DocsPath,
    IterationContext: iterCtx,
    TaskOverride:     taskOverride,
    ReviewContext:    reviewCtx,
    InjectedContext:  injectedContext,
    ContextMap:       contextMapStr,
})
```

Add import at the top of `builder.go`:

```go
graphctx "github.com/lofari/golem/internal/graph/context"
```

Update `cmd/code.go` to pass the new fields:

```go
result, err := runner.RunBuilderLoop(ctx, runner.BuilderConfig{
    // ... existing fields ...
    ContextMap:       rc.ContextMap,
    ContextMapLimit:  rc.ContextMapLimit,
})
```

Update `cmd/helpers.go` to add the `--no-context-map` flag:

In `addAgentFlags`:
```go
cmd.Flags().Bool("no-context-map", false, "disable context map injection")
```

In `resolveConfig`:
```go
if cmd.Flags().Changed("no-context-map") {
    noCtx, _ := cmd.Flags().GetBool("no-context-map")
    if noCtx {
        cfg.ContextMap = false
    }
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/runner/ -run TestBuilderConfig_ContextMapFields -v`
Expected: PASS

Then run the full build: `go build ./...`
Expected: compiles without errors.

**Step 5: Commit**

```bash
git add internal/runner/builder.go cmd/code.go cmd/helpers.go
git commit -m "feat(runner): integrate context engine into builder loop"
```

---

### Task 9: Full pipeline integration test

**Files:**
- Modify: `internal/graph/context/context_test.go`

**Step 1: Write the integration test**

```go
func TestBuildContextMap_FullPipeline(t *testing.T) {
	store := setupTestStore(t)
	embedder := &fakeEmbedder{}

	// Set up a realistic graph: functions, calls, file nodes, co-changes, commits
	nodes := []graph.Node{
		{ID: "file:auth/login.go", Type: "file", Name: "login.go", Path: "auth/login.go", Line: 0},
		{ID: "func:auth/login.go:ValidateCredentials", Type: "function", Name: "ValidateCredentials", Path: "auth/login.go", Line: 45},
		{ID: "func:auth/crypto.go:CheckPassword", Type: "function", Name: "CheckPassword", Path: "auth/crypto.go", Line: 30},
		{ID: "file:auth/crypto.go", Type: "file", Name: "crypto.go", Path: "auth/crypto.go", Line: 0},
		{ID: "func:util/logger.go:LogInfo", Type: "function", Name: "LogInfo", Path: "util/logger.go", Line: 10},
	}
	edges := []graph.Edge{
		{From: "func:auth/login.go:ValidateCredentials", To: "func:auth/crypto.go:CheckPassword", Type: "CALLS"},
	}
	store.InsertBatch(nodes, edges)

	// Add embeddings
	for _, n := range nodes {
		store.InsertEmbedding(n.ID, embedder.vectorFor(n.Name+" "+n.Path))
	}

	// Add co-change
	store.InsertEdgeWithWeight("file:auth/login.go", "file:auth/crypto.go", "CO_CHANGED", 5)

	// Add recent commit
	store.InsertCommitBatch([]graph.Commit{
		{SHA: "abc123", Message: "fix login", AuthorEmail: "dev@test.com", Timestamp: 9999999999, Additions: 10, Deletions: 5},
	})
	store.InsertBatch(nil, []graph.Edge{
		{From: "commit:abc123", To: "file:auth/login.go", Type: "MODIFIES"},
	})

	cm, err := BuildContextMap(store, embedder, "fix login validation", 15)
	if err != nil {
		t.Fatal(err)
	}

	if len(cm.Symbols) == 0 {
		t.Fatal("expected symbols in context map")
	}

	// Verify format produces valid output
	formatted := cm.Format()
	if formatted == "" {
		t.Error("expected non-empty formatted output")
	}
	if !strings.Contains(formatted, "## Relevant Context") {
		t.Error("missing header in formatted output")
	}

	t.Logf("Context map for 'fix login validation':\n%s", formatted)
}
```

**Step 2: Run the full test suite**

Run: `go test ./internal/graph/context/ -v`
Expected: ALL PASS

**Step 3: Run the full project test suite**

Run: `go test ./...`
Expected: ALL PASS

**Step 4: Commit**

```bash
git add internal/graph/context/context_test.go
git commit -m "test(context): add full pipeline integration test"
```

---

### Task 10: Update embedded template

**Files:**
- Modify: `templates/embed.go` (or wherever templates are embedded)
- Modify: `internal/scaffold/scaffold.go` (to copy updated template on `golem init`)

**Step 1: Verify the template update**

The `templates/prompt.md` was already updated in Task 7. Now ensure the embedded copy and scaffold code pick it up.

Run: `go build ./...`
Expected: compiles — embedded templates auto-update.

**Step 2: Test scaffold**

Run: `go test ./internal/scaffold/ -v`
Expected: PASS — existing scaffold tests still work with the new template.

**Step 3: Run full suite**

Run: `go test ./...`
Expected: ALL PASS

**Step 4: Commit**

If any changes were needed:
```bash
git add templates/ internal/scaffold/
git commit -m "chore: update embedded templates with CONTEXT_MAP placeholder"
```
