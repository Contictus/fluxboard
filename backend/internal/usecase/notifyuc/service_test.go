package notifyuc

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain/notify"
)

// ---- fakes -----------------------------------------------------------------

type fakeNotifs struct{ batches [][]notify.Notification }

func (f *fakeNotifs) CreateBatch(_ context.Context, _ string, ns []notify.Notification) error {
	f.batches = append(f.batches, ns)
	return nil
}
func (f *fakeNotifs) List(context.Context, string, notify.ListFilter) ([]notify.Notification, error) {
	return nil, nil
}
func (f *fakeNotifs) UnreadCount(context.Context, string, string) (int, error) { return 0, nil }
func (f *fakeNotifs) MarkRead(context.Context, string, string, string, time.Time) error {
	return nil
}
func (f *fakeNotifs) MarkAllRead(context.Context, string, string, time.Time) (int, error) {
	return 0, nil
}

// created returns every notification row across all batches.
func (f *fakeNotifs) created() []notify.Notification {
	var out []notify.Notification
	for _, b := range f.batches {
		out = append(out, b...)
	}
	return out
}

// fakePrefs returns a per-(user,category) stored pref; absent ⇒ default applies.
type fakePrefs struct{ stored map[string]notify.Pref }

func (f *fakePrefs) key(userID string, cat notify.Category) string { return userID + "|" + string(cat) }
func (f *fakePrefs) GetForUser(context.Context, string, string) (map[notify.Category]notify.Pref, error) {
	return nil, nil
}
func (f *fakePrefs) Get(_ context.Context, _, userID string, cat notify.Category) (notify.Pref, bool, error) {
	p, ok := f.stored[f.key(userID, cat)]
	return p, ok, nil
}
func (f *fakePrefs) Upsert(context.Context, string, notify.Pref) error { return nil }

type fakeDir struct{ members []UserRef }

func (f *fakeDir) ProjectMembers(context.Context, string, string) ([]UserRef, error) {
	return f.members, nil
}
func (f *fakeDir) UsersByID(_ context.Context, _ string, ids []string) ([]UserRef, error) {
	var out []UserRef
	for _, m := range f.members {
		for _, id := range ids {
			if m.ID == id {
				out = append(out, m)
			}
		}
	}
	return out, nil
}

type fakeOutbox struct{ emails []EmailPayload }

func (f *fakeOutbox) InsertEmail(_ context.Context, _ string, payload json.RawMessage) error {
	var p EmailPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return err
	}
	f.emails = append(f.emails, p)
	return nil
}

type fakeBus struct {
	replay []notify.Event
	gap    bool
}

func (f *fakeBus) Publish(context.Context, string, notify.Event) (string, error) { return "1-0", nil }
func (f *fakeBus) Subscribe(context.Context, string) (<-chan notify.Event, func(), error) {
	ch := make(chan notify.Event)
	close(ch)
	return ch, func() {}, nil
}
func (f *fakeBus) Replay(context.Context, string, string) ([]notify.Event, bool, error) {
	return f.replay, f.gap, nil
}

// ---- StreamInit (5.8.2) ----------------------------------------------------

func TestStreamInit_EmptyLastEventID_NoBacklog(t *testing.T) {
	s := New(Deps{Bus: &fakeBus{replay: []notify.Event{{ID: "5-0", Name: "task.created"}}}})
	backlog, resync, err := s.StreamInit(context.Background(), "org1", "")
	if err != nil || resync || backlog != nil {
		t.Fatalf("fresh connect: got backlog=%v resync=%v err=%v; want nil/false/nil", backlog, resync, err)
	}
}

func TestStreamInit_ReplaysBacklog(t *testing.T) {
	want := []notify.Event{{ID: "6-0", Name: "task.moved"}}
	s := New(Deps{Bus: &fakeBus{replay: want}})
	backlog, resync, err := s.StreamInit(context.Background(), "org1", "5-0")
	if err != nil || resync {
		t.Fatalf("replay: resync=%v err=%v; want false/nil", resync, err)
	}
	if len(backlog) != 1 || backlog[0].ID != "6-0" {
		t.Fatalf("backlog = %+v; want the single replayed event", backlog)
	}
}

func TestStreamInit_GapEmitsResync(t *testing.T) {
	s := New(Deps{Bus: &fakeBus{gap: true}})
	backlog, resync, err := s.StreamInit(context.Background(), "org1", "1-0")
	if err != nil || !resync || backlog != nil {
		t.Fatalf("gap: got backlog=%v resync=%v err=%v; want nil/true/nil", backlog, resync, err)
	}
}

// ---- Fan-out @mention (5.8.4) ----------------------------------------------

func TestFanOutComment_MentionRowAndEmailForOptedInOnly(t *testing.T) {
	alice := UserRef{ID: "u-alice", Email: "alice@example.com", Name: "Alice"}
	bob := UserRef{ID: "u-bob", Email: "bob@example.com", Name: "Bob"}
	actor := UserRef{ID: "u-actor", Email: "actor@example.com", Name: "Actor"}

	notifs := &fakeNotifs{}
	outbox := &fakeOutbox{}
	prefs := &fakePrefs{stored: map[string]notify.Pref{
		// Bob (a comment target) opted out of email for comments; still wants in-app.
		(&fakePrefs{}).key(bob.ID, notify.CategoryComment): {
			OrgID: "org1", UserID: bob.ID, Category: notify.CategoryComment, Email: false, InApp: true,
		},
	}}
	s := New(Deps{
		Notifs: notifs,
		Prefs:  prefs,
		Dir:    &fakeDir{members: []UserRef{alice, bob, actor}},
		Outbox: outbox,
		Bus:    &fakeBus{},
	})

	// @alice is mentioned; bob is a comment target (task creator/assignee).
	s.FanOutComment(context.Background(), "org1", actor.ID, "proj1", "task1", "c1",
		"hey @alice please look, cc @actor", []string{bob.ID})

	rows := notifs.created()
	// Both alice (mention) and bob (comment) get in-app rows; the actor never
	// notifies themselves even when self-mentioned.
	if len(rows) != 2 {
		t.Fatalf("in-app rows = %d (%+v); want 2 (alice+bob, not actor)", len(rows), rows)
	}
	gotCat := map[string]notify.Category{}
	for _, r := range rows {
		if r.UserID == actor.ID {
			t.Fatalf("actor should never be notified: %+v", r)
		}
		gotCat[r.UserID] = r.Category
	}
	if gotCat[alice.ID] != notify.CategoryMention {
		t.Fatalf("alice category = %q; want mention", gotCat[alice.ID])
	}
	if gotCat[bob.ID] != notify.CategoryComment {
		t.Fatalf("bob category = %q; want comment", gotCat[bob.ID])
	}

	// Email: alice opted-in (default) → emailed; bob opted out of comment email → not.
	if len(outbox.emails) != 1 {
		t.Fatalf("email outbox = %d (%+v); want 1 (alice only)", len(outbox.emails), outbox.emails)
	}
	if outbox.emails[0].UserID != alice.ID || outbox.emails[0].OrgID != "org1" {
		t.Fatalf("email payload = %+v; want alice/org1", outbox.emails[0])
	}
}
