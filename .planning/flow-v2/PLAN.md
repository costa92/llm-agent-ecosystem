# Flow v2 — Co-designed Engine Reset: typed `Graph[I,O]` + checkpoint/HITL (one engine, born together)

`llm-agent-flow` v2 is a **major-version engine reset**, not an additive minor. Two efforts that two rounds of review (Plan agent + codex) proved cannot be bolted onto the v0.1 engine — the typed generic `Graph` data plane and the checkpoint/interrupt/resume (HITL) state plane — are **designed as one engine** here. The v0.1 frozen string surface (`NodeKind` / `Runner` / `FlowEvent` / `Store` / `Engine.Run(map[string]string)`, `compatibility.md:9`, baselined in `api/v0.1.snapshot.txt`) is **kept verbatim and frozen forever** in the `flow` package; v2 lands at a **new module path** `github.com/costa92/llm-agent-flow/v2` — the escape hatch `compatibility.md:42-44` already promises for exactly this class of change.

The differentiator — the **JSON DAG** (serializable, persistable, visualizable, flowd-CRUD'd, CEL-routed) — is **not dropped**. v2 keeps a JSON DAG front-end as a first-class, default route. What changes is the *carrier* underneath it: the new engine's port type is `any` (with reflect type metadata), the typed `Graph[I,O]` builder is a **first-class second front-end** of the *same* v2 engine (not an external package reaching into unexported internals), and checkpoint is a **native engine capability** whose snapshot boundary, RunID ownership, and event shape are co-designed with the typed data plane rather than retrofitted.

Positioning: **JSON DAG = portable / persistable / visualizable / string- or JSON-typed / CEL-routed / checkpointable. Typed Graph = compile-checked boundary + reflect-checked interior / arbitrary Go values / streaming-native / closure-capable / checkpointable only when it lowers to serializable form.** Both compile to one v2 IR and run on one v2 engine, one event stream, one Runner interface, one otel decorator, one flowd.

Status: **PLAN — flow v2 co-design spec. Round-2 (codex) review DONE 2026-06-05; the 7 Phase-1 must-fixes (RV-1..7) are RESOLVED in §6 (implementation blueprint).** Phase 0 ships now; Phase 1 follows §6. Supersedes `.planning/flow-graph/PLAN.md` and `.planning/flow-checkpoint/PLAN.md`.

> §1.E's `FiredEdges` cursor is **superseded by §6 RV-1** (activation-based `Activated` + per-edge `EdgeFireState`). Read §6 as the authoritative Phase-1 design where it conflicts with §1.E.

---

## Round-2 review (codex, 2026-06-05) — Phase 1 必改项，先于实现解决

**Phase 0（P-1 AccumulateStream EOF 修复）已确认实现就绪。** 以下是 Phase 1 进实现前必须在本 spec 里解决的 7 点（§1.E / §3 / §4 相应内容据此修订）：

**RV-1 [最高风险，致命] resume 游标模型与真实调度器不符。** 现有引擎是**激活式**：节点跑不跑看 per-node `activated` 布尔（`engine.go:256-265` 跳过未激活节点），**任一入边 fire 即 `activated[target]=true`**（`engine.go:353-354`）——不是"所有前驱完成"也不是"边就绪"模型。所以 §1.E 把 `FiredEdges` 说成"权威游标"是错的：权威状态应是持久化的 **`Activated` map**（spec 已含，但定位错了）。且真正缺的状态是"边求值为 false / 源端口缺失 / 源被 skip"这类**非 fire**信息（`engine.go:334-339` 缺端口不 fire、`engine.go:340-350` 条件 false 不 fire），不只是 fired edges。**修订**：resume 以持久化 `Activated` 为权威，`FiredEdges` 仅用于"不重复 fire"；快照必须同时记录已 skip/false 的边状态。重写 §1.E 的游标定义。

**RV-2 [致命] FC-NEW3"Compile 期拒同层多中断"不可静态判定。** 任何节点的 `Run` 都能返回 `InterruptError`（`node.go:22-25`，v2 `Node.Run` 也只返回 plain error），中断只是 error sentinel——Compile **无法知道哪些节点会中断**，除非 v2 给节点加**显式 interrupt-capable 标记**（如节点声明 `CanInterrupt() bool` 或 config 位）并**禁止未标记节点中断**。当前 spec 两者都没做。**修订**：要么加能力标记 + 强制，要么放弃静态拒、改运行时安全处理（drain 但不让已副作用 sibling 误判）。

**RV-3 [必改] suspend 拼接点要给可执行控制流规则。** 中断 error 若逃进 fanout，`fanout.WithFailFast` 会取消 siblings（`fanout.go:147-150`）——所以中断路径必须**返回 nil 给 fanout、把中断记在带外**。真实顺序：fanout 错误检查（`engine.go:306-319`）→ 之后才 fire 边（`engine.go:322-356`）。suspend 分支必须拼在"结果检查之后、fire 边之前"，并把 `InterruptError` 与真 error 分类。spec 只说了顺序没给规则。注意：先 fire 已完成 sibling 的边可能经**另一条路径**激活中断节点的下游——必须明确"停在下一层之前、注入后再从下一层续"的不变量。

**RV-4 [必改] `/v2` 嵌套模块的 umbrella 工具链没准备好。** `go.work` 只列 `./llm-agent-flow`（`go.work:32-47`），不含嵌套 `v2`；`scripts/eco.sh:237-247` 从 `llm-agent-flow` 跑 `go build ./...` 覆盖不到嵌套 `v2/go.mod`；`cmd/depcheck/main.go:264-270` 只读 `root/name/go.mod`，忽略嵌套；且子目录模块需 **`v2/vX.Y.Z` 前缀 tag**，而 depcheck 只认裸 `vX.Y.Z`（`main.go:276-315`）。**修订 §4**：列出 go.work 注册、eco.sh/depcheck 适配、子目录 tag 约定的具体改动。

**RV-5 [必改] "v1 string flow 在 v2 引擎上输出一致"不免费。** v1 CEL 把 `value` 声明为 string（`cel.go:46-52`），用 `map[string]any{"value": env.Value}`(string) 求值（`condition.go:40-43`）。v2 把 `CondEnv.Value` 放宽到 `any` 后，`value.startsWith("hello")` 会炸。**修订**：给精确规则——JSON/v1 路由的条件**只收 string 值**，不是任意 any。

**RV-6 [必改] otelflow/v2 的 type-assert-and-wrap 会隐藏 ResumableRunner。** 现有 `otelflow.Wrap` 返回具体 `*wrapper`，只实现 `Run`/`RunStream`（`otelflow.go:37-151`）。要让 `ResumableRunner` 能力透出，必须**单独的 resumable wrapper 类型**，照搬现有 wrapper 会把能力藏掉。

**RV-7 [必改] store 生命周期缺 suspend API。** 现有 store 只有 `StartRun`/`FinishRun`（`store.go:118-125`），`FinishRun` 只写 done/failed（`runs.go:37-48`）；flowd 事件循环后总调 `persistFinish`（`server.go:485`）。需新增显式 `SuspendRun`/`MarkRunSuspended`，并给 flowd 一个 suspended-run 例外，否则 checkpoint 存了但 run 行仍 `running` 或被误 finalize。

---

---

## 0. Prerequisite fixes (contract repo — land BEFORE any v2 work)

These are not part of v2's module but block it. Each is a tiny, independently-shippable contract change.

**P-1 — `llm.AccumulateStream` EOF detection (`llm-agent-contract/llm/stream.go:179-181`).** Today `isEOF` does `err.Error() == "EOF"`. The v2 `concatenate` adapter (stream→value) drains `llm.StreamReader` through `AccumulateStream`; a stream wrapper that returns a *wrapped* EOF (e.g. `fmt.Errorf("...: %w", io.EOF)`) silently fails the string compare and turns a clean end into a hang-or-error. Fix to `errors.Is(err, io.EOF)`.
- *Verify (TDD):* add a `StreamReader` test double whose `Next` returns `fmt.Errorf("provider closed: %w", io.EOF)` on termination; assert `AccumulateStream` returns the accumulated `Response` with `err == nil`. This test fails against the current string compare, passes after the fix. Existing `AccumulateStream` tests stay green. Tag `llm-agent-contract v0.5.1` (or fold into the v0.5.0 chat-template release if it hasn't shipped).

**P-2 — Tool-face decision (resolved here, no contract code change required).** Two Tool shapes exist: flow-local `flow.Tool{Name, Execute}` (`node.go:70-73`) and `contract/agents.Tool{Name, Description, Schema, Execute}` (`agents/tool.go:14-19`). **Decision: v2's Tool node consumes `contract/agents.Tool` directly** (the richer face: `Description`/`Schema` feed the ChatModel tool-advertisement path, and it's the published contract type). The narrow `flow.Tool` stays alive only inside the legacy `flow` package for v0.1 back-compat. v2 keeps a one-line internal adapter (`agents.Tool` → the v2 engine's internal tool-exec signature), superseding `flow/adapter_llmagent.go`'s `FromAgentTool`. No new contract symbol; `contract` moves indirect→direct in the v2 go.mod (it already is direct-worthy — see §5).

---

## 1. Architecture spine (A–F): decisions + signatures

All v2 code lives under module path `github.com/costa92/llm-agent-flow/v2`, package root `flow` (the v2 `flow`, a distinct import path from v0.1's `flow`).

```
llm-agent-flow/v2/
  flow/                 # v2 engine: any-carrier run loop, typed events, Runner, checkpoint
      engine.go         # runAny loop, layer scheduler, edge-firing, suspend/resume cursor
      ir.go             # v2 Flow IR (port Type metadata; lowering target)
      node.go           # Node interface (any in/out), registry, Deps
      event.go          # Event (typed union; superset of v0.1 FlowEvent + interrupt/suspend)
      runner.go         # Runner interface (typed), ResumableRunner
      checkpoint.go     # Checkpoint, CheckpointStore, Suspension, RunResult, ErrNotCheckpointable
      interrupt.go      # InterruptError sentinel, InterruptRequest, Interrupt()
      stream.go         # StreamReader[T], box, concatenate, Concatenator registry
      nodes/            # native node kinds: lambda, chatmodel, template, tool, branch, passthrough
  flow/graph/           # typed generic builder front-end (first-class, in-module)
      graph.go          # Graph[I,O], NewGraph, AddEdge, Entry/Exit, AddBranch, Compile, Runnable, Invoke, Stream
      typecheck.go      # reflect assignability (3 rules)
      lower.go          # Graph -> v2 flow.Flow IR for the serializable subset
  flow/store/           # v2 store contract (superset; CheckpointStore native)
      sqlite/           # adds checkpoints table; owns run identity (see E/FC-3)
  cmd/flowd/            # v2 server: typed event mapping, /resume, suspended runs
  api/                  # snapshot baseline (see §5 on the harness filename)
```

### A. Compatibility strategy / module version

**Decision: new module path `/v2`; keep v1 alive frozen; JSON DAG survives as a first-class v2 front-end.**

| Option | Verdict |
|---|---|
| (a) Mutate the v0.1 `flow` package in place (export `runAny`, widen `Runner`/`FlowEvent`) | **Rejected.** Violates `compatibility.md:9` (no re-signing exported funcs/interfaces); the apisnapshot gate fires; every downstream (`otelflow`, `flowd`, customer-support) breaks. Two rounds of review named this the single highest-risk false assumption. |
| (b) Pure new `/v2` engine, **delete** the JSON DAG / v1 path | **Rejected.** The JSON DAG (persist/visualize/flowd) is the project's differentiator vs eino; throwing it away to get typed graphs is a net loss. |
| (c) **New `/v2` module; v0.1 `flow` stays frozen & supported; v2 carries BOTH a JSON DAG front-end (default, serializable) and the typed `Graph` front-end over one `any` engine** | **Recommended.** Honors the frozen promise (v1 untouched), takes the breaking carrier change cleanly at a new import path, and preserves the JSON DAG as a v2 first-class citizen. |

**Migration path (detail in §5):** v0.1 users keep importing `github.com/costa92/llm-agent-flow/flow` — nothing moves under them. New work imports `.../v2/flow`. flowd ships a v2 binary; the v2 JSON IR is a **superset** of v1's (adds optional port `Type` metadata, all v1 JSON flows load unchanged). A v1 flow JSON → v2 engine path is provided so existing persisted flows keep running.

**typed Graph ↔ JSON DAG coexistence in v2:**
```
   TYPED FRONT-END                          JSON FRONT-END (default, serializable)
   graph.NewGraph[I,O]()                    flow.Load(json) / flowd CRUD
     │ AddNode/AddEdge (reflect-checked)      │ flow.Compile (Validate + topo)
     │ Compile → lower serializable subset    │
     ▼                                        ▼
   graph.lower() ──serializable subset──▶  v2 flow.Flow IR  ◀── persist / visualize / replay / checkpoint
     │ (Lambda/Go-struct nodes: in-process,                    │
     │  Serializable()==false, not checkpointable)             │
     ▼                                        ▼
     └──────────────►  v2 flow.Engine (port carrier: any + reflect type meta)  ◀──────┘
                              │
                     Invoke / Stream / RunResumable / Resume  (+ string convenience shim)
```

### B. typed data carrier + type alignment

**Decision: `any` ports + reflect type metadata; reflect-checked builder (eino route, Decision 1-B from flow-graph). The typed execution path is a first-class engine front-end, NOT an external package calling unexported internals.** This directly closes the structural blocker that sank the additive plan: `flow/graph` could not reach v0.1's unexported `runAny`. In v2, `graph.Compile` lowers to the v2 `flow.Flow` IR / builds the v2 engine through **exported** v2 constructors — no internal reach-through.

The v2 engine's port store is `map[string]map[string]any` (vs v0.1's `map[string]map[string]string`). A v2 Node is:

```go
// v2 flow/node.go
type Node interface {
    Inputs() []Port
    Outputs() []Port
    // Run executes against resolved any-typed inputs. Values carry the
    // node's declared in/out reflect.Type for edge checking at compile.
    Run(ctx context.Context, in map[string]any) (map[string]any, error)
}

// Port gains a reflect.Type (nil for the JSON/string route, which uses
// Port.Schema string instead — both routes share the struct).
type Port struct {
    Name   string
    Schema string       // JSON-schema-ish, used by the JSON front-end
    GoType reflect.Type // set by the typed front-end; nil for JSON route
}
```

Typed builder (full signatures; the boundary is generically typed, the interior is reflect-checked):

```go
// v2 flow/graph/graph.go
package graph

type Graph[I, O any] struct { /* nodes, edges, branches, latchedErr */ }

func NewGraph[I, O any]() *Graph[I, O]

// NodeRef is opaque; carries id + declared in/out reflect.Type.
type NodeRef struct { /* id string; inT, outT reflect.Type */ }

// Typed node constructors capture real Go types via reflect.TypeFor.
func AddLambdaNode[GI, GO, In, Out any](g *Graph[GI, GO], id string,
    fn func(ctx context.Context, in In) (Out, error)) (NodeRef, error)             // PROGRAM-ONLY (closure)
func AddChatModelNode[GI, GO any](g *Graph[GI, GO], id string, m llm.ChatModel) (NodeRef, error)   // llm.Request -> llm.Response
func AddTemplateNode[GI, GO any](g *Graph[GI, GO], id string, t prompt.Requester) (NodeRef, error)  // prompt.Vars -> llm.Request
func AddToolNode[GI, GO any](g *Graph[GI, GO], id string, t agents.Tool) (NodeRef, error)            // json.RawMessage -> string
func AddPassthroughNode[GI, GO, T any](g *Graph[GI, GO], id string) (NodeRef, error)

// Wiring. AddEdge checks the 3 assignability rules at build time and
// latches any error onto g so Compile reports it even if ignored.
func (g *Graph[I, O]) AddEdge(from, to NodeRef) error
func (g *Graph[I, O]) Entry(to NodeRef) error    // I assignable to to.in
func (g *Graph[I, O]) Exit(from NodeRef) error   // from.out assignable to O
func (g *Graph[I, O]) AddBranch(from NodeRef,
    key func(ctx context.Context, out any) (string, error), routes map[string]NodeRef) error

// Compile runs full reflect type-check + topo, lowers to the v2 engine,
// returns a typed Runnable. opts thread through to flow.Compile (incl.
// WithCheckpointStore, WithConditionEvaluator).
func (g *Graph[I, O]) Compile(ctx context.Context, opts ...flow.EngineOption) (*Runnable[I, O], error)

type Runnable[I, O any] struct { /* eng *flow.Engine; inT, outT reflect.Type */ }

func (r *Runnable[I, O]) Invoke(ctx context.Context, in I) (O, error)
func (r *Runnable[I, O]) Stream(ctx context.Context, in I) (StreamReader[O], error)

// Serializable reports whether the graph lowered fully to JSON IR (no
// Lambda/non-JSON Go-struct nodes). Checkpointable() == Serializable()
// AND no in-flight stream at the checkpoint boundary (see E).
func (r *Runnable[I, O]) Serializable() bool
func (r *Runnable[I, O]) Checkpointable() bool
func (r *Runnable[I, O]) StreamCapable() bool   // true only for linear chains in v2 (see D)
```

**Three assignability rules (`typecheck.go`, from flow-graph Decision 1):** (1) same type `from.out == to.in`; (2) interface impl `to.in.Kind()==Interface && from.out.Implements(to.in)` (try `reflect.PointerTo` for pointer receivers); (3) `any` — `to.in == reflect.TypeFor[any]()` accepts anything, `from.out == any` defers to a checked runtime assertion in the downstream node. Type-check cost is one-time at compile, not per-invoke.

### C. New typed Runner / decorator / events

The v0.1 `Runner` (`runner.go:11`) and `FlowEvent` (`event.go:29`) are frozen string surfaces; `otelflow.Wrap(inner flow.Runner, cfg) flow.Runner` (`otelflow.go:37`) wraps that string surface. v2 introduces a **typed Runner + typed Event** so otel and flowd attach to the v2 engine; the v0.1 wrapper keeps working against v0.1 unchanged.

```go
// v2 flow/runner.go
type Runner interface {
    // String-keyed convenience entry for the JSON/string route. Returns
    // any-typed outputs boxed back to their declared form.
    Run(ctx context.Context, inputs map[string]any) (map[string]any, error)
    RunStream(ctx context.Context, inputs map[string]any) (<-chan Event, error)
}

// ResumableRunner is the optional capability a checkpoint-enabled engine
// adds. Decorators (otel) detect it by type-assert and wrap if present —
// exactly the AppendRunEvents optional-capability precedent (store.go:78-91).
type ResumableRunner interface {
    Runner
    RunResumable(ctx context.Context, runID string, inputs map[string]any) (RunResult, error)
    Resume(ctx context.Context, runID, token string, humanInput map[string]any) (RunResult, error)
}

var _ Runner = (*Engine)(nil)
var _ ResumableRunner = (*Engine)(nil)
```

```go
// v2 flow/event.go — typed union, superset of v0.1 FlowEvent + HITL kinds
type EventKind uint8
const (
    FlowStarted EventKind = iota
    NodeStarted
    NodeFinished
    NodeSkipped
    NodeInterrupted   // a node returned Interrupt; Request populated
    FlowSuspended     // terminal-for-this-leg: snapshot saved; ResumeToken set
    FlowDone
    FlowErr
)

type Event struct {
    Kind     EventKind
    FlowID   string
    NodeID   string
    Input    map[string]any
    Output   map[string]any
    Outputs  map[string]any
    Metadata map[string]string
    Err      error
    // HITL additive fields:
    Request     *InterruptRequest // set on NodeInterrupted
    ResumeToken string            // set on FlowSuspended
}
```

**otel adaptation (`otelflow` v2):** add `otelflow/v2` with `Wrap(inner flow.Runner, cfg) flow.Runner` over the v2 `Runner`. If `inner` also satisfies `flow.ResumableRunner`, the returned wrapper does too (type-assert + a v2 wrapper that spans `RunResumable`/`Resume` with a `flow.suspend` / `flow.resume` span pair). v0.1 `otelflow.Wrap` is untouched.

### D. Streaming through the DAG

v0.1 `RunStream` only emits node-lifecycle events (NodeStarted/Finished) — there is **no token-level data pipe between nodes**. v2 `Runnable.Stream` is a genuinely new execution path: `StreamReader[T]` threaded port-to-port.

```go
// v2 flow/stream.go
type StreamReader[T any] interface {  // mirrors llm.StreamReader contract
    Next() (T, error)  // io.EOF at clean end
    Close() error      // idempotent, mandatory
}
func box[T any](v T) StreamReader[T]                       // value -> 1-elem stream
type Concatenator[T any] func(StreamReader[T]) (T, error)  // stream -> value
// v2 registers: llm.StreamEvent->llm.Response (via llm.AccumulateStream, depends on P-1),
// and string-join. Other T needs a registered Concatenator or Compile errors.
```

**v2 streaming boundary (honest, minimal):**
- `Invoke` / `RunResumable` run the **layered/barrier scheduler unchanged** — box+concatenate make every node see a complete value. Correctness preserved; no real-time streaming inside.
- `Stream` is restricted in v2 to a **linear chain** (`template→chatmodel→parser`): the engine runs it as a pipeline, threading `StreamReader[T]`, boxing/concatenating only at impedance mismatches. **Branch/Parallel graphs under `Stream` degrade to boxed-final-output** (documented, surfaced via `StreamCapable() bool`), they do NOT error. Real-time streaming through branch/parallel is **deferred to v2.x** (needs a partial-barrier scheduler rework). `merge`/`copy` stream adapters also deferred.

**Conflict with checkpoint (decided): a checkpoint is only taken at a layer barrier where every port holds a concrete value.** A `StreamReader` in flight has no serializable form — checkpoint refuses (`ErrNotCheckpointable`). This is why `Stream` and `RunResumable` are distinct entry points: you cannot suspend mid-stream. See E.

### E. checkpoint / interrupt / resume (born with the engine)

Interrupt mechanism (from flow-checkpoint D1, kept): a node returns a sentinel from `Run`; engine detects via `errors.As`. Zero new required interface method.

```go
// v2 flow/interrupt.go
type InterruptRequest struct {
    Kind   string          `json:"kind,omitempty"`   // "approval"|"input"|"confirm_dangerous"
    Prompt string          `json:"prompt,omitempty"`
    Schema json.RawMessage `json:"schema,omitempty"` // advisory at MVP
}
type InterruptError struct{ Request InterruptRequest }
func (e *InterruptError) Error() string { return "flow: node requested human interrupt" }
func Interrupt(req InterruptRequest) error { return &InterruptError{Request: req} }

// v2 flow/checkpoint.go
type RunResult struct {
    Outputs   map[string]any
    Suspended *Suspension
}
type Suspension struct {
    ResumeToken string
    NodeID      string
    Request     InterruptRequest
}
type CheckpointStore interface {
    SaveCheckpoint(ctx context.Context, cp Checkpoint) error
    LoadCheckpoint(ctx context.Context, token string) (Checkpoint, error)
    DeleteCheckpoint(ctx context.Context, token string) error
    ListCheckpoints(ctx context.Context, flowID string, limit int) ([]CheckpointMeta, error)
}
func WithCheckpointStore(cs CheckpointStore) EngineOption
var (
    ErrNoCheckpointStore = errors.New("flow: interrupt requested but no CheckpointStore configured")
    ErrCheckpointNotFound = errors.New("flow: resume: checkpoint not found")
    ErrFlowChanged        = errors.New("flow: resume: flow definition changed since checkpoint")
    ErrNotCheckpointable  = errors.New("flow: graph not checkpointable (non-serializable port or in-flight stream)")
    ErrMultipleInterrupts = errors.New("flow: multiple pending interrupts in one checkpoint")
)
```

The eight must-fix review items, each resolved:

**FC-3 (fatal) — RunID ownership.** In v0.1, `StartRun`/`newRunID` are private to the sqlite Store (`runs.go:16,144`); the Engine holds only a `CheckpointStore` and cannot mint a RunID that exists in the `runs` table, so `AppendRunEvent` would `ErrNotFound` (`events.go:24-32`). **v2 decision: the Engine receives run identity from the caller; it does NOT mint its own.** Three sub-options were weighed:

| Option | Verdict |
|---|---|
| (a) Engine holds the `Store` and calls `StartRun` itself | Rejected: forces `flow`→`store` import (D5 said keep `flow` free of `store`); also couples library to persistence. |
| (b) Engine accepts an **external `runID`** on the resumable entry points; the *caller* (flowd) owns `StartRun`/`FinishRun` | **Recommended.** flowd already calls `StartRun` (`server.go:380`) and owns the run lifecycle; the Engine just threads the id into the `CheckpointStore` and event-append path. Clean layering preserved. |
| (c) Checkpoint uses an **independent token** unrelated to RunID | Rejected as primary: breaks "one identity for the whole HITL lifecycle" (start→suspend→resume→done greppable in `runs`). |

Resolved signatures thread the run identity in:
```go
func (e *Engine) RunResumable(ctx context.Context, runID string, inputs map[string]any) (RunResult, error)
func (e *Engine) Resume(ctx context.Context, runID, token string, humanInput map[string]any) (RunResult, error)
```
At MVP `token == runID`. The library-only path (no flowd) gets a helper `flow.NewRunID()` exported from v2 so an embedding caller can mint one without the sqlite store — closing the FC-3 gap that `newRunID` was private.

**FC-NEW1 (fatal) — `FinishRun` can't finish a suspended run.** v0.1 `FinishRun` updates only rows `WHERE status = 'running'` (`runs.go:46-48`). v2 store: `FinishRun` accepts `suspended → done/failed` transition (widen the `WHERE status IN ('running','suspended')`). Add `RunStatusSuspended = "suspended"`. flowd transitions `running→suspended` on `FlowSuspended` and `suspended→done` on resume completion.

**OQ-A + FC-NEW2 (fatal/must-fix) — resume cursor must not drop sibling out-edges, and sibling conditional-edge errors must fail before checkpoint.** This is the load-bearing correctness point. In v0.1, edge-firing is a **separate stage after `fanout.Run`** (`engine.go:322-356`), and conditional-edge evaluation can return an error (`engine.go:342-348`). The drain-then-suspend path must, **before writing the checkpoint:**
1. Drain the layer (siblings finish — D6 drain trick, physically sound per review).
2. **Fire the non-interrupted nodes' out-edges**, including **evaluating their conditional edges** — a conditional-edge eval error here must `FlowErr` and write **no checkpoint** (a deterministic failure must not become a recoverable suspension).
3. **Defer only the interrupted node's out-edges** until resume injects `humanInput`.
4. Then snapshot.

The snapshot cursor is **NOT just a layer index.** Under layered + multi-incoming-edge graphs, a bare layer index can re-fire or drop edges on resume. v2 cursor = a **fired-edge set + activation set**:
```go
type Checkpoint struct {
    Version    int    `json:"version"`
    RunID      string `json:"run_id"`
    FlowID     string `json:"flow_id"`
    FlowHash   string `json:"flow_hash"`           // sha256 of canonical v2 Flow JSON

    PortValues map[string]map[string]json.RawMessage `json:"port_values"` // any -> tagged JSON (serializable subset only)
    Activated  map[string]bool                       `json:"activated"`
    // FiredEdges records which edges (by stable flow-edge index) have
    // already fired, so resume re-derives readiness WITHOUT re-evaluating
    // or re-firing them. This — not LayerIndex — is the authoritative
    // resume cursor on multi-input / fan-in graphs.
    FiredEdges []int `json:"fired_edges"`
    // LayerIndex is retained as a coarse progress hint / sanity check only.
    LayerIndex int `json:"layer_index"`

    InterruptNodeID string           `json:"interrupt_node_id"`
    InterruptReq    InterruptRequest `json:"interrupt_req"`
    OriginalInputs  map[string]json.RawMessage `json:"original_inputs"`
    CreatedAt       time.Time `json:"created_at"`
}
```
On resume: restore `PortValues`/`Activated`/`FiredEdges`; inject `humanInput` into the interrupted node's output ports; mark it done; fire **its** out-edges (now adding to `FiredEdges`); then continue the layer loop, where edge readiness is computed against `FiredEdges` so no edge fires twice and no fan-in target is short-changed.

**FC-NEW3 (must-fix) — multiple interrupts.** **Decision: reject at compile/pre-execution, not at drain time.** A graph topology that can place >1 interruptible node in the same layer is rejected by `Compile` when a `CheckpointStore` is configured (a static check: no two interrupt-capable nodes share a layer). This avoids the "drain-then-refuse after siblings already side-effected" trap (codex's concern) and keeps the single-pending-interrupt invariant honest. Multi-pending-interrupt-per-checkpoint is explicitly deferred (open question OQ-3). Runtime still guards with `ErrMultipleInterrupts` as a belt-and-suspenders if the static check is somehow bypassed.

**FC-NEW4 (must-fix) — resume validates humanInput.** `Validate` only checks graph shape (`validate.go:24-94`). `Resume` looks up the runtime node `e.nodes[InterruptNodeID].Outputs()` and rejects `humanInput` keys not in the node's declared output ports (`ErrInvalidHumanInput`). JSON-schema validation against `InterruptRequest.Schema` stays advisory/deferred.

**FC-NEW5 + flowd — suspend events / resume token in SSE + replay; hand-mapped event names updated.** flowd's `eventKindString` (`server.go:745-762`) and `streamPayload` (`server.go:764-793`) are hand-written maps that must learn the v2 kinds: `node_interrupted`, `flow_suspended` (with `resume_token` + `request` in the payload). Add `RunEventNodeInterrupted`/`RunEventFlowSuspended` to the store's `RunEventKind` set. Add `POST /flows/{id}/runs/{runID}/resume` (body = humanInput) and surface `Suspended` (token, node, request) in the run-start response and in `GET /runs/{id}`. Replay (`handleReplayRun`, `server.go:543`) already byte-forwards stored payloads, so suspend/resume events replay for free once persisted.

**typed-value serialization / what's checkpointable.** A graph is checkpointable iff `Serializable() == true` (it lowered fully to JSON IR — no Lambda closures, no non-JSON Go structs) **AND** no port holds a live `StreamReader` at the layer boundary. Port values snapshot as **type-tagged JSON** (a `{"t":"llm.Response","v":{...}}` envelope) decoded via a small **per-type codec registry** (`RegisterCodec[T]`) — preferred over gob+global registry (cross-module `gob.Register` brittleness, the gotcha phase-a hit). Lambda/closure and in-flight stream → `ErrNotCheckpointable` at the suspend attempt, with a node-level reason. Checkpoint boundary = **layer boundary**, by construction (resolves CP-1).

### F. Node types + Tool-face unification

**Landing order (MVP first):** `Lambda` (unblocks all engine testing without providers), `Passthrough`, `Branch` (typed analogue of CEL edges — structural minimum), then the three component nodes `ChatModel` / `Template` / `Tool` that make the headline `template→chatmodel→tool` example real. **Parallel sugar, merge/copy, graph State** are deferred (post-MVP).

- **Template node** consumes `prompt.Requester` (`contract/prompt`, already shipped: `FormatRequest(ctx, Vars, []llm.Message) (llm.Request, error)`, `prompt.go:36-38,203-225`). In = `prompt.Vars`, out = `llm.Request`. Locked against the chat-template contract (CT-1, resolved).
- **ChatModel node** consumes `llm.ChatModel` (`chatmodel.go:17-21`): in = `llm.Request`, out = `llm.Response`; streaming via `ChatModel.Stream` → `StreamReader[llm.StreamEvent]`.
- **Tool node** consumes `contract/agents.Tool` (P-2 decision): in = `json.RawMessage` (typed In marshalled), out = `string`. Supersedes the v0.1 narrow `flow.Tool` + `FromAgentTool` shim.

---

## 2. End-to-end examples

### ① Typed `Invoke` (template → chatmodel → tool)
```go
g := graph.NewGraph[prompt.Vars, string]()

tpl,  _ := graph.AddTemplateNode(g, "tpl", myRequester)   // prompt.Vars  -> llm.Request
cm,   _ := graph.AddChatModelNode(g, "llm", openaiModel)   // llm.Request  -> llm.Response
ex,   _ := graph.AddLambdaNode(g, "extract",
    func(ctx context.Context, r llm.Response) (json.RawMessage, error) {
        if len(r.ToolCalls) == 0 { return nil, errNoCall }
        return r.ToolCalls[0].Arguments, nil
    })                                                     // llm.Response -> json.RawMessage  (PROGRAM-ONLY)
tool, _ := graph.AddToolNode(g, "search", searchTool)      // json.RawMessage -> string

_ = g.Entry(tpl)
_ = g.AddEdge(tpl, cm)   // llm.Request  == llm.Request   ✓
_ = g.AddEdge(cm, ex)    // llm.Response == llm.Response   ✓
_ = g.AddEdge(ex, tool)  // json.RawMessage -> tool args   ✓
_ = g.Exit(tool)         // string == O(string)            ✓

run, err := g.Compile(ctx)       // FAILS here if any edge type is wrong
// run.Serializable() == false   (the Lambda makes it in-process-only / not checkpointable)
out, err := run.Invoke(ctx, prompt.Vars{"q": "weather in SF"})
```

### ② HITL (a node interrupts → suspend → resume)
```go
g := graph.NewGraph[prompt.Vars, string]()
// ... tpl, cm wired as above ...
appr, _ := graph.AddLambdaNode(g, "approve",
    func(ctx context.Context, r llm.Response) (string, error) {
        if needsHuman(r) {
            return "", flow.Interrupt(flow.InterruptRequest{Kind: "approval",
                Prompt: "Approve refund $4000?"})
        }
        return autoDecision(r), nil
    })
// NOTE: this graph has a Lambda → not Serializable → NOT checkpointable.
// For a checkpointable HITL flow, build it on the JSON front-end (serializable
// node kinds only) so Checkpointable()==true. Shown here on the JSON route:

eng, _ := flow.LoadCompile(jsonFlow, reg, deps,
    flow.WithCheckpointStore(cs), flow.WithConditionEvaluator(cel))

runID := flow.NewRunID()                       // or flowd's StartRun (FC-3 option b)
res, _ := eng.RunResumable(ctx, runID, map[string]any{"amount": "4000"})
// res.Suspended != nil; res.Suspended.ResumeToken == runID
//   .NodeID == "approve_refund"; .Request.Prompt == "Approve refund $4000?"

// ... hours later, human clicks Approve in a UI ...
res2, _ := eng.Resume(ctx, runID, res.Suspended.ResumeToken,
    map[string]any{"output": "approved"})       // keys validated vs node.Outputs() (FC-NEW4)
// res2.Outputs == {"result": "refund issued"}  — behavior-transparent vs a non-interrupted run
```
**Verification invariant:** for a flow whose interrupt node, had it returned `{"output":"approved"}` directly, yields outputs `X`, a `RunResumable → Resume("approved")` round-trip yields **exactly `X`**.

---

## 3. Phased roadmap (TDD: write the failing test first, then implement)

### Phase 0 — Prerequisite (contract repo, ships first)
- **P-1** AccumulateStream EOF (§0). **P-2** is a decision, no code.

### Phase 1 — MVP: v2 engine skeleton + typed `Invoke` + checkpoint on the serializable route
This is the **MVP boundary**: one `any` engine, typed `Invoke` for arbitrary graphs, typed `Stream` for **linear chains only**, checkpoint/resume on the **serializable (JSON-lowerable) route only**, flowd `/resume`. No Parallel, no merge/copy, no graph State, no streaming-through-branch.

1. **v2 module + `any` engine core.** New `/v2` module; `runAny` layer loop with `map[string]any` ports; edge-firing as a distinct post-fanout stage (port v0.1 `engine.go:250-357` to `any`); typed `Event` union; v2 `Runner`.
   - *Verify:* a 2-node `any` pipeline (non-string value) runs end-to-end through `Run`; a string convenience shim round-trips a v1-style string-map flow to identical output.
2. **`flow/graph` builder + reflect type-check (3 rules).** `NodeRef` with reflect types; `AddEdge`/`Entry`/`Exit` checks + error latching; `Compile` → v2 engine via exported constructors (no internal reach-through).
   - *Verify:* wiring `llm.Response → (node wanting llm.Request)` errors at `AddEdge` AND `Compile`; matching types compile; interface rule (`*concreteTool → agents.Tool`) passes; `any`-source defers and a runtime mismatch errors in `Invoke`.
3. **Lambda + Passthrough + Branch nodes; `Invoke`.**
   - *Verify:* `NewGraph[int,int]` two chained `(x)->x+1` Lambdas → `Invoke(1)==3`; type mismatch fails at build; a Branch routes "a"→nodeA, "b"→nodeB and skips the other (assert via RunStream events).
4. **ChatModel + Template + Tool nodes.** ChatModel→`llm.ChatModel.Generate`; Template→`prompt.Requester.FormatRequest`; Tool→`agents.Tool` (typed In marshalled to JSON).
   - *Verify:* the §2-① `template→chatmodel→tool` example test (scripted mock model from contract) returns the expected string via `Invoke`.
5. **box + concatenate; linear `Stream`.** `StreamReader[T]`, `box`, `concatenate` (uses `llm.AccumulateStream`, depends on P-1); `Stream` for linear chains; non-linear degrades to boxed-final via `StreamCapable()==false`.
   - *Verify:* `template→chatmodel→parser` `Stream` yields incremental output (scripted streaming model, 3 deltas); parser sees concatenated text; `sr.Close()` leaks no goroutine (goroutine-count assertion).
6. **Lowering + serializability.** `lower.go`: serializable-subset Graph → v2 `flow.Flow` IR; `Serializable()`/`Checkpointable()` flags; Lambda/non-JSON report a node-level reason.
   - *Verify:* a serializable typed graph lowers, round-trips through v2 `Marshal`/`Load`, reloaded flow `Run`s to the same output; a Lambda graph reports `Serializable()==false` with reason.
7. **Checkpoint engine path (serializable route).** `WithCheckpointStore`; interrupt detection via `errors.As`; **drain → fire non-interrupt out-edges (incl. conditional-edge eval that fails BEFORE checkpoint, FC-NEW2) → snapshot with FiredEdges cursor (OQ-A) → return `Suspended`**; `RunResumable`/`Resume` (external runID, FC-3 option b; `flow.NewRunID()` helper); `Resume` validates humanInput vs `node.Outputs()` (FC-NEW4); reject same-layer multi-interrupt topology at Compile (FC-NEW3); in-memory `CheckpointStore` for tests; codec registry for type-tagged JSON port values.
   - *Verify (headline invariant):* `RunResumable → Resume("approved")` yields exactly the non-interrupted output `X`. Negative: fan-in graph where one parent suspends — resume fires the interrupted parent's out-edge and the fan-in target runs exactly once (FiredEdges correctness). Negative: a sibling conditional-edge eval error at suspend time → `FlowErr`, **no checkpoint written**. Negative: `Resume` with an unknown output-port key → `ErrInvalidHumanInput`. A non-serializable (Lambda) graph + interrupt → `ErrNotCheckpointable`.
8. **sqlite CheckpointStore + run-status transition.** `checkpoints` table (additive `CREATE TABLE IF NOT EXISTS`, `open.go:77`); `Save/Load/Delete/ListCheckpoints`; `RunStatusSuspended`; `FinishRun` accepts `suspended→done` (FC-NEW1).
   - *Verify:* Phase-1.7 suite passes against `sqlite.Open(":memory:")`; a WAL persistence test (save → Close → reopen → Load → resume-to-completion); `FinishRun` finishes a suspended run.
9. **flowd v2 integration.** `eventKindString`/`streamPayload` learn `node_interrupted`/`flow_suspended` + `resume_token`; `POST .../resume`; suspended runs in listings; replay carries suspend/resume.
   - *Verify:* SSE replay test — a run emits `flow_suspended`, a `POST .../resume` drives it to `flow_done`, persisted history holds start→suspend→resume→done in seq order.
10. **Docs + API snapshot + examples.** v2 `doc.go`; `examples/typed_graph/` + `examples/hitl/`; snapshot baseline (see §5).
    - *Verify:* apisnapshot test green; example tests green.

### Phase 2 (post-MVP)
- Parallel node sugar; `merge`/`copy` stream adapters; otel `otelflow/v2` resumable wrapper; structural flow-hash diff (only reject resume if the interrupted node's neighborhood changed).

### Phase 3 (deferred)
- Real-time streaming through Branch/Parallel (partial-barrier scheduler rework); graph **State** (`flow-graph-state`, co-designed with checkpoint, JSON-serializable-by-construction); multi-pending-interrupt-per-checkpoint (OQ-3).

---

## 4. Migration / compatibility

- **v0.1 users:** nothing moves. `github.com/costa92/llm-agent-flow/flow` stays frozen, additive-only, baselined at `api/v0.1.snapshot.txt`. The compatibility doc (`compatibility.md:42-44`) already designates `/v2` for breaking changes — v2 spends exactly that escape hatch.
- **JSON flows:** v2 IR is a superset (optional port `Type`/`GoType` metadata); all v0.1 flow JSON loads unchanged. flowd v2 reads existing flow rows. A v1-flow → v2-engine path keeps persisted flows running.
- **flowd:** ship a v2 binary (or a v2 server package); the v1 server stays for v0.1 callers. SSE event names for the existing kinds are byte-identical (`flow_started`/`node_started`/…); only **new** kinds are added.
- **api-snapshot harness — explicit decision (do NOT hand-wave "regenerate v0.2"):** the harness hardcodes `api/v0.1.snapshot.txt` (`apisnapshot_test.go:31`, `apisnapshot.go`) and the `-update` flag rewrites *that* file. Two honest choices, pick one in execution:
  - **(a) v2 gets its own harness + baseline file** under `/v2/internal/apisnapshot/` writing `/v2/api/v2.0.snapshot.txt` — the v2 module is a separate Go module with its own `internal/`, so this is the clean separation. **Recommended.**
  - (b) Keep one harness but parameterize the filename (replace the hardcoded `"v0.1.snapshot.txt"` with a const/flag). Heavier, couples two modules' gates.
  The frozen v0.1 baseline file is **never** regenerated for v2 changes — v2 lives in its own module with its own baseline. Do not run `-update` against `api/v0.1.snapshot.txt` for v2 symbols.
- **Versioning:** v2 module starts at `v2.0.0`. contract moves indirect→direct in the v2 go.mod (Template/ChatModel/Tool nodes import `contract/{llm,agents,prompt}`); pin contract ≥ v0.5.1 (P-1 + chat-template `prompt` package).

---

## 5. Risks / open questions (highest-risk flagged)

**HIGHEST RISK — resume cursor correctness on conditional-CEL edges + multi-layer fan-in.** The `FiredEdges` cursor (§E, OQ-A/FC-NEW2) is the crux. The danger surfaces specifically when: (i) a suspended node's sibling has a CEL conditional out-edge whose evaluation errors — that must fail **before** checkpoint, else a deterministic failure becomes a recoverable suspension; and (ii) a fan-in node has one parent that suspended and others that completed — resume must fire exactly the deferred parent's edge and run the fan-in target exactly once. A bare layer-index cursor is provably insufficient here. **This is the make-or-break invariant; gate Phase 1 on its negative tests (1.7) before building flowd.**

**Other open questions (flagged, not silently decided):**
- **OQ-stream-branch:** v2 `Stream` is linear-only; branch/parallel degrade to boxed-final-output. Is that an acceptable v2.0 boundary, or is streaming-through-branch a hard requirement for a real consumer? *Leaning: linear-only v2.0, revisit on consumer pull.*
- **OQ-JSON-typed:** should the JSON DAG front-end *also* become typed (port `GoType` enforced for JSON nodes), or stay string/JSON-schema-typed while only the `Graph` front-end is reflect-typed? *Leaning: JSON route stays schema-typed (its values must JSON-round-trip anyway); reflect typing is the typed-Graph front-end's job.* This decides whether `lower.go` is the only bridge or whether JSON nodes carry `reflect.Type` too.
- **OQ-3 multi-interrupt:** v2.0 rejects same-layer multi-interrupt at Compile (FC-NEW3). Is single-pending-interrupt too restrictive for approval-fan-out flows? *Defer; revisit only with a concrete consumer.*
- **OQ-codec:** type-tagged JSON + `RegisterCodec[T]` vs requiring node-output types to be JSON-marshalable by contract. *Leaning: contract-marshalable-by-default, codec registry as the escape hatch for `llm.Response`-like interface-bearing types.*
- **OQ-flowhash:** `ErrFlowChanged` on any hash mismatch is conservative (an unrelated node-config tweak blocks resume). Structural-diff refinement deferred to Phase 2.
- **OQ-runid-helper:** `flow.NewRunID()` for the library-only path duplicates the sqlite store's `newRunID` entropy logic. Acceptable (different module); revisit if a `flow-contract` leaf ever emerges.

**Residual risks:**
- The `any`-carrier engine is the riskiest single component (every run path goes through it). Mitigation: build the string convenience shim + a full v1-flow-on-v2 conformance suite before any typed/checkpoint code lands.
- `fanout.WithFailFast` + the drain trick must not mask a *real* sibling error concurrent with an interrupt: if any sibling returns a genuine error, that wins (fail the run, no checkpoint). Cover with a test (sibling error + sibling interrupt same layer → run fails, no checkpoint).
- `copy`/tee buffering is unbounded for slow consumers — deferred; document the trade-off when built.

---

### Critical Files for Implementation
- `llm-agent-flow/flow/engine.go` (the v0.1 run loop, layer scheduler, and the edge-firing-after-fanout stage at lines 322-356 that the v2 `any` engine + FiredEdges suspend cursor are ported from)
- `llm-agent-flow/flow/store/sqlite/runs.go` (private `StartRun`/`newRunID` at 16/144 and `FinishRun`'s `WHERE status='running'` guard at 46-48 — the FC-3 and FC-NEW1 fixes live here)
- `llm-agent-flow/cmd/flowd/server/server.go` (hand-written `eventKindString`/`streamPayload` at 745-793 + `StartRun` ownership at 380 — FC-NEW5 + FC-3-option-b integration)
- `llm-agent-contract/llm/stream.go` (the `isEOF` string-compare at 179-181 — prerequisite P-1; and the `AccumulateStream`/`StreamReader` contract the `concatenate` adapter builds on)
- `llm-agent-flow/internal/apisnapshot/apisnapshot_test.go` (hardcoded `v0.1.snapshot.txt` at line 31 — the §4 harness decision must be made explicitly)

---

## 6. Round-2 resolutions (RV-1..7) — Phase 1 实现蓝图

每条基于真实 `engine.go` 调度事实：节点跑/skip = per-node `activated`（`engine.go:263`）；fanout `WithFailFast` 下真 task 错误落 `results[i].Err`、`runErr` 仅外层 ctx 取消时非 nil（`fanout.go:33-35,93-96`），引擎逐个查 `results[i].Err`（`engine.go:311-319`）；**edge-firing 是 fanout 之后的独立阶段**（`engine.go:322-356`）：缺源端口不 fire（`:334-339`）、条件 false 不 fire（`:340-350`）、cond eval 出错 → 整 run FlowErr（`:343-348`）、**任一入边 fire 即 `activated[target]=true`**（`:353-354`，fan-in 不等所有前驱）。

### RV-1 — resume 游标改为「激活式」（取代 §1.E FiredEdges）

权威游标 = 持久化 `Activated` map；外加 per-edge 终局 `EdgeFireState` 记录 codex 指出的缺失「非 fire」状态。

```go
// v2 flow/checkpoint.go
type EdgeFireState uint8
const (
    EdgePending     EdgeFireState = iota // 源层未处理；resume 时重算
    EdgeFired                            // setPort+activate 已做(engine.go:353-354) — 不重 fire
    EdgeDeadFalse                        // 条件 false(:349) — 本 run 永不 fire
    EdgeDeadNoPort                       // 源跑了但没产出该端口(:335)
    EdgeDeadSrcSkip                      // 源未激活被 skip(:326-328)
    EdgeDeferred                         // 中断节点的出边；resume 注入 humanInput 后再求值
)

type Checkpoint struct {
    Version, RunID, FlowID, FlowHash       // FlowHash=sha256(canonical v2 Flow JSON), ErrFlowChanged
    PortValues map[string]map[string]json.RawMessage // any->type-tagged JSON (RegisterCodec[T])
    Activated  map[string]bool             // 权威 resume 游标(镜像 engine.go:229 activated)
    EdgeStates []EdgeFireState             // len==len(flow.Edges)；按边 stable index
    SuspendLayer int                       // 中断时正在跑的层；resume 从 +1 续；fire-完成-sibling-但停在下一层前的边界
    InterruptNodeID string; InterruptReq InterruptRequest
    OriginalInputs map[string]json.RawMessage; CreatedAt time.Time
}
```
`FiredEdges []int` 删除（语义并入 `EdgeStates==EdgeFired`，但能多区分 4 种 dead + pending）。

**suspend 建快照**（drain 后、写 checkpoint 前；接 RV-3）：对 layers `0..SuspendLayer` 的每个源的每条出边——中断节点出边标 `EdgeDeferred`；源未激活 `EdgeDeadSrcSkip`；缺端口 `EdgeDeadNoPort`；有条件先 `Evaluate`（**eval 错 → FlowErr 且不写 checkpoint**，FC-NEW2），false 标 `EdgeDeadFalse`，否则 `EdgeFired`（这些边在 live drain 已实际 fire，建快照只打标签不再改 `activated`）。

**resume 算法**：`LoadCheckpoint`(无→ErrCheckpointNotFound) → FlowHash 校验(不符→ErrFlowChanged) → 恢复 `activated`/`portValues`(decode type-tagged JSON) → 校验 humanInput 键 ∈ `node.Outputs()`(否则 ErrInvalidHumanInput, FC-NEW4) → 注入为中断节点输出端口 → fire `EdgeDeferred` 边（条件只收 string，RV-5；eval 错→FlowErr）→ **从 `SuspendLayer+1` 续跑 `engine.go:250-357` 原循环，edge-firing 阶段加唯一守卫 `if EdgeStates[i] != EdgePending { continue }`**。fresh run 时 EdgeStates 全 pending → 与 v0.1 逐字一致。

**反例走查**（A→{B,C(cond false),S(interrupt)}→D fan-in）证明：D 只在其 layer 被调度一次（fan-in 的 e3-live + e4-resume 都只是幂等 `activated[D]=true`+setPort）；e3 供 D.x、e4 供 D.y 入参完整无漏激活；已 fire/dead 边 resume 守卫跳过无重复；C 因 e1 dead-false 永不激活；sibling cond eval 错则 suspend 返回 FlowErr 无 checkpoint。

**TDD**：`TestResume_FanInOneParentSuspends`（D 恰跑 1 次 + 输出等于非中断基线）；`TestResume_SiblingDeadFalseEdgeNotReEvaluated`（cond Evaluate 调用数不增）；`TestSuspend_SiblingCondEvalError_NoCheckpoint`。

### RV-2 — 多中断：显式能力标记 + Compile 静态拒

```go
// v2 flow/node.go —— 可选 sibling 接口(沿用 MetadataAware 先例 node.go:43-46)
type InterruptCapable interface { Node; CanInterrupt() bool }
var (
    ErrMultipleInterrupts      = errors.New("flow: multiple pending interrupts in one layer")
    ErrUninstrumentedInterrupt = errors.New("flow: node returned InterruptError but is not InterruptCapable")
)
```
Compile（算完 layers 后、仅当配了 CheckpointStore）：每层 `CanInterrupt()==true` 的节点 >1 → `ErrMultipleInterrupts`。运行时强制：节点返 `*InterruptError` 但未实现/未声明 capable → 当**真 error** 失败（`ErrUninstrumentedInterrupt`），堵住「偷偷中断」绕过静态前提。无 store 时不拒（中断节点退化为普通节点）。
**TDD**：`TestCompile_TwoInterruptCapableSameLayer_Rejected`、`TestRun_UninstrumentedInterrupt_FailsRun`、`TestCompile_OneInterruptPerLayer_OK`。
开放：typed `AddLambdaNode` 要中断需 `AddInterruptibleLambdaNode`（构造标 capable）——Phase 2；HITL MVP 走 JSON 路由（node kind 注册时声明能力）。

### RV-3 — suspend 拼接的可执行控制流

事实：failfast 下中断 error 自然落 `results[i].Err`，但会 cancel runCtx 打断 siblings。规则：**node task 内捕获 `*InterruptError`，带外记录 + 返回 `(nil,nil)`**（让 siblings drain），并先校验该节点 `InterruptCapable`（否则真 error）。插入点 = fanout 结果检查（`engine.go:306-320`，真 error 优先、即使同时有中断也整 run 失败无 checkpoint）**之后、edge-firing（`:322`）之前**：

```
pend := drainPendingInterrupts()
if len(pend) > 0 {
    if checkpointStore == nil { return ErrNoCheckpointStore }
    if len(pend) > 1 { return ErrMultipleInterrupts }            // 不写 checkpoint
    cp, ferr := buildCheckpoint(ctx, layerIdx, pend[0].nodeID, activated, portValues) // RV-1
    if ferr != nil { emit(FlowErr); return ferr }               // sibling cond eval 错
    checkpointStore.SaveCheckpoint(ctx, cp)
    emit(FlowSuspended{ResumeToken: cp.RunID, Request: &pend[0].req})
    return &suspendedSentinel{token: cp.RunID, ...}             // RunResumable 译成 RunResult{Suspended}
}
// else 正常 edge-firing(engine.go:322-356)
```
**不变量**（写回 spec）：suspend 已 drain 当前层；已 fire 所有非中断节点出边并求值其条件边（错→FlowErr 无 checkpoint）；仅延迟中断节点出边；**停在调度下一层之前**——fan-in target 必在更后层，故「先 fire 已完成 sibling 边」绝不会经另一路径把中断节点下游提前跑掉。需 `intMu` 互斥（仿 `engine.go:184/203` 的 pvMu/emitMu）。
**TDD**：`TestSuspend_SiblingDrainsToCompletion`、`TestSuspend_RealSiblingErrorWins`（无 checkpoint）、`TestSuspend_StopsBeforeNextLayer`。

### RV-4 — `/v2` 嵌套模块工具链

- **`go.work`**：`use` 加 `./llm-agent-flow/v2`（嵌套 module 不被父 `use` 递归含）。CI 仍 `GOWORK=off`。
- **`eco.sh`**（`:223-247`）：build/test/release-check 迭代里自动发现嵌套 module——`find <repo> -mindepth 2 -name go.mod -printf '%h\n'`，对每个也 `run_go_cmd`（避免每加一个 vN 改脚本）。
- **`depcheck`**（`main.go`）：`loadNestedModules` WalkDir 找深度≥2 的 go.mod 成独立 `repoInfo`（名如 `llm-agent-flow/v2`）；`repoFromModulePath`（`:139-149`）已把 `.../v2` 映射回父 repo（DAG 边正确，不改）；**子目录 tag `v2/vX.Y.Z`**——给嵌套 module 的 `latestLocalTag` 传 `tagPrefix`，过滤 `HasPrefix` 后 `isSemverTag(TrimPrefix)`。
- **pre-commit replace-guard**（`:36`）：`git ls-files|grep go.mod` 天然含 v2 go.mod、逐 dir 处理，**自动生效**；spec 写明 v2 的本地 replace 相对路径深度 `../../`。
- **TDD**：`TestLatestLocalTag_SubdirPrefix`、`TestLoadNestedModules`；smoke `cd llm-agent-flow/v2 && GOWORK=off go build ./...`。
- 开放：depcheck stale 检测的 `latestByRepo` 键需用 module-path 而非 repo-name 区分 v2（Phase-1 可先只覆盖 build/test，stale 列 known-gap）。

### RV-5 — CEL string→any 精确规则

`CondEnv.Value` **保持 string 不放宽**（`condition.go:43` 逐字不变），JSON/v1 条件路由继续喂 string，`cel.go` 完全复用，`value.startsWith(...)` 永不炸。edge-firing 求值前对源端口 `any` 做 `v.(string)` 投影，非 string 带条件边 → 运行时 FlowErr + **lowering/Compile 静态拒**（「CEL 条件边只允许在 string 载体」）。**typed 非 string 路由用 `AddBranch` 的 Go key 函数路由，不走 CEL**。
**TDD**：`TestEdge_CELConditionOnStringPort_Works`（v1-on-v2 逐字一致）、`TestCompile_CELConditionOnNonStringPort_Rejected`、`TestBranch_TypedRoute_UsesGoKey`。

### RV-6 — otelflow/v2 不藏 ResumableRunner

两个 wrapper 类型：`baseWrapper`（Run/RunStream）、`resumableWrapper{ baseWrapper; rinner flow.ResumableRunner }`（额外实现 RunResumable/Resume，`flow.suspend`/`flow.resume` 成对 span + `flow.resume_token` 属性）。`Wrap` type-assert inner 是否 `ResumableRunner` 决定返回哪个 → 能力透出（返回类型仍 `flow.Runner`，但动态类型可 assert）。v0.1 `otelflow.Wrap` 不动。
**TDD**：`TestWrap_ResumableInnerExposesCapability`(ok==true)、`TestWrap_NonResumableInner_NoFalsePositive`(ok==false)、`TestWrap_ResumeEmitsSpanPair`。

### RV-7 — store suspend 生命周期

`RunStatusSuspended = "suspended"`；新增 `SuspendRun(ctx, runID, resumeToken, interruptNodeID) error`（`UPDATE ... SET status='suspended',resume_token,interrupt_node WHERE id=? AND status IN('running','suspended')` 幂等）；`runs` 表加 `resume_token`/`interrupt_node` 列（与 checkpoints 表同批 additive）；**`FinishRun` 放宽** `WHERE status IN('running','suspended')`（FC-NEW1，允许 suspended→done）；flowd 事件循环捕获 `FlowSuspended` → 调 `SuspendRun` 而非无条件 `persistFinish`（`server.go:485`），resume 端点跑完照常 `persistFinish`。
**TDD**：`TestSuspendRun_RunningToSuspended`、`TestFinishRun_SuspendedToDone`、`TestFlowd_SuspendedRunNotFinalized`、复用 WAL 持久化测试 resume-to-completion。
开放：确认 `AppendRunEvent` 不卡 status（suspended 非终态天然允许）；`RunRecord` 加 `ResumeToken`/`InterruptNode` 供 `GET /runs/{id}` 透出。
