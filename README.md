# llm-agent-ecosystem

Umbrella project for the `llm-agent` family. This directory is the top-level
workspace shell; each subdirectory is a subproject with its own GitHub repo,
branch, tags, and release cycle. The root owns navigation and conventions, not
product code.

This root is the coordination point for the ecosystem, not a monorepo that
absorbs subproject source trees.

## Docs

- [Docs index](./docs/README.md)
- [Current project analysis](./docs/current-project-analysis.md)
- [当前项目分析](./docs/current-project-analysis.zh-CN.md)

## Subprojects

```
llm-agent-ecosystem/
├── llm-agent/                       # core framework — stdlib-only, zero-dep
├── llm-agent-rag/                   # standalone RAG SDK (frozen v1.x public API)
├── llm-agent-otel/                  # capability-preserving OpenTelemetry wrappers
├── llm-agent-providers/             # OpenAI / Anthropic / Ollama / DeepSeek / MiniMax adapters
├── llm-agent-customer-support/      # demo customer-support reference service
└── llm-agent-flow/                  # serializable flow IR + DAG executor (v0.0.x)
```

## Repository roster

| Subproject | Role | Current tag | Default branch | Upstream |
|---|---|---|---|---|
| `llm-agent` | core framework, agent paradigms, memory, RAG facade, `llm/v2` | **v0.5.1** | `main` | <https://github.com/costa92/llm-agent> |
| `llm-agent-rag` | standalone RAG SDK — import, retrieval, generation, GraphRAG | **v1.0.2** | `master` | <https://github.com/costa92/llm-agent-rag> |
| `llm-agent-otel` | OpenTelemetry decorator wrappers for `ChatModel` / `RAGSystem` / `flow.Runner` | **v0.2.2** | `main` | <https://github.com/costa92/llm-agent-otel> |
| `llm-agent-providers` | real provider adapters (OpenAI, Anthropic, Ollama, DeepSeek, MiniMax) | **v0.2.2** | `main` | <https://github.com/costa92/llm-agent-providers> |
| `llm-agent-customer-support` | deployable demo service tying the stack together | **v0.2.3** | `main` | <https://github.com/costa92/llm-agent-customer-support> |
| `llm-agent-flow` | serializable flow IR + DAG executor (v0.1.x stable) | **v0.1.1** | `main` | <https://github.com/costa92/llm-agent-flow> |

Tag layout as of the v1.1 close (2026-05-20) + `llm-agent-flow`
introduced 2026-05-21 (v0.0.1 walking skeleton → v0.0.2 per-layer
parallelism + `cmd/flowd` HTTP → v0.0.3 tool manifest → v0.0.4 CEL
conditional edges → v0.0.5 SQLite run history + CRUD → v0.0.6
per-event persistence → v0.0.7 Runner-interface seam → `otelflow`
wrapper in `llm-agent-otel` v0.2.2 → v0.0.8 bearer-token auth →
v0.0.9 replay endpoint → **v0.1.0 SemVer freeze + API snapshot
gate** → v0.1.1 LRU engine cache + sync-run event batching →
`flowrunner` in `llm-agent-customer-support` v0.2.3). v1.1
ecosystem alignment
milestone shipped; v1.2 Core Capability Deepening is the active
milestone.

## Dependency direction

```
llm-agent-customer-support  ──depends on──▶  llm-agent + llm-agent-otel + llm-agent-providers + llm-agent-flow + llm-agent-rag
llm-agent-otel              ──depends on──▶  llm-agent + llm-agent-rag + llm-agent-flow
llm-agent-providers         ──depends on──▶  llm-agent
llm-agent-flow              ──depends on──▶  llm-agent
llm-agent                   ──depends on──▶  (nothing — stdlib only, zero third-party requires)
llm-agent-rag               ──depends on──▶  (stdlib only at v1.0.0; `postgres` subpackage may pull pgx)
```

`llm-agent-rag` is the **fixed point** every other repo aligns *to* — its
v1.x public API is additive-only; breaking changes go to a `/v2` module path.
Downstreams that need RAG import it directly; the core `llm-agent` no
longer ships a facade re-export (P0-2 decision, 2026-05-21).

## Project rules

These are enforced by CI gates across every repo. They are non-negotiable.

