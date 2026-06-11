# llm-agent-kb M3 (GraphRAG) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the shipped `llm-agent-kb` M1+M2 backend (module published at **v0.2.0**) with the §13 **M3** scope: wire the rag GraphRAG components (`graph.LLMEntityExtractor` + `graph.LouvainDetector` + `graph.LLMCommunitySummarizer`, plus optional `graph.EmbeddingEntityResolver`) into `rag.New`, expand the narrow `ragsvc.RagPort` with `AskGlobal` / `AskDrift` / `PrewarmCommunityReports` + community-view reads, prewarm community reports after a successful Import, add the `POST /api/kb/{id}/ask/global` + `POST /api/kb/{id}/ask/drift` + community-view endpoints, and trigger a namespace community **recompute** after document/kb deletes (§16.4). Folds in the deferred M2 follow-up `GET /api/kb/{id}/documents/{docId}` single-doc fetch. All M1/M2 behavior (auth, vector/hybrid ask, citations, async ingest, delete cascade) is preserved.

**Architecture:** Same single Go binary `kbd` (BFF + embedded `rag.System`). M3 grows four packages and adds none:
- `ragsvc` (the sole importer of `rag`/`postgres`/`otelrag`, now also `rag/graph`): `Deps` gains the three graph-component seams (interface-typed so tests inject deterministic substitutes); `New` passes them into `rag.Options{EntityExtractor, EntityResolver, CommunityDetector, CommunitySummarizer}`; `RagPort` gains `AskGlobal`/`AskDrift`/`PrewarmCommunityReports` (delegated to `Wrapper.Inner()` = `*rag.System`, with kb-self-instrumented spans because `*otelrag.Wrapper` lacks them) and the two community reads `ListCommunities`/`CommunityReport` (delegated to the held `*postgres.Store`, which satisfies `store.CommunityStore`).
- `retrieval`: gains `AskGlobal`/`AskDrift` use-cases that map `rag.Answer` (+ `Diagnostics.Global`/`Diagnostics.Drift`) to the kb JSON shape — global/drift answers carry **no** `Citations`, handled gracefully.
- `ingest`: `worker.process` calls `PrewarmCommunityReports(ctx, namespace)` after a successful Import (best-effort, logged, never fails the job); `delete.go` gains a `RecomputeCommunities` call after the cascade (full-namespace Louvain recompute via the rag re-detect path + prewarm), invoked by `DeleteDocument`/`DeleteAllDocumentsForKB`.
- `httpapi`: `POST /api/kb/{id}/ask/global`, `POST /api/kb/{id}/ask/drift` (viewer+), `GET /api/kb/{id}/communities` (list, viewer+), `GET /api/kb/{id}/communities/{cid}` (report, viewer+), `GET /api/kb/{id}/documents/{docId}` (single-doc, viewer+). Widens `Asker` + adds a `CommunityReader` surface; `Ingester` unchanged.
- `cmd/kbd`: `build` passes the production LLM-backed graph components into `ragsvc.Deps`.

Boundaries unchanged (spec §4): `ragsvc` is the ONLY package importing `rag`/`postgres`/`otelrag`/`rag/graph`. `retrieval`/`ingest`/`httpapi` depend only on `ragsvc.RagPort` (+ pool/authz). `httpapi` holds no business rules.

**Tech Stack:** Go 1.26.0 · `github.com/jackc/pgx/v5` v5.9.2 (pgxpool) · `github.com/costa92/llm-agent-authz v0.1.0` · `github.com/costa92/llm-agent-rag v1.11.0` (graph/global/drift/community APIs — **all already present, no version bump**) · `github.com/costa92/llm-agent-contract v0.5.0` (`llm.NewScriptedLLM`) · `github.com/costa92/llm-agent-providers v0.7.0` · `github.com/costa92/llm-agent-otel v0.4.0` (`Wrapper.Inner()`) · stdlib `context`, `encoding/json`, `net/http`, `log/slog`. **No new external deps** (graph components live in the already-pinned rag module). If `go build` adds a `go.sum` entry for a rag sub-package, run targeted `GOWORK=off go get github.com/costa92/llm-agent-rag/graph@v1.11.0` (NOT `tidy`).

**Spec:** `docs/superpowers/specs/2026-06-09-llm-agent-kb-design.md` (this plan implements its **M3**, §13). Scope authority: M3 list §13; community building §6 step 4; global/drift APIs §7 (`GlobalOptions{Namespace,MaxCommunities}`, `DriftOptions{Namespace,MaxCommunities,Rounds,TopK}`); endpoints §16.2 (`POST /ask/global`, `POST /ask/drift`, single-doc GET); community-recompute-on-delete §16.4; namespace-only isolation §8 (`GlobalOptions`/`DriftOptions` have **no** `SecurityFilters`); kb-self-instrumented global/drift spans §11. The M1/M2 plans (`2026-06-09-llm-agent-kb-m1.md`, `-m2.md`) are the style/gating/TDD-rhythm reference.

**Live base (read before editing — DO NOT reinvent M1/M2):**
- `internal/ragsvc/ragsvc.go` — `AskRequest{Namespace,TopK,Hybrid,MaxTotalTokens}`; the **narrow** `RagPort` (Ask/Import/ListChunkIDs/RemoveGraphBySource/RemoveChunks — NO global/drift/prewarm); `Deps{Model,Embedder,RagStore,ChunkStore,Tracer}`; `Service{wrapper *otelrag.Wrapper, chunkStore *ragpostgres.Store}`; `New` calls `ragcore.New(ragcore.Options{Model, Embedder, Store})` then `otelrag.Wrap`. M3 widens `Deps`, `RagPort`, `Service`, and `New`'s `Options`.
- `internal/ragsvc/adapters.go` — `ragModelAdapter{inner llm.ChatModel}` (→ `raggenerate.Model`) and `ragEmbedderAdapter{inner llm.Embedder}` (→ `ragembed.Embedder`+`BatchEmbedder`). These are the seams the graph components reuse.
- `internal/retrieval/retrieval.go` — `Config{MaxAskTokens,SnippetChars}`; `AskInput{Namespace,Question,Mode,TopK}`; `Citation`/`AskOutput`; `Service{rag ragsvc.RagPort, cfg}`; `Ask` rejects modes != vector|hybrid. M3 adds `AskGlobal`/`AskDrift` and routes mode `global`/`drift`.
- `internal/ingest/worker.go` — `WorkerConfig{Pool,Rag,Fetcher,WorkerID,Lease,MaxAttempts,BaseBackoff,ParseTimeout,Clock,Logger}`; `process` (parse → `Rag.Import(ReplaceSource:true)` → atomic `document→ready`+`job→done` tx). M3 adds the prewarm call after the success tx commits.
- `internal/ingest/delete.go` — `DeleteDocument(ctx, namespace, documentID)` (List→RemoveGraphBySource→RemoveChunks→delete row) + `DeleteAllDocumentsForKB`. M3 appends a community-recompute step.
- `internal/ingest/ingest.go` — `Service{pool,rag}`, `DocumentStatus(ctx,kbID,docID) (status,phase,cc,err,error)`, `ListDocuments`, `DocumentView`. The single-doc GET reuses `DocumentStatus` + a small `DocumentDetail` read.
- `internal/httpapi/httpapi.go` — `NewMux(Deps)`, `Asker`/`Ingester`/`kbGetter`/`DocStatusReader` interfaces, `chain(min,h)`, route table, `askHandler` (hardcodes `Namespace: "kb_"+id`), `writeJSON`. M3 widens `Asker`, adds `CommunityReader`, adds routes.
- `internal/storage/storage.go` — `businessMigrations`, `Migrate` (rag `Migrate` builds chunks/graph/community tables → business migrations). **No schema change in M3** (community/graph tables already created by `ragStore.Migrate`).
- `cmd/kbd/main.go` — `build(ctx,cfg)` assembly; `ragsvc.New(ragsvc.Deps{Model,Embedder,RagStore,ChunkStore,Tracer})`; `providerOverride` test seam. M3 adds the graph components to the `Deps` literal.
- `internal/config/config.go` — `Config`, `LoadFromLookup`, `envOr/envInt/envBool/envFloat?` (no float helper yet — M3 adds one for the resolver threshold).

