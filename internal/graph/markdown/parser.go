package markdown

import (
	"regexp"
	"strings"
)

// DocSection represents a section of a markdown document.
type DocSection struct {
	Heading string   // Section heading text
	Level   int      // H1=1, H2=2, etc.
	Line    int      // 1-indexed line number
	Body    string   // Section content (between this heading and next)
	Refs    []string // Backtick-quoted identifiers found in body
}

var (
	headingRe  = regexp.MustCompile(`^(#{1,6})\s+(.+)$`)
	backtickRe = regexp.MustCompile("`([^`]+)`")
)

// ParseMarkdown splits a markdown file into sections at heading boundaries.
// Each section captures its heading, level, body text, and backtick-quoted references.
func ParseMarkdown(path string, content []byte) ([]DocSection, error) {
	lines := strings.Split(string(content), "\n")

	var sections []DocSection
	var currentBody strings.Builder
	currentIdx := -1

	for i, line := range lines {
		if m := headingRe.FindStringSubmatch(line); m != nil {
			// Close previous section
			if currentIdx >= 0 {
				body := strings.TrimSpace(currentBody.String())
				sections[currentIdx].Body = body
				sections[currentIdx].Refs = extractRefs(body)
			}

			// Start new section
			sections = append(sections, DocSection{
				Heading: strings.TrimSpace(m[2]),
				Level:   len(m[1]),
				Line:    i + 1, // 1-indexed
			})
			currentIdx = len(sections) - 1
			currentBody.Reset()
		} else if currentIdx >= 0 {
			currentBody.WriteString(line)
			currentBody.WriteString("\n")
		}
	}

	// Close last section
	if currentIdx >= 0 {
		body := strings.TrimSpace(currentBody.String())
		sections[currentIdx].Body = body
		sections[currentIdx].Refs = extractRefs(body)
	}

	return sections, nil
}

// extractRefs finds all backtick-quoted identifiers in text.
// Filters out common non-code references (shell commands, paths, etc.)
func extractRefs(text string) []string {
	matches := backtickRe.FindAllStringSubmatch(text, -1)
	seen := map[string]bool{}
	var refs []string
	for _, m := range matches {
		ref := m[1]
		// Skip things that look like shell commands or paths
		if strings.ContainsAny(ref, " /\\") {
			continue
		}
		if !seen[ref] {
			seen[ref] = true
			refs = append(refs, ref)
		}
	}
	return refs
}
