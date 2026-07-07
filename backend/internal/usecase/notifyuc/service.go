// Package notifyuc holds the application services for the notification center
// and realtime fan-out (docs/09-REALTIME-JOBS.md §3, FR-NTF-002/003/004).
// Domain-only deps: it consumes the notify ports plus two local adapter ports
// (Directory, OutboxWriter). It never imports another usecase package (clean
// layering); producers call it through interfaces they declare locally.
package notifyuc

import (
	"context"
	"encoding/json"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/notify"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/uuidv7"
)

// UserRef is a directory entry used for @mention resolution and email targeting.
type UserRef struct {
	ID    string
	Email string
	Name  string
}

// Directory resolves project membership and user contact info for fan-out.
// Implemented by a Postgres adapter (joins project_members + users).
type Directory interface {
	// ProjectMembers returns the members of a project (for @mention matching).
	ProjectMembers(ctx context.Context, orgID, projectID string) ([]UserRef, error)
	// UsersByID resolves contact info for specific user ids (targeted notifs).
	UsersByID(ctx context.Context, orgID string, ids []string) ([]UserRef, error)
}

// OutboxWriter enqueues an email:send outbox row in its own tenant tx (09 §2).
// Implemented by the Postgres OutboxRepo.
type OutboxWriter interface {
	InsertEmail(ctx context.Context, orgID string, payload json.RawMessage) error
}

// EmailPayload is the Phase-5 notification variant of the email:send outbox
// payload. The email:send worker rechecks the (UserID, Category) preference
// before sending (09 §3). Distinguished from the billing payload by UserID being
// set (billing uses Template + OrgID).
type EmailPayload struct {
	OrgID    string `json:"org_id"`
	UserID   string `json:"user_id"`
	Category string `json:"category"`
	ToEmail  string `json:"to_email,omitempty"`
	Subject  string `json:"subject"`
	Body     string `json:"body"`
}

// Deps wires the notification service.
type Deps struct {
	Notifs notify.NotificationRepository
	Prefs  notify.PrefRepository
	Bus    notify.EventBus // nil ⇒ no realtime publish (tests / SSE disabled)
	Dir    Directory       // nil ⇒ no @mention resolution
	Outbox OutboxWriter    // nil ⇒ no email fan-out
	Logger *slog.Logger
	Now    func() time.Time
}

// Service implements the notification application logic.
type Service struct {
	notifs notify.NotificationRepository
	prefs  notify.PrefRepository
	bus    notify.EventBus
	dir    Directory
	outbox OutboxWriter
	logger *slog.Logger
	now    func() time.Time
}

// New builds a Service from Deps.
func New(d Deps) *Service {
	now := d.Now
	if now == nil {
		now = time.Now
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		notifs: d.Notifs, prefs: d.Prefs, bus: d.Bus, dir: d.Dir,
		outbox: d.Outbox, logger: logger, now: now,
	}
}

func newID() string { return uuidv7.New().String() }

// ---- Notification center reads (5.3.1) ------------------------------------

const maxListLimit = 50

// List returns a user's notifications newest-first. limit is capped at 50.
func (s *Service) List(ctx context.Context, orgID, userID string, onlyUnread bool, limit int, before *time.Time) ([]notify.Notification, error) {
	if limit <= 0 || limit > maxListLimit {
		limit = maxListLimit
	}
	return s.notifs.List(ctx, orgID, notify.ListFilter{
		UserID: userID, OnlyUnread: onlyUnread, Limit: limit, Before: before,
	})
}

// UnreadCount returns the user's unread total.
func (s *Service) UnreadCount(ctx context.Context, orgID, userID string) (int, error) {
	return s.notifs.UnreadCount(ctx, orgID, userID)
}

// MarkRead marks one of the user's notifications read.
func (s *Service) MarkRead(ctx context.Context, orgID, userID, id string) error {
	return s.notifs.MarkRead(ctx, orgID, userID, id, s.now().UTC())
}

// MarkAllRead marks every unread notification of the user read; returns count.
func (s *Service) MarkAllRead(ctx context.Context, orgID, userID string) (int, error) {
	return s.notifs.MarkAllRead(ctx, orgID, userID, s.now().UTC())
}

// ---- Preferences (5.3.6 producer side; FR-NTF-004) ------------------------

