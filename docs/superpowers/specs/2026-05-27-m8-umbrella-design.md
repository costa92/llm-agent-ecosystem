# M8 Umbrella Design — Sub-Milestone Split

> Date: 2026-05-27
> Status: M8-shaping spec. M8 in the current roadmap absorbs (a) the original Phase E v2-breaking work and (b) the substrate that M7 had to defer (Working tier, atomic promotion API, dedupe primitive, typed RecallObserver, Consolidation Worker, remaining 12 validation counters, relay delivery hardening, reason-enum freeze). The roadmap already calls M8 XL and notes "will be split into ≥3 sub-PRs." This spec locks the split into named sub-milestones, fixes the highest-impact cross-cutting decisions, and hands off per-sub-milestone detail to follow-on specs.
> Companion to: `docs/superpowers/plans/2026-05-25-llm-agent-memory-roadmap.md` (§M8 row to be amended), `docs/superpowers/specs/2026-05-27-m7-workers-and-validation-design.md` (§13 — what was deferred and why).

## 1. Goal

Three deliverables from this umbrella spec:

1. **Define the sub-milestones** (M8a / M8b / M8c / M8d) with sharp dependency edges and one-PR-each rule.
2. **Lock the cross-cutting decisions** that any sub-milestone would have to revisit otherwise — Working-tier schema representation, scoredStore relocation, atomic-promotion API shape, LastAccessAt write path strategy, reason-enum freeze process.
3. **Hand off** per-sub-milestone detail to follow-on specs. This umbrella does NOT enumerate task-level changes, doc-level wording, or exit-criteria checklists for each sub-milestone — those go into their own spec at sub-milestone kick-off.

What this spec does **not** do:

- It does not write any code or migration.
- It does not pick a vector backend, queue runtime, or specific Prometheus exporter (per-sub-milestone decisions).
- It does not order the sub-milestones in calendar time. The dependency edges below establish *what must precede what*, not *when*.
- It does not commit to a v2.0.0 ship date. M8a alone may or may not warrant a release tag — that's a decision at sub-milestone close.

## 2. Verified Codebase Facts (Re-Reading the Substrate, May 2026)

Spec claims here are grounded in code as of commits up to `b182c95` on `main`.

### 2.1 Schema today (`llm-agent-memory-postgres/postgres/schema.go`)

`memory_record` already has:

- `kind TEXT NOT NULL CHECK (kind IN ('episodic', 'semantic'))` — **no `working` value** today.
- `last_access_at TIMESTAMPTZ` — **column exists**, but no mutation SQL writes it.
- `hit_count BIGINT NOT NULL DEFAULT 0` — **column exists**, never incremented.
- `consolidated_from_event_id TEXT` — **column exists**, never populated.
- `source TEXT NOT NULL CHECK (source IN ('user_saved', 'agent_inferred', 'system'))` — typed column. M7's flawed spec section that read `r.Metadata["source"]` was reading from the wrong place; the field is `MemoryRecord.Source` (line 18 of `durable.go`).

**Implication:** Working-tier introduction does NOT need new columns. Three of the columns the Consolidation Worker needs already exist. The §M7 deferred work is *less* schema-breaking than the original roadmap row implied.

### 2.2 SDK contracts (`llm-agent-memory/memory/durable.go`)

`RecordStore` exposes single-record operations only: `WriteRecord`, `PatchRecord`, `DeleteRecord`, `PinRecord`, `DisableRecord`, `GetRecord`. There is no transactional unit-of-work primitive. `EventStore.AppendEvent` and `Outbox.EnqueueOutbox` are separate interfaces (codex C-3 finding).

**Implication:** atomic promotion across record-state + event + outbox CANNOT be expressed by composing existing SDK methods. M8 must add a primitive.

### 2.3 scoredStore lives in core, not in the sibling (`llm-agent/memory/internal_score.go`)

The shared in-memory engine for Working / Episodic / Semantic is `scoredStore` in the `llm-agent/memory/` core module — line 19, single `sync.Mutex`. The sibling `llm-agent-memory` imports core types and wraps with scoped-lifecycle, but does NOT have its own scoredStore.

