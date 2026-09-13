# Kafka consumer lag on EKS

[kafka_exporter](https://github.com/danielqsj/kafka_exporter) polls the MSK
cluster for consumer-group offsets and topic high-water marks and exposes them
on `:9308`. It is the same image and flags as the `kafka-exporter` service in
`docker-compose.yml`, so dashboards built locally work unchanged.

## Deploy

```bash
BROKERS=$(terraform -chdir=terraform output -raw msk_bootstrap_brokers)
sed "s|REPLACE_WITH_MSK_BOOTSTRAP_BROKERS|$BROKERS|" deploy/kafka-exporter/deployment.yaml | kubectl apply -f -
kubectl apply -f deploy/kafka-exporter/service.yaml
kubectl apply -f deploy/kafka-exporter/vmscrape.yaml   # only with the VictoriaMetrics operator
```

Check:

```bash
kubectl port-forward svc/kafka-exporter 9308 &
curl -s localhost:9308/metrics | grep kafka_consumergroup_lag
```

No Terraform change is needed: the MSK security group already allows 9092 from
the EKS node group, and the cluster is unauthenticated (`terraform/msk.tf`).

## Metrics

| Metric | Labels | Meaning |
|---|---|---|
| `kafka_consumergroup_lag` | consumergroup, topic, partition | Messages behind the high-water mark |
| `kafka_consumergroup_lag_sum` | consumergroup, topic | Lag summed over partitions |
| `kafka_consumergroup_current_offset` | consumergroup, topic, partition | Committed offset |
| `kafka_consumergroup_members` | consumergroup | Members in the group |
| `kafka_topic_partition_current_offset` | topic, partition | High-water mark |

MSK auto-creates `game-actions` with 6 partitions (`num.partitions` in
`terraform/msk.tf`), local Kafka with 1. Use `kafka_consumergroup_lag_sum` for
totals; never assume the partition count.

Useful queries:

```
sum by (consumergroup, topic) (kafka_consumergroup_lag_sum)
sum(rate(kafka_topic_partition_current_offset{topic="game-actions"}[1m]))   # produce rate
sum(rate(kafka_consumergroup_current_offset{topic="game-actions"}[1m]))     # consume rate
```

## MSK IAM auth (later)

If `client_authentication` in `terraform/msk.tf` is switched to `sasl { iam = true }`:

1. Use the IAM bootstrap string (port 9098) and add `--tls.enabled`,
   `--sasl.enabled`, `--sasl.mechanism=awsiam`, `--sasl.aws-region=<region>`.
2. Give the pod an IRSA role allowing `kafka-cluster:Connect`,
   `kafka-cluster:DescribeCluster`, `kafka-cluster:DescribeTopic`,
   `kafka-cluster:DescribeGroup`, `kafka-cluster:ReadData` on the cluster.
