# Coding Conventions

**Analysis Date:** 2026-05-20

This document captures the conventions enforced across the umbrella
`llm-agent-ecosystem`. Each subproject keeps its own repo-local CI gates,
but all five share the conventions below. Code samples are taken from the
working tree as of the analysis date.

## Naming Patterns

**Modules:**
- Module paths follow the GitHub canonical form
  `github.com/costa92/llm-agent[-<facet>]`. Five modules exist:
  `llm-agent`, `llm-agent-rag`, `llm-agent-otel`, `llm-agent-providers`,
  `llm-agent-customer-support`. See each `go.mod`.
- `llm-agent-rag` deliberately names its root package `ragkit` instead of
  the module's basename — recorded in `llm-agent-rag/doc.go` as an
  intentional short brand identifier, not an accidental mismatch.

**Packages:**
- One-word, all lowercase, importable: `agents`, `llm`, `budget`,
  `rag`, `retrieve`, `store`, `embed`, `ingest`, `generate`, `pack`,
  `prompt`, `rerank`, `graph`, `eval`, `otelmodel`, `otelrag`,
  `otelagent`, `otelmetrics`, `otelslog`, `openai`, `anthropic`,
  `ollama`, `deepseek`, `minimax`, `app`, `httpapi`, `guardrails`,
  `sessionstore`, `supportflow`, `knowledgebase`, `limits`, `config`.
- The core repo's root Go package is `agents`, not `llm-agent` — the
  repository name and Go package name diverge by design (see
  `llm-agent/doc.go` lines 1-5).

**Files:**
- Implementation: `snake_case.go` for multi-word files (`agent_chatmodel.go`,
  `function_call.go`, `plan_solve.go`, `budget_integration_test.go`).
- Tests: paired `<impl>_test.go` (`simple.go` ↔ `simple_test.go`,
  `chain.go` ↔ `chain_test.go`).
- Example tests: `example_<scenario>_test.go` (e.g.
  `llm-agent/example_simple_test.go`, `example_tool_use_test.go`,
  `example_multi_agent_test.go`).
- Integration tests: `<topic>_integration_test.go` (e.g.
  `llm-agent/budget_integration_test.go`).
- Conformance tests: `<topic>_conformance_test.go` (e.g.
  `llm-agent-rag/store/inmemory_conformance_test.go`,
  `llm-agent-rag/postgres/postgres_conformance_test.go`).
- Build-tagged source: kept beside same-package files but with explicit
  `//go:build <tag>` lines (e.g. `llm-agent-rag/adapter/llmagent/*.go`
  uses `//go:build llmagent`, `llm-agent-providers/internal/contract/ollama_live_test.go`
  uses `//go:build ollama_live`).

**Types / functions:**
- Exported public surface uses `CamelCase` (Go-idiomatic). Examples:
  `agents.Agent`, `agents.Result`, `agents.Step`, `agents.StepKind`,
  `llm.ChatModel`, `llm.Request`, `llm.Response`, `llm.ScriptedLLM`,
  `budget.Budget`, `budget.Tracker`.
- Sentinel errors are exported as `Err<Symbol>` (camelcase, no spaces),
  always declared at package scope with `errors.New`:

  ```go
  // llm-agent/agent.go:127-136
  var (
      ErrMaxStepsExceeded      = errors.New("agents: max steps exceeded")
      ErrToolNotFound          = errors.New("agents: tool not found")
      ErrToolAlreadyRegistered = errors.New("agents: tool already registered")
      ErrPlanningFailed        = errors.New("agents: planning failed")
      ErrParseToolCall         = errors.New("agents: failed to parse tool call")
      ErrEmptyInput            = errors.New("agents: empty input")
  )
  ```

  Error message always starts with `"<package>: <reason>"`. Callers test
  via `errors.Is` (see `llm-agent/agent_test.go:18-33`).
- Functional-options constructors: `New<Type>(opts ...<Type>Option)` with
  per-option helpers `With<Field>(value)` (see
  `llm-agent/llm/scripted.go:50-64`, `llm-agent-providers/openai`,
  `llm-agent-providers/anthropic`).
- Compile-time interface checks live in production files, not just
  tests, so capability claims appear in godoc:

  ```go
  // llm-agent/llm/scripted.go:42-48
  var (
      _ ChatModel         = (*ScriptedLLM)(nil)
      _ ToolCaller        = (*ScriptedLLM)(nil)
      _ Embedder          = (*ScriptedLLM)(nil)
      _ StructuredOutputs = (*ScriptedLLM)(nil)
  )
  ```

  Test-side mirror at `llm-agent/agent_test.go:10-16`.

