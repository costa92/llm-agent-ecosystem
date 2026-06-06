# ChatTemplate / Prompt Template Plan: a contract-layer prompt component (`llm-agent-contract/prompt`)

Add a first-class, **stdlib-only** prompt-template component to `llm-agent-contract`, alongside `llm/`, `agents/`, `memory/`. Like eino's `ChatTemplate`, it sits beside `ChatModel`: it owns variable interpolation and System/few-shot/history/user message assembly, turning `(vars) → []llm.Message`. It is the missing reusable seam — today only `llm-agent-rag/prompt` has a template, bound to RAG's own `generate.Request` and `store.Hit`, so nothing else can reuse it. The new component's output type is contract's own `[]llm.Message` / `llm.Request`, so agents, RAG, and the future flow-graph node layer share one prompt primitive.

Goal alignment (PROJECT.md core value): "core stays stdlib-only minimal; opt into deps one package at a time." The template engine is `text/template` (stdlib) plus a self-written minimal `{var}` interpolator — **zero third-party deps**. It is additive to contract (a new leaf sub-package importing only `llm/`), so it is a clean minor bump with no API churn to existing packages.

This plan delivers the component and its agent integration seam. It deliberately does **not** build the flow node — it only fixes the interface so a node wrapper is trivial later (see sibling `flow-graph` plan).

Status: **PLANNED — revised after 2-round review (Plan agent + codex). Verdict: APPROVE-WITH-CHANGES.**

> **Cross-plan contract (pinned in `.planning/eino-gap-plans/README.md`):** the base `Template.Format` returns `[]llm.Message`; the optional `Requester.FormatRequest` returns `llm.Request`. The sibling `flow-graph` Template node consumes **`prompt.Requester`** (FormatRequest → `llm.Request`). This reconciles flow-graph's open question CT-1.

---

## 审核修订（2-round review, 2026-06-05）— 必读，覆盖下文相应决策

两轮审核结论：**作为 contract 叶子包落地是对的**，但下游集成（T9/T10）和几处接口细节被纠正。本里程碑**只交付 `prompt` 包本身**；agent 集成与 RAG 迁移**移出本里程碑**。

**R1 — D5/T9 的 agent 集成修法错了（codex 证伪）。** 核心仓 `NewSimpleAgent` 吃**具体 `SimpleOptions` 结构体**（`simple.go:17,24`），ReAct 同理（`react.go:29,52`）——**不是 variadic 函数式选项**。`agents.WithPromptTemplate(...)` 是引入新 API 风格，不是最小集成。**正确修法**：核心仓抽一个 `generateRequestWithBudget(ctx, model, req)` helper，让 string-prompt 路径和 template 路径都走它，**保住 `budget.Charge` 的 pre/post 计费**（`agent_chatmodel.go:31,42`）。否则注入 template 静默丢预算。**此修正取代 D5(b) 与 T9 的"functional option"措辞。**

**R2 — SimpleAgent 不是全部 prompt 面（第一轮漏）。** ReAct 用 `fmt.Sprintf` 拼 scratchpad/system（`react.go:97,104`），native ReAct 完全绕过只发 input（`react.go:154,160`）。"包级模板故事"需 **per-agent 语义**，单个 option 盖不住。集成因此是**多步、跨 agent 范式**的工作，独立里程碑处理。

**R3 — `FormatRequest` 必须透传 `llm.Request.Metadata`（第一轮漏）。** `llm.Request` 有 `Metadata`（`types.go:10`），RAG 默认模板保留 `rc.Metadata`（`default.go:40`）。`Requester.FormatRequest` 若不设计 metadata 透传，桥接 RAG 时静默丢。**T5 验收加：Metadata 透传用例。**

**R4 — vision 在 v1 无机制，必须显式声明（两轮一致）。** `Spec`/`Turn`/`Vars` 全 string，但 `llm.Message` 有 `Images`（`types.go:28`）。doc.go **明写 v1 只产纯文本 turn**，image 需后续 escape-hatch。别让 D3 的"vision-intact"叙事变空头支票。

**R5 — RAG 迁移是 RAG 仓的类型边界重构（两轮一致）。** RAG 有同名 `prompt.Template` 返回 RAG 私有 `generate.Request`（非 `llm.Request`，`generate/types.go`）。T10 **明确为 RAG 仓独立任务、bridge-only、移出本里程碑**。

