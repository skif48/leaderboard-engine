// Package scenario defines the YAML traffic-scenario schema and its
// validation. A scenario is a set of personas plus an ordered list of phases;
// each phase says how many sessions should be active over time, what share of
// them should be brand-new users, and which personas they use.
package scenario

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/skif48/leaderboard-engine/bot/internal/persona"
)

const (
	ShapeLinear = "linear"
	ShapeSine   = "sine"

	ExcessDrain  = "drain"
	ExcessCancel = "cancel"
)

// Duration is a time.Duration that unmarshals from Go duration strings ("30s", "5m").
type Duration time.Duration

func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	var s string
	if err := n.Decode(&s); err != nil {
		return fmt.Errorf("duration must be a string such as 30s or 5m: %w", err)
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	*d = Duration(v)
	return nil
}

func (d Duration) D() time.Duration { return time.Duration(d) }

type DistSpec struct {
	Mean   Duration `yaml:"mean"`
	Stddev Duration `yaml:"stddev"`
	Min    Duration `yaml:"min"`
	Max    Duration `yaml:"max"`
}

func (s DistSpec) dist() persona.Dist {
	return persona.Dist{Mean: s.Mean.D(), Stddev: s.Stddev.D(), Min: s.Min.D(), Max: s.Max.D()}
}

type PersonaSpec struct {
	Actions             map[string]float64 `yaml:"actions"`
	ThinkTime           DistSpec           `yaml:"think_time"`
	SessionLength       DistSpec           `yaml:"session_length"`
	LeaderboardViewProb float64            `yaml:"leaderboard_view_prob"`
}

// ActiveUsers accepts either a bare integer (constant) or a mapping with
// from/to (linear ramp) or min/max (sine).
type ActiveUsers struct {
	Scalar *int
	From   *int
	To     *int
	Min    *int
	Max    *int
}

func (a *ActiveUsers) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		var v int
		if err := n.Decode(&v); err != nil {
			return fmt.Errorf("active_users: %w", err)
		}
		a.Scalar = &v
		return nil
	case yaml.MappingNode:
		var aux struct {
			From *int `yaml:"from"`
			To   *int `yaml:"to"`
			Min  *int `yaml:"min"`
			Max  *int `yaml:"max"`
		}
		if err := n.Decode(&aux); err != nil {
			return fmt.Errorf("active_users: %w", err)
		}
		a.From, a.To, a.Min, a.Max = aux.From, aux.To, aux.Min, aux.Max
		return nil
	default:
		return errors.New("active_users must be an integer or a mapping")
	}
}

type Phase struct {
	Name           string             `yaml:"name"`
	Duration       Duration           `yaml:"duration"`
	ActiveUsers    ActiveUsers        `yaml:"active_users"`
	Shape          string             `yaml:"shape"`
	Period         Duration           `yaml:"period"`
	NewUserRatio   float64            `yaml:"new_user_ratio"`
	PersonaMix     map[string]float64 `yaml:"persona_mix"`
	ExcessSessions string             `yaml:"excess_sessions"`

	// resolved by Validate
	from, to int
	offset   time.Duration
}

type Scenario struct {
	Name           string                 `yaml:"name"`
	Loop           bool                   `yaml:"loop"`
	ExcessSessions string                 `yaml:"excess_sessions"`
	Personas       map[string]PersonaSpec `yaml:"personas"`
	Phases         []Phase                `yaml:"phases"`

	total     time.Duration
	validated bool
}

// Load reads and parses a scenario file. Call Validate before using it.
func Load(path string) (*Scenario, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(b)
}

// Parse decodes YAML strictly: unknown fields are errors, which catches typos
// such as `new_users_ratio`.
func Parse(b []byte) (*Scenario, error) {
	s := &Scenario{}
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(s); err != nil {
		return nil, fmt.Errorf("parse scenario: %w", err)
	}
	return s, nil
}

