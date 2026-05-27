# llm-agent-memory-gateway

HTTP gateway and service-composition module for durable memory.

## Scope

This module owns:

- HTTP endpoints
- auth-derived tenant binding
- runtime configuration
- request validation and error mapping
- composition of SDK abstractions with backend implementations
- process startup

This module depends on:

- `github.com/costa92/llm-agent-memory`
- `github.com/costa92/llm-agent-memory-postgres`

## First-batch endpoints

- `POST /memory/recall/unified`
- `POST /memory/write`
- `PATCH /memory/items/{memory_id}`
- `POST /memory/items/{memory_id}/pin`
- `POST /memory/items/{memory_id}/unpin`
- `POST /memory/items/{memory_id}/disable`
- `POST /memory/items/{memory_id}/enable`
- `DELETE /memory/items/{memory_id}`
- `POST /memory/sessions/{session_id}/close`
- `POST /memory/sessions/{session_id}/heartbeat`
- `GET /metrics`

## Runtime configuration

- `LLM_AGENT_MEMORY_PG_URL` required
- `LLM_AGENT_MEMORY_GATEWAY_ADDR` optional, default `:8080`
- `LLM_AGENT_MEMORY_GATEWAY_READ_ONLY` optional, `true|false`
- `LLM_AGENT_MEMORY_GATEWAY_SESSION_IDLE_TTL` optional, default `30m`
- `LLM_AGENT_MEMORY_GATEWAY_RECALL_MODE` optional, `lexical|hybrid`, default `lexical`
- `LLM_AGENT_MEMORY_GATEWAY_VECTOR_ENABLED` optional, `true|false`, default `false`
- `LLM_AGENT_MEMORY_GATEWAY_VECTOR_TABLE` optional, default `memory_gateway_vectors`
- `LLM_AGENT_MEMORY_GATEWAY_VECTOR_DIMENSION` optional, default `32`
- `LLM_AGENT_MEMORY_GATEWAY_VECTOR_NAMESPACE` optional, default empty
- `LLM_AGENT_MEMORY_GATEWAY_VECTOR_INDEX` optional, `none|ivfflat|hnsw`, default `none`
- `LLM_AGENT_MEMORY_GATEWAY_OUTBOX_POLL_INTERVAL` optional, default `1s`
- `LLM_AGENT_MEMORY_GATEWAY_OUTBOX_BATCH_SIZE` optional, default `100`

## Run

```bash
LLM_AGENT_MEMORY_PG_URL=postgres://... GOWORK=off go run ./cmd/memory-gateway
```

Startup now performs explicit gateway-owned schema migration for:

- `memory_gateway_session`
- `memory_gateway_scope_version`

You can also run the gateway-only migration command explicitly:

```bash
LLM_AGENT_MEMORY_PG_URL=postgres://... GOWORK=off go run ./cmd/memory-gateway-migrate
```

Auth scope for M6 is header-derived:

- `X-Tenant-Id`
- `X-User-Id`
- optional `X-Project-Id`
- optional `X-Session-Id`

Client JSON `scope` is accepted for shape parity, but the gateway always
overrides tenant/user scope with the auth-derived scope.

## Current recall behavior

The first M6 recall path now uses a pluggable hybrid recall seam:

- candidate generation is pluggable behind gateway-owned `RecallCandidateSource`
- truth-source hydration is pluggable behind gateway-owned `RecallRecordHydrator`
- default runtime wiring currently uses a Postgres lexical candidate source
  with simple `ILIKE` matching over content/category/tags
- `hybrid` mode is now wired at startup and includes a null vector candidate source placeholder
  so future vector backends can be attached without changing handler/service boundaries
- gateway now includes concrete runtime wiring for `llm-agent-rag/postgres` as a vector
  candidate source when `LLM_AGENT_MEMORY_GATEWAY_VECTOR_ENABLED=true`
- the current default embedder in that path is `llm-agent-rag/embed.HashEmbedder`
- vector candidates still only act as candidates; final visibility and returned payloads
  continue to be enforced through the gateway truth-source hydrator
- durable memory mutations write backend outbox events first, then a gateway-owned
  outbox relay worker projects those events into the configured vector store
- current outbox projection semantics are:
  - `write` / `patch` / `pin` / `unpin` / `enable` project as vector upsert
  - `disable` / `delete` project as vector remove
- before applying a projection event, the worker now re-reads the durable truth
  source and only applies the event when `event.version == current.version`
- outbox projection now emits structured observations for:
  - `projected`
  - `stale`
  - `failed`
  - `ignored`
- gateway now also exposes a minimal in-process metrics endpoint at `GET /metrics`
  with counters for current recall/outbox activity:
  - `recall_l1_hit_total` for eventual cache hits
  - `recall_l2_hit_total` for bounded cache hits
  - `recall_origin_total`
  - `recall_stale_served_total`
  - `recall_cache_fill_total`
  - `recall_invalidation_total`
  - `outbox_projection_projected_total`
  - `outbox_projection_stale_total`
  - `outbox_projection_failed_total`
  - `outbox_projection_ignored_total`
- request handlers no longer perform synchronous vector projection in the write path
- candidate hits are always re-hydrated from the durable truth source before
  they are returned
- DB-side scope filtering remains enforced from tenant/user/project/session
  inputs even if future vector candidate sources are added
- service-side candidate selection prefers memories that are:
  - pinned
  - `user_saved`
  - shorter / cheaper to fit into prompt budget
- gateway-owned L1 in-memory recall result cache is enabled
- consistency behavior currently is:
  - `eventual`: may reuse cached result and can serve stale cache when `allow_stale_cache=true`
  - `bounded`: only uses fresh L1 cache entries whose cached scope-version snapshot still matches the shared scope version token, and whose cached hit versions still match current truth-source object versions; otherwise it forces truth-source read
  - `strong`: bypasses recall cache and reads through to the truth source
- structured trace stages are emitted from the service layer
- token cost estimate is returned per hit
- concrete vector candidate source wiring and production recall quality are deferred

## Current session close behavior

`POST /memory/sessions/{session_id}/close` now enforces:

- auth-derived scope precedence
- explicit mode validation:
  - `expire_working`
  - `promote_and_expire`
- gateway-owned session lifecycle registry records closed sessions
- default runtime wiring persists session lifecycle state in Postgres
- default runtime wiring also persists shared scope-version tokens in Postgres for bounded-consistency invalidation
- recall against a closed session is rejected with `403`
- structured gateway trace emission

The gateway still does not own working-memory contents. It only owns session
lifecycle state, leaving local working data and cleanup mechanics to the agent
side.

`POST /memory/sessions/{session_id}/heartbeat` now:

- marks or refreshes an active session lifecycle record
- records active liveness separately from close state
- rejects heartbeats for already closed sessions with `403`
- rejects heartbeats for sessions idle past the configured session TTL with `403`
- keeps session lifecycle state gateway-owned without taking ownership of local
  working-memory payloads

Gateway session lifecycle now also enforces idle expiry:

- default idle TTL is `30m`
- idle TTL is refreshed by heartbeat
- recall against an idle-expired session is rejected with `403`

Design references:

- `docs/superpowers/specs/2026-05-26-memory-gateway-module-design.md`
- `docs/memory-gateway-api-contract.zh-CN.md`
