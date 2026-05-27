# M7 Validation Telemetry + Decision Trace — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the narrow M7 scope per `docs/superpowers/specs/2026-05-27-m7-workers-and-validation-design.md`: persist decision traces via a best-effort async Postgres sink, and emit the 10-counter measurable subset of the v1 validation metric set. Zero SDK changes; one Postgres migration; gateway-only code changes otherwise.

**Architecture:** A new `memory_decision_trace` table is added by extending the existing `Store.migrationStatements()` slice in `llm-agent-memory-postgres`. Inside the gateway, a new `DecisionTraceSink` implementation (just another `service.TraceEmitter`) is composed with the existing metrics emitter through the existing `observability.ComposeTraceEmitters` helper — so the sink picks up every `traceEmitter.Emit` call site without changing call-site shape. A bounded channel + writer goroutine batch-insert rows; overflow and DB errors increment a `trace_dropped_total` counter rather than blocking the request path. A separate periodic goroutine snapshots storage bytes. Ten new counters land on the existing `*observability.Metrics` struct via the same atomic-int + text-format exporter the gateway already ships.

**Tech Stack:** Go 1.26.0; stdlib `context`, `database/sql`-equivalent via `github.com/jackc/pgx/v5`, `encoding/json`, `time`, `sync/atomic`, `testing`, `net/http/httptest`; `github.com/costa92/llm-agent-memory/memory` (read-only — no SDK change); `github.com/costa92/llm-agent-memory-postgres/postgres`; existing workspace `go.work`.

---

## File Structure

| Path | Action | Responsibility |
|---|---|---|
| `llm-agent-memory-postgres/postgres/schema.go` | Modify | Append `memory_decision_trace` `CREATE TABLE IF NOT EXISTS` to `migrationStatements()`; add the three minimum indexes |
| `llm-agent-memory-postgres/postgres/schema_test.go` | Modify | Assert migration creates `memory_decision_trace` and the three indexes; idempotency re-run |
| `llm-agent-memory-postgres/CHANGELOG.md` | Modify | Note schema addition for M7 |
| `llm-agent-memory-gateway/internal/service/trace_sink.go` | Create | `DecisionTraceSink` interface + `TraceRow` value type + `nopDecisionTraceSink` |
| `llm-agent-memory-gateway/internal/service/trace_sink_test.go` | Create | Contract tests against the nop sink + interface assertion |
| `llm-agent-memory-gateway/internal/service/trace_sink_postgres.go` | Create | Bounded-channel async sink with writer goroutine + batch insert + retry-with-backoff |
| `llm-agent-memory-gateway/internal/service/trace_sink_postgres_test.go` | Create | TDD for sink: enqueue/drain semantics; overflow drops; batch flush; shutdown drain |
| `llm-agent-memory-gateway/internal/service/storage_metrics_cron.go` | Create | Periodic goroutine; per-tenant-bucket aggregate from Postgres; emits `memory/vector_storage_bytes_total` |
| `llm-agent-memory-gateway/internal/service/storage_metrics_cron_test.go` | Create | TDD for cron tick + failure backoff + shutdown |
| `llm-agent-memory-gateway/internal/service/tenant_bucket.go` | Create | `tenantBucket(tenantID string) string` — stable FNV-32 hash mod 32 |
| `llm-agent-memory-gateway/internal/service/tenant_bucket_test.go` | Create | TDD: stability, uniform-enough distribution sanity, fixed-modulus contract |
| `llm-agent-memory-gateway/internal/observability/metrics.go` | Modify | Add 10 new counters + `Snapshot` fields + emission methods + text-format rows |
| `llm-agent-memory-gateway/internal/observability/metrics_test.go` | Modify | TDD per counter: Snapshot reflects increments; exposition contains the expected lines |
| `llm-agent-memory-gateway/internal/observability/trace_sink_emitter.go` | Create | Thin adapter: `service.TraceEmitter` → `DecisionTraceSink.Record(...)`. Lives in observability/ to avoid an import cycle with the sink construction code |
| `llm-agent-memory-gateway/internal/observability/trace_sink_emitter_test.go` | Create | TDD: emitter forwards every `Emit` to `Record` with correct stage + tenant extraction |
| `llm-agent-memory-gateway/internal/config/config.go` | Modify | Add knobs: `TraceSinkBufferSize`, `TraceSinkBatchSize`, `TraceSinkShutdownTimeout`, `StorageMetricsInterval`, `TenantBucketModulus`, `TraceRetentionEnabled` |
| `llm-agent-memory-gateway/internal/config/config_test.go` | Modify | TDD env parsing for the new knobs |
| `llm-agent-memory-gateway/cmd/memory-gateway/main.go` | Modify | Open second pool (or reuse) for sink; construct sink + cron; compose into existing TraceEmitter chain; start/stop on lifecycle |
| `llm-agent-memory-gateway/cmd/memory-gateway/main_test.go` | Modify | Compile/config smoke: command builds with new knobs and shuts down cleanly |
| `llm-agent-memory-gateway/internal/service/cross_tenant_test.go` | Create | Property test: forged `tenant_id` cannot read another tenant's trace rows or cause cross-tenant counter bleed |
| `llm-agent-memory-gateway/README.md` | Modify | Document the new env knobs, the 10 counters, the trace table |
| `llm-agent-memory-gateway/doc.go` | Modify | One-line update mentioning persisted decision traces |
| `llm-agent-memory-gateway/CHANGELOG.md` | Modify (or create) | M7 entry: trace persistence + 10 counters; no breaking changes |