**Verified rag GraphRAG APIs (inspected in the on-disk `llm-agent-rag` working tree == tag v1.11.0 — DO NOT change these calls):**
- `(*rag.System).AskGlobal(ctx, question string, opts rag.GlobalOptions) (rag.Answer, error)` — `rag/global.go:62`.
- `(*rag.System).AskDrift(ctx, question string, opts rag.DriftOptions) (rag.Answer, error)` — `rag/drift.go:75`.
- `(*rag.System).PrewarmCommunityReports(ctx, namespace string) (int, error)` — `rag/global.go:247` (returns count generated; store-without-`CommunityStore` → `0, nil`; cache miss with no summarizer → `graph.ErrCommunitySummarizerRequired`).
- `rag.GlobalOptions{Namespace string; MaxCommunities int; MaxTotalTokens int}` — `rag/options.go:196` (**no `SecurityFilters`**).
- `rag.DriftOptions{Namespace string; MaxCommunities int; Rounds int; TopK int; MaxTotalTokens int}` — `rag/options.go:221` (**no `SecurityFilters`**).
- `rag.Answer{Text string; Hits []store.Hit; Citations []rag.Citation; Diagnostics rag.Diagnostics; ...}` — `rag/system.go:30`. **`AskGlobal`/`AskDrift` populate `Text` + `Diagnostics.Global`/`Diagnostics.Drift` and leave `Citations` empty** (`rag/global.go:215` builds `Answer{Text, Diagnostics{Global:...}}` only).
- `rag.Diagnostics{HitCount int; Global rag.GlobalDiagnostics; Drift rag.DriftDiagnostics; ...}` — `rag/system.go:55`.
- `rag.GlobalDiagnostics{CommunityIDs []string; MapScores map[string]int; MapCalls int; ReduceCalls int; ConsultedReports []graph.CommunityReport}` — `rag/system.go:219`.
- `rag.DriftDiagnostics{PrimerCommunityIDs []string; Rounds int; RoundEntityIDs [][]string; ConsultedReports []graph.CommunityReport}` — `rag/system.go:197`.
- `rag.Options{...; EntityExtractor graph.EntityExtractor; EntityResolver graph.EntityResolver; CommunitySummarizer graph.CommunitySummarizer; CommunityDetector graph.CommunityDetector; ...}` — `rag/options.go:255-286`. A nil `EntityResolver` defaults to `graph.NoopEntityResolver{}` (`rag/system.go:411-413`). `Import` auto-extracts entities when `EntityExtractor != nil && store implements GraphStore`, then auto-detects communities when `CommunityDetector != nil && store implements CommunityStore` (`rag/import.go:201-216`). Community reports are lazy (built on AskGlobal cache-miss or by `PrewarmCommunityReports`).
- Graph component constructors are **struct literals** (no `New*` funcs):
  - `graph.LLMEntityExtractor{Model: generate.Model}` — `graph/extract.go:27`; satisfies `graph.EntityExtractor` (`Extract(ctx, chunkID, text) ([]Entity,[]Relation,error)`).
  - `graph.LouvainDetector{Resolution: float64}` (Resolution<=0 → 1.0) — `graph/louvain.go:20`; satisfies `graph.CommunityDetector` (`Detect(ctx, Graph) ([]Community,error)`).
  - `graph.LLMCommunitySummarizer{Model: generate.Model}` — `graph/summary.go:77`; satisfies `graph.CommunitySummarizer` (`Summarize(ctx, Community, Graph) (CommunityReport,error)`).
  - `graph.EmbeddingEntityResolver{Embedder: embed.Embedder; Threshold: float64}` (Threshold<=0 → default high) — `graph/resolve.go:62`; satisfies `graph.EntityResolver` (`Resolve(ctx, []Entity, []Relation) ([]Entity,[]Relation,error)`).
  - **Deterministic test substitutes** (rag's own tests use these — `rag/community_test.go:21`, `rag/global_test.go:54`): `graph.DictionaryEntityExtractor{Terms: map[string]string}` (gazetteer, zero-LLM, `graph/dictionary.go:14`) and a hand-written deterministic `CommunitySummarizer`. We mirror this for gated GraphRAG tests so cursor-based scripted-LLM call-counting is avoided.
- `(*postgres.Store)` satisfies `store.CommunityStore` (`store/store.go:83`): `GraphSnapshot(ctx, ns) (graph.Graph,error)`, `UpsertCommunities(ctx, ns, []graph.Community) error`, `Communities(ctx, ns) ([]graph.Community,error)`, `PutCommunityReport(ctx, ns, graph.CommunityReport) error`, `CommunityReport(ctx, ns, communityID) (graph.CommunityReport,bool,error)` — `postgres/community.go:26-119`. And `store.GraphStore`: `UpsertGraph`/`RemoveGraphBySource`/`Neighborhood`/`FindEntities` — `postgres/graph.go`.
- `graph.Community{ID,Level,ParentID,EntityIDs,RelationIDs}` — `graph/community.go:14`. `graph.CommunityReport{CommunityID,Title,Summary,ContentHash}` — `graph/summary.go:22`.
- `(*otelrag.Wrapper).Inner() *rag.System` — `otelrag/otelrag.go:96`. The Wrapper only instruments `Ask`/`Import`/`Retrieve` (`:99/:124/:145`); global/drift/prewarm MUST go through `Inner()` with kb-self-instrumented spans.

**Conventions (carried from M1/M2, re-verified):** Go commands need `GOWORK=off` (umbrella `go.work` excludes kb). DB-touching tests gate on `LLM_AGENT_KB_PG_URL` + `t.Skipf`. **Each gated test sets up its OWN fresh DB** (drops the rag tables it owns: `chunks`, `chunks_entities`, `chunks_relations`, `chunks_communities`, `chunks_community_reports`) — not co-runnable. GraphRAG tests are made deterministic with `graph.DictionaryEntityExtractor` + `graph.LouvainDetector` + a deterministic summarizer (NOT an LLM-cursor count). Develop on a **new branch** `feat/m3-graphrag` off `main` (mirrors the M2 implementer). The replace-guard pre-commit hook strips local `costa92` replaces — do NOT `--no-verify` unless intentional.

**Gated test DB (build pgvector from source, per constraints):**
```bash
docker run -d --name kb_m3_pg -e POSTGRES_PASSWORD=pw postgres:16-alpine
docker exec -u root kb_m3_pg sh -c 'apk add --no-cache build-base clang19 llvm19-dev git && cd /tmp && git clone --depth 1 --branch v0.8.0 https://github.com/pgvector/pgvector && cd pgvector && make OPTFLAGS="" install'
IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' kb_m3_pg)
export LLM_AGENT_KB_PG_URL="postgres://postgres:pw@$IP:5432/postgres?sslmode=disable"
# teardown when done: docker rm -f kb_m3_pg
```
(Host port binding is blocked — connect via the container bridge IP, no `-p`. `pgvector/pgvector:pg16` is acceptable if reachable; the from-source build is the constraint-mandated fallback.)

---

## File Structure

```
llm-agent-kb/                          module github.com/costa92/llm-agent-kb (v0.2.0, M3 on feat/m3-graphrag)
├── go.mod                                                    # unchanged (rag v1.11.0 already provides graph/*)
├── internal/
│   ├── config/config.go            (CHANGED)                 # + graph enable + Louvain resolution + resolver threshold/enable; + envFloat helper
│   │   config_test.go              (CHANGED)
│   ├── ragsvc/ragsvc.go            (CHANGED)                 # Deps + graph seams; New passes graph Options; RagPort + AskGlobal/AskDrift/Prewarm/community reads
│   │   ragsvc_test.go              (CHANGED)                 # + gated GraphRAG test (deterministic extractor/detector/summarizer)
│   ├── retrieval/retrieval.go      (CHANGED)                 # + AskGlobal/AskDrift use-cases + Global/Drift diagnostics mapping
│   │   retrieval_test.go           (CHANGED)
│   ├── ingest/worker.go            (CHANGED)                 # prewarm after success tx
│   │   worker_test.go              (CHANGED)
│   ├── ingest/delete.go            (CHANGED)                 # community recompute after cascade
│   │   delete_test.go              (CHANGED)
│   ├── ingest/ingest.go            (CHANGED)                 # + DocumentDetail single-doc read
│   │   ingest_test.go              (CHANGED)
│   ├── httpapi/httpapi.go          (CHANGED)                 # widen Asker; + CommunityReader; + ask/global, ask/drift, communities, single-doc GET routes
│   │   httpapi.go handlers.go      (CHANGED)                 # + global/drift/community/single-doc handlers
│   │   ask_test.go / community_test.go (NEW/CHANGED)
│   └── ...
└── cmd/kbd/main.go                 (CHANGED)                 # build wires LLM-backed graph components into ragsvc.Deps
    main_test.go                    (CHANGED)                 # gated e2e: ingest → communities built → AskGlobal answer → AskDrift
```

---

## Task 1 — config: graph knobs + envFloat helper

Add GraphRAG config: enable flag, Louvain resolution, entity-resolver enable + threshold, default MaxCommunities/Rounds/TopK for global/drift. Add an `envFloat` helper (none exists yet).

- [ ] **Step 1: Failing test** — `internal/config/config_test.go` (append)

```go
func TestLoadGraphDefaults(t *testing.T) {
	cfg, err := LoadFromLookup(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.GraphEnabled {
		t.Errorf("GraphEnabled default = false, want true")
	}
	if cfg.LouvainResolution != 1.0 {
		t.Errorf("LouvainResolution = %v, want 1.0", cfg.LouvainResolution)
	}
	if cfg.EntityResolverEnabled {
		t.Errorf("EntityResolverEnabled default = true, want false (opt-in)")
	}
	if cfg.GlobalMaxCommunities != 8 {
		t.Errorf("GlobalMaxCommunities = %d, want 8", cfg.GlobalMaxCommunities)
	}
	if cfg.DriftRounds != 2 {
		t.Errorf("DriftRounds = %d, want 2", cfg.DriftRounds)
	}
}

func TestLoadGraphOverrides(t *testing.T) {
	env := map[string]string{
		"GRAPH_ENABLED":           "false",
		"LOUVAIN_RESOLUTION":      "1.5",
		"ENTITY_RESOLVER_ENABLED": "true",
		"ENTITY_RESOLVER_THRESHOLD": "0.92",
		"GLOBAL_MAX_COMMUNITIES":  "16",
	}
	cfg, err := LoadFromLookup(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GraphEnabled {
		t.Error("GRAPH_ENABLED=false not honored")
	}
	if cfg.LouvainResolution != 1.5 {
		t.Errorf("LouvainResolution = %v, want 1.5", cfg.LouvainResolution)
	}
	if !cfg.EntityResolverEnabled || cfg.EntityResolverThreshold != 0.92 {
		t.Errorf("resolver override not honored: enabled=%v thr=%v", cfg.EntityResolverEnabled, cfg.EntityResolverThreshold)
	}
	if cfg.GlobalMaxCommunities != 16 {
		t.Errorf("GlobalMaxCommunities = %d, want 16", cfg.GlobalMaxCommunities)
	}
}
```

- [ ] **Step 2: Run — fails**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/config/ -run TestLoadGraph`
Expected: compile error — `cfg.GraphEnabled undefined (type Config has no field or method GraphEnabled)`.

- [ ] **Step 3: Minimal impl** — `internal/config/config.go`

Add fields to `Config` (after the M2 SSRF block, before `ServiceName`):

```go
	// M3 GraphRAG (§13 M3, §6 step 4, §7).
	GraphEnabled            bool    // wire EntityExtractor/Louvain/Summarizer into rag.New (default true)
	LouvainResolution       float64 // graph.LouvainDetector.Resolution; <=0 → 1.0
	EntityResolverEnabled   bool    // opt-in near-dup entity merge (graph.EmbeddingEntityResolver)
	EntityResolverThreshold float64 // resolver cosine-similarity threshold; <=0 → rag default
	GlobalMaxCommunities    int     // GlobalOptions.MaxCommunities default (also DriftOptions.MaxCommunities)
	DriftRounds             int     // DriftOptions.Rounds default
	DriftTopK               int     // DriftOptions.TopK default
```

Add to the `cfg := Config{...}` literal in `LoadFromLookup` (after `FetchMaxBytes`):

```go
		GraphEnabled:            envBool(lookup, "GRAPH_ENABLED", true),
		LouvainResolution:       envFloat(lookup, "LOUVAIN_RESOLUTION", 1.0),
		EntityResolverEnabled:   envBool(lookup, "ENTITY_RESOLVER_ENABLED", false),
		EntityResolverThreshold: envFloat(lookup, "ENTITY_RESOLVER_THRESHOLD", 0),
		GlobalMaxCommunities:    envInt(lookup, "GLOBAL_MAX_COMMUNITIES", 8),
		DriftRounds:             envInt(lookup, "DRIFT_ROUNDS", 2),
		DriftTopK:               envInt(lookup, "DRIFT_TOP_K", 5),
```

Add the helper (after `envBool`):

```go
func envFloat(lookup func(string) (string, bool), key string, def float64) float64 {
	if v, ok := lookup(key); ok {
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return f
		}
	}
	return def
}
```

- [ ] **Step 4: Run — passes**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/config/`
Expected: `ok  	github.com/costa92/llm-agent-kb/internal/config	0.0XXs`.

- [ ] **Step 5: Commit**

```bash
cd llm-agent-kb && git checkout -b feat/m3-graphrag 2>/dev/null || git checkout feat/m3-graphrag
git add internal/config/config.go internal/config/config_test.go && \
git commit -m "config: GraphRAG knobs (enable/resolution/resolver/global+drift defaults) — M3 wires graph components per env"
```

---

## Task 2 — ragsvc: expand RagPort with AskGlobal/AskDrift/Prewarm + community reads

The narrow M1 `RagPort` deliberately excluded global/drift/prewarm. M3 adds them. `AskGlobal`/`AskDrift`/`PrewarmCommunityReports` delegate to `Wrapper.Inner()` (`*rag.System`) with kb-self-instrumented spans (the otelrag Wrapper lacks them). The two community-view reads delegate to the held `*postgres.Store` (a `store.CommunityStore`). This task does NOT yet wire graph components into `New` (Task 3) — it adds the methods + a span helper, exercised first with the in-memory store (which supports community ops but, with no detector configured, has zero communities — AskGlobal returns an empty answer, no error).

- [ ] **Step 1: Failing test** — `internal/ragsvc/ragsvc_test.go` (append)

```go
// Compile-time: the widened RagPort surface. Community reads return kb-local
// DTOs (CommunityView/CommunityReportView) so importers never see rag/graph.
var _ interface {
	AskGlobal(ctx context.Context, question string, req GlobalRequest) (ragcore.Answer, error)
	AskDrift(ctx context.Context, question string, req DriftRequest) (ragcore.Answer, error)
	PrewarmCommunityReports(ctx context.Context, namespace string) (int, error)
	ListCommunities(ctx context.Context, namespace string) ([]CommunityView, error)
	CommunityReport(ctx context.Context, namespace, communityID string) (CommunityReportView, bool, error)
} = (*Service)(nil)

func TestAskGlobalEmptyWhenNoCommunities(t *testing.T) {
	// In-memory store, no detector configured → zero communities → AskGlobal
	// returns an empty (no-error) answer. Proves the Inner() delegation + span
	// path compiles and runs without a DB.
	model := llm.NewScriptedLLM(llm.WithResponses(llm.TextResponse("unused")))
	embedder := llm.NewScriptedLLM(llm.WithEmbedDimensions(8))
	svc := New(Deps{Model: model, Embedder: embedder})
	ctx := context.Background()
	ans, err := svc.AskGlobal(ctx, "what themes?", GlobalRequest{Namespace: "ns", MaxCommunities: 4})
	if err != nil {
		t.Fatalf("AskGlobal: %v", err)
	}
	if ans.Text != "" {
		t.Fatalf("AskGlobal over a community-less namespace = %q, want empty", ans.Text)
	}
}
```

(The test file does not yet need `raggraph "github.com/costa92/llm-agent-rag/graph"` — the widened community reads return kb-local DTOs. The `raggraph` import is added to the test file in Task 3 when `fixedSummarizer` is introduced; production `ragsvc.go` still imports `raggraph` for the DTO-mapping in `New`/the methods below.)

- [ ] **Step 2: Run — fails**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/ragsvc/ -run TestAskGlobalEmpty`
Expected: compile error — `svc.AskGlobal undefined` / `GlobalRequest undefined`.

- [ ] **Step 3: Minimal impl** — `internal/ragsvc/ragsvc.go`

Add imports: `raggraph "github.com/costa92/llm-agent-rag/graph"`, and `"go.opentelemetry.io/otel/trace"` is already imported; add `"go.opentelemetry.io/otel"` is NOT needed — use the held tracer. Add a tracer field to `Service` and capture it in `New`.

Add request structs (after `AskRequest`):

```go
// GlobalRequest is the kb-side AskGlobal request. Isolation is Namespace-ONLY
// (spec §8): GlobalOptions has no SecurityFilters; namespace isolation runs
// after RBAC.
type GlobalRequest struct {
	Namespace      string
	MaxCommunities int
	MaxTotalTokens int
}

// DriftRequest is the kb-side AskDrift request. Namespace-only isolation (§8).
type DriftRequest struct {
	Namespace      string
	MaxCommunities int
	Rounds         int
	TopK           int
	MaxTotalTokens int
}
```

Widen `RagPort`:

```go
type RagPort interface {
	Ask(ctx context.Context, question string, req AskRequest) (ragcore.Answer, error)
	Import(ctx context.Context, docs []ragingest.Document, opts ragingest.ImportOptions) (ragingest.ImportResult, error)
	ListChunkIDs(ctx context.Context, namespace, sourceID string) ([]string, error)
	RemoveGraphBySource(ctx context.Context, namespace string, chunkIDs []string) error
	RemoveChunks(ctx context.Context, namespace, sourceID string) (int, error)
	// M3 GraphRAG. AskGlobal/AskDrift/PrewarmCommunityReports delegate to
	// Wrapper.Inner() (*rag.System) with kb-self-instrumented spans — the
	// otelrag Wrapper does NOT instrument the GraphRAG paths (§11).
	AskGlobal(ctx context.Context, question string, req GlobalRequest) (ragcore.Answer, error)
	AskDrift(ctx context.Context, question string, req DriftRequest) (ragcore.Answer, error)
	PrewarmCommunityReports(ctx context.Context, namespace string) (int, error)
	// Community views read directly from the held postgres.Store
	// (a store.CommunityStore) and return kb-local DTOs (not rag/graph types)
	// so importers (retrieval/httpapi) never depend on rag/graph (spec §4).
	ListCommunities(ctx context.Context, namespace string) ([]CommunityView, error)
	CommunityReport(ctx context.Context, namespace, communityID string) (CommunityReportView, bool, error)
}

