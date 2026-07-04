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
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/mesutokul/fluxboard/backend/internal/config"
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

	// Metrics endpoint on its own port (scraped separately from the api).
	metricsSrv := startMetrics(logger, cfg.WorkerMetricsAddr)

	srv := asynq.NewServer(
		asynq.RedisClientOpt{Addr: cfg.RedisAddr},
		asynq.Config{
			Concurrency: 10,
			Logger:      asynqLogger{logger},
		},
	)

	// TODO(phase5): register task handlers (email, usage aggregation,
	// stats rollup, webhook retry, GC, org hard-delete, audit purge).
	mux := asynq.NewServeMux()

	if err := srv.Start(mux); err != nil {
		return err
	}
	logger.Info("worker running")

	<-ctx.Done()
	logger.Info("shutdown signal received")
	srv.Shutdown() // stops claiming new tasks, finishes in-flight

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := metricsSrv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	logger.Info("shutdown complete")
	return nil
}

func startMetrics(logger *slog.Logger, addr string) *http.Server {
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
	return srv
}

// asynqLogger adapts slog to asynq.Logger.
type asynqLogger struct{ l *slog.Logger }

func (a asynqLogger) Debug(args ...any) { a.l.Debug("asynq", "msg", args) }
func (a asynqLogger) Info(args ...any)  { a.l.Info("asynq", "msg", args) }
func (a asynqLogger) Warn(args ...any)  { a.l.Warn("asynq", "msg", args) }
func (a asynqLogger) Error(args ...any) { a.l.Error("asynq", "msg", args) }
func (a asynqLogger) Fatal(args ...any) { a.l.Error("asynq fatal", "msg", args) }
