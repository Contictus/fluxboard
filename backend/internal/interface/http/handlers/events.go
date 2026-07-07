// This file is the Server-Sent-Events realtime stream (docs/09-REALTIME-JOBS.md
// §1, FR-NTF-001). GET /orgs/{orgId}/events opens a text/event-stream: on connect
// it replays any events after the client's Last-Event-ID (or emits `resync` when
// that id predates stream retention), then forwards live org events until the
// client disconnects. One Redis Stream per org backs the bus, so any API replica
// can serve any subscriber.
package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/notify"
	mw "github.com/mesutokul/fluxboard/backend/internal/interface/http/middleware"
	"github.com/mesutokul/fluxboard/backend/internal/interface/http/response"
	"github.com/mesutokul/fluxboard/backend/internal/usecase/notifyuc"
)

// sseHeartbeat is the keep-alive cadence; it matches the bus XREAD BLOCK window
// so a live connection writes at least one frame every ~25s (proxies drop idle
// streams sooner).
const sseHeartbeat = 25 * time.Second

// EventHandlers serves the SSE realtime stream.
type EventHandlers struct {
	svc    *notifyuc.Service
	logger *slog.Logger
}

// NewEventHandlers builds EventHandlers.
func NewEventHandlers(svc *notifyuc.Service, logger *slog.Logger) *EventHandlers {
	if logger == nil {
		logger = slog.Default()
	}
	return &EventHandlers{svc: svc, logger: logger}
}

// Stream is the SSE endpoint (FR-NTF-001).
func (h *EventHandlers) Stream(w http.ResponseWriter, r *http.Request) {
	tc, ok := mw.TenantFrom(r.Context())
	if !ok {
		response.Error(w, domain.ErrForbidden)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		// A non-streaming ResponseWriter can't serve SSE (should not happen with
		// the stdlib server; guards test recorders).
		response.Error(w, domain.ErrValidation)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable proxy buffering (nginx)
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx := r.Context()

	// Replay backlog (or resync) from Last-Event-ID before going live.
	lastID := r.Header.Get("Last-Event-ID")
	if lastID == "" {
		lastID = r.URL.Query().Get("last") // curl/browser fallback
	}
	backlog, resync, err := h.svc.StreamInit(ctx, tc.OrgID, lastID)
	if err != nil {
		h.logger.Error("sse stream init failed", "org", tc.OrgID, "err", err)
		// Headers already sent; degrade to a resync so the client refetches.
		resync, backlog = true, nil
	}
	if resync {
		h.writeResync(w)
		flusher.Flush()
	}
	for _, ev := range backlog {
		h.writeEvent(w, ev)
	}
	flusher.Flush()

	// Go live.
	events, cancel, err := h.svc.Subscribe(ctx, tc.OrgID)
	if err != nil {
		h.logger.Error("sse subscribe failed", "org", tc.OrgID, "err", err)
		return
	}
	defer cancel()

	ticker := time.NewTicker(sseHeartbeat)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case ev, open := <-events:
			if !open {
				return
			}
			h.writeEvent(w, ev)
			flusher.Flush()
		case <-ticker.C:
			if _, err := fmt.Fprint(w, ":ka\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// writeEvent serializes one event as an SSE frame (id/event/data). A marshal
// error skips the event rather than tearing down the stream.
func (h *EventHandlers) writeEvent(w http.ResponseWriter, ev notify.Event) {
	data, err := json.Marshal(ev.Data)
	if err != nil {
		h.logger.Warn("sse event marshal failed", "event", ev.Name, "err", err)
		return
	}
	if ev.ID != "" {
		fmt.Fprintf(w, "id: %s\n", ev.ID)
	}
	fmt.Fprintf(w, "event: %s\n", ev.Name)
	fmt.Fprintf(w, "data: %s\n\n", data)
}

// writeResync emits the server-initiated resync frame (client invalidates caches
// and refetches; 09 §1).
func (h *EventHandlers) writeResync(w http.ResponseWriter) {
	fmt.Fprintf(w, "event: %s\n", notify.EventResync)
	fmt.Fprint(w, "data: {}\n\n")
}
