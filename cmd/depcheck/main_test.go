package main

import (
	"reflect"
	"testing"
)

func TestParseGoMod(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantModule  string
		wantRequire []require
	}{
		{
			name: "single-line require",
			input: `module github.com/costa92/llm-agent

go 1.26.0

require github.com/costa92/llm-agent-rag v1.0.1
`,
			wantModule:  "github.com/costa92/llm-agent",
			wantRequire: []require{{Path: "github.com/costa92/llm-agent-rag", Version: "v1.0.1"}},
		},
		{
			name: "require block with org + non-org",
			input: `module github.com/costa92/llm-agent-flow

go 1.26.0

require (
	github.com/costa92/llm-agent v0.5.1
	github.com/google/cel-go v0.28.1
)

require (
	github.com/costa92/llm-agent-rag v1.0.1 // indirect
	golang.org/x/sys v0.42.0 // indirect
)
`,
			wantModule: "github.com/costa92/llm-agent-flow",
			wantRequire: []require{
				{Path: "github.com/costa92/llm-agent", Version: "v0.5.1"},
				{Path: "github.com/costa92/llm-agent-rag", Version: "v1.0.1"},
			},
		},
		{
			name: "comment-only line is ignored",
			input: `module github.com/costa92/llm-agent

// this is a comment
go 1.26.0
`,
			wantModule:  "github.com/costa92/llm-agent",
			wantRequire: nil,
		},
		{
			name:        "empty input",
			input:       "",
			wantModule:  "",
			wantRequire: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mod, reqs := parseGoMod(tc.input)
			if mod != tc.wantModule {
				t.Errorf("module: got %q want %q", mod, tc.wantModule)
			}
			if !reflect.DeepEqual(reqs, tc.wantRequire) {
				t.Errorf("requires:\n got: %#v\nwant: %#v", reqs, tc.wantRequire)
			}
		})
	}
}