// CommunityView is the kb-local projection of a graph.Community. Keeping it
// here (not exposing raggraph.Community) makes ragsvc the SOLE importer of
// rag/graph (spec §4) — retrieval/httpapi/cmd-kbd type against these DTOs.
type CommunityView struct {
	ID          string
	Level       int
	ParentID    string
	EntityCount int
}

// CommunityReportView is the kb-local projection of a graph.CommunityReport.
type CommunityReportView struct {
	ID      string // the CommunityID the report describes
	Title   string
	Summary string
}
```

Add a tracer to `Service` + capture in `New` (the otelrag Wrapper holds the same TracerProvider but exposes no GraphRAG spans, so kb keeps its own):

```go
type Service struct {
	wrapper    *otelrag.Wrapper
	chunkStore *ragpostgres.Store
	tracer     trace.Tracer
}
```

In `New`, after building `wrapper`, set the tracer (replace the `return &Service{...}` line):

```go
	tracer := trace.Tracer(nil)
	if d.Tracer != nil {
		tracer = d.Tracer.Tracer("github.com/costa92/llm-agent-kb/internal/ragsvc")
	} else {
		tracer = noop.NewTracerProvider().Tracer("ragsvc")
	}
	return &Service{wrapper: wrapper, chunkStore: d.chunkStoreOrNil(d), tracer: tracer}
```

Simplify: keep the existing `chunkStore: d.ChunkStore` assignment and just add `tracer: tracer`. Add import `"go.opentelemetry.io/otel/trace/noop"`. Final `New` return:

```go
	return &Service{wrapper: wrapper, chunkStore: d.ChunkStore, tracer: tracer}
```

Add the methods (after `RemoveChunks`):

```go
func (s *Service) AskGlobal(ctx context.Context, question string, req GlobalRequest) (ragcore.Answer, error) {
	ctx, span := s.tracer.Start(ctx, "ragsvc.AskGlobal")
	defer span.End()
	span.SetAttributes(attribute.String("rag.namespace", req.Namespace))
	ans, err := s.wrapper.Inner().AskGlobal(ctx, question, ragcore.GlobalOptions{
		Namespace:      req.Namespace,
		MaxCommunities: req.MaxCommunities,
		MaxTotalTokens: req.MaxTotalTokens,
	})
	if err != nil {
		span.RecordError(err)
	}
	return ans, err
}

func (s *Service) AskDrift(ctx context.Context, question string, req DriftRequest) (ragcore.Answer, error) {
	ctx, span := s.tracer.Start(ctx, "ragsvc.AskDrift")
	defer span.End()
	span.SetAttributes(attribute.String("rag.namespace", req.Namespace))
	ans, err := s.wrapper.Inner().AskDrift(ctx, question, ragcore.DriftOptions{
		Namespace:      req.Namespace,
		MaxCommunities: req.MaxCommunities,
		Rounds:         req.Rounds,
		TopK:           req.TopK,
		MaxTotalTokens: req.MaxTotalTokens,
	})
	if err != nil {
		span.RecordError(err)
	}
	return ans, err
}

func (s *Service) PrewarmCommunityReports(ctx context.Context, namespace string) (int, error) {
	ctx, span := s.tracer.Start(ctx, "ragsvc.PrewarmCommunityReports")
	defer span.End()
	span.SetAttributes(attribute.String("rag.namespace", namespace))
	n, err := s.wrapper.Inner().PrewarmCommunityReports(ctx, namespace)
	if err != nil {
		span.RecordError(err)
	}
	return n, err
}

func (s *Service) ListCommunities(ctx context.Context, namespace string) ([]CommunityView, error) {
	if s.chunkStore == nil {
		return nil, fmt.Errorf("ragsvc: chunk store not configured")
	}
	comms, err := s.chunkStore.Communities(ctx, namespace)
	if err != nil {
		return nil, err
	}
	views := make([]CommunityView, 0, len(comms))
	for _, c := range comms {
		views = append(views, CommunityView{
			ID:          c.ID,
			Level:       c.Level,
			ParentID:    c.ParentID,
			EntityCount: len(c.EntityIDs),
		})
	}
	return views, nil
}

func (s *Service) CommunityReport(ctx context.Context, namespace, communityID string) (CommunityReportView, bool, error) {
	if s.chunkStore == nil {
		return CommunityReportView{}, false, fmt.Errorf("ragsvc: chunk store not configured")
	}
	rep, ok, err := s.chunkStore.CommunityReport(ctx, namespace, communityID)
	if err != nil || !ok {
		return CommunityReportView{}, ok, err
	}
	return CommunityReportView{ID: rep.CommunityID, Title: rep.Title, Summary: rep.Summary}, true, nil
}
```

Add imports `"go.opentelemetry.io/otel/attribute"` and `"go.opentelemetry.io/otel/trace/noop"` to `ragsvc.go`.

- [ ] **Step 4: Run — passes**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/ragsvc/ -run 'TestAskGlobalEmpty|TestNewWires'`
Expected: `ok  	github.com/costa92/llm-agent-kb/internal/ragsvc	0.0XXs`.
If `go build` reports a missing `go.sum` entry for `llm-agent-rag/graph`, run `GOWORK=off go get github.com/costa92/llm-agent-rag/graph@v1.11.0` (NOT tidy) and re-run.

- [ ] **Step 5: Commit**

```bash
cd llm-agent-kb && git add internal/ragsvc/ragsvc.go internal/ragsvc/ragsvc_test.go && \
git commit -m "ragsvc: widen RagPort with AskGlobal/AskDrift/Prewarm + community reads — global/drift via Wrapper.Inner() with kb-self spans (§11), reads via held postgres.Store"
```

---

## Task 3 — ragsvc: wire graph components into rag.New (Deps seams)

`New` must pass the graph components into `rag.Options` so `Import` auto-builds the entity graph + communities. `Deps` gains three interface-typed seams (so production injects LLM-backed and tests inject deterministic). The graph components reuse the SAME chat Model + embedder via the existing adapters.

- [ ] **Step 1: Failing test** — `internal/ragsvc/ragsvc_test.go` (append; deterministic GraphRAG, in-memory store — no DB)

> **Why this test does NOT assert community existence here:** `ListCommunities`/`CommunityReport` delegate to the held `*ragpostgres.Store` (Task 2), NOT to the in-memory rag store. With no `RagStore`/`ChunkStore` in `Deps`, `s.chunkStore` is nil and `ListCommunities` returns the "chunk store not configured" guard error — so a no-DB community-read assertion would FAIL regardless of wiring. This task therefore proves only that the `Deps` graph seams compile and `Import` *runs without error* with the deterministic components attached (the auto-extract/auto-detect path executes against the in-memory store internally). The store-backed assertion that communities actually persist + are readable is the keystone of **Task 4** (gated live pgvector), which sets `RagStore`+`ChunkStore` to a real store.

```go
// TestGraphComponentsWiredImport proves New passes the graph components into
// rag.Options so Import drives the auto-extract/auto-detect path without error.
// Uses the DETERMINISTIC DictionaryEntityExtractor + LouvainDetector + a fixed
// summarizer — NO scripted-LLM cursor counting (rag's own community_test.go
// pattern). The in-memory rag store implements CommunityStore, so Import's
// detection block runs; community READS (ListCommunities) go through the held
// postgres.Store, so they are asserted in the gated Task 4, not here.
func TestGraphComponentsWiredImport(t *testing.T) {
	model := llm.NewScriptedLLM(llm.WithResponses(llm.TextResponse("unused")))
	embedder := llm.NewScriptedLLM(llm.WithEmbedDimensions(8))
	svc := New(Deps{
		Model: model, Embedder: embedder,
		EntityExtractor: raggraph.DictionaryEntityExtractor{Terms: map[string]string{
			"alpha": "topic", "bravo": "topic", "carbon": "topic", "delta": "topic",
		}},
		CommunityDetector:   raggraph.LouvainDetector{},
		CommunitySummarizer: fixedSummarizer{},
	})
	ctx := context.Background()
	docs := []ragingest.Document{
		{ID: "d1", SourceID: "d1", Title: "T1", Content: "alpha bravo alpha bravo"},
		{ID: "d2", SourceID: "d2", Title: "T2", Content: "carbon delta carbon delta"},
	}
	// Import must succeed with the graph seams attached (auto-extract +
	// auto-detect run internally against the in-memory store; summarization is
	// lazy so the scripted model's "unused" response is never consumed).
	if _, err := svc.Import(ctx, docs, ragingest.ImportOptions{Namespace: "ns", ReplaceSource: true}); err != nil {
		t.Fatalf("Import with graph components wired: %v", err)
	}
}

// fixedSummarizer is a deterministic CommunitySummarizer (mirrors rag's
// staticSummarizer) so reports are reproducible without an LLM cursor. Reused
// by Task 4's gated live-pgvector test.
type fixedSummarizer struct{}

func (fixedSummarizer) Summarize(_ context.Context, c raggraph.Community, _ raggraph.Graph) (raggraph.CommunityReport, error) {
	return raggraph.CommunityReport{
		CommunityID: c.ID,
		Title:       "Theme " + c.ID,
		Summary:     "Summary of community " + c.ID,
		ContentHash: raggraph.CommunityContentHash(c),
	}, nil
}
```

- [ ] **Step 2: Run — fails**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/ragsvc/ -run TestGraphComponentsWired`
Expected: compile error — `unknown field EntityExtractor in struct literal of type ragsvc.Deps`.

- [ ] **Step 3: Minimal impl** — `internal/ragsvc/ragsvc.go`

Widen `Deps` (add after `Tracer`):

```go
	// M3 GraphRAG seams (spec §6 step 4). Nil leaves the path disabled —
	// Import skips extraction/detection gracefully (rag/import.go:201). Tests
	// inject deterministic substitutes; production injects LLM-backed ones.
	EntityExtractor     raggraph.EntityExtractor
	EntityResolver      raggraph.EntityResolver // optional near-dup merge; nil → rag NoopEntityResolver
	CommunityDetector   raggraph.CommunityDetector
	CommunitySummarizer raggraph.CommunitySummarizer
```

In `New`, widen the `ragcore.Options` literal:

```go
	sys := ragcore.New(ragcore.Options{
		Model:               ragModelAdapter{inner: d.Model},
		Embedder:            ragEmbedderAdapter{inner: d.Embedder},
		Store:               d.RagStore,
		EntityExtractor:     d.EntityExtractor,
		EntityResolver:      d.EntityResolver, // nil → rag defaults to NoopEntityResolver
		CommunityDetector:   d.CommunityDetector,
		CommunitySummarizer: d.CommunitySummarizer,
	})
```

- [ ] **Step 4: Run — passes**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/ragsvc/ -run 'TestGraphComponentsWired|TestAskGlobalEmpty|TestNewWires'`
Expected: `ok  	github.com/costa92/llm-agent-kb/internal/ragsvc	0.0XXs`.

- [ ] **Step 5: Commit**

```bash
cd llm-agent-kb && git add internal/ragsvc/ragsvc.go internal/ragsvc/ragsvc_test.go && \
git commit -m "ragsvc: wire graph EntityExtractor/Resolver/Detector/Summarizer into rag.New Options — Import auto-builds entity graph + communities (§6 step 4)"
```

---

## Task 4 — ragsvc: gated live-pgvector GraphRAG end-to-end

Prove the full GraphRAG path against real pgvector: deterministic extraction + Louvain detection persist communities to the `chunks_communities` table during Import, prewarm fills `chunks_community_reports`, and `AskGlobal` returns a non-empty answer (the scripted model supplies the map+reduce text). This is the keystone correctness proof.

- [ ] **Step 1: Failing test** — `internal/ragsvc/ragsvc_test.go` (append)

