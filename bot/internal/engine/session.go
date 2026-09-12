package engine

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/skif48/leaderboard-engine/bot/internal/client"
	"github.com/skif48/leaderboard-engine/bot/internal/metrics"
	"github.com/skif48/leaderboard-engine/bot/internal/persona"
	"github.com/skif48/leaderboard-engine/bot/internal/xtime"
)

const (
	reasonCompleted     = "completed"
	reasonCanceled      = "canceled"
	reasonUserGone      = "user_gone"
	reasonSignupFailed  = "signup_failed"
	reasonPoolExhausted = "pool_exhausted"
)

// session is one simulated player visit: a user playing as a persona for a
// bounded time.
type session struct {
	id        uint64
	persona   *persona.Persona
	rng       *rand.Rand
	cancel    context.CancelFunc
	startedAt time.Time
	canceled  bool // set by the scheduler when shedding excess sessions
}

func (s *Scheduler) runSession(ctx context.Context, sess *session, wantNew bool) {
	defer s.wg.Done()
	defer s.removeSession(sess)
	defer sess.cancel()

	p := sess.persona
	metrics.SessionStarted(p.Name)
	s.sessionsStarted.Add(1)

	u, err := s.pop.Acquire(ctx, p.Name, wantNew)
	if err != nil {
		reason := reasonSignupFailed
		switch {
		case errors.Is(err, ErrPoolExhausted):
			reason = reasonPoolExhausted
			s.throttle.Warn("pool_exhausted", "user pool exhausted; lower the target or raise BOT_MAX_USERS")
		case ctx.Err() != nil:
			reason = reasonCanceled
		}
		s.endSession(sess, reason)
		return
	}

	length := p.SessionLength.Sample(sess.rng)
	sctx, cancel := context.WithTimeout(ctx, length)
	defer cancel()
	slog.Debug("session started", "session", sess.id, "persona", p.Name, "nickname", u.Nickname, "length", length)

	reason := reasonCompleted
loop:
	for sctx.Err() == nil {
		action := p.PickAction(sess.rng)
		if err := s.client.SendAction(sctx, u.ID, action); err != nil {
			var apiErr *client.APIError
			kind := "unknown"
			if errors.As(err, &apiErr) {
				kind = apiErr.Kind.String()
			}
			switch {
			case apiErr != nil && apiErr.Kind == client.KindCanceled:
				break loop
			case apiErr != nil && apiErr.Kind == client.KindNotFound:
				// Retries inside the client already covered the read-after-write
				// window, so the user really is gone (e.g. a purge happened).
				s.actionsFailed.Add(1)
				metrics.ActionFailed(action, kind)
				s.pop.Evict(u)
				u = nil
				reason = reasonUserGone
				break loop
			default:
				s.actionsFailed.Add(1)
				metrics.ActionFailed(action, kind)
			}
		} else {
			s.actionsSent.Add(1)
			metrics.ActionSent(action)
			slog.Debug("action sent", "session", sess.id, "nickname", u.Nickname, "action", action)
		}

		if p.LeaderboardViewProb > 0 && sess.rng.Float64() < p.LeaderboardViewProb {
			_ = s.client.GetLeaderboards(sctx) // best effort; the client records metrics and logs
		}
		if !xtime.Sleep(sctx, p.ThinkTime.Sample(sess.rng)) {
			break
		}
	}

	if u != nil {
		s.pop.Release(u)
	}
	if reason == reasonCompleted && !errors.Is(sctx.Err(), context.DeadlineExceeded) {
		reason = reasonCanceled
	}
	s.endSession(sess, reason)
}

func (s *Scheduler) endSession(sess *session, reason string) {
	metrics.SessionEnded(sess.persona.Name, reason)
	s.sessionsEnded.Add(1)
	slog.Debug("session ended", "session", sess.id, "persona", sess.persona.Name, "reason", reason, "elapsed", time.Since(sess.startedAt))
}
