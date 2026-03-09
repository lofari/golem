package lsp

import (
	"fmt"
	"os"
	"path/filepath"
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
