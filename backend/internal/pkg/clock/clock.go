// Package clock — deterministic time abstraction for tests and production.
// All timestamps are UTC (see docs/CLAUDE.md §Non-Negotiable Invariants #6).
package clock

import "time"

// Clock yields the current instant. Inject it instead of calling time.Now
// directly so usecases can be frozen in tests.
type Clock interface {
	Now() time.Time
}

// System is the production Clock backed by the wall clock, always in UTC.
type System struct{}

// Now returns the current UTC time.
func (System) Now() time.Time { return time.Now().UTC() }

// Fixed is a Clock frozen at T, for tests.
type Fixed struct{ T time.Time }

// Now returns the frozen instant.
func (f Fixed) Now() time.Time { return f.T }
