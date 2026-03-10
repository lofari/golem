package cmd

import "testing"

func TestShouldUseDSL(t *testing.T) {
	tests := []struct {
		engine   string
		wantsDSL bool
	}{
		{"go", false},
		{"dsl", true},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.engine, func(t *testing.T) {
			result := shouldUseDSL(tt.engine)
			if result != tt.wantsDSL {
				t.Fatalf("engine=%q: expected shouldUseDSL=%v, got %v", tt.engine, tt.wantsDSL, result)
			}
		})
	}
}
