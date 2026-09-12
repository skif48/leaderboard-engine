package main

import (
	"path/filepath"
	"testing"

	"github.com/skif48/leaderboard-engine/bot/internal/scenario"
	"github.com/skif48/leaderboard-engine/game_config"
)

// Every bundled scenario must parse, validate against the real game config,
// and build its personas. This is the check that catches a renamed action or
// a typo in a scenario file before it reaches a cluster.
func TestBundledScenariosAreValid(t *testing.T) {
	files, err := filepath.Glob("scenarios/*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 4 {
		t.Fatalf("expected at least 4 bundled scenarios, found %d", len(files))
	}
	gc := game_config.NewGameConfig()
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			sc, err := scenario.Load(f)
			if err != nil {
				t.Fatal(err)
			}
			if err := sc.Validate(gc.ActionsScoreMap); err != nil {
				t.Fatal(err)
			}
			personas, err := sc.BuildPersonas(gc.ActionsScoreMap)
			if err != nil {
				t.Fatal(err)
			}
			if len(personas) == 0 {
				t.Fatal("no personas built")
			}
			for _, p := range sc.Phases {
				if p.TargetAt(0) < 0 || p.TargetAt(p.Duration.D()) < 0 {
					t.Fatalf("phase %q yields a negative target", p.Name)
				}
			}
		})
	}
}