## Open Decisions Locked For This Plan

- **Sink composition path:** `observability.ComposeTraceEmitters(existingMetricsEmitter, traceSinkAdapter)` is the wiring point. The sink is *another* `TraceEmitter` — no call-site changes anywhere in `internal/service/*.go`.
- **Sink storage:** the sink writes to the same Postgres database that `llm-agent-memory-postgres.Store` already uses. We reuse the same connection pool. No second DSN.
- **Failure policy** (locks the spec §5.2 commitment):
  - Bounded `chan TraceRow` of capacity `TraceSinkBufferSize` (default 1024).
  - On full channel: non-blocking send fails; increment `trace_dropped_total{reason="buffer_full"}`.
  - Writer batches up to `TraceSinkBatchSize` (default 50) per `INSERT`.
  - On insert error: retry with exponential backoff up to 3 attempts; then `trace_dropped_total{reason="db_error"}` per row in batch.
  - On shutdown: drain channel for up to `TraceSinkShutdownTimeout` (default 5s); remaining rows → `trace_dropped_total{reason="shutdown"}`.
- **Reason column at v1:** free-form `text NOT NULL`. The §13.4 enum is M8 work. Plan does not enforce a value set.
- **Tenant extraction in the sink:** the sink reads `tenant_id` from the `fields` map (whatever the call site put there); if absent, falls back to context (if a context-key helper exists) or persists the row with empty tenant + `request_id NULL`. The plan does *not* add new fields to existing `Emit` call sites — that's an M8 concern when the typed observer lands.
- **Cron interval:** default 5 minutes; configurable via env. Plan does not implement retention (`TraceRetentionEnabled` is a config knob defaulted to `false`; OD-5 from the spec — operator obligation).
- **Tenant bucket:** FNV-32a hash of `tenant_id` mod 32. Plan reuses `hash/fnv` from stdlib — no new dep.
- **No new module.** Everything except the migration lives inside `llm-agent-memory-gateway/`.
- **No SDK changes.** If a task discovers an SDK change is required, stop and escalate to the spec — that's a sign the task is reaching into deferred-to-M8 work.
- **Cardinality:** the 10 counters carry only `tenant_bucket` as a label dimension. The `trace_dropped_total` counter carries `reason` (3 values) — that's the *one* legitimate non-tenant_bucket label, justified because it's a dropped-trace observability counter and the value set is bounded at compile time.

## Sequencing Rules

- **Strict TDD.** Every task starts from a failing test.
- **Postgres-touching tasks** must work in two modes: (a) unit-test path that uses the existing `dockertest`-style integration harness behind `-tags=integration`; (b) compile-and-shape verification path that runs without a database. Default CI invocation `GOWORK=off go test ./... -count=1` must stay green without a live Postgres.
- **Gateway-only ownership after Task 1.** No edits to `llm-agent-memory` (the SDK). If you find yourself needing one — stop.
- **No call-site changes to `s.traceEmitter.Emit(...)` in `internal/service/service.go`.** The sink picks up traces via composition only.
- **Cardinality discipline.** Every metric Add path runs through `tenantBucket(tenantID)` for its label dimension. No raw `tenant_id` labels.
- **Endpoint rollout order doesn't apply here** — this plan has no new HTTP endpoints. The order below follows the dependency graph: schema → sink → wiring → counters → cron → composition → docs.

---

## Task Plan

### Task 1: Add `memory_decision_trace` migration in postgres backend

**Files:**
- Modify: `llm-agent-memory-postgres/postgres/schema.go`
- Modify: `llm-agent-memory-postgres/postgres/schema_test.go`
- Modify: `llm-agent-memory-postgres/CHANGELOG.md`

- [ ] **Step 1: Write failing migration tests**

Add to `schema_test.go`:

```go
func TestMigrate_CreatesDecisionTraceTable(t *testing.T) { /* table exists after Migrate */ }
func TestMigrate_DecisionTraceIndexes(t *testing.T)     { /* idx_trace_tenant_time, idx_trace_request, idx_trace_stage_reason all present */ }
func TestMigrate_DecisionTraceIdempotent(t *testing.T)   { /* second Migrate is a no-op */ }
```

Tests follow the existing pattern in `schema_test.go`: call `openTestPool(t, ctx)` which `t.Skipf`s when `LLM_AGENT_MEMORY_PG_URL` is unset. Use a unique `TablePrefix` per test (e.g. `m7_<nanos>_trace`) so parallel runs don't collide. No build tags involved.

