# M8a-prep — Migration Framework v2 + Relay Hardening + Event Dispatch

> Date: 2026-05-27
> Status: first sub-milestone spec under the M8 umbrella. Delivers the three pieces of infrastructure that the rest of M8 depends on but that v1.x cannot ship as additive on top of (umbrella v2.1 §3.1): (a) a staged migration framework that supports expand/validate/contract phases, (b) relay delivery hardening per codex C-1 from M7, and (c) extension of the gateway's event-type allowlist plus the vector publisher's switch dispatch to handle the two new event types M8a will introduce.
> Parent: `docs/superpowers/specs/2026-05-27-m8-umbrella-design.md` (umbrella v2.1).

## 1. Goal

Three independently shippable deliverables, in dependency order:

1. **Migration framework v2** (`llm-agent-memory-postgres`) — a per-group migration runner that records each version transition discretely, so M8a's expand-first 5-phase working-tier rollout has somewhere to live.
2. **Relay delivery hardening** (`llm-agent-memory-postgres`) — write the durable `processing` status with a worker-lease/heartbeat model, replacing today's batch-tx-at-commit semantics (codex C-1 mitigation). M8b's worker depends on this.
3. **Event dispatch extension** (`llm-agent-memory-gateway`) — register `memory_promoted` and `memory_dedupe_collapsed` event-type constants in the postgres backend AND add the explicit `case` arms in `outbox_vector_publisher.go` so these events actually dispatch instead of falling through to "unsupported_event."

Non-goals (per umbrella §6 + this sub-spec):

- No Working-tier schema changes here. M8a-prep adds the *capability* to run staged migrations; M8a will use it.
- No Promoter or Deduper interface introduced. Those land in M8a.
- No Consolidation Worker. M8b.
- No SDK changes beyond constants and event-type literals.
- No change to existing 7 event types' dispatch behavior. The 2 new arms are additive.

## 2. Verified Substrate Facts

Re-verified against `b182c95`.

### 2.1 Current migration runner (`postgres/schema.go`)

```
const SchemaVersion = 1

func (s *Store) Migrate(ctx) error {
    current := currentSchemaVersion(ctx)         // MAX(version) from schema_version table
    if current > SchemaVersion { return ErrSchemaVersionAhead }
    for _, stmt := range s.migrationStatements() {
        s.pool.Exec(ctx, stmt)                   // single flat list of CREATE IF NOT EXISTS
    }
    s.pool.Exec(`INSERT INTO schema_version (version) VALUES (1) ON CONFLICT DO NOTHING`)
}
```

Three properties that matter for the design below:

- **The version table is already multi-row capable.** It's `PRIMARY KEY (version)` with `INSERT ... ON CONFLICT (version) DO NOTHING`. Recording v2, v3, v4, v5 alongside v1 needs no schema change.
- **`migrationStatements()` is a flat list, every statement `IF NOT EXISTS`.** The current statements are idempotent on schema objects but not on *changes to* existing objects (e.g., re-running an `ALTER CONSTRAINT` is not safe without prior introspection).
- **`SchemaVersion = 1` is a hard-coded code-side constant.** When code expects v2 but db is at v1, `Migrate()` runs everything again (because of the `for _, stmt := range`). The "all statements idempotent" property protects this, but it's fragile.

### 2.2 Current relay (`postgres/relay.go`)

```
RunOnce(ctx):
    tx := store.pool.Begin(ctx)
    rows := SELECT outbox_id, payload FROM outbox_event
            WHERE status='pending' ORDER BY created_at
            FOR UPDATE SKIP LOCKED LIMIT batch_size
    for each row:
        err := publisher.Publish(ctx, row.payload)   // synchronous, inside outer tx
        if err:
            UPDATE outbox SET status='pending', attempt_count++, last_error=err WHERE outbox_id=X
        else:
            UPDATE outbox SET status='sent', sent_at=NOW() WHERE outbox_id=X
    tx.Commit(ctx)
```

Failure modes confirmed in the source:

