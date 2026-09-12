# Running the bot fleet on EKS

The bot runs as ordinary pods in the same namespace as `leaderboard-engine` and
talks to it over the ClusterIP service. Nothing here needs an ingress, TLS or
credentials.

## Files

| File | Purpose |
|---|---|
| `deployment.yaml` | The fleet. `replicas` is the fleet size. |
| `configmap.yaml` | Generated copy of `bot/scenarios/*.yaml`, mounted at `/scenarios`. |
| `service.yaml` | ClusterIP exposing `/metrics` on 9100 with `prometheus.io/*` annotations. |
| `vmscrape.yaml` | `VMServiceScrape` for the VictoriaMetrics operator, plus a commented `kubernetes_sd` job for plain vmagent. |
| `gen-configmap.sh` | Regenerates `configmap.yaml` after editing a scenario. |

## Fleet semantics

**Scenario numbers are per pod.** A scenario phase with `active_users: 500`
and three replicas means roughly 1500 concurrent sessions in total. Scale
load by changing replicas, and shape it by changing the scenario:

```bash
kubectl scale deployment/leaderboard-bot --replicas=3
```

Each pod gets `BOT_INSTANCE_ID` from its pod name. That value seeds the
random streams and tags every nickname (`SwiftRunner7-a1f3`) so replicas never
collide. Pods are otherwise independent; there is no coordinator.

## Build and push

```bash
docker build -f bot/Dockerfile -t <account>.dkr.ecr.<region>.amazonaws.com/leaderboard-bot:latest .
docker push <account>.dkr.ecr.<region>.amazonaws.com/leaderboard-bot:latest
```

Then set `image:` in `deployment.yaml`.

## Deploy

```bash
sh bot/deploy/gen-configmap.sh                       # after editing a scenario
kubectl apply -f bot/deploy/configmap.yaml -f bot/deploy/service.yaml -f bot/deploy/deployment.yaml
kubectl apply -f bot/deploy/vmscrape.yaml            # only with the VictoriaMetrics operator
```

Change scenario: edit `BOT_SCENARIO_FILE` in the Deployment (or the ConfigMap
content), then `kubectl rollout restart deployment/leaderboard-bot`. ConfigMap
edits are not picked up by a running pod.

## One-shot runs

A Deployment restarts a container that exits, so the manifest sets
`BOT_LOOP_SCENARIO=true`. For a single pass of a finite scenario (e.g. `spike`)
run it as a Job instead:

```yaml
apiVersion: batch/v1
kind: Job
metadata: { name: leaderboard-bot-spike }
spec:
  template:
    spec:
      restartPolicy: Never
      terminationGracePeriodSeconds: 30
      containers:
        - name: bot
          image: REGISTRY/leaderboard-bot:latest
          env:
            - { name: BOT_BASE_URL, value: http://leaderboard-engine:3000 }
            - { name: BOT_SCENARIO_FILE, value: /scenarios/spike.yaml }
            - { name: BOT_LOOP_SCENARIO, value: "false" }
```

The bot exits 0 when the scenario completes.

## Environment variables

| Variable | Default | Meaning |
|---|---|---|
| `BOT_BASE_URL` | `http://localhost:3000` | Server endpoint |
| `BOT_SCENARIO_FILE` | `/scenarios/steady.yaml` | Scenario to run |
| `BOT_INSTANCE_ID` | hostname | Replica identity (nickname tag, seed) |
| `BOT_MAX_RPS` | `0` | Global request-rate cap for this pod, 0 = off |
| `BOT_MAX_CONNS` | `200` | In-flight HTTP request cap (transport `MaxConnsPerHost`) |
| `BOT_MAX_USERS` | `0` | Cap on users this pod registers, 0 = unlimited |
| `BOT_MAX_SPAWN_PER_SEC` | `100` | Session starts per second, smooths sign-up bursts |
| `BOT_SIGNUP_CONCURRENCY` | `16` | Concurrent sign-up requests |
| `BOT_LOOP_SCENARIO` | `false` | Repeat the scenario when it ends |
| `BOT_SEED` | `0` | Fixed seed for reproducible runs, 0 = time-based |
| `BOT_SHUTDOWN_TIMEOUT` | `10s` | Drain budget after SIGTERM, keep below `terminationGracePeriodSeconds` |
| `BOT_SUMMARY_INTERVAL` | `10s` | Periodic summary log line, 0 = off |
| `BOT_METRICS_PORT` | `9100` | `/metrics` and `/health` |
| `LOG_LEVEL` | `info` | `debug` logs every action; keep `info` under load |

## Reading the metrics

- `bot_sessions_active` vs `bot_target_active_sessions`: the scheduler is keeping up when they track each other.
- `rate(bot_actions_sent_total[1m])` on the bot vs `rate(http_requests_total{path="/api/v1/users/actions"}[1m])` on the server should match.
- `bot_http_request_duration_seconds` (bot side) vs `http_requests_latency` (server side): a growing gap means requests are queueing in the bot's transport. Raise `BOT_MAX_CONNS` or add replicas.
- `bot_http_retries_total{reason="not_found"}` is expected to be small but non-zero while new users are being created: the server reads profiles at consistency ONE right after a QUORUM write.
- `bot_sessions_ended_total{reason="user_gone"}` climbing means the server forgot users (for example after a purge); the pool re-registers automatically.
