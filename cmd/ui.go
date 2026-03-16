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

	"github.com/lofari/golem/internal/scaffold"
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

		// Auto-scaffold if no .ctx/ exists
		if !hasCtx {
			fmt.Fprintf(os.Stderr, "golem ui: no .ctx/ found, initializing project...\n")
			if _, err := scaffold.Init(dir, scaffold.InitOptions{}); err != nil {
				return fmt.Errorf("auto-init: %w", err)
			}
			hasCtx = true
			fmt.Fprintf(os.Stderr, "golem ui: run `golem setup` in the terminal pane to auto-configure\n")
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
			fmt.Fprintf(os.Stderr, "golem ui: desktop app not found — run `golem ui install` to build and install it\n")
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

var uiInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Build the Flutter desktop app and install it next to the golem binary",
	RunE: func(cmd *cobra.Command, args []string) error {
		source, _ := cmd.Flags().GetString("source")

		// Find Flutter source directory
		flutterDir, err := findFlutterSource(source)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "golem ui install: found Flutter source at %s\n", flutterDir)

		// Check flutter is available
		if _, err := exec.LookPath("flutter"); err != nil {
			return fmt.Errorf("flutter CLI not found in PATH — install Flutter SDK first")
		}

		// Build
		fmt.Fprintf(os.Stderr, "golem ui install: building desktop app...\n")
		build := exec.Command("flutter", "build", "linux")
		build.Dir = flutterDir
		build.Stdout = os.Stdout
		build.Stderr = os.Stderr
		if err := build.Run(); err != nil {
			return fmt.Errorf("flutter build failed: %w", err)
		}

		// Find the bundle
		bundleSrc := filepath.Join(flutterDir, "build", "linux", "x64", "release", "bundle")
		if _, err := os.Stat(bundleSrc); err != nil {
			return fmt.Errorf("build output not found at %s", bundleSrc)
		}

		// Determine install destination (next to golem binary)
		installDir, err := golemBinDir()
		if err != nil {
			return err
		}

		bundleDst := filepath.Join(installDir, "golem-ui-bundle")
		symlinkPath := filepath.Join(installDir, "golem_ui")

		// Remove old install if present
		os.RemoveAll(bundleDst)
		os.Remove(symlinkPath)

		// Copy bundle directory
		fmt.Fprintf(os.Stderr, "golem ui install: copying to %s\n", bundleDst)
		if err := copyDir(bundleSrc, bundleDst); err != nil {
			return fmt.Errorf("copying bundle: %w", err)
		}

		// Create symlink
		if err := os.Symlink(filepath.Join(bundleDst, "golem_ui"), symlinkPath); err != nil {
			return fmt.Errorf("creating symlink: %w", err)
		}

		fmt.Fprintf(os.Stderr, "golem ui install: installed to %s\n", symlinkPath)

		// Initialize project if no .ctx/ exists
		dir, _ := os.Getwd()
		if !scaffold.CtxExists(dir) {
			fmt.Fprintf(os.Stderr, "golem ui install: initializing project...\n")
			result, err := scaffold.Init(dir, scaffold.InitOptions{})
			if err != nil {
				return fmt.Errorf("init: %w", err)
			}
			fmt.Fprintf(os.Stderr, "golem ui install: created .ctx/ with %d files\n", len(result.Created))
		}

		fmt.Fprintf(os.Stderr, "golem ui install: done — run `golem ui` to launch\n")
		return nil
	},
}

// findFlutterSource locates the ui/flutter directory.
func findFlutterSource(explicit string) (string, error) {
	if explicit != "" {
		if _, err := os.Stat(filepath.Join(explicit, "pubspec.yaml")); err != nil {
			return "", fmt.Errorf("no pubspec.yaml found in %s", explicit)
		}
		return explicit, nil
	}

	// Try relative to the repo root (git rev-parse)
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err == nil {
		candidate := filepath.Join(strings.TrimSpace(string(out)), "ui", "flutter")
		if _, err := os.Stat(filepath.Join(candidate, "pubspec.yaml")); err == nil {
			return candidate, nil
		}
	}

	// Try relative to golem binary
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "..", "ui", "flutter")
		if _, err := os.Stat(filepath.Join(candidate, "pubspec.yaml")); err == nil {
			return filepath.Clean(candidate), nil
		}
	}

	return "", fmt.Errorf("flutter source not found — use --source to specify the ui/flutter directory")
}

// golemBinDir returns the directory containing the golem binary.
func golemBinDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("cannot determine golem binary location: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("resolving golem binary path: %w", err)
	}
	return filepath.Dir(resolved), nil
}

// copyDir recursively copies src to dst.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}

		// Handle symlinks
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}

func init() {
	rootCmd.AddCommand(uiCmd)
	uiCmd.AddCommand(uiInstallCmd)
	uiCmd.Flags().String("addr", ":8314", "server listen address")
	uiInstallCmd.Flags().String("source", "", "path to ui/flutter directory (auto-detected if omitted)")
}
