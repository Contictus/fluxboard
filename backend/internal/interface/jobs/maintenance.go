// Package jobs holds Asynq task types, their handlers and the periodic schedule.
// Phase 3b ships two maintenance jobs (docs/01 §TASK): nightly Trash purge
// (FR-TASK-009) and attachment orphan GC (FR-TASK-006). Both fan out per-tenant.
package jobs

import (
	"context"
	"log/slog"

	"github.com/hibiken/asynq"

	"github.com/mesutokul/fluxboard/backend/internal/usecase/taskuc"
)

// Task type names.
const (
	TypeTrashPurge   = "maintenance:trash_purge"
	TypeAttachmentGC = "maintenance:attachment_gc"
)

// OrgLister enumerates active orgs so a job can run under each tenant.
type OrgLister interface {
	ListActiveOrgIDs(ctx context.Context) ([]string, error)
}

// Maintenance runs the periodic per-tenant housekeeping jobs.
type Maintenance struct {
	tasks  *taskuc.Service
	orgs   OrgLister
	logger *slog.Logger
}

// NewMaintenance builds the maintenance job handlers.
func NewMaintenance(tasks *taskuc.Service, orgs OrgLister, logger *slog.Logger) *Maintenance {
	if logger == nil {
		logger = slog.Default()
	}
	return &Maintenance{tasks: tasks, orgs: orgs, logger: logger}
}

// Register wires the handlers onto an Asynq mux.
func (m *Maintenance) Register(mux *asynq.ServeMux) {
	mux.HandleFunc(TypeTrashPurge, m.handleTrashPurge)
	mux.HandleFunc(TypeAttachmentGC, m.handleAttachmentGC)
}

// forEachOrg runs fn under every active tenant, logging and continuing past a
// single tenant's failure so one bad org never stalls the whole sweep (09 §2
// per-org error isolation). Shared by the maintenance and billing jobs.
func forEachOrg(ctx context.Context, orgs OrgLister, logger *slog.Logger, job string, fn func(orgID string) (int, error)) error {
	ids, err := orgs.ListActiveOrgIDs(ctx)
	if err != nil {
		return err
	}
	total := 0
	for _, orgID := range ids {
		n, err := fn(orgID)
		if err != nil {
			logger.Error("job: org failed", "job", job, "org", orgID, "err", err)
			continue
		}
		total += n
	}
	logger.Info("job complete", "job", job, "orgs", len(ids), "processed", total)
	return nil
}

func (m *Maintenance) forEachOrg(ctx context.Context, job string, fn func(orgID string) (int, error)) error {
	return forEachOrg(ctx, m.orgs, m.logger, job, fn)
}

// handleTrashPurge hard-deletes tasks past the Trash retention window (FR-TASK-009).
func (m *Maintenance) handleTrashPurge(ctx context.Context, _ *asynq.Task) error {
	return m.forEachOrg(ctx, TypeTrashPurge, func(orgID string) (int, error) {
		return m.tasks.PurgeExpiredTrash(ctx, orgID)
	})
}

// handleAttachmentGC removes orphaned pending uploads and their objects (FR-TASK-006).
func (m *Maintenance) handleAttachmentGC(ctx context.Context, _ *asynq.Task) error {
	return m.forEachOrg(ctx, TypeAttachmentGC, func(orgID string) (int, error) {
		return m.tasks.GCOrphanAttachments(ctx, orgID)
	})
}

// ScheduleEntry is one periodic-schedule registration (cron spec + task +
// enqueue options, e.g. the target queue per 09 §2 priorities).
type ScheduleEntry struct {
	Cron string
	Task *asynq.Task
	Opts []asynq.Option
}

// Schedule returns the periodic entries the worker registers with an
// asynq.Scheduler. Queues follow 09 §2: billing sync + outbox drain on
// critical, rollups/GC on low. Nightly jobs are offset so they do not contend.
func Schedule() []ScheduleEntry {
	return []ScheduleEntry{
		{Cron: "0 3 * * *", Task: asynq.NewTask(TypeTrashPurge, nil), Opts: []asynq.Option{asynq.Queue(QueueLow)}},
		{Cron: "30 3 * * *", Task: asynq.NewTask(TypeAttachmentGC, nil), Opts: []asynq.Option{asynq.Queue(QueueLow)}},
		{Cron: "@every 5s", Task: asynq.NewTask(TypeOutboxDrain, nil), Opts: []asynq.Option{asynq.Queue(QueueCritical)}},
		{Cron: "0 * * * *", Task: asynq.NewTask(TypeUsageAggregate, nil), Opts: []asynq.Option{asynq.Queue(QueueLow)}},
		{Cron: "0 2 * * *", Task: asynq.NewTask(TypeUsagePushStripe, nil), Opts: []asynq.Option{asynq.Queue(QueueLow)}},
		{Cron: "0 4 * * *", Task: asynq.NewTask(TypeBillingReconcile, nil), Opts: []asynq.Option{asynq.Queue(QueueCritical)}},
		{Cron: "15 3 * * *", Task: asynq.NewTask(TypeStatsRollup, nil), Opts: []asynq.Option{asynq.Queue(QueueLow)}},    // FR-AN-001; offset from purge/GC
		{Cron: "45 3 * * *", Task: asynq.NewTask(TypeAuditRetention, nil), Opts: []asynq.Option{asynq.Queue(QueueLow)}}, // FR-AUD-002; offset from rollup
		{Cron: "0 5 * * *", Task: asynq.NewTask(TypeAIRetention, nil), Opts: []asynq.Option{asynq.Queue(QueueLow)}},     // FR-AI-008; offset from audit retention
	}
}
