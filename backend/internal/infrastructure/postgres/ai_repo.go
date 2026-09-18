package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/mesutokul/fluxboard/backend/internal/domain"
	"github.com/mesutokul/fluxboard/backend/internal/domain/ai"
)

// AIRepo is the Postgres-backed ai.RunRepository + ai.RiskRepository ([T],
// RLS). Hand-written pgx through TenantPool.WithTenantTx (audit_repo.go
// precedent): new tables, no sqlc queries, gen/ untouched. Every statement
// filters org_id (defense in depth under the RLS backstop, invariant #1).
type AIRepo struct {
	tp *TenantPool
}

// NewAIRepo builds an AIRepo. tp must be the tenant-scoped pool.
func NewAIRepo(tp *TenantPool) *AIRepo { return &AIRepo{tp: tp} }

// Compile-time port checks.
var (
	_ ai.RunRepository  = (*AIRepo)(nil)
	_ ai.RiskRepository = (*AIRepo)(nil)
)

const insertAIRunSQL = `
INSERT INTO ai_runs
  (id, org_id, user_id, kind, input, output, model,
   prompt_tokens, completion_tokens, status, idempotency_key)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`

// Create appends a run. Duplicate (org, idempotency_key) ⇒ ErrConflict.
func (r *AIRepo) Create(ctx context.Context, orgID string, run *ai.Run) error {
	id, err := parseUUID(run.ID)
	if err != nil {
		return fmt.Errorf("ai run create: %w", err)
	}
	uid, err := parseUUID(run.UserID)
	if err != nil {
		return fmt.Errorf("ai run create: user: %w", err)
	}
	out := []byte("{}")
	if len(run.Output) > 0 {
		out = []byte(run.Output)
	}
	status := run.Status
	if status == "" {
		status = "ok"
	}
	err = r.tp.WithTenantTx(ctx, orgID, func(tx pgx.Tx) error {
		oid, _ := parseUUID(orgID)
		_, err := tx.Exec(ctx, insertAIRunSQL,
			id, oid, uid, run.Kind, run.Input, out, run.Model,
			run.PromptTokens, run.CompletionTokens, status, ptrOrNil(run.IdempotencyKey),
		)
		return err
	})
	if isUnique(err) {
		return domain.ErrConflict
	}
	return err
}

const listAIRunsSQL = `
SELECT id::text, org_id::text, user_id::text, kind, input, output::text, model,
       prompt_tokens, completion_tokens, status, COALESCE(idempotency_key, ''),
       created_at
FROM ai_runs
WHERE org_id = $1
ORDER BY created_at DESC
LIMIT $2`

func scanAIRun(scan func(dest ...any) error) (*ai.Run, error) {
	var run ai.Run
	var out string
	var idem string
	if err := scan(
		&run.ID, &run.OrgID, &run.UserID, &run.Kind, &run.Input, &out, &run.Model,
		&run.PromptTokens, &run.CompletionTokens, &run.Status, &idem,
		&run.CreatedAt,
	); err != nil {
		return nil, err
	}
	run.Output = json.RawMessage(out)
	run.IdempotencyKey = idem
	return &run, nil
}

// ListRecent returns the newest runs, bounded by limit (chat history).
func (r *AIRepo) ListRecent(ctx context.Context, orgID string, limit int) ([]ai.Run, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	out := make([]ai.Run, 0)
	err := r.tp.WithTenantTx(ctx, orgID, func(tx pgx.Tx) error {
		oid, _ := parseUUID(orgID)
		rows, err := tx.Query(ctx, listAIRunsSQL, oid, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			run, err := scanAIRun(rows.Scan)
			if err != nil {
				return err
			}
			out = append(out, *run)
		}
		return rows.Err()
	})
	return out, err
}

const getAIRunSQL = `
SELECT id::text, org_id::text, user_id::text, kind, input, output::text, model,
       prompt_tokens, completion_tokens, status, COALESCE(idempotency_key, ''),
       created_at
FROM ai_runs
WHERE org_id = $1 AND id = $2`

// Get returns one run, or ErrNotFound (idempotency replay path).
func (r *AIRepo) Get(ctx context.Context, orgID, id string) (*ai.Run, error) {
	rid, err := parseUUID(id)
	if err != nil {
		return nil, fmt.Errorf("ai run get: %w", err)
	}
	var out *ai.Run
	err = r.tp.WithTenantTx(ctx, orgID, func(tx pgx.Tx) error {
		oid, _ := parseUUID(orgID)
		run, err := scanAIRun(tx.QueryRow(ctx, getAIRunSQL, oid, rid).Scan)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		out = run
		return nil
	})
	if err != nil {
		return nil, err
	}
	if out == nil {
		return nil, domain.ErrNotFound
	}
	return out, nil
}

