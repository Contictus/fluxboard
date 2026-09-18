// Command worker runs the Asynq background-job processor. It shares internal/
// and config with the api but imports usecases only — never HTTP handlers
// (docs/03-ARCHITECTURE.md §1). Phase 0 boots the server and a /metrics
// endpoint with zero task handlers registered; handlers arrive in Phase 5.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"

	"github.com/mesutokul/fluxboard/backend/internal/config"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/mailer"
	miniox "github.com/mesutokul/fluxboard/backend/internal/infrastructure/minio"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres"
	redisx "github.com/mesutokul/fluxboard/backend/internal/infrastructure/redis"
	stripex "github.com/mesutokul/fluxboard/backend/internal/infrastructure/stripe"
	"github.com/mesutokul/fluxboard/backend/internal/interface/jobs"
	"github.com/mesutokul/fluxboard/backend/internal/usecase/aiuc"
	"github.com/mesutokul/fluxboard/backend/internal/usecase/billinguc"
	"github.com/mesutokul/fluxboard/backend/internal/usecase/taskuc"
)

var version = "dev"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("worker exited with error", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger.Info("starting worker", "version", version, "config", cfg.String())

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// Data clients + the task service the maintenance jobs run against.
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	tenantPool := postgres.NewTenantPool(pool)
	taskSvc := taskuc.New(taskuc.Deps{
		Tasks:       postgres.NewTaskRepo(tenantPool),
		Subtasks:    postgres.NewSubtaskRepo(tenantPool),
		Labels:      postgres.NewLabelRepo(tenantPool),
		Comments:    postgres.NewCommentRepo(tenantPool),
		Activity:    postgres.NewActivityRepo(tenantPool),
		Attachments: postgres.NewAttachmentRepo(tenantPool),
		Projects:    postgres.NewProjectRepo(tenantPool),
		Members:     postgres.NewProjectMemberRepo(tenantPool),
		Boards:      postgres.NewBoardRepo(tenantPool),
		Columns:     postgres.NewColumnRepo(tenantPool),
		Store:       loadObjectStore(ctx, cfg, logger),
		Logger:      logger,
	})
	maintenanceRepo := postgres.NewMaintenanceRepo(pool)
	maintenance := jobs.NewMaintenance(taskSvc, maintenanceRepo, logger)

	// Metrics endpoint on its own port (scraped separately from the api).
	metricsSrv, metricsReg := startMetrics(logger, cfg.WorkerMetricsAddr)

	redisOpt := asynq.RedisClientOpt{Addr: cfg.RedisAddr}
	srv := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: 10,
		Queues:      jobs.Queues(), // critical:6 default:3 low:1 (09 §2)
		Logger:      asynqLogger{logger},
	})

	// Billing jobs (Phase 4 §7): outbox drain, usage pipeline, reconciliation.
	stripeGW, err := stripex.New(cfg.StripeMode, cfg.StripeWebhookSecret, cfg.WebOrigin)
	if err != nil {
		return err
	}
	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	defer rdb.Close()
	asynqClient := asynq.NewClient(redisOpt)
	defer asynqClient.Close()
	driftCounter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "billing_reconciliation_drift_total",
		Help: "Subscriptions found drifted from Stripe by the nightly reconcile (06 §8).",
	})
	metricsReg.MustRegister(driftCounter)
	mail := mailer.New(mailer.Config{Host: cfg.SMTPHost, Port: cfg.SMTPPort, Username: cfg.SMTPUser, Password: cfg.SMTPPassword, From: cfg.SMTPFrom, WebOrigin: cfg.WebOrigin}, logger)
	membershipRepo := postgres.NewMembershipRepo(tenantPool)
	billingJobs := jobs.NewBilling(jobs.BillingDeps{
		Outbox:      postgres.NewOutboxRepo(tenantPool),
		Usage:       postgres.NewUsageRepo(tenantPool),
		Subs:        postgres.NewSubscriptionRepo(tenantPool),
		Plans:       postgres.NewPlanRepo(pool),
		Gateway:     stripeGW,
		Cache:       redisx.NewEntitlementCache(rdb),
		Counters:    redisx.NewUsageCounter(rdb),
		Mailer:      mail,
		OwnerEmails: membershipRepo.ListOwnerEmails,
		Client:      asynqClient,
		Drift:       driftCounter,
		Orgs:        maintenanceRepo,
		Logger:      logger,
	})

	// Phase 6 §6: webhook:retry. The admin webhook browser enqueues a retry by
	// event id; the handler reloads the stored Stripe event and re-runs the
	// idempotent webhook consumer (billinguc.Service). Built with the same billing
	// deps as the api so ProcessEvent's side effects and dedup are identical.
	processedEventRepo := postgres.NewProcessedEventRepo(pool)
	eventBus := redisx.NewEventBus(rdb, logger)
	billingSvc := billinguc.New(billinguc.Deps{
		Plans:    postgres.NewPlanRepo(pool),
		Subs:     postgres.NewSubscriptionRepo(tenantPool),
		Invoices: postgres.NewInvoiceRepo(tenantPool),
		Events:   processedEventRepo,
		Webhooks: postgres.NewWebhookRepo(tenantPool),
		Usage:    postgres.NewUsageRepo(tenantPool),
		Gateway:  stripeGW,
		Cache:    redisx.NewEntitlementCache(rdb),
		Bus:      eventBus,
		Logger:   logger,
		BaseURL:  cfg.WebOrigin,
	})
	webhookRetry := jobs.NewWebhookRetry(processedEventRepo, billingSvc, logger)

	// Phase 6 §7: audit:retention (FR-AUD-002). The DELETE runs on the OWNER pool —
	// audit_log DELETE is revoked from the app role (0005 append-only). Disabled
	// (logged no-op) when DATABASE_URL_MIGRATE is unset, so no widening of the app
	// role and no retry storm from the scheduled task.
	var auditPurger jobs.AuditPurger
	if cfg.DatabaseURLMigrate != "" {
		ownerPool, err := pgxpool.New(ctx, cfg.DatabaseURLMigrate)
		if err != nil {
			return err
		}
		defer ownerPool.Close()
		auditPurger = postgres.NewAuditRetentionRepo(ownerPool)
	} else {
		logger.Warn("DATABASE_URL_MIGRATE not set; audit:retention disabled")
	}
	auditRetention := jobs.NewAuditRetention(maintenanceRepo, billingSvc, auditPurger, logger)

	// Phase 5 §7: notification email delivery (send-time pref recheck) + the
	// nightly project-stats rollup. Notify owns the shared email:send handler and
	// delegates billing-shaped payloads to billingJobs.HandleEmailSend.
	notifyJobs := jobs.NewNotify(jobs.NotifyDeps{
		Prefs:        postgres.NewPrefRepo(tenantPool),
		Stats:        postgres.NewStatsRepo(tenantPool),
		Mailer:       mail,
		Orgs:         maintenanceRepo,
		BillingEmail: billingJobs.HandleEmailSend,
		Logger:       logger,
	})

	mux := asynq.NewServeMux()
	// Job counters by type + result (docs/10-INFRA-DEVOPS.md §5). Middleware wraps
	// every handler so counts are uniform across job kinds.
	jobCounter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "asynq_task_processed_total", Help: "Asynq tasks processed by type and status (docs/10 §5).",
	}, []string{"type", "status"})
	metricsReg.MustRegister(jobCounter)
	mux.Use(func(next asynq.Handler) asynq.Handler {
		return asynq.HandlerFunc(func(ctx context.Context, t *asynq.Task) error {
			err := next.ProcessTask(ctx, t)
			status := "success"
			if err != nil {
				status = "error"
			}
			jobCounter.WithLabelValues(t.Type(), status).Inc()
			return err
		})
	})
	maintenance.Register(mux)
	billingJobs.Register(mux)
	notifyJobs.Register(mux)
	webhookRetry.Register(mux)
	auditRetention.Register(mux)

	// AI retention (ADR-025, FR-AI-008). The sweep only needs the ledger, so
	// the service is built with Runs alone; gates/metering are API-path only.
	aiJobs := jobs.NewAI(aiuc.New(aiuc.Deps{
		Runs:   postgres.NewAIRepo(tenantPool),
		Logger: logger,
	}), maintenanceRepo, logger)
	aiJobs.Register(mux)

	if err := srv.Start(mux); err != nil {
		return err
	}
	logger.Info("worker running")

	// Scheduler enqueues the periodic maintenance tasks (docs/01 §TASK).
	scheduler := asynq.NewScheduler(redisOpt, &asynq.SchedulerOpts{Logger: asynqLogger{logger}})
	for _, e := range jobs.Schedule() {
		if _, err := scheduler.Register(e.Cron, e.Task, e.Opts...); err != nil {
			return err
		}
	}
	if err := scheduler.Start(); err != nil {
		return err
	}
	logger.Info("scheduler running", "jobs", len(jobs.Schedule()))

	<-ctx.Done()
	logger.Info("shutdown signal received")
	scheduler.Shutdown() // stop enqueuing periodic tasks
	srv.Shutdown()       // stops claiming new tasks, finishes in-flight

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := metricsSrv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	logger.Info("shutdown complete")
	return nil
}