- [ ] **Step 2: Run tests, verify they fail**

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-postgres
GOWORK=off GOCACHE=/tmp/go-build go test ./postgres -run 'TestMigrate_DecisionTrace' -count=1
```

Expected without `LLM_AGENT_MEMORY_PG_URL` set: test skips (existing harness behavior). To verify failure locally, export a Postgres URL first (`export LLM_AGENT_MEMORY_PG_URL=postgres://...`). CI runs with the URL set and catches real failures.

- [ ] **Step 3: Implement the migration**

Append to `Store.migrationStatements()`:

```sql
CREATE TABLE IF NOT EXISTS memory_decision_trace (
  trace_id     uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id    text        NOT NULL,
  request_id   text        NULL,
  stage        text        NOT NULL,
  reason       text        NOT NULL,
  memory_id    text        NULL,
  version      bigint      NULL,
  emitted_at   timestamptz NOT NULL DEFAULT now(),
  emitter      text        NOT NULL,
  payload      jsonb       NULL
)
```

Then three index statements (separate elements in the slice):

```sql
CREATE INDEX IF NOT EXISTS idx_trace_tenant_time   ON memory_decision_trace (tenant_id, emitted_at DESC);
CREATE INDEX IF NOT EXISTS idx_trace_request       ON memory_decision_trace (request_id) WHERE request_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_trace_stage_reason  ON memory_decision_trace (stage, reason);
```

Add a column comment via `COMMENT ON COLUMN memory_decision_trace.reason IS 'free-form in v1.x (M7); enum frozen in v2 (M8)'` as a fourth statement.

- [ ] **Step 4: Re-run focused tests**

Expected: `PASS`. Also re-run the existing `schema_test.go` suite to confirm no regressions.

- [ ] **Step 5: Commit**

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-postgres
git add postgres/schema.go postgres/schema_test.go CHANGELOG.md
git commit -m "feat(memory-postgres): add memory_decision_trace table for M7"
```

### Task 2: Declare `DecisionTraceSink` interface + `TraceRow` + noop

**Files:**
- Create: `llm-agent-memory-gateway/internal/service/trace_sink.go`
- Create: `llm-agent-memory-gateway/internal/service/trace_sink_test.go`

- [ ] **Step 1: Write failing contract tests**

```go
func TestDecisionTraceSink_NopRecordReturnsNil(t *testing.T) {}
func TestDecisionTraceSink_NopAcceptsEmptyRow(t *testing.T)  {}
func TestDecisionTraceSink_InterfaceShape(t *testing.T)       {} // compile-time assertion
```

- [ ] **Step 2: Verify failure**

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-gateway
GOWORK=off GOCACHE=/tmp/go-build go test ./internal/service -run 'TestDecisionTraceSink' -count=1
```

- [ ] **Step 3: Implement interface + types + nop**

```go
package service

import (
    "context"
    "time"
)

type TraceRow struct {
    TenantID  string
    RequestID string
    Stage     string
    Reason    string
    MemoryID  string
    Version   int64
    EmittedAt time.Time
    Emitter   string
    Payload   map[string]any
}

type DecisionTraceSink interface {
    Record(ctx context.Context, row TraceRow) error
}

type nopDecisionTraceSink struct{}

func (nopDecisionTraceSink) Record(context.Context, TraceRow) error { return nil }

func NewNopDecisionTraceSink() DecisionTraceSink { return nopDecisionTraceSink{} }
```

- [ ] **Step 4: Re-run focused tests** — expect `PASS`.
- [ ] **Step 5: Commit**

```bash
git add internal/service/trace_sink.go internal/service/trace_sink_test.go
git commit -m "feat(memory-gateway): declare DecisionTraceSink seam"
```

### Task 3: Postgres-backed async sink with bounded channel

**Files:**
- Create: `llm-agent-memory-gateway/internal/service/trace_sink_postgres.go`
- Create: `llm-agent-memory-gateway/internal/service/trace_sink_postgres_test.go`

- [ ] **Step 1: Write failing tests**

Cover:

- `Record` accepts a row under capacity; writer drains it.
- `Record` non-blocking under load: when channel full, returns nil but increments a `Dropped()` counter.
- Batch insert flushes when batch size reached or after a tick.
- Insert error retries with backoff (use an injectable `insertFunc` for deterministic test).
- `Stop(ctx)` drains within shutdown timeout; remaining rows counted as `dropped{reason="shutdown"}`.

Use a fake `insertFunc func(ctx context.Context, rows []TraceRow) error` injected via the constructor so tests don't need a live database. The Postgres-specific insert wiring lives in a thin closure constructed in `cmd/memory-gateway/main.go` (Task 11).

- [ ] **Step 2: Verify failure**

```bash
GOWORK=off GOCACHE=/tmp/go-build go test ./internal/service -run 'TestPostgresDecisionTraceSink' -count=1
```

