# UI Improvements & Knowledge Graph Integration — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Transform the Flutter desktop UI into a polished daily-driver for agentic coding, with knowledge graph integration, cumulative diffs, and visual polish.

**Architecture:** Server-side additions (3 new Go endpoints: graph stats, diff, context map) feed into new Flutter models, providers, and views. The Flutter app gets a project switcher, enriched dashboard with graph summary + diff card, process view with tabbed sidebar (Tasks/Context/Diff), and a full-screen graph explorer overlay. Visual polish applied throughout.

**Tech Stack:** Go (server), Flutter/Dart (UI), Riverpod (state), xterm (terminal), SQLite (graph)

**Design doc:** `docs/plans/2026-03-08-ui-improvements-design.md`

---

### Task 1: Add graph stats API endpoint

**Files:**
- Modify: `internal/server/graph.go`
- Modify: `internal/server/server.go:64` (add route)
- Create: `internal/server/graph_test.go`

**Step 1: Write the test**

```go
// internal/server/graph_test.go
package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/lofari/golem/internal/graph"
)

func TestHandleGraphStats(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ctx"), 0755)

	// Create a graph with some nodes
	dbPath := filepath.Join(dir, ".ctx", "graph.db")
	store, err := graph.OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	store.UpsertNode(graph.Node{ID: "fn:main.go:main", Type: "function", Name: "main", Path: "main.go", Line: 1})
	store.UpsertNode(graph.Node{ID: "fn:main.go:hello", Type: "function", Name: "hello", Path: "main.go", Line: 10})
	store.UpsertEdge(graph.Edge{From: "fn:main.go:main", To: "fn:main.go:hello", Type: "CALLS"})
	store.Close()

	srv := New(Config{})
	srv.RegisterProject(dir)

	// Find the project ID
	var projID string
	for id := range srv.projects {
		projID = id
	}

	req := httptest.NewRequest("GET", "/api/projects/"+projID+"/graph/stats", nil)
	req.SetPathValue("id", projID)
	w := httptest.NewRecorder()

	srv.handleGraphStats(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]interface{}
	json.NewDecoder(w.Body).Decode(&result)

	if result["totalNodes"].(float64) != 2 {
		t.Errorf("expected 2 nodes, got %v", result["totalNodes"])
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run TestHandleGraphStats -v`
Expected: FAIL — `handleGraphStats` not defined

**Step 3: Implement the handler**

Add to `internal/server/graph.go`:

```go
// GraphStatsResponse is the JSON shape for GET /graph/stats.
type GraphStatsResponse struct {
	TotalNodes     int            `json:"totalNodes"`
	TotalEdges     int            `json:"totalEdges"`
	NodeTypes      map[string]int `json:"nodeTypes"`
	EdgeTypes      map[string]int `json:"edgeTypes"`
	EmbeddingCount int            `json:"embeddingCount"`
	EmbedModel     string         `json:"embedModel,omitempty"`
	LastIndexed    string         `json:"lastIndexed,omitempty"`
	CommitCount    int            `json:"commitCount"`
	AuthorCount    int            `json:"authorCount"`
	CoChangeCount  int            `json:"coChangeCount"`
	ExecutionCount int            `json:"executionCount"`
}

func (s *Server) handleGraphStats(w http.ResponseWriter, r *http.Request) {
	p, ok := s.getProject(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	store, err := s.openProjectGraph(p)
	if err != nil {
		writeError(w, http.StatusNotFound, "graph not found — run golem graph build")
		return
	}
	defer store.Close()

	stats, err := store.Stats()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := GraphStatsResponse{
		TotalNodes: stats.TotalNodes,
		TotalEdges: stats.TotalEdges,
		NodeTypes:  stats.NodeTypes,
		EdgeTypes:  stats.EdgeTypes,
	}
	resp.EmbeddingCount, _ = store.EmbeddingCount()
	resp.EmbedModel, _ = store.GetMeta("embed_model")
	resp.LastIndexed, _ = store.GetMeta("last_indexed")
	resp.CommitCount, _ = store.CommitCount()
	resp.AuthorCount, _ = store.AuthorCount()
	resp.CoChangeCount, _ = store.CoChangedCount()
	resp.ExecutionCount, _ = store.ExecutionCount()

	writeJSON(w, http.StatusOK, resp)
}
```

Add route in `internal/server/server.go` after the existing graph routes (line 66):

```go
s.mux.HandleFunc("GET /api/projects/{id}/graph/stats", s.handleGraphStats)
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/server/ -run TestHandleGraphStats -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/server/graph.go internal/server/graph_test.go internal/server/server.go
git commit -m "feat(server): add graph stats API endpoint"
```

---

### Task 2: Add diff API endpoint

**Files:**
- Modify: `internal/server/server.go` (add route)
- Modify: `internal/server/projects.go` (add handler)
- Modify: `internal/git/git.go` (add DiffSummary function)
- Create: `internal/git/git_test.go`

**Step 1: Write the git diff helper test**

```go
// internal/git/git_test.go
package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDiffSummary(t *testing.T) {
	dir := t.TempDir()

	// Init git repo
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %s", args, out)
		}
	}

	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "test")
	os.WriteFile(filepath.Join(dir, "hello.go"), []byte("package main\n\nfunc main() {}\n"), 0644)
	run("add", ".")
	run("commit", "-m", "initial")

	// Get the base ref
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	baseOut, _ := cmd.Output()
	baseRef := string(baseOut[:len(baseOut)-1])

	// Make a change
	os.WriteFile(filepath.Join(dir, "hello.go"), []byte("package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n"), 0644)
	run("add", ".")
	run("commit", "-m", "add hello")

	summary, err := DiffSummary(dir, baseRef)
	if err != nil {
		t.Fatal(err)
	}

	if len(summary.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(summary.Files))
	}
	if summary.Files[0].Path != "hello.go" {
		t.Errorf("expected hello.go, got %s", summary.Files[0].Path)
	}
	if summary.TotalAdded == 0 {
		t.Error("expected additions > 0")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/git/ -run TestDiffSummary -v`
Expected: FAIL — `DiffSummary` not defined

**Step 3: Implement DiffSummary in git.go**

Add to `internal/git/git.go`:

```go
// FileDiff represents a single file's diff stats.
type FileDiff struct {
	Path      string `json:"path"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Patch     string `json:"patch,omitempty"`
}

// DiffSummaryResult holds the cumulative diff.
type DiffSummaryResult struct {
	BaseRef      string     `json:"baseRef"`
	Files        []FileDiff `json:"files"`
	TotalAdded   int        `json:"totalAdded"`
	TotalRemoved int        `json:"totalRemoved"`
}

// DiffSummary returns a cumulative diff from baseRef to HEAD.
// If baseRef is empty, diffs against HEAD~1.
func DiffSummary(dir string, baseRef string) (*DiffSummaryResult, error) {
	if baseRef == "" {
		baseRef = "HEAD~1"
	}

	// Get numstat for file-level stats
	cmd := exec.Command("git", "diff", "--numstat", baseRef+"..HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return &DiffSummaryResult{BaseRef: baseRef}, nil
	}

	result := &DiffSummaryResult{BaseRef: baseRef}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		added := 0
		deleted := 0
		// Binary files show "-" for stats
		if parts[0] != "-" {
			fmt.Sscanf(parts[0], "%d", &added)
		}
		if parts[1] != "-" {
			fmt.Sscanf(parts[1], "%d", &deleted)
		}
		result.Files = append(result.Files, FileDiff{
			Path:      parts[2],
			Additions: added,
			Deletions: deleted,
		})
		result.TotalAdded += added
		result.TotalRemoved += deleted
	}

	return result, nil
}

