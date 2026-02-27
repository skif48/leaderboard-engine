package servers

import (
	"context"
	"encoding/json"
	"github.com/VictoriaMetrics/metrics"
	"github.com/segmentio/kafka-go"
	"github.com/skif48/leaderboard-engine/app_config"
	"github.com/skif48/leaderboard-engine/entities"
	"github.com/skif48/leaderboard-engine/graceful_shutdown"
	"github.com/skif48/leaderboard-engine/services"
	"log/slog"
	"sync"
	"time"
)

type chMsg struct {
	ga *entities.GameAction
}

type KafkaConsumer struct {
	r         *kafka.Reader
	ch        []chan *chMsg
	workersWg *sync.WaitGroup

	gas *services.GameActionsService
}

func RunKafkaConsumer(ac *app_config.AppConfig, gas *services.GameActionsService) {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        ac.KafkaBrokers,
		GroupID:        ac.KafkaConsumerGroupId,
		Topic:          ac.KafkaTopic,
		MinBytes:       ac.KafkaLeaderboardTopicConsumerMinBytes,
		MaxBytes:       ac.KafkaLeaderboardTopicConsumerMaxBytes,
		MaxWait:        ac.KafkaLeaderboardTopicConsumerMaxWait,
		CommitInterval: 1 * time.Second,
	})

	kc := &KafkaConsumer{
		r:         r,
		ch:        make([]chan *chMsg, ac.KafkaLeaderboardTopicConsumerConcurrency),
		gas:       gas,
		workersWg: &sync.WaitGroup{},
	}

	for i := 0; i < len(kc.ch); i++ {
		kc.ch[i] = make(chan *chMsg, ac.KafkaLeaderboardTopicConsumerBufferSize)
	}

	kc.runWorkers()
	ctx, cancel := context.WithCancel(context.Background())
	graceful_shutdown.AddInputShutdownFunc(func() {
		slog.Info("Kafka consumer stopping")
		cancel()
		if err := r.Close(); err != nil {
			slog.With("error", err).Error("Failed to close kafka reader")
		}
		slog.Info("Kafka reader stopped")
		for i := 0; i < len(kc.ch); i++ {
			close(kc.ch[i])
		}
		slog.Info("Kafka consumer channels closed")
		kc.workersWg.Wait()
		slog.Info("Kafka consumer workers stopped")
		slog.Info("Kafka consumer stopped")
	})
	go kc.listen(ctx)
}

func (kc *KafkaConsumer) runWorkers() {
	for i := 0; i < len(kc.ch); i++ {
		kc.workersWg.Add(1)
		go func(i int) {
			defer kc.workersWg.Done()
			ch := kc.ch[i]
			for m := range ch {
				start := time.Now()
				if err := kc.gas.HandleAction(m.ga); err != nil {
					slog.With("error", err).Error("Failed to handle action")
					continue
				}
				metrics.GetOrCreateCounter(`kafka_processed_messages{topic="leaderboard"}`).Inc()
				metrics.GetOrCreateHistogram(`kafka_processing_time_milliseconds{topic="leaderboard"}`).Update(float64(time.Since(start).Milliseconds()))
			}
		}(i)
	}
}

func (kc *KafkaConsumer) listen(ctx context.Context) {
	for {
		m, err := kc.r.ReadMessage(ctx)
		if err != nil {
			if err.Error() != "context canceled" {
				slog.With("error", err).Error("error while fetching messages from kafka reader")
			}
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
		}
	}
}
