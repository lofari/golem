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