// DiffPatch returns the unified diff for a specific file from baseRef to HEAD.
func DiffPatch(dir string, baseRef string, filePath string) (string, error) {
	if baseRef == "" {
		baseRef = "HEAD~1"
	}
	cmd := exec.Command("git", "diff", baseRef+"..HEAD", "--", filePath)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/git/ -run TestDiffSummary -v`
Expected: PASS

**Step 5: Add the server handler and route**

Add to `internal/server/projects.go`:

```go
import (
	"github.com/lofari/golem/internal/git"
)

func (s *Server) handleDiff(w http.ResponseWriter, r *http.Request) {
	p, ok := s.getProject(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	baseRef := r.URL.Query().Get("ref")
	file := r.URL.Query().Get("file")

	// If a specific file is requested, return its patch
	if file != "" {
		patch, err := git.DiffPatch(p.path, baseRef, file)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"patch": patch})
		return
	}

	summary, err := git.DiffSummary(p.path, baseRef)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, summary)
}
```

Add route in `internal/server/server.go` after the graph routes:

```go
s.mux.HandleFunc("GET /api/projects/{id}/diff", s.handleDiff)
```

**Step 6: Run all tests**

Run: `go test ./internal/... -v`
Expected: PASS

**Step 7: Commit**

```bash
git add internal/git/git.go internal/git/git_test.go internal/server/projects.go internal/server/server.go
git commit -m "feat(server): add diff API endpoint with git summary and file patches"
```

---

### Task 3: Add context map API endpoint

**Files:**
- Modify: `internal/server/graph.go`
- Modify: `internal/server/server.go` (add route)

**Step 1: Implement the handler**

Add to `internal/server/graph.go`:

```go
import (
	graphctx "github.com/lofari/golem/internal/graph/context"
)

func (s *Server) handleContextMap(w http.ResponseWriter, r *http.Request) {
	p, ok := s.getProject(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	task := r.URL.Query().Get("task")
	if task == "" {
		writeError(w, http.StatusBadRequest, "task parameter is required")
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 15
	if limitStr != "" {
		if v, err := strconv.Atoi(limitStr); err == nil {
			limit = v
		}
	}

	store, err := s.openProjectGraph(p)
	if err != nil {
		writeError(w, http.StatusNotFound, "graph not found — run golem graph build")
		return
	}
	defer store.Close()

	modelDir, err := embed.EnsureModel(embed.DefaultModelID, embed.DefaultModelDir())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading embedding model: "+err.Error())
		return
	}
	embedder, err := embed.NewONNXEmbedder(modelDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "creating embedder: "+err.Error())
		return
	}
	defer embedder.Close()

	cm, err := graphctx.BuildContextMap(store, embedder, task, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, cm)
}
```

Add route in `internal/server/server.go`:

```go
s.mux.HandleFunc("GET /api/projects/{id}/graph/context-map", s.handleContextMap)
```

**Step 2: Run all server tests**

Run: `go test ./internal/server/ -v`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/server/graph.go internal/server/server.go
git commit -m "feat(server): add context map API endpoint"
```

---

### Task 4: Flutter models for graph stats, diff, and context map

**Files:**
- Create: `ui/flutter/lib/models/graph.dart`

**Step 1: Create the models file**

```dart
// ui/flutter/lib/models/graph.dart

class GraphStats {
  final int totalNodes;
  final int totalEdges;
  final Map<String, int> nodeTypes;
  final Map<String, int> edgeTypes;
  final int embeddingCount;
  final String embedModel;
  final String lastIndexed;
  final int commitCount;
  final int authorCount;
  final int coChangeCount;
  final int executionCount;

  GraphStats({
    required this.totalNodes,
    required this.totalEdges,
    required this.nodeTypes,
    required this.edgeTypes,
    required this.embeddingCount,
    this.embedModel = '',
    this.lastIndexed = '',
    this.commitCount = 0,
    this.authorCount = 0,
    this.coChangeCount = 0,
    this.executionCount = 0,
  });

  factory GraphStats.fromJson(Map<String, dynamic> json) => GraphStats(
        totalNodes: json['totalNodes'] as int? ?? 0,
        totalEdges: json['totalEdges'] as int? ?? 0,
        nodeTypes: (json['nodeTypes'] as Map<String, dynamic>?)
                ?.map((k, v) => MapEntry(k, v as int)) ??
            {},
        edgeTypes: (json['edgeTypes'] as Map<String, dynamic>?)
                ?.map((k, v) => MapEntry(k, v as int)) ??
            {},
        embeddingCount: json['embeddingCount'] as int? ?? 0,
        embedModel: json['embedModel'] as String? ?? '',
        lastIndexed: json['lastIndexed'] as String? ?? '',
        commitCount: json['commitCount'] as int? ?? 0,
        authorCount: json['authorCount'] as int? ?? 0,
        coChangeCount: json['coChangeCount'] as int? ?? 0,
        executionCount: json['executionCount'] as int? ?? 0,
      );
}

class DiffSummary {
  final String baseRef;
  final List<FileDiff> files;
  final int totalAdded;
  final int totalRemoved;

  DiffSummary({
    this.baseRef = '',
    required this.files,
    this.totalAdded = 0,
    this.totalRemoved = 0,
  });

  factory DiffSummary.fromJson(Map<String, dynamic> json) => DiffSummary(
        baseRef: json['baseRef'] as String? ?? '',
        files: (json['files'] as List<dynamic>?)
                ?.map((e) => FileDiff.fromJson(e as Map<String, dynamic>))
                .toList() ??
            [],
        totalAdded: json['totalAdded'] as int? ?? 0,
        totalRemoved: json['totalRemoved'] as int? ?? 0,
      );
}

class FileDiff {
  final String path;
  final int additions;
  final int deletions;
  String? patch; // loaded on demand

  FileDiff({
    required this.path,
    this.additions = 0,
    this.deletions = 0,
    this.patch,
  });

  factory FileDiff.fromJson(Map<String, dynamic> json) => FileDiff(
        path: json['path'] as String? ?? '',
        additions: json['additions'] as int? ?? 0,
        deletions: json['deletions'] as int? ?? 0,
      );
}

class ContextMapResult {
  final String task;
  final List<ContextSymbol> symbols;

  ContextMapResult({required this.task, required this.symbols});

  factory ContextMapResult.fromJson(Map<String, dynamic> json) =>
      ContextMapResult(
        task: json['Task'] as String? ?? '',
        symbols: (json['Symbols'] as List<dynamic>?)
                ?.map((e) => ContextSymbol.fromJson(e as Map<String, dynamic>))
                .toList() ??
            [],
      );
}

class ContextSymbol {
  final String name;
  final String kind;
  final String path;
  final int line;
  final double score;
  final List<String> relations;

  ContextSymbol({
    required this.name,
    required this.kind,
    required this.path,
    this.line = 0,
    this.score = 0,
    this.relations = const [],
  });

  factory ContextSymbol.fromJson(Map<String, dynamic> json) => ContextSymbol(
        name: json['Name'] as String? ?? '',
        kind: json['Kind'] as String? ?? '',
        path: json['Path'] as String? ?? '',
        line: json['Line'] as int? ?? 0,
        score: (json['Score'] as num?)?.toDouble() ?? 0,
        relations:
            (json['Relations'] as List<dynamic>?)?.cast<String>() ?? [],
      );
}

class GraphSearchResult {
  final String name;
  final String path;
  final int line;
  final String type;
  final double score;

  GraphSearchResult({
    required this.name,
    required this.path,
    this.line = 0,
    required this.type,
    this.score = 0,
  });

  factory GraphSearchResult.fromJson(Map<String, dynamic> json) =>
      GraphSearchResult(
        name: json['name'] as String? ?? '',
        path: json['path'] as String? ?? '',
        line: json['line'] as int? ?? 0,
        type: json['type'] as String? ?? '',
        score: (json['score'] as num?)?.toDouble() ?? 0,
      );
}

class GraphNode {
  final String id;
  final String type;
  final String name;
  final String path;
  final int line;

  GraphNode({
    required this.id,
    required this.type,
    required this.name,
    required this.path,
    this.line = 0,
  });

  factory GraphNode.fromJson(Map<String, dynamic> json) => GraphNode(
        id: json['id'] as String? ?? '',
        type: json['type'] as String? ?? '',
        name: json['name'] as String? ?? '',
        path: json['path'] as String? ?? '',
        line: json['line'] as int? ?? 0,
      );
}

class GraphEdge {
  final String from;
  final String to;
  final String type;

  GraphEdge({required this.from, required this.to, required this.type});

  factory GraphEdge.fromJson(Map<String, dynamic> json) => GraphEdge(
        from: json['from'] as String? ?? '',
        to: json['to'] as String? ?? '',
        type: json['type'] as String? ?? '',
      );
}

class GraphRelatedResult {
  final List<GraphNode> nodes;
  final List<GraphEdge> edges;

  GraphRelatedResult({required this.nodes, required this.edges});

  factory GraphRelatedResult.fromJson(Map<String, dynamic> json) =>
      GraphRelatedResult(
        nodes: (json['nodes'] as List<dynamic>?)
                ?.map((e) => GraphNode.fromJson(e as Map<String, dynamic>))
                .toList() ??
            [],
        edges: (json['edges'] as List<dynamic>?)
                ?.map((e) => GraphEdge.fromJson(e as Map<String, dynamic>))
                .toList() ??
            [],
      );
}
```

