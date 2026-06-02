# M8 Working-Memory Lifecycle — Implementation Plan

> Date: 2026-06-02
> Status: **Ready to execute (steps 1–7). Step 0 already DONE.**
> Design basis (locked, two-round reviewed): [`../../m8-working-memory-lifecycle-enablement.zh-CN.md`](../../m8-working-memory-lifecycle-enablement.zh-CN.md) (v4). Decisions D1–D8 are locked in §7 of that doc; this plan derives the executable steps from it. Do not re-litigate decisions here — if a decision needs changing, amend the design doc first.
> Parent: [`../specs/2026-05-27-m8-umbrella-design.md`](../specs/2026-05-27-m8-umbrella-design.md).

## 0. Scope recap (why this is multi-repo, not single-repo activation)

D3 (sink promotion contract to `llm-agent-memory-contract`) + D8 (add write fence) were taken as the **heavier** options. Net: this spans **4 repos** (contract + postgres + worker + gateway) with a **contract version bump (v0.1.0 → v0.2.0)** and a **write-API semantic change**, requiring lockstep tagging. Treat it as milestone-scale coordination, not a one-shot.

Baseline facts (verified 2026-06-02):
- contract latest tag `v0.1.0`; gateway and worker both pin `v0.1.0` with no `replace` directives.
- contract has **no** existing promotion/dedupe helpers → D3 additions are purely additive.
- gateway promotion logic: `llm-agent-memory-gateway/internal/service/durable_session_closer.go` (`shouldPromoteOnSessionClose`, `sessionCloseDedupeKey`, threshold `0.7`).
- worker promotion logic: `llm-agent-memory-worker/internal/service/consolidation_publisher.go` (`shouldPromote`, `dedupeKey`, threshold `0.7`). The two are **judgment-equivalent** on the shared surface (source→eligibility, 0.7, dedupe-key construction) but not byte-identical (names/Reason/idempotency-salt differ deliberately — those stay per-caller).

## 1. Step 0 — DONE ✅ (postgres C1 fix)

`ResolveDedupe` first-writer race fixed via atomic `INSERT … ON CONFLICT DO NOTHING RETURNING`. Merged: `llm-agent-memory-postgres` **PR #2** (merge `6793bac`). This is a hard prerequisite for wiring a second concurrent dedupe caller (the session closer); it also fixes a latent production hazard on its own. No further action.

## 2. Tag / lockstep sequence (the critical ordering)

```
contract v0.2.0  (step 1: PromotionPolicy + DedupeKey constructor — additive)
        │
        ├─► worker  bump contract dep → v0.2.0, refactor to consume (step 2)  → tag worker
        └─► gateway bump contract dep → v0.2.0, refactor to consume (step 2)
                    │
                    ├─ adapter            (step 3, gateway-internal)
                    ├─ D7 metrics         (step 4, gateway-internal)
                    ├─ D8 write fence      (step 5, gateway-internal, API-semantic)
                    ├─ wire closer         (step 6, depends on 3 + 4 + step-0 postgres)
                    └─ D2 promote observ.  (step 7, depends on 6)
                            │
                            └─► gateway final tag
```

**Rule:** contract tag must land before worker/gateway bump their dep (per M8 umbrella §4.7 lockstep). No sibling references an unpublished tag. Each repo commits all PR changes before `gh pr create` (orphan-commit iron rule).

## 3. Steps

Each step: one branch → one PR per repo → atomic commits → `GOWORK=off go build ./... && go vet ./... && go test ./...` green → PR → auto-merge. Review between steps (one implementer at a time).

### Step 1 — contract: single source of truth (D3) → v0.2.0

- **Repo:** `llm-agent-memory-contract`
- **Add** (additive, new file e.g. `contract/promotion.go`):
  - `PromotionEligible(record MemoryRecord) bool` — the shared rule: `user_saved` → true; `agent_inferred` → `Importance >= PromoteImportanceThreshold`; else false.
  - `const PromoteImportanceThreshold = 0.7`.
  - `DedupeKey(record MemoryRecord) string` — the shared dedupe-key construction: `sha256(tenant || user || category || project_id || NormalizeDedupeContent(record))`, hex. Plus `NormalizeDedupeContent(record) string` (prefer `NormalizedContentHash` if set, else lowercase+collapse-whitespace of `Content`). Mirror exactly what gateway/worker compute today so the value is unchanged.
