# GitHub Workflows Design Guide

> Document version: 2026-05-22
> Code snapshot: 2026-05-22

This document describes the GitHub Actions workflows currently present across the 6 repositories in the `llm-agent-ecosystem`, how they are layered, which paths are primary versus fallback, and why a few repositories intentionally differ.

The goal is not to restate YAML line by line, but to answer four questions:

1. Which workflows exist today across the ecosystem.
2. Which workflows are the primary path versus fallback or historical residue.
3. How a PR actually moves from open to merge to branch cleanup.
4. Why a few repositories have different CI shapes.

## Scope

This document covers:

- `llm-agent`
- `llm-agent-rag`
- `llm-agent-flow`
- `llm-agent-providers`
- `llm-agent-otel`
- `llm-agent-customer-support`

Workflow locations:

- `llm-agent/.github/workflows/*`
- `llm-agent-rag/.github/workflows/*`
- `llm-agent-flow/.github/workflows/*`
- `llm-agent-providers/.github/workflows/*`
- `llm-agent-otel/.github/workflows/*`
- `llm-agent-customer-support/.github/workflows/*`

## Design Goals

The workflow design serves five goals at once:

1. Every repository must keep its own independent CI.
2. Owner-authored PRs must not get stuck behind GitHub's built-in required-review gate.
3. External PRs must still require a current-head review from `costa92`.
4. Release branches must not carry `replace` directives.
5. Changes in the core repo must be validated against downstream repositories.

## Workflow Inventory

### By Repository

| Repo | Default Branch | Workflows |
|---|---|---|
| `llm-agent` | `main` | `pr-governance`, `delete-merged-branch`, `release-precheck`, `test`, `umbrella` |
| `llm-agent-rag` | `master` | `pr-governance`, `delete-merged-branch`, `release-precheck`, `test` |
| `llm-agent-flow` | `main` | `pr-governance`, `delete-merged-branch`, `test` |
| `llm-agent-providers` | `main` | `pr-governance`, `delete-merged-branch`, `release-precheck`, `test`, `nightly-ollama-live` |
| `llm-agent-otel` | `main` | `pr-governance`, `delete-merged-branch`, `release-precheck`, `test` |
| `llm-agent-customer-support` | `main` | `pr-governance`, `delete-merged-branch`, `release-precheck`, `ci` |

### By Responsibility

| Category | Workflow | Responsibility | Current Role |
|---|---|---|---|
| PR governance | `pr-governance.yml` | author-sensitive merge gate, owner auto-merge, post-merge branch deletion | Primary |
| Repo CI | `test.yml` or `ci` | compile, test, format, packaging checks | Primary |
| Release gate | `release-precheck.yml` | reject `replace` directives on `release/**` | Primary |
| Cross-repo validation | `umbrella.yml` | validate `llm-agent` changes against downstream repos | Primary, core repo only |
| Nightly live validation | `nightly-ollama-live.yml` | real Ollama conformance in `providers` | Primary, repo-specific |
| Post-merge cleanup | `delete-merged-branch.yml` | delete merged branches after default-branch push | Fallback |

## Global Principles

### Governance and code execution are separated

`pr-governance.yml` uses `pull_request_target` to inspect PR metadata and review state. It does not check out PR code and does not execute untrusted logic from PR branches.

Its responsibilities are:

- decide whether `governance` passes
- request `costa92` review when needed
- enable auto-merge for owner PRs
- delete the same-repo head branch after merge

Implementation reference:

- `llm-agent/.github/workflows/pr-governance.yml:3-15`
- `llm-agent/.github/workflows/pr-governance.yml:17-67`
- `llm-agent/.github/workflows/pr-governance.yml:69-133`

### Each repository keeps independent CI

Every repository has its own CI so that basic correctness does not depend on umbrella-level orchestration. A single-repo PR must still be able to close its own validation loop locally and in GitHub.

### Cross-repo validation stays in the core repo

Only `llm-agent` has `umbrella.yml`, because core API changes are the ones that naturally fan out into the rest of the ecosystem. Running cross-repo validation everywhere would add significant CI cost without proportional value.

### `deleteBranchOnMerge` is not the only dependency

Repository-level `deleteBranchOnMerge = true` is still enabled, but it is no longer treated as the only branch-cleanup mechanism. The primary path is now the explicit delete step inside `pr-governance.yml`.

### `GOWORK=off` is the default CI constraint

All Go CI workflows except umbrella explicitly set `GOWORK=off` so local workspace behavior does not leak into CI.

Examples:

- `llm-agent/.github/workflows/test.yml:13-15`
- `llm-agent-providers/.github/workflows/test.yml:16-18`
- `llm-agent-customer-support/.github/workflows/test.yml:16-18`

## Primary PR Governance Path

### Triggers and permissions

`pr-governance.yml` consistently listens to:

- `pull_request_target`: `opened`, `reopened`, `synchronize`, `ready_for_review`
- `pull_request_review`: `submitted`, `dismissed`, `edited`

