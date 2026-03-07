package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/lofari/golem/internal/server"
	"github.com/spf13/cobra"
)

var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Start the golem server and open the desktop app",
	Long:  "Starts the API server and launches the Golem desktop application.",
	RunE: func(cmd *cobra.Command, args []string) error {
		addr, _ := cmd.Flags().GetString("addr")

		srv := server.New(server.Config{Addr: addr})

		// Auto-register current directory if it has .ctx/
		dir, _ := os.Getwd()
		if _, err := os.Stat(dir + "/.ctx"); err == nil {
			srv.RegisterProject(dir)
			fmt.Fprintf(os.Stderr, "golem ui: registered project at %s\n", dir)
		}

		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		fmt.Fprintf(os.Stderr, "golem ui: starting server on %s\n", addr)

		errCh := make(chan error, 1)
		go func() {
			errCh <- srv.ListenAndServe()
		}()

		// Find and launch the Tauri app
		appPath := findAppBinary()
		if appPath == "" {
			fmt.Fprintf(os.Stderr, "golem ui: desktop app not found, server running at http://localhost%s\n", addr)
		} else {
			fmt.Fprintf(os.Stderr, "golem ui: launching desktop app\n")
			appCmd := exec.CommandContext(ctx, appPath)
			appCmd.Stdout = os.Stdout
			appCmd.Stderr = os.Stderr
			if err := appCmd.Start(); err != nil {
				fmt.Fprintf(os.Stderr, "golem ui: failed to launch app: %v\n", err)
			} else {
				// When the app exits, shut down
				go func() {
					appCmd.Wait()
					stop()
				}()
			}
		}

		select {
		case <-ctx.Done():
			fmt.Fprintf(os.Stderr, "\ngolem ui: shutting down\n")
			return nil
		case err := <-errCh:
			return err
		}
	},
}

// findAppBinary looks for the Golem desktop app binary in common locations.
func findAppBinary() string {
	// Check next to the golem binary
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exe)
		candidates := []string{
			filepath.Join(dir, "golem-ui"),
			filepath.Join(dir, "Golem"),
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				return c
			}
		}
	}

	// Check in PATH
	if path, err := exec.LookPath("golem-ui"); err == nil {
		return path
	}

	// Check the Tauri build output relative to golem source
	dir, _ := os.Getwd()
	tauriRelease := filepath.Join(dir, "ui", "src-tauri", "target", "release", "golem-ui")
	if _, err := os.Stat(tauriRelease); err == nil {
		return tauriRelease
	}

	return ""
}

func init() {
	rootCmd.AddCommand(uiCmd)
	uiCmd.Flags().String("addr", ":8314", "server listen address")
}
