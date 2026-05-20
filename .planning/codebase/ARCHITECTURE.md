<!-- refreshed: 2026-05-20 -->
# Architecture

**Analysis Date:** 2026-05-20

## System Overview

```text
┌─────────────────────────────────────────────────────────────────────┐
│              llm-agent-ecosystem (umbrella, this repo)              │
│  `go.work` + `Makefile` + `scripts/eco.sh` + `scripts/workspace.sh` │
│  Coordination only — no product code lives here.                    │
└──────────────────────────────┬──────────────────────────────────────┘
                               │ orchestrates 5 sibling repos
        ┌──────────────────────┴──────────────────────┐
        ▼                                              ▼
┌──────────────────────────────┐    ┌──────────────────────────────────────┐
│ llm-agent-customer-support   │    │ llm-agent-otel                       │
│ Deployable demo service      │    │ OTel decorator wrappers              │
│ `cmd/server/main.go`         │    │ `otelmodel/`, `otelagent/`,          │
│ `internal/{app,httpapi,…}/`  │    │ `otelrag/`, `otelmetrics/`,          │
│ `compose/compose.yaml`       │    │ `otelslog/`                          │
└─────┬────────────┬───────────┘    └──────────┬───────────────────────────┘
      │            │                           │
      │ uses       │ uses                      │ wraps
      ▼            ▼                           ▼
┌──────────────────────────────┐    ┌──────────────────────────────────────┐
│ llm-agent-providers          │    │ llm-agent (core)                     │
│ Real provider adapters       │◀───│ Stdlib-only framework                │
│ `openai/`, `anthropic/`,     │    │ `agent.go`, `tool.go`, `registry.go`,│
│ `ollama/`, `deepseek/`,      │    │ `react.go`, `plan_solve.go`,         │
│ `minimax/`                   │    │ `reflection.go`, `chain.go`,         │
│ Each binds 1 model at New()  │    │ `simple.go`, `function_call.go`,    │
└──────────────┬───────────────┘    │ `agent_chatmodel.go`, `async.go`     │
               │                    │ `llm/` (ChatModel, StreamEvent,…)    │
               │ implements         │ `memory/`, `orchestrate/`, `budget/` │
               │ `llm.ChatModel`,   │ `rag/` (facade, re-exports rag SDK)  │
               │ `llm.ToolCaller`,  └──────────────┬───────────────────────┘
               │ `llm.Embedder`                    │
               ▼                                   │ RAG facade
       ┌──────────────────────────────────────────▼──────────────┐
       │ llm-agent-rag (standalone SDK, v1.0 frozen API)         │
       │ `rag/`, `ingest/`, `retrieve/`, `generate/`,            │
       │ `store/`, `embed/`, `graph/`, `prompt/`, `pack/`,       │
       │ `rerank/`, `eval/`, `obs/`, `guard/`, `postgres/`       │
       │ stdlib-only at v1.0.0 (pgx only in `postgres/` subpkg)  │
       └─────────────────────────────────────────────────────────┘
```

## Component Responsibilities

| Component | Responsibility | Path |
|-----------|----------------|------|
| Umbrella shell | go.work generation, multi-repo build / test / up / down, dependency-currency gate | `/home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/` |
| `llm-agent` | Agent paradigms, Tool/Registry, Memory, `llm.ChatModel` + capability interfaces, typed streaming events, async tool execution, RAG facade | `llm-agent/` |
| `llm-agent-rag` | Import/retrieve/generate primitives, GraphRAG (Louvain communities, DRIFT, path-ranked subgraphs), pgvector backend, `rag.Observer` hook | `llm-agent-rag/` |
| `llm-agent-providers` | Real OpenAI / Anthropic / Ollama / DeepSeek / MiniMax adapters; each implements the relevant `llm.*` capability interfaces | `llm-agent-providers/` |
| `llm-agent-otel` | `otelmodel.Wrap(ChatModel)`, `otelagent.Wrap(Agent)`, `otelrag.Wrap(System)`, low-cardinality metrics helpers, slog bridge, OTLP exporter wiring | `llm-agent-otel/` |
| `llm-agent-customer-support` | HTTP + SSE service, StateGraph triage, RAG knowledge lookup, durable session store, day-one guardrails, observability stack via compose | `llm-agent-customer-support/` |

## Pattern Overview

**Overall:** Umbrella + independently-versioned sibling repos. The umbrella is a coordination shell; product code lives in 5 sibling Git repos pinned together by `go.work` (local dev) and by `go.mod require` (release builds). Three keystone decisions define the cross-repo contract surface.

