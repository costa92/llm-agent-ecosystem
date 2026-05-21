<!-- refreshed: 2026-05-21 -->
# Architecture

**Analysis Date:** 2026-05-21

## System Overview

```text
┌─────────────────────────────────────────────────────────────────────┐
│              llm-agent-ecosystem (umbrella, this repo)              │
│  `go.work` + `Makefile` + `scripts/eco.sh` + `scripts/workspace.sh` │
│  Coordination only — no product code lives here.                    │
└──────────────────────────────┬──────────────────────────────────────┘
                               │ orchestrates 6 sibling repos
        ┌──────────────────────┴──────────────────────┐
        ▼                                              ▼
┌──────────────────────────────┐    ┌──────────────────────────────────────┐
│ llm-agent-customer-support   │    │ llm-agent-otel                       │
│ Deployable demo service      │    │ OTel decorator wrappers              │
│ `cmd/server/main.go`         │    │ `otelmodel/`, `otelagent/`,          │
│ `internal/{app,httpapi,      │    │ `otelrag/`, `otelmetrics/`,          │
│   flowrunner,…}/`            │    │ `otelslog/`, `otelflow/`             │
│ `compose/compose.yaml`       │    │                                      │
└─────┬──────┬──────┬──────────┘    └──────────┬───────────┬───────────────┘
      │      │      │ uses                     │ wraps     │ wraps flow.Runner
      │      │      ▼                          ▼           ▼
      │      │  ┌──────────────────┐    ┌──────────────────────────────┐
      │      │  │ llm-agent-flow   │◀───│ llm-agent (core)             │
      │      │  │ Flow IR + DAG    │    │ Stdlib-only framework        │
      │      │  │ executor         │    │ `agent.go`, `tool.go`,       │
      │      │  │ `flow/`,         │    │ `registry.go`, `react.go`,   │
      │      │  │ `flow/cond/cel`, │    │ `plan_solve.go`, …           │
      │      │  │ `flow/store/     │    │ `llm/` (ChatModel, …)        │
      │      │  │   sqlite`,       │    │ `memory/`, `orchestrate/`,   │
      │      │  │ `cmd/flow`,      │    │ `budget/`, `pkg/fanout/`,    │
      │      │  │ `cmd/flowd`      │    │ `rag/` (facade)              │
      │      │  └────────┬─────────┘    └──────────────┬───────────────┘
      │      │           │ uses                        │ RAG facade
      │      ▼           ▼                             ▼
      │  ┌──────────────────────────────┐    ┌──────────────────────────────┐
      │  │ llm-agent-providers          │    │ llm-agent-rag (v1.x frozen)  │
      │  │ Real provider adapters       │◀───│ `rag/`, `ingest/`,           │
      │  │ `openai/`, `anthropic/`,     │    │ `retrieve/`, `generate/`,    │
      │  │ `ollama/`, `deepseek/`,      │    │ `store/`, `embed/`,          │
      │  │ `minimax/`                   │    │ `graph/`, `prompt/`, `pack/`,│
      │  └──────────────┬───────────────┘    │ `postgres/`, `obs/`, `guard/`│
      │                 │                    │ stdlib-only at v1.0.x        │
      │                 │ implements         └──────────────────────────────┘
      │                 │ `llm.ChatModel`,
      │                 ▼ `llm.ToolCaller`, `llm.Embedder`
      └─────────────────┘
```

## Component Responsibilities

| Component | Responsibility | Path |
|-----------|----------------|------|
| Umbrella shell | go.work generation, multi-repo build / test / up / down, dependency-currency gate | `/home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/` |
| `llm-agent` (v0.5.1) | Agent paradigms, Tool/Registry, Memory, `llm.ChatModel` + capability interfaces, typed streaming events, async tool execution, RAG facade | `llm-agent/` |
| `llm-agent-rag` (v1.0.2) | Import/retrieve/generate primitives, GraphRAG (Louvain communities, DRIFT, path-ranked subgraphs), pgvector backend, `rag.Observer` hook; v1.x frozen surface via `internal/apisnapshot` + `api/v1.snapshot.txt` | `llm-agent-rag/` |
| `llm-agent-providers` (v0.2.2) | Real OpenAI / Anthropic / Ollama / DeepSeek / MiniMax adapters; each implements the relevant `llm.*` capability interfaces | `llm-agent-providers/` |
| `llm-agent-otel` (v0.2.2) | `otelmodel.Wrap(ChatModel)`, `otelagent.Wrap(Agent)`, `otelrag.Wrap(System)`, **`otelflow.Wrap(flow.Runner)`** (new), low-cardinality metrics helpers, slog bridge, OTLP exporter wiring | `llm-agent-otel/` |
| `llm-agent-customer-support` (v0.2.3) | HTTP + SSE service, StateGraph triage, RAG knowledge lookup, durable session store, day-one guardrails, observability stack via compose, **`internal/flowrunner/`** bridge to `llm-agent-flow` | `llm-agent-customer-support/` |
| `llm-agent-flow` (v0.1.1) | Serializable flow IR + DAG executor; library `flow/` is stdlib-only outside the back-edge to `llm-agent`; `cmd/flowd` HTTP service with SQLite-backed CRUD + run history + replay + bearer auth | `llm-agent-flow/` |

## Pattern Overview

**Overall:** Umbrella + independently-versioned sibling repos. The umbrella is a coordination shell; product code lives in 6 sibling Git repos pinned together by `go.work` (local dev) and by `go.mod require` (release builds). Three keystone decisions define the cross-repo contract surface; a fourth (K3 — decorator) is now reused by `otelflow` to compose over `llm-agent-flow`.

**Key Characteristics:**
- **Core stays stdlib-only.** `llm-agent/go.mod` lists exactly one non-stdlib require — `github.com/costa92/llm-agent-rag v1.0.1` — and nothing else. Every other non-stdlib dep is in a sister repo a caller opts into one `go get` at a time. `llm-agent-flow/flow/` (the library package) follows the same rule: stdlib-only outside the back-edge to `llm-agent`.
- **Each subproject is a separate GitHub repo with its own tags, branch, CI, and release cycle.** The umbrella's `scripts/eco.sh` clones the missing siblings on `make bootstrap` (now 6 repos); `scripts/workspace.sh` writes the local `go.work` over 6 modules.
- **Dependency direction is acyclic and one-way.** `customer-support → providers + otel + flow → llm-agent → llm-agent-rag`. `otel → flow` is the new edge introduced in `llm-agent-otel v0.2.2` (the `otelflow/` sub-package). `customer-support → flow` is the new edge introduced in `llm-agent-customer-support v0.2.3` (the `internal/flowrunner/` sub-package). `llm-agent-rag` is the fixed point every other repo aligns *to*.
- **`replace` directives are a local-only escape hatch.** The release-precheck CI gate refuses to tag a commit whose `go.mod` carries a `replace`. `go.work` is `.gitignore`d in every sister repo and CI runs with `GOWORK=off`.
- **Two SemVer-frozen surfaces.** `llm-agent-rag` v1.x has been additive-only since v1.0.0 (gate: `internal/apisnapshot` + `api/v1.snapshot.txt`). `llm-agent-flow` v0.1.x adopted the same pattern at v0.1.0 — committed `docs/compatibility.md` promise, `internal/apisnapshot` gate, `api/v0.1.snapshot.txt` baseline. Both gates are pure stdlib (`go/parser` + `go/printer`) and run on every `go test`.

