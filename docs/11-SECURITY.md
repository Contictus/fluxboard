# 11 — Security Model

Complements 04 (auth) and 05 (isolation). Framework: OWASP ASVS v4 level 2
as the target bar; STRIDE-lite threat model per trust boundary.

## 1. Trust Boundaries

```
B1: Browser ↔ API            (hostile client)
B2: Stripe ↔ webhook endpoint (unauthenticated network path, signature-gated)
B3: Tenant ↔ tenant           (inside one DB — RLS + app filtering)
B4: User ↔ platform admin     (privilege boundary, impersonation)
B5: API ↔ MinIO               (presigned URLs leak scope if mis-scoped)
B6: App ↔ background worker   (outbox payloads are internal but user-derived)
```

## 2. Threat → Control Matrix

| Threat | Boundary | Control | Verified by |
|---|---|---|---|
| Credential stuffing | B1 | Argon2id, login rate limit (email,IP), uniform timing, optional TOTP | 12 §4 |
| Session token theft (XSS) | B1 | access token memory-only; refresh httpOnly cookie; CSP (below); no tokens in localStorage | CSP header test, code lint |
| CSRF on refresh/logout | B1 | SameSite=Strict + custom header requirement + Origin allowlist | integration test |
| Refresh token replay | B1 | rotation + family revocation (04 §3) | replay test |
| User enumeration | B1 | identical responses on register/forgot/invite; uniform timing | response-diff test |
| IDOR / cross-tenant read | B3 | RLS fail-closed + app-level org filter + 404-not-403 for foreign IDs | 12 §6 suite |
| Privilege escalation via stale token | B1 | no role claims in JWT (ADR-008); session-state check per request | role-change propagation test |
| Forged webhook | B2 | signature verification, 5-min tolerance, no auth bypass side channel | tamper test |
| Webhook replay | B2 | processed_stripe_events unique insert (06 §4) | 5× concurrent replay test |
| SSRF | B1 | no user-supplied URL fetching anywhere in v1 (design-level elimination) | code review gate |
| SQL injection | B1 | sqlc parameterized only; zero string-built SQL (lint: forbid fmt.Sprintf into Exec) | lint + sqlmap smoke |
| Stored XSS via markdown | B1 | server renders nothing; client renders markdown with rehype-sanitize allowlist (no raw HTML) | XSS payload corpus test |
| Malicious file upload | B5 | MIME allowlist + size cap at presign; Content-Type pinned in presigned PUT; downloads `Content-Disposition: attachment`; bucket non-public | upload matrix test |
| Presigned URL overreach | B5 | object-key includes org_id; presign only after DB-level access check; 5-min GET TTL | cross-tenant download test |
| Admin abuse / insider | B4 | impersonation read-only + dual-identity audit (FR-ADM-003); admin_ro DB role; TOTP mandatory | audit assertions |
| Secrets in logs | all | redacting config String(); log-field allowlist; gitleaks in CI | CI |
| Dependency vulns | all | govulncheck + npm audit in CI, Dependabot | CI |
| DoS (cheap) | B1 | per-plan rate limits, body size limits (1 MB JSON, presign for files), pagination caps, statement_timeout=5s | k6 abuse profile |

## 3. HTTP Security Headers (set by API and Next.js middleware)

```
Content-Security-Policy: default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline';
  img-src 'self' data: https://<minio-host>; connect-src 'self' https://api.stripe.com;
  frame-src https://checkout.stripe.com; frame-ancestors 'none'
Strict-Transport-Security: max-age=63072000; includeSubDomains
X-Content-Type-Options: nosniff
Referrer-Policy: strict-origin-when-cross-origin
Permissions-Policy: camera=(), microphone=(), geolocation=()
```

## 4. Data Classification & Handling

| Class | Examples | Handling |
|---|---|---|
| Secret | password hashes, token hashes, TOTP secret | hashed or AES-GCM encrypted; never logged; never in API responses |
| Payment | — | does not exist in our systems (Stripe-hosted); only brand/last4 mirror |
| PII | email, name, IP (audit) | logged minimally; export/delete path documented (account deletion FR); audit IP retained per plan retention |
| Tenant content | tasks, comments, files | RLS-guarded; encrypted at rest = deployment concern (documented) |

## 5. Secure Development Practices (enforced, not aspirational)

- Every new endpoint PR must state: authn path, authz check (Casbin object/
  action + usecase fine gate), tenant scoping, rate-limit class — PR template
  checklist.
- `errmap.go` guarantees no internal error text reaches clients (500 body is
  generic + request_id).
- Security-relevant events (list in FR-AUD-001) MUST write audit entries —
  a usecase test asserts this per event type.
- Quarterly (or pre-"release") self-pentest checklist in
  `docs/runbooks/security-review.md`: authz matrix walk, OWASP Top 10 pass,
  dependency audit, secrets scan — results committed (portfolio artifact).
