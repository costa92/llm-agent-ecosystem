# M7 — Validation Telemetry + Decision Trace Design

> Date: 2026-05-27 (rescoped, replaces 2026-05-27 v1 in place)
> Status: **rescoped.** Original M7 ("Consolidation Worker + 4-Class Validation Metrics + Decision Trace") was reviewed by two independent reviewers (Plan-type internal agent + Codex CLI). Both found that the original scope required substrate changes (Working-tier schema, atomic promotion API, dedupe primitive, typed RecallObserver, relay delivery hardening) that belong with the v2-breaking work in M8. This spec keeps only what is implementable today without changing the SDK boundary or breaking the v1 release line. The deferred work is consolidated into M8 — see the roadmap amendment landing in the same change.
> Companion to: `docs/superpowers/plans/2026-05-25-llm-agent-memory-roadmap.md` (M7 + M8 rows updated), `docs/multi-service-memory-architecture.zh-CN.md` §9.12 / §13.4, `docs/memory-roadmap.zh-CN.md` §11.1, `docs/superpowers/specs/2026-05-26-memory-architecture-current-state.md`.

## 1. Goal

Produce two narrow, independently verifiable deliverables on top of the existing M5/M6 substrate, without touching the SDK module:

1. **Persisted decision traces** — a `memory_decision_trace` Postgres table plus a gateway-internal sink implementation that the existing `TraceEmitter.Emit` call sites write through. Best-effort async write with bounded loss accounting.
2. **A 10-counter validation-metric subset** — the strictly-measurable subset of the v1 4-class set, emitted from code paths that already exist today. The other 12 counters from the original M7 plan are explicitly deferred to M8.

What this design **does not do**:

- No Consolidation Worker. No promote semantics. No `memory_promoted` event. No two-layer dedupe.
- No new SDK methods, interfaces, or types.
- No new sibling Go module.
- No relay protocol changes.
- No reason-enum freeze (the §13.4 canonical enum waits for M8 when the typed RecallObserver lands).

## 2. Why this scope

Two independent reviews of the original M7 spec (Plan-type internal agent + Codex CLI second opinion) reached the same verdict: the original M7 was load-bearing on substrate that does not exist in the v1 SDK. Specifically:

- **Working tier has no schema representation.** `llm-agent-memory-postgres/postgres/schema.go` defines `kind IN ('episodic','semantic')` with no `working` value. Promotion has no source state to read.
- **Atomic promotion across record + event + outbox is not expressible in the v1 SDK.** `EventStore.AppendEvent` (`llm-agent-memory/memory/durable.go:141`) and `Outbox.EnqueueOutbox` (`durable.go:150`) are separate interfaces. The Postgres backend bundles them internally per-mutation, but the SDK exposes no transactional unit-of-work.
- **The relay's batch-level claim transaction does not provide per-message ack.** A mid-batch failure replays the entire already-processed prefix (`llm-agent-memory-postgres/postgres/relay.go:52-79`).
- **`RecallObserver` does not carry the `memory_id` / `request_id` correlation needed for the Class A / Class B counters** (`llm-agent-memory-gateway/internal/service/recall_observer.go:5`).
- **`LastAccessAt` / `HitCount` fields exist in `MemoryRecord` but the mutation SQL never writes them**, so `working_dropped_before_use_total` has no signal source.

Adding all of this to M7 would expand it 2-3× and force partial rewrites of M5 and M6. Adding it to M8 — where the v2-breaking storage refactor is already planned — keeps consumer churn in one window. Narrow-M7 ships the parts that do not depend on any of the above.

## 3. What M5/M6 Actually Delivered (Verified)

Spec claims here are grounded in code, not aspirational:

- **SDK contracts** (`llm-agent-memory/memory/durable.go`): `RecordStore`, `EventStore`, `IdempotencyStore`, `Outbox`, `MessagePublisher`. Domain types: `MemoryRecord` (with `Source string` — *not* a `Metadata` map), `StoredEvent`, `OutboxMessage`, `IdempotencyEntry`.
- **Postgres backend** (`llm-agent-memory-postgres/postgres/`): full implementation of the SDK contracts. Event-type vocabulary fixed at 7 values in `store.go:15-21`: `memory_created`, `memory_updated`, `memory_deleted`, `memory_pinned`, `memory_unpinned`, `memory_disabled`, `memory_enabled`. Relay (`relay.go`) marks outbox rows `sent` on success and re-queues to `pending` on failure (there is no `failed` terminal status and no dead-letter state in M5; the original spec's §9.4 was fictional).
- **Gateway composition** (`llm-agent-memory-gateway/internal/service/`): real, not skeletal. Owns:
  - `TraceEmitter` interface — fire-and-forget, no error return (`service.go:29`).
  - `RecallObserver` — carries `ConsistencyLevel`, `CacheLevel`, `StaleServed` only (`recall_observer.go:5`). No `memory_id`, no `request_id`, no `reason`.
  - `OutboxObserver` for the outbox→vector-index publisher path.
  - A `Snapshot`+`Handler` flat-text Prometheus-style metrics exporter in `internal/observability/metrics.go:84-101`. **No third-party Prometheus dependency.**
- **What is *not* there** despite spec-time misreadings:
  - No `working` tier value, no `tier` column.
  - No persisted `LastAccessAt` write path.
  - No reason-typed trace emission (only free-form `mode` strings on existing `promote_decided` call sites, e.g. `"deferred"`, `"heartbeat"`, `"<close mode>"`).

## 4. In-Scope / Out-of-Scope / Deferred-to-M8

| Item | M7 (narrow) | Deferred to M8 |
|---|---|---|
| Persisted `memory_decision_trace` table | ✅ | — |
| Gateway-internal `DecisionTraceSink` impl (best-effort async) | ✅ | — |
| Reason enum frozen at SDK boundary | — | ✅ |
| Cost-class counters (4 embedding + 2 storage-bytes) | ✅ | — |
| Lifecycle counters: `episodic_disabled_total`, `episodic_deleted_total` | ✅ | — |
| Recall counters: `recall_returned_total`, `recall_selected_total` | ✅ | — |
| Working-tier introduction (schema + write-path default + migration) | — | ✅ |
| Atomic-promotion SDK method | — | ✅ |
| Dedupe primitive (cross-record atomic survivor selection) | — | ✅ |
| Typed `RecallObserver` (with `memory_id`, `request_id`, `reason`) | — | ✅ |
| `LastAccessAt` / `HitCount` write path on recall | — | ✅ |
| Consolidation Worker (promote rules, version-fence, idempotency) | — | ✅ |
| `memory_promoted` event type | — | ✅ |
| `recall_helpful_total` / `recall_unhelpful_total` + feedback endpoint | — | ✅ or later |
| `working_*` lifecycle counters (4 of 6) | — | ✅ (gated on Working tier) |
| All 5 promote counters | — | ✅ (gated on Consolidation Worker) |
| `recall_dropped_total` reason-bucketed counter | — | ✅ (gated on typed observer) |
| `memory_stale_hit_total` | — | ✅ (gated on Worker version-fence) |
| Relay per-row ack / lease / `processing` durable state | — | ✅ (codex C-1) |
| `ConsolidatedFromEventID` column usage (`schema.go:91`) | — | ✅ (used by future Worker) |

10 counters land in M7. 12 + the entire async consumption / promotion / dedupe stack defers to M8.

## 5. Decision-Trace Persistence

### 5.1 Sink location

A new gateway-internal interface defined alongside the existing `TraceEmitter`:

```go
// llm-agent-memory-gateway/internal/service/trace_sink.go
type DecisionTraceSink interface {
    Record(ctx context.Context, row TraceRow) error
}

type TraceRow struct {
    TenantID  string
    RequestID string            // may be empty if upstream call lacked it; counts toward trace_unbound_total
    Stage     string            // recalled | selected | dropped | promote_decided
    Reason    string            // free-form in narrow-M7; enum freeze deferred to M8
    MemoryID  string            // optional, payload-only — never indexed
    Version   int64             // optional
    EmittedAt time.Time
    Emitter   string            // "gateway.recall" / "gateway.session_close" / "gateway.heartbeat" / etc.
    Payload   map[string]any    // verbatim attrs the existing TraceEmitter.Emit call sites pass
}
```

This interface lives **only** in the gateway module. The SDK is not touched. The Postgres implementation lives in `llm-agent-memory-gateway/internal/service/trace_sink_postgres.go`.

### 5.2 Failure policy — best-effort async with bounded loss

Codex C-6 raised this explicitly. Locked here, not deferred:

- A bounded `chan TraceRow` (default capacity 1024) sits between the gateway request path and the Postgres writer goroutine.
- Request-path calls do **not block** on a full channel. On overflow, increment `trace_dropped_total{reason="buffer_full"}` and drop the row.
- The writer goroutine batches up to N rows per Postgres insert (default N=50, configurable).
- On Postgres insert failure, the batch is retried with exponential backoff up to a small ceiling (3 attempts), then dropped with `trace_dropped_total{reason="db_error"}`.
- On shutdown, the goroutine drains the channel with a finite timeout (default 5s). Anything still buffered at timeout is dropped with `trace_dropped_total{reason="shutdown"}`.

Net contract: **traces are observability, not durability.** Loss is accounted, not absorbed silently.

### 5.3 Schema

```sql
-- migration owned by llm-agent-memory-postgres
CREATE TABLE memory_decision_trace (
  trace_id     uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id    text        NOT NULL,
  request_id   text        NULL,
  stage        text        NOT NULL,
  reason       text        NOT NULL,
  memory_id    text        NULL,    -- not indexed (high cardinality, see §7)
  version      bigint      NULL,
  emitted_at   timestamptz NOT NULL DEFAULT now(),
  emitter      text        NOT NULL,
  payload      jsonb       NULL
);

CREATE INDEX idx_trace_tenant_time ON memory_decision_trace (tenant_id, emitted_at DESC);
CREATE INDEX idx_trace_request      ON memory_decision_trace (request_id) WHERE request_id IS NOT NULL;
CREATE INDEX idx_trace_stage_reason ON memory_decision_trace (stage, reason);
```

No `memory_id` index by design — see §7.1.

### 5.4 Wiring into existing call sites

Today the gateway has `TraceEmitter.Emit(stage, attrs)` fire-and-forget calls. The wiring rule for narrow-M7 is:

- Where a call site emits a `recalled` / `selected` / `dropped` / `promote_decided` trace today (via `TraceEmitter.Emit`), the same call is mirrored to `DecisionTraceSink.Record`. Existing `TraceEmitter.Emit` log behavior remains unchanged.
- No call-site shape change. No reason-enum enforcement. Whatever the call site passes today, that's what gets persisted.

This keeps narrow-M7's gateway change additive — easily revertable per call-site.

## 6. Storage-Bytes Cron

### 6.1 Location

```
llm-agent-memory-gateway/internal/service/
  storage_metrics_cron.go        # periodic counter for memory_storage_bytes_total + vector_storage_bytes_total
  storage_metrics_cron_test.go
```

A single goroutine started by the gateway's main composition with a `time.Ticker` (default interval 5 minutes, configurable via env var). Reads counts and size estimates from Postgres:

```sql
SELECT
  COALESCE(SUM(octet_length(content)), 0)             AS memory_bytes,
  COALESCE(SUM(octet_length(vector_chunks.embedding::text)), 0) AS vector_bytes
FROM memory_record
LEFT JOIN vector_chunks USING (memory_id)
WHERE tenant_id = $1
GROUP BY tenant_bucket;
```

Per-tenant-bucket aggregation only; per-tenant is gated behind a separate env flag because of cardinality (§7).

### 6.2 Counters set

- `memory_storage_bytes_total{tenant_bucket}` — sum of `octet_length(content)` over `memory_record`.
- `vector_storage_bytes_total{tenant_bucket}` — sum of vector-chunk bytes.

Both are gauges in spirit but reported as Prometheus counters with `_total` suffix to match the original spec's vocabulary; recipients should `last_over_time` to read them as gauges.

### 6.3 Failure mode

On query failure: increment `storage_cron_failures_total`, log, skip the tick. No backpressure on gateway request path.

## 7. Metric Set — 10 Counters

| Class | Counter | Owner | Source | Notes |
|---|---|---|---|---|
| Cost (D) | `embedding_request_total` | gateway | existing embedding-call site | — |
| Cost (D) | `embedding_applied_total` | gateway | existing embedding-call site | success-only count |
| Cost (D) | `embedding_tokens_total` | gateway | embedding response | — |
| Cost (D) | `embedding_cost_total` | gateway | derived from provider pricing config | — |
| Cost (D) | `memory_storage_bytes_total` | gateway (cron) | §6 | — |
| Cost (D) | `vector_storage_bytes_total` | gateway (cron) | §6 | — |
| Lifecycle (C) | `episodic_disabled_total` | gateway | observed on `memory_disabled` event from outbox | existing event type |
| Lifecycle (C) | `episodic_deleted_total` | gateway | observed on `memory_deleted` event from outbox | existing event type |
| Recall (A) | `recall_returned_total` | gateway | existing recall-path hit count | total hits returned to caller |
| Recall (A) | `recall_selected_total` | gateway | existing recall-path post-budget count | hits surviving token-budget filter |

All counters carry only `tenant_bucket` as label (plus the counter-name itself). No `memory_id`, no `user_id`, no `request_id`, no `reason` (the reason-bucketed counters are M8). See §7.1.

### 7.1 Cardinality rule (preserved from original spec)

Forbidden as labels: `memory_id`, `user_id`, `session_id`, `request_id`, `dedupe_key`, reason strings (until M8 freezes the enum).
Allowed: `tenant_bucket` (default hash `tenant_id` to one of 32 buckets), counter-name suffix variants.

For a 1000-tenant fleet, the 10-counter set with `tenant_bucket{32}` produces ≤ 320 active series — comfortably under any scrape budget.

### 7.2 Where the counters emit from

The gateway already has `internal/observability/metrics.go:84-101` running an atomic-int + text-format exporter. Narrow-M7 reuses that exporter. No new Prometheus client dependency, no new exporter pathway. The §8.1 "MetricsObserver in SDK" from the original spec is dropped — there is no SDK interface to define.

## 8. Cross-Module Impact (Honest, Narrow)

### 8.1 `llm-agent-memory` (SDK)

**Untouched.** No new methods, no new types. The original spec's `PromoteRecord` / `DecisionTraceSink` / `MetricsObserver` additions are all deferred to M8.

### 8.2 `llm-agent-memory-postgres`

- One new migration: `memory_decision_trace` table per §5.3.
- One new helper in the postgres module if the migration framework requires it (otherwise the migration is a single SQL file referenced by the existing migrator).
- **No** event-type allowlist change. **No** `memory_promoted` value added. **No** schema change to `memory_record`.
- Existing relay, store, mutation transactions: **not touched.**

### 8.3 `llm-agent-memory-gateway`

- New file: `internal/service/trace_sink.go` (interface).
- New file: `internal/service/trace_sink_postgres.go` (impl).
- New file: `internal/service/storage_metrics_cron.go`.
- Modified: `internal/service/service.go` — every existing `TraceEmitter.Emit(stage, attrs)` call site adds a `traceSink.Record(...)` call alongside. Behavioral note: the persistence add is **additive** — the existing log emission stays.
- Modified: `cmd/memory-gateway/main.go` — wire the sink and the cron into composition.
- Modified: `internal/config/config.go` — env vars for trace-sink buffer size and cron interval.
- Modified: `internal/observability/metrics.go` — register the 10 new counters.

### 8.4 No new module

No `llm-agent-memory-worker` sibling. The trace sink and the storage-bytes cron both live in the gateway. M8 introduces the sibling when there is real outbox consumption work that justifies the separate release line.

## 9. Cross-Tenant Rule

Every Postgres query in §5 and §6 includes `tenant_id` in `WHERE` (or `GROUP BY` for aggregates). The M5+ CI grep rule for DAL queries applies unchanged. The trace insert is parameterized on `tenant_id` from the request context, never from client claim.

## 10. Non-Goals (Explicit)

- Promotion of any kind. No record-state mutation by background process.
- Any new event type.
- Any change to the SDK module.
- Any change to the relay protocol.
- Any change to the existing 7 event types.
- Feedback ingest endpoint for helpful/unhelpful counters.
- A frozen reason enum.
- A new sibling Go module.
- Per-tenant (rather than per-bucket) metric labels.

## 11. Open Decisions (Recommendation + Alternative)

| # | Decision | Recommendation | Defer-to |
|---|---|---|---|
| OD-1 | Storage-bytes cron interval default | 5 minutes | none — accept default at plan stage |
| OD-2 | Trace-sink buffer capacity default | 1024 rows | none — accept default at plan stage |
| OD-3 | Batch insert size | 50 rows | none — accept default at plan stage |
| OD-4 | `tenant_bucket` hash modulus | 32 | none |
| OD-5 | Retention for `memory_decision_trace` | 30 days (a periodic `DELETE` in the same cron or a separate maintenance task) | plan stage — confirm whether to run retention in M7 or skip until growth becomes a problem |

OD-5 is the only decision worth re-confirming. The other four are sensible defaults.

## 12. Acceptance Criteria

The spec is approved when:

1. The narrow scope (§4 in-scope / deferred table) is endorsed and any "should be in M7" items are reclassified into one of the three columns.
2. The decision to leave the SDK untouched is endorsed.
3. The decision to keep narrow-M7 inside the gateway (no new sibling) is endorsed.
4. The 10-counter set (§7) is endorsed and no counter is reclassified without a paired source-of-signal demonstration.
5. The trace-sink failure policy (best-effort async + bounded loss accounting) is endorsed.
6. OD-5 (trace retention) has either a value or an explicit "do not implement in M7" verdict.

Once §12 is signed off, the next artifact is the implementation plan at `docs/superpowers/plans/2026-10-XX-m7-validation-telemetry-and-trace.md`, with task-level TDD checkboxes.

## 13. Self-Check Against the Cross-Review Findings

How each finding from the two prior reviews lands in the narrow scope:

| Finding | Source | Disposition in narrow M7 |
|---|---|---|
| C-1 (fictional `failed` / dead-letter status) | Plan | **Removed.** Narrow-M7 doesn't talk about relay status at all. |
| C-2 (gateway `mode` strings outside frozen enum) | Plan | **Resolved by descoping.** Narrow-M7 accepts free-form `reason` strings. M8 freezes the enum. |
| C-3 (Working tier nonexistent in schema) | Plan / codex S-1 | **Deferred to M8.** Promotion is not in scope. |
| C-4 (`r.Metadata["source"]` is wrong) | Plan | **Removed.** No promote rules to encode. |
| C-5 (Promote/Worker idempotency table not used) | Plan | **Deferred to M8.** |
| C-6 (DecisionTraceSink ownership contradictory) | Plan | **Resolved.** Sink lives in gateway-internal only. SDK untouched. |
| HC-1 (gateway sync trace write adds 4-5 DB round-trips) | Plan | **Resolved.** §5.2 commits to best-effort async with bounded loss; request path never blocks. |
| HC-2 (worker needs bundle interface) | Plan | **Removed.** No worker. |
| HC-3 (two parallel metric pathways) | Plan | **Resolved.** §7.2 reuses the existing gateway exporter; no SDK MetricsObserver. |
| OR-1 (pool starvation if worker in-process) | Plan | **Removed.** No worker. |
| OR-2 (worker-tx vs relay-tx race) | Plan / codex C-1 | **Deferred to M8.** |
| OR-3 (trace table hot path) | Plan | **Bounded by §5.2.** Async + buffer + retention (OD-5). |
| OR-4 (`memory_promoted` has no consumer) | Plan | **Removed.** Event no longer emitted. |
| OR-5 (Class A is symbolic without feedback) | Plan | **Acknowledged.** `recall_helpful_total` / `recall_unhelpful_total` are deferred — counted in the §4 deferred row. |
| OR-6 (Class D needs periodic-job abstraction) | Plan | **Resolved by §6** — single goroutine + ticker, no generic abstraction needed. |
| codex C-1 (batch replay amplification) | codex | **Deferred to M8** — no worker depends on the relay in M7. |
| codex C-2 (no atomic dedupe primitive) | codex | **Deferred to M8.** |
| codex C-3 (no atomic event+outbox) | codex | **Deferred to M8.** |
| codex C-4 (Class A/B not observable in current observer) | codex | **Deferred to M8** — Class B fully deferred; Class A scoped to just `_returned_total` and `_selected_total` which the current observer can produce. |
| codex C-5 (no `LastAccessAt` write path) | codex | **Deferred to M8** — `working_dropped_before_use_total` not in narrow set. |
| codex C-6 (trace failure policy undefined) | codex | **Resolved.** §5.2. |
| `ConsolidatedFromEventID` column at `schema.go:91` exists but unused | codex | **Documented.** Column kept for M8 Consolidation Worker use; narrow-M7 does not write or read it. |

All cross-review findings are either resolved in narrow M7, explicitly deferred to M8, or removed by descoping.

## 14. Self-Check Against Spec Discipline

| Check | Pass? |
|---|---|
| Every claim about the existing codebase is grounded in a file:line reference (§2, §3) | Yes |
| Module ownership stated up front with allowed/forbidden edges (§8) | Yes |
| Non-goals enumerated (§10) | Yes |
| Open decisions surfaced with recommendation (§11) | Yes |
| Acceptance criteria for the spec itself (§12) | Yes |
| No SDK-boundary touch, no premature task breakdown | Yes |
| Every original-spec section that was removed has either a "deferred to M8" marker or a "removed by descoping" justification (§13) | Yes |

## 15. Known Weaknesses

- **W-1.** The 10-counter set is roughly half of the original v1 "4-class minimum." This is a real product retreat. The roadmap amendment in the same change must acknowledge that the v1 minimum metric set lands across M7 + M8 rather than fully in M7.
- **W-2.** The decision-trace `reason` column is free-form in narrow M7. Downstream consumers that join trace rows by reason must tolerate non-canonical values until M8 freezes the enum. A column comment should call this out at migration time.
- **W-3.** The storage-bytes cron does a periodic full-aggregate query. For tenants with millions of `memory_record` rows this becomes a slow query at scale. Acceptable at v1 (single-tenant or small fleet); review if a v1 deployment exceeds ~1M records per tenant.
- **W-4.** OD-5 trace retention is left to plan stage with a defaulted recommendation. If a deployment runs without retention enabled, the table will grow unboundedly. Plan stage must either implement retention or document the operator obligation.
- **W-5.** The gateway file-list in §8.3 is the minimum surface; a real implementation may discover additional touch points (config validation, smoke test wiring). Spec leaves that to plan stage rather than enumerating speculatively.
