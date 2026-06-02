#!/usr/bin/env bash
# scripts/promotion-policy-parity-check.sh — D3 single-source-of-truth gate.
#
# The promotion eligibility rule, the 0.7 importance threshold, and the
# dedupe-key construction live in ONE place: llm-agent-memory-contract
# (corememory.PromotionEligible / PromoteImportanceThreshold / DedupeKey).
#
# Background. There are TWO promotion triggers: the consolidation worker
# (async, outbox-driven — consolidation_publisher.go) and the gateway
# session-close path (synchronous — durable_session_closer.go). They used to
# carry byte-equivalent private copies of the rule. The danger was a
# dedupe-key divergence: the two paths would resolve the same content to
# DIFFERENT winners. M8 decision D3 sank the shared surface into the contract.
#
# This umbrella-level gate is the only place with both sibling trees checked
# out side by side, so it owns the assertion. Per consumer, fail-fast:
#   1. MUST call corememory.PromotionEligible AND corememory.DedupeKey.
#   2. MUST NOT re-inline a hardcoded importance threshold (Importance >= N)
#      or re-introduce a local promotion/dedupe helper.
#
# A failure means a maintainer re-inlined the rule in one consumer instead of
# using the contract — exactly the drift D3 exists to prevent. Resolution:
# call the contract helpers; if the rule itself must change, change it in
# llm-agent-memory-contract and bump both consumers in lockstep.
#
# Stdlib bash + grep. No go, no new dependency.
set -euo pipefail

ECOSYSTEM_ROOT=${ECOSYSTEM_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}
echo "ECOSYSTEM_ROOT=$ECOSYSTEM_ROOT"

WORKER="$ECOSYSTEM_ROOT/llm-agent-memory-worker/internal/service/consolidation_publisher.go"
GATEWAY="$ECOSYSTEM_ROOT/llm-agent-memory-gateway/internal/service/durable_session_closer.go"

fail=0

check_consumer() {
  local label="$1" file="$2" cfail=0
  if [[ ! -f "$file" ]]; then
    echo "FAIL [$label]: file not found: $file"
    fail=1
    return
  fi
  if ! grep -q 'corememory\.PromotionEligible(' "$file"; then
    echo "FAIL [$label]: does not call corememory.PromotionEligible — D3 bypassed"
    cfail=1
  fi
  if ! grep -q 'corememory\.DedupeKey(' "$file"; then
    echo "FAIL [$label]: does not call corememory.DedupeKey — D3 bypassed"
    cfail=1
  fi
  if grep -Eq 'Importance[[:space:]]*>=[[:space:]]*[0-9]' "$file"; then
    echo "FAIL [$label]: re-inlined a hardcoded importance threshold (use corememory.PromoteImportanceThreshold)"
    cfail=1
  fi
  if grep -Eq 'func[[:space:]]+(shouldPromote|[a-zA-Z]*[Dd]edupeKey|[a-zA-Z]*[Nn]ormalize[a-zA-Z]*DedupeContent)\b' "$file"; then
    echo "FAIL [$label]: re-introduced a local promotion/dedupe helper (sink to contract instead)"
    cfail=1
  fi
  if [[ $cfail -eq 0 ]]; then
    echo "OK [$label]: uses contract PromotionEligible + DedupeKey, no local copies"
  else
    fail=1
  fi
}

check_consumer "worker" "$WORKER"
check_consumer "gateway" "$GATEWAY"

if [[ $fail -ne 0 ]]; then
  echo ""
  echo "Promotion-policy parity gate FAILED (D3). Both the worker and the"
  echo "gateway session-close path must use the shared contract helpers so the"
  echo "two promotion triggers cannot drift onto different dedupe winners."
  echo "See docs/m8-working-memory-lifecycle-enablement.zh-CN.md §7 D3."
  exit 1
fi
echo "promotion-policy parity gate: PASS"
