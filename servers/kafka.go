package servers

import (
	"context"
	"github.com/segmentio/kafka-go"
	"log/slog"
)

type KafkaConsumer struct {
	r *kafka.Reader
}

func RunKafkaConsumer() {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{"localhost:9092"},
		GroupID: "consumer-group-id",
		Topic:   "game-actions",
	})

	kc := &KafkaConsumer{r: r}

	go kc.listen()
}

func (kc *KafkaConsumer) listen() {
	for {
		m, err := kc.r.ReadMessage(context.Background())
		if err != nil {
			slog.Error(err.Error())
			break
		}
		slog.Info(string(m.Value))
	}
}
