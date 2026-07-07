// This file is the org-scoped notification center surface (docs/09-REALTIME-JOBS.md
// §3, FR-NTF-002/004). All routes run under TenantGuard with read:org; a user
// only ever sees and mutates their own notifications (userID from TenantContext,
// never a path/body param — invariant #1 defense-in-depth).
package handlers

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/notify"
	mw "github.com/mesutokul/fluxboard/backend/internal/interface/http/middleware"
	"github.com/mesutokul/fluxboard/backend/internal/interface/http/response"
	"github.com/mesutokul/fluxboard/backend/internal/usecase/notifyuc"
)

// NotificationHandlers serves the in-app notification center + delivery prefs.
type NotificationHandlers struct {
	svc    *notifyuc.Service
	logger *slog.Logger
}

// NewNotificationHandlers builds NotificationHandlers.
func NewNotificationHandlers(svc *notifyuc.Service, logger *slog.Logger) *NotificationHandlers {
	if logger == nil {
		logger = slog.Default()
	}
	return &NotificationHandlers{svc: svc, logger: logger}
}

// notificationListCap bounds a single list page (FR-NTF-002).
const notificationListCap = 50

// ---- DTOs -----------------------------------------------------------------

type notificationResp struct {
	ID         string     `json:"id"`
	Category   string     `json:"category"`
	Title      string     `json:"title"`
	Body       string     `json:"body"`
	EntityType string     `json:"entity_type,omitempty"`
	EntityID   string     `json:"entity_id,omitempty"`
	ReadAt     *time.Time `json:"read_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

func toNotificationResp(n notify.Notification) notificationResp {
	return notificationResp{
		ID: n.ID, Category: string(n.Category), Title: n.Title, Body: n.Body,
		EntityType: n.EntityType, EntityID: n.EntityID,
		ReadAt: n.ReadAt, CreatedAt: n.CreatedAt,
	}
}

type prefResp struct {
	Category string `json:"category"`
	Email    bool   `json:"email"`
	InApp    bool   `json:"in_app"`
}

// ---- Read -----------------------------------------------------------------

// List returns the caller's notifications newest-first (FR-NTF-002). Query:
// ?unread=1 (unread tab), ?limit=N (≤50), ?before=RFC3339 (cursor).
func (h *NotificationHandlers) List(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	q := r.URL.Query()
	onlyUnread := q.Get("unread") == "1" || q.Get("unread") == "true"
	limit := notificationListCap
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n < limit {
			limit = n
		}
	}
	var before *time.Time
	if v := q.Get("before"); v != "" {
		t, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			response.Error(w, domain.ErrValidation)
			return
		}
		tt := t.UTC()
		before = &tt
	}
	ns, err := h.svc.List(r.Context(), tc.OrgID, tc.UserID, onlyUnread, limit, before)
	if err != nil {
		response.Error(w, err)
		return
	}
	out := make([]notificationResp, 0, len(ns))
	for i := range ns {
		out = append(out, toNotificationResp(ns[i]))
	}
	response.JSON(w, http.StatusOK, map[string]any{"notifications": out})
}

// UnreadCount returns the caller's unread total (badge count).
func (h *NotificationHandlers) UnreadCount(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	n, err := h.svc.UnreadCount(r.Context(), tc.OrgID, tc.UserID)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"unread": n})
}

// ---- Mutations ------------------------------------------------------------

// MarkRead stamps one notification read (404 if absent / not the caller's).
func (h *NotificationHandlers) MarkRead(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	id := chi.URLParam(r, "id")
	if err := h.svc.MarkRead(r.Context(), tc.OrgID, tc.UserID, id); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// MarkAllRead stamps every unread notification read; returns the count affected.
func (h *NotificationHandlers) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	n, err := h.svc.MarkAllRead(r.Context(), tc.OrgID, tc.UserID)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"marked": n})
}

// ---- Preferences (FR-NTF-004) ---------------------------------------------

// GetPrefs returns the caller's delivery-preference matrix (defaults filled in
// for categories with no stored row).
func (h *NotificationHandlers) GetPrefs(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	prefs, err := h.svc.GetPrefs(r.Context(), tc.OrgID, tc.UserID)
	if err != nil {
		response.Error(w, err)
		return
	}
	out := make([]prefResp, 0, len(prefs))
	for _, p := range prefs {
		out = append(out, prefResp{Category: string(p.Category), Email: p.Email, InApp: p.InApp})
	}
	response.JSON(w, http.StatusOK, map[string]any{"prefs": out})
}

type setPrefReq struct {
	Category string `json:"category"`
	Email    bool   `json:"email"`
	InApp    bool   `json:"in_app"`
}

// SetPref upserts one category's delivery preference for the caller.
func (h *NotificationHandlers) SetPref(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	var req setPrefReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.svc.SetPref(r.Context(), tc.OrgID, tc.UserID, notify.Category(req.Category), req.Email, req.InApp); err != nil {
		response.Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