**Key Characteristics:**
- **Core stays stdlib-only.** `llm-agent/go.mod` lists exactly one non-stdlib require — `github.com/costa92/llm-agent-rag v1.0.1` — and nothing else. Every other non-stdlib dep is in a sister repo a caller opts into one `go get` at a time.
- **Each subproject is a separate GitHub repo with its own tags, branch, CI, and release cycle.** The umbrella's `scripts/eco.sh` clones the missing siblings on `make bootstrap`; `scripts/workspace.sh` writes the local `go.work`.
- **Dependency direction is acyclic and one-way.** `customer-support → providers + otel → llm-agent → llm-agent-rag`. `llm-agent-rag` is the fixed point every other repo aligns *to*.
- **`replace` directives are a local-only escape hatch.** The release-precheck CI gate refuses to tag a commit whose `go.mod` carries a `replace`. `go.work` is `.gitignore`d in every repo and CI runs with `GOWORK=off`.

## Why each subproject is a separate repo

- **Independent versioning.** `llm-agent v0.5.1`, `llm-agent-rag v1.0.1`, `llm-agent-otel v0.2.1`, `llm-agent-providers v0.2.1`, `llm-agent-customer-support v0.2.2` — each cuts its own SemVer track. A breaking RAG API change can go to `llm-agent-rag/v2` without forcing a core bump.
- **Separate CI surfaces.** Provider adapters need Docker for `testcontainers` Ollama coverage; core does not. RAG needs a Postgres+pgvector container for the conformance suite; core does not. Splitting CI keeps the core run fast and stdlib-only.
- **Focused docs and README per repo.** Each repo's `README.md` is its own surface area; the umbrella README is only a navigation index.
- **`go.mod` blast radius.** Importing `llm-agent` does not pull in OpenAI/Anthropic SDKs or OTel exporters; those only arrive when the caller explicitly `go get`s a provider or `otel` package.
- **Tagged-release isolation.** `INFRA-04` refuses to tag a commit with a `replace` directive, so every release of every repo is reproducible from `proxy.golang.org` alone.
- **Coordinated bump pattern (Phase 33).** When all repos need to move together (e.g., the v1.1 ecosystem-alignment wave on 2026-05-21), they're re-tagged in dependency order: rag → core → otel → providers → customer-support.

## Layers

**Umbrella shell (this repo):**
- Purpose: workspace coordination only; no product source.
- Location: `/home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/`
- Contains: `go.work`, `Makefile`, `scripts/eco.sh`, `scripts/workspace.sh`, `docs/`, `.planning/`.
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
- Depends on: stdlib only at v1.0.0; `postgres/` subpackage pulls `pgx/v5` + `pgvector-go`.
- Used by: `llm-agent` (facade), `llm-agent-otel` (`otelrag/`), `llm-agent-customer-support` (transitive).

**Providers (`llm-agent-providers`):**
- Purpose: real provider adapters implementing `llm.ChatModel` and capability interfaces.
- Location: `llm-agent-providers/{openai,anthropic,ollama,deepseek,minimax}/`
- Depends on: `llm-agent v0.5.1` + each provider's official Go SDK.
- Used by: `llm-agent-customer-support`.

**OTel decorators (`llm-agent-otel`):**
- Purpose: capability-preserving OpenTelemetry wrappers — `otelmodel.Wrap`, `otelagent.Wrap`, `otelrag.Wrap`.
- Location: `llm-agent-otel/{otelmodel,otelagent,otelrag,otelmetrics,otelslog}/`
- Depends on: `llm-agent v0.5.1` + `llm-agent-rag v1.0.1` + OTel SDKs.
- Used by: `llm-agent-customer-support`.

**Reference service (`llm-agent-customer-support`):**
- Purpose: deployable demo that wires the full stack.
- Location: `llm-agent-customer-support/{cmd/server,internal/...,compose/}`
- Depends on: core + providers + otel (and transitively rag).
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

Why decorator, not hook: a hook lives inside the core type and forces every adapter to know about OTel; the decorator lives in a sister repo and is opt-in per call site. The core `llm-agent` carries zero OTel symbols.

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

## Customer-support service architecture

`llm-agent-customer-support/cmd/server/main.go` loads config, installs signal handling, calls `app.New(...)`, runs `http.Server` until SIGINT/SIGTERM.

`internal/app/app.go` is the composition root. Its `Options` exposes four factory seams (`ModelFactory`, `EmbedderFactory`, `SessionStoreFactory`, `TracerProviderFactory`) so tests can inject doubles. It builds in order:

1. **Config + tracer.** `internal/config/config.go` parses env (provider-aware defaults for openai / anthropic / ollama); `TracerProviderFactory` wires OTLP via `otelroot.NewTracerProvider`.
2. **Chat + embed providers — independently selected.** `internal/providers/providers.go` is the split chat/embedding factory: `LLM_PROVIDER=openai|anthropic|ollama|deepseek|minimax` and `EMBEDDING_PROVIDER=openai|ollama` are separate knobs, so `anthropic` chat can co-exist with `ollama` embeddings.
3. **OTel decorators.** `otelmodel.Wrap(model)` (K3) on the chat model; `otelagent.Wrap(agent)` on the final agent.
4. **Session store.** `internal/sessionstore/` exposes a `Store` contract with SQLite (`modernc.org/sqlite`) and Postgres (`lib/pq`) implementations selected by `SESSION_BACKEND=sqlite|postgres`. `context.go` propagates session ID through the request context.
5. **Knowledge base.** `internal/knowledgebase/` seeds a RAG corpus on boot (uses the `llm-agent-rag` SDK transitively).
6. **Limits guard.** `internal/limits/limits.go` enforces config-driven hard caps from Day 1 (K7): `MAX_TOKENS_PER_REQUEST`, `MAX_TOOL_CALLS_PER_AGENT_LOOP`, `MAX_REQUESTS_PER_IP_PER_MINUTE`, `RETRY_MAX_ATTEMPTS`, `DAILY_TOKEN_BUDGET`, plus `DISABLE_LLM=1` live panic switch (returns 503 without restart).
7. **Guardrails.** `internal/guardrails/guardrails.go` runs day-one prompt-injection defenses: suspicious-input filter with safe fallback, tool allowlist with server-side `user_id` enforcement, retrieved RAG content marked untrusted in the system-prompt path.
8. **Support flow.** `internal/supportflow/supportflow.go` is typed StateGraph triage — chargeback/fraud → human handoff, missing order ID → clarification, refund/order → tool-backed RAG lookup. `toolagent.go` wires the FunctionCallAgent variant.
9. **HTTP transport.** `internal/httpapi/httpapi.go` exposes `POST /chat`, `POST /chat/stream` (SSE), `GET /healthz`, `GET /readyz`, propagates `X-Trace-Id` and `X-Session-Id`, returns 429 on preflight cap failure.
10. **Compose stack.** `compose/compose.yaml` brings up app + Ollama + `grafana/otel-lgtm` + an OpenTelemetry Collector with tail-sampling (`compose/otel-collector.yaml`): 100% errors, 100% latency >5s, 10% baseline. Pre-provisioned Grafana dashboard at `dashboards/customer-support-observability.json`.

## Data Flow

### Primary chat request (`POST /chat`)

1. HTTP handler at `llm-agent-customer-support/internal/httpapi/httpapi.go` receives the request, resolves/creates `session_id`.
2. `internal/limits/limits.go` preflight: rate-limit, token cap, daily budget, panic switch. Failure → 429 / 503.
3. `internal/guardrails/guardrails.go` filters input; suspicious content takes the safe-fallback path.
4. `internal/supportflow/supportflow.go` classifies the intent (chargeback / refund / unknown) and selects the route.
5. RAG path: knowledge lookup against the seeded `rag.System` from `llm-agent-rag`; retrieved chunks are marked untrusted before injection into the system prompt.
6. `agents.Agent` (wrapped by `otelagent.Wrap`) executes. The bound `ChatModel` is wrapped by `otelmodel.Wrap`. Streaming consumers key on `StreamEvent.ToolCall.Index` (K1).
7. Session writes go to `internal/sessionstore` (SQLite or Postgres).
8. Response carries `X-Trace-Id` for cross-referencing in Grafana.

### Streaming chat (`POST /chat/stream`)

1–5 identical.
6. `agent.RunStream(ctx, input)` returns `<-chan StepEvent`. Handler converts to SSE; discards `res.Trace` after the channel closes (Trace memory contract — `agent.go`).
7–8 identical.

**State Management:**
- Per-request: ctx-keyed (budget tracker via `budget.From(ctx)`, session ID via `sessionstore` context helpers).
- Per-session: durable in SQLite/Postgres.
- Per-process: tool `Registry`, knowledge-base `rag.System` instance, OTel `TracerProvider` (all owned by `App`).

## Key Abstractions

**`StreamEvent` typed union:**
- Purpose: provider-native granularity surfaced to consumers without lowest-common-denominator loss.
- Examples: `llm-agent/llm/stream.go` (definition), `llm-agent-providers/openai/openai.go` (emit), `llm-agent-customer-support/internal/httpapi/httpapi.go` (SSE consume).
- Pattern: typed-union (K1); stable per-tool-call `Index`.

**`Capabilities` value struct:**
- Purpose: runtime, JSON-serializable per-(provider × model) capability signal.
- Examples: `llm-agent/llm/info.go` (definition); each `*/options.go` in `llm-agent-providers` (where `New()` sets it).
- Pattern: K2 — bound at construction.

