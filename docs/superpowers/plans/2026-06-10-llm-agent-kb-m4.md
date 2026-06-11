# llm-agent-kb M4 (Quality: eval/drift + sessions + quota hardening + E2E) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the shipped `llm-agent-kb` M1+M2+M3 backend (module published at **v0.3.0**) with the §13 **M4** scope: a kb-level eval use case wrapping rag's `RetrievalEvaluator`/`TriadEvaluator`/`GlobalEvaluator`/`DriftEvaluator` + `CompareBenchmarks`, persisted eval runs + Q&A sessions, four new endpoints, an eval-run rate guard, and a gated end-to-end test that exercises ask→session→eval→drift.

**Architecture:** Same single Go binary `kbd` (BFF + embedded `rag.System`). M4 adds **two** new internal packages and grows three existing ones:
- **`internal/eval`** (NEW): the SECOND permitted importer of `rag/eval` (and `rag` option types) besides `ragsvc`. It builds rag's evaluators over thin adapters around `ragsvc.RagPort` + a `raggenerate.Model` judge, runs a `kind ∈ {retrieval,triad,global,drift}`, and returns **kb-local DTOs** (`EvalResult`, `DriftView`) so `httpapi` never sees `rag/eval` types — mirroring how M3 kept `rag/graph` behind `ragsvc.CommunityView`. For `drift`, it compares the current `eval.BenchmarkResult` against the previous stored one for the same kb via `eval.CompareBenchmarks`.
- **`internal/sessions`** (NEW): `qa_session` + `qa_message` repo (create-session-on-first-ask, append a user/assistant message pair, list sessions, read transcript).
- `ragsvc`: `RagPort` gains `Retrieve` (the retrieval evaluator needs `store.Hit`) and the `Service` exposes a `JudgeModel() raggenerate.Model` accessor (the LLM-as-judge seam) + ask-path Asker adapters; all delegated to the existing `wrapper`/`ragModelAdapter`.
- `retrieval`: the ask use-cases persist a `qa_message` pair into a session (create-on-first-ask, optional inbound `sessionId`).
- `httpapi`: `POST /api/kb/{id}/eval/run` (editor+), `GET /api/kb/{id}/eval/runs` (viewer+, paginated), `GET /api/kb/{id}/sessions` (viewer+, paginated), `GET /api/kb/{id}/sessions/{sid}` (viewer+). Adds an eval-run rate guard.
- `storage`: three new idempotent tables `eval_run`, `qa_session`, `qa_message`.
- `config`: eval-run rate knob + eval defaults.
- `cmd/kbd`: wire the eval service + session store + new routes; extend the gated e2e.

Boundaries (spec §4) preserved: `rag/eval` + `rag/*` are imported ONLY by `ragsvc` and `internal/eval`. `retrieval`/`sessions`/`httpapi`/`cmd-kbd` depend only on kb-local DTOs and `ragsvc.RagPort`. `httpapi` holds no business rules.

**Tech Stack:** Go 1.26.0 · `github.com/jackc/pgx/v5` v5.9.2 (pgxpool) · `github.com/costa92/llm-agent-authz v0.1.0` · `github.com/costa92/llm-agent-rag v1.11.0` (**`eval` sub-package — already present, no version bump**) · `github.com/costa92/llm-agent-contract v0.5.0` (`llm.NewScriptedLLM`) · `github.com/costa92/llm-agent-providers v0.7.0` · `github.com/costa92/llm-agent-otel v0.4.0` · stdlib `context`, `encoding/json`, `net/http`, `time`, `sync`. **No new external deps** (`eval` is a sub-package of the already-pinned rag module). If `go build ./internal/eval/...` adds a `go.sum` entry for the rag `eval` sub-package, run targeted `GOWORK=off go get github.com/costa92/llm-agent-rag/eval@v1.11.0` — **NEVER `go mod tidy`** (the umbrella `go.work` excludes this sibling; tidy would churn the graph).

**Spec:** `docs/superpowers/specs/2026-06-09-llm-agent-kb-design.md` (this plan implements its **M4**, §13). Scope authority: M4 list §13; data model §5 (`eval_run`, `qa_session`, `qa_message` — exact columns + `metrics_json`/`drift_json` shapes); eval execution §9 (which evaluator per kind, `LoadJSONL`, `CompareBenchmarks`); endpoints §16.2 (sessions + eval); §11 quota/limits; §16.5 PII/injection (already in M3 via rag `sanitizeHits` — M4 does NOT add new guard wiring). The M1/M2/M3 plans (`2026-06-09-llm-agent-kb-m1.md`, `-m2.md`, `-m3.md`) are the style/gating/TDD-rhythm reference.

**§4-boundary decision (load-bearing — read before Task 1):** `internal/eval` is added as a SECOND permitted importer of `rag/eval` + `rag` (for `rag.SearchOptions`/`rag.AskOptions`/`store.Hit` types the evaluators require). This is unavoidable: the evaluators are *struct literals* parameterized by rag interfaces (`eval.Retriever`, `eval.Asker`, `eval.GlobalAsker`, `eval.DriftAsker`, `eval.Judge`), so the glue that constructs them MUST see rag types. We keep §4 intact at the OUTWARD boundary: `internal/eval` exports only kb-local DTOs (`EvalResult`, `MetricsView`, `DriftView`); `httpapi`/`sessions`/`retrieval`/`cmd-kbd` never import `rag/eval`. `ragsvc` stays the sole holder of the live `rag.System`; `internal/eval` reaches rag *behavior* only through `ragsvc.RagPort` adapters + the `ragsvc.JudgeModel()` accessor (so it never constructs a second `rag.System`).

**Live base (read before editing — DO NOT reinvent M1/M2/M3):**
- `internal/ragsvc/ragsvc.go` — `RagPort` (Ask/Import/ListChunkIDs/RemoveGraphBySource/RemoveChunks/AskGlobal/AskDrift/PrewarmCommunityReports/RecomputeCommunities/ListCommunities/CommunityReport). `AskRequest{Namespace,TopK,Hybrid,MaxTotalTokens}`, `GlobalRequest{Namespace,MaxCommunities,MaxTotalTokens}`, `DriftRequest{Namespace,MaxCommunities,Rounds,TopK,MaxTotalTokens}`. `Service{wrapper *otelrag.Wrapper, chunkStore *ragpostgres.Store, tracer, detector}`. `Deps{Model llm.ChatModel, Embedder, RagStore, ChunkStore, Tracer, EntityExtractor,...}`. `New` builds `ragModelAdapter{inner:d.Model}` internally. **M4 widens `RagPort` with `Retrieve`, adds a `model llm.ChatModel` field to `Service` + a `JudgeModel()` accessor.**
- `internal/ragsvc/adapters.go` — `ragModelAdapter{inner llm.ChatModel}` → `raggenerate.Model` (satisfies `eval.LLMJudge.Model`); `ragEmbedderAdapter`. M4 reuses `ragModelAdapter` for the judge.
- `internal/retrieval/retrieval.go` — `Service{rag ragsvc.RagPort, cfg}`; `Ask`/`AskGlobal`/`AskDrift` return `AskOutput{Answer,Citations,Diagnostics}`. **M4 adds a `sessions.Recorder` dep + session persistence to all three ask paths** (and an inbound optional `SessionID`).
- `internal/httpapi/httpapi.go` + `handlers.go` — `NewMux(Deps)`; `Asker`/`CommunityReader`/`Ingester`/`kbGetter`/`OrgLookup`/`DocStatusReader` narrow interfaces; `chain(min,h)` kb-scoped RBAC shorthand; `scoped`/`authOnly`; `withUserLimit(guard,h)`; `writeJSON`; cursor envelope `{items, next_cursor}`; `askHandler` hardcodes `Namespace:"kb_"+id`. M4 adds an `EvalRunner` + `SessionReader` surface, four routes, and an eval-run guard.
- `internal/orgkb/orgkb.go` — `KB{ID,OrgID,Name,Namespace,...}`, `Get`, `ListByOrg` (keyset pagination by id, `next` = last id when `len==limit`), namespace = `"kb_"+id`.
- `internal/limits/limits.go` — `Guard{perMinute}` fixed-window per-user counter; `New(perMinute)`, `Allow(userID)`, `AllowAt(userID, now)`. perMinute<=0 → unlimited. M4 adds a SEPARATE eval-run guard instance (same type, smaller budget).
- `internal/storage/storage.go` — `businessMigrations []string` (idempotent), `Migrate` (rag `Migrate` then business). `ensureVectorExtension` runs inside `Open`. M4 appends three `CREATE TABLE IF NOT EXISTS` statements.
- `internal/config/config.go` — `Config`, `LoadFromLookup`, `envOr/envInt/envBool/envFloat`. M4 adds `MaxEvalRunsPerUserPerMinute` + `EvalDefaultTopK`.
- `cmd/kbd/main.go` — `build(ctx,cfg)` assembly; `providerOverride` test seam; `cmd/kbd/main_test.go` `do` closure + `cleanDB` + scripted LLM. M4 wires the eval + sessions deps and extends the e2e.

**Verified rag `eval` APIs (inspected on-disk == tag v1.11.0 — DO NOT change these calls):**
- `eval.Dataset{Name string; TopK int; Examples []eval.Example}` — `eval/eval.go:38`. `eval.Example{Query, Namespace, GoldDocIDs []string, GoldChunkIDs []string, Notes string}` (JSON tags `query`/`namespace`/`gold_doc_ids`/`gold_chunk_ids`/`notes`) — `eval/eval.go:29`.
- `eval.LoadJSONL(path string) (eval.Dataset, error)` — `eval/loader.go:29`. Name = file basename; TopK from first line's `top_k` field else 5.
- `eval.Metrics{PrecisionAtK, RecallAtK, MRR, GroundingAtK float64; Examples, TopK int}` — `eval/eval.go:45` (NO json tags).
- `eval.RetrievalEvaluator{Retriever eval.Retriever; Options rag.SearchOptions}` + `.Run(ctx, eval.Dataset) (eval.RetrievalResult, error)` — `eval/eval.go:85,92`. `eval.Retriever` = `Retrieve(ctx, query string, opts rag.SearchOptions) ([]store.Hit, error)`. `eval.RetrievalResult{Dataset; Metrics eval.Metrics; PerExample []eval.ExampleResult}`.
- `eval.TriadEvaluator{Asker eval.Asker; Judge eval.Judge; Options rag.AskOptions}` + `.Run(...) (eval.TriadResult, error)` — `eval/triad.go:52,60`. `eval.Asker` = `Ask(ctx, q string, opts rag.AskOptions) (rag.Answer, error)`. `eval.TriadResult{Dataset; Retrieval eval.Metrics; Generation eval.GenerationMetrics; PerExample}`. `eval.GenerationMetrics{MeanGroundedness, MeanAnswerRelevance float64; Examples int}` (json tags `mean_groundedness`/`mean_answer_relevance`/`examples`).
- `eval.GlobalEvaluator{Asker eval.GlobalAsker; Judge eval.Judge; MaxCommunities int}` + `.Run(...) (eval.GlobalEvalResult, error)` — `eval/global.go:51,63`. `eval.GlobalAsker` = `AskGlobal(ctx, q string, opts rag.GlobalOptions) (rag.Answer, error)`. `eval.GlobalEvalResult{MeanGroundedness, MeanAnswerRelevance float64; Examples int; PerExample}`.
- `eval.DriftEvaluator{Asker eval.DriftAsker; Judge eval.Judge; MaxCommunities int; Rounds int}` + `.Run(...) (eval.DriftEvalResult, error)` — `eval/drift.go:55,72`. `eval.DriftAsker` = `AskDrift(ctx, q string, opts rag.DriftOptions) (rag.Answer, error)`. `eval.DriftEvalResult{MeanGroundedness, MeanAnswerRelevance float64; Examples int; PerExample}`.
- `eval.Judge` interface = `Judge(ctx, eval.JudgeRequest) (eval.Judgement, error)` — `eval/judge.go:41`. `eval.LLMJudge{Model generate.Model}` satisfies it — `eval/judge.go:48`. `ragsvc.ragModelAdapter` already implements `generate.Model`.
- `eval.BenchmarkResult{Dataset eval.AnswerDataset; Metrics eval.BenchmarkMetrics; PerExample []eval.AnswerExampleResult}` — `eval/benchmark.go:183`. `eval.BenchmarkMetrics{Examples int; ExactMatch, F1Token, RequiredPhraseRecall, ReflectionRoundsMean float64; AdoptedRoundCounts []int; GraderAdoptionRate, FollowupQueriesUsedMean, ActiveRetrievalFireRate, MeanGroundedness, MeanAnswerRelevance float64}` (NO json tags — `math.NaN()` does not round-trip; wrap in your own wire type) — `eval/benchmark.go:139`. `eval.AnswerDataset{Name string; TopK int; Examples []eval.AnswerExample}` — `eval/benchmark.go:60`. `eval.AnswerExample{eval.Example (embedded); GoldAnswers, RequiredPhrases []string}` — `eval/benchmark.go:52`.
- `eval.AnswerBenchmark{Asker eval.Asker; Options rag.AskOptions; Judge eval.Judge; Parallelism int}` + `.Run(ctx, eval.AnswerDataset) (eval.BenchmarkResult, error)` — `eval/benchmark.go:257,324`. This is what produces the `BenchmarkResult` the drift comparator eats.
- `eval.CompareBenchmarks(prev, curr eval.BenchmarkResult) eval.DriftReport` — `eval/compare.go:196`. `eval.DriftReport{Dataset string; Deltas []eval.MetricDelta; NewExamples, DroppedExamples []string; Histograms []eval.HistogramDelta}`. `eval.MetricDelta{Name string; Prev, Curr, Delta float64; Direction eval.Direction}`. `eval.Direction` ∈ `"improved"|"regressed"|"unchanged"|"undefined"`. **`DriftReport` has NO json tags but all-float64/string fields with no `NaN` in `Direction`** — `Delta`/`Prev`/`Curr` CAN be `NaN`, so M4 serializes through a wire DTO that maps `NaN`→`null` (see Task 2).
- `rag.SearchOptions{Namespace string; TopK int; EnableRerank bool; ...}` — `rag/options.go:19`. `rag.AskOptions{Search rag.SearchOptions; MaxTotalTokens int; ...}` — `rag/options.go:165`.

**Conventions (carried from M1/M2/M3, re-verified):** Go commands need `GOWORK=off`. DB-touching tests gate on `LLM_AGENT_KB_PG_URL` + `t.Skipf`. **Each gated test sets up its OWN fresh DB** and drops the tables it owns — `DROP TABLE` (NOT `DROP SCHEMA`, which would kill the pgvector extension). The cmd/kbd e2e reuses `cleanDB` (drops business+authz+rag tables) which calls `storage.Open`→`ensureVectorExtension` on rebuild, so the `vector` extension is always present. New gated tests that touch `eval_run`/`qa_session`/`qa_message` add those table names to their own drop list. DB-free handler tests inject narrow-interface fakes. Develop on the existing branch **`feat/m4-eval`** (already checked out). The replace-guard pre-commit hook strips local `costa92` replaces — `go.mod` currently has NONE, so a clean commit is a no-op for the guard; do NOT `--no-verify` unless intentional.

**Whole-module vet rule:** any task that WIDENS an interface (`RagPort`, `Asker`) MUST update every fake/implementor in the same task and run `GOWORK=off go vet ./...` (whole module) before commit — a narrowed fake left stale is a compile break downstream.

**Gated test DB (build pgvector from source, per constraints — same as M3):**
```bash
docker run -d --name kb_m4_pg -e POSTGRES_PASSWORD=pw postgres:16-alpine
docker exec -u root kb_m4_pg sh -c 'apk add --no-cache build-base clang19 llvm19-dev git && cd /tmp && git clone --depth 1 --branch v0.8.0 https://github.com/pgvector/pgvector && cd pgvector && make OPTFLAGS="" install'
IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' kb_m4_pg)
export LLM_AGENT_KB_PG_URL="postgres://postgres:pw@$IP:5432/postgres?sslmode=disable"
```

---

## File Structure

