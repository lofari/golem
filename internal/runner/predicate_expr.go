package runner

import (
	"fmt"
	"strconv"
	"strings"
)

// PredicateExpr represents a parsed predicate expression: path op value.
type PredicateExpr struct {
	Path     string   // dotted path, e.g. "review-feedback.verdict"
	Op       string   // ==, !=, >, <, >=, <=
	Value    any      // string, float64, or bool
	IsConfig bool     // true if path starts with "config."
	Segments []string // path split by "."
}

var validOps = map[string]bool{
	"==": true, "!=": true,
	">": true, "<": true,
	">=": true, "<=": true,
}

// ParsePredicateExpr parses an expression like: path.to.key == "value"
func ParsePredicateExpr(expr string) (*PredicateExpr, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, fmt.Errorf("empty predicate expression")
	}

	var op string
	var opIdx int
	for _, candidate := range []string{">=", "<=", "!=", "==", ">", "<"} {
		idx := strings.Index(expr, candidate)
		if idx > 0 {
			op = candidate
			opIdx = idx
			break
		}
	}
	if op == "" {
		return nil, fmt.Errorf("no operator found in %q (expected ==, !=, >, <, >=, <=)", expr)
	}

	path := strings.TrimSpace(expr[:opIdx])
	rawVal := strings.TrimSpace(expr[opIdx+len(op):])

	if path == "" {
		return nil, fmt.Errorf("missing path in %q", expr)
	}
	if rawVal == "" {
		return nil, fmt.Errorf("missing value in %q", expr)
	}

	val, err := parseValue(rawVal)
	if err != nil {
		return nil, fmt.Errorf("invalid value in %q: %w", expr, err)
	}

	p := &PredicateExpr{
		Path:  path,
		Op:    op,
		Value: val,
	}

	if strings.HasPrefix(path, "config.") {
		p.IsConfig = true
		p.Segments = strings.Split(path[len("config."):], ".")
	} else {
		p.Segments = strings.Split(path, ".")
	}

	return p, nil
}

func parseValue(raw string) (any, error) {
	if raw == "true" {
		return true, nil
	}
	if raw == "false" {
		return false, nil
	}
	if strings.HasPrefix(raw, `"`) && strings.HasSuffix(raw, `"`) && len(raw) >= 2 {
		return raw[1 : len(raw)-1], nil
	}
	if f, err := strconv.ParseFloat(raw, 64); err == nil {
		return f, nil
	}
	return nil, fmt.Errorf("unrecognized value %q (use quoted strings, numbers, or true/false)", raw)
}

// Eval evaluates the predicate expression against state and config maps.
// Returns false for missing paths or type mismatches.
func (p *PredicateExpr) Eval(state, config map[string]any) bool {
	var resolved any
	if p.IsConfig {
		resolved = resolvePath(config, p.Segments)
	} else {
		resolved = resolvePath(state, p.Segments)
	}
	if resolved == nil {
		return false
	}
	return compare(resolved, p.Op, p.Value)
}

func resolvePath(m map[string]any, segments []string) any {
	if m == nil || len(segments) == 0 {
		return nil
	}
	val, ok := m[segments[0]]
	if !ok || val == nil {
		return nil
	}
	if len(segments) == 1 {
		return val
	}
	nested, ok := val.(map[string]any)
	if !ok {
		return nil
	}
	return resolvePath(nested, segments[1:])
}

func compare(left any, op string, right any) bool {
	switch rv := right.(type) {
	case string:
		lv, ok := left.(string)
		if !ok {
			return false
		}
		return compareString(lv, op, rv)
	case bool:
		lv, ok := left.(bool)
		if !ok {
			return false
		}
		if op == "==" {
			return lv == rv
		}
		if op == "!=" {
			return lv != rv
		}
		return false
	case float64:
		lf := toFloat64(left)
		if lf == nil {
			return false
		}
		return compareFloat(*lf, op, rv)
	}
	return false
}

func compareString(a, op, b string) bool {
	switch op {
	case "==":
		return a == b
	case "!=":
		return a != b
	case ">":
		return a > b
	case "<":
		return a < b
	case ">=":
		return a >= b
	case "<=":
		return a <= b
	}
	return false
}

func compareFloat(a float64, op string, b float64) bool {
	switch op {
	case "==":
		return a == b
	case "!=":
		return a != b
	case ">":
		return a > b
	case "<":
		return a < b
	case ">=":
		return a >= b
	case "<=":
		return a <= b
	}
	return false
}

func toFloat64(v any) *float64 {
	switch n := v.(type) {
	case float64:
		return &n
	case int:
		f := float64(n)
		return &f
	case int64:
		f := float64(n)
		return &f
	}
	return nil
}