## Why each subproject is a separate repo

- **Independent versioning.** `llm-agent v0.5.1`, `llm-agent-rag v1.0.2`, `llm-agent-otel v0.2.2`, `llm-agent-providers v0.2.2`, `llm-agent-customer-support v0.2.3`, `llm-agent-flow v0.1.1` — each cuts its own SemVer track. A breaking RAG API change can go to `llm-agent-rag/v2` without forcing a core bump; the same applies to the flow JSON IR (any field removal would go to `llm-agent-flow/v2`).
- **Separate CI surfaces.** Provider adapters need Docker for `testcontainers` Ollama coverage; core does not. RAG needs a Postgres+pgvector container for the conformance suite; core does not. `llm-agent-flow` carries the CEL evaluator (`google/cel-go`) and a pure-Go SQLite store (`modernc.org/sqlite`) — neither belongs in core. Splitting CI keeps the core run fast and stdlib-only.
- **Focused docs and README per repo.** Each repo's `README.md` is its own surface area; the umbrella README is only a navigation index.
- **`go.mod` blast radius.** Importing `llm-agent` does not pull in OpenAI/Anthropic SDKs, OTel exporters, or `cel-go`/SQLite. Those only arrive when the caller explicitly `go get`s the relevant sister repo.
- **Tagged-release isolation.** `INFRA-04` refuses to tag a commit with a `replace` directive, so every release of every repo is reproducible from `proxy.golang.org` alone.
- **Coordinated bump pattern (Phase 33).** When all repos need to move together (e.g., the v1.1 ecosystem-alignment wave on 2026-05-20), they're re-tagged in dependency order: rag → core → flow → otel → providers → customer-support. The v0.0.x → v0.1.0 freeze of `llm-agent-flow` happened mid-stream within this 2026-05-21 wave.

## Layers

**Umbrella shell (this repo):**
- Purpose: workspace coordination only; no product source.
- Location: `/home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/`
- Contains: `go.work` (now `use` 6 modules), `Makefile`, `scripts/eco.sh`, `scripts/workspace.sh`, `docs/`, `.planning/`.
- Depends on: nothing.
- Used by: developers; not imported by any repo.

**Core framework (`llm-agent`):**
- Purpose: stdlib-only Agent + Tool + Memory + LLM capability surface.
- Location: `llm-agent/`
- Contains: top-level paradigm `.go` files, `llm/`, `memory/`, `orchestrate/`, `budget/`, `pkg/fanout/`, `rag/` (facade), `builtin/`, `comm/`, `context/`, `rl/`.
- Depends on: `llm-agent-rag v1.0.1` (RAG facade only).
- Used by: every other sibling.

**RAG SDK (`llm-agent-rag`):**
- Purpose: import → retrieve → generate primitives + GraphRAG + pgvector backend.
- Location: `llm-agent-rag/`
- Depends on: stdlib only at v1.0.x; `postgres/` subpackage pulls `pgx/v5` + `pgvector-go`.
- Used by: `llm-agent` (facade), `llm-agent-otel` (`otelrag/`), `llm-agent-customer-support` (transitive).

**Providers (`llm-agent-providers`):**
- Purpose: real provider adapters implementing `llm.ChatModel` and capability interfaces.
- Location: `llm-agent-providers/{openai,anthropic,ollama,deepseek,minimax}/`
- Depends on: `llm-agent v0.5.1` + each provider's official Go SDK.
- Used by: `llm-agent-customer-support`.

**Flow IR + DAG executor (`llm-agent-flow`):**
- Purpose: serializable flow JSON → topological DAG engine; `flow.Runner` interface as the wrap seam for telemetry / replay / policy decorators; `cmd/flowd` HTTP service with SQLite-backed CRUD + run history + per-event replay + bearer-token auth.
- Location: `llm-agent-flow/`
- Depends on: `llm-agent v0.5.1` (the back-edge for `agents.Tool` adaption + `pkg/fanout` per-layer parallelism). The library `flow/` package itself is stdlib-only at the source level; sub-packages `flow/cond/cel` and `flow/store/sqlite` pull in `google/cel-go` and `modernc.org/sqlite` respectively, but only when imported.
- Used by: `llm-agent-otel` (`otelflow/`), `llm-agent-customer-support` (`internal/flowrunner/`).

**OTel decorators (`llm-agent-otel`):**
- Purpose: capability-preserving OpenTelemetry wrappers — `otelmodel.Wrap`, `otelagent.Wrap`, `otelrag.Wrap`, **`otelflow.Wrap`** (new in v0.2.2 — Keystone K3 applied to `flow.Runner`).
- Location: `llm-agent-otel/{otelmodel,otelagent,otelrag,otelmetrics,otelslog,otelflow}/`
- Depends on: `llm-agent v0.5.1` + `llm-agent-rag v1.0.1` + **`llm-agent-flow v0.0.7`** + OTel SDKs.
- Used by: `llm-agent-customer-support`.

**Reference service (`llm-agent-customer-support`):**
- Purpose: deployable demo that wires the full stack — chat / streaming / RAG + (new at v0.2.3) flow execution via `internal/flowrunner/`.
- Location: `llm-agent-customer-support/{cmd/server,internal/...,compose/}`
- Depends on: core + providers + otel + **flow** (direct require in `go.mod`) — and transitively rag.
- Used by: end-user demos.

## Core abstractions in `llm-agent/`

All agent paradigms compose three orthogonal primitives.

**`llm.ChatModel` (`llm-agent/llm/chatmodel.go`):**
The smallest possible interface — `Generate`, `Stream`, `Info`. Capabilities beyond text generation are expressed as separate interfaces detected by type assertion plus a runtime `Capabilities` struct on `ProviderInfo` (`llm-agent/llm/info.go`).

```go
type ChatModel interface {
    Generate(ctx context.Context, req Request) (Response, error)
    Stream(ctx context.Context, req Request) (StreamReader, error)
    Info() ProviderInfo
}
```