**R6 — contract tag 的 git 上下文坑。** "最新 v0.4.0" 须在 `cd llm-agent-contract` 内跑 `git tag`（contract 是 gitignore 出去的独立仓）；umbrella 根目录 `git tag` 只看到 umbrella 自己的 v0.1.0，别被误导。新包发 **v0.5.0**。

---

## Decided design / end-state

### Package: `llm-agent-contract/prompt`

Chosen name **`prompt/`** over `template/`:
- It mirrors `llm-agent-rag/prompt` (the existing precedent), easing the eventual RAG bridge/migration and matching team vocabulary.
- `template` over-narrows to "the engine"; this package is about prompt assembly (system + few-shot + history + user), of which templating is one part.
- eino calls the component `ChatTemplate`, but in Go a package named `template` collides conceptually with stdlib `text/template`; `prompt.Template` reads cleanly (`prompt.New(...)`, `prompt.Template`).

Import surface: **`llm` only** (for `Message` / `Request`). No other contract package, no third-party. Stays a leaf.

### Core types and interface (copy-ready signatures)

```go
// Package prompt owns the contract's reusable prompt-template seam.
//
// A Template turns named variables into an ordered []llm.Message: a
// System turn, optional few-shot example pairs, an injected history
// slice, and the current user turn. It is the prompt-assembly analogue
// of llm.ChatModel — variable interpolation + message layout — and its
// output type is contract's own llm.Message, so agents, RAG, and the
// future flow node layer share one primitive.
//
// The engine is stdlib-only: Go text/template for FromTemplate, or a
// minimal {var} interpolator for FromMessages. No third-party deps.
package prompt

import (
	"context"

	"github.com/costa92/llm-agent-contract/llm"
)

// Vars is the interpolation input. Values are formatted with the
// engine's default rules (text/template's %v-equivalent for {var}).
type Vars map[string]any

// Template renders Vars into an ordered message list. Implementations
// MUST be safe for concurrent use (a compiled template is immutable);
// Format is pure given (vars, history) and performs no I/O.
type Template interface {
	// Format interpolates vars and assembles the messages. history is
	// spliced in at the template's history slot (see WithHistory); pass
	// nil when there is no prior conversation.
	Format(ctx context.Context, vars Vars, history []llm.Message) ([]llm.Message, error)
}

// Requester is an OPTIONAL capability: a Template that can emit a full
// llm.Request (lifting the System turn into Request.SystemPrompt per the
// llm contract). Callers test for it; the base interface stays minimal.
//
//	if r, ok := tmpl.(prompt.Requester); ok { req, err := r.FormatRequest(ctx, vars, history) }
type Requester interface {
	FormatRequest(ctx context.Context, vars Vars, history []llm.Message) (llm.Request, error)
}
```

### Message-list specification (the spec object that builds a Template)

```go
// Spec declares the message layout a Template renders. It is the
// builder input; New(spec) compiles it once into an immutable Template.
type Spec struct {
	// System is the system instruction template (engine-interpolated).
	// Empty means no system turn.
	System string

	// FewShot are fixed example turns, interpolated with the SAME vars
	// as the rest (usually constant, but vars are allowed). They are
	// emitted after System and before History.
	FewShot []Turn

	// User is the current user-turn template. Required (non-empty).
	User string

	// HistorySlot controls where the runtime []llm.Message history is
	// spliced. Default (BeforeUser) is the common case: system, few-shot,
	// history, user.
	HistorySlot HistoryPlacement

	// Engine selects the interpolation syntax. Zero value = EngineBrace
	// (minimal {var}); EngineGoTemplate opts into text/template.
	Engine Engine
}

// Turn is one literal template turn (role + content template).
type Turn struct {
	Role    string // "system" | "user" | "assistant" (llm role strings)
	Content string // interpolated with the same Vars
}

type HistoryPlacement int

const (
	BeforeUser HistoryPlacement = iota // system, fewshot, HISTORY, user  (default)
	AfterFewShot                       // alias of BeforeUser today; reserved for clarity
	NoHistory                          // ignore the history arg entirely
)

type Engine int

const (
	EngineBrace      Engine = iota // {var} minimal interpolation (default, injection-safe)
	EngineGoTemplate               // text/template ({{.var}} + pipelines, range, if)
)

// New compiles spec into an immutable Template. It validates the engine
// syntax up front (returns an error on a malformed template or an
// undeclared-at-parse-time text/template) so render-time failures are
// limited to missing vars / type mismatches.
func New(spec Spec) (Template, error)

// MustNew is the test/package-var convenience: panics on a compile error.
func MustNew(spec Spec) Template
```

