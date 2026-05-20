# Codebase Structure

**Analysis Date:** 2026-05-20

## Directory Layout

```text
llm-agent-ecosystem/                  # umbrella shell — coordination only
├── go.work                           # workspace pinning 5 sibling modules
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
│   ├── eco.sh                        # multi-repo dispatcher
│   └── workspace.sh                  # writes the local go.work
├── llm-agent/                        # core framework — stdlib-only
├── llm-agent-rag/                    # standalone RAG SDK (frozen v1.x API)
├── llm-agent-otel/                   # OTel decorator wrappers
├── llm-agent-providers/              # provider adapters
└── llm-agent-customer-support/       # demo service
```

## Directory Purposes

**`/`(umbrella root):**
- Purpose: coordination shell; no product source code.
- Contains: `go.work` (workspace pinning), `Makefile` (bootstrap / build / test / up / down), `README.md` (navigation index), `PROJECT.md` (one-line descriptor), `.planning/` (this map), `docs/`, `scripts/`.
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
- Contains: `eco.sh` (`bootstrap | pull | status | build | test | up | down`), `workspace.sh` (writes `go.work`).
- Key files: `eco.sh` (driver), `workspace.sh`.

**`llm-agent/`** — see subproject detail below.
**`llm-agent-rag/`** — see subproject detail below.
**`llm-agent-otel/`** — see subproject detail below.
**`llm-agent-providers/`** — see subproject detail below.
**`llm-agent-customer-support/`** — see subproject detail below.

## Subproject layouts

### `llm-agent/` — core framework (135 Go files, ~19,358 lines)

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
│   ├── memory.go                     # Memory interface + MemoryItem
│   ├── manager.go                    # Manager (Consolidate + Forget)
│   ├── working.go
│   ├── episodic.go
│   ├── semantic.go
│   ├── internal_score.go
│   ├── tool.go                       # MemoryTool exposed to agents
│   ├── doc.go
│   └── *_test.go
├── orchestrate/                      # multi-agent coordination primitives
│   ├── graph.go                      # StateGraph
│   ├── fanout.go
│   ├── pipeline.go
│   ├── roleplay.go
│   ├── roundrobin.go
│   ├── termination.go
│   ├── doc.go
│   └── *_test.go
├── pkg/
│   └── fanout/                       # public goroutine fan-out helper
├── rag/                              # RAG facade (empty dir; types re-exported elsewhere)
├── builtin/                          # built-in tools
├── comm/                             # inter-agent communication
├── context/                          # ctx-keyed helpers
├── rl/                               # reinforcement-learning utilities
├── internal/
│   └── testenv/
├── third_party/
├── docs/                             # core repo docs
├── examples/
│   ├── go.mod                        # examples are their own module
│   ├── README.md
│   ├── 01-simple-agent/
│   ├── 02-tool-use/
│   ├── 03-pipeline/
│   ├── 04-state-graph/
│   ├── 05-fanout/
│   ├── 06-budget/
│   └── scriptedllm/
├── bench/
├── scripts/
├── example_simple_test.go            # in-package example tests
├── example_tool_use_test.go
├── example_multi_agent_test.go
├── scriptedllm_test.go
├── .github/                          # core CI
├── .agents/
└── .codex/
```

**Tests:** co-located `_test.go` files throughout (`agent_test.go`, `registry_test.go`, etc.). In-package example tests (`example_*_test.go`) double as runnable docs. `bench/` for benchmarks. `examples/` is a separate Go module to keep example deps out of the core module.

**Docs:** `README.md`, `CHANGELOG.md`, `PROVIDER_AUTHORING.md`, `DEPRECATIONS.md`, `CLAUDE.md`, `docs/`.

### `llm-agent-rag/` — standalone RAG SDK (125 Go files, ~20,904 lines)

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
├── store/                            # vector store seam + InMemoryStore
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
└── internal/                         # apisnapshot, etc.
```

**Tests:** co-located `_test.go` (e.g., `rag/system_test.go`, `rag/community_test.go`, `rag/drift_test.go`).

**Docs:** `README.md`, `CHANGELOG.md`, `docs/production-deployment.md`, `docs/backend-selection.md`, `docs/core-compatibility.md`, `docs/compatibility.md` (the v1.x additive-only promise).

### `llm-agent-otel/` — OTel decorator wrappers (21 Go files, ~2,373 lines)

