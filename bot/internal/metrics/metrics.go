// Package metrics wraps the VictoriaMetrics client (the same library the server
// uses) behind named helpers so the rest of the bot never builds metric names
// by hand, and serves them on a small dedicated HTTP listener.
package metrics

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	vm "github.com/VictoriaMetrics/metrics"
)

// --- HTTP client ---

// HTTPRequest records one attempt against the server. status is the numeric
// status code or one of net_error / timeout / canceled.
func HTTPRequest(endpoint, status string, start time.Time) {
	vm.GetOrCreateCounter(fmt.Sprintf(`bot_http_requests_total{endpoint=%q, status=%q}`, endpoint, status)).Inc()
	vm.GetOrCreateHistogram(fmt.Sprintf(`bot_http_request_duration_seconds{endpoint=%q}`, endpoint)).UpdateDuration(start)
}

func HTTPRetry(endpoint, reason string) {
	vm.GetOrCreateCounter(fmt.Sprintf(`bot_http_retries_total{endpoint=%q, reason=%q}`, endpoint, reason)).Inc()
}

// --- Game traffic ---

func ActionSent(action string) {
	vm.GetOrCreateCounter(fmt.Sprintf(`bot_actions_sent_total{action=%q}`, action)).Inc()
}

func ActionFailed(action, kind string) {
	vm.GetOrCreateCounter(fmt.Sprintf(`bot_actions_failed_total{action=%q, kind=%q}`, action, kind)).Inc()
}

func UserRegistered() {
	vm.GetOrCreateCounter(`bot_users_registered_total`).Inc()
}

// --- Sessions & scenario ---

func SessionStarted(persona string) {
	vm.GetOrCreateCounter(fmt.Sprintf(`bot_sessions_started_total{persona=%q}`, persona)).Inc()
}

func SessionEnded(persona, reason string) {
	vm.GetOrCreateCounter(fmt.Sprintf(`bot_sessions_ended_total{persona=%q, reason=%q}`, persona, reason)).Inc()
}

func SetTargetSessions(n int) {
	vm.GetOrCreateGauge(`bot_target_active_sessions`, nil).Set(float64(n))
}

// SetPhase flips the info gauge for a phase: 1 for the current phase, 0 once it ends.
func SetPhase(name string, active bool) {
	v := 0.0
	if active {
		v = 1
	}
	vm.GetOrCreateGauge(fmt.Sprintf(`bot_phase_info{phase=%q}`, name), nil).Set(v)
}

func SetScenarioElapsed(d time.Duration) {
	vm.GetOrCreateGauge(`bot_scenario_elapsed_seconds`, nil).Set(d.Seconds())
}

func ScenarioLooped() {
	vm.GetOrCreateCounter(`bot_scenario_loops_total`).Inc()
}

// RegisterGauge registers a callback-backed gauge (pool sizes, active sessions).
func RegisterGauge(name string, f func() float64) {
	vm.GetOrCreateGauge(name, f)
}

// Serve exposes /metrics and /health on the given port until ctx is done.
// It returns nil on a clean shutdown and the listen error otherwise.
func Serve(ctx context.Context, port int) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		vm.WritePrometheus(w, true)
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	srv := &http.Server{
		Addr:              ":" + strconv.Itoa(port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
