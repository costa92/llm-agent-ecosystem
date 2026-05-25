# M0+M1: Scaffold llm-agent-memory and Land Phase A Correctness Fixes

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create a new `llm-agent-memory/` sibling Go module and land the three Phase A correctness fixes (scoped lifecycle, consolidate dedupe, SearchUnified) as additive wrappers — without modifying `llm-agent/memory/` at all.

**Architecture:** New module `github.com/costa92/llm-agent-memory` imports core memory types from `github.com/costa92/llm-agent/memory` and adds three new components:
- `ScopedLifecycleManager` — wraps `*memory.ScopedManager`; adds `ConsolidateScoped`, `ForgetScoped`, `StatsScoped` that honor non-zero scope by enumerating the underlying `Lister` capability.
- `Consolidator` — wraps `*memory.Manager`; promotes Working→Episodic exactly like core's `Consolidate` but writes dedupe metadata keys (`_consolidated_at`, `_promoted_from`, `_promotion_count`) on source items and skips already-promoted items.
- `UnifiedSearcher` — wraps `*memory.Manager`; exposes `SearchUnified(ctx, query, topK)` that fans out to all three tiers, merges, dedupes by `(ID, Content)`, sorts by score desc, and respects `topK`.

All test-driven, stdlib-only.

**Tech Stack:** Go 1.26.0; stdlib only (no external deps); `github.com/costa92/llm-agent v0.5.0` (matches `llm-agent-rag/go.mod`); standard `testing` package; `github.com/costa92/llm-agent/llm` for `ScriptedLLM` embedder in tests.

---

## File Structure

| Path | Action | Responsibility |
|---|---|---|
| `llm-agent-memory/go.mod` | Create | Declare module `github.com/costa92/llm-agent-memory`, Go 1.26.0; require `github.com/costa92/llm-agent v0.5.0` |
| `llm-agent-memory/doc.go` | Create | Root-package doc anchor (mirrors `llm-agent-rag/doc.go` style); package name `memorykit` exports nothing at root |
| `llm-agent-memory/README.md` | Create | One-page sibling README; status, scope, three components |
| `llm-agent-memory/CHANGELOG.md` | Create | Keep-a-Changelog seed with `## [0.1.0] - 2026-05-26` Unreleased entry |
| `llm-agent-memory/memory/doc.go` | Create | Package `memory` (subpackage) doc; states that this package extends `github.com/costa92/llm-agent/memory` |
| `llm-agent-memory/memory/version.go` | Create | Exports `const Version = "0.1.0"` |
| `llm-agent-memory/memory/version_test.go` | Create | Smoke test: `Version` is non-empty and parses three numeric dotted parts |
| `llm-agent-memory/memory/scoped_lifecycle.go` | Create | A-1 impl: `ScopedLifecycleManager` with `ConsolidateScoped`, `ForgetScoped`, `StatsScoped` |
| `llm-agent-memory/memory/scoped_lifecycle_test.go` | Create | A-1 tests: cross-scope isolation for each method |
| `llm-agent-memory/memory/consolidator.go` | Create | A-2 impl: `Consolidator` type with `Consolidate` that writes/reads dedupe metadata |
| `llm-agent-memory/memory/consolidator_test.go` | Create | A-2 tests: first promotion writes metadata; second call is a no-op; round-trip through Export/Import preserves metadata |
| `llm-agent-memory/memory/unified_search.go` | Create | A-3 impl: `UnifiedSearcher` with `SearchUnified(ctx, query, topK)` |
| `llm-agent-memory/memory/unified_search_test.go` | Create | A-3 tests: fan-out, merge, dedupe by `(ID, Content)`, sort, topK |
| `llm-agent-memory/memory/testutil_test.go` | Create | Shared test fixtures (embedder, manager builders) |
| `go.work` | Modify | Add `./llm-agent-memory` to `use ( … )` block |
| `Makefile` | Modify | No-op (Makefile dispatches via `scripts/eco.sh`, so only `eco.sh` needs change) — leave as-is |
| `scripts/eco.sh` | Modify | Add `llm-agent-memory` to `all_repos` and `repo_url` |
| `.github/workflows/umbrella.yml` | Modify | Add checkout + build steps for `llm-agent-memory` mirroring siblings |

---

## Open Decisions Resolved in This Plan

- **Subpackage name:** New module's importable subpackage is named `memory` (so callers write `import "github.com/costa92/llm-agent-memory/memory"` and reference `memory.ScopedLifecycleManager`). To avoid a name collision with the imported `github.com/costa92/llm-agent/memory`, every Go file in the new package imports core as `coremem "github.com/costa92/llm-agent/memory"`.
- **Root package name:** `memorykit` (mirrors `ragkit` in `llm-agent-rag/doc.go`). Root exports nothing; only documentation anchor.
- **Dedupe metadata keys:** Public, exported `const` in `consolidator.go`. The core package's `metaKey*` are private; we cannot collide with them by construction (different package). Names:
  - `MetaKeyConsolidatedAt = "_consolidated_at"`
  - `MetaKeyPromotedFrom = "_promoted_from"`
  - `MetaKeyPromotionCount = "_promotion_count"`
