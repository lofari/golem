# UX Simplification Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Simplify golem's UX by adding a config system, removing TUI, renaming `run` to `code`, and adding a `qa` agent mode.

**Architecture:** A new `internal/config` package handles two-layer config resolution (global + project). TUI removal deletes `internal/tui/` and its Bubble Tea dependency. Commands are refactored: `run` becomes `code` (with alias), `review`/`qa` share common flag setup. A `golem config` subcommand manages settings.

**Tech Stack:** Go, Cobra CLI, YAML (gopkg.in/yaml.v3)

---

### Task 1: Create config package — types and loading

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Step 1: Write the failing test**

```go
// internal/config/config_test.go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	cfg := Load("", "")
	if cfg.MaxIterations != 20 {
		t.Errorf("expected MaxIterations=20, got %d", cfg.MaxIterations)
	}
	if cfg.MaxTurns != 200 {
		t.Errorf("expected MaxTurns=200, got %d", cfg.MaxTurns)
	}
	if cfg.MCP != true {
		t.Error("expected MCP=true")
	}
	if cfg.Parallel != 1 {
		t.Errorf("expected Parallel=1, got %d", cfg.Parallel)
	}
}

func TestLoad_GlobalOverrides(t *testing.T) {
	globalDir := t.TempDir()
	globalFile := filepath.Join(globalDir, "config.yaml")
	os.WriteFile(globalFile, []byte("verbose: true\nmax-turns: 300\nsandbox: true\n"), 0644)

	cfg := Load(globalFile, "")
	if !cfg.Verbose {
		t.Error("expected Verbose=true from global config")
	}
	if cfg.MaxTurns != 300 {
		t.Errorf("expected MaxTurns=300, got %d", cfg.MaxTurns)
	}
	if !cfg.Sandbox {
		t.Error("expected Sandbox=true from global config")
	}
	// Unset values should remain default
	if cfg.MaxIterations != 20 {
		t.Errorf("expected MaxIterations=20 (default), got %d", cfg.MaxIterations)
	}
}

func TestLoad_ProjectOverridesGlobal(t *testing.T) {
	globalDir := t.TempDir()
	globalFile := filepath.Join(globalDir, "config.yaml")
	os.WriteFile(globalFile, []byte("verbose: true\nmax-iterations: 30\n"), 0644)

	projectDir := t.TempDir()
	projectFile := filepath.Join(projectDir, "config.yaml")
	os.WriteFile(projectFile, []byte("max-iterations: 10\n"), 0644)

	cfg := Load(globalFile, projectFile)
	if cfg.MaxIterations != 10 {
		t.Errorf("expected MaxIterations=10 (project override), got %d", cfg.MaxIterations)
	}
	// Global value should still be present for unoverridden fields
	if !cfg.Verbose {
		t.Error("expected Verbose=true from global config")
	}
}

func TestLoad_PluginDirMerge(t *testing.T) {
	globalDir := t.TempDir()
	globalFile := filepath.Join(globalDir, "config.yaml")
	os.WriteFile(globalFile, []byte("plugin-dir:\n  - /global/plugin\n"), 0644)

	projectDir := t.TempDir()
	projectFile := filepath.Join(projectDir, "config.yaml")
	os.WriteFile(projectFile, []byte("plugin-dir:\n  - /project/plugin\n"), 0644)

	cfg := Load(globalFile, projectFile)
	// Project replaces global for slices
	if len(cfg.PluginDir) != 1 || cfg.PluginDir[0] != "/project/plugin" {
		t.Errorf("expected project plugin-dir to override, got %v", cfg.PluginDir)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /home/winler/projects/golem && go test ./internal/config/ -v`
Expected: FAIL (package does not exist)

**Step 3: Write minimal implementation**

```go
// internal/config/config.go
package config

import (
	"os"
	"path/filepath"

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
```

**Step 4: Run test to verify it passes**

Run: `cd /home/winler/projects/golem && go test ./internal/config/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): add two-layer config system with defaults, global, and project"
```

---

### Task 2: Add `golem config` command

**Files:**
- Create: `cmd/config.go`

**Step 1: Write the config command**