// GetPrefs returns the user's delivery matrix, filling absent categories with
// the opt-in default so the client always renders a full grid.
func (s *Service) GetPrefs(ctx context.Context, orgID, userID string) ([]notify.Pref, error) {
	stored, err := s.prefs.GetForUser(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}
	out := make([]notify.Pref, 0, len(notify.CenterCategories()))
	for _, cat := range notify.CenterCategories() {
		if p, ok := stored[cat]; ok {
			out = append(out, p)
			continue
		}
		out = append(out, notify.DefaultPref(orgID, userID, cat))
	}
	return out, nil
}

// SetPref upserts one category's delivery toggles.
func (s *Service) SetPref(ctx context.Context, orgID, userID string, cat notify.Category, email, inApp bool) error {
	if !cat.Valid() {
		return domain.ErrValidation
	}
	return s.prefs.Upsert(ctx, orgID, notify.Pref{
		OrgID: orgID, UserID: userID, Category: cat, Email: email, InApp: inApp,
	})
}

// ---- Fan-out (5.3.2 / 5.3.3) ----------------------------------------------

// mentionRE matches @handle tokens where handle is an email local-part
// (users have no separate username, so @alice targets alice@… — see
// docs/build/PHASE-5 §3). Case-insensitive match is applied after lowering.
var mentionRE = regexp.MustCompile(`@([A-Za-z0-9._%+\-]+)`)

