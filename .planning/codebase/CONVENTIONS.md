<!-- refreshed: 2026-05-21 -->
# Coding Conventions

**Analysis Date:** 2026-05-21

This document captures the conventions enforced across the umbrella
`llm-agent-ecosystem`. Each subproject keeps its own repo-local CI gates,
but all six share the conventions below. Code samples are taken from the
working tree as of the analysis date.

## Naming Patterns

**Modules:**
- Module paths follow the GitHub canonical form
  `github.com/costa92/llm-agent[-<facet>]`. Six modules exist:
  `llm-agent`, `llm-agent-rag`, `llm-agent-otel`, `llm-agent-providers`,
  `llm-agent-customer-support`, `llm-agent-flow`. See each `go.mod`.
- `llm-agent-rag` deliberately names its root package `ragkit` instead of
  the module's basename — recorded in `llm-agent-rag/doc.go` as an
  intentional short brand identifier, not an accidental mismatch.
- `llm-agent-flow` uses package `flow` for the library (`flow/` directory)
  and `cmd/flow` + `cmd/flowd` for binaries; the module-root `doc.go`
  carries the package overview.

**Packages:**
- One-word, all lowercase, importable: `agents`, `llm`, `budget`,
  `rag`, `retrieve`, `store`, `embed`, `ingest`, `generate`, `pack`,
  `prompt`, `rerank`, `graph`, `eval`, `otelmodel`, `otelrag`,
  `otelagent`, `otelmetrics`, `otelslog`, `otelflow`, `openai`,
  `anthropic`, `ollama`, `deepseek`, `minimax`, `app`, `httpapi`,
  `guardrails`, `sessionstore`, `supportflow`, `flowrunner`,
  `knowledgebase`, `limits`, `config`, `flow`, `cond`, `cel`,
  `sqlite`, `tools`, `server`.
- The core repo's root Go package is `agents`, not `llm-agent` — the
  repository name and Go package name diverge by design (see
  `llm-agent/doc.go` lines 1-5).

**Files:**
- Implementation: `snake_case.go` for multi-word files (`agent_chatmodel.go`,
  `function_call.go`, `plan_solve.go`, `budget_integration_test.go`,
  `tool_node.go`, `engine_cond_test.go`, `events_batch_test.go`).
- Tests: paired `<impl>_test.go` (`simple.go` ↔ `simple_test.go`,
  `chain.go` ↔ `chain_test.go`, `engine.go` ↔ `engine_test.go`,
  `auth.go` ↔ `auth_test.go`).
- Example tests: `example_<scenario>_test.go` (e.g.
  `llm-agent/example_simple_test.go`, `example_tool_use_test.go`,
  `llm-agent-flow/examples/echo_chain/example_test.go`).
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
  `budget.Budget`, `budget.Tracker`, `flow.Engine`, `flow.Runner`,
  `flow.FlowEvent`, `flow.NodeRegistry`, `server.Authenticator`,
  `server.BearerTokenAuthenticator`.
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

  ```go
  // llm-agent-flow/cmd/flowd/server/auth.go:26
  var ErrUnauthorized = errors.New("unauthorized")

  // llm-agent-flow/flow/store/store.go (excerpt)
  var ErrNotFound = errors.New("flow/store: not found")
  ```

  Error message always starts with `"<package>: <reason>"` (with a small
  exception for `flowd/server/auth.go` where the literal `"unauthorized"`
  is the contract a 401 response surfaces). Callers test via `errors.Is`
  (see `llm-agent/agent_test.go:18-33`,
  `llm-agent-flow/cmd/flowd/server/auth_test.go:55-58` for 401 mapping).
- Functional-options constructors: `New<Type>(opts ...<Type>Option)` with
  per-option helpers `With<Field>(value)` (see
  `llm-agent/llm/scripted.go:50-64`, `llm-agent-providers/openai`,
  `llm-agent-providers/anthropic`, `llm-agent-flow/flow.WithConditionEvaluator`,
  `llm-agent-flow/flow.WithMaxNodeConcurrency`,
  `llm-agent-otel/otelflow.Wrap(inner, Config{...})`).
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

  `llm-agent-flow` mirrors this idiom: `var _ flow.Runner = (*Engine)(nil)`
  (`llm-agent-flow/flow/runner.go`) so the seam consumed by
  `otelflow.Wrap` is compile-pinned.

