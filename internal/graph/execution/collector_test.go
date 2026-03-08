package execution

import (
	"path/filepath"
	"testing"

	"github.com/lofari/golem/internal/graph"
)

func TestCollector_BasicFlow(t *testing.T) {
	dir := t.TempDir()
	store, err := graph.OpenStore(filepath.Join(dir, "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Insert a known file node so ACCESSES edges can be created
	store.InsertBatch(
		[]graph.Node{{ID: "file:main.go", Type: "file", Name: "main.go", Path: "main.go"}},
		nil,
	)

	c := NewCollector(store, "test-session")
	c.Start()

	// Simulate a bash command
	c.OnBashCommand("go build ./...", "")
	c.OnBashResult(0, "build successful\n", "")

	// Simulate a failing command referencing main.go
	c.OnBashCommand("go test ./...", "")
	c.OnBashResult(1, "--- FAIL: TestMain (0.01s)\n    main.go:42: error\nFAIL", "exit status 1")

	c.Finish("completed")

	// Verify execution was created
	exec, err := store.LatestExecution()
	if err != nil {
		t.Fatal(err)
	}
	if exec.SessionID != "test-session" {
		t.Fatalf("expected test-session, got %s", exec.SessionID)
	}
	if exec.Status != "completed" {
		t.Fatalf("expected completed, got %s", exec.Status)
	}

	// Verify commands
	cmds, _ := store.QueryCommandsBySession("test-session")
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(cmds))
	}
	if cmds[0].Command != "go build ./..." {
		t.Fatalf("unexpected command: %s", cmds[0].Command)
	}
	if cmds[1].ExitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", cmds[1].ExitCode)
	}

	// Verify test results were extracted
	tests, _ := store.QueryTestResults("test-session", "")
	if len(tests) != 1 {
		t.Fatalf("expected 1 test result, got %d", len(tests))
	}
	if tests[0].Name != "TestMain" {
		t.Fatalf("expected TestMain, got %s", tests[0].Name)
	}
	if tests[0].Passed {
		t.Fatal("expected TestMain to fail")
	}

	// Verify error was created for failing command
	errors, _ := store.QueryErrorsBySession("test-session")
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errors))
	}

	// Verify ACCESSES edge was created (main.go was referenced in output)
	edges, _ := store.EdgesFrom(cmds[1].ID)
	foundAccess := false
	for _, e := range edges {
		if e.Type == "ACCESSES" && e.To == "file:main.go" {
			foundAccess = true
		}
	}
	if !foundAccess {
		t.Fatal("expected ACCESSES edge from command to file:main.go")
	}
}

func TestCollector_OutputTruncation(t *testing.T) {
	dir := t.TempDir()
	store, err := graph.OpenStore(filepath.Join(dir, "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	c := NewCollector(store, "trunc-session")
	c.Start()

	// Generate long output (200 lines)
	var longOutput string
	for i := 0; i < 200; i++ {
		longOutput += "output line\n"
	}

	c.OnBashCommand("echo lots", "")
	c.OnBashResult(0, longOutput, "")
	c.Finish("completed")

	// Verify output was truncated
	cmds, _ := store.QueryCommandsBySession("trunc-session")
	out, _ := store.QueryOutput(cmds[0].ID)
	if !out.Truncated {
		t.Fatal("expected output to be truncated")
	}
}