```go
// cmd/config.go
package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/lofari/golem/internal/config"
	"github.com/lofari/golem/internal/scaffold"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage golem configuration",
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key, value := args[0], args[1]
		global, _ := cmd.Flags().GetBool("global")

		var path string
		if global {
			path = config.GlobalPath()
		} else {
			dir, err := os.Getwd()
			if err != nil {
				return err
			}
			if !scaffold.CtxExists(dir) {
				return fmt.Errorf(".ctx/ not found — run `golem init` first, or use --global")
			}
			path = config.ProjectPath(dir)
		}

		return config.SetValue(path, key, value)
	},
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a resolved configuration value",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]

		dir, _ := os.Getwd()
		globalPath := config.GlobalPath()
		projectPath := ""
		if scaffold.CtxExists(dir) {
			projectPath = config.ProjectPath(dir)
		}
		cfg := config.Load(globalPath, projectPath)

		val, err := config.GetValue(cfg, key)
		if err != nil {
			return err
		}
		fmt.Println(val)
		return nil
	},
}

var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all resolved configuration values",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, _ := os.Getwd()
		globalPath := config.GlobalPath()
		projectPath := ""
		if scaffold.CtxExists(dir) {
			projectPath = config.ProjectPath(dir)
		}
		cfg := config.Load(globalPath, projectPath)
		config.PrintConfig(os.Stdout, cfg)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configListCmd)

	configSetCmd.Flags().Bool("global", false, "set in global config (~/.config/golem/config.yaml)")
}
```

**Step 2: Add SetValue, GetValue, and PrintConfig to config package**

Add to `internal/config/config.go`:

```go
// SetValue reads an existing config file (or starts empty), sets one key, and writes back.
func SetValue(path, key, value string) error {
	// Read existing file as raw map to preserve other values
	existing := make(map[string]interface{})
	if data, err := os.ReadFile(path); err == nil {
		yaml.Unmarshal(data, &existing)
	}

	// Parse value to appropriate type
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
	// Check for comma-separated list
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
```

Note: add `"fmt"`, `"io"`, `"strconv"`, `"strings"` to config.go imports.

**Step 3: Run build to verify compilation**

Run: `cd /home/winler/projects/golem && go build ./...`
Expected: PASS

**Step 4: Commit**

```bash
git add cmd/config.go internal/config/config.go
git commit -m "feat(config): add golem config set/get/list commands"
```

---

### Task 3: Wire config into `cmd/run.go` (before rename)

**Files:**
- Modify: `cmd/run.go`

**Step 1: Update run command to load config and use it as defaults**

The key change: load config first, then let flags override only when explicitly set. Cobra's `cmd.Flags().Changed("flag-name")` detects if a flag was explicitly passed.

Modify `cmd/run.go`:
- Add import for `"github.com/lofari/golem/internal/config"`
- In the `RunE` function, after getting `dir`, load config:

```go
globalPath := config.GlobalPath()
projectPath := config.ProjectPath(dir)
cfg := config.Load(globalPath, projectPath)
```

- For each flag, only use the flag value if it was explicitly set:

```go
maxIter := cfg.MaxIterations
if cmd.Flags().Changed("max-iterations") {
    maxIter, _ = cmd.Flags().GetInt("max-iterations")
}
maxTurns := cfg.MaxTurns
if cmd.Flags().Changed("max-turns") {
    maxTurns, _ = cmd.Flags().GetInt("max-turns")
}
// ... same pattern for all flags
```

- Update the default values in flag registration to match new defaults:

```go
runCmd.Flags().Int("max-turns", 200, "max turns per Claude Code session")
```

**Step 2: Do the same for `cmd/review.go`**

Same pattern: load config, use config values as defaults, let flags override.

**Step 3: Do the same for `cmd/plan.go`**

Only `model` and `plugin-dir` apply here.

**Step 4: Run tests**

