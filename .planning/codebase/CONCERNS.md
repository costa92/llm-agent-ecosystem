# Codebase Concerns

**Analysis Date:** 2026-05-20

Scope: the `llm-agent-ecosystem` umbrella root and what it sees of its 5
subprojects. Concerns are scoped to what the umbrella owns (navigation,
conventions, cross-repo coordination) and to risks the umbrella is positioned
to detect or mitigate. Subproject-internal concerns are flagged only where
they materially affect the umbrella's contract or where they create drift
visible from the root.

## Known incomplete work

### Umbrella README claims v1.1 in flight; subproject source-of-truth says shipped and closed

- Description: The umbrella `README.md` Status section asserts "v1.1 —
  Ecosystem alignment (this milestone) — **in flight**; Phases 31-33 complete
  (4/5 requirements done), Phase 34 pending (umbrella dependency-currency CI
  gate + audit + close)." The source-of-truth planning tree in
  `llm-agent/.planning/STATE.md` (`stopped_at`, line 7) and
  `llm-agent/.planning/v1.1-MILESTONE-AUDIT.md` (`Verdict: ✅ PASS`, line 11)
  both record v1.1 shipped and closed 2026-05-20 with audit PASS 5/5 and
  Phase 34 complete (9 slices). The umbrella root is the only document still
  claiming v1.1 is open.
- Evidence:
  - `README.md:125-128` (umbrella root)
  - `llm-agent/.planning/STATE.md:7,178-189`
  - `llm-agent/.planning/v1.1-MILESTONE-AUDIT.md:1-18`
- Severity: high
- Suggested next action: rewrite the umbrella `README.md` Status block to
  reflect v1.1 closed (audit PASS 5/5, final tag set
  `llm-agent v0.5.1 / llm-agent-rag v1.0.1 / llm-agent-otel v0.2.1 /
  llm-agent-providers v0.2.1 / llm-agent-customer-support v0.2.2`) and
  v1.2 Core Capability Deepening active. Add a `<!-- refreshed: YYYY-MM-DD -->`
  marker so future drift is auditable.

### v1.2 milestone activity exists in `llm-agent` but umbrella docs do not mention it

- Description: `llm-agent` HEAD (`e28c8a7`, 8 commits past `v0.5.1`) carries
  v1.2 Phase 35 work (commits `581caea feat(budget): add ctx-keyed budget
  package`, `d141bf6 feat(agents): wire budget enforcement`, `535375f
  test(agents): wide paradigm budget integration`, `e28c8a7 docs(agents):
  retire stale "deprecated" markers`). The umbrella root has zero mention of
  v1.2, v0.6.0, the `budget` package, or that an active milestone is running.
- Evidence:
  - `git -C llm-agent log --oneline -5` shows Phase 35 Wave 1-4 commits past
    `v0.5.1`
  - `llm-agent/.planning/STATE.md:25,29-51` documents v1.2 active
  - `README.md` umbrella-root: no v1.2 reference
- Severity: medium
- Suggested next action: add a "Currently active milestones" subsection to
  the umbrella `README.md` Status block referencing
  `llm-agent/.planning/STATE.md` as the authoritative pointer. The umbrella
  is the front door and must point at the live milestone.

### `llm-agent-customer-support` is 3 commits past its latest tag

- Description: `git describe --tags` returns `v0.2.2-3-g4325046`. New CI
  workflow expansion commits exist past the v1.1-final tag. No follow-up tag
  is recorded; CHANGELOG status is not verified here, but the divergence is
  invisible from the umbrella.
- Evidence:
  - `git -C llm-agent-customer-support describe --tags` → `v0.2.2-3-g4325046`
  - `git -C llm-agent-customer-support log --oneline -5`:
    `4325046 Expand CI workflow coverage`, `3de0dd1 Remove legacy
    supportflow facade paths`, `ba993b0 test(supportflow): drop
    "legacy-compatible" wording`
- Severity: low
- Suggested next action: confirm with the operator whether these post-tag
  commits are queued for a `v0.2.3` cascade or are pure infra and intended
  to stay un-tagged. If un-tagged on purpose, document it under "Tagging
  policy" in the umbrella README.

## Cross-repo coordination risks

### Dep-currency gate lives in `llm-agent`, not in the umbrella root

