# llm-agent-memory / postgres / gateway Split Design

> Date: 2026-05-26
> Scope: split the current mixed SDK + Postgres durable-storage work into three modules with clean boundaries, with SDK-owned pluggable data abstractions

## Goal

Restore `llm-agent-memory` to a pure SDK boundary, define pluggable durable-memory data abstractions in the SDK, move the in-progress Postgres truth-source/outbox implementation into a dedicated sibling module, and create a separate gateway module skeleton for future M6 HTTP/service work.

## Decision

Use a 3-layer split:

1. `llm-agent-memory`
2. `llm-agent-memory-postgres`
3. `llm-agent-memory-gateway`

The `gateway` module is created now as a skeleton only. No HTTP handlers or service logic are implemented in this split step.

The critical architectural constraint is:

- data-related code must be abstracted in SDK-owned interfaces and neutral domain models
- concrete database behavior must live only in backend modules
- backend choice must be swappable without changing SDK-core code

## Why

The current `llm-agent-memory` module is an SDK. Keeping concrete database schema, relay state transitions, migration commands, and environment-driven process wiring inside the SDK creates the wrong release boundary.

The durable Postgres implementation and the future gateway are both valid products, but they are not SDK-core concerns:

- SDK concerns:
  - capability interfaces
  - memory coordination
  - recall/policy/observer surfaces
  - pluggable durable-memory abstractions
- Postgres backend concerns:
  - schema DDL
  - OCC and idempotency persistence rules
  - transactional outbox relay
  - migration command
- Gateway concerns:
  - auth
  - tenant binding
  - HTTP contracts
  - deployment/runtime configuration

## Module Boundaries

## Abstraction Rule

The split follows **storage-layer pluggability**.

`llm-agent-memory` owns the durable-memory contracts. Backends implement them.

The SDK must define neutral interfaces such as:

- `RecordStore`
- `EventStore`
- `IdempotencyStore`
- `Outbox`
- `MessagePublisher` or equivalent publisher seam used by relay/service composition

The SDK may also define neutral durable domain models such as:

- `MemoryRecord`
- `RecordMutation`
- `StoredEvent`
- `IdempotencyEntry`
- `OutboxMessage`

These SDK-owned types must stay database-neutral:

- no SQL column assumptions
- no Postgres-specific types
- no `pgx`
- no DSN/env-var driven wiring
- no baked-in table names
- no persistence-engine-specific status machine names unless they are promoted to true domain semantics

### 1. `llm-agent-memory`

Remains the pure SDK module.

Owns:

- `memory.Manager`
- `memory.RecallEngine`
- observer, policy, sqlite snapshot store, consolidator, scoped lifecycle wrappers
- shared abstract types that are database-neutral
- pluggable durable-storage abstractions for records/events/idempotency/outbox

Must not own:

- Postgres schema
- outbox relay
- process commands
- DSN/env-var driven wiring
- concrete database implementations

### 2. `llm-agent-memory-postgres`

New module for the durable Postgres backend.

Owns:

- implementations of the SDK-owned durable-storage abstractions
- Postgres schema and migration runner
- `WriteRecord`, OCC mutation methods, idempotency persistence
- outbox relay implementation
- `cmd/memory-migrate`

May depend on:

- `llm-agent-memory`
- `pgx/v5`

Must not own:

- HTTP contracts
- auth extraction
- gateway routing
- a parallel copy of SDK-level durable domain models unless strictly implementation-private

### 3. `llm-agent-memory-gateway`

New module for the future service layer.

This split step creates only:

- `go.mod`
- `README.md`
- `doc.go`
- placeholder package layout for future `cmd/` and `internal/`

Later M6 work will own:

- HTTP endpoints
- auth/scope binding
- request validation
- runtime config
- composition of SDK abstractions + chosen backend implementation

## Dependency Direction

Allowed:

- `llm-agent-memory-postgres` -> `llm-agent-memory`
- `llm-agent-memory-gateway` -> `llm-agent-memory`
- `llm-agent-memory-gateway` -> `llm-agent-memory-postgres`

Forbidden:

- `llm-agent-memory` -> `llm-agent-memory-postgres`
- `llm-agent-memory` -> `llm-agent-memory-gateway`
- `llm-agent-memory-postgres` -> `llm-agent-memory-gateway`

Also forbidden:

- backend-only types leaking back into SDK public APIs
- gateway depending on Postgres-specific row/schema concepts instead of SDK abstractions where a neutral contract exists

## Migration Plan

### Phase 1: Create new modules

Create:

- `llm-agent-memory-postgres/`
- `llm-agent-memory-gateway/`

For each new module:

- add `go.mod`
- add `README.md`
- add `doc.go`