- **Do NOT** move: per-caller `Reason` strings, idempotency-key salts (`"session_close_promote"` vs `"promote"`) — those are intentionally distinct and stay in each caller.
- **Tests:** `promotion_test.go` — table tests for eligibility (all source/importance combos) + dedupe-key determinism + golden-value equivalence to the current gateway/worker output (copy a couple of known inputs, assert the hex matches what the existing code produces).
- **Validation:** build/vet/test green; no `go.mod` drift.
- **Release:** tag `v0.2.0` after merge (additive minor bump).

### Step 2 — worker + gateway: consume contract D3, delete duplicates

Two PRs (worker, gateway), each bumping contract to `v0.2.0` first.

- **worker** (`consolidation_publisher.go`):
  - Replace `shouldPromote` → `corememory.PromotionEligible`; `dedupeKey`/`normalizedDedupeContent` → `corememory.DedupeKey`/`NormalizeDedupeContent`; `agentInferredImportanceThreshold` → `corememory.PromoteImportanceThreshold`.
  - Keep `promoteReason`, `promotionIdempotencyKey` (salt `"promote"`) local.
  - Delete the now-orphaned local funcs.
  - Tests: existing worker tests stay green (behavior unchanged); add an assertion that the worker uses the contract constant (optional).
- **gateway** (`durable_session_closer.go`):
  - Replace `shouldPromoteOnSessionClose` → `corememory.PromotionEligible`; `sessionCloseDedupeKey`/`sessionCloseNormalizedDedupeContent` → `corememory.DedupeKey`/`NormalizeDedupeContent`; `sessionClosePromoteImportanceThreshold` → `corememory.PromoteImportanceThreshold`.
  - Keep `sessionClosePromoteReason`, `sessionClosePromotionKey` (salt `"session_close_promote"`) local.
  - Delete orphaned local funcs.
  - Tests: existing `durable_session_closer_test.go` stays green.
- **Anti-divergence guard (D3):** add a cross-repo consistency test or an umbrella gate (analogous to `scripts/regex-parity-check.sh`) asserting both consumers call the contract helpers and the threshold constant — so a future edit can't silently re-fork. (Lightweight: a grep-gate that fails if `0.7` literal or a local `shouldPromote` reappears in either file.)
- **Release:** tag worker `v0.x.(y+1)`; gateway not yet (more gateway steps follow).

### Step 3 — gateway: `sessionWorkingStore` adapter

- **Repo:** gateway. **File:** new `internal/service/postgres_session_working_store.go` (+ test).
- **Add** an adapter wrapping `*pgmemory.Store`:
  - Embed/forward `corememory.RecordStore` + `corememory.Promoter` + `corememory.Deduper` (the embedded `*pgmemory.Store` already satisfies these).
  - Implement `ListSessionWorking(...) ([]service.SessionWorkingRecord, error)` by calling the store's `ListSessionWorking` (returns `[]pgmemory.SessionWorkingRecord`) and converting each element to `service.SessionWorkingRecord{Record, LatestEventID}`.
- **Tests:** unit test the type conversion (fake/stub store returning pgmemory records → adapter yields service records 1:1, fields preserved); nil-safety.
- **Validation:** build/vet/test green.

### Step 4 — gateway: D7 lifecycle metrics (tenant_bucket)

- **Repo:** gateway. **Files:** `internal/observability/metrics.go` (+ test), `internal/service/working_lifecycle_observer.go` consumer wiring.
- **Add** to `Metrics`: bucketed counters `working_expired_total` and `working_dropped_before_use_total`, both labeled `tenant_bucket` (reuse `service.TenantBucket`; unify empty-tenant to the gateway rule). `AddWorkingExpired(tenantID)`, `AddWorkingDroppedBeforeUse(tenantID)` + snapshot render.
- **Add** `Metrics.WorkingLifecycleObserver() service.WorkingLifecycleObserver` returning an impl that maps `ObserveWorkingLifecycle(obs)` → `AddWorkingExpired(obs.TenantID, obs.Expired)` / `AddWorkingDroppedBeforeUse(obs.TenantID, obs.DroppedBeforeUse)`.
- **Tests:** observer increments counters by the observation counts, buckets by tenant; zero-obs is a no-op; snapshot exposes the metrics.
- **Validation:** build/vet/test green.

### Step 5 — gateway: D8 write fence (API-semantic change)