**Variables / constants:**
- `StepKind` enum is a typed string with lowercase string values
  (`"thought"`, `"action"`, `"observation"`, `"reflection"`, `"plan"`,
  `"final"`); pinned by `TestStepKind_Constants`
  (`llm-agent/agent_test.go:35-49`).

## Code Style

**Formatting:**
- `gofmt` is mandatory; the `llm-agent-customer-support` CI workflow
  promotes it to a hard gate
  (`llm-agent-customer-support/.github/workflows/test.yml` job
  `format`, step `gofmt (drift check)`). Other repos rely on `go vet`
  + `go mod tidy` drift checks and treat `gofmt`-cleanness as a
  norm (recorded as "the repository is now `gofmt`-clean" in the
  `llm-agent-rag` v1.0.0 changelog).
- No `.golangci.yml`, `.editorconfig`, or external linter config is
  committed anywhere — the toolchain is pure `go vet` + `gofmt` +
  `go mod tidy`.

**Toolchain:**
- All five modules pin `go 1.26.0` in their `go.mod`. CI uses
  `actions/setup-go@v5` with `go-version-file: go.mod` to follow the
  module pin.
- `GOWORK=off` is the CI default in every repo (recorded as `INFRA-02`).
  Local development may opt into a `go.work` at the umbrella root; the
  per-repo `.gitignore` covers `go.work` and `go.work.sum`.

**Linting:**
- Per-repo `test.yml` enforces:
  1. `go mod tidy` drift check (`go mod tidy` must be a no-op).
  2. `go vet ./...`
  3. `go build ./...`
  4. `go test ./...`
  Source: `llm-agent/.github/workflows/test.yml`,
  `llm-agent-providers/.github/workflows/test.yml`,
  `llm-agent-otel/.github/workflows/test.yml`,
  `llm-agent-rag/.github/workflows/test.yml`,
  `llm-agent-customer-support/.github/workflows/test.yml`.
- `llm-agent` runs the same gauntlet a second time inside `examples/`
  (the examples module has its own `go.mod`).
- `llm-agent-rag` adds a `core module boundary` gate that `rg`-greps for
  forbidden imports of `github.com/costa92/llm-agent` outside
  `adapter/` (test.yml lines 24-39) and a `Build-tagged adapter
  (llmagent)` gate that runs the `//go:build llmagent` slice.

## Import Organization

**Order (observed in every file inspected):**
1. Stdlib (`context`, `encoding/json`, `errors`, `fmt`, `net/http`,
   `testing`, `sync`, `time`, etc.)
2. Blank line separator
3. Third-party / sibling-repo (`github.com/costa92/llm-agent/...`,
   `github.com/costa92/llm-agent-rag/...`, `go.opentelemetry.io/...`,
   `go.uber.org/goleak`)
4. Same-module subpackages

Example: `llm-agent/budget_integration_test.go:17-26`,
`llm-agent-otel/otelmodel/otelmodel_test.go:1-14`.

**Path Aliases:**
- None. Plain Go-module paths only. The umbrella never uses Go module
  `replace` directives in any committed `go.mod` (verified by
  `grep -rn "^replace " llm-agent*/go.mod` returning no matches), and a
  CI gate (`INFRA-04`, `release-precheck.yml`) refuses to tag a commit
  whose `go.mod` carries a `replace`.

## Public API Discipline

**`llm-agent-rag` (frozen v1.x):**
- Within the `v1.x` series, the public API is **additive-only**.
  Exported symbols are not renamed, removed, or re-signed. Breaking
  changes go to `/v2` (separate module path).
  Source: `llm-agent-rag/docs/compatibility.md:18-89`,
  `llm-agent-rag/CHANGELOG.md:9-39`.
- The freeze is gated by a committed exported-surface snapshot at
  `llm-agent-rag/api/v1.snapshot.txt` and an in-tree test
  `llm-agent-rag/internal/apisnapshot/apisnapshot_test.go` (function
  `TestAPISnapshot`). Deliberate additive changes regenerate the
  baseline via `go test ./internal/apisnapshot/ -run TestAPISnapshot
  -update`.
