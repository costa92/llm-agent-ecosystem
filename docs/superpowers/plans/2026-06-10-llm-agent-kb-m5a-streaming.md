# M5a Real Streaming Backend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a real token-streaming ask path to `llm-agent-kb` — `POST /api/kb/{id}/ask/stream` emits SSE `token` deltas then a terminal `done` event with citations, while persisting the streamed answer as a qa_message pair.

**Architecture:** Option A (token-stream via `llm.ChatModel.Stream` over a kb-assembled grounded context). `ragsvc` gains a `StreamAnswer` seam that (1) calls the existing `RagPort.Retrieve` to get `[]store.Hit`, (2) renders the SAME prompt `rag.System` uses — `rag/prompt.DefaultQATemplate.Render` — over those hits, (3) calls the held `llm.ChatModel.Stream`, emitting kb-local `StreamEvent`s (token deltas + a terminal event carrying hits-derived citations). `retrieval` gains `AskStream` that maps those events to wire DTOs and persists the accumulated answer via the M4 `Recorder`. `httpapi` gains an SSE handler reusing the exact `progressHandler` flush pattern. `ragsvc` stays the SOLE `rag/*` importer (§4): retrieval/httpapi see only kb-local event types.

**Tech Stack:** Go 1.26, module `github.com/costa92/llm-agent-kb`. `llm-agent-contract@v0.5.0` (`llm.ChatModel.Stream`, `StreamReader`, `StreamEvent`, `ScriptedLLM.Stream`), `llm-agent-rag@v1.11.0` (`prompt.DefaultQATemplate`, `store.Hit`, `rag.SearchOptions`). No version bumps (streaming uses already-pinned deps). **ALL Go commands prefixed `GOWORK=off`.**

---

## STREAMING-APPROACH DECISION: Option A (token-stream via model.Stream)

**Justification (2 sentences):** rag's generation seam `generate.Model` exposes ONLY `Generate` (no Stream) and `*rag.System` exposes no streaming Ask, so real token-level streaming can only come from `llm.ChatModel.Stream`, which kb already holds in `ragsvc.Service.model`. Option A stays faithful to rag by reusing rag's own `prompt.DefaultQATemplate.Render` (the default template `rag.System.askRound` renders) over `RagPort.Retrieve` hits and deriving citations from those same hits — so the prompt shape and citation mapping are rag's, not reimplemented — while delivering genuine progressive token output that Option B (coarse stage events) cannot.

**Faithfulness boundary (documented tradeoff):** Option A bypasses rag's reflection/grader/active-retrieval/rerank/pack orchestration. The non-stream `POST /ask` path (M1) is UNCHANGED and keeps the full `rag.System.Ask` orchestration; `/ask/stream` is a deliberately lighter, single-pass grounded-answer path optimized for streaming UX. NOTE: the stream path uses `rag.System.Retrieve`, which does NOT rerank — rerank only runs in rag's Ask/pack path (`rag/retrieve.go` ignores `EnableRerank`). So in the stream path `Hybrid` only affects the mode label in diagnostics; `vector` and `hybrid` return identical hits (rerank parity with non-stream `/ask` is OUT of M5a scope). We still pass `EnableRerank: req.Hybrid` (harmless), but make no claim that it reranks. This is the M5a-scoped tradeoff: token-by-token UX now, full-orchestration (reflection + rerank) streaming deferred.

---

## SSE EVENT WIRE CONTRACT (the M5b frontend consumes this — confirm before building frontend)

Endpoint: `POST /api/kb/{id}/ask/stream`
Request body (identical fields to non-stream `/ask`): `{"q":"...","mode":"vector|hybrid","topK":5,"sessionId":"..."}`
Response: `Content-Type: text/event-stream`, `Cache-Control: no-cache`, `Connection: keep-alive`. One SSE event per chunk, flushed immediately.

Event sequence: zero or more `token` events, then exactly ONE terminal event (`done` on success, `error` on failure). After the terminal event the stream closes.

```
event: token
data: {"text":"partial answer text"}

event: done
data: {"citations":[{"chunkId":"c1","docId":"d1","title":"Doc One","sectionPath":["Intro"],"score":0.91,"snippet":"..."}],"diagnostics":{"mode":"hybrid","hitCount":3},"sessionId":"ab12..."}

event: error
data: {"error":"human-readable message"}
```

Field shapes (exact):
- `token` `data`: `{"text": string}` — one delta; concatenating all `token.text` in arrival order yields the full answer.
- `done` `data`: `{"citations": Citation[], "diagnostics": {"mode": string, "hitCount": number}, "sessionId": string}` where `Citation` is the M4 shape `{chunkId, docId, title, sectionPath?, score, snippet}` (lowerCamel JSON, `sectionPath` omitted when empty).
- `error` `data`: `{"error": string}`. Emitted INSTEAD OF `done` when retrieval or streaming fails after headers are sent. (Pre-stream failures — bad body, bad mode — return a normal HTTP 4xx before any SSE byte.)

Citation-set note: `/ask/stream` citations are derived from the RAW retrieved hits (one per hit), whereas non-stream `/ask` citations come from rag's packed/deduped/sanitized `Answer.Citations`. So for the same query the stream `citations` may differ in count and order from `/ask` — only the field shape matches (`retrieval.Citation`: `chunkId/docId/title/sectionPath?/score/snippet`).

Persistence: on a successful `done`, the accumulated answer + citations are persisted as a qa_message pair via the M4 Recorder; `sessionId` in `done` is the ensured/created session id.

---

## File Structure

