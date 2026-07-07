package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/mesutokul/fluxboard/backend/internal/domain/notify"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres/gen"
)

// PrefRepo is the Postgres-backed notify.PrefRepository. notification_prefs is
// [T] (RLS); every method runs through TenantPool.WithTenant (invariant #1).
type PrefRepo struct{ tp *TenantPool }

// NewPrefRepo builds a PrefRepo over the tenant pool.
func NewPrefRepo(tp *TenantPool) *PrefRepo { return &PrefRepo{tp: tp} }

var _ notify.PrefRepository = (*PrefRepo)(nil)

// GetForUser returns the user's stored prefs keyed by category. Missing
// categories fall back to DefaultPref in the domain.
func (r *PrefRepo) GetForUser(ctx context.Context, orgID, userID string) (map[notify.Category]notify.Pref, error) {
	out := map[notify.Category]notify.Pref{}
	err := r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		uid, err := parseUUID(userID)
		if err != nil {
			return err
		}
		rows, err := q.ListNotificationPrefs(ctx, gen.ListNotificationPrefsParams{OrgID: oid, UserID: uid})
		if err != nil {
			return err
		}
		for _, row := range rows {
			cat := notify.Category(row.Category)
			out[cat] = notify.Pref{
				OrgID:    row.OrgID.String(),
				UserID:   row.UserID.String(),
				Category: cat,
				Email:    row.Email,
				InApp:    row.InApp,
			}
		}
		return nil
	})
	return out, err
}

// Get returns the stored pref for one (user, category); ok=false ⇒ default
// applies. Used at send-time recheck by the worker (09 §3).
func (r *PrefRepo) Get(ctx context.Context, orgID, userID string, cat notify.Category) (notify.Pref, bool, error) {
	var (
		p  notify.Pref
		ok bool
	)
	err := r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		uid, err := parseUUID(userID)
		if err != nil {
			return err
		}
		row, err := q.GetNotificationPref(ctx, gen.GetNotificationPrefParams{
			OrgID:    oid,
			UserID:   uid,
			Category: string(cat),
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil // ok stays false ⇒ caller applies DefaultPref
			}
			return err
		}
		p = notify.Pref{
			OrgID:    row.OrgID.String(),
			UserID:   row.UserID.String(),
			Category: notify.Category(row.Category),
			Email:    row.Email,
			InApp:    row.InApp,
		}
		ok = true
		return nil
	})
	return p, ok, err
}

// Upsert writes one preference row (set-semantics on the PK).
func (r *PrefRepo) Upsert(ctx context.Context, orgID string, p notify.Pref) error {
	return r.tp.WithTenant(ctx, orgID, func(q *gen.Queries) error {
		oid, _ := parseUUID(orgID)
		uid, err := parseUUID(p.UserID)
		if err != nil {
			return err
		}
		return q.UpsertNotificationPref(ctx, gen.UpsertNotificationPrefParams{
			OrgID:    oid,
			UserID:   uid,
			Category: string(p.Category),
			Email:    p.Email,
			InApp:    p.InApp,
		})
	})
}
