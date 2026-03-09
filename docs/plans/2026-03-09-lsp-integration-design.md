# LSP Integration for Knowledge Graph

## Overview

Replace tree-sitter-based code extraction with LSP servers for precise structural edges in the knowledge graph. LSP serves two roles: build-time edge extraction (replacing tree-sitter) and live session-time tools for the agent. Tree-sitter remains as a fallback when LSP servers are unavailable.

Additionally, simplify the git history graph by removing redundant data that git itself already stores.

## Language Registry & Detection

New package `internal/graph/lsp/` with a registry mapping languages to LSP server configs.

```go
type ServerConfig struct {
    Language    string   // "go", "python", "typescript", etc.
    Binary      string   // "gopls", "pyright", "dart", etc.
    Args        []string // startup args
    InstallHint string   // "go install golang.org/x/tools/gopls@latest"
    Extensions  []string // [".go"], [".py"], [".ts", ".tsx"]
}
```

Supported languages:

| Language | Binary | Install Hint |
|----------|--------|-------------|
| Go | `gopls` | `go install golang.org/x/tools/gopls@latest` |
| Python | `pyright` | `npm install -g pyright` |
| TypeScript/JS | `typescript-language-server` | `npm install -g typescript-language-server typescript` |
| Rust | `rust-analyzer` | `rustup component add rust-analyzer` |
| Java | `jdtls` | `brew install jdtls` / manual install |
| Kotlin | `kotlin-language-server` | `brew install kotlin-language-server` / manual install |
| Dart | `dart` | Ships with Dart SDK (`dart language-server`) |

Detection flow at build time:

1. Walk project files, collect unique extensions.
2. Map extensions to languages via registry.
3. Check if binary exists in `$PATH` via `exec.LookPath`.
4. If missing, log: `"pyright not found. Install with: npm install -g pyright"`.
5. Return list of available ServerConfigs.

## LSP Client & Server Lifecycle

```go
// internal/graph/lsp/client.go
type Client struct {
    lang         string
    cmd          *exec.Cmd
    conn         jsonrpc2.Conn  // JSON-RPC 2.0 over stdin/stdout
    capabilities ServerCapabilities
}

func Start(cfg ServerConfig, projectRoot string) (*Client, error)
func (c *Client) Initialize() error
func (c *Client) Shutdown() error
```

Shared manager for both build-time and session-time use:

```go
// internal/graph/lsp/manager.go
type Manager struct {
    clients map[string]*Client
    root    string
}

func NewManager(root string) *Manager
func (m *Manager) Start(langs []ServerConfig) error   // start all in parallel
func (m *Manager) ClientFor(lang string) *Client
func (m *Manager) Shutdown() error
```

- Build time: Builder creates Manager, starts servers, extracts edges, calls Shutdown().
- Session time: Runner creates Manager at session start, MCP tools query through it, Shutdown() at session end.
- Protocol: JSON-RPC 2.0 over stdin/stdout using `go.lsp.dev/jsonrpc2` or `go.lsp.dev/protocol`.
- File sync: Send `textDocument/didSave` when files change between iterations to keep LSP state current.

## Build-Time Edge Extraction

Hybrid approach: LSP for symbols and definition resolution, tree-sitter for call site detection.

Per-file extraction flow:

1. `textDocument/documentSymbol` — all symbols in file (functions, methods, classes, types). Produces nodes + DEFINES edges.
2. Tree-sitter parse — find call expressions syntactically (cheap, no LSP request needed).
3. `textDocument/definition` — for each call site from step 2, resolve the exact target. Produces precise CALLS edges with fully qualified target IDs.
4. Imports — tree-sitter handles these (syntax-level, no LSP needed).

Fallback chain in Builder:

```
for each file:
    lang = detectLanguage(ext)
    if manager.ClientFor(lang) != nil:
        nodes, edges = lsp.Extract(client, file, treeForCallSites)
    else if treesitter.Supported(lang):
        nodes, edges = treesitter.Extract(file, src)
    else:
        nodes, edges = ExtractFileOnly(file)
```

Files grouped by language, each group processed concurrently using its LSP client. Within a group, requests are sequential (LSP servers are typically single-threaded).

Accuracy gains:

- `CALLS fn:main.go:Run -> call:Open` becomes `CALLS fn:main.go:Run -> fn:store.go:Open` (resolved to exact target).
- Cross-package calls resolve correctly.
- Method calls on interfaces resolve to the interface method.

## Live LSP MCP Tools

MCP tools available to the agent during sessions:

| Tool | LSP Request | Purpose |
|------|------------|---------|
| `lsp_definition` | `textDocument/definition` | Jump to where a symbol is defined |
| `lsp_references` | `textDocument/references` | Find all usages of a symbol |
| `lsp_hover` | `textDocument/hover` | Get type info, signature, docs |
| `lsp_diagnostics` | `textDocument/publishDiagnostics` | Type errors, lint warnings for a file |

Tool inputs: file path + line + column (or symbol name resolved to position via graph).

Graceful degradation: if no LSP server available for a language, return error with install suggestion. Agent falls back to graph tools and grep.

## Git History Simplification

Remove duplicated git data, keep only what git cannot provide natively.

Remove:

- `commits` table
- `authors` table
- `MODIFIES` edges (commit -> file)
- `AUTHORED_BY` edges (commit -> author)
- `HistoryBuilder` (most of history.go)
- MCP tools: `find_recent_changes`, `find_file_history`

Keep:

- `CO_CHANGED` edges — pre-computed during build by running `git log --name-only`, grouping files by commit, counting co-occurrences >= 3. Stored as edges with weight.
- `find_co_changed` MCP tool.
- Co-change boost in context engine.

Change:

- Recency boost in context engine: switch from querying commits table to running `git log --format=%H --since=7d -- <path>` on-the-fly during context map building.
- Schema migration: drop commits/authors tables on rebuild.

## Integration Points

`golem graph build` (modified):

1. Detect languages from file extensions (registry).
2. Check which LSP binaries are available.
3. Log suggestions for missing servers.
4. Start available LSP servers in parallel (Manager).
5. Wait for initialization, emit progress events.
6. Walk files — extract via LSP/tree-sitter/file-only fallback chain.
7. Compute CO_CHANGED from git log (no commits/authors stored).
8. Shutdown LSP servers.
9. Build embeddings (existing).

`golem code` / `golem qa` (modified):

1. Before first iteration: start LSP Manager for detected languages.
2. MCP server registers both graph tools and LSP tools.
3. Between iterations: notify LSP servers of changed files (didSave).
4. On session end: shutdown Manager.

`golem review` / `golem plan`:

- Start LSP servers for live tools. Startup cost worth it for precise references/diagnostics.

Config:

```yaml
lsp: true  # enable/disable LSP (default: true)
```

Flag: `--no-lsp` disables both build-time extraction and session-time tools.

No changes to: store schema (nodes/edges tables), query package, embedding pipeline, context engine (except recency boost source), existing graph MCP tools.