- **Repo:** gateway. **Files:** `internal/service/service.go` (`WriteMemory` ~261, `PatchMemory` ~307, `PinMemory` ~392, `DeleteMemory` ~569); api-contract doc.
- **Change:** at the start of each mutator (after scope merge, before backend write), call `validateSessionState` against the session and reject with the existing closed-session error when `Status=="closed"` (and honor the existing expired handling). Match the pattern already used by `RecallUnified` (service.go:119-123).
  - Decide (in the PR): whether closed→reject applies to all four mutators or only `WriteMemory` (creation). Recommend all four for a consistent terminal semantic; confirm against api-contract.
- **Docs:** update `docs/memory-gateway-api-contract.zh-CN.md` — closed-session write goes from silent-success to 4xx (`session is closed`). Note this is a **behavioral/contract change**.
- **Tests:** writing/patching/pinning/deleting on a `closed` session → 4xx; on an `active` session → unchanged; late-write race (close then write) → rejected.
- **Risk gate:** this is the one step that changes external behavior. Verify no internal caller (e.g. the closer itself) writes after marking closed. The closer mutates BEFORE `sessionRegistry.Close` (service.go:636-641), so it is unaffected — confirm with a test.

### Step 6 — gateway: wire the closer

- **Repo:** gateway. **File:** `cmd/memory-gateway/main.go:140`.
- **Change:** `noOpSessionCloser{}` → `service.NewDurableSessionCloser(adapter, metrics.WorkingLifecycleObserver())`, where `adapter` wraps the production `store` (step 3) and the observer is from step 4.
- **Depends on:** steps 0 (postgres C1, merged), 3 (adapter), 4 (observer); shares the contract policy from step 2.
- **Tests:** integration — `POST /sessions/{id}/close` with `expire_working` reclaims all session working records and increments `working_expired_total`; with `promote_and_expire` promotes eligible (kind→episodic) and expires the rest; read-only gateway rejects via `ensureWritable`; already-closed replay short-circuits (no double promote/expire, no duplicate trace). End-to-end "dead metric → live" assertion.

### Step 7 — gateway: D2 promote observability

- **Repo:** gateway. **Files:** `internal/service/working_lifecycle_observer.go`, `durable_session_closer.go`, `internal/observability/metrics.go`.
- **Change:** add `Promoted int` to `WorkingLifecycleObservation`; in `DurableSessionCloser.CloseSession` count promotions and **change the firing condition** (`durable_session_closer.go:79`) so a promote-only close also emits an observation (currently fires only when `expired>0 || droppedBeforeUse>0`). Add `working_promoted_total{tenant_bucket}` (session-close path) to metrics + observer.
- **Tests:** a `promote_and_expire` close that promotes N, expires 0 → observation fires with `Promoted=N`; `working_promoted_total` increments.
- **Release:** gateway final tag after this merges.

## 4. Cross-cutting

- **Validation gate (every PR):** `GOWORK=off go build ./... && go vet ./... && go test ./...` green; no `go.mod` drift (CI currency gate); live-DB tests (postgres/gateway) verified locally against a throwaway postgres (`LLM_AGENT_MEMORY_PG_URL`) since CI skips them without a DB.
- **Lockstep tags:** contract `v0.2.0` → worker bump+tag → gateway bump (then gateway steps 3–7) → gateway final tag. Never reference an unpublished tag.
- **Rollback:** step 1 additive (safe). Step 2 code-level (revert + re-pin). Step 5 is the riskiest (API semantic) — gate behind review; rollback = revert the mutator fence (no DDL). Step 6 wiring revert = restore `noOpSessionCloser{}`.
- **Execution discipline:** one implementer/PR at a time; review between steps; `git stash -u` untracked siblings; delete any out-of-scope generated code. (Per established polyrepo discipline.)
- **Out of scope (decided):** D6 orphaned-session reaper (idle sweep); hard-delete GC of expired working rows. Tracked as future items; do not creep them in.

## 5. Acceptance (rolls up design §8)

1. Production gateway: explicit `/close` runs real expire/promote (both modes); integration-tested.
2. `working_expired_total` / `working_dropped_before_use_total` (+ `working_promoted_total` from step 7) exposed on `/metrics`, tenant-bucketed, non-zero when reclamation happens ("dead metric → live").
3. No double-promote across worker + closer (idempotency + version fence + the step-0 race fix).
4. D8: closed-session writes rejected (4xx); behavior change reflected in api-contract doc + tests.
5. D3: both consumers use the contract helpers; anti-divergence guard in place.
6. All four repos build/vet/test green; lockstep tags cut in order.
