package runner

import "testing"

func TestPredicate_NeedsWork(t *testing.T) {
	tests := []struct {
		name   string
		state  map[string]any
		config map[string]any
		want   bool
	}{
		{"missing key", map[string]any{}, nil, false},
		{"approved", map[string]any{"review-feedback": map[string]any{"verdict": "approved"}}, nil, false},
		{"needs-work", map[string]any{"review-feedback": map[string]any{"verdict": "needs-work"}}, nil, true},
		{"nil value", map[string]any{"review-feedback": nil}, nil, false},
		{"wrong type", map[string]any{"review-feedback": "string"}, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EvalPredicate("needs-work", tt.state, tt.config)
			if got != tt.want {
				t.Errorf("EvalPredicate(needs-work) = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPredicate_Failed(t *testing.T) {
	tests := []struct {
		name  string
		state map[string]any
		want  bool
	}{
		{"missing", map[string]any{}, false},
		{"pass", map[string]any{"test-results": map[string]any{"status": "pass"}}, false},
		{"fail", map[string]any{"test-results": map[string]any{"status": "fail"}}, true},
		{"skipped", map[string]any{"test-results": map[string]any{"status": "skipped"}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EvalPredicate("failed", tt.state, nil)
			if got != tt.want {
				t.Errorf("EvalPredicate(failed) = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPredicate_CIEnabled(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]any
		want   bool
	}{
		{"nil config", nil, false},
		{"not set", map[string]any{}, false},
		{"false", map[string]any{"ci-enabled": false}, false},
		{"true", map[string]any{"ci-enabled": true}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EvalPredicate("ci-enabled", nil, tt.config)
			if got != tt.want {
				t.Errorf("EvalPredicate(ci-enabled) = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPredicate_Unknown(t *testing.T) {
	got := EvalPredicate("nonexistent", nil, nil)
	if got != false {
		t.Error("unknown predicate should return false")
	}
}