- **Initial dedupe policy:** "Promote at most once." (Cold-window re-promote is deferred to a future task; the roadmap calls it optional.)
- **GitHub remote:** Plan assumes the repo `costa92/llm-agent-memory` exists on GitHub (per umbrella checkout pattern). If it doesn't yet exist on `main`, Task M0-7 (umbrella CI update) will fail at checkout; the executor MUST create the empty repo on GitHub before running Task M0-7. This is the single executor-side action that cannot be done in-tree.
- **Working module version:** Match `llm-agent-rag/go.mod` exactly — require `github.com/costa92/llm-agent v0.5.0`. No `replace` directive (umbrella's INFRA-04 gate forbids `replace` on release branches).
- **scoredStore enumeration from outside the package:** Use the exported `memory.Lister` interface (`List(ctx, filter, pageSize, cursor)` on `WorkingMemory`/`EpisodicMemory`/`SemanticMemory`) — NOT `storeOf` (which is package-private). All three bundled types implement `Lister`.

---

## Sequencing Rules

- **M0 tasks (1–7) need no TDD** — they are configuration. Each ends with a `go build` or `go test` smoke verification + a `git commit`.
- **M1 tasks (8–22) are strict TDD** — every step writes a failing test first, then the minimal impl that turns it green.
- **Commit cadence:** every task ends in exactly one commit. Step 5 is always "Commit".

---

# M0 — Scaffolding

## Task 1: Create `llm-agent-memory/go.mod`

**Files:**
- Create: `llm-agent-memory/go.mod`

- [ ] **Step 1: Create the go.mod**

  Write `llm-agent-memory/go.mod` with this exact content:

  ```
  module github.com/costa92/llm-agent-memory

  go 1.26.0

  require github.com/costa92/llm-agent v0.5.0
  ```

- [ ] **Step 2: Run `go mod download` to populate the module graph**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go mod download`
  Expected: no output, exit code 0. Creates a `go.sum`.

- [ ] **Step 3: Verify `go vet ./...` runs (no packages yet, should be a no-op)**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go vet ./...`
  Expected: no output (no Go files yet → no packages → no vet output), exit code 0.

- [ ] **Step 4: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem
  git add llm-agent-memory/go.mod llm-agent-memory/go.sum
  git commit -m "chore(memory): scaffold llm-agent-memory go.mod (Go 1.26.0, require llm-agent v0.5.0)"
  ```

---

## Task 2: Create root `doc.go`, `README.md`, `CHANGELOG.md`

**Files:**
- Create: `llm-agent-memory/doc.go`
- Create: `llm-agent-memory/README.md`
- Create: `llm-agent-memory/CHANGELOG.md`

- [ ] **Step 1: Write `llm-agent-memory/doc.go`**

  ```go
  // Package memorykit is the short brand name for the standalone
  // memory extension SDK whose module path is
  // github.com/costa92/llm-agent-memory.
  //
  // The root package is a documentation anchor only: it exports no
  // symbols. Callers import the subpackage:
  //
  //     import "github.com/costa92/llm-agent-memory/memory"
  //
  // The subpackage adds three additive capabilities on top of
  // github.com/costa92/llm-agent/memory without modifying core:
  //
  //   - ScopedLifecycleManager — scope-honoring ConsolidateScoped /
  //     ForgetScoped / StatsScoped (closes the v0.7 gap noted on
  //     llm-agent/memory/scoped_manager.go:12-17).
  //   - Consolidator — Working→Episodic promotion with dedupe metadata
  //     so the same working item is not promoted twice.
  //   - UnifiedSearcher — SearchUnified(ctx, query, topK) returning a
  //     single merged + deduped + sorted []SearchResult.
  //
  // The memorykit name diverges from the llm-agent-memory module path
  // on purpose, to give the SDK a concise import-free identity.
  package memorykit
  ```

- [ ] **Step 2: Write `llm-agent-memory/README.md`**

  ```markdown
  # llm-agent-memory

  Sibling Go module under the `llm-agent-ecosystem` umbrella. Extends
  `github.com/costa92/llm-agent/memory` with three additive
  capabilities — no modification to core.

  Status: 0.1.0 (M0 + M1 of the master memory roadmap).

  ## Import

  ```go
  import "github.com/costa92/llm-agent-memory/memory"
  ```

  ## What this module adds

  - `ScopedLifecycleManager` — scope-honoring Consolidate/Forget/Stats.
  - `Consolidator` — dedupe-aware Working→Episodic promotion.
  - `UnifiedSearcher` — `SearchUnified(ctx, query, topK)` cross-tier merge.

  ## Boundary

  This module **wraps** core. It does not fork or modify any file under
  `github.com/costa92/llm-agent/memory`. The core SDK remains
  stdlib-only and authoritative.

  See `docs/superpowers/plans/2026-05-25-llm-agent-memory-roadmap.md`
  in the umbrella for the full subproject roadmap.
  ```

- [ ] **Step 3: Write `llm-agent-memory/CHANGELOG.md`**

  ```markdown
  # Changelog

  All notable changes to `github.com/costa92/llm-agent-memory` will be
  documented in this file.

  <!-- Keep a Changelog format: https://keepachangelog.com/en/1.1.0/ -->
  <!-- Semver: https://semver.org/ -->

  ## [0.1.0] - 2026-05-26

  ### Added

  - Initial subproject scaffolding (M0 of master roadmap).
  - `memory.Version` constant.
  - `memory.ScopedLifecycleManager` with `ConsolidateScoped`,
    `ForgetScoped`, `StatsScoped` (Phase A item A-1).
  - `memory.Consolidator` with promote-once dedupe metadata
    (`_consolidated_at`, `_promoted_from`, `_promotion_count`)
    (Phase A item A-2).
  - `memory.UnifiedSearcher.SearchUnified(ctx, query, topK)`
    (Phase A item A-3).
  ```

- [ ] **Step 4: Verify `go vet` and `go build` still pass**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go vet ./... && GOWORK=off go build ./...`
  Expected: no output, exit code 0.

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem
  git add llm-agent-memory/doc.go llm-agent-memory/README.md llm-agent-memory/CHANGELOG.md
  git commit -m "docs(memory): add llm-agent-memory root doc.go, README, CHANGELOG"
  ```

---

## Task 3: Create stub `memory/` subpackage with `doc.go` + `version.go` + `version_test.go`

**Files:**
- Create: `llm-agent-memory/memory/doc.go`
- Create: `llm-agent-memory/memory/version.go`
- Create: `llm-agent-memory/memory/version_test.go`

- [ ] **Step 1: Write the failing test `llm-agent-memory/memory/version_test.go`**

  ```go
  package memory

  import (
  	"strconv"
  	"strings"
  	"testing"
  )

  func TestVersion_NonEmptyAndSemver(t *testing.T) {
  	if Version == "" {
  		t.Fatal("Version is empty")
  	}
  	parts := strings.Split(Version, ".")
  	if len(parts) != 3 {
  		t.Fatalf("Version = %q, want three dot-separated parts", Version)
  	}
  	for i, p := range parts {
  		if _, err := strconv.Atoi(p); err != nil {
  			t.Errorf("Version part %d (%q) not numeric: %v", i, p, err)
  		}
  	}
  }
  ```

- [ ] **Step 2: Run the test to verify it fails**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestVersion_NonEmptyAndSemver -v`
  Expected: compile error — `undefined: Version` — because `version.go` does not exist yet.

- [ ] **Step 3: Write the minimal implementation `llm-agent-memory/memory/version.go`**

  ```go
  package memory

  // Version is the current llm-agent-memory release tag (semver).
  // Bumped at every tagged release; see CHANGELOG.md in the module root.
  const Version = "0.1.0"
  ```

  And `llm-agent-memory/memory/doc.go`:

  ```go
  // Package memory extends github.com/costa92/llm-agent/memory with
  // three additive capabilities introduced in milestones M0 + M1 of the
  // llm-agent-memory roadmap:
  //
  //   - ScopedLifecycleManager — adds ConsolidateScoped, ForgetScoped,
  //     and StatsScoped methods that honor non-zero ctx scope.
  //     Closes the v0.7 limitation on
  //     github.com/costa92/llm-agent/memory.ScopedManager.
  //
  //   - Consolidator — Working→Episodic promotion with dedupe metadata
  //     so the same working item is not promoted twice. Writes the
  //     reserved metadata keys MetaKeyConsolidatedAt, MetaKeyPromotedFrom,
  //     and MetaKeyPromotionCount on source items.
  //
  //   - UnifiedSearcher — SearchUnified(ctx, query, topK) fans out to
  //     working/episodic/semantic, merges, dedupes by (ID, Content),
  //     sorts by score descending, and returns a single []SearchResult.
  //
  // All three components wrap (never modify) core memory types. Every
  // Go file in this package aliases the core import as `coremem` to
  // avoid a name collision with this package's own name.
  package memory
  ```

- [ ] **Step 4: Run the test to verify it passes**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestVersion_NonEmptyAndSemver -v`
  Expected: `PASS` and `ok github.com/costa92/llm-agent-memory/memory ...`.

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem
  git add llm-agent-memory/memory/doc.go llm-agent-memory/memory/version.go llm-agent-memory/memory/version_test.go
  git commit -m "feat(memory): add memory subpackage with Version constant and smoke test"
  ```

---

## Task 4: Add `./llm-agent-memory` to `go.work`

**Files:**
- Modify: `go.work`

- [ ] **Step 1: Edit `go.work` to add `./llm-agent-memory`**

  Current content (verified by reading file):

  ```
  go 1.26.0

  use (
  	./llm-agent
  	./llm-agent-customer-support
  	./llm-agent-flow
  	./llm-agent-otel
  	./llm-agent-providers
  	./llm-agent-rag
  )
  ```

  Replace the `use ( ... )` block with:

  ```
  use (
  	./llm-agent
  	./llm-agent-customer-support
  	./llm-agent-flow
  	./llm-agent-memory
  	./llm-agent-otel
  	./llm-agent-providers
  	./llm-agent-rag
  )
  ```

- [ ] **Step 2: Run `go work sync` to verify the workspace resolves**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem && go work sync`
  Expected: no output, exit code 0. May update `go.work.sum`.

- [ ] **Step 3: Verify the new module builds inside the workspace**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem && go build ./llm-agent-memory/...`
  Expected: no output, exit code 0.

- [ ] **Step 4: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem
  git add go.work go.work.sum
  git commit -m "chore(workspace): join llm-agent-memory into go.work"
  ```

---

## Task 5: Register `llm-agent-memory` in `scripts/eco.sh`

**Files:**
- Modify: `scripts/eco.sh:6-13` (`all_repos` array)
- Modify: `scripts/eco.sh:21-29` (`repo_url` case)

- [ ] **Step 1: Edit the `all_repos` array (lines 6–13)**

  Replace:

  ```bash
  all_repos=(
    llm-agent
    llm-agent-rag
    llm-agent-otel
    llm-agent-providers
    llm-agent-customer-support
    llm-agent-flow
  )
  ```

  With:

  ```bash
  all_repos=(
    llm-agent
    llm-agent-rag
    llm-agent-otel
    llm-agent-providers
    llm-agent-customer-support
    llm-agent-flow
    llm-agent-memory
  )
  ```

- [ ] **Step 2: Edit the `repo_url()` case statement (lines 21–29) — add a new case BEFORE the wildcard `*`**

  Replace:

  ```bash
  repo_url() {
    case "$1" in
      llm-agent) printf '%s\n' 'https://github.com/costa92/llm-agent.git' ;;
      llm-agent-rag) printf '%s\n' 'https://github.com/costa92/llm-agent-rag.git' ;;
      llm-agent-otel) printf '%s\n' 'https://github.com/costa92/llm-agent-otel.git' ;;
      llm-agent-providers) printf '%s\n' 'https://github.com/costa92/llm-agent-providers.git' ;;
      llm-agent-customer-support) printf '%s\n' 'https://github.com/costa92/llm-agent-customer-support.git' ;;
      llm-agent-flow) printf '%s\n' 'https://github.com/costa92/llm-agent-flow.git' ;;
      *) printf '%s\n' "" ;;
    esac
  }
  ```

  With:

  ```bash
  repo_url() {
    case "$1" in
      llm-agent) printf '%s\n' 'https://github.com/costa92/llm-agent.git' ;;
      llm-agent-rag) printf '%s\n' 'https://github.com/costa92/llm-agent-rag.git' ;;
      llm-agent-otel) printf '%s\n' 'https://github.com/costa92/llm-agent-otel.git' ;;
      llm-agent-providers) printf '%s\n' 'https://github.com/costa92/llm-agent-providers.git' ;;
      llm-agent-customer-support) printf '%s\n' 'https://github.com/costa92/llm-agent-customer-support.git' ;;
      llm-agent-flow) printf '%s\n' 'https://github.com/costa92/llm-agent-flow.git' ;;
      llm-agent-memory) printf '%s\n' 'https://github.com/costa92/llm-agent-memory.git' ;;
      *) printf '%s\n' "" ;;
    esac
  }
  ```

- [ ] **Step 3: Verify `scripts/eco.sh build` enumerates the new sibling**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem && bash scripts/eco.sh build llm-agent-memory`
  Expected: a `(cd .../llm-agent-memory && GOWORK=off go build ./...)` invocation, exit code 0, no output.

- [ ] **Step 4: Verify `scripts/eco.sh test` runs the version test through the sibling**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem && bash scripts/eco.sh test llm-agent-memory`
  Expected: `ok github.com/costa92/llm-agent-memory/memory ...`, exit code 0.

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem
  git add scripts/eco.sh
  git commit -m "chore(eco): register llm-agent-memory in scripts/eco.sh (all_repos + repo_url)"
  ```

---

## Task 6: Add `llm-agent-memory` to the umbrella CI workflow

**Files:**
- Modify: `.github/workflows/umbrella.yml`

Prerequisite reminder: this task assumes the repo `costa92/llm-agent-memory` exists on GitHub on branch `main`. If not, create an empty repo on GitHub with default branch `main` BEFORE pushing this commit; otherwise the umbrella `cross-repo-build` job will fail at the `actions/checkout@v4` step for the new sibling.

- [ ] **Step 1: Add the checkout block for the new sibling**

  Insert immediately AFTER the existing "Checkout llm-agent-customer-support" block (currently lines 50–55) and BEFORE the `- uses: actions/setup-go@v5` block. New content to insert:

  ```yaml
      - name: Checkout llm-agent-memory
        uses: actions/checkout@v4
        with:
          repository: costa92/llm-agent-memory
          ref: main
          path: llm-agent-memory
  ```

- [ ] **Step 2: Add the build step for the new sibling**

  Insert immediately AFTER the existing "Build llm-agent-customer-support" block (currently lines 98–103) and BEFORE the "B3 — depcheck cascade tool" block. New content to insert:

  ```yaml
      - name: Build llm-agent-memory
        working-directory: llm-agent-memory
        run: |
          GOWORK=off go vet ./...
          GOWORK=off go build ./...
          GOWORK=off go test ./... -count=1
  ```

- [ ] **Step 3: Lint the YAML locally**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem && python3 -c "import yaml; yaml.safe_load(open('.github/workflows/umbrella.yml'))"`
  Expected: no output, exit code 0.

- [ ] **Step 4: Verify the local in-tree build sequence still works** (proxy for CI)

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go vet ./... && GOWORK=off go build ./... && GOWORK=off go test ./... -count=1`
  Expected: `ok github.com/costa92/llm-agent-memory/memory ...`, exit code 0.

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem
  git add .github/workflows/umbrella.yml
  git commit -m "ci(umbrella): add llm-agent-memory to cross-repo-build matrix"
  ```

---

## Task 7: Final M0 smoke — full umbrella build still green

**Files:** none (verification only)

- [ ] **Step 1: Run the full umbrella build via eco.sh**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem && bash scripts/eco.sh build`
  Expected: every sibling builds, exit code 0. The new sibling appears in the loop output.

