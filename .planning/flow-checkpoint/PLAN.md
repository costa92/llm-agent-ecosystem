# Flow Checkpoint / Interrupt / Resume Plan: Human-in-the-Loop (HITL) for `llm-agent-flow`

Add **pause → wait-for-human → resume** to the `llm-agent-flow` engine, mirroring cloudwego/eino's checkpoint + interrupt + resume. A flow run reaches a node that needs a human (approval, parameter completion, dangerous-op confirmation), the engine **suspends**: it snapshots run state to a checkpoint, returns a **resume token**, and stops. Later a caller invokes `Resume(ctx, token, humanInput)` to rehydrate the snapshot, inject the human's input into the interrupted node, and continue from the suspension point to completion.

Goal alignment (PROJECT.md): "core stays stdlib-only minimal; reference services live in sister repos." Checkpoint is a `flow`-repo capability built on the **existing** `flow/store` abstraction (already stdlib + modernc sqlite), reusing the `map[string]string` data flow which is the natural, JSON-serializable substrate this plan exploits.

Status: **RE-SCOPED after 2-round review (Plan agent + codex). Verdict: NEEDS-REWORK → co-designed with `flow-graph` as one "flow v2" effort.**

---

## ⚠️ 审核修订 + 架构决定（2026-06-05，用户拍板）— 必读

用户决定 flow-graph 走 **v2 引擎重设、与 checkpoint 联合设计**。因此本计划**不再作为先行独立 MVP**，而是 flow v2 的一部分：checkpoint 的序列化/事件/Runner/store 语义与 typed 引擎**一次性协同设计**。下文的 MVP-on-string-map 设计**作为 v2 中 checkpoint 这一面的参考**保留，但以下两轮审核暴露的**致命/必改项**必须在 v2 设计里解决：

**FC-3 [致命，第一轮+codex 双确认] RunID 所有权断裂。** Engine 只持 `CheckpointStore`，调不到 `StartRun`/`newRunID`（私有于 sqlite Store，`runs.go:15,141`）；自 mint 的 RunID 在 `runs` 表无行 → `AppendRunEvent` 报 `ErrNotFound`（`events.go:24`）。token==RunID + 同-run 事件续写**跑不通**。v2 必须重新设计 run 身份所有权（谁 StartRun、Engine 是否接收外部 runID）。

**FC-NEW1 [致命，codex 新发现] `RunStatusSuspended` 打破 `FinishRun`。** `FinishRun` 只更新 `status='running'` 的行（`runs.go:43`）。标 suspended 后，resume 跑完**无法 finish 该 run**。v2 要改 `FinishRun` 接受 suspended→done 转移。

**OQ-A [致命，第一轮+codex] resume 游标漏 fire 同层 sibling 出边。** edge-firing 是 fanout **之后**的独立阶段（`engine.go:306→322`）；中断路径在到达它之前就 snapshot。修复：suspend 前必须 **fire 非中断节点的出边**（中断节点出边推迟到注入 humanInput 后）。

**FC-NEW2 [必改，codex 新发现] sibling 条件边 ERROR 必须在 checkpoint 前 fail。** 边条件求值可返错（`engine.go:340`）。先存 checkpoint 再求值，会把确定性失败变成可恢复挂起。snapshot 前要先把同层 sibling 的条件边求值完。

**FC-NEW3 [必改，codex 新发现] 多中断不能"drain 后才拒"。** drain 整层后 siblings 可能已产生副作用；此时返 `ErrMultipleInterrupts` + 不写 checkpoint = 既没 resume 也没 exactly-once。要么 v2 支持一个 checkpoint 里多个 pending 中断，要么**执行前**就拒绝这种拓扑。

**FC-NEW4 [必改，codex 新发现] resume 必须自校验 humanInput。** `Validate` 只查图形状（`validate.go:24`）。`Resume` 要拿运行时 `e.nodes[id].Outputs()` 校验 humanInput 键。

**FC-NEW5 [必改] flowd 集成非可选。** 事件名/payload 手写映射（`server.go:745,764`）；suspend 事件 + resume token 不更新就不进 SSE/replay。run 历史是本特性的一部分 → flowd 改动纳入 v2。

**✅ 经核实仍成立**：sentinel-error 零改 NodeKind、D5 分层（flow 不 import store）、fanout drain trick 物理可行、`flow.Marshal` 确定性（FlowHash 可靠但字节敏感）。

---

Status (原始): **PROPOSED — MVP scoped to the current string-map engine. Not yet executed.**

