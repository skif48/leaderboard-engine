// Package client is the bot's HTTP client for the leaderboard-engine API. It
// owns connection pooling, the global request-rate limiter, retry/backoff,
// error classification and per-request metrics, so the engine only deals with
// "did this action land or not".
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/skif48/leaderboard-engine/bot/internal/logx"
	"github.com/skif48/leaderboard-engine/bot/internal/metrics"
	"github.com/skif48/leaderboard-engine/bot/internal/xtime"
	"github.com/skif48/leaderboard-engine/entities"
)

// Kind classifies a failed request.
type Kind int

const (
	KindNone     Kind = iota
	KindNotFound      // 404: the user does not exist (yet, or any more)
	KindClient        // other 4xx: almost certainly a bot bug, never retried
	KindServer        // 5xx / 429
	KindNetwork       // dial / connection errors
	KindTimeout       // per-attempt deadline exceeded
	KindCanceled      // the caller's context ended
	KindDecode        // 2xx with an unusable body
)

func (k Kind) String() string {
	switch k {
	case KindNotFound:
		return "not_found"
	case KindClient:
		return "client_error"
	case KindServer:
		return "server_error"
	case KindNetwork:
		return "network"
	case KindTimeout:
		return "timeout"
	case KindCanceled:
		return "canceled"
	case KindDecode:
		return "decode"
	default:
		return "none"
	}
}

// metricLabel is the value recorded in bot_http_requests_total{status} for
// attempts that never produced a status code.
func (k Kind) metricLabel() string {
	switch k {
	case KindNetwork:
		return "net_error"
	case KindTimeout:
		return "timeout"
	case KindCanceled:
		return "canceled"
	default:
		return "error"
	}
}

// APIError is returned for every failed call after retries are exhausted.
type APIError struct {
	Endpoint string
	Status   int
	Kind     Kind
	Attempts int
	Body     string
	Err      error
}

func (e *APIError) Error() string {
	if e.Status > 0 {
		return fmt.Sprintf("%s: %s (status %d, %d attempt(s)): %v", e.Endpoint, e.Kind, e.Status, e.Attempts, e.Err)
	}
	return fmt.Sprintf("%s: %s (%d attempt(s)): %v", e.Endpoint, e.Kind, e.Attempts, e.Err)
}

func (e *APIError) Unwrap() error { return e.Err }

type Options struct {
	BaseURL string
	// MaxConns bounds in-flight requests (transport MaxConnsPerHost).
	MaxConns int
	// MaxRPS caps the request rate across all callers; 0 disables the limiter.
	MaxRPS float64
	// Throttle rate-limits error logging; a default is created when nil.
	Throttle *logx.Throttle
}

type Client struct {
	http     *http.Client
	base     string
	limiter  *rate.Limiter
	throttle *logx.Throttle

	signUpPolicy       retryPolicy
	actionPolicy       retryPolicy
	leaderboardsPolicy retryPolicy
}

