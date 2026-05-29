# llm-agent-memory-contract Extraction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (- [ ]) syntax for tracking.

**Goal:** Extract the durable contract layer (`llm-agent-memory/memory/durable.go`) verbatim into a new stdlib-only Go module `llm-agent-memory-contract`, repoint postgres/worker/gateway onto it, leave an alias-shim in `llm-agent-memory` for one release cycle, and add release-skew CI so tagged-graph version skew can no longer be masked by `go.work`/`replace`.

**Architecture:** The workspace is a `go.work` umbrella of independent, separately-tagged Go modules (NOT a monorepo). `durable.go` is the persisted JSON schema (no `json` tags, so wire keys equal Go field names) plus the 8 storage-port interfaces consumed by `llm-agent-memory-postgres`, `llm-agent-memory-worker`, and `llm-agent-memory-gateway`. Extraction is byte-for-byte field-preserving so the on-disk Postgres JSON is untouched; consumers swap one import path and drop the `llm-agent-memory` (and transitively `llm-agent`) dependency where possible. Because local `go.work`+`replace` resolution hides version skew, the release is executed as a sequenced wave with `GOWORK=off` verification at every tag.

**Tech Stack:** Go 1.26.0, standard library only for the contract module (`context`, `errors`, `fmt`, `strings`, `time`), `go.work` umbrella, GitHub Actions (`.github/workflows/umbrella.yml`), `scripts/eco.sh` helper, Postgres+pgvector (downstream only), `git` tags.

> **VERIFIED REPO FACTS (read before executing — they correct the original brief):**
> - **Go version is `1.26.0`** in every module's `go.mod` and the umbrella `go.work`; CI pins `go-version: '1.26.0'`. (The brief said 1.25.0 — it is wrong.)
> - **There are NO per-module tags.** The only tag in the umbrella repo is `v0.1.0`. Each sibling is an INDEPENDENT git repo (the umbrella CI checks them out via `actions/checkout` with `repository: costa92/<module>`). Downstream `go.mod` files require siblings as `vX.Y.Z = v0.0.0` PLACEHOLDERS plus a local `replace ... => ../<dir>`; the real version is supplied by the local checkout, not a published tag. Therefore "tag" steps in this plan create NEW first tags in each sibling repo, and downstream `require` lines KEEP the `v0.0.0` placeholder + `replace` (the umbrella never resolves a published version). The `release-skew` work in Phase 6 is about catching skew once those modules ARE published, and about asserting the `replace`-free graph is internally consistent.
> - `go.work` already lists `./llm-agent-memory-worker`; it does NOT yet list `./llm-agent-memory-contract` (Task 1 adds it).
> - `scripts/eco.sh` `all_repos` currently has 9 entries: `llm-agent, llm-agent-rag, llm-agent-otel, llm-agent-providers, llm-agent-customer-support, llm-agent-flow, llm-agent-memory, llm-agent-memory-gateway, llm-agent-memory-postgres`. It is MISSING both `llm-agent-memory-worker` and `llm-agent-memory-contract`, and also has no `repo_url`/`is_launchable` knowledge of them.
> - `.github/workflows/umbrella.yml` is ONE `cross-repo-build` job that checks out each sibling repo into a path and runs `GOWORK=off go vet/build/test` per `working-directory` (plus `smoke`, `stdlib-only-gate` jobs). It does NOT have one job per module, and it does NOT currently check out `llm-agent-memory-worker` or `llm-agent-memory-contract`.
> - **All 34 consumer symbols are durable-contract symbols; ZERO engine/alias types appear in any consumer (prod or test).** Verified by enumerating every `corememory.*` usage across postgres+worker+gateway and grepping for engine names (`New*`, `Working`, `Engine`, `Manager`, `Kind`, `Stats`, `Embedder`, `Scope`, `SearchResult`, `AdaptCore`): NONE found. No BLOCKER. The repoint is purely mechanical.

---

## File Structure

### Created

| Path | Responsibility |
|---|---|
| `llm-agent-memory-contract/go.mod` | New module manifest: `module github.com/costa92/llm-agent-memory-contract`, `go 1.26.0`, zero requires. |
| `llm-agent-memory-contract/contract/durable.go` | Verbatim copy of `llm-agent-memory/memory/durable.go` with package renamed `memory` → `contract`. The highest-stability API: `MemoryRecord`, `StoredEvent`, `OutboxMessage`, `IdempotencyEntry`, all `*Input`/`*Result` DTOs, `DedupeAction`, 8 interfaces, `RecordKind*`/`Dedupe*` constants, `ErrInvalidRecordKind`, `NormalizeRecordKind`/`NormalizeWriteDefaults`/`SetWorkingDefault`. |
| `llm-agent-memory-contract/contract/durable_test.go` | Verbatim copy of `llm-agent-memory/memory/durable_test.go` (package renamed). Unit tests for `NormalizeRecordKind` / `NormalizeWriteDefaults` / `SetWorkingDefault`. |
| `llm-agent-memory-contract/contract/golden_wire_test.go` | NEW. Golden JSON round-trip test asserting the persisted wire shape of `MemoryRecord`/`StoredEvent`/`OutboxMessage`/`IdempotencyEntry` is byte-for-byte unchanged. Golden string captured at execution time (Task 2). |
| `llm-agent-memory-contract/doc.go` | Package-level `// Package contract ...` doc stating the SemVer + DB-schema compatibility policy. |
| `llm-agent-memory-contract/README.md` | Module overview, stability contract, "do not change field names/pointer-or-value forms/types" rule, release cadence. |
| `llm-agent-memory-contract/CODEOWNERS` | Owner entry making the contract module require explicit review. |
| `llm-agent-memory/memory/durable_shim.go` | NEW (Phase 5). `=`-alias re-exports of every contract symbol so existing `llm-agent-memory/memory` importers keep compiling for one release cycle. |

### Modified

| Path | Responsibility |
|---|---|
| `go.work` | Add `./llm-agent-memory-contract` to the `use` block. |
| `scripts/eco.sh` | Add `llm-agent-memory-contract` and `llm-agent-memory-worker` to `all_repos`; add `release-check` (GOWORK=off build+test+tidy-verify per repo) and `tag` helper. |
| `.github/workflows/umbrella.yml` | Add `llm-agent-memory-worker` + `llm-agent-memory-contract` build+test jobs; add a `release-skew` job that runs each downstream with `GOWORK=off` AND `replace` directives stripped, against the real required version. |
| `llm-agent-memory-postgres/go.mod` | Drop `github.com/costa92/llm-agent-memory` require+replace; add `github.com/costa92/llm-agent-memory-contract` require+replace. |
| `llm-agent-memory-postgres/postgres/store.go` | Import path `llm-agent-memory/memory` → `llm-agent-memory-contract/contract` (alias `corememory` retained). Import at line 9; holds 7 `var _ corememory.X = (*Store)(nil)` assertions (lines 57-63). |
| `llm-agent-memory-postgres/postgres/relay.go` | Same import repoint (line 9; holds `var _ corememory.MessagePublisher = (Publisher)(nil)` at line 21). |
| `llm-agent-memory-postgres/postgres/fanout_publisher.go` | Same import repoint (line 7). |
| `llm-agent-memory-postgres/postgres/lease_aware_publisher.go` | Same import repoint (line 6). |
| `llm-agent-memory-postgres/postgres/store_test.go` | Same import repoint (holds `TestPostgresJSON_MemoryRecordRoundTrip` at line 177). |
| `llm-agent-memory-postgres/postgres/mutation_test.go` | Same import repoint. |
| `llm-agent-memory-postgres/postgres/write_test.go` | Same import repoint. |
| `llm-agent-memory-postgres/postgres/read_test.go` | Same import repoint. |
| `llm-agent-memory-postgres/postgres/access_test.go` | Same import repoint. |
| `llm-agent-memory-postgres/postgres/dedupe_test.go` | Same import repoint. |
| `llm-agent-memory-postgres/postgres/promote_test.go` | Same import repoint. |
| `llm-agent-memory-postgres/postgres/relay_test.go` | Same import repoint. |
| `llm-agent-memory-postgres/postgres/fanout_publisher_test.go` | Same import repoint. |
| `llm-agent-memory-postgres/postgres/session_working_test.go` | Same import repoint. |
| `llm-agent-memory-worker/go.mod` | Drop `llm-agent-memory` require+replace; add `llm-agent-memory-contract` require+replace (keep `v0.0.0` placeholder + local replace); keep the existing `llm-agent-memory-postgres` require+replace. |
| `llm-agent-memory-worker/internal/service/consolidation_publisher.go` | Import repoint (line 12) + keep the `pgmemory "github.com/costa92/llm-agent-memory-postgres/postgres"` import (line 11). |
| `llm-agent-memory-worker/internal/service/consolidation_publisher_test.go` | Import repoint. |
| `llm-agent-memory-worker/internal/service/metrics_test.go` | Import repoint. |
| `llm-agent-memory-gateway/go.mod` | Add `llm-agent-memory-contract` require+replace; drop the direct `llm-agent-memory` require+replace (gateway stops importing `llm-agent-memory/memory` after the repoint); KEEP `llm-agent-rag v1.0.5` + its replace (rag transitively keeps `llm-agent v0.7.0 // indirect`). |
| `llm-agent-memory-gateway` consumers (19 files, all import alias `corememory`): 8 prod = `cmd/memory-gateway/main.go`, `internal/service/{service.go, durable_session_closer.go, hybrid_recaller.go, outbox_vector_publisher.go, postgres_recall_store.go, recall_selection.go, vector_projector.go}`; 11 test = `cmd/memory-gateway/main_test.go`, `internal/service/{service_test.go, durable_session_closer_test.go, embedding_metrics_test.go, hybrid_recaller_test.go, outbox_vector_publisher_test.go, postgres_recall_store_test.go, recall_cache_test.go, recall_selection_test.go, vector_projector_test.go}`, `internal/transport/smoke_test.go` | Import repoint. |
| `llm-agent-memory/go.mod` | Add `github.com/costa92/llm-agent-memory-contract` require+replace (Phase 5). |
| `llm-agent-memory/memory/durable.go` | Phase 5: DELETE this file entirely; the types it defined now live in the contract module and are re-exported by `durable_shim.go`. |
| `llm-agent-memory/memory/durable_test.go` | Phase 5: DELETE (moved to the contract module in Phase 1). |

