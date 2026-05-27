# `llm-agent-memory` Subproject Roadmap

> Doc version: 2026-05-25
> Owner: ecosystem root
> Status: Draft master roadmap. Each milestone gets a dedicated detailed plan under `docs/superpowers/plans/`.
> Source-of-truth design docs (all paths relative to umbrella root):
> - `docs/memory-roadmap.zh-CN.md`
> - `docs/multi-service-memory-architecture.zh-CN.md`
> - `docs/memory-gateway-api-contract.zh-CN.md`
> - `docs/memory-postgres-outbox-schema.zh-CN.md`

---

## 1. Goal & Scope Statement

### What this subproject IS

`llm-agent-memory/` is a new Go module hosted as a sibling under the `llm-agent-ecosystem` umbrella (joined via `go.work`). It is the long-term home of the three-tier memory system as it evolves from an in-process SDK into a **multi-service memory platform**: SDK + Memory Gateway HTTP service + Postgres truth source + Transactional Outbox + async workers (Embedding / Consolidation / Index Sync / Cache Invalidate).

It begins life as a *mirror+extension* of the current `llm-agent/memory/` package and incrementally adds capabilities the core stdlib-only module cannot host (Postgres driver, vector index clients, HTTP server, MQ relay). It owns Phases A→E from `docs/memory-roadmap.zh-CN.md` and the v1 minimum closed loop from `docs/multi-service-memory-architecture.zh-CN.md` §9.12.

### What this subproject IS NOT

- Not a replacement for `llm-agent` core. The core remains stdlib-only (enforced by `scripts/stdlib-only-check.sh`).
- Not a vector database. Vector indexes are pluggable backends; Postgres is the truth source (`docs/multi-service-memory-architecture.zh-CN.md` §8.2).
- Not a learning system. Salience/learned-rerank/decay learning are explicitly out of v1 (`docs/memory-roadmap.zh-CN.md` §11.3, `multi-service-memory-architecture.zh-CN.md` §9.11).
- Not a prompt builder. `context.Builder` lives in `llm-agent/context/` and remains the prompt assembler; this subproject only feeds it `MemoryHits` (`docs/multi-service-memory-architecture.zh-CN.md` §6.1).

### Boundary with `llm-agent/memory/`

| Phase | `llm-agent/memory/` (core) | `llm-agent-memory/` (new) |
|---|---|---|
| M0–M1 | Authoritative SDK. Untouched. | Imports core types via `github.com/costa92/llm-agent/memory`. Adds **scoped-lifecycle**, **consolidate dedupe**, **SearchUnified** as additive wrappers/extensions. |
| M2 | Untouched. | Adds observer hooks + parallel SearchAll; may upstream PRs to core if they remain stdlib-only. |
| M3 | Untouched. Core keeps only `FilesystemStore`. | Hosts Postgres / SQLite snapshot stores and the new write-policy interface. |
| M4 (v1 breaking) | **Frozen surface** — bug fixes only. | Introduces capability-interface `ManagerOptions` + `RecallEngine` facade. New consumers should adopt this. |
| M5–M7 | Untouched. | Owns Postgres schema (M5), HTTP Gateway (M6), persisted decision traces + measurable metric subset (M7 rescoped 2026-05-27). |
| M8 (v2 breaking) | **Deprecation announced.** Consumers should migrate to the v1+ subproject surfaces. | Owns scope-as-first-class-key + storage refactor + schema normalization **plus** the deferred M7 substrate: Working tier, atomic promotion API, dedupe primitive, typed `RecallObserver`, `LastAccessAt` write path, Consolidation Worker, remaining 12 validation counters, reason-enum freeze. |

**Deprecation point.** `llm-agent/memory/` continues to be maintained as the stdlib-only in-process SDK through M3. From M4 onwards, new development happens in `llm-agent-memory/`. Final deprecation of `llm-agent/memory/` is decided at M8 kick-off based on consumer migration progress; expect at least one release window where both exist.

### Boundary with other subprojects

- **`llm-agent-rag/`**: independent. RAG owns retrieval over user document corpora; memory owns user/agent-state recall. They may share embedding providers but never store types.
- **`llm-agent-otel/`**: optional consumer of M2 observer hooks. Memory does not depend on otel; otel can wrap memory.
- **`llm-agent-providers/`**: supplies `llm.Embedder` implementations. Memory takes the same `Embedder` interface used by core.
- **`llm-agent-customer-support/`**: reference consumer of the Memory Gateway client SDK from M6 onwards.