```go
// TestGraphRAGLivePgvector ingests two thematically-distinct docs into a real
// pgvector store with the deterministic graph components, asserts communities
// land in the store, prewarms reports, then asserts AskGlobal returns the
// scripted reduce text. Gated on LLM_AGENT_KB_PG_URL. Owns its DB (drops the
// rag tables it uses).
func TestGraphRAGLivePgvector(t *testing.T) {
	ctx := context.Background()
	dsn := os.Getenv("LLM_AGENT_KB_PG_URL")
	if dsn == "" {
		t.Skipf("set LLM_AGENT_KB_PG_URL (pgvector) to run the live GraphRAG test")
	}
	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	poolCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		return ragpostgres.RegisterTypes(ctx, conn)
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	for _, tbl := range []string{"chunks_community_reports", "chunks_communities", "chunks_relations", "chunks_entities", "chunks"} {
		_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS "+tbl+" CASCADE")
	}
	chunkStore, err := ragpostgres.New(pool, ragpostgres.Config{Dimension: 8})
	if err != nil {
		t.Fatal(err)
	}
	if err := chunkStore.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	model := llm.NewScriptedLLM(llm.WithResponses(
		// Map responses MUST carry a `Score:` prefix — rag's parseGlobalMap only
		// assigns a point-score on a leading `Score:` line (global.go:157); plain
		// text parses to score 0, the reduce stage drops all score-0 partials,
		// ReduceCalls stays 0, and AskGlobal returns the "no relevant community
		// information" answer (mirrors rag's own global_test.go scripting).
		llm.TextResponse("Score: 80\nmap-a"), llm.TextResponse("Score: 80\nmap-b"),
		llm.TextResponse("Score: 80\nmap-c"), llm.TextResponse("Score: 80\nmap-d"),
		llm.TextResponse("GLOBAL ANSWER: two themes"),
	))
	embedder := llm.NewScriptedLLM(llm.WithEmbedDimensions(8))
	svc := New(Deps{
		Model: model, Embedder: embedder,
		RagStore: chunkStore, ChunkStore: chunkStore,
		EntityExtractor: raggraph.DictionaryEntityExtractor{Terms: map[string]string{
			"alpha": "topic", "bravo": "topic", "carbon": "topic", "delta": "topic",
		}},
		CommunityDetector:   raggraph.LouvainDetector{},
		CommunitySummarizer: fixedSummarizer{},
	})
	docs := []ragingest.Document{
		{ID: "d1", SourceID: "d1", Title: "T1", Content: "alpha bravo alpha bravo alpha"},
		{ID: "d2", SourceID: "d2", Title: "T2", Content: "carbon delta carbon delta carbon"},
	}
	if _, err := svc.Import(ctx, docs, ragingest.ImportOptions{Namespace: "g1", ReplaceSource: true}); err != nil {
		t.Fatalf("Import: %v", err)
	}
	comms, err := svc.ListCommunities(ctx, "g1")
	if err != nil {
		t.Fatalf("ListCommunities: %v", err)
	}
	if len(comms) == 0 {
		t.Fatal("Import did not persist communities to pgvector")
	}
	if _, err := svc.PrewarmCommunityReports(ctx, "g1"); err != nil {
		t.Fatalf("Prewarm: %v", err)
	}
	rep, ok, err := svc.CommunityReport(ctx, "g1", comms[0].ID)
	if err != nil || !ok {
		t.Fatalf("CommunityReport(%s) ok=%v err=%v", comms[0].ID, ok, err)
	}
	if rep.Title == "" {
		t.Fatalf("prewarmed report has empty title")
	}
	ans, err := svc.AskGlobal(ctx, "what are the themes?", GlobalRequest{Namespace: "g1", MaxCommunities: 8})
	if err != nil {
		t.Fatalf("AskGlobal: %v", err)
	}
	if ans.Text == "" {
		t.Fatalf("AskGlobal returned empty text over a populated namespace")
	}
	if ans.Diagnostics.Global.ReduceCalls != 1 {
		t.Fatalf("expected exactly 1 reduce call, got %d (map=%d)", ans.Diagnostics.Global.ReduceCalls, ans.Diagnostics.Global.MapCalls)
	}
}
```

> **Test-design note (the trickiest M3 question):** With `DictionaryEntityExtractor` (zero-LLM) the scripted Model's cursor is touched ONLY by AskGlobal's map+reduce. Map issues one `Generate` per *selected* community (`rag/global.go:139`) and reduce issues exactly one (`:188`). The script supplies 4 map + 1 reduce responses — generous (after prewarm the reports are cache hits, so summarization does NOT consume the cursor). **Each map response carries a `Score: 80\n` prefix** — `parseGlobalMap` (`rag/global.go:157`) assigns a point-score only when the response's leading line is `Score:` (case-insensitive); the reduce stage keeps only `score>0` survivors and sets `ReduceCalls=1` only when survivors exist (`rag/global.go:164-208`). A plain-text map response parses to score 0 → all partials dropped → `ReduceCalls=0` and the "No relevant community information was found" canned answer → the `Diagnostics.Global.ReduceCalls == 1` and non-empty-answer assertions FAIL. The `Score: 80\n…` prefix mirrors rag's own `global_test.go` (`mapText: "Score: 80\n…"`). If AskGlobal selects fewer than 4 communities the extra map responses are simply unused; if it ever needs more the cursor exhausts and `Generate` returns `ErrScriptExhausted`, surfacing as an AskGlobal error the test would catch. Using an LLM-backed extractor/summarizer here would make the cursor count depend on chunk/community cardinality (brittle) — hence the deterministic substitutes, exactly as `rag/community_test.go` and `rag/global_test.go` do.

- [ ] **Step 2: Run — fails (skips without DB; with DB, before Task 3 it would not compile — but Task 3 already shipped, so this is a fresh assertion)**

Run (no DB): `cd llm-agent-kb && GOWORK=off go test ./internal/ragsvc/ -run TestGraphRAGLivePgvector`
Expected: `--- SKIP: TestGraphRAGLivePgvector (set LLM_AGENT_KB_PG_URL ...)` then `ok`.
Run (with DB, fresh assertion green): same command with `LLM_AGENT_KB_PG_URL` exported.

- [ ] **Step 3: (impl already present from Tasks 2+3 — no new production code)**

- [ ] **Step 4: Run — passes (gated)**

Run: `cd llm-agent-kb && LLM_AGENT_KB_PG_URL="$LLM_AGENT_KB_PG_URL" GOWORK=off go test ./internal/ragsvc/ -run TestGraphRAGLivePgvector -v`
Expected: `--- PASS: TestGraphRAGLivePgvector` … `ok  	github.com/costa92/llm-agent-kb/internal/ragsvc	0.XXs`.

- [ ] **Step 5: Commit**

```bash
cd llm-agent-kb && git add internal/ragsvc/ragsvc_test.go && \
git commit -m "ragsvc: gated live-pgvector GraphRAG e2e — deterministic extractor/Louvain build communities, prewarm fills reports, AskGlobal returns scripted reduce text"
```

---

## Task 5 — retrieval: AskGlobal/AskDrift use-cases + Global/Drift diagnostics mapping

Add the kb-side global/drift use-cases. `AskGlobal`/`AskDrift` answers carry NO `Citations` — map only `Text` + `Diagnostics.Global`/`Diagnostics.Drift` into the kb JSON shape, reusing the existing `AskOutput` (citations will be empty). Route the new modes through the existing `Ask` so `mode=global|drift` no longer errors.

- [ ] **Step 1: Failing test** — `internal/retrieval/retrieval_test.go` (append; uses a fake RagPort)

```go
func TestAskGlobalMapsDiagnostics(t *testing.T) {
	fake := &fakeRag{globalAns: ragcore.Answer{
		Text: "global answer",
		Diagnostics: ragcore.Diagnostics{
			Global: ragcore.GlobalDiagnostics{
				CommunityIDs: []string{"c1", "c2"}, MapCalls: 2, ReduceCalls: 1,
			},
		},
	}}
	svc := New(fake, Config{GlobalMaxCommunities: 8})
	out, err := svc.AskGlobal(context.Background(), GlobalInput{Namespace: "ns", Question: "themes?", MaxCommunities: 4})
	if err != nil {
		t.Fatal(err)
	}
	if out.Answer != "global answer" {
		t.Fatalf("answer=%q", out.Answer)
	}
	if len(out.Citations) != 0 {
		t.Fatalf("global answers carry no citations, got %d", len(out.Citations))
	}
	if out.Diagnostics["mode"] != "global" {
		t.Fatalf("diagnostics.mode=%v", out.Diagnostics["mode"])
	}
	if out.Diagnostics["mapCalls"] != 2 {
		t.Fatalf("diagnostics.mapCalls=%v want 2", out.Diagnostics["mapCalls"])
	}
}

func TestAskDriftMapsDiagnostics(t *testing.T) {
	fake := &fakeRag{driftAns: ragcore.Answer{
		Text: "drift answer",
		Diagnostics: ragcore.Diagnostics{
			Drift: ragcore.DriftDiagnostics{PrimerCommunityIDs: []string{"c1"}, Rounds: 2},
		},
	}}
	svc := New(fake, Config{DriftRounds: 2, DriftTopK: 5})
	out, err := svc.AskDrift(context.Background(), DriftInput{Namespace: "ns", Question: "detail?", Rounds: 0})
	if err != nil {
		t.Fatal(err)
	}
	if out.Answer != "drift answer" || out.Diagnostics["mode"] != "drift" {
		t.Fatalf("unexpected: %+v", out)
	}
	if out.Diagnostics["rounds"] != 2 {
		t.Fatalf("diagnostics.rounds=%v want 2", out.Diagnostics["rounds"])
	}
}
```

Extend the existing test fake (or add to `retrieval_test.go`) so it satisfies the widened `ragsvc.RagPort`. Minimal fake:

```go
type fakeRag struct {
	globalAns ragcore.Answer
	driftAns  ragcore.Answer
}

func (f *fakeRag) Ask(context.Context, string, ragsvc.AskRequest) (ragcore.Answer, error) { return ragcore.Answer{}, nil }
func (f *fakeRag) Import(context.Context, []ragingest.Document, ragingest.ImportOptions) (ragingest.ImportResult, error) { return ragingest.ImportResult{}, nil }
func (f *fakeRag) ListChunkIDs(context.Context, string, string) ([]string, error) { return nil, nil }
func (f *fakeRag) RemoveGraphBySource(context.Context, string, []string) error { return nil }
func (f *fakeRag) RemoveChunks(context.Context, string, string) (int, error) { return 0, nil }
func (f *fakeRag) AskGlobal(_ context.Context, _ string, _ ragsvc.GlobalRequest) (ragcore.Answer, error) { return f.globalAns, nil }
func (f *fakeRag) AskDrift(_ context.Context, _ string, _ ragsvc.DriftRequest) (ragcore.Answer, error) { return f.driftAns, nil }
func (f *fakeRag) PrewarmCommunityReports(context.Context, string) (int, error) { return 0, nil }
func (f *fakeRag) ListCommunities(context.Context, string) ([]ragsvc.CommunityView, error) { return nil, nil }
func (f *fakeRag) CommunityReport(context.Context, string, string) (ragsvc.CommunityReportView, bool, error) { return ragsvc.CommunityReportView{}, false, nil }
```

Add imports to the test: `ragcore "github.com/costa92/llm-agent-rag/rag"`, `ragingest "github.com/costa92/llm-agent-rag/ingest"`, `"github.com/costa92/llm-agent-kb/internal/ragsvc"`. **No `rag/graph` import** — the community reads return kb-local `ragsvc.CommunityView`/`ragsvc.CommunityReportView` DTOs, so `retrieval`/`retrieval_test` never depend on `rag/graph` (spec §4).

- [ ] **Step 2: Run — fails**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/retrieval/ -run 'TestAskGlobalMaps|TestAskDriftMaps'`
Expected: compile error — `svc.AskGlobal undefined` / `GlobalInput undefined`.

- [ ] **Step 3: Minimal impl** — `internal/retrieval/retrieval.go`

Add config fields to `Config`:

```go
	GlobalMaxCommunities int // GlobalOptions.MaxCommunities when the request omits it
	DriftRounds          int // DriftOptions.Rounds default
	DriftTopK            int // DriftOptions.TopK default
```

Add inputs + use-cases:

```go
// GlobalInput is the kb-side AskGlobal input (spec §7). Isolation is
// Namespace-ONLY (§8) — no SecurityFilters on the global path.
type GlobalInput struct {
	Namespace      string
	Question       string
	MaxCommunities int
}

// DriftInput is the kb-side AskDrift input (spec §7). Namespace-only (§8).
type DriftInput struct {
	Namespace      string
	Question       string
	MaxCommunities int
	Rounds         int
	TopK           int
}

// AskGlobal runs GraphRAG global map-reduce. The Answer carries no Citations
// (global answers are community-level), so AskOutput.Citations is empty.
func (s *Service) AskGlobal(ctx context.Context, in GlobalInput) (AskOutput, error) {
	maxC := in.MaxCommunities
	if maxC <= 0 {
		maxC = s.cfg.GlobalMaxCommunities
	}
	ans, err := s.rag.AskGlobal(ctx, in.Question, ragsvc.GlobalRequest{
		Namespace:      in.Namespace,
		MaxCommunities: maxC,
		MaxTotalTokens: s.cfg.MaxAskTokens,
	})
	if err != nil {
		return AskOutput{}, err
	}
	return AskOutput{
		Answer:    ans.Text,
		Citations: []Citation{},
		Diagnostics: map[string]any{
			"mode":         "global",
			"communityIds": ans.Diagnostics.Global.CommunityIDs,
			"mapCalls":     ans.Diagnostics.Global.MapCalls,
			"reduceCalls":  ans.Diagnostics.Global.ReduceCalls,
		},
	}, nil
}

// AskDrift runs GraphRAG drift (global primer + local follow-up). No Citations.
func (s *Service) AskDrift(ctx context.Context, in DriftInput) (AskOutput, error) {
	rounds := in.Rounds
	if rounds <= 0 {
		rounds = s.cfg.DriftRounds
	}
	topK := in.TopK
	if topK <= 0 {
		topK = s.cfg.DriftTopK
	}
	maxC := in.MaxCommunities
	if maxC <= 0 {
		maxC = s.cfg.GlobalMaxCommunities
	}
	ans, err := s.rag.AskDrift(ctx, in.Question, ragsvc.DriftRequest{
		Namespace:      in.Namespace,
		MaxCommunities: maxC,
		Rounds:         rounds,
		TopK:           topK,
		MaxTotalTokens: s.cfg.MaxAskTokens,
	})
	if err != nil {
		return AskOutput{}, err
	}
	return AskOutput{
		Answer:    ans.Text,
		Citations: []Citation{},
		Diagnostics: map[string]any{
			"mode":               "drift",
			"primerCommunityIds": ans.Diagnostics.Drift.PrimerCommunityIDs,
			"rounds":             ans.Diagnostics.Drift.Rounds,
		},
	}, nil
}
```

- [ ] **Step 4: Run — passes**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/retrieval/`
Expected: `ok  	github.com/costa92/llm-agent-kb/internal/retrieval	0.0XXs`.

