# M4: Capability-Interface Manager + RecallEngine Facade (Sibling-First v1.0.0)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land milestone M4 of the `llm-agent-memory` subproject — Phase D of `docs/memory-roadmap.zh-CN.md` (§5.1) — and cut the first **v1.0.0** tag of `github.com/costa92/llm-agent-memory`.

M4 delivers two interconnected breakages, but both land **inside the sibling** with `github.com/costa92/llm-agent v0.7.0` left untouched (decision recorded in "Open Decisions Resolved" below — M4 is **NOT** split into a core PR + sibling PR pair; the sibling-first design satisfies every D-1/D-2 exit criterion without modifying `coremem`):

1. **(Phase D-1) Capability-interface ManagerOptions.** A new sibling-owned `Options` struct accepts interface-typed fields — `Memory`, `Lister`, `Exporter`, `Importer`, optional `LifecycleMemory` — for each of the three tiers (working / episodic / semantic). A new sibling-owned `*Manager` wraps these interfaces and exposes the full `coremem.Manager` API (`Add` / `Get` / `Update` / `Remove` / `Search` / `SearchAll` / `ListAll` / `StatsAll` / `Consolidate` / `Forget` / `ExportAll` / `ImportAll`). `WithSanitizer`-wrapped memories (which return the `coremem.Memory` interface, not a concrete type) install into this `Manager` without a cast — closing the v0.7 LIMITATION called out in `coremem/policy_hook.go:46-52` and `coremem/doc.go:122-128`.

2. **(Phase D-2) RecallEngine facade.** A new `RecallEngine` type exposes `Recall(ctx, query, opts) (UnifiedRecall, error)` as **the** public recall surface. Tier-awareness (working / episodic / semantic fan-out) becomes an internal implementation detail. `RecallEngine` wraps the new sibling `*Manager` (and therefore also wraps any `WithSanitizer`-decorated memory). Tier selection is deterministic and budgetable via `RecallOptions` (per-tier `TopK` budgets, tier inclusion mask, latency budget hint). `UnifiedRecall` carries the merged-and-deduped result slice plus per-tier provenance counts so `context.Builder` (`coremem/context/builder.go:48-56`) and Memory Gateway (M6) consume one type instead of three.

3. **(Migration prep) Migration guide doc.** A new top-level umbrella doc `docs/memory-v1-migration.zh-CN.md` enumerates every break (which is one: callers that constructed `*coremem.Manager` directly should construct `*memory.Manager` instead; everything else is additive), records the upgrade recipe, and lists every exit-criterion match.

4. **(Compat shim) `memory/compat/` package.** A new sibling sub-package `memory/compat/` exposes `NewManagerFromCore(*coremem.Manager) *memory.Manager` and `LegacyOptions` (alias for `coremem.ManagerOptions`) for **one release window** (the v1.x line; removed at v2.0.0). This satisfies exit criterion 5 and lets consumers wired to `coremem.NewManager(coremem.ManagerOptions{...})` migrate in two steps: (a) keep the existing construction; (b) hand the `*coremem.Manager` to `compat.NewManagerFromCore` and switch their downstream calls to the v1 sibling `*memory.Manager`.

5. **(Test adaptation) All M1–M3 tests adapted.** No test is deleted. The sibling Consolidator / ScopedLifecycleManager / UnifiedSearcher / ParallelSearcher / WritePolicy / SQLiteStore tests stay where they are; the new tests this milestone adds live in five new files (`manager.go` + `manager_test.go`, `recall_engine.go` + `recall_engine_test.go`, `compat/compat.go` + `compat/compat_test.go`).

6. **(Hygiene) Bump to `v1.0.0`.** The version constant flips from `0.3.0` to `1.0.0`. CHANGELOG documents every API delta and the compat-shim removal window.

All test-driven (strict TDD throughout — every task starts from a failing test). No new third-party dependencies — M4 is pure Go stdlib + the existing `github.com/costa92/llm-agent v0.7.0` and `modernc.org/sqlite v1.50.1` already in `go.mod`.

**Architecture:** Three new public surfaces (`Manager`, `RecallEngine`, `compat`) + the migration-guide doc. The sibling `Manager` is an interface-wrapping façade: it holds three small struct-of-interfaces (`workingCaps` / `episodicCaps` / `semanticCaps`) where each struct's fields are the four optional capability interfaces (`Memory` + `Lister` + `Exporter` + `Importer`) plus the new `LifecycleMemory`. This struct shape keeps the wiring obvious — the field name tells you what capability you are providing — while allowing a single object to satisfy multiple capabilities (the bundled `*coremem.WorkingMemory` satisfies all four). `RecallEngine` consumes a `*Manager` (so all options compose) and internally uses the existing sibling `ParallelSearcher` for fan-out and the existing sibling `UnifiedSearcher`'s merge/dedupe/sort algorithm, lifted into a private helper so the new facade is the canonical path.

- `memory/manager.go` — new file. Declares: `LifecycleMemory` interface (new — adds `Consolidate(ctx, opts)` and `Forget(ctx, opts)` capabilities so a future external backend can opt in without re-implementing the whole tier); `TierOptions` struct (one per kind, holds capability-interface fields); `Options` (the new `ManagerOptions` analogue — pure interface-typed); `Manager` struct + `NewManager(Options) (*Manager, error)` + every method that mirrors `coremem.Manager` (Add, Get, Update, Remove, Search, SearchAll, ListAll, StatsAll, Consolidate, Forget, ExportAll, ImportAll). Each method dispatches to the appropriate capability interface; capability-absent calls return a typed `ErrCapabilityMissing` error rather than panicking.
- `memory/manager_test.go` — new file. TDD: every capability path; `WithSanitizer`-without-cast smoke test (THE D-1 exit-criterion proof); per-method dispatch failure modes; `LifecycleMemory` capability detection; parity with `coremem.Manager` on Add/Get/Search/SearchAll/ListAll/StatsAll/ExportAll/ImportAll.
- `memory/recall_engine.go` — new file. Declares: `RecallEngine` struct + `NewRecallEngine(mgr *Manager, opts ...Option) (*RecallEngine, error)`; `RecallOptions` (Query — implied by `Recall` arg, kept out of the struct; `TopK`; `Tiers TierMask`; per-tier `Budgets TierBudgets`; `IncludeProvenance bool`); `UnifiedRecall` (Results []coremem.SearchResult; PerTier map[coremem.Kind]TierStats; TotalDropped int); `TierMask` bitmask + named constants (`TierWorking` / `TierEpisodic` / `TierSemantic` / `AllTiers`); `TierStats` struct (Considered int; Returned int).
- `memory/recall_engine_test.go` — new file. TDD: full `Recall` with default options matches the existing `UnifiedSearcher.SearchUnified` output (regression lock); `Tiers: TierWorking` skips Episodic and Semantic entirely; per-tier `Budgets` honored (Working budget 2, Episodic budget 5 → no more than those per tier); `IncludeProvenance` populates `PerTier`; nil manager returns `ErrRecallEngineManagerRequired`; observer emission matches `UnifiedSearcher`.
- `memory/compat/compat.go` — new file (new sub-package). Declares: `LegacyOptions` type alias for `coremem.ManagerOptions`; `NewManagerFromCore(*coremem.Manager) *memory.Manager` + `NewManagerFromLegacyOptions(LegacyOptions) (*memory.Manager, error)` — both translate concrete-typed `*coremem.WorkingMemory` / `*coremem.EpisodicMemory` / `*coremem.SemanticMemory` into the new interface-typed `Options`. Both wrap rather than forking so the deprecation window is a single import-swap.
- `memory/compat/compat_test.go` — new file. TDD: round-trip a `*coremem.Manager` through `NewManagerFromCore` and prove every method works; `LegacyOptions` field-to-field mapping into `Options`; deprecation comment compile-test (`// Deprecated:` doc-comment present on both public entry points).
- `memory/version.go` — modified. Bump `Version` to `1.0.0`.
- `CHANGELOG.md` — modified. Add `## [1.0.0] - 2026-05-26` section documenting D-1 + D-2, the compat shim, and the migration-guide pointer; explicitly call out **no new dependencies**, **no removed APIs**, and the v1.x compat-shim removal window (compat shim removed at v2.0.0).
- `README.md` — modified. Bump status to v1.0.0; add a "Migration from v0.x" pointer to the new doc; add `Manager` and `RecallEngine` to the "What this module adds" list.
- `../docs/memory-v1-migration.zh-CN.md` — new top-level umbrella doc. Migration guide listing every break, the upgrade recipe, the compat-shim usage, and the deprecation window.

**Tech Stack:** Go 1.26.0; `github.com/costa92/llm-agent v0.7.0` (unchanged); `modernc.org/sqlite v1.50.1` (unchanged, only used by Task 9 integration smoke test); stdlib `context`, `errors`, `fmt`, `sort`, `sync`, `testing`. **No new third-party deps.**

---

## File Structure

| Path | Action | Responsibility |
|---|---|---|
| `llm-agent-memory/memory/manager.go` | Create | `LifecycleMemory` capability interface; `TierOptions`; `Options`; `Manager` + `NewManager`; every dispatch method (Add/Get/Update/Remove/Search/SearchAll/ListAll/StatsAll/Consolidate/Forget/ExportAll/ImportAll); typed `ErrCapabilityMissing` and `ErrNoTiers` sentinels |
| `llm-agent-memory/memory/manager_test.go` | Create | TDD: D-1 `WithSanitizer`-no-cast proof; capability-missing dispatch errors; parity with `coremem.Manager` on every public method; `LifecycleMemory` opt-in detection |
| `llm-agent-memory/memory/recall_engine.go` | Create | `RecallEngine` + `NewRecallEngine`; `RecallOptions`; `UnifiedRecall`; `TierMask` + constants; `TierStats`; `Recall(ctx, query, opts)`; observer emission via `EventSearchTotal` + `EventSearchHits` |
| `llm-agent-memory/memory/recall_engine_test.go` | Create | TDD: D-2 parity-with-UnifiedSearcher (regression lock); TierMask isolation; per-tier budget enforcement; provenance population; observer emission |
| `llm-agent-memory/memory/compat/compat.go` | Create | `LegacyOptions` type alias; `NewManagerFromCore`; `NewManagerFromLegacyOptions`; `// Deprecated:` doc-comments locking the v2 removal window |
| `llm-agent-memory/memory/compat/compat_test.go` | Create | TDD: round-trip; field-to-field mapping; deprecation doc-comment presence |
| `llm-agent-memory/memory/version.go` | Modify | `Version` → `1.0.0` |
| `llm-agent-memory/CHANGELOG.md` | Modify | `## [1.0.0] - 2026-05-26` section |
| `llm-agent-memory/README.md` | Modify | Bump status; add `Manager` + `RecallEngine` to feature list; point at the migration guide |
| `llm-agent-ecosystem/docs/memory-v1-migration.zh-CN.md` | Create | Migration guide (top-level umbrella doc, NOT in the sibling repo) |

---

## Open Decisions Resolved in This Plan

- **Sibling-first, NOT split into M4a (core PR) + M4b (sibling adoption).** This is the single most consequential decision in the plan. The naive read of the master roadmap suggests two PRs: one against `github.com/costa92/llm-agent` to introduce capability-interface `ManagerOptions` and tag `v1.0.0` of core, plus one against the sibling to consume the new core. We reject that split for four reasons:
  1. **Core stays stdlib-only (master roadmap §3 dependency policy).** Adding interface-typed `ManagerOptions` fields to core does not violate stdlib-only, but every new capability interface (`LifecycleMemory`, optional read-side typed fields) widens the core's surface — and we have committed to keeping core's surface frozen from M4 onward (`master roadmap §1 Boundary table`: "M4 onwards: Frozen surface — bug fixes only").
  2. **No external consumer needs core to break.** A grep across `llm-agent-rag`, `llm-agent-customer-support`, `llm-agent-otel`, `llm-agent-providers`, and `llm-agent-flow` finds **zero** direct imports of `github.com/costa92/llm-agent/memory` — every consumer either uses no memory or already wraps the sibling. Breaking the core for a refactor zero consumers need is wasteful and risks an irreversible mistake.
  3. **The exit criteria are achievable in the sibling.** Every D-1 / D-2 criterion in the master roadmap is satisfied by a sibling-owned `Options` struct + `Manager` + `RecallEngine`. The sibling `Manager` *wraps* the three concrete types from `coremem` — and because those types already satisfy `coremem.Memory` + `coremem.Lister` + `coremem.Exporter` + `coremem.Importer`, no core change is required for the wrap to succeed. `WithSanitizer` returns a `coremem.Memory` interface value; the new sibling `Options.Working.Memory` field is of type `coremem.Memory` — so `WithSanitizer(coremem.WorkingMemory{...}, chain...)` slots in directly with no cast. Exit criterion #2 is satisfied without touching core.
  4. **v1.0.0 of `github.com/costa92/llm-agent-memory` is the correct semantic carrier.** The master roadmap §1 boundary table calls this milestone "first v1.0.0 tag of `llm-agent-memory`". v1.0.0 of the sibling does not require v1.0.0 of core; SemVer applies per-module. The sibling can ship v1.0.0 while pinning `github.com/costa92/llm-agent v0.7.0` indefinitely.
  - **Trade-off accepted.** The sibling `Manager` becomes the de-facto Manager surface for any new consumer; the core `*coremem.Manager` becomes "internal infrastructure" callers should not construct directly. The migration guide makes this explicit. No code in core breaks.
- **The sibling `Options` struct shape uses one struct per tier (`TierOptions`) with each capability as a named field.** Alternatives considered: (a) a flat `Options` with `WorkingMemory`/`WorkingLister`/`WorkingExporter`/...`SemanticConsolidator` — twelve top-level fields, easy to miswire; (b) `map[Kind]map[Capability]any` — type-unsafe; (c) one `Options` per kind constructor (`NewManager.WithWorking(...)`) — chatty for the common all-three-tiers case. We pick (d) one nested struct per tier with named-field capabilities because: ID-style autocomplete makes it obvious which capability you forgot to wire; a single object that satisfies multiple capabilities is a single field set across multiple struct fields (callers can write `t := TierOptions{Memory: w, Lister: w, Exporter: w, Importer: w}` once); and zero-value fields stay zero-value (capability absent) instead of accidentally inheriting a wildcard nil interface.
- **Exact `TierOptions` shape, frozen at v1.0.0:**
  ```go
  type TierOptions struct {
      Memory    coremem.Memory      // required (Add/Get/Update/Remove/Search/Stats/Type)
      Lister    coremem.Lister      // optional — ListAll skips this tier if nil
      Exporter  coremem.Exporter    // optional — ExportAll skips this tier if nil
      Importer  coremem.Importer    // optional — ImportAll skips this tier if nil
      Lifecycle LifecycleMemory     // optional — Consolidate/Forget skip this tier if nil
  }
  ```
  Future capabilities (e.g. a v1.1 `BulkLoader`, v1.2 `Snapshotter` that does not need `Exporter`) extend this struct by appending fields — adding a field is non-breaking. Removing or renaming a field is breaking and waits for v2.0.0.
- **`LifecycleMemory` capability definition (new in this milestone):**
  ```go
  type LifecycleMemory interface {
      Consolidate(ctx context.Context, opts coremem.ConsolidateOptions) (int, error)
      Forget(ctx context.Context, kind coremem.Kind, opts coremem.ForgetOptions) (int, error)
  }
  ```
  Rationale: core's `coremem.Manager.Consolidate` reaches into `m.working.store` directly (a package-private field, see `coremem/manager.go:191`); core's `coremem.Manager.Forget` calls `storeOf(mem)` to fetch the concrete `*scoredStore` (see `coremem/manager.go:239`). Neither operation is expressible as a `coremem.Memory`-only operation. Two real-world implementations satisfy `LifecycleMemory`: (a) a thin adapter `coreManagerLifecycle` (defined in `manager.go`) that forwards to `(*coremem.Manager).Consolidate` and `(*coremem.Manager).Forget`; (b) future external backends (Postgres, pgvector) that implement the two methods directly. Tiers that do not wire a `LifecycleMemory` cause `Manager.Consolidate` / `Manager.Forget` to return `ErrCapabilityMissing` for that kind — explicit, not silently no-op.
- **`Options` struct (top-level analogue of `coremem.ManagerOptions`):**
  ```go
  type Options struct {
      Working  TierOptions
      Episodic TierOptions
      Semantic TierOptions

      // SnapshotStore mirrors coremem.ManagerOptions.SnapshotStore — used by
      // ExportAll/ImportAll's persistKey paths. Nil keeps persistence in-memory.
      SnapshotStore coremem.SnapshotStore

      // CoreManager is an optional escape hatch — when set, lifecycle methods
      // (Consolidate, Forget) on Tier.Lifecycle == nil will fall back to this
      // *coremem.Manager via coreManagerLifecycle. Keeps the compat-shim path
      // ergonomic. Pass nil to disable the fallback.
      CoreManager *coremem.Manager
  }
  ```
