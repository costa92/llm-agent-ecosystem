# Changelog

All notable changes to `github.com/costa92/llm-agent-memory-postgres` will be
documented in this file.

<!-- Keep a Changelog format: https://keepachangelog.com/en/1.1.0/ -->
<!-- Semver: https://semver.org/ -->

## [0.1.0] - 2026-05-26

### Added

- Initial Postgres durable-storage backend split out from the SDK module.
- `postgres.Store` with:
  - schema migration
  - idempotent `WriteRecord`
  - OCC mutation paths: `PatchRecord`, `DeleteRecord`, `PinRecord`, `DisableRecord`
  - tenant-bound `GetRecord`
- polling outbox relay with pluggable publisher interface
- `cmd/memory-migrate`

### Dependencies

- `github.com/costa92/llm-agent-memory` for SDK-owned durable abstractions
- `github.com/jackc/pgx/v5` for Postgres connectivity

### Notes

- Live Postgres tests are env-gated behind `LLM_AGENT_MEMORY_PG_URL`.
- Gateway HTTP and service composition are intentionally not part of this
  module.
