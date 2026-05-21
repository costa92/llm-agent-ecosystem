<!-- written: 2026-05-21 -->
# llm-agent-flow — research summary

**Reference:** https://github.com/langflow-ai/langflow (`main` @ pushedAt
2026-05-20T23:59:37Z, langflow `pyproject.toml` `version = "1.9.3"`, 148k stars, MIT)

**Question this doc answers:** should the ecosystem gain a sister repo that
delivers visual/JSON workflow orchestration in the Langflow mould, and if so,
where does it sit relative to `llm-agent`'s already-in-flight
`orchestrate.Supervisor` / `StateGraph[S]` work?

---

## Section 1 — Langflow at a glance

### Stack (what it actually is)

- **Backend: Python 3.10–3.13, FastAPI.** `pyproject.toml` declares
  `requires-python = ">=3.10,<3.15"`, version `1.9.3`. Entrypoint
  `src/backend/base/langflow/main.py` imports `from fastapi import FastAPI,
  HTTPException, ...` and mounts routers via `langflow.api.router`. The
  server is started by `langflow_launcher.py` / `__main__.py` and uvicorn.
- **Frontend: React 19 + Vite + TypeScript.** `src/frontend/package.json`
  declares `"react": "^19.2.1"`, `"@xyflow/react": "^12.3.6"` and
  `"reactflow": "^11.11.3"` (React Flow / xyflow is the canvas), plus
  `"zustand": "^4.5.2"` (state) and `"@tanstack/react-query": "^5.49.2"`.
  Build/dev via Vite.
- **Database: SQLModel (= SQLAlchemy + Pydantic) with Alembic migrations.**
  `src/backend/base/langflow/services/database/models/flow/model.py`
  defines `class FlowBase(SQLModel)` with `data: dict | None =
  Field(default=None, ..., sa_column=Column(JSON, ...))`. Migrations live
  in `src/backend/base/langflow/alembic/`.
- **Three Python packages in one repo:**
  `pyproject.toml`'s `[tool.uv.workspace]` lists members
  `["src/backend/base", ".", "src/lfx", "src/sdk"]`. So the codebase is
  `langflow` (heavy app, DB + auth + workspace), `langflow-base` (older
  base), `lfx` (Langflow Executor — stateless lightweight runner), and
  `langflow-sdk`.
- **Distribution:**
  - PyPI: `uv pip install langflow -U` then `uv run langflow run` (README
    Quickstart); also `lfx` for the stateless mode (`src/lfx/README.md`).
  - Docker: `docker run -p 7860:7860 langflowai/langflow:latest` (README).
  - Langflow Desktop (Windows/macOS) bundles Python deps (README highlights).
  - Hosted "Langflow Cloud" exists per README marketing but is closed
    source; the OSS repo is MIT-licensed.

### Core abstractions (the real names)

- **`Component`** — the smallest unit, `src/lfx/src/lfx/custom/custom_component/component.py`
  `class Component`. Each node on the canvas is a Component instance with
  typed `Input`s and `Output`s (defined via
  `lfx.template.field.base.Input/Output`). Sub-base `CustomComponent`
  enables runtime-loaded user code.
- **`Vertex`** — runtime wrapper for a Component inside a Graph.
  `src/lfx/src/lfx/graph/vertex/base.py` `class Vertex` (~38k bytes); has
  `VertexStates {ACTIVE, INACTIVE, ERROR}` and reads `is_input`,
  `is_output`, `has_session_id`, `params`. Specialized subclasses live in
  `vertex/vertex_types.py`: `ComponentVertex`, `InterfaceVertex`,
  `StateVertex`.
- **`Edge` / `CycleEdge`** — `src/lfx/src/lfx/graph/edge/base.py` `class Edge`
  pairs `source_id` + `target_id` plus `source_handle` (`SourceHandle`) and
  `target_handle` (`TargetHandle`). Type compatibility checked via
  `types_compatible(output_types, input_types)` with a `TYPE_MIGRATIONS`
  dict (`Data`→`JSON`, `DataFrame`→`Table`). `CycleEdge` marks back-edges
  detected during graph build.
