# LSP Integration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace tree-sitter edge extraction with LSP-based extraction for precise call resolution, add live LSP MCP tools for agent sessions, and simplify the git history graph.

**Architecture:** New `internal/graph/lsp/` package with a language registry, JSON-RPC 2.0 client, and server manager. The builder uses LSP for symbol extraction and call resolution (falling back to tree-sitter), while a separate set of MCP tools expose live LSP queries during agent sessions. Git history tables are stripped down to CO_CHANGED edges only.

**Tech Stack:** `go.lsp.dev/protocol` + `go.lsp.dev/jsonrpc2` for LSP communication, existing `go-tree-sitter` for call site detection in hybrid mode, existing SQLite graph store unchanged.

---

### Task 1: Add LSP Protocol Dependencies

**Files:**
- Modify: `go.mod`

**Step 1: Add LSP dependencies**

Run:
```bash
cd /home/winler/projects/golem && go get go.lsp.dev/protocol@latest go.lsp.dev/jsonrpc2@latest go.lsp.dev/uri@latest
```

**Step 2: Verify dependencies resolve**

Run: `go build ./...`
Expected: clean build

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "feat(graph): add LSP protocol dependencies"
```

---

### Task 2: Language Registry

**Files:**
- Create: `internal/graph/lsp/registry.go`
- Create: `internal/graph/lsp/registry_test.go`

**Step 1: Write the failing test**

```go
// internal/graph/lsp/registry_test.go
package lsp

import (
	"testing"
)

func TestDetectLanguages(t *testing.T) {
	files := []string{
		"main.go",
		"lib.py",
		"app.ts",
		"index.js",
		"server.rs",
		"App.kt",
		"widget.dart",
		"Main.java",
		"README.md",
	}

	langs := DetectLanguages(files)

	expected := map[string]bool{
		"go":         true,
		"python":     true,
		"typescript": true,
		"javascript": true,
		"rust":       true,
		"kotlin":     true,
		"dart":       true,
		"java":       true,
	}

	if len(langs) != len(expected) {
		t.Fatalf("expected %d languages, got %d: %v", len(expected), len(langs), langNames(langs))
	}

	for _, cfg := range langs {
		if !expected[cfg.Language] {
			t.Errorf("unexpected language: %s", cfg.Language)
		}
	}
}

func TestDetectLanguages_duplicates(t *testing.T) {
	files := []string{"a.go", "b.go", "c.go"}
	langs := DetectLanguages(files)
	if len(langs) != 1 {
		t.Fatalf("expected 1 language, got %d", len(langs))
	}
	if langs[0].Language != "go" {
		t.Errorf("expected go, got %s", langs[0].Language)
	}
}

func TestServerConfigForExt(t *testing.T) {
	tests := []struct {
		ext  string
		lang string
	}{
		{".go", "go"},
		{".py", "python"},
		{".ts", "typescript"},
		{".tsx", "typescript"},
		{".js", "javascript"},
		{".jsx", "javascript"},
		{".rs", "rust"},
		{".kt", "kotlin"},
		{".kts", "kotlin"},
		{".dart", "dart"},
		{".java", "java"},
		{".txt", ""},
	}

	for _, tt := range tests {
		cfg := ConfigForExt(tt.ext)
		if tt.lang == "" {
			if cfg != nil {
				t.Errorf("ext %s: expected nil, got %s", tt.ext, cfg.Language)
			}
			continue
		}
		if cfg == nil {
			t.Errorf("ext %s: expected %s, got nil", tt.ext, tt.lang)
			continue
		}
		if cfg.Language != tt.lang {
			t.Errorf("ext %s: expected %s, got %s", tt.ext, tt.lang, cfg.Language)
		}
	}
}

func langNames(cfgs []ServerConfig) []string {
	var names []string
	for _, c := range cfgs {
		names = append(names, c.Language)
	}
	return names
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/graph/lsp/ -run TestDetect -v`
Expected: FAIL — package doesn't exist yet

**Step 3: Write the implementation**

```go
// internal/graph/lsp/registry.go
package lsp

import (
	"os/exec"
	"path/filepath"
	"strings"
)

// ServerConfig describes how to launch an LSP server for a language.
type ServerConfig struct {
	Language    string
	Binary      string
	Args        []string
	InstallHint string
	Extensions  []string
}

// registry is the built-in set of supported LSP servers.
var registry = []ServerConfig{
	{
		Language:    "go",
		Binary:      "gopls",
		Args:        []string{"serve"},
		InstallHint: "go install golang.org/x/tools/gopls@latest",
		Extensions:  []string{".go"},
	},
	{
		Language:    "python",
		Binary:      "pyright-langserver",
		Args:        []string{"--stdio"},
		InstallHint: "npm install -g pyright",
		Extensions:  []string{".py"},
	},
	{
		Language:    "typescript",
		Binary:      "typescript-language-server",
		Args:        []string{"--stdio"},
		InstallHint: "npm install -g typescript-language-server typescript",
		Extensions:  []string{".ts", ".tsx"},
	},
	{
		Language:    "javascript",
		Binary:      "typescript-language-server",
		Args:        []string{"--stdio"},
		InstallHint: "npm install -g typescript-language-server typescript",
		Extensions:  []string{".js", ".jsx", ".mjs", ".cjs"},
	},
	{
		Language:    "rust",
		Binary:      "rust-analyzer",
		Args:        nil,
		InstallHint: "rustup component add rust-analyzer",
		Extensions:  []string{".rs"},
	},
	{
		Language:    "java",
		Binary:      "jdtls",
		Args:        nil,
		InstallHint: "Install Eclipse JDT Language Server: https://github.com/eclipse-jdtls/eclipse.jdt.ls",
		Extensions:  []string{".java"},
	},
	{
		Language:    "kotlin",
		Binary:      "kotlin-language-server",
		Args:        nil,
		InstallHint: "Install Kotlin Language Server: https://github.com/fwcd/kotlin-language-server",
		Extensions:  []string{".kt", ".kts"},
	},
	{
		Language:    "dart",
		Binary:      "dart",
		Args:        []string{"language-server", "--protocol=lsp"},
		InstallHint: "Install Dart SDK: https://dart.dev/get-dart",
		Extensions:  []string{".dart"},
	},
}

// extToConfig maps file extension to registry index.
var extToConfig map[string]*ServerConfig

func init() {
	extToConfig = make(map[string]*ServerConfig)
	for i := range registry {
		for _, ext := range registry[i].Extensions {
			extToConfig[ext] = &registry[i]
		}
	}
}

// ConfigForExt returns the ServerConfig for a file extension, or nil.
func ConfigForExt(ext string) *ServerConfig {
	return extToConfig[ext]
}

// DetectLanguages scans file paths and returns unique ServerConfigs for detected languages.
func DetectLanguages(filePaths []string) []ServerConfig {
	seen := make(map[string]bool)
	var result []ServerConfig
	for _, path := range filePaths {
		ext := strings.ToLower(filepath.Ext(path))
		cfg := ConfigForExt(ext)
		if cfg == nil || seen[cfg.Language] {
			continue
		}
		seen[cfg.Language] = true
		result = append(result, *cfg)
	}
	return result
}

// CheckAvailability tests which LSP servers are installed.
// Returns two slices: available configs and missing configs with install hints.
func CheckAvailability(configs []ServerConfig) (available, missing []ServerConfig) {
	for _, cfg := range configs {
		if _, err := exec.LookPath(cfg.Binary); err == nil {
			available = append(available, cfg)
		} else {
			missing = append(missing, cfg)
		}
	}
	return
}
```

**Step 4: Run tests**

Run: `go test ./internal/graph/lsp/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/graph/lsp/registry.go internal/graph/lsp/registry_test.go
git commit -m "feat(lsp): add language registry with detection and availability checks"
```

---

### Task 3: LSP Client (JSON-RPC 2.0)

**Files:**
- Create: `internal/graph/lsp/client.go`
- Create: `internal/graph/lsp/client_test.go`

**Step 1: Write the failing test**

```go
// internal/graph/lsp/client_test.go
package lsp

import (
	"os/exec"
	"testing"
)

func TestClientStartShutdown(t *testing.T) {
	// Skip if gopls not installed
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not installed")
	}

	cfg := ServerConfig{
		Language: "go",
		Binary:   "gopls",
		Args:     []string{"serve"},
	}

	client, err := StartClient(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	if client.ServerName() == "" {
		t.Error("expected server name after initialize")
	}

	if err := client.Shutdown(); err != nil {
		t.Errorf("shutdown: %v", err)
	}
}

func TestClientDocumentSymbols(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not installed")
	}

	dir := t.TempDir()

	// Create a minimal Go module
	writeTestFile(t, dir, "go.mod", "module testmod\n\ngo 1.21\n")
	writeTestFile(t, dir, "main.go", `package main

func main() {}

func helper() string {
	return "hello"
}

type Config struct {
	Name string
}
`)

	cfg := ServerConfig{
		Language: "go",
		Binary:   "gopls",
		Args:     []string{"serve"},
	}

	client, err := StartClient(cfg, dir)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer client.Shutdown()

	symbols, err := client.DocumentSymbols(dir + "/main.go")
	if err != nil {
		t.Fatalf("symbols: %v", err)
	}

	names := make(map[string]bool)
	for _, s := range symbols {
		names[s.Name] = true
	}

	for _, expected := range []string{"main", "helper", "Config"} {
		if !names[expected] {
			t.Errorf("missing symbol %q in %v", expected, names)
		}
	}
}

