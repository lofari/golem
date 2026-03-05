package runner

import (
	"testing"

	"github.com/lofari/golem/internal/ctx"
)

func TestNewStrategy(t *testing.T) {
	s := NewStrategy()
	if s == nil {
		t.Fatal("NewStrategy returned nil")
	}
	d := s.Evaluate(ctx.State{}, ctx.Log{}, "")
	if d.Action != ActionContinue {
		t.Errorf("empty state should return Continue, got %v", d.Action)
	}
}
