#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

all_repos=(
  llm-agent
  llm-agent-rag
  llm-agent-otel
  llm-agent-providers
  llm-agent-customer-support
  llm-agent-flow
  llm-agent-memory-contract
  llm-agent-memory
  llm-agent-memory-gateway
  llm-agent-memory-postgres
  llm-agent-memory-worker
  llm-agent-memory-client
)

launchable_repos=(
  llm-agent-otel
  llm-agent-customer-support
)

repo_url() {
  case "$1" in
    llm-agent) printf '%s\n' 'https://github.com/costa92/llm-agent.git' ;;
    llm-agent-rag) printf '%s\n' 'https://github.com/costa92/llm-agent-rag.git' ;;
    llm-agent-otel) printf '%s\n' 'https://github.com/costa92/llm-agent-otel.git' ;;
    llm-agent-providers) printf '%s\n' 'https://github.com/costa92/llm-agent-providers.git' ;;
    llm-agent-customer-support) printf '%s\n' 'https://github.com/costa92/llm-agent-customer-support.git' ;;
    llm-agent-flow) printf '%s\n' 'https://github.com/costa92/llm-agent-flow.git' ;;
    llm-agent-memory) printf '%s\n' 'https://github.com/costa92/llm-agent-memory.git' ;;
    llm-agent-memory-gateway) printf '%s\n' 'https://github.com/costa92/llm-agent-memory-gateway.git' ;;
    llm-agent-memory-postgres) printf '%s\n' 'https://github.com/costa92/llm-agent-memory-postgres.git' ;;
    llm-agent-memory-contract) printf '%s\n' 'https://github.com/costa92/llm-agent-memory-contract.git' ;;
    llm-agent-memory-worker) printf '%s\n' 'https://github.com/costa92/llm-agent-memory-worker.git' ;;
    llm-agent-memory-client) printf '%s\n' 'https://github.com/costa92/llm-agent-memory-client.git' ;;
    *) printf '%s\n' "" ;;
  esac
}

is_launchable() {
  case "$1" in
    llm-agent-otel|llm-agent-customer-support) return 0 ;;
    *) return 1 ;;
  esac
}

normalize_targets() {
  local raw="${1:-all}"
  local -n items_ref="$2"
  if [ "$raw" = "all" ]; then
    printf '%s\n' "${items_ref[@]}"
    return 0
  fi

  IFS=',' read -r -a items <<<"$raw"
  printf '%s\n' "${items[@]}"
}

require_repo() {
  local repo="$1"
  if [ ! -d "$root_dir/$repo/.git" ]; then
    echo "missing subproject: $repo (run: make bootstrap)" >&2
    exit 1
  fi
}

bootstrap_repo() {
  local repo="$1"
  local url
  url="$(repo_url "$repo")"
  if [ -z "$url" ]; then
    echo "unknown repo: $repo" >&2
    exit 1
  fi
  if [ -d "$root_dir/$repo/.git" ]; then
    git -C "$root_dir/$repo" pull --ff-only
    return 0
  fi
  if [ -e "$root_dir/$repo" ]; then
    echo "path exists but is not a git repo: $root_dir/$repo" >&2
    exit 1
  fi
  git clone "$url" "$root_dir/$repo"
}

run_go_cmd() {
  local repo="$1"
  local cmd="$2"
  require_repo "$repo"
  (cd "$root_dir/$repo" && GOWORK=off bash -lc "$cmd")
}

run_compose() {
  local repo="$1"
  local action="$2"
  require_repo "$repo"

  case "$repo:$action" in
    llm-agent-customer-support:up)
      (cd "$root_dir/$repo" && \
        CS_APP_PORT=8080 \
        CS_GRAFANA_PORT=3000 \
        CS_OLLAMA_PORT=11434 \
        CS_OTEL_GRPC_PORT=4317 \
        CS_OTEL_HTTP_PORT=4318 \
        docker compose -f compose/compose.yaml up -d --build)
      ;;
    llm-agent-otel:up)
      (cd "$root_dir/$repo" && \
        OTEL_DEMO_GRAFANA_PORT=3001 \
        OTEL_DEMO_OTLP_GRPC_PORT=4319 \
        OTEL_DEMO_OTLP_HTTP_PORT=4320 \
        docker compose -f compose/compose.yaml up -d --build)
      ;;
    *:down)
      (cd "$root_dir/$repo" && docker compose -f compose/compose.yaml down --remove-orphans)
      ;;
    *)
      echo "unsupported compose action for $repo: $action" >&2
      exit 1
      ;;
  esac
}

