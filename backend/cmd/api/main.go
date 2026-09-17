// Command api is the Fluxboard HTTP API. It boots a prod-shaped server: config,
// structured logging, DB + Redis clients, Prometheus metrics, the
// contract-ordered middleware chain, health probes, the auth surface, and
// graceful shutdown.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"

	"github.com/mesutokul/fluxboard/backend/internal/config"
	"github.com/mesutokul/fluxboard/backend/internal/domain/auth"
	"github.com/mesutokul/fluxboard/backend/internal/domain/project"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/casbinx"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/mailer"
	miniox "github.com/mesutokul/fluxboard/backend/internal/infrastructure/minio"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/oauthgoogle"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres"
	redisx "github.com/mesutokul/fluxboard/backend/internal/infrastructure/redis"
	stripex "github.com/mesutokul/fluxboard/backend/internal/infrastructure/stripe"
	httpx "github.com/mesutokul/fluxboard/backend/internal/interface/http"
	"github.com/mesutokul/fluxboard/backend/internal/interface/http/handlers"
	mw "github.com/mesutokul/fluxboard/backend/internal/interface/http/middleware"
	"github.com/mesutokul/fluxboard/backend/internal/interface/jobs"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/aesgcm"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/jwtx"
	"github.com/mesutokul/fluxboard/backend/internal/usecase/adminuc"
	"github.com/mesutokul/fluxboard/backend/internal/usecase/analyticsuc"
	"github.com/mesutokul/fluxboard/backend/internal/usecase/apikeyuc"
	"github.com/mesutokul/fluxboard/backend/internal/usecase/audituc"
	"github.com/mesutokul/fluxboard/backend/internal/usecase/authuc"
	"github.com/mesutokul/fluxboard/backend/internal/usecase/billinguc"
	"github.com/mesutokul/fluxboard/backend/internal/usecase/notifyuc"
	"github.com/mesutokul/fluxboard/backend/internal/usecase/automationuc"
	"github.com/mesutokul/fluxboard/backend/internal/usecase/projectuc"
	"github.com/mesutokul/fluxboard/backend/internal/usecase/taskuc"
	"github.com/mesutokul/fluxboard/backend/internal/usecase/tenantuc"
	"github.com/mesutokul/fluxboard/backend/internal/usecase/useruc"
)

// version is injected at build time via -ldflags "-X main.version=...".
var version = "dev"

const shutdownDrain = 20 * time.Second

// Login throttle budget (docs/04-AUTH.md §4, FR-AUTH-011).
const (
	loginRateLimit  = 10
	loginRateWindow = 15 * time.Minute
)

// Per-IP throttle budget for the unauthenticated auth endpoints (register,
// email-verify, password forgot/reset) — blunts enumeration + email-bombing
// without locking out legitimate retries. Keyed per endpoint tag + IP.
const (
	authThrottleLimit  = 20
	authThrottleWindow = 15 * time.Minute
)

// impersonationTTL bounds an admin impersonation token (docs/build/PHASE-6 §5,
// FR-ADM-003). Short-lived: the admin re-mints when it lapses.
const impersonationTTL = 15 * time.Minute