**Variables / constants:**
- `StepKind` enum is a typed string with lowercase string values
  (`"thought"`, `"action"`, `"observation"`, `"reflection"`, `"plan"`,
  `"final"`); pinned by `TestStepKind_Constants`
  (`llm-agent/agent_test.go:35-49`).
- `flow.FlowEventKind` follows the same typed-string convention —
  values match the SSE event names so a single decoder serves live and
  replayed streams (`llm-agent-flow/flow/event.go`).
- `flow/store.RunEventKind` (`flow_started`, `node_started`,
  `node_finished`, `node_skipped`, `flow_done`, `flow_err`) is the
  durable mirror of `flow.FlowEventKind` and shares the exact string
  values (`llm-agent-flow/flow/store/store.go`).

## Code Style

**Formatting:**
- `gofmt` is mandatory; the `llm-agent-customer-support` CI workflow
  promotes it to a hard gate
  (`llm-agent-customer-support/.github/workflows/test.yml` job
  `format`, step `gofmt (drift check)`). Other repos rely on `go vet`
  + `go mod tidy` drift checks and treat `gofmt`-cleanness as a
  norm. The `2abd67f` / `14403a9` commits in `llm-agent-customer-support`
  document the discipline in action — a `gofmt` drift on `flowrunner`
  was caught and corrected in a follow-up PR.
- No `.golangci.yml`, `.editorconfig`, or external linter config is
  committed anywhere — the toolchain is pure `go vet` + `gofmt` +
  `go mod tidy`.

**Toolchain:**
- All six modules pin `go 1.26.0` in their `go.mod`. CI uses
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
  `llm-agent-customer-support/.github/workflows/test.yml`,
  `llm-agent-flow/.github/workflows/test.yml`.
- `llm-agent` runs the same gauntlet a second time inside `examples/`
  (the examples module has its own `go.mod`).
- `llm-agent-rag` adds a `core module boundary` gate that `rg`-greps for
  forbidden imports of `github.com/costa92/llm-agent` outside
  `adapter/` (test.yml lines 24-39) and a `Build-tagged adapter
  (llmagent)` gate that runs the `//go:build llmagent` slice.
- `llm-agent-flow`'s `test.yml` carries the same four-step gauntlet;
  the API snapshot gate is enforced inside `go test ./...` (see Public
  API Discipline below) rather than as a separate CI step.

## Import Organization

**Order (observed in every file inspected):**
1. Stdlib (`context`, `encoding/json`, `errors`, `fmt`, `net/http`,
   `testing`, `sync`, `time`, etc.)
2. Blank line separator
3. Third-party / sibling-repo (`github.com/costa92/llm-agent/...`,
   `github.com/costa92/llm-agent-rag/...`,
   `github.com/costa92/llm-agent-flow/...`,
   `go.opentelemetry.io/...`, `go.uber.org/goleak`)
4. Same-module subpackages

Example: `llm-agent/budget_integration_test.go:17-26`,
`llm-agent-otel/otelmodel/otelmodel_test.go:1-14`,
`llm-agent-otel/otelflow/otelflow_test.go:1-17`,
`llm-agent-customer-support/internal/flowrunner/flowrunner.go:1-11`.

**Path Aliases:**
- None. Plain Go-module paths only. The umbrella never uses Go module
  `replace` directives in any committed `go.mod` (verified by
  `grep -rn "^replace " llm-agent*/go.mod` returning no matches), and a
  CI gate (`INFRA-04`, `release-precheck.yml`) refuses to tag a commit
  whose `go.mod` carries a `replace`.

## Public API Discipline

**Two stability tiers exist as of v0.1.0 of `llm-agent-flow` (2026-05-21):**

