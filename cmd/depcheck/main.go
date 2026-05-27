// Command depcheck scans the umbrella's sibling repos, builds a
// dependency DAG of github.com/costa92/* pins, and emits a topological
// cascade order (leaves first) along with staleness markers.
//
// Stdlib-only. Run from the umbrella root or from inside cmd/depcheck.
//
//	cd cmd/depcheck && go run .          # human-readable table
//	cd cmd/depcheck && go run . --json   # JSON for CI artifacts
//
// Exit codes:
//   - 0: all pins current (or cannot determine — informational mode)
//   - 1: at least one stale pin detected
//
// The tool DOES NOT hit the network. Latest tags come from local
// `git tag` output of each sibling clone. Pinned versions come from
// the sibling's go.mod direct `require` block.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// repoList is the canonical sibling roster. Edit here to add/remove a
// repo — every downstream behavior (DAG, topo, table) keys off this.
var repoList = []string{
	"llm-agent",
	"llm-agent-rag",
	"llm-agent-otel",
	"llm-agent-providers",
	"llm-agent-customer-support",
	"llm-agent-flow",
	"llm-agent-memory",
	"llm-agent-memory-gateway",
	"llm-agent-memory-postgres",
}

// modulePrefix is the org path. Any require line under this prefix is
// treated as an in-ecosystem edge in the DAG.
const modulePrefix = "github.com/costa92/"

// require is one parsed `require` entry from a go.mod.
type require struct {
	Path    string `json:"path"`
	Version string `json:"version"`
}

// repoInfo is the full per-repo snapshot used by both the table and
// the DAG.
type repoInfo struct {
	Name      string    `json:"name"`
	Module    string    `json:"module"`
	LatestTag string    `json:"latest_tag"`
	Requires  []require `json:"requires"` // only in-ecosystem edges
}

// report is the JSON output shape.
type report struct {
	CascadeOrder []string   `json:"cascade_order"`
	Repos        []repoInfo `json:"repos"`
	Stale        []string   `json:"stale"` // human-readable lines
}

// parseGoMod extracts the module name and every github.com/costa92/*
// require entry from a go.mod's text. Both single-line and
// `require (...)` block forms are handled. `// indirect` is preserved
// as-is for in-ecosystem deps because we still want them in the DAG.
//
// Pure string -> data; safe to call in tests with hand-crafted fixtures.
func parseGoMod(text string) (module string, reqs []require) {
	lines := strings.Split(text, "\n")
	inBlock := false
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		// strip line comments first so `// indirect` doesn't confuse parsing
		if idx := strings.Index(line, "//"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "module ") {
			module = strings.TrimSpace(strings.TrimPrefix(line, "module"))
			continue
		}
		if strings.HasPrefix(line, "require (") {
			inBlock = true
			continue
		}
		if inBlock && line == ")" {
			inBlock = false
			continue
		}
		if inBlock {
			if r, ok := parseRequireLine(line); ok {
				reqs = append(reqs, r)
			}
			continue
		}
		// single-line require
		if strings.HasPrefix(line, "require ") {
			rest := strings.TrimSpace(strings.TrimPrefix(line, "require"))
			if r, ok := parseRequireLine(rest); ok {
				reqs = append(reqs, r)
			}
		}
	}
	return module, reqs
}

// parseRequireLine parses a single `path version` token pair into a
// require. Returns ok=false for lines we don't care about (non-org).
func parseRequireLine(line string) (require, bool) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return require{}, false
	}
	path, ver := fields[0], fields[1]
	if !strings.HasPrefix(path, modulePrefix) {
		return require{}, false
	}
	return require{Path: path, Version: ver}, true
}

// repoFromModulePath maps a full module path to its sibling repo name.
// `github.com/costa92/llm-agent-rag/v2` -> `llm-agent-rag` (we treat
// /vN suffixes as the same repo for DAG purposes — they share a tag
// stream from the operator's POV).
func repoFromModulePath(p string) string {
	if !strings.HasPrefix(p, modulePrefix) {
		return ""
	}
	rest := strings.TrimPrefix(p, modulePrefix)
	// strip /vN+ suffix
	if i := strings.Index(rest, "/"); i >= 0 {
		rest = rest[:i]
	}
	return rest
}