- [ ] **Step 5: Commit**

```bash
cd llm-agent-kb && git add internal/retrieval/retrieval.go internal/retrieval/retrieval_test.go && \
git commit -m "retrieval: AskGlobal/AskDrift use-cases — map Diagnostics.Global/Drift to kb JSON; global/drift answers carry no citations (handled)"
```

---

## Task 6 — ingest worker: prewarm community reports after a successful Import

After `process` commits the success tx (`document→ready` + `job→done`), call `PrewarmCommunityReports(ctx, namespace)` so the first global query is all-cache-hits (spec §6 step 4, §13 "prewarm"). Best-effort: a prewarm failure is logged, never re-fails the job (the doc is already `ready` and communities are detected — reports are lazily fillable on AskGlobal anyway).

- [ ] **Step 1: Failing test** — `internal/ingest/worker_test.go` (append; uses a fake RagPort that records prewarm calls — DB-free for the prewarm assertion via a thin seam, OR gated). Add a recording fake:

```go
// prewarmRecorder wraps a RagPort and records PrewarmCommunityReports calls.
type prewarmRecorder struct {
	ragsvc.RagPort
	mu        sync.Mutex
	prewarmed []string
}

func (r *prewarmRecorder) PrewarmCommunityReports(ctx context.Context, ns string) (int, error) {
	r.mu.Lock()
	r.prewarmed = append(r.prewarmed, ns)
	r.mu.Unlock()
	return 0, nil
}

func TestWorkerPrewarmsAfterImport(t *testing.T) {
	ctx := context.Background()
	dsn := os.Getenv("LLM_AGENT_KB_PG_URL")
	if dsn == "" {
		t.Skipf("set LLM_AGENT_KB_PG_URL to run the worker prewarm test")
	}
	// ... build pool + storage.Migrate + a kb row + an enqueued paste document
	// (reuse the existing worker_test harness helpers) ...
	rec := &prewarmRecorder{RagPort: realRagSvc} // realRagSvc is the gated ragsvc.New over the test store
	w := NewWorker(WorkerConfig{Pool: pool, Rag: rec, WorkerID: "t", MaxAttempts: 3})
	if ok, err := w.RunOnce(ctx); err != nil || !ok {
		t.Fatalf("RunOnce ok=%v err=%v", ok, err)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.prewarmed) != 1 || rec.prewarmed[0] != "kb_testkb" {
		t.Fatalf("expected one prewarm for kb_testkb, got %v", rec.prewarmed)
	}
}
```

> Use the existing `worker_test.go` setup helpers (the M2 suite already builds a pool, migrates, inserts a kb + a pending paste document, and constructs a gated `ragsvc.Service`). The recorder wraps that service so the assertion needs no new DB plumbing. If the M2 suite has no reusable helper, replicate its minimal fresh-DB setup at the top of this test (drop `chunks*`, `ingest_job`, `document`, `knowledge_base`; migrate; insert one kb with namespace `kb_testkb` + one pending paste doc).

- [ ] **Step 2: Run — fails**

Run: `cd llm-agent-kb && LLM_AGENT_KB_PG_URL="$LLM_AGENT_KB_PG_URL" GOWORK=off go test ./internal/ingest/ -run TestWorkerPrewarmsAfterImport`
Expected: `prewarm recorder length 0` assertion failure (worker does not prewarm yet).

- [ ] **Step 3: Minimal impl** — `internal/ingest/worker.go`

In `process`, immediately AFTER the success-path `tx.Commit(ctx)` succeeds and before `return`, add the best-effort prewarm. Replace the success-path tail:

```go
			if err := tx.Commit(ctx); err != nil {
				w.cfg.Logger.Error("ingest: commit success tx failed", "job", c.jobID, "err", err)
				return
			}
			// M3: community reports are lazy; prewarm so the first AskGlobal is
			// all-cache-hits (§6 step 4). Best-effort — the doc is already
			// 'ready' and communities are detected during Import; a prewarm
			// failure is logged, never re-fails the job.
			if _, perr := w.cfg.Rag.PrewarmCommunityReports(ctx, c.namespace); perr != nil {
				w.cfg.Logger.Warn("ingest: prewarm community reports failed", "namespace", c.namespace, "err", perr)
			}
			return
```

(`WorkerConfig.Rag` is `ragsvc.RagPort`, now carrying `PrewarmCommunityReports` from Task 2 — no signature change to the worker struct.)

- [ ] **Step 4: Run — passes (gated)**

Run: `cd llm-agent-kb && LLM_AGENT_KB_PG_URL="$LLM_AGENT_KB_PG_URL" GOWORK=off go test ./internal/ingest/ -run TestWorkerPrewarmsAfterImport -v`
Expected: `--- PASS: TestWorkerPrewarmsAfterImport`.
Also run the ungated regression: `cd llm-agent-kb && GOWORK=off go test ./internal/ingest/` → `ok` (skips the gated ones).

- [ ] **Step 5: Commit**

```bash
cd llm-agent-kb && git add internal/ingest/worker.go internal/ingest/worker_test.go && \
git commit -m "ingest/worker: prewarm community reports after a successful Import (§6 step 4) — best-effort, never re-fails the job"
```

---

## Task 7 — ingest delete: community recompute after the §16.4 cascade

Deleting a doc/kb leaves communities stale; per §16.4 there is no per-source community delete (Louvain is full-namespace). After the cascade, recompute the namespace's communities. The recompute is: re-detect over the now-reduced graph + `UpsertCommunities` + prewarm. `*rag.System.Import` already re-detects on the next ingest, but a *delete* has no Import — so we trigger it explicitly. The simplest correct trigger that reuses shipped behavior: add a `RecomputeCommunities(ctx, namespace)` to `RagPort` that delegates to a small `*rag.System`-backed path. **Decision:** rag exposes no standalone "re-detect over the current graph" method, but `Import` of an EMPTY doc slice with `ReplaceSource:false` runs the detection block (`rag/import.go:201` keys only on the detector + community-store, not on having new chunks) — verify this, else add a tiny ragsvc method that snapshots+detects+upserts+prewarms via `Inner()`. We take the explicit ragsvc method (no reliance on empty-Import semantics).

- [ ] **Step 1: Failing test** — `internal/ingest/delete_test.go` (append; gated). After importing two docs (communities detected), delete one, then assert the namespace's communities reflect the remaining graph (still present, recomputed) and a recompute was triggered. **Then delete the LAST remaining doc and assert the empty-namespace edge:** the recompute over an empty graph leaves zero communities and returns no error (GraphSnapshot empty → `LouvainDetector.Detect` returns `(nil,nil)`, `graph/louvain.go` early-return on zero entity IDs → `UpsertCommunities` replace-all DELETE leaves the table empty → `PrewarmCommunityReports` returns `(0,nil)` on zero communities, `rag/global.go:256`).

```go
func TestDeleteRecomputesCommunities(t *testing.T) {
	ctx := context.Background()
	dsn := os.Getenv("LLM_AGENT_KB_PG_URL")
	if dsn == "" {
		t.Skipf("set LLM_AGENT_KB_PG_URL to run the delete-recompute test")
	}
	// ... fresh-DB setup (drop chunks*, document, knowledge_base; migrate) ...
	// build a gated ragsvc.Service `rag` with deterministic graph components
	// (DictionaryEntityExtractor{alpha,bravo,carbon,delta} + LouvainDetector +
	// fixedSummarizer) over the test store, and an ingest.Service `s`.
	// Insert kb namespace "kb_del" + import two docs d1(alpha bravo), d2(carbon delta)
	// directly via rag.Import so communities are detected.
	before, _ := rag.ListCommunities(ctx, "kb_del")
	if len(before) == 0 {
		t.Fatal("setup: expected communities before delete")
	}
	if err := s.DeleteDocument(ctx, "kb_del", "d1"); err != nil {
		t.Fatalf("DeleteDocument: %v", err)
	}
	after, err := rag.ListCommunities(ctx, "kb_del")
	if err != nil {
		t.Fatalf("ListCommunities: %v", err)
	}
	// d1's chunks/graph are gone; the recompute re-ran Louvain over the
	// remaining (d2-only) graph and replaced the namespace community set.
	if len(after) == 0 {
		t.Fatal("delete left no communities — recompute did not run over the remaining graph")
	}
	// Every remaining community's entities must belong to the surviving doc's
	// graph (no alpha/bravo-only community lingering). Spot-check report freshness.
	if _, ok, _ := rag.CommunityReport(ctx, "kb_del", after[0].ID); !ok {
		t.Fatal("recompute did not prewarm the surviving community's report")
	}

	// Empty-namespace edge (§16.4): deleting the LAST document leaves the
	// namespace graph empty → GraphSnapshot empty → Detect zero communities →
	// UpsertCommunities replace-all deletes the prior set → Prewarm returns
	// (0, nil). RecomputeCommunities must return no error and leave zero
	// communities (the recompute must not choke on an empty graph).
	if err := s.DeleteDocument(ctx, "kb_del", "d2"); err != nil {
		t.Fatalf("DeleteDocument(last doc): %v", err)
	}
	empty, err := rag.ListCommunities(ctx, "kb_del")
	if err != nil {
		t.Fatalf("ListCommunities after last delete: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("deleting the last document left %d communities, want 0", len(empty))
	}
}
```

- [ ] **Step 2: Run — fails**

Run: `cd llm-agent-kb && LLM_AGENT_KB_PG_URL="$LLM_AGENT_KB_PG_URL" GOWORK=off go test ./internal/ingest/ -run TestDeleteRecomputesCommunities`
Expected: compile error (`s.rag.RecomputeCommunities undefined`) then, once added but un-called, the post-delete report-freshness assertion fails.

- [ ] **Step 3: Minimal impl**

**3a. ragsvc** — add `RecomputeCommunities` to `RagPort` and `Service` (`internal/ragsvc/ragsvc.go`). It snapshots the current graph, re-detects, upserts, and prewarms — all via `Inner()`. The detector + summarizer must be configured (they are, in production + the test).

Add to `RagPort`:

```go
	// RecomputeCommunities re-detects the namespace's communities over the
	// CURRENT (post-delete) graph and refreshes reports. §16.4: communities
	// cannot be deleted per-source (Louvain is full-namespace), so after a
	// delete the caller recomputes. No-op when the store/detector is absent.
	RecomputeCommunities(ctx context.Context, namespace string) error
```

Add the method + store the detector on the Service so recompute can reach it. **Store the detector on `Service`** (capture in `New`):

```go
type Service struct {
	wrapper    *otelrag.Wrapper
	chunkStore *ragpostgres.Store
	tracer     trace.Tracer
	detector   raggraph.CommunityDetector // for §16.4 post-delete recompute; nil → recompute is a no-op
}
```

In `New`, set `detector: d.CommunityDetector` in the returned literal.

```go
func (s *Service) RecomputeCommunities(ctx context.Context, namespace string) error {
	ctx, span := s.tracer.Start(ctx, "ragsvc.RecomputeCommunities")
	defer span.End()
	span.SetAttributes(attribute.String("rag.namespace", namespace))
	if s.chunkStore == nil || s.detector == nil {
		return nil // graceful: no graph capability → nothing to recompute
	}
	snap, err := s.chunkStore.GraphSnapshot(ctx, namespace)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("ragsvc: recompute snapshot: %w", err)
	}
	communities, err := s.detector.Detect(ctx, snap)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("ragsvc: recompute detect: %w", err)
	}
	if err := s.chunkStore.UpsertCommunities(ctx, namespace, communities); err != nil {
		span.RecordError(err)
		return fmt.Errorf("ragsvc: recompute upsert: %w", err)
	}
	// Refresh reports for the new community set (stale-hash entries regenerate).
	if _, err := s.wrapper.Inner().PrewarmCommunityReports(ctx, namespace); err != nil {
		span.RecordError(err)
		return fmt.Errorf("ragsvc: recompute prewarm: %w", err)
	}
	return nil
}
```

> Verified against source: `(*postgres.Store).GraphSnapshot`/`UpsertCommunities` (`postgres/community.go:26,48`), `graph.CommunityDetector.Detect` (`graph/community.go`), `(*rag.System).PrewarmCommunityReports` (`rag/global.go:247`). `UpsertCommunities` is replace-all ("全 namespace 删后重插", §16.4) — exactly the recompute semantics.

**3b. ingest delete.go** — call recompute at the end of `DeleteDocument` and once at the end of `DeleteAllDocumentsForKB` (NOT per-doc — §16.4 "非每删一文档即重算").

In `DeleteDocument`, after the row delete (step 4), append:

```go
	// 5. §16.4: communities cannot be deleted per-source; recompute the
	// namespace's community set over the now-reduced graph + refresh reports.
	if err := s.rag.RecomputeCommunities(ctx, namespace); err != nil {
		return fmt.Errorf("ingest: recompute communities: %w", err)
	}
	return nil
```

(remove the old `return nil`.)

In `DeleteAllDocumentsForKB`, the per-doc loop must NOT recompute each time. Change the loop to call a private cascade that skips recompute, then recompute once. Refactor: extract the steps 1–4 into `deleteDocumentChunksAndRow(ctx, namespace, id)` (no recompute), have `DeleteDocument` call it then recompute, and `DeleteAllDocumentsForKB` call it in the loop then recompute ONCE:

