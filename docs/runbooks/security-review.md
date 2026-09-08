# Security review runbook

Use this checklist before merging a security-sensitive change. Record the
scope, commands, findings, and unresolved risks in the pull request.

## Review sequence

1. Confirm every tenant-scoped query uses the tenant pool and that the handler
   applies the corresponding authorization check.
2. Check authentication, refresh-token rotation, cookie flags, and redirect
   validation for affected flows.
3. Run the backend build, vet, tests, architecture check, and vulnerability
   scan relevant to the change.
4. Exercise negative authorization cases with a second tenant and, when
   applicable, an impersonation token.
5. Check logs and metrics for token material, passwords, card data, or other
   sensitive values before approving the change.

## Evidence

Attach the exact command output or CI job links to the review. A passing unit
test does not prove PostgreSQL RLS isolation; that claim requires an integration
test using the `fluxboard_app` role. Track any accepted residual risk in the
pull request and link the related ADR or requirement ID.