// @title           Fluxboard API
// @version         1.0
// @description     Multi-tenant project management + usage-based billing API. Bearer
// @description     auth accepts either a user session JWT or an org API key (fbk_live_…).
// @BasePath        /api/v1
// @securityDefinitions.apikey  BearerAuth
// @in              header
// @name            Authorization
func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("api exited with error", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger.Info("starting api", "version", version, "config", cfg.String())

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// Data clients. pgxpool connects lazily; a down DB does not block boot.
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	defer func() { _ = rdb.Close() }()

	// Metrics registry.
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	registerPoolStats(reg, pool)
	obs := newAuthObserver(reg, logger)
	httpMetrics := newHTTPMetrics(reg)
	sseGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "sse_connections_active", Help: "Current number of live SSE subscribers (docs/10 §5).",
	})
	reg.MustRegister(sseGauge)

	// Auth wiring: signer/verifier, repositories, session cache, usecase.
	signer, err := loadSigner(cfg, logger)
	if err != nil {
		return err
	}
	verifier := jwtx.VerifierFromSigners(signer)

	userRepo := postgres.NewUserRepo(pool)
	sessionRepo := postgres.NewSessionRepo(pool)
	tokenRepo := postgres.NewTokenRepo(pool)
	recoveryRepo := postgres.NewRecoveryRepo(pool)
	oauthRepo := postgres.NewOAuthRepo(pool)
	auditRepo := postgres.NewAuditRepo(pool)
	sessionCache := redisx.NewSessionCache(rdb)
	limiter := redisx.NewLoginRateLimiter(rdb, loginRateLimit, loginRateWindow)
	authThrottleLimiter := redisx.NewLoginRateLimiter(rdb, authThrottleLimit, authThrottleWindow)
	oauthStates := redisx.NewOAuthStateStore(rdb)

	mail := mailer.New(mailer.Config{
		Host: cfg.SMTPHost, Port: cfg.SMTPPort, Username: cfg.SMTPUser,
		Password: cfg.SMTPPassword, From: cfg.SMTPFrom, WebOrigin: cfg.WebOrigin,
	}, logger)

	cipher, err := loadTOTPCipher(cfg, logger)
	if err != nil {
		return err
	}
	google := loadGoogleProvider(cfg, logger)

	authSvc := authuc.New(authuc.Deps{
		Users:    userRepo,
		Sessions: sessionRepo,
		Tokens:   tokenRepo,
		Recovery: recoveryRepo,
		OAuth:    oauthRepo,
		Cache:    sessionCache,
		Limiter:  limiter,
		Mailer:   mail,
		Signer:   signer,
		Verifier: verifier,
		Cipher:   cipher,
		Google:   google,
		Observer: obs,
		Audit:    auditRepo,
	})
	var states auth.OAuthStateStore
	if google != nil {
		states = oauthStates
	}
	authHandlers := handlers.NewAuthHandlers(handlers.AuthConfig{
		Service:      authSvc,
		States:       states,
		Logger:       logger,
		WebOrigin:    cfg.WebOrigin,
		CookieSecure: cfg.IsProd(),
	})
	authenticator := &mw.Authenticator{
		Verifier: verifier,
		Cache:    sessionCache,
		Sessions: sessionRepo,
		Logger:   logger,
	}

	// Tenancy + RBAC wiring (docs/05-TENANCY-RBAC.md).
	tenantPool := postgres.NewTenantPool(pool)
	orgRepo := postgres.NewOrgRepo(pool)
	membershipRepo := postgres.NewMembershipRepo(tenantPool)
	invitationRepo := postgres.NewInvitationRepo(pool, tenantPool)
	membershipCache := redisx.NewMembershipCache(rdb)
	enforcer, err := casbinx.New()
	if err != nil {
		return err
	}

	// Phase 5 — realtime bus + notification fan-out (docs/09 §1/§3). eventBus is
	// the Redis-Stream SSE backbone; notifySvc writes in-app rows + email:send
	// outbox entries and publishes notification.created. The producer services
	// (task/project/tenant/billing) take the bus + notifier below so their writes
	// emit realtime events and fan-out notifications.
	eventBus := redisx.NewEventBus(rdb, logger, redisx.WithSubscriberGauge(sseGauge))
	notifySvc := notifyuc.New(notifyuc.Deps{
		Notifs: postgres.NewNotificationRepo(tenantPool),
		Prefs:  postgres.NewPrefRepo(tenantPool),
		Bus:    eventBus,
		Dir:    notifyDirectory{postgres.NewDirectoryRepo(tenantPool)},
		Outbox: postgres.NewOutboxRepo(tenantPool),
		Logger: logger,
	})

	tenantSvc := tenantuc.New(tenantuc.Deps{
		Orgs:     orgRepo,
		Members:  membershipRepo,
		Invites:  invitationRepo,
		Users:    userRepo,
		Cache:    membershipCache,
		Mailer:   mail,
		Audit:    auditRepo,
		Idem:     redisx.NewIdempotencyStore(rdb),
		Events:   eventBus,
		Notifier: notifySvc,
		Logger:   logger,
	})
	orgHandlers := handlers.NewOrgHandlers(tenantSvc, logger)
	tenantGuard := &mw.TenantGuard{
		Members: membershipRepo,
		Cache:   membershipCache,
		Authz:   enforcer,
		Logger:  logger,
	}

	// Phase 3a — projects/boards/tasks wiring. All repos are tenant-scoped [T]
	// over the RLS-enforcing TenantPool (docs/01 §PROJ/§TASK).
	projectRepo := postgres.NewProjectRepo(tenantPool)
	projectMemberRepo := postgres.NewProjectMemberRepo(tenantPool)
	boardRepo := postgres.NewBoardRepo(tenantPool)
	columnRepo := postgres.NewColumnRepo(tenantPool)
	taskRepo := postgres.NewTaskRepo(tenantPool)
	subtaskRepo := postgres.NewSubtaskRepo(tenantPool)
	labelRepo := postgres.NewLabelRepo(tenantPool)
	commentRepo := postgres.NewCommentRepo(tenantPool)
	activityRepo := postgres.NewActivityRepo(tenantPool)
	attachmentRepo := postgres.NewAttachmentRepo(tenantPool)
	automationRepo := postgres.NewAutomationRepo(tenantPool)
	objectStore := loadObjectStore(ctx, cfg, logger) // FR-TASK-006; nil disables attachments

	// Phase 4 — billing wiring (docs/06). Built before taskSvc so the storage
	// quota check can resolve the org's plan ceiling. The gateway selects stub vs
	// live from STRIPE_MODE; stub needs no keys (the webhook secret is the HMAC
	// key). Plan + processed-event tables are global (plain pool);
	// subscription/invoice/usage/outbox/webhook are [T] over the RLS-enforcing
	// TenantPool. BaseURL is the public app origin for Checkout/Portal returns.
	stripeGW, err := stripex.New(cfg.StripeMode, cfg.StripeWebhookSecret, cfg.WebOrigin)
	if err != nil {
		return err
	}
	entitlementCache := redisx.NewEntitlementCache(rdb)
	billingSvc := billinguc.New(billinguc.Deps{
		Plans:    postgres.NewPlanRepo(pool),
		Subs:     postgres.NewSubscriptionRepo(tenantPool),
		Invoices: postgres.NewInvoiceRepo(tenantPool),
		Events:   postgres.NewProcessedEventRepo(pool),
		Webhooks: postgres.NewWebhookRepo(tenantPool),
		Usage:    postgres.NewUsageRepo(tenantPool),
		Gateway:  stripeGW,
		Cache:    entitlementCache,
		Bus:      eventBus,
		Logger:   logger,
		BaseURL:  cfg.WebOrigin,
	})

	// Phase 6 — platform admin, org API keys, analytics, audit viewer
	// (docs/build/PHASE-6). Cross-org admin reads + API-key by-hash auth run on the
	// OWNER pool, which bypasses the non-FORCE RLS the app role is subject to; the
	// platform-admin guard (+ API-key scope guard) is the access control. When
	// DATABASE_URL_MIGRATE is unset there is no owner pool, so these surfaces stay
	// disabled (nil Deps ⇒ routes not mounted) rather than booting on the app role.
	var (
		apiKeyHandlers    *handlers.APIKeyHandlers
		auditHandlers     *handlers.AuditHandlers
		analyticsHandlers *handlers.AnalyticsHandlers
		adminHandlers     *handlers.AdminHandlers
		platformGuard     *mw.PlatformAdminGuard
		apiKeyResolver    mw.APIKeyResolver
		userSoleOwner     useruc.SoleOwnerReader // cross-org sole-owner reader (owner pool); nil ⇒ account deletion disabled
	)
	if cfg.DatabaseURLMigrate == "" {
		logger.Warn("DATABASE_URL_MIGRATE not set; platform-admin + API-key surfaces disabled")
	} else {
		ownerPool, err := pgxpool.New(ctx, cfg.DatabaseURLMigrate)
		if err != nil {
			return err
		}
		defer ownerPool.Close()

		asynqOpt := asynq.RedisClientOpt{Addr: cfg.RedisAddr}
		asynqClient := asynq.NewClient(asynqOpt)
		defer func() { _ = asynqClient.Close() }()
		inspector := asynq.NewInspector(asynqOpt)
		defer func() { _ = inspector.Close() }()

		analyticsRepo := postgres.NewAnalyticsRepo(tenantPool)
		apikeySvc := apikeyuc.New(apikeyuc.Deps{
			Keys:   postgres.NewAPIKeyRepo(tenantPool, ownerPool),
			Audit:  auditRepo,
			Logger: logger,
		})
		analyticsSvc := analyticsuc.New(analyticsuc.Deps{
			Stats: analyticsRepo,
			Usage: analyticsRepo,
			Subs:  postgres.NewSubscriptionRepo(tenantPool),
			Plans: postgres.NewPlanRepo(pool),
		})
		auditSvc := audituc.New(audituc.Deps{Reader: postgres.NewAuditReadRepo(pool)})
		adminSvc := adminuc.New(adminuc.Deps{
			Tenants:   postgres.NewAdminRepo(ownerPool),
			Flags:     postgres.NewFeatureFlagRepo(tenantPool),
			Overrides: postgres.NewOverrideRepo(tenantPool),
			Subs:      postgres.NewSubscriptionRepo(tenantPool),
			Invoices:  postgres.NewInvoiceRepo(tenantPool),
			Audit:     auditRepo,
			Minter:    impersonationMinter{signer: signer, ttl: impersonationTTL},
			Retrier:   webhookRetrier{client: asynqClient},
			Logger:    logger,
		})

		apiKeyHandlers = handlers.NewAPIKeyHandlers(apikeySvc, logger)
		auditHandlers = handlers.NewAuditHandlers(auditSvc, logger)
		analyticsHandlers = handlers.NewAnalyticsHandlers(analyticsSvc, logger)
		adminHandlers = handlers.NewAdminHandlers(adminSvc, auditSvc, jobsInspector{insp: inspector}, logger)
		platformGuard = &mw.PlatformAdminGuard{Users: userRepo, Logger: logger}
		apiKeyResolver = apikeySvc
		userSoleOwner = postgres.NewUserOwnerRepo(ownerPool) // account-delete sole-owner guard (cross-org)
	}

	// Account self-service (docs/08 §3). Avatars reuse the MinIO object store; the
	// sole-owner guard needs the owner pool (nil ⇒ DELETE /me returns 403).
	userSvc := useruc.New(useruc.Deps{
		Users:     userRepo,
		Avatars:   objectStore,
		Sessions:  sessionRepo,
		SoleOwner: userSoleOwner,
		Logger:    logger,
	})
	userHandlers := handlers.NewUserHandlers(userSvc, logger)

	automationSvc := automationuc.New(automationuc.Deps{
		Rules: automationRepo, Tasks: taskRepo, Labels: labelRepo,
		Columns: columnRepo, Boards: boardRepo, Logger: logger,
	})
	automationHandlers := handlers.NewAutomationHandlers(automationSvc, logger)
	projectSvc := projectuc.New(projectuc.Deps{
		Projects: projectRepo, Members: projectMemberRepo, Boards: boardRepo,
		Columns: columnRepo, Tasks: taskRepo, Labels: labelRepo,
		Subtasks: subtaskRepo, Comments: commentRepo,
		Automation: automationSvc,
		Events: eventBus, Logger: logger,
	})
	taskSvc := taskuc.New(taskuc.Deps{
		Tasks: taskRepo, Subtasks: subtaskRepo, Labels: labelRepo, Comments: commentRepo,
		Activity: activityRepo, Attachments: attachmentRepo, Projects: projectRepo,
		Members: projectMemberRepo, Boards: boardRepo, Columns: columnRepo,
		Store: objectStore, Entitlements: billingSvc,
		Events: eventBus, Notifier: notifySvc, Automation: automationSvc, Logger: logger,
	})
	projectHandlers := handlers.NewProjectHandlers(projectSvc, logger)
	taskHandlers := handlers.NewTaskHandlers(taskSvc, logger)
	billingHandlers := handlers.NewBillingHandlers(billingSvc, logger)
	webhookHandlers := handlers.NewWebhookHandlers(billingSvc, stripeGW, logger)
	eventHandlers := handlers.NewEventHandlers(notifySvc, logger)
	notificationHandlers := handlers.NewNotificationHandlers(notifySvc, logger)

	// EntitlementGuard gates resource-creating writes on the org's plan limits
	// (FR-BILL-009). The counts live in the project/tenant domains, so they enter
	// as closures over the concrete repos (§6.2).
	entitlementGuard := &mw.EntitlementGuard{
		Resolver:     billingSvc,
		ProjectCount: projectRepo.CountByOrg,
		MemberCount:  membershipRepo.CountMembers,
		Logger:       logger,
	}

	// Plan-tier rate limit + request-path usage counters (06 §5/§7, FR-BILL-009).
	rateLimiter := &mw.RateLimiter{
		Resolver: billingSvc,
		Counter:  redisx.NewUsageCounter(rdb),
		Logger:   logger,
	}

	router := httpx.NewRouter(httpx.Deps{
		Logger:        logger,
		WebOrigin:     cfg.WebOrigin,
		Health:        httpx.Health{DB: pool, Redis: redisPinger{rdb}},
		MetricsHTTP:   promhttp.HandlerFor(reg, promhttp.HandlerOpts{}),
		Auth:          authHandlers,
		User:          userHandlers,
		Orgs:          orgHandlers,
		Projects:      projectHandlers,
		Tasks:         taskHandlers,
		Automations:   automationHandlers,
		Billing:       billingHandlers,
		Webhooks:      webhookHandlers,
		Events:        eventHandlers,
		Notifications: notificationHandlers,
		APIKeys:       apiKeyHandlers,
		AuditView:     auditHandlers,
		Analytics:     analyticsHandlers,
		Admin:         adminHandlers,
		OpenAPI:       handlers.NewOpenAPIHandlers(),
		DevDocs:       !cfg.IsProd(),
		Authenticator: authenticator,
		Tenant:        tenantGuard,
		Entitlement:   entitlementGuard,
		AuthThrottle:  &mw.AuthThrottle{Limiter: authThrottleLimiter, Logger: logger},
		RateLimit:     rateLimiter,
		PlatformAdmin: platformGuard,
		APIKeyResolver: apiKeyResolver,
		HTTPMetrics:    httpMetrics,
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received, draining", "drain", shutdownDrain.String())
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownDrain)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return err
		}
		logger.Info("shutdown complete")
		return nil
	}
}

