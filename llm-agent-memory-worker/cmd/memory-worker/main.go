package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/costa92/llm-agent-memory-worker/internal/config"
	"github.com/costa92/llm-agent-memory-worker/internal/service"
	pgmemory "github.com/costa92/llm-agent-memory-postgres/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if err := run(context.Background()); err != nil {
		slog.New(slog.NewTextHandler(os.Stderr, nil)).Error("memory worker failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	pool, err := pgxpool.New(ctx, cfg.PostgresURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	store, err := pgmemory.New(pool, pgmemory.Config{})
	if err != nil {
		return err
	}
	relay, err := pgmemory.NewRelay(store, service.NewConsolidationPublisher(store), pgmemory.RelayConfig{
		BatchSize:   cfg.RelayBatchSize,
		LeaseTTL:    cfg.RelayLeaseTTL,
		MaxAttempts: cfg.RelayMaxAttempts,
	})
	if err != nil {
		return err
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := relay.Release(releaseCtx); err != nil {
			logger.Error("memory worker relay release failed", "error", err)
		}
	}()

	logger.Info("starting memory worker")
	return runRelayLoop(ctx, logger, cfg.PollInterval, relay)
}

type relayRunner interface {
	RunOnce(ctx context.Context) (pgmemory.RunStats, error)
}

func runRelayLoop(parent context.Context, logger *slog.Logger, interval time.Duration, runner relayRunner) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		stats, err := runner.RunOnce(parent)
		if err != nil {
			return err
		}
		if stats.Published > 0 || stats.Failed > 0 || stats.LeaseLost > 0 {
			logger.Info("memory worker relay tick",
				"published", stats.Published,
				"failed", stats.Failed,
				"lease_lost", stats.LeaseLost,
			)
		}
		select {
		case <-parent.Done():
			return parent.Err()
		case <-ticker.C:
		}
	}
}
