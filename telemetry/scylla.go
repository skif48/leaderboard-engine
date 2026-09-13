package telemetry

import (
	"context"
	"errors"
	"fmt"
	"strings"

	vm "github.com/VictoriaMetrics/metrics"
	"github.com/gocql/gocql"
)

type scyllaOpKey struct{}

// WithScyllaOp tags a query context with a short operation name that becomes
// the op label on Scylla metrics. Repositories set it per method.
func WithScyllaOp(ctx context.Context, op string) context.Context {
	return context.WithValue(ctx, scyllaOpKey{}, op)
}

// ScyllaObserver implements gocql.QueryObserver and gocql.ConnectObserver.
// Set both on the production ClusterConfig.
type ScyllaObserver struct{}

func (ScyllaObserver) ObserveQuery(ctx context.Context, q gocql.ObservedQuery) {
	op := scyllaOp(ctx, q.Statement)
	vm.GetOrCreateHistogram(fmt.Sprintf(`engine_scylla_query_duration_seconds{op=%q}`, op)).Update(q.End.Sub(q.Start).Seconds())
	vm.GetOrCreateCounter(fmt.Sprintf(`engine_scylla_queries_total{op=%q,result=%q}`, op, scyllaResult(q.Err))).Inc()
	if q.Attempt > 0 {
		vm.GetOrCreateCounter(fmt.Sprintf(`engine_scylla_query_attempts_total{op=%q}`, op)).Inc()
	}
}

func (ScyllaObserver) ObserveConnect(c gocql.ObservedConnect) {
	result := "ok"
	if c.Err != nil {
		result = "error"
	}
	vm.GetOrCreateCounter(fmt.Sprintf(`engine_scylla_connects_total{result=%q}`, result)).Inc()
}

func scyllaOp(ctx context.Context, statement string) string {
	if op, ok := ctx.Value(scyllaOpKey{}).(string); ok && op != "" {
		return op
	}
	// Fallback: first CQL keyword, e.g. "select". Bounded by CQL grammar.
	f := strings.Fields(statement)
	if len(f) == 0 {
		return "unknown"
	}
	return strings.ToLower(f[0])
}

func scyllaResult(err error) string {
	switch {
	case err == nil:
		return "ok"
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, gocql.ErrTimeoutNoResponse):
		return "timeout"
	}
	var unavailable *gocql.RequestErrUnavailable
	var readTimeout *gocql.RequestErrReadTimeout
	var writeTimeout *gocql.RequestErrWriteTimeout
	switch {
	case errors.As(err, &unavailable):
		return "unavailable"
	case errors.As(err, &readTimeout), errors.As(err, &writeTimeout):
		return "timeout"
	}
	return "error"
}
