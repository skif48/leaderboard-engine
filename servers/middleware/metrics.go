package middleware

import (
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/skif48/leaderboard-engine/telemetry"
)

const unmatchedRoute = "<unmatched>"

// MetricsMiddleware records request counts and latency per registered route
// pattern. It labels by ctx.Route().Path, never by the raw URL, so a path
// parameter such as a user id cannot create one series per value. Scrapes
// and profiler requests are not counted.
//
// Two Fiber details shape this: the matched route is only known after
// Next() returns, and an error returned from the chain (404, 405, handler
// errors) is turned into a status code by the app's error handler *after*
// this middleware returns, so the status must be derived from the error.
func MetricsMiddleware() fiber.Handler {
	return func(ctx fiber.Ctx) error {
		path := ctx.Path()
		if path == "/metrics" || strings.HasPrefix(path, "/debug/pprof") {
			return ctx.Next()
		}
		start := time.Now()
		err := ctx.Next()
		telemetry.ObserveHTTP(routeLabel(ctx, err), ctx.Method(), statusOf(ctx, err), start)
		return err
	}
}

func routeLabel(ctx fiber.Ctx, err error) string {
	if errors.Is(err, fiber.ErrNotFound) || errors.Is(err, fiber.ErrMethodNotAllowed) {
		return unmatchedRoute
	}
	r := ctx.Route()
	// No handlers: Route() returned its fallback (raw path). Path "/": the last
	// match was a Use middleware, so no real handler ran.
	if r == nil || len(r.Handlers) == 0 || r.Path == "/" {
		return unmatchedRoute
	}
	return r.Path
}

func statusOf(ctx fiber.Ctx, err error) int {
	if err == nil {
		return ctx.Response().StatusCode()
	}
	var fe *fiber.Error
	if errors.As(err, &fe) {
		return fe.Code
	}
	return fiber.StatusInternalServerError
}