| File | Responsibility | Action |
|---|---|---|
| `internal/eval/eval.go` | kb eval use case + kb-local DTOs (`EvalResult`, `MetricsView`, `DriftView`); wraps rag's four evaluators + `CompareBenchmarks`; the SECOND permitted `rag/eval` importer | Create |
| `internal/eval/adapters.go` | thin `eval.Retriever`/`eval.Asker`/`eval.GlobalAsker`/`eval.DriftAsker` adapters over `ragsvc.RagPort` | Create |
| `internal/eval/store.go` | `eval_run` repo: `Insert`, `ListByKB` (cursor), `LatestBenchmark` (for drift) | Create |
| `internal/eval/eval_test.go` | DB-free use-case tests with a fake RagPort + scripted judge | Create |
| `internal/eval/store_test.go` | gated `eval_run` repo test on live pgvector | Create |
| `internal/sessions/sessions.go` | `qa_session`+`qa_message` repo: `EnsureSession`, `AppendPair`, `ListByKB`, `Transcript` | Create |
| `internal/sessions/sessions_test.go` | gated session repo test on live pgvector | Create |
| `internal/ragsvc/ragsvc.go` | add `Retrieve` to `RagPort`+`Service`; add `model` field + `JudgeModel()` | Modify |
| `internal/ragsvc/ragsvc_test.go` | extend fakes for the widened `RagPort` | Modify |
| `internal/retrieval/retrieval.go` | persist a `qa_message` pair per ask; optional inbound `SessionID` | Modify |
| `internal/retrieval/retrieval_test.go` | assert session persistence with a fake recorder | Modify |
| `internal/storage/storage.go` | append `eval_run`/`qa_session`/`qa_message` migrations | Modify |
| `internal/config/config.go` | `MaxEvalRunsPerUserPerMinute` + `EvalDefaultTopK` | Modify |
| `internal/config/config_test.go` | assert the new knobs' defaults | Modify |
| `internal/httpapi/httpapi.go` | `EvalRunner`+`SessionReader` surfaces, 4 routes, eval-run guard; thread `KBID`/`UserID`/`SessionID` through the 3 ask handlers | Modify |
| `internal/httpapi/eval.go` | eval-run + eval-list + session-list + session-transcript handlers | Create |
| `internal/httpapi/eval_test.go` | DB-free handler tests with fakes | Create |
| `internal/httpapi/community_test.go` + `ask_test.go` | extend `fakeAsker` to capture + assert ask handlers thread session fields | Modify |
| `cmd/kbd/main.go` | wire eval svc + session store + new Deps | Modify |
| `cmd/kbd/main_test.go` | extend the gated e2e: ask→session→eval→drift | Modify |

---

