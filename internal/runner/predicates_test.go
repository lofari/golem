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
			got, found := evalBuiltinPredicate("needs-work", tt.state, tt.config)
			if !found {
				t.Error("needs-work should be a recognized predicate")
			}
			if got != tt.want {
				t.Errorf("evalBuiltinPredicate(needs-work) = %v, want %v", got, tt.want)
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
			got, found := evalBuiltinPredicate("failed", tt.state, nil)
			if !found {
				t.Error("failed should be a recognized predicate")
			}
			if got != tt.want {
				t.Errorf("evalBuiltinPredicate(failed) = %v, want %v", got, tt.want)
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
			got, found := evalBuiltinPredicate("ci-enabled", nil, tt.config)
			if !found {
				t.Error("ci-enabled should be a recognized predicate")
			}
			if got != tt.want {
				t.Errorf("evalBuiltinPredicate(ci-enabled) = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPredicate_Unknown(t *testing.T) {
	got, found := evalBuiltinPredicate("nonexistent", nil, nil)
	if found {
		t.Error("unknown predicate should return found=false")
	}
	if got != false {
		t.Error("unknown predicate should return false")
	}
}
