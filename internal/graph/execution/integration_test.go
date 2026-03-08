package execution

import (
	"path/filepath"
	"testing"

	"github.com/lofari/golem/internal/graph"
)

func TestIntegration_CollectorToStore(t *testing.T) {
	dir := t.TempDir()
	store, err := graph.OpenStore(filepath.Join(dir, "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Seed with file nodes
	store.InsertBatch(
		[]graph.Node{
			{ID: "file:main.go", Type: "file", Name: "main.go", Path: "main.go"},
			{ID: "fn:main.go:Foo", Type: "function", Name: "Foo", Path: "main.go", Line: 10},
		},
		nil,
	)

	// Set up collector and simulate a session
	collector := NewCollector(store, "integration-test")
	collector.Start()

	// Simulate bash command + result (same flow as StreamParser callbacks)
	collector.OnBashCommand("go test ./...", "")
	collector.OnBashResult(1, "--- FAIL: TestFoo (0.01s)\n    main.go:10: assertion failed\nFAIL", "")

	collector.Finish("completed")

	// Verify execution session
	exec, _ := store.LatestExecution()
	if exec == nil || exec.Status != "completed" {
		t.Fatal("expected completed execution")
	}

	// Verify command captured
	cmds, _ := store.QueryCommandsBySession("integration-test")
	if len(cmds) != 1 || cmds[0].Command != "go test ./..." {
		t.Fatalf("unexpected commands: %v", cmds)
	}

	// Verify test result extracted
	tests, _ := store.QueryTestResults("integration-test", "")
	if len(tests) != 1 || tests[0].Name != "TestFoo" || tests[0].Passed {
		t.Fatalf("unexpected test results: %v", tests)
	}

	// Verify TESTS edge to Foo function
	edges, _ := store.EdgesFrom(tests[0].ID)
	foundTests := false
	for _, e := range edges {
		if e.Type == "TESTS" && e.To == "fn:main.go:Foo" {
			foundTests = true
		}
	}
	if !foundTests {
		t.Fatal("expected TESTS edge from test result to Foo function")
	}

	// Verify ACCESSES edge from command to file
	cmdEdges, _ := store.EdgesFrom(cmds[0].ID)
	foundAccess := false
	for _, e := range cmdEdges {
		if e.Type == "ACCESSES" && e.To == "file:main.go" {
			foundAccess = true
		}
	}
	if !foundAccess {
		t.Fatal("expected ACCESSES edge from command to file:main.go")
	}
}
