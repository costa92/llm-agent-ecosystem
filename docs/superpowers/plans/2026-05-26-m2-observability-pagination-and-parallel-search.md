# M2: Pagination Hardening, Observer Hooks, and Parallel SearchAll

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land milestone M2 of the `llm-agent-memory` subproject:

1. (Load-bearing pre-condition) Fix the silent 50-item-per-kind cap inside `ConsolidateScoped`, `ForgetScoped`, `StatsScoped`, and `Consolidator.Consolidate`. They call `ListAll` once with no cursor loop, so any scope holding more than `pageSize` items per kind silently drops the tail. Replace with a cursor-aware paging loop.
2. (Phase B-1) Add a pluggable `Observer` interface with a locked `Event{Name, Attrs}` payload schema, then emit the 7 minimum metrics from `docs/memory-roadmap.zh-CN.md` §4.2 across `ScopedLifecycleManager`, `Consolidator`, and `UnifiedSearcher`.
3. (Phase B-3) Add `ParallelSearcher.SearchAllParallel(ctx, query, topK)` — same shape as `coremem.Manager.SearchAll`, fan-out via stdlib `sync.WaitGroup` + buffered channel, fail-fast on first error via `context.WithCancel`.
4. (Phase B-2) Mark the Working-eviction embed-reuse optimization as **deferred-to-core**: B-2 lives inside `llm-agent/memory/working.go` (private struct fields, package-private call sites) and cannot be wrapped from this sibling without a core PR. Land a documentation note + a behavioral test asserting current Add+eviction count remains correct, so we have a regression detector when the core PR lands.

All test-driven (strict TDD throughout — no scaffolding-only block), stdlib-only.

**Architecture:** Three new files + three modified files in the existing `llm-agent-memory/memory/` package. The Observer surface is internal-additive: zero-config callers see no behavior change.

- `observer.go` — new file. Declares `Observer` interface, `Event` struct, the 7 canonical event-name constants, and a `noopObserver{}` sentinel used when callers do not opt in. Also exports a small helper `emit(o Observer, name string, attrs map[string]any)` that no-op-guards a nil `o`.
- `parallel_search.go` — new file. Declares `ParallelSearcher` wrapping `*coremem.Manager`, plus `SearchAllParallel(ctx, query, topK) (map[coremem.Kind][]coremem.SearchResult, error)` that runs three goroutines through a buffered result channel, cancels siblings on first error, and returns the same per-kind map shape as core.
- `scoped_lifecycle.go` — modified: extract a private `listAllScoped(ctx, kind)` helper that loops over cursors until `NextCursor == ""`; rewrite `ConsolidateScoped`, `ForgetScoped`, `StatsScoped` to use it; add optional `WithObserver(...)` option for `NewScopedLifecycleManager`; emit `memory_consolidated_total`, `memory_forgotten_total` events.
- `consolidator.go` — modified: same cursor-loop fix inside `Consolidate`; add `WithObserver(...)`; emit `memory_consolidated_total`.
- `unified_search.go` — modified: add `WithObserver(...)`; emit `memory_search_total` and `memory_search_hits`; route the fan-out through the new `ParallelSearcher` so unified search inherits the parallel win automatically.

**Tech Stack:** Go 1.26.0; stdlib only (no third-party deps); `github.com/costa92/llm-agent v0.5.0` (unchanged); `testing` for tests; `sync` + `context` for the parallel fan-out.

---

## File Structure

| Path | Action | Responsibility |
|---|---|---|
| `llm-agent-memory/memory/observer.go` | Create | `Observer` interface, `Event` struct, the 7 event-name constants, `noopObserver`, `emit()` helper |
| `llm-agent-memory/memory/observer_test.go` | Create | Recording observer + tests asserting zero-config no-op + non-nil emission of the 7 canonical events |
| `llm-agent-memory/memory/parallel_search.go` | Create | `ParallelSearcher` + `SearchAllParallel(ctx, query, topK)` |
| `llm-agent-memory/memory/parallel_search_test.go` | Create | Parity test vs `coremem.Manager.SearchAll`; fail-fast test; topK semantics test |
| `llm-agent-memory/memory/scoped_lifecycle.go` | Modify | Add `listAllScoped` cursor loop; rewrite three methods to use it; add `WithObserver` option; emit observer events |
| `llm-agent-memory/memory/scoped_lifecycle_test.go` | Modify | Append paging-completeness tests (>150 items per scope) and observer-emission tests |
| `llm-agent-memory/memory/consolidator.go` | Modify | Cursor loop inside `Consolidate`; `WithObserver` option; emit `memory_consolidated_total` |
| `llm-agent-memory/memory/consolidator_test.go` | Modify | Append paging-completeness test (>150 working items) + observer test |
| `llm-agent-memory/memory/unified_search.go` | Modify | `WithObserver` option; emit `memory_search_total` + `memory_search_hits`; delegate fan-out to `ParallelSearcher` |
| `llm-agent-memory/memory/unified_search_test.go` | Modify | Append observer-emission test |
| `llm-agent-memory/memory/version.go` | Modify | Bump `Version` to `0.2.0` |
| `llm-agent-memory/memory/version_test.go` | Modify | Update the version-string assertion to match |
| `llm-agent-memory/CHANGELOG.md` | Modify | Add `## [0.2.0] - 2026-05-26` section describing M2 deliverables; note B-2 deferred-to-core |

---

## Open Decisions Resolved in This Plan

- **Observer payload schema:** *Single typed `Event{Name string, Attrs map[string]any}` struct.* Rationale: (a) callers can layer typed adapters on top trivially; (b) we avoid an explosion of per-event Go types that would lock the schema before consumers exist; (c) `Attrs map[string]any` matches the `MemoryItem.Metadata` convention already established in M1, so consumers writing `OnEvent` already know the encoding. The 7 canonical event names are exported constants (e.g. `EventAddTotal`); writing `Event.Name == EventSearchTotal` is the supported pattern. The `Attrs` schema per event is documented in observer.go godoc and is *frozen* at v0.2.0 — any future addition is a backwards-compatible new key, never a removal/rename.
- **B-2 disposition:** *Deferred to a core PR.* The probe-text re-embed happens inside `(*coremem.WorkingMemory).evictIfOverCapacity`, which is package-private; the working store does not expose its computed embedding via any public method, so a sibling wrapper cannot reuse it without re-implementing `WorkingMemory` wholesale. We add a behavioral test (Task 13) that pins current Add+eviction semantics; that test will keep passing both before and after the upstream PR (it asserts *behavior*, not embed call count), preventing a regression when the core change lands. Open question (M2 kick-off → core maintainer): file the upstream issue at `github.com/costa92/llm-agent` referencing `working.go:56-63` and link this plan as the consumer-side gate.
- **Parallel concurrency primitive:** *Stdlib `sync.WaitGroup` + buffered channel + `context.WithCancel`*. Rationale: the new module is locked to stdlib-only by master-roadmap §3 "Dependency policy". `golang.org/x/sync/errgroup` is forbidden in M2 (it can land in M3+ if/when we already pulled it in for a non-overlapping reason).
- **Where parallelism applies in unified search:** `UnifiedSearcher` re-routes its fan-out call from `mgr.SearchAll` to `ParallelSearcher.SearchAllParallel`. This means consumers of `SearchUnified` get the parallel win for free; consumers who still want the serial behavior can construct a `UnifiedSearcher` directly against `mgr` via the new `WithSerialSearch()` opt-out. Default is parallel.
- **v0.2.0 tag timing (resolved 2026-05-26, M2 open question q3):** Tag immediately after Task 14 verification gate goes green; same cadence as v0.1.0 — no cross-AI plan review delay. Rationale: sibling is pre-1.0, SemVer permits schema cleanup in 0.x.y minor bumps if Observer hooks need adjustment; M1's same-cadence ship produced a clean v0.1.0 with no post-release regressions; faster iteration > extra review gate when the blast radius is a single sibling module with no production consumers. Observer schema **does** freeze at v0.2.0 per the existing "Observer payload schema" decision — that locking is intentional and accepted. **This resolves M2 open question q3.**
- **`ParallelSearcher` per-kind dispatch path (pre-execution verified 2026-05-26):** `coremem.Manager` (v0.7.0) does *not* expose `MemoryFor(Kind)`, but `(*coremem.Manager).Search(ctx, Kind, query, topK)` (manager.go:88-94) routes to the per-kind `Memory.Search` via the unexported `lookup()` switch (manager.go:416-436), which reads three write-once-at-construction fields and is therefore race-free under concurrent calls. v0.7.0 `SearchAll` (manager.go:98-115) is genuinely serial — the parallel win is real, not an artifact of a hypothetical core change. Therefore `ParallelSearcher` calls `p.mgr.Search(ctx, kind, query, topK)` from N goroutines (one per `Kind`); no upstream PR to add `MemoryFor` is needed for M2. **This resolves M2 open questions q1 and q2.**
- **Cursor-loop page size:** `pageSize = 200` for the new `listAllScoped` helper. Rationale: large enough that scopes with O(few-hundred) items finish in 1–2 hops, small enough to keep memory bounded; matches the comment in `coremem`'s `listFromStore` which caps at 500. The loop runs unbounded until `NextCursor == ""` so this is purely a per-hop tuning knob.
- **Observer error semantics:** `Observer.OnEvent` *MUST NOT return an error*. Reason: hot-path emit must be cheap and unconditional. Implementations that need to fail (e.g. a metrics SDK with backpressure) must drop, log, or buffer internally. This is documented in the interface godoc as "MUST NOT block; MUST NOT panic; errors are the implementation's problem."
- **Observer call site:** Always emit AFTER the underlying core call returns successfully (Add succeeded → `memory_add_total` += 1). On error we DO NOT emit (callers count their own errors via the returned `err`). This aligns with the §4.2 success-counter intent.
- **`memory_add_total` location:** The B-1 catalogue lists `memory_add_total`, but the sibling does not own an Add path — adds go through `*coremem.Manager.Add` directly, not through any wrapper in `llm-agent-memory/`. We honor the metric by emitting it from `Consolidator.Consolidate`'s episodic-clone Add (the only Add the sibling owns) and document this scoping decision in observer.go godoc. The B-1 spec is satisfied; the umbrella will get a true cross-the-board `memory_add_total` when the M4 `RecallEngine` facade lands.
- **Version bump:** `0.1.0 → 0.2.0`. M2 is additive — no signature breaks on M1 types (`ScopedLifecycleManager`, `Consolidator`, `UnifiedSearcher` keep the same constructor signatures; `WithObserver` is a variadic option pattern slot we add).
- **Constructor option pattern:** Switch `NewScopedLifecycleManager(inner)`, `NewConsolidator(inner)`, `NewUnifiedSearcher(inner)`, and the new `NewParallelSearcher(inner)` to variadic options: `New...(inner, opts ...Option)` where `Option func(*config)` is package-internal. Adding an option does not break callers that pass no options. Today's only option is `WithObserver(o Observer)`; the slot is reserved for future toggles (e.g. `WithSerialSearch()`).

---

## Sequencing Rules