```go
func (s *Service) DeleteDocument(ctx context.Context, namespace, documentID string) error {
	if err := s.deleteDocumentChunksAndRow(ctx, namespace, documentID); err != nil {
		return err
	}
	if err := s.rag.RecomputeCommunities(ctx, namespace); err != nil {
		return fmt.Errorf("ingest: recompute communities: %w", err)
	}
	return nil
}

func (s *Service) deleteDocumentChunksAndRow(ctx context.Context, namespace, documentID string) error {
	ids, err := s.rag.ListChunkIDs(ctx, namespace, documentID)
	if err != nil {
		return fmt.Errorf("ingest: list chunks: %w", err)
	}
	if err := s.rag.RemoveGraphBySource(ctx, namespace, ids); err != nil {
		return fmt.Errorf("ingest: remove graph: %w", err)
	}
	if _, err := s.rag.RemoveChunks(ctx, namespace, documentID); err != nil {
		return fmt.Errorf("ingest: remove chunks: %w", err)
	}
	if _, err := s.pool.Exec(ctx, `DELETE FROM document WHERE id = $1`, documentID); err != nil {
		return fmt.Errorf("ingest: delete document row: %w", err)
	}
	return nil
}

func (s *Service) DeleteAllDocumentsForKB(ctx context.Context, namespace, kbID string) error {
	rows, err := s.pool.Query(ctx, `SELECT id FROM document WHERE kb_id = $1`, kbID)
	if err != nil {
		return fmt.Errorf("ingest: list kb documents: %w", err)
	}
	var docIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		docIDs = append(docIDs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range docIDs {
		if err := s.deleteDocumentChunksAndRow(ctx, namespace, id); err != nil {
			return err
		}
	}
	// §16.4: one recompute after the whole kb cascade (not per-document).
	if err := s.rag.RecomputeCommunities(ctx, namespace); err != nil {
		return fmt.Errorf("ingest: recompute communities: %w", err)
	}
	return nil
}
```

**3c. RagPort-fake audit (compile-break check across the whole module).** Adding the six methods to `RagPort` (`AskGlobal`/`AskDrift`/`PrewarmCommunityReports`/`RecomputeCommunities`/`ListCommunities`/`CommunityReport`) breaks any type that implements `RagPort`. Audited the live kb tree — the ONLY `RagPort` implementor is the concrete `*ragsvc.Service` (kept satisfying via the real methods added in Tasks 2/7); there is **no standalone in-package fake** in `ingest`:
> - `internal/ingest/delete_test.go` — uses a REAL gated `*ragsvc.Service` via the `openIngest` helper (`ragsvc.New(Deps{...})`), NOT a fake. No method to add. ✓ (the `prewarmRecorder` in worker_test embeds `ragsvc.RagPort`, so it inherits the new methods for free.)
> - `internal/ingest/worker_test.go` — uses a REAL gated `*ragsvc.Service` via `freshWorkerDB`. No fake. ✓
> - `internal/ingest/ingest_test.go` — no `RagPort` fake (the `ingest.Service` uses the real `rag`). ✓
> The hand-written fakes that DO need the new methods live in OTHER packages, updated in their own tasks: `internal/retrieval/retrieval_test.go`'s `fakeRag` (Task 5 — adds all six, community reads return kb DTOs) and `internal/httpapi`'s `fakeAsker`/`fakeCommunityReader` + the pre-existing `fakeAsk` (Task 8 — `Asker`/`CommunityReader`, not the full `RagPort`). No in-package ingest fake needs editing.

> **Background-task note (§16.4):** the spec calls for the recompute to run as a background task, not inline on the HTTP delete. M3 keeps the recompute *synchronous within the delete use-case* for correctness + testability (a single Louvain pass over one namespace is cheap), and documents that moving it to the ingest worker queue is a deferred optimization. This is the minimal-correct choice; the recompute is idempotent (replace-all UpsertCommunities) so a future async move is a drop-in.

- [ ] **Step 4: Run — passes (gated) + unit fakes compile**

Run: `cd llm-agent-kb && LLM_AGENT_KB_PG_URL="$LLM_AGENT_KB_PG_URL" GOWORK=off go test ./internal/ingest/ -run 'TestDeleteRecomputes|TestDelete' -v`
Expected: `--- PASS: TestDeleteRecomputesCommunities`; existing M1/M2 delete tests still pass.
Run (ungated compile): `cd llm-agent-kb && GOWORK=off go vet ./internal/ingest/ ./internal/ragsvc/` → no output (clean).

- [ ] **Step 5: Commit**

```bash
cd llm-agent-kb && git add internal/ragsvc/ragsvc.go internal/ingest/delete.go internal/ingest/delete_test.go && \
git commit -m "ingest+ragsvc: §16.4 community recompute after delete — full-namespace Louvain re-detect + UpsertCommunities + prewarm, once per kb cascade (not per-doc)"
```

---

## Task 8 — httpapi: ask/global, ask/drift, community views + single-doc GET

Add the new endpoints. Widen `Asker` with `AskGlobal`/`AskDrift`; add a `CommunityReader` surface (satisfied by `*ragsvc.Service` — but to keep httpapi DB/rag-free in unit tests, route through a thin `*retrieval`-or-`*ragsvc` adapter; we use `*ragsvc.Service` directly via a narrow interface). Add `GET /api/kb/{id}/documents/{docId}` (folded M2 follow-up). All viewer+.

- [ ] **Step 1: Failing test** — `internal/httpapi/community_test.go` (NEW) + extend ask test. DB-free, using fakes.

```go
func TestAskGlobalHandler(t *testing.T) {
	asker := &fakeAsker{globalOut: retrieval.AskOutput{Answer: "G", Citations: []retrieval.Citation{}, Diagnostics: map[string]any{"mode": "global"}}}
	h := askGlobalHandler(asker)
	req := httptest.NewRequest("POST", "/api/kb/k1/ask/global", strings.NewReader(`{"q":"themes?","maxCommunities":4}`))
	req.SetPathValue("id", "k1")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out["answer"] != "G" {
		t.Fatalf("answer=%v", out["answer"])
	}
}

func TestListCommunitiesHandler(t *testing.T) {
	cr := &fakeCommunityReader{communities: []ragsvc.CommunityView{{ID: "c1", Level: 0}, {ID: "c2", Level: 1}}}
	h := listCommunitiesHandler(staticKBGetter{ns: "kb_k1"}, cr)
	req := httptest.NewRequest("GET", "/api/kb/k1/communities", nil)
	req.SetPathValue("id", "k1")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code=%d", rec.Code)
	}
	var out struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Items) != 2 {
		t.Fatalf("items=%d want 2", len(out.Items))
	}
}

func TestCommunityReportHandler(t *testing.T) {
	cr := &fakeCommunityReader{report: ragsvc.CommunityReportView{ID: "c1", Title: "Theme", Summary: "S"}, reportOK: true}
	h := communityReportHandler(staticKBGetter{ns: "kb_k1"}, cr)
	req := httptest.NewRequest("GET", "/api/kb/k1/communities/c1", nil)
	req.SetPathValue("id", "k1")
	req.SetPathValue("cid", "c1")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code=%d", rec.Code)
	}
	// missing report → 404
	cr.reportOK = false
	rec2 := httptest.NewRecorder()
	h(rec2, req)
	if rec2.Code != 404 {
		t.Fatalf("missing report code=%d want 404", rec2.Code)
	}
}
```

Add fakes + a `staticKBGetter{ns string}` returning `orgkb.KB{ID:"k1", Namespace: ns}` from `Get`. The fakes use kb-local DTOs (no `rag/graph` import in `httpapi`/its tests):

```go
type fakeAsker struct {
	out       retrieval.AskOutput
	globalOut retrieval.AskOutput
	driftOut  retrieval.AskOutput
}

func (f *fakeAsker) Ask(context.Context, retrieval.AskInput) (retrieval.AskOutput, error) { return f.out, nil }
func (f *fakeAsker) AskGlobal(context.Context, retrieval.GlobalInput) (retrieval.AskOutput, error) { return f.globalOut, nil }
func (f *fakeAsker) AskDrift(context.Context, retrieval.DriftInput) (retrieval.AskOutput, error) { return f.driftOut, nil }

type fakeCommunityReader struct {
	communities []ragsvc.CommunityView
	report      ragsvc.CommunityReportView
	reportOK    bool
}

func (f *fakeCommunityReader) ListCommunities(context.Context, string) ([]ragsvc.CommunityView, error) { return f.communities, nil }
func (f *fakeCommunityReader) CommunityReport(context.Context, string, string) (ragsvc.CommunityReportView, bool, error) { return f.report, f.reportOK, nil }
```

**Pre-existing fake to update (compile-break audit):** `internal/httpapi/httpapi_test.go` already defines `fakeAsk struct{ out retrieval.AskOutput }` with ONLY an `Ask` method, used by the M1/M2 ask tests (`NewMux(Deps{... Asker: fakeAsk{...}})`). Widening `Asker` with `AskGlobal`/`AskDrift` (3b) BREAKS this fake's interface satisfaction → the whole `httpapi` test package fails to compile. Add the two methods to the existing `fakeAsk` (return a zero `retrieval.AskOutput`, or reuse `out`):

```go
func (f fakeAsk) AskGlobal(context.Context, retrieval.GlobalInput) (retrieval.AskOutput, error) { return f.out, nil }
func (f fakeAsk) AskDrift(context.Context, retrieval.DriftInput) (retrieval.AskOutput, error)  { return f.out, nil }
```

Add the test imports `"github.com/costa92/llm-agent-kb/internal/ragsvc"` (for the DTO types) — but **NOT** `rag/graph`.

- [ ] **Step 2: Run — fails**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/httpapi/ -run 'TestAskGlobalHandler|TestListCommunities|TestCommunityReport'`
Expected: compile error — `askGlobalHandler undefined` etc.

- [ ] **Step 3: Minimal impl**

**3a. `internal/httpapi/httpapi.go`** — widen `Asker`, add `CommunityReader`, mount routes.

```go
type Asker interface {
	Ask(ctx context.Context, in retrieval.AskInput) (retrieval.AskOutput, error)
	AskGlobal(ctx context.Context, in retrieval.GlobalInput) (retrieval.AskOutput, error)
	AskDrift(ctx context.Context, in retrieval.DriftInput) (retrieval.AskOutput, error)
}

// CommunityReader reads the GraphRAG community views (satisfied by
// *ragsvc.Service). Returns kb-local DTOs, NOT rag/graph types — so httpapi
// never imports rag/graph (spec §4: ragsvc is the sole rag/graph importer).
type CommunityReader interface {
	ListCommunities(ctx context.Context, namespace string) ([]ragsvc.CommunityView, error)
	CommunityReport(ctx context.Context, namespace, communityID string) (ragsvc.CommunityReportView, bool, error)
}
```

Add `Community CommunityReader` to `Deps`. Add import `"github.com/costa92/llm-agent-kb/internal/ragsvc"` (for the DTO types). **Do NOT import `rag/graph`** into httpapi.

In `NewMux`, after the existing `POST /api/kb/{id}/ask` route, add (guarded by the same `chain`):

```go
	mux.Handle("POST /api/kb/{id}/ask/global", chain(authzrole.RoleViewer, askGlobalHandler(d.Asker)))
	mux.Handle("POST /api/kb/{id}/ask/drift", chain(authzrole.RoleViewer, askDriftHandler(d.Asker)))
	if d.Community != nil && d.KBRepo != nil {
		mux.Handle("GET /api/kb/{id}/communities", chain(authzrole.RoleViewer, listCommunitiesHandler(d.KBRepo, d.Community)))
		mux.Handle("GET /api/kb/{id}/communities/{cid}", chain(authzrole.RoleViewer, communityReportHandler(d.KBRepo, d.Community)))
	}