func TestClientDefinition(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not installed")
	}

	dir := t.TempDir()
	writeTestFile(t, dir, "go.mod", "module testmod\n\ngo 1.21\n")
	writeTestFile(t, dir, "main.go", `package main

func main() {
	helper()
}

func helper() {}
`)

	cfg := ServerConfig{
		Language: "go",
		Binary:   "gopls",
		Args:     []string{"serve"},
	}

	client, err := StartClient(cfg, dir)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer client.Shutdown()

	// Line 3 (0-indexed), col for "helper" in helper() call — col 1
	locs, err := client.Definition(dir+"/main.go", 3, 1)
	if err != nil {
		t.Fatalf("definition: %v", err)
	}

	if len(locs) == 0 {
		t.Fatal("expected at least 1 definition location")
	}

	// Should point to line 6 (0-indexed) where helper is defined
	if locs[0].Line != 6 {
		t.Errorf("expected definition at line 6, got %d", locs[0].Line)
	}
}

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := dir + "/" + name
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/graph/lsp/ -run TestClient -v -timeout 60s`
Expected: FAIL — StartClient not defined

**Step 3: Write the implementation**

```go
// internal/graph/lsp/client.go
package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// Symbol represents a code symbol extracted from a document.
type Symbol struct {
	Name string
	Kind protocol.SymbolKind
	Line int // 0-indexed
	Col  int // 0-indexed
}

// Location represents a source code location.
type Location struct {
	URI  string
	Line int // 0-indexed
	Col  int // 0-indexed
}

// HoverResult holds hover information for a position.
type HoverResult struct {
	Contents string
}

// Diagnostic represents a compiler/linter diagnostic.
type Diagnostic struct {
	Message  string
	Severity int // 1=Error, 2=Warning, 3=Info, 4=Hint
	Line     int // 0-indexed
	Col      int // 0-indexed
}

// Client manages an LSP server subprocess and communicates via JSON-RPC 2.0.
type Client struct {
	cfg        ServerConfig
	cmd        *exec.Cmd
	conn       jsonrpc2.Conn
	rootURI    protocol.DocumentURI
	serverName string
	mu         sync.Mutex
	openFiles  map[string]int // uri -> version
}

// StartClient launches an LSP server and performs the initialize handshake.
func StartClient(cfg ServerConfig, projectRoot string) (*Client, error) {
	args := cfg.Args
	cmd := exec.Command(cfg.Binary, args...)
	cmd.Dir = projectRoot

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = nil // discard server stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting %s: %w", cfg.Binary, err)
	}

	stream := jsonrpc2.NewStream(newReadWriteCloser(stdout, stdin))
	ctx := context.Background()
	conn := jsonrpc2.NewConn(stream)

	// Start connection handler in background
	go conn.Run(ctx)

	absRoot, _ := filepath.Abs(projectRoot)
	rootURI := uri.File(absRoot)

	c := &Client{
		cfg:       cfg,
		cmd:       cmd,
		conn:      conn,
		rootURI:   protocol.DocumentURI(rootURI),
		openFiles: make(map[string]int),
	}

	if err := c.initialize(ctx); err != nil {
		c.kill()
		return nil, fmt.Errorf("initialize: %w", err)
	}

	return c, nil
}

func (c *Client) initialize(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	params := protocol.InitializeParams{
		RootURI: c.rootURI,
		Capabilities: protocol.ClientCapabilities{},
	}

	var result protocol.InitializeResult
	if err := c.call(ctx, "initialize", params, &result); err != nil {
		return err
	}

	c.serverName = result.ServerInfo.Name

	// Send initialized notification
	return c.notify(ctx, "initialized", struct{}{})
}

// ServerName returns the name reported by the LSP server.
func (c *Client) ServerName() string {
	return c.serverName
}

// DocumentSymbols returns all symbols defined in a file.
func (c *Client) DocumentSymbols(filePath string) ([]Symbol, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fileURI := uri.File(filePath)
	if err := c.ensureOpen(ctx, filePath); err != nil {
		return nil, err
	}

	params := protocol.DocumentSymbolParams{
		TextDocument: protocol.TextDocumentIdentifier{
			URI: protocol.DocumentURI(fileURI),
		},
	}

	var raw json.RawMessage
	if err := c.call(ctx, "textDocument/documentSymbol", params, &raw); err != nil {
		return nil, err
	}

	// Response can be []DocumentSymbol or []SymbolInformation
	var docSymbols []protocol.DocumentSymbol
	if err := json.Unmarshal(raw, &docSymbols); err == nil && len(docSymbols) > 0 {
		return flattenDocSymbols(docSymbols), nil
	}

	var symInfos []protocol.SymbolInformation
	if err := json.Unmarshal(raw, &symInfos); err == nil {
		var syms []Symbol
		for _, si := range symInfos {
			syms = append(syms, Symbol{
				Name: si.Name,
				Kind: si.Kind,
				Line: int(si.Location.Range.Start.Line),
				Col:  int(si.Location.Range.Start.Character),
			})
		}
		return syms, nil
	}

	return nil, nil
}

// Definition returns the definition location(s) for a symbol at the given position.
func (c *Client) Definition(filePath string, line, col int) ([]Location, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fileURI := uri.File(filePath)
	if err := c.ensureOpen(ctx, filePath); err != nil {
		return nil, err
	}

	params := protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{
				URI: protocol.DocumentURI(fileURI),
			},
			Position: protocol.Position{
				Line:      uint32(line),
				Character: uint32(col),
			},
		},
	}

	var raw json.RawMessage
	if err := c.call(ctx, "textDocument/definition", params, &raw); err != nil {
		return nil, err
	}

	return parseLocations(raw)
}