- [ ] **Step 2: Run the full umbrella test via eco.sh**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem && bash scripts/eco.sh test`
  Expected: all siblings pass; `ok github.com/costa92/llm-agent-memory/memory ...` appears in the output.

- [ ] **Step 3: Run the stdlib-only gate to confirm the new module did not regress core**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem && bash scripts/stdlib-only-check.sh`
  Expected: `stdlib-only-check: PASS`, exit code 0. (The script only targets `llm-agent/`, never reads our new module.)

- [ ] **Step 4: No commit — verification gate only.** If any step fails, fix the regression in a fresh follow-up commit before proceeding to M1.

---

# M1 — Phase A correctness fixes

## Task 8: Add shared test fixtures `testutil_test.go`

**Files:**
- Create: `llm-agent-memory/memory/testutil_test.go`

- [ ] **Step 1: Write the test helpers**

  ```go
  package memory

  import (
  	"testing"
  	"time"

  	"github.com/costa92/llm-agent/llm"
  	coremem "github.com/costa92/llm-agent/memory"
  )

  // newCoreEmbedder returns a deterministic ScriptedLLM embedder with
  // 64-dim vectors — matches the pattern in
  // github.com/costa92/llm-agent/memory/memory_test.go newWorking.
  func newCoreEmbedder() coremem.Embedder {
  	return llm.NewScriptedLLM(llm.WithEmbedDimensions(64))
  }

  // newCoreWorking builds a *coremem.WorkingMemory with capacity 16
  // and a 24h decay window. Capacity is generous so eviction is not
  // triggered by the small test corpora.
  func newCoreWorking(t *testing.T) *coremem.WorkingMemory {
  	t.Helper()
  	w, err := coremem.NewWorking(newCoreEmbedder(), coremem.WorkingOptions{
  		Capacity: 16,
  		Decay:    24 * time.Hour,
  	})
  	if err != nil {
  		t.Fatalf("coremem.NewWorking: %v", err)
  	}
  	return w
  }

  // newCoreEpisodic builds a *coremem.EpisodicMemory with default options.
  func newCoreEpisodic(t *testing.T) *coremem.EpisodicMemory {
  	t.Helper()
  	m, err := coremem.NewEpisodic(newCoreEmbedder(), coremem.EpisodicOptions{})
  	if err != nil {
  		t.Fatalf("coremem.NewEpisodic: %v", err)
  	}
  	return m
  }

  // newCoreSemantic builds a *coremem.SemanticMemory with default options.
  func newCoreSemantic(t *testing.T) *coremem.SemanticMemory {
  	t.Helper()
  	m, err := coremem.NewSemantic(newCoreEmbedder(), coremem.SemanticOptions{})
  	if err != nil {
  		t.Fatalf("coremem.NewSemantic: %v", err)
  	}
  	return m
  }

  // newCoreManager wires all three memory kinds into a *coremem.Manager.
  func newCoreManager(t *testing.T) *coremem.Manager {
  	t.Helper()
  	mgr, err := coremem.NewManager(coremem.ManagerOptions{
  		Working:  newCoreWorking(t),
  		Episodic: newCoreEpisodic(t),
  		Semantic: newCoreSemantic(t),
  	})
  	if err != nil {
  		t.Fatalf("coremem.NewManager: %v", err)
  	}
  	return mgr
  }

  // newCoreScopedManager wraps the manager produced by newCoreManager.
  func newCoreScopedManager(t *testing.T) *coremem.ScopedManager {
  	t.Helper()
  	sm, err := coremem.NewScopedManager(newCoreManager(t))
  	if err != nil {
  		t.Fatalf("coremem.NewScopedManager: %v", err)
  	}
  	return sm
  }
  ```

- [ ] **Step 2: Verify package still compiles (no test runs yet — no test names match anything new)**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestVersion_NonEmptyAndSemver -v`
  Expected: `PASS` for the version test; the helper file compiles cleanly.

- [ ] **Step 3: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem
  git add llm-agent-memory/memory/testutil_test.go
  git commit -m "test(memory): add shared test fixtures (embedder + manager builders)"
  ```

---

## Task 9: A-1.1 — `ConsolidateScoped` failing test

**Files:**
- Create: `llm-agent-memory/memory/scoped_lifecycle_test.go`

- [ ] **Step 1: Write the failing test**

  ```go
  package memory

  import (
  	"context"
  	"testing"

  	coremem "github.com/costa92/llm-agent/memory"
  )

  func TestScopedLifecycle_ConsolidateScoped_OnlyPromotesMatchingScope(t *testing.T) {
  	sm := newCoreScopedManager(t)
  	slm, err := NewScopedLifecycleManager(sm)
  	if err != nil {
  		t.Fatalf("NewScopedLifecycleManager: %v", err)
  	}

  	scopeA := coremem.Scope{User: "alice", Project: "p1"}
  	scopeB := coremem.Scope{User: "bob", Project: "p1"}

  	ctxA := coremem.WithScope(context.Background(), scopeA)
  	ctxB := coremem.WithScope(context.Background(), scopeB)

  	// Alice writes a working item with importance high enough to promote.
  	if _, err := sm.Add(ctxA, coremem.KindWorking, coremem.MemoryItem{
  		Content: "alice note", Importance: 0.9,
  	}); err != nil {
  		t.Fatalf("alice Add: %v", err)
  	}
  	// Bob writes one too.
  	if _, err := sm.Add(ctxB, coremem.KindWorking, coremem.MemoryItem{
  		Content: "bob note", Importance: 0.9,
  	}); err != nil {
  		t.Fatalf("bob Add: %v", err)
  	}

  	// Alice runs ConsolidateScoped. Only her item should be promoted.
  	n, err := slm.ConsolidateScoped(ctxA, coremem.ConsolidateOptions{Threshold: 0.7})
  	if err != nil {
  		t.Fatalf("ConsolidateScoped: %v", err)
  	}
  	if n != 1 {
  		t.Fatalf("promoted = %d, want 1", n)
  	}

  	// Inspect the episodic tier via the inner *Manager: exactly one item,
  	// and it carries Alice's scope.
  	pages, err := sm.Inner().ListAll(context.Background(), coremem.ListFilter{}, 100, nil)
  	if err != nil {
  		t.Fatalf("inner ListAll: %v", err)
  	}
  	epi := pages[coremem.KindEpisodic].Items
  	if len(epi) != 1 {
  		t.Fatalf("episodic count = %d, want 1", len(epi))
  	}
  	if epi[0].Content != "alice note" {
  		t.Errorf("promoted content = %q, want %q", epi[0].Content, "alice note")
  	}
  }
  ```

