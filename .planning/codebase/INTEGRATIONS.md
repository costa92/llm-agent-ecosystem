# External Integrations

**Analysis Date:** 2026-05-20

## Inter-Module Dependency Graph

Source: `README.md` lines 41-48, cross-checked against each subproject's `go.mod`.

```
llm-agent-customer-support  ──depends on──▶  llm-agent + llm-agent-otel + llm-agent-providers
llm-agent-otel              ──depends on──▶  llm-agent + llm-agent-rag
llm-agent-providers         ──depends on──▶  llm-agent
llm-agent                   ──depends on──▶  llm-agent-rag (RAG facade only)
llm-agent-rag               ──depends on──▶  (stdlib + pgx for postgres subpackage)
```

`llm-agent-rag` is the fixed point of the ecosystem; its v1.x public API is additive-only with breaking changes routed to a `/v2` module path (`README.md` lines 50-52). Concrete back-edges in tagged form:

- `llm-agent/go.mod` line 5: `github.com/costa92/llm-agent-rag v1.0.1`.
- `llm-agent-rag/go.mod` line 6: `github.com/costa92/llm-agent v0.5.0` — but only consumed under the `//go:build llmagent` build tag in `llm-agent-rag/adapter/llmagent/model.go` lines 1-15. The adapter is opt-in and never linked into the stdlib-only core build.
- `llm-agent-otel/go.mod` lines 5-8: `github.com/costa92/llm-agent v0.5.1`, `github.com/costa92/llm-agent-rag v1.0.1`.
- `llm-agent-providers/go.mod` line 7: `github.com/costa92/llm-agent v0.5.1`.
- `llm-agent-customer-support/go.mod` lines 6-8: `github.com/costa92/llm-agent v0.5.1`, `github.com/costa92/llm-agent-otel v0.2.1`, `github.com/costa92/llm-agent-providers v0.2.1`.

## LLM Provider Adapters (`llm-agent-providers/`)

Each adapter implements `llm.ChatModel`, `llm.ToolCaller`, optionally `llm.Embedder` and `llm.StructuredOutputs`. Per the K2 keystone rule, capabilities are reported per-(provider × model): each `New` binds one model at construction (`*/options.go` `WithModel` is required and errors otherwise — e.g. `openai/options.go` lines 42-44).

### OpenAI — `llm-agent-providers/openai/`
- SDK: `github.com/openai/openai-go/v3` v3.35.0 (`go.mod` line 9; imported `openai/options.go` lines 10-11).
- Auth: `WithAPIKey(...)` or `OPENAI_API_KEY` env (`openai/options.go` line 46).
- Endpoint: SDK default; overridable via `WithBaseURL(...)` and `WithOrganization(...)` (`openai/options.go` lines 29, 35, 54-62).
- Capabilities (`openai/options.go` lines 74-85):
  - `Tools: true` for all bound models.
  - `Embeddings: true` only when the bound model is `text-embedding-3-small`, `text-embedding-3-large`, or `text-embedding-ada-002` (`openai/options.go` lines 68-72).
  - `StructuredOutputs: false`, `PromptCaching: false`.
- Models wired in tests / docs: `gpt-4o-mini` (`openai_test.go` line 29, `openai/README.md` line 22); embedding models `text-embedding-3-small` (contract suite at `internal/contract/generate_test.go` line 46).
- README scope: Generate, Stream, native tool calling, embeddings (`openai/README.md`, `providers/README.md` lines 33-39).

### Anthropic — `llm-agent-providers/anthropic/`
- SDK: `github.com/anthropics/anthropic-sdk-go` v1.41.0 (`go.mod` line 6; imported `anthropic/options.go` lines 9-10).
- Auth: `WithAPIKey(...)` or `ANTHROPIC_API_KEY` env (`anthropic/options.go` line 46).
- Endpoint: SDK default; overridable via `WithBaseURL(...)`. `WithBetaHeader(...)` toggles the `anthropic-beta` header (`anthropic/options.go` lines 60-62).
- Capabilities (`anthropic/options.go` lines 70-79):
  - `Tools: true`.
  - `Embeddings: false` — explicit gap; `Info().Capabilities.Embeddings` is always false (`anthropic/README.md` lines 12-16). Callers branch on `llm.ErrCapabilityNotSupported`.
  - `StructuredOutputs: false`, `PromptCaching: false`.