---

## 2. Milestone Breakdown

Milestones are linearly ordered; each builds only on its predecessors. Every milestone produces working, testable software (`go test ./...` green) and is independently shippable.

### M0 — Subproject Scaffolding

| Attribute | Value |
|---|---|
| Goal | Produces a new `llm-agent-memory/` Go module that builds, vets, and tests cleanly under `go.work` and umbrella CI. |
| Phase mapping | Pre-Phase-A infrastructure. |
| P0/P1/P2 | P0 (blocker for everything else). |
| Doc refs | `Makefile`; `.github/workflows/umbrella.yml`; sibling pattern from `llm-agent-rag/go.mod`. |
| Independently shippable | Yes — empty module that builds. |
| Exit criteria | 1. `llm-agent-memory/go.mod` with module path `github.com/costa92/llm-agent-memory`, Go 1.26.0. 2. Added to `go.work` `use` block. 3. Stub package `memory/` with `doc.go` and a no-op `version_test.go`. 4. Added to `.github/workflows/umbrella.yml` cross-repo-build matrix mirroring siblings (`GOWORK=off go vet/build/test ./... -count=1`). 5. `Makefile` `TARGETS` and `scripts/eco.sh` updated to recognize the new sibling. |
| Complexity | S |
| Detailed plan file | `docs/superpowers/plans/2026-05-26-m0-llm-agent-memory-scaffold.md` |

### M1 — Phase A: Scoped Lifecycle + Consolidate Dedupe + SearchUnified

| Attribute | Value |
|---|---|
| Goal | Produces a `memory/` package that mirrors the core SDK and adds the three Phase A correctness fixes as additive APIs. |
| Phase mapping | Phase A (`docs/memory-roadmap.zh-CN.md` §4.1, items A-1/A-2/A-3). |
| P0/P1/P2 | P0 (multi-tenant correctness + recall quality + episodic-growth control). |
| Doc refs | `memory-roadmap.zh-CN.md` §4.1, §9 items 1–3, §11.1; existing code `llm-agent/memory/scoped_manager.go:128-144`, `manager.go:178-208`, `manager.go:96-115`. |
| Independently shippable | Yes — drop-in SDK extension. |
| Exit criteria | 1. `ConsolidateScoped` / `ForgetScoped` / `StatsScoped` honor non-zero scope; cross-scope mutation tests pass. 2. `Consolidate` writes `_consolidated_at` / `_promoted_from` / `_promotion_count` metadata; same working item consolidated twice does not duplicate in episodic. 3. `SearchUnified(ctx, query, topK)` returns one merged, deduped, sorted `[]SearchResult`; respects `topK`; old `SearchAll` unchanged. 4. Backwards-compat tests demonstrate old-API behavior unchanged. 5. Snapshot import/export round-trips the new metadata keys. |
| Complexity | M |
| Detailed plan file | `docs/superpowers/plans/2026-06-XX-m1-phase-a-correctness.md` |

**Open dependency on core**: M1 must decide whether to *fork* the three impacted files or *wrap* them. Recommendation: wrap. Fork only if upstream patches are blocked.

### M2 — Phase B: Observer Hooks + Working Eviction Reuse + Parallel SearchAll

| Attribute | Value |
|---|---|
| Goal | Produces observability hooks + write-path latency wins; no behavior change when observer is nil. |
| Phase mapping | Phase B (`docs/memory-roadmap.zh-CN.md` §4.2, items B-1/B-2/B-3). |
| P0/P1/P2 | P0 (the 7-metric minimum observability set is a P0 prerequisite for validation in M7). |
| Doc refs | `memory-roadmap.zh-CN.md` §4.2; §9 items 4–5; observability metric names in §4.2 (B-1) and §11.1; existing `llm-agent/memory/working.go:54-63,166-197`, `manager.go:96-149`. |
| Independently shippable | Yes. |
| Exit criteria | 1. `Observer` interface defined; zero-config is no-op. 2. The 7 metrics from B-1 emitted: `memory_add_total`, `memory_search_total`, `memory_search_hits`, `memory_consolidated_total`, `memory_forgotten_total`, `memory_snapshot_items`, `memory_snapshot_vectors_bytes`. 3. Working Add+eviction makes exactly 1 embed call (down from 2); behavior test asserts capacity-full eviction picks lowest-scored item. 4. Parallel `SearchAll`/`ListAll` benchmark shows wall-time reduction vs serial; error semantics identical. 5. No third-party metrics SDK dependency. |
| Complexity | M |
| Detailed plan file | `docs/superpowers/plans/2026-06-XX-m2-phase-b-observability.md` |