// parseMentions extracts the distinct lowercased handles referenced in text.
func parseMentions(text string) []string {
	ms := mentionRE.FindAllStringSubmatch(text, -1)
	seen := map[string]bool{}
	var out []string
	for _, m := range ms {
		h := strings.ToLower(m[1])
		if !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	return out
}

// handle returns the lowercased email local-part of a user (the @mention key).
func handle(u UserRef) string {
	if i := strings.IndexByte(u.Email, '@'); i > 0 {
		return strings.ToLower(u.Email[:i])
	}
	return strings.ToLower(u.Email)
}

// target is one resolved fan-out recipient with its category + rendered copy.
type target struct {
	ref        UserRef
	cat        notify.Category
	title      string
	body       string
	entityType string
	entityID   string
}

// FanOutComment resolves @mentions among project members and, together with the
// explicit commentTargets (task creator/assignee, supplied by the caller so this
// service needs no task repo), creates in-app notifications + email outbox rows
// for opted-in recipients and publishes notification.created (09 §3). The actor
// never notifies themselves. Best-effort: a fan-out error is logged, never
// blocks the comment (the comment already committed in taskuc).
func (s *Service) FanOutComment(ctx context.Context, orgID, actorID, projectID, taskID, commentID, body string, commentTargets []string) {
	if s.dir == nil {
		return
	}
	members, err := s.dir.ProjectMembers(ctx, orgID, projectID)
	if err != nil {
		s.logger.Error("notify: project members lookup failed", "err", err, "project", projectID)
		return
	}
	byHandle := make(map[string]UserRef, len(members))
	byID := make(map[string]UserRef, len(members))
	for _, m := range members {
		byHandle[handle(m)] = m
		byID[m.ID] = m
	}

	// category per user: mention wins over comment when a user is both.
	cats := map[string]notify.Category{}
	for _, h := range parseMentions(body) {
		if m, ok := byHandle[h]; ok && m.ID != actorID {
			cats[m.ID] = notify.CategoryMention
		}
	}
	for _, uid := range commentTargets {
		if uid == "" || uid == actorID {
			continue
		}
		if _, ok := cats[uid]; ok {
			continue // already a mention
		}
		cats[uid] = notify.CategoryComment
	}
	if len(cats) == 0 {
		return
	}

	var ts []target
	for uid, cat := range cats {
		ref, ok := byID[uid]
		if !ok {
			// comment target may not be a project member (rare); resolve directly.
			refs, err := s.dir.UsersByID(ctx, orgID, []string{uid})
			if err != nil || len(refs) == 0 {
				continue
			}
			ref = refs[0]
		}
		title, bd := commentCopy(cat, ref.Name)
		ts = append(ts, target{ref: ref, cat: cat, title: title, body: bd, entityType: "task", entityID: taskID})
	}
	s.deliver(ctx, orgID, actorID, ts)
}

// commentCopy renders the notification title/body for a comment/mention.
func commentCopy(cat notify.Category, _ string) (title, body string) {
	if cat == notify.CategoryMention {
		return "You were mentioned in a comment", "Someone mentioned you in a task comment."
	}
	return "New comment on a task you follow", "There is a new comment on a task you created or are assigned to."
}

// NotifyAssigned notifies a task's new assignee (skips self-assignment).
func (s *Service) NotifyAssigned(ctx context.Context, orgID, actorID, taskID, assigneeID, taskTitle string) {
	if assigneeID == "" || assigneeID == actorID || s.dir == nil {
		return
	}
	refs, err := s.dir.UsersByID(ctx, orgID, []string{assigneeID})
	if err != nil || len(refs) == 0 {
		if err != nil {
			s.logger.Error("notify: assignee lookup failed", "err", err)
		}
		return
	}
	title := "You were assigned a task"
	body := "You have been assigned to “" + taskTitle + "”."
	s.deliver(ctx, orgID, actorID, []target{{
		ref: refs[0], cat: notify.CategoryTaskAssigned,
		title: title, body: body, entityType: "task", entityID: taskID,
	}})
}

// NotifyTargets fans a single category to explicit user ids (invite accepted,
// billing status). Used by tenant/billing producers.
func (s *Service) NotifyTargets(ctx context.Context, orgID, actorID string, cat notify.Category, userIDs []string, title, body, entityType, entityID string) {
	if len(userIDs) == 0 || s.dir == nil {
		return
	}
	refs, err := s.dir.UsersByID(ctx, orgID, userIDs)
	if err != nil {
		s.logger.Error("notify: targets lookup failed", "err", err, "cat", cat)
		return
	}
	var ts []target
	for _, r := range refs {
		if r.ID == actorID {
			continue
		}
		ts = append(ts, target{ref: r, cat: cat, title: title, body: body, entityType: entityType, entityID: entityID})
	}
	s.deliver(ctx, orgID, actorID, ts)
}

// deliver creates in-app rows for opted-in targets, enqueues email outbox rows
// (rechecked at send), and publishes notification.created per recipient. Pref is
// read per target; absent ⇒ opt-in default (both channels).
func (s *Service) deliver(ctx context.Context, orgID, actorID string, ts []target) {
	if len(ts) == 0 {
		return
	}
	now := s.now().UTC()
	var rows []notify.Notification
	var emails []target
	for _, t := range ts {
		pref := s.prefFor(ctx, orgID, t.ref.ID, t.cat)
		if notify.WantsChannel(pref, t.cat, notify.ChannelInApp) {
			rows = append(rows, notify.Notification{
				ID: newID(), OrgID: orgID, UserID: t.ref.ID, Category: t.cat,
				Title: t.title, Body: t.body, EntityType: t.entityType, EntityID: t.entityID,
				CreatedAt: now,
			})
		}
		if s.outbox != nil && notify.WantsChannel(pref, t.cat, notify.ChannelEmail) {
			emails = append(emails, t)
		}
	}
	if len(rows) > 0 {
		if err := s.notifs.CreateBatch(ctx, orgID, rows); err != nil {
			s.logger.Error("notify: create rows failed", "err", err)
			return
		}
	}
	// notification.created events (targeted by user_id, client-filtered).
	for _, r := range rows {
		s.publishNotificationCreated(ctx, orgID, actorID, r)
	}
	// email outbox — send-time pref recheck happens in the worker.
	for _, t := range emails {
		p := EmailPayload{
			OrgID: orgID, UserID: t.ref.ID, Category: string(t.cat), ToEmail: t.ref.Email,
			Subject: t.title, Body: t.body,
		}
		raw, err := json.Marshal(p)
		if err != nil {
			continue
		}
		if err := s.outbox.InsertEmail(ctx, orgID, raw); err != nil {
			s.logger.Error("notify: outbox insert failed", "err", err, "user", t.ref.ID)
		}
	}
}

func (s *Service) publishNotificationCreated(ctx context.Context, orgID, actorID string, n notify.Notification) {
	if s.bus == nil {
		return
	}
	ev := notify.NewEvent(notify.EventNotificationCreated, actorID, map[string]any{
		"notification_id": n.ID,
		"category":        string(n.Category),
		"user_id":         n.UserID,
	})
	if _, err := s.bus.Publish(ctx, orgID, ev); err != nil {
		s.logger.Warn("notify: publish notification.created failed", "err", err)
	}
}

// prefFor reads a target's stored pref for a category (nil ⇒ default applies).
func (s *Service) prefFor(ctx context.Context, orgID, userID string, cat notify.Category) *notify.Pref {
	p, ok, err := s.prefs.Get(ctx, orgID, userID, cat)
	if err != nil || !ok {
		return nil
	}
	return &p
}
