# Testing Patterns

**Analysis Date:** 2026-05-21

This document describes testing as actually practiced across the six
subprojects. The umbrella ecosystem has no top-level test target of its
own — `make test` is a thin wrapper that delegates to each subproject's
`go test ./...`. CI happens at the sub-repo level plus one umbrella
cross-repo integration job.

## Test Framework

**Runner:**
- Stdlib `testing` only. No `stretchr/testify`, no `ginkgo`, no
  `gomega` in the core or in any hand-written test file (the few
  `stretchr/testify` lines visible in `llm-agent-providers/go.mod`
  are *indirect* — pulled in by `testcontainers-go`).
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
    `llm-agent-rag/postgres/postgres_test.go:18-19`,
    `llm-agent-flow/cmd/flowd/server/server_crud_test.go:40-74`).

**Run Commands:**
```bash
# All-repos (delegates to each subproject):
make test                        # umbrella Makefile → scripts/eco.sh test all

# Per-repo:
cd <subproject> && GOWORK=off go test ./...

# Subset (selecting subprojects):
make test TARGETS=llm-agent,llm-agent-rag,llm-agent-flow

# Single test by name:
go test -run TestSimpleAgent_Run_TransparentlyForwards ./...

# Snapshot baseline update (rag and flow both expose -update):
go test ./internal/apisnapshot/ -run TestAPISnapshot -update

# Live integration (gated by build tag + env):
go test -tags ollama_live -run TestGenerate_Ollama_Live ./internal/contract/...
go test -tags llmagent ./adapter/...     # llm-agent-rag adapter slice
LLM_AGENT_RAG_PG_URL=postgres://... go test ./postgres/...
```

## Test File Inventory

The umbrella has **153 `*_test.go` files** outside `third_party/` and
`.git/` (was 108 at the v1.1 close — the +45 reflects v1.2 Phase 35
budget tests in core, otelflow + flowrunner additions, and the entire
`llm-agent-flow` test surface). Per subproject:

| Subproject | Test files | Notes |
|---|---|---|
| `llm-agent` | 42 | core paradigm tests + builtin tools + orchestrate + `budget/` (v1.2 Phase 35 added the cross-paradigm budget tests) |
| `llm-agent-rag` | 59 (the largest surface — every package has `*_test.go`) | rag, retrieve, store, ingest, embed, generate, prompt, pack, rerank, graph, eval, agentic, advanced, adapter/llmagent, contract, examples, feedback, guard, obs, pack, postgres, prompt, store, tree, internal/apisnapshot |
| `llm-agent-otel` | 8 | `exporters_test.go`, `semconv_gen_ai_test.go`, plus one per wrapper subpackage: `otelagent`, `otelmetrics`, `otelmodel`, `otelrag`, `otelslog`, **`otelflow`** (new at v0.2.2) |
| `llm-agent-providers` | 8 | one per adapter: `anthropic`, `deepseek`, `minimax`, `ollama`, `openai`, plus `internal/contract/{generate_test.go,main_test.go,ollama_live_test.go}` |
| `llm-agent-customer-support` | 12 | `cmd/server/main_test.go`, `compose/assets_test.go`, `internal/{app,config,guardrails,httpapi,knowledgebase,limits,sessionstore,supportflow}/*_test.go`, plus **`internal/flowrunner/flowrunner_test.go`** (new at v0.2.3) |
| `llm-agent-flow` | **24** | flow library + flowd HTTP server + sqlite store + tools + cond/cel + examples + internal/apisnapshot (whole new repo since v1.1 close) |

### `llm-agent-flow/` test surface in detail

The 24 test files fall into four layers:

| Layer | Files | Coverage |
|---|---|---|
| **Library — flow engine** | `flow/engine_test.go` (113 LOC), `flow/engine_parallel_test.go` (252), `flow/engine_cond_test.go` (275), `flow/ir_test.go` (63), `flow/runner_test.go` (27), `flow/validate_test.go` (77) | engine topological exec; per-layer parallel exec; conditional edges + activation algorithm; Flow/Node/Edge JSON round-trip; `Runner` interface compile-pin |
| **Library — tools, cond, store** | `flow/tools/{exec,http,manifest}_test.go`, `flow/cond/cel/{cel,helpers}_test.go`, `flow/store/sqlite/{store,events,events_batch}_test.go` | tool manifest loading; CEL adapter; SQLite Flow/Run CRUD; event append (single + bulk batch) |
| **HTTP layer** | `cmd/flowd/server/{server,server_crud,server_events,server_replay,auth,lru}_test.go` | full REST surface via `httptest.NewServer`; bearer-token auth (8 cases); LRU engine cache (5 cases); per-event audit + replay |
| **Examples + gate** | `examples/{echo_chain,http_tool,router}/example_test.go`, `internal/apisnapshot/apisnapshot_test.go` | runnable godoc examples; API snapshot gate (see below) |