### M3 — Phase C: Write Policy Interface + Persistence Backends

| Attribute | Value |
|---|---|
| Goal | Produces (a) an explicit write-policy `Decide(ctx, input)` interface unifying remember/infer/reject, and (b) at least one non-filesystem `SnapshotStore` (SQLite or Postgres). |
| Phase mapping | Phase C (`docs/memory-roadmap.zh-CN.md` §4.3, items C-1/C-2). |
| P0/P1/P2 | P1 (policy DSL); P0 prerequisite for M5 (a real DB driver must exist in the sibling first). |
| Doc refs | `memory-roadmap.zh-CN.md` §4.3; existing `llm-agent/memory/policy_hook.go:8-24,67-79`, `persistence.go:178-247`, `manager.go:291-379` (Import/Export). |
| Independently shippable | Yes (each sub-deliverable independently). |
| Exit criteria | 1. `WritePolicy` interface with `Decide(ctx, input) -> {kind, importance, tags, keep}`; covers user-saved, agent-inferred, reject, redact. 2. `SQLiteStore` or `PostgresStore` implementing `SnapshotStore`; Save/Load/Delete/List all tested. 3. Round-trips through `ImportAll` / `ExportAll`. 4. Stdlib-only assertion still passes for core `llm-agent` (new deps added to `llm-agent-memory` only). 5. Migration script or in-code migrator for store schema v1. |
| Complexity | M (sqlite) or L (postgres). Recommend SQLite first for M3, defer Postgres to M5. |
| Detailed plan file | `docs/superpowers/plans/2026-07-XX-m3-phase-c-policy-and-stores.md` |

### M4 — Phase D v1 Breaking: Capability Interfaces + RecallEngine

| Attribute | Value |
|---|---|
| Goal | Produces a v1.0.0 API of `llm-agent-memory/` where `ManagerOptions` accepts capability interfaces (not concrete types) and a unified `RecallEngine` is the public recall surface. |
| Phase mapping | Phase D (`docs/memory-roadmap.zh-CN.md` §5.1, items D-1/D-2). |
| P0/P1/P2 | P0 prerequisite for M5/M6 (Gateway cannot evolve cleanly while `ManagerOptions` is concrete). |
| Doc refs | `memory-roadmap.zh-CN.md` §5.1; existing `llm-agent/memory/manager.go:22-35`, `policy_hook.go:37-45,53-60`, `memory.go:24-31`, `context/builder.go:48-56`. |
| Independently shippable | Yes — first v1.0.0 tag of `llm-agent-memory`. |
| Exit criteria | 1. Manager construction moves to sibling-owned interface-typed options with `Memory`, `Lister`, `Exporter`, `Importer`, optional `LifecycleMemory`. 2. `WithSanitizer`-wrapped memory installs into `Manager` without a cast. 3. `RecallEngine` facade exposes `Recall(ctx, query, opts) -> UnifiedRecall`; tier-awareness becomes internal. 4. Migration guide doc (`docs/memory-v1-migration.zh-CN.md`) lists the direct upgrade path. 5. No compatibility shim or fallback path is kept in the SDK; consumers move directly to the new sibling surface. 6. All M1–M3 tests adapted; no test deletion without justification. |
| Complexity | L |
| Detailed plan file | `docs/superpowers/plans/2026-07-XX-m4-phase-d-capability-interfaces.md` |

### M5 — Postgres Truth Source + Outbox Schema (Ready-to-Code Minimum)

