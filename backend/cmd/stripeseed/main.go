// Command stripeseed idempotently populates the plans table (docs/06-BILLING.md
// §1, FR-BILL-002). It is the local-dev equivalent of `make stripe-seed`.
//
// MODE=stub (default): upsert the three static plan rows with null Stripe price
// IDs — no external calls. This is enough for entitlement derivation, quota
// enforcement, and the stub checkout flow.
//
// MODE=live (future): create Stripe products + licensed seat prices + metered
// prices, then upsert the returned price IDs. Left as a documented branch; the
// real stripe-go adapter (Phase 4 §4) fills it in.
//
// Re-running is safe: the upsert is keyed on plans.code (ON CONFLICT DO UPDATE),
// so limits stay in sync with this file on every run.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mesutokul/fluxboard/backend/internal/config"
)

// plan mirrors the frozen tier table (docs/build/PHASE-4-BILLING.md §4.0.3).
// Money-independent limits only; -1 = unlimited.
type plan struct {
	code               string
	name               string
	maxMembers         int
	maxProjects        int
	maxStorageBytes    int64
	apiRatePerMin      int
	auditRetentionDays int
	metered            bool
	monthlyPrice       int64 // recurring price in minor units (cents); backs admin MRR (FR-ADM-002)
}

var plans = []plan{
	{"free", "Free", 5, 3, 2 << 30, 60, 7, false, 0},
	{"pro", "Pro", 25, 50, 50 << 30, 300, 30, false, 1200},
	{"business", "Business", -1, -1, 500 << 30, 1200, 365, true, 4900},
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger); err != nil {
		logger.Error("stripeseed failed", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.StripeMode == "live" {
		return fmt.Errorf("stripeseed: MODE=live not implemented yet (stub only); wire the stripe-go adapter first")
	}

	ctx := context.Background()
	// Prefer the owner DSN (DDL/seed role) when set; plans has no RLS so the app
	// role also works, but the owner keeps seed writes off the runtime role.
	dsn := cfg.DatabaseURLMigrate
	if dsn == "" {
		dsn = cfg.DatabaseURL
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	const q = `
INSERT INTO plans (code, name, max_members, max_projects, max_storage_bytes,
                   api_rate_per_min, audit_retention_days, metered, monthly_price)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT (code) DO UPDATE SET
  name                 = EXCLUDED.name,
  max_members          = EXCLUDED.max_members,
  max_projects         = EXCLUDED.max_projects,
  max_storage_bytes    = EXCLUDED.max_storage_bytes,
  api_rate_per_min     = EXCLUDED.api_rate_per_min,
  audit_retention_days = EXCLUDED.audit_retention_days,
  metered              = EXCLUDED.metered,
  monthly_price        = EXCLUDED.monthly_price`

	for _, p := range plans {
		if _, err := pool.Exec(ctx, q, p.code, p.name, p.maxMembers, p.maxProjects,
			p.maxStorageBytes, p.apiRatePerMin, p.auditRetentionDays, p.metered, p.monthlyPrice); err != nil {
			return fmt.Errorf("seed plan %q: %w", p.code, err)
		}
		logger.Info("seeded plan", "code", p.code, "max_members", p.maxMembers,
			"max_projects", p.maxProjects, "metered", p.metered)
	}
	logger.Info("stripeseed complete (stub)", "plans", len(plans))
	return nil
}