- Description: The strict ecosystem dep-currency check is implemented at
  `llm-agent/scripts/dep-currency-check.sh` and invoked from
  `llm-agent/.github/workflows/umbrella.yml`. The umbrella root's own
  workflow `.github/workflows/umbrella.yml` only does cross-repo build/test
  and validates root files — it does **not** invoke the dep-currency check.
  The gate's coverage is therefore conditional on `llm-agent` staying alive
  and reachable; if `llm-agent`'s remote becomes unavailable or its
  workflow is restructured, the umbrella loses the gate silently.
- Evidence:
  - `.github/workflows/umbrella.yml:62-103` (umbrella root — no
    `dep-currency-check.sh` reference; `grep -c dep-currency` returns 0)
  - `llm-agent/.github/workflows/umbrella.yml:77-80` is where it lives
  - `llm-agent/scripts/dep-currency-check.sh:1-43` is the script
- Severity: high
- Suggested next action: vendor a thin wrapper (e.g.
  `scripts/dep-currency-check.sh` in this repo, or a re-checkout step in
  the umbrella workflow) that runs the script from whichever sister repo
  hosts it, OR fold the existing umbrella workflow into the same job that
  runs the gate so the gate is invoked from the root too. This is the
  Phase 34 KE-6 keystone; the umbrella owns "cross-repo coordination" but
  delegates the gate to a sister repo today.

### Strict-equality dep-currency gate has one auditable exemption (`rag → core`)

- Description: `llm-agent-rag/go.mod` requires `github.com/costa92/llm-agent
  v0.5.0`, but the current latest core tag is `v0.5.1`. The dep-currency
  gate handles this with a hard-coded skip ("SKIP: rag back-edge to core
  (cycle exemption — KE-2 corollary)" at
  `llm-agent/scripts/dep-currency-check.sh:59-78`). It is documented and
  intentional, but it is also the only auditable strict-equality exemption
  in the system. A second exemption would erode the gate; a forgotten
  exemption would mask drift.
- Evidence:
  - `llm-agent-rag/go.mod:5` (`github.com/costa92/llm-agent v0.5.0` while
    latest is `v0.5.1`)
  - `llm-agent/scripts/dep-currency-check.sh:59-78` exemption block
  - `llm-agent/.planning/v1.1-MILESTONE-AUDIT.md:99-103` documents the
    exemption as one of three documented trade-offs
- Severity: medium
- Suggested next action: add an explicit `EXEMPTIONS` section in the
  umbrella `README.md` listing this single exemption and the rationale, so
  the existence (and singularity) of the exemption is reviewable from the
  front door. Reject any second exemption that does not first earn a
  README diff.

### Coordinated bump + re-tag wave is documented as pattern, but no `tsort` tool exists

- Description: Phase 33's cascade had a topological-order miss that
  required a 9-slice expansion of Phase 34 to repair. The audit explicitly
  flags "future cascades must `tsort` against the dep DAG". The umbrella
  `README.md` (lines 113-121) and `scripts/eco.sh` describe the workflow
  but do not provide a tool to enforce or even emit the topological order
  before tagging.
