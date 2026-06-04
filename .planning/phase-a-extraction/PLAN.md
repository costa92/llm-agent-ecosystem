# Phase A Extraction Plan: policy / comm / builtin → standalone sibling repos

Slim the core `llm-agent` module by relocating three **leaf, contract-consuming** packages
into their own sibling repos, mirroring how `llm-agent-otel` / `llm-agent-providers` already
live outside core. Direction is the inverse of agents-extraction (`.planning/agents-extraction/PLAN.md`):
that moved a contract-worthy subset *into* `llm-agent-contract`; this moves *implementation* packages
*out* of core into peer repos that depend on contract (and, where unavoidable, on core).

Goal alignment (PROJECT.md core value): "core stays stdlib-only minimal; providers, telemetry, and
reference services live in sister repos so users opt into deps one package at a time." policy
(ChatModel decorator), comm (inter-agent protocols), builtin (tool library) are exactly such opt-ins.

Status: **EXECUTED — builtin + policy + comm(option D) ALL DONE 2026-06-04.**

### comm (option D) — DONE 2026-06-04
`llm-agent-comm v0.1.0` = comm base + a2a + mcp (PR #1 merged). Core PR #23 merged: base/a2a/mcp removed,
**anp kept in core**. Pure contract-only leaf: vendored `internal/functool` (replaces core
`agents.NewFuncTool`) + copied `internal/testenv`. Chosen over all-of-comm (option A) after codex 2nd
review: anp self-documents as learning-scope, and verification showed a2a/anp don't import the comm base
(only mcp does) + anp depends only on core `NewFuncTool` → clean to leave in core. cs→providers drift
fixed separately (cs PR #39). Memory-family currency drift (memory→contract, gateway→rag) remains —
pre-existing, out of scope, tied to in-flight memory-inversion.


### Execution progress (2026-06-04)
- **A.0 prep:** `builtin/terminal.go` repointed root→`contract/agents` (1-line, verified green) — folded into A.1.
- **A.1 builtin: DONE.** `llm-agent-builtin v0.1.0` (PR #1 merged) — Calculator/Note/Search/Terminal,
  contract-only leaf, go.sum pinned contract v0.2.0. Core PR #21 merged: `builtin/` removed,
  `example_tool_use_test.go` rewritten with an inline FuncTool (keeps core's tool-use doc example
  dependency-free), examples (02-tool-use, 10-ollama-tools) repinned to `llm-agent-builtin v0.1.0`.
- **A.2 policy: DONE.** `llm-agent-policy v0.1.0` (PR #1 merged) — PII/injection/length gates,
  contract-only production, test-only `require core v0.9.0` for the relocated `integration_test.go`.
  Core PR #22 merged: `policy/` removed, `orchestrate/supervisor_budget_test.go` rewritten with an inline
  `blockOnNthModel` decorator + local `errBlocked` (preserves ctx-propagation + budget-beats-block coverage;
  `BudgetBeatsPolicy` hits==0 still holds), examples/07-policy repinned to `llm-agent-policy v0.1.0`.
- **Core NOT re-tagged (deliberate):** core main is ahead of v0.9.0 with both packages removed, but a new
  tag isn't cut — that avoids forcing the otel/cs/flow/rag/policy repin cascade for a change that affects
  no consumer (zero external importers). Cutting `core v0.10.0` + cascade is a follow-up when a release is wanted.
- **Open / not mine:** umbrella CI `cross-repo-build` is RED on a PRE-EXISTING drift unrelated to Phase A —
  `llm-agent-customer-support` pins `llm-agent-providers v0.3.0` but latest is `v0.3.1`. Required PR checks
  (go+governance) were green; merged past the non-required gate.
- **Umbrella registration** (go.work/.gitignore/eco.sh/cmd/depcheck for both new repos) is applied in the
  working tree but UNCOMMITTED — bundled with the user's pre-existing feat/add-subproject-script edits; left
  for the user to commit. Manual follow-ups still open: umbrella README roster rows + dep-graph edges.

Design synthesized from a Plan-agent split analysis + direct fact verification, hardened by codex review.
**Hard gate: `make add-subproject` creates a PUBLIC GitHub repo + pushes immediately — that step is
outward-facing and requires explicit user go-ahead before running.**

### codex review outcomes (2026-06-04)
- **D1 → option (b):** comm vendors its own ~12-line funcTool helper; do NOT move `NewFuncTool` into
  contract. contract/agents/doc.go:7 documents constructors stay in core; moving it changes the published
  contract story + forces consumer repins. (Moot for Phase A now — comm deferred.)
- **D2 → move tests, don't delete coverage.** policy's OWN `policy/integration_test.go:36` imports root +
  budget (broader than just supervisor_budget_test.go). Move policy integration tests + the root-level
  `example_tool_use_test.go` WITH their packages. The supervisor+policy precedence cases
  (`supervisor_budget_test.go:129/144/158`) test a real core+policy contract — relocate as a core-importing
  integration test in the policy repo (acyclic: core doesn't import policy), do NOT stub them out.
- **D3 → DEFER comm to Phase B.** comm is the worst candidate: `NewFuncTool` coupling in all three
  a2a/anp/mcp tool.go + `internal/testenv` leakage in `comm/comm_test.go:16`, `comm/a2a/a2a_test.go:12`,
  `comm/a2a/server_cancel_test.go:14`. Ship builtin + policy first.
- **CI gate correction:** there is NO `scripts/dep-currency-check.sh`. The gate is `cmd/depcheck/main.go`
  — it reads LATEST LOCAL git tags (`git tag --sort=-v:refname`, `main.go:275`), NOT remote. So currency
  is judged against local sibling clones; tag locally before relying on the gate.
- **examples→leaf edges (was handwaved):** `examples/02-tool-use/main.go:21` imports builtin,
  `examples/07-policy/main.go:32` imports policy, `examples/10-ollama-tools/main.go:41` imports builtin.
  After extraction `examples/go.mod` MUST add direct `require` + dev `replace` for each new leaf repo.
- **Pre-existing roster skew (flag to owner, not this plan's fix):** `go.work` lists memory repos absent
  from `cmd/depcheck` repoList (`go.work:9` vs `main.go:32`).

---

## Decided end-state

```
llm-agent-contract  (leaf: llm/ + memory/ + agents/)   [+ maybe agents.NewFuncTool — see Decision D1]
   ^         ^          ^
   |         |          |
llm-agent   llm-agent-policy   llm-agent-comm   llm-agent-builtin
(core,     (ChatModel          (a2a/anp/mcp      (Calculator/HTTP/
 leaner)    decorator)          + envelope)       Terminal tools)
```

Each new repo: own go.mod (`module github.com/costa92/llm-agent-<name>`), own tags, own CI (4 verbatim
workflows + self-only umbrella.yml), gitignored from umbrella, registered in go.work / eco.sh /
cmd/depcheck. They `require llm-agent-contract` (latest tag). They do **NOT** get imported by core
production code (verified: core has zero non-test reverse-deps on them), so core's go.mod stays clean
of new *production* deps.

---

## Verified evidence base (file:line, GOWORK=off, 2026-06-04)

### Zero external cascade (decisive de-risk)
- **No sibling repo imports `llm-agent/policy`, `/comm`(+a2a/anp/mcp), or `/builtin`** — grep across all
  13 siblings: zero hits. So extraction forces **no consumer repin** (unlike agents-extraction). The only
  importers are inside core itself (examples submodule + a few core test files).

### policy — clean contract-only leaf, test-coupling to resolve
- `go list ./policy` non-test imports: **only `contract/llm`**. The `llm-agent` + `llm-agent/budget`
  imports are **TEST-only** (`go list TestImports`). Production code is a pure `llm.ChatModel` decorator.
- Internal consumers: `orchestrate/supervisor_budget_test.go:14` (core test) and `examples/07-policy/main.go`.

### builtin — true zero-friction
- Only root-package coupling: `builtin/terminal.go:14` imports `github.com/costa92/llm-agent` purely for
  the **`agents.Tool` interface** (compile-time assertion `var _ agents.Tool = (*TerminalTool)(nil)`,
  `terminal.go:201`). `agents.Tool` IS in `contract/agents` → repoint import to `contract/agents`, done.
- Internal consumers: `example_tool_use_test.go:9` (core ROOT-module test), `examples/02-tool-use`,
  `examples/10-ollama-tools`.

### comm — BLOCKED on a core constructor (the load-bearing gotcha)
- `comm/a2a/tool.go:8`, `comm/anp/tool.go:9`, `comm/mcp/tool.go:8` all import root pkg and call
  **`agents.NewFuncTool(...)`** to wrap remote skills/tools.
- `NewFuncTool` is defined in **core `tool.go:37`** and `aliases.go` (lines 6/11) **explicitly keeps it
  in core — it is NOT re-exported to contract/agents.** contract/agents exports the *types* it needs
  (`Tool`, `ExecuteFunc`) but not the constructor.
- ⇒ comm cannot become a contract-only leaf by a bare import repoint. See Decision D1.
- comm base pkg imports `internal/testenv` **in tests** → internal pkgs can't cross module boundaries,
  so the comm repo needs its own copy of the listen helper (`internal/testenv/listen.go`).

### Tooling reality
- `scripts/add-subproject.sh` (`make add-subproject NAME=llm-agent-foo`): `gh repo create --public`,
  scaffolds go.mod + 4 workflows + self-only umbrella.yml, **pushes**, applies branch protection
  (strict, required `go`+`governance`, enforce_admins), then edits go.work / .gitignore / eco.sh /
  cmd/depcheck. Idempotent. Prints a manual checklist (README roster, umbrella cross-build edges,
  consumer require/replace wiring) — NOT automated.
- Replace-guard pre-commit hook strips local `replace` on commit → use `git commit --no-verify` while
  developing against local `replace => ../llm-agent-<x>`.
- dep-currency-check (`scripts/dep-currency-check.sh`, run by core's umbrella.yml) is STRICT: every pin
  must equal the sibling's latest remote tag. add-subproject adds the new repo to `cmd/depcheck`
  repoList; ensure new repos pin **latest contract tag** or the gate goes red.

---

## Decisions to lock in review

**D1 — How comm gets `NewFuncTool` without depending on core.** Options:
  - **(a) Move `NewFuncTool` (+ unexported `funcTool` impl, `tool.go:37-65`) into `contract/agents`**,
    re-export from core via alias (same pattern agents-extraction used for types). Cleanest architecturally
    (ExecuteFunc + Tool already live there); costs a contract PR + retag (`v0.2.1`/`v0.3.0`) + core alias +
    repin cascade for the agents-extraction consumers. *Heavier.*
  - **(b) comm repo vendors a tiny internal `funcTool` helper** (~12 lines implementing `contract/agents.Tool`
    over an `ExecuteFunc`). comm then depends ONLY on contract/agents. No contract change, no cascade.
    Mild duplication. *Simpler / more surgical — aligns with "简单优先".*
  - **Recommendation: (b)** unless we want NewFuncTool to be a first-class contract primitive for other
    future leaves. Flag to codex.

**D2 — Where do the core test files that exercise extracted pkgs go?** After extraction, three core tests
  import packages that left:
  - `example_tool_use_test.go` (core root module → builtin): **move to llm-agent-builtin** (it's a builtin demo).
  - `examples/02-tool-use`, `examples/10-ollama-tools`, `examples/07-policy`: repoint imports to the new
    repos; examples submodule adds `require` + dev `replace => ../../llm-agent-<x>`.
  - `orchestrate/supervisor_budget_test.go` (core → policy + budget): genuine cross-cutting integration test
    of core's Supervisor *with* policy. **Decision: keep in core, drop the policy import** (assert budget
    propagation with a plain scripted model, not via policy wrapping) OR move to llm-agent-policy as an
    integration test importing core. Prefer the former to avoid a core→policy test edge. Flag to codex.

**D3 — Extraction granularity / is comm worth it now?** builtin + policy are clean wins. comm carries D1
  cost + the testenv copy. If review judges comm's cost > benefit, defer comm to Phase B and ship
  builtin+policy now. (Honors "杜绝臆测性设计" — don't pay the comm tax without a consumer pull.)

---

## Execution phases (one repo at a time; review/verify between)

All `go` commands run with **`GOWORK=off`** in the relevant module. Core is on `main` → branch first
(`feat/extract-<name>`). Local cross-repo dev uses `replace => ../llm-agent-<name>`; commit with
`--no-verify` until the lockstep drop-replace step.

### Phase A.0 — in-core decoupling refactor (REVERSIBLE, no new repos)
Prove the leaves work against contract before moving them. One core branch/PR.
1. **builtin**: repoint `builtin/terminal.go:14` import `github.com/costa92/llm-agent` →
   `github.com/costa92/llm-agent-contract/agents` (it only uses `agents.Tool`). Verify no other builtin
   file uses a core-only symbol.
2. **comm** (only if D1=(b)): add comm-internal funcTool helper; repoint a2a/anp/mcp `tool.go` →
   `contract/agents`, swap `agents.NewFuncTool` → local helper.
   - If D1=(a): this step waits for the contract retag instead.
3. Verify: `GOWORK=off go vet ./... && go build ./... && go test ./... -count=1` GREEN in core **and**
   in `examples/` submodule. Zero behavior change.
   - **Gate:** if green, the "leaves are contract-only" thesis is proven in CI. This PR is independently
     valuable (cleaner deps) even if later phases stall.

### Phase A.1 — extract `llm-agent-builtin` (cleanest; proves the pipeline end-to-end)
1. `make add-subproject NAME=llm-agent-builtin` *(GATE: creates public repo)*.
2. `git mv` builtin/*.go → new repo; replace scaffold doc.go/README; set `require contract <latest>`.
3. Move `example_tool_use_test.go` → new repo (D2). Delete from core.
4. Wire: examples submodule + any mover repins to `llm-agent-builtin` (local `replace` for dev).
5. Verify `GOWORK=off vet/build/test` green in: new repo, core, examples.
6. Lockstep publish: tag `llm-agent-builtin v0.1.0`; repin local consumers; drop replaces; dep-currency green.

### Phase A.2 — extract `llm-agent-policy`
Same recipe. Resolve D2 supervisor_budget_test first. `require contract <latest>`. examples/07-policy repoint.
Tag `v0.1.0`.

### Phase A.3 — extract `llm-agent-comm` (only if D3 keeps it in scope)
Same recipe + copy `internal/testenv/listen.go` into the comm repo's own `internal/testenv`.
If D1=(a), contract retag + core alias land first. Tag `v0.1.0`.

### Phase A.4 — umbrella reconciliation
README roster/tree/dep-graph rows for new repos; umbrella.yml cross-build edges if needed; `go work sync`;
update STATE.md / PROJECT.md subproject roster; dep-currency-check green end-to-end (local + remote dispatch).

---

## Rollback
Each phase is an isolated branch/PR set. Pre-publish (before `make add-subproject`) everything is local &
reversible (`git checkout`/`git stash -u`). Post-publish a repo can be archived; core can re-vendor the
package from git history. A.0 (decoupling) is safe to keep regardless — it's a pure dependency-cleanup.

## Open risks
- A new public repo is hard to fully un-create (archive only) — hence the gate.
- dep-currency gate red window during each lockstep (mitigated: zero EXTERNAL consumers, so the only pins
  are core-internal examples + the new repo→contract edge).
- D1=(a) reopens the agents-extraction consumer cascade (otel/cs/flow/rag repin contract) — avoid unless justified.
