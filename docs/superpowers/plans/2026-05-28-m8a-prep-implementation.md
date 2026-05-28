# M8a-prep Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans`. TDD throughout; one focused commit per task.

**Goal:** Implement the three deliverables in M8a-prep spec v2.1 (`docs/superpowers/specs/2026-05-27-m8a-prep-migration-framework-and-relay.md`) — migration framework v2, relay delivery hardening with lease + worker identity + graceful shutdown, and event dispatch extension with write-side allowlist enforcement. Zero SDK changes. Postgres backend + gateway only. Existing M7 behavior preserved; all changes additive to live-write paths.

**Architecture:** The migration framework migrates from a single-shot flat-list to a group-based runner that records each version atomically. The relay migrates from a batch-tx-at-commit model to per-row ack semantics with a durable `processing` status, cryptographically-random worker IDs, lease-expiry reclaim of orphaned rows, and graceful shutdown via explicit lease release. Event dispatch gains two new constants, a centralized write-side validator, and matching arms in the gateway's vector publisher.

**Tech Stack:** Go 1.26.0; `context`, `crypto/rand`, `encoding/hex`, `errors`, `fmt`, `os`, `sync/atomic`, `time`, `testing`; `github.com/jackc/pgx/v5`; existing workspace `go.work`. Postgres 11+ assumed (nullable `ADD COLUMN` is metadata-only).

---

## File Structure

| Path | Action | Responsibility |
|---|---|---|
| `llm-agent-memory-postgres/postgres/schema.go` | Modify | Introduce `migrationGroup` struct + `Transactional` field; refactor `Migrate()` to group-based runner with `AcceptableSkewVersions` tolerance; preserve fresh-DB bootstrap behavior |
| `llm-agent-memory-postgres/postgres/schema_test.go` | Modify | Tests for: per-group atomicity; skip-already-applied; non-tx escape hatch; AcceptableSkewVersions tolerance; fresh-DB bootstrap explicit assertion |
| `llm-agent-memory-postgres/postgres/store.go` | Modify | Add `eventTypeMemoryPromoted`, `eventTypeMemoryDedupeCollapsed` constants; add `validateEventType` helper + `allowedEventTypes` map; wire into `AppendEvent` / `EnqueueOutbox` / `mutateRecord`; add `RequeueFailed` + `ListFailed` operator methods |
| `llm-agent-memory-postgres/postgres/store_test.go` | Modify | Allowlist tests at three insertion points; existing fixture audit (no unknown EventType literals); `RequeueFailed` / `ListFailed` happy paths + edge cases |
| `llm-agent-memory-postgres/postgres/errors.go` | Modify | Add `ErrInvalidEventType`, `ErrLeaseLost` sentinels |
| `llm-agent-memory-postgres/postgres/relay.go` | Modify | Add `WorkerID` field + `RelayConfig`; rewrite `RunOnce` to claim + per-row ack flow; add `Release(ctx)`, `ClaimBatch(ctx)`, `Ack(ctx, ok, err)`; remove batch-tx outer transaction |
| `llm-agent-memory-postgres/postgres/relay_test.go` | Modify | Delete batch-tx-shape tests; add lease-state tests; cover Ack ownership predicate + `ErrLeaseLost`; cover RunOnce continues on ack failure |
| `llm-agent-memory-postgres/postgres/worker_id.go` | Create | `NewRandomWorkerID()` returning `<hostname>-<128-bit-hex>` with `unknown-<hex>` fallback on hostname failure |
| `llm-agent-memory-postgres/postgres/worker_id_test.go` | Create | TDD: stability per-process; uniqueness across processes (table-driven 100-iteration sample); hostname-failure fallback |
| `llm-agent-memory-postgres/postgres/lease_aware_publisher.go` | Create | `LeaseAwarePublisher` test fake with `PublishHook` for delay/error injection |
| `llm-agent-memory-postgres/postgres/CHANGELOG.md` | Modify | M8a-prep entry: relay hardening + framework v2 + write-side allowlist |
| `llm-agent-memory-gateway/internal/service/outbox_vector_publisher.go` | Modify | Add `case "memory_promoted"` (no-op + observation) and `case "memory_dedupe_collapsed"` (observation; cleanup via `memory_deleted` event) |
| `llm-agent-memory-gateway/internal/service/outbox_vector_publisher_test.go` | Modify | TDD for two new cases; assert no projection mutation; assert observation labels match spec |
| `llm-agent-memory-gateway/README.md` | Modify | Document new relay env vars (`LLM_AGENT_MEMORY_GATEWAY_RELAY_LEASE_TTL`, `..._MAX_ATTEMPTS`, etc.); deployment topology section (preStop, terminationGracePeriodSeconds) |
| `llm-agent-memory-gateway/internal/config/config.go` | Modify | Add `RelayLeaseTTL` (default 180s), `RelayMaxAttempts` (default 5), `RelayBatchSize` (default 100) knobs |
| `llm-agent-memory-gateway/internal/config/config_test.go` | Modify | TDD for the three new knobs (default + override) |
| `llm-agent-memory-gateway/cmd/memory-gateway/main.go` | Modify | Wire `Relay.Release(ctx)` into shutdown sequence BEFORE pool close; pass `RelayConfig` through composition |

