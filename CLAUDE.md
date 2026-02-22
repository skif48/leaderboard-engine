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

# Run the load testing bot
cd bot && go run main.go

# Start infrastructure (Kafka, Redis cluster, ScyllaDB)
docker-compose up -d

# Start monitoring stack (VictoriaMetrics, Grafana)
docker-compose -f docker-compose.metrics.yml up -d
```

No automated tests exist yet. Manual API testing is done via `users.http` (IDE REST client format).

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
- `POST /backoffice-api/purge` — Clear all data