- [ ] **Step 3: Implement**

Shape:

```go
type PostgresDecisionTraceSink struct {
    insertFunc      func(ctx context.Context, rows []TraceRow) error
    ch              chan TraceRow
    batchSize       int
    flushInterval   time.Duration
    shutdownTimeout time.Duration

    droppedBufferFull atomic.Uint64
    droppedDBError    atomic.Uint64
    droppedShutdown   atomic.Uint64

    done chan struct{}
}

func NewPostgresDecisionTraceSink(cfg PostgresDecisionTraceSinkConfig) *PostgresDecisionTraceSink { ... }
func (s *PostgresDecisionTraceSink) Run(ctx context.Context)                                       { ... } // writer goroutine
func (s *PostgresDecisionTraceSink) Record(ctx context.Context, row TraceRow) error                { ... } // non-blocking send
func (s *PostgresDecisionTraceSink) Stop(ctx context.Context)                                       { ... }
func (s *PostgresDecisionTraceSink) DroppedSnapshot() TraceDroppedSnapshot                          { ... }
```

`Record` uses `select { case s.ch <- row: default: s.droppedBufferFull.Add(1) }`.

The retry loop: 3 attempts, sleep 50ms / 200ms / 800ms (jitter optional). On final failure, add `len(batch)` to `droppedDBError` and discard.

- [ ] **Step 4: Re-run focused tests** — expect `PASS`.
- [ ] **Step 5: Commit**

```bash
git add internal/service/trace_sink_postgres.go internal/service/trace_sink_postgres_test.go
git commit -m "feat(memory-gateway): add bounded-channel postgres decision-trace sink"
```

### Task 4: TraceEmitter adapter that forwards into the sink

**Files:**
- Create: `llm-agent-memory-gateway/internal/observability/trace_sink_emitter.go`
- Create: `llm-agent-memory-gateway/internal/observability/trace_sink_emitter_test.go`

- [ ] **Step 1: Write failing tests**

Cover:

- Adapter implements `service.TraceEmitter`.
- For each input `Emit(ctx, "recalled", fields)`, the adapter constructs a `TraceRow` with `Stage="recalled"`, extracts `tenant_id` / `request_id` / `memory_id` / `version` / `reason` from `fields` if present, copies the rest into `Payload`.
- `EmittedAt` is populated from a clock injection (deterministic test).
- `Emitter` value is the constructor arg (e.g. `"gateway"`).

- [ ] **Step 2: Verify failure**

```bash
GOWORK=off GOCACHE=/tmp/go-build go test ./internal/observability -run 'TestTraceSinkEmitter' -count=1
```

- [ ] **Step 3: Implement**

```go
type TraceSinkEmitter struct {
    sink    service.DecisionTraceSink
    emitter string
    now     func() time.Time
}

func NewTraceSinkEmitter(sink service.DecisionTraceSink, emitter string, now func() time.Time) *TraceSinkEmitter { ... }

func (e *TraceSinkEmitter) Emit(ctx context.Context, stage string, fields map[string]any) {
    row := service.TraceRow{Stage: stage, EmittedAt: e.now(), Emitter: e.emitter, Payload: make(map[string]any, len(fields))}
    for k, v := range fields {
        switch k {
        case "tenant_id":   row.TenantID, _   = v.(string)
        case "request_id":  row.RequestID, _  = v.(string)
        case "memory_id":   row.MemoryID, _   = v.(string)
        case "version":     row.Version, _    = v.(int64)
        case "reason":      row.Reason, _     = v.(string)
        case "mode":        if row.Reason == "" { row.Reason, _ = v.(string) } // back-compat: existing promote_decided emissions use "mode"
        default:            row.Payload[k] = v
        }
    }
    _ = e.sink.Record(ctx, row)
}
```

The `mode` → `Reason` fallback is the only call-site-shape concession — it preserves existing promote_decided trace content under the new schema without changing `service.go`.

- [ ] **Step 4: Re-run focused tests** — expect `PASS`.
- [ ] **Step 5: Commit**

```bash
git add internal/observability/trace_sink_emitter.go internal/observability/trace_sink_emitter_test.go
git commit -m "feat(memory-gateway): adapt DecisionTraceSink into the TraceEmitter chain"
```

### Task 5: `tenant_bucket` helper

**Files:**
- Create: `llm-agent-memory-gateway/internal/service/tenant_bucket.go`
- Create: `llm-agent-memory-gateway/internal/service/tenant_bucket_test.go`

- [ ] **Step 1: Failing tests**

```go
func TestTenantBucket_Stability(t *testing.T) {} // same input → same bucket across calls
func TestTenantBucket_Modulus(t *testing.T)   {} // outputs ∈ ["00".."31"]
func TestTenantBucket_DistributionSanity(t *testing.T) {} // 1000 random IDs hit ≥20 distinct buckets
func TestTenantBucket_EmptyTenantID(t *testing.T) {} // empty string → "unknown" bucket
```