// loadSigner resolves the ES256 signing key: inline PEM, a file path, or (dev
// only) a freshly generated ephemeral key with a loud warning.
func loadSigner(cfg *config.Config, logger *slog.Logger) (*jwtx.Signer, error) {
	pemStr := cfg.JWTPrivateKeyPEM
	switch {
	case pemStr == "":
		logger.Warn("JWT_PRIVATE_KEY_PEM not set; generating an EPHEMERAL dev signing key (tokens will not survive restart)")
		gen, err := jwtx.GenerateES256PEM()
		if err != nil {
			return nil, err
		}
		pemStr = gen
	case !strings.Contains(pemStr, "BEGIN"):
		b, err := os.ReadFile(pemStr)
		if err != nil {
			return nil, fmt.Errorf("read jwt key file: %w", err)
		}
		pemStr = string(b)
	}
	priv, err := jwtx.LoadPrivateKeyPEM(pemStr)
	if err != nil {
		return nil, err
	}
	return jwtx.NewSigner(priv)
}

// loadTOTPCipher builds the AES-GCM cipher for TOTP secrets, or returns a nil
// interface (2FA disabled) when TOTP_ENC_KEY is unset. Returning an explicit nil
// interface — not a typed nil — matters: the usecase gates on `cipher == nil`.
func loadTOTPCipher(cfg *config.Config, logger *slog.Logger) (authuc.SecretCipher, error) {
	if cfg.TOTPEncKey == "" {
		logger.Warn("TOTP_ENC_KEY not set; 2FA endpoints are disabled")
		return nil, nil
	}
	key, err := aesgcm.ParseKey(cfg.TOTPEncKey)
	if err != nil {
		return nil, fmt.Errorf("totp enc key: %w", err)
	}
	c, err := aesgcm.New(key)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// loadGoogleProvider builds the Google OAuth provider, or a nil interface when
// client credentials are unset (Google login routes then 403).
func loadGoogleProvider(cfg *config.Config, logger *slog.Logger) auth.OAuthProvider {
	if cfg.GoogleClientID == "" || cfg.GoogleClientSecret == "" {
		logger.Warn("GOOGLE_CLIENT_ID/GOOGLE_CLIENT_SECRET not set; Google OAuth is disabled")
		return nil
	}
	return oauthgoogle.New(cfg.GoogleClientID, cfg.GoogleClientSecret, cfg.GoogleRedirectURL)
}

// loadObjectStore builds the MinIO object store, or a nil interface when
// MINIO_ENDPOINT is unset (attachment routes then 403). A dial/bucket error is
// logged and treated as disabled so the API still boots (FR-TASK-006).
func loadObjectStore(ctx context.Context, cfg *config.Config, logger *slog.Logger) project.ObjectStore {
	if cfg.MinIOEndpoint == "" {
		logger.Warn("MINIO_ENDPOINT not set; attachment endpoints are disabled")
		return nil
	}
	store, err := miniox.New(ctx, miniox.Config{
		Endpoint: cfg.MinIOEndpoint, AccessKey: cfg.MinIOAccessKey, SecretKey: cfg.MinIOSecretKey,
		Bucket: cfg.MinIOBucket, UseSSL: cfg.MinIOUseSSL,
	})
	if err != nil {
		logger.Error("minio init failed; attachments disabled", "err", err)
		return nil
	}
	return store
}

// notifyDirectory adapts postgres.DirectoryRepo (which returns a postgres-layer
// DTO) to notifyuc.Directory. Living in the composition root keeps the postgres
// package free of a usecase import (clean-arch layering).
type notifyDirectory struct{ repo *postgres.DirectoryRepo }

func (d notifyDirectory) ProjectMembers(ctx context.Context, orgID, projectID string) ([]notifyuc.UserRef, error) {
	us, err := d.repo.ProjectMembers(ctx, orgID, projectID)
	return toUserRefs(us), err
}

func (d notifyDirectory) UsersByID(ctx context.Context, orgID string, ids []string) ([]notifyuc.UserRef, error) {
	us, err := d.repo.UsersByID(ctx, orgID, ids)
	return toUserRefs(us), err
}

func toUserRefs(us []postgres.DirUser) []notifyuc.UserRef {
	if us == nil {
		return nil
	}
	out := make([]notifyuc.UserRef, 0, len(us))
	for _, u := range us {
		out = append(out, notifyuc.UserRef{ID: u.ID, Email: u.Email, Name: u.Name})
	}
	return out
}

// impersonationMinter adapts jwtx.Signer to adminuc.ImpersonationMinter — it mints
// a short-lived token carrying the `imp` claim (target org) under the admin's own
// session id, so session revocation still applies (docs/build/PHASE-6 §5).
type impersonationMinter struct {
	signer *jwtx.Signer
	ttl    time.Duration
}

func (m impersonationMinter) Mint(adminUserID, adminSID, targetOrg string) (string, time.Time, error) {
	now := time.Now().UTC()
	tok, err := m.signer.SignImpersonation(adminUserID, adminSID, targetOrg, now, m.ttl)
	if err != nil {
		return "", time.Time{}, err
	}
	return tok, now.Add(m.ttl), nil
}

// webhookRetrier adapts asynq.Client to adminuc.WebhookRetrier — it enqueues a
// webhook:retry task carrying the event id for the worker to replay.
type webhookRetrier struct{ client *asynq.Client }

func (r webhookRetrier) Enqueue(ctx context.Context, eventID string) error {
	payload, err := json.Marshal(jobs.WebhookRetryPayload{EventID: eventID})
	if err != nil {
		return err
	}
	_, err = r.client.EnqueueContext(ctx, asynq.NewTask(jobs.TypeWebhookRetry, payload))
	return err
}

// jobsInspector adapts asynq.Inspector to handlers.JobsInspector — it summarizes
// each queue's task counts for the admin jobs view (docs/build/PHASE-6 §5).
type jobsInspector struct{ insp *asynq.Inspector }

func (j jobsInspector) Summary(_ context.Context) (map[string]any, error) {
	names, err := j.insp.Queues()
	if err != nil {
		return nil, err
	}
	out := make(map[string]any, len(names))
	for _, n := range names {
		info, err := j.insp.GetQueueInfo(n)
		if err != nil {
			continue
		}
		out[n] = map[string]int{
			"size": info.Size, "pending": info.Pending, "active": info.Active,
			"scheduled": info.Scheduled, "retry": info.Retry, "archived": info.Archived,
			"completed": info.Completed, "processed": info.Processed, "failed": info.Failed,
		}
	}
	return out, nil
}

// redisPinger adapts *redis.Client to httpx.Pinger.
type redisPinger struct{ c *redis.Client }

func (p redisPinger) Ping(ctx context.Context) error { return p.c.Ping(ctx).Err() }

// authObserver implements authuc.Observer over Prometheus counters
// (docs/10-INFRA-DEVOPS.md §5).
type authObserver struct {
	login  *prometheus.CounterVec
	reuse  prometheus.Counter
	logger *slog.Logger
}

func newAuthObserver(reg prometheus.Registerer, logger *slog.Logger) *authObserver {
	login := prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "auth_login_total", Help: "Login attempts by result."},
		[]string{"result"},
	)
	reuse := prometheus.NewCounter(
		prometheus.CounterOpts{Name: "auth_refresh_reuse_total", Help: "Refresh-token reuse events (security signal)."},
	)
	reg.MustRegister(login, reuse)
	return &authObserver{login: login, reuse: reuse, logger: logger}
}