## Test Structure

**Naming:** `Test<Type>_<Behavior>` is the dominant convention.
Examples: `TestSimpleAgent_Run_TransparentlyForwards`,
`TestReActAgent_HappyPath_FinalAfterOneAction`,
`TestRegistry_RegisterDuplicate_ReturnsErr`,
`TestFunctionCallAgent_RunsToolCallsInParallel`,
`TestAuthMissingHeaderIs401`,
`TestEngineCacheLRUEviction`,
`TestAppendRunEventsBatchHappyPath`,
`TestReplayRunReproducesEvents`.

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
    t.Run(tc.name, func(t *testing.T) { ... })
}
```

`llm-agent-flow` uses the same idiom heavily; e.g.
`engine_cond_test.go` exercises the activation algorithm via 6
condition shapes (`always`, `never`, `==<lit>`, `!=<lit>`, `bad`,
`boom`) declared as a `[]struct{...}` and iterated with `t.Run`.

**Table-driven:** the canonical form is `tests := []struct{...}` (or
`cases := []struct{...}`), iterated with `for _, tc := range tests`.
Used in `llm-agent/react_test.go`,
`llm-agent-customer-support/internal/app/app_test.go`,
`llm-agent/llm/llm_test.go`,
`llm-agent-providers/internal/contract/generate_test.go`,
`llm-agent-rag/...` (multiple files), `llm-agent/budget_integration_test.go`,
and across `llm-agent-flow` (e.g.
`flow/engine_parallel_test.go`, `cmd/flowd/server/server_crud_test.go`).

**`Example*` tests:** runnable godoc examples with `// Output:`
blocks. Located at `llm-agent/example_simple_test.go`,
`example_tool_use_test.go`, `example_multi_agent_test.go`,
`llm-agent/pkg/fanout/example_test.go`, and (new)
`llm-agent-flow/examples/{echo_chain,http_tool,router}/example_test.go`.

**`t.Parallel()`:** observed sparingly. `llm-agent/function_call_test.go`,
selected goroutine-leak-sensitive tests in
`llm-agent-flow/flow/engine_parallel_test.go`. Tests are otherwise
serial — fast and deterministic, no need to opt into parallelism.

**`t.Helper()`:** used in fixture / assertion helpers
(`llm-agent/examples/06-budget/main_test.go:60-67`,
`llm-agent-rag/internal/apisnapshot/apisnapshot_test.go:20-27`,
`llm-agent-flow/internal/apisnapshot/apisnapshot_test.go`,
`llm-agent-flow/cmd/flowd/server/server_crud_test.go:40-74`).

**`t.Cleanup`** is the favored teardown idiom:
- `t.Cleanup(srv.Close)` in `flowd` `httptest.NewServer` setups
  (`cmd/flowd/server/server_test.go:19-40`).
- `t.Cleanup(func() { _ = sr.Close() })` for streamed readers
  (`llm-agent/llm/llm_test.go:55`).

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

`llm-agent-flow/flow/runner.go` carries the same idiom in production
code for the seam `otelflow.Wrap` consumes:

```go
var _ flow.Runner = (*Engine)(nil)
```

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
`llm-agent/scriptedllm_test.go` defines an *unexported* sibling
`scriptedLLM` plus `textResp` helper, scoped to the `agents` package's
test files.

**HTTP-level mocks (provider adapters + flowd):**
`net/http/httptest.NewServer` + `http.HandlerFunc` is the canonical
network-mocking pattern.

