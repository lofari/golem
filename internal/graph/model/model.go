package model

// Node represents a code entity in the graph.
type Node struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Name string `json:"name"`
	Path string `json:"path,omitempty"`
	Line int    `json:"line,omitempty"`
}

// Edge represents a relationship between two nodes.
type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"`
}

// Stats holds graph statistics.
type Stats struct {
	TotalNodes int            `json:"totalNodes"`
	TotalEdges int            `json:"totalEdges"`
	NodeTypes  map[string]int `json:"nodeTypes"`
	EdgeTypes  map[string]int `json:"edgeTypes"`
}

// CoChangedResult represents a file that co-changed with another file.
type CoChangedResult struct {
	File  string `json:"file"`
	Count int    `json:"count"`
}

// Execution represents an agent session (golem code/review/qa run).
type Execution struct {
	SessionID string `json:"sessionId"`
	StartedAt int64  `json:"startedAt"`
	EndedAt   int64  `json:"endedAt,omitempty"`
	Status    string `json:"status"` // running, completed, failed
}

// Command represents a shell command executed during a session.
type Command struct {
	ID         string `json:"id"`        // cmd:{sessionId}:{seq}
	SessionID  string `json:"sessionId"`
	Seq        int    `json:"seq"`
	Command    string `json:"command"`
	ExitCode   int    `json:"exitCode"`
	WorkingDir string `json:"workingDir,omitempty"`
}

// Output holds stdout/stderr for a command.
type Output struct {
	CommandID string `json:"commandId"`
	Stdout    string `json:"stdout,omitempty"`
	Stderr    string `json:"stderr,omitempty"`
	Truncated bool   `json:"truncated"`
}

// TestResult represents a parsed test outcome.
type TestResult struct {
	ID         string `json:"id"`        // test:{sessionId}:{name}
	SessionID  string `json:"sessionId"`
	Name       string `json:"name"`
	Passed     bool   `json:"passed"`
	DurationMs int    `json:"durationMs,omitempty"`
	Output     string `json:"output,omitempty"`
}

// Error represents a runtime error extracted from command output.
type Error struct {
	ID         string `json:"id"`        // err:{sessionId}:{seq}
	CommandID  string `json:"commandId"`
	Message    string `json:"message"`
	StackTrace string `json:"stackTrace,omitempty"`
}
