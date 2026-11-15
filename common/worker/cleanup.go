package worker

import (
	"context"
	"time"
)

// Cleanup returns a context for recording an outcome after the work's own
// context has ended. It keeps parent's values (request IDs, tenant) but not its
// cancellation, and is bounded by timeout so that shutdown still finishes.
//
// Background work is usually cancelled or timed out by the time it records
// what happened, so writing with the work context fails at once; writing with
// context.Background drops the values and has no deadline at all.
func Cleanup(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), timeout)
}
