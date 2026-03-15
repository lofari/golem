package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lofari/golem/internal/server"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the golem API server for the desktop app",
	Long:  "Starts an HTTP/WebSocket server that manages golem processes and exposes project state.",
	RunE: func(cmd *cobra.Command, args []string) error {
		addr, _ := cmd.Flags().GetString("addr")

		srv := server.New(server.Config{Addr: addr})

		// Auto-register current directory if it has .ctx/
		dir, _ := os.Getwd()
		if _, err := os.Stat(dir + "/.ctx"); err == nil {
			srv.RegisterProject(dir)
			fmt.Fprintf(os.Stderr, "golem serve: registered project at %s\n", dir)
		}

		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		fmt.Fprintf(os.Stderr, "golem serve: listening on %s\n", addr)

		errCh := make(chan error, 1)
		go func() {
			errCh <- srv.ListenAndServe()
		}()

		select {
		case <-ctx.Done():
			fmt.Fprintf(os.Stderr, "\ngolem serve: shutting down\n")
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			return srv.Shutdown(shutdownCtx)
		case err := <-errCh:
			return err
		}
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.Flags().String("addr", ":8314", "listen address")
}
