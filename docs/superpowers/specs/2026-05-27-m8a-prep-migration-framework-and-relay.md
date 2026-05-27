# M8a-prep — Migration Framework v2 + Relay Hardening + Event Dispatch (v2)

> Date: 2026-05-27
> Status: **revised after two-round review.** v1 produced 21 findings across Plan-type internal review (one CRITICAL + four HIGH + many MEDIUM) and codex consult (three more HIGH on process-boundary concerns). Both verdicts: "needs another spec pass." v2 addresses all 21 — most importantly the missing `WHERE claimed_by=$workerID AND lease_expires_at>NOW()` predicate on Ack (without which the lease+processing model is *worse* than today's batch-tx-at-commit because it actively enables double-delivery), the missing DDL-outside-tx escape hatch, the unspecified worker identity / graceful-shutdown story, the write-side event-type allowlist gap, and the inverted `memory_dedupe_collapsed` payload convention.
> Parent: `docs/superpowers/specs/2026-05-27-m8-umbrella-design.md` (umbrella v2.1).

## 1. Goal

Three deliverables, in dependency order:

1. **Migration framework v2** — group-based runner with per-group transactionality control (default in-tx; opt-out for `CREATE INDEX CONCURRENTLY` and similar non-transactional DDL).
2. **Relay delivery hardening** — `processing` durable status + worker identity + per-ack ownership check + graceful release on shutdown + `failed` terminal status with a defined operator requeue interface.
3. **Event dispatch extension** — postgres-side write allowlist enforcement (NEW in v2 — was missing entirely from v1), two new event-type constants, two new dispatch case arms in the gateway, and reversed `memory_dedupe_collapsed` payload convention (MemoryID = winner, loser_id in metadata).

Non-goals (unchanged from v1): no Working-tier schema yet, no Promoter/Deduper yet, no Consolidation Worker yet. M8a-prep delivers infrastructure that M8a then uses.

## 2. Verified Substrate Facts

Re-verified against `5ce5c17`. v1's facts hold; this section restates only items that drive v2's additions.

### 2.1 Write-side event-type allowlist is missing today (codex finding)

`store.go:300` (`AppendEvent`), `store.go:362` (`EnqueueOutbox`), `store.go:391` (`mutateRecord`) all accept caller-supplied `EventType string` and insert verbatim. A typo (`memry_promoted`) persists to both `memory_event` and `outbox_event` rows without complaint. M8a's Promoter has no compile-time guarantee against this. **(v2 fix: §5.4.)**

### 2.2 `GetRecord` masks deleted/disabled rows (codex finding)

`store.go:263` returns `ErrNotFound` when `record.Deleted` is true. Vector publisher's `currentRecord(ctx, msg)` at `outbox_vector_publisher.go:33` is a thin wrapper. **Consequence:** the v1 spec's "memory_dedupe_collapsed: MemoryID=loser" convention forces all future consumers to a special-case lookup pattern. If they use the standard pattern, they silently leak projections (`GetRecord` returns `ErrNotFound` on the loser, consumer treats as stale, no-ops). **(v2 fix: §5.3 — reverse convention.)**

### 2.3 Fresh-database bootstrap relies on `information_schema` probe

`schema.go:35-49` checks `information_schema.tables` for the version table before querying `MAX(version)`. Returns 0 on a fresh DB. This is the invariant the new framework must preserve — v2 makes it an explicit acceptance criterion.

### 2.4 No worker-identity mechanism in the current relay

`relay.go:39` `NewRelay(store, publisher, batchSize)` — no worker-ID parameter. There is no symbol named `workerID` anywhere in the relay package. v1 referenced `$workerID` in pseudo-code without specifying how a worker obtains one.

## 3. Deliverable 1: Migration Framework v2

### 3.1 Group-based runner with per-group transactionality

```go
type migrationGroup struct {
    Version       int
    Statements    []string
    Transactional bool  // default true; set false for CREATE INDEX CONCURRENTLY etc.
}

const HeadSchemaVersion = 2  // bumped per group added during M8a-prep + M8a phases

func (s *Store) migrationGroups() []migrationGroup {
    return []migrationGroup{
        {Version: 1, Transactional: true,  Statements: s.v1Statements()},
        {Version: 2, Transactional: true,  Statements: s.v2RelayLeaseStatements()},
        // M8a will add Version: 3 NOT VALID kind constraint, Version: 4 VALIDATE, etc.
    }
}
```

Each group is applied at most once per database (tracked by `schema_version` table). Bootstrap for a fresh DB: `currentSchemaVersion` returns 0, all groups apply in order. Bootstrap for an existing v1 DB: skip v1, apply v2+.

### 3.2 Transactional vs non-transactional

For `Transactional == true` groups (default):

```go
tx := s.pool.Begin(ctx)
for _, stmt := range group.Statements { tx.Exec(stmt) }
recordVersion(tx, group.Version)
tx.Commit()
```

For `Transactional == false`:

```go
for _, stmt := range group.Statements { s.pool.Exec(ctx, stmt) }  // no tx
recordVersion(s.pool, group.Version)  // last-step: write version row
```

The non-tx path is **not atomic**. If a statement fails partway, the version row is not written; rerun re-attempts all statements (so all statements in non-tx groups must be individually idempotent — typically `CREATE INDEX CONCURRENTLY IF NOT EXISTS` and similar). M8a-prep does not ship any non-tx groups; the escape hatch exists for M8a to use when it adds the dedupe index.

### 3.3 Idempotency for `ALTER TABLE ADD CONSTRAINT` (NOT VALID)

**(v2 fix.)** v1's `ALTER TABLE ... ADD CONSTRAINT ... NOT VALID` is not natively `IF NOT EXISTS`-able in Postgres ≤15. Use the `DO $$ ... EXCEPTION` idempotency wrapper:

```sql
DO $$
BEGIN
    ALTER TABLE memory_record
      ADD CONSTRAINT memory_record_kind_v2_check
      CHECK (kind IN ('working','episodic','semantic')) NOT VALID;
EXCEPTION
    WHEN duplicate_object THEN NULL;
END;
$$;
```

Every constraint-adding group statement in M8a-prep and M8a must use this wrapper. Spec-time enforcement: M8a-prep's plan checklist includes "constraint adds use DO/EXCEPTION wrapper" as a TDD assertion.

### 3.4 Rolling-deploy version tolerance

**(v2 fix.)** v1 kept the strict `current > HeadSchemaVersion → ErrSchemaVersionAhead`. During rolling deploys this fails the older pods. v2 changes the rule:

```go
if current > HeadSchemaVersion + AcceptableSkewVersions {
    return ErrSchemaVersionAhead
}
// otherwise: warn, do not fail
```

`AcceptableSkewVersions` defaults to 5 (covers a one-minor-version deploy window across ~5 staged groups). Older pods log a warning, continue starting, and run against a DB that's ahead. They don't run new migrations themselves (`group.Version > HeadSchemaVersion` is skipped naturally by the loop).

### 3.5 Acceptance criterion: fresh-DB bootstrap

The v2 framework's first M8a-prep test asserts: starting from a database with `schema_version` table NOT EXISTING, `Migrate(ctx)` succeeds and leaves the database at HeadSchemaVersion. This is the codex-flagged invariant pinned.

## 4. Deliverable 2: Relay Delivery Hardening

### 4.1 Schema additions (group v2)

```sql
ALTER TABLE <s.outboxTable()> ADD COLUMN claimed_by         TEXT;
ALTER TABLE <s.outboxTable()> ADD COLUMN claimed_at         TIMESTAMPTZ;
ALTER TABLE <s.outboxTable()> ADD COLUMN lease_expires_at   TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS <s.outboxTable()>_lease_idx
    ON <s.outboxTable()> (status, lease_expires_at)
    WHERE status = 'processing';
```

**(v2 fix.)** All table references use `s.outboxTable()` (v1 wrote raw `outbox_event` — bug.) Lease column adds rely on Postgres 11+ for metadata-only nullable adds; spec pins **Postgres 11+ as minimum** at top of doc.

### 4.2 Status state machine

Same as v1: `pending → processing → sent | failed`, with lease-expiry recovery for orphaned `processing` rows. `failed` terminal until operator `RequeueFailed` runs (see §4.7).

### 4.3 Worker identity (NEW in v2)

```go
type RelayConfig struct {
    BatchSize       int
    LeaseTTL        time.Duration  // default 180s — see §4.5
    MaxAttempts     int            // default 5
    WorkerIDFunc    func() string  // default: NewRandomWorkerID()
}

// NewRandomWorkerID returns a process-lifetime unique ID:
//   "<hostname>-<random-128-bit-hex>"
// The hostname is informational only; uniqueness comes from the random suffix.
// Two pods with the same k8s pod name get DIFFERENT WorkerIDs because they each
// generate a fresh random suffix at startup. Pod restart = new WorkerID.
func NewRandomWorkerID() string { ... }
```

**Why random, not hostname/pod-name:** k8s pod names can repeat across restarts (StatefulSet ordinals, Deployment hash collisions). A restarted pod that inherits the same name would also inherit any pending leases owned by its dead self — but only by satisfying the `WHERE claimed_by=$workerID` predicate, which would *enable* the very corruption we're trying to prevent. Random per-process IDs break this aliasing.

`WorkerID` is opaque to the schema (column type `TEXT`). The relay records it on claim; uses it on ack.

### 4.3a ClaimBatch SQL (v2.1 — explicit attempt_count increment)

**(v2.1 fix — v2 had no ClaimBatch section; the SQL was implicit.)** ClaimBatch's UPDATE writes the lease state AND increments `attempt_count` atomically. This is the single mutation site for `attempt_count` (§4.9 invariant); Ack never touches it.

```sql
-- Inside ClaimBatch's outer tx, after the SELECT FOR UPDATE SKIP LOCKED:
UPDATE <s.outboxTable()>
   SET status           = 'processing',
       claimed_by       = $workerID,
       claimed_at       = NOW(),
       lease_expires_at = NOW() + $leaseTTL,
       attempt_count    = attempt_count + 1
 WHERE outbox_id = ANY($rowIDs)
```

`MaxAttempts` gate (§4.4 Ack-fail branch) trips when this incremented value reaches the threshold AND the publish for *this* claim fails. A successful publish on the first claim leaves `attempt_count=1` durably on the `sent` row — informational, not actionable.

### 4.4 Ack with ownership predicate (CRITICAL FIX)

**(v2 CRITICAL fix.)** v1's Ack UPDATE had no ownership check. v2:

```go
func (r *Relay) Ack(ctx context.Context, outboxID string, ok bool, publishErr error) error {
    var stmt string
    var args []any
    if ok {
        stmt = `UPDATE ` + r.store.outboxTable() + ` SET status='sent', sent_at=NOW(),
                    claimed_by=NULL, lease_expires_at=NULL
                WHERE outbox_id=$1 AND claimed_by=$2 AND lease_expires_at > NOW()`
        args = []any{outboxID, r.workerID}
    } else if attemptCountReachedMax {
        stmt = `UPDATE ... SET status='failed', last_error=$3,
                    claimed_by=NULL, lease_expires_at=NULL
                WHERE outbox_id=$1 AND claimed_by=$2 AND lease_expires_at > NOW()`
        args = []any{outboxID, r.workerID, publishErr.Error()}
    } else {
        stmt = `UPDATE ... SET status='pending', last_error=$3,
                    claimed_by=NULL, lease_expires_at=NULL
                WHERE outbox_id=$1 AND claimed_by=$2 AND lease_expires_at > NOW()`
        args = []any{outboxID, r.workerID, publishErr.Error()}
    }
    tag, err := r.store.pool.Exec(ctx, stmt, args...)
    if err != nil { return err }
    if tag.RowsAffected() == 0 {
        // Lease lost before Ack — another worker has reclaimed this row
        // OR our claim has expired. Do NOT touch the row.
        return ErrLeaseLost
    }
    return nil
}
```

**`ErrLeaseLost`** is a typed sentinel. Callers observe it via the `RunOnce` loop (see §4.6) and increment a `relay_lease_lost_total` counter (operational telemetry). The lost row is **not** double-acked — the worker that re-claimed it owns the outcome.

### 4.5 Lease TTL default raised to 180s

**(v2 fix — was 60s in v1.)** The vector publisher's `Publish` path calls into the embedder via SDK + provider HTTP. Under provider degradation, p99 of that chain can reach 60-90 seconds. A 60s lease default would systematically expire mid-publish during degraded periods, triggering double-delivery + Ack-after-lease-loss.

180s is conservative but accommodates 2× p99 embedding latency under degradation. Operators tune via `RelayConfig.LeaseTTL`. M8b's worker may add heartbeat (extend lease mid-publish) — out of scope for M8a-prep. Until heartbeat ships, the 180s default is the safety margin.

### 4.6 RunOnce continues on ack failure

**(v2 fix.)** v1's RunOnce early-returned on the first Ack failure, abandoning the rest of the batch to lease expiry. v2 collects errors:

```go
func (r *Relay) RunOnce(ctx context.Context) (RunStats, error) {
    claimed, err := r.ClaimBatch(ctx)
    if err != nil { return RunStats{}, err }
    stats := RunStats{}
    var ackErrors []error
    for _, msg := range claimed {
        publishErr := r.publisher.Publish(ctx, msg.Payload)
        ackErr := r.Ack(ctx, msg.OutboxID, publishErr == nil, publishErr)
        switch {
        case ackErr == ErrLeaseLost:
            stats.LeaseLost++
        case ackErr != nil:
            ackErrors = append(ackErrors, ackErr)
            // continue — DO NOT abandon remaining rows
        case publishErr == nil:
            stats.Published++
        default:
            stats.Failed++
        }
    }
    if len(ackErrors) > 0 {
        return stats, fmt.Errorf("ack errors during run: %w", errors.Join(ackErrors...))
    }
    return stats, nil
}
```

`stats.Published` increments only on (publish-ok AND ack-ok). `stats.LeaseLost` is new (telemetry for the workerID ownership race). `stats.Failed` is publish-failed AND ack-ok.

### 4.7 Graceful shutdown — `Release(ctx)`

**(NEW in v2.)** Codex finding: every rolling deploy leaves up to 60s (or 180s in v2) of stale leases. Fix:

```go
func (r *Relay) Release(ctx context.Context) error {
    _, err := r.store.pool.Exec(ctx,
        `UPDATE ` + r.store.outboxTable() + `
         SET status='pending', claimed_by=NULL, claimed_at=NULL, lease_expires_at=NULL
         WHERE claimed_by=$1 AND status='processing'`,
        r.workerID,
    )
    return err
}
```

Called from main on SIGTERM / preStop. Best-effort (DB may be unreachable); on failure, leases simply expire after `LeaseTTL`. Result is the same; difference is rollout latency.

Convention: deployments configure `terminationGracePeriodSeconds >= LeaseTTL + Publish-budget` so in-flight publishes complete OR get reclaimed cleanly.

### 4.8 `failed` operator interface (NEW in v2)

```go
func (s *Store) RequeueFailed(ctx context.Context, outboxID string) (RequeueResult, error) {
    // UPDATE outbox_event SET
    //     status='pending',
    //     attempt_count=0,
    //     last_error=NULL,
    //     claimed_by=NULL,
    //     lease_expires_at=NULL
    //   WHERE outbox_id=$1 AND status='failed'
    // Returns rows-affected.
}

func (s *Store) ListFailed(ctx context.Context, limit int) ([]FailedOutboxRow, error) {
    // SELECT outbox_id, aggregate_id, event_type, attempt_count, last_error, created_at
    //   FROM outbox_event
    //   WHERE status='failed'
    //   ORDER BY created_at DESC
    //   LIMIT $1
}
```

Audit trail: the original `memory_event` row remains untouched. Resetting `attempt_count` to 0 means a retried row that fails again will exhaust its retries with a clean slate. Operators that want to preserve "this row has been retried N times" should query `memory_event` history (which has the durable record) rather than relying on the outbox row's counter.

### 4.9 attempt_count semantic clarification

**(v2 fix.)** v1 changed the semantics implicitly (increment-on-claim vs increment-on-fail). v2 makes this explicit:

- **`attempt_count` counts CLAIM attempts, not failure events.** Every `ClaimBatch` that touches a row increments it. Successful publish + ack still leaves attempt_count > 0 on the durable row before status transitions to `sent` (where the column becomes irrelevant).
- The `MaxAttempts` gate trips when `attempt_count >= MaxAttempts` AND the publish fails on that claim.
- Existing test `assertOutboxAttemptCount … want 1` continues to pass under the new semantics (one claim = attempt_count=1).

### 4.10 Cross-tenant fairness — explicitly out of scope

**(v2 documentation.)** Codex flagged that the claim query has no tenant dimension, so a single tenant's flood of writes can monopolize all workers. v2 acknowledges this is intentional for M8a-prep: the relay is global FIFO. Tenant-aware fairness is deferred to a future sub-spec (potentially M8b or post-M8). Document in code comment + sub-spec acceptance criteria.

## 5. Deliverable 3: Event Dispatch Extension

### 5.1 Write-side allowlist enforcement (NEW in v2)

**(NEW deliverable in v2 — codex finding.)** Add a centralized allowlist in `postgres/store.go`:

```go
var allowedEventTypes = map[string]struct{}{
    eventTypeMemoryCreated:         {},
    eventTypeMemoryUpdated:         {},
    eventTypeMemoryDeleted:         {},
    eventTypeMemoryPinned:          {},
    eventTypeMemoryUnpinned:        {},
    eventTypeMemoryDisabled:        {},
    eventTypeMemoryEnabled:         {},
    eventTypeMemoryPromoted:        {},  // NEW (M8a-prep)
    eventTypeMemoryDedupeCollapsed: {},  // NEW (M8a-prep)
}

func validateEventType(eventType string) error {
    if _, ok := allowedEventTypes[eventType]; !ok {
        return fmt.Errorf("%w: %q", ErrInvalidEventType, eventType)
    }
    return nil
}
```

`AppendEvent` (`store.go:300`), `EnqueueOutbox` (`store.go:362`), and `mutateRecord` (`store.go:391`) all call `validateEventType` before any INSERT/UPDATE. Typo'd event types fail at the SDK boundary, not in production.

### 5.2 Vector publisher dispatch (`outbox_vector_publisher.go`)

```go
switch msg.EventType {
case "memory_created", "memory_updated", "memory_pinned", "memory_unpinned", "memory_enabled":
    // existing — project upsert
case "memory_disabled", "memory_deleted":
    // existing — project remove (uses p.projector.ProjectRemove(...))

case "memory_promoted":
    // Working→Episodic tier transition. Vector was projected at memory_created
    // (current default policy — every record projects). No-op for the index.
    // M8c may change the policy ("only project after promotion"); this case
    // will gain an upsert call then.
    p.observe(ctx, msg, "promoted_noop", msg.Version, "")
    return nil

case "memory_dedupe_collapsed":
    // Loser collapsed into winner. Payload MemoryID = WINNER (the surviving
    // record — matches the convention every other event uses). The loser's ID
    // is in msg.Record.Metadata["dedupe_collapsed_loser_id"]. The loser is
    // marked deleted=TRUE; this event is emitted AFTER the deletion, so any
    // existing memory_deleted event for the loser has already removed its
    // projection via the standard path. This handler is therefore a no-op for
    // projection state; it exists only to observe the collapse event.
    p.observe(ctx, msg, "dedupe_collapsed_observed", msg.Version, "")
    return nil

default:
    p.observe(ctx, msg, "unsupported_event", 0, msg.EventType)
    return nil
}
```

### 5.3 Payload convention REVERSED (v2 fix)

**(v2 CRITICAL fix.)** v1 said "MemoryID = loser." v2 reverses: **MemoryID = winner.** The loser's ID lives in `msg.Record.Metadata["dedupe_collapsed_loser_id"]`.

**Why the reverse:** every other event in the system has `MemoryID = subject = the record being acted on`. Consumers naturally use `GetRecord(msg.MemoryID)` to fetch current state. If MemoryID were the loser (a `deleted=TRUE` row), the standard pattern would silently return `ErrNotFound` and the consumer would no-op — leaking the projection cleanup. Reversing the convention preserves the universal consumer pattern.

**Emission order (locked here so M8a's Deduper implementer doesn't reinvent):**

1. Dedupe collapse decided.
2. Inside one transaction:
    - `UPDATE memory_record SET deleted=TRUE WHERE memory_id = loser_id`
    - `INSERT memory_event (event_type='memory_deleted', memory_id=loser_id, ...)` — standard delete path, triggers normal projection removal downstream
    - `INSERT memory_event (event_type='memory_dedupe_collapsed', memory_id=winner_id, payload includes metadata.dedupe_collapsed_loser_id=loser_id)` — observability + audit
    - `INSERT outbox_event` for both events
3. Commit.

The `memory_deleted` event handles cleanup. The `memory_dedupe_collapsed` event is observational — useful for analytics, audit, and any future consumer that wants to react to the *collapse* fact specifically (vs. an unrelated deletion). Vector publisher no-ops on `memory_dedupe_collapsed` because `memory_deleted` already did the cleanup.

### 5.4 Test fake update

**(v2 fix.)** Codex finding: `MemoryPublisher` test fake at `relay.go:150` has no lease semantics. v2 adds a new fake alongside:

```go
type LeaseAwarePublisher struct {
    Events       []corememory.OutboxMessage
    PublishHook  func(ctx context.Context, msg corememory.OutboxMessage) error  // optional: simulate slow publish
}
```

The hook lets tests inject delays, errors, and even sleeps long enough to trigger lease expiry. Existing `MemoryPublisher` stays for back-compat with existing relay tests; new ack/lease tests use `LeaseAwarePublisher`.

Existing tests (`relay_test.go:134-185`) that exercise the old batch-tx model are **deleted, not adjusted**. The new model is semantically different (no outer tx boundary to assert against). The M8a-prep impl plan must list these test deletions explicitly so reviewers see the regression-coverage shape change.

## 6. Deployment Topology

**(NEW in v2.)** Codex finding: assumptions implicit. v2 makes them explicit:

- **Replica count:** the relay is multi-replica-safe (DB-level `FOR UPDATE SKIP LOCKED` partitions work across replicas). Recommended: 2-3 replicas for HA; one replica is acceptable if outage windows of `LeaseTTL` are tolerable.
- **Leader election:** NOT required. SKIP LOCKED handles partition naturally.
- **Pod naming:** WorkerID is per-process random; pod name reuse is safe.
- **preStop hook:** must call the relay's `Release(ctx)` endpoint OR signal the worker process to do so. `terminationGracePeriodSeconds >= LeaseTTL + Publish budget` to allow in-flight publishes to complete.
- **Worker fleet:** for M8a-prep, the relay runs inside the gateway process (no separate sibling). M8b introduces the Consolidation Worker sibling which adds a SECOND relay consumer pool against the same outbox.

## 7. Cross-Module Impact (v2 — expanded)

| Module | Change | Version |
|---|---|---|
| `llm-agent-memory` SDK | No code changes (consistent with v1). | v1.0.0 unchanged. |
| `llm-agent-memory-postgres` | Migration framework v2 + schema group v2 (relay lease columns + index) + relay rewrite + worker identity + 2 new event constants + write-side allowlist + RequeueFailed/ListFailed operator API + LeaseAwarePublisher test fake. | v0.(x+1).0 (additive but behavior-changing for relay). |
| `llm-agent-memory-gateway` | Vector publisher: 2 new case arms with no-op semantics; observation labels added. | v0.(y+1).0 (additive). |

## 8. Acceptance Criteria (v2)

The sub-spec is approved when:

1. The CRITICAL Ack ownership check (§4.4) is endorsed.
2. The DDL escape hatch (`Transactional bool`, §3.2) is endorsed.
3. The write-side event-type allowlist (§5.1) is endorsed as part of M8a-prep, not deferred.
4. The reversed `memory_dedupe_collapsed` payload convention (§5.3) is endorsed; emission order is locked at umbrella + sub-spec level (M8a's Deduper implementer follows it verbatim).
5. The graceful shutdown `Release(ctx)` API (§4.7) and `failed` operator API (`RequeueFailed` / `ListFailed`, §4.8) are endorsed.
6. The 180s lease TTL default (§4.5) is endorsed; M8b heartbeat is the long-term mitigation.
7. The cross-tenant fairness deferral (§4.10) is endorsed as v1.x-acceptable; future sub-spec owns it.
8. Postgres 11+ minimum (§4.1) is endorsed.

Once §8 is signed off, the implementation plan branches: `docs/superpowers/plans/<date>-m8a-prep-implementation.md`.

## 9. Review-Finding Disposition (v1 → v2)

How each of the 21 round-1 and round-2 findings landed:

| # | Severity | Finding | v2 disposition |
|---|---|---|---|
| 1 | CRITICAL | Ack missing claimed_by + lease_expires_at predicate | §4.4 — CLOSED with explicit predicate + ErrLeaseLost return |
| 2 | HIGH | DDL escape hatch for CREATE INDEX CONCURRENTLY | §3.2 — CLOSED with `Transactional bool` field |
| 3 | HIGH | RunOnce abandons remaining rows on ack failure | §4.6 — CLOSED, collects errors, continues loop |
| 4 | HIGH | Worker identity unspecified | §4.3 — CLOSED with `NewRandomWorkerID()` + rationale |
| 5 | HIGH | No graceful shutdown / lease release | §4.7 — CLOSED with `Release(ctx)` API |
| 6 | HIGH | Write-side event-type allowlist absent | §5.1 — CLOSED, scope expanded to add allowlist validation as M8a-prep deliverable |
| 7 | MED | ALTER ADD CONSTRAINT lacks idempotency | §3.3 — CLOSED with DO/EXCEPTION wrapper |
| 8 | MED | Lease TTL 60s too aggressive | §4.5 — CLOSED, default raised to 180s |
| 9 | MED | Test fake has no lease semantics | §5.4 — CLOSED with LeaseAwarePublisher |
| 10 | MED | Fresh-DB bootstrap invariant unpinned | §3.5 — CLOSED as explicit acceptance criterion |
| 11 | MED | `failed` operator interface undefined | §4.8 — CLOSED with RequeueFailed / ListFailed |
| 12 | MED | `memory_dedupe_collapsed` MemoryID inversion | §5.3 — CLOSED, convention reversed (MemoryID=winner) |
| 13 | MED | Cross-tenant starvation | §4.10 — DEFERRED, documented as v1.x-acceptable |
| 14 | MED | k8s topology implicit | §6 — CLOSED, explicit deployment topology section |
| 15 | LOW | §5.2 calls `p.projectRemove` (no such method) | §5.2 — CLOSED, no longer invented method (memory_deleted path handles cleanup) |
| 16 | LOW | attempt_count semantic shift unflagged | §4.9 — CLOSED, explicit clarification |
| 17 | LOW | §4.1 missing `s.outboxTable()` prefix | §4.1 — CLOSED |
| 18 | LOW | OR-branch ClaimBatch query index efficiency | §4.1 — CLOSED via partial index on `(status, lease_expires_at) WHERE status='processing'` |
| 19 | LOW | Rolling-deploy ErrSchemaVersionAhead | §3.4 — CLOSED with AcceptableSkewVersions tolerance |
| 20 | LOW | ADD COLUMN safety pg version assumption | §4.1 — CLOSED, Postgres 11+ pinned |
| 21 | LOW | W-2 throughput regression unmeasured | §10 W-1 — DOCUMENTED as known weakness; impl plan must benchmark before M8b consumes the relay |

## 10. Known Weaknesses (v2)

- **W-1.** Per-row ack adds N transactions per batch. The implementation plan must benchmark before M8b's worker consumes the relay; if regression > 5×, batch-ack on the success path becomes an M8b-prep follow-on.
- **W-2.** `LeaseAwarePublisher` test fake covers the lease-expiry hazard but cannot cover real GC-pause-then-resume semantics (Go's runtime makes that hard to simulate deterministically). The impl plan adds integration tests against a real Postgres + injected `time.Sleep` to cover this.
- **W-3.** Cross-tenant fairness (§4.10) is intentionally deferred. Operators running multi-tenant gateway deployments must monitor `recall_returned_total{tenant_bucket}` divergence under load; sustained tenant-X dominance is a signal that fairness needs to land.
- **W-4.** The `dedupe_collapsed_loser_id` metadata key is a string convention. Future M8c work that lifts Metadata fields into typed Go fields should promote it. Acknowledged.
- **W-5.** The 180s lease TTL is a safety margin; the real fix is M8b heartbeat. If embedder provider p99 grows past 90s for sustained periods, M8b's heartbeat must land before the next provider-degradation incident.
- **W-6 (v2.1, from third-round review).** §6 notes M8b adds a SECOND relay consumer pool against the same outbox. Combined with §4.10's deferred tenant fairness, the M8b ship doubles the starvation surface area: two pools sharing a global FIFO under skewed tenant load is a known-bad pattern. If M8b ships before tenant fairness lands, the two-pool topology must be re-evaluated — either (a) the second pool subscribes only to events of a different aggregate_type filter, (b) per-pool tenant-bucket sharding ships alongside M8b, or (c) the fairness work itself is pulled forward.
- **W-7 (v2.1, deferred to impl plan).** Third-round review flagged four MED items that don't block spec approval but must land as TDD assertions in the M8a-prep impl plan: (a) `NewRandomWorkerID()` hostname-failure fallback must be defined; (b) existing test fixtures must be audited for unknown EventType literals before §5.1's validateEventType ships; (c) `Release(ctx)` does NOT decrement attempt_count (by design — claims count for retry budget); (d) §3.5 fresh-DB bootstrap acceptance criterion needs an implementation contract stating `currentSchemaVersion` runs OUTSIDE the migration tx and `CREATE TABLE IF NOT EXISTS schema_version` provides the concurrency safety net. None of these are spec-shape concerns.
