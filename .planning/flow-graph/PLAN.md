# Flow Plan: typed generic Graph builder + structured/streaming ports

Add a **code-first, compile-time-typed `Graph[I, O]` builder** to `llm-agent-flow` — the eino-style orchestration core (typed nodes, structured data between nodes, automatic streaming adaptation, native ChatModel/Template/Tool/Lambda/Branch node types) — **without abandoning the JSON DAG**. The existing serializable `Flow` IR (map[string]string data flow, CEL conditional edges, layered parallel execution, SQLite-persisted run history, flowd HTTP service) is this project's differentiator versus eino and stays the first-class, default path. The typed `Graph` is a **second, additive construction surface** that targets in-process Go programs that want eino-grade type safety and end-to-end streaming and are willing to give up free serialization/visualization for the parts that can't be expressed in JSON.

Positioning statement: **JSON DAG = portable, persistable, visualizable, string-typed, CEL-routed. Typed Graph = compile-checked, `any`/structured-typed, streaming-native, Go-closure-capable.** They share one execution engine after a v1 generalization of the data carrier from `map[string]string` to `map[string]any`. The typed Graph is "JSON DAG plus types plus closures" — every typed Graph that uses only serializable node kinds can be lowered to the JSON IR; Graphs that embed Lambda closures or Go-typed structs are program-only and are explicitly flagged "not serializable / not checkpointable."

Status: **PLAN — RE-SCOPED after 2-round review (Plan agent + codex). Verdict: NEEDS-REWORK → now folded into a co-designed "flow v2" effort with `flow-checkpoint`.**

---

## ⚠️ 架构决定（2026-06-05，用户拍板）— 取代下文 v1 "shared-engine additive" 框架

两轮审核把"**typed Graph 与 JSON Flow 在 carrier 泛化后共用现有引擎**"定为**单一最高风险假设**——若错，沉掉本计划 + 逼 checkpoint 返工。结构性证据：

- **`flow/graph` 独立包调不到未导出的 `flow.runAny`**——而 flow 的导出契约全是 string 类型。要么导出 `runAny`（加 API 面、破坏 internal 封装），要么 graph 复用不了引擎。下文 Decision 2-C / Phase 1 task 1 的"内部泛化 + string wrapper"在这点上不成立。
- **`Runner`（`runner.go:11`）和 `FlowEvent`（`event.go:29`）是冻结的 string 公共面**，`compatibility.md:9` 禁止改签名。"同引擎 + any 值 + 同事件"**不是小内部重构**，是破坏性演进。
- **flowd 缓存的是具体 `*flow.Engine`（`server.go:145`）、store run API 是 string-map（`store.go:118`）、事件名/payload 手写映射（`server.go:745,764`）**——typed graph 若要共享 otel/flowd，独立执行器也得配全新 TypedRunner/decorator/server。

**决定**：放弃"additive minor、共用现有引擎"路线。flow-graph = **`llm-agent-flow` v2 引擎重设（major bump）**，与 `flow-checkpoint` **联合设计**：typed 数据载体、`Runner`/decorator、`FlowEvent` 形状、store 语义、**checkpoint 序列化**一次性协同设计，不并行、不分先后。原计划下文（泛型 builder API、reflect 类型对齐、box/concatenate 流式、节点类型）**作为 v2 的设计参考保留**，但凡涉及"保留 v0.1 string API 为 wrapper / additive minor / Phase 1 先泛化现有引擎"的措辞**一律作废**，由 v2 协同设计取代。

**v2 协同设计前的先决修复（contract 仓，先行）**：
- `llm.AccumulateStream` 的 EOF 判断用 `err.Error()=="EOF"` 而非 `errors.Is(io.EOF)`（`stream.go:175`）——concatenate 适配器依赖它，**建链前先修 contract**。
- 统一 Tool 面：flow 本地 `Tool{Name,Execute}`（`node.go:70`）vs contract `agents.Tool{+Description,Schema}`（`tool.go:14`）——v2 Tool 节点要决定吃哪个。

