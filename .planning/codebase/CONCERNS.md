# Codebase Concerns

**Analysis Date:** 2026-05-21

Scope: the `llm-agent-ecosystem` umbrella root and what it sees of its 6
subprojects. Concerns are scoped to what the umbrella owns (navigation,
conventions, cross-repo coordination) and to risks the umbrella is positioned
to detect or mitigate. Subproject-internal concerns are flagged only where
they materially affect the umbrella's contract or where they create drift
visible from the root.

## Resolved since v1.1-close audit

Concerns from the 2026-05-20 audit that have been **addressed** and are
no longer open:

- **Umbrella README claims v1.1 in flight / "shipped 2026-05-21" date drift.**
  Fixed. `README.md:125-127` now records v1.1 shipped 2026-05-20, v1.0
  shipped 2026-05-21, and v1.2 Core Capability Deepening as the active
  milestone. Date conflict resolved.
- **v1.2 milestone activity invisible from umbrella docs.** Fixed.
  `README.md:131-138` now mentions v1.2, the `budget` / `policy` /
  `orchestrate.Supervisor` packages, Phase 35 active, and the source-of-
  truth pointer at `llm-agent/.planning/STATE.md`.
- **`llm-agent-customer-support` 3 commits past tag.** Resolved by the
  `v0.2.3` tag (`002f43a feat(flowrunner): bridge to llm-agent-flow +
  otelflow`).
- **`llm-agent-rag` CHANGELOG missing `v1.0.1` entry.** Fixed at
  `07e2a3c docs: backfill v1.0.1 CHANGELOG entry`. `CHANGELOG.md:9-22`
  now carries the v1.0.1 row.
- **`scripts/eco.sh` launchable repo list hard-coded twice.** Still
  technically duplicated (`launchable_repos[]` at line 14-17 and the
  `case` block at line 32-35), but this is a 4-line pattern and the
  comment burden is minimal — downgraded to low / no action.

## Known incomplete work

### `llm-agent-rag` tag `v1.0.2` exists but has no CHANGELOG entry

- Description: `git -C llm-agent-rag tag --sort=-v:refname` lists
  `v1.0.2` as the latest tag, and the umbrella `README.md:34` already
  cites `v1.0.2`. The CHANGELOG (`grep -n "^## " llm-agent-rag/CHANGELOG.md`)
  jumps from `v1.0.1` (2026-05-20) straight to `v1.0.0` (2026-05-21);
  there is no `v1.0.2` row. Identical to the now-resolved v1.0.1
  problem; the discipline has slipped on the next patch.
- Evidence:
  - `git -C llm-agent-rag tag --sort=-v:refname` → `v1.0.2`, `v1.0.1`, ...
  - `llm-agent-rag/CHANGELOG.md:9` jumps from v1.0.1 to v1.0.0
  - `README.md:34` references v1.0.2
- Severity: medium
- Suggested next action: open a tiny PR in `llm-agent-rag` adding a
  `## [v1.0.2] - <ship-date>` block with the actual patch contents
  (likely doc / surface-doc per the `e0c5e1c docs: surface
  advanced/agentic/feedback/guard/obs as v1 surface` commit). The
  umbrella's dependency-currency gate cannot fix sister-repo CHANGELOGs
  but should flag them at audit time.

### `llm-agent-flow` is missing from the umbrella CI workflow

- Description: `.github/workflows/umbrella.yml` checks out the five
  pre-existing sister repos (`llm-agent`, `llm-agent-rag`,
  `llm-agent-otel`, `llm-agent-providers`, `llm-agent-customer-support`)
  and runs the build/test gauntlet on each. It does **not** check out
  `llm-agent-flow`. Cross-repo regressions touching flow + otelflow
  (sister) + flowrunner (downstream) would not be caught by the
  umbrella job — they would only surface in each repo's solo CI.
- Evidence:
  - `.github/workflows/umbrella.yml:21-55` six explicit `checkout`
    steps; no `llm-agent-flow` entry
  - `llm-agent-customer-support/go.mod:23` and
    `llm-agent-otel/go.mod:21` both require `llm-agent-flow` (at
    `v0.1.1` and `v0.0.7` respectively — see next concern)