- [ ] **Step 2: Run test to verify it fails**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestScopedLifecycle_ConsolidateScoped_OnlyPromotesMatchingScope -v`
  Expected: compile error — `undefined: NewScopedLifecycleManager`.

- [ ] **Step 3: Write the minimal implementation `llm-agent-memory/memory/scoped_lifecycle.go`**

  ```go
  package memory

  import (
  	"context"
  	"errors"
  	"fmt"

  	coremem "github.com/costa92/llm-agent/memory"
  )

  // ScopedLifecycleManager wraps a *coremem.ScopedManager and adds three
  // lifecycle methods that honor the ctx scope (closing the v0.7 gap on
  // coremem.ScopedManager: Consolidate / Forget / StatsAll all ignore
  // scope upstream — see llm-agent/memory/scoped_manager.go:128-144).
  //
  // Scope enforcement strategy: enumerate items via the exported
  // coremem.Lister interface (which all three bundled memory types
  // implement), filter by ctx scope using coremem's matching rules, then
  // act on only the matching IDs.
  type ScopedLifecycleManager struct {
  	sm *coremem.ScopedManager
  }

  // ErrScopedManagerRequired is returned by NewScopedLifecycleManager
  // when the inner *coremem.ScopedManager is nil.
  var ErrScopedManagerRequired = errors.New("memory: scoped manager required")

  // NewScopedLifecycleManager wraps an existing *coremem.ScopedManager.
  // Returns ErrScopedManagerRequired if inner is nil.
  func NewScopedLifecycleManager(inner *coremem.ScopedManager) (*ScopedLifecycleManager, error) {
  	if inner == nil {
  		return nil, ErrScopedManagerRequired
  	}
  	return &ScopedLifecycleManager{sm: inner}, nil
  }

  // ConsolidateScoped promotes Working→Episodic only for items whose
  // stored scope matches the ctx scope. A zero-value ctx scope behaves
  // like coremem.Manager.Consolidate (wildcard — every item considered).
  //
  // Threshold defaults to 0.7 if unset, mirroring coremem.Consolidate.
  // MinAge is honored verbatim.
  func (s *ScopedLifecycleManager) ConsolidateScoped(ctx context.Context, opts coremem.ConsolidateOptions) (int, error) {
  	if opts.Threshold <= 0 {
  		opts.Threshold = 0.7
  	}
  	mgr := s.sm.Inner()
  	// Enumerate working items in this scope via the ctx-aware
  	// ScopedManager.ListAll, which applies scope filtering automatically.
  	pages, err := s.sm.ListAll(ctx, coremem.ListFilter{}, 0, nil)
  	if err != nil {
  		return 0, fmt.Errorf("memory: list working: %w", err)
  	}
  	working := pages[coremem.KindWorking].Items
  	count := 0
  	for _, it := range working {
  		if it.Importance < opts.Threshold {
  			continue
  		}
  		if opts.MinAge > 0 {
  			if it.CreatedAt.IsZero() {
  				continue
  			}
  			if !it.CreatedAt.Add(opts.MinAge).Before(timeNow()) {
  				continue
  			}
  		}
  		clone := it
  		clone.ID = "" // let episodic re-generate
  		if _, err := mgr.Add(ctx, coremem.KindEpisodic, clone); err != nil {
  			return count, fmt.Errorf("memory: consolidate-scoped add: %w", err)
  		}
  		count++
  	}
  	return count, nil
  }
  ```

  And add the `timeNow` helper at the bottom of the same file (separate so the dedupe-aware Consolidator in Task 13 can share it):

  ```go
  // timeNow is overridable in tests if a future task needs deterministic
  // clocks; today it is a plain alias to time.Now.
  var timeNow = func() time.Time { return time.Now() }
  ```

  Add the `"time"` import at the top of the file as well — the final import block must read:

  ```go
  import (
  	"context"
  	"errors"
  	"fmt"
  	"time"

  	coremem "github.com/costa92/llm-agent/memory"
  )
  ```

  Note on `s.sm.ListAll`: `coremem.ScopedManager.ListAll(ctx, filter, pageSize, cursors)` (lines 121–126 of `scoped_manager.go`) overrides `filter.Scope` with the ctx scope when non-zero, so passing zero values is correct here. Pass `pageSize=0` — `coremem.listFromStore` does not require a positive page size; the default behavior returns up to the internal page cap (currently 50). For test corpora ≤ 16 items this is sufficient. (For production, follow-up M2 work pages explicitly; this is captured in the M2 roadmap.)

  Caveat: `pageSize=0` may cause `listFromStore` to take its zero-branch. The TEST verifies actual behavior end-to-end, so if `pageSize=0` returns nothing we will see a failing test; in that case use `pageSize=100` (the test corpora are ≤ 16 items so this is safe).

- [ ] **Step 4: Run test to verify it passes**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestScopedLifecycle_ConsolidateScoped_OnlyPromotesMatchingScope -v`
  Expected: `PASS`.

  If the test fails because `pageSize=0` returns empty pages, change the call in `ConsolidateScoped` from `s.sm.ListAll(ctx, coremem.ListFilter{}, 0, nil)` to `s.sm.ListAll(ctx, coremem.ListFilter{}, 100, nil)` and re-run. Re-running is also the verification that the impl is correct.

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem
  git add llm-agent-memory/memory/scoped_lifecycle.go llm-agent-memory/memory/scoped_lifecycle_test.go
  git commit -m "feat(memory): add ScopedLifecycleManager.ConsolidateScoped (A-1)"
  ```

---

## Task 10: A-1.1 — `ConsolidateScoped` cross-scope isolation test

**Files:**
- Modify: `llm-agent-memory/memory/scoped_lifecycle_test.go` (append)

- [ ] **Step 1: Append the failing test**

  ```go
  func TestScopedLifecycle_ConsolidateScoped_DoesNotPromoteOtherScope(t *testing.T) {
  	sm := newCoreScopedManager(t)
  	slm, err := NewScopedLifecycleManager(sm)
  	if err != nil {
  		t.Fatalf("NewScopedLifecycleManager: %v", err)
  	}

  	scopeA := coremem.Scope{User: "alice"}
  	scopeB := coremem.Scope{User: "bob"}

  	ctxA := coremem.WithScope(context.Background(), scopeA)
  	ctxB := coremem.WithScope(context.Background(), scopeB)

  	if _, err := sm.Add(ctxA, coremem.KindWorking, coremem.MemoryItem{Content: "a", Importance: 0.9}); err != nil {
  		t.Fatalf("alice Add: %v", err)
  	}
  	if _, err := sm.Add(ctxB, coremem.KindWorking, coremem.MemoryItem{Content: "b", Importance: 0.9}); err != nil {
  		t.Fatalf("bob Add: %v", err)
  	}

  	// Bob runs ConsolidateScoped. Alice's item must NOT be promoted.
  	n, err := slm.ConsolidateScoped(ctxB, coremem.ConsolidateOptions{Threshold: 0.7})
  	if err != nil {
  		t.Fatalf("ConsolidateScoped: %v", err)
  	}
  	if n != 1 {
  		t.Fatalf("promoted = %d, want 1 (only bob)", n)
  	}

  	pages, _ := sm.Inner().ListAll(context.Background(), coremem.ListFilter{}, 100, nil)
  	epi := pages[coremem.KindEpisodic].Items
  	if len(epi) != 1 {
  		t.Fatalf("episodic count = %d, want 1", len(epi))
  	}
  	if epi[0].Content != "b" {
  		t.Errorf("episodic content = %q, want %q (alice leak!)", epi[0].Content, "b")
  	}
  }
  ```

- [ ] **Step 2: Run test to verify it passes (the impl from Task 9 already enforces scope)**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestScopedLifecycle_ConsolidateScoped_DoesNotPromoteOtherScope -v`
  Expected: `PASS`. If it fails, the impl in Task 9 has a scope leak — fix it before proceeding.

- [ ] **Step 3: No new impl needed — Task 9's `ConsolidateScoped` is the unit under test.**

- [ ] **Step 4: (Skip — verification is Step 2.)**

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem
  git add llm-agent-memory/memory/scoped_lifecycle_test.go
  git commit -m "test(memory): assert ConsolidateScoped does not leak across scopes"
  ```

---

## Task 11: A-1.2 — `ForgetScoped` failing test + impl

**Files:**
- Modify: `llm-agent-memory/memory/scoped_lifecycle.go` (add `ForgetScoped`)
- Modify: `llm-agent-memory/memory/scoped_lifecycle_test.go` (append)

- [ ] **Step 1: Append the failing test**

  ```go
  func TestScopedLifecycle_ForgetScoped_OnlyDeletesMatchingScope(t *testing.T) {
  	sm := newCoreScopedManager(t)
  	slm, err := NewScopedLifecycleManager(sm)
  	if err != nil {
  		t.Fatalf("NewScopedLifecycleManager: %v", err)
  	}

  	scopeA := coremem.Scope{User: "alice"}
  	scopeB := coremem.Scope{User: "bob"}

  	ctxA := coremem.WithScope(context.Background(), scopeA)
  	ctxB := coremem.WithScope(context.Background(), scopeB)

  	// Both users add a low-importance episodic item.
  	if _, err := sm.Add(ctxA, coremem.KindEpisodic, coremem.MemoryItem{Content: "a", Importance: 0.1}); err != nil {
  		t.Fatalf("alice Add: %v", err)
  	}
  	if _, err := sm.Add(ctxB, coremem.KindEpisodic, coremem.MemoryItem{Content: "b", Importance: 0.1}); err != nil {
  		t.Fatalf("bob Add: %v", err)
  	}

  	// Alice forgets by importance threshold 0.5. Bob's item must survive.
  	n, err := slm.ForgetScoped(ctxA, coremem.KindEpisodic, coremem.ForgetOptions{
  		Strategy:  coremem.ForgetByImportance,
  		Threshold: 0.5,
  	})
  	if err != nil {
  		t.Fatalf("ForgetScoped: %v", err)
  	}
  	if n != 1 {
  		t.Fatalf("forgotten = %d, want 1", n)
  	}

  	pages, _ := sm.Inner().ListAll(context.Background(), coremem.ListFilter{}, 100, nil)
  	epi := pages[coremem.KindEpisodic].Items
  	if len(epi) != 1 {
  		t.Fatalf("survivors = %d, want 1 (bob)", len(epi))
  	}
  	if epi[0].Content != "b" {
  		t.Errorf("surviving content = %q, want %q", epi[0].Content, "b")
  	}
  }
  ```

- [ ] **Step 2: Run test to verify it fails**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestScopedLifecycle_ForgetScoped_OnlyDeletesMatchingScope -v`
  Expected: compile error — `slm.ForgetScoped undefined`.

- [ ] **Step 3: Add `ForgetScoped` to `scoped_lifecycle.go`**

  Append to `llm-agent-memory/memory/scoped_lifecycle.go`:

  ```go
  // ForgetScoped applies the given Forget strategy ONLY to items whose
  // stored scope matches the ctx scope. A zero-value ctx scope behaves
  // like coremem.Manager.Forget (every item considered).
  //
  // Pinned items are always skipped, mirroring coremem.Manager.Forget.
  // Strategies supported: ForgetByImportance, ForgetByAge, ForgetByCapacity.
  func (s *ScopedLifecycleManager) ForgetScoped(ctx context.Context, kind coremem.Kind, opts coremem.ForgetOptions) (int, error) {
  	mgr := s.sm.Inner()
  	// Enumerate items in this scope via the ctx-aware ListAll.
  	pages, err := s.sm.ListAll(ctx, coremem.ListFilter{}, 100, nil)
  	if err != nil {
  		return 0, fmt.Errorf("memory: list %s: %w", kind, err)
  	}
  	candidates := pages[kind].Items
  	switch opts.Strategy {
  	case coremem.ForgetByImportance:
  		count := 0
  		for _, it := range candidates {
  			if coremem.IsPinned(it) {
  				continue
  			}
  			if it.Importance < opts.Threshold {
  				if err := mgr.Remove(ctx, kind, it.ID); err == nil {
  					count++
  				}
  			}
  		}
  		return count, nil
  	case coremem.ForgetByAge:
  		if opts.MaxAge <= 0 {
  			return 0, fmt.Errorf("memory: forget by age requires MaxAge > 0")
  		}
  		now := timeNow()
  		count := 0
  		for _, it := range candidates {
  			if coremem.IsPinned(it) {
  				continue
  			}
  			if now.Sub(it.CreatedAt) > opts.MaxAge {
  				if err := mgr.Remove(ctx, kind, it.ID); err == nil {
  					count++
  				}
  			}
  		}
  		return count, nil
  	case coremem.ForgetByCapacity:
  		if opts.Keep <= 0 {
  			return 0, nil
  		}
  		// Sort by importance ascending; evict the lowest first. Pinned
  		// items are excluded entirely (they don't count toward Keep nor
  		// get removed).
  		type pair struct {
  			id  string
  			imp float64
  		}
  		all := make([]pair, 0, len(candidates))
  		for _, it := range candidates {
  			if coremem.IsPinned(it) {
  				continue
  			}
  			all = append(all, pair{it.ID, it.Importance})
  		}
  		if len(all) <= opts.Keep {
  			return 0, nil
  		}
  		sortPairsByImpAsc(all)
  		toEvict := len(all) - opts.Keep
  		count := 0
  		for i := 0; i < toEvict; i++ {
  			if err := mgr.Remove(ctx, kind, all[i].id); err == nil {
  				count++
  			}
  		}
  		return count, nil
  	default:
  		return 0, fmt.Errorf("memory: unknown forget strategy %q", opts.Strategy)
  	}
  }

  // sortPairsByImpAsc is a small sort helper kept package-local so the
  // ForgetByCapacity branch above does not pull in coremem internals.
  func sortPairsByImpAsc(pairs []struct {
  	id  string
  	imp float64
  }) {
  	// Insertion sort — stable, simple, and the only place in this
  	// package that needs ordering. N is small (page size 100).
  	for i := 1; i < len(pairs); i++ {
  		for j := i; j > 0 && pairs[j-1].imp > pairs[j].imp; j-- {
  			pairs[j-1], pairs[j] = pairs[j], pairs[j-1]
  		}
  	}
  }
  ```

  Note on type signature: the `sortPairsByImpAsc` helper uses an anonymous struct type identical to the one inside `ForgetByCapacity`'s scope. Anonymous struct types with the same field set are assignable in Go, but to avoid friction, lift the type to a package-private named type at the top of the file (just below `ScopedLifecycleManager`):

  ```go
  // forgetPair is the (id, importance) tuple used by the capacity-based
  // Forget branch. Kept package-private — no caller need.
  type forgetPair struct {
  	id  string
  	imp float64
  }
  ```

  Then change the `ForgetByCapacity` branch to declare `all := make([]forgetPair, 0, len(candidates))`, append `forgetPair{it.ID, it.Importance}`, and change `sortPairsByImpAsc` to accept `pairs []forgetPair`.

