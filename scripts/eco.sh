#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

all_repos=(
  llm-agent
  llm-agent-rag
  llm-agent-otel
  llm-agent-providers
  llm-agent-customer-support
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
  up [all|llm-agent-otel,llm-agent-customer-support]
  down [all|llm-agent-otel,llm-agent-customer-support]
EOF
    ;;
  *)
    echo "unknown command: $command" >&2
    exit 1
    ;;
esac