Run: `cd /home/winler/projects/golem && go test ./... && go build ./...`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/run.go cmd/review.go cmd/plan.go
git commit -m "feat(config): wire config loading into run, review, and plan commands"
```

---

### Task 4: Remove TUI

**Files:**
- Delete: `internal/tui/run.go`
- Delete: `internal/tui/status.go`
- Delete: `internal/tui/components.go`
- Delete: `internal/tui/components_test.go`
- Delete: `internal/tui/writer.go`
- Delete: `internal/tui/writer_test.go`
- Delete: `internal/tui/styles.go`
- Modify: `cmd/run.go` — remove TUI branch, remove `--no-tui` flag
- Modify: `cmd/status.go` — remove TUI branch, remove `--no-tui` flag, add `--watch` flag
- Modify: `go.mod` — remove Bubble Tea dependencies

**Step 1: Simplify `cmd/run.go`**

Remove:
- Import of `tea "github.com/charmbracelet/bubbletea"`, `"golang.org/x/term"`, `"github.com/lofari/golem/internal/tui"`
- The `useTUI` check and `runWithTUI` branch
- The entire `runWithTUI` function
- The `--no-tui` flag registration

The `RunE` should just call `runWithoutTUI` directly (rename to inline or keep as helper). The `runWithoutTUI` function stays as-is but gets renamed or inlined.

**Step 2: Simplify `cmd/status.go`**

Remove:
- Import of `tea`, `"golang.org/x/term"`, `"github.com/lofari/golem/internal/tui"`
- The TUI branch

Add `--watch` flag:

```go
watch, _ := cmd.Flags().GetBool("watch")
if watch {
    return watchStatus(dir)
}
```

Implement `watchStatus`:

```go
func watchStatus(dir string) error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Print once immediately
	printStatusOnce(dir)

	for {
		select {
		case <-sigCh:
			return nil
		case <-ticker.C:
			// Clear screen and reprint
			fmt.Print("\033[H\033[2J")
			printStatusOnce(dir)
		}
	}
}

func printStatusOnce(dir string) {
	state, err := ctx.ReadState(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading state: %v\n", err)
		return
	}
	log, err := ctx.ReadLog(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading log: %v\n", err)
		return
	}
	display.PrintStatus(os.Stdout, state, len(log.Sessions))
}
```

**Step 3: Delete TUI directory**

```bash
rm -rf internal/tui/
```

**Step 4: Remove Bubble Tea dependencies from go.mod**

```bash
cd /home/winler/projects/golem && go mod tidy
```

This should remove `charmbracelet/bubbles`, `charmbracelet/bubbletea`, `charmbracelet/lipgloss`, and their transitive deps. `golang.org/x/term` may still be needed — check if anything else imports it. If not, it gets removed too.

**Step 5: Run tests**

Run: `cd /home/winler/projects/golem && go test ./... && go build ./...`
Expected: PASS

**Step 6: Commit**

```bash
git add -A
git commit -m "refactor: remove TUI mode, add golem status --watch"
```

---

### Task 5: Rename `run` to `code`

**Files:**
- Rename: `cmd/run.go` → `cmd/code.go`
- Modify: all references from `runCmd` to `codeCmd`

**Step 1: Rename and update the command**

In `cmd/code.go` (renamed from `cmd/run.go`):

```go
var codeCmd = &cobra.Command{
	Use:     "code",
	Aliases: []string{"run"},    // backwards compat
	Short:   "Run the autonomous builder loop",
	// ... RunE stays the same
}
```

Update `init()`:
```go
rootCmd.AddCommand(codeCmd)
codeCmd.Flags().Int("max-iterations", 20, "maximum number of iterations")
// ... etc
```

**Step 2: Run tests**

Run: `cd /home/winler/projects/golem && go test ./... && go build ./...`
Expected: PASS

**Step 3: Verify alias works**

Run: `cd /home/winler/projects/golem && go run . run --help`
Expected: Should show help for the code command

**Step 4: Commit**

```bash
git add cmd/code.go
git rm cmd/run.go
git commit -m "refactor: rename golem run to golem code (run kept as alias)"
```

---

### Task 6: Add `golem qa` command

**Files:**
- Create: `templates/qa-prompt.md`
- Modify: `templates/embed.go` — add qa-prompt.md to embed directive
- Modify: `internal/scaffold/scaffold.go` — add qa-prompt.md to template files
- Create: `cmd/qa.go`

**Step 1: Write the QA prompt template**

```markdown
You are a QA tester for this project. You are NOT a builder or code reviewer.
Your job is to run the application, test user flows, and report bugs.

## What to Read
1. All design and implementation docs in `{{DOCS_PATH}}` for expected behavior.
2. `.ctx/state.yaml` for current progress and what's been built.
3. `.ctx/log.yaml` for recent session history.

## What to Do
1. Build and run the application.
2. Test the user flows described in the design docs.
3. Try edge cases, invalid inputs, and error scenarios.
4. Verify that completed tasks actually work end-to-end.