- **Provider adapters:** every adapter has one — see
  `llm-agent-providers/openai/openai_test.go:60-101`,
  `llm-agent-providers/anthropic/anthropic_test.go`,
  `llm-agent-providers/ollama/ollama_test.go`,
  `llm-agent-providers/deepseek/deepseek_test.go`,
  `llm-agent-providers/minimax/minimax_test.go`. The handler asserts
  request shape (URL, method, headers, body fragments) and returns
  canned JSON; `sync/atomic` counters check call counts.

- **flowd HTTP server (new pattern):**
  `llm-agent-flow/cmd/flowd/server/server_test.go:19-40` wraps the
  server-under-test directly:

  ```go
  func newTestServer(t *testing.T) *httptest.Server {
      reg := flow.NewNodeRegistry()
      _ = flow.RegisterToolNode(reg)
      engine, err := flow.LoadCompile(...)
      ...
      return httptest.NewServer(server.NewMux(engine, nil))
  }
  ```

  `server_crud_test.go:40-74` extends the pattern with the full
  `Server` (CRUD + persistence):

  ```go
  func newStoreServer(t *testing.T, opts ...serverOption) (*httptest.Server, flowstore.Store) {
      store, err := sqlitestore.Open(":memory:")
      ...
      srv, err := server.New(server.Config{Store: store, Registry: reg, ...})
      return httptest.NewServer(srv.Handler()), store
  }
  ```

  This is **end-to-end-HTTP testing without booting the flowd binary** —
  the same handler chain a real `cmd/flowd` invocation builds, exercised
  in-process by Go's stdlib HTTP test machinery.

**In-memory SQLite for store tests:**
A new pattern in this analysis. `llm-agent-flow/flow/store/sqlite/store_test.go`
opens `sqlite.Open(":memory:")` and exercises the full schema /
migrations / CRUD path without touching disk:

```go
// flow/store/sqlite/store_test.go:14
s, err := sqlite.Open(":memory:")
```

The `:memory:` DSN is **documented as part of the Store API contract**
(`flow/store/sqlite/doc.go:20`, `flow/store/sqlite/open.go:28`); the
implementation sets `SetMaxOpenConns(1)` for `:memory:` connections so
the connection-per-test pattern stays race-free under the default
journal mode (`events.go:17`). On-disk SQLite users get WAL + row-level
locks; in-memory tests get serialized single-conn behavior.

`flowd` HTTP tests stack this with `httptest.NewServer` — every CRUD /
event / replay / auth test uses `:memory:` for total isolation
(`cmd/flowd/server/server_crud_test.go:42`,
`server_crud_test.go:369`). No `testdata/` SQL fixtures; the schema
is regenerated on every `Open` call.

**OTel test harness (`llm-agent-otel` + downstream):**
- `tracetest.NewInMemoryExporter()` + `sdktrace.NewTracerProvider(WithSyncer(...))`
  builds an in-memory span collector. Each `otelmodel`/`otelrag`/
  `otelagent`/`otelflow` test fetches `exp.GetSpans()` and asserts
  span name, attributes, events, and status.
  See `llm-agent-otel/otelmodel/otelmodel_test.go:16-21` and the
  newer `llm-agent-otel/otelflow/otelflow_test.go:14-17`.
- The same in-memory exporter pattern is used by
  `llm-agent-customer-support/internal/flowrunner/flowrunner_test.go`
  to assert that the end-to-end customer-support → otelflow → flow
  trace path emits the expected spans.

**Stub-evaluator pattern (new in flow):**
`llm-agent-flow/flow/engine_cond_test.go:1-80` introduces a deterministic
in-package `stubEvaluator` that exercises the conditional-edge
activation algorithm **without pulling cel-go into the core test
binary**:

```go
// flow/engine_cond_test.go:13-22
type stubEvaluator struct{}
// Supports: "==<lit>", "!=<lit>", "always", "never", "bad", "boom"

func (stubEvaluator) Compile(expr string) (Condition, error) { ... }
```

The cel-go integration lives in `flow/cond/cel/` (separate sub-package
with its own tests at `flow/cond/cel/{cel,helpers}_test.go`). The
core engine package tests cover edge-firing semantics via the stub —
fast, dependency-free, and isolated from cel-go quirks. cel-go is
still in `go.mod` (transitive), but the core engine test binary does
not link it.

**Tool fixtures:**
- `recordingTool`, `counterTool` / `failingTool` / `panicTool` /
  `slowTool`, `upperTool` / `reverseTool`, `fixedTool` (across
  `llm-agent/{react,async,chain,registry}_test.go`).
