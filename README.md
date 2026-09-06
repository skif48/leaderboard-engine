# leaderboard-engine

## Performance Observations

| Unique Users | Actions/sec | Kafka Avg Latency (ms) | Consumer Concurrency | Scylla Pool Size | Workers Utilization |
|--------------|-------------|------------------------|----------------------|------------------|---------------------|
| 100          | 400         | 3.7                    | 5                    | 2                | ~30%                |
| 500          | 2,000       | 3.7                    | 5                    | 2                | ~82%                |
| 500          | 2,000       | 4.84                   | 10                   | 2                | ~68%                |

## Load-generation bot

`bot/` is a scenario-driven traffic simulator. Instead of N users on a fixed
ticker it runs *sessions*: a session picks a persona (weighted actions, think
time, session length, leaderboard-view probability), acquires a user from a
pool (or registers a new one), plays, and leaves. A scheduler keeps the number
of active sessions tracking the current phase of a YAML scenario, which is how
spikes, growth curves and diurnal cycles are expressed.

```bash
# local server on :3000, VictoriaMetrics scraping host.docker.internal:9100
BOT_SCENARIO_FILE=bot/scenarios/spike.yaml go run ./bot
curl -s localhost:9100/metrics | grep '^bot_'
```

Bundled scenarios in `bot/scenarios/`:

| Scenario | Shape |
|---|---|
| `steady.yaml` | 100 sessions forever, 5% new users |
| `spike.yaml` | 50 → 500 in 30s, 2m peak, crash to 50 with session cancellation, recovery; ends after ~9m |
| `growth.yaml` | 0 → 600 over ~30m, new-user ratio falling from 100% to 5%, then holds |
| `diurnal.yaml` | sine wave 20 ↔ 300 over a 24h period, loops |

Scenario numbers are per bot instance; on Kubernetes the fleet size is the
Deployment's replica count. Manifests, environment variables and metric
cheat-sheet: `bot/deploy/README.md`.
