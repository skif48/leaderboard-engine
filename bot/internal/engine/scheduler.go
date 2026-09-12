// Package engine drives a scenario: it keeps the number of active sessions
// tracking the phase target, starts sessions with the right persona and
// new-vs-returning user mix, sheds sessions on a crash phase, and stops
// cleanly on shutdown.
//
// One goroutine per active session is deliberate: a parked goroutine costs a
// few KB, so thousands are cheap. The real throughput ceilings are the HTTP
// transport's MaxConnsPerHost and the optional rate limiter; when demand
// exceeds them requests queue in the transport and the bot-side latency
// histogram diverges from the server's, which is the signal to raise
// BOT_MAX_CONNS or add replicas.
package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/skif48/leaderboard-engine/bot/internal/client"
	"github.com/skif48/leaderboard-engine/bot/internal/logx"
	"github.com/skif48/leaderboard-engine/bot/internal/metrics"
	"github.com/skif48/leaderboard-engine/bot/internal/persona"
	"github.com/skif48/leaderboard-engine/bot/internal/scenario"
)

type Options struct {
	// MaxSpawnPerTick bounds session starts per tick to smooth sign-up bursts.
	MaxSpawnPerTick int
	// ShutdownTimeout bounds how long Run waits for sessions to exit.
	ShutdownTimeout time.Duration
	// SummaryInterval controls the periodic Info summary; 0 disables it.
	SummaryInterval time.Duration
	// Loop forces the scenario to repeat (OR-ed with the scenario's own flag).
	Loop bool
	// Seed drives persona/user choices and per-session RNGs.
	Seed uint64
	// Tick is the scheduling interval; defaults to 1s.
	Tick time.Duration
}

type Scheduler struct {
	opts     Options
	sc       *scenario.Scenario
	personas map[string]*persona.Persona
	pop      *Population
	client   *client.Client
	throttle *logx.Throttle
	rng      *rand.Rand // only touched from the Run goroutine

	mu        sync.Mutex
	sessions  map[uint64]*session
	canceling int
	nextID    uint64
	wg        sync.WaitGroup

	actionsSent, actionsFailed, sessionsStarted, sessionsEnded atomic.Int64
}

func New(opts Options, sc *scenario.Scenario, personas map[string]*persona.Persona, pop *Population, c *client.Client, throttle *logx.Throttle) (*Scheduler, error) {
	if !sc.Validated() {
		return nil, errors.New("scenario must be validated before scheduling")
	}
	for name := range sc.Personas {
		if personas[name] == nil {
			return nil, fmt.Errorf("persona %q is declared in the scenario but was not built", name)
		}
	}
	if opts.Tick <= 0 {
		opts.Tick = time.Second
	}
	if opts.MaxSpawnPerTick <= 0 {
		opts.MaxSpawnPerTick = 100
	}
	if opts.ShutdownTimeout <= 0 {
		opts.ShutdownTimeout = 10 * time.Second
	}
	if throttle == nil {
		throttle = logx.New(10 * time.Second)
	}
	s := &Scheduler{
		opts:     opts,
		sc:       sc,
		personas: personas,
		pop:      pop,
		client:   c,
		throttle: throttle,
		rng:      rand.New(rand.NewPCG(opts.Seed, opts.Seed^0xD1B54A32D192ED03)),
		sessions: map[uint64]*session{},
	}
	metrics.RegisterGauge("bot_sessions_active", func() float64 { return float64(s.Active()) })
	metrics.RegisterGauge("bot_users_pool_size", func() float64 { return float64(pop.Size()) })
	metrics.RegisterGauge("bot_users_pool_idle", func() float64 { return float64(pop.Idle()) })
	metrics.RegisterGauge("bot_signup_inflight", func() float64 { return float64(pop.SignupsInFlight()) })
	return s, nil
}

// Active is the number of live sessions, excluding ones already told to stop.
func (s *Scheduler) Active() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions) - s.canceling
}

type runState struct {
	phase  int
	loops  int
	target int
	prev   summarySnapshot
}

type summarySnapshot struct {
	sent, failed, started, ended int64
	at                           time.Time
}

// Run executes the scenario until it completes (non-looping) or ctx is done.
// It always returns nil after a bounded drain; a non-nil error means the
// scheduler could not start.
func (s *Scheduler) Run(ctx context.Context) error {
	loop := s.opts.Loop || s.sc.Loop
	start := time.Now()
	state := &runState{phase: -1, prev: summarySnapshot{at: start}}

	slog.Info("scenario starting",
		"scenario", s.sc.Name,
		"phases", len(s.sc.Phases),
		"personas", s.sc.PersonaNames(),
		"loop", loop,
		"total_duration", s.sc.Total().String(),
		"tick", s.opts.Tick.String())

	ticker := time.NewTicker(s.opts.Tick)
	defer ticker.Stop()
	var summary <-chan time.Time
	if s.opts.SummaryInterval > 0 {
		st := time.NewTicker(s.opts.SummaryInterval)
		defer st.Stop()
		summary = st.C
	}

	if s.tick(ctx, start, loop, state) {
		return s.finish(state, "scenario complete")
	}
	for {
		select {
		case <-ctx.Done():
			return s.finish(state, "shutdown requested")
		case <-summary:
			s.logSummary(state, "summary")
		case <-ticker.C:
			if s.tick(ctx, start, loop, state) {
				return s.finish(state, "scenario complete")
			}
		}
	}
}

