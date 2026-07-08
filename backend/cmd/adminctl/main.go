// Command adminctl is the platform-operator CLI (docs/build/PHASE-6 §6). It runs
// on the OWNER pool (DATABASE_URL_MIGRATE) — the platform_role column is not
// writable by the app role — and deliberately has no hardcoded bootstrap user:
// an operator grants the first admin explicitly.
//
// Usage:
//
//	adminctl grant <email>    # idempotently set users.platform_role = 'admin'
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mesutokul/fluxboard/backend/internal/config"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "adminctl:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return usage()
	}
	switch args[0] {
	case "grant":
		if len(args) != 2 {
			return usage()
		}
		return grant(args[1])
	default:
		return usage()
	}
}

func usage() error {
	return fmt.Errorf("usage: adminctl grant <email>")
}

// grant sets platform_role='admin' for the user with the given email. Idempotent:
// re-granting an existing admin is a no-op. Runs on the owner pool because the app
// role cannot write platform_role (docs/11-SECURITY.md).
func grant(email string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	dsn := cfg.DatabaseURLMigrate
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL_MIGRATE is required (owner pool) but unset")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	tag, err := pool.Exec(ctx,
		`UPDATE users SET platform_role = 'admin' WHERE email = $1`, email)
	if err != nil {
		return fmt.Errorf("grant admin: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("no user with email %q", email)
	}
	fmt.Printf("granted platform_role=admin to %s\n", email)
	return nil
}
