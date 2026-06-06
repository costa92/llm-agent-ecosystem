# flow-v2 Phase 3 Item #1 — Streaming through Branch/Parallel — Co-Design

Status: **DESIGN — round 1 (Plan agent) + round 2 (adversarial review) DONE; codex round deferred (quota-blocked, optional 3rd pass). REVISED below. NO implementation code until the user ratifies the revised scope.** Governing principle: build the streaming DAG executor as a SEPARATE path reachable only via `Runnable.Stream` on `StreamCapable()` graphs, that NEVER touches `runCore`/`fireLayerEdges`/`snapshotCheckpoint`/resume — so the three invariants are protected by construction (run-state proof VERIFIED in round 2).

## Round 2 (adversarial review) — REVISED PLAN (supersedes the sub-task list below where they conflict)
Round 2 caught a BLOCKER before any code was written:
- **MAJOR-3 (blocker): the tee (Copy, buf=1, broadcaster sends to ALL consumers per-frame) DEADLOCKS against the "inline single-goroutine walk"** — sequential consumer-by-consumer fold can't satisfy a broadcaster that needs all consumers drained concurrently. **→ DEFER the tee (drop sub-task 1c this milestone).** It's only needed for direct chatmodel-stream-out → N-targets, a shape no consumer has asked for (AddParallel emits a VALUE to all routes — no tee). Build it later with a deliberate goroutine-per-consumer model.
- **MAJOR-4: `runLinearStream` has a REAL latent leak** — no `defer`-close of a live carrier on its error returns (370/377/394/409/415). **→ PRE-TASK: fix it first** with an explicit live-carrier cleanup set (close-all on error return + tee-broadcaster cancel), and reuse that helper in the DAG executor. Backport, don't leave two divergent error stories.
- **MAJOR-2: Tier-A fold-then-combine makes combine-TERMINATED graphs Invoke-equivalent (ZERO incremental streaming).** Real streaming happens only when the exit is downstream of a stream node WITHOUT crossing a combine (the branch case). **→ 1d (parallel+combine) is CORRECTNESS-only (pruning + leak discipline), not throughput; re-justify or defer. Document the equivalence honestly (its equivalence-vs-Invoke test passing is the TELL that no streaming occurred).**
- **MAJOR-1: `StreamCapable()` is registry-snapshot-dependent** (the global `concatRegistry` is read by Compile). Not an invariant break (Invoke unaffected), but document: `buildStreamPlan` snapshots folder-availability at Compile; `RegisterConcatenator` after Compile doesn't retroactively change a Runnable. Add a register-after-compile regression test. The executor must only READ via `lookupConcat`, never `registerConcat`.
- **OQ decisions (round 2):** (1) KEEP `runLinearStream` + extract shared fold/Close helpers — do NOT subsume (the linear path is the byte-identical oracle; subsuming loses the cross-check). (2) Backport the leak fix to runLinearStream FIRST (pre-task). (3) DEFER the tee. (4) Single-folder-per-type registry is sufficient (per-edge override is YAGNI); prefer concrete combine-port types to avoid interface ambiguity.
- **VERIFIED sound:** the CEL-edges-don't-exist-on-typed-path claim (graphEdge has no Condition; neither Compile nor lower set it) and the run-state proof-by-construction (Invoke→engine, Stream→executor, disjoint; adapter instances NOT shared — newKind() called separately).

**Revised implementable scope (this milestone): PRE-TASK (leak fix) → 1a (DAG executor skeleton, KEEP linear + shared helpers) → 1b (branch streaming + pruning — the genuinely-streaming case).** DEFER 1c (tee — deadlock). 1d (parallel+combine) = correctness-only, defer or re-justify. 1e (StreamCapable widening) folds into 1a/1b with the registry-snapshot note. This is much smaller and honester than the round-1 plan.

---
### (round 1) original design follows — read with the round-2 revisions above taking precedence