**Capability interfaces (`llm-agent/llm/capabilities.go`):**
- `ToolCaller` — `WithTools` is IMMUTABLE; returns a new instance. Rejects the deprecated Eino `BindTools` mutation pattern.
- `Embedder` — does NOT embed `ChatModel`; orthogonal so a future embed-only adapter (voyageai) can implement just this.
- `StructuredOutputs` — `WithSchema` is IMMUTABLE and returns `ChatModel` (not `StructuredOutputs`) because re-applying a schema is meaningless.

**`Agent` (`llm-agent/agent.go`):**
- `Name() string`
- `Run(ctx, input) (Result, error)` — synchronous; `Result.Trace` is the full debug trace (no size limit; high-concurrency callers must discard).
- `RunStream(ctx, input) (<-chan StepEvent, error)` — streaming; channel closes after the terminal event (`Done=true` with one of `Final`/`Err` set). Phase-8 SSE handlers are the natural consumer.
- `Step` enum: `thought`, `action`, `observation`, `reflection`, `plan`, `final`.

**`Tool` (`llm-agent/tool.go`):**
- Interface: `Name() / Description() / Schema() json.RawMessage / Execute(ctx, args) (string, error)`.
- `AsLLMTool(Tool) llm.Tool` adapter so an agent's tool can be passed to a tool-capable `ChatModel`.
- `NewFuncTool(name, desc, schema, fn)` for trivial cases.

**`Registry` (`llm-agent/registry.go`):**
- Per-Agent name→Tool map; no `init()`-time global singleton (test isolation).
- `NewRegistry(tools...)` panics on duplicate names (constructor-time safety; prefers panic to silent shadowing).
- Thread-safe with `sync.RWMutex`; `List()` returns deterministic sort by Name.

**`Memory` (`llm-agent/memory/memory.go`):**
- Three kinds: `KindWorking`, `KindEpisodic`, `KindSemantic`.
- Single `Memory` interface so an agent can route by Kind via `Manager`.
- Backend dependency: `llm.Embedder` for semantic scoring; tests use `ScriptedLLM`.

**`RAGSystem` — facade only:**
- `llm-agent/rag/` is an empty directory; the facade lives elsewhere in the core repo and re-exports types from `llm-agent-rag/rag/system.go`. The contract is: anyone who needs RAG types imports them either from `github.com/costa92/llm-agent/rag` (stable facade) or directly from `github.com/costa92/llm-agent-rag/rag`. The `v1.0.1` back-edge in `llm-agent/go.mod` is the ONLY non-stdlib line allowed by INFRA-01.

**Agent paradigms** (top-level files in `llm-agent/`):
- `simple.go` — `SimpleAgent`: one LLM call, no tools, no loop. The minimum.
- `react.go` — `ReActAgent`: Thought→Action→Observation loop, prompt-based parsing, MaxSteps cap.
- `plan_solve.go` — `PlanAndSolveAgent`: plan once → execute each step → synthesize. Three prompt slots.
- `reflection.go` — `ReflectionAgent`: generate → critique → revise, stops on `APPROVED`.
- `chain.go` — `Chain`: pipes Tools sequentially; each tool's string output becomes the next tool's `{"input": ...}`. `Chain` itself satisfies `Tool` so it can be registered or nested.
- `function_call.go` — `FunctionCallAgent`: native OpenAI-style function-calling using `llm.ToolCaller`; single-turn (multi-turn blocked on `pkg/llm.Message.ToolCallID` enhancement); executes tool calls in parallel via `AsyncRunner`.
- `agent_chatmodel.go` — bridges Agent execution with `ChatModel`; integrates `budget.Tracker` from ctx at the `generateFromPrompt` chokepoint (KC-4 ctx-keyed budget propagation).
- `async.go` — `AsyncRunner`: parallel tool execution via `pkg/fanout`; panics are recovered as `TaskResult.Err` (`*fanout.ErrTaskPanic`) so one bad tool doesn't crash the process.
- `registry.go` — see above.

## Core abstractions in `llm-agent-flow/`

**`flow.Flow` IR (`llm-agent-flow/flow/ir.go`):**
A serializable DAG — `{ID, Name, Description, Nodes, Edges, Inputs, Outputs}`. Nodes carry a `Type` (resolved via `NodeRegistry`) and an opaque `Config json.RawMessage`. Edges carry `Source PortRef`, `Target PortRef`, and an optional `Condition` string (CEL expression evaluated at run time). JSON round-trip via `Load(r) / Marshal(f)`; `Validate(f)` rejects cycles, dangling edges, duplicate node IDs.

**`flow.Engine` (`llm-agent-flow/flow/engine.go`):**
The DAG executor. Single immutable `Compile` step (`Validate` → resolve `NodeKind`s → topological-layer ordering → precompile every non-empty `Edge.Condition`). Per-layer goroutine fan-out via `github.com/costa92/llm-agent/pkg/fanout` (the back-edge); `WithMaxNodeConcurrency(n)` caps the layer goroutine count. Fail-fast: the first node error cancels in-flight peers within the layer. `Run` is the sync entry; `RunStream(ctx, inputs) (<-chan FlowEvent, error)` is the streaming entry.

**`flow.Runner` interface (`llm-agent-flow/flow/runner.go` — new in v0.0.7):**
```go
type Runner interface {
    Run(ctx context.Context, inputs map[string]string) (map[string]string, error)
    RunStream(ctx context.Context, inputs map[string]string) (<-chan FlowEvent, error)
}

var _ Runner = (*Engine)(nil) // compile-time assertion
```
This is **the wrap seam** decorators target. `*Engine` satisfies it directly; the package carries a compile-time assertion so wrapper packages can rely on it without explicit type assertions. `(*Engine).FlowID()` / `FlowName()` getters expose the compiled flow's identity so wrappers can attach span attributes without reaching into private fields.

**`FlowEvent` typed union (`llm-agent-flow/flow/event.go`):**
Mirrors the K1 streaming idiom on the LLM side. Kinds: `FlowStarted`, `NodeStarted`, `NodeFinished`, `NodeSkipped`, `FlowDone`, `FlowErr`. Per-node ordering and the FlowStarted-first / FlowDone-last invariants hold; sibling events within a layer may interleave under parallel execution.

**Two stability tiers in `llm-agent-flow`:**