- [ ] **Step 4: Run test to verify it passes**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestScopedLifecycle_ForgetScoped_OnlyDeletesMatchingScope -v`
  Expected: `PASS`.

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem
  git add llm-agent-memory/memory/scoped_lifecycle.go llm-agent-memory/memory/scoped_lifecycle_test.go
  git commit -m "feat(memory): add ScopedLifecycleManager.ForgetScoped (A-1)"
  ```

---

## Task 12: A-1.3 — `StatsScoped` failing test + impl

**Files:**
- Modify: `llm-agent-memory/memory/scoped_lifecycle.go` (add `StatsScoped`)
- Modify: `llm-agent-memory/memory/scoped_lifecycle_test.go` (append)

- [ ] **Step 1: Append the failing test**

  ```go
  func TestScopedLifecycle_StatsScoped_CountsOnlyMatchingScope(t *testing.T) {
  	sm := newCoreScopedManager(t)
  	slm, err := NewScopedLifecycleManager(sm)
  	if err != nil {
  		t.Fatalf("NewScopedLifecycleManager: %v", err)
  	}

  	scopeA := coremem.Scope{User: "alice"}
  	scopeB := coremem.Scope{User: "bob"}

  	ctxA := coremem.WithScope(context.Background(), scopeA)
  	ctxB := coremem.WithScope(context.Background(), scopeB)

  	if _, err := sm.Add(ctxA, coremem.KindWorking, coremem.MemoryItem{Content: "a1", Importance: 0.5}); err != nil {
  		t.Fatalf("alice Add 1: %v", err)
  	}
  	if _, err := sm.Add(ctxA, coremem.KindWorking, coremem.MemoryItem{Content: "a2", Importance: 0.5}); err != nil {
  		t.Fatalf("alice Add 2: %v", err)
  	}
  	if _, err := sm.Add(ctxB, coremem.KindWorking, coremem.MemoryItem{Content: "b1", Importance: 0.5}); err != nil {
  		t.Fatalf("bob Add: %v", err)
  	}

  	statsA, err := slm.StatsScoped(ctxA)
  	if err != nil {
  		t.Fatalf("StatsScoped(alice): %v", err)
  	}
  	if got := statsA[coremem.KindWorking].Count; got != 2 {
  		t.Errorf("alice working Count = %d, want 2", got)
  	}

  	statsB, err := slm.StatsScoped(ctxB)
  	if err != nil {
  		t.Fatalf("StatsScoped(bob): %v", err)
  	}
  	if got := statsB[coremem.KindWorking].Count; got != 1 {
  		t.Errorf("bob working Count = %d, want 1", got)
  	}
  }
  ```

- [ ] **Step 2: Run test to verify it fails**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestScopedLifecycle_StatsScoped_CountsOnlyMatchingScope -v`
  Expected: compile error — `slm.StatsScoped undefined`.

- [ ] **Step 3: Add `StatsScoped` to `scoped_lifecycle.go`**

  Append to `llm-agent-memory/memory/scoped_lifecycle.go`:

  ```go
  // StatsScoped returns per-kind Stats covering only items whose stored
  // scope matches the ctx scope. A zero-value ctx scope behaves like
  // coremem.Manager.StatsAll (every item counted).
  //
  // Returned Stats.Capacity mirrors the underlying memory's capacity
  // (NOT a scope-local cap), because capacity is a per-memory-type
  // attribute, not a per-scope one.
  func (s *ScopedLifecycleManager) StatsScoped(ctx context.Context) (map[coremem.Kind]coremem.Stats, error) {
  	pages, err := s.sm.ListAll(ctx, coremem.ListFilter{}, 100, nil)
  	if err != nil {
  		return nil, fmt.Errorf("memory: stats list: %w", err)
  	}
  	innerStats := s.sm.Inner().StatsAll()
  	out := make(map[coremem.Kind]coremem.Stats, len(pages))
  	now := timeNow()
  	for kind, page := range pages {
  		var (
  			count   = len(page.Items)
  			impSum  float64
  			oldest  time.Time
  			hasItem bool
  		)
  		for _, it := range page.Items {
  			impSum += it.Importance
  			if !hasItem || it.CreatedAt.Before(oldest) {
  				oldest = it.CreatedAt
  				hasItem = true
  			}
  		}
  		var avg float64
  		if count > 0 {
  			avg = impSum / float64(count)
  		}
  		var oldestAge time.Duration
  		if hasItem {
  			oldestAge = now.Sub(oldest)
  		}
  		out[kind] = coremem.Stats{
  			Count:         count,
  			Capacity:      innerStats[kind].Capacity,
  			OldestAge:     oldestAge,
  			AvgImportance: avg,
  		}
  	}
  	return out, nil
  }
  ```

- [ ] **Step 4: Run test to verify it passes**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestScopedLifecycle_StatsScoped_CountsOnlyMatchingScope -v`
  Expected: `PASS`.

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem
  git add llm-agent-memory/memory/scoped_lifecycle.go llm-agent-memory/memory/scoped_lifecycle_test.go
  git commit -m "feat(memory): add ScopedLifecycleManager.StatsScoped (A-1)"
  ```

---

## Task 13: A-2.1 — Consolidator writes dedupe metadata on first promotion

**Files:**
- Create: `llm-agent-memory/memory/consolidator.go`
- Create: `llm-agent-memory/memory/consolidator_test.go`

- [ ] **Step 1: Write the failing test**

  ```go
  package memory

  import (
  	"context"
  	"testing"

  	coremem "github.com/costa92/llm-agent/memory"
  )

  func TestConsolidator_FirstPromote_WritesDedupeMetadata(t *testing.T) {
  	mgr := newCoreManager(t)
  	c, err := NewConsolidator(mgr)
  	if err != nil {
  		t.Fatalf("NewConsolidator: %v", err)
  	}

  	ctx := context.Background()
  	id, err := mgr.Add(ctx, coremem.KindWorking, coremem.MemoryItem{
  		Content: "important note", Importance: 0.9,
  	})
  	if err != nil {
  		t.Fatalf("Add: %v", err)
  	}

  	n, err := c.Consolidate(ctx, coremem.ConsolidateOptions{Threshold: 0.7})
  	if err != nil {
  		t.Fatalf("Consolidate: %v", err)
  	}
  	if n != 1 {
  		t.Fatalf("promoted = %d, want 1", n)
  	}

  	// Source item must now carry the dedupe metadata keys.
  	src, err := mgr.Get(ctx, coremem.KindWorking, id)
  	if err != nil {
  		t.Fatalf("Get source: %v", err)
  	}
  	if _, ok := src.Metadata[MetaKeyConsolidatedAt]; !ok {
  		t.Errorf("Metadata[%q] missing", MetaKeyConsolidatedAt)
  	}
  	if got, _ := src.Metadata[MetaKeyPromotionCount].(int); got != 1 {
  		t.Errorf("Metadata[%q] = %v, want int 1", MetaKeyPromotionCount, src.Metadata[MetaKeyPromotionCount])
  	}

  	// The episodic clone must carry the back-reference.
  	pages, _ := mgr.ListAll(ctx, coremem.ListFilter{}, 100, nil)
  	epi := pages[coremem.KindEpisodic].Items
  	if len(epi) != 1 {
  		t.Fatalf("episodic count = %d, want 1", len(epi))
  	}
  	if got, _ := epi[0].Metadata[MetaKeyPromotedFrom].(string); got != id {
  		t.Errorf("episodic Metadata[%q] = %q, want %q", MetaKeyPromotedFrom, got, id)
  	}
  }
  ```

- [ ] **Step 2: Run test to verify it fails**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestConsolidator_FirstPromote_WritesDedupeMetadata -v`
  Expected: compile error — `undefined: NewConsolidator`, `MetaKeyConsolidatedAt`, etc.

