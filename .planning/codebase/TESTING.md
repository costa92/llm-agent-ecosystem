# Testing Patterns

**Analysis Date:** 2026-05-20

This document describes testing as actually practiced across the five
subprojects. The umbrella ecosystem has no top-level test target of its
own — `make test` is a thin wrapper that delegates to each subproject's
`go test ./...`. CI happens at the sub-repo level plus one umbrella
cross-repo integration job.

## Test Framework

**Runner:**
- Stdlib `testing` only. No `stretchr/testify`, no `ginkgo`, no
  `gomega` in the core or in any test file inspected (the few
  `stretchr/testify` lines visible in `llm-agent-providers/go.mod`
  are *indirect* — pulled in by `testcontainers-go`, not used by
  hand-written tests).
- Go toolchain: `go 1.26.0` pinned in every `go.mod`.
- One module imports `go.uber.org/goleak` (v1.3.0) for goroutine-leak
  detection: `llm-agent-providers/internal/contract/main_test.go`
  uses `goleak.VerifyTestMain(m)` inside `TestMain`.

**Assertion Library:**
- Hand-written `t.Errorf` / `t.Fatalf` / `t.Helper()` everywhere. No
  third-party assertion DSL.
- Common idioms:
  - `if got != want { t.Errorf("...= %q, want %q", got, want) }`
  - `if !errors.Is(err, sentinel) { t.Fatalf("err = %v, want %v", err, sentinel) }`
  - `if !reflect.DeepEqual(got, want) { ... }` for whole structs
  - `t.Helper()` in test-local fixture functions
    (`llm-agent/examples/06-budget/main_test.go:60-67`,
    `llm-agent-rag/postgres/postgres_test.go:18-19`).

**Run Commands:**
```bash
# All-repos (delegates to each subproject):
make test                        # umbrella Makefile → scripts/eco.sh test all

# Per-repo:
cd <subproject> && GOWORK=off go test ./...

# Subset (selecting subprojects):
make test TARGETS=llm-agent,llm-agent-rag

# Single test by name:
go test -run TestSimpleAgent_Run_TransparentlyForwards ./...

# Snapshot baseline update (llm-agent-rag only):
go test ./internal/apisnapshot/ -run TestAPISnapshot -update

# Live integration (gated by build tag + env):
go test -tags ollama_live -run TestGenerate_Ollama_Live ./internal/contract/...
go test -tags llmagent ./adapter/...     # llm-agent-rag adapter slice
LLM_AGENT_RAG_PG_URL=postgres://... go test ./postgres/...
```

## Test File Inventory

The umbrella has **108 `*_test.go` files** outside `third_party/` and
`.git/`. Per subproject:

| Subproject | Test files | Notes |
|---|---|---|
| `llm-agent` | 31 (16 in package root, plus `budget/`, `builtin/`, `bench/`, `comm/{a2a,anp,mcp,common}`, `context/`, `examples/06-budget/`, `llm/`, `memory/`, `orchestrate/`, `pkg/fanout/`, `rl/`) | core paradigm tests + builtin tools + orchestrate |
| `llm-agent-rag` | 56 (largest test surface — every package has `*_test.go`) | rag, retrieve, store, ingest, embed, generate, prompt, pack, rerank, graph, eval, agentic, advanced, adapter/llmagent, contract, examples, feedback, guard, obs, pack, postgres, prompt, store, tree, internal/apisnapshot |
| `llm-agent-otel` | 7 (`exporters_test.go`, `semconv_gen_ai_test.go`, plus one per wrapper subpackage: `otelagent`, `otelmetrics`, `otelmodel`, `otelrag`, `otelslog`) | every public wrapper has paired tests |
| `llm-agent-providers` | 7 (one per adapter: `anthropic`, `deepseek`, `minimax`, `ollama`, `openai`, plus `internal/contract/{generate_test.go,main_test.go,ollama_live_test.go}`) | conformance + adapter-local |
| `llm-agent-customer-support` | 11 (`cmd/server/main_test.go`, `compose/assets_test.go`, `internal/{app,config,guardrails,httpapi,knowledgebase,limits,sessionstore,supportflow}/*_test.go`) | reference-service tests |

### `llm-agent/` top-level test files (package `agents`)

Paradigm coverage (one test file per paradigm + helpers):