- **`Graph`** — `src/lfx/src/lfx/graph/graph/base.py` `class Graph`
  (~104k bytes; the centerpiece). Holds `vertices: list[Vertex]`,
  `edges: list[CycleEdge]`, `predecessor_map`/`successor_map`,
  `vertices_layers: list[list[str]]` (topological layers), `run_manager
  = RunnableVerticesManager()`. Tracks `inactivated_vertices`,
  `conditionally_excluded_vertices`, `vertices_to_run`.
- **`GraphData` (the serialized shape)** —
  `src/lfx/src/lfx/graph/graph/schema.py`:
  ```python
  class GraphData(TypedDict):
      nodes: list[NodeData]
      edges: list[EdgeData]
      viewport: NotRequired[ViewPort]   # x, y, zoom — UI hint
  ```
  This is literally React Flow's node/edge JSON: a starter project
  (e.g. `initial_setup/starter_projects/Basic Prompting.json`) contains
  top-level keys `['data', 'description', 'endpoint_name', 'id',
  'is_component', 'last_tested_version', 'name', 'tags']` and `data` has
  `['edges', 'nodes', 'viewport']`. Each node has
  `['data', 'dragging', 'height', 'id', 'measured', 'position',
  'positionAbsolute', 'selected', 'type']` (most fields are React Flow
  UI state). Each edge carries handle metadata including
  `sourceHandle.output_types: ["Message"]` and
  `targetHandle.inputTypes: ["Message"]` — type-checked at load.
- **`Flow` (the DB row)** — `src/backend/base/langflow/services/database/models/flow/model.py`
  `class FlowBase(SQLModel)`: fields include `name`, `description`,
  `data: dict | None` (the entire GraphData), `is_component: bool`,
  `endpoint_name`, `webhook: bool`, `mcp_enabled: bool`, `access_type:
  AccessTypeEnum {PRIVATE, PUBLIC}`, `locked`, `tags`.

### Execution model

- **Layered topological execution, not pure DAG.** Cycles are first-class:
  `find_all_cycle_edges`, `find_cycle_vertices`, `should_continue` live
  in `src/lfx/src/lfx/graph/graph/utils.py`. The graph has back-edges
  represented as `CycleEdge.is_cycle = True` and a `MAX_CYCLE_APPEARANCES
  = 2` budget per vertex (`utils.py`).
- **Entry-point discovery is heuristic.** `find_start_component_id`
  scans for vertex IDs containing `webhook` or `chat` (`PRIORITY_LIST_OF_INPUTS
  = ["webhook", "chat"]`) — not a declarative `start_id` field.
- **Conditional routing as a separate dimension from cycles.**
  `Graph.conditionally_excluded_vertices` and
  `conditional_exclusion_sources: dict[str, set[str]]` (see
  `Graph.__init__`); this is distinct from `inactive_vertices` /
  `inactivated_vertices` used by the cycle manager.
- **Async-first with `arun`/`_run` and sync fallback.** `graph.base.py`:
  - `def start(...)` line 432 — sync entry that spins an event loop.
  - `async def _run(...)` line 768.
  - `async def arun(...)` line 852 — main async entry.
  - `async def build_vertex(...)` line 1540.
  - `async def process(...)` line 1678.
  The HTTP layer always calls `arun` via
  `langflow/processing/process.py` `run_graph_internal` (line 25) →
  `await graph.arun(inputs=..., stream=stream, session_id=..., event_manager=event_manager)`.
- **Streaming events use the AG-UI protocol.** `graph/base.py` imports
  `from ag_ui.core import RunFinishedEvent, RunStartedEvent` and
  `vertex/base.py` imports `StepFinishedEvent, StepStartedEvent` from
  the same package. Events are pushed through an `EventManager`
  (`events/event_manager.py`); the HTTP layer wraps them as SSE.
- **HTTP surface for runs:**
  - V1: `POST /api/v1/run/{flow_id_or_name}`
    (`api/v1/endpoints.py` line 582), `POST /api/v1/webhook/{flow_id_or_name}`
    (line 767), `POST /api/v1/custom_component` (line 1060) for live-build.
  - V2: `POST /api/v2/workflow` with sync / stream / background modes
    (`api/v2/workflow.py` lines 1-40, `EXECUTION_TIMEOUT = 300s`).
  - Flows CRUD: `api/v1/flows.py` `POST/GET/GET{id}/PATCH/PUT/DELETE/`,
    `POST /upload/`, `POST /download/`, `GET /basic_examples/`.

