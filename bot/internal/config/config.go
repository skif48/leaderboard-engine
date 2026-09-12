// Package config loads the bot's process-level configuration from environment
// variables. Everything that describes *traffic shape* lives in the scenario
// YAML instead; env vars only cover where to send traffic, how to identify this
// instance, hard safety caps and operational knobs.
package config

import (
	"context"
	"fmt"
	"hash/fnv"
	"os"
	"time"

	"github.com/sethvargo/go-envconfig"
)

type Config struct {
	// BaseURL is the leaderboard-engine HTTP endpoint (in-cluster: the ClusterIP service).
	BaseURL string `env:"BOT_BASE_URL, default=http://localhost:3000"`
	// ScenarioFile is the YAML scenario to run. The image bakes the bundled
	// scenarios into /scenarios; a ConfigMap can be mounted over that path.
	ScenarioFile string `env:"BOT_SCENARIO_FILE, default=/scenarios/steady.yaml"`
	// InstanceID distinguishes replicas (nickname tag, seed derivation). Defaults to the hostname.
	InstanceID string `env:"BOT_INSTANCE_ID"`
	// MaxRPS is a global request-rate cap across all sessions of this instance. 0 disables it.
	MaxRPS float64 `env:"BOT_MAX_RPS, default=0"`
	// MaxConns bounds in-flight HTTP requests (transport MaxConnsPerHost).
	MaxConns int `env:"BOT_MAX_CONNS, default=200"`
	// MaxUsers caps how many users this instance registers over its lifetime. 0 = unlimited.
	MaxUsers int `env:"BOT_MAX_USERS, default=0"`
	// MaxSpawnPerSec bounds how many sessions the scheduler starts per tick, smoothing sign-up bursts.
	MaxSpawnPerSec int `env:"BOT_MAX_SPAWN_PER_SEC, default=100"`
	// SignupConcurrency bounds concurrent sign-up requests.
	SignupConcurrency int `env:"BOT_SIGNUP_CONCURRENCY, default=16"`
	// MetricsPort serves /metrics and /health.
	MetricsPort int    `env:"BOT_METRICS_PORT, default=9100"`
	LogLevel    string `env:"LOG_LEVEL, default=info"`
	// LoopScenario forces the scenario to repeat when it ends (overrides the scenario's own `loop`).
	LoopScenario bool `env:"BOT_LOOP_SCENARIO, default=false"`
	// Seed makes nickname/action sequences reproducible for a given InstanceID. 0 = time-based.
	Seed int64 `env:"BOT_SEED, default=0"`
	// ShutdownTimeout bounds how long Run waits for sessions after SIGTERM. Keep it below the
	// pod's terminationGracePeriodSeconds.
	ShutdownTimeout time.Duration `env:"BOT_SHUTDOWN_TIMEOUT, default=10s"`
	// SummaryInterval controls the periodic Info summary line. 0 disables it.
	SummaryInterval time.Duration `env:"BOT_SUMMARY_INTERVAL, default=10s"`
}

func Load(ctx context.Context) (*Config, error) {
	c := &Config{}
	if err := envconfig.Process(ctx, c); err != nil {
		return nil, err
	}
	if c.InstanceID == "" {
		h, err := os.Hostname()
		if err != nil || h == "" {
			h = "local"
		}
		c.InstanceID = h
	}
	if c.BaseURL == "" {
		return nil, fmt.Errorf("BOT_BASE_URL must not be empty")
	}
	if c.MaxRPS < 0 {
		return nil, fmt.Errorf("BOT_MAX_RPS must be >= 0, got %v", c.MaxRPS)
	}
	if c.MaxConns <= 0 {
		return nil, fmt.Errorf("BOT_MAX_CONNS must be > 0, got %d", c.MaxConns)
	}
	if c.MaxUsers < 0 {
		return nil, fmt.Errorf("BOT_MAX_USERS must be >= 0, got %d", c.MaxUsers)
	}
	if c.MaxSpawnPerSec <= 0 {
		return nil, fmt.Errorf("BOT_MAX_SPAWN_PER_SEC must be > 0, got %d", c.MaxSpawnPerSec)
	}
	if c.SignupConcurrency <= 0 {
		return nil, fmt.Errorf("BOT_SIGNUP_CONCURRENCY must be > 0, got %d", c.SignupConcurrency)
	}
	if c.MetricsPort <= 0 || c.MetricsPort > 65535 {
		return nil, fmt.Errorf("BOT_METRICS_PORT out of range: %d", c.MetricsPort)
	}
	if c.ShutdownTimeout <= 0 {
		return nil, fmt.Errorf("BOT_SHUTDOWN_TIMEOUT must be > 0")
	}
	return c, nil
}

// EffectiveSeed folds the instance id into the configured seed so that replicas
// sharing the same BOT_SEED still produce different nicknames and action streams.
func (c *Config) EffectiveSeed() uint64 {
	seed := c.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(c.InstanceID))
	return uint64(seed) ^ h.Sum64()
}

// InstanceTag is a short, stable suffix derived from InstanceID used to keep
// nicknames unique across the fleet.
func (c *Config) InstanceTag() string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(c.InstanceID))
	return fmt.Sprintf("%04x", h.Sum32()&0xffff)
}