**Step 2: Commit**

```bash
git add ui/flutter/lib/models/graph.dart
git commit -m "feat(ui): add models for graph stats, diff, context map, and search"
```

---

### Task 5: Flutter API client extensions

**Files:**
- Modify: `ui/flutter/lib/api/client.dart`

**Step 1: Add new API methods to GolemApiClient**

Add these methods after the existing config methods (after line 132):

```dart
  // Graph
  Future<Map<String, dynamic>> getGraphStats(String projectId) async {
    return _getJson('/api/projects/$projectId/graph/stats');
  }

  Future<List<dynamic>> graphSearch(
      String projectId, String query,
      {int limit = 10, List<String>? types}) async {
    final body = <String, dynamic>{'query': query, 'limit': limit};
    if (types != null && types.isNotEmpty) body['types'] = types;
    final resp = await _http.post(
      Uri.parse('$baseUrl/api/projects/$projectId/graph/search'),
      headers: {'Content-Type': 'application/json'},
      body: jsonEncode(body),
    );
    if (resp.statusCode >= 400) {
      final b = jsonDecode(resp.body) as Map<String, dynamic>;
      throw ApiException(b['error'] as String? ?? 'Search failed');
    }
    return jsonDecode(resp.body) as List<dynamic>;
  }

  Future<Map<String, dynamic>> graphRelated(
      String projectId, String name,
      {String direction = 'all', int depth = 1}) async {
    return _getJson(
        '/api/projects/$projectId/graph/related?name=${Uri.encodeComponent(name)}&direction=$direction&depth=$depth');
  }

  Future<Map<String, dynamic>> getContextMap(
      String projectId, String task,
      {int limit = 15}) async {
    return _getJson(
        '/api/projects/$projectId/graph/context-map?task=${Uri.encodeComponent(task)}&limit=$limit');
  }

  // Diff
  Future<Map<String, dynamic>> getDiff(String projectId, {String? ref}) async {
    final query = ref != null ? '?ref=${Uri.encodeComponent(ref)}' : '';
    return _getJson('/api/projects/$projectId/diff$query');
  }

  Future<String> getFilePatch(String projectId, String filePath,
      {String? ref}) async {
    final refQuery = ref != null ? '&ref=${Uri.encodeComponent(ref)}' : '';
    final json = await _getJson(
        '/api/projects/$projectId/diff?file=${Uri.encodeComponent(filePath)}$refQuery');
    return json['patch'] as String? ?? '';
  }
```

Add the import for `graph.dart` model at the top of client.dart:

```dart
import '../models/graph.dart';
```

**Step 2: Verify build**

Run: `cd ui/flutter && flutter analyze`
Expected: No errors

**Step 3: Commit**

```bash
git add ui/flutter/lib/api/client.dart
git commit -m "feat(ui): add graph stats, search, diff, and context map API methods"
```

---

### Task 6: Flutter providers for graph and diff data

**Files:**
- Create: `ui/flutter/lib/providers/graph.dart`

**Step 1: Create the providers file**