func New(o Options) *Client {
	if o.MaxConns <= 0 {
		o.MaxConns = 100
	}
	transport := &http.Transport{
		MaxIdleConns:        o.MaxConns,
		MaxIdleConnsPerHost: o.MaxConns,
		MaxConnsPerHost:     o.MaxConns,
		IdleConnTimeout:     90 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	var limiter *rate.Limiter
	if o.MaxRPS > 0 {
		burst := int(o.MaxRPS)
		if burst < 1 {
			burst = 1
		}
		limiter = rate.NewLimiter(rate.Limit(o.MaxRPS), burst)
	}
	throttle := o.Throttle
	if throttle == nil {
		throttle = logx.New(10 * time.Second)
	}
	return &Client{
		http:               &http.Client{Transport: transport},
		base:               strings.TrimRight(o.BaseURL, "/"),
		limiter:            limiter,
		throttle:           throttle,
		signUpPolicy:       defaultSignUpPolicy,
		actionPolicy:       defaultActionPolicy,
		leaderboardsPolicy: defaultLeaderboardsPolicy,
	}
}

// SignUp registers a user and returns the server-assigned profile.
func (c *Client) SignUp(ctx context.Context, nickname string) (*entities.UserProfile, error) {
	body, err := json.Marshal(entities.SignUpRequest{Nickname: nickname})
	if err != nil {
		return nil, err
	}
	var profile entities.UserProfile
	err = c.do(ctx, "sign_up", http.MethodPost, "/api/v1/users/sign-up", body, c.signUpPolicy, func(r *http.Response) error {
		return json.NewDecoder(r.Body).Decode(&profile)
	})
	if err != nil {
		return nil, err
	}
	if profile.Id == "" {
		return nil, &APIError{Endpoint: "sign_up", Kind: KindDecode, Attempts: 1, Err: errors.New("sign-up response has no id")}
	}
	return &profile, nil
}

// SendAction submits a game action for an existing user. The server ignores
// leaderboard_id and never reads timestamp, but both are part of the contract.
func (c *Client) SendAction(ctx context.Context, userID, action string) error {
	body, err := json.Marshal(entities.GameAction{
		UserId:    userID,
		Action:    action,
		Timestamp: float64(time.Now().UnixMilli()) / 1000,
	})
	if err != nil {
		return err
	}
	return c.do(ctx, "action", http.MethodPost, "/api/v1/users/actions", body, c.actionPolicy, nil)
}

// GetLeaderboards fetches the HTML leaderboard page and discards it. It is
// best-effort read traffic and is never retried.
func (c *Client) GetLeaderboards(ctx context.Context) error {
	return c.do(ctx, "leaderboards", http.MethodGet, "/leaderboards", nil, c.leaderboardsPolicy, nil)
}

func (c *Client) do(ctx context.Context, endpoint, method, path string, body []byte, pol retryPolicy, decode func(*http.Response) error) error {
	notFoundTries, transientTries := 0, 0
	for attempt := 1; ; attempt++ {
		apiErr := c.attempt(ctx, endpoint, method, path, body, pol.perAttempt, decode)
		if apiErr == nil {
			return nil
		}
		apiErr.Attempts = attempt
		if apiErr.Kind == KindCanceled {
			return apiErr
		}

		var spec backoffSpec
		var counter *int
		var reason string
		switch {
		case apiErr.Kind == KindNotFound:
			spec, counter, reason = pol.notFound, &notFoundTries, "not_found"
		case apiErr.Kind == KindServer:
			spec, counter, reason = pol.transient, &transientTries, "server_error"
		case apiErr.Kind == KindNetwork:
			spec, counter, reason = pol.transient, &transientTries, "network"
		case apiErr.Kind == KindTimeout:
			spec, counter, reason = pol.transient, &transientTries, "timeout"
		}
		if counter == nil || *counter+1 >= spec.attempts {
			c.logFailure(apiErr)
			return apiErr
		}
		*counter++
		metrics.HTTPRetry(endpoint, reason)
		if !xtime.Sleep(ctx, spec.delay(*counter)) {
			apiErr.Kind = KindCanceled
			return apiErr
		}
	}
}

func (c *Client) attempt(ctx context.Context, endpoint, method, path string, body []byte, timeout time.Duration, decode func(*http.Response) error) *APIError {
	if c.limiter != nil {
		if err := c.limiter.Wait(ctx); err != nil {
			kind := KindTimeout
			if ctx.Err() != nil {
				kind = KindCanceled
			}
			return &APIError{Endpoint: endpoint, Kind: kind, Err: err}
		}
	}
	actx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(actx, method, c.base+path, rdr)
	if err != nil {
		return &APIError{Endpoint: endpoint, Kind: KindClient, Err: err}
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	start := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		kind := classifyErr(ctx, err)
		metrics.HTTPRequest(endpoint, kind.metricLabel(), start)
		return &APIError{Endpoint: endpoint, Kind: kind, Err: err}
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	metrics.HTTPRequest(endpoint, strconv.Itoa(resp.StatusCode), start)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if decode != nil {
			if err := decode(resp); err != nil {
				return &APIError{Endpoint: endpoint, Status: resp.StatusCode, Kind: KindDecode, Err: err}
			}
		}
		return nil
	}
	return &APIError{
		Endpoint: endpoint,
		Status:   resp.StatusCode,
		Kind:     classifyStatus(resp.StatusCode),
		Body:     readSnippet(resp.Body, 256),
		Err:      fmt.Errorf("unexpected status %d", resp.StatusCode),
	}
}

func classifyStatus(status int) Kind {
	switch {
	case status == http.StatusNotFound:
		return KindNotFound
	case status == http.StatusTooManyRequests:
		return KindServer
	case status >= 500:
		return KindServer
	case status >= 400:
		return KindClient
	default:
		return KindDecode
	}
}

func classifyErr(ctx context.Context, err error) Kind {
	if ctx.Err() != nil {
		return KindCanceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return KindTimeout
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return KindTimeout
	}
	return KindNetwork
}

func readSnippet(r io.Reader, n int) string {
	b, _ := io.ReadAll(io.LimitReader(r, int64(n)))
	return strings.TrimSpace(string(b))
}

func (c *Client) logFailure(e *APIError) {
	args := []any{"endpoint", e.Endpoint, "kind", e.Kind.String(), "attempts", e.Attempts, "error", e.Err}
	if e.Status > 0 {
		args = append(args, "status", e.Status)
	}
	if e.Body != "" {
		args = append(args, "body", e.Body)
	}
	c.throttle.Error(e.Endpoint+"/"+e.Kind.String(), "request failed", args...)
}
