package jobs

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/hibiken/asynq"

	"github.com/mesutokul/fluxboard/backend/internal/domain/notify"
	"github.com/mesutokul/fluxboard/backend/internal/usecase/notifyuc"
)

// ---- fakes -----------------------------------------------------------------

type fakePrefRepo struct{ stored map[string]notify.Pref }

func prefKey(userID string, cat notify.Category) string { return userID + "|" + string(cat) }
func (f *fakePrefRepo) GetForUser(context.Context, string, string) (map[notify.Category]notify.Pref, error) {
	return nil, nil
}
func (f *fakePrefRepo) Get(_ context.Context, _, userID string, cat notify.Category) (notify.Pref, bool, error) {
	p, ok := f.stored[prefKey(userID, cat)]
	return p, ok, nil
}
func (f *fakePrefRepo) Upsert(context.Context, string, notify.Pref) error { return nil }

type sentEmail struct{ to, subject, body string }

type fakeNotifMailer struct{ sent []sentEmail }

func (f *fakeNotifMailer) SendNotification(_ context.Context, to, subject, body string) error {
	f.sent = append(f.sent, sentEmail{to, subject, body})
	return nil
}

type fakeStats struct {
	ids      []string
	upserted []notify.ProjectStat
}

func (f *fakeStats) ProjectIDs(context.Context, string) ([]string, error) { return f.ids, nil }
func (f *fakeStats) ComputeDay(_ context.Context, orgID, projectID string, day time.Time) (notify.ProjectStat, error) {
	return notify.ProjectStat{OrgID: orgID, ProjectID: projectID, Day: day, CreatedCount: 1}, nil
}
func (f *fakeStats) Upsert(_ context.Context, _ string, s notify.ProjectStat) error {
	f.upserted = append(f.upserted, s)
	return nil
}

func notifTask(t *testing.T, p notifyuc.EmailPayload) *asynq.Task {
	t.Helper()
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	return asynq.NewTask(TypeEmailSend, b)
}

// ---- email:send (5.7.1 / 5.8.4 send-time recheck) --------------------------

func TestEmailSend_NotificationDeliveredWhenOptedIn(t *testing.T) {
	mailer := &fakeNotifMailer{}
	n := NewNotify(NotifyDeps{Prefs: &fakePrefRepo{stored: map[string]notify.Pref{}}, Mailer: mailer})
	err := n.HandleEmailSend(context.Background(), notifTask(t, notifyuc.EmailPayload{
		OrgID: "org1", UserID: "u1", Category: string(notify.CategoryMention),
		ToEmail: "u1@example.com", Subject: "You were mentioned", Body: "hi",
	}))
	if err != nil {
		t.Fatalf("HandleEmailSend: %v", err)
	}
	if len(mailer.sent) != 1 || mailer.sent[0].to != "u1@example.com" {
		t.Fatalf("sent = %+v; want one email to u1", mailer.sent)
	}
}

func TestEmailSend_SkippedWhenEmailOptedOut(t *testing.T) {
	mailer := &fakeNotifMailer{}
	prefs := &fakePrefRepo{stored: map[string]notify.Pref{
		prefKey("u1", notify.CategoryMention): {Category: notify.CategoryMention, Email: false, InApp: true},
	}}
	n := NewNotify(NotifyDeps{Prefs: prefs, Mailer: mailer})
	err := n.HandleEmailSend(context.Background(), notifTask(t, notifyuc.EmailPayload{
		OrgID: "org1", UserID: "u1", Category: string(notify.CategoryMention),
		ToEmail: "u1@example.com", Subject: "s", Body: "b",
	}))
	if err != nil {
		t.Fatalf("HandleEmailSend: %v", err)
	}
	if len(mailer.sent) != 0 {
		t.Fatalf("sent = %+v; want none (opted out at send-time)", mailer.sent)
	}
}

func TestEmailSend_BillingPayloadDelegates(t *testing.T) {
	mailer := &fakeNotifMailer{}
	delegated := false
	n := NewNotify(NotifyDeps{
		Prefs:  &fakePrefRepo{stored: map[string]notify.Pref{}},
		Mailer: mailer,
		BillingEmail: func(context.Context, *asynq.Task) error {
			delegated = true
			return nil
		},
	})
	// Billing payload: no user_id, carries a template.
	body, _ := json.Marshal(map[string]string{"template": "billing.dunning", "org_id": "org1"})
	if err := n.HandleEmailSend(context.Background(), asynq.NewTask(TypeEmailSend, body)); err != nil {
		t.Fatalf("HandleEmailSend: %v", err)
	}
	if !delegated {
		t.Fatal("billing-shaped payload should delegate to BillingEmail")
	}
	if len(mailer.sent) != 0 {
		t.Fatalf("notification mailer should not fire for a billing payload: %+v", mailer.sent)
	}
}

// ---- stats:rollup (5.7.3) --------------------------------------------------

func TestStatsRollup_UpsertsEachProject(t *testing.T) {
	stats := &fakeStats{ids: []string{"p1", "p2"}}
	n := NewNotify(NotifyDeps{Stats: stats, Orgs: &fakeOrgs{ids: []string{"org1"}}})
	if err := n.HandleStatsRollup(context.Background(), asynq.NewTask(TypeStatsRollup, nil)); err != nil {
		t.Fatalf("HandleStatsRollup: %v", err)
	}
	if len(stats.upserted) != 2 {
		t.Fatalf("upserts = %d; want 2 (one per project)", len(stats.upserted))
	}
	// Rollup targets yesterday's UTC grain.
	wantDay := utcDay(time.Now().Add(-24 * time.Hour))
	for _, s := range stats.upserted {
		if !s.Day.Equal(wantDay) {
			t.Fatalf("stat day = %v; want yesterday %v", s.Day, wantDay)
		}
	}
}