### State / where data lives

- **SQLModel-backed Postgres / SQLite.** `langflow.services.database.*`
  (factory at `services/database/factory.py`). Migrations in
  `langflow/alembic/`. Tables include `flow`, `folder`, `user`,
  `api_key`, `variable`, `deployment`, etc. The entire flow graph lives
  as a JSON column on the `flow` row.
- **`lfx` (stateless variant) replaces the DB with a `NoopSession`**
  (`src/lfx/README.md`: "any stateful operations ... will not persist").
- **Cache + chat services** live under `lfx/services/`; pluggable
  (`src/lfx/PLUGGABLE_SERVICES.md`).

### Load-bearing design choices

- **Components are arbitrary Python.** `lfx/custom/custom_component/component.py`
  uses `ast`-parsed user code (`import ast`); validation in
  `lfx/custom/validate.py`. Hot-reloadable via `code` field on the
  component template; `api/v1/endpoints.py` `POST /custom_component`
  re-instantiates. This is langflow's whole selling point — and its
  whole attack surface.
- **Heavy LangChain coupling.** `lfx/custom/custom_component/component.py`
  imports `from langchain_core.tools import StructuredTool`;
  `lfx/components/` has 100+ component packages, the vast majority
  thin wrappers over LangChain integrations.
- **Pydantic v2 everywhere** — Flow models, request/response, settings.
  `main.py` even has `warnings.filterwarnings("ignore",
  category=PydanticDeprecatedSince20)` for LangChain's legacy code.
- **MCP-server-as-feature.** Every flow can be exposed as MCP tools
  (`Flow.mcp_enabled`, `api/v1/mcp.py`, `api/v2/mcp.py`).
- **Deployment-mappers as a registry pattern.**
  `api/v1/mappers/deployments/` (per-target adapter; current target is
  `watsonx_orchestrate`) — Langflow's bet that exported flows become
  the universal IR for downstream serving platforms.
- **The `lfx` split is intentional reuse.** Same `Graph` engine,
  different service shell; the umbrella ecosystem's analogue is
  splitting library / service.

---

## Section 2 — Mapping Langflow to the existing Go ecosystem

| Langflow concept | Already covered by current Go ecosystem? | Where |
|---|---|---|
| `Component` (typed inputs/outputs) | **Partial.** `Tool` (`llm-agent/tool.go`) has `Name/Description/Schema/Execute`; agents have `Run/RunStream`. No I/O-port typing beyond `json.RawMessage` schema. | `llm-agent/tool.go`, `llm-agent/agent.go` |
| `Component` library / catalog | **Partial.** `Registry` (`llm-agent/registry.go`) is per-agent. No JSON-discoverable global catalog. | `llm-agent/registry.go` |
| `Vertex` runtime state machine | **None.** No per-node ACTIVE/INACTIVE/ERROR FSM. | — |
| `Graph` topological execution | **Partial.** `orchestrate.StateGraph[S]` is **edge-driven** state-machine (LangGraph-style), not **DAG-driven** like Langflow. No vertex layering, no parallel vertex execution. | `llm-agent/orchestrate/graph.go` |
| Cycle handling | **None as Langflow defines it.** `StateGraph` allows loops via conditional edges + `MaxSteps`; no `CycleEdge` first-class concept. | `llm-agent/orchestrate/graph.go:152` (`WithMaxSteps`) |
| Conditional routing | **Yes.** `AddConditionalEdge(from, ConditionFunc[S])` in `StateGraph`. | `llm-agent/orchestrate/graph.go:71` |
| Persistable flow JSON | **None.** `StateGraph` is built in Go code, never serialized. | — |
| `Edge` type compatibility | **None.** Connections between agents/tools are by Go types only. | — |
| Multi-agent supervisor | **In-flight (v1.2 Phase 37).** `orchestrate.Supervisor` ships as a thin `StateGraph[S]` facade per `v1.2-ROADMAP.md` lines 152-184; KC-1 (research SUMMARY line 38). | `llm-agent/orchestrate/...` (Phase 37 deliverable) |
| Parallel fanout to tools | **Yes.** `async.AsyncRunner` + `pkg/fanout`. | `llm-agent/async.go`, `llm-agent/pkg/fanout/` |
| Sequential pipeline | **Yes.** `Chain` (`llm-agent/chain.go`) + `orchestrate/pipeline.go`. | `llm-agent/chain.go` |
| Streaming events | **Yes** (provider-side). `StreamEvent` typed union with stable `Index`. **But not at flow level** — no FlowEvent / VertexEvent. | `llm-agent/llm/stream.go` |
| HTTP API to run something | **Customer-support only,** scoped to chat. Not a flow-CRUD/runner. | `llm-agent-customer-support/internal/httpapi/` |
| Flow / run history DB | **None.** `sessionstore` is per-chat-session, not per-flow-run. | `llm-agent-customer-support/internal/sessionstore/` |
| Visual editor | **None.** No frontend code anywhere in the ecosystem. | — |
| MCP server | **None.** Mentioned in v1.2 future work but not built. | — |

