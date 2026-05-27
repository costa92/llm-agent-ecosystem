package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/costa92/llm-agent-memory-gateway/internal/authz"
	"github.com/costa92/llm-agent-memory-gateway/internal/config"
	"github.com/costa92/llm-agent-memory-gateway/internal/observability"
	"github.com/costa92/llm-agent-memory-gateway/internal/service"
	"github.com/costa92/llm-agent-memory-gateway/internal/transport"
	pgmemory "github.com/costa92/llm-agent-memory-postgres/postgres"
	corememory "github.com/costa92/llm-agent-memory/memory"
	ragembed "github.com/costa92/llm-agent-rag/embed"
	ragpg "github.com/costa92/llm-agent-rag/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if err := run(context.Background()); err != nil {
		slog.New(slog.NewTextHandler(os.Stderr, nil)).Error("memory gateway failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	handler, cleanup, err := buildHandler(ctx, logger, cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	server := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: handler,
	}

	logger.Info("starting memory gateway", "addr", cfg.ListenAddr, "read_only", cfg.ReadOnly)
	return server.ListenAndServe()
}

func buildHandler(ctx context.Context, logger *slog.Logger, cfg config.Config) (http.Handler, func(), error) {
	pool, err := pgxpool.New(ctx, cfg.PostgresURL)
	if err != nil {
		return nil, nil, err
	}
	cleanupFns := []func(){pool.Close}

	if err := runMigrations(ctx, service.NewPostgresGatewayMigrator(pool)); err != nil {
		pool.Close()
		return nil, nil, err
	}

	store, err := pgmemory.New(pool, pgmemory.Config{})
	if err != nil {
		pool.Close()
		return nil, nil, err
	}

	metrics := observability.NewMetrics()

	vectorSource, vectorCleanup, err := buildVectorCandidateSource(ctx, cfg, metrics)
	if err != nil {
		pool.Close()
		return nil, nil, err
	}
	if vectorCleanup != nil {
		cleanupFns = append(cleanupFns, vectorCleanup)
	}
	svc, err := service.New(
		store,
		buildRecallBackend(pool, store, cfg, vectorSource),
		noOpSessionCloser{},
		observability.ComposeTraceEmitters(
			slogTraceEmitter{logger: logger},
			metrics.TraceEmitter(),
		),
		service.Config{
			ReadOnly:            cfg.ReadOnly,
			SessionStateStore:   service.NewPostgresSessionStateStore(pool),
			ScopeVersionStore:   service.NewPostgresScopeVersionStore(pool),
			SessionIdleTTL:      cfg.SessionIdleTTL,
			RecallObserver:      metrics.RecallObserver(),
			RecallCacheObserver: metrics.RecallCacheObserver(),
		},
	)
	if err != nil {
		for i := len(cleanupFns) - 1; i >= 0; i-- {
			cleanupFns[i]()
		}
		return nil, nil, err
	}

	if cfg.VectorEnabled {
		projector := buildVectorProjector(cfg, vectorSource, metrics)
		if projector != nil {
			relayObserver := slogOutboxProjectionObserver{logger: logger}
			relay, err := pgmemory.NewRelay(store, service.NewOutboxVectorPublisher(
				store,
				projector,
				multiOutboxObserver{
					observers: []service.OutboxProjectionObserver{
						relayObserver,
						metrics.OutboxObserver(),
					},
				},
			), cfg.OutboxBatchSize)
			if err != nil {
				for i := len(cleanupFns) - 1; i >= 0; i-- {
					cleanupFns[i]()
				}
				return nil, nil, err
			}
			cleanupFns = append(cleanupFns, startOutboxRelayWorker(ctx, logger, cfg.OutboxPollInterval, relay))
		}
	}

	return transport.NewHandler(svc, func(mux *http.ServeMux) {
			mux.Handle("GET /metrics", metrics.Handler())
		}), func() {
			for i := len(cleanupFns) - 1; i >= 0; i-- {
				cleanupFns[i]()
			}
		}, nil
}

func buildRecallBackend(pool *pgxpool.Pool, store corememory.RecordStore, cfg config.Config, vector service.RecallCandidateSource) service.RecallBackend {
	hydrator := service.NewPostgresRecordHydrator(store)
	lexical := service.NewPostgresLexicalCandidateSource(pool)

	switch cfg.RecallMode {
	case "hybrid":
		return service.NewHybridRecaller(
			hydrator,
			lexical,
			vector,
		)
	default:
		return service.NewHybridRecaller(
			hydrator,
			lexical,
		)
	}
}

