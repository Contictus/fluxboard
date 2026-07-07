package notify

import "testing"

func TestWantsChannel_DefaultOptIn(t *testing.T) {
	// No stored pref ⇒ both channels on for a center category.
	if !WantsChannel(nil, CategoryMention, ChannelEmail) {
		t.Fatal("nil pref email: want true")
	}
	if !WantsChannel(nil, CategoryMention, ChannelInApp) {
		t.Fatal("nil pref in_app: want true")
	}
}

func TestWantsChannel_OptOutHonored(t *testing.T) {
	p := &Pref{Category: CategoryComment, Email: false, InApp: true}
	if WantsChannel(p, CategoryComment, ChannelEmail) {
		t.Error("email opt-out: want false")
	}
	if !WantsChannel(p, CategoryComment, ChannelInApp) {
		t.Error("in_app still on: want true")
	}
}

func TestWantsChannel_TransactionalBypassesPrefs(t *testing.T) {
	// Even an explicit email opt-out cannot suppress a transactional auth email.
	p := &Pref{Category: CategoryAuthVerify, Email: false, InApp: false}
	if !WantsChannel(p, CategoryAuthVerify, ChannelEmail) {
		t.Error("auth_verify email must bypass prefs: want true")
	}
	if !WantsChannel(nil, CategoryAuthReset, ChannelEmail) {
		t.Error("auth_reset email with no pref: want true")
	}
}

func TestCategoryValid(t *testing.T) {
	for _, c := range CenterCategories() {
		if !c.Valid() {
			t.Errorf("%s: want valid center category", c)
		}
	}
	for _, c := range []Category{CategoryAuthVerify, CategoryAuthReset, Category("bogus")} {
		if c.Valid() {
			t.Errorf("%s: want invalid (not a center category)", c)
		}
	}
}

func TestCategoryTransactional(t *testing.T) {
	if !CategoryAuthVerify.Transactional() || !CategoryAuthReset.Transactional() {
		t.Error("auth categories must be transactional")
	}
	if CategoryMention.Transactional() || CategoryBilling.Transactional() {
		t.Error("center categories must not be transactional")
	}
}

func TestDefaultPref(t *testing.T) {
	p := DefaultPref("org1", "u1", CategoryTaskAssigned)
	if !p.Email || !p.InApp {
		t.Error("default pref must be opt-in on both channels")
	}
	if p.OrgID != "org1" || p.UserID != "u1" || p.Category != CategoryTaskAssigned {
		t.Error("default pref identity fields not set")
	}
}
