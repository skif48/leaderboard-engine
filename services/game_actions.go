package services

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/segmentio/kafka-go"
	"github.com/skif48/leaderboard-engine/entities"
	"github.com/skif48/leaderboard-engine/repositories"
)

type GameActionsService struct {
	kw  *kafka.Writer
	lr  repositories.LeaderboardRepo
	upr repositories.UserProfileRepository
	gc  *GameConfig
}

func NewGameActionsService(gc *GameConfig, lr repositories.LeaderboardRepo, upr repositories.UserProfileRepository) *GameActionsService {
	kw := &kafka.Writer{
		Addr:                   kafka.TCP("localhost:9092"),
		Topic:                  "game-actions",
		Balancer:               &kafka.Murmur2Balancer{Consistent: true},
		AllowAutoTopicCreation: true,
	}

	return &GameActionsService{
		kw:  kw,
		lr:  lr,
		upr: upr,
		gc:  gc,
	}
}

func (gas *GameActionsService) ProduceAction(action *entities.GameAction) error {
	bytes, err := json.Marshal(action)
	if err != nil {
		return err
	}
	return gas.kw.WriteMessages(context.Background(), kafka.Message{
		Key:   []byte(action.UserId),
		Value: bytes,
	})
}

func (gas *GameActionsService) HandleAction(action *entities.GameAction) error {
	score, ok := gas.gc.ActionsScoreMap[action.Action]
	if !ok {
		return fmt.Errorf("unknown action: %s", action.Action)
	}
	userProfile, err := gas.upr.GetUserProfile(action.UserId)
	if err != nil {
		return err
	}
	_, err = gas.lr.UpdateScore(userProfile.Leaderboard, action.UserId, score)
	return err
}
