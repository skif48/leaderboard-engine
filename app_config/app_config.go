package app_config

import (
	"context"
	"github.com/sethvargo/go-envconfig"
	"log/slog"
	"os"
	"time"
)

type AppConfig struct {
	FiberPort int `env:"FIBER_PORT, default=3000"`

	LogLevel string `env:"LOG_LEVEL, default=info"`

	KafkaBrokers                             []string      `env:"KAFKA_BROKERS, default=localhost:9092"`
	KafkaConsumerGroupId                     string        `env:"KAFKA_CONSUMER_GROUP_ID, default=consumer-group-id"`
	KafkaTopic                               string        `env:"KAFKA_TOPIC, default=game-actions"`
	KafkaLeaderboardTopicConsumerConcurrency int           `env:"KAFKA_LEADERBOARD_TOPIC_CONSUMER_CONCURRENCY, default=5"`
	KafkaLeaderboardTopicConsumerBufferSize  int           `env:"KAFKA_LEADERBOARD_TOPIC_CONSUMER_BUFFER_SIZE, default=1000"`
	KafkaLeaderboardTopicConsumerMinBytes    int           `env:"KAFKA_LEADERBOARD_TOPIC_CONSUMER_MIN_BYTES, default=1024"`
	KafkaLeaderboardTopicConsumerMaxBytes    int           `env:"KAFKA_LEADERBOARD_TOPIC_CONSUMER_MAX_BYTES, default=10485760"`
	KafkaLeaderboardTopicConsumerMaxWait     time.Duration `env:"KAFKA_LEADERBOARD_TOPIC_CONSUMER_MAX_WAIT, default=100ms"`

	ScyllaUrl      string `env:"SCYLLA_URL, default=127.0.0.1:9042"`
	ScyllaNumConns int    `env:"SCYLLA_NUM_CONNS, default=10"`

	RedisUrl string `env:"REDIS_URL, default=127.0.0.1:6379"`
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