- Anthropic-specific behavior: `Request.SystemPrompt` and `role=system` messages are lifted to top-level `system`; `529 overloaded_error` maps to `*llm.RateLimitError` (`anthropic/README.md` lines 18-22).
- Models wired in tests / docs: `claude-3-5-haiku-20241022` (`anthropic_test.go` line 29, `anthropic/README.md` line 27, contract suite `internal/contract/generate_test.go` line 30).

### Ollama — `llm-agent-providers/ollama/`
- SDK: `github.com/ollama/ollama` v0.23.2 (`go.mod` line 8; imported `ollama/options.go` line 13 as `api "github.com/ollama/ollama/api"`).
- Auth: keyless. Endpoint resolution order: `WithBaseURL(...)` → `WithHost(...)` (alias) → `OLLAMA_HOST` env → `http://localhost:11434` (`ollama/options.go` lines 58-66).
- HTTP transport is wrapped by a `statusCapturingTransport` to surface upstream status codes for error mapping (`ollama/options.go` lines 35-46, 90-93).
- Capabilities (`ollama/options.go` lines 100-110): per-model strategy table.
  - **Tool support** by model family (`ollama/tool_strategy.go` lines 20-48):
    - `llama3.1*` — strategy `python_tag` (native or `<|python_tag|>` parser), `supportsTool: true`.
    - `qwen2.5-coder*` — strategy `qwen_json_or_xml`, `supportsTool: true`.
    - `qwen3-coder*` — strategy `qwen_json_or_xml`, `supportsTool: true`.
    - Everything else: `supportsTool: false`.
  - **Embedding support** by model family (`ollama/embed_strategy.go` lines 10-20):
    - `nomic-embed-text*` → 768-dim embeddings.
    - `all-minilm*` → 384-dim embeddings.
    - Everything else: embeddings unavailable (`unsupportedEmbeddingError` returns `llm.ErrCapabilityNotSupported`).
- Models wired in tests / compose: `llama3.1:8b` (default chat model, `compose/compose.yaml` line 25, customer-support `config/config.go` line 106), `nomic-embed-text` (default embedder, `compose/compose.yaml` line 26, `config/config.go` line 126). Test fixtures additionally exercise `llama2` (`internal/contract/generate_test.go` lines 161, 246).
- Nightly live CI path runs against a real Ollama (`providers/README.md` line 19); `testcontainers-go/modules/ollama` v0.42.0 is the test-time backend (`providers/go.mod` line 10).

### DeepSeek — `llm-agent-providers/deepseek/`
- SDK: re-uses `github.com/openai/openai-go/v3` v3.35.0 (OpenAI-compatible API; `deepseek/options.go` lines 10-11).
- Auth: `WithAPIKey(...)` or `DEEPSEEK_API_KEY` env (`deepseek/options.go` line 64).
- Endpoint: `defaultBaseURL = "https://api.deepseek.com"` (`deepseek/options.go` line 21). `WithRegion(RegionCN | RegionGlobal)` selects a regional preset (currently both regions point at the same default endpoint — `deepseek/options.go` lines 46-53). `WithBaseURL(...)` overrides.
- Capabilities (`deepseek/options.go` lines 92-99):
  - `Tools: true`.
  - `Embeddings: false` (explicit gap — `deepseek/README.md` lines 12-15).
  - `StructuredOutputs: false`, `PromptCaching: false`.
- Models wired in tests / docs: `deepseek-chat` (`deepseek_test.go` line 29, `deepseek/README.md` line 26).

### MiniMax — `llm-agent-providers/minimax/`
- SDK: re-uses `github.com/anthropics/anthropic-sdk-go` v1.41.0 (`minimax/options.go` lines 9-10). The Anthropic SDK is wired against MiniMax's endpoint.
- Auth: `WithAPIKey(...)` or `MINIMAX_API_KEY` env (`minimax/options.go` line 64).
- Endpoint: `defaultBaseURL = "https://api.minimax.chat"` (`minimax/options.go` line 21). `WithRegion(RegionCN | RegionGlobal)` (`minimax/options.go` lines 46-53). `WithBaseURL(...)` overrides.
- Capabilities (`minimax/options.go` lines 92-99):
  - `Tools: true`.
  - `Embeddings: false` (explicit gap — `minimax/README.md` lines 12-15).
  - `StructuredOutputs: false`, `PromptCaching: false`.