### Render-error contract

```go
// ErrMissingVar is returned (wrapped) by Format when a referenced
// placeholder has no value in Vars. For EngineBrace the missing key
// name is included; for EngineGoTemplate the underlying text/template
// "map has no entry" error is wrapped. Callers test with errors.Is.
var ErrMissingVar = errors.New("prompt: missing template variable")
```

EngineBrace renders strictly: an unfilled `{name}` is an error (not a silent empty), so typos surface in tests rather than shipping a broken prompt. A literal brace is escaped `{{` → `{` (documented in `doc.go`).

### Usage example (goes in `example_test.go`)

```go
tmpl := prompt.MustNew(prompt.Spec{
	System: "You are a {persona} assistant. Answer in {lang}.",
	FewShot: []prompt.Turn{
		{Role: "user", Content: "ping"},
		{Role: "assistant", Content: "pong"},
	},
	User:        "{question}",
	HistorySlot: prompt.BeforeUser,
})

history := []llm.Message{{Role: "user", Content: "hi"}, {Role: "assistant", Content: "hello"}}

msgs, err := tmpl.Format(ctx, prompt.Vars{
	"persona":  "concise",
	"lang":     "English",
	"question": "what is 2+2?",
}, history)
// msgs => [system, user:ping, assistant:pong, user:hi, assistant:hello, user:"what is 2+2?"]

// Capability path: emit a ready llm.Request with System lifted out.
req, _ := tmpl.(prompt.Requester).FormatRequest(ctx, vars, history)
resp, _ := model.Generate(ctx, req)
```

---

## Key decision records