```text
llm-agent-otel/
├── go.mod                            # require: llm-agent + llm-agent-rag + OTel SDKs
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

**Tests:** co-located `_test.go` per package (e.g., `otelmodel/otelmodel_test.go`). No examples directory; the README has the canonical Quick-start snippet.

**Docs:** `README.md` (Quick-start, exporter defaults, opt-in semantics, demo compose flow).

### `llm-agent-providers/` — provider adapters (36 Go files, ~6,220 lines)

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
│   ├── anthropic.go
│   ├── options.go
│   ├── map.go
│   ├── errors.go
│   ├── doc.go
│   ├── anthropic_test.go
│   └── README.md
├── ollama/                           # ChatModel + ToolCaller (model-aware) + Embedder
│   ├── ollama.go
│   ├── options.go
│   ├── map.go
│   ├── errors.go
│   ├── tool_strategy.go              # per-model tool strategy
│   ├── embed_strategy.go             # per-model embed strategy
│   ├── doc.go
│   ├── ollama_test.go
│   └── README.md
├── deepseek/                         # ChatModel + ToolCaller, regional endpoint presets
│   ├── deepseek.go
│   ├── options.go
│   ├── map.go
│   ├── errors.go
│   ├── doc.go
│   ├── deepseek_test.go
│   └── README.md
├── minimax/                          # ChatModel + ToolCaller, regional endpoint presets
│   ├── minimax.go
│   ├── options.go
│   ├── map.go
│   ├── errors.go
│   ├── doc.go
│   ├── minimax_test.go
│   └── README.md
├── internal/
│   └── contract/                     # shared fixture-driven cross-provider conformance suite
├── docs/
│   └── superpowers/
└── scripts/
```

**Tests:** co-located per provider + shared `internal/contract/` conformance suite + nightly live Ollama CI path.

**Docs:** top-level `README.md` + per-provider `README.md`.

### `llm-agent-customer-support/` — demo service (24 Go files, ~3,875 lines)

