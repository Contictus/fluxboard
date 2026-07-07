package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/notify"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres/gen"
)

// NotificationRepo is the Postgres-backed notify.NotificationRepository.
// notifications is [T] (RLS), so every method runs through TenantPool.WithTenant
// (invariant #1).
type NotificationRepo struct{ tp *TenantPool }

// NewNotificationRepo builds a NotificationRepo over the tenant pool.
func NewNotificationRepo(tp *TenantPool) *NotificationRepo { return &NotificationRepo{tp: tp} }

var _ notify.NotificationRepository = (*NotificationRepo)(nil)

// CreateBatch inserts fan-out rows in one tenant tx. Rows with an empty ID get a
// fresh uuid; an empty slice is a no-op.
func (r *NotificationRepo) CreateBatch(ctx context.Context, orgID string, ns []notify.Notification) error {
	if len(ns) == 0 {
		return nil
	}
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		for _, n := range ns {
			id := n.ID
			if id == "" {
				id = uuid.NewString()
			}
			nid, err := parseUUID(id)
			if err != nil {
				return err
			}
			uid, err := parseUUID(n.UserID)
			if err != nil {
				return err
			}
			created := n.CreatedAt
			if created.IsZero() {
				created = time.Now().UTC()
			}
			if err := q.InsertNotification(ctx, gen.InsertNotificationParams{
				ID:         nid,
				OrgID:      oid,
				UserID:     uid,
				Category:   string(n.Category),
				Title:      n.Title,
				Body:       n.Body,
				EntityType: strPtr(n.EntityType),
				EntityID:   strPtr(n.EntityID),
				CreatedAt:  created.UTC(),
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

// List returns a user's notifications newest-first per the filter.
func (r *NotificationRepo) List(ctx context.Context, orgID string, f notify.ListFilter) ([]notify.Notification, error) {
	var out []notify.Notification
	err := r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		uid, err := parseUUID(f.UserID)
		if err != nil {
			return err
		}
		rows, err := q.ListNotifications(ctx, gen.ListNotificationsParams{
			OrgID:      oid,
			UserID:     uid,
			OnlyUnread: f.OnlyUnread,
			Before:     nullTS(f.Before),
			Lim:        int32(f.Limit),
		})
		if err != nil {
			return err
		}
		for _, row := range rows {
			out = append(out, notify.Notification{
				ID:         row.ID.String(),
				OrgID:      row.OrgID.String(),
				UserID:     row.UserID.String(),
				Category:   notify.Category(row.Category),
				Title:      row.Title,
				Body:       row.Body,
				EntityType: derefStr(row.EntityType),
				EntityID:   derefStr(row.EntityID),
				ReadAt:     tsPtr(row.ReadAt),
				CreatedAt:  row.CreatedAt,
			})
		}
		return nil
	})
	return out, err
}

// UnreadCount returns the user's unread total.
func (r *NotificationRepo) UnreadCount(ctx context.Context, orgID, userID string) (int, error) {
	var n int
	err := r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		uid, err := parseUUID(userID)
		if err != nil {
			return err
		}
		c, err := q.CountUnreadNotifications(ctx, gen.CountUnreadNotificationsParams{OrgID: oid, UserID: uid})
		if err != nil {
			return err
		}
		n = int(c)
		return nil
	})
	return n, err
}

// MarkRead stamps read_at on one notification the user owns; ErrNotFound if it is
// absent, already read, or belongs to another user.
func (r *NotificationRepo) MarkRead(ctx context.Context, orgID, userID, id string, at time.Time) error {
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		uid, err := parseUUID(userID)
		if err != nil {
			return err
		}
		nid, err := parseUUID(id)
		if err != nil {
			return err
		}
		rows, err := q.MarkNotificationRead(ctx, gen.MarkNotificationReadParams{
			ReadAt: tsVal(at.UTC()),
			OrgID:  oid,
			UserID: uid,
			ID:     nid,
		})
		if err != nil {
			return err
		}
		if rows == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// MarkAllRead stamps read_at on all of the user's unread rows; returns the count
// affected.
func (r *NotificationRepo) MarkAllRead(ctx context.Context, orgID, userID string, at time.Time) (int, error) {
	var n int
	err := r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		uid, err := parseUUID(userID)
		if err != nil {
			return err
		}
		rows, err := q.MarkAllNotificationsRead(ctx, gen.MarkAllNotificationsReadParams{
			ReadAt: tsVal(at.UTC()),
			OrgID:  oid,
			UserID: uid,
		})
		if err != nil {
			return err
		}
		n = int(rows)
		return nil
	})
	return n, err
}

// strPtr returns nil for an empty string (nullable text columns), else &s.
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