- Models wired in tests / docs: `MiniMax-M1` (`minimax_test.go` line 29, `minimax/README.md` line 26).

### Provider × Model Capability Matrix (Keystone K2)

| Provider | Model wired in code | Tools | Embeddings | Structured | Prompt cache |
|---|---|:-:|:-:|:-:|:-:|
| openai | `gpt-4o-mini` | ✓ | — | — | — |
| openai | `text-embedding-3-small` / `-3-large` / `-ada-002` | ✓ | ✓ | — | — |
| anthropic | `claude-3-5-haiku-20241022` | ✓ | — (`ErrNotSupported`) | — | — |
| ollama | `llama3.1:8b` (family `llama3.1`) | ✓ (`python_tag`) | — | — | — |
| ollama | `qwen2.5-coder*` | ✓ (`qwen_json_or_xml`) | — | — | — |
| ollama | `qwen3-coder*` | ✓ (`qwen_json_or_xml`) | — | — | — |
| ollama | `nomic-embed-text` | — | ✓ (768-dim) | — | — |
| ollama | `all-minilm` | — | ✓ (384-dim) | — | — |
| ollama | other (e.g. `llama2`) | — (`ErrNotSupported`) | — | — | — |
| deepseek | `deepseek-chat` | ✓ | — (`ErrNotSupported`) | — | — |
| minimax | `MiniMax-M1` | ✓ | — (`ErrNotSupported`) | — | — |

`StructuredOutputs` and `PromptCaching` are uniformly off across all adapters in v0.2.x (see each `New` constructor).

## OpenTelemetry Surface (`llm-agent-otel/`)

### Wrappers
- `otelmodel.Wrap(inner llm.ChatModel, cfg otelmodel.Config) llm.ChatModel` — capability-preserving decorator (`llm-agent-otel/otelmodel/otelmodel.go` lines 20-44). The wrapper inspects `ToolCaller / Embedder / StructuredOutputs` and returns a composite that re-implements only the interfaces the inner type implements (per Keystone K3).
- `otelagent.Wrap(agent agents.Agent, cfg otelagent.Config) agents.Agent` (`llm-agent-otel/otelagent/otelagent.go`).
- `otelrag.*` — RAG-system decorator built against `llm-agent-rag` types (`llm-agent-otel/otelrag/`).
- `otelmetrics.*` — low-cardinality metric helpers (`llm-agent-otel/otelmetrics/`).
- `otelslog.NewHandler(...)` — `slog.Handler` bridge that pulls span context (`llm-agent-otel/otelslog/otelslog.go`).
- Semantic conventions: centralized `gen_ai.*` constants in `llm-agent-otel/semconv_gen_ai.go` (file at repo root of the otel module).

### OTLP Exporter Wiring
- `otel.ExporterConfig{Protocol, Endpoint, Insecure}` (`exporters.go` lines 16-21).
- Defaults: protocol `http`, endpoint `http://localhost:4318`, insecure (`exporters.go` lines 22-28).
- HTTP exporter: `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp` v1.43.0 (`exporters_http.go` line 8).
- gRPC exporter: `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc` v1.40.0 (`exporters_grpc.go` line 8).
- `NewTracerProvider(ctx, cfg)` returns `*trace.TracerProvider` from `go.opentelemetry.io/otel/sdk/trace` v1.43.0 (`exporters.go` lines 30-36).
- Endpoint normalization strips `http://`/`https://` prefixes and uses `url.Parse` to extract host before passing to the OTLP SDK (`exporters_http.go` lines 23-31, `exporters_grpc.go` lines 23-31).

