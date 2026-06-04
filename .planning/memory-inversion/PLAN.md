# Memory Inversion Migration Plan

Fix the inverted dependency: `llm-agent-memory` (foundational, tag v1.0.0) currently
`require github.com/costa92/llm-agent v0.7.0` and imports `coremem "github.com/costa92/llm-agent/memory"`
across ~23 files. Decided architecture: **Option A (relocate engine) + memory contract folded into
`llm-agent-contract/memory`**.

Status: Phases 0–2 DONE. The inversion is FULLY RESOLVED:
  llm-agent-contract v0.1.0 (leaf + memory/) ← llm-agent-memory v2.0.0 (contract-only)
                                             ← llm-agent v0.8.0 (contract-only, engine amputated).
- Phase 0: contract memory subpkg, tagged v0.1.0.
- Phase 1: llm-agent-memory collapsed onto contract, tagged v2.0.0.
- Phase 2: llm-agent PR #17 merged (decouple + amputate; superseded PR #16; also tidied the examples
  submodule for the transitive contract dep), tagged v0.8.0. release-precheck green.

Phase 3 = full lockstep decoupling finalization: DONE (see LOCKSTEP-PLAN.md). All consumers finalized onto
the tagged contract, zero replaces ecosystem-wide, dep-currency-check.sh GREEN. Tags cut:
  flow v0.2.0, providers v0.3.0, rag v1.10.0, otel v0.4.0, customer-support v0.3.0.
MIGRATION COMPLETE. Final ecosystem:
  contract v0.1.0 (llm/ + memory/) ← memory v2.0.0 (/v2), llm-agent v0.8.0 (engine amputated),
  flow v0.2.0, providers v0.3.0, rag v1.10.0, otel v0.4.0, customer-support v0.3.0 — all contract-pinned,
  zero replaces. (One bad rag tag was caught + re-cut: a git-pull ref-lock left local master stale, so
  always verify go.mod content before tagging.) Remaining: umbrella.yml cross-repo-build flow-checkout
  fix (separate TODO, non-blocking).
Reviews: Plan agent design → codex consult (architecture, 2026-06-03) → codex challenge (sequencing,
2026-06-03, found 6 issues — all incorporated below).

NEW FINDING (Phase 0.1): the 8 metadata helpers were NOT byte-identical between the two source repos —
`llm-agent/memory/profile.go` setters stored strongly-TYPED Source/Category + always-write + lenient
getters; `llm-agent-memory` setters stored STRINGS + delete-on-zero + string-only getters. (The
MemoryItem STRUCT is still identical; only the helpers diverged.) Contract resolution: union getters
(accept typed OR string) + string-storage delete-on-zero setters = reads anything, writes JSON-stable
form. Locked by profile_test.go. Phase 1/2 delete BOTH source helper copies in favor of contractmem.

## Decided end-state

```
llm-agent-contract               <- leaf contract: llm/ + NEW memory/ (interfaces + canonical data types + helpers)
   ^                    ^
llm-agent-memory /v2   llm-agent
(concrete engines:      (framework; memory/tool.go stays as agents.Tool adapter over
 Working/Episodic/       contract memory.Manager interface; context/builder.go uses contract types)
 Semantic + Manager
 + Consolidator/
 RecallEngine + sqlite)
```

## Verified evidence base (file:line)

- **Zero external importers of `llm-agent/memory`**: only `llm-agent/context/builder.go:8,53` and
  `llm-agent/context/context_test.go:247,269,285` consume it (data-only). Island confirmed.
- **Zero external constructor call sites**; **nobody requires `llm-agent-memory` in any go.mod**
  (postgres/worker/gateway/client depend on `llm-agent-memory-contract`, a different durable-storage
  concern). So the `/v2` path break costs nothing externally.
- **Engines already native in llm-agent-memory** (`engine_working.go:21` etc., no coremem). Remaining
  work = collapse coremem type refs onto contract, not rebuild engines.
- **Dual MemoryItem = SAFE**: identical fields, no JSON tags → byte-identical JSON; Snapshot tags match;
  `SnapshotVersion=1`; scope/source/category live in `Metadata` keys. No migration needed.
- **Tag state**: llm-agent `v0.7.0`; llm-agent-memory `v1.0.0`; **llm-agent-contract UNTAGGED** (first
  tag). Contract stdlib-only (no go.sum).

### CI gate reality (corrected after codex challenge — only two STRICT gates)
- **`release-precheck.yml`**: runs ONLY on `release/**` branches; fails if ANY `replace` directive exists.
  (Ordinary main-branch PRs are NOT subject to it.)