- Severity: high
- Suggested next action: add a sixth `checkout` step + a sixth
  per-repo `Build llm-agent-flow` block to
  `.github/workflows/umbrella.yml`. Add `llm-agent-flow` to the
  umbrella's `dep-currency-check.sh` audit set (currently invoked from
  `llm-agent`'s workflow — see below).

### `llm-agent-otel` v0.2.2 still pins `llm-agent-flow v0.0.7`, not v0.1.1

- Description: `llm-agent-otel/go.mod:21` requires
  `github.com/costa92/llm-agent-flow v0.0.7`. The current
  `llm-agent-flow` tag is `v0.1.1` and `llm-agent-customer-support/go.mod:23`
  already requires `v0.1.1`. `otelflow` is the wrapper for the
  `flow.Runner` interface introduced in flow v0.0.7; the wrapper has
  not been re-pinned to the stable v0.1.x line. This means
  `llm-agent-customer-support` transitively brings in **two flow
  versions** (`go.mod` requires v0.1.1 explicitly, otel transitively
  pulls v0.0.7); MVS will pick v0.1.1, so the build still works, but
  the umbrella's "dep-currency" guarantee is violated for one edge.
- Evidence:
  - `llm-agent-otel/go.mod:21` `github.com/costa92/llm-agent-flow v0.0.7`
  - `llm-agent-customer-support/go.mod:23`
    `github.com/costa92/llm-agent-flow v0.1.1`
  - `llm-agent-flow` latest tag: `v0.1.1`
- Severity: medium
- Suggested next action: cascade-bump `llm-agent-otel` to require
  `llm-agent-flow v0.1.1` and re-tag (likely `v0.2.3`). Then bump
  `llm-agent-customer-support` to require the new `otel` tag (likely
  `v0.2.4`). This is exactly the "topological-order cascade" pattern
  the v1.1 audit recorded (see `llm-agent/.planning/v1.1-MILESTONE-AUDIT.md:177-186`).
  Add `llm-agent-flow` to the dep-currency gate's audit set as part
  of the same wave.

## Cross-repo coordination risks

### Dep-currency gate lives in `llm-agent`, not in the umbrella root

- Description: The strict ecosystem dep-currency check is implemented at
  `llm-agent/scripts/dep-currency-check.sh` and invoked from
  `llm-agent/.github/workflows/umbrella.yml`. The umbrella root's own
  workflow `.github/workflows/umbrella.yml` only does cross-repo build/test
  and validates root files — it does **not** invoke the dep-currency
  check. The gate's coverage is therefore conditional on `llm-agent`
  staying alive and reachable; if `llm-agent`'s remote becomes
  unavailable or its workflow is restructured, the umbrella loses the
  gate silently. Worse, the gate does not yet know about
  `llm-agent-flow` (see the open concern above).
- Evidence:
  - `.github/workflows/umbrella.yml:62-103` (umbrella root — no
    `dep-currency-check.sh` reference)
  - `llm-agent/scripts/dep-currency-check.sh` is the script
- Severity: high
- Suggested next action: vendor a thin wrapper (e.g.
  `scripts/dep-currency-check.sh` in this repo, or a re-checkout step
  in the umbrella workflow) that runs the script from whichever sister
  repo hosts it. Same recommendation as the v1.1 audit; still open.

### Strict-equality dep-currency gate has documented exemption (`rag → core`)

- Description: `llm-agent-rag/go.mod` requires `github.com/costa92/llm-agent
  v0.5.0`, but the current latest core tag is `v0.5.1`. The dep-currency
  gate handles this with a hard-coded skip (`KE-2 corollary` exemption).
  Documented and intentional, but it is the only auditable strict-
  equality exemption in the system. A future second exemption would
  erode the gate.
- Evidence:
  - `llm-agent-rag/go.mod` (`v0.5.0` while latest is `v0.5.1`)
  - `llm-agent/scripts/dep-currency-check.sh` exemption block
- Severity: medium
- Suggested next action: add an explicit `EXEMPTIONS` section in the
  umbrella `README.md` listing this single exemption and the rationale.

### Coordinated bump + re-tag wave is documented as pattern, but no `tsort` tool exists