// References returns all references to the symbol at the given position.
func (c *Client) References(filePath string, line, col int) ([]Location, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	fileURI := uri.File(filePath)
	if err := c.ensureOpen(ctx, filePath); err != nil {
		return nil, err
	}

	params := protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{
				URI: protocol.DocumentURI(fileURI),
			},
			Position: protocol.Position{
				Line:      uint32(line),
				Character: uint32(col),
			},
		},
		Context: protocol.ReferenceContext{
			IncludeDeclaration: false,
		},
	}

	var raw json.RawMessage
	if err := c.call(ctx, "textDocument/references", params, &raw); err != nil {
		return nil, err
	}

	return parseLocations(raw)
}

// Hover returns hover information for the symbol at the given position.
func (c *Client) Hover(filePath string, line, col int) (*HoverResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fileURI := uri.File(filePath)
	if err := c.ensureOpen(ctx, filePath); err != nil {
		return nil, err
	}

	params := protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{
				URI: protocol.DocumentURI(fileURI),
			},
			Position: protocol.Position{
				Line:      uint32(line),
				Character: uint32(col),
			},
		},
	}

	var result protocol.Hover
	if err := c.call(ctx, "textDocument/hover", params, &result); err != nil {
		return nil, err
	}

	return &HoverResult{Contents: result.Contents.Value}, nil
}

// NotifyDidSave tells the LSP server a file was saved.
func (c *Client) NotifyDidSave(filePath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	fileURI := uri.File(filePath)
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	params := protocol.DidSaveTextDocumentParams{
		TextDocument: protocol.TextDocumentIdentifier{
			URI: protocol.DocumentURI(fileURI),
		},
		Text: string(content),
	}

	return c.notify(ctx, "textDocument/didSave", params)
}

// Shutdown gracefully stops the LSP server.
func (c *Client) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Close all open documents
	c.mu.Lock()
	for fileURI := range c.openFiles {
		c.notify(ctx, "textDocument/didClose", protocol.DidCloseTextDocumentParams{
			TextDocument: protocol.TextDocumentIdentifier{
				URI: protocol.DocumentURI(fileURI),
			},
		})
	}
	c.openFiles = make(map[string]int)
	c.mu.Unlock()

	var nothing interface{}
	c.call(ctx, "shutdown", nil, &nothing)
	c.notify(ctx, "exit", nil)
	return c.cmd.Wait()
}

func (c *Client) kill() {
	if c.cmd.Process != nil {
		c.cmd.Process.Kill()
		c.cmd.Wait()
	}
}

func (c *Client) ensureOpen(ctx context.Context, filePath string) error {
	fileURI := string(uri.File(filePath))

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.openFiles[fileURI]; ok {
		return nil
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	langID := c.cfg.Language
	params := protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        protocol.DocumentURI(fileURI),
			LanguageID: protocol.LanguageIdentifier(langID),
			Version:    1,
			Text:       string(content),
		},
	}

	c.openFiles[fileURI] = 1
	return c.notify(ctx, "textDocument/didOpen", params)
}

func (c *Client) call(ctx context.Context, method string, params, result interface{}) error {
	data, err := json.Marshal(params)
	if err != nil {
		return err
	}
	raw := json.RawMessage(data)
	resp, err := c.conn.Call(ctx, method, raw)
	if err != nil {
		return err
	}
	if result != nil {
		return json.Unmarshal(resp, result)
	}
	return nil
}

func (c *Client) notify(ctx context.Context, method string, params interface{}) error {
	data, err := json.Marshal(params)
	if err != nil {
		return err
	}
	raw := json.RawMessage(data)
	return c.conn.Notify(ctx, method, raw)
}

func flattenDocSymbols(symbols []protocol.DocumentSymbol) []Symbol {
	var result []Symbol
	for _, s := range symbols {
		result = append(result, Symbol{
			Name: s.Name,
			Kind: s.Kind,
			Line: int(s.Range.Start.Line),
			Col:  int(s.Range.Start.Character),
		})
		if len(s.Children) > 0 {
			result = append(result, flattenDocSymbols(s.Children)...)
		}
	}
	return result
}

func parseLocations(raw json.RawMessage) ([]Location, error) {
	if raw == nil || string(raw) == "null" {
		return nil, nil
	}

	// Try as single Location
	var single protocol.Location
	if err := json.Unmarshal(raw, &single); err == nil && single.URI != "" {
		return []Location{{
			URI:  string(single.URI),
			Line: int(single.Range.Start.Line),
			Col:  int(single.Range.Start.Character),
		}}, nil
	}

	// Try as []Location
	var locs []protocol.Location
	if err := json.Unmarshal(raw, &locs); err == nil {
		var result []Location
		for _, l := range locs {
			result = append(result, Location{
				URI:  string(l.URI),
				Line: int(l.Range.Start.Line),
				Col:  int(l.Range.Start.Character),
			})
		}
		return result, nil
	}

	return nil, nil
}
```

Also create a helper for the stream:

```go
// internal/graph/lsp/stream.go
package lsp

import "io"

type readWriteCloser struct {
	io.ReadCloser
	io.WriteCloser
}

func newReadWriteCloser(r io.ReadCloser, w io.WriteCloser) io.ReadWriteCloser {
	return &readWriteCloser{r, w}
}

func (rwc *readWriteCloser) Close() error {
	rerr := rwc.ReadCloser.Close()
	werr := rwc.WriteCloser.Close()
	if rerr != nil {
		return rerr
	}
	return werr
}
```

**Step 4: Run tests**

Run: `go test ./internal/graph/lsp/ -run TestClient -v -timeout 60s`
Expected: PASS (or SKIP if gopls not installed)

**Step 5: Commit**

```bash
git add internal/graph/lsp/client.go internal/graph/lsp/client_test.go internal/graph/lsp/stream.go
git commit -m "feat(lsp): add LSP client with JSON-RPC 2.0 communication"
```

---

### Task 4: LSP Manager (Multi-Language Server Lifecycle)

**Files:**
- Create: `internal/graph/lsp/manager.go`
- Create: `internal/graph/lsp/manager_test.go`

**Step 1: Write the failing test**

```go
// internal/graph/lsp/manager_test.go
package lsp

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestManagerStartShutdown(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not installed")
	}

	dir := t.TempDir()
	writeTestFile(t, dir, "go.mod", "module testmod\n\ngo 1.21\n")
	writeTestFile(t, dir, "main.go", "package main\nfunc main() {}\n")

	mgr := NewManager(dir)

	configs := []ServerConfig{
		{Language: "go", Binary: "gopls", Args: []string{"serve"}},
	}

	if err := mgr.Start(configs); err != nil {
		t.Fatalf("start: %v", err)
	}

	client := mgr.ClientFor("go")
	if client == nil {
		t.Fatal("expected go client")
	}

	if mgr.ClientFor("python") != nil {
		t.Error("expected nil for unavailable language")
	}

	if err := mgr.Shutdown(); err != nil {
		t.Errorf("shutdown: %v", err)
	}
}