func buildVectorCandidateSource(ctx context.Context, cfg config.Config, metrics *observability.Metrics) (service.RecallCandidateSource, func(), error) {
	if !cfg.VectorEnabled {
		return service.NewNullVectorCandidateSource(), nil, nil
	}

	poolCfg, err := pgxpool.ParseConfig(cfg.PostgresURL)
	if err != nil {
		return nil, nil, err
	}
	poolCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		return ragpg.RegisterTypes(ctx, conn)
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, nil, err
	}

	vectorStore, err := ragpg.New(pool, ragpg.Config{
		Table:       cfg.VectorTable,
		Dimension:   cfg.VectorDimension,
		VectorIndex: parseVectorIndex(cfg.VectorIndex),
	})
	if err != nil {
		pool.Close()
		return nil, nil, err
	}
	if err := vectorStore.Migrate(ctx); err != nil {
		pool.Close()
		return nil, nil, err
	}

	source := service.NewRAGStoreVectorCandidateSource(
		ragembed.NewHashEmbedder(cfg.VectorDimension),
		vectorStore,
		cfg.VectorNamespace,
	)
	if metrics != nil {
		source.SetEmbeddingMetrics(metrics, cfg.EmbeddingCostMicrosPerToken)
	}
	return source, pool.Close, nil
}

func buildVectorProjector(cfg config.Config, source service.RecallCandidateSource, metrics *observability.Metrics) service.VectorProjector {
	if !cfg.VectorEnabled {
		return nil
	}
	ragSource, ok := source.(*service.RAGStoreVectorCandidateSource)
	if !ok {
		return nil
	}
	projector := service.NewRAGVectorProjector(
		ragembed.NewHashEmbedder(cfg.VectorDimension),
		ragSource.Store(),
		cfg.VectorNamespace,
	)
	if metrics != nil {
		projector.SetEmbeddingMetrics(metrics, cfg.EmbeddingCostMicrosPerToken)
	}
	return projector
}

func parseVectorIndex(index string) ragpg.VectorIndex {
	switch index {
	case "ivfflat":
		return ragpg.VectorIndexIVFFlat
	case "hnsw":
		return ragpg.VectorIndexHNSW
	default:
		return ragpg.VectorIndexNone
	}
}

func runMigrations(ctx context.Context, migrator service.Migrator) error {
	if migrator == nil {
		return nil
	}
	return migrator.Migrate(ctx)
}

type noOpSessionCloser struct{}

func (noOpSessionCloser) CloseSession(context.Context, authz.Scope, string) error {
	return nil
}

type slogTraceEmitter struct {
	logger *slog.Logger
}

func (e slogTraceEmitter) Emit(_ context.Context, stage string, fields map[string]any) {
	if e.logger == nil {
		return
	}

	args := make([]any, 0, len(fields)*2+2)
	args = append(args, "stage", stage)
	for key, value := range fields {
		args = append(args, key, value)
	}
	e.logger.Info("memory gateway trace", args...)
}

type relayRunner interface {
	RunOnce(ctx context.Context) (pgmemory.RunStats, error)
}

func startOutboxRelayWorker(parent context.Context, logger *slog.Logger, interval time.Duration, runner relayRunner) func() {
	if runner == nil {
		return func() {}
	}

	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	var once sync.Once

	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			stats, err := runner.RunOnce(ctx)
			if err != nil && logger != nil && !errors.Is(err, context.Canceled) {
				logger.Error("memory gateway outbox relay failed", "error", err)
			}
			if logger != nil && (stats.Published > 0 || stats.Failed > 0) {
				logger.Info("memory gateway outbox relay tick", "published", stats.Published, "failed", stats.Failed)
			}

			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	return func() {
		once.Do(func() {
			cancel()
			<-done
		})
	}
}

type slogOutboxProjectionObserver struct {
	logger *slog.Logger
}

func (o slogOutboxProjectionObserver) ObserveProjection(_ context.Context, obs service.OutboxProjectionObservation) {
	if o.logger == nil {
		return
	}
	o.logger.Info(
		"memory gateway outbox projection",
		"status", obs.Status,
		"event_type", obs.EventType,
		"tenant_id", obs.TenantID,
		"memory_id", obs.MemoryID,
		"event_version", obs.EventVersion,
		"current_version", obs.CurrentVersion,
		"reason", obs.Reason,
	)
}

type multiOutboxObserver struct {
	observers []service.OutboxProjectionObserver
}

func (o multiOutboxObserver) ObserveProjection(ctx context.Context, obs service.OutboxProjectionObservation) {
	for _, observer := range o.observers {
		if observer != nil {
			observer.ObserveProjection(ctx, obs)
		}
	}
}
