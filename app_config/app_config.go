package app_config

import (
	"context"
	"github.com/sethvargo/go-envconfig"
	"log/slog"
	"os"
)

type AppConfig struct {
	FiberPort int `env:"FIBER_PORT, default=3000"`

	LogLevel string `env:"LOG_LEVEL, default=info"`

	KafkaBrokers                             []string `env:"KAFKA_BROKERS, default=localhost:9092"`
	KafkaConsumerGroupId                     string   `env:"KAFKA_CONSUMER_GROUP_ID, default=consumer-group-id"`
	KafkaTopic                               string   `env:"KAFKA_TOPIC, default=game-actions"`
	KafkaLeaderboardTopicConsumerConcurrency int      `env:"KAFKA_LEADERBOARD_TOPIC_CONSUMER_CONCURRENCY, default=100"`

	ScyllaUrl string `env:"SCYLLA_URL, default=127.0.0.1:9042"`

	RedisUrl string `env:"REDIS_URL, default=127.0.0.1:6379"`

	MaxLeaderboards int `env:"MAX_LEADERBOARDS, default=5"`
}

func NewAppConfig() *AppConfig {
	ac := &AppConfig{}
	if err := envconfig.Process(context.Background(), ac); err != nil {
		slog.With("err", err).Error(
			"Failed to load environment variables",
		)
		os.Exit(1)
	}
	return ac
}
