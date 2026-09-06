package engine

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/skif48/leaderboard-engine/bot/internal/client"
	"github.com/skif48/leaderboard-engine/bot/internal/metrics"
	"github.com/skif48/leaderboard-engine/bot/internal/names"
)

// User is a registered player owned by this bot instance.
type User struct {
	ID       string
	Nickname string
	// Persona the user last played as; idle users are grouped by it so a
	// returning "grinder" stays a grinder when possible.
	Persona string
}

var ErrPoolExhausted = errors.New("user pool exhausted: BOT_MAX_USERS reached and no idle users")

// Population owns the pool of registered users. Sessions acquire a user
// (idle or freshly registered), play, and release it back.
type Population struct {
	mu        sync.Mutex
	idle      map[string][]*User
	idleCount int
	total     int // registered and not evicted, including reservations in flight
	max       int // 0 = unlimited

	sem    chan struct{}
	client *client.Client
	names  *names.Generator
}

func NewPopulation(c *client.Client, n *names.Generator, maxUsers, signupConcurrency int) *Population {
	if signupConcurrency <= 0 {
		signupConcurrency = 1
	}
	return &Population{
		idle:   map[string][]*User{},
		max:    maxUsers,
		sem:    make(chan struct{}, signupConcurrency),
		client: c,
		names:  n,
	}
}

// Acquire returns a user for a session. With wantNew a fresh user is
// registered (this is how scenarios express user growth); otherwise an idle
// user is reused, falling back to registration when none is idle. Once the
// pool cap is reached only idle users are handed out.
func (p *Population) Acquire(ctx context.Context, persona string, wantNew bool) (*User, error) {
	p.mu.Lock()
	capped := p.max > 0 && p.total >= p.max
	if capped {
		wantNew = false
	}
	if !wantNew {
		if u := p.takeIdleLocked(persona); u != nil {
			p.mu.Unlock()
			return u, nil
		}
	}
	if capped {
		p.mu.Unlock()
		return nil, ErrPoolExhausted
	}
	p.total++ // reserve the slot while the sign-up is in flight
	p.mu.Unlock()

	u, err := p.register(ctx, persona)
	if err != nil {
		p.mu.Lock()
		p.total--
		p.mu.Unlock()
		return nil, err
	}
	return u, nil
}

func (p *Population) register(ctx context.Context, persona string) (*User, error) {
	select {
	case p.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-p.sem }()

	nick := p.names.Unique()
	profile, err := p.client.SignUp(ctx, nick)
	if err != nil {
		return nil, err
	}
	metrics.UserRegistered()
	slog.Debug("registered user", "nickname", profile.Nickname, "user_id", profile.Id, "leaderboard", profile.Leaderboard)
	return &User{ID: profile.Id, Nickname: profile.Nickname, Persona: persona}, nil
}

func (p *Population) takeIdleLocked(persona string) *User {
	if l := p.idle[persona]; len(l) > 0 {
		u := l[len(l)-1]
		p.idle[persona] = l[:len(l)-1]
		p.idleCount--
		return u
	}
	for name, l := range p.idle {
		if len(l) == 0 {
			continue
		}
		u := l[len(l)-1]
		p.idle[name] = l[:len(l)-1]
		p.idleCount--
		u.Persona = persona
		return u
	}
	return nil
}

// Release returns a user to the idle pool.
func (p *Population) Release(u *User) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.idle[u.Persona] = append(p.idle[u.Persona], u)
	p.idleCount++
}

// Evict drops a user that the server no longer knows (for example after a
// purge) so the pool can register a replacement.
func (p *Population) Evict(u *User) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.total--
	slog.Debug("evicted user", "nickname", u.Nickname, "user_id", u.ID)
}

// Size is the number of registered users (including sign-ups in flight).
func (p *Population) Size() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.total
}

// Idle is the number of users not currently in a session.
func (p *Population) Idle() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.idleCount
}

// SignupsInFlight is the number of sign-up requests currently running.
func (p *Population) SignupsInFlight() int { return len(p.sem) }