```

Inside the existing `if d.Ingester != nil && d.KBRepo != nil {` block (next to the documents routes), add the single-doc GET:

```go
		mux.Handle("GET /api/kb/{id}/documents/{docId}", chain(authzrole.RoleViewer, getDocHandler(d.KBRepo, d.DocStatus)))
```

**3b. `internal/httpapi/httpapi.go`** — the ask/global + ask/drift handlers (next to `askHandler`). Namespace = `"kb_"+id` (same convention as `askHandler`):

```go
func askGlobalHandler(asker Asker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Q              string `json:"q"`
			MaxCommunities int    `json:"maxCommunities"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		out, err := asker.AskGlobal(r.Context(), retrieval.GlobalInput{
			Namespace:      "kb_" + r.PathValue("id"),
			Question:       req.Q,
			MaxCommunities: req.MaxCommunities,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func askDriftHandler(asker Asker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Q              string `json:"q"`
			MaxCommunities int    `json:"maxCommunities"`
			Rounds         int    `json:"rounds"`
			TopK           int    `json:"topK"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		out, err := asker.AskDrift(r.Context(), retrieval.DriftInput{
			Namespace:      "kb_" + r.PathValue("id"),
			Question:       req.Q,
			MaxCommunities: req.MaxCommunities,
			Rounds:         req.Rounds,
			TopK:           req.TopK,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}
```

**3c. `internal/httpapi/handlers.go`** — community + single-doc handlers:

```go
// listCommunitiesHandler returns the kb namespace's community set (viewer+).
func listCommunitiesHandler(repo kbGetter, cr CommunityReader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		kb, err := repo.Get(r.Context(), r.PathValue("id"))
		if errors.Is(err, orgkb.ErrNotFound) {
			http.Error(w, "kb not found", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		comms, err := cr.ListCommunities(r.Context(), kb.Namespace)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		items := make([]map[string]any, 0, len(comms))
		for _, c := range comms {
			items = append(items, map[string]any{
				"id": c.ID, "level": c.Level, "parentId": c.ParentID,
				"entityCount": c.EntityCount,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

// communityReportHandler returns one community's report (viewer+); 404 on a
// community with no persisted report yet (cache miss is not an error).
func communityReportHandler(repo kbGetter, cr CommunityReader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		kb, err := repo.Get(r.Context(), r.PathValue("id"))
		if errors.Is(err, orgkb.ErrNotFound) {
			http.Error(w, "kb not found", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		rep, ok, err := cr.CommunityReport(r.Context(), kb.Namespace, r.PathValue("cid"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "community report not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"communityId": rep.ID, "title": rep.Title, "summary": rep.Summary,
		})
	}
}

// getDocHandler returns one document's detail (viewer+; folded M2 follow-up,
// §16.2 GET /documents/{docId}). Reuses the existing DocumentStatus read.
func getDocHandler(repo kbGetter, reader DocStatusReader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		kb, err := repo.Get(r.Context(), r.PathValue("id"))
		if errors.Is(err, orgkb.ErrNotFound) {
			http.Error(w, "kb not found", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		status, phase, cc, errMsg, err := reader.DocumentStatus(r.Context(), kb.ID, r.PathValue("docId"))
		if err != nil {
			http.Error(w, "document not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id": r.PathValue("docId"), "status": status, "phase": phase,
			"chunkCount": cc, "error": errMsg,
		})
	}
}
```

(`getDocHandler` reuses the existing `DocStatusReader` interface — no new ingest read needed; `kb.ID` is the document's `kb_id`.)

- [ ] **Step 4: Run — passes**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/httpapi/`
Expected: `ok  	github.com/costa92/llm-agent-kb/internal/httpapi	0.0XXs`.

- [ ] **Step 5: Commit**

```bash
cd llm-agent-kb && git add internal/httpapi/httpapi.go internal/httpapi/handlers.go internal/httpapi/community_test.go internal/httpapi/ask_test.go && \
git commit -m "httpapi: POST /ask/global + /ask/drift (viewer+) + GET /communities[/{cid}] community views + GET /documents/{docId} (folded M2 follow-up)"
```

---

## Task 9 — cmd/kbd: wire LLM-backed graph components into ragsvc.Deps + new httpapi Deps

Production assembly: build `graph.LLMEntityExtractor` + `graph.LouvainDetector` + `graph.LLMCommunitySummarizer` (reusing the same chat Model via the ragsvc adapters) and pass them into `ragsvc.Deps`; pass the optional `EmbeddingEntityResolver` when `cfg.EntityResolverEnabled`. Pass the retrieval Config's new defaults + the `Community` reader to `httpapi.Deps`.

> **Boundary:** cmd/kbd must NOT import `rag/graph` directly (that would make a second importer of a rag sub-package outside ragsvc). Instead, expose the LLM-backed components via ragsvc helper constructors so the graph types stay encapsulated in ragsvc. Add to `ragsvc`:
> ```go
> // NewLLMGraphComponents builds the production GraphRAG components over the
> // given chat model + embedder, honoring the resolver toggle. cmd/kbd calls
> // this so it never imports rag/graph directly (boundary: ragsvc is the sole
> // importer of rag/*).
> func NewLLMGraphComponents(model llm.ChatModel, embedder llm.Embedder, opts GraphConfig) GraphComponents {
> 	gc := GraphComponents{
> 		EntityExtractor:     raggraph.LLMEntityExtractor{Model: ragModelAdapter{inner: model}},
> 		CommunityDetector:   raggraph.LouvainDetector{Resolution: opts.LouvainResolution},
> 		CommunitySummarizer: raggraph.LLMCommunitySummarizer{Model: ragModelAdapter{inner: model}},
> 	}
> 	if opts.ResolverEnabled {
> 		gc.EntityResolver = raggraph.EmbeddingEntityResolver{
> 			Embedder:  ragEmbedderAdapter{inner: embedder},
> 			Threshold: opts.ResolverThreshold,
> 		}
> 	}
> 	return gc
> }
>
> type GraphConfig struct {
> 	LouvainResolution float64
> 	ResolverEnabled   bool
> 	ResolverThreshold float64
> }
>
> type GraphComponents struct {
> 	EntityExtractor     raggraph.EntityExtractor
> 	EntityResolver      raggraph.EntityResolver
> 	CommunityDetector   raggraph.CommunityDetector
> 	CommunitySummarizer raggraph.CommunitySummarizer
> }
> ```

- [ ] **Step 1: Failing test** — `internal/ragsvc/ragsvc_test.go` (append; pure unit, no DB)

```go
func TestNewLLMGraphComponentsHonorsResolverToggle(t *testing.T) {
	model := llm.NewScriptedLLM()
	embedder := llm.NewScriptedLLM(llm.WithEmbedDimensions(8))
	off := NewLLMGraphComponents(model, embedder, GraphConfig{LouvainResolution: 1.0, ResolverEnabled: false})
	if off.EntityExtractor == nil || off.CommunityDetector == nil || off.CommunitySummarizer == nil {
		t.Fatal("core components must be non-nil")
	}
	if off.EntityResolver != nil {
		t.Fatal("resolver disabled → EntityResolver must be nil (rag defaults to Noop)")
	}
	on := NewLLMGraphComponents(model, embedder, GraphConfig{LouvainResolution: 1.5, ResolverEnabled: true, ResolverThreshold: 0.9})
	if on.EntityResolver == nil {
		t.Fatal("resolver enabled → EntityResolver must be set")
	}
}
```

- [ ] **Step 2: Run — fails**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/ragsvc/ -run TestNewLLMGraphComponents`
Expected: compile error — `NewLLMGraphComponents undefined`.

- [ ] **Step 3: Minimal impl**

**3a.** Add `NewLLMGraphComponents` + `GraphConfig` + `GraphComponents` to `internal/ragsvc/ragsvc.go` (as above).

**3b. `cmd/kbd/main.go`** — in `build`, replace the `rag := ragsvc.New(...)` block:

```go
	var graphComps ragsvc.GraphComponents
	if cfg.GraphEnabled {
		graphComps = ragsvc.NewLLMGraphComponents(model, embedder, ragsvc.GraphConfig{
			LouvainResolution: cfg.LouvainResolution,
			ResolverEnabled:   cfg.EntityResolverEnabled,
			ResolverThreshold: cfg.EntityResolverThreshold,
		})
	}
	rag := ragsvc.New(ragsvc.Deps{
		Model: model, Embedder: embedder,
		RagStore: st.RagStore(), ChunkStore: st.RagStore(),
		Tracer:              tp,
		EntityExtractor:     graphComps.EntityExtractor,
		EntityResolver:      graphComps.EntityResolver,
		CommunityDetector:   graphComps.CommunityDetector,
		CommunitySummarizer: graphComps.CommunitySummarizer,
	})
```

Update the `retrieval.New` call to pass the new defaults:

```go
	retrievalSvc := retrieval.New(rag, retrieval.Config{
		MaxAskTokens:         cfg.MaxAskTokens,
		GlobalMaxCommunities: cfg.GlobalMaxCommunities,
		DriftRounds:          cfg.DriftRounds,
		DriftTopK:            cfg.DriftTopK,
	})
```

Add `Community: rag,` to the `httpapi.NewMux(httpapi.Deps{...})` literal (`*ragsvc.Service` satisfies `CommunityReader`).

- [ ] **Step 4: Run — passes + full build/vet**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/ragsvc/ -run TestNewLLMGraphComponents && GOWORK=off go build ./... && GOWORK=off go vet ./... && echo OK`
Expected: `ok  	.../ragsvc ...` then `OK`.

- [ ] **Step 5: Commit**

```bash
cd llm-agent-kb && git add internal/ragsvc/ragsvc.go internal/ragsvc/ragsvc_test.go cmd/kbd/main.go && \
git commit -m "cmd/kbd: wire LLM-backed graph components (Louvain/extractor/summarizer + opt-in resolver) into ragsvc.Deps via NewLLMGraphComponents — keeps ragsvc the sole rag/graph importer"
```

---

## Task 10 — gated e2e: ingest → communities built → AskGlobal → AskDrift over the full server

End-to-end through `cmd/kbd.build` with `providerOverride` injecting a deterministic graph setup. Because the production path uses `LLMEntityExtractor` (cursor-coupled), the e2e overrides the provider with a scripted Model AND sets `GRAPH_ENABLED` so detection runs — but to keep the cursor deterministic, the e2e drives a SMALL corpus and asserts the structural outcome (communities exist, AskGlobal returns 200 with a non-empty answer, AskDrift returns 200). Where the scripted-LLM extraction cursor is hard to bound, the e2e instead imports via the gated ragsvc path with deterministic components (Task 4 already proves the engine); this e2e proves the HTTP wiring + RBAC + namespace.

> **Decision:** the e2e proves the **HTTP+auth+routing** layer end-to-end, not the LLM extraction quality. It uses `providerOverride` to inject a scripted Model with enough responses for: nothing during ingest if we set `GRAPH_ENABLED=false` for the upload step... — but that would skip detection. Cleaner: keep `GRAPH_ENABLED=true`, upload ONE tiny paste doc, and accept that `LLMEntityExtractor` issues exactly one `Generate` per chunk (one chunk for a tiny doc → one extraction call), Louvain needs zero LLM, summarization is lazy (prewarm: one `Generate` per community), then AskGlobal issues map(one per community)+reduce. Script a comfortably large response list (e.g. 12 `TextResponse`s, all returning parseable pipe-lines for extraction and plain text for map/reduce). Assert: `POST /documents` 202 → poll `GET /documents/{id}` until `ready` → `GET /communities` returns ≥0 items (≥1 if extraction produced entities) → `POST /ask/global` 200 non-empty → `POST /ask/drift` 200. Tolerate zero communities for a degenerate single-entity doc by asserting only the 200s + the ready status; the engine-level community assertion lives in Task 4.

- [ ] **Step 1: Failing test** — `cmd/kbd/main_test.go` (append; gated)

```go
func TestGraphRAGEndToEnd(t *testing.T) {
	dsn := os.Getenv("LLM_AGENT_KB_PG_URL")
	if dsn == "" {
		t.Skipf("set LLM_AGENT_KB_PG_URL to run the GraphRAG e2e")
	}
	// Fresh DB: drop kb + rag tables (reuse the M1/M2 e2e teardown helper).
	dropAllTables(t, dsn) // existing helper from M1/M2 main_test.go

	// Scripted model: extraction pipe-lines for ingest + map/reduce text for
	// AskGlobal + primer/local/synthesis text for AskDrift. Generous list so
	// the cursor never exhausts for a tiny corpus.
	scripted := llm.NewScriptedLLM(
		llm.WithEmbedDimensions(8),
		llm.WithResponses(
			llm.TextResponse("ENTITY | Acme | org | a company\nENTITY | Paris | city | a place\nRELATION | Acme | Paris | located_in | hq"),
			llm.TextResponse("Theme: Acme\nReport about Acme and Paris."), // summary
			llm.TextResponse("Score: 80\nmap result"),                       // global map (Score: prefix required, else score-0 → ReduceCalls=0 → "no relevant community information")
			llm.TextResponse("GLOBAL: Acme is in Paris."),                   // global reduce
			llm.TextResponse("primer result"),                               // drift primer
			llm.TextResponse("DRIFT: details about Acme."),                  // drift synthesis
			llm.TextResponse("extra"), llm.TextResponse("extra2"),
			llm.TextResponse("extra3"), llm.TextResponse("extra4"),
		),
	)
	providerOverride = func(config.Config) (llm.ChatModel, llm.Embedder, error) {
		return scripted, scripted, nil
	}
	t.Cleanup(func() { providerOverride = nil })

	cfg := testConfig(t, dsn) // existing helper; sets EmbeddingDim=8, GRAPH_ENABLED on by default
	app, cleanup, err := build(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	srv := httptest.NewServer(app)
	t.Cleanup(srv.Close)

	// login → org → kb → upload paste → poll ready (reuse M1/M2 e2e do/login helpers)
	tok := registerAndLogin(t, srv) // existing helper
	orgID := createOrg(t, srv, tok, "Org")
	kbID := createKB(t, srv, tok, orgID, "KB")
	docID := uploadPaste(t, srv, tok, kbID, "Acme is a company headquartered in Paris. Acme operates in Paris.")
	waitReady(t, srv, tok, kbID, docID) // polls GET /documents/{id} until status==ready

	// GET /communities (≥0; tolerate degenerate)
	doGet(t, srv, tok, "/api/kb/"+kbID+"/communities", 200)

	// POST /ask/global → 200, non-empty answer
	gBody := doPost(t, srv, tok, "/api/kb/"+kbID+"/ask/global", `{"q":"where is Acme?","maxCommunities":8}`, 200)
	if !strings.Contains(gBody, `"answer"`) || strings.Contains(gBody, `"answer":""`) {
		t.Fatalf("ask/global returned empty answer: %s", gBody)
	}

	// POST /ask/drift → 200
	doPost(t, srv, tok, "/api/kb/"+kbID+"/ask/drift", `{"q":"tell me about Acme","rounds":1,"topK":3}`, 200)
}
```

> Reuse the M1/M2 `main_test.go` helpers (`dropAllTables`, `testConfig`, `registerAndLogin`, `createOrg`, `createKB`, `uploadPaste`, `waitReady`, `doGet`, `doPost`). If a helper does not exist under that exact name, adapt to the M1 e2e's `do`-closure pattern (the M1 plan Task 11 smoke test). The e2e is deterministic because the worker drains synchronously under the test's poll loop and the scripted cursor is over-provisioned.

- [ ] **Step 2: Run — fails (skips without DB)**

Run: `cd llm-agent-kb && GOWORK=off go test ./cmd/kbd/ -run TestGraphRAGEndToEnd`
Expected: `--- SKIP` then `ok` (no DB); with DB, fails until all prior tasks are wired (they are) — then it is the integration assertion.

- [ ] **Step 3: (no new production code — exercises Tasks 1–9)**

- [ ] **Step 4: Run — passes (gated)**

Run: `cd llm-agent-kb && LLM_AGENT_KB_PG_URL="$LLM_AGENT_KB_PG_URL" GOWORK=off go test ./cmd/kbd/ -run TestGraphRAGEndToEnd -v`
Expected: `--- PASS: TestGraphRAGEndToEnd`.

- [ ] **Step 5: Commit**

```bash
cd llm-agent-kb && git add cmd/kbd/main_test.go && \
git commit -m "cmd/kbd: gated GraphRAG e2e — upload → ready → communities view → ask/global non-empty → ask/drift, full RBAC+namespace path"
```

---

## Task 11 — final verification + tag v0.3.0

- [ ] **Step 1: Full build + vet + unit suite**

Run: `cd llm-agent-kb && GOWORK=off go build ./... && GOWORK=off go vet ./... && GOWORK=off go test ./... && echo ALL_GREEN`
Expected: all packages `ok` (gated DB suites SKIP without `LLM_AGENT_KB_PG_URL`), final line `ALL_GREEN`.

- [ ] **Step 2: Full gated suite against live pgvector**

```bash
docker run -d --name kb_m3_pg -e POSTGRES_PASSWORD=pw postgres:16-alpine
docker exec -u root kb_m3_pg sh -c 'apk add --no-cache build-base clang19 llvm19-dev git && cd /tmp && git clone --depth 1 --branch v0.8.0 https://github.com/pgvector/pgvector && cd pgvector && make OPTFLAGS="" install'
IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' kb_m3_pg)
cd llm-agent-kb && LLM_AGENT_KB_PG_URL="postgres://postgres:pw@$IP:5432/postgres?sslmode=disable" GOWORK=off go test ./... -count=1
docker rm -f kb_m3_pg
```
Expected: all suites `ok` including `TestGraphRAGLivePgvector`, `TestWorkerPrewarmsAfterImport`, `TestDeleteRecomputesCommunities`, `TestGraphRAGEndToEnd`.
> Each gated test owns its DB by dropping its tables at the top — run with `-count=1` to defeat cache; they are NOT co-runnable in parallel on one DB (run package-by-package if a cross-test table clash appears: `go test ./internal/ragsvc/`, `./internal/ingest/`, `./cmd/kbd/` sequentially).

- [ ] **Step 3: Tag v0.3.0**

```bash
cd llm-agent-kb && git checkout main && git merge --no-ff feat/m3-graphrag -m "M3 GraphRAG: AskGlobal/AskDrift + community views + prewarm + recompute-on-delete" && \
git tag -a v0.3.0 -m "llm-agent-kb v0.3.0 — M3 GraphRAG (entity graph + Louvain communities + AskGlobal/AskDrift + community views + prewarm + §16.4 recompute)" && \
git push origin main --tags
```
Expected: branch merged, `v0.3.0` tag pushed.
> If the project convention is PR-based (M2 used a feature branch + PR), open a PR instead of a local merge: `gh pr create --base main --head feat/m3-graphrag`. Note `gh pr edit/view` may fail (token lacks `read:org`); edit via `gh api -X PATCH repos/costa92/llm-agent-kb/pulls/N`. Tag after merge.

---

## Self-Review

**Spec coverage (§13 M3 = "EntityExtractor/Louvain/Summarizer 接线 + AskGlobal/AskDrift + 社区视图 + prewarm"):**
- **EntityExtractor/Louvain/Summarizer wired** → Task 3 widens `ragsvc.Deps` with the four graph seams and passes them into `ragcore.New(Options{EntityExtractor, EntityResolver, CommunityDetector, CommunitySummarizer})`; Task 9 builds the production LLM-backed components (`graph.LLMEntityExtractor{Model: ragModelAdapter{model}}`, `graph.LouvainDetector{Resolution}`, `graph.LLMCommunitySummarizer{Model}`, opt-in `graph.EmbeddingEntityResolver{Embedder: ragEmbedderAdapter{embedder}, Threshold}`) via `ragsvc.NewLLMGraphComponents` so cmd/kbd never imports `rag/graph` (boundary preserved). Import auto-builds graph + communities (verified `rag/import.go:201`). ✓
- **AskGlobal/AskDrift** → Task 2 adds them to `RagPort`/`Service`, delegating to `Wrapper.Inner().AskGlobal/AskDrift` with kb-self-instrumented spans (`*otelrag.Wrapper` has no GraphRAG spans, §11; `Inner()` confirmed `otelrag.go:96`); Task 5 adds the `retrieval.AskGlobal/AskDrift` use-cases mapping `Diagnostics.Global`/`Diagnostics.Drift`; Task 8 adds `POST /ask/global` + `POST /ask/drift` (viewer+). ✓
- **社区视图 (community views)** → Task 2 adds `ListCommunities`/`CommunityReport` (delegating to the held `*postgres.Store`, a `store.CommunityStore`, then mapping `raggraph.Community`/`CommunityReport` → kb-local `ragsvc.CommunityView`/`CommunityReportView` DTOs so importers never see `rag/graph`); Task 8 adds `GET /api/kb/{id}/communities` (list) + `GET /api/kb/{id}/communities/{cid}` (report, 404 on cache miss) (viewer+). ✓
- **prewarm** → Task 6: `worker.process` calls `PrewarmCommunityReports(ctx, namespace)` after the success tx commits (best-effort, logged). ✓
- **§16.4 community recompute on delete** → Task 7: `RecomputeCommunities` (snapshot → `LouvainDetector.Detect` → `UpsertCommunities` replace-all → prewarm) runs after `DeleteDocument` and ONCE after the `DeleteAllDocumentsForKB` cascade (NOT per-document); M2 cascade order (List→RemoveGraphBySource→RemoveChunks→delete row) untouched, recompute appended as step 5. ✓
- **Folded M2 follow-up** → Task 8 adds `GET /api/kb/{id}/documents/{docId}` (single-doc, reuses `DocumentStatus`). Other M2 follow-ups (URL quota, retry wording, otel ServiceName) noted, not done — none were trivial/natural within M3 scope. ✓

**Global/drift namespace-only isolation (§8) — explicitly honored:** `GlobalOptions`/`DriftOptions` carry only `Namespace` (+ graph params + `MaxTotalTokens`), NO `SecurityFilters` (verified `rag/options.go:196,221`). `GlobalRequest`/`DriftRequest` and the retrieval inputs deliberately expose no filter knob; isolation is the `Namespace="kb_"+id` set by the handler AFTER the RBAC `chain(RoleViewer, ...)` passes. Documented in `GlobalRequest`/`GlobalInput` doc comments. ✓

**New graph/community APIs verified against live source (rag v1.11.0 working tree, NOT assumed):** `AskGlobal`/`AskDrift`/`PrewarmCommunityReports` signatures (`rag/global.go:62,247`, `rag/drift.go:75`); `GlobalOptions`/`DriftOptions` fields (`rag/options.go:196,221`); `rag.Answer` populates `Text`+`Diagnostics.Global`/`.Drift` and leaves `Citations` EMPTY on global/drift (`rag/global.go:215` — handled: `AskOutput.Citations: []Citation{}`); `GlobalDiagnostics`/`DriftDiagnostics` fields (`rag/system.go:219,197`); `rag.Options` graph fields + nil-resolver→Noop default (`rag/options.go:255-286`, `rag/system.go:411`); Import detection trigger keys on detector+CommunityStore (`rag/import.go:201`); graph components are **struct literals** (`graph.LLMEntityExtractor{Model}`, `graph.LouvainDetector{Resolution}`, `graph.LLMCommunitySummarizer{Model}`, `graph.EmbeddingEntityResolver{Embedder,Threshold}` — no `New*` funcs; `graph/extract.go:27`, `louvain.go:20`, `summary.go:77`, `resolve.go:62`); `(*postgres.Store)` CommunityStore methods (`postgres/community.go:26-119`); `(*otelrag.Wrapper).Inner()` (`otelrag/otelrag.go:96`). ✓

**Scripted-LLM GraphRAG test design (the trickiest question) — resolved explicitly:** `ScriptedLLM.Generate` consumes a single mutex-protected cursor and returns `ErrScriptExhausted` when depleted (contract `scripted.go:67`), so LLM-backed extraction makes the required response count depend on chunk/community cardinality — brittle. **Resolution:** engine-level gated tests (Tasks 3, 4, 7) use the SAME deterministic substitutes rag's own tests use — `graph.DictionaryEntityExtractor` (zero-LLM, `dictionary.go:14`) + `graph.LouvainDetector` (zero-LLM, deterministic) + a hand-written `fixedSummarizer` (mirrors rag's `staticSummarizer`, `global_test.go:54`). The scripted Model is then touched ONLY by AskGlobal's map(1/community)+reduce(1) (`global.go:139,188`), which is bounded and over-provisioned. The HTTP e2e (Task 10) keeps the production `LLMEntityExtractor` but over-provisions the script and asserts only structural outcomes (ready status + 200 + non-empty global answer), tolerating degenerate community counts — engine correctness is proved by Task 4. ✓

**Wrapper.Inner() delegation (§4/§11):** `Ask`/`Import`/`Retrieve` keep going through `*otelrag.Wrapper` (auto-span); `AskGlobal`/`AskDrift`/`PrewarmCommunityReports`/`RecomputeCommunities` go through `Wrapper.Inner()` (`*rag.System`) wrapped in kb-self-instrumented spans from the held `trace.Tracer` (nil TracerProvider → `noop.NewTracerProvider`). Community-view reads bypass rag entirely and hit the held `*postgres.Store`. ✓

**Boundaries:** `ragsvc` is the SOLE importer of `rag`/`postgres`/`otelrag`/`rag/graph` (spec §4) — **fully held, no relaxation**. The community-view reads return **kb-local DTOs** `ragsvc.CommunityView{ID, Level, ParentID, EntityCount}` and `ragsvc.CommunityReportView{ID, Title, Summary}` (mapped from `raggraph.Community`/`raggraph.CommunityReport` inside `ragsvc.ListCommunities`/`CommunityReport`), so `RagPort`, the httpapi `CommunityReader`, the retrieval/httpapi test fakes, and cmd/kbd type only against `ragsvc` DTOs — none import `rag/graph`. cmd/kbd builds the production graph components via `ragsvc.NewLLMGraphComponents`/`GraphComponents` (graph types stay encapsulated). `retrieval`/`ingest`/`httpapi` depend only on `ragsvc` (`RagPort` + DTOs). No import cycles. ✓

**Gating:** all DB-touching suites use `LLM_AGENT_KB_PG_URL`+`t.Skipf`; each gated test drops its rag tables (`chunks`, `chunks_entities`, `chunks_relations`, `chunks_communities`, `chunks_community_reports`) + kb tables and migrates fresh — not co-runnable on a shared DB. Pure suites (config / ragsvc-unit / retrieval / httpapi / ingest-delete-fakes) run always. Live DB is pgvector built from source into `postgres:16-alpine` per constraints (Task 11). ✓

**No version bump / no new deps:** rag v1.11.0 already ships all graph/global/drift/community APIs (verified on disk == tag); go.mod unchanged. Only a possible `go.sum` line for `rag/graph` → targeted `go get …/graph@v1.11.0`, never `tidy`. ✓

**Placeholder scan:** every code step shows actual, compiling code with real rag/graph/postgres/otelrag signatures. Test harness reuse (Task 6/7/10) references the EXISTING M1/M2 `worker_test.go`/`delete_test.go`/`main_test.go` helpers by name with an explicit fallback to replicate minimal setup if a helper name differs — no invented stubs. The only deliberately-described-not-fully-spelled portions are the gated tests' fresh-DB boilerplate (drop+migrate+seed), which exactly mirrors the M1/M2 gated pattern shown in `ragsvc_test.go:55-100`. No TBDs. ✓

**Ambiguities resolved:**
1. **Recompute trigger (§16.4 "background task")** — kept synchronous inside the delete use-case (cheap single Louvain pass, testable, idempotent replace-all); async-to-worker is a documented deferred optimization.
2. **cmd/kbd boundary** — graph components built via `ragsvc.NewLLMGraphComponents` so cmd never imports `rag/graph`.
3. **Global/drift citations** — empty by rag design; `AskOutput.Citations: []Citation{}` (not nil) so JSON renders `[]`.
4. **httpapi/retrieval community reads** — **resolved via kb-local DTOs**: `RagPort`/`CommunityReader` return `ragsvc.CommunityView`/`ragsvc.CommunityReportView` (mapped from `raggraph` types inside ragsvc), so §4 holds strictly — `httpapi`/`retrieval`/`cmd-kbd` never import `rag/graph`. (Earlier draft typed these on `raggraph.Community`/`CommunityReport`, which would have made httpapi a second `rag/graph` importer — that relaxation is now eliminated.)
5. **No standalone rag "re-detect" method** — recompute open-codes snapshot→Detect→UpsertCommunities→Prewarm in ragsvc (all confirmed exported), rather than relying on empty-`Import` semantics.
