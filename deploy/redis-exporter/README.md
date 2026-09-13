# Redis metrics on EKS

[redis_exporter](https://github.com/oliver006/redis_exporter) polls the
ElastiCache cluster with `INFO` and `CLUSTER INFO` and exposes the result on
`:9121`. Same image as the `redis-exporter` service in `docker-compose.yml`,
so local dashboards work unchanged.

Locally the exporter runs in multi-target mode and VictoriaMetrics scrapes each
of the six cluster nodes through `/scrape?target=`. On EKS it points at the
ElastiCache configuration endpoint with `REDIS_EXPORTER_IS_CLUSTER=true`, which
reports the node it lands on plus cluster-wide state. For per-shard detail
switch to multi-target with the individual node endpoints from
`aws elasticache describe-replication-groups`.

## Deploy

```bash
ENDPOINT=$(terraform -chdir=terraform output -raw elasticache_endpoint)
sed "s|REPLACE_WITH_ELASTICACHE_ENDPOINT|$ENDPOINT|" deploy/redis-exporter/deployment.yaml | kubectl apply -f -
kubectl apply -f deploy/redis-exporter/service.yaml
kubectl apply -f deploy/redis-exporter/vmscrape.yaml   # only with the VictoriaMetrics operator
```

Check:

```bash
kubectl port-forward svc/redis-exporter 9121 &
curl -s localhost:9121/metrics | grep -E '^redis_(up|cluster_enabled|commands_processed_total)'
```

No Terraform change is needed: the ElastiCache security group already allows
6379 from the EKS node group, and transit encryption is off (`terraform/elasticache.tf`).

## Metrics

| Metric | Meaning |
|---|---|
| `redis_up` | Exporter could reach the node |
| `redis_commands_processed_total` | Ops counter; `rate()` for ops/s |
| `redis_commands_total{cmd}` | Per-command counter |
| `redis_commands_duration_seconds_total{cmd}` | Per-command time; divide rate by rate of `redis_commands_total` for avg latency |
| `redis_connected_clients` | Client connections |
| `redis_memory_used_bytes` / `redis_memory_max_bytes` | Memory |
| `redis_cluster_state` / `redis_cluster_slots_ok` | Cluster health |
| `redis_db_keys{db}` | Key count |

Useful queries:

```
sum(rate(redis_commands_processed_total[1m]))                                     # cluster ops/s
sum by (cmd) (rate(redis_commands_duration_seconds_total[1m]))
  / sum by (cmd) (rate(redis_commands_total[1m]))                                  # avg latency per command
```