- **`llm-agent/.github/workflows/umbrella.yml:74` → `scripts/dep-currency-check.sh`**: STRICT. Queries
  each sibling's **latest REMOTE tag** and fails (`::error::`, exit 1) on any pin that ≠ latest.
  REPOS list = `llm-agent, llm-agent-rag, llm-agent-otel, llm-agent-providers, llm-agent-customer-support`
  (does NOT include llm-agent-memory or llm-agent-contract). `rag→core` back-edge is the one cycle exemption
  (dep-currency-check.sh:59-71). **This is the gate that bites when llm-agent tags v0.8.0.**
- **Umbrella `cmd/depcheck`** (umbrella.yml:203): INFORMATIONAL — swallows stale failures. Not a gate.
  Also major-version-blind: `repoFromModulePath` strips `/vN` (main.go:132) + `detectStale` exact-equality
  (main.go:314), so a future v1 consumer of `llm-agent-memory` would be falsely flagged once v2.0.0 exists.
  Not a blocker now (zero consumers); noted as a known limitation.
- **`llm-agent-memory` main-PR CI** (`test.yml`) does NOT run any depcheck.

### Key correctness facts the sequence depends on (verified)
- **v2 `Manager` has NO `Lookup`** — exports HasKind/Add/Get/Update/Remove/Search/StatsAll/SearchAll/
  ListAll/Consolidate/Forget/ExportAll/ImportAll (manager.go:181-443). `tool.go:276` calls unexported
  `mgr.lookup`. → must ADD exported `Lookup(Kind)(Memory,error)` on the v2 Manager.
- **Metadata helpers = 8 total** (memory_item_helpers.go): `GetSource/SetSource/GetCategory/SetCategory/
  IsPinned/SetPinned/IsDisabled/SetDisabled` (lines 47-128). ALL go to the contract (pure Metadata ops,
  needed by both tool.go and v2 runtime).
- **coremem runtime refs span ≥6 files** (manager.go:31, memory_item_helpers.go:3, write_policy.go:25,
  recall_engine.go:21, sqlite_store.go:23, parallel_search.go:9) + bridges types_alias.go:7 & core_adapters.go,
  PLUS all `_test.go` importing coremem — NOT just `m8c_*`. The collapse PR must rewrite every one.
- **otel + customer-support pin `llm-agent v0.7.0`** (otel/go.mod:5, customer-support/go.mod:6) → the
  v0.8.0 stale-pin cascade.

---

## Phase 0 — Make `llm-agent-contract` releasable

### 0.1 Author `llm-agent-contract/memory/` subpackage (PR, stdlib-only, no go.sum)
- `types.go`: unified `MemoryItem` (no JSON tags), `SearchResult`, `Stats`, `ListPage`, `ListFilter`,
  `Scope` (+IsZero/Equal/Matches), `Source` (+consts), `Category` (+consts), `Kind`
  (+KindWorking/Episodic/Semantic), `ForgetStrategy` (+consts), `ConsolidateOptions`, `ForgetOptions`.
  **ALL 8 metadata helpers**: GetSource/SetSource/GetCategory/SetCategory/IsPinned/SetPinned/IsDisabled/SetDisabled.
- `snapshot.go`: `Snapshot`, `SnapshotItem` (preserve json tags), `SnapshotVersion=1`, `ImportMode`
  (+consts), `ImportReport`, `SnapshotStore`.
- `interfaces.go`: `Memory`, `Lister`, `Exporter`, `Importer`, and **`Manager`** interface — MUST include
  **`Lookup(Kind)(Memory,error)`** plus every method tool.go calls (Add/Search/SearchAll/Get/Update/Remove/
  Consolidate/Forget/StatsAll/ListAll/ExportAll/ImportAll/HasKind).
- `errors.go`: ErrNotFound, ErrEmptyQuery, ErrEmbedderRequired, ErrKindDisabled, snapshot errors.
  (v2 keeps ErrNoMemories/ErrRejectedByPolicy/ErrTierDisabled wrapping contractmem.ErrKindDisabled.)
- `embedder.go`: `type Embedder = llm.Embedder` (reuse llm-agent-contract/llm).
- Verify `GOWORK=off`: `go vet ./... && go test ./... && go build ./...`; assert `test ! -f go.sum`.

### 0.2 Tag contract `v0.1.0` from `release/v0.1.0` (zero replaces). Push tag.
> Front-loading `Lookup` + all 8 helpers here avoids a second contract tag (v0.2.0) mid-migration.