// Validate checks the scenario against the game's action set and resolves
// derived values (ramp start values, phase offsets, defaults).
func (s *Scenario) Validate(actions map[string]int) error {
	if s.Name == "" {
		return errors.New("name is required")
	}
	if len(s.Personas) == 0 {
		return errors.New("at least one persona is required")
	}
	if len(s.Phases) == 0 {
		return errors.New("at least one phase is required")
	}
	if s.ExcessSessions == "" {
		s.ExcessSessions = ExcessDrain
	}
	if err := validateExcess(s.ExcessSessions); err != nil {
		return err
	}
	for name, ps := range s.Personas {
		if _, err := ps.build(name, actions); err != nil {
			return fmt.Errorf("persona %q: %w", name, err)
		}
	}

	prevEnd := 0
	var offset time.Duration
	last := len(s.Phases) - 1
	for i := range s.Phases {
		p := &s.Phases[i]
		if p.Name == "" {
			p.Name = fmt.Sprintf("phase-%d", i+1)
		}
		wrap := func(err error) error { return fmt.Errorf("phase %q: %w", p.Name, err) }

		if p.Duration < 0 {
			return wrap(errors.New("duration must be >= 0"))
		}
		if p.Duration == 0 && i != last {
			return wrap(errors.New("duration 0 (hold forever) is only allowed on the last phase"))
		}
		if p.Shape == "" {
			p.Shape = ShapeLinear
		}
		au := p.ActiveUsers
		switch p.Shape {
		case ShapeLinear:
			if au.Min != nil || au.Max != nil {
				return wrap(errors.New("active_users min/max are only valid with shape: sine"))
			}
			switch {
			case au.Scalar != nil:
				p.from, p.to = *au.Scalar, *au.Scalar
			case au.To != nil:
				p.to = *au.To
				p.from = prevEnd
				if au.From != nil {
					p.from = *au.From
				}
			default:
				return wrap(errors.New("active_users is required (integer or {from, to})"))
			}
			if p.from < 0 || p.to < 0 {
				return wrap(errors.New("active_users must be >= 0"))
			}
			if p.Duration == 0 && p.from != p.to {
				return wrap(errors.New("a ramp needs a duration > 0"))
			}
		case ShapeSine:
			if au.Scalar != nil || au.From != nil || au.To != nil {
				return wrap(errors.New("shape: sine requires active_users {min, max}"))
			}
			if au.Min == nil || au.Max == nil {
				return wrap(errors.New("shape: sine requires active_users {min, max}"))
			}
			if *au.Min < 0 || *au.Max < *au.Min {
				return wrap(errors.New("active_users must satisfy 0 <= min <= max"))
			}
			if p.Period <= 0 {
				return wrap(errors.New("shape: sine requires period > 0"))
			}
		default:
			return wrap(fmt.Errorf("unknown shape %q (linear|sine)", p.Shape))
		}
		if p.NewUserRatio < 0 || p.NewUserRatio > 1 {
			return wrap(errors.New("new_user_ratio must be within [0,1]"))
		}
		if len(p.PersonaMix) == 0 {
			p.PersonaMix = make(map[string]float64, len(s.Personas))
			for name := range s.Personas {
				p.PersonaMix[name] = 1
			}
		}
		for name, w := range p.PersonaMix {
			if _, ok := s.Personas[name]; !ok {
				return wrap(fmt.Errorf("persona_mix references unknown persona %q", name))
			}
			if w <= 0 {
				return wrap(fmt.Errorf("persona_mix weight for %q must be > 0", name))
			}
		}
		if p.ExcessSessions == "" {
			p.ExcessSessions = s.ExcessSessions
		}
		if err := validateExcess(p.ExcessSessions); err != nil {
			return wrap(err)
		}
		p.offset = offset
		offset += p.Duration.D()
		prevEnd = p.TargetAt(p.Duration.D())
	}
	s.total = offset
	if s.Loop && s.Phases[last].Duration == 0 {
		return errors.New("loop: true requires the last phase to have a duration")
	}
	s.validated = true
	return nil
}

func validateExcess(v string) error {
	switch v {
	case ExcessDrain, ExcessCancel:
		return nil
	default:
		return fmt.Errorf("excess_sessions must be %q or %q, got %q", ExcessDrain, ExcessCancel, v)
	}
}

func (ps PersonaSpec) build(name string, actions map[string]int) (*persona.Persona, error) {
	return persona.New(name, ps.Actions, ps.ThinkTime.dist(), ps.SessionLength.dist(), ps.LeaderboardViewProb, actions)
}

// BuildPersonas instantiates every persona in the scenario.
func (s *Scenario) BuildPersonas(actions map[string]int) (map[string]*persona.Persona, error) {
	out := make(map[string]*persona.Persona, len(s.Personas))
	for name, ps := range s.Personas {
		p, err := ps.build(name, actions)
		if err != nil {
			return nil, fmt.Errorf("persona %q: %w", name, err)
		}
		out[name] = p
	}
	return out, nil
}

// PersonaNames returns persona names in sorted order (for logging).
func (s *Scenario) PersonaNames() []string {
	names := make([]string, 0, len(s.Personas))
	for n := range s.Personas {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Total is the sum of finite phase durations. A trailing phase with duration 0
// does not contribute.
func (s *Scenario) Total() time.Duration { return s.total }

// Validated reports whether Validate succeeded.
func (s *Scenario) Validated() bool { return s.validated }
