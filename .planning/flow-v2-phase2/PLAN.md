# flow-v2 Phase 2 — Implementation Plan

Status: **PLAN — Phase 1 DONE (10/10) + audit fixes landed. Phase 2 scoped/sequenced 2026-06-05.** Source: `.planning/flow-v2/PLAN.md` §3 "Phase 2 (post-MVP)" + RV-6 + the Phase-1 audit carry-over.

Branch: `feat/v2-engine-core` (all v2 work, unpushed). GOWORK=off everywhere. LSP false-positives on the v2 nested module — trust only real `go build/test`. Root + otel module commits need `--no-verify` until v2 is tagged (local `replace …/v2 => ./v2`).

## Sequencing (decided): 5 → 1 → 2 → 3 → 4
- **5 first**: smallest, no exported surface, de-risks item 4 by making concatenator lookup consistent.
- **1 second**: self-contained, RV-6-specced, lives in `llm-agent-otel` module (zero new dep on v2 go.mod).
- **2 third**: correctness-sensitive engine work, isolated before ergonomics.
- **3 then 4**: graph items; 4's fan-in merge targets 3's fan-in node shape. 4 depends on 5.

## Accepted design decisions (Plan-agent defaults, internal/reversible)
1. **Item 2 safe-scope** = out-of-region `Node.Config`/metadata changes only; arbitrary structural add/remove out-of-region → Phase 3 (the `EdgeStates`-by-stable-index coupling makes index-remap risky). Satisfies the "config tweak blocks resume" motivating case.
2. **Item 2 storage** = store a structured region descriptor in the Checkpoint (`Version`→2, additive; v1 checkpoints with empty descriptor fall back to strict full-hash compare).
3. **Item 3 fan-in** = multi-input-port combiner node (no engine change; per-port threading already exists). Rejected slice-valued single port (needs engine aggregation).
4. **Item 4 `copy` backpressure** = bounded shared-rate tee (slow consumer slows all; must Close). Rejected unbounded-per-consumer (OOM/leak footgun).
5. **Item 5 ambiguity** = fail-loud when >1 registered folder is assignable but none exact (extended error msg). Rejected pick-first.

---

## Item 5 — assignability-aware `lookupConcat` (S, ~0.5d)
**File:** `v2/flow/graph/stream.go` only. No exported-surface change → no apisnapshot regen.
- `lookupConcat(targetT)`: exact hit → return (fast path). Miss → scan registry keys `k`; usable iff `assignable(k, targetT)` is `(ok=true, defer=false)` (reuse `typecheck.go` `assignable`). Exactly one assignable → use it; >1 and none exact → `(nil,false)` + extended "N candidates assignable, none exact" error.
- Caveat: `buildLinearPipeline` static check (`runnable.go`) gets the fix for free; grep stream_test/lower_test that no test asserts the OLD rejection as desired.
- TDD: ExactStillWins, InterfaceTargetViaAssignability, AnyTarget, AmbiguousAssignable_Errors, Stream_NotMoreRestrictiveThanInvoke.

## Item 1 — otelflow/v2 resumable wrapper (M, ~1.5d)
**New package `llm-agent-otel/otelflow/v2/`** (import `github.com/costa92/llm-agent-otel/otelflow/v2`) — keeps otel out of v2 go.mod. Add to `llm-agent-otel/go.mod`: `require …/v2` + `replace …/v2 => ../llm-agent-flow/v2`.
- `baseWrapper{inner flow.Runner,...}` Run/RunStream (port v0.1 goroutine-drain + per-node child spans over v2 typed `flow.Event`; annotate `NodeInterrupted`/`FlowSuspended`). `resumableWrapper{baseWrapper; rinner flow.ResumableRunner}` adds RunResumable (`flow.suspend` span) / Resume (`flow.resume` span + `flow.resume_token` attr). `Wrap(inner, cfg) flow.Runner` type-asserts ResumableRunner → returns the richer wrapper; capability via dynamic type. v0.1 `otelflow.Wrap` untouched.
- TDD (tracetest.SpanRecorder): ResumableInnerExposesCapability, NonResumableInner_NoFalsePositive, ResumeEmitsSpanPair, RunStreamSpansV2Events. Check `llm-agent-otel/` for its own apisnapshot gate.

