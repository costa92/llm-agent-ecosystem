package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const liveEnvVar = "LLM_AGENT_MEMORY_PG_URL"

func openTestPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv(liveEnvVar)
	if dsn == "" {
		t.Skipf("set %s to run live postgres tests", liveEnvVar)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestMigrate_FirstRunCreatesTables(t *testing.T) {
	ctx := context.Background()
	pool := openTestPool(t, ctx)

	prefix := fmt.Sprintf("m5_%d_first", time.Now().UnixNano())
	s, err := New(pool, Config{TablePrefix: prefix})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	assertTableExists(t, ctx, pool, s.memoryRecordTable())
	assertTableExists(t, ctx, pool, s.memoryEventTable())
	assertTableExists(t, ctx, pool, s.idempotencyTable())
	assertTableExists(t, ctx, pool, s.outboxTable())
}

func TestMigrate_IsIdempotent(t *testing.T) {
	ctx := context.Background()
	pool := openTestPool(t, ctx)

	prefix := fmt.Sprintf("m5_%d_idem", time.Now().UnixNano())
	s, err := New(pool, Config{TablePrefix: prefix})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate first run: %v", err)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate second run: %v", err)
	}
}

func TestMigrate_RejectsFutureSchemaVersion(t *testing.T) {
	ctx := context.Background()
	pool := openTestPool(t, ctx)

	prefix := fmt.Sprintf("m5_%d_future", time.Now().UnixNano())
	s, err := New(pool, Config{TablePrefix: prefix})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if _, err := pool.Exec(ctx,
		fmt.Sprintf(`INSERT INTO %s (version) VALUES ($1)`, s.schemaVersionTable()),
		SchemaVersion+1,
	); err != nil {
		t.Fatalf("insert future version: %v", err)
	}
	if err := s.Migrate(ctx); err == nil {
		t.Fatal("expected future schema version error")
	}
}

func TestMigrate_CreatesDecisionTraceTable(t *testing.T) {
	ctx := context.Background()
	pool := openTestPool(t, ctx)

	prefix := fmt.Sprintf("m7_%d_trace", time.Now().UnixNano())
	s, err := New(pool, Config{TablePrefix: prefix})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	assertTableExists(t, ctx, pool, s.memoryDecisionTraceTable())
}

func TestMigrate_DecisionTraceIndexes(t *testing.T) {
	ctx := context.Background()
	pool := openTestPool(t, ctx)

	prefix := fmt.Sprintf("m7_%d_idx", time.Now().UnixNano())
	s, err := New(pool, Config{TablePrefix: prefix})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	table := s.memoryDecisionTraceTable()
	for _, idx := range []string{
		table + "_tenant_time_idx",
		table + "_request_idx",
		table + "_stage_reason_idx",
	} {
		assertIndexExists(t, ctx, pool, table, idx)
	}
}

func TestMigrate_DecisionTraceIdempotent(t *testing.T) {
	ctx := context.Background()
	pool := openTestPool(t, ctx)

	prefix := fmt.Sprintf("m7_%d_idem", time.Now().UnixNano())
	s, err := New(pool, Config{TablePrefix: prefix})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate first run: %v", err)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate second run: %v", err)
	}

	assertTableExists(t, ctx, pool, s.memoryDecisionTraceTable())
}

func assertIndexExists(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table, index string) {
	t.Helper()
	var exists bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1
			FROM pg_indexes
			WHERE schemaname = current_schema()
			  AND tablename = $1
			  AND indexname = $2
		)`,
		table, index,
	).Scan(&exists); err != nil {
		t.Fatalf("index exists query for %s on %s: %v", index, table, err)
	}
	if !exists {
		t.Fatalf("expected index %s on table %s to exist", index, table)
	}
}

func assertTableExists(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table string) {
	t.Helper()
	var exists bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1
			FROM information_schema.tables
			WHERE table_schema = current_schema()
			  AND table_name = $1
		)`,
		table,
	).Scan(&exists); err != nil {
		t.Fatalf("table exists query for %s: %v", table, err)
	}
	if !exists {
		t.Fatalf("expected table %s to exist", table)
	}
}