## Open Decisions Locked For This Plan

- **`migrationGroup` is package-private.** External callers don't construct groups; they're fixed in `migrationGroups()`. Exporting buys nothing.
- **`Migrate()` runs each group in its own pgx transaction (default).** Non-tx groups execute statements directly against the pool with the version-row insert as a final separate statement.
- **`HeadSchemaVersion` is bumped to 2 in this PR.** Group v2 contains the relay lease columns + partial index. M8a will bump it further when adding the kind-CHECK constraint.
- **`AcceptableSkewVersions = 5`** default. Plan task asserts the constant exists; runtime tolerance is exercised only by integration tests against a "future DB" fixture.
- **`WorkerID` generation:** `<hostname>-<32-hex-char>` where the hex is `crypto/rand` 128-bit. On `os.Hostname()` failure (any error), substitute `"unknown"`.
- **Per-row Ack uses one pgx Exec per row.** Batch-ack on success path is a future optimization (W-1) — explicitly out of scope here.
- **`Release(ctx)` is best-effort.** Errors are logged, not propagated to shutdown caller.
- **`RequeueFailed` resets `attempt_count` to 0.** No audit row written; operators query `memory_event` history if they need provenance.
- **`memory_dedupe_collapsed` is observation-only in the vector publisher.** Loser cleanup happens via the `memory_deleted` event emitted in the same M8a transaction; this case logs the collapse fact but does not mutate the vector index.
- **`validateEventType` is the only write-side check.** No CHECK constraint on the DB column (would break ordering of future event-type additions vs schema migrations).

## Sequencing Rules

- **Strict TDD.** Every task starts from a failing test.
- **Postgres-touching tests** use the existing `openTestPool(t, ctx)` helper which `t.Skipf`s when `LLM_AGENT_MEMORY_PG_URL` is unset. Default CI invocation (`GOWORK=off go test ./... -count=1`) stays green without a live Postgres.
- **No SDK changes.** If a task needs an SDK signature change, stop and escalate — that's M8a territory, not M8a-prep.
- **No event-type literals in test fixtures.** Tests that need to assert on event types must reference the package constants (post-Task 4, the allowlist will reject typos at write time, so fixtures using bare string literals will start failing).
- **Gateway tests run after postgres tests in CI** — no cross-module test dependencies.
- **Each task ends with a focused commit per the M7 plan discipline.** No batch commits.

---

## Task Plan

### Task 1: Introduce `migrationGroup` type + per-group transactionality

**Files:**
- Modify: `llm-agent-memory-postgres/postgres/schema.go`
- Modify: `llm-agent-memory-postgres/postgres/schema_test.go`

- [ ] **Step 1: Write failing tests**

```go
func TestMigrationGroup_TypeExists(t *testing.T) {} // compile-time
func TestMigrate_RunsGroupsInOrder(t *testing.T) {} // v1 group applied first, v2 next
func TestMigrate_SkipsAlreadyAppliedGroups(t *testing.T) {} // re-run is a no-op
func TestMigrate_TransactionalGroupRollsBackOnError(t *testing.T) {} // failed mid-group → no version recorded
```

- [ ] **Step 2: Run, verify failure**

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-postgres
GOWORK=off GOCACHE=/tmp/go-build go test ./postgres -run 'TestMigrate_|TestMigrationGroup_' -count=1
```

- [ ] **Step 3: Implement**

Introduce:

```go
type migrationGroup struct {
    Version       int
    Transactional bool
    Statements    []string
}