| File | Coverage |
|---|---|
| `agent_test.go` | compile-time interface assertions for all 5 agents; sentinel-error `errors.Is` matrix; `StepKind` constants |
| `agent_chatmodel_test.go` | `agents` ↔ `llm.ChatModel` bridge |
| `simple_test.go` | `SimpleAgent` happy path, empty input, RunStream, ctx cancel, OnStep, budget exhaustion |
| `react_test.go` | `ReActAgent` Thought/Action/Observation loop + native tool-call path; table-driven cases |
| `reflection_test.go` | `ReflectionAgent` gen→critique→revise + budget gate (35-04 / CC-1) |
| `plan_solve_test.go` | `PlanAndSolveAgent` plan + step exec; budget gate |
| `function_call_test.go` | `FunctionCallAgent` no-tool-call, parallel tool calls, unknown-tool, budget |
| `chain_test.go` | `Chain` piping, empty-chain error, satisfies `Tool` |
| `async_test.go` | `AsyncRunner` parallel exec, ctx cancellation, panic recovery; counterTool / failingTool / panicTool / slowTool fixtures |
| `registry_test.go` | `Registry` register, duplicate, sorted list, `AsLLMTools`, `PromptDescription` |
| `tool_test.go` | `Tool` interface helpers, `NewFuncTool` |
| `scriptedllm_test.go` | the in-package `scriptedLLM` mock + `textResp` helper |
| `budget_integration_test.go` | **cross-paradigm uniformity** — every paradigm propagates identical sentinel shape under MaxCalls (CC-1) |
| `example_simple_test.go` | runnable godoc `ExampleSimpleAgent` (deterministic `// Output:` block) |
| `example_tool_use_test.go` | runnable godoc tool-use example |
| `example_multi_agent_test.go` | runnable godoc multi-agent example |

## Test Structure

**Naming:** `Test<Type>_<Behavior>` is the dominant convention.
Examples: `TestSimpleAgent_Run_TransparentlyForwards`,
`TestReActAgent_HappyPath_FinalAfterOneAction`,
`TestRegistry_RegisterDuplicate_ReturnsErr`,
`TestFunctionCallAgent_RunsToolCallsInParallel`,
`TestChatOnlyMockExcludesCapabilities`.

**Suite Organization:** flat — no `setUp`/`tearDown`, no shared
state. Each `Test...` function is self-contained:

```go
// llm-agent/simple_test.go:12-29
func TestSimpleAgent_Run_TransparentlyForwards(t *testing.T) {
    llmMock := newScriptedLLM(textResp("hello world"))
    a := NewSimpleAgent(llmMock, SimpleOptions{SystemPrompt: "you are helpful"})

    res, err := a.Run(context.Background(), "hi")
    if err != nil {
        t.Fatalf("Run: %v", err)
    }
    if res.Answer != "hello world" {
        t.Errorf("Answer = %q", res.Answer)
    }
    ...
}
```

**Subtests (`t.Run`):** used where the same Arrange/Act/Assert applies
across a fixture set:

```go
// llm-agent-customer-support/internal/app/app_test.go:22-53
tests := []struct {
    name     string
    provider string
    model    string
}{
    {name: "openai", provider: config.ProviderOpenAI, model: "gpt-4o-mini"},
    {name: "anthropic", provider: config.ProviderAnthropic, model: "claude-3-5-haiku-20241022"},
    {name: "ollama", provider: config.ProviderOllama, model: "llama3.1:8b"},
}
for _, tc := range tests {
    t.Run(tc.name, func(t *testing.T) {
        model, err := DefaultModelFactory(config.Config{...})
        ...
    })
}
```

Files using `t.Run` (at least): `simple_test.go` (4), `react_test.go`
(1), `budget_integration_test.go` (6), `function_call_test.go` (6),
`example_simple_test.go`, `example_tool_use_test.go`,
`llm-agent-customer-support/internal/app/app_test.go`,
`llm-agent-rag/internal/apisnapshot/apisnapshot_test.go`,
`llm-agent/llm/llm_test.go`, `llm-agent-providers/internal/contract/generate_test.go`.

**Table-driven:** the canonical form is `tests := []struct{...}` (or
`cases := []struct{...}`), iterated with `for _, tc := range tests`.
Used in `llm-agent/react_test.go`, `llm-agent-customer-support/internal/app/app_test.go`,
`llm-agent/llm/llm_test.go`, `llm-agent-providers/internal/contract/generate_test.go`,
`llm-agent-rag/...` (multiple files), and the new
`budget_integration_test.go` "paradigmCase" sets — see
`budget_integration_test.go:29-100` for a representative paradigm-
factory table.

**`Example*` tests:** runnable godoc examples with `// Output:`
blocks. Located at `llm-agent/example_simple_test.go`,
`example_tool_use_test.go`, `example_multi_agent_test.go`, and
`llm-agent/pkg/fanout/example_test.go`. They double as godoc and as
deterministic smoke tests.