**Implication:** Phase E's "scoredStore concurrency refactor" cannot land in the sibling without first **lifting** scoredStore out of core. Per the roadmap §1.3 deprecation policy ("M8 deprecation announced" for core), the lift is the right move regardless — the refactor work goes into the sibling, and core gets a deprecation patch.

### 2.4 Relay delivery (`llm-agent-memory-postgres/postgres/relay.go`)

`RunOnce` claims a batch under one outer transaction, calls `Publisher.Publish` per row inside the tx, marks rows `sent` only at commit time. Failures re-queue to `pending` (no `failed` state, no dead-letter). Codex C-1: a mid-batch hang/crash replays the entire processed prefix.

**Implication:** M8 must harden delivery semantics before the Consolidation Worker can be safely written on top of it. Either per-row tx or durable `processing` state with lease/heartbeat.

### 2.5 Validation telemetry (`llm-agent-memory-gateway/internal/observability/metrics.go`, post-M7)

Gateway exporter already supports per-tenant_bucket counters with thread-safe per-bucket maps. M7 landed 10 validation counters + `trace_dropped_total` + `storage_cron_failures_total`. The infrastructure for the deferred 12 counters exists; only the counter declarations and emission call sites need to be added.

**Implication:** the M8 "remaining 12 counters" work is mostly emit-call-site work, not exporter-infra work. Smaller than the original roadmap row suggests.

## 3. Sub-Milestone Split

Four sub-milestones. Dependency edges are strict.

```
M8a (substrate)
    ├── adds Working tier (extends `kind` CHECK constraint, no new column)
    ├── adds atomic promotion API (RecordStore.PromoteRecord — see §4.2)
    ├── adds LastAccessAt write path on recall (small mutation already-existing columns)
    └── adds dedupe primitive (RecordStore.ResolveDedupe — see §4.3)
    ↓
M8b (worker)
    ├── new `llm-agent-memory-worker` sibling
    ├── consumes outbox via existing MessagePublisher seam
    ├── promotes Working→Episodic using M8a primitives
    └── adds the 5 Promote-class + 4 Working-lifecycle counters that depend on it
M8b in parallel with M8c.
    ↓
M8c (Phase E storage refactor)
    ├── lifts scoredStore out of `llm-agent/memory/internal_score.go` into the sibling
    ├── refactors concurrency (RWMutex / CoW / shard-by-kind — locked in M8c spec)
    ├── introduces pluggable VectorBackend (pgvector first wiring)
    ├── lifts Source / Category / Pinned / Disabled / Scope out of Metadata to typed Go fields
    └── snapshot v1→v2 migration
M8c is orthogonal to M8a/M8b at the schema level — no shared touch points.
    ↓
M8d (typed observer + remaining telemetry + reason-enum freeze)
    ├── extends RecallObserver with memory_id / request_id / reason (DEPENDS on M8a's promote API for the `was_promoted` flag)
    ├── adds the 3 Recall-class deferred counters (recall_dropped_total reason-bucketed, recall_helpful_total, recall_unhelpful_total) + feedback ingest endpoint
    ├── adds `memory_stale_hit_total` (DEPENDS on M8b worker version-fence)
    ├── relay delivery hardening (codex C-1) — DEPENDS on M8b being live so the contract is testable
    └── freezes the §13.4 reason enum at the SDK boundary; migrates existing M7 free-form trace rows
```

### 3.1 Why this split (and not a different one)

- **M8a is the keystone.** Nothing else can use the worker if the promote API doesn't exist; nothing measures "stale before use" if `LastAccessAt` isn't written. M8a must land first.
- **M8b and M8c are orthogonal.** Storage refactor (M8c) and Consolidation Worker (M8b) touch different modules and different concerns. They can ship in either order after M8a.
- **M8d is the wrap.** Reason-enum freeze can't happen until the worker has produced a stable corpus of reason values to migrate. Typed observer is meaningful only once `was_promoted` is queryable (M8a). Relay hardening is meaningful only with a real consumer (M8b) running.
- **Not splitting smaller** is intentional. M8a's four pieces (Working tier, promote API, LastAccessAt path, dedupe primitive) are tightly coupled — promote semantics depend on Working tier existing, dedupe depends on promote being atomic. Splitting M8a further would create cross-PR fragility.