```dart
// ui/flutter/lib/providers/graph.dart
import 'dart:async';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../api/client.dart';
import '../models/graph.dart';
import 'connection.dart';
import 'project.dart';

// Graph stats — fetched on demand, refreshable
final graphStatsProvider =
    StateNotifierProvider<GraphStatsNotifier, GraphStats?>((ref) {
  final projectInfo = ref.watch(projectInfoProvider);
  final api = ref.read(apiClientProvider);
  return GraphStatsNotifier(api, projectInfo?.id);
});

class GraphStatsNotifier extends StateNotifier<GraphStats?> {
  final GolemApiClient _api;
  final String? _projectId;

  GraphStatsNotifier(this._api, this._projectId) : super(null) {
    if (_projectId != null) _fetch();
  }

  Future<void> _fetch() async {
    try {
      final json = await _api.getGraphStats(_projectId!);
      state = GraphStats.fromJson(json);
    } catch (_) {
      // Graph doesn't exist — leave null
    }
  }

  void refresh() => _fetch();
}

// Diff summary — fetched on demand
final diffProvider =
    StateNotifierProvider<DiffNotifier, DiffSummary?>((ref) {
  final projectInfo = ref.watch(projectInfoProvider);
  final api = ref.read(apiClientProvider);
  return DiffNotifier(api, projectInfo?.id);
});

class DiffNotifier extends StateNotifier<DiffSummary?> {
  final GolemApiClient _api;
  final String? _projectId;

  DiffNotifier(this._api, this._projectId) : super(null) {
    if (_projectId != null) _fetch();
  }

  Future<void> _fetch() async {
    try {
      final json = await _api.getDiff(_projectId!);
      state = DiffSummary.fromJson(json);
    } catch (_) {}
  }

  void refresh() => _fetch();

  Future<String> loadPatch(String filePath) async {
    if (_projectId == null) return '';
    return _api.getFilePatch(_projectId!, filePath);
  }
}

// Graph search results
final graphSearchProvider = StateNotifierProvider.family<
    GraphSearchNotifier, List<GraphSearchResult>, String>((ref, query) {
  final projectInfo = ref.read(projectInfoProvider);
  final api = ref.read(apiClientProvider);
  return GraphSearchNotifier(api, projectInfo?.id, query);
});

class GraphSearchNotifier extends StateNotifier<List<GraphSearchResult>> {
  final GolemApiClient _api;
  final String? _projectId;

  GraphSearchNotifier(this._api, this._projectId, String query) : super([]) {
    if (_projectId != null && query.isNotEmpty) _search(query);
  }

  Future<void> _search(String query, {List<String>? types}) async {
    try {
      final results = await _api.graphSearch(_projectId!, query, types: types);
      state = results
          .map((e) => GraphSearchResult.fromJson(e as Map<String, dynamic>))
          .toList();
    } catch (_) {}
  }
}

// Graph related (for explorer)
final graphRelatedProvider = FutureProvider.family<GraphRelatedResult, String>(
    (ref, name) async {
  final projectInfo = ref.read(projectInfoProvider);
  final api = ref.read(apiClientProvider);
  if (projectInfo == null) return GraphRelatedResult(nodes: [], edges: []);
  final json = await api.graphRelated(projectInfo.id, name);
  return GraphRelatedResult.fromJson(json);
});

// Project list for switcher
final projectListProvider =
    StateNotifierProvider<ProjectListNotifier, List<ProjectInfo>>((ref) {
  final api = ref.read(apiClientProvider);
  return ProjectListNotifier(api);
});

class ProjectListNotifier extends StateNotifier<List<ProjectInfo>> {
  final GolemApiClient _api;

  ProjectListNotifier(this._api) : super([]) {
    _fetch();
  }

  Future<void> _fetch() async {
    try {
      final projects = await _api.listProjects();
      state = projects;
    } catch (_) {}
  }

  void refresh() => _fetch();
}
```

Note: We need to import `ProjectInfo` from the existing models. Add to the imports in `providers/graph.dart`:

```dart
import '../models/project.dart';
```

**Step 2: Verify build**

Run: `cd ui/flutter && flutter analyze`
Expected: No errors

**Step 3: Commit**

```bash
git add ui/flutter/lib/providers/graph.dart
git commit -m "feat(ui): add providers for graph stats, diff, search, and project list"
```

---

### Task 7: Visual polish — theme refinements

**Files:**
- Modify: `ui/flutter/lib/theme.dart`

**Step 1: Add phase colors and polish helpers to GolemTheme**

Add after line 14 (after `static const red = ...`):

```dart
  static const purple = Color(0xFFbc8cff);

  /// Phase badge color mapping.
  static Color phaseColor(String phase) {
    return switch (phase) {
      'planning' => accent,
      'building' => green,
      'fixing' => yellow,
      'polishing' => purple,
      _ => textSecondary,
    };
  }

  /// Small monospace style for metadata.
  static TextStyle metaStyle({double fontSize = 11}) {
    return GoogleFonts.jetBrainsMono(
      fontSize: fontSize,
      color: textSecondary,
      height: 1.4,
    );
  }
```

**Step 2: Commit**

```bash
git add ui/flutter/lib/theme.dart
git commit -m "feat(ui): add phase colors and metadata text style to theme"
```

---

### Task 8: Project switcher widget

**Files:**
- Create: `ui/flutter/lib/views/project_switcher.dart`
- Modify: `ui/flutter/lib/views/shell.dart` (replace project name with switcher)

**Step 1: Create the project switcher**

```dart
// ui/flutter/lib/views/project_switcher.dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/project.dart';
import '../providers/graph.dart';
import '../providers/project.dart';
import '../theme.dart';

class ProjectSwitcher extends ConsumerWidget {
  const ProjectSwitcher({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final current = ref.watch(projectInfoProvider);
    final projects = ref.watch(projectListProvider);

    return PopupMenuButton<String>(
      offset: const Offset(0, 36),
      color: GolemTheme.bgElevated,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(8),
        side: const BorderSide(color: GolemTheme.border),
      ),
      onSelected: (id) {
        if (id == '__add__') {
          // TODO: show add project dialog
          return;
        }
        // Switch project by updating the projectInfo provider
        final project = projects.firstWhere((p) => p.id == id);
        ref.read(projectInfoProvider.notifier).set(project);
      },
      itemBuilder: (_) => [
        ...projects.map((p) => PopupMenuItem<String>(
              value: p.id,
              child: Row(
                children: [
                  if (p.id == current?.id)
                    const Icon(Icons.check, size: 14, color: GolemTheme.accent)
                  else
                    const SizedBox(width: 14),
                  const SizedBox(width: 8),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        Text(
                          p.name.isNotEmpty ? p.name : p.path.split('/').last,
                          style: const TextStyle(fontSize: 13),
                        ),
                        Text(
                          p.path,
                          style: GolemTheme.metaStyle(fontSize: 10),
                          overflow: TextOverflow.ellipsis,
                        ),
                      ],
                    ),
                  ),
                  const SizedBox(width: 8),
                  _PhaseBadge(phase: p.phase),
                ],
              ),
            )),
        const PopupMenuDivider(),
        const PopupMenuItem<String>(
          value: '__add__',
          child: Row(
            children: [
              SizedBox(width: 14),
              SizedBox(width: 8),
              Icon(Icons.add, size: 14, color: GolemTheme.textSecondary),
              SizedBox(width: 6),
              Text('Add project...', style: TextStyle(fontSize: 13, color: GolemTheme.textSecondary)),
            ],
          ),
        ),
      ],
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(
            current?.name ?? 'Golem',
            style: const TextStyle(
              fontSize: 15,
              fontWeight: FontWeight.w600,
              color: GolemTheme.textPrimary,
            ),
          ),
          const SizedBox(width: 4),
          const Icon(Icons.expand_more, size: 16, color: GolemTheme.textSecondary),
        ],
      ),
    );
  }
}

class _PhaseBadge extends StatelessWidget {
  final String phase;
  const _PhaseBadge({required this.phase});

  @override
  Widget build(BuildContext context) {
    if (phase.isEmpty) return const SizedBox.shrink();
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: GolemTheme.phaseColor(phase).withOpacity(0.15),
        borderRadius: BorderRadius.circular(4),
      ),
      child: Text(
        phase,
        style: TextStyle(
          fontSize: 10,
          color: GolemTheme.phaseColor(phase),
          fontWeight: FontWeight.w500,
        ),
      ),
    );
  }
}
```

**Step 2: Add `set` method to ProjectInfoNotifier**

In `ui/flutter/lib/providers/project.dart`, add to `ProjectInfoNotifier`:

```dart
  void set(ProjectInfo info) {
    state = info;
  }
```

**Step 3: Update shell.dart top bar**

Replace the project name `Text` widget and phase badge in `_TopBar.build()` (the section at lines 120-144) with the `ProjectSwitcher`. The `_TopBar` will receive an `onGraphExplorer` callback instead of `projectName` and `phase`.

In `shell.dart`, replace `_TopBar` constructor and build to use ProjectSwitcher widget in the left section, and add a graph icon button before Launch.

