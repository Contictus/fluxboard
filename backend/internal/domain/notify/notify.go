// Package notify models the Phase 5 realtime + notification domain: the
// in-app Notification, per-user delivery Preference, the realtime Event, the
// daily project-stats rollup grain, and their ports. Stdlib-only
// (docs/03-ARCHITECTURE.md ADR-001); times are UTC (invariant #6).
package notify

import "time"

// ---- Categories & channels ------------------------------------------------

// Category classifies a notification and keys its delivery preferences
// (docs/09-REALTIME-JOBS.md §3, FR-NTF-002/004).
type Category string

const (
	CategoryTaskAssigned   Category = "task_assigned"
	CategoryMention        Category = "mention"
	CategoryComment        Category = "comment"
	CategoryInviteAccepted Category = "invite_accepted"
	CategoryBilling        Category = "billing"

	// Auth-flow categories never create in-app rows; they exist only so
	// transactional emails (verify/reset) bypass user preferences via the
	// whitelist below (FR-NTF-003). They are not stored in notification_prefs.
	CategoryAuthVerify Category = "auth_verify"
	CategoryAuthReset  Category = "auth_reset"
)

// centerCategories are the categories that surface in the notification center
// and expose preference toggles.
var centerCategories = map[Category]bool{
	CategoryTaskAssigned:   true,
	CategoryMention:        true,
	CategoryComment:        true,
	CategoryInviteAccepted: true,
	CategoryBilling:        true,
}

// Valid reports whether c is a known notification-center category.
func (c Category) Valid() bool { return centerCategories[c] }

// Transactional reports whether c is an auth-flow category whose email always
// sends regardless of stored preferences (FR-NTF-003 whitelist).
func (c Category) Transactional() bool {
	switch c {
	case CategoryAuthVerify, CategoryAuthReset:
		return true
	default:
		return false
	}
}

// CenterCategories returns the preference-exposing categories (stable order for
// the prefs API + defaults seeding).
func CenterCategories() []Category {
	return []Category{
		CategoryTaskAssigned,
		CategoryMention,
		CategoryComment,
		CategoryInviteAccepted,
		CategoryBilling,
	}
}

// Channel is a delivery channel for a notification.
type Channel string

const (
	ChannelEmail Channel = "email"
	ChannelInApp Channel = "in_app"
)

// ---- Models ---------------------------------------------------------------

// Notification is one in-app notification row (FR-NTF-002). ReadAt nil ⇒ unread.
// EntityType/EntityID deep-link the client to the source (task/comment/…).
type Notification struct {
	ID         string
	OrgID      string
	UserID     string
	Category   Category
	Title      string
	Body       string
	EntityType string
	EntityID   string
	ReadAt     *time.Time
	CreatedAt  time.Time
}

// Pref is a user's delivery preference for one category in one org
// (FR-NTF-004). Absent row ⇒ DefaultPref (both channels on).
type Pref struct {
	OrgID    string
	UserID   string
	Category Category
	Email    bool
	InApp    bool
}

// DefaultPref is the opt-in default applied when a user has no stored row for a
// category (both channels enabled).
func DefaultPref(orgID, userID string, cat Category) Pref {
	return Pref{OrgID: orgID, UserID: userID, Category: cat, Email: true, InApp: true}
}

// WantsChannel reports whether a notification of category cat should be
// delivered to a user on channel ch, given their stored pref for that category
// (nil ⇒ default on). Transactional auth emails bypass preferences entirely
// (FR-NTF-003).
func WantsChannel(pref *Pref, cat Category, ch Channel) bool {
	if ch == ChannelEmail && cat.Transactional() {
		return true
	}
	if pref == nil {
		return true // no stored row ⇒ opt-in default
	}
	switch ch {
	case ChannelEmail:
		return pref.Email
	case ChannelInApp:
		return pref.InApp
	default:
		return false
	}
}

// ---- Project stats rollup grain (FR-AN-001) -------------------------------

// ProjectStat is one (project, day) rollup row backing analytics (Phase 6).
// Recomputed nightly by stats:rollup with set-semantics (re-runnable).
type ProjectStat struct {
	OrgID           string
	ProjectID       string
	Day             time.Time // UTC date
	CreatedCount    int
	CompletedCount  int
	ColumnSnapshot  map[string]int // column_id → open task count
	AvgCycleSeconds *int64         // nil until any task completes
}
