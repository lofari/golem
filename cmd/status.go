// cmd/status.go
package cmd

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/lofari/golem/internal/ctx"
	"github.com/lofari/golem/internal/display"
	"github.com/lofari/golem/internal/scaffold"
	"github.com/lofari/golem/internal/tui"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current project state",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		if !scaffold.CtxExists(dir) {
			return fmt.Errorf(".ctx/ not found — run `golem init` first")
		}

		noTUI, _ := cmd.Flags().GetBool("no-tui")
		useTUI := !noTUI && term.IsTerminal(int(os.Stdout.Fd()))

		if useTUI {
			m := tui.NewStatusModel(dir)
			p := tea.NewProgram(m, tea.WithAltScreen())
			if _, err := p.Run(); err != nil {
				return fmt.Errorf("TUI error: %w", err)
			}
			return nil
		}

		// Plain text fallback
		state, err := ctx.ReadState(dir)
		if err != nil {
			return err
		}
		log, err := ctx.ReadLog(dir)
		if err != nil {
			return err
		}
		display.PrintStatus(os.Stdout, state, len(log.Sessions))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
	statusCmd.Flags().Bool("no-tui", false, "disable terminal UI (plain text output)")
}
