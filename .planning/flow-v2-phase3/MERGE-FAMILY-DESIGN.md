# flow-v2 Phase 3 Item #3 — The Merge Family (streaming fan-in NODES + tail-fold) — Co-Design

Status: **DESIGN — round 1 (Plan agent) + round 2 (adversarial) DONE. REVISED below. STOP — awaiting user ratification before any implementation code.** Builds directly on #2 (Copy-only, SHIPPED, v2.1.0). Governing principle (inherited, non-negotiable): the streaming executor is a SEPARATE, Stream-only, NON-checkpointable path that NEVER touches `runCore`/`fireLayerEdges`/`snapshotCheckpoint`/resume; I1/I2/I3 protected by construction; `runLinearStream` stays the byte-identical oracle; equivalence-vs-Invoke per shape is the cross-validator. **#2 Round-2 LAWs are binding** and are cited inline.

## Round 2 (adversarial review) — REVISED RESOLUTIONS (supersede §1–§8 where they conflict)
Round 2 PROVED one BLOCKER + one MAJOR with scratch tests (now deleted; source unchanged), and REFUTED the round-1 R-ZIPDEADLOCK suspicion.

- **BLOCKER (Q9/§B.3) — DEFER THE DIAMOND.** The shipped `fanOutCopy` single-tail model is NOT surgically extendable. Three v2.1.0 mechanisms assume "one tail subtree reaches exit, others are drain-leaves": (1) `subtreeReaches` (streamgraph.go:520) walks transitively THROUGH any node with no boundary concept; (2) **`driveLeafSubtree` (streamgraph.go:492) DRAINS-TO-EOF-AND-CLOSES, discarding its subtree's output** — in the diamond, the "loser" Copy target (a real Merge source) has its frames **silently dropped into the void → silent data loss** (a #4-class silent-clobber bug, proven: `subtreeReaches` returns TRUE for BOTH copy targets, tailIdx picks one, the other's data is discarded); (3) `walkChain` has no merge boundary (a merge node falls through to the value-node `default` and gets single-port `Run`, wrong for an N-source fan-in). Shipping the diamond requires fan-in dominator analysis at plan-build + threading a merge-boundary through `subtreeReaches`/`fanOutCopy`/`walkChain` — a structural rewrite of shipped Copy with real regression risk. **→ MVP = `AddStreamMerge`/`AddZip` with NO upstream Copy in the graph.** Then `subtreeReaches`/`driveLeafSubtree`/`fanOutCopy` never execute (no Copy present) and `fanInMerge` is PURELY ADDITIVE. The diamond + §B.3 restructure becomes its own future task. (Mirrors #1 cutting the tee, #2 cutting Merge, #4 cutting streaming-State.)
- **MAJOR (Q3) — the combine is NOT a `lookupConcat` tail-folder (Q2/Q3 contradict).** `materialiseTail` folds via `lookupConcat(r.outT) → func(StreamReader[any])(any,error)`; the user combine is `func(ctx,[]T)(R,error)` — they don't unify (combine takes a materialized `[]T`+ctx; the folder takes a live stream, no ctx). Routing combine through the global `registerConcat` is **exactly the register-after-Compile-invisibility wart #2-r2 forbids** (and that Q2 itself rejects). **→ Build an explicit combine→fold bridge OWNED BY THE MERGE NODE's continuation (drain merged `StreamReader[any]` → `[]T` → `combine(ctx,[]T)`), NOT the global registry.** Document: the tail fold FULLY BUFFERS `[]T` at the merge boundary (acceptable — the merge IS the buffering point; the streaming advantage is upstream of it). Delete the round-1 "no new fold machinery" claim.
- **MINOR (Q1/3b) — `roundRobinMerge` is FEASIBLE (R-ZIPDEADLOCK REFUTED).** Scratch tests proved: deterministic output over 50 runs; no deadlock on Copy→{A,B}→Zip at N=8 (≥buf+2); no stall with a 3ms/frame slow source-0 over a shared Copy (the output selector's `<-chans[0]` is a RECEIVE that lets source-0's puller advance while source-1's puller drains concurrently). **buf-1 lookahead is SUFFICIENT — no unbounded lookahead, no OOM.** Requirement: `roundRobinMerge` must replicate `Merge`'s exact two-stage Close per source channel (the prototype leaked 2 goroutines on early-Close until the stage-2 drains were added) — gate on `TestRoundRobinMerge_EarlyClose_NoLeak`.
- **MINOR (Q6) — classifier:** the relaxed fan-in rule MUST still run the implicit-tee `portFanout>1` check (streamgraph.go:102) — a merge source port must not double as an implicit-tee origin. Make `mergeKind` a DISTINCT interface (not satisfied by `combineAdapter`) so `AddCombine` fan-in still degrades. `out map[string]string` + new inbound `sources []mergeSource` coexist fine (representable; `reaches` iterates `out`, unaffected).
- **MINOR (Q5) — cross-validator:** `Merge`'s per-source FIFO IS guaranteed (each `pullMerge` loops one source strictly, combinators.go:226) so reorder-within-a-source can't happen via `Merge` — but ADD the explicit per-source-order assertion (cheap insurance; FIFO is an impl property, not type-enforced). Multiset+count+completeness adequate for unordered; `AddZip` SEQUENCE equality sound given proven determinism.
- **VERIFIED sound:** the #2 sequential-drain LAW re-proven (sequential merge over shared Copy hangs 3s at N=5; concurrent completes instantly); `roundRobinMerge` determinism+deadlock-freedom (3 probes); `Merge` per-source FIFO (code); shipped `fanOutCopy`/`subtreeReaches`/`driveLeafSubtree` single-tail (traced + diamond probe); plan `out`+`sources` coexistence; value-path sorted-key determinism.

**REVISED MVP scope (recommended): ship `AddStreamMerge` (unordered, reuse `Merge[any]`) + `AddZip` (round-robin, new `roundRobinMerge[any]`) — STANDALONE fan-in only, NO upstream Copy. DEFER the Copy→{A,B}→Merge diamond and ALL §B.3 `fanOutCopy` restructuring to its own future task** (needs fan-in-dominator plan analysis + a `driveLeafSubtree`-doesn't-discard-merge-sources fix). Value path = sorted-key gather + explicit user combine, with a merge-node-owned combine→fold bridge (NOT global registry). `roundRobinMerge` buf-1 + faithful two-stage Close. Classifier preserves the implicit-tee guard + distinct `mergeKind`.

Revised sub-tasks: 3a (value merge/zip nodes + Invoke) → 3b (`roundRobinMerge` + two-stage Close + leak gate) → 3c (classifier fan-in extension, preserve tee guard) → 3d (`fanInMerge` STANDALONE — N concurrent source goroutines → merged reader → continuation w/ combine→fold bridge + elemT re-stamp + liveSet; NO diamond). **3e (diamond) DEFERRED to a future task.**

---
### (round 1) original design follows — read with the round-2 revisions above taking precedence

## 0. Load-bearing code facts (verified against the SHIPPED post-#2 source)

1. **The shipped `streamGraphPlan` adjacency is fan-OUT-only.** Both `streamGraphNode.out` and `streamPlanNode.out` are `map[string]string` = *source-port → single target id* (`streamgraph.go:44`, `streamdag.go:50`). Represents a Copy's N target ports perfectly; **cannot represent N sources converging on one Merge node's named input ports**. **Merge needs a new plan representation** (`sources []mergeSource` on the Merge plan node). Biggest structural delta.
2. **`buildStreamGraphPlan` hard-degrades on ANY fan-in TODAY** (`streamgraph.go:96-99`): `if e.toPort != portIn { return nil, nil }`. Merge sources wire via `AddEdgeTo(src, merge, key)` → `toPort = key ≠ portIn`. **Every Merge-bearing graph currently degrades.** Classifier extension is mandatory and is the I1-preservation boundary.
3. **`walkChain` is a single-CHAIN primitive** (`streamgraph.go:257`): threads ONE carrier forward following `chainNext(portOut)`, STOPS at a Copy boundary by returning the carrier flowing IN. **A Merge is the inverse** (reached by walking INTO it from N source chains). `walkChain` cannot drive a Merge — needs a *fan-in driver* analogous to `fanOutCopy`.
4. **`fanOutCopy` assumes exactly ONE tail subtree** (`streamgraph.go:438-454`): single Copy target whose subtree `subtreeReaches(exitID)` walked on the calling goroutine, rest drained as leaves. **With a Merge, the exit is reached THROUGH the Merge from multiple sources** → "one tail, N leaves" inverts to "N sources, one continuation." `subtreeReaches` from a Copy target now passes *through* the Merge → multiple targets "reach exit," breaking the `tailIdx` single-winner assumption (the diamond, §C / R-DIAMOND).
5. **`Merge[any](srcs...)` (Phase-2) is shippable verbatim for UNORDERED** (`combinators.go:198`). One puller per source → shared buf-1 chan; two-stage Close; closer goroutine; `N=0`→immediate EOF; error forwarded then source done. Leak-tested 3 modes. NO ordering guarantee. Ordered/zip needs a NEW reader.
6. **`elemT` erasure is real and already handled for Copy.** `copyReader` carries `chan copyItem[any]` (no elemT); `fanOutCopy` re-stamps `elemT` from `ck.copyElemT()`. `Merge[any]` likewise erases → the Merge driver must re-stamp `elemT` on the continuing carrier from the Merge node's declared T (the #2-r2 LAW).
7. **The folder/tail-fold mechanism is COMPLETE and reusable** (`lookupConcat`/`concatToValue`/`foldTail`/`materialiseTail`). A Merge yields `StreamReader[T]`; downstream folding to T is the existing mechanism, NO new work (Q3).
8. **`AddCombine` is the value-path fan-in precedent and the answer to the `[]T` problem** (`combineAdapter`, `parallel.go:153`): gathers present named ports in stable sorted-key order, runs a user `combine func`. Merge's value path collapses to this (Q2).
9. **Disjointness holds unchanged.** `Stream` routes to `runStreamGraph` only when `streamGraphPlan != nil`; Invoke→`runCore`. `markInProcess` → non-serializable → non-checkpointable. The #1/#2 by-construction proof carries.

## 1. Decisive resolutions

| # | Question | Resolution | Rejected alternatives + why |
|---|---|---|---|
| **Q1** | Scope + API | **Ship TWO constructors: `AddStreamMerge` (unordered, reuses Phase-2 `Merge[any]`) + `AddZip` (deterministic ordered via round-robin pull). NO `MergeMode` enum.** Two named constructors mirror the existing family; each carries its own value-path contract (Q2) + cross-validator (Q5). | `AddMerge(mode)` enum — hides the value-path/oracle/N-mismatch asymmetry. "source-1-fully-then-source-2" ordered mode — **DEADLOCKS under the #2 BLOCKER LAW** (sequential subtree materialization). Round-robin is the only ordered mode compatible with concurrent fan-in. |
| **Q2** | `[]T` value-path (LAW: `[]T` ungeneralizable) | **CUT the `[]T` value path. Both constructors take a user `combine`: `AddStreamMerge(sources, combine func(ctx,[]T)(R,error))`, `AddZip(sources, combine func(ctx,[][]T)(R,error))`. On Invoke, run combine over sorted-key-gathered source values; on Stream, the combine is the registered tail-folder for the merged stream.** I1 holds (deterministic), the impossible `Concatenator[T]→[]T` registration is dead. | Stream-path-ONLY (error on Invoke) — asymmetric wart, breaks "Stream degrades to Invoke." Auto-register `[]T` folder at build — rejected by #2-r2 LAW (global mutation, register-after-Compile invisibility). |
| **Q3** | Tail-fold of Merge output → T | **The EXISTING `materialiseTail`/`lookupConcat`/`foldTail` applied at the Merge downstream boundary — NO new fold machinery.** The user combine (Q2) IS the fold (T-stream→R), routed through the existing path. | Bespoke merge folder / new registry — duplicates `foldTail`. |
| **Q4** | Concurrent fan-in (LAW) | **New `fanInMerge` driver mirroring `fanOutCopy`: ONE goroutine per source subtree, each walking via `walkChain` to a `StreamReader[any]`, collect N readers concurrently, feed `Merge[any]`/`roundRobinMerge[any]`, continue from the Merge's single `portOut` (elemT re-stamped). Concurrent by construction → no deadlock.** Each source reader + merged reader registered in `liveSet` independently. | Sequential source walk — VIOLATES the #2 BLOCKER LAW (diamond deadlock). |
| **Q5** | Ordered determinism + Invoke oracle | **Unordered cross-validator = MULTISET + count + per-source completeness (tag frames by source) + N=1 sequence + forced-concurrency small-buffer stress (LAW). Ordered (`AddZip`) round-robin = DETERMINISTIC → SEQUENCE equality vs an Invoke oracle that gathers each source's full list and round-robin-interleaves in the SAME sorted-key order.** | "ordered merge has no Invoke oracle" — the oracle is the deterministic gather order (sorted key) + interleave rule (round-robin), both reproducible on materialized values. Equal-length positional zip — hangs on mismatch (R-ZIPLEN); round-robin tolerates ragged lengths. |
| **Q6** | Classifier + degrade | **Extend `buildStreamGraphPlan`: (a) recognize merge nodes via new `mergeKind`; (b) REPLACE blanket `toPort != portIn → degrade` with "allowed ONLY when `to` is a merge node and toPort is a declared source key; else degrade"; (c) add inbound `sources` to the merge plan node. Folder = the combine, validated per source boundary; missing → Compile error. Degrade preserved: implicit raw tee, cycles, unknown kinds, AddCombine fan-in, hetero-T.** Linear/branch/copy UNCHANGED + byte-identical (gated). | Folding merge into `buildStreamPlan` (branch-DAG, single-goroutine) — structurally can't do concurrent fan-in. |
| **Q7** | Leak discipline | **N source-subtree goroutines + Merge reader + N internal pullers all in `liveSet`; root cancel-ctx; two-stage Close on every source reader AND merged reader; `streamGraphReader.Close` cancels root + `closeAll`. 3-mode `settleGoroutines` per shape, N∈{1,2,3}, + diamond.** Hard rule (LAW): executor registers+Closes EVERY source reader independently. | Letting `Merge` own source cleanup — a source erroring before `Merge` consumes it could wedge. |
| **Q8** | Checkpoint | **Merge-bearing graphs non-checkpointable by construction** (Stream-only path + `markInProcess`). Double protection. | n/a |
| **Q9** | The Copy→{A,B}→Merge diamond | **MVP SHIPS the diamond, gated on concurrent fan-in LAW + the §B.3 tail-selection restructure + a dedicated diamond leak+liveness test.** BUT `subtreeReaches(exit)` single-tail breaks (both Copy targets reach exit through Merge) → requires §B.3 restructure, not just adding `fanInMerge`. If round 2 finds the restructure risky, defer the diamond (MVP = Merge-without-upstream-Copy). | Defer the diamond entirely — it's the most-requested shape and #2 paid for the hard (Copy) half. |

## 2. Scope / public API

```go
// AddStreamMerge — UNORDERED streaming fan-in. N sources of same element type T
// interleaved by arrival on the STREAM path (Phase-2 Merge); on Invoke, gather
// present sources in stable sorted-key order and apply combine over []T. Same
// combine = tail-folder for the merged stream → Stream/Invoke equivalent R.
// markInProcess. inPorts = {key: src.outT (==T)}, outT = R.
func AddStreamMerge[GI, GO, T, R any](
    g *Graph[GI, GO], id string,
    sources map[string]NodeRef,
    combine func(ctx context.Context, items []T) (R, error),
) (NodeRef, error)

// AddZip — ORDERED streaming fan-in via round-robin pull (one frame per source
// in sorted-source-key order, repeating; ended source skipped; ragged lengths
// tolerated). On Invoke, gather each source's ordered []T (sorted key) into
// [][]T and apply combine. Stream/Invoke equivalent by the SAME deterministic
// interleave. markInProcess.
func AddZip[GI, GO, T, R any](
    g *Graph[GI, GO], id string,
    sources map[string]NodeRef,
    combine func(ctx context.Context, perSource [][]T) (R, error),
) (NodeRef, error)
```

**MVP boundary:** `AddStreamMerge` (unordered, reuse `Merge[any]`) + `AddZip` (round-robin, new `roundRobinMerge[any]`); concurrent `fanInMerge`; deterministic sorted-key value path + user combine (I1 holds); new `mergeKind` + classifier extension; the diamond (gated on §B.3 + round 2).

**Explicit DEFERRALS:** equal-length positional zip-tuples (hang footgun → never); heterogeneous-T merge (→ `AddCombine`); streaming combine emitting R incrementally (→ future); arbitrary copy/merge meshes (MVP = single diamond); Merge+checkpoint (no item).

## 3. Concurrency contract

### A. Unordered fan-in (`fanInMerge`, mirrors `fanOutCopy`)
1. Merge reached via an edge whose `to` is a merge node. Merge has N inbound edges → executor drives it from a **fan-in entry point**, not mid-`walkChain` (fact #3). Plan records `sources []mergeSource{key, srcNodeID}` (sorted by key).
2. At the boundary: spawn ONE goroutine per source, each walking its subtree via `walkChain` to a terminal carrier (value folded to a boxed stream → every source yields `StreamReader[any]`). **All N concurrent** → #2 BLOCKER LAW obeyed (shared-upstream-Copy: both copyReaders pulled concurrently, broadcaster never wedges).
3. Collect N readers (sorted-key, deterministic), register each in `liveSet`, call `Merge[any]`/`roundRobinMerge[any]`, register the merged reader, re-stamp `elemT = mergeNode.elemT`.
4. Continue the forward walk from the Merge's single `portOut`, carrying the merged stream → folds to R at the next value node / root tail via combine-as-folder (Q3).

**Subtlety vs `fanOutCopy` (dual):** all N sources on spawned goroutines, the continuation on the calling goroutine. The calling goroutine does NOT block on sources — it constructs `Merge(readers...)` immediately (lazy pull) and continues; source goroutines + Merge's internal pullers run concurrently; buf-1 shared chan = backpressure.

### B. The diamond — executor restructure (the real #3 complexity)
Shipped model = "one tail subtree reaches exit, others are leaves" (`streamgraph.go:438`). Diamond `entry→Copy→{A,B}→Merge→exit`: both A and B reach exit *through* Merge → `subtreeReaches(exit)` true for both → `tailIdx` arbitrary, the other wrongly treated as a drained leaf (it's a Merge source). **Correctness bug if `fanInMerge` is bolted on naively.**

**§B.3 restructure:** `fanInMerge` OWNS all source subtrees, including ones starting at a Copy target (each starts from its copyReader entry). `fanOutCopy`'s role reduces to teeing; convergent subtrees are NOT tail/leaves but Merge sources. The top-level executor becomes a small scheduler dispatching the right driver at each fan boundary (Copy OR Merge). **This is the load-bearing restructure round 2 must stress (R-DIAMOND, OQ-1).**

### C. Diamond deadlock proof (buf-1 cycle)
Channels form a DAG (Copy→A/B→Merge→Next), all buf-1, every blocked send has a concurrently-running receiver (broadcaster→copyReaderA drained by A-walk; A-walk→mergePullerA drained by mergePuller; mergePuller→sharedChan drained by Next). No cycle → **no deadlock**, PROVIDED all subtrees pulled concurrently (LAW). Round 2 reproduces with a scratch test at ≥(buf+2) frames (the #2 BLOCKER gate).

### D. Round-robin (zip) determinism + pull mechanism
`roundRobinMerge[any](readers...)` = NEW reader. **Determinism:** output order fixed by source index + frame index, independent of arrival timing → SEQUENCE equality vs Invoke. **HAZARD (R-ZIPDEADLOCK):** naive serial pull ("pull source 0, then 1, …") deadlocks if source 0 blocks on a shared upstream Copy needing source 1 drained. **Mitigation:** ONE puller goroutine per source + per-source buf-1 lookahead; round-robin order applied at OUTPUT selection (pick from per-source buffers in index order), NOT by serializing pulls → every source concurrently drained (LAW) while output stays deterministic. **The #3-specific deadlock risk round 2 must hammer (OQ-2).**

## 4. Invariant protection (proof-by-construction)
- **I1:** Invoke→`runCore`; `streamMergeAdapter.Run`/`zipAdapter.Run` gather sorted-key + user combine → deterministic R. Streaming executor never on Invoke path (fact #9). Cross-check per shape (§5).
- **I2:** `fanInMerge` never touches Checkpoint/cursor/`r.engine.*`; non-serializable → suspend unreachable.
- **I3:** sibling-error-wins lives in `runCore`; unreachable from streaming.
- **Oracle preservation:** `runLinearStream` + `runStreamDAG` untouched; the #2 Copy path touched ONLY by §B.3 — gated by ALL existing `streamgraph_test.go`/`copy_test.go` passing byte-identically BEFORE any Merge test.
- **Equivalence-vs-Invoke:** `AddStreamMerge` = MULTISET + count + per-source completeness + N=1 sequence; `AddZip` = SEQUENCE.

## 5. Sub-tasks (TDD, each its own commit)
- **3a — Value-path merge nodes + Invoke (no streaming).** `AddStreamMerge`/`streamMergeAdapter` (sorted-key gather→[]T→combine→R), `AddZip`/`zipAdapter` (sorted-key→[][]T→combine→R). `mergeKind` capability. `markInProcess`. Tests: `TestAddStreamMerge_InvokeGathersSortedDeterministic` (200× byte-identical), `TestAddStreamMerge_SkippedSourceAbsent`, `TestAddZip_InvokeRoundRobinDeterministic`, `TestMerge_HeteroT_BuildRejected`, `TestMerge_EmptySources_BuildRejected`. apisnapshot +2.
- **3b — `roundRobinMerge[any]` reader.** Per-source puller + buf-1 lookahead; deterministic index-order; ragged drain; two-stage Close; N=0→EOF. Tests: `TestRoundRobinMerge_DeterministicOrder` (100 runs), `_RaggedLengths`, `_ErrorPropagates`, 3-mode leak, **`TestRoundRobinMerge_ConcurrentPull_NoDeadlock`** (R-ZIPDEADLOCK gate).
- **3c — classifier fan-in extension (no executor).** Recognize merge; allow `toPort != portIn` iff merge node + valid source key; add `sources` to plan; folder validation. Tests: `TestMergeGraph_IsStreamCapable`, `TestLinearStillCapable_Regression`, `TestBranchDAGStillStreamPlan_Regression`, `TestCopyOnlyStillStreamGraphPlan_Regression`, `TestCombineFanIn_StillDegrades`, `TestMissingCombine_CompileError`, `TestImplicitRawTee_StillDegrades`.
- **3d — `fanInMerge`: standalone Merge (no upstream Copy).** Tests: `TestStreamGraph_StreamMerge_InterleavesTwoStreams` (multiset), `_EquivalenceVsInvoke_Multiset`, `TestStreamGraph_Zip_EquivalenceVsInvoke_Sequence`, `TestStreamGraph_Merge_N1_SequenceEquality`, **`TestStreamGraph_Merge_ConcurrentFanIn_SmallBuffer_NoHang`** (BLOCKER-LAW gate), `_OneSourceErrors_PropagatesAndNoLeak`, 3-mode leak N∈{1,2,3}.
- **3e — the diamond + §B.3 restructure.** Tests: `TestStreamGraph_CopyToMergeDiamond_Completes`, `_EquivalenceVsInvoke`, **`TestStreamGraph_Diamond_BufPlus2_NoDeadlock`** (§C gate), `_{FullDrain,EarlyClose,CloseBeforeNext}_NoLeak`, `_MidWalkError_ClosesAllLiveStreams`. **Regression gate: existing streamgraph/copy tests byte-identical BEFORE new tests.**

Mandatory: `settleGoroutines()` base-vs-after every streaming test; equivalence-vs-Invoke per shape.

## 6. Risk register
| Risk | Sev | Mitigation |
|---|---|---|
| **R-DIAMOND — §B.3 restructure regresses the shipped Copy path** (v2.1.0 fan-out). | **High** | Regression gate (existing tests byte-identical first); minimal change (`fanInMerge` owns source subtrees, `fanOutCopy` only tees); round-2 reviews the diff. |
| **R-ZIPDEADLOCK — round-robin serial pull deadlocks on shared-upstream-Copy** (#2 BLOCKER in zip clothing). | **High** | Per-source puller + buf-1 lookahead, order at output; `TestRoundRobinMerge_ConcurrentPull_NoDeadlock` + Copy→{A,B}→Zip; round-2 scratch-test gate. |
| **R-ZIPLEN — positional equal-length zip hangs on mismatch.** | High (avoided) | MVP zip = round-robin ragged-tolerant; positional zip rejected. `_RaggedLengths` proves no hang. |
| **R1 — unordered multiset weakens cross-check** (drop-A+dup-B cancel). | High | multiset + count + per-source completeness (tag by source) + N=1 sequence. |
| **R2 — N source goroutines + N pullers leak on early Close.** | High | every reader in `liveSet`; root cancel; two-stage Close; 3-mode tests incl. diamond. |
| **R3 — Merge backpressure stalls siblings.** | Med | inherited bounded shared-rate; godoc + characterization test. |
| **R4 — elemT not re-stamped on merged carrier.** | Med | re-stamp from `mergeNode.elemT`; `TestStreamGraph_Merge_ElemTPreserved`. |
| **R5 — classifier mis-allows `AddCombine` as merge.** | Med | allowance gated on `mergeKind`; `TestCombineFanIn_StillDegrades`. |
| **R6 — folder validation for N source boundaries + combine-as-folder bridge.** | Med | Compile folder walk over merge inbound edges; `TestMissingCombine_CompileError`. |

## 7. Open questions for round 2 (hammer these)
1. **(BLOCKER candidate — R-DIAMOND) Is the §B.3 restructure safe, or should the diamond defer?** Routing convergent Copy targets into `fanInMerge` (not `fanOutCopy`'s tail/leaf split) changes v2.1.0-shipped code. Round 2 decides: ship the diamond (restructure) or defer (MVP = Merge-without-upstream-Copy). Scratch-test: restructure doesn't regress Copy fan-out + diamond completes leak-clean at ≥(buf+2) frames.
2. **(BLOCKER candidate — R-ZIPDEADLOCK) Does `roundRobinMerge` deadlock under concurrent pull?** The determinism requirement tempts serial pull (deadlocks on shared-upstream-Copy). Prove the per-source-puller+buf-1-lookahead mitigation leak-clean AND deadlock-free with a scratch test BEFORE code. Is per-source unbounded lookahead an OOM footgun, or buf-1 sufficient?
3. **(Value-path) Is the user-`combine` requirement (Q2) right, or over-burdensome?** Merging two `string` streams forces `func(ctx,[]string)(string,error)`. Ship a `combine==nil` convenience requiring a registered `Concatenator[T]→R` for built-in T (reuse `lookupConcat`), else require combine? Or is explicit combine cleaner (no hidden registry, no register-after-Compile invisibility)?
4. **(Ordered semantics) Round-robin vs positional zip-tuples?** Round-robin interleaves frames; a consumer wanting `(a_i,b_i)` tuples gets a flat stream. Is the value-path `[][]T` sufficient to reconstruct tuples, making stream-path round-robin a transport detail? Confirm value `[][]T` + stream round-robin are equivalent for zip.
5. **(Cross-validator adequacy) Does multiset+count+completeness catch all unordered bugs?** A reorder-WITHIN-a-source (A emits 1,2,3 → merge delivers 1,3,2) passes multiset + completeness. Need per-source *order* preservation as a separate assertion? (`Merge`'s puller is FIFO per source → should hold, but assert it.)

## 8. Deferred (out of MVP, with why)
- Equal-length positional zip-tuples — rejected permanently (hangs on mismatch; round-robin safer).
- Heterogeneous-type Merge — `AddCombine`'s value-only job.
- Streaming combine emitting R incrementally — future; MVP folds to one R.
- Arbitrary copy/merge meshes — MVP = single Copy→{A,B}→Merge; general meshes pending round-2 reachability analysis.
- Merge + checkpoint — no item (live stream ≠ positional cursor).
- Unifying `AddCombine` with `AddStreamMerge` — kept separate (value-only-arbitrary vs homogeneous-T-streaming).