The v0.1.0 freeze (`docs/compatibility.md`) splits the surface into:
- **Stable v0.1.x — exported API of every importable package:**
  - `flow/` — `Flow`, `Node`, `Edge`, `PortRef`, `NamedPortRef`, `FlowEvent`, `FlowEventKind`, `Engine`, `Runner`, `Condition`, `ConditionEvaluator`, `CondEnv`, `Deps`, `NodeRegistry`, `NodeFactory`, `NodeKind`, `Port`, `Tool`, `ToolMap`, `ToolLookup`, `ValidateError`. Funcs: `Load`, `Marshal`, `Validate`, `Compile`, `LoadCompile`, `NewNodeRegistry`, `RegisterToolNode`, `FromAgentTool`, `FromAgentTools`, `WithMaxNodeConcurrency`, `WithConditionEvaluator`. Constants: `TypeTool`, `FlowStarted` … `NodeSkipped`.
  - `flow/cond/cel` — `NewEvaluator`. Stable; CEL evaluator implementing `flow.ConditionEvaluator`.
  - `flow/store` — `Store` interface, `FlowMeta`, `FlowRecord`, `RunMeta`, `RunRecord`, `RunStatus`, `RunEvent`, `RunEventKind`, `RunEventBatchItem` (additive in v0.1.1), `ErrNotFound`, `ErrAlreadyExists`.
  - `flow/store/sqlite` — `Open(dsn)`, `*Store` (`Close`, `PutFlow`, `GetFlow`, `ListFlows`, `DeleteFlow`, `StartRun`, `FinishRun`, `GetRun`, `ListRuns`, `AppendRunEvent`, `ListRunEvents`, `AppendRunEvents`).
  - `flow/tools` — `Manifest`, `Entry`, `KindRegistry`, `LoadManifest`, `LoadAndBuild`, built-in `http` + `exec` kinds.
  - `cmd/flowd/server` — `Config`, `Server`, `New`, `NewMux`, `Authenticator`, `BearerTokenAuthenticator`, `ErrUnauthorized`. **The HTTP wire shape is also v0.1.x stable** (every endpoint listed in the README — `/flows` CRUD, `/flows/{id}/run`, `/flows/{id}/run/stream`, `/flows/{id}/runs`, `/runs/{id}`, `/runs/{id}/events`, `/runs/{id}/replay`, `/healthz`).
  - The committed baseline is `llm-agent-flow/api/v0.1.snapshot.txt`; the gate is `llm-agent-flow/internal/apisnapshot/`. Both are checked on every `go test`.

- **Unstable — `internal/`:**
  - `llm-agent-flow/internal/apisnapshot/` is the gate itself and is not importable outside the module.

- **NOT covered by v0.1 (still allowed to break with notice):**
  - Behavior under unspecified inputs (e.g., concurrent CRUD races the SQLite store does not serialize).
  - Command-line flag *additions* (existing flag meanings are stable; new flags can appear).
  - Internal layout changes (e.g., the v0.1.1 swap of the engine cache from `sync.Map` to a `container/list`-backed LRU).

**Same pattern as `llm-agent-rag`:** Both repos use the `internal/apisnapshot/` + `api/v<N>.snapshot.txt` gate; both promise additive-only within a major (rag v1.x, flow v0.1.x); both route breaking changes to a `/v2` module path. `llm-agent-flow`'s freeze landed at v0.1.0 (2026-05-21), the same wave that re-tagged `llm-agent-rag` to v1.0.2.

## Decorator pattern (Keystone K3)

OTel attaches as decorator wrappers, **never as hooks** on the core types.

```go
// llm-agent-otel/otelmodel/otelmodel.go
func Wrap(model llm.ChatModel, opts ...Config) llm.ChatModel
```

The wrapper preserves the inner model's capability interfaces by inspecting the type at wrap time:

```go
if tc, ok := model.(llm.ToolCaller); ok {
    if emb, ok := model.(llm.Embedder); ok {
        if so, ok := model.(llm.StructuredOutputs); ok {
            return &toolEmbedSchemaWrapper{wrapper: base, toolCaller: tc, embedder: emb, structured: so}
        }
        return &toolEmbedWrapper{wrapper: base, toolCaller: tc, embedder: emb}
    }
    if so, ok := model.(llm.StructuredOutputs); ok {
        return &toolSchemaWrapper{wrapper: base, toolCaller: tc, structured: so}
    }
    return &toolWrapper{wrapper: base, toolCaller: tc}
}
// ...etc.
```

There are 8 wrapper types (one per power-set of `{ToolCaller, Embedder, StructuredOutputs}`) so `Wrap(inner).(ToolCaller)` succeeds iff `inner.(ToolCaller)` succeeds. The same shape is used by `otelagent.Wrap(Agent)`, `otelrag.Wrap(System)` (via `rag.Observer`), and v1.2's planned `policy.Wrap(model) ChatModel`.

**`otelflow.Wrap(flow.Runner) flow.Runner` (new in v0.2.2):**
The fourth K3 application. `llm-agent-otel/otelflow/otelflow.go` (`Wrap(inner flow.Runner, cfg Config) flow.Runner`) emits two layers of OTel spans:

```text
flow.run <FlowID>                ← root span (Run or RunStream)
└── flow.node <NodeID>           ← child span per node (RunStream only)
    flow.node <NodeID> skipped=true   ← zero-duration span for CEL-skipped nodes
```

Span attributes from `llm-agent-otel/otelflow/config.go`: `flow.id`, `flow.name`, `flow.node.id`, `flow.node.skipped`, `flow.event.kind`, `flow.finish_reason`, `flow.input_count`, `flow.output_count`. Identity is pulled from `(*flow.Engine).FlowID()` / `FlowName()` when the inner `Runner` satisfies the `flowIdentifier` interface; `Config.FlowID` overrides for callers using an external identifier (UUID assigned by an upstream orchestrator). Sync `Run` produces one root span only — per-node spans need the streaming event channel to fire NodeStarted / NodeFinished pairs.

Why decorator, not hook: a hook lives inside the core type and forces every adapter to know about OTel; the decorator lives in a sister repo and is opt-in per call site. The core `llm-agent-flow/flow/` package carries zero OTel symbols, just like `llm-agent` itself.

## Provider adapter pattern (Keystone K2)

**A provider instance binds a model at construction time.** Capabilities are per-`(provider × model)`, not per-provider.

```go
// llm-agent-providers/openai/openai.go
type OpenAI struct {
    client *openai.Client
    info   llm.ProviderInfo  // Provider + Model + Capabilities — set at New()
    tools  []llm.Tool
}

var (
    _ llm.ChatModel  = (*OpenAI)(nil)
    _ llm.ToolCaller = (*OpenAI)(nil)
    _ llm.Embedder   = (*OpenAI)(nil)
)
```

`ProviderInfo.Capabilities` (`llm-agent/llm/info.go`) is a value-typed struct (NOT a bitmask; NOT methods) so it is JSON-serializable for OTel attribute emission and self-documenting in test failures:

```go
type Capabilities struct {
    Tools             bool
    Embeddings        bool
    StructuredOutputs bool
    PromptCaching     bool
}
```

Type assertion is the **compile-time** signal (`if tc, ok := model.(ToolCaller); ok`). `Capabilities` is the **runtime** signal for variation that type assertion can't see — Ollama's Go type implements `ToolCaller`, but for `llama2` `Capabilities.Tools == false`. The canonical negotiation idiom lives in `llm-agent/llm/doc.go`.