---

## Phase 1 — `llm-agent-memory`: collapse + drop inversion + rename `/v2` → tag v2.0.0 (ONE owner PR)
Codex confirmed 0.3-repin folds in here; collapse can also include the rename. (If you prefer smaller
diffs, split into 1a "collapse on old path, green" then 1b "rename /v2" — same end-state.)

- go.mod: `llm-agent-contract v0.0.0`→`v0.1.0`; **drop BOTH replaces** (`=> ../llm-agent-contract` and
  `=> ../llm-agent`); **drop `require llm-agent v0.7.0`**; set `module github.com/costa92/llm-agent-memory/v2`.
- Move remaining engine source from `llm-agent/memory/` not already native (core Manager extras, scope.go,
  profile.go, policy_hook.go Sanitizer, internal_score.go, persistence.go) into `llm-agent-memory/memory/`;
  reconcile dup with existing `engine_*.go` (keep one impl). **Exclude `tool.go`.**
- **Delete `types_alias.go` + `core_adapters.go`.** Rewrite EVERY `coremem.X`→`contractmem.X` across all
  ≥6 runtime files AND all `_test.go` (not just m8c_*). Converters/`AdaptCoreMemory` family disappear.
- **Add exported `Lookup(kind Kind)(contractmem.Memory,error)` on Manager** (wrap the existing private path)
  so `tool.go`'s contract `Manager` interface is satisfiable.
- Rewrite internal self-imports `github.com/costa92/llm-agent-memory/...`→`.../v2/...`. `version.go`:
  `Version="2.0.0"`. Update doc.go/README import examples to `/v2/memory`.
- Update the m8c_* guard tests + any surface guards in the same PR.
- Verify `GOWORK=off`: `go mod tidy && go vet ./... && go test ./... && go build ./...`.
- Tag `v2.0.0` from `release/v2.0.0` (zero replaces). Push tag.
> Does NOT trip dep-currency-check.sh (llm-agent-memory/contract not in its REPOS list). Zero external
> importers → path break harmless.

---

## Phase 2 — `llm-agent`: repin + amputate engine, keep `tool.go` → tag v0.8.0 (ONE owner PR)
- go.mod: `llm-agent-contract v0.0.0`→`v0.1.0`; drop `=> ../llm-agent-contract` replace. (Stays go.sum-free —
  contract is stdlib-only.)
- Delete engine source from `llm-agent/memory/` (working/episodic/semantic/manager/scoped_manager/scope/
  profile/policy_hook/internal_score/persistence/recall + _test.go).
- Keep `tool.go` in thin `llm-agent/memory` pkg, rewritten: `import contractmem ...`;
  `AsTool(mgr contractmem.Manager) agents.Tool`; replace `mgr.lookup`→`mgr.Lookup`, and SetPinned/SetDisabled
  → contractmem helpers.
- `builder.go:53`: `[]memory.SearchResult`→`[]contractmem.SearchResult`. Update context_test.go:247-286.
- Add back-compat alias shim `llm-agent/memory/aliases.go` (type aliases to contract types + const re-exports;
  NO constructor re-exports).
- Verify `GOWORK=off`: `go vet ./... && go test ./...`.
- Tag `v0.8.0` from `release/v0.8.0` (zero replaces). Push tag.

---

## Phase 3 — Drain the v0.8.0 stale-pin cascade (the trap codex caught) + contract laggards
**CRITICAL, immediately after tagging llm-agent v0.8.0:** repin every dep-currency REPOS member that pins
`llm-agent v0.7.0` → `v0.8.0`, else `llm-agent`'s umbrella CI (dep-currency-check.sh) goes RED on every
subsequent PR.
- Confirmed laggards: **`llm-agent-otel`** (go.mod:5), **`llm-agent-customer-support`** (go.mod:6).
- Verify whether `llm-agent-providers` pins llm-agent (it's in REPOS); repin if so. (`llm-agent-rag`→core is
  exempt — do not need to repin for this gate.)
- Each: owner PR bumping `llm-agent v0.7.0`→`v0.8.0`, `GOWORK=off go build/test`, drop any contract replace
  while there.
- **Contract laggards** (customer-support, flow, rag, providers, otel): repin `llm-agent-contract`
  `v0.0.0`→`v0.1.0` + drop replace, opportunistically — only blocks each repo's OWN release-precheck
  (not the dep-currency gate). Fold into the same PRs as the v0.8.0 repin where the repo overlaps.

---

