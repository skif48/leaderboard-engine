package scenario

import (
	"strings"
	"testing"
	"time"
)

var actions = map[string]int{"spawn": 1, "kill": 8, "heal": 3}

const personasYAML = `
personas:
  casual:
    actions: {spawn: 1, heal: 1}
    think_time: {mean: 1s, stddev: 200ms, min: 100ms, max: 3s}
    session_length: {mean: 1m, stddev: 10s, min: 10s, max: 2m}
  grinder:
    actions: {kill: 1}
    think_time: {mean: 500ms, min: 100ms, max: 1s}
    session_length: {mean: 2m, min: 30s, max: 5m}
`

func mustParse(t *testing.T, yml string) *Scenario {
	t.Helper()
	s, err := Parse([]byte(yml))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Validate(actions); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestLinearInterpolationAndFromInheritance(t *testing.T) {
	s := mustParse(t, `name: t`+personasYAML+`
phases:
  - {name: a, duration: 10s, active_users: 50, persona_mix: {casual: 1}}
  - {name: b, duration: 10s, active_users: {to: 150}, persona_mix: {casual: 1}}
  - {name: c, duration: 10s, active_users: {from: 20, to: 40}, persona_mix: {grinder: 1}}
`)
	a, b, c := &s.Phases[0], &s.Phases[1], &s.Phases[2]
	if got := a.TargetAt(5 * time.Second); got != 50 {
		t.Fatalf("step phase: got %d want 50", got)
	}
	if from, to := b.Range(); from != 50 || to != 150 {
		t.Fatalf("from inheritance: got %d..%d want 50..150", from, to)
	}
	if got := b.TargetAt(5 * time.Second); got != 100 {
		t.Fatalf("midpoint: got %d want 100", got)
	}
	if got := b.TargetAt(20 * time.Second); got != 150 {
		t.Fatalf("past end clamps: got %d want 150", got)
	}
	if from, to := c.Range(); from != 20 || to != 40 {
		t.Fatalf("explicit from: got %d..%d", from, to)
	}
	if s.Total() != 30*time.Second {
		t.Fatalf("total = %v", s.Total())
	}
	if c.Offset() != 20*time.Second {
		t.Fatalf("offset = %v", c.Offset())
	}
}

func TestSineShape(t *testing.T) {
	s := mustParse(t, `name: t`+personasYAML+`
phases:
  - {name: d, duration: 60s, shape: sine, period: 60s, active_users: {min: 10, max: 110}}
`)
	p := &s.Phases[0]
	cases := map[time.Duration]int{0: 10, 15 * time.Second: 60, 30 * time.Second: 110, 45 * time.Second: 60, 60 * time.Second: 10}
	for at, want := range cases {
		if got := p.TargetAt(at); got != want {
			t.Fatalf("sine at %v: got %d want %d", at, got, want)
		}
	}
}

func TestLocateLoopAndDone(t *testing.T) {
	s := mustParse(t, `name: t`+personasYAML+`
phases:
  - {name: a, duration: 10s, active_users: 1}
  - {name: b, duration: 5s, active_users: 2}
`)
	idx, pe, loops, done := s.Locate(12*time.Second, false)
	if idx != 1 || pe != 2*time.Second || loops != 0 || done {
		t.Fatalf("locate in b: idx=%d pe=%v loops=%d done=%v", idx, pe, loops, done)
	}
	if _, _, _, done := s.Locate(15*time.Second, false); !done {
		t.Fatal("expected done at end of non-looping scenario")
	}
	idx, pe, loops, done = s.Locate(33*time.Second, true)
	if idx != 0 || pe != 3*time.Second || loops != 2 || done {
		t.Fatalf("locate looped: idx=%d pe=%v loops=%d done=%v", idx, pe, loops, done)
	}
}

func TestLocateInfiniteLastPhase(t *testing.T) {
	s := mustParse(t, `name: t`+personasYAML+`
phases:
  - {name: a, duration: 10s, active_users: 1}
  - {name: hold, duration: 0, active_users: 2}
`)
	idx, pe, _, done := s.Locate(time.Hour, false)
	if idx != 1 || pe != time.Hour-10*time.Second || done {
		t.Fatalf("infinite hold: idx=%d pe=%v done=%v", idx, pe, done)
	}
}

func TestDefaultsAndValidationErrors(t *testing.T) {
	s := mustParse(t, `name: t`+personasYAML+`
phases:
  - {duration: 10s, active_users: 1}
`)
	p := s.Phases[0]
	if p.Name != "phase-1" || p.ExcessSessions != ExcessDrain || len(p.PersonaMix) != 2 || p.Shape != ShapeLinear {
		t.Fatalf("defaults not applied: %+v", p)
	}

	bad := []struct{ name, yml, want string }{
		{"unknown field", `name: t` + personasYAML + `
phases:
  - {duration: 10s, active_users: 1, new_users_ratio: 0.5}`, "field new_users_ratio not found"},
		{"unknown action", `name: t
personas:
  x: {actions: {fly: 1}, think_time: {mean: 1s}, session_length: {mean: 1s}}
phases:
  - {duration: 10s, active_users: 1}`, "not defined in game_config"},
		{"zero duration mid-scenario", `name: t` + personasYAML + `
phases:
  - {duration: 0, active_users: 1}
  - {duration: 10s, active_users: 1}`, "only allowed on the last phase"},
		{"unknown persona in mix", `name: t` + personasYAML + `
phases:
  - {duration: 10s, active_users: 1, persona_mix: {nobody: 1}}`, "unknown persona"},
		{"bad ratio", `name: t` + personasYAML + `
phases:
  - {duration: 10s, active_users: 1, new_user_ratio: 2}`, "new_user_ratio"},
		{"sine without period", `name: t` + personasYAML + `
phases:
  - {duration: 10s, shape: sine, active_users: {min: 1, max: 2}}`, "period"},
		{"loop with infinite phase", `name: t
loop: true` + personasYAML + `
phases:
  - {duration: 0, active_users: 1}`, "loop: true requires"},
		{"bad excess", `name: t` + personasYAML + `
phases:
  - {duration: 10s, active_users: 1, excess_sessions: kill}`, "excess_sessions"},
	}
	for _, c := range bad {
		t.Run(c.name, func(t *testing.T) {
			s, err := Parse([]byte(c.yml))
			if err == nil {
				err = s.Validate(actions)
			}
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("got err %v, want containing %q", err, c.want)
			}
		})
	}
}
