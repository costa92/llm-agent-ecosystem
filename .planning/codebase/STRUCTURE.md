# Codebase Structure

**Analysis Date:** 2026-05-21

## Directory Layout

```text
llm-agent-ecosystem/                  # umbrella shell — coordination only
├── go.work                           # workspace pinning 6 sibling modules
├── Makefile                          # bootstrap / workspace / build / test / up / down
├── PROJECT.md                        # one-line project descriptor
├── README.md                         # navigation index (umbrella, not product docs)
├── .gitignore
├── .planning/                        # umbrella-level planning + this codebase map
│   ├── PROJECT.md
│   ├── README.md
│   └── codebase/
│       ├── ARCHITECTURE.md           # written by this gsd:map-codebase run
│       └── STRUCTURE.md              # ← this file
├── .github/                          # umbrella CI (dependency-currency gate, etc.)
├── .agents/                          # agent runtime convention dir
├── .codex/                           # codex CLI metadata
├── docs/                             # umbrella docs index + project analyses
│   ├── README.md
│   ├── current-project-analysis.md
│   └── current-project-analysis.zh-CN.md
├── scripts/                          # umbrella scripts
│   ├── eco.sh                        # multi-repo dispatcher (6 repos)
│   └── workspace.sh                  # writes the local go.work over 6 modules
├── llm-agent/                        # core framework — stdlib-only
├── llm-agent-rag/                    # standalone RAG SDK (frozen v1.x API)
├── llm-agent-otel/                   # OTel decorator wrappers (incl. otelflow)
├── llm-agent-providers/              # provider adapters
├── llm-agent-customer-support/       # demo service (incl. flowrunner bridge)
└── llm-agent-flow/                   # serializable flow IR + DAG executor (v0.1.x)
```

## Directory Purposes

**`/`(umbrella root):**
- Purpose: coordination shell; no product source code.
- Contains: `go.work` (workspace pinning 6 modules), `Makefile` (bootstrap / build / test / up / down), `README.md` (navigation index), `PROJECT.md` (one-line descriptor), `.planning/` (this map), `docs/`, `scripts/`.
- Key files: `go.work`, `Makefile`, `scripts/eco.sh`, `scripts/workspace.sh`.

**`.planning/`:**
- Purpose: umbrella-level planning. The **product** source-of-truth planning lives in `llm-agent/.planning/`, not here.
- Contains: `PROJECT.md`, `README.md`, `codebase/` (this map).
- Key files: `codebase/ARCHITECTURE.md`, `codebase/STRUCTURE.md`.

**`docs/`:**
- Purpose: human-facing umbrella docs.
- Contains: index README + two project-analysis docs (EN + zh-CN).
- Key files: `current-project-analysis.md`.

**`scripts/`:**
- Purpose: workspace orchestration shell scripts.
- Contains: `eco.sh` (`bootstrap | pull | status | build | test | up | down`), `workspace.sh` (writes `go.work` over 6 modules).
- Key files: `eco.sh` (driver — now ships the 6-repo list including `llm-agent-flow`), `workspace.sh`.

**`llm-agent/`** — see subproject detail below.
**`llm-agent-rag/`** — see subproject detail below.
**`llm-agent-otel/`** — see subproject detail below.
**`llm-agent-providers/`** — see subproject detail below.
**`llm-agent-customer-support/`** — see subproject detail below.
**`llm-agent-flow/`** — see subproject detail below.

## Subproject layouts

### `llm-agent/` — core framework (v0.5.1, 135 Go files, ~19,358 lines)

