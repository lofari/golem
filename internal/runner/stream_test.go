package runner

import (
	"strings"
	"testing"
)

func TestStreamParser_BashCallback(t *testing.T) {
	var commands []string
	var results []struct {
		exitCode int
		stdout   string
	}

	parser := NewStreamParser(&strings.Builder{})
	parser.OnBashCommand = func(command, workingDir string) {
		commands = append(commands, command)
	}
	parser.OnBashResult = func(exitCode int, stdout, stderr string) {
		results = append(results, struct {
			exitCode int
			stdout   string
		}{exitCode, stdout})
	}

	// Simulate stream-json with a bash tool use and result
	input := `{"type":"tool_use","tool":{"name":"Bash","input":{"command":"echo hello"}}}
{"type":"tool_result","tool":{"name":"Bash"},"content":"hello\n","exit_code":0}
`
	parser.Parse(strings.NewReader(input))

	if len(commands) != 1 || commands[0] != "echo hello" {
		t.Fatalf("expected 'echo hello', got %v", commands)
	}
	if len(results) != 1 || results[0].exitCode != 0 {
		t.Fatalf("unexpected result: %v", results)
	}
}