- The root package `ragkit` exports nothing; callers import the
  subpackages (`rag`, `retrieve`, `store`, `embed`, `ingest`,
  `generate`, `eval`, ...). This anchors documentation without
  freezing a root surface.

**`llm-agent` (core):**
- Stays stdlib-only. The only allowed dependency line in `go.mod` is
  the `llm-agent-rag` back-edge for the RAG facade
  (`require github.com/costa92/llm-agent-rag v1.0.1`). No `go.sum`
  is committed when the back-edge is the only require — currently
  `llm-agent/go.mod` has the back-edge so a `go.sum` exists.
- The Phase-7 v0.4 cycle removed every legacy `Deprecated:` symbol
  in `llm/` (see `llm-agent/DEPRECATIONS.md`). New deprecations
  must add an "Active deprecations" row with the godoc format
  `Deprecated: <use what instead>. Will be removed in vX.Y.Z.`
- Compile-time `var _ Interface = (*Impl)(nil)` assertions sit in
  production files so the capability claims surface in godoc rather
  than only in tests (e.g. `llm/scripted.go:42-48`,
  `agent_test.go:10-16`).

**Sister repos (`otel`, `providers`, `customer-support`):**
- Each follows SemVer 0.x with BC-compatible minor/patch and lock-step
  cascade bumps coordinated by the umbrella (`v1.1` milestone).
- No additive-only freeze yet — they remain in 0.x.

## Versioning

**SemVer with the following live tags (as of 2026-05-20):**

| Module | Current tag | Default branch |
|---|---|---|
| `llm-agent` | `v0.5.1` (next bump → `v0.6.0` for v1.2 milestone) | `main` |
| `llm-agent-rag` | `v1.0.1` | `master` |
| `llm-agent-otel` | `v0.2.1` | `main` |
| `llm-agent-providers` | `v0.2.1` | `main` |
| `llm-agent-customer-support` | `v0.2.2` | `main` |

Roster source: root `README.md` lines 30-38, with cascade-bump tags
visible via `git log --oneline` in each sister repo
(`v1.1-cascade-bump`, `v1.1-alignment`).

**Per-repo `CHANGELOG.md`:**
- Format: Keep-a-Changelog 1.1 (sections: Added / Changed /
  Deprecated / Removed / Fixed / Security / Breaking).
- Each release entry carries the date in `YYYY-MM-DD`.
- 0.x bumps may break across minor (`0.x → 0.y` where `y > x`); patch
  is BC. v1.x bumps are additive-only as above.
- Header comments at the top of `llm-agent/CHANGELOG.md` record the
  release-section convention and the 0.x BC policy.

## Documentation Conventions

**Per-repo docs:**
- `README.md` — terse navigation; for the umbrella, every project rule.
- `CHANGELOG.md` — per release.
- `CLAUDE.md` (`llm-agent/CLAUDE.md`) — AI-assistant guide. Lists the
  read-first GSD planning files and the eight hard rules.
- `PROVIDER_AUTHORING.md` (`llm-agent/PROVIDER_AUTHORING.md`) — the
  contract a Go provider adapter must satisfy to claim `llm.ChatModel`
  conformance. References the canonical examples in
  `llm-agent-providers/{openai,anthropic,ollama,deepseek,minimax}`.
- `DEPRECATIONS.md` (`llm-agent/DEPRECATIONS.md`) — single source of
  truth for what is deprecated and when it disappears. Pitfall-15
  countermeasure: every `// Deprecated:` godoc comment in the tree
  must appear in this file with a target removal version.
- `docs/compatibility.md` (`llm-agent-rag/docs/compatibility.md`) — the
  written v1.x additive-only promise.

**Godoc:**
- Each importable package has a `doc.go` with a multi-paragraph package
  comment. Example: `llm-agent/doc.go` covers the five agent paradigms,
  the portability contract, the observation channels, and the tool
  subsystem (lines 1-57).
