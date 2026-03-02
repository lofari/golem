package tui

import (
	"testing"
)

func TestLineWriter_CompleteLine(t *testing.T) {
	ch := make(chan string, 10)
	w := NewLineWriter(ch)

	n, err := w.Write([]byte("hello world\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 12 {
		t.Errorf("expected n=12, got %d", n)
	}

	select {
	case line := <-ch:
		if line != "hello world" {
			t.Errorf("expected %q, got %q", "hello world", line)
		}
	default:
		t.Error("expected a line on the channel")
	}
}

func TestLineWriter_MultipleLines(t *testing.T) {
	ch := make(chan string, 10)
	w := NewLineWriter(ch)

	w.Write([]byte("line1\nline2\nline3\n"))

	expected := []string{"line1", "line2", "line3"}
	for _, exp := range expected {
		select {
		case line := <-ch:
			if line != exp {
				t.Errorf("expected %q, got %q", exp, line)
			}
		default:
			t.Errorf("expected line %q on channel", exp)
		}
	}
}

func TestLineWriter_PartialLine(t *testing.T) {
	ch := make(chan string, 10)
	w := NewLineWriter(ch)

	// Write partial line
	w.Write([]byte("hel"))
	select {
	case <-ch:
		t.Error("should not send partial line")
	default:
	}

	// Complete the line
	w.Write([]byte("lo\n"))
	select {
	case line := <-ch:
		if line != "hello" {
			t.Errorf("expected %q, got %q", "hello", line)
		}
	default:
		t.Error("expected completed line on channel")
	}
}

func TestLineWriter_Flush(t *testing.T) {
	ch := make(chan string, 10)
	w := NewLineWriter(ch)

	w.Write([]byte("no newline"))
	w.Flush()

	select {
	case line := <-ch:
		if line != "no newline" {
			t.Errorf("expected %q, got %q", "no newline", line)
		}
	default:
		t.Error("expected flushed line on channel")
	}
}