- Description: Phase 33's cascade had a topological-order miss that
  required a 9-slice expansion of Phase 34 to repair. The audit
  explicitly flags "future cascades must `tsort` against the dep DAG".
  The umbrella `README.md` and `scripts/eco.sh` describe the workflow
  but provide no tool to enforce or even emit the topological order
  before tagging. The current `llm-agent-otel` flow-pin drift (above)
  is exactly the kind of miss this tool would catch.
- Evidence:
  - `README.md` workflow guidance
  - `llm-agent/.planning/v1.1-MILESTONE-AUDIT.md:177-186` trade-off entry
  - `scripts/eco.sh` has no tsort helper
- Severity: medium
- Suggested next action: add a `scripts/eco.sh tag-order` (or
  `scripts/release-order.sh`) helper that emits the topological dep
  order from the live `go.mod` files. Make it the documented
  prerequisite for any coordinated bump wave.

### Stale remote branches across sister repos are not pruned from the umbrella view

- Description: `llm-agent/.planning/STATE.md` records stale remote
  branches in sister repos. `make status` runs `git status` per repo
  but does not surface stale remote refs.
- Evidence:
  - `scripts/eco.sh` status implementation
- Severity: low
- Suggested next action: add a `make prune` step that lists merged-but-
  not-deleted remote branches.

## Architectural concerns

### Two stability tiers + active v0.0.x deployers — the upgrade story is missing

- Description: With `llm-agent-flow` v0.1.0 freezing the library + flowd
  surface, the ecosystem now has **two stability tiers**: v1.x frozen
  (`rag`), v0.1.x frozen (`flow`), and SemVer 0.x BC for the rest. The
  freeze docs (`flow/docs/compatibility.md`) only describe the v0.1.x
  promise from v0.1.0 onward. There is **no documented upgrade path
  for v0.0.x ad-hoc deployers** — anyone running `cmd/flowd` from
  v0.0.5 (SQLite + CRUD), v0.0.6 (per-event persistence), v0.0.8
  (bearer auth), or v0.0.9 (replay) needs to know:
  - Whether the v0.0.x SQLite schema is forward-compatible with v0.1.x
    (the CHANGELOG says it is for v0.0.6's `run_events` table; later
    additions are not stated).
  - Whether v0.0.x HTTP clients (curl recipes, scripted callers) still
    work against v0.1.x (the answer is yes by `flow/docs/compatibility.md`
    — endpoints are stable in v0.1 if listed under Implemented — but
    this is not surfaced as an upgrade note).
  - Which v0.0.x tag is the recommended "if you can't go to v0.1.x yet"
    fallback.
- Evidence:
  - `llm-agent-flow/docs/compatibility.md:1-70` — describes v0.1
    forward, not v0.0 → v0.1 migration
  - `llm-agent-flow/CHANGELOG.md` — each v0.0.x release describes
    additions but no "upgrading from prior v0.0.x" notes
  - `README.md` umbrella roster references only the v0.1.1 tag
- Severity: medium
- Suggested next action: add a "Upgrading from v0.0.x" section to
  `llm-agent-flow/docs/compatibility.md` (or a sibling
  `docs/upgrading.md`) that explicitly:
  1. Confirms v0.0.6+ SQLite databases auto-migrate on `Open` (already
     true from `flow/store/sqlite/open.go`).
  2. Confirms HTTP endpoints are unchanged from v0.0.9 to v0.1.1.
  3. States that v0.0.x clients can move to v0.1.x patch-by-patch
     without code changes (other than possible `EngineCacheSize` /
     `AppendRunEvents` opt-in additions in v0.1.1).
  4. Picks one v0.0.x tag (likely v0.0.9) as the "stay where you are"
     escape hatch with a deprecation horizon.

### `cel-go` + `sqlite` + `modernc` transitive deps land in every downstream `go.sum`

