# M3: WritePolicy Interface and SQLite SnapshotStore Backend

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land milestone M3 of the `llm-agent-memory` subproject — Phase C of `docs/memory-roadmap.zh-CN.md`:

1. (Phase C-1) Introduce an explicit `WritePolicy` interface in the sibling that unifies the four documented write decisions (`user-saved`, `agent-inferred`, `reject`, `redact`) under a single `Decide(ctx, ProposedWrite) WritePolicyDecision` signature. Provide a `PolicyEnforcingMemory` wrapper that consumes a `WritePolicy` and translates decisions into core `coremem.Memory` operations (`Add` to the policy-chosen kind, redact item before write, or reject with `ErrRejectedByPolicy`). Also provide a thin `PolicyAdapter` that exposes the policy through the existing `coremem.Sanitizer` interface so callers wired to the core sanitizer chain see a policy-aware Add path.
2. (Phase C-2) Ship `SQLiteStore` — the first non-filesystem implementation of `coremem.SnapshotStore`. Uses the pure-Go SQLite driver `modernc.org/sqlite` (no CGO). All four interface methods (`Save`, `Load`, `Delete`, `List`) plus the optional `LoadKind` shape consumed by `coremem.Manager.ImportAll` (see `manager.go:369-391`) are implemented and tested.
3. (Phase C-2) Provide an in-code migrator for store schema v1 — applies idempotent `CREATE TABLE IF NOT EXISTS` + indexes on first `NewSQLiteStore` call, records the applied version in a `memory_store_schema` table, and refuses to operate against a future-version schema.
4. (Integration) Round-trip working/episodic/semantic snapshots produced by `(*coremem.Manager).ExportAll` through `SQLiteStore.Save` → `SQLiteStore.Load{,Kind}` → `(*coremem.Manager).ImportAll`. Lock the round-trip with a sibling-side integration test that mirrors the M1 `jsonRoundTripSnap` test pattern.
5. (Observability) Emit a new `EventWritePolicyDecided` from `PolicyEnforcingMemory.Add` covering all four decision verdicts. Reuses the existing `Observer` / `Event{Name, Attrs}` machinery from M2 — no new payload type.
6. (Hygiene) Bump version to `0.3.0`; add the first third-party dependency to the sibling's `go.mod` (`modernc.org/sqlite`) with a single-sentence justification in `CHANGELOG.md`; reconfirm the core `llm-agent` module's `scripts/stdlib-only-check.sh` still passes.

All test-driven (strict TDD throughout — every task starts from a failing test). The new SQLite dependency lives in `llm-agent-memory` **only**; the umbrella stdlib gate on `llm-agent/` remains untouched.

**Architecture:** Five new files + three modified files in `llm-agent-memory/memory/`. WritePolicy is sibling-owned (new interface, not a wrap of `coremem.Sanitizer`); the policy-enforcing wrapper consumes `*coremem.Manager` so it can route to any kind, and exposes a `Sanitize` adapter so legacy code paths that already use `WithSanitizer` get policy semantics for free. SQLite store reuses `modernc.org/sqlite` for portability (no CGO), and stores one row per `(key, kind)` carrying the raw JSON bytes that `coremem.SnapshotStore` already produces — the SQL surface is intentionally minimal.

- `write_policy.go` — new file. Declares `WritePolicy` interface, `ProposedWrite` input struct, `WritePolicyDecision` output struct, `Verdict` enum (`VerdictAccept` / `VerdictRedact` / `VerdictReject`), `PolicyEnforcingMemory` wrapper, `NewPolicyEnforcingMemory(inner *coremem.Manager, policy WritePolicy, opts ...Option)` constructor, `PolicyFunc` adapter, `PolicyAdapter` (implements `coremem.Sanitizer`), and the new event-name constant `EventWritePolicyDecided`.
- `write_policy_test.go` — new file. Failing-first tests for: interface shape; the four canonical verdict paths (accept, accept-redact, accept-reroute-to-different-kind, reject); observer emission for each; PolicyAdapter Sanitizer parity.
- `sqlite_store.go` — new file. Declares `SQLiteStore`, `NewSQLiteStore(dsn string) (*SQLiteStore, error)`, `Save` / `Load` / `LoadKind` / `Delete` / `List`, and the in-code migrator. Imports the side-effect-only `modernc.org/sqlite` driver registration.
- `sqlite_store_test.go` — new file. In-memory (`file::memory:?cache=shared`) tests for: migration idempotency; per-kind Save/Load round-trip; multi-kind Save then Load picking the per-kind file; Delete; List sorted-unique; conflict-on-future-schema rejection; concurrent-Save same-key serialization; integration round-trip via `coremem.Manager.ExportAll`/`ImportAll`.
- `sqlite_store_helpers_test.go` — new file. Test-only helpers: `newTempSQLiteStore(t *testing.T) (*SQLiteStore, func())` returning an in-memory store + a cleanup function; small `assertSnapshotEqual` deep-comparison helper that tolerates `time.Time` JSON round-trips.
- `version.go` — modified. Bump `Version` to `0.3.0`.
- `CHANGELOG.md` — modified. Add `## [0.3.0] - 2026-05-26` section documenting C-1 + C-2; explicitly call out the first third-party dep (`modernc.org/sqlite`) with the one-sentence justification.
- `go.mod` / `go.sum` — modified. Add `modernc.org/sqlite v1.41.4` (or the latest 1.41.x at execution time — the executor resolves the precise minor at `go get` time).
- `observer.go` — modified. Append the new `EventWritePolicyDecided` constant and the documented Attrs schema for it.

**Tech Stack:** Go 1.26.0; `github.com/costa92/llm-agent v0.7.0` (unchanged); first third-party dep is `modernc.org/sqlite` (pure-Go, no CGO, transitive pulls in `modernc.org/libc`, `modernc.org/mathutil`, etc. — all pure-Go); `database/sql` from stdlib; `encoding/json` for snapshot serialization; `testing` for tests; `context` + `sync` as needed.

---

## File Structure

| Path | Action | Responsibility |
|---|---|---|
| `llm-agent-memory/memory/write_policy.go` | Create | `WritePolicy` / `ProposedWrite` / `WritePolicyDecision` / `Verdict` enum; `PolicyEnforcingMemory` wrapper; `PolicyFunc` adapter; `PolicyAdapter` implementing `coremem.Sanitizer`; `EventWritePolicyDecided` Attrs documentation |
| `llm-agent-memory/memory/write_policy_test.go` | Create | TDD: interface declaration, four-verdict matrix, observer emission, kind reroute, PolicyAdapter parity |
| `llm-agent-memory/memory/sqlite_store.go` | Create | `SQLiteStore` + `NewSQLiteStore`; `Save` / `Load` / `LoadKind` / `Delete` / `List`; in-code migrator with schema-version table |
| `llm-agent-memory/memory/sqlite_store_test.go` | Create | TDD: migration idempotency; round-trip per-kind; LoadKind selection; Delete; List; future-schema rejection; concurrent-Save same-key; full coremem ExportAll → SQLite → ImportAll integration |
| `llm-agent-memory/memory/sqlite_store_helpers_test.go` | Create | Test-only `newTempSQLiteStore` helper + `assertSnapshotEqual` |
| `llm-agent-memory/memory/observer.go` | Modify | Append `EventWritePolicyDecided` constant; update Attrs schema doc block |
| `llm-agent-memory/memory/version.go` | Modify | Bump `Version` to `0.3.0` |
| `llm-agent-memory/CHANGELOG.md` | Modify | Add `## [0.3.0] - 2026-05-26` entry; call out new third-party dep |
| `llm-agent-memory/go.mod` | Modify | Add `modernc.org/sqlite` direct dep (executor runs `go get modernc.org/sqlite`) |
| `llm-agent-memory/go.sum` | Modify | Auto-populated by `go mod tidy` |

---

## Open Decisions Resolved in This Plan

- **SQLite driver choice — `modernc.org/sqlite` (pure-Go, CGO-free).** Tradeoff: `github.com/mattn/go-sqlite3` is faster (binds the official SQLite C lib) but requires CGO, blocks cross-compile, and breaks reproducible builds on hosts without a C toolchain. `modernc.org/sqlite` is a pure-Go transpilation of the SQLite C source — slower for write-heavy benchmarks, but: (a) the memory subsystem snapshot path is low-frequency (one Save per session close or scheduled snapshot, not per-Add); (b) we keep the umbrella's "no CGO in CI" property; (c) eliminates the cross-platform complexity that the customer-support and rag siblings would otherwise inherit. Decision: `modernc.org/sqlite`. Justification documented in CHANGELOG.
- **Schema migration strategy — in-code migrator.** External `.sql` files would force a build-tag-gated file-embed + a CI step to verify they match the in-code shape. For schema v1 (one migration, two tables) the in-code path is simpler and self-contained. We add a `memory_store_schema` table that records the applied version; future versions append entries with a wall-clock timestamp. Migration runs lazily on the first `NewSQLiteStore` call against a given DSN and is a no-op if the current version already matches.
- **Build on existing `coremem.SnapshotStore` interface, do NOT define a new one.** `coremem.SnapshotStore` in `llm-agent/memory/persistence.go:171-176` is already pluggable — the FilesystemStore is one impl, SQLiteStore is just another. We additionally implement the optional `LoadKind(ctx, key, kind) (Snapshot, error)` method (`manager.go:369-371` type-asserts on this method) so `Manager.ImportAll` picks the per-kind snapshot deterministically. Pattern matches M1/M2: wrap-and-extend, never fork.
- **WritePolicy location — sibling-owned in `llm-agent-memory/memory/write_policy.go`.** Rationale: WritePolicy is a *richer* contract than core `Sanitizer` (which can only Add-time accept-or-reject-or-mutate). Specifically, WritePolicy adds (a) **importance assignment** (the verdict carries the final `Importance`), (b) **tag enrichment** (final `Tags`), (c) **kind routing** (the verdict carries the final `Kind`, so a user-typed "remember this" can route to Episodic while an inferred fact can route to Semantic), and (d) **structured reason** (the verdict carries a `Reason` string used for observability). None of these fit cleanly into `Sanitizer`'s `(MemoryItem, bool, error)` return. The `PolicyAdapter` lets the inverse hold: a WritePolicy *can* always satisfy the narrower `Sanitizer` contract, so callers wired to `WithSanitizer` get policy semantics for free.
- **WritePolicy signature — `Decide(ctx, ProposedWrite) WritePolicyDecision`.**
  - `ProposedWrite` struct: `Kind coremem.Kind`, `Item coremem.MemoryItem`, `Source WriteSource` (enum: `SourceUserSaved` / `SourceAgentInferred` / `SourceSystem`), `Hint map[string]any` (caller-supplied free-form, never inspected by core).
  - `WritePolicyDecision` struct: `Verdict Verdict` (enum: `VerdictAccept` / `VerdictRedact` / `VerdictReject`), `Kind coremem.Kind` (may differ from input), `Item coremem.MemoryItem` (final form to write; ignored for `VerdictReject`), `Reason string`.
  - No `error` in the verdict struct — implementations that need to surface a true Go error MUST return a `Verdict` of `VerdictReject` with `Reason = "policy:" + err.Error()`. Rationale: keeps the verdict a plain value and lets `PolicyEnforcingMemory` cleanly translate to `ErrRejectedByPolicy`. Implementations that *fail to make a decision* (e.g., panicked downstream) are not the policy interface's responsibility; the wrapper recovers and converts to a Go `error` separately.
- **Verdict semantics matrix:**
  - `VerdictAccept`: write the decision's `Item` to the decision's `Kind` verbatim. Decision's `Item.Importance` and `Item.Tags` override the input's values. Decision's `Kind` may reroute (e.g., Working → Episodic) — the wrapper calls `mgr.Add(ctx, decision.Kind, decision.Item)`.
  - `VerdictRedact`: identical to `VerdictAccept` except observability marks the write as redacted. Provided as a distinct verdict so consumers can count `redact_total` vs `accept_total` independently.
  - `VerdictReject`: do not write. `PolicyEnforcingMemory.Add` returns the sentinel error `ErrRejectedByPolicy = errors.New("memory: item rejected by write policy")` (sibling-local; aliased into the godoc so consumers can `errors.Is` it). Decision's `Reason` flows into the observer event.
