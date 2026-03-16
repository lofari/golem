// cmd/setup.go
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/lofari/golem/internal/config"
	golemctx "github.com/lofari/golem/internal/ctx"
	"github.com/lofari/golem/internal/runner"
	"github.com/lofari/golem/internal/scaffold"
	"github.com/lofari/golem/templates"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Auto-detect project stack and configure golem interactively",
	Long:  "Analyzes the project to detect tech stack, test/lint commands, and CI setup, then writes .ctx/config.yaml.",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		if !scaffold.CtxExists(dir) {
			return fmt.Errorf(".ctx/ not found — run `golem init` first")
		}

		if err := runSetupSession(cmd, dir); err != nil {
			return err
		}

		return applySetupOutput(dir)
	},
}

func runSetupSession(cmd *cobra.Command, dir string) error {
	promptData, err := templates.FS.ReadFile("prompts/setup.md")
	if err != nil {
		return fmt.Errorf("reading setup prompt: %w", err)
	}

	// Inject current config context if already configured
	prompt := string(promptData)
	cfgPath := config.ProjectPath(dir)
	if _, err := os.Stat(cfgPath); err == nil {
		cfg := config.Load("", cfgPath)
		prompt += fmt.Sprintf("\n\n## Current Configuration\nThis project already has a config. Current values:\n"+
			"- test-cmd: %s\n- lint-cmd: %s\n- agent: %s\n- sandbox: %v\n- mcp: %v\n",
			cfgVal(cfg.AgentOpts, "test-cmd"), cfgVal(cfg.AgentOpts, "lint-cmd"),
			cfg.Agent, cfg.Sandbox, cfg.MCP)
	}

	// Load config for model/plugin overrides
	globalPath := config.GlobalPath()
	projectPath := config.ProjectPath(dir)
	cfgFull := config.Load(globalPath, projectPath)

	model := cfgFull.Model
	if cmd.Flags().Changed("model") {
		model, _ = cmd.Flags().GetString("model")
	}

	claudeArgs := []string{"--append-system-prompt", prompt}
	if model != "" {
		claudeArgs = append(claudeArgs, "--model", model)
	}
	for _, d := range cfgFull.PluginDir {
		claudeArgs = append(claudeArgs, "--plugin-dir", d)
	}
	if cfgFull.MCP {
		mcpPath, mcpErr := runner.WriteMCPConfig(dir, true)
		if mcpErr == nil {
			claudeArgs = append(claudeArgs, "--mcp-config", mcpPath)
		}
	}

	fmt.Fprintln(os.Stderr, "golem: analyzing project and configuring...")
	fmt.Fprintln(os.Stderr)

	claude := exec.Command("claude", claudeArgs...)
	claude.Dir = dir
	claude.Stdin = os.Stdin
	claude.Stdout = os.Stdout
	claude.Stderr = os.Stderr

	return claude.Run()
}

func cfgVal(opts map[string]interface{}, key string) string {
	if opts == nil {
		return "(not set)"
	}
	if v, ok := opts[key]; ok {
		return fmt.Sprintf("%v", v)
	}
	return "(not set)"
}

// setupOutput is the JSON structure Claude writes to session-output.json.
type setupOutput struct {
	Config map[string]interface{} `json:"config"`
	State  struct {
		Stack string `json:"stack"`
		Name  string `json:"name"`
	} `json:"state"`
	Graph bool `json:"graph"`
}

// applySetupOutput reads session-output.json and routes values to config and state files.
func applySetupOutput(dir string) error {
	path := filepath.Join(dir, "session-output.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, "golem: no session-output.json found — setup may have been cancelled")
			return nil
		}
		return fmt.Errorf("reading session-output.json: %w", err)
	}
	defer os.Remove(path)

	var output setupOutput
	if err := json.Unmarshal(data, &output); err != nil {
		return fmt.Errorf("parsing session-output.json: %w", err)
	}

	// Write config keys to .ctx/config.yaml
	cfgPath := config.ProjectPath(dir)
	var keysWritten []string
	for key, val := range output.Config {
		if val == nil {
			continue
		}
		strVal := fmt.Sprintf("%v", val)
		if err := config.SetValue(cfgPath, key, strVal); err != nil {
			fmt.Fprintf(os.Stderr, "golem: warning: could not set %s: %v\n", key, err)
			continue
		}
		keysWritten = append(keysWritten, key)
	}

	// Write state keys to .ctx/state.yaml
	if output.State.Stack != "" || output.State.Name != "" {
		state, err := golemctx.ReadState(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "golem: warning: could not read state: %v\n", err)
		} else {
			if output.State.Stack != "" {
				state.Project.Stack = output.State.Stack
			}
			if output.State.Name != "" {
				state.Project.Name = output.State.Name
			}
			if err := golemctx.WriteState(dir, state); err != nil {
				fmt.Fprintf(os.Stderr, "golem: warning: could not write state: %v\n", err)
			}
		}
	}

	// Summary
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "golem: setup complete\n")
	if len(keysWritten) > 0 {
		fmt.Fprintf(os.Stderr, "golem: configured: %s\n", joinKeys(keysWritten))
	}
	if output.State.Stack != "" {
		fmt.Fprintf(os.Stderr, "golem: stack: %s\n", output.State.Stack)
	}
	if output.Graph {
		fmt.Fprintln(os.Stderr, "golem: tip: run `golem graph build && golem graph embed` to enable semantic search")
	}

	return nil
}

func joinKeys(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	result := keys[0]
	for _, k := range keys[1:] {
		result += ", " + k
	}
	return result
}

func init() {
	rootCmd.AddCommand(setupCmd)
}
