package engine

import (
	"math/rand/v2"
	"sort"
)

// Plan decides how many sessions to start and how many to cancel this tick.
// Spawning is capped per tick to smooth sign-up bursts; cancelling only
// happens when cancelExcess is set (scenario `excess_sessions: cancel`),
// otherwise surplus sessions simply run out on their own.
func Plan(active, target, maxSpawn int, cancelExcess bool) (spawn, cancel int) {
	if target > active {
		spawn = target - active
		if spawn > maxSpawn {
			spawn = maxSpawn
		}
	}
	if cancelExcess && active > target {
		cancel = active - target
	}
	return spawn, cancel
}

// PickPersona draws a persona name according to the phase's weights. Keys are
// visited in sorted order so a fixed seed yields a fixed sequence.
func PickPersona(mix map[string]float64, r *rand.Rand) string {
	names := make([]string, 0, len(mix))
	total := 0.0
	for n, w := range mix {
		if w > 0 {
			names = append(names, n)
			total += w
		}
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	x := r.Float64() * total
	acc := 0.0
	for _, n := range names {
		acc += mix[n]
		if x < acc {
			return n
		}
	}
	return names[len(names)-1]
}