## Decisive verdicts (resolved open questions)
| OQ | Decision |
|---|---|
| Streaming + checkpoint coexist? | **NO.** Stream is a Stream-only, non-checkpointable path; Invoke/RunResumable/Resume stay on `runCore`. Streamable graphs hold an injected ChatModel → non-serializable → Checkpointable()==false, so they're never run with checkpoint=true. |
| Accepted shapes / conditional edges? | **Parallel (AddParallel+AddCombine) + Branch (AddBranch) stream.** CEL/conditional edges DON'T EXIST at the typed-graph layer (`graphEdge` has no Condition; CEL is IR-only via Invoke) — so the "can't project a live stream to a CEL string" problem never arises on the streaming path. Branch routing is a Go key func over a materialized value. Unsupported shapes degrade to box(Invoke), never error. |
| Fan-out 1→N | Reuse Phase-2 `Copy` (tee), bounded shared-rate backpressure (accept as-is; per-branch unbounded rejected — OOM footgun). Note: AddParallel emits the same VALUE on all routes (no tee); tee only needed for direct chatmodel-stream-out → N targets. |
| Fan-in N→1 | **Tier A (this milestone): fold-each-branch-then-combine** — at the combine boundary every branch stream is folded to a value (per-port `lookupConcat`), combine runs once on concrete inputs (branch-pruned ports absent). **Tier B (DEFERRED to item #3): a streaming combiner consuming N live streams via Merge/zip** — needs a new node interface; out of scope. |
| Leak discipline | One goroutine per stream-producing node + `Copy` broadcasters; cancel-ctx + two-stage-Close propagated root→leaf via Copy/Merge/foldTail skeletons; proven by per-shape `settleGoroutines()` assertions in 3 modes (full drain / early Close / Close-before-Next). |

## Load-bearing code facts
1. Typed graph layer has NO conditional edges (CEL is IR-only). Branch routing = Go func over a materialized value. So the streaming path has no CEL impedance mismatch.
2. Only `chatModelAdapter` produces a real stream (sole `streamNode`). Streaming "spreads" only downstream of a ChatModel; everywhere else the carrier is already a value.
3. branch/parallel/combine adapters are value-Run nodes; the executor must re-implement branch pruning ("unselected route's downstream subgraph not scheduled").
4. `Copy`/`Merge` (Phase-2) are vetted + leak-tested; the executor wires them, doesn't reinvent.
5. `buildLinearPipeline` returns nil for any fan-out (`fromPort!=portOut`) / fan-in (`toPort!=portIn`) → degrade. That's the classification boundary we widen.

## Architecture
- **Per-edge `streamCarrier`** (value-or-stream union, reused). `box`/`concatToValue`/`foldTail`/`lookupConcat` apply unchanged. No new IR.
- **New `runStreamDAG` SUBSUMES `runLinearStream`** (linear = degenerate DAG with in/out degree ≤1). Acceptance gate: must pass every existing linear `stream_test.go` case byte-identically before any branch/parallel test is added. (Codex OQ: subsume-and-delete vs keep-linear + shared helper.)
- **Scheduling:** layered topological walk over the typed graph (recomputed in `graph` from `g.edges` — never touches the engine). Fold-on-input at value boundaries; `streamNode` re-emits a stream; branch prunes; parallel emits-all; combine folds-then-runs.
- **Goroutines:** the walk runs inline on the consumer goroutine (like runLinearStream); goroutines appear only from Copy broadcasters (+ Merge in Tier B) + the underlying llm stream. Tail via foldTail/box/direct.

## `StreamCapable()` widening — `buildStreamPlan()` (sibling of buildLinearPipeline), 3-way:
- **plan** iff streamable DAG: every node ∈ {streamNode, value-lambda/template/tool/passthrough, branch, parallel, combine} AND every stream→value boundary (each combine input port + tail) has a registered folder. Linear is the degenerate case, run identically.
- **(nil,nil) → degrade to box(Invoke)** for any shape it can't PROVE streamable (whitelist detection; unknown node type → degrade). No error.
- **(nil,err) → Compile error** ONLY when the graph IS streamable but a folder is missing (same escalation as buildLinearPipeline). `StreamCapable() == r.plan != nil`. Mandatory regression: every currently-linear graph stays StreamCapable + byte-identical.

## Sub-tasks (TDD, executor-ready, each its own commit)
- **1a** Streaming DAG executor skeleton (subsumes linear; linear + single-branch-no-tee). New `streamdag.go`; `Runnable.plan`; `Stream` dispatch. Gate: existing linear suite passes unchanged + observable-equivalence (Stream concat == Invoke).
- **1b** Branch streaming + pruning. Only selected route's forward closure scheduled; unselected never allocated/run. Tests: StreamsSelectedRoute, UnselectedNotInvoked (spy), leak, equivalence-vs-Invoke both routes.
- **1c** Fan-out tee (`Copy`) for stream-port→N edges. Tests: ChatModelOut_FeedsTwoNodes_Tee, backpressure characterization, early-Close releases all broadcasters (leak).
- **1d** Parallel fan-out + Combine fan-in (Tier A fold-then-combine). Tests: both-branches-stream-combine-folds, skipped-branch-absent, per-shape (1/2/3) leak, equivalence-vs-Invoke.
- **1e** Widen StreamCapable/buildStreamPlan + degrade safety. Tests: LinearStillCapable (regression), Parallel/Branch IsCapable, UnknownShape_DegradesNoError, MissingFolder_CompileError. apisnapshot: Tier A adds NO exported symbols → empty diff.

**Mandatory across all:** `settleGoroutines()` base-vs-after every test; explicit observable-equivalence-vs-Invoke per shape (the cross-validator against scheduler divergence).

## Invariant-protection (proof-by-construction)
1. **Resume cursor**: streaming executor never constructs/reads/writes a Checkpoint/EdgeFireState/Activated; can't reach snapshotCheckpoint/resume seed. No cursor state to corrupt.
2. **Byte-identical Invoke**: Invoke→`r.engine.Run`→runCore; Stream→runStreamDAG which doesn't call `r.engine` at all. Disjoint code. Equivalence tests prove same RESULT.
3. **Sibling-error-wins-no-checkpoint**: that ordering is in runCore's suspend splice; streamable graphs are non-checkpointable and Stream never enters runCore. Unreachable → unperturbable.
Shared surface is read-only topology + the same node.Run/streamRun. The executor re-implements pruning/layering (duplication, not coupling); the equivalence-vs-Invoke tests are the cross-check against divergence.

## Risk register
| Risk | Sev | Mitigation |
|---|---|---|
| Scheduler divergence (streaming pruning/order ≠ engine) | High | observable-equivalence-vs-Invoke per shape |
| Latent error-path leak in linear executor inherited by DAG | Med | DAG executor defer-closes all live carriers on error + a node-errors-mid-stream leak test (note: pre-existing gap in runLinearStream) |
| Tee shared-rate stalls siblings | Med | document in Stream godoc; characterization test |
| Misclassify future node type as streamable | Med | whitelist detection → degrade; UnknownShape test |
| Combine interface-typed port folder ambiguity | Low | reuse lookupConcat fail-loud-on-ambiguity |

## Deferred
Streaming fan-in / live-stream combiner (AddStreamCombine, Merge/zip) → item #3. In-graph merge/copy NODES → item #2 (builds on this executor). Streaming+checkpoint → needs cursor redesign, no planned item (positional EdgeStates can't represent an in-flight stream — see #6 drop note). graph State + streaming → #4, same non-checkpointable rule.

## Open questions for codex (round 2)
1. Subsume-and-delete runLinearStream vs keep-linear + extract shared fold/Close helper (divergence risk vs regression risk)?
2. Fix the latent mid-graph error-path leak only in the new DAG executor, or backport to runLinearStream as a pre-task?
3. Build the tee (1c) now, or defer direct-chatmodel-stream-fanout to box(Invoke) until a consumer needs it?
4. Per-port combine folder via single-folder-per-type registry sufficient, or need per-edge folder override?