- **`outboxStatusProcessing` is declared but never written** (`store.go:23`).
- **`outboxStatusFailed` does not exist as a status value.** The relay re-queues to `'pending'` after any error, forever (no attempt-count gate).
- **Process crash mid-batch replays the entire processed prefix** — the outer tx hasn't committed yet, so all the per-row UPDATEs are still uncommitted; next `RunOnce` re-claims the same rows.

### 2.3 Current vector publisher dispatch (`gateway/internal/service/outbox_vector_publisher.go`)

```
Publish(ctx, msg corememory.OutboxMessage) error {
    switch msg.EventType {
    case "memory_created", "memory_updated", "memory_pinned", "memory_unpinned", "memory_enabled":
        // project upsert
    case "memory_disabled", "memory_deleted":
        // project remove
    default:
        // emit "unsupported_event" observation; no projection mutation
    }
}
```

Confirmed: M8a-prep must add `case "memory_promoted"` and `case "memory_dedupe_collapsed"` explicitly. Allowlist alone in the postgres backend is not sufficient — that allowlist controls what gets *written* to the outbox; this switch controls what the gateway *consumes from* the outbox.

## 3. Deliverable 1: Migration Framework v2

### 3.1 Lock the shape

Group-based migrations. Each group is a discrete version transition. Existing v1 state (flat statement list) becomes a single "v1 group" without behavioral change.

```go
// postgres/schema.go (replaces SchemaVersion constant + migrationStatements function)
type migrationGroup struct {
    Version    int
    Statements []string
}

const HeadSchemaVersion = 1  // bumped per group landed in M8a-prep + M8a phases

func (s *Store) migrationGroups() []migrationGroup {
    return []migrationGroup{
        {Version: 1, Statements: s.v1Statements()},  // identical to today's migrationStatements() output
        // v2 added by Deliverable 2 (relay lease columns) — see §4.1
        // v3+ added by M8a Phase 1, Phase 2, etc.
    }
}

func (s *Store) Migrate(ctx context.Context) error {
    current, err := s.currentSchemaVersion(ctx)
    if err != nil { return err }
    if current > HeadSchemaVersion { return ErrSchemaVersionAhead }

    for _, group := range s.migrationGroups() {
        if group.Version <= current { continue }
        tx, err := s.pool.Begin(ctx)
        if err != nil { return err }
        if err := s.runGroup(ctx, tx, group); err != nil {
            tx.Rollback(ctx)
            return err
        }
        if err := s.recordVersion(ctx, tx, group.Version); err != nil {
            tx.Rollback(ctx)
            return err
        }
        if err := tx.Commit(ctx); err != nil { return err }
    }
    return nil
}
```

Two locked-in properties:

- **Each group is its own transaction.** A crashed `Migrate` halfway through v3 leaves v1+v2 applied; restart picks up at v3.
- **The `schema_version` table records each version atomically with the group's DDL.** No drift between "what code thinks is applied" and "what's actually in the database."

### 3.2 Bootstrap: existing databases at v1 see only new groups

A database that's already at `version=1` (any production deployment shipped before M8a-prep) starts the loop, sees `group.Version=1 ≤ current`, skips, and proceeds to v2+. **No re-application of v1 statements.** This is the migration that doesn't need a migration to migrate to it.

### 3.3 Idempotency policy per group

Groups must be **internally idempotent** but the framework no longer relies on that property at run time — each group runs at most once per database, guaranteed by the version table. Idempotency is a recovery property only (re-running a failed group after manual repair).

The v1 group remains `CREATE TABLE IF NOT EXISTS` / `CREATE INDEX IF NOT EXISTS` style (already idempotent in source). New groups (v2+) may use `ALTER TABLE ... ADD CONSTRAINT IF NOT EXISTS` (Postgres 9.6+) and `UPDATE ... WHERE` with explicit safety predicates.

### 3.4 Constraint additions that need NOT VALID

Locked by umbrella §4.1 for M8a; M8a-prep's framework must support this pattern. Convention: groups that intend a NOT VALID add + later VALIDATE split into two consecutive groups:

```go
// example, not delivered in M8a-prep — illustrates the pattern M8a will use
{Version: 3, Statements: []string{
    `ALTER TABLE memory_record ADD CONSTRAINT memory_record_kind_v2_check
       CHECK (kind IN ('working','episodic','semantic')) NOT VALID`,
}},
{Version: 4, Statements: []string{
    `ALTER TABLE memory_record VALIDATE CONSTRAINT memory_record_kind_v2_check`,
}},
```

The Migrate runner doesn't know or care that v3 + v4 are "the same logical change in two phases." From the framework's standpoint they're two unrelated groups. From the operator's standpoint, the gap between them is exactly where staging confidence + off-peak deploy windows happen.

## 4. Deliverable 2: Relay Delivery Hardening

### 4.1 Schema additions (group v2 in M8a-prep)

```sql
-- migration group v2, ships as part of M8a-prep
ALTER TABLE outbox_event ADD COLUMN claimed_by         TEXT;
ALTER TABLE outbox_event ADD COLUMN claimed_at         TIMESTAMPTZ;
ALTER TABLE outbox_event ADD COLUMN lease_expires_at   TIMESTAMPTZ;
CREATE INDEX outbox_event_lease_idx ON outbox_event (status, lease_expires_at)
    WHERE status = 'processing';
```

All three columns nullable. Existing rows have NULL → behave as "no lease held." New status value added to the enumerated set (no DB-level CHECK on status today — the code-side constant set is authoritative).

### 4.2 Status state machine (v2)

```
                  publish ok      retries exhausted
   pending  ──claim──>  processing  ──ack──> sent     ───
                          │                          │
                          │  lease expired           │
                          ▼                          │
                       pending (reclaimable)         │
                          │  attempt_count >= N      │
                          ▼                          │
                       failed (manual ops only)      │
```

