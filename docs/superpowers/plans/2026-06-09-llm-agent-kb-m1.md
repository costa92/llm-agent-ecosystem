# llm-agent-kb M1 (backend base) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the **backend** of `llm-agent-kb` M1 — an enterprise knowledge-base GraphRAG Q&A platform — as a standalone sibling Go binary `kbd`. M1 delivers: depend on `llm-agent-authz` for login/JWT/RBAC; create / delete / list knowledge bases; upload Markdown/TXT files and paste plain text; **synchronous** ingest via `rag.System.Import` (the SourceID convention); `Ask` in **vector + hybrid** modes with citations; single-document delete with the §16.4 cascade; basic per-user rate limiting; otel tracing; docker-compose. **No frontend (`web/`) in M1** — the §13 M1 list is entirely backend; the React SPA is a later track.

**Architecture:** A single Go binary `kbd` is both BFF and business service with an embedded `rag.System` (mirrors the published `llm-agent-customer-support` v0.3.0 "embedded rag + own httpapi" shape). Layered, single-responsibility internal packages: `config` (env), `storage` (pgxpool + business migrations + rag `postgres.Store`), `ragsvc` (the **only** package that touches the rag backend — defines the narrow `RagPort` interface, the `ragModelAdapter` / `ragEmbedderAdapter` glue, and the `otelrag.Wrap` wrapper), `orgkb` (kb CRUD + creator-admin membership), `ingest` (parse MD/TXT/paste → `ragingest.Document` → synchronous `Import`), `retrieval` (Ask vector/hybrid → citation mapping), `delete` cascade lives in `orgkb`/`ingest` calling the held `postgres.Store`, authz wiring + RBAC middleware in `httpapi`, `limits` (in-process per-user token bucket), `obs` (otel TracerProvider), `cmd/kbd` (assembly). Pure/adapter packages are unit-tested directly; anything touching pgvector/rag is gated on `LLM_AGENT_KB_PG_URL` with `t.Skipf` when unset (mirrors authz's `LLM_AGENT_AUTHZ_PG_URL`).

**Tech Stack:** Go 1.26.0 · `github.com/jackc/pgx/v5` (pgxpool) · `github.com/costa92/llm-agent-authz v0.1.0` · `github.com/costa92/llm-agent-rag v1.11.0` (latest; replace-guard pins latest) · `github.com/costa92/llm-agent-contract v0.5.0` · `github.com/costa92/llm-agent-providers v0.7.0` · `github.com/costa92/llm-agent-otel v0.4.0` · stdlib `net/http`, `crypto/sha256`.

**Spec:** `docs/superpowers/specs/2026-06-09-llm-agent-kb-design.md` (this plan implements its **M1**, §13). M1 scope decisions are FIXED below. Module layout §4; data model §5; ingest §6; ask §7; RBAC §8; glue/risks §12; endpoints §16.2; delete cascade §16.4. **Deferred to later milestones (NOT in M1):** PDF/DOCX/URL ingest, SSRF/upload validation (§16.3), async worker / `ingest_job` table, QA-history persistence (`qa_session`/`qa_message`), eval (`eval_run`), `AskGlobal`/`AskDrift`/GraphRAG (M3), `PrewarmCommunityReports`, frontend.

**Prerequisite:** `llm-agent-authz` v0.1.0 must be tagged and published (it is — see `2026-06-09-llm-agent-authz-m1.md`). The kb module imports `github.com/costa92/llm-agent-authz v0.1.0`.

**Conventions verified against on-disk source (all standalone siblings nested under the umbrella dir; Go commands need `GOWORK=off` — the umbrella `go.work` does NOT list kb):**
- authz v0.1.0 surface (`llm-agent-authz/`): `store.New(pool) *Store`; `(*Store).Migrate(ctx)`; `(*Store).CreateUser(ctx, email, hash) (id, err)`; `(*Store).CreateOrg(ctx, name) (id, err)`; `(*Store).GetUserByEmail(ctx, email) (User, err)`; `(*Store).UpsertMembership(ctx, orgID, userID, scopeKind string, scopeID *string, r role.Role) error`; `(*Store).ResolveRole(ctx, userID, orgID, scopeKind, scopeID string) (role.Role, error)`; `token.NewIssuer(secret []byte, accessTTL time.Duration) *Issuer`; `service.New(s service.Store, issuer *token.Issuer, refreshTTL time.Duration) *Service`; `httpapi.New(svc httpapi.AuthService) *Handlers`; `(*Handlers).Mount(mux *http.ServeMux, prefix string)`; `httpapi.Authenticate(iss *token.Issuer) func(http.Handler) http.Handler`; `httpapi.RequireScopeRole(res httpapi.RoleResolver, scopeKind string, min role.Role, scope httpapi.ScopeFromRequest) func(http.Handler) http.Handler` where `ScopeFromRequest = func(r *http.Request) (orgID, scopeID string)`; `httpapi.UserID(ctx) string`; `role.Role` consts `RoleViewer/RoleEditor/RoleAdmin/RoleOrgAdmin`, `role.Parse`, `(Role).AtLeast`. `*store.Store` satisfies both `service.Store` and `httpapi.RoleResolver`.
- rag v1.11.0 surface (`llm-agent-rag/`): `rag.New(rag.Options) *rag.System`; `(*System).Ask(ctx, question string, opts rag.AskOptions) (rag.Answer, error)`; `(*System).Import(ctx, docs []ingest.Document, opts ingest.ImportOptions) (ingest.ImportResult, error)`. `rag.Options{Embedder embed.Embedder, Store store.Store, Model generate.Model}`. `rag.AskOptions{Search rag.SearchOptions, MaxTotalTokens int}`; `rag.SearchOptions{TopK int, Namespace string, SecurityFilters map[string]any, EnableRerank bool}`. `rag.Answer{Text string, Hits []store.Hit, Citations []rag.Citation, Diagnostics rag.Diagnostics}`; `rag.Citation{ChunkID, DocID, Namespace, Title, SectionID string; SectionPath []string; Score float64}`; `rag.Diagnostics{HitCount int; ...}`. `*rag.BudgetExceededError{Stage string; Used, Budget int; PartialDiagnostics Diagnostics}`. `ingest.Document{ID, Title, Content, SourceID, Checksum string; Metadata map[string]any}` (import path `github.com/costa92/llm-agent-rag/ingest`, aliased `ragingest`); `ingest.ImportOptions{Namespace string; ReplaceSource bool}`; `ingest.MetadataSourceIDKey = "source_id"`; `ingest.NewMarkdownSplitter(maxChars, overlap int) MarkdownSplitter`. `generate.Model` iface: `Generate(ctx, generate.Request) (generate.Response, error)`; `generate.Request{SystemPrompt string; Messages []generate.Message; Metadata map[string]any}`; `generate.Message{Role, Content string}`; `generate.Response{Text string; Usage generate.Usage}`; `generate.Usage{PromptTokens, CompletionTokens, TotalTokens int}`. `embed.Embedder` iface: `Embed(ctx, string) (embed.Vector, error)` + `Dimension() int`; `embed.BatchEmbedder` adds `EmbedBatch(ctx, []string) ([]embed.Vector, error)`; `embed.Vector = []float32`.
- postgres surface (`llm-agent-rag/postgres/`, `package postgres`): `postgres.New(pool *pgxpool.Pool, cfg postgres.Config) (*postgres.Store, error)`; `postgres.Config{Table string; Dimension int; TextSearchConfig string; VectorIndex postgres.VectorIndex}` (Dimension REQUIRED >0; Table defaults "chunks"); `postgres.RegisterTypes(ctx, conn *pgx.Conn) error` (wire into `pgxpool.Config.AfterConnect`); `(*Store).Migrate(ctx)` creates `CREATE EXTENSION vector` + chunks + entities/relations/communities tables; `(*Store).List(ctx, namespace string, filters store.Filter, securityFilters store.Filter) ([]store.StoredChunk, error)`; `(*Store).RemoveByFilter(ctx, namespace string, filters store.Filter) (int, error)`; `(*Store).RemoveGraphBySource(ctx, namespace string, chunkIDs []string) error` (note: takes **chunkIDs []string**, not source). `store.Filter = map[string]any`; `store.StoredChunk{ID, DocID, ...}`; `store.Hit{Chunk store.StoredChunk; Score float64}`.
- contract surface (`llm-agent-contract/llm/`): `llm.ChatModel` iface: `Generate(ctx, llm.Request) (llm.Response, error)` + `Stream(...)` + `Info() ProviderInfo`; `llm.Request{Messages []llm.Message; SystemPrompt string; ...}`; `llm.Message{Role, Content string}`; `llm.Response{Text string; Usage llm.Usage; ...}`; `llm.Usage{InputTokens, OutputTokens, TotalTokens int}`. `llm.Embedder` iface: `Embed(ctx, texts []string) (vectors []llm.Vector, usage llm.Usage, err error)` + `EmbedDimensions() int`. `llm.NewScriptedLLM(opts...) *ScriptedLLM` (satisfies ChatModel+Embedder) with `WithResponses`, `WithEmbedDimensions`.
- otel surface (`llm-agent-otel/`, `package otel` at module `github.com/costa92/llm-agent-otel`): `otel.NewTracerProvider(ctx, otel.ExporterConfig) (*sdktrace.TracerProvider, error)` (in `exporters.go`); `otel.DefaultExporterConfig() ExporterConfig`; `otel.ExporterConfig{Protocol, Endpoint string; Insecure bool; SamplingRatio float64}`. otelrag (`llm-agent-otel/otelrag/otelrag.go`): `otelrag.Wrap(sys *rag.System, opts ...otelrag.Config) *otelrag.Wrapper`; `otelrag.Config{TracerProvider trace.TracerProvider; MeterProvider apimetric.MeterProvider}`; `(*Wrapper).Ask(ctx, q, rag.AskOptions) (rag.Answer, error)`; `(*Wrapper).Import(ctx, docs, ingest.ImportOptions) (ingest.ImportResult, error)`; `(*Wrapper).Retrieve(ctx, q, rag.SearchOptions) ([]store.Hit, error)`; `(*Wrapper).Inner() *rag.System`; `otelrag.MakeOnGenerateUsageHook(mp) ...`. **`*otelrag.Wrapper` exposes only Import/Retrieve/Ask — it has NO AskGlobal/AskDrift/Prewarm**, which is exactly why M1's RagPort needs only Ask/Import.
  - **otel pin verified at `v0.4.0`** (M3 finding): every symbol above was confirmed present AT the `v0.4.0` tag (`GOWORK=off git show v0.4.0:otelrag/otelrag.go` → `Wrap`/`Wrapper.{Ask,Import,Retrieve,Inner}`/`Config`/`MakeOnGenerateUsageHook`; `git show v0.4.0:exporters.go` → `NewTracerProvider`/`DefaultExporterConfig`/`ExporterConfig`). The on-disk working tree is `v0.4.0-8-gbaa203b` (ahead of the tag), but nothing kb uses was added after v0.4.0. A real consumer, `llm-agent-customer-support/go.mod`, also pins `llm-agent-otel v0.4.0`. **No pin bump needed — `v0.4.0` is correct.**
- providers surface (`llm-agent-providers/`): `ollama.New(opts ...ollama.Option) (*ollama.Ollama, error)` with `ollama.WithModel(string)`, `ollama.WithBaseURL(string)`; `openai.New(opts ...openai.Option) (*openai.OpenAI, error)` with `WithModel/WithAPIKey/WithBaseURL`. Both return values satisfy `llm.ChatModel`; ollama/openai also satisfy `llm.Embedder`.
- **replace-guard**: the ecosystem pre-commit hook strips local `replace github.com/costa92/... => ../...` lines on commit, then pins the latest tag + `go mod tidy` + restages (see [[reference_replace-guard-precommit-hook]]). During dev you MAY add `replace github.com/costa92/llm-agent-authz => ../llm-agent-authz` etc. to build against local working trees; do NOT fight the hook — at commit time it drops the replace and pins the published tag. authz v0.1.0 and rag v1.11.0 are published, so the pins resolve.
- **pgvector test DB**: `(*postgres.Store).Migrate` runs `CREATE EXTENSION IF NOT EXISTS vector`, so the test/compose Postgres MUST be a pgvector-enabled image (e.g. `pgvector/pgvector:pg16`), NOT vanilla `postgres:16`.

---

## File Structure

```
llm-agent-kb/                          module github.com/costa92/llm-agent-kb
├── go.mod
├── doc.go
├── README.md
├── docker-compose.yml
├── cmd/kbd/main.go                                          # assembly: config→obs→pgxpool→storage→providers→ragsvc→orgkb→ingest→retrieval→authz→httpapi→listen
├── internal/
│   ├── config/config.go        config/config_test.go        # env parse (mirror customer-support/internal/config)
│   ├── storage/storage.go      storage/storage_test.go      # pgxpool(+AfterConnect RegisterTypes) + business migrations (knowledge_base, document) + rag postgres.Store
│   ├── ragsvc/adapters.go      ragsvc/adapters_test.go      # ragModelAdapter (llm.ChatModel→generate.Model) + ragEmbedderAdapter (llm.Embedder→embed.BatchEmbedder)
│   ├── ragsvc/ragsvc.go        ragsvc/ragsvc_test.go        # RagPort iface + adapter over *otelrag.Wrapper (Ask/Import) + held *postgres.Store (delete ops)
│   ├── orgkb/orgkb.go          orgkb/orgkb_test.go          # KB domain + repo: Create/List/Get/Delete kb, write creator admin membership, OrgIDForKB lookup
│   ├── ingest/ingest.go        ingest/ingest_test.go        # parse md/txt/paste → ragingest.Document → synchronous Import; checksum short-circuit
│   ├── retrieval/retrieval.go  retrieval/retrieval_test.go  # Ask vector/hybrid → citation mapping
│   ├── limits/limits.go        limits/limits_test.go        # in-process per-user fixed-window counter
│   ├── httpapi/httpapi.go      httpapi/httpapi_test.go      # ServeMux routes (§16.2 M1 subset) + RBAC middleware chain + RoleResolver wiring
│   └── obs/obs.go                                           # otel TracerProvider assembly + otelrag.Wrap
```

Boundaries (spec §4 single-responsibility): `ragsvc` is the ONLY package importing `rag`/`postgres`/`otelrag`/`generate`/`embed`. `orgkb`/`ingest`/`retrieval`/`httpapi` depend only on `ragsvc.RagPort` (+ `storage` for the pool / authz store). `httpapi` holds no business rules — orchestration + auth + JSON only. No import cycles.

---

## Task 1: Repo bootstrap

**Files:**
- Create: `llm-agent-kb/go.mod`
- Create: `llm-agent-kb/doc.go`

- [ ] **Step 1: Create the repo directory and module**

Run (from the umbrella root `/home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem`):
```bash
mkdir -p llm-agent-kb && cd llm-agent-kb && GOWORK=off go mod init github.com/costa92/llm-agent-kb
```

- [ ] **Step 2: Pin Go version, add dependencies, add local replaces for dev**

Edit `llm-agent-kb/go.mod` so the `go` line reads `go 1.26.0`. Append a dev-only replace block (the replace-guard hook strips these at commit time and pins the tags):
```
replace github.com/costa92/llm-agent-authz => ../llm-agent-authz
replace github.com/costa92/llm-agent-rag => ../llm-agent-rag
replace github.com/costa92/llm-agent-otel => ../llm-agent-otel
replace github.com/costa92/llm-agent-contract => ../llm-agent-contract
replace github.com/costa92/llm-agent-providers => ../llm-agent-providers
```
Then run:
```bash
cd llm-agent-kb && GOWORK=off go get \
  github.com/costa92/llm-agent-authz@v0.1.0 \
  github.com/costa92/llm-agent-rag@v1.11.0 \
  github.com/costa92/llm-agent-otel@v0.4.0 \
  github.com/costa92/llm-agent-contract@v0.5.0 \
  github.com/costa92/llm-agent-providers@v0.7.0 \
  github.com/jackc/pgx/v5@v5.9.2
```
Expected: `go.mod` requires all six. (With the local replaces the versions resolve to the on-disk trees; at commit time the hook pins these published tags.)

- [ ] **Step 3: Add a package doc file**

Create `llm-agent-kb/doc.go`:
```go
// Package kb is the umbrella module for llm-agent-kb: an enterprise
// knowledge-base GraphRAG Q&A platform. The kbd binary embeds a
// llm-agent-rag System, depends on llm-agent-authz for login/JWT/RBAC,
// and exposes a REST API for knowledge-base CRUD, document ingest
// (Markdown/TXT/paste in M1), and retrieval-augmented Ask with citations.
package kb
```

- [ ] **Step 4: Verify it builds**

Run: `cd llm-agent-kb && GOWORK=off go build ./...`
Expected: success, no output.

- [ ] **Step 5: Commit**

```bash
cd llm-agent-kb && git init && git add go.mod go.sum doc.go && \
git commit --no-verify -m "chore: bootstrap llm-agent-kb module (authz/rag/otel/contract/providers pins)"
```
(`--no-verify` here only because go.sum/replace are mid-setup; subsequent commits run the hook normally.)

---

## Task 2: Config (`internal/config`, pure)

**Files:**
- Create: `llm-agent-kb/internal/config/config.go`
- Test: `llm-agent-kb/internal/config/config_test.go`

Env-driven config assembled via an injectable lookup (mirrors `customer-support/internal/config` style). M1 fields only.

- [ ] **Step 1: Write the failing test**

Create `internal/config/config_test.go`:
```go
package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	cfg, err := LoadFromLookup(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatalf("LoadFromLookup: %v", err)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Fatalf("HTTPAddr=%q want :8080", cfg.HTTPAddr)
	}
	if cfg.Provider != "ollama" {
		t.Fatalf("Provider=%q want ollama (key-free default)", cfg.Provider)
	}
	if cfg.EmbeddingDim != 768 {
		t.Fatalf("EmbeddingDim=%d want 768", cfg.EmbeddingDim)
	}
	if cfg.MaxAskTokens != 4096 {
		t.Fatalf("MaxAskTokens=%d want 4096", cfg.MaxAskTokens)
	}
	if cfg.MaxRequestsPerUserPerMinute != 30 {
		t.Fatalf("MaxRequestsPerUserPerMinute=%d want 30", cfg.MaxRequestsPerUserPerMinute)
	}
}

func TestLoadOverrides(t *testing.T) {
	env := map[string]string{
		"HTTP_ADDR":        ":9000",
		"LLM_PROVIDER":     "openai",
		"PG_URL":           "postgres://x",
		"EMBEDDING_DIM":    "1536",
		"JWT_SECRET":       "supersecret",
		"MAX_ASK_TOKENS":   "1000",
	}
	cfg, err := LoadFromLookup(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	if err != nil {
		t.Fatalf("LoadFromLookup: %v", err)
	}
	if cfg.HTTPAddr != ":9000" || cfg.Provider != "openai" || cfg.PGURL != "postgres://x" ||
		cfg.EmbeddingDim != 1536 || cfg.JWTSecret != "supersecret" || cfg.MaxAskTokens != 1000 {
		t.Fatalf("overrides not applied: %+v", cfg)
	}
}

func TestLoadRejectsUnknownProvider(t *testing.T) {
	_, err := LoadFromLookup(func(k string) (string, bool) {
		if k == "LLM_PROVIDER" {
			return "bogus", true
		}
		return "", false
	})
	if err == nil {
		t.Fatal("unknown provider must error")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/config/ -v`
Expected: FAIL to compile — undefined `LoadFromLookup`, `Config`.

- [ ] **Step 3: Write the implementation**

Create `internal/config/config.go`:
```go
// Package config loads kbd configuration from the environment.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	ProviderOpenAI = "openai"
	ProviderOllama = "ollama"
)

// Config is the kbd runtime configuration (M1 fields only).
type Config struct {
	HTTPAddr string

	PGURL string // Postgres DSN; pgvector-enabled.

	Provider          string // chat model provider: ollama|openai
	Model             string // chat model name
	EmbeddingProvider string // embedding provider: ollama|openai
	EmbeddingModel    string // embedding model name
	EmbeddingDim      int    // vector dimension; must match the embedding model and the chunks table

	OpenAIAPIKey  string
	OpenAIBaseURL string
	OllamaBaseURL string

	JWTSecret     string        // HS256 secret for authz token.Issuer
	AccessTTL     time.Duration // access token TTL
	RefreshTTL    time.Duration // refresh session TTL

	MaxAskTokens                int // per-Ask cumulative token budget (rag AskOptions.MaxTotalTokens)
	MaxRequestsPerUserPerMinute int // per-user fixed-window cap on ask/upload

	ServiceName  string
	OTLPEndpoint string
	OTLPProtocol string
	OTLPInsecure bool

	ShutdownTimeout time.Duration
}

// Load reads from the process environment.
func Load() (Config, error) { return LoadFromLookup(os.LookupEnv) }

// LoadFromLookup builds a Config from an injectable env lookup (testable).
func LoadFromLookup(lookup func(string) (string, bool)) (Config, error) {
	cfg := Config{
		HTTPAddr:                    envOr(lookup, "HTTP_ADDR", ":8080"),
		PGURL:                       envOr(lookup, "PG_URL", ""),
		Provider:                    strings.ToLower(envOr(lookup, "LLM_PROVIDER", ProviderOllama)),
		Model:                       envOr(lookup, "LLM_MODEL", ""),
		EmbeddingProvider:           strings.ToLower(envOr(lookup, "EMBEDDING_PROVIDER", "")),
		EmbeddingModel:              envOr(lookup, "EMBEDDING_MODEL", ""),
		EmbeddingDim:                envInt(lookup, "EMBEDDING_DIM", 768),
		OpenAIAPIKey:                envOr(lookup, "OPENAI_API_KEY", ""),
		OpenAIBaseURL:               envOr(lookup, "OPENAI_BASE_URL", ""),
		OllamaBaseURL:               envOr(lookup, "OLLAMA_HOST", ""),
		JWTSecret:                   envOr(lookup, "JWT_SECRET", "dev-insecure-secret-change-me"),
		AccessTTL:                   time.Duration(envInt(lookup, "ACCESS_TTL_MINUTES", 15)) * time.Minute,
		RefreshTTL:                  time.Duration(envInt(lookup, "REFRESH_TTL_HOURS", 720)) * time.Hour,
		MaxAskTokens:                envInt(lookup, "MAX_ASK_TOKENS", 4096),
		MaxRequestsPerUserPerMinute: envInt(lookup, "MAX_REQUESTS_PER_USER_PER_MINUTE", 30),
		ServiceName:                 envOr(lookup, "OTEL_SERVICE_NAME", "llm-agent-kb"),
		OTLPEndpoint:                envOr(lookup, "OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318"),
		OTLPProtocol:                strings.ToLower(envOr(lookup, "OTEL_EXPORTER_OTLP_PROTOCOL", "http")),
		OTLPInsecure:                envBool(lookup, "OTEL_EXPORTER_OTLP_INSECURE", true),
		ShutdownTimeout:             time.Duration(envInt(lookup, "SHUTDOWN_TIMEOUT_SECONDS", 10)) * time.Second,
	}
	if cfg.Provider != ProviderOllama && cfg.Provider != ProviderOpenAI {
		return Config{}, fmt.Errorf("config: unsupported LLM_PROVIDER %q", cfg.Provider)
	}
	if cfg.Model == "" {
		cfg.Model = defaultModel(cfg.Provider)
	}
	if cfg.EmbeddingProvider == "" {
		cfg.EmbeddingProvider = cfg.Provider
	}
	if cfg.EmbeddingProvider != ProviderOllama && cfg.EmbeddingProvider != ProviderOpenAI {
		return Config{}, fmt.Errorf("config: unsupported EMBEDDING_PROVIDER %q", cfg.EmbeddingProvider)
	}
	if cfg.EmbeddingModel == "" {
		cfg.EmbeddingModel = defaultEmbeddingModel(cfg.EmbeddingProvider)
	}
	return cfg, nil
}

func defaultModel(p string) string {
	if p == ProviderOpenAI {
		return "gpt-4o-mini"
	}
	return "llama3.1"
}

func defaultEmbeddingModel(p string) string {
	if p == ProviderOpenAI {
		return "text-embedding-3-small"
	}
	return "nomic-embed-text"
}

func envOr(lookup func(string) (string, bool), key, def string) string {
	if v, ok := lookup(key); ok && v != "" {
		return v
	}
	return def
}

func envInt(lookup func(string) (string, bool), key string, def int) int {
	if v, ok := lookup(key); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return def
}

func envBool(lookup func(string) (string, bool), key string, def bool) bool {
	if v, ok := lookup(key); ok {
		if b, err := strconv.ParseBool(strings.TrimSpace(v)); err == nil {
			return b
		}
	}
	return def
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/config/ -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
cd llm-agent-kb && git add internal/config/ && \
git commit -m "feat(config): env-driven kbd configuration (M1 fields)"
```

---

## Task 3: ragsvc adapters (`ragModelAdapter` + `ragEmbedderAdapter`, pure/unit)

**Files:**
- Create: `llm-agent-kb/internal/ragsvc/adapters.go`
- Test: `llm-agent-kb/internal/ragsvc/adapters_test.go`

Per spec §12.1: `adapter/llmagent` is build-tagged (pulls the `llm-agent` core), so kb must hand-write a `ChatModel → generate.Model` adapter. `ragEmbedderAdapter` mirrors `customer-support/internal/knowledgebase`. Both are pure transforms (no DB), unit-tested with `llm.NewScriptedLLM`.

- [ ] **Step 1: Write the failing test**

Create `internal/ragsvc/adapters_test.go`:
```go
package ragsvc

import (
	"context"
	"testing"

	"github.com/costa92/llm-agent-contract/llm"
	ragembed "github.com/costa92/llm-agent-rag/embed"
	raggenerate "github.com/costa92/llm-agent-rag/generate"
)

func TestModelAdapterMapsRequestAndUsage(t *testing.T) {
	scripted := llm.NewScriptedLLM(llm.WithResponses(llm.Response{
		Text:  "the answer",
		Usage: llm.Usage{InputTokens: 7, OutputTokens: 3, TotalTokens: 10},
	}))
	m := ragModelAdapter{inner: scripted}
	resp, err := m.Generate(context.Background(), raggenerate.Request{
		SystemPrompt: "be terse",
		Messages:     []raggenerate.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Text != "the answer" {
		t.Fatalf("Text=%q want 'the answer'", resp.Text)
	}
	if resp.Usage.PromptTokens != 7 || resp.Usage.CompletionTokens != 3 || resp.Usage.TotalTokens != 10 {
		t.Fatalf("Usage=%+v want {7 3 10}", resp.Usage)
	}
}

func TestModelAdapterFillsTotalWhenZero(t *testing.T) {
	scripted := llm.NewScriptedLLM(llm.WithResponses(llm.Response{
		Text:  "x",
		Usage: llm.Usage{InputTokens: 4, OutputTokens: 6, TotalTokens: 0},
	}))
	m := ragModelAdapter{inner: scripted}
	resp, _ := m.Generate(context.Background(), raggenerate.Request{})
	if resp.Usage.TotalTokens != 10 {
		t.Fatalf("TotalTokens=%d want 10 (derived from 4+6)", resp.Usage.TotalTokens)
	}
}

func TestEmbedderAdapterEmbedAndDimension(t *testing.T) {
	scripted := llm.NewScriptedLLM(llm.WithEmbedDimensions(8))
	a := ragEmbedderAdapter{inner: scripted}
	if a.Dimension() != 8 {
		t.Fatalf("Dimension=%d want 8", a.Dimension())
	}
	v, err := a.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(v) != 8 {
		t.Fatalf("len(vector)=%d want 8", len(v))
	}
}

func TestEmbedderAdapterBatchPreservesOrder(t *testing.T) {
	scripted := llm.NewScriptedLLM(llm.WithEmbedDimensions(4))
	var _ ragembed.BatchEmbedder = ragEmbedderAdapter{inner: scripted}
	a := ragEmbedderAdapter{inner: scripted}
	vecs, err := a.EmbedBatch(context.Background(), []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("EmbedBatch: %v", err)
	}
	if len(vecs) != 3 {
		t.Fatalf("len=%d want 3", len(vecs))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/ragsvc/ -v`
Expected: FAIL to compile — undefined `ragModelAdapter`, `ragEmbedderAdapter`.

- [ ] **Step 3: Write the implementation**

Create `internal/ragsvc/adapters.go`:
```go
package ragsvc

import (
	"context"

	"github.com/costa92/llm-agent-contract/llm"
	ragembed "github.com/costa92/llm-agent-rag/embed"
	raggenerate "github.com/costa92/llm-agent-rag/generate"
)

// ragModelAdapter adapts a contract llm.ChatModel into the rag generate.Model
// seam. Hand-written because the shipped adapter/llmagent is build-tagged and
// would pull the llm-agent core (spec §12.1).
type ragModelAdapter struct {
	inner llm.ChatModel
}

func (a ragModelAdapter) Generate(ctx context.Context, req raggenerate.Request) (raggenerate.Response, error) {
	msgs := make([]llm.Message, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = llm.Message{Role: m.Role, Content: m.Content}
	}
	resp, err := a.inner.Generate(ctx, llm.Request{
		SystemPrompt: req.SystemPrompt,
		Messages:     msgs,
		Metadata:     req.Metadata,
	})
	if err != nil {
		return raggenerate.Response{}, err
	}
	total := resp.Usage.TotalTokens
	if total == 0 {
		total = resp.Usage.InputTokens + resp.Usage.OutputTokens
	}
	return raggenerate.Response{
		Text: resp.Text,
		Usage: raggenerate.Usage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      total,
		},
	}, nil
}

// ragEmbedderAdapter adapts a contract llm.Embedder into the rag embed.Embedder
// + embed.BatchEmbedder seams (mirrors customer-support/internal/knowledgebase).
// Implementing EmbedBatch engages rag.System.Import's batch fast path.
type ragEmbedderAdapter struct {
	inner llm.Embedder
}

func (a ragEmbedderAdapter) Embed(ctx context.Context, text string) (ragembed.Vector, error) {
	vectors, _, err := a.inner.Embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, nil
	}
	return ragembed.Vector(vectors[0]), nil
}

func (a ragEmbedderAdapter) Dimension() int { return a.inner.EmbedDimensions() }

func (a ragEmbedderAdapter) EmbedBatch(ctx context.Context, texts []string) ([]ragembed.Vector, error) {
	vectors, _, err := a.inner.Embed(ctx, texts)
	if err != nil {
		return nil, err
	}
	out := make([]ragembed.Vector, len(vectors))
	for i, v := range vectors {
		out[i] = ragembed.Vector(v)
	}
	return out, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/ragsvc/ -v`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
cd llm-agent-kb && git add internal/ragsvc/adapters.go internal/ragsvc/adapters_test.go && \
git commit -m "feat(ragsvc): ragModelAdapter + ragEmbedderAdapter glue (verified rag/contract signatures)"
```

---

## Task 4: Storage — pgxpool + RegisterTypes + business migrations + rag store

**Files:**
- Create: `llm-agent-kb/internal/storage/storage.go`
- Test: `llm-agent-kb/internal/storage/storage_test.go`

Opens the pool with `postgres.RegisterTypes` in `AfterConnect`, builds the rag `postgres.Store`, and applies the **M1** business migrations (`knowledge_base` + `document` only — `ingest_job`/`qa_*`/`eval_run` deferred). Live-PG test gated by `LLM_AGENT_KB_PG_URL` (pgvector image required).

- [ ] **Step 1: Write the failing test**

Create `internal/storage/storage_test.go`:
```go
package storage

import (
	"context"
	"os"
	"testing"
)

const liveEnvVar = "LLM_AGENT_KB_PG_URL"

func openTestStorage(t *testing.T, ctx context.Context) *Storage {
	t.Helper()
	dsn := os.Getenv(liveEnvVar)
	if dsn == "" {
		t.Skipf("set %s (pgvector-enabled Postgres) to run live tests", liveEnvVar)
	}
	st, err := Open(ctx, Config{PGURL: dsn, EmbeddingDim: 8})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(st.Close)
	// Clean slate for deterministic tests.
	for _, tbl := range []string{"document", "knowledge_base", "chunks", "chunks_entities", "chunks_relations", "chunks_communities", "chunks_community_reports"} {
		_, _ = st.Pool().Exec(ctx, "DROP TABLE IF EXISTS "+tbl+" CASCADE")
	}
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return st
}

func TestMigrateCreatesBusinessAndRagTables(t *testing.T) {
	ctx := context.Background()
	st := openTestStorage(t, ctx)
	if err := st.Migrate(ctx); err != nil { // idempotent second run
		t.Fatalf("second Migrate: %v", err)
	}
	for _, tbl := range []string{"knowledge_base", "document", "chunks"} {
		var n int
		if err := st.Pool().QueryRow(ctx, "SELECT count(*) FROM "+tbl).Scan(&n); err != nil {
			t.Fatalf("table %s not queryable after migrate: %v", tbl, err)
		}
	}
}

func TestRagStoreIsUsable(t *testing.T) {
	ctx := context.Background()
	st := openTestStorage(t, ctx)
	if st.RagStore() == nil {
		t.Fatal("RagStore() is nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/storage/ -v`
Expected: FAIL to compile — undefined `Open`, `Storage`, `Config`.

- [ ] **Step 3: Write the implementation**

Create `internal/storage/storage.go`:
```go
// Package storage owns the pgxpool, the kb business-table migrations
// (knowledge_base + document in M1), and the rag postgres.Store.
package storage

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	ragpostgres "github.com/costa92/llm-agent-rag/postgres"
)

// Config configures Open.
type Config struct {
	PGURL        string
	EmbeddingDim int // chunks vector(dim); must match the embedding model
}

// Storage holds the pool and the rag store.
type Storage struct {
	pool     *pgxpool.Pool
	ragStore *ragpostgres.Store
}

// Open builds the pool (registering the pgvector codec on every connection)
// and the rag postgres.Store. The caller owns Close.
func Open(ctx context.Context, cfg Config) (*Storage, error) {
	if cfg.PGURL == "" {
		return nil, fmt.Errorf("storage: PGURL is required")
	}
	if cfg.EmbeddingDim <= 0 {
		return nil, fmt.Errorf("storage: EmbeddingDim must be > 0")
	}
	poolCfg, err := pgxpool.ParseConfig(cfg.PGURL)
	if err != nil {
		return nil, fmt.Errorf("storage: parse dsn: %w", err)
	}
	// Register the pgvector type codec on every pooled connection.
	poolCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		return ragpostgres.RegisterTypes(ctx, conn)
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("storage: new pool: %w", err)
	}
	ragStore, err := ragpostgres.New(pool, ragpostgres.Config{
		Dimension: cfg.EmbeddingDim,
		// VectorIndex left at default (none) for M1; add IVFFlat/HNSW later.
	})
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("storage: rag store: %w", err)
	}
	return &Storage{pool: pool, ragStore: ragStore}, nil
}

// Pool returns the underlying pgxpool.
func (s *Storage) Pool() *pgxpool.Pool { return s.pool }

// RagStore returns the rag postgres.Store (used by ragsvc for vector ops + delete).
func (s *Storage) RagStore() *ragpostgres.Store { return s.ragStore }

// Close releases the pool.
func (s *Storage) Close() { s.pool.Close() }

// businessMigrations are the M1 kb-owned tables. ingest_job, qa_session,
// qa_message, and eval_run are deferred to their milestones (spec §13).
var businessMigrations = []string{
	`CREATE TABLE IF NOT EXISTS knowledge_base (
		id              TEXT PRIMARY KEY,
		org_id          TEXT NOT NULL,
		name            TEXT NOT NULL,
		namespace       TEXT NOT NULL UNIQUE,
		embedding_model TEXT NOT NULL DEFAULT '',
		embedding_dim   INT  NOT NULL DEFAULT 0,
		created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
	)`,
	`CREATE TABLE IF NOT EXISTS document (
		id           TEXT PRIMARY KEY,
		kb_id        TEXT NOT NULL REFERENCES knowledge_base(id) ON DELETE CASCADE,
		title        TEXT NOT NULL,
		source_type  TEXT NOT NULL,
		source_ref   TEXT NOT NULL DEFAULT '',
		source_id    TEXT NOT NULL,
		checksum     TEXT NOT NULL DEFAULT '',
		status       TEXT NOT NULL DEFAULT 'pending',
		error        TEXT NOT NULL DEFAULT '',
		chunk_count  INT  NOT NULL DEFAULT 0,
		created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
	)`,
	`CREATE INDEX IF NOT EXISTS document_kb_idx ON document (kb_id)`,
}

// Migrate applies the rag store migrations (chunks/graph/community + the
// pgvector extension) then the kb business migrations. Idempotent.
func (s *Storage) Migrate(ctx context.Context) error {
	if err := s.ragStore.Migrate(ctx); err != nil {
		return fmt.Errorf("storage: rag migrate: %w", err)
	}
	for _, stmt := range businessMigrations {
		if _, err := s.pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("storage: business migrate: %w", err)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run (with a disposable pgvector Postgres):
```bash
docker run -d --rm --name kb-pg -e POSTGRES_PASSWORD=pw -p 55433:5432 pgvector/pgvector:pg16
export LLM_AGENT_KB_PG_URL='postgres://postgres:pw@localhost:55433/postgres'
cd llm-agent-kb && GOWORK=off go test ./internal/storage/ -v
```
Expected: PASS. (Unset env → SKIP; you MUST run once with live PG to prove green. Tear down: `docker stop kb-pg`.)

- [ ] **Step 5: Commit**

```bash
cd llm-agent-kb && git add internal/storage/ && \
git commit -m "feat(storage): pgxpool + RegisterTypes + business migrations (knowledge_base, document) + rag Migrate"
```

---

## Task 5: ragsvc RagPort — Ask/Import via Wrapper + delete ops via held Store

**Files:**
- Create: `llm-agent-kb/internal/ragsvc/ragsvc.go`
- Test: `llm-agent-kb/internal/ragsvc/ragsvc_test.go`

`RagPort` is the narrow interface every non-rag package depends on. Per spec §4/§5, M1 needs ONLY `Ask` + `Import` (delegated to `*otelrag.Wrapper`, auto-spans) plus the three delete primitives (`ListChunkIDs`, `RemoveGraphBySource`, `RemoveChunks`) backed by the directly-held `*postgres.Store`. NO `AskGlobal`/`AskDrift`/`Prewarm` (M3). The constructor wires `rag.New` + `otelrag.Wrap`. **Tenant isolation is by `SearchOptions.Namespace` ALONE — `SecurityFilters` is NOT set (it applies as `metadata @> $n` and chunks carry no `namespace` metadata key, so it would match zero chunks; the live Ask test guards against that regression).**

- [ ] **Step 1: Write the failing test**

Create `internal/ragsvc/ragsvc_test.go`:
```go
package ragsvc

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/costa92/llm-agent-contract/llm"
	ragingest "github.com/costa92/llm-agent-rag/ingest"
	ragpostgres "github.com/costa92/llm-agent-rag/postgres"
	ragstore "github.com/costa92/llm-agent-rag/store"
)

// Compile-time: the concrete service must satisfy RagPort.
var _ RagPort = (*Service)(nil)

func TestNewWiresInMemorySystemForUnitTest(t *testing.T) {
	// With a nil store the rag default in-memory store is used; this proves
	// New wires the adapters + system without a DB. (Delete ops need the
	// postgres store and are exercised in the storage-gated cascade test.)
	model := llm.NewScriptedLLM(llm.WithResponses(llm.Response{Text: "hello", Usage: llm.Usage{TotalTokens: 3}}))
	embedder := llm.NewScriptedLLM(llm.WithEmbedDimensions(8))
	svc := New(Deps{Model: model, Embedder: embedder, RagStore: nil, ChunkStore: nil})
	if svc == nil {
		t.Fatal("New returned nil")
	}
	ctx := context.Background()
	if _, err := svc.Import(ctx, []ragingest.Document{{
		ID: "d1", SourceID: "d1", Content: "hello world", Title: "T",
	}}, ragingest.ImportOptions{Namespace: "ns"}); err != nil {
		t.Fatalf("Import: %v", err)
	}
	ans, err := svc.Ask(ctx, "hi", AskRequest{Namespace: "ns", TopK: 3, Hybrid: false})
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if ans.Text != "hello" {
		t.Fatalf("answer text=%q want hello", ans.Text)
	}
	// store deref guard: ChunkStore nil → delete ops error, not panic.
	if _, err := svc.ListChunkIDs(ctx, "ns", "d1"); err == nil {
		t.Fatal("ListChunkIDs with nil ChunkStore should error")
	}
	_ = ragstore.Filter{}
}

// TestAskReturnsHitsOnLivePgvector is the M2 regression guard: it imports a doc
// into a real pgvector store then asserts the Ask path returns at least one
// hit/citation. If SecurityFilters were (wrongly) set to {"namespace":...}, the
// `metadata @> $n` filter would match zero chunks and this test would catch it.
// Gated on LLM_AGENT_KB_PG_URL (pgvector-enabled).
func TestAskReturnsHitsOnLivePgvector(t *testing.T) {
	ctx := context.Background()
	dsn := os.Getenv("LLM_AGENT_KB_PG_URL")
	if dsn == "" {
		t.Skipf("set LLM_AGENT_KB_PG_URL (pgvector) to run the live Ask test")
	}
	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	poolCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		return ragpostgres.RegisterTypes(ctx, conn)
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	for _, tbl := range []string{"chunks", "chunks_entities", "chunks_relations", "chunks_communities", "chunks_community_reports"} {
		_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS "+tbl+" CASCADE")
	}
	chunkStore, err := ragpostgres.New(pool, ragpostgres.Config{Dimension: 8})
	if err != nil {
		t.Fatal(err)
	}
	if err := chunkStore.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	model := llm.NewScriptedLLM(llm.WithResponses(llm.Response{Text: "answer", Usage: llm.Usage{TotalTokens: 3}}))
	embedder := llm.NewScriptedLLM(llm.WithEmbedDimensions(8))
	svc := New(Deps{Model: model, Embedder: embedder, RagStore: chunkStore, ChunkStore: chunkStore})

	if _, err := svc.Import(ctx, []ragingest.Document{{
		ID: "d1", SourceID: "d1", Title: "T",
		Content: "the quick brown fox jumps over the lazy dog repeatedly",
	}}, ragingest.ImportOptions{Namespace: "ns1", ReplaceSource: true}); err != nil {
		t.Fatalf("Import: %v", err)
	}
	ans, err := svc.Ask(ctx, "fox", AskRequest{Namespace: "ns1", TopK: 5})
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if ans.Diagnostics.HitCount == 0 && len(ans.Citations) == 0 {
		t.Fatalf("Ask returned no hits/citations (HitCount=%d, citations=%d) — Namespace isolation broken or SecurityFilters regression", ans.Diagnostics.HitCount, len(ans.Citations))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/ragsvc/ -run TestNew -v`
Expected: FAIL to compile — undefined `RagPort`, `Service`, `New`, `Deps`, `AskRequest`.

- [ ] **Step 3: Write the implementation**

Create `internal/ragsvc/ragsvc.go`:
```go
package ragsvc

import (
	"context"
	"fmt"

	"github.com/costa92/llm-agent-contract/llm"
	ragingest "github.com/costa92/llm-agent-rag/ingest"
	ragpostgres "github.com/costa92/llm-agent-rag/postgres"
	ragcore "github.com/costa92/llm-agent-rag/rag"
	ragstore "github.com/costa92/llm-agent-rag/store"
	"github.com/costa92/llm-agent-otel/otelrag"

	"go.opentelemetry.io/otel/trace"
)

// AskRequest is the kb-side ask request. Hybrid=false is vector mode
// (lexical/rerank off); Hybrid=true enables reranking. NO global/drift in M1.
type AskRequest struct {
	Namespace      string
	TopK           int
	Hybrid         bool
	MaxTotalTokens int
}

// RagPort is the narrow rag surface every non-rag kb package depends on.
// M1: Ask + Import + the three delete primitives (§16.4). No AskGlobal/
// AskDrift/Prewarm — those arrive in M3.
type RagPort interface {
	Ask(ctx context.Context, question string, req AskRequest) (ragcore.Answer, error)
	Import(ctx context.Context, docs []ragingest.Document, opts ragingest.ImportOptions) (ragingest.ImportResult, error)
	// ListChunkIDs returns the chunk IDs for a source in a namespace
	// (collected BEFORE removal so the graph can be reconciled by ID).
	ListChunkIDs(ctx context.Context, namespace, sourceID string) ([]string, error)
	// RemoveGraphBySource drops the given chunk IDs from the entity graph.
	RemoveGraphBySource(ctx context.Context, namespace string, chunkIDs []string) error
	// RemoveChunks deletes every chunk for a source in a namespace.
	RemoveChunks(ctx context.Context, namespace, sourceID string) (int, error)
}

// Deps are the construction inputs for the service.
type Deps struct {
	Model      llm.ChatModel
	Embedder   llm.Embedder
	RagStore   ragstore.Store        // backing store for rag.New (the postgres.Store)
	ChunkStore *ragpostgres.Store    // same store, concrete, for List/Remove delete ops
	Tracer     trace.TracerProvider  // optional; nil → no-op spans
}

// Service is the only unit that holds the rag backend.
type Service struct {
	wrapper    *otelrag.Wrapper
	chunkStore *ragpostgres.Store
}

// New wires the adapters, rag.System, and the otelrag wrapper.
func New(d Deps) *Service {
	sys := ragcore.New(ragcore.Options{
		Model:    ragModelAdapter{inner: d.Model},
		Embedder: ragEmbedderAdapter{inner: d.Embedder},
		Store:    d.RagStore, // nil → rag default in-memory store (unit tests)
	})
	var wrapper *otelrag.Wrapper
	if d.Tracer != nil {
		wrapper = otelrag.Wrap(sys, otelrag.Config{TracerProvider: d.Tracer})
	} else {
		wrapper = otelrag.Wrap(sys)
	}
	return &Service{wrapper: wrapper, chunkStore: d.ChunkStore}
}

func (s *Service) Ask(ctx context.Context, question string, req AskRequest) (ragcore.Answer, error) {
	// M1 tenant isolation is by Namespace ALONE (one kb per namespace). We do
	// NOT set SecurityFilters: it is applied as `metadata @> $n` (postgres.go
	// buildWhere → metadataJSONFilter), and chunks carry no "namespace"
	// metadata key, so {"namespace":...} would match zero chunks and silently
	// return no hits. Per-source/per-field metadata filtering is a later
	// concern (multi-source-per-namespace), out of M1 scope.
	return s.wrapper.Ask(ctx, question, ragcore.AskOptions{
		Search: ragcore.SearchOptions{
			Namespace:    req.Namespace,
			TopK:         req.TopK,
			EnableRerank: req.Hybrid, // hybrid on → rerank on; vector mode → off
		},
		MaxTotalTokens: req.MaxTotalTokens,
	})
}

func (s *Service) Import(ctx context.Context, docs []ragingest.Document, opts ragingest.ImportOptions) (ragingest.ImportResult, error) {
	return s.wrapper.Import(ctx, docs, opts)
}

func (s *Service) ListChunkIDs(ctx context.Context, namespace, sourceID string) ([]string, error) {
	if s.chunkStore == nil {
		return nil, fmt.Errorf("ragsvc: chunk store not configured")
	}
	chunks, err := s.chunkStore.List(ctx, namespace, ragstore.Filter{
		ragingest.MetadataSourceIDKey: sourceID,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("ragsvc: list source %s: %w", sourceID, err)
	}
	ids := make([]string, 0, len(chunks))
	for _, c := range chunks {
		ids = append(ids, c.ID)
	}
	return ids, nil
}

func (s *Service) RemoveGraphBySource(ctx context.Context, namespace string, chunkIDs []string) error {
	if s.chunkStore == nil {
		return fmt.Errorf("ragsvc: chunk store not configured")
	}
	return s.chunkStore.RemoveGraphBySource(ctx, namespace, chunkIDs)
}

func (s *Service) RemoveChunks(ctx context.Context, namespace, sourceID string) (int, error) {
	if s.chunkStore == nil {
		return 0, fmt.Errorf("ragsvc: chunk store not configured")
	}
	return s.chunkStore.RemoveByFilter(ctx, namespace, ragstore.Filter{
		ragingest.MetadataSourceIDKey: sourceID,
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/ragsvc/ -v` (pure: adapter tests + `TestNewWiresInMemorySystemForUnitTest`). With pgvector PG up + `LLM_AGENT_KB_PG_URL` set, also runs `TestAskReturnsHitsOnLivePgvector`.
Expected: PASS (live Ask test SKIPs without PG, PASSes with it).

- [ ] **Step 5: Commit**

```bash
cd llm-agent-kb && git add internal/ragsvc/ragsvc.go internal/ragsvc/ragsvc_test.go && \
git commit -m "feat(ragsvc): narrow RagPort (Ask/Import + §16.4 delete primitives) over otelrag.Wrapper; Namespace-only isolation (no bogus SecurityFilters) + live Ask hit guard"
```

---

## Task 6: orgkb — KB CRUD + creator-admin membership + org bootstrap (CreateOrg) + org lookup

**Files:**
- Create: `llm-agent-kb/internal/orgkb/orgkb.go`
- Test: `llm-agent-kb/internal/orgkb/orgkb_test.go`

Per spec §5/§8: `knowledge_base` rows carry `org_id`; creating a kb writes the creator's `admin` membership via `authz.Store.UpsertMembership(scopeKind="kb", scopeID=&kbID)`. `CreateOrg` is the org-bootstrap path: it creates an org and writes the caller as an **org-level** `org_admin` (`scopeKind="kb"`, `scopeID=nil`) — the only way the first org_admin can come to exist (POST /api/orgs, H1). `OrgIDForKB` is the lookup the RBAC middleware needs (it has the path kbID but must resolve org_id). `DeleteRow` deletes the kb row AND its kb-scope `auth_membership` rows in one transaction (§16.4, M1). Cursor pagination per §16.2. Live-PG gated.

- [ ] **Step 1: Write the failing test**

Create `internal/orgkb/orgkb_test.go`:
```go
package orgkb

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	authzrole "github.com/costa92/llm-agent-authz/role"
	authzstore "github.com/costa92/llm-agent-authz/store"
)

const liveEnvVar = "LLM_AGENT_KB_PG_URL"

func openRepo(t *testing.T, ctx context.Context) (*Repo, *authzstore.Store, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv(liveEnvVar)
	if dsn == "" {
		t.Skipf("set %s to run live tests", liveEnvVar)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	for _, tbl := range []string{"document", "knowledge_base", "auth_membership", "auth_session", "auth_user", "auth_org"} {
		_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS "+tbl+" CASCADE")
	}
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS authz_schema_version")
	az := authzstore.New(pool)
	if err := az.Migrate(ctx); err != nil {
		t.Fatalf("authz migrate: %v", err)
	}
	// kb business tables.
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS knowledge_base (
		id TEXT PRIMARY KEY, org_id TEXT NOT NULL, name TEXT NOT NULL,
		namespace TEXT NOT NULL UNIQUE, embedding_model TEXT NOT NULL DEFAULT '',
		embedding_dim INT NOT NULL DEFAULT 0, created_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		t.Fatalf("create knowledge_base: %v", err)
	}
	return New(pool, az), az, pool
}

func TestCreateKBWritesAdminMembership(t *testing.T) {
	ctx := context.Background()
	repo, az, _ := openRepo(t, ctx)
	uid, err := az.CreateUser(ctx, "creator@x.com", "h")
	if err != nil {
		t.Fatal(err)
	}
	oid, err := az.CreateOrg(ctx, "Acme")
	if err != nil {
		t.Fatal(err)
	}
	kb, err := repo.Create(ctx, CreateInput{OrgID: oid, Name: "Docs", CreatorUserID: uid, EmbeddingModel: "nomic", EmbeddingDim: 8})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if kb.Namespace == "" || kb.ID == "" {
		t.Fatalf("kb missing id/namespace: %+v", kb)
	}
	// Creator must be admin on the new kb scope.
	got, err := az.ResolveRole(ctx, uid, oid, "kb", kb.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got != authzrole.RoleAdmin {
		t.Fatalf("creator role=%q want admin", got)
	}
}

func TestOrgIDForKB(t *testing.T) {
	ctx := context.Background()
	repo, az, _ := openRepo(t, ctx)
	uid, _ := az.CreateUser(ctx, "c@x.com", "h")
	oid, _ := az.CreateOrg(ctx, "Acme")
	kb, _ := repo.Create(ctx, CreateInput{OrgID: oid, Name: "D", CreatorUserID: uid, EmbeddingDim: 8})
	gotOrg, err := repo.OrgIDForKB(ctx, kb.ID)
	if err != nil || gotOrg != oid {
		t.Fatalf("OrgIDForKB=%q,%v want %q", gotOrg, err, oid)
	}
	if _, err := repo.OrgIDForKB(ctx, "nope"); err != ErrNotFound {
		t.Fatalf("missing kb err=%v want ErrNotFound", err)
	}
}

func TestListAndDelete(t *testing.T) {
	ctx := context.Background()
	repo, az, _ := openRepo(t, ctx)
	uid, _ := az.CreateUser(ctx, "c@x.com", "h")
	oid, _ := az.CreateOrg(ctx, "Acme")
	a, _ := repo.Create(ctx, CreateInput{OrgID: oid, Name: "A", CreatorUserID: uid, EmbeddingDim: 8})
	_, _ = repo.Create(ctx, CreateInput{OrgID: oid, Name: "B", CreatorUserID: uid, EmbeddingDim: 8})
	items, _, err := repo.ListByOrg(ctx, oid, 10, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items)=%d want 2", len(items))
	}
	// Creator has kb-scope admin on A before delete.
	if got, _ := az.ResolveRole(ctx, uid, oid, "kb", a.ID); got != authzrole.RoleAdmin {
		t.Fatalf("pre-delete role=%q want admin", got)
	}
	if err := repo.DeleteRow(ctx, a.ID); err != nil {
		t.Fatalf("DeleteRow: %v", err)
	}
	if _, err := repo.Get(ctx, a.ID); err != ErrNotFound {
		t.Fatalf("deleted kb get err=%v want ErrNotFound", err)
	}
	// §16.4 (M1): the kb-scope membership is gone after delete.
	if got, _ := az.ResolveRole(ctx, uid, oid, "kb", a.ID); got != authzrole.RoleNone {
		t.Fatalf("post-delete role=%q want none (membership must be removed)", got)
	}
}

func TestCreateOrgGrantsOrgAdminWhoCanCreateKB(t *testing.T) {
	ctx := context.Background()
	repo, az, _ := openRepo(t, ctx)
	uid, _ := az.CreateUser(ctx, "boss@x.com", "h")
	oid, err := repo.CreateOrg(ctx, "Acme", uid)
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	// Org creator is an org-level org_admin: org-level row (scope_id NULL)
	// matches any kb-scope resolve, and org_admin outranks admin.
	got, err := az.ResolveRole(ctx, uid, oid, "kb", "any-kb-id")
	if err != nil {
		t.Fatal(err)
	}
	if got != authzrole.RoleOrgAdmin {
		t.Fatalf("org creator role=%q want org_admin", got)
	}
	if !got.AtLeast(authzrole.RoleAdmin) {
		t.Fatal("org_admin must satisfy admin minimum for kb-create")
	}
	// And the org_admin can create a kb in the org.
	kb, err := repo.Create(ctx, CreateInput{OrgID: oid, Name: "Docs", CreatorUserID: uid, EmbeddingDim: 8})
	if err != nil || kb.ID == "" {
		t.Fatalf("org_admin Create kb: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/orgkb/ -v`
Expected: FAIL to compile — undefined `New`, `Repo`, `CreateInput`, etc.

- [ ] **Step 3: Write the implementation**

Create `internal/orgkb/orgkb.go`:
```go
// Package orgkb is the knowledge-base resource domain: create/list/get/delete
// kb rows and write the creator's admin membership via authz. It never touches
// vectors (spec §4).
package orgkb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	authzrole "github.com/costa92/llm-agent-authz/role"
	authzstore "github.com/costa92/llm-agent-authz/store"
)

// ErrNotFound is returned when a kb row does not exist.
var ErrNotFound = errors.New("orgkb: not found")

// KB is a knowledge_base row.
type KB struct {
	ID             string
	OrgID          string
	Name           string
	Namespace      string
	EmbeddingModel string
	EmbeddingDim   int
}

// CreateInput is the input to Create.
type CreateInput struct {
	OrgID          string
	Name           string
	CreatorUserID  string
	EmbeddingModel string
	EmbeddingDim   int
}

// Repo persists knowledge bases and writes authz memberships.
type Repo struct {
	pool  *pgxpool.Pool
	authz *authzstore.Store
}

// New builds a Repo.
func New(pool *pgxpool.Pool, az *authzstore.Store) *Repo { return &Repo{pool: pool, authz: az} }

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Create inserts a kb row and grants the creator admin on the new kb scope,
// in one transaction. namespace = "kb_" + id so it is unique and stable.
func (r *Repo) Create(ctx context.Context, in CreateInput) (KB, error) {
	if in.OrgID == "" || in.Name == "" || in.CreatorUserID == "" {
		return KB{}, fmt.Errorf("orgkb: OrgID, Name, CreatorUserID required")
	}
	kb := KB{
		ID:             newID(),
		OrgID:          in.OrgID,
		Name:           in.Name,
		EmbeddingModel: in.EmbeddingModel,
		EmbeddingDim:   in.EmbeddingDim,
	}
	kb.Namespace = "kb_" + kb.ID

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return KB{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx,
		`INSERT INTO knowledge_base (id, org_id, name, namespace, embedding_model, embedding_dim)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		kb.ID, kb.OrgID, kb.Name, kb.Namespace, kb.EmbeddingModel, kb.EmbeddingDim); err != nil {
		return KB{}, fmt.Errorf("orgkb: insert kb: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return KB{}, err
	}
	// Membership is in the authz schema; UpsertMembership is idempotent so a
	// retry is safe. scope_id is the kb id.
	kbID := kb.ID
	if err := r.authz.UpsertMembership(ctx, kb.OrgID, in.CreatorUserID, "kb", &kbID, authzrole.RoleAdmin); err != nil {
		return KB{}, fmt.Errorf("orgkb: grant creator admin: %w", err)
	}
	return kb, nil
}

// Get returns a kb row by id.
func (r *Repo) Get(ctx context.Context, id string) (KB, error) {
	var kb KB
	err := r.pool.QueryRow(ctx,
		`SELECT id, org_id, name, namespace, embedding_model, embedding_dim
		 FROM knowledge_base WHERE id = $1`, id).
		Scan(&kb.ID, &kb.OrgID, &kb.Name, &kb.Namespace, &kb.EmbeddingModel, &kb.EmbeddingDim)
	if errors.Is(err, pgx.ErrNoRows) {
		return KB{}, ErrNotFound
	}
	return kb, err
}

// OrgIDForKB resolves the org_id for a kb (used by the RBAC middleware, which
// only has the kb id from the path).
func (r *Repo) OrgIDForKB(ctx context.Context, kbID string) (string, error) {
	var orgID string
	err := r.pool.QueryRow(ctx, `SELECT org_id FROM knowledge_base WHERE id = $1`, kbID).Scan(&orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return orgID, err
}

// ListByOrg returns up to limit kbs for an org, ordered by id, with a cursor
// (the last id seen). Empty cursor starts from the beginning.
func (r *Repo) ListByOrg(ctx context.Context, orgID string, limit int, cursor string) ([]KB, string, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id, org_id, name, namespace, embedding_model, embedding_dim
		 FROM knowledge_base
		 WHERE org_id = $1 AND id > $2
		 ORDER BY id ASC
		 LIMIT $3`, orgID, cursor, limit)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	var out []KB
	for rows.Next() {
		var kb KB
		if err := rows.Scan(&kb.ID, &kb.OrgID, &kb.Name, &kb.Namespace, &kb.EmbeddingModel, &kb.EmbeddingDim); err != nil {
			return nil, "", err
		}
		out = append(out, kb)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(out) == limit {
		next = out[len(out)-1].ID
	}
	return out, next, nil
}

// DeleteRow deletes the knowledge_base row AND the authz kb-scope memberships
// for that kb, in one transaction (§16.4: "删 knowledge_base 行 + authz 中该
// scope 的 membership"). The chunk/graph cascade runs BEFORE this (in the
// httpapi delete-kb handler via DeleteAllDocumentsForKB); this is the final
// step. auth_membership lives in the authz schema but shares the same pgxpool,
// so the delete is part of the same transaction.
func (r *Repo) DeleteRow(ctx context.Context, id string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `DELETE FROM knowledge_base WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	// Remove every kb-scope membership for this kb (creator admin + any granted).
	if _, err := tx.Exec(ctx,
		`DELETE FROM auth_membership WHERE scope_kind = 'kb' AND scope_id = $1`, id); err != nil {
		return fmt.Errorf("orgkb: delete kb memberships: %w", err)
	}
	return tx.Commit(ctx)
}

// CreateOrg is the org-bootstrap path (POST /api/orgs): it creates an org via
// the authz store and immediately writes the caller as an ORG-LEVEL admin
// (scope_kind="kb", scope_id=nil, role=org_admin). Org-level rows match every
// kb-scope ResolveRole (memberships query: `scope_id IS NULL OR scope_id=$4`),
// so the org creator becomes an org_admin who can create/list kbs in the org.
// This is the only seam by which the first org_admin can exist.
func (r *Repo) CreateOrg(ctx context.Context, name, creatorUserID string) (string, error) {
	if name == "" || creatorUserID == "" {
		return "", fmt.Errorf("orgkb: org name and creatorUserID required")
	}
	orgID, err := r.authz.CreateOrg(ctx, name)
	if err != nil {
		return "", fmt.Errorf("orgkb: create org: %w", err)
	}
	// scope_id=nil → org-level membership (matches any kb scope on resolve).
	if err := r.authz.UpsertMembership(ctx, orgID, creatorUserID, "kb", nil, authzrole.RoleOrgAdmin); err != nil {
		return "", fmt.Errorf("orgkb: grant creator org_admin: %w", err)
	}
	return orgID, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run (PG env set): `cd llm-agent-kb && GOWORK=off go test ./internal/orgkb/ -v`
Expected: PASS (5 tests: create-admin, OrgIDForKB, list+delete+membership-gone, create-org→org_admin→can-create-kb).

- [ ] **Step 5: Commit**

```bash
cd llm-agent-kb && git add internal/orgkb/ && \
git commit -m "feat(orgkb): kb CRUD + creator-admin membership + org bootstrap (CreateOrg→org_admin) + OrgIDForKB + §16.4 membership cleanup on delete"
```

---

## Task 7: ingest — parse MD/TXT/paste → ragingest.Document → synchronous Import

**Files:**
- Create: `llm-agent-kb/internal/ingest/ingest.go`
- Test: `llm-agent-kb/internal/ingest/ingest_test.go`

Per spec §6/§13 M1: **synchronous** ingest. The handler calls `Service.Ingest`, which writes the `document` row, parses inline by `source_type ∈ {markdown, txt, paste}`, builds the `ragingest.Document` (`ID = SourceID = document.id`, `Checksum`, `Metadata{kb_id, source_type}` — NOT doc_id), calls `RagPort.Import` with `Namespace=kb.namespace, ReplaceSource=true`, then sets `document.status=ready` + `chunk_count`. Checksum short-circuit: unchanged content skips Import. The pure `parse`/`makeDocument` helpers are unit-tested without a DB; the DB-touching `Ingest` orchestration is gated.

- [ ] **Step 1: Write the failing test**

Create `internal/ingest/ingest_test.go`:
```go
package ingest

import (
	"strings"
	"testing"

	ragingest "github.com/costa92/llm-agent-rag/ingest"
)

func TestParseTXTAndPasteArePlainText(t *testing.T) {
	got, err := parse(SourceTypeTXT, []byte("line one\nline two"))
	if err != nil || got != "line one\nline two" {
		t.Fatalf("parse txt=%q,%v", got, err)
	}
	got, err = parse(SourceTypePaste, []byte("pasted"))
	if err != nil || got != "pasted" {
		t.Fatalf("parse paste=%q,%v", got, err)
	}
}

func TestParseMarkdownKeptVerbatim(t *testing.T) {
	md := "# Title\n\nbody text"
	got, err := parse(SourceTypeMarkdown, []byte(md))
	if err != nil || got != md {
		t.Fatalf("parse md=%q,%v", got, err)
	}
}

func TestParseRejectsUnsupportedType(t *testing.T) {
	if _, err := parse("pdf", []byte("x")); err == nil {
		t.Fatal("pdf is M2 — must be rejected in M1")
	}
}

func TestMakeDocumentSetsSourceIDToDocIDAndMetadata(t *testing.T) {
	doc := makeDocument("doc-1", "kb-9", SourceTypeMarkdown, "My Title", "# H\n\ntext")
	if doc.ID != "doc-1" || doc.SourceID != "doc-1" {
		t.Fatalf("ID/SourceID=%q/%q want doc-1/doc-1", doc.ID, doc.SourceID)
	}
	if doc.Title != "My Title" {
		t.Fatalf("Title=%q", doc.Title)
	}
	if doc.Checksum == "" {
		t.Fatal("Checksum must be set for the unchanged-skip short-circuit")
	}
	// doc_id must NOT be in metadata (citation DocID comes from Chunk.DocID).
	if _, exists := doc.Metadata["doc_id"]; exists {
		t.Fatal("metadata must NOT carry doc_id (spec §6 / §7)")
	}
	if doc.Metadata["kb_id"] != "kb-9" || doc.Metadata["source_type"] != string(SourceTypeMarkdown) {
		t.Fatalf("metadata=%v want kb_id/source_type", doc.Metadata)
	}
	var _ ragingest.Document = doc
}

func TestChecksumStableAndContentSensitive(t *testing.T) {
	a := checksum("hello")
	b := checksum("hello")
	c := checksum("world")
	if a != b {
		t.Fatal("checksum must be stable for identical content")
	}
	if a == c {
		t.Fatal("checksum must differ for different content")
	}
	if !strings.HasPrefix(a, "sha256:") {
		t.Fatalf("checksum=%q want sha256: prefix", a)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/ingest/ -v`
Expected: FAIL to compile — undefined `parse`, `makeDocument`, `checksum`, source-type consts.

- [ ] **Step 3: Write the implementation**

Create `internal/ingest/ingest.go`:
```go
// Package ingest parses uploaded/pasted text into a ragingest.Document and
// imports it synchronously via the RagPort (spec §6, M1 = sync, MD/TXT/paste).
package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	ragingest "github.com/costa92/llm-agent-rag/ingest"

	"github.com/costa92/llm-agent-kb/internal/ragsvc"
)

// SourceType is the M1 set of accepted document source types. PDF/DOCX/URL are M2.
type SourceType string

const (
	SourceTypeMarkdown SourceType = "markdown"
	SourceTypeTXT      SourceType = "txt"
	SourceTypePaste    SourceType = "paste"
)

// parse converts raw bytes to text for an M1 source type. MD/TXT/paste are all
// treated as text (the rag splitter handles markdown structure downstream).
func parse(st SourceType, raw []byte) (string, error) {
	switch st {
	case SourceTypeMarkdown, SourceTypeTXT, SourceTypePaste:
		return string(raw), nil
	default:
		return "", fmt.Errorf("ingest: unsupported source_type %q (M1 supports markdown/txt/paste)", st)
	}
}

func checksum(content string) string {
	sum := sha256.Sum256([]byte(content))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// makeDocument builds the ragingest.Document. ID=SourceID=docID drives
// ReplaceSource + per-source delete. doc_id is deliberately NOT in Metadata —
// citation DocID comes from Chunk.DocID (spec §6/§7).
func makeDocument(docID, kbID string, st SourceType, title, content string) ragingest.Document {
	return ragingest.Document{
		ID:       docID,
		SourceID: docID,
		Title:    title,
		Content:  content,
		Checksum: checksum(content),
		Metadata: map[string]any{
			"kb_id":       kbID,
			"source_type": string(st),
		},
	}
}

// Service performs synchronous ingest: write document row → parse → Import →
// mark ready.
type Service struct {
	pool *pgxpool.Pool
	rag  ragsvc.RagPort
}

// New builds an ingest Service.
func New(pool *pgxpool.Pool, rag ragsvc.RagPort) *Service {
	return &Service{pool: pool, rag: rag}
}

// IngestInput is the input to Ingest.
type IngestInput struct {
	KBID       string
	Namespace  string
	Title      string
	SourceType SourceType
	Raw        []byte
}

// Result is the outcome of Ingest.
type Result struct {
	DocumentID string
	Status     string
	ChunkCount int
}

// Ingest writes the document row, parses, imports synchronously, and marks the
// document ready. On parse/import failure the row is marked failed with the
// error recorded.
func (s *Service) Ingest(ctx context.Context, in IngestInput) (Result, error) {
	content, err := parse(in.SourceType, in.Raw)
	if err != nil {
		return Result{}, err
	}
	docID := newID()
	cs := checksum(content)
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO document (id, kb_id, title, source_type, source_id, checksum, status)
		 VALUES ($1, $2, $3, $4, $1, $5, 'parsing')`,
		docID, in.KBID, in.Title, string(in.SourceType), cs); err != nil {
		return Result{}, fmt.Errorf("ingest: insert document: %w", err)
	}

	doc := makeDocument(docID, in.KBID, in.SourceType, in.Title, content)
	res, err := s.rag.Import(ctx, []ragingest.Document{doc}, ragingest.ImportOptions{
		Namespace:     in.Namespace,
		ReplaceSource: true,
	})
	if err != nil {
		_, _ = s.pool.Exec(ctx,
			`UPDATE document SET status='failed', error=$2 WHERE id=$1`, docID, err.Error())
		return Result{}, fmt.Errorf("ingest: import: %w", err)
	}
	if _, err := s.pool.Exec(ctx,
		`UPDATE document SET status='ready', chunk_count=$2 WHERE id=$1`, docID, res.Chunks); err != nil {
		return Result{}, fmt.Errorf("ingest: mark ready: %w", err)
	}
	return Result{DocumentID: docID, Status: "ready", ChunkCount: res.Chunks}, nil
}

func newID() string {
	b := make([]byte, 16)
	_, _ = randRead(b)
	return hex.EncodeToString(b)
}
```

Append to the SAME file the small `randRead` indirection (keeps `crypto/rand` import local and testable):
```go
import "crypto/rand"

func randRead(b []byte) (int, error) { return rand.Read(b) }
```
> Implementation note: merge the two `import` blocks — the file has one import group containing `context`, `crypto/rand`, `crypto/sha256`, `encoding/hex`, `fmt`, the pgxpool import, the ragingest import, and the ragsvc import. The split above is for readability in the plan only.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/ingest/ -v`
Expected: PASS (5 tests; all pure — no DB). The DB-touching `Ingest` is exercised end-to-end in the httpapi test (Task 10) and the cascade test (Task 8).

- [ ] **Step 5: Commit**

```bash
cd llm-agent-kb && git add internal/ingest/ && \
git commit -m "feat(ingest): synchronous MD/TXT/paste parse → ragingest.Document (SourceID=docID) → Import → ready"
```

---

## Task 8: Delete cascade (§16.4) — document + kb

**Files:**
- Create: `llm-agent-kb/internal/ingest/delete.go`
- Test: `llm-agent-kb/internal/ingest/delete_test.go`

Per spec §16.4 strict order: `ids := List(ns, source_id)` → `RemoveGraphBySource(ns, ids)` → `RemoveChunks(ns, source_id)` → delete `document` row. In M1 the graph is empty so `RemoveGraphBySource` is a no-op, but the call is kept to honor the contract and stay forward-safe. Delete-kb deletes all docs the same way, then the kb row + membership (the membership removal is wired in the httpapi delete-kb handler, Task 10). This task delivers the document-level cascade. Live-PG gated end-to-end (uses a real Import then a real delete).

- [ ] **Step 1: Write the failing test**

Create `internal/ingest/delete_test.go`:
```go
package ingest

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/costa92/llm-agent-contract/llm"
	ragpostgres "github.com/costa92/llm-agent-rag/postgres"
	"github.com/costa92/llm-agent-kb/internal/ragsvc"
)

const liveEnvVar = "LLM_AGENT_KB_PG_URL"

func openIngest(t *testing.T, ctx context.Context) (*Service, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv(liveEnvVar)
	if dsn == "" {
		t.Skipf("set %s (pgvector) to run live tests", liveEnvVar)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	for _, tbl := range []string{"document", "knowledge_base", "chunks", "chunks_entities", "chunks_relations", "chunks_communities", "chunks_community_reports"} {
		_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS "+tbl+" CASCADE")
	}
	chunkStore, err := ragpostgres.New(pool, ragpostgres.Config{Dimension: 8})
	if err != nil {
		t.Fatalf("rag store: %v", err)
	}
	if err := chunkStore.Migrate(ctx); err != nil {
		t.Fatalf("rag migrate: %v", err)
	}
	if _, err := pool.Exec(ctx, `CREATE TABLE knowledge_base (id TEXT PRIMARY KEY, org_id TEXT NOT NULL, name TEXT NOT NULL, namespace TEXT NOT NULL UNIQUE, embedding_model TEXT NOT NULL DEFAULT '', embedding_dim INT NOT NULL DEFAULT 0, created_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		t.Fatalf("create kb table: %v", err)
	}
	if _, err := pool.Exec(ctx, `CREATE TABLE document (id TEXT PRIMARY KEY, kb_id TEXT NOT NULL REFERENCES knowledge_base(id) ON DELETE CASCADE, title TEXT NOT NULL, source_type TEXT NOT NULL, source_ref TEXT NOT NULL DEFAULT '', source_id TEXT NOT NULL, checksum TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'pending', error TEXT NOT NULL DEFAULT '', chunk_count INT NOT NULL DEFAULT 0, created_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		t.Fatalf("create document table: %v", err)
	}
	model := llm.NewScriptedLLM(llm.WithResponses(llm.Response{Text: "ok"}))
	embedder := llm.NewScriptedLLM(llm.WithEmbedDimensions(8))
	rag := ragsvc.New(ragsvc.Deps{Model: model, Embedder: embedder, RagStore: chunkStore, ChunkStore: chunkStore})
	return New(pool, rag), pool
}

func TestDeleteDocumentCascade(t *testing.T) {
	ctx := context.Background()
	svc, pool := openIngest(t, ctx)
	_, _ = pool.Exec(ctx, `INSERT INTO knowledge_base (id, org_id, name, namespace, embedding_dim) VALUES ('kb1','org1','KB','ns1',8)`)
	res, err := svc.Ingest(ctx, IngestInput{KBID: "kb1", Namespace: "ns1", Title: "T", SourceType: SourceTypeMarkdown, Raw: []byte("# H\n\nbody one body two body three")})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if res.ChunkCount == 0 {
		t.Fatal("expected at least one chunk")
	}
	// Chunks exist before delete.
	ids, _ := svc.rag.ListChunkIDs(ctx, "ns1", res.DocumentID)
	if len(ids) == 0 {
		t.Fatal("expected chunks before delete")
	}
	if err := svc.DeleteDocument(ctx, "ns1", res.DocumentID); err != nil {
		t.Fatalf("DeleteDocument: %v", err)
	}
	// Chunks gone.
	ids, _ = svc.rag.ListChunkIDs(ctx, "ns1", res.DocumentID)
	if len(ids) != 0 {
		t.Fatalf("chunks remain after delete: %d", len(ids))
	}
	// document row gone.
	var n int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM document WHERE id=$1`, res.DocumentID).Scan(&n)
	if n != 0 {
		t.Fatalf("document row remains: %d", n)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/ingest/ -run TestDeleteDocumentCascade -v`
Expected: FAIL to compile — undefined `DeleteDocument`. (Or SKIP if no PG — run with PG.)

- [ ] **Step 3: Write the implementation**

Create `internal/ingest/delete.go`:
```go
package ingest

import (
	"context"
	"fmt"
)

// DeleteDocument removes a document and its chunks/graph contributions in the
// strict §16.4 order, then deletes the document row. The RemoveGraphBySource
// call is a no-op in M1 (no graph) but kept to honor the contract.
func (s *Service) DeleteDocument(ctx context.Context, namespace, documentID string) error {
	// 1. Collect chunk IDs BEFORE removal (RemoveByFilter returns only a count).
	ids, err := s.rag.ListChunkIDs(ctx, namespace, documentID)
	if err != nil {
		return fmt.Errorf("ingest: list chunks: %w", err)
	}
	// 2. Reconcile the graph by chunk ID (must precede chunk deletion).
	if err := s.rag.RemoveGraphBySource(ctx, namespace, ids); err != nil {
		return fmt.Errorf("ingest: remove graph: %w", err)
	}
	// 3. Delete the chunks.
	if _, err := s.rag.RemoveChunks(ctx, namespace, documentID); err != nil {
		return fmt.Errorf("ingest: remove chunks: %w", err)
	}
	// 4. Delete the business row.
	if _, err := s.pool.Exec(ctx, `DELETE FROM document WHERE id = $1`, documentID); err != nil {
		return fmt.Errorf("ingest: delete document row: %w", err)
	}
	return nil
}

// DeleteAllDocumentsForKB applies the §16.4 cascade to every document in a kb.
// The caller (httpapi delete-kb handler) then deletes the kb row + membership.
func (s *Service) DeleteAllDocumentsForKB(ctx context.Context, namespace, kbID string) error {
	rows, err := s.pool.Query(ctx, `SELECT id FROM document WHERE kb_id = $1`, kbID)
	if err != nil {
		return fmt.Errorf("ingest: list kb documents: %w", err)
	}
	var docIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		docIDs = append(docIDs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range docIDs {
		if err := s.DeleteDocument(ctx, namespace, id); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run (PG env set): `cd llm-agent-kb && GOWORK=off go test ./internal/ingest/ -v`
Expected: PASS (pure parse tests + `TestDeleteDocumentCascade`).

- [ ] **Step 5: Commit**

```bash
cd llm-agent-kb && git add internal/ingest/delete.go internal/ingest/delete_test.go && \
git commit -m "feat(ingest): §16.4 delete cascade (list→graph→chunks→row) for document and kb"
```

---

## Task 9: retrieval — Ask vector/hybrid → citation mapping + limits

### 9a: limits (`internal/limits`, pure)

**Files:**
- Create: `llm-agent-kb/internal/limits/limits.go`
- Test: `llm-agent-kb/internal/limits/limits_test.go`

Per spec §11/§13 M1: a minimal in-process per-user fixed-window counter (NOT customer-support's `WrapAgent`, which wraps `agents.Agent` and pulls the llm-agent core). Token budget is delegated to rag via `AskOptions.MaxTotalTokens` (Task 5), so `limits` only does the request-rate window.

- [ ] **Step 1: Write the failing test**

Create `internal/limits/limits_test.go`:
```go
package limits

import (
	"testing"
	"time"
)

func TestPerUserWindowAllowsUpToLimit(t *testing.T) {
	now := time.Unix(0, 0)
	g := New(2)
	for i := 0; i < 2; i++ {
		if !g.AllowAt("u1", now) {
			t.Fatalf("request %d for u1 should be allowed", i)
		}
	}
	if g.AllowAt("u1", now) {
		t.Fatal("3rd request in the same window must be denied")
	}
	// A different user has an independent budget.
	if !g.AllowAt("u2", now) {
		t.Fatal("u2 must have its own budget")
	}
}

func TestWindowResetsNextMinute(t *testing.T) {
	g := New(1)
	now := time.Unix(0, 0)
	if !g.AllowAt("u1", now) {
		t.Fatal("first allowed")
	}
	if g.AllowAt("u1", now) {
		t.Fatal("second denied in same minute")
	}
	if !g.AllowAt("u1", now.Add(time.Minute)) {
		t.Fatal("allowed again next minute")
	}
}

func TestZeroLimitIsUnlimited(t *testing.T) {
	g := New(0)
	now := time.Unix(0, 0)
	for i := 0; i < 100; i++ {
		if !g.AllowAt("u1", now) {
			t.Fatal("limit 0 means unlimited")
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/limits/ -v`
Expected: FAIL to compile — undefined `New`, `AllowAt`.

- [ ] **Step 3: Write the implementation**

Create `internal/limits/limits.go`:
```go
// Package limits is an in-process per-user fixed-window request counter.
// Token budgets are enforced by rag (AskOptions.MaxTotalTokens), not here.
package limits

import (
	"sync"
	"time"
)

// Guard caps requests per user per minute.
type Guard struct {
	perMinute int
	mu        sync.Mutex
	buckets   map[string]*bucket
}

type bucket struct {
	minute int64
	count  int
}

// New builds a Guard. perMinute <= 0 disables limiting (unlimited).
func New(perMinute int) *Guard {
	return &Guard{perMinute: perMinute, buckets: map[string]*bucket{}}
}

// Allow reports whether userID may make a request now.
func (g *Guard) Allow(userID string) bool { return g.AllowAt(userID, time.Now()) }

// AllowAt is Allow with an injectable clock (testable).
func (g *Guard) AllowAt(userID string, now time.Time) bool {
	if g.perMinute <= 0 {
		return true
	}
	minute := now.UTC().Unix() / 60
	g.mu.Lock()
	defer g.mu.Unlock()
	b := g.buckets[userID]
	if b == nil || b.minute != minute {
		b = &bucket{minute: minute}
		g.buckets[userID] = b
	}
	if b.count >= g.perMinute {
		return false
	}
	b.count++
	return true
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/limits/ -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
cd llm-agent-kb && git add internal/limits/ && \
git commit -m "feat(limits): in-process per-user fixed-window request counter"
```

### 9b: retrieval (`internal/retrieval`)

**Files:**
- Create: `llm-agent-kb/internal/retrieval/retrieval.go`
- Test: `llm-agent-kb/internal/retrieval/retrieval_test.go`

Maps the HTTP ask request to a `RagPort.Ask` call and `rag.Answer` to the external citation shape (spec §7/§16.2). Uses a fake `RagPort` so the citation mapping is unit-tested without a DB.

- [ ] **Step 1: Write the failing test**

Create `internal/retrieval/retrieval_test.go`:
```go
package retrieval

import (
	"context"
	"testing"

	ragingest "github.com/costa92/llm-agent-rag/ingest"
	ragcore "github.com/costa92/llm-agent-rag/rag"
	ragstore "github.com/costa92/llm-agent-rag/store"

	"github.com/costa92/llm-agent-kb/internal/ragsvc"
)

type fakeRag struct {
	gotReq ragsvc.AskRequest
	answer ragcore.Answer
}

func (f *fakeRag) Ask(_ context.Context, _ string, req ragsvc.AskRequest) (ragcore.Answer, error) {
	f.gotReq = req
	return f.answer, nil
}
func (f *fakeRag) Import(context.Context, []ragingest.Document, ragingest.ImportOptions) (ragingest.ImportResult, error) {
	return ragingest.ImportResult{}, nil
}
func (f *fakeRag) ListChunkIDs(context.Context, string, string) ([]string, error) { return nil, nil }
func (f *fakeRag) RemoveGraphBySource(context.Context, string, []string) error    { return nil }
func (f *fakeRag) RemoveChunks(context.Context, string, string) (int, error)      { return 0, nil }

func TestAskMapsModeAndCitations(t *testing.T) {
	f := &fakeRag{answer: ragcore.Answer{
		Text: "the answer",
		Hits: []ragstore.Hit{{Chunk: ragstore.StoredChunk{ID: "c1", Content: "a long snippet of source content"}, Score: 0.9}},
		Citations: []ragcore.Citation{{
			ChunkID: "c1", DocID: "d1", Title: "Doc One",
			SectionPath: []string{"Intro"}, Score: 0.9,
		}},
		Diagnostics: ragcore.Diagnostics{HitCount: 1},
	}}
	svc := New(f, Config{MaxAskTokens: 4096, SnippetChars: 10})

	out, err := svc.Ask(context.Background(), AskInput{Namespace: "ns1", Question: "q", Mode: "hybrid", TopK: 5})
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if !f.gotReq.Hybrid {
		t.Fatal("mode=hybrid must set Hybrid=true (rerank on)")
	}
	if f.gotReq.TopK != 5 || f.gotReq.Namespace != "ns1" || f.gotReq.MaxTotalTokens != 4096 {
		t.Fatalf("ask req=%+v", f.gotReq)
	}
	if out.Answer != "the answer" {
		t.Fatalf("Answer=%q", out.Answer)
	}
	if len(out.Citations) != 1 || out.Citations[0].ChunkID != "c1" || out.Citations[0].DocID != "d1" {
		t.Fatalf("citations=%+v", out.Citations)
	}
	if out.Citations[0].Snippet != "a long sni" { // SnippetChars=10
		t.Fatalf("snippet=%q want first 10 chars", out.Citations[0].Snippet)
	}
	if out.Diagnostics["mode"] != "hybrid" {
		t.Fatalf("diagnostics mode=%v", out.Diagnostics["mode"])
	}
}

func TestVectorModeDisablesRerank(t *testing.T) {
	f := &fakeRag{}
	svc := New(f, Config{MaxAskTokens: 100})
	if _, err := svc.Ask(context.Background(), AskInput{Namespace: "ns", Question: "q", Mode: "vector", TopK: 3}); err != nil {
		t.Fatal(err)
	}
	if f.gotReq.Hybrid {
		t.Fatal("mode=vector must set Hybrid=false")
	}
}

func TestRejectsUnknownMode(t *testing.T) {
	svc := New(&fakeRag{}, Config{})
	if _, err := svc.Ask(context.Background(), AskInput{Mode: "global"}); err == nil {
		t.Fatal("global is M3 — must be rejected in M1")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/retrieval/ -v`
Expected: FAIL to compile — undefined `New`, `Config`, `AskInput`.

- [ ] **Step 3: Write the implementation**

Create `internal/retrieval/retrieval.go`:
```go
// Package retrieval turns the HTTP ask request into a RagPort.Ask call and maps
// rag.Answer to the external citation/diagnostics JSON shape (spec §7).
package retrieval

import (
	"context"
	"fmt"

	"github.com/costa92/llm-agent-kb/internal/ragsvc"
)

// Config tunes the retrieval service.
type Config struct {
	MaxAskTokens int // → rag AskOptions.MaxTotalTokens
	SnippetChars int // citation snippet length; <=0 defaults to 240
}

// AskInput is the kb-side ask input.
type AskInput struct {
	Namespace string
	Question  string
	Mode      string // "vector" | "hybrid" (M1 only)
	TopK      int
}

// Citation is the external citation shape (spec §16.2).
type Citation struct {
	ChunkID     string   `json:"chunkId"`
	DocID       string   `json:"docId"`
	Title       string   `json:"title"`
	SectionPath []string `json:"sectionPath,omitempty"`
	Score       float64  `json:"score"`
	Snippet     string   `json:"snippet"`
}

// AskOutput is the external ask response.
type AskOutput struct {
	Answer      string         `json:"answer"`
	Citations   []Citation     `json:"citations"`
	Diagnostics map[string]any `json:"diagnostics"`
}

// Service maps ask requests/responses.
type Service struct {
	rag ragsvc.RagPort
	cfg Config
}

// New builds a retrieval Service.
func New(rag ragsvc.RagPort, cfg Config) *Service {
	if cfg.SnippetChars <= 0 {
		cfg.SnippetChars = 240
	}
	return &Service{rag: rag, cfg: cfg}
}

// Ask runs the vector/hybrid ask and maps citations. Modes other than
// vector/hybrid (global/drift) are M3 and rejected here.
func (s *Service) Ask(ctx context.Context, in AskInput) (AskOutput, error) {
	var hybrid bool
	switch in.Mode {
	case "hybrid":
		hybrid = true
	case "vector":
		hybrid = false
	default:
		return AskOutput{}, fmt.Errorf("retrieval: unsupported mode %q (M1 supports vector|hybrid)", in.Mode)
	}
	topK := in.TopK
	if topK <= 0 {
		topK = 5
	}
	ans, err := s.rag.Ask(ctx, in.Question, ragsvc.AskRequest{
		Namespace:      in.Namespace,
		TopK:           topK,
		Hybrid:         hybrid,
		MaxTotalTokens: s.cfg.MaxAskTokens,
	})
	if err != nil {
		return AskOutput{}, err
	}
	// Snippet source: the matching hit's content, by chunk id.
	snippetByChunk := map[string]string{}
	for _, h := range ans.Hits {
		snippetByChunk[h.Chunk.ID] = h.Chunk.Content
	}
	cites := make([]Citation, 0, len(ans.Citations))
	for _, c := range ans.Citations {
		cites = append(cites, Citation{
			ChunkID:     c.ChunkID,
			DocID:       c.DocID,
			Title:       c.Title,
			SectionPath: c.SectionPath,
			Score:       c.Score,
			Snippet:     truncate(snippetByChunk[c.ChunkID], s.cfg.SnippetChars),
		})
	}
	return AskOutput{
		Answer:    ans.Text,
		Citations: cites,
		Diagnostics: map[string]any{
			"mode":     in.Mode,
			"hitCount": ans.Diagnostics.HitCount,
		},
	}, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/retrieval/ -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
cd llm-agent-kb && git add internal/retrieval/ && \
git commit -m "feat(retrieval): vector/hybrid Ask → citation/diagnostics mapping (snippet from Hits)"
```

---

## Task 10: httpapi — routes (§16.2 M1 subset) + RBAC middleware chain + RoleResolver

**Files:**
- Create: `llm-agent-kb/internal/httpapi/httpapi.go`
- Create: `llm-agent-kb/internal/httpapi/handlers.go`
- Test: `llm-agent-kb/internal/httpapi/httpapi_test.go`

Wires the authz `Mount` for `/api/auth/*`, the §16.2 M1 routes, and the middleware chain `Authenticate → RequireScopeRole(scopeKind="kb", minRole, scope) → limits → handler`. Two scope resolvers:
- **kb-scoped** (`kbScopeFromRequest`, path `{id}`) resolves `(orgID, kbID)`: kbID from `r.PathValue("id")`, orgID via `orgkb.Repo.OrgIDForKB` (so `RequireScopeRole` gets the right org).
- **org-scoped** (`orgScopeFromRequest`, path `{org}`) resolves `(orgID, "")`; with `scopeID=""` the memberships query (`scope_id IS NULL OR scope_id=$4`) matches the org-level `org_admin` row, and `org_admin` (rank 4) outranks `admin`/`viewer` minimums.

§16.2 M1 routes (H1): `POST /api/orgs` (Authenticate only — the org-bootstrap seam; creator → org_admin), `POST /api/orgs/{org}/kbs` (org-level admin), `GET /api/orgs/{org}/kbs` (org-level viewer+, `{items, next_cursor}` envelope), plus `POST /api/kb/{id}/ask`, `POST|DELETE /api/kb/{id}/documents[/{docId}]`, `GET|DELETE /api/kb/{id}`.

**Users:** there is NO signup endpoint in M1 (matches §16.2, which lists only login/refresh/logout under `/api/auth/*`). Users are seeded directly via the authz store — `authzStore.CreateUser(ctx, email, password.Hash(plain))` — in tests and any bootstrap script. The plan adopts this test-seeding approach for M1 (minimal); a `--seed-user` CLI flag is deferred.

The ask/limit handlers are tested via `httptest` with a fake `RoleResolver` (no DB). The org/kb create+list handlers use the concrete `*orgkb.Repo` + real authz `RoleResolver`, so their httptest test (`TestOrgEndpointsBootstrapAndRBAC`) is gated on `LLM_AGENT_KB_PG_URL` (no pgvector needed — no rag ops).

- [ ] **Step 1: Write the failing test**

Create `internal/httpapi/httpapi_test.go`:
```go
package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/costa92/llm-agent-authz/password"
	authzrole "github.com/costa92/llm-agent-authz/role"
	authzstore "github.com/costa92/llm-agent-authz/store"
	authztoken "github.com/costa92/llm-agent-authz/token"

	"github.com/costa92/llm-agent-kb/internal/orgkb"
	"github.com/costa92/llm-agent-kb/internal/retrieval"
)

// fakeResolver implements authzhttp.RoleResolver semantics for tests.
type fakeResolver struct{ role authzrole.Role }

func (f fakeResolver) ResolveRole(_ context.Context, _, _, _, _ string) (authzrole.Role, error) {
	return f.role, nil
}

// fakeOrgLookup returns a fixed org for any kb.
type fakeOrgLookup struct{}

func (fakeOrgLookup) OrgIDForKB(_ context.Context, _ string) (string, error) { return "org-1", nil }

// fakeAsk implements the Asker the ask handler needs.
type fakeAsk struct{ out retrieval.AskOutput }

func (f fakeAsk) Ask(context.Context, retrieval.AskInput) (retrieval.AskOutput, error) {
	return f.out, nil
}

func bearer(t *testing.T, iss *authztoken.Issuer, uid string) string {
	t.Helper()
	tok, err := iss.Issue(uid, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return "Bearer " + tok
}

func TestAskRequiresViewer(t *testing.T) {
	iss := authztoken.NewIssuer([]byte("s"), time.Minute)
	deps := Deps{
		Issuer:       iss,
		RoleResolver: fakeResolver{role: authzrole.RoleNone}, // no membership
		OrgLookup:    fakeOrgLookup{},
		Asker:        fakeAsk{out: retrieval.AskOutput{Answer: "x"}},
		PerUserLimit: 0,
	}
	mux := NewMux(deps)
	req := httptest.NewRequest("POST", "/api/kb/kb-1/ask", strings.NewReader(`{"q":"hi","mode":"vector"}`))
	req.Header.Set("Authorization", bearer(t, iss, "u1"))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("no-membership ask code=%d want 403", rec.Code)
	}
}

func TestAskAllowedForViewer(t *testing.T) {
	iss := authztoken.NewIssuer([]byte("s"), time.Minute)
	deps := Deps{
		Issuer:       iss,
		RoleResolver: fakeResolver{role: authzrole.RoleViewer},
		OrgLookup:    fakeOrgLookup{},
		Asker:        fakeAsk{out: retrieval.AskOutput{Answer: "the answer", Citations: []retrieval.Citation{}}},
	}
	mux := NewMux(deps)
	req := httptest.NewRequest("POST", "/api/kb/kb-1/ask", strings.NewReader(`{"q":"hi","mode":"vector"}`))
	req.Header.Set("Authorization", bearer(t, iss, "u1"))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("viewer ask code=%d body=%s want 200", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "the answer") {
		t.Fatalf("body=%s", rec.Body)
	}
}

func TestAskUnauthenticated401(t *testing.T) {
	iss := authztoken.NewIssuer([]byte("s"), time.Minute)
	mux := NewMux(Deps{Issuer: iss, RoleResolver: fakeResolver{role: authzrole.RoleViewer}, OrgLookup: fakeOrgLookup{}, Asker: fakeAsk{}})
	req := httptest.NewRequest("POST", "/api/kb/kb-1/ask", strings.NewReader(`{"q":"hi","mode":"vector"}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no-token ask code=%d want 401", rec.Code)
	}
}

func TestRateLimitExceeded429(t *testing.T) {
	iss := authztoken.NewIssuer([]byte("s"), time.Minute)
	deps := Deps{
		Issuer:       iss,
		RoleResolver: fakeResolver{role: authzrole.RoleViewer},
		OrgLookup:    fakeOrgLookup{},
		Asker:        fakeAsk{out: retrieval.AskOutput{Answer: "x"}},
		PerUserLimit: 1,
	}
	mux := NewMux(deps)
	do := func() int {
		req := httptest.NewRequest("POST", "/api/kb/kb-1/ask", strings.NewReader(`{"q":"hi","mode":"vector"}`))
		req.Header.Set("Authorization", bearer(t, iss, "u1"))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}
	if do() != http.StatusOK {
		t.Fatal("first request should pass")
	}
	if do() != http.StatusTooManyRequests {
		t.Fatal("second request should be rate-limited (429)")
	}
}

// --- Org/kb create+list endpoints (H1), gated on a live DB ---
// These routes use the concrete *orgkb.Repo (CreateOrg/Create/ListByOrg) and a
// real authz RoleResolver, so they need a DB (no pgvector required — no rag
// ops). Gated on LLM_AGENT_KB_PG_URL.

func openOrgEndpointDeps(t *testing.T, ctx context.Context) (Deps, *authzstore.Store, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("LLM_AGENT_KB_PG_URL")
	if dsn == "" {
		t.Skipf("set %s to run org-endpoint tests", "LLM_AGENT_KB_PG_URL")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	for _, tbl := range []string{"document", "knowledge_base", "auth_membership", "auth_session", "auth_user", "auth_org"} {
		_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS "+tbl+" CASCADE")
	}
	az := authzstore.New(pool)
	if err := az.Migrate(ctx); err != nil {
		t.Fatalf("authz migrate: %v", err)
	}
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS knowledge_base (
		id TEXT PRIMARY KEY, org_id TEXT NOT NULL, name TEXT NOT NULL,
		namespace TEXT NOT NULL UNIQUE, embedding_model TEXT NOT NULL DEFAULT '',
		embedding_dim INT NOT NULL DEFAULT 0, created_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		t.Fatalf("create knowledge_base: %v", err)
	}
	repo := orgkb.New(pool, az)
	iss := authztoken.NewIssuer([]byte("s"), time.Minute)
	return Deps{
		Issuer:       iss,
		RoleResolver: az, // real resolver: org-level org_admin matches kb scope
		OrgLookup:    repo,
		KBRepo:       repo,
	}, az, pool
}

func TestOrgEndpointsBootstrapAndRBAC(t *testing.T) {
	ctx := context.Background()
	deps, az, pool := openOrgEndpointDeps(t, ctx)
	mux := NewMux(deps)

	// Seed two users directly via the authz store (no signup endpoint in M1).
	hash, _ := password.Hash("pw")
	bossID, err := az.CreateUser(ctx, "boss@x.com", hash)
	if err != nil {
		t.Fatal(err)
	}
	outsiderID, _ := az.CreateUser(ctx, "outsider@x.com", hash)

	post := func(uid, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		req.Header.Set("Authorization", bearer(t, deps.Issuer, uid))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	// 1. POST /api/orgs — any authenticated user; creator becomes org_admin.
	rec := post(bossID, "/api/orgs", `{"name":"Acme"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/orgs code=%d body=%s want 200", rec.Code, rec.Body)
	}
	var org struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &org); err != nil || org.ID == "" {
		t.Fatalf("create org body=%s err=%v", rec.Body, err)
	}
	if got, _ := az.ResolveRole(ctx, bossID, org.ID, "kb", "any"); got != authzrole.RoleOrgAdmin {
		t.Fatalf("creator role=%q want org_admin", got)
	}

	// 2. POST /api/orgs/{org}/kbs — 403 for a non-admin outsider.
	if rec := post(outsiderID, "/api/orgs/"+org.ID+"/kbs", `{"name":"Docs","embeddingDim":8}`); rec.Code != http.StatusForbidden {
		t.Fatalf("outsider create-kb code=%d want 403", rec.Code)
	}
	// 200 for the org_admin; writes creator kb-admin.
	rec = post(bossID, "/api/orgs/"+org.ID+"/kbs", `{"name":"Docs","embeddingDim":8}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("org_admin create-kb code=%d body=%s want 200", rec.Code, rec.Body)
	}
	var kb struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &kb)
	// Create writes the creator's kb-scope admin row. ResolveRole would return
	// org_admin here (org-level row outranks it in the merge), so assert the
	// kb-scope row exists directly.
	var kbScopeRole string
	if err := pool.QueryRow(ctx,
		`SELECT role FROM auth_membership WHERE user_id=$1 AND org_id=$2 AND scope_kind='kb' AND scope_id=$3`,
		bossID, org.ID, kb.ID).Scan(&kbScopeRole); err != nil {
		t.Fatalf("kb-scope membership row not found: %v", err)
	}
	if kbScopeRole != string(authzrole.RoleAdmin) {
		t.Fatalf("kb-scope role=%q want admin", kbScopeRole)
	}

	// 3. GET /api/orgs/{org}/kbs — RBAC-gated; org_admin (viewer+) sees the kb.
	get := func(uid string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/api/orgs/"+org.ID+"/kbs", nil)
		req.Header.Set("Authorization", bearer(t, deps.Issuer, uid))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	if rec := get(outsiderID); rec.Code != http.StatusForbidden {
		t.Fatalf("outsider list code=%d want 403", rec.Code)
	}
	rec = get(bossID)
	if rec.Code != http.StatusOK {
		t.Fatalf("org_admin list code=%d body=%s want 200", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), kb.ID) || !strings.Contains(rec.Body.String(), "next_cursor") {
		t.Fatalf("list body=%s want kb id + next_cursor envelope", rec.Body)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/httpapi/ -v`
Expected: FAIL to compile — undefined `NewMux`, `Deps`, etc.

- [ ] **Step 3: Write the implementation**

Create `internal/httpapi/httpapi.go`:
```go
// Package httpapi mounts the kb REST routes (spec §16.2 M1 subset) and wires
// the RBAC middleware chain over authz. It holds no business rules: it
// orchestrates auth + limits + JSON only.
package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	authzhttp "github.com/costa92/llm-agent-authz/httpapi"
	authzrole "github.com/costa92/llm-agent-authz/role"
	authztoken "github.com/costa92/llm-agent-authz/token"

	"github.com/costa92/llm-agent-kb/internal/ingest"
	"github.com/costa92/llm-agent-kb/internal/limits"
	"github.com/costa92/llm-agent-kb/internal/orgkb"
	"github.com/costa92/llm-agent-kb/internal/retrieval"
)

// Asker is the retrieval surface the ask handler needs (satisfied by *retrieval.Service).
type Asker interface {
	Ask(ctx context.Context, in retrieval.AskInput) (retrieval.AskOutput, error)
}

// OrgLookup resolves a kb's org (satisfied by *orgkb.Repo).
type OrgLookup interface {
	OrgIDForKB(ctx context.Context, kbID string) (string, error)
}

// Ingester is the document-ingest surface (satisfied by *ingest.Service).
// DeleteAllDocumentsForKB runs the §16.4 cascade over every doc in a kb and is
// used by the delete-kb handler before the kb row + memberships are removed.
type Ingester interface {
	Ingest(ctx context.Context, in ingest.IngestInput) (ingest.Result, error)
	DeleteDocument(ctx context.Context, namespace, documentID string) error
	DeleteAllDocumentsForKB(ctx context.Context, namespace, kbID string) error
}

// Deps are the dependencies NewMux wires together.
type Deps struct {
	Issuer       *authztoken.Issuer
	AuthHandlers *authzhttp.Handlers // /api/auth/*; nil in unit tests that skip auth routes
	RoleResolver authzhttp.RoleResolver
	OrgLookup    OrgLookup
	Asker        Asker
	Ingester     Ingester
	KBRepo       *orgkb.Repo // used by kb CRUD handlers; nil in focused unit tests
	PerUserLimit int
}

// kbScopeFromRequest builds the ScopeFromRequest closure for kb-scoped routes
// (path `{id}`): kbID from the path, orgID via the OrgLookup. On a missing kb it
// returns ("",kbID) which resolves to RoleNone → 403, the safe default.
func kbScopeFromRequest(lookup OrgLookup) authzhttp.ScopeFromRequest {
	return func(r *http.Request) (string, string) {
		kbID := r.PathValue("id")
		orgID, err := lookup.OrgIDForKB(r.Context(), kbID)
		if err != nil {
			return "", kbID
		}
		return orgID, kbID
	}
}

// orgScopeFromRequest builds the ScopeFromRequest for ORG-scoped routes
// (path `{org}`): orgID from the path, scopeID="" so ResolveRole(...,"kb","")
// matches the org-level membership row (`scope_id IS NULL OR scope_id=$4`). An
// org_admin (rank 4) thus satisfies the kb admin/viewer minimum on these routes.
func orgScopeFromRequest(r *http.Request) (string, string) {
	return r.PathValue("org"), ""
}

// NewMux builds the kb ServeMux with the auth routes and the RBAC-guarded kb routes.
func NewMux(d Deps) *http.ServeMux {
	mux := http.NewServeMux()
	if d.AuthHandlers != nil {
		d.AuthHandlers.Mount(mux, "/api/auth")
	}
	guard := limits.New(d.PerUserLimit)
	kbScope := kbScopeFromRequest(d.OrgLookup)

	// authOnly composes: Authenticate → per-user limit → handler. No scope role
	// check — used by POST /api/orgs (any authenticated user may create an org).
	authOnly := func(h http.HandlerFunc) http.Handler {
		var handler http.Handler = withUserLimit(guard, h)
		return authzhttp.Authenticate(d.Issuer)(handler)
	}
	// scoped composes: Authenticate → RequireScopeRole(min, scope) → per-user
	// limit → handler, for an arbitrary ScopeFromRequest.
	scoped := func(min authzrole.Role, scope authzhttp.ScopeFromRequest, h http.HandlerFunc) http.Handler {
		var handler http.Handler = withUserLimit(guard, h)
		handler = authzhttp.RequireScopeRole(d.RoleResolver, "kb", min, scope)(handler)
		return authzhttp.Authenticate(d.Issuer)(handler)
	}
	// chain is the kb-scoped (`{id}`) shorthand.
	chain := func(min authzrole.Role, h http.HandlerFunc) http.Handler {
		return scoped(min, kbScope, h)
	}

	// Org bootstrap + kb create/list (§16.2). Wired only when KBRepo is set.
	if d.KBRepo != nil {
		// POST /api/orgs — any authenticated user; creator becomes org_admin.
		mux.Handle("POST /api/orgs", authOnly(createOrgHandler(d.KBRepo)))
		// POST/GET /api/orgs/{org}/kbs — org-level RBAC (org_admin satisfies it).
		mux.Handle("POST /api/orgs/{org}/kbs", scoped(authzrole.RoleAdmin, orgScopeFromRequest, createKBHandler(d.KBRepo)))
		mux.Handle("GET /api/orgs/{org}/kbs", scoped(authzrole.RoleViewer, orgScopeFromRequest, listKBHandler(d.KBRepo)))
	}

	// Q&A — viewer+.
	mux.Handle("POST /api/kb/{id}/ask", chain(authzrole.RoleViewer, askHandler(d.Asker)))
	// Documents — upload requires editor+, read viewer+, delete editor+.
	if d.Ingester != nil && d.KBRepo != nil {
		mux.Handle("POST /api/kb/{id}/documents", chain(authzrole.RoleEditor, uploadHandler(d.KBRepo, d.Ingester)))
		mux.Handle("DELETE /api/kb/{id}/documents/{docId}", chain(authzrole.RoleEditor, deleteDocHandler(d.KBRepo, d.Ingester)))
		// kb resource — delete is admin (cascade wired in deleteKBHandler).
		mux.Handle("GET /api/kb/{id}", chain(authzrole.RoleViewer, getKBHandler(d.KBRepo)))
		mux.Handle("DELETE /api/kb/{id}", chain(authzrole.RoleAdmin, deleteKBHandler(d.KBRepo, d.Ingester)))
	}
	return mux
}

// withUserLimit enforces the per-user request budget after auth (so UserID is set).
func withUserLimit(g *limits.Guard, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := authzhttp.UserID(r.Context())
		if !g.Allow(uid) {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func askHandler(asker Asker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Q    string `json:"q"`
			Mode string `json:"mode"`
			TopK int    `json:"topK"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		out, err := asker.Ask(r.Context(), retrieval.AskInput{
			Namespace: "kb_" + r.PathValue("id"), // namespace = "kb_"+id (orgkb.Create convention)
			Question:  req.Q,
			Mode:      req.Mode,
			TopK:      req.TopK,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}
```

Create the document/kb/org handlers in the same package — `internal/httpapi/handlers.go`:
```go
package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	authzhttp "github.com/costa92/llm-agent-authz/httpapi"

	"github.com/costa92/llm-agent-kb/internal/ingest"
	"github.com/costa92/llm-agent-kb/internal/orgkb"
)

// maxUploadBytes caps a single document body in M1 (full upload validation is M2).
const maxUploadBytes = 10 << 20 // 10 MiB

func uploadHandler(repo *orgkb.Repo, ing Ingester) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		kb, err := repo.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			http.Error(w, "kb not found", http.StatusNotFound)
			return
		}
		// JSON paste/markdown/txt body: {title, sourceType, content}. (multipart
		// file upload is wired the same way; M1 accepts the JSON shape.)
		var req struct {
			Title      string `json:"title"`
			SourceType string `json:"sourceType"`
			Content    string `json:"content"`
		}
		body := http.MaxBytesReader(w, r.Body, maxUploadBytes)
		raw, err := io.ReadAll(body)
		if err != nil {
			http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
			return
		}
		if err := json.Unmarshal(raw, &req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		res, err := ing.Ingest(r.Context(), ingest.IngestInput{
			KBID:       kb.ID,
			Namespace:  kb.Namespace,
			Title:      req.Title,
			SourceType: ingest.SourceType(req.SourceType),
			Raw:        []byte(req.Content),
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"documentId": res.DocumentID, "status": res.Status, "chunkCount": res.ChunkCount,
		})
	}
}

func deleteDocHandler(repo *orgkb.Repo, ing Ingester) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		kb, err := repo.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			http.Error(w, "kb not found", http.StatusNotFound)
			return
		}
		if err := ing.DeleteDocument(r.Context(), kb.Namespace, r.PathValue("docId")); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func getKBHandler(repo *orgkb.Repo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		kb, err := repo.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			http.Error(w, "kb not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id": kb.ID, "orgId": kb.OrgID, "name": kb.Name, "namespace": kb.Namespace,
		})
	}
}

// createOrgHandler (POST /api/orgs) is the bootstrap seam: any authenticated
// user creates an org and is written as that org's org_admin (orgkb.CreateOrg).
func createOrgHandler(repo *orgkb.Repo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := authzhttp.UserID(r.Context())
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
			http.Error(w, "bad request: name required", http.StatusBadRequest)
			return
		}
		orgID, err := repo.CreateOrg(r.Context(), req.Name, uid)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": orgID, "name": req.Name})
	}
}

// createKBHandler (POST /api/orgs/{org}/kbs) requires org-level admin. Create
// writes the creator's kb-scope admin membership (orgkb.Create).
func createKBHandler(repo *orgkb.Repo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := authzhttp.UserID(r.Context())
		var req struct {
			Name           string `json:"name"`
			EmbeddingModel string `json:"embeddingModel"`
			EmbeddingDim   int    `json:"embeddingDim"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
			http.Error(w, "bad request: name required", http.StatusBadRequest)
			return
		}
		kb, err := repo.Create(r.Context(), orgkb.CreateInput{
			OrgID:          r.PathValue("org"),
			Name:           req.Name,
			CreatorUserID:  uid,
			EmbeddingModel: req.EmbeddingModel,
			EmbeddingDim:   req.EmbeddingDim,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id": kb.ID, "orgId": kb.OrgID, "name": kb.Name, "namespace": kb.Namespace,
		})
	}
}

// listKBHandler (GET /api/orgs/{org}/kbs) requires org-level viewer+. Returns
// the §16.2 cursor envelope {items, next_cursor}.
func listKBHandler(repo *orgkb.Repo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 0
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				limit = n
			}
		}
		items, next, err := repo.ListByOrg(r.Context(), r.PathValue("org"), limit, r.URL.Query().Get("cursor"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		out := make([]map[string]any, 0, len(items))
		for _, kb := range items {
			out = append(out, map[string]any{
				"id": kb.ID, "orgId": kb.OrgID, "name": kb.Name, "namespace": kb.Namespace,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": out, "next_cursor": next})
	}
}

// deleteKBHandler applies the §16.4 delete-kb cascade in strict order:
// (1) delete every document in the kb (chunks + graph reconcile, via the
// widened Ingester), then (2) repo.DeleteRow, which removes the knowledge_base
// row AND the kb-scope auth_membership rows in one transaction (Task 6).
func deleteKBHandler(repo *orgkb.Repo, ing Ingester) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		kb, err := repo.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			http.Error(w, "kb not found", http.StatusNotFound)
			return
		}
		// 1. Cascade-delete all documents (chunks + graph) for the kb.
		if err := ing.DeleteAllDocumentsForKB(r.Context(), kb.Namespace, kb.ID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// 2. Delete the kb row + its kb-scope memberships (one tx, §16.4).
		if err := repo.DeleteRow(r.Context(), kb.ID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
```

> Note: `*ingest.Service` satisfies the widened `Ingester` interface via `DeleteAllDocumentsForKB` (Task 8). The authz membership cleanup is the `DELETE FROM auth_membership WHERE scope_kind='kb' AND scope_id=$1` exec inside `orgkb.Repo.DeleteRow`'s transaction (Task 6) — committed there, not here. The httpapi unit tests in this task exercise the ask/limit paths only (no DB); the delete-kb cascade is covered end-to-end by the cmd/kbd smoke test (Task 11, gated on PG).

- [ ] **Step 4: Run test to verify it passes**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/httpapi/ -v` (4 ask/limit tests run always). With `LLM_AGENT_KB_PG_URL` set, `TestOrgEndpointsBootstrapAndRBAC` also runs (org bootstrap + create/list RBAC).
Expected: PASS (org-endpoint test SKIPs without PG, PASSes with it). The document/kb-delete handlers are covered end-to-end in Task 11's `cmd/kbd` smoke test.

- [ ] **Step 5: Commit**

```bash
cd llm-agent-kb && git add internal/httpapi/ && \
git commit -m "feat(httpapi): §16.2 M1 routes (orgs bootstrap + kb create/list + ask/docs/kb) + RBAC chain (kb + org scope resolvers) + per-user limit"
```

---

## Task 11: obs + cmd/kbd assembly + end-to-end smoke test

**Files:**
- Create: `llm-agent-kb/internal/obs/obs.go`
- Create: `llm-agent-kb/cmd/kbd/main.go`
- Test: `llm-agent-kb/cmd/kbd/main_test.go`

`obs` builds the otel TracerProvider; `cmd/kbd` assembles everything in dependency order. The smoke test (gated on PG) drives the full M1 success path over real HTTP: seed user → `/api/auth/login` → `POST /api/orgs` → `POST /api/orgs/{org}/kbs` → upload paste → ask (hybrid, ≥1 citation) → delete document → assert chunks gone — proving login + RBAC + ingest + ask + delete-cascade end-to-end.

- [ ] **Step 1: Write `internal/obs/obs.go`**

```go
// Package obs assembles the otel TracerProvider for kbd.
package obs

import (
	"context"

	otelexport "github.com/costa92/llm-agent-otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Config selects the OTLP exporter target.
type Config struct {
	ServiceName  string
	Endpoint     string
	Protocol     string
	Insecure     bool
}

// NewTracerProvider builds an SDK TracerProvider via the otel helper. The
// caller owns Shutdown.
func NewTracerProvider(ctx context.Context, cfg Config) (*sdktrace.TracerProvider, error) {
	return otelexport.NewTracerProvider(ctx, otelexport.ExporterConfig{
		Protocol: cfg.Protocol,
		Endpoint: cfg.Endpoint,
		Insecure: cfg.Insecure,
	})
}
```
> Note: the otel module's package is `package otel` at import path `github.com/costa92/llm-agent-otel`; alias it `otelexport` to avoid colliding with `go.opentelemetry.io/otel`.

- [ ] **Step 2: Write `cmd/kbd/main.go`**

```go
// Command kbd is the llm-agent-kb backend server.
package main

import (
	"context"
	"log"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	authzhttp "github.com/costa92/llm-agent-authz/httpapi"
	authzsvc "github.com/costa92/llm-agent-authz/service"
	authzstore "github.com/costa92/llm-agent-authz/store"
	authztoken "github.com/costa92/llm-agent-authz/token"
	"github.com/costa92/llm-agent-contract/llm"
	ollamaprovider "github.com/costa92/llm-agent-providers/ollama"
	openaiprovider "github.com/costa92/llm-agent-providers/openai"

	"github.com/costa92/llm-agent-kb/internal/config"
	"github.com/costa92/llm-agent-kb/internal/httpapi"
	"github.com/costa92/llm-agent-kb/internal/ingest"
	"github.com/costa92/llm-agent-kb/internal/obs"
	"github.com/costa92/llm-agent-kb/internal/orgkb"
	"github.com/costa92/llm-agent-kb/internal/ragsvc"
	"github.com/costa92/llm-agent-kb/internal/retrieval"
	"github.com/costa92/llm-agent-kb/internal/storage"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("kbd: config: %v", err)
	}
	ctx := context.Background()

	app, cleanup, err := build(ctx, cfg)
	if err != nil {
		log.Fatalf("kbd: build: %v", err)
	}
	defer cleanup()

	log.Printf("kbd: listening on %s", cfg.HTTPAddr)
	if err := http.ListenAndServe(cfg.HTTPAddr, app); err != nil {
		log.Fatalf("kbd: serve: %v", err)
	}
}

// build wires every dependency and returns the root handler + a cleanup func.
// Exported-shape (lowercase but package-visible) so main_test.go can drive it.
func build(ctx context.Context, cfg config.Config) (http.Handler, func(), error) {
	tp, err := obs.NewTracerProvider(ctx, obs.Config{
		ServiceName: cfg.ServiceName, Endpoint: cfg.OTLPEndpoint,
		Protocol: cfg.OTLPProtocol, Insecure: cfg.OTLPInsecure,
	})
	if err != nil {
		return nil, nil, err
	}

	st, err := storage.Open(ctx, storage.Config{PGURL: cfg.PGURL, EmbeddingDim: cfg.EmbeddingDim})
	if err != nil {
		return nil, nil, err
	}
	if err := st.Migrate(ctx); err != nil {
		st.Close()
		return nil, nil, err
	}

	az := authzstore.New(st.Pool())
	if err := az.Migrate(ctx); err != nil {
		st.Close()
		return nil, nil, err
	}

	model, embedder, err := buildProviders(cfg)
	if err != nil {
		st.Close()
		return nil, nil, err
	}

	rag := ragsvc.New(ragsvc.Deps{
		Model: model, Embedder: embedder,
		RagStore: st.RagStore(), ChunkStore: st.RagStore(),
		Tracer: tp,
	})

	kbRepo := orgkb.New(st.Pool(), az)
	ingestSvc := ingest.New(st.Pool(), rag)
	retrievalSvc := retrieval.New(rag, retrieval.Config{MaxAskTokens: cfg.MaxAskTokens})

	issuer := authztoken.NewIssuer([]byte(cfg.JWTSecret), cfg.AccessTTL)
	authService := authzsvc.New(az, issuer, cfg.RefreshTTL)
	authHandlers := authzhttp.New(authService)

	mux := httpapi.NewMux(httpapi.Deps{
		Issuer:       issuer,
		AuthHandlers: authHandlers,
		RoleResolver: az, // *store.Store satisfies authzhttp.RoleResolver
		OrgLookup:    kbRepo,
		Asker:        retrievalSvc,
		Ingester:     ingestSvc,
		KBRepo:       kbRepo,
		PerUserLimit: cfg.MaxRequestsPerUserPerMinute,
	})

	cleanup := func() {
		_ = tp.Shutdown(ctx)
		st.Close()
	}
	return mux, cleanup, nil
}

func buildProviders(cfg config.Config) (llm.ChatModel, llm.Embedder, error) {
	chat, err := buildChat(cfg)
	if err != nil {
		return nil, nil, err
	}
	emb, err := buildEmbedder(cfg)
	if err != nil {
		return nil, nil, err
	}
	return chat, emb, nil
}

func buildChat(cfg config.Config) (llm.ChatModel, error) {
	switch cfg.Provider {
	case config.ProviderOpenAI:
		return openaiprovider.New(
			openaiprovider.WithModel(cfg.Model),
			openaiprovider.WithAPIKey(cfg.OpenAIAPIKey),
			openaiprovider.WithBaseURL(cfg.OpenAIBaseURL),
		)
	default: // ollama
		opts := []ollamaprovider.Option{ollamaprovider.WithModel(cfg.Model)}
		if cfg.OllamaBaseURL != "" {
			opts = append(opts, ollamaprovider.WithBaseURL(cfg.OllamaBaseURL))
		}
		return ollamaprovider.New(opts...)
	}
}

func buildEmbedder(cfg config.Config) (llm.Embedder, error) {
	switch cfg.EmbeddingProvider {
	case config.ProviderOpenAI:
		return openaiprovider.New(
			openaiprovider.WithModel(cfg.EmbeddingModel),
			openaiprovider.WithAPIKey(cfg.OpenAIAPIKey),
			openaiprovider.WithBaseURL(cfg.OpenAIBaseURL),
		)
	default: // ollama
		opts := []ollamaprovider.Option{ollamaprovider.WithModel(cfg.EmbeddingModel)}
		if cfg.OllamaBaseURL != "" {
			opts = append(opts, ollamaprovider.WithBaseURL(cfg.OllamaBaseURL))
		}
		return ollamaprovider.New(opts...)
	}
}
```

- [ ] **Step 3: Write the smoke test `cmd/kbd/main_test.go`**

This test bypasses real providers by injecting `llm.NewScriptedLLM` — so it does not need ollama/openai, only PG. It drives `build`-shaped wiring via a small test seam: factor the provider build out so the test can pass scripted models. Add to `main.go`:
```go
// buildWith is build with injectable models (used by tests to avoid real providers).
var providerOverride func(config.Config) (llm.ChatModel, llm.Embedder, error)
```
and in `buildProviders`, prefer the override:
```go
func buildProviders(cfg config.Config) (llm.ChatModel, llm.Embedder, error) {
	if providerOverride != nil {
		return providerOverride(cfg)
	}
	// ... existing buildChat/buildEmbedder ...
}
```
Then `cmd/kbd/main_test.go`:
```go
package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/costa92/llm-agent-authz/password"
	authzstore "github.com/costa92/llm-agent-authz/store"
	"github.com/costa92/llm-agent-contract/llm"

	"github.com/costa92/llm-agent-kb/internal/config"
)

// TestEndToEndLoginCreateKBUploadAskDelete drives the full M1 success path
// (spec §1, scoped to M1) over the real HTTP surface on live pgvector:
// seed user → login → POST /api/orgs → POST /api/orgs/{org}/kbs → upload paste →
// ask (hybrid) → assert ≥1 citation → delete document → assert chunks gone.
// Providers are scripted (no ollama/openai); only PG is required.
func TestEndToEndLoginCreateKBUploadAskDelete(t *testing.T) {
	dsn := os.Getenv("LLM_AGENT_KB_PG_URL")
	if dsn == "" {
		t.Skipf("set LLM_AGENT_KB_PG_URL (pgvector) to run the e2e smoke test")
	}
	ctx := context.Background()
	cleanDB(t, ctx, dsn)

	providerOverride = func(config.Config) (llm.ChatModel, llm.Embedder, error) {
		return llm.NewScriptedLLM(llm.WithResponses(llm.Response{Text: "scripted answer"})),
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

	// Seed a user directly via the authz store (no signup endpoint in M1).
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	hash, err := password.Hash("pw")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authzstore.New(pool).CreateUser(ctx, "e2e@x.com", hash); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	client := srv.Client()
	// do issues a JSON request with optional bearer and returns status + decoded body.
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

	// 1. Login → access_token.
	code, body := do("POST", "/api/auth/login", "", `{"Email":"e2e@x.com","Password":"pw"}`)
	if code != http.StatusOK {
		t.Fatalf("login code=%d body=%v want 200", code, body)
	}
	token, _ := body["access_token"].(string)
	if token == "" {
		t.Fatalf("login returned no access_token: %v", body)
	}

	// 2. POST /api/orgs → creator becomes org_admin.
	code, body = do("POST", "/api/orgs", token, `{"name":"Acme"}`)
	if code != http.StatusOK {
		t.Fatalf("create org code=%d body=%v want 200", code, body)
	}
	orgID, _ := body["id"].(string)

	// 3. POST /api/orgs/{org}/kbs (org_admin) → kb id.
	code, body = do("POST", "/api/orgs/"+orgID+"/kbs", token, `{"name":"Docs","embeddingDim":8}`)
	if code != http.StatusOK {
		t.Fatalf("create kb code=%d body=%v want 200", code, body)
	}
	kbID, _ := body["id"].(string)
	if kbID == "" {
		t.Fatalf("create kb returned no id: %v", body)
	}

	// 4. POST /api/kb/{id}/documents (paste) → chunkCount>0.
	code, body = do("POST", "/api/kb/"+kbID+"/documents", token,
		`{"title":"Doc","sourceType":"paste","content":"the quick brown fox jumps over the lazy dog repeatedly"}`)
	if code != http.StatusOK {
		t.Fatalf("upload code=%d body=%v want 200", code, body)
	}
	docID, _ := body["documentId"].(string)
	if cc, _ := body["chunkCount"].(float64); cc <= 0 {
		t.Fatalf("upload chunkCount=%v want >0", body["chunkCount"])
	}

	// 5. POST /api/kb/{id}/ask (hybrid) → answer + ≥1 citation.
	code, body = do("POST", "/api/kb/"+kbID+"/ask", token, `{"q":"fox","mode":"hybrid","topK":5}`)
	if code != http.StatusOK {
		t.Fatalf("ask code=%d body=%v want 200", code, body)
	}
	if ans, _ := body["answer"].(string); ans != "scripted answer" {
		t.Fatalf("ask answer=%v want 'scripted answer'", body["answer"])
	}
	cites, _ := body["citations"].([]any)
	if len(cites) == 0 {
		t.Fatalf("ask returned 0 citations: %v", body)
	}

	// 6. DELETE /api/kb/{id}/documents/{docId} → 204, chunks gone.
	req, _ := http.NewRequest("DELETE", srv.URL+"/api/kb/"+kbID+"/documents/"+docID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("delete doc: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete doc code=%d want 204", resp.StatusCode)
	}
	var remaining int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM chunks WHERE namespace = $1`, "kb_"+kbID).Scan(&remaining); err != nil {
		t.Fatalf("count chunks: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("chunks remain after document delete: %d", remaining)
	}
}

// cleanDB drops business + authz + rag tables for a deterministic run.
func cleanDB(t *testing.T, ctx context.Context, dsn string) {
	t.Helper()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("cleanDB pool: %v", err)
	}
	defer pool.Close()
	for _, tbl := range []string{
		"document", "knowledge_base",
		"chunks", "chunks_entities", "chunks_relations", "chunks_communities", "chunks_community_reports",
		"auth_membership", "auth_session", "auth_user", "auth_org",
	} {
		_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS "+tbl+" CASCADE")
	}
}
```
> Note: the chunks table name is `chunks` (rag `postgres.Config.Table` default); the namespace is `"kb_"+kbID` (orgkb.Create convention). The ask uses `mode:"hybrid"` to exercise the rerank path; the scripted embedder makes retrieval deterministic.

- [ ] **Step 4: Build, vet, and run the full suite**

Run:
```bash
cd llm-agent-kb && GOWORK=off go build ./... && GOWORK=off go vet ./...
# pure suites (always):
GOWORK=off go test ./internal/config/ ./internal/ragsvc/ ./internal/ingest/ ./internal/retrieval/ ./internal/limits/ ./internal/httpapi/
# gated suites (with pgvector PG):
docker run -d --rm --name kb-pg -e POSTGRES_PASSWORD=pw -p 55433:5432 pgvector/pgvector:pg16
export LLM_AGENT_KB_PG_URL='postgres://postgres:pw@localhost:55433/postgres'
GOWORK=off go test ./... && echo ALL_GREEN
docker stop kb-pg
```
Expected: build+vet clean; pure suites PASS; with PG up, `ALL_GREEN`. (Note: `ragsvc`/`ingest` parse + adapter tests pass without PG; storage/orgkb/cascade/e2e tests SKIP without PG and PASS with it.)

- [ ] **Step 5: Commit**

```bash
cd llm-agent-kb && git add internal/obs/ cmd/ && \
git commit -m "feat(cmd): kbd assembly (config→obs→storage→authz→providers→ragsvc→httpapi) + e2e smoke test"
```

---

## Task 12: docker-compose + README

**Files:**
- Create: `llm-agent-kb/docker-compose.yml`
- Create: `llm-agent-kb/README.md`

- [ ] **Step 1: Write `docker-compose.yml`**

```yaml
services:
  postgres:
    image: pgvector/pgvector:pg16
    environment:
      POSTGRES_PASSWORD: kb
      POSTGRES_DB: kb
    ports:
      - "5432:5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres -d kb"]
      interval: 5s
      timeout: 3s
      retries: 10
    volumes:
      - kb_pgdata:/var/lib/postgresql/data

  # Optional key-free local LLM + embeddings.
  ollama:
    image: ollama/ollama:latest
    ports:
      - "11434:11434"
    volumes:
      - kb_ollama:/root/.ollama

  kbd:
    build: .
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      HTTP_ADDR: ":8080"
      PG_URL: "postgres://postgres:kb@postgres:5432/kb?sslmode=disable"
      LLM_PROVIDER: "ollama"
      LLM_MODEL: "llama3.1"
      EMBEDDING_PROVIDER: "ollama"
      EMBEDDING_MODEL: "nomic-embed-text"
      EMBEDDING_DIM: "768"
      OLLAMA_HOST: "http://ollama:11434"
      JWT_SECRET: "dev-insecure-secret-change-me"
      OTEL_EXPORTER_OTLP_ENDPOINT: "http://localhost:4318"
    ports:
      - "8080:8080"

volumes:
  kb_pgdata:
  kb_ollama:
```
> Note: `kbd` runs business migrations + `authz.Migrate` + `ragStore.Migrate` on boot (Task 11 `build`). `EMBEDDING_DIM` MUST match the embedding model's true dimension (`nomic-embed-text` = 768). Also add a `Dockerfile` (multi-stage `golang:1.26` build → `gcr.io/distroless/static`); the executor writes a standard two-stage Dockerfile building `./cmd/kbd`.

- [ ] **Step 2: Write `README.md`**

```markdown
# llm-agent-kb

Enterprise knowledge-base GraphRAG Q&A platform (backend). M1 delivers: authz-backed login/JWT/RBAC, knowledge-base CRUD, Markdown/TXT/paste ingest (synchronous), vector + hybrid `Ask` with citations, single-document delete with cascade cleanup, basic per-user rate limiting, otel tracing, and docker-compose. PDF/DOCX/URL ingest, async workers, GraphRAG global/drift, eval dashboards, and the React frontend are later milestones.

## Run

```bash
docker compose up --build
# pull models once: docker compose exec ollama ollama pull llama3.1 nomic-embed-text
```

## Architecture

Single Go binary `kbd` embedding a llm-agent-rag System. Layered internal packages: config · storage (pgxpool + business migrations + rag postgres.Store) · ragsvc (the only rag-touching package; RagPort + adapters + otelrag.Wrap) · orgkb · ingest · retrieval · limits · httpapi · obs. Tenancy/auth comes from the imported `llm-agent-authz` library.

## Tests

Pure/unit suites run with `GOWORK=off go test ./internal/config/ ./internal/ragsvc/ ./internal/ingest/ ./internal/retrieval/ ./internal/limits/ ./internal/httpapi/`. Storage/orgkb/cascade/e2e tests need a **pgvector-enabled** Postgres DSN in `LLM_AGENT_KB_PG_URL` (skipped otherwise) — e.g. `pgvector/pgvector:pg16`, NOT vanilla postgres.

## Note

Standalone sibling repo; run Go commands with `GOWORK=off` (the umbrella `go.work` does not list kb). The ecosystem replace-guard pre-commit hook strips local `replace` directives and pins published tags on commit.
```

- [ ] **Step 3: Final verification**

Run: `cd llm-agent-kb && GOWORK=off go build ./... && GOWORK=off go vet ./... && echo OK`
Expected: `OK`.

- [ ] **Step 4: Commit**

```bash
cd llm-agent-kb && git add docker-compose.yml Dockerfile README.md && \
git commit -m "docs+ops: docker-compose (pgvector + ollama + kbd) and README for M1"
```

---

## Self-Review

**Spec coverage (§13 M1 backend list):**
- depend on authz (login/JWT/RBAC) → Task 11 mounts `authzhttp.New(svc).Mount(mux, "/api/auth")`; middleware chain `Authenticate → RequireScopeRole` (Task 10); e2e smoke test logs in via `POST /api/auth/login` (Task 11) ✓
- create/delete/list kb → `orgkb` (Task 6) + httpapi routes: `POST /api/orgs` (org bootstrap → org_admin), `POST /api/orgs/{org}/kbs` (create, org-admin), `GET /api/orgs/{org}/kbs` (list, `{items, next_cursor}` cursor pagination), `GET|DELETE /api/kb/{id}` (Task 10, H1) ✓
- upload MD/TXT + paste plain text → `ingest` source types {markdown, txt, paste}, uploadHandler (Tasks 7, 10); PDF/DOCX/URL explicitly rejected ✓
- Import with SourceID convention → `makeDocument` sets `ID=SourceID=docID`, `Metadata{kb_id,source_type}` (no doc_id), `ReplaceSource=true` (Task 7) ✓
- Ask vector + hybrid → `retrieval.Ask` maps mode→`EnableRerank`; global/drift rejected (Task 9b); RagPort.Ask uses `SearchOptions{Namespace, TopK, EnableRerank}` (NO SecurityFilters — Namespace-only tenant isolation, M2) + `MaxTotalTokens` (Task 5); live Ask test guards HitCount>0 ✓
- citation → `retrieval.Citation` from `rag.Answer.Citations` + snippet from `Hits` (Task 9b) ✓
- single-document delete cascade §16.4 → `ingest.DeleteDocument`: List→RemoveGraphBySource→RemoveChunks→delete row, verified against real signatures (Task 8) ✓; delete-kb cascade + membership removal: `deleteKBHandler` calls `ing.DeleteAllDocumentsForKB` then `repo.DeleteRow`, which deletes the kb row AND `auth_membership WHERE scope_kind='kb' AND scope_id=$kbID` in one tx (Task 10 + Task 6, M1); gated test asserts `ResolveRole(...) == RoleNone` after delete ✓
- basic per-user limit → `limits` fixed-window + httpapi `withUserLimit` 429 (Task 9a, 10) ✓
- docker-compose + otel → Task 12 (pgvector + ollama + kbd) + `obs` TracerProvider + `otelrag.Wrap` (Tasks 5, 11); otel pinned `v0.4.0` (symbols verified at tag, M3) ✓

**Deferred (correctly NOT in M1):** PDF/DOCX/URL + SSRF/upload validation (§16.3, M2); async worker + `ingest_job` (M2); `qa_session`/`qa_message` + QA history (later); `eval_run` + dashboards (M4); `AskGlobal`/`AskDrift`/GraphRAG + `PrewarmCommunityReports` (M3); frontend `web/` (later track). The M1 RagPort deliberately excludes AskGlobal/AskDrift/Prewarm, matching `*otelrag.Wrapper`'s surface. ✓

**Signatures grounded in on-disk source (verified, not assumed):** authz `CreateUser(ctx, email, hash) (id, error)` / `CreateOrg(ctx, name) (id, error)` / `UpsertMembership(ctx, orgID, userID, scopeKind, *scopeID, role.Role)` (nil scopeID → org-level) / `ResolveRole(ctx, userID, orgID, scopeKind, scopeID)` (query `WHERE user_id=$1 AND org_id=$2 AND scope_kind=$3 AND (scope_id IS NULL OR scope_id=$4)`, store/memberships.go:21-23, then `role.Merge`) / `RequireScopeRole(res, kind, min, ScopeFromRequest)` (401 unauth, 403 if `!eff.AtLeast(min)`, middleware.go:54) / `Authenticate(*token.Issuer)` / `UserID(ctx)` / `(*Issuer).Issue(uid, now)` / `password.Hash(plain) (string, error)` (package `.../authz/password`) / `service.New(Store, *Issuer, ttl)` / `httpapi.New(svc).Mount(mux, prefix)` (login body `{Email,Password}` → `{access_token, expires_in}`); role consts `RoleNone/Viewer/Editor/Admin/OrgAdmin` with `Rank` 0/1/2/3/4 and `AtLeast` = rank≥min∧rank>0 — so **org_admin (4) satisfies admin (3)**, the keystone of the H1 bootstrap. rag `New(Options{Model, Embedder, Store})`, `Ask(ctx, q, AskOptions{Search, MaxTotalTokens})`, `Import(ctx, []ingest.Document, ImportOptions{Namespace, ReplaceSource})` → `ImportResult{Chunks int}`, `Citation{ChunkID,DocID,Title,SectionPath,Score}`; postgres `New(pool, Config{Dimension})`, `RegisterTypes(ctx, *pgx.Conn)`, `List(ctx, ns, Filter, Filter)`, `RemoveByFilter(ctx, ns, Filter) (int,error)`, `RemoveGraphBySource(ctx, ns, []string)`, **SecurityFilters applied as `metadata @> $n` (postgres.go:559-561) → reason M1 drops it (M2)**; ingest `MetadataSourceIDKey="source_id"`; generate.Usage{PromptTokens,CompletionTokens,TotalTokens}; llm.Usage{InputTokens,OutputTokens,TotalTokens}; otel `NewTracerProvider(ctx, ExporterConfig)` (exporters.go:60); otelrag `Wrap(sys, Config{TracerProvider})`, `Wrapper.{Ask,Import,Retrieve,Inner}` — **all confirmed present at tag v0.4.0** (M3). rag working tree confirmed == tag v1.11.0; customer-support pins otel v0.4.0. ✓

**Postgres-test gating:** all DB-touching suites use `LLM_AGENT_KB_PG_URL` + `t.Skipf` (mirrors authz `LLM_AGENT_AUTHZ_PG_URL`); the test DB MUST be pgvector-enabled (`CREATE EXTENSION vector` in `ragStore.Migrate`) — called out in Tasks 4/8/11/12. Pure suites (config/ragsvc adapters/ingest parse/retrieval/limits/httpapi-ask) run always. ✓

**RBAC exercised (accurate scope, L1):** M1 exercises BOTH membership levels, but NOT the full §8 org⊕kb merge of *independently-granted* org and kb roles. What is wired and tested:
- **org-level** `org_admin` — written by `POST /api/orgs` (creator); resolved via `orgScopeFromRequest` (`scope_id=""` → matches the `scope_id IS NULL` row); gates `POST/GET /api/orgs/{org}/kbs`. Tested in `orgkb` (`TestCreateOrgGrantsOrgAdminWhoCanCreateKB`) and `httpapi` (`TestOrgEndpointsBootstrapAndRBAC`: 403 for non-member, 200 for org_admin).
- **kb-level** `admin`/`editor`/`viewer` — kb creator gets kb-scope `admin`; resolved via `kbScopeFromRequest` (org_id from `OrgIDForKB`); gates `ask`(viewer+), `documents`(editor+), `GET /api/kb/{id}`(viewer+), `DELETE /api/kb/{id}`(admin). Tested in `httpapi` ask tests (RoleNone→403, RoleViewer→200) and the e2e smoke test.
- The `Merge`-over-multiple-rows path (org row + kb row both present, taking the max) is the same `ResolveRole` call M1 already uses (the org-level org_admin row IS merged in), so it is structurally exercised; what M1 does NOT separately test is granting a user a *lower* kb-scope role than their org-scope role to observe the merge picking the higher — that membership-management surface (`POST /api/kb/{id}/memberships`, §16.2) is out of M1 scope. ✓

**Placeholder scan:** every code step shows actual, compiling code. Task 10's `deleteKBHandler` is now inlined as a real handler (`ing.DeleteAllDocumentsForKB` → `repo.DeleteRow`) — the broken `interface{ Done() <-chan struct{} }` assertion and the prose-only `deleteKBCascade`/`_ = di` guard are gone; the `Ingester` interface is widened in `httpapi.go`. Task 11's smoke test is a complete, runnable flow (login → orgs → kbs → upload → ask → delete) with real request bodies and assertions, no stubs. No TBDs remain in touched sections. ✓

**Boundaries:** `ragsvc` is the sole importer of rag/postgres/otelrag/generate/embed; orgkb/ingest/retrieval/httpapi depend only on `ragsvc.RagPort` (+ pool/authz store). No import cycles. ✓