const HeadSchemaVersion = 2  // bumped to 2 in this PR (relay lease columns are group v2)

func (s *Store) migrationGroups() []migrationGroup {
    return []migrationGroup{
        {Version: 1, Transactional: true, Statements: s.v1Statements()},
        {Version: 2, Transactional: true, Statements: s.v2RelayLeaseStatements()}, // Task 3 will fill this
    }
}
```

`v1Statements()` returns today's flat list verbatim (extracted from the existing `migrationStatements()`). Rewrite `Migrate()` to loop over groups, skip applied, run inside tx (if `Transactional`), record version row.

- [ ] **Step 4: Re-run focused tests** — expect `PASS`.
- [ ] **Step 5: Commit**

```bash
git add postgres/schema.go postgres/schema_test.go
git commit -m "refactor(memory-postgres): introduce group-based migration runner"
```

### Task 2: Non-transactional escape hatch + AcceptableSkewVersions tolerance + fresh-DB acceptance criterion

**Files:**
- Modify: `llm-agent-memory-postgres/postgres/schema.go`
- Modify: `llm-agent-memory-postgres/postgres/schema_test.go`

- [ ] **Step 1: Failing tests**

```go
func TestMigrate_NonTransactionalGroupRunsWithoutOuterTx(t *testing.T) {}
func TestMigrate_TolerablyAheadDB_WarnsButProceeds(t *testing.T) {} // current = HeadSchemaVersion + 2 → no error
func TestMigrate_TooFarAheadDB_ReturnsErrSchemaVersionAhead(t *testing.T) {} // current > HeadSchemaVersion + 5
func TestCurrentSchemaVersion_RunsOnFreshDB(t *testing.T) {} // table doesn't exist → returns 0, no error
func TestMigrate_FreshDB_AppliesAllGroups(t *testing.T) {} // current=0, HeadSchemaVersion=2 → both groups applied
```

- [ ] **Step 2: Verify failure**
- [ ] **Step 3: Implement**

```go
const AcceptableSkewVersions = 5

func (s *Store) Migrate(ctx context.Context) error {
    current, err := s.currentSchemaVersion(ctx)
    if err != nil { return err }
    if current > HeadSchemaVersion + AcceptableSkewVersions {
        return fmt.Errorf("%w: db=%d code=%d", ErrSchemaVersionAhead, current, HeadSchemaVersion)
    }
    if current > HeadSchemaVersion {
        // tolerable skew — log and proceed
        // (logger emission is intentionally outside the spec — wire via existing observer or slog)
    }
    for _, group := range s.migrationGroups() {
        if group.Version <= current { continue }
        if group.Transactional {
            if err := s.runGroupInTx(ctx, group); err != nil { return err }
        } else {
            if err := s.runGroupDirect(ctx, group); err != nil { return err }
        }
    }
    return nil
}
```

`runGroupInTx` opens a pgx tx, runs statements, inserts version row, commits. `runGroupDirect` runs statements against the pool and then inserts version row.

`currentSchemaVersion` stays unchanged (existence probe + MAX query).

- [ ] **Step 4: Re-run focused tests** — expect `PASS`.
- [ ] **Step 5: Commit**

```bash
git add postgres/schema.go postgres/schema_test.go
git commit -m "feat(memory-postgres): migration framework v2 — escape hatch + skew tolerance"
```

### Task 3: Schema group v2 — relay lease columns + partial index

**Files:**
- Modify: `llm-agent-memory-postgres/postgres/schema.go`
- Modify: `llm-agent-memory-postgres/postgres/schema_test.go`

- [ ] **Step 1: Failing tests**

```go
func TestMigrate_RelayLeaseColumns_Added(t *testing.T) {} // claimed_by, claimed_at, lease_expires_at exist after group v2
func TestMigrate_RelayLeaseIndex_PartialOnProcessing(t *testing.T) {} // partial WHERE status='processing'
```

- [ ] **Step 2: Verify failure**
- [ ] **Step 3: Implement**

`v2RelayLeaseStatements()` returns:

```sql
ALTER TABLE <s.outboxTable()> ADD COLUMN claimed_by TEXT
ALTER TABLE <s.outboxTable()> ADD COLUMN claimed_at TIMESTAMPTZ
ALTER TABLE <s.outboxTable()> ADD COLUMN lease_expires_at TIMESTAMPTZ
CREATE INDEX IF NOT EXISTS <s.outboxTable()>_lease_idx
    ON <s.outboxTable()> (status, lease_expires_at)
    WHERE status = 'processing'