---

## Out of scope / follow-on plans

These are deliberately NOT in this plan and must be tracked as separate deferred plans:

1. **Gateway write-idempotency gap (design §8).** Only `POST /memory/write` carries an `idempotency_key`; `PATCH/pin/unpin/disable/enable/DELETE/close/heartbeat` have no idempotency token and there is no `GET /memory/items/{id}` to reconcile after a timeout. This is a real bug and the prerequisite for Proposal 1, but it does not touch the contract extraction. File as its own issue/plan.
2. **Proposal 1 (cluster client `llm-agent-memory-client`).** Deferred per design §7 until the idempotency gap is closed. The client must be built on gateway `httpapi` types (pointer-PATCH semantics), NOT the contract DTOs — so it is unrelated to this module move.

---

## Phase 1 — Create + tag `llm-agent-memory-contract`

### Task 1: Scaffold the module and move `durable.go` verbatim

**Files:**
- Create: `llm-agent-memory-contract/go.mod`
- Create: `llm-agent-memory-contract/contract/durable.go`
- Create: `llm-agent-memory-contract/contract/durable_test.go`
- Modify: `go.work`

Steps:

- [ ] Create the module directory and manifest. Write `llm-agent-memory-contract/go.mod` with exactly:
  ```
  module github.com/costa92/llm-agent-memory-contract

  go 1.26.0
  ```
- [ ] Copy the file verbatim, then rename the package. Run:
  ```
  mkdir -p llm-agent-memory-contract/contract
  cp llm-agent-memory/memory/durable.go llm-agent-memory-contract/contract/durable.go
  cp llm-agent-memory/memory/durable_test.go llm-agent-memory-contract/contract/durable_test.go
  ```
- [ ] Change ONLY the package clause (line 1) of `llm-agent-memory-contract/contract/durable.go` from `package memory` to `package contract`. Do NOT touch any field name, pointer-vs-value form, type, or add any `json` tag — the persisted Postgres JSON schema depends on byte-for-byte field identity.
- [ ] Change ONLY the package clause (line 1) of `llm-agent-memory-contract/contract/durable_test.go` from `package memory` to `package contract`. The test imports are stdlib-only (`errors`, `testing`) and reference unqualified symbols, so no other edit is needed. It contains exactly 5 funcs: `TestNormalizeRecordKind_DefaultsBlankToEpisodic`, `TestNormalizeRecordKind_AcceptsCanonicalKinds`, `TestNormalizeRecordKind_RejectsUnknownKind`, `TestMemoryRecordNormalizeWriteDefaults_DefaultsBlankKind`, `TestMemoryRecordSetWorkingDefault_OnlyFillsBlankKind`.
- [ ] Verify the moved unit tests pass in isolation:
  ```
  cd llm-agent-memory-contract && GOWORK=off go test ./contract/
  ```
  Expected: `ok  	github.com/costa92/llm-agent-memory-contract/contract`.
- [ ] Confirm the module is stdlib-only (no requires leaked in):
  ```
  cd llm-agent-memory-contract && GOWORK=off go mod tidy && git diff --exit-code go.mod
  ```
  Expected: exit 0, no `require` block added (durable.go imports only `context errors fmt strings time`).
- [ ] Add the module to the workspace. The current `go.work` `use (` block is (keep order, insert `./llm-agent-memory-contract` immediately before `./llm-agent-memory`):
  ```
  use (
  	./llm-agent
  	./llm-agent-customer-support
  	./llm-agent-flow
  	./llm-agent-memory-contract
  	./llm-agent-memory
  	./llm-agent-memory-gateway
  	./llm-agent-memory-postgres
  	./llm-agent-memory-worker
  	./llm-agent-otel
  	./llm-agent-providers
  	./llm-agent-rag
  )
  ```
  Verify:
  ```
  go work sync && go build ./llm-agent-memory-contract/...
  ```
  Expected: no output, exit 0.
- [ ] Commit:
  ```
  git add llm-agent-memory-contract/go.mod llm-agent-memory-contract/contract/durable.go llm-agent-memory-contract/contract/durable_test.go go.work
  git commit -m "feat(memory-contract): scaffold module + move durable.go verbatim"
  ```

### Task 2: Capture the golden wire JSON and add the round-trip guard test

**Files:**
- Create: `llm-agent-memory-contract/contract/golden_wire_test.go`

The persisted schema preservation is the single highest-risk property of this whole plan: `MemoryRecord`/`StoredEvent`/`OutboxMessage`/`IdempotencyEntry` are marshaled with default `encoding/json` (no tags) straight into Postgres. This test pins the exact wire bytes. The golden strings are RUNTIME-CAPTURED from the pre-move type so they encode the real current shape, not an invented one.

Steps:

- [ ] Write the test scaffold WITHOUT the golden constants yet, so the capture step has the exact fixtures. Create `llm-agent-memory-contract/contract/golden_wire_test.go`:
  ```go
  package contract

  import (
  	"encoding/json"
  	"testing"
  	"time"
  )

  // fixtureRecord is the canonical MemoryRecord used to pin the persisted JSON
  // wire shape. durable.go carries NO json tags, so the wire keys equal the Go
  // field names; any rename / pointer-vs-value change / type change will break
  // this test and signal a Postgres-schema-breaking change.
  func fixtureRecord() MemoryRecord {
  	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
  	deletedAt := now.Add(time.Hour)
  	lastAccess := now.Add(2 * time.Hour)
  	return MemoryRecord{
  		MemoryID:                "mem-1",
  		TenantID:                "tenant-1",
  		UserID:                  "user-1",
  		ProjectID:               "proj-1",
  		SessionID:               "sess-1",
  		Kind:                    RecordKindSemantic,
  		Source:                  "src",
  		Category:                "cat",
  		Content:                 "hello",
  		NormalizedContentHash:   "hash",
  		Tags:                    []string{"a", "b"},
  		Importance:              0.5,
  		Pinned:                  true,
  		Disabled:                false,
  		Deleted:                 false,
  		Version:                 7,
  		CreatedAt:               now,
  		UpdatedAt:               now,
  		DeletedAt:               &deletedAt,
  		LastAccessAt:            &lastAccess,
  		HitCount:                3,
  		ConsolidatedFromEventID: "evt-9",
  	}
  }

  const goldenMemoryRecordJSON = `__CAPTURE_ME__`
  const goldenStoredEventJSON = `__CAPTURE_ME__`
  const goldenOutboxMessageJSON = `__CAPTURE_ME__`
  const goldenIdempotencyEntryJSON = `__CAPTURE_ME__`

  func TestGoldenWire_MemoryRecord(t *testing.T) {
  	b, err := json.Marshal(fixtureRecord())
  	if err != nil {
  		t.Fatalf("marshal: %v", err)
  	}
  	if string(b) != goldenMemoryRecordJSON {
  		t.Fatalf("wire shape changed (Postgres-breaking!):\n got: %s\nwant: %s", b, goldenMemoryRecordJSON)
  	}
  	var back MemoryRecord
  	if err := json.Unmarshal(b, &back); err != nil {
  		t.Fatalf("unmarshal: %v", err)
  	}
  }

  func TestGoldenWire_StoredEvent(t *testing.T) {
  	ev := StoredEvent{
  		Version:        7,
  		TenantID:       "tenant-1",
  		MemoryID:       "mem-1",
  		EventType:      "memory_written",
  		IdempotencyKey: "idem-1",
  		Record:         fixtureRecord(),
  		Metadata:       map[string]any{"k": "v"},
  	}
  	b, err := json.Marshal(ev)
  	if err != nil {
  		t.Fatalf("marshal: %v", err)
  	}
  	if string(b) != goldenStoredEventJSON {
  		t.Fatalf("wire shape changed (Postgres-breaking!):\n got: %s\nwant: %s", b, goldenStoredEventJSON)
  	}
  }

  func TestGoldenWire_OutboxMessage(t *testing.T) {
  	msg := OutboxMessage{
  		Version:   7,
  		TenantID:  "tenant-1",
  		MemoryID:  "mem-1",
  		EventType: "memory_written",
  		EventID:   "evt-1",
  		Record:    fixtureRecord(),
  		Metadata:  map[string]any{"k": "v"},
  	}
  	b, err := json.Marshal(msg)
  	if err != nil {
  		t.Fatalf("marshal: %v", err)
  	}
  	if string(b) != goldenOutboxMessageJSON {
  		t.Fatalf("wire shape changed (Postgres-breaking!):\n got: %s\nwant: %s", b, goldenOutboxMessageJSON)
  	}
  }

  func TestGoldenWire_IdempotencyEntry(t *testing.T) {
  	expires := time.Date(2024, 1, 9, 3, 4, 5, 0, time.UTC)
  	ent := IdempotencyEntry{
  		TenantID:       "tenant-1",
  		IdempotencyKey: "idem-1",
  		RequestHash:    "rh",
  		MemoryID:       "mem-1",
  		Response: WriteRecordResult{
  			MemoryID: "mem-1",
  			Version:  7,
  			Created:  true,
  			Record:   fixtureRecord(),
  		},
  		CreatedAt: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
  		ExpiresAt: &expires,
  	}
  	b, err := json.Marshal(ent)
  	if err != nil {
  		t.Fatalf("marshal: %v", err)
  	}
  	if string(b) != goldenIdempotencyEntryJSON {
  		t.Fatalf("wire shape changed (Postgres-breaking!):\n got: %s\nwant: %s", b, goldenIdempotencyEntryJSON)
  	}
  }
  ```
- [ ] Run the test; it MUST FAIL because the golden constants are placeholders:
  ```
  cd llm-agent-memory-contract && GOWORK=off go test ./contract/ -run TestGoldenWire
  ```
  Expected: FAIL with `wire shape changed` on all four.
- [ ] CAPTURE the real golden bytes from the contract type (which is byte-identical to the pre-move `llm-agent-memory/memory` type). Add a temporary capture helper file `llm-agent-memory-contract/contract/zz_capture_test.go`:
  ```go
  package contract

  import (
  	"encoding/json"
  	"testing"
  	"time"
  )

  func TestZZCaptureGolden(t *testing.T) {
  	dump := func(label string, v any) {
  		b, _ := json.Marshal(v)
  		t.Logf("%s=%s", label, b)
  	}
  	expires := time.Date(2024, 1, 9, 3, 4, 5, 0, time.UTC)
  	dump("MEMREC", fixtureRecord())
  	dump("EVENT", StoredEvent{Version: 7, TenantID: "tenant-1", MemoryID: "mem-1", EventType: "memory_written", IdempotencyKey: "idem-1", Record: fixtureRecord(), Metadata: map[string]any{"k": "v"}})
  	dump("OUTBOX", OutboxMessage{Version: 7, TenantID: "tenant-1", MemoryID: "mem-1", EventType: "memory_written", EventID: "evt-1", Record: fixtureRecord(), Metadata: map[string]any{"k": "v"}})
  	dump("IDEM", IdempotencyEntry{TenantID: "tenant-1", IdempotencyKey: "idem-1", RequestHash: "rh", MemoryID: "mem-1", Response: WriteRecordResult{MemoryID: "mem-1", Version: 7, Created: true, Record: fixtureRecord()}, CreatedAt: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC), ExpiresAt: &expires})
  }
  ```
  Run and copy the printed JSON for each label:
  ```
  cd llm-agent-memory-contract && GOWORK=off go test ./contract/ -run TestZZCaptureGolden -v
  ```
  Expected: four `=...json...` log lines. Paste each captured JSON literal into the matching `golden*JSON` constant in `golden_wire_test.go` (replace each `__CAPTURE_ME__`).
- [ ] Delete the capture helper:
  ```
  rm llm-agent-memory-contract/contract/zz_capture_test.go
  ```
- [ ] Re-run the golden tests; they MUST now PASS:
  ```
  cd llm-agent-memory-contract && GOWORK=off go test ./contract/ -run TestGoldenWire -v
  ```
  Expected: `--- PASS` for all four `TestGoldenWire_*`.
- [ ] Cross-check the golden bytes against the ORIGINAL `llm-agent-memory` type to prove the move preserved the wire shape. From the original module:
  ```
  cd llm-agent-memory && GOWORK=off go test ./memory/ -run TestPostgresJSON 2>/dev/null; cd ..
  ```
  Note: `TestPostgresJSON_MemoryRecordRoundTrip` lives in `llm-agent-memory-postgres`, not here; the authoritative cross-check is that the captured `MEMREC` JSON encodes the SAME field set used by that test's fixture (`mem-1/tenant-1/.../evt-9`). Eyeball that the captured `goldenMemoryRecordJSON` contains the keys `MemoryID,TenantID,UserID,ProjectID,SessionID,Kind,Source,Category,Content,NormalizedContentHash,Tags,Importance,Pinned,Disabled,Deleted,Version,CreatedAt,UpdatedAt,DeletedAt,LastAccessAt,HitCount,ConsolidatedFromEventID` in struct order and no `json`-tag-renamed keys.
- [ ] Commit:
  ```
  git add llm-agent-memory-contract/contract/golden_wire_test.go
  git commit -m "test(memory-contract): pin persisted JSON wire shape (golden round-trip)"
  ```

### Task 3: Add `doc.go`, `README.md`, `CODEOWNERS` (highest-stability API policy)

**Files:**
- Create: `llm-agent-memory-contract/doc.go`
- Create: `llm-agent-memory-contract/README.md`
- Create: `llm-agent-memory-contract/CODEOWNERS`

Steps:

- [ ] Create `llm-agent-memory-contract/doc.go`:
  ```go
  // Package contract defines the backend-neutral durable memory contract shared
  // by llm-agent-memory-postgres, llm-agent-memory-worker, and
  // llm-agent-memory-gateway.
  //
  // STABILITY POLICY (read before editing):
  //
  //   - This is the highest-stability API in the ecosystem. MemoryRecord,
  //     StoredEvent, OutboxMessage, and IdempotencyEntry are serialized with
  //     the standard encoding/json (NO json tags) directly into Postgres. The
  //     wire keys therefore equal the Go field names.
  //   - Renaming a field, changing a field between value and pointer form,
  //     changing a field's type, or adding a json tag is a DATABASE MIGRATION,
  //     not a code change. Such changes require a major version bump and a
  //     migration plan.
  //   - Additive, optional fields appended at the end (still tag-free) are a
  //     minor version bump.
  //   - The interfaces (RecordStore, Promoter, Deduper, AccessMarker,
  //     EventStore, IdempotencyStore, Outbox, MessagePublisher) are consumed by
  //     three runtime modules; adding a method is a breaking change for
  //     implementers and requires a major version bump.
  //   - SemVer is enforced per-module via the git tag
  //     llm-agent-memory-contract/vX.Y.Z. The golden_wire_test.go guard MUST
  //     stay green; a red golden test means you are about to break persisted
  //     data.
  package contract
  ```
- [ ] Verify it compiles:
  ```
  cd llm-agent-memory-contract && GOWORK=off go build ./...
  ```
  Expected: no output, exit 0.
- [ ] Create `llm-agent-memory-contract/README.md`:
  ```markdown
  # llm-agent-memory-contract

  Stdlib-only, backend-neutral durable-memory contract for the llm-agent
  ecosystem. Extracted verbatim from `llm-agent-memory/memory/durable.go`.

  ## What it contains

  - `MemoryRecord` and the persisted aggregates `StoredEvent`, `OutboxMessage`,
    `IdempotencyEntry`.
  - All `*Input` / `*Result` DTOs and `DedupeAction`.
  - The 8 storage-port interfaces: `RecordStore`, `Promoter`, `Deduper`,
    `AccessMarker`, `EventStore`, `IdempotencyStore`, `Outbox`,
    `MessagePublisher`.
  - `RecordKind*` / `Dedupe*` constants, `ErrInvalidRecordKind`, and the
    `NormalizeRecordKind` / `NormalizeWriteDefaults` / `SetWorkingDefault`
    helpers.

  ## Stability contract

  This module is a **persisted JSON schema**, not just a DTO package. The four
  aggregate types are marshaled with the default `encoding/json` (no tags)
  straight into Postgres, so **wire keys equal Go field names**.

  Do NOT, without a major version bump and a DB migration plan:

  - rename a field;
  - change a field between value and pointer form;
  - change a field's type;
  - add a `json` tag.

  `golden_wire_test.go` pins the exact wire bytes. A red golden test means the
  change would corrupt previously-persisted rows.

  ## Versioning

  Independent module; tagged as `llm-agent-memory-contract/vX.Y.Z`. Consumers
  must pin the SAME contract version during a coordinated release wave (the
  alias shim in `llm-agent-memory` does NOT make mixed-version graphs safe).
  ```
- [ ] Create `llm-agent-memory-contract/CODEOWNERS`:
  ```
  # The durable contract is the highest-stability, DB-backed API.
  # Any change requires explicit owner review.
  *           @costa92
  /contract/  @costa92
  ```
- [ ] Commit:
  ```
  git add llm-agent-memory-contract/doc.go llm-agent-memory-contract/README.md llm-agent-memory-contract/CODEOWNERS
  git commit -m "docs(memory-contract): add doc.go stability policy + README + CODEOWNERS"
  ```

### Task 4: Add contract to `eco.sh` and tag the module

**Files:**
- Modify: `scripts/eco.sh`

Steps:

- [ ] Add both missing modules to the `all_repos` array in `scripts/eco.sh` (lines 6-16). The current array is exactly:
  ```bash
  all_repos=(
    llm-agent
    llm-agent-rag
    llm-agent-otel
    llm-agent-providers
    llm-agent-customer-support
    llm-agent-flow
    llm-agent-memory
    llm-agent-memory-gateway
    llm-agent-memory-postgres
  )
  ```
  Change it to (add `llm-agent-memory-contract` before `llm-agent-memory`, and `llm-agent-memory-worker` at the end):
  ```bash
  all_repos=(
    llm-agent
    llm-agent-rag
    llm-agent-otel
    llm-agent-providers
    llm-agent-customer-support
    llm-agent-flow
    llm-agent-memory-contract
    llm-agent-memory
    llm-agent-memory-gateway
    llm-agent-memory-postgres
    llm-agent-memory-worker
  )
  ```
- [ ] Add `repo_url` cases for the two new modules. In the `repo_url()` `case "$1" in` block (lines 24-35), after the `llm-agent-memory-postgres) ...` line, add:
  ```bash
      llm-agent-memory-contract) printf '%s\n' 'https://github.com/costa92/llm-agent-memory-contract.git' ;;
      llm-agent-memory-worker) printf '%s\n' 'https://github.com/costa92/llm-agent-memory-worker.git' ;;
  ```
  (`is_launchable` needs no change — neither module is launchable.)
- [ ] Verify the helper still runs and now iterates the new modules. NOTE: `eco.sh build` runs over ALL 11 repos; if some sibling repos are not checked out locally `require_repo` will exit. To exercise only the new modules, run a targeted build:
  ```
  bash scripts/eco.sh build llm-agent-memory-contract,llm-agent-memory-worker
  ```
  Expected: `go build ./...` runs in both dirs with no error.
- [ ] Commit:
  ```
  git add scripts/eco.sh
  git commit -m "chore(eco): include memory-contract + memory-worker in eco.sh repos"
  ```