```text
llm-agent-customer-support/
├── go.mod                            # require: llm-agent + providers + otel + uuid + lib/pq + sqlite + OTel SDKs
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

## Where tests / examples / docs live

| Subproject | Tests | Examples | Docs |
|---|---|---|---|
| `llm-agent/` | co-located `_test.go`; `bench/`; `example_*_test.go` doubling as runnable docs | `examples/` (separate Go module) | `README.md`, `CHANGELOG.md`, `PROVIDER_AUTHORING.md`, `DEPRECATIONS.md`, `CLAUDE.md`, `docs/` |
| `llm-agent-rag/` | co-located `_test.go`; `eval/` (CI eval gate); `store/storetest` (shared conformance suite) | `examples/` | `README.md`, `docs/{production-deployment,backend-selection,core-compatibility,compatibility}.md` |
| `llm-agent-otel/` | co-located `_test.go` per package | `cmd/tailprobe/`, `compose/demo/` | `README.md` |
| `llm-agent-providers/` | co-located per provider + `internal/contract/` shared suite + nightly live Ollama path | none top-level (each provider README has snippets) | `README.md` + per-provider `README.md`, `docs/superpowers/` |
| `llm-agent-customer-support/` | co-located per package + `compose/assets_test.go` | none — the service itself is the example | `README.md` |

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

The umbrella's `.planning/` (this repo) holds only ecosystem-wide artifacts (this `codebase/` map; umbrella PROJECT/README). Each sister repo keeps its own focused `README.md`; this top-level READMEs is a navigation index, not a planning home.

## Build entry points (`Makefile`)

| Target | Action |
|---|---|
| `make bootstrap` | `./scripts/eco.sh bootstrap $TARGETS` — clones missing subprojects (5 known repos) |
| `make workspace` | `./scripts/workspace.sh` — writes the shared `go.work` for local cross-repo dev |
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

Library-only repos (`llm-agent`, `llm-agent-rag`, `llm-agent-providers`) still participate in `build` and `test` but have no `up`/`down`.

## `scripts/` contents

```text
scripts/
├── eco.sh         # multi-repo dispatcher; hard-codes the 5-repo list + URLs + launchable subset
└── workspace.sh   # writes `./go.work` with `use ./llm-agent ./llm-agent-rag ./llm-agent-otel ./llm-agent-providers ./llm-agent-customer-support`
```

`eco.sh` exports per-service port env vars (`CS_APP_PORT`, `CS_GRAFANA_PORT`, `CS_OLLAMA_PORT`, `CS_OTEL_GRPC_PORT`, `CS_OTEL_HTTP_PORT`, `OTEL_DEMO_GRAFANA_PORT`, …) before invoking `docker compose`.

## Naming Conventions

**Files:**
- One `_test.go` co-located with each source file (`agent.go` ↔ `agent_test.go`).
- Each provider package has the same skeleton: `<name>.go`, `options.go`, `map.go`, `errors.go`, `doc.go`, `<name>_test.go`, `README.md`.
- Service internal packages use `<name>.go` + `<name>_test.go` per package (one source file per package by convention).
- OTel decorator packages use `<package>.go` (e.g., `otelmodel.go`) + `config.go` + `<package>_test.go`.

**Directories:**
- Repo-level dirs are lowercase, hyphen-separated where they map to Go module names (`llm-agent-customer-support`).
- Go package dirs are lowercase, no separators (`sessionstore`, `knowledgebase`, `supportflow`, `otelmodel`).
- `internal/` for private packages; `cmd/<binary>/` for executables.
- `compose/` for docker-compose assets; `dashboards/` for Grafana dashboard JSON.
- `.planning/` for GSD planning artifacts.

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
- Pattern: mirror `otelmodel/` — preserve capability interfaces via 2ⁿ wrapper types.
- Tests: `llm-agent-otel/otel<thing>/otel<thing>_test.go`.

**New RAG capability:**
- Primary code: add to the relevant `llm-agent-rag/<package>/` (e.g., a new retrieval policy → `retrieve/`; a new store backend → its own subpackage like `postgres/`).
- API stability: v1.x is additive-only — new exports OK, breaking renames forbidden. Snapshot test in `internal/apisnapshot/` will fail until `api/v1.snapshot.txt` is regenerated.
- Tests: co-located `_test.go`; backends must satisfy `store/storetest.RunConformance`.

**New HTTP endpoint in customer-support:**
- Primary code: extend `llm-agent-customer-support/internal/httpapi/httpapi.go`.
- Wiring: register the handler in `internal/app/app.go` mux build.
- Tests: `internal/httpapi/httpapi_test.go`.

**New customer-support cross-cutting concern (rate limit, guardrail, session backend, etc.):**
- Add a new `internal/<concern>/` package with `<concern>.go` + `<concern>_test.go`.
- Wire it into `App.New` in `internal/app/app.go` (the composition root).

**New top-level umbrella subproject:**
- Add the dir at repo root, update `go.work` `use(...)` block, add the repo URL + (optional) launchable flag in `scripts/eco.sh`, update the README's subproject table.

**Shared helper across subprojects:**
- Subprojects do NOT share code directly. If `llm-agent-otel` needs a helper that `llm-agent-customer-support` also wants, the helper goes into `llm-agent-otel` (or, rarely, into `llm-agent` core) and the consumer imports it.

## Special Directories

**`.planning/` (per repo):**
- Purpose: GSD milestone/phase planning.
- Generated: No.
- Committed: Yes (umbrella's `.planning/` + `llm-agent/.planning/` are tracked).

**`go.work` (umbrella root):**
- Purpose: local cross-repo workspace.
- Generated: Yes (by `scripts/workspace.sh` or by hand).
- Committed: **In this umbrella repo, YES** — see `go.work` already at the root. Every *sister* repo `.gitignore`s `go.work` because they tag from CI with `GOWORK=off`.

**`compose/` (per launchable repo):**
- Purpose: docker-compose demo stack.
- Generated: No.
- Committed: Yes.

**`examples/` (in `llm-agent/`):**
- Purpose: runnable example programs.
- Generated: No.
- Committed: Yes — as a separate Go module so example deps don't enter the core `go.mod`.

**`internal/` (per repo):**
- Purpose: package-private to that module — Go enforces import restriction.
- Generated: No.
- Committed: Yes.

**`third_party/` (in `llm-agent/`):**
- Purpose: vendored stdlib-compatible code where needed.
- Generated: No.
- Committed: Yes.

## File-count summary per subproject

| Subproject | Go files | Total LOC (Go) |
|---|---:|---:|
| `llm-agent/` | 135 | 19,358 |
| `llm-agent-rag/` | 125 | 20,904 |
| `llm-agent-providers/` | 36 | 6,220 |
| `llm-agent-customer-support/` | 24 | 3,875 |
| `llm-agent-otel/` | 21 | 2,373 |
| **Ecosystem total** | **341** | **52,730** |

Counted with `find … -name "*.go" | xargs wc -l`. Excludes `.git/`. Note that `llm-agent-rag` is larger than core `llm-agent` because it carries the GraphRAG implementation (Louvain communities, DRIFT, path-ranked subgraphs), the pgvector backend, the eval framework, and the shared store conformance suite.

---

*Structure analysis: 2026-05-20*
