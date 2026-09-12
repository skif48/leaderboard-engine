// Package persona models *how* a simulated player behaves: which actions it
// favours, how long it pauses between them, how long it stays, and how often
// it looks at the leaderboards. Personas are defined in the scenario YAML; the
// engine never hardcodes any.
package persona

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"sort"
	"time"
)

// Dist is a clamped normal distribution over durations. Stddev 0 makes it
// constant; Max 0 means "no upper clamp".
type Dist struct {
	Mean, Stddev, Min, Max time.Duration
}

func (d Dist) Validate() error {
	switch {
	case d.Mean <= 0:
		return errors.New("mean must be > 0")
	case d.Stddev < 0:
		return errors.New("stddev must be >= 0")
	case d.Min < 0:
		return errors.New("min must be >= 0")
	case d.Max > 0 && d.Max < d.Min:
		return errors.New("max must be >= min")
	case d.Mean < d.Min:
		return errors.New("mean must be >= min")
	case d.Max > 0 && d.Mean > d.Max:
		return errors.New("mean must be <= max")
	}
	return nil
}

func (d Dist) Sample(r *rand.Rand) time.Duration {
	v := float64(d.Mean)
	if d.Stddev > 0 {
		v += r.NormFloat64() * float64(d.Stddev)
	}
	if v < float64(d.Min) {
		v = float64(d.Min)
	}
	if d.Max > 0 && v > float64(d.Max) {
		v = float64(d.Max)
	}
	return time.Duration(v)
}

type Persona struct {
	Name                string
	ThinkTime           Dist
	SessionLength       Dist
	LeaderboardViewProb float64

	actions []string  // sorted for deterministic picking
	cum     []float64 // cumulative weights aligned with actions
}

// New builds a persona. weights must be positive and, when known is non-nil,
// every action must exist in it (the server accepts unknown action names over
// HTTP and only rejects them in the Kafka consumer, so the bot validates).
func New(name string, weights map[string]float64, think, session Dist, lbProb float64, known map[string]int) (*Persona, error) {
	if name == "" {
		return nil, errors.New("persona name must not be empty")
	}
	if len(weights) == 0 {
		return nil, errors.New("at least one action weight is required")
	}
	actions := make([]string, 0, len(weights))
	for a := range weights {
		actions = append(actions, a)
	}
	sort.Strings(actions)
	cum := make([]float64, 0, len(actions))
	total := 0.0
	for _, a := range actions {
		w := weights[a]
		if w <= 0 {
			return nil, fmt.Errorf("action %q: weight must be > 0, got %v", a, w)
		}
		if known != nil {
			if _, ok := known[a]; !ok {
				return nil, fmt.Errorf("action %q is not defined in game_config actions_score_map", a)
			}
		}
		total += w
		cum = append(cum, total)
	}
	if err := think.Validate(); err != nil {
		return nil, fmt.Errorf("think_time: %w", err)
	}
	if err := session.Validate(); err != nil {
		return nil, fmt.Errorf("session_length: %w", err)
	}
	if lbProb < 0 || lbProb > 1 {
		return nil, fmt.Errorf("leaderboard_view_prob must be within [0,1], got %v", lbProb)
	}
	return &Persona{
		Name:                name,
		ThinkTime:           think,
		SessionLength:       session,
		LeaderboardViewProb: lbProb,
		actions:             actions,
		cum:                 cum,
	}, nil
}

// PickAction returns an action drawn according to the configured weights.
func (p *Persona) PickAction(r *rand.Rand) string {
	if len(p.actions) == 1 {
		return p.actions[0]
	}
	x := r.Float64() * p.cum[len(p.cum)-1]
	i := sort.SearchFloat64s(p.cum, x)
	if i >= len(p.actions) {
		i = len(p.actions) - 1
	}
	return p.actions[i]
}

// Actions lists the persona's actions in picking order.
func (p *Persona) Actions() []string {
	out := make([]string, len(p.actions))
	copy(out, p.actions)
	return out
}
