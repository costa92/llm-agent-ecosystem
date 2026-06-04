# LOCKSTEP-PLAN: Leaf-First Finalization of the Contract Decoupling

Finalize the in-flight contract decoupling of the consumer repos so they pin the TAGGED
`llm-agent-contract v0.1.0` + current `llm-agent v0.8.0`, drop all `replace` directives, and tag.
Designed by Plan agent (2026-06-03) after the memory-inversion Phases 0–2 landed.

## Ground-truth corrections (verified)
- **rag**: default branch is `master` (no `main`); requires `llm-agent v0.5.0`; HAS `feat/decouple-llm-contract` (+1 `66aa7a2`). IN SCOPE. Cycle-exempt for the dep-currency gate (rag→llm-agent).
- **flow**: imports ZERO symbols from llm-agent-contract (contract is `// indirect` cruft from the replace). `go mod tidy` after dropping the replace DELETES the contract line. flow's only real sibling dep is `llm-agent` (uses `agents.{Agent,Tool,Registry,NewFuncTool}`). main/feat pin `llm-agent v0.5.1`.
- **otel feat is doubly-stale**: still pins llm-agent v0.7.0, flow v0.0.7, rag v1.9.0 (built pre-cascade) + contract v0.0.0 + 4 replaces. Needs FOUR bumps, not just replace-drop.
- **customer-support**: decoupling already MERGED on main (contract v0.0.0 + 6 replaces); feat==main. Repin-only PR off main.
- No repo imports a `llm-agent/memory` engine constructor (amputation-safe).

## Leaf-first topological tag order
```
contract v0.1.0 ✓, llm-agent v0.8.0 ✓ (done)
Wave A (independent — deps already tagged): flow, rag, providers
   ↓
otel  (waits on flow + rag)
   ↓
customer-support  (waits on flow, rag, providers, otel — LAST, integration test)
```

## Per-repo finalization (proposed versions — see Open Questions)
Each: finalize the existing `feat/decouple-llm-contract` branch (cs = repin off main); `GOWORK=off go mod tidy`
+ vet + build + test green + tidy fixed-point; PR → main (rag → master); tag from `release/vX` (release-precheck
confirms zero replace).

1. **flow → v0.2.0**: drop both replaces; `llm-agent v0.5.1→v0.8.0`; tidy drops orphan contract `//indirect`. End go.mod: llm-agent v0.8.0 + cel-go + sqlite, NO contract, NO replace.
2. **rag → v1.10.0** (off master): contract v0.0.0→v0.1.0; llm-agent →v0.8.0; drop 2 replaces.
3. **providers → v0.3.0**: contract v0.0.0→v0.1.0; drop the contract replace. (Already dropped llm-agent on feat.)
4. **otel → v0.4.0** (after flow+rag): contract→v0.1.0; llm-agent v0.7.0→v0.8.0; flow v0.0.7→v0.2.0; rag v1.9.0→v1.10.0; drop 4 replaces.
5. **customer-support → v0.3.0** (LAST, off main): contract→v0.1.0; llm-agent→v0.8.0; otel→v0.4.0; providers→v0.3.0; flow v0.1.1→v0.2.0; rag v1.9.0→v1.10.0; drop all 6 replaces.

## CI gates
- **test.yml** (GOWORK=off): tidy drift + vet + build + test per repo. (Does NOT run on release/**; green proof from PR-to-main.)
- **release-precheck.yml** (release/**): rejects any replace → tag from release/vX with zero replaces.
- **llm-agent umbrella dep-currency-check.sh**: strict-equality of REPOS-member sibling pins vs latest remote tag. REPOS = {llm-agent, rag, otel, providers, customer-support}; flow ∉ REPOS; rag→llm-agent exempt. **Do not open a PR against llm-agent main until steps 1–5 are all tagged** (else transient red).

## Staleness windows (closed by the leaf-first order)
- S0 (pre-existing): otel+cs pin llm-agent v0.7.0 → llm-agent umbrella RED now. Closed by steps 4+5.
- S1: rag v1.10.0 stales otel+cs rag pins → closed by steps 4+5 (run after rag).
- S2: otel v0.4.0 stales cs otel pin → closed by step 5.
- S3: providers v0.3.0 stales cs providers pin → closed by step 5.
- flow tag stales nothing in the gate (flow ∉ REPOS) — repin anyway for correctness.

## Risk register
- R1 flow v0.5.1→v0.8.0 jump (crosses v0.6/0.7): flow only uses `agents.*` (survived amputation); the step-1 build against real v0.8.0 is the proof. Recommend the direct jump.
- R2 flow invisible to dep-currency gate → manually repin flow in otel+cs; consider adding flow to REPOS (separate decision).
- R3 rag cycle: bumping rag's llm-agent pin to v0.8.0 is nice-to-have (clean standalone build), not gate-required.
- R4 cs already on main → repin-only PR dropping 6 replaces; cs is LAST so its green build integration-tests the cascade.
- R5 feat branches each exactly +1 over base (no hidden divergence); rebase on current main/master before finalizing.
- R6 otel doubly-stale → FOUR bumps, not just replace-drop (don't under-scope like providers).
- R7 providers/otel/rag/flow/cs have go.sum → commit refreshed go.sum with go.mod.

## Open Questions (USER DECISION)
1. Version convention: minor (flow v0.2.0/rag v1.10.0/providers v0.3.0/otel v0.4.0/cs v0.3.0) vs patch. Recommend MINOR (architectural dep change).
2. flow in scope? Recommend YES (finalize as a llm-agent-currency bump; else otel/cs pull two llm-agent versions transitively).
3. rag in scope? Recommend YES (it has the feat decoupling; cycle exemption makes currency optional for the gate but standalone build wants it).
4. flow → add to dep-currency REPOS? (closes blind spot; out of scope for this cascade, flagged.)
5. flow v0.5.1→v0.8.0: direct jump (recommended, build-validated) vs intermediate v0.7.0 tag first.
