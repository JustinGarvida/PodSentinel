// Command agent runs the PodSentinel Go agent: it polls the
// Kubernetes Metrics API, writes to Postgres, publishes per-pod
// samples to RabbitMQ, and serves the REST API the dashboard reads
// from.
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

	"podsentinel/internal/api"
	"podsentinel/internal/config"
	"podsentinel/internal/ingest"
	"podsentinel/internal/k8s"
	"podsentinel/internal/logging"
	"podsentinel/internal/mq"
	"podsentinel/internal/store"
)

// Purpose: loads configuration and dependencies, starts polling and
// the HTTP server, and blocks until a shutdown signal.
// Params: none.
// Returns: nothing; exits the process via os.Exit(1) on startup or
// shutdown failure.
func main() {
	cfg := config.Load()
	logger := logging.New(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	clients, err := k8s.BuildClients()
	if err != nil {
		logger.Error("building kubernetes clients failed", "error", err)
		os.Exit(1)
	}

	st, err := store.Open(ctx, cfg.PostgresDSN)
	if err != nil {
		logger.Error("connecting to postgres failed", "error", err)
		os.Exit(1)
	}
	defer st.Close()

	// Left nil (not a typed-nil *mq.Publisher) when disabled so ingest's nil check works.
	var pub ingest.Publisher
	if cfg.RabbitMQURL == "" {
		logger.Warn("RABBITMQ_URL not set, publishing to rabbitmq disabled")
	} else {
		p := mq.NewPublisher(cfg.RabbitMQURL, logger)
		defer p.Close()
		pub = p
	}

	poller := k8s.NewPoller(clients, cfg.WatchNamespaces, logger)

	go runPollLoop(ctx, poller, st, pub, logger, cfg.PollInterval)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: api.NewRouter(logger, st, cfg.CORSOrigin),
	}

	go func() {
		logger.Info("starting server", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}

	logger.Info("shutdown complete")
}

// Purpose: runs one poll-persist-publish cycle immediately, then again on
// every tick of interval, until ctx is cancelled.
// Params:
//   - ctx: cancelled to stop the loop (e.g. on shutdown signal).
//   - poller: source of joined Kubernetes pod/metric samples.
//   - st: destination store for persisted samples.
//   - pub: destination for per-pod metrics.raw messages; nil disables
//     publishing.
//   - logger: structured logger for cycle-level errors.
//   - interval: time between poll cycles.
//
// Returns: nothing; blocks until ctx is done.
func runPollLoop(ctx context.Context, poller *k8s.Poller, st *store.Store, pub ingest.Publisher, logger *slog.Logger, interval time.Duration) {
	ingest.Run(ctx, poller, st, pub, logger)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ingest.Run(ctx, poller, st, pub, logger)
		}
	}
}