**与 checkpoint 的联合点**：typed 值的可序列化性、`StreamReader` 不可快照、layer-边界快照——见 `flow-checkpoint/PLAN.md`，两份计划在 v2 里是一个工程的两面。

---

Status (原始，部分作废见上): **PLAN — not started.** This is the largest of three sibling planning efforts; it MUST ship in stages. v1 = typed builder + `any` engine + box/concatenate streaming + Lambda/ChatModel/Template/Tool/Passthrough/Branch. Merge/copy stream adaptation, Parallel sugar, and graph State are explicitly deferred to v2/v3.

> **Cross-plan contract (pinned in `.planning/eino-gap-plans/README.md`):** the Template node consumes `prompt.Requester` from the sibling `chat-template` plan — `FormatRequest(ctx, map[string]any, []llm.Message) (llm.Request, error)`. This resolves open question CT-1 below (the Template node's signature is locked against the chat-template contract: in = `Vars` bag, out = `llm.Request`).

---

## Decided design / end-state

### Package layout (new sub-package, additive)

```
llm-agent-flow/
  flow/                      # UNCHANGED IR + Engine; data carrier generalized any (back-compat shim keeps string API)
  flow/graph/                # NEW: typed generic builder + compile + typed runtime
      graph.go               # Graph[I,O], NewGraph, AddXxxNode, AddEdge, AddBranch, Compile
      node.go                # typed node adapters (Lambda, ChatModel, Template, Tool, Passthrough)
      stream.go              # StreamReader[T] generic carrier + box/concatenate adapters
      typecheck.go           # reflect-based AddEdge assignability rules
      lower.go               # (v2) Graph -> flow.Flow IR lowering for serializable subset
```

`flow/graph` imports `flow` (for the shared `Engine`/IR after generalization) and imports `llm-agent-contract/llm` + `llm-agent-contract/agents` directly (today contract is an *indirect* dep via core — the ChatModel/Template/Tool typed nodes promote it to a **direct** require; acyclic, contract is a leaf). No new third-party deps.

### Core generic types (copy-ready signatures)

```go
package graph

// Graph is the typed builder. I = whole-graph input, O = whole-graph output.
// Type parameters are recorded only at the boundary; interior node wiring is
// checked by reflect at AddEdge/Compile time (see Decision 1).
type Graph[I, O any] struct { /* unexported: nodes, edges, branches, err */ }

func NewGraph[I, O any]() *Graph[I, O]

// Node handle — opaque, carries the node's declared in/out reflect.Type.
type NodeRef struct { /* id string; inT, outT reflect.Type */ }

// --- typed node constructors (generic; capture real Go types) ---

// Lambda: an inline function. PROGRAM-ONLY (closure can't serialize).
func AddLambdaNode[In, Out any](g *Graph[I, O], id string,
    fn func(ctx context.Context, in In) (Out, error)) (NodeRef, error)

// ChatModel: consumes llm.Request, produces llm.Response (and streams llm.StreamEvent).
func AddChatModelNode[I, O any](g *Graph[I, O], id string, m llm.ChatModel) (NodeRef, error)

// Template: consumes a data map, produces llm.Request. Aligns with the
// chat-template sibling plan's prompt.Requester contract (see Cross-plan).
func AddTemplateNode[I, O any](g *Graph[I, O], id string, t Template) (NodeRef, error)

// Tool: consumes args (typed In marshalled to json), produces string (or typed Out).
func AddToolNode[I, O any](g *Graph[I, O], id string, t agents.Tool) (NodeRef, error)

// Passthrough: identity node, used to fan-in/fan-out or name a join point.
func AddPassthroughNode[T any](g *Graph[I, O], id string) (NodeRef, error)

// --- wiring ---

// AddEdge connects from.out -> to.in. Returns an error at BUILD time if the
// types are not assignable under the three rules (Decision 1). The error is
// also latched onto g so Compile reports it even if the caller ignores it.
func (g *Graph[I, O]) AddEdge(from, to NodeRef) error

// Entry / exit wiring binds the graph's I/O type params to interior nodes.
func (g *Graph[I, O]) Entry(to NodeRef) error   // checks I assignable to to.in
func (g *Graph[I, O]) Exit(from NodeRef) error  // checks from.out assignable to O

// AddBranch: route by a key function over the source node's output.
func (g *Graph[I, O]) AddBranch(from NodeRef, key func(ctx context.Context, out any) (string, error),
    routes map[string]NodeRef) error

// Compile freezes the graph: runs full reflect type-check, cycle/topo,
// lowers onto the shared flow.Engine. Returns a typed Runnable.
func (g *Graph[I, O]) Compile(ctx context.Context, opts ...flow.EngineOption) (*Runnable[I, O], error)

// Runnable is the typed execution handle.
type Runnable[I, O any] struct { /* eng *flow.Engine; inT/outT */ }

func (r *Runnable[I, O]) Invoke(ctx context.Context, in I) (O, error)

// Stream returns a typed stream of the graph output. v1 supports the
// retriever/template -> chatmodel -> parser end-to-end streaming case by
// boxing non-streaming nodes and concatenating upstream streams.
func (r *Runnable[I, O]) Stream(ctx context.Context, in I) (StreamReader[O], error)
```

### Generic stream carrier

```go
// StreamReader[T] is the typed analogue of llm.StreamReader. Same iterator
// contract (Next returns io.EOF at clean end; Close idempotent & mandatory).
type StreamReader[T any] interface {
    Next() (T, error)
    Close() error
}

// box: wrap a single non-streaming value as a one-element StreamReader.
func box[T any](v T) StreamReader[T]

// concatenate: a non-streaming node downstream of a streaming node first
// drains+joins the upstream stream into one T (T must be concatenable —
// llm.Response via llm.AccumulateStream, or string via join). v1 supports
// llm.StreamEvent->llm.Response and string concatenation; other T concat
// requires a registered Concatenator[T] or Compile errors.
type Concatenator[T any] func(StreamReader[T]) (T, error)
```

### End-to-end usage example (template -> chatmodel -> tool)

```go
g := graph.NewGraph[map[string]any, string]()

tpl, _ := graph.AddTemplateNode(g, "tpl", myChatTemplate)   // map[string]any -> llm.Request
cm,  _ := graph.AddChatModelNode(g, "llm", openaiModel)      // llm.Request -> llm.Response
// Lambda extracts the tool args string from the model's tool call:
ex,  _ := graph.AddLambdaNode(g, "extract",
    func(ctx context.Context, r llm.Response) (json.RawMessage, error) {
        if len(r.ToolCalls) == 0 { return nil, errNoCall }
        return r.ToolCalls[0].Arguments, nil
    })
tool, _ := graph.AddToolNode(g, "search", searchTool)        // json.RawMessage -> string

_ = g.Entry(tpl)
_ = g.AddEdge(tpl, cm)      // llm.Request == llm.Request  ✓
_ = g.AddEdge(cm, ex)       // llm.Response == llm.Response ✓
_ = g.AddEdge(ex, tool)     // json.RawMessage -> tool args ✓
_ = g.Exit(tool)            // string == O(string) ✓

run, err := g.Compile(ctx)  // would FAIL here if any AddEdge type was wrong
out, err := run.Invoke(ctx, map[string]any{"q": "weather in SF"})

// Streaming end-to-end: tpl boxed -> cm streams llm.StreamEvent -> downstream
// nodes that need a full Response concatenate; final string streams out.
sr, _ := run.Stream(ctx, map[string]any{"q": "..."})
defer sr.Close()
for { chunk, err := sr.Next(); if err != nil { break }; print(chunk) }
```

### Two-route coexistence architecture

```
   CODE-FIRST ROUTE                         JSON-FIRST ROUTE (unchanged, default)
   graph.NewGraph[I,O]()                    flow.Load(json) / flowd CRUD
        │ AddNode/AddEdge (reflect-checked)        │
        │ Compile                                  │ flow.Compile (Validate + topo)
        ▼                                          ▼
   graph.lower()  ──serializable subset──▶  flow.Flow IR  ◀── (persist, visualize, replay)
        │  (Lambda/typed-struct nodes: NOT lowerable — kept in-process)
        ▼                                          ▼
        └──────────────►  flow.Engine (data carrier: map[string]any)  ◀──────┘
                                   │
                          Run / RunStream  (+ typed Invoke/Stream wrappers)
```

Shared engine, two front-ends. The JSON route only ever sees `any` values that are JSON-encodable (it round-trips through the IR's `json.RawMessage` config + string ports via the back-compat shim). The typed route can carry arbitrary Go values in-process.

---

## Key decisions

### Decision 1 — type-alignment mechanism

**What Go generics CAN guarantee at compile time:** only the *graph boundary* (`NewGraph[I,O]`, `Invoke(I)→O`) and *per-node-constructor* parameters (`AddLambdaNode[In,Out]` forces `fn` to be `func(In)(Out,error)`). Go generics **cannot** express the cross-node constraint "node A's `Out` is assignable to node B's `In`" at `AddEdge`, because by the time edges are added the node handles have been type-erased to a common `NodeRef` (you cannot have a heterogeneous typed list of `Graph` nodes without a different `NodeRef` Go type per node, which makes a builder unusable). eino hit exactly this wall and chose reflection.

- **Option A — pure generics, no reflection.** Force every edge through a generic `Connect[T](g, from NodeRef[_,T], to NodeRef[T,_])`. Requires `NodeRef` to be `NodeRef[In,Out any]` (two type params), which makes `AddEdge` chains and `map[string]NodeRef` (branches) impossible to write. Rejected: kills the ergonomic builder; can't express Branch/Parallel.
- **Option B — generic builder + reflect type-check at AddEdge/Compile (eino route).** Constructors capture real `reflect.Type` for in/out via `reflect.TypeFor[In]()`. `AddEdge` checks assignability immediately and returns an error; `Compile` re-checks the whole graph. Boundary stays generically typed (`Invoke(I)→O`). **Recommended.**
- **Option C — combine: generics at boundary + constructors, reflect interior, plus a thin generic `Connect[T]` helper offered as opt-in sugar for the common single-out→single-in case.** This is B with an ergonomic add-on; ship B's reflect path first, add `Connect[T]` sugar in v2 if demanded.

**Recommendation: B (reflect-checked builder), with the door open to C's sugar.** Reason: it's the only option that supports Branch/Parallel/multi-input nodes and a fluent builder, it matches eino (proven), and "the wrong-type graph fails at `AddEdge`/`Compile`, not at run" still delivers the headline guarantee — just at build time via reflect, not via the Go type checker. The three assignability rules (`typecheck.go`):
1. **same type** — `from.out == to.in`.
2. **interface implementation** — `to.in.Kind()==Interface && from.out` implements it (`from.out.Implements(to.in)` or, for pointer receivers, `reflect.PointerTo(from.out)`).
3. **any** — `to.in == reflect.TypeFor[any]()` accepts anything; `from.out == any` defers the check to runtime (a typed downstream then does a checked assertion, error on mismatch).

### Decision 2 — two-route coexistence model

- **Option A — independent new engine for typed Graph.** Duplicates topo/parallel/event/otel-decorator/store wiring. Rejected: huge surface duplication, splits the otel + flowd ecosystem, violates "don't over-build."
- **Option B — typed Graph is purely a construction layer that lowers to today's `map[string]string` IR.** Clean for serialization but **cannot carry structured/streaming data** (the whole point) — string ports can't hold `llm.Response` or `StreamReader`. Rejected as the *primary* mechanism; retained as the *optional* lowering path for the serializable subset (Decision: `lower.go`, v2).
- **Option C — generalize the existing engine's data carrier from `map[string]string` to `map[string]any` + per-port type metadata; both routes share it.** The JSON route keeps string behavior via a back-compat shim (string ports are just `any` holding `string`); the typed route puts real Go values in. **Recommended.**

**Recommendation: C (shared `any` engine) + B's lowering as an opt-in for serializable graphs.** Impact on the JSON DAG's advantages: persistence/visualization/replay are **preserved for the JSON route and for any typed Graph that lowers** (only serializable node kinds, JSON-encodable port values). Typed Graphs containing Lambda closures or non-JSON Go structs are **in-process only** — `Compile` sets a `Serializable() bool` flag; `flowd`/store reject non-serializable graphs with a clear error. This keeps the differentiator intact where it applies and is honest where it can't.

Engine generalization is the load-bearing v1 task: `Run(ctx, map[string]string)` becomes the typed-friendly `runAny(ctx, map[string]any)`; the existing exported `Run/RunStream(map[string]string)` are kept verbatim as thin wrappers that box strings into `any` and unbox on output (zero API break — see Decision 6).

### Decision 3 — node-to-node data carrier

- **Option A — `any` + a type registry/metadata per port.** One engine, one event shape; values are `any` at the seam, type-checked at edges by reflect (Decision 1). Existing string nodes become `any` nodes holding `string` via an adapter. **Recommended.**
- **Option B — fully parameterized typed nodes all the way through.** Forces the parameterized-NodeRef problem of Decision 1-A. Rejected.

**Recommendation: A.** Compatibility with existing string-map nodes: the engine's port store becomes `map[string]map[string]any`; a `stringPortShim` wraps every existing `NodeKind` (string in/out) so it reads `any`→`string` on input and writes `string`→`any` on output. `cloneStrMap` is kept for the string API surface; a new `cloneAnyMap` (shallow) serves the `any` path. Existing `MetadataAware` stays string-keyed and untouched.

### Decision 4 — streaming ports through the DAG

Carrier: a per-port "stream-or-value" union. Internally each port value is either a concrete `T` or a `StreamReader[T]`. The four eino adaptations:
- **box** (value → stream): trivial one-element reader. **v1.**
- **concatenate** (stream → value): drain + join when a non-streaming node sits downstream of a streaming one. v1 supports `llm.StreamEvent→llm.Response` (via `llm.AccumulateStream`) and `string` join; other types need a registered `Concatenator[T]` or `Compile` errors. **v1.**
- **merge** (N streams → 1 stream, e.g. fan-in to a Lambda that takes multiple streams). **Deferred v2.**
- **copy** (1 stream → N consumers; a stream can only be read once, so fan-out needs a tee). **Deferred v2.**

**Conflict with layered execution — yes, and it's the central streaming design point.** Today the engine runs strict topological *layers* with a barrier between them (a layer fully completes before the next starts). True end-to-end streaming (`retriever→chatmodel→parser` where the parser starts consuming before the model finishes) **breaks the barrier** for the nodes on a streaming path. v1 resolution (minimal, honest): **`Invoke` keeps the existing layered/barrier engine unchanged** (box+concatenate make every node see a complete value — correctness preserved, no real-time streaming inside). **`Stream` is restricted in v1 to a *linear chain* of nodes** (the common `template→chatmodel→parser` shape): the engine runs them as a pipeline, threading `StreamReader[T]` from one to the next, only boxing/concatenating at impedance mismatches. Branch/Parallel graphs fall back to `Invoke` semantics under `Stream` (final output boxed). This avoids rewriting the parallel scheduler in v1; full streaming-through-parallel is **deferred to v3**.

### Decision 5 — native node-type landing order

- **v1 (MVP):** `Lambda` (unblocks everything — any Go function becomes a node, lets us test the engine without real providers), `ChatModel`, `Template`, `Tool` (wrap the existing tool path), `Passthrough`. `Branch` (key-routing — the typed analogue of CEL edges, needed for any non-linear graph).
- **v2:** `Parallel` (sugar over fan-out/fan-in to N nodes + a join Lambda; the engine already runs siblings concurrently, so this is mostly builder ergonomics + merge stream support), `Retriever` (once a Retriever contract exists), Branch with multi-key.
- **Rationale:** Lambda + Passthrough + Branch are the structural minimum; ChatModel + Template + Tool are the three component nodes that make the `template→model→tool` headline example real and align with sibling plans. Parallel is deferrable because the JSON route + Branch already cover fan-out for v1.

### Decision 6 — graph State: in or out of scope?

**Recommendation: OUT of this plan — split to a dedicated small follow-up (`flow-graph-state`), sequenced AFTER v1.** Reasons: (1) request-level shared state + `StatePreHandler`/`PostHandler` + auto-locking + streaming-state variants is a self-contained feature with its own API surface and test matrix; bundling it bloats the MVP and violates the staging constraint. (2) It is **tightly coupled to the `flow-checkpoint` sibling plan's serialization** — see Cross-plan interaction. v1 ships stateless typed graphs (state threaded explicitly through node I/O, which Lambda already allows). When State lands, it must declare its serializability posture jointly with checkpoint.

---

## Phased implementation plan

### Phase 1 — MVP: typed builder + `any` engine + box/concat streaming

Each task TDD: write the test described in "Verify" first, watch it fail, then implement.

1. **Generalize engine data carrier to `any`.** Introduce internal `runAny(ctx, map[string]any) (map[string]any, error)`; port store becomes `map[string]map[string]any`; add `stringPortShim` wrapping existing `NodeKind`s; keep exported `Run/RunStream(map[string]string)` as boxing wrappers.
   - *Verify:* all existing `flow` package tests pass unchanged (`go test ./flow/... -count=1` green); a new test drives `runAny` with a non-string `any` value end-to-end through a 2-node pipeline. API snapshot for `flow` package is byte-identical (no exported change yet).

2. **`flow/graph` package skeleton + reflect type-check.** `NodeRef` with `inT/outT reflect.Type`; `AddEdge` implementing the three assignability rules; error latching on `g`.
   - *Verify:* a graph wiring `llm.Response → (node wanting llm.Request)` returns an error at `AddEdge` AND at `Compile`; a graph wiring matching types compiles. Interface rule: `*concreteTool → agents.Tool` input passes. `any` source defers and a runtime-mismatch test errors in `Invoke`.

3. **Lambda + Passthrough nodes + `Compile` + `Invoke`.** Lower the typed graph onto the generalized engine (each typed node becomes an internal `anyNode` adapter).
   - *Verify:* `NewGraph[int,int]` with two Lambda nodes `(x)->x+1` chained gives `Invoke(ctx,1)==3`; type mismatch between Lambdas fails at build.

4. **ChatModel + Template + Tool nodes.** ChatModel node calls `llm.ChatModel.Generate`; Template node calls `prompt.Requester.FormatRequest` → `llm.Request`; Tool node wraps `agents.Tool` (reuse `FromAgentTool` shim, marshal typed In to json).
   - *Verify:* the end-to-end `template→chatmodel→tool` example test (using `llm.scripted` mock model from contract) returns the expected string via `Invoke`.

5. **Branch node (key-routing).** `AddBranch(from, key, routes)`; unselected branches skip (reuse engine's activation/skip model).
   - *Verify:* a graph branching on a Lambda key routes input "a" to nodeA, "b" to nodeB; the non-selected node is skipped (assert via RunStream events).

6. **Streaming: box + concatenate, linear `Stream`.** `StreamReader[T]`, `box`, `concatenate` (with `llm.AccumulateStream` for `llm.Response`); `Runnable.Stream` for linear chains; non-linear graphs fall back to boxed-final-value.
   - *Verify:* `template→chatmodel→parser(Lambda)` `Stream` yields incremental output where the chatmodel node streams (use a scripted streaming model emitting 3 text deltas); parser sees concatenated text; `sr.Close()` causes no goroutine leak (goroutine-count assertion).

7. **Docs + API snapshot + example.** New `flow/graph` doc.go; add an `examples/typed_graph/` example; regenerate `api/v0.2.snapshot.txt` (new package = additive).
   - *Verify:* `internal/apisnapshot` test green with the additive snapshot; example test green.

**MVP boundary:** v1 ships typed `Invoke` for arbitrary graphs and typed `Stream` for **linear chains only**. No merge/copy, no Parallel sugar, no graph State, no lowering-to-JSON.

### Phase 2 — lowering, Parallel, merge/copy

8. **`lower.go` — typed Graph → `flow.Flow` IR for the serializable subset.** Graphs using only ChatModel/Template/Tool/Branch/Passthrough nodes whose port values are JSON-encodable lower to JSON; `Compile` exposes `Serializable() bool`; Lambda/non-JSON graphs report why they can't lower.
   - *Verify:* a serializable typed graph lowers, round-trips through `flow.Marshal`/`Load`, and the reloaded JSON flow `Run`s to the same output; a Lambda graph reports `Serializable()==false` with a node-level reason.
9. **Parallel node** (fan-out to N + join Lambda) + **merge** stream adapter.
10. **copy** stream adapter (tee) for stream fan-out.

### Phase 3 — streaming through Branch/Parallel; graph State (separate plan)

11. Real-time streaming across non-linear topologies (requires partial-barrier scheduler rework).
12. Graph State — split to `flow-graph-state` plan; co-designed with `flow-checkpoint`.

---

## Risks / open questions / cross-plan interaction

### Cross-plan: `chat-template`
The `AddTemplateNode` consumes a `Template` interface that **must match the `chat-template` sibling plan's `prompt.Requester` contract** (it produces `llm.Request` from a `Vars` map + history). **Open question CT-1 — RESOLVED:** chat-template's base `Format` returns `[]llm.Message`; its optional `Requester.FormatRequest` returns `llm.Request`. This plan's Template node consumes `prompt.Requester`. chat-template lands in `contract` (the `prompt` package), so the Template node imports it directly (acyclic; contract is a leaf).

### Cross-plan: `flow-checkpoint`
The typed/`any` data carrier **makes run-state snapshotting harder than today's `map[string]string`** (which serializes trivially). Analysis for checkpoint:
- **String/JSON-encodable `any` values** snapshot fine via `json.Marshal` + a type tag (the lowering subset). These graphs are checkpointable.
- **Arbitrary Go structs / `llm.Response`** need either a registered codec or `json.Marshal` round-trip (lossy for unexported fields / interfaces) — checkpointable only with a registered `Codec[T]`.
- **`StreamReader[T]` in flight and Lambda closures** are **fundamentally not snapshottable** (a live iterator / a closure has no serializable form). **Recommendation to checkpoint plan:** a graph is checkpointable iff it lowers to JSON (`Serializable()==true`) AND no port currently holds a live stream at the checkpoint boundary; otherwise checkpoint must refuse with a clear error. Checkpoint boundaries should be placed at **layer barriers** (where all values are concrete, not streaming). **Open question CP-1:** does checkpoint snapshot at node boundaries or layer boundaries? This plan recommends **layer boundaries** so no stream is ever mid-flight in a snapshot. (flow-checkpoint plan adopts this: snapshot at layer boundaries, `ErrNotCheckpointable` for mid-stream/non-serializable graphs.)
- **State design guidance (for the deferred state plan):** design graph State to be **JSON-serializable by construction** (struct of JSON-encodable fields), so the State half of a checkpoint is always serializable even when the data-flow half might not be.

### Other risks
- **Reflect cost:** type-checking is at build/compile time (once), not per-invocation — negligible runtime cost. Per-edge `any`-boxing has minor allocation overhead vs strings; acceptable.
- **`Stream` linear-only limitation may surprise users** who build a branching graph and call `Stream`. Mitigation: `Stream` on a non-linear graph is documented to degrade to boxed-final-output, not error — but emit a doc warning; consider a `Compile`-time `StreamCapable() bool`.
- **`copy`/tee semantics (v2):** a `StreamReader` read once can't be re-read; the tee must buffer, which is unbounded for slow consumers. Defer until a concrete need; document the buffering trade-off when built.
- **`any` engine generalization is the riskiest single task** (touches the core run loop every existing test exercises). Mitigation: keep the string API as a verified wrapper and gate Phase 1 task 1 on the *entire existing test suite passing unchanged* before any typed code lands.

---

## Version & release impact
- **New sub-package `flow/graph` is additive** — no break to existing `flow` exported API. The engine generalization (Decision 2-C) is internal; exported `Run/RunStream(map[string]string)` signatures are preserved as wrappers (Decision 6).
- **`llm-agent-contract` moves from indirect → direct require** in `llm-agent-flow/go.mod` (the ChatModel/Template/Tool typed nodes import `contract/llm` and `contract/agents` directly). Pin the latest contract tag; acyclic (contract is a leaf). Note the dependency on the `chat-template` plan shipping `contract/prompt` (v0.5.0) before the Template node can compile against the published tag.
- **Tag `llm-agent-flow v0.2.0`** when Phase 1 lands (new package, new direct dep = minor bump). Regenerate `api/v0.2.snapshot.txt` (additive rows for the `flow/graph` package); the `internal/apisnapshot` test guards it.
- **No flowd/store break in v1** (typed Graphs are in-process; only the v2 lowering path touches the store, and only for serializable graphs).

### Open questions to resolve in review
- **Q1 (Decision 1):** accept reflect-based interior checking (eino route), or hold out for a pure-generic builder with worse ergonomics? Recommend reflect.
- **Q2 (Decision 4):** is linear-only `Stream` an acceptable v1 boundary, or is streaming-through-Branch a hard v1 requirement? Recommend linear-only v1.
- **Q3 (Decision 6):** confirm graph State splits to its own plan after v1.
- **Q4 (Cross-plan CT-1):** RESOLVED — Template node consumes `prompt.Requester` (see Cross-plan).
- **Q5 (Cross-plan CP-1):** confirm checkpoint snapshots at layer boundaries and refuses non-serializable / mid-stream graphs. (flow-checkpoint plan adopts this.)

---

### Critical Files for Implementation
- `llm-agent-flow/flow/engine.go` (data-carrier generalization map[string]string → map[string]any; the run loop every typed node executes through)
- `llm-agent-flow/flow/node.go` (NodeKind/Deps/registry; new anyNode adapter + stringPortShim live alongside this)
- `llm-agent-flow/flow/ir.go` (Flow/Node/Edge/Port — the lowering target in Phase 2)
- `llm-agent-contract/llm/stream.go` (StreamReader/StreamEvent/AccumulateStream — the concatenate adapter and ChatModel streaming node build on this)
- `llm-agent-contract/llm/chatmodel.go` + `llm-agent-contract/agents/tool.go` (the ChatModel/Tool component interfaces the native typed nodes wrap)

### Facts confirmed during planning
- `llm-agent-contract` is currently an **indirect** dep of `llm-agent-flow` (pulled via core `llm-agent v0.9.0`) — the typed nodes promote it to direct, the main go.mod change.
- The `chat-template` and `flow-checkpoint` sibling plans now exist under `.planning/`; CT-1 is resolved against the written chat-template contract.
