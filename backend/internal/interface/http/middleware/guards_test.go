package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mesutokul/fluxboard/backend/internal/domain/apikey"
)

func runGuard(guard func(http.Handler) http.Handler, r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	guard(next).ServeHTTP(rec, r)
	return rec
}

// FR-ADM-003: an impersonation token may read but never write.
func TestImpersonationReadOnly(t *testing.T) {
	tests := []struct {
		name         string
		method       string
		impersonated bool
		want         int
	}{
		{"read allowed", http.MethodGet, true, http.StatusOK},
		{"write blocked", http.MethodPost, true, http.StatusForbidden},
		{"patch blocked", http.MethodPatch, true, http.StatusForbidden},
		{"delete blocked", http.MethodDelete, true, http.StatusForbidden},
		{"non-impersonated write allowed", http.MethodPost, false, http.StatusOK},
		{"no tenant context passes", http.MethodPost, false, http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, "/orgs/org1/x", nil)
			if tc.name != "no tenant context passes" {
				ctx := context.WithValue(r.Context(), ctxKeyTenant,
					TenantContext{OrgID: "org1", Impersonated: tc.impersonated})
				r = r.WithContext(ctx)
			}
			rec := runGuard(ImpersonationReadOnly, r)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}

// FR-ADM-003: an impersonation token is read-only everywhere, keyed off the
// Principal's `imp` claim — so it also guards the org-independent secured routes.
func TestRejectImpersonationWrite(t *testing.T) {
	tests := []struct {
		name   string
		method string
		imp    string
		want   int
	}{
		{"imp GET allowed", http.MethodGet, "org1", http.StatusOK},
		{"imp POST blocked", http.MethodPost, "org1", http.StatusForbidden},
		{"imp PATCH blocked", http.MethodPatch, "org1", http.StatusForbidden},
		{"imp DELETE blocked", http.MethodDelete, "org1", http.StatusForbidden},
		{"non-imp POST allowed", http.MethodPost, "", http.StatusOK},
		{"no principal passes", http.MethodPost, "\x00none", http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, "/me", nil)
			if tc.imp != "\x00none" {
				r = r.WithContext(WithPrincipal(r.Context(), Principal{UserID: "admin", ImpersonatedOrg: tc.imp}))
			}
			rec := runGuard(RejectImpersonationWrite, r)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}

// FR-API-002: a read-scoped API key may read but never write; a write key may do both.
func TestAPIKeyScopeGuard(t *testing.T) {
	readKey := APIKeyInfo{OrgID: "org1", Scopes: []apikey.Scope{apikey.ScopeRead}}
	writeKey := APIKeyInfo{OrgID: "org1", Scopes: []apikey.Scope{apikey.ScopeWrite}}
	tests := []struct {
		name   string
		method string
		info   *APIKeyInfo
		want   int
	}{
		{"read key GET ok", http.MethodGet, &readKey, http.StatusOK},
		{"read key POST 403", http.MethodPost, &readKey, http.StatusForbidden},
		{"write key POST ok", http.MethodPost, &writeKey, http.StatusOK},
		{"no api-key context passes", http.MethodPost, nil, http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, "/orgs/org1/x", nil)
			if tc.info != nil {
				r = r.WithContext(WithAPIKey(r.Context(), *tc.info))
			}
			rec := runGuard(APIKeyScopeGuard, r)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}
