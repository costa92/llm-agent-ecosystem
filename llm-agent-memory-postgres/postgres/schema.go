package postgres

import (
	"context"
	"fmt"
)

const SchemaVersion = 1

func (s *Store) Migrate(ctx context.Context) error {
	current, err := s.currentSchemaVersion(ctx)
	if err != nil {
		return err
	}
	if current > SchemaVersion {
		return fmt.Errorf("%w: db=%d code=%d", ErrSchemaVersionAhead, current, SchemaVersion)
	}
	for _, stmt := range s.migrationStatements() {
		if _, err := s.pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("memory/postgres: migrate: %w", err)
		}
	}
	if _, err := s.pool.Exec(ctx,
		fmt.Sprintf(
			`INSERT INTO %s (version) VALUES ($1) ON CONFLICT (version) DO NOTHING`,
			s.schemaVersionTable(),
		),
		SchemaVersion,
	); err != nil {
		return fmt.Errorf("memory/postgres: migrate record version: %w", err)
	}
	return nil
}

func (s *Store) currentSchemaVersion(ctx context.Context) (int, error) {
	var exists bool
	if err := s.pool.QueryRow(
		ctx,
		`SELECT EXISTS (
			SELECT 1
			FROM information_schema.tables
			WHERE table_schema = current_schema()
			  AND table_name = $1
		)`,
		s.schemaVersionTable(),
	).Scan(&exists); err != nil {
		return 0, fmt.Errorf("memory/postgres: schema version table exists: %w", err)
	}
	if !exists {
		return 0, nil
	}

	var version int
	if err := s.pool.QueryRow(
		ctx,
		fmt.Sprintf(`SELECT COALESCE(MAX(version), 0) FROM %s`, s.schemaVersionTable()),
	).Scan(&version); err != nil {
		return 0, fmt.Errorf("memory/postgres: current schema version: %w", err)
	}
	return version, nil
}

func (s *Store) migrationStatements() []string {
	return []string{
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			version BIGINT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`, s.schemaVersionTable()),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			memory_id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			project_id TEXT,
			session_id TEXT,
			kind TEXT NOT NULL,
			source TEXT NOT NULL,
			category TEXT NOT NULL,
			content TEXT NOT NULL,
			normalized_content_hash TEXT NOT NULL,
			tags JSONB NOT NULL DEFAULT '[]'::jsonb,
			importance DOUBLE PRECISION NOT NULL,
			pinned BOOLEAN NOT NULL DEFAULT FALSE,
			disabled BOOLEAN NOT NULL DEFAULT FALSE,
			deleted BOOLEAN NOT NULL DEFAULT FALSE,
			version BIGINT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ,
			last_access_at TIMESTAMPTZ,
			hit_count BIGINT NOT NULL DEFAULT 0,
			consolidated_from_event_id TEXT,
			UNIQUE (tenant_id, memory_id),
			CHECK (importance >= 0 AND importance <= 1),
			CHECK (kind IN ('episodic', 'semantic')),
			CHECK (source IN ('user_saved', 'agent_inferred', 'system'))
		)`, s.memoryRecordTable()),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			event_id TEXT PRIMARY KEY,
			memory_id TEXT NOT NULL,
			tenant_id TEXT NOT NULL,
			event_type TEXT NOT NULL,
			version BIGINT NOT NULL,
			idempotency_key TEXT,
			payload JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (memory_id, version, event_type)
		)`, s.memoryEventTable()),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			tenant_id TEXT NOT NULL,
			idempotency_key TEXT NOT NULL,
			request_hash TEXT NOT NULL,
			memory_id TEXT,
			response_snapshot JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			expires_at TIMESTAMPTZ,
			PRIMARY KEY (tenant_id, idempotency_key)
		)`, s.idempotencyTable()),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			outbox_id TEXT PRIMARY KEY,
			aggregate_type TEXT NOT NULL,
			aggregate_id TEXT NOT NULL,
			tenant_id TEXT NOT NULL,
			event_id TEXT NOT NULL,
			event_type TEXT NOT NULL,
			payload JSONB NOT NULL,
			status TEXT NOT NULL,
			attempt_count INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			sent_at TIMESTAMPTZ,
			last_error TEXT
		)`, s.outboxTable()),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s_scope_idx ON %s (tenant_id, user_id, project_id, deleted, disabled)`,
			s.memoryRecordTable(), s.memoryRecordTable()),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s_category_idx ON %s (tenant_id, user_id, category, deleted, disabled)`,
			s.memoryRecordTable(), s.memoryRecordTable()),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s_hash_idx ON %s (tenant_id, user_id, normalized_content_hash)`,
			s.memoryRecordTable(), s.memoryRecordTable()),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s_updated_at_idx ON %s (tenant_id, updated_at DESC)`,
			s.memoryRecordTable(), s.memoryRecordTable()),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s_memory_version_idx ON %s (memory_id, version)`,
			s.memoryEventTable(), s.memoryEventTable()),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s_tenant_created_idx ON %s (tenant_id, created_at DESC)`,
			s.memoryEventTable(), s.memoryEventTable()),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s_event_type_created_idx ON %s (event_type, created_at DESC)`,
			s.memoryEventTable(), s.memoryEventTable()),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s_status_created_idx ON %s (status, created_at)`,
			s.outboxTable(), s.outboxTable()),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s_aggregate_created_idx ON %s (aggregate_id, created_at)`,
			s.outboxTable(), s.outboxTable()),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s_event_id_idx ON %s (event_id)`,
			s.outboxTable(), s.outboxTable()),
	}
}
