#!/usr/bin/env bash
#
# install-git-hooks.sh — install the replace-guard pre-commit hook into the
# umbrella and every sibling repo nested under it.
#
# Git hooks are NOT distributed by git, so each clone needs the hook installed
# locally. Run this once per machine (and again after the hook script changes).
#
# Usage:
#   scripts/install-git-hooks.sh            # umbrella + every nested git repo
#   scripts/install-git-hooks.sh <dir>...   # the named repo dir(s) only
#
# Idempotent. Repos are de-duplicated by git top-level, so non-repo subdirs
# (cmd/, docs/, scripts/) that resolve to the umbrella are not double-installed.
# An existing DIFFERENT pre-commit hook is backed up to pre-commit.local first.
set -euo pipefail

UMBRELLA=$(cd "$(dirname "$0")/.." && pwd)
SRC="$UMBRELLA/scripts/hooks/pre-commit"
[ -f "$SRC" ] || { echo "error: hook source not found: $SRC" >&2; exit 1; }

declare -A SEEN

install_one() {
  local repo="$1" tl gitdir hookdir dest
  tl=$(git -C "$repo" rev-parse --show-toplevel 2>/dev/null) || return 0   # not a git repo
  [ -n "${SEEN[$tl]:-}" ] && return 0                                       # already done this repo
  SEEN[$tl]=1
  gitdir=$(git -C "$tl" rev-parse --absolute-git-dir 2>/dev/null) || return 0
  hookdir="$gitdir/hooks"; mkdir -p "$hookdir"; dest="$hookdir/pre-commit"
  if [ -f "$dest" ] && ! cmp -s "$SRC" "$dest" && [ ! -f "$hookdir/pre-commit.local" ]; then
    mv "$dest" "$hookdir/pre-commit.local"
    echo "  ($(basename "$tl")) backed up existing pre-commit -> pre-commit.local"
  fi
  cp "$SRC" "$dest"; chmod +x "$dest"
  echo "installed replace-guard pre-commit -> $(basename "$tl")"
}

if [ "$#" -gt 0 ]; then
  for d in "$@"; do install_one "$d"; done
else
  install_one "$UMBRELLA"                                  # umbrella tracks cmd/depcheck/go.mod
  for d in "$UMBRELLA"/*/; do install_one "${d%/}"; done   # each nested sibling repo
fi
echo "done. (bypass for an intentional local-dev commit: git commit --no-verify)"