func TestRepoFromModulePath(t *testing.T) {
	cases := map[string]string{
		"github.com/costa92/llm-agent":          "llm-agent",
		"github.com/costa92/llm-agent-rag":      "llm-agent-rag",
		"github.com/costa92/llm-agent-rag/v2":   "llm-agent-rag",
		"github.com/costa92/llm-agent/internal": "llm-agent",
		"github.com/google/cel-go":              "",
		"":                                      "",
	}
	for in, want := range cases {
		if got := repoFromModulePath(in); got != want {
			t.Errorf("repoFromModulePath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsSemverTag(t *testing.T) {
	cases := map[string]bool{
		"v1.0.0":      true,
		"v0.6.1":      true,
		"v1.0":        false,
		"1.0.0":       false,
		"v1.0.0-rc1":  false,
		"":            false,
		"vX.Y.Z":      false,
		"v1.2.3.4":    false,
		"v01.0.0":     true, // we don't reject leading zeros — git tag sort still works
	}
	for in, want := range cases {
		if got := isSemverTag(in); got != want {
			t.Errorf("isSemverTag(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestBuildDAG(t *testing.T) {
	roster := []string{"core", "rag", "flow", "otel"}
	infos := []repoInfo{
		{Name: "core", Requires: []require{
			{Path: "github.com/costa92/rag", Version: "v1.0.1"},
		}},
		{Name: "rag", Requires: []require{ // back-edge cycle
			{Path: "github.com/costa92/core", Version: "v0.5.0"},
		}},
		{Name: "flow", Requires: []require{
			{Path: "github.com/costa92/core", Version: "v0.5.1"},
		}},
		{Name: "otel", Requires: []require{
			{Path: "github.com/costa92/core", Version: "v0.5.1"},
			{Path: "github.com/costa92/rag", Version: "v1.0.1"},
			{Path: "github.com/costa92/flow", Version: "v0.0.7"},
			{Path: "github.com/google/cel-go", Version: "v0.28.1"},     // dropped (non-org)
			{Path: "github.com/costa92/not-in-roster", Version: "v1"},  // dropped (not in roster)
		}},
	}
	got := buildDAG(infos, roster)
	want := map[string][]string{
		"core": {"rag"},
		"rag":  {"core"},
		"flow": {"core"},
		"otel": {"core", "flow", "rag"},
	}
	for k, v := range want {
		if !reflect.DeepEqual(got[k], v) {
			t.Errorf("DAG[%s] = %v, want %v", k, got[k], v)
		}
	}
}

func TestBuildDAG_DedupesDuplicateEdges(t *testing.T) {
	roster := []string{"a", "b"}
	infos := []repoInfo{
		{Name: "a", Requires: []require{
			{Path: "github.com/costa92/b", Version: "v1"},
			{Path: "github.com/costa92/b/v2", Version: "v2.0.0"}, // same repo, different module path
		}},
	}
	got := buildDAG(infos, roster)
	if !reflect.DeepEqual(got["a"], []string{"b"}) {
		t.Errorf("expected single edge a->b, got %v", got["a"])
	}
}

func TestTopoSort_LeavesFirst(t *testing.T) {
	// Acyclic DAG:
	//   leaf <- mid <- top
	dag := map[string][]string{
		"top":  {"mid"},
		"mid":  {"leaf"},
		"leaf": nil,
	}
	got, cycles := topoSort(dag)
	want := []string{"leaf", "mid", "top"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("order: got %v want %v", got, want)
	}
	if len(cycles) != 0 {
		t.Errorf("expected no cycles, got %v", cycles)
	}
}

func TestTopoSort_HandlesBackEdgeCycle(t *testing.T) {
	// core <-> rag is the canonical back-edge in this ecosystem.
	// `std` is a real leaf so it must come first. When the frontier
	// drains, the cycle-break heuristic picks the smallest-degree
	// node (tie -> alphabetical) and emits it, which lets the rest
	// fall out naturally. Only the forcibly-broken node is reported
	// in `cycles`.
	dag := map[string][]string{
		"core": {"rag"},
		"rag":  {"core"},
		"std":  nil,
	}
	got, cycles := topoSort(dag)
	if !reflect.DeepEqual(cycles, []string{"core"}) {
		t.Errorf("cycles: got %v want [core]", cycles)
	}
	want := []string{"std", "core", "rag"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("order: got %v want %v", got, want)
	}
}

func TestTopoSort_DeterministicTieBreak(t *testing.T) {
	// Two independent leaves — must come out alphabetically.
	dag := map[string][]string{
		"banana": nil,
		"apple":  nil,
	}
	got, _ := topoSort(dag)
	if !reflect.DeepEqual(got, []string{"apple", "banana"}) {
		t.Errorf("expected alpha tie-break, got %v", got)
	}
}

func TestDetectStale(t *testing.T) {
	infos := []repoInfo{
		{Name: "core", LatestTag: "v0.6.1", Requires: []require{
			{Path: "github.com/costa92/rag", Version: "v1.0.1"},
		}},
		{Name: "rag", LatestTag: "v1.0.2"},
		{Name: "flow", LatestTag: "v0.1.1", Requires: []require{
			{Path: "github.com/costa92/core", Version: "v0.6.1"}, // current
		}},
	}
	got := detectStale(infos)
	want := []string{"core pins rag@v1.0.1 (latest v1.0.2)"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("stale:\n got: %v\nwant: %v", got, want)
	}
}

func TestDetectStale_AllCurrent(t *testing.T) {
	infos := []repoInfo{
		{Name: "core", LatestTag: "v0.6.1", Requires: []require{
			{Path: "github.com/costa92/rag", Version: "v1.0.2"},
		}},
		{Name: "rag", LatestTag: "v1.0.2"},
	}
	if got := detectStale(infos); len(got) != 0 {
		t.Errorf("expected no stale entries, got %v", got)
	}
}

func TestDetectStale_NoLatestTag_Skips(t *testing.T) {
	// Without a latest tag we can't compare — must not flag as stale.
	infos := []repoInfo{
		{Name: "core", LatestTag: "", Requires: []require{
			{Path: "github.com/costa92/rag", Version: "v1.0.1"},
		}},
		{Name: "rag", LatestTag: ""},
	}
	if got := detectStale(infos); len(got) != 0 {
		t.Errorf("expected zero stale when latest unknown, got %v", got)
	}
}