> **Cross-plan contract (pinned in `.planning/eino-gap-plans/README.md`):** this MVP covers **only** the current string-map engine. The sibling `flow-graph` plan's typed/`any` data plane + `StreamReader` ports are **not** checkpointable in v1; snapshots are taken at **layer boundaries** (never mid-stream), and a graph that can't serialize → `ErrNotCheckpointable`. This resolves flow-graph open question CP-1.

HITL scenarios in scope for MVP:
- **Approval gate**: a node says "this action needs human sign-off" → suspend → resume with `{"approved":"true"}`.
- **Parameter completion**: a node is missing an argument → suspend → resume with the human-supplied value.
- **Dangerous-op confirmation**: same shape as approval, distinct semantic label.

All three reduce to the same primitive: **one node, at a known layer boundary, requests human input; the engine snapshots and returns; resume reinjects and proceeds.** MVP supports exactly that — a single in-flight interrupt at a time per run (karpathy: smallest thing that delivers the watershed capability; no nested/concurrent-branch interrupt theory yet).

---

## 1. Decided design / end-state

### 1.1 Interrupt mechanism — node-driven `ErrInterrupt` sentinel

A node signals "I need a human" by returning a typed sentinel error from its existing `Run` / `RunWithMetadata`. **No new NodeKind interface method, no new required interface** — this is the only option that is fully backward-compatible with the frozen `NodeKind` contract (`api/v0.1.snapshot.txt` lines 104-107) and with every existing node (`toolNode`, etc.).

New public types in `flow` package:

```go
// InterruptRequest is the payload a node attaches to ErrInterrupt to
// describe what it needs from a human. It is snapshotted verbatim and
// surfaced to the caller so a UI can render the prompt.
type InterruptRequest struct {
    // Kind is a free-form semantic label: "approval", "input",
    // "confirm_dangerous". Engine does not interpret it.
    Kind string `json:"kind,omitempty"`
    // Prompt is the human-readable question/explanation.
    Prompt string `json:"prompt,omitempty"`
    // Schema (optional) is a JSON-schema-ish hint of what humanInput
    // the resume should provide. Advisory only at MVP.
    Schema json.RawMessage `json:"schema,omitempty"`
}

// InterruptError is the typed error a node returns from Run to suspend
// the flow at that node. The engine recognizes it via errors.As.
type InterruptError struct {
    Request InterruptRequest
}

func (e *InterruptError) Error() string { return "flow: node requested human interrupt" }

// Interrupt is the constructor nodes use:
//   return nil, flow.Interrupt(flow.InterruptRequest{Kind:"approval", Prompt:"Approve refund $4000?"})
func Interrupt(req InterruptRequest) error { return &InterruptError{Request: req} }
```

The engine detects it with `errors.As(err, *InterruptError)` at the node-result handling site in `run` (engine.go:291-297), and instead of wrapping it as a `FlowErr`, it routes into the suspend path.

> Rejected alternatives recorded in §2 (Decision D1): a dedicated `InterruptNode` type (b), and a context-injected suspend handler (c).

### 1.2 What the snapshot contains

Because the engine's entire live data plane is `portValues map[string]map[string]string` plus a few boolean/cursor maps (engine.go:202-234), the snapshot is a direct JSON dump of that state. **MVP snapshots only at the interrupt point** (not at every node boundary — §2 D2).

```go
// Checkpoint is the serializable suspension state of a single run.
// v1 is intentionally string-map-only (see §5 cross-plan note).
type Checkpoint struct {
    Version    int    `json:"version"`     // schema version, starts at 1
    RunID      string `json:"run_id"`
    FlowID     string `json:"flow_id"`
    FlowHash   string `json:"flow_hash"`   // sha256 of canonical Flow JSON at suspend time

    // PortValues is the full live data plane: nodeID -> port -> value.
    PortValues map[string]map[string]string `json:"port_values"`
    // Activated mirrors engine `activated` so resume re-derives which
    // nodes will run without recomputing from inputs.
    Activated  map[string]bool `json:"activated"`

    // Cursor identifies HOW FAR execution got. Under layered execution
    // the cursor is "the index of the layer that was executing when the
    // interrupt fired". On resume the engine replays edge-firing for all
    // layers < LayerIndex from the snapshot (cheap, no node re-run) and
    // resumes node execution at LayerIndex.
    LayerIndex int `json:"layer_index"`

    // Interrupt identifies the node that suspended and what it asked for.
    InterruptNodeID string           `json:"interrupt_node_id"`
    InterruptReq    InterruptRequest `json:"interrupt_req"`

    // OriginalInputs is the caller's Run() inputs, retained so resume is
    // fully self-contained from the token alone.
    OriginalInputs map[string]string `json:"original_inputs"`

    CreatedAt time.Time `json:"created_at"`
}
```

