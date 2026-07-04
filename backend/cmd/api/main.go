// Command api is the Fluxboard HTTP API. Phase 0 boots a prod-shaped server:
// config, structured logging, DB + Redis clients, Prometheus metrics, the
// contract-ordered middleware chain, health probes, and graceful shutdown —
// with no business routes yet.
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

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"

	"github.com/mesutokul/fluxboard/backend/internal/config"
	httpx "github.com/mesutokul/fluxboard/backend/internal/interface/http"
)

// version is injected at build time via -ldflags "-X main.version=...".
var version = "dev"

const shutdownDrain = 20 * time.Second

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

	// Root context cancelled on SIGTERM/SIGINT.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// Data clients. pgxpool connects lazily, so a down DB does not block boot;
	// /readyz reports it instead.
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	defer func() { _ = rdb.Close() }()

	// Metrics registry: go runtime + process + pgxpool pool stats.
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	registerPoolStats(reg, pool)

	router := httpx.NewRouter(httpx.Deps{
		Logger:    logger,
		WebOrigin: cfg.WebOrigin,
		Health: httpx.Health{
			DB:    pool,
			Redis: redisPinger{rdb},
		},
		MetricsHTTP: promhttp.HandlerFor(reg, promhttp.HandlerOpts{}),
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Serve until the signal context is cancelled.
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

// redisPinger adapts *redis.Client to httpx.Pinger.
type redisPinger struct{ c *redis.Client }

func (p redisPinger) Ping(ctx context.Context) error { return p.c.Ping(ctx).Err() }

// registerPoolStats exposes pgxpool connection counts as gauges. Cheap and
// answered on scrape, so it always reflects the live pool.
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
