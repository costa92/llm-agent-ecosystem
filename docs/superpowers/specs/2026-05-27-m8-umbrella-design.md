# M8 Umbrella Design — Sub-Milestone Split (v2)

> Date: 2026-05-27
> Status: **revised after two-round review.** v1 of this spec went through Plan-type internal review (14 findings) and Codex CLI consult (6 additional findings). Both reviewers verdict: "needs another umbrella pass." This v2 addresses all 20 findings — most importantly the live-write rollout discovery (current gateway emits `kind` verbatim, schema rejects anything outside `('episodic','semantic')`, so M8a is NOT additive without an expand-first migration), the dead `WHERE kind IS NULL` back-fill (`kind` is `NOT NULL`), the missing `memory_promoted` event commitment from M7, and the codex-recommended `Promoter` / `Deduper` interface-extension pattern that avoids breaking existing `RecordStore` mocks.
> Companion to: `docs/superpowers/plans/2026-05-25-llm-agent-memory-roadmap.md` (§M8 row to be amended again), `docs/superpowers/specs/2026-05-27-m7-workers-and-validation-design.md` (§13 — what was deferred and why), `docs/m7-staging-observability.md` (data source for §4.7 reason-enum freeze trigger).

## 1. Goal

Three deliverables from this umbrella:

1. **Sub-milestone split**: M8a-prep → M8a → (M8b ∥ M8c) → M8d. The v1 spec was missing M8a-prep; without it, M8a's substrate changes break live writes.
2. **Cross-cutting decisions locked at umbrella level** so they don't get re-fought per sub-milestone — Working-tier migration shape, atomic-promotion interface boundary, dedupe primitive contract (with loser-resolution semantics, fixed in v2), LastAccessAt write path, scoredStore lift scope (corrected upward in v2 from "one file" to "package cluster"), reason-enum freeze readiness, relay hardening placement (moved earlier in v2).
3. **Hand-off**: per-sub-milestone spec writing comes next. Each sub-spec runs through the same two-round review pattern before its implementation plan is written.

## 2. Verified Codebase Facts (re-verified during v2 review)

Spec claims here are grounded in code at `b182c95` on `main`. Items marked **(v2 correction)** were wrong in v1.

### 2.1 Schema today (`llm-agent-memory-postgres/postgres/schema.go`)

`memory_record` already has:

- `kind TEXT NOT NULL CHECK (kind IN ('episodic', 'semantic'))` — **(v2 correction)** `kind` is `NOT NULL`. The v1 spec's `UPDATE memory_record SET kind='episodic' WHERE kind IS NULL` was dead code; this is now addressed in §4.1.
- `last_access_at`, `hit_count`, `consolidated_from_event_id` — columns exist; mutation SQL never writes them.
- `source` CHECK is `('user_saved', 'agent_inferred', 'system')` — three sources, not two. M8a's promote rules act on `source`, not on a hypothetical Metadata field.

### 2.2 Migration framework (`schema.go:8-63`)

`SchemaVersion` is a single integer. `Migrate()` runs `migrationStatements()` as one flat list, once. **There is no concept of stage / phase / dual-write / contract.** **(v2 correction)** v1 assumed staged migrations were possible without saying so. M8a-prep now makes this explicit.

### 2.3 Write path (`store.go:32-160` + gateway `service.go:240-340`)

`WriteRecord` accepts `record.Kind` from the caller and inserts verbatim (`store.go:70`). Gateway passes the caller's claimed kind through without normalization (`service.go:269,278`). **Implication: if any caller sets `kind="working"` before the schema accepts that value, the write fails. M8a is NOT additive without an expand-first migration.**

### 2.4 SDK contracts (`durable.go:132-156`)

`RecordStore` has 6 methods. `EventStore.AppendEvent` and `Outbox.EnqueueOutbox` are separate. No transactional unit-of-work primitive. **The only `RecordStore` implementer in the workspace is `postgres.Store`** (3 test fakes in gateway tests are the only other "implementers"). Local `go.mod` `replace` directives hide what published downstream consumers see — they pin `llm-agent-memory v1.0.0`, so any interface-level change is a real break for them.