## Task 1 — config: eval-run rate + eval default knobs

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/config/config_test.go`:

```go
func TestLoadDefaults_M4EvalKnobs(t *testing.T) {
	cfg, err := config.LoadFromLookup(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxEvalRunsPerUserPerMinute != 5 {
		t.Errorf("MaxEvalRunsPerUserPerMinute = %d, want 5", cfg.MaxEvalRunsPerUserPerMinute)
	}
	if cfg.EvalDefaultTopK != 5 {
		t.Errorf("EvalDefaultTopK = %d, want 5", cfg.EvalDefaultTopK)
	}
}

func TestLoad_M4EvalKnobsOverride(t *testing.T) {
	cfg, err := config.LoadFromLookup(func(k string) (string, bool) {
		switch k {
		case "MAX_EVAL_RUNS_PER_USER_PER_MINUTE":
			return "2", true
		case "EVAL_DEFAULT_TOP_K":
			return "8", true
		}
		return "", false
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxEvalRunsPerUserPerMinute != 2 {
		t.Errorf("MaxEvalRunsPerUserPerMinute = %d, want 2", cfg.MaxEvalRunsPerUserPerMinute)
	}
	if cfg.EvalDefaultTopK != 8 {
		t.Errorf("EvalDefaultTopK = %d, want 8", cfg.EvalDefaultTopK)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOWORK=off go test ./internal/config/ -run M4Eval -v`
Expected: FAIL — compile error `cfg.MaxEvalRunsPerUserPerMinute undefined`.

- [ ] **Step 3: Add the fields + parsing**

In `internal/config/config.go`, add to the `Config` struct (after the existing `MaxRequestsPerUserPerMinute int` line):

```go
	// M4 eval quota + defaults (§11, §13).
	MaxEvalRunsPerUserPerMinute int // per-user fixed-window cap on POST /eval/run (eval is LLM/compute-heavy)
	EvalDefaultTopK             int // default eval.Dataset.TopK when the uploaded dataset omits top_k and the request omits it
```

In `LoadFromLookup`, add to the `cfg := Config{...}` literal (after the `MaxRequestsPerUserPerMinute:` line):

```go
		MaxEvalRunsPerUserPerMinute: envInt(lookup, "MAX_EVAL_RUNS_PER_USER_PER_MINUTE", 5),
		EvalDefaultTopK:             envInt(lookup, "EVAL_DEFAULT_TOP_K", 5),
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOWORK=off go test ./internal/config/ -run M4Eval -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "config: add M4 eval-run rate + eval default-topK knobs

eval is LLM/compute-heavy so it gets its own per-user budget separate
from the ask/upload limiter (§11/§13)."
```

---

## Task 2 — internal/eval: kb-local DTOs + JSON wire mapping (no rag handles yet)

**Files:**
- Create: `internal/eval/eval.go`
- Test: `internal/eval/eval_test.go`

This task defines the DTOs and the metric→JSON projection in isolation (pure functions), so later tasks can build on a stable surface. NaN-safe JSON is load-bearing for drift.

- [ ] **Step 1: Write the failing test**

Create `internal/eval/eval_test.go`:

```go
package eval

import (
	"encoding/json"
	"math"
	"testing"

	rageval "github.com/costa92/llm-agent-rag/eval"
)

func TestRetrievalMetricsView(t *testing.T) {
	m := rageval.Metrics{PrecisionAtK: 0.5, RecallAtK: 0.25, MRR: 0.75, GroundingAtK: 1.0, Examples: 4, TopK: 5}
	v := metricsFromRetrieval(m)
	if v.PrecisionAtK != 0.5 || v.RecallAtK != 0.25 || v.MRR != 0.75 || v.GroundingAtK != 1.0 {
		t.Fatalf("unexpected view: %+v", v)
	}
	if v.Examples != 4 || v.TopK != 5 {
		t.Fatalf("examples/topk: %+v", v)
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if want := `"precisionAtK":0.5`; !contains(string(b), want) {
		t.Fatalf("json %s missing %s", b, want)
	}
}

func TestGenerationMetricsView(t *testing.T) {
	v := metricsFromGeneration(0.8, 0.9, 3)
	if v.MeanGroundedness != 0.8 || v.MeanAnswerRelevance != 0.9 || v.Examples != 3 {
		t.Fatalf("unexpected view: %+v", v)
	}
}

func TestDriftViewNaNBecomesNull(t *testing.T) {
	report := rageval.DriftReport{
		Dataset: "ds",
		Deltas: []rageval.MetricDelta{
			{Name: "MeanGroundedness", Prev: 0.5, Curr: 0.7, Delta: 0.2, Direction: rageval.DirectionImproved},
			{Name: "ExactMatch", Prev: math.NaN(), Curr: math.NaN(), Delta: math.NaN(), Direction: rageval.DirectionUndefined},
		},
		NewExamples: []string{"q2"},
		Histograms: []rageval.HistogramDelta{
			{Name: "AdoptedRoundCounts", Prev: []int{1, 0}, Curr: []int{0, 1}, Delta: []int{-1, 1}, L1Distance: 2},
		},
	}
	v := driftView(report)
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal drift view (NaN must not break json): %v", err)
	}
	s := string(b)
	if !contains(s, `"direction":"improved"`) {
		t.Fatalf("drift json missing improved direction: %s", s)
	}
	if !contains(s, `"delta":null`) {
		t.Fatalf("NaN delta must serialize as null: %s", s)
	}
	if !contains(s, `"newExamples":["q2"]`) {
		t.Fatalf("drift json missing newExamples: %s", s)
	}
	if len(v.Histograms) != 1 || v.Histograms[0].Name != "AdoptedRoundCounts" || v.Histograms[0].L1Distance != 2 {
		t.Fatalf("histogram projection missing: %+v", v.Histograms)
	}
	if !contains(s, `"histograms":[`) || !contains(s, `"l1Distance":2`) {
		t.Fatalf("drift json missing histograms projection: %s", s)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOWORK=off go test ./internal/eval/ -v`
Expected: FAIL — `internal/eval/eval.go` does not exist (no such package / undefined `metricsFromRetrieval`).

- [ ] **Step 3: Write the DTOs + projections**

Create `internal/eval/eval.go`:

```go
// Package eval is the kb-level quality use case (§9, §13 M4). It is — alongside
// ragsvc — a permitted importer of rag/eval (spec §4): the rag evaluators are
// struct literals parameterized by rag interfaces, so the glue that builds them
// must see rag types. The OUTWARD boundary is preserved: this package exports
// only kb-local DTOs (EvalResult / MetricsView / GenerationView / DriftView);
// httpapi/retrieval/sessions/cmd-kbd never import rag/eval.
package eval

import (
	"math"

	rageval "github.com/costa92/llm-agent-rag/eval"
)

// Kind enumerates the supported eval kinds (§5 eval_run.kind).
type Kind string

const (
	KindRetrieval Kind = "retrieval"
	KindTriad     Kind = "triad"
	KindGlobal    Kind = "global"
	KindDrift     Kind = "drift"
)

// MetricsView is the kb-local projection of eval.Metrics (retrieval leg).
type MetricsView struct {
	PrecisionAtK float64 `json:"precisionAtK"`
	RecallAtK    float64 `json:"recallAtK"`
	MRR          float64 `json:"mrr"`
	GroundingAtK float64 `json:"groundingAtK"`
	Examples     int     `json:"examples"`
	TopK         int     `json:"topK"`
}

// GenerationView is the kb-local projection of the generation-side legs
// (triad/global/drift share groundedness + answer relevance).
type GenerationView struct {
	MeanGroundedness    float64 `json:"meanGroundedness"`
	MeanAnswerRelevance float64 `json:"meanAnswerRelevance"`
	Examples            int     `json:"examples"`
}

// MetricDeltaView is the kb-local projection of one eval.MetricDelta. NaN
// scalars (the rag "feature off everywhere" sentinel) serialize to JSON null
// via *float64, since math.NaN() does not round-trip through encoding/json.
type MetricDeltaView struct {
	Name      string   `json:"name"`
	Prev      *float64 `json:"prev"`
	Curr      *float64 `json:"curr"`
	Delta     *float64 `json:"delta"`
	Direction string   `json:"direction"`
}

// HistogramDeltaView is the kb-local projection of one eval.HistogramDelta
// (§9 drift dashboard). All fields are JSON-friendly (no NaN): two int-bucket
// histograms + their per-bucket delta + the L1 distance.
type HistogramDeltaView struct {
	Name       string  `json:"name"`
	Prev       []int   `json:"prev"`
	Curr       []int   `json:"curr"`
	Delta      []int   `json:"delta"`
	L1Distance float64 `json:"l1Distance"`
}

// DriftView is the kb-local projection of eval.DriftReport.
type DriftView struct {
	Dataset         string               `json:"dataset"`
	Deltas          []MetricDeltaView    `json:"deltas"`
	Histograms      []HistogramDeltaView `json:"histograms"`
	NewExamples     []string             `json:"newExamples"`
	DroppedExamples []string             `json:"droppedExamples"`
}

// EvalResult is the kb-local result of one eval run. Exactly one of the metric
// views is populated per kind; Drift is set only for KindDrift.
type EvalResult struct {
	Kind        Kind            `json:"kind"`
	DatasetName string          `json:"datasetName"`
	Retrieval   *MetricsView    `json:"retrieval,omitempty"`
	Generation  *GenerationView `json:"generation,omitempty"`
	Drift       *DriftView      `json:"drift,omitempty"`
}

func metricsFromRetrieval(m rageval.Metrics) MetricsView {
	return MetricsView{
		PrecisionAtK: m.PrecisionAtK,
		RecallAtK:    m.RecallAtK,
		MRR:          m.MRR,
		GroundingAtK: m.GroundingAtK,
		Examples:     m.Examples,
		TopK:         m.TopK,
	}
}

func metricsFromGeneration(groundedness, relevance float64, examples int) GenerationView {
	return GenerationView{
		MeanGroundedness:    groundedness,
		MeanAnswerRelevance: relevance,
		Examples:            examples,
	}
}

// nullableFloat maps NaN→nil so encoding/json emits null instead of failing.
func nullableFloat(f float64) *float64 {
	if math.IsNaN(f) {
		return nil
	}
	return &f
}

func driftView(r rageval.DriftReport) DriftView {
	deltas := make([]MetricDeltaView, 0, len(r.Deltas))
	for _, d := range r.Deltas {
		deltas = append(deltas, MetricDeltaView{
			Name:      d.Name,
			Prev:      nullableFloat(d.Prev),
			Curr:      nullableFloat(d.Curr),
			Delta:     nullableFloat(d.Delta),
			Direction: string(d.Direction),
		})
	}
	hists := make([]HistogramDeltaView, 0, len(r.Histograms))
	for _, h := range r.Histograms {
		hists = append(hists, HistogramDeltaView{
			Name:       h.Name,
			Prev:       h.Prev,
			Curr:       h.Curr,
			Delta:      h.Delta,
			L1Distance: h.L1Distance,
		})
	}
	newEx := r.NewExamples
	if newEx == nil {
		newEx = []string{}
	}
	dropped := r.DroppedExamples
	if dropped == nil {
		dropped = []string{}
	}
	return DriftView{
		Dataset:         r.Dataset,
		Deltas:          deltas,
		Histograms:      hists,
		NewExamples:     newEx,
		DroppedExamples: dropped,
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOWORK=off go test ./internal/eval/ -v`
Expected: PASS (3 tests). If `go build` printed a `go.sum` line for `github.com/costa92/llm-agent-rag/eval`, run `GOWORK=off go get github.com/costa92/llm-agent-rag/eval@v1.11.0` (NOT tidy) then re-run.

- [ ] **Step 5: Commit**

```bash
git add internal/eval/eval.go internal/eval/eval_test.go
git commit -m "eval: kb-local DTOs + NaN-safe drift JSON projection

internal/eval is the second permitted rag/eval importer (§4); exports
only kb-local views so httpapi never sees rag/eval types. NaN deltas map
to JSON null because math.NaN does not round-trip encoding/json."
```

---

## Task 3 — ragsvc: widen RagPort with Retrieve + expose JudgeModel

**Files:**
- Modify: `internal/ragsvc/ragsvc.go`
- Test: `internal/ragsvc/ragsvc_test.go`

The retrieval evaluator needs `eval.Retriever` (= `Retrieve(ctx, q, rag.SearchOptions) ([]store.Hit, error)`); the triad/global/drift evaluators need a `generate.Model` judge. `internal/eval` reaches both through `ragsvc` so it never builds a second `rag.System`.

- [ ] **Step 1: Write the failing test**

Append to `internal/ragsvc/ragsvc_test.go`:

```go
func TestServiceExposesJudgeModel(t *testing.T) {
	svc := New(Deps{
		Model:    llm.NewScriptedLLM(llm.WithResponses(llm.Response{Text: `{"groundedness":1,"answer_relevance":1}`})),
		Embedder: llm.NewScriptedLLM(llm.WithEmbedDimensions(4)),
	})
	jm := svc.JudgeModel()
	if jm == nil {
		t.Fatal("JudgeModel() returned nil")
	}
	resp, err := jm.Generate(context.Background(), raggenerate.Request{
		Messages: []raggenerate.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("judge model generate: %v", err)
	}
	if resp.Text == "" {
		t.Fatal("judge model returned empty text")
	}
}
```

If `ragsvc_test.go` lacks the imports, add `"context"` and `raggenerate "github.com/costa92/llm-agent-rag/generate"` and `"github.com/costa92/llm-agent-contract/llm"` to its import block (skip any already present).

- [ ] **Step 2: Run test to verify it fails**

Run: `GOWORK=off go test ./internal/ragsvc/ -run JudgeModel -v`
Expected: FAIL — `svc.JudgeModel undefined`.

- [ ] **Step 3: Implement Retrieve + JudgeModel + model field**

In `internal/ragsvc/ragsvc.go`:

(a) Add to the `RagPort` interface (after the `Ask(...)` line):

```go
	// Retrieve runs retrieval only (no generation) — backs the kb eval
	// RetrievalEvaluator (§9). Delegated to the otelrag Wrapper (auto-span).
	Retrieve(ctx context.Context, query string, opts ragcore.SearchOptions) ([]ragstore.Hit, error)
```

(b) Add a `model` field to `Service` (after `wrapper *otelrag.Wrapper`):

```go
	model      llm.ChatModel              // kept for the eval LLM-as-judge seam (JudgeModel)
```

(c) In `New`, set it in the returned literal — change the final return to:

```go
	return &Service{wrapper: wrapper, chunkStore: d.ChunkStore, tracer: tracer, detector: d.CommunityDetector, model: d.Model}
```

(d) Add the two methods (after `Import`):

```go
// Retrieve runs retrieval only via the otelrag Wrapper (auto-instrumented).
func (s *Service) Retrieve(ctx context.Context, query string, opts ragcore.SearchOptions) ([]ragstore.Hit, error) {
	return s.wrapper.Retrieve(ctx, query, opts)
}

// JudgeModel returns the rag generate.Model seam (the chat model wrapped by
// ragModelAdapter) for the kb eval LLM-as-judge. Kept here so internal/eval
// never constructs a second rag.System or its own adapter (spec §4).
func (s *Service) JudgeModel() raggenerate.Model {
	return ragModelAdapter{inner: s.model}
}
```

(e) Add the import `raggenerate "github.com/costa92/llm-agent-rag/generate"` to `ragsvc.go`'s import block (it is currently only in `adapters.go`).

- [ ] **Step 4: Update every RagPort fake (whole-module vet rule)**

Search for stale fakes: `GOWORK=off go vet ./... 2>&1 | head`. Any type asserted as `ragsvc.RagPort` (the `retrieval` fakes in `internal/retrieval/retrieval_test.go`, the `httpapi` fakes) must gain a `Retrieve` method. Add to each fake that implements `RagPort`:

```go
func (f *fakeRag) Retrieve(ctx context.Context, query string, opts ragcore.SearchOptions) ([]ragstore.Hit, error) {
	return nil, nil
}
```

(Match the existing fake's receiver name + the rag import aliases already used in that test file. `internal/retrieval/retrieval_test.go`'s fake is the primary one; `httpapi` tests use a `retrieval.Service`-shaped fake — only fakes that satisfy `ragsvc.RagPort` need this.)

- [ ] **Step 5: Run tests + whole-module vet to verify pass**

Run: `GOWORK=off go test ./internal/ragsvc/ -run JudgeModel -v && GOWORK=off go vet ./...`
Expected: PASS; vet clean (no "does not implement RagPort" errors).

- [ ] **Step 6: Commit**

```bash
git add internal/ragsvc/ragsvc.go internal/ragsvc/ragsvc_test.go internal/retrieval/retrieval_test.go
git commit -m "ragsvc: expose Retrieve + JudgeModel for the kb eval use case

RetrievalEvaluator needs store.Hit retrieval; the triad/global/drift
evaluators need a generate.Model judge. Both reached through ragsvc so
internal/eval never builds a second rag.System (§4)."
```

---

## Task 4 — internal/eval: rag evaluator adapters over RagPort

**Files:**
- Create: `internal/eval/adapters.go`
- Test: `internal/eval/eval_test.go` (append)

These adapt `ragsvc.RagPort`'s kb-shaped requests into the rag `eval.Retriever`/`eval.Asker`/`eval.GlobalAsker`/`eval.DriftAsker` seams (which speak raw `rag.SearchOptions`/`rag.AskOptions`/`rag.GlobalOptions`/`rag.DriftOptions`).

- [ ] **Step 1: Write the failing test**

Append to `internal/eval/eval_test.go`:

```go
import additions (add to the existing import block):
  "context"
  ragcore "github.com/costa92/llm-agent-rag/rag"
  ragstore "github.com/costa92/llm-agent-rag/store"
  "github.com/costa92/llm-agent-kb/internal/ragsvc"

// fakePort implements just the RagPort methods the adapters call.
type fakePort struct {
	hits   []ragstore.Hit
	answer ragcore.Answer
}

func (f fakePort) Retrieve(ctx context.Context, q string, opts ragcore.SearchOptions) ([]ragstore.Hit, error) {
	return f.hits, nil
}
func (f fakePort) Ask(ctx context.Context, q string, req ragsvc.AskRequest) (ragcore.Answer, error) {
	return f.answer, nil
}
func (f fakePort) AskGlobal(ctx context.Context, q string, req ragsvc.GlobalRequest) (ragcore.Answer, error) {
	return f.answer, nil
}
func (f fakePort) AskDrift(ctx context.Context, q string, req ragsvc.DriftRequest) (ragcore.Answer, error) {
	return f.answer, nil
}

func TestRetrieverAdapterMapsOptions(t *testing.T) {
	port := fakePort{hits: []ragstore.Hit{{}, {}}}
	r := retrieverAdapter{port: port, namespace: "kb_x"}
	hits, err := r.Retrieve(context.Background(), "q", ragcore.SearchOptions{TopK: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 {
		t.Fatalf("hits = %d, want 2", len(hits))
	}
}

func TestAskerAdapterReturnsAnswer(t *testing.T) {
	port := fakePort{answer: ragcore.Answer{Text: "hi"}}
	a := askerAdapter{port: port, namespace: "kb_x", maxTokens: 100}
	ans, err := a.Ask(context.Background(), "q", ragcore.AskOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if ans.Text != "hi" {
		t.Fatalf("answer = %q", ans.Text)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOWORK=off go test ./internal/eval/ -run Adapter -v`
Expected: FAIL — `retrieverAdapter`/`askerAdapter` undefined.

- [ ] **Step 3: Write the adapters**

Create `internal/eval/adapters.go`:

```go
package eval

import (
	"context"

	ragcore "github.com/costa92/llm-agent-rag/rag"
	ragstore "github.com/costa92/llm-agent-rag/store"

	"github.com/costa92/llm-agent-kb/internal/ragsvc"
)

// Port is the slice of ragsvc.RagPort the eval adapters call. Narrowing keeps
// the eval use case testable with a tiny fake.
type Port interface {
	Retrieve(ctx context.Context, query string, opts ragcore.SearchOptions) ([]ragstore.Hit, error)
	Ask(ctx context.Context, question string, req ragsvc.AskRequest) (ragcore.Answer, error)
	AskGlobal(ctx context.Context, question string, req ragsvc.GlobalRequest) (ragcore.Answer, error)
	AskDrift(ctx context.Context, question string, req ragsvc.DriftRequest) (ragcore.Answer, error)
}

// retrieverAdapter satisfies eval.Retriever. The evaluator passes rag.SearchOptions
// (TopK + per-example Namespace overlay); we force the kb namespace so an
// uploaded dataset can never escape its tenant.
type retrieverAdapter struct {
	port      Port
	namespace string
}

func (r retrieverAdapter) Retrieve(ctx context.Context, query string, opts ragcore.SearchOptions) ([]ragstore.Hit, error) {
	opts.Namespace = r.namespace
	return r.port.Retrieve(ctx, query, opts)
}

// askerAdapter satisfies eval.Asker (full retrieve+generate). It forces the kb
// namespace + token budget; hybrid (rerank) on for answer quality.
type askerAdapter struct {
	port      Port
	namespace string
	maxTokens int
}

func (a askerAdapter) Ask(ctx context.Context, question string, opts ragcore.AskOptions) (ragcore.Answer, error) {
	return a.port.Ask(ctx, question, ragsvc.AskRequest{
		Namespace:      a.namespace,
		TopK:           opts.Search.TopK,
		Hybrid:         true,
		MaxTotalTokens: a.maxTokens,
	})
}

// globalAskerAdapter satisfies eval.GlobalAsker.
type globalAskerAdapter struct {
	port      Port
	namespace string
	maxTokens int
}

func (g globalAskerAdapter) AskGlobal(ctx context.Context, question string, opts ragcore.GlobalOptions) (ragcore.Answer, error) {
	return g.port.AskGlobal(ctx, question, ragsvc.GlobalRequest{
		Namespace:      g.namespace,
		MaxCommunities: opts.MaxCommunities,
		MaxTotalTokens: g.maxTokens,
	})
}

// driftAskerAdapter satisfies eval.DriftAsker.
type driftAskerAdapter struct {
	port      Port
	namespace string
	maxTokens int
}

func (d driftAskerAdapter) AskDrift(ctx context.Context, question string, opts ragcore.DriftOptions) (ragcore.Answer, error) {
	return d.port.AskDrift(ctx, question, ragsvc.DriftRequest{
		Namespace:      d.namespace,
		MaxCommunities: opts.MaxCommunities,
		Rounds:         opts.Rounds,
		TopK:           opts.TopK,
		MaxTotalTokens: d.maxTokens,
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOWORK=off go test ./internal/eval/ -run Adapter -v`
Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/eval/adapters.go internal/eval/eval_test.go
git commit -m "eval: rag evaluator adapters over a narrow RagPort

retriever/asker/global/drift adapters force the kb namespace so an
uploaded dataset can never cross tenants; token budget threaded through."
```

---

## Task 5 — internal/eval: eval_run repo (Insert / ListByKB / LatestBenchmark)

**Files:**
- Create: `internal/eval/store.go`
- Test: `internal/eval/store_test.go` (gated)

`metrics_json` shape per §5: retrieval = `MetricsView`; triad/global/drift = `GenerationView`. `drift_json` = the serialized `DriftView`. For drift comparison we also persist a compact `eval.BenchmarkResult` so the NEXT drift run can `CompareBenchmarks(prev, curr)` — stored in `metrics_json` under the `benchmark` key for `kind='drift'` runs (see Task 6 for how it is produced).

- [ ] **Step 1: Write the failing test**

Create `internal/eval/store_test.go`:

```go
package eval

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/costa92/llm-agent-kb/internal/storage"
)

func TestEvalRunStore_InsertListLatest(t *testing.T) {
	dsn := os.Getenv("LLM_AGENT_KB_PG_URL")
	if dsn == "" {
		t.Skipf("set LLM_AGENT_KB_PG_URL (pgvector) to run the eval_run repo test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	// This test owns its tables — drop then migrate (DROP TABLE, never SCHEMA,
	// so the pgvector extension survives). storage.Open ensures the extension.
	for _, tbl := range []string{"eval_run", "qa_message", "qa_session", "document", "knowledge_base"} {
		_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS "+tbl+" CASCADE")
	}
	st, err := storage.Open(ctx, storage.Config{PGURL: dsn, EmbeddingDim: 8})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	repo := NewStore(pool)
	id1, err := repo.Insert(ctx, InsertInput{
		KBID: "kb1", Kind: KindRetrieval, DatasetName: "ds",
		MetricsJSON: []byte(`{"precisionAtK":0.5}`),
	})
	if err != nil || id1 == "" {
		t.Fatalf("insert1: id=%q err=%v", id1, err)
	}
	if _, err := repo.Insert(ctx, InsertInput{
		KBID: "kb1", Kind: KindDrift, DatasetName: "ds",
		MetricsJSON: []byte(`{"benchmark":{"x":1}}`), DriftJSON: []byte(`{"dataset":"ds"}`),
	}); err != nil {
		t.Fatalf("insert2: %v", err)
	}

	rows, next, err := repo.ListByKB(ctx, "kb1", 10, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("ListByKB = %d rows, want 2", len(rows))
	}
	if next != "" {
		t.Fatalf("next cursor = %q, want empty (page not full)", next)
	}

	// Latest drift-kind benchmark for kb1+ds is the second insert.
	raw, ok, err := repo.LatestBenchmark(ctx, "kb1", "ds")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || string(raw) != `{"benchmark":{"x":1}}` {
		t.Fatalf("LatestBenchmark ok=%v raw=%s", ok, raw)
	}

	// Isolation: a different kb sees nothing.
	if _, ok, _ := repo.LatestBenchmark(ctx, "kb2", "ds"); ok {
		t.Fatal("kb2 must not see kb1's benchmark")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOWORK=off go test ./internal/eval/ -run EvalRunStore -v`
Expected: FAIL — `NewStore`/`InsertInput`/`KindRetrieval`... some undefined (`storage.Migrate` has no `eval_run` table yet → also fails). The migration is added in Task 8; for now the compile error on `NewStore` is the expected failure.

- [ ] **Step 3: Write the repo**

Create `internal/eval/store.go`:

```go
package eval

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store persists eval_run rows (§5).
type Store struct{ pool *pgxpool.Pool }

// NewStore builds an eval_run repo.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// InsertInput is one eval_run row to persist. MetricsJSON is the kind-specific
// metric sub-struct (retrieval=MetricsView, triad/global/drift=GenerationView,
// drift additionally stores the BenchmarkResult under "benchmark"). DriftJSON is
// the serialized DriftView (only for kind=drift; nil otherwise).
type InsertInput struct {
	KBID        string
	Kind        Kind
	DatasetName string
	MetricsJSON []byte
	DriftJSON   []byte // nil for non-drift kinds
}

// RunRow is one persisted eval_run row, as read back for listing.
type RunRow struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	DatasetName string `json:"datasetName"`
	MetricsJSON []byte `json:"-"`
	DriftJSON   []byte `json:"-"`
	CreatedAt   string `json:"createdAt"`
}

// Insert writes one eval_run row and returns its id.
func (s *Store) Insert(ctx context.Context, in InsertInput) (string, error) {
	id := newID()
	var driftArg any
	if in.DriftJSON != nil {
		driftArg = in.DriftJSON
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO eval_run (id, kb_id, kind, dataset_name, metrics_json, drift_json)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		id, in.KBID, string(in.Kind), in.DatasetName, in.MetricsJSON, driftArg)
	if err != nil {
		return "", fmt.Errorf("eval: insert run: %w", err)
	}
	return id, nil
}

// ListByKB returns up to limit runs for a kb, newest first, keyset-paginated by
// the (created_at, id) compound cursor encoded as id (id is a random hex so it
// is unique; we order by created_at DESC, id DESC and page on id). Empty cursor
// starts from the newest.
func (s *Store) ListByKB(ctx context.Context, kbID string, limit int, cursor string) ([]RunRow, string, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	// Page on id: ORDER BY created_at DESC, id DESC; cursor is the last id seen.
	// Using id as the keyset key keeps it simple and stable (id is unique).
	rows, err := s.pool.Query(ctx,
		`SELECT id, kind, dataset_name, metrics_json, drift_json, created_at::text
		 FROM eval_run
		 WHERE kb_id = $1 AND ($2 = '' OR id < $2)
		 ORDER BY id DESC
		 LIMIT $3`, kbID, cursor, limit)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	var out []RunRow
	for rows.Next() {
		var r RunRow
		var drift []byte
		if err := rows.Scan(&r.ID, &r.Kind, &r.DatasetName, &r.MetricsJSON, &drift, &r.CreatedAt); err != nil {
			return nil, "", err
		}
		r.DriftJSON = drift
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(out) == limit {
		next = out[len(out)-1].ID
	}
	return out, next, nil
}

// LatestBenchmark returns the metrics_json of the most recent drift-kind run for
// kb+dataset (the stored BenchmarkResult under "benchmark"). ok=false when no
// prior drift run exists — the first drift run has no baseline to compare.
func (s *Store) LatestBenchmark(ctx context.Context, kbID, datasetName string) ([]byte, bool, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx,
		`SELECT metrics_json FROM eval_run
		 WHERE kb_id = $1 AND dataset_name = $2 AND kind = 'drift'
		 ORDER BY id DESC LIMIT 1`, kbID, datasetName).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return raw, true, nil
}
```

- [ ] **Step 4: Run test (still failing — table created in Task 8)**

Run: `GOWORK=off go build ./internal/eval/...`
Expected: PASS (compiles). The gated test still fails at runtime until Task 8 adds the `eval_run` table; that is fine — Task 8 closes the loop. Note this explicitly in the commit.

- [ ] **Step 5: Commit**

```bash
git add internal/eval/store.go internal/eval/store_test.go
git commit -m "eval: eval_run repo (insert/list-by-kb/latest-benchmark)

metrics_json shape per §5: retrieval=MetricsView, triad/global/drift=
GenerationView, drift also stores the BenchmarkResult for the next
CompareBenchmarks. Gated test green after Task 8 adds the table."
```

---

## Task 6 — internal/eval: the Run use case (build evaluators, run kind, drift compare)

**Files:**
- Modify: `internal/eval/eval.go`
- Test: `internal/eval/eval_test.go` (append)

This is the heart of M4: given a `kind`, a `Dataset`, and the kb namespace, build the right rag evaluator, run it, project to `EvalResult`. For `drift`: run an `eval.AnswerBenchmark` to get a `BenchmarkResult`, load the previous stored one, `CompareBenchmarks`, and project the `DriftReport`.

- [ ] **Step 1: Write the failing test**

Append to `internal/eval/eval_test.go`:

```go
// scriptedJudgeModel + scriptedAnswer reuse the contract scripted LLM via the
// rag generate.Model seam. The judge model returns a fixed JSON judgement.
import additions:
  rageval "..."(already), ragcore (already), ragstore (already), "context" (already)
  raggenerate "github.com/costa92/llm-agent-rag/generate"

type stubJudgeModel struct{}

func (stubJudgeModel) Generate(ctx context.Context, req raggenerate.Request) (raggenerate.Response, error) {
	return raggenerate.Response{Text: `{"groundedness":1.0,"answer_relevance":1.0}`}, nil
}

func TestRunRetrieval(t *testing.T) {
	port := fakePort{hits: []ragstore.Hit{{}}}
	svc := NewService(port, stubJudgeModel{}, ServiceConfig{MaxAskTokens: 100, GlobalMaxCommunities: 4, DriftRounds: 1, DriftTopK: 3})
	ds := rageval.Dataset{Name: "ds", TopK: 5, Examples: []rageval.Example{{Query: "q1", GoldDocIDs: []string{"d1"}}}}
	res, err := svc.Run(context.Background(), RunRequest{KBID: "kb1", Namespace: "kb_kb1", Kind: KindRetrieval, Dataset: ds})
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindRetrieval || res.Retrieval == nil {
		t.Fatalf("unexpected result: %+v", res)
	}
	if res.Retrieval.Examples != 1 {
		t.Fatalf("examples = %d, want 1", res.Retrieval.Examples)
	}
}

func TestRunTriad(t *testing.T) {
	port := fakePort{answer: ragcore.Answer{Text: "a"}}
	svc := NewService(port, stubJudgeModel{}, ServiceConfig{MaxAskTokens: 100})
	ds := rageval.Dataset{Name: "ds", TopK: 5, Examples: []rageval.Example{{Query: "q1"}}}
	res, err := svc.Run(context.Background(), RunRequest{KBID: "kb1", Namespace: "kb_kb1", Kind: KindTriad, Dataset: ds})
	if err != nil {
		t.Fatal(err)
	}
	if res.Generation == nil || res.Generation.MeanGroundedness != 1.0 {
		t.Fatalf("triad generation: %+v", res.Generation)
	}
}

func TestRunRejectsUnknownKind(t *testing.T) {
	svc := NewService(fakePort{}, stubJudgeModel{}, ServiceConfig{})
	_, err := svc.Run(context.Background(), RunRequest{KBID: "kb1", Namespace: "kb_kb1", Kind: Kind("bogus"), Dataset: rageval.Dataset{TopK: 5}})
	if err == nil {
		t.Fatal("expected error for unknown kind")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOWORK=off go test ./internal/eval/ -run "TestRun" -v`
Expected: FAIL — `NewService`/`Service`/`RunRequest`/`ServiceConfig` undefined.

- [ ] **Step 3: Implement the Service + Run**

Append to `internal/eval/eval.go`:

```go
import additions to eval.go (extend the import block):
  "context"
  "encoding/json"
  "fmt"
  raggenerate "github.com/costa92/llm-agent-rag/generate"
  ragcore "github.com/costa92/llm-agent-rag/rag"

// ServiceConfig tunes the eval use case.
type ServiceConfig struct {
	MaxAskTokens         int // rag MaxTotalTokens budget for triad/global/drift asks
	GlobalMaxCommunities int // GlobalEvaluator.MaxCommunities / DriftEvaluator.MaxCommunities
	DriftRounds          int // DriftEvaluator.Rounds
	DriftTopK            int // DriftEvaluator dataset TopK fallback
}

// Service is the kb eval use case. It builds rag's evaluators over a RagPort +
// a judge model and projects results to kb-local DTOs.
type Service struct {
	port  Port
	judge raggenerate.Model
	cfg   ServiceConfig
}

// NewService builds the eval use case. judge is the rag generate.Model seam
// (ragsvc.JudgeModel()); port is ragsvc.RagPort.
func NewService(port Port, judge raggenerate.Model, cfg ServiceConfig) *Service {
	return &Service{port: port, judge: judge, cfg: cfg}
}

// RunRequest is one eval invocation. Dataset is already parsed (httpapi parses
// inline JSONL via LoadDataset). Prev is supplied by the caller for drift via
// the eval_run store; the Service itself is store-free for unit-testability.
type RunRequest struct {
	KBID      string
	Namespace string
	Kind      Kind
	Dataset   rageval.Dataset
	// PrevBenchmarkJSON is the previously stored BenchmarkResult metrics_json
	// (from Store.LatestBenchmark) for drift compare; nil = no baseline (first run).
	PrevBenchmarkJSON []byte
}

// Run executes the eval for the request's kind and returns a kb-local result.
// For drift it also returns the current BenchmarkResult JSON so the caller can
// persist it as the next baseline (CurrBenchmarkJSON).
func (s *Service) Run(ctx context.Context, req RunRequest) (EvalResult, error) {
	switch req.Kind {
	case KindRetrieval:
		ev := rageval.RetrievalEvaluator{
			Retriever: retrieverAdapter{port: s.port, namespace: req.Namespace},
			Options:   ragcore.SearchOptions{Namespace: req.Namespace},
		}
		r, err := ev.Run(ctx, req.Dataset)
		if err != nil {
			return EvalResult{}, fmt.Errorf("eval: retrieval run: %w", err)
		}
		mv := metricsFromRetrieval(r.Metrics)
		return EvalResult{Kind: KindRetrieval, DatasetName: req.Dataset.Name, Retrieval: &mv}, nil

	case KindTriad:
		ev := rageval.TriadEvaluator{
			Asker:   askerAdapter{port: s.port, namespace: req.Namespace, maxTokens: s.cfg.MaxAskTokens},
			Judge:   rageval.LLMJudge{Model: s.judge},
			Options: ragcore.AskOptions{Search: ragcore.SearchOptions{Namespace: req.Namespace}},
		}
		r, err := ev.Run(ctx, req.Dataset)
		if err != nil {
			return EvalResult{}, fmt.Errorf("eval: triad run: %w", err)
		}
		gv := metricsFromGeneration(r.Generation.MeanGroundedness, r.Generation.MeanAnswerRelevance, r.Generation.Examples)
		return EvalResult{Kind: KindTriad, DatasetName: req.Dataset.Name, Generation: &gv}, nil

	case KindGlobal:
		ev := rageval.GlobalEvaluator{
			Asker:          globalAskerAdapter{port: s.port, namespace: req.Namespace, maxTokens: s.cfg.MaxAskTokens},
			Judge:          rageval.LLMJudge{Model: s.judge},
			MaxCommunities: s.cfg.GlobalMaxCommunities,
		}
		r, err := ev.Run(ctx, req.Dataset)
		if err != nil {
			return EvalResult{}, fmt.Errorf("eval: global run: %w", err)
		}
		gv := metricsFromGeneration(r.MeanGroundedness, r.MeanAnswerRelevance, r.Examples)
		return EvalResult{Kind: KindGlobal, DatasetName: req.Dataset.Name, Generation: &gv}, nil

	case KindDrift:
		// runDrift returns (result, baselineJSON, err); Run discards the baseline
		// (only the Runner persists it). The drift result still serializes here.
		res, _, err := s.runDrift(ctx, req)
		return res, err

	default:
		return EvalResult{}, fmt.Errorf("eval: unsupported kind %q", req.Kind)
	}
}

// runDrift produces the current BenchmarkResult via AnswerBenchmark over the
// local ask path, compares against the previous stored baseline
// (PrevBenchmarkJSON), projects the DriftReport, AND returns the scrubbed curr
// BenchmarkResult JSON so the caller persists it as the next baseline. When
// there is no baseline, Deltas compare against a zero-value benchmark (prev=NaN
// sentinels) — the report still serializes; Direction reads "undefined" for
// unscored metrics.
func (s *Service) runDrift(ctx context.Context, req RunRequest) (EvalResult, []byte, error) {
	// Build the answer dataset from the retrieval dataset (gold answers absent
	// → textual metrics degenerate; drift here tracks groundedness/relevance).
	answerDS := rageval.AnswerDataset{Name: req.Dataset.Name, TopK: req.Dataset.TopK}
	for _, ex := range req.Dataset.Examples {
		answerDS.Examples = append(answerDS.Examples, rageval.AnswerExample{Example: ex})
	}
	bench := rageval.AnswerBenchmark{
		Asker:   askerAdapter{port: s.port, namespace: req.Namespace, maxTokens: s.cfg.MaxAskTokens},
		Judge:   rageval.LLMJudge{Model: s.judge},
		Options: ragcore.AskOptions{Search: ragcore.SearchOptions{Namespace: req.Namespace}},
	}
	curr, err := bench.Run(ctx, answerDS)
	if err != nil {
		return EvalResult{}, nil, fmt.Errorf("eval: drift benchmark run: %w", err)
	}
	var prev rageval.BenchmarkResult
	if len(req.PrevBenchmarkJSON) > 0 {
		if err := json.Unmarshal(req.PrevBenchmarkJSON, &prev); err != nil {
			return EvalResult{}, nil, fmt.Errorf("eval: decode prev benchmark: %w", err)
		}
	}
	report := rageval.CompareBenchmarks(prev, curr)
	dv := driftView(report)
	gv := metricsFromGeneration(curr.Metrics.MeanGroundedness, curr.Metrics.MeanAnswerRelevance, curr.Metrics.Examples)
	baseline, err := marshalBaseline(curr)
	if err != nil {
		return EvalResult{}, nil, fmt.Errorf("eval: marshal baseline: %w", err)
	}
	return EvalResult{Kind: KindDrift, DatasetName: req.Dataset.Name, Generation: &gv, Drift: &dv}, baseline, nil
}

// RunDrift is the drift entry point used by the Runner (httpapi path): it
// returns the kb-local result AND the scrubbed current BenchmarkResult JSON to
// persist as the next baseline.
func (s *Service) RunDrift(ctx context.Context, req RunRequest) (EvalResult, []byte, error) {
	return s.runDrift(ctx, req)
}
```

> `runDrift` and `RunDrift` reference `marshalBaseline`, added in Step 4 below. Write all three (the `Run` switch above, `runDrift`, `RunDrift`) in this step; `go build` stays red until Step 4 adds `marshalBaseline`, which is expected — Step 4's run command is the green gate.

- [ ] **Step 4: Add the NaN-safe baseline marshal**

`BenchmarkMetrics` carries `math.NaN()` sentinels and has NO json tags, so round-tripping through `encoding/json` emits `NaN` (invalid JSON) and fails to re-decode. To keep the stored baseline re-loadable for the NEXT `CompareBenchmarks`, scrub NaN→0 before storage. The drift DELTA for those textual metrics then reads "unchanged"/"undefined" — acceptable: M4 drift tracks the two judge legs (MeanGroundedness/MeanAnswerRelevance), finite whenever the judge ran.

Append to `internal/eval/eval.go`:

```go
// marshalBaseline serializes a BenchmarkResult for storage, scrubbing NaN
// (BenchmarkMetrics has no json tags and NaN does not round-trip). NaN→0.
func marshalBaseline(r rageval.BenchmarkResult) ([]byte, error) {
	scrub := func(f float64) float64 {
		if math.IsNaN(f) {
			return 0
		}
		return f
	}
	r.Metrics.ExactMatch = scrub(r.Metrics.ExactMatch)
	r.Metrics.F1Token = scrub(r.Metrics.F1Token)
	r.Metrics.RequiredPhraseRecall = scrub(r.Metrics.RequiredPhraseRecall)
	r.Metrics.ReflectionRoundsMean = scrub(r.Metrics.ReflectionRoundsMean)
	r.Metrics.GraderAdoptionRate = scrub(r.Metrics.GraderAdoptionRate)
	r.Metrics.FollowupQueriesUsedMean = scrub(r.Metrics.FollowupQueriesUsedMean)
	r.Metrics.ActiveRetrievalFireRate = scrub(r.Metrics.ActiveRetrievalFireRate)
	r.Metrics.MeanGroundedness = scrub(r.Metrics.MeanGroundedness)
	r.Metrics.MeanAnswerRelevance = scrub(r.Metrics.MeanAnswerRelevance)
	return json.Marshal(r)
}
```

- [ ] **Step 5: Add the baseline marshal test**

Append to `internal/eval/eval_test.go` (`"math"` is already in the test import block from Task 2):

```go
func TestMarshalBaselineScrubsNaN(t *testing.T) {
	curr := rageval.BenchmarkResult{
		Dataset: rageval.AnswerDataset{Name: "ds", TopK: 5},
		Metrics: rageval.BenchmarkMetrics{Examples: 1, MeanGroundedness: 0.8, MeanAnswerRelevance: 0.9, ExactMatch: math.NaN(), F1Token: math.NaN()},
	}
	raw, err := marshalBaseline(curr)
	if err != nil {
		t.Fatal(err)
	}
	// Must re-decode cleanly (NaN scrubbed to 0).
	var back rageval.BenchmarkResult
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("baseline must round-trip: %v (raw=%s)", err, raw)
	}
	if back.Metrics.MeanGroundedness != 0.8 {
		t.Fatalf("groundedness lost: %+v", back.Metrics)
	}
}
```

- [ ] **Step 6: Run test to verify all eval unit tests pass**

Run: `GOWORK=off go test ./internal/eval/ -run "TestRun|TestMarshalBaseline|Adapter|MetricsView|GenerationView|DriftView" -v`
Expected: PASS (all). (`TestEvalRunStore` still skips/fails-on-table until Task 8.)

- [ ] **Step 7: Commit**

```bash
git add internal/eval/eval.go internal/eval/eval_test.go
git commit -m "eval: Run use case wrapping rag's four evaluators + drift compare

retrieval/triad/global via the rag evaluators; drift runs AnswerBenchmark,
CompareBenchmarks vs the stored baseline, and returns a NaN-scrubbed
BenchmarkResult to persist as the next baseline (§9)."
```

---

## Task 7 — internal/sessions: qa_session + qa_message repo

**Files:**
- Create: `internal/sessions/sessions.go`
- Test: `internal/sessions/sessions_test.go` (gated)

- [ ] **Step 1: Write the failing test**

Create `internal/sessions/sessions_test.go`:

```go
package sessions

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/costa92/llm-agent-kb/internal/storage"
)

func TestSessions_EnsureAppendListTranscript(t *testing.T) {
	dsn := os.Getenv("LLM_AGENT_KB_PG_URL")
	if dsn == "" {
		t.Skipf("set LLM_AGENT_KB_PG_URL (pgvector) to run the sessions repo test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	for _, tbl := range []string{"qa_message", "qa_session", "eval_run", "document", "knowledge_base"} {
		_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS "+tbl+" CASCADE")
	}
	st, err := storage.Open(ctx, storage.Config{PGURL: dsn, EmbeddingDim: 8})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	repo := New(pool)
	// EnsureSession with empty id creates one.
	sid, err := repo.EnsureSession(ctx, "kb1", "user1", "", "fox?")
	if err != nil || sid == "" {
		t.Fatalf("ensure: sid=%q err=%v", sid, err)
	}
	// Same id is reused.
	sid2, err := repo.EnsureSession(ctx, "kb1", "user1", sid, "ignored")
	if err != nil || sid2 != sid {
		t.Fatalf("ensure reuse: sid2=%q want %q err=%v", sid2, sid, err)
	}
	if err := repo.AppendPair(ctx, sid, "fox?", "the fox", []byte(`[{"chunkId":"c1"}]`), "hybrid"); err != nil {
		t.Fatalf("append: %v", err)
	}

	list, next, err := repo.ListByKB(ctx, "kb1", 10, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != sid {
		t.Fatalf("list = %+v", list)
	}
	if next != "" {
		t.Fatalf("next = %q want empty", next)
	}

	msgs, err := repo.Transcript(ctx, "kb1", sid)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 || msgs[0].Role != "user" || msgs[1].Role != "assistant" {
		t.Fatalf("transcript = %+v", msgs)
	}
	if msgs[1].Mode != "hybrid" {
		t.Fatalf("assistant mode = %q", msgs[1].Mode)
	}

	// Isolation: kb2 sees nothing.
	if l, _, _ := repo.ListByKB(ctx, "kb2", 10, ""); len(l) != 0 {
		t.Fatalf("kb2 leak: %+v", l)
	}
	if _, err := repo.Transcript(ctx, "kb2", sid); err == nil {
		t.Fatal("kb2 transcript of kb1 session must error (not found)")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOWORK=off go build ./internal/sessions/...`
Expected: FAIL — no such package / undefined `New`.

- [ ] **Step 3: Write the repo**

Create `internal/sessions/sessions.go`:

```go
// Package sessions persists Q&A history (§5 qa_session + qa_message). It writes
// no vectors; the ask path calls EnsureSession + AppendPair, the API reads
// ListByKB + Transcript. kb isolation is enforced by joining qa_message→
// qa_session and filtering on kb_id.
package sessions

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a session does not exist for the kb.
var ErrNotFound = errors.New("sessions: not found")

// Repo persists sessions + messages.
type Repo struct{ pool *pgxpool.Pool }

// New builds a sessions Repo.
func New(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Session is one qa_session row.
type Session struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	CreatedAt string `json:"createdAt"`
}

// Message is one qa_message row.
type Message struct {
	ID            string `json:"id"`
	Role          string `json:"role"`
	Content       string `json:"content"`
	CitationsJSON []byte `json:"-"`
	Mode          string `json:"mode"`
	CreatedAt     string `json:"createdAt"`
}

// EnsureSession returns sessionID when non-empty AND it belongs to kb+user;
// otherwise it creates a new session titled from the first question (trimmed).
// An inbound sessionID that does not match kb+user is treated as "create new"
// (no cross-tenant write).
func (r *Repo) EnsureSession(ctx context.Context, kbID, userID, sessionID, firstQuestion string) (string, error) {
	if sessionID != "" {
		var owned bool
		err := r.pool.QueryRow(ctx,
			`SELECT true FROM qa_session WHERE id = $1 AND kb_id = $2 AND user_id = $3`,
			sessionID, kbID, userID).Scan(&owned)
		if err == nil && owned {
			return sessionID, nil
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return "", err
		}
		// not owned → fall through to create
	}
	id := newID()
	title := firstQuestion
	if len(title) > 80 {
		title = title[:80]
	}
	if _, err := r.pool.Exec(ctx,
		`INSERT INTO qa_session (id, kb_id, user_id, title) VALUES ($1, $2, $3, $4)`,
		id, kbID, userID, title); err != nil {
		return "", fmt.Errorf("sessions: create: %w", err)
	}
	return id, nil
}

// AppendPair writes the user question + assistant answer as two qa_message rows
// in one transaction. citationsJSON is the serialized citation array (assistant
// row); mode is the ask mode (vector/hybrid/global/drift).
func (r *Repo) AppendPair(ctx context.Context, sessionID, question, answer string, citationsJSON []byte, mode string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx,
		`INSERT INTO qa_message (id, session_id, role, content, citations_json, mode)
		 VALUES ($1, $2, 'user', $3, NULL, $4)`,
		newID(), sessionID, question, mode); err != nil {
		return fmt.Errorf("sessions: append user msg: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO qa_message (id, session_id, role, content, citations_json, mode)
		 VALUES ($1, $2, 'assistant', $3, $4, $5)`,
		newID(), sessionID, answer, citationsJSON, mode); err != nil {
		return fmt.Errorf("sessions: append assistant msg: %w", err)
	}
	return tx.Commit(ctx)
}

// ListByKB returns up to limit sessions for a kb, newest first, keyset-paginated
// on id (cursor = last id seen).
func (r *Repo) ListByKB(ctx context.Context, kbID string, limit int, cursor string) ([]Session, string, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id, title, created_at::text FROM qa_session
		 WHERE kb_id = $1 AND ($2 = '' OR id < $2)
		 ORDER BY id DESC LIMIT $3`, kbID, cursor, limit)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		var s Session
		if err := rows.Scan(&s.ID, &s.Title, &s.CreatedAt); err != nil {
			return nil, "", err
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(out) == limit {
		next = out[len(out)-1].ID
	}
	return out, next, nil
}

// Transcript returns the messages of a session in chronological order, after
// verifying the session belongs to kbID (cross-tenant read → ErrNotFound).
func (r *Repo) Transcript(ctx context.Context, kbID, sessionID string) ([]Message, error) {
	var owned bool
	err := r.pool.QueryRow(ctx,
		`SELECT true FROM qa_session WHERE id = $1 AND kb_id = $2`, sessionID, kbID).Scan(&owned)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id, role, content, citations_json, mode, created_at::text
		 FROM qa_message WHERE session_id = $1 ORDER BY created_at ASC, id ASC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var m Message
		var cites []byte
		if err := rows.Scan(&m.ID, &m.Role, &m.Content, &cites, &m.Mode, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.CitationsJSON = cites
		out = append(out, m)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Build (gated test runs after Task 8 adds tables)**

Run: `GOWORK=off go build ./internal/sessions/...`
Expected: PASS (compiles). Gated test passes after Task 8.

- [ ] **Step 5: Commit**

```bash
git add internal/sessions/sessions.go internal/sessions/sessions_test.go
git commit -m "sessions: qa_session + qa_message repo (§5)

EnsureSession (create-on-first-ask, reuse owned id, no cross-tenant
write), AppendPair (user+assistant in one tx), ListByKB, Transcript
(kb-scoped). Gated test green after Task 8 adds the tables."
```

---

## Task 8 — storage: eval_run + qa_session + qa_message migrations

**Files:**
- Modify: `internal/storage/storage.go`

This closes the loop for the Task 5 + Task 7 gated tests.

- [ ] **Step 1: Write the failing test**

The gated repo tests from Tasks 5 + 7 ARE the failing tests. Run them first to confirm the table is missing:

Run: `GOWORK=off go test ./internal/eval/ -run EvalRunStore -v` (with `LLM_AGENT_KB_PG_URL` set)
Expected: FAIL — `relation "eval_run" does not exist`.

- [ ] **Step 2: Add the migrations**

In `internal/storage/storage.go`, append to the `businessMigrations` slice (before the closing `}`):

```go
	// M4 eval + sessions (§5).
	`CREATE TABLE IF NOT EXISTS eval_run (
		id           TEXT PRIMARY KEY,
		kb_id        TEXT NOT NULL,
		kind         TEXT NOT NULL,
		dataset_name TEXT NOT NULL,
		metrics_json JSONB NOT NULL,
		drift_json   JSONB,
		created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
	)`,
	`CREATE INDEX IF NOT EXISTS eval_run_kb_idx ON eval_run (kb_id, id DESC)`,
	`CREATE INDEX IF NOT EXISTS eval_run_drift_idx ON eval_run (kb_id, dataset_name, kind, id DESC)`,
	`CREATE TABLE IF NOT EXISTS qa_session (
		id         TEXT PRIMARY KEY,
		kb_id      TEXT NOT NULL,
		user_id    TEXT NOT NULL,
		title      TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`,
	`CREATE INDEX IF NOT EXISTS qa_session_kb_idx ON qa_session (kb_id, id DESC)`,
	`CREATE TABLE IF NOT EXISTS qa_message (
		id             TEXT PRIMARY KEY,
		session_id     TEXT NOT NULL REFERENCES qa_session(id) ON DELETE CASCADE,
		role           TEXT NOT NULL,
		content        TEXT NOT NULL,
		citations_json JSONB,
		mode           TEXT NOT NULL DEFAULT '',
		created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
	)`,
	`CREATE INDEX IF NOT EXISTS qa_message_session_idx ON qa_message (session_id, created_at)`,
```

Note: `eval_run.kb_id` / `qa_session.kb_id` are deliberately NOT foreign keys to `knowledge_base` — kb deletion semantics for history are out of M4 scope (history outlives the kb row in v1; spec §16.4 cascade covers chunks/graph, not history). `qa_message`→`qa_session` IS a FK with cascade so message rows die with their session.

- [ ] **Step 3: Run the gated repo tests to verify they pass**

Run: `GOWORK=off go test ./internal/eval/ -run EvalRunStore -v && GOWORK=off go test ./internal/sessions/ -run Sessions_ -v`
Expected: PASS (both gated tests). If `LLM_AGENT_KB_PG_URL` is unset they SKIP — set it first (see Gated test DB block).

- [ ] **Step 4: Commit**

```bash
git add internal/storage/storage.go
git commit -m "storage: eval_run + qa_session + qa_message migrations (§5)

Idempotent CREATE TABLE IF NOT EXISTS + indices for kb-scoped listing
and the drift baseline lookup. qa_message cascades on qa_session delete;
history rows intentionally not FK'd to knowledge_base (out of M4 scope)."
```

---

## Task 9 — retrieval: persist a qa_message pair per ask

**Files:**
- Modify: `internal/retrieval/retrieval.go`
- Test: `internal/retrieval/retrieval_test.go`

The ask path now records history. To keep `retrieval` free of a DB dependency (and DB-free unit-testable), it takes a narrow `Recorder` interface; nil disables persistence (focused unit tests that don't care).

- [ ] **Step 1: Write the failing test**

Append to `internal/retrieval/retrieval_test.go`:

```go
// fakeRecorder records the persistence calls.
type fakeRecorder struct {
	ensured  bool
	appended bool
	gotMode  string
	sid      string
}

func (f *fakeRecorder) EnsureSession(ctx context.Context, kbID, userID, sessionID, firstQuestion string) (string, error) {
	f.ensured = true
	f.sid = "sess1"
	return f.sid, nil
}
func (f *fakeRecorder) AppendPair(ctx context.Context, sessionID, question, answer string, citationsJSON []byte, mode string) error {
	f.appended = true
	f.gotMode = mode
	return nil
}

func TestAskPersistsSession(t *testing.T) {
	rec := &fakeRecorder{}
	svc := New(fakeRagForAsk(), Config{})  // fakeRagForAsk = the existing test fake returning ≥1 hit
	svc.SetRecorder(rec)
	out, err := svc.Ask(context.Background(), AskInput{
		Namespace: "kb_x", KBID: "x", UserID: "u1", Question: "fox?", Mode: "hybrid", TopK: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.SessionID != "sess1" {
		t.Fatalf("SessionID = %q, want sess1", out.SessionID)
	}
	if !rec.ensured || !rec.appended {
		t.Fatalf("recorder not called: ensured=%v appended=%v", rec.ensured, rec.appended)
	}
	if rec.gotMode != "hybrid" {
		t.Fatalf("mode = %q", rec.gotMode)
	}
}
```

Use the EXISTING ask-path fake in `retrieval_test.go` for `fakeRagForAsk()` — match its actual constructor/name in that file (read it first; if the test builds the fake inline, build it the same way). The new assertions are `SetRecorder`, `AskInput.{KBID,UserID}`, and `AskOutput.SessionID`.

- [ ] **Step 2: Run test to verify it fails**

Run: `GOWORK=off go test ./internal/retrieval/ -run PersistsSession -v`
Expected: FAIL — `svc.SetRecorder undefined` / `AskInput.KBID undefined` / `out.SessionID undefined`.

- [ ] **Step 3: Implement the Recorder seam + persistence**

In `internal/retrieval/retrieval.go`:

(a) Add the `Recorder` interface + the new fields. Add after the `Service` struct:

```go
// Recorder persists Q&A history (satisfied by *sessions.Repo). nil disables
// persistence (focused unit tests). Kept a narrow interface so retrieval has no
// DB dependency.
type Recorder interface {
	EnsureSession(ctx context.Context, kbID, userID, sessionID, firstQuestion string) (string, error)
	AppendPair(ctx context.Context, sessionID, question, answer string, citationsJSON []byte, mode string) error
}

// SetRecorder wires history persistence after construction (cmd/kbd sets it).
func (s *Service) SetRecorder(r Recorder) { s.recorder = r }
```

Add `recorder Recorder` to the `Service` struct.

(b) Add `KBID`, `UserID`, `SessionID` to `AskInput`, `GlobalInput`, `DriftInput` (the persistence needs them). For `AskInput`:

```go
type AskInput struct {
	Namespace string
	KBID      string
	UserID    string
	SessionID string // optional; empty = create-on-first-ask
	Question  string
	Mode      string
	TopK      int
}
```

Add the same three fields (`KBID`, `UserID`, `SessionID`) to `GlobalInput` and `DriftInput`.

(c) Add `SessionID string \`json:"sessionId,omitempty"\`` to `AskOutput`.

(d) Add a private helper + call it at the end of each ask method (`Ask`, `AskGlobal`, `AskDrift`) just before the successful return. Add the helper:

```go
// persist records the q/a pair into a session (create-on-first-ask). Best effort
// for the session id but errors propagate so callers see a 500 on a broken DB;
// a nil recorder is a no-op (returns the inbound sessionID unchanged).
func (s *Service) persist(ctx context.Context, kbID, userID, sessionID, question, mode string, out AskOutput) (string, error) {
	if s.recorder == nil {
		return sessionID, nil
	}
	sid, err := s.recorder.EnsureSession(ctx, kbID, userID, sessionID, question)
	if err != nil {
		return "", err
	}
	citesJSON, _ := json.Marshal(out.Citations)
	if err := s.recorder.AppendPair(ctx, sid, question, out.Answer, citesJSON, mode); err != nil {
		return "", err
	}
	return sid, nil
}
```

Add `"encoding/json"` to the import block.

In `Ask`, replace the final `return AskOutput{...}, nil` with: build the `out` value into a variable, then:

```go
	out := AskOutput{
		Answer:    ans.Text,
		Citations: cites,
		Diagnostics: map[string]any{
			"mode":     in.Mode,
			"hitCount": ans.Diagnostics.HitCount,
		},
	}
	sid, err := s.persist(ctx, in.KBID, in.UserID, in.SessionID, in.Question, in.Mode, out)
	if err != nil {
		return AskOutput{}, err
	}
	out.SessionID = sid
	return out, nil
```

Do the equivalent in `AskGlobal` (mode `"global"`) and `AskDrift` (mode `"drift"`): capture the existing returned `AskOutput` into a local `out`, call `s.persist(ctx, in.KBID, in.UserID, in.SessionID, in.Question, "global"/"drift", out)`, set `out.SessionID = sid`, return.

- [ ] **Step 4: Run test + whole-module vet (interface unchanged but signature grew)**

Run: `GOWORK=off go test ./internal/retrieval/ -v && GOWORK=off go vet ./...`
Expected: PASS; vet clean. The `httpapi.Asker` interface (`Ask(ctx, retrieval.AskInput)`) is unchanged in shape (the struct grew fields, not the method), so httpapi fakes still compile.

- [ ] **Step 5: Commit**

```bash
git add internal/retrieval/retrieval.go internal/retrieval/retrieval_test.go
git commit -m "retrieval: persist a qa_message pair per ask

create-on-first-ask via a narrow Recorder seam (nil = no-op); inbound
optional sessionId reused when owned; AskOutput returns the sessionId
(§5, §16.2)."
```

---

## Task 10 — httpapi: eval + sessions endpoints + eval-run guard

**Files:**
- Create: `internal/httpapi/eval.go`
- Modify: `internal/httpapi/httpapi.go` (eval/session routes + Deps; **Step 4b** threads session fields through the three ask handlers)
- Test: `internal/httpapi/eval_test.go`; `internal/httpapi/community_test.go` (extend `fakeAsker` to capture) + `internal/httpapi/ask_test.go` (Step 4b test)

Routes (§16.2): `POST /api/kb/{id}/eval/run` (editor+), `GET /api/kb/{id}/eval/runs` (viewer+), `GET /api/kb/{id}/sessions` (viewer+), `GET /api/kb/{id}/sessions/{sid}` (viewer+). The eval-run handler is additionally wrapped by a SEPARATE per-user `limits.Guard` (eval is expensive). **Step 4b additionally wires the existing ask handlers to thread `KBID`/`UserID`/`SessionID` into the retrieval inputs (B1 blocker — without it sessions persist orphaned).**

- [ ] **Step 1: Write the failing test**

Create `internal/httpapi/eval_test.go`:

```go
package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	kbeval "github.com/costa92/llm-agent-kb/internal/eval"
	"github.com/costa92/llm-agent-kb/internal/sessions"
)

// fakeEvalRunner satisfies EvalRunner.
type fakeEvalRunner struct {
	lastKind kbeval.Kind
	lastNS   string
}

func (f *fakeEvalRunner) RunEval(ctx context.Context, kbID, namespace string, kind kbeval.Kind, datasetJSONL []byte) (kbeval.EvalResult, string, error) {
	f.lastKind = kind
	f.lastNS = namespace
	mv := kbeval.MetricsView{PrecisionAtK: 0.5, Examples: 1, TopK: 5}
	return kbeval.EvalResult{Kind: kind, DatasetName: "ds", Retrieval: &mv}, "run123", nil
}
func (f *fakeEvalRunner) ListRuns(ctx context.Context, kbID string, limit int, cursor string) ([]kbeval.RunRow, string, error) {
	return []kbeval.RunRow{{ID: "run123", Kind: "retrieval", DatasetName: "ds"}}, "", nil
}

// fakeSessionReader satisfies SessionReader.
type fakeSessionReader struct{}

func (fakeSessionReader) ListByKB(ctx context.Context, kbID string, limit int, cursor string) ([]sessions.Session, string, error) {
	return []sessions.Session{{ID: "s1", Title: "fox?"}}, "", nil
}
func (fakeSessionReader) Transcript(ctx context.Context, kbID, sessionID string) ([]sessions.Message, error) {
	return []sessions.Message{{ID: "m1", Role: "user", Content: "fox?"}, {ID: "m2", Role: "assistant", Content: "the fox", Mode: "hybrid"}}, nil
}

func TestEvalRunHandler(t *testing.T) {
	runner := &fakeEvalRunner{}
	h := evalRunHandler(stubKBGetter{ns: "kb_x"}, runner, mustGuard())
	req := httptest.NewRequest("POST", "/api/kb/x/eval/run", strings.NewReader(`{"kind":"retrieval","dataset":"{\"query\":\"q\",\"top_k\":5}"}`))
	req.SetPathValue("id", "x")
	w := httptest.NewRecorder()
	h(w, req) // no uid set: unlimited guard ignores the empty key (authzhttp has no WithUserID)
	if w.Code != http.StatusOK {
		t.Fatalf("code = %d body=%s want 200", w.Code, w.Body.String())
	}
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["runId"] != "run123" {
		t.Fatalf("runId = %v", body["runId"])
	}
	if runner.lastNS != "kb_x" {
		t.Fatalf("namespace forced = %q, want kb_x", runner.lastNS)
	}
}

func TestEvalRunHandlerRejectsBadKind(t *testing.T) {
	h := evalRunHandler(stubKBGetter{ns: "kb_x"}, &fakeEvalRunner{}, mustGuard())
	req := httptest.NewRequest("POST", "/api/kb/x/eval/run", strings.NewReader(`{"kind":"bogus","dataset":"{}"}`))
	req.SetPathValue("id", "x")
	w := httptest.NewRecorder()
	h(w, req) // no uid set: unlimited guard ignores the empty key (authzhttp has no WithUserID)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("code = %d want 400", w.Code)
	}
}

func TestListRunsHandler(t *testing.T) {
	h := listRunsHandler(stubKBGetter{ns: "kb_x"}, &fakeEvalRunner{})
	req := httptest.NewRequest("GET", "/api/kb/x/eval/runs", nil)
	req.SetPathValue("id", "x")
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code = %d", w.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if _, ok := body["items"]; !ok {
		t.Fatalf("missing items envelope: %s", w.Body.String())
	}
}

func TestSessionTranscriptHandler(t *testing.T) {
	h := sessionTranscriptHandler(stubKBGetter{ns: "kb_x"}, fakeSessionReader{})
	req := httptest.NewRequest("GET", "/api/kb/x/sessions/s1", nil)
	req.SetPathValue("id", "x")
	req.SetPathValue("sid", "s1")
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code = %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"messages"`) {
		t.Fatalf("missing messages: %s", w.Body.String())
	}
}
```

Add the small test helpers at the bottom of `eval_test.go` (or reuse equivalents if they already exist in `httpapi_test.go` — check first and DELETE these if duplicated):

```go
import additions for helpers:
  "github.com/costa92/llm-agent-kb/internal/limits"
  "github.com/costa92/llm-agent-kb/internal/orgkb"

type stubKBGetter struct{ ns string }

func (s stubKBGetter) Get(ctx context.Context, id string) (orgkb.KB, error) {
	return orgkb.KB{ID: id, Namespace: s.ns}, nil
}

func mustGuard() *limits.Guard { return limits.New(0) } // 0 = unlimited
```

> Verified against `llm-agent-authz@v0.1.0`: `httpapi` exports only `func UserID(ctx context.Context) string` — there is NO exported `WithUserID` setter (the uid is set via an unexported key inside `authzhttp.Authenticate`). So these DB-free handler tests do NOT set a uid: the eval-run guard keys on `authzhttp.UserID(ctx)`, which returns "" outside a real auth chain, and the unlimited guard (`limits.New(0)`) ignores the key — empty uid is harmless. Call `h(w, req)` directly; there is no `withUID` wrapper. (No `authzhttp` import needed in the test.)

- [ ] **Step 2: Run test to verify it fails**

Run: `GOWORK=off go test ./internal/httpapi/ -run "EvalRun|ListRuns|SessionTranscript" -v`
Expected: FAIL — `evalRunHandler`/`EvalRunner`/`SessionReader` undefined.

- [ ] **Step 3: Write the handlers**

Create `internal/httpapi/eval.go`:

```go
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	kbeval "github.com/costa92/llm-agent-kb/internal/eval"
	"github.com/costa92/llm-agent-kb/internal/limits"
	"github.com/costa92/llm-agent-kb/internal/orgkb"
	"github.com/costa92/llm-agent-kb/internal/sessions"
)

// EvalRunner is the eval surface (satisfied by *eval.Runner — Task 11 wires it).
// RunEval forces the kb namespace; ListRuns is cursor-paginated.
type EvalRunner interface {
	RunEval(ctx context.Context, kbID, namespace string, kind kbeval.Kind, datasetJSONL []byte) (kbeval.EvalResult, string, error)
	ListRuns(ctx context.Context, kbID string, limit int, cursor string) ([]kbeval.RunRow, string, error)
}

// SessionReader is the read surface for Q&A history (satisfied by *sessions.Repo).
type SessionReader interface {
	ListByKB(ctx context.Context, kbID string, limit int, cursor string) ([]sessions.Session, string, error)
	Transcript(ctx context.Context, kbID, sessionID string) ([]sessions.Message, error)
}

// validEvalKind reports whether k is one of the four supported kinds.
func validEvalKind(k kbeval.Kind) bool {
	switch k {
	case kbeval.KindRetrieval, kbeval.KindTriad, kbeval.KindGlobal, kbeval.KindDrift:
		return true
	}
	return false
}

// evalRunHandler runs an eval (editor+, §16.2). Body: {kind, datasetName?,
// dataset}. dataset is inline JSONL (one Example per line; LoadDataset parses).
// The handler enforces a SEPARATE per-user eval-run budget (eval is expensive).
func evalRunHandler(repo kbGetter, runner EvalRunner, evalGuard *limits.Guard) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := userIDFromCtx(r)
		if !evalGuard.Allow(uid) {
			http.Error(w, "eval-run rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		kb, err := repo.Get(r.Context(), r.PathValue("id"))
		if errors.Is(err, orgkb.ErrNotFound) {
			http.Error(w, "kb not found", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		var req struct {
			Kind    string `json:"kind"`
			Dataset string `json:"dataset"` // inline JSONL
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		kind := kbeval.Kind(req.Kind)
		if !validEvalKind(kind) {
			http.Error(w, "unsupported eval kind (retrieval|triad|global|drift)", http.StatusBadRequest)
			return
		}
		if req.Dataset == "" {
			http.Error(w, "dataset (inline JSONL) required", http.StatusBadRequest)
			return
		}
		res, runID, err := runner.RunEval(r.Context(), kb.ID, kb.Namespace, kind, []byte(req.Dataset))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"runId": runID, "result": res})
	}
}

// listRunsHandler lists eval runs for a kb (viewer+, cursor envelope).
func listRunsHandler(repo kbGetter, runner EvalRunner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		kb, err := repo.Get(r.Context(), r.PathValue("id"))
		if errors.Is(err, orgkb.ErrNotFound) {
			http.Error(w, "kb not found", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		limit := parseLimit(r)
		rows, next, err := runner.ListRuns(r.Context(), kb.ID, limit, r.URL.Query().Get("cursor"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		items := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			item := map[string]any{
				"id": row.ID, "kind": row.Kind, "datasetName": row.DatasetName,
				"createdAt": row.CreatedAt, "metrics": json.RawMessage(row.MetricsJSON),
			}
			if len(row.DriftJSON) > 0 {
				item["drift"] = json.RawMessage(row.DriftJSON)
			}
			items = append(items, item)
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items, "next_cursor": next})
	}
}

// listSessionsHandler lists Q&A sessions for a kb (viewer+, cursor envelope).
func listSessionsHandler(repo kbGetter, reader SessionReader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		kb, err := repo.Get(r.Context(), r.PathValue("id"))
		if errors.Is(err, orgkb.ErrNotFound) {
			http.Error(w, "kb not found", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		limit := parseLimit(r)
		rows, next, err := reader.ListByKB(r.Context(), kb.ID, limit, r.URL.Query().Get("cursor"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		items := make([]map[string]any, 0, len(rows))
		for _, s := range rows {
			items = append(items, map[string]any{"id": s.ID, "title": s.Title, "createdAt": s.CreatedAt})
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items, "next_cursor": next})
	}
}

// sessionTranscriptHandler returns a session's messages (viewer+). A session
// belonging to another kb resolves to 404 (sessions.ErrNotFound).
func sessionTranscriptHandler(repo kbGetter, reader SessionReader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		kb, err := repo.Get(r.Context(), r.PathValue("id"))
		if errors.Is(err, orgkb.ErrNotFound) {
			http.Error(w, "kb not found", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		msgs, err := reader.Transcript(r.Context(), kb.ID, r.PathValue("sid"))
		if errors.Is(err, sessions.ErrNotFound) {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		items := make([]map[string]any, 0, len(msgs))
		for _, m := range msgs {
			item := map[string]any{"id": m.ID, "role": m.Role, "content": m.Content, "mode": m.Mode, "createdAt": m.CreatedAt}
			if len(m.CitationsJSON) > 0 {
				item["citations"] = json.RawMessage(m.CitationsJSON)
			}
			items = append(items, item)
		}
		writeJSON(w, http.StatusOK, map[string]any{"sessionId": r.PathValue("sid"), "messages": items})
	}
}

// parseLimit reads ?limit= (0 = repo default).
func parseLimit(r *http.Request) int {
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return 0
}
```

Add a tiny `userIDFromCtx` helper to `eval.go` that wraps the authz accessor (so the handler doesn't import authzhttp twice):

```go
// userIDFromCtx returns the authenticated user id (empty outside the auth chain).
func userIDFromCtx(r *http.Request) string { return authzhttp.UserID(r.Context()) }
```

Add `authzhttp "github.com/costa92/llm-agent-authz/httpapi"` to `eval.go`'s imports.

- [ ] **Step 4: Wire the routes + Deps + eval guard in httpapi.go**

In `internal/httpapi/httpapi.go`:

(a) Add to `Deps`:

```go
	EvalRunner            EvalRunner    // eval run/list (M4); nil disables eval routes
	SessionReader         SessionReader // Q&A history reads (M4); nil disables session routes
	EvalRunsPerUserMinute int           // per-user eval-run budget (separate, smaller than the ask limiter)
```

(b) In `NewMux`, after the existing `guard := limits.New(d.PerUserLimit)` line, add:

```go
	evalGuard := limits.New(d.EvalRunsPerUserMinute)
```

(c) Add the routes inside `NewMux`. Place them after the document routes block, gated on the deps being present:

```go
	// Eval (M4) — run is editor+, list is viewer+. Wired only when an EvalRunner + KBRepo are set.
	if d.EvalRunner != nil && d.KBRepo != nil {
		mux.Handle("POST /api/kb/{id}/eval/run", chain(authzrole.RoleEditor, evalRunHandler(d.KBRepo, d.EvalRunner, evalGuard)))
		mux.Handle("GET /api/kb/{id}/eval/runs", chain(authzrole.RoleViewer, listRunsHandler(d.KBRepo, d.EvalRunner)))
	}
	// Sessions (M4) — viewer+. Wired only when a SessionReader + KBRepo are set.
	if d.SessionReader != nil && d.KBRepo != nil {
		mux.Handle("GET /api/kb/{id}/sessions", chain(authzrole.RoleViewer, listSessionsHandler(d.KBRepo, d.SessionReader)))
		mux.Handle("GET /api/kb/{id}/sessions/{sid}", chain(authzrole.RoleViewer, sessionTranscriptHandler(d.KBRepo, d.SessionReader)))
	}
```

- [ ] **Step 4b: Thread KBID/UserID/SessionID through the three ask handlers (BLOCKER — sessions persist orphaned without this)**

The ask handlers (`askHandler`/`askGlobalHandler`/`askDriftHandler` in `internal/httpapi/httpapi.go`) currently build `retrieval.AskInput{Namespace, Question, Mode, TopK}` (and the Global/Drift equivalents) only — they do NOT populate `KBID`/`UserID`/`SessionID` (added to those structs in Task 9), so the Task 9 `persist(...)` call would create a session with empty kb/user and the Task 13 e2e (`GET /sessions` after an ask) would see nothing. Wire the fields here, in the package that owns httpapi.

(a) **Write the failing handler test.** The package already has a shared `fakeAsker` (in `internal/httpapi/community_test.go`: `type fakeAsker struct{ out, globalOut, driftOut retrieval.AskOutput }`, used by `ask_test.go`) — its `Ask` discards the input. Extend it to capture, then assert. In `internal/httpapi/community_test.go`, add a `gotAsk retrieval.AskInput` field and record it in `Ask`:

```go
type fakeAsker struct {
	out       retrieval.AskOutput
	globalOut retrieval.AskOutput
	driftOut  retrieval.AskOutput
	gotAsk    retrieval.AskInput // captured by Ask for the session-threading test
}

func (f *fakeAsker) Ask(_ context.Context, in retrieval.AskInput) (retrieval.AskOutput, error) {
	f.gotAsk = in
	return f.out, nil
}
```

(Leave `AskGlobal`/`AskDrift` as they are.) Then append the test to `internal/httpapi/ask_test.go` (it already imports `retrieval`, `httptest`, `strings`, `testing`; add `"net/http"`):

```go
func TestAskHandlerThreadsSessionFields(t *testing.T) {
	asker := &fakeAsker{out: retrieval.AskOutput{Answer: "a", SessionID: "sess1"}}
	h := askHandler(asker)
	req := httptest.NewRequest("POST", "/api/kb/x/ask", strings.NewReader(`{"q":"fox","mode":"hybrid","topK":5,"sessionId":"s9"}`))
	req.SetPathValue("id", "x")
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code = %d body=%s", w.Code, w.Body.String())
	}
	if asker.gotAsk.KBID != "x" {
		t.Fatalf("KBID = %q, want x", asker.gotAsk.KBID)
	}
	if asker.gotAsk.SessionID != "s9" {
		t.Fatalf("SessionID = %q, want s9", asker.gotAsk.SessionID)
	}
	// UserID comes from authzhttp.UserID(ctx) — empty outside the auth chain, which
	// is correct: the handler must read it from context, not the body.
	if asker.gotAsk.Namespace != "kb_x" {
		t.Fatalf("Namespace = %q, want kb_x", asker.gotAsk.Namespace)
	}
}
```

(b) **Run it to confirm it fails:** `GOWORK=off go test ./internal/httpapi/ -run AskHandlerThreadsSessionFields -v` → FAIL (`asker.got.KBID` always empty / `sessionId` not parsed).

(c) **Extend the three ask request structs + populate the fields.** In `internal/httpapi/httpapi.go`:

`askHandler` — add `SessionID` to the request struct and set the three fields on `AskInput`:

```go
		var req struct {
			Q         string `json:"q"`
			Mode      string `json:"mode"`
			TopK      int    `json:"topK"`
			SessionID string `json:"sessionId"`
		}
		...
		out, err := asker.Ask(r.Context(), retrieval.AskInput{
			Namespace: "kb_" + r.PathValue("id"), // namespace = "kb_"+id (orgkb.Create convention)
			KBID:      r.PathValue("id"),
			UserID:    authzhttp.UserID(r.Context()),
			SessionID: req.SessionID,
			Question:  req.Q,
			Mode:      req.Mode,
			TopK:      req.TopK,
		})
```

`askGlobalHandler` — add `SessionID string \`json:"sessionId"\`` to its request struct and set `KBID: r.PathValue("id"), UserID: authzhttp.UserID(r.Context()), SessionID: req.SessionID` on the `retrieval.GlobalInput{...}`.

`askDriftHandler` — same: add `SessionID string \`json:"sessionId"\`` to its request struct and set `KBID: r.PathValue("id"), UserID: authzhttp.UserID(r.Context()), SessionID: req.SessionID` on the `retrieval.DriftInput{...}`.

(`authzhttp` is ALREADY imported in `httpapi.go`; `authzhttp.UserID(ctx) string` exists — verified against `llm-agent-authz@v0.1.0`. Keeps the spec §16.2 "optional inbound sessionId" contract: empty `sessionId` → create-on-first-ask.)

(d) **Run it to confirm it passes:** `GOWORK=off go test ./internal/httpapi/ -run AskHandlerThreadsSessionFields -v` → PASS. This also unblocks the Task 13 e2e (`GET /sessions` after an ask returns the persisted session).

- [ ] **Step 5: Run tests + whole-module vet**

Run: `GOWORK=off go test ./internal/httpapi/ -v && GOWORK=off go vet ./...`
Expected: PASS; vet clean. (The new handler tests pass; existing httpapi tests unaffected — new Deps fields are nil there.)

- [ ] **Step 6: Commit**

```bash
git add internal/httpapi/eval.go internal/httpapi/httpapi.go internal/httpapi/eval_test.go internal/httpapi/community_test.go internal/httpapi/ask_test.go
git commit -m "httpapi: eval run/list + session list/transcript + eval-run guard

POST /eval/run (editor+, separate per-user eval budget), GET /eval/runs,
GET /sessions, GET /sessions/{sid} (viewer+), cursor envelope. Handlers
force kb namespace + kb-scoped reads; cross-kb session → 404 (§16.2).
The three ask handlers now thread KBID/UserID(from ctx)/SessionID(from
body) into AskInput/GlobalInput/DriftInput so sessions persist scoped."
```

---

## Task 11 — internal/eval: the Runner (parse JSONL + persist + drift baseline)

**Files:**
- Modify: `internal/eval/eval.go` (add `Runner` that satisfies `httpapi.EvalRunner`)
- Test: `internal/eval/eval_test.go` (append)

`httpapi.EvalRunner.RunEval` takes inline JSONL bytes. The `Runner` parses it to a `Dataset`, runs the `Service`, persists the `eval_run` row (and the drift baseline), and returns the result + run id. It composes `Service` + `Store`.

- [ ] **Step 1: Write the failing test**

Append to `internal/eval/eval_test.go`:

```go
// fakeStore captures Insert + serves a baseline.
type fakeStore struct {
	inserted   InsertInput
	insertedID string
	baseline   []byte
}

func (f *fakeStore) Insert(ctx context.Context, in InsertInput) (string, error) {
	f.inserted = in
	f.insertedID = "run-1"
	return f.insertedID, nil
}
func (f *fakeStore) LatestBenchmark(ctx context.Context, kbID, ds string) ([]byte, bool, error) {
	if f.baseline == nil {
		return nil, false, nil
	}
	return f.baseline, true, nil
}
func (f *fakeStore) ListByKB(ctx context.Context, kbID string, limit int, cursor string) ([]RunRow, string, error) {
	return nil, "", nil
}

func TestRunnerRunEvalRetrieval(t *testing.T) {
	port := fakePort{hits: []ragstore.Hit{{}}}
	svc := NewService(port, stubJudgeModel{}, ServiceConfig{})
	store := &fakeStore{}
	runner := NewRunner(svc, store, 5)
	jsonl := `{"query":"q1","gold_doc_ids":["d1"],"top_k":5}`
	res, id, err := runner.RunEval(context.Background(), "kb1", "kb_kb1", KindRetrieval, []byte(jsonl))
	if err != nil {
		t.Fatal(err)
	}
	if id != "run-1" {
		t.Fatalf("id = %q", id)
	}
	if res.Retrieval == nil {
		t.Fatalf("no retrieval metrics: %+v", res)
	}
	if store.inserted.Kind != KindRetrieval || store.inserted.KBID != "kb1" {
		t.Fatalf("inserted = %+v", store.inserted)
	}
	if store.inserted.DriftJSON != nil {
		t.Fatalf("retrieval run must not set drift_json")
	}
}

func TestRunnerRunEvalDriftPersistsBaseline(t *testing.T) {
	port := fakePort{answer: ragcore.Answer{Text: "a"}}
	svc := NewService(port, stubJudgeModel{}, ServiceConfig{})
	store := &fakeStore{}
	runner := NewRunner(svc, store, 5)
	jsonl := `{"query":"q1","top_k":5}`
	res, _, err := runner.RunEval(context.Background(), "kb1", "kb_kb1", KindDrift, []byte(jsonl))
	if err != nil {
		t.Fatal(err)
	}
	if res.Drift == nil {
		t.Fatalf("no drift view: %+v", res)
	}
	// metrics_json for drift carries the scrubbed benchmark (must re-decode).
	var bench rageval.BenchmarkResult
	if err := json.Unmarshal(store.inserted.MetricsJSON, &bench); err != nil {
		t.Fatalf("drift metrics_json must be a decodable BenchmarkResult: %v (raw=%s)", err, store.inserted.MetricsJSON)
	}
	if store.inserted.DriftJSON == nil {
		t.Fatalf("drift run must set drift_json")
	}
}

func TestRunnerRejectsEmptyDataset(t *testing.T) {
	runner := NewRunner(NewService(fakePort{}, stubJudgeModel{}, ServiceConfig{}), &fakeStore{}, 5)
	_, _, err := runner.RunEval(context.Background(), "kb1", "kb_kb1", KindRetrieval, []byte("\n\n"))
	if err == nil {
		t.Fatal("empty dataset must error")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOWORK=off go test ./internal/eval/ -run Runner -v`
Expected: FAIL — `NewRunner`/`Runner` undefined.

- [ ] **Step 3: Implement Runner + LoadDataset + ListRuns**

Append to `internal/eval/eval.go`:

```go
import additions to eval.go:
  "bufio"
  "bytes"
  "strings"

// runStore is the eval_run persistence surface the Runner needs (satisfied by
// *Store). Narrowed for unit-testability; ListByKB is included so ListRuns is a
// compile-time guarantee (no runtime type assertion).
type runStore interface {
	Insert(ctx context.Context, in InsertInput) (string, error)
	LatestBenchmark(ctx context.Context, kbID, datasetName string) ([]byte, bool, error)
	ListByKB(ctx context.Context, kbID string, limit int, cursor string) ([]RunRow, string, error)
}

// Runner is the httpapi.EvalRunner implementation: parse JSONL → run → persist.
type Runner struct {
	svc         *Service
	store       runStore
	defaultTopK int
}

// NewRunner composes the eval Service + eval_run store. defaultTopK is the
// fallback Dataset.TopK when the inline JSONL omits top_k.
func NewRunner(svc *Service, store runStore, defaultTopK int) *Runner {
	if defaultTopK <= 0 {
		defaultTopK = 5
	}
	return &Runner{svc: svc, store: store, defaultTopK: defaultTopK}
}

// LoadDataset parses inline JSONL bytes into an eval.Dataset (one Example per
// line; lines beginning with // or # and blank lines skipped). TopK comes from
// the first line that carries top_k, else defaultTopK. Mirrors eval.LoadJSONL
// but reads from memory (the upload is inline, not a file path).
func (rn *Runner) LoadDataset(name string, jsonl []byte) (rageval.Dataset, error) {
	ds := rageval.Dataset{Name: name, TopK: rn.defaultTopK}
	type wire struct {
		rageval.Example
		TopK int `json:"top_k,omitempty"`
	}
	sc := bufio.NewScanner(bytes.NewReader(jsonl))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	topKSet := false
	lineNo := 0
	for sc.Scan() {
		lineNo++
		raw := strings.TrimSpace(sc.Text())
		if raw == "" || strings.HasPrefix(raw, "//") || strings.HasPrefix(raw, "#") {
			continue
		}
		var w wire
		if err := json.Unmarshal([]byte(raw), &w); err != nil {
			return rageval.Dataset{}, fmt.Errorf("eval: dataset line %d: %w", lineNo, err)
		}
		if w.TopK > 0 && !topKSet {
			ds.TopK = w.TopK
			topKSet = true
		}
		ds.Examples = append(ds.Examples, w.Example)
	}
	if err := sc.Err(); err != nil {
		return rageval.Dataset{}, fmt.Errorf("eval: scan dataset: %w", err)
	}
	if len(ds.Examples) == 0 {
		return rageval.Dataset{}, fmt.Errorf("eval: dataset is empty")
	}
	return ds, nil
}

// RunEval parses the dataset, runs the kind, persists the eval_run row, and
// returns the kb-local result + run id. Satisfies httpapi.EvalRunner.
func (rn *Runner) RunEval(ctx context.Context, kbID, namespace string, kind Kind, datasetJSONL []byte) (EvalResult, string, error) {
	ds, err := rn.LoadDataset("inline", datasetJSONL)
	if err != nil {
		return EvalResult{}, "", err
	}
	if kind == KindDrift {
		prev, _, err := rn.store.LatestBenchmark(ctx, kbID, ds.Name)
		if err != nil {
			return EvalResult{}, "", fmt.Errorf("eval: load baseline: %w", err)
		}
		res, baseline, err := rn.svc.RunDrift(ctx, RunRequest{
			KBID: kbID, Namespace: namespace, Kind: kind, Dataset: ds, PrevBenchmarkJSON: prev,
		})
		if err != nil {
			return EvalResult{}, "", err
		}
		driftJSON, err := json.Marshal(res.Drift)
		if err != nil {
			return EvalResult{}, "", fmt.Errorf("eval: marshal drift: %w", err)
		}
		// metrics_json for drift = the scrubbed BenchmarkResult (next baseline).
		id, err := rn.store.Insert(ctx, InsertInput{
			KBID: kbID, Kind: kind, DatasetName: ds.Name,
			MetricsJSON: baseline, DriftJSON: driftJSON,
		})
		if err != nil {
			return EvalResult{}, "", err
		}
		return res, id, nil
	}

	res, err := rn.svc.Run(ctx, RunRequest{KBID: kbID, Namespace: namespace, Kind: kind, Dataset: ds})
	if err != nil {
		return EvalResult{}, "", err
	}
	var metricsJSON []byte
	switch {
	case res.Retrieval != nil:
		metricsJSON, err = json.Marshal(res.Retrieval)
	case res.Generation != nil:
		metricsJSON, err = json.Marshal(res.Generation)
	default:
		metricsJSON = []byte(`{}`)
	}
	if err != nil {
		return EvalResult{}, "", fmt.Errorf("eval: marshal metrics: %w", err)
	}
	id, err := rn.store.Insert(ctx, InsertInput{
		KBID: kbID, Kind: kind, DatasetName: ds.Name, MetricsJSON: metricsJSON,
	})
	if err != nil {
		return EvalResult{}, "", err
	}
	return res, id, nil
}

// ListRuns lists eval runs for a kb (satisfies httpapi.EvalRunner).
func (rn *Runner) ListRuns(ctx context.Context, kbID string, limit int, cursor string) ([]RunRow, string, error) {
	return rn.store.ListByKB(ctx, kbID, limit, cursor)
}
```

> `runStore` includes `ListByKB`, so `ListRuns` is a plain compile-time-checked delegation (no runtime type assertion). The production `*Store` (Task 5) already implements all three methods; the unit-test `fakeStore` (Step 1) gains a trivial `ListByKB`. The gated e2e (Task 13) exercises the real list path.

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOWORK=off go test ./internal/eval/ -run Runner -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Verify the full eval package compiles + unit tests pass**

Run: `GOWORK=off go test ./internal/eval/ -v 2>&1 | grep -E "^(ok|FAIL|---)"`
Expected: the DB-free tests PASS; the gated `TestEvalRunStore` SKIPs without `LLM_AGENT_KB_PG_URL` (or PASSes with it).

- [ ] **Step 6: Commit**

```bash
git add internal/eval/eval.go internal/eval/eval_test.go
git commit -m "eval: Runner parses inline JSONL, runs, persists eval_run

drift persists the scrubbed BenchmarkResult as metrics_json (next
baseline) + the DriftView as drift_json; other kinds persist the metric
sub-struct. Satisfies httpapi.EvalRunner (§9, §5)."
```

---

## Task 12 — cmd/kbd: wire eval service + session store + new routes

**Files:**
- Modify: `cmd/kbd/main.go`

- [ ] **Step 1: Wire the deps in build()**

In `cmd/kbd/main.go`, add the imports:

```go
	kbeval "github.com/costa92/llm-agent-kb/internal/eval"
	"github.com/costa92/llm-agent-kb/internal/sessions"
```

In `build`, after `retrievalSvc := retrieval.New(...)`, add:

```go
	// M4: Q&A history + eval. The session repo backs both the ask-path recorder
	// and the read endpoints; the eval Runner composes the eval Service (over the
	// same RagPort) + the eval_run store.
	sessionRepo := sessions.New(st.Pool())
	retrievalSvc.SetRecorder(sessionRepo)

	evalSvc := kbeval.NewService(rag, rag.JudgeModel(), kbeval.ServiceConfig{
		MaxAskTokens:         cfg.MaxAskTokens,
		GlobalMaxCommunities: cfg.GlobalMaxCommunities,
		DriftRounds:          cfg.DriftRounds,
		DriftTopK:            cfg.DriftTopK,
	})
	evalRunner := kbeval.NewRunner(evalSvc, kbeval.NewStore(st.Pool()), cfg.EvalDefaultTopK)
```

> `rag` here is the `*ragsvc.Service` (it satisfies `kbeval.Port` via Retrieve/Ask/AskGlobal/AskDrift and exposes `JudgeModel()`). Confirm the local var name in `build` is `rag` (it is, per the M3 wiring).

In the `httpapi.NewMux(httpapi.Deps{...})` literal, add:

```go
		EvalRunner:            evalRunner,
		SessionReader:         sessionRepo,
		EvalRunsPerUserMinute: cfg.MaxEvalRunsPerUserPerMinute,
```

- [ ] **Step 2: Verify it builds + whole-module vet**

Run: `GOWORK=off go build ./... && GOWORK=off go vet ./...`
Expected: PASS. If `*ragsvc.Service` does not satisfy `kbeval.Port`, the build fails with a clear "does not implement" message — it should satisfy it (Retrieve added in Task 3; Ask/AskGlobal/AskDrift exist since M3).

- [ ] **Step 3: Run the full DB-free unit suite**

Run: `GOWORK=off go test ./... 2>&1 | grep -E "^(ok|FAIL|----)" `
Expected: all packages `ok` or `(cached)`; gated tests SKIP without the PG URL. No `FAIL`.

- [ ] **Step 4: Commit**

```bash
git add cmd/kbd/main.go
git commit -m "cmd/kbd: wire eval Runner + session repo + M4 routes

session repo backs the ask-path recorder and the read endpoints; eval
Runner composes the eval Service (over the ragsvc RagPort + JudgeModel)
and the eval_run store."
```

---

## Task 13 — gated e2e: ask→session→eval→drift over the full server

**Files:**
- Modify: `cmd/kbd/main_test.go`

Extend the existing e2e flow with M4 assertions. A NEW test function (do not perturb the existing two) drives: login→org→kb→upload→ready→ask (assert a session was created + the message persisted via GET /sessions)→POST /eval/run (retrieval, tiny inline JSONL)→GET /eval/runs (assert the run stored)→a second eval/run (drift) → assert the drift run stored. Over-provision the scripted LLM; tolerate degenerate metrics (assert structural: 200s + rows stored).

- [ ] **Step 1: Add the cleanDB tables + the new test**

In `cmd/kbd/main_test.go`, extend the `cleanDB` drop list to include the M4 tables. `cleanDB` already loops over its table slice with a single `pool.Exec(ctx, "DROP TABLE IF EXISTS "+tbl+" CASCADE")` (verified on-disk) — so just adding the names inherits `CASCADE`, which is required since `qa_message` FKs `qa_session ON DELETE CASCADE` (and dropping `qa_session` before `qa_message` would otherwise fail). Add (after `"ingest_job",`):

```go
		"qa_message", "qa_session", "eval_run",
```

Append the new gated test:

```go
// TestEvalAndSessionsEndToEnd drives the M4 quality surface on live pgvector:
// login → org → kb → upload paste → poll ready → ask (hybrid) → GET /sessions
// (assert the ask created a session + persisted the pair) → GET /sessions/{sid}
// (assert 2 messages) → POST /eval/run (retrieval, tiny inline JSONL) → GET
// /eval/runs (assert ≥1 run) → POST /eval/run (drift) → GET /eval/runs (assert
// the drift run stored). Engine correctness is rag's own concern; this proves
// the HTTP + auth + persistence wiring. The scripted model is over-provisioned
// so the judge/ask cursors never exhaust.
func TestEvalAndSessionsEndToEnd(t *testing.T) {
	dsn := os.Getenv("LLM_AGENT_KB_PG_URL")
	if dsn == "" {
		t.Skipf("set LLM_AGENT_KB_PG_URL (pgvector) to run the M4 eval/sessions e2e")
	}
	ctx := context.Background()
	cleanDB(t, ctx, dsn)

	// One scripted Generate cursor backs ask answers + the LLM judge JSON.
	// Over-provision generously (ingest with GRAPH_ENABLED=false consumes none;
	// ask + each eval example + each drift example each draw one).
	responses := []llm.Response{}
	for i := 0; i < 40; i++ {
		responses = append(responses, llm.Response{Text: `{"groundedness":0.9,"answer_relevance":0.9}`})
	}
	model := llm.NewScriptedLLM(llm.WithResponses(responses...))
	providerOverride = func(config.Config) (llm.ChatModel, llm.Embedder, error) {
		return model, llm.NewScriptedLLM(llm.WithEmbedDimensions(8)), nil
	}
	t.Cleanup(func() { providerOverride = nil })

	cfg, err := config.LoadFromLookup(func(k string) (string, bool) {
		switch k {
		case "PG_URL":
			return dsn, true
		case "EMBEDDING_DIM":
			return "8", true
		case "JWT_SECRET":
			return "test-secret", true
		case "GRAPH_ENABLED":
			return "false", true // eval here is retrieval+drift over the local path
		}
		return "", false
	})
	if err != nil {
		t.Fatal(err)
	}
	handler, cleanup, err := build(ctx, cfg)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	defer cleanup()
	srv := httptest.NewServer(handler)
	defer srv.Close()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	hash, err := password.Hash("pw")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authzstore.New(pool).CreateUser(ctx, "m4@x.com", hash); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	client := srv.Client()
	do := func(method, path, bearer, body string) (int, map[string]any) {
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		if m == nil {
			m = map[string]any{"_raw": string(raw)}
		}
		return resp.StatusCode, m
	}

	code, body := do("POST", "/api/auth/login", "", `{"Email":"m4@x.com","Password":"pw"}`)
	if code != http.StatusOK {
		t.Fatalf("login code=%d body=%v", code, body)
	}
	token, _ := body["access_token"].(string)

	code, body = do("POST", "/api/orgs", token, `{"name":"Acme"}`)
	if code != http.StatusOK {
		t.Fatalf("create org code=%d body=%v", code, body)
	}
	orgID, _ := body["id"].(string)

	code, body = do("POST", "/api/orgs/"+orgID+"/kbs", token, `{"name":"Docs","embeddingDim":8}`)
	if code != http.StatusOK {
		t.Fatalf("create kb code=%d body=%v", code, body)
	}
	kbID, _ := body["id"].(string)

	code, body = do("POST", "/api/kb/"+kbID+"/documents", token,
		`{"title":"Doc","sourceType":"paste","content":"the quick brown fox jumps over the lazy dog repeatedly"}`)
	if code != http.StatusAccepted {
		t.Fatalf("upload code=%d body=%v", code, body)
	}
	docID, _ := body["documentId"].(string)
	ready := false
	for i := 0; i < 50; i++ {
		code, listBody := do("GET", "/api/kb/"+kbID+"/documents", token, "")
		if code == http.StatusOK && hasReadyDoc(listBody, docID) {
			ready = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ready {
		t.Fatalf("document never ready")
	}

	// Ask (hybrid) → assert a session id comes back.
	code, body = do("POST", "/api/kb/"+kbID+"/ask", token, `{"q":"fox","mode":"hybrid","topK":5}`)
	if code != http.StatusOK {
		t.Fatalf("ask code=%d body=%v", code, body)
	}
	sid, _ := body["sessionId"].(string)
	if sid == "" {
		t.Fatalf("ask returned no sessionId: %v", body)
	}

	// GET /sessions → assert the session is listed.
	code, body = do("GET", "/api/kb/"+kbID+"/sessions", token, "")
	if code != http.StatusOK {
		t.Fatalf("list sessions code=%d body=%v", code, body)
	}
	sessItems, _ := body["items"].([]any)
	if len(sessItems) == 0 {
		t.Fatalf("no sessions persisted: %v", body)
	}

	// GET /sessions/{sid} → assert 2 messages (user + assistant).
	code, body = do("GET", "/api/kb/"+kbID+"/sessions/"+sid, token, "")
	if code != http.StatusOK {
		t.Fatalf("transcript code=%d body=%v", code, body)
	}
	msgs, _ := body["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("transcript msgs = %d, want 2: %v", len(msgs), body)
	}

	// POST /eval/run (retrieval) → 200 + runId. Tiny inline JSONL dataset.
	evalBody := `{"kind":"retrieval","dataset":"{\"query\":\"fox\",\"gold_doc_ids\":[\"` + docID + `\"],\"top_k\":5}"}`
	code, body = do("POST", "/api/kb/"+kbID+"/eval/run", token, evalBody)
	if code != http.StatusOK {
		t.Fatalf("eval/run retrieval code=%d body=%v", code, body)
	}
	if body["runId"] == nil || body["runId"] == "" {
		t.Fatalf("eval/run returned no runId: %v", body)
	}

	// GET /eval/runs → assert ≥1 stored run.
	code, body = do("GET", "/api/kb/"+kbID+"/eval/runs", token, "")
	if code != http.StatusOK {
		t.Fatalf("eval/runs code=%d body=%v", code, body)
	}
	runItems, _ := body["items"].([]any)
	if len(runItems) == 0 {
		t.Fatalf("no eval runs stored: %v", body)
	}

	// POST /eval/run (drift) twice so the second has a baseline; assert both 200.
	driftBody := `{"kind":"drift","dataset":"{\"query\":\"fox\",\"top_k\":5}"}`
	if code, body = do("POST", "/api/kb/"+kbID+"/eval/run", token, driftBody); code != http.StatusOK {
		t.Fatalf("eval/run drift#1 code=%d body=%v", code, body)
	}
	if code, body = do("POST", "/api/kb/"+kbID+"/eval/run", token, driftBody); code != http.StatusOK {
		t.Fatalf("eval/run drift#2 code=%d body=%v", code, body)
	}

	// GET /eval/runs → assert the drift run(s) are stored (≥3 total now).
	code, body = do("GET", "/api/kb/"+kbID+"/eval/runs?limit=100", token, "")
	if code != http.StatusOK {
		t.Fatalf("eval/runs#2 code=%d body=%v", code, body)
	}
	runItems, _ = body["items"].([]any)
	if len(runItems) < 3 {
		t.Fatalf("eval runs = %d, want ≥3 (1 retrieval + 2 drift): %v", len(runItems), body)
	}
	// Assert a drift run carries the drift envelope.
	foundDrift := false
	for _, it := range runItems {
		m, _ := it.(map[string]any)
		if m["kind"] == "drift" && m["drift"] != nil {
			foundDrift = true
		}
	}
	if !foundDrift {
		t.Fatalf("no drift run with a drift payload: %v", runItems)
	}
}
```

> Note on the eval-run guard: the default `MaxEvalRunsPerUserPerMinute` is 5; this test issues 3 eval runs, comfortably under the budget, so no 429.

- [ ] **Step 2: Run the gated e2e (requires the PG URL)**

Run: `GOWORK=off go test ./cmd/kbd/ -run TestEvalAndSessionsEndToEnd -v`
Expected: PASS with `LLM_AGENT_KB_PG_URL` set; SKIP without it.

- [ ] **Step 3: Run the WHOLE gated suite to confirm no cross-test table contention**

Run: `GOWORK=off go test ./... -v 2>&1 | grep -E "^(=== RUN|--- (PASS|FAIL|SKIP)|ok|FAIL)" | tail -60`
Expected: all PASS or SKIP; no FAIL. (Each gated test owns + drops its tables, so they are co-runnable serially.)

- [ ] **Step 4: Commit**

```bash
git add cmd/kbd/main_test.go
git commit -m "e2e: ask→session→eval→drift over the full kbd server

asserts the ask path persists a session + message pair, retrieval eval
stores a run, and two drift runs store with a drift payload. Structural
assertions only; engine correctness is rag's concern (§13 M4 E2E)."
```

---

## Task 14 — final verification + tag v0.4.0

**Files:** none (verification + release)

- [ ] **Step 1: Full build + vet + DB-free test**

Run:
```bash
GOWORK=off go build ./... && GOWORK=off go vet ./... && GOWORK=off go test ./...
```
Expected: build clean, vet clean, all packages `ok` (gated tests SKIP without the PG URL).

- [ ] **Step 2: Full gated suite (with pgvector up)**

Run (with `LLM_AGENT_KB_PG_URL` exported per the Gated test DB block):
```bash
GOWORK=off go test ./... -count=1
```
Expected: all `ok` including `internal/eval`, `internal/sessions`, `cmd/kbd` gated tests. Note in the PR/commit body: "Gated suite (eval_run/qa_session/qa_message repos + ask→session→eval→drift e2e) green on pgvector v0.8.0 / postgres 16."

- [ ] **Step 3: Confirm go.mod is unchanged (no version bump, no tidy)**

Run:
```bash
git diff --stat go.mod go.sum
```
Expected: NO changes to `go.mod` (eval is a sub-package of the already-required `rag v1.11.0`). If `go.sum` gained a line for `github.com/costa92/llm-agent-rag/eval`, that is the only acceptable delta — keep it. If `go.mod` changed, you ran `tidy` by mistake; `git checkout go.mod` and re-verify with `GOWORK=off go build ./...` (which adds only the needed `go.sum` entry).

- [ ] **Step 4: Merge + tag v0.4.0 (mirror M3 Task 11)**

```bash
cd llm-agent-kb && git checkout main && \
git merge --no-ff feat/m4-eval -m "M4 quality: eval(retrieval/triad/global/drift) + dashboard data + sessions + quota hardening + E2E" && \
git tag -a v0.4.0 -m "llm-agent-kb v0.4.0 — M4 (eval use case over rag's four evaluators + drift compare, eval_run/qa_session/qa_message persistence, 4 new endpoints, per-user eval-run guard, ask→session→eval→drift E2E)" && \
git push origin main --tags
```
Expected: branch merged, `v0.4.0` tagged + pushed.

> If the project convention is PR-based (M2/M3 used a feature branch + PR), open a PR instead of a local merge: `gh pr create --base main --head feat/m4-eval`. Note `gh pr edit/view` may fail (token lacks `read:org`); edit the PR title/body via `gh api -X PATCH repos/costa92/llm-agent-kb/pulls/N`. Tag after merge. The replace-guard pre-commit hook strips local `costa92` replaces — `go.mod` has none, so the hook is a no-op; do NOT `--no-verify`.

---

## Self-Review

**1. Spec coverage (§13 M4 = eval(retrieval/triad/global/drift) + dashboard + 配额/限流硬化 + E2E):**
- eval retrieval/triad/global/drift → Task 6 (`Service.Run` builds all four rag evaluators) + Task 11 (`Runner` parses+persists). ✓
- drift compare vs previous stored BenchmarkResult → Task 6 `runDrift` + Task 11 (`LatestBenchmark` baseline) + Task 5 store. ✓
- §9 drift dashboard renders `HistogramDelta` → Task 2 `DriftView.Histograms []HistogramDeltaView` projected from `eval.DriftReport.Histograms` (clean int-bucket struct, no NaN) in `driftView`; covered by the extended `TestDriftViewNaNBecomesNull`. ✓
- `eval_run` table + repo (insert/list/latest), exact `metrics_json`/`drift_json` shapes per §5 → Task 5 + Task 8. metrics_json: retrieval=MetricsView, triad/global=GenerationView, drift=scrubbed BenchmarkResult; drift_json=DriftView. Documented in Task 5/Task 11. ✓
- `qa_session`+`qa_message` + persistence on ask path → Task 7 + Task 9 + Task 8. The three production ask handlers thread `KBID`/`UserID`(from ctx)/`SessionID`(from body `sessionId`) into AskInput/GlobalInput/DriftInput → Task 10 Step 4b (without this the persisted session is orphaned and the Task 13 e2e fails). ✓
- Endpoints POST /eval/run (editor+), GET /eval/runs (viewer+), GET /sessions (viewer+), GET /sessions/{sid} (viewer+) with `{items,next_cursor}` → Task 10. ✓
- quota/ratelimit hardening (separate per-user eval-run guard) → Task 1 (knob) + Task 10 (`evalGuard`). ✓
- gated E2E ask→session→eval→drift → Task 13. ✓
- §16.5 PII/injection: spec says "启用 rag 的注入/PII 防护 (sanitizeHits)" which is inside rag's `Ask` (already exercised since M1/M3 via `Answer.Diagnostics`); M4 adds NO new guard wiring — correctly out of scope, noted in the header. ✓
- Tag prep mirrors M3 Task 11 → Task 14 (merge/tag v0.4.0, `gh api -X PATCH` note, replace-guard note, no version bump verification). ✓
- §4 boundary decision (internal/eval as 2nd permitted rag/eval importer, kb-local DTOs outward) → header + Task 2/4. ✓

**2. Placeholder scan:** No "TBD"/"implement later"/"add error handling"/"similar to Task N". Every code step shows complete, final code. Task 6 builds `Run` (Step 3), `runDrift`/`RunDrift` (Step 3, two-return signature so the Runner gets the next baseline), and `marshalBaseline` (Step 4) directly — no draft-then-replace detour. `authzhttp.UserID`/`WithUserID` was verified on-disk (only `UserID` exists), so Task 10's helper is now concrete (no fallback branch). One remaining judgement-call note (local var name `rag` in Task 12) instructs a grep-verify before writing — acceptable since it depends on earlier-task surface, and it gives the fallback. ✓

**3. Type consistency:**
- `Kind` constants (`KindRetrieval`/`KindTriad`/`KindGlobal`/`KindDrift`) — defined Task 2, used Tasks 5/6/10/11/13 consistently. ✓
- `EvalResult{Kind, DatasetName, Retrieval *MetricsView, Generation *GenerationView, Drift *DriftView}` — Task 2; consumed Task 6/11. ✓
- `MetricsView`/`GenerationView`/`DriftView` JSON tags match the handler serialization (Task 10 emits `json.RawMessage(row.MetricsJSON)`, so field tags only matter for the live `result` echo — consistent). ✓
- `Store.Insert(InsertInput{KBID,Kind,DatasetName,MetricsJSON,DriftJSON})` — Task 5; called Task 11 with the same fields; `fakeStore` in Task 11 implements the `runStore` subset (Insert + LatestBenchmark). ✓
- `RagPort.Retrieve(ctx, query, ragcore.SearchOptions) ([]ragstore.Hit, error)` — Task 3; `Port` interface in Task 4 mirrors it; `*ragsvc.Service` satisfies `kbeval.Port` (Task 12). ✓
- `Service.JudgeModel() raggenerate.Model` — Task 3; passed to `kbeval.NewService(rag, rag.JudgeModel(), ...)` in Task 12; `eval.LLMJudge{Model: s.judge}` consumes it (Task 6). ✓
- `retrieval.Recorder` (EnsureSession/AppendPair) — Task 9; `*sessions.Repo` satisfies it (Task 7 method names match exactly: `EnsureSession(ctx,kbID,userID,sessionID,firstQuestion)`, `AppendPair(ctx,sessionID,question,answer,citationsJSON,mode)`). ✓
- `httpapi.EvalRunner` (RunEval/ListRuns) ↔ `*eval.Runner` (Task 11) — signatures match: `RunEval(ctx,kbID,namespace,Kind,[]byte) (EvalResult,string,error)`, `ListRuns(ctx,kbID,limit,cursor) ([]RunRow,string,error)`. ✓
- `httpapi.SessionReader` (ListByKB/Transcript) ↔ `*sessions.Repo` (Task 7) — match. ✓
- `RunRow{ID,Kind,DatasetName,MetricsJSON,DriftJSON,CreatedAt}` — Task 5; serialized in Task 10 `listRunsHandler`. ✓

**Open questions / risks (could not fully resolve from source):**
1. **`AnswerBenchmark.Run` for drift draws one judge Generate per example** — the e2e (Task 13) over-provisions 40 scripted responses and uses GRAPH_ENABLED=false, so the single cursor should suffice for 1 ask + 1 retrieval-eval (no judge — retrieval evaluator does not judge) + 2 drift runs (1 example each → 1 judge call each). Risk: if `AnswerBenchmark` also calls the model for the *answer* (it does — `Asker.Ask` runs the full pipeline), each drift example draws ≥2 Generate calls (ask + judge). 40 is a generous margin but verify the cursor count empirically on first gated run; bump the loop count if a drift run 500s with a cursor-exhausted error.
2. **`authzhttp.WithUserID` is NOT exported** (verified against `llm-agent-authz@v0.1.0`: `httpapi` exports only `UserID(ctx) string`) — resolved in Task 10: the DB-free handler tests set no uid and call `h(w, req)` directly; the unlimited guard (`limits.New(0)`) ignores the empty key. No `withUID` helper, no `authzhttp` import in the test. Affects only the DB-free handler test, not production.
3. **Drift baseline semantics** — M4 stores the *scrubbed* (NaN→0) BenchmarkResult as the next baseline so it re-decodes. Consequence: ExactMatch/F1Token deltas read 0/"unchanged" when gold answers are absent (the common kb case — datasets carry gold *doc/chunk ids*, not gold answer strings). The two judge legs (MeanGroundedness/MeanAnswerRelevance) are the meaningful drift signal, which is finite whenever the judge ran. This is a deliberate, documented simplification consistent with §9's "drift uses Direction improved/regressed/unchanged" — not a gap, but worth a reviewer's eye if richer textual drift is wanted later (would need datasets with `gold_answers`).
4. **`eval_run`/`qa_session` not FK'd to `knowledge_base`** (Task 8) — history outlives a deleted kb in v1 (§16.4 cascade covers chunks/graph only). If product wants history purged on kb-delete, add a cascade step in the `deleteKBHandler` (out of M4 scope; noted in Task 8).

## Execution Handoff

**Plan complete and saved to `docs/superpowers/plans/2026-06-10-llm-agent-kb-m4.md`. Two execution options:**

**1. Subagent-Driven (recommended)** - dispatch a fresh subagent per task, review between tasks, fast iteration.

**2. Inline Execution** - execute tasks in this session using executing-plans, batch execution with checkpoints.

**Which approach?**