- Every exported symbol of every importable package carries a doc
  comment (recorded as "complete package- and exported-symbol-level
  doc-comment coverage" in `llm-agent-rag/CHANGELOG.md:51-53`). Spot-
  check: `llm-agent/agent.go` lines 12-76, every exported type/method
  has a `// <Name> ...` godoc.

**Comment density:**
- Comments are heavily used to record *why* (eng-review notes, memory
  budgets, back-pressure trade-offs). E.g. `llm-agent/agent.go:36-44`
  documents the `Result.Trace` memory contract with the exact
  multiplier (~4KB per Step, ~40MB wasted at 100 concurrent handlers);
  `agent.go:91-97` explains the streaming buffer-size choice.
  Of 135 Go files under `llm-agent/` (excluding `third_party/` and
  `.planning/`), 125 contain at least one full-line comment — comment
  presence is the rule, not the exception.

## Error Handling

- Sentinel errors only — callers translate to their project taxonomy
  via `errors.Is` at the boundary (`llm-agent/agent.go:127-128`).
- Wrap with `fmt.Errorf("...: %w", err)` to preserve sentinel identity
  through layers (`llm-agent/llm/scripted.go:72`, `scriptedllm_test.go:29`).
- `errors.Is` is the canonical assertion in tests, never string-match
  (`llm-agent/simple_test.go:33-36`, `agent_test.go:18-33`).
- Provider adapters expose typed error families in `llm/`:
  `llm.AuthError`, `llm.RateLimitError`, `llm.InvalidRequestError`,
  `llm.TransientError` (`llm-agent/PROVIDER_AUTHORING.md:66-77`).
- The cross-paradigm `budget_integration_test.go` asserts an "umbrella
  + dimensional" error shape: every budget exhaustion satisfies both
  `errors.Is(err, budget.ErrCallsExceeded)` AND
  `errors.Is(err, budget.ErrBudgetExceeded)` (the umbrella). See
  `budget_integration_test.go:5-15`.

## Commit Message Style

Recent samples (run `git log --oneline -20` per repo):

**`llm-agent` (Conventional-Commits style with parenthesised scope):**

```
e28c8a7 docs(agents): retire stale "deprecated"/"backwards-compat" markers
535375f test(agents): wide paradigm budget integration + stdlib-only exit gate (CC-1 / Phase 35 Wave 4)
39950e2 docs(examples): add 06-budget deterministic example (CC-1 / Phase 35 Wave 3)
d141bf6 feat(agents): wire budget enforcement into generateFromPrompt chokepoint (CC-1 / Phase 35 Wave 2)
581caea feat(budget): add ctx-keyed budget package (CC-1 / Phase 35 Wave 1)
acb3253 ci(umbrella): add dependency-currency gate for sibling repos
88db43e chore: bump llm-agent-rag to v1.0.1
```

**Conventions observed:**
- Type prefix: `feat`, `fix`, `docs`, `test`, `ci`, `chore`, `build`,
  `refactor`.
- Optional scope in parens: `feat(budget):`, `docs(agents):`,
  `ci(umbrella):`.
- Requirement traceability tag suffix in parens:
  `(CC-1 / Phase 35 Wave 1)`, `(phases 26-27)`. These link commits to
  `.planning/REQUIREMENTS.md` rows and `.planning/ROADMAP.md` phases.
- Cascade bumps follow a stable template:
  `chore: bump to llm-agent v0.5.1 + llm-agent-otel v0.2.1 + llm-agent-rag v1.0.1 (v1.1 cascade)`.

**`llm-agent-rag`, `llm-agent-otel`, `llm-agent-providers`,
`llm-agent-customer-support`:** same conventional-commits shape; the
sister repos additionally use merge-commit messages from
`gh pr merge --auto --merge` because the `pr-governance` workflow auto-
merges owner-authored PRs (see
`llm-agent-providers/.github/workflows/pr-governance.yml` jobs
`governance` + `auto-merge-owner`).

## Function Design

- Compact functions; the `Agent` interface has two methods only
  (`Name`, `Run`, plus `RunStream`) — `llm-agent/agent.go:13-21`.
- Result types are structs with documented size contracts
  (`agents.Result.Trace` memory note, `agent.go:36-44`).
- Functional options preferred over multi-arg constructors for
  optional configuration (every provider's `New(opts ...Option)`).
- Channel-based streaming surfaces are documented to a per-event level
  (`StepEvent` semantics: `Done=false` → intermediate;
  `Done=true` → exactly one of `Final` or `Err` set; channel close
  after terminal event signals end. `agent.go:23-33`).

## Module Design

**Boundaries enforced by CI:**
- Core `llm-agent` keeps a single back-edge (`llm-agent-rag`) and no
  other non-stdlib deps. Re-checked on every PR via `go mod tidy`
  drift gate.
- `llm-agent-rag` core packages may NOT import
  `github.com/costa92/llm-agent`; only `adapter/llmagent` (build-tagged
  `//go:build llmagent`) may. Enforced by the `rg`-based grep gate in
  `llm-agent-rag/.github/workflows/test.yml:24-39`.
- `replace` directives are forbidden on tagged-release branches
  (`INFRA-04`, identical `release-precheck.yml` across the four sister
  repos plus the core).
- An umbrella `dependency-currency` CI gate (added in commit
  `acb3253 ci(umbrella): add dependency-currency gate for sibling
  repos`) enforces that sister repos point at the freshest
  `llm-agent-rag` tag.

**Exports:**
- Subpackages own their public surface. No barrel/`_all.go` files.
- The umbrella's `llm-agent-rag/doc.go` deliberately makes the root
  package zero-symbol — see lines 1-12.

## CI Gate Names (referenced throughout the codebase)

| Code | What it enforces | Where |
|---|---|---|
| `INFRA-02` | CI runs with `GOWORK=off`; never picks up a workspace silently | every per-repo `test.yml` env block |
| `INFRA-04` | Tagged-release branches reject `replace` directives | `release-precheck.yml` (every repo) |
| `INFRA-06` | Cross-repo iteration pattern (coordinated bump + retag wave) | `llm-agent-customer-support/README.md:77`, `llm-agent-providers/README.md:79` |
| `35-04` / `CC-1` | Budget chokepoint enforcement at `generateFromPrompt` | `llm-agent/budget_integration_test.go`, `reflection_test.go:55`, `plan_solve_test.go:88` |
| `K1` | Streaming events are a typed union with stable `Index` | root `README.md`, `llm-agent/CLAUDE.md` hard rules |
| `K2` | Capabilities are per-(provider × model), not per-provider | root `README.md`, OpenAI/Anthropic/Ollama `TestInfo_*` |
| `K3` | OTel attaches as decorator wrappers (`Wrap(inner) ChatModel`) — never hooks | `llm-agent-otel/otelmodel`, `otelrag`, `otelagent` |
| `K7` | Refsvc hard caps + `DISABLE_LLM=1` panic switch from Day 1 | `llm-agent-customer-support/README.md:73` |
| `OLL-08` | Nightly conformance against a real Ollama container | `llm-agent-providers/.github/workflows/nightly-ollama-live.yml` |
| `pr-governance` | Owner-only approvals + auto-merge for owner PRs | `llm-agent-otel`, `llm-agent-providers`, `llm-agent-customer-support` |

## The `.planning/` Workflow Convention

**Two-tier layout:**

1. **Umbrella `.planning/` (slim):** at the ecosystem root
   (`/.planning/`). Contains `PROJECT.md`, `README.md`, and the
   generated `codebase/` mapping documents. Used only for cross-repo
   coordination — milestones spanning multiple modules, dependency-
   alignment notes, release-wave checklists. The README explicitly
   says: *"Do not copy subproject-local plans here unless the change
   spans multiple repos."*

2. **Source-of-truth `.planning/` (rich):** at the core repo
   (`llm-agent/.planning/`). Contains:
   - `PROJECT.md` — what the project IS, core value, hard rules
   - `STATE.md` — current milestone + active phase (front-matter YAML
     header with `gsd_state_version`, `milestone`, `status`,
     `progress`)
   - `ROADMAP.md` — phase plan for the active milestone
   - `REQUIREMENTS.md` — milestone requirements with traceability
   - `research/` — keystone decision dossiers (K1–K7, KC-1..KC-4)
   - `phases/<NN>-<slug>/` — per-phase plans and audits
   - `milestones/` — archived per-milestone roadmaps
   - `v<N>-MILESTONE-AUDIT.md` — close-out audits, one per shipped
     milestone (`v0.3` through `v1.1` present)
   - `config.json` — GSD workflow toggles (granularity, gates,
     parallelization)
   - `todos/` — running todo lists

**Rationale:** the umbrella root owns navigation and conventions, not
product code; the core repo owns the source-of-truth planning because
that's where most cross-cut decisions land. Sister repos keep their
own focused `README.md` and (when present) repo-local planning notes,
but they defer cross-cut milestone state to `llm-agent/.planning/`.

This split is recorded at the ecosystem root `README.md` lines 76-95
("Root planning" / "Source of truth for planning") and at the umbrella
`.planning/README.md`.

---

*Convention analysis: 2026-05-20*
