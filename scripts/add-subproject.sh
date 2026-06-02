#!/usr/bin/env bash
# scripts/add-subproject.sh — create a new sibling repo and wire it into the
# umbrella. Idempotent: every step skips if already done, so it is safe to
# re-run (e.g. to retrofit a repo that was created before this script).
#
# What it automates (the mechanical, uniform parts):
#   A. provision the repo: gh repo create (public) → scaffold go.mod + skeleton
#      → 4 verbatim workflows + a self-only umbrella.yml → push → branch
#      protection (strict, required `go`+`governance`, enforce_admins).
#   B. register into umbrella tooling: go.work, .gitignore, scripts/eco.sh
#      (all_repos + repo_url, plus launchable_repos when --launchable),
#      cmd/depcheck/main.go (repoList).
#
# What it deliberately does NOT touch (needs human judgment — printed as a
# checklist instead): README roster/tree/dependency-graph prose, the umbrella
# umbrella.yml cross-build edges, and consumer require/replace wiring.
#
# Requires: bash, git, go, gh (authenticated), python3 — the same toolset the
# repo's CI workflows already assume.
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
owner="costa92"
tmpl_dir="$root_dir/scripts/templates/workflows"

usage() {
  cat <<EOF
usage: scripts/add-subproject.sh <name> [--launchable]

  <name>         repo name, e.g. llm-agent-foo (becomes github.com/$owner/<name>)
  --launchable   also register in eco.sh launchable_repos / is_launchable
                 (docker-compose up/down targets)

Run from anywhere; paths resolve against the umbrella root.
EOF
}

name=""
launchable=0
while [ $# -gt 0 ]; do
  case "$1" in
    --launchable) launchable=1 ;;
    -h|--help) usage; exit 0 ;;
    -*) echo "unknown flag: $1" >&2; usage >&2; exit 1 ;;
    *)
      if [ -z "$name" ]; then name="$1"
      else echo "unexpected argument: $1" >&2; exit 1; fi
      ;;
  esac
  shift
done
[ -n "$name" ] || { echo "error: repo name required" >&2; usage >&2; exit 1; }

case "$name" in
  llm-agent*) : ;;
  *) echo "warning: '$name' does not match the llm-agent* naming convention" >&2 ;;
esac

repo_dir="$root_dir/$name"
url="https://github.com/$owner/$name.git"
slug="$owner/$name"

log()  { printf '\n\033[1m==> %s\033[0m\n' "$*"; }
note() { printf '    %s\n' "$*"; }

# ---------------------------------------------------------------------------
# A. Provision the repo
# ---------------------------------------------------------------------------
log "A. Provisioning $slug"

if [ -d "$repo_dir/.git" ]; then
  note "local checkout exists — skipping create/clone"
elif gh repo view "$slug" >/dev/null 2>&1; then
  note "remote exists — cloning"
  git -C "$root_dir" clone "$url" "$name"
else
  note "creating public repo + cloning"
  (cd "$root_dir" && gh repo create "$slug" --public --clone)
fi

# Ensure we are on a 'main' branch (fresh clone of an empty repo has no HEAD).
if ! git -C "$repo_dir" rev-parse --abbrev-ref HEAD >/dev/null 2>&1 \
   || [ "$(git -C "$repo_dir" rev-parse --abbrev-ref HEAD 2>/dev/null)" = "HEAD" ]; then
  git -C "$repo_dir" checkout -b main 2>/dev/null || git -C "$repo_dir" switch -c main 2>/dev/null || true
fi

# Scaffold go.mod + a placeholder package so `go build/vet/test ./...` is green.
if [ ! -f "$repo_dir/go.mod" ]; then
  note "scaffolding go.mod"
  cat > "$repo_dir/go.mod" <<EOF
module github.com/$owner/$name

go 1.26.0
EOF
fi

if [ -z "$(find "$repo_dir" -maxdepth 2 -name '*.go' -not -path '*/.git/*' 2>/dev/null | head -1)" ]; then
  pkg="$(printf '%s' "$name" | sed 's/.*-//' | tr -cd '[:alnum:]')"
  [ -n "$pkg" ] || pkg="agent"
  note "scaffolding placeholder package '$pkg' (replace with your real layout)"
  cat > "$repo_dir/doc.go" <<EOF
