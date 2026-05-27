# M5: Postgres Truth Source + Outbox Backend Plan

> Date: 2026-05-26
> Scope: implement M5 on the new three-module split, with the Postgres truth-source/outbox work living only in `llm-agent-memory-postgres`

## Goal

Deliver the M5 durable-write stack with the current module boundaries:

1. `llm-agent-memory`
   - pure SDK
   - owns backend-neutral durable abstractions and domain types
2. `llm-agent-memory-postgres`
   - concrete Postgres backend for M5
   - owns schema, DAL, idempotency persistence, relay, and migration binary
3. `llm-agent-memory-gateway`
   - skeleton only in this step
   - no HTTP implementation yet

This plan supersedes the earlier in-SDK Postgres layout. No `memory/store/pg` work remains inside `llm-agent-memory`.

## Architecture

M5 is now a backend-module milestone, not an SDK-module milestone.

`llm-agent-memory` owns:

- `memory.MemoryRecord`
- `memory.StoredEvent`
- `memory.OutboxMessage`
- `memory.IdempotencyEntry`
- `memory.WriteRecordInput`
- `memory.PatchRecordInput`
- `memory.DeleteRecordInput`
- `memory.PinRecordInput`
- `memory.DisableRecordInput`
- `memory.RecordStore`
- `memory.EventStore`
- `memory.IdempotencyStore`
- `memory.Outbox`
- `memory.MessagePublisher`

`llm-agent-memory-postgres` owns:

- concrete Postgres schema and migrations
- OCC mutation logic
- idempotency replay/conflict rules
- polling transactional outbox relay
- `cmd/memory-migrate`

`llm-agent-memory-gateway` is intentionally out of scope for M5 implementation.

## Current File Map

### SDK module

- `llm-agent-memory/memory/durable.go`
  - durable abstractions and neutral durable-memory domain models

### Postgres backend module

- `llm-agent-memory-postgres/postgres/config.go`
- `llm-agent-memory-postgres/postgres/errors.go`
- `llm-agent-memory-postgres/postgres/models.go`
- `llm-agent-memory-postgres/postgres/schema.go`
- `llm-agent-memory-postgres/postgres/store.go`
- `llm-agent-memory-postgres/postgres/relay.go`
- `llm-agent-memory-postgres/cmd/memory-migrate/main.go`

### Gateway skeleton module

- `llm-agent-memory-gateway/go.mod`
- `llm-agent-memory-gateway/doc.go`
- `llm-agent-memory-gateway/README.md`

## Exit Criteria

M5 is complete when the following hold:

1. `llm-agent-memory` remains a pure SDK and contains no Postgres-specific implementation code.
2. `llm-agent-memory-postgres` contains the concrete truth-source backend:
   - schema migration runner
   - `WriteRecord`
   - `PatchRecord`
   - `DeleteRecord`
   - `PinRecord`
   - `DisableRecord`
   - `GetRecord`
   - relay `RunOnce`
3. Every mutation is a single Postgres transaction spanning:
   - `memory_record`
   - `memory_event`
   - `outbox_event`
   - and `memory_idempotency` where applicable
4. OCC uses `expected_version` and returns typed `ErrVersionConflict`.
5. Idempotency behavior is enforced:
   - same `(tenant_id, idempotency_key, request_hash)` replays the original result
   - same key with different hash returns `ErrIdempotencyConflict`
6. Migration command is runnable from:
   - `llm-agent-memory-postgres/cmd/memory-migrate`
7. `llm-agent-memory-gateway` remains skeleton-only and does not absorb service logic early.

## Sequencing

1. Keep SDK abstractions stable in `llm-agent-memory/memory/durable.go`.
2. Finish all concrete M5 durable behavior inside `llm-agent-memory-postgres`.
3. Keep gateway concerns deferred to M6.
4. Do not add compatibility bridges or dual-path support while doing the split.

## Verification

Required verification commands:

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
GOCACHE=/tmp/go-build go test ./... -count=1

cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-postgres
GOWORK=off GOCACHE=/tmp/go-build go test ./... -count=1

cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-gateway
GOWORK=off GOCACHE=/tmp/go-build go test ./... -count=1
```

## Notes

- No compatibility package or compatibility fallback is part of this plan.
- No SDK-owned concrete database types are allowed.
- No gateway HTTP code is part of M5.