```

Use the `DO $$ ... EXCEPTION WHEN duplicate_column THEN NULL; END $$;` wrapper for ADD COLUMN to make the group idempotent in case of a partial-failure re-run (Postgres 9.6+ supports `ADD COLUMN IF NOT EXISTS`; check the deployed version and use whichever wraps cleaner).

- [ ] **Step 4: Re-run** — `PASS`.
- [ ] **Step 5: Commit**

```bash
git add postgres/schema.go postgres/schema_test.go
git commit -m "feat(memory-postgres): schema group v2 — relay lease columns + partial index"
```

### Task 4: Event-type constants + `validateEventType` helper

**Files:**
- Modify: `llm-agent-memory-postgres/postgres/store.go`
- Modify: `llm-agent-memory-postgres/postgres/store_test.go`
- Modify: `llm-agent-memory-postgres/postgres/errors.go`

- [ ] **Step 1: Failing tests**

```go
func TestEventTypeConstants_PromotedAndDedupeCollapsed(t *testing.T) {} // compile-time check
func TestValidateEventType_AcceptsKnownTypes(t *testing.T) {}
func TestValidateEventType_RejectsTypo(t *testing.T) {}
func TestValidateEventType_RejectsEmpty(t *testing.T) {}
```

- [ ] **Step 2: Verify failure**
- [ ] **Step 3: Implement**

```go
const (
    eventTypeMemoryCreated         = "memory_created"
    // ... existing 7 ...
    eventTypeMemoryPromoted        = "memory_promoted"
    eventTypeMemoryDedupeCollapsed = "memory_dedupe_collapsed"
)

var allowedEventTypes = map[string]struct{}{
    eventTypeMemoryCreated:         {},
    // ... all 9 ...
}

func validateEventType(eventType string) error {
    if _, ok := allowedEventTypes[eventType]; !ok {
        return fmt.Errorf("%w: %q", ErrInvalidEventType, eventType)
    }
    return nil
}
```

Add `ErrInvalidEventType` sentinel to `errors.go`.

- [ ] **Step 4: Re-run** — `PASS`.
- [ ] **Step 5: Commit**

```bash
git add postgres/store.go postgres/store_test.go postgres/errors.go
git commit -m "feat(memory-postgres): add event-type allowlist + validator helper"
```

### Task 5: Wire `validateEventType` into write-path

**Files:**
- Modify: `llm-agent-memory-postgres/postgres/store.go`
- Modify: `llm-agent-memory-postgres/postgres/store_test.go`

- [ ] **Step 1: Failing tests**

```go
func TestAppendEvent_RejectsInvalidEventType(t *testing.T) {}
func TestEnqueueOutbox_RejectsInvalidEventType(t *testing.T) {}
func TestMutateRecord_RejectsInvalidEventType(t *testing.T) {}
```

Each test feeds a deliberate typo and asserts `errors.Is(err, ErrInvalidEventType)`.

Also: audit existing test fixtures (`store_test.go`, `relay_test.go`, gateway tests that construct OutboxMessage). If any use bare string literals (`"memory_created"`), they continue to pass because those values ARE in the allowlist — the audit confirms no typos exist in fixtures.

- [ ] **Step 2: Verify failure**
- [ ] **Step 3: Implement**

Insert `if err := validateEventType(eventType); err != nil { return ..., err }` early in:
- `AppendEvent` (`store.go:300`-ish)
- `EnqueueOutbox` (`store.go:362`-ish)
- `mutateRecord` (`store.go:391`-ish)

- [ ] **Step 4: Re-run full module tests** — `go test ./... -count=1` must pass (audit confirms fixture integrity).
- [ ] **Step 5: Commit**

```bash
git add postgres/store.go postgres/store_test.go
git commit -m "feat(memory-postgres): enforce event-type allowlist at write-path"
```

### Task 6: Worker identity helper

**Files:**
- Create: `llm-agent-memory-postgres/postgres/worker_id.go`
- Create: `llm-agent-memory-postgres/postgres/worker_id_test.go`

- [ ] **Step 1: Failing tests**

```go
func TestNewRandomWorkerID_Format(t *testing.T) {}        // matches "<hostname>-<32hex>"
func TestNewRandomWorkerID_UniqueAcrossCalls(t *testing.T) {} // 100 iterations, all distinct
func TestNewRandomWorkerID_HostnameFailureFallback(t *testing.T) {} // inject hostname failure → "unknown-<hex>"
```

The hostname-failure test injects a fake `hostnameFn func() (string, error)` so it doesn't need to actually corrupt `/etc/hostname`.

- [ ] **Step 2: Verify failure**
- [ ] **Step 3: Implement**

```go
package postgres