{{ITERATION_CONTEXT}}

{{TASK_OVERRIDE}}

## Reporting
For each bug found, add a task to `.ctx/state.yaml`:
- Prefix the name with `[qa]`
- Set status to `todo`
- Include reproduction steps in `notes`

For each task marked `done` that you verified works:
- Add a note confirming it was tested

## End of Session
Use the golem MCP tools to update state:
1. Call `log_session` with task: "qa testing", outcome, summary, and files tested.
2. Call `add_pitfall` for any gotchas discovered.

If the golem MCP tools are not available, edit `.ctx/state.yaml` and `.ctx/log.yaml` directly.

If you found bugs that need fixing:
  output <promise>NEEDS_WORK</promise>

If all tested flows pass:
  output <promise>APPROVED</promise>
```

**Step 2: Update embed directive**

In `templates/embed.go`:
```go
//go:embed state.yaml log.yaml prompt.md review-prompt.md qa-prompt.md claude.md
var FS embed.FS
```

**Step 3: Add qa-prompt.md to scaffold template files**

In `internal/scaffold/scaffold.go`, add to the `templateFiles` map:
```go
"qa-prompt.md": "qa-prompt.md",
```

**Step 4: Create the qa command**

```go
// cmd/qa.go
package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/lofari/golem/internal/config"
	"github.com/lofari/golem/internal/runner"
	"github.com/lofari/golem/internal/scaffold"
)

