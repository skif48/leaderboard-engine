package persona

import (
	"math"
	"math/rand/v2"
	"testing"
	"time"
)

var known = map[string]int{"spawn": 1, "kill": 8, "heal": 3}

func fixedRand() *rand.Rand { return rand.New(rand.NewPCG(1, 2)) }

func validDist() Dist {
	return Dist{Mean: time.Second, Stddev: 200 * time.Millisecond, Min: 100 * time.Millisecond, Max: 5 * time.Second}
}

func TestPickActionFollowsWeights(t *testing.T) {
	p, err := New("p", map[string]float64{"spawn": 1, "kill": 3}, validDist(), validDist(), 0, known)
	if err != nil {
		t.Fatal(err)
	}
	r := fixedRand()
	counts := map[string]int{}
	const n = 20000
	for i := 0; i < n; i++ {
		counts[p.PickAction(r)]++
	}
	if counts["spawn"]+counts["kill"] != n {
		t.Fatalf("unexpected actions: %v", counts)
	}
	killShare := float64(counts["kill"]) / n
	if math.Abs(killShare-0.75) > 0.02 {
		t.Fatalf("kill share = %.3f, want ~0.75", killShare)
	}
}

func TestPickActionDeterministicForSeed(t *testing.T) {
	p, _ := New("p", map[string]float64{"spawn": 1, "kill": 1, "heal": 1}, validDist(), validDist(), 0, known)
	a, b := fixedRand(), fixedRand()
	for i := 0; i < 100; i++ {
		if x, y := p.PickAction(a), p.PickAction(b); x != y {
			t.Fatalf("sequence diverged at %d: %s vs %s", i, x, y)
		}
	}
}

func TestNewRejectsBadInput(t *testing.T) {
	cases := []struct {
		name    string
		weights map[string]float64
		think   Dist
		lb      float64
	}{
		{"unknown action", map[string]float64{"fly": 1}, validDist(), 0},
		{"zero weight", map[string]float64{"spawn": 0}, validDist(), 0},
		{"empty weights", map[string]float64{}, validDist(), 0},
		{"bad think mean", map[string]float64{"spawn": 1}, Dist{Mean: 0}, 0},
		{"mean below min", map[string]float64{"spawn": 1}, Dist{Mean: time.Second, Min: 2 * time.Second}, 0},
		{"max below min", map[string]float64{"spawn": 1}, Dist{Mean: time.Second, Min: time.Second, Max: 500 * time.Millisecond}, 0},
		{"prob out of range", map[string]float64{"spawn": 1}, validDist(), 1.5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := New("p", c.weights, c.think, validDist(), c.lb, known); err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}

func TestDistSampleClamps(t *testing.T) {
	d := Dist{Mean: time.Second, Stddev: 10 * time.Second, Min: 500 * time.Millisecond, Max: 2 * time.Second}
	r := fixedRand()
	for i := 0; i < 1000; i++ {
		v := d.Sample(r)
		if v < d.Min || v > d.Max {
			t.Fatalf("sample %v outside [%v,%v]", v, d.Min, d.Max)
		}
	}
	constant := Dist{Mean: 3 * time.Second}
	if v := constant.Sample(r); v != 3*time.Second {
		t.Fatalf("stddev 0 should be constant, got %v", v)
	}
}