**Decorator `Wrap(inner) Inner`:**
- Purpose: opt-in cross-cutting concern without touching the inner type.
- Examples: `llm-agent-otel/otelmodel/otelmodel.go`, `otelagent/otelagent.go`, `otelrag/otelrag.go`, and the planned v1.2 `policy.Wrap`.
- Pattern: K3 — decorator that preserves capability interfaces via 2ⁿ wrapper types.

## Entry Points

**Umbrella build entry points:**
- Location: `Makefile` at repo root.
- Triggers: `make bootstrap | workspace | pull | status | build | test | up | down`.
- Responsibilities: dispatch to `scripts/eco.sh <action> $TARGETS` or `scripts/workspace.sh`.

**Customer-support runtime entry point:**
- Location: `llm-agent-customer-support/cmd/server/main.go`.
- Triggers: `docker compose -f compose/compose.yaml up --build` or direct `go run`.
- Responsibilities: load config → build `App` → run HTTP server → graceful shutdown on signal.

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
- **No `replace` in tagged-release branches.** INFRA-04 gate; `replace` is local-dev only.
- **`go.work` is `.gitignore`d in every repo; CI runs `GOWORK=off`.** Local cross-repo dev uses `make workspace`; release builds use module proxy.
- **No K8s / Helm anywhere.** Standing non-goal; see Pitfall 16. Half-shipped K8s is worse than none.
- **`llm-agent-rag` v1.x public API is additive-only.** Breaking changes go to a `/v2` module path. v1.0 froze the surface via `internal/apisnapshot` + `api/v1.snapshot.txt`.
- **Capabilities are per-(provider × model).** Not per-provider, not per-type. `Info().Capabilities` is the runtime truth.
- **`WithTools` / `WithSchema` are immutable.** No mutation of bound state.
- **Threading.** `ChatModel` implementations MUST be safe for concurrent use; concurrent `Generate` / `Stream` on the same value is part of the contract (`llm/chatmodel.go` doc-comment).
- **Trace memory contract.** Streaming consumers MUST discard `Result.Trace` after the channel closes — same information twice would otherwise hold 50–100 Steps (~4KB each) per in-flight handler (`agent.go`).

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
**Do this instead:** `otelmodel.Wrap(provider) llm.ChatModel` (K3). The wrapper preserves capability interfaces; the core stays stdlib-only.

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
**Do this instead:** Use `make workspace` (writes `go.work`, which is `.gitignore`d). For release, follow the coordinated bump pattern (Phase 33).

## Error Handling

**Strategy:** Sentinel errors at the contract boundary + wrapped errors for transport/parse failures.

**Patterns:**
- `llm.ErrCapabilityNotSupported` returned by adapters lacking a capability (e.g., Anthropic `Embed` — `llm-agent-providers/anthropic/errors.go`).
- `agents.ErrToolAlreadyRegistered` (`llm-agent/registry.go`).
- `budget.ErrBudgetExceeded` from the v1.2 budget package — agent integration returns BOTH the response AND the sentinel on post-call deny (see `agent_chatmodel.go`).
- Streaming: `io.EOF` for clean termination; `ctx.Err()` for cancellation; both propagate through `StreamReader.Next()`.
- Provider streaming errors wrap the SDK's error via `wrapErr` (`llm-agent-providers/*/errors.go`).
- Async tools: a panic is recovered as `TaskResult.Err == *fanout.ErrTaskPanic` so one bad tool doesn't crash the process (`llm-agent/async.go`).

## Cross-Cutting Concerns

**Logging:** `log/slog` only in core. `otelslog.NewHandler(...)` (`llm-agent-otel/otelslog/`) bridges slog records to OTel logs without forcing the core to depend on OTel.

**Validation:** JSON schema validation is delegated upstream — `Tool.Schema()` returns raw `json.RawMessage` and the provider validates (`llm-agent/tool.go`). Server-side `user_id` validation happens in the customer-support tool allowlist (`internal/guardrails/`).

**Authentication:** Provider auth is per-adapter (`OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, etc.) read inside each `New(...)`. The `/chat` HTTP surface has NO auth — documented demo-only limitation.

**Observability:** OTel via `otelmodel.Wrap` / `otelagent.Wrap` / `otelrag.Wrap` (K3). Opt-in semconv emission gated by `OTEL_SEMCONV_STABILITY_OPT_IN=gen_ai_latest_experimental`; prompt/response content capture gated by `OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT=true` and routed through `otelmodel`'s built-in redactor.

**Budget / cancellation:** ctx-keyed via `budget.Tracker` on `context.Context` (`llm-agent/budget/`, KC-4); enforced once at the `generateFromPrompt` chokepoint in `llm-agent/agent_chatmodel.go`. No tracker on ctx → zero cost (opt-in).

---

*Architecture analysis: 2026-05-20*
