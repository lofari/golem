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
	MaxIterations  int      `yaml:"max-iterations"`
	MaxTurns       int      `yaml:"max-turns"`
	Verbose        bool     `yaml:"verbose"`
	Sandbox        bool     `yaml:"sandbox"`
	SandboxTools   []string `yaml:"sandbox-tools"`
	SandboxTimeout string   `yaml:"sandbox-timeout"`
	SandboxMemory  string   `yaml:"sandbox-memory"`
	MCP            bool     `yaml:"mcp"`
	Parallel       int      `yaml:"parallel"`
	PluginDir      []string `yaml:"plugin-dir"`
	Model          string   `yaml:"model"`
}

// Defaults returns a Config with built-in default values.
func Defaults() Config {
	return Config{
		MaxIterations: 20,
		MaxTurns:      200,
		MCP:           true,
		Parallel:      1,
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
	MaxTurns       *int     `yaml:"max-turns"`
	Verbose        *bool    `yaml:"verbose"`
	Sandbox        *bool    `yaml:"sandbox"`
	SandboxTools   []string `yaml:"sandbox-tools"`
	SandboxTimeout *string  `yaml:"sandbox-timeout"`
	SandboxMemory  *string  `yaml:"sandbox-memory"`
	MCP            *bool    `yaml:"mcp"`
	Parallel       *int     `yaml:"parallel"`
	PluginDir      []string `yaml:"plugin-dir"`
	Model          *string  `yaml:"model"`
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
	if layer.MaxTurns != nil {
		base.MaxTurns = *layer.MaxTurns
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
	case "max-turns":
		return strconv.Itoa(cfg.MaxTurns), nil
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
	default:
		return "", fmt.Errorf("unknown config key: %q", key)
	}
}

// PrintConfig writes all config values to w.
func PrintConfig(w io.Writer, cfg Config) {
	fmt.Fprintf(w, "max-iterations: %d\n", cfg.MaxIterations)
	fmt.Fprintf(w, "max-turns: %d\n", cfg.MaxTurns)
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