## Phase 4 — go.work / go.work.sum (umbrella, local-only, gitignored)
- `/v2` rename needs no `go.work` edit (dir path unchanged). Run `go work sync` to refresh + regenerate
  `go.work.sum`. Inversion lives entirely in each go.mod; go.work is local-dev only (CI uses GOWORK=off).

---

## Forced ordering (minimal)
1. contract `v0.1.0` BEFORE any GOWORK=off consumer of `llm-agent-contract/memory`.
2. Phase 1 collapse BEFORE tagging `/v2`.
3. core amputation (Phase 2) only AFTER the contract surface exists.
4. v0.8.0 repin cascade (Phase 3) IMMEDIATELY AFTER tagging llm-agent v0.8.0.

Minimal safe sequence:
`contract v0.1.0` → `llm-agent-memory` [repin+collapse+Lookup+rename] one PR → tag `v2.0.0`
→ `llm-agent` [repin+amputate] one PR → tag `v0.8.0` → repin otel + customer-support (+providers?) to v0.8.0.

## Version-skew windows
| Window | When | Risk | Mitigation |
|---|---|---|---|
| W1 | v2.0.0 tagged, llm-agent v0.8.0 not | old engine + v2 both ship; no module imports both | dead code until Phase 2; not in dep-currency REPOS |
| **W2** | **llm-agent v0.8.0 tagged, otel/customer-support not repinned** | **llm-agent umbrella CI RED on every PR (dep-currency-check.sh strict-equality vs remote latest)** | **Phase 3 repins immediately; do them back-to-back with the tag** |
| W3 | contract laggards (5 repos) | each can't cut its own release (release-precheck) until repinned | off critical path; opportunistic |

## Risk register
1. Dual-MemoryItem divergence — LOW (verified identical). Add Phase-1 snapshot round-trip golden test; watch `_scope`/`_source`/`_category` keys.
2. Contract `Manager` not satisfiable without `Lookup` — RESOLVED by adding exported `Lookup` in Phase 1 + putting it in the v0.1.0 interface.
3. Incomplete contract helper set — RESOLVED: all 8 helpers in v0.1.0.
4. coremem rewrite scope understated — RESOLVED: Phase 1 rewrites all ≥6 runtime files + all coremem tests.
5. **v0.8.0 stale-pin cascade (codex's biggest catch)** — MEDIUM. Phase 3 repins otel + customer-support (+providers?) immediately after the v0.8.0 tag.
6. Contract stdlib-only purity — LOW. Subpkg imports only context/time/errors + sibling llm. Assert no go.sum.
7. depcheck major-version blindness — LOW/future. No v1 consumers today; flag before any v1 consumer appears.

## Rollback per phase
- P0: subpkg additive → revert PR; bad tag → ship v0.1.1 (don't delete). Repin rollbacks restore v0.0.0+replace (go.work instant).
- P1: revert the PR → restores old path + types_alias/core_adapters + require llm-agent. v2.0.0 tag orphaned, harmless (zero consumers); delete or supersede.
- P2: revert → restore llm-agent/memory engine + builder import; drop alias shim. Pre-tag free; post-tag ship v0.8.1.
- P3: revert the sibling repins (restore v0.7.0 pins) — but then re-accept the v0.8.0 cascade; prefer rolling forward.
- P4: local-only, gitignored; re-sync at will.

## Execution checklist (GOWORK=off for CI parity)
```
# 0.1 (llm-agent-contract): vet+test, assert no go.sum
GOWORK=off go vet ./... && GOWORK=off go test ./... && test ! -f go.sum
# 0.2 tag
git switch -c release/v0.1.0 && git tag v0.1.0 && git push origin v0.1.0
# 1 (llm-agent-memory: repin+collapse+Lookup+rename /v2)
GOWORK=off go mod tidy && GOWORK=off go vet ./... && GOWORK=off go test ./... && GOWORK=off go build ./...
git switch -c release/v2.0.0 && git tag v2.0.0 && git push origin v2.0.0
# 2 (llm-agent: repin+amputate, keep tool.go)
GOWORK=off go vet ./... && GOWORK=off go test ./...
git switch -c release/v0.8.0 && git tag v0.8.0 && git push origin v0.8.0
# 3 (IMMEDIATELY: repin otel + customer-support [+providers?] -> llm-agent v0.8.0)
#   per repo: edit go.mod, GOWORK=off go build ./... && go test ./..., open owner PR (auto-merges)
# verify llm-agent umbrella CI green afterward
# 4 (umbrella)
go work sync
```