func TestManagerStartSkipsMissing(t *testing.T) {
	dir := t.TempDir()

	mgr := NewManager(dir)
	configs := []ServerConfig{
		{Language: "fake", Binary: "nonexistent-lsp-binary-12345"},
	}

	// Start should not error — it skips servers that fail to start
	err := mgr.Start(configs)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if mgr.ClientFor("fake") != nil {
		t.Error("expected nil for failed server")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/graph/lsp/ -run TestManager -v -timeout 60s`
Expected: FAIL — NewManager not defined

**Step 3: Write the implementation**

```go
// internal/graph/lsp/manager.go
package lsp

import (
	"fmt"
	"os"
	"sync"
)

// Manager manages multiple LSP server instances for different languages.
type Manager struct {
	root    string
	clients map[string]*Client
	mu      sync.RWMutex
}

// NewManager creates a new LSP manager for the given project root.
func NewManager(root string) *Manager {
	return &Manager{
		root:    root,
		clients: make(map[string]*Client),
	}
}

// Start launches LSP servers for the given configs in parallel.
// Servers that fail to start are logged and skipped (not an error).
func (m *Manager) Start(configs []ServerConfig) error {
	type result struct {
		lang   string
		client *Client
		err    error
	}

	ch := make(chan result, len(configs))
	for _, cfg := range configs {
		go func(c ServerConfig) {
			client, err := StartClient(c, m.root)
			ch <- result{lang: c.Language, client: client, err: err}
		}(cfg)
	}

	for range configs {
		r := <-ch
		if r.err != nil {
			fmt.Fprintf(os.Stderr, "golem: LSP %s failed to start: %v\n", r.lang, r.err)
			continue
		}
		m.mu.Lock()
		m.clients[r.lang] = r.client
		m.mu.Unlock()
	}

	return nil
}

// ClientFor returns the LSP client for a language, or nil if not available.
func (m *Manager) ClientFor(lang string) *Client {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.clients[lang]
}

// Languages returns the list of languages with running LSP servers.
func (m *Manager) Languages() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var langs []string
	for lang := range m.clients {
		langs = append(langs, lang)
	}
	return langs
}

// NotifyFilesChanged sends didSave notifications for changed files to the appropriate LSP servers.
func (m *Manager) NotifyFilesChanged(filePaths []string) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, path := range filePaths {
		cfg := ConfigForExt(filepath.Ext(path))
		if cfg == nil {
			continue
		}
		client := m.clients[cfg.Language]
		if client == nil {
			continue
		}
		client.NotifyDidSave(path)
	}
}

// Shutdown gracefully stops all running LSP servers.
func (m *Manager) Shutdown() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var firstErr error
	for lang, client := range m.clients {
		if err := client.Shutdown(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("shutting down %s: %w", lang, err)
		}
	}
	m.clients = make(map[string]*Client)
	return firstErr
}
```

Note: add `"path/filepath"` to imports in manager.go.

**Step 4: Run tests**

Run: `go test ./internal/graph/lsp/ -run TestManager -v -timeout 60s`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/graph/lsp/manager.go internal/graph/lsp/manager_test.go
git commit -m "feat(lsp): add manager for parallel multi-language server lifecycle"
```

---

### Task 5: LSP Extractor (Hybrid Node/Edge Extraction)

**Files:**
- Create: `internal/graph/lsp/extractor.go`
- Create: `internal/graph/lsp/extractor_test.go`

**Step 1: Write the failing test**

```go
// internal/graph/lsp/extractor_test.go
package lsp

import (
	"os/exec"
	"testing"

	"github.com/lofari/golem/internal/graph/model"
)

func TestExtract(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not installed")
	}

	dir := t.TempDir()
	writeTestFile(t, dir, "go.mod", "module testmod\n\ngo 1.21\n")
	writeTestFile(t, dir, "main.go", `package main

func main() {
	helper()
}

func helper() string {
	return "hello"
}

type Config struct {
	Name string
}
`)

	cfg := ServerConfig{
		Language: "go",
		Binary:   "gopls",
		Args:     []string{"serve"},
	}
	client, err := StartClient(cfg, dir)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer client.Shutdown()

	nodes, edges, err := Extract(client, dir, "main.go")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}

	// Should have: file node, main, helper, Config
	nodeNames := make(map[string]bool)
	for _, n := range nodes {
		nodeNames[n.Name] = true
	}

	for _, expected := range []string{"main", "helper", "Config"} {
		if !nodeNames[expected] {
			t.Errorf("missing node %q", expected)
		}
	}

	// Should have DEFINES edges
	hasDefines := false
	for _, e := range edges {
		if e.Type == "DEFINES" {
			hasDefines = true
			break
		}
	}
	if !hasDefines {
		t.Error("expected DEFINES edges")
	}

	// Should have a CALLS edge from main to helper (resolved)
	hasCalls := false
	for _, e := range edges {
		if e.Type == "CALLS" {
			hasCalls = true
			// Verify it's resolved — target should not start with "call:"
			if len(e.To) > 5 && e.To[:5] == "call:" {
				t.Error("CALLS edge target should be resolved, not a call: prefix")
			}
			break
		}
	}
	if !hasCalls {
		t.Error("expected CALLS edges")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/graph/lsp/ -run TestExtract -v -timeout 60s`
Expected: FAIL — Extract not defined

**Step 3: Write the implementation**

```go
// internal/graph/lsp/extractor.go
package lsp

import (
	"fmt"
	"path/filepath"
	"strings"

	"go.lsp.dev/protocol"

	"github.com/lofari/golem/internal/graph/model"
	"github.com/lofari/golem/internal/graph/treesitter"
)

// Extract uses the LSP client for symbol extraction and definition resolution.
// Tree-sitter is used for call-site detection. The filePath should be relative
// to the project root; absDir is the absolute project root.
func Extract(client *Client, absDir, relPath string) ([]model.Node, []model.Edge, error) {
	absPath := filepath.Join(absDir, relPath)
	var nodes []model.Node
	var edges []model.Edge

	// File node
	fileID := fmt.Sprintf("file:%s", relPath)
	nodes = append(nodes, model.Node{
		ID:   fileID,
		Type: "file",
		Name: relPath,
		Path: relPath,
		Line: 1,
	})

	// Step 1: Get symbols from LSP
	symbols, err := client.DocumentSymbols(absPath)
	if err != nil {
		return nodes, edges, fmt.Errorf("document symbols: %w", err)
	}

	for _, sym := range symbols {
		nodeType, prefix := symbolKindToNodeType(sym.Kind)
		if nodeType == "" {
			continue
		}

		id := fmt.Sprintf("%s:%s:%s", prefix, relPath, sym.Name)
		nodes = append(nodes, model.Node{
			ID:   id,
			Type: nodeType,
			Name: sym.Name,
			Path: relPath,
			Line: sym.Line + 1, // convert 0-indexed to 1-indexed
		})
		edges = append(edges, model.Edge{From: fileID, To: id, Type: "DEFINES"})
	}

	// Step 2: Use tree-sitter to find call sites
	src, err := os.ReadFile(absPath)
	if err != nil {
		return nodes, edges, nil // can't read — skip call extraction
	}

	lang := treesitter.DetectLanguage(relPath)
	if lang == "" {
		return nodes, edges, nil // unsupported for tree-sitter call detection
	}

	tree, _, err := treesitter.ParseBytes(src, lang)
	if err != nil {
		return nodes, edges, nil
	}

	callSites := treesitter.ExtractCallSites(relPath, lang, tree, src)

	// Step 3: Resolve each call site via LSP definition
	for _, cs := range callSites {
		locs, err := client.Definition(absPath, cs.Line, cs.Col)
		if err != nil || len(locs) == 0 {
			// Fallback: unresolved call edge
			edges = append(edges, model.Edge{
				From: cs.CallerID,
				To:   fmt.Sprintf("call:%s", cs.Name),
				Type: "CALLS",
			})
			continue
		}

		// Resolve to target node ID
		targetPath := uriToRelPath(locs[0].URI, absDir)
		targetLine := locs[0].Line + 1 // convert to 1-indexed

		targetID := resolveTargetID(nodes, targetPath, targetLine)
		if targetID == "" {
			// Target is in another file — construct best-guess ID
			targetID = fmt.Sprintf("fn:%s:%s", targetPath, cs.Name)
		}

		edges = append(edges, model.Edge{
			From: cs.CallerID,
			To:   targetID,
			Type: "CALLS",
		})
	}

	return nodes, edges, nil
}

func symbolKindToNodeType(kind protocol.SymbolKind) (nodeType, prefix string) {
	switch kind {
	case protocol.SymbolKindFunction:
		return "function", "fn"
	case protocol.SymbolKindMethod:
		return "method", "method"
	case protocol.SymbolKindClass:
		return "type", "type"
	case protocol.SymbolKindStruct:
		return "type", "type"
	case protocol.SymbolKindInterface:
		return "type", "type"
	case protocol.SymbolKindEnum:
		return "type", "type"
	default:
		return "", ""
	}
}

func uriToRelPath(fileURI string, absDir string) string {
	// Strip file:// prefix
	path := strings.TrimPrefix(fileURI, "file://")
	rel, err := filepath.Rel(absDir, path)
	if err != nil {
		return path
	}
	return rel
}

func resolveTargetID(nodes []model.Node, targetPath string, targetLine int) string {
	for _, n := range nodes {
		if n.Path == targetPath && n.Line == targetLine {
			return n.ID
		}
	}
	return ""
}
```