**Step 4: Verify build**

Run: `cd ui/flutter && flutter analyze`
Expected: No errors

**Step 5: Commit**

```bash
git add ui/flutter/lib/views/project_switcher.dart ui/flutter/lib/views/shell.dart ui/flutter/lib/providers/project.dart
git commit -m "feat(ui): add project switcher dropdown in top bar"
```

---

### Task 9: Dashboard — graph summary card

**Files:**
- Modify: `ui/flutter/lib/views/dashboard.dart`

**Step 1: Refactor dashboard to use card grid layout**

Replace the current single-column layout in `DashboardView.build()` with a responsive layout. The dashboard should have:

1. Project header (same, tightened)
2. Three-card row: Tasks card (wider) | Graph summary card | Recent sessions card
3. Cumulative diff card (full width) — placeholder for Task 10
4. Decisions & Pitfalls row

The graph summary card reads from `graphStatsProvider` and shows:
- Node/edge count: "342 nodes · 891 edges"
- Embedding status with model name
- Last indexed timestamp
- "Explore graph" button
- Empty state with CLI command when no graph exists

Import the new providers:
```dart
import '../providers/graph.dart';
```

The tasks section should add filter chips (All / Active / Done / Blocked) and expandable task notes on click.

Session cards should show `filesChanged?.length` as a badge.

**Step 2: Verify build**

Run: `cd ui/flutter && flutter analyze`
Expected: No errors

**Step 3: Commit**

```bash
git add ui/flutter/lib/views/dashboard.dart
git commit -m "feat(ui): add graph summary card and grid layout to dashboard"
```

---

### Task 10: Diff viewer widget

**Files:**
- Create: `ui/flutter/lib/views/diff_viewer.dart`
- Modify: `ui/flutter/lib/views/dashboard.dart` (add diff card)

**Step 1: Create the diff viewer**

```dart
// ui/flutter/lib/views/diff_viewer.dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/graph.dart';
import '../providers/graph.dart';
import '../theme.dart';

/// A reusable diff viewer that shows file list and expandable patches.
/// Used in both the dashboard (full width) and process view sidebar (compact).
class DiffViewer extends ConsumerStatefulWidget {
  final bool compact; // true for sidebar use
  const DiffViewer({super.key, this.compact = false});

  @override
  ConsumerState<DiffViewer> createState() => _DiffViewerState();
}

class _DiffViewerState extends ConsumerState<DiffViewer> {
  String? _expandedFile;
  String? _loadedPatch;
  bool _loading = false;

  @override
  Widget build(BuildContext context) {
    final diff = ref.watch(diffProvider);

    if (diff == null || diff.files.isEmpty) {
      return Padding(
        padding: EdgeInsets.all(widget.compact ? 12 : 16),
        child: Text(
          'No changes since last run',
          style: TextStyle(
            fontSize: 12,
            color: GolemTheme.textSecondary,
          ),
        ),
      );
    }

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        // Header
        Padding(
          padding: EdgeInsets.all(widget.compact ? 8 : 16),
          child: Row(
            children: [
              Text(
                '${diff.files.length} file${diff.files.length != 1 ? "s" : ""} changed',
                style: const TextStyle(fontSize: 12, fontWeight: FontWeight.w600),
              ),
              const SizedBox(width: 8),
              Text(
                '+${diff.totalAdded}',
                style: const TextStyle(fontSize: 11, color: GolemTheme.green),
              ),
              const SizedBox(width: 4),
              Text(
                '\u2212${diff.totalRemoved}',
                style: const TextStyle(fontSize: 11, color: GolemTheme.red),
              ),
            ],
          ),
        ),
        // File list
        ...diff.files.map((f) => _FileRow(
              file: f,
              expanded: _expandedFile == f.path,
              patch: _expandedFile == f.path ? _loadedPatch : null,
              loading: _expandedFile == f.path && _loading,
              onTap: () => _toggleFile(f.path),
              compact: widget.compact,
            )),
      ],
    );
  }

  Future<void> _toggleFile(String path) async {
    if (_expandedFile == path) {
      setState(() {
        _expandedFile = null;
        _loadedPatch = null;
      });
      return;
    }

    setState(() {
      _expandedFile = path;
      _loadedPatch = null;
      _loading = true;
    });

    final patch = await ref.read(diffProvider.notifier).loadPatch(path);
    if (mounted && _expandedFile == path) {
      setState(() {
        _loadedPatch = patch;
        _loading = false;
      });
    }
  }
}

class _FileRow extends StatelessWidget {
  final FileDiff file;
  final bool expanded;
  final String? patch;
  final bool loading;
  final VoidCallback onTap;
  final bool compact;

  const _FileRow({
    required this.file,
    required this.expanded,
    required this.patch,
    required this.loading,
    required this.onTap,
    required this.compact,
  });

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        InkWell(
          onTap: onTap,
          child: Padding(
            padding: EdgeInsets.symmetric(
              horizontal: compact ? 8 : 16,
              vertical: 4,
            ),
            child: Row(
              children: [
                Icon(
                  expanded ? Icons.expand_more : Icons.chevron_right,
                  size: 14,
                  color: GolemTheme.textSecondary,
                ),
                const SizedBox(width: 4),
                Expanded(
                  child: Text(
                    file.path,
                    style: GolemTheme.monoStyle(fontSize: 12),
                    overflow: TextOverflow.ellipsis,
                  ),
                ),
                const SizedBox(width: 8),
                if (file.additions > 0)
                  Text('+${file.additions}',
                      style: const TextStyle(fontSize: 10, color: GolemTheme.green)),
                if (file.additions > 0 && file.deletions > 0)
                  const SizedBox(width: 4),
                if (file.deletions > 0)
                  Text('\u2212${file.deletions}',
                      style: const TextStyle(fontSize: 10, color: GolemTheme.red)),
              ],
            ),
          ),
        ),
        if (expanded && loading)
          const Padding(
            padding: EdgeInsets.all(16),
            child: SizedBox(
              width: 16,
              height: 16,
              child: CircularProgressIndicator(strokeWidth: 2, color: GolemTheme.accent),
            ),
          ),
        if (expanded && patch != null) _PatchView(patch: patch!, compact: compact),
      ],
    );
  }
}

class _PatchView extends StatelessWidget {
  final String patch;
  final bool compact;

  const _PatchView({required this.patch, required this.compact});

  @override
  Widget build(BuildContext context) {
    if (patch.isEmpty) {
      return Padding(
        padding: EdgeInsets.symmetric(horizontal: compact ? 8 : 16, vertical: 4),
        child: const Text('Binary file or no diff available',
            style: TextStyle(fontSize: 11, color: GolemTheme.textSecondary)),
      );
    }

    final lines = patch.split('\n');
    return Container(
      margin: EdgeInsets.symmetric(horizontal: compact ? 4 : 16, vertical: 4),
      decoration: BoxDecoration(
        color: GolemTheme.bgPrimary,
        borderRadius: BorderRadius.circular(6),
        border: Border.all(color: GolemTheme.border),
      ),
      child: SingleChildScrollView(
        scrollDirection: Axis.horizontal,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: lines.map((line) {
            Color bg = Colors.transparent;
            Color fg = GolemTheme.textPrimary;
            if (line.startsWith('+') && !line.startsWith('+++')) {
              bg = GolemTheme.green.withOpacity(0.1);
              fg = GolemTheme.green;
            } else if (line.startsWith('-') && !line.startsWith('---')) {
              bg = GolemTheme.red.withOpacity(0.1);
              fg = GolemTheme.red;
            } else if (line.startsWith('@@')) {
              fg = GolemTheme.accent;
            }
            return Container(
              width: 2000, // wide enough for horizontal scroll
              color: bg,
              padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 1),
              child: Text(
                line,
                style: GolemTheme.monoStyle(fontSize: 11).copyWith(color: fg, height: 1.3),
              ),
            );
          }).toList(),
        ),
      ),
    );
  }
}
```

