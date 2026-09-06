// Package names generates friendly, human-readable nicknames. The generator is
// safe for concurrent use, deterministic for a given seed, and tags every name
// with a short instance suffix so replicas of the bot never collide.
package names

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"sync"
)

var (
	adjectives = []string{
		"Happy", "Clever", "Bright", "Swift", "Kind", "Gentle", "Brave", "Calm",
		"Cheerful", "Wise", "Friendly", "Jolly", "Lively", "Merry", "Noble", "Peaceful",
		"Quick", "Smart", "Sunny", "Warm", "Amazing", "Awesome", "Cool", "Epic",
		"Fantastic", "Great", "Mighty", "Super", "Wonderful", "Brilliant", "Creative",
		"Dynamic", "Energetic", "Graceful", "Humble", "Inspiring", "Joyful",
	}
	nouns = []string{
		"Explorer", "Builder", "Creator", "Dreamer", "Hunter", "Seeker", "Wanderer", "Guardian",
		"Champion", "Hero", "Legend", "Master", "Pioneer", "Sage", "Scholar", "Warrior",
		"Artist", "Inventor", "Navigator", "Pilot", "Runner", "Swimmer", "Climber", "Dancer",
		"Singer", "Writer", "Player", "Gamer", "Coder", "Hacker", "Ninja", "Wizard",
		"Knight", "Ranger", "Scout", "Captain", "Admiral", "General", "Commander", "Leader",
	}
	colors = []string{
		"Blue", "Green", "Red", "Purple", "Orange", "Yellow", "Pink", "Cyan",
		"Silver", "Gold", "Crimson", "Azure", "Emerald", "Violet", "Amber", "Rose",
		"Coral", "Mint", "Lime", "Teal", "Indigo", "Magenta", "Turquoise", "Lavender",
	}
)

type Generator struct {
	mu   sync.Mutex
	r    *rand.Rand
	used map[string]struct{}
	tag  string
}

// New returns a generator seeded deterministically. tag (may be empty) is
// appended to every nickname as "-tag".
func New(seed uint64, tag string) *Generator {
	return &Generator{
		r:    rand.New(rand.NewPCG(seed, seed^0x9E3779B97F4A7C15)),
		used: map[string]struct{}{},
		tag:  tag,
	}
}

func (g *Generator) pick(list []string) string { return list[g.r.IntN(len(list))] }

func (g *Generator) candidate() string {
	switch g.r.IntN(5) {
	case 0:
		return g.pick(adjectives) + g.pick(nouns)
	case 1:
		return g.pick(colors) + g.pick(nouns)
	case 2:
		return g.pick(adjectives) + g.pick(colors) + g.pick(nouns)
	case 3:
		return fmt.Sprintf("%s%d", g.pick(nouns), g.r.IntN(100)+1)
	default:
		return fmt.Sprintf("%s%s%d", g.pick(adjectives), g.pick(nouns), g.r.IntN(100)+1)
	}
}

// Unique returns a nickname not previously returned by this generator.
func (g *Generator) Unique() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	for attempt := 0; attempt < 100; attempt++ {
		n := g.candidate()
		if g.claim(n) {
			return g.withTag(n)
		}
	}
	for {
		n := fmt.Sprintf("%s%d", g.candidate(), g.r.IntN(1_000_000))
		if g.claim(n) {
			return g.withTag(n)
		}
	}
}

func (g *Generator) claim(n string) bool {
	key := strings.ToLower(n)
	if _, taken := g.used[key]; taken {
		return false
	}
	g.used[key] = struct{}{}
	return true
}

func (g *Generator) withTag(n string) string {
	if g.tag == "" {
		return n
	}
	return n + "-" + g.tag
}
