package cmd

import (
	"bufio"
	"fmt"
	"os"
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
	Use:   "set [key] [value]",
	Short: "Set a configuration value (interactive wizard if no args)",
	Args:  cobra.MaximumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		global, _ := cmd.Flags().GetBool("global")

		if len(args) == 1 {
			return fmt.Errorf("expected 0 args (wizard) or 2 args (key value), got 1")
		}

		// Determine target path
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

		// Direct set: golem config set <key> <value>
		if len(args) == 2 {
			return config.SetValue(path, args[0], args[1])
		}

		// Wizard mode: golem config set
		return runConfigWizard(path, global)
	},
}

func runConfigWizard(path string, global bool) error {
	scope := "project"
	if global {
		scope = "global"
	}
	fmt.Fprintf(os.Stderr, "golem: configuring %s settings (%s)\n", scope, path)
	fmt.Fprintf(os.Stderr, "golem: press Enter to keep current value, or type a new value\n\n")

	// Load current resolved config to show current values
	dir, _ := os.Getwd()
	globalPath := config.GlobalPath()
	projectPath := ""
	if scaffold.CtxExists(dir) {
		projectPath = config.ProjectPath(dir)
	}
	cfg := config.Load(globalPath, projectPath)

	scanner := bufio.NewScanner(os.Stdin)
	changed := 0

	for _, ki := range config.Keys() {
		current, _ := config.GetValue(cfg, ki.Key)
		if current == "" {
			current = "(not set)"
		}

		fmt.Fprintf(os.Stderr, "  %s — %s\n", ki.Key, ki.Description)
		fmt.Fprintf(os.Stderr, "  current: %s\n", current)
		fmt.Fprintf(os.Stderr, "  > ")

		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			fmt.Fprintln(os.Stderr)
			continue
		}

		if err := config.SetValue(path, ki.Key, input); err != nil {
			return fmt.Errorf("setting %s: %w", ki.Key, err)
		}
		changed++
		fmt.Fprintln(os.Stderr)
	}

	if changed == 0 {
		fmt.Fprintln(os.Stderr, "golem: no changes made")
	} else {
		fmt.Fprintf(os.Stderr, "golem: saved %d setting(s) to %s\n", changed, path)
	}
	return nil
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
