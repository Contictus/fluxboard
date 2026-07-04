# 04 — Authentication Specification

Implements FR-AUTH-001…015. Standards references: RFC 6749 (OAuth2),
RFC 7636 (PKCE), RFC 6750 (Bearer), OWASP ASVS v4 §2/§3.

## 1. Credential Storage

Argon2id via `alexedwards/argon2id`:

```go
var Params = &argon2id.Params{
    Memory: 64 * 1024, Iterations: 3, Parallelism: 2,
    SaltLength: 16, KeyLength: 32,
}
// stored format: $argon2id$v=19$m=65536,t=3,p=2$<salt>$<hash>
```

Password checks on register/reset: length ≥ 12, ≤ 128; not in embedded
top-10k breach list (`embed.FS`, lowercase exact match); not equal to email
local part. Comparison via `argon2id.ComparePasswordAndHash` (constant-time).

## 2. Token Model

| Token | Form | TTL | Transport | Storage |
|---|---|---|---|---|
| Access | JWT ES256 | 15 min | `Authorization: Bearer` | client memory only |
| Refresh | 32B random, base64url | 7 d sliding | httpOnly Secure SameSite=Strict cookie, `Path=/api/v1/auth` | DB: SHA-256 hash |
| Email verify | 32B random | 24 h | email link | DB: SHA-256 hash, single-use |
| Password reset | 32B random | 1 h | email link | DB: SHA-256 hash, single-use |
| Pending-2FA | JWT, `scope=pending_2fa` | 5 min | JSON body | stateless |
| Invitation | 32B random | 7 d | email link | DB: SHA-256 hash |
| API key | `fbk_live_` + 32B | until revoked | Bearer | DB: SHA-256 hash |

JWT claims (FR-AUTH-004):

```json
{"iss":"fluxboard","aud":"fluxboard-api","sub":"<user_uuid>",
 "sid":"<session_uuid>","iat":1720000000,"exp":1720000900}
```

ES256 keypair loaded from env/PEM files; `kid` header present; key rotation =
publish new key, keep old for verification until max access TTL passes
(15 min) — a two-element in-process JWKS.

Access token validation also checks the session: `sid` looked up in Redis
(`sess:{sid}` → status, 15 min TTL, backfilled from PG on miss). Revoked
session ⇒ 401 even if signature/exp valid. This bounds revocation latency to
one cache round-trip, not token expiry.

## 3. Session & Refresh Rotation (FR-AUTH-005/006)

Table `sessions` (see 07 §2): each login creates a session row = one refresh
token **family** (`family_id`). Refresh flow, single serializable transaction:

```
1. hash = SHA256(presented_token); SELECT ... FOR UPDATE by token_hash
2. not found                    → 401
3. revoked_at != null           → 401
4. rotated_at != null           → REUSE DETECTED:
       UPDATE sessions SET revoked_at=now(), revoke_reason='token_reuse'
       WHERE family_id = $family;
       audit(severity=security, action=auth.refresh_reuse); → 401
5. expires_at < now()           → mark revoked → 401
6. otherwise: SET rotated_at=now();
   INSERT new row (same family_id, new token_hash, expires_at=now()+7d);
   issue new access JWT (same sid);
   Set-Cookie new refresh token
```

Concurrency note: two parallel refreshes with the same token — the second
hits `rotated_at != null` inside the row lock and triggers family revocation.
Frontend prevents the benign case with a **single-flight refresh mutex**
(03 §4): all concurrent 401-retries await one refresh promise.

## 4. Login Flows

**Password login:** rate-limit gate (FR-AUTH-011, Redis key
`rl:login:{sha1(email)}:{ip}`) → user lookup (unknown email: run Argon2id
against a static dummy hash — uniform timing) → verify → if TOTP enabled
return `{status:"2fa_required", pending_token}` → else create session,
return access token + set refresh cookie. Failures return identical
`invalid_credentials` regardless of cause.

**TOTP step (FR-AUTH-013):** `POST /auth/2fa/verify {pending_token, code}` —
validates JWT scope, checks TOTP (±1 window, 30s step) or a recovery-code
hash (consumed on use) → creates session.

**Google OAuth (FR-AUTH-009):** `GET /auth/oauth/google/start` generates
`state` (32B) + PKCE `code_verifier` (43–128 chars), stores
`oauth:{state} → {verifier, redirect_after}` in Redis (10 min), redirects to
Google with `code_challenge=S256`. Callback: state must exist (delete on
read), exchange code+verifier, require `email_verified=true` from Google,
then:

```
identity (provider,google_sub) exists → login that user
else email matches existing VERIFIED user → link identity, login
else email matches UNVERIFIED user      → 409 verify_first (no silent takeover)
else                                    → create user (email_verified=true), login
```

## 5. Endpoint Summary (details in 08-API-SPEC.md)

```
POST /auth/register            POST /auth/login            POST /auth/2fa/verify
POST /auth/refresh             POST /auth/logout           POST /auth/logout-all
GET  /auth/oauth/google/start  GET  /auth/oauth/google/callback
POST /auth/verify-email/request        POST /auth/verify-email/confirm
POST /auth/password/forgot     POST /auth/password/reset   POST /auth/password/change
GET  /auth/sessions            DELETE /auth/sessions/{id}
POST /auth/2fa/enroll          POST /auth/2fa/activate     DELETE /auth/2fa
```

`/auth/refresh` CSRF posture: SameSite=Strict cookie + required header
`X-Requested-With: fetch` (server rejects otherwise) + `Origin` allowlist.

## 6. Security Requirements Checklist (tested in 12 §4)

- [ ] Timing-uniform login for unknown vs wrong-password (dummy hash)
- [ ] No user enumeration on register (generic "check your email" if taken), forgot-password, invite
- [ ] All single-use tokens stored hashed; consumed transactionally (`UPDATE … WHERE used_at IS NULL RETURNING`)
- [ ] Password change/reset revokes sessions per FR-AUTH-010/012
- [ ] Refresh reuse revokes family + security audit entry
- [ ] Cookie flags: HttpOnly, Secure, SameSite=Strict, scoped Path
- [ ] `WWW-Authenticate: Bearer` on 401; no token material ever logged
- [ ] Argon2id params asserted by a unit test (regression guard against silent downgrade)
