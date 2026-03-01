// cmd/log.go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/lofari/golem/internal/ctx"
	"github.com/lofari/golem/internal/display"
	"github.com/lofari/golem/internal/scaffold"
)

var logCmd = &cobra.Command{
	Use:   "log",
	Short: "Show iteration history",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		if !scaffold.CtxExists(dir) {
			return fmt.Errorf(".ctx/ not found — run `golem init` first")
		}

		l, err := ctx.ReadLog(dir)
		if err != nil {
			return err
		}

		failures, _ := cmd.Flags().GetBool("failures")
		last, _ := cmd.Flags().GetInt("last")

		sessions := l.Sessions
		if failures {
			sessions = l.FailedSessions()
		}
		if last > 0 {
			log := ctx.Log{Sessions: sessions}
			sessions = log.LastNSessions(last)
		}

		display.PrintLog(os.Stdout, sessions)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(logCmd)
	logCmd.Flags().Int("last", 0, "show only the last N entries")
	logCmd.Flags().Bool("failures", false, "show only blocked/unproductive sessions")
}