- [ ] **Step 3: Write the minimal implementation `llm-agent-memory/memory/consolidator.go`**

  ```go
  package memory

  import (
  	"context"
  	"errors"
  	"fmt"

  	coremem "github.com/costa92/llm-agent/memory"
  )

  // Reserved metadata keys written by Consolidator.Consolidate on
  // promotion. They start with a single underscore to match the
  // convention used by coremem's private keys (_scope, _source, etc.)
  // and to clearly mark them as internal-extension.
  const (
  	// MetaKeyConsolidatedAt is written on the source working item as a
  	// time.Time (formatted RFC 3339 when JSON-round-tripped via
  	// encoding/json — go's default behavior).
  	MetaKeyConsolidatedAt = "_consolidated_at"

  	// MetaKeyPromotedFrom is written on the episodic clone as a string
  	// pointing at the source working item's ID.
  	MetaKeyPromotedFrom = "_promoted_from"

  	// MetaKeyPromotionCount is written on the source working item as an
  	// int. Incremented on each successful promotion (currently capped at
  	// 1 by the promote-once policy enforced by Consolidator.Consolidate).
  	MetaKeyPromotionCount = "_promotion_count"
  )

  // Consolidator wraps a *coremem.Manager and exposes a dedupe-aware
  // Consolidate that mirrors coremem.Manager.Consolidate (copy
  // Working→Episodic by importance + min-age) but additionally:
  //
  //  1. Writes MetaKeyConsolidatedAt and MetaKeyPromotionCount on the
  //     source working item via coremem.Manager.Update.
  //  2. Writes MetaKeyPromotedFrom on the episodic clone.
  //  3. Skips items whose MetaKeyPromotionCount is already ≥ 1
  //     (the v0.1 promote-once policy).
  //
  // Source items are NOT removed (mirrors coremem semantics). Pinned and
  // disabled status on the source are preserved verbatim by Update.
  type Consolidator struct {
  	mgr *coremem.Manager
  }

  // ErrManagerRequired is returned by NewConsolidator when the inner
  // *coremem.Manager is nil. (Same sentinel name as coremem's, but
  // distinct identity — callers should errors.Is on the local one.)
  var ErrManagerRequired = errors.New("memory: manager required")

  // NewConsolidator wraps an existing *coremem.Manager. Returns
  // ErrManagerRequired if inner is nil.
  func NewConsolidator(inner *coremem.Manager) (*Consolidator, error) {
  	if inner == nil {
  		return nil, ErrManagerRequired
  	}
  	return &Consolidator{mgr: inner}, nil
  }

  // Consolidate enumerates Working via the Lister capability, applies
  // Threshold + MinAge, skips items already promoted (MetaKeyPromotionCount
  // ≥ 1), copies survivors into Episodic with MetaKeyPromotedFrom set,
  // and writes MetaKeyConsolidatedAt + MetaKeyPromotionCount on each
  // source. Returns the number of items promoted in this call.
  //
  // Threshold defaults to 0.7 if unset (matches coremem).
  func (c *Consolidator) Consolidate(ctx context.Context, opts coremem.ConsolidateOptions) (int, error) {
  	if opts.Threshold <= 0 {
  		opts.Threshold = 0.7
  	}
  	pages, err := c.mgr.ListAll(ctx, coremem.ListFilter{}, 100, nil)
  	if err != nil {
  		return 0, fmt.Errorf("memory: consolidate list: %w", err)
  	}
  	working := pages[coremem.KindWorking].Items
  	now := timeNow()
  	count := 0
  	for _, it := range working {
  		if it.Importance < opts.Threshold {
  			continue
  		}
  		if opts.MinAge > 0 && now.Sub(it.CreatedAt) < opts.MinAge {
  			continue
  		}
  		if promotionCountOf(it) >= 1 {
  			continue
  		}
  		clone := it
  		clone.ID = "" // let episodic re-generate
  		if clone.Metadata == nil {
  			clone.Metadata = map[string]any{}
  		} else {
  			// Deep-copy the metadata map so we don't mutate the source
  			// item's map indirectly.
  			cp := make(map[string]any, len(clone.Metadata)+1)
  			for k, v := range clone.Metadata {
  				cp[k] = v
  			}
  			clone.Metadata = cp
  		}
  		clone.Metadata[MetaKeyPromotedFrom] = it.ID
  		if _, err := c.mgr.Add(ctx, coremem.KindEpisodic, clone); err != nil {
  			return count, fmt.Errorf("memory: consolidate add: %w", err)
  		}
  		srcID := it.ID
  		err := c.mgr.Update(ctx, coremem.KindWorking, srcID, func(m *coremem.MemoryItem) {
  			if m.Metadata == nil {
  				m.Metadata = map[string]any{}
  			}
  			m.Metadata[MetaKeyConsolidatedAt] = now
  			m.Metadata[MetaKeyPromotionCount] = promotionCountOf(*m) + 1
  		})
  		if err != nil {
  			return count, fmt.Errorf("memory: consolidate stamp source: %w", err)
  		}
  		count++
  	}
  	return count, nil
  }

  // promotionCountOf reads MetaKeyPromotionCount from an item, tolerating
  // both int (what we write) and float64 (what JSON round-trips produce).
  // Returns 0 if absent or wrong type.
  func promotionCountOf(it coremem.MemoryItem) int {
  	if it.Metadata == nil {
  		return 0
  	}
  	raw, ok := it.Metadata[MetaKeyPromotionCount]
  	if !ok {
  		return 0
  	}
  	switch v := raw.(type) {
  	case int:
  		return v
  	case int64:
  		return int(v)
  	case float64:
  		return int(v)
  	default:
  		return 0
  	}
  }
  ```

- [ ] **Step 4: Run test to verify it passes**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestConsolidator_FirstPromote_WritesDedupeMetadata -v`
  Expected: `PASS`.

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem
  git add llm-agent-memory/memory/consolidator.go llm-agent-memory/memory/consolidator_test.go
  git commit -m "feat(memory): add Consolidator that stamps dedupe metadata on first promote (A-2)"
  ```

---

## Task 14: A-2.2 — Second Consolidate call is a no-op (promote-once)

**Files:**
- Modify: `llm-agent-memory/memory/consolidator_test.go` (append)

- [ ] **Step 1: Append the failing test**

  ```go
  func TestConsolidator_SecondCall_DoesNotRePromote(t *testing.T) {
  	mgr := newCoreManager(t)
  	c, err := NewConsolidator(mgr)
  	if err != nil {
  		t.Fatalf("NewConsolidator: %v", err)
  	}

  	ctx := context.Background()
  	if _, err := mgr.Add(ctx, coremem.KindWorking, coremem.MemoryItem{
  		Content: "promote me once", Importance: 0.9,
  	}); err != nil {
  		t.Fatalf("Add: %v", err)
  	}

  	n1, err := c.Consolidate(ctx, coremem.ConsolidateOptions{Threshold: 0.7})
  	if err != nil {
  		t.Fatalf("Consolidate #1: %v", err)
  	}
  	if n1 != 1 {
  		t.Fatalf("Consolidate #1 promoted = %d, want 1", n1)
  	}

  	n2, err := c.Consolidate(ctx, coremem.ConsolidateOptions{Threshold: 0.7})
  	if err != nil {
  		t.Fatalf("Consolidate #2: %v", err)
  	}
  	if n2 != 0 {
  		t.Errorf("Consolidate #2 promoted = %d, want 0 (promote-once policy)", n2)
  	}

  	// Episodic must still hold exactly one copy.
  	pages, _ := mgr.ListAll(ctx, coremem.ListFilter{}, 100, nil)
  	if got := len(pages[coremem.KindEpisodic].Items); got != 1 {
  		t.Errorf("episodic count = %d, want 1 (no duplicate)", got)
  	}
  }
  ```

- [ ] **Step 2: Run test to verify it passes (impl from Task 13 already enforces promote-once)**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestConsolidator_SecondCall_DoesNotRePromote -v`
  Expected: `PASS`. If it fails, the dedupe gate in Task 13 has a bug — fix `promotionCountOf` or the call site.

- [ ] **Step 3: (No new impl.)**

- [ ] **Step 4: (Skip — verification is Step 2.)**

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem
  git add llm-agent-memory/memory/consolidator_test.go
  git commit -m "test(memory): assert Consolidator enforces promote-once policy"
  ```

---

## Task 15: A-2.3 — Dedupe metadata round-trips through Export/Import

**Files:**
- Modify: `llm-agent-memory/memory/consolidator_test.go` (append)

- [ ] **Step 1: Append the failing test**

  ```go
  func TestConsolidator_DedupeMetadata_RoundTripsThroughExportImport(t *testing.T) {
  	// Build mgr A, promote once, export Working snapshot, import into a
  	// fresh mgr B, then assert: (a) the source still carries the dedupe
  	// metadata, and (b) re-running Consolidate on mgr B is a no-op.
  	mgrA := newCoreManager(t)
  	c, err := NewConsolidator(mgrA)
  	if err != nil {
  		t.Fatalf("NewConsolidator: %v", err)
  	}

  	ctx := context.Background()
  	if _, err := mgrA.Add(ctx, coremem.KindWorking, coremem.MemoryItem{
  		Content: "ride-along", Importance: 0.9,
  	}); err != nil {
  		t.Fatalf("Add: %v", err)
  	}
  	if _, err := c.Consolidate(ctx, coremem.ConsolidateOptions{Threshold: 0.7}); err != nil {
  		t.Fatalf("Consolidate: %v", err)
  	}

  	snaps, err := mgrA.ExportAll(ctx, "")
  	if err != nil {
  		t.Fatalf("ExportAll: %v", err)
  	}
  	workingSnap, ok := snaps[coremem.KindWorking]
  	if !ok {
  		t.Fatalf("working snapshot missing")
  	}

  	// Force a JSON round-trip so map[string]any types reflect what an
  	// over-the-wire reload would produce.
  	roundTripped := jsonRoundTripSnap(t, workingSnap)

  	mgrB := newCoreManager(t)
  	rpt, err := mgrB.ImportAll(ctx, map[coremem.Kind]coremem.Snapshot{
  		coremem.KindWorking: roundTripped,
  	}, "", coremem.ImportReplace)
  	if err != nil {
  		t.Fatalf("ImportAll: %v", err)
  	}
  	if rpt[coremem.KindWorking].Loaded != 1 {
  		t.Fatalf("Loaded = %d, want 1", rpt[coremem.KindWorking].Loaded)
  	}

  	// Re-run Consolidate on mgr B — must be a no-op because the
  	// imported source item still carries MetaKeyPromotionCount == 1.
  	cB, err := NewConsolidator(mgrB)
  	if err != nil {
  		t.Fatalf("NewConsolidator B: %v", err)
  	}
  	n, err := cB.Consolidate(ctx, coremem.ConsolidateOptions{Threshold: 0.7})
  	if err != nil {
  		t.Fatalf("Consolidate B: %v", err)
  	}
  	if n != 0 {
  		t.Errorf("Consolidate after import promoted = %d, want 0 (metadata must survive round-trip)", n)
  	}
  }
  ```

- [ ] **Step 2: Add the JSON round-trip helper to `testutil_test.go`**

  Append to `llm-agent-memory/memory/testutil_test.go`:

  ```go
  // jsonRoundTripSnap encodes then decodes a Snapshot through
  // encoding/json. This forces Metadata maps to use the concrete types
  // that the wire format actually produces (int → float64, etc.) so
  // downstream readers like promotionCountOf are tested under the same
  // conditions an Import-from-disk path would see.
  func jsonRoundTripSnap(t *testing.T, snap coremem.Snapshot) coremem.Snapshot {
  	t.Helper()
  	b, err := json.Marshal(snap)
  	if err != nil {
  		t.Fatalf("json.Marshal snapshot: %v", err)
  	}
  	var out coremem.Snapshot
  	if err := json.Unmarshal(b, &out); err != nil {
  		t.Fatalf("json.Unmarshal snapshot: %v", err)
  	}
  	return out
  }
  ```

  And add `"encoding/json"` to the import block of `testutil_test.go`.

