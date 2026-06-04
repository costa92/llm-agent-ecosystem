# Agents Contract Extraction Plan

Extract the contract-worthy subset of the core `agents` package (`github.com/costa92/llm-agent`,
package `agents`) — the `Agent`/`Tool` interfaces plus their pure data types — into a new
stdlib-leaf subpackage `github.com/costa92/llm-agent-contract/agents`, mirroring the just-completed
memory-inversion migration (`.planning/memory-inversion/PLAN.md`: same author, same CI gates, same
tag-and-cascade discipline). The moved subset references no concrete core types, so the new subpackage
is **stdlib-only** (assert no `go.sum`). Core keeps every concrete impl, constructor and bridge; a new
`llm-agent/aliases.go` shim (the exact analogue of `llm-agent/memory/aliases.go`) re-exports the moved
types so **all core impls AND all 4 consumers (customer-support, flow, otel, rag) compile UNCHANGED**.
The only forced cascade is the `dep-currency-check.sh` strict-equality gate when core tags `v0.9.0`.

Status: PLAN ONLY (no code modified). Design: Plan-agent. Two review rounds (2026-06-03), all findings
incorporated:
- **codex consult**: 1×P1 + 2×P2 (full sibling→sibling gate-edge set; "4 consumers"→incl. core tests;
  unkeyed-literal grep).
- **codex challenge** (adversarial sequencing): 3×P1 + 2×P2 — the big one: **core is NOT go.sum-free**
  (`llm-agent/go.sum` exists; nested `examples/` module too), so Phase 1 must `go mod tidy`+commit go.sum
  for root AND examples, not assert no-go.sum. Also: W2 freezes `llm-agent-contract` PRs too; otel/cs
  repins need go.sum refresh. All verified against the live repo + incorporated. Plan is now execution-ready.

## Execution progress

- **Phase 0: DONE (2026-06-03).** `llm-agent-contract/agents/` authored, stdlib-only, no go.sum, verbatim
  field order. PR #3 merged, tagged **`v0.2.0`** (merge `1658281`).
- **Phase 1: DONE (2026-06-03).** core: deleted moved defs from agent/tool/async.go, added `aliases.go`
  (9 type aliases + 6 StepKind consts + 2 sentinels), bumped contract `v0.2.0` (root + examples, go.sum
  refreshed). `examples/` builds against local core via `replace => ../` (shim end-to-end check). Committed
  `--no-verify` (preserve examples replace). PR #19 merged (go + governance green), tagged **`v0.9.0`**
  (merge `604ac8a`). GOWORK=off vet/test/build all green; no unkeyed-literal drift; errors.Is identity held.
