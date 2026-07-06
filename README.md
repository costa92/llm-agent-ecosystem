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
├── llm-agent/                       # core framework — stdlib-only (one contract dep)
├── llm-agent-contract/              # stdlib-only LLM-provider contract (ChatModel + capabilities)
├── llm-agent-rag/                   # standalone RAG SDK (additive v1.x public API on main)
├── llm-agent-otel/                  # capability-preserving OpenTelemetry wrappers
├── llm-agent-providers/             # OpenAI / Anthropic / Ollama / DeepSeek / MiniMax adapters
├── llm-agent-customer-support/      # demo customer-support reference service
├── llm-agent-flow/                  # flow IR + DAG executor (root) + /v2 typed-graph engine
├── llm-agent-builtin/               # ready-to-use agent Tools (calculator, note, search, terminal)
├── llm-agent-policy/                # ChatModel policy decorator (PII / injection / length gates)
├── llm-agent-comm/                  # inter-agent comm: base transport + A2A + MCP protocols
├── llm-agent-memory/                # durable memory abstractions + manager (/v2 module, contract-backed)
├── llm-agent-memory-contract/       # backend-neutral durable contract (records, events, outbox)
├── llm-agent-memory-postgres/       # Postgres durable backend + transactional outbox relay
├── llm-agent-memory-gateway/        # HTTP gateway, recall cache, session lifecycle, metrics
├── llm-agent-memory-worker/         # async consolidation worker (outbox → working→episodic)
├── llm-agent-memory-client/         # stdlib-only Go HTTP client for the memory gateway
│
│   # Applications & case-study services (consume the framework family)
├── llm-agent-authz/                 # importable multi-tenant authz library (org→scope, argon2id, JWT)
├── llm-agent-kb/                    # enterprise GraphRAG knowledge-base platform (kbd + React SPA)
├── llm-agent-studio/                # AI Studio — visual workflow orchestration platform
└── llm-agent-console/               # unified ops console (HTTP-only BFF over gateway/flowd/cs)
```

## Repository roster

Tags below are the current released versions as of 2026-07-06; each repo's own
CHANGELOG is the source of truth for its release history.

| Subproject | Role | Current tag | Default branch | Upstream |
|---|---|---|---|---|
| `llm-agent` | core framework, agent paradigms, memory, `llm/v2` | **v0.9.0** | `main` | <https://github.com/costa92/llm-agent> |
| `llm-agent-contract` | stdlib-only LLM-provider contract — `ChatModel`, capability interfaces, streaming, mocks | **v0.6.0** | `main` | <https://github.com/costa92/llm-agent-contract> |
| `llm-agent-rag` | standalone RAG SDK — import, retrieval, generation, GraphRAG | **v1.11.0** | `main` | <https://github.com/costa92/llm-agent-rag> |
| `llm-agent-otel` | OpenTelemetry decorator wrappers for `ChatModel` / `RAGSystem` / `flow.Runner` | **v0.4.0** | `main` | <https://github.com/costa92/llm-agent-otel> |
| `llm-agent-providers` | real provider adapters (OpenAI, Anthropic, Ollama, DeepSeek, MiniMax) | **v0.7.0** | `main` | <https://github.com/costa92/llm-agent-providers> |
| `llm-agent-customer-support` | deployable demo service tying the stack together | **v0.3.0** | `main` | <https://github.com/costa92/llm-agent-customer-support> |
| `llm-agent-flow` | serializable flow IR + DAG executor (root module) + `/v2` typed-graph engine (streaming, checkpoint/resume) | **v0.2.0 (root) / v2.2.0 (`/v2`)** | `main` | <https://github.com/costa92/llm-agent-flow> |
| `llm-agent-builtin` | ready-to-use agent `Tool` implementations (calculator, note, search, terminal) | **v0.1.0** | `main` | <https://github.com/costa92/llm-agent-builtin> |
| `llm-agent-policy` | `ChatModel` policy decorator — PII redaction, injection scanning, length gates | **v0.1.0** | `main` | <https://github.com/costa92/llm-agent-policy> |
| `llm-agent-comm` | inter-agent communication — transport/envelope base + A2A + MCP protocols | **v0.1.0** | `main` | <https://github.com/costa92/llm-agent-comm> |
| `llm-agent-memory` | durable memory abstractions + manager surface (`/v2` module, contract-backed) | **v2.0.0** | `main` | <https://github.com/costa92/llm-agent-memory> |
| `llm-agent-memory-contract` | backend-neutral durable contract (records, events, outbox, idempotency) | **v0.2.0** | `main` | <https://github.com/costa92/llm-agent-memory-contract> |
| `llm-agent-memory-postgres` | concrete Postgres memory backend, migrations, and outbox relay | **v0.1.1** | `main` | <https://github.com/costa92/llm-agent-memory-postgres> |
| `llm-agent-memory-gateway` | HTTP memory gateway, recall cache, session state, and metrics | **v0.4.0** | `main` | <https://github.com/costa92/llm-agent-memory-gateway> |
| `llm-agent-memory-worker` | async consolidation worker — drains outbox, promotes working→episodic | **v0.2.1** | `main` | <https://github.com/costa92/llm-agent-memory-worker> |
| `llm-agent-memory-client` | stdlib-only Go HTTP client for the memory gateway | **unreleased** | `main` | <https://github.com/costa92/llm-agent-memory-client> |

### Applications & case-study services

These sibling repos **consume** the framework family (edges point application →
framework, never the reverse) and live in their own repos with independent
release cycles. They are applications and reusable app-layer libraries, kept
separate from the framework roster above.

| Subproject | Role | Current tag | Upstream |
|---|---|---|---|
| `llm-agent-authz` | importable multi-tenant authz library — org→scope model, argon2id, JWT, refresh sessions, middleware (stdlib + argon2/jwt; no sibling deps) | **v0.4.1** | <https://github.com/costa92/llm-agent-authz> |
| `llm-agent-kb` | enterprise GraphRAG knowledge-base Q&A platform — Go `kbd` backend + React SPA (consumes `rag` / `authz` / `otel` / `providers` / `contract`) | **v0.5.0** | <https://github.com/costa92/llm-agent-kb> |
| `llm-agent-studio` | AI Studio — multi-tenant visual workflow orchestration / content-production platform (consumes `llm-agent` / `authz` / `otel` / `providers` / `contract`) | **v0.9.0** | <https://github.com/costa92/llm-agent-studio> |
| `llm-agent-console` | unified ops console — HTTP-only BFF / reverse proxy over `memory-gateway` + `flowd` + `customer-support` (no Go module edges) | **unreleased** | <https://github.com/costa92/llm-agent-console> |

Each repo tracks its own release history in its CHANGELOG. Where the ecosystem
stands now, relative to the v1.1 alignment close (2026-05-20):

- `llm-agent-contract` was extracted and released (v0.6.0); the contract
  migration is **complete** — every consumer now pins a real `require`, no
  `replace` placeholders remain.
- `llm-agent-rag` continued additive v1.x releases on `main` (now v1.11.0). The
  v1 public API stays additive-only; breaking changes are reserved for the `/v2`
  module path (RFC: [`llm-agent-rag/docs/v2-rfc.md`](https://github.com/costa92/llm-agent-rag/blob/main/docs/v2-rfc.md)).
- `llm-agent-flow` shipped its typed-graph `/v2` engine (v2.2.0 — streaming,
  checkpoint/resume) alongside the stable root module (v0.2.0).
- The memory cluster split into standalone repos and moved to the `/v2` module
  (`llm-agent-memory` v2.0.0, contract-backed; gateway/worker/postgres released
  independently).
- Four application/case-study repos joined the workspace: `llm-agent-authz`,
  `llm-agent-kb`, `llm-agent-studio`, `llm-agent-console` (see the roster above).

## Dependency direction

Framework family (verified against each repo's `go.mod` direct requires):

```
llm-agent-customer-support  ──depends on──▶  llm-agent + llm-agent-contract + llm-agent-otel + llm-agent-providers + llm-agent-flow + llm-agent-rag
llm-agent-otel              ──depends on──▶  llm-agent + llm-agent-contract + llm-agent-rag + llm-agent-flow + llm-agent-flow/v2
llm-agent-providers         ──depends on──▶  llm-agent-contract  (contract-only now — the llm-agent edge was dropped)
llm-agent-flow              ──depends on──▶  llm-agent + llm-agent-flow/v2 (+ llm-agent-contract, indirect)
llm-agent-builtin           ──depends on──▶  llm-agent-contract (Tool interface; stdlib otherwise)
llm-agent-policy            ──depends on──▶  llm-agent-contract (ChatModel decorator; stdlib otherwise; llm-agent is test-only)
llm-agent-comm              ──depends on──▶  llm-agent-contract (A2A/MCP + base; stdlib otherwise; ANP stayed in core)
llm-agent                   ──depends on──▶  llm-agent-contract (its only require; core + contract stay a stdlib-only closure)
llm-agent-contract          ──depends on──▶  (nothing — stdlib only, capability-aware LLM-provider contract)
llm-agent-rag               ──depends on──▶  llm-agent-contract (core); the opt-in adapter/llmagent subpackage also pulls llm-agent
llm-agent-memory (/v2)      ──depends on──▶  llm-agent-contract (contract-backed; the llm-agent edge was removed in the /v2 inversion)
llm-agent-memory-contract   ──depends on──▶  (nothing — stdlib only, backend-neutral durable contract)
llm-agent-memory-postgres   ──depends on──▶  llm-agent-memory-contract
llm-agent-memory-gateway    ──depends on──▶  llm-agent-memory-contract + llm-agent-memory-postgres + llm-agent-rag
llm-agent-memory-worker     ──depends on──▶  llm-agent-memory-contract + llm-agent-memory-postgres
llm-agent-memory-client     ──depends on──▶  (nothing — stdlib-only HTTP client for the gateway)
```

Applications & case-study services consume the framework (edges point application → framework):

```
llm-agent-studio            ──depends on──▶  llm-agent + llm-agent-authz + llm-agent-contract + llm-agent-otel + llm-agent-providers
llm-agent-kb                ──depends on──▶  llm-agent-rag + llm-agent-authz + llm-agent-otel + llm-agent-providers + llm-agent-contract
llm-agent-console           ──depends on──▶  (nothing — HTTP-only BFF / reverse proxy; no Go module edges)
llm-agent-authz             ──depends on──▶  (nothing — stdlib + argon2/jwt; no sibling edges)
```

> **Contract migration complete.** The `llm-agent-contract` extraction is done:
> every consumer pins a real `require` (no `go.work` `replace` / `v0.0.0`
> placeholders remain), and the `INFRA-04` gate keeps `replace` off tagged
> branches.

`llm-agent-rag` is the **fixed point** every framework repo aligns *to* — its
v1.x public API is additive-only (now at v1.11.0 on `main`); breaking changes go
to a `/v2` module path. Downstreams that need RAG import it directly; the core
`llm-agent` no longer ships a facade re-export (P0-2 decision, 2026-05-21).

## Project rules

These are enforced by CI gates across every repo. They are non-negotiable.

1. **Core `llm-agent` stays stdlib-only.** Zero *third-party* deps. The only
   permitted `require` is `github.com/costa92/llm-agent-contract` — the
   LLM-provider contract, which is itself stdlib-only, so "core + contract"
   remains a stdlib-only closure. The previous `llm-agent-rag` back-edge
   exception was removed in P0-2 (2026-05-21) because the facade was an empty
   directory in practice. The B4 gate (`scripts/stdlib-only-check.sh`) asserts
   no direct requires *other than* the contract, and that the transitive dep
   set is stdlib + `llm-agent` + `llm-agent-contract` only.
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

### Adding a subproject

```bash
make add-subproject NAME=llm-agent-foo [LAUNCHABLE=1]
```

`scripts/add-subproject.sh` automates the mechanical, uniform parts: it
`gh repo create`s a public repo, scaffolds `go.mod` + a placeholder package,
copies the four standard workflows (`test` / `pr-governance` /
`release-precheck` / `delete-merged-branch`) plus a self-only `umbrella.yml`,
pushes, applies branch protection (strict, required `go`+`governance`,
`enforce_admins`), and registers the repo in `go.work`, `.gitignore`,
`scripts/eco.sh`, and the `depcheck` roster. It is idempotent — re-run it to
retrofit a repo created before the script existed. The workflow templates live
in `scripts/templates/workflows/`. It deliberately does **not** edit the
judgment-driven parts (this README's roster/graph, the umbrella `umbrella.yml`
cross-build edges, consumer `require`/`replace` wiring) — those are printed as a
checklist for you to complete by hand.

Suggested workflow for cross-repo changes:

1. Run `make bootstrap` once to clone missing subprojects.
2. Run `make workspace` to write the shared `go.work`.
3. Use `make up` for all launchable services or `make up TARGETS=...` for a subset.
4. Before tagging a subproject, keep the repo independent and follow its own release flow.

This is the "coordinated bump + re-tag wave" pattern used in v1.1
(Phase 33 — see `.planning/phases/33-coordinated-bump-and-retag-wave/`).

## Status

Per-repo status lives in each repo's own release notes; the roster above lists
current tags. This root does not track a single umbrella version — the family
has diverged into independently-versioned libraries (framework) and applications
(studio / kb / console).

Historical milestones (v1.0 rag stabilization, v1.1 ecosystem alignment,
v1.2 core-capability deepening, v1.3 rag perf-wave) shipped through 2026-05.
Since then the framework continued additive releases (contract extraction,
flow `/v2`, memory `/v2` cluster split) and the application layer was added.

The authoritative live milestone state for the core framework is
`llm-agent/.planning/STATE.md`; each application repo keeps its own planning.

---
*Workspace consolidated 2026-05-20 from prior `/tmp/` and `costa92/`
sibling locations into this single ecosystem directory.*
