-- Usage metering — Phase 4 (docs/06-BILLING.md §5, FR-BILL-007) ----------------
-- usage_records [T]: hourly idempotent UPSERT of the per-(org, metric, day)
-- aggregate; daily job pushes Business-plan aggregates to Stripe with action=set.

-- name: UpsertUsage :exec
-- Idempotent set (not increment) so re-running the hourly aggregate is harmless.
INSERT INTO usage_records (org_id, metric, period_date, value)
VALUES (@org_id, @metric, @period_date, @value)
ON CONFLICT (org_id, metric, period_date) DO UPDATE SET
  value = EXCLUDED.value;

-- name: ListUsageForPush :many
-- Metered aggregates for a day not yet pushed to Stripe.
SELECT org_id, metric, period_date, value, pushed_at
FROM usage_records
WHERE org_id = @org_id AND period_date = @period_date AND pushed_at IS NULL;

-- name: MarkUsagePushed :exec
UPDATE usage_records
SET pushed_at = @pushed_at
WHERE org_id = @org_id AND metric = @metric AND period_date = @period_date;