| Attribute | Value |
|---|---|
| Goal | Produces the Postgres schema, migrations, and durable DAL in the dedicated `llm-agent-memory-postgres` module, with `llm-agent-memory` remaining a pure SDK and `llm-agent-memory-gateway` reserved for later service composition. |
| Phase mapping | Service-layer P0 from `docs/memory-roadmap.zh-CN.md` §11.1; specifically the truth-source + Outbox half of the v1 minimum closed loop. |
| P0/P1/P2 | P0. |
| Doc refs | `memory-postgres-outbox-schema.zh-CN.md` §3 (tables list), §4 (`memory_record`), §5 (`memory_event`), §6 (`memory_idempotency`), §7 (`outbox_event`), §8 (optional `memory_recall_invalidation`), §9 (write-transaction sketches), §10 (OCC), §11 (minimum index set), §12 (invariants); `multi-service-memory-architecture.zh-CN.md` §8.3 (Outbox), §8.4 (version-race / stale-resurrection). |
| Independently shippable | Yes — a usable `llm-agent-memory-postgres/postgres` backend package plus `llm-agent-memory-postgres/cmd/memory-migrate`. |
| Exit criteria | 1. `llm-agent-memory` remains SDK-only and owns only backend-neutral durable abstractions. 2. SQL migrations in `llm-agent-memory-postgres` create all 4 mandatory tables with §11 minimum indexes; `memory_recall_invalidation` deferred. 3. The Postgres backend exposes `WriteRecord` / `PatchRecord` / `DeleteRecord` / `PinRecord` / `DisableRecord` — each one a single transaction covering `memory_record` + `memory_event` + `outbox_event` + (where applicable) `memory_idempotency`. 4. OCC: every modify call requires `expected_version` and returns a typed `ErrVersionConflict`. 5. Outbox relay polls `status='pending'`, publishes to a pluggable `Publisher` interface (initial impl: in-memory + log), and marks rows sent or failed. 6. Idempotency: same `(tenant_id, idempotency_key, request_hash)` returns the prior result; different hash returns `ErrIdempotencyConflict`. 7. Migration tool runnable via `llm-agent-memory-postgres/cmd/memory-migrate`. 8. `llm-agent-memory-gateway` stays skeleton-only in M5. |
| Complexity | L |
| Detailed plan file | `docs/superpowers/plans/2026-05-26-m5-postgres-outbox.md` |

### M6 — Memory Gateway HTTP API (First-Batch 7 Endpoints)

| Attribute | Value |
|---|---|
| Goal | Produces a runnable `llm-agent-memory-gateway` HTTP/service module exposing the §11 first-batch 7 endpoints, composed against SDK abstractions plus the M5 Postgres backend. |
| Phase mapping | The Gateway half of `docs/memory-roadmap.zh-CN.md` §11.1 P0; `docs/memory-gateway-api-contract.zh-CN.md` §11 first batch. |
| P0/P1/P2 | P0. |
| Doc refs | `memory-gateway-api-contract.zh-CN.md` §2 (response headers + errors), §3 (object model), §4.1 (`POST /memory/recall/unified`), §5.1 (`POST /memory/write`), §6.1 (`pin`), §6.3 (`disable`), §6.5 (`PATCH`), §7.2 (`DELETE`), §8.1 (`POST /memory/sessions/{id}/close`), §9 (read-only / consistency-level defaults), §10 (idempotency + `expected_version`), §11 (first vs second batch); `multi-service-memory-architecture.zh-CN.md` §5.1–5.4 (write-path failure semantics), §6.1–6.4 (recall boundaries / token-budget / session), §8.2.1 (vector-index reliability), §13.4 (decision trace JSON). |
| Independently shippable | Yes — gateway module and binary, without pushing HTTP concerns into the SDK or backend module. |
| Exit criteria | 1. All 7 first-batch endpoints implemented end-to-end: `POST /memory/recall/unified`, `POST /memory/write`, `PATCH /memory/items/{memory_id}`, `POST /memory/items/{memory_id}/pin`, `POST /memory/items/{memory_id}/disable`, `DELETE /memory/items/{memory_id}`, `POST /memory/sessions/{session_id}/close`. 2. Tenant boundary: auth-extracted `tenant_id`/`user_id` always override client-claimed scope; DB-side enforcement on every read/write. 3. `idempotency_key` required on writes; OCC `expected_version` required on modifies. 4. Error model conforms to §2.2/§2.3 exactly. 5. Response headers include `X-Request-Id`, `X-Memory-Version`, `X-Consistency-Level`. 6. Recall path accepts `consistency_level` (`eventual`/`bounded`/`strong`) with at minimum `eventual` and `strong` behaviors implemented; `strong` bypasses any cache. 7. Recall hits include `token_cost_estimate` in `metadata`. 8. Decision trace emitted as structured logs covering the 4 stages (`recalled` / `selected` / `dropped` / `promote_decided`) with the §13.4 minimum reason enum. 9. Vector index abstraction `VectorIndex` defined; first impl is in-process (reuse `scoredStore`); pgvector/Milvus deferred. 10. Smoke test in CI: spin up gateway, run write→recall→pin→disable→delete sequence. |
| Complexity | L |
| Detailed plan file | `docs/superpowers/plans/2026-09-XX-m6-memory-gateway.md` |

