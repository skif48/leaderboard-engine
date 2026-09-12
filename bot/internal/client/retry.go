package client

import (
	"math/rand/v2"
	"time"
)

// backoffSpec describes retries for one class of failure. attempts is the
// total number of tries (first try included); 0 or 1 means "never retry".
type backoffSpec struct {
	attempts int
	base     time.Duration
	cap      time.Duration
}

// delay returns a full-jitter backoff for the given retry number (1-based):
// rand(0, min(cap, base * 2^(retry-1))).
func (b backoffSpec) delay(retry int) time.Duration {
	if retry < 1 {
		retry = 1
	}
	if retry > 20 {
		retry = 20
	}
	d := b.base << uint(retry-1)
	if d <= 0 || d > b.cap {
		d = b.cap
	}
	if d <= 0 {
		return 0
	}
	return time.Duration(rand.Int64N(int64(d) + 1))
}

// retryPolicy is the per-endpoint retry configuration.
//
// notFound covers the server's read-after-write window: profiles are inserted
// at QUORUM but the action handler reads them at consistency ONE, so a freshly
// registered user can 404 for a moment. transient covers 5xx / 429 / network /
// timeout failures.
type retryPolicy struct {
	perAttempt time.Duration
	notFound   backoffSpec
	transient  backoffSpec
}

var (
	defaultSignUpPolicy = retryPolicy{
		perAttempt: 15 * time.Second,
		transient:  backoffSpec{attempts: 5, base: 200 * time.Millisecond, cap: 3 * time.Second},
	}
	defaultActionPolicy = retryPolicy{
		perAttempt: 10 * time.Second,
		notFound:   backoffSpec{attempts: 6, base: 100 * time.Millisecond, cap: 2 * time.Second},
		transient:  backoffSpec{attempts: 3, base: 200 * time.Millisecond, cap: 2 * time.Second},
	}
	defaultLeaderboardsPolicy = retryPolicy{
		perAttempt: 15 * time.Second,
	}
)