### 2.5 scoredStore in core (`llm-agent/memory/internal_score.go` + dependencies)

Single `sync.Mutex`. **(v2.1 correction — lift scope under-counted again at v2, now corrected via third-round verification.)** `*scoredStore` is bound by FOUR memory-type files: `working.go:48,77,95,121` (WorkingMemory), `episodic.go:18` (EpisodicMemory), `semantic.go:18` (SemanticMemory), plus indirect bindings through `recall.go:140` (`listFromStore`) and `manager.go:438,442` (`storeOf` accessor). `persistence.go:79,105` binds it for snapshot/import. Sibling `consolidator_test.go:94` and `testutil_test.go:54` depend on core export/import round-trip. The `MemoryItem` type (`memory.go:25-30`), `Kind` enum, `Stats`, and `Memory` interface all live in core and are imported by the sibling. **A lift requires moving ~8 files (internal_score.go + memory.go + working.go + episodic.go + semantic.go + recall.go + manager.go's storeOf + persistence.go) plus updating ≥6 sibling imports.**

**No benchmark harness exists in `llm-agent/memory/`.** `rg '^func Benchmark'` returns zero. M8c's "≥2× throughput improvement" claim has no baseline measurement infrastructure.

### 2.6 Relay delivery (`relay.go` + `store.go:23`)

Batch-tx ack model (codex C-1 from M7 review). **The `outboxStatusProcessing` constant exists (`store.go:23`) but is dead code — the relay never writes that status.** The substrate for "durable processing state with lease" is half-built. M8a-prep can wire it up without a new column.

### 2.7 Event-type allowlist coupling (`outbox_vector_publisher.go:31`)

The vector projector's outbox consumer dispatches on `EventType` and ignores unknown values. New event types (`memory_promoted`, `memory_dedupe_collapsed`) emitted without a paired allowlist update get silently dropped from the vector index path.

## 3. Sub-Milestone Split (v2 — adds M8a-prep)

```
M8a-prep (infrastructure: enables everything else)
    ├── upgrade migration framework to support staged transitions
    ├── relay delivery hardening (write `processing` state, per-row ack)
    ├── event-type allowlist extension for `memory_promoted` and `memory_dedupe_collapsed`
    └── vector projector switch-statement: add `case "memory_dedupe_collapsed"` arm that removes loser's vector chunks (allowlist alone won't dispatch — see §4.3 step 4)
    ↓
M8a (substrate)
    ├── Working-tier rollout (expand-first; see §4.1 — 4 phases)
    ├── Promoter interface + Postgres impl (see §4.2 — separate interface, not RecordStore method)
    ├── Deduper interface + Postgres impl + memory_dedupe_index table + loser-resolution semantics (see §4.3)
    └── LastAccessAt batched write path (see §4.4)
    ↓ (after M8a's migration phases reach "kind='working' accepted by schema")
M8b (Consolidation Worker)              M8c (Phase E storage refactor)
    ├── new sibling llm-agent-memory-worker  ├── benchmark harness FIRST
    ├── consumes outbox via MessagePublisher ├── package-cluster lift (5+ files; see §4.5)
    ├── promotes via Promoter interface      ├── concurrency refactor (RWMutex / CoW / shard — picked by benchmark)
    ├── dedupes via Deduper interface        ├── VectorBackend pluggability
    └── adds 5 Promote + 4 Working-lifecycle counters   └── snapshot v1→v2 + Metadata→typed-field lift
    M8b and M8c are orthogonal at the schema level after M8a;
    cross-coupling around `*scoredStore` is now M8c-internal only.
    ↓
M8d (typed observer + reason-enum freeze)
    ├── extend RecallObservation with memory_id / request_id / reason / was_promoted
    ├── emit the 3 deferred Recall counters + feedback ingest endpoint
    ├── emit `memory_stale_hit_total` (depends on M8b worker version-fence)
    └── freeze §13.4 reason enum + migrate existing M7 free-form trace rows
```

### 3.1 Why M8a-prep (new in v2)

Three reasons one HIGH-severity review finding per:

- **Live-write safety:** the migration framework cannot do expand/validate/contract phases today. Without that, M8a's working-tier introduction blocks the write path during the constraint swap (codex finding #3).
- **Relay hardening before worker:** shipping the Consolidation Worker (M8b) on the current unhardened relay (codex C-1 from M7) inverts safety. The worker amplifies the mid-batch crash hazard, not mitigates it. v1 wrongly placed relay hardening in M8d *after* M8b was live (Plan finding HC-5).
- **Allowlist before emission:** consumers (vector projector at `outbox_vector_publisher.go:31`) silently drop unknown event types. The allowlist for `memory_promoted` (committed in M7 spec §15 but missing from v1 of this umbrella) and `memory_dedupe_collapsed` (new in v2 §4.3) must extend before any code emits them.

### 3.2 Why M8b and M8c are *actually* orthogonal now (Plan W-1 mitigation)

The v1 orthogonality claim was asserted, not demonstrated. v2 demonstrates:

- M8a defines `Promoter` and `Deduper` interfaces over the **durable** record store, not over the in-memory `scoredStore`. M8b's worker calls these against the SDK abstractions only.
- M8c's scoredStore lift+refactor is an *internal-implementation* change: same SDK surface, different in-memory engine. M8b doesn't read `*scoredStore` directly — it goes through `RecordStore.GetRecord` etc.
- The only remaining shared surface is `MemoryItem` (lifted from core to sibling in M8c). M8b consumes it via SDK return values; if M8c renames typed fields on `MemoryItem`, M8b sees that as a routine SDK upgrade, not a behavioral change.

## 4. Locked Cross-Cutting Decisions

### 4.1 Working tier rollout: expand-first / dual-write / validate / contract

**(v2 — complete rewrite)** Replaces v1's single-statement constraint swap and dead `WHERE kind IS NULL` back-fill.

Five phases. Each phase is one migration framework version (`SchemaVersion N → N+1 → ...`). M8a-prep delivers the staged-migration capability; M8a uses it.

**Phase 1 (M8a-prep, online-safe).** Add a new CHECK constraint as `NOT VALID`:

```sql
ALTER TABLE memory_record
  ADD CONSTRAINT memory_record_kind_v2_check
  CHECK (kind IN ('working', 'episodic', 'semantic')) NOT VALID;
```

`NOT VALID` skips existing-row validation and takes only `SHARE UPDATE EXCLUSIVE` (no read block).

**Phase 2 (M8a-prep, off-peak).** Validate the constraint against existing rows. Since every existing row has `kind IN ('episodic','semantic')`, validation passes trivially:

```sql
ALTER TABLE memory_record VALIDATE CONSTRAINT memory_record_kind_v2_check;
```

**Phase 3 (M8a, dual-write window).** SDK code path begins choosing `kind`:

- `WriteRecord` callers that explicitly request `kind='working'` succeed (new acceptance).
- `WriteRecord` callers that don't specify kind continue defaulting to `'episodic'` (back-compat — keeps existing gateway paths identical).
- A new SDK helper `MemoryRecord.SetWorkingDefault()` makes the new-write path opt-in per call site.

This phase is **dual-write**, not back-fill. Existing rows are untouched and stay `'episodic'`. New writes that go through the new helper land as `'working'`. Provenance for "was this row promoted from working" is **`consolidated_from_event_id IS NOT NULL`** (Plan finding mitigation), NOT `kind='episodic'` (which can't distinguish "always was episodic" from "was promoted").

**Phase 4 (M8b, worker turned on).** Worker calls `Promoter.Promote(...)` which sets `kind='episodic'` AND populates `consolidated_from_event_id` in one transaction. The "was promoted" semantic is reliably queryable via the column, not the kind value.

**Phase 5 (M8d-tail, contract).** Once all new gateway write paths use the helper from Phase 3 and the worker is steady-state in Phase 4, drop the old CHECK constraint:

```sql
ALTER TABLE memory_record DROP CONSTRAINT memory_record_kind_check;
```

The new constraint (added in Phase 1) becomes the only one.

**Rollback path.** Any phase fails: roll back to the prior `SchemaVersion`. Phase 1's `NOT VALID` constraint is removable with `DROP CONSTRAINT`. Phases 3-4 are code-level rollbacks (no DDL). Phase 5 is delayed long enough that rollback is "don't drop yet" — trivial.

### 4.2 Promoter interface (separate from RecordStore — v2 change from extending it)

**(v2 — interface-extension pattern.)** v1 extended `RecordStore` with `PromoteRecord`, which (a) breaks every mock and (b) mixes idempotency patterns. v2 introduces a new SDK interface:

```go
// llm-agent-memory/memory/durable.go (added in M8a)
type PromoteRecordInput struct {
    TenantID        string
    MemoryID        string
    ExpectedVersion int64
    SourceEventID   string  // outbox-row provenance; populates consolidated_from_event_id
    IdempotencyKey  string  // composed by worker as sha256(tenant||memory||event_id||"promote")
    Reason          string  // free-form until M8d enum freeze
}

type PromoteRecordResult struct {
    MemoryID string
    Version  int64
    Record   MemoryRecord
    Created  bool  // true if first promote at this version; false if idempotent replay
}

type Promoter interface {
    Promote(ctx context.Context, in PromoteRecordInput) (PromoteRecordResult, error)
}
```

The Postgres backend (`postgres.Store`) implements both `RecordStore` (existing) and `Promoter` (new) on the same struct. Existing test fakes that satisfy `RecordStore` are **unchanged** — `Promoter` is taken by the worker only, and the worker mocks it separately.

**Event emission.** `Promote` emits the `memory_promoted` event type as part of the same transaction that updates `memory_record`. The event payload's `EventType = "memory_promoted"`. M8a-prep already extended the allowlist for this value before M8a code paths emit it (§3.1 ordering rule).

**Idempotency.** `IdempotencyKey` is required (not optional). The Postgres backend uses the same `memory_idempotency` table as `WriteRecord`. Same `(tenant_id, idempotency_key)` returns the prior result; different hash returns `ErrIdempotencyConflict`. The worker derives the key deterministically from the outbox EventID so relay redelivery is a no-op.

**Why a new interface (Plan finding W-2 + codex finding #6 mitigation):**

- Doesn't break `RecordStore` mocks (gateway has 3, plus the production `postgres.Store`).
- Doesn't break published v1.0.0 downstream consumers (`replace`-directive concern from codex).
- Cleanly separates "worker writes promote events" from "gateway writes user-initiated records." Different idempotency, different reason-enum, different callers.

**SDK version impact.** Adding `Promoter` is purely additive — new type, new interface, no existing interface mutates. `llm-agent-memory` SDK bumps to **v1.1.0 (additive)**, no v2.0.0 needed for this change. (v2.0.0 is reserved for M8c's storage refactor + snapshot v2.)

### 4.3 Deduper interface + loser-resolution semantics (v2 — adds tombstone path)

**(v2 — complete loser contract.)** v1 said `ResolveDedupe` returns `WinnerID + Action` but never specified what the loser's caller does next.

```go
type DedupeAction int
const (
    DedupeNoCollision     DedupeAction = iota  // candidate stored, no dedupe row existed
    DedupeMergedExisting                       // candidate collapsed into existing winner
    DedupeCollapsedByPin                       // existing winner is pinned; loser is dropped
)

type ResolveDedupeInput struct {
    TenantID   string
    DedupeKey  string  // sha256(tenant||user||category||project_id||normalize(content))
    Candidate  MemoryRecord  // full record, not just ID — needed for tombstone payload
}

type ResolveDedupeResult struct {
    WinnerID  string
    Action    DedupeAction
}

type Deduper interface {
    ResolveDedupe(ctx context.Context, in ResolveDedupeInput) (ResolveDedupeResult, error)
}
```

**Storage:** new `memory_dedupe_index(tenant_id, dedupe_key, winner_memory_id, created_at)` with `UNIQUE (tenant_id, dedupe_key)`. Migration lands in M8a (after M8a-prep migration framework is online).

**Loser-resolution flow (when `Action = DedupeMergedExisting`):**

1. Backend transaction inserts into `memory_dedupe_index`; the `UNIQUE` constraint deterministically picks the first writer as winner. The loser's transaction sees the constraint violation and reads the winner row.
2. Backend writes loser's `memory_record.deleted = TRUE` (schema already supports this — column exists).
3. Backend emits a **new event type `memory_dedupe_collapsed`** with payload `{winner_id, loser_id, tenant_id}`. M8a-prep already extended the allowlist for this value.
4. Vector projector handles `memory_dedupe_collapsed` and removes the loser's vector chunks. **(v2.1 correction.)** The projector's `outbox_vector_publisher.go:31-72` is a hard-coded switch on event type with a `default → "unsupported_event"` no-op branch. **Adding the event to the allowlist alone is not sufficient** — M8a-prep must commit to adding an explicit `case "memory_dedupe_collapsed"` arm that calls the equivalent of `ProjectRemove(ctx, loserID)`. This is a code-handler deliverable, not just config.
5. Any *pending* outbox rows for the loser memory_id reach the projector AFTER the collapse event; the projector's existing `GetRecord` check returns the loser as `deleted` and the publisher marks the row stale (existing behavior at `outbox_vector_publisher.go:35,42`).

**Net property:** the loser is removed from durable storage AND from vector indexes AND its remaining outbox messages are non-destructive (they no-op against the deleted record). No orphans.

**Dedupe key normalization:** lowercase + collapse whitespace + strip ASCII punctuation. Unicode-aware normalization deferred (M7 W-1). Dedupe key **includes `project_id`** (Plan finding S-3 mitigation): two users in different projects with identical content do not collide.

**`session_id` is NOT in the key** by design: same content across sessions IS a valid dedupe candidate.

### 4.4 LastAccessAt batched async writes (cardinality fix)

**(v2 — restore `tenant_bucket` label.)** v1 said `last_access_write_failures_total` has no labels. v2 keeps the `tenant_bucket` label per cardinality convention. Operators want per-tenant attribution.

Mechanism unchanged from v1: bounded channel + writer goroutine + UPDATE batched across hits. On failure: `last_access_write_failures_total{tenant_bucket}` increments. On drop (channel full): `last_access_dropped_total{tenant_bucket,reason}` with reasons `buffer_full / shutdown` (mirrors `trace_dropped_total` shape from M7).

**Plan W-3 acknowledgment:** under sustained load, drops accumulate. The dropped counter signals this. M8d's `working_dropped_before_use_total` interpretation must account for the gap between "no recall happened" and "recall happened, last_access write dropped." Acceptable for v2.0.0 telemetry use; documented at M8d spec time.

### 4.5 scoredStore relocation — package-cluster lift with benchmark first

**(v2 — corrected scope.)** v1 said "copy `internal_score.go` + tests." Verified false in §2.5.

M8c's first deliverable is a **benchmark harness** in `llm-agent-memory/memory/internal_score_bench_test.go`. Workload mix: 80% read / 20% write, 10k items, 16 concurrent goroutines. Reports baseline before any concurrency change is made.

The lift then proceeds as a **package-cluster move**:

1. Copy 8 core files: `internal_score.go`, `memory.go` (for `MemoryItem`, `Kind`, `Stats`, `Memory` interface), `persistence.go` (snapshot/import helpers), `working.go` (`WorkingMemory`'s direct `*scoredStore` dependency), `episodic.go` + `semantic.go` (same — verified during v2.1 review), `recall.go` (`listFromStore` is `*scoredStore`-typed), and the `storeOf` accessor from `manager.go`.
2. Update sibling files that import from core: `recall_engine.go`, `consolidator.go`, `parallel_search.go`, `manager.go`, `consolidator_test.go`, `testutil_test.go` (6 files, verified during v2 review).
3. Core retains the original files as deprecation patches (announce in core CHANGELOG; final patch tag).
4. Refactor concurrency in the sibling-local copy. Strategy decided by benchmark, not opinion. The benchmark target is "≥2× concurrent-read throughput at 10k items" (roadmap exit criterion).

**Sub-decision deferred to M8c spec:** which concurrency strategy. Three candidates from roadmap §6.1. Benchmark methodology pinned in M8c spec.

### 4.6 Reason-enum freeze readiness criteria (v2 — quantified)

**(v2 — adds objective trigger.)** v1 said "freeze from staging data" with no threshold.

M8d kickoff requires:

- ≥10,000 rows in `memory_decision_trace` covering ≥14 days of real traffic.
- ≥3 distinct tenant_ids in the trace.
- The top-10 most-frequent `reason` values cover ≥80% of trace rows.
- Long-tail reasons (<1% of rows individually) covered by an `'other'` bucket.

If any of the above fails at M8d kickoff, **M8d delays** (not "freeze on inadequate data"). Status check via the queries in `docs/m7-staging-observability.md` (S1, S5).

### 4.7 Versioning + release sequencing (v2 — addresses `replace`-directive caveat)

**(v2 — explicit lockstep policy.)** Codex finding #6: local `go.mod` replace directives hide what published consumers see.

Required lockstep tagging:

- **M8a-prep ship:** `llm-agent-memory v1.0.1` (migration framework only — bug-fix-level, backwards-compatible additions to `Migrate()` shape) and `llm-agent-memory-postgres v0.x.y` (relay hardening + new event-type allowlist).
- **M8a ship:** `llm-agent-memory v1.1.0` (additive: `Promoter`, `Deduper` interfaces) and `llm-agent-memory-postgres v0.(x+1).0` (impls). Gateway pinned to both new versions in lockstep.
- **M8b ship:** new `llm-agent-memory-worker v0.1.0` sibling, depends on the v1.1.0 SDK.
- **M8c ship:** `llm-agent-memory v2.0.0` (storage refactor + snapshot v2 + MemoryItem typed-field lift). Other siblings track this with their own major bump.
- **M8d ship:** `llm-agent-memory v2.x.y` (typed RecallObservation extension — additive on v2). Gateway + worker pinned to it.

**No interface mutates without a major-version bump.** `Promoter` and `Deduper` are additive (new types). `MemoryItem` field changes (M8c) are breaking and gate v2.0.0.

Existing `go.mod` `replace` directives stay during umbrella development. Each ship cuts a real tag and bumps the consumer go.mod to the tag, dropping the replace.

## 5. Cross-Module Impact (v2 — expanded)

### 5.1 `llm-agent-memory` (SDK)

- M8a-prep: bug-fix patch (migration framework helper). v1.0.1.
- M8a: add `Promoter`, `Deduper`, paired input/output types, `MemoryRecord.SetWorkingDefault()` helper. v1.1.0 additive.
- M8c: full scoredStore package-cluster move + concurrency refactor + `MemoryItem` typed-field lift. **v2.0.0 major.**
- M8d: extend `RecallObservation` typed fields. v2.1.0 additive.

### 5.2 `llm-agent-memory-postgres`

- M8a-prep: implement staged-migration support; wire `outboxStatusProcessing` writes (relay hardening); extend in-code event-type allowlist with `memory_promoted` + `memory_dedupe_collapsed`.
- M8a: schema migrations for kind-v2 CHECK + `memory_dedupe_index` table. Implement `Promoter`, `Deduper` on `Store`.
- M8d: schema migration for `memory_decision_trace.reason` enum freeze (validate against frozen value set; rows that don't match get `reason='legacy_unmigrated'`).

### 5.3 `llm-agent-memory-gateway`

- M8a: wire batched `LastAccessAt` writer; emit `last_access_*` counters.
- M8c: track sibling SDK major bump; rewire any `MemoryItem` field access that moves from `Metadata` to typed fields.
- M8d: extend `RecallObserver` impl; emit deferred 7 counters; add feedback ingest HTTP endpoint.

### 5.4 New module: `llm-agent-memory-worker` (M8b)

Brand new sibling. Module shape mirrors gateway (config / transport / service / observability). Depends on `llm-agent-memory v1.1.0` for `Promoter` + `Deduper`.

### 5.5 `llm-agent` (core)

- M8a-prep / M8a: no change (core was deprecation-frozen at M4).
- M8c: deprecation patch — announce in `CHANGELOG.md` that the storage engine has moved; final patch tag cuts. Core stays buildable but emits deprecation notices on import.

## 6. Non-Goals

- Salience / learned rerank / decay learning. Out of v2 per memory-roadmap §11.3.
- Vector-similarity dedupe. P2; gated behind opt-in flag, default off.
- Cross-region or DR planning. Out of M8 entirely; v3 concern.
- Replacing the M7 trace-sink's free-form `reason` column with the frozen enum at the column level. The M8d migration retags rows but the column stays `text` for forward-compat with future enum extensions.

## 7. Acceptance Criteria for This Umbrella v2

The umbrella v2 is approved when:

1. The 5-stage sub-milestone split (§3) is endorsed: M8a-prep is necessary, not optional.
2. The 7 locked decisions (§4.1–§4.7) are endorsed without exception. Any "I'd prefer differently" item demotes back to "open question" and re-surfaces at the relevant sub-milestone spec.
3. The Promoter/Deduper interface-extension pattern (vs RecordStore method addition) is endorsed (§4.2 / §4.3).
4. Lockstep versioning policy (§4.7) is endorsed: every interface change ships with a paired sibling version bump.

Once §7 is signed off, sub-milestone specs branch:

- `docs/superpowers/specs/<date>-m8a-prep-migration-framework-and-relay.md` ← first
- `docs/superpowers/specs/<date>-m8a-substrate-design.md` ← then
- `docs/superpowers/specs/<date>-m8b-consolidation-worker-design.md` and `<date>-m8c-storage-refactor-design.md` in parallel
- `docs/superpowers/specs/<date>-m8d-typed-observer-and-enum-freeze-design.md` last

Each goes through the two-round review pattern before its plan is written.

## 8. Review-Finding Disposition (v1 → v2)

How each of the 20 round-1 and round-2 findings landed:

| Finding | Source | Disposition in v2 |
|---|---|---|
| HIGH — back-fill `WHERE kind IS NULL` is dead code (kind is NOT NULL) | codex | §4.1 fully rewritten; provenance via `consolidated_from_event_id IS NOT NULL`, not `kind` |
| HIGH — migration framework can't do expand/contract; need staged transitions | codex | §3.1 + §4.1 add M8a-prep with staged-migration capability |
| HIGH — M8a not additive; current write path emits `kind` verbatim | codex | §4.1 Phase 1+2 (NOT VALID then VALIDATE) lands BEFORE any write-path code change |
| HIGH — ALTER constraint takes ACCESS EXCLUSIVE | Plan | §4.1 Phase 1 uses `NOT VALID` (SHARE UPDATE EXCLUSIVE only) |
| HIGH — `memory_promoted` event never mentioned in umbrella | Plan | §3.1 + §4.2 commit allowlist extension to M8a-prep; emission to M8a Promoter |
| HIGH — back-fill mislabels `agent_inferred` as "promoted" | Plan | §4.1 Phase 3 keeps existing rows untouched (still `episodic`); provenance via `consolidated_from_event_id` |
| HIGH — PromoteRecord idempotency key doesn't match WriteRecord's | Plan | §4.2 adds `IdempotencyKey` to input; aligns with `memory_idempotency` table |
| HIGH — ResolveDedupe loser-contract undefined | Plan + codex | §4.3 specifies loser flow: deleted=TRUE + `memory_dedupe_collapsed` event + vector cleanup |
| HIGH — ResolveDedupe orphan outbox messages | codex | §4.3 step 5: loser's pending outbox rows no-op against deleted record |
| HIGH — relay hardening placement wrong (worker before relay safe) | Plan | §3 moves relay hardening to M8a-prep |
| HIGH — version-skew / `replace`-directive hides break | codex | §4.7 lockstep tagging policy; §4.2 Promoter interface avoids breaking RecordStore mocks anyway |
| HIGH — interface extension recommended over RecordStore method add | codex | §4.2 + §4.3 use Promoter/Deduper separate interfaces |
| MEDIUM — scoredStore lift scope under-claimed | Plan + codex | §4.5 + §2.5 quantify: 5+ files in lift, 6+ sibling import updates, plus benchmark harness creation |
| MEDIUM — no benchmark harness exists | codex | §4.5 step 1 of M8c is benchmark harness creation |
| MEDIUM — concurrency strategy decision unmethodical | Plan | §4.5 defers to benchmark; methodology pinned in M8c spec |
| MEDIUM — staging-data threshold for enum freeze qualitative | Plan | §4.6 quantifies: 10k rows / 14 days / 3 tenants / top-10 ≥80% |
| MEDIUM — M8b ∥ M8c orthogonality asserted not demonstrated | Plan | §3.2 demonstrates: Promoter/Deduper are durable-side; in-memory refactor stays M8c-internal |
| MEDIUM — last_access cardinality convention violation | Plan | §4.4 restores `tenant_bucket` label |
| MEDIUM — M8d bundles relay + observer (unrelated workstreams) | Plan | §3 splits: relay → M8a-prep; observer → M8d (sole focus now) |
| LOW — `memory_dedupe_index` retention not specified | Plan | §4.3 storage: rows persist for the life of the dedupe relation (long-lived index, like primary key); GC tied to hard-delete of winner |

All 20 findings have an explicit landing place. Nothing was silently dropped.

## 9. Known Weaknesses of This v2

- **W-1.** §4.1 Phase 5 contract step is delayed long enough that "real M8 completion" arrives only at M8d-tail. If a deployment never finishes the migration sequence, the old `kind_check` constraint survives forever as benign dead code. Operational, not correctness.
- **W-2.** §4.3's normalization (ASCII-only) collides false-positives for users typing the same content in different cases. Unicode-aware deferred. Document at M8a spec time.
- **W-3.** §4.6's threshold (10k rows / 14 days) is operator-friendly but assumes M7 actually gets staging traffic of that scale. If M7 sits idle longer than 14 days, M8d delays automatically — feature, not bug.
- **W-4.** §4.7's lockstep tagging policy is a coordination rule, not a tooling enforcement. A future contributor could ship `llm-agent-memory v1.2.0` without bumping postgres or gateway. Tooling-side enforcement (CI check) is M8a-prep-adjacent but not in scope here.
- **W-5.** The Promoter / Deduper interface-extension pattern works for current callers, but if a future caller wants atomic "promote-and-dedupe-in-one-tx" semantics, they'd need a composite operation that doesn't exist in either interface. Recognized; designed-in if needed at sub-milestone spec time.
- **W-6 (from v2.1 review).** §4.1 Phase 5 (drop old constraint) is described as "delayed long enough" — informal. M8d-tail sub-spec must pin an objective gate: e.g., "no row with `kind NOT IN ('working','episodic','semantic')` for ≥7 consecutive days in staging + production." Without this, Phase 5 risks dropping a constraint while ambiguous rows survive.
- **W-7 (from v2.1 review).** §4.2's `Promoter.Promote` semantics around `ExpectedVersion` are not pinned: does the Postgres impl re-read under row-lock, or does it trust the caller's `ExpectedVersion` and let the worker retry on `ErrVersionConflict`? Both are valid; the umbrella punts to M8a sub-spec. Recommend re-read under row-lock (matches existing `mutateRecord` pattern) and document there.
- **W-8 (from v2.1 review).** §4.7's lockstep tagging is a coordination rule with no CI tooling today. M8a-prep sub-spec should add a `make verify-lockstep` script that fails CI if a sibling's `go.mod` references an unpublished tag.
- **W-9 (from v2.1 review).** §4.1's dual-write window (Phase 3) has an unspecified idempotency-replay edge case: if a Phase-3 write fails after the new constraint exists but before the old is dropped, replays of the same `idempotency_key` will return failure. Documented; will not affect sub-milestone branching. M8a sub-spec should clarify.
