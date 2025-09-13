package servers

import (
	"context"
	"encoding/json"
	"github.com/segmentio/kafka-go"
	"github.com/skif48/leaderboard-engine/app_config"
	"github.com/skif48/leaderboard-engine/entities"
	"github.com/skif48/leaderboard-engine/services"
	"log/slog"
)

type chMsg struct {
	ga *entities.GameAction
	km kafka.Message
}

type KafkaConsumer struct {
	r  *kafka.Reader
	ch []chan *chMsg

	gas *services.GameActionsService
}

func RunKafkaConsumer(ac *app_config.AppConfig, gas *services.GameActionsService) {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  ac.KafkaBrokers,
		GroupID:  ac.KafkaConsumerGroupId,
		Topic:    ac.KafkaTopic,
		MinBytes: ac.KafkaLeaderboardTopicConsumerMinBytes,
		MaxBytes: ac.KafkaLeaderboardTopicConsumerMaxBytes,
		MaxWait:  ac.KafkaLeaderboardTopicConsumerMaxWait,
	})

	kc := &KafkaConsumer{
		r:   r,
		ch:  make([]chan *chMsg, ac.KafkaLeaderboardTopicConsumerConcurrency),
		gas: gas,
	}

	for i := 0; i < len(kc.ch); i++ {
		kc.ch[i] = make(chan *chMsg)
	}

	kc.runWorkers()
	go kc.listen()
}

func (kc *KafkaConsumer) runWorkers() {
	for i := 0; i < len(kc.ch); i++ {
		go func(i int) {
			ch := kc.ch[i]
			for m := range ch {
				if err := kc.gas.HandleAction(m.ga); err != nil {
					slog.With(err).Error("Failed to handle action")
					continue
				}
				if err := kc.r.CommitMessages(context.Background(), m.km); err != nil {
					slog.With(err).Error("Failed to commit message")
					continue
				}
			}
		}(i)
	}
}

func (kc *KafkaConsumer) listen() {
	for {
		m, err := kc.r.FetchMessage(context.Background())
		if err != nil {
			slog.Error(err.Error())
			break
		}
		gameAction := &entities.GameAction{}
		if err := json.Unmarshal(m.Value, gameAction); err != nil {
			slog.Error(err.Error())
			continue
		}
		leaderboardId := gameAction.LeaderboardId
		channelId := leaderboardId % len(kc.ch)
		kc.ch[channelId] <- &chMsg{
			ga: gameAction,
			km: m,
		}
	}
}