- **`ErrRejectedByPolicy` vs core's `coremem.ErrRejectedByPolicy` — alias the core sentinel, do NOT define a new one.** Core already has `coremem.ErrRejectedByPolicy` (see `policy_hook.go:39`). We export `ErrRejectedByPolicy = coremem.ErrRejectedByPolicy` from this package as a type alias so consumers can `errors.Is(err, memory.ErrRejectedByPolicy)` without dual-import. This avoids the two-sentinel trap M2 narrowly avoided with `ErrManagerRequired` (where we *did* split because the constructor surface differs).
- **PolicyEnforcingMemory wraps `*coremem.Manager`, not `coremem.Memory`.** Why: the policy can reroute to a different kind, which only `Manager` exposes through its three-kind dispatch. Wrapping a single `Memory` would force one-policy-per-kind and lose the routing affordance.
- **PolicyAdapter API.** `func (p PolicyAdapter) Sanitize(ctx context.Context, kind coremem.Kind, item coremem.MemoryItem) (coremem.MemoryItem, bool, error)` — translates the decision: `VerdictAccept` or `VerdictRedact` → `(decision.Item, true, nil)`; `VerdictReject` → `(coremem.MemoryItem{}, false, nil)`; if the policy chose a *different* `Kind` than the input, PolicyAdapter returns `(coremem.MemoryItem{}, false, ErrPolicyKindRerouteUnsupported)` because `Sanitizer` cannot reroute. Consumers wanting reroute semantics must use `PolicyEnforcingMemory` directly. Documented as a known sharp edge.
- **`EventWritePolicyDecided` Attrs schema (v0.3.0, frozen):** `{"verdict": string, "input_kind": coremem.Kind, "decided_kind": coremem.Kind, "source": string, "reason": string}`. New keys may be added in future versions; existing keys are never removed/renamed (same compatibility contract as the M2 events). Emitted *after* the underlying `Add` (or `Reject`) returns. On Add failure (`mgr.Add` returns non-nil err) we DO NOT emit — same convention as M2 (callers count their own errors).
- **SQLite DSN convention.** `NewSQLiteStore(dsn)` accepts any modernc.org/sqlite DSN. Tests use the shared in-memory form `file::memory:?cache=shared` so concurrent `*sql.DB` connections see the same data. Production callers pass a file path (`file:/var/lib/llm-agent-memory/store.db?_pragma=journal_mode(WAL)`). We do NOT prescribe pragmas in the constructor — callers control durability/perf via the DSN. The single behavior we DO enforce: `db.SetMaxOpenConns(1)` when the DSN contains `:memory:` (a hard requirement for shared-in-memory tests; multiple open conns to `:memory:` would each get a private database).
- **Schema v1 — exactly two tables:**
  ```sql
  CREATE TABLE IF NOT EXISTS memory_store_schema (
      version   INTEGER PRIMARY KEY,
      applied_at TEXT NOT NULL
  );
  CREATE TABLE IF NOT EXISTS memory_snapshots (
      key        TEXT NOT NULL,
      kind       TEXT NOT NULL,
      snapshot_json BLOB NOT NULL,
      updated_at TEXT NOT NULL,
      PRIMARY KEY (key, kind)
  );
  CREATE INDEX IF NOT EXISTS idx_memory_snapshots_key ON memory_snapshots(key);
  ```
  - `key` is sanitized identically to `FilesystemStore.sanitizeKey` (replace non-`[a-zA-Z0-9_-]` with `_`, empty → `_`) so the same caller key maps to the same row across stores. We reuse the sanitizer rule but NOT the function — we inline a copy with a `// keep in sync with coremem persistence.go sanitizeKey` comment.
  - `snapshot_json` carries the bytes produced by `json.Marshal(snap)` verbatim. `Load` decodes via `json.Unmarshal` into a `coremem.Snapshot`.
  - `updated_at` is `time.Now().UTC().Format(time.RFC3339Nano)`; informational only — never read by Save/Load.
