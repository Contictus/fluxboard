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

// nullTS maps a *time.Time to a nullable timestamptz param (nil ⇒ SQL NULL).
func nullTS(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

// nullableUUID maps an optional string id to a nullable uuid param. An empty/nil
// id is SQL NULL; a malformed id surfaces as an error. (Distinct from the audit
// package's best-effort nullUUID which swallows parse errors.)
func nullableUUID(s *string) (pgtype.UUID, error) {
	if s == nil || *s == "" {
		return pgtype.UUID{}, nil
	}
	u, err := uuid.Parse(*s)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return pgtype.UUID{Bytes: u, Valid: true}, nil
}

// uuidStrPtr maps a nullable uuid column to *string (nil when SQL NULL).
func uuidStrPtr(u pgtype.UUID) *string {
	if !u.Valid {
		return nil
	}
	s := uuid.UUID(u.Bytes).String()
	return &s
}

// int32Ptr narrows a *int to *int32 for nullable int params (nil ⇒ SQL NULL).
func int32Ptr(p *int) *int32 {
	if p == nil {
		return nil
	}
	v := int32(*p)
	return &v
}

// intPtr widens a nullable *int32 column to *int (nil when SQL NULL).
func intPtr(p *int32) *int {
	if p == nil {
		return nil
	}
	v := int(*p)
	return &v
}
