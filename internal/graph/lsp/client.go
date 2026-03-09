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
	Message  string `json:"message"`
	Severity int    `json:"severity"` // 1=Error, 2=Warning, 3=Info, 4=Hint
	Line     int    `json:"line"`     // 0-indexed
	Col      int    `json:"col"`      // 0-indexed
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
	diagMu     sync.RWMutex
	diags      map[string][]Diagnostic // uri -> diagnostics
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
	conn := jsonrpc2.NewConn(stream)

	absRoot, _ := filepath.Abs(projectRoot)
	rootURI := uri.File(absRoot)

	c := &Client{
		cfg:       cfg,
		cmd:       cmd,
		conn:      conn,
		rootURI:   protocol.DocumentURI(rootURI),
		openFiles: make(map[string]int),
		diags:     make(map[string][]Diagnostic),
	}

	ctx := context.Background()

	// Start connection handler with our notification handler
	conn.Go(ctx, c.handleNotification)

	if err := c.initialize(ctx); err != nil {
		c.kill()
		return nil, fmt.Errorf("initialize: %w", err)
	}

	return c, nil
}

// handleNotification processes incoming notifications from the LSP server.
func (c *Client) handleNotification(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	switch req.Method() {
	case "textDocument/publishDiagnostics":
		var params protocol.PublishDiagnosticsParams
		if err := json.Unmarshal(req.Params(), &params); err == nil {
			var diags []Diagnostic
			for _, d := range params.Diagnostics {
				diags = append(diags, Diagnostic{
					Message:  d.Message,
					Severity: int(d.Severity),
					Line:     int(d.Range.Start.Line),
					Col:      int(d.Range.Start.Character),
				})
			}
			c.diagMu.Lock()
			c.diags[string(params.URI)] = diags
			c.diagMu.Unlock()
		}
		return reply(ctx, nil, nil)
	}
	// For requests (with ID), send empty response
	return reply(ctx, nil, nil)
}

func (c *Client) initialize(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	pid := int32(os.Getpid())
	params := protocol.InitializeParams{
		ProcessID: pid,
		RootURI:   c.rootURI,
		Capabilities: protocol.ClientCapabilities{
			TextDocument: &protocol.TextDocumentClientCapabilities{
				DocumentSymbol: &protocol.DocumentSymbolClientCapabilities{},
				Definition:     &protocol.DefinitionTextDocumentClientCapabilities{},
				References:     &protocol.ReferencesTextDocumentClientCapabilities{},
				Hover:          &protocol.HoverTextDocumentClientCapabilities{},
			},
		},
	}

	var result protocol.InitializeResult
	if _, err := c.conn.Call(ctx, "initialize", params, &result); err != nil {
		return err
	}

	if result.ServerInfo != nil {
		c.serverName = result.ServerInfo.Name
	}

	// Send initialized notification
	return c.conn.Notify(ctx, "initialized", &protocol.InitializedParams{})
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
	if _, err := c.conn.Call(ctx, "textDocument/documentSymbol", params, &raw); err != nil {
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
	if _, err := c.conn.Call(ctx, "textDocument/definition", params, &raw); err != nil {
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
	if _, err := c.conn.Call(ctx, "textDocument/references", params, &raw); err != nil {
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
	if _, err := c.conn.Call(ctx, "textDocument/hover", params, &result); err != nil {
		return nil, err
	}

	return &HoverResult{Contents: result.Contents.Value}, nil
}

// Diagnostics returns the latest published diagnostics for a file.
func (c *Client) Diagnostics(filePath string) ([]Diagnostic, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := c.ensureOpen(ctx, filePath); err != nil {
		return nil, err
	}

	// Give the server a moment to publish diagnostics after open
	time.Sleep(500 * time.Millisecond)

	fileURI := string(uri.File(filePath))

	c.diagMu.RLock()
	diags := c.diags[fileURI]
	c.diagMu.RUnlock()

	return diags, nil
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

	return c.conn.Notify(ctx, "textDocument/didSave", params)
}

// Shutdown gracefully stops the LSP server.
func (c *Client) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Close all open documents
	c.mu.Lock()
	for fileURI := range c.openFiles {
		c.conn.Notify(ctx, "textDocument/didClose", protocol.DidCloseTextDocumentParams{
			TextDocument: protocol.TextDocumentIdentifier{
				URI: protocol.DocumentURI(fileURI),
			},
		})
	}
	c.openFiles = make(map[string]int)
	c.mu.Unlock()

	var nothing json.RawMessage
	c.conn.Call(ctx, "shutdown", nil, &nothing)
	c.conn.Notify(ctx, "exit", nil)
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
	return c.conn.Notify(ctx, "textDocument/didOpen", params)
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