**`t.Parallel()`:** observed in `llm-agent/function_call_test.go`
only. Tests are otherwise serial — fast and deterministic, no
need to opt into parallelism.

**`t.Helper()`:** used in fixture / assertion helpers
(`llm-agent/examples/06-budget/main_test.go:60-67`,
`llm-agent-rag/internal/apisnapshot/apisnapshot_test.go:20-27`,
`llm-agent-rag/postgres/postgres_test.go:18`).

**Compile-time assertions (test side):**
```go
// llm-agent/agent_test.go:10-16
var (
    _ Agent = (*SimpleAgent)(nil)
    _ Agent = (*ReActAgent)(nil)
    _ Agent = (*ReflectionAgent)(nil)
    _ Agent = (*PlanAndSolveAgent)(nil)
    _ Agent = (*FunctionCallAgent)(nil)
)
```

These compile-time `var _ Iface = (*Impl)(nil)` patterns guarantee that
the interface compatibility is checked even if no test body covers it.

## Mocking

**Framework:** none. Mocks are hand-written stdlib types.

**The canonical mock — `llm.ScriptedLLM`:**
A deterministic full-capability ChatModel implementation in production
code at `llm-agent/llm/scripted.go`. Implements `ChatModel` +
`ToolCaller` + `Embedder` + `StructuredOutputs`. Construction is
functional-options:

```go
// llm-agent/llm/scripted.go:50-64
m := llm.NewScriptedLLM(
    llm.WithProvider("scripted"),
    llm.WithModel("test-1"),
    llm.WithCapabilities(llm.Capabilities{Tools: true, Embeddings: true}),
    llm.WithResponses(
        llm.TextResponse("hello"),
        llm.ToolCallResponse("calc", `{"a":2,"b":3}`),
    ),
)
```

Concurrency-safe (cursor guarded by `sync.Mutex`). Returns
`llm.ErrScriptExhausted` (wrapped via `fmt.Errorf("scripted: %w", ...)`)
when the response list is exhausted, so tests can assert via
`errors.Is`.

**The agents-package twin — `scriptedLLM`:**
`llm-agent/scriptedllm_test.go` (lines 1-72) defines an *unexported*
sibling `scriptedLLM` plus `textResp` helper, scoped to the `agents`
package's test files. Used by every paradigm test inside the
package to avoid pulling in the public `llm.ScriptedLLM` builder.
Same shape as the public one — `sync.Mutex`, cursor, capability info
— deliberately simpler.

**HTTP-level mocks (provider adapters):**
`net/http/httptest.NewServer` + `http.HandlerFunc` is the only
network-mocking pattern. Every provider adapter has one — see
`llm-agent-providers/openai/openai_test.go:60-101`,
`llm-agent-providers/anthropic/anthropic_test.go`,
`llm-agent-providers/ollama/ollama_test.go`,
`llm-agent-providers/deepseek/deepseek_test.go`,
`llm-agent-providers/minimax/minimax_test.go`. The handler asserts
request shape (URL, method, headers, body fragments) and returns
canned JSON; `sync/atomic` counters check call counts.

**OTel test harness (`llm-agent-otel`):**
- `tracetest.NewInMemoryExporter()` + `sdktrace.NewTracerProvider(WithSyncer(...))`
  builds an in-memory span collector. Each `otelmodel`/`otelrag`/
  `otelagent` test fetches `exp.GetSpans()` and asserts span name,
  attributes, events, and status.
  See `llm-agent-otel/otelmodel/otelmodel_test.go:16-21`.
- `go.opentelemetry.io/otel/trace/noop` is used when a test wires an
  OTel-dependent component but doesn't want to assert on spans —
  e.g. `llm-agent-customer-support/internal/app/app_test.go:18`.

**Tool fixtures in `agents` tests:**
Local recording tools per file (none shared across files):
- `recordingTool` (`react_test.go:14-27`) records `Execute` args.
- `counterTool` / `failingTool` / `panicTool` / `slowTool`
  (`async_test.go:14-65`).
- `upperTool` / `reverseTool` (`chain_test.go:11-36`).
- `fixedTool` (`registry_test.go`, referenced).

**Stdout capture:**
`llm-agent/examples/06-budget/main_test.go:21-58` shows the
`os.Pipe()` + `os.Stdout = w` + goroutine `io.ReadAll(r)` pattern for
capturing `main()` output and asserting on transcript fragments.

## Fixtures and Factories

