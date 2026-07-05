package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	mw "github.com/mesutokul/fluxboard/backend/internal/interface/http/middleware"
	"github.com/mesutokul/fluxboard/backend/internal/interface/http/response"
)

// --- Password: forgot / reset / change ------------------------------------

type forgotReq struct {
	Email string `json:"email"`
}

// PasswordForgot always returns 202 (no enumeration); the email is sent by the
// usecase when the account exists.
func (h *AuthHandlers) PasswordForgot(w http.ResponseWriter, r *http.Request) {
	var req forgotReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.svc.ForgotPassword(r.Context(), req.Email); err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusAccepted, map[string]string{
		"message": "If the email is valid, a reset link has been sent.",
	})
}

type resetReq struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

// PasswordReset consumes a reset token and sets a new password (revokes all
// sessions). Clears the refresh cookie since every session is now dead.
func (h *AuthHandlers) PasswordReset(w http.ResponseWriter, r *http.Request) {
	var req resetReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.svc.ResetPassword(r.Context(), req.Token, req.Password); err != nil {
		response.Error(w, err)
		return
	}
	h.clearRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

type changeReq struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// PasswordChange updates the caller's password (requires auth) and revokes every
// session — the client must re-authenticate afterwards.
func (h *AuthHandlers) PasswordChange(w http.ResponseWriter, r *http.Request) {
	var req changeReq
	if !decodeJSON(w, r, &req) {
		return
	}
	p, _ := mw.PrincipalFrom(r.Context())
	if err := h.svc.ChangePassword(r.Context(), p.UserID, req.CurrentPassword, req.NewPassword, p.SID); err != nil {
		response.Error(w, err)
		return
	}
	h.clearRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// --- Email verification (resend) ------------------------------------------

// VerifyEmailRequest re-sends the verification link. Always 202 (no enumeration).
func (h *AuthHandlers) VerifyEmailRequest(w http.ResponseWriter, r *http.Request) {
	var req forgotReq // reuse {email}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.svc.ResendEmailVerify(r.Context(), req.Email); err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusAccepted, map[string]string{
		"message": "If the email is valid and unverified, a verification link has been sent.",
	})
}

// --- Sessions -------------------------------------------------------------