func (s *Scheduler) tick(ctx context.Context, start time.Time, loop bool, state *runState) (done bool) {
	elapsed := time.Since(start)
	idx, phaseElapsed, loops, done := s.sc.Locate(elapsed, loop)
	metrics.SetScenarioElapsed(elapsed)
	if done {
		return true
	}
	for state.loops < loops {
		state.loops++
		metrics.ScenarioLooped()
		slog.Info("scenario looped", "loops", state.loops)
	}

	phase := &s.sc.Phases[idx]
	if idx != state.phase {
		if state.phase >= 0 {
			metrics.SetPhase(s.sc.Phases[state.phase].Name, false)
		}
		metrics.SetPhase(phase.Name, true)
		from, to := phase.Range()
		slog.Info("entering phase",
			"phase", phase.Name,
			"duration", phase.Duration.D().String(),
			"shape", phase.Shape,
			"active_users_from", from,
			"active_users_to", to,
			"new_user_ratio", phase.NewUserRatio,
			"persona_mix", phase.PersonaMix,
			"excess_sessions", phase.ExcessSessions)
		state.phase = idx
	}

	target := phase.TargetAt(phaseElapsed)
	state.target = target
	metrics.SetTargetSessions(target)

	spawn, cancel := Plan(s.Active(), target, s.opts.MaxSpawnPerTick, phase.ExcessSessions == scenario.ExcessCancel)
	for i := 0; i < spawn; i++ {
		name := PickPersona(phase.PersonaMix, s.rng)
		wantNew := s.rng.Float64() < phase.NewUserRatio
		s.spawn(ctx, s.personas[name], wantNew)
	}
	if cancel > 0 {
		s.cancelNewest(cancel)
	}
	return false
}

func (s *Scheduler) spawn(ctx context.Context, p *persona.Persona, wantNew bool) {
	sctx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.nextID++
	sess := &session{
		id:        s.nextID,
		persona:   p,
		rng:       rand.New(rand.NewPCG(s.rng.Uint64(), s.rng.Uint64())),
		cancel:    cancel,
		startedAt: time.Now(),
	}
	s.sessions[sess.id] = sess
	s.mu.Unlock()
	s.wg.Add(1)
	go s.runSession(sctx, sess, wantNew)
}

// cancelNewest stops the n most recently started sessions. Newest-first keeps
// long-lived players alive and sheds the burst, which is what a crash phase
// after a spike should look like.
func (s *Scheduler) cancelNewest(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	candidates := make([]*session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		if !sess.canceled {
			candidates = append(candidates, sess)
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].startedAt.After(candidates[j].startedAt) })
	if n > len(candidates) {
		n = len(candidates)
	}
	for _, sess := range candidates[:n] {
		sess.canceled = true
		s.canceling++
		sess.cancel()
	}
}

func (s *Scheduler) removeSession(sess *session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess.canceled {
		s.canceling--
	}
	delete(s.sessions, sess.id)
}

func (s *Scheduler) finish(state *runState, why string) error {
	s.mu.Lock()
	n := len(s.sessions)
	for _, sess := range s.sessions {
		sess.cancel()
	}
	s.mu.Unlock()
	slog.Info(why, "stopping_sessions", n, "timeout", s.opts.ShutdownTimeout.String())

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(s.opts.ShutdownTimeout):
		slog.Warn("shutdown timeout elapsed, abandoning sessions", "remaining", s.Active())
	}
	s.logSummary(state, "final summary")
	return nil
}

func (s *Scheduler) logSummary(state *runState, msg string) {
	now := time.Now()
	cur := summarySnapshot{
		sent:    s.actionsSent.Load(),
		failed:  s.actionsFailed.Load(),
		started: s.sessionsStarted.Load(),
		ended:   s.sessionsEnded.Load(),
		at:      now,
	}
	window := now.Sub(state.prev.at).Seconds()
	rate := 0.0
	if window > 0 {
		rate = float64(cur.sent-state.prev.sent) / window
	}
	phaseName := ""
	if state.phase >= 0 {
		phaseName = s.sc.Phases[state.phase].Name
	}
	slog.Info(msg,
		"phase", phaseName,
		"target", state.target,
		"active", s.Active(),
		"users_total", s.pop.Size(),
		"users_idle", s.pop.Idle(),
		"signups_inflight", s.pop.SignupsInFlight(),
		"actions_sent", cur.sent-state.prev.sent,
		"actions_failed", cur.failed-state.prev.failed,
		"actions_per_sec", fmt.Sprintf("%.1f", rate),
		"sessions_started", cur.started-state.prev.started,
		"sessions_ended", cur.ended-state.prev.ended,
		"actions_sent_total", cur.sent,
		"actions_failed_total", cur.failed)
	state.prev = cur
}
