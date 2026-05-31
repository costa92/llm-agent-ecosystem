# refactor(memory): extract durable contract into `llm-agent-memory-contract` and repoint satellites

## Summary

Extracts the durable-memory contract (`MemoryRecord`, the persisted aggregates,
and the 8 storage-port interfaces) verbatim out of `llm-agent-memory/memory`
into a new **stdlib-only** module `github.com/costa92/llm-agent-memory-contract`,
then repoints the three satellite modules onto it. This severs the
`llm-agent` dependency from `postgres` and `worker`, keeps `llm-agent` in
`gateway` only via `llm-agent-rag` (as designed), and lets every runtime module
build against a single, frozen, persisted-schema contract.

Implements **Proposal 2** from `docs/memory-cluster-evolution-design.zh-CN.md`
(a two-round-reviewed design: Plan-agent + codex). Branch:
`feat/memory-contract-extraction` (12 commits).

Scope: **64 files, +4184 / −151.**

## What's in this PR (Phases 1–4 + 6)

| Phase | Module | Result |
|---|---|---|
| 1 | `llm-agent-memory-contract` (new, 17 files) | `durable.go` copied byte-for-byte (only package clause changed); golden JSON wire test pins the persisted Postgres shape; `doc.go` stability policy + README + CODEOWNERS; stdlib-only (0 requires). |
| 2 | `llm-agent-memory-postgres` (34 files) | Repointed to contract; **dropped `llm-agent`**. Implemented the missing `Store` methods the durable contracts require: `Promote`, `ResolveDedupe`, `MarkAccess`, `ListSessionWorking` + schema v3 (widen `kind` CHECK to admit `working`, add `memory_dedupe_index`). Fixed two pre-existing NULL-handling bugs in `store.go` (`project_id`/`session_id` scan, `tags` NOT NULL) surfaced by the first real DB run. |
| 2 (hardening) | `llm-agent-memory-postgres` | **C1**: `ResolveDedupe` collision path made fully transactional (loser-delete + collapse event/outbox now atomic, was 3 non-atomic ops). **C2**: `Promote` idempotency hash keyed on the full request (`MemoryID`+`SourceEventID`+`ExpectedVersion`), not just `MemoryID`. Both findings came from an adversarial code review of DB-gated code that CI cannot run. |
| 3 | `llm-agent-memory-worker` | Repointed to contract; **dropped `llm-agent`**. |
| 4 | `llm-agent-memory-gateway` (19 import sites) | Repointed to contract; **keeps `llm-agent` via rag** (not a regression — rag transitively requires it). |
| 6 | `.github/workflows/umbrella.yml`, `scripts/eco.sh` | CI builds/tests `memory-contract` + `memory-worker` (previously missing). `eco.sh` gains a `release-check` action (skew-honest `GOWORK=off build+vet` per repo). |

Also adds design + planning docs under `docs/`.

## Verification

All four repointed modules verified with **a real Postgres 16** (throwaway
Docker container) and with `GOWORK=off` (so the workspace cannot mask a
tagged-graph break):

| Module | build | vet | test | old `llm-agent-memory` dep | `llm-agent` dep |
|---|---|---|---|---|---|
| contract | ✅ | ✅ | ✅ (golden wire) | — | — |
| postgres | ✅ | ✅ | ✅ (full suite, live DB, 0 fail) | **0** | **0** |
| worker | ✅ | ✅ | ✅ (0 fail) | **0** | **0** |
| gateway | ✅ | ✅ | ✅ (live DB, 0 fail) | **0** | 1 (via rag) |

The postgres DB suite went 19 → 1 → 0 failures as the NULL-handling and schema-v3
migration bugs were fixed. The schema v3 migration uses a name-agnostic `DO`
block to drop the auto-named `kind` CHECK constraint (the constructed
`<table>_kind_check` literal did not match Postgres's truncated auto-name).

## NOT in this PR — Phase 5 (intentionally deferred)

Phase 5 (slim `llm-agent-memory` to a contract **alias shim**) is **not** here.
`llm-agent-memory` is a separate, gitignored repo currently holding a large
in-progress refactor (~48 uncommitted files; its `go vet` is red mid-refactor),
and `memory/durable.go` there is **untracked WIP with no git backup** — deleting
it to install the shim would destroy unsaved work. A ready-to-apply, one-step
handoff (full shim source + go.mod edits + verify/commit commands) is in
**`docs/memory-contract-phase5-handoff.md`**. Apply it after landing the
`llm-agent-memory` WIP on a clean, green baseline.

## Release / migration notes

- Modules use the umbrella's `v0.0.0` placeholder + local `replace` convention;
  per-module tags (`llm-agent-memory-contract/v0.1.0` etc.) are created locally
  but **not pushed** — pushing/tagging is a separate, coordinated wave.
- The contract module is now the **highest-stability, DB-backed API**: its types
  are serialized with default `encoding/json` (no tags) straight into Postgres,
  so wire keys == Go field names. Renaming a field / changing value↔pointer /
  changing a type / adding a json tag is a **DB migration**, not a code change
  (enforced by the golden wire test + documented in the module's `doc.go`).
- The Phase 5 alias shim only unifies types when the whole graph resolves to a
  **single** contract version; pin one version across modules during the wave.

## Commits

```
docs(memory): add Phase 5 handoff (ready-to-apply alias shim)
chore(eco): add release-check action (skew-honest build+vet per repo)
ci(umbrella): add memory-contract + memory-worker build/checkout steps
refactor(memory-gateway): repoint durable contract; keep llm-agent via rag
feat(memory-worker): repoint durable contract; drop llm-agent
fix(memory-postgres): make ResolveDedupe atomic (C1) and key Promote idempotency (C2)
feat(memory-postgres): repoint to contract + implement Promote/ResolveDedupe/MarkAccess/ListSessionWorking
chore(eco): include memory-contract + memory-worker in eco.sh repos
docs(memory): add cluster-evolution design + contract-extraction plan
test(memory-contract): pin persisted JSON wire shape (golden round-trip)
docs(memory-contract): add doc.go stability policy + README + CODEOWNERS
feat(memory-contract): scaffold module + move durable.go verbatim
```

## Reviewer notes

- `llm-agent` core is untouched (still stdlib-only, never imports the extension/contract).
- The user's untracked WIP test files in `postgres`/`gateway` are committed as part
  of their module repoint (they already imported the contract path); the
  `llm-agent-memory` repo's WIP is **not** touched by this PR.
- Two design docs to read for context: `docs/memory-cluster-evolution-design.zh-CN.md`
  (the reviewed design) and `docs/superpowers/plans/2026-05-29-llm-agent-memory-contract-extraction.md`
  (the execution plan).
