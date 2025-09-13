package inits

import (
	"github.com/redis/rueidis"
	"github.com/skif48/leaderboard-engine/app_config"
)

func NewRedisClient(ac *app_config.AppConfig) rueidis.Client {
	client, err := rueidis.NewClient(rueidis.ClientOption{
		InitAddress: []string{ac.RedisUrl},
		ShuffleInit: true,
	})
	if err != nil {
		panic(err)
	}
	return client
}