**Step 2: Add diff card to dashboard**

In `dashboard.dart`, add a diff card section after the three-card row, using the `DiffViewer` widget:

```dart
// After the three-card row
const SizedBox(height: 16),
_SectionHeader('Changes'),
const SizedBox(height: 8),
Card(child: DiffViewer()),
```

**Step 3: Verify build**

Run: `cd ui/flutter && flutter analyze`
Expected: No errors

**Step 4: Commit**

```bash
git add ui/flutter/lib/views/diff_viewer.dart ui/flutter/lib/views/dashboard.dart
git commit -m "feat(ui): add diff viewer with syntax-highlighted patches"
```

---

### Task 11: Process view — tabbed sidebar with Context and Diff tabs

**Files:**
- Modify: `ui/flutter/lib/views/process_view.dart`

**Step 1: Refactor the right sidebar**

Replace `_TaskPanel` with a new `_SidePanel` that has three tabs: Tasks, Context, Diff.

Key changes:
- Add a resizable sidebar (GestureDetector drag handle on the left edge, min 220px, max 450px)
- Tab bar at top: Tasks | Context | Diff
- Tasks tab: same as current `_TaskPanel` content
- Context tab: reads from `contextMapProvider` (or call API with current task), displays symbol list
- Diff tab: uses `DiffViewer(compact: true)`
- Terminal header bar showing command + elapsed time

The process view `ProcessView` widget gets a terminal header bar above the xterm widget showing the command name and elapsed time.

**Step 2: Verify build**

Run: `cd ui/flutter && flutter analyze`
Expected: No errors

**Step 3: Commit**

```bash
git add ui/flutter/lib/views/process_view.dart
git commit -m "feat(ui): add tabbed sidebar with Context and Diff tabs to process view"
```

---

### Task 12: Graph explorer overlay

**Files:**
- Create: `ui/flutter/lib/views/graph_explorer.dart`
- Modify: `ui/flutter/lib/views/shell.dart` (add graph icon to top bar)

**Step 1: Create the graph explorer**

The overlay has two panes:
- **Left (300px):** Search bar + type filter chips + results list
- **Right (expanded):** Symbol detail with relationships, co-changed files, commits

Search uses `graphSearch` API method. Selecting a result loads `graphRelated` for the symbol.

The overlay is shown via `showDialog` with a full-screen dialog.

