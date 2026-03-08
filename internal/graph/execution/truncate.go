package execution

import (
	"fmt"
	"strings"
)

// TruncateOutput keeps the first N and last N lines of text.
// Returns the truncated text and whether truncation occurred.
func TruncateOutput(text string, keepLines int) (string, bool) {
	lines := strings.Split(text, "\n")
	total := len(lines)

	if total <= keepLines*2 {
		return text, false
	}

	head := lines[:keepLines]
	tail := lines[total-keepLines:]
	omitted := total - keepLines*2

	result := strings.Join(head, "\n") +
		fmt.Sprintf("\n... [%d lines truncated] ...\n", omitted) +
		strings.Join(tail, "\n")

	return result, true
}