## Item 2 — structural flow-hash diff (L, ~2.5-3d) ★ correctness-sensitive
**Files:** `resume.go` (replace `cp.FlowHash != e.flowHash` guard with `e.regionChanged(cp)`; keep fast-path equality), `checkpoint.go` (+`RegionSig`/structured `Neighborhood` field, `Version`→2, v1 fallback), `engine.go` `snapshotCheckpoint` (compute+store region at suspend), new `region.go` (`computeRegion`, `regionChanged`).
- **Region** = interrupt node {id,type,sorted output ports} + its out-edges (deferred) + transitive forward closure over live (pending/deferred) edges in layers ≥ SuspendLayer, with those nodes' in/out edges + input ports.
- **Breaking (reject ErrFlowChanged):** interrupt node removed/retyped/output-ports changed; any region edge added/removed/retargeted/condition-changed; downstream-in-region node removed/retyped/input-ports changed; **any edit that shifts the stable index of a region edge** (the `EdgeStates[i]` cursor landmine — compare by identity tuple AND verify index stability).
- **Safe (allow):** out-of-region `Node.Config`/metadata only.
- TDD: OutOfRegionConfigChange_Allowed (yields headline X), InterruptNodeOutputPortChanged_Rejected, RegionEdgeConditionChanged_Rejected, DownstreamInRegionRetyped_Rejected, **RegionEdgeIndexShift_Rejected** (mandatory), IdenticalFlow_FastPath, OldCheckpointWithoutRegionSig_FallsBackToStrict. apisnapshot regen (Checkpoint field/Version).

## Item 3 — parallel node sugar (L, ~3d)
Builder ergonomics over the existing layered engine (already runs a layer concurrently via fanout). Needs **multi-input-port** generalization of the graph layer (today single `portIn`).
**Files:** new `parallel.go` (`AddParallel` fan-out: forward `In` to N branches, all fire; `AddCombine` fan-in: node declares one in-port per source, adapter gathers + calls `combine`), modify `graph.go` (multi-named-input-port `NodeRef`/adapter + `AddEdgeTo(from,to,toPort)`; plain `AddEdge` defaults to portIn), `typecheck.go` (per-port type check). Skipped source → combiner treats as absent, passes present subset.
- Diamond fan-in works because topo layering puts the combiner past all predecessors. Multi-port node → `StreamCapable()==false` (degrades to box(Invoke)) — respects Phase-3 streaming boundary.
- TDD: FanOutRunsAllBranches, Combine_GathersAllSources, Combine_TypeMismatch_FailsAtBuild, Parallel_NotStreamCapable, ByteIdenticalToManualWiring. apisnapshot regen.

## Item 4 — merge/copy stream adapters (L, ~3d) — depends on item 5
**Phase-2 boundary: `StreamReader[T]` combinator FUNCTIONS, NOT in-graph engine-scheduled nodes** (the latter needs the partial-barrier scheduler → Phase 3).
**Files:** new `combinators.go` (+test). Promote the `pipedReader` template (`stream.go`) to real use.
- `Copy[T](src, n) []StreamReader[T]` (tee): one broadcaster goroutine, bounded per-consumer buffers, shared-rate backpressure; `defer src.Close()`; all-consumers-closed → cancel + release src; per-consumer Close drains (two-stage).
- `Merge[T](srcs...) StreamReader[T]` (interleave): one puller goroutine per src into a shared bounded channel; clean EOF when all done; Close = cancel-then-drain releasing every src.
- TDD (goroutine-leak assertions MANDATORY): Copy_AllConsumersSeeAll, Copy_CloseAllReleasesSource_NoLeak, Copy_EarlyConsumerClose_NoLeak, Merge_InterleavesAll, Merge_CloseMidStream_NoLeak, Merge_SourceErrorPropagates. apisnapshot regen.

---

## Deferred to Phase 3
- In-graph merge/copy NODES scheduled mid-graph (partial-barrier scheduler rework).
- Ordered/zip merge tied to node scheduling.
- Arbitrary structural add/remove out-of-region in flow-hash diff.
- `AddInterruptibleLambdaNode` (typed-front-end HITL) — RV-2 open note; only if a consumer needs it (today HITL via JSON route).

## Phase-1 invariant interactions
- Resume cursor (make-or-break): only item 2 touches it — the index-shift rejection test is mandatory. Items 1/3/4/5 don't touch the resume path.
- No-goroutine-leak streaming: items 1 (RunStream span goroutine) + 4 (tee/merge) — mandatory leak-assertion tests, reuse vetted skeletons.
- Byte-identical fresh runs: no engine scheduling change in any item. Preserved.