And consistently requires:

- `contents: write`
- `pull-requests: write`

That permission pair is required for owner auto-merge. Without `contents: write`, `gh pr merge --auto` fails with the `enablePullRequestAutoMerge` permission error.

Reference:

- `llm-agent/.github/workflows/pr-governance.yml:3-15`

### The `governance` job

The logic is:

```text
if draft:
  pass
elif author == costa92:
  pass
else:
  request review from costa92
  if latest costa92 APPROVED review matches current head SHA:
    pass
  else:
    fail
```

Important properties:

- external PRs require a review on the current head, not just any historical approval
- `COMMENTED` does not count as approval
- reviewer request routing is tolerant of repeated calls

Reference:

- `llm-agent/.github/workflows/pr-governance.yml:21-67`

### The `auto-merge-owner` job

This job only acts on owner PRs and has four stages:

1. Skip drafts.
2. Skip non-owner PRs.
3. Enable `gh pr merge --auto --merge` if auto-merge is not already enabled.
4. Poll until the PR is visibly merged, then delete the same-repo head branch.

Important details:

- it checks `autoMergeRequest != null` first for idempotency
- it no longer treats `--delete-branch` as the primary mechanism
- it explicitly skips fork branches and the default branch

Reference:

- `llm-agent/.github/workflows/pr-governance.yml:69-133`

### Why branch deletion lives inside `pr-governance.yml`

The final design converged on "the workflow that enables owner auto-merge is also responsible for post-merge branch deletion".

Primary path:

```text
owner PR opened
  -> governance passes
  -> auto-merge-owner enables auto-merge
  -> PR merges
  -> same workflow polls MERGED
  -> same workflow deletes same-repo head branch
```

Not the primary path anymore:

```text
PR merges
  -> rely on a separate downstream cleanup workflow
  -> hope it runs reliably afterward
```

## Fallback Branch Cleanup

All six repositories still keep `delete-merged-branch.yml`.

It runs on default-branch `push` and:

1. finds the PR associated with the merge commit
2. identifies the same-repo head branch
3. deletes the branch if it still exists

Reference:

- `llm-agent/.github/workflows/delete-merged-branch.yml:3-15`
- `llm-agent/.github/workflows/delete-merged-branch.yml:17-82`

Its current role is fallback, not primary:

- if `pr-governance.yml` already deleted the branch, it sees 404 and exits cleanly
- if the primary path misses the merge visibility window, this workflow may still clean up later

## Per-Repository CI Design

### `llm-agent`

The core repo CI adds two drift checks:

- top-level `go mod tidy`
- `examples/` `go mod tidy`

Then it runs:

- `go vet`
- `go build`
- `go test`
- `examples` vet and build

Reference:

- `llm-agent/.github/workflows/test.yml:16-60`

### `llm-agent-rag`

`rag` has two repo-specific constraints:

1. core packages must not import `github.com/costa92/llm-agent`; only `adapter/` may do that
2. CI must cover the `llmagent` build tag for the adapter layer

It also surfaces the API snapshot gate explicitly.

Reference:

- `llm-agent-rag/.github/workflows/test.yml:16-57`

### `llm-agent-flow`, `llm-agent-providers`, `llm-agent-otel`

These three currently use a single-job test workflow:

- `go mod tidy` drift check
- `go vet`
- `go build`
- `go test`

Reference:

- `llm-agent-flow/.github/workflows/test.yml:16-41`
- `llm-agent-providers/.github/workflows/test.yml:19-44`
- `llm-agent-otel/.github/workflows/test.yml:19-44`

### `llm-agent-customer-support`

This repo has the heaviest CI shape, split into four jobs:

- `format`
- `go`
- `compose`
- `docker`

Dependencies:

- `go` depends on `format`
- `compose` depends on `format`
- `docker` depends on `go`

Reference:

- `llm-agent-customer-support/.github/workflows/test.yml:19-90`

## Release Precheck

`release-precheck.yml` currently exists in:

- `llm-agent`
- `llm-agent-rag`
- `llm-agent-providers`
- `llm-agent-otel`
- `llm-agent-customer-support`

`llm-agent-flow` does not currently carry it.

The logic is stable across repos:

- trigger on `push` and `pull_request` for `release/**`
- parse `go mod edit -json`
- fail if any `Replace` entries exist

Reference:

- `llm-agent/.github/workflows/release-precheck.yml:6-36`
- `llm-agent-providers/.github/workflows/release-precheck.yml:6-36`

## `llm-agent`-Only Umbrella Validation

`umbrella.yml` exists only in `llm-agent` and performs ecosystem validation:

- checks out the current `llm-agent` PR
- checks out downstream repos
- verifies `scripts/workspace.sh` byte identity across the core set
- runs the dependency currency gate
- creates a temporary `go.work`
- builds downstream repos against the PR version of `llm-agent`

Reference:

- `llm-agent/.github/workflows/umbrella.yml:14-122`

