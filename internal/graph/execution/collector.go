package execution

import (
	"fmt"
	"strings"
	"time"

	"github.com/lofari/golem/internal/graph"
	"github.com/lofari/golem/internal/graph/model"
)

const defaultKeepLines = 50

// Collector receives bash tool-use events and populates the execution graph.
type Collector struct {
	store      *graph.Store
	sessionID  string
	seq        int
	pendingCmd string
	pendingDir string
	knownFiles map[string]bool // cache of known file paths in graph
}

// NewCollector creates a new execution collector for the given session.
func NewCollector(store *graph.Store, sessionID string) *Collector {
	return &Collector{
		store:     store,
		sessionID: sessionID,
	}
}

// Start initializes the execution session.
func (c *Collector) Start() {
	c.loadKnownFiles()
	c.store.InsertExecution(model.Execution{
		SessionID: c.sessionID,
		StartedAt: time.Now().Unix(),
		Status:    "running",
	})
}

// OnBashCommand records a new bash command being executed.
func (c *Collector) OnBashCommand(command, workingDir string) {
	c.seq++
	c.pendingCmd = command
	c.pendingDir = workingDir
}

// OnBashResult records the result of the pending bash command.
func (c *Collector) OnBashResult(exitCode int, stdout, stderr string) {
	if c.pendingCmd == "" {
		return
	}

	cmdID := fmt.Sprintf("cmd:%s:%d", c.sessionID, c.seq)

	// Store command
	c.store.InsertCommand(model.Command{
		ID:         cmdID,
		SessionID:  c.sessionID,
		Seq:        c.seq,
		Command:    c.pendingCmd,
		ExitCode:   exitCode,
		WorkingDir: c.pendingDir,
	})

	// Store output (truncated for success, full for failure)
	truncatedStdout := stdout
	truncated := false
	if exitCode == 0 {
		truncatedStdout, truncated = TruncateOutput(stdout, defaultKeepLines)
	}
	c.store.InsertOutput(model.Output{
		CommandID: cmdID,
		Stdout:    truncatedStdout,
		Stderr:    stderr,
		Truncated: truncated,
	})

	// Extract references from combined command + output
	combined := c.pendingCmd + "\n" + stdout + "\n" + stderr
	c.extractReferences(cmdID, combined, exitCode)

	// Create error node for failures
	if exitCode != 0 {
		errMsg := extractErrorMessage(stderr, stdout)
		stackTrace := ""
		stackFiles := ExtractGoStackFiles(combined)
		if len(stackFiles) > 0 {
			stackTrace = extractStackTraceText(combined)
		}

		errID := fmt.Sprintf("err:%s:%d", c.sessionID, c.seq)
		c.store.InsertError(model.Error{
			ID:         errID,
			CommandID:  cmdID,
			Message:    errMsg,
			StackTrace: stackTrace,
		})

		// Create FAILS_IN edges for stack trace files
		var edges []graph.Edge
		for _, f := range stackFiles {
			fileNodeID := "file:" + f.Path
			if c.knownFiles[f.Path] {
				edges = append(edges, graph.Edge{From: errID, To: fileNodeID, Type: "FAILS_IN"})
			}
		}
		if len(edges) > 0 {
			c.store.InsertBatch(nil, edges)
		}
	}

	c.pendingCmd = ""
	c.pendingDir = ""
}

// Finish finalizes the execution session.
func (c *Collector) Finish(status string) {
	c.store.FinalizeExecution(c.sessionID, time.Now().Unix(), status)
}

func (c *Collector) extractReferences(cmdID, text string, exitCode int) {
	// Extract file path references -> ACCESSES edges
	filePaths := ExtractFilePaths(text, c.knownFiles)
	var edges []graph.Edge
	for _, p := range filePaths {
		edges = append(edges, graph.Edge{From: cmdID, To: "file:" + p, Type: "ACCESSES"})
	}

	// Extract Go test results
	testResults := ExtractGoTestResults(text)
	for _, tr := range testResults {
		testID := fmt.Sprintf("test:%s:%s", c.sessionID, tr.Name)
		c.store.InsertTestResult(model.TestResult{
			ID:         testID,
			SessionID:  c.sessionID,
			Name:       tr.Name,
			Passed:     tr.Passed,
			DurationMs: tr.DurationMs,
		})

		// Try to link test to function node (best-effort)
		fnName := strings.TrimPrefix(tr.Name, "Test")
		if idx := strings.Index(fnName, "/"); idx > 0 {
			fnName = fnName[:idx]
		}
		if fnName != "" {
			fnNodes, _ := c.store.FindNodesByName(fnName)
			for _, n := range fnNodes {
				if n.Type == "function" || n.Type == "method" {
					edges = append(edges, graph.Edge{From: testID, To: n.ID, Type: "TESTS"})
				}
			}
		}

		// PRODUCES edge from command to test result
		edges = append(edges, graph.Edge{From: cmdID, To: testID, Type: "PRODUCES"})
	}

	// Extract Python traceback files -> FAILS_IN edges
	if exitCode != 0 {
		pyFiles := ExtractPythonTracebackFiles(text)
		for _, f := range pyFiles {
			if c.knownFiles[f.Path] {
				errID := fmt.Sprintf("err:%s:%d", c.sessionID, c.seq)
				edges = append(edges, graph.Edge{From: errID, To: "file:" + f.Path, Type: "FAILS_IN"})
			}
		}
	}

	if len(edges) > 0 {
		c.store.InsertBatch(nil, edges)
	}
}

func (c *Collector) loadKnownFiles() {
	c.knownFiles = make(map[string]bool)
	fileNodes, err := c.store.NodesByType("file")
	if err != nil {
		return
	}
	for _, n := range fileNodes {
		c.knownFiles[n.Path] = true
	}
}

// extractErrorMessage builds an error message from stderr (or stdout if stderr is empty).
func extractErrorMessage(stderr, stdout string) string {
	source := stderr
	if strings.TrimSpace(source) == "" {
		source = stdout
	}
	lines := strings.Split(strings.TrimSpace(source), "\n")
	if len(lines) > 10 {
		lines = lines[len(lines)-10:]
	}
	return strings.Join(lines, "\n")
}

// extractStackTraceText extracts the raw stack trace portion from output.
func extractStackTraceText(text string) string {
	idx := strings.Index(text, "goroutine ")
	if idx < 0 {
		return ""
	}
	rest := text[idx:]
	lines := strings.Split(rest, "\n")
	if len(lines) > 50 {
		lines = lines[:50]
	}
	return strings.Join(lines, "\n")
}