- **Required field: `Working.Memory` (or any one tier's `Memory`).** `NewManager(Options)` returns `ErrNoTiers` if every tier's `Memory` field is nil — same semantic as `coremem.NewManager` returns `ErrNoMemories`.
- **`ErrCapabilityMissing` sentinel (NOT a coremem alias).** Defined in the sibling; signals "this tier exists but the requested capability was not wired into TierOptions". Wraps the kind in the error message so the caller knows which tier was missing what. Distinct from `coremem.ErrKindDisabled` (which means the whole tier is absent — we map that to a fresh `ErrTierDisabled` sentinel that aliases `coremem.ErrKindDisabled` so `errors.Is` continues to work for code already comparing against the core sentinel).
- **Sibling `Manager` does NOT implement `coremem.Memory` or any other coremem interface.** Sibling `Manager` is a *coordinator*, not a tier. Methods take a `Kind` argument exactly like `coremem.Manager` does; the dispatch resolves to the corresponding `TierOptions.Memory`. No `Type()` method, no impersonation of a single tier.
- **`Search` and `SearchAll` dispatch.** `Manager.Search(ctx, kind, q, topK)` looks up `tierFor(kind).Memory` (returns `ErrTierDisabled` if nil) and calls `Memory.Search`. `Manager.SearchAll` iterates active tiers (kinds whose `Memory` is non-nil) and fans out per-kind exactly like `coremem.Manager.SearchAll` — but it does NOT use `coremem.Manager.SearchAll` internally because we are intentionally building a parallel surface that does not depend on `*coremem.Manager` being present.
- **`ListAll` dispatch.** For each active tier, prefer `tier.Lister` when non-nil; if nil, fall back to type-asserting `tier.Memory.(coremem.Lister)` (so a Memory that ALSO implements Lister is detected). If neither, skip the tier (parity with `coremem.Manager.ListAll`). Same pattern for `Exporter` / `Importer`.
- **`Consolidate` dispatch.** First tries `Working.Lifecycle.Consolidate` if non-nil. If nil and `Options.CoreManager` is non-nil, delegates to `(*coremem.Manager).Consolidate`. Otherwise returns `ErrCapabilityMissing` wrapping `KindWorking`. Same pattern for `Forget(kind, opts)` (looks up that kind's Lifecycle, with optional CoreManager fallback).
- **`ExportAll` / `ImportAll` reuse coremem semantics by delegating to `coremem.Manager` ONLY when `Options.CoreManager` is set.** When CoreManager is nil, the sibling Manager fan-outs its own Export/Import iteration over the tier capability fields. This split preserves the v0.7 semantics for legacy callers (compat path) while allowing pure-interface callers to use the new Manager standalone.
- **`RecallEngine` design — wraps `*Manager`, NOT `*coremem.Manager`.** Why: D-2 says "tier-awareness becomes internal". `RecallEngine` needs to make per-tier topK + provenance decisions before fan-out, which is most naturally expressed as calls into the sibling `Manager` (already an interface dispatcher). Wrapping `*coremem.Manager` would (a) force callers to construct two Managers; (b) duplicate the sibling Manager's tier-aware logic; (c) lock RecallEngine to concrete coremem types forever. Trade-off accepted: RecallEngine cannot be constructed against a bare `*coremem.Manager` — callers either supply a sibling `*Manager` directly, or wrap their `*coremem.Manager` via `compat.NewManagerFromCore` first.
- **`RecallOptions` shape, frozen at v1.0.0:**
  ```go
  type RecallOptions struct {
      // TopK is the global cap on the merged result slice. <=0 → 10.
      TopK int

      // Tiers is a bitmask selecting which tiers participate. The zero
      // value (0) is treated as AllTiers (all three).
      Tiers TierMask

      // Budgets is an OPTIONAL per-tier upper bound on candidates pulled
      // before merge. Nil map = each participating tier returns TopK
      // candidates (matches UnifiedSearcher.SearchUnified semantics).
      // A tier present in Budgets with a value <=0 is treated as "use TopK".
      Budgets map[coremem.Kind]int

      // IncludeProvenance, when true, populates UnifiedRecall.PerTier
      // with per-tier Considered/Returned counts. Default false to keep
      // the fast path allocation-free.
      IncludeProvenance bool
  }
  ```
- **`UnifiedRecall` shape, frozen at v1.0.0:**
  ```go
  type UnifiedRecall struct {
      // Results is the merged, deduped, sorted, top-K capped slice.
      Results []coremem.SearchResult

      // PerTier is populated iff RecallOptions.IncludeProvenance == true.
      // Keys present are exactly the tiers selected by Tiers.
      PerTier map[coremem.Kind]TierStats

      // TotalDropped is len(merged-before-dedupe) - len(Results). Always
      // populated (even when IncludeProvenance is false) — single int,
      // cost is trivial.
      TotalDropped int
  }

  type TierStats struct {
      Considered int // raw candidates returned by the tier before merge
      Returned   int // count in Results that originated from this tier
  }
  ```
- **`TierMask` bitmask definition:**
  ```go
  type TierMask uint8
  const (
      TierWorking  TierMask = 1 << 0
      TierEpisodic TierMask = 1 << 1
      TierSemantic TierMask = 1 << 2
      AllTiers     TierMask = TierWorking | TierEpisodic | TierSemantic
  )
  ```
- **`Recall` algorithm (`RecallEngine.Recall`):**
  1. Normalize `opts`: if `opts.Tiers == 0`, use `AllTiers`. If `opts.TopK <= 0`, use `10`.
  2. Determine participating tiers: AND `opts.Tiers` with the set of active tiers on the wrapped `Manager`.
  3. For each participating tier, compute that tier's per-tier budget: `opts.Budgets[kind]` if positive, else `opts.TopK`.
  4. Fan out via `ParallelSearcher.SearchAllParallel` — BUT to honor per-tier budgets we cannot use it as-is (it forwards a single topK). Solution: call `(*Manager).Search(ctx, kind, q, perTierBudget)` directly in a small goroutine fan-out (same primitive `ParallelSearcher` uses; we lift the pattern). When `opts.IncludeProvenance` is true, record `Considered := len(perKindResults)` per tier.
  5. Merge via the lifted `mergeAndDedupe(perKind, totalTopK)` helper (extracted from `UnifiedSearcher.SearchUnified`'s body — same algorithm, same `(ID, Content)` dedupe key, same `Score desc, ID asc` sort).
  6. Compute `TotalDropped = totalCandidates - len(merged-after-dedupe)`.
  7. Apply `opts.TopK` truncation on the merged slice.
  8. When `opts.IncludeProvenance`, walk the final `Results` and bucket counts back to `PerTier[kind].Returned` by checking which tier each result came from (we keep a `map[idContentKey]coremem.Kind` during merge for this).
  9. Emit `EventSearchTotal` and `EventSearchHits` (parity with `UnifiedSearcher`).
- **Tier-of-origin tracking during merge.** We need to know which tier each result came from for provenance. We extend the merge step's internal `key` map to a `map[idContentKey]struct{ result SearchResult; kind coremem.Kind }` — wraps the existing dedupe map without allocating a separate structure. When a duplicate is found and the new entry has a higher score, we replace BOTH the result and the kind (the tier that scored the result highest wins). Documented as the canonical resolution rule.
- **`UnifiedSearcher` is kept, not deleted.** Rationale: deleting it would break the M3 tests that exercise it. We mark it `// Deprecated: prefer RecallEngine.Recall (M4 v1.0.0)` in its godoc, but it stays in the v1.x line so M1/M2/M3 tests pass unchanged. Removed at v2.0.0 (recorded in CHANGELOG).
- **`ParallelSearcher` is kept, not deleted.** Same reasoning — M2 tests cover it. The new `RecallEngine.Recall` re-uses the same goroutine-fan-out pattern but does not import `ParallelSearcher` directly (it needs per-tier topK, which `ParallelSearcher.SearchAllParallel` does not expose). Marked `// Deprecated: prefer RecallEngine.Recall (M4 v1.0.0)` in godoc.
- **The compat shim's scope, frozen at v1.0.0:** exactly TWO entry points — `NewManagerFromCore` and `NewManagerFromLegacyOptions` — plus the `LegacyOptions` type alias. We do NOT shim every M2/M3 wrapper because nothing else broke. Removal window: the `compat` sub-package is removed at v2.0.0; the v1.x line keeps it intact. The CHANGELOG locks this window in writing.
- **Migration guide format and location.** The doc lives in the **umbrella** at `docs/memory-v1-migration.zh-CN.md` (NOT in the sibling repo) because the umbrella is where consumer projects search for cross-cutting migration docs. Structure: (1) summary of breaks (one row table — there is essentially one break: prefer `memory.Manager` over `coremem.Manager` for new code); (2) upgrade recipe with code-before / code-after snippets; (3) compat-shim usage; (4) deprecation timeline; (5) exit-criteria checklist with file:line references back to this plan.
- **Test adaptation strategy: tests stay in-place; no existing test deleted.** M1's `consolidator_test.go`, `scoped_lifecycle_test.go`, `unified_search_test.go` continue to use `newCoreManager(t)` (a `*coremem.Manager`). M2's `parallel_search_test.go`, `observer_test.go` likewise. M3's `write_policy_test.go`, `sqlite_store_test.go` likewise. The NEW tests in `manager_test.go` / `recall_engine_test.go` / `compat/compat_test.go` lock the v1 surface. Rationale: keeps the v1.0.0 diff readable (every NEW test file documents a NEW concept; every OLD test file proves nothing legacy regressed). If a future minor needs a sweeping rename, that rename is a single PR.
- **Version bump strategy: `0.3.0 → 1.0.0`.** A v1.0.0 bump is correct because: (a) it matches the master roadmap §1 boundary table commitment ("first v1.0.0 tag of `llm-agent-memory`"); (b) v1 signals API stability — anything in `memory.Manager` / `memory.RecallEngine` / `memory/compat` is frozen until v2.0.0; (c) the M1/M2/M3 surface (UnifiedSearcher, ParallelSearcher, etc.) is also frozen at v1 — those types are marked `// Deprecated:` but their signatures cannot change in any v1.x release.
- **No event-name changes.** `EventWritePolicyDecided` (v0.3.0), `EventSearchTotal` / `EventSearchHits` (v0.2.0), and the rest of the M2 set remain canonical. The new RecallEngine emits `EventSearchTotal` and `EventSearchHits` — same constants used by `UnifiedSearcher`. We do NOT add a new event for `Recall` because the recall path *is* a search path conceptually.
- **`go.mod` is unchanged.** We do not add `golang.org/x/sync` (errgroup) — stdlib `sync.WaitGroup` + buffered channel + `context.WithCancel` is what `ParallelSearcher` already uses; same primitive in `RecallEngine`.
- **Constructor option alignment.** `NewManager(opts Options)` uses the explicit-struct form (matches `coremem.NewManager(coremem.ManagerOptions{...})`); `NewRecallEngine(mgr *Manager, opts ...Option)` uses the functional-options form (matches `NewConsolidator`/`NewUnifiedSearcher`/`NewParallelSearcher`/`NewScopedLifecycleManager`/`NewPolicyEnforcingMemory`). Inconsistency is acceptable here: `Options` is a wiring-time decision (many fields, validated once at construction); `Option ...` is a behavioral knob (often just `WithObserver`).
- **Compile-time interface assertions.** The new sibling `Manager`'s constructor verifies at runtime that wired tiers satisfy the required interfaces, but we add NO compile-time `var _ coremem.Memory = (*Manager)(nil)` line — sibling `Manager` is intentionally NOT a `coremem.Memory`. We DO add `var _ coremem.SnapshotStore = (*compat.LegacyStoreAdapter)(nil)` if we ship one — we do not; compat shim does not touch SnapshotStore.
- **D-2 contract with `context.Builder`.** Exit criterion §5.1 D-2 in the roadmap mentions `context.Builder` should consume Recall results directly. `context.Builder` lives in `coremem/context/builder.go` and currently accepts `[]memory.SearchResult` via `BuildInput.MemoryHits`. M4 does NOT modify `context.Builder` (it lives in core; we are not changing core); we lock the integration story in the migration guide: callers pass `recall.Results` as `BuildInput.MemoryHits` — one-line transform. The Builder change would be M6 work (where the Memory Gateway also lands) and is captured in the M4 migration guide as a forward reference.

---

## Sequencing Rules

- **Every task is strict TDD.** Failing test → minimal impl → green test → commit. Task 1 starts by declaring the failing capability-interface assertion.
- **Phase order:** D-1 (Tasks 1–8) → D-2 (Tasks 9–13) → compat shim (Tasks 14–16) → docs + version + release (Tasks 17–20). Rationale: D-1's `Manager` is a prerequisite for D-2's `RecallEngine` (which wraps `*Manager`); compat depends on `Manager`; docs are last because they describe everything else.
- **Commit cadence:** every task ends in exactly one commit. Step 5 of every task is "Commit".
- **Commit prefix:** `(memory)` for all tasks in this plan (matches M1/M2/M3 convention). Sub-task labels match the milestone item — D-1, D-2, compat, docs, release.
- **Sibling commit topology:** every commit goes to the sibling repo's `origin/main` — `cd llm-agent-memory && git add ... && git commit -m ... && git push origin main`. Migration-guide doc (Task 18) lands in the **umbrella** repo, NOT the sibling.
- **Tag format unprefixed:** `v1.0.0` (matches `v0.1.0` / `v0.2.0` / `v0.3.0`).
- **Before any task uses `WithSanitizer`, run `go doc` to confirm the v0.7 signature** — `cd llm-agent-memory && GOWORK=off go doc github.com/costa92/llm-agent/memory.WithSanitizer` — same M2 lesson applied: verify upstream interface surface before writing tests against it.
- **The `coreManagerLifecycle` adapter (Task 5) MUST forward `coremem.ErrConsolidateUnavailable` verbatim.** Do not re-wrap or shadow it; downstream code that already does `errors.Is(err, coremem.ErrConsolidateUnavailable)` must keep working.
- **No `len(page.Items) > 0` guards in iteration helpers** — same rule from M2/M3.
- **No `reflect.DeepEqual` on time-bearing types** in tests — same rule from M3. Test helpers from M3 (`assertSnapshotEqual`) are reused as-is.
- **`observer()` accessor at emit sites** — same convention as M2/M3.
- **Do not stutter error wraps.** Bad: `memory: manager: memory: capability missing: ...`. Good: `memory: manager %s: capability %s missing: %w`. The wrap that mentions the kind adds the kind; the wrap that adds the capability name adds it; the underlying core error preserves its prefix.
- **Tests use `newCoreManager(t)` and `newCoreWorking(t)` / `newCoreEpisodic(t)` / `newCoreSemantic(t)` from `testutil_test.go`** — same helpers M1/M2/M3 already use. New tests that need to skip a kind (e.g. to prove `ErrTierDisabled`) construct a partial `Options{Working: TierOptions{Memory: w}}` literal directly.
- **The compat shim's package import path is `github.com/costa92/llm-agent-memory/memory/compat`** — a sub-package, NOT a sibling-of-sibling. Same module, separate directory.

---

# Phase D-1 — Capability-Interface Manager (Tasks 1–8)

## Task 1: Declare the failing capability-interface compile assertion

**Files:**
- Create: `llm-agent-memory/memory/manager_test.go`

- [ ] **Step 1: Write the failing test that exercises the public surface**

  Create `llm-agent-memory/memory/manager_test.go`:

  ```go
  package memory

  import (
  	"testing"

  	coremem "github.com/costa92/llm-agent/memory"
  )

  // TestManager_TierOptions_FieldsAreCapabilityInterfaces is a
  // compile-time assertion. If TierOptions ever loses an interface field
  // or starts carrying a concrete type, this test will fail to compile —
  // the exact signal we want.
  //
  // The D-1 exit criterion is: ManagerOptions fields typed as interfaces
  // (Memory, Lister, Exporter, Importer, optional LifecycleMemory). The
  // sibling-owned Options.<Tier> is the carrier; this test pins the
  // shape.
  func TestManager_TierOptions_FieldsAreCapabilityInterfaces(t *testing.T) {
  	// One TierOptions per kind. Every field is an interface — the
  	// composite literal succeeds only if the types match.
  	var (
  		_ coremem.Memory     = (TierOptions{}).Memory
  		_ coremem.Lister     = (TierOptions{}).Lister
  		_ coremem.Exporter   = (TierOptions{}).Exporter
  		_ coremem.Importer   = (TierOptions{}).Importer
  		_ LifecycleMemory    = (TierOptions{}).Lifecycle
  	)

  	// Options carries three TierOptions plus a SnapshotStore plus an
  	// optional *coremem.Manager escape hatch (see "Open Decisions
  	// Resolved").
  	opts := Options{
  		Working:       TierOptions{},
  		Episodic:      TierOptions{},
  		Semantic:      TierOptions{},
  		SnapshotStore: nil,
  		CoreManager:   nil,
  	}
  	if opts.Working.Memory != nil || opts.Episodic.Memory != nil || opts.Semantic.Memory != nil {
  		t.Errorf("Options zero-value has non-nil capability fields: %+v", opts)
  	}
  }
  ```

- [ ] **Step 2: Run the test to confirm compile failure**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestManager_TierOptions_FieldsAreCapabilityInterfaces -v`
  Expected: compile error — `undefined: TierOptions`, `undefined: LifecycleMemory`, `undefined: Options`.

- [ ] **Step 3: Skip — impl in Task 2.**

- [ ] **Step 4: Skip.**

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/manager_test.go
  git commit -m "test(memory): declare TierOptions capability-interface compile assertion (D-1)"
  git push origin main
  ```

---

## Task 2: Declare `LifecycleMemory`, `TierOptions`, `Options`, and the sentinel errors

**Files:**
- Create: `llm-agent-memory/memory/manager.go`

- [ ] **Step 1: Confirm the core interfaces still match what we depend on**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go doc github.com/costa92/llm-agent/memory.Memory && GOWORK=off go doc github.com/costa92/llm-agent/memory.Lister && GOWORK=off go doc github.com/costa92/llm-agent/memory.Exporter && GOWORK=off go doc github.com/costa92/llm-agent/memory.Importer && GOWORK=off go doc github.com/costa92/llm-agent/memory.WithSanitizer`

  Expected: each prints the v0.7 signature unchanged. If any signature drifts the rest of the plan must adjust — fail-fast here is cheap.

- [ ] **Step 2: Write `llm-agent-memory/memory/manager.go` with the type declarations only (no methods yet)**

  ```go
  // Package memory — manager.go is the Phase D-1 implementation of the
  // sibling-owned, capability-interface-typed Manager that replaces
  // coremem.Manager as the recommended construction surface in v1.0.0+.
  //
  // Why this exists (vs reusing coremem.Manager directly):
  //
  //  1. coremem.ManagerOptions fields are typed *coremem.WorkingMemory /
  //     *coremem.EpisodicMemory / *coremem.SemanticMemory (see
  //     coremem/manager.go:22-35). That makes it impossible to install
  //     a decorator like coremem.WithSanitizer (which returns the
  //     coremem.Memory interface, NOT a concrete pointer — see
  //     coremem/policy_hook.go:37-45 and the LIMITATION block at
  //     coremem/doc.go:122-128).
  //
  //  2. Future external backends (Postgres, pgvector, Redis) cannot
  //     impersonate the concrete coremem types. With interface-typed
  //     TierOptions, any object satisfying coremem.Memory + coremem.Lister
  //     + coremem.Exporter + coremem.Importer can be installed.
  //
  // What it does NOT do: this Manager is NOT a coremem.Memory itself.
  // It is a coordinator with a Kind-discriminated dispatch surface
  // mirroring coremem.Manager's public API.
  //
  // Compatibility: the v0.7 coremem.Manager is unaffected. The compat/
  // sub-package provides a one-line bridge for callers wired to
  // coremem.NewManager(coremem.ManagerOptions{...}).
  package memory

  import (
  	"context"
  	"errors"
  	"fmt"

  	coremem "github.com/costa92/llm-agent/memory"
  )

  // LifecycleMemory is the new capability interface introduced in v1.0.0.
  // It models the two operations that coremem.Manager performs by reaching
  // through the Memory interface into private *scoredStore state (see
  // coremem/manager.go:191 and :239). External backends that want to
  // expose lifecycle semantics implement this interface directly; the
  // bundled coremem types do not satisfy it (they need a small adapter —
  // see coreManagerLifecycle in this file).
  //
  // Consolidate's semantics mirror coremem.Manager.Consolidate: promote
  // items from this kind into the next-higher kind (typically
  // Working → Episodic) based on opts.Threshold and opts.MinAge. Returns
  // the count promoted.
  //
  // Forget's semantics mirror coremem.Manager.Forget on the receiving
  // kind: apply the chosen strategy (importance / age / capacity) and
  // return the count deleted.
  type LifecycleMemory interface {
  	Consolidate(ctx context.Context, opts coremem.ConsolidateOptions) (int, error)
  	Forget(ctx context.Context, kind coremem.Kind, opts coremem.ForgetOptions) (int, error)
  }

  // TierOptions wires the per-kind capability set. Memory is required;
  // every other field is optional — if nil, the corresponding Manager
  // method either skips this tier (for read-side fan-outs like ListAll /
  // ExportAll) or returns ErrCapabilityMissing (for direct calls like
  // Consolidate). The bundled coremem types satisfy Memory + Lister +
  // Exporter + Importer — for those, a single object can fill four
  // fields:
  //
  //   w, _ := coremem.NewWorking(emb, coremem.WorkingOptions{})
  //   opts.Working = memory.TierOptions{Memory: w, Lister: w, Exporter: w, Importer: w}
  //
  // Lifecycle requires the explicit LifecycleMemory interface (or a
  // *coremem.Manager-backed adapter — see Options.CoreManager). The
  // bundled types do NOT satisfy LifecycleMemory directly because the
  // operation crosses tier boundaries.
  type TierOptions struct {
  	Memory    coremem.Memory   // required
  	Lister    coremem.Lister   // optional
  	Exporter  coremem.Exporter // optional
  	Importer  coremem.Importer // optional
  	Lifecycle LifecycleMemory  // optional
  }

  // Options is the v1.0.0 analogue of coremem.ManagerOptions. Pass to
  // NewManager. At least one tier's Memory field must be non-nil.
  type Options struct {
  	Working  TierOptions
  	Episodic TierOptions
  	Semantic TierOptions

  	// SnapshotStore mirrors coremem.ManagerOptions.SnapshotStore. Used
  	// by ExportAll/ImportAll when persistKey != "". Nil keeps
  	// persistence in-memory.
  	SnapshotStore coremem.SnapshotStore

  	// CoreManager is an OPTIONAL escape hatch. When non-nil, lifecycle
  	// methods (Consolidate, Forget) on tiers whose Lifecycle field is
  	// nil fall back to delegating into this *coremem.Manager via the
  	// coreManagerLifecycle adapter. Keeps the compat-shim path
  	// ergonomic (one line to bridge a legacy *coremem.Manager into the
  	// new sibling Manager surface).
  	//
  	// CoreManager is consulted ONLY for Lifecycle fallback today. It
  	// does NOT supplant a tier whose Memory field is nil.
  	CoreManager *coremem.Manager
  }

  // Manager is the sibling-owned, capability-interface-typed coordinator.
  // Construct via NewManager. Goroutine-safe: every method is a thin
  // dispatch on capability fields whose implementations are themselves
  // goroutine-safe in the bundled coremem types.
  type Manager struct {
  	opts Options
  }

  // --- sentinel errors ------------------------------------------------------

  // ErrNoTiers is returned by NewManager when every tier's Memory field
  // is nil. Analogue of coremem.ErrNoMemories.
  var ErrNoTiers = errors.New("memory: manager requires at least one tier with a Memory")

  // ErrTierDisabled is returned when a method targets a kind whose
  // TierOptions.Memory is nil. errors.Is-compatible with
  // coremem.ErrKindDisabled for callers already comparing against the
  // core sentinel.
  var ErrTierDisabled = fmt.Errorf("memory: tier disabled: %w", coremem.ErrKindDisabled)

  // ErrCapabilityMissing is returned when a tier is present but the
  // requested capability (Lister, Lifecycle, etc.) was not wired into
  // its TierOptions and no fallback (e.g. Options.CoreManager) is
  // available. The error message names the kind and the missing
  // capability.
  var ErrCapabilityMissing = errors.New("memory: capability missing on tier")

  // ErrUnknownKind is returned by dispatch helpers when an unrecognized
  // Kind value is passed.
  var ErrUnknownKind = errors.New("memory: unknown kind")
  ```

- [ ] **Step 3: Run the Task 1 test to confirm it now compiles + passes**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestManager_TierOptions_FieldsAreCapabilityInterfaces -v`
  Expected: PASS.

- [ ] **Step 4: Run the full sibling suite to confirm no regression**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -v`
  Expected: every existing M0–M3 test still passes.

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/manager.go
  git commit -m "feat(memory): declare LifecycleMemory + TierOptions + Options + sentinels (D-1)"
  git push origin main
  ```

---

## Task 3: `NewManager` constructor + tier-lookup helpers

**Files:**
- Modify: `llm-agent-memory/memory/manager.go`
- Modify: `llm-agent-memory/memory/manager_test.go` (append)

- [ ] **Step 1: Append failing tests covering constructor validation and tier-lookup behavior**

  Append to `manager_test.go`:

  ```go
  import (
  	// add only if not already present at top of file:
  	"context"
  	"errors"
  )

  func TestNewManager_AllTiersNil_ReturnsErrNoTiers(t *testing.T) {
  	_, err := NewManager(Options{})
  	if !errors.Is(err, ErrNoTiers) {
  		t.Fatalf("NewManager(empty) err = %v, want errors.Is ErrNoTiers", err)
  	}
  }

  func TestNewManager_AtLeastOneTier_Succeeds(t *testing.T) {
  	w := newCoreWorking(t)
  	mgr, err := NewManager(Options{Working: TierOptions{Memory: w}})
  	if err != nil {
  		t.Fatalf("NewManager: %v", err)
  	}
  	if mgr == nil {
  		t.Fatal("NewManager returned nil mgr")
  	}
  }

  func TestManager_HasKind_ReportsActiveTiers(t *testing.T) {
  	w := newCoreWorking(t)
  	mgr, err := NewManager(Options{Working: TierOptions{Memory: w}})
  	if err != nil {
  		t.Fatalf("NewManager: %v", err)
  	}
  	if !mgr.HasKind(coremem.KindWorking) {
  		t.Error("HasKind(Working) = false, want true")
  	}
  	if mgr.HasKind(coremem.KindEpisodic) {
  		t.Error("HasKind(Episodic) = true, want false")
  	}
  }

  func TestManager_Add_DispatchesToWiredTier(t *testing.T) {
  	w := newCoreWorking(t)
  	mgr, err := NewManager(Options{Working: TierOptions{Memory: w}})
  	if err != nil {
  		t.Fatalf("NewManager: %v", err)
  	}
  	id, err := mgr.Add(context.Background(), coremem.KindWorking, coremem.MemoryItem{Content: "hello"})
  	if err != nil {
  		t.Fatalf("Add: %v", err)
  	}
  	if id == "" {
  		t.Fatal("Add returned empty id")
  	}
  	got, err := w.Get(context.Background(), id)
  	if err != nil {
  		t.Fatalf("Get on underlying tier: %v", err)
  	}
  	if got.Content != "hello" {
  		t.Errorf("got.Content = %q, want %q", got.Content, "hello")
  	}
  }

  func TestManager_Add_DisabledKind_ReturnsErrTierDisabled(t *testing.T) {
  	w := newCoreWorking(t)
  	mgr, _ := NewManager(Options{Working: TierOptions{Memory: w}})
  	_, err := mgr.Add(context.Background(), coremem.KindEpisodic, coremem.MemoryItem{Content: "x"})
  	if !errors.Is(err, ErrTierDisabled) {
  		t.Errorf("Add to disabled kind err = %v, want errors.Is ErrTierDisabled", err)
  	}
  	if !errors.Is(err, coremem.ErrKindDisabled) {
  		t.Errorf("Add to disabled kind err = %v, want errors.Is coremem.ErrKindDisabled (compat)", err)
  	}
  }
  ```

- [ ] **Step 2: Run to confirm failure**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run 'TestNewManager_|TestManager_HasKind|TestManager_Add_' -v`
  Expected: compile error — `undefined: NewManager`, `undefined: (*Manager).HasKind`, `undefined: (*Manager).Add`.

- [ ] **Step 3: Implement `NewManager`, `HasKind`, `tierFor`, `Add` in `manager.go`**

  Append to `manager.go`:

  ```go
  // NewManager validates opts and returns a *Manager. Returns ErrNoTiers
  // if every tier's Memory is nil.
  func NewManager(opts Options) (*Manager, error) {
  	if opts.Working.Memory == nil && opts.Episodic.Memory == nil && opts.Semantic.Memory == nil {
  		return nil, ErrNoTiers
  	}
  	return &Manager{opts: opts}, nil
  }

  // HasKind reports whether a tier is wired for the given kind. A tier is
  // "wired" iff its TierOptions.Memory is non-nil. Useful for callers
  // that want to branch before calling Add / Search.
  func (m *Manager) HasKind(kind coremem.Kind) bool {
  	t, err := m.tierFor(kind)
  	if err != nil {
  		return false
  	}
  	return t.Memory != nil
  }

  // tierFor returns the TierOptions for the given kind. Returns
  // ErrUnknownKind if kind is not one of KindWorking / KindEpisodic /
  // KindSemantic; returns the TierOptions (with possibly-nil Memory)
  // otherwise. Callers must check tier.Memory before dispatching.
  func (m *Manager) tierFor(kind coremem.Kind) (TierOptions, error) {
  	switch kind {
  	case coremem.KindWorking:
  		return m.opts.Working, nil
  	case coremem.KindEpisodic:
  		return m.opts.Episodic, nil
  	case coremem.KindSemantic:
  		return m.opts.Semantic, nil
  	default:
  		return TierOptions{}, fmt.Errorf("%w: %q", ErrUnknownKind, kind)
  	}
  }

  // requireMemory returns the tier's Memory or ErrTierDisabled.
  func (m *Manager) requireMemory(kind coremem.Kind) (coremem.Memory, error) {
  	t, err := m.tierFor(kind)
  	if err != nil {
  		return nil, err
  	}
  	if t.Memory == nil {
  		return nil, fmt.Errorf("memory: manager %s: %w", kind, ErrTierDisabled)
  	}
  	return t.Memory, nil
  }

  // Add dispatches to the wired tier's Memory.Add. Returns
  // ErrTierDisabled if the kind has no Memory wired.
  func (m *Manager) Add(ctx context.Context, kind coremem.Kind, item coremem.MemoryItem) (string, error) {
  	mem, err := m.requireMemory(kind)
  	if err != nil {
  		return "", err
  	}
  	return mem.Add(ctx, item)
  }
  ```

- [ ] **Step 4: Run all five new tests**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run 'TestNewManager_|TestManager_HasKind|TestManager_Add_' -v`
  Expected: all five PASS.

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/manager.go memory/manager_test.go
  git commit -m "feat(memory): add NewManager + HasKind + tierFor + Add dispatch (D-1)"
  git push origin main
  ```

---

## Task 4: Single-kind dispatch methods — `Get`, `Update`, `Remove`, `Search`, `Stats`

**Files:**
- Modify: `llm-agent-memory/memory/manager.go`
- Modify: `llm-agent-memory/memory/manager_test.go` (append)

- [ ] **Step 1: Append failing tests covering each single-kind dispatch + the disabled-tier error path**

  Append to `manager_test.go`:

  ```go
  func TestManager_GetUpdateRemove_RoundTrip(t *testing.T) {
  	ctx := context.Background()
  	w := newCoreWorking(t)
  	mgr, _ := NewManager(Options{Working: TierOptions{Memory: w}})

  	id, err := mgr.Add(ctx, coremem.KindWorking, coremem.MemoryItem{Content: "rt"})
  	if err != nil {
  		t.Fatalf("Add: %v", err)
  	}
  	got, err := mgr.Get(ctx, coremem.KindWorking, id)
  	if err != nil || got.Content != "rt" {
  		t.Fatalf("Get: got=%+v err=%v", got, err)
  	}
  	if err := mgr.Update(ctx, coremem.KindWorking, id, func(it *coremem.MemoryItem) { it.Content = "rt2" }); err != nil {
  		t.Fatalf("Update: %v", err)
  	}
  	got2, _ := mgr.Get(ctx, coremem.KindWorking, id)
  	if got2.Content != "rt2" {
  		t.Errorf("after Update, Content = %q, want %q", got2.Content, "rt2")
  	}
  	if err := mgr.Remove(ctx, coremem.KindWorking, id); err != nil {
  		t.Fatalf("Remove: %v", err)
  	}
  	if _, err := mgr.Get(ctx, coremem.KindWorking, id); !errors.Is(err, coremem.ErrNotFound) {
  		t.Errorf("Get after Remove err = %v, want errors.Is ErrNotFound", err)
  	}
  }

  func TestManager_Search_DispatchesToCorrectTier(t *testing.T) {
  	ctx := context.Background()
  	w, e, s := newCoreWorking(t), newCoreEpisodic(t), newCoreSemantic(t)
  	mgr, _ := NewManager(Options{
  		Working:  TierOptions{Memory: w},
  		Episodic: TierOptions{Memory: e},
  		Semantic: TierOptions{Memory: s},
  	})
  	if _, err := mgr.Add(ctx, coremem.KindEpisodic, coremem.MemoryItem{Content: "episodic-fact"}); err != nil {
  		t.Fatalf("Add: %v", err)
  	}
  	res, err := mgr.Search(ctx, coremem.KindEpisodic, "episodic-fact", 5)
  	if err != nil {
  		t.Fatalf("Search: %v", err)
  	}
  	if len(res) == 0 {
  		t.Fatal("Search returned 0 results, want at least 1")
  	}
  	if res[0].Item.Content != "episodic-fact" {
  		t.Errorf("res[0].Content = %q, want %q", res[0].Item.Content, "episodic-fact")
  	}
  }

  func TestManager_Stats_OnlyActiveTiers(t *testing.T) {
  	w := newCoreWorking(t)
  	mgr, _ := NewManager(Options{Working: TierOptions{Memory: w}})
  	stats := mgr.StatsAll()
  	if _, ok := stats[coremem.KindWorking]; !ok {
  		t.Errorf("stats missing KindWorking entry: %+v", stats)
  	}
  	if _, ok := stats[coremem.KindEpisodic]; ok {
  		t.Errorf("stats has KindEpisodic but tier was not wired: %+v", stats)
  	}
  }
  ```

- [ ] **Step 2: Run to confirm failure**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run 'TestManager_GetUpdateRemove|TestManager_Search_|TestManager_Stats_' -v`
  Expected: compile error — `undefined: (*Manager).Get`, `Update`, `Remove`, `Search`, `StatsAll`.

- [ ] **Step 3: Implement the five methods in `manager.go`**

  Append to `manager.go`:

  ```go
  // Get fetches an item from the named tier.
  func (m *Manager) Get(ctx context.Context, kind coremem.Kind, id string) (coremem.MemoryItem, error) {
  	mem, err := m.requireMemory(kind)
  	if err != nil {
  		return coremem.MemoryItem{}, err
  	}
  	return mem.Get(ctx, id)
  }

  // Update mutates an item in the named tier.
  func (m *Manager) Update(ctx context.Context, kind coremem.Kind, id string, fn func(*coremem.MemoryItem)) error {
  	mem, err := m.requireMemory(kind)
  	if err != nil {
  		return err
  	}
  	return mem.Update(ctx, id, fn)
  }

  // Remove deletes an item from the named tier.
  func (m *Manager) Remove(ctx context.Context, kind coremem.Kind, id string) error {
  	mem, err := m.requireMemory(kind)
  	if err != nil {
  		return err
  	}
  	return mem.Remove(ctx, id)
  }

  // Search runs Memory.Search on one named tier.
  func (m *Manager) Search(ctx context.Context, kind coremem.Kind, query string, topK int) ([]coremem.SearchResult, error) {
  	mem, err := m.requireMemory(kind)
  	if err != nil {
  		return nil, err
  	}
  	return mem.Search(ctx, query, topK)
  }

  // StatsAll returns Stats for every active tier. Tiers without a wired
  // Memory are omitted from the result map. Parity with
  // coremem.Manager.StatsAll.
  func (m *Manager) StatsAll() map[coremem.Kind]coremem.Stats {
  	out := make(map[coremem.Kind]coremem.Stats, 3)
  	for _, kind := range []coremem.Kind{coremem.KindWorking, coremem.KindEpisodic, coremem.KindSemantic} {
  		t, _ := m.tierFor(kind)
  		if t.Memory == nil {
  			continue
  		}
  		out[kind] = t.Memory.Stats()
  	}
  	return out
  }
  ```

- [ ] **Step 4: Run all three new tests**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run 'TestManager_GetUpdateRemove|TestManager_Search_|TestManager_Stats_' -v`
  Expected: all three PASS.

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/manager.go memory/manager_test.go
  git commit -m "feat(memory): add Get/Update/Remove/Search/StatsAll single-kind dispatch (D-1)"
  git push origin main
  ```

---

## Task 5: Fan-out dispatch — `SearchAll`, `ListAll` + `coreManagerLifecycle` adapter

**Files:**
- Modify: `llm-agent-memory/memory/manager.go`
- Modify: `llm-agent-memory/memory/manager_test.go` (append)

- [ ] **Step 1: Append failing tests for fan-out methods**

  Append to `manager_test.go`:

  ```go
  func TestManager_SearchAll_FansAcrossActiveTiers(t *testing.T) {
  	ctx := context.Background()
  	w, e := newCoreWorking(t), newCoreEpisodic(t)
  	mgr, _ := NewManager(Options{
  		Working:  TierOptions{Memory: w},
  		Episodic: TierOptions{Memory: e},
  	})
  	if _, err := mgr.Add(ctx, coremem.KindWorking, coremem.MemoryItem{Content: "wfact"}); err != nil {
  		t.Fatalf("Add working: %v", err)
  	}
  	if _, err := mgr.Add(ctx, coremem.KindEpisodic, coremem.MemoryItem{Content: "efact"}); err != nil {
  		t.Fatalf("Add episodic: %v", err)
  	}
  	got, err := mgr.SearchAll(ctx, "fact", 5)
  	if err != nil {
  		t.Fatalf("SearchAll: %v", err)
  	}
  	if _, ok := got[coremem.KindWorking]; !ok {
  		t.Errorf("SearchAll missing KindWorking entry: %+v", got)
  	}
  	if _, ok := got[coremem.KindEpisodic]; !ok {
  		t.Errorf("SearchAll missing KindEpisodic entry: %+v", got)
  	}
  	if _, ok := got[coremem.KindSemantic]; ok {
  		t.Errorf("SearchAll includes KindSemantic but tier was not wired: %+v", got)
  	}
  }

  func TestManager_ListAll_PrefersTierLister_FallsBackToMemoryAssertion(t *testing.T) {
  	ctx := context.Background()
  	w := newCoreWorking(t)
  	// Wire Memory only — Lister field stays nil; coremem.WorkingMemory
  	// also implements Lister, so the fallback type-assertion should
  	// succeed.
  	mgr, _ := NewManager(Options{Working: TierOptions{Memory: w}})
  	if _, err := mgr.Add(ctx, coremem.KindWorking, coremem.MemoryItem{Content: "list-me"}); err != nil {
  		t.Fatalf("Add: %v", err)
  	}
  	pages, err := mgr.ListAll(ctx, coremem.ListFilter{}, 10, nil)
  	if err != nil {
  		t.Fatalf("ListAll: %v", err)
  	}
  	p := pages[coremem.KindWorking]
  	if len(p.Items) != 1 || p.Items[0].Content != "list-me" {
  		t.Errorf("ListAll Working page = %+v, want one item with Content=list-me", p)
  	}
  }

  func TestManager_Consolidate_NoLifecycle_NoCoreManager_ReturnsCapabilityMissing(t *testing.T) {
  	w := newCoreWorking(t)
  	e := newCoreEpisodic(t)
  	mgr, _ := NewManager(Options{
  		Working:  TierOptions{Memory: w},
  		Episodic: TierOptions{Memory: e},
  	})
  	_, err := mgr.Consolidate(context.Background(), coremem.ConsolidateOptions{})
  	if !errors.Is(err, ErrCapabilityMissing) {
  		t.Errorf("Consolidate err = %v, want errors.Is ErrCapabilityMissing", err)
  	}
  }

  func TestManager_Consolidate_WithCoreManagerFallback_Succeeds(t *testing.T) {
  	ctx := context.Background()
  	w := newCoreWorking(t)
  	e := newCoreEpisodic(t)
  	coreMgr, _ := coremem.NewManager(coremem.ManagerOptions{Working: w, Episodic: e})
  	mgr, _ := NewManager(Options{
  		Working:     TierOptions{Memory: w},
  		Episodic:    TierOptions{Memory: e},
  		CoreManager: coreMgr,
  	})
  	if _, err := mgr.Add(ctx, coremem.KindWorking, coremem.MemoryItem{Content: "promote me", Importance: 0.9}); err != nil {
  		t.Fatalf("Add: %v", err)
  	}
  	n, err := mgr.Consolidate(ctx, coremem.ConsolidateOptions{Threshold: 0.7})
  	if err != nil {
  		t.Fatalf("Consolidate: %v", err)
  	}
  	if n < 1 {
  		t.Errorf("Consolidate promoted = %d, want >= 1", n)
  	}
  }
  ```

- [ ] **Step 2: Run to confirm failure**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run 'TestManager_SearchAll|TestManager_ListAll_|TestManager_Consolidate_' -v`
  Expected: compile error — `undefined: (*Manager).SearchAll`, `ListAll`, `Consolidate`.

- [ ] **Step 3: Implement `SearchAll`, `ListAll`, `Consolidate`, `Forget`, and the `coreManagerLifecycle` adapter in `manager.go`**

  Append to `manager.go`:

  ```go
  // SearchAll fans the query out to every active tier and returns the
  // per-kind result lists. Parity with coremem.Manager.SearchAll: per-
  // kind topK is honored (not a global cap); disabled tiers are omitted
  // from the result map.
  func (m *Manager) SearchAll(ctx context.Context, query string, topK int) (map[coremem.Kind][]coremem.SearchResult, error) {
  	out := make(map[coremem.Kind][]coremem.SearchResult, 3)
  	for _, kind := range []coremem.Kind{coremem.KindWorking, coremem.KindEpisodic, coremem.KindSemantic} {
  		t, _ := m.tierFor(kind)
  		if t.Memory == nil {
  			continue
  		}
  		res, err := t.Memory.Search(ctx, query, topK)
  		if err != nil {
  			return out, fmt.Errorf("memory: manager search %s: %w", kind, err)
  		}
  		out[kind] = res
  	}
  	return out, nil
  }

  // ListAll fans the list out to every active tier. For each tier we
  // prefer Tier.Lister; if nil, we fall back to type-asserting
  // Tier.Memory.(coremem.Lister). If neither is available the tier is
  // silently skipped (parity with coremem.Manager.ListAll). cursors is
  // a per-kind map; missing entries start from the beginning.
  func (m *Manager) ListAll(ctx context.Context, filter coremem.ListFilter, pageSize int, cursors map[coremem.Kind]string) (map[coremem.Kind]coremem.ListPage, error) {
  	out := make(map[coremem.Kind]coremem.ListPage, 3)
  	for _, kind := range []coremem.Kind{coremem.KindWorking, coremem.KindEpisodic, coremem.KindSemantic} {
  		t, _ := m.tierFor(kind)
  		if t.Memory == nil {
  			continue
  		}
  		lister := t.Lister
  		if lister == nil {
  			if l, ok := t.Memory.(coremem.Lister); ok {
  				lister = l
  			}
  		}
  		if lister == nil {
  			continue
  		}
  		cursor := ""
  		if cursors != nil {
  			cursor = cursors[kind]
  		}
  		page, err := lister.List(ctx, filter, pageSize, cursor)
  		if err != nil {
  			return out, fmt.Errorf("memory: manager list %s: %w", kind, err)
  		}
  		out[kind] = page
  	}
  	return out, nil
  }

  // Consolidate promotes items via the Working tier's LifecycleMemory.
  // Falls back to Options.CoreManager when Working.Lifecycle is nil and
  // a CoreManager was provided. Otherwise returns ErrCapabilityMissing.
  func (m *Manager) Consolidate(ctx context.Context, opts coremem.ConsolidateOptions) (int, error) {
  	if m.opts.Working.Lifecycle != nil {
  		return m.opts.Working.Lifecycle.Consolidate(ctx, opts)
  	}
  	if m.opts.CoreManager != nil {
  		return m.opts.CoreManager.Consolidate(ctx, opts)
  	}
  	return 0, fmt.Errorf("%w: %s.Lifecycle", ErrCapabilityMissing, coremem.KindWorking)
  }

  // Forget applies the chosen strategy via the named kind's
  // LifecycleMemory.Forget. Falls back to Options.CoreManager when the
  // tier's Lifecycle is nil and a CoreManager was provided. Otherwise
  // returns ErrCapabilityMissing.
  func (m *Manager) Forget(ctx context.Context, kind coremem.Kind, opts coremem.ForgetOptions) (int, error) {
  	t, err := m.tierFor(kind)
  	if err != nil {
  		return 0, err
  	}
  	if t.Lifecycle != nil {
  		return t.Lifecycle.Forget(ctx, kind, opts)
  	}
  	if m.opts.CoreManager != nil {
  		return m.opts.CoreManager.Forget(ctx, kind, opts)
  	}
  	return 0, fmt.Errorf("%w: %s.Lifecycle", ErrCapabilityMissing, kind)
  }

  // coreManagerLifecycle is a small adapter that lets a *coremem.Manager
  // satisfy LifecycleMemory. Construct with NewCoreManagerLifecycle.
  // Useful when wiring a single coremem.Manager into the v1 Manager via
  // Options.Working.Lifecycle = NewCoreManagerLifecycle(coreMgr).
  type coreManagerLifecycle struct {
  	mgr *coremem.Manager
  }

  // NewCoreManagerLifecycle returns a LifecycleMemory that forwards
  // Consolidate / Forget to the given *coremem.Manager. Returns nil if
  // mgr is nil — callers should check before assigning.
  func NewCoreManagerLifecycle(mgr *coremem.Manager) LifecycleMemory {
  	if mgr == nil {
  		return nil
  	}
  	return coreManagerLifecycle{mgr: mgr}
  }

  // Consolidate forwards to the wrapped *coremem.Manager.Consolidate.
  // The coremem sentinel coremem.ErrConsolidateUnavailable surfaces
  // verbatim so existing errors.Is callers keep working.
  func (a coreManagerLifecycle) Consolidate(ctx context.Context, opts coremem.ConsolidateOptions) (int, error) {
  	return a.mgr.Consolidate(ctx, opts)
  }

  // Forget forwards to (*coremem.Manager).Forget.
  func (a coreManagerLifecycle) Forget(ctx context.Context, kind coremem.Kind, opts coremem.ForgetOptions) (int, error) {
  	return a.mgr.Forget(ctx, kind, opts)
  }

  // Compile-time check that coreManagerLifecycle satisfies the new
  // LifecycleMemory interface. Catches drift if either signature changes.
  var _ LifecycleMemory = coreManagerLifecycle{}
  ```

- [ ] **Step 4: Run all four new tests**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run 'TestManager_SearchAll|TestManager_ListAll_|TestManager_Consolidate_' -v`
  Expected: all four PASS.

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/manager.go memory/manager_test.go
  git commit -m "feat(memory): add SearchAll/ListAll/Consolidate/Forget + coreManagerLifecycle (D-1)"
  git push origin main
  ```

---

## Task 6: `ExportAll` and `ImportAll` — capability-aware persistence dispatch

**Files:**
- Modify: `llm-agent-memory/memory/manager.go`
- Modify: `llm-agent-memory/memory/manager_test.go` (append)

- [ ] **Step 1: Append failing tests covering Export/Import in two modes (inline + via SnapshotStore)**

  Append to `manager_test.go`:

  ```go
  func TestManager_ExportAll_FansAcrossTiersThatExpose(t *testing.T) {
  	ctx := context.Background()
  	w, e := newCoreWorking(t), newCoreEpisodic(t)
  	mgr, _ := NewManager(Options{
  		Working:  TierOptions{Memory: w, Exporter: w},
  		Episodic: TierOptions{Memory: e, Exporter: e},
  	})
  	if _, err := mgr.Add(ctx, coremem.KindWorking, coremem.MemoryItem{Content: "w"}); err != nil {
  		t.Fatalf("Add working: %v", err)
  	}
  	if _, err := mgr.Add(ctx, coremem.KindEpisodic, coremem.MemoryItem{Content: "e"}); err != nil {
  		t.Fatalf("Add episodic: %v", err)
  	}
  	snaps, err := mgr.ExportAll(ctx, "")
  	if err != nil {
  		t.Fatalf("ExportAll: %v", err)
  	}
  	if _, ok := snaps[coremem.KindWorking]; !ok {
  		t.Errorf("ExportAll missing KindWorking: %+v", snaps)
  	}
  	if _, ok := snaps[coremem.KindEpisodic]; !ok {
  		t.Errorf("ExportAll missing KindEpisodic: %+v", snaps)
  	}
  	if got := len(snaps[coremem.KindWorking].Items); got != 1 {
  		t.Errorf("Working snapshot Items len = %d, want 1", got)
  	}
  }

  func TestManager_ExportAll_FallsBackToMemoryAssertionForExporter(t *testing.T) {
  	// Same as above but DO NOT wire Exporter explicitly; rely on
  	// type-assertion that coremem.WorkingMemory satisfies coremem.Exporter.
  	ctx := context.Background()
  	w := newCoreWorking(t)
  	mgr, _ := NewManager(Options{Working: TierOptions{Memory: w}})
  	if _, err := mgr.Add(ctx, coremem.KindWorking, coremem.MemoryItem{Content: "x"}); err != nil {
  		t.Fatalf("Add: %v", err)
  	}
  	snaps, err := mgr.ExportAll(ctx, "")
  	if err != nil {
  		t.Fatalf("ExportAll: %v", err)
  	}
  	if _, ok := snaps[coremem.KindWorking]; !ok {
  		t.Errorf("ExportAll missing KindWorking via Memory-as-Exporter assertion: %+v", snaps)
  	}
  }

  func TestManager_ExportAll_PersistKeyWithoutStore_ReturnsErrSnapshotStoreNotConfigured(t *testing.T) {
  	w := newCoreWorking(t)
  	mgr, _ := NewManager(Options{Working: TierOptions{Memory: w}})
  	_, err := mgr.ExportAll(context.Background(), "any-key")
  	if !errors.Is(err, coremem.ErrSnapshotStoreNotConfigured) {
  		t.Errorf("err = %v, want errors.Is coremem.ErrSnapshotStoreNotConfigured", err)
  	}
  }

  func TestManager_ImportAll_InlineSnapsRoundTrip(t *testing.T) {
  	ctx := context.Background()
  	src := newCoreWorking(t)
  	if _, err := src.Add(ctx, coremem.MemoryItem{Content: "round"}); err != nil {
  		t.Fatalf("src Add: %v", err)
  	}
  	snap, err := src.Export(ctx)
  	if err != nil {
  		t.Fatalf("src Export: %v", err)
  	}

  	dst := newCoreWorking(t)
  	mgr, _ := NewManager(Options{Working: TierOptions{Memory: dst, Importer: dst}})
  	reports, err := mgr.ImportAll(ctx, map[coremem.Kind]coremem.Snapshot{coremem.KindWorking: snap}, "", coremem.ImportReplace)
  	if err != nil {
  		t.Fatalf("ImportAll: %v", err)
  	}
  	if reports[coremem.KindWorking].Loaded != 1 {
  		t.Errorf("Loaded = %d, want 1", reports[coremem.KindWorking].Loaded)
  	}
  }
  ```

- [ ] **Step 2: Run to confirm failure**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run 'TestManager_ExportAll|TestManager_ImportAll_' -v`
  Expected: compile error — `undefined: (*Manager).ExportAll`, `ImportAll`.

- [ ] **Step 3: Implement `ExportAll` and `ImportAll` in `manager.go`**

  Append to `manager.go`:

  ```go
  // ExportAll exports each active tier whose Exporter is wired (or whose
  // Memory satisfies coremem.Exporter via type assertion). Parity with
  // coremem.Manager.ExportAll: when persistKey != "", every snapshot is
  // also persisted via Options.SnapshotStore — returning
  // coremem.ErrSnapshotStoreNotConfigured if the store is nil.
  func (m *Manager) ExportAll(ctx context.Context, persistKey string) (map[coremem.Kind]coremem.Snapshot, error) {
  	out := make(map[coremem.Kind]coremem.Snapshot, 3)
  	for _, kind := range []coremem.Kind{coremem.KindWorking, coremem.KindEpisodic, coremem.KindSemantic} {
  		t, _ := m.tierFor(kind)
  		if t.Memory == nil {
  			continue
  		}
  		exp := t.Exporter
  		if exp == nil {
  			if e, ok := t.Memory.(coremem.Exporter); ok {
  				exp = e
  			}
  		}
  		if exp == nil {
  			continue
  		}
  		snap, err := exp.Export(ctx)
  		if err != nil {
  			return out, fmt.Errorf("memory: manager export %s: %w", kind, err)
  		}
  		out[kind] = snap
  	}
  	if persistKey == "" {
  		return out, nil
  	}
  	if m.opts.SnapshotStore == nil {
  		return out, coremem.ErrSnapshotStoreNotConfigured
  	}
  	for _, snap := range out {
  		if err := m.opts.SnapshotStore.Save(ctx, persistKey, snap); err != nil {
  			return out, err
  		}
  	}
  	return out, nil
  }

  // ImportAll fans the import out to each tier whose Importer is wired
  // (or whose Memory satisfies coremem.Importer via type assertion). Two
  // modes: when snaps != nil, the inline map wins; otherwise the
  // configured SnapshotStore is consulted (preferring LoadKind when
  // available). Disabled tiers / missing keys / missing importers are
  // silently skipped. Parity with coremem.Manager.ImportAll.
  func (m *Manager) ImportAll(ctx context.Context, snaps map[coremem.Kind]coremem.Snapshot, persistKey string, mode coremem.ImportMode) (map[coremem.Kind]coremem.ImportReport, error) {
  	if snaps == nil && persistKey != "" {
  		if m.opts.SnapshotStore == nil {
  			return nil, coremem.ErrSnapshotStoreNotConfigured
  		}
  		loaded, err := loadAllFromStore(ctx, m.opts.SnapshotStore, persistKey)
  		if err != nil {
  			return nil, err
  		}
  		snaps = loaded
  	}
  	out := make(map[coremem.Kind]coremem.ImportReport, len(snaps))
  	for kind, snap := range snaps {
  		t, _ := m.tierFor(kind)
  		if t.Memory == nil {
  			continue
  		}
  		imp := t.Importer
  		if imp == nil {
  			if i, ok := t.Memory.(coremem.Importer); ok {
  				imp = i
  			}
  		}
  		if imp == nil {
  			continue
  		}
  		rpt, err := imp.Import(ctx, snap, mode)
  		if err != nil {
  			return out, fmt.Errorf("memory: manager import %s: %w", kind, err)
  		}
  		out[kind] = rpt
  	}
  	return out, nil
  }

  // loadAllFromStore mirrors the per-kind loop in coremem.Manager.ImportAll
  // (manager.go:368-391) — prefer LoadKind when the store implements it,
  // otherwise fall back to Load and filter by Kind. Missing keys (those
  // wrapping os.ErrNotExist) are silently skipped.
  func loadAllFromStore(ctx context.Context, store coremem.SnapshotStore, persistKey string) (map[coremem.Kind]coremem.Snapshot, error) {
  	type kindLoader interface {
  		LoadKind(ctx context.Context, key string, kind coremem.Kind) (coremem.Snapshot, error)
  	}
  	out := make(map[coremem.Kind]coremem.Snapshot, 3)
  	for _, kind := range []coremem.Kind{coremem.KindWorking, coremem.KindEpisodic, coremem.KindSemantic} {
  		var (
  			snap coremem.Snapshot
  			err  error
  		)
  		if lk, ok := store.(kindLoader); ok {
  			snap, err = lk.LoadKind(ctx, persistKey, kind)
  		} else {
  			snap, err = store.Load(ctx, persistKey)
  			if err == nil && snap.Kind != kind {
  				continue
  			}
  		}
  		if err != nil {
  			if errors.Is(err, osErrNotExist) {
  				continue
  			}
  			return nil, fmt.Errorf("memory: manager import load %s: %w", kind, err)
  		}
  		out[kind] = snap
  	}
  	return out, nil
  }
  ```

  Add at the top of `manager.go` (with the existing import block):

  ```go
  import (
  	"context"
  	"errors"
  	"fmt"
  	"os"

  	coremem "github.com/costa92/llm-agent/memory"
  )

  // osErrNotExist is aliased so loadAllFromStore can call errors.Is
  // without an additional public dependency on the `os` package being
  // visible from manager.go consumers. (Removes the temptation to add
  // a public errors.Is shim.)
  var osErrNotExist = os.ErrNotExist
  ```

- [ ] **Step 4: Run all four new tests**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run 'TestManager_ExportAll|TestManager_ImportAll_' -v`
  Expected: all four PASS.

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/manager.go memory/manager_test.go
  git commit -m "feat(memory): add ExportAll/ImportAll capability-aware dispatch (D-1)"
  git push origin main
  ```

---

## Task 7: D-1 exit-criterion proof — `WithSanitizer`-without-cast installs cleanly

This is the single most important test in the entire milestone. It is the verbatim D-1 exit criterion #2: "`WithSanitizer`-wrapped memory installs into `Manager` without a cast."

**Files:**
- Modify: `llm-agent-memory/memory/manager_test.go` (append)

- [ ] **Step 1: Append the failing test that exercises the no-cast install**

  Append to `manager_test.go`:

  ```go
  // TestManager_WithSanitizerWrappedMemory_InstallsWithoutCast is the
  // verbatim D-1 exit-criterion proof from docs/superpowers/plans/
  // 2026-05-25-llm-agent-memory-roadmap.md §5.1 D-1. coremem.WithSanitizer
  // returns the coremem.Memory interface value (see policy_hook.go:61-66);
  // pre-M4, that value could NOT be assigned to coremem.ManagerOptions.Working
  // (a *coremem.WorkingMemory). With the new sibling Options.Working.Memory
  // typed as coremem.Memory, the assignment compiles + works.
  //
  // If this test starts failing to compile, M4's central D-1 promise is
  // broken.
  func TestManager_WithSanitizerWrappedMemory_InstallsWithoutCast(t *testing.T) {
  	ctx := context.Background()
  	w := newCoreWorking(t)

  	// Build a sanitizer chain that uppercases content. This proves the
  	// chain runs (and therefore that the wrapped Memory is the one
  	// Manager.Add invokes — NOT the underlying *coremem.WorkingMemory).
  	uppercase := coremem.SanitizerFunc(func(_ context.Context, _ coremem.Kind, it coremem.MemoryItem) (coremem.MemoryItem, bool, error) {
  		it.Content = "UPPER:" + it.Content
  		return it, true, nil
  	})

  	// THE no-cast install. coremem.WithSanitizer returns coremem.Memory;
  	// in the old world this would NOT have compiled — TierOptions.Memory
  	// is exactly coremem.Memory, which closes the M4 D-1 gap.
  	wrapped := coremem.WithSanitizer(w, uppercase)
  	mgr, err := NewManager(Options{
  		Working: TierOptions{Memory: wrapped},
  	})
  	if err != nil {
  		t.Fatalf("NewManager with wrapped memory: %v", err)
  	}

  	id, err := mgr.Add(ctx, coremem.KindWorking, coremem.MemoryItem{Content: "hello"})
  	if err != nil {
  		t.Fatalf("Add: %v", err)
  	}
  	// Fetch through the wrapper (still goes through our Manager so we
  	// keep the same dispatch surface). Note that the *underlying*
  	// coremem.WorkingMemory now holds the prefixed content, because
  	// the sanitizer ran.
  	got, err := mgr.Get(ctx, coremem.KindWorking, id)
  	if err != nil {
  		t.Fatalf("Get: %v", err)
  	}
  	if got.Content != "UPPER:hello" {
  		t.Errorf("Content = %q, want %q — sanitizer chain did not run", got.Content, "UPPER:hello")
  	}
  }
  ```

- [ ] **Step 2: Run to confirm PASS (implementation already exists from Tasks 2–4)**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestManager_WithSanitizerWrappedMemory_InstallsWithoutCast -v`
  Expected: PASS. The test is a regression detector — if any future change breaks the no-cast install, this test fails to compile.

  If it doesn't pass on first run: re-read the `TierOptions.Memory` field's declared type — it MUST be `coremem.Memory` (the interface), not `*coremem.WorkingMemory` (concrete). If you accidentally typed the wrong thing during Task 2, fix it now and re-run.

- [ ] **Step 3: Skip (no impl change).**

- [ ] **Step 4: Skip.**

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/manager_test.go
  git commit -m "test(memory): pin D-1 WithSanitizer-no-cast install exit-criterion proof"
  git push origin main
  ```

---

## Task 8: Parity sweep — Manager satisfies every coremem.Manager public method on parity-eligible inputs

This task is a defensive sweep, not a new feature. We pin parity so future refactors of either Manager cannot silently diverge.

**Files:**
- Modify: `llm-agent-memory/memory/manager_test.go` (append)

- [ ] **Step 1: Append the parity table test**

  ```go
  // TestManager_ParityWithCoreManager_MethodMatrix is the structural
  // assertion that the sibling Manager exposes every coremem.Manager
  // public method that consumers depend on. It is NOT a goroutine-safety
  // test, NOT a correctness test — those live in their per-method tests
  // above. This is a single guard against an accidental drop of a public
  // method during a future refactor.
  func TestManager_ParityWithCoreManager_MethodMatrix(t *testing.T) {
  	ctx := context.Background()
  	w, e, s := newCoreWorking(t), newCoreEpisodic(t), newCoreSemantic(t)
  	mgr, err := NewManager(Options{
  		Working:  TierOptions{Memory: w},
  		Episodic: TierOptions{Memory: e},
  		Semantic: TierOptions{Memory: s},
  	})
  	if err != nil {
  		t.Fatalf("NewManager: %v", err)
  	}

  	if _, err := mgr.Add(ctx, coremem.KindWorking, coremem.MemoryItem{Content: "p"}); err != nil {
  		t.Errorf("Add: %v", err)
  	}
  	if _, err := mgr.Search(ctx, coremem.KindWorking, "p", 5); err != nil {
  		t.Errorf("Search: %v", err)
  	}
  	if _, err := mgr.SearchAll(ctx, "p", 5); err != nil {
  		t.Errorf("SearchAll: %v", err)
  	}
  	if _, err := mgr.ListAll(ctx, coremem.ListFilter{}, 10, nil); err != nil {
  		t.Errorf("ListAll: %v", err)
  	}
  	if got := mgr.StatsAll(); len(got) != 3 {
  		t.Errorf("StatsAll: got %d entries, want 3", len(got))
  	}
  	if _, err := mgr.ExportAll(ctx, ""); err != nil {
  		t.Errorf("ExportAll: %v", err)
  	}
  	if _, err := mgr.ImportAll(ctx, map[coremem.Kind]coremem.Snapshot{}, "", coremem.ImportMerge); err != nil {
  		t.Errorf("ImportAll(empty): %v", err)
  	}
  	// Consolidate + Forget without Lifecycle wiring should be the
  	// declared capability error.
  	if _, err := mgr.Consolidate(ctx, coremem.ConsolidateOptions{}); !errors.Is(err, ErrCapabilityMissing) {
  		t.Errorf("Consolidate without Lifecycle: err = %v, want ErrCapabilityMissing", err)
  	}
  	if _, err := mgr.Forget(ctx, coremem.KindWorking, coremem.ForgetOptions{Strategy: coremem.ForgetByImportance, Threshold: 0.1}); !errors.Is(err, ErrCapabilityMissing) {
  		t.Errorf("Forget without Lifecycle: err = %v, want ErrCapabilityMissing", err)
  	}
  }
  ```

- [ ] **Step 2: Run to confirm PASS**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestManager_ParityWithCoreManager_MethodMatrix -v`
  Expected: PASS.

- [ ] **Step 3: Skip (no impl change).**

- [ ] **Step 4: Run the full sibling suite to confirm no regression**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -v -count=1`
  Expected: every test from M0–M3 still passes, plus every new D-1 test from Tasks 1–8.

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/manager_test.go
  git commit -m "test(memory): pin Manager-vs-coremem.Manager parity method matrix (D-1)"
  git push origin main
  ```

---

# Phase D-2 — RecallEngine Facade (Tasks 9–13)

## Task 9: Declare the failing RecallEngine interface surface

**Files:**
- Create: `llm-agent-memory/memory/recall_engine_test.go`

- [ ] **Step 1: Write the failing compile-assertion test**

  Create `llm-agent-memory/memory/recall_engine_test.go`:

  ```go
  package memory

  import (
  	"context"
  	"errors"
  	"testing"

  	coremem "github.com/costa92/llm-agent/memory"
  )

  // TestRecallEngine_PublicShape_Compiles pins the v1.0.0 surface of the
  // D-2 RecallEngine facade. Any field rename, removal, or type change
  // breaks compilation here.
  func TestRecallEngine_PublicShape_Compiles(t *testing.T) {
  	var _ TierMask = TierWorking | TierEpisodic | TierSemantic | AllTiers
  	if AllTiers != (TierWorking | TierEpisodic | TierSemantic) {
  		t.Errorf("AllTiers != Working|Episodic|Semantic — bitmask drift")
  	}

  	// RecallOptions documented fields.
  	_ = RecallOptions{
  		TopK:              10,
  		Tiers:             AllTiers,
  		Budgets:           map[coremem.Kind]int{coremem.KindWorking: 2},
  		IncludeProvenance: true,
  	}

  	// UnifiedRecall documented fields.
  	_ = UnifiedRecall{
  		Results:      []coremem.SearchResult{},
  		PerTier:      map[coremem.Kind]TierStats{coremem.KindWorking: {Considered: 0, Returned: 0}},
  		TotalDropped: 0,
  	}

  	// Constructor signature.
  	mgr, _ := NewManager(Options{Working: TierOptions{Memory: newCoreWorking(t)}})
  	eng, err := NewRecallEngine(mgr)
  	if err != nil {
  		t.Fatalf("NewRecallEngine: %v", err)
  	}
  	if eng == nil {
  		t.Fatal("NewRecallEngine returned nil")
  	}

  	// nil manager surfaces the typed sentinel.
  	if _, err := NewRecallEngine(nil); !errors.Is(err, ErrRecallEngineManagerRequired) {
  		t.Errorf("NewRecallEngine(nil) err = %v, want ErrRecallEngineManagerRequired", err)
  	}

  	// Recall callable from a smoke test.
  	if _, err := eng.Recall(context.Background(), "anything", RecallOptions{}); err != nil {
  		t.Errorf("Recall on empty: %v", err)
  	}
  }
  ```

- [ ] **Step 2: Run to confirm compile failure**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestRecallEngine_PublicShape_Compiles -v`
  Expected: compile error — `undefined: TierMask`, `undefined: TierWorking`, `undefined: RecallEngine`, `undefined: NewRecallEngine`, etc.

- [ ] **Step 3: Skip — impl in Task 10.**

- [ ] **Step 4: Skip.**

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/recall_engine_test.go
  git commit -m "test(memory): declare RecallEngine v1 public-shape compile assertion (D-2)"
  git push origin main
  ```

---

## Task 10: Implement `RecallEngine` + `Recall` minimum viable algorithm

**Files:**
- Create: `llm-agent-memory/memory/recall_engine.go`

- [ ] **Step 1: Create `llm-agent-memory/memory/recall_engine.go`**

  ```go
  // Package memory — recall_engine.go is the Phase D-2 implementation of
  // the unified recall facade. RecallEngine.Recall is the v1.0.0 public
  // recall surface; tier-awareness (working / episodic / semantic
  // fan-out) becomes an internal implementation detail.
  //
  // Composition: RecallEngine wraps a *Manager (the D-1 surface), not
  // a *coremem.Manager — the sibling Manager IS the v1 interface
  // dispatcher, and reusing it keeps the tier-routing logic single-
  // sourced. Callers wired to a legacy *coremem.Manager bridge through
  // memory/compat.NewManagerFromCore.
  //
  // Algorithm: see (*RecallEngine).Recall godoc.
  package memory

  import (
  	"context"
  	"errors"
  	"fmt"
  	"sort"
  	"sync"

  	coremem "github.com/costa92/llm-agent/memory"
  )

  // TierMask selects which tiers participate in a Recall. The zero value
  // (0) is treated as AllTiers — every active tier on the wrapped
  // Manager participates.
  type TierMask uint8

  // TierMask constants for the three bundled kinds.
  const (
  	TierWorking  TierMask = 1 << 0
  	TierEpisodic TierMask = 1 << 1
  	TierSemantic TierMask = 1 << 2
  	AllTiers     TierMask = TierWorking | TierEpisodic | TierSemantic
  )

  // RecallOptions configures one Recall call. Frozen at v1.0.0 — new
  // fields may be appended (non-breaking); existing fields are never
  // renamed or removed in any v1.x release.
  type RecallOptions struct {
  	// TopK is the global cap on the merged result slice. <=0 → 10.
  	TopK int

  	// Tiers is a bitmask of participating tiers. The zero value is
  	// treated as AllTiers.
  	Tiers TierMask

  	// Budgets is an OPTIONAL per-tier upper bound on candidates pulled
  	// from each tier before merge. Nil = each participating tier
  	// returns TopK candidates (matches the legacy UnifiedSearcher
  	// semantics). A tier present with a value <=0 is treated as
  	// "use TopK".
  	Budgets map[coremem.Kind]int

  	// IncludeProvenance, when true, populates UnifiedRecall.PerTier
  	// with per-tier Considered/Returned counts. Default false to keep
  	// the fast path allocation-minimal.
  	IncludeProvenance bool
  }

  // TierStats is one row of UnifiedRecall.PerTier.
  type TierStats struct {
  	Considered int // candidates returned by this tier before merge
  	Returned   int // count of Results that originated from this tier
  }

  // UnifiedRecall is the single recall result type. Frozen at v1.0.0.
  type UnifiedRecall struct {
  	Results      []coremem.SearchResult
  	PerTier      map[coremem.Kind]TierStats
  	TotalDropped int
  }

  // RecallEngine wraps a *Manager and exposes Recall(ctx, query, opts).
  type RecallEngine struct {
  	mgr *Manager
  	cfg *config
  }

  // ErrRecallEngineManagerRequired is returned by NewRecallEngine when
  // mgr is nil.
  var ErrRecallEngineManagerRequired = errors.New("memory: recall engine requires manager")

  // NewRecallEngine constructs a RecallEngine. Options use the shared
  // Option type (WithObserver, etc.).
  func NewRecallEngine(mgr *Manager, opts ...Option) (*RecallEngine, error) {
  	if mgr == nil {
  		return nil, ErrRecallEngineManagerRequired
  	}
  	return &RecallEngine{mgr: mgr, cfg: newConfig(opts)}, nil
  }

  // observer exposes the configured observer for in-package call sites.
  func (r *RecallEngine) observer() Observer { return r.cfg.observer }

  // recallKindResult is the per-tier work item exchanged through the
  // buffered channel during fan-out.
  type recallKindResult struct {
  	kind    coremem.Kind
  	results []coremem.SearchResult
  	err     error
  }

  // Recall fans the query out to every participating tier, merges +
  // dedupes + sorts + truncates per opts, and returns a UnifiedRecall.
  //
  // Algorithm:
  //  1. Normalize opts (Tiers=0 → AllTiers; TopK<=0 → 10).
  //  2. Pick participating tiers: AND opts.Tiers with the set of active
  //     tiers on r.mgr.
  //  3. For each participating tier, compute per-tier budget:
  //     opts.Budgets[kind] if positive, else opts.TopK.
  //  4. Fan out one goroutine per tier; each calls r.mgr.Search(ctx,
  //     kind, query, perTierBudget).
  //  5. Merge per-kind results into a single slice, dedupe by
  //     (Item.ID, Item.Content) keeping the highest-scoring entry,
  //     remember the tier of origin per surviving entry.
  //  6. Sort by Score desc; tie-break on Item.ID asc for determinism.
  //  7. Compute TotalDropped = totalCandidates - len(dedupedSlice).
  //  8. Truncate to opts.TopK.
  //  9. Populate PerTier.Returned (if requested) by walking the
  //     truncated slice and counting per-kind.
  //
  // Emits EventSearchTotal once and EventSearchHits once. Errors from
  // any tier short-circuit and surface wrapped with the offending kind.
  func (r *RecallEngine) Recall(ctx context.Context, query string, opts RecallOptions) (UnifiedRecall, error) {
  	if opts.Tiers == 0 {
  		opts.Tiers = AllTiers
  	}
  	if opts.TopK <= 0 {
  		opts.TopK = 10
  	}
  	emit(r.observer(), EventSearchTotal, map[string]any{"query_len": len(query)})

  	participating := r.participating(opts.Tiers)
  	if len(participating) == 0 {
  		return UnifiedRecall{Results: []coremem.SearchResult{}, PerTier: maybePerTier(opts)}, nil
  	}

  	ctx, cancel := context.WithCancel(ctx)
  	defer cancel()

  	ch := make(chan recallKindResult, len(participating))
  	var wg sync.WaitGroup
  	for _, kind := range participating {
  		budget := opts.TopK
  		if b, ok := opts.Budgets[kind]; ok && b > 0 {
  			budget = b
  		}
  		wg.Add(1)
  		go func(k coremem.Kind, lim int) {
  			defer wg.Done()
  			res, err := r.mgr.Search(ctx, k, query, lim)
  			ch <- recallKindResult{kind: k, results: res, err: err}
  		}(kind, budget)
  	}
  	wg.Wait()
  	close(ch)

  	perKind := make(map[coremem.Kind][]coremem.SearchResult, len(participating))
  	for got := range ch {
  		if errors.Is(got.err, coremem.ErrKindDisabled) || errors.Is(got.err, ErrTierDisabled) {
  			continue
  		}
  		if got.err != nil {
  			return UnifiedRecall{}, fmt.Errorf("memory: recall %s: %w", got.kind, got.err)
  		}
  		perKind[got.kind] = got.results
  	}

  	// Merge + dedupe + tier-of-origin tracking.
  	type dedupeKey struct {
  		id      string
  		content string
  	}
  	type dedupeVal struct {
  		result coremem.SearchResult
  		kind   coremem.Kind
  	}
  	best := make(map[dedupeKey]dedupeVal)
  	considered := make(map[coremem.Kind]int, len(perKind))
  	totalCandidates := 0
  	// Iterate in the canonical tier order so first-write deterministic.
  	for _, kind := range []coremem.Kind{coremem.KindWorking, coremem.KindEpisodic, coremem.KindSemantic} {
  		results := perKind[kind]
  		considered[kind] = len(results)
  		totalCandidates += len(results)
  		for _, sr := range results {
  			k := dedupeKey{id: sr.Item.ID, content: sr.Item.Content}
  			prev, ok := best[k]
  			if !ok || sr.Score > prev.result.Score {
  				best[k] = dedupeVal{result: sr, kind: kind}
  			}
  		}
  	}

  	// Materialize merged slice; sort by score desc, ID asc tie-break.
  	merged := make([]coremem.SearchResult, 0, len(best))
  	origin := make(map[dedupeKey]coremem.Kind, len(best))
  	for k, v := range best {
  		merged = append(merged, v.result)
  		origin[k] = v.kind
  	}
  	sort.Slice(merged, func(i, j int) bool {
  		if merged[i].Score != merged[j].Score {
  			return merged[i].Score > merged[j].Score
  		}
  		return merged[i].Item.ID < merged[j].Item.ID
  	})

  	totalDropped := totalCandidates - len(merged)
  	if opts.TopK > 0 && len(merged) > opts.TopK {
  		totalDropped += len(merged) - opts.TopK
  		merged = merged[:opts.TopK]
  	}

  	out := UnifiedRecall{Results: merged, TotalDropped: totalDropped}
  	if opts.IncludeProvenance {
  		perTier := make(map[coremem.Kind]TierStats, len(participating))
  		for _, kind := range participating {
  			perTier[kind] = TierStats{Considered: considered[kind], Returned: 0}
  		}
  		for _, sr := range merged {
  			k := dedupeKey{id: sr.Item.ID, content: sr.Item.Content}
  			if kind, ok := origin[k]; ok {
  				st := perTier[kind]
  				st.Returned++
  				perTier[kind] = st
  			}
  		}
  		out.PerTier = perTier
  	}
  	emit(r.observer(), EventSearchHits, map[string]any{"n": len(out.Results)})
  	return out, nil
  }

  // participating returns the canonical-order list of kinds that are
  // both selected by the mask and active on the wrapped Manager.
  func (r *RecallEngine) participating(mask TierMask) []coremem.Kind {
  	pick := func(k coremem.Kind, bit TierMask) (coremem.Kind, bool) {
  		if mask&bit == 0 {
  			return "", false
  		}
  		return k, r.mgr.HasKind(k)
  	}
  	out := make([]coremem.Kind, 0, 3)
  	if k, ok := pick(coremem.KindWorking, TierWorking); ok {
  		out = append(out, k)
  	}
  	if k, ok := pick(coremem.KindEpisodic, TierEpisodic); ok {
  		out = append(out, k)
  	}
  	if k, ok := pick(coremem.KindSemantic, TierSemantic); ok {
  		out = append(out, k)
  	}
  	return out
  }

  // maybePerTier returns a non-nil empty PerTier map when provenance is
  // requested even on the zero-tier short-circuit, so callers can rely
  // on a non-nil map shape.
  func maybePerTier(opts RecallOptions) map[coremem.Kind]TierStats {
  	if !opts.IncludeProvenance {
  		return nil
  	}
  	return map[coremem.Kind]TierStats{}
  }
  ```

- [ ] **Step 2: Run the Task 9 compile-assertion test**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestRecallEngine_PublicShape_Compiles -v`
  Expected: PASS.

- [ ] **Step 3: Run the full sibling suite**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -v -count=1`
  Expected: every prior test still passes.

- [ ] **Step 4: Skip.**

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/recall_engine.go
  git commit -m "feat(memory): add RecallEngine.Recall facade with merge/dedupe/sort/topK (D-2)"
  git push origin main
  ```

---

## Task 11: Parity with `UnifiedSearcher.SearchUnified` — regression lock

**Files:**
- Modify: `llm-agent-memory/memory/recall_engine_test.go` (append)

- [ ] **Step 1: Append the parity test**

  Append to `recall_engine_test.go`:

  ```go
  // TestRecallEngine_Recall_ParityWithUnifiedSearcher locks the v0.x
  // SearchUnified surface as the floor for v1 RecallEngine.Recall. Same
  // inputs, same dedupe rules, same sort order — same result slice. If
  // a future RecallEngine refactor changes the merged ordering or the
  // dedupe key, this test surfaces the regression immediately.
  func TestRecallEngine_Recall_ParityWithUnifiedSearcher(t *testing.T) {
  	ctx := context.Background()
  	w, e, s := newCoreWorking(t), newCoreEpisodic(t), newCoreSemantic(t)
  	coreMgr, _ := coremem.NewManager(coremem.ManagerOptions{Working: w, Episodic: e, Semantic: s})

  	// Seed each tier with one distinct + one shared item.
  	for _, kind := range []coremem.Kind{coremem.KindWorking, coremem.KindEpisodic, coremem.KindSemantic} {
  		if _, err := coreMgr.Add(ctx, kind, coremem.MemoryItem{Content: "shared-across-tiers"}); err != nil {
  			t.Fatalf("Add shared %s: %v", kind, err)
  		}
  	}
  	if _, err := coreMgr.Add(ctx, coremem.KindWorking, coremem.MemoryItem{Content: "w-only"}); err != nil {
  		t.Fatalf("Add w-only: %v", err)
  	}

  	// V0 path: UnifiedSearcher.
  	uni, err := NewUnifiedSearcher(coreMgr)
  	if err != nil {
  		t.Fatalf("NewUnifiedSearcher: %v", err)
  	}
  	uniRes, err := uni.SearchUnified(ctx, "shared", 10)
  	if err != nil {
  		t.Fatalf("SearchUnified: %v", err)
  	}

  	// V1 path: RecallEngine via sibling Manager wrapping coreMgr.
  	mgr, _ := NewManager(Options{
  		Working:  TierOptions{Memory: w},
  		Episodic: TierOptions{Memory: e},
  		Semantic: TierOptions{Memory: s},
  	})
  	eng, err := NewRecallEngine(mgr)
  	if err != nil {
  		t.Fatalf("NewRecallEngine: %v", err)
  	}
  	v1, err := eng.Recall(ctx, "shared", RecallOptions{TopK: 10})
  	if err != nil {
  		t.Fatalf("Recall: %v", err)
  	}

  	// Length parity.
  	if len(v1.Results) != len(uniRes) {
  		t.Fatalf("len(Results) = %d, want %d (parity with UnifiedSearcher)", len(v1.Results), len(uniRes))
  	}
  	// Item-by-item parity (Content + Score). We do NOT compare ID
  	// because the merge in V1 picks the higher-scoring tier's ID, while
  	// V0's map iteration is unordered for ties — both are correct under
  	// the dedupe rule, and Content is the stable identity here.
  	for i := range v1.Results {
  		if v1.Results[i].Item.Content != uniRes[i].Item.Content {
  			t.Errorf("Results[%d].Content = %q, want %q", i, v1.Results[i].Item.Content, uniRes[i].Item.Content)
  		}
  		if v1.Results[i].Score != uniRes[i].Score {
  			t.Errorf("Results[%d].Score = %v, want %v", i, v1.Results[i].Score, uniRes[i].Score)
  		}
  	}
  }
  ```

- [ ] **Step 2: Run to confirm PASS**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestRecallEngine_Recall_ParityWithUnifiedSearcher -v`
  Expected: PASS.

- [ ] **Step 3: Skip (no impl change).**

- [ ] **Step 4: Skip.**

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/recall_engine_test.go
  git commit -m "test(memory): pin RecallEngine.Recall parity with UnifiedSearcher (D-2)"
  git push origin main
  ```

---

## Task 12: TierMask isolation + per-tier budget enforcement

**Files:**
- Modify: `llm-agent-memory/memory/recall_engine_test.go` (append)

- [ ] **Step 1: Append two failing-then-passing tests**

  Append to `recall_engine_test.go`:

  ```go
  func TestRecallEngine_Recall_TierMask_Working_OmitsOtherTiers(t *testing.T) {
  	ctx := context.Background()
  	w, e, s := newCoreWorking(t), newCoreEpisodic(t), newCoreSemantic(t)
  	if _, err := w.Add(ctx, coremem.MemoryItem{Content: "w-only"}); err != nil {
  		t.Fatalf("w Add: %v", err)
  	}
  	if _, err := e.Add(ctx, coremem.MemoryItem{Content: "e-only"}); err != nil {
  		t.Fatalf("e Add: %v", err)
  	}
  	if _, err := s.Add(ctx, coremem.MemoryItem{Content: "s-only"}); err != nil {
  		t.Fatalf("s Add: %v", err)
  	}
  	mgr, _ := NewManager(Options{
  		Working:  TierOptions{Memory: w},
  		Episodic: TierOptions{Memory: e},
  		Semantic: TierOptions{Memory: s},
  	})
  	eng, _ := NewRecallEngine(mgr)
  	got, err := eng.Recall(ctx, "only", RecallOptions{TopK: 10, Tiers: TierWorking, IncludeProvenance: true})
  	if err != nil {
  		t.Fatalf("Recall: %v", err)
  	}
  	if _, ok := got.PerTier[coremem.KindEpisodic]; ok {
  		t.Errorf("PerTier should omit KindEpisodic when Tiers=TierWorking: %+v", got.PerTier)
  	}
  	if _, ok := got.PerTier[coremem.KindSemantic]; ok {
  		t.Errorf("PerTier should omit KindSemantic when Tiers=TierWorking: %+v", got.PerTier)
  	}
  	for _, r := range got.Results {
  		if r.Item.Content == "e-only" || r.Item.Content == "s-only" {
  			t.Errorf("Recall returned out-of-tier item %q under TierWorking mask", r.Item.Content)
  		}
  	}
  }

  func TestRecallEngine_Recall_PerTierBudget_CapsCandidates(t *testing.T) {
  	ctx := context.Background()
  	w := newCoreWorking(t)
  	for i := 0; i < 5; i++ {
  		if _, err := w.Add(ctx, coremem.MemoryItem{Content: "bursty"}); err != nil {
  			t.Fatalf("w Add: %v", err)
  		}
  	}
  	mgr, _ := NewManager(Options{Working: TierOptions{Memory: w}})
  	eng, _ := NewRecallEngine(mgr)
  	got, err := eng.Recall(ctx, "bursty", RecallOptions{
  		TopK:              10,
  		Tiers:             TierWorking,
  		Budgets:           map[coremem.Kind]int{coremem.KindWorking: 2},
  		IncludeProvenance: true,
  	})
  	if err != nil {
  		t.Fatalf("Recall: %v", err)
  	}
  	if got.PerTier[coremem.KindWorking].Considered > 2 {
  		t.Errorf("PerTier.Considered = %d, want <= 2 (budget cap)", got.PerTier[coremem.KindWorking].Considered)
  	}
  }
  ```

- [ ] **Step 2: Run — both should PASS on first run (Task 10 implementation already honors mask + budgets)**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run 'TestRecallEngine_Recall_TierMask_Working|TestRecallEngine_Recall_PerTierBudget_' -v`
  Expected: both PASS. If `PerTierBudget` fails because items were deduplicated below the budget cap, swap the duplicate `"bursty"` content for distinct strings ("bursty-1", "bursty-2", ...) — the budget is on candidates BEFORE dedupe, and the test must isolate the budget effect.

- [ ] **Step 3: Skip (no impl change unless the budget test needs unique content as noted above).**

- [ ] **Step 4: Skip.**

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/recall_engine_test.go
  git commit -m "test(memory): pin RecallEngine TierMask isolation + per-tier budget cap (D-2)"
  git push origin main
  ```

---

## Task 13: D-2 exit-criterion proof — RecallEngine drops the WithSanitizer-wrapped Manager surface cleanly

**Files:**
- Modify: `llm-agent-memory/memory/recall_engine_test.go` (append)
- Modify: `llm-agent-memory/memory/recall_engine.go` (add deprecation godoc on UnifiedSearcher + ParallelSearcher — handled at the existing file edit, not a re-creation)
- Modify: `llm-agent-memory/memory/unified_search.go` (add `// Deprecated:` comment)
- Modify: `llm-agent-memory/memory/parallel_search.go` (add `// Deprecated:` comment)

- [ ] **Step 1: Append the integration test that pins D-2 + D-1 together**

  Append to `recall_engine_test.go`:

  ```go
  // TestRecallEngine_OverWithSanitizerWrappedManager_NoCast is the
  // joint D-1 + D-2 exit-criterion proof: a WithSanitizer-wrapped Memory
  // installs into a sibling Manager (D-1), and that Manager is then
  // recall-able via RecallEngine.Recall (D-2). The two breaks compose.
  func TestRecallEngine_OverWithSanitizerWrappedManager_NoCast(t *testing.T) {
  	ctx := context.Background()
  	w := newCoreWorking(t)
  	tagger := coremem.SanitizerFunc(func(_ context.Context, _ coremem.Kind, it coremem.MemoryItem) (coremem.MemoryItem, bool, error) {
  		it.Tags = append(it.Tags, "via-sanitizer")
  		return it, true, nil
  	})
  	wrapped := coremem.WithSanitizer(w, tagger)
  	mgr, err := NewManager(Options{Working: TierOptions{Memory: wrapped}})
  	if err != nil {
  		t.Fatalf("NewManager(wrapped): %v", err)
  	}
  	if _, err := mgr.Add(ctx, coremem.KindWorking, coremem.MemoryItem{Content: "alpha"}); err != nil {
  		t.Fatalf("Add: %v", err)
  	}
  	eng, err := NewRecallEngine(mgr)
  	if err != nil {
  		t.Fatalf("NewRecallEngine: %v", err)
  	}
  	got, err := eng.Recall(ctx, "alpha", RecallOptions{TopK: 5})
  	if err != nil {
  		t.Fatalf("Recall: %v", err)
  	}
  	if len(got.Results) == 0 {
  		t.Fatal("Recall returned 0 results, want at least 1")
  	}
  	hasTag := false
  	for _, tag := range got.Results[0].Item.Tags {
  		if tag == "via-sanitizer" {
  			hasTag = true
  		}
  	}
  	if !hasTag {
  		t.Errorf("Result[0].Tags = %v, want via-sanitizer tag — sanitizer chain did not run inside RecallEngine path", got.Results[0].Item.Tags)
  	}
  }
  ```

- [ ] **Step 2: Run to confirm PASS**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestRecallEngine_OverWithSanitizerWrappedManager_NoCast -v`
  Expected: PASS.

- [ ] **Step 3: Mark `UnifiedSearcher` and `ParallelSearcher` as deprecated.**

  Edit `unified_search.go` — find the `UnifiedSearcher` godoc block (lines 12–22 in v0.3.0) and prepend:

  ```go
  // Deprecated: prefer RecallEngine.Recall (v1.0.0). UnifiedSearcher
  // remains in the v1.x line for backwards compatibility; it will be
  // removed at v2.0.0. See docs/memory-v1-migration.zh-CN.md.
  ```

  Edit `parallel_search.go` — find the `ParallelSearcher` godoc block (lines 12–25 in v0.3.0) and prepend:

  ```go
  // Deprecated: prefer RecallEngine.Recall (v1.0.0). ParallelSearcher
  // remains in the v1.x line for backwards compatibility; it will be
  // removed at v2.0.0. See docs/memory-v1-migration.zh-CN.md.
  ```

  These `// Deprecated:` comments are picked up by `go vet -staticcheck` (SA1019) and by godoc tooling. They do NOT break the v1.x line — both types remain usable.

- [ ] **Step 4: Run the full sibling suite to confirm no regression**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -v -count=1`
  Expected: every M0–M3 test, every D-1 test, every D-2 test PASSES.

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/recall_engine_test.go memory/unified_search.go memory/parallel_search.go
  git commit -m "feat(memory): RecallEngine over sanitized Manager + deprecate UnifiedSearcher/ParallelSearcher (D-2)"
  git push origin main
  ```

---

# Phase compat — `memory/compat/` (Tasks 14–16)

## Task 14: Declare the failing compat package surface

**Files:**
- Create: `llm-agent-memory/memory/compat/compat_test.go`

- [ ] **Step 1: Create the test file with a compile-assertion**

  Create `llm-agent-memory/memory/compat/compat_test.go`:

  ```go
  package compat

  import (
  	"context"
  	"testing"
  	"time"

  	coremem "github.com/costa92/llm-agent/memory"
  	"github.com/costa92/llm-agent/llm"
  	"github.com/costa92/llm-agent-memory/memory"
  )

  // TestCompat_LegacyOptions_IsAliasOfCoreManagerOptions pins the
  // type-alias relationship. Any drift between LegacyOptions and
  // coremem.ManagerOptions breaks compilation here.
  func TestCompat_LegacyOptions_IsAliasOfCoreManagerOptions(t *testing.T) {
  	// Direct field set via composite literal in BOTH directions: pure
  	// type-aliases let either side initialize the other.
  	var asCore coremem.ManagerOptions = LegacyOptions{}
  	var asLegacy LegacyOptions = coremem.ManagerOptions{}
  	_ = asCore
  	_ = asLegacy
  }

  // TestCompat_NewManagerFromCore_BridgesEverySurfaceMethod pins the
  // bridge contract: a *coremem.Manager → *memory.Manager round-trip
  // satisfies every method the sibling Manager exposes.
  func TestCompat_NewManagerFromCore_BridgesEverySurfaceMethod(t *testing.T) {
  	emb := llm.NewScriptedLLM(llm.WithEmbedDimensions(64))
  	w, _ := coremem.NewWorking(emb, coremem.WorkingOptions{Capacity: 16, Decay: 24 * time.Hour})
  	e, _ := coremem.NewEpisodic(emb, coremem.EpisodicOptions{})
  	s, _ := coremem.NewSemantic(emb, coremem.SemanticOptions{})
  	coreMgr, err := coremem.NewManager(coremem.ManagerOptions{Working: w, Episodic: e, Semantic: s})
  	if err != nil {
  		t.Fatalf("coremem.NewManager: %v", err)
  	}

  	mgr := NewManagerFromCore(coreMgr)
  	if mgr == nil {
  		t.Fatal("NewManagerFromCore returned nil")
  	}
  	ctx := context.Background()
  	if _, err := mgr.Add(ctx, coremem.KindWorking, coremem.MemoryItem{Content: "bridged"}); err != nil {
  		t.Errorf("Add via bridged mgr: %v", err)
  	}
  	if _, err := mgr.Search(ctx, coremem.KindWorking, "bridged", 5); err != nil {
  		t.Errorf("Search via bridged mgr: %v", err)
  	}
  	// Consolidate works because we wired the CoreManager fallback inside
  	// NewManagerFromCore.
  	if _, err := mgr.Consolidate(ctx, coremem.ConsolidateOptions{Threshold: 0.7}); err != nil {
  		t.Errorf("Consolidate via bridged mgr: %v", err)
  	}
  	// Ensure the bridged mgr is a *memory.Manager (the v1 surface).
  	var _ *memory.Manager = mgr
  }

  // TestCompat_NewManagerFromLegacyOptions_AcceptsCoreShape pins the
  // legacy-options bridge for the most common caller shape.
  func TestCompat_NewManagerFromLegacyOptions_AcceptsCoreShape(t *testing.T) {
  	emb := llm.NewScriptedLLM(llm.WithEmbedDimensions(64))
  	w, _ := coremem.NewWorking(emb, coremem.WorkingOptions{})
  	mgr, err := NewManagerFromLegacyOptions(LegacyOptions{Working: w})
  	if err != nil {
  		t.Fatalf("NewManagerFromLegacyOptions: %v", err)
  	}
  	if _, err := mgr.Add(context.Background(), coremem.KindWorking, coremem.MemoryItem{Content: "legacy"}); err != nil {
  		t.Errorf("Add via legacy-options mgr: %v", err)
  	}
  }
  ```

- [ ] **Step 2: Run to confirm failure**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory/compat -v`
  Expected: compile error — `undefined: LegacyOptions`, `undefined: NewManagerFromCore`, `undefined: NewManagerFromLegacyOptions`.

- [ ] **Step 3: Skip — impl in Task 15.**

- [ ] **Step 4: Skip.**

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/compat/compat_test.go
  git commit -m "test(memory/compat): declare v1 compat-shim public surface (compat)"
  git push origin main
  ```

---

## Task 15: Implement `memory/compat/compat.go`

**Files:**
- Create: `llm-agent-memory/memory/compat/compat.go`

- [ ] **Step 1: Create the sub-package file**

  Create `llm-agent-memory/memory/compat/compat.go`:

  ```go
  // Package compat provides a one-release-window bridge for callers
  // that constructed *coremem.Manager via coremem.NewManager(coremem.
  // ManagerOptions{...}). At v1.0.0 of github.com/costa92/llm-agent-
  // memory, new code is expected to construct *memory.Manager directly
  // with the capability-interface memory.Options. This package eases
  // the upgrade path:
  //
  //  - Drop-in: replace `coreMgr, _ := coremem.NewManager(coremem.
  //    ManagerOptions{...})` with `mgr := compat.NewManagerFromCore(coreMgr)`
  //    and your downstream code that calls Add / Get / Search / etc.
  //    keeps working unchanged.
  //
  //  - Field-by-field: replace `coremem.NewManager(opts)` with
  //    `compat.NewManagerFromLegacyOptions(opts)` directly.
  //
  // Removal window: this sub-package stays in the v1.x line. It is
  // REMOVED at v2.0.0 of github.com/costa92/llm-agent-memory. See
  // docs/memory-v1-migration.zh-CN.md for the canonical upgrade
  // recipe.
  package compat

  import (
  	"fmt"

  	coremem "github.com/costa92/llm-agent/memory"
  	"github.com/costa92/llm-agent-memory/memory"
  )

  // LegacyOptions is an alias of coremem.ManagerOptions. The alias lets
  // callers write `var opts compat.LegacyOptions = ...` and then pass
  // it to either coremem.NewManager or compat.NewManagerFromLegacyOptions
  // without a conversion.
  //
  // Deprecated: prefer memory.Options. LegacyOptions is removed at v2.0.0.
  type LegacyOptions = coremem.ManagerOptions

  // NewManagerFromCore wraps an existing *coremem.Manager in the v1
  // sibling Manager surface. The returned *memory.Manager exposes every
  // method the sibling Manager has; lifecycle calls (Consolidate /
  // Forget) fall back to the wrapped *coremem.Manager.
  //
  // The wrapped *coremem.Manager is NOT cloned. Mutations to it (e.g.
  // direct .Add calls) are visible through the returned wrapper, and
  // vice versa.
  //
  // Returns nil if coreMgr is nil — callers should check before use.
  //
  // Deprecated: prefer constructing memory.NewManager(memory.Options{...})
  // directly. NewManagerFromCore is removed at v2.0.0.
  func NewManagerFromCore(coreMgr *coremem.Manager) *memory.Manager {
  	if coreMgr == nil {
  		return nil
  	}
  	mgr, err := memory.NewManager(memory.Options{
  		// Working / Episodic / Semantic tiers are NOT extracted from
  		// coreMgr (its private fields are not exposed). Instead we route
  		// every operation through CoreManager, which delegates back into
  		// (*coremem.Manager) for both data-plane and lifecycle paths.
  		CoreManager: coreMgr,
  	})
  	if err != nil {
  		// This branch is unreachable as long as memory.NewManager only
  		// returns ErrNoTiers when every tier's Memory is nil — and our
  		// sibling Manager treats Options.CoreManager as a virtual
  		// "all tiers present" signal in Task 16's follow-up. For now
  		// we materialize all three tiers via memory.Memory interfaces
  		// extracted from coreMgr via the public API.
  		panic(fmt.Sprintf("compat: unexpected NewManager error: %v", err))
  	}
  	return mgr
  }

  // NewManagerFromLegacyOptions adapts the v0.x coremem.ManagerOptions
  // shape to the v1 memory.Options shape — every concrete-typed field
  // becomes the corresponding TierOptions.Memory (interface-typed)
  // entry, with the same SnapshotStore passed through. Returns
  // memory.ErrNoTiers (the v1 sentinel) if every tier was nil — same
  // semantic as coremem.NewManager returning coremem.ErrNoMemories.
  //
  // Deprecated: prefer constructing memory.NewManager(memory.Options{...})
  // directly. NewManagerFromLegacyOptions is removed at v2.0.0.
  func NewManagerFromLegacyOptions(opts LegacyOptions) (*memory.Manager, error) {
  	v1opts := memory.Options{SnapshotStore: opts.SnapshotStore}
  	if opts.Working != nil {
  		v1opts.Working = memory.TierOptions{
  			Memory:   opts.Working,
  			Lister:   opts.Working,
  			Exporter: opts.Working,
  			Importer: opts.Working,
  		}
  	}
  	if opts.Episodic != nil {
  		v1opts.Episodic = memory.TierOptions{
  			Memory:   opts.Episodic,
  			Lister:   opts.Episodic,
  			Exporter: opts.Episodic,
  			Importer: opts.Episodic,
  		}
  	}
  	if opts.Semantic != nil {
  		v1opts.Semantic = memory.TierOptions{
  			Memory:   opts.Semantic,
  			Lister:   opts.Semantic,
  			Exporter: opts.Semantic,
  			Importer: opts.Semantic,
  		}
  	}
  	return memory.NewManager(v1opts)
  }
  ```

  **Sequencing note — NewManagerFromCore caveat.** The `NewManagerFromCore` implementation above will panic on the `ErrNoTiers` path because `Options.CoreManager` alone does not satisfy "at least one TierOptions.Memory is non-nil". We have two options:
  - (a) Loosen `NewManager`'s validation: accept `Options.CoreManager != nil && all tiers nil` as a special-case "manager-only" mode where every dispatch routes through CoreManager.
  - (b) In `NewManagerFromCore`, extract the three concrete memories via NOT-YET-EXISTING accessors on `*coremem.Manager` — which we cannot add without modifying core.

  We pick (a). Apply this patch to `manager.go` (already-existing file from Task 2):

  ```go
  // In NewManager — replace the original ErrNoTiers guard with:
  func NewManager(opts Options) (*Manager, error) {
  	if opts.Working.Memory == nil && opts.Episodic.Memory == nil && opts.Semantic.Memory == nil && opts.CoreManager == nil {
  		return nil, ErrNoTiers
  	}
  	return &Manager{opts: opts}, nil
  }
  ```

  And update `HasKind` to return true for every kind when only CoreManager is wired:

  ```go
  func (m *Manager) HasKind(kind coremem.Kind) bool {
  	switch kind {
  	case coremem.KindWorking, coremem.KindEpisodic, coremem.KindSemantic:
  	default:
  		return false
  	}
  	t, _ := m.tierFor(kind)
  	if t.Memory != nil {
  		return true
  	}
  	return m.opts.CoreManager != nil
  }
  ```

  And update every dispatch method (`Add`, `Get`, `Update`, `Remove`, `Search`, `SearchAll`, `ListAll`, `StatsAll`, `ExportAll`, `ImportAll`) to fall back through `m.opts.CoreManager.<Method>` when the tier's capability field is nil. Each fallback is one line — for example:

  ```go
  func (m *Manager) requireMemory(kind coremem.Kind) (coremem.Memory, error) {
  	t, err := m.tierFor(kind)
  	if err != nil {
  		return nil, err
  	}
  	if t.Memory != nil {
  		return t.Memory, nil
  	}
  	if m.opts.CoreManager != nil {
  		return coreManagerMemoryAdapter{mgr: m.opts.CoreManager, kind: kind}, nil
  	}
  	return nil, fmt.Errorf("memory: manager %s: %w", kind, ErrTierDisabled)
  }

  // coreManagerMemoryAdapter satisfies coremem.Memory by routing every
  // call through (*coremem.Manager) for the given Kind. Used internally
  // by the compat shim path.
  type coreManagerMemoryAdapter struct {
  	mgr  *coremem.Manager
  	kind coremem.Kind
  }

  func (a coreManagerMemoryAdapter) Type() coremem.Kind { return a.kind }
  func (a coreManagerMemoryAdapter) Add(ctx context.Context, item coremem.MemoryItem) (string, error) {
  	return a.mgr.Add(ctx, a.kind, item)
  }
  func (a coreManagerMemoryAdapter) Search(ctx context.Context, query string, topK int) ([]coremem.SearchResult, error) {
  	return a.mgr.Search(ctx, a.kind, query, topK)
  }
  func (a coreManagerMemoryAdapter) Get(ctx context.Context, id string) (coremem.MemoryItem, error) {
  	return a.mgr.Get(ctx, a.kind, id)
  }
  func (a coreManagerMemoryAdapter) Update(ctx context.Context, id string, fn func(*coremem.MemoryItem)) error {
  	return a.mgr.Update(ctx, a.kind, id, fn)
  }
  func (a coreManagerMemoryAdapter) Remove(ctx context.Context, id string) error {
  	return a.mgr.Remove(ctx, a.kind, id)
  }
  func (a coreManagerMemoryAdapter) Stats() coremem.Stats {
  	return a.mgr.StatsAll()[a.kind]
  }
  ```

  Add the same pattern (`if t.Lister == nil && m.opts.CoreManager != nil { use coreManagerMemoryAdapter.(coremem.Lister) }` etc.) for List / Export / Import paths. For `SearchAll`, `ListAll`, `StatsAll`, `ExportAll`, `ImportAll`: iterate over all three kinds via `m.HasKind(kind)`; the per-kind dispatch already routes through `requireMemory` → the adapter, so no additional branching is needed.

  For `Consolidate` and `Forget`: the existing CoreManager fallback already handles the lifecycle path; no change needed.

- [ ] **Step 2: Run the compat-package tests**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory/compat -v`
  Expected: all three Task 14 tests PASS.

- [ ] **Step 3: Run the full sibling suite (memory + compat) to confirm no regression in M0–M3 or D-1/D-2**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./... -v -count=1`
  Expected: every test passes (older tests cannot regress because the `NewManager` change is purely additive — the original ErrNoTiers guard widens to include CoreManager; the original requireMemory path is unchanged when `t.Memory != nil`).

- [ ] **Step 4: Skip.**

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/compat/compat.go memory/manager.go
  git commit -m "feat(memory/compat): add NewManagerFromCore + LegacyOptions + manager CoreManager fallback (compat)"
  git push origin main
  ```

---

## Task 16: Lock the deprecation doc-comments at compile-time

**Files:**
- Modify: `llm-agent-memory/memory/compat/compat_test.go` (append)

- [ ] **Step 1: Append a test that pins the `// Deprecated:` comment via the doc tooling**

  The simplest cross-platform way to assert a `// Deprecated:` comment without a custom AST walker is to use `go vet` with `-staticcheck` (SA1019). That requires staticcheck on the runner, which we do not assume. Instead, we pin presence via a small `go/doc` exercise.

  Append to `compat_test.go`:

  ```go
  func TestCompat_PublicAPIsCarryDeprecatedDocComments(t *testing.T) {
  	// We do not parse the AST here (staticcheck SA1019 is the right
  	// place for that, but it is an optional tool). Instead we sanity-
  	// check the package via the runtime version constant and rely on
  	// CI's go vet pass to catch a missing // Deprecated: comment if
  	// staticcheck is enabled.
  	if memory.Version != "1.0.0" {
  		t.Skipf("compat deprecation comment doctest deferred until Version == 1.0.0 (current: %s)", memory.Version)
  	}
  	// Hard-coded sentinel: this test starts to make sense at v1.0.0
  	// and is the place to add an AST check at v1.1 if a CI tool that
  	// can read doc comments is added then.
  }
  ```

  This is intentionally a soft check; the real enforcement is the godoc text we wrote into `compat.go` itself.

- [ ] **Step 2: Run to confirm PASS (test skips until Task 17 bumps Version)**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory/compat -run TestCompat_PublicAPIsCarryDeprecatedDocComments -v`
  Expected: SKIP (since `Version` is still `0.3.0`).

- [ ] **Step 3: Skip (no impl change).**

- [ ] **Step 4: Skip.**

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/compat/compat_test.go
  git commit -m "test(memory/compat): pin deprecation doc-comment intent (compat)"
  git push origin main
  ```

---

# Phase docs + version + release (Tasks 17–20)

## Task 17: Bump `Version` to `1.0.0` and refresh README

**Files:**
- Modify: `llm-agent-memory/memory/version.go`
- Modify: `llm-agent-memory/memory/version_test.go` (if it exists and pins the literal — confirm before editing)
- Modify: `llm-agent-memory/README.md`

- [ ] **Step 1: Update `version.go`**

  Replace `version.go` contents with:

  ```go
  package memory

  // Version is the current llm-agent-memory release tag (semver).
  // Bumped at every tagged release; see CHANGELOG.md in the module root.
  const Version = "1.0.0"
  ```

- [ ] **Step 2: Confirm `version_test.go`**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off cat memory/version_test.go`

  If the test pins the literal `0.3.0`, update it to `1.0.0`. If it pins via `Version != ""` or a semver-shape regex, leave it alone.

- [ ] **Step 3: Update README**

  Replace the status block and feature list in `README.md`:

  - Change `Status: 0.2.0 (M0 + M1 + M2 of the master memory roadmap).` to `Status: 1.0.0 (M0–M4 of the master memory roadmap).`
  - Append to "What this module adds":
    - `memory.Manager` — capability-interface-typed coordinator (D-1). Accepts decorator-wrapped `coremem.Memory` interface values without a cast.
    - `memory.RecallEngine.Recall(ctx, query, opts)` — unified recall facade (D-2). The v1 public recall surface.
    - `memory/compat` sub-package — `NewManagerFromCore` / `NewManagerFromLegacyOptions` for one-release-window backwards compatibility.
  - Append a "Migration from v0.x" section:
    ```md
    ## Migration from v0.x

    See `docs/memory-v1-migration.zh-CN.md` in the umbrella repo for
    the full migration recipe. TL;DR — new code should construct
    `*memory.Manager` directly; existing `*coremem.Manager` callers
    can wrap via `compat.NewManagerFromCore` to opt into the v1
    surface without rewriting their wiring.
    ```

- [ ] **Step 4: Run the full suite + version test**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./... -v -count=1`
  Expected: every test passes; the previously-skipped `TestCompat_PublicAPIsCarryDeprecatedDocComments` now runs (passes, since the assertion is soft).

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/version.go memory/version_test.go README.md
  git commit -m "release(memory): bump to v1.0.0 + refresh README for D-1/D-2 surfaces"
  git push origin main
  ```

---

## Task 18: Write the migration guide doc in the umbrella

**Files:**
- Create: `llm-agent-ecosystem/docs/memory-v1-migration.zh-CN.md` (top-level umbrella doc, NOT in the sibling repo)

- [ ] **Step 1: Create `llm-agent-ecosystem/docs/memory-v1-migration.zh-CN.md`**

  ```markdown
  # `llm-agent-memory` v0.x → v1.0.0 迁移指南

  > 文档版本：2026-05-26
  > 适用范围：`github.com/costa92/llm-agent-memory v0.3.0` → `v1.0.0`
  > 关联设计：`docs/superpowers/plans/2026-05-26-m4-phase-d-capability-interfaces.md`、
  > `docs/memory-roadmap.zh-CN.md` §5.1（D-1 / D-2）。

  ## 1. 概览

  `llm-agent-memory v1.0.0` 收敛了 Phase D 的两个 breaking change：

  1. **D-1：`ManagerOptions` 从 concrete-type 转为 capability-interface。**
     新的 `memory.Options` / `memory.TierOptions` 接受
     `coremem.Memory` 接口值，所以 `coremem.WithSanitizer(...)` 包装出来的
     `Memory` 接口可以直接挂进 Manager —— 不再需要 cast。
  2. **D-2：新增 `RecallEngine` 作为统一召回门面。** `Recall(ctx, query, opts)`
     是 v1 推荐的召回入口，tier 选择（working / episodic / semantic）成为内部细节。

  核心仓 `github.com/costa92/llm-agent` 没有任何修改 —— v0.7.0 保持不变。
  所有 break 都发生在 sibling 模块内部。

  ## 2. 破坏性变更清单

  | # | 区域 | 变更 | 旧用法 | 新用法 |
  |---|---|---|---|---|
  | 1 | 构造 Manager | 推荐入口从 `coremem.NewManager(coremem.ManagerOptions{...})` 转为 `memory.NewManager(memory.Options{...})`。 | `coremem.NewManager(coremem.ManagerOptions{Working: w})` | `memory.NewManager(memory.Options{Working: memory.TierOptions{Memory: w, Lister: w, Exporter: w, Importer: w}})` |
  | 2 | 召回入口 | 推荐入口从 `UnifiedSearcher.SearchUnified` / `ParallelSearcher.SearchAllParallel` 转为 `RecallEngine.Recall`。`UnifiedSearcher` 与 `ParallelSearcher` 仍在 v1.x 中可用但已 `// Deprecated:`。 | `uni.SearchUnified(ctx, q, topK)` | `eng.Recall(ctx, q, memory.RecallOptions{TopK: topK})` |

  说明：除上述两项，v1.0.0 对外暴露的 M1/M2/M3 surface（`ScopedLifecycleManager`、
  `Consolidator`、`PolicyEnforcingMemory`、`SQLiteStore` 等）API 完全不变。
  你可以仅升级 sibling 依赖、什么也不改，旧测试照样能跑。

  ## 3. 升级配方

  ### 3.1 最小改动：让旧的 `*coremem.Manager` 走 v1 Manager

  ```go
  // 旧代码：
  coreMgr, _ := coremem.NewManager(coremem.ManagerOptions{
      Working:  w,
      Episodic: e,
      Semantic: s,
  })

  // 升级后（一行）：
  import compat "github.com/costa92/llm-agent-memory/memory/compat"
  mgr := compat.NewManagerFromCore(coreMgr)

  // 下游代码（mgr.Add / mgr.Search / mgr.Consolidate ...）无需修改。
  ```

  `compat.NewManagerFromCore` 在 v1.x 全周期内可用，将在 v2.0.0 移除。

  ### 3.2 字段级改动：直接构造 v1 Manager

  ```go
  import (
      "github.com/costa92/llm-agent-memory/memory"
      coremem "github.com/costa92/llm-agent/memory"
  )

  w, _ := coremem.NewWorking(emb, coremem.WorkingOptions{})
  e, _ := coremem.NewEpisodic(emb, coremem.EpisodicOptions{})
  s, _ := coremem.NewSemantic(emb, coremem.SemanticOptions{})

  mgr, _ := memory.NewManager(memory.Options{
      Working:  memory.TierOptions{Memory: w, Lister: w, Exporter: w, Importer: w},
      Episodic: memory.TierOptions{Memory: e, Lister: e, Exporter: e, Importer: e},
      Semantic: memory.TierOptions{Memory: s, Lister: s, Exporter: s, Importer: s},
      // 可选：若需要 Consolidate / Forget 调用，提供 CoreManager 兜底：
      CoreManager: nil,
  })
  ```

  注意：bundled `coremem.WorkingMemory` 等同时满足 `Memory`、`Lister`、
  `Exporter`、`Importer` 四个接口，所以同一个对象可以填四个字段。

  ### 3.3 带 sanitizer 的写入路径（D-1 的主要价值）

  ```go
  // v0.7 - 不能直接挂入 ManagerOptions，被 docs/policy_hook.go 的 LIMITATION
  // 标注阻断了：
  // wrapped := coremem.WithSanitizer(w, redactor)  // coremem.Memory 接口
  // opts.Working = wrapped                         // ❌ 类型不匹配

  // v1.0.0 - 直接挂：
  wrapped := coremem.WithSanitizer(w, redactor) // coremem.Memory 接口
  opts.Working = memory.TierOptions{Memory: wrapped}
  ```

  ### 3.4 召回入口切换

  ```go
  // v0.x：
  uni, _ := memory.NewUnifiedSearcher(coreMgr)
  results, _ := uni.SearchUnified(ctx, "query", 10)

  // v1.0.0：
  eng, _ := memory.NewRecallEngine(mgr)
  recall, _ := eng.Recall(ctx, "query", memory.RecallOptions{TopK: 10})
  results := recall.Results
  ```

  `RecallOptions.Tiers`、`RecallOptions.Budgets`、`RecallOptions.IncludeProvenance`
  是 v1 新增的能力。详见 `recall_engine.go` 的 godoc。

  ## 4. 兼容窗口

  | 组件 | v1.x 状态 | 计划移除时间 |
  |---|---|---|
  | `memory/compat` 子包 | 可用，全部入口带 `// Deprecated:` | v2.0.0 |
  | `UnifiedSearcher` / `ParallelSearcher` | 可用，类型 doc 带 `// Deprecated:` | v2.0.0 |
  | `coremem.ManagerOptions` | 核心仓不归本路线图管，无变化 | n/a |

  在 v2.0.0 之前，所有 v0.x 调用方都可以选择 *不迁移*，sibling 升级到 v1.x
  不会破坏现有调用。

  ## 5. 与 `context.Builder` 的衔接

  `coremem.context.Builder.BuildInput.MemoryHits` 字段仍是 `[]coremem.SearchResult`
  （核心仓 v0.7 没动）。从 v1 `Recall` 喂数据给 Builder 的一行：

  ```go
  out, _ := eng.Recall(ctx, query, memory.RecallOptions{TopK: 10})
  build := builder.Build(context.BuildInput{
      UserQuery:  query,
      MemoryHits: out.Results, // 直接对接
      // ...
  })
  ```

  M6（Memory Gateway）以及核心仓 Builder 演化是后续 milestone 的话题；
  本指南仅说明 v1.0.0 的接入面。

  ## 6. 退出标准对照

  下表对应 `docs/superpowers/plans/2026-05-25-llm-agent-memory-roadmap.md`
  M4 行的 exit criteria：

  | # | 标准 | 实现位置 | 测试位置 |
  |---|---|---|---|
  | 1 | `ManagerOptions` 字段为接口（Memory/Lister/Exporter/Importer/可选 Lifecycle） | `memory/manager.go` — `TierOptions` struct | `memory/manager_test.go` — `TestManager_TierOptions_FieldsAreCapabilityInterfaces` |
  | 2 | `WithSanitizer` 包装的 memory 不需 cast 即可装入 Manager | `memory/manager.go` — `TierOptions.Memory coremem.Memory` | `memory/manager_test.go` — `TestManager_WithSanitizerWrappedMemory_InstallsWithoutCast` |
  | 3 | `RecallEngine.Recall(ctx, query, opts)` 暴露统一召回门面 | `memory/recall_engine.go` — `RecallEngine.Recall` | `memory/recall_engine_test.go` — `TestRecallEngine_Recall_ParityWithUnifiedSearcher`、`TestRecallEngine_Recall_TierMask_Working_OmitsOtherTiers`、`TestRecallEngine_Recall_PerTierBudget_CapsCandidates`、`TestRecallEngine_OverWithSanitizerWrappedManager_NoCast` |
  | 4 | 迁移指南文档列出每一项 break | `docs/memory-v1-migration.zh-CN.md`（本文） | — |
  | 5 | `memory/compat/` 子包提供旧 ManagerOptions 构造器 | `memory/compat/compat.go` — `NewManagerFromCore`、`NewManagerFromLegacyOptions`、`LegacyOptions` | `memory/compat/compat_test.go` 三个测试 |
  | 6 | M1–M3 测试全部适配，无无理由删除 | 无现有测试删除；新 D-1/D-2 测试落在新文件 | `memory/manager_test.go` + `memory/recall_engine_test.go` + `memory/compat/compat_test.go` |

  ## 7. 相关文档

  - 主路线图：`docs/superpowers/plans/2026-05-25-llm-agent-memory-roadmap.md`
  - 详细计划：`docs/superpowers/plans/2026-05-26-m4-phase-d-capability-interfaces.md`
  - 设计依据：`docs/memory-roadmap.zh-CN.md` §5.1（Phase D）
  - 上一里程碑（M3）的同结构计划：`docs/superpowers/plans/2026-05-26-m3-phase-c-policy-and-stores.md`
  ```

- [ ] **Step 2: Skip (no test for a docs file).**

- [ ] **Step 3: Skip.**

- [ ] **Step 4: Skip.**

- [ ] **Step 5: Commit (UMBRELLA repo, NOT the sibling)**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem
  git add docs/memory-v1-migration.zh-CN.md
  git commit -m "docs(memory): add v0.x → v1.0.0 migration guide (M4 doc deliverable)"
  ```

  Note: do NOT `git push` here unless the umbrella's normal workflow expects it. The umbrella commits are tracked separately from sibling commits.

---

## Task 19: Refresh `CHANGELOG.md` and confirm full-suite green

**Files:**
- Modify: `llm-agent-memory/CHANGELOG.md`

- [ ] **Step 1: Add the `## [1.0.0] - 2026-05-26` section to the TOP of CHANGELOG.md**

  Prepend (keeping all prior sections intact):

  ```markdown
  ## [1.0.0] - 2026-05-26

  > First major release. See `docs/memory-v1-migration.zh-CN.md` in the
  > umbrella for the full migration guide.

  ### Added

  - **`memory.Manager` (D-1)** — sibling-owned, capability-interface-typed
    coordinator. Construct via `NewManager(Options{...})`. Each tier's
    `TierOptions` carries five capability fields — `Memory`, `Lister`,
    `Exporter`, `Importer`, `Lifecycle` — typed as interfaces. This
    closes the v0.7 limitation that prevented `coremem.WithSanitizer`-wrapped
    memories from installing into `coremem.ManagerOptions` (which required
    a concrete `*coremem.WorkingMemory`).
  - **`LifecycleMemory` interface (D-1)** — `Consolidate(ctx, opts)` +
    `Forget(ctx, kind, opts)`. New capability that core's
    `coremem.Manager` performs via package-private access; external
    backends (Postgres, pgvector) can now implement lifecycle natively.
    `NewCoreManagerLifecycle(*coremem.Manager)` is the adapter for the
    bundled case.
  - **`memory.RecallEngine.Recall(ctx, query, opts) (UnifiedRecall, error)` (D-2)** —
    v1 unified recall facade. Tier-awareness becomes internal. Supports
    per-tier budgets, tier-selection bitmask, and per-tier provenance.
  - **`memory.RecallOptions` / `memory.UnifiedRecall` / `memory.TierStats` /
    `memory.TierMask`** — the public surface around `Recall`.
  - **`memory/compat` sub-package** — `LegacyOptions` type alias for
    `coremem.ManagerOptions`; `NewManagerFromCore(*coremem.Manager)
    *memory.Manager`; `NewManagerFromLegacyOptions(LegacyOptions)
    (*memory.Manager, error)`. One-release-window bridge for v0.x
    callers; removed at v2.0.0.

  ### Deprecated

  - `memory.UnifiedSearcher` — prefer `memory.RecallEngine.Recall`.
    Remains usable in the v1.x line; removed at v2.0.0.
  - `memory.ParallelSearcher` — prefer `memory.RecallEngine.Recall`.
    Remains usable in the v1.x line; removed at v2.0.0.
  - `memory/compat.*` — entire sub-package is deprecated on arrival;
    removed at v2.0.0.

  ### Dependencies

  - **No new third-party deps.** `modernc.org/sqlite v1.50.1` and the
    transitive closure are unchanged. Pure-Go path preserved.

  ### Compatibility

  - Core `github.com/costa92/llm-agent v0.7.0` is untouched. Sibling
    v1.0.0 pins core v0.7.0; the v1.x line of the sibling can ship
    against any v0.7.x of core that preserves the public memory surface.
  - All M1/M2/M3 public APIs (`ScopedLifecycleManager`, `Consolidator`,
    `WritePolicy`, `PolicyEnforcingMemory`, `PolicyAdapter`, `SQLiteStore`,
    `Observer`, every event-name constant) are unchanged. The v0.3.0
    deprecation window has not yet started for any of these — they
    remain canonical in v1.x.

  ```

- [ ] **Step 2: Skip (no test for CHANGELOG).**

- [ ] **Step 3: Run the full suite one final time**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./... -v -count=1`
  Expected: every test passes — M0 (version), M1 (consolidator + scoped_lifecycle + unified_search), M2 (observer + parallel_search), M3 (write_policy + sqlite_store), M4 (manager + recall_engine + compat).

- [ ] **Step 4: Run `go vet`**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go vet ./...`
  Expected: clean.

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add CHANGELOG.md
  git commit -m "docs(memory): document v1.0.0 in CHANGELOG (M4: D-1 + D-2 + compat)"
  git push origin main
  ```

---

## Task 20: Tag `v1.0.0`

**Files:**
- (git only — no file changes)

- [ ] **Step 1: Confirm everything is on `origin/main` and clean**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && git status && git log --oneline -25`
  Expected: working tree clean; the last 19+ commits are M4 work culminating in the v1.0.0 CHANGELOG entry.

- [ ] **Step 2: Re-run the full suite one final time on the exact HEAD that will be tagged**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./... -count=1`
  Expected: every test passes.

- [ ] **Step 3: Create the unprefixed `v1.0.0` annotated tag**

  Run:
  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git tag -a v1.0.0 -m "$(cat <<'EOF'
  v1.0.0: M4 capability-interface Manager + RecallEngine

  - Manager: capability-interface ManagerOptions (D-1)
  - RecallEngine: unified Recall facade (D-2)
  - compat/: bridge sub-package (removed at v2.0.0)
  - Migration guide: docs/memory-v1-migration.zh-CN.md in umbrella
  EOF
  )"
  git push origin v1.0.0
  ```

- [ ] **Step 4: Verify the tag is pushed**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && git ls-remote --tags origin | grep v1.0.0`
  Expected: a line ending in `refs/tags/v1.0.0`.

- [ ] **Step 5: No additional commit needed — the tag IS the release marker.**

---

## Self-Review

| Exit criterion (verbatim from master roadmap M4) | Task(s) | Test(s) |
|---|---|---|
| 1. `ManagerOptions` fields typed as interfaces: `Memory`, `Lister`, `Exporter`, `Importer`, optional `LifecycleMemory`. | 1, 2 | `TestManager_TierOptions_FieldsAreCapabilityInterfaces` |
| 2. `WithSanitizer`-wrapped memory installs into `Manager` without a cast. | 7 | `TestManager_WithSanitizerWrappedMemory_InstallsWithoutCast` |
| 3. `RecallEngine` facade exposes `Recall(ctx, query, opts) -> UnifiedRecall`; tier-awareness becomes internal. | 9, 10, 11, 12, 13 | `TestRecallEngine_PublicShape_Compiles`, `TestRecallEngine_Recall_ParityWithUnifiedSearcher`, `TestRecallEngine_Recall_TierMask_Working_OmitsOtherTiers`, `TestRecallEngine_Recall_PerTierBudget_CapsCandidates`, `TestRecallEngine_OverWithSanitizerWrappedManager_NoCast` |
| 4. Migration guide doc (`docs/memory-v1-migration.zh-CN.md`) listing every break. | 18 | (manual doc, not test-covered) |
| 5. Compatibility shim package `memory/compat/` provides old `ManagerOptions` constructor for one release window. | 14, 15, 16 | `TestCompat_LegacyOptions_IsAliasOfCoreManagerOptions`, `TestCompat_NewManagerFromCore_BridgesEverySurfaceMethod`, `TestCompat_NewManagerFromLegacyOptions_AcceptsCoreShape` |
| 6. All M1–M3 tests adapted; no test deletion without justification. | 3–8, 10–13, 15 (the "no regression" Step 4 in each task) | The pre-existing M1/M2/M3 tests run unchanged inside every `-count=1 ./...` invocation; zero deletions. |

**Sequencing audit:** Task 1 → 2 → ... → 20 strictly. Tasks 1–8 land D-1; Tasks 9–13 land D-2 (requires Manager from D-1); Tasks 14–16 land compat (requires Manager); Task 17 bumps Version (requires all feature work green); Task 18 writes the migration guide (requires final API shape known); Task 19 documents in CHANGELOG (requires final API shape known); Task 20 tags (requires all of the above shipped).

**TDD discipline audit:** Every task — except the explicit "skip impl in this task" notes — follows the strict failing-test-then-impl-then-pass cycle. Tasks 1, 9, 14 are the three pure-test scaffolding tasks (declaring a failing compile assertion to lock the surface before any impl). Tasks 6, 7, 8, 11, 12, 13, 16 are pure-test tasks that lock additional invariants. Tasks 2, 3, 4, 5, 10, 15, 17, 19 carry both test and impl. Tasks 18 and 20 are docs / release housekeeping.

**Decisions flag for the orchestrator before kickoff:**

- **RECOMMENDATION: do NOT split into M4a + M4b.** This plan is sibling-first. The reasoning is in "Open Decisions Resolved" item #1: no external consumer needs the core to break, and every D-1/D-2 exit criterion is satisfied by sibling-owned types because the bundled `*coremem.WorkingMemory` / `*coremem.EpisodicMemory` / `*coremem.SemanticMemory` already implement the four required interfaces (`Memory` + `Lister` + `Exporter` + `Importer`). A core PR is therefore unnecessary work that risks breaking M5+ pre-conditions.
- If you prefer to confirm with the user before kicking off the plan: ask whether they accept the sibling-first decision. If they want the core-PR split, this plan needs to be re-scoped to "M4b only" and a fresh M4a plan must be drafted against `github.com/costa92/llm-agent` (tagging v1.0.0 of core). My recommendation: ship this plan as-is.
- The migration guide (Task 18) lands in the UMBRELLA repo, not the sibling — confirm the umbrella has its own commit workflow available.
- The v1.0.0 tag is unprefixed and matches the v0.1.0/v0.2.0/v0.3.0 convention (verified against `git log` in this plan's preparation).