- [ ] **Step 2: Verify failure**

```bash
GOWORK=off GOCACHE=/tmp/go-build go test ./internal/service -run 'TestTenantBucket' -count=1
```

- [ ] **Step 3: Implement**

```go
const TenantBucketModulus = 32

func tenantBucket(tenantID string) string {
    if tenantID == "" {
        return "unknown"
    }
    h := fnv.New32a()
    _, _ = h.Write([]byte(tenantID))
    return fmt.Sprintf("%02d", h.Sum32()%TenantBucketModulus)
}
```

Lowercase identifier — package-private. Exported wrapper not needed in M7.

- [ ] **Step 4: Re-run focused tests** — expect `PASS`.
- [ ] **Step 5: Commit**

```bash
git add internal/service/tenant_bucket.go internal/service/tenant_bucket_test.go
git commit -m "feat(memory-gateway): add tenant_bucket hashing for metric labels"
```

### Task 6: 10 counters in `observability.Metrics`

**Files:**
- Modify: `llm-agent-memory-gateway/internal/observability/metrics.go`
- Modify: `llm-agent-memory-gateway/internal/observability/metrics_test.go`

- [ ] **Step 1: Failing tests**

For each of the 10 counters, write `TestMetrics_AddXxx_AppearsInSnapshot` and `TestMetrics_HandlerExposes_Xxx`. The 10 counters:

1. `embedding_request_total`
2. `embedding_applied_total`
3. `embedding_tokens_total`
4. `embedding_cost_total`
5. `memory_storage_bytes_total`
6. `vector_storage_bytes_total`
7. `episodic_disabled_total`
8. `episodic_deleted_total`
9. `recall_returned_total`
10. `recall_selected_total`

Plus the dropped counter (with `reason` label):

11. `trace_dropped_total{reason="buffer_full"|"db_error"|"shutdown"}`

All counters carry `tenant_bucket` label except `trace_dropped_total` (which uses `reason` instead — see Open Decisions cardinality note).

Each `TestMetrics_HandlerExposes_*` asserts the handler's text body contains the expected metric-name line for the bucket.

- [ ] **Step 2: Verify failure**

```bash
GOWORK=off GOCACHE=/tmp/go-build go test ./internal/observability -run 'TestMetrics_(Add|Handler)' -count=1
```

- [ ] **Step 3: Implement counters + snapshot + text exposition**

Mirror the existing pattern: package-private `atomic.Uint64` fields, `Add*` methods that take `tenantBucket string` (or `reason string` for `trace_dropped_total`), `Snapshot` exposes a `map[bucket]uint64` per counter, `Handler()` emits one line per (counter, bucket) entry in standard Prometheus text format.

Counter Add signatures:

```go
func (m *Metrics) AddEmbeddingRequest(tenantBucket string)           { ... }
func (m *Metrics) AddEmbeddingApplied(tenantBucket string)           { ... }
func (m *Metrics) AddEmbeddingTokens(tenantBucket string, n uint64)  { ... }
func (m *Metrics) AddEmbeddingCost(tenantBucket string, micro uint64){ ... } // cost in micro-units to keep integer
func (m *Metrics) SetMemoryStorageBytes(tenantBucket string, b uint64)  { ... } // gauge-like: SET not ADD (cron snapshot)
func (m *Metrics) SetVectorStorageBytes(tenantBucket string, b uint64)  { ... } // same
func (m *Metrics) AddEpisodicDisabled(tenantBucket string)           { ... }
func (m *Metrics) AddEpisodicDeleted(tenantBucket string)            { ... }
func (m *Metrics) AddRecallReturned(tenantBucket string, n uint64)   { ... }
func (m *Metrics) AddRecallSelected(tenantBucket string, n uint64)   { ... }
func (m *Metrics) AddTraceDropped(reason string, n uint64)           { ... }
```

Use `sync.Map` or `map[string]*atomic.Uint64` guarded by a `sync.RWMutex` for the per-bucket counters. The hot path stays lock-free for an existing bucket entry.

- [ ] **Step 4: Re-run focused tests** — expect `PASS`.
- [ ] **Step 5: Commit**

```bash
git add internal/observability/metrics.go internal/observability/metrics_test.go
git commit -m "feat(memory-gateway): add 10 M7 validation counters + trace dropped counter"
```

### Task 7: Storage-bytes cron

**Files:**
- Create: `llm-agent-memory-gateway/internal/service/storage_metrics_cron.go`
- Create: `llm-agent-memory-gateway/internal/service/storage_metrics_cron_test.go`

- [ ] **Step 1: Failing tests**

Cover:

- Cron calls the injected `query` function on every tick.
- Result rows update the metrics via `SetMemoryStorageBytes` / `SetVectorStorageBytes` keyed on `tenant_bucket`.
- Query failure increments `storage_cron_failures_total` and skips that tick; next tick still runs.
- `Stop(ctx)` cancels the ticker promptly.

