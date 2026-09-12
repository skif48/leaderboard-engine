package engine

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestPlan(t *testing.T) {
	cases := []struct {
		active, target, maxSpawn int
		cancelExcess             bool
		spawn, cancel            int
	}{
		{0, 10, 100, false, 10, 0},
		{0, 500, 100, false, 100, 0},
		{10, 10, 100, false, 0, 0},
		{50, 10, 100, false, 0, 0},
		{50, 10, 100, true, 0, 40},
		{10, 50, 5, true, 5, 0},
	}
	for _, c := range cases {
		spawn, cancel := Plan(c.active, c.target, c.maxSpawn, c.cancelExcess)
		if spawn != c.spawn || cancel != c.cancel {
			t.Fatalf("Plan(%d,%d,%d,%v) = (%d,%d), want (%d,%d)",
				c.active, c.target, c.maxSpawn, c.cancelExcess, spawn, cancel, c.spawn, c.cancel)
		}
	}
}

func TestPickPersona(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 11))
	mix := map[string]float64{"casual": 3, "grinder": 1, "ignored": 0}
	counts := map[string]int{}
	const n = 20000
	for i := 0; i < n; i++ {
		counts[PickPersona(mix, r)]++
	}
	if counts["ignored"] != 0 {
		t.Fatal("zero-weight persona was picked")
	}
	if share := float64(counts["casual"]) / n; math.Abs(share-0.75) > 0.02 {
		t.Fatalf("casual share %.3f, want ~0.75", share)
	}
	if PickPersona(map[string]float64{}, r) != "" {
		t.Fatal("empty mix should yield empty name")
	}
}