- Evidence:
  - `README.md:113-121` ("Suggested workflow for cross-repo changes …
    keep the repo independent and follow its own release flow")
  - `llm-agent/.planning/v1.1-MILESTONE-AUDIT.md:177-186` trade-off entry
  - `scripts/eco.sh:1-183` has no tsort, no bump-order helper
- Severity: medium
- Suggested next action: add a `scripts/eco.sh tag-order` (or
  `scripts/release-order.sh`) helper that emits the topological dep order
  from the live `go.mod` files. Make it the documented prerequisite for
  any coordinated bump wave.

### Stale remote branches across sister repos are not pruned from the umbrella view

- Description: `llm-agent/.planning/STATE.md:249-252` records stale remote
  branches in sister repos (`chore/bump-llm-agent-v0.4.0`,
  `docs/link-governance-guides`, merged `fix/*`, `verify/*`). Phase 32
  pruned local branches only. The umbrella `make status` runs `git status
  --short --branch` per repo but does not surface stale remote refs.
- Evidence:
  - `llm-agent/.planning/STATE.md:249-252`
  - `scripts/eco.sh:133-141` status implementation
- Severity: low
- Suggested next action: add a `make prune` (or `scripts/eco.sh prune`)
  step that lists merged-but-not-deleted remote branches; do not auto-
  delete (umbrella is coordination-only, not destructive).

## Architectural concerns

### Rule 3 (`go.work` is `.gitignore`d) is correctly enforced but the file `go.work` is checked in at the umbrella root

- Description: The "Project rules" block (`README.md:63-65`) states
  "`go.work` is `.gitignore`d in every repo. CI runs with `GOWORK=off`."
  The umbrella root **owns** a checked-in `go.work` (`go.work:1-9`)
  declaring all 5 submodules. The umbrella's own `.gitignore` does NOT
  ignore `go.work` at the root (it ignores `go.work.sum` only —
  `.gitignore:8`). This is consistent with rule 3 (the rule applies to
  each sister repo's `.gitignore`, and per-sister `.gitignore`s do cover
  it), but a reader landing on the umbrella will see a checked-in
  `go.work` immediately and may copy that pattern into a sister repo.
- Evidence:
  - `README.md:63-65` rule 3 text
  - `go.work:1-9` checked in at the umbrella root
  - `.gitignore:1-9` umbrella `.gitignore` (covers
    `/llm-agent/`, sister-repo dirs, and `go.work.sum`, NOT `go.work`)
  - `scripts/workspace.sh:18-22` writes the file unconditionally
- Severity: low
- Suggested next action: add a short note to `README.md` under "Project
  rules" explaining that rule 3 applies *inside each sister repo*, and
  that the umbrella-root `go.work` is the single authorized location for
  a checked-in workspace because the umbrella IS the workspace. Or rename
  the umbrella's file to a different convention if the asymmetry is
  confusing.

### Rule 1 (core stdlib-only) holds, but the back-edge in `llm-agent/go.mod` is the *only* `require` line — its visibility matters

- Description: `llm-agent/go.mod:1-5` is `module github.com/costa92/llm-agent
  / go 1.26.0 / require github.com/costa92/llm-agent-rag v1.0.1`. The
  README enforces this is the single allowed back-edge (rule 1). The risk
  is not that the rule is broken — it isn't — but that any future PR that
  adds a `require` line will pass `go vet`/`go test` and only get caught
  by the cross-repo CI. The umbrella does not own a check that asserts
  exactly one back-edge line.
- Evidence:
  - `llm-agent/go.mod:1-5`
  - `README.md:57-59` rule 1 text
  - umbrella `.github/workflows/umbrella.yml:62-68` validates root files
    only; no `llm-agent/go.mod`-shape assertion
- Severity: low
- Suggested next action: add a one-liner in `scripts/dep-currency-check.sh`
  (or a sibling `scripts/core-stdlib-check.sh`) that asserts
  `wc -l < llm-agent/go.sum` is exactly 1 and that `llm-agent/go.mod`
  contains exactly one `require` line pointing at `llm-agent-rag`. Fail
  loud if either invariant breaks.

### Rule 6 (OTel as decorator, not hooks) and rule 5 (per-`(provider × model)` capabilities) are enforced inside sister repos, not from the umbrella

- Description: Rules 5, 6, 7 are core architectural keystones (K1, K2, K3).
  They are enforced inside `llm-agent` and `llm-agent-otel` by code review
  and contract tests. The umbrella states them but provides no
  cross-repo invariant check. A future sister repo (or a fork) could
  technically violate K2/K3 without the umbrella CI noticing.
- Evidence:
  - `README.md:67-74` rules 5-7
  - `.github/workflows/umbrella.yml:62-103` umbrella CI does build/test
    only
- Severity: low
- Suggested next action: leave as-is for now (these are design rules that
  resist mechanical checking). Document that enforcement is by review +
  the per-sister test suite, not by umbrella CI, so future maintainers do
  not assume the umbrella catches it.

## Operational concerns

### `make up` brings up only 2 of 5 subprojects and the launchable list is hard-coded twice

- Description: `scripts/eco.sh:14-17` defines `launchable_repos=(
  llm-agent-otel llm-agent-customer-support )` and `scripts/eco.sh:30-35`
  duplicates the same list in `is_launchable()`. Adding a new launchable
  subproject requires editing two places.
- Evidence:
  - `scripts/eco.sh:14-17, 30-35`
- Severity: low
- Suggested next action: collapse `is_launchable` to iterate over
  `launchable_repos[@]` so the list is the single source of truth.

### `make up` hard-codes ports for the demo stack; no env-override docs

- Description: `scripts/eco.sh:89-103` hard-codes `CS_APP_PORT=8080`,
  `CS_GRAFANA_PORT=3000`, `CS_OLLAMA_PORT=11434`, `CS_OTEL_GRPC_PORT=4317`,
  `CS_OTEL_HTTP_PORT=4318`, plus a parallel set for `llm-agent-otel` on
  different ports. There is no documented way to override these from
  `make up`; a developer with conflicting local ports must edit the
  script.
- Evidence:
  - `scripts/eco.sh:89-104`
- Severity: low
- Suggested next action: read each port from the environment with a
  default (`${CS_APP_PORT:-8080}` etc.) and document the override
  variables in `README.md` under "Working with the umbrella locally".

### Provider credentials are not surfaced as part of the umbrella story

- Description: The 5 subprojects ultimately need credentials for OpenAI,
  Anthropic, DeepSeek, MiniMax, etc. (see `llm-agent-providers/go.mod`).
  The umbrella `README.md`, `Makefile`, and `scripts/eco.sh` do not
  document how provider keys flow into the launchable demo stack. There
  is no `.env.example` at the umbrella root. A new developer running
  `make up` will get a stack that comes up but cannot reach any provider
  without separately reading each sister repo's docs.
- Evidence:
  - `README.md:97-108` "Working with the umbrella locally" — no
    credential guidance
  - `scripts/eco.sh:89-104` `run_compose` does not pass through provider
    env vars
  - `grep -c API_KEY /home/.../README.md` → 0
- Severity: medium
- Suggested next action: add a "Provider credentials" subsection to
  `README.md` pointing at each sister repo's credential docs and noting
  that the umbrella does not own a central secrets file. If a shared
  `.env.example` is desired, add it at the root and `.gitignore` the
  resolved `.env`. Do NOT introduce a secrets store at the umbrella —
  rule asymmetry would invite leakage.

### Bootstrap clones from HTTPS, ignoring any per-user SSH rewrite

- Description: `scripts/eco.sh:21-25` uses `https://github.com/costa92/<repo>.git`
  literals. `llm-agent/.planning/STATE.md:264-267` records the operator
  has a global rewrite `git config --global url."git@github.com:".insteadOf
  "https://github.com/"` so `go mod` works over SSH; the umbrella script
  works because of that rewrite, but a new contributor without it will
  fail to clone private sister repos.
- Evidence:
  - `scripts/eco.sh:19-28`
  - `llm-agent/.planning/STATE.md:264-267`
- Severity: low
- Suggested next action: document the SSH rewrite requirement (or an SSH
  alternative) in the umbrella `README.md` "Working with the umbrella
  locally" section. Optionally accept a `LLM_AGENT_REPO_PREFIX` env var
  in `scripts/eco.sh` to swap `https://` for `git@github.com:`.

## Documentation drift

### `docs/current-project-analysis.md` predates v1.1 close

- Description: The analysis doc was authored 2026-05-20 19:20 (per `ls`).
  Its "Subproject Analysis" sections reference tag positions that match
  pre-v1.1 state (e.g. it describes `llm-agent-rag` "stable `v1.0`
  positioning in README" with no mention of `v1.0.1`; it has no Status
  section mapping to the current ecosystem tag set). It is a structural
  analysis, not a release-state doc, so the drift is partial — but the
  doc is the headline link under "Docs" in `README.md:14-15` and reads
  as authoritative.
- Evidence:
  - `docs/current-project-analysis.md` (entire file, especially the
    `llm-agent-rag` subsection lines 130-202)
  - `README.md:13-15` links to it
  - latest rag tag is `v1.0.1` (not mentioned), latest core tag is
    `v0.5.1` (not mentioned anywhere in the doc), and v1.2 work is
    invisible
- Severity: medium
- Suggested next action: add a "Snapshot as of YYYY-MM-DD / ecosystem tag
  set: …" header to the top of `docs/current-project-analysis.md` and
  `docs/current-project-analysis.zh-CN.md` so readers know what tag set
  the analysis describes. Tie the refresh to v1.2 close.

### `README.md:38` says "Tag layout as of v1.1 (ecosystem alignment milestone, 2026-05-21)" — the date conflicts with current date

- Description: Today is 2026-05-20 (operator-provided). The umbrella
  `README.md:38` cites the v1.1 ecosystem-alignment milestone as
  2026-05-21, and `README.md:125` cites v1.0 shipped 2026-05-21. The
  `llm-agent/.planning/STATE.md:7` and the v1.1 audit both record v1.1
  shipped 2026-05-20. The 2026-05-21 marker is the v1.0 (rag freeze)
  ship date that has been transcribed into the v1.1 line. Minor
  inconsistency; not load-bearing, but visible.
- Evidence:
  - `README.md:38` ("v1.1 … 2026-05-21")
  - `README.md:125` ("v1.0 … shipped 2026-05-21")
  - `llm-agent/.planning/STATE.md:7,178-189` (v1.1 closed 2026-05-20)
  - `llm-agent-rag/CHANGELOG.md:9` (`## [v1.0.0] - 2026-05-21`)
- Severity: low
- Suggested next action: separate the two dates explicitly in `README.md`
  Status block: "v1.0 (rag freeze) shipped 2026-05-21; v1.1 (ecosystem
  alignment) shipped and closed 2026-05-20." The current text reads as
  if v1.1 is scheduled for tomorrow.

