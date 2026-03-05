package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/lofari/golem/internal/ctx"
	"github.com/lofari/golem/internal/display"
	"github.com/lofari/golem/internal/scaffold"
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

		watch, _ := cmd.Flags().GetBool("watch")
		if watch {
			return watchStatus(dir)
		}

		return printStatusOnce(dir)
	},
}

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
			fmt.Print("\033[H\033[2J")
			printStatusOnce(dir)
		}
	}
}

func printStatusOnce(dir string) error {
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
}

func init() {
	rootCmd.AddCommand(statusCmd)
	statusCmd.Flags().Bool("watch", false, "continuously refresh status every 2 seconds")
}