### OTel SDK Versions
- `go.opentelemetry.io/otel` v1.43.0 (and `sdk`, `trace`, `metric`, `sdk/metric` at v1.43.0).
- `otlptracegrpc` v1.40.0 (pinned older than the rest — see `llm-agent-otel/go.mod` line 10).
- `otlptracehttp` v1.43.0.
- Indirect: `go.opentelemetry.io/auto/sdk` v1.2.1, `go.opentelemetry.io/proto/otlp` v1.10.0.
- The provider repo separately pulls in `go.opentelemetry.io/otel` v1.41.0 as an indirect via `testcontainers-go` (`llm-agent-providers/go.mod` line 70) — kept indirect so the user-facing surface stays consistent at v1.43.0 through `llm-agent-otel`.

## RAG Storage Backends (`llm-agent-rag/`)

### `store` package — backend seam
- `store.Store` interface: `Upsert`, `Search`, `List`, `Get` (`store/store.go` lines 32-40+).
- Capability-opt-in interfaces: `LexicalSearcher`, `CommunityStore`, `GraphStore` — type-asserted by callers (`store/store.go` lines 1-5 docstring).
- Built-in backend: `InMemoryStore` (`store/inmemory.go`, conformance via `store/inmemory_conformance_test.go`).

### `postgres` subpackage — pgvector backend
- File: `llm-agent-rag/postgres/postgres.go`.
- Direct deps (only pulled when this subpackage is imported):
  - `github.com/jackc/pgx/v5` v5.9.2 — pool + driver (`postgres.go` lines 16-18).
  - `github.com/pgvector/pgvector-go` v0.3.0 with `pgvector/pgvector-go/pgx` for type registration (`postgres.go` lines 19-20).
- Config (`postgres.go` lines 26-36):
  - `Table` (default `"chunks"`).
  - `Dimension` (required; mismatched upserts return `store.ErrDimensionMismatch`).
  - `TextSearchConfig` (default `"english"`).
- Implements both `store.Store` and `store.LexicalSearcher` (compile-time assertion at `postgres.go` lines 45-48).
- Additional files: `postgres/graph.go` (graph store), `postgres/community.go` (community store), `postgres/postgres_conformance_test.go`.

### `llmagent` adapter (build-tagged)
- `llm-agent-rag/adapter/llmagent/model.go` — built only under `//go:build llmagent` (line 1).
- Provides `ModelAdapter` (wraps `corellm.ChatModel` into `generate.Model`) and `AsTool` (exposes `rag.System` as an `agents.Tool`). This is how the core framework consumes the RAG SDK while staying stdlib-only by default.

## Customer-Support Service Wiring (`llm-agent-customer-support/`)

### Provider Factories (`internal/providers/providers.go`)
- `NewChatModel(cfg)` switch (`providers.go` lines 13-36):
  - `ProviderOpenAI` → `openaiprovider.New(WithModel, WithAPIKey, WithBaseURL)`.
  - `ProviderAnthropic` → `anthropicprovider.New(WithModel, WithAPIKey, WithBaseURL)`.
  - `ProviderOllama` → `ollamaprovider.New(WithModel, [WithBaseURL])`.
- `NewEmbedder(cfg)` switch (`providers.go` lines 38-57):
  - `ProviderOpenAI` → OpenAI embedding adapter.
  - `ProviderOllama` → Ollama embedding adapter.
  - `ProviderAnthropic` → explicit `unsupported` error (v0.3 gap).
- DeepSeek and MiniMax are wired in the adapter repo but **not currently bound** by the customer-support service factory.

### Default Models (`internal/config/config.go` lines 99-128)
- Provider `openai` → chat default `gpt-4o-mini`, embedder default `text-embedding-3-small`.
- Provider `anthropic` → chat default `claude-3-5-haiku-20241022`, embedder forced to `openai` (Anthropic has no embedding surface).
- Provider `ollama` → chat default `llama3.1:8b`, embedder default `nomic-embed-text`.

### Session Store (`internal/sessionstore/sessionstore.go`)
- Two dialects gated by `SESSION_BACKEND`:
  - `sqlite` (default DSN `file:support_sessions.db?_pragma=journal_mode(WAL)`, opened via `sql.Open("sqlite", dsn)` using `modernc.org/sqlite` blank-imported on line 12).
  - `postgres` (default DSN `postgres://localhost:5432/llm_agent_customer_support?sslmode=disable`, opened via `sql.Open("postgres", dsn)` using `github.com/lib/pq` blank-imported on line 11).