Inject a `query func(ctx context.Context) ([]storageRow, error)` and a clock — no Postgres needed in test.

- [ ] **Step 2: Verify failure**

```bash
GOWORK=off GOCACHE=/tmp/go-build go test ./internal/service -run 'TestStorageMetricsCron' -count=1
```

- [ ] **Step 3: Implement**

```go
type storageRow struct {
    TenantBucket string
    MemoryBytes  uint64
    VectorBytes  uint64
}

type StorageMetricsCron struct {
    query    func(ctx context.Context) ([]storageRow, error)
    metrics  *observability.Metrics
    interval time.Duration
    now      func() time.Time
    done     chan struct{}
}

func (c *StorageMetricsCron) Run(ctx context.Context) { ... }
func (c *StorageMetricsCron) Stop(ctx context.Context) { ... }
```

The Postgres-specific query closure is constructed in `cmd/memory-gateway/main.go` (Task 11). It SHOULD select `tenant_id, sum(octet_length(content)) AS memory_bytes, sum(octet_length(vector::text)) AS vector_bytes` grouped by `tenant_id`, then aggregate to `tenant_bucket` in Go.

- [ ] **Step 4: Re-run focused tests** — expect `PASS`.
- [ ] **Step 5: Commit**

```bash
git add internal/service/storage_metrics_cron.go internal/service/storage_metrics_cron_test.go
git commit -m "feat(memory-gateway): add periodic storage-bytes metric reader"
```

### Task 8: Wire `episodic_disabled_total` + `episodic_deleted_total` from outbox events

**Files:**
- Modify: `llm-agent-memory-gateway/internal/service/outbox_observer.go`
- Modify: `llm-agent-memory-gateway/internal/service/outbox_observer_test.go` (create if not present)

- [ ] **Step 1: Failing tests**

Cover:

- Observing an outbox `memory_disabled` event increments `episodic_disabled_total{tenant_bucket=...}`.
- Observing `memory_deleted` increments `episodic_deleted_total{tenant_bucket=...}`.
- Observing any other event type does **not** increment either.
- `tenant_bucket` derives from the event's `tenant_id` via `tenantBucket(...)`.

- [ ] **Step 2: Verify failure**

```bash
GOWORK=off GOCACHE=/tmp/go-build go test ./internal/service -run 'TestOutboxObserver_(Disabled|Deleted|Other)' -count=1
```

- [ ] **Step 3: Implement**

Extend the existing `OutboxProjectionObserver` impl (or add a parallel observer wired through `ComposeOutboxObservers` if that helper exists) to inspect the event type and call the right `metrics.Add*` method. **Do not** branch on `tenant_id` directly — always bucket.

- [ ] **Step 4: Re-run focused tests** — expect `PASS`.
- [ ] **Step 5: Commit**

```bash
git add internal/service/outbox_observer.go internal/service/outbox_observer_test.go
git commit -m "feat(memory-gateway): emit episodic_disabled/deleted lifecycle counters"
```

### Task 9: Wire `recall_returned_total` + `recall_selected_total` from recall path

**Files:**
- Modify: `llm-agent-memory-gateway/internal/service/recall_observer.go`
- Modify: `llm-agent-memory-gateway/internal/service/recall_observer_test.go` (create if not present)

- [ ] **Step 1: Failing tests**

Cover:

- After a recall returns N hits, `recall_returned_total{tenant_bucket}` increases by N.
- After post-budget filtering selects M ≤ N hits, `recall_selected_total{tenant_bucket}` increases by M.
- Zero-hit recall increments neither.
- The counter increments use `tenantBucket(observation.TenantID)`, not the raw tenant id.

- [ ] **Step 2: Verify failure**

```bash
GOWORK=off GOCACHE=/tmp/go-build go test ./internal/service -run 'TestRecallObserver_(Returned|Selected|Zero)' -count=1
```

- [ ] **Step 3: Implement**

Extend the existing `recallMetricsObserver.ObserveRecall(...)` to derive `Returned`/`Selected` counts from the observation struct. If the current `RecallObservation` does not carry these counts, **stop** — that means M7's narrow scope was wrong about the observer carrying enough signal. Plan-stage escalation: re-read spec §4 to confirm `recall_returned_total` is still in the in-scope row; if yes, then a narrow extension to `RecallObservation` is legitimate (it doesn't add `memory_id` or `request_id`, just counts). If the extension would require breaking changes — escalate.

- [ ] **Step 4: Re-run focused tests** — expect `PASS`.
- [ ] **Step 5: Commit**

```bash
git add internal/service/recall_observer.go internal/service/recall_observer_test.go
git commit -m "feat(memory-gateway): emit recall_returned/selected counters"
```

### Task 10: Wire embedding cost-class counters

**Files:**
- Modify: existing embedding-call sites in `llm-agent-memory-gateway/internal/service/` (likely in `vector_projector.go`, `outbox_vector_publisher.go`, or wherever `Embedder.Embed` is called)
- Modify: corresponding `_test.go` files

