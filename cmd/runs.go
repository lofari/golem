package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/spf13/cobra"
)

var runsCmd = &cobra.Command{
	Use:   "runs",
	Short: "Manage blueprint engine runs",
}

var runsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List recent runs",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, _ := os.Getwd()
		runsDir := filepath.Join(dir, ".ctx", "runs")
		entries, err := os.ReadDir(runsDir)
		if err != nil {
			return fmt.Errorf("no runs found (is .ctx/runs/ present?)")
		}

		type runInfo struct {
			ID       string
			Agent    string
			Status   string
			Duration string
			ModTime  time.Time
		}

		var runs []runInfo
		for _, e := range entries {
			if !e.IsDir() || e.Name() == "current" {
				continue
			}
			info := runInfo{ID: e.Name()}
			fi, _ := e.Info()
			if fi != nil {
				info.ModTime = fi.ModTime()
			}

			// Read log.json for agent name and status
			logPath := filepath.Join(runsDir, e.Name(), "log.json")
			if f, err := os.Open(logPath); err == nil {
				scanner := bufio.NewScanner(f)
				for scanner.Scan() {
					var ev map[string]any
					if json.Unmarshal(scanner.Bytes(), &ev) == nil {
						if ev["type"] == "pipeline-start" {
							info.Agent, _ = ev["agent"].(string)
						}
						if ev["type"] == "pipeline-end" {
							info.Status, _ = ev["status"].(string)
							if d, ok := ev["duration-ms"].(float64); ok {
								info.Duration = fmt.Sprintf("%.1fs", d/1000)
							}
						}
					}
				}
				f.Close()
			}
			if info.Status == "" {
				info.Status = "unknown"
			}
			runs = append(runs, info)
		}

		sort.Slice(runs, func(i, j int) bool {
			return runs[i].ModTime.After(runs[j].ModTime)
		})

		if len(runs) == 0 {
			fmt.Println("No runs found.")
			return nil
		}

		fmt.Printf("%-25s %-20s %-10s %s\n", "RUN ID", "AGENT", "STATUS", "DURATION")
		for _, r := range runs {
			fmt.Printf("%-25s %-20s %-10s %s\n", r.ID, r.Agent, r.Status, r.Duration)
		}
		return nil
	},
}

var runsInspectCmd = &cobra.Command{
	Use:   "inspect <run-id>",
	Short: "Show details of a run",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, _ := os.Getwd()
		runDir := filepath.Join(dir, ".ctx", "runs", args[0])

		// Read state
		stateData, err := os.ReadFile(filepath.Join(runDir, "state.json"))
		if err != nil {
			return fmt.Errorf("run %q not found", args[0])
		}

		fmt.Printf("=== Run: %s ===\n\n", args[0])

		// Read and display timeline from log.json
		logPath := filepath.Join(runDir, "log.json")
		if f, err := os.Open(logPath); err == nil {
			fmt.Println("Timeline:")
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				var ev map[string]any
				if json.Unmarshal(scanner.Bytes(), &ev) == nil {
					evType, _ := ev["type"].(string)
					step, _ := ev["step"].(string)
					status, _ := ev["status"].(string)
					dur, _ := ev["duration-ms"].(float64)

					switch evType {
					case "pipeline-start":
						agent, _ := ev["agent"].(string)
						goal, _ := ev["goal"].(string)
						fmt.Printf("  [start] agent=%s goal=%q\n", agent, goal)
					case "step-start":
						stepType, _ := ev["step-type"].(string)
						fmt.Printf("  [step]  %s (%s)\n", step, stepType)
					case "step-end":
						fmt.Printf("  [done]  %s status=%s duration=%.1fs\n", step, status, dur/1000)
					case "loop-enter":
						pred, _ := ev["predicate"].(string)
						iter, _ := ev["iteration"].(float64)
						fmt.Printf("  [loop]  %s iteration=%d\n", pred, int(iter))
					case "loop-exit":
						pred, _ := ev["predicate"].(string)
						reason, _ := ev["reason"].(string)
						fmt.Printf("  [exit]  %s reason=%s\n", pred, reason)
					case "error-retry":
						attempt, _ := ev["attempt"].(float64)
						fmt.Printf("  [retry] %s attempt=%d\n", step, int(attempt))
					case "pipeline-end":
						fmt.Printf("  [end]   status=%s duration=%.1fs\n", status, dur/1000)
					}
				}
			}
			f.Close()
			fmt.Println()
		}

		// Display final state
		fmt.Println("Final State:")
		var state map[string]any
		json.Unmarshal(stateData, &state)
		for k, v := range state {
			valStr, _ := json.Marshal(v)
			s := string(valStr)
			if len(s) > 100 {
				s = s[:97] + "..."
			}
			fmt.Printf("  %s: %s\n", k, s)
		}

		return nil
	},
}

var runsCleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Delete old runs",
	RunE: func(cmd *cobra.Command, args []string) error {
		keep, _ := cmd.Flags().GetInt("keep")
		dir, _ := os.Getwd()
		runsDir := filepath.Join(dir, ".ctx", "runs")
		entries, err := os.ReadDir(runsDir)
		if err != nil {
			return nil
		}

		var dirs []os.DirEntry
		for _, e := range entries {
			if e.IsDir() && e.Name() != "current" {
				dirs = append(dirs, e)
			}
		}

		sort.Slice(dirs, func(i, j int) bool {
			fi, _ := dirs[i].Info()
			fj, _ := dirs[j].Info()
			if fi == nil || fj == nil {
				return false
			}
			return fi.ModTime().After(fj.ModTime())
		})

		removed := 0
		for i := keep; i < len(dirs); i++ {
			path := filepath.Join(runsDir, dirs[i].Name())
			if err := os.RemoveAll(path); err == nil {
				removed++
			}
		}
		fmt.Printf("Removed %d run(s), kept %d\n", removed, min(keep, len(dirs)))
		return nil
	},
}

var runsWatchCmd = &cobra.Command{
	Use:   "watch <run-id>",
	Short: "Tail events from a run",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, _ := os.Getwd()
		logPath := filepath.Join(dir, ".ctx", "runs", args[0], "log.json")

		f, err := os.Open(logPath)
		if err != nil {
			return fmt.Errorf("run %q not found", args[0])
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			var ev map[string]any
			if json.Unmarshal(scanner.Bytes(), &ev) == nil {
				evType, _ := ev["type"].(string)
				step, _ := ev["step"].(string)
				ts, _ := ev["timestamp"].(string)

				compact := fmt.Sprintf("[%s] %s", evType, step)
				if ts != "" {
					if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
						compact = t.Format("15:04:05") + " " + compact
					}
				}
				fmt.Println(compact)
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(runsCmd)
	runsCmd.AddCommand(runsListCmd)
	runsCmd.AddCommand(runsInspectCmd)
	runsCleanCmd.Flags().Int("keep", 5, "number of recent runs to keep")
	runsCmd.AddCommand(runsCleanCmd)
	runsCmd.AddCommand(runsWatchCmd)
}