**Step 4: Add ExtractCallSites to tree-sitter package**

This is needed for the hybrid approach. Add to `internal/graph/treesitter/extractor.go`:

```go
// CallSite represents a function call found by tree-sitter.
type CallSite struct {
	Name     string // called function name
	CallerID string // node ID of enclosing function
	Line     int    // 0-indexed line
	Col      int    // 0-indexed column
}

// ExtractCallSites finds call expressions using tree-sitter AST walking.
func ExtractCallSites(filePath, lang string, tree *sitter.Tree, src []byte) []CallSite {
	var sites []CallSite
	walkCallSites(tree.RootNode(), filePath, lang, src, &sites)
	return sites
}

func walkCallSites(node *sitter.Node, filePath, lang string, src []byte, sites *[]CallSite) {
	if node.Type() == "call_expression" {
		fnNode := node.ChildByFieldName("function")
		if fnNode != nil {
			callName := fnNode.Content(src)
			if callName != "" && !strings.Contains(callName, "(") {
				caller := findEnclosingFunc(node, filePath, src)
				if caller != "" {
					*sites = append(*sites, CallSite{
						Name:     callName,
						CallerID: caller,
						Line:     int(fnNode.StartPoint().Row),
						Col:      int(fnNode.StartPoint().Column),
					})
				}
			}
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child != nil {
			walkCallSites(child, filePath, lang, src, sites)
		}
	}
}
```

**Step 5: Run tests**

Run: `go test ./internal/graph/lsp/ -run TestExtract -v -timeout 60s`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/graph/lsp/extractor.go internal/graph/lsp/extractor_test.go internal/graph/treesitter/extractor.go
git commit -m "feat(lsp): add hybrid LSP+tree-sitter extractor with call resolution"
```

---

### Task 6: Integrate LSP Extractor into Builder

**Files:**
- Modify: `internal/graph/builder.go:30-44` (Builder struct + NewBuilder)
- Modify: `internal/graph/builder.go:47-124` (BuildFull)
- Modify: `internal/graph/builder.go:128-212` (Sync)
- Modify: `internal/graph/builder_test.go`

**Step 1: Write the failing test**

Add to `internal/graph/builder_test.go`:

```go
func TestBuilderWithLSPFallback(t *testing.T) {
	// Test that builder works with no LSP (falls back to tree-sitter)
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module testmod\n\ngo 1.21\n")
	writeFile(t, dir, "main.go", "package main\nfunc main() {}\n")

	dbPath := filepath.Join(dir, "graph.db")
	store, err := graph.OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	builder := graph.NewBuilder(store)
	if err := builder.BuildFull(dir); err != nil {
		t.Fatalf("build: %v", err)
	}

	stats, _ := store.Stats()
	if stats.TotalNodes == 0 {
		t.Error("expected nodes")
	}
}
```

**Step 2: Run test to verify current behavior**

Run: `go test ./internal/graph/ -run TestBuilderWithLSP -v`
Expected: May pass with existing code, but we need to update the builder

**Step 3: Update Builder to accept optional LSP Manager**

Modify `internal/graph/builder.go`:

- Add `lspManager *lsp.Manager` field to Builder struct
- Add `WithLSP(mgr *lsp.Manager)` option
- In BuildFull/Sync, check if LSP client exists for the file's language before falling back to tree-sitter
- The fallback chain: LSP extract -> tree-sitter extract -> file-only

Key changes to `BuildFull`:

```go
func (b *Builder) BuildFull(projectPath string) error {
	// ... existing clear logic ...

	err := filepath.WalkDir(projectPath, func(path string, d os.DirEntry, err error) error {
		// ... existing skip logic ...

		relPath, _ := filepath.Rel(projectPath, path)
		src, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		// Try LSP first
		if b.lspManager != nil {
			lang := lspLangForFile(relPath)
			if client := b.lspManager.ClientFor(lang); client != nil {
				nodes, edges, err := lsp.Extract(client, projectPath, relPath)
				if err == nil {
					allNodes = append(allNodes, nodes...)
					allEdges = append(allEdges, edges...)
					return nil
				}
				// LSP failed — fall through to tree-sitter
			}
		}

		// Tree-sitter fallback
		lang := treesitter.DetectLanguage(relPath)
		if lang == "" {
			nodes, edges := treesitter.ExtractFileOnly(relPath)
			allNodes = append(allNodes, nodes...)
			allEdges = append(allEdges, edges...)
			return nil
		}

		tree, _, err := treesitter.ParseBytes(src, lang)
		if err != nil {
			return nil
		}
		nodes, edges := treesitter.Extract(relPath, lang, tree, src)
		allNodes = append(allNodes, nodes...)
		allEdges = append(allEdges, edges...)
		return nil
	})
	// ... rest unchanged ...
}
```

**Step 4: Run tests**

Run: `go test ./internal/graph/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/graph/builder.go internal/graph/builder_test.go
git commit -m "feat(graph): integrate LSP extractor into builder with fallback chain"
```

---

### Task 7: Simplify Git History — Remove Commits/Authors Tables

**Files:**
- Modify: `internal/graph/store.go:57-146` (schema — remove commits, authors tables)
- Modify: `internal/graph/store.go:214-218` (Clear — remove commits/authors references)
- Modify: `internal/graph/store.go:403-617` (remove commit/author methods)
- Modify: `internal/graph/history.go` (rewrite to only compute CO_CHANGED)
- Modify: `internal/graph/model/model.go:28-47` (remove Commit, Author types)
- Modify: `internal/graph/builder.go:112-115` (update history call)
- Modify: `internal/graph/history_test.go`
- Modify: `internal/graph/store_test.go`
- Modify: `internal/mcp/graph_tools.go` (remove find_recent_changes, find_file_history)
- Modify: `internal/mcp/server.go:27,48-49` (remove tool registrations)
- Modify: `internal/graph/context/engine.go:236-251` (recency boost via git)
- Modify: `cmd/graph.go:57-63` (remove commit/author stats)

**Step 1: Remove Commit/Author types from model**

Remove `Commit` and `Author` structs from `internal/graph/model/model.go:28-41`. Keep `CoChangedResult`.

**Step 2: Remove commits/authors table schema from store**

In `internal/graph/store.go`, remove from the schema string:
- `CREATE TABLE IF NOT EXISTS commits` block (lines 77-84)
- `CREATE TABLE IF NOT EXISTS authors` block (lines 85-88)
- `idx_commits_author` and `idx_commits_timestamp` indexes (lines 94-95)

Remove methods: `InsertCommitBatch`, `InsertAuthorBatch`, `QueryAuthorByEmail`, `QueryRecentChanges`, `QueryFilesModifiedByCommit`, `QueryCommitsByFile`, `QueryCommitBySHA`, `CommitCount`, `AuthorCount`, `DeleteHistory`.

Update `Clear()` to only delete nodes, edges (no commits/authors).

**Step 3: Rewrite history.go to only compute CO_CHANGED**

Replace `HistoryBuilder` with a simpler `ComputeCoChanged` function:

```go
// internal/graph/history.go
package graph