### `llm-agent-rag` CHANGELOG has no `v1.0.1` entry though the tag exists

- Description: `git -C llm-agent-rag describe --tags` returns `v1.0.1`.
  The CHANGELOG (`grep -E "^## "`) lists `v1.0.0` and `v0.6.0` but no
  `v1.0.1` row. The v1.1 audit
  (`llm-agent/.planning/v1.1-MILESTONE-AUDIT.md:99-103`) classifies
  `v1.0.0 → v1.0.1` as a "freeze-day-after, chore-only patch" — that
  is a release event that under Keep-a-Changelog convention should
  still appear, even if empty / "No exported-API change".
- Evidence:
  - `git -C llm-agent-rag describe --tags` → `v1.0.1`
  - `grep -E "^## " llm-agent-rag/CHANGELOG.md` shows no v1.0.1 row
- Severity: medium
- Suggested next action: open a tiny PR in `llm-agent-rag` adding a
  `## [v1.0.1] - 2026-05-20` block with the v1.1-audit phrasing
  ("freeze-day-after chore-only patch, no exported-API change"). The
  umbrella cannot fix sister-repo CHANGELOGs but should flag this
  during the next cascade audit.

### `llm-agent/CHANGELOG.md` carries an empty `## [Unreleased]` while in-flight v1.2 code exists