For gateway only:

- add empty `cmd/` and `internal/` directories only if needed to anchor future structure

### Phase 2: Move Postgres implementation out of SDK

Before moving files, extract SDK-owned abstractions and neutral durable types out of the current Postgres-centric code.

Move from `llm-agent-memory`:

- `memory/postgres/*`
- `cmd/memory-migrate/*`

Into `llm-agent-memory-postgres`:

- `postgres/*`
- `cmd/memory-migrate/*`

Adjust:

- SDK-level interfaces and neutral types
- package paths
- module import paths
- tests
- go.mod dependencies
- backend code so it implements SDK abstractions rather than owning the contracts itself

### Phase 3: Clean the SDK boundary

In `llm-agent-memory`:

- keep only abstraction definitions and neutral durable domain types
- remove Postgres-specific README sections
- remove Postgres-specific CHANGELOG entries that belong to the backend module instead
- remove `pgx/v5` from the SDK module if no longer needed
- ensure `go test ./...` passes without the Postgres implementation present

### Phase 4: Add gateway skeleton

Create `llm-agent-memory-gateway` with:

- module declaration
- package docs
- README describing future responsibility

Do not add:

- HTTP handlers
- env var wiring
- service startup code

## Naming Decision

Use explicit product names:

- `llm-agent-memory`
- `llm-agent-memory-postgres`
- `llm-agent-memory-gateway`

This is clearer than hiding the backend under `adapter/` or `plugin/` naming because the ecosystem already uses sibling modules with explicit product identities.

## Release Semantics

Recommended:

- keep `llm-agent-memory` on its own SDK release line
- start `llm-agent-memory-postgres` at `v0.x`
- start `llm-agent-memory-gateway` at `v0.x`

Reason:

- the SDK already has a public surface
- the Postgres backend is new and still in milestone flow
- the gateway does not yet have production HTTP behavior

## Risks

1. Import churn during the move

The in-progress Postgres tests and command code will need path updates. This is manageable if done in one focused refactor.

2. README / CHANGELOG drift

The current SDK docs already mention Postgres work. If not cleaned, the old module will continue advertising backend capabilities it no longer owns.

3. Future shared types

Some currently neutral-looking durable-storage payload structs may actually encode Postgres/outbox assumptions. The split must classify them explicitly before moving code. The rule is:

- if the type expresses cross-backend domain semantics, keep it in SDK
- if the type exists to satisfy a concrete Postgres storage shape, move it to the Postgres module

4. Contract leakage

If the SDK abstractions are defined too low-level, the Gateway will still end up depending on Postgres details indirectly. If they are defined too high-level, backend implementations become awkward. The split should target storage-layer pluggability only, not full transaction-manager abstraction.

## Type Ownership Rule

This rule is mandatory for the split:

- Any type that represents portable durable-memory domain semantics belongs in `llm-agent-memory`
- Any type that represents only a concrete Postgres storage or relay implementation belongs in `llm-agent-memory-postgres`

Examples that should stay in SDK if they remain backend-neutral:

- record identity and scope model
- event envelope model
- idempotency contract model
- outbox message contract

Examples that must stay out of SDK:

- SQL row helper structs
- migration metadata tables
- table-prefix configuration
- exact relay persistence status handling if not elevated to domain semantics

## Documentation Migration Rule

The split must not simply delete the current SDK module's Postgres documentation.

Required:

- remove Postgres ownership claims from `llm-agent-memory`
- add equivalent ownership documentation to `llm-agent-memory-postgres`
- preserve the M5 backend narrative in the new backend module's README / CHANGELOG

This prevents loss of feature history during the move.

## Acceptance Criteria

The split is successful when all of the following are true:

- `llm-agent-memory` no longer contains `memory/postgres` or `cmd/memory-migrate`
- `llm-agent-memory` no longer depends on `pgx/v5`
- `llm-agent-memory` exposes durable-memory abstractions in a backend-neutral way
- `llm-agent-memory-postgres` builds and tests independently
- `llm-agent-memory-postgres` implements the SDK-owned data abstractions instead of redefining them
- `llm-agent-memory-gateway` exists as a separate module skeleton
- README and CHANGELOG in each module describe only what that module owns
- dependency direction matches the allowed graph above
- default `go test ./...` in each module passes without requiring a live Postgres instance; live Postgres tests remain env-gated

## Non-Goals

This split step does not:

- finish M6
- implement HTTP endpoints
- add a real MQ backend
- abstract transaction management beyond storage-layer pluggability
- change durable-storage behavior except where required by module relocation

## Recommendation

Proceed with the split now, before adding any more durable-storage code to `llm-agent-memory`.

This is the lowest-cost point to restore the correct architecture boundary.
