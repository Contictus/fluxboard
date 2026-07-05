package postgres

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// parseUUID converts a domain string id to uuid.UUID; invalid ids surface as an
// error to the caller (they should never occur for ids we minted).
func parseUUID(s string) (uuid.UUID, error) { return uuid.Parse(s) }

// tsPtr maps a nullable timestamptz to *time.Time (nil when SQL NULL).
func tsPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

// ptrOrNil returns nil for the empty string, else &s — for nullable text params.
func ptrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// tsVal wraps a time.Time as a valid pgtype.Timestamptz (for non-null params).
func tsVal(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}