**Open question to resolve at M6 kick-off**: HTTP framework — stdlib `net/http` (recommended; matches sibling style) vs. chi/gin. Default: stdlib + lightweight in-tree router.

### M7 — Validation Telemetry + Decision Trace Persistence (rescoped 2026-05-27)

| Attribute | Value |
|---|---|
| Goal | Produces persisted decision traces (best-effort async, Postgres-backed) and the measurable subset of the v1 validation metric set — 10 counters across Cost (6) / Lifecycle (2) / Recall (2). The Consolidation Worker, Working-tier introduction, atomic promotion API, two-layer dedupe primitive, typed `RecallObserver`, `LastAccessAt` write path, and the remaining 12 counters of the v1 4-class set were originally part of M7 but are moved to M8 after cross-review (Plan-type internal agent + Codex CLI) showed they require substrate changes that belong with the v2-breaking work. Rescope rationale documented in `docs/superpowers/specs/2026-05-27-m7-workers-and-validation-design.md` §2. |
| Phase mapping | `docs/memory-roadmap.zh-CN.md` §11.1 P0 (decision trace + measurable metric subset); `multi-service-memory-architecture.zh-CN.md` §9.12 (validation methods — partially covered), §13.4 (reason enum **not yet frozen** — deferred to M8). |
| P0/P1/P2 | Decision-trace persistence: P0. Cost-class counters: P0. Lifecycle subset (`episodic_disabled_total`, `episodic_deleted_total`): P0. Recall subset (`recall_returned_total`, `recall_selected_total`): P0. |
| Doc refs | `memory-roadmap.zh-CN.md` §11.1 (decision-trace persistence + 4-class metrics — measurable subset only); rescope rationale and full deferred-item disposition in the M7 spec §2 and §13. |
| Independently shippable | Yes — gateway-only code changes plus one new Postgres migration (`memory_decision_trace` table). No new sibling module. |
| Exit criteria | 1. `memory_decision_trace` table migrated in `llm-agent-memory-postgres`. 2. Gateway-internal `DecisionTraceSink` interface + Postgres impl in `llm-agent-memory-gateway/internal/service/`. 3. Sink is best-effort async with bounded channel and a `trace_dropped_total` loss counter. 4. Every existing `TraceEmitter.Emit` call site mirrors to `DecisionTraceSink.Record`; existing log emission preserved. 5. 10 counters emitted via the existing gateway `internal/observability/metrics.go` exporter — 6 Cost (`embedding_request/applied/tokens/cost_total`, `memory/vector_storage_bytes_total`), 2 Lifecycle (`episodic_disabled_total`, `episodic_deleted_total`), 2 Recall (`recall_returned_total`, `recall_selected_total`). 6. Storage-bytes cron goroutine in gateway, configurable interval (default 5 min). 7. Cardinality rule: only `tenant_bucket` allowed as label across all 10 counters. 8. Cross-tenant isolation test: a forged `tenant_id` cannot read/write trace rows across boundary. 9. **No SDK changes.** **No** new event types. **No** new sibling module. |
| Complexity | M |
| Detailed plan file | `docs/superpowers/plans/2026-05-27-m7-validation-telemetry-and-trace.md` |

### M8 — Phase E v2 Breaking: Storage Refactor + Working Tier + Consolidation Worker + Full Validation Set

