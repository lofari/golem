package tui

import (
	"bytes"
	"strings"
)

// LineWriter is an io.Writer that buffers input and sends complete lines
// (stripped of trailing newline) to a channel.
type LineWriter struct {
	ch  chan<- string
	buf bytes.Buffer
}

// NewLineWriter creates a LineWriter that sends lines to ch.
func NewLineWriter(ch chan<- string) *LineWriter {
	return &LineWriter{ch: ch}
}

func (w *LineWriter) Write(p []byte) (int, error) {
	n := len(p)
	w.buf.Write(p)

	for {
		line, err := w.buf.ReadString('\n')
		if err != nil {
			// Incomplete line — put it back
			w.buf.WriteString(line)
			break
		}
		w.ch <- strings.TrimRight(line, "\n\r")
	}

	return n, nil
}

// Flush sends any remaining buffered text as a final line.
func (w *LineWriter) Flush() {
	if w.buf.Len() > 0 {
		w.ch <- w.buf.String()
		w.buf.Reset()
	}
}