// loadObjectStore builds the MinIO store, or a nil interface when MINIO_ENDPOINT
// is unset (attachment GC then no-ops). A dial error is logged, not fatal.
func loadObjectStore(ctx context.Context, cfg *config.Config, logger *slog.Logger) project.ObjectStore {
	if cfg.MinIOEndpoint == "" {
		logger.Warn("MINIO_ENDPOINT not set; attachment GC disabled")
		return nil
	}
	store, err := miniox.New(ctx, miniox.Config{
		Endpoint: cfg.MinIOEndpoint, AccessKey: cfg.MinIOAccessKey, SecretKey: cfg.MinIOSecretKey,
		Bucket: cfg.MinIOBucket, UseSSL: cfg.MinIOUseSSL,
	})
	if err != nil {
		logger.Error("minio init failed; attachment GC disabled", "err", err)
		return nil
	}
	return store
}

func startMetrics(logger *slog.Logger, addr string) (*http.Server, *prometheus.Registry) {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		logger.Info("worker metrics listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("metrics server error", "err", err)
		}
	}()
	return srv, reg
}

// asynqLogger adapts slog to asynq.Logger.
type asynqLogger struct{ l *slog.Logger }

func (a asynqLogger) Debug(args ...any) { a.l.Debug("asynq", "msg", args) }
func (a asynqLogger) Info(args ...any)  { a.l.Info("asynq", "msg", args) }
func (a asynqLogger) Warn(args ...any)  { a.l.Warn("asynq", "msg", args) }
func (a asynqLogger) Error(args ...any) { a.l.Error("asynq", "msg", args) }
func (a asynqLogger) Fatal(args ...any) { a.l.Error("asynq fatal", "msg", args) }
