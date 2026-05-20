# Technology Stack

**Analysis Date:** 2026-05-20

## Languages

**Primary:**
- Go 1.26.0 — every subproject declares `go 1.26.0` (`llm-agent/go.mod` line 3, `llm-agent-rag/go.mod` line 3, `llm-agent-otel/go.mod` line 3, `llm-agent-providers/go.mod` line 3, `llm-agent-customer-support/go.mod` line 3).

**Secondary:**
- Bash — workspace bootstrap and per-repo orchestration in `scripts/eco.sh` and `scripts/workspace.sh`.
- YAML — Docker Compose (`llm-agent-customer-support/compose/compose.yaml`, `llm-agent-otel/compose/compose.yaml`), OpenTelemetry Collector config (`llm-agent-customer-support/compose/otel-collector.yaml`), CI workflow (`.github/workflows/umbrella.yml`).
- Dockerfile — `llm-agent-customer-support/compose/Dockerfile`.

## Runtime

**Environment:**
- Go toolchain 1.26.0 (pinned by every module's `go.mod` and by `go.work` line 1).
- Workspace multiplexing via `go.work` listing all five `./llm-agent*` modules (`go.work` lines 3-9).

**Package Manager:**
- `go mod` per subproject. CI runs every subproject in isolation with `GOWORK=off` (`.github/workflows/umbrella.yml` lines 71-103), enforcing the rule that `go.work` is a local-only development affordance.
- Lockfiles (`go.sum`): present for `llm-agent-rag`, `llm-agent-otel`, `llm-agent-providers`, `llm-agent-customer-support`. The core `llm-agent` keeps only the single back-edge entry to `llm-agent-rag` (see Notable Rules below).

## Frameworks

**Core:**
- `github.com/costa92/llm-agent` v0.5.0 / v0.5.1 — stdlib-only agent framework (consumed by every sister repo). Itself only requires `github.com/costa92/llm-agent-rag v1.0.1` for the RAG facade (`llm-agent/go.mod` line 5).
- `github.com/costa92/llm-agent-rag` v1.0.1 — standalone RAG SDK frozen at v1.x public API (`README.md` lines 50-52).

**Testing:**
- Go's built-in `testing` package across every module.
- `github.com/stretchr/testify` v1.11.1 (indirect via providers — `llm-agent-providers/go.mod` line 58).
- `github.com/testcontainers/testcontainers-go/modules/ollama` v0.42.0 — Ollama-backed integration tests (`llm-agent-providers/go.mod` line 10).
- `go.uber.org/goleak` v1.3.0 — goroutine leak detection in providers (`llm-agent-providers/go.mod` line 11).

**Build/Dev:**
- `make` — root Makefile delegates to `scripts/eco.sh` for `bootstrap | workspace | pull | status | build | test | up | down` (`Makefile` lines 7-40).
- `docker compose` — launchable subprojects (`llm-agent-customer-support`, `llm-agent-otel`) via `compose/compose.yaml` (`scripts/eco.sh` lines 88-112).
- Multistage `golang:1.26 → gcr.io/distroless/base-debian12` build for the customer-support service (`llm-agent-customer-support/compose/Dockerfile` lines 1-25).

## Key Dependencies

### `llm-agent` (core)
- `github.com/costa92/llm-agent-rag` v1.0.1 — only non-stdlib direct require (`llm-agent/go.mod` line 5).
- `go.sum` contains exactly two lines, both for the rag back-edge (`llm-agent/go.sum`):
  - `github.com/costa92/llm-agent-rag v1.0.1 h1:...`
  - `github.com/costa92/llm-agent-rag v1.0.1/go.mod h1:...`

### `llm-agent-rag`
- `github.com/costa92/llm-agent` v0.5.0 — adapter back-edge used only under `//go:build llmagent` (`llm-agent-rag/adapter/llmagent/model.go` lines 1-15).
- `github.com/jackc/pgx/v5` v5.9.2 — Postgres driver for the `postgres` subpackage (`llm-agent-rag/go.mod` line 7, `llm-agent-rag/postgres/postgres.go` lines 16-18).
- `github.com/pgvector/pgvector-go` v0.3.0 — pgvector type registration and vector params (`llm-agent-rag/go.mod` line 8, `llm-agent-rag/postgres/postgres.go` lines 19-20).
- Indirects: `github.com/jackc/pgpassfile` v1.0.0, `github.com/jackc/pgservicefile`, `github.com/jackc/puddle/v2` v2.2.2, `github.com/x448/float16` v0.8.4, `golang.org/x/sync` v0.17.0, `golang.org/x/text` v0.29.0 (`llm-agent-rag/go.mod` lines 11-18).

### `llm-agent-otel`
- `github.com/costa92/llm-agent` v0.5.1 — core back-edge for the chat-model decorator (`llm-agent-otel/go.mod` line 5).
- `github.com/costa92/llm-agent-rag` v1.0.1 — RAG-system decorator surface (`llm-agent-otel/go.mod` line 8).
- `go.opentelemetry.io/otel` v1.43.0, `otel/sdk` v1.43.0, `otel/trace` v1.43.0, `otel/metric` v1.43.0, `otel/sdk/metric` v1.43.0 (`llm-agent-otel/go.mod` lines 9-15).
- OTLP exporters: `otlptracegrpc` v1.40.0, `otlptracehttp` v1.43.0 (`llm-agent-otel/go.mod` lines 10-11).
- Indirects of note: `google.golang.org/grpc` v1.80.0, `google.golang.org/protobuf` v1.36.11, `go.opentelemetry.io/proto/otlp` v1.10.0 (`llm-agent-otel/go.mod` lines 31-34).

### `llm-agent-providers`
- `github.com/costa92/llm-agent` v0.5.1 (`llm-agent-providers/go.mod` line 7).
- `github.com/openai/openai-go/v3` v3.35.0 — official OpenAI SDK, also used by the DeepSeek adapter (`llm-agent-providers/go.mod` line 9, `openai/options.go` lines 10-11, `deepseek/options.go` lines 10-11).
- `github.com/anthropics/anthropic-sdk-go` v1.41.0 — official Anthropic SDK, also re-used by the MiniMax adapter (`llm-agent-providers/go.mod` line 6, `anthropic/options.go` lines 9-10, `minimax/options.go` lines 9-10).
- `github.com/ollama/ollama` v0.23.2 — Ollama API client (`llm-agent-providers/go.mod` line 8, `ollama/options.go` line 13).
- `github.com/testcontainers/testcontainers-go/modules/ollama` v0.42.0 — test-time Ollama container (`llm-agent-providers/go.mod` line 10).
- `go.uber.org/goleak` v1.3.0 (`llm-agent-providers/go.mod` line 11).
- Large indirect graph (docker/containerd helpers, OTel v1.41, gopsutil, etc.) at `llm-agent-providers/go.mod` lines 14-77.

### `llm-agent-customer-support`
- `github.com/costa92/llm-agent` v0.5.1, `github.com/costa92/llm-agent-otel` v0.2.1, `github.com/costa92/llm-agent-providers` v0.2.1 (`llm-agent-customer-support/go.mod` lines 6-8).
- `github.com/google/uuid` v1.6.0 (`llm-agent-customer-support/go.mod` line 9).
- `github.com/lib/pq` v1.12.3 — Postgres `database/sql` driver registered for session storage (`llm-agent-customer-support/go.mod` line 10, `internal/sessionstore/sessionstore.go` line 11).
- `modernc.org/sqlite` v1.49.1 — pure-Go SQLite for session storage (`llm-agent-customer-support/go.mod` line 14, `internal/sessionstore/sessionstore.go` line 12).
- `go.opentelemetry.io/otel` v1.43.0, `otel/sdk` v1.43.0, `otel/trace` v1.43.0 (`llm-agent-customer-support/go.mod` lines 11-13).
- Indirects bring in the transitive provider SDKs (anthropic, openai, ollama), pgx via the otel/rag layer, and the rest of the OTel exporter stack (`llm-agent-customer-support/go.mod` lines 17-60).

## Configuration

**Workspace:**
- `go.work` at repo root pins toolchain `1.26.0` and `use`s the five subproject directories (`go.work` lines 1-9).
- `go.work` and `go.work.sum` are `.gitignore`d (`.gitignore` line 8 plus README rule §3); CI runs subprojects with `GOWORK=off` (`.github/workflows/umbrella.yml`).

**Environment:**
- Customer-support service is fully env-driven (`llm-agent-customer-support/internal/config/config.go` lines 48-97) — see INTEGRATIONS.md for the keyset.
- Provider adapters read `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `DEEPSEEK_API_KEY`, `MINIMAX_API_KEY`, `OLLAMA_HOST` from the environment when their `WithAPIKey` / `WithBaseURL` options are unset (`llm-agent-providers/openai/options.go` line 46, `anthropic/options.go` line 46, `deepseek/options.go` line 64, `minimax/options.go` line 64, `ollama/options.go` lines 58-61).

**Build:**
- `go.work` (workspace), each subproject's `go.mod` / `go.sum` (release builds), `Makefile`, `scripts/eco.sh`, `scripts/workspace.sh`.
- Customer-support multistage container builds with `CGO_ENABLED=0 GOOS=linux GOARCH=amd64` and runs on distroless (`llm-agent-customer-support/compose/Dockerfile` lines 15-23).

## Notable Rules (Enforced by Project Contract)

1. **Core `llm-agent` is stdlib-only.** No non-stdlib direct or indirect requires in `llm-agent/go.mod`; the only `go.sum` lines allowed are the v1.0.1 back-edge to `llm-agent-rag` for the RAG facade. Verified by reading `llm-agent/go.mod` (5 lines, single require) and `llm-agent/go.sum` (2 lines, both for `llm-agent-rag`). (`README.md` §Project rules item 1.)
2. **No `replace` directives on tagged-release branches.** Local-dev escape hatch only; INFRA-04 CI gate refuses to tag commits with a `replace` block. The customer-support Dockerfile actively scrubs any local `replace` lines before `go mod download` (`llm-agent-customer-support/compose/Dockerfile` lines 7-9). (`README.md` §Project rules item 2.)
3. **`go.work` is `.gitignore`d in every repo; CI runs `GOWORK=off`** (`.github/workflows/umbrella.yml` lines 73-103; `.gitignore` line 8). (`README.md` §Project rules item 3.)
4. **No K8s / Helm packaging anywhere** — standing non-goal. (`README.md` §Project rules item 4.)
5. **Capabilities are per-`(provider × model)`,** not per-provider — each `provider.New` binds a model at construction and `Info()` reflects that bound model's capabilities. (`README.md` §Project rules item 5; see Keystone K2 in INTEGRATIONS.md.)
6. **OTel attaches as decorator wrappers, never hooks** — `otelmodel.Wrap(inner) llm.ChatModel` (`llm-agent-otel/otelmodel/otelmodel.go` line 20, `README.md` §Project rules item 6).

## Platform Requirements

**Development:**
- Go 1.26 toolchain.
- Docker Engine + `docker compose` for launching `llm-agent-customer-support` and `llm-agent-otel` demo stacks (`scripts/eco.sh` lines 88-112).
- Bash 4+ (`scripts/eco.sh`, `scripts/workspace.sh`).

**Production / Demo Target:**
- Container images: `gcr.io/distroless/base-debian12` for the app, `grafana/otel-lgtm:latest`, `otel/opentelemetry-collector-contrib:0.137.0`, `grafana/grafana:12.2.0`, `ollama/ollama:latest`, `curlimages/curl:8.16.0` (`llm-agent-customer-support/compose/compose.yaml`, `llm-agent-otel/compose/compose.yaml`).
- The compose stack is explicitly demo-only; no TLS, auth, secret management, or multi-tenant isolation is included (`llm-agent-customer-support/README.md` line 5).

## CI Tooling

- Single workflow at `.github/workflows/umbrella.yml` named `umbrella`, triggered on push/PR to `main` and `workflow_dispatch`. Runs on `ubuntu-latest` with a 30-minute timeout.
- Cross-checks out all five sister repos at their default branches (`llm-agent@main`, `llm-agent-rag@master`, `llm-agent-otel@main`, `llm-agent-providers@main`, `llm-agent-customer-support@main`) (`.github/workflows/umbrella.yml` lines 22-55).
- Sets up Go via `actions/setup-go@v5` pinned to `1.26.0` with caching disabled (`.github/workflows/umbrella.yml` lines 57-60).
- Validates umbrella root files exist (`README.md`, `PROJECT.md`, `.planning/PROJECT.md`, `.planning/README.md`, `go.work`) (`.github/workflows/umbrella.yml` lines 62-68).
- For each subproject runs `GOWORK=off go vet ./...`, `GOWORK=off go build ./...`, `GOWORK=off go test ./... -count=1` (`.github/workflows/umbrella.yml` lines 70-103).
- No standalone linter is configured at the umbrella level; per-subproject `go vet` is the only static-analysis gate.

## Docker / Container Surface

- **Single Dockerfile:** `llm-agent-customer-support/compose/Dockerfile` — multistage `golang:1.26` → distroless, builds `./cmd/server`, exposes port 8080.
- **Compose stacks:**
  - `llm-agent-customer-support/compose/compose.yaml` — full demo: `ollama`, `ollama-init` (one-shot model puller for `llama3.1:8b` + `nomic-embed-text`), `otel-lgtm`, `otel-collector` (custom config), `grafana`, `app`. Persistent volumes `ollama-data`, `support-data`. Default port map: app 8080, Grafana 3000, Ollama 11434, OTLP gRPC 4317, OTLP HTTP 4318 (`scripts/eco.sh` lines 90-97).
  - `llm-agent-otel/compose/compose.yaml` — minimal demo: `otel-lgtm` + a `golang:1.26` container running the demo program. Default port map: Grafana 3001, OTLP gRPC 4319, OTLP HTTP 4320 (`scripts/eco.sh` lines 98-104).
- Other subprojects (`llm-agent`, `llm-agent-rag`, `llm-agent-providers`) ship no container artifacts — they are libraries.

## Subproject Tagging Snapshot

Per `README.md` repository roster (lines 30-37):

| Subproject | Current tag | Default branch |
|---|---|---|
| `llm-agent` | v0.5.0 | `main` |
| `llm-agent-rag` | v1.0.0 | `master` |
| `llm-agent-otel` | v0.2.0 | `main` |
| `llm-agent-providers` | v0.2.0 | `main` |
| `llm-agent-customer-support` | v0.2.0 | `main` |

The `go.mod` files reference v0.5.1 / v0.2.1 in places — those are local release-prep state for the in-flight v1.1 ecosystem-alignment milestone (`README.md` lines 124-127).

---

*Stack analysis: 2026-05-20*