| Attribute | Value |
|---|---|
| Goal | Produces a v2.0.0 release that bundles every breaking-or-substrate-level change into one upgrade window: scope-as-first-class storage key, `scoredStore` concurrency refactor, pluggable vector backend, `Metadata`→typed-field promotion (Phase E original scope) **plus** the substrate originally targeted at M7 — Working-tier schema, atomic promotion SDK API, two-layer dedupe primitive, typed `RecallObserver` carrying `memory_id` + `request_id` + `reason`, `LastAccessAt` / `HitCount` write path, the async Consolidation Worker as a new sibling module, the remaining 12 v1 validation counters that depend on any of the above, and the freeze of the reason enum from `multi-service-memory-architecture.zh-CN.md` §13.4. |
| Phase mapping | Phase E (`docs/memory-roadmap.zh-CN.md` §6.1, items E-1/E-2/E-3/E-4) + deferred M7 items (Worker, Working tier, atomic promotion, dedupe, typed observer, full metric set, reason-enum freeze). |
| P0/P1/P2 | Storage refactor / scope-as-key: P0. Working tier + atomic promotion + Consolidation Worker: P0. Typed `RecallObserver`: P0. Full reason enum + remaining 12 counters: P0. Vector-similarity dedupe + learned features: P2 / out-of-scope. |
| Doc refs | `memory-roadmap.zh-CN.md` §6.1 (Phase E items), §11.2 (workers + dedupe); `multi-service-memory-architecture.zh-CN.md` §5.3 (promote conditions), §5.4 (failure state machine), §8.1 (recommended Postgres fields), §8.2.1 (vector-index reliability), §8.4 (version-fence rule), §13.4 (reason enum + JSON schema), §9.12 (full validation methods); deferred-item rationale in `docs/superpowers/specs/2026-05-27-m7-workers-and-validation-design.md` §13. |
| Independently shippable | Yes — v2.0.0 tag with an explicit migration window. Will be split into ≥3 sub-PRs per the M8 risk row. |
| Exit criteria | 1. `scoredStore` no longer takes a single `sync.Mutex`; either RWMutex+immutable-iter-view, CoW, or shard-by-kind. Benchmark shows ≥2× concurrent-read throughput at 10k items. 2. Episodic/Semantic accept pluggable `VectorBackend`; pgvector backend implemented and wired to the existing Postgres deployment. 3. Scope becomes a primary column on `memory_record`; in-memory store gains a parallel partition key; "filter at result side" code paths are deleted. 4. `MemoryItem` v2 lifts `Source` / `Category` / `Pinned` / `Disabled` / `Scope` out of `Metadata` to typed fields. 5. Snapshot version bumped to `2`; migration tool reads v1 and writes v2. 6. **Working tier introduced** — `memory_record.kind` extended (or a paired typed column added) to include `working`; existing rows migrated with a documented default; write path defaults new records to `working`. 7. **Atomic promotion SDK API** — SDK gains a single bundled operation that atomically updates record + appends event + enqueues outbox; Postgres backend implements it as one transaction. 8. **Dedupe primitive** — SDK gains an atomic cross-record survivor-selection operation; Postgres backend uses a unique index + lock ordering. 9. **Typed `RecallObserver`** — carries `memory_id`, `request_id`, `stage`, `reason`, `was_promoted`; gateway recall path emits via this seam. 10. **`LastAccessAt` / `HitCount` write path** — recall path persists access marks (may batch). 11. **Consolidation Worker** — new `llm-agent-memory-worker` sibling module; consumes outbox via the SDK `MessagePublisher` seam; promotes Working→Episodic with the §5.3 default rules, version-fence per §8.4, two-layer dedupe; idempotent under at-least-once + relay redelivery. 12. **Relay delivery hardening** — per-row ack or durable `processing` state with lease/heartbeat (codex C-1 mitigation). 13. **Reason enum frozen** at the SDK boundary per §13.4; freeform reasons rejected. 14. **Remaining 12 counters land** — 5 Promote, 4 Working-lifecycle (`working_expired/promoted/dropped_before_use_total`, `memory_stale_hit_total`), 1 reason-bucketed `recall_dropped_total`, plus `recall_helpful_total` / `recall_unhelpful_total` paired with a feedback-ingest endpoint on the gateway. 15. Vector-similarity dedupe (P2) gated behind an opt-in policy flag — non-default. 16. Decommission plan for `llm-agent/memory/` declared; final patch release of core memory cuts. |
| Complexity | XL (split into sub-PRs) |
| Detailed plan file | `docs/superpowers/plans/2027-XX-XX-m8-phase-e-storage-refactor.md` |

---

## 3. Cross-Cutting Concerns