| Tier | Repos | Guarantee |
|---|---|---|
| **v1.x frozen, additive-only** | `llm-agent-rag` v1.x | No exported symbol removed, renamed, or re-signed within the series. Snapshot gate `internal/apisnapshot/` + compile-pin `contract/`. |
| **v0.1.x frozen, additive-only** | `llm-agent-flow` v0.1.x | Same rules as v1.x but starting at v0.1.0; snapshot gate at `internal/apisnapshot/`. Pre-v0.1.0 (v0.0.x) was an exploration band with no stability promise. |
| **SemVer 0.x BC** | `llm-agent`, `llm-agent-otel`, `llm-agent-providers`, `llm-agent-customer-support` | Patch is BC. Minor `0.x → 0.y` may break. No snapshot gate yet. |

**`llm-agent-rag` (frozen v1.x):**
- Within the `v1.x` series, the public API is **additive-only**.
  Exported symbols are not renamed, removed, or re-signed. Breaking
  changes go to `/v2` (separate module path).
  Source: `llm-agent-rag/docs/compatibility.md:18-89`,
  `llm-agent-rag/CHANGELOG.md:9-39`.
- **Two gates in parallel:**
  1. **Whole-surface snapshot gate** at
     `llm-agent-rag/internal/apisnapshot/` — pure stdlib
     (`go/parser` + `go/printer`); compares the rendered surface
     against `llm-agent-rag/api/v1.snapshot.txt`. Regenerate via
     `go test ./internal/apisnapshot/ -run TestAPISnapshot -update`.
  2. **Cross-repo compile-pin** at `llm-agent-rag/contract/contract_test.go`
     — pins the subset of symbols current `llm-agent` integrations
     consume. Any rename or removal breaks `go build` on this file,
     so `go test ./...` is the contract gate.
- The root package `ragkit` exports nothing; callers import the
  subpackages (`rag`, `retrieve`, `store`, `embed`, `ingest`,
  `generate`, `eval`, ...). This anchors documentation without
  freezing a root surface.

**`llm-agent-flow` (frozen v0.1.x, new at this analysis):**
- Within `v0.1.x` the exported API of every importable package is
  **additive-only** (same rules as rag v1.x). Source:
  `llm-agent-flow/docs/compatibility.md:1-70`.
- **Single gate:** whole-surface snapshot at
  `llm-agent-flow/internal/apisnapshot/` (`apisnapshot.go` 343 lines,
  `apisnapshot_test.go` 118 lines, baseline
  `llm-agent-flow/api/v0.1.snapshot.txt` 207 lines). Same pure-stdlib
  `go/parser` + `go/printer` shape as rag's; the file header literally
  reads `# llm-agent-flow v0.1 exported API snapshot — generated, do
  not hand-edit.` Regenerate with
  `go test ./internal/apisnapshot/ -run TestAPISnapshot -update`.
- No `contract/` compile-pin yet (flow has no cross-repo dependents
  beyond `llm-agent-otel/otelflow` and
  `llm-agent-customer-support/internal/flowrunner`, both of which are
  themselves under SemVer 0.x). The snapshot is the only freeze.
