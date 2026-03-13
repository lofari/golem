package runner

// EvalPredicate evaluates a named predicate against pipeline state and config.
// Unknown predicates and missing keys return false.
func EvalPredicate(name string, state map[string]any, config map[string]any) bool {
	switch name {
	case "needs-work":
		return getNestedString(state, "review-feedback", "verdict") == "needs-work"
	case "failed":
		return getNestedString(state, "test-results", "status") == "fail"
	case "lint-failed":
		return getNestedString(state, "lint-results", "status") == "fail"
	case "ci-enabled":
		if config == nil {
			return false
		}
		v, ok := config["ci-enabled"]
		if !ok {
			return false
		}
		b, ok := v.(bool)
		return ok && b
	case "ci-failed":
		return getNestedString(state, "ci-results", "status") == "fail"
	default:
		return false
	}
}

// getNestedString safely extracts state[key1][key2] as a string.
func getNestedString(state map[string]any, key1, key2 string) string {
	if state == nil {
		return ""
	}
	v, ok := state[key1]
	if !ok || v == nil {
		return ""
	}
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	s, ok := m[key2]
	if !ok || s == nil {
		return ""
	}
	str, ok := s.(string)
	if !ok {
		return ""
	}
	return str
}
