package middleware

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/pkg/reqmeta"
)

type fakeThrottleLimiter struct {
	allow  bool
	retry  time.Duration
	err    error
	gotKey string
}

func (f *fakeThrottleLimiter) Allow(_ context.Context, key string) (bool, time.Duration, error) {
	f.gotKey = key
	return f.allow, f.retry, f.err
}

func newThrottle(l IPThrottleLimiter) *AuthThrottle {
	return &AuthThrottle{Limiter: l, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func withIP(r *http.Request, ip string) *http.Request {
	return r.WithContext(reqmeta.WithMeta(r.Context(), reqmeta.Meta{IP: ip}))
}

func TestAuthThrottle_PerIP(t *testing.T) {
	t.Run("allowed request passes and keys by tag+ip", func(t *testing.T) {
		lim := &fakeThrottleLimiter{allow: true}
		r := withIP(httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/auth/register", nil), "1.2.3.4")
		rec := runGuard(newThrottle(lim).PerIP("register"), r)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if lim.gotKey != "rl:auth:register:1.2.3.4" {
			t.Fatalf("key = %q", lim.gotKey)
		}
	})

	t.Run("over-limit returns 429 with Retry-After", func(t *testing.T) {
		lim := &fakeThrottleLimiter{allow: false, retry: 42 * time.Second}
		r := withIP(httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/auth/password/forgot", nil), "1.2.3.4")
		rec := runGuard(newThrottle(lim).PerIP("password_forgot"), r)
		if rec.Code != http.StatusTooManyRequests {
			t.Fatalf("status = %d, want 429", rec.Code)
		}
		if got := rec.Header().Get("Retry-After"); got != "42" {
			t.Fatalf("Retry-After = %q, want 42", got)
		}
	})

	t.Run("limiter error fails open", func(t *testing.T) {
		lim := &fakeThrottleLimiter{err: errors.New("redis down")}
		r := withIP(httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/auth/register", nil), "1.2.3.4")
		rec := runGuard(newThrottle(lim).PerIP("register"), r)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (fail-open)", rec.Code)
		}
	})

	t.Run("missing IP is not throttled", func(t *testing.T) {
		lim := &fakeThrottleLimiter{allow: false}
		r := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/auth/register", nil) // no reqmeta IP
		rec := runGuard(newThrottle(lim).PerIP("register"), r)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (no IP to key on)", rec.Code)
		}
	})
}