var qaCmd = &cobra.Command{
	Use:   "qa",
	Short: "Run QA testing — execute the app and test user flows",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		if !scaffold.CtxExists(dir) {
			return fmt.Errorf(".ctx/ not found — run `golem init` first")
		}

		ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		// Load config
		globalPath := config.GlobalPath()
		projectPath := config.ProjectPath(dir)
		cfg := config.Load(globalPath, projectPath)

		// Flags override config
		maxIter := cfg.MaxIterations
		if cmd.Flags().Changed("max-iterations") {
			maxIter, _ = cmd.Flags().GetInt("max-iterations")
		}
		maxTurns := cfg.MaxTurns
		if cmd.Flags().Changed("max-turns") {
			maxTurns, _ = cmd.Flags().GetInt("max-turns")
		}
		task, _ := cmd.Flags().GetString("task")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		verbose := cfg.Verbose
		if cmd.Flags().Changed("verbose") {
			verbose, _ = cmd.Flags().GetBool("verbose")
		}
		model := cfg.Model
		if cmd.Flags().Changed("model") {
			model, _ = cmd.Flags().GetString("model")
		}
		pluginDirs := cfg.PluginDir
		if cmd.Flags().Changed("plugin-dir") {
			pluginDirs, _ = cmd.Flags().GetStringSlice("plugin-dir")
		}
		sandbox := cfg.Sandbox
		if cmd.Flags().Changed("sandbox") {
			sandbox, _ = cmd.Flags().GetBool("sandbox")
		}
		sandboxTools := cfg.SandboxTools
		if cmd.Flags().Changed("sandbox-tools") {
			sandboxTools, _ = cmd.Flags().GetStringSlice("sandbox-tools")
		}
		sandboxTimeout := cfg.SandboxTimeout
		if cmd.Flags().Changed("sandbox-timeout") {
			sandboxTimeout, _ = cmd.Flags().GetString("sandbox-timeout")
		}
		sandboxMemory := cfg.SandboxMemory
		if cmd.Flags().Changed("sandbox-memory") {
			sandboxMemory, _ = cmd.Flags().GetString("sandbox-memory")
		}
		mcpEnabled := cfg.MCP
		if cmd.Flags().Changed("mcp") {
			mcpEnabled, _ = cmd.Flags().GetBool("mcp")
		}

		claudeRunner := &runner.ClaudeRunner{
			Verbose:        verbose,
			StreamJSON:     true,
			PluginDirs:     pluginDirs,
			Sandbox:        sandbox,
			SandboxTools:   sandboxTools,
			SandboxTimeout: sandboxTimeout,
			SandboxMemory:  sandboxMemory,
		}

		// QA uses the qa-prompt.md template via the builder loop
		// We override the prompt template by using a QA-specific config
		result, err := runner.RunBuilderLoop(ctx, runner.BuilderConfig{
			Dir:            dir,
			MaxIterations:  maxIter,
			MaxTurns:       maxTurns,
			Model:          model,
			TaskOverride:   task,
			DryRun:         dryRun,
			Verbose:        verbose,
			MCPEnabled:     mcpEnabled,
			Runner:         claudeRunner,
			PromptTemplate: "qa-prompt.md",
		})
		if err != nil {
			return err
		}
		if result.Halted {
			return fmt.Errorf("qa halted: %s", result.HaltReason)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(qaCmd)
	qaCmd.Flags().Int("max-iterations", 20, "maximum number of iterations")
	qaCmd.Flags().Int("max-turns", 200, "max turns per Claude Code session")
	qaCmd.Flags().String("task", "", "force agent to test a specific task")
	qaCmd.Flags().Bool("dry-run", false, "show rendered prompt without executing")
	qaCmd.Flags().Bool("verbose", false, "extra output detail")
	qaCmd.Flags().Bool("sandbox", false, "run Claude inside a warden sandbox container")
	qaCmd.Flags().StringSlice("sandbox-tools", nil, "additional warden tools (e.g., go,node)")
	qaCmd.Flags().String("sandbox-timeout", "", "sandbox timeout (e.g., 2h)")
	qaCmd.Flags().String("sandbox-memory", "", "sandbox memory limit (e.g., 8g)")
	qaCmd.Flags().Bool("mcp", true, "enable golem MCP server")
}
```

**Step 5: Add `PromptTemplate` field to `BuilderConfig`**

In `internal/runner/builder.go`, add to `BuilderConfig`:
```go
PromptTemplate string // prompt template filename (default: "prompt.md")
```

In `RunBuilderLoop`, where the prompt is rendered (around line 99), use the field:
```go
templateFile := cfg.PromptTemplate
if templateFile == "" {
    templateFile = "prompt.md"
}
prompt, err := RenderPrompt(cfg.Dir, templateFile, PromptVars{
```

**Step 6: Run tests**

Run: `cd /home/winler/projects/golem && go test ./... && go build ./...`
Expected: PASS

**Step 7: Commit**

```bash
git add templates/qa-prompt.md templates/embed.go internal/scaffold/scaffold.go cmd/qa.go internal/runner/builder.go
git commit -m "feat: add golem qa command for autonomous QA testing"
```

---

### Task 7: Reduce flag duplication across commands

**Files:**
- Create: `cmd/helpers.go`
- Modify: `cmd/code.go`, `cmd/review.go`, `cmd/qa.go`

**Step 1: Extract common flag registration and config resolution**

The flag-loading boilerplate (load config, check `cmd.Flags().Changed()`, resolve each value) is repeated across code/review/qa. Extract it.

```go
// cmd/helpers.go
package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/lofari/golem/internal/config"
	"github.com/lofari/golem/internal/runner"
	"github.com/lofari/golem/internal/scaffold"
)

// resolvedConfig holds the final config after merging config files + flags.
type resolvedConfig struct {
	config.Config
	Task   string
	DryRun bool
	Review bool
}

// addAgentFlags registers the common set of flags for agent commands (code, review, qa).
func addAgentFlags(cmd *cobra.Command) {
	cmd.Flags().Int("max-iterations", 20, "maximum number of iterations")
	cmd.Flags().Int("max-turns", 200, "max turns per Claude Code session")
	cmd.Flags().String("task", "", "force agent to work on a specific task")
	cmd.Flags().Bool("dry-run", false, "show rendered prompt without executing")
	cmd.Flags().Bool("verbose", false, "extra output detail")
	cmd.Flags().Bool("sandbox", false, "run Claude inside a warden sandbox container")
	cmd.Flags().StringSlice("sandbox-tools", nil, "additional warden tools (e.g., go,node,python)")
	cmd.Flags().String("sandbox-timeout", "", "sandbox execution timeout (e.g., 2h, 30m)")
	cmd.Flags().String("sandbox-memory", "", "sandbox memory limit (e.g., 8g)")
	cmd.Flags().Bool("mcp", true, "enable golem MCP server for structured state updates")
}

// resolveConfig loads config files and applies flag overrides.
func resolveConfig(cmd *cobra.Command, dir string) resolvedConfig {
	globalPath := config.GlobalPath()
	projectPath := ""
	if scaffold.CtxExists(dir) {
		projectPath = config.ProjectPath(dir)
	}
	cfg := config.Load(globalPath, projectPath)

	if cmd.Flags().Changed("max-iterations") {
		cfg.MaxIterations, _ = cmd.Flags().GetInt("max-iterations")
	}
	if cmd.Flags().Changed("max-turns") {
		cfg.MaxTurns, _ = cmd.Flags().GetInt("max-turns")
	}
	if cmd.Flags().Changed("verbose") {
		cfg.Verbose, _ = cmd.Flags().GetBool("verbose")
	}
	if cmd.Flags().Changed("sandbox") {
		cfg.Sandbox, _ = cmd.Flags().GetBool("sandbox")
	}
	if cmd.Flags().Changed("sandbox-tools") {
		cfg.SandboxTools, _ = cmd.Flags().GetStringSlice("sandbox-tools")
	}
	if cmd.Flags().Changed("sandbox-timeout") {
		cfg.SandboxTimeout, _ = cmd.Flags().GetString("sandbox-timeout")
	}
	if cmd.Flags().Changed("sandbox-memory") {
		cfg.SandboxMemory, _ = cmd.Flags().GetString("sandbox-memory")
	}
	if cmd.Flags().Changed("mcp") {
		cfg.MCP, _ = cmd.Flags().GetBool("mcp")
	}
	if cmd.Flags().Changed("model") {
		cfg.Model, _ = cmd.Flags().GetString("model")
	}
	if cmd.Flags().Changed("plugin-dir") {
		cfg.PluginDir, _ = cmd.Flags().GetStringSlice("plugin-dir")
	}

	rc := resolvedConfig{Config: cfg}
	rc.Task, _ = cmd.Flags().GetString("task")
	rc.DryRun, _ = cmd.Flags().GetBool("dry-run")
	rc.Review, _ = cmd.Flags().GetBool("review")
	return rc
}

// newClaudeRunner creates a ClaudeRunner from resolved config.
func newClaudeRunner(cfg resolvedConfig) *runner.ClaudeRunner {
	return &runner.ClaudeRunner{
		Verbose:        cfg.Verbose,
		StreamJSON:     true,
		PluginDirs:     cfg.PluginDir,
		Sandbox:        cfg.Sandbox,
		SandboxTools:   cfg.SandboxTools,
		SandboxTimeout: cfg.SandboxTimeout,
		SandboxMemory:  cfg.SandboxMemory,
	}
}
```

**Step 2: Simplify code.go, review.go, qa.go to use helpers**

Each command's `RunE` becomes much shorter — load dir, call `resolveConfig`, call `newClaudeRunner`, run the loop.

**Step 3: Run tests**

Run: `cd /home/winler/projects/golem && go test ./... && go build ./...`
Expected: PASS

**Step 4: Commit**

```bash
git add cmd/helpers.go cmd/code.go cmd/review.go cmd/qa.go
git commit -m "refactor: extract shared flag handling into cmd/helpers.go"
```

---

### Task 8: Update CLAUDE.md template and project docs

**Files:**
- Modify: `templates/claude.md` — update to reference new commands
- Modify: `CLAUDE.md` — update project docs

**Step 1: Update templates/claude.md**

Update references from `golem run` to `golem code`, mention config system, remove TUI references.

**Step 2: Update CLAUDE.md**

Update the project CLAUDE.md to reflect:
- `golem code` instead of `golem run`
- Config system
- No TUI
- `golem qa` command

**Step 3: Commit**

```bash
git add templates/claude.md CLAUDE.md
git commit -m "docs: update CLAUDE.md and template for new UX"
```

---

### Task 9: Final verification

**Step 1: Run full test suite**

Run: `cd /home/winler/projects/golem && go test ./...`
Expected: All PASS

**Step 2: Build and verify help output**

```bash
cd /home/winler/projects/golem && go build -o golem . && ./golem --help
./golem code --help
./golem review --help
./golem qa --help
./golem config --help
./golem config set --help
./golem status --help
```

Verify:
- `golem code` shows as primary command
- `golem run` works as alias
- No TUI-related flags appear
- `golem config` subcommands work
- `golem qa` exists with correct flags
- `golem status --watch` flag exists

**Step 3: Clean up binary**

```bash
rm ./golem
```

**Step 4: Commit any fixes**

If any issues found, fix and commit.
