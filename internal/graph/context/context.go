package context

import (
	"fmt"
	"strings"
)

// ContextMap holds ranked symbols relevant to a task.
type ContextMap struct {
	Task    string
	Symbols []SymbolEntry
}

// SymbolEntry is a single relevant symbol with location and relationships.
type SymbolEntry struct {
	Name      string
	Kind      string   // function, method, type, file
	Path      string
	Line      int
	Score     float64  // internal ranking score
	Relations []string // e.g. "calls CheckPassword"
}

// Format renders the context map as a markdown section for prompt injection.
// Returns empty string if there are no symbols.
func (cm *ContextMap) Format() string {
	if cm == nil || len(cm.Symbols) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Relevant Context\n\n")
	b.WriteString("The following symbols are relevant to your current task. Start here.\n\n")

	for _, s := range cm.Symbols {
		b.WriteString(fmt.Sprintf("- `%s` %s (%s:%d)", s.Name, s.Kind, s.Path, s.Line))
		if len(s.Relations) > 0 {
			b.WriteString(" — ")
			b.WriteString(strings.Join(s.Relations, ", "))
		}
		b.WriteString("\n")
	}

	return b.String()
}