- [ ] **Step 1: Identify embedding call sites**

```bash
grep -rn "Embed\b\|Embedder\." llm-agent-memory-gateway/internal/service --include='*.go' | grep -v _test
```

- [ ] **Step 2: Failing tests at each call site**

For each call site, write a test that:

- Stubs `Embedder.Embed` returning `(vec, tokens=N, err=nil)`.
- Asserts `embedding_request_total{tenant_bucket}` incremented by 1.
- Asserts `embedding_applied_total{tenant_bucket}` incremented by 1.
- Asserts `embedding_tokens_total{tenant_bucket}` incremented by N.
- Asserts `embedding_cost_total{tenant_bucket}` incremented by `costPerToken * N` (using a configurable rate; default 0 if not set).
- On error, only `embedding_request_total` increments — `applied/tokens/cost` do not.

- [ ] **Step 3: Verify failure**

```bash
GOWORK=off GOCACHE=/tmp/go-build go test ./internal/service -run 'TestEmbedding(Request|Applied|Tokens|Cost)' -count=1
```

- [ ] **Step 4: Implement**

Add `metrics.AddEmbedding*` calls around each embedding call site. The cost rate should come from a new config knob (`EmbeddingCostMicrosPerToken`, default 0). Use micro-units to keep counters integer.

- [ ] **Step 5: Re-run focused tests** — expect `PASS`.
- [ ] **Step 6: Commit**

```bash
git add internal/service/*.go internal/service/*_test.go internal/config/config.go internal/config/config_test.go
git commit -m "feat(memory-gateway): emit cost-class embedding counters"
```

### Task 11: Compose sink + cron into command + config knobs

**Files:**
- Modify: `llm-agent-memory-gateway/internal/config/config.go`
- Modify: `llm-agent-memory-gateway/internal/config/config_test.go`
- Modify: `llm-agent-memory-gateway/cmd/memory-gateway/main.go`
- Modify: `llm-agent-memory-gateway/cmd/memory-gateway/main_test.go`

- [ ] **Step 1: Failing config tests**

```go
func TestLoadFromEnv_TraceSinkBufferSizeDefault(t *testing.T) {}
func TestLoadFromEnv_TraceSinkBatchSizeDefault(t *testing.T)  {}
func TestLoadFromEnv_StorageMetricsIntervalDefault(t *testing.T) {}
func TestLoadFromEnv_EmbeddingCostMicrosPerTokenDefault(t *testing.T) {}
func TestLoadFromEnv_TraceRetentionEnabledDefault(t *testing.T) {} // default false
```

- [ ] **Step 2: Verify failure**

```bash
GOWORK=off GOCACHE=/tmp/go-build go test ./internal/config -run 'TestLoadFromEnv_(Trace|Storage|Embedding)' -count=1
```

- [ ] **Step 3: Implement config + main wiring**

Add to `Config`:

```go
TraceSinkBufferSize        int           // env LLM_AGENT_MEMORY_GATEWAY_TRACE_BUFFER, default 1024
TraceSinkBatchSize         int           // env LLM_AGENT_MEMORY_GATEWAY_TRACE_BATCH, default 50
TraceSinkShutdownTimeout   time.Duration // env LLM_AGENT_MEMORY_GATEWAY_TRACE_SHUTDOWN, default 5s
StorageMetricsInterval     time.Duration // env LLM_AGENT_MEMORY_GATEWAY_STORAGE_INTERVAL, default 5m
EmbeddingCostMicrosPerToken uint64        // env LLM_AGENT_MEMORY_GATEWAY_EMBED_COST_MICROS, default 0
TraceRetentionEnabled      bool          // env LLM_AGENT_MEMORY_GATEWAY_TRACE_RETENTION, default false
```

In `main.go`:

1. After opening the pgx pool, construct the sink's `insertFunc` closure (single `INSERT ... VALUES (...)` per batch).
2. Construct `cron`'s `query` closure (the `SELECT ... GROUP BY tenant_id` query).
3. `sink := service.NewPostgresDecisionTraceSink(...)` ; `cron := service.NewStorageMetricsCron(...)` ; `traceSinkEmitter := observability.NewTraceSinkEmitter(sink, "gateway", time.Now)`.
4. `composedEmitter := observability.ComposeTraceEmitters(metrics.TraceEmitter(), traceSinkEmitter)`.
5. Start sink + cron in goroutines bound to a shared `errgroup` or context.
6. On shutdown signal, call `sink.Stop(timeoutCtx)` and `cron.Stop(timeoutCtx)` before closing the pool.

- [ ] **Step 4: Run full module tests**

```bash
GOWORK=off GOCACHE=/tmp/go-build go test ./... -count=1
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go cmd/memory-gateway/main.go cmd/memory-gateway/main_test.go
git commit -m "feat(memory-gateway): wire trace sink + storage cron into gateway command"
```

### Task 12: Cross-tenant isolation property test + docs + module verification