```text
llm-agent/
├── go.mod                            # require: llm-agent-rag v1.0.1 (only non-stdlib line)
├── go.sum
├── README.md
├── CHANGELOG.md
├── CLAUDE.md                         # agent-runtime contract notes
├── DEPRECATIONS.md
├── PROVIDER_AUTHORING.md             # how to write a provider
├── LICENSE
├── OWNERS
├── doc.go
├── .planning/                        # ← THE planning source-of-truth for the ecosystem
│   ├── PROJECT.md                    # what the project is, core value, hard rules
│   ├── STATE.md                      # current milestone (v1.2 active) + active phase
│   ├── ROADMAP.md                    # phase plan for active milestone
│   ├── REQUIREMENTS.md               # active milestone requirements + traceability
│   ├── config.json
│   ├── milestones/                   # archived milestone roadmaps
│   ├── phases/                       # per-phase plans
│   ├── research/                     # cross-cut audits, keystone decisions
│   ├── todos/
│   ├── v0.3-MILESTONE-AUDIT.md … v1.1-MILESTONE-AUDIT.md
│   ├── v1.2-REQUIREMENTS.md
│   └── v1.2-ROADMAP.md
├── llm/                              # capability surface (ChatModel + interfaces)
│   ├── chatmodel.go                  # ChatModel interface (Generate / Stream / Info)
│   ├── capabilities.go               # ToolCaller, Embedder, StructuredOutputs
│   ├── stream.go                     # StreamEvent typed union (Keystone K1)
│   ├── info.go                       # ProviderInfo + Capabilities struct (K2)
│   ├── types.go                      # Request / Response / Message / Usage / etc.
│   ├── errors.go
│   ├── scripted.go                   # ScriptedLLM for tests
│   ├── chat_only_mock.go
│   ├── doc.go                        # canonical capability-negotiation idiom
│   ├── errors_test.go
│   └── llm_test.go
├── agent.go                          # Agent interface + Result + StepEvent + Step
├── agent_test.go
├── tool.go                           # Tool interface + AsLLMTool + NewFuncTool
├── tool_test.go
├── registry.go                       # Registry (per-Agent name→Tool map)
├── registry_test.go
├── simple.go                         # SimpleAgent paradigm
├── react.go                          # ReActAgent paradigm
├── plan_solve.go                     # PlanAndSolveAgent paradigm
├── reflection.go                     # ReflectionAgent paradigm
├── chain.go                          # Chain (sequential tool pipe, itself a Tool)
├── function_call.go                  # FunctionCallAgent (native tool calling)
├── agent_chatmodel.go                # generateFromPrompt + budget chokepoint
├── async.go                          # AsyncRunner (parallel tool execution)
├── budget/                           # v1.2 budget tracker (CC-1)
├── budget_integration_test.go
├── memory/                           # Working / Episodic / Semantic memory
├── orchestrate/                      # multi-agent coordination primitives
├── pkg/
│   └── fanout/                       # public goroutine fan-out helper (reused by flow.Engine)
├── rag/                              # RAG facade (empty dir; types re-exported elsewhere)
├── builtin/                          # built-in tools
├── comm/                             # inter-agent communication
├── context/                          # ctx-keyed helpers
├── rl/                               # reinforcement-learning utilities
├── internal/
│   └── testenv/
├── third_party/
├── docs/                             # core repo docs
├── examples/                         # separate Go module (deps don't enter core)
├── bench/
├── scripts/
└── example_*_test.go                 # in-package runnable example docs
```

**Tests:** co-located `_test.go` files throughout. In-package example tests (`example_*_test.go`) double as runnable docs. `bench/` for benchmarks. `examples/` is a separate Go module to keep example deps out of the core module.

**Docs:** `README.md`, `CHANGELOG.md`, `PROVIDER_AUTHORING.md`, `DEPRECATIONS.md`, `CLAUDE.md`, `docs/`.

### `llm-agent-rag/` — standalone RAG SDK (v1.0.2, 125 Go files, ~20,904 lines)

```text
llm-agent-rag/
├── go.mod                            # require: pgx/v5, pgvector-go (postgres subpkg only)
├── go.sum
├── README.md
├── CHANGELOG.md
├── LICENSE
├── doc.go
├── api/                              # v1 exported-surface snapshot (frozen)
├── rag/                              # orchestration layer (System, Ask, AskGlobal, AskDrift)
├── ingest/                           # documents, sources, splitters
├── embed/                            # Embedder seam + default HashEmbedder
├── store/                            # vector store seam + InMemoryStore + storetest
├── postgres/                         # pgvector backend (opt-in deps)
├── retrieve/                         # hybrid retrieval, BM25, structure-aware, route policy
├── pack/                             # token-budget context packing
├── rerank/                           # heuristic reranker
├── generate/                         # text-generation seam
├── prompt/                           # prompt template seam + default QA template
├── eval/                             # dataset format + metrics + JSONL loader (CI gate)
├── tree/                             # document-tree primitives for structured corpora
├── graph/                            # GraphRAG construction + traversal
├── obs/                              # observability hooks (consumed by otelrag)
├── guard/                            # content safety (PII redaction, injection defense)
├── agentic/                          # agentic retrieval loops
├── adapter/                          # adapter/llmagent (build tag `llmagent`)
├── advanced/
├── feedback/                         # feedback loop
├── contract/                         # cross-repo contract gates
├── examples/
├── docs/                             # production-deployment, backend-selection, compatibility
└── internal/                         # apisnapshot — v1 freeze gate
```

**Tests:** co-located `_test.go` (e.g., `rag/system_test.go`, `rag/community_test.go`, `rag/drift_test.go`).

**Docs:** `README.md`, `CHANGELOG.md`, `docs/production-deployment.md`, `docs/backend-selection.md`, `docs/core-compatibility.md`, `docs/compatibility.md` (the v1.x additive-only promise).

### `llm-agent-otel/` — OTel decorator wrappers (v0.2.2, 25 Go files, ~2,936 lines)

