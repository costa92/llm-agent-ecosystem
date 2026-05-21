#!/usr/bin/env bash
# scripts/stdlib-only-check.sh — B4 gate: core llm-agent stays stdlib-only.
#
# Three assertions, fail-fast:
#
#   1. llm-agent/go.mod direct `require` block contains ZERO entries.
#      The core module has no third-party runtime deps — not even the
#      ecosystem siblings. Any forward edge to llm-agent-rag (or anything
#      else) is a regression of P0-2 (the "RAG facade 名实不副" decision)
#      and exits non-zero.
#
#   2. The full transitive dep set of llm-agent/... (via `go list -deps`)
#      contains ONLY:
#        - stdlib packages (no dot in path, or vendor/ stdlib alias)
#        - github.com/costa92/llm-agent and its subpackages (self)
#      Anything else is a leak and exits non-zero.
#
#   3. The stdlib-clean sub-packages (policy/, budget/, agentstest/) must
#      have ZERO external deps — the rule mirrors Assertion 2 for these
#      sub-packages individually so they cannot regress independently.
#
# Resolution:
#   ECOSYSTEM_ROOT defaults to this script's parent directory's parent
#   (so `scripts/stdlib-only-check.sh` lives at `<root>/scripts/`).
#   Override by exporting ECOSYSTEM_ROOT explicitly.
#
# Stdlib-only bash + go + standard POSIX tools. No new dependency.
set -euo pipefail

ECOSYSTEM_ROOT=${ECOSYSTEM_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}
echo "ECOSYSTEM_ROOT=$ECOSYSTEM_ROOT"

CORE_DIR="$ECOSYSTEM_ROOT/llm-agent"
if [ ! -d "$CORE_DIR" ]; then
  echo "::error::llm-agent sibling not found at $CORE_DIR"
  exit 2
fi

fail=0

# ---------- Assertion 1: direct require count + identity ----------
echo "[1/3] checking $CORE_DIR/go.mod direct require block..."
GOMOD="$CORE_DIR/go.mod"
if [ ! -f "$GOMOD" ]; then
  echo "::error::missing $GOMOD"
  exit 2
fi

# Extract direct require lines (block + single-line forms). Strip
# comments, trim, skip blanks.
# A direct require is one NOT marked `// indirect`.
DIRECT=$(awk '
  BEGIN { inblock=0 }
  /^require *\(/ { inblock=1; next }
  inblock && /^\)/ { inblock=0; next }
  {
    line=$0
    # strip line comments
    cidx=index(line, "//")
    has_indirect=0
    if (cidx > 0) {
      if (index(substr(line, cidx), "indirect") > 0) has_indirect=1
      line=substr(line, 1, cidx-1)
    }
    sub(/^[ \t]+/, "", line)
    sub(/[ \t]+$/, "", line)
    if (line == "") next
    if (inblock) {
      if (!has_indirect) print line
      next
    }
    if (line ~ /^require /) {
      sub(/^require /, "", line)
      if (!has_indirect) print line
    }
  }
' "$GOMOD")

DIRECT_COUNT=$(printf '%s\n' "$DIRECT" | grep -c '^[^[:space:]]' || true)
echo "  direct require count: $DIRECT_COUNT"
echo "  direct require lines:"
printf '%s\n' "$DIRECT" | sed 's/^/    /'

if [ "$DIRECT_COUNT" -ne 0 ]; then
  echo "::error::expected ZERO direct requires in core go.mod (P0-2: no RAG back-edge), got $DIRECT_COUNT"
  fail=1
fi

# ---------- Assertion 2: go list -deps whitelist ----------
echo "[2/3] checking transitive deps of llm-agent/... via go list -deps..."
# Build the whitelist regex. The set is closed under the brief's policy:
#   - stdlib: no dot at all in the path (e.g. `context`, `encoding/json`)
#   - stdlib vendoring: `vendor/golang.org/x/...` and similar `vendor/...`
#   - stdlib internals like `crypto/internal/entropy/v1.0.0`
#   - in-ecosystem: github.com/costa92/llm-agent($|/) and
#     github.com/costa92/llm-agent-rag($|/)
#   - golang.org/x/... (rag-transitive)
DEPS_ALL=$(cd "$CORE_DIR" && GOWORK=off go list -deps ./...) || {
  echo "::error::go list -deps failed in $CORE_DIR (see above)"
  exit 2
}

# Filter out allowed entries. Anything left is a leak.
LEAKS=$(printf '%s\n' "$DEPS_ALL" | awk '
  {
    p=$0
    # stdlib-pure: no dot in the whole path
    if (index(p, ".") == 0) next
    # vendor/ prefix is stdlib internal vendoring
    if (p ~ /^vendor\//) next
    # crypto/internal pseudo-versioned (e.g. crypto/internal/entropy/v1.0.0)
    if (p ~ /^crypto\/internal\//) next
    # in-ecosystem self only — no sibling allowance after P0-2
    if (p == "github.com/costa92/llm-agent" || p ~ /^github\.com\/costa92\/llm-agent\//) next
    print p
  }
')

if [ -n "$LEAKS" ]; then
  echo "::error::non-stdlib leaks detected in llm-agent transitive deps:"
  printf '%s\n' "$LEAKS" | sed 's/^/    /'
  fail=1
else
  echo "  (no leaks)"
fi

# ---------- Assertion 3: pure sub-packages have ZERO external deps ----------
echo "[3/3] checking pure sub-packages (policy/, budget/, agentstest/)..."
for sub in policy budget agentstest; do
  if [ ! -d "$CORE_DIR/$sub" ]; then
    echo "::warning::$CORE_DIR/$sub not present — skipping"
    continue
  fi
  SUBDEPS=$(cd "$CORE_DIR" && GOWORK=off go list -deps ./"$sub") || {
    echo "::error::go list -deps ./$sub failed (see above)"
    fail=1
    continue
  }
  SUB_LEAKS=$(printf '%s\n' "$SUBDEPS" | awk '
    {
      p=$0
      if (index(p, ".") == 0) next
      if (p ~ /^vendor\//) next
      if (p ~ /^crypto\/internal\//) next
      # self-module subpackages are fine
      if (p == "github.com/costa92/llm-agent" || p ~ /^github\.com\/costa92\/llm-agent\//) next
      # NOTE: NO allowance for llm-agent-rag here — these subs must
      # NOT pull the back-edge. That is the entire point of the gate.
      print p
    }
  ')
  if [ -n "$SUB_LEAKS" ]; then
    echo "::error::sub-package $sub has third-party deps:"
    printf '%s\n' "$SUB_LEAKS" | sed 's/^/    /'
    fail=1
  else
    echo "  $sub: clean"
  fi
done

if [ $fail -ne 0 ]; then
  echo "stdlib-only-check: FAIL"
  exit 1
fi
echo "stdlib-only-check: PASS"