`WithTools` returns a **new** value (no mutation) so concurrent calls on the same model with different tool sets do not race.

## Stream event model (Keystone K1)

Streaming events are a **typed union**, not lowest-common-denominator chunks. The provider adapter emits at its NATIVE granularity (OpenAI per-index deltas, Anthropic per-content-block deltas, Ollama whole-tool-call); consumers that don't care use `AccumulateStream`.

```go
// llm-agent/llm/stream.go
type StreamEventKind uint8
const (
    EventTextDelta StreamEventKind = iota
    EventToolCallStart
    EventToolCallArgsDelta
    EventToolCallEnd
    EventThinkingDelta
    EventDone
)

type StreamEvent struct {
    Kind         StreamEventKind
    Text         string
    ToolCall     *ToolCallDelta
    Usage        *Usage
    FinishReason FinishReason
}

type ToolCallDelta struct {
    Index     int    // stable across chunks for a single tool call
    ID        string // provider-assigned ID
    Name      string // populated ONCE on EventToolCallStart
    ArgsDelta string // partial JSON; concat across chunks for this Index
}
```

`StreamReader` is an iterator (`Next() (StreamEvent, error)`; `Close() error`) — chosen over `<-chan StreamEvent` for explicit cancellation, single-call error propagation, leak prevention when consumers break out early, and composability with the K4 retry state machine.

The **stable `Index` field** is what makes this work: the agent-layer accumulator joins by `Index`, NOT by `Name` (Pitfall 1: "OpenAI streaming tool_calls — losing chunks because you keyed by name"). Adapters in `llm-agent-providers` all emit `Index`; consumers in `llm-agent` and `llm-agent-customer-support` all key by `Index`.

The same typed-union shape is replicated at the flow layer: `flow.FlowEvent.Kind` enum, sibling layer events may interleave, per-node ordering preserved.

## Customer-support service architecture

`llm-agent-customer-support/cmd/server/main.go` loads config, installs signal handling, calls `app.New(...)`, runs `http.Server` until SIGINT/SIGTERM.

`internal/app/app.go` is the composition root. Its `Options` exposes four factory seams (`ModelFactory`, `EmbedderFactory`, `SessionStoreFactory`, `TracerProviderFactory`) so tests can inject doubles. It builds in order:

1. **Config + tracer.** `internal/config/config.go` parses env (provider-aware defaults for openai / anthropic / ollama); `TracerProviderFactory` wires OTLP via `otelroot.NewTracerProvider`.
2. **Chat + embed providers — independently selected.** `internal/providers/providers.go` is the split chat/embedding factory: `LLM_PROVIDER=openai|anthropic|ollama|deepseek|minimax` and `EMBEDDING_PROVIDER=openai|ollama` are separate knobs, so `anthropic` chat can co-exist with `ollama` embeddings.
3. **OTel decorators.** `otelmodel.Wrap(model)` (K3) on the chat model; `otelagent.Wrap(agent)` on the final agent.
4. **Session store.** `internal/sessionstore/` exposes a `Store` contract with SQLite (`modernc.org/sqlite`) and Postgres (`lib/pq`) implementations selected by `SESSION_BACKEND=sqlite|postgres`. `context.go` propagates session ID through the request context.
5. **Knowledge base.** `internal/knowledgebase/` seeds a RAG corpus on boot (uses the `llm-agent-rag` SDK transitively).
6. **Limits guard.** `internal/limits/limits.go` enforces config-driven hard caps from Day 1 (K7).
7. **Guardrails.** `internal/guardrails/guardrails.go` runs day-one prompt-injection defenses.
8. **Support flow.** `internal/supportflow/supportflow.go` is typed StateGraph triage. `toolagent.go` wires the FunctionCallAgent variant.
9. **Flow runner bridge (new in v0.2.3).** `internal/flowrunner/flowrunner.go` is the downstream integration example for `llm-agent-flow`. `flowrunner.New(Config{TracerProvider, Tools, Cond})` builds a `*flow.NodeRegistry` with `RegisterToolNode`; `Execute` / `ExecuteStream` compile a flow JSON via `flow.LoadCompile`, wrap the resulting `*flow.Engine` with `otelflow.Wrap(engine, otelflow.Config{TracerProvider})`, then call `Run` / `RunStream`. Per-flow + per-node spans land in the same trace tree as chat / tool / RAG spans because the host's OTel `TracerProvider` is reused. Scope at v0.2.3 is deliberately narrow — load + compile + wrap + run; persistence and HTTP exposure stay in `cmd/flowd` (the dedicated `llm-agent-flow` binary).
10. **HTTP transport.** `internal/httpapi/httpapi.go` exposes `POST /chat`, `POST /chat/stream` (SSE), `GET /healthz`, `GET /readyz`.
11. **Compose stack.** `compose/compose.yaml` brings up app + Ollama + `grafana/otel-lgtm` + an OpenTelemetry Collector with tail-sampling.

## flowd HTTP service (`llm-agent-flow/cmd/flowd/`)

`llm-agent-flow/cmd/flowd/main.go` boots the long-running HTTP variant. Flags: `--addr`, `--db` (SQLite DSN; defaults to `:memory:`), `--flow` (optional seed flow + legacy `/run` alias), `--tools` (optional tool manifest), `--max-node-concurrency`, `--token` (or `FLOWD_TOKEN` env) for bearer auth, `--read-timeout`, `--write-timeout` (0 disables — required for SSE).

`cmd/flowd/server/server.go` exposes `(*Server).Handler()`:

| Endpoint | Method | Purpose |
|---|---|---|
| `/healthz` | GET | Liveness; bypasses authenticator |
| `/flows` | POST | Create flow (compile-probe validates body); returns FlowRecord |
| `/flows` | GET | List flows |
| `/flows/{id}` | GET | Get one flow |
| `/flows/{id}` | PUT | Replace; invalidates engine cache entry |
| `/flows/{id}` | DELETE | Remove; invalidates engine cache entry |
| `/flows/{id}/run` | POST | Sync run; returns `X-Run-ID` + outputs |
| `/flows/{id}/run/stream` | POST | SSE run; one frame per `FlowEvent` |
| `/flows/{id}/runs` | GET | Run history for a flow |
| `/runs/{id}` | GET | Single run record (inputs / outputs / error) |
| `/runs/{id}/events` | GET | Full ordered FlowEvent history for a run |
| `/runs/{id}/replay` | POST | Re-stream a run's persisted events as a fresh SSE session; sets `X-Replay: true` |
| `/run`, `/run/stream` | POST | Legacy aliases against the seed flow; only mounted when `--flow` is supplied |

