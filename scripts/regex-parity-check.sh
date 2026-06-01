#!/usr/bin/env bash
# scripts/regex-parity-check.sh — KC-3 drift gate: the PII + injection
# regex tables that are COPIED (not imported) between the core and rag
# repos must not silently diverge.
#
# Background. llm-agent/policy/patterns.go carries a verbatim copy of the
# regex source strings from llm-agent-rag/guard/{redact,inject}.go. The
# copy is intentional (KC-3 per-repo source-of-truth; KS-5 keeps rag a
# frozen fixed point — no shared upstream module). The cost of "copy, not
# import" is that the two halves can drift with nobody noticing — neither
# repo's own CI can see the other. This umbrella-level gate is the only
# place that has both trees checked out side by side, so it owns the
# parity assertion.
#
# Two assertions, fail-fast:
#
#   1. The 4 injection patterns (instruction_override, disregard_above,
#      role_override, prompt_exfiltration) must match BYTE-FOR-BYTE
#      between core policy and rag guard. rag is the source of truth.
#
#   2. The 3 SHARED PII patterns (email, phone, ipv4) must match
#      BYTE-FOR-BYTE. rag additionally ships ssn + credit_card; core
#      deliberately drops them (Q2 ratification — see patterns.go). The
#      gate therefore checks only the 3 shared keys and does NOT require
#      core to carry ssn/credit_card.
#
# A failure here means a maintainer edited one side's regex without
# mirroring the other (exactly the drift KC-3's citation block warns
# about). Resolution: re-sync the two source strings — rag wins.
#
# Resolution of paths:
#   ECOSYSTEM_ROOT defaults to this script's parent directory's parent
#   (so `scripts/regex-parity-check.sh` lives at `<root>/scripts/`).
#   Override by exporting ECOSYSTEM_ROOT explicitly.
#
# Stdlib-only bash + awk. No go, no new dependency.
set -euo pipefail

ECOSYSTEM_ROOT=${ECOSYSTEM_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}
echo "ECOSYSTEM_ROOT=$ECOSYSTEM_ROOT"

CORE_FILE="$ECOSYSTEM_ROOT/llm-agent/policy/patterns.go"
RAG_INJECT="$ECOSYSTEM_ROOT/llm-agent-rag/guard/inject.go"
RAG_REDACT="$ECOSYSTEM_ROOT/llm-agent-rag/guard/redact.go"

for f in "$CORE_FILE" "$RAG_INJECT" "$RAG_REDACT"; do
  if [ ! -f "$f" ]; then
    echo "::error::required source not found: $f"
    exit 2
  fi
done

# extract_body <file> <rule-name> — print the regexp.MustCompile raw-string
# body (the text between backticks) of the rule whose name/kind field
# equals <rule-name>. Empty output means "no such rule in that file".
#
# Both files use the same shape: a `name:`/`Name:` (injection) or
# `kind:`/`Kind:` (PII) line carrying a double-quoted identifier, followed
# by a `pattern:`/`Pattern:` line whose regexp.MustCompile argument is a
# backtick-delimited raw string. Go raw strings cannot contain a backtick,
# so "first backtick .. next backtick" is an exact, unambiguous extraction.
extract_body() {
  local file="$1" name="$2"
  awk -v want="$name" '
    match($0, /(name|Name|kind|Kind):[ \t]*"[^"]*"/) {
      s = substr($0, RSTART, RLENGTH)
      match(s, /"[^"]*"/)
      cur = substr(s, RSTART + 1, RLENGTH - 2)
      next
    }
    /regexp\.MustCompile\(/ {
      f = index($0, "`")
      if (f > 0 && cur == want) {
        rest = substr($0, f + 1)
        l = index(rest, "`")
        print substr(rest, 1, l - 1)
        exit
      }
    }
  ' "$file"
}

# assert_parity <label> <rule-name> <rag-file>
#   Compare core policy's copy of <rule-name> against rag's source.
fail=0
assert_parity() {
  local label="$1" name="$2" ragfile="$3"
  local core rag
  core=$(extract_body "$CORE_FILE" "$name")
  rag=$(extract_body "$ragfile" "$name")

  if [ -z "$rag" ]; then
    echo "::error::[$label] rule '$name' not found in rag source $ragfile"
    fail=1
    return
  fi
  if [ -z "$core" ]; then
    echo "::error::[$label] rule '$name' missing from core policy/patterns.go (expected a verbatim copy of rag)"
    fail=1
    return
  fi
  if [ "$core" != "$rag" ]; then
    echo "::error::[$label] regex drift on rule '$name' — core and rag diverged:"
    echo "    core: $core"
    echo "    rag : $rag"
    echo "    fix : re-sync the two source strings (rag is the source of truth; see patterns.go citation block)"
    fail=1
    return
  fi
  echo "  $label/$name: in sync"
}

echo "[1/2] injection patterns (core policy <-> rag guard/inject.go)..."
for name in instruction_override disregard_above role_override prompt_exfiltration; do
  assert_parity injection "$name" "$RAG_INJECT"
done

echo "[2/2] shared PII patterns (core policy <-> rag guard/redact.go)..."
# Only the 3 language-agnostic patterns are shared. rag's ssn + credit_card
# are intentionally NOT mirrored into core (Q2 drop) and are not checked.
for name in email phone ipv4; do
  assert_parity pii "$name" "$RAG_REDACT"
done

if [ $fail -ne 0 ]; then
  echo "regex-parity-check: FAIL"
  exit 1
fi
echo "regex-parity-check: PASS"
