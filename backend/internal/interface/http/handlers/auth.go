// Package handlers holds the HTTP handlers, one file per resource. This file is
// the /auth/* surface (docs/04-AUTH.md §5). Handlers decode/validate the
// request, call a usecase, and render via the response package; no business
// logic lives here.
package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/auth"
	mw "github.com/mesutokul/fluxboard/backend/internal/interface/http/middleware"
	"github.com/mesutokul/fluxboard/backend/internal/interface/http/response"
	"github.com/mesutokul/fluxboard/backend/internal/usecase/authuc"
)

const (
	refreshCookieName = "fluxboard_refresh"
	refreshCookiePath = "/api/v1/auth"
	maxBodyBytes      = 1 << 20 // 1 MiB
)

// AuthHandlers serves the authentication endpoints.
type AuthHandlers struct {
	svc          *authuc.Service
	states       auth.OAuthStateStore
	logger       *slog.Logger
	cookieSecure bool
	webOrigin    string
}

// AuthConfig carries the collaborators for the auth surface.
type AuthConfig struct {
	Service   *authuc.Service
	States    auth.OAuthStateStore // nil when Google OAuth is unconfigured
	Logger    *slog.Logger
	WebOrigin string
	// CookieSecure should be true in prod (HTTPS); false in local dev over http
	// so the cookie is still set.
	CookieSecure bool
}

// NewAuthHandlers builds AuthHandlers from AuthConfig.
func NewAuthHandlers(c AuthConfig) *AuthHandlers {
	return &AuthHandlers{
		svc: c.Service, states: c.States, logger: c.Logger,
		cookieSecure: c.CookieSecure, webOrigin: c.WebOrigin,
	}
}

type registerReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

// Register creates an account. The response is identical whether or not the
// email was already taken (no enumeration, docs/04-AUTH.md §6).
func (h *AuthHandlers) Register(w http.ResponseWriter, r *http.Request) {
	var req registerReq
	if !decodeJSON(w, r, &req) {
		return
	}
	res, err := h.svc.Register(r.Context(), authuc.RegisterInput{
		Email: req.Email, Password: req.Password, Name: req.Name,
	})
	if err != nil {
		response.Error(w, err)
		return
	}
	// The verification link is sent by the usecase via the mailer (dev builds
	// log it when SMTP is unconfigured). Response is identical regardless of
	// whether an account was created (no enumeration).
	_ = res
	response.JSON(w, http.StatusAccepted, map[string]string{
		"message": "If the email is valid, a verification link has been sent.",
	})
}

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type tokenResp struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	ExpiresAt   time.Time `json:"expires_at"`
	UserID      string    `json:"user_id"`
}

type twoFactorResp struct {
	Status       string `json:"status"` // always "2fa_required"
	PendingToken string `json:"pending_token"`
}

// Login authenticates and either issues an access token (JSON) + refresh cookie,
// or — for TOTP users — returns a pending-2FA token to exchange at /auth/2fa/verify.
func (h *AuthHandlers) Login(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if !decodeJSON(w, r, &req) {
		return
	}
	res, err := h.svc.Login(r.Context(), authuc.LoginInput{
		Email: req.Email, Password: req.Password,
		UserAgent: r.UserAgent(), IP: clientIP(r),
	})
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if res.TwoFactor {
		response.JSON(w, http.StatusOK, twoFactorResp{
			Status: "2fa_required", PendingToken: res.PendingToken,
		})
		return
	}
	h.writeSession(w, res.Tokens)
}

// writeSession sets the refresh cookie and renders the access-token body shared
// by login, 2FA verify, and OAuth callback.
func (h *AuthHandlers) writeSession(w http.ResponseWriter, t *authuc.Tokens) {
	h.setRefreshCookie(w, t.RefreshToken, t.RefreshExpiresAt)
	response.JSON(w, http.StatusOK, tokenResp{
		AccessToken: t.AccessToken, TokenType: "Bearer",
		ExpiresAt: t.AccessExpiresAt, UserID: t.UserID,
	})
}

// writeAuthError adds a Retry-After header for throttled logins, then renders
// the standard error envelope.
func writeAuthError(w http.ResponseWriter, err error) {
	var rle *authuc.RateLimitError
	if errors.As(err, &rle) {
		secs := int(rle.RetryAfter.Seconds())
		if secs < 1 {
			secs = 1
		}
		w.Header().Set("Retry-After", strconv.Itoa(secs))
	}
	response.Error(w, err)
}

// Refresh rotates the refresh-cookie token. CSRF posture (docs/04-AUTH.md §5):
// SameSite=Strict cookie + required X-Requested-With: fetch header (Origin is
// allow-listed by CORS). Any failure clears the cookie.
func (h *AuthHandlers) Refresh(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Requested-With") != "fetch" {
		response.Error(w, domain.ErrForbidden)
		return
	}
	c, err := r.Cookie(refreshCookieName)
	if err != nil {
		response.Error(w, domain.ErrUnauthorized)
		return
	}
	tokens, err := h.svc.Refresh(r.Context(), c.Value, r.UserAgent(), clientIP(r))
	if err != nil {
		h.clearRefreshCookie(w)
		response.Error(w, err)
		return
	}
	h.setRefreshCookie(w, tokens.RefreshToken, tokens.RefreshExpiresAt)
	response.JSON(w, http.StatusOK, tokenResp{
		AccessToken: tokens.AccessToken, TokenType: "Bearer",
		ExpiresAt: tokens.AccessExpiresAt, UserID: tokens.UserID,
	})
}

// Logout revokes the current session (requires auth).
func (h *AuthHandlers) Logout(w http.ResponseWriter, r *http.Request) {
	p, _ := mw.PrincipalFrom(r.Context())
	if err := h.svc.Logout(r.Context(), p.SID); err != nil {
		response.Error(w, err)
		return
	}
	h.clearRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// LogoutAll revokes every session of the caller (requires auth).
func (h *AuthHandlers) LogoutAll(w http.ResponseWriter, r *http.Request) {
	p, _ := mw.PrincipalFrom(r.Context())
	if err := h.svc.LogoutAll(r.Context(), p.UserID, p.SID); err != nil {
		response.Error(w, err)
		return
	}
	h.clearRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

type verifyEmailReq struct {
	Token string `json:"token"`
}

// VerifyEmailConfirm consumes an email-verification token.
func (h *AuthHandlers) VerifyEmailConfirm(w http.ResponseWriter, r *http.Request) {
	var req verifyEmailReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.svc.VerifyEmail(r.Context(), req.Token); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- cookie + helpers -----------------------------------------------------

func (h *AuthHandlers) setRefreshCookie(w http.ResponseWriter, value string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    value,
		Path:     refreshCookiePath,
		Expires:  expires,
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
}

func (h *AuthHandlers) clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     refreshCookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
}

// decodeJSON reads a size-limited JSON body. On failure it writes a 422 and
// returns false.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		response.Error(w, errors.Join(domain.ErrValidation, err))
		return false
	}
	return true
}

// clientIP extracts a best-effort client IP for the session row. Trusts
// X-Forwarded-For's first hop when present (dev/behind-proxy); otherwise
// RemoteAddr. Returns "" when unparseable (stored as NULL).
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if host, _, err := net.SplitHostPort(xff); err == nil {
			return host
		}
		return xff
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return ""
	}
	return host
}
