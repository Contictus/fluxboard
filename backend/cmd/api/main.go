// Command api is the Fluxboard HTTP API. It boots a prod-shaped server: config,
// structured logging, DB + Redis clients, Prometheus metrics, the
// contract-ordered middleware chain, health probes, the auth surface, and
// graceful shutdown.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"

	"github.com/mesutokul/fluxboard/backend/internal/config"
	"github.com/mesutokul/fluxboard/backend/internal/domain/auth"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/casbinx"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/mailer"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/oauthgoogle"
	"github.com/mesutokul/fluxboard/backend/internal/infrastructure/postgres"
	redisx "github.com/mesutokul/fluxboard/backend/internal/infrastructure/redis"
	httpx "github.com/mesutokul/fluxboard/backend/internal/interface/http"
	"github.com/mesutokul/fluxboard/backend/internal/interface/http/handlers"
	mw "github.com/mesutokul/fluxboard/backend/internal/interface/http/middleware"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/aesgcm"
	"github.com/mesutokul/fluxboard/backend/internal/pkg/jwtx"
	"github.com/mesutokul/fluxboard/backend/internal/usecase/authuc"
	"github.com/mesutokul/fluxboard/backend/internal/usecase/projectuc"
	"github.com/mesutokul/fluxboard/backend/internal/usecase/taskuc"
	"github.com/mesutokul/fluxboard/backend/internal/usecase/tenantuc"
)

// version is injected at build time via -ldflags "-X main.version=...".
var version = "dev"

const shutdownDrain = 20 * time.Second

// Login throttle budget (docs/04-AUTH.md §4, FR-AUTH-011).
const (
	loginRateLimit  = 10
	loginRateWindow = 15 * time.Minute
)

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
	tenantSvc := tenantuc.New(tenantuc.Deps{
		Orgs:    orgRepo,
		Members: membershipRepo,
		Invites: invitationRepo,
		Users:   userRepo,
		Cache:   membershipCache,
		Mailer:  mail,
		Audit:   auditRepo,
		Idem:    redisx.NewIdempotencyStore(rdb),
		Logger:  logger,
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

	projectSvc := projectuc.New(projectuc.Deps{
		Projects: projectRepo, Members: projectMemberRepo, Boards: boardRepo,
		Columns: columnRepo, Tasks: taskRepo, Logger: logger,
	})
	taskSvc := taskuc.New(taskuc.Deps{
		Tasks: taskRepo, Subtasks: subtaskRepo, Labels: labelRepo, Comments: commentRepo,
		Activity: activityRepo, Projects: projectRepo, Members: projectMemberRepo,
		Boards: boardRepo, Columns: columnRepo, Logger: logger,
	})
	projectHandlers := handlers.NewProjectHandlers(projectSvc, logger)
	taskHandlers := handlers.NewTaskHandlers(taskSvc, logger)

	router := httpx.NewRouter(httpx.Deps{
		Logger:        logger,
		WebOrigin:     cfg.WebOrigin,
		Health:        httpx.Health{DB: pool, Redis: redisPinger{rdb}},
		MetricsHTTP:   promhttp.HandlerFor(reg, promhttp.HandlerOpts{}),
		Auth:          authHandlers,
		Orgs:          orgHandlers,
		Projects:      projectHandlers,
		Tasks:         taskHandlers,
		Authenticator: authenticator,
		Tenant:        tenantGuard,
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
