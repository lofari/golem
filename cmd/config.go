package cmd

import (
	"fmt"
	"os"

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