import (
    "crypto/rand"
    "encoding/hex"
    "fmt"
    "os"
)

var hostnameFn = os.Hostname

func NewRandomWorkerID() string {
    name, err := hostnameFn()
    if err != nil || name == "" { name = "unknown" }
    var buf [16]byte
    _, _ = rand.Read(buf[:])
    return fmt.Sprintf("%s-%s", name, hex.EncodeToString(buf[:]))
}
```

- [ ] **Step 4: Re-run** — `PASS`.
- [ ] **Step 5: Commit**

```bash
git add postgres/worker_id.go postgres/worker_id_test.go
git commit -m "feat(memory-postgres): add NewRandomWorkerID for relay worker identity"
```

### Task 7: Relay rewrite — `RelayConfig` + `ClaimBatch` with lease state + attempt_count increment

**Files:**
- Modify: `llm-agent-memory-postgres/postgres/relay.go`
- Modify: `llm-agent-memory-postgres/postgres/relay_test.go`

- [ ] **Step 1: Failing tests**

```go
func TestRelayConfig_Defaults(t *testing.T) {}            // LeaseTTL=180s, MaxAttempts=5, BatchSize=100
func TestClaimBatch_SetsLeaseColumns(t *testing.T) {}      // claimed_by, claimed_at, lease_expires_at populated
func TestClaimBatch_IncrementsAttemptCount(t *testing.T) {} // attempt_count++ on every claim
func TestClaimBatch_ReclaimsExpiredLeases(t *testing.T) {} // status='processing' AND lease_expires_at < NOW() → reclaimable
func TestClaimBatch_RespectsBatchSize(t *testing.T) {}
```

- [ ] **Step 2: Verify failure**

Note: this task will BREAK existing relay_test.go (`TestRelay_RunOnceClaimsRowsBeforePublish` etc.) because they assert the old batch-tx shape. **Delete those tests in this commit** — they're being replaced by the new TDD set. List each deleted test in the commit message.

- [ ] **Step 3: Implement**

```go
type RelayConfig struct {
    BatchSize    int
    LeaseTTL     time.Duration
    MaxAttempts  int
    WorkerIDFunc func() string
}

func defaultRelayConfig() RelayConfig {
    return RelayConfig{
        BatchSize:    100,
        LeaseTTL:     180 * time.Second,
        MaxAttempts:  5,
        WorkerIDFunc: NewRandomWorkerID,
    }
}

type Relay struct {
    store     *Store
    publisher Publisher
    cfg       RelayConfig
    workerID  string
}

func NewRelay(store *Store, publisher Publisher, cfg RelayConfig) (*Relay, error) { /* ... */ }

