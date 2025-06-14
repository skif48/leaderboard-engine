package services

import (
	"context"
	"encoding/json"
	"github.com/segmentio/kafka-go"
	"github.com/skif48/leaderboard-engine/entities"
)

type GameActionsService struct {
	kw *kafka.Writer
}

func NewGameActionsService() *GameActionsService {
	kw := &kafka.Writer{
		Addr:                   kafka.TCP("localhost:9092"),
		Topic:                  "game-actions",
		Balancer:               &kafka.Murmur2Balancer{Consistent: true},
		AllowAutoTopicCreation: true,
	}

	return &GameActionsService{kw: kw}
}

func (gas *GameActionsService) Action(action *entities.GameAction) error {
	bytes, err := json.Marshal(action)
	if err != nil {
		return err
	}
	return gas.kw.WriteMessages(context.Background(), kafka.Message{
		Key:   []byte(action.UserId),
		Value: bytes,
	})
}