- `passthroughTool` and `fakeTool` for flow tests
  (`llm-agent-flow/flow/engine_cond_test.go:62-79`,
  `llm-agent-otel/otelflow/otelflow_test.go:38-50`).

## Fixtures and Factories

**Test data:**
- `llm-agent-providers/internal/contract/testdata/` holds per-provider
  scenario fixtures: subdirectories `anthropic/`, `ollama/`, `openai/`.
  Used by `TestGenerate_Conformance` to replay request/response pairs
  for happy paths, 401, 429, 500, 529.
- `llm-agent-rag/api/v1.snapshot.txt` is the committed exported-API
  baseline for rag.
- `llm-agent-flow/api/v0.1.snapshot.txt` (207 lines) is the committed
  exported-API baseline for flow.
- No other `testdata/` directories detected across the umbrella.

**Factories:**
- Provider conformance uses a `ChatModelFactory` map keyed by provider
  name (`generate_test.go:20-64`) so the same test body exercises all
  three adapters via the same scenario set.
- `llm-agent-customer-support/internal/app` uses dependency-injection
  options (`WithModelFactory`, `WithEmbedderFactory`,
  `WithSessionStoreFactory`, `WithTracerProviderFactory`) so tests
  build a real `App` with scripted models.
- `llm-agent-flow/cmd/flowd/server` uses a `serverOption` functional-
  options pattern in the test helpers
  (`cmd/flowd/server/server_crud_test.go:40-74`) so individual tests
  can mix in `withAuth("sekret")`, custom store, custom registry, etc.,
  without duplicating the boilerplate.

## API snapshot gate (now in two repos)

Both `llm-agent-rag` and `llm-agent-flow` carry an in-tree snapshot
gate that runs **as part of `go test ./...`** — no separate workflow
step, no extra tool to install. Pure stdlib (`go/parser` +
`go/printer`).

**How it works:**

1. The package (`internal/apisnapshot/`) walks the module's source
   under `go/parser`, collects every exported declaration (`const`,
   `var`, `func`, `type`, struct fields, interface methods), and
   renders a deterministic text representation sorted by package +
   kind + name.
2. `TestAPISnapshot` compares the rendered text against the committed
   baseline (`api/v1.snapshot.txt` for rag, `api/v0.1.snapshot.txt`
   for flow).
3. Any rename, removal, or re-sign of an exported symbol changes the
   render, fails the test, and shows up as a diff in code review next
   to the source diff.
4. Deliberate additive changes regenerate the baseline via
   `go test ./internal/apisnapshot/ -run TestAPISnapshot -update`.

**Why this is "go test"-level instead of a separate CI step:**
A workflow-level gate that runs `go run ./tools/snapshot` is fine but
requires (a) the tool to exist as a binary, (b) the workflow to know
to call it, and (c) reviewers to remember it's a gate. A gate that
fires inside `go test ./...` means *every* developer running `go test`
locally hits it, the local + CI behaviors are identical, and there is
no separate binary to keep in sync. The flow snapshot's own docstring
calls this out: *"The gate is pure stdlib (`go/parser` + `go/printer`)
— no module dependency, no separate tool to install, runs in every
`go test`."*

**Skip rules:**
- Files in `internal/` are excluded (non-importable externally).
- `_test.go` files are excluded (not part of the public surface).
- Build-tagged files (e.g. `//go:build llmagent`) **are included** —
  `go/parser` ignores build constraints, so the build-tagged
  `adapter/llmagent` slice in `llm-agent-rag` is covered. This was a
  deliberate decision to prevent build-tagged code from drifting
  un-policed (`llm-agent-rag/internal/apisnapshot/apisnapshot.go:18-23`).

## Integration vs Unit Split

**Unit (the overwhelming majority):**
- Live in the same package as the code under test (Go default).
- Run in-process with `ScriptedLLM` / `scriptedLLM` / `recordingTool`
  / `stubEvaluator` fixtures. No network, no docker.
- Default `go test ./...` runs them all.

**Integration:**
- `llm-agent/budget_integration_test.go` — cross-paradigm uniformity
  test for the budget chokepoint (35-04 / CC-1). Runs as part of
  normal `go test ./...`.