- **Save semantics: idempotent UPSERT.** `INSERT INTO memory_snapshots (...) VALUES (...) ON CONFLICT(key,kind) DO UPDATE SET snapshot_json=excluded.snapshot_json, updated_at=excluded.updated_at`. Matches `FilesystemStore.Save`'s "last write wins" guarantee.
- **Load behavior parity with FilesystemStore.** `Load(key)` returns the first snapshot found in working → episodic → semantic order; `LoadKind(key, kind)` returns the row for that exact key/kind. Missing row returns an error wrapping `os.ErrNotExist` so `coremem.Manager.ImportAll`'s `errors.Is(err, os.ErrNotExist)` check (`manager.go:384`) keeps working. We use `fmt.Errorf("memory: sqlite store: no snapshot for key %q kind %q: %w", key, kind, os.ErrNotExist)` — same shape as FilesystemStore.
- **Delete semantics.** `DELETE FROM memory_snapshots WHERE key = ?` — removes all kinds at once; matches FilesystemStore behavior. Missing rows are not an error.
- **List semantics.** `SELECT DISTINCT key FROM memory_snapshots ORDER BY key ASC` — returns sorted unique keys; matches FilesystemStore.
- **Concurrency.** `*sql.DB` is goroutine-safe; we add no additional locking. The single behavior we enforce is `SetMaxOpenConns(1)` for `:memory:` DSNs (see above). For file-based DSNs we leave the pool defaults (limited by SQLite's writer-lock semantics; readers parallelize fine under WAL mode if the caller sets that pragma in the DSN).
- **Migrator atomicity.** Wrap the migrator in a transaction. Each migration step is `tx.Exec` of one statement; if any step fails we `tx.Rollback` and return the underlying error wrapped with `memory: sqlite store migrate v%d:`. Successful migration ends with `INSERT INTO memory_store_schema (version, applied_at) VALUES (?, ?)` and `tx.Commit`.
- **Future-version refusal.** On `NewSQLiteStore`, after running the migrator (which is idempotent at the current target version), we read `SELECT MAX(version) FROM memory_store_schema`. If the result exceeds the target version constant (`SchemaVersion = 1`), return `ErrSchemaVersionAhead = errors.New("memory: sqlite store schema ahead of code")`. This prevents an older binary from silently downgrading state written by a newer one.
- **Version bump:** `0.2.0 → 0.3.0`. M3 is additive — the new sentinel `ErrRejectedByPolicy` aliases an existing core sentinel; no signature breaks. A minor bump is right because we introduce the first third-party dependency.
- **Test infrastructure for SQLite.** We use the shared in-memory DSN `file::memory:?cache=shared` with `db.SetMaxOpenConns(1)`. Each test that needs an isolated store calls `newTempSQLiteStore(t)`, which appends a unique URL-encoded `name=` query parameter so two tests in the same `go test ./memory -run ...` invocation cannot collide. The helper returns a cleanup func that calls `Close()` and is registered via `t.Cleanup`. No filesystem writes during tests.
- **No `database/sql` "ping on construct" anti-pattern.** `NewSQLiteStore` opens the DB, runs the migrator (which executes statements — that's an implicit ping), then returns. No explicit `db.PingContext` call. Rationale: the migrator already exercises the connection; double-pinging is noise.
- **Observer call site for `EventWritePolicyDecided`.** Always emitted (in *both* the C-1 success path and the reject path). The Reject case is the only one in the entire sibling that emits on a non-success-of-underlying-call path; documented as an intentional exception because the rejection IS the success outcome of the policy decision.
- **No `RecallEngine` work in M3.** The C-1 spec mentions "let upper layer collapse remember/infer/ignore into one place"; we honor that exactly through the policy interface. The unified-recall facade is M4's job — we do not preview it here.
- **Constructor option pattern alignment.** `NewPolicyEnforcingMemory(inner *coremem.Manager, policy WritePolicy, opts ...Option)` and `NewSQLiteStore(dsn string)` use the same shared `Option`/`config`/`newConfig` machinery from M2's `observer.go` (do NOT redefine). `NewSQLiteStore` does not currently take an Observer option (the SnapshotStore interface is consumed by `coremem.Manager.ExportAll`, which already gets snapshot events emitted by `Consolidator.ExportAll` in M2); if a future need arises, `WithObserver` slots in trivially.

---

## Sequencing Rules

- **Every task is strict TDD.** Failing test → minimal impl → green test → commit. There is no scaffolding-only block; even Task 1 (interface declaration) starts from a failing test that references the not-yet-existing types.
- **Phase order: C-1 first (Tasks 1–6), then C-2 (Tasks 7–12), then integration + housekeeping (Tasks 13–15).** Rationale: C-1 changes no `go.mod`; C-2 introduces the first third-party dep. Landing C-1 first keeps the dep blast radius contained to a single commit window in case `go mod tidy` surfaces a surprise.
- **Commit cadence:** every task ends in exactly one commit. Step 5 (or the last numbered Step) is always "Commit". Use `git add` with explicit file paths — never `git add -A` (sibling has its own `.git`; umbrella `.gitignore` excludes `/llm-agent-memory/`).
- **All commit prefixes use `(memory)` scope** to match M1/M2 convention.
- **Sibling commit topology:** every commit goes to the sibling repo's `origin/main` — `cd llm-agent-memory && git add ... && git commit -m ... && git push origin main`. The umbrella does NOT see these commits; the umbrella reads the sibling via its own pinned tag once Task 15 lands `v0.3.0`.
- **Tag format unprefixed:** `v0.3.0` (matches the existing `v0.1.0` and `v0.2.0` tags in the sibling).
- **Before adding the third-party dep (Task 8) the executor MUST run `go doc` to confirm the modernc.org/sqlite driver-name string** (`sqlite`) and the package path (`modernc.org/sqlite`) match the import in `sqlite_store.go`. This is the M2 lesson "verify upstream interfaces with `go doc` before writing test shims" applied to dependency hygiene.
- **`len(page.Items) > 0` guards are forbidden inside iteration helpers.** M2 Task 3 I-1 found that a `len > 0` guard around an `append` introduced a subtle non-empty-key invariant on the result map. We do not need any iteration helpers in M3 (snapshots are loaded as a whole), but the rule applies to any future addition.
- **Do not stutter error wraps.** Bad: `memory: sqlite store: memory: sqlite: ...`. Good: `memory: sqlite store: <reason>: %w`. Each error is wrapped exactly once at the call site that adds context. The migrator wraps with `memory: sqlite store migrate v%d:`; `Save`/`Load`/`Delete`/`List` wrap with `memory: sqlite store <op>:`.
- **Use the `observer()` accessor consistently at emit sites** — same convention as M2. PolicyEnforcingMemory's emit calls go through `m.observer()`, not `m.cfg.observer`. The exception is the internal `emit(o, name, attrs)` helper which takes an `Observer` value directly.
- **`reflect.DeepEqual` on snapshots is forbidden in tests.** Snapshots contain `time.Time` fields that lose monotonic clock readings through JSON round-trip; M2's Task 11 BLOCKED finding was on this exact pattern. Use the `assertSnapshotEqual` helper that compares `Items` field-by-field, treating `time.Time` with `.Equal()` and `Vector` with element-wise `==`.

---

# Phase C-1 — WritePolicy Interface (Tasks 1–6)

## Task 1: Declare the failing `WritePolicy` interface shape

**Files:**
- Create: `llm-agent-memory/memory/write_policy_test.go`

- [ ] **Step 1: Write the failing test that exercises the public surface**

  Create `llm-agent-memory/memory/write_policy_test.go`:

  ```go
  package memory

  import (
  	"context"
  	"testing"

  	coremem "github.com/costa92/llm-agent/memory"
  )

  // TestWritePolicy_InterfaceSurface_Compiles is a compile-time
  // assertion. If the types in this file go missing or change shape,
  // this test will fail to compile, which is the exact signal we want.
  func TestWritePolicy_InterfaceSurface_Compiles(t *testing.T) {
  	var _ WritePolicy = PolicyFunc(func(_ context.Context, in ProposedWrite) WritePolicyDecision {
  		return WritePolicyDecision{
  			Verdict: VerdictAccept,
  			Kind:    in.Kind,
  			Item:    in.Item,
  		}
  	})

  	// All three verdicts must be distinct constants.
  	if VerdictAccept == VerdictRedact || VerdictAccept == VerdictReject || VerdictRedact == VerdictReject {
  		t.Errorf("verdicts must be distinct: accept=%v redact=%v reject=%v",
  			VerdictAccept, VerdictRedact, VerdictReject)
  	}

  	// All three sources must be distinct.
  	if SourceUserSaved == SourceAgentInferred || SourceUserSaved == SourceSystem || SourceAgentInferred == SourceSystem {
  		t.Errorf("sources must be distinct: user=%v agent=%v system=%v",
  			SourceUserSaved, SourceAgentInferred, SourceSystem)
  	}

  	// ProposedWrite and WritePolicyDecision must accept the documented field set.
  	in := ProposedWrite{
  		Kind:   coremem.KindWorking,
  		Item:   coremem.MemoryItem{Content: "x"},
  		Source: SourceUserSaved,
  		Hint:   map[string]any{"channel": "chat"},
  	}
  	out := WritePolicyDecision{
  		Verdict: VerdictAccept,
  		Kind:    coremem.KindEpisodic,
  		Item:    in.Item,
  		Reason:  "promote-user-saved",
  	}
  	if out.Verdict != VerdictAccept || out.Kind != coremem.KindEpisodic {
  		t.Errorf("WritePolicyDecision did not round-trip: %+v", out)
  	}
  }
  ```

- [ ] **Step 2: Run the test to confirm compile failure**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestWritePolicy_InterfaceSurface_Compiles -v`
  Expected: compile error — `undefined: WritePolicy`, `undefined: PolicyFunc`, `undefined: ProposedWrite`, `undefined: WritePolicyDecision`, `undefined: VerdictAccept`, etc.

- [ ] **Step 3: Skip — interface impl lands in Task 2.**

- [ ] **Step 4: Skip.**

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/write_policy_test.go
  git commit -m "test(memory): declare WritePolicy interface-surface compile assertion (C-1)"
  git push origin main
  ```

---

## Task 2: Implement `WritePolicy` interface, types, and `PolicyFunc` adapter

**Files:**
- Create: `llm-agent-memory/memory/write_policy.go`

- [ ] **Step 1: Write `llm-agent-memory/memory/write_policy.go` with just the interface + types**

  ```go
  // Package memory — write_policy.go declares the Phase C-1 write-policy
  // surface: a single interface that consumes a ProposedWrite and emits
  // a WritePolicyDecision covering accept / redact / reject. The
  // interface intentionally lifts the four decisions documented in
  // docs/memory-roadmap.zh-CN.md §4.3 (item C-1) — user-saved,
  // agent-inferred, reject, redact — into a single funnel so upper
  // layers don't reinvent them per-feature.
  //
  // WritePolicy is NOT a wrap of coremem.Sanitizer. Sanitizer can only
  // accept-or-reject-or-mutate (see policy_hook.go:23). WritePolicy
  // additionally carries the final Kind (so a user-typed "remember
  // this" can reroute to Episodic), the final Importance + Tags, and
  // a structured Reason field used by the EventWritePolicyDecided
  // observer event.
  //
  // For callers wired to the existing coremem.Sanitizer chain,
  // PolicyAdapter lets a WritePolicy satisfy the narrower interface —
  // see PolicyAdapter godoc for the rerouting limitation.
  package memory

  import (
  	"context"
  	"errors"

  	coremem "github.com/costa92/llm-agent/memory"
  )

  // WriteSource tags the origin of a ProposedWrite. Policies use it to
  // pick different rules for user-typed memories vs agent-inferred
  // memories. Add new variants by appending; never renumber.
  type WriteSource string

  const (
  	// SourceUserSaved means a human explicitly asked to remember this.
  	// Policies typically promote these directly to Episodic.
  	SourceUserSaved WriteSource = "user_saved"
  	// SourceAgentInferred means the agent inferred a fact from
  	// conversation. Policies typically gate these by importance.
  	SourceAgentInferred WriteSource = "agent_inferred"
  	// SourceSystem means a background process (consolidator,
  	// importer, migration) is writing. Policies usually pass through.
  	SourceSystem WriteSource = "system"
  )

  // Verdict is the policy's decision on a ProposedWrite.
  type Verdict string

  const (
  	// VerdictAccept means the wrapper writes Decision.Item to
  	// Decision.Kind verbatim.
  	VerdictAccept Verdict = "accept"
  	// VerdictRedact means the wrapper writes Decision.Item to
  	// Decision.Kind — identical write path as VerdictAccept, but
  	// observability distinguishes "this was a redaction" from "this was
  	// a clean accept" so consumers can count them separately.
  	VerdictRedact Verdict = "redact"
  	// VerdictReject means the wrapper does NOT write; Add returns
  	// ErrRejectedByPolicy. Decision.Reason flows into the observer
  	// event and the wrapped error's context if the caller logs it.
  	VerdictReject Verdict = "reject"
  )

  // ProposedWrite is the input to WritePolicy.Decide. Kind is the
  // caller's intended kind; the policy may override it via the returned
  // Decision.Kind. Item is the caller's intended payload; the policy
  // may mutate, redact, or replace it. Hint is a free-form per-call
  // bag of caller context (e.g. the chat-channel ID); core never
  // inspects it.
  type ProposedWrite struct {
  	Kind   coremem.Kind
  	Item   coremem.MemoryItem
  	Source WriteSource
  	Hint   map[string]any
  }

  // WritePolicyDecision is the output of WritePolicy.Decide. Item and
  // Kind are ignored when Verdict == VerdictReject. Reason is opaque
  // to core (flows into observer events and reject errors).
  type WritePolicyDecision struct {
  	Verdict Verdict
  	Kind    coremem.Kind
  	Item    coremem.MemoryItem
  	Reason  string
  }

  // WritePolicy is the single funnel for write decisions. Implementations
  // MUST be goroutine-safe (Decide is called from arbitrary caller
  // goroutines). Implementations MUST NOT block on external I/O on the
  // hot path — the wrapper does not budget for that today.
  type WritePolicy interface {
  	Decide(ctx context.Context, in ProposedWrite) WritePolicyDecision
  }

  // PolicyFunc adapts a plain function to the WritePolicy interface.
  type PolicyFunc func(ctx context.Context, in ProposedWrite) WritePolicyDecision

  // Decide calls f.
  func (f PolicyFunc) Decide(ctx context.Context, in ProposedWrite) WritePolicyDecision {
  	return f(ctx, in)
  }

  // ErrRejectedByPolicy is returned by PolicyEnforcingMemory.Add when
  // the configured WritePolicy returns VerdictReject. It aliases the
  // identically-named core sentinel so consumers can errors.Is(err,
  // memory.ErrRejectedByPolicy) without dual-importing the core
  // package. The alias is intentional: there is exactly one rejection
  // condition across the stack.
  var ErrRejectedByPolicy = coremem.ErrRejectedByPolicy

  // ErrPolicyKindRerouteUnsupported is returned by PolicyAdapter.Sanitize
  // when the wrapped policy returns a Decision.Kind that differs from
  // the input kind. Sanitizer cannot reroute (its return triple does
  // not carry a kind), so PolicyAdapter rejects rather than silently
  // dropping the reroute. Consumers wanting reroute semantics must
  // use PolicyEnforcingMemory directly.
  var ErrPolicyKindRerouteUnsupported = errors.New("memory: policy adapter cannot reroute kind via Sanitizer interface")
  ```

- [ ] **Step 2: Run the Task 1 test to confirm it now compiles + passes**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestWritePolicy_InterfaceSurface_Compiles -v`
  Expected: PASS.

- [ ] **Step 3: Run the full sibling suite to confirm no regression**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -v`
  Expected: every existing M0–M2 test still passes.

- [ ] **Step 4: Skip.**

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/write_policy.go
  git commit -m "feat(memory): add WritePolicy interface + Verdict/Source enums + PolicyFunc (C-1)"
  git push origin main
  ```

---

## Task 3: `PolicyEnforcingMemory` wrapper — accept and reject paths

**Files:**
- Modify: `llm-agent-memory/memory/write_policy.go`
- Modify: `llm-agent-memory/memory/write_policy_test.go` (append)

- [ ] **Step 1: Append failing tests covering accept and reject**

  Append to `llm-agent-memory/memory/write_policy_test.go`:

  ```go
  func TestPolicyEnforcingMemory_Add_VerdictAccept_WritesToDecidedKind(t *testing.T) {
  	mgr := newCoreManager(t)
  	policy := PolicyFunc(func(_ context.Context, in ProposedWrite) WritePolicyDecision {
  		// Reroute: user-saved memories go to Episodic regardless of input kind.
  		return WritePolicyDecision{
  			Verdict: VerdictAccept,
  			Kind:    coremem.KindEpisodic,
  			Item:    in.Item,
  			Reason:  "promote-user-saved",
  		}
  	})
  	pem, err := NewPolicyEnforcingMemory(mgr, policy)
  	if err != nil {
  		t.Fatalf("NewPolicyEnforcingMemory: %v", err)
  	}

  	ctx := context.Background()
  	id, err := pem.Add(ctx, ProposedWrite{
  		Kind:   coremem.KindWorking, // caller asked for Working...
  		Item:   coremem.MemoryItem{Content: "remember me"},
  		Source: SourceUserSaved,
  	})
  	if err != nil {
  		t.Fatalf("Add: %v", err)
  	}
  	if id == "" {
  		t.Fatal("Add returned empty id")
  	}

  	// Confirm the item landed in Episodic (the decided kind), not Working.
  	got, err := mgr.Get(ctx, coremem.KindEpisodic, id)
  	if err != nil {
  		t.Fatalf("Get from Episodic: %v", err)
  	}
  	if got.Content != "remember me" {
  		t.Errorf("Episodic item Content = %q, want %q", got.Content, "remember me")
  	}
  }

  func TestPolicyEnforcingMemory_Add_VerdictReject_ReturnsErrRejectedByPolicy(t *testing.T) {
  	mgr := newCoreManager(t)
  	policy := PolicyFunc(func(_ context.Context, _ ProposedWrite) WritePolicyDecision {
  		return WritePolicyDecision{Verdict: VerdictReject, Reason: "test-reject"}
  	})
  	pem, err := NewPolicyEnforcingMemory(mgr, policy)
  	if err != nil {
  		t.Fatalf("NewPolicyEnforcingMemory: %v", err)
  	}

  	_, err = pem.Add(context.Background(), ProposedWrite{
  		Kind: coremem.KindWorking,
  		Item: coremem.MemoryItem{Content: "blocked"},
  	})
  	if err == nil {
  		t.Fatal("Add returned nil error on VerdictReject")
  	}
  	if !errors.Is(err, ErrRejectedByPolicy) {
  		t.Errorf("Add err = %v, want errors.Is ErrRejectedByPolicy", err)
  	}
  }
  ```

  Add the import for `"errors"` to the test file if not already present.

- [ ] **Step 2: Run to confirm failure**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run 'TestPolicyEnforcingMemory_Add_Verdict(Accept|Reject)' -v`
  Expected: compile error — `undefined: NewPolicyEnforcingMemory`.

- [ ] **Step 3: Implement `PolicyEnforcingMemory` in `write_policy.go`**

  Append to `write_policy.go`:

  ```go
  // PolicyEnforcingMemory wraps a *coremem.Manager and routes every Add
  // through the configured WritePolicy. The wrapper does not implement
  // the coremem.Memory interface — its Add takes a ProposedWrite (with
  // Source + Hint context) rather than a bare MemoryItem, because the
  // policy contract is richer than coremem.Memory.Add.
  //
  // Read paths (Get, Search, Update, Remove, Stats, ListAll) are not
  // exposed by this wrapper: policy enforcement is an Add-time concern,
  // and callers needing reads operate on the underlying *coremem.Manager
  // directly. This mirrors the M2 Consolidator pattern (writes only).
  type PolicyEnforcingMemory struct {
  	mgr    *coremem.Manager
  	policy WritePolicy
  	cfg    *config
  }

  // ErrPolicyEnforcingManagerRequired is returned when the inner
  // *coremem.Manager is nil.
  var ErrPolicyEnforcingManagerRequired = errors.New("memory: policy-enforcing memory requires manager")

  // ErrPolicyRequired is returned when the WritePolicy is nil.
  var ErrPolicyRequired = errors.New("memory: policy-enforcing memory requires a non-nil WritePolicy")

  // NewPolicyEnforcingMemory wraps an existing *coremem.Manager with the
  // given policy. opts is the shared functional-option list from
  // observer.go (e.g., WithObserver).
  func NewPolicyEnforcingMemory(inner *coremem.Manager, policy WritePolicy, opts ...Option) (*PolicyEnforcingMemory, error) {
  	if inner == nil {
  		return nil, ErrPolicyEnforcingManagerRequired
  	}
  	if policy == nil {
  		return nil, ErrPolicyRequired
  	}
  	return &PolicyEnforcingMemory{
  		mgr:    inner,
  		policy: policy,
  		cfg:    newConfig(opts),
  	}, nil
  }

  // observer exposes the configured observer for in-package call sites
  // and tests. Package-private — callers should not depend on the
  // accessor.
  func (p *PolicyEnforcingMemory) observer() Observer { return p.cfg.observer }

  // Add dispatches in through the WritePolicy. On VerdictAccept and
  // VerdictRedact, the decided Item is written to the decided Kind.
  // On VerdictReject, ErrRejectedByPolicy is returned. The EventWrite-
  // PolicyDecided observer event is emitted in all three cases.
  func (p *PolicyEnforcingMemory) Add(ctx context.Context, in ProposedWrite) (string, error) {
  	decision := p.policy.Decide(ctx, in)
  	switch decision.Verdict {
  	case VerdictAccept, VerdictRedact:
  		id, err := p.mgr.Add(ctx, decision.Kind, decision.Item)
  		if err != nil {
  			return "", err
  		}
  		emit(p.observer(), EventWritePolicyDecided, map[string]any{
  			"verdict":      string(decision.Verdict),
  			"input_kind":   in.Kind,
  			"decided_kind": decision.Kind,
  			"source":       string(in.Source),
  			"reason":       decision.Reason,
  		})
  		return id, nil
  	case VerdictReject:
  		emit(p.observer(), EventWritePolicyDecided, map[string]any{
  			"verdict":      string(decision.Verdict),
  			"input_kind":   in.Kind,
  			"decided_kind": in.Kind, // no reroute happened; mirror input
  			"source":       string(in.Source),
  			"reason":       decision.Reason,
  		})
  		return "", ErrRejectedByPolicy
  	default:
  		return "", errors.New("memory: write policy returned unknown verdict")
  	}
  }
  ```

- [ ] **Step 4: Run the two new tests**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run 'TestPolicyEnforcingMemory_Add_Verdict(Accept|Reject)' -v`
  Expected: both PASS. (`EventWritePolicyDecided` is undefined here; we accept the compile of the file *will* fail. Move the emit calls to use `EventWritePolicyDecided` only after Task 4 declares the constant.)

  **Sequencing note:** Step 3 above introduces `emit(p.observer(), EventWritePolicyDecided, ...)` which references a constant added in Task 4. To keep this task self-contained, temporarily change both `emit(...)` calls to use the literal string `"memory_write_policy_decided"` and add a `// FIXME(M3-Task4): swap for EventWritePolicyDecided constant` comment immediately above each. Task 4 will switch them to the constant and remove the FIXMEs.

  Re-run after the literal-string substitution. Both tests must PASS.

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/write_policy.go memory/write_policy_test.go
  git commit -m "feat(memory): add PolicyEnforcingMemory with Accept/Reject paths (C-1)"
  git push origin main
  ```

---

## Task 4: Add `EventWritePolicyDecided` constant + observer-emission tests

**Files:**
- Modify: `llm-agent-memory/memory/observer.go`
- Modify: `llm-agent-memory/memory/write_policy.go`
- Modify: `llm-agent-memory/memory/write_policy_test.go` (append)

- [ ] **Step 1: Append failing tests asserting observer emission**

  Append to `write_policy_test.go`:

  ```go
  func TestPolicyEnforcingMemory_Add_EmitsEventWritePolicyDecidedOnAccept(t *testing.T) {
  	rec := &recordingObserver{}
  	mgr := newCoreManager(t)
  	policy := PolicyFunc(func(_ context.Context, in ProposedWrite) WritePolicyDecision {
  		return WritePolicyDecision{Verdict: VerdictAccept, Kind: in.Kind, Item: in.Item, Reason: "ok"}
  	})
  	pem, err := NewPolicyEnforcingMemory(mgr, policy, WithObserver(rec))
  	if err != nil {
  		t.Fatalf("NewPolicyEnforcingMemory: %v", err)
  	}
  	if _, err := pem.Add(context.Background(), ProposedWrite{
  		Kind: coremem.KindWorking, Item: coremem.MemoryItem{Content: "x"}, Source: SourceUserSaved,
  	}); err != nil {
  		t.Fatalf("Add: %v", err)
  	}
  	got := rec.snapshot()
  	if len(got) != 1 {
  		t.Fatalf("expected exactly 1 event, got %d: %+v", len(got), got)
  	}
  	if got[0].Name != EventWritePolicyDecided {
  		t.Errorf("event Name = %q, want %q", got[0].Name, EventWritePolicyDecided)
  	}
  	if v, _ := got[0].Attrs["verdict"].(string); v != string(VerdictAccept) {
  		t.Errorf("verdict = %v, want %q", got[0].Attrs["verdict"], VerdictAccept)
  	}
  	if s, _ := got[0].Attrs["source"].(string); s != string(SourceUserSaved) {
  		t.Errorf("source = %v, want %q", got[0].Attrs["source"], SourceUserSaved)
  	}
  	if r, _ := got[0].Attrs["reason"].(string); r != "ok" {
  		t.Errorf("reason = %v, want %q", got[0].Attrs["reason"], "ok")
  	}
  }

  func TestPolicyEnforcingMemory_Add_EmitsEventWritePolicyDecidedOnRedact(t *testing.T) {
  	rec := &recordingObserver{}
  	mgr := newCoreManager(t)
  	policy := PolicyFunc(func(_ context.Context, in ProposedWrite) WritePolicyDecision {
  		redacted := in.Item
  		redacted.Content = "[REDACTED]"
  		return WritePolicyDecision{Verdict: VerdictRedact, Kind: in.Kind, Item: redacted, Reason: "pii"}
  	})
  	pem, err := NewPolicyEnforcingMemory(mgr, policy, WithObserver(rec))
  	if err != nil {
  		t.Fatalf("NewPolicyEnforcingMemory: %v", err)
  	}
  	if _, err := pem.Add(context.Background(), ProposedWrite{
  		Kind: coremem.KindWorking, Item: coremem.MemoryItem{Content: "ssn 123-45-6789"},
  	}); err != nil {
  		t.Fatalf("Add: %v", err)
  	}
  	got := rec.snapshot()
  	if len(got) != 1 || got[0].Name != EventWritePolicyDecided {
  		t.Fatalf("expected 1 EventWritePolicyDecided event, got %+v", got)
  	}
  	if v, _ := got[0].Attrs["verdict"].(string); v != string(VerdictRedact) {
  		t.Errorf("verdict = %v, want %q", got[0].Attrs["verdict"], VerdictRedact)
  	}
  }

  func TestPolicyEnforcingMemory_Add_EmitsEventWritePolicyDecidedOnReject(t *testing.T) {
  	rec := &recordingObserver{}
  	mgr := newCoreManager(t)
  	policy := PolicyFunc(func(_ context.Context, _ ProposedWrite) WritePolicyDecision {
  		return WritePolicyDecision{Verdict: VerdictReject, Reason: "policy:no-pii"}
  	})
  	pem, err := NewPolicyEnforcingMemory(mgr, policy, WithObserver(rec))
  	if err != nil {
  		t.Fatalf("NewPolicyEnforcingMemory: %v", err)
  	}
  	_, _ = pem.Add(context.Background(), ProposedWrite{
  		Kind: coremem.KindWorking, Item: coremem.MemoryItem{Content: "x"}, Source: SourceAgentInferred,
  	})
  	got := rec.snapshot()
  	if len(got) != 1 || got[0].Name != EventWritePolicyDecided {
  		t.Fatalf("expected 1 EventWritePolicyDecided event, got %+v", got)
  	}
  	if v, _ := got[0].Attrs["verdict"].(string); v != string(VerdictReject) {
  		t.Errorf("verdict = %v, want %q", got[0].Attrs["verdict"], VerdictReject)
  	}
  	if r, _ := got[0].Attrs["reason"].(string); r != "policy:no-pii" {
  		t.Errorf("reason = %v, want %q", got[0].Attrs["reason"], "policy:no-pii")
  	}
  }
  ```

- [ ] **Step 2: Run to confirm failure**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run 'TestPolicyEnforcingMemory_Add_EmitsEventWritePolicyDecided' -v`
  Expected: compile error — `undefined: EventWritePolicyDecided`.

- [ ] **Step 3: Declare the constant in `observer.go`**

  Append to the existing constant block in `observer.go` (immediately after `EventSnapshotVectorsBytes`):

  ```go
  	// EventWritePolicyDecided is emitted by PolicyEnforcingMemory.Add
  	// after every policy decision (accept, redact, reject). Attrs schema:
  	//   "verdict":      string  (one of VerdictAccept/Redact/Reject)
  	//   "input_kind":   coremem.Kind
  	//   "decided_kind": coremem.Kind  (mirrors input_kind on reject)
  	//   "source":       string  (the WriteSource of the ProposedWrite)
  	//   "reason":       string  (the WritePolicyDecision.Reason)
  	EventWritePolicyDecided = "memory_write_policy_decided"
  ```

  Also update the schema doc-block at the top of `observer.go` (the `Attribute schemas per event name (v0.2.0):` block) — change the version note to `(v0.2.0; EventWritePolicyDecided added v0.3.0)` and append the new event's row to the table.

- [ ] **Step 4: Replace the two FIXME literal strings in `write_policy.go` with `EventWritePolicyDecided` and remove the FIXME comments. Then run all four tests**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run 'TestPolicyEnforcingMemory_Add_(EmitsEventWritePolicyDecided|Verdict)' -v`
  Expected: all four PASS.

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/observer.go memory/write_policy.go memory/write_policy_test.go
  git commit -m "feat(memory): emit memory_write_policy_decided from PolicyEnforcingMemory (C-1)"
  git push origin main
  ```

---

## Task 5: `PolicyAdapter` — let `WritePolicy` satisfy `coremem.Sanitizer`

**Files:**
- Modify: `llm-agent-memory/memory/write_policy.go`
- Modify: `llm-agent-memory/memory/write_policy_test.go` (append)

- [ ] **Step 1: Append failing tests covering Sanitizer-shape decisions and reroute rejection**

  ```go
  func TestPolicyAdapter_Sanitize_AcceptReturnsKeepTrue(t *testing.T) {
  	policy := PolicyFunc(func(_ context.Context, in ProposedWrite) WritePolicyDecision {
  		return WritePolicyDecision{Verdict: VerdictAccept, Kind: in.Kind, Item: in.Item}
  	})
  	adapter := PolicyAdapter{Policy: policy}
  	out, keep, err := adapter.Sanitize(context.Background(), coremem.KindWorking, coremem.MemoryItem{Content: "x"})
  	if err != nil {
  		t.Fatalf("Sanitize: %v", err)
  	}
  	if !keep {
  		t.Error("keep = false on VerdictAccept")
  	}
  	if out.Content != "x" {
  		t.Errorf("Content = %q, want %q", out.Content, "x")
  	}
  }

  func TestPolicyAdapter_Sanitize_RejectReturnsKeepFalse(t *testing.T) {
  	policy := PolicyFunc(func(_ context.Context, _ ProposedWrite) WritePolicyDecision {
  		return WritePolicyDecision{Verdict: VerdictReject}
  	})
  	adapter := PolicyAdapter{Policy: policy}
  	_, keep, err := adapter.Sanitize(context.Background(), coremem.KindWorking, coremem.MemoryItem{Content: "x"})
  	if err != nil {
  		t.Fatalf("Sanitize: %v", err)
  	}
  	if keep {
  		t.Error("keep = true on VerdictReject")
  	}
  }

  func TestPolicyAdapter_Sanitize_RerouteReturnsKindRerouteUnsupported(t *testing.T) {
  	policy := PolicyFunc(func(_ context.Context, in ProposedWrite) WritePolicyDecision {
  		return WritePolicyDecision{Verdict: VerdictAccept, Kind: coremem.KindEpisodic, Item: in.Item}
  	})
  	adapter := PolicyAdapter{Policy: policy}
  	_, _, err := adapter.Sanitize(context.Background(), coremem.KindWorking, coremem.MemoryItem{Content: "x"})
  	if !errors.Is(err, ErrPolicyKindRerouteUnsupported) {
  		t.Errorf("err = %v, want errors.Is ErrPolicyKindRerouteUnsupported", err)
  	}
  }
  ```

- [ ] **Step 2: Run to confirm failure**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run 'TestPolicyAdapter_Sanitize_' -v`
  Expected: compile error — `undefined: PolicyAdapter`.

- [ ] **Step 3: Implement `PolicyAdapter` in `write_policy.go`**

  Append to `write_policy.go`:

  ```go
  // PolicyAdapter exposes a WritePolicy as a coremem.Sanitizer so
  // callers wired to the existing WithSanitizer chain (see core
  // policy_hook.go) get policy semantics for free. The adapter cannot
  // reroute kinds — Sanitizer's return triple has no kind slot. When
  // the wrapped policy returns a Decision.Kind that differs from the
  // input kind, Sanitize returns ErrPolicyKindRerouteUnsupported.
  //
  // Source defaults to SourceSystem for adapter calls because the
  // Sanitizer interface carries no source hint. Callers wanting
  // source-specific policy decisions must use PolicyEnforcingMemory
  // directly.
  type PolicyAdapter struct {
  	Policy WritePolicy
  }

  // Sanitize satisfies coremem.Sanitizer. See PolicyAdapter godoc for
  // the reroute limitation.
  func (a PolicyAdapter) Sanitize(ctx context.Context, kind coremem.Kind, item coremem.MemoryItem) (coremem.MemoryItem, bool, error) {
  	decision := a.Policy.Decide(ctx, ProposedWrite{
  		Kind:   kind,
  		Item:   item,
  		Source: SourceSystem,
  	})
  	switch decision.Verdict {
  	case VerdictAccept, VerdictRedact:
  		if decision.Kind != kind {
  			return coremem.MemoryItem{}, false, ErrPolicyKindRerouteUnsupported
  		}
  		return decision.Item, true, nil
  	case VerdictReject:
  		return coremem.MemoryItem{}, false, nil
  	default:
  		return coremem.MemoryItem{}, false, errors.New("memory: write policy returned unknown verdict")
  	}
  }

  // Compile-time check that PolicyAdapter satisfies the core Sanitizer
  // contract. If coremem renames or restructures Sanitizer, this line
  // will fail to compile — a deliberate early-warning signal.
  var _ coremem.Sanitizer = PolicyAdapter{}
  ```

- [ ] **Step 4: Run the three new tests**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run 'TestPolicyAdapter_Sanitize_' -v`
  Expected: all three PASS.

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/write_policy.go memory/write_policy_test.go
  git commit -m "feat(memory): add PolicyAdapter for coremem.Sanitizer interop (C-1)"
  git push origin main
  ```

---

## Task 6: WritePolicy four-decision matrix proof — single table-driven test

**Files:**
- Modify: `llm-agent-memory/memory/write_policy_test.go` (append)

This task is the explicit C-1 exit-criterion proof: a single test that exercises every one of the four documented decisions (`user-saved`, `agent-inferred`, `reject`, `redact`) through `PolicyEnforcingMemory`.

- [ ] **Step 1: Append the table-driven proof test**

  ```go
  func TestPolicyEnforcingMemory_CoversAllFourDocumentedDecisions(t *testing.T) {
  	// Per docs/memory-roadmap.zh-CN.md §4.3 C-1 exit criterion: the
  	// policy interface must cover user-saved, agent-inferred, reject,
  	// and redact decisions. One mgr per case so writes don't bleed
  	// between assertions.
  	cases := []struct {
  		name         string
  		source       WriteSource
  		decide       func(ProposedWrite) WritePolicyDecision
  		wantErr      error // nil for success; ErrRejectedByPolicy for reject
  		wantLanded   bool  // whether to check that an item exists in mgr after Add
  		wantInKind   coremem.Kind
  		wantContent  string // exact content to expect in the landed item; "" for reject
  	}{
  		{
  			name:        "user-saved direct to episodic",
  			source:      SourceUserSaved,
  			decide:      func(in ProposedWrite) WritePolicyDecision { return WritePolicyDecision{Verdict: VerdictAccept, Kind: coremem.KindEpisodic, Item: in.Item, Reason: "user-saved-promote"} },
  			wantLanded:  true,
  			wantInKind:  coremem.KindEpisodic,
  			wantContent: "user typed this",
  		},
  		{
  			name:        "agent-inferred routes to working",
  			source:      SourceAgentInferred,
  			decide:      func(in ProposedWrite) WritePolicyDecision { return WritePolicyDecision{Verdict: VerdictAccept, Kind: coremem.KindWorking, Item: in.Item, Reason: "agent-inferred-defer"} },
  			wantLanded:  true,
  			wantInKind:  coremem.KindWorking,
  			wantContent: "agent inferred this",
  		},
  		{
  			name:    "reject pii",
  			source:  SourceAgentInferred,
  			decide:  func(_ ProposedWrite) WritePolicyDecision { return WritePolicyDecision{Verdict: VerdictReject, Reason: "policy:pii"} },
  			wantErr: ErrRejectedByPolicy,
  		},
  		{
  			name:   "redact secret",
  			source: SourceUserSaved,
  			decide: func(in ProposedWrite) WritePolicyDecision {
  				it := in.Item
  				it.Content = "[REDACTED]"
  				return WritePolicyDecision{Verdict: VerdictRedact, Kind: in.Kind, Item: it, Reason: "policy:secret"}
  			},
  			wantLanded:  true,
  			wantInKind:  coremem.KindWorking,
  			wantContent: "[REDACTED]",
  		},
  	}

  	for _, tc := range cases {
  		t.Run(tc.name, func(t *testing.T) {
  			mgr := newCoreManager(t)
  			pem, err := NewPolicyEnforcingMemory(mgr, PolicyFunc(func(_ context.Context, in ProposedWrite) WritePolicyDecision {
  				return tc.decide(in)
  			}))
  			if err != nil {
  				t.Fatalf("NewPolicyEnforcingMemory: %v", err)
  			}

  			content := tc.wantContent
  			if content == "" {
  				content = "blocked-content"
  			}
  			id, err := pem.Add(context.Background(), ProposedWrite{
  				Kind:   coremem.KindWorking,
  				Item:   coremem.MemoryItem{Content: content},
  				Source: tc.source,
  			})
  			if tc.wantErr != nil {
  				if !errors.Is(err, tc.wantErr) {
  					t.Fatalf("err = %v, want errors.Is %v", err, tc.wantErr)
  				}
  				return
  			}
  			if err != nil {
  				t.Fatalf("Add: %v", err)
  			}
  			if tc.wantLanded {
  				got, err := mgr.Get(context.Background(), tc.wantInKind, id)
  				if err != nil {
  					t.Fatalf("Get (kind=%v): %v", tc.wantInKind, err)
  				}
  				if got.Content != tc.wantContent {
  					t.Errorf("Content = %q, want %q", got.Content, tc.wantContent)
  				}
  			}
  		})
  	}
  }
  ```

- [ ] **Step 2: Run to confirm PASS (implementation already exists)**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestPolicyEnforcingMemory_CoversAllFourDocumentedDecisions -v`
  Expected: all four sub-tests PASS. (No implementation change needed; this test exists to lock the four-decision exit criterion as a single regression detector.)

- [ ] **Step 3: Skip (no impl change).**

- [ ] **Step 4: Skip.**

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/write_policy_test.go
  git commit -m "test(memory): pin all four C-1 documented decisions in one table test"
  git push origin main
  ```

---

# Phase C-2 — SQLiteStore (Tasks 7–12)

## Task 7: Failing test for `SQLiteStore` interface compliance + migration

**Files:**
- Create: `llm-agent-memory/memory/sqlite_store_test.go`
- Create: `llm-agent-memory/memory/sqlite_store_helpers_test.go`

- [ ] **Step 1: Create the test helpers file**

  Create `llm-agent-memory/memory/sqlite_store_helpers_test.go`:

  ```go
  package memory

  import (
  	"context"
  	"fmt"
  	"sync/atomic"
  	"testing"

  	coremem "github.com/costa92/llm-agent/memory"
  )

  var sqliteTestCounter uint64

  // newTempSQLiteStore creates a fresh in-memory SQLiteStore with a
  // unique URL-encoded name so concurrent tests do not share state.
  // The returned cleanup function MUST be deferred (or registered via
  // t.Cleanup, which the helper already does).
  func newTempSQLiteStore(t *testing.T) *SQLiteStore {
  	t.Helper()
  	n := atomic.AddUint64(&sqliteTestCounter, 1)
  	// `file:` + a unique `name=` parameter + `mode=memory` + `cache=shared`
  	// gives each test its own in-memory database that survives across
  	// pool connections within this test only.
  	dsn := fmt.Sprintf("file:sqlitetest_%d?mode=memory&cache=shared", n)
  	store, err := NewSQLiteStore(dsn)
  	if err != nil {
  		t.Fatalf("NewSQLiteStore: %v", err)
  	}
  	t.Cleanup(func() {
  		if err := store.Close(); err != nil {
  			t.Errorf("SQLiteStore.Close: %v", err)
  		}
  	})
  	return store
  }

  // assertSnapshotEqual compares two coremem.Snapshot values without
  // using reflect.DeepEqual — Item.CreatedAt / AccessedAt come back
  // from JSON without monotonic clock, so reflect.DeepEqual on a
  // round-trip is a known false-negative trap (see M2 Task 11 BLOCKED).
  func assertSnapshotEqual(t *testing.T, got, want coremem.Snapshot) {
  	t.Helper()
  	if got.Version != want.Version {
  		t.Errorf("Version: got %d, want %d", got.Version, want.Version)
  	}
  	if got.Kind != want.Kind {
  		t.Errorf("Kind: got %q, want %q", got.Kind, want.Kind)
  	}
  	if len(got.Items) != len(want.Items) {
  		t.Fatalf("len(Items): got %d, want %d", len(got.Items), len(want.Items))
  	}
  	for i := range got.Items {
  		gi, wi := got.Items[i], want.Items[i]
  		if gi.Item.ID != wi.Item.ID {
  			t.Errorf("Items[%d].Item.ID: got %q, want %q", i, gi.Item.ID, wi.Item.ID)
  		}
  		if gi.Item.Content != wi.Item.Content {
  			t.Errorf("Items[%d].Item.Content: got %q, want %q", i, gi.Item.Content, wi.Item.Content)
  		}
  		if !gi.Item.CreatedAt.Equal(wi.Item.CreatedAt) {
  			t.Errorf("Items[%d].Item.CreatedAt: got %v, want %v", i, gi.Item.CreatedAt, wi.Item.CreatedAt)
  		}
  		if len(gi.Vector) != len(wi.Vector) {
  			t.Errorf("Items[%d].Vector len: got %d, want %d", i, len(gi.Vector), len(wi.Vector))
  			continue
  		}
  		for j := range gi.Vector {
  			if gi.Vector[j] != wi.Vector[j] {
  				t.Errorf("Items[%d].Vector[%d]: got %v, want %v", i, j, gi.Vector[j], wi.Vector[j])
  				break
  			}
  		}
  	}
  	_ = context.Background() // pin the import so cleanup of unused imports doesn't bite later
  }
  ```

- [ ] **Step 2: Create the test file with the interface-compliance test**

  Create `llm-agent-memory/memory/sqlite_store_test.go`:

  ```go
  package memory

  import (
  	"context"
  	"errors"
  	"os"
  	"testing"

  	coremem "github.com/costa92/llm-agent/memory"
  )

  // TestSQLiteStore_SatisfiesSnapshotStore is a compile-time + runtime
  // assertion that SQLiteStore satisfies the coremem.SnapshotStore
  // contract from llm-agent/memory/persistence.go:171-176.
  func TestSQLiteStore_SatisfiesSnapshotStore(t *testing.T) {
  	store := newTempSQLiteStore(t)
  	var _ coremem.SnapshotStore = store // compile-time check
  	// Touch each interface method via a no-op call.
  	if _, err := store.List(context.Background()); err != nil {
  		t.Errorf("List on empty store: %v", err)
  	}
  	if err := store.Delete(context.Background(), "nonexistent"); err != nil {
  		t.Errorf("Delete on missing key should be a no-op, got: %v", err)
  	}
  	if _, err := store.Load(context.Background(), "nonexistent"); !errors.Is(err, os.ErrNotExist) {
  		t.Errorf("Load on missing key err = %v, want wraps os.ErrNotExist", err)
  	}
  }
  ```

- [ ] **Step 3: Run to confirm failure**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestSQLiteStore_SatisfiesSnapshotStore -v`
  Expected: compile error — `undefined: SQLiteStore`, `undefined: NewSQLiteStore`. (Also the `modernc.org/sqlite` dep is not yet in `go.mod` — that lands in Task 8.)

- [ ] **Step 4: Skip — impl in Task 8.**

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/sqlite_store_test.go memory/sqlite_store_helpers_test.go
  git commit -m "test(memory): scaffold SQLiteStore interface-compliance test (C-2)"
  git push origin main
  ```

---

## Task 8: Add `modernc.org/sqlite` dependency + implement `SQLiteStore` skeleton + migration

**Files:**
- Modify: `llm-agent-memory/go.mod`
- Modify: `llm-agent-memory/go.sum`
- Create: `llm-agent-memory/memory/sqlite_store.go`

- [ ] **Step 1: Pin the dependency. Use `go get` to add the latest 1.41.x release.**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go get modernc.org/sqlite@latest && GOWORK=off go mod tidy`
  Expected: `go.mod` now lists `modernc.org/sqlite vX.Y.Z` as a direct require; `go.sum` populated with the full transitive closure (modernc.org/libc, modernc.org/mathutil, modernc.org/memory, modernc.org/strutil, modernc.org/token, etc. — all pure-Go).

  Verify the executor saw a real version: `cat go.mod | grep modernc.org/sqlite` should show `require modernc.org/sqlite v1.XX.Y` (no `latest` literal).

- [ ] **Step 2: Verify driver registration name with `go doc`**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go doc modernc.org/sqlite | head -30`
  Expected: the package documents the side-effect import (`_ "modernc.org/sqlite"`) registers itself under the driver name `"sqlite"`. If the doc shows a different driver name, update the `sql.Open("sqlite", dsn)` call in Step 3 accordingly.

- [ ] **Step 3: Create `llm-agent-memory/memory/sqlite_store.go`**

  ```go
  // Package memory — sqlite_store.go is the Phase C-2 implementation
  // of coremem.SnapshotStore backed by SQLite via the pure-Go
  // modernc.org/sqlite driver. No CGO required.
  //
  // Schema: see SchemaVersion + the migrator below. Two tables:
  //   memory_store_schema (version, applied_at)
  //   memory_snapshots    (key, kind, snapshot_json, updated_at)
  // with a single index on memory_snapshots(key).
  package memory

  import (
  	"context"
  	"database/sql"
  	"encoding/json"
  	"errors"
  	"fmt"
  	"os"
  	"strings"
  	"time"

  	coremem "github.com/costa92/llm-agent/memory"

  	_ "modernc.org/sqlite" // registers the "sqlite" driver
  )

  // SchemaVersion is the SQLiteStore schema version this binary
  // implements. NewSQLiteStore migrates up to this version on open;
  // a database recording a HIGHER version is refused with
  // ErrSchemaVersionAhead.
  const SchemaVersion = 1

  // ErrSchemaVersionAhead is returned by NewSQLiteStore when the
  // database's recorded schema version exceeds SchemaVersion. This
  // protects against an older binary silently downgrading state
  // written by a newer one.
  var ErrSchemaVersionAhead = errors.New("memory: sqlite store schema ahead of code")

  // ErrSQLiteDSNRequired is returned by NewSQLiteStore when dsn is
  // empty.
  var ErrSQLiteDSNRequired = errors.New("memory: sqlite store requires a non-empty DSN")

  // SQLiteStore implements coremem.SnapshotStore + the optional
  // LoadKind(ctx, key, kind) method consumed by
  // coremem.Manager.ImportAll (manager.go:369-371). Goroutine-safe via
  // the underlying *sql.DB.
  type SQLiteStore struct {
  	db *sql.DB
  }

  // NewSQLiteStore opens dsn (any modernc.org/sqlite DSN — file path,
  // file:URI, or in-memory `file::memory:?cache=shared`), runs the
  // in-code migrator up to SchemaVersion, and returns the store. Pool
  // is left at driver defaults except for `:memory:` DSNs, where
  // SetMaxOpenConns(1) is required for shared-in-memory tests.
  func NewSQLiteStore(dsn string) (*SQLiteStore, error) {
  	if dsn == "" {
  		return nil, ErrSQLiteDSNRequired
  	}
  	db, err := sql.Open("sqlite", dsn)
  	if err != nil {
  		return nil, fmt.Errorf("memory: sqlite store open: %w", err)
  	}
  	if strings.Contains(dsn, ":memory:") || strings.Contains(dsn, "mode=memory") {
  		db.SetMaxOpenConns(1)
  	}
  	s := &SQLiteStore{db: db}
  	if err := s.migrate(context.Background()); err != nil {
  		_ = db.Close()
  		return nil, err
  	}
  	current, err := s.currentVersion(context.Background())
  	if err != nil {
  		_ = db.Close()
  		return nil, err
  	}
  	if current > SchemaVersion {
  		_ = db.Close()
  		return nil, fmt.Errorf("%w: db=%d, code=%d", ErrSchemaVersionAhead, current, SchemaVersion)
  	}
  	return s, nil
  }

  // Close closes the underlying database handle. Idempotent.
  func (s *SQLiteStore) Close() error {
  	if s.db == nil {
  		return nil
  	}
  	err := s.db.Close()
  	s.db = nil
  	return err
  }

  // migrate runs every pending migration step in a single transaction.
  // Idempotent: re-running against an already-current database is a
  // no-op. Migrations are numbered 1..SchemaVersion and applied in
  // order; a future SchemaVersion>1 extends the migrations slice
  // below.
  func (s *SQLiteStore) migrate(ctx context.Context) error {
  	migrations := []struct {
  		version int
  		stmts   []string
  	}{
  		{
  			version: 1,
  			stmts: []string{
  				`CREATE TABLE IF NOT EXISTS memory_store_schema (
  					version    INTEGER PRIMARY KEY,
  					applied_at TEXT NOT NULL
  				)`,
  				`CREATE TABLE IF NOT EXISTS memory_snapshots (
  					key           TEXT NOT NULL,
  					kind          TEXT NOT NULL,
  					snapshot_json BLOB NOT NULL,
  					updated_at    TEXT NOT NULL,
  					PRIMARY KEY (key, kind)
  				)`,
  				`CREATE INDEX IF NOT EXISTS idx_memory_snapshots_key ON memory_snapshots(key)`,
  			},
  		},
  	}

  	// Find which versions are not yet applied.
  	applied, err := s.appliedVersions(ctx)
  	if err != nil && !isNoSchemaTable(err) {
  		return fmt.Errorf("memory: sqlite store migrate: read applied versions: %w", err)
  	}

  	tx, err := s.db.BeginTx(ctx, nil)
  	if err != nil {
  		return fmt.Errorf("memory: sqlite store migrate: begin tx: %w", err)
  	}
  	defer func() { _ = tx.Rollback() }() // no-op after Commit

  	for _, m := range migrations {
  		if applied[m.version] {
  			continue
  		}
  		for _, stmt := range m.stmts {
  			if _, err := tx.ExecContext(ctx, stmt); err != nil {
  				return fmt.Errorf("memory: sqlite store migrate v%d: %w", m.version, err)
  			}
  		}
  		if _, err := tx.ExecContext(ctx,
  			`INSERT OR IGNORE INTO memory_store_schema (version, applied_at) VALUES (?, ?)`,
  			m.version, time.Now().UTC().Format(time.RFC3339Nano),
  		); err != nil {
  			return fmt.Errorf("memory: sqlite store migrate v%d: record: %w", m.version, err)
  		}
  	}
  	if err := tx.Commit(); err != nil {
  		return fmt.Errorf("memory: sqlite store migrate: commit: %w", err)
  	}
  	return nil
  }

  // appliedVersions reads the memory_store_schema table and returns a
  // set of applied version numbers. Returns an empty set + nil error
  // if the table does not exist yet (first migration run).
  func (s *SQLiteStore) appliedVersions(ctx context.Context) (map[int]bool, error) {
  	out := map[int]bool{}
  	rows, err := s.db.QueryContext(ctx, `SELECT version FROM memory_store_schema`)
  	if err != nil {
  		return out, err
  	}
  	defer rows.Close()
  	for rows.Next() {
  		var v int
  		if err := rows.Scan(&v); err != nil {
  			return out, err
  		}
  		out[v] = true
  	}
  	return out, rows.Err()
  }

  func (s *SQLiteStore) currentVersion(ctx context.Context) (int, error) {
  	var v sql.NullInt64
  	if err := s.db.QueryRowContext(ctx, `SELECT MAX(version) FROM memory_store_schema`).Scan(&v); err != nil {
  		return 0, fmt.Errorf("memory: sqlite store: read current version: %w", err)
  	}
  	if !v.Valid {
  		return 0, nil
  	}
  	return int(v.Int64), nil
  }

  // isNoSchemaTable returns true if the error indicates the
  // memory_store_schema table does not exist yet. modernc.org/sqlite
  // surfaces this via a SQLITE_ERROR with the message containing
  // "no such table".
  func isNoSchemaTable(err error) bool {
  	if err == nil {
  		return false
  	}
  	return strings.Contains(err.Error(), "no such table: memory_store_schema")
  }

  // sanitizeSQLiteKey replaces every character outside [a-zA-Z0-9_-]
  // with '_'. Empty input becomes "_". Keep in sync with the
  // sanitizer in github.com/costa92/llm-agent/memory/persistence.go
  // (persistence.go:206-219) so a caller key normalizes identically
  // across SnapshotStore impls.
  func sanitizeSQLiteKey(s string) string {
  	var b strings.Builder
  	for _, r := range s {
  		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
  			b.WriteRune(r)
  		} else {
  			b.WriteRune('_')
  		}
  	}
  	if b.Len() == 0 {
  		return "_"
  	}
  	return b.String()
  }

  // Save UPSERTs (key, snap.Kind) → snap. snap.Kind must be non-empty.
  func (s *SQLiteStore) Save(ctx context.Context, key string, snap coremem.Snapshot) error {
  	if snap.Kind == "" {
  		return errors.New("memory: sqlite store save: snapshot kind is required")
  	}
  	payload, err := json.Marshal(snap)
  	if err != nil {
  		return fmt.Errorf("memory: sqlite store save: encode: %w", err)
  	}
  	sk := sanitizeSQLiteKey(key)
  	_, err = s.db.ExecContext(ctx,
  		`INSERT INTO memory_snapshots (key, kind, snapshot_json, updated_at) VALUES (?, ?, ?, ?)
  		 ON CONFLICT(key, kind) DO UPDATE SET snapshot_json = excluded.snapshot_json, updated_at = excluded.updated_at`,
  		sk, string(snap.Kind), payload, time.Now().UTC().Format(time.RFC3339Nano),
  	)
  	if err != nil {
  		return fmt.Errorf("memory: sqlite store save: %w", err)
  	}
  	return nil
  }

  // Load returns the first snapshot found for key across the three
  // kinds (working → episodic → semantic). Returns an error wrapping
  // os.ErrNotExist when no row exists for any kind. Mirrors
  // (*FilesystemStore).Load semantics from persistence.go:254-265.
  func (s *SQLiteStore) Load(ctx context.Context, key string) (coremem.Snapshot, error) {
  	for _, kind := range []coremem.Kind{coremem.KindWorking, coremem.KindEpisodic, coremem.KindSemantic} {
  		snap, err := s.LoadKind(ctx, key, kind)
  		if err == nil {
  			return snap, nil
  		}
  		if !errors.Is(err, os.ErrNotExist) {
  			return coremem.Snapshot{}, err
  		}
  	}
  	return coremem.Snapshot{}, fmt.Errorf("memory: sqlite store: no snapshot for key %q: %w", key, os.ErrNotExist)
  }

  // LoadKind returns the snapshot for the exact (key, kind) tuple.
  // Returns an error wrapping os.ErrNotExist when no row exists. The
  // method exists explicitly so coremem.Manager.ImportAll's optional
  // kindLoader type-assertion (manager.go:369-371) finds it and uses
  // the per-kind path instead of falling back to Load.
  func (s *SQLiteStore) LoadKind(ctx context.Context, key string, kind coremem.Kind) (coremem.Snapshot, error) {
  	sk := sanitizeSQLiteKey(key)
  	var payload []byte
  	err := s.db.QueryRowContext(ctx,
  		`SELECT snapshot_json FROM memory_snapshots WHERE key = ? AND kind = ?`,
  		sk, string(kind),
  	).Scan(&payload)
  	if errors.Is(err, sql.ErrNoRows) {
  		return coremem.Snapshot{}, fmt.Errorf("memory: sqlite store: no snapshot for key %q kind %q: %w", key, kind, os.ErrNotExist)
  	}
  	if err != nil {
  		return coremem.Snapshot{}, fmt.Errorf("memory: sqlite store load: %w", err)
  	}
  	var snap coremem.Snapshot
  	if err := json.Unmarshal(payload, &snap); err != nil {
  		return coremem.Snapshot{}, fmt.Errorf("memory: sqlite store load: decode: %w", err)
  	}
  	return snap, nil
  }

  // Delete removes all rows at key (across kinds). Missing rows are
  // not an error. Mirrors (*FilesystemStore).Delete semantics from
  // persistence.go:289-300.
  func (s *SQLiteStore) Delete(ctx context.Context, key string) error {
  	sk := sanitizeSQLiteKey(key)
  	if _, err := s.db.ExecContext(ctx, `DELETE FROM memory_snapshots WHERE key = ?`, sk); err != nil {
  		return fmt.Errorf("memory: sqlite store delete: %w", err)
  	}
  	return nil
  }

  // List returns the sorted set of unique keys present in the store.
  // Mirrors (*FilesystemStore).List semantics from persistence.go:305-333.
  func (s *SQLiteStore) List(ctx context.Context) ([]string, error) {
  	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT key FROM memory_snapshots ORDER BY key ASC`)
  	if err != nil {
  		return nil, fmt.Errorf("memory: sqlite store list: %w", err)
  	}
  	defer rows.Close()
  	out := []string{}
  	for rows.Next() {
  		var k string
  		if err := rows.Scan(&k); err != nil {
  			return nil, fmt.Errorf("memory: sqlite store list scan: %w", err)
  		}
  		out = append(out, k)
  	}
  	if err := rows.Err(); err != nil {
  		return nil, fmt.Errorf("memory: sqlite store list iter: %w", err)
  	}
  	return out, nil
  }
  ```

- [ ] **Step 4: Run the Task 7 test plus the new file's compile-check**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestSQLiteStore_SatisfiesSnapshotStore -v`
  Expected: PASS. (`Load on missing key` returns an os.ErrNotExist-wrapped error; `Delete` is a no-op on missing key; `List` returns an empty slice with nil error.)

  Also run: `GOWORK=off go vet ./memory`
  Expected: no output.

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/sqlite_store.go go.mod go.sum
  git commit -m "feat(memory): add SQLiteStore + in-code migrator (modernc.org/sqlite) (C-2)"
  git push origin main
  ```

---

## Task 9: Per-kind round-trip + LoadKind selection tests

**Files:**
- Modify: `llm-agent-memory/memory/sqlite_store_test.go` (append)

- [ ] **Step 1: Append failing tests**

  ```go
  func TestSQLiteStore_Save_Load_RoundTripsSingleKind(t *testing.T) {
  	store := newTempSQLiteStore(t)
  	ctx := context.Background()
  	snap := coremem.Snapshot{
  		Version: coremem.SnapshotVersion,
  		Kind:    coremem.KindEpisodic,
  		Items: []coremem.SnapshotItem{
  			{
  				Item: coremem.MemoryItem{
  					ID: "a", Content: "alpha", Importance: 0.5,
  					CreatedAt: time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC),
  				},
  				Vector: []float32{0.1, 0.2, 0.3},
  			},
  		},
  	}
  	if err := store.Save(ctx, "session-1", snap); err != nil {
  		t.Fatalf("Save: %v", err)
  	}
  	got, err := store.Load(ctx, "session-1")
  	if err != nil {
  		t.Fatalf("Load: %v", err)
  	}
  	assertSnapshotEqual(t, got, snap)
  }

  func TestSQLiteStore_LoadKind_SelectsExactKind(t *testing.T) {
  	store := newTempSQLiteStore(t)
  	ctx := context.Background()
  	mk := func(kind coremem.Kind, content string) coremem.Snapshot {
  		return coremem.Snapshot{
  			Version: coremem.SnapshotVersion,
  			Kind:    kind,
  			Items: []coremem.SnapshotItem{
  				{Item: coremem.MemoryItem{ID: "x", Content: content}, Vector: []float32{1}},
  			},
  		}
  	}
  	if err := store.Save(ctx, "k", mk(coremem.KindWorking, "w")); err != nil {
  		t.Fatalf("Save working: %v", err)
  	}
  	if err := store.Save(ctx, "k", mk(coremem.KindEpisodic, "e")); err != nil {
  		t.Fatalf("Save episodic: %v", err)
  	}
  	wsnap, err := store.LoadKind(ctx, "k", coremem.KindWorking)
  	if err != nil {
  		t.Fatalf("LoadKind working: %v", err)
  	}
  	if wsnap.Items[0].Item.Content != "w" {
  		t.Errorf("LoadKind working Content = %q, want %q", wsnap.Items[0].Item.Content, "w")
  	}
  	esnap, err := store.LoadKind(ctx, "k", coremem.KindEpisodic)
  	if err != nil {
  		t.Fatalf("LoadKind episodic: %v", err)
  	}
  	if esnap.Items[0].Item.Content != "e" {
  		t.Errorf("LoadKind episodic Content = %q, want %q", esnap.Items[0].Item.Content, "e")
  	}
  }

  func TestSQLiteStore_LoadKind_MissingReturnsErrNotExist(t *testing.T) {
  	store := newTempSQLiteStore(t)
  	_, err := store.LoadKind(context.Background(), "missing", coremem.KindWorking)
  	if !errors.Is(err, os.ErrNotExist) {
  		t.Errorf("LoadKind missing err = %v, want wraps os.ErrNotExist", err)
  	}
  }
  ```

  Add the `"time"` import to the test file.

- [ ] **Step 2: Run to confirm PASS**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run 'TestSQLiteStore_(Save_Load|LoadKind)' -v`
  Expected: all three PASS.

- [ ] **Step 3: Skip (no impl change).**

- [ ] **Step 4: Skip.**

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/sqlite_store_test.go
  git commit -m "test(memory): pin SQLiteStore Save/Load per-kind round-trip (C-2)"
  git push origin main
  ```

---

## Task 10: Delete + List + idempotent Save tests

**Files:**
- Modify: `llm-agent-memory/memory/sqlite_store_test.go` (append)

- [ ] **Step 1: Append failing tests**

  ```go
  func TestSQLiteStore_Delete_RemovesAllKindsAtKey(t *testing.T) {
  	store := newTempSQLiteStore(t)
  	ctx := context.Background()
  	for _, kind := range []coremem.Kind{coremem.KindWorking, coremem.KindEpisodic} {
  		if err := store.Save(ctx, "k", coremem.Snapshot{
  			Version: coremem.SnapshotVersion,
  			Kind:    kind,
  			Items:   []coremem.SnapshotItem{{Item: coremem.MemoryItem{ID: "x"}, Vector: []float32{1}}},
  		}); err != nil {
  			t.Fatalf("Save %v: %v", kind, err)
  		}
  	}
  	if err := store.Delete(ctx, "k"); err != nil {
  		t.Fatalf("Delete: %v", err)
  	}
  	if _, err := store.LoadKind(ctx, "k", coremem.KindWorking); !errors.Is(err, os.ErrNotExist) {
  		t.Errorf("LoadKind working post-delete err = %v, want os.ErrNotExist", err)
  	}
  	if _, err := store.LoadKind(ctx, "k", coremem.KindEpisodic); !errors.Is(err, os.ErrNotExist) {
  		t.Errorf("LoadKind episodic post-delete err = %v, want os.ErrNotExist", err)
  	}
  }

  func TestSQLiteStore_List_ReturnsSortedUniqueKeys(t *testing.T) {
  	store := newTempSQLiteStore(t)
  	ctx := context.Background()
  	keys := []string{"charlie", "alpha", "bravo"}
  	for _, k := range keys {
  		if err := store.Save(ctx, k, coremem.Snapshot{
  			Version: coremem.SnapshotVersion,
  			Kind:    coremem.KindWorking,
  			Items:   []coremem.SnapshotItem{{Item: coremem.MemoryItem{ID: "x"}, Vector: []float32{1}}},
  		}); err != nil {
  			t.Fatalf("Save %q: %v", k, err)
  		}
  	}
  	// Save same key with a different kind — must not duplicate in List.
  	if err := store.Save(ctx, "alpha", coremem.Snapshot{
  		Version: coremem.SnapshotVersion,
  		Kind:    coremem.KindEpisodic,
  		Items:   []coremem.SnapshotItem{{Item: coremem.MemoryItem{ID: "x"}, Vector: []float32{1}}},
  	}); err != nil {
  		t.Fatalf("Save alpha episodic: %v", err)
  	}
  	got, err := store.List(ctx)
  	if err != nil {
  		t.Fatalf("List: %v", err)
  	}
  	want := []string{"alpha", "bravo", "charlie"}
  	if len(got) != len(want) {
  		t.Fatalf("List got %v, want %v", got, want)
  	}
  	for i := range want {
  		if got[i] != want[i] {
  			t.Errorf("List[%d] = %q, want %q", i, got[i], want[i])
  		}
  	}
  }

  func TestSQLiteStore_Save_OnConflict_OverwritesExistingRow(t *testing.T) {
  	store := newTempSQLiteStore(t)
  	ctx := context.Background()
  	snap1 := coremem.Snapshot{
  		Version: coremem.SnapshotVersion,
  		Kind:    coremem.KindWorking,
  		Items:   []coremem.SnapshotItem{{Item: coremem.MemoryItem{ID: "a", Content: "v1"}, Vector: []float32{1}}},
  	}
  	snap2 := coremem.Snapshot{
  		Version: coremem.SnapshotVersion,
  		Kind:    coremem.KindWorking,
  		Items:   []coremem.SnapshotItem{{Item: coremem.MemoryItem{ID: "a", Content: "v2"}, Vector: []float32{2}}},
  	}
  	if err := store.Save(ctx, "k", snap1); err != nil {
  		t.Fatalf("Save v1: %v", err)
  	}
  	if err := store.Save(ctx, "k", snap2); err != nil {
  		t.Fatalf("Save v2: %v", err)
  	}
  	got, err := store.LoadKind(ctx, "k", coremem.KindWorking)
  	if err != nil {
  		t.Fatalf("LoadKind: %v", err)
  	}
  	if got.Items[0].Item.Content != "v2" {
  		t.Errorf("Content = %q, want %q (UPSERT did not overwrite)", got.Items[0].Item.Content, "v2")
  	}
  }
  ```

- [ ] **Step 2: Run to confirm PASS**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run 'TestSQLiteStore_(Delete|List|Save_OnConflict)' -v`
  Expected: all three PASS.

- [ ] **Step 3: Skip.**

- [ ] **Step 4: Skip.**

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/sqlite_store_test.go
  git commit -m "test(memory): pin SQLiteStore Delete, List, and UPSERT-overwrite semantics (C-2)"
  git push origin main
  ```

---

## Task 11: Migration idempotency + future-schema refusal tests

**Files:**
- Modify: `llm-agent-memory/memory/sqlite_store_test.go` (append)

- [ ] **Step 1: Append failing tests**

  ```go
  func TestSQLiteStore_Migration_IsIdempotentAcrossReopens(t *testing.T) {
  	// Open, close, reopen the SAME shared in-memory DSN. The second
  	// open must NOT fail and must NOT insert a duplicate version row.
  	dsn := "file:sqlitetest_idem?mode=memory&cache=shared"

  	// Hold a sentinel open to keep the shared-memory DB alive across the
  	// two NewSQLiteStore calls in this test (closing the only conn would
  	// destroy the DB).
  	sentinel, err := NewSQLiteStore(dsn)
  	if err != nil {
  		t.Fatalf("sentinel open: %v", err)
  	}
  	t.Cleanup(func() { _ = sentinel.Close() })

  	first, err := NewSQLiteStore(dsn)
  	if err != nil {
  		t.Fatalf("first open: %v", err)
  	}
  	v1, err := first.currentVersion(context.Background())
  	if err != nil {
  		t.Fatalf("first currentVersion: %v", err)
  	}
  	if v1 != SchemaVersion {
  		t.Errorf("first currentVersion = %d, want %d", v1, SchemaVersion)
  	}
  	if err := first.Close(); err != nil {
  		t.Fatalf("first close: %v", err)
  	}

  	second, err := NewSQLiteStore(dsn)
  	if err != nil {
  		t.Fatalf("second open: %v", err)
  	}
  	t.Cleanup(func() { _ = second.Close() })
  	v2, err := second.currentVersion(context.Background())
  	if err != nil {
  		t.Fatalf("second currentVersion: %v", err)
  	}
  	if v2 != SchemaVersion {
  		t.Errorf("second currentVersion = %d, want %d", v2, SchemaVersion)
  	}

  	// Count rows: must be exactly 1, not 2 (no duplicate INSERTs).
  	var n int
  	if err := second.db.QueryRowContext(context.Background(),
  		`SELECT COUNT(*) FROM memory_store_schema`).Scan(&n); err != nil {
  		t.Fatalf("count: %v", err)
  	}
  	if n != SchemaVersion {
  		t.Errorf("memory_store_schema rows = %d, want %d", n, SchemaVersion)
  	}
  }

  func TestSQLiteStore_NewSQLiteStore_RefusesFutureSchemaVersion(t *testing.T) {
  	dsn := "file:sqlitetest_future?mode=memory&cache=shared"
  	sentinel, err := NewSQLiteStore(dsn)
  	if err != nil {
  		t.Fatalf("sentinel open: %v", err)
  	}
  	t.Cleanup(func() { _ = sentinel.Close() })

  	// Forge a future-version row by writing directly to the shared DB.
  	if _, err := sentinel.db.ExecContext(context.Background(),
  		`INSERT INTO memory_store_schema (version, applied_at) VALUES (?, ?)`,
  		SchemaVersion+1, "2099-01-01T00:00:00Z",
  	); err != nil {
  		t.Fatalf("forge future version: %v", err)
  	}

  	// Re-opening should detect SchemaVersion+1 > SchemaVersion and refuse.
  	_, err = NewSQLiteStore(dsn)
  	if !errors.Is(err, ErrSchemaVersionAhead) {
  		t.Errorf("NewSQLiteStore err = %v, want errors.Is ErrSchemaVersionAhead", err)
  	}
  }
  ```

- [ ] **Step 2: Run to confirm PASS**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run 'TestSQLiteStore_(Migration_IsIdempotent|NewSQLiteStore_RefusesFuture)' -v`
  Expected: both PASS.

- [ ] **Step 3: Skip.**

- [ ] **Step 4: Skip.**

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/sqlite_store_test.go
  git commit -m "test(memory): pin migrator idempotency + future-schema refusal (C-2)"
  git push origin main
  ```

---

## Task 12: Concurrent same-key Save serialization test

**Files:**
- Modify: `llm-agent-memory/memory/sqlite_store_test.go` (append)

- [ ] **Step 1: Append a concurrency test that hammers the same key from multiple goroutines**

  ```go
  func TestSQLiteStore_Save_ConcurrentSameKey_SerializesCleanly(t *testing.T) {
  	store := newTempSQLiteStore(t)
  	ctx := context.Background()

  	const goroutines = 8
  	const writesPerGoroutine = 25
  	done := make(chan error, goroutines)
  	for g := 0; g < goroutines; g++ {
  		go func(g int) {
  			for i := 0; i < writesPerGoroutine; i++ {
  				snap := coremem.Snapshot{
  					Version: coremem.SnapshotVersion,
  					Kind:    coremem.KindWorking,
  					Items: []coremem.SnapshotItem{
  						{Item: coremem.MemoryItem{ID: "x", Content: "w"}, Vector: []float32{1}},
  					},
  				}
  				if err := store.Save(ctx, "race-key", snap); err != nil {
  					done <- err
  					return
  				}
  			}
  			done <- nil
  		}(g)
  	}
  	for i := 0; i < goroutines; i++ {
  		if err := <-done; err != nil {
  			t.Fatalf("goroutine err: %v", err)
  		}
  	}

  	// Exactly one row should exist for (race-key, working) regardless
  	// of write count — UPSERT collapsed all writes into one.
  	var n int
  	if err := store.db.QueryRowContext(ctx,
  		`SELECT COUNT(*) FROM memory_snapshots WHERE key = ? AND kind = ?`,
  		"race-key", string(coremem.KindWorking),
  	).Scan(&n); err != nil {
  		t.Fatalf("count: %v", err)
  	}
  	if n != 1 {
  		t.Errorf("post-race rows = %d, want 1", n)
  	}
  }
  ```

- [ ] **Step 2: Run with the race detector**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestSQLiteStore_Save_ConcurrentSameKey_SerializesCleanly -race -v`
  Expected: PASS, no race warnings.

- [ ] **Step 3: Skip.**

- [ ] **Step 4: Skip.**

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/sqlite_store_test.go
  git commit -m "test(memory): pin SQLiteStore concurrent-same-key Save semantics (C-2)"
  git push origin main
  ```

---

# Integration (Tasks 13–14)

## Task 13: `coremem.Manager.ExportAll` → `SQLiteStore` → `ImportAll` integration round-trip

**Files:**
- Modify: `llm-agent-memory/memory/sqlite_store_test.go` (append)

This is the C-2 exit-criterion proof: snapshots produced by core's `ExportAll` survive a SQLite round-trip and restore via core's `ImportAll`.

- [ ] **Step 1: Append the integration test**

  ```go
  func TestSQLiteStore_RoundTripsThroughCoreExportAllImportAll(t *testing.T) {
  	store := newTempSQLiteStore(t)

  	// Build a manager wired to use the SQLite store as its persistence
  	// backend. This proves SQLiteStore is a drop-in for FilesystemStore
  	// at the coremem.ManagerOptions.SnapshotStore slot.
  	mgr, err := coremem.NewManager(coremem.ManagerOptions{
  		Working:       newCoreWorking(t),
  		Episodic:      newCoreEpisodic(t),
  		Semantic:      newCoreSemantic(t),
  		SnapshotStore: store,
  	})
  	if err != nil {
  		t.Fatalf("NewManager: %v", err)
  	}

  	ctx := context.Background()
  	// Seed every active kind so all three snapshots round-trip.
  	if _, err := mgr.Add(ctx, coremem.KindWorking, coremem.MemoryItem{Content: "w1", Importance: 0.3}); err != nil {
  		t.Fatalf("Add working: %v", err)
  	}
  	if _, err := mgr.Add(ctx, coremem.KindEpisodic, coremem.MemoryItem{Content: "e1", Importance: 0.5}); err != nil {
  		t.Fatalf("Add episodic: %v", err)
  	}
  	if _, err := mgr.Add(ctx, coremem.KindSemantic, coremem.MemoryItem{Content: "s1", Tags: []string{"tag"}, Importance: 0.7}); err != nil {
  		t.Fatalf("Add semantic: %v", err)
  	}

  	// Export → persisted via SQLiteStore.Save under key "session-rt".
  	exported, err := mgr.ExportAll(ctx, "session-rt")
  	if err != nil {
  		t.Fatalf("ExportAll: %v", err)
  	}
  	if len(exported) != 3 {
  		t.Fatalf("ExportAll produced %d kinds, want 3", len(exported))
  	}

  	// Build a SECOND, empty manager backed by the SAME store and
  	// ImportAll from the persisted snapshots — proves Save was real.
  	mgr2, err := coremem.NewManager(coremem.ManagerOptions{
  		Working:       newCoreWorking(t),
  		Episodic:      newCoreEpisodic(t),
  		Semantic:      newCoreSemantic(t),
  		SnapshotStore: store,
  	})
  	if err != nil {
  		t.Fatalf("NewManager #2: %v", err)
  	}
  	report, err := mgr2.ImportAll(ctx, nil, "session-rt", coremem.ImportReplace)
  	if err != nil {
  		t.Fatalf("ImportAll: %v", err)
  	}
  	if len(report) != 3 {
  		t.Errorf("ImportAll report kinds = %d, want 3", len(report))
  	}
  	for kind, rpt := range report {
  		if rpt.Loaded == 0 {
  			t.Errorf("kind %v: Loaded = 0, want > 0", kind)
  		}
  	}

  	// Confirm the restored manager Search returns the expected content.
  	hits, err := mgr2.Search(ctx, coremem.KindEpisodic, "e1", 5)
  	if err != nil {
  		t.Fatalf("Search episodic: %v", err)
  	}
  	if len(hits) == 0 {
  		t.Error("Search returned 0 hits for restored episodic content")
  	}
  }
  ```

- [ ] **Step 2: Run the integration test**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestSQLiteStore_RoundTripsThroughCoreExportAllImportAll -v`
  Expected: PASS.

  If `coremem.ManagerOptions` (v0.7.0) names the snapshot-store field something other than `SnapshotStore`, the executor MUST first run `go doc github.com/costa92/llm-agent/memory.ManagerOptions` to confirm the actual field name and update the literal. The expected field name per the manager.go:31-32 comment block is `SnapshotStore`. If `go doc` reports it as `Store`, change the struct literal to `Store: store,` and re-run.

- [ ] **Step 3: Skip.**

- [ ] **Step 4: Skip.**

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/sqlite_store_test.go
  git commit -m "test(memory): pin SQLiteStore round-trip through core ExportAll/ImportAll (C-2)"
  git push origin main
  ```

---

## Task 14: Version bump + CHANGELOG + final verification gate

**Files:**
- Modify: `llm-agent-memory/memory/version.go`
- Modify: `llm-agent-memory/CHANGELOG.md`

- [ ] **Step 1: Bump version**

  Edit `llm-agent-memory/memory/version.go` to:

  ```go
  package memory

  // Version is the current llm-agent-memory release tag (semver).
  // Bumped at every tagged release; see CHANGELOG.md in the module root.
  const Version = "0.3.0"
  ```

  The existing `version_test.go` semver-shape test (3 numeric dot-separated parts) keeps passing with `0.3.0`; no change there.

- [ ] **Step 2: Prepend the 0.3.0 entry to `llm-agent-memory/CHANGELOG.md`** (above the existing `## [0.2.0]` entry)

  ```markdown
  ## [0.3.0] - 2026-05-26

  ### Added

  - `WritePolicy` interface with `Decide(ctx, ProposedWrite) WritePolicyDecision`
    covering all four documented decisions from `docs/memory-roadmap.zh-CN.md`
    §4.3 (C-1): user-saved, agent-inferred, reject, redact. Includes
    `Verdict` enum (`accept` / `redact` / `reject`), `WriteSource` enum
    (`user_saved` / `agent_inferred` / `system`), and the `PolicyFunc`
    function-to-interface adapter.
  - `PolicyEnforcingMemory` wrapper that consumes a `WritePolicy` and
    translates each verdict into a `*coremem.Manager.Add` call (or a
    rejection with the aliased `ErrRejectedByPolicy`). Reroutes the
    write kind when the policy returns a different `Kind` than the
    input.
  - `PolicyAdapter` lets a `WritePolicy` satisfy the existing
    `coremem.Sanitizer` interface for callers wired to `WithSanitizer`.
    Returns `ErrPolicyKindRerouteUnsupported` if the policy attempts
    to reroute kinds (Sanitizer's return triple has no kind slot).
  - `SQLiteStore` (C-2): first non-filesystem implementation of
    `coremem.SnapshotStore`. Implements `Save` / `Load` / `LoadKind` /
    `Delete` / `List`. Idempotent in-code migrator (schema v1, two
    tables, one index) with future-version refusal via
    `ErrSchemaVersionAhead`. Round-trips through `coremem.Manager.
    ExportAll` / `ImportAll`.
  - `EventWritePolicyDecided` observer event, emitted by
    `PolicyEnforcingMemory.Add` for all three verdicts. Attrs schema:
    `verdict`, `input_kind`, `decided_kind`, `source`, `reason`.

  ### Dependencies

  - First third-party dependency: `modernc.org/sqlite` (pure-Go, no CGO).
    Justification: enables a non-filesystem SnapshotStore without
    forcing CGO on downstream siblings or breaking cross-compile.
    Core `llm-agent` remains stdlib-only — this dep is contained to
    the sibling.
  ```

- [ ] **Step 3: Run the entire sibling suite with the race detector**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./... -count=1 -race`
  Expected: `ok github.com/costa92/llm-agent-memory/memory ...`, exit code 0, no race warnings. Every M0–M3 test passes.

  Also run: `GOWORK=off go vet ./...`
  Expected: no output.

- [ ] **Step 4: Confirm the stdlib-only gate still passes on core**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem && bash scripts/stdlib-only-check.sh`
  Expected: `stdlib-only-check: PASS`. The new `modernc.org/sqlite` dep lives in `llm-agent-memory/go.mod`, NOT in `llm-agent/go.mod`; the gate scans the core module only and must remain clean.

  Also run the umbrella sibling matrix: `bash scripts/eco.sh test`
  Expected: every sibling passes.

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory
  git add memory/version.go CHANGELOG.md
  git commit -m "release(memory): bump to v0.3.0 (M3: WritePolicy + SQLiteStore)"
  git push origin main
  ```

---

## Task 15: Tag `v0.3.0`

**Files:** none (git operation only).

- [ ] **Step 1: Verify the sibling working tree is clean and on `main`**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && git status` and `git log --oneline -10`
  Expected: clean tree on `main`; the last commit is `release(memory): bump to v0.3.0 (M3: WritePolicy + SQLiteStore)`.

- [ ] **Step 2: Reconfirm Task 14 gate**

  Same cadence as v0.2.0: if Task 14 was green, proceed to tag immediately. Sanity-check by re-running `GOWORK=off go test -race ./...` from the sibling root and `bash scripts/stdlib-only-check.sh` from the umbrella root. Both must be green before Step 3.

- [ ] **Step 3: Create the annotated tag**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && git tag -a v0.3.0 -m "llm-agent-memory v0.3.0 — WritePolicy interface + SQLiteStore (M3)"`
  Expected: tag created locally. Use unprefixed `v<X.Y.Z>` format to match existing `v0.1.0` and `v0.2.0` tags.

- [ ] **Step 4: Push the tag to sibling's `origin`**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && git push origin v0.3.0`
  Expected: tag uploaded to the sibling's GitHub remote. Confirm with `git tag --list 'v*'` — expect `v0.1.0`, `v0.2.0`, `v0.3.0`.

- [ ] **Step 5: No commit (the tag is the artifact).**

---

## Self-Review

### Exit-criteria mapping (from master roadmap M3)

| # | Master-roadmap exit criterion | Plan task(s) that satisfy it |
|---|---|---|
| 1 | `WritePolicy` interface with `Decide(ctx, input) -> {kind, importance, tags, keep}`; covers user-saved, agent-inferred, reject, redact. | Tasks 1, 2, 3, 4, 5, 6. Task 6 explicitly pins all four documented decisions in one table-driven proof test. (Note: `keep` from the roadmap shorthand maps to the `Verdict` enum — `keep=true` ↔ `VerdictAccept`/`VerdictRedact`, `keep=false` ↔ `VerdictReject`. `importance` and `tags` flow through `WritePolicyDecision.Item.Importance` and `.Tags`.) |
| 2 | `SQLiteStore` implementing `SnapshotStore`; Save/Load/Delete/List all tested. | Tasks 7, 8, 9, 10. Task 7 asserts interface compliance; Task 9 tests Save/Load round-trip; Task 10 tests Delete + List + UPSERT-overwrite. |
| 3 | Round-trips through `ImportAll` / `ExportAll`. | Task 13 — the explicit integration test that builds two `*coremem.Manager` instances around the same `*SQLiteStore` and verifies ExportAll-then-ImportAll. |
| 4 | Stdlib-only assertion still passes for core `llm-agent` (new deps added to `llm-agent-memory` only). | Task 14 Step 4 reruns `scripts/stdlib-only-check.sh`. Dep is added to `llm-agent-memory/go.mod` only (Task 8 Step 1); `llm-agent/go.mod` is never touched. |
| 5 | Migration script or in-code migrator for store schema v1. | Task 8 (in-code migrator with `memory_store_schema` table). Task 11 pins idempotency + future-version refusal as a regression detector. |

### Coverage check

- Every documented WritePolicy decision (user-saved, agent-inferred, reject, redact) — Task 6, one row per decision.
- Every `SnapshotStore` method (Save, Load, Delete, List) — Tasks 9 + 10.
- Optional `LoadKind` consumed by `coremem.Manager.ImportAll` — Task 9 sub-tests 2 and 3.
- Observer parity with M2 conventions — Task 4 (three verdict-specific tests).
- Concurrency safety of the SQL store — Task 12 (race-tested).
- End-to-end integration with core's Export/Import — Task 13.

### Type consistency

- `WritePolicy` / `ProposedWrite` / `WritePolicyDecision` / `Verdict` / `WriteSource` — defined once in `write_policy.go`, referenced uniformly across tests.
- `PolicyEnforcingMemory` / `NewPolicyEnforcingMemory` / `PolicyAdapter` / `PolicyFunc` — single definitions.
- `ErrRejectedByPolicy` aliases `coremem.ErrRejectedByPolicy` (intentional; documented).
- `ErrPolicyEnforcingManagerRequired`, `ErrPolicyRequired`, `ErrPolicyKindRerouteUnsupported`, `ErrSchemaVersionAhead`, `ErrSQLiteDSNRequired` — five new sentinels; each defined exactly once.
- `SchemaVersion` constant — single source of truth; the migrator iterates a `migrations` slice whose max element MUST equal `SchemaVersion`.
- `EventWritePolicyDecided` — single constant declared in `observer.go`; tests + emit-sites import from there.
- `Option` / `config` / `newConfig` / `WithObserver` — reused from M2's `observer.go` (NOT redefined).

### Placeholder scan

- No `TODO`, `tbd`, `similar to above`, or `implement later` in any code block.
- Task 3 Step 4 carries one intentional FIXME-then-removed sequence to keep the task self-contained; Task 4 Step 4 removes both FIXMEs.
- Every shell command uses an absolute path and shows expected output.

### Test-name disambiguation from M1/M2

Every new test name starts with a fresh, disjoint prefix. Verified:

- `TestWritePolicy_*` — new prefix, no M1/M2 collisions.
- `TestPolicyEnforcingMemory_*` — new prefix.
- `TestPolicyAdapter_*` — new prefix.
- `TestSQLiteStore_*` — new prefix.

No collisions with M1's `TestScopedLifecycle_*`, `TestConsolidator_*`, `TestUnifiedSearcher_*`, or M2's `TestObserver_*`, `TestParallelSearcher_*` families.

### Stdlib-only check (for `llm-agent` core; sibling is now NOT stdlib-only)

- `llm-agent-memory/memory/write_policy.go` imports: `context`, `errors`, `github.com/costa92/llm-agent/memory`. No third-party.
- `llm-agent-memory/memory/sqlite_store.go` imports: `context`, `database/sql`, `encoding/json`, `errors`, `fmt`, `os`, `strings`, `time`, `github.com/costa92/llm-agent/memory`, `modernc.org/sqlite` (registration-only). One third-party dep, contained to this sibling per master-roadmap §3 dependency policy.
- `llm-agent-memory/memory/sqlite_store_test.go` imports: `context`, `errors`, `os`, `testing`, `time`, `github.com/costa92/llm-agent/memory`. No third-party in tests.
- `llm-agent-memory/memory/write_policy_test.go` imports: `context`, `errors`, `testing`, `github.com/costa92/llm-agent/memory`. No third-party in tests.
- `llm-agent/` core module: untouched. `scripts/stdlib-only-check.sh` is rerun at Task 14 Step 4 and must pass.

### Lessons-from-M2 audit

- No `len(page.Items) > 0` guards added — M3 does not introduce iteration helpers.
- No bit-exact `reflect.DeepEqual` on time-bearing structures — `assertSnapshotEqual` uses `time.Time.Equal()` instead.
- No stuttering error wraps — every wrap site adds exactly one prefix segment.
- `observer()` accessor used consistently — `PolicyEnforcingMemory.Add` calls `p.observer()` at both emit sites.
- Upstream interfaces (`coremem.SnapshotStore`, `coremem.Sanitizer`, `coremem.ManagerOptions.SnapshotStore`) verified via `go doc` before writing (Task 8 Step 2; Task 13 Step 2 fallback note).
- Sibling commit topology used everywhere — every commit goes to `llm-agent-memory`'s own `origin/main`; umbrella never sees these commits except via the v0.3.0 tag.
- Tag format unprefixed (`v0.3.0`) — matches existing v0.1.0/v0.2.0 in this sibling.
- Variadic-options pattern reuses the M2 `Option`/`config`/`newConfig` machinery — no redefinition.

No drift detected. Plan is ready to execute.