```text
llm-agent-otel/
├── go.mod                            # require: llm-agent + llm-agent-rag + llm-agent-flow v0.0.7 + OTel SDKs
├── go.sum
├── README.md
├── LICENSE
├── OWNERS
├── exporters.go                      # OTLP exporter constructor
├── exporters_grpc.go
├── exporters_http.go
├── exporters_test.go
├── semconv_gen_ai.go                 # gen_ai.* attribute constants + gates
├── semconv_gen_ai_test.go
├── otelmodel/                        # llm.ChatModel decorator (K3)
│   ├── otelmodel.go                  # Wrap(model) preserves ToolCaller/Embedder/StructuredOutputs
│   ├── config.go
│   ├── semconv_gen_ai.go
│   └── otelmodel_test.go
├── otelagent/                        # agents.Agent decorator
│   ├── otelagent.go
│   ├── config.go
│   └── otelagent_test.go
├── otelrag/                          # rag.System decorator (via rag.Observer)
│   ├── otelrag.go
│   ├── metrics.go
│   └── otelrag_test.go
├── otelflow/                         # ⭐ NEW v0.2.2 — flow.Runner decorator (K3 application)
│   ├── doc.go                        # package docs — wrap pattern + span shape
│   ├── otelflow.go                   # Wrap(inner flow.Runner, cfg Config) flow.Runner
│   ├── config.go                     # Config + flow.* attribute key constants
│   └── otelflow_test.go              # per-flow + per-node span coverage
├── otelmetrics/                      # low-cardinality metrics helpers
│   ├── otelmetrics.go
│   └── otelmetrics_test.go
├── otelslog/                         # slog → OTel logs bridge
│   ├── otelslog.go
│   └── otelslog_test.go
├── cmd/
│   └── tailprobe/                    # demo emitter for the compose stack
├── compose/
│   ├── compose.yaml                  # grafana/otel-lgtm single-container demo
│   └── demo/
└── scripts/
```

**Tests:** co-located `_test.go` per package (e.g., `otelmodel/otelmodel_test.go`, `otelflow/otelflow_test.go`).

**Docs:** `README.md` (Quick-start, exporter defaults, opt-in semantics, demo compose flow).

