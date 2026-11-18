// Package deadline applies the deadline of a call to its context.
package deadline

import (
	"context"
	"time"
)

// Apply bounds ctx by d, or returns ctx unchanged when d is zero.
func Apply(ctx context.Context, d time.Time) (context.Context, context.CancelFunc) {
	if d.IsZero() {
		return ctx, func() {}
	}
	return context.WithDeadline(ctx, d)
}
