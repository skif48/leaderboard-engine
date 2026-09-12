// Command bot is a scenario-driven traffic generator for leaderboard-engine.
//
// It simulates a population of players: sessions come and go according to a
// YAML scenario (steady load, spikes, growth, diurnal cycles), each session
// behaves like one of the scenario's personas, and everything it does is
// exposed as Prometheus metrics on BOT_METRICS_PORT. Process configuration is
// read from BOT_* environment variables (see internal/config); traffic shape
// lives entirely in the scenario file.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/skif48/leaderboard-engine/bot/internal/client"
	"github.com/skif48/leaderboard-engine/bot/internal/config"
	"github.com/skif48/leaderboard-engine/bot/internal/engine"
	"github.com/skif48/leaderboard-engine/bot/internal/logx"
	"github.com/skif48/leaderboard-engine/bot/internal/metrics"
	"github.com/skif48/leaderboard-engine/bot/internal/names"
	"github.com/skif48/leaderboard-engine/bot/internal/scenario"
	"github.com/skif48/leaderboard-engine/game_config"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Log at Info until the configured level is known.
	setupLogger(slog.LevelInfo)

	cfg, err := config.Load(ctx)
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		return 1
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		slog.Error("invalid LOG_LEVEL", "value", cfg.LogLevel, "error", err)
		return 1
	}
	setupLogger(level)

	gc := game_config.NewGameConfig()
	sc, err := scenario.Load(cfg.ScenarioFile)
	if err != nil {
		slog.Error("failed to load scenario", "file", cfg.ScenarioFile, "error", err)
		return 1
	}
	if err := sc.Validate(gc.ActionsScoreMap); err != nil {
		slog.Error("invalid scenario", "file", cfg.ScenarioFile, "error", err)
		return 1
	}
	personas, err := sc.BuildPersonas(gc.ActionsScoreMap)
	if err != nil {
		slog.Error("failed to build personas", "error", err)
		return 1
	}

	throttle := logx.New(10 * time.Second)
	cl := client.New(client.Options{
		BaseURL:  cfg.BaseURL,
		MaxConns: cfg.MaxConns,
		MaxRPS:   cfg.MaxRPS,
		Throttle: throttle,
	})
	seed := cfg.EffectiveSeed()
	gen := names.New(seed, cfg.InstanceTag())
	pop := engine.NewPopulation(cl, gen, cfg.MaxUsers, cfg.SignupConcurrency)
	sched, err := engine.New(engine.Options{
		MaxSpawnPerTick: cfg.MaxSpawnPerSec,
		ShutdownTimeout: cfg.ShutdownTimeout,
		SummaryInterval: cfg.SummaryInterval,
		Loop:            cfg.LoopScenario,
		Seed:            seed,
	}, sc, personas, pop, cl, throttle)
	if err != nil {
		slog.Error("failed to create scheduler", "error", err)
		return 1
	}

	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()
	metricsErr := make(chan error, 1)
	go func() {
		err := metrics.Serve(runCtx, cfg.MetricsPort)
		if err != nil {
			slog.Error("metrics server failed", "port", cfg.MetricsPort, "error", err)
			cancelRun()
		}
		metricsErr <- err
	}()

	slog.Info("bot starting",
		"base_url", cfg.BaseURL,
		"scenario", sc.Name,
		"scenario_file", cfg.ScenarioFile,
		"instance_id", cfg.InstanceID,
		"instance_tag", cfg.InstanceTag(),
		"seed", cfg.Seed,
		"loop_override", cfg.LoopScenario,
		"max_rps", cfg.MaxRPS,
		"max_conns", cfg.MaxConns,
		"max_users", cfg.MaxUsers,
		"max_spawn_per_sec", cfg.MaxSpawnPerSec,
		"signup_concurrency", cfg.SignupConcurrency,
		"metrics_port", cfg.MetricsPort,
		"shutdown_timeout", cfg.ShutdownTimeout.String())

	if err := sched.Run(runCtx); err != nil {
		slog.Error("scheduler failed", "error", err)
		return 1
	}
	cancelRun()
	if err := <-metricsErr; err != nil {
		return 1
	}
	slog.Info("bot stopped")
	return 0
}

func setupLogger(level slog.Level) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)
}