**Dep edge added in v0.2.2:** `llm-agent-otel → llm-agent-flow` (the `otelflow/` package's compile-time import of `github.com/costa92/llm-agent-flow/flow`).

### `llm-agent-providers/` — provider adapters (v0.2.2, 36 Go files, ~6,220 lines)

```text
llm-agent-providers/
├── go.mod                            # require: llm-agent + each provider's official SDK
├── go.sum
├── README.md
├── LICENSE
├── OWNERS
├── openai/
│   ├── openai.go                     # OpenAI struct: implements ChatModel + ToolCaller + Embedder
│   ├── options.go                    # WithModel / WithAPIKey / WithBaseURL / etc.
│   ├── map.go                        # llm.Request ↔ openai SDK request mapping
│   ├── errors.go                     # wrapErr
│   ├── doc.go
│   ├── openai_test.go
│   └── README.md
├── anthropic/                        # ChatModel + ToolCaller (Embed returns ErrNotSupported)
├── ollama/                           # ChatModel + ToolCaller (model-aware) + Embedder
├── deepseek/                         # ChatModel + ToolCaller, regional endpoint presets
├── minimax/                          # ChatModel + ToolCaller, regional endpoint presets
├── internal/
│   └── contract/                     # shared fixture-driven cross-provider conformance suite
├── docs/
│   └── superpowers/
└── scripts/
```

**Tests:** co-located per provider + shared `internal/contract/` conformance suite + nightly live Ollama CI path.

**Docs:** top-level `README.md` + per-provider `README.md`.

### `llm-agent-customer-support/` — demo service (v0.2.3, 27 Go files, ~4,104 lines)

```text
llm-agent-customer-support/
├── go.mod                            # require: llm-agent + providers + otel + llm-agent-flow v0.1.1 + uuid + lib/pq + sqlite + OTel SDKs
├── go.sum
├── README.md
├── LICENSE
├── OWNERS
├── cmd/
│   └── server/
│       ├── main.go                   # signal handling + App lifecycle (RUNTIME ENTRY POINT)
│       └── main_test.go
├── internal/
│   ├── app/
│   │   ├── app.go                    # COMPOSITION ROOT — wires model+embedder+session+agent+mux
│   │   └── app_test.go
│   ├── config/                       # env parsing + provider-aware defaults
│   │   ├── config.go
│   │   └── config_test.go
│   ├── providers/                    # split chat/embedding factory seam
│   │   └── providers.go
│   ├── httpapi/                      # POST /chat, POST /chat/stream (SSE), /healthz, /readyz
│   │   ├── httpapi.go
│   │   └── httpapi_test.go
│   ├── limits/                       # config-driven hard caps + DISABLE_LLM panic switch (K7)
│   │   ├── limits.go
│   │   └── limits_test.go
│   ├── guardrails/                   # day-one prompt-injection defenses
│   │   ├── guardrails.go
│   │   └── guardrails_test.go
│   ├── supportflow/                  # StateGraph triage + RAG lookup + tool agent
│   │   ├── supportflow.go
│   │   ├── toolagent.go
│   │   ├── doc.go
│   │   ├── supportflow_test.go
│   │   └── toolagent_test.go
│   ├── flowrunner/                   # ⭐ NEW v0.2.3 — bridge to llm-agent-flow + otelflow
│   │   ├── doc.go                    # scope: load → compile → otelflow.Wrap → Run
│   │   ├── flowrunner.go             # Runner: Execute / ExecuteStream / Register
│   │   └── flowrunner_test.go        # end-to-end: echo flow + traced child spans
│   ├── knowledgebase/                # seeded RAG corpus on boot
│   │   ├── knowledgebase.go
│   │   └── knowledgebase_test.go
│   └── sessionstore/                 # SQLite + Postgres durable session contract
│       ├── sessionstore.go
│       ├── context.go
│       └── sessionstore_test.go
├── compose/
│   ├── compose.yaml                  # app + Ollama + grafana/otel-lgtm + Collector
│   ├── otel-collector.yaml           # tail-sampling policy
│   ├── Dockerfile
│   ├── grafana/                      # provisioned dashboards / datasources
│   └── assets_test.go
├── dashboards/
│   └── customer-support-observability.json
└── scripts/
```

**Tests:** co-located per package; `compose/assets_test.go` verifies the compose/dashboard assets parse cleanly.

**Docs:** `README.md` (compose quick-start, env knobs, hard caps, cross-repo iteration pattern).

**Dep edge added in v0.2.3:** `llm-agent-customer-support → llm-agent-flow` (the `internal/flowrunner/` package directly imports `github.com/costa92/llm-agent-flow/flow` + `github.com/costa92/llm-agent-otel/otelflow`). `go.mod` lists `llm-agent-flow v0.1.1` as a direct require.

### `llm-agent-flow/` — flow IR + DAG executor (v0.1.1, 57 Go files, ~7,491 lines)

```text
llm-agent-flow/
├── go.mod                            # require: llm-agent v0.5.1 + cel-go + modernc.org/sqlite
├── go.sum
├── README.md
├── CHANGELOG.md                      # v0.0.1 → v0.0.9 → v0.1.0 (SemVer freeze) → v0.1.1 (perf)
├── LICENSE
├── doc.go                            # package llmflow — module documentation anchor
├── .gitignore
├── .github/                          # repo CI
├── api/
│   └── v0.1.snapshot.txt             # ⭐ committed v0.1 exported-API baseline
├── docs/
│   └── compatibility.md              # written stability promise (additive-only v0.1.x)
├── internal/
│   └── apisnapshot/                  # ⭐ pure-stdlib snapshot gate (go/parser + go/printer)
├── flow/                             # STABLE v0.1.x library surface — stdlib-only outside back-edge to llm-agent
│   ├── doc.go                        # package overview
│   ├── ir.go                         # Flow / Node / Edge / PortRef / NamedPortRef
│   ├── ir_test.go
│   ├── node.go                       # NodeKind / NodeFactory / NodeRegistry / Port / Tool / ToolMap / ToolLookup / Deps
│   ├── tool_node.go                  # ToolNode adapter (wraps agents.Tool as a flow node)
│   ├── adapter_llmagent.go           # FromAgentTool / FromAgentTools helpers
│   ├── validate.go                   # Validate: cycles, dangling edges, dup ids
│   ├── validate_test.go
│   ├── engine.go                     # Engine: Compile / Run / RunStream + WithMaxNodeConcurrency / WithConditionEvaluator
│   ├── engine_test.go
│   ├── engine_cond_test.go           # CEL-routed flows: skip propagation, evaluator-required
│   ├── engine_parallel_test.go       # per-layer fanout via pkg/fanout
│   ├── event.go                      # FlowEvent typed union (FlowStarted ... FlowDone / FlowErr)
│   ├── condition.go                  # Condition / ConditionEvaluator / CondEnv
│   ├── runner.go                     # ⭐ v0.0.7 — flow.Runner interface + (*Engine).FlowID/Name getters
│   ├── runner_test.go
│   ├── cond/
│   │   └── cel/                      # STABLE — CEL evaluator implementing flow.ConditionEvaluator
│   │       ├── cel.go                # NewEvaluator (cel-go-backed)
│   │       ├── cel_test.go
│   │       └── helpers_test.go
│   ├── store/                        # STABLE — Store contract + pluggable backend
│   │   ├── doc.go
│   │   ├── store.go                  # Store interface, FlowMeta/FlowRecord, RunMeta/RunRecord,
│   │   │                             # RunStatus, RunEvent / RunEventKind, RunEventBatchItem (v0.1.1),
│   │   │                             # ErrNotFound, ErrAlreadyExists
│   │   └── sqlite/                   # STABLE — pure-Go SQLite backend (modernc.org/sqlite, no CGO)
│   │       ├── doc.go
│   │       ├── open.go               # Open(dsn); idempotent schema migration on every Open
│   │       ├── flows.go              # PutFlow / GetFlow / ListFlows / DeleteFlow
│   │       ├── runs.go               # StartRun / FinishRun / GetRun / ListRuns
│   │       ├── events.go             # AppendRunEvent / ListRunEvents (v0.0.6)
│   │       ├── events_batch_test.go  # v0.1.1 — AppendRunEvents batch insert
│   │       ├── events_test.go        # v0.0.6 — per-event persistence
│   │       └── store_test.go         # v0.0.5 — flow + run CRUD lifecycle
│   └── tools/                        # STABLE — tool-manifest format
│       ├── doc.go
│       ├── manifest.go               # Manifest + Entry + KindRegistry + LoadManifest / LoadAndBuild
│       ├── manifest_test.go
│       ├── http.go                   # built-in "http" kind — POST JSON args, decode {output} | raw body
│       ├── http_test.go
│       ├── exec.go                   # built-in "exec" kind — spawn cmd, stdin JSON, stdout capture
│       └── exec_test.go
├── cmd/
│   ├── flow/                         # one-shot CLI: flow run <file.json> --input k=v [--stream] [--tools ...]
│   │   └── main.go
│   └── flowd/                        # ⭐ long-running HTTP service — v0.1.x stable wire shape
│       ├── main.go                   # flags: --addr / --db / --flow / --tools / --token / FLOWD_TOKEN env
│       ├── helpers.go                # seedFlow + bytesReader
│       └── server/                   # STABLE — server.Server + Config + Authenticator
│           ├── server.go             # New / NewMux / Handler — full REST surface
│           ├── helpers.go
│           ├── auth.go               # ⭐ v0.0.8 — Authenticator + BearerTokenAuthenticator + ErrUnauthorized
│           ├── lru.go                # ⭐ v0.1.1 — LRU-bounded compiled-engine cache (Config.EngineCacheSize)
│           ├── lru_test.go
│           ├── auth_test.go
│           ├── server_test.go        # /healthz, /run, /run/stream
│           ├── server_crud_test.go   # /flows CRUD + /flows/{id}/run + /flows/{id}/runs + /runs/{id}
│           ├── server_events_test.go # v0.0.6 — every event persists; GET /runs/{id}/events
│           └── server_replay_test.go # ⭐ v0.0.9 — POST /runs/{id}/replay re-streams from store
└── examples/
    ├── echo_chain/                   # 2-node demo: upper → reverse
    ├── http_tool/                    # HTTP-backed tool manifest end-to-end
    └── router/                       # 2-branch CEL-routed flow (greet / other)
```

**Tests:** co-located `_test.go` throughout; `internal/apisnapshot/` gate runs on every `go test`; `examples/*/example_test.go` provide integration coverage for the CLI / SSE / replay paths.

**Docs:** `README.md` (quick-start CLI + flowd, full HTTP surface table, JSON IR shape, conditional routing), `CHANGELOG.md` (per-tag deltas from v0.0.1 through v0.1.1), `docs/compatibility.md` (the v0.1.x additive-only promise).

**Two stability tiers** (per `docs/compatibility.md` + `api/v0.1.snapshot.txt`):
- **STABLE in v0.1.x:** `flow/`, `flow/cond/cel`, `flow/store`, `flow/store/sqlite`, `flow/tools`, `cmd/flowd/server` — every exported symbol is additive-only. The flowd HTTP wire shape is also v0.1.x stable (every endpoint listed in README under "Implemented").
- **UNSTABLE:** `internal/apisnapshot/` — the freeze gate itself; not importable outside the module.
- The gate is pure stdlib (`go/parser` + `go/printer`), runs on every `go test`, fails any drift against `api/v0.1.snapshot.txt`, and accepts deliberate additive changes via `go test ./internal/apisnapshot/ -run TestAPISnapshot -update`.

## Where tests / examples / docs live

| Subproject | Tests | Examples | Docs |
|---|---|---|---|
| `llm-agent/` | co-located `_test.go`; `bench/`; `example_*_test.go` doubling as runnable docs | `examples/` (separate Go module) | `README.md`, `CHANGELOG.md`, `PROVIDER_AUTHORING.md`, `DEPRECATIONS.md`, `CLAUDE.md`, `docs/` |
| `llm-agent-rag/` | co-located `_test.go`; `eval/` (CI eval gate); `store/storetest` (shared conformance suite); `internal/apisnapshot/` (v1 freeze gate) | `examples/` | `README.md`, `docs/{production-deployment,backend-selection,core-compatibility,compatibility}.md` |
| `llm-agent-otel/` | co-located `_test.go` per package (otelmodel / otelagent / otelrag / otelflow / otelmetrics / otelslog) | `cmd/tailprobe/`, `compose/demo/` | `README.md` |
| `llm-agent-providers/` | co-located per provider + `internal/contract/` shared suite + nightly live Ollama path | none top-level (each provider README has snippets) | `README.md` + per-provider `README.md`, `docs/superpowers/` |
| `llm-agent-customer-support/` | co-located per package + `compose/assets_test.go` + new `internal/flowrunner/flowrunner_test.go` | none — the service itself is the example | `README.md` |
| `llm-agent-flow/` | co-located `_test.go` throughout; `internal/apisnapshot/` (v0.1 freeze gate) | `examples/{echo_chain,http_tool,router}/` (each with `example_test.go`) | `README.md`, `CHANGELOG.md`, `docs/compatibility.md` |

## Planning source-of-truth

Milestone planning, requirements, decisions, and phase plans live in:

```text
llm-agent/.planning/
├── PROJECT.md                        # what the project is, core value, hard rules
├── STATE.md                          # current milestone (v1.2 active) + active phase
├── ROADMAP.md                        # phase plan for active milestone
├── REQUIREMENTS.md                   # active milestone requirements + traceability
├── config.json
├── milestones/                       # archived milestone roadmaps
├── phases/                           # per-phase plans (Phase 33 — coordinated-bump-and-retag-wave, Phase 35 — budget, etc.)
├── research/                         # cross-cut audits, keystone decisions KE-1…KE-7
├── todos/
├── v0.3-MILESTONE-AUDIT.md … v1.1-MILESTONE-AUDIT.md
├── v1.2-REQUIREMENTS.md
└── v1.2-ROADMAP.md
```

The umbrella's `.planning/` (this repo) holds only ecosystem-wide artifacts (this `codebase/` map; umbrella PROJECT/README). Each sister repo keeps its own focused `README.md` and (for `llm-agent-flow`) a `CHANGELOG.md` mirroring the per-tag delta; this top-level README is a navigation index, not a planning home.

## Build entry points (`Makefile`)

| Target | Action |
|---|---|
| `make bootstrap` | `./scripts/eco.sh bootstrap $TARGETS` — clones missing subprojects (now 6 known repos) |
| `make workspace` | `./scripts/workspace.sh` — writes the shared `go.work` for local cross-repo dev (over 6 modules) |
| `make pull` | `./scripts/eco.sh pull $TARGETS` — `git pull --ff-only` in each cloned subproject |
| `make status` | `./scripts/eco.sh status $TARGETS` — `git status` summary per subproject |
| `make build` | `./scripts/eco.sh build $TARGETS` — `GOWORK=off go build ./...` per subproject |
| `make test` | `./scripts/eco.sh test $TARGETS` — `GOWORK=off go test ./...` per subproject |
| `make up` | `./scripts/eco.sh up $TARGETS` — `docker compose up -d --build` for launchable repos |
| `make down` | `./scripts/eco.sh down $TARGETS` — `docker compose down` for launchable repos |

`TARGETS=all` by default; pass `TARGETS=llm-agent-customer-support` (or comma-list) to scope to a subset.

**Launchable subprojects** (per `scripts/eco.sh` `is_launchable`):
- `llm-agent-otel` → `compose/compose.yaml` (Grafana port 3001, OTLP gRPC 4317 / HTTP 4318 by default)
- `llm-agent-customer-support` → `compose/compose.yaml` (app 8080, Grafana 3000, Ollama 11434, OTLP 4317/4318)

Library-only repos (`llm-agent`, `llm-agent-rag`, `llm-agent-providers`, **`llm-agent-flow`**) still participate in `build` and `test` but have no `up`/`down`. `llm-agent-flow` ships an HTTP service (`cmd/flowd`) but is not yet wired as a launchable compose target — it's invoked in-process via `internal/flowrunner/` from the customer-support service.

## `scripts/` contents

```text
scripts/
├── eco.sh         # multi-repo dispatcher; hard-codes the 6-repo list + URLs + launchable subset
└── workspace.sh   # writes `./go.work` with `use` over 6 modules
                   # ./llm-agent ./llm-agent-rag ./llm-agent-otel ./llm-agent-providers
                   # ./llm-agent-customer-support ./llm-agent-flow
```

`eco.sh` exports per-service port env vars (`CS_APP_PORT`, `CS_GRAFANA_PORT`, `CS_OLLAMA_PORT`, `CS_OTEL_GRPC_PORT`, `CS_OTEL_HTTP_PORT`, `OTEL_DEMO_GRAFANA_PORT`, …) before invoking `docker compose`. The `all_repos` array now contains 6 entries; `workspace.sh` requires all 6 to be present and exits non-zero otherwise.

## Naming Conventions

**Files:**
- One `_test.go` co-located with each source file (`agent.go` ↔ `agent_test.go`).
- Each provider package has the same skeleton: `<name>.go`, `options.go`, `map.go`, `errors.go`, `doc.go`, `<name>_test.go`, `README.md`.
- Service internal packages use `<name>.go` + `<name>_test.go` per package (one source file per package by convention; `flowrunner/` adds a `doc.go`).
- OTel decorator packages use `<package>.go` (e.g., `otelmodel.go`, `otelflow.go`) + `config.go` + `<package>_test.go`; `doc.go` for the package-level overview.
- Flow library uses one file per concept: `ir.go`, `engine.go`, `event.go`, `validate.go`, `runner.go`, `condition.go`, `node.go`, etc.

**Directories:**
- Repo-level dirs are lowercase, hyphen-separated where they map to Go module names (`llm-agent-customer-support`, `llm-agent-flow`).
- Go package dirs are lowercase, no separators (`sessionstore`, `knowledgebase`, `supportflow`, `otelmodel`, `otelflow`, `flowrunner`).
- `internal/` for private packages; `cmd/<binary>/` for executables (`cmd/flow`, `cmd/flowd`, `cmd/server`, `cmd/tailprobe`).
- `compose/` for docker-compose assets; `dashboards/` for Grafana dashboard JSON.
- `.planning/` for GSD planning artifacts.
- `api/v<N>.snapshot.txt` for the committed exported-API baseline (rag v1, flow v0.1).

## Where to Add New Code

**New agent paradigm:**
- Primary code: `llm-agent/<name>.go` (top-level — paradigms are first-class siblings of `react.go`, `plan_solve.go`, etc.).
- Tests: `llm-agent/<name>_test.go` co-located.
- Example: `llm-agent/example_<name>_test.go` (runnable doc).

**New provider adapter:**
- Primary code: `llm-agent-providers/<provider>/<provider>.go` + `options.go` + `map.go` + `errors.go` + `doc.go`.
- Tests: `llm-agent-providers/<provider>/<provider>_test.go`.
- Contract coverage: wire into `llm-agent-providers/internal/contract/`.
- Author guide: follow `llm-agent/PROVIDER_AUTHORING.md`.

**New OTel decorator for a new core abstraction:**
- Primary code: `llm-agent-otel/otel<thing>/otel<thing>.go` + `config.go`.
- Pattern for capability-preserving decorators (model side): mirror `otelmodel/` — preserve capability interfaces via 2ⁿ wrapper types.
- Pattern for uniform-interface decorators (flow side): mirror `otelflow/` — a single wrapper struct around the inner interface, two-layer span tree (root + per-event children).
- Tests: `llm-agent-otel/otel<thing>/otel<thing>_test.go`.

**New RAG capability:**
- Primary code: add to the relevant `llm-agent-rag/<package>/` (e.g., a new retrieval policy → `retrieve/`; a new store backend → its own subpackage like `postgres/`).
- API stability: v1.x is additive-only — new exports OK, breaking renames forbidden. Snapshot test in `internal/apisnapshot/` will fail until `api/v1.snapshot.txt` is regenerated.
- Tests: co-located `_test.go`; backends must satisfy `store/storetest.RunConformance`.

**New flow node type:**
- Primary code: pick a sub-package under `llm-agent-flow/flow/` and add a `NodeKind` implementation; register a `NodeFactory` via `(*flow.NodeRegistry).Register(typ, factory)`.
- The bundled `tool` type is the reference (`flow/tool_node.go` + `flow/node.go`). Downstream apps can add custom kinds without forking — see `flowrunner.Runner.Register` for the wiring example.
- Tests: co-located `_test.go`.
- API stability: any new exported symbol on a stable package is additive — regenerate `api/v0.1.snapshot.txt` via `go test ./internal/apisnapshot/ -run TestAPISnapshot -update`.

**New flow store backend:**
- Primary code: new sub-package under `llm-agent-flow/flow/store/<backend>/` implementing the `flow/store.Store` interface; optionally satisfy `(*sqlite.Store).AppendRunEvents` shape for batch insertion (declared in `flow/store/store.go` as `RunEventBatchItem` + the optional interface).
- Wire it into flowd via `server.Config.Store`.
- Tests: a reusable conformance suite under `llm-agent-flow/flow/store/storetest/` is the natural home (mirroring `llm-agent-rag/store/storetest`); current code keeps tests co-located in `flow/store/sqlite/`.

**New flowd HTTP endpoint:**
- Primary code: extend `llm-agent-flow/cmd/flowd/server/server.go` — register the handler in `(*Server).Handler()`'s `mux.HandleFunc("METHOD /path", …)` block.
- Note: the v0.1.x wire shape is **stable** — new endpoints are additive (OK), renames or response-shape changes need `/v2`.
- Tests: add a per-endpoint `server_<thing>_test.go` (existing: `server_test.go`, `server_crud_test.go`, `server_events_test.go`, `server_replay_test.go`, `auth_test.go`, `lru_test.go`).

**New HTTP endpoint in customer-support:**
- Primary code: extend `llm-agent-customer-support/internal/httpapi/httpapi.go`.
- Wiring: register the handler in `internal/app/app.go` mux build.
- Tests: `internal/httpapi/httpapi_test.go`.

**New customer-support cross-cutting concern (rate limit, guardrail, session backend, flow runner, etc.):**
- Add a new `internal/<concern>/` package with `<concern>.go` + `<concern>_test.go` (+ `doc.go` for the package overview, per the `flowrunner/` precedent).
- Wire it into `App.New` in `internal/app/app.go` (the composition root).

**New top-level umbrella subproject:**
- Add the dir at repo root, update `go.work` `use(...)` block, add the repo URL + (optional) launchable flag in `scripts/eco.sh` (`all_repos` + `repo_url` + `is_launchable`), update `scripts/workspace.sh`'s expected module count, update the README's subproject table.

**Shared helper across subprojects:**
- Subprojects do NOT share code directly. If `llm-agent-otel` needs a helper that `llm-agent-customer-support` also wants, the helper goes into `llm-agent-otel` (or, rarely, into `llm-agent` core) and the consumer imports it. `flow.Runner` is the canonical pattern — defined in `llm-agent-flow/flow/runner.go`, consumed by `llm-agent-otel/otelflow/`, consumed by `llm-agent-customer-support/internal/flowrunner/`.

## Special Directories

**`.planning/` (per repo):**
- Purpose: GSD milestone/phase planning.
- Generated: No.
- Committed: Yes (umbrella's `.planning/` + `llm-agent/.planning/` are tracked).

**`go.work` (umbrella root):**
- Purpose: local cross-repo workspace.
- Generated: Yes (by `scripts/workspace.sh` or by hand).
- Committed: **In this umbrella repo, YES** — see `go.work` already at the root listing all 6 modules. Every *sister* repo `.gitignore`s `go.work` because they tag from CI with `GOWORK=off`.

**`compose/` (per launchable repo):**
- Purpose: docker-compose demo stack.
- Generated: No.
- Committed: Yes.

**`examples/` (in `llm-agent/`):**
- Purpose: runnable example programs.
- Generated: No.
- Committed: Yes — as a separate Go module so example deps don't enter the core `go.mod`.

**`examples/` (in `llm-agent-flow/`):**
- Purpose: runnable flow JSON files + bundled demo tools (`Tools() []agents.Tool`) used by `cmd/flow` and `cmd/flowd` as the default fallback catalog.
- Generated: No.
- Committed: Yes.

**`internal/` (per repo):**
- Purpose: package-private to that module — Go enforces import restriction.
- Generated: No.
- Committed: Yes.

**`internal/apisnapshot/` (per frozen-surface repo: `llm-agent-rag`, `llm-agent-flow`):**
- Purpose: pure-stdlib exported-API snapshot gate (`go/parser` + `go/printer`); runs on every `go test`; fails any drift against the committed `api/v<N>.snapshot.txt` baseline.
- Generated: No (the gate itself); the baseline IS generated via `-update`.
- Committed: Yes (both the gate and the baseline).

**`api/v<N>.snapshot.txt` (per frozen-surface repo):**
- Purpose: committed exported-API baseline for SemVer additive-only enforcement.
- Generated: Yes — `go test ./internal/apisnapshot/ -run TestAPISnapshot -update`.
- Committed: Yes — every additive change updates this file in the same commit.

**`third_party/` (in `llm-agent/`):**
- Purpose: vendored stdlib-compatible code where needed.
- Generated: No.
- Committed: Yes.

## File-count summary per subproject

| Subproject | Tag | Go files | Total LOC (Go) |
|---|---|---:|---:|
| `llm-agent/` | v0.5.1 | 135 | 19,358 |
| `llm-agent-rag/` | v1.0.2 | 125 | 20,904 |
| `llm-agent-flow/` | v0.1.1 | 57 | 7,491 |
| `llm-agent-providers/` | v0.2.2 | 36 | 6,220 |
| `llm-agent-customer-support/` | v0.2.3 | 27 | 4,104 |
| `llm-agent-otel/` | v0.2.2 | 25 | 2,936 |
| **Ecosystem total** | | **405** | **61,013** |

Counted with `find … -name "*.go" -not -path '*/.*' | xargs wc -l`. Excludes `.git/` and other hidden dirs. Note that `llm-agent-rag` remains larger than core `llm-agent` (GraphRAG implementation, Louvain communities, DRIFT, path-ranked subgraphs, pgvector backend, eval framework, shared store conformance suite). `llm-agent-flow` slots in as the third-largest subproject after rag and core — its 57 files cover the IR, engine, validators, condition evaluator, three example flows + tests, the SQLite store + every test, the tool-manifest format + tests, the CLI, and the flowd HTTP server + tests for every endpoint.

---

*Structure analysis: 2026-05-21*