- Schema is bootstrapped by `ensureSchema` on open (`sessionstore.go` lines 72-78).

### OTel Wiring (`internal/app/app.go` lines 16-21)
- The service consumes `otelroot.NewTracerProvider`, `otelagent.Wrap`, and `otelmodel.Wrap` from the otel module.
- Exporter endpoint is env-driven (`OTEL_EXPORTER_OTLP_*`) via `config.Config` fields `OTLPProtocol`, `OTLPEndpoint`, `OTLPInsecure` (`config.go` lines 37-39, 59-61).

### Required Environment Variables (Customer-Support)
Read by `LoadFromLookup` in `internal/config/config.go` lines 52-96:

- HTTP / runtime: `HTTP_ADDR` (default `:8080`), `SHUTDOWN_TIMEOUT` (default 5s), `SYSTEM_PROMPT`.
- LLM selection: `LLM_PROVIDER` (`openai` | `anthropic` | `ollama`; default `ollama`), `LLM_MODEL`, `EMBEDDING_PROVIDER`, `EMBEDDING_MODEL`.
- Provider creds: `OPENAI_API_KEY`, `OPENAI_BASE_URL`, `ANTHROPIC_API_KEY`, `ANTHROPIC_BASE_URL`, `OLLAMA_HOST`.
- Session store: `SESSION_BACKEND` (`sqlite` | `postgres`; default `sqlite`), `SESSION_DSN`.
- Guardrails: `MAX_TOKENS_PER_REQUEST` (1024), `MAX_TOOL_CALLS_PER_AGENT_LOOP` (4), `MAX_REQUESTS_PER_IP_PER_MINUTE` (60), `RETRY_MAX_ATTEMPTS` (2), `DAILY_TOKEN_BUDGET` (100000), `DISABLE_LLM` (Keystone K7 kill switch).
- OTel: `OTEL_SERVICE_NAME` (default `llm-agent-customer-support`), `OTEL_EXPORTER_OTLP_PROTOCOL` (`http` | `grpc`; default `http`), `OTEL_EXPORTER_OTLP_ENDPOINT` (default `http://localhost:4318`), `OTEL_EXPORTER_OTLP_INSECURE` (default true), `OTEL_SEMCONV_STABILITY_OPT_IN` (set to `gen_ai_latest_experimental` in compose).

## Docker Compose Demo Stacks

### Customer-Support (`llm-agent-customer-support/compose/compose.yaml`)
Services:
- `ollama` — `ollama/ollama:latest`; port `${CS_OLLAMA_PORT:-11434}:11434`; persistent volume `ollama-data`; healthcheck `ollama list` every 10s.
- `ollama-init` — `curlimages/curl:8.16.0` one-shot; pulls `llama3.1:8b` and `nomic-embed-text` via `/api/pull` after `ollama` is healthy (`compose.yaml` lines 14-27).
- `otel-lgtm` — `grafana/otel-lgtm:latest` (single-container Loki/Tempo/Mimir + Grafana); OTLP gRPC `4317`, HTTP `4318`.
- `otel-collector` — `otel/opentelemetry-collector-contrib:0.137.0`; custom config in `compose/otel-collector.yaml`; exposes Prometheus exporter on `8889`.
- `grafana` — `grafana/grafana:12.2.0`; anonymous Admin login; provisioning from `compose/grafana/` and `dashboards/`; port `${CS_GRAFANA_PORT:-3000}:3000`.
- `app` — built from `compose/Dockerfile`; environment wires LLM provider/embedder to Ollama, session store to sqlite (`/data/support_sessions.db`), OTLP endpoint to `http://otel-collector:4318`; port `${CS_APP_PORT:-8080}:8080`; persistent volume `support-data` mounted at `/data`.