**Files:**
- Create: `llm-agent-memory-gateway/internal/service/cross_tenant_test.go`
- Modify: `llm-agent-memory-gateway/README.md`
- Modify: `llm-agent-memory-gateway/doc.go`
- Modify or Create: `llm-agent-memory-gateway/CHANGELOG.md`

- [ ] **Step 1: Failing cross-tenant test**

```go
func TestCrossTenant_TraceRowsIsolated(t *testing.T) {
    // Write a trace row for tenant_a; query for tenant_b returns none.
}
func TestCrossTenant_StorageMetricsIsolated(t *testing.T) {
    // Storage-bytes cron groups by tenant; bucket A's value does not bleed into bucket B's value.
}
func TestCrossTenant_CounterBucketingStable(t *testing.T) {
    // Forging a different tenant_id with the same bucket should not corrupt the counter for the legitimate tenant.
}
```

The third test asserts the *bucket scheme* doesn't accidentally enable a forgery channel — two tenants in the same bucket share a metric, but neither can read the underlying tenant_id back from the counter. (Documenting the privacy guarantee: bucket aggregation is one-way.)

- [ ] **Step 2: Verify failure**

```bash
GOWORK=off GOCACHE=/tmp/go-build go test ./internal/service -run 'TestCrossTenant' -count=1
```

- [ ] **Step 3: Implement**

The tests are mostly property-based / table-driven. The "implementation" is just ensuring the existing code already satisfies them — if a test fails, that's a real bug introduced by the M7 work.

- [ ] **Step 4: Update docs**

In `README.md`, add:

- New env vars (the 6 from Task 11) with defaults and one-line semantics.
- The 10 counters in a table with their `tenant_bucket` label and Prometheus name.
- The trace table name + reason-enum-deferred caveat.
- A note that decision-trace persistence is best-effort.

In `doc.go`, append one sentence: package now persists decision traces and emits the M7 validation-metric subset.

In `CHANGELOG.md`, add an entry under an "Unreleased" / next-version heading describing the M7 add-on. No version bump in this commit (the gateway has not tagged a release yet; the spec leaves the version bump to a future ship moment).

- [ ] **Step 5: Run full verification**

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-gateway
GOWORK=off GOCACHE=/tmp/go-build go test ./... -count=1

cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-postgres
GOWORK=off GOCACHE=/tmp/go-build go test ./... -count=1
GOWORK=off GOCACHE=/tmp/go-build LLM_AGENT_MEMORY_PG_URL=$PG_URL go test ./... -count=1   # only if a live Postgres is available; export the URL first
```

Expected: all pass.

- [ ] **Step 6: Commit**

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-gateway
git add internal/service/cross_tenant_test.go README.md doc.go CHANGELOG.md
git commit -m "docs(memory-gateway): document M7 trace + 10-counter set; add cross-tenant test"
```

---

## Self-Review

- All 9 spec exit criteria from the rescoped roadmap M7 row are covered:
  1. `memory_decision_trace` migrated → Task 1.
  2. Gateway-internal `DecisionTraceSink` interface + Postgres impl → Tasks 2, 3.
  3. Best-effort async with bounded buffer + `trace_dropped_total` → Task 3 + Task 6 (counter).
  4. `TraceEmitter.Emit` call sites mirror to sink → Task 4 (via composition; no call-site edit).
  5. 10 counters emitted via existing exporter → Task 6.
  6. Storage-bytes cron with default 5-min interval → Task 7 + Task 11.
  7. Cardinality rule (tenant_bucket only) → Task 5 + enforced in Tasks 6/8/9/10.
  8. Cross-tenant isolation test → Task 12.
  9. No SDK changes, no new event types, no new sibling → confirmed across all tasks; Sequencing Rule "Gateway-only ownership after Task 1."
- No task introduces a new event type, a new SDK method, or a new sibling module.
- No task touches `llm-agent-memory` (the SDK) module.
- Task 1 is the only postgres-module touch — strictly a migration extension via the existing `migrationStatements()` slice.
- Strict TDD: every task has a Step 1 with failing tests.
- Atomic commits: every task ends with one focused commit. No batch commits.
- Trace failure policy is *locked* at plan stage (Open Decisions), not deferred.
- Each metric's signal source is *named* in the relevant task — none of the 10 counters land without a paired demonstration of where the increment originates.
- The two genuine spec-stage punts (W-1 Unicode normalization, OD-5 retention) do not affect this plan: there is no dedupe in narrow M7 (W-1 N/A); retention is a config knob defaulted off (OD-5 acknowledged but not implemented).

Plan complete and saved to `docs/superpowers/plans/2026-05-27-m7-validation-telemetry-and-trace.md`. Two execution options:

1. **Subagent-Driven** (recommended) — dispatch a fresh subagent per task, review between tasks, fast iteration.
2. **Inline Execution** — execute tasks in this session using `executing-plans`, with checkpoints.

Choose at execute-time.
