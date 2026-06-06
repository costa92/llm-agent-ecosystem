# flow-v2 Phase 3 — Implementation Plan

Status: **PLAN — Phases 1+2 DONE. Phase 3 scoped/sequenced 2026-06-05.** Source: `.planning/flow-v2/PLAN.md` §3 "Phase 3 (deferred)" + Phase-2 deferrals. Plan-agent investigated against live code (v2 green).

Constraints: GOWORK=off; LSP false-positives on v2 nested module (trust real build/test); v2 apisnapshot = own harness → `v2/api/v2.0.snapshot.txt` (never touch v0.1); root+otel commits need `--no-verify` until v2 tagged. TDD throughout.

## Assessment + verdict
| # | Item | Size | Risk | Dep | Verdict |
|---|---|---|---|---|---|
| 7 | AddInterruptibleLambdaNode (typed HITL) | S | Low | — | **Do now (1st)** |
| 6 | Precise neighborhood flow-hash diff | M | Med | — | **Do now (2nd)** |
| 1 | Real-time streaming through Branch/Parallel (partial-barrier scheduler) | XL | **Very high** | — | **STOP — own co-design + 2-round review; NOT a single task** |
| 4 | graph State (flow-graph-state) | XL | High | — | **STOP — own co-design + 2-round review** |
| 2 | in-graph merge/copy NODES | L | High | #1 | Gated on #1 |
| 3 | ordered/zip merge | M | Med | #1,#2 | Gated on #1 |
| 5 | multi-pending-interrupt (OQ-3) | L | High | — | **Defer** (cursor complexity vs speculative value; spec leans defer) |

**Key architectural fact**: streaming and checkpoint already live on DISJOINT code paths — `Runnable.Stream`→`runLinearStream` BYPASSES the engine; `Invoke`/`RunResumable`/`Resume`→`runCore` (barrier). The spec decided you cannot suspend mid-stream. So #1 must be built as a SEPARATE streaming-only DAG executor that NEVER touches `runCore`/`fireLayerEdges`/`snapshotCheckpoint` and is non-checkpointable — protecting the resume cursor + fresh-run + sibling-error-wins invariants by construction.

## Accepted decisions (Plan-agent recommendations)
- #7 ships the interrupt CAPABILITY; a typed-lambda HITL graph is non-serializable (closure) → `Checkpointable()==false`, suspend → `ErrNotCheckpointable` unless port values are codec-serializable. That honest signal is the intended semantic (matches spec §2-② note). Document it.
- #7 scope = node constructor only (one new exported symbol). Do NOT add typed `Runnable.RunResumable`/`Resume` now (clean separate follow-up). Test capability white-box via `run.engine` (tests are `package graph`).

## #1 decomposition (for the eventual co-design — NOT this milestone)
Add a streaming DAG executor ALONGSIDE the barrier scheduler, reachable only via `Stream` on `StreamCapable()` graphs, explicitly non-checkpointable (mirrors the existing `runLinearStream`-bypasses-engine pattern). Sub-tasks: 1a streaming DAG executor skeleton (per-node goroutine, per-edge streamCarrier, no checkpoint) → 1b fan-out tee (reuse Phase-2 `Copy`) → 1c fan-in (reuse `Merge`) → 1d widen `StreamCapable()`/`buildLinearPipeline` without regressing the linear fast path. **Open Qs before code**: (a) streaming + checkpoint? → recommend NO (stream-only, non-checkpointable); (b) accept all DAGs or DAGs-minus-conditional-edges? (conditional edges need a materialized string → lean reject→degrade); (c) mid-graph tee backpressure policy; (d) goroutine lifecycle on early-Close/error (no-leak proofs on every shape).

## #4 graph State skeleton (for the eventual co-design — NOT this milestone)
JSON-serializable-by-construction shared State threaded through the graph, snapshotted into Checkpoint. **Open Qs before code**: (a) concurrent writes under layer fanout — merge/conflict contract (portValues avoids this; State is shared mutable); (b) does State schema participate in #6's region/structural hash? (lean yes); (c) serializability — State only on the JSON/serializable route? (d) State + streaming = non-checkpointable (same rule).

---