**Engine cache (`cmd/flowd/server/lru.go`, new in v0.1.1):** Compiled `*flow.Engine` instances are kept in an LRU-bounded cache keyed by flow id. `Config.EngineCacheSize <= 0` (the default) disables bounding — every compiled engine stays cached indefinitely, matching v0.1.0 behavior. Positive values enable LRU eviction; PUT and DELETE handlers still evict their entry immediately regardless of cache size.

**Event persistence:** Every `FlowEvent` is persisted to `run_events` before being forwarded (in stream mode) to the SSE client. A client that drops mid-stream still leaves a complete audit trail. v0.1.1 introduces a sync-path optimization: events collected during a `POST /flows/{id}/run` (no streaming) are flushed in **one transaction** at the end of the run via the optional `(*sqlite.Store).AppendRunEvents(ctx, runID, items)` bulk-insert method. Stream runs (`/run/stream`) still persist per-event before forwarding to preserve the "events outlive a dropped client" guarantee.

**Authentication (v0.0.8 / stable in v0.1):** `server.Authenticator` interface; `server.BearerTokenAuthenticator{Token: ...}` is the bundled static-token implementation with constant-time comparison. Returning `server.ErrUnauthorized` → 401 + `WWW-Authenticate: Bearer realm="flowd"` challenge; any other error → 403. `/healthz` is always bypassed so external monitors work without a token. A nil `Authenticator` leaves the API fully open (backward compatible).

## Data Flow

### Primary chat request (`POST /chat`)

1. HTTP handler at `llm-agent-customer-support/internal/httpapi/httpapi.go` receives the request, resolves/creates `session_id`.
2. `internal/limits/limits.go` preflight: rate-limit, token cap, daily budget, panic switch.
3. `internal/guardrails/guardrails.go` filters input.
4. `internal/supportflow/supportflow.go` classifies intent (chargeback / refund / unknown) and selects the route.
5. RAG path: knowledge lookup against the seeded `rag.System` from `llm-agent-rag`.
6. `agents.Agent` (wrapped by `otelagent.Wrap`) executes. The bound `ChatModel` is wrapped by `otelmodel.Wrap`.
7. Session writes go to `internal/sessionstore` (SQLite or Postgres).
8. Response carries `X-Trace-Id` for cross-referencing in Grafana.

### Flow execution (in-process via flowrunner)

1. Caller hands `flowrunner.Execute(ctx, flowJSON, inputs)` a flow JSON reader and an input map (`llm-agent-customer-support/internal/flowrunner/flowrunner.go`).
2. `compileAndWrap` calls `flow.LoadCompile(flowJSON, registry, Deps{Tools}, opts...)` with `WithConditionEvaluator(cfg.Cond)` if a CEL evaluator is wired.
3. The resulting `*flow.Engine` is wrapped with `otelflow.Wrap(engine, otelflow.Config{TracerProvider})` returning a `flow.Runner`.
4. `Runner.Run(ctx, inputs)` opens the root span `flow.run <FlowID>`, calls the underlying engine, records error or success on the span, returns outputs.
5. `Runner.RunStream(ctx, inputs)` opens the root span `flow.run.stream <FlowID>`, forwards each `FlowEvent` to the caller, opens / closes child spans `flow.node <NodeID>` on NodeStarted / NodeFinished, emits zero-duration spans for NodeSkipped, closes the root after the inner channel drains (including on ctx cancellation).
6. Trace tree composes naturally with the host's chat / tool / RAG spans because the same `TracerProvider` was passed.

### Flow execution (over HTTP via flowd)

1. Client `POST /flows/{id}/run` with `{"inputs":{...}}`.
2. `Server.handleRun` (`llm-agent-flow/cmd/flowd/server/server.go`) loads or compiles the engine (LRU-cached), records a new run row (`store.StartRun`), invokes `engine.RunStream` to collect events, flushes events as a batch via `AppendRunEvents` at the end (v0.1.1 sync optimization), updates the run row (`store.FinishRun`), returns `{"outputs":...,"run_id":...}` with the `X-Run-ID` header.
3. SSE variant `POST /flows/{id}/run/stream` persists each event before forwarding it as an SSE frame, so a dropped client still leaves a complete event log.
4. Replay variant `POST /runs/{id}/replay` re-streams the persisted events directly from `run_events` — no new engine invocation; `X-Replay: true` + `X-Run-ID` identify the replay.

### Streaming chat (`POST /chat/stream`)

1–5 identical to chat.
6. `agent.RunStream(ctx, input)` returns `<-chan StepEvent`. Handler converts to SSE; discards `res.Trace` after the channel closes.
7–8 identical.

**State Management:**
- Per-request: ctx-keyed (budget tracker via `budget.From(ctx)`, session ID via `sessionstore` context helpers).
- Per-session: durable in SQLite/Postgres.
- Per-process: tool `Registry`, knowledge-base `rag.System` instance, OTel `TracerProvider`, `flowrunner.Runner` instance (all owned by `App`).
- Per-flow-run (flowd): durable in `flow/store/sqlite` — flow CRUD rows, run rows, ordered `run_events` rows.

## Key Abstractions

**`StreamEvent` typed union:**
- Purpose: provider-native granularity surfaced to consumers without lowest-common-denominator loss.
- Examples: `llm-agent/llm/stream.go` (definition), `llm-agent-providers/openai/openai.go` (emit), `llm-agent-customer-support/internal/httpapi/httpapi.go` (SSE consume).
- Pattern: typed-union (K1); stable per-tool-call `Index`.

**`FlowEvent` typed union:**
- Purpose: per-node lifecycle events surfaced during flow execution.
- Examples: `llm-agent-flow/flow/event.go` (definition), `llm-agent-flow/flow/engine.go` (emit), `llm-agent-flow/cmd/flowd/server/server.go` (persist + SSE forward), `llm-agent-otel/otelflow/otelflow.go` (consume to drive per-node spans).
- Pattern: typed-union mirroring K1; per-node ordering preserved across parallel siblings within a layer.

**`Capabilities` value struct:**
- Purpose: runtime, JSON-serializable per-(provider × model) capability signal.
- Examples: `llm-agent/llm/info.go` (definition); each `*/options.go` in `llm-agent-providers` (where `New()` sets it).
- Pattern: K2 — bound at construction.

**Decorator `Wrap(inner) Inner`:**
- Purpose: opt-in cross-cutting concern without touching the inner type.
- Examples: `llm-agent-otel/otelmodel/otelmodel.go`, `otelagent/otelagent.go`, `otelrag/otelrag.go`, **`otelflow/otelflow.go`** (new), and the planned v1.2 `policy.Wrap`.
- Pattern: K3 — decorator that preserves capability interfaces via 2ⁿ wrapper types (model side) OR a single wrapper struct around a uniform interface (flow side — `flow.Runner` has no capability sub-interfaces, so one type suffices).