- Description: `llm-agent/CHANGELOG.md:12` shows `## [Unreleased]` with no
  body, but 4 commits past `v0.5.1` already implement the v1.2 budget
  package (`581caea`, `d141bf6`, `535375f`, `e28c8a7`). Keep-a-Changelog
  convention is to accumulate `Added/Changed` entries under `Unreleased`
  as features land, then rename to the tagged version at release time.
- Evidence:
  - `llm-agent/CHANGELOG.md:12-13`
  - `git -C llm-agent log --oneline -5` Phase 35 commits
  - `llm-agent/.planning/STATE.md:25` ("Core module bump: `v0.5.1 →
    v0.6.0`")
- Severity: low
- Suggested next action: discipline matter — each v1.2 phase should add
  an `Unreleased` bullet at completion time. The umbrella `README.md`
  could state this as a shared convention under "Project rules" so all
  sister repos follow it.

## Testing gaps

### No ecosystem-level smoke test verifying the launchable stack actually serves a request

- Description: `.github/workflows/umbrella.yml:70-103` runs `go
  vet`/`build`/`test` per repo, but never starts the demo stack via
  `make up` and never hits the HTTP API. The v1.1 audit's "test -short"
  results say "9 packages OK; 1 `?` (`internal/providers`)" for
  customer-support — meaning the provider wiring is not exercised even
  at the sister-repo level.
- Evidence:
  - `.github/workflows/umbrella.yml:70-103` (per-repo `go test`)
  - `llm-agent/.planning/v1.1-MILESTONE-AUDIT.md:55`
    (`internal/providers` `?` in the customer-support test table)
- Severity: medium
- Suggested next action: add a minimal `make smoke` target that runs
  `make up TARGETS=llm-agent-customer-support`, polls `/healthz`, posts
  one trivial chat request with `DISABLE_LLM=1` (the panic-switch path,
  per Keystone K7), and tears down. Run it in umbrella CI as a separate
  job; do not block the default cross-repo build on it.

### No umbrella-owned test asserting `make bootstrap` + `make build` + `make test` succeed from a clean checkout

- Description: The CI workflow (`.github/workflows/umbrella.yml`) clones
  each sister explicitly via `actions/checkout@v4` instead of using
  `make bootstrap`. That means the umbrella's primary developer
  command (`make bootstrap`) is not exercised in CI; a regression in
  `scripts/eco.sh` would only be caught by a human running it locally.
- Evidence:
  - `.github/workflows/umbrella.yml:21-55` six explicit `checkout`
    steps; never invokes `scripts/eco.sh bootstrap`
- Severity: low
- Suggested next action: add a second job (`bootstrap-smoke`) that
  clones the umbrella, runs `make bootstrap && make workspace &&
  make build && make test`, and times out at 30 minutes. Keep the
  existing fan-out job for fast per-repo isolation.

### Live-Postgres CI wiring is documented as pending in core planning

- Description: `llm-agent/.planning/STATE.md:208-212` records "Live-
  Postgres CI wiring (testcontainers-go or GH Actions services) —
  carried forward from v0.5; the Phase 14 `tsvector` path, the Phase 21
  `postgres` graph path, and the v0.8 `postgres`
  `_communities`/`_community_reports` paths are all unverified against
  a live DB." This is a `llm-agent-rag` concern but it surfaces here
  because the umbrella's customer-support service ultimately depends on
  pgvector behavior.
- Evidence:
  - `llm-agent/.planning/STATE.md:208-212`
- Severity: medium
- Suggested next action: scope into a future ecosystem-alignment
  milestone (rag-side; umbrella tracks). Not a v1.2 concern (KS-5 — rag
  is a fixed point for v1.2).

## Subproject-specific concerns

### `llm-agent/DEPRECATIONS.md` is currently empty under "Active deprecations" — verify the discipline is real

- Description: `llm-agent/DEPRECATIONS.md:12-14` lists "*(none)*" under
  active deprecations. The file's stated discipline is that every
  `// Deprecated:` godoc comment in the repo MUST appear here. The file
  itself states the v0.4 cut removed all prior entries
  (`llm-agent/DEPRECATIONS.md:18-29`). The discipline is sound; the
  risk is that with v1.2 adding new packages (`budget`, `policy`,
  `orchestrate.Supervisor`), a future deprecation could slip in
  without a matching DEPRECATIONS.md row.
- Evidence:
  - `llm-agent/DEPRECATIONS.md:12-14, 18-29`
  - `llm-agent/.planning/STATE.md:25,29-51` v1.2 new packages
- Severity: low
- Suggested next action: leave as-is; the file is the gate. If v1.2
  introduces any deprecation, the per-phase audit must update this
  file (the file states this rule itself at
  `llm-agent/DEPRECATIONS.md:32-43`).

### `llm-agent-otel` pins `llm-agent v0.5.1` AND `llm-agent-rag v1.0.1` directly — strong but expensive coupling

- Description: `llm-agent-otel/go.mod:5-6` carries strict pins to two
  upstream tags. Any patch bump to either upstream forces a cascade tag
  on `llm-agent-otel`, which then cascades to `llm-agent-customer-
  support`. The v1.1 cascade expanded Phase 34 from 3 slices to 9
  because of this exact propagation. Architecturally correct (rule 4-
  adjacent, dep-currency gate enforces it), but operationally costly
  for any future patch.
- Evidence:
  - `llm-agent-otel/go.mod:5-6`
  - `llm-agent-customer-support/go.mod:5-7`
  - `llm-agent/.planning/v1.1-MILESTONE-AUDIT.md:177-186` cascade
    expansion documented
- Severity: medium
- Suggested next action: when scoping the next ecosystem-alignment
  milestone (post-v1.2), evaluate whether `llm-agent-otel`'s pin on
  `llm-agent-rag` is actually required for the wrapper code or whether
  it can be relaxed to a `~v1.0.x` style soft pin via Go's MVS
  semantics. Tighten or loosen deliberately rather than by accident.

### `llm-agent-customer-support` has the largest dep tree — provider keys, OTel exporters, sqlite, pgvector — and is the smoke-test target

- Description: `llm-agent-customer-support/go.mod` has direct deps on
  `llm-agent`, `llm-agent-otel`, `llm-agent-providers`, `lib/pq`,
  `modernc.org/sqlite`, and OTel SDK + trace. Indirect deps include
  every provider SDK (`anthropic`, `openai`, `ollama`). Each provider
  SDK ages independently; supply-chain risk concentrates here.
- Evidence:
  - `llm-agent-customer-support/go.mod:1-60`
- Severity: medium
- Suggested next action: schedule a periodic (quarterly) dep-audit on
  `llm-agent-customer-support` only — `go list -u -m all` + manual
  review for breaking changes. Track in the umbrella roadmap, not the
  customer-support repo's planning, because the umbrella owns
  cross-repo coordination.

---

*Concerns audit: 2026-05-20*