### 3.2 Out of scope for this umbrella (handled in sub-milestone specs)

- Specific concurrency strategy for scoredStore (RWMutex vs CoW vs shard-by-kind) — M8c.
- Specific vector backend choice (pgvector vs Qdrant vs Milvus) — M8c.
- Specific worker queue runtime (polling relay vs NATS/Kafka) — M8b. Default recommendation: hardened polling relay (codex C-1 mitigation in M8d) — defer broker introduction until measured demand.
- Specific reason-enum values — M8d, informed by `memory_decision_trace` data gathered while M7 runs in staging (see `docs/m7-staging-observability.md` S1/S5).

## 4. Locked Cross-Cutting Decisions

These would have to be re-decided in each sub-milestone if not pinned here. Locking them at umbrella level prevents drift.

### 4.1 Working tier representation: extend `kind` CHECK, no new column

**Decision.** M8a extends `memory_record`'s constraint to `CHECK (kind IN ('working', 'episodic', 'semantic'))`. Existing rows back-fill to `'episodic'` (preserves current observable behavior — every row already in the durable store has effectively been "promoted").

**Alternatives considered:**

- *Add a separate `tier` column.* Rejected. Doubles the migration cost (two columns to back-fill, two semantics for callers to track) without adding signal — `kind` already carries enough information once extended.
- *Use `NULL kind` for working state.* Rejected. NULLs are correctness hazards; CHECK constraints handle them poorly.

**Migration:** one SQL statement to drop+recreate the CHECK constraint; back-fill `UPDATE memory_record SET kind='episodic' WHERE kind IS NULL`; new writes default `kind='working'` via the SDK write path. M8a spec details the exact migration.

### 4.2 Atomic promotion API: extend `RecordStore`, do not introduce `MemoryStore.Apply`

**Decision.** M8a adds a single method to the existing `RecordStore` interface:

```go
type PromoteRecordInput struct {
    TenantID        string
    MemoryID        string
    ExpectedVersion int64
    SourceEventID   string  // populates consolidated_from_event_id; codex S-5 mitigation
    Reason          string  // typed once M8d freezes the reason enum
}

type PromoteRecordResult struct {
    MemoryID string
    Version  int64
    Record   MemoryRecord
}

type RecordStore interface {
    // existing methods...
    PromoteRecord(ctx context.Context, in PromoteRecordInput) (PromoteRecordResult, error)
}
```

The Postgres backend bundles the three writes (record state change + event append + outbox enqueue) into one transaction internally, matching the existing pattern (`store.go:43,399` opens its own pgx tx for `WriteRecord`/`PatchRecord` — same shape for `PromoteRecord`).

**Alternatives considered:**

- *Introduce a top-level transactional primitive `MemoryStore.Apply(ctx, op)` that exposes a `Tx` to callers.* Rejected. Leaks DB semantics into the SDK; every backend would have to invent its own `Tx` type; callers gain a footgun (forgetting to commit). Existing mutations already bundle internally — `PromoteRecord` matches that style.
- *Compose existing methods (PatchRecord + AppendEvent + EnqueueOutbox).* Rejected. Three round-trips, no atomicity guarantee — exactly the failure mode codex C-3 surfaced.

**Idempotency.** `PromoteRecord` is idempotent on `(tenant_id, memory_id, expected_version, source_event_id)`. A redelivered outbox message that already promoted returns the existing record with `Version` unchanged. The `source_event_id` is the durable idempotency key the worker derives from the outbox row.

**SDK release impact.** `RecordStore` is an interface — adding a method breaks every existing mock and adapter (codex previously found 3 mocks in gateway tests). Mitigated by `MockRecordStore` embedding pattern or by bundling the mock updates into the M8a PR. SDK version: bumps to **v1.1.0** (additive but interface-extending = minor bump under semver-ish for interfaces).

### 4.3 Dedupe primitive: `ResolveDedupe` returns winner_id, atomic per dedupe-key

**Decision.** M8a adds a second new method on `RecordStore`:

```go
type ResolveDedupeInput struct {
    TenantID  string
    DedupeKey string  // sha256(tenant_id || user_id || category || project_id || normalize(content))
    Candidate MemoryID  // the incoming candidate
}

type ResolveDedupeResult struct {
    WinnerID  MemoryID  // may equal Candidate or be an existing memory_id
    Action    DedupeAction  // KEPT_NEW | MERGED_INTO_EXISTING | COLLAPSED_BY_PIN
}
```

