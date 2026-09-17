package tenantuc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/auth"
	"github.com/mesutokul/fluxboard/backend/internal/domain/tenant"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/token"
)

var testNow = time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)

// ---- fakes ----------------------------------------------------------------

type fakeOrgs struct {
	orgs        map[string]*tenant.Organization
	members     *fakeMembers
	slugHistory map[string]string // old slug -> org id (301 window)
}

func (f *fakeOrgs) CreateWithOwner(_ context.Context, o *tenant.Organization, ownerID string) error {
	for _, e := range f.orgs {
		if e.Slug == o.Slug {
			return domain.ErrConflict
		}
	}
	cp := *o
	cp.CreatedAt, cp.UpdatedAt = testNow, testNow
	f.orgs[o.ID] = &cp
	f.members.add(o.ID, ownerID, tenant.RoleOwner)
	return nil
}

func (f *fakeOrgs) GetByID(_ context.Context, id string) (*tenant.Organization, error) {
	if o, ok := f.orgs[id]; ok {
		cp := *o
		return &cp, nil
	}
	return nil, domain.ErrNotFound
}

func (f *fakeOrgs) GetBySlug(_ context.Context, slug string) (*tenant.Organization, error) {
	for _, o := range f.orgs {
		if o.Slug == slug && o.DeletedAt == nil {
			cp := *o
			return &cp, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f *fakeOrgs) SlugRedirectTarget(_ context.Context, oldSlug string) (string, error) {
	if id, ok := f.slugHistory[oldSlug]; ok {
		return id, nil
	}
	return "", domain.ErrNotFound
}

func (f *fakeOrgs) ListForUser(_ context.Context, userID string) ([]tenant.OrgMembership, error) {
	var out []tenant.OrgMembership
	for orgID, users := range f.members.m {
		if m, ok := users[userID]; ok {
			o := f.orgs[orgID]
			if o == nil || o.DeletedAt != nil {
				continue
			}
			out = append(out, tenant.OrgMembership{OrgID: orgID, Role: m.Role, Slug: o.Slug, Name: o.Name})
		}
	}
	return out, nil
}

func (f *fakeOrgs) UpdateProfile(_ context.Context, id, name string, logoKey *string) error {
	o, ok := f.orgs[id]
	if !ok {
		return domain.ErrNotFound
	}
	o.Name = name
	if logoKey != nil {
		o.LogoKey = *logoKey
	}
	return nil
}

func (f *fakeOrgs) ClearLogo(_ context.Context, id string) error {
	o, ok := f.orgs[id]
	if !ok {
		return domain.ErrNotFound
	}
	o.LogoKey = ""
	return nil
}

func (f *fakeOrgs) UpdateSlug(_ context.Context, id, newSlug string, _ time.Time) error {
	for _, e := range f.orgs {
		if e.Slug == newSlug {
			return domain.ErrConflict
		}
	}
	o, ok := f.orgs[id]
	if !ok {
		return domain.ErrNotFound
	}
	if f.slugHistory == nil {
		f.slugHistory = map[string]string{}
	}
	f.slugHistory[o.Slug] = id // record old slug for 301 window
	o.Slug = newSlug
	return nil
}

func (f *fakeOrgs) SoftDelete(_ context.Context, id string, purgeAfter time.Time) error {
	o, ok := f.orgs[id]
	if !ok {
		return domain.ErrNotFound
	}
	o.DeletedAt = &testNow
	o.PurgeAfter = &purgeAfter
	return nil
}

func (f *fakeOrgs) Restore(_ context.Context, id string) error {
	o, ok := f.orgs[id]
	if !ok {
		return domain.ErrNotFound
	}
	o.DeletedAt, o.PurgeAfter = nil, nil
	return nil
}

type fakeMembers struct {
	m       map[string]map[string]*tenant.Membership
	invites *fakeInvites
}

func (f *fakeMembers) add(orgID, userID string, role tenant.OrgRole) {
	if f.m[orgID] == nil {
		f.m[orgID] = map[string]*tenant.Membership{}
	}
	f.m[orgID][userID] = &tenant.Membership{OrgID: orgID, UserID: userID, Role: role, CreatedAt: testNow}
}

func (f *fakeMembers) Get(_ context.Context, orgID, userID string) (*tenant.Membership, error) {
	if u := f.m[orgID]; u != nil {
		if m, ok := u[userID]; ok {
			cp := *m
			return &cp, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f *fakeMembers) List(_ context.Context, orgID string, q tenant.MemberQuery) ([]tenant.Member, error) {
	var out []tenant.Member
	for _, m := range f.m[orgID] {
		if q.Role != nil && m.Role != *q.Role {
			continue
		}
		out = append(out, tenant.Member{UserID: m.UserID, Role: m.Role, CreatedAt: m.CreatedAt})
	}
	return out, nil
}

func (f *fakeMembers) UpdateRole(_ context.Context, orgID, userID string, role tenant.OrgRole) error {
	if u := f.m[orgID]; u != nil {
		if m, ok := u[userID]; ok {
			m.Role = role
			return nil
		}
	}
	return domain.ErrNotFound
}

func (f *fakeMembers) Delete(_ context.Context, orgID, userID string) error {
	if u := f.m[orgID]; u != nil {
		if _, ok := u[userID]; ok {
			delete(u, userID)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (f *fakeMembers) CountByRole(_ context.Context, orgID string, role tenant.OrgRole) (int, error) {
	n := 0
	for _, m := range f.m[orgID] {
		if m.Role == role {
			n++
		}
	}
	return n, nil
}

func (f *fakeMembers) TransferOwnership(_ context.Context, orgID, fromUserID, toUserID string) error {
	u := f.m[orgID]
	if u == nil || u[toUserID] == nil {
		return domain.ErrNotFound
	}
	u[toUserID].Role = tenant.RoleOwner
	if u[fromUserID] != nil {
		u[fromUserID].Role = tenant.RoleAdmin
	}
	return nil
}

func (f *fakeMembers) AcceptInvitation(_ context.Context, orgID, invitationID, userID string, role tenant.OrgRole) error {
	inv := f.invites.byID[invitationID]
	if inv == nil || !inv.Pending(testNow) {
		return domain.ErrConflict
	}
	if u := f.m[orgID]; u != nil {
		if _, ok := u[userID]; ok {
			return domain.ErrConflict
		}
	}
	inv.AcceptedAt = &testNow
	f.add(orgID, userID, role)
	return nil
}

type fakeInvites struct {
	byID    map[string]*tenant.Invitation
	byToken map[string]*tenant.Invitation
}

func (f *fakeInvites) Create(_ context.Context, inv *tenant.Invitation) error {
	for _, e := range f.byID {
		if e.OrgID == inv.OrgID && e.Email == inv.Email {
			return domain.ErrConflict
		}
	}
	cp := *inv
	cp.CreatedAt = testNow
	f.byID[inv.ID] = &cp
	f.byToken[string(inv.TokenHash)] = &cp
	return nil
}

func (f *fakeInvites) Get(_ context.Context, orgID, id string) (*tenant.Invitation, error) {
	if inv, ok := f.byID[id]; ok && inv.OrgID == orgID {
		cp := *inv
		return &cp, nil
	}
	return nil, domain.ErrNotFound
}

func (f *fakeInvites) GetByEmail(_ context.Context, orgID, email string) (*tenant.Invitation, error) {
	for _, inv := range f.byID {
		if inv.OrgID == orgID && inv.Email == email {
			cp := *inv
			return &cp, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f *fakeInvites) ListPending(_ context.Context, orgID string) ([]tenant.Invitation, error) {
	var out []tenant.Invitation
	for _, inv := range f.byID {
		if inv.OrgID == orgID && inv.Pending(testNow) {
			out = append(out, *inv)
		}
	}
	return out, nil
}

func (f *fakeInvites) ResolveByToken(_ context.Context, tokenHash []byte) (*tenant.Invitation, error) {
	if inv, ok := f.byToken[string(tokenHash)]; ok {
		cp := *inv
		return &cp, nil
	}
	return nil, domain.ErrNotFound
}

func (f *fakeInvites) Revoke(_ context.Context, orgID, id string) error {
	if inv, ok := f.byID[id]; ok && inv.OrgID == orgID && inv.Pending(testNow) {
		inv.RevokedAt = &testNow
		return nil
	}
	return domain.ErrNotFound
}

func (f *fakeInvites) UpdateToken(_ context.Context, orgID, id string, tokenHash []byte, expiresAt time.Time) error {
	inv, ok := f.byID[id]
	if !ok || inv.OrgID != orgID {
		return domain.ErrNotFound
	}
	delete(f.byToken, string(inv.TokenHash))
	inv.TokenHash = tokenHash
	inv.ExpiresAt = expiresAt
	f.byToken[string(tokenHash)] = inv
	return nil
}

type fakeUsers struct{ byEmail map[string]*auth.User }

func (f *fakeUsers) Create(context.Context, *auth.User) error { return nil }
func (f *fakeUsers) GetByEmail(_ context.Context, email string) (*auth.User, error) {
	if u, ok := f.byEmail[email]; ok {
		return u, nil
	}
	return nil, domain.ErrNotFound
}
func (f *fakeUsers) GetByID(context.Context, string) (*auth.User, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeUsers) UpdatePasswordHash(context.Context, string, string) error { return nil }
func (f *fakeUsers) MarkEmailVerified(context.Context, string) error          { return nil }
func (f *fakeUsers) SetTOTP(context.Context, string, string, bool) error      { return nil }

type fakeCache struct{}

func (fakeCache) GetRole(context.Context, string, string) (tenant.OrgRole, bool, error) {
	return "", false, nil
}
func (fakeCache) SetRole(context.Context, string, string, tenant.OrgRole, time.Duration) error {
	return nil
}
func (fakeCache) Invalidate(context.Context, string, string) error { return nil }

type fakeMailer struct{ sent int }

func (f *fakeMailer) SendInvitation(context.Context, string, string, string) error {
	f.sent++
	return nil
}

// ---- harness --------------------------------------------------------------

type harness struct {
	svc     *Service
	orgs    *fakeOrgs
	members *fakeMembers
	invites *fakeInvites
	users   *fakeUsers
	mailer  *fakeMailer
}

func newHarness() *harness {
	invites := &fakeInvites{byID: map[string]*tenant.Invitation{}, byToken: map[string]*tenant.Invitation{}}
	members := &fakeMembers{m: map[string]map[string]*tenant.Membership{}, invites: invites}
	orgs := &fakeOrgs{orgs: map[string]*tenant.Organization{}, members: members}
	users := &fakeUsers{byEmail: map[string]*auth.User{}}
	mailer := &fakeMailer{}
	svc := New(Deps{
		Orgs: orgs, Members: members, Invites: invites, Users: users,
		Cache: fakeCache{}, Mailer: mailer, Now: func() time.Time { return testNow },
	})
	return &harness{svc: svc, orgs: orgs, members: members, invites: invites, users: users, mailer: mailer}
}

func ctx() context.Context { return context.Background() }

// ---- org tests ------------------------------------------------------------

func TestCreateOrg_OwnerMembership(t *testing.T) {
	h := newHarness()
	org, err := h.svc.CreateOrg(ctx(), "user-1", "Acme Inc", "acme")
	if err != nil {
		t.Fatal(err)
	}
	m, err := h.members.Get(ctx(), org.ID, "user-1")
	if err != nil || m.Role != tenant.RoleOwner {
		t.Fatalf("creator should be OWNER; got %v err=%v", m, err)
	}
}

func TestCreateOrg_BadSlug(t *testing.T) {
	h := newHarness()
	// Note: mixed case is normalized (lowercased), not rejected.
	for _, slug := range []string{"ab", "has space", "-lead", "trail-", "under_score", "toolongtoolongtoolongtoolongtoolongtoolong"} {
		if _, err := h.svc.CreateOrg(ctx(), "u", "Name", slug); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("slug %q: want validation error, got %v", slug, err)
		}
	}
}

func TestCreateOrg_DuplicateSlug(t *testing.T) {
	h := newHarness()
	if _, err := h.svc.CreateOrg(ctx(), "u1", "One", "dup"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.CreateOrg(ctx(), "u2", "Two", "dup"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("want conflict, got %v", err)
	}
}

// ---- last-owner invariant -------------------------------------------------

func TestLeave_LastOwnerBlocked(t *testing.T) {
	h := newHarness()
	org, _ := h.svc.CreateOrg(ctx(), "owner-1", "Org", "org1")
	if err := h.svc.Leave(ctx(), org.ID, "owner-1"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("last owner leaving must 409, got %v", err)
	}
}

func TestLeave_OwnerWithCoOwnerOK(t *testing.T) {
	h := newHarness()
	org, _ := h.svc.CreateOrg(ctx(), "owner-1", "Org", "org1")
	h.members.add(org.ID, "owner-2", tenant.RoleOwner)
	if err := h.svc.Leave(ctx(), org.ID, "owner-1"); err != nil {
		t.Fatalf("owner with co-owner may leave, got %v", err)
	}
}

func TestRemoveMember_LastOwnerBlocked(t *testing.T) {
	h := newHarness()
	org, _ := h.svc.CreateOrg(ctx(), "owner-1", "Org", "org1")
	err := h.svc.RemoveMember(ctx(), org.ID, tenant.RoleOwner, "owner-1")
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("removing sole owner must 409, got %v", err)
	}
}

func TestRemoveMember_AdminCannotRemoveOwner(t *testing.T) {
	h := newHarness()
	org, _ := h.svc.CreateOrg(ctx(), "owner-1", "Org", "org1")
	h.members.add(org.ID, "admin-1", tenant.RoleAdmin)
	err := h.svc.RemoveMember(ctx(), org.ID, tenant.RoleAdmin, "owner-1")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("admin removing owner must 403, got %v", err)
	}
}

// ---- role change guards ---------------------------------------------------

func TestChangeRole_CannotAssignOwner(t *testing.T) {
	h := newHarness()
	org, _ := h.svc.CreateOrg(ctx(), "owner-1", "Org", "org1")
	h.members.add(org.ID, "m-1", tenant.RoleMember)
	err := h.svc.ChangeMemberRole(ctx(), org.ID, tenant.RoleOwner, "m-1", tenant.RoleOwner)
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("assigning OWNER via role change must be validation error, got %v", err)
	}
}

func TestChangeRole_CannotChangeOwner(t *testing.T) {
	h := newHarness()
	org, _ := h.svc.CreateOrg(ctx(), "owner-1", "Org", "org1")
	err := h.svc.ChangeMemberRole(ctx(), org.ID, tenant.RoleOwner, "owner-1", tenant.RoleAdmin)
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("changing an owner's role must 409 (use transfer), got %v", err)
	}
}

func TestTransferOwnership(t *testing.T) {
	h := newHarness()
	org, _ := h.svc.CreateOrg(ctx(), "owner-1", "Org", "org1")
	h.members.add(org.ID, "m-1", tenant.RoleMember)
	if err := h.svc.TransferOwnership(ctx(), org.ID, "owner-1", "m-1"); err != nil {
		t.Fatal(err)
	}
	newOwner, _ := h.members.Get(ctx(), org.ID, "m-1")
	oldOwner, _ := h.members.Get(ctx(), org.ID, "owner-1")
	if newOwner.Role != tenant.RoleOwner || oldOwner.Role != tenant.RoleAdmin {
		t.Fatalf("transfer wrong: new=%s old=%s", newOwner.Role, oldOwner.Role)
	}
}

// ---- invitations ----------------------------------------------------------

func TestCreateInvitation_AlreadyMember(t *testing.T) {
	h := newHarness()
	org, _ := h.svc.CreateOrg(ctx(), "owner-1", "Org", "org1")
	h.users.byEmail["bob@x.com"] = &auth.User{ID: "bob", Email: "bob@x.com"}
	h.members.add(org.ID, "bob", tenant.RoleMember)
	_, err := h.svc.CreateInvitation(ctx(), org.ID, "owner-1", "bob@x.com", tenant.RoleMember, "")
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("inviting an existing member must 409, got %v", err)
	}
}

func TestCreateInvitation_AlreadyPending(t *testing.T) {
	h := newHarness()
	org, _ := h.svc.CreateOrg(ctx(), "owner-1", "Org", "org1")
	if _, err := h.svc.CreateInvitation(ctx(), org.ID, "owner-1", "new@x.com", tenant.RoleMember, ""); err != nil {
		t.Fatal(err)
	}
	_, err := h.svc.CreateInvitation(ctx(), org.ID, "owner-1", "new@x.com", tenant.RoleMember, "")
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("second pending invite must 409, got %v", err)
	}
}

func TestCreateInvitation_BadRole(t *testing.T) {
	h := newHarness()
	org, _ := h.svc.CreateOrg(ctx(), "owner-1", "Org", "org1")
	_, err := h.svc.CreateInvitation(ctx(), org.ID, "owner-1", "x@x.com", tenant.RoleOwner, "")
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("inviting as OWNER must be validation error, got %v", err)
	}
}

func TestAccept_SuccessAndReuse(t *testing.T) {
	h := newHarness()
	org, _ := h.svc.CreateOrg(ctx(), "owner-1", "Org", "org1")
	// Seed a pending invitation with a known raw token.
	raw := "raw-invite-token"
	inv := &tenant.Invitation{
		ID: "inv-1", OrgID: org.ID, Email: "carol@x.com", Role: tenant.RoleMember,
		TokenHash: token.Hash(raw), InvitedBy: "owner-1", ExpiresAt: testNow.Add(time.Hour),
	}
	if err := h.invites.Create(ctx(), inv); err != nil {
		t.Fatal(err)
	}
	out, err := h.svc.AcceptInvitation(ctx(), "carol", raw)
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if out.OrgID != org.ID || out.Role != tenant.RoleMember {
		t.Fatalf("accept outcome wrong: %+v", out)
	}
	m, err := h.members.Get(ctx(), org.ID, "carol")
	if err != nil || m.Role != tenant.RoleMember {
		t.Fatalf("membership not created: %v", err)
	}
	// Reuse of a consumed token must fail.
	if _, err := h.svc.AcceptInvitation(ctx(), "carol", raw); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("reusing accepted invite must 409, got %v", err)
	}
}

func TestAccept_Expired(t *testing.T) {
	h := newHarness()
	org, _ := h.svc.CreateOrg(ctx(), "owner-1", "Org", "org1")
	raw := "expired-token"
	inv := &tenant.Invitation{
		ID: "inv-2", OrgID: org.ID, Email: "d@x.com", Role: tenant.RoleGuest,
		TokenHash: token.Hash(raw), InvitedBy: "owner-1", ExpiresAt: testNow.Add(-time.Hour),
	}
	_ = h.invites.Create(ctx(), inv)
	if _, err := h.svc.AcceptInvitation(ctx(), "d", raw); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expired invite must 409, got %v", err)
	}
}

func TestAccept_UnknownToken(t *testing.T) {
	h := newHarness()
	if _, err := h.svc.AcceptInvitation(ctx(), "x", "nope"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown token must 404, got %v", err)
	}
}