1. **Core `llm-agent` stays stdlib-only.** Zero third-party deps:
   `go.mod` carries no `require` block, `go.sum` is empty. The previous
   `llm-agent-rag` back-edge exception was removed in P0-2 (2026-05-21)
   because the facade was an empty directory in practice; the B4 gate
   (`scripts/stdlib-only-check.sh`) now asserts zero direct requires.
2. **No `replace` directives in tagged-release branches.** `replace` is a
   local-dev escape hatch only. The `INFRA-04` CI gate refuses to tag a
   commit whose `go.mod` carries a `replace`.
3. **`go.work` is `.gitignore`d in every repo.** CI runs with `GOWORK=off`.
   If you want a local workspace, drop a `go.work` at this directory's
   root — every repo's `.gitignore` already covers it.
4. **No K8s / Helm packaging** anywhere in the ecosystem. Standing non-goal.
5. **Capabilities are per-`(provider × model)`,** not per-provider. A
   provider instance binds a model at construction; `Info()` reflects that
   model's capabilities. (Keystone K2.)
6. **OTel attaches as decorator wrappers, never hooks** —
   `otelmodel.Wrap(inner) ChatModel`. (Keystone K3.)
7. **Streaming events are a typed union, not lowest-common-denominator
   chunks.** `StreamEvent.Kind` enum with a stable per-tool-call `Index`
   field. (Keystone K1.)

## Root planning

The umbrella-level planning docs live under `./.planning/` when present. They
describe the ecosystem as a whole, while each subproject keeps its own repo-
local planning and release metadata.

## Source of truth for planning

Milestone planning, requirements, decisions, and phase plans live in
`llm-agent/.planning/` (the core repo). Useful starting points:

- `llm-agent/.planning/PROJECT.md` — what the project is, core value, hard rules
- `llm-agent/.planning/STATE.md` — current milestone + active phase
- `llm-agent/.planning/ROADMAP.md` — phase plan for the active milestone
- `llm-agent/.planning/REQUIREMENTS.md` — the active milestone's requirements + traceability
- `llm-agent/.planning/research/v1.1-ecosystem-alignment-SUMMARY.md` —
  the cross-cut audit, keystone decisions KE-1…KE-7

Each sister repo keeps its own focused `README.md` for its own surface
area. This top-level README is only a navigation index.

## Working with the umbrella locally

```bash
make bootstrap
make workspace
make status
make build
make test
make up
make up TARGETS=llm-agent-customer-support
make down TARGETS=llm-agent-customer-support
```

`make up` starts the launchable subprojects; `TARGETS=` lets you select one or
more by name. Library-only subprojects still participate in `build` and `test`.

Suggested workflow for cross-repo changes:

1. Run `make bootstrap` once to clone missing subprojects.
2. Run `make workspace` to write the shared `go.work`.
3. Use `make up` for all launchable services or `make up TARGETS=...` for a subset.
4. Before tagging a subproject, keep the repo independent and follow its own release flow.

This is the "coordinated bump + re-tag wave" pattern used in v1.1
(Phase 33 — see `.planning/phases/33-coordinated-bump-and-retag-wave/`).

## Status

- v1.0 — `llm-agent-rag` API stabilization — **shipped** 2026-05-21.
- v1.1 — Ecosystem alignment — **shipped** 2026-05-20.
  All 5 ECO requirements delivered, all 7 KE keystones honored,
  coordinated 5-repo tag set internally consistent end-to-end,
  umbrella dependency-currency CI gate live and green. Audit:
  `llm-agent/.planning/v1.1-MILESTONE-AUDIT.md` (PASS 5/5).
- **v1.2 — Core Capability Deepening — in flight.** First
  core-feature milestone since v0.3. Theme: Core v0.6 — capability
  additions to core `llm-agent` (`budget`, `policy`,
  `orchestrate.Supervisor`); memory tiering deferred to v1.3 per
  KC-2. Phase 35 (budget / cancellation context, requirement CC-1)
  in active execution; Phases 36-38 plan budget→policy→supervisor→
  audit/close. Source of truth: `llm-agent/.planning/STATE.md`.

---
*Workspace consolidated 2026-05-20 from prior `/tmp/` and `costa92/`
sibling locations into this single ecosystem directory.*
