package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skif48/leaderboard-engine/entities"
)

// fast policies keep the tests quick while preserving attempt counts.
func fastClient(url string) *Client {
	c := New(Options{BaseURL: url, MaxConns: 10})
	quick := backoffSpec{base: time.Millisecond, cap: 2 * time.Millisecond}
	c.signUpPolicy = retryPolicy{perAttempt: time.Second, transient: backoffSpec{attempts: 5, base: quick.base, cap: quick.cap}}
	c.actionPolicy = retryPolicy{
		perAttempt: time.Second,
		notFound:   backoffSpec{attempts: 6, base: quick.base, cap: quick.cap},
		transient:  backoffSpec{attempts: 3, base: quick.base, cap: quick.cap},
	}
	return c
}

func TestSendActionRetriesNotFoundThenSucceeds(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/users/actions" || r.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var ga entities.GameAction
		if err := json.NewDecoder(r.Body).Decode(&ga); err != nil || ga.UserId != "u1" || ga.Action != "kill" {
			t.Errorf("bad body: %+v err=%v", ga, err)
		}
		if hits.Add(1) <= 2 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	c := fastClient(srv.URL)
	if err := c.SendAction(context.Background(), "u1", "kill"); err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if got := hits.Load(); got != 3 {
		t.Fatalf("expected 3 attempts, got %d", got)
	}
}

func TestSendActionGivesUpOnPersistentNotFound(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	err := fastClient(srv.URL).SendAction(context.Background(), "gone", "spawn")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Kind != KindNotFound || apiErr.Attempts != 6 {
		t.Fatalf("expected not_found after 6 attempts, got %v", err)
	}
	if hits.Load() != 6 {
		t.Fatalf("expected 6 hits, got %d", hits.Load())
	}
}

func TestSignUpRetriesServerErrors(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(entities.UserProfile{Id: "abc", Nickname: "Nick", Leaderboard: 4})
	}))
	defer srv.Close()

	p, err := fastClient(srv.URL).SignUp(context.Background(), "Nick")
	if err != nil {
		t.Fatal(err)
	}
	if p.Id != "abc" || p.Leaderboard != 4 {
		t.Fatalf("unexpected profile %+v", p)
	}
	if hits.Load() != 3 {
		t.Fatalf("expected 3 hits, got %d", hits.Load())
	}
}

func TestClientErrorsAreNotRetried(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad json"))
	}))
	defer srv.Close()

	err := fastClient(srv.URL).SendAction(context.Background(), "u", "spawn")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Kind != KindClient || apiErr.Body != "bad json" {
		t.Fatalf("expected client_error with body, got %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("expected exactly 1 hit, got %d", hits.Load())
	}
}

func TestCanceledContextShortCircuits(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := fastClient(srv.URL).SendAction(ctx, "u", "spawn")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Kind != KindCanceled {
		t.Fatalf("expected canceled, got %v", err)
	}
}

func TestRateLimiterCapsThroughput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	c := New(Options{BaseURL: srv.URL, MaxConns: 10, MaxRPS: 20})
	start := time.Now()
	for i := 0; i < 30; i++ {
		if err := c.SendAction(context.Background(), "u", "spawn"); err != nil {
			t.Fatal(err)
		}
	}
	// burst of 20 is free, the remaining 10 need >= 0.5s at 20 rps
	if elapsed := time.Since(start); elapsed < 400*time.Millisecond {
		t.Fatalf("limiter did not slow requests: %v", elapsed)
	}
}
