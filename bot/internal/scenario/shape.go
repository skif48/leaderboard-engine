package scenario

import (
	"math"
	"time"
)

// Range returns the resolved start and end target for a linear phase. For a
// sine phase it returns min and max.
func (p *Phase) Range() (from, to int) {
	if p.Shape == ShapeSine {
		return *p.ActiveUsers.Min, *p.ActiveUsers.Max
	}
	return p.from, p.to
}

// Offset is the phase's start time relative to scenario start.
func (p *Phase) Offset() time.Duration { return p.offset }

// TargetAt returns the desired number of active sessions `elapsed` into the phase.
func (p *Phase) TargetAt(elapsed time.Duration) int {
	if elapsed < 0 {
		elapsed = 0
	}
	switch p.Shape {
	case ShapeSine:
		mn, mx := float64(*p.ActiveUsers.Min), float64(*p.ActiveUsers.Max)
		t := float64(elapsed) / float64(p.Period.D())
		v := mn + (mx-mn)*(1-math.Cos(2*math.Pi*t))/2
		return int(math.Round(v))
	default:
		d := p.Duration.D()
		if d <= 0 || elapsed >= d {
			return p.to
		}
		frac := float64(elapsed) / float64(d)
		return int(math.Round(float64(p.from) + float64(p.to-p.from)*frac))
	}
}

// Locate maps scenario-relative elapsed time to a phase. loops counts how many
// full passes have completed when looping; done is true when a non-looping
// scenario has run past its last finite phase.
func (s *Scenario) Locate(elapsed time.Duration, loop bool) (idx int, phaseElapsed time.Duration, loops int, done bool) {
	last := len(s.Phases) - 1
	lp := &s.Phases[last]
	infinite := lp.Duration == 0
	if elapsed < 0 {
		elapsed = 0
	}
	if elapsed >= s.total {
		if infinite {
			return last, elapsed - lp.offset, 0, false
		}
		if !loop || s.total <= 0 {
			return last, lp.Duration.D(), 0, true
		}
		loops = int(elapsed / s.total)
		elapsed = elapsed % s.total
	}
	for i := range s.Phases {
		p := &s.Phases[i]
		if p.Duration == 0 || elapsed < p.offset+p.Duration.D() {
			return i, elapsed - p.offset, loops, false
		}
	}
	return last, elapsed - lp.offset, loops, false
}
