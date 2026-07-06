package billing

import "testing"

var (
	freePlan = Plan{Code: PlanFree, MaxMembers: 5, MaxProjects: 3, MaxStorageBytes: 2 << 30, APIRatePerMin: 60, AuditRetentionDays: 7, Metered: false}
	proPlan  = Plan{Code: PlanPro, MaxMembers: 25, MaxProjects: 50, MaxStorageBytes: 50 << 30, APIRatePerMin: 300, AuditRetentionDays: 30, Metered: false}
	bizPlan  = Plan{Code: PlanBusiness, MaxMembers: -1, MaxProjects: -1, MaxStorageBytes: 500 << 30, APIRatePerMin: 1200, AuditRetentionDays: 365, Metered: true}
)

func TestPlanAndStatusValid(t *testing.T) {
	for _, c := range []PlanCode{PlanFree, PlanPro, PlanBusiness} {
		if !c.Valid() {
			t.Errorf("plan %q should be valid", c)
		}
	}
	if PlanCode("enterprise").Valid() {
		t.Error("unknown plan should be invalid")
	}
	for _, s := range []SubStatus{StatusNone, StatusTrialing, StatusActive, StatusPastDue, StatusUnpaid, StatusCanceled} {
		if !s.Valid() {
			t.Errorf("status %q should be valid", s)
		}
	}
	if SubStatus("paused").Valid() {
		t.Error("unknown status should be invalid")
	}
}

// TestDeriveEntitlements covers every (plan, status) pair: entitled statuses keep
// the subscribed plan's limits; the rest fall back to Free (docs/06 §2/§7).
func TestDeriveEntitlements(t *testing.T) {
	subscribed := proPlan
	cases := []struct {
		status   SubStatus
		wantPlan PlanCode
		wantMax  int // max_projects to assert the limits followed the plan
		wantWarn bool
	}{
		{StatusActive, PlanPro, 50, false},
		{StatusTrialing, PlanPro, 50, false},
		{StatusPastDue, PlanPro, 50, true}, // keeps plan, flags banner
		{StatusUnpaid, PlanFree, 3, false},
		{StatusCanceled, PlanFree, 3, false},
		{StatusNone, PlanFree, 3, false},
	}
	for _, c := range cases {
		e := DeriveEntitlements(subscribed, freePlan, c.status)
		if e.Plan != c.wantPlan {
			t.Errorf("status %s: plan = %s, want %s", c.status, e.Plan, c.wantPlan)
		}
		if e.MaxProjects != c.wantMax {
			t.Errorf("status %s: max_projects = %d, want %d", c.status, e.MaxProjects, c.wantMax)
		}
		if e.PastDueWarning != c.wantWarn {
			t.Errorf("status %s: warning = %v, want %v", c.status, e.PastDueWarning, c.wantWarn)
		}
		if e.Status != c.status {
			t.Errorf("status %s: echoed status = %s", c.status, e.Status)
		}
	}
}

func TestDeriveEntitlementsBusinessMetered(t *testing.T) {
	e := DeriveEntitlements(bizPlan, freePlan, StatusActive)
	if !e.Metered || e.MaxMembers != -1 {
		t.Errorf("business active: metered=%v maxMembers=%d, want true/-1", e.Metered, e.MaxMembers)
	}
	// Canceled business drops to Free: not metered, finite members.
	e = DeriveEntitlements(bizPlan, freePlan, StatusCanceled)
	if e.Metered || e.MaxMembers != 5 {
		t.Errorf("business canceled: metered=%v maxMembers=%d, want false/5", e.Metered, e.MaxMembers)
	}
}

func TestUnlimited(t *testing.T) {
	if !Unlimited(-1) || Unlimited(0) || Unlimited(5) {
		t.Error("Unlimited should be true only for negative caps")
	}
}

// TestCanTransition asserts the legal edges of the state machine (06 §2) and
// rejects representative illegal ones.
func TestCanTransition(t *testing.T) {
	legal := [][2]SubStatus{
		{StatusNone, StatusActive},
		{StatusNone, StatusTrialing},
		{StatusTrialing, StatusActive},
		{StatusActive, StatusPastDue},
		{StatusPastDue, StatusActive},
		{StatusPastDue, StatusUnpaid},
		{StatusActive, StatusCanceled},
		{StatusUnpaid, StatusCanceled},
		{StatusCanceled, StatusActive}, // resubscribe
		{StatusActive, StatusActive},   // idempotent self-edge
	}
	for _, e := range legal {
		if !CanTransition(e[0], e[1]) {
			t.Errorf("expected legal transition %s → %s", e[0], e[1])
		}
	}
	illegal := [][2]SubStatus{
		{StatusNone, StatusPastDue},
		{StatusNone, StatusCanceled},
		{StatusCanceled, StatusPastDue},
		{StatusUnpaid, StatusPastDue},
		{SubStatus("x"), StatusActive},
		{StatusActive, SubStatus("y")},
	}
	for _, e := range illegal {
		if CanTransition(e[0], e[1]) {
			t.Errorf("expected illegal transition %s → %s", e[0], e[1])
		}
	}
}