OTel Collector pipeline (`compose/otel-collector.yaml`):
- Receivers: OTLP gRPC `0.0.0.0:4317` + HTTP `0.0.0.0:4318`.
- Processors: `batch` and `tail_sampling` (30s decision wait, error/high-latency/probabilistic policies — `otel-collector.yaml` lines 12-29).
- Connectors: `spanmetrics` with explicit-bucket histogram and dimensions `agent.name`, `agent.step.kind`, `tool.name`, `gen_ai.system`, `gen_ai.request.model`.
- Exporters: `otlphttp/lgtm` → `http://otel-lgtm:4318` (insecure); `prometheus` → `0.0.0.0:8889`.
- Pipelines: `traces` (otlp → tail_sampling → batch → otlphttp/lgtm + spanmetrics); `metrics` (spanmetrics → prometheus).

### OTel Demo (`llm-agent-otel/compose/compose.yaml`)
- `otel-lgtm` — `grafana/otel-lgtm:latest`; port maps Grafana `${OTEL_DEMO_GRAFANA_PORT:-3000}:3000`, OTLP gRPC `${OTEL_DEMO_OTLP_GRPC_PORT:-4317}:4317`, OTLP HTTP `${OTEL_DEMO_OTLP_HTTP_PORT:-4318}:4318`.
- `demo` — `golang:1.26` runtime mounting the repo and executing `cd compose/demo && go run .`; env points OTLP at `http://otel-lgtm:4318`.

The umbrella script remaps ports to `3001 / 4319 / 4320` when launching this stack alongside the customer-support stack (`scripts/eco.sh` lines 98-104).

## HTTP / gRPC Clients in the Ecosystem

- HTTP (`net/http`): every provider adapter uses `net/http` either directly (Ollama, via the Ollama SDK's `api.NewClient(u, httpClient)`) or through the upstream SDK's `WithHTTPClient` option (OpenAI, Anthropic, DeepSeek, MiniMax). Custom `*http.Client` plumbing is exposed via `WithHTTPClient` and `WithTimeout` options in every adapter.
- gRPC (`google.golang.org/grpc` v1.80.0): pulled in only by `llm-agent-otel` (via `otlptracegrpc`) and transitively by `llm-agent-customer-support`. No first-party gRPC servers; only OTLP gRPC client traffic to the collector.

## Database Drivers

- `github.com/jackc/pgx/v5` v5.9.2 — used **only** by `llm-agent-rag/postgres/` (`llm-agent-rag/go.mod` line 7, `postgres/postgres.go` lines 16-18). Customers of the RAG SDK own the `*pgxpool.Pool` lifecycle and register pgvector types via `RegisterTypes` in `AfterConnect` (`postgres/postgres.go` lines 1-7 docstring).
- `github.com/lib/pq` v1.12.3 — `database/sql` Postgres driver used by `llm-agent-customer-support/internal/sessionstore/` for session storage (`go.mod` line 10, `sessionstore.go` line 11).
- `modernc.org/sqlite` v1.49.1 — pure-Go SQLite driver used by `llm-agent-customer-support/internal/sessionstore/` (`go.mod` line 14, `sessionstore.go` line 12).
- `github.com/pgvector/pgvector-go` v0.3.0 — pgvector type bindings (`llm-agent-rag/go.mod` line 8).

## Webhooks & Callbacks

- **Incoming:** none. The customer-support service exposes `/chat` HTTP endpoints (`internal/httpapi/`) but no webhook listeners; the umbrella ships no webhook surface.
- **Outgoing:** none. Every external call is a synchronous LLM/embedding API request or an OTLP push to the collector.
- The `github.com/standard-webhooks/standard-webhooks/libraries` package appearing in indirects (`llm-agent-providers/go.mod` line 57) is a transitive dep of the OpenAI SDK, not a first-party integration.

## Secrets / Config Surface

- No secrets are committed. Every adapter falls back to environment variables (`OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `DEEPSEEK_API_KEY`, `MINIMAX_API_KEY`, `OLLAMA_HOST`) when their `WithAPIKey` / `WithBaseURL` options are unset.
- The customer-support compose stack runs Ollama locally (keyless) and never sets API keys for cloud providers — it is a closed-loop local demo.
- Repo-level `.gitignore` excludes `/llm-agent*/` subprojects (they live in their own repos and are bootstrapped on demand) and `/go.work.sum`.

---

*Integration audit: 2026-05-20*
