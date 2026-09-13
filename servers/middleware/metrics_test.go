package middleware

import (
	"bytes"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/VictoriaMetrics/metrics"
	"github.com/gofiber/fiber/v3"
)

// TestMetricsMiddlewareLabels checks that the route label is the registered
// pattern (never a path parameter value), that unmatched paths and wrong
// methods collapse into one label, and that the status reflects errors
// returned from the chain.
func TestMetricsMiddlewareLabels(t *testing.T) {
	app := fiber.New()
	app.Use(MetricsMiddleware())
	app.Get("/api/v1/users/:userId/profile", func(c fiber.Ctx) error {
		if c.Params("userId") == "missing" {
			return c.SendStatus(fiber.StatusNotFound)
		}
		return c.SendString("ok")
	})
	app.Get("/boom", func(c fiber.Ctx) error { return errors.New("boom") })
	app.Get("/teapot", func(c fiber.Ctx) error { return fiber.NewError(fiber.StatusTeapot) })

	requests := []struct{ method, path string }{
		{"GET", "/api/v1/users/11111111-1111-1111-1111-111111111111/profile"},
		{"GET", "/api/v1/users/22222222-2222-2222-2222-222222222222/profile"},
		{"GET", "/api/v1/users/missing/profile"},
		{"POST", "/api/v1/users/x/profile"}, // 405
		{"GET", "/nope"},                    // 404
		{"GET", "/boom"},                    // 500 via non-fiber error
		{"GET", "/teapot"},                  // 418 via fiber.Error
		{"GET", "/metrics"},                 // skipped
	}
	for _, r := range requests {
		if _, err := app.Test(httptest.NewRequest(r.method, r.path, nil)); err != nil {
			t.Fatalf("%s %s: %v", r.method, r.path, err)
		}
	}

	var buf bytes.Buffer
	metrics.WritePrometheus(&buf, false)
	out := buf.String()

	want := []string{
		`engine_http_requests_total{route="/api/v1/users/:userId/profile",method="GET",status="200"} 2`,
		`engine_http_requests_total{route="/api/v1/users/:userId/profile",method="GET",status="404"} 1`,
		`engine_http_requests_total{route="<unmatched>",method="POST",status="405"} 1`,
		`engine_http_requests_total{route="<unmatched>",method="GET",status="404"} 1`,
		`engine_http_requests_total{route="/boom",method="GET",status="500"} 1`,
		`engine_http_requests_total{route="/teapot",method="GET",status="418"} 1`,
	}
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("missing series %q", w)
		}
	}
	for _, bad := range []string{"11111111", "22222222", `route="/metrics"`, `route="/nope"`} {
		if strings.Contains(out, bad) {
			t.Errorf("unexpected %q in output", bad)
		}
	}
	if t.Failed() {
		t.Log(out)
	}
}
