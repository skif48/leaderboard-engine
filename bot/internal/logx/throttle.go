// Package logx contains small logging helpers. Under load the same error tends
// to repeat thousands of times per second; Throttle keeps one line per key per
// window and reports how many were dropped.
package logx

import (
	"log/slog"
	"sync"
	"time"
)

type Throttle struct {
	mu      sync.Mutex
	window  time.Duration
	entries map[string]*entry
	now     func() time.Time
}

type entry struct {
	last       time.Time
	suppressed int
}

func New(window time.Duration) *Throttle {
	return &Throttle{window: window, entries: map[string]*entry{}, now: time.Now}
}

// Allow reports whether a message for key should be emitted now. When it
// returns true, suppressed is the number of calls dropped since the last emit.
func (t *Throttle) Allow(key string) (ok bool, suppressed int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	e, exists := t.entries[key]
	if !exists {
		t.entries[key] = &entry{last: now}
		return true, 0
	}
	if now.Sub(e.last) < t.window {
		e.suppressed++
		return false, 0
	}
	suppressed = e.suppressed
	e.last = now
	e.suppressed = 0
	return true, suppressed
}

// Error logs at Error level unless key was logged within the window. The
// suppressed count is attached when it is non-zero.
func (t *Throttle) Error(key, msg string, args ...any) {
	ok, suppressed := t.Allow(key)
	if !ok {
		return
	}
	if suppressed > 0 {
		args = append(args, "suppressed", suppressed)
	}
	slog.Error(msg, args...)
}

// Warn is the Warn-level counterpart of Error.
func (t *Throttle) Warn(key, msg string, args ...any) {
	ok, suppressed := t.Allow(key)
	if !ok {
		return
	}
	if suppressed > 0 {
		args = append(args, "suppressed", suppressed)
	}
	slog.Warn(msg, args...)
}
