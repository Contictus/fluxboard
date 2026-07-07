package notify

import (
	"context"
	"time"
)

// Repositories persist notification data. All three tables are tenant-owned
// ([T]) and take orgID first, opening a tenant-scoped transaction (RLS backstop,
// invariant #1). Reads return domain.ErrNotFound when a single row is absent.

// ListFilter parameters a notification list query (FR-NTF-002). Cursor pagination
// by created_at DESC: Before nil ⇒ newest page.
type ListFilter struct {
	UserID     string
	OnlyUnread bool
	Limit      int        // capped by the handler
	Before     *time.Time // exclusive upper bound on created_at (cursor)
}

// NotificationRepository persists the in-app notification center.
type NotificationRepository interface {
	// CreateBatch inserts fan-out rows in one tenant tx (one comment/@mention may
	// target several users). No-op on an empty slice.
	CreateBatch(ctx context.Context, orgID string, ns []Notification) error
	// List returns a user's notifications newest-first per the filter.
	List(ctx context.Context, orgID string, f ListFilter) ([]Notification, error)
	// UnreadCount returns the user's unread total.
	UnreadCount(ctx context.Context, orgID, userID string) (int, error)
	// MarkRead stamps read_at on one notification the user owns; ErrNotFound if
	// it is absent or belongs to another user.
	MarkRead(ctx context.Context, orgID, userID, id string, at time.Time) error
	// MarkAllRead stamps read_at on all of the user's unread rows; returns the
	// count affected.
	MarkAllRead(ctx context.Context, orgID, userID string, at time.Time) (int, error)
}

// PrefRepository persists per-user delivery preferences (FR-NTF-004).
type PrefRepository interface {
	// GetForUser returns the user's stored prefs keyed by category. Missing
	// categories fall back to DefaultPref in the domain.
	GetForUser(ctx context.Context, orgID, userID string) (map[Category]Pref, error)
	// Get returns the stored pref for one (user, category); ok=false ⇒ default
	// applies. Used at send-time recheck by the worker (09 §3).
	Get(ctx context.Context, orgID, userID string, cat Category) (p Pref, ok bool, err error)
	// Upsert writes one preference row (set-semantics on the PK).
	Upsert(ctx context.Context, orgID string, p Pref) error
}

// StatsRepository backs the nightly stats:rollup and Phase-6 analytics reads
// (FR-AN-001).
type StatsRepository interface {
	// ProjectIDs lists an org's non-archived project ids (rollup iteration).
	ProjectIDs(ctx context.Context, orgID string) ([]string, error)
	// ComputeDay aggregates a project's task activity for the given UTC day into
	// a ProjectStat (source read over tasks/activity).
	ComputeDay(ctx context.Context, orgID, projectID string, day time.Time) (ProjectStat, error)
	// Upsert writes the daily rollup row (set-semantics, re-runnable).
	Upsert(ctx context.Context, orgID string, s ProjectStat) error
}

// ---- Realtime bus (adapter port) ------------------------------------------

// EventBus is the outbound realtime port (09 §1). The Redis Stream
// implementation makes SSE horizontally scalable: producers Publish to
// events:{orgId}; each API replica Subscribes one consumer per org with active
// clients; a reconnecting client Replays from its Last-Event-ID.
type EventBus interface {
	// Publish appends ev to the org stream (XADD MAXLEN ~1000) and returns the
	// assigned entry id (the SSE id).
	Publish(ctx context.Context, orgID string, ev Event) (id string, err error)
	// Subscribe returns a channel of live events for the org and a cancel func
	// the caller MUST invoke to release the subscription. The channel closes when
	// ctx is done or cancel is called.
	Subscribe(ctx context.Context, orgID string) (events <-chan Event, cancel func(), err error)
	// Replay returns events after lastID (exclusive). gap=true when lastID is
	// older than stream retention (caller emits a resync instead of the backlog).
	Replay(ctx context.Context, orgID, lastID string) (events []Event, gap bool, err error)
}
