package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skif48/leaderboard-engine/bot/internal/client"
	"github.com/skif48/leaderboard-engine/bot/internal/logx"
	"github.com/skif48/leaderboard-engine/bot/internal/names"
	"github.com/skif48/leaderboard-engine/bot/internal/scenario"
	"github.com/skif48/leaderboard-engine/entities"
)

var testActions = map[string]int{"spawn": 1, "kill": 8, "heal": 3}

// fakeServer mimics the two write endpoints, including the read-after-write
// window: the first action for a new user 404s once.
type fakeServer struct {
	mu        sync.Mutex
	users     map[string]int // id -> actions seen
	firstMiss map[string]bool
	signups   atomic.Int64
	actions   atomic.Int64
	notFound  atomic.Int64
	lbViews   atomic.Int64
}

func newFakeServer(t *testing.T) (*fakeServer, *httptest.Server) {
	fs := &fakeServer{users: map[string]int{}, firstMiss: map[string]bool{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/users/sign-up", func(w http.ResponseWriter, r *http.Request) {
		var req entities.SignUpRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Nickname == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		n := fs.signups.Add(1)
		id := fmt.Sprintf("user-%d", n)
		fs.mu.Lock()
		fs.users[id] = 0
		fs.firstMiss[id] = true
		fs.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(entities.UserProfile{Id: id, Nickname: req.Nickname, Leaderboard: 1})
	})
	mux.HandleFunc("/api/v1/users/actions", func(w http.ResponseWriter, r *http.Request) {
		var ga entities.GameAction
		if err := json.NewDecoder(r.Body).Decode(&ga); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if _, ok := testActions[ga.Action]; !ok {
			t.Errorf("unknown action %q reached the server", ga.Action)
		}
		fs.mu.Lock()
		defer fs.mu.Unlock()
		if _, ok := fs.users[ga.UserId]; !ok {
			fs.notFound.Add(1)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if fs.firstMiss[ga.UserId] {
			fs.firstMiss[ga.UserId] = false
			fs.notFound.Add(1)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fs.users[ga.UserId]++
		fs.actions.Add(1)
		w.WriteHeader(http.StatusAccepted)
	})
	mux.HandleFunc("/leaderboards", func(w http.ResponseWriter, _ *http.Request) {
		fs.lbViews.Add(1)
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html></html>"))
	})
	return fs, httptest.NewServer(mux)
}

const testScenario = `
name: e2e
excess_sessions: cancel
personas:
  fast:
    actions: {spawn: 1, kill: 2}
    think_time: {mean: 20ms, stddev: 5ms, min: 5ms, max: 50ms}
    session_length: {mean: 400ms, stddev: 50ms, min: 200ms, max: 600ms}
    leaderboard_view_prob: 0.2
phases:
  - {name: ramp, duration: 500ms, active_users: {from: 0, to: 20}, new_user_ratio: 1.0}
  - {name: hold, duration: 700ms, active_users: 20, new_user_ratio: 0.2}
  - {name: crash, duration: 300ms, active_users: 2, new_user_ratio: 0}
  - {name: tail, duration: 300ms, active_users: 2, new_user_ratio: 0}
`

func newTestScheduler(t *testing.T, url string, seed uint64, loop bool) (*Scheduler, *Population) {
	sc, err := scenario.Parse([]byte(testScenario))
	if err != nil {
		t.Fatal(err)
	}
	if err := sc.Validate(testActions); err != nil {
		t.Fatal(err)
	}
	personas, err := sc.BuildPersonas(testActions)
	if err != nil {
		t.Fatal(err)
	}
	cl := client.New(client.Options{BaseURL: url, MaxConns: 50})
	pop := NewPopulation(cl, names.New(seed, "t"), 0, 8)
	s, err := New(Options{
		MaxSpawnPerTick: 100,
		ShutdownTimeout: 2 * time.Second,
		Seed:            seed,
		Tick:            50 * time.Millisecond,
		Loop:            loop,
	}, sc, personas, pop, cl, logx.New(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return s, pop
}

func TestSchedulerRunsScenarioEndToEnd(t *testing.T) {
	fs, srv := newFakeServer(t)
	defer srv.Close()
	s, pop := newTestScheduler(t, srv.URL, 42, false)

	// sample the active count while the scenario runs
	var peak atomic.Int64
	stopSampling := make(chan struct{})
	go func() {
		tk := time.NewTicker(20 * time.Millisecond)
		defer tk.Stop()
		for {
			select {
			case <-stopSampling:
				return
			case <-tk.C:
				if a := int64(s.Active()); a > peak.Load() {
					peak.Store(a)
				}
			}
		}
	}()

	start := time.Now()
	if err := s.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	close(stopSampling)
	elapsed := time.Since(start)

	if elapsed < 1700*time.Millisecond || elapsed > 5*time.Second {
		t.Fatalf("scenario took %v, expected ~1.8s + drain", elapsed)
	}
	if s.Active() != 0 {
		t.Fatalf("sessions still active after Run: %d", s.Active())
	}
	if p := peak.Load(); p < 15 || p > 20 {
		t.Fatalf("peak active sessions = %d, want ~20", p)
	}
	if fs.signups.Load() < 15 {
		t.Fatalf("expected >= 15 sign-ups during a new_user_ratio=1 ramp, got %d", fs.signups.Load())
	}
	if fs.actions.Load() == 0 {
		t.Fatal("no actions reached the server")
	}
	if fs.notFound.Load() == 0 {
		t.Fatal("fake server should have exercised the 404 retry path")
	}
	if fs.lbViews.Load() == 0 {
		t.Fatal("expected some leaderboard views")
	}
	// The server may accept a request whose session was cancelled while it was
	// in flight (crash phase, final drain); the bot counts those as cancelled,
	// not sent. That gap is bounded by one request per session.
	sent, seen := s.actionsSent.Load(), fs.actions.Load()
	if seen < sent || seen-sent > s.sessionsStarted.Load() {
		t.Fatalf("bot counted %d sent actions, server saw %d (sessions started %d)", sent, seen, s.sessionsStarted.Load())
	}
	if pop.Idle() != pop.Size() {
		t.Fatalf("all users should be idle after drain: idle=%d size=%d", pop.Idle(), pop.Size())
	}
}

func TestSchedulerStopsOnContextCancel(t *testing.T) {
	_, srv := newFakeServer(t)
	defer srv.Close()
	s, _ := newTestScheduler(t, srv.URL, 7, true) // loop forever

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	time.Sleep(600 * time.Millisecond)
	if s.Active() == 0 {
		t.Fatal("expected active sessions before cancel")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
	if s.Active() != 0 {
		t.Fatalf("sessions still active after shutdown: %d", s.Active())
	}
}

func TestSchedulerIsDeterministicForSeed(t *testing.T) {
	run := func() []string {
		_, srv := newFakeServer(t)
		defer srv.Close()
		var mu sync.Mutex
		var nicks []string
		// wrap the server to capture nicknames in sign-up order
		capture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v1/users/sign-up" {
				var req entities.SignUpRequest
				_ = json.NewDecoder(r.Body).Decode(&req)
				mu.Lock()
				nicks = append(nicks, req.Nickname)
				mu.Unlock()
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(entities.UserProfile{Id: req.Nickname, Nickname: req.Nickname})
				return
			}
			w.WriteHeader(http.StatusAccepted)
		}))
		defer capture.Close()
		s, _ := newTestScheduler(t, capture.URL, 99, false)
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()
		_ = s.Run(ctx)
		mu.Lock()
		defer mu.Unlock()
		out := make([]string, len(nicks))
		copy(out, nicks)
		return out
	}
	a, b := run(), run()
	if len(a) == 0 || len(b) == 0 {
		t.Fatal("no sign-ups captured")
	}
	// The set of nicknames handed out is seed-determined; ordering may differ
	// because sign-ups run concurrently.
	set := map[string]bool{}
	for _, n := range a {
		set[n] = true
	}
	for _, n := range b[:min(len(a), len(b))] {
		if !set[n] {
			t.Fatalf("nickname %q from run 2 not produced by run 1 with the same seed", n)
		}
	}
}
