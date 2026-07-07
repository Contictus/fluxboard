package jobs

// This file holds the Phase 5 §7 notification + analytics jobs
// (docs/09-REALTIME-JOBS.md §2/§3): the shared email:send handler (which
// dispatches notification vs billing payloads and rechecks delivery prefs at
// send-time) and the nightly project-stats rollup. org:hard_delete,
// audit:retention and webhook:retry are deferred — see the deferral note in
// docs/build/PHASE-5-REALTIME-JOBS.md §7.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"

	"github.com/mesutokul/fluxboard/backend/internal/domain/notify"
	"github.com/mesutokul/fluxboard/backend/internal/usecase/notifyuc"
)

// TypeStatsRollup recomputes yesterday's project_stats_daily grain (FR-AN-001).
const TypeStatsRollup = "stats:rollup"

// NotificationMailer delivers one rendered notification email (implemented by
// mailer.Mailer).
type NotificationMailer interface {
	SendNotification(ctx context.Context, to, subject, body string) error
}

// NotifyDeps wires the notification/analytics job handlers.
type NotifyDeps struct {
	Prefs  notify.PrefRepository
	Stats  notify.StatsRepository
	Mailer NotificationMailer
	Orgs   OrgLister
	// BillingEmail handles a billing-shaped email:send payload (no user_id). The
	// outbox multiplexes billing + notification emails under one kind, so the
	// single email:send handler delegates the billing case here.
	BillingEmail func(ctx context.Context, t *asynq.Task) error
	Logger       *slog.Logger
	Now          func() time.Time // nil ⇒ time.Now
}

// Notify runs the Phase 5 notification email delivery + stats rollup.
type Notify struct{ d NotifyDeps }

// NewNotify builds the notification job handlers.
func NewNotify(d NotifyDeps) *Notify {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Notify{d: d}
}

// Register wires the handlers onto an Asynq mux. Notify owns the shared
// email:send handler (see Billing.Register).
func (n *Notify) Register(mux *asynq.ServeMux) {
	mux.HandleFunc(TypeEmailSend, n.HandleEmailSend)
	mux.HandleFunc(TypeStatsRollup, n.HandleStatsRollup)
}

// ---- email:send -------------------------------------------------------------

// HandleEmailSend delivers one outbox email. The outbox multiplexes two payload
// shapes under the email:send kind: a notification payload (carries user_id) and
// a billing payload (carries template). A notification is delivered only after a
// send-time preference recheck (09 §3), so a user who opted out of email between
// enqueue and delivery is not mailed. Billing payloads delegate to Billing.
func (n *Notify) HandleEmailSend(ctx context.Context, t *asynq.Task) error {
	var p notifyuc.EmailPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("email:send: bad payload: %w", err)
	}
	if p.UserID == "" {
		// Billing-shaped payload (template/org_id). Delegate.
		if n.d.BillingEmail == nil {
			n.d.Logger.Warn("email:send: billing payload but no billing handler wired; dropping")
			return nil
		}
		return n.d.BillingEmail(ctx, t)
	}

	cat := notify.Category(p.Category)
	// Send-time preference recheck (09 §3). Missing row ⇒ opt-in default; a
	// transactional category bypasses prefs inside WantsChannel.
	var prefPtr *notify.Pref
	if n.d.Prefs != nil {
		pref, ok, err := n.d.Prefs.Get(ctx, p.OrgID, p.UserID, cat)
		if err != nil {
			return fmt.Errorf("email:send: pref recheck: %w", err)
		}
		if ok {
			prefPtr = &pref
		}
	}
	if !notify.WantsChannel(prefPtr, cat, notify.ChannelEmail) {
		n.d.Logger.Info("email:send: recipient opted out of email; skipping",
			"org", p.OrgID, "user", p.UserID, "category", p.Category)
		return nil
	}
	if p.ToEmail == "" {
		n.d.Logger.Warn("email:send: notification payload has no recipient address; dropping",
			"org", p.OrgID, "user", p.UserID)
		return nil
	}
	return n.d.Mailer.SendNotification(ctx, p.ToEmail, p.Subject, p.Body)
}

// ---- stats:rollup -----------------------------------------------------------

// HandleStatsRollup recomputes yesterday's project_stats_daily grain for every
// org's projects. Set-semantics upsert ⇒ re-runnable (FR-AN-001).
func (n *Notify) HandleStatsRollup(ctx context.Context, _ *asynq.Task) error {
	day := utcDay(n.d.Now().Add(-24 * time.Hour))
	return forEachOrg(ctx, n.d.Orgs, n.d.Logger, TypeStatsRollup, func(orgID string) (int, error) {
		pids, err := n.d.Stats.ProjectIDs(ctx, orgID)
		if err != nil {
			return 0, err
		}
		count := 0
		for _, pid := range pids {
			stat, err := n.d.Stats.ComputeDay(ctx, orgID, pid, day)
			if err != nil {
				return count, err
			}
			if err := n.d.Stats.Upsert(ctx, orgID, stat); err != nil {
				return count, err
			}
			count++
		}
		return count, nil
	})
}