func (r *Relay) ClaimBatch(ctx context.Context) ([]ClaimedMessage, error) {
    // BEGIN tx
    // SELECT outbox_id, payload FROM <outboxTable> WHERE
    //     status='pending' OR (status='processing' AND lease_expires_at < NOW())
    //   ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT batchSize
    // UPDATE <outboxTable> SET
    //     status='processing', claimed_by=$workerID, claimed_at=NOW(),
    //     lease_expires_at=NOW()+$leaseTTL, attempt_count=attempt_count+1
    //   WHERE outbox_id = ANY($rowIDs)
    // COMMIT
}
```

- [ ] **Step 4: Re-run focused tests** — `PASS`.
- [ ] **Step 5: Commit**

```bash
git add postgres/relay.go postgres/relay_test.go
git commit -m "feat(memory-postgres): relay ClaimBatch with lease state and attempt_count"
```

### Task 8: Relay Ack with ownership predicate + `ErrLeaseLost`

**Files:**
- Modify: `llm-agent-memory-postgres/postgres/relay.go`
- Modify: `llm-agent-memory-postgres/postgres/relay_test.go`
- Modify: `llm-agent-memory-postgres/postgres/errors.go`

- [ ] **Step 1: Failing tests**

```go
func TestAck_SuccessPath(t *testing.T) {}                  // status='sent', sent_at set, lease cleared
func TestAck_RetryPath(t *testing.T) {}                    // status='pending', last_error set, attempt < max
func TestAck_FailedPath(t *testing.T) {}                   // status='failed' when attempt >= MaxAttempts
func TestAck_RejectsExpiredLease(t *testing.T) {}          // lease expired → 0 rows affected → ErrLeaseLost
func TestAck_RejectsStolenLease(t *testing.T) {}           // different workerID owns row → ErrLeaseLost
func TestAck_DoesNotMutateOnLeaseLost(t *testing.T) {}     // row state unchanged after ErrLeaseLost
```

- [ ] **Step 2: Verify failure**
- [ ] **Step 3: Implement**

Three UPDATE statements, each with `WHERE outbox_id=$1 AND claimed_by=$2 AND lease_expires_at > NOW()`. On zero rows-affected, return `ErrLeaseLost`. Add the sentinel to `errors.go`.

Determining "attempt reached max" requires reading the current `attempt_count` from the row OR computing it via comparison. Simpler: track in-memory in the claim batch (returned from ClaimBatch as part of ClaimedMessage shape).

- [ ] **Step 4: Re-run** — `PASS`.
- [ ] **Step 5: Commit**

```bash
git add postgres/relay.go postgres/relay_test.go postgres/errors.go
git commit -m "feat(memory-postgres): relay Ack with ownership predicate + ErrLeaseLost"
```

### Task 9: Relay RunOnce error collection (no early return)

**Files:**
- Modify: `llm-agent-memory-postgres/postgres/relay.go`
- Modify: `llm-agent-memory-postgres/postgres/relay_test.go`

- [ ] **Step 1: Failing tests**

```go
func TestRunOnce_ContinuesAfterAckFailure(t *testing.T) {} // first row fails Ack, second row still processed
func TestRunOnce_StatsLeaseLost(t *testing.T) {}           // RunStats.LeaseLost reflects ErrLeaseLost count
func TestRunOnce_AggregatesAckErrors(t *testing.T) {}      // multiple Ack errors → errors.Join
func TestRunOnce_PublishedCountedOnlyIfAckOK(t *testing.T) {} // Published++ requires both publish ok AND ack ok
```

- [ ] **Step 2: Verify failure**
- [ ] **Step 3: Implement**

Per spec §4.6 pseudocode. Add `LeaseLost int` field to `RunStats`.

- [ ] **Step 4: Re-run** — `PASS`.
- [ ] **Step 5: Commit**

```bash
git add postgres/relay.go postgres/relay_test.go
git commit -m "feat(memory-postgres): relay RunOnce continues on ack failure"
```

### Task 10: Relay `Release(ctx)` graceful shutdown

**Files:**
- Modify: `llm-agent-memory-postgres/postgres/relay.go`
- Modify: `llm-agent-memory-postgres/postgres/relay_test.go`

- [ ] **Step 1: Failing tests**

```go
func TestRelease_ClearsOwnedLeases(t *testing.T) {}       // rows with claimed_by=$workerID → pending, lease cleared
func TestRelease_DoesNotClearOtherWorkersLeases(t *testing.T) {}
func TestRelease_DoesNotResetAttemptCount(t *testing.T) {} // by design — claims count for retry budget
func TestRelease_NoOpOnFreshRelay(t *testing.T) {}        // no claimed rows → no error
```

- [ ] **Step 2: Verify failure**
- [ ] **Step 3: Implement**

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

- [ ] **Step 4: Re-run** — `PASS`.
- [ ] **Step 5: Commit**

```bash
git add postgres/relay.go postgres/relay_test.go
git commit -m "feat(memory-postgres): relay Release for graceful shutdown"
```

### Task 11: Operator API — `RequeueFailed` + `ListFailed`

**Files:**
- Modify: `llm-agent-memory-postgres/postgres/store.go`
- Modify: `llm-agent-memory-postgres/postgres/store_test.go`

- [ ] **Step 1: Failing tests**

```go
func TestRequeueFailed_ResetsToPending(t *testing.T) {}
func TestRequeueFailed_ResetsAttemptCountToZero(t *testing.T) {}
func TestRequeueFailed_NoOpOnNonFailedRow(t *testing.T) {}      // rows affected = 0
func TestListFailed_OrdersByCreatedAtDesc(t *testing.T) {}
func TestListFailed_RespectsLimit(t *testing.T) {}
```

- [ ] **Step 2: Verify failure**
- [ ] **Step 3: Implement**

```go
type RequeueResult struct { RowsAffected int64 }
type FailedOutboxRow struct {
    OutboxID     string
    AggregateID  string
    EventType    string
    AttemptCount int
    LastError    string
    CreatedAt    time.Time
}