- [ ] **Step 3: Run test to verify it passes (impl from Task 13's `promotionCountOf` already handles `float64`)**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestConsolidator_DedupeMetadata_RoundTripsThroughExportImport -v`
  Expected: `PASS`. If it fails because `promotionCountOf` returns 0 after JSON round-trip, audit the `switch v := raw.(type)` branches to confirm `float64` is covered (it is, per the Task 13 impl).

- [ ] **Step 4: (No new impl unless Step 3 reveals a regression.)**

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem
  git add llm-agent-memory/memory/consolidator_test.go llm-agent-memory/memory/testutil_test.go
  git commit -m "test(memory): assert Consolidator dedupe metadata survives Export+JSON+Import"
  ```

---

## Task 16: A-3.1 — `SearchUnified` returns results from every tier (fan-out)

**Files:**
- Create: `llm-agent-memory/memory/unified_search.go`
- Create: `llm-agent-memory/memory/unified_search_test.go`

- [ ] **Step 1: Write the failing test**

  ```go
  package memory

  import (
  	"context"
  	"testing"

  	coremem "github.com/costa92/llm-agent/memory"
  )

  func TestUnifiedSearcher_FansOutToAllTiers(t *testing.T) {
  	mgr := newCoreManager(t)
  	u, err := NewUnifiedSearcher(mgr)
  	if err != nil {
  		t.Fatalf("NewUnifiedSearcher: %v", err)
  	}

  	ctx := context.Background()
  	if _, err := mgr.Add(ctx, coremem.KindWorking, coremem.MemoryItem{Content: "go modules", Importance: 0.5}); err != nil {
  		t.Fatalf("working Add: %v", err)
  	}
  	if _, err := mgr.Add(ctx, coremem.KindEpisodic, coremem.MemoryItem{Content: "go modules history", Importance: 0.5}); err != nil {
  		t.Fatalf("episodic Add: %v", err)
  	}
  	if _, err := mgr.Add(ctx, coremem.KindSemantic, coremem.MemoryItem{Content: "go modules guide", Tags: []string{"go"}, Importance: 0.5}); err != nil {
  		t.Fatalf("semantic Add: %v", err)
  	}

  	results, err := u.SearchUnified(ctx, "go modules", 10)
  	if err != nil {
  		t.Fatalf("SearchUnified: %v", err)
  	}
  	if len(results) == 0 {
  		t.Fatal("got 0 results, want ≥ 1 from each tier merged")
  	}

  	// Collect the set of contents we saw — must include at least one
  	// item from each of the three tiers (proven by content marker).
  	seen := make(map[string]bool, len(results))
  	for _, r := range results {
  		seen[r.Item.Content] = true
  	}
  	for _, want := range []string{"go modules", "go modules history", "go modules guide"} {
  		if !seen[want] {
  			t.Errorf("SearchUnified missing %q (results: %v)", want, contentsOf(results))
  		}
  	}
  }

  // contentsOf is a small test helper to print the content slice on
  // failure. Kept inside _test.go so it doesn't leak into the public API.
  func contentsOf(rs []coremem.SearchResult) []string {
  	out := make([]string, len(rs))
  	for i, r := range rs {
  		out[i] = r.Item.Content
  	}
  	return out
  }
  ```

- [ ] **Step 2: Run test to verify it fails**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestUnifiedSearcher_FansOutToAllTiers -v`
  Expected: compile error — `undefined: NewUnifiedSearcher`.

- [ ] **Step 3: Write the minimal implementation `llm-agent-memory/memory/unified_search.go`**

  ```go
  package memory

  import (
  	"context"
  	"errors"
  	"fmt"
  	"sort"

  	coremem "github.com/costa92/llm-agent/memory"
  )

  // UnifiedSearcher wraps a *coremem.Manager and exposes SearchUnified,
  // a single cross-tier recall surface that merges, dedupes, sorts, and
  // caps results across Working / Episodic / Semantic. It complements
  // (does not replace) coremem.Manager.SearchAll, which keeps the per-
  // kind buckets useful for debugging.
  //
  // Score semantics: each tier returns scores in its own scale. v0.1
  // performs a HEURISTIC merge — scores are kept as-is and sorted
  // descending. A future task may introduce per-tier normalization
  // (this is captured in the roadmap as an M1 open question).
  type UnifiedSearcher struct {
  	mgr *coremem.Manager
  }

  // ErrUnifiedManagerRequired is returned by NewUnifiedSearcher when the
  // inner *coremem.Manager is nil.
  var ErrUnifiedManagerRequired = errors.New("memory: unified searcher requires manager")

  // NewUnifiedSearcher wraps an existing *coremem.Manager. Returns
  // ErrUnifiedManagerRequired if inner is nil.
  func NewUnifiedSearcher(inner *coremem.Manager) (*UnifiedSearcher, error) {
  	if inner == nil {
  		return nil, ErrUnifiedManagerRequired
  	}
  	return &UnifiedSearcher{mgr: inner}, nil
  }

  // SearchUnified fans out the query to every active memory kind via
  // coremem.Manager.SearchAll, merges the per-kind result lists into a
  // single slice, dedupes by (Item.ID, Item.Content) keeping the
  // highest-scoring entry, sorts by Score descending, and truncates to
  // topK (when topK > 0; topK ≤ 0 returns the full merged set).
  //
  // The per-kind topK passed to SearchAll is the same topK argument the
  // caller provides, so each tier returns its top-topK candidates before
  // merge. This means SearchUnified inspects at most 3 × topK candidates.
  func (u *UnifiedSearcher) SearchUnified(ctx context.Context, query string, topK int) ([]coremem.SearchResult, error) {
  	perKind, err := u.mgr.SearchAll(ctx, query, topK)
  	if err != nil {
  		return nil, fmt.Errorf("memory: unified search fan-out: %w", err)
  	}
  	// Merge.
  	merged := make([]coremem.SearchResult, 0)
  	for _, kind := range []coremem.Kind{coremem.KindWorking, coremem.KindEpisodic, coremem.KindSemantic} {
  		merged = append(merged, perKind[kind]...)
  	}
  	// Dedupe by (ID, Content). Keep the highest-scoring entry per key.
  	type key struct {
  		id      string
  		content string
  	}
  	best := make(map[key]coremem.SearchResult, len(merged))
  	for _, r := range merged {
  		k := key{id: r.Item.ID, content: r.Item.Content}
  		prev, ok := best[k]
  		if !ok || r.Score > prev.Score {
  			best[k] = r
  		}
  	}
  	out := make([]coremem.SearchResult, 0, len(best))
  	for _, r := range best {
  		out = append(out, r)
  	}
  	// Sort by Score desc; break ties on ID asc for determinism.
  	sort.Slice(out, func(i, j int) bool {
  		if out[i].Score != out[j].Score {
  			return out[i].Score > out[j].Score
  		}
  		return out[i].Item.ID < out[j].Item.ID
  	})
  	if topK > 0 && len(out) > topK {
  		out = out[:topK]
  	}
  	return out, nil
  }
  ```

- [ ] **Step 4: Run test to verify it passes**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestUnifiedSearcher_FansOutToAllTiers -v`
  Expected: `PASS`.

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem
  git add llm-agent-memory/memory/unified_search.go llm-agent-memory/memory/unified_search_test.go
  git commit -m "feat(memory): add UnifiedSearcher.SearchUnified with cross-tier fan-out (A-3)"
  ```

---

## Task 17: A-3.2 — `SearchUnified` dedupes by (ID, Content)

**Files:**
- Modify: `llm-agent-memory/memory/unified_search_test.go` (append)

- [ ] **Step 1: Append the failing test**

  ```go
  func TestUnifiedSearcher_DedupesByIDAndContent(t *testing.T) {
  	mgr := newCoreManager(t)
  	u, err := NewUnifiedSearcher(mgr)
  	if err != nil {
  		t.Fatalf("NewUnifiedSearcher: %v", err)
  	}

  	ctx := context.Background()
  	// Same Content + same ID across Working and Episodic — should
  	// collapse to a single result.
  	const sharedID = "fixed-id-001"
  	const sharedContent = "duplicated note"
  	if _, err := mgr.Add(ctx, coremem.KindWorking, coremem.MemoryItem{ID: sharedID, Content: sharedContent, Importance: 0.5}); err != nil {
  		t.Fatalf("working Add: %v", err)
  	}
  	if _, err := mgr.Add(ctx, coremem.KindEpisodic, coremem.MemoryItem{ID: sharedID, Content: sharedContent, Importance: 0.5}); err != nil {
  		t.Fatalf("episodic Add: %v", err)
  	}

  	results, err := u.SearchUnified(ctx, "duplicated note", 10)
  	if err != nil {
  		t.Fatalf("SearchUnified: %v", err)
  	}

  	dupCount := 0
  	for _, r := range results {
  		if r.Item.ID == sharedID && r.Item.Content == sharedContent {
  			dupCount++
  		}
  	}
  	if dupCount != 1 {
  		t.Errorf("dup count = %d, want 1 (got results: %v)", dupCount, contentsOf(results))
  	}
  }
  ```

- [ ] **Step 2: Run test to verify it passes (the dedupe map in Task 16 keys by `(ID, Content)`)**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestUnifiedSearcher_DedupesByIDAndContent -v`
  Expected: `PASS`. If it fails (dupCount != 1), audit the dedupe map key in `SearchUnified`.

  Caveat: coremem's three memory types generate IDs when `item.ID == ""`. We pass a fixed ID; each tier accepts it verbatim (per `coremem.WorkingMemory.Add` and friends — verify by reading line 56 of `working.go` which uses the supplied ID when non-empty). If a tier overwrites the ID, the test will reveal it by failing.

- [ ] **Step 3: (No new impl unless Step 2 reveals a regression.)**

- [ ] **Step 4: (Skip — verification is Step 2.)**

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem
  git add llm-agent-memory/memory/unified_search_test.go
  git commit -m "test(memory): assert SearchUnified collapses (ID, Content) duplicates"
  ```

---

## Task 18: A-3.3 — `SearchUnified` sorts by Score descending

**Files:**
- Modify: `llm-agent-memory/memory/unified_search_test.go` (append)

- [ ] **Step 1: Append the failing test**

  ```go
  func TestUnifiedSearcher_SortsByScoreDescending(t *testing.T) {
  	mgr := newCoreManager(t)
  	u, err := NewUnifiedSearcher(mgr)
  	if err != nil {
  		t.Fatalf("NewUnifiedSearcher: %v", err)
  	}

  	ctx := context.Background()
  	// Two clearly-distinguishable contents so any tier produces a
  	// score difference. We don't assert exact scores — just that the
  	// returned slice is monotonically non-increasing in Score.
  	if _, err := mgr.Add(ctx, coremem.KindWorking, coremem.MemoryItem{Content: "go modules", Importance: 0.5}); err != nil {
  		t.Fatalf("Add 1: %v", err)
  	}
  	if _, err := mgr.Add(ctx, coremem.KindEpisodic, coremem.MemoryItem{Content: "unrelated cooking recipe", Importance: 0.5}); err != nil {
  		t.Fatalf("Add 2: %v", err)
  	}
  	if _, err := mgr.Add(ctx, coremem.KindSemantic, coremem.MemoryItem{Content: "go modules guide", Tags: []string{"go"}, Importance: 0.5}); err != nil {
  		t.Fatalf("Add 3: %v", err)
  	}

  	results, err := u.SearchUnified(ctx, "go modules", 10)
  	if err != nil {
  		t.Fatalf("SearchUnified: %v", err)
  	}
  	for i := 1; i < len(results); i++ {
  		if results[i-1].Score < results[i].Score {
  			t.Errorf("results not sorted desc at i=%d: %v < %v", i, results[i-1].Score, results[i].Score)
  		}
  	}
  }
  ```

- [ ] **Step 2: Run test to verify it passes (sort exists in Task 16 impl)**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestUnifiedSearcher_SortsByScoreDescending -v`
  Expected: `PASS`.

- [ ] **Step 3: (No new impl.)**

- [ ] **Step 4: (Skip.)**

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem
  git add llm-agent-memory/memory/unified_search_test.go
  git commit -m "test(memory): assert SearchUnified returns results sorted by Score desc"
  ```

---

## Task 19: A-3.4 — `SearchUnified` honors `topK`

**Files:**
- Modify: `llm-agent-memory/memory/unified_search_test.go` (append)

- [ ] **Step 1: Append the failing test**

  ```go
  func TestUnifiedSearcher_HonorsTopK(t *testing.T) {
  	mgr := newCoreManager(t)
  	u, err := NewUnifiedSearcher(mgr)
  	if err != nil {
  		t.Fatalf("NewUnifiedSearcher: %v", err)
  	}

  	ctx := context.Background()
  	// Seed 5 distinct items across tiers, all matching the query.
  	contents := []struct {
  		kind    coremem.Kind
  		content string
  	}{
  		{coremem.KindWorking, "go alpha"},
  		{coremem.KindWorking, "go bravo"},
  		{coremem.KindEpisodic, "go charlie"},
  		{coremem.KindEpisodic, "go delta"},
  		{coremem.KindSemantic, "go echo"},
  	}
  	for _, c := range contents {
  		if _, err := mgr.Add(ctx, c.kind, coremem.MemoryItem{Content: c.content, Importance: 0.5}); err != nil {
  			t.Fatalf("Add %s/%s: %v", c.kind, c.content, err)
  		}
  	}

  	results, err := u.SearchUnified(ctx, "go", 3)
  	if err != nil {
  		t.Fatalf("SearchUnified: %v", err)
  	}
  	if got := len(results); got > 3 {
  		t.Errorf("len(results) = %d, want ≤ 3", got)
  	}
  }
  ```

- [ ] **Step 2: Run test to verify it passes (truncation lives in Task 16 impl)**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestUnifiedSearcher_HonorsTopK -v`
  Expected: `PASS`.

- [ ] **Step 3: (No new impl.)**

- [ ] **Step 4: (Skip.)**

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem
  git add llm-agent-memory/memory/unified_search_test.go
  git commit -m "test(memory): assert SearchUnified honors topK cap"
  ```

---

## Task 20: Backwards-compat assertion — core `SearchAll` is untouched

**Files:**
- Modify: `llm-agent-memory/memory/unified_search_test.go` (append)

- [ ] **Step 1: Append the test**

  ```go
  func TestUnifiedSearcher_DoesNotAlterCoreSearchAll(t *testing.T) {
  	mgr := newCoreManager(t)
  	u, err := NewUnifiedSearcher(mgr)
  	if err != nil {
  		t.Fatalf("NewUnifiedSearcher: %v", err)
  	}

  	ctx := context.Background()
  	if _, err := mgr.Add(ctx, coremem.KindEpisodic, coremem.MemoryItem{Content: "alpha", Importance: 0.5}); err != nil {
  		t.Fatalf("Add: %v", err)
  	}

  	// First run unified — must not mutate state that SearchAll sees.
  	if _, err := u.SearchUnified(ctx, "alpha", 5); err != nil {
  		t.Fatalf("SearchUnified: %v", err)
  	}

  	out, err := mgr.SearchAll(ctx, "alpha", 5)
  	if err != nil {
  		t.Fatalf("SearchAll: %v", err)
  	}
  	if len(out[coremem.KindEpisodic]) == 0 {
  		t.Errorf("SearchAll lost the episodic result post-SearchUnified")
  	}
  }
  ```

- [ ] **Step 2: Run the test**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./memory -run TestUnifiedSearcher_DoesNotAlterCoreSearchAll -v`
  Expected: `PASS`.

- [ ] **Step 3: (No new impl.)**

- [ ] **Step 4: (Skip.)**

- [ ] **Step 5: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem
  git add llm-agent-memory/memory/unified_search_test.go
  git commit -m "test(memory): assert SearchUnified leaves core SearchAll behavior unchanged"
  ```

---

## Task 21: Full-package smoke + coverage check

**Files:** none (verification only)

- [ ] **Step 1: Run the entire new module's test suite with race detector**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go test ./... -count=1 -race`
  Expected: `ok github.com/costa92/llm-agent-memory/memory ...`, exit code 0, no race warnings.

- [ ] **Step 2: Run `go vet` on the entire module**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && GOWORK=off go vet ./...`
  Expected: no output, exit code 0.

- [ ] **Step 3: Confirm the umbrella sibling suite is still green**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem && bash scripts/eco.sh test`
  Expected: every sibling passes, exit code 0.

- [ ] **Step 4: Confirm the stdlib-only gate still passes (sanity — we never touched llm-agent/)**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem && bash scripts/stdlib-only-check.sh`
  Expected: `stdlib-only-check: PASS`.

- [ ] **Step 5: No commit — gate only.**

---

## Task 22: Update CHANGELOG for the M1 deliverable

**Files:**
- Modify: `llm-agent-memory/CHANGELOG.md`

- [ ] **Step 1: Promote the `[0.1.0]` entry from "scaffold only" to the full M0+M1 deliverable**

  Replace the existing `## [0.1.0] - 2026-05-26` body (created in Task 2) with the final language listing all three components. The exact text already lives in Task 2's draft and accurately describes what M1 ships — no further edits needed. If you added partial language during M1, replace with the Task 2 final-form text now.

- [ ] **Step 2: Sanity-check the CHANGELOG renders**

  Run: `cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory && head -40 CHANGELOG.md`
  Expected: the `## [0.1.0] - 2026-05-26` section listing `ScopedLifecycleManager`, `Consolidator`, and `UnifiedSearcher`.

- [ ] **Step 3: Commit**

  ```bash
  cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem
  git add llm-agent-memory/CHANGELOG.md
  git commit -m "docs(memory): finalize CHANGELOG entry for 0.1.0 (M0 + Phase A)"
  ```

---

## Self-Review

### Spec coverage

Roadmap M0 exit criteria (`docs/superpowers/plans/2026-05-25-llm-agent-memory-roadmap.md`, M0 row):

1. `llm-agent-memory/go.mod` with module path + Go 1.26.0 — Task 1.
2. Added to `go.work` `use` block — Task 4.
3. Stub `memory/` package with `doc.go` and a no-op `version_test.go` — Task 3.
4. Added to `.github/workflows/umbrella.yml` cross-repo-build matrix mirroring siblings — Task 6.
5. `Makefile` `TARGETS` and `scripts/eco.sh` updated to recognize the new sibling — Task 5 (Makefile is no-op because it dispatches via eco.sh; documented in the file-structure table).

Roadmap M1 exit criteria (M1 row, with `docs/memory-roadmap.zh-CN.md` §4.1 as source of truth):

1. `ConsolidateScoped` / `ForgetScoped` / `StatsScoped` honor non-zero scope; cross-scope mutation tests pass — Tasks 9, 10, 11, 12.
2. `Consolidate` writes `_consolidated_at` / `_promoted_from` / `_promotion_count` metadata; double-consolidate is no-op — Tasks 13, 14.
3. `SearchUnified(ctx, query, topK)` returns merged, deduped, sorted, topK-capped `[]SearchResult`; old `SearchAll` unchanged — Tasks 16, 17, 18, 19, 20.
4. Backwards-compat tests demonstrate old-API behavior unchanged — Task 20 (asserts `mgr.SearchAll` still works post-`SearchUnified`); the entire plan keeps `llm-agent/memory/` untouched by construction.
5. Snapshot import/export round-trips the new metadata keys — Task 15.

Every checkbox accounted for. No gap.

### Placeholder scan

- No `TODO`, `tbd`, `fill in`, `similar to above`, or `implement later` in any code block.
- Every Go file's full body is spelled out in the plan; no "remaining methods follow the same pattern" hand-waving.
- Every shell command uses an absolute path and shows expected output.

### Type consistency

- `ScopedLifecycleManager` — name identical across Tasks 9, 10, 11, 12.
- Methods on `ScopedLifecycleManager`: `ConsolidateScoped(ctx, opts)`, `ForgetScoped(ctx, kind, opts)`, `StatsScoped(ctx)` — consistent across all tasks.
- `Consolidator` with method `Consolidate(ctx, opts)` — consistent across Tasks 13, 14, 15.
- `UnifiedSearcher` with method `SearchUnified(ctx, query, topK)` — consistent across Tasks 16–20.
- Metadata constants `MetaKeyConsolidatedAt`, `MetaKeyPromotedFrom`, `MetaKeyPromotionCount` — defined exactly once (Task 13), referenced uniformly thereafter.
- Sentinel errors: `ErrScopedManagerRequired`, `ErrManagerRequired`, `ErrUnifiedManagerRequired` — three distinct names, no collisions, each defined exactly once.
- Core import alias `coremem` — used uniformly in every file (impl and test).

No drift detected.