prune_repo_branches() {
  local dir="$1"
  local label="$2"
  if [ ! -d "$dir/.git" ]; then
    printf '\n=== %s ===\n  skip (not cloned)\n' "$label"
    return 0
  fi
  printf '\n=== %s ===\n' "$label"

  # Drop stale remote-tracking refs (origin/<branch> whose remote was deleted,
  # e.g. after a merged PR with delete-branch-on-merge). Tolerate offline.
  if ! git -C "$dir" fetch --prune origin >/dev/null 2>&1; then
    echo "  warning: fetch --prune failed (offline?); cleaning local-only"
    git -C "$dir" remote prune origin >/dev/null 2>&1 || true
  fi

  # Resolve the default branch (main / master) from origin/HEAD; the merged
  # check runs against origin/<default> so a branch already merged on the
  # remote is detected even if the local default has not been pulled.
  local def merge_target
  def="$(git -C "$dir" symbolic-ref --short refs/remotes/origin/HEAD 2>/dev/null | sed 's@^origin/@@' || true)"
  if [ -z "$def" ]; then
    if git -C "$dir" rev-parse --verify --quiet origin/main >/dev/null 2>&1; then
      def="main"
    elif git -C "$dir" rev-parse --verify --quiet origin/master >/dev/null 2>&1; then
      def="master"
    else
      def="$(git -C "$dir" rev-parse --abbrev-ref HEAD 2>/dev/null || echo main)"
    fi
  fi
  if git -C "$dir" rev-parse --verify --quiet "origin/$def" >/dev/null 2>&1; then
    merge_target="origin/$def"
  else
    merge_target="$def"
  fi

  # Delete local branches whose upstream is gone — but only when fully merged
  # into the default branch (ancestor check), so local-only work is never lost.
  local deleted=0 kept=0
  while read -r br track; do
    [ -n "$br" ] || continue
    [ "$br" = "$def" ] && continue
    case "$track" in
      *gone*)
        if git -C "$dir" merge-base --is-ancestor "$br" "$merge_target" 2>/dev/null; then
          git -C "$dir" branch -D "$br" >/dev/null 2>&1 && { echo "  deleted (merged): $br"; deleted=$((deleted + 1)); }
        else
          echo "  KEPT (upstream gone but NOT merged into $def — has local-only commits): $br"
          kept=$((kept + 1))
        fi
        ;;
    esac
  done < <(git -C "$dir" for-each-ref --format '%(refname:short) %(upstream:track)' refs/heads)

  if [ "$deleted" -eq 0 ] && [ "$kept" -eq 0 ]; then
    echo "  nothing to prune"
  fi
}

command="${1:-help}"
shift || true

case "$command" in
  bootstrap)
    targets="$(normalize_targets "${1:-all}" all_repos)"
    while IFS= read -r repo; do
      [ -n "$repo" ] || continue
      bootstrap_repo "$repo"
    done <<<"$targets"
    ;;
  pull)
    targets="$(normalize_targets "${1:-all}" all_repos)"
    while IFS= read -r repo; do
      [ -n "$repo" ] || continue
      bootstrap_repo "$repo"
    done <<<"$targets"
    ;;
  status)
    targets="$(normalize_targets "${1:-all}" all_repos)"
    while IFS= read -r repo; do
      [ -n "$repo" ] || continue
      require_repo "$repo"
      printf '\n=== %s ===\n' "$repo"
      git -C "$root_dir/$repo" status --short --branch
    done <<<"$targets"
    ;;
  build)
    targets="$(normalize_targets "${1:-all}" all_repos)"
    while IFS= read -r repo; do
      [ -n "$repo" ] || continue
      run_go_cmd "$repo" 'go build ./...'
    done <<<"$targets"
    ;;
  test)
    targets="$(normalize_targets "${1:-all}" all_repos)"
    while IFS= read -r repo; do
      [ -n "$repo" ] || continue
      run_go_cmd "$repo" 'go test ./... -count=1'
    done <<<"$targets"
    ;;
  release-check)
    # Skew-honest pre-release gate: build + vet each module with GOWORK=off so
    # only that module's own go.mod (+ its replaces) resolve deps — the
    # workspace cannot mask a tagged-graph break. Mirrors the umbrella CI
    # cross-repo-build job locally. Tests are excluded (some modules need a
    # live Postgres); use `eco.sh test <repo>` for those.
    targets="$(normalize_targets "${1:-all}" all_repos)"
    while IFS= read -r repo; do
      [ -n "$repo" ] || continue
      run_go_cmd "$repo" 'go build ./... && go vet ./...'
    done <<<"$targets"
    ;;
  prune-branches)
    # Clean up local branches whose remote was deleted (e.g. after a merged PR
    # with delete-branch-on-merge) plus the stale remote-tracking refs they
    # leave behind. Safe by default: only branches fully merged into the
    # default branch are removed; branches with local-only commits are KEPT and
    # reported. Covers the umbrella repo and every cloned subproject.
    prune_repo_branches "$root_dir" "$(basename "$root_dir") (umbrella)"
    targets="$(normalize_targets "${1:-all}" all_repos)"
    while IFS= read -r repo; do
      [ -n "$repo" ] || continue
      prune_repo_branches "$root_dir/$repo" "$repo"
    done <<<"$targets"
    ;;
  up|down)
    targets="$(normalize_targets "${1:-all}" launchable_repos)"
    while IFS= read -r repo; do
      [ -n "$repo" ] || continue
      if ! is_launchable "$repo"; then
        echo "subproject is not launchable: $repo" >&2
        exit 1
      fi
      run_compose "$repo" "$command"
    done <<<"$targets"
    ;;
  help|--help|-h)
    cat <<'EOF'
ecosystem commands:
  bootstrap [all|repo1,repo2]
  pull [all|repo1,repo2]
  status [all|repo1,repo2]
  build [all|repo1,repo2]
  test [all|repo1,repo2]
  release-check [all|repo1,repo2]
  prune-branches [all|repo1,repo2]
  up [all|llm-agent-otel,llm-agent-customer-support]
  down [all|llm-agent-otel,llm-agent-customer-support]
EOF
    ;;
  *)
    echo "unknown command: $command" >&2
    exit 1
    ;;
esac