- **Create** `internal/ragsvc/stream.go` — kb-local `StreamEvent`/`StreamEventKind` types + `Service.StreamAnswer`. The ONLY new rag/* importing code (uses `rag/prompt`, `rag/store`, `rag/rag.SearchOptions`).
- **Modify** `internal/ragsvc/ragsvc.go` — add `StreamAnswer` to the `RagPort` interface.
- **Create** `internal/ragsvc/stream_test.go` — unit test for `StreamAnswer` over a `ScriptedLLM` + in-memory rag store.
- **Modify** `internal/retrieval/retrieval.go` — add `StreamCallback`/`AskStream` mapping rag events → wire DTOs + persistence; reuse existing `Citation`, `persist`, `truncate`.
- **Modify** `internal/retrieval/retrieval_test.go` — extend `fakeRag` with `StreamAnswer`; add `AskStream` unit test.
- **Modify** `internal/httpapi/httpapi.go` — add `AskStream` to the `Asker` interface and the route registration.
- **Modify** `internal/httpapi/handlers.go` — add `askStreamHandler` here (the SSE sibling of `progressHandler`; `io`/`encoding/json` already imported) + add the `retrieval` import.
- **Modify** `internal/httpapi/handlers_test.go` (or a new `internal/httpapi/stream_test.go`) — handler-level SSE test with a fake Asker.
- **Modify** `cmd/kbd/main_test.go` — add the gated streaming e2e (`TestStreamingAskEndToEnd`).
- `cmd/kbd/main.go` needs NO change — `retrievalSvc` already satisfies the widened `Asker` once `AskStream` is added.

---

### Task 1: Branch + baseline green

**Files:** none (setup)

- [ ] **Step 1: Create the feature branch**

Run:
```bash
git -C /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb checkout -b feat/m5a-streaming
```
Expected: `Switched to a new branch 'feat/m5a-streaming'`

- [ ] **Step 2: Confirm baseline build/vet/test green**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb && GOWORK=off go vet ./... && GOWORK=off go test ./...
```
Expected: `go vet` prints nothing; `go test` ends with all packages `ok` or `(cached)` (gated PG tests `SKIP` because `LLM_AGENT_KB_PG_URL` is unset). If anything fails, STOP — the branch must start green.

---

### Task 2: ragsvc kb-local stream types + StreamAnswer seam (TDD)

**Files:**
- Create: `internal/ragsvc/stream.go`
- Test: `internal/ragsvc/stream_test.go`
- Modify: `internal/ragsvc/ragsvc.go` (add `StreamAnswer` to `RagPort`)

Design note: `StreamAnswer` keeps `rag/*` types internal. It returns kb-local `StreamEvent`s through a callback so importers never see `llm.StreamEvent` or `store.Hit`. Citations are kb-local `StreamCitation` (a flat struct), derived from the retrieved hits exactly as the non-stream path derives them from `ans.Hits`/`ans.Citations`.

- [ ] **Step 1: Write the failing test**

Create `internal/ragsvc/stream_test.go`. Use rag's real in-memory store (`ragstore.NewInMemoryStore(8)`) seeded via `Upsert` — NOT a hand-rolled fake. The store's `Upsert` rejects vectors whose length != the store dim (8), and `Search` filters by namespace, so the seeded chunk MUST carry `Namespace: "ns1"` and an 8-dim `Vector`. The scripted embedder (`WithEmbedDimensions(8)`) embeds every text to a deterministic 8-dim vector of `0.1`, so a stored chunk vector of all-`0.1` yields cosine similarity 1.0 and the retrieve returns the seeded hit:
```go
package ragsvc

import (
	"context"
	"testing"

	"github.com/costa92/llm-agent-contract/llm"
	ragstore "github.com/costa92/llm-agent-rag/store"
)

// vec8 is the 8-dim all-0.1 vector — matches the scripted embedder's
// deterministic output (float32(1)/10 per component) so the seeded chunk
// scores cosine-1.0 against any embedded query and the retrieve returns it.
func vec8() []float32 {
	v := make([]float32, 8)
	for i := range v {
		v[i] = 0.1
	}
	return v
}

func TestStreamAnswerEmitsTokensThenDone(t *testing.T) {
	store := ragstore.NewInMemoryStore(8)
	if err := store.Upsert(context.Background(), []ragstore.StoredChunk{
		{ID: "c1", Namespace: "ns1", DocID: "d1", Title: "Doc One", SectionPath: []string{"Intro"}, Content: "the quick brown fox", Vector: vec8()},
	}); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	model := llm.NewScriptedLLM(llm.WithResponses(llm.Response{Text: "streamed answer", Usage: llm.Usage{TotalTokens: 2}}))
	embedder := llm.NewScriptedLLM(llm.WithEmbedDimensions(8))
	svc := New(Deps{Model: model, Embedder: embedder, RagStore: store})

	var tokens []string
	var doneCites []StreamCitation
	var doneHitCount int
	var doneSeen bool
	err := svc.StreamAnswer(context.Background(), "fox", StreamRequest{Namespace: "ns1", TopK: 5, Hybrid: false},
		func(ev StreamEvent) error {
			switch ev.Kind {
			case StreamEventToken:
				tokens = append(tokens, ev.Text)
			case StreamEventDone:
				doneSeen = true
				doneCites = ev.Citations
				doneHitCount = ev.HitCount
			}
			return nil
		})
	if err != nil {
		t.Fatalf("StreamAnswer: %v", err)
	}
	if len(tokens) == 0 {
		t.Fatal("expected at least one token event")
	}
	if got := joinTokens(tokens); got != "streamed answer" {
		t.Fatalf("concatenated tokens = %q, want %q", got, "streamed answer")
	}
	if !doneSeen {
		t.Fatal("expected a terminal done event")
	}
	if doneHitCount != 1 {
		t.Fatalf("done HitCount = %d, want 1", doneHitCount)
	}
	if len(doneCites) != 1 || doneCites[0].ChunkID != "c1" || doneCites[0].DocID != "d1" || doneCites[0].Title != "Doc One" {
		t.Fatalf("done citations = %+v, want one c1/d1/Doc One", doneCites)
	}
	if doneCites[0].Snippet != "the quick brown fox" {
		t.Fatalf("citation snippet = %q, want chunk content", doneCites[0].Snippet)
	}
}

func joinTokens(ts []string) string {
	out := ""
	for _, t := range ts {
		out += t
	}
	return out
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb && GOWORK=off go test ./internal/ragsvc/ -run TestStreamAnswerEmitsTokensThenDone -v
```
Expected: COMPILE FAIL — `undefined: StreamCitation`, `undefined: StreamEvent`, `undefined: StreamRequest`, `svc.StreamAnswer undefined`.

- [ ] **Step 3: Create the StreamAnswer implementation**

Create `internal/ragsvc/stream.go`:
```go
package ragsvc

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/costa92/llm-agent-contract/llm"
	ragprompt "github.com/costa92/llm-agent-rag/prompt"
	ragcore "github.com/costa92/llm-agent-rag/rag"
)

// StreamRequest is the kb-side streaming-ask request. Mirrors AskRequest, but
// note rag's Retrieve path ignores EnableRerank (rerank runs only in Ask/pack),
// so Hybrid here only labels the diagnostics mode — it does NOT rerank the
// stream hits (rerank parity with non-stream /ask is out of M5a scope).
type StreamRequest struct {
	Namespace string
	TopK      int
	Hybrid    bool
}

// StreamEventKind enumerates the kb-local streaming event variants. Kept
// kb-local (NOT llm.StreamEventKind) so retrieval/httpapi never import the
// contract stream types — ragsvc stays the boundary (spec §4).
type StreamEventKind uint8

const (
	StreamEventToken StreamEventKind = iota // Text holds one answer delta
	StreamEventDone                         // terminal; Citations + HitCount populated
)

// StreamCitation is the kb-local projection of a retrieved hit, flattened so
// importers never see rag/store types. Mirrors the fields retrieval.Citation
// maps to the external JSON shape.
type StreamCitation struct {
	ChunkID     string
	DocID       string
	Title       string
	SectionPath []string
	Score       float64
	Snippet     string
}

// StreamEvent is the kb-local streaming union. Field population is gated by
// Kind: StreamEventToken → Text; StreamEventDone → Citations + HitCount.
type StreamEvent struct {
	Kind      StreamEventKind
	Text      string
	Citations []StreamCitation
	HitCount  int
}

// StreamAnswer runs a single-pass grounded streaming answer (Option A): it
// retrieves context via the held rag System, renders the SAME prompt the rag
// default QA template uses, then streams the chat model's tokens. Token deltas
// are delivered to emit as they arrive; a terminal StreamEventDone carries the
// citations derived from the retrieved hits. This deliberately bypasses rag's
// reflection/grader orchestration (M5a tradeoff) — the non-stream Ask path
// keeps the full pipeline.
func (s *Service) StreamAnswer(ctx context.Context, question string, req StreamRequest, emit func(StreamEvent) error) error {
	ctx, span := s.tracer.Start(ctx, "ragsvc.StreamAnswer")
	defer span.End()
	hits, err := s.wrapper.Retrieve(ctx, question, ragcore.SearchOptions{
		Namespace: req.Namespace,
		TopK:      req.TopK,
		// EnableRerank is set for symmetry but is a NO-OP here: rag's Retrieve
		// path ignores it (rerank runs only in Ask/pack), so Hybrid does not
		// change the stream hits — it only labels the diagnostics mode upstream.
		EnableRerank: req.Hybrid,
	})
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("ragsvc: stream retrieve: %w", err)
	}
	// Render the grounded prompt with rag's own default QA template over the
	// retrieved hits — same template rag.System.askRound renders by default,
	// so the prompt shape stays faithful to the non-stream path.
	genReq, err := ragprompt.DefaultQATemplate{}.Render(ctx, ragprompt.RenderContext{
		Question:  question,
		Namespace: req.Namespace,
		Hits:      hits,
	})
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("ragsvc: stream render: %w", err)
	}
	llmReq := llm.Request{SystemPrompt: genReq.SystemPrompt}
	for _, m := range genReq.Messages {
		llmReq.Messages = append(llmReq.Messages, llm.Message{Role: m.Role, Content: m.Content})
	}
	sr, err := s.model.Stream(ctx, llmReq)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("ragsvc: stream model: %w", err)
	}
	defer sr.Close()
	for {
		ev, err := sr.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			span.RecordError(err)
			return fmt.Errorf("ragsvc: stream next: %w", err)
		}
		if ev.Kind == llm.EventTextDelta && ev.Text != "" {
			if cbErr := emit(StreamEvent{Kind: StreamEventToken, Text: ev.Text}); cbErr != nil {
				return cbErr
			}
		}
		// EventDone / tool / thinking deltas: ignored for the QA stream.
	}
	cites := make([]StreamCitation, 0, len(hits))
	for _, h := range hits {
		cites = append(cites, StreamCitation{
			ChunkID:     h.Chunk.ID,
			DocID:       h.Chunk.DocID,
			Title:       h.Chunk.Title,
			SectionPath: append([]string(nil), h.Chunk.SectionPath...),
			Score:       h.Score,
			Snippet:     h.Chunk.Content,
		})
	}
	return emit(StreamEvent{Kind: StreamEventDone, Citations: cites, HitCount: len(hits)})
}
```

- [ ] **Step 4: Add StreamAnswer to the RagPort interface**

In `internal/ragsvc/ragsvc.go`, inside the `RagPort interface` block, add the method right after the `Retrieve` line (after `internal/ragsvc/ragsvc.go:54`):
```go
	// StreamAnswer runs the M5a single-pass grounded streaming answer
	// (Option A): retrieve → render rag's default QA prompt → model.Stream,
	// delivering token deltas + a terminal done event to emit. Bypasses rag's
	// reflection/grader orchestration by design (the non-stream Ask keeps it).
	StreamAnswer(ctx context.Context, question string, req StreamRequest, emit func(StreamEvent) error) error
```

- [ ] **Step 5: Run the test to verify it passes**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb && GOWORK=off go test ./internal/ragsvc/ -run TestStreamAnswerEmitsTokensThenDone -v
```
Expected: `--- PASS: TestStreamAnswerEmitsTokensThenDone` then `ok`.

- [ ] **Step 6: Whole-module vet (interface widened → fakes must still compile)**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb && GOWORK=off go vet ./...
```
Expected: FAIL — `internal/retrieval/retrieval_test.go`'s `*fakeRag` no longer satisfies `ragsvc.RagPort` (missing `StreamAnswer`). This is expected; Task 4 fixes the fake. Do NOT fix it here — commit the ragsvc unit first.

- [ ] **Step 7: Commit**

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb && git add internal/ragsvc/stream.go internal/ragsvc/stream_test.go internal/ragsvc/ragsvc.go && git commit -m "feat(ragsvc): StreamAnswer seam for real token streaming

Option A: retrieve + rag default QA prompt + ChatModel.Stream. Emits
kb-local token/done events so retrieval/httpapi never import rag stream
types. ragsvc stays the sole rag/* importer (spec §4)."
```

---

### Task 3: retrieval AskStream — map events to wire DTOs + persist (TDD)

**Files:**
- Modify: `internal/retrieval/retrieval.go`
- Modify: `internal/retrieval/retrieval_test.go`

Design note: `AskStream` accepts a `StreamCallback` the handler fills. It forwards each `token` (wire `{text}`), accumulates the answer text, and on `done` derives wire `Citation`s (reusing the existing `truncate` for snippet length), persists the q/a pair via the M4 `Recorder` (reusing `persist`), and forwards the terminal `done` (with `sessionId`). Mode parsing mirrors `Ask` (vector|hybrid only).

- [ ] **Step 1: Write the failing test**

Add to `internal/retrieval/retrieval_test.go` — first extend `fakeRag` with `StreamAnswer` (place after the `Retrieve` method at line ~33):
```go
func (f *fakeRag) StreamAnswer(_ context.Context, _ string, req ragsvc.StreamRequest, emit func(ragsvc.StreamEvent) error) error {
	f.gotStreamReq = req
	if err := emit(ragsvc.StreamEvent{Kind: ragsvc.StreamEventToken, Text: "hello "}); err != nil {
		return err
	}
	if err := emit(ragsvc.StreamEvent{Kind: ragsvc.StreamEventToken, Text: "world"}); err != nil {
		return err
	}
	return emit(ragsvc.StreamEvent{
		Kind:      ragsvc.StreamEventDone,
		HitCount:  1,
		Citations: []ragsvc.StreamCitation{{ChunkID: "c1", DocID: "d1", Title: "Doc One", SectionPath: []string{"Intro"}, Score: 0.9, Snippet: "a long snippet of source content"}},
	})
}
```
Add the field to the `fakeRag` struct (with the other fields at the top):
```go
	gotStreamReq ragsvc.StreamRequest
```
Add a recording recorder + the test (append at the end of the file):
```go
type recordingRecorder struct {
	ensuredSession string
	appendedAnswer string
	appendedCites  []byte
	appendedMode   string
}

func (r *recordingRecorder) EnsureSession(_ context.Context, _, _, sessionID, _ string) (string, error) {
	if sessionID != "" {
		r.ensuredSession = sessionID
		return sessionID, nil
	}
	r.ensuredSession = "new-sid"
	return "new-sid", nil
}
func (r *recordingRecorder) AppendPair(_ context.Context, _, _, answer string, citationsJSON []byte, mode string) error {
	r.appendedAnswer = answer
	r.appendedCites = citationsJSON
	r.appendedMode = mode
	return nil
}

func TestAskStreamForwardsTokensPersistsAndEmitsDone(t *testing.T) {
	f := &fakeRag{}
	svc := New(f, Config{MaxAskTokens: 4096, SnippetChars: 10})
	rec := &recordingRecorder{}
	svc.SetRecorder(rec)

	var tokens []string
	var done StreamDone
	var doneSeen bool
	err := svc.AskStream(context.Background(),
		AskInput{Namespace: "ns1", KBID: "kb1", UserID: "u1", Question: "q", Mode: "hybrid", TopK: 5},
		StreamCallback{
			OnToken: func(text string) error { tokens = append(tokens, text); return nil },
			OnDone:  func(d StreamDone) error { doneSeen = true; done = d; return nil },
		})
	if err != nil {
		t.Fatalf("AskStream: %v", err)
	}
	if !f.gotStreamReq.Hybrid || f.gotStreamReq.TopK != 5 || f.gotStreamReq.Namespace != "ns1" {
		t.Fatalf("stream req not mapped: %+v", f.gotStreamReq)
	}
	if len(tokens) != 2 || tokens[0] != "hello " || tokens[1] != "world" {
		t.Fatalf("tokens = %v, want [hello  world]", tokens)
	}
	if !doneSeen {
		t.Fatal("expected OnDone")
	}
	if len(done.Citations) != 1 || done.Citations[0].ChunkID != "c1" {
		t.Fatalf("done citations = %+v", done.Citations)
	}
	if done.Citations[0].Snippet != "a long sni" { // truncated to SnippetChars=10
		t.Fatalf("snippet not truncated to 10: %q", done.Citations[0].Snippet)
	}
	if done.Diagnostics["mode"] != "hybrid" || done.Diagnostics["hitCount"] != 1 {
		t.Fatalf("done diagnostics = %v", done.Diagnostics)
	}
	if done.SessionID != "new-sid" {
		t.Fatalf("done sessionId = %q, want new-sid", done.SessionID)
	}
	// Persistence: accumulated answer + mode recorded.
	if rec.appendedAnswer != "hello world" {
		t.Fatalf("persisted answer = %q, want %q", rec.appendedAnswer, "hello world")
	}
	if rec.appendedMode != "hybrid" {
		t.Fatalf("persisted mode = %q, want hybrid", rec.appendedMode)
	}
}

func TestAskStreamRejectsBadMode(t *testing.T) {
	svc := New(&fakeRag{}, Config{})
	err := svc.AskStream(context.Background(), AskInput{Mode: "global"}, StreamCallback{
		OnToken: func(string) error { return nil },
		OnDone:  func(StreamDone) error { return nil },
	})
	if err == nil {
		t.Fatal("expected error for unsupported mode global")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb && GOWORK=off go test ./internal/retrieval/ -run TestAskStream -v
```
Expected: COMPILE FAIL — `undefined: StreamDone`, `undefined: StreamCallback`, `svc.AskStream undefined`.

- [ ] **Step 3: Implement AskStream + types in retrieval.go**

Append to `internal/retrieval/retrieval.go` (after the `AskDrift` method, before `func truncate`):
```go
// StreamDone is the terminal payload delivered to StreamCallback.OnDone: the
// citations, diagnostics, and the ensured/created session id.
type StreamDone struct {
	Citations   []Citation
	Diagnostics map[string]any
	SessionID   string
}

// StreamCallback receives streaming-ask events. OnToken fires once per answer
// delta (concatenation yields the full answer); OnDone fires once at the end
// with citations + diagnostics + sessionId. Returning a non-nil error from
// either aborts the stream (the handler uses this for client disconnect).
type StreamCallback struct {
	OnToken func(text string) error
	OnDone  func(StreamDone) error
}

// AskStream runs the M5a single-pass streaming ask: it forwards token deltas
// to cb.OnToken, accumulates the answer, then on completion derives citations,
// persists the q/a pair (reusing the M4 Recorder), and delivers cb.OnDone.
// Modes other than vector/hybrid are rejected (global/drift are not streamed
// in M5a). The accumulated answer is persisted AFTER the stream completes.
func (s *Service) AskStream(ctx context.Context, in AskInput, cb StreamCallback) error {
	var hybrid bool
	switch in.Mode {
	case "hybrid":
		hybrid = true
	case "vector":
		hybrid = false
	default:
		return fmt.Errorf("retrieval: unsupported mode %q (stream supports vector|hybrid)", in.Mode)
	}
	topK := in.TopK
	if topK <= 0 {
		topK = 5
	}
	var answer string
	var out StreamDone
	err := s.rag.StreamAnswer(ctx, in.Question, ragsvc.StreamRequest{
		Namespace: in.Namespace,
		TopK:      topK,
		Hybrid:    hybrid,
	}, func(ev ragsvc.StreamEvent) error {
		switch ev.Kind {
		case ragsvc.StreamEventToken:
			answer += ev.Text
			return cb.OnToken(ev.Text)
		case ragsvc.StreamEventDone:
			cites := make([]Citation, 0, len(ev.Citations))
			for _, c := range ev.Citations {
				cites = append(cites, Citation{
					ChunkID:     c.ChunkID,
					DocID:       c.DocID,
					Title:       c.Title,
					SectionPath: c.SectionPath,
					Score:       c.Score,
					Snippet:     truncate(c.Snippet, s.cfg.SnippetChars),
				})
			}
			out = StreamDone{
				Citations: cites,
				Diagnostics: map[string]any{
					"mode":     in.Mode,
					"hitCount": ev.HitCount,
				},
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	// Persist the accumulated answer as a qa_message pair (reuse M4 seam).
	persisted := AskOutput{Answer: answer, Citations: out.Citations}
	sid, perr := s.persist(ctx, in.KBID, in.UserID, in.SessionID, in.Question, in.Mode, persisted)
	if perr != nil {
		return perr
	}
	out.SessionID = sid
	return cb.OnDone(out)
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb && GOWORK=off go test ./internal/retrieval/ -run TestAskStream -v
```
Expected: `--- PASS: TestAskStreamForwardsTokensPersistsAndEmitsDone` and `--- PASS: TestAskStreamRejectsBadMode`, then `ok`.

- [ ] **Step 5: Whole-package + ragsvc vet (fake now satisfies RagPort)**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb && GOWORK=off go vet ./internal/retrieval/ ./internal/ragsvc/ && GOWORK=off go test ./internal/retrieval/ ./internal/ragsvc/
```
Expected: vet prints nothing; tests `ok` for both packages.

- [ ] **Step 6: Commit**

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb && git add internal/retrieval/retrieval.go internal/retrieval/retrieval_test.go && git commit -m "feat(retrieval): AskStream maps stream events to wire DTOs + persists

Forwards token deltas, accumulates the answer, derives citations, and
records the q/a pair via the M4 Recorder on completion. fakeRag widened
to satisfy the new RagPort.StreamAnswer."
```

---

### Task 4: httpapi SSE endpoint (TDD)

**Files:**
- Modify: `internal/httpapi/httpapi.go` (Asker interface + route registration)
- Modify: `internal/httpapi/handlers.go` (`askStreamHandler` + `retrieval` import)
- Create: `internal/httpapi/stream_test.go`

Design note: reuse the EXACT `progressHandler` SSE pattern (http.Flusher assertion, `text/event-stream` + `no-cache` + `keep-alive` headers, flush per event). The handler decodes the same body as `askHandler`, builds the `AskInput` with namespace `"kb_"+id`, then calls `Asker.AskStream`, writing `event: token` / `event: done` / `event: error` frames. Client disconnect is handled by `r.Context()` (already plumbed into `AskStream` → `StreamAnswer` → `model.Stream`, whose `Next()` returns `ctx.Err()`; the callbacks also surface write failures). Body-decode/mode errors that occur BEFORE any SSE byte return a normal HTTP 400.

- [ ] **Step 1: Write the failing test**

Create `internal/httpapi/stream_test.go`:
```go
package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/costa92/llm-agent-kb/internal/retrieval"
)

// fakeStreamAsker satisfies Asker; only AskStream is exercised here.
type fakeStreamAsker struct {
	gotInput retrieval.AskInput
	tokens   []string
	done     retrieval.StreamDone
	streamErr error
}

func (f *fakeStreamAsker) Ask(context.Context, retrieval.AskInput) (retrieval.AskOutput, error) {
	return retrieval.AskOutput{}, nil
}
func (f *fakeStreamAsker) AskGlobal(context.Context, retrieval.GlobalInput) (retrieval.AskOutput, error) {
	return retrieval.AskOutput{}, nil
}
func (f *fakeStreamAsker) AskDrift(context.Context, retrieval.DriftInput) (retrieval.AskOutput, error) {
	return retrieval.AskOutput{}, nil
}
func (f *fakeStreamAsker) AskStream(_ context.Context, in retrieval.AskInput, cb retrieval.StreamCallback) error {
	f.gotInput = in
	if f.streamErr != nil {
		return f.streamErr
	}
	for _, tok := range f.tokens {
		if err := cb.OnToken(tok); err != nil {
			return err
		}
	}
	return cb.OnDone(f.done)
}

func TestAskStreamHandlerEmitsSSE(t *testing.T) {
	asker := &fakeStreamAsker{
		tokens: []string{"hello ", "world"},
		done: retrieval.StreamDone{
			Citations:   []retrieval.Citation{{ChunkID: "c1", DocID: "d1", Title: "Doc", Score: 0.9, Snippet: "snip"}},
			Diagnostics: map[string]any{"mode": "hybrid", "hitCount": 1},
			SessionID:   "sid-1",
		},
	}
	// Drive the bare handler (no auth chain) — RBAC is covered by the
	// route-wiring + e2e; this isolates the SSE framing. PathValue("id") is
	// set via SetPathValue so the handler builds namespace "kb_demo".
	req := httptest.NewRequest("POST", "/api/kb/demo/ask/stream", strings.NewReader(`{"q":"fox","mode":"hybrid","topK":5}`))
	req.SetPathValue("id", "demo")
	rec := httptest.NewRecorder()
	askStreamHandler(asker)(rec, req)

	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Fatalf("Cache-Control = %q, want no-cache", cc)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "event: token\ndata: {\"text\":\"hello \"}\n\n") {
		t.Fatalf("missing first token frame; body=\n%s", body)
	}
	if !strings.Contains(body, "event: token\ndata: {\"text\":\"world\"}\n\n") {
		t.Fatalf("missing second token frame; body=\n%s", body)
	}
	if !strings.Contains(body, "event: done\ndata: ") || !strings.Contains(body, "\"sessionId\":\"sid-1\"") {
		t.Fatalf("missing/incomplete done frame; body=\n%s", body)
	}
	if !strings.Contains(body, "\"citations\":[") || !strings.Contains(body, "\"chunkId\":\"c1\"") {
		t.Fatalf("done frame missing citations; body=\n%s", body)
	}
	if asker.gotInput.Namespace != "kb_demo" {
		t.Fatalf("namespace = %q, want kb_demo", asker.gotInput.Namespace)
	}
}

func TestAskStreamHandlerBadBodyReturns400(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/kb/demo/ask/stream", strings.NewReader(`not json`))
	req.SetPathValue("id", "demo")
	rec := httptest.NewRecorder()
	askStreamHandler(&fakeStreamAsker{})(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb && GOWORK=off go test ./internal/httpapi/ -run TestAskStreamHandler -v
```
Expected: COMPILE FAIL — `askStreamHandler undefined` and `f.AskStream` makes `*fakeStreamAsker` satisfy `Asker` only once the interface is widened. (`undefined: askStreamHandler`.)

- [ ] **Step 3: Widen the Asker interface**

In `internal/httpapi/httpapi.go`, in the `Asker interface` block (after the `AskDrift` line, ~line 27), add:
```go
	AskStream(ctx context.Context, in retrieval.AskInput, cb retrieval.StreamCallback) error
```

- [ ] **Step 4: Add the askStreamHandler**

Place `askStreamHandler` in `internal/httpapi/handlers.go` (the SSE sibling of `progressHandler`), append at the end of the file. `handlers.go` already imports `io`, `encoding/json`, `net/http`, and `authzhttp` — but NOT `retrieval`, so ADD `"github.com/costa92/llm-agent-kb/internal/retrieval"` to handlers.go's import block first. (httpapi.go imports `retrieval` but NOT `io`, which is why the handler goes in handlers.go.)
```go
// askStreamHandler streams a grounded answer as Server-Sent Events (M5a,
// viewer+). Wire contract: zero+ `event: token` frames ({"text":...}) then a
// terminal `event: done` ({citations,diagnostics,sessionId}) or `event: error`.
// Reuses the progressHandler SSE pattern: Flusher + text/event-stream headers,
// flush per frame, client disconnect via r.Context() (plumbed through AskStream
// → StreamAnswer → model.Stream). Body/mode errors before the first frame are
// a normal HTTP 400; failures after headers are sent become an error frame.
func askStreamHandler(asker Asker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Q         string `json:"q"`
			Mode      string `json:"mode"`
			TopK      int    `json:"topK"`
			SessionID string `json:"sessionId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		in := retrieval.AskInput{
			Namespace: "kb_" + r.PathValue("id"),
			KBID:      r.PathValue("id"),
			UserID:    authzhttp.UserID(r.Context()),
			SessionID: req.SessionID,
			Question:  req.Q,
			Mode:      req.Mode,
			TopK:      req.TopK,
		}
		writeFrame := func(event string, payload any) error {
			data, _ := json.Marshal(payload)
			if _, err := io.WriteString(w, "event: "+event+"\ndata: "+string(data)+"\n\n"); err != nil {
				return err
			}
			flusher.Flush()
			return nil
		}
		err := asker.AskStream(r.Context(), in, retrieval.StreamCallback{
			OnToken: func(text string) error {
				return writeFrame("token", map[string]any{"text": text})
			},
			OnDone: func(d retrieval.StreamDone) error {
				return writeFrame("done", map[string]any{
					"citations":   d.Citations,
					"diagnostics": d.Diagnostics,
					"sessionId":   d.SessionID,
				})
			},
		})
		if err != nil {
			// Headers are already sent (text/event-stream); surface the failure
			// as an SSE error frame rather than a (now-impossible) HTTP status.
			_ = writeFrame("error", map[string]any{"error": err.Error()})
		}
	}
}
```

- [ ] **Step 5: Run the handler test to verify it passes**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb && GOWORK=off go test ./internal/httpapi/ -run TestAskStreamHandler -v
```
Expected: `--- PASS: TestAskStreamHandlerEmitsSSE` and `--- PASS: TestAskStreamHandlerBadBodyReturns400`, then `ok`.

- [ ] **Step 6: Register the route**

In `internal/httpapi/httpapi.go`, in `NewMux`, immediately after the non-stream ask route registration (after `internal/httpapi/httpapi.go:150` — the `POST /api/kb/{id}/ask` line), add:
```go
	// Streaming Q&A (M5a) — viewer+, same chain + kb scope as the non-stream ask.
	mux.Handle("POST /api/kb/{id}/ask/stream", chain(authzrole.RoleViewer, askStreamHandler(d.Asker)))
```

- [ ] **Step 7: Whole-module vet + full test (Asker widened; retrievalSvc must satisfy it)**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb && GOWORK=off go vet ./... && GOWORK=off go test ./...
```
Expected: vet prints nothing (`*retrieval.Service` satisfies the widened `Asker` because Task 3 added `AskStream`; `cmd/kbd` needs no change); all packages `ok`/`(cached)`, gated PG tests `SKIP`.

- [ ] **Step 8: Commit**

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb && git add internal/httpapi/httpapi.go internal/httpapi/handlers.go internal/httpapi/stream_test.go && git commit -m "feat(httpapi): POST /api/kb/{id}/ask/stream SSE endpoint

Token/done/error SSE frames over the progressHandler flush pattern;
viewer+ via the same chain + kbScope as /ask. Defines the M5b frontend
wire contract."
```

---

### Task 5: Gated streaming e2e (TDD over live pgvector)

**Files:**
- Modify: `cmd/kbd/main_test.go`

Design note: mirror `TestEndToEndLoginCreateKBUploadAskDelete` exactly through the upload→ready step, then issue a raw streaming POST (NOT the JSON-decoding `do` helper) and parse the SSE frames. The scripted model's `Stream` emits one `EventTextDelta` (the response Text) then `EventDone`, so the e2e asserts ≥1 `token` frame, exactly one `done` frame with ≥1 citation + a sessionId, and that the session row persisted. Gated on `LLM_AGENT_KB_PG_URL`.

- [ ] **Step 1: Write the failing test**

Append to `cmd/kbd/main_test.go` (end of file). The `bufio` import must be added to the import block (`"bufio"`):
```go
// TestStreamingAskEndToEnd drives POST /api/kb/{id}/ask/stream over live
// pgvector: login → org → kb → upload paste → poll ready → open the SSE stream
// → assert ≥1 token frame + a terminal done frame with ≥1 citation + sessionId,
// then assert the q/a pair persisted (qa_session row exists). Providers are
// scripted: ScriptedLLM.Stream emits one EventTextDelta then EventDone, so the
// answer arrives as a single token frame. Gated on LLM_AGENT_KB_PG_URL.
func TestStreamingAskEndToEnd(t *testing.T) {
	dsn := os.Getenv("LLM_AGENT_KB_PG_URL")
	if dsn == "" {
		t.Skipf("set LLM_AGENT_KB_PG_URL (pgvector) to run the streaming e2e")
	}
	ctx := context.Background()
	cleanDB(t, ctx, dsn)

	providerOverride = func(config.Config) (llm.ChatModel, llm.Embedder, error) {
		return llm.NewScriptedLLM(llm.WithResponses(llm.Response{Text: "scripted streamed answer"})),
			llm.NewScriptedLLM(llm.WithEmbedDimensions(8)), nil
	}
	t.Cleanup(func() { providerOverride = nil })

	cfg, err := config.LoadFromLookup(func(k string) (string, bool) {
		switch k {
		case "PG_URL":
			return dsn, true
		case "EMBEDDING_DIM":
			return "8", true
		case "JWT_SECRET":
			return "test-secret", true
		case "GRAPH_ENABLED":
			return "false", true
		}
		return "", false
	})
	if err != nil {
		t.Fatal(err)
	}
	handler, cleanup, err := build(ctx, cfg)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	defer cleanup()
	srv := httptest.NewServer(handler)
	defer srv.Close()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	hash, err := password.Hash("pw")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authzstore.New(pool).CreateUser(ctx, "stream@x.com", hash); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	client := srv.Client()
	do := func(method, path, bearer, body string) (int, map[string]any) {
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		if m == nil {
			m = map[string]any{"_raw": string(raw)}
		}
		return resp.StatusCode, m
	}

	code, body := do("POST", "/api/auth/login", "", `{"Email":"stream@x.com","Password":"pw"}`)
	if code != http.StatusOK {
		t.Fatalf("login code=%d body=%v want 200", code, body)
	}
	token, _ := body["access_token"].(string)
	if token == "" {
		t.Fatalf("login returned no access_token: %v", body)
	}

	code, body = do("POST", "/api/orgs", token, `{"name":"Acme"}`)
	if code != http.StatusOK {
		t.Fatalf("create org code=%d body=%v want 200", code, body)
	}
	orgID, _ := body["id"].(string)

	code, body = do("POST", "/api/orgs/"+orgID+"/kbs", token, `{"name":"Docs","embeddingDim":8}`)
	if code != http.StatusOK {
		t.Fatalf("create kb code=%d body=%v want 200", code, body)
	}
	kbID, _ := body["id"].(string)
	if kbID == "" {
		t.Fatalf("create kb returned no id: %v", body)
	}

	code, body = do("POST", "/api/kb/"+kbID+"/documents", token,
		`{"title":"Doc","sourceType":"paste","content":"the quick brown fox jumps over the lazy dog repeatedly"}`)
	if code != http.StatusAccepted {
		t.Fatalf("upload code=%d body=%v want 202", code, body)
	}
	docID, _ := body["documentId"].(string)
	if docID == "" {
		t.Fatalf("upload returned no documentId: %v", body)
	}

	ready := false
	for i := 0; i < 50; i++ {
		code, listBody := do("GET", "/api/kb/"+kbID+"/documents", token, "")
		if code == http.StatusOK && hasReadyDoc(listBody, docID) {
			ready = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ready {
		t.Fatalf("document %s never reached ready", docID)
	}

	// Open the SSE stream and parse frames.
	streamReq, _ := http.NewRequest("POST", srv.URL+"/api/kb/"+kbID+"/ask/stream",
		strings.NewReader(`{"q":"fox","mode":"hybrid","topK":5}`))
	streamReq.Header.Set("Content-Type", "application/json")
	streamReq.Header.Set("Authorization", "Bearer "+token)
	streamResp, err := client.Do(streamReq)
	if err != nil {
		t.Fatalf("ask/stream: %v", err)
	}
	defer streamResp.Body.Close()
	if streamResp.StatusCode != http.StatusOK {
		t.Fatalf("ask/stream code=%d want 200", streamResp.StatusCode)
	}
	if ct := streamResp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("ask/stream Content-Type=%q want text/event-stream", ct)
	}

	var tokenCount int
	var doneData map[string]any
	var curEvent string
	sc := bufio.NewScanner(streamResp.Body)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			curEvent = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data := strings.TrimPrefix(line, "data: ")
			switch curEvent {
			case "token":
				tokenCount++
			case "done":
				_ = json.Unmarshal([]byte(data), &doneData)
			case "error":
				t.Fatalf("stream emitted error frame: %s", data)
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan SSE: %v", err)
	}
	if tokenCount == 0 {
		t.Fatal("expected at least one token frame")
	}
	if doneData == nil {
		t.Fatal("expected a terminal done frame")
	}
	cites, _ := doneData["citations"].([]any)
	if len(cites) == 0 {
		t.Fatalf("done frame returned 0 citations: %v", doneData)
	}
	sid, _ := doneData["sessionId"].(string)
	if sid == "" {
		t.Fatalf("done frame missing sessionId: %v", doneData)
	}

	// Persistence: the streamed q/a pair created a session row for this kb.
	var sessions int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM qa_session WHERE kb_id = $1`, kbID).Scan(&sessions); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if sessions == 0 {
		t.Fatal("streamed ask did not persist a qa_session row")
	}
}
```

- [ ] **Step 2: Add the bufio import**

In `cmd/kbd/main_test.go`, add `"bufio"` to the import block (first line of the stdlib group, before `"context"`).

- [ ] **Step 3: Run the e2e to verify it passes (live PG)**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb && LLM_AGENT_KB_PG_URL='postgres://postgres:pw@172.17.0.3:5432/postgres?sslmode=disable' GOWORK=off go test ./cmd/kbd/ -run TestStreamingAskEndToEnd -v
```
Expected: `--- PASS: TestStreamingAskEndToEnd`, then `ok`. (Container `kb_m3_pg` must be up. If the container is down, the test would FAIL to connect, not skip — bring it up first.)

- [ ] **Step 4: Confirm it still SKIPs without the env var (gating)**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb && GOWORK=off go test ./cmd/kbd/ -run TestStreamingAskEndToEnd -v
```
Expected: `--- SKIP: TestStreamingAskEndToEnd` with the `set LLM_AGENT_KB_PG_URL` message.

- [ ] **Step 5: Commit**

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb && git add cmd/kbd/main_test.go && git commit -m "test(e2e): gated streaming-ask SSE end-to-end

login→kb→upload→ready→POST /ask/stream → parse SSE frames → assert
token+done frames, citations, sessionId, and a persisted qa_session row.
Gated on LLM_AGENT_KB_PG_URL; scripted ChatModel.Stream backs it."
```

---

### Task 6: Final verification + merge (tag DEFERRED to M5b)

**Files:** none (verification + integration)

**Tagging decision:** Do NOT tag `v0.5.0` at the end of M5a. M5a ships the streaming BACKEND only; the `/ask/stream` wire contract is consumed by the M5b frontend, and the contract may need a confirming tweak once the frontend integrates. Recommendation: M5a merges to `main` (the streaming endpoint is independently complete and tested), and the `v0.5.0` tag is cut at the M5b close-out so the released version reflects the full streaming feature (backend + frontend). State this in the merge/PR description.

- [ ] **Step 1: Full module build + vet + test (no gated)**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb && GOWORK=off go build ./... && GOWORK=off go vet ./... && GOWORK=off go test ./...
```
Expected: build silent; vet silent; all packages `ok`/`(cached)`; gated PG tests `SKIP`.

- [ ] **Step 2: Full test WITH live PG (gated suite green)**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb && LLM_AGENT_KB_PG_URL='postgres://postgres:pw@172.17.0.3:5432/postgres?sslmode=disable' GOWORK=off go test ./... -count=1
```
Expected: all packages `ok`, including `cmd/kbd` (all e2e tests, the new streaming one among them, PASS). Confirm container `kb_m3_pg` is up first.

- [ ] **Step 3: Confirm no dependency drift (do NOT run go mod tidy)**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb && git diff --stat go.mod go.sum
```
Expected: EMPTY output (no go.mod/go.sum changes — streaming uses already-pinned `llm-agent-contract@v0.5.0` + `llm-agent-rag@v1.11.0`). If non-empty, STOP and investigate — no version bump is expected for M5a.

- [ ] **Step 4: Confirm the §4 boundary is intact (ragsvc is the sole rag/* importer)**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb && grep -rl "llm-agent-rag/" internal/ cmd/ --include=*.go | grep -v "_test.go" | grep -v "internal/ragsvc/"
```
Expected: EMPTY output (only `internal/ragsvc/*.go` non-test files import `llm-agent-rag/*`). The new `internal/ragsvc/stream.go` is the only added rag importer; `retrieval`/`httpapi` see kb-local types only. (Test files like `retrieval_test.go` legitimately import rag types for fixtures — they are excluded.)

- [ ] **Step 5: Merge to main (no tag)**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb && git checkout main && git merge --no-ff feat/m5a-streaming -m "Merge M5a: real token-streaming ask backend (POST /api/kb/{id}/ask/stream)

Option A token streaming via ChatModel.Stream over rag-retrieved,
rag-prompt-rendered context. SSE token/done/error wire contract for the
M5b frontend. Persists the streamed q/a pair. v0.5.0 tag DEFERRED to the
M5b close-out (frontend consumes this contract)."
```
Expected: a merge commit on `main`. Do NOT run `git tag v0.5.0` — the tag is deferred to M5b close-out (see the tagging decision above).

---

## Self-Review

**1. Spec coverage (M5a SCOPE 1–5):**
- (1) Streaming ask path + narrow ragsvc seam → Task 2 (`StreamAnswer` on `RagPort`, kb-local event types) + Task 3 (`AskStream` on retrieval). §4 boundary preserved: kb-local `StreamEvent`/`StreamCitation`/`StreamDone`, only `ragsvc/stream.go` imports rag. ✅
- (2) SSE endpoint `POST /api/kb/{id}/ask/stream`, RBAC viewer+ via same `chain()` + `kbScopeFromRequest`, SSE headers, flush per event, disconnect via `r.Context()`, exact wire format → Task 4 + the wire-contract section. ✅ (Chose POST to match the non-stream `/ask` body; documented.)
- (3) Session persistence reusing M4 Recorder, accumulate streamed text, persist on completion → Task 3 `AskStream` (accumulates `answer`, calls `persist`). ✅
- (4) cmd/kbd wiring (none needed — `retrievalSvc` satisfies widened `Asker`; verified in Task 4 Step 7) + gated e2e with scripted Stream → Task 5. ✅
- (5) Final build/vet/test + merge, tag deferred to M5b → Task 6 (explicit no-tag decision). ✅

**2. Placeholder scan:** No TBD/TODO/"add error handling"/"similar to". Every code step is complete; every command has expected output. ✅

**3. Type consistency:** `StreamEvent{Kind, Text, Citations, HitCount}` + `StreamEventKind{StreamEventToken, StreamEventDone}` + `StreamCitation` defined in Task 2 are used identically in Tasks 3–5. `StreamRequest{Namespace, TopK, Hybrid}` consistent (ragsvc + fakeRag + retrieval). `StreamCallback{OnToken func(string)error, OnDone func(StreamDone)error}` + `StreamDone{Citations, Diagnostics, SessionID}` defined in Task 3, used identically in Task 4 handler + tests. `AskStream(ctx, AskInput, StreamCallback) error` signature identical across retrieval impl, Asker interface, fakeStreamAsker, e2e. Citation wire fields (`chunkId/docId/title/sectionPath/score/snippet`) match the M4 `retrieval.Citation` json tags. `emit func(StreamEvent) error` callback signature consistent between `RagPort.StreamAnswer` and the impl. ✅

**Verified against source:** `llm.ChatModel.Stream` is part of the interface (chatmodel.go:19); `ScriptedLLM.Stream` emits one `EventTextDelta` (if Text != "") then `EventDone` (scripted.go:228); `rag/prompt.DefaultQATemplate.Render` is exported and is the default template rag's `askRound` renders (ask.go:418-489); `RagPort.Retrieve` + the held `model` + `wrapper.Retrieve(ctx, query, opts)` (embeds internally) all exist; `progressHandler` SSE pattern (Flusher + headers + flush) is the reuse target; `persist`/`truncate`/`SetRecorder` are the M4 seams reused.