**What's genuinely NEW that a flow subproject would add:**

1. A **serializable, version-stable flow IR** (JSON schema). Today's
   `StateGraph[S]` is Go-code-only; you cannot ship a flow as a file.
2. A **DAG executor with layered vertex execution** (Langflow's
   `vertices_layers` + `RunnableVerticesManager`). `StateGraph`'s
   one-node-at-a-time loop (`graph.go:160-204`) cannot run independent
   branches in parallel by construction.
3. A **typed I/O port** model so two nodes can be connected by name +
   type, with compatibility checked at load time. Today inter-tool
   contracts are duck-typed via `Tool.Execute(args) (string, error)`.
4. A **flow registry + run-history store** — load by ID, list, version,
   inspect prior runs. Today nothing persists at the flow level.
5. An **HTTP service** that loads a JSON flow and exposes
   `POST /flows/{id}/run` (sync + stream) plus CRUD. The
   customer-support service is hardcoded to one chat pipeline.
6. A **typed flow-event stream** (Vertex started / Vertex finished /
   Flow done) at the orchestration layer, mirroring `StreamEvent` but
   for the flow rather than the LLM.
7. (Optional / scope tier C) a **visual editor** — React Flow canvas
   wired to the HTTP API.

---

## Section 3 — Proposed scope tiers for `llm-agent-flow`

### Tier A — minimal (library-only DAG runner)

**IN:**
- `flow` Go package providing `type Flow struct { Nodes []Node; Edges []Edge }`
  with JSON marshalling.
- `Node` is a typed wrapper around an existing primitive:
  `Tool` | `Agent` (`llm.ChatModel` is reachable transitively via Agent).
- `Edge.SourcePort` / `Edge.TargetPort` (string names) + `Compatible(srcType, dstType)` check.
- Topological-sort executor with per-layer parallelism via
  `pkg/fanout`. Cycle detection → error at `Compile`.
- Typed `FlowEvent` union (FlowStarted, NodeStarted, NodeArgsDelta,
  NodeFinished, FlowDone) following K1 streaming idiom.
- Pluggable `NodeRegistry` so callers register their own node
  constructors (`func(json.RawMessage) (Node, error)`).
- Examples + tests using `ScriptedLLM` from `llm-agent`.

**OUT:** No HTTP service. No persistence. No CLI. No visual editor.
No conditional/cycle support (use `orchestrate.StateGraph` for that).

**Direct deps:** `github.com/costa92/llm-agent` only. Stdlib otherwise.

**Position in dep graph:**
`flow → llm-agent → llm-agent-rag` (siblings only). Symmetric to
`llm-agent-otel`.

**Effort:** small (1-2 phases, ~2 weeks side-project pace).

**Risks:**
- Duplicates `orchestrate.Supervisor` if Node-types overlap with
  Supervisor Workers; **mitigation**: explicitly position `flow` as
  DAG-shaped and `Supervisor` as LLM-driven-routing-shaped.