| Concern | Approach |
|---|---|
| **Testing strategy** | Per-milestone: stdlib `testing` + table-driven tests. M5+ adds `dockertest`-style integration tests behind a build tag (`-tags=integration`) so umbrella CI default stays fast. M6 adds an in-process HTTP smoke test in the umbrella `umbrella.yml`. M7 adds a cross-tenant fuzz/property test. Per-subproject test invocation: `GOWORK=off go test ./... -count=1`. |
| **Multi-tenant safety enforcement** | DB-side enforcement is mandatory per `memory-gateway-api-contract.zh-CN.md` §2.4 ("tenant filtering must DB side enforce"). Every DAL query must include `tenant_id` in `WHERE`; reviewed via a CI grep rule from M5 onwards. Vector-index results are treated as candidates only — final filtering happens in Postgres (`multi-service-memory-architecture.zh-CN.md` §8.2.1). |
| **Observability / decision trace** | Observer hooks land in M2 (no third-party SDK). M6 wires structured-log decision traces. M7 persists them via best-effort async sink and exposes the **measurable subset** (10/22) of the v1 4-class metrics. The reason enum in `multi-service-memory-architecture.zh-CN.md` §13.4 is **deferred to M8** (paired with the typed `RecallObserver`); narrow M7 accepts free-form `reason` strings in the trace table. `memory_id` stays out of metric labels (high cardinality) — only `tenant_id` (or bucket) is allowed. |
| **Dependency policy** | `llm-agent/` (core) remains **stdlib-only**, enforced by `scripts/stdlib-only-check.sh` (B4 gate). `llm-agent-memory/` may take third-party deps (precedent: `llm-agent-rag/go.mod` pulls `pgx/v5` and `pgvector-go`). Hard rules for the new module: (a) no transitive pulling-in of `llm-agent-rag` or other siblings — memory does not depend sideways; (b) every new direct dep needs a 1-sentence justification in the milestone plan; (c) prefer stdlib `net/http`, `database/sql`, `encoding/json` over heavier frameworks. |
| **Backwards compat with `llm-agent/memory/` consumers** | M1–M3 are pure additive — old SDK still works. M4 is the intentional boundary reset: consumers move directly to the sibling-owned `memory.Manager` / `RecallEngine` surface with the migration guide as the canonical upgrade path. No compatibility shim is preserved in the SDK. Snapshot format gets a version bump at M8 only (v1→v2). Symptomatically: anyone on `llm-agent` v0.5+ can adopt `llm-agent-memory` up to M3 with zero code changes beyond `import` swap; from M4 onward they must update construction and integration code explicitly. |

---

## 4. Risks & Open Questions

### Per-milestone risks (max 2)

| Milestone | Top risks |
|---|---|
| M0 | (1) CI matrix expansion blocked by sibling-repo checkout pattern (umbrella checks out each sibling by ref `main`/`master`); need to register `llm-agent-memory` on GitHub first. |
| M1 | (1) Cross-scope `Consolidate` test exposes latent unscoped data; need a migration plan for items without scope metadata. (2) `SearchUnified` cross-tier score-normalization choice is essentially product-policy; risk of choosing wrong heuristic. |
| M2 | (1) Observer hooks may leak callsite information that consumers later assume is stable; lock the event payload schema early. (2) `evictIfOverCapacity` refactor risks subtle eviction-order regression. |
| M3 | (1) Picking SQLite vs Postgres for first non-fs store affects M5 — recommend SQLite to keep M3 independent of infra. (2) `WritePolicy` interface scope-creep into a business-DSL — keep it to {kind, importance, tags, keep}. |
| M4 | (1) Breaking-change blast radius across siblings (rag, customer-support). Mitigation: `compat/` shim + 1 release window. (2) `RecallEngine` design may over-commit to a model that doesn't survive M8. |
| M5 | (1) Postgres driver choice (`pgx/v5` vs `lib/pq`) is sticky; rag already uses `pgx/v5` — recommend match. (2) Outbox relay at-least-once + worker idempotency mismatch could cause double-promote. |
| M6 | (1) Token-cost estimator is approximate at v1; risk of consumers treating it as authoritative. (2) `strong` consistency without a real cache may be vacuously trivial — re-evaluate when cache lands. |
| M7 | (1) Trace-sink async buffer overflow may mask real signal under load — `trace_dropped_total` must be alerted on, not just exported. (2) Storage-bytes cron does a periodic full aggregate; at >1M records per tenant it becomes a slow query and may need partitioned reads. (Promote-threshold and broader metric-cardinality risks have moved to M8 with the deferred work.) |
| M8 | (1) Storage refactor + scope-first-class is the highest-risk change; must be split into ≥3 sub-PRs. (2) Snapshot v1→v2 migration: legacy unscoped data needs an explicit dump target. |