**Test data:**
- `llm-agent-providers/internal/contract/testdata/` holds per-provider
  scenario fixtures: subdirectories `anthropic/`, `ollama/`, `openai/`.
  Used by `TestGenerate_Conformance` to replay request/response pairs
  for happy paths, 401, 429, 500, 529 (`generate_test.go:66-100`).
- `llm-agent-rag/api/v1.snapshot.txt` is the committed exported-API
  baseline; `TestAPISnapshot` regenerates and diffs against it.
- No other `testdata/` directories detected across the umbrella.

**Factories:**
- Provider conformance uses a `ChatModelFactory` map keyed by provider
  name (`generate_test.go:20-64`) so the same test body exercises all
  three adapters via the same scenario set.
- `llm-agent-customer-support/internal/app` uses dependency-injection
  options (`WithModelFactory`, `WithEmbedderFactory`,
  `WithSessionStoreFactory`, `WithTracerProviderFactory`) so tests
  build a real `App` with scripted models — see
  `app_test.go:66-86`.

## Integration vs Unit Split

**Unit (the overwhelming majority):**
- Live in the same package as the code under test (Go default).
- Run in-process with `ScriptedLLM` / `scriptedLLM` / `recordingTool`
  fixtures. No network, no docker.
- Default `go test ./...` runs them all.

**Integration:**
- `llm-agent/budget_integration_test.go` (filename-tagged "integration"
  but in-process) — exercises every agent paradigm against the
  shared budget chokepoint. See header comment lines 1-15: "cross-
  paradigm uniformity test for the budget chokepoint (35-04 /
  CC-1)". Runs as part of normal `go test ./...`.
- `llm-agent/examples/06-budget/main_test.go` — end-to-end demo
  smoke test. Lives in the side `examples/` module so the core test
  suite stays minimal; CI builds and tests `examples/` separately
  (`llm-agent/.github/workflows/test.yml:32-49`).
- `llm-agent-rag/internal/apisnapshot/apisnapshot_test.go` — API
  freeze gate; runs in default `go test ./...`.
- `llm-agent-rag/postgres/postgres_conformance_test.go` and
  `postgres_test.go` — `t.Skipf` unless `LLM_AGENT_RAG_PG_URL` is
  set (real Postgres + pgvector required). Conformance is opt-in.
- `llm-agent-rag/adapter/llmagent/{model,tool}_test.go` — build-
  tagged `//go:build llmagent`; the *only* slice that imports the
  core `github.com/costa92/llm-agent`. Run via
  `go test -tags llmagent ./adapter/...` (the
  `llm-agent-rag` CI workflow runs this explicitly as the last step).
- `llm-agent-providers/internal/contract/ollama_live_test.go` —
  build-tagged `//go:build ollama_live`; spins a real Ollama
  container via `testcontainers-go` (`testcontainers/testcontainers-go/modules/ollama`).
  Runs only in the nightly workflow
  (`llm-agent-providers/.github/workflows/nightly-ollama-live.yml`)
  with `go test -v -timeout 30m -tags ollama_live -run TestGenerate_Ollama_Live`.

## CI Test Execution

**Every per-repo `test.yml` runs with `GOWORK=off`** (INFRA-02). The
core gauntlet is identical across the four sister repos:

```yaml
env:
  GOWORK: off
steps:
  - uses: actions/checkout@v4
  - uses: actions/setup-go@v5
    with:
      go-version-file: go.mod
      cache: true
  - name: go mod tidy (drift check)
  - name: go vet
  - name: go build
  - name: go test
```

**Variations:**
- `llm-agent/.github/workflows/test.yml` adds an `examples` block that
  runs the same gauntlet inside `examples/` (lines 32-49) and
  retains `-count=1` only inside the umbrella job.
- `llm-agent-rag/.github/workflows/test.yml` adds:
  - "Verify core packages do not import llm-agent" `rg`-based gate
    (lines 24-39).
  - Explicit `API snapshot gate` step (visibility only — already
    runs inside `go test ./...`).
  - `Build-tagged adapter (llmagent)` step
    (`go build -tags llmagent ./...`, then
    `go test -tags llmagent ./adapter/...`).
- `llm-agent-customer-support/.github/workflows/test.yml` adds a
  pre-build `format` job (`gofmt` drift), a `compose` validate job
  (`docker compose -f compose/compose.yaml config`), and a `docker`
  release-image build (`docker build -f compose/Dockerfile .`).
- `llm-agent-providers/.github/workflows/nightly-ollama-live.yml`
  runs only on schedule (`cron: '0 3 * * *'`) and
  `workflow_dispatch`, with model-cache and the live-tag test.

