#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

modules=()
for repo in llm-agent llm-agent-rag llm-agent-otel llm-agent-providers llm-agent-customer-support llm-agent-flow llm-agent-memory llm-agent-memory-gateway llm-agent-memory-postgres; do
  if [ -f "$root_dir/$repo/go.mod" ]; then
    modules+=("./$repo")
  fi
done

if [ "${#modules[@]}" -ne 9 ]; then
  echo "scripts/workspace.sh: bootstrap all 9 subprojects first" >&2
  exit 1
fi

cd "$root_dir"
rm -f go.work go.work.sum
go work init "${modules[@]}"

echo "wrote $root_dir/go.work"