```dart
// ui/flutter/lib/views/graph_explorer.dart
import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../api/client.dart';
import '../models/graph.dart';
import '../providers/connection.dart';
import '../providers/graph.dart';
import '../providers/project.dart';
import '../theme.dart';

class GraphExplorer extends ConsumerStatefulWidget {
  const GraphExplorer({super.key});

  @override
  ConsumerState<GraphExplorer> createState() => _GraphExplorerState();
}

class _GraphExplorerState extends ConsumerState<GraphExplorer> {
  final _searchController = TextEditingController();
  Timer? _debounce;
  List<GraphSearchResult> _results = [];
  GraphSearchResult? _selected;
  GraphRelatedResult? _related;
  String _typeFilter = 'all';
  bool _searching = false;
  bool _loadingRelated = false;

  final _typeFilters = ['all', 'function', 'type', 'file', 'document'];

  @override
  void dispose() {
    _searchController.dispose();
    _debounce?.cancel();
    super.dispose();
  }

  void _onSearchChanged(String query) {
    _debounce?.cancel();
    _debounce = Timer(const Duration(milliseconds: 300), () {
      if (query.length >= 2) _search(query);
    });
  }

  Future<void> _search(String query) async {
    final projectInfo = ref.read(projectInfoProvider);
    if (projectInfo == null) return;

    setState(() => _searching = true);
    try {
      final api = ref.read(apiClientProvider);
      final types = _typeFilter == 'all' ? null : [_typeFilter];
      final raw = await api.graphSearch(projectInfo.id, query, types: types);
      setState(() {
        _results = raw
            .map((e) => GraphSearchResult.fromJson(e as Map<String, dynamic>))
            .toList();
        _searching = false;
      });
    } catch (e) {
      setState(() => _searching = false);
    }
  }

  Future<void> _selectResult(GraphSearchResult result) async {
    final projectInfo = ref.read(projectInfoProvider);
    if (projectInfo == null) return;

    setState(() {
      _selected = result;
      _loadingRelated = true;
    });

    try {
      final api = ref.read(apiClientProvider);
      final json = await api.graphRelated(projectInfo.id, result.name);
      setState(() {
        _related = GraphRelatedResult.fromJson(json);
        _loadingRelated = false;
      });
    } catch (e) {
      setState(() => _loadingRelated = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final graphStats = ref.watch(graphStatsProvider);
    final size = MediaQuery.of(context).size;

    return Dialog(
      backgroundColor: GolemTheme.bgSurface,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(12),
        side: const BorderSide(color: GolemTheme.border),
      ),
      child: SizedBox(
        width: size.width * 0.9,
        height: size.height * 0.9,
        child: Column(
          children: [
            // Title bar
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
              decoration: const BoxDecoration(
                border: Border(bottom: BorderSide(color: GolemTheme.border)),
              ),
              child: Row(
                children: [
                  const Icon(Icons.account_tree, size: 18, color: GolemTheme.accent),
                  const SizedBox(width: 8),
                  const Text('Graph Explorer',
                      style: TextStyle(fontSize: 15, fontWeight: FontWeight.w600)),
                  const Spacer(),
                  IconButton(
                    icon: const Icon(Icons.close, size: 18),
                    color: GolemTheme.textSecondary,
                    onPressed: () => Navigator.of(context).pop(),
                  ),
                ],
              ),
            ),
            // Content
            Expanded(
              child: Row(
                children: [
                  // Left pane — search
                  SizedBox(
                    width: 300,
                    child: _SearchPane(
                      controller: _searchController,
                      onChanged: _onSearchChanged,
                      results: _results,
                      selectedName: _selected?.name,
                      onSelect: _selectResult,
                      searching: _searching,
                      typeFilter: _typeFilter,
                      typeFilters: _typeFilters,
                      onTypeChanged: (t) {
                        setState(() => _typeFilter = t);
                        if (_searchController.text.length >= 2) {
                          _search(_searchController.text);
                        }
                      },
                    ),
                  ),
                  // Divider
                  const VerticalDivider(width: 1, color: GolemTheme.border),
                  // Right pane — detail
                  Expanded(
                    child: _selected != null
                        ? _DetailPane(
                            result: _selected!,
                            related: _related,
                            loading: _loadingRelated,
                            onNavigate: (name) {
                              _searchController.text = name;
                              _search(name);
                            },
                          )
                        : _StatsOverview(stats: graphStats),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _SearchPane extends StatelessWidget {
  final TextEditingController controller;
  final ValueChanged<String> onChanged;
  final List<GraphSearchResult> results;
  final String? selectedName;
  final ValueChanged<GraphSearchResult> onSelect;
  final bool searching;
  final String typeFilter;
  final List<String> typeFilters;
  final ValueChanged<String> onTypeChanged;

  const _SearchPane({
    required this.controller,
    required this.onChanged,
    required this.results,
    required this.selectedName,
    required this.onSelect,
    required this.searching,
    required this.typeFilter,
    required this.typeFilters,
    required this.onTypeChanged,
  });

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        Padding(
          padding: const EdgeInsets.all(12),
          child: TextField(
            controller: controller,
            onChanged: onChanged,
            style: const TextStyle(fontSize: 13),
            decoration: const InputDecoration(
              hintText: 'Search symbols...',
              prefixIcon: Icon(Icons.search, size: 18),
            ),
          ),
        ),
        // Type filter chips
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 12),
          child: Wrap(
            spacing: 4,
            children: typeFilters.map((t) => ChoiceChip(
                  label: Text(t == 'all' ? 'All' : t[0].toUpperCase() + t.substring(1),
                      style: const TextStyle(fontSize: 11)),
                  selected: typeFilter == t,
                  onSelected: (_) => onTypeChanged(t),
                  selectedColor: GolemTheme.accent.withOpacity(0.2),
                  backgroundColor: GolemTheme.bgPrimary,
                  side: const BorderSide(color: GolemTheme.border),
                  padding: EdgeInsets.zero,
                  labelPadding: const EdgeInsets.symmetric(horizontal: 6),
                  visualDensity: VisualDensity.compact,
                )).toList(),
          ),
        ),
        const SizedBox(height: 8),
        if (searching)
          const Padding(
            padding: EdgeInsets.all(16),
            child: CircularProgressIndicator(strokeWidth: 2, color: GolemTheme.accent),
          ),
        Expanded(
          child: ListView.builder(
            itemCount: results.length,
            itemBuilder: (_, i) {
              final r = results[i];
              final isSelected = r.name == selectedName;
              return InkWell(
                onTap: () => onSelect(r),
                child: Container(
                  color: isSelected ? GolemTheme.accent.withOpacity(0.1) : null,
                  padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Row(
                        children: [
                          _TypeIcon(type: r.type),
                          const SizedBox(width: 6),
                          Expanded(
                            child: Text(r.name, style: const TextStyle(fontSize: 13)),
                          ),
                          Text(
                            (r.score * 100).toStringAsFixed(0) + '%',
                            style: GolemTheme.metaStyle(fontSize: 10),
                          ),
                        ],
                      ),
                      Padding(
                        padding: const EdgeInsets.only(left: 22),
                        child: Text(
                          '${r.path}${r.line > 0 ? ":${r.line}" : ""}',
                          style: GolemTheme.metaStyle(fontSize: 10),
                          overflow: TextOverflow.ellipsis,
                        ),
                      ),
                    ],
                  ),
                ),
              );
            },
          ),
        ),
      ],
    );
  }
}

class _TypeIcon extends StatelessWidget {
  final String type;
  const _TypeIcon({required this.type});

  @override
  Widget build(BuildContext context) {
    final (icon, color) = switch (type) {
      'function' => (Icons.functions, GolemTheme.accent),
      'method' => (Icons.functions, GolemTheme.purple),
      'type' || 'class' || 'interface' => (Icons.data_object, GolemTheme.yellow),
      'file' => (Icons.insert_drive_file_outlined, GolemTheme.textSecondary),
      'document' || 'section' => (Icons.description_outlined, GolemTheme.green),
      _ => (Icons.code, GolemTheme.textSecondary),
    };
    return Icon(icon, size: 14, color: color);
  }
}

class _DetailPane extends StatelessWidget {
  final GraphSearchResult result;
  final GraphRelatedResult? related;
  final bool loading;
  final ValueChanged<String> onNavigate;

  const _DetailPane({
    required this.result,
    required this.related,
    required this.loading,
    required this.onNavigate,
  });

  @override
  Widget build(BuildContext context) {
    return SingleChildScrollView(
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // Header
          Row(
            children: [
              _TypeIcon(type: result.type),
              const SizedBox(width: 8),
              Text(result.name,
                  style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w600)),
            ],
          ),
          const SizedBox(height: 4),
          Text(
            '${result.type} \u2014 ${result.path}${result.line > 0 ? ":${result.line}" : ""}',
            style: GolemTheme.metaStyle(fontSize: 12),
          ),
          const SizedBox(height: 24),
          if (loading)
            const Center(
                child: CircularProgressIndicator(strokeWidth: 2, color: GolemTheme.accent))
          else if (related != null) ...[
            _RelationSection(
              title: 'Calls',
              nodes: related!.nodes
                  .where((n) => related!.edges
                      .any((e) => e.from == result.name || _nodeMatchesFrom(e, n, related!)))
                  .toList(),
              edges: related!.edges.where((e) => e.type == 'CALLS').toList(),
              sourceId: result.name,
              onNavigate: onNavigate,
              isOutbound: true,
            ),
            _RelationSection(
              title: 'Called by',
              nodes: related!.nodes,
              edges: related!.edges.where((e) => e.type == 'CALLS').toList(),
              sourceId: result.name,
              onNavigate: onNavigate,
              isOutbound: false,
            ),
            _RelationSection(
              title: 'Imports',
              nodes: related!.nodes,
              edges: related!.edges.where((e) => e.type == 'IMPORTS').toList(),
              sourceId: result.name,
              onNavigate: onNavigate,
              isOutbound: true,
            ),
          ],
        ],
      ),
    );
  }

  bool _nodeMatchesFrom(GraphEdge edge, GraphNode node, GraphRelatedResult related) {
    return edge.to == node.id;
  }
}

class _RelationSection extends StatelessWidget {
  final String title;
  final List<GraphNode> nodes;
  final List<GraphEdge> edges;
  final String sourceId;
  final ValueChanged<String> onNavigate;
  final bool isOutbound;

  const _RelationSection({
    required this.title,
    required this.nodes,
    required this.edges,
    required this.sourceId,
    required this.onNavigate,
    required this.isOutbound,
  });

  @override
  Widget build(BuildContext context) {
    // Filter relevant nodes based on edge direction
    final relevantNodeIds = <String>{};
    for (final e in edges) {
      if (isOutbound) {
        relevantNodeIds.add(e.to);
      } else {
        relevantNodeIds.add(e.from);
      }
    }

    final filteredNodes =
        nodes.where((n) => relevantNodeIds.contains(n.id) && n.name != sourceId).toList();

    if (filteredNodes.isEmpty) return const SizedBox.shrink();

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(title,
            style: const TextStyle(
                fontSize: 12, fontWeight: FontWeight.w600, color: GolemTheme.textSecondary)),
        const SizedBox(height: 4),
        ...filteredNodes.map((n) => InkWell(
              onTap: () => onNavigate(n.name),
              child: Padding(
                padding: const EdgeInsets.symmetric(vertical: 3),
                child: Row(
                  children: [
                    _TypeIcon(type: n.type),
                    const SizedBox(width: 6),
                    Text(n.name, style: const TextStyle(fontSize: 13, color: GolemTheme.accent)),
                    const SizedBox(width: 8),
                    Expanded(
                      child: Text(
                        '${n.path}${n.line > 0 ? ":${n.line}" : ""}',
                        style: GolemTheme.metaStyle(fontSize: 10),
                        overflow: TextOverflow.ellipsis,
                      ),
                    ),
                  ],
                ),
              ),
            )),
        const SizedBox(height: 16),
      ],
    );
  }
}

class _StatsOverview extends StatelessWidget {
  final GraphStats? stats;
  const _StatsOverview({this.stats});

  @override
  Widget build(BuildContext context) {
    if (stats == null) {
      return Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.account_tree, size: 48, color: GolemTheme.border),
            const SizedBox(height: 12),
            const Text('No knowledge graph', style: TextStyle(fontSize: 15)),
            const SizedBox(height: 8),
            Container(
              padding: const EdgeInsets.all(12),
              decoration: BoxDecoration(
                color: GolemTheme.bgPrimary,
                borderRadius: BorderRadius.circular(6),
              ),
              child: Text(
                'golem graph build && golem graph embed',
                style: GolemTheme.monoStyle(fontSize: 12),
              ),
            ),
          ],
        ),
      );
    }

    return SingleChildScrollView(
      padding: const EdgeInsets.all(24),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text('Graph Overview',
              style: TextStyle(fontSize: 18, fontWeight: FontWeight.w600)),
          const SizedBox(height: 4),
          const Text('Select a symbol from search to explore relationships.',
              style: TextStyle(fontSize: 13, color: GolemTheme.textSecondary)),
          const SizedBox(height: 24),
          Row(
            children: [
              _StatCard('Nodes', '${stats!.totalNodes}'),
              const SizedBox(width: 12),
              _StatCard('Edges', '${stats!.totalEdges}'),
              const SizedBox(width: 12),
              _StatCard('Embeddings', '${stats!.embeddingCount}'),
              const SizedBox(width: 12),
              _StatCard('Commits', '${stats!.commitCount}'),
            ],
          ),
          const SizedBox(height: 24),
          const Text('Node types',
              style: TextStyle(fontSize: 13, fontWeight: FontWeight.w600)),
          const SizedBox(height: 8),
          ...stats!.nodeTypes.entries.map((e) => Padding(
                padding: const EdgeInsets.symmetric(vertical: 2),
                child: Row(
                  mainAxisAlignment: MainAxisAlignment.spaceBetween,
                  children: [
                    Text(e.key, style: const TextStyle(fontSize: 12)),
                    Text('${e.value}',
                        style: const TextStyle(fontSize: 12, color: GolemTheme.textSecondary)),
                  ],
                ),
              )),
        ],
      ),
    );
  }
}

class _StatCard extends StatelessWidget {
  final String label;
  final String value;
  const _StatCard(this.label, this.value);

  @override
  Widget build(BuildContext context) {
    return Expanded(
      child: Container(
        padding: const EdgeInsets.all(12),
        decoration: BoxDecoration(
          color: GolemTheme.bgPrimary,
          borderRadius: BorderRadius.circular(8),
          border: Border.all(color: GolemTheme.border),
        ),
        child: Column(
          children: [
            Text(value,
                style: const TextStyle(fontSize: 20, fontWeight: FontWeight.w600)),
            const SizedBox(height: 2),
            Text(label,
                style: const TextStyle(fontSize: 11, color: GolemTheme.textSecondary)),
          ],
        ),
      ),
    );
  }
}
```

