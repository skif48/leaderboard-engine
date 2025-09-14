package middleware

import (
	"fmt"
	"github.com/VictoriaMetrics/metrics"
	"github.com/gofiber/fiber/v3"
	"time"
)

func MetricsMiddleware() fiber.Handler {
	return func(ctx fiber.Ctx) error {
		start := time.Now()
		defer func() {
			metrics.GetOrCreateCounter(fmt.Sprintf(`http_requests_total{path=%q, method=%q, status="%d"}`, ctx.Path(), ctx.Method(), ctx.Response().StatusCode())).Inc()
			metrics.GetOrCreateHistogram(fmt.Sprintf(`http_requests_latency{path=%q, method=%q, status="%d"}`, ctx.Path(), ctx.Method(), ctx.Response().StatusCode())).UpdateDuration(start)
		}()
		return ctx.Next()
	}
}
