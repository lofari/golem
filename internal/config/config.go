package config

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config holds all golem configuration values.
type Config struct {
	MaxIterations  int      `yaml:"max-iterations" json:"max-iterations"`
	MaxToolCalls   int      `yaml:"max-tool-calls" json:"max-tool-calls"`
	Verbose        bool     `yaml:"verbose" json:"verbose"`
	Sandbox        bool     `yaml:"sandbox" json:"sandbox"`
	SandboxTools   []string `yaml:"sandbox-tools" json:"sandbox-tools,omitempty"`
	SandboxTimeout string   `yaml:"sandbox-timeout" json:"sandbox-timeout,omitempty"`
	SandboxMemory  string   `yaml:"sandbox-memory" json:"sandbox-memory,omitempty"`
	MCP            bool     `yaml:"mcp" json:"mcp"`
	Parallel       int      `yaml:"parallel" json:"parallel"`
	PluginDir      []string `yaml:"plugin-dir" json:"plugin-dir,omitempty"`
	Model            string   `yaml:"model" json:"model"`
	ExecutionHistory int      `yaml:"execution-history" json:"execution-history"`
	ContextMap       bool     `yaml:"context-map" json:"context-map"`
	ContextMapLimit  int      `yaml:"context-map-limit" json:"context-map-limit"`
	LSP              bool     `yaml:"lsp" json:"lsp"`
	Engine           string   `yaml:"engine" json:"engine"`
	DSLCommand       string   `yaml:"dsl-command" json:"dsl-command"`
}

// Defaults returns a Config with built-in default values.
func Defaults() Config {
	return Config{
		MaxIterations: 20,
		MaxToolCalls:      200,
		MCP:              true,
		Parallel:         1,
		ExecutionHistory: 5,
		ContextMap:       true,
		ContextMapLimit:  15,
		LSP:             true,
		Engine:          "go",
		DSLCommand:      "golem-dsl",
	}
}

// GlobalPath returns the default global config file path.
func GlobalPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "golem", "config.yaml")
}

// ProjectPath returns the project config file path for the given directory.
func ProjectPath(dir string) string {
	return filepath.Join(dir, ".ctx", "config.yaml")
}

// Load reads config from global and project files, merging with defaults.
// Resolution order: defaults < global < project.
// Empty paths are skipped.
func Load(globalPath, projectPath string) Config {
	cfg := Defaults()

	if globalPath != "" {
		if layer, err := readFile(globalPath); err == nil {
			cfg = merge(cfg, layer)
		}
	}

	if projectPath != "" {
		if layer, err := readFile(projectPath); err == nil {
			cfg = merge(cfg, layer)
		}
	}

	return cfg
}

// configLayer is used for partial YAML parsing where zero values mean "not set".
type configLayer struct {
	MaxIterations  *int     `yaml:"max-iterations"`
	MaxToolCalls       *int     `yaml:"max-tool-calls"`
	Verbose        *bool    `yaml:"verbose"`
	Sandbox        *bool    `yaml:"sandbox"`
	SandboxTools   []string `yaml:"sandbox-tools"`
	SandboxTimeout *string  `yaml:"sandbox-timeout"`
	SandboxMemory  *string  `yaml:"sandbox-memory"`
	MCP            *bool    `yaml:"mcp"`
	Parallel       *int     `yaml:"parallel"`
	PluginDir      []string `yaml:"plugin-dir"`
	Model            *string `yaml:"model"`
	ExecutionHistory *int    `yaml:"execution-history"`
	ContextMap       *bool   `yaml:"context-map"`
	ContextMapLimit  *int    `yaml:"context-map-limit"`
	LSP              *bool   `yaml:"lsp"`
	Engine           *string `yaml:"engine"`
	DSLCommand       *string `yaml:"dsl-command"`
}

func readFile(path string) (configLayer, error) {
	var layer configLayer
	data, err := os.ReadFile(path)
	if err != nil {
		return layer, err
	}
	err = yaml.Unmarshal(data, &layer)
	return layer, err
}

func merge(base Config, layer configLayer) Config {
	if layer.MaxIterations != nil {
		base.MaxIterations = *layer.MaxIterations
	}
	if layer.MaxToolCalls != nil {
		base.MaxToolCalls = *layer.MaxToolCalls
	}
	if layer.Verbose != nil {
		base.Verbose = *layer.Verbose
	}
	if layer.Sandbox != nil {
		base.Sandbox = *layer.Sandbox
	}
	if layer.SandboxTools != nil {
		base.SandboxTools = layer.SandboxTools
	}
	if layer.SandboxTimeout != nil {
		base.SandboxTimeout = *layer.SandboxTimeout
	}
	if layer.SandboxMemory != nil {
		base.SandboxMemory = *layer.SandboxMemory
	}
	if layer.MCP != nil {
		base.MCP = *layer.MCP
	}
	if layer.Parallel != nil {
		base.Parallel = *layer.Parallel
	}
	if layer.PluginDir != nil {
		base.PluginDir = layer.PluginDir
	}
	if layer.Model != nil {
		base.Model = *layer.Model
	}
	if layer.ExecutionHistory != nil {
		base.ExecutionHistory = *layer.ExecutionHistory
	}
	if layer.ContextMap != nil {
		base.ContextMap = *layer.ContextMap
	}
	if layer.ContextMapLimit != nil {
		base.ContextMapLimit = *layer.ContextMapLimit
	}
	if layer.LSP != nil {
		base.LSP = *layer.LSP
	}
	if layer.Engine != nil {
		base.Engine = *layer.Engine
	}
	if layer.DSLCommand != nil {
		base.DSLCommand = *layer.DSLCommand
	}
	return base
}

