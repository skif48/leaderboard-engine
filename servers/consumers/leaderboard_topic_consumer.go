package consumers

import (
	"github.com/skif48/leaderboard-engine/app_config"
	"github.com/skif48/leaderboard-engine/entities"
	"github.com/skif48/leaderboard-engine/services"
	"log/slog"
)

type LeaderboardTopicConsumer struct {
	ch []chan *entities.GameAction

	gas *services.GameActionsService
}

func NewLeaderboardTopicConsumer(ac *app_config.AppConfig, gas *services.GameActionsService) *LeaderboardTopicConsumer {
	concurrency := ac.KafkaLeaderboardTopicConsumerConcurrency
	l := &LeaderboardTopicConsumer{
		ch: make([]chan *entities.GameAction, concurrency),

		gas: gas,
	}
}

func (l *LeaderboardTopicConsumer) start() {
	for i := 0; i < len(l.ch); i++ {
		go func(i int) {
			for g := range l.ch[i] {
				if err := l.gas.HandleAction(g); err != nil {
					slog.With(err).Error("Failed to handle action")
					continue
				}
			}
		}(i)
	}
}