**Umbrella cross-repo job** (`.github/workflows/umbrella.yml`):
Checks out all five repos at their default branches and runs the same
`GOWORK=off go vet/build/test ./... -count=1` for each one. This is the
cross-repo integration safety net — catches breakages that pass each
repo's solo CI but fail the umbrella.

## Coverage Tools / Reports

**No coverage targets exist anywhere in the tree.** `make test` runs
plain `go test ./...` with no `-cover`, `-coverprofile`, or
`-covermode` flag. The umbrella `Makefile` and `scripts/eco.sh test`
do not pass coverage flags either.

`.gitignore` reserves `coverage.txt` and `coverage.html` paths
(`llm-agent/.gitignore`), but no committed CI step generates them.

Coverage is implicitly tracked through:
1. Strict CI on every PR (`go test ./...` must pass everywhere).
2. The "every public wrapper has a paired test" convention in
   `llm-agent-otel`.
3. The cross-paradigm `budget_integration_test.go` and the
   `paradigmCase` table — adding a new agent paradigm forces a new
   row, which forces test coverage of the chokepoint.

## Test Patterns: Quick Reference

**Subtests vs flat tests:**
- Subtests when the same arrange/act/assert applies across a fixture
  set (provider matrix, scenario matrix, paradigm matrix).
- Flat tests when each scenario has its own setup.

**Setup helpers:**
- Per-file `func testConfig() (Config, *exporter)` etc., declared at
  file top (e.g. `otelmodel_test.go:16-21`).
- `t.Helper()` inside helpers so `t.Fatalf` reports the caller's line.

**Cleanup:**
- `t.Cleanup(func() { _ = sr.Close() })` for streamed readers
  (`llm-agent/llm/llm_test.go:55`).
- `defer cancel()` and `defer mu.Unlock()` everywhere.

**Error-path patterns:**
- Always use `errors.Is`, never string-match:
  ```go
  if !errors.Is(err, budget.ErrCallsExceeded) {
      t.Fatalf("err = %v, want ErrCallsExceeded", err)
  }
  ```
- For the budget chokepoint, assert BOTH the dimensional sentinel and
  the umbrella sentinel
  (`budget_integration_test.go` and per-paradigm files).

**Async/streaming tests:**
- Drain channels with `for range` to assert close:
  ```go
  // simple_test.go:64-66
  ch, _ := a.RunStream(ctx, "x")
  cancel()
  for range ch { } // drain — channel must close, not deadlock
  ```
- Use `sync/atomic.Int32` counters in HTTP mocks to count requests.

## Gaps

What is **not** tested at the umbrella level:

1. **No `go test` at the umbrella root.** The umbrella has no `go.mod`
   of its own; `make test` just iterates subprojects. There is no
   ecosystem-level integration test that spans modules and lives in
   the umbrella tree. The closest thing is
   `.github/workflows/umbrella.yml`, but that simply re-runs each
   subproject's `go test ./...`.
2. **No coverage measurement.** No `-cover` flag is ever set; no
   `coverage.txt` is uploaded anywhere; no `codecov`/`coveralls`
   integration is configured.
3. **No fuzz tests.** `go test -fuzz` is not invoked in any CI
   workflow inspected; no `FuzzXxx` functions detected at scan time.
4. **No benchmark gates.** `llm-agent/bench/bench_test.go` exists
   (the only `bench/` directory found) but no CI step runs
   `go test -bench`. Performance regressions would land silently.
5. **Conformance against real provider APIs is opt-in only.**
   - `ollama_live` runs nightly (real container).
   - OpenAI / Anthropic / DeepSeek / MiniMax have *no* live-tag
     equivalent — only `httptest.NewServer`-based mock servers.
     Real API drift (e.g. OpenAI changes its tool-call schema) would
     not be caught until a downstream user reports it.
6. **No mutation testing, no property-based testing.** Determinism
   is achieved through ScriptedLLM, not through stochastic input
   generation.
7. **Postgres conformance is opt-in via env var.** PR CI does not
   spin a Postgres container; only developers with
   `LLM_AGENT_RAG_PG_URL` set will exercise
   `llm-agent-rag/postgres/`.
8. **No end-to-end stack test in the umbrella.** The
   `llm-agent-customer-support` repo has its own `compose` validate
   + Docker build, but no test brings up the full stack (Ollama +
   OTel collector + Grafana + the support service) and exercises a
   real request-response cycle.

---

*Testing analysis: 2026-05-20*
