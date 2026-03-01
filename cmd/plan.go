// cmd/plan.go
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/lofari/golem/internal/scaffold"
)

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Open an interactive Claude Code session for planning",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		if !scaffold.CtxExists(dir) {
			return fmt.Errorf(".ctx/ not found — run `golem init` first")
		}

		fmt.Fprintln(os.Stderr, "golem: launching interactive Claude Code session...")
		fmt.Fprintln(os.Stderr, "golem: CLAUDE.md conventions are active — the agent knows about .ctx/")
		fmt.Fprintln(os.Stderr, "golem: exit the session when planning is complete")
		fmt.Fprintln(os.Stderr)

		model, _ := cmd.Flags().GetString("model")

		claudeArgs := []string{}
		if model != "" {
			claudeArgs = append(claudeArgs, "--model", model)
		}

		ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		claude := exec.CommandContext(ctx, "claude", claudeArgs...)
		claude.Dir = dir
		claude.Stdin = os.Stdin
		claude.Stdout = os.Stdout
		claude.Stderr = os.Stderr
		claude.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		claude.Cancel = func() error {
			return syscall.Kill(-claude.Process.Pid, syscall.SIGINT)
		}

		return claude.Run()
	},
}

func init() {
	rootCmd.AddCommand(planCmd)
}