**`flow.Runner` wrap seam:**
- Purpose: stable interface that decorators target without depending on `*flow.Engine`.
- Examples: `llm-agent-flow/flow/runner.go` (definition + compile-time `_ Runner = (*Engine)(nil)`), `llm-agent-otel/otelflow/otelflow.go` (decoration), `llm-agent-customer-support/internal/flowrunner/flowrunner.go` (composition example).
- Pattern: K3 applied to the flow layer; `Engine.FlowID()` / `FlowName()` getters surface identity without exposing private fields.

## Entry Points

**Umbrella build entry points:**
- Location: `Makefile` at repo root.
- Triggers: `make bootstrap | workspace | pull | status | build | test | up | down`.
- Responsibilities: dispatch to `scripts/eco.sh <action> $TARGETS` (now 6 repos) or `scripts/workspace.sh`.

**Customer-support runtime entry point:**
- Location: `llm-agent-customer-support/cmd/server/main.go`.
- Triggers: `docker compose -f compose/compose.yaml up --build` or direct `go run`.
- Responsibilities: load config → build `App` → run HTTP server → graceful shutdown on signal.

**flowd runtime entry point:**
- Location: `llm-agent-flow/cmd/flowd/main.go`.
- Triggers: `go install github.com/costa92/llm-agent-flow/cmd/flowd@latest` then `flowd --addr :7861 --db /var/lib/flowd/flow.db [--token …] [--flow file.json] [--tools manifest.json]`.
- Responsibilities: open SQLite store → build node registry → load tools → wire CEL evaluator → build `server.Server` → ListenAndServe → graceful shutdown on signal.

**flow CLI entry point:**
- Location: `llm-agent-flow/cmd/flow/main.go`.
- Triggers: `flow run examples/echo_chain/flow.json --input in=hello [--stream] [--tools manifest.json]`.
- Responsibilities: one-shot in-process execution; no HTTP, no persistence.

**OTel demo entry point:**
- Location: `llm-agent-otel/cmd/tailprobe/` + `compose/compose.yaml`.
- Triggers: `make up TARGETS=llm-agent-otel`.
- Responsibilities: emit demo telemetry against `grafana/otel-lgtm`.

**Planning entry point:**
- Location: `llm-agent/.planning/STATE.md` + `ROADMAP.md` + `REQUIREMENTS.md`.
- Triggers: GSD commands (`/gsd:plan-phase`, `/gsd:execute-phase`).
- Responsibilities: declare the active milestone (currently v1.2 Core Capability Deepening) and the next concrete phase.

## Architectural Constraints

- **Stdlib-only core.** Anything that adds a non-stdlib import to `llm-agent/go.mod` beyond the single `llm-agent-rag` back-edge is rejected by CI.
- **Stdlib-only `flow/` library package.** The `llm-agent-flow/flow/` directory imports only stdlib + `github.com/costa92/llm-agent` (for `agents.Tool` adaption + `pkg/fanout`). Non-stdlib deps (`cel-go`, `modernc.org/sqlite`) live exclusively under sub-packages (`flow/cond/cel`, `flow/store/sqlite`) that callers opt into by import.
- **No `replace` in tagged-release branches.** INFRA-04 gate; `replace` is local-dev only.
- **`go.work` is `.gitignore`d in every sister repo; CI runs `GOWORK=off`.** Local cross-repo dev uses `make workspace`; release builds use module proxy. The umbrella's own `go.work` IS committed (lists 6 modules).
- **No K8s / Helm anywhere.** Standing non-goal; see Pitfall 16. Half-shipped K8s is worse than none.
- **`llm-agent-rag` v1.x public API is additive-only.** Breaking changes go to a `/v2` module path. v1.0 froze the surface via `internal/apisnapshot` + `api/v1.snapshot.txt`.
- **`llm-agent-flow` v0.1.x public API is additive-only.** Same pattern: `internal/apisnapshot` + `api/v0.1.snapshot.txt`. The frozen surface spans `flow/`, `flow/cond/cel`, `flow/store`, `flow/store/sqlite`, `flow/tools`, and `cmd/flowd/server`. The flowd HTTP wire shape is also stable in v0.1.x. Breaking changes go to `/v2`.
- **Capabilities are per-(provider × model).** Not per-provider, not per-type. `Info().Capabilities` is the runtime truth.
- **`WithTools` / `WithSchema` are immutable.** No mutation of bound state.
- **`*flow.Engine` is immutable post-Compile.** Concurrency-safe `Run` / `RunStream`; reuse a compiled engine across calls.
- **Threading.** `ChatModel` implementations MUST be safe for concurrent use; concurrent `Generate` / `Stream` on the same value is part of the contract (`llm/chatmodel.go` doc-comment). Same applies to `flow.Runner` (`flow/runner.go` doc-comment).
- **Trace memory contract.** Streaming consumers MUST discard `Result.Trace` after the channel closes — same information twice would otherwise hold 50–100 Steps (~4KB each) per in-flight handler (`agent.go`).
- **Per-event persistence is the audit-trail contract.** flowd's `/run/stream` path persists each event to `run_events` BEFORE forwarding to the SSE client. A client that drops still leaves a complete trail; `/runs/{id}/events` and `/runs/{id}/replay` rely on this.

## Anti-Patterns

### Keying streaming tool_call accumulation by `Name`

**What happens:** Naive consumers join `EventToolCallArgsDelta` chunks by `ToolCall.Name`, lose chunks where Name is empty, and end up with a truncated args JSON.
**Why it's wrong:** Provider adapters populate `Name` ONLY on `EventToolCallStart`; subsequent `*ArgsDelta` events carry `Name == ""`.
**Do this instead:** Key by the stable `ToolCall.Index` field. See `llm-agent/llm/stream.go` doc-comment and `llm-agent/llm/llm_test.go` for the canonical pattern.

### `BindTools(...)` mutation on a shared `ChatModel`

**What happens:** Code calls `model.BindTools(toolsA)` then concurrently calls `model.BindTools(toolsB)` — the second call races with the first, and a concurrent `Generate` may see a half-rewritten tool set.
**Why it's wrong:** Mutation of a shared model violates the documented thread-safety contract.
**Do this instead:** `mA, _ := model.WithTools(toolsA); mB, _ := model.WithTools(toolsB)` — `WithTools` is IMMUTABLE and returns a new `ToolCaller` (`llm-agent/llm/capabilities.go`).

### Attaching OTel via an `OnSpan` hook inside `ChatModel`