- `llm-agent/examples/06-budget/main_test.go` — end-to-end demo
  smoke test in the side `examples/` module.
- `llm-agent-rag/internal/apisnapshot/apisnapshot_test.go` and
  `llm-agent-flow/internal/apisnapshot/apisnapshot_test.go` — API
  freeze gates; run in default `go test ./...`.
- `llm-agent-flow/cmd/flowd/server/*_test.go` — **httptest end-to-end
  flowd**: every test boots a `*httptest.Server` wrapping the full
  handler chain (mux + auth + store + engine cache + per-event
  persistence), exercises real HTTP via `http.Get` / `http.Post`,
  and asserts on response bodies + headers (`X-Run-ID`, `X-Replay`,
  `WWW-Authenticate`). All persistence goes to `:memory:` SQLite.
  Runs in default `go test ./...`.
- `llm-agent-customer-support/internal/flowrunner/flowrunner_test.go`
  — exercises `flowrunner.Runner` against a real `flow.Engine` and a
  real OTel `tracetest.NewInMemoryExporter`. Tests the full
  customer-support → otelflow → flow trace path. Runs in default
  `go test ./...`.
- `llm-agent-rag/postgres/postgres_conformance_test.go` — `t.Skipf`
  unless `LLM_AGENT_RAG_PG_URL` is set (real Postgres + pgvector
  required). Conformance is opt-in.
- `llm-agent-rag/adapter/llmagent/{model,tool}_test.go` — build-
  tagged `//go:build llmagent`; the *only* slice that imports the
  core `github.com/costa92/llm-agent`. Run via
  `go test -tags llmagent ./adapter/...`.
- `llm-agent-providers/internal/contract/ollama_live_test.go` —
  build-tagged `//go:build ollama_live`; spins a real Ollama
  container via `testcontainers-go`. Runs only in the nightly
  workflow.

## CI Test Execution

**Every per-repo `test.yml` runs with `GOWORK=off`** (INFRA-02). The
core gauntlet is identical across the five sister repos plus
`llm-agent-flow`:

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
  runs the same gauntlet inside `examples/`.
- `llm-agent-rag/.github/workflows/test.yml` adds:
  - "Verify core packages do not import llm-agent" `rg`-based gate.
  - Explicit `API snapshot gate` step (visibility only — already
    runs inside `go test ./...`).
  - `Build-tagged adapter (llmagent)` step
    (`go build -tags llmagent ./...`, then
    `go test -tags llmagent ./adapter/...`).
- `llm-agent-flow/.github/workflows/test.yml` is the minimal canonical
  form — four-step gauntlet only. The API snapshot gate runs implicitly
  inside `go test ./...` (the test lives in `internal/apisnapshot/`).
- `llm-agent-customer-support/.github/workflows/test.yml` adds a
  pre-build `format` job (`gofmt` drift), a `compose` validate job,
  and a `docker` release-image build.
- `llm-agent-providers/.github/workflows/nightly-ollama-live.yml`
  runs only on schedule.

**Umbrella cross-repo job** (`.github/workflows/umbrella.yml`):
Checks out all five non-flow sister repos at their default branches
and runs the same `GOWORK=off go vet/build/test ./... -count=1` for
each one. **`llm-agent-flow` is NOT yet checked out by this workflow**
(the workflow file has not been updated since the flow repo was
introduced — see CONCERNS.md).

## Coverage Tools / Reports

**No coverage targets exist anywhere in the tree.** `make test` runs
plain `go test ./...` with no `-cover`, `-coverprofile`, or
`-covermode` flag. The umbrella `Makefile` and `scripts/eco.sh test`
do not pass coverage flags either.

Coverage is implicitly tracked through:
1. Strict CI on every PR (`go test ./...` must pass everywhere).
2. The "every public wrapper has a paired test" convention in
   `llm-agent-otel` (now including `otelflow`).
3. The cross-paradigm `budget_integration_test.go` and the
   `paradigmCase` table.
4. The API snapshot gate forces tests to be regenerated alongside
   surface changes (otherwise the snapshot drifts and the gate
   trips).

## Test Patterns: Quick Reference

**Subtests vs flat tests:**
- Subtests when the same arrange/act/assert applies across a fixture
  set (provider matrix, scenario matrix, paradigm matrix, condition
  matrix in `engine_cond_test`).