## Item #7 — AddInterruptibleLambdaNode (DELIVER FIRST)
Typed lambda that may return `flow.Interrupt(...)` and is `InterruptCapable`. Engine already detects the sentinel + checks `node.(InterruptCapable)` (an ordinary lambdaAdapter is NOT capable → `ErrUninstrumentedInterrupt`).
- **File:** `flow/graph/graph.go` — new `AddInterruptibleLambdaNode[GI,GO,In,Out any](g, id, fn func(ctx,In)(Out,error)) (NodeRef, error)`; identical to `AddLambdaNode` except `newKind` returns a dedicated `interruptibleLambdaAdapter[In,Out]` = lambdaAdapter behavior + `CanInterrupt() bool { return true }`. Keep `markInProcess` (closure → non-serializable).
- **TDD** (`flow/graph/interrupt_test.go`, package graph, white-box): IsInterruptCapable (resolved kind satisfies InterruptCapable); RunResumableSuspends (string in/out + registered codec → `run.engine.RunResumable` suspends with correct NodeID/Request, then Resume completes); PlainLambda_Interrupt_StillUninstrumented (regression: AddLambdaNode interrupt → ErrUninstrumentedInterrupt); TwoInterruptibleLambdas_SameLayer_Rejected (Compile+store → ErrMultipleInterrupts); OnePerLayer_OK.
- **apisnapshot**: +AddInterruptibleLambdaNode only.

## Item #6 — Precise neighborhood flow-hash diff — DROPPED (2026-06-05)

**Outcome: implemented, then REVERTED. Not worth shipping.** Implementation surfaced a load-bearing constraint: the resume cursor `EdgeStates` is POSITIONAL (resume + `runCore.fireLayerEdges` iterate `for i := range e.flow.Edges { edgeStates[i] }`). So ANY added/removed reachable node changes the edge count → the cursor can't fit (panic-unsafe) → **must reject regardless of region**. Therefore the headline capability (tolerate out-of-region STRUCTURAL add/remove) is impossible without redesigning the cursor from positional to identity-keyed — a much larger change (Phase 3+). The only residual capability a region diff adds over Phase-2's conservative whole-graph `structHash` is tolerating out-of-region edge-RETARGET / port-changing-retype (same edge count), which is niche AND arguably less safe (resuming into a structurally-edited flow). Phase-2's "reject any structural change, allow Config-only" is the better, clearer default. Dropped per simplicity; finer-grained diff deferred to a future milestone that also redesigns the cursor. Original design retained below for reference.

### (reference) original design — Precise neighborhood flow-hash diff
Refine Phase-2's whole-graph `structHash` to a resume-region-scoped comparison so out-of-region structural changes become resume-safe; still reject anything that corrupts the positional `EdgeStates` cursor.
- **Region** = interrupt node {id,type,sorted out-ports} + its out-edges (deferred) + transitive forward closure over live (Pending/Deferred) edges in layers ≥ SuspendLayer, with those nodes' {id,type,sorted in/out ports} + in/out edges (identity tuple AND stable index). Out-of-region = nodes wholly in layers < SuspendLayer with all edges terminal.
- **Reject**: interrupt node removed/retyped/out-ports changed; region edge added/removed/retargeted/cond-changed; **region-edge index shift** (mandatory test — cursor landmine); downstream-in-region node removed/retyped/in-ports changed. **Allow**: out-of-region Config/port/topology changes; in-region port-neutral Config.
- **Files**: new `flow/region.go` (`computeRegion`, `regionChanged`); `flow/checkpoint.go` (store structured region descriptor at suspend; empty → fall back to Phase-2 strict StructHash compare for already-saved checkpoints); `flow/engine.go` snapshotCheckpoint (compute+store region); `flow/resume.go` (replace `cp.StructHash != e.structHash` branch with `e.regionChanged(cp)`, keep FlowHash fast-path + v1 strict fallback).
- **TDD** (`flow/region_test.go`): OutOfRegionConfigChange_Allowed (+ out-of-region node/edge appended at END so region indices intact → resume yields X); InterruptNodeOutputPortChanged_Rejected; RegionEdgeConditionChanged_Rejected; DownstreamInRegionRetyped_Rejected; **RegionEdgeIndexShift_Rejected** (mandatory); IdenticalFlow_FastPath; OldCheckpointWithoutRegionDescriptor_FallsBackToStrict.
- **apisnapshot**: +region descriptor field on Checkpoint (exported, JSON-serializable so sqlite persists it for free).
- **Open Q**: region descriptor inflates checkpoint payload (bounded by live forward closure, usually small — accept).

## Deferred to a future milestone (post-Phase-3-tractable)
#1 (scheduler rework, co-design first), #4 (graph State, co-design first), #2/#3 (gated on #1), #5 (multi-interrupt, defer).