import (
	"os/exec"
	"strconv"
	"strings"
)

const coChangedMinCount = 3

// ComputeCoChanged parses git log and creates CO_CHANGED edges for files
// that frequently change together. Does not store commits or authors.
func ComputeCoChanged(store *Store, projectPath string, depth int) error {
	if depth <= 0 {
		depth = 500
	}

	// Clear existing CO_CHANGED edges
	store.db.Exec("DELETE FROM edges WHERE type = 'CO_CHANGED'")

	// Get file lists per commit from git log
	cmd := exec.Command("git", "log",
		"--format=%H",
		"--name-only",
		"-n", strconv.Itoa(depth),
	)
	cmd.Dir = projectPath
	out, err := cmd.Output()
	if err != nil {
		return err
	}

	// Parse: SHA line, then file lines, then blank line
	commitFiles := parseCommitFiles(string(out))

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
			store.InsertEdgeWithWeight("file:"+pair[0], "file:"+pair[1], "CO_CHANGED", count)
		}
	}

	return nil
}

func parseCommitFiles(output string) [][]string {
	var result [][]string
	var currentFiles []string

	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			if len(currentFiles) > 0 {
				result = append(result, currentFiles)
				currentFiles = nil
			}
			continue
		}
		if isSHA(line) {
			if len(currentFiles) > 0 {
				result = append(result, currentFiles)
				currentFiles = nil
			}
			continue
		}
		currentFiles = append(currentFiles, line)
	}
	if len(currentFiles) > 0 {
		result = append(result, currentFiles)
	}
	return result
}

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
```

**Step 4: Update builder to use ComputeCoChanged**

In `internal/graph/builder.go`, replace:
```go
hb := NewHistoryBuilder(b.store, b.historyDepth)
if err := hb.Build(projectPath); err != nil {
```
with:
```go
if err := ComputeCoChanged(b.store, projectPath, b.historyDepth); err != nil {
```

Same for `Sync`.

**Step 5: Update context engine recency boost**

In `internal/graph/context/engine.go`, change `recencyBoost` to use git directly:

```go
func recencyBoost(projectDir string, candidates []candidate, recentN int) []candidate {
	for i, c := range candidates {
		if c.Node.Path == "" {
			continue
		}
		cmd := exec.Command("git", "log", "--format=%H", "--since=7d", "-n", "1", "--", c.Node.Path)
		cmd.Dir = projectDir
		out, err := cmd.Output()
		if err != nil || len(strings.TrimSpace(string(out))) == 0 {
			continue
		}
		candidates[i].Score += recencyBoostMax
	}
	return candidates
}
```

Note: `BuildContextMap` signature needs to accept `projectDir string` parameter for git access.

**Step 6: Remove MCP tools find_recent_changes and find_file_history**

In `internal/mcp/graph_tools.go`, delete:
- `findRecentChangesTool()` + `handleFindRecentChanges` (lines 336-417)
- `findFileHistoryTool()` + `handleFindFileHistory` (lines 419-485)

In `internal/mcp/server.go`:
- Remove from `ListTools()` return: `"find_recent_changes"`, `"find_file_history"`
- Remove from `registerTools()`: the two `AddTool` calls for these tools

**Step 7: Update cmd/graph.go**

Remove commit/author stats printing from `graphBuildCmd` and `graphStatusCmd`.

**Step 8: Run all tests**

Run: `go test ./internal/graph/... ./internal/mcp/... ./cmd/... -v`
Expected: PASS (some test files will need updates to remove references to removed types)

**Step 9: Commit**

```bash
git add internal/graph/ internal/mcp/ cmd/graph.go
git commit -m "refactor(graph): remove git history tables, keep only CO_CHANGED edges"
```

---

### Task 8: Live LSP MCP Tools

**Files:**
- Create: `internal/mcp/lsp_tools.go`
- Modify: `internal/mcp/server.go` (add LSP manager field, register LSP tools)

**Step 1: Write the failing test**

Add to `internal/mcp/lsp_tools_test.go`:

```go
package mcp

import (
	"testing"
)

func TestLSPToolDefinitions(t *testing.T) {
	tools := []struct {
		name string
		fn   func() mcp.Tool
	}{
		{"lsp_definition", lspDefinitionTool},
		{"lsp_references", lspReferencesTool},
		{"lsp_hover", lspHoverTool},
		{"lsp_diagnostics", lspDiagnosticsTool},
	}

	for _, tt := range tools {
		tool := tt.fn()
		if tool.Name != tt.name {
			t.Errorf("expected name %s, got %s", tt.name, tool.Name)
		}
		if tool.InputSchema.Type != "object" {
			t.Errorf("%s: expected object schema", tt.name)
		}
		if _, ok := tool.InputSchema.Properties["file"]; !ok {
			t.Errorf("%s: missing 'file' parameter", tt.name)
		}
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/mcp/ -run TestLSPTool -v`
Expected: FAIL — functions not defined

**Step 3: Write the implementation**

```go
// internal/mcp/lsp_tools.go
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/lofari/golem/internal/graph/lsp"
)

// --- lsp_definition ---

func lspDefinitionTool() mcp.Tool {
	return mcp.Tool{
		Name:        "lsp_definition",
		Description: "Jump to where a symbol is defined. Provide file path, line, and column of the symbol reference.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"file":   map[string]string{"type": "string", "description": "File path (relative to project root)"},
				"line":   map[string]string{"type": "integer", "description": "Line number (1-indexed)"},
				"column": map[string]string{"type": "integer", "description": "Column number (1-indexed)"},
			},
			Required: []string{"file", "line", "column"},
		},
	}
}

func (gs *GolemServer) handleLSPDefinition(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if gs.lspManager == nil {
		return mcp.NewToolResultError("LSP not available — start golem with LSP enabled"), nil
	}

	args := req.GetArguments()
	file := getStr(args, "file")
	line := getInt(args, "line", 0)
	col := getInt(args, "column", 0)

	client := gs.clientForFile(file)
	if client == nil {
		cfg := lsp.ConfigForExt(filepath.Ext(file))
		hint := ""
		if cfg != nil {
			hint = fmt.Sprintf(" Install with: %s", cfg.InstallHint)
		}
		return mcp.NewToolResultError(fmt.Sprintf("No LSP server for %s.%s", file, hint)), nil
	}

	absPath := filepath.Join(gs.dir, file)
	locs, err := client.Definition(absPath, line-1, col-1) // convert to 0-indexed
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("definition: %v", err)), nil
	}

	if len(locs) == 0 {
		return mcp.NewToolResultText("no definition found"), nil
	}

	type defResult struct {
		File   string `json:"file"`
		Line   int    `json:"line"`
		Column int    `json:"column"`
	}

	var results []defResult
	for _, l := range locs {
		relPath := uriToRel(l.URI, gs.dir)
		results = append(results, defResult{
			File:   relPath,
			Line:   l.Line + 1,
			Column: l.Col + 1,
		})
	}

	out, _ := json.MarshalIndent(results, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

// --- lsp_references ---

func lspReferencesTool() mcp.Tool {
	return mcp.Tool{
		Name:        "lsp_references",
		Description: "Find all usages of a symbol. Returns every location where the symbol at the given position is referenced.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"file":   map[string]string{"type": "string", "description": "File path (relative to project root)"},
				"line":   map[string]string{"type": "integer", "description": "Line number (1-indexed)"},
				"column": map[string]string{"type": "integer", "description": "Column number (1-indexed)"},
			},
			Required: []string{"file", "line", "column"},
		},
	}
}