// buildDAG flattens the per-repo require lists into edges
// `repo -> requiredRepo`. Only edges between members of `roster` are
// kept; everything else is dropped silently.
//
// The result is keyed by repo name with stable, deduped, sorted edge lists.
func buildDAG(infos []repoInfo, roster []string) map[string][]string {
	want := make(map[string]bool, len(roster))
	for _, r := range roster {
		want[r] = true
	}
	out := make(map[string][]string, len(roster))
	for _, r := range roster {
		out[r] = nil
	}
	for _, info := range infos {
		if !want[info.Name] {
			continue
		}
		seen := make(map[string]bool)
		for _, req := range info.Requires {
			target := repoFromModulePath(req.Path)
			if target == "" || !want[target] || target == info.Name || seen[target] {
				continue
			}
			seen[target] = true
			out[info.Name] = append(out[info.Name], target)
		}
		sort.Strings(out[info.Name])
	}
	return out
}

// topoSort returns repos in cascade order: leaves first (no
// in-ecosystem deps), then dependents. Modified Kahn's algorithm:
// when no zero-degree nodes remain but unvisited nodes exist (i.e. a
// cycle), emit the unvisited node with the fewest outstanding deps,
// breaking ties alphabetically. This handles the canonical back-edge
// (llm-agent <-> llm-agent-rag) gracefully — rag, despite having a
// requires-llm-agent back-edge, still emits first because it has the
// lowest cycle out-degree.
func topoSort(dag map[string][]string) (order []string, cycles []string) {
	// outDegree[n] = how many in-ecosystem deps n still needs satisfied.
	outDegree := make(map[string]int, len(dag))
	// reverse[r] = list of nodes that depend on r (so when r emits,
	// their outDegree drops).
	reverse := make(map[string][]string, len(dag))
	for n, deps := range dag {
		outDegree[n] = len(deps)
		for _, d := range deps {
			reverse[d] = append(reverse[d], n)
		}
	}

	emitted := make(map[string]bool, len(dag))
	cyclesSet := make(map[string]bool)

	emit := func(n string) {
		emitted[n] = true
		order = append(order, n)
		dependents := append([]string(nil), reverse[n]...)
		sort.Strings(dependents)
		for _, dep := range dependents {
			outDegree[dep]--
		}
	}

	for len(emitted) < len(dag) {
		// Frontier: every unemitted node with degree 0.
		var frontier []string
		for n := range dag {
			if !emitted[n] && outDegree[n] <= 0 {
				frontier = append(frontier, n)
			}
		}
		if len(frontier) > 0 {
			sort.Strings(frontier)
			emit(frontier[0])
			continue
		}
		// Cycle: pick unemitted node with smallest residual degree,
		// alphabetical tie-break. Mark it as part of a cycle for
		// audit visibility.
		var candidates []string
		for n := range dag {
			if !emitted[n] {
				candidates = append(candidates, n)
			}
		}
		if len(candidates) == 0 {
			break
		}
		sort.Slice(candidates, func(i, j int) bool {
			a, b := candidates[i], candidates[j]
			if outDegree[a] != outDegree[b] {
				return outDegree[a] < outDegree[b]
			}
			return a < b
		})
		pick := candidates[0]
		cyclesSet[pick] = true
		emit(pick)
	}

	for n := range cyclesSet {
		cycles = append(cycles, n)
	}
	sort.Strings(cycles)
	return order, cycles
}

// loadRepoInfo reads `<root>/<repo>/go.mod` and resolves the latest
// local git tag for the repo. Errors are non-fatal — repos that can't
// be read get an empty record so the cascade order still computes.
func loadRepoInfo(root, name string) repoInfo {
	info := repoInfo{Name: name}
	gomod := filepath.Join(root, name, "go.mod")
	if data, err := os.ReadFile(gomod); err == nil {
		mod, reqs := parseGoMod(string(data))
		info.Module = mod
		info.Requires = reqs
	}
	info.LatestTag = latestLocalTag(filepath.Join(root, name))
	return info
}

