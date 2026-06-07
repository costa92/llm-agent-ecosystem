# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this repo is

`llm-agent-ecosystem` is the **umbrella shell** for the `llm-agent` family of Go
modules. The root owns coordination (navigation, shared conventions, cross-repo
CI) — it does **not** own product code. Each subproject
(`llm-agent`, `llm-agent-rag`, `llm-agent-providers`, `llm-agent-otel`,
`llm-agent-customer-support`, `llm-agent-flow`, the `llm-agent-memory-*` set,
and the `llm-agent-{contract,builtin,policy,comm}` siblings) is an
independently versioned repo with its own git history, tags, CI, and release
flow. The README's roster is the source of truth for the subproject list and
current tag set.

If the task is to change product code in a specific subproject, do the work
**inside that subproject's directory** (or its own repo) and use **its**
CLAUDE.md / `.planning/` / `docs/` for context. The umbrella root is for
cross-repo work only: dependency wiring, shared scripts, CI workflows, the
depcheck cascade tool, the umbrella CI workflow, and the `README.md` /
`PROJECT.md` / `docs/` navigation.

## Build / test / status commands

All commands run from the umbrella root unless noted. Every repo-aware command
in the Makefile delegates to `scripts/eco.sh` and exports `GOWORK=off` per
subproject so the workspace file cannot mask a tagged-graph break.

```bash
make bootstrap              # clone missing subprojects + install pre-commit hooks
make install-hooks          # re-install the replace-guard pre-commit hook
make workspace              # (re)write go.work for local cross-repo development
make pull                   # fast-forward every cloned subproject
make status                 # one-line git status per subproject
make build                  # go build ./... in every subproject
make test                   # go test ./... -count=1 in every subproject
make prune-branches         # delete local branches whose remote was deleted (merged-only, safe)
make up                     # docker compose up for the launchable subprojects
make down                   # docker compose down
make add-subproject NAME=llm-agent-foo [LAUNCHABLE=1]
                            # gh repo create + scaffold + register
```

`make build` / `make test` operate on every subproject by default. Target a
subset by exporting `TARGETS=llm-agent,llm-agent-rag`. Two launchable
subprojects (`llm-agent-otel`, `llm-agent-customer-support`) participate in
`make up` / `make down` — see `scripts/eco.sh:25-28` for the roster.

Useful `scripts/eco.sh` subcommands not exposed via the Makefile:

```bash
./scripts/eco.sh release-check [repo,...]   # build + vet, no tests (skew-honest tag-graph check)
./scripts/eco.sh bootstrap [repo,...]       # same as make bootstrap but scoped
```

### Single-test workflow inside a subproject

```bash
cd llm-agent
GOWORK=off go test ./agents/... -run TestReAct -count=1
```

`GOWORK=off` is required to match CI behavior. The `go.work` file is
`.gitignore`d in every repo; without it, your local clone resolves deps against
the workspace instead of the subproject's own `go.mod`.

## Hard rules (enforced by CI gates)

These are non-negotiable across every repo. They are documented in
`README.md` §"Project rules" and re-asserted in each subproject's own CLAUDE.md.

1. **Core `llm-agent` stays stdlib-only.** Its only permitted `require` is
   `github.com/costa92/llm-agent-contract` (itself stdlib-only). No back-edge
   to `llm-agent-rag`, no third-party deps, no `go.sum` entries. Enforced by
   `scripts/stdlib-only-check.sh` (B4 CI gate) and `.github/workflows/umbrella.yml:298-326`.
2. **No `replace` directives in tagged-release branches.** `replace` is a
   local-dev escape hatch only. INFRA-04 CI gate refuses to tag a commit whose
   `go.mod` carries a `replace`. The pre-commit hook (`scripts/hooks/pre-commit`,
   installed by `make install-hooks`) blocks the same pattern locally.
3. **`go.work` is `.gitignore`d in every repo.** CI runs with `GOWORK=off`.
4. **No K8s / Helm packaging anywhere.** Standing non-goal.
5. **Capabilities are per-`(provider × model)`, not per-provider.** A provider
   instance binds a model at construction; `Info()` reflects THAT model's
   capabilities. (Keystone K2.)
6. **OTel attaches as decorator wrappers, never hooks.** `otelmodel.Wrap(inner) ChatModel`. (Keystone K3.)
7. **Streaming events are a typed union, not lowest-common-denominator chunks.**
   `StreamEvent.Kind` enum with a stable per-tool-call `Index` field. (Keystone K1.)

Keystones K4–K7 (and the CC-*, ECO-*, KC-* requirement families) are tracked
in `llm-agent/.planning/research/` and re-stated in each subproject's
CLAUDE.md.

## CI gates run by the umbrella workflow

