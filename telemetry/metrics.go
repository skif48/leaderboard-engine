// Package telemetry is the single home for the engine's Prometheus metrics.
// Every metric is engine_-prefixed, histograms are in seconds, counters end in
// _total and every label value is bounded. Other packages call the helpers
// here instead of building metric names by hand.
//
// Metrics are exported by servers/fiber.go via metrics.WritePrometheus.
package telemetry

import (
	"fmt"
	"time"

	vm "github.com/VictoriaMetrics/metrics"
)

// --- HTTP -------------------------------------------------------------------

// ObserveHTTP records one served request. route must be the registered route
// pattern (never the raw path) so cardinality stays bounded.
func ObserveHTTP(route, method string, status int, start time.Time) {
	vm.GetOrCreateCounter(fmt.Sprintf(`engine_http_requests_total{route=%q,method=%q,status="%d"}`, route, method, status)).Inc()
	vm.GetOrCreateHistogram(fmt.Sprintf(`engine_http_request_duration_seconds{route=%q,method=%q}`, route, method)).UpdateDuration(start)
}

// --- Kafka consumer ---------------------------------------------------------

var (
	consumerProcessed      = vm.NewCounter(`engine_kafka_consumer_messages_total{result="processed"}`)
	consumerHandlerError   = vm.NewCounter(`engine_kafka_consumer_messages_total{result="handler_error"}`)
	consumerUnmarshalError = vm.NewCounter(`engine_kafka_consumer_messages_total{result="unmarshal_error"}`)
	consumerMessageAge     = vm.NewHistogram(`engine_kafka_consumer_message_age_seconds`)
	consumerQueueWait      = vm.NewHistogram(`engine_kafka_consumer_queue_wait_seconds`)
	consumerHandleDuration = vm.NewHistogram(`engine_kafka_consumer_handle_duration_seconds`)
)

// ObserveMessageAge records how old a message was when the consumer read it
// (broker timestamp to now). This is consumer lag expressed in time.
func ObserveMessageAge(produced time.Time) {
	if !produced.IsZero() {
		consumerMessageAge.UpdateDuration(produced)
	}
}

// ObserveQueueWait records the time a message sat in a worker channel.
func ObserveQueueWait(enqueued time.Time) { consumerQueueWait.UpdateDuration(enqueued) }

// ObserveHandle records the outcome and duration of one worker handling a message.
func ObserveHandle(start time.Time, err error) {
	consumerHandleDuration.UpdateDuration(start)
	if err != nil {
		consumerHandlerError.Inc()
		return
	}
	consumerProcessed.Inc()
}

// CountUnmarshalError counts a message the consumer could not decode.
func CountUnmarshalError() { consumerUnmarshalError.Inc() }

// RegisterWorkerQueue exposes a worker channel's depth and capacity as
// callback gauges. Call once per worker at startup.
func RegisterWorkerQueue[T any](worker int, ch chan T) {
	vm.NewGauge(fmt.Sprintf(`engine_kafka_consumer_queue_depth{worker="%d"}`, worker), func() float64 { return float64(len(ch)) })
	vm.NewGauge(fmt.Sprintf(`engine_kafka_consumer_queue_capacity{worker="%d"}`, worker), func() float64 { return float64(cap(ch)) })
}

// --- Domain -----------------------------------------------------------------

var (
	levelUpsApplied = vm.NewCounter(`engine_level_ups_total{result="applied"}`)
	levelUpsIgnored = vm.NewCounter(`engine_level_ups_total{result="ignored"}`)
)

// CountGameAction counts one handled action by type. action values come from
// game_config.json, so the label set is bounded.
func CountGameAction(action string) {
	vm.GetOrCreateCounter(fmt.Sprintf(`engine_game_actions_total{action=%q}`, action)).Inc()
}

// CountLevelUp records a level-up attempt; applied is false when the
// conditional write lost a race.
func CountLevelUp(applied bool) {
	if applied {
		levelUpsApplied.Inc()
	} else {
		levelUpsIgnored.Inc()
	}
}