- Description: `llm-agent-flow/go.mod` directly requires
  `github.com/google/cel-go v0.28.1` and `modernc.org/sqlite v1.50.1`.
  Even though cel-go is isolated to the `flow/cond/cel/` sub-package
  and SQLite to `flow/store/sqlite/`, **the Go module system pulls
  both into the go.sum of every downstream consumer** — including
  `llm-agent-customer-support` which already has its own `modernc.org/sqlite`
  pin for `internal/sessionstore`. Indirect transitives now include
  `cel.dev/expr`, `github.com/antlr4-go/antlr/v4` (cel-go's grammar),
  `golang.org/x/exp`, `google.golang.org/genproto/...api`,
  `google.golang.org/genproto/...rpc`, `modernc.org/libc`,
  `modernc.org/mathutil`, `modernc.org/memory`. A consumer that
  imports only `llm-agent-flow/flow` (no `cond/cel`, no `store/sqlite`)
  still inherits these in `go.sum` — they don't link into the binary
  thanks to dead-code elimination, but `go mod tidy` and supply-chain
  audits see them all.
- Evidence:
  - `llm-agent-flow/go.mod:5-9` direct requires
  - `llm-agent-flow/go.mod:11-28` indirect requires (the cel-go +
    sqlite transitive closure)
  - `llm-agent-customer-support/go.mod` shows
    `github.com/antlr4-go/antlr/v4`, `cel.dev/expr`,
    `google.golang.org/genproto/...` propagated up
- Severity: medium
- Suggested next action: document the trade-off explicitly. The
  options are:
  1. **Status quo (current).** Document in `flow/docs/compatibility.md`
     that downstream consumers will see cel-go + sqlite + modernc in
     `go.sum` even if they don't import the sub-packages. State that
     this is the Go-module-system trade-off for keeping `cond/cel/` and
     `store/sqlite/` in the same module as the library.
  2. **Split-module.** Move `flow/cond/cel/` and `flow/store/sqlite/`
     into separate Go modules (`github.com/costa92/llm-agent-flow-cel`,
     `github.com/costa92/llm-agent-flow-sqlite`). This is the
     `llm-agent-rag/postgres/`-style approach but is a v0.1 → v0.2 (or
     /v2) break.
  3. **Status quo for v0.1, plan split for v2.** Land option 1 now,
     bake the split into the v2 plan.
  Pick option 3 unless a downstream actively objects.

### Rule 1 (core stdlib-only) holds, but no umbrella check exists

- Description: `llm-agent/go.mod` is `module github.com/costa92/llm-agent
  / go 1.26.0 / require github.com/costa92/llm-agent-rag v1.0.1`. The
  README enforces this is the single allowed back-edge. Risk is that
  a future PR adding a `require` line will pass local `go vet`/`go test`
  and only get caught by the cross-repo CI. The umbrella does not own
  a check that asserts exactly one back-edge line.
- Evidence:
  - `llm-agent/go.mod:1-5`
  - umbrella `.github/workflows/umbrella.yml:62-68` validates root
    files only; no `llm-agent/go.mod`-shape assertion
- Severity: low
- Suggested next action: add a one-liner in
  `scripts/dep-currency-check.sh` (or a sibling
  `scripts/core-stdlib-check.sh`) that asserts
  `wc -l < llm-agent/go.sum` is bounded and that `llm-agent/go.mod`
  contains exactly one `require` line pointing at `llm-agent-rag`.

### Rule 6 (OTel as decorator, not hooks) and rule 5 (per-`(provider × model)` capabilities) enforced inside sister repos, not from the umbrella

- Description: K1/K2/K3 are core architectural keystones enforced
  inside `llm-agent` + `llm-agent-otel` by code review and contract
  tests. The umbrella states them but provides no cross-repo invariant
  check. The K3 decorator pattern has now extended to `otelflow.Wrap(flow.Runner)`
  with the same shape — good news, but still policed by review only.
- Evidence:
  - `README.md` rules 5-7
  - `llm-agent-otel/otelflow/otelflow.go:30-50` `Wrap(inner)`
- Severity: low
- Suggested next action: leave as-is. Document that enforcement is by
  review + the per-sister test suite.

## Operational concerns

### `make up` brings up only 2 of 6 subprojects and the launchable list is hard-coded twice

- Description: `scripts/eco.sh:14-17` defines `launchable_repos=(
  llm-agent-otel llm-agent-customer-support )` and `scripts/eco.sh:32-35`
  duplicates the same list in `is_launchable()`. Adding a new launchable
  subproject (e.g. `flowd` as a daemon) requires editing two places.
  `llm-agent-flow` now has `cmd/flowd` — a natural candidate for the
  launchable set once the demo wiring exists.
- Evidence:
  - `scripts/eco.sh:14-17, 32-35`
- Severity: low
- Suggested next action: collapse `is_launchable` to iterate over
  `launchable_repos[@]`. Optionally add `llm-agent-flow` to
  `launchable_repos` once a compose file or container image exists
  (none yet).

### Bearer-token auth has no rate-limit, no audit log, no token rotation

- Description: `llm-agent-flow/cmd/flowd/server/auth.go` is the first
  HTTP-layer security primitive in the ecosystem. It is correct on
  the basics (constant-time compare, 401 vs 403 sentinel mapping,
  `/healthz` bypass) but lacks three operational features that matter
  at scale:
  1. **No rate-limiting.** A brute-force loop against `/flows` or
     `/runs/{id}/replay` is not throttled at the handler level. A
     load balancer in front of flowd can do this, but the binary does
     not.
  2. **No audit log of auth events.** `/runs/{id}/events` is the
     audit log for runs; nothing similar exists for auth. Failed
     authentications return 401/403 and disappear — no record of who
     tried, when, or how many times. `log.Logger` writes the error
     but to stdout only.
  3. **No token rotation.** `BearerTokenAuthenticator.Token` is set
     at server construction and never changes. Rotating a leaked
     token requires a flowd restart.
  These omissions are acceptable for v0.0.8/v0.1.0 (single-tenant,
  single-deployer) but become operational risks if flowd is deployed
  behind a public network boundary.
- Evidence:
  - `llm-agent-flow/cmd/flowd/server/auth.go` (full file)
  - `llm-agent-flow/cmd/flowd/server/auth_test.go` — 8 cases, none
    exercise repeated-failure rate-limit or audit-log behavior
- Severity: medium (low if flowd stays a private/internal daemon;
  high if the demo stack ever exposes it on a public LB)
- Suggested next action: scope into the next flow milestone:
  1. Add a `Config.RateLimiter` seam (any `func(*http.Request) error`
     returning a configurable error → 429 with `Retry-After`).
  2. Add a `Config.AuthEventLogger func(r *http.Request, err error)`
     hook fired on every auth pass and fail, so callers can route to
     OTel logs / file / database. Default no-op.
  3. Document that token rotation is operator's responsibility and
     requires a graceful restart; or add `Server.SetAuthenticator(a)`
     (note: this would widen the v0.1.x surface — needs the freeze
     to allow it).

### Stream-mode events still per-event INSERT; sync uses batch — operators must understand the trade-off

- Description: v0.1.1 introduced single-transaction batched persistence
  for **sync** runs (`POST /flows/{id}/run`) — events collected during
  the engine loop are flushed in one transaction at the end of the
  run. **Stream runs (`POST /flows/{id}/run/stream`) still persist
  per-event** before forwarding to the SSE client, preserving the
  v0.0.6 "events outlive a dropped client" guarantee. This is the
  right call for durability — a stream consumer that disconnects
  mid-run still leaves a complete audit trail — but it means stream
  mode has higher write amplification under load.
- Evidence:
  - `llm-agent-flow/CHANGELOG.md` v0.1.1 entry: "Stream runs
    (`/run/stream`) unchanged — they still persist per-event before
    forwarding"
  - `llm-agent-flow/cmd/flowd/server/server.go:426-460` shows the
    per-event vs batch code path split
- Severity: medium (depends on operator's load profile)
- Suggested next action: add a deployment-notes section to
  `llm-agent-flow/README.md` (or `flow/docs/architecture.md`) that
  states:
  1. Per-event INSERT on stream runs is by-design for durability.
  2. Estimated overhead: one INSERT per event (~50-200µs on local
     SQLite, more on networked DBs).
  3. Operators with high-volume stream traffic may want to (a) use
     `:memory:` for ephemeral debug runs, or (b) front flowd with a
     write-batching proxy. (Documenting the trade-off does not change
     the v0.1 contract; it just sets expectations.)

### `make up` hard-codes ports; no env-override docs

- Description: `scripts/eco.sh:89-103` hard-codes ports for app /
  grafana / ollama / otel. There is no documented way to override
  these from `make up`.
- Evidence:
  - `scripts/eco.sh:89-104`
- Severity: low
- Suggested next action: read each port from the environment with a
  default (`${CS_APP_PORT:-8080}` etc.) and document the override
  variables in `README.md`.

### Provider credentials are not surfaced as part of the umbrella story

- Description: The subprojects ultimately need credentials for OpenAI,
  Anthropic, DeepSeek, MiniMax. The umbrella `README.md`, `Makefile`,
  and `scripts/eco.sh` do not document how provider keys flow into
  the launchable demo stack. No `.env.example` at the umbrella root.
- Evidence:
  - `README.md` "Working with the umbrella locally" — no credential guidance
  - `scripts/eco.sh:89-104` `run_compose` does not pass through provider env vars
- Severity: medium
- Suggested next action: add a "Provider credentials" subsection to
  `README.md` pointing at each sister repo's credential docs.

### Bootstrap clones from HTTPS, ignoring any per-user SSH rewrite

- Description: `scripts/eco.sh:21-25` uses
  `https://github.com/costa92/<repo>.git` literals. A new contributor
  without the per-user `url.git@github.com:.insteadOf https://github.com/`
  rewrite will fail to clone private sister repos.
- Evidence:
  - `scripts/eco.sh:19-28`
- Severity: low
- Suggested next action: document the SSH rewrite requirement or
  accept a `LLM_AGENT_REPO_PREFIX` env var.

## Documentation drift

### Umbrella README says flow is "v0.0.x" in the subprojects list but the roster table says v0.1.1

- Description: `README.md:26` lists `llm-agent-flow/` with the
  parenthetical "(v0.0.x)" in the directory tree, but `README.md:38`
  in the roster table cites `v0.1.1` and v0.1.x stable. A reader
  glancing at the tree will assume flow is still walking-skeleton
  exploration; the table-row contradiction lands further down.
- Evidence:
  - `README.md:26` "llm-agent-flow/ # serializable flow IR + DAG executor (v0.0.x)"
  - `README.md:38` "llm-agent-flow | ... | **v0.1.1** | main"
- Severity: low
- Suggested next action: update line 26 to "(v0.1.x stable)" to match
  the roster.

### `docs/current-project-analysis.md` predates v1.1 close and never mentions flow

- Description: The analysis doc still references pre-v1.1 tag positions
  for the older sister repos and contains zero mention of
  `llm-agent-flow`. It is the headline link under "Docs" in
  `README.md:14-15`.
- Evidence:
  - `docs/current-project-analysis.md` (entire file)
  - `README.md:13-15` links to it
- Severity: medium
- Suggested next action: add a "Snapshot as of YYYY-MM-DD / ecosystem
  tag set: …" header to the top of the analysis doc and refresh the
  flow section. Tie the refresh to v1.2 close.

### `llm-agent/CHANGELOG.md` carries an empty `## [Unreleased]` while in-flight v1.2 code exists

- Description: `llm-agent/CHANGELOG.md` shows `## [Unreleased]` with no
  body, but 8 commits past `v0.5.1` already implement the v1.2 budget
  package. Keep-a-Changelog convention is to accumulate
  `Added/Changed` entries under `Unreleased` as features land.
- Evidence:
  - `llm-agent/CHANGELOG.md:12-13`
  - `git -C llm-agent log --oneline -5` Phase 35 commits
- Severity: low
- Suggested next action: each v1.2 phase should add an `Unreleased`
  bullet at completion time. Could be promoted to a shared convention
  under "Project rules" in the umbrella `README.md`.

## Testing gaps

### No ecosystem-level smoke test verifying the launchable stack actually serves a request

- Description: `.github/workflows/umbrella.yml` runs `go vet`/`build`/
  `test` per repo, but never starts the demo stack via `make up` and
  never hits the HTTP API. With flowd now existing as a second daemon
  (alongside customer-support), there are two HTTP surfaces that go
  unsmoked in CI.
- Evidence:
  - `.github/workflows/umbrella.yml` (per-repo `go test`)
  - `llm-agent-flow/cmd/flowd/main.go` has no compose/smoke wiring
    from the umbrella
- Severity: medium
- Suggested next action: add a minimal `make smoke` target that
  starts customer-support with `DISABLE_LLM=1` and starts flowd with
  `--db :memory:`, polls `/healthz` on both, posts one trivial chat
  request + one trivial flow run, and tears down. Run it in umbrella
  CI as a separate job.

### No umbrella-owned test asserting `make bootstrap` + `make build` + `make test` succeed from a clean checkout

- Description: The CI workflow clones each sister explicitly via
  `actions/checkout@v4` instead of using `make bootstrap`. The
  umbrella's primary developer command is not exercised in CI.
- Evidence:
  - `.github/workflows/umbrella.yml:21-55` six explicit `checkout`
    steps; never invokes `scripts/eco.sh bootstrap`
- Severity: low
- Suggested next action: add a second job (`bootstrap-smoke`) that
  clones the umbrella, runs `make bootstrap && make workspace &&
  make build && make test`, and times out at 30 minutes.

### Live-Postgres CI wiring is documented as pending in core planning

- Description: `llm-agent/.planning/STATE.md` records "Live-Postgres
  CI wiring (testcontainers-go or GH Actions services)" as pending.
- Evidence:
  - `llm-agent/.planning/STATE.md`
- Severity: medium
- Suggested next action: scope into a future ecosystem-alignment
  milestone (rag-side; umbrella tracks).

## Subproject-specific concerns

### `llm-agent-flow` v0.1 freezes the library but not the JSON IR — schema drift could silently invalidate flow files

- Description: `flow/docs/compatibility.md` explicitly excludes
  "JSON IR additions" from the v0.1 freeze: new optional fields may
  appear; removal of an existing field would need a major bump. This
  is the right choice (the IR is the data contract; freezing it as
  hard as the Go API would block iteration) but it means **a flow
  file written against v0.0.6 may behave differently under v0.1.1**
  if a field's optional default semantics evolved. The flow's own
  CHANGELOG records concrete instances: `Edge.Condition` joined
  v0.0.4, `flow.tools` joined v0.0.3 — fields that were absent in
  earlier versions but defaulted-in by later ones.
- Evidence:
  - `llm-agent-flow/docs/compatibility.md` "What v0.1 does NOT cover"
  - `llm-agent-flow/CHANGELOG.md` v0.0.3, v0.0.4 entries
- Severity: medium
- Suggested next action: add a `flow/json-schema/v0.1.json` (or
  `internal/jsonschema/v0.1.txt`) that pins the v0.1 JSON IR shape
  the way `internal/apisnapshot/` pins the Go API. Reject unknown
  fields with a warning (not an error — backward compat) in
  `flow.Load`. Reject removed-field semantics with an error.

### `llm-agent/DEPRECATIONS.md` is empty — discipline must hold across v1.2

- Description: `llm-agent/DEPRECATIONS.md` lists "*(none)*" under
  active deprecations. With v1.2 adding `budget`, `policy`,
  `orchestrate.Supervisor` packages, a future deprecation could slip
  in without a matching row.
- Evidence:
  - `llm-agent/DEPRECATIONS.md`
- Severity: low
- Suggested next action: leave as-is; the file is the gate. The
  per-phase audit must update this file on any new deprecation.

### `llm-agent-customer-support` has the largest dep tree, now including the flow + cel + sqlite transitive closure

- Description: `llm-agent-customer-support/go.mod` directly requires
  `llm-agent`, `llm-agent-otel`, `llm-agent-providers`, `lib/pq`,
  `modernc.org/sqlite`, OTel SDK + trace; **and now indirectly
  requires `llm-agent-flow`, `llm-agent-rag`, plus the cel-go +
  antlr4 + modernc + genproto transitive chain via otelflow/flowrunner**.
  Supply-chain risk concentrates here.
- Evidence:
  - `llm-agent-customer-support/go.mod:1-60` (the full transitive tree)
- Severity: medium
- Suggested next action: schedule a periodic (quarterly) dep-audit on
  `llm-agent-customer-support` only — `go list -u -m all` + manual
  review for breaking changes. Track in the umbrella roadmap.

---

*Concerns audit: 2026-05-21*
