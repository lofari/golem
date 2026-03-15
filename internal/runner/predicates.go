package runner

// evalBuiltinPredicate evaluates a named built-in predicate against pipeline state and config.
// Returns (result, found) where found indicates whether the predicate name was recognized.
func evalBuiltinPredicate(name string, state map[string]any, config map[string]any) (bool, bool) {
	switch name {
	case "needs-work":
		return getNestedString(state, "review-feedback", "verdict") == "needs-work", true
	case "failed":
		return getNestedString(state, "test-results", "status") == "fail", true
	case "lint-failed":
		return getNestedString(state, "lint-results", "status") == "fail", true
	case "ci-enabled":
		if config == nil {
			return false, true
		}
		v, ok := config["ci-enabled"]
		if !ok {
			return false, true
		}
		b, ok := v.(bool)
		return ok && b, true
	case "ci-failed":
		return getNestedString(state, "ci-results", "status") == "fail", true
	case "tasks-remaining":
		if halt, _ := state["_halt"].(bool); halt {
			return false, true
		}
		tasks, ok := state["tasks"].([]any)
		if !ok {
			return false, true
		}
		for _, t := range tasks {
			tm, ok := t.(map[string]any)
			if !ok {
				continue
			}
			status, _ := tm["status"].(string)
			if status == "todo" || status == "in-progress" {
				return true, true
			}
		}
		return false, true
	default:
		return false, false
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