// latestLocalTag runs `git tag --sort=-v:refname` and picks the
// first vX.Y.Z tag. Returns empty string when no tags or git fails.
func latestLocalTag(repoDir string) string {
	cmd := exec.Command("git", "tag", "--sort=-v:refname")
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if isSemverTag(line) {
			return line
		}
	}
	return ""
}

// isSemverTag is a cheap vX.Y.Z matcher — avoids regexp dep.
func isSemverTag(s string) bool {
	if !strings.HasPrefix(s, "v") {
		return false
	}
	rest := s[1:]
	parts := strings.Split(rest, ".")
	if len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}

// detectStale compares each repo's pinned versions against the latest
// tag of the target repo. Returns one human-readable line per stale
// edge. Empty result == nothing stale.
func detectStale(infos []repoInfo) []string {
	latestByRepo := make(map[string]string, len(infos))
	for _, info := range infos {
		latestByRepo[info.Name] = info.LatestTag
	}
	var stale []string
	for _, info := range infos {
		for _, req := range info.Requires {
			target := repoFromModulePath(req.Path)
			latest := latestByRepo[target]
			if latest == "" || req.Version == "" {
				continue
			}
			if req.Version != latest {
				stale = append(stale, fmt.Sprintf("%s pins %s@%s (latest %s)",
					info.Name, target, req.Version, latest))
			}
		}
	}
	sort.Strings(stale)
	return stale
}

// findUmbrellaRoot walks up from cwd until it finds a directory
// containing the canonical umbrella markers (README.md + Makefile +
// at least one sibling clone). Falls back to "..".
func findUmbrellaRoot() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ".."
	}
	dir := cwd
	for i := 0; i < 5; i++ {
		if isUmbrellaRoot(dir) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ".."
}

func isUmbrellaRoot(dir string) bool {
	for _, want := range []string{"README.md", "Makefile"} {
		if _, err := os.Stat(filepath.Join(dir, want)); err != nil {
			return false
		}
	}
	// must have at least one sibling clone
	for _, r := range repoList {
		if st, err := os.Stat(filepath.Join(dir, r)); err == nil && st.IsDir() {
			return true
		}
	}
	return false
}

func renderTable(w *strings.Builder, order []string, infos []repoInfo, stale []string) {
	byName := make(map[string]repoInfo, len(infos))
	for _, info := range infos {
		byName[info.Name] = info
	}
	fmt.Fprintln(w, "CASCADE ORDER (bump leaves first):")
	for i, n := range order {
		info := byName[n]
		latest := info.LatestTag
		if latest == "" {
			latest = "n/a"
		}
		var pins []string
		for _, req := range info.Requires {
			target := repoFromModulePath(req.Path)
			pins = append(pins, fmt.Sprintf("%s@%s", target, req.Version))
		}
		pinStr := strings.Join(pins, ", ")
		if pinStr == "" {
			pinStr = "(no in-ecosystem deps)"
		}
		fmt.Fprintf(w, "  %d. %-30s  latest:%-8s  pins: %s\n", i+1, n, latest, pinStr)
	}
	if len(stale) > 0 {
		fmt.Fprintln(w, "\nSTALE PINS:")
		for _, s := range stale {
			fmt.Fprintln(w, "  - "+s)
		}
	} else {
		fmt.Fprintln(w, "\nAll pins current.")
	}
}

func main() {
	jsonOut := flag.Bool("json", false, "emit JSON instead of a human table")
	rootFlag := flag.String("root", "", "umbrella root (defaults to auto-detect)")
	flag.Parse()

	root := *rootFlag
	if root == "" {
		root = findUmbrellaRoot()
	}

	var infos []repoInfo
	for _, name := range repoList {
		infos = append(infos, loadRepoInfo(root, name))
	}
	dag := buildDAG(infos, repoList)
	order, _ := topoSort(dag)
	stale := detectStale(infos)

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(report{CascadeOrder: order, Repos: infos, Stale: stale})
	} else {
		var b strings.Builder
		renderTable(&b, order, infos, stale)
		fmt.Print(b.String())
	}

	if len(stale) > 0 {
		os.Exit(1)
	}
}