- Breaking changes go to `github.com/costa92/llm-agent-flow/v2`.
- HTTP endpoints, JSON IR additions, internal/ packages, and CLI
  flag additions are explicitly **out of scope** for the v0.1 freeze
  (see `llm-agent-flow/docs/compatibility.md` "What v0.1 does NOT
  cover").

**`llm-agent` (core):**
- Stays stdlib-only. The only allowed dependency line in `go.mod` is
  the `llm-agent-rag` back-edge for the RAG facade
  (`require github.com/costa92/llm-agent-rag v1.0.1`). The
  `llm-agent/go.sum` is correspondingly tiny.
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
- No additive-only freeze yet — they remain in 0.x. A future
  ecosystem-alignment milestone may promote `otel` / `providers`
  to v1.x once their public surfaces settle.

## Authentication & Security Primitives

**First explicit security primitive at the HTTP layer (new at v0.0.8 / v0.1.0 of `llm-agent-flow`):**

The ecosystem's first HTTP-layer auth primitive lives in `llm-agent-flow`:

```go
// llm-agent-flow/cmd/flowd/server/auth.go:19-34
type Authenticator interface {
    Authenticate(r *http.Request) error
}

var ErrUnauthorized = errors.New("unauthorized")

type BearerTokenAuthenticator struct {
    Token string
}
```

**Conventions established by this primitive (and expected to extend to other
HTTP surfaces in future ecosystem milestones):**

- **Pluggable interface, not framework magic.** `Authenticator` is a
  one-method interface so callers can wire JWT, mTLS, OAuth, or IP
  allowlist by satisfying it. The bundled `BearerTokenAuthenticator`
  is just one implementation — see `auth_test.go:109-114` for a custom
  `X-Internal-Caller` implementation in the tests.
- **Backward compatibility by zero value.** `Config.Authenticator == nil`
  short-circuits to pass-through, preserving the pre-v0.0.8 open API.
  An empty `BearerTokenAuthenticator{}.Token` also short-circuits, so
  partially-configured deployments fail open, not closed. This is
  documented as deliberate ("for backward compatibility with v0.0.7
  callers" — `auth.go:30-31`).
- **Health-check bypass.** `/healthz` is hard-coded to skip the
  authenticator so k8s liveness / load-balancer probes work without a
  token (`auth.go:69-75`).
- **Constant-time comparison.** `BearerTokenAuthenticator.Authenticate`
  uses a manual byte-XOR loop with length-prefix gate
  (`auth.go:54-65`) to avoid timing-side-channel leaks. The comment
  flags `crypto/subtle` as ideal but acceptable to defer for v0.0.x.
- **Error → status mapping is by sentinel.** Returning `ErrUnauthorized`
  → 401 + `WWW-Authenticate: Bearer realm="flowd"`; any other non-nil
  error → 403. `withAuth` middleware does the mapping
  (`auth.go:80-100`).
- **CLI flag + env var.** `cmd/flowd --token <secret>` or
  `FLOWD_TOKEN` environment variable (`cmd/flowd/main.go:92-94`).

`llm-agent-customer-support`'s `/chat` HTTP surface still has no auth
(documented demo-only limitation). When the customer-support service
acquires auth it should reuse the `flowd/server.Authenticator` shape
(or a generalization promoted out of flow into a shared sub-package)
so the ecosystem converges on one interface.

## Compatibility Docs

**Written stability promises live as compatibility docs alongside each
frozen repo:**

| Repo | Doc | Purpose |
|---|---|---|
| `llm-agent-rag` | `llm-agent-rag/docs/compatibility.md` | v1.x additive-only promise; calls out import-compatibility, struct-field stability, and `/v2` escape hatch. |
| `llm-agent-rag` | `llm-agent-rag/docs/core-compatibility.md` | How the rag↔core split works and which features cross the boundary. |
| `llm-agent-flow` | `llm-agent-flow/docs/compatibility.md` | v0.1.x additive-only promise; mirrors rag's structure 1:1. |
| `llm-agent-flow` | `llm-agent-flow/docs/architecture.md` | Internal architecture (engine, store, server). |

Both compatibility docs ship under `docs/compatibility.md` so the
convention is grep-able across the umbrella. A future v1.x freeze on
any other repo is expected to add the same file.

## Versioning

**SemVer with the following live tags (as of 2026-05-21):**

| Module | Current tag | Default branch |
|---|---|---|
| `llm-agent` | `v0.5.1` (8 commits ahead — v1.2 Phase 35 in flight) | `main` |
| `llm-agent-rag` | `v1.0.2` | `master` |
| `llm-agent-otel` | `v0.2.2` (added `otelflow/`) | `main` |
| `llm-agent-providers` | `v0.2.2` | `main` |
| `llm-agent-customer-support` | `v0.2.3` (added `internal/flowrunner/`) | `main` |
| `llm-agent-flow` | `v0.1.1` (freeze + LRU + event batching) | `main` |

Roster source: root `README.md` lines 30-38, with cascade-bump tags
visible via `git log --oneline` in each sister repo.

**Per-repo `CHANGELOG.md`:**
- Format: Keep-a-Changelog 1.1 (sections: Added / Changed /
  Deprecated / Removed / Fixed / Security / Breaking).
- Each release entry carries the date in `YYYY-MM-DD`.
- 0.x bumps may break across minor (`0.x → 0.y` where `y > x`); patch
  is BC. v0.1.x (`llm-agent-flow`) and v1.x (`llm-agent-rag`) are
  additive-only as above.
- Header comments at the top of `llm-agent/CHANGELOG.md` record the
  release-section convention and the 0.x BC policy.
- `llm-agent-flow/CHANGELOG.md` is the newest reference: every
  release (v0.0.1 through v0.1.1) has an `Added` / `Changed` /
  `Tests` block, and v0.1.0 plus v0.1.1 each include a "Snapshot
  baseline" footer noting whether `api/v0.1.snapshot.txt` was
  regenerated.

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
  truth for what is deprecated and when it disappears.
- `docs/compatibility.md` — the written additive-only promise (rag v1.x
  + flow v0.1.x).

**Godoc:**
- Each importable package has a `doc.go` with a multi-paragraph package
  comment. Example: `llm-agent/doc.go` covers the five agent paradigms,
  the portability contract, the observation channels, and the tool
  subsystem (lines 1-57). `llm-agent-flow/doc.go`, `flow/doc.go`,
  `flow/store/doc.go`, `flow/store/sqlite/doc.go`,
  `llm-agent-otel/otelflow/doc.go`,
  `llm-agent-customer-support/internal/flowrunner/doc.go` all follow
  the same convention.
- Every exported symbol of every importable package carries a doc
  comment.

## Error Handling

- Sentinel errors only — callers translate to their project taxonomy
  via `errors.Is` at the boundary (`llm-agent/agent.go:127-128`,
  `llm-agent-flow/flow/store/store.go` for `ErrNotFound`).
- Wrap with `fmt.Errorf("...: %w", err)` to preserve sentinel identity
  through layers (`llm-agent/llm/scripted.go:72`, `scriptedllm_test.go:29`,
  `llm-agent-flow/flow/store/sqlite/events.go:30`).
- `errors.Is` is the canonical assertion in tests, never string-match
  (`llm-agent/simple_test.go:33-36`, `agent_test.go:18-33`,
  `llm-agent-flow/flow/store/sqlite/events_batch_test.go:56-60`).
- Provider adapters expose typed error families in `llm/`:
  `llm.AuthError`, `llm.RateLimitError`, `llm.InvalidRequestError`,
  `llm.TransientError` (`llm-agent/PROVIDER_AUTHORING.md:66-77`).
- The cross-paradigm `budget_integration_test.go` asserts an "umbrella
  + dimensional" error shape: every budget exhaustion satisfies both
  `errors.Is(err, budget.ErrCallsExceeded)` AND
  `errors.Is(err, budget.ErrBudgetExceeded)`
  (`budget_integration_test.go:5-15`).
- `flowd/server/auth.go` uses sentinel-as-status-mapper:
  `errors.Is(err, ErrUnauthorized)` → 401, any other error → 403.

## Commit Message Style

Recent samples (run `git log --oneline -20` per repo):

**`llm-agent-flow` (the new entrant; Conventional-Commits with version tag):**

```
cfbf3a9 perf: engine cache LRU + sync-run event batching (v0.1.1)
d0e7e62 chore: v0.1.0 — SemVer freeze + API snapshot gate
e6d9f47 docs: CHANGELOG for v0.0.9
19b9123 feat(flowd): POST /runs/{id}/replay — SSE replay of persisted events (v0.0.9)
9aeb070 feat(flowd): bearer-token auth + pluggable Authenticator (v0.0.8)
647e006 feat(flow): Runner interface + Engine.FlowID/FlowName getters (v0.0.7)
```

**`llm-agent` (Conventional-Commits with parenthesised scope + traceability tag):**

```
e28c8a7 docs(agents): retire stale "deprecated"/"backwards-compat" markers
535375f test(agents): wide paradigm budget integration + stdlib-only exit gate (CC-1 / Phase 35 Wave 4)
d141bf6 feat(agents): wire budget enforcement into generateFromPrompt chokepoint (CC-1 / Phase 35 Wave 2)
581caea feat(budget): add ctx-keyed budget package (CC-1 / Phase 35 Wave 1)
```

**`llm-agent-otel` (cascade + feature commits):**

```
e7f1b69 feat(otelflow): OTel wrapper for llm-agent-flow.Runner (v0.2.2)
d5f0fa7 chore: bump to llm-agent v0.5.1 + llm-agent-rag v1.0.1 (v1.1 cascade)
```

**`llm-agent-customer-support` (cascade + feature commits):**

```
14403a9 style: gofmt drift fix for flowrunner + pre-existing files
002f43a feat(flowrunner): bridge to llm-agent-flow + otelflow (v0.2.3)
5984f3b chore: bump llm-agent-providers to v0.2.1 (v1.1 cascade follow-up — topological-order fix)
```

**Conventions observed:**
- Type prefix: `feat`, `fix`, `docs`, `test`, `ci`, `chore`, `build`,
  `refactor`, `perf`, `style`.
- Optional scope in parens: `feat(budget):`, `docs(agents):`,
  `feat(flowd):`, `feat(otelflow):`, `feat(flowrunner):`.
- Trailing `(vX.Y.Z)` parenthetical for release commits (`flow` style)
  OR trailing `(CC-1 / Phase 35 Wave 1)` traceability tag (core style).
  Both shapes are accepted; pick one per repo.
- Cascade bumps follow a stable template:
  `chore: bump to llm-agent v0.5.1 + llm-agent-otel v0.2.1 + llm-agent-rag v1.0.1 (v1.1 cascade)`.

## Function Design

- Compact functions; the `Agent` interface has two methods only
  (`Name`, `Run`, plus `RunStream`) — `llm-agent/agent.go:13-21`.
- `flow.Runner` is similarly tight: exactly `Run(ctx, inputs)` and
  `RunStream(ctx, inputs)` (`llm-agent-flow/flow/runner.go`). This is
  the seam `otelflow.Wrap` and `flowrunner.compileAndWrap` consume.
- Result types are structs with documented size contracts
  (`agents.Result.Trace` memory note, `agent.go:36-44`).
- Functional options preferred over multi-arg constructors for
  optional configuration (every provider's `New(opts ...Option)`,
  `flow.WithMaxNodeConcurrency`, `flow.WithConditionEvaluator`).
- Channel-based streaming surfaces are documented to a per-event level
  (`StepEvent` semantics: `Done=false` → intermediate;
  `Done=true` → exactly one of `Final` or `Err` set; channel close
  after terminal event signals end. `agent.go:23-33`).
  `flow.FlowEvent` follows the same shape: `FlowStarted` first,
  `FlowDone | FlowErr` exactly once, channel close after terminal
  (`llm-agent-flow/flow/event.go`).

## Module Design

**Boundaries enforced by CI:**
- Core `llm-agent` keeps a single back-edge (`llm-agent-rag`) and no
  other non-stdlib deps. Re-checked on every PR via `go mod tidy`
  drift gate.
- `llm-agent-rag` core packages may NOT import
  `github.com/costa92/llm-agent`; only `adapter/llmagent` (build-tagged
  `//go:build llmagent`) may. Enforced by the `rg`-based grep gate in
  `llm-agent-rag/.github/workflows/test.yml:24-39`.
- `llm-agent-flow` library (`flow/`) is stdlib-only outside the
  back-edge to `github.com/costa92/llm-agent`. cel-go is isolated in
  `flow/cond/cel/` (a separate sub-package) so consumers who never
  use conditional edges do not pay the cel-go dependency at link time
  for the library — though it remains in `go.mod` and `go.sum`
  (see CONCERNS.md).
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
- `llm-agent-flow/doc.go` makes the module root a thin package overview;
  product types live under `flow/` and `flow/store/`.

**Optional capabilities via type assertion:**
`llm-agent-flow` introduces a pattern that may spread to other repos:
the `flow/store.Store` interface stays minimal (read/write/list), and
bulk operations are exposed as **optional capabilities** discovered by
type assertion:

```go
// cmd/flowd/server/server.go:430-433
batcher, ok := s.cfg.Store.(interface {
    AppendRunEvents(ctx context.Context, runID string, items []flowstore.RunEventBatchItem) error
})
```

This keeps the `Store` interface source-compatible across v0.1.x (the
freeze) while letting performance-minded implementations opt into
batched persistence. Mirrors the K2 pattern (capability surface
declared by satisfying an interface, not by adding a method to a
core interface).

## CI Gate Names (referenced throughout the codebase)

| Code | What it enforces | Where |
|---|---|---|
| `INFRA-02` | CI runs with `GOWORK=off`; never picks up a workspace silently | every per-repo `test.yml` env block |
| `INFRA-04` | Tagged-release branches reject `replace` directives | `release-precheck.yml` (every repo) |
| `INFRA-06` | Cross-repo iteration pattern (coordinated bump + retag wave) | `llm-agent-customer-support/README.md:77`, `llm-agent-providers/README.md:79` |
| `35-04` / `CC-1` | Budget chokepoint enforcement at `generateFromPrompt` | `llm-agent/budget_integration_test.go`, `reflection_test.go:55`, `plan_solve_test.go:88` |
| `K1` | Streaming events are a typed union with stable `Index` | root `README.md`, `llm-agent/CLAUDE.md` hard rules; mirrored by `flow.FlowEvent` |
| `K2` | Capabilities are per-(provider × model), not per-provider | root `README.md`, OpenAI/Anthropic/Ollama `TestInfo_*` |
| `K3` | OTel attaches as decorator wrappers (`Wrap(inner) ChatModel`) — never hooks | `llm-agent-otel/otelmodel`, `otelrag`, `otelagent`; extended to `otelflow.Wrap(flow.Runner) flow.Runner` |
| `K7` | Refsvc hard caps + `DISABLE_LLM=1` panic switch from Day 1 | `llm-agent-customer-support/README.md:73` |
| `OLL-08` | Nightly conformance against a real Ollama container | `llm-agent-providers/.github/workflows/nightly-ollama-live.yml` |
| `pr-governance` | Owner-only approvals + auto-merge for owner PRs | `llm-agent-otel`, `llm-agent-providers`, `llm-agent-customer-support` |
| **API snapshot gate** | Whole-surface diff against committed `*.snapshot.txt` baseline | `llm-agent-rag/internal/apisnapshot/`, `llm-agent-flow/internal/apisnapshot/` (runs inside `go test ./...`, no separate workflow step) |

## The `.planning/` Workflow Convention

**Two-tier layout:**

1. **Umbrella `.planning/` (slim):** at the ecosystem root
   (`/.planning/`). Contains `PROJECT.md`, `README.md`, and the
   generated `codebase/` mapping documents. Used only for cross-repo
   coordination — milestones spanning multiple modules, dependency-
   alignment notes, release-wave checklists.

2. **Source-of-truth `.planning/` (rich):** at the core repo
   (`llm-agent/.planning/`). Contains:
   - `PROJECT.md` — what the project IS, core value, hard rules
   - `STATE.md` — current milestone + active phase (front-matter YAML
     header with `gsd_state_version`, `milestone`, `status`,
     `progress`)
   - `ROADMAP.md` + `v1.2-ROADMAP.md` — phase plan for the active milestone
   - `REQUIREMENTS.md` + `v1.2-REQUIREMENTS.md` — milestone requirements with traceability
   - `research/` — keystone decision dossiers (K1–K7, KC-1..KC-4)
   - `phases/<NN>-<slug>/` — per-phase plans and audits
   - `milestones/` — archived per-milestone roadmaps
   - `v<N>-MILESTONE-AUDIT.md` — close-out audits, one per shipped
     milestone (`v0.3` through `v1.1` present)
   - `config.json` — GSD workflow toggles
   - `todos/` — running todo lists

**Rationale:** the umbrella root owns navigation and conventions, not
product code; the core repo owns the source-of-truth planning because
that's where most cross-cut decisions land. Sister repos keep their
own focused `README.md` and (when present) repo-local planning notes,
but they defer cross-cut milestone state to `llm-agent/.planning/`.

This split is recorded at the ecosystem root `README.md` and at the
umbrella `.planning/README.md`.

---

*Convention analysis: 2026-05-21*
