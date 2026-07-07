-- Organizations (global pool, no RLS) ---------------------------------------

-- name: CreateOrg :exec
INSERT INTO organizations (id, slug, name)
VALUES (@id, @slug, @name);

-- name: GetOrgByID :one
SELECT id, slug, name,
       coalesce(logo_key, '')::text AS logo_key,
       coalesce(stripe_customer_id, '')::text AS stripe_customer_id,
       deleted_at, purge_after, created_at, updated_at
FROM organizations
WHERE id = @id;

-- name: GetOrgBySlug :one
SELECT id, slug, name,
       coalesce(logo_key, '')::text AS logo_key,
       coalesce(stripe_customer_id, '')::text AS stripe_customer_id,
       deleted_at, purge_after, created_at, updated_at
FROM organizations
WHERE slug = @slug AND deleted_at IS NULL;

-- name: UpdateOrgProfile :exec
UPDATE organizations
SET name = @name,
    logo_key = coalesce(sqlc.narg('logo_key'), logo_key)
WHERE id = @id AND deleted_at IS NULL;

-- name: UpdateOrgSlug :exec
UPDATE organizations SET slug = @slug WHERE id = @id AND deleted_at IS NULL;

-- name: InsertSlugHistory :exec
INSERT INTO slug_history (old_slug, org_id, expires_at)
VALUES (@old_slug, @org_id, @expires_at)
ON CONFLICT (old_slug) DO UPDATE SET org_id = EXCLUDED.org_id, expires_at = EXCLUDED.expires_at;

-- name: GetSlugRedirect :one
-- Resolve a stale slug to its org within the 30-day 301 window (FR-TEN-007).
SELECT org_id FROM slug_history WHERE old_slug = @old_slug AND expires_at > now();

-- name: SoftDeleteOrg :exec
UPDATE organizations
SET deleted_at = now(), purge_after = @purge_after
WHERE id = @id AND deleted_at IS NULL;

-- name: RestoreOrg :exec
UPDATE organizations
SET deleted_at = NULL, purge_after = NULL
WHERE id = @id AND deleted_at IS NOT NULL;

-- ListUserOrgs and ResolveInvitationByToken call SECURITY DEFINER set-returning
-- functions (app_current_user_orgs / app_invitation_by_token). sqlc's parser
-- can't infer their RETURNS TABLE columns, so they are hand-written pgx in the
-- org / invitation repos rather than generated here.

-- Memberships ([T], tenant-scoped) ------------------------------------------

-- name: CreateMembership :exec
INSERT INTO memberships (org_id, user_id, role)
VALUES (@org_id, @user_id, @role);

-- name: GetMembership :one
SELECT org_id, user_id, role, created_at
FROM memberships
WHERE org_id = @org_id AND user_id = @user_id;

-- name: ListMembers :many
SELECT m.user_id, u.email, u.name,
       coalesce(u.avatar_key, '')::text AS avatar_key,
       m.role, m.created_at
FROM memberships m
JOIN users u ON u.id = m.user_id
WHERE m.org_id = @org_id
ORDER BY m.created_at;

-- name: ListMembersFiltered :many
-- Filtered + keyset-paginated member list (docs/08 §4 ?role=&q=). Optional role
-- and text (name/email) filters; the (created_at,user_id) cursor is exclusive.
SELECT m.user_id, u.email, u.name,
       coalesce(u.avatar_key, '')::text AS avatar_key,
       m.role, m.created_at
FROM memberships m
JOIN users u ON u.id = m.user_id
WHERE m.org_id = @org_id
  AND (sqlc.narg('role')::text IS NULL OR m.role = sqlc.narg('role'))
  AND (
        sqlc.narg('q')::text IS NULL
        OR u.name ILIKE '%' || sqlc.narg('q') || '%'
        OR u.email ILIKE '%' || sqlc.narg('q') || '%'
      )
  AND (
        sqlc.narg('after_created')::timestamptz IS NULL
        OR (m.created_at, m.user_id) > (sqlc.narg('after_created'), sqlc.narg('after_user')::uuid)
      )
ORDER BY m.created_at, m.user_id
LIMIT sqlc.arg('lim');

-- name: UpdateMemberRole :execrows
UPDATE memberships SET role = @role
WHERE org_id = @org_id AND user_id = @user_id;

-- name: DeleteMembership :execrows
DELETE FROM memberships WHERE org_id = @org_id AND user_id = @user_id;

-- name: CountMembersByRole :one
SELECT count(*) FROM memberships WHERE org_id = @org_id AND role = @role;

-- name: CountMembers :one
-- Total member (seat) count for the plan-limit gate (FR-BILL-009).
SELECT count(*) FROM memberships WHERE org_id = @org_id;

-- name: ListOrgOwnerEmails :many
-- OWNER addresses for billing notifications (dunning/cancel emails, 06 §4).
SELECT u.email
FROM memberships m
JOIN users u ON u.id = m.user_id
WHERE m.org_id = @org_id AND m.role = 'OWNER'
ORDER BY u.email;

-- Invitations ([T], tenant-scoped) ------------------------------------------

-- name: CreateInvitation :exec
INSERT INTO invitations (id, org_id, email, role, token_hash, invited_by, expires_at)
VALUES (@id, @org_id, @email, @role, @token_hash, @invited_by, @expires_at);

-- name: GetInvitation :one
SELECT id, org_id, email, role, token_hash, invited_by,
       expires_at, accepted_at, revoked_at, created_at
FROM invitations
WHERE org_id = @org_id AND id = @id;

-- name: GetInvitationByEmail :one
SELECT id, org_id, email, role, token_hash, invited_by,
       expires_at, accepted_at, revoked_at, created_at
FROM invitations
WHERE org_id = @org_id AND email = @email;

-- name: ListPendingInvitations :many
SELECT id, org_id, email, role, token_hash, invited_by,
       expires_at, accepted_at, revoked_at, created_at
FROM invitations
WHERE org_id = @org_id
  AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at > now()
ORDER BY created_at DESC;

-- name: RevokeInvitation :execrows
UPDATE invitations SET revoked_at = now()
WHERE org_id = @org_id AND id = @id AND accepted_at IS NULL AND revoked_at IS NULL;

-- name: UpdateInvitationToken :execrows
UPDATE invitations SET token_hash = @token_hash, expires_at = @expires_at
WHERE org_id = @org_id AND id = @id AND accepted_at IS NULL AND revoked_at IS NULL;

-- name: MarkInvitationAccepted :execrows
UPDATE invitations SET accepted_at = now()
WHERE org_id = @org_id AND id = @id AND accepted_at IS NULL AND revoked_at IS NULL;

-- name: ListActiveOrgIDs :many
-- All non-deleted org ids (organizations has no RLS). Drives per-tenant
-- maintenance jobs (trash purge, attachment GC) which then run under WithTenant.
SELECT id FROM organizations WHERE deleted_at IS NULL ORDER BY created_at;