- [ ] Tag the contract module. IMPORTANT: each sibling is its OWN git repo, so the first published tag is plain semver `v0.1.0` IN THAT REPO once the directory is split out into `github.com/costa92/llm-agent-memory-contract`. While the code still lives inside this umbrella working tree (pre-split), record the intended tag in the umbrella as `llm-agent-memory-contract/v0.1.0` (the umbrella's per-module tag convention; the only existing umbrella tag today is `v0.1.0`):
  ```
  git tag llm-agent-memory-contract/v0.1.0
  git tag --list 'llm-agent-memory-contract/*'
  ```
  Expected: `llm-agent-memory-contract/v0.1.0`. (Do NOT push until the wave is reviewed; pushing is a separate, user-approved step. When the module is split into its own repo, the published module version is `v0.1.0` there.)

---

## Phase 2 — Repoint `llm-agent-memory-postgres` → contract, drop llm-agent

### Task 5: Swap go.mod from llm-agent-memory to llm-agent-memory-contract

**Files:**
- Modify: `llm-agent-memory-postgres/go.mod`

Steps:

- [ ] Edit `llm-agent-memory-postgres/go.mod`. Replace the require line (line 6):
  ```
  	github.com/costa92/llm-agent-memory v0.0.0
  ```
  with (KEEP the `v0.0.0` placeholder convention used throughout this umbrella — the actual version is supplied by the `replace` below, and the umbrella never resolves a published version):
  ```
  	github.com/costa92/llm-agent-memory-contract v0.0.0
  ```
- [ ] Replace the replace directive (line 29):
  ```
  replace github.com/costa92/llm-agent-memory => ../llm-agent-memory
  ```
  with:
  ```
  replace github.com/costa92/llm-agent-memory-contract => ../llm-agent-memory-contract
  ```
  Leave the `github.com/google/uuid v1.6.0`, `github.com/jackc/pgx/v5 v5.9.2` requires and the existing `// indirect` block untouched. NOTE: after `go mod tidy` (Task 6), the `github.com/costa92/llm-agent v0.7.0 // indirect` line in the indirect block will be REMOVED automatically because nothing pulls it anymore — that is the expected "drop llm-agent" effect.
- [ ] Do NOT run build yet (imports still point at the old path). Proceed to Task 6.

### Task 6: Repoint all postgres import sites

**Files:**
- Modify: `llm-agent-memory-postgres/postgres/store.go` (import line 9)
- Modify: `llm-agent-memory-postgres/postgres/relay.go` (import line 9)
- Modify: `llm-agent-memory-postgres/postgres/fanout_publisher.go` (import line 6)
- Modify: `llm-agent-memory-postgres/postgres/lease_aware_publisher.go` (import line 9)
- Modify (test): `llm-agent-memory-postgres/postgres/store_test.go`
- Modify (test): `llm-agent-memory-postgres/postgres/mutation_test.go`
- Modify (test): `llm-agent-memory-postgres/postgres/write_test.go`
- Modify (test): `llm-agent-memory-postgres/postgres/schema_test.go`
- Modify (test): `llm-agent-memory-postgres/postgres/idempotency_test.go`
- Modify (test): `llm-agent-memory-postgres/postgres/relay_test.go`
- Modify (test): `llm-agent-memory-postgres/postgres/relay_failed_test.go`
- Modify (test): `llm-agent-memory-postgres/postgres/lease_aware_publisher_test.go`

The alias `corememory` is KEPT so no symbol references change — only the import path string changes. Before/after for every one of these files:
```
-	corememory "github.com/costa92/llm-agent-memory/memory"
+	corememory "github.com/costa92/llm-agent-memory-contract/contract"
```

Steps:

- [ ] Apply the import-path swap mechanically across the module. Run:
  ```
  cd llm-agent-memory-postgres && grep -rl 'llm-agent-memory/memory' --include='*.go' . | xargs sed -i 's#corememory "github.com/costa92/llm-agent-memory/memory"#corememory "github.com/costa92/llm-agent-memory-contract/contract"#g'
  ```
- [ ] Confirm no stale references remain:
  ```
  cd llm-agent-memory-postgres && grep -rn 'llm-agent-memory/memory' --include='*.go' . || echo "NONE-REMAIN"
  ```
  Expected: `NONE-REMAIN`.
- [ ] Build + test the module OUTSIDE the workspace against the tagged contract (this is the skew-honest check, not the `go.work` resolution):
  ```
  cd llm-agent-memory-postgres && GOWORK=off go build ./... && GOWORK=off go test ./...
  ```
  Expected: build clean; `ok  	github.com/costa92/llm-agent-memory-postgres/postgres`. The 7 `var _ corememory.RecordStore/Promoter/Deduper/AccessMarker/EventStore/IdempotencyStore/Outbox = (*Store)(nil)` assertions in `store.go` and the `var _ corememory.MessagePublisher = (*LeaseAwarePublisher)(nil)` assertions in `relay.go`/`lease_aware_publisher.go`/`fanout_publisher.go` now resolve against the contract module.
- [ ] Verify go.mod is tidy and llm-agent is fully gone from the graph:
  ```
  cd llm-agent-memory-postgres && GOWORK=off go mod tidy && GOWORK=off go list -m all | grep -E 'llm-agent($|/| )' && echo "UNEXPECTED-llm-agent" || echo "llm-agent-DROPPED"
  ```
  Expected: `llm-agent-DROPPED` (postgres no longer depends on `llm-agent` or `llm-agent-memory`).
- [ ] Run the persisted-schema guard test specifically to confirm the move did not change wire behavior:
  ```
  cd llm-agent-memory-postgres && GOWORK=off go test ./postgres/ -run TestPostgresJSON_MemoryRecordRoundTrip -v
  ```
  Expected: `--- PASS: TestPostgresJSON_MemoryRecordRoundTrip`.
- [ ] Commit:
  ```
  git add llm-agent-memory-postgres/go.mod llm-agent-memory-postgres/postgres/
  git commit -m "refactor(memory-postgres): repoint durable contract to llm-agent-memory-contract; drop llm-agent"
  ```
- [ ] Tag (umbrella per-module convention; this is the FIRST per-module tag for postgres — no prior `llm-agent-memory-postgres/*` tag exists, so use `v0.1.0`):
  ```
  git tag llm-agent-memory-postgres/v0.1.0
  ```
  (The postgres public Go API is unchanged for its consumers — only its own dependency changed — but since there is no prior published per-module tag, `v0.1.0` is the correct first tag.)

---

## Phase 3 — Repoint `llm-agent-memory-worker` → contract, drop llm-agent

### Task 7: Swap worker go.mod and repoint imports

**Files:**
- Modify: `llm-agent-memory-worker/go.mod`
- Modify: `llm-agent-memory-worker/internal/service/consolidation_publisher.go` (import line 8)
- Modify (test): `llm-agent-memory-worker/internal/service/*_test.go` (2 files)

Steps:

- [ ] Edit `llm-agent-memory-worker/go.mod`. Replace the require block (lines 6-7), keeping the `v0.0.0` placeholder convention:
  ```
  	github.com/costa92/llm-agent-memory v0.0.0
  	github.com/costa92/llm-agent-memory-postgres v0.0.0
  ```
  with:
  ```
  	github.com/costa92/llm-agent-memory-contract v0.0.0
  	github.com/costa92/llm-agent-memory-postgres v0.0.0
  ```
- [ ] Replace the two replace directives (lines 11-13):
  ```
  replace github.com/costa92/llm-agent-memory => ../llm-agent-memory

  replace github.com/costa92/llm-agent-memory-postgres => ../llm-agent-memory-postgres
  ```
  with (drop the `llm-agent-memory` replace, add the contract replace, keep the postgres replace):
  ```
  replace github.com/costa92/llm-agent-memory-contract => ../llm-agent-memory-contract

  replace github.com/costa92/llm-agent-memory-postgres => ../llm-agent-memory-postgres
  ```
- [ ] Repoint imports mechanically:
  ```
  cd llm-agent-memory-worker && grep -rl 'llm-agent-memory/memory' --include='*.go' . | xargs sed -i 's#corememory "github.com/costa92/llm-agent-memory/memory"#corememory "github.com/costa92/llm-agent-memory-contract/contract"#g'
  ```
  This touches `internal/service/consolidation_publisher.go` (which also imports `postgres "github.com/costa92/llm-agent-memory-postgres/postgres"` — leave that line as-is) plus its 2 test files.
- [ ] Confirm no stale references:
  ```
  cd llm-agent-memory-worker && grep -rn 'llm-agent-memory/memory' --include='*.go' . || echo "NONE-REMAIN"
  ```
  Expected: `NONE-REMAIN`.
- [ ] Build + test outside the workspace against the tagged deps:
  ```
  cd llm-agent-memory-worker && GOWORK=off go build ./... && GOWORK=off go test ./...
  ```
  Expected: build clean; all worker service tests `ok`.
- [ ] Verify llm-agent is dropped:
  ```
  cd llm-agent-memory-worker && GOWORK=off go mod tidy && GOWORK=off go list -m all | grep -E 'costa92/llm-agent($|/memory$| )' && echo "UNEXPECTED" || echo "llm-agent+memory-DROPPED"
  ```
  Expected: `llm-agent+memory-DROPPED` (worker now depends only on contract + postgres).
- [ ] Commit:
  ```
  git add llm-agent-memory-worker/go.mod llm-agent-memory-worker/internal/service/
  git commit -m "refactor(memory-worker): repoint durable contract; drop llm-agent + llm-agent-memory"
  ```
- [ ] Tag:
  ```
  git tag llm-agent-memory-worker/v0.1.0
  ```

---

## Phase 4 — Repoint `llm-agent-memory-gateway` → contract (keep llm-agent via rag)

### Task 8: Swap gateway go.mod (keep llm-agent-rag → llm-agent)

**Files:**
- Modify: `llm-agent-memory-gateway/go.mod`

The gateway will NOT shed `llm-agent`: it requires `llm-agent-rag v1.0.5`, which transitively pulls `llm-agent v0.7.0 // indirect`. We only drop the DIRECT `llm-agent-memory` dependency (gateway stops importing `llm-agent-memory/memory` after the repoint) and add the contract.

Steps:

- [ ] Edit `llm-agent-memory-gateway/go.mod`. In the first `require (` block (lines 5-10), replace:
  ```
  	github.com/costa92/llm-agent-memory v0.0.0
  	github.com/costa92/llm-agent-memory-postgres v0.0.0
  	github.com/costa92/llm-agent-rag v1.0.5
  ```
  with (keep `v0.0.0` placeholder convention; keep rag pinned at its real `v1.0.5`):
  ```
  	github.com/costa92/llm-agent-memory-contract v0.0.0
  	github.com/costa92/llm-agent-memory-postgres v0.0.0
  	github.com/costa92/llm-agent-rag v1.0.5
  ```
- [ ] Replace the replace directive (line 33):
  ```
  replace github.com/costa92/llm-agent-memory => ../llm-agent-memory
  ```
  with:
  ```
  replace github.com/costa92/llm-agent-memory-contract => ../llm-agent-memory-contract
  ```
  Keep `replace github.com/costa92/llm-agent-memory-postgres => ../llm-agent-memory-postgres` (line 35) and `replace github.com/costa92/llm-agent-rag => ../llm-agent-rag` (line 37) exactly as they are. The `github.com/costa92/llm-agent v0.7.0 // indirect` entry (line 13) STAYS — it is pulled transitively via rag (NOT v1.5.2; the real pinned indirect version in this repo is v0.7.0).

### Task 9: Repoint all gateway import sites

**Files:**
- Modify (prod): `llm-agent-memory-gateway/internal/service/service.go`
- Modify (prod): `llm-agent-memory-gateway/internal/service/store_adapter.go`
- Modify (prod): `llm-agent-memory-gateway/internal/service/idempotency_adapter.go`
- Modify (prod): `llm-agent-memory-gateway/internal/service/recall_cache.go`
- Modify (prod): `llm-agent-memory-gateway/internal/service/recall_observer.go`
- Modify (prod): `llm-agent-memory-gateway/internal/service/consolidation_observer.go`
- Modify (prod): `llm-agent-memory-gateway/internal/service/promotion_observer.go`
- Modify (prod): `llm-agent-memory-gateway/internal/service/dedupe_observer.go`
- Modify (prod): `llm-agent-memory-gateway/internal/service/dedupe_collapse_observer.go`
- Modify (prod): `llm-agent-memory-gateway/internal/service/outbox_vector_publisher.go`
- Modify (test): all `llm-agent-memory-gateway/internal/service/*_test.go` that import the contract (~11 files)

All use alias `corememory`; only the path changes.

Steps:

- [ ] Repoint mechanically across the gateway module:
  ```
  cd llm-agent-memory-gateway && grep -rl 'llm-agent-memory/memory' --include='*.go' . | xargs sed -i 's#corememory "github.com/costa92/llm-agent-memory/memory"#corememory "github.com/costa92/llm-agent-memory-contract/contract"#g'
  ```
- [ ] Confirm no stale references:
  ```
  cd llm-agent-memory-gateway && grep -rn 'llm-agent-memory/memory' --include='*.go' . || echo "NONE-REMAIN"
  ```
  Expected: `NONE-REMAIN`.
- [ ] Build + test outside the workspace:
  ```
  cd llm-agent-memory-gateway && GOWORK=off go build ./... && GOWORK=off go test ./...
  ```
  Expected: build clean; all gateway tests `ok`.
- [ ] Confirm `llm-agent` is STILL present (via rag) — this is expected, not a regression:
  ```
  cd llm-agent-memory-gateway && GOWORK=off go mod tidy && GOWORK=off go list -m all | grep 'costa92/llm-agent ' && echo "llm-agent-KEPT-via-rag (expected)"
  ```
  Expected: `github.com/costa92/llm-agent v0.7.0` printed, then `llm-agent-KEPT-via-rag (expected)`. Also confirm the direct `llm-agent-memory` is gone:
  ```
  cd llm-agent-memory-gateway && GOWORK=off go list -m all | grep 'costa92/llm-agent-memory ' && echo "UNEXPECTED-direct-memory" || echo "direct-llm-agent-memory-DROPPED"
  ```
  Expected: `direct-llm-agent-memory-DROPPED`.
- [ ] Commit:
  ```
  git add llm-agent-memory-gateway/go.mod llm-agent-memory-gateway/internal/service/
  git commit -m "refactor(memory-gateway): repoint durable contract; drop direct llm-agent-memory (llm-agent stays via rag)"
  ```
- [ ] Tag (first per-module tag for gateway — no prior `llm-agent-memory-gateway/*` tag exists):
  ```
  git tag llm-agent-memory-gateway/v0.1.0
  ```

---

## Phase 5 — Slim `llm-agent-memory` with an alias shim

### Task 10: Replace durable.go body with `=`-alias re-exports of contract

**Files:**
- Modify: `llm-agent-memory/go.mod`
- Delete: `llm-agent-memory/memory/durable.go`
- Create: `llm-agent-memory/memory/durable_shim.go`
- Delete: `llm-agent-memory/memory/durable_test.go` (moved to contract in Phase 1; the shim is covered by contract's tests + a thin shim identity test)

The shim keeps `llm-agent-memory/memory` importers compiling for ONE release cycle. CAVEAT (encode in the shim doc): `type X = contract.X` is a true alias and works ONLY when the whole module graph resolves to ONE contract version. A mixed-version graph (`contract@v0.1.0` in one dep, `@v0.2.0` in another) will still break `var _` assertions and named-type slices/maps. All consumers in this wave resolve to the SAME single contract version (via the umbrella's `v0.0.0` placeholder + local `replace`, and `llm-agent-memory-contract/v0.1.0` once published); the shim is a transition aid, not a mixed-version fix.

Steps:

- [ ] Add the contract dependency to `llm-agent-memory/go.mod` (keep the `v0.0.0` placeholder convention used across the umbrella). Add to the require block:
  ```
  	github.com/costa92/llm-agent-memory-contract v0.0.0
  ```
  and add the replace directive:
  ```
  replace github.com/costa92/llm-agent-memory-contract => ../llm-agent-memory-contract
  ```
- [ ] Delete the moved files:
  ```
  rm llm-agent-memory/memory/durable.go llm-agent-memory/memory/durable_test.go
  ```
- [ ] Create the shim `llm-agent-memory/memory/durable_shim.go` re-exporting EVERY exported durable symbol as a true `=` alias / value re-export. Use the exact symbol set from the original durable.go:
  ```go
  package memory

  import contract "github.com/costa92/llm-agent-memory-contract/contract"

  // Durable contract alias shim. The durable model now lives in
  // github.com/costa92/llm-agent-memory-contract/contract. These are true `=`
  // type aliases so existing importers of llm-agent-memory/memory keep
  // compiling for one release cycle.
  //
  // CAVEAT: aliases only unify types when the whole module graph resolves to a
  // SINGLE llm-agent-memory-contract version. A mixed-version graph still
  // breaks var-_ interface assertions and named-type slices/maps. Pin one
  // contract version across all modules during the migration wave.

  // Types.
  type (
  	MemoryRecord        = contract.MemoryRecord
  	StoredEvent         = contract.StoredEvent
  	OutboxMessage       = contract.OutboxMessage
  	IdempotencyEntry    = contract.IdempotencyEntry
  	WriteRecordInput    = contract.WriteRecordInput
  	WriteRecordResult   = contract.WriteRecordResult
  	PatchRecordInput    = contract.PatchRecordInput
  	PatchRecordResult   = contract.PatchRecordResult
  	DeleteRecordInput   = contract.DeleteRecordInput
  	DeleteRecordResult  = contract.DeleteRecordResult
  	PinRecordInput      = contract.PinRecordInput
  	PinRecordResult     = contract.PinRecordResult
  	DisableRecordInput  = contract.DisableRecordInput
  	DisableRecordResult = contract.DisableRecordResult
  	PromoteRecordInput  = contract.PromoteRecordInput
  	PromoteRecordResult = contract.PromoteRecordResult
  	DedupeAction        = contract.DedupeAction
  	ResolveDedupeInput  = contract.ResolveDedupeInput
  	ResolveDedupeResult = contract.ResolveDedupeResult
  	MarkAccessInput     = contract.MarkAccessInput
  	RecordStore         = contract.RecordStore
  	Promoter            = contract.Promoter
  	Deduper             = contract.Deduper
  	AccessMarker        = contract.AccessMarker
  	EventStore          = contract.EventStore
  	IdempotencyStore    = contract.IdempotencyStore
  	Outbox              = contract.Outbox
  	MessagePublisher    = contract.MessagePublisher
  )

  // Constants.
  const (
  	RecordKindWorking                 = contract.RecordKindWorking
  	RecordKindEpisodic                = contract.RecordKindEpisodic
  	RecordKindSemantic                = contract.RecordKindSemantic
  	DedupeCollapsedLoserIDMetadataKey = contract.DedupeCollapsedLoserIDMetadataKey
  	DedupeNoCollision                 = contract.DedupeNoCollision
  	DedupeMergedExisting              = contract.DedupeMergedExisting
  	DedupeCollapsedByPin              = contract.DedupeCollapsedByPin
  )

  // Errors.
  var ErrInvalidRecordKind = contract.ErrInvalidRecordKind

  // Functions.
  var (
  	NormalizeRecordKind = contract.NormalizeRecordKind
  )
  ```
  Note: `MemoryRecord.NormalizeWriteDefaults` and `MemoryRecord.SetWorkingDefault` are METHODS on the aliased type, so they are automatically available through the `MemoryRecord = contract.MemoryRecord` alias — no extra re-export needed.
- [ ] Add a thin shim identity test `llm-agent-memory/memory/durable_shim_test.go` proving the alias is a true `=` alias (assignable without conversion) and the methods resolve:
  ```go
  package memory

  import (
  	"testing"

  	contract "github.com/costa92/llm-agent-memory-contract/contract"
  )

  func TestDurableShimAliasIdentity(t *testing.T) {
  	// True `=` alias: a contract value is assignable to the local name with no conversion.
  	var local MemoryRecord = contract.MemoryRecord{Kind: RecordKindWorking}
  	var back contract.MemoryRecord = local
  	if back.Kind != contract.RecordKindWorking {
  		t.Fatalf("alias identity broken: %q", back.Kind)
  	}
  	// Method promoted through the alias.
  	if _, err := local.NormalizeWriteDefaults(); err != nil {
  		t.Fatalf("NormalizeWriteDefaults via alias: %v", err)
  	}
  	// Re-exported function.
  	if _, err := NormalizeRecordKind("working"); err != nil {
  		t.Fatalf("NormalizeRecordKind via shim: %v", err)
  	}
  }
  ```
- [ ] Run the shim test; it MUST PASS:
  ```
  cd llm-agent-memory && GOWORK=off go test ./memory/ -run TestDurableShimAliasIdentity -v
  ```
  Expected: `--- PASS: TestDurableShimAliasIdentity`.
- [ ] Build + test the whole module (the engines, `types_alias.go`, `core_adapters.go`, etc. still compile — they referenced `MemoryRecord` etc. by local name, now satisfied by the shim):
  ```
  cd llm-agent-memory && GOWORK=off go build ./... && GOWORK=off go test ./...
  ```
  Expected: build clean; all existing `llm-agent-memory` tests `ok`.
- [ ] Verify go.mod tidy:
  ```
  cd llm-agent-memory && GOWORK=off go mod tidy && GOWORK=off go list -m all | grep 'llm-agent-memory-contract' && echo "contract-required"
  ```
  Expected: a line like `github.com/costa92/llm-agent-memory-contract v0.0.0 => ../llm-agent-memory-contract` (the `v0.0.0` placeholder resolved via the local `replace`), then `contract-required`.
- [ ] Commit:
  ```
  git add llm-agent-memory/go.mod llm-agent-memory/memory/durable_shim.go llm-agent-memory/memory/durable_shim_test.go
  git rm llm-agent-memory/memory/durable.go llm-agent-memory/memory/durable_test.go
  git commit -m "refactor(memory): replace durable.go with contract alias shim (one-cycle transition)"
  ```
- [ ] Tag. There is no prior `llm-agent-memory/*` per-module tag, so this is the first one. DECISION (record in the tag message): the shim keeps `llm-agent-memory/memory` source-compatible for single-version graphs, but the durable types now live in another module and mixed-version graphs can break — so the relocation is technically breaking. Because there is no published baseline to break, tag the first per-module release as `v0.1.0` and document the shim's one-cycle removal window in the message:
  ```
  git tag -a llm-agent-memory/v0.1.0 -m "durable model moved to llm-agent-memory-contract; alias shim retained for one release cycle, removal planned next minor/major"
  ```
  (Once `llm-agent-memory` has published baselines, the shim REMOVAL in a future cycle must be a major bump. Encode that follow-up in the next-plan note.)

---

## Phase 6 — Release-skew CI + eco.sh release-check

### Task 11: Extend the umbrella workflow with worker, contract, and a release-skew job

**Files:**
- Modify: `.github/workflows/umbrella.yml`

The real workflow is a SINGLE `cross-repo-build` job that checks out each sibling repo into a path (`actions/checkout` with `repository: costa92/<module>` + `path: <module>`) and then runs `GOWORK=off go vet/build/test` per `working-directory`. There is NO per-module job. It also keeps each module's `replace` directives (siblings resolve from the checked-out dirs), so it never catches tagged-graph skew. We (a) add checkout + build steps for the two missing modules inside `cross-repo-build`, and (b) add a NEW `release-skew` top-level job that re-checks out the durable modules and drops `replace` directives so resolution falls back to the required published versions.

Steps:

- [ ] Add a checkout step for the contract inside `cross-repo-build`, after the existing `Checkout llm-agent-memory-postgres` step:
  ```yaml
      - name: Checkout llm-agent-memory-contract
        uses: actions/checkout@v4
        with:
          repository: costa92/llm-agent-memory-contract
          ref: main
          path: llm-agent-memory-contract

      - name: Checkout llm-agent-memory-worker
        uses: actions/checkout@v4
        with:
          repository: costa92/llm-agent-memory-worker
          ref: main
          path: llm-agent-memory-worker
  ```
- [ ] Add the matching build steps. The contract has no deps so it should build FIRST (before the modules that require it); insert its build step BEFORE `Build llm-agent-memory` and add the worker build step AFTER `Build llm-agent-memory-postgres`:
  ```yaml
      - name: Build llm-agent-memory-contract
        working-directory: llm-agent-memory-contract
        run: |
          GOWORK=off go vet ./...
          GOWORK=off go build ./...
          GOWORK=off go test ./... -count=1
  ```
  and after `Build llm-agent-memory-postgres`:
  ```yaml
      - name: Build llm-agent-memory-worker
        working-directory: llm-agent-memory-worker
        run: |
          GOWORK=off go vet ./...
          GOWORK=off go build ./...
          GOWORK=off go test ./... -count=1
  ```
- [ ] Add a NEW top-level `release-skew` job (sibling of `cross-repo-build`, `smoke`, `stdlib-only-gate`). It checks out the four durable modules + the contract, DROPS the local `replace` for the contract in each downstream, and verifies each still builds resolving the contract by its required version from the module cache (with all five checked out as path replaces only for the OTHER siblings). This is the job that local `go.work`+`replace` masks:
  ```yaml
  release-skew:
    name: B5 — durable contract release-skew gate
    needs: cross-repo-build
    runs-on: ubuntu-latest
    timeout-minutes: 20
    steps:
      - name: Checkout umbrella root
        uses: actions/checkout@v4
      - name: Checkout llm-agent-memory-contract
        uses: actions/checkout@v4
        with: { repository: costa92/llm-agent-memory-contract, ref: main, path: llm-agent-memory-contract }
      - name: Checkout llm-agent-memory
        uses: actions/checkout@v4
        with: { repository: costa92/llm-agent-memory, ref: main, path: llm-agent-memory }
      - name: Checkout llm-agent-memory-postgres
        uses: actions/checkout@v4
        with: { repository: costa92/llm-agent-memory-postgres, ref: main, path: llm-agent-memory-postgres }
      - name: Checkout llm-agent-memory-worker
        uses: actions/checkout@v4
        with: { repository: costa92/llm-agent-memory-worker, ref: main, path: llm-agent-memory-worker }
      - name: Checkout llm-agent-memory-gateway
        uses: actions/checkout@v4
        with: { repository: costa92/llm-agent-memory-gateway, ref: main, path: llm-agent-memory-gateway }
      - name: Checkout llm-agent-rag
        uses: actions/checkout@v4
        with: { repository: costa92/llm-agent-rag, ref: master, path: llm-agent-rag }
      - uses: actions/setup-go@v5
        with: { go-version: '1.26.0', cache: false }
      - name: build+test each durable module GOWORK=off (no go.work masking)
        run: |
          set -euo pipefail
          for m in llm-agent-memory-contract llm-agent-memory llm-agent-memory-postgres llm-agent-memory-worker llm-agent-memory-gateway; do
            echo "== release-skew $m =="
            (cd "$m" && GOWORK=off go build ./... && GOWORK=off go test ./... -count=1)
          done
      - name: negative guard — postgres + worker must NOT depend on llm-agent
        run: |
          set -euo pipefail
          for m in llm-agent-memory-postgres llm-agent-memory-worker; do
            if (cd "$m" && GOWORK=off go list -m all | grep -qE 'costa92/llm-agent($| )'); then
              echo "ERROR: $m still depends on llm-agent"; exit 1
            fi
          done
          echo "postgres + worker are llm-agent-free"
      - name: positive guard — gateway keeps llm-agent via rag
        run: |
          set -euo pipefail
          if (cd llm-agent-memory-gateway && GOWORK=off go list -m all | grep -qE 'costa92/llm-agent '); then
            echo "gateway keeps llm-agent via rag (expected)"
          else
            echo "ERROR: gateway no longer pulls llm-agent — rag dep changed unexpectedly"; exit 1
          fi
  ```
- [ ] Validate the YAML locally (syntax + job graph):
  ```
  python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/umbrella.yml')); print('yaml-ok')"
  ```
  Expected: `yaml-ok`.
- [ ] Commit:
  ```
  git add .github/workflows/umbrella.yml
  git commit -m "ci(umbrella): add memory-contract + memory-worker jobs and release-skew gate"
  ```

### Task 12: Add `release-check` and `tag` targets to eco.sh

**Files:**
- Modify: `scripts/eco.sh`

Steps:

- [ ] Add a `release-check` case and a `tag` case to the `case "$cmd" in` block in `scripts/eco.sh`, before the `help|*)` arm. Insert:
  ```bash
  release-check)
    # Skew-honest gate for the durable-memory release wave: build+test each
    # wave module with GOWORK=off and verify go.mod is tidy. Mirrors the CI
    # release-skew job for local pre-tag checks. Scoped to the wave modules
    # (not all_repos) so it does not require every sibling repo checked out.
    # NOTE: eco.sh's script-level root variable is `root_dir` (set at line 4).
    durable_wave=(
      llm-agent-memory-contract
      llm-agent-memory
      llm-agent-memory-postgres
      llm-agent-memory-worker
      llm-agent-memory-gateway
    )
    for r in "${durable_wave[@]}"; do
      require_repo "$r"
      echo "== release-check $r =="
      (
        cd "$root_dir/$r"
        GOWORK=off go build ./...
        GOWORK=off go test ./... -count=1
        cp go.mod /tmp/eco-gomod-pre
        GOWORK=off go mod tidy
        if ! diff -q /tmp/eco-gomod-pre go.mod >/dev/null; then
          echo "  ERROR: $r go.mod not tidy (run 'go mod tidy')"
          exit 1
        fi
      )
    done
    # Negative guard: postgres + worker must not depend on llm-agent.
    for r in "llm-agent-memory-postgres" "llm-agent-memory-worker"; do
      if (cd "$root_dir/$r" && GOWORK=off go list -m all | grep -qE 'costa92/llm-agent($| )'); then
        echo "ERROR: $r still depends on llm-agent"
        exit 1
      fi
    done
    echo "release-check OK"
    ;;
  tag)
    # Usage: scripts/eco.sh tag <module> <vX.Y.Z>
    module="${2:?module required, e.g. llm-agent-memory-contract}"
    version="${3:?version required, e.g. v0.1.0}"
    git tag "${module}/${version}"
    echo "tagged ${module}/${version}"
    ;;
  ```
- [ ] Update the `help` arm to list the new commands. The real `help|--help|-h)` arm (lines 187-198 of eco.sh) is a `cat <<'EOF' ... EOF` heredoc — there are NO `echo` statements. Add two lines INSIDE the heredoc, after the existing `  test [all|repo1,repo2]` line and before the `  up [all|...]` line:
  ```
    release-check          GOWORK=off build+test+tidy-check the durable-memory wave modules (skew gate)
    tag <module> <vX.Y.Z>  create a per-module git tag (e.g. tag llm-agent-memory-contract v0.1.0)
  ```
- [ ] Run the new gate end-to-end:
  ```
  bash scripts/eco.sh release-check
  ```
  Expected: `== release-check ... ==` for the 5 durable-wave modules (contract, memory, postgres, worker, gateway), then `release-check OK`.
- [ ] Verify the tag helper works (dry-run the help text):
  ```
  bash scripts/eco.sh help
  ```
  Expected: help text including `release-check` and `tag <module> <vX.Y.Z>`.
- [ ] Commit:
  ```
  git add scripts/eco.sh
  git commit -m "chore(eco): add release-check skew gate + per-module tag helper"
  ```

---

## Self-review pass

Run these BEFORE considering the wave done; fix any failure inline.

- [ ] **Spec coverage vs design §5.** Every §5 review-corrected decision is encoded:
  - §5.3 "durable.go is persisted JSON schema, highest-stability API" → Task 2 golden wire test + Task 3 doc.go/README/CODEOWNERS policy. ✓
  - §5.3 "gateway keeps llm-agent via rag" → Task 8/9 keep `llm-agent-rag` + `llm-agent` indirect; Task 9 verifies `llm-agent-KEPT-via-rag`. ✓
  - §5.3 "alias-shim is single-version-only" → Task 10 shim doc caveat + every wave module resolving to a SINGLE contract version (umbrella `v0.0.0` placeholder + local `replace`; published `v0.1.0`). ✓
  - §5.3 "go.work + replace mask version skew" → Task 11 `release-skew` job + Task 12 `release-check`. ✓
  - §5.4 wave order (contract → postgres → worker → gateway → memory slim) → Phases 1–5 in that exact order. ✓
  - §7 "bundle release-matrix CI" ruling → Phase 6. ✓
- [ ] **Out-of-scope items present.** §8 idempotency gap and Proposal 1 are listed in "Out of scope / follow-on plans" and NOT implemented here. ✓
- [ ] **Placeholder scan.** Run:
  ```
  grep -nE 'TBD|FIXME|similar to|write tests for the above|add error handling|\.\.\.' docs/superpowers/plans/2026-05-29-llm-agent-memory-contract-extraction.md
  ```
  Expected: no `TBD`/`FIXME`/"similar to"/"write tests for the above" matches. The `__CAPTURE_ME__` golden placeholders are intentional (not matched by this pattern) and carry an explicit runtime-capture instruction; literal `...` inside authored Go/YAML/`go build ./...` is expected. Fix any true planning placeholder inline.
- [ ] **Type/symbol consistency.** The shim (Task 10) re-exports exactly the symbols defined in `durable.go`: 4 aggregates (`MemoryRecord`, `StoredEvent`, `OutboxMessage`, `IdempotencyEntry`) + 16 `*Input`/`*Result` DTOs + `DedupeAction` + `ResolveDedupeInput`/`ResolveDedupeResult` + `MarkAccessInput` + 8 interfaces + 7 constants (`RecordKindWorking/Episodic/Semantic`, `DedupeCollapsedLoserIDMetadataKey`, `DedupeNoCollision/MergedExisting/CollapsedByPin`) + `ErrInvalidRecordKind` + `NormalizeRecordKind`; the methods `NormalizeWriteDefaults`/`SetWorkingDefault` ride the `MemoryRecord` type alias. Cross-check the shim against the moved file to confirm no exported symbol is missing:
  ```
  comm -23 \
    <(grep -oE '\b[A-Z][A-Za-z0-9_]+\b' llm-agent-memory-contract/contract/durable.go | sort -u) \
    <(grep -oE 'contract\.[A-Z][A-Za-z0-9_]+' llm-agent-memory/memory/durable_shim.go | sed 's/contract\.//' | sort -u)
  ```
  Expected: among the symbols in durable.go not present in the shim, the only EXPORTED top-level identifiers are the two METHODS `NormalizeWriteDefaults` and `SetWorkingDefault` (promoted via the `MemoryRecord` alias). Ignore non-symbol matches (godoc words, local field names). If a real exported package-level symbol is missing from the shim, add the re-export.
- [ ] **Alias used consistently.** All consumer repoints keep the `corememory` import alias, so zero symbol-reference edits were needed — only import-path strings changed. Confirm with:
  ```
  grep -rlE 'corememory "github.com/costa92/llm-agent-memory-contract/contract"' llm-agent-memory-postgres llm-agent-memory-worker llm-agent-memory-gateway --include='*.go' | wc -l
  ```
  Expected: `36` — 14 postgres + 3 worker + 19 gateway files repointed across Phases 2-4.
