package telemetry

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	vm "github.com/VictoriaMetrics/metrics"
	"github.com/redis/rueidis"
	"github.com/redis/rueidis/rueidishook"
)

var redisMultiBatchSize = vm.NewHistogram(`engine_redis_multi_batch_size`)

// RedisHook records per-command latency and outcomes. Do records one
// observation per command; DoMulti records one duration for the whole call
// (cmd="domulti") plus a count per inner command. Only Do and DoMulti are
// instrumented; the engine does not use caching, streaming or pub/sub, so the
// remaining Hook methods delegate untouched.
type RedisHook struct{}

var _ rueidishook.Hook = RedisHook{}

// WrapRedis returns client with RedisHook attached. Per-node clients from
// Nodes() are wrapped as well.
func WrapRedis(client rueidis.Client) rueidis.Client {
	return rueidishook.WithHook(client, RedisHook{})
}

func (RedisHook) Do(client rueidis.Client, ctx context.Context, cmd rueidis.Completed) rueidis.RedisResult {
	// Read the name before delegating: Completed is recycled after Do.
	name := commandName(cmd.Commands())
	start := time.Now()
	resp := client.Do(ctx, cmd)
	observeRedis(name, start, resp.Error())
	return resp
}

func (RedisHook) DoMulti(client rueidis.Client, ctx context.Context, multi ...rueidis.Completed) []rueidis.RedisResult {
	names := make([]string, len(multi))
	for i, c := range multi {
		names[i] = commandName(c.Commands())
	}
	start := time.Now()
	resps := client.DoMulti(ctx, multi...)
	redisMultiBatchSize.Update(float64(len(multi)))
	var firstErr error
	for i, r := range resps {
		err := r.Error()
		if err != nil && firstErr == nil {
			firstErr = err
		}
		if i < len(names) {
			vm.GetOrCreateCounter(fmt.Sprintf(`engine_redis_commands_total{cmd=%q,result=%q}`, names[i], redisResult(err))).Inc()
		}
	}
	// "domulti" is the whole pipelined call; Redis's own MULTI command inside
	// a transaction is counted separately under cmd="multi".
	observeRedis("domulti", start, firstErr)
	return resps
}

func (RedisHook) DoCache(client rueidis.Client, ctx context.Context, cmd rueidis.Cacheable, ttl time.Duration) rueidis.RedisResult {
	return client.DoCache(ctx, cmd, ttl)
}

func (RedisHook) DoMultiCache(client rueidis.Client, ctx context.Context, multi ...rueidis.CacheableTTL) []rueidis.RedisResult {
	return client.DoMultiCache(ctx, multi...)
}

func (RedisHook) Receive(client rueidis.Client, ctx context.Context, subscribe rueidis.Completed, fn func(msg rueidis.PubSubMessage)) error {
	return client.Receive(ctx, subscribe, fn)
}

func (RedisHook) DoStream(client rueidis.Client, ctx context.Context, cmd rueidis.Completed) rueidis.RedisResultStream {
	return client.DoStream(ctx, cmd)
}

func (RedisHook) DoMultiStream(client rueidis.Client, ctx context.Context, multi ...rueidis.Completed) rueidis.MultiRedisResultStream {
	return client.DoMultiStream(ctx, multi...)
}

func observeRedis(cmd string, start time.Time, err error) {
	vm.GetOrCreateHistogram(fmt.Sprintf(`engine_redis_command_duration_seconds{cmd=%q}`, cmd)).UpdateDuration(start)
	vm.GetOrCreateCounter(fmt.Sprintf(`engine_redis_commands_total{cmd=%q,result=%q}`, cmd, redisResult(err))).Inc()
}

func commandName(parts []string) string {
	if len(parts) == 0 {
		return "unknown"
	}
	return strings.ToLower(parts[0])
}

func redisResult(err error) string {
	switch {
	case err == nil:
		return "ok"
	case rueidis.IsRedisNil(err):
		return "nil"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	}
	if rerr, ok := rueidis.IsRedisErr(err); ok {
		if _, moved := rerr.IsMoved(); moved {
			return "moved"
		}
		if _, ask := rerr.IsAsk(); ask {
			return "ask"
		}
		if strings.HasPrefix(rerr.Error(), "READONLY") {
			return "readonly"
		}
	}
	return "error"
}
