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
└── llm-agent-customer-support/      # demo customer-support reference service
```

## Repository roster

| Subproject | Role | Current tag | Default branch | Upstream |
|---|---|---|---|---|
| `llm-agent` | core framework, agent paradigms, memory, RAG facade, `llm/v2` | **v0.5.0** | `main` | <https://github.com/costa92/llm-agent> |
| `llm-agent-rag` | standalone RAG SDK — import, retrieval, generation, GraphRAG | **v1.0.0** | `master` | <https://github.com/costa92/llm-agent-rag> |
| `llm-agent-otel` | OpenTelemetry decorator wrappers for `ChatModel` / `RAGSystem` | **v0.2.0** | `main` | <https://github.com/costa92/llm-agent-otel> |
| `llm-agent-providers` | real provider adapters (OpenAI, Anthropic, Ollama, DeepSeek, MiniMax) | **v0.2.0** | `main` | <https://github.com/costa92/llm-agent-providers> |
| `llm-agent-customer-support` | deployable demo service tying the stack together | **v0.2.0** | `main` | <https://github.com/costa92/llm-agent-customer-support> |

Tag layout as of v1.1 (ecosystem alignment milestone, 2026-05-21).

## Dependency direction

```
llm-agent-customer-support  ──depends on──▶  llm-agent + llm-agent-otel + llm-agent-providers
llm-agent-otel              ──depends on──▶  llm-agent + llm-agent-rag
llm-agent-providers         ──depends on──▶  llm-agent
llm-agent                   ──depends on──▶  llm-agent-rag (RAG facade only)
llm-agent-rag               ──depends on──▶  (stdlib only at v1.0.0; `postgres` subpackage may pull pgx)
```

`llm-agent-rag` is the **fixed point** every other repo aligns *to* — its
v1.x public API is additive-only; breaking changes go to a `/v2` module path.

## Project rules

These are enforced by CI gates across every repo. They are non-negotiable.

1. **Core `llm-agent` stays stdlib-only.** No `go.sum` with non-stdlib deps;
   no non-stdlib in `go.mod`. The only `go.sum` line allowed is the
   `llm-agent-rag` back-edge for the RAG facade.
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
- v1.1 — Ecosystem alignment (this milestone) — **in flight**;
  Phases 31-33 complete (4/5 requirements done), Phase 34 pending
  (umbrella dependency-currency CI gate + audit + close).

---
*Workspace consolidated 2026-05-20 from prior `/tmp/` and `costa92/`
sibling locations into this single ecosystem directory.*