- **`pending`** — ready to claim. Initial state when outbox row inserted.
- **`processing`** — claimed under lease. `claimed_by`, `claimed_at`, `lease_expires_at` are set.
- **`sent`** — published successfully. Terminal. `sent_at` set.
- **`failed`** — exhausted retries (`attempt_count >= MaxAttempts`, default 5). Terminal except for manual ops. Spec adds the *value* to the code-side constant set; M8a-prep ships it as a constant only — no automated alerting wired here (that's an M8d concern alongside the trace sink work).

### 4.3 New claim flow

```
ClaimBatch(ctx, workerID, leaseTTL, batchSize) ([]ClaimedMessage, error):
    BEGIN
    rows := SELECT outbox_id, payload FROM outbox_event
            WHERE
              status = 'pending'
              OR (status = 'processing' AND lease_expires_at < NOW())
            ORDER BY created_at
            FOR UPDATE SKIP LOCKED
            LIMIT batchSize
    UPDATE outbox_event SET
              status           = 'processing',
              claimed_by       = $workerID,
              claimed_at       = NOW(),
              lease_expires_at = NOW() + $leaseTTL,
              attempt_count    = attempt_count + 1
          WHERE outbox_id = ANY($rowIDs)
    COMMIT
    return claimed
```

Per-claim cost: one transaction, two statements. Reclaim of expired-lease `processing` rows: same query (the `OR (status='processing' AND lease_expires_at < NOW())` branch).

### 4.4 Per-row ack flow

```
Ack(ctx, outboxID, ok bool, publishErr error) error:
    if ok:
        UPDATE outbox_event SET status = 'sent', sent_at = NOW(),
                                claimed_by = NULL, lease_expires_at = NULL
            WHERE outbox_id = $outboxID
    else:
        IF attempt_count >= MaxAttempts:
            UPDATE outbox_event SET status = 'failed', last_error = $publishErr,
                                    claimed_by = NULL, lease_expires_at = NULL
                WHERE outbox_id = $outboxID
        ELSE:
            UPDATE outbox_event SET status = 'pending', last_error = $publishErr,
                                    claimed_by = NULL, lease_expires_at = NULL
                WHERE outbox_id = $outboxID
```

Each ack is its own transaction. N round-trips per batch (was 1). Trade-off: durability over throughput. Documented; if throughput becomes a problem, batch-ack on the success path is a future optimization (M8b spec time).

### 4.5 `RunOnce` rewritten

```go
func (r *Relay) RunOnce(ctx context.Context) (RunStats, error) {
    claimed, err := r.ClaimBatch(ctx, r.workerID, r.leaseTTL, r.batchSize)
    if err != nil { return RunStats{}, err }
    stats := RunStats{}
    for _, msg := range claimed {
        publishErr := r.publisher.Publish(ctx, msg.Payload)
        if publishErr == nil {
            stats.Published++
            if err := r.Ack(ctx, msg.OutboxID, true, nil); err != nil { return stats, err }
        } else {
            stats.Failed++
            if err := r.Ack(ctx, msg.OutboxID, false, publishErr); err != nil { return stats, err }
        }
    }
    return stats, nil
}
```

No outer transaction. Each `Ack` durably commits its own row's status. A process crash mid-batch: only the un-acked rows revert to claimable (via lease expiry), already-acked rows are durably `sent` or `pending+attempt_count++` or `failed`.

### 4.6 Lease TTL default + tuning

Default lease TTL: 60 seconds. Configurable via `RelayConfig.LeaseTTL`. The TTL must exceed the maximum reasonable `Publish` call time (vector index write, downstream HTTP), or work gets reclaimed mid-publish and double-delivered.

`Publish` calls that exceed the lease TTL: undefined behavior in v2 (the second worker will claim and re-deliver). M8b's worker should heartbeat (extend the lease) on long-running publishes — but that's M8b's concern, not M8a-prep's.

## 5. Deliverable 3: Event Dispatch Extension

### 5.1 Postgres backend: event-type constants (`postgres/store.go`)

Add two constants alongside the existing seven (`store.go:15-21`):

```go
const (
    eventTypeMemoryCreated         = "memory_created"
    // ... existing seven ...
    eventTypeMemoryPromoted        = "memory_promoted"          // NEW — emitted by M8a's Promoter
    eventTypeMemoryDedupeCollapsed = "memory_dedupe_collapsed"  // NEW — emitted by M8a's Deduper
)
```

These are unused by M8a-prep's own code (no emission path). M8a will wire `PromoteRecord` / `ResolveDedupe` to emit them. M8a-prep just declares the symbols.

### 5.2 Gateway: vector publisher dispatch (`outbox_vector_publisher.go`)

Add two `case` arms:

```go
switch msg.EventType {
case "memory_created", "memory_updated", "memory_pinned", "memory_unpinned", "memory_enabled":
    // existing — project upsert
case "memory_disabled", "memory_deleted":
    // existing — project remove

case "memory_promoted":
    // M8a-prep addition. Working->Episodic transition. Per umbrella decision: vectors
    // were already projected at memory_created time (existing default behavior),
    // so promotion is a no-op for the vector index. Emit "promoted_noop" observation
    // for visibility. Re-projection policy ("only project after promotion") is
    // an M8c-time decision and is intentionally NOT made here.
    p.observe(ctx, msg, "promoted_noop", msg.Version, "")
    return nil

case "memory_dedupe_collapsed":
    // M8a-prep addition. Loser's vector chunks must be removed. The OutboxMessage
    // payload's MemoryID is the LOSER (not the winner) for this event type — see
    // M8a's ResolveDedupe contract. Call ProjectRemove on the loser.
    return p.projectRemove(ctx, msg)

default:
    p.observe(ctx, msg, "unsupported_event", 0, msg.EventType)
    return nil
}
```

### 5.3 Convention: who owns the payload shape for new event types

`memory_promoted`: payload `MemoryRecord` is the full record AFTER promotion (`kind='episodic'`, `consolidated_from_event_id` set). MemoryID is the same record as before; this is just a tier transition.

`memory_dedupe_collapsed`: payload `MemoryRecord` is the LOSER record (marked `deleted=TRUE`). MemoryID is the loser's. Winner's ID lives in `MemoryRecord.Metadata["dedupe_winner_id"]`.

These conventions are locked here so M8a's emitters and M8a-prep's consumers can agree without further coordination.

### 5.4 Tests

- `outbox_vector_publisher_test.go` adds: `TestPublish_MemoryPromoted_Noop` and `TestPublish_MemoryDedupeCollapsed_RemovesLoser`.
- Both use the existing test scaffolding (no live Postgres).
- The `promoted_noop` observation can be asserted via the existing `OutboxProjectionObserver` mock.

## 6. Cross-Module Impact

| Module | Change | Version |
|---|---|---|
| `llm-agent-memory` (SDK) | No code changes. Constants and event-type literals live in the postgres backend. | Untouched — stays v1.0.0. |
| `llm-agent-memory-postgres` | Migration framework v2 (group-based runner). Schema group v2 (relay lease columns). Relay rewrite (per-row ack + lease). New status constants (`failed`). New event-type constants (`memory_promoted`, `memory_dedupe_collapsed`). | Bumps to v0.(x+1).0 (additive but behavior-changing for relay). |
| `llm-agent-memory-gateway` | Vector publisher: two new `case` arms + payload-shape assumptions documented. | Bumps to v0.(y+1).0 (additive). |
| New module | None. M8a-prep ships entirely in existing modules. | — |

## 7. Sequencing Within M8a-prep

Three deliverables; ordering matters:

1. **D1 (migration framework v2) ships first.** Required so D2's schema additions go through a tracked group.
2. **D2 (relay hardening) ships second.** Independent of D3 but should land before M8a so M8a's emit paths are written against a hardened relay.
3. **D3 (event dispatch) ships third or in parallel with D2.** No dependency between them. Recommended: bundle with D2 in the same PR if scope allows; otherwise serial.

Each deliverable produces its own commit (one feature commit + tests; matching the M7 plan's discipline).

## 8. Acceptance Criteria

The sub-spec is approved when:

1. Migration framework v2 design (§3.1) is endorsed — group-based runner with the existing version table.
2. Relay lease/heartbeat design (§4) is endorsed — `processing` durable status with timeout-based reclaim; `failed` terminal status.
3. New event-type constants + dispatch arms (§5) are endorsed including the payload-shape convention for `memory_dedupe_collapsed` (loser is the payload, winner_id in metadata).
4. Per-row ack model trade-off (durability over throughput, no outer tx) is endorsed; throughput optimization deferred.
5. Lease TTL default (60s) is endorsed or replaced with a different default.

Once §8 is signed off, the implementation plan branches: `docs/superpowers/plans/<date>-m8a-prep-implementation.md` with task-level TDD checkboxes in the M7 plan style.

## 9. Known Weaknesses

- **W-1.** `failed` status has no automated alerting in M8a-prep. Operators must poll `SELECT COUNT(*) WHERE status='failed'`. M8d's reason-enum freeze + observability work could surface this; until then, it's an ops dashboard query, not a paged alert.
- **W-2.** Per-row ack adds N transactions per batch. For batch_size=100 against a high-latency Postgres, this is a 10× round-trip increase. No production load data available pre-shipping. M8b spec should benchmark before turning the Consolidation Worker on.
- **W-3.** Lease TTL of 60s assumes `Publish` calls complete in ≪60s. The current vector publisher calls into the RAG store which could exceed this under load. Heartbeat is M8b's responsibility, but the relay needs to expose a `RenewLease(outboxID, additionalTTL)` API for that. Spec leaves this as a future addition; M8b sub-spec must require it.
- **W-4.** The `memory_promoted` no-op decision in §5.2 is conservative. If M8c later changes vector-projection policy to "only project after promotion" (saves vector store space), the gateway must re-project on promoted events — that's an M8c-time follow-up that touches code added here. Cross-reference noted.
- **W-5.** `memory_dedupe_collapsed` payload's "winner_id in `MemoryRecord.Metadata`" is a convention, not a typed field. A future cleanup (M8c's Metadata→typed-fields lift) should promote it to a typed field. Until then, consumers must defensively read `metadata["dedupe_winner_id"].(string)`.