func (o *authObserver) LoginAttempt(_ context.Context, result string) {
	o.login.WithLabelValues(result).Inc()
}

func (o *authObserver) RefreshReuse(_ context.Context) {
	o.reuse.Inc()
	o.logger.Warn("refresh token reuse detected (family revoked)")
}

// httpMetrics implements mw.HTTPMetrics over a Prometheus histogram
// (docs/10-INFRA-DEVOPS.md §5). Labels are bounded: method, route pattern, status.
type httpMetrics struct{ dur *prometheus.HistogramVec }

func newHTTPMetrics(reg prometheus.Registerer) *httpMetrics {
	dur := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request duration by route, method, and status.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "route", "status"})
	reg.MustRegister(dur)
	return &httpMetrics{dur: dur}
}

func (h *httpMetrics) ObserveRequest(method, route string, status int, seconds float64) {
	h.dur.WithLabelValues(method, route, strconv.Itoa(status)).Observe(seconds)
}

// registerPoolStats exposes pgxpool connection counts as gauges.
func registerPoolStats(reg prometheus.Registerer, pool *pgxpool.Pool) {
	reg.MustRegister(prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{Name: "pgxpool_acquired_conns", Help: "Currently acquired connections."},
		func() float64 { return float64(pool.Stat().AcquiredConns()) },
	))
	reg.MustRegister(prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{Name: "pgxpool_idle_conns", Help: "Currently idle connections."},
		func() float64 { return float64(pool.Stat().IdleConns()) },
	))
	reg.MustRegister(prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{Name: "pgxpool_total_conns", Help: "Total connections in the pool."},
		func() float64 { return float64(pool.Stat().TotalConns()) },
	))
}
