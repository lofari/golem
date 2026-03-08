package execution

import (
	"fmt"

	"github.com/lofari/golem/internal/graph"
)

// PruneSessions removes the oldest execution sessions beyond the retention limit.
// Returns the number of sessions pruned.
func PruneSessions(store *graph.Store, keep int) (int, error) {
	execs, err := store.QueryExecutions(1000) // ordered newest-first
	if err != nil {
		return 0, fmt.Errorf("querying executions: %w", err)
	}

	if len(execs) <= keep {
		return 0, nil
	}

	// Sessions to prune are those beyond the keep limit (oldest ones)
	toPrune := execs[keep:]
	for _, e := range toPrune {
		if err := store.DeleteExecution(e.SessionID); err != nil {
			return 0, fmt.Errorf("deleting session %s: %w", e.SessionID, err)
		}
	}

	return len(toPrune), nil
}