### Open questions per milestone (must resolve at kick-off)

- **M0**: Does `llm-agent-memory` get its own GitHub repo (matching the per-subproject pattern in `umbrella.yml`) or live in-tree under the umbrella? Recommended: own repo, registered like siblings.
- **M1**: Fork or wrap the three modified files in core? Recommend wrap.
- **M2**: Observer payload schema — typed struct per event or single `Event{Name, Attrs map[string]any}`?
- **M3**: SQLite vs Postgres for the first non-fs store? Recommend SQLite; Postgres comes for free at M5.
- **M4**: What is the minimum surface the `compat/` shim must preserve? Audit `llm-agent-rag` and `llm-agent-customer-support` usage of `memory.ManagerOptions` first.
- **M5**: `pgx/v5` (matches rag) vs `database/sql + lib/pq`? Recommend `pgx/v5`. MQ publisher interface: which first impl — log/in-memory only, or include NATS?
- **M6**: HTTP framework — stdlib `net/http` (recommended) or chi/gin? Auth model — JWT vs HMAC-signed header for tenant binding?
- **M7**: Resolved at 2026-05-27 rescope: traces persist in same Postgres (`memory_decision_trace`), best-effort async via a bounded channel; no queue runtime needed (no async consumption left in narrow M7). Only remaining open: trace retention policy (default proposal in spec §11 OD-5).
- **M8**: Vector backend choice — pgvector (already in rag), Qdrant, or Milvus? Migration path for v0/v1 unscoped data — back-fill with `tenant_id=legacy`? **New (absorbed from M7 rescope):** Working-tier representation — extend `memory_record.kind` enum vs add a dedicated `tier` column? Atomic-promotion API shape — extend `RecordStore` with a `Promote` method vs introduce a top-level transactional `MemoryStore.Apply` primitive? Worker queue runtime — keep polling relay (with per-row ack hardening per codex C-1) or introduce NATS/Kafka? Reason-enum freeze process — how to migrate or null out free-form `reason` rows that M7 already persisted?

---

## 5. Recommended First Detailed Plan

**Recommendation: M0 + M1 combined.**

Rationale:
- M0 alone is too small to be worth a standalone plan; it's mostly scaffolding (one go.mod, one workflow update, one stub package).
- M1 has real design decisions (fork vs wrap, scope-normalization for unscoped legacy items, score-merge heuristic in `SearchUnified`) that benefit from a dedicated plan.
- Combining them lets the plan exercise the new module end-to-end (scaffold + first real feature) before the team commits to M2 observability work.
- M0+M1 together is roughly one "phase" of effort (~2 weeks at side-project pace).

Suggested filename for the combined plan: `docs/superpowers/plans/2026-05-26-m0-m1-scaffold-and-phase-a.md`.

Subsequent plans should split: M2 alone (focused on hook interface design), M3 alone (sqlite store deserves its own plan), then each of M4/M5/M6/M7/M8 as standalone documents.

---

## Self-Review Checklist

- [x] Every Phase (A–E) from `memory-roadmap.zh-CN.md` §4–6 mapped: A→M1, B→M2, C→M3, D→M4, E→M8.
- [x] Every first-batch endpoint in `memory-gateway-api-contract.zh-CN.md` §11 (7 endpoints) covered by M6.
- [x] Every table in `memory-postgres-outbox-schema.zh-CN.md` §3 (4 mandatory + 1 optional) covered by M5.
- [x] M0 produces something testable (`go test ./...` runs).
- [x] Each milestone depends only on prior milestones — no forward refs (verified M0→M1→M2→M3→M4→M5→M6→M7→M8).
- [x] Decision trace minimum (`reason` enums) on roadmap: M6 (emit free-form via `TraceEmitter`), M7 (persist free-form via best-effort sink), M8 (freeze enum at SDK boundary). Reason→counter map deferred to M8 with the typed observer.
- [~] v1 4-class validation metrics (recall / promote / lifecycle / cost): partially landed across M7 + M8 after 2026-05-27 rescope. M7 ships 10/22 counters (measurable subset); the remaining 12 land in M8 once Working tier, Consolidation Worker, typed `RecallObserver`, and `LastAccessAt` write path land.