func (gs *GolemServer) handleLSPReferences(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if gs.lspManager == nil {
		return mcp.NewToolResultError("LSP not available"), nil
	}

	args := req.GetArguments()
	file := getStr(args, "file")
	line := getInt(args, "line", 0)
	col := getInt(args, "column", 0)

	client := gs.clientForFile(file)
	if client == nil {
		return mcp.NewToolResultError(fmt.Sprintf("No LSP server for %s", file)), nil
	}

	absPath := filepath.Join(gs.dir, file)
	locs, err := client.References(absPath, line-1, col-1)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("references: %v", err)), nil
	}

	if len(locs) == 0 {
		return mcp.NewToolResultText("no references found"), nil
	}

	type refResult struct {
		File   string `json:"file"`
		Line   int    `json:"line"`
		Column int    `json:"column"`
	}

	var results []refResult
	for _, l := range locs {
		results = append(results, refResult{
			File:   uriToRel(l.URI, gs.dir),
			Line:   l.Line + 1,
			Column: l.Col + 1,
		})
	}

	out, _ := json.MarshalIndent(results, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

// --- lsp_hover ---

func lspHoverTool() mcp.Tool {
	return mcp.Tool{
		Name:        "lsp_hover",
		Description: "Get type information, signature, and documentation for a symbol at a position.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"file":   map[string]string{"type": "string", "description": "File path (relative to project root)"},
				"line":   map[string]string{"type": "integer", "description": "Line number (1-indexed)"},
				"column": map[string]string{"type": "integer", "description": "Column number (1-indexed)"},
			},
			Required: []string{"file", "line", "column"},
		},
	}
}

func (gs *GolemServer) handleLSPHover(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if gs.lspManager == nil {
		return mcp.NewToolResultError("LSP not available"), nil
	}

	args := req.GetArguments()
	file := getStr(args, "file")
	line := getInt(args, "line", 0)
	col := getInt(args, "column", 0)

	client := gs.clientForFile(file)
	if client == nil {
		return mcp.NewToolResultError(fmt.Sprintf("No LSP server for %s", file)), nil
	}

	absPath := filepath.Join(gs.dir, file)
	result, err := client.Hover(absPath, line-1, col-1)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("hover: %v", err)), nil
	}

	if result == nil || result.Contents == "" {
		return mcp.NewToolResultText("no hover information"), nil
	}

	return mcp.NewToolResultText(result.Contents), nil
}

// --- lsp_diagnostics ---

func lspDiagnosticsTool() mcp.Tool {
	return mcp.Tool{
		Name:        "lsp_diagnostics",
		Description: "Get type errors, lint warnings, and other diagnostics for a file.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"file": map[string]string{"type": "string", "description": "File path (relative to project root)"},
			},
			Required: []string{"file"},
		},
	}
}