Current limitation:

- it does not yet include `llm-agent-flow` in the cross-repo build matrix

## `llm-agent-providers` Nightly Live Validation

`nightly-ollama-live.yml` is the only scheduled live-environment workflow.

Triggers:

- `schedule`
- `workflow_dispatch`

It intentionally stays out of PR CI because it is slower, Docker-backed, and better suited to periodic health verification than per-PR gating.

Reference:

- `llm-agent-providers/.github/workflows/nightly-ollama-live.yml:5-47`

## End-to-End PR State Machines

### Owner PR

```text
owner opens PR
  -> repo CI starts
  -> governance passes immediately
  -> auto-merge-owner enables auto-merge
  -> required checks pass
  -> GitHub merges PR
  -> same governance workflow polls MERGED
  -> same governance workflow deletes same-repo head branch
  -> fallback cleanup may still run later on default-branch push
```

### External PR

```text
external contributor opens PR
  -> repo CI starts
  -> governance requests costa92 review
  -> governance fails until costa92 approves current head
  -> after current-head approval, governance passes
  -> merge remains manual
  -> after merge, repo setting or fallback cleanup may delete the branch
```

## Why duplicate YAML still exists

The repositories still keep separate workflow files instead of moving everything into reusable workflows because:

1. the current priority is stable behavior, not YAML deduplication
2. CI behavior is not actually identical across all repos
3. keeping copies repo-local still makes rollback and repo-specific edits simpler

Today the important property is behavioral consistency, not file deduplication.

## Known Limitations

1. `delete-merged-branch.yml` is still present everywhere even though it is now fallback-only.
2. `umbrella.yml` does not yet include `llm-agent-flow`.
3. `llm-agent-customer-support` still only requires `go + governance` at the branch-protection layer even though its CI also produces `format`, `compose`, and `docker`.
4. the merged-state polling window in `pr-governance.yml` is finite
5. there is no reusable workflow layer yet, so cross-repo workflow changes still require multi-repo edits

## Recommended Ops Checklist

1. Confirm required checks still include `go` and `governance`.
2. Confirm `allow_auto_merge = true` on each repo.
3. Confirm `deleteBranchOnMerge = true` on each repo.
4. Confirm `pr-governance.yml` still has `contents: write` and `pull-requests: write`.
5. Confirm owner PRs still auto-merge and delete same-repo branches.
6. Confirm external PRs still request `costa92` review and require current-head approval.
7. Confirm `llm-agent` umbrella validation still builds downstream repos.
8. Confirm `release/**` still rejects `replace` directives.

## Required Submission Flow

From this point on, future code changes should follow this workflow by default rather than bypassing it:

1. Create a feature or fix branch from the repository's default branch.
2. Commit on that branch instead of pushing directly to the default branch.
3. Open a PR and let repo CI plus `pr-governance.yml` run automatically.
4. Wait until the required checks at least include:
   - `go`
   - `governance`
5. If the PR author is `costa92`:
   - `pr-governance.yml` enables auto-merge
   - the PR merges automatically after checks pass
   - the workflow deletes the same-repo branch after merge
6. If the PR author is not `costa92`:
   - review is requested from `costa92`
   - a current-head approval from `costa92` is required before merge
   - the merge remains manual

Operationally, this means:

- the default branch is no longer the normal direct-push entrypoint
- PRs are the standard change entrypoint
- `governance` is the author-sensitive merge gate
- owner auto-merge is the standard path, not a special exception

### What this means for protected repos

For repositories with default-branch protection enabled, this is not just a recommendation. It is the only stable path.

Direct pushes such as:

- `git push origin main`
- `git push origin master`

should be expected to fail when protection and required checks are working correctly.

### What this means for repos not yet fully aligned

Even where GitHub-side settings are not yet fully aligned, future submissions should still follow the same PR-based process rather than treating current direct-push capability as the long-term model.

The workflow files, repository settings, and contributor behavior should converge on the same rule:

- all code changes land through PRs
- default-branch merges are gated by `go + governance`
- owner PRs auto-merge
- external PRs require owner review

## One-Line Summary

The ecosystem's GitHub workflow design has three layers:

- per-repo CI for local correctness
- `pr-governance.yml` for author-sensitive merge control and owner-branch deletion
- `umbrella.yml` in the core repo for cross-repo compatibility validation

## Further Reading

- [`../llm-agent/docs/PR-GOVERNANCE-OVERVIEW.md`](../llm-agent/docs/PR-GOVERNANCE-OVERVIEW.md)
- [`../llm-agent/docs/PR-GOVERNANCE-RULES.md`](../llm-agent/docs/PR-GOVERNANCE-RULES.md)
- [`../llm-agent/docs/PR-GOVERNANCE-OPERATIONS.md`](../llm-agent/docs/PR-GOVERNANCE-OPERATIONS.md)
- [`./source-design-umbrella-root.zh-CN.md`](./source-design-umbrella-root.zh-CN.md)