- No persistence means no run-history → harder to demo as a "flow
  product." Library tier accepts this trade.

### Tier B — substantive (library + HTTP service + CLI)

**IN:** All of Tier A, plus:
- A separate sub-binary `cmd/flowd` exposing
  `POST /flows`, `GET /flows`, `GET /flows/{id}`, `PATCH /flows/{id}`,
  `DELETE /flows/{id}`, `POST /flows/{id}/run` (sync),
  `POST /flows/{id}/run/stream` (SSE), `POST /flows/validate`,
  `GET /runs/{run_id}`. Mirrors Langflow v1 surface but slimmer.
- A `runstore` package with two implementations: in-memory + SQLite
  (`modernc.org/sqlite`, reusing the dep already vetted by
  customer-support's sessionstore — `llm-agent-customer-support/go.mod`
  line 14). Persists `flow_def`, `run`, `run_event` tables.
- `cmd/flow` CLI mirroring `lfx run` / `lfx serve` shape:
  `flow run <file.json> --input "..."` and
  `flow serve <file.json> --addr :7861`.
- Conditional edges (lift `StateGraph[S]`'s `ConditionFunc` into the
  flow IR as `Edge.Condition: jsonata-or-cel-or-plain-go` — pick CEL,
  see Risks).
- OTel decorator support: `otelflow.Wrap(Engine) Engine` lives in
  `llm-agent-otel` as a follow-up; not blocking.
- Optional sister: import-time conversion from a subset of Langflow's
  JSON (just enough to claim "Langflow-compatible" for a starter
  flow) — read-only, no round-trip required.

**OUT:** No visual editor, no auth, no multi-tenancy, no MCP server,
no run-cancellation distributed across nodes (simple ctx cancel only),
no plugin model for non-Go node implementations, no K8s deployment
artifacts.

**Direct deps:** `llm-agent`, plus (in `cmd/flowd` only)
`modernc.org/sqlite` for the run store; CEL eval ⇒
`github.com/google/cel-go` (~6 transitive). Library package itself
stays stdlib-only.

**Position in dep graph:**
`flow (library) → llm-agent → rag`
`flowd (cmd) → flow + sqlite + cel`
`otelflow (in llm-agent-otel sister, optional follow-up) → flow + otel`

**Effort:** medium (3-5 phases, 1-1.5 milestones).

**Risks:**
- **Overlap with `orchestrate.Supervisor` v1.2 Phase 37** if `flow`'s
  HTTP service starts hosting Supervisor-style routing. Resolution: the
  service runs `Flow` (DAG + cycles), not `Supervisor` — Supervisor
  remains a Go-API library primitive that can be *invoked by a Node*.
- CEL adds a transitive dep on the binary side. Acceptable if the CEL
  evaluator is confined to `cmd/flowd` and the `flow` library stays
  stdlib-only via an interface (`Evaluator interface { Eval(ctx, expr,
  vars) (any, error) }`) with a default no-op implementation.
- SQLite schema migrations need their own discipline. Reuse the
  customer-support pattern (raw `database/sql` + embedded SQL
  migrations on boot).

### Tier C — ambitious (library + service + visual editor)

**IN:** All of Tier B, plus:
- A `web/` directory with a React 19 + Vite + `@xyflow/react` canvas,
  Zustand store, TanStack Query, served from `flowd` either via
  `embed.FS` (pre-built static assets) or proxied at dev time.
- Real-time canvas updates (one user, single-tenant) via SSE from
  `/flows/{id}/runs/{run_id}/events`.
- Drag-from-palette node creation; left rail enumerates the
  `NodeRegistry`; right inspector edits node JSON.
