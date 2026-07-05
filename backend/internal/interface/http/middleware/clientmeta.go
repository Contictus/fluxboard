package middleware

import (
	"net"
	"net/http"
	"strings"

	"github.com/mesutokul/fluxboard/backend/internal/pkg/reqmeta"
)

// ClientMeta stashes the client IP and user-agent on the request context so deep
// usecase code can enrich audit entries without threading them through every
// signature (see pkg/reqmeta). Installed early in the infrastructure chain.
func ClientMeta(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := reqmeta.WithMeta(r.Context(), reqmeta.Meta{
			IP:        clientIP(r),
			UserAgent: r.UserAgent(),
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// clientIP resolves the caller IP: the first hop of X-Forwarded-For when present
// (the edge proxy prepends the real client), else the RemoteAddr host. Returns ""
// when nothing parses so the audit column stays NULL rather than storing garbage.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		first := strings.TrimSpace(strings.SplitN(xff, ",", 2)[0])
		if net.ParseIP(first) != nil {
			return first
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if net.ParseIP(host) == nil {
		return ""
	}
	return host
}
