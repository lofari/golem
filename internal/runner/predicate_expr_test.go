package runner

import "testing"

func TestParsePredicateExpr_Valid(t *testing.T) {
	tests := []struct {
		name string
		expr string
	}{
		{"string equality", `review-feedback.verdict == "needs-work"`},
		{"numeric greater", `test-results.coverage > 80`},
		{"boolean equality", `config.ci-enabled == true`},
		{"not equal string", `test-results.status != "fail"`},
		{"numeric less-equal", `test-results.coverage <= 100`},
		{"float comparison", `metrics.score >= 3.14`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := ParsePredicateExpr(tt.expr)
			if err != nil {
				t.Fatalf("ParsePredicateExpr(%q) error: %v", tt.expr, err)
			}
			if expr == nil {
				t.Fatal("expected non-nil expr")
			}
		})
	}
}

func TestParsePredicateExpr_Invalid(t *testing.T) {
	tests := []struct {
		name string
		expr string
	}{
		{"empty", ""},
		{"no operator", "review-feedback.verdict"},
		{"bad operator", `review-feedback.verdict ~~ "x"`},
		{"missing value", `review-feedback.verdict ==`},
		{"missing path", `== "value"`},
		{"unquoted string", `review-feedback.verdict == needs-work`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParsePredicateExpr(tt.expr)
			if err == nil {
				t.Fatalf("ParsePredicateExpr(%q) should error", tt.expr)
			}
		})
	}
}

func TestPredicateExpr_Eval(t *testing.T) {
	state := map[string]any{
		"review-feedback": map[string]any{"verdict": "needs-work"},
		"test-results":    map[string]any{"status": "fail", "coverage": float64(85)},
	}
	config := map[string]any{"ci-enabled": true}

	tests := []struct {
		name string
		expr string
		want bool
	}{
		{"string match", `review-feedback.verdict == "needs-work"`, true},
		{"string mismatch", `review-feedback.verdict == "approved"`, false},
		{"string not-equal", `review-feedback.verdict != "approved"`, true},
		{"numeric greater true", `test-results.coverage > 80`, true},
		{"numeric greater false", `test-results.coverage > 90`, false},
		{"numeric less-equal", `test-results.coverage <= 85`, true},
		{"config bool", `config.ci-enabled == true`, true},
		{"config bool false", `config.ci-enabled == false`, false},
		{"missing path", `nonexistent.key == "x"`, false},
		{"missing nested", `review-feedback.nonexistent == "x"`, false},
		{"type mismatch num vs string", `test-results.coverage == "85"`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := ParsePredicateExpr(tt.expr)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			got := expr.Eval(state, config)
			if got != tt.want {
				t.Errorf("Eval(%q) = %v, want %v", tt.expr, got, tt.want)
			}
		})
	}
}