**What happens:** Caller patches a `func(ctx, ProviderInfo) context.Context` hook onto the core `ChatModel` interface so OTel can attach spans.
**Why it's wrong:** Couples the stdlib-only core to OTel symbols (violates INFRA-01); every adapter has to know about the hook; opt-out is impossible.
**Do this instead:** `otelmodel.Wrap(provider) llm.ChatModel` (K3). The wrapper preserves capability interfaces; the core stays stdlib-only. Same shape for `otelflow.Wrap(flow.Runner) flow.Runner` — `llm-agent-flow/flow/` carries zero OTel symbols.

### Trusting `ProviderInfo.Provider == "ollama"` to mean tools are available

**What happens:** Code does `if info.Provider == "ollama" { tools enabled }` and silently fails when the model is `llama2` (no tool support).
**Why it's wrong:** Capabilities are per-(provider × model), not per-provider.
**Do this instead:** `if model.Info().Capabilities.Tools { ... }` AND `tc, ok := model.(llm.ToolCaller); ok` — both signals at once.

### Reading `Result.Trace` in a streaming SSE handler

**What happens:** Handler streams `StepEvent`s to the client, then on `Done=true` reads `res.Trace` "for completeness." Holds 50–100 ~4KB Steps per in-flight handler; 100 concurrent → 40MB wasted.
**Why it's wrong:** `Trace` is the same information already flushed via `StepEvent`.
**Do this instead:** Discard `res.Trace` after the channel closes (see `llm-agent/agent.go` doc-comment).

### `replace` directive on a tagged-release branch

**What happens:** Developer iterates with `go mod edit -replace=...=../sibling`, then tags the branch.
**Why it's wrong:** `replace` makes the build irreproducible from `proxy.golang.org`; INFRA-04 CI gate rejects the tag.
**Do this instead:** Use `make workspace` (writes `go.work`). For release, follow the coordinated bump pattern (Phase 33).

### Reaching into `*flow.Engine` private fields from a wrapper

**What happens:** A telemetry wrapper does `unsafe`-style reflection on `*flow.Engine` to extract the flow id for span attributes.
**Why it's wrong:** Private state is private; the freeze gate fails the moment the underlying field is renamed in a v0.1.x point release.
**Do this instead:** Target `flow.Runner` and use the exported `(*Engine).FlowID()` / `FlowName()` getters; the `otelflow.flowIdentifier` interface (`llm-agent-otel/otelflow/otelflow.go`) is the canonical pattern, and `otelflow.Config.FlowID` is the override hook for callers using an external identifier.

### Skipping per-event persistence on a flow run

**What happens:** A flowd-style server keeps events in-memory until the run terminates, then writes a single summary row.
**Why it's wrong:** A client that drops mid-stream loses all observability; replay (`POST /runs/{id}/replay`) becomes impossible; debugging a hung run is reduced to "the run died, no idea why."
**Do this instead:** Persist BEFORE forwarding for stream paths (the v0.0.6 / v0.1.x contract); batch-persist at the end for sync paths only (the v0.1.1 optimization via `(*sqlite.Store).AppendRunEvents`).

## Error Handling

**Strategy:** Sentinel errors at the contract boundary + wrapped errors for transport/parse failures.

**Patterns:**
- `llm.ErrCapabilityNotSupported` returned by adapters lacking a capability (e.g., Anthropic `Embed` — `llm-agent-providers/anthropic/errors.go`).
- `agents.ErrToolAlreadyRegistered` (`llm-agent/registry.go`).
- `budget.ErrBudgetExceeded` from the v1.2 budget package — agent integration returns BOTH the response AND the sentinel on post-call deny (see `agent_chatmodel.go`).
- `flowstore.ErrNotFound` / `ErrAlreadyExists` (`llm-agent-flow/flow/store/store.go`) — surfaced through flowd as 404 / 409.
- `server.ErrUnauthorized` (`llm-agent-flow/cmd/flowd/server/auth.go`) — surfaced as 401 + `WWW-Authenticate` Bearer challenge; any other Authenticator error → 403.
- Streaming: `io.EOF` for clean termination; `ctx.Err()` for cancellation; both propagate through `StreamReader.Next()` and `<-chan FlowEvent`.
- Provider streaming errors wrap the SDK's error via `wrapErr` (`llm-agent-providers/*/errors.go`).
- Async tools: a panic is recovered as `TaskResult.Err == *fanout.ErrTaskPanic` so one bad tool doesn't crash the process (`llm-agent/async.go`). Same per-layer fan-out is reused inside `flow.Engine` (`llm-agent-flow/flow/engine.go`).

## Cross-Cutting Concerns

**Logging:** `log/slog` only in core. `otelslog.NewHandler(...)` (`llm-agent-otel/otelslog/`) bridges slog records to OTel logs without forcing the core to depend on OTel.

**Validation:** JSON schema validation is delegated upstream — `Tool.Schema()` returns raw `json.RawMessage` and the provider validates (`llm-agent/tool.go`). Server-side `user_id` validation happens in the customer-support tool allowlist (`internal/guardrails/`). Flow JSON validation runs at `Load` + `Compile` time (`llm-agent-flow/flow/validate.go`).

**Authentication:** Provider auth is per-adapter (`OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, etc.) read inside each `New(...)`. The `/chat` HTTP surface has NO auth — documented demo-only limitation. **flowd** has a pluggable `server.Authenticator` interface with a bundled `BearerTokenAuthenticator` (constant-time comparison; `/healthz` bypassed); nil authenticator leaves the API open for v0.0.7-compatible callers.

**Observability:** OTel via `otelmodel.Wrap` / `otelagent.Wrap` / `otelrag.Wrap` / **`otelflow.Wrap`** (K3). Opt-in semconv emission gated by `OTEL_SEMCONV_STABILITY_OPT_IN=gen_ai_latest_experimental`; prompt/response content capture gated by `OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT=true` and routed through `otelmodel`'s built-in redactor. Flow telemetry uses the `flow.*` attribute namespace (not gen_ai semconv) defined in `llm-agent-otel/otelflow/config.go`.

**Budget / cancellation:** ctx-keyed via `budget.Tracker` on `context.Context` (`llm-agent/budget/`, KC-4); enforced once at the `generateFromPrompt` chokepoint in `llm-agent/agent_chatmodel.go`. No tracker on ctx → zero cost (opt-in).

**Persistence:** Pluggable per-domain. Customer-support uses `internal/sessionstore` (SQLite or Postgres). Flowd uses `flow/store` with the pure-Go `flow/store/sqlite` reference implementation; downstream callers may write a Postgres-backed `Store` without changing flowd.

**API stability gates:** `internal/apisnapshot/` + `api/v<N>.snapshot.txt` in both `llm-agent-rag` (v1.x) and `llm-agent-flow` (v0.1.x). Pure stdlib (`go/parser` + `go/printer`); runs on every `go test`; fails any drift; additive changes regenerate the baseline with `-update`.

---

*Architecture analysis: 2026-05-21*