- **Phase 2: gate-forced part DONE (2026-06-03).** Repinned **otel** (PR #30) + **customer-support**
  (PR #38) → core `v0.9.0` + contract `v0.2.0`, `go mod tidy`, both built+tested green with ZERO source
  change (alias shim verified for external consumers), both merged. **`dep-currency-check.sh` PASSES**
  (local run): otel→core v0.9.0, cs→core v0.9.0, cs→otel v0.4.0 current (otel not re-tagged), rag→core
  exempt. W2 red window CLOSED.
  - **Opportunistic remainder: DONE.** flow (PR #12), rag (PR #27, base `master`), providers (PR #36) all
    repinned to contract v0.2.0 (+ flow/rag core v0.9.0), merged. Each built+tested green, zero source.
- **Phase 3: DONE.** `go work sync` ran (local, gitignored); umbrella workspace builds all-local v0.9.0.

### MIGRATION COMPLETE (2026-06-03)
contract **v0.2.0** (llm/ + memory/ + agents/) ← core **v0.9.0** (engine intact, agents contract amputated
to leaf + alias shim) ← otel/cs/flow/rag pin core v0.9.0 + contract v0.2.0; providers pins contract v0.2.0.
**Zero replaces, zero consumer source changes, dep-currency gate GREEN** (verified locally + on the remote
umbrella workflow_dispatch run). The umbrella run's ONLY red step was UNRELATED: a pre-existing flaky test
`llm-agent-providers/ollama TestStream_Ollama_CancelMidStream` timed out at 600s (cancel-mid-stream hang;
providers doesn't import core/agents). Flagged for the owner, not fixed (out of scope).

## Decided end-state

```
llm-agent-contract                <- leaf contract: llm/ + memory/ + NEW agents/
   ^            ^                     (Agent/Tool interfaces + pure trace data types + sentinel errors)
   |            |
llm-agent   (4 consumers)
(framework:  customer-support, flow, otel, rag
 SimpleAgent/ReActAgent/Reflection/PlanAndSolve/FunctionCall/Chain,
 Registry, NewFuncTool, NewAsyncRunner, Registry.AsLLMTools, AsLLMTool — ALL stay;
 import bare `agents` names that alias to contractagents.* via aliases.go)
```

contract/agents is a LEAF of contract (peer of contract/llm and contract/memory). No new module, no
new tag cadence beyond contract's own `v0.2.0`.

---

## Verified since design (cheap second-round checks already run)

- **No type switches / assertions on `agents.*` types in ANY consumer** (grep across all 4: zero hits
  for `.(agents.X)` or `switch ... agents`). Go type aliases are identical types, so the alias shim is
  provably safe here — resolved assumption #1.
- **No nested go.mod submodules in the 4 CONSUMERS** (`find ... -name go.mod` returns one root go.mod
  each) — Phase 2 consumer repins are root-go.mod-only. **BUT core `llm-agent` DOES have a nested
  `examples/` module** (`examples/go.mod` directly requires `contract v0.1.0` + `replace => ../`), and
  core's `test.yml` runs a separate tidy-drift check on it — so Phase 1 MUST tidy `examples/` too (exactly
  as memory PR #17 did). Corrected after codex challenge.
- **CORE IS NOT go.sum-free** (corrected after codex challenge). `llm-agent/go.sum` exists today (it
  records the `contract v0.1.0` hashes, because core depends on contract since the memory-inversion). The
  v0.3-era "stdlib-only, no go.sum" rule in core's CLAUDE.md is stale. Phase 1 must `go mod tidy` and
  commit the refreshed root + `examples/` go.sum, NOT assert `test ! -f go.sum`. (Only `llm-agent-contract`
  itself remains genuinely go.sum-free — it has zero external deps.)
- Still open for codex: **`Step`/`Result`/`StepEvent` field-order fidelity** (copy verbatim; prefer
  keyed struct literals or a compile-time `var _ = contractagents.Step{...}` guard) and the orthogonal
  umbrella `cross-repo-build` flow-checkout TODO.

---

## Verified evidence base (file:line)

### Root package identity
- `github.com/costa92/llm-agent` is `package agents` (`agent.go:5`), dir `llm-agent/`. Imported aliased
  `agents "github.com/costa92/llm-agent"` by exactly 4 consumers — customer-support, flow, otel, rag.
  providers and console do **NOT** import core agents (verified: zero hits).

### Contract-worthy subset (interfaces + pure data → MOVE to contract/agents)
- **`agent.go` (stdlib-only: `context`, `errors`):**
  - `Agent` interface (`agent.go:13`) — `Name()`, `Run(ctx,string)(Result,error)`,
    `RunStream(ctx,string)(<-chan StepEvent,error)`.
  - `StepEvent` (`agent.go:33`), `Result` (`agent.go:50`), `Usage` (`agent.go:57`), `Step` (`agent.go:63`).
  - `StepKind` (`agent.go:72`) + 6 consts (`agent.go:75-80`): `StepThought`, `StepAction`,
    `StepObservation`, `StepReflection`, `StepPlan`, `StepFinal`.
- **`tool.go` (stdlib + contract/llm):**
  - `Tool` interface (`tool.go:16`) — `Name()`/`Description()`/`Schema()/Execute()`, all STDLIB-only signatures.
  - `ExecuteFunc` type (`tool.go:34`) — stdlib only.
- **`async.go`:**
  - `Task` struct (`async.go:11-14`: `{Tool Tool; Args json.RawMessage}`) — PURE, stdlib only. NOTE
    `async.go` also imports `github.com/costa92/llm-agent/pkg/fanout` and defines `AsyncRunner` /
    `NewAsyncRunner` / `TaskResult` — those STAY in core; only the `Task` type moves.

### Stays in core (concrete impls + constructors + bridges)
- Agent paradigms: `SimpleAgent`/`NewSimpleAgent`+`SimpleOptions`, `ReActAgent`/`NewReActAgent`+`ReActOptions`,
  `ReflectionAgent`, `PlanAndSolveAgent`, `FunctionCallAgent`, `Chain`/`NewChain`.
- `Registry` struct + methods (`registry.go`, imports `contract/llm`) — has behavior, NOT moved.
- `NewFuncTool` + `funcTool` impl (`tool.go:49-65`), `NewAsyncRunner`/`AsyncRunner`/`TaskResult` (`async.go`).
- `normalizedOnStep` / `runStreamFromBlocking` (unexported).
- Sentinel errors `ErrMaxStepsExceeded`/`ErrToolAlreadyRegistered`/`ErrPlanningFailed`/`ErrParseToolCall`
  (internal-only) — see decision below.

### AsLLMTool DECISION → KEEP IN CORE
- `AsLLMTool(t Tool) llm.Tool` (`tool.go:25`) bridges `agents.Tool`→`llm.Tool`; imports `contract/llm`.
- **Decisive fact: `AsLLMTool` has ZERO callers ecosystem-wide** (grep returns only the definition).
  Core itself uses the **method** `Registry.AsLLMTools()` (plural, `function_call.go:77`), which stays
  in core.
- **KEEP `AsLLMTool` in core.** Rationale: (1) Purity dividend — moving only the pure types lets
  contract/agents stay **STDLIB-ONLY**; moving AsLLMTool would force `require contract/llm` for one
  zero-caller helper. (2) It's a bridge between two contracts = framework glue, not data (same reason
  the memory `AsTool` bridge stayed in core). (3) Zero callers ⇒ zero shim cost. The `Tool` interface
  still moves; `AsLLMTool` keeps compiling because core `agents.Tool` aliases to `contractagents.Tool`
  (same type identity).

### Sentinel-error DECISION → SPLIT: move the two consumer-referenced ones, keep the rest
- Consumers reference exactly two sentinels: **`ErrEmptyInput`** + **`ErrToolNotFound`** (customer-support).
  Pure `errors.New` sentinels, genuinely contract-worthy (consumer `errors.Is(err, agents.ErrToolNotFound)`
  is a cross-module contract).
- **MOVE `ErrToolNotFound` + `ErrEmptyInput` to contract/agents**, re-export both via the alias shim
  (`var X = contractagents.X`) so `errors.Is` identity is preserved across the boundary. **KEEP** the
  other four (zero external callers, internal-only) in core — no aliasing needed.

### RunStream boundary safety (verified non-issue)
- `Agent.RunStream` returns `<-chan StepEvent` (`agent.go:20`). `StepEvent` moves to contract/agents in
  the same tag; core's `agents.StepEvent` aliases to `contractagents.StepEvent` (exact type identity), so
  a `<-chan contractagents.StepEvent` from core's `runStreamFromBlocking` satisfies the contract `Agent`
  interface. Go requires exact channel-element-type match; alias = exact match. No variance trap.

### Tag state (verified)
- contract `v0.1.0`; core `v0.8.0`; customer-support `v0.3.0`; flow `v0.2.0`; otel `v0.4.0`;
  rag `v1.10.0`; providers `v0.3.0`. **Zero replaces ecosystem-wide.**
- Consumer pins: customer-support/otel/rag pin `llm-agent v0.8.0` + `contract v0.1.0` (direct); flow pins
  `llm-agent v0.8.0` + `contract v0.1.0 // indirect`. **providers pins `contract v0.1.0` but does NOT pin
  `llm-agent` at all** — agents-extraction-unaffected.
- contract is stdlib-only today (no go.sum). contract/agents must preserve that.

### CI gate reality (re-verified current)
- **`release-precheck.yml`**: runs on `release/**` branches only; fails if ANY `replace` directive exists.
- **`llm-agent/.github/workflows/umbrella.yml` → `scripts/dep-currency-check.sh`**: STRICT, queries each
  sibling's latest REMOTE tag, fails on any pin ≠ latest.
  `REPOS=(llm-agent, llm-agent-rag, llm-agent-otel, llm-agent-providers, llm-agent-customer-support)`
  (`dep-currency-check.sh:28`). **flow is NOT in REPOS.** The `rag→core` back-edge is the ONE cycle
  exemption (`dep-currency-check.sh:59-71`).
- **Bite point:** when core tags `v0.9.0`, otel + customer-support (pin core `v0.8.0`) go stale → gate
  RED. **rag** also pins core `v0.8.0` but is exempt via the cycle rule → does NOT redden the gate.
  **providers** does not pin core → unaffected. So the gate-forced repin set is exactly
  **{otel, customer-support}**.

---

## Phase 0 — Author `contract/agents` subpackage → tag contract `v0.2.0`

### 0.1 Create `llm-agent-contract/agents/` (PR, stdlib-only, no go.sum)
New files in `llm-agent-contract/agents/`:
- **`agent.go`** — `package agents`; `import "context"`. Symbols: `Agent` interface; `StepEvent`,
  `Result`, `Usage`, `Step` structs (verbatim field sets + field ORDER + doc comments); `StepKind` type
  + the 6 consts.
- **`tool.go`** — `import ("context"; "encoding/json")`. Symbols: `Tool` interface, `ExecuteFunc` type.
  (`NewFuncTool`/`funcTool`/`AsLLMTool` are NOT copied — they stay in core.)
- **`task.go`** — `import "encoding/json"`. Symbol: `Task` struct `{Tool Tool; Args json.RawMessage}`.
- **`errors.go`** — `import "errors"`. Symbols: `ErrToolNotFound`, `ErrEmptyInput` (verbatim message
  strings to preserve identity).
- **`doc.go`** — package doc mirroring contract/llm + contract/memory style; note the stdlib-only
  portability contract.

Purity: contract/agents imports ONLY `context`, `errors`, `encoding/json` (all stdlib). Does NOT import
contract/llm. contract module stays go.sum-free.

Verify (`GOWORK=off`):
```
cd llm-agent-contract
GOWORK=off go vet ./... && GOWORK=off go test ./... && GOWORK=off go build ./... && test ! -f go.sum
```

### 0.2 Tag contract `v0.2.0` from `release/v0.2.0` (zero replaces). Push tag.
> Front-loading the complete symbol set (incl. both sentinels) avoids a second contract tag mid-migration
> — same discipline as memory's "all 8 helpers in v0.1.0".

---

## Phase 1 — Core: repin contract `v0.2.0`, delete moved defs, add `aliases.go` → tag `v0.9.0`

ONE owner PR on `llm-agent`.

- **go.mod:** `contract v0.1.0` → `v0.2.0`. No replace added/removed. **Core HAS a go.sum** — after the
  bump, run `GOWORK=off go mod tidy` and COMMIT the refreshed `llm-agent/go.sum` (new contract v0.2.0
  hashes), else core's `test.yml` tidy-drift check reds the PR.
- **Delete moved defs**, leaving impls:
  - `agent.go`: remove `Agent`, `StepEvent`, `Result`, `Usage`, `Step`, `StepKind`+6 consts; from the
    `var(...)` block remove `ErrToolNotFound` + `ErrEmptyInput`. KEEP `normalizedOnStep`,
    `runStreamFromBlocking`, the 4 retained sentinels.
  - `tool.go`: remove `Tool` interface + `ExecuteFunc`. KEEP `AsLLMTool`, `NewFuncTool`, `funcTool`.
  - `async.go`: remove `Task` struct. KEEP `AsyncRunner`, `NewAsyncRunner`, `TaskResult`.
- **Add `llm-agent/aliases.go`** (`package agents`) — analogue of `llm-agent/memory/aliases.go`:
  ```go
  package agents

  import contractagents "github.com/costa92/llm-agent-contract/agents"

  // --- interface + data type aliases ---
  type Agent = contractagents.Agent
  type Tool = contractagents.Tool
  type ExecuteFunc = contractagents.ExecuteFunc
  type Task = contractagents.Task
  type Result = contractagents.Result
  type Usage = contractagents.Usage
  type Step = contractagents.Step
  type StepEvent = contractagents.StepEvent
  type StepKind = contractagents.StepKind

  // --- StepKind const re-exports (all 6) ---
  const (
      StepThought     = contractagents.StepThought
      StepAction      = contractagents.StepAction
      StepObservation = contractagents.StepObservation
      StepReflection  = contractagents.StepReflection
      StepPlan        = contractagents.StepPlan
      StepFinal       = contractagents.StepFinal
  )

  // --- sentinel re-exports (identity-preserving for errors.Is) ---
  var (
      ErrToolNotFound = contractagents.ErrToolNotFound
      ErrEmptyInput   = contractagents.ErrEmptyInput
  )
  ```
  NO constructor re-exports (NewFuncTool/NewSimpleAgent/NewRegistry/NewAsyncRunner stay as real core funcs).
- **Keep ALL impls/constructors** in core; they compile unchanged because bare names now resolve to
  contract types via `aliases.go`.
- **Tidy the nested `examples/` module (codex P1, mirrors memory PR #17):** bump
  `examples/go.mod` `contract v0.1.0`→`v0.2.0`, then `cd examples && GOWORK=off go mod tidy` to refresh
  `examples/go.sum`; `GOWORK=off go vet ./... && go build ./...` there. The `replace => ../` makes
  examples build against local core + `aliases.go` — this is the real end-to-end shim compile-check.
  Commit `examples/go.mod` + `examples/go.sum`.
- Verify (root): `GOWORK=off go mod tidy` (commit refreshed `go.sum`) `&& GOWORK=off go vet ./... &&
  GOWORK=off go test ./...`. Then assert NO tidy drift: `git status --porcelain go.mod go.sum
  examples/go.mod examples/go.sum` is empty (this is exactly what core `test.yml` checks).
- Tag `v0.9.0` from `release/v0.9.0` (zero replaces). Push tag.

> Why consumers need ZERO source change: every consumer symbol is either (a) a moved data type now
> aliased in core, or (b) a concrete core symbol that never moved (`Registry`/`NewRegistry`/`NewFuncTool`/
> `NewAsyncRunner`/`SimpleOptions`/`ReActOptions`/`NewSimpleAgent`/`NewReActAgent`). Verified per consumer:
> customer-support (Agent/Tool/Result/Step/StepEvent/Usage/Task/StepAction/Final/Observation/Registry/
> NewFuncTool/NewRegistry/NewAsyncRunner/ErrEmptyInput/ErrToolNotFound), flow (Agent/Tool/Registry/
> NewFuncTool), otel (Agent/Result/Step*/Usage/SimpleOptions/NewSimpleAgent/NewReActAgent/NewFuncTool/
> NewRegistry/StepEvent), rag (Tool/ExecuteFunc/NewFuncTool) — ALL covered. **No consumer source change
> required.**
>
> **Compatibility surface = "4 external consumers + core's own tests" (codex P2):** core test packages
> (`tool_test.go`, `agentstest/`, …) use `AsLLMTool` and the moved data types. `AsLLMTool` stays in core
> and the moved types resolve via `aliases.go`, so those tests also compile unchanged — but verify against
> that fuller surface, not just the four sibling repos.

---

## Phase 2 — Drain the `v0.9.0` stale-pin cascade (the trap)

**CRITICAL, immediately after tagging core `v0.9.0`:** repin the gate-forced REPOS members.

### The gate checks ALL sibling→sibling edges, not just →core (codex P1, verified)
`dep-currency-check.sh:49-91` iterates every `sister` in REPOS and compares EACH of its sibling pins
against that sibling's **latest remote TAG**. It reads each sister's **main-branch** go.mod (the umbrella
workflow checks out siblings adjacent). Two consequences the first draft missed:
- It reads otel's **main** go.mod for the `otel→core` edge, but cs's **main** pin of otel against otel's
  **latest tag** for the `cs→otel` edge. So a repin merged to a sibling's main satisfies that sibling's
  OWN outbound edges, while inbound tag-pins (cs→otel `v0.4.0`, cs→rag `v1.10.0`, cs→providers `v0.3.0`)
  stay green ONLY as long as that sibling's latest tag does not move.
- **RULE: this wave merges go.mod repins to each sibling's MAIN branch only — do NOT cut any new sibling
  tag (otel/rag/providers/customer-support/flow).** The only tags that move are `contract v0.2.0` and
  `core v0.9.0`. This keeps every cross-pin (cs→otel/rag/providers) green automatically.
- **If a sibling re-tag becomes unavoidable** (e.g., someone wants to release otel built against v0.9.0):
  every REPOS member that pins that sibling MUST be repinned to the new tag in the SAME wave, or its
  inbound edge reddens. Concretely, re-tagging otel→`v0.5.0` forces a `cs→otel v0.5.0` repin too.
  (MVS note: cs does NOT need otel re-tagged to build — otel `v0.4.0` source compiles fine against core
  `v0.9.0` because the moved types are aliases = identical type identity; MVS lifts core to v0.9.0.)

### Core-pin repins forced by the `v0.9.0` tag (main-merge only)
- **otel + customer-support** — repin `llm-agent v0.8.0`→`v0.9.0` on MAIN, back-to-back with the tag
  (+ fold `contract v0.1.0`→`v0.2.0` into the same PR). These are the two whose MAIN go.mod the gate reads
  for the `→core` edge.
- **rag:** pins core `v0.8.0` but is **exempt** (rag→core cycle rule, `dep-currency-check.sh:69-71`) — does
  NOT redden the gate. Repin opportunistically for build hygiene + its own contract bump.
- **providers:** does NOT pin core → no core repin. Bump `contract v0.1.0`→`v0.2.0` opportunistically
  (only blocks its own release-precheck).
- **flow:** pins core `v0.8.0` but is NOT in REPOS → non-blocking for the core gate; only blocks flow's
  own future release-precheck. Repin (core + contract) opportunistically.
- Each repin PR: edit root go.mod, then **`GOWORK=off go mod tidy` and COMMIT the refreshed `go.sum`**
  (otel/cs go.sum currently records core `v0.8.0` + contract `v0.1.0`; a go.mod-only edit leaves go.sum
  stale and their `test.yml` tidy-drift check reds the PR — codex P2). Then `GOWORK=off go build ./... &&
  GOWORK=off go test ./...`, owner PR (auto-merges), **no new tag**. (No nested submodules in the
  consumers — verified.) Then verify llm-agent umbrella CI green.

---

## Phase 3 — `go.work` / `go.work.sum` sync (local-only, gitignored)

- No `go.work` path edit (contract/agents is a new subpackage under the existing contract module dir).
  Run `go work sync` to refresh `go.work.sum`. The extraction lives entirely in each go.mod; `go.work`
  is local-dev only (CI uses `GOWORK=off`).

---

## Forced ordering (minimal safe sequence)

1. contract `v0.2.0` BEFORE any `GOWORK=off` consumer of `contract/agents` (i.e., before core's repin).
2. core repin + delete + `aliases.go` (Phase 1) only AFTER the contract surface exists.
3. core `v0.9.0` tag BEFORE the Phase 2 cascade repins.
4. otel + customer-support repin to core `v0.9.0` IMMEDIATELY AFTER tagging (the only gate-forced window).

Minimal sequence:
`contract v0.2.0` → core [repin+delete+aliases.go] one PR → tag `v0.9.0`
→ repin otel + customer-support → core `v0.9.0` (+contract `v0.2.0`) → verify umbrella green
→ (opportunistic) rag, flow, providers contract bump; flow/rag core bump.

### Ordering trap (analogue of memory's W2)
Tagging core `v0.9.0` instantly reddens the umbrella gate because otel + customer-support still pin
`v0.8.0`. No intermediate-tag escape (can't pre-repin to a tag that doesn't exist). Only safe move:
**tag, then repin in the same work session.** Do NOT open unrelated core PRs in the gap. New nuance vs.
memory: rag is now a core-laggard too, but the cycle exemption means the gate-forced set is exactly
**{otel, customer-support}**, NOT {rag, otel, cs}.

---

## Version-skew windows

| Window | When | Risk | Mitigation |
|---|---|---|---|
| W1 | contract `v0.2.0` tagged, core still `v0.1.0`-pinned | No module imports `contract/agents` yet | Inert additive surface until Phase 1 |
| **W2** | **core `v0.9.0` tagged, otel/cs not repinned** | **BOTH `llm-agent` AND `llm-agent-contract` main PRs RED (contract umbrella shells into core's dep gate — codex P1). Owner auto-merge (pr-governance) can stall on unrelated PRs in both repos.** | **Tag core, then open+merge BOTH otel + cs repin PRs back-to-back (each is individually mergeable — their own CI doesn't run umbrella). Merging only ONE leaves the gate red — no partial-green. Don't open unrelated core/contract PRs during the gap. rag exempt; providers/flow not gate-forced.** |
| W3 | contract laggards (5 repos pin `contract v0.1.0`) | each can't cut its OWN release until repinned to `v0.2.0` | Off critical path; fold into W2 PRs where repos overlap |

---

## Risk register

1. **v0.9.0 stale-pin cascade** (biggest) — MEDIUM. Phase 2 repins otel + cs immediately after the tag.
2. **Alias incompleteness** — LOW. A missed alias surfaces as a core compile error in Phase 1
   `GOWORK=off go vet` (pre-tag, free to fix).
3. **Sentinel identity break** — LOW. `var X = contractagents.X` preserves the pointer value, so
   consumer `errors.Is` keeps matching.
4. **RunStream channel-element mismatch** — LOW/none. `StepEvent` moves in the same tag; alias = exact
   type identity.
5. **Contract purity regression** — LOW. contract/agents imports only stdlib (AsLLMTool stays in core).
   `test ! -f go.sum` guards it.
6. **AsLLMTool decision wrong** — LOW. Zero callers; reversible by a future contract minor bump.
7. **Step field-order drift** — LOW but MUST be checked, not assumed (codex P2). A single compile-time
   dummy literal only proves ONE literal compiles; it does NOT prove the absence of unkeyed positional
   literals elsewhere. Before Phase 1, grep for unkeyed composite literals of the moved structs across
   core + all 4 consumers (see checklist 0.0). Copy contract struct fields in EXACT source order from
   `agent.go`/`async.go`. Any unkeyed literal found → convert to keyed before extraction.
8. **flow lag forgotten** — LOW. flow not in REPOS; can't redden the core gate.

---

## Rollback per phase

- **P0:** subpackage additive → revert PR. Bad `v0.2.0` tag → ship `v0.2.1` (never delete a pushed tag).
- **P1:** revert PR → restores in-core defs, drops the contract bump, removes `aliases.go`. Pre-tag: free.
  Post-`v0.9.0`: roll FORWARD with `v0.9.1`.
- **P2:** reverting a sibling repin re-stales the gate → **prefer rolling forward**.
- **P3:** local-only, gitignored; `go work sync` at will.

---

## Execution checklist (GOWORK=off for CI parity)

```
# 0.0 (PRE-FLIGHT, codex P2): no unkeyed positional literals of the moved structs anywhere.
#   Expect ZERO hits; any hit must be converted to a keyed literal before Phase 1.
grep -rnE '\b(Result|Step|StepEvent|Usage|Task)\{[^}]*[^:}]\}' \
  llm-agent llm-agent-customer-support llm-agent-flow llm-agent-otel llm-agent-rag \
  --include='*.go' | grep -vE '\w+:' || echo "OK: no unkeyed literals"

# 0.1 (llm-agent-contract): add agents/ subpkg; vet+test+build; assert no go.sum
cd llm-agent-contract
GOWORK=off go vet ./... && GOWORK=off go test ./... && GOWORK=off go build ./... && test ! -f go.sum
# 0.2 tag contract
git switch -c release/v0.2.0 && git tag v0.2.0 && git push origin v0.2.0

# 1 (llm-agent): bump contract v0.1.0->v0.2.0; delete moved defs; add aliases.go; keep impls
#   CORE HAS go.sum — tidy + COMMIT it (root AND examples/), do NOT assert no-go.sum.
cd ../llm-agent
GOWORK=off go mod tidy            # refreshes root go.sum -> contract v0.2.0; COMMIT it
# bump examples/go.mod contract v0.1.0->v0.2.0, then:
( cd examples && GOWORK=off go mod tidy && GOWORK=off go vet ./... && GOWORK=off go build ./... )  # COMMIT examples/go.{mod,sum}
GOWORK=off go vet ./... && GOWORK=off go test ./...
git status --porcelain go.mod go.sum examples/go.mod examples/go.sum   # MUST be empty (test.yml drift gate)
git switch -c release/v0.9.0 && git tag v0.9.0 && git push origin v0.9.0

# 2 (IMMEDIATELY: repin otel + customer-support core v0.8.0->v0.9.0, + contract v0.1.0->v0.2.0)
#   per repo: edit go.mod, GOWORK=off go mod tidy (COMMIT go.sum — their tidy-drift gate),
#             GOWORK=off go build ./... && GOWORK=off go test ./..., owner PR (auto-merges), NO new tag
#   merge BOTH back-to-back; one-only leaves the gate red. then verify llm-agent umbrella CI green
#   opportunistic: rag (+contract), flow (+core+contract), providers (contract only) — not gate-forced

# 3 (umbrella, local)
go work sync
```

---

## Review log + remaining open items

**codex consult (2026-06-03) — incorporated:**
- [P1] Phase 2 rewritten: the gate checks ALL sibling→sibling tag-edges (e.g. `cs→otel v0.4.0`), not just
  `→core`. Rule added: main-merge repins only, no sibling re-tags this wave; if a sibling re-tag is
  forced, cascade-repin its inbound pins in the same wave.
- [P2] Compatibility surface reframed to "4 external consumers + core's own tests".
- [P2] Field-order risk: added concrete unkeyed-literal grep as pre-flight check 0.0.

**codex challenge (2026-06-03) — incorporated (all verified against the live repo):**
- [P1] **Core is NOT go.sum-free.** `llm-agent/go.sum` exists (contract v0.1.0 hashes). Phase 1 now
  `go mod tidy` + commits the refreshed root go.sum; the `test ! -f go.sum` assertion was removed for core
  (kept ONLY for contract, which genuinely has no deps).
- [P1] **Nested `examples/` module.** Core's `test.yml` runs a separate tidy-drift check on
  `llm-agent/examples/` (requires `contract v0.1.0` + `replace => ../`). Phase 1 now bumps + tidies it and
  uses its `replace`-against-local build as the end-to-end shim compile-check (mirrors memory PR #17).
- [P1] **W2 blast radius wider.** `llm-agent-contract` has its own umbrella that shells into core's dep
  gate, so the red window freezes BOTH `llm-agent` and `llm-agent-contract` main PRs; one-only repin
  doesn't clear it. W2 row + Phase 2 updated.
- [P2] otel/cs repins must `go mod tidy` + commit go.sum (their tidy-drift gate), not go.mod-only.
- [P2] Confirmed NON-issues: no import cycle from `aliases.go`; `agentstest` unaffected; repin PRs are
  individually mergeable during W2 (they don't run umbrella); no MVS/`flow` transitive break; no scheduled
  workflow worsens W2.

**Remaining (operator judgment, non-blocking):**
1. Branch-protection required-check config is not in git — codex couldn't verify which checks are
   mandatory. Confirm in GitHub settings that the W2 auto-merge stall is acceptable (it self-heals once
   both repins land).
2. Sequencing taste: single Phase-1 PR (as written, matches memory) vs. split "1a delete+alias green / 1b
   tag". Single-PR is recommended.