- Export / import as portable JSON (`flow.json`).
- An "MCP server" mode exposing each `Flow` as a callable MCP tool
  (matches Langflow's `Flow.mcp_enabled`).

**OUT:** auth, multi-user, plugins, K8s deploy. Still no Langflow
component compatibility (we are NOT a Langflow drop-in; we own the
node taxonomy).

**Direct deps (binary):** Tier B set + a pinned Node toolchain to
build the static assets at CI time. The Go binary embeds via `embed.FS`
and ships zero JS deps at install time.

**Position in dep graph:** same as Tier B; the JS workspace is its
own thing.

**Effort:** large, multi-milestone (3+ months side-project pace).

**Risks:**
- **A JS workspace in a Go-pure ecosystem.** No other sibling has any
  TS. The ecosystem's hard rule is "no K8s"; "no JS" is implicit but
  not codified. This needs explicit sign-off; without it the editor
  is the camel's nose for a frontend platform we don't want to own.
- **Maintenance.** A React 19 / xyflow 12 surface is a moving target;
  Langflow itself ships breaking frontend changes every minor release
  (see `RELEASE.md` / `CHANGELOG.md`).
- **Scope blur.** Once you have a canvas, users expect node-creation
  UI, validation tooling, theming, mobile, auth ... none of which is
  on the table.

---

## Section 4 — Recommendation

**Pick Tier B.** Justify:

1. Tier A is the library inside Tier B with one switch flipped; we get
   the lib + service in one go without paying Tier C's frontend tax.
2. **Tier B does NOT duplicate `orchestrate.Supervisor` (Phase 37).**
   `Supervisor` is a Go API that wraps `StateGraph[S]` to drive LLM-
   routed multi-agent loops (Planner / Workers / ParseDispatch /
   BuildAggregate per `v1.2-ROADMAP.md` line 171). `flow` is a
   JSON-serializable DAG IR + executor. The two compose:
   `Supervisor` can be the implementation behind a single `flow`
   Node, and a `flow` Edge can route to a `Supervisor`-backed Node.
   Different shape, different lifetime, different consumer. The
   research SUMMARY's KC-1 explicitly chose Supervisor-as-StateGraph-
   facade so this composition works.
3. **Visual editor is OUT.** The ecosystem has no precedent for JS,
   and the visual editor is the largest single cost driver in
   Langflow's repo (`src/frontend/` has more LOC than `src/backend/`).
   Strategy: ship a clean JSON schema + Go SDK + HTTP API; let the
   community (or a future, optional `llm-agent-flow-ui` sister) own
   the canvas. We document a minimal `flow.schema.json` and ship
   `flow run` + `flow serve` so a developer never has to touch a UI.
4. **Naming.** `llm-agent-flow`. Alternatives considered:
   - `llm-agent-workflow` — too long, no win.
   - `llm-agent-orchestrator` — collides with `llm-agent/orchestrate/`.
   - `llm-agent-graph` — too generic; "graph" already means GraphRAG
     in `llm-agent-rag`.
   - `llm-agent-dag` — accurate but jargon-heavy and excludes the
     conditional-edge feature.
   `flow` matches the Langflow inspiration and the React Flow
   vocabulary without claiming Langflow compatibility.
5. **Ship a CLI AND an HTTP service. Library-only is insufficient.**
   The CLI is `flow run/serve <file.json>` (mirrors `lfx run`/`lfx
   serve`); the HTTP service is the same binary in long-running mode.
   Both compile from `cmd/flow` / `cmd/flowd` inside the new repo.
6. **Module path:** `github.com/costa92/llm-agent-flow` (matches sibling
   convention `llm-agent-{rag,providers,otel,customer-support}`).
7. **Branch / repo conventions:** default branch `main` (matches every
   sibling except `llm-agent-rag` which is `master`); independent SemVer
   track starting `v0.1.0`; release-precheck gate from
   `.planning/codebase/ARCHITECTURE.md` Architectural Constraints (no
   `replace` on tagged-release branches, `go.work` `.gitignore`d, CI
   runs `GOWORK=off`).

---

## Section 5 — Non-goals (cited against ecosystem rules)

- **No K8s / Helm packaging.** `README` Project rules item 4 + STACK.md
  Notable Rules item 4 + ARCHITECTURE.md "Architectural Constraints"
  bullet "No K8s / Helm anywhere — standing non-goal". Single-binary
  Docker container at most.
- **Library tier (`flow/`) stays stdlib-only.** Mirrors `llm-agent`
  rule: non-stdlib deps go to the `cmd/` binaries, never to importable
  packages (`README` item 1 + `STACK.md` Notable Rules item 1).
- **No `replace` directives on tagged-release branches.** INFRA-04;
  `STACK.md` Notable Rules item 2.
- **No visual editor in v0.x.** Reconsider only if the Tier B HTTP API
  + JSON schema has external users requesting it. Owning a React
  codebase is a strategic decision, not a phase deliverable.
- **No Langflow JSON drop-in compatibility goal.** A minimal import
  shim for a subset of node types is fine; reaching parity with
  Langflow's 100+ components is explicitly not the goal.
- **`flow` does NOT replace `orchestrate.StateGraph[S]`.** They
  coexist: `flow` is the *file* format and DAG executor; `StateGraph`
  is the *in-process* state machine. Removing or merging is a
  v1.x-style breaking change and not on the table.
- **No flow-engine-becomes-its-own-AI-framework.** No bundled prompt
  library, no built-in chains, no agentic-router-of-the-week. Nodes
  wrap existing `Tool` / `Agent` / `ChatModel`; primitives stay in
  `llm-agent`.
- **No auth in v0.x.** Matches the customer-support service's
  "demo-only" stance (`STACK.md` Platform Requirements).
- **No multi-tenant DB.** Single-user SQLite; matches existing
  `sessionstore` (`llm-agent-customer-support/internal/sessionstore/`).

---

## Section 6 — First-phase outline (if greenlit)

Phase 1 = the walking skeleton for `llm-agent-flow v0.1.0`.

1. **Repo bootstrap.** New GitHub repo `costa92/llm-agent-flow`; `main`
   default; `go.mod` `module github.com/costa92/llm-agent-flow go 1.26.0`;
   single require `github.com/costa92/llm-agent v0.6.0` (post-v1.2
   Supervisor tag). Add to umbrella `scripts/eco.sh` + `go.work`.
2. **`flow.schema.json`** — author and freeze the v0 JSON shape:
   `{ id, name, description, nodes: [{ id, type, config }],
   edges: [{ source: {node, port}, target: {node, port}, condition? }] }`.
   Crucially this is a SUBSET of React Flow's shape — node `type` is
   our registry key, not React Flow's `genericNode`.
3. **`flow` library package** — `Flow`, `Node`, `Edge`, `Port` Go
   types; `Load(r io.Reader) (Flow, error)`; `Validate(Flow) error`
   (cycles, dangling edges, port-type mismatch).
4. **`NodeRegistry` + `Tool` adapter** — wrap any `agents.Tool` as a
   `Node` with one input port (`input`) and one output port
   (`output`). Wrap any `agents.Agent` similarly. This gets us a
   non-trivial demo with zero new node types written.
5. **DAG executor (`Engine`)** — topological-layer evaluation;
   per-layer parallel fanout via `llm-agent/pkg/fanout`;
   `Run(ctx, Flow, inputs) (Outputs, error)` for one-shot and
   `RunStream(ctx, Flow, inputs) (<-chan FlowEvent, error)` for the
   streaming variant. Reject cycles for v0.1 with a clear error.
6. **`FlowEvent` typed union** — mirror K1 streaming idiom:
   `FlowStarted | NodeStarted | NodeOutputDelta | NodeFinished |
   FlowDone | FlowErr`. Stable `NodeID` field; closes after terminal.
7. **`cmd/flow run <file.json>`** — load → validate → execute →
   pretty-print outputs. Use `ScriptedLLM` from `llm-agent` for the
   golden test path so the binary runs in CI without keys.
8. **`cmd/flow serve <file.json>` (minimum mode)** — load + validate
   at boot; expose `POST /run` (sync) and `POST /run/stream` (SSE).
   No DB. Persistence is Phase 2.
9. **Example flow + integration test** — `examples/echo_chain/flow.json`
   chains two `ScriptedLLM`-backed nodes; `examples_test.go` asserts
   final output. This is the canonical "did it actually run" gate.
10. **Phase exit gate** — `GOWORK=off go vet ./... && go test ./...`
    green; `flow run examples/echo_chain/flow.json` returns the
    expected output deterministically; doc README explains the JSON
    schema with one annotated example; tag `v0.1.0`.