func (s *Store) RequeueFailed(ctx context.Context, outboxID string) (RequeueResult, error) { /* ... */ }
func (s *Store) ListFailed(ctx context.Context, limit int) ([]FailedOutboxRow, error) { /* ... */ }
```

- [ ] **Step 4: Re-run** — `PASS`.
- [ ] **Step 5: Commit**

```bash
git add postgres/store.go postgres/store_test.go
git commit -m "feat(memory-postgres): add RequeueFailed/ListFailed operator API"
```

### Task 12: `LeaseAwarePublisher` test fake + integration coverage

**Files:**
- Create: `llm-agent-memory-postgres/postgres/lease_aware_publisher.go`
- Modify: `llm-agent-memory-postgres/postgres/relay_test.go`

- [ ] **Step 1: Failing tests**

```go
func TestLeaseAwarePublisher_HookInvoked(t *testing.T) {}
func TestLeaseAwarePublisher_NilHookActsLikeMemoryPublisher(t *testing.T) {}
// Then integration scenarios using the new fake:
func TestRelay_LeaseExpiresDuringPublish_AckReturnsLeaseLost(t *testing.T) {} // hook sleeps > LeaseTTL
func TestRelay_ConcurrentClaimAfterExpiry_PreservesAtMostOnceWithinClaim(t *testing.T) {}
```

The lease-expiry tests configure `LeaseTTL=100ms` and the hook sleeps 200ms; verify the worker's Ack returns `ErrLeaseLost`.

- [ ] **Step 2: Verify failure**
- [ ] **Step 3: Implement**

```go
type LeaseAwarePublisher struct {
    Events      []corememory.OutboxMessage
    PublishHook func(ctx context.Context, msg corememory.OutboxMessage) error
}

