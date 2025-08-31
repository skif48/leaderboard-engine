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

type KafkaConsumer struct {
	r   *kafka.Reader
	gas *services.GameActionsService
}

func RunKafkaConsumer(ac *app_config.AppConfig, gas *services.GameActionsService) {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers: ac.KafkaBrokers,
		GroupID: ac.KafkaConsumerGroupId,
		Topic:   ac.KafkaTopic,
	})

	kc := &KafkaConsumer{
		r:   r,
		gas: gas,
	}

	go kc.listen()
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
		}
		err = kc.gas.HandleAction(gameAction)
		if err != nil {
			slog.Error(err.Error())
			continue
		}
		if err := kc.r.CommitMessages(context.Background(), m); err != nil {
			slog.Error(err.Error())
		}
	}
}