Backed by a new `memory_dedupe_index` table with `UNIQUE (tenant_id, dedupe_key)`. The Postgres implementation runs the insert + winner-selection in one transaction (codex C-2 mitigation).

**Dedupe key includes `project_id`** per Plan-agent S-3 finding (cross-project collapse hazard if omitted). `session_id` is NOT in the key — cross-session promotion remains supported.

**Normalization** for the content portion: lowercase + collapse whitespace + strip ASCII punctuation. Unicode-aware normalization deferred (M7 spec W-1 acknowledgment).

### 4.4 LastAccessAt write path: batched, non-blocking, recall-side only

**Decision.** M8a adds an `UPDATE memory_record SET last_access_at = $1, hit_count = hit_count + 1 WHERE tenant_id = $2 AND memory_id = ANY($3)` call from the gateway recall path, batched across all hits in the response, using a separate non-transactional write that fires AFTER the recall response has been sent to the client.

**Reason:** sync-writing access marks on every recall would add a write-amplification storm. Async fire-and-forget (with the same bounded-channel pattern as the M7 trace sink) gives "best effort" semantics that match the observability use case.

**On failure:** increment `last_access_write_failures_total` (operational counter, no `tenant_bucket` label, matches `storage_cron_failures_total` shape). No retry — next recall will re-write anyway.

### 4.5 scoredStore relocation: lift first, refactor second

**Decision.** M8c starts by **copying** `internal_score.go` + its tests from `llm-agent/memory/` into `llm-agent-memory/memory/`, then making the sibling's Working/Episodic/Semantic types reference the local copy instead of the core import. Core `scoredStore` becomes the deprecation patch's only remaining surface. Refactor (RWMutex / CoW / shard-by-kind) happens against the lifted copy in the sibling.

**Reason:** refactoring in core while announcing core deprecation is wasted work. Lifting first gives the sibling a stable starting point for the refactor and lets core enter pure-bug-fix-only mode immediately.

**Sub-decision deferred to M8c spec:** which concurrency strategy. Three candidates from roadmap §6.1:

| Candidate | Pro | Con |
|---|---|---|
| RWMutex + immutable iterator view | Smallest delta | Reader doesn't see writes mid-iteration; OK for recall but quirky for tests |
| Copy-on-Write per write | Zero reader contention | Allocation pressure under high write rate |
| Shard by kind (Working / Episodic / Semantic each get their own mutex) | Cleanest separation; matches the tier model | Cross-shard atomic ops (promotion!) need extra coordination |

The roadmap target is "≥2× concurrent-read throughput at 10k items." M8c picks based on benchmark, not opinion.

### 4.6 Reason enum freeze process

**Decision.** M8d freezes the reason enum at the SDK boundary. Migration of existing M7 free-form trace rows: rows with reasons that map cleanly to enum values get re-tagged; ambiguous values get `reason='legacy_unmigrated'` and the original written into payload. No row deletion.

The enum's first-class values are picked **from M7 staging data** (see `docs/m7-staging-observability.md` S1/S5). Long-tail values (<1% of rows) collapse to `'other'` in the frozen enum.

**Implication for sub-milestone ordering:** M8d benefits from M7 having run in staging long enough to populate `memory_decision_trace` with realistic reason values. Calendar-time loose-coupling between M7 ship and M8d kickoff is a feature, not a bug.

## 5. Cross-Module Impact (Honest Inventory)

Far larger than M7's narrow scope. Listed by module:

### 5.1 `llm-agent-memory` (SDK)

- M8a: extend `RecordStore` interface (+2 methods); add `PromoteRecordInput/Result`, `ResolveDedupeInput/Result`, `DedupeAction` types. **First minor bump since v1.0.0.**
- M8c: full scoredStore relocation + refactor. **Major bump to v2.0.0.**
- M8d: extend `RecallObservation` with typed fields; freeze reason enum. **Possibly another minor between v2.0.0 and any post-v2 line.**

### 5.2 `llm-agent-memory-postgres`

