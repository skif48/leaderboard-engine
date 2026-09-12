// Package xtime holds tiny time helpers shared across the bot.
package xtime

import (
	"context"
	"time"
)

// Sleep waits for d or until ctx is done. It returns true when the full
// duration elapsed and false when the context ended first.
func Sleep(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
