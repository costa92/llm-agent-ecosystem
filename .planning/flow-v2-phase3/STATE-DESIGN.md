# flow-v2 Phase 3 Item #4 — graph State (`flow-graph-state`) — Co-Design

Status: **IMPLEMENTED (2026-06-05) — MVP tasks 1-6 shipped on branch `feat/v2-engine-core` (commits df3c018, 4535b83, 26e4344, 5b0c645, d6344c3, 35807e8).** All round-2 corrections applied + independently verified (BLOCKER-1 reject-on-collision + WithReducer; BLOCKER-2 nodeID-wrap in engine closure + mu-guarded reduce; MAJOR-3 post-reduce snapshot; MAJOR-4 fingerprinted/namespaced StateCodec, structHash fold confirmed moot & omitted; MAJOR-5 reduce pinned after error loop; MINOR-6 no streaming-path State). Design history (round 1 Plan agent + round 2 adversarial) retained below. Governing principle (reused from #1): State must NOT regress byte-identical fresh `Invoke`, the resume cursor, or sibling-error-wins — verified.

## Round 2 (adversarial review) — REVISED RESOLUTIONS (supersede the table below where they conflict)
Round 2 caught two blockers + a silent-corruption hole before any code. VERIFIED sound: byte-determinism (narrowly), no mid-layer write leak (given snapshot-once), single-goroutine streaming executors (zero fanout/go in runLinearStream + runStreamDAG).
- **BLOCKER-1 — default reducer MUST be reject-on-collision, NOT last-wins.** Last-in-nodeID-sorted-order is deterministic but SILENTLY CLOBBERS a sibling's write (the T-DET byte-identity test passes while the feature is broken — the marquee "running summary across nodes" case). **→ default: if >1 node in a layer staged a write and no `WithReducer` was supplied, return `ErrStateWriteConflict`. Ship `WithReducer[S]` in MVP (move from deferred) as the explicit merge escape hatch.**
- **BLOCKER-2 — the per-node nodeID-in-context threading is WRONG against `fanout.Run`.** fanout passes ALL siblings ONE shared `runCtx` (fanout.go:78/147), so you can't read a per-node id from a fanout-populated ctx. **→ the wrap must happen INSIDE the engine task closure (engine.go:447, which already rebinds `nodeID := nodeID` at :438): `nodeCtx := withStateNode(taskCtx, cell, nodeID); node.Run(nodeCtx, in)`.** Also: the barrier reduce MUST acquire `stateCell.mu` (a node that leaks a goroutine outliving Run could write `staged` after join → race). Document: SetState is honored only synchronously before Run returns; goroutine-outliving-Run writes are unsupported.
- **MAJOR-3 — resume does NOT re-run the suspend layer** (`startLayer = SuspendLayer+1`). So "reduce per-layer identical fresh vs resume" is imprecise; the real requirement: **the snapshot written into `Checkpoint.State` is the POST-reduce committed State of the suspend layer** (reduce before snapshotCheckpoint). Limitation to document: an interrupt node returned `(nil,nil)` (errored out of Run) so it CANNOT stage a State write in its suspending leg; human input is injected as port values, not State. HITL+State authors must write State in a non-interrupting node.
- **MAJOR-4 — `StateCodec` tag-compare is insufficient** (codec registry is first-write-wins; a same-tag/different-shape `S` swap passes the name check then `json.Unmarshal`s silently → corruption). **→ (a) add a structural fingerprint of S to the stored tag (tag + sha256 of field schema); (b) fold S into `structHash` on the TYPED route where S is build-known (O2 = BOTH); (c) namespace State codec tags (`state:` prefix) so they never alias the port `string` codec.** For the pure-engine route where S is a run option, document that tag-stability-across-shape-change is the caller's responsibility and the failure is best-effort (can't fully self-protect).
- **MAJOR-5 — pin the reduce ordering**: place it AFTER the `fanout.Run` error check AND the results-error loop (after engine.go:491, both checks passed), BEFORE the `if checkpoint` suspend block (:496). So State commits on suspend (snapshot needs it) but NEVER on an erroring layer (sibling-error-wins preserved).
- **MINOR-6 — TRIM streaming-path State from MVP.** Don't thread State into runLinearStream/runStreamDAG (zero durability, and the same node behaves differently stream vs degrade-to-Invoke → silent-loss footgun). Remove resolution #6's "streaming nodes MAY read/write State"; MVP = no State access on the stream path.
- **Note on task 6**: `AddStatefulLambdaNode` is a closure (markInProcess) → graph non-serializable → its State+checkpoint path is NOT testable by construction; State+checkpoint is testable ONLY on the engine/JSON route (task 4 T-RESUME). State the impossibility so the implementer doesn't write an impossible test.
- **O-answers**: O1 moot; O2 BOTH (field + structHash-on-typed + fingerprint); O3 seed-fresh+warn (ignore restored State if new run supplies none, don't crash); O4 reject-on-collision; O5 no State on stream path in MVP.

**Revised MVP scope**: tasks 1-6 with the corrections above — default reject-on-collision + WithReducer shipped; nodeID-wrap in the engine closure + mu-guarded reduce; post-reduce snapshot; fingerprinted/namespaced StateCodec + typed-route structHash fold; reduce pinned after engine.go:491; NO streaming-path State. Ready to implement after the user ratifies.

---
### (round 1) original resolutions follow — read with the round-2 revisions above taking precedence

## Decisive resolutions
| # | Question | Resolution |
|---|---|---|
| 1 | Concurrent writes under fanout | **Read-only committed snapshot during a layer; per-node writes BUFFERED; applied at the layer barrier via a deterministic nodeID-SORTED reduce.** Reject mutex/last-writer (nondeterministic). |
| 2 | Threading | **State carried in `context.Context`** via typed helpers `StateFromContext[S]`/`SetState[S]` (optional-capability precedent; no breaking `Run` sig change). |
| 3 | Checkpoint | New `Checkpoint.State json.RawMessage` via the existing `codec.go` tagged-JSON (`RegisterCodec[S]` required for durability). Version→3; State-less (v1/v2) checkpoints restore empty (back-compat). |
| 4 | Struct-hash | A State-type change between suspend/resume → `ErrFlowChanged`. Compile doesn't know S (run option), so guard via a new `Checkpoint.StateCodec` tag field compared at resume (dual-fold into structHash only on the typed route where S is build-known). |
| 5 | Serializability route | State available on ALL graphs at runtime; checkpointable only when graph serializable AND S has a codec (mirrors `Checkpointable()==Serializable()` + runtime `ErrNotCheckpointable`). |
| 6 | State + streaming | Stream-path run is non-checkpointable (precedent); streaming nodes MAY read/write State (best-effort, non-durable, deterministic because the stream executor is single-goroutine per route). |
| 7 | Typed vs JSON front-end | MVP = engine route + a typed `AddStatefulLambdaNode[...,S]`. NO `Graph[I,O,S]` third param (deferred). |

**MVP boundary:** one whole-graph JSON-serializable State `S`, context helpers, per-node staged write reduced at each layer barrier by a single declared reducer `func(prev S, writes []S) S` (default = last-in-nodeID-sorted-order), snapshotted into checkpoint + restored on resume. Defer: per-key reducers, `Graph[I,O,S]`, State migration, cross-run store.

## Concurrency contract (the crux) — determinism argument
- `runCore` runs a layer's nodes CONCURRENTLY (`fanout.Run`); completion order is nondeterministic.
- State is read as a COPY of the committed snapshot (no node sees a sibling's write mid-layer). Each node stages ≤1 write keyed by nodeID (last `SetState` wins per node).
- After `fanout.Run` joins AND after the error early-returns, on the single engine goroutine, BEFORE edge-firing: `committed' = reduce(committed, writes_sorted_by_nodeID)`. Default reducer = last-in-sorted-order.
- **Determinism**: the reduce input is a pure function of *which nodes wrote what* (nodeID-sorted), never of goroutine finish order. So byte-identical fresh `Invoke` is preserved. The only mutation point is the single-threaded sorted reduce at the barrier (same place `fireLayerEdges`/`snapshotCheckpoint` run with no live node goroutines).
- State unused → `hasState=false`, entire path inert (zero cost/behavior change for existing flows).

## Suspend-splice ordering
join → **reduce State [NEW, every layer]** → (checkpoint path) collect interrupts → `fireLayerEdges` → `snapshotCheckpoint`(reads committed State) → save. Error early-return happens BEFORE reduce → an erroring layer commits no State (sibling-error-wins preserved).

## Invariant protection
- I1 byte-identical Invoke: determinism argument above; State-less flows inert.
- I2 resume cursor: State is restored data, not a cursor input; reduce per-layer identical fresh vs resume; StateCodec guard prevents type-swap corruption.
- I3 sibling-error-wins: reduce runs after the error early-return; erroring layer discards staged writes.
- Streaming path: single-goroutine per route → deterministic; non-checkpointable → State never snapshotted there (best-effort, documented).

## Sub-tasks (TDD, MVP = tasks 1-6)
1. **State carrier + context helpers** (`state.go`): `stateCell`, `StateOption`, `WithInitialState[S]`/`WithReducer[S]`, `StateFromContext[S]`, `SetState[S]`, `defaultReduce`. Tests: no-op without cell; last-write-per-node; nodeID-sorted reduce deterministic; copy-on-read.
2. **Engine threading + per-layer reduce** (`engine.go runCore`): wrap taskCtx with nodeID+cell; commit reduce after join, before suspend splice + fireLayerEdges. Tests: **T-DET** (2 sibling writes, run 200×, byte-identical), **T-ERR** (erroring sibling → no commit), single-node write visible next layer.
3. **`RunWithState`/`RunResumableWithState`/`ResumeWithState`** entry points (additive; bare Run unchanged). Regression: existing conformance byte-identical. apisnapshot regen.
4. **Checkpoint snapshot+restore + Version 3** (`checkpoint.go`/`engine.go`/`resume.go`): `State`/`StateCodec` fields; encode via codec (missing → ErrNotCheckpointable); resumeSeed restore. Tests: **T-RESUME** (State written pre-suspend observable post-resume), v2 back-compat, missing-codec ErrNotCheckpointable, clone safe.
5. **State-type-change guard on resume** (`StateCodec` compare → ErrFlowChanged). Test: S=V1 suspend, S=V2 resume → ErrFlowChanged.
6. **Typed `AddStatefulLambdaNode[GI,GO,In,Out,S]`** + `InvokeWithState` (`graph.go`/`runnable.go`): adapter reads StateFromContext, calls fn(ctx,in,s), may SetState; markInProcess (non-checkpointable, documented). Test: running-summary across 3 nodes.

## Open questions for round 2
- O1: (moot) planning docs are in the umbrella repo, not llm-agent-flow — Plan agent derived from code (authoritative); precedent correctly reconstructed.
- O2: dual struct-hash + checkpoint-StateCodec guard vs checkpoint-field-only (simpler, slightly later rejection)?
- O3: resume a State-less v2 checkpoint where caller now supplies WithInitialState — reject / seed-fresh / ignore? (lean seed-fresh + warn).
- O4: default reducer = last-by-nodeID vs reject-if->1-sibling-wrote-in-a-layer (safer vs noisier)? (lean last-wins for ergonomics — but reconsider: silent clobber risk).
- O5: allow SetState on the stream path (best-effort) vs hard-error (footgun prevention)? (lean allow+document).

## Deferred
Per-key reducers; `Graph[I,O,S]` third param; checkpointable State on lambda/typed route (non-serializable); streaming-path State durability; State schema migration; cross-run/shared State store.
