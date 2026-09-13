# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Go-based event-driven leaderboard engine for real-time multiplayer game scoring. Uses Kafka for async game action processing, Redis for leaderboard/XP storage, and ScyllaDB for user profiles.

## Commands

```bash
# Run the application
go run main.go

# Build
go build -o leaderboard-engine .

# Run the load-generation bot against a local server (scenarios live in bot/scenarios/)
BOT_SCENARIO_FILE=bot/scenarios/spike.yaml go run ./bot

# Bot unit tests
go test ./bot/...

# Build the bot image (context is the repo root)
docker build -f bot/Dockerfile -t leaderboard-bot:dev .

# Start infrastructure (Kafka, Redis cluster, ScyllaDB)
docker-compose up -d

# Start monitoring stack (VictoriaMetrics, Grafana on :3001)
docker-compose -f docker-compose.metrics.yml up -d
# After editing prometheus.yml, reload VictoriaMetrics without a restart
curl -X POST localhost:8428/-/reload
```

The server has no automated tests; manual API testing is done via `users.http` (IDE REST client format). The bot has unit tests under `bot/internal/`.

## Architecture

**Dependency injection** via `go.uber.org/fx` — all wiring is in `main.go`.

**Data flow:**
```
HTTP Request → GameActionsService → Kafka → Consumer Workers → Redis/ScyllaDB
```

**Layers:**
- `servers/` — HTTP (Fiber v3 beta) and Kafka consumer with concurrent workers
- `services/` — Business logic (game action handling, leaderboard aggregation)
- `repositories/` — Data access (Redis sorted sets for leaderboards/XP, ScyllaDB for profiles)
- `entities/` — Domain models and DTOs
- `app_config/` — Environment variable config via `go-envconfig`
- `game_config/` — Game scoring rules and level thresholds loaded from `game_config.json`
- `graceful_shutdown/` — Signal-based ordered shutdown (inputs stop → 10s delay → outputs stop)

**Kafka consumer concurrency:** Messages are routed to workers via modulo on leaderboard count (`servers/kafka.go`). Worker count is configurable via `KAFKA_LEADERBOARD_TOPIC_CONSUMER_CONCURRENCY` (default: 5).

**Key interfaces** are defined in `repositories/` and `services/` files — all repos and services use interface-based contracts.

## Load-generation bot (`bot/`)

Separate `main` package in the root module; imports `entities/` and `game_config/` directly. Traffic shape is defined in YAML scenarios (`bot/scenarios/*.yaml`: personas with weighted actions and think/session-time distributions, plus ordered phases with active-session targets, new-user ratio and persona mix). Process config is `BOT_*` env vars (`bot/internal/config`).

- `bot/internal/scenario` — schema, validation (actions checked against `game_config`), phase shapes (step/linear/sine)
- `bot/internal/engine` — 1s scheduler keeps active sessions at the phase target; goroutine per session; user pool with bounded sign-ups
- `bot/internal/client` — HTTP client with rate limiter, jittered backoff, and retry on 404 (the server reads profiles at consistency ONE right after a QUORUM write)
- `bot/internal/metrics` — `bot_*` Prometheus metrics on `BOT_METRICS_PORT` (default 9100) via the same VictoriaMetrics library as the server
- `bot/deploy/` — plain k8s manifests; scenario numbers are per pod, fleet size = replicas. See `bot/deploy/README.md`.

Per-action logs are Debug only. Never add per-request Info logging to the bot.

## Configuration

All config is via environment variables (see `app_config/app_config.go`). Key defaults:
- `FIBER_PORT`: 3000
- `KAFKA_BROKERS`: localhost:9092
- `SCYLLA_URL`: 127.0.0.1:9042
- `REDIS_URL`: 127.0.0.1:6379

## API Endpoints

- `POST /api/v1/users/sign-up` — Register user
- `GET /api/v1/users/:userId/profile` — Get profile
- `POST /api/v1/users/actions` — Submit game action
- `GET /leaderboards` — HTML leaderboard view
- `GET /metrics` — Prometheus metrics

## Monitoring

- Engine metrics on `:3000/metrics`, bot on `:9100/metrics`, both scraped by VictoriaMetrics via `prometheus.yml`.
- All engine metrics live in `telemetry/` and are `engine_`-prefixed: histograms are seconds (`_seconds`), counters end in `_total`, label values are bounded. Never build a metric name outside that package. HTTP metrics are labelled by route pattern (`route="/api/v1/users/:userId/profile"`), never the raw path; unmatched requests get `route="<unmatched>"`.
- Instrumentation points: HTTP middleware (`servers/middleware/metrics.go`), Kafka worker pipeline and reader stats (`servers/kafka.go`), producer writer stats (`services/game_actions.go`), Scylla via gocql `QueryObserver` with a per-method `op` label (`repositories/user_profile.go`), Redis via a `rueidishook` wrapper (`inits/redis.go`).
- Profiler: `go tool pprof http://localhost:3000/debug/pprof/profile?seconds=10` while under load.
- Infra exporters in `docker-compose.yml`: `kafka-exporter` (`:9308`, consumer lag as `kafka_consumergroup_lag_sum`), `redis-exporter` (`:9121`, multi-target over the six cluster nodes), Scylla's native endpoint (`:9180`). EKS manifests for the exporters against MSK/ElastiCache live in `deploy/kafka-exporter/` and `deploy/redis-exporter/`.
- Grafana dashboards are provisioned from `grafana/provisioning/dashboards/`: "Engine overview" (`engine-overview.json`) and "Kafka lag" (`kafka-lag.json`).
- `POST /backoffice-api/purge` — Clear all data (note: truncates ScyllaDB only; Redis leaderboards/XP are not cleared)