### D1 — Template syntax: `text/template` vs minimal `{var}` vs both
- **(a) `text/template` only.** Max expressivity (`range`, `if`, pipelines — good for "loop few-shot over a slice"). Cost: `{{.x}}` ergonomics are clunky for simple prompts; the action language is a (small) injection/footgun surface if a template string ever comes from user input; needs HTML-escaping awareness (we'd use `text/template`, not `html/template`, so no auto-escape — fine for prompts but a sharp edge).
- **(b) Minimal `{var}` only.** Self-written ~40-line interpolator: scan for `{name}`, look up in `Vars`, strict-miss → `ErrMissingVar`. Injection-safe (no code execution; values are inserted verbatim, never re-parsed). Trivial to teach. Cost: no loops/conditionals — few-shot must be expressed structurally (the `Spec.FewShot` slice), not in-string.
- **(c) Both, selected by `Spec.Engine`.** Default `EngineBrace` for the 90% case; `EngineGoTemplate` opt-in for power users who need `range`/`if`.
- **Recommendation: (c), defaulting to (b).** Few-shot expressivity is handled by the **structural** `Spec.FewShot` slice + `Spec.HistorySlot`, not by in-string loops, so the brace engine covers the common case with the smallest, safest surface. `text/template` is one field away for callers who genuinely need control flow, and it costs nothing extra (stdlib). Karpathy: ship the minimal thing, expose the power as an explicit opt-in rather than forcing `{{.}}` on everyone.

### D2 — Interface shape: `[]Message` vs `llm.Request`; streaming; single-text Render
- **(a) `Format(...) ([]llm.Message, error)` as the base.** Composable: caller decides whether to lift System into `Request.SystemPrompt`, attach tools, set temperature. Matches how core agents already hand-build `[]llm.Message` then wrap a `Request`.
- **(b) Return `llm.Request` from the base.** One-call convenience, but bakes in `SystemPrompt` lifting + opinionates about a one-shot request, and a template genuinely doesn't know `MaxOutputTokens`/`Temperature`. Couples the prompt primitive to request-construction policy.
- **(c) Both — base returns messages, optional `Requester` returns a Request.**
- **Recommendation: (c).** Base `Format → []llm.Message` keeps the primitive pure and composable (matches `agents/agent_chatmodel.go`'s inline pattern and the flow-node contract in D6). `Requester` is an **optional** capability interface (the established contract idiom — see `llm.ToolCaller`, `memory.Lister`) for callers who want a ready `llm.Request` with System lifted out per the `llm.Request` doc. **No streaming variant** — templates are pure CPU string assembly; a `StreamFormat` would be ceremony with no benefit (open question O3 only if a future use case appears). **No separate `Render(...) (string, error)`** in v1: a single-text prompt is just `Spec{User: "..."}` → `Format` → `msgs[0].Content`; adding a `Renderer` is a cheap follow-up if demand shows (tracked as O2), not paid for speculatively.

### D3 — Message-list templating: System + few-shot + history + current user
- **(a) All-in-string** (one big `text/template` producing a serialized transcript). Rejected: loses the `[]llm.Message` structure providers need (role separation, System lifting, vision images on user turns).
- **(b) Structural `Spec` with a typed few-shot slice + a `HistorySlot` enum for the `[]llm.Message` injection point.** History is a runtime `[]llm.Message` arg to `Format` (not a template var) — it's already structured messages, not text to interpolate.
- **(c) Hybrid:** few-shot as a template `range` over a vars slice.
- **Recommendation: (b).** Few-shot pairs are *structure* (role-tagged turns), so they belong in `Spec.FewShot []Turn`, interpolated with the same `Vars`. History is *already* `[]llm.Message` from memory/agent state — passing it as a typed `Format` argument and splicing at `HistorySlot` avoids the lossy round-trip of serializing messages into a string and re-parsing. This keeps roles, and later vision images, intact. (c)'s in-string `range` is available to `EngineGoTemplate` users for *content* generation but is not the load-bearing few-shot mechanism.

### D4 — Composition / partials / named fragments
- **(a) None** — flat `Spec` only.
- **(b) Named partials registry** (eino-style sub-templates, `{{template "x"}}`).
- **(c) Go-level composition only:** since `Spec` is a plain struct, callers compose by building `Spec` fields from shared `[]Turn` / strings in their own code; `EngineGoTemplate` users additionally get stdlib `text/template`'s native `{{define}}`/`{{template}}` for free within a single template string.
- **Recommendation: (c) (≈ minimal).** No bespoke partial registry in v1 — it's a maintenance surface with no proven pull (the only existing template, RAG's, has zero partials). Reuse is achieved by sharing Go values (a package-level `var baseFewShot = []prompt.Turn{...}`), and power users inherit stdlib `text/template`'s own composition. Revisit only if a consumer needs cross-template fragment sharing (open question O1).

### D5 — Agent integration point
- **(a) Inject a `prompt.Template` into `SimpleAgent`/`ReActAgent` constructors** (new required field). Cleanest conceptually but a breaking change to core agent constructors and forces every caller to supply a template.
- **(b) Optional functional option** (`agents.WithPromptTemplate(t)`); agents keep their current hand-built default and use the injected template only when set.
- **(c) Leave core agents untouched in this plan; only expose the component + a tiny adapter, integrate agents in a follow-up.**
- **Recommendation: (b), but the agent-side change lives in the `llm-agent` core repo, not in this contract PR.** This contract plan ships only the `prompt` package (the seam). Wiring `SimpleAgent`/`ReActAgent` to optionally consume a `prompt.Template` is a **separate core-repo task** (listed in the implementation plan as cross-repo follow-up, gated on the new contract tag) so the contract release stays purely additive and reviewable on its own. The integration is non-breaking: agents fall back to their current `generateFromPrompt` path when no template is injected. **Open question O4:** confirm whether the agent option belongs to this milestone or the flow-graph milestone.

### D6 — Flow-node forward-compat
The flow node contract (sibling `flow-graph` plan) needs a node `func(ctx, in) (out, error)`. The template's `Format(ctx, Vars, []llm.Message) ([]llm.Message, error)` is already node-shaped:
- **Input** is `Vars` (a `map[string]any`) + a structured history slice — directly mappable from a flow node's input bag.
- **Output** is `[]llm.Message` — exactly what a downstream `ChatModel` node consumes.
- **Decision: keep `Format`'s signature `ctx`-first and return `([]llm.Message, error)`** (no node-specific types leak into the prompt package). The flow repo will wrap a `prompt.Template` in its own `Node` adapter; nothing node-specific is added here. The flow-graph **Template node consumes the optional `Requester`** (FormatRequest → `llm.Request`) so a downstream ChatModel node receives a ready `llm.Request`. **Open question O5:** does the flow node want `Vars` to come from a typed struct or the `map[string]any` bag? Align the bag key convention with the flow plan before either ships, but no code coupling now.

---

## Implementation plan (ordered, each task carries a verification standard)

All `go` commands run with **`GOWORK=off`** in `llm-agent-contract`. Branch off `main` first (`feat/prompt-component`). Karpathy/TDD: write the failing test first where noted.

### T1 — Package skeleton + interface
Create `prompt/prompt.go` with `Template`, `Requester`, `Vars`, `Spec`, `Turn`, `Engine`, `HistoryPlacement`, `ErrMissingVar`, `New`, `MustNew`.
- **Verify:** `GOWORK=off go build ./prompt/...` compiles; `go vet ./...` clean; no import outside `context`, `errors`, `strings`, `text/template`, `fmt`, `github.com/costa92/llm-agent-contract/llm`.

### T2 — Brace engine, strict-miss first (TDD)
Write `prompt/brace_test.go` asserting that `{missing}` returns an error wrapping `ErrMissingVar` **before** writing the interpolator. Then implement the `{var}` scanner (handle `{{`→`{` escape, multi-occurrence, adjacent placeholders).
- **Verify:** the missing-var test goes red→green; table tests cover escape, repeated var, empty value (allowed), unmatched `{` (parse error from `New`, not `Format`).

### T3 — `text/template` engine
Implement `EngineGoTemplate`: compile in `New` with `Option("missingkey=error")` so a missing key wraps to `ErrMissingVar` at render.
- **Verify:** test a `{{range}}` few-shot-in-content case renders; a `{{.absent}}` render returns `ErrMissingVar`; a malformed template fails at `New`, not `Format`.

### T4 — Message assembly + HistorySlot (TDD)
Implement `Format`: emit System (if non-empty), FewShot turns, splice history per `HistorySlot`, then User. Write the ordering test from the usage example first.
- **Verify:** order test (`system, fewshot…, history…, user`) passes; `NoHistory` ignores the arg; `nil` history is a no-op; empty `Spec.User` → `New` returns an error (User is required); concurrent `Format` from 10 goroutines on one compiled template is race-clean under `go test -race`.

### T5 — `Requester` / `FormatRequest`
Implement the optional `Requester` on the concrete template: run `Format`, then lift a leading `system` message into `Request.SystemPrompt` (per the `llm.Request` doc convention), leaving the rest in `Messages`.
- **Verify:** test asserts `req.SystemPrompt` is set and `req.Messages` has no system turn; a Spec with empty System yields empty `SystemPrompt`; `tmpl.(prompt.Requester)` type-asserts true.

### T6 — `doc.go` (package doc, contract style)
Write `prompt/doc.go` mirroring `llm/doc.go` / `memory/interfaces.go` prose: enumerate the exported surface, the two engines, the strict-miss + `{{`-escape rules, the optional-`Requester` idiom, and the "no streaming, no partials in v1, output is `[]llm.Message`" stance.
- **Verify:** `go doc ./prompt` renders the intended surface; no exported symbol lacks a doc comment (`go vet`-adjacent manual check).

### T7 — Example test
Add `prompt/example_test.go` (`Example_` with `// Output:`) matching the usage example, using only stdlib + `llm`.
- **Verify:** `go test ./prompt/...` runs the example and the `// Output:` matches.

### T8 — README + CHANGELOG + version bump
Add a `prompt` row to contract `README.md` / `README.zh-CN.md` package roster; note "NEW in v0.5" the way `doc.go` annotates Vision/ImageGen by version. **Latest contract tag is `v0.4.0`** (verified `git tag --sort=-v:refname`) → additive new package → **`v0.5.0`** minor bump.
- **Verify:** `go mod tidy` produces no drift (stdlib-only — the test.yml drift gate stays green); `go vet ./... && go build ./... && go test ./... -count=1` all green; release-precheck (no-replace) N/A until a `release/**` branch.
- **Note:** contract has **no api-snapshot tooling** (verified — no apidiff/snapshot in `.github/`). The "api snapshot" deliverable from the brief is **not applicable** to this repo; CI compatibility is the tidy/vet/build/test gate only. (Flagged so reviewers don't expect a snapshot file.)

### T9 (cross-repo follow-up, NOT in the contract PR) — agent integration
In `llm-agent` core: add `agents.WithPromptTemplate(prompt.Template)` option (D5(b)); `generateFromPrompt`-path agents use the template when set, else current behavior. Requires repinning core to `contract v0.5.0`.
- **Verify:** existing core agent tests pass unchanged with no option set; a new test asserts an injected template drives the message list. Gated on the contract tag existing (dep-currency green).

### T10 (optional follow-up) — RAG bridge/migration
In `llm-agent-rag`: provide a thin adapter so `prompt.Template` can satisfy RAG's needs, or migrate `DefaultQATemplate` to build on the contract `prompt` package. RAG already imports `contract v0.2.0`; bumping to `v0.5.0` is the prerequisite. Keep RAG's `RenderContext`/`store.Hit` mapping (chunks → a `Vars["context"]` string or few-shot turns) in RAG, not in contract.
- **Verify:** RAG's existing `prompt/default_test.go` semantics preserved (same rendered output) whether kept as-is or rebuilt on the new primitive; no RAG behavior change without a test asserting it. **Open question O6:** is RAG migration in scope for this milestone or deferred — RAG's template is request-shaped (`generate.Request`) and chunk-aware, so a full migration is non-trivial; a bridge is cheaper.

---

## Risks / open questions / sibling-plan interaction

- **O1 — Partials.** Do any consumers need cross-template fragment sharing? If yes, reconsider D4. Default: no.
- **O2 — Single-text `Renderer`.** Add `Render(...) (string, error)` only if a non-message caller appears. Currently derivable from `Format`.
- **O3 — Streaming.** No `StreamFormat` planned; revisit only with a concrete streaming-template use case.
- **O4 — Where does agent integration land** — this milestone (T9) or the flow-graph milestone? Affects whether the contract PR is followed immediately by a core repin.
- **O5 — flow-graph Vars contract.** Align the `Vars` key/bag convention with the sibling `flow-graph` plan before either ships, so a `prompt.Template`-as-node wrapper maps cleanly. **No code coupling now**, but the input/output type contract (`Vars map[string]any` in, `llm.Request` out via `Requester`) must match what flow nodes expect. This is the single most important cross-plan handshake (= flow-graph CT-1).
- **O6 — RAG migration vs bridge** (T10 scope). RAG's template returns its own `generate.Request` and is chunk-aware; full migration is non-trivial. Recommend a bridge first, migration deferred.
- **Risk — `text/template` non-escaping.** `EngineGoTemplate` uses `text/template` (no auto-escaping). For prompts that's correct (we want raw text), but document that template *strings* should be developer-authored, not user-supplied, to avoid action-injection. `EngineBrace` (the default) has no such surface.
- **Risk — history-splice ambiguity** if a `Spec` both has `FewShot` and history at the same slot; `HistorySlot` semantics are documented (history always after few-shot in `BeforeUser`).
- **Sibling `flow-graph`:** this plan deliberately stops at the seam; the flow node wrapper is theirs. The only contract is O5/CT-1.

---

## Version & release impact

- **Repo:** `llm-agent-contract` only (the new package + docs).
- **Tag:** **`v0.5.0`** — additive new sub-package, no change to `llm/`/`agents/`/`memory/` exports; minor bump from current latest **`v0.4.0`** (verified locally).
- **Consumer repin cascade:** **None forced by the contract PR itself** — adding a package breaks no existing importer. The cascade is **opt-in and lazy**: a repo only repins to `v0.5.0` when it wants to *use* `prompt`. Concretely:
  - **T9 (core agents)** would repin `llm-agent` → `contract v0.5.0` when integrating the option. That is a *core* release decision, not part of this tag.
  - **T10 (RAG)** would repin `llm-agent-rag` (currently `contract v0.2.0`) → `v0.5.0` only if/when it adopts/bridges the component.
- **dep-currency gate:** judged against **latest local git tags** (`cmd/depcheck` reads `git tag --sort=-v:refname`, local not remote). After cutting `v0.5.0` locally + remotely, any repo *that already pins contract* may show as stale until it repins — but since none are forced to consume `prompt`, no repo is *broken*. Coordinate the tag push with the umbrella dep-currency expectation so the gate isn't red on a no-op drift.
- **pre-commit replace-guard:** during local cross-repo dev (T9/T10) against `replace => ../llm-agent-contract`, commit with `--no-verify` until the lockstep drop-replace + repin to the published `v0.5.0` tag, matching the established workflow.
