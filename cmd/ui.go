package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
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

		dir, _ := os.Getwd()
		hasCtx := false
		if _, err := os.Stat(dir + "/.ctx"); err == nil {
			hasCtx = true
		}

		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		// Check if the port is already in use (another golem ui instance)
		ownServer := true
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			var opErr *net.OpError
			if errors.As(err, &opErr) && errors.Is(opErr.Err, syscall.EADDRINUSE) {
				fmt.Fprintf(os.Stderr, "golem ui: server already running on %s, reusing\n", addr)
				ownServer = false
			} else {
				return fmt.Errorf("listen %s: %w", addr, err)
			}
		}

		if hasCtx {
			if ownServer {
				srv.RegisterProject(dir)
			} else {
				// Register with the existing server via API
				body := strings.NewReader(`{"dir":"` + dir + `"}`)
				http.Post("http://localhost"+addr+"/api/projects", "application/json", body)
			}
			fmt.Fprintf(os.Stderr, "golem ui: registered project at %s\n", dir)
		}

		var errCh chan error
		if ownServer {
			fmt.Fprintf(os.Stderr, "golem ui: starting server on %s\n", addr)
			errCh = make(chan error, 1)
			go func() {
				errCh <- srv.Serve(ln)
			}()
		}

		// Find and launch the desktop app
		appPath := findAppBinary()
		if appPath == "" {
			fmt.Fprintf(os.Stderr, "golem ui: desktop app not found (install golem-ui next to golem binary or in PATH)\n")
			fmt.Fprintf(os.Stderr, "golem ui: server running at http://localhost%s\n", addr)
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

		if ownServer {
			select {
			case <-ctx.Done():
				fmt.Fprintf(os.Stderr, "\ngolem ui: shutting down\n")
				return nil
			case err := <-errCh:
				return err
			}
		} else {
			<-ctx.Done()
			fmt.Fprintf(os.Stderr, "\ngolem ui: shutting down\n")
			return nil
		}
	},
}

// findAppBinary looks for the Golem desktop app binary in common locations.
func findAppBinary() string {
	names := []string{"golem-ui", "golem_ui"}

	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		for _, name := range names {
			p := filepath.Join(dir, name)
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}

	for _, name := range names {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}

	return ""
}

func init() {
	rootCmd.AddCommand(uiCmd)
	uiCmd.Flags().String("addr", ":8314", "server listen address")
}
