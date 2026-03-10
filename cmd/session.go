package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Run a single Claude session with a prompt file",
	Long:  "Spawns one Claude session. Used by golem-dsl as a session adapter.",
	RunE: func(cmd *cobra.Command, args []string) error {
		promptFile, _ := cmd.Flags().GetString("prompt")
		if promptFile == "" {
			return fmt.Errorf("--prompt is required")
		}

		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		if d, _ := cmd.Flags().GetString("dir"); d != "" {
			dir = d
		}

		dryRun, _ := cmd.Flags().GetBool("dry-run")

		prompt, err := os.ReadFile(promptFile)
		if err != nil {
			return fmt.Errorf("reading prompt file: %w", err)
		}

		if dryRun {
			fmt.Print(string(prompt))
			return nil
		}

		rc := resolveConfig(cmd, dir)
		cr := newClaudeRunner(rc)

		maxTurns := rc.MaxToolCalls
		if cmd.Flags().Changed("max-turns") {
			maxTurns, _ = cmd.Flags().GetInt("max-turns")
		}

		output, err := cr.Run(cmd.Context(), dir, string(prompt), maxTurns, rc.Model)
		if err != nil {
			return err
		}

		fmt.Print(output)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(sessionCmd)
	sessionCmd.Flags().String("prompt", "", "Path to prompt file (required)")
	sessionCmd.Flags().String("dir", "", "Working directory (default: cwd)")
	sessionCmd.Flags().Int("max-turns", 200, "Maximum tool calls for the session")
	addAgentFlags(sessionCmd)
}