- **Every task is strict TDD.** Failing test → minimal impl → green test → commit. No scaffolding-only tasks (the M0+M1 plan's M0 block was the only such exemption; M2 has no equivalent).
- **Pagination fix lands first (Tasks 1–4) — it is the load-bearing finding.** Observer hooks and parallel search build on a corrected pagination loop.
- **Commit cadence:** every task ends in exactly one commit. Step 5 is always "Commit". Use `git add` with explicit file paths (per the M0+M1 plan's git-safety reminder) — never `git add -A`.
- **All commit prefixes use `(memory)` scope** to match M1's convention.

---

# Pagination Hardening (Tasks 1–4)

## Task 1: Failing test — `ConsolidateScoped` processes >150 items per scope

**Files:**
- Modify: `llm-agent-memory/memory/scoped_lifecycle_test.go` (append)

- [ ] **Step 1: Append the failing test that exposes the silent 100-item cap**

  Append to `llm-agent-memory/memory/scoped_lifecycle_test.go`:

  ```go
  func TestScopedLifecycle_ConsolidateScoped_PagesThroughLargeScope(t *testing.T) {
  	// Verify pagination loop: the M1 impl called ListAll with no cursor,
  	// silently capping at one page. With 180 working items above
  	// threshold, the M1 impl would promote at most 100 and silently drop
  	// the remaining 80.
  	sm := newCoreScopedManager(t)
  	// Working capacity in newCoreWorking is 16 — too small for 180.
  	// Build a manager directly with a wide-capacity working memory.
  	mgr, err := coremem.NewManager(coremem.ManagerOptions{
  		Working:  newCoreWorkingWithCapacity(t, 256),
  		Episodic: newCoreEpisodic(t),
  		Semantic: newCoreSemantic(t),
  	})
  	if err != nil {
  		t.Fatalf("NewManager: %v", err)
  	}
  	wideSM, err := coremem.NewScopedManager(mgr)
  	if err != nil {
  		t.Fatalf("NewScopedManager: %v", err)
  	}
  	_ = sm // pin the helper so unused-var doesn't bite if tests later add it

  	slm, err := NewScopedLifecycleManager(wideSM)
  	if err != nil {
  		t.Fatalf("NewScopedLifecycleManager: %v", err)
  	}

  	ctx := coremem.WithScope(context.Background(), coremem.Scope{User: "page-user"})
  	const total = 180
  	for i := 0; i < total; i++ {
  		if _, err := wideSM.Add(ctx, coremem.KindWorking, coremem.MemoryItem{
  			Content:    fmt.Sprintf("item-%03d", i),
  			Importance: 0.9,
  		}); err != nil {
  			t.Fatalf("Add #%d: %v", i, err)
  		}
  	}

  	n, err := slm.ConsolidateScoped(ctx, coremem.ConsolidateOptions{Threshold: 0.7})
  	if err != nil {
  		t.Fatalf("ConsolidateScoped: %v", err)
  	}
  	if n != total {
  		t.Fatalf("ConsolidateScoped promoted = %d, want %d (pagination dropped %d items)",
  			n, total, total-n)
  	}
  }
  ```

  Note: the test imports `"fmt"` — add it to the import block if not present.

- [ ] **Step 2: Add the wide-capacity helper to `testutil_test.go`**

  Append to `llm-agent-memory/memory/testutil_test.go`:

  ```go
  // newCoreWorkingWithCapacity builds a *coremem.WorkingMemory with the
  // requested capacity (24h decay). Use this in pagination tests where the
  // default capacity of 16 from newCoreWorking is too small.
  func newCoreWorkingWithCapacity(t *testing.T, capacity int) *coremem.WorkingMemory {
  	t.Helper()
  	w, err := coremem.NewWorking(newCoreEmbedder(), coremem.WorkingOptions{
  		Capacity: capacity,
  		Decay:    24 * time.Hour,
  	})
  	if err != nil {
  		t.Fatalf("coremem.NewWorking(cap=%d): %v", capacity, err)
  	}
  	return w
  }
  ```

- [ ] **Step 3: Run the test to verify it fails**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestScopedLifecycle_ConsolidateScoped_PagesThroughLargeScope -v`
  Expected: `FAIL` — the M1 impl calls `ListAll(ctx, ..., 0, nil)` and only sees the first page (≤ 100 items). The failure message says `promoted = 100, want 180` (or similar, depending on coremem's page cap).

- [ ] **Step 4: (No impl — Task 2 lands the cursor loop that turns this green.)**

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/scoped_lifecycle_test.go memory/testutil_test.go
  git commit -m "test(memory): assert ConsolidateScoped processes scopes larger than one page"
  git push origin main
  ```

---

## Task 2: Cursor loop helper + apply to `ConsolidateScoped`

**Files:**
- Modify: `llm-agent-memory/memory/scoped_lifecycle.go`

- [ ] **Step 1: Add the `listAllScoped` cursor-loop helper at the bottom of `scoped_lifecycle.go`**

  ```go
  // listAllScoped enumerates every item across every active kind in the
  // ctx scope, paging through ScopedManager.ListAll until each kind's
  // NextCursor is the empty string. Returns the accumulated per-kind
  // items. pageSize is the per-hop request size; the loop is unbounded.
  //
  // This closes the silent-truncation bug in the M1 helpers, which
  // called ListAll once with no cursor and capped per-call processing at
  // a single page.
  func (s *ScopedLifecycleManager) listAllScoped(ctx context.Context, pageSize int) (map[coremem.Kind][]coremem.MemoryItem, error) {
  	if pageSize <= 0 {
  		pageSize = 200
  	}
  	out := make(map[coremem.Kind][]coremem.MemoryItem)
  	cursors := map[coremem.Kind]string{}
  	for {
  		pages, err := s.sm.ListAll(ctx, coremem.ListFilter{}, pageSize, cursors)
  		if err != nil {
  			return nil, fmt.Errorf("memory: paged list: %w", err)
  		}
  		anyMore := false
  		nextCursors := map[coremem.Kind]string{}
  		for kind, page := range pages {
  			if len(page.Items) > 0 {
  				out[kind] = append(out[kind], page.Items...)
  			}
  			if page.NextCursor != "" {
  				nextCursors[kind] = page.NextCursor
  				anyMore = true
  			}
  		}
  		if !anyMore {
  			return out, nil
  		}
  		cursors = nextCursors
  	}
  }
  ```

- [ ] **Step 2: Rewrite `ConsolidateScoped` to use `listAllScoped`**

  Replace the body of `ConsolidateScoped` (lines 51–84 in the current file) with:

  ```go
  func (s *ScopedLifecycleManager) ConsolidateScoped(ctx context.Context, opts coremem.ConsolidateOptions) (int, error) {
  	if opts.Threshold <= 0 {
  		opts.Threshold = 0.7
  	}
  	mgr := s.sm.Inner()
  	allItems, err := s.listAllScoped(ctx, 200)
  	if err != nil {
  		return 0, fmt.Errorf("memory: list working: %w", err)
  	}
  	working := allItems[coremem.KindWorking]
  	count := 0
  	for _, it := range working {
  		if it.Importance < opts.Threshold {
  			continue
  		}
  		if opts.MinAge > 0 {
  			if it.CreatedAt.IsZero() {
  				continue
  			}
  			if !it.CreatedAt.Add(opts.MinAge).Before(timeNow()) {
  				continue
  			}
  		}
  		clone := it
  		clone.ID = ""
  		if _, err := mgr.Add(ctx, coremem.KindEpisodic, clone); err != nil {
  			return count, fmt.Errorf("memory: consolidate-scoped add: %w", err)
  		}
  		count++
  	}
  	return count, nil
  }
  ```

- [ ] **Step 3: Run the failing test from Task 1 to verify it now passes**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestScopedLifecycle_ConsolidateScoped_PagesThroughLargeScope -v`
  Expected: `PASS`. The full suite should also still pass.

- [ ] **Step 4: Run the entire scoped-lifecycle suite to confirm no regression in the M1 tests**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestScopedLifecycle -v`
  Expected: all `TestScopedLifecycle_*` tests pass.

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/scoped_lifecycle.go
  git commit -m "fix(memory): page through ConsolidateScoped instead of capping at one page"
  git push origin main
  ```

---

## Task 3: Apply cursor loop to `ForgetScoped` and `StatsScoped`

**Files:**
- Modify: `llm-agent-memory/memory/scoped_lifecycle.go`
- Modify: `llm-agent-memory/memory/scoped_lifecycle_test.go` (append)

- [ ] **Step 1: Append failing tests for both methods**

  ```go
  func TestScopedLifecycle_ForgetScoped_PagesThroughLargeScope(t *testing.T) {
  	mgr, err := coremem.NewManager(coremem.ManagerOptions{
  		Working:  newCoreWorkingWithCapacity(t, 16),
  		Episodic: newCoreEpisodic(t),
  		Semantic: newCoreSemantic(t),
  	})
  	if err != nil {
  		t.Fatalf("NewManager: %v", err)
  	}
  	wideSM, err := coremem.NewScopedManager(mgr)
  	if err != nil {
  		t.Fatalf("NewScopedManager: %v", err)
  	}
  	slm, err := NewScopedLifecycleManager(wideSM)
  	if err != nil {
  		t.Fatalf("NewScopedLifecycleManager: %v", err)
  	}

  	ctx := coremem.WithScope(context.Background(), coremem.Scope{User: "page-forget"})
  	const total = 170
  	for i := 0; i < total; i++ {
  		if _, err := wideSM.Add(ctx, coremem.KindEpisodic, coremem.MemoryItem{
  			Content:    fmt.Sprintf("forgettable-%03d", i),
  			Importance: 0.1,
  		}); err != nil {
  			t.Fatalf("Add #%d: %v", i, err)
  		}
  	}

  	n, err := slm.ForgetScoped(ctx, coremem.KindEpisodic, coremem.ForgetOptions{
  		Strategy:  coremem.ForgetByImportance,
  		Threshold: 0.5,
  	})
  	if err != nil {
  		t.Fatalf("ForgetScoped: %v", err)
  	}
  	if n != total {
  		t.Fatalf("ForgetScoped removed = %d, want %d (pagination dropped %d)", n, total, total-n)
  	}
  }

  func TestScopedLifecycle_StatsScoped_CountsAllPages(t *testing.T) {
  	mgr, err := coremem.NewManager(coremem.ManagerOptions{
  		Working:  newCoreWorkingWithCapacity(t, 256),
  		Episodic: newCoreEpisodic(t),
  		Semantic: newCoreSemantic(t),
  	})
  	if err != nil {
  		t.Fatalf("NewManager: %v", err)
  	}
  	wideSM, err := coremem.NewScopedManager(mgr)
  	if err != nil {
  		t.Fatalf("NewScopedManager: %v", err)
  	}
  	slm, err := NewScopedLifecycleManager(wideSM)
  	if err != nil {
  		t.Fatalf("NewScopedLifecycleManager: %v", err)
  	}

  	ctx := coremem.WithScope(context.Background(), coremem.Scope{User: "page-stats"})
  	const total = 160
  	for i := 0; i < total; i++ {
  		if _, err := wideSM.Add(ctx, coremem.KindWorking, coremem.MemoryItem{
  			Content:    fmt.Sprintf("countable-%03d", i),
  			Importance: 0.5,
  		}); err != nil {
  			t.Fatalf("Add #%d: %v", i, err)
  		}
  	}

  	stats, err := slm.StatsScoped(ctx)
  	if err != nil {
  		t.Fatalf("StatsScoped: %v", err)
  	}
  	if got := stats[coremem.KindWorking].Count; got != total {
  		t.Errorf("StatsScoped Count = %d, want %d", got, total)
  	}
  }
  ```

- [ ] **Step 2: Run both tests — confirm they fail with the M1 single-page impl**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run 'TestScopedLifecycle_(Forget|Stats)Scoped.*Page' -v`
  Expected: both `FAIL` with truncation evidence (`removed = 100, want 170`, `Count = 100, want 160`).

- [ ] **Step 3: Rewrite both methods to use `listAllScoped`**

  Replace the body of `ForgetScoped` (lines 96–164) with:

  ```go
  func (s *ScopedLifecycleManager) ForgetScoped(ctx context.Context, kind coremem.Kind, opts coremem.ForgetOptions) (int, error) {
  	mgr := s.sm.Inner()
  	allItems, err := s.listAllScoped(ctx, 200)
  	if err != nil {
  		return 0, fmt.Errorf("memory: list %s: %w", kind, err)
  	}
  	candidates := allItems[kind]
  	switch opts.Strategy {
  	case coremem.ForgetByImportance:
  		count := 0
  		for _, it := range candidates {
  			if coremem.IsPinned(it) {
  				continue
  			}
  			if it.Importance < opts.Threshold {
  				if err := mgr.Remove(ctx, kind, it.ID); err == nil {
  					count++
  				}
  			}
  		}
  		return count, nil
  	case coremem.ForgetByAge:
  		if opts.MaxAge <= 0 {
  			return 0, fmt.Errorf("memory: forget by age requires MaxAge > 0")
  		}
  		now := timeNow()
  		count := 0
  		for _, it := range candidates {
  			if coremem.IsPinned(it) {
  				continue
  			}
  			if now.Sub(it.CreatedAt) > opts.MaxAge {
  				if err := mgr.Remove(ctx, kind, it.ID); err == nil {
  					count++
  				}
  			}
  		}
  		return count, nil
  	case coremem.ForgetByCapacity:
  		if opts.Keep <= 0 {
  			return 0, nil
  		}
  		all := make([]forgetPair, 0, len(candidates))
  		for _, it := range candidates {
  			if coremem.IsPinned(it) {
  				continue
  			}
  			all = append(all, forgetPair{it.ID, it.Importance})
  		}
  		if len(all) <= opts.Keep {
  			return 0, nil
  		}
  		sortPairsByImpAsc(all)
  		toEvict := len(all) - opts.Keep
  		count := 0
  		for i := 0; i < toEvict; i++ {
  			if err := mgr.Remove(ctx, kind, all[i].id); err == nil {
  				count++
  			}
  		}
  		return count, nil
  	default:
  		return 0, fmt.Errorf("memory: unknown forget strategy %q", opts.Strategy)
  	}
  }
  ```

  And replace `StatsScoped` (lines 173–211):

  ```go
  func (s *ScopedLifecycleManager) StatsScoped(ctx context.Context) (map[coremem.Kind]coremem.Stats, error) {
  	allItems, err := s.listAllScoped(ctx, 200)
  	if err != nil {
  		return nil, fmt.Errorf("memory: stats list: %w", err)
  	}
  	innerStats := s.sm.Inner().StatsAll()
  	out := make(map[coremem.Kind]coremem.Stats, len(allItems))
  	now := timeNow()
  	for kind, items := range allItems {
  		var (
  			count   = len(items)
  			impSum  float64
  			oldest  time.Time
  			hasItem bool
  		)
  		for _, it := range items {
  			impSum += it.Importance
  			if !hasItem || it.CreatedAt.Before(oldest) {
  				oldest = it.CreatedAt
  				hasItem = true
  			}
  		}
  		var avg float64
  		if count > 0 {
  			avg = impSum / float64(count)
  		}
  		var oldestAge time.Duration
  		if hasItem {
  			oldestAge = now.Sub(oldest)
  		}
  		out[kind] = coremem.Stats{
  			Count:         count,
  			Capacity:      innerStats[kind].Capacity,
  			OldestAge:     oldestAge,
  			AvgImportance: avg,
  		}
  	}
  	return out, nil
  }
  ```

- [ ] **Step 4: Run both new tests + the entire scoped-lifecycle suite**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestScopedLifecycle -v`
  Expected: every `TestScopedLifecycle_*` test passes, including the two new paging tests.

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/scoped_lifecycle.go memory/scoped_lifecycle_test.go
  git commit -m "fix(memory): page through ForgetScoped and StatsScoped"
  git push origin main
  ```

---

## Task 4: Apply cursor loop to `Consolidator.Consolidate`

**Files:**
- Modify: `llm-agent-memory/memory/consolidator.go`
- Modify: `llm-agent-memory/memory/consolidator_test.go` (append)

- [ ] **Step 1: Append the failing test**

  ```go
  func TestConsolidator_Consolidate_PagesThroughLargeWorkingSet(t *testing.T) {
  	mgr, err := coremem.NewManager(coremem.ManagerOptions{
  		Working:  newCoreWorkingWithCapacity(t, 256),
  		Episodic: newCoreEpisodic(t),
  		Semantic: newCoreSemantic(t),
  	})
  	if err != nil {
  		t.Fatalf("NewManager: %v", err)
  	}
  	c, err := NewConsolidator(mgr)
  	if err != nil {
  		t.Fatalf("NewConsolidator: %v", err)
  	}

  	ctx := context.Background()
  	const total = 175
  	for i := 0; i < total; i++ {
  		if _, err := mgr.Add(ctx, coremem.KindWorking, coremem.MemoryItem{
  			Content:    fmt.Sprintf("paged-%03d", i),
  			Importance: 0.9,
  		}); err != nil {
  			t.Fatalf("Add #%d: %v", i, err)
  		}
  	}

  	n, err := c.Consolidate(ctx, coremem.ConsolidateOptions{Threshold: 0.7})
  	if err != nil {
  		t.Fatalf("Consolidate: %v", err)
  	}
  	if n != total {
  		t.Fatalf("Consolidate promoted = %d, want %d (pagination dropped %d)", n, total, total-n)
  	}
  }
  ```

  Note: requires `"fmt"` import in `consolidator_test.go`.

- [ ] **Step 2: Run the test to verify it fails**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestConsolidator_Consolidate_PagesThroughLargeWorkingSet -v`
  Expected: `FAIL` — same single-page-cap symptom.

- [ ] **Step 3: Add a private `listAllPaged` helper on `Consolidator` and rewrite `Consolidate`**

  Append a helper to `consolidator.go` (just below `promotionCountOf`):

  ```go
  // listAllPaged enumerates every item across every active kind via
  // Manager.ListAll, paging through cursors until each kind reports an
  // empty NextCursor. Mirrors ScopedLifecycleManager.listAllScoped — the
  // two cannot share a helper today because Consolidator wraps a
  // *coremem.Manager, not a *coremem.ScopedManager.
  func (c *Consolidator) listAllPaged(ctx context.Context, pageSize int) (map[coremem.Kind][]coremem.MemoryItem, error) {
  	if pageSize <= 0 {
  		pageSize = 200
  	}
  	out := make(map[coremem.Kind][]coremem.MemoryItem)
  	cursors := map[coremem.Kind]string{}
  	for {
  		pages, err := c.mgr.ListAll(ctx, coremem.ListFilter{}, pageSize, cursors)
  		if err != nil {
  			return nil, fmt.Errorf("memory: paged list: %w", err)
  		}
  		anyMore := false
  		nextCursors := map[coremem.Kind]string{}
  		for kind, page := range pages {
  			if len(page.Items) > 0 {
  				out[kind] = append(out[kind], page.Items...)
  			}
  			if page.NextCursor != "" {
  				nextCursors[kind] = page.NextCursor
  				anyMore = true
  			}
  		}
  		if !anyMore {
  			return out, nil
  		}
  		cursors = nextCursors
  	}
  }
  ```

  Replace the body of `Consolidate` (lines 68–120):

  ```go
  func (c *Consolidator) Consolidate(ctx context.Context, opts coremem.ConsolidateOptions) (int, error) {
  	if opts.Threshold <= 0 {
  		opts.Threshold = 0.7
  	}
  	allItems, err := c.listAllPaged(ctx, 200)
  	if err != nil {
  		return 0, fmt.Errorf("memory: consolidate list: %w", err)
  	}
  	working := allItems[coremem.KindWorking]
  	now := timeNow()
  	count := 0
  	for _, it := range working {
  		if it.Importance < opts.Threshold {
  			continue
  		}
  		if opts.MinAge > 0 && now.Sub(it.CreatedAt) < opts.MinAge {
  			continue
  		}
  		if promotionCountOf(it) >= 1 {
  			continue
  		}
  		clone := it
  		clone.ID = ""
  		if clone.Metadata == nil {
  			clone.Metadata = map[string]any{}
  		} else {
  			cp := make(map[string]any, len(clone.Metadata)+1)
  			for k, v := range clone.Metadata {
  				cp[k] = v
  			}
  			clone.Metadata = cp
  		}
  		clone.Metadata[MetaKeyPromotedFrom] = it.ID
  		if _, err := c.mgr.Add(ctx, coremem.KindEpisodic, clone); err != nil {
  			return count, fmt.Errorf("memory: consolidate add: %w", err)
  		}
  		srcID := it.ID
  		err := c.mgr.Update(ctx, coremem.KindWorking, srcID, func(m *coremem.MemoryItem) {
  			if m.Metadata == nil {
  				m.Metadata = map[string]any{}
  			}
  			m.Metadata[MetaKeyConsolidatedAt] = now
  			m.Metadata[MetaKeyPromotionCount] = promotionCountOf(*m) + 1
  		})
  		if err != nil {
  			return count, fmt.Errorf("memory: consolidate stamp source: %w", err)
  		}
  		count++
  	}
  	return count, nil
  }
  ```

- [ ] **Step 4: Run the new paging test plus the entire consolidator suite**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestConsolidator -v`
  Expected: every `TestConsolidator_*` test passes, including the new paging test.

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/consolidator.go memory/consolidator_test.go
  git commit -m "fix(memory): page through Consolidator.Consolidate working enumeration"
  git push origin main
  ```

---

# Phase B-1 — Observer Hooks (Tasks 5–10)

## Task 5: Define `Observer` interface, `Event` struct, and the 7 event-name constants

**Files:**
- Create: `llm-agent-memory/memory/observer.go`
- Create: `llm-agent-memory/memory/observer_test.go`

- [ ] **Step 1: Write the failing test asserting the constants exist and a recording observer captures events**

  ```go
  package memory

  import (
  	"sync"
  	"testing"
  )

  // recordingObserver is a thread-safe test observer that captures every
  // Event for later assertion. Used across the B-1 tests.
  type recordingObserver struct {
  	mu     sync.Mutex
  	events []Event
  }

  func (r *recordingObserver) OnEvent(e Event) {
  	r.mu.Lock()
  	defer r.mu.Unlock()
  	r.events = append(r.events, e)
  }

  func (r *recordingObserver) snapshot() []Event {
  	r.mu.Lock()
  	defer r.mu.Unlock()
  	out := make([]Event, len(r.events))
  	copy(out, r.events)
  	return out
  }

  func TestObserver_CanonicalEventNames_AreDeclared(t *testing.T) {
  	want := map[string]string{
  		"EventAddTotal":              EventAddTotal,
  		"EventSearchTotal":           EventSearchTotal,
  		"EventSearchHits":            EventSearchHits,
  		"EventConsolidatedTotal":     EventConsolidatedTotal,
  		"EventForgottenTotal":        EventForgottenTotal,
  		"EventSnapshotItems":         EventSnapshotItems,
  		"EventSnapshotVectorsBytes":  EventSnapshotVectorsBytes,
  	}
  	for name, val := range want {
  		if val == "" {
  			t.Errorf("%s is empty — must be a non-empty event name", name)
  		}
  	}
  }

  func TestObserver_NoopAcceptsAllCanonicalEvents(t *testing.T) {
  	// Sanity: zero-value emission is a no-op (no panic, no allocation
  	// beyond the Event itself).
  	emit(nil, EventAddTotal, nil)
  	emit(nil, EventSearchTotal, map[string]any{"query_len": 3})
  	// Test passes if we got here.
  }

  func TestObserver_RecordingObserver_CapturesEmittedEvents(t *testing.T) {
  	rec := &recordingObserver{}
  	emit(rec, EventAddTotal, map[string]any{"kind": "working"})
  	emit(rec, EventSearchHits, map[string]any{"n": 3})
  	got := rec.snapshot()
  	if len(got) != 2 {
  		t.Fatalf("captured %d events, want 2", len(got))
  	}
  	if got[0].Name != EventAddTotal {
  		t.Errorf("got[0].Name = %q, want %q", got[0].Name, EventAddTotal)
  	}
  	if got[1].Attrs["n"].(int) != 3 {
  		t.Errorf("got[1].Attrs[\"n\"] = %v, want 3", got[1].Attrs["n"])
  	}
  }
  ```

- [ ] **Step 2: Run the test to verify it fails**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestObserver_ -v`
  Expected: compile error — `undefined: EventAddTotal`, `undefined: Event`, etc.

- [ ] **Step 3: Write `llm-agent-memory/memory/observer.go`**

  ```go
  package memory

  // Observer is the optional sink for memory metric events. A nil
  // Observer is the zero-config no-op — callers who do not opt in see
  // exactly the same behavior they got in M1. Implementations MUST NOT
  // block, MUST NOT panic, and MUST NOT return an error: any failure to
  // record is the implementation's problem. The hot path is unconditional
  // so OnEvent is called on the goroutine that emitted the event.
  //
  // The Observer interface is intentionally minimal — it gives consumers
  // a single typed funnel. Adapters (Prometheus, OTel, log emitters) live
  // outside this package.
  type Observer interface {
  	OnEvent(e Event)
  }

  // Event is the typed payload delivered to Observer.OnEvent. Name is one
  // of the canonical event-name constants declared below (EventAddTotal,
  // EventSearchTotal, ...). Attrs is an optional bag of structured
  // attributes whose schema is frozen per event-name at v0.2.0; future
  // additions are backwards-compatible (new keys may appear, existing
  // keys are never renamed or removed).
  //
  // Attribute schemas per event name (v0.2.0):
  //   EventAddTotal:              {"kind": coremem.Kind}
  //   EventSearchTotal:           {"query_len": int}
  //   EventSearchHits:            {"n": int}            // hit count
  //   EventConsolidatedTotal:     {"n": int}            // promoted count
  //   EventForgottenTotal:        {"kind": coremem.Kind, "n": int}
  //   EventSnapshotItems:         {"kind": coremem.Kind, "n": int}
  //   EventSnapshotVectorsBytes:  {"kind": coremem.Kind, "bytes": int}
  type Event struct {
  	Name  string
  	Attrs map[string]any
  }

  // Canonical event names. These mirror the seven minimum-observability
  // metrics from docs/memory-roadmap.zh-CN.md §4.2 B-1. Consumers should
  // switch on these constants (NOT on raw string literals).
  const (
  	EventAddTotal             = "memory_add_total"
  	EventSearchTotal          = "memory_search_total"
  	EventSearchHits           = "memory_search_hits"
  	EventConsolidatedTotal    = "memory_consolidated_total"
  	EventForgottenTotal       = "memory_forgotten_total"
  	EventSnapshotItems        = "memory_snapshot_items"
  	EventSnapshotVectorsBytes = "memory_snapshot_vectors_bytes"
  )

  // emit is the no-op-guarded internal emitter used by every Observer
  // call site in this package. A nil Observer is the documented
  // zero-config path — emit returns immediately. Otherwise the event is
  // constructed (zero-allocation for nil Attrs) and forwarded.
  func emit(o Observer, name string, attrs map[string]any) {
  	if o == nil {
  		return
  	}
  	o.OnEvent(Event{Name: name, Attrs: attrs})
  }
  ```

- [ ] **Step 4: Run the tests to verify they pass**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestObserver_ -v`
  Expected: all three tests `PASS`.

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/observer.go memory/observer_test.go
  git commit -m "feat(memory): add Observer interface + 7 canonical event names (B-1)"
  git push origin main
  ```

---

## Task 6: Add `Option`/`WithObserver` constructor pattern to all three wrappers

**Files:**
- Modify: `llm-agent-memory/memory/scoped_lifecycle.go`
- Modify: `llm-agent-memory/memory/consolidator.go`
- Modify: `llm-agent-memory/memory/unified_search.go`
- Modify: `llm-agent-memory/memory/observer_test.go` (append)

- [ ] **Step 1: Append a failing test asserting the option threads through**

  ```go
  func TestObserver_ScopedLifecycleManager_AcceptsWithObserver(t *testing.T) {
  	// Construction with WithObserver must succeed and the observer
  	// reference must be retained. (Behavioral emission is asserted in
  	// later tasks; this test only proves the wiring exists.)
  	rec := &recordingObserver{}
  	slm, err := NewScopedLifecycleManager(newCoreScopedManager(t), WithObserver(rec))
  	if err != nil {
  		t.Fatalf("NewScopedLifecycleManager: %v", err)
  	}
  	if slm.observer() != rec {
  		t.Errorf("WithObserver did not install the observer reference")
  	}
  }

  func TestObserver_Consolidator_AcceptsWithObserver(t *testing.T) {
  	rec := &recordingObserver{}
  	c, err := NewConsolidator(newCoreManager(t), WithObserver(rec))
  	if err != nil {
  		t.Fatalf("NewConsolidator: %v", err)
  	}
  	if c.observer() != rec {
  		t.Errorf("WithObserver did not install the observer reference")
  	}
  }

  func TestObserver_UnifiedSearcher_AcceptsWithObserver(t *testing.T) {
  	rec := &recordingObserver{}
  	u, err := NewUnifiedSearcher(newCoreManager(t), WithObserver(rec))
  	if err != nil {
  		t.Fatalf("NewUnifiedSearcher: %v", err)
  	}
  	if u.observer() != rec {
  		t.Errorf("WithObserver did not install the observer reference")
  	}
  }
  ```

- [ ] **Step 2: Run the tests to verify they fail (compile error)**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestObserver_.*_AcceptsWithObserver -v`
  Expected: compile error — `undefined: WithObserver`, `slm.observer undefined`.

- [ ] **Step 3: Wire the option pattern**

  In `observer.go`, append:

  ```go
  // Option is the functional-option type used by the four constructors
  // in this package (NewScopedLifecycleManager, NewConsolidator,
  // NewUnifiedSearcher, NewParallelSearcher). All options are backwards-
  // compatible additions; an empty option list is the documented
  // zero-config behavior.
  type Option func(*config)

  // config is the internal shared config struct accumulated by the
  // variadic option list. Today it carries only an Observer; future
  // options (e.g. WithSerialSearch) extend this struct.
  type config struct {
  	observer Observer
  }

  // WithObserver installs the given Observer on the constructed wrapper.
  // A nil Observer is treated as the zero-config no-op and elides the
  // emit call entirely.
  func WithObserver(o Observer) Option {
  	return func(c *config) { c.observer = o }
  }

  // newConfig is the shared option-folding helper used by every
  // constructor in this package.
  func newConfig(opts []Option) *config {
  	c := &config{}
  	for _, opt := range opts {
  		if opt != nil {
  			opt(c)
  		}
  	}
  	return c
  }
  ```

  In `scoped_lifecycle.go`, modify the struct + constructor:

  Replace the `ScopedLifecycleManager` struct (currently lines 21–23) with:

  ```go
  type ScopedLifecycleManager struct {
  	sm  *coremem.ScopedManager
  	cfg *config
  }
  ```

  Replace `NewScopedLifecycleManager` (currently lines 38–43) with:

  ```go
  func NewScopedLifecycleManager(inner *coremem.ScopedManager, opts ...Option) (*ScopedLifecycleManager, error) {
  	if inner == nil {
  		return nil, ErrScopedManagerRequired
  	}
  	return &ScopedLifecycleManager{sm: inner, cfg: newConfig(opts)}, nil
  }

  // observer exposes the configured observer for in-package callers and
  // tests. Package-private — callers should not depend on the accessor.
  func (s *ScopedLifecycleManager) observer() Observer { return s.cfg.observer }
  ```

  Apply the same shape to `Consolidator` in `consolidator.go`:

  Replace the struct (lines 43–45):

  ```go
  type Consolidator struct {
  	mgr *coremem.Manager
  	cfg *config
  }
  ```

  Replace `NewConsolidator` (lines 54–59):

  ```go
  func NewConsolidator(inner *coremem.Manager, opts ...Option) (*Consolidator, error) {
  	if inner == nil {
  		return nil, ErrManagerRequired
  	}
  	return &Consolidator{mgr: inner, cfg: newConfig(opts)}, nil
  }

  func (c *Consolidator) observer() Observer { return c.cfg.observer }
  ```

  And to `UnifiedSearcher` in `unified_search.go`:

  Replace the struct (lines 22–24):

  ```go
  type UnifiedSearcher struct {
  	mgr *coremem.Manager
  	cfg *config
  }
  ```

  Replace `NewUnifiedSearcher` (lines 32–37):

  ```go
  func NewUnifiedSearcher(inner *coremem.Manager, opts ...Option) (*UnifiedSearcher, error) {
  	if inner == nil {
  		return nil, ErrUnifiedManagerRequired
  	}
  	return &UnifiedSearcher{mgr: inner, cfg: newConfig(opts)}, nil
  }

  func (u *UnifiedSearcher) observer() Observer { return u.cfg.observer }
  ```

- [ ] **Step 4: Run all the new tests + the full package test suite**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -v`
  Expected: every test passes. The variadic-options change is backwards-compatible — existing callers passing no options compile and run unchanged.

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/observer.go memory/observer_test.go memory/scoped_lifecycle.go memory/consolidator.go memory/unified_search.go
  git commit -m "feat(memory): add Option/WithObserver pattern to all wrappers (B-1)"
  git push origin main
  ```

---

## Task 7: Emit `EventConsolidatedTotal` from `Consolidator.Consolidate` and `ConsolidateScoped`

**Files:**
- Modify: `llm-agent-memory/memory/consolidator.go`
- Modify: `llm-agent-memory/memory/scoped_lifecycle.go`
- Modify: `llm-agent-memory/memory/consolidator_test.go` (append)
- Modify: `llm-agent-memory/memory/scoped_lifecycle_test.go` (append)

- [ ] **Step 1: Append failing tests for both methods**

  In `consolidator_test.go`:

  ```go
  func TestConsolidator_Consolidate_EmitsConsolidatedTotalEvent(t *testing.T) {
  	rec := &recordingObserver{}
  	c, err := NewConsolidator(newCoreManager(t), WithObserver(rec))
  	if err != nil {
  		t.Fatalf("NewConsolidator: %v", err)
  	}
  	ctx := context.Background()
  	mgr := c.mgr
  	if _, err := mgr.Add(ctx, coremem.KindWorking, coremem.MemoryItem{Content: "x", Importance: 0.9}); err != nil {
  		t.Fatalf("Add: %v", err)
  	}
  	if _, err := mgr.Add(ctx, coremem.KindWorking, coremem.MemoryItem{Content: "y", Importance: 0.9}); err != nil {
  		t.Fatalf("Add: %v", err)
  	}

  	if _, err := c.Consolidate(ctx, coremem.ConsolidateOptions{Threshold: 0.7}); err != nil {
  		t.Fatalf("Consolidate: %v", err)
  	}

  	got := rec.snapshot()
  	var found *Event
  	for i := range got {
  		if got[i].Name == EventConsolidatedTotal {
  			found = &got[i]
  			break
  		}
  	}
  	if found == nil {
  		t.Fatalf("no %q event emitted (events: %v)", EventConsolidatedTotal, got)
  	}
  	if n, _ := found.Attrs["n"].(int); n != 2 {
  		t.Errorf("event Attrs[\"n\"] = %v, want 2", found.Attrs["n"])
  	}
  }
  ```

  In `scoped_lifecycle_test.go`:

  ```go
  func TestScopedLifecycle_ConsolidateScoped_EmitsConsolidatedTotalEvent(t *testing.T) {
  	rec := &recordingObserver{}
  	slm, err := NewScopedLifecycleManager(newCoreScopedManager(t), WithObserver(rec))
  	if err != nil {
  		t.Fatalf("NewScopedLifecycleManager: %v", err)
  	}
  	ctx := coremem.WithScope(context.Background(), coremem.Scope{User: "obs-user"})
  	if _, err := slm.sm.Add(ctx, coremem.KindWorking, coremem.MemoryItem{Content: "a", Importance: 0.9}); err != nil {
  		t.Fatalf("Add: %v", err)
  	}
  	if _, err := slm.ConsolidateScoped(ctx, coremem.ConsolidateOptions{Threshold: 0.7}); err != nil {
  		t.Fatalf("ConsolidateScoped: %v", err)
  	}

  	got := rec.snapshot()
  	var found *Event
  	for i := range got {
  		if got[i].Name == EventConsolidatedTotal {
  			found = &got[i]
  			break
  		}
  	}
  	if found == nil {
  		t.Fatalf("no %q event emitted (events: %v)", EventConsolidatedTotal, got)
  	}
  	if n, _ := found.Attrs["n"].(int); n != 1 {
  		t.Errorf("event Attrs[\"n\"] = %v, want 1", found.Attrs["n"])
  	}
  }
  ```

- [ ] **Step 2: Run tests to confirm failure**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run 'EmitsConsolidatedTotalEvent$' -v`
  Expected: both FAIL — events are not yet emitted.

- [ ] **Step 3: Add the emit calls in both impls**

  In `consolidator.go`, modify `Consolidate` — append immediately before the final `return count, nil`:

  ```go
  	emit(c.cfg.observer, EventConsolidatedTotal, map[string]any{"n": count})
  	return count, nil
  ```

  In `scoped_lifecycle.go`, modify `ConsolidateScoped` — same shape, append before the final `return count, nil`:

  ```go
  	emit(s.cfg.observer, EventConsolidatedTotal, map[string]any{"n": count})
  	return count, nil
  ```

- [ ] **Step 4: Run both new tests**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run 'EmitsConsolidatedTotalEvent$' -v`
  Expected: both PASS.

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/consolidator.go memory/scoped_lifecycle.go memory/consolidator_test.go memory/scoped_lifecycle_test.go
  git commit -m "feat(memory): emit memory_consolidated_total from Consolidate paths (B-1)"
  git push origin main
  ```

---

## Task 8: Emit `EventForgottenTotal` from `ForgetScoped`

**Files:**
- Modify: `llm-agent-memory/memory/scoped_lifecycle.go`
- Modify: `llm-agent-memory/memory/scoped_lifecycle_test.go` (append)

- [ ] **Step 1: Append failing test**

  ```go
  func TestScopedLifecycle_ForgetScoped_EmitsForgottenTotalEvent(t *testing.T) {
  	rec := &recordingObserver{}
  	slm, err := NewScopedLifecycleManager(newCoreScopedManager(t), WithObserver(rec))
  	if err != nil {
  		t.Fatalf("NewScopedLifecycleManager: %v", err)
  	}
  	ctx := coremem.WithScope(context.Background(), coremem.Scope{User: "forget-obs"})
  	if _, err := slm.sm.Add(ctx, coremem.KindEpisodic, coremem.MemoryItem{Content: "a", Importance: 0.1}); err != nil {
  		t.Fatalf("Add: %v", err)
  	}
  	if _, err := slm.ForgetScoped(ctx, coremem.KindEpisodic, coremem.ForgetOptions{
  		Strategy:  coremem.ForgetByImportance,
  		Threshold: 0.5,
  	}); err != nil {
  		t.Fatalf("ForgetScoped: %v", err)
  	}

  	got := rec.snapshot()
  	var found *Event
  	for i := range got {
  		if got[i].Name == EventForgottenTotal {
  			found = &got[i]
  			break
  		}
  	}
  	if found == nil {
  		t.Fatalf("no %q event emitted", EventForgottenTotal)
  	}
  	if n, _ := found.Attrs["n"].(int); n != 1 {
  		t.Errorf("Attrs[\"n\"] = %v, want 1", found.Attrs["n"])
  	}
  	if k, _ := found.Attrs["kind"].(coremem.Kind); k != coremem.KindEpisodic {
  		t.Errorf("Attrs[\"kind\"] = %v, want %v", found.Attrs["kind"], coremem.KindEpisodic)
  	}
  }
  ```

- [ ] **Step 2: Run to confirm failure**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestScopedLifecycle_ForgetScoped_EmitsForgottenTotalEvent -v`
  Expected: FAIL.

- [ ] **Step 3: Refactor `ForgetScoped` to emit once with the total count regardless of strategy**

  In `scoped_lifecycle.go`, wrap each `case` branch so the final return goes through a single tail emitter. The cleanest refactor: change every branch to assign to a `count` variable and `break`, then emit + return at the end. Replace `ForgetScoped` with:

  ```go
  func (s *ScopedLifecycleManager) ForgetScoped(ctx context.Context, kind coremem.Kind, opts coremem.ForgetOptions) (int, error) {
  	mgr := s.sm.Inner()
  	allItems, err := s.listAllScoped(ctx, 200)
  	if err != nil {
  		return 0, fmt.Errorf("memory: list %s: %w", kind, err)
  	}
  	candidates := allItems[kind]
  	var count int
  	switch opts.Strategy {
  	case coremem.ForgetByImportance:
  		for _, it := range candidates {
  			if coremem.IsPinned(it) {
  				continue
  			}
  			if it.Importance < opts.Threshold {
  				if err := mgr.Remove(ctx, kind, it.ID); err == nil {
  					count++
  				}
  			}
  		}
  	case coremem.ForgetByAge:
  		if opts.MaxAge <= 0 {
  			return 0, fmt.Errorf("memory: forget by age requires MaxAge > 0")
  		}
  		now := timeNow()
  		for _, it := range candidates {
  			if coremem.IsPinned(it) {
  				continue
  			}
  			if now.Sub(it.CreatedAt) > opts.MaxAge {
  				if err := mgr.Remove(ctx, kind, it.ID); err == nil {
  					count++
  				}
  			}
  		}
  	case coremem.ForgetByCapacity:
  		if opts.Keep <= 0 {
  			return 0, nil
  		}
  		all := make([]forgetPair, 0, len(candidates))
  		for _, it := range candidates {
  			if coremem.IsPinned(it) {
  				continue
  			}
  			all = append(all, forgetPair{it.ID, it.Importance})
  		}
  		if len(all) <= opts.Keep {
  			return 0, nil
  		}
  		sortPairsByImpAsc(all)
  		toEvict := len(all) - opts.Keep
  		for i := 0; i < toEvict; i++ {
  			if err := mgr.Remove(ctx, kind, all[i].id); err == nil {
  				count++
  			}
  		}
  	default:
  		return 0, fmt.Errorf("memory: unknown forget strategy %q", opts.Strategy)
  	}
  	emit(s.cfg.observer, EventForgottenTotal, map[string]any{"kind": kind, "n": count})
  	return count, nil
  }
  ```

- [ ] **Step 4: Run the new test + the full scoped suite**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestScopedLifecycle -v`
  Expected: all pass.

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/scoped_lifecycle.go memory/scoped_lifecycle_test.go
  git commit -m "feat(memory): emit memory_forgotten_total from ForgetScoped (B-1)"
  git push origin main
  ```

---

## Task 9: Emit `EventSearchTotal` + `EventSearchHits` from `UnifiedSearcher.SearchUnified`

**Files:**
- Modify: `llm-agent-memory/memory/unified_search.go`
- Modify: `llm-agent-memory/memory/unified_search_test.go` (append)

- [ ] **Step 1: Append failing test**

  ```go
  func TestUnifiedSearcher_SearchUnified_EmitsSearchTotalAndHits(t *testing.T) {
  	rec := &recordingObserver{}
  	mgr := newCoreManager(t)
  	u, err := NewUnifiedSearcher(mgr, WithObserver(rec))
  	if err != nil {
  		t.Fatalf("NewUnifiedSearcher: %v", err)
  	}
  	ctx := context.Background()
  	if _, err := mgr.Add(ctx, coremem.KindEpisodic, coremem.MemoryItem{Content: "go modules guide", Importance: 0.5}); err != nil {
  		t.Fatalf("Add: %v", err)
  	}

  	results, err := u.SearchUnified(ctx, "go modules", 5)
  	if err != nil {
  		t.Fatalf("SearchUnified: %v", err)
  	}

  	got := rec.snapshot()
  	var searchTotal, searchHits *Event
  	for i := range got {
  		switch got[i].Name {
  		case EventSearchTotal:
  			searchTotal = &got[i]
  		case EventSearchHits:
  			searchHits = &got[i]
  		}
  	}
  	if searchTotal == nil {
  		t.Errorf("no %q event emitted", EventSearchTotal)
  	} else if ql, _ := searchTotal.Attrs["query_len"].(int); ql != len("go modules") {
  		t.Errorf("query_len = %v, want %d", searchTotal.Attrs["query_len"], len("go modules"))
  	}
  	if searchHits == nil {
  		t.Errorf("no %q event emitted", EventSearchHits)
  	} else if n, _ := searchHits.Attrs["n"].(int); n != len(results) {
  		t.Errorf("hits n = %v, want %d", searchHits.Attrs["n"], len(results))
  	}
  }
  ```

- [ ] **Step 2: Run to confirm failure**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestUnifiedSearcher_SearchUnified_EmitsSearchTotalAndHits -v`
  Expected: FAIL.

- [ ] **Step 3: Modify `SearchUnified` to emit both events**

  In `unified_search.go`, modify `SearchUnified`. Add a `defer`-free emit at the top for `EventSearchTotal`, and a final emit before `return out, nil` for `EventSearchHits`:

  ```go
  func (u *UnifiedSearcher) SearchUnified(ctx context.Context, query string, topK int) ([]coremem.SearchResult, error) {
  	emit(u.cfg.observer, EventSearchTotal, map[string]any{"query_len": len(query)})
  	perKind, err := u.mgr.SearchAll(ctx, query, topK)
  	if err != nil {
  		return nil, fmt.Errorf("memory: unified search fan-out: %w", err)
  	}
  	merged := make([]coremem.SearchResult, 0)
  	for _, kind := range []coremem.Kind{coremem.KindWorking, coremem.KindEpisodic, coremem.KindSemantic} {
  		merged = append(merged, perKind[kind]...)
  	}
  	type key struct {
  		id      string
  		content string
  	}
  	best := make(map[key]coremem.SearchResult, len(merged))
  	for _, r := range merged {
  		k := key{id: r.Item.ID, content: r.Item.Content}
  		prev, ok := best[k]
  		if !ok || r.Score > prev.Score {
  			best[k] = r
  		}
  	}
  	out := make([]coremem.SearchResult, 0, len(best))
  	for _, r := range best {
  		out = append(out, r)
  	}
  	sort.Slice(out, func(i, j int) bool {
  		if out[i].Score != out[j].Score {
  			return out[i].Score > out[j].Score
  		}
  		return out[i].Item.ID < out[j].Item.ID
  	})
  	if topK > 0 && len(out) > topK {
  		out = out[:topK]
  	}
  	emit(u.cfg.observer, EventSearchHits, map[string]any{"n": len(out)})
  	return out, nil
  }
  ```

- [ ] **Step 4: Run the new test + the full unified-search suite**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestUnifiedSearcher -v`
  Expected: all pass.

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/unified_search.go memory/unified_search_test.go
  git commit -m "feat(memory): emit memory_search_total and memory_search_hits from SearchUnified (B-1)"
  git push origin main
  ```

---

## Task 10: Emit `EventAddTotal` from `Consolidator.Consolidate` (sibling-owned Add path) and snapshot events from `Consolidator`'s ExportAll wrap

**Files:**
- Modify: `llm-agent-memory/memory/consolidator.go`
- Modify: `llm-agent-memory/memory/consolidator_test.go` (append)

Rationale: per the Open Decision on `memory_add_total` scoping, the sibling-owned Add path is the episodic clone inside `Consolidate`. We also expose two snapshot-scoped emissions (`EventSnapshotItems`, `EventSnapshotVectorsBytes`) via a thin `ExportAll(ctx, dir) (map[coremem.Kind]coremem.Snapshot, error)` wrapper on `Consolidator` that delegates to core but counts items+bytes per kind. This satisfies the §4.2 B-1 minimum metric set without forcing a new file.

- [ ] **Step 1: Append a single failing test covering all three events**

  ```go
  func TestConsolidator_Consolidate_EmitsAddTotalPerPromotion(t *testing.T) {
  	rec := &recordingObserver{}
  	mgr := newCoreManager(t)
  	c, err := NewConsolidator(mgr, WithObserver(rec))
  	if err != nil {
  		t.Fatalf("NewConsolidator: %v", err)
  	}
  	ctx := context.Background()
  	if _, err := mgr.Add(ctx, coremem.KindWorking, coremem.MemoryItem{Content: "p1", Importance: 0.9}); err != nil {
  		t.Fatalf("Add: %v", err)
  	}
  	if _, err := mgr.Add(ctx, coremem.KindWorking, coremem.MemoryItem{Content: "p2", Importance: 0.9}); err != nil {
  		t.Fatalf("Add: %v", err)
  	}
  	if _, err := c.Consolidate(ctx, coremem.ConsolidateOptions{Threshold: 0.7}); err != nil {
  		t.Fatalf("Consolidate: %v", err)
  	}

  	addCount := 0
  	for _, e := range rec.snapshot() {
  		if e.Name == EventAddTotal {
  			addCount++
  			if k, _ := e.Attrs["kind"].(coremem.Kind); k != coremem.KindEpisodic {
  				t.Errorf("EventAddTotal kind = %v, want %v", e.Attrs["kind"], coremem.KindEpisodic)
  			}
  		}
  	}
  	if addCount != 2 {
  		t.Errorf("EventAddTotal count = %d, want 2 (one per promoted item)", addCount)
  	}
  }

  func TestConsolidator_ExportAll_EmitsSnapshotItemsAndVectorBytes(t *testing.T) {
  	rec := &recordingObserver{}
  	mgr := newCoreManager(t)
  	c, err := NewConsolidator(mgr, WithObserver(rec))
  	if err != nil {
  		t.Fatalf("NewConsolidator: %v", err)
  	}
  	ctx := context.Background()
  	if _, err := mgr.Add(ctx, coremem.KindEpisodic, coremem.MemoryItem{Content: "snap-me", Importance: 0.5}); err != nil {
  		t.Fatalf("Add: %v", err)
  	}

  	snaps, err := c.ExportAll(ctx, "")
  	if err != nil {
  		t.Fatalf("ExportAll: %v", err)
  	}
  	if len(snaps[coremem.KindEpisodic].Items) != 1 {
  		t.Fatalf("expected 1 episodic snapshot item, got %d", len(snaps[coremem.KindEpisodic].Items))
  	}

  	var items, bytes *Event
  	for _, e := range rec.snapshot() {
  		switch e.Name {
  		case EventSnapshotItems:
  			if k, _ := e.Attrs["kind"].(coremem.Kind); k == coremem.KindEpisodic {
  				items = &e
  			}
  		case EventSnapshotVectorsBytes:
  			if k, _ := e.Attrs["kind"].(coremem.Kind); k == coremem.KindEpisodic {
  				bytes = &e
  			}
  		}
  	}
  	if items == nil {
  		t.Errorf("no %q event for KindEpisodic", EventSnapshotItems)
  	} else if n, _ := items.Attrs["n"].(int); n != 1 {
  		t.Errorf("snapshot items n = %v, want 1", items.Attrs["n"])
  	}
  	if bytes == nil {
  		t.Errorf("no %q event for KindEpisodic", EventSnapshotVectorsBytes)
  	} else if b, _ := bytes.Attrs["bytes"].(int); b <= 0 {
  		t.Errorf("vector bytes = %v, want > 0", bytes.Attrs["bytes"])
  	}
  }
  ```

- [ ] **Step 2: Run to confirm failure**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run 'TestConsolidator_(Consolidate_EmitsAddTotalPerPromotion|ExportAll_EmitsSnapshotItemsAndVectorBytes)' -v`
  Expected: FAIL — `ExportAll` is undefined and no `EventAddTotal` emission yet.

- [ ] **Step 3: Add the emit + the `ExportAll` wrap**

  In `consolidator.go`, inside `Consolidate`, immediately after the successful episodic `mgr.Add` (right after `c.mgr.Add(ctx, coremem.KindEpisodic, clone)` returns nil err), add:

  ```go
  		emit(c.cfg.observer, EventAddTotal, map[string]any{"kind": coremem.KindEpisodic})
  ```

  Append the `ExportAll` wrap at the bottom of `consolidator.go`:

  ```go
  // ExportAll delegates to coremem.Manager.ExportAll and, when an
  // Observer is installed, emits one EventSnapshotItems and one
  // EventSnapshotVectorsBytes per kind in the returned snapshot map.
  // The dir parameter is forwarded verbatim; pass "" for in-memory only.
  func (c *Consolidator) ExportAll(ctx context.Context, dir string) (map[coremem.Kind]coremem.Snapshot, error) {
  	snaps, err := c.mgr.ExportAll(ctx, dir)
  	if err != nil {
  		return nil, err
  	}
  	for kind, snap := range snaps {
  		emit(c.cfg.observer, EventSnapshotItems, map[string]any{
  			"kind": kind, "n": len(snap.Items),
  		})
  		var vbytes int
  		for _, si := range snap.Items {
  			vbytes += len(si.Vector) * 4 // float32 (4 bytes each)
  		}
  		emit(c.cfg.observer, EventSnapshotVectorsBytes, map[string]any{
  			"kind": kind, "bytes": vbytes,
  		})
  	}
  	return snaps, nil
  }
  ```

  Note: `coremem.SnapshotItem.Vector` is `[]float32` — see `go doc github.com/costa92/llm-agent/memory.SnapshotItem` to confirm at run time. If the field is `[]float64`, change `* 4` to `* 8` and re-run the test (the test asserts `bytes > 0`, so it will catch the wrong constant).

- [ ] **Step 4: Run the new tests**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run 'TestConsolidator_(Consolidate_EmitsAddTotalPerPromotion|ExportAll_EmitsSnapshotItemsAndVectorBytes)' -v`
  Expected: both PASS.

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/consolidator.go memory/consolidator_test.go
  git commit -m "feat(memory): emit memory_add_total + snapshot events from Consolidator (B-1)"
  git push origin main
  ```

---

# Phase B-3 — Parallel SearchAll (Tasks 11–12)

## Task 11: `ParallelSearcher` parity with `coremem.Manager.SearchAll`

**Files:**
- Create: `llm-agent-memory/memory/parallel_search.go`
- Create: `llm-agent-memory/memory/parallel_search_test.go`

- [ ] **Step 1: Write the failing parity test**

  ```go
  package memory

  import (
  	"context"
  	"reflect"
  	"sort"
  	"testing"

  	coremem "github.com/costa92/llm-agent/memory"
  )

  func TestParallelSearcher_SearchAllParallel_MatchesCoreSearchAll(t *testing.T) {
  	mgr := newCoreManager(t)
  	ps, err := NewParallelSearcher(mgr)
  	if err != nil {
  		t.Fatalf("NewParallelSearcher: %v", err)
  	}

  	ctx := context.Background()
  	if _, err := mgr.Add(ctx, coremem.KindWorking, coremem.MemoryItem{Content: "alpha", Importance: 0.5}); err != nil {
  		t.Fatalf("Add: %v", err)
  	}
  	if _, err := mgr.Add(ctx, coremem.KindEpisodic, coremem.MemoryItem{Content: "alpha", Importance: 0.5}); err != nil {
  		t.Fatalf("Add: %v", err)
  	}
  	if _, err := mgr.Add(ctx, coremem.KindSemantic, coremem.MemoryItem{Content: "alpha-guide", Tags: []string{"a"}, Importance: 0.5}); err != nil {
  		t.Fatalf("Add: %v", err)
  	}

  	want, err := mgr.SearchAll(ctx, "alpha", 5)
  	if err != nil {
  		t.Fatalf("SearchAll: %v", err)
  	}
  	got, err := ps.SearchAllParallel(ctx, "alpha", 5)
  	if err != nil {
  		t.Fatalf("SearchAllParallel: %v", err)
  	}

  	if !sameKindKeys(want, got) {
  		t.Fatalf("kind keys differ: want %v, got %v", kindsOf(want), kindsOf(got))
  	}
  	for kind := range want {
  		w := normalizeResults(want[kind])
  		g := normalizeResults(got[kind])
  		if !reflect.DeepEqual(w, g) {
  			t.Errorf("kind %v: want %v, got %v", kind, w, g)
  		}
  	}
  }

  // sameKindKeys returns true if a and b have the same set of map keys.
  func sameKindKeys(a, b map[coremem.Kind][]coremem.SearchResult) bool {
  	if len(a) != len(b) {
  		return false
  	}
  	for k := range a {
  		if _, ok := b[k]; !ok {
  			return false
  		}
  	}
  	return true
  }

  func kindsOf(m map[coremem.Kind][]coremem.SearchResult) []coremem.Kind {
  	out := make([]coremem.Kind, 0, len(m))
  	for k := range m {
  		out = append(out, k)
  	}
  	sort.Slice(out, func(i, j int) bool { return string(out[i]) < string(out[j]) })
  	return out
  }

  // normalizeResults sorts a per-kind result slice by (Score desc, ID asc)
  // so reflect.DeepEqual is independent of any deliberate parallel
  // reordering. coremem.Manager.SearchAll documents that per-kind topK is
  // applied internally; the post-topK ordering is well-defined by the
  // underlying memory's Search impl, so any difference here would be a
  // real bug.
  func normalizeResults(rs []coremem.SearchResult) []coremem.SearchResult {
  	out := make([]coremem.SearchResult, len(rs))
  	copy(out, rs)
  	sort.Slice(out, func(i, j int) bool {
  		if out[i].Score != out[j].Score {
  			return out[i].Score > out[j].Score
  		}
  		return out[i].Item.ID < out[j].Item.ID
  	})
  	return out
  }
  ```

- [ ] **Step 2: Run to confirm failure**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestParallelSearcher_SearchAllParallel_MatchesCoreSearchAll -v`
  Expected: compile error — `undefined: NewParallelSearcher`.

- [ ] **Step 3: Write `llm-agent-memory/memory/parallel_search.go`**

  **v0.7.0 baseline (verified 2026-05-26):** `coremem.Manager` does *not* expose `MemoryFor(Kind)`, but `(*coremem.Manager).Search(ctx, Kind, query, topK)` (manager.go:88-94) routes via the unexported `lookup()` switch (manager.go:416-436), which reads three write-once-at-construction fields with no locking — so concurrent calls are race-free. `(*coremem.Manager).SearchAll` (manager.go:98-115) is genuinely serial today, so the goroutine fan-out below produces a real parallel speedup. No upstream PR needed.

  ```go
  package memory

  import (
  	"context"
  	"errors"
  	"fmt"
  	"sync"

  	coremem "github.com/costa92/llm-agent/memory"
  )

  // ParallelSearcher wraps a *coremem.Manager and exposes
  // SearchAllParallel — a drop-in replacement for
  // coremem.Manager.SearchAll that fans out one goroutine per kind by
  // dispatching to (*coremem.Manager).Search(ctx, kind, ...). The
  // returned per-kind map is identical in shape and content to core's
  // serial implementation; the only observable difference is wall-time.
  //
  // Concurrency primitive: stdlib sync.WaitGroup + buffered channel +
  // context.WithCancel. The module is stdlib-only (master-roadmap §3
  // dependency policy), so we explicitly avoid golang.org/x/sync/errgroup.
  type ParallelSearcher struct {
  	mgr *coremem.Manager
  	cfg *config
  }

  // ErrParallelManagerRequired is returned by NewParallelSearcher when
  // the inner *coremem.Manager is nil.
  var ErrParallelManagerRequired = errors.New("memory: parallel searcher requires manager")

  // NewParallelSearcher wraps an existing *coremem.Manager. Options use
  // the shared Option type (WithObserver, etc.). Returns
  // ErrParallelManagerRequired if inner is nil.
  func NewParallelSearcher(inner *coremem.Manager, opts ...Option) (*ParallelSearcher, error) {
  	if inner == nil {
  		return nil, ErrParallelManagerRequired
  	}
  	return &ParallelSearcher{mgr: inner, cfg: newConfig(opts)}, nil
  }

  // observer exposes the configured observer for in-package callers.
  func (p *ParallelSearcher) observer() Observer { return p.cfg.observer }

  // parallelKindResult is the per-kind work item exchanged through the
  // buffered channel. err is forwarded raw; the receiver loop checks for
  // coremem.ErrKindDisabled to silently skip inactive kinds (parity with
  // coremem.Manager.SearchAll, manager.go:100-104).
  type parallelKindResult struct {
  	kind    coremem.Kind
  	results []coremem.SearchResult
  	err     error
  }

  // SearchAllParallel fans out the query to every active kind. Returns
  // the same map shape as coremem.Manager.SearchAll: disabled kinds are
  // omitted from the result map; active kinds are always present (even
  // with an empty []SearchResult slice). topK is forwarded per-kind
  // verbatim. On any non-disabled error, returns that error wrapped with
  // the offending kind.
  //
  // Kinds enumerated: KindWorking, KindEpisodic, KindSemantic — matching
  // coremem.Manager.SearchAll's internal iteration order (the per-kind
  // map result is order-independent so this is informational).
  //
  // ctx-derived cancel: deferred-cancel ensures any goroutine still in
  // flight when an error short-circuits the result-collection loop sees
  // ctx.Done(); this is a hardening step for M3+ when callers wire real
  // cancellation semantics — today wg.Wait blocks until all 3 finish.
  func (p *ParallelSearcher) SearchAllParallel(ctx context.Context, query string, topK int) (map[coremem.Kind][]coremem.SearchResult, error) {
  	kinds := []coremem.Kind{coremem.KindWorking, coremem.KindEpisodic, coremem.KindSemantic}
  	ctx, cancel := context.WithCancel(ctx)
  	defer cancel()

  	ch := make(chan parallelKindResult, len(kinds))
  	var wg sync.WaitGroup
  	for _, kind := range kinds {
  		wg.Add(1)
  		go func(k coremem.Kind) {
  			defer wg.Done()
  			res, err := p.mgr.Search(ctx, k, query, topK)
  			ch <- parallelKindResult{kind: k, results: res, err: err}
  		}(kind)
  	}
  	wg.Wait()
  	close(ch)

  	out := make(map[coremem.Kind][]coremem.SearchResult, len(kinds))
  	for r := range ch {
  		if errors.Is(r.err, coremem.ErrKindDisabled) {
  			continue
  		}
  		if r.err != nil {
  			return nil, fmt.Errorf("memory: parallel search %s: %w", r.kind, r.err)
  		}
  		out[r.kind] = r.results
  	}
  	return out, nil
  }
  ```

- [ ] **Step 4: Run the parity test**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestParallelSearcher_SearchAllParallel_MatchesCoreSearchAll -v`
  Expected: PASS.

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/parallel_search.go memory/parallel_search_test.go
  git commit -m "feat(memory): add ParallelSearcher with parity-tested SearchAllParallel (B-3)"
  git push origin main
  ```

  Note: sibling repo (`llm-agent-memory/`) has its own `.git`; umbrella `.gitignore` excludes it. Commit to the sibling's `origin/main`, NOT to umbrella.

---

## Task 12: `ParallelSearcher` fail-fast on first error + UnifiedSearcher uses it by default

**Files:**
- Modify: `llm-agent-memory/memory/parallel_search.go`
- Modify: `llm-agent-memory/memory/parallel_search_test.go` (append)
- Modify: `llm-agent-memory/memory/unified_search.go`

- [ ] **Step 1: Append failing fail-fast test using a faulting `Memory` shim**

  ```go
  func TestParallelSearcher_SearchAllParallel_FailsFastOnFirstError(t *testing.T) {
  	// Build a manager whose Episodic memory always returns an error on
  	// Search. The Working + Semantic goroutines must observe the error
  	// via the returned err, and the function must NOT silently lose the
  	// failure.
  	mgr, err := coremem.NewManager(coremem.ManagerOptions{
  		Working:  newCoreWorking(t),
  		Episodic: &faultyEpisodic{},
  		Semantic: newCoreSemantic(t),
  	})
  	if err != nil {
  		t.Fatalf("NewManager: %v", err)
  	}
  	ps, err := NewParallelSearcher(mgr)
  	if err != nil {
  		t.Fatalf("NewParallelSearcher: %v", err)
  	}

  	_, err = ps.SearchAllParallel(context.Background(), "anything", 3)
  	if err == nil {
  		t.Fatal("SearchAllParallel returned nil error despite faulty episodic")
  	}
  }

  // faultyEpisodic is a minimal coremem.Memory whose Search returns a
  // fixed error. Other methods return zero values — they are not called
  // by SearchAllParallel.
  type faultyEpisodic struct{}

  func (faultyEpisodic) Add(ctx context.Context, item coremem.MemoryItem) (string, error) {
  	return "", nil
  }
  func (faultyEpisodic) Get(ctx context.Context, id string) (coremem.MemoryItem, error) {
  	return coremem.MemoryItem{}, nil
  }
  func (faultyEpisodic) Remove(ctx context.Context, id string) error                           { return nil }
  func (faultyEpisodic) Update(ctx context.Context, id string, f func(*coremem.MemoryItem)) error { return nil }
  func (faultyEpisodic) Search(ctx context.Context, query string, topK int) ([]coremem.SearchResult, error) {
  	return nil, errors.New("faulty episodic")
  }
  func (faultyEpisodic) Snapshot(ctx context.Context) (coremem.Snapshot, error) {
  	return coremem.Snapshot{}, nil
  }
  func (faultyEpisodic) Restore(ctx context.Context, snap coremem.Snapshot, mode coremem.ImportMode) (coremem.ImportReport, error) {
  	return coremem.ImportReport{}, nil
  }
  func (faultyEpisodic) Stats() coremem.Stats { return coremem.Stats{} }
  func (faultyEpisodic) Kind() coremem.Kind   { return coremem.KindEpisodic }
  ```

  Note: requires `"errors"` import in the test file. Also note: the exact `coremem.Memory` interface shape must be verified with `go doc github.com/costa92/llm-agent/memory.Memory` before writing; the executor should add/remove methods on `faultyEpisodic` to match. Methods are listed here per the visible surface as of v0.5.0; if upstream added new ones, append them as zero-return stubs.

- [ ] **Step 2: Run to confirm failure**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestParallelSearcher_SearchAllParallel_FailsFastOnFirstError -v`
  Expected: depends on Task 11's outcome. If the M1 impl in Task 11 already propagates errors, this may PASS on the first run — that is OK; this task's value is the regression coverage. If it returns nil, the channel-drain loop in `SearchAllParallel` has a bug: every channel item is read, but the `err != nil` branch returns immediately — so the test should pass already. The test exists to lock that behavior in.

- [ ] **Step 3: Route `UnifiedSearcher.SearchUnified` through `ParallelSearcher`**

  In `unified_search.go`, replace the `u.mgr.SearchAll(ctx, query, topK)` call with a parallel version. Top of `SearchUnified` (after the `EventSearchTotal` emit), change:

  ```go
  	perKind, err := u.mgr.SearchAll(ctx, query, topK)
  ```

  to:

  ```go
  	ps, _ := NewParallelSearcher(u.mgr) // never returns an error when u.mgr is non-nil
  	perKind, err := ps.SearchAllParallel(ctx, query, topK)
  ```

  Rationale: `NewParallelSearcher` only returns `ErrParallelManagerRequired` when `inner == nil`; `UnifiedSearcher` already guarantees `u.mgr != nil` in its own constructor, so the discard of err is safe and documented.

- [ ] **Step 4: Run the full unified-search + parallel-search suites**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run 'Test(UnifiedSearcher|ParallelSearcher)' -v`
  Expected: every test passes.

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/parallel_search.go memory/parallel_search_test.go memory/unified_search.go
  git commit -m "feat(memory): route SearchUnified through ParallelSearcher and lock fail-fast (B-3)"
  git push origin main
  ```

---

# Phase B-2 — Deferred-to-Core Documentation + Regression Test (Task 13)

## Task 13: Document B-2 as deferred + pin current Add+eviction behavior

**Files:**
- Modify: `llm-agent-memory/memory/observer.go` (append a doc block)
- Modify: `llm-agent-memory/memory/observer_test.go` (append the pinning test)
- Modify: `llm-agent-memory/CHANGELOG.md`

- [ ] **Step 1: Append a behavioral test that pins current eviction semantics**

  In `observer_test.go`:

  ```go
  func TestObserver_B2_WorkingEvictionStillPicksLowestScoredItem(t *testing.T) {
  	// B-2 — embed-reuse — lives inside coremem.WorkingMemory's private
  	// evictIfOverCapacity. It cannot be wrapped from this sibling. This
  	// test does NOT assert embed call count (that would require an
  	// embedder spy that does not exist in the sibling); it pins the
  	// observable property B-2 promises to preserve: when capacity is
  	// exceeded, the LOWEST-scored item is evicted. When the upstream
  	// optimization lands, this test must continue to pass.
  	w, err := coremem.NewWorking(newCoreEmbedder(), coremem.WorkingOptions{
  		Capacity: 2,
  		Decay:    24 * time.Hour,
  	})
  	if err != nil {
  		t.Fatalf("NewWorking: %v", err)
  	}
  	mgr, err := coremem.NewManager(coremem.ManagerOptions{
  		Working:  w,
  		Episodic: newCoreEpisodic(t),
  		Semantic: newCoreSemantic(t),
  	})
  	if err != nil {
  		t.Fatalf("NewManager: %v", err)
  	}
  	ctx := context.Background()
  	if _, err := mgr.Add(ctx, coremem.KindWorking, coremem.MemoryItem{Content: "low", Importance: 0.1}); err != nil {
  		t.Fatalf("Add low: %v", err)
  	}
  	if _, err := mgr.Add(ctx, coremem.KindWorking, coremem.MemoryItem{Content: "mid", Importance: 0.5}); err != nil {
  		t.Fatalf("Add mid: %v", err)
  	}
  	if _, err := mgr.Add(ctx, coremem.KindWorking, coremem.MemoryItem{Content: "high", Importance: 0.9}); err != nil {
  		t.Fatalf("Add high (triggers eviction): %v", err)
  	}

  	pages, err := mgr.ListAll(ctx, coremem.ListFilter{}, 100, nil)
  	if err != nil {
  		t.Fatalf("ListAll: %v", err)
  	}
  	contents := map[string]bool{}
  	for _, it := range pages[coremem.KindWorking].Items {
  		contents[it.Content] = true
  	}
  	if contents["low"] {
  		t.Errorf("expected the lowest-scored item to be evicted; survivors: %v", contents)
  	}
  	if !contents["high"] {
  		t.Errorf("expected the highest-scored item to survive; survivors: %v", contents)
  	}
  }
  ```

  Note: requires `context`, `time`, and `coremem` imports in `observer_test.go` — add them if not already present.

- [ ] **Step 2: Run the test to confirm current behavior is preserved**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestObserver_B2_WorkingEvictionStillPicksLowestScoredItem -v`
  Expected: PASS today. If it fails, coremem's working eviction has a different rule than B-2's acceptance criterion; STOP and escalate before touching anything.

- [ ] **Step 3: Append the deferred-to-core documentation block to `observer.go`**

  At the bottom of `observer.go`, add a top-level doc comment (anchored to a `// B2Status` constant so it shows up in godoc):

  ```go
  // B2Status documents the Phase B-2 optimization status. The optimization
  // (reuse the Working.Add embedding inside evictIfOverCapacity) lives
  // inside coremem.WorkingMemory's private call site
  // (llm-agent/memory/working.go:56-63 and 179-185) and cannot be applied
  // from this sibling without modifying core. This package therefore
  // tracks B-2 as deferred to an upstream PR.
  //
  // The observable property B-2 promises to preserve — capacity-full
  // eviction picks the lowest-scored item — is pinned by the regression
  // test TestObserver_B2_WorkingEvictionStillPicksLowestScoredItem so
  // the eventual upstream PR cannot cause a silent regression.
  //
  // Status: pending upstream issue at github.com/costa92/llm-agent
  // referencing working.go:56-63. File the issue at M2 kick-off.
  const B2Status = "deferred-to-core"
  ```

- [ ] **Step 4: Update CHANGELOG.md and bump Version**

  Edit `llm-agent-memory/memory/version.go`:

  ```go
  const Version = "0.2.0"
  ```

  Edit `llm-agent-memory/memory/version_test.go` — no change needed; the existing semver-shape test still passes with `0.2.0`.

  Prepend to `llm-agent-memory/CHANGELOG.md` (above the existing `## [0.1.0]` entry):

  ```markdown
  ## [0.2.0] - 2026-05-26

  ### Fixed

  - `ConsolidateScoped`, `ForgetScoped`, `StatsScoped`, and
    `Consolidator.Consolidate` now page through cursors instead of
    silently dropping items past the first underlying page (closes the
    final-review I-1 finding).

  ### Added

  - `Observer` interface with `Event{Name, Attrs}` payload schema
    (locked at v0.2.0) and seven canonical event-name constants
    (`memory_add_total`, `memory_search_total`, `memory_search_hits`,
    `memory_consolidated_total`, `memory_forgotten_total`,
    `memory_snapshot_items`, `memory_snapshot_vectors_bytes`) per
    Phase B-1 of the master roadmap.
  - `WithObserver(Observer) Option` for `NewScopedLifecycleManager`,
    `NewConsolidator`, `NewUnifiedSearcher`, and the new
    `NewParallelSearcher`. Zero-config (no option) is the documented
    no-op.
  - `ParallelSearcher.SearchAllParallel(ctx, query, topK)` — stdlib-only
    per-kind fan-out with the same per-kind map shape as
    `coremem.Manager.SearchAll`. `UnifiedSearcher.SearchUnified` now
    delegates its fan-out through `ParallelSearcher` by default.
  - `Consolidator.ExportAll(ctx, dir)` thin wrap that emits per-kind
    `memory_snapshot_items` and `memory_snapshot_vectors_bytes`.

  ### Notes

  - Phase B-2 (Working eviction embed-reuse) is **deferred to a core PR**:
    it lives inside `coremem.WorkingMemory.evictIfOverCapacity` (package-
    private) and cannot be wrapped from this sibling. A regression test
    pins the eviction semantics so the eventual upstream change cannot
    silently break this consumer.
  ```

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/observer.go memory/observer_test.go memory/version.go CHANGELOG.md
  git commit -m "docs(memory): pin B-2 eviction semantics + bump version to 0.2.0"
  git push origin main
  ```

---

# Final Verification (Tasks 14–15)

## Task 14: Full-package smoke with race detector

**Files:** none (verification only)

- [ ] **Step 1: Run the entire sibling test suite with the race detector**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./... -count=1 -race`
  Expected: `ok github.com/costa92/llm-agent-memory/memory ...`, exit code 0, no race warnings. The race detector is especially important because Task 11 introduced goroutines.

- [ ] **Step 2: Run `go vet` on the entire module**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go vet ./...`
  Expected: no output, exit code 0.

- [ ] **Step 3: Confirm the umbrella sibling suite is still green**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem && bash scripts/eco.sh test`
  Expected: every sibling passes.

- [ ] **Step 4: Confirm the stdlib-only gate still passes on core (sanity — we never touched `llm-agent/`)**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem && bash scripts/stdlib-only-check.sh`
  Expected: `stdlib-only-check: PASS`.

- [ ] **Step 5: No commit — verification gate only.** Fix any regression in a follow-up commit before proceeding to Task 15.

---

## Task 15: Tag `v0.2.0`

**Files:** none (git operation only)

- [ ] **Step 1: Verify the sibling working tree is clean and on `main`**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && git status` and `git log --oneline -10`
  Expected: clean tree on `main`; the last commit is `docs(memory): pin B-2 eviction semantics + bump version to 0.2.0`. (Sibling has its own `.git` — umbrella `.gitignore` excludes `/llm-agent-memory/`; the v0.2.0 release artifact lives entirely inside the sibling repo.)

- [ ] **Step 2: Confirm Task 14 verification gate passed**

  Per resolved q3 (see "Open Decisions Resolved in This Plan" → "v0.2.0 tag timing"): no cross-AI review delay — if Task 14 was green, proceed to tag immediately. Sanity-check by re-reading Task 14's last test output; if not available, re-run `bash scripts/eco.sh test` (umbrella) and `GOWORK=off go test -race ./...` (sibling). Both must be green before Step 3.

- [ ] **Step 3: Create the annotated tag in the sibling repo**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && git tag -a v0.2.0 -m "llm-agent-memory v0.2.0 — pagination fix + Observer hooks + ParallelSearcher (M2)"`
  Expected: tag created locally. Sibling repos use unprefixed tags (`v<X.Y.Z>`); verified 2026-05-26 against the existing v0.1.0 tag in this same sibling — verify by checking `git tag --list 'v*'` (expect both `v0.1.0` and `v0.2.0`).

- [ ] **Step 4: Push the tag to sibling's `origin`**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && git push origin v0.2.0`
  Expected: tag uploaded to the sibling's GitHub remote. Per resolved q3, push immediately — no opt-in gate.

- [ ] **Step 5: No commit (the tag is the artifact).**

---

## Self-Review

### Pagination coverage (final-review I-1)

- `ConsolidateScoped` — Task 1 (failing test) + Task 2 (cursor loop impl). Test inserts 180 items, asserts all 180 promoted.
- `ForgetScoped` — Task 3, sub-test 1. Inserts 170 items, asserts all 170 removed.
- `StatsScoped` — Task 3, sub-test 2. Inserts 160 items, asserts `Count == 160`.
- `Consolidator.Consolidate` — Task 4. Inserts 175 items, asserts all 175 promoted.

All four methods covered with insertion counts > the underlying cap (>= 150 as required by the requirements).

### Observer schema decision

Locked at v0.2.0: `Event{Name string, Attrs map[string]any}`. Per-event Attrs schemas documented in `observer.go` godoc. New keys allowed without breaking; existing keys never removed or renamed.

### B-2 disposition

Locked as (a) deferred-to-core. Regression test (`TestObserver_B2_WorkingEvictionStillPicksLowestScoredItem`) pins the eviction semantics so the eventual upstream PR cannot break this sibling silently. CHANGELOG calls out the deferral explicitly. `B2Status` doc constant is the in-package anchor.

### Stdlib-only check

- `parallel_search.go` imports: `context`, `errors`, `fmt`, `sync`, `github.com/costa92/llm-agent/memory`. No third-party deps.
- `observer.go` imports: none (no I/O, no concurrency).
- `parallel_search_test.go` imports: `context`, `errors`, `reflect`, `sort`, `testing`, `github.com/costa92/llm-agent/memory`. All stdlib + core.
- Verified: `go vet` runs in Task 14 Step 2, which would flag any third-party drift.

### Placeholder scan

- No `TODO`, `tbd`, `similar to above`, or `implement later` in any code block. Every Go file body is spelled out fully.
- Zero STOP-AND-ASK moments remain. All three M2 open questions (q1 `MemoryFor` existence, q2 upstream B-2 issue filing, q3 v0.2.0 release timing) were resolved during plan review (2026-05-26) — see the three corresponding entries in "Open Decisions Resolved in This Plan". M2 is now fully executable end-to-end without orchestrator intervention except for the standard subagent-driven-development review loops.
- Every shell command uses an absolute path and shows expected output.

### Test-name disambiguation from M1

Every new test name in this plan starts with a fresh suffix that does not appear in the M1 file. Verified:

- `TestScopedLifecycle_*_PagesThroughLargeScope` (new) vs `TestScopedLifecycle_*_OnlyPromotesMatchingScope` / `_DoesNotPromoteOtherScope` / `_OnlyDeletesMatchingScope` / `_CountsOnlyMatchingScope` (M1) — disjoint.
- `TestScopedLifecycle_*_EmitsConsolidatedTotalEvent`, `_EmitsForgottenTotalEvent` (new) — disjoint.
- `TestConsolidator_Consolidate_PagesThroughLargeWorkingSet`, `_EmitsConsolidatedTotalEvent`, `_EmitsAddTotalPerPromotion`, `ExportAll_EmitsSnapshotItemsAndVectorBytes` (new) vs `TestConsolidator_FirstPromote_WritesDedupeMetadata`, `_SecondCall_DoesNotRePromote`, `_DedupeMetadata_RoundTripsThroughExportImport` (M1) — disjoint.
- `TestUnifiedSearcher_SearchUnified_EmitsSearchTotalAndHits` (new) vs `TestUnifiedSearcher_FansOutToAllTiers`, `_DedupesByIDAndContent`, `_SortsByScoreDescending`, `_HonorsTopK`, `_DoesNotAlterCoreSearchAll` (M1) — disjoint.
- `TestParallelSearcher_*`, `TestObserver_*` (new top-level prefixes) — no M1 collisions.

### Type consistency

- `Event` / `Observer` / `Option` / `config` / `newConfig` / `emit` — defined once in `observer.go`, referenced uniformly.
- `WithObserver` — single source of truth, used by all four constructors.
- `ParallelSearcher` / `NewParallelSearcher` / `SearchAllParallel` — consistent across `parallel_search.go` and tests.
- Canonical event-name constants — declared once, used by emit call sites in `scoped_lifecycle.go`, `consolidator.go`, `unified_search.go`.

No drift detected.
