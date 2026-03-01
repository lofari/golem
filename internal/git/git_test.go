// internal/git/git_test.go
package git

import (
	"testing"
)

func TestCheckLockedPaths(t *testing.T) {
	tests := []struct {
		name    string
		changed []string
		locked  []string
		wantLen int
	}{
		{
			name:    "no violations",
			changed: []string{"src/main.go", "tests/main_test.go"},
			locked:  []string{"src/auth/"},
			wantLen: 0,
		},
		{
			name:    "file under locked path",
			changed: []string{"src/auth/handler.go", "src/main.go"},
			locked:  []string{"src/auth/"},
			wantLen: 1,
		},
		{
			name:    "multiple violations",
			changed: []string{"src/auth/handler.go", "src/auth/middleware.go", "src/main.go"},
			locked:  []string{"src/auth/"},
			wantLen: 2,
		},
		{
			name:    "locked path without trailing slash",
			changed: []string{"src/auth/handler.go"},
			locked:  []string{"src/auth"},
			wantLen: 1,
		},
		{
			name:    "multiple locked paths",
			changed: []string{"src/auth/handler.go", "src/db/schema.go", "src/main.go"},
			locked:  []string{"src/auth/", "src/db/"},
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckLockedPaths(tt.changed, tt.locked)
			if len(got) != tt.wantLen {
				t.Errorf("CheckLockedPaths() returned %d violations, want %d: %v", len(got), tt.wantLen, got)
			}
		})
	}
}
