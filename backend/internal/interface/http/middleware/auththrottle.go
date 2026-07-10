package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/interface/http/response"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/reqmeta"
)

// IPThrottleLimiter is the sliding-window limiter the auth throttle needs
// (implemented by redisx.LoginRateLimiter). Declared here to keep the middleware
// package off the infrastructure package, mirroring RequestCounter.
type IPThrottleLimiter interface {
	Allow(ctx context.Context, key string) (bool, time.Duration, error)
}

// AuthThrottle rate-limits the unauthenticated auth endpoints (register,
// email-verify request, password forgot/reset) per client IP. Login is throttled
// separately by the usecase on (email, IP); these endpoints have no trusted email
// to key on, so IP is the abuse boundary against enumeration and email-bombing.
//
// Redis trouble fails OPEN (log + allow), matching RateLimiter: an availability
// blip must not lock users out of account recovery.
type AuthThrottle struct {
	Limiter IPThrottleLimiter
	Logger  *slog.Logger
}

// PerIP returns a middleware that throttles by "auth:{tag}:{ip}". tag scopes the
// window per endpoint so a burst of password resets doesn't starve registration.
func (t *AuthThrottle) PerIP(tag string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := reqmeta.From(r.Context()).IP
			if ip == "" { // unparseable client address: nothing safe to key on
				next.ServeHTTP(w, r)
				return
			}
			allowed, retryAfter, err := t.Limiter.Allow(r.Context(), "rl:auth:"+tag+":"+ip)
			if err != nil {
				t.Logger.Error("auth throttle: limiter failed; allowing", "tag", tag, "err", err)
				next.ServeHTTP(w, r)
				return
			}
			if !allowed {
				response.AuthThrottled(w, int(retryAfter.Seconds()+0.999))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
