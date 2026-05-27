# M5: Postgres Truth Source + Transactional Outbox Implementation

> Date: 2026-05-26
> Status: updated to match the SDK / Postgres backend / gateway split

## Summary

The earlier M5 plan assumed the Postgres durable store would live inside `llm-agent-memory`.
That is no longer the target architecture.

The correct implementation split is:

- `llm-agent-memory`
  - pure SDK
  - owns neutral durable-memory contracts and domain models
- `llm-agent-memory-postgres`
  - owns the full M5 concrete implementation
- `llm-agent-memory-gateway`
  - created as a future M6 composition shell only

## M5 Ownership

### In `llm-agent-memory`

Own only:

- backend-neutral types in `memory/durable.go`
- interfaces that backends implement
- no concrete schema, relay, migration command, or `pgx` usage

### In `llm-agent-memory-postgres`

Own:

- `postgres.Config`
- typed Postgres backend errors
- schema migration logic
- durable record writes and mutations
- idempotency persistence and replay
- polling transactional outbox relay
- `cmd/memory-migrate`

### In `llm-agent-memory-gateway`

Own later in M6:

- HTTP APIs
- auth and tenant binding
- runtime/service composition

Own now:

- skeleton module metadata only

## Code Layout

The concrete M5 implementation lives at:

- `llm-agent-memory-postgres/postgres/config.go`
- `llm-agent-memory-postgres/postgres/errors.go`
- `llm-agent-memory-postgres/postgres/models.go`
- `llm-agent-memory-postgres/postgres/schema.go`
- `llm-agent-memory-postgres/postgres/store.go`
- `llm-agent-memory-postgres/postgres/relay.go`
- `llm-agent-memory-postgres/cmd/memory-migrate/main.go`

The SDK abstraction layer it depends on lives at:

- `llm-agent-memory/memory/durable.go`

## Durable Abstractions

The backend is expected to consume SDK-owned abstractions directly, including:

- `memory.MemoryRecord`
- `memory.StoredEvent`
- `memory.OutboxMessage`
- `memory.IdempotencyEntry`
- `memory.WriteRecordInput`
- `memory.WriteRecordResult`
- `memory.PatchRecordInput`
- `memory.DeleteRecordInput`
- `memory.PinRecordInput`
- `memory.DisableRecordInput`
- `memory.RecordStore`
- `memory.MessagePublisher`

No local alias compatibility layer should be introduced in the backend.

## Constraints

1. No compatibility code.
2. No old in-SDK Postgres package path support.
3. No backend-specific contracts leaking into SDK public APIs.
4. No gateway HTTP code added during M5 cleanup.
5. Data-layer pluggability is enforced at the storage abstraction layer.

## Verification

Use these commands as the implementation gate:

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
GOCACHE=/tmp/go-build go test ./... -count=1

cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-postgres
GOWORK=off GOCACHE=/tmp/go-build go test ./... -count=1

cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-gateway
GOWORK=off GOCACHE=/tmp/go-build go test ./... -count=1
```

## Follow-on

After M5 closes on the backend module, M6 should plan against `llm-agent-memory-gateway`, not by pushing HTTP concerns back into the SDK or Postgres backend module.
