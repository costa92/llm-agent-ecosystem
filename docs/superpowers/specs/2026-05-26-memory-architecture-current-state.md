# Memory Architecture Current State

> Date: 2026-05-26
> Purpose: canonical current-state and next-step entry for follow-on M5/M6 work

## Current Module Boundary

The memory stack is currently split into three modules:

1. `llm-agent-memory`
   - pure SDK
   - current version: `v1.0.0`
   - owns memory coordination APIs plus backend-neutral durable abstractions
2. `llm-agent-memory-postgres`
   - concrete Postgres durable backend
   - current version: `v0.1.0`
   - owns M5 truth-source, idempotency, outbox relay, and migration command
3. `llm-agent-memory-gateway`
   - skeleton only
   - no HTTP or runtime logic implemented yet

## What Is Already Done

### SDK

`llm-agent-memory` currently owns:

- `memory.Manager`
- `memory.RecallEngine`
- observer / policy / sqlite snapshot store
- durable abstraction layer in `memory/durable.go`

The durable abstraction layer already includes:

- `MemoryRecord`
- `StoredEvent`
- `OutboxMessage`
- `IdempotencyEntry`
- `WriteRecordInput` / `WriteRecordResult`
- `PatchRecordInput` / `PatchRecordResult`
- `DeleteRecordInput` / `DeleteRecordResult`
- `PinRecordInput` / `PinRecordResult`
- `DisableRecordInput` / `DisableRecordResult`
- `RecordStore`
- `EventStore`
- `IdempotencyStore`
- `Outbox`
- `MessagePublisher`

### Postgres backend

`llm-agent-memory-postgres` already implements:

- schema migration
- idempotent `WriteRecord`
- OCC mutation paths:
  - `PatchRecord`
  - `DeleteRecord`
  - `PinRecord`
  - `DisableRecord`
- tenant-bound `GetRecord`
- polling outbox relay with `RunOnce`
- `cmd/memory-migrate`

### Gateway

`llm-agent-memory-gateway` currently contains only:

- `go.mod`
- `doc.go`
- `README.md`

There is no HTTP surface yet.

## Explicit Non-Goals Right Now

These are not implemented yet:

- gateway HTTP handlers
- auth and tenant extraction
- service startup/runtime config
- real MQ publishers
- worker fleet
- cache invalidation pipeline
- vector-index-backed recall service composition

## Important Constraints

1. `llm-agent-memory` must stay SDK-only.
2. Concrete database behavior must stay in backend modules only.
3. No compatibility shim, alias layer, or fallback path should be reintroduced.
4. Gateway work must start in `llm-agent-memory-gateway`, not in the SDK or backend module.
5. Storage pluggability is enforced at the SDK abstraction layer.

## Current Verification Baseline

At the current checkpoint, these commands are the expected baseline:

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
GOCACHE=/tmp/go-build go test ./... -count=1

cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-postgres
GOWORK=off GOCACHE=/tmp/go-build go test ./... -count=1

cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-gateway
GOWORK=off GOCACHE=/tmp/go-build go test ./... -count=1
```

## Recommended Next Step

The next execution target should be one of these two, explicitly chosen:

1. **Finish M5 closeout**
   - audit `llm-agent-memory-postgres` for any remaining implementation/documentation gaps against the M5 exit criteria
   - stabilize release notes and versioning for the backend module
2. **Start M6**
   - plan `llm-agent-memory-gateway`
   - define service composition, transport boundary, auth binding, and endpoint rollout order

## Recommended Source Documents

Use these as the active reference set:

- `docs/superpowers/plans/2026-05-25-llm-agent-memory-roadmap.md`
- `docs/superpowers/plans/2026-05-26-m5-postgres-outbox.md`
- `docs/superpowers/plans/2026-05-26-m5-postgres-truth-source-outbox.md`
- `docs/superpowers/specs/2026-05-26-memory-sdk-postgres-gateway-split-design.md`
- `docs/superpowers/specs/2026-05-26-memory-gateway-module-design.md`

Treat older M4 compatibility planning notes as historical context only, not as the current implementation contract.
