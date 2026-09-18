package jobs

import (
	"context"
	"errors"
	"testing"

	"github.com/hibiken/asynq"
)

type stubPurger struct {
	calls []string
	fail  map[string]bool
}

func (s *stubPurger) PurgeExpired(_ context.Context, orgID string) (int64, error) {
	s.calls = append(s.calls, orgID)
	if s.fail[orgID] {
		return 0, errors.New("boom")
	}
	return 3, nil
}

type stubOrgs struct{ ids []string }

func (s *stubOrgs) ListActiveOrgIDs(_ context.Context) ([]string, error) {
	return s.ids, nil
}

func TestAIRetentionFansOut(t *testing.T) {
	p := &stubPurger{fail: map[string]bool{"bad": true}}
	j := NewAI(p, &stubOrgs{ids: []string{"a", "bad", "c"}}, nil)
	if err := j.handleRetention(context.Background(), asynq.NewTask(TypeAIRetention, nil)); err != nil {
		t.Fatal(err)
	}
	if len(p.calls) != 3 {
		t.Fatalf("calls = %v", p.calls)
	}
}

func TestAIRetentionSchedule(t *testing.T) {
	found := false
	for _, e := range Schedule() {
		if e.Task.Type() == TypeAIRetention {
			found = true
		}
	}
	if !found {
		t.Fatal("ai:retention missing from Schedule()")
	}
}