func (gs *GolemServer) handleLSPDiagnostics(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if gs.lspManager == nil {
		return mcp.NewToolResultError("LSP not available"), nil
	}

	args := req.GetArguments()
	file := getStr(args, "file")

	client := gs.clientForFile(file)
	if client == nil {
		return mcp.NewToolResultError(fmt.Sprintf("No LSP server for %s", file)), nil
	}

	absPath := filepath.Join(gs.dir, file)
	diags, err := client.Diagnostics(absPath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("diagnostics: %v", err)), nil
	}

	if len(diags) == 0 {
		return mcp.NewToolResultText("no diagnostics"), nil
	}

	out, _ := json.MarshalIndent(diags, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

// helpers

func (gs *GolemServer) clientForFile(relPath string) *lsp.Client {
	if gs.lspManager == nil {
		return nil
	}
	cfg := lsp.ConfigForExt(filepath.Ext(relPath))
	if cfg == nil {
		return nil
	}
	return gs.lspManager.ClientFor(cfg.Language)
}

func uriToRel(fileURI, baseDir string) string {
	path := strings.TrimPrefix(fileURI, "file://")
	rel, err := filepath.Rel(baseDir, path)
	if err != nil {
		return path
	}
	return rel
}
```

**Step 4: Update server.go to accept LSP Manager**

Modify `internal/mcp/server.go`:

```go
type GolemServer struct {
	mcpServer  *server.MCPServer
	dir        string
	lspManager *lsp.Manager // nil if LSP disabled
}

func NewServer(dir string, lspManager *lsp.Manager) *GolemServer {
	// ... existing ...
	gs := &GolemServer{mcpServer: s, dir: dir, lspManager: lspManager}
	// ...
}
```

Add LSP tool registrations to `registerTools()`:

```go
// LSP tools (only if manager is available)
if gs.lspManager != nil {
	gs.mcpServer.AddTool(lspDefinitionTool(), gs.handleLSPDefinition)
	gs.mcpServer.AddTool(lspReferencesTool(), gs.handleLSPReferences)
	gs.mcpServer.AddTool(lspHoverTool(), gs.handleLSPHover)
	gs.mcpServer.AddTool(lspDiagnosticsTool(), gs.handleLSPDiagnostics)
}
```

Update `ListTools()` to conditionally include LSP tools.

**Step 5: Add Diagnostics method to Client**

Add to `internal/graph/lsp/client.go`:

```go
// Diagnostics returns diagnostics for a file by opening it and collecting published diagnostics.
func (c *Client) Diagnostics(filePath string) ([]Diagnostic, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := c.ensureOpen(ctx, filePath); err != nil {
		return nil, err
	}

	// Request pull diagnostics if supported, otherwise return empty
	// Note: diagnostics are typically pushed via notifications; for pull,
	// we use textDocument/diagnostic if available
	params := protocol.DocumentDiagnosticParams{
		TextDocument: protocol.TextDocumentIdentifier{
			URI: protocol.DocumentURI(uri.File(filePath)),
		},
	}

	var result protocol.DocumentDiagnosticReport
	if err := c.call(ctx, "textDocument/diagnostic", params, &result); err != nil {
		// Server may not support pull diagnostics — return empty
		return nil, nil
	}

	var diags []Diagnostic
	// Extract from full or unchanged report
	// ... parse result based on Kind ...

	return diags, nil
}
```

**Step 6: Run tests**

Run: `go test ./internal/mcp/ -v`
Expected: PASS

**Step 7: Commit**

```bash
git add internal/mcp/lsp_tools.go internal/mcp/server.go internal/graph/lsp/client.go
git commit -m "feat(mcp): add live LSP tools — definition, references, hover, diagnostics"
```

---

### Task 9: Wire LSP into Runner (Session Lifecycle)

**Files:**
- Modify: `internal/runner/builder.go:19-35` (BuilderConfig — add LSP toggle)
- Modify: `internal/runner/builder.go` (builder loop — start/stop LSP manager)
- Modify: caller of `NewServer` to pass LSP manager

**Step 1: Add LSP config to BuilderConfig**

Add to `internal/runner/builder.go` BuilderConfig struct:
```go
LSPEnabled bool // enable LSP servers during sessions
```

**Step 2: Start LSP Manager before first iteration**

In the builder loop setup (where MCP server is created):

```go
var lspMgr *lsp.Manager
if cfg.LSPEnabled {
	lspMgr = lsp.NewManager(cfg.Dir)
	// Detect languages and start available servers
	files := collectFileExtensions(cfg.Dir)
	detected := lsp.DetectLanguages(files)
	available, missing := lsp.CheckAvailability(detected)
	for _, m := range missing {
		cfg.emit(Event{Type: "lsp_missing", Message: fmt.Sprintf("%s not found. Install: %s", m.Binary, m.InstallHint)})
	}
	if len(available) > 0 {
		if err := lspMgr.Start(available); err != nil {
			cfg.emit(Event{Type: "lsp_error", Message: err.Error()})
		}
	}
	defer lspMgr.Shutdown()
}

mcpServer := mcp.NewServer(cfg.Dir, lspMgr)
```

**Step 3: Notify LSP of changes between iterations**

After each iteration completes, before the next one:

```go
if lspMgr != nil {
	changedFiles := getChangedFilesSinceLastIteration()
	lspMgr.NotifyFilesChanged(changedFiles)
}
```

**Step 4: Add --no-lsp flag to CLI commands**

In `cmd/code.go` (and similar), add:
```go
codeCmd.Flags().Bool("no-lsp", false, "disable LSP servers during sessions")
```

Wire to `BuilderConfig.LSPEnabled = !noLSP`.

**Step 5: Run tests**

Run: `go test ./internal/runner/ ./cmd/ -v`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/runner/builder.go cmd/code.go
git commit -m "feat(runner): wire LSP manager into builder loop with session lifecycle"
```

---

### Task 10: Wire LSP into Graph Build Command

**Files:**
- Modify: `cmd/graph.go:21-66` (graphBuildCmd)

**Step 1: Update graphBuildCmd to use LSP**

```go
var graphBuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build or rebuild the code knowledge graph",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}

		if !scaffold.CtxExists(dir) {
			return fmt.Errorf(".ctx/ not found — run `golem init` first")
		}

		dbPath := filepath.Join(dir, ".ctx", "graph.db")
		store, err := graph.OpenStore(dbPath)
		if err != nil {
			return fmt.Errorf("opening graph db: %w", err)
		}
		defer store.Close()

		depth, _ := cmd.Flags().GetInt("depth")
		noLSP, _ := cmd.Flags().GetBool("no-lsp")

		// Detect and start LSP servers
		var lspMgr *lsp.Manager
		if !noLSP {
			files := collectProjectFiles(dir)
			detected := lsp.DetectLanguages(files)
			available, missing := lsp.CheckAvailability(detected)

			for _, m := range missing {
				fmt.Fprintf(os.Stderr, "golem: %s not found. Install: %s\n", m.Binary, m.InstallHint)
			}

			if len(available) > 0 {
				lspMgr = lsp.NewManager(dir)
				fmt.Fprintf(os.Stderr, "golem: starting LSP servers: %s\n", langList(available))
				if err := lspMgr.Start(available); err != nil {
					fmt.Fprintf(os.Stderr, "golem: LSP start error: %v\n", err)
				}
				defer lspMgr.Shutdown()
			}
		}

		builder := graph.NewBuilder(store, depth)
		if lspMgr != nil {
			builder.WithLSP(lspMgr)
		}

		fmt.Fprintf(os.Stderr, "golem: building code graph...\n")
		if err := builder.BuildFull(dir); err != nil {
			return fmt.Errorf("building graph: %w", err)
		}

		stats, _ := store.Stats()
		fmt.Fprintf(os.Stderr, "golem: graph built — %d nodes, %d edges\n", stats.TotalNodes, stats.TotalEdges)

		for t, count := range stats.NodeTypes {
			fmt.Fprintf(os.Stderr, "golem:   %s: %d\n", t, count)
		}

		coCount, _ := store.CoChangedCount()
		if coCount > 0 {
			fmt.Fprintf(os.Stderr, "golem: co-change pairs: %d\n", coCount)
		}

		return nil
	},
}
```

Add `--no-lsp` flag:
```go
graphBuildCmd.Flags().Bool("no-lsp", false, "disable LSP extraction (use tree-sitter only)")
```

**Step 2: Run test**

Run: `go build ./... && go test ./cmd/ -v`
Expected: PASS

**Step 3: Commit**

```bash
git add cmd/graph.go
git commit -m "feat(cmd): wire LSP into graph build command with progress output"
```

---

### Task 11: Config Support for LSP

**Files:**
- Modify: `internal/config/config.go` (add lsp field)

**Step 1: Add lsp field to config**

Add to the config struct:
```go
LSP bool `yaml:"lsp"` // default: true
```

Ensure default is `true`.

**Step 2: Wire config to builder and graph commands**

Ensure `BuilderConfig.LSPEnabled` reads from config, overridden by `--no-lsp` flag.

**Step 3: Run tests**

Run: `go test ./internal/config/ -v`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/config/
git commit -m "feat(config): add lsp config option (default true)"
```

---

### Task 12: Integration Test — Full LSP Graph Build

**Files:**
- Create: `internal/graph/lsp/integration_test.go`

**Step 1: Write integration test**

```go
// internal/graph/lsp/integration_test.go
package lsp

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/lofari/golem/internal/graph"
)

func TestIntegrationBuildWithLSP(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not installed")
	}

	dir := t.TempDir()
	writeTestFile(t, dir, "go.mod", "module testmod\n\ngo 1.21\n")
	writeTestFile(t, dir, "main.go", `package main

import "fmt"

func main() {
	msg := greet("world")
	fmt.Println(msg)
}

func greet(name string) string {
	return "Hello, " + name
}

type Server struct {
	Port int
}

func (s *Server) Start() error {
	return nil
}
`)

	// Init git repo for CO_CHANGED
	exec.Command("git", "init", dir).Run()
	exec.Command("git", "-C", dir, "add", ".").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "init").Run()

	dbPath := filepath.Join(dir, "graph.db")
	store, err := graph.OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Start LSP
	mgr := NewManager(dir)
	configs := []ServerConfig{
		{Language: "go", Binary: "gopls", Args: []string{"serve"}},
	}
	if err := mgr.Start(configs); err != nil {
		t.Fatal(err)
	}
	defer mgr.Shutdown()

	builder := graph.NewBuilder(store)
	builder.WithLSP(mgr)

	if err := builder.BuildFull(dir); err != nil {
		t.Fatalf("build: %v", err)
	}

	stats, _ := store.Stats()
	if stats.TotalNodes < 4 {
		t.Errorf("expected at least 4 nodes (file, main, greet, Server), got %d", stats.TotalNodes)
	}

	// Check for resolved CALLS edges (not call: prefix)
	edges, _ := store.EdgesFrom("fn:main.go:main")
	hasResolvedCall := false
	for _, e := range edges {
		if e.Type == "CALLS" && len(e.To) > 3 && e.To[:3] == "fn:" {
			hasResolvedCall = true
		}
	}
	if !hasResolvedCall {
		t.Error("expected resolved CALLS edge from main to greet")
	}
}
```

**Step 2: Run test**

Run: `go test ./internal/graph/lsp/ -run TestIntegration -v -timeout 120s`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/graph/lsp/integration_test.go
git commit -m "test(lsp): add integration test for full LSP graph build"
```

---

### Task 13: Final Cleanup and Verification

**Step 1: Run full test suite**

Run: `go test ./... -timeout 120s`
Expected: PASS

**Step 2: Build and verify**

Run: `go build ./...`
Expected: clean build

**Step 3: Manual smoke test**

Run: `go run . graph build` in a Go project with gopls installed
Expected: Graph builds with LSP output, shows install hints for missing servers

**Step 4: Commit any remaining fixes**

```bash
git add -A
git commit -m "chore: final cleanup for LSP integration"
```
