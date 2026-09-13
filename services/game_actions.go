package services

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/segmentio/kafka-go"
	"github.com/skif48/leaderboard-engine/app_config"
	"github.com/skif48/leaderboard-engine/entities"
	"github.com/skif48/leaderboard-engine/game_config"
	"github.com/skif48/leaderboard-engine/graceful_shutdown"
	"github.com/skif48/leaderboard-engine/repositories"
	"github.com/skif48/leaderboard-engine/telemetry"
	"log/slog"
)

type GameActionsService struct {
	kw  *kafka.Writer
	lr  repositories.LeaderboardRepo
	upr repositories.UserProfileRepository
	uxr repositories.UserXpRepository
	gc  *game_config.GameConfig
}

func NewGameActionsService(ac *app_config.AppConfig, gc *game_config.GameConfig, lr repositories.LeaderboardRepo, upr repositories.UserProfileRepository, uxr repositories.UserXpRepository) *GameActionsService {
	kw := &kafka.Writer{
		Addr:                   kafka.TCP(ac.KafkaBrokers...),
		Topic:                  "game-actions",
		Balancer:               &kafka.Murmur2Balancer{Consistent: true},
		AllowAutoTopicCreation: true,
		BatchTimeout:           ac.KafkaProducerBatchTimeout,
	}
	statsCtx, stopStats := context.WithCancel(context.Background())
	telemetry.PollKafkaWriter(statsCtx, kw)
	graceful_shutdown.AddOutputShutdownFunc(stopStats)

	return &GameActionsService{
		kw:  kw,
		lr:  lr,
		upr: upr,
		uxr: uxr,
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
	telemetry.CountGameAction(action.Action)
	userProfile, err := gas.upr.GetUserProfile(action.UserId)
	if err != nil {
		return err
	}
	_, err = gas.lr.UpdateScore(userProfile.Leaderboard, action.UserId, score)
	if err != nil {
		return err
	}
	newXp, err := gas.uxr.IncrementXp(action.UserId, score)
	if err != nil {
		return err
	}

	newLevel := 0

	for i, threshold := range gas.gc.XpToLevelThresholds {
		if newXp >= threshold && userProfile.Level <= i {
			newLevel = i + 1
		}
	}

	if newLevel > userProfile.Level {
		updated, err := gas.upr.UpdateLevel(action.UserId, userProfile.Level, newLevel)
		if err != nil {
			return err
		}
		telemetry.CountLevelUp(updated)
		if !updated {
			slog.With("userId", action.UserId).Warn("User level update was ignored, race condition")
		}
	}

	return err
}
