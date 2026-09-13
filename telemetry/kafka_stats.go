package telemetry

import (
	"context"
	"time"

	vm "github.com/VictoriaMetrics/metrics"
	"github.com/segmentio/kafka-go"
)

// kafka-go's Reader.Stats() and Writer.Stats() reset their counters and
// summaries on every call, so exactly one goroutine may poll each. The pollers
// below add the returned deltas to monotonic counters and publish the
// per-interval averages as gauges. Avg/Min/Max are not fed into histograms:
// they are already aggregates.

const statsInterval = 10 * time.Second

var (
	readerFetches    = vm.NewCounter(`engine_kafka_reader_fetches_total`)
	readerMessages   = vm.NewCounter(`engine_kafka_reader_messages_total`)
	readerBytes      = vm.NewCounter(`engine_kafka_reader_bytes_total`)
	readerErrors     = vm.NewCounter(`engine_kafka_reader_errors_total`)
	readerRebalances = vm.NewCounter(`engine_kafka_reader_rebalances_total`)
	readerTimeouts   = vm.NewCounter(`engine_kafka_reader_timeouts_total`)
	readerQueueLen   = vm.NewGauge(`engine_kafka_reader_queue_length`, nil)
	readerQueueCap   = vm.NewGauge(`engine_kafka_reader_queue_capacity`, nil)
	readerWaitAvg    = vm.NewGauge(`engine_kafka_reader_wait_seconds_avg`, nil)
	readerFetchSize  = vm.NewGauge(`engine_kafka_reader_fetch_size_avg`, nil)

	writerWrites       = vm.NewCounter(`engine_kafka_writer_writes_total`)
	writerMessages     = vm.NewCounter(`engine_kafka_writer_messages_total`)
	writerBytes        = vm.NewCounter(`engine_kafka_writer_bytes_total`)
	writerErrors       = vm.NewCounter(`engine_kafka_writer_errors_total`)
	writerRetries      = vm.NewCounter(`engine_kafka_writer_retries_total`)
	writerBatchSizeAvg = vm.NewGauge(`engine_kafka_writer_batch_size_avg`, nil)
	writerBatchSizeMax = vm.NewGauge(`engine_kafka_writer_batch_size_max`, nil)
	writerBatchWait    = vm.NewGauge(`engine_kafka_writer_batch_wait_seconds_avg`, nil)
	writerWriteTime    = vm.NewGauge(`engine_kafka_writer_write_seconds_avg`, nil)
	writerWaitTime     = vm.NewGauge(`engine_kafka_writer_wait_seconds_avg`, nil)
)

// PollKafkaReader publishes Reader stats until ctx is cancelled.
func PollKafkaReader(ctx context.Context, r *kafka.Reader) {
	go poll(ctx, func() {
		s := r.Stats()
		readerFetches.Add(int(s.Fetches))
		readerMessages.Add(int(s.Messages))
		readerBytes.Add(int(s.Bytes))
		readerErrors.Add(int(s.Errors))
		readerRebalances.Add(int(s.Rebalances))
		readerTimeouts.Add(int(s.Timeouts))
		readerQueueLen.Set(float64(s.QueueLength))
		readerQueueCap.Set(float64(s.QueueCapacity))
		readerWaitAvg.Set(s.WaitTime.Avg.Seconds())
		readerFetchSize.Set(float64(s.FetchSize.Avg))
	})
}

// PollKafkaWriter publishes Writer stats until ctx is cancelled.
func PollKafkaWriter(ctx context.Context, w *kafka.Writer) {
	go poll(ctx, func() {
		s := w.Stats()
		writerWrites.Add(int(s.Writes))
		writerMessages.Add(int(s.Messages))
		writerBytes.Add(int(s.Bytes))
		writerErrors.Add(int(s.Errors))
		writerRetries.Add(int(s.Retries))
		writerBatchSizeAvg.Set(float64(s.BatchSize.Avg))
		writerBatchSizeMax.Set(float64(s.BatchSize.Max))
		writerBatchWait.Set(s.BatchTime.Avg.Seconds())
		writerWriteTime.Set(s.WriteTime.Avg.Seconds())
		writerWaitTime.Set(s.WaitTime.Avg.Seconds())
	})
}

func poll(ctx context.Context, tick func()) {
	t := time.NewTicker(statsInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			tick() // flush the last interval
			return
		case <-t.C:
			tick()
		}
	}
}