`.github/workflows/umbrella.yml` runs four jobs on every push / PR to `main`:

- `cross-repo-build` — checks out every sibling at `main` and runs
  `GOWORK=off go vet ./... && go build ./... && go test ./... -count=1` per
  repo, then runs `cmd/depcheck` to emit the cascade-order JSON artifact.
- `B2 — flowd binary smoke gate` — builds `cmd/flowd` from `llm-agent-flow`,
  starts it, hits `/healthz` and `/flows`, tears it down.
- `B4 — core llm-agent stdlib-only assertion` — runs `scripts/stdlib-only-check.sh`.
- `B5 — PII/injection regex parity (policy <-> rag)` — runs `scripts/regex-parity-check.sh`.
- `B6 — promotion policy parity (worker <-> gateway)` — runs `scripts/promotion-policy-parity-check.sh`.

PR governance: `.github/workflows/pr-governance.yml` requires a current-head
approval from `@costa92` for non-owner PRs and auto-enables auto-merge for
owner PRs. Owner-authored PRs are auto-merged on a clean green build.

## Cross-repo coordination model

- `llm-agent-rag` is the **fixed point** every other repo aligns *to*. Its v1.x
  public API is additive-only; breaking changes go to a `/v2` module path (RFC
  lives in `llm-agent-rag/docs/v2-rfc.md`).
- Downstreams that need RAG import it directly; the core `llm-agent` no longer
  ships a facade re-export (P0-2 decision).
- The `llm-agent-contract` extraction is in flight: locally wired via `go.work`
  + `replace … => ../llm-agent-contract` (`v0.0.0` placeholder) in each
  consumer. The INFRA-04 gate refuses `replace` on tagged-release branches, so
  finishing the migration is a lockstep: tag `llm-agent-contract` v0.x.0 →
  bump each consumer's `require` and drop the `replace` → push.
- Bumping the cascade: `cmd/depcheck` (`go run ./cmd/depcheck`) prints the
  leaf-first order for a coordinated tag set and exits 1 if any pin is stale.

## Files you should NOT touch without explicit ask

- `README.md` §"Subprojects" roster, tag column, and dependency-direction
  diagram — only update at coordinated bump + re-tag waves.
- `.github/workflows/umbrella.yml` — the cross-repo-build job's repo list and
  job matrix are the source of truth for "what gets built on every PR." Adding
  or removing a subproject means editing this file AND `scripts/eco.sh:6-23`
  AND `cmd/depcheck/main.go:32-46` AND the `go.work` `use` block in lockstep.
- `go.work` — workspace file, `.gitignore`d, regenerated by `make workspace`.
- `scripts/hooks/pre-commit` — the replace-guard hook. Any new "block on X" rule
  belongs here, not as a CI-only check.
- The `INFRA-04` posture: do not weaken the `replace`-in-tagged-branches
  enforcement even briefly. The migration path for
  `llm-agent-contract` extraction is "tag the contract, drop the `replace`, push" — not "loosen the gate."

## When the user asks for code

- **Default to the subproject where the change lives.** If the change is in
  core agent code, edit `llm-agent/...`. If it's an OTel wrapper, edit
  `llm-agent-otel/...`. The umbrella root is for cross-repo wiring.
- **Trust the existing keystone decisions.** If a request would break K1–K3
  (e.g. "make OTel attach via hook", "have `Info()` return provider-level
  capabilities", "stream text deltas as `[]string` chunks"), push back and
  cite the keystone — the keystones exist because earlier alternatives were
  tried and rejected.
- **No speculative abstractions in the umbrella scripts.** The umbrella's job
  is narrow: wire repos, run CI, render status. Resist the urge to make
  `scripts/eco.sh` or `cmd/depcheck` do things the subprojects should do
  themselves.
- **Single-test, single-package, single-file** all use the standard
  `GOWORK=off go test ./pkg/... -run TestX -count=1` form from inside the
  subproject directory. The `-count=1` flag is what CI uses — without it,
  `go test` caches results across runs.

## When in doubt

- **For product-code questions:** read `<subproject>/CLAUDE.md` and
  `<subproject>/.planning/STATE.md` first. The core's planning bundle
  (`llm-agent/.planning/`) is the most detailed; sister repos keep their own
  focused planning.
- **For cross-repo questions:** read `README.md` (navigation + rules),
  `docs/README.md` (ecosystem-level design docs), and the relevant
  `source-design-*.zh-CN.md` in `docs/`.
- **For CI / gate questions:** read `.github/workflows/umbrella.yml` end-to-end
  before changing anything; the gate comments name the keystone or rule they
  enforce.
- **When the right answer is "edit it in the subproject repo, not here":** say
  so. The umbrella is a coordination shell, not a sink.
