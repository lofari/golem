package execution

import (
	"fmt"
	"strings"
	"testing"
)

func TestTruncateOutput_Short(t *testing.T) {
	text := "line1\nline2\nline3"
	result, truncated := TruncateOutput(text, 50)
	if truncated {
		t.Fatal("should not truncate short output")
	}
	if result != text {
		t.Fatalf("expected original text, got %q", result)
	}
}

func TestTruncateOutput_Long(t *testing.T) {
	var lines []string
	for i := 1; i <= 200; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	text := strings.Join(lines, "\n")

	result, truncated := TruncateOutput(text, 50)
	if !truncated {
		t.Fatal("should truncate long output")
	}

	resultLines := strings.Split(result, "\n")
	// Should have 50 + 1 (separator) + 50 = 101 lines
	if len(resultLines) != 101 {
		t.Fatalf("expected 101 lines, got %d", len(resultLines))
	}
	if resultLines[0] != "line 1" {
		t.Fatalf("first line should be 'line 1', got %q", resultLines[0])
	}
	if resultLines[100] != "line 200" {
		t.Fatalf("last line should be 'line 200', got %q", resultLines[100])
	}
	// Check separator
	if !strings.Contains(resultLines[50], "truncated") {
		t.Fatalf("expected truncation marker, got %q", resultLines[50])
	}
}

func TestTruncateOutput_ExactBoundary(t *testing.T) {
	var lines []string
	for i := 1; i <= 100; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	text := strings.Join(lines, "\n")

	_, truncated := TruncateOutput(text, 50)
	if truncated {
		t.Fatal("100 lines with limit 50 should not truncate (50+50 = 100)")
	}
}