// Package $pkg is the $name module — replace this placeholder with the
// real package layout.
package $pkg
EOF
fi

if [ ! -f "$repo_dir/README.md" ]; then
  note "scaffolding README.md"
  cat > "$repo_dir/README.md" <<EOF
# $name

Part of the [llm-agent ecosystem](https://github.com/$owner/llm-agent-ecosystem).

> Scaffolded by \`make add-subproject\`. Replace this placeholder with a real
> description, and the placeholder \`package\` with the module's actual layout.

## Development

\`\`\`bash
GOWORK=off go vet ./...
GOWORK=off go build ./...
GOWORK=off go test ./... -count=1
\`\`\`
EOF
fi

# Workflows: 4 copied verbatim, umbrella.yml generated self-only.
mkdir -p "$repo_dir/.github/workflows"
for wf in test pr-governance release-precheck delete-merged-branch; do
  if [ ! -f "$repo_dir/.github/workflows/$wf.yml" ]; then
    note "adding workflow: $wf.yml"
    cp "$tmpl_dir/$wf.yml" "$repo_dir/.github/workflows/$wf.yml"
  fi
done
if [ ! -f "$repo_dir/.github/workflows/umbrella.yml" ]; then
  note "generating workflow: umbrella.yml (self-only)"
  sed "s/__REPO__/$name/g" "$tmpl_dir/umbrella.yml.tmpl" > "$repo_dir/.github/workflows/umbrella.yml"
fi

# Commit + push the scaffold (no-op if nothing changed). MUST precede branch
# protection — enforce_admins blocks direct pushes to a protected main.
if [ -n "$(git -C "$repo_dir" status --porcelain)" ]; then
  note "committing scaffold"
  git -C "$repo_dir" add -A
  git -C "$repo_dir" commit -q -m "chore: scaffold $name (go.mod + workflows)"
fi
if git -C "$repo_dir" rev-parse HEAD >/dev/null 2>&1; then
  if ! git -C "$repo_dir" rev-parse --abbrev-ref --symbolic-full-name '@{u}' >/dev/null 2>&1; then
    note "pushing main -> origin"
    git -C "$repo_dir" push -u origin main
  elif [ -n "$(git -C "$repo_dir" log '@{u}..HEAD' --oneline 2>/dev/null)" ]; then
    note "pushing pending commits"
    git -C "$repo_dir" push
  fi
fi

# Branch protection — mirror llm-agent exactly. Idempotent PUT.
log "Applying branch protection to $slug@main (strict, go+governance, enforce_admins)"
if gh api -X PUT "repos/$slug/branches/main/protection" \
     -H "Accept: application/vnd.github+json" --input - >/dev/null 2>&1 <<'JSON'
{
  "required_status_checks": {"strict": true, "contexts": ["go", "governance"]},
  "enforce_admins": true,
  "required_pull_request_reviews": null,
  "restrictions": null
}
JSON
then
  note "protection applied"
else
  note "WARNING: could not apply branch protection (token scope? branch not pushed yet) — set it manually"
fi

# ---------------------------------------------------------------------------
# B. Register into umbrella tooling
# ---------------------------------------------------------------------------
log "B. Registering into umbrella tooling"

# .gitignore — append once.
if ! grep -qxF "/$name/" "$root_dir/.gitignore" 2>/dev/null; then
  printf '/%s/\n' "$name" >> "$root_dir/.gitignore"
  note ".gitignore: /$name/"
else
  note ".gitignore: already present"
fi

# go.work (use block), eco.sh (all_repos + repo_url [+ launchable]) and
# depcheck repoList. Line-based, presence-guarded, order-preserving edits via
# python3 — robust against awk/sed quoting hazards, and avoids `go work use`
# re-sorting the whole use() block.
python3 - "$name" "$url" "$launchable" "$root_dir/scripts/eco.sh" "$root_dir/cmd/depcheck/main.go" "$root_dir/go.work" <<'PY'
import os, sys

name, url, launchable, eco_path, dep_path, gowork_path = sys.argv[1:7]
launchable = launchable == "1"
changed = []

# ---- go.work (insert ./<name> before the use() block close; keep order) ----
if os.path.exists(gowork_path):
    gw = open(gowork_path).read().split("\n")
    gout, in_use = [], False
    have_gw = any(l.strip() == "./" + name for l in gw)
    for ln in gw:
        s = ln.strip()
        if in_use and s == ")":
            if not have_gw:
                gout.append("\t./" + name); changed.append("go.work:use")
            in_use = False
        if s.startswith("use ("):
            in_use = True
        gout.append(ln)
    if not have_gw:
        open(gowork_path, "w").write("\n".join(gout))

# ---- eco.sh ----
eco = open(eco_path).read().split("\n")
out, in_all, in_launch, in_islaunch = [], False, False, False
have_all = any(l.strip() == name for l in eco)
have_url = any(l.strip().startswith(name + ")") and "printf" in l for l in eco)
url_inserted = False
for ln in eco:
    s = ln.strip()
    # close all_repos block -> insert name before ')'
    if in_all and s == ")":
        if not have_all:
            out.append("  " + name); changed.append("eco.sh:all_repos")
        in_all = False
    if in_launch and s == ")":
        if launchable and not any(x.strip() == name for x in eco):
            out.append("  " + name); changed.append("eco.sh:launchable_repos")
        in_launch = False
    # repo_url fallthrough -> insert case before '*)'
    if s.startswith("*)") and "printf" in ln and not url_inserted:
        if not have_url:
            out.append("    %s) printf '%%s\\n' '%s' ;;" % (name, url))
            changed.append("eco.sh:repo_url")
        url_inserted = True
    # is_launchable fallthrough -> insert 'name) return 0 ;;' before '*) return 1'
    if launchable and in_islaunch and s.startswith("*) return 1"):
        if not any((name + ")") in x and "return 0" in x for x in eco):
            out.append("    %s) return 0 ;;" % name); changed.append("eco.sh:is_launchable")
        in_islaunch = False
    if s == "all_repos=(":
        in_all = True
    if s == "launchable_repos=(":
        in_launch = True
    if s.startswith("is_launchable()"):
        in_islaunch = True
    out.append(ln)
if changed:
    open(eco_path, "w").write("\n".join(out))

# ---- depcheck repoList ----
dep = open(dep_path).read().split("\n")
dout, in_list = [], False
have_dep = any(('"%s"' % name) in l for l in dep)
for ln in dep:
    s = ln.strip()
    if in_list and s == "}":
        if not have_dep:
            dout.append('\t"%s",' % name); changed.append("depcheck:repoList")
        in_list = False
    if "repoList = []string{" in ln:
        in_list = True
    dout.append(ln)
if not have_dep:
    open(dep_path, "w").write("\n".join(dout))

print("    edits: " + (", ".join(changed) if changed else "none (all already present)"))
PY

# ---------------------------------------------------------------------------
# Validate
# ---------------------------------------------------------------------------
log "Validating"
bash -n "$root_dir/scripts/eco.sh" && note "eco.sh: syntax OK"
( cd "$root_dir/cmd/depcheck" && GOWORK=off go build ./... ) && note "depcheck: builds OK"
for wf in "$repo_dir"/.github/workflows/*.yml; do
  python3 -c "import yaml,sys; yaml.safe_load(open(sys.argv[1]))" "$wf" \
    && note "yaml OK: $(basename "$wf")"
done

# ---------------------------------------------------------------------------
# C. Manual checklist (judgment-driven — NOT automated)
# ---------------------------------------------------------------------------
log "Manual follow-ups (judgment-driven — do these by hand)"
cat <<EOF
    [ ] README.md (umbrella): add the tree line, a roster-table row, and the
        dependency-graph edges for $name (needs its role + who depends on it).
    [ ] .github/workflows/umbrella.yml (umbrella): if $name becomes a build
        dependency, add its checkout (+ build) to cross-repo-build / smoke /
        the stdlib/parity gates as appropriate.
    [ ] $name/.github/workflows/umbrella.yml: add cross-build edges as
        consumer/dependency relationships appear (mirror a sibling).
    [ ] Consumer wiring: in any repo that will import $name, add the
        require (+ local replace for dev), then complete the lockstep
        tag → bump → drop-replace → push migration.
    [ ] Replace the placeholder package / README in $name with the real thing.
EOF

log "Done: $name created, scaffolded, protected, and registered."
