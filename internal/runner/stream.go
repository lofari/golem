// internal/runner/stream.go
package runner

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// StreamParser reads stream-json lines from Claude CLI, extracts displayable
// text for the TUI, and accumulates the full response for completion detection.
type StreamParser struct {
	display  io.Writer       // receives human-readable lines for TUI
	text     strings.Builder // accumulates full assistant text
	debugLog *os.File        // optional: raw stream dump for debugging
}

func NewStreamParser(display io.Writer) *StreamParser {
	return &StreamParser{display: display}
}

// EnableDebugLog writes raw stream-json to .ctx/sessions/stream-debug.jsonl
func (sp *StreamParser) EnableDebugLog(dir string) {
	sessDir := filepath.Join(dir, ".ctx", "sessions")
	os.MkdirAll(sessDir, 0755)
	f, err := os.Create(filepath.Join(sessDir, "stream-debug.jsonl"))
	if err == nil {
		sp.debugLog = f
	}
}

// Close cleans up the debug log if open.
func (sp *StreamParser) Close() {
	if sp.debugLog != nil {
		sp.debugLog.Close()
	}
}

// Text returns the accumulated assistant response text.
func (sp *StreamParser) Text() string {
	return sp.text.String()
}

// Parse reads stream-json lines from r and processes them.
func (sp *StreamParser) Parse(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Write raw line to debug log
		if sp.debugLog != nil {
			fmt.Fprintln(sp.debugLog, line)
		}

		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			// Not JSON — show as-is
			fmt.Fprintln(sp.display, line)
			continue
		}

		var eventType string
		if t, ok := raw["type"]; ok {
			json.Unmarshal(t, &eventType)
		}

		switch eventType {
		case "assistant":
			sp.handleAssistantMsg(raw)

		case "content_block_start":
			sp.handleContentBlockStart(raw)

		case "content_block_delta":
			sp.handleContentBlockDelta(raw)

		case "tool_use":
			sp.handleToolUse(raw)

		case "result":
			sp.handleResultMsg(raw)

		case "system", "ping", "message_start", "message_delta", "message_stop",
			"content_block_stop", "user", "tool_result", "rate_limit_event":
			// Skip silently

		default:
			// Skip unknown events silently
		}
	}
}

// CLI format: {"type":"assistant","message":{"content":[{"type":"text","text":"..."}]}}
func (sp *StreamParser) handleAssistantMsg(raw map[string]json.RawMessage) {
	msgRaw, ok := raw["message"]
	if !ok {
		return
	}

	var msg struct {
		Content []struct {
			Type  string                 `json:"type"`
			Text  string                 `json:"text"`
			Name  string                 `json:"name"`
			Input map[string]interface{} `json:"input"`
		} `json:"content"`
	}
	if err := json.Unmarshal(msgRaw, &msg); err != nil {
		return
	}

	for _, block := range msg.Content {
		switch block.Type {
		case "text":
			if block.Text != "" {
				sp.text.WriteString(block.Text)
				fmt.Fprintln(sp.display, block.Text)
			}
		case "tool_use":
			fmt.Fprintln(sp.display, formatToolCall(block.Name, block.Input))
		}
	}
}

// formatToolCall produces a concise one-liner for a tool invocation.
func formatToolCall(name string, input map[string]interface{}) string {
	switch name {
	case "Read":
		return fmt.Sprintf("  Read %s", shortPath(strVal(input, "file_path")))
	case "Write":
		return fmt.Sprintf("  Write %s", shortPath(strVal(input, "file_path")))
	case "Edit":
		return fmt.Sprintf("  Edit %s", shortPath(strVal(input, "file_path")))
	case "Glob":
		return fmt.Sprintf("  Glob %s", strVal(input, "pattern"))
	case "Grep":
		p := strVal(input, "pattern")
		path := strVal(input, "path")
		if path != "" {
			return fmt.Sprintf("  Grep %q in %s", p, shortPath(path))
		}
		return fmt.Sprintf("  Grep %q", p)
	case "Bash":
		cmd := strVal(input, "command")
		if len(cmd) > 80 {
			cmd = cmd[:77] + "..."
		}
		return fmt.Sprintf("  $ %s", cmd)
	case "Agent":
		desc := strVal(input, "description")
		kind := strVal(input, "subagent_type")
		if desc != "" {
			return fmt.Sprintf("  Agent(%s) %s", kind, desc)
		}
		return fmt.Sprintf("  Agent(%s)", kind)
	case "Skill":
		return fmt.Sprintf("  Skill %s", strVal(input, "skill"))
	case "TodoWrite":
		return "  TodoWrite"
	default:
		return fmt.Sprintf("  %s", name)
	}
}

func strVal(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// shortPath trims common home directory prefix.
func shortPath(p string) string {
	home, _ := os.UserHomeDir()
	if home != "" && strings.HasPrefix(p, home) {
		return "~" + p[len(home):]
	}
	return p
}

// API format: {"type":"content_block_start","content_block":{"type":"tool_use","name":"Read"}}
func (sp *StreamParser) handleContentBlockStart(raw map[string]json.RawMessage) {
	if blockRaw, ok := raw["content_block"]; ok {
		var block struct {
			Type  string                 `json:"type"`
			Name  string                 `json:"name"`
			Input map[string]interface{} `json:"input"`
		}
		if err := json.Unmarshal(blockRaw, &block); err == nil {
			if block.Type == "tool_use" && block.Name != "" {
				fmt.Fprintln(sp.display, formatToolCall(block.Name, block.Input))
			}
		}
	}
}

// API format: {"type":"content_block_delta","delta":{"type":"text_delta","text":"..."}}
func (sp *StreamParser) handleContentBlockDelta(raw map[string]json.RawMessage) {
	if deltaRaw, ok := raw["delta"]; ok {
		var delta struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(deltaRaw, &delta); err == nil {
			if delta.Type == "text_delta" && delta.Text != "" {
				sp.text.WriteString(delta.Text)
				fmt.Fprint(sp.display, delta.Text)
			}
		}
	}
}

// CLI format: {"type":"tool_use","tool":{"name":"Read","input":{...}}}
func (sp *StreamParser) handleToolUse(raw map[string]json.RawMessage) {
	if toolRaw, ok := raw["tool"]; ok {
		var tool struct {
			Name  string                 `json:"name"`
			Input map[string]interface{} `json:"input"`
		}
		if err := json.Unmarshal(toolRaw, &tool); err == nil && tool.Name != "" {
			fmt.Fprintln(sp.display, formatToolCall(tool.Name, tool.Input))
		}
	}
}

// CLI format: {"type":"result","result":"response text"}
func (sp *StreamParser) handleResultMsg(raw map[string]json.RawMessage) {
	if resultRaw, ok := raw["result"]; ok {
		var result string
		if err := json.Unmarshal(resultRaw, &result); err == nil && result != "" {
			sp.text.WriteString(result)
			fmt.Fprintln(sp.display, result)
		}
	}
}
