package execution

import (
	"testing"
)

func TestExtractFilePaths(t *testing.T) {
	// Known project files
	known := map[string]bool{
		"internal/runner/builder.go": true,
		"cmd/graph.go":              true,
		"main.go":                   true,
	}

	text := `go test ./internal/runner/
--- FAIL: TestBuilder (0.01s)
    builder_test.go:42: assertion failed
FAIL	internal/runner	0.015s
Error in cmd/graph.go:15`

	paths := ExtractFilePaths(text, known)
	if len(paths) == 0 {
		t.Fatal("expected file paths")
	}

	found := make(map[string]bool)
	for _, p := range paths {
		found[p] = true
	}
	if !found["cmd/graph.go"] {
		t.Error("expected cmd/graph.go")
	}
}

func TestExtractGoTestResults(t *testing.T) {
	output := `=== RUN   TestFoo
--- PASS: TestFoo (0.01s)
=== RUN   TestBar
--- FAIL: TestBar (0.05s)
    bar_test.go:10: expected 1, got 2
=== RUN   TestBaz/subtest
--- PASS: TestBaz/subtest (0.00s)
FAIL
exit status 1`

	results := ExtractGoTestResults(output)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	byName := make(map[string]bool)
	for _, r := range results {
		byName[r.Name] = r.Passed
	}
	if !byName["TestFoo"] {
		t.Error("TestFoo should pass")
	}
	if byName["TestBar"] {
		t.Error("TestBar should fail")
	}
	if !byName["TestBaz/subtest"] {
		t.Error("TestBaz/subtest should pass")
	}
}

func TestExtractGoStackTrace(t *testing.T) {
	output := `goroutine 1 [running]:
main.main()
	/home/user/project/main.go:42 +0x1a4
github.com/foo/bar.Init()
	/home/user/project/internal/runner/builder.go:100 +0x84`

	files := ExtractGoStackFiles(output)
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}

	found := make(map[string]bool)
	for _, f := range files {
		found[f.Path] = true
	}
	if !found["main.go"] {
		t.Error("expected main.go")
	}
	if !found["internal/runner/builder.go"] {
		t.Error("expected internal/runner/builder.go")
	}
}

func TestExtractPythonTracebackFiles(t *testing.T) {
	output := `Traceback (most recent call last):
  File "src/app.py", line 42, in main
    result = process()
  File "src/utils/helper.py", line 10, in process
    raise ValueError("bad")`

	files := ExtractPythonTracebackFiles(output)
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
}