**Step 2: Add graph icon to shell top bar**

In `shell.dart`, add an `IconButton` for the graph explorer before the Launch button:

```dart
IconButton(
  icon: const Icon(Icons.account_tree, size: 18),
  color: GolemTheme.textSecondary,
  onPressed: () => showDialog(
    context: context,
    builder: (_) => const GraphExplorer(),
  ),
  tooltip: 'Graph Explorer',
  splashRadius: 18,
),
const SizedBox(width: 4),
```

**Step 3: Verify build**

Run: `cd ui/flutter && flutter analyze`
Expected: No errors

**Step 4: Commit**

```bash
git add ui/flutter/lib/views/graph_explorer.dart ui/flutter/lib/views/shell.dart
git commit -m "feat(ui): add graph explorer overlay with search and symbol navigation"
```

---

### Task 13: Process tab and status bar visual polish

**Files:**
- Modify: `ui/flutter/lib/views/shell.dart`

**Step 1: Polish process tabs**

Update `_ProcessTabs` to use pill-shaped backgrounds instead of bottom borders. Add a subtle pulsing animation for running process dots.

Update `_StatusBar` to show iteration info when a process is running (read from the state provider's sessions/tasks).

Use `GolemTheme.phaseColor` for the phase badge in the top bar.

**Step 2: Verify build**

Run: `cd ui/flutter && flutter analyze`
Expected: No errors

**Step 3: Commit**

```bash
git add ui/flutter/lib/views/shell.dart
git commit -m "feat(ui): polish process tabs and status bar"
```

---

### Task 14: Sessions auto-update via WebSocket

**Files:**
- Modify: `ui/flutter/lib/providers/project.dart`

**Step 1: Fix sessions to update via WebSocket**

The `ProjectStateNotifier` already handles `state_changed` messages. We need `SessionsNotifier` to also listen for `log_appended` messages from the same WebSocket.

Update `ProjectStateNotifier._connectWs` to also emit log updates. The simplest approach: add a callback in the provider that `SessionsNotifier` watches.

Alternative (simpler): make `SessionsNotifier` also connect to the watch WebSocket and listen for `log_appended` events.

In `ProjectStateNotifier._connectWs`, extend the onMessage handler:

```dart
onMessage: (data) {
  if (data['type'] == 'state_changed' && data['state'] != null) {
    state = ProjectState.fromJson(data['state'] as Map<String, dynamic>);
  }
  if (data['type'] == 'log_appended' && data['session'] != null) {
    _onLogAppended?.call(Session.fromJson(data['session'] as Map<String, dynamic>));
  }
},
```

Then wire a cross-provider callback so `SessionsNotifier` picks up new sessions.

**Step 2: Verify build**

Run: `cd ui/flutter && flutter analyze`
Expected: No errors

**Step 3: Commit**

```bash
git add ui/flutter/lib/providers/project.dart
git commit -m "fix(ui): auto-update sessions list via WebSocket log_appended events"
```

---

### Task 15: Integration testing and final verification

**Step 1: Run all Go tests**

Run: `go test ./... -v`
Expected: PASS

**Step 2: Run Flutter analysis**

Run: `cd ui/flutter && flutter analyze`
Expected: No issues

**Step 3: Build Go binary**

Run: `go build ./...`
Expected: Clean build

**Step 4: Manual smoke test**

1. Start server: `go run . serve`
2. In another terminal: `go run . graph build && go run . graph embed`
3. Launch UI (or test via curl):
   - `curl localhost:8314/api/projects` — verify projects listed
   - `curl localhost:8314/api/projects/<id>/graph/stats` — verify graph stats
   - `curl localhost:8314/api/projects/<id>/diff` — verify diff summary
4. If Flutter desktop is buildable: `cd ui/flutter && flutter run -d linux`

**Step 5: Commit any fixes**

```bash
git add -A
git commit -m "test: integration verification for UI improvements"
```