Snapshot granularity decision (§2 D2): **checkpoint only at the interrupt point.** The cursor is the **layer index**. On resume, layers `[0, LayerIndex)` are *not re-executed* — their node outputs already live in `PortValues`; the engine only needs to (a) restore `portValues`/`activated`, (b) inject `humanInput` into the interrupted node's output ports, mark it activated-and-done, (c) re-fire that node's outgoing edges, then (d) continue the normal layer loop from `LayerIndex+1` (or finish `LayerIndex`'s remaining nodes — see §1.6 concurrency).

### 1.3 Suspend/Resume API surface

New methods alongside `Run`/`RunStream`. **`Runner` interface is NOT modified** (frozen, snapshot line 115-117) — suspend/resume are *additive* methods on `*Engine` and a new optional `ResumableRunner` interface for wrappers that want to compose over them.

```go
// RunResult is the outcome of a Run that may suspend. Exactly one of
// {Outputs non-nil, Suspended non-nil} is set on success.
type RunResult struct {
    Outputs   map[string]string // set when the run completed normally
    Suspended *Suspension       // set when the run hit an interrupt
}

// Suspension is what the caller gets back when a run pauses. ResumeToken
// is opaque; the caller persists it and passes it back to Resume.
type Suspension struct {
    ResumeToken string           // == checkpoint RunID at MVP
    NodeID      string           // interrupted node
    Request     InterruptRequest // what the human is being asked
}

// RunResumable executes like Run but returns a *Suspension instead of an
// error when a node calls flow.Interrupt. The checkpoint is persisted to
// the configured CheckpointStore before returning.
func (e *Engine) RunResumable(ctx context.Context, inputs map[string]string) (RunResult, error)

// Resume loads the checkpoint named by token, injects humanInput into the
// interrupted node's output ports, and continues to completion (or to the
// NEXT interrupt, returning another Suspension).
//
// humanInput keys are the interrupted node's OUTPUT port names. The node
// "produces" these as if it had returned them from Run.
func (e *Engine) Resume(ctx context.Context, token string, humanInput map[string]string) (RunResult, error)
```

Engine gains an optional checkpoint store, wired via a new option (mirrors `WithConditionEvaluator`):

```go
// WithCheckpointStore enables suspend/resume. Without it, a node that
// returns flow.Interrupt causes RunResumable to fail with ErrNoCheckpointStore
// (and plain Run treats the interrupt as an ordinary error, unchanged).
func WithCheckpointStore(cs CheckpointStore) EngineOption

var ErrNoCheckpointStore = errors.New("flow: interrupt requested but no CheckpointStore configured")
var ErrCheckpointNotFound = errors.New("flow: resume: checkpoint not found")
var ErrFlowChanged = errors.New("flow: resume: flow definition changed since checkpoint")
```

`RunResumableStream` / `ResumeStream` (the event-emitting siblings) are deferred to a fast-follow after the sync path is proven (§4 phase F4).

### 1.4 CheckpointStore interface + sqlite schema

New interface in `flow` (engine concept; **does not** touch the frozen `Store` interface). Implementations expose it as an optional capability detected by type-assert, exactly like the existing `AppendRunEvents` precedent documented at store.go:78-91.

```go
// CheckpointStore persists/loads flow Checkpoints. It is an OPTIONAL
// capability sibling to Store — sqlite.Store implements both, but the
// core Store interface is unchanged for backward compatibility.
type CheckpointStore interface {
    // SaveCheckpoint upserts the checkpoint keyed by RunID. Overwrites an
    // existing checkpoint for the same RunID (a run has at most one live
    // suspension at MVP).
    SaveCheckpoint(ctx context.Context, cp Checkpoint) error
    // LoadCheckpoint returns the checkpoint for token, or ErrNotFound.
    LoadCheckpoint(ctx context.Context, token string) (Checkpoint, error)
    // DeleteCheckpoint removes a checkpoint (called after a successful
    // resume-to-completion so a token can't be replayed).
    DeleteCheckpoint(ctx context.Context, token string) error
    // ListCheckpoints returns live (non-expired) checkpoints for a flow.
    ListCheckpoints(ctx context.Context, flowID string, limit int) ([]CheckpointMeta, error)
}
```

New sqlite table (added to `ensureSchema` DDL, open.go:77 — additive `CREATE TABLE IF NOT EXISTS`, zero migration risk on existing DBs):

```sql
CREATE TABLE IF NOT EXISTS checkpoints (
  run_id            TEXT PRIMARY KEY,        -- == resume token
  flow_id           TEXT NOT NULL,
  flow_hash         TEXT NOT NULL,           -- sha256 of canonical Flow JSON
  version           INTEGER NOT NULL,
  layer_index       INTEGER NOT NULL,
  interrupt_node    TEXT NOT NULL,
  interrupt_req_json TEXT,                   -- InterruptRequest
  snapshot_json     TEXT NOT NULL,           -- full Checkpoint payload
  created_at        INTEGER NOT NULL,
  expires_at        INTEGER                  -- NULL = no TTL (see §2 D7)
);

CREATE INDEX IF NOT EXISTS idx_checkpoints_flow_id_created
  ON checkpoints(flow_id, created_at DESC);
```

Storing the full `Checkpoint` as one `snapshot_json` blob plus a few promoted columns (for `ListCheckpoints` / TTL sweeps) mirrors the existing `runs.inputs_json` / `outputs_json` pattern (runs.go:24-31). The promoted columns are query handles; the blob is the source of truth.

`runs.status` gains a new value `"suspended"` so run history correctly reflects a paused run:

```go
const RunStatusSuspended RunStatus = "suspended"   // added to store.go:29-33
```

### 1.5 New flow events

Two additive `FlowEventKind` constants (appended at the END of the iota block so existing numeric values don't shift — `kind` is persisted as the *string* name via `store.RunEventKind`, so numeric stability matters only for in-process consumers; append anyway for safety):

```go
// event.go — appended after FlowErr
NodeInterrupted   // a node returned flow.Interrupt; Request populated
FlowSuspended     // terminal-for-this-run: snapshot saved; ResumeToken set
```

`FlowEvent` gains two optional fields (additive, nil for all existing kinds):

```go
Request     *InterruptRequest // set on NodeInterrupted
ResumeToken string            // set on FlowSuspended
```

Persisted-event mirror in `flow/store` (store.go:58-65): add `RunEventNodeInterrupted RunEventKind = "node_interrupted"` and `RunEventFlowSuspended RunEventKind = "flow_suspended"`. Resume **continues the same run**: it appends to the existing run's event history (the store's `AppendRunEvent` is explicitly safe on a non-running run, store.go:135-138).

### 1.6 Concurrency / cancel semantics at the interrupt point

Layered parallel execution (engine.go:250-320, `fanout.Run` with `WithFailFast`) means when one node in a layer returns `Interrupt`, sibling nodes in the same layer may still be running. MVP rule (§2 D6):

- **Drain the current layer, then suspend.** When a node returns `*InterruptError`, the task does **not** return an error to fanout (which would fail-fast-cancel siblings); instead it records the interrupt in a layer-local collector and returns `(nil, nil)` so fanout lets siblings finish naturally. After `fanout.Run` returns, the engine checks the collector: if any interrupt fired, it snapshots **after all siblings completed** (their outputs are already in `portValues`), sets the cursor to the current `LayerIndex`, persists, and returns `Suspended`.
- The interrupted node itself produced **no output** — on resume the human supplies it. Its outgoing edges are NOT fired pre-suspend; they fire on resume once `humanInput` populates its ports.
- **Multiple interrupts in one layer**: MVP rejects this as unsupported — if the collector has >1 entry, return `ErrMultipleInterrupts` (don't silently pick one). This keeps the "single in-flight interrupt" invariant honest. (Recorded as an explicit limitation, §5.)
- **External ctx cancel during a suspended state**: irrelevant — suspension already returned; the run is durably checkpointed independent of the original ctx.

### 1.7 End-to-end example (approval gate)

```go
reg := flow.NewNodeRegistry()
_ = flow.RegisterToolNode(reg)
reg.Register("approval", approvalNodeFactory) // returns flow.Interrupt(...)

cs, _ := sqlite.Open("flows.db")              // implements flow.CheckpointStore
eng, _ := flow.Compile(f, reg, deps, flow.WithCheckpointStore(cs))

// 1. First leg: runs until the approval node interrupts.
res, err := eng.RunResumable(ctx, map[string]string{"amount": "4000"})
// err == nil; res.Suspended != nil
//   res.Suspended.ResumeToken == "a1b2c3..."
//   res.Suspended.NodeID      == "approve_refund"
//   res.Suspended.Request     == {Kind:"approval", Prompt:"Approve refund $4000?"}

// ... hours later, a human clicks "Approve" in a UI ...

// 2. Resume: inject the decision into the approval node's "output" port.
res2, err := eng.Resume(ctx, "a1b2c3...", map[string]string{"output": "approved"})
// res2.Outputs == {"result": "refund issued"}  — identical to a non-interrupted run
```

**Verification invariant** (drives the MVP test): for a flow where the approval node, if it had returned `{"output":"approved"}` directly, would produce outputs `X`, a `RunResumable`→`Resume("approved")` round-trip must produce **exactly `X`**. Checkpoint round-trip is behavior-transparent.

---

## 2. Key decisions (options · recommendation · rationale)

**D1 — Interrupt trigger mechanism.**
- (a) **Node returns `*InterruptError` sentinel** from existing `Run`. Engine detects via `errors.As`.
- (b) Dedicated `InterruptNode` registered node type that always suspends.
- (c) Context-injected suspend handler: `flow.SuspendFromContext(ctx).Interrupt(req)` blocks/signals.
- **Recommendation: (a).** It requires **zero change to the frozen `NodeKind` interface** (snapshot 104-107) — every existing node keeps compiling untouched, and *any* node can opt into HITL by returning the sentinel (an LLM node can interrupt for clarification, a tool node for confirmation). (b) is too rigid (an approval is rarely a standalone no-op node; it's usually "this tool call needs sign-off"), though we'll *also* ship a thin bundled `InterruptNode` built on (a) as a convenience for the pure-approval case. (c) fights the synchronous `Run(ctx,in)(out,err)` model — it implies the node goroutine parks, complicating the fanout/drain story. Rationale: (a) is the minimal, composable, backward-compatible primitive; (b) is sugar over it, (c) is over-engineering.

**D2 — Snapshot granularity.**
- (a) Checkpoint at **every node/layer boundary** (full replay-debugging, costly).
- (b) Checkpoint **only at interrupt points**.
- **Recommendation: (b).** karpathy/MVP: the watershed capability is HITL pause-resume, not time-travel debugging. Per-boundary checkpointing multiplies write volume and forces resolving "re-run side-effecting nodes vs. trust cached output" — out of scope. The cursor is the **layer index**; completed layers live in `PortValues` and are never re-run. Per-boundary checkpointing is a clean future extension (the schema already supports it — just call `SaveCheckpoint` more often).

**D3 — State serialization (and the flow-graph collision).**
- Current data plane is `map[string]map[string]string` → **trivially `encoding/json`-serializable.** v1 uses plain JSON, no codec registry.
- **Recommendation: JSON for v1, gated by a serializability contract.** `Compile` records whether the flow is "checkpoint-eligible": MVP = always true (string-map engine). For the future typed/`any` data plane from the sibling **flow-graph** plan, JSON breaks (interface values lose their concrete type on decode) and in-flight `StreamReader`s are not serializable at all. Strategy: (i) v1 checkpoints **only** the string-map engine; (ii) define `ErrNotCheckpointable` for graphs the future engine marks unserializable; (iii) for typed state, prefer **"node outputs must be JSON-marshalable"** as a published constraint over gob+type-registry (gob needs `gob.Register` per concrete type — brittle across module boundaries, the same gotcha phase-a hit with cross-module internals). Concrete cross-plan ask in §5.

**D4 — Resume token form & human-input injection.**
- Token = the **checkpoint RunID** (16-char hex, reuses `newRunID`, runs.go:144). Opaque to callers; doubles as the run-history key so a suspended run is greppable in `runs`.
- `humanInput map[string]string` keyed by the **interrupted node's output port names** — the human "produces" what the node would have produced. The engine writes these into `portValues[interruptNode]` then fires that node's outgoing edges. Symmetric with how a normal node's `out` map populates ports (engine.go:298-300).
- **Recommendation: as above.** Reusing RunID keeps one identity for the whole HITL lifecycle (start → suspend → resume → done) — `GetRun(token)` works throughout. Keying humanInput by output ports (not a magic "value" key) means multi-output interrupt nodes are expressible without API churn.

**D5 — Where `Checkpoint`/`CheckpointStore` live.**
- (a) Both in `flow/store` (store owns persisted shapes).
- (b) `Checkpoint` + `CheckpointStore` interface in `flow`; `sqlite.Store` implements `flow.CheckpointStore`.
- **Recommendation: (b).** The `flow` library package today does **not** import `flow/store` (verified: only `cmd/flowd` wires them together). Putting the interface in `flow` keeps that clean layering — `sqlite` already imports `flow/store`; it adds an import of `flow` to satisfy `flow.CheckpointStore`, which is acyclic (`flow` does not import `sqlite`). This mirrors the existing `Runner`/`Store` split.

**D6 — Concurrency at interrupt (same-layer siblings).**
- (a) Cancel siblings immediately (fail-fast style).
- (b) **Drain the layer, then suspend.**
- **Recommendation: (b)** — see §1.6. Cancelling siblings would lose their work (they'd re-run on resume, breaking the "no re-run of completed nodes" and "side-effects once" guarantees). Draining is also simpler against `fanout.WithFailFast`: the interrupt task returns `(nil,nil)`, siblings complete, the engine inspects a post-layer collector. **Limitation locked:** >1 interrupt in a single layer → `ErrMultipleInterrupts` (unsupported at MVP, not silently resolved).

**D7 — TTL / flow-version validation.**
- **Flow version:** snapshot stores `FlowHash = sha256(canonical Flow JSON)`. `Resume` recomputes the hash of the engine's compiled flow; mismatch → `ErrFlowChanged` (don't resume into a structurally different graph — node IDs/edges may have moved). This is cheap and catches the dangerous case. Marshal via existing `flow.Marshal` (ir.go:99) for canonical bytes.
- **TTL:** `expires_at` column, nullable. MVP default = no expiry (NULL); `WithCheckpointTTL(d)` option sets it. A `DeleteExpired(ctx)` sweep method on the sqlite store, called opportunistically by `LoadCheckpoint` (lazy expiry) — no background goroutine in v1 (keeps the library side-effect-free; flowd can schedule a sweep).
- **Recommendation: ship hash-validation in MVP, TTL column in MVP but enforcement lazy + opt-in.** Hash-validation is a correctness guard (cheap, must-have). TTL is an ops concern — wire the column now so no migration later, but don't add background machinery.

---

## 3. Backward-compatibility & API-snapshot impact

Everything is **additive**. The frozen surfaces stay byte-identical:
- `NodeKind` interface: unchanged. Existing nodes need no edits.
- `Runner` interface: unchanged. `Run`/`RunStream` behavior unchanged — a node returning `*InterruptError` under plain `Run` (no `RunResumable`) surfaces as an ordinary wrapped `FlowErr` (acceptable: callers who never opted into checkpointing see an error, not a silent hang).
- `Store` interface: unchanged. `CheckpointStore` is a separate optional interface, detected by type-assert (the `AppendRunEvents` precedent, store.go:78-91).
- `api/v0.1.snapshot.txt`: **grows** (new `Compile` option, `RunResumable`/`Resume` methods, `RunResult`/`Suspension`/`Checkpoint`/`InterruptRequest`/`InterruptError` types, `CheckpointStore` interface, new event kinds/fields, new sqlite methods, `RunStatusSuspended`). Net-new lines only; no removals/renames → semver **minor** bump.

---

## 4. Phased implementation plan (each task carries a verification gate)

All `go` commands run in the `llm-agent-flow` module. Branch from `main` (`feat/flow-checkpoint`). Go 1.26.

### F0 — Interrupt primitive (pure types, no engine wiring)
1. Add `InterruptRequest`, `InterruptError`, `Interrupt()` constructor, `ErrNoCheckpointStore`/`ErrCheckpointNotFound`/`ErrFlowChanged` to a new `flow/interrupt.go`.
2. Add `Checkpoint`, `RunResult`, `Suspension`, `CheckpointStore` interface, `CheckpointMeta` to `flow/checkpoint.go`.
- **Verify:** `go build ./...` green; `go test ./...` unchanged (no behavior touched yet). Snapshot test (`internal/apisnapshot`) will flag the new exports — regenerate snapshot, confirm only additions.

### F1 — Engine suspend path (in-memory store first)
1. Add `WithCheckpointStore` option + `checkpoints CheckpointStore` field to `Engine`/`engineConfig` (engine.go:42-61, 143-151).
2. In `run`'s node task (engine.go:271-303), detect `errors.As(err, *InterruptError)`: record into a layer-local interrupt collector instead of returning the error; return `(nil,nil)`.
3. After `fanout.Run` (engine.go:306), inspect the collector: 0 → proceed; 1 → build `Checkpoint` from `portValues`+`activated`+`LayerIndex`, save via store, emit `NodeInterrupted`+`FlowSuspended`, return `RunResult{Suspended:...}`; >1 → `ErrMultipleInterrupts`.
4. Add `RunResumable` wrapping `run` with a flag that enables the suspend path (plain `Run` keeps treating interrupt as error).
5. Provide an in-memory `CheckpointStore` (`flow.MemCheckpointStore`) for tests.
- **Verify (TDD — write the test first, then make it pass):** a 3-node chain `A→approve→C` where `approve` returns `flow.Interrupt`. `RunResumable` returns `Suspended{NodeID:"approve"}`, a non-nil token, and **does not** run `C`. Assert `portValues[A]` is in the saved checkpoint and `C` has no output. Assert plain `Run` on the same flow returns a `FlowErr`.

### F2 — Engine resume path
1. Add `Resume(ctx, token, humanInput)`: load checkpoint, validate `FlowHash`, restore `portValues`/`activated`, inject `humanInput` into `portValues[InterruptNodeID]`, mark it activated+done, fire its outgoing edges, then run the layer loop from `LayerIndex+1` (the interrupted node's layer is already drained) to completion. On clean completion: `DeleteCheckpoint`, return `RunResult{Outputs}`. On a second interrupt: save a new checkpoint, return `Suspended` again.
2. `FlowHash` mismatch → `ErrFlowChanged`; missing token → `ErrCheckpointNotFound`.
- **Verify (the headline invariant):** for a flow whose `approve` node, if it returned `{"output":"approved"}` directly, yields outputs `X`, assert `RunResumable → Resume(token,{"output":"approved"})` yields **exactly `X`**. Add a negative test: `Resume` after mutating one node's config (different `FlowHash`) → `ErrFlowChanged`. Add an idempotency test: second `Resume` with the same token after success → `ErrCheckpointNotFound` (token consumed).

### F3 — sqlite CheckpointStore
1. Extend `ensureSchema` DDL (open.go:77) with the `checkpoints` table + index (additive `IF NOT EXISTS`).
2. New `flow/store/sqlite/checkpoints.go`: `SaveCheckpoint` (upsert), `LoadCheckpoint`, `DeleteCheckpoint`, `ListCheckpoints`, `DeleteExpired`. Mirror the JSON-blob+promoted-columns pattern of `runs.go`.
3. Make `sqlite.Store` satisfy `flow.CheckpointStore` (compile-time assert `var _ flow.CheckpointStore = (*Store)(nil)`).
4. Add `RunStatusSuspended` to `flow/store`; have flowd transition `runs.status` to `suspended` on `FlowSuspended` and back via resume.
- **Verify:** run the F1+F2 suite against `sqlite.Open(":memory:")` instead of `MemCheckpointStore` — identical pass. Add a WAL/persistence test (mirror `wal_test.go`): save checkpoint, `Close`, reopen DB, `LoadCheckpoint` returns the snapshot; resume to completion against the reopened store. Add a `DeleteExpired` test with a past `expires_at`.

### F4 — Streaming + flowd HTTP surface (fast-follow, optional for MVP merge)
1. `RunResumableStream` / `ResumeStream` emitting `NodeInterrupted`/`FlowSuspended` and appending to the run's persisted event history (`AppendRunEvent`, safe on non-running runs).
2. `cmd/flowd/server`: add `POST /runs/{id}/resume` (body = humanInput map) and surface `Suspended` in the run-start response. Wire the new `RunStatusSuspended` into the runs listing.
3. Bundled convenience `InterruptNode` type (`flow.RegisterInterruptNode`) for the pure-approval case — thin wrapper that returns `flow.Interrupt` with config-supplied prompt.
- **Verify:** an SSE replay test (mirror `server_replay_test.go`) showing a run that emits `flow_suspended`, then a `POST .../resume` that drives it to `flow_done`, with the persisted event history containing the full start→suspend→resume→done sequence in seq order.

### F5 — Docs, snapshot, release
1. Regenerate `api/v0.1.snapshot.txt`; confirm additions-only.
2. `docs/architecture.md` + `README.md`: HITL section with the §1.7 example.
3. `flow/doc.go`: document the suspend/resume contract and the **string-map-only** limitation.
- **Verify:** `go vet ./... && go test ./... -count=1` green; snapshot test green; doc example compiles as an `example_test.go`.

---

## 5. Risks · open questions · cross-plan interaction

**Cross-plan: flow-graph serialization collision (the central risk).** The sibling **flow-graph** plan (typed/generic `Graph` with `any` ports + `StreamReader`) directly invalidates this plan's JSON-snapshot assumption:
- This plan's v1 checkpoint **only covers the current string-map engine.** Explicit limitation in `doc.go`.
- **Streaming nodes cannot be checkpointed mid-flight.** A `StreamReader` in flight has no serializable form; checkpointing it would require draining it first (losing streaming) or persisting an unbounded buffer. **MVP rule to publish: a flow containing an in-flight stream at an interrupt point is not checkpointable → `ErrNotCheckpointable`.** Interrupts must occur at fully-materialized (non-streaming) boundaries — which aligns with flow-graph's recommendation to snapshot at **layer boundaries** (CP-1, resolved).
- **Constraint to hand to the flow-graph plan:** typed node state should be **JSON-marshalable by contract** (or carry an explicit `Snapshot()/Restore()` codec), and the graph should expose a compile-time `Checkpointable() bool`. Avoid gob+global-type-registry (cross-module `gob.Register` brittleness — the same class of cross-module-internal gotcha phase-a documented). This plan's `Checkpoint.Version` field is the forward-compat seam for a v2 codec.

**Open questions (flagged, not silently decided):**
- **OQ1 — `Checkpoint`/`CheckpointStore` placement (D5).** Recommended (b) `flow` package, but if a future `llm-agent-flow-contract` leaf emerges (mirroring the contract-extraction pattern), these are contract-worthy. Decide whether to pre-place them in a leaf. *Leaning: keep in `flow` now, revisit on extraction.*
- **OQ2 — humanInput validation.** MVP injects `humanInput` into output ports with no schema enforcement (the `InterruptRequest.Schema` is advisory). Should `Resume` validate humanInput against declared output ports / schema and reject unknown keys? *Leaning: validate against the node's declared `Outputs()` port names (reject unknown), defer JSON-schema validation.*
- **OQ3 — side-effecting interrupt nodes on resume.** The interrupted node does NOT re-run on resume (the human supplies its output). But what if a node does work *then* interrupts (partial side-effect)? MVP contract: **a node that interrupts must do so before any side-effect** (interrupt-first). Document as a node-author rule; can't enforce.
- **OQ4 — multiple concurrent interrupts.** Locked as unsupported (`ErrMultipleInterrupts`, D6). Is single-interrupt-per-layer too restrictive for real approval-fan-out flows? *Defer; revisit only with a concrete consumer pull (杜绝臆测性设计).*
- **OQ5 — flow-hash strictness.** `ErrFlowChanged` on any hash mismatch is conservative; a node-config tweak unrelated to the suspended subgraph also blocks resume. Acceptable for MVP; a structural-diff (only reject if the interrupted node's neighborhood changed) is a future refinement.

**Risks:**
- `fanout.WithFailFast` + the "return `(nil,nil)` to drain" trick must not mask a *real* sibling error that happens concurrently with an interrupt. Mitigation: if any sibling returns a genuine error, that wins (fail the run); the interrupt collector is only honored when the layer otherwise succeeded. Cover with a test (sibling errors + sibling interrupts in same layer → run fails, no checkpoint written).
- Snapshot/restore drift if engine internals (`activated`, layer derivation) change later. Mitigation: the round-trip invariant test (F2) is the regression guard.

---

## 6. Version & release impact

- **`llm-agent-flow`**: **minor** version bump (additive API). Update `api/v0.1.snapshot.txt` (additions-only — gate it through the existing `internal/apisnapshot` test). New optional `CheckpointStore` capability + new sqlite table (`CREATE TABLE IF NOT EXISTS` → no migration for existing DBs).
- **Backward compatibility**: existing flows, nodes, `Run`/`RunStream`, `Runner`, `Store` all unchanged. A flow that never uses `flow.Interrupt` behaves identically. Wrappers (`otelflow.Wrap` over `Runner`) keep working — they wrap `Run`/`RunStream`; a future `ResumableRunner` interface lets them opt into wrapping `RunResumable`/`Resume`.
- **No cross-repo cascade**: the change is internal to `llm-agent-flow` (no contract change, no sibling repin) — unlike the extraction plans. `cmd/flowd` is in-repo and updated in F4.
- **Sequencing vs flow-graph**: ship this MVP **before** the flow-graph typed-state migration, then treat checkpoint as a constraint input to that plan (§5). Document the `Checkpoint.Version` seam so the typed-state codec lands as a v2 without breaking v1 snapshots.

---

### Critical Files for Implementation
- `llm-agent-flow/flow/engine.go`
- `llm-agent-flow/flow/event.go`
- `llm-agent-flow/flow/store/store.go`
- `llm-agent-flow/flow/store/sqlite/open.go`
- `llm-agent-flow/flow/store/sqlite/runs.go`