- M8a: extend `kind` CHECK; add `memory_dedupe_index` table; implement `PromoteRecord` + `ResolveDedupe`; add `LastAccessAt` update statement.
- M8d: relay delivery hardening (per-row ack or `processing` state with lease).

### 5.3 `llm-agent-memory-gateway`

- M8a: call `PromoteRecord` from any code path that currently does a manual update + event emit + outbox enqueue (audit during M8a spec).
- M8a: wire the batched `LastAccessAt` writer; add the operational failure counter.
- M8d: extend `RecallObserver` impls; emit the 7 deferred counters; add the feedback ingest HTTP endpoint.

### 5.4 New module: `llm-agent-memory-worker` (M8b)

New sibling. Cmd binary `cmd/consolidation-worker/main.go`. Internal packages mirror the gateway shape — config, transport (consumer side), service, observability.

### 5.5 `llm-agent` (core)

Deprecation patch: announce in `CHANGELOG.md`; cut a final patch release; no new features. Core `scoredStore` becomes the last functional thing left after M8c lifts.

## 6. Non-Goals (Explicit)

- Salience / learned rerank / decay learning. Out of v2 per memory-roadmap §11.3.
- Vector-similarity dedupe (third layer). Out of M8 even after M8a's dedupe primitive lands — that's a P2 future enhancement gated behind an opt-in policy flag.
- Multi-tenant isolation strengthening beyond what M5 + M7 already enforce.
- HTTP API additions beyond the feedback ingest endpoint in M8d.
- Schema-level partition / retention automation for `memory_decision_trace` — operator obligation per M7 OD-5.

## 7. Acceptance Criteria for This Umbrella

The umbrella is approved when:

1. The 4 sub-milestones are endorsed in name and dependency order (§3).
2. The 6 locked decisions (§4.1–§4.6) are endorsed; any not-endorsed item demotes back to "open question" and re-surfaces at the relevant sub-milestone spec.
3. Cross-module impact (§5) is endorsed: SDK gets a v1.1.0 minor, a v2.0.0 major, and likely a follow-on minor; new sibling appears in M8b; core gets a deprecation patch in M8c.
4. The reason-enum freeze approach (§4.6) — collect data from M7 staging, then freeze — is endorsed as a sequencing rule, not just an aspiration.

Once §7 is signed off, the next four artifacts are sub-milestone specs:

- `docs/superpowers/specs/<date>-m8a-substrate-design.md`
- `docs/superpowers/specs/<date>-m8b-consolidation-worker-design.md`
- `docs/superpowers/specs/<date>-m8c-storage-refactor-design.md`
- `docs/superpowers/specs/<date>-m8d-typed-observer-and-enum-freeze-design.md`

Each will go through the two-round review pattern (Plan-type agent + codex consult) per the `feedback_two-round-spec-review` memory before its implementation plan is written.

## 8. Known Weaknesses of This Umbrella

- **W-1.** The §3 dependency graph claims M8b and M8c are orthogonal. This holds at the schema level but may break at the in-memory level — M8c's scoredStore refactor changes the internal storage shape that M8b's worker may want to read for dedupe purposes. M8c spec should explicitly verify "no worker code path reads `*scoredStore` directly" before claiming orthogonality.
- **W-2.** §4.2's `PromoteRecord` addition breaks existing test mocks of `RecordStore`. M8a spec must enumerate every existing mock and bundle the updates atomically — incremental landing will produce broken intermediate states.
- **W-3.** §4.4's batched `LastAccessAt` writes are fire-and-forget. Under sustained recall load, the buffer fills and we drop access marks; the operational counter signals this, but it means the working_dropped_before_use_total metric M8d depends on becomes less accurate at scale. Acceptable for v2.0.0 telemetry use; document the limit at M8d spec time.
- **W-4.** §4.5's lift-first-refactor-second plan assumes `internal_score.go` is genuinely copyable — no hidden imports from elsewhere in core, no test fixtures that bind to package-private symbols. M8c spec should grep before committing.
- **W-5.** §4.6's "freeze enum from staging data" assumes M7 actually runs in staging long enough to produce a stable reason corpus. If M8 starts before that, M8d either delays or freezes on inadequate data — flag at sub-milestone kickoff.