func (p *LeaseAwarePublisher) Publish(ctx context.Context, evt corememory.OutboxMessage) error {
    if p.PublishHook != nil {
        if err := p.PublishHook(ctx, evt); err != nil { return err }
    }
    p.Events = append(p.Events, evt)
    return nil
}
```

- [ ] **Step 4: Re-run** — `PASS` (with live Postgres; skip otherwise).
- [ ] **Step 5: Commit**

```bash
git add postgres/lease_aware_publisher.go postgres/relay_test.go
git commit -m "test(memory-postgres): add LeaseAwarePublisher for lease-expiry coverage"
```

### Task 13: Vector publisher dispatch — `memory_promoted` + `memory_dedupe_collapsed`

**Files:**
- Modify: `llm-agent-memory-gateway/internal/service/outbox_vector_publisher.go`
- Modify: `llm-agent-memory-gateway/internal/service/outbox_vector_publisher_test.go`

- [ ] **Step 1: Failing tests**

```go
func TestPublish_MemoryPromoted_NoProjection(t *testing.T) {}
func TestPublish_MemoryPromoted_EmitsPromotedNoopObservation(t *testing.T) {}
func TestPublish_MemoryDedupeCollapsed_NoProjection(t *testing.T) {}
func TestPublish_MemoryDedupeCollapsed_EmitsObservation(t *testing.T) {}
```

- [ ] **Step 2: Verify failure**
- [ ] **Step 3: Implement**

Add two `case` arms per spec §5.2. Both call `p.observe(ctx, msg, status, msg.Version, "")` and return nil. No `ProjectUpsert` / `ProjectRemove` call.

- [ ] **Step 4: Re-run** — `PASS`.
- [ ] **Step 5: Commit**

```bash
git add internal/service/outbox_vector_publisher.go internal/service/outbox_vector_publisher_test.go
git commit -m "feat(memory-gateway): handle memory_promoted + memory_dedupe_collapsed events"
```

### Task 14: Gateway composition — wire `Release(ctx)` + config knobs + docs

**Files:**
- Modify: `llm-agent-memory-gateway/internal/config/config.go`
- Modify: `llm-agent-memory-gateway/internal/config/config_test.go`
- Modify: `llm-agent-memory-gateway/cmd/memory-gateway/main.go`
- Modify: `llm-agent-memory-gateway/README.md`
- Modify: `llm-agent-memory-postgres/CHANGELOG.md`
- Modify: `llm-agent-memory-gateway/CHANGELOG.md`

- [ ] **Step 1: Failing tests**

```go
func TestLoadFromEnv_RelayLeaseTTLDefault(t *testing.T) {}
func TestLoadFromEnv_RelayLeaseTTLOverride(t *testing.T) {}
func TestLoadFromEnv_RelayMaxAttemptsDefault(t *testing.T) {}
func TestLoadFromEnv_RelayBatchSizeDefault(t *testing.T) {}
```

- [ ] **Step 2: Verify failure**
- [ ] **Step 3: Implement**

Add to `Config`:

```go
RelayLeaseTTL    time.Duration  // env LLM_AGENT_MEMORY_GATEWAY_RELAY_LEASE_TTL, default 180s
RelayMaxAttempts int            // env LLM_AGENT_MEMORY_GATEWAY_RELAY_MAX_ATTEMPTS, default 5
RelayBatchSize   int            // env LLM_AGENT_MEMORY_GATEWAY_RELAY_BATCH_SIZE, default 100
```

In `main.go`: construct `postgres.RelayConfig` from these knobs; pass through to `NewRelay`. In the existing shutdown sequence (`cleanupFns` LIFO from M7 Task 11), register `relay.Release(ctx)` BEFORE `pool.Close`.

Docs updates:
- `gateway/README.md`: new env vars table; deployment topology section (preStop, terminationGracePeriodSeconds, replica safety, worker ID rationale).
- `postgres/CHANGELOG.md`: M8a-prep entry covering migration framework v2, relay hardening, write-side allowlist, operator API.
- `gateway/CHANGELOG.md`: M8a-prep entry covering the two new vector-publisher dispatch arms.

- [ ] **Step 4: Run full module verification**

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-gateway
GOWORK=off GOCACHE=/tmp/go-build go test ./... -count=1
GOWORK=off GOCACHE=/tmp/go-build go test ./... -race -count=3

cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-postgres
GOWORK=off GOCACHE=/tmp/go-build go test ./... -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go cmd/memory-gateway/main.go README.md CHANGELOG.md
cd ../llm-agent-memory-postgres && git add CHANGELOG.md
git commit -m "feat(memory-gateway): wire relay config + Release in shutdown; update docs"
```

---

## Self-Review

- All 9 spec acceptance criteria from M8a-prep v2.1 §8 are covered:
  1. Ack ownership check → Task 8.
  2. DDL escape hatch (`Transactional bool`) → Task 1 (type), Task 2 (runtime).
  3. Write-side event-type allowlist → Tasks 4 + 5.
  4. Reversed `memory_dedupe_collapsed` payload convention → Task 13 (consumer side); emission convention enforced at M8a sub-spec (Promoter/Deduper) time.
  5. `Release(ctx)` graceful shutdown → Task 10 + Task 14 (wiring).
  6. `RequeueFailed`/`ListFailed` operator API → Task 11.
  7. Lease TTL 180s default → Task 7 (config) + Task 14 (env var default).
  8. Cross-tenant fairness deferral → documented in README (Task 14).
  9. Postgres 11+ minimum → README + CHANGELOG (Task 14).
- All 4 W-7 impl-plan items from spec v2.1 §10 are TDD'd:
  - Hostname-failure fallback → Task 6.
  - Test fixture audit for unknown EventType literals → Task 5.
  - `Release` does NOT decrement `attempt_count` → Task 10.
  - Fresh-DB bootstrap implementation contract → Task 2.
- All 14 tasks have a Step 1 with failing tests (strict TDD).
- All tasks commit atomically per the M7 plan discipline.
- No SDK changes — confirmed across all tasks.
- No new sibling module — M8a-prep stays in `llm-agent-memory-postgres` + `llm-agent-memory-gateway` modules.

Plan complete. Two execution options at execute-time:

1. **Subagent-driven** (recommended): dispatch fresh subagent per task, review between, fast iteration. Mirrors M7's execution shape.
2. **Inline execution**: execute tasks in-session using `executing-plans`.

Choose at execute-time. Spec + plan are now both on `main`; sub-milestone ready for implementation.