type sessionResp struct {
	ID        string    `json:"id"`
	UserAgent string    `json:"user_agent"`
	IP        string    `json:"ip,omitempty"`
	Current   bool      `json:"current"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Sessions lists the caller's live sessions, flagging the one making the request.
func (h *AuthHandlers) Sessions(w http.ResponseWriter, r *http.Request) {
	p, _ := mw.PrincipalFrom(r.Context())
	sessions, err := h.svc.ListSessions(r.Context(), p.UserID)
	if err != nil {
		response.Error(w, err)
		return
	}
	items := make([]sessionResp, 0, len(sessions))
	for _, s := range sessions {
		items = append(items, sessionResp{
			ID: s.ID, UserAgent: s.UserAgent, IP: s.IP,
			Current: s.ID == p.SID, CreatedAt: s.CreatedAt, ExpiresAt: s.ExpiresAt,
		})
	}
	response.JSON(w, http.StatusOK, map[string]any{"items": items})
}

// RevokeSession revokes one of the caller's sessions by id.
func (h *AuthHandlers) RevokeSession(w http.ResponseWriter, r *http.Request) {
	p, _ := mw.PrincipalFrom(r.Context())
	id := chi.URLParam(r, "id")
	if err := h.svc.RevokeSession(r.Context(), p.UserID, id); err != nil {
		response.Error(w, err)
		return
	}
	// Revoking the current session invalidates the refresh cookie too.
	if id == p.SID {
		h.clearRefreshCookie(w)
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- TOTP 2FA -------------------------------------------------------------

type enroll2FAResp struct {
	Secret          string `json:"secret"`
	ProvisioningURI string `json:"provisioning_uri"`
}

// Enroll2FA generates a TOTP secret for the caller (not yet active).
func (h *AuthHandlers) Enroll2FA(w http.ResponseWriter, r *http.Request) {
	p, _ := mw.PrincipalFrom(r.Context())
	res, err := h.svc.Enroll2FA(r.Context(), p.UserID)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusOK, enroll2FAResp{Secret: res.Secret, ProvisioningURI: res.ProvisioningURI})
}

type codeReq struct {
	Code string `json:"code"`
}

// Activate2FA confirms a code and turns on 2FA, returning one-time recovery codes.
func (h *AuthHandlers) Activate2FA(w http.ResponseWriter, r *http.Request) {
	var req codeReq
	if !decodeJSON(w, r, &req) {
		return
	}
	p, _ := mw.PrincipalFrom(r.Context())
	codes, err := h.svc.Activate2FA(r.Context(), p.UserID, req.Code)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"recovery_codes": codes})
}

// Disable2FA turns off 2FA after re-verifying a current code/recovery code.
func (h *AuthHandlers) Disable2FA(w http.ResponseWriter, r *http.Request) {
	var req codeReq
	if !decodeJSON(w, r, &req) {
		return
	}
	p, _ := mw.PrincipalFrom(r.Context())
	if err := h.svc.Disable2FA(r.Context(), p.UserID, req.Code); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type verify2FAReq struct {
	PendingToken string `json:"pending_token"`
	Code         string `json:"code"`
}

// Verify2FA exchanges a pending-2FA token + code for a session.
func (h *AuthHandlers) Verify2FA(w http.ResponseWriter, r *http.Request) {
	var req verify2FAReq
	if !decodeJSON(w, r, &req) {
		return
	}
	tokens, err := h.svc.Verify2FA(r.Context(), req.PendingToken, req.Code, r.UserAgent(), clientIP(r))
	if err != nil {
		response.Error(w, err)
		return
	}
	h.writeSession(w, &tokens)
}

// --- Google OAuth (PKCE) --------------------------------------------------

// OAuthGoogleStart redirects the browser to Google's consent screen.
func (h *AuthHandlers) OAuthGoogleStart(w http.ResponseWriter, r *http.Request) {
	if h.states == nil {
		response.Error(w, domain.ErrForbidden) // OAuth not configured
		return
	}
	redirectAfter := sanitizeRedirect(r.URL.Query().Get("redirect"))
	url, err := h.svc.StartOAuth(r.Context(), h.states, redirectAfter)
	if err != nil {
		response.Error(w, err)
		return
	}
	http.Redirect(w, r, url, http.StatusFound)
}

// OAuthGoogleCallback completes the flow: validates state, exchanges the code,
// sets the refresh cookie, and redirects back to the web app with the access
// token in the URL fragment (kept out of server logs / Referer).
func (h *AuthHandlers) OAuthGoogleCallback(w http.ResponseWriter, r *http.Request) {
	if h.states == nil {
		response.Error(w, domain.ErrForbidden)
		return
	}
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		response.Error(w, domain.ErrUnauthorized)
		return
	}
	res, err := h.svc.CompleteOAuth(r.Context(), h.states, q.Get("code"), q.Get("state"), r.UserAgent(), clientIP(r))
	if err != nil {
		response.Error(w, err)
		return
	}
	h.setRefreshCookie(w, res.Tokens.RefreshToken, res.Tokens.RefreshExpiresAt)
	dest := strings.TrimRight(h.webOrigin, "/") + sanitizeRedirect(res.RedirectAfter) +
		"#access_token=" + res.Tokens.AccessToken
	http.Redirect(w, r, dest, http.StatusFound)
}

// sanitizeRedirect confines post-login redirects to same-origin paths so the
// flow cannot be turned into an open redirect. Anything not starting with a
// single "/" (or starting with "//") collapses to "/".
func sanitizeRedirect(p string) string {
	if p == "" || !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") {
		return "/"
	}
	return p
}
