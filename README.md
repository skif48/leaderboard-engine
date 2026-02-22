# leaderboard-engine

## Performance Observations

| Unique Users | Actions/sec | Kafka Avg Latency (ms) | Consumer Concurrency | Scylla Pool Size | Workers Utilization |
|--------------|-------------|------------------------|----------------------|------------------|---------------------|
| 100          | 400         | 3.7                    | 5                    | 2                | ~30%                |
| 500          | 2,000       | 3.7                    | 5                    | 2                | ~82%                |
| 500          | 2,000       | 4.84                   | 10                   | 2                | ~68%                |