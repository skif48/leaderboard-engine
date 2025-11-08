package inits

import (
	"github.com/redis/rueidis"
	"github.com/skif48/leaderboard-engine/app_config"
	"github.com/skif48/leaderboard-engine/graceful_shutdown"
)

func NewRedisClient(ac *app_config.AppConfig) rueidis.Client {
	client, err := rueidis.NewClient(rueidis.ClientOption{
		InitAddress: []string{ac.RedisUrl},
		ShuffleInit: true,
	})
	if err != nil {
		panic(err)
	}
	graceful_shutdown.AddOutputShutdownFunc(func() {
		client.Close()
	})
	return client
}