const countAIRunsSQL = `
SELECT count(*) FROM ai_runs WHERE org_id = $1 AND created_at >= $2`

// CountSince counts runs of any kind after t (monthly metering, FR-AI-008).
func (r *AIRepo) CountSince(ctx context.Context, orgID string, t time.Time) (int64, error) {
	var n int64
	err := r.tp.WithTenantTx(ctx, orgID, func(tx pgx.Tx) error {
		oid, _ := parseUUID(orgID)
		return tx.QueryRow(ctx, countAIRunsSQL, oid, t).Scan(&n)
	})
	return n, err
}

const purgeAIRunsSQL = `DELETE FROM ai_runs WHERE org_id = $1 AND created_at < $2`

// PurgeBefore deletes runs older than t (30d retention, FR-AI-008).
func (r *AIRepo) PurgeBefore(ctx context.Context, orgID string, t time.Time) (int64, error) {
	var n int64
	err := r.tp.WithTenantTx(ctx, orgID, func(tx pgx.Tx) error {
		oid, _ := parseUUID(orgID)
		tag, err := tx.Exec(ctx, purgeAIRunsSQL, oid, t)
		if err != nil {
			return err
		}
		n = tag.RowsAffected()
		return nil
	})
	return n, err
}

const upsertAIRiskSQL = `
INSERT INTO ai_risks (id, org_id, project_id, task_id, score, signals)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (org_id, task_id) WHERE dismissed_at IS NULL
DO UPDATE SET score = EXCLUDED.score, signals = EXCLUDED.signals,
              updated_at = now()`

// UpsertOpen inserts or replaces the open risk for (org, task).
func (r *AIRepo) UpsertOpen(ctx context.Context, orgID string, risk *ai.Risk) error {
	id, err := parseUUID(risk.ID)
	if err != nil {
		return fmt.Errorf("ai risk upsert: %w", err)
	}
	pid, err := parseUUID(risk.ProjectID)
	if err != nil {
		return fmt.Errorf("ai risk upsert: project: %w", err)
	}
	tid, err := parseUUID(risk.TaskID)
	if err != nil {
		return fmt.Errorf("ai risk upsert: task: %w", err)
	}
	sig, err := json.Marshal(risk.Signals)
	if err != nil {
		return fmt.Errorf("ai risk upsert: signals: %w", err)
	}
	if sig == nil {
		sig = []byte("[]")
	}
	return r.tp.WithTenantTx(ctx, orgID, func(tx pgx.Tx) error {
		oid, _ := parseUUID(orgID)
		_, err := tx.Exec(ctx, upsertAIRiskSQL, id, oid, pid, tid, risk.Score, sig)
		return err
	})
}

const listOpenAIRisksSQL = `
SELECT id::text, org_id::text, project_id::text, task_id::text, score,
       signals::text, created_at, updated_at
FROM ai_risks
WHERE org_id = $1 AND project_id = $2 AND dismissed_at IS NULL
ORDER BY created_at DESC`

// ListOpen returns open risks for a project (risk board).
func (r *AIRepo) ListOpen(ctx context.Context, orgID, projectID string) ([]ai.Risk, error) {
	pid, err := parseUUID(projectID)
	if err != nil {
		return nil, fmt.Errorf("ai risk list: project: %w", err)
	}
	out := make([]ai.Risk, 0)
	err = r.tp.WithTenantTx(ctx, orgID, func(tx pgx.Tx) error {
		oid, _ := parseUUID(orgID)
		rows, err := tx.Query(ctx, listOpenAIRisksSQL, oid, pid)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var risk ai.Risk
			var sig string
			if err := rows.Scan(
				&risk.ID, &risk.OrgID, &risk.ProjectID, &risk.TaskID,
				&risk.Score, &sig, &risk.CreatedAt, &risk.UpdatedAt,
			); err != nil {
				return err
			}
			if err := json.Unmarshal([]byte(sig), &risk.Signals); err != nil {
				return fmt.Errorf("ai risk list: signals: %w", err)
			}
			out = append(out, risk)
		}
		return rows.Err()
	})
	return out, err
}

const dismissAIRiskSQL = `
UPDATE ai_risks SET dismissed_at = now(), updated_at = now()
WHERE org_id = $1 AND task_id = $2 AND dismissed_at IS NULL`

// Dismiss stamps dismissed_at. ErrNotFound if no open risk.
func (r *AIRepo) Dismiss(ctx context.Context, orgID, taskID string) error {
	tid, err := parseUUID(taskID)
	if err != nil {
		return fmt.Errorf("ai risk dismiss: task: %w", err)
	}
	return r.tp.WithTenantTx(ctx, orgID, func(tx pgx.Tx) error {
		oid, _ := parseUUID(orgID)
		tag, err := tx.Exec(ctx, dismissAIRiskSQL, oid, tid)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}