// SetValue reads an existing config file (or starts empty), sets one key, and writes back.
func SetValue(path, key, value string) error {
	existing := make(map[string]interface{})
	if data, err := os.ReadFile(path); err == nil {
		yaml.Unmarshal(data, &existing)
	}

	existing[key] = parseValue(value)

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(existing)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func parseValue(s string) interface{} {
	if b, err := strconv.ParseBool(s); err == nil {
		return b
	}
	if i, err := strconv.Atoi(s); err == nil {
		return i
	}
	if strings.Contains(s, ",") {
		return strings.Split(s, ",")
	}
	return s
}

// GetValue returns a string representation of a config field by key name.
func GetValue(cfg Config, key string) (string, error) {
	switch key {
	case "max-iterations":
		return strconv.Itoa(cfg.MaxIterations), nil
	case "max-tool-calls":
		return strconv.Itoa(cfg.MaxToolCalls), nil
	case "verbose":
		return strconv.FormatBool(cfg.Verbose), nil
	case "sandbox":
		return strconv.FormatBool(cfg.Sandbox), nil
	case "sandbox-tools":
		return strings.Join(cfg.SandboxTools, ","), nil
	case "sandbox-timeout":
		return cfg.SandboxTimeout, nil
	case "sandbox-memory":
		return cfg.SandboxMemory, nil
	case "mcp":
		return strconv.FormatBool(cfg.MCP), nil
	case "parallel":
		return strconv.Itoa(cfg.Parallel), nil
	case "plugin-dir":
		return strings.Join(cfg.PluginDir, ","), nil
	case "model":
		return cfg.Model, nil
	case "execution-history":
		return strconv.Itoa(cfg.ExecutionHistory), nil
	case "context-map":
		return strconv.FormatBool(cfg.ContextMap), nil
	case "context-map-limit":
		return strconv.Itoa(cfg.ContextMapLimit), nil
	case "lsp":
		return strconv.FormatBool(cfg.LSP), nil
	case "engine":
		return cfg.Engine, nil
	case "dsl-command":
		return cfg.DSLCommand, nil
	default:
		return "", fmt.Errorf("unknown config key: %q", key)
	}
}

// PrintConfig writes all config values to w.
func PrintConfig(w io.Writer, cfg Config) {
	fmt.Fprintf(w, "max-iterations: %d\n", cfg.MaxIterations)
	fmt.Fprintf(w, "max-turns: %d\n", cfg.MaxToolCalls)
	fmt.Fprintf(w, "verbose: %v\n", cfg.Verbose)
	fmt.Fprintf(w, "sandbox: %v\n", cfg.Sandbox)
	if len(cfg.SandboxTools) > 0 {
		fmt.Fprintf(w, "sandbox-tools: %s\n", strings.Join(cfg.SandboxTools, ","))
	}
	if cfg.SandboxTimeout != "" {
		fmt.Fprintf(w, "sandbox-timeout: %s\n", cfg.SandboxTimeout)
	}
	if cfg.SandboxMemory != "" {
		fmt.Fprintf(w, "sandbox-memory: %s\n", cfg.SandboxMemory)
	}
	fmt.Fprintf(w, "mcp: %v\n", cfg.MCP)
	fmt.Fprintf(w, "parallel: %d\n", cfg.Parallel)
	if len(cfg.PluginDir) > 0 {
		for _, d := range cfg.PluginDir {
			fmt.Fprintf(w, "plugin-dir: %s\n", d)
		}
	}
	if cfg.Model != "" {
		fmt.Fprintf(w, "model: %s\n", cfg.Model)
	}
	fmt.Fprintf(w, "execution-history: %d\n", cfg.ExecutionHistory)
	fmt.Fprintf(w, "context-map: %v\n", cfg.ContextMap)
	fmt.Fprintf(w, "context-map-limit: %d\n", cfg.ContextMapLimit)
	fmt.Fprintf(w, "lsp: %v\n", cfg.LSP)
	fmt.Fprintf(w, "engine: %s\n", cfg.Engine)
	if cfg.DSLCommand != "golem-dsl" {
		fmt.Fprintf(w, "dsl-command: %s\n", cfg.DSLCommand)
	}
}

// KeyInfo describes a config key for the interactive wizard.
type KeyInfo struct {
	Key         string
	Description string
}

// Keys returns all config keys with descriptions, in display order.
func Keys() []KeyInfo {
	return []KeyInfo{
		{"max-iterations", "maximum number of builder iterations"},
		{"max-tool-calls", "max tool calls per Claude Code session"},
		{"verbose", "extra output detail (true/false)"},
		{"model", "Claude model (sonnet, opus, haiku)"},
		{"sandbox", "run Claude in warden sandbox (true/false)"},
		{"sandbox-tools", "additional sandbox tools (comma-separated)"},
		{"sandbox-timeout", "sandbox timeout (e.g., 2h, 30m)"},
		{"sandbox-memory", "sandbox memory limit (e.g., 8g)"},
		{"mcp", "enable golem MCP server (true/false)"},
		{"parallel", "max parallel task sessions (1 = sequential)"},
		{"plugin-dir", "plugin directories (comma-separated)"},
		{"execution-history", "number of execution sessions to retain (default 5)"},
		{"context-map", "enable context map injection (true/false)"},
		{"context-map-limit", "max symbols in context map (default 15)"},
		{"lsp", "enable LSP servers for code intelligence (true/false)"},
		{"engine", "orchestration engine: go or dsl (default: go)"},
		{"dsl-command", "path to golem-dsl binary (default: golem-dsl)"},
	}
}

// WriteFile writes a Config to a YAML file, creating parent directories as needed.
func WriteFile(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