- Flat tests when each scenario has its own setup.

**Setup helpers:**
- Per-file `func newTestServer(t *testing.T) *httptest.Server` etc.,
  declared at file top (e.g. `flowd/server/server_test.go:19-40`).
- `t.Helper()` inside helpers so `t.Fatalf` reports the caller's line.

**Cleanup:**
- `t.Cleanup(srv.Close)` and `defer resp.Body.Close()` in HTTP tests
  (every flowd test file).
- `t.Cleanup(func() { _ = sr.Close() })` for streamed readers.
- `defer cancel()` and `defer mu.Unlock()` everywhere.

**Error-path patterns:**
- Always use `errors.Is`, never string-match:
  ```go
  if !errors.Is(err, budget.ErrCallsExceeded) {
      t.Fatalf("err = %v, want ErrCallsExceeded", err)
  }
  if !errors.Is(err, flowstore.ErrNotFound) {
      t.Fatalf("err = %v, want ErrNotFound", err)
  }
  ```
- For the budget chokepoint, assert BOTH the dimensional sentinel and
  the umbrella sentinel.

**Async/streaming tests:**
- Drain channels with `for range` to assert close.
- Use `sync/atomic.Int32` counters in HTTP mocks to count requests.
- SSE assertions in flowd use `bufio.Scanner` over the response body
  (`cmd/flowd/server/server_replay_test.go:36-40`); test asserts on
  `event:` / `data:` prefix lines and on `X-Replay: true` header.

## Gaps

What is **not** tested at the umbrella level:

1. **No `go test` at the umbrella root.** The umbrella has no `go.mod`
   of its own; `make test` just iterates subprojects. There is no
   ecosystem-level integration test that spans modules and lives in
   the umbrella tree. The closest thing is
   `.github/workflows/umbrella.yml`, but that simply re-runs each
   subproject's `go test ./...`.
2. **`llm-agent-flow` is missing from the umbrella CI workflow.**
   `.github/workflows/umbrella.yml` checks out five sister repos but
   not flow; flow's tests run only via its own per-repo `test.yml`.
   A cross-repo integration regression touching flow + otelflow +
   flowrunner would not be caught by the umbrella job.
3. **No coverage measurement.** No `-cover` flag is ever set; no
   `coverage.txt` is uploaded anywhere; no `codecov`/`coveralls`
   integration is configured.
4. **No fuzz tests.** `go test -fuzz` is not invoked in any CI
   workflow inspected; no `FuzzXxx` functions detected at scan time.
5. **No benchmark gates.** `llm-agent/bench/bench_test.go` exists
   (the only `bench/` directory found) but no CI step runs
   `go test -bench`. v0.1.1's "perf" tag on the engine-cache + event-
   batching change has no benchmark in CI — the gains are asserted
   in the CHANGELOG, not in a tracked benchmark.
6. **Conformance against real provider APIs is opt-in only.**
   - `ollama_live` runs nightly (real container).
   - OpenAI / Anthropic / DeepSeek / MiniMax have *no* live-tag
     equivalent — only `httptest.NewServer`-based mock servers.
     Real API drift would not be caught until a downstream user
     reports it.
7. **No mutation testing, no property-based testing.** Determinism
   is achieved through ScriptedLLM, stub-evaluators, and `:memory:`
   SQLite — not through stochastic input generation.
8. **Postgres conformance is opt-in via env var.** PR CI does not
   spin a Postgres container; only developers with
   `LLM_AGENT_RAG_PG_URL` set will exercise
   `llm-agent-rag/postgres/`.
9. **No end-to-end stack test in the umbrella.** The
   `llm-agent-customer-support` repo has its own `compose` validate
   + Docker build, but no test brings up the full stack (Ollama +
   OTel collector + Grafana + the support service) and exercises a
   real request-response cycle.
10. **Auth tests cover correctness, not security depth.** The 8 cases
    in `flowd/server/auth_test.go` cover header parsing, bypass, 401
    vs 403 mapping, and constant-time comparison. They do **not**
    cover rate-limiting (no rate limiter exists), audit logging (no
    audit log exists — `/runs/{id}/events` is the only persistent
    log and only fires for runs, not for auth events), or
    brute-force protection. See CONCERNS.md.

---

*Testing analysis: 2026-05-21*
