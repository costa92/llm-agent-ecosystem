# Ecosystem Architecture Overview

> Document version: 2026-05-28
> Code snapshot: 2026-05-28
> Reading goal: build a correct mental model of `llm-agent-ecosystem` in
> about 30 minutes, then continue down the main source-code paths.

---

## 1. Project Positioning

`llm-agent-ecosystem` is not a single application. It is a **multi-repo
umbrella workspace**. The root repo itself is responsible only for:

- workspace organization and cross-repo scripts
- ecosystem-wide dependency direction and rules
- documentation navigation and planning entrypoints

The real functionality is implemented across 9 subprojects:

```text
llm-agent-ecosystem/
├── llm-agent                     # core agent framework
├── llm-agent-rag                 # standalone RAG SDK
├── llm-agent-providers           # model-provider adapter layer
├── llm-agent-otel                # OTel wrappers
├── llm-agent-customer-support    # reference customer-support service
├── llm-agent-flow                # serializable DAG/Flow runtime
├── llm-agent-memory              # memory SDK extension layer
├── llm-agent-memory-postgres     # durable memory Postgres backend
└── llm-agent-memory-gateway      # durable memory HTTP gateway
```

Useful root entrypoints:

- [`../README.md`](../README.md)
- [`../Makefile`](../Makefile)
- [`../scripts/eco.sh`](../scripts/eco.sh)
- [`../scripts/workspace.sh`](../scripts/workspace.sh)

---

## 2. Ecosystem Layers

The ecosystem is easiest to understand as four layers:

```text
Coordination layer
  llm-agent-ecosystem(root)

Core capability layer
  llm-agent
  llm-agent-rag
  llm-agent-memory

Infrastructure layer
  llm-agent-providers
  llm-agent-otel
  llm-agent-flow
  llm-agent-memory-postgres
  llm-agent-memory-gateway

Application layer
  llm-agent-customer-support
```

### 2.1 Dependency Direction

The implemented dependency direction can be summarized as:

```text
llm-agent-customer-support
  -> llm-agent
  -> llm-agent-providers
  -> llm-agent-otel
  -> llm-agent-flow
  -> llm-agent-rag

llm-agent-otel
  -> llm-agent
  -> llm-agent-rag
  -> llm-agent-flow

llm-agent-providers
  -> llm-agent

llm-agent-flow
  -> llm-agent

llm-agent-memory-postgres
  -> llm-agent-memory

llm-agent-memory-gateway
  -> llm-agent-memory
  -> llm-agent-memory-postgres
  -> llm-agent-rag
```

### 2.2 Core Design Principles

- `llm-agent` stays stdlib-only and does not pull third-party deps.
- Model integration uses "smallest possible interface + optional capability"
  seams.
- OTel is attached through decorators rather than hooks into the core.
- `llm-agent-rag` is a fixed point and compatibility anchor for downstreams.
- Durable memory uses a truth-source + outbox + relay + projection model.
- `flow` and `StateGraph` have distinct roles: the former is a persisted DAG
  runtime, the latter is an in-process typed state machine.

---

## 3. What Each Subproject Implements

### 3.1 `llm-agent`

The core Go agent framework. It defines:

- the `Agent` abstraction
- five agent paradigms
- tool system and tool execution
- `llm.ChatModel` and related capability interfaces
- `StateGraph`, pipeline, fan-out/fan-in, and other control-flow primitives
- cross-cutting governance such as budget and policy

Key files:

- [`../llm-agent/doc.go`](../llm-agent/doc.go)
- [`../llm-agent/llm/chatmodel.go`](../llm-agent/llm/chatmodel.go)
- [`../llm-agent/agent_chatmodel.go`](../llm-agent/agent_chatmodel.go)
- [`../llm-agent/orchestrate/graph.go`](../llm-agent/orchestrate/graph.go)

### 3.2 `llm-agent-rag`

The standalone RAG SDK. It defines:

- document ingest
- embedding seam
- store seam
- retrieval strategies
- `rag.System`
- GraphRAG / AskGlobal / AskDrift
- diagnostics, traces, and evaluation

Key files:

- [`../llm-agent-rag/rag/system.go`](../llm-agent-rag/rag/system.go)
- [`../llm-agent-rag/rag/ask.go`](../llm-agent-rag/rag/ask.go)
- [`../llm-agent-rag/retrieve/retrieve.go`](../llm-agent-rag/retrieve/retrieve.go)
- [`../llm-agent-rag/store/store.go`](../llm-agent-rag/store/store.go)

### 3.3 `llm-agent-providers`

The model-provider adapter layer. It connects concrete vendor APIs to the
`llm-agent/llm` abstractions.

Current provider coverage:

- OpenAI
- Anthropic
- Ollama
- DeepSeek
- MiniMax

Key files:

- [`../llm-agent-providers/openai/openai.go`](../llm-agent-providers/openai/openai.go)
- [`../llm-agent-providers/anthropic/anthropic.go`](../llm-agent-providers/anthropic/anthropic.go)
- [`../llm-agent-providers/ollama/ollama.go`](../llm-agent-providers/ollama/ollama.go)
- [`../llm-agent-providers/internal/contract/contract.go`](../llm-agent-providers/internal/contract/contract.go)

### 3.4 `llm-agent-otel`

The OpenTelemetry wrapper layer. It injects tracing, metrics, and log
bridging into:

- `llm.ChatModel`
- `agents.Agent`
- `rag.System`
- `flow.Runner`

Key files:

- [`../llm-agent-otel/otelmodel/otelmodel.go`](../llm-agent-otel/otelmodel/otelmodel.go)
- [`../llm-agent-otel/otelagent/otelagent.go`](../llm-agent-otel/otelagent/otelagent.go)
- [`../llm-agent-otel/otelrag/otelrag.go`](../llm-agent-otel/otelrag/otelrag.go)
- [`../llm-agent-otel/otelflow/otelflow.go`](../llm-agent-otel/otelflow/otelflow.go)

### 3.5 `llm-agent-customer-support`

The reference application. It composes agents, providers, knowledge,
sessions, guardrails, limits, and OTel into a runnable support service.
The repo also contains a `flowrunner` bridge to `llm-agent-flow`, but its
public HTTP surface does not currently expose `/flow/run`-style flow
endpoints.

External surface:

- `POST /chat`
- `POST /chat/stream`
- `GET /healthz`
- `GET /readyz`

Key files:

- [`../llm-agent-customer-support/internal/app/app.go`](../llm-agent-customer-support/internal/app/app.go)
- [`../llm-agent-customer-support/internal/supportflow/supportflow.go`](../llm-agent-customer-support/internal/supportflow/supportflow.go)
- [`../llm-agent-customer-support/internal/httpapi/httpapi.go`](../llm-agent-customer-support/internal/httpapi/httpapi.go)

### 3.6 `llm-agent-flow`

The serializable DAG/Flow runtime. It provides:

- JSON Flow IR
- DAG validate / compile / run
- topological-layer parallel execution
- event streams
- `flowd` as a persisted HTTP runtime

Key files:

- [`../llm-agent-flow/flow/ir.go`](../llm-agent-flow/flow/ir.go)
- [`../llm-agent-flow/flow/engine.go`](../llm-agent-flow/flow/engine.go)
- [`../llm-agent-flow/cmd/flowd/server/server.go`](../llm-agent-flow/cmd/flowd/server/server.go)

### 3.7 `llm-agent-memory`

The memory SDK extension layer. It provides:

- capability-interface-typed manager
- unified search
- recall engine
- consolidator
- scoped lifecycle
- durable abstractions

Key files:

- [`../llm-agent-memory/memory/manager.go`](../llm-agent-memory/memory/manager.go)
- [`../llm-agent-memory/memory/recall_engine.go`](../llm-agent-memory/memory/recall_engine.go)
- [`../llm-agent-memory/memory/consolidator.go`](../llm-agent-memory/memory/consolidator.go)

### 3.8 `llm-agent-memory-postgres`

The durable memory Postgres backend. It provides:

- schema / migration
- `memory_record`
- `memory_event`
- `outbox_event`
- idempotency
- leased relay delivery

Key files:

- [`../llm-agent-memory-postgres/postgres/store.go`](../llm-agent-memory-postgres/postgres/store.go)
- [`../llm-agent-memory-postgres/postgres/relay.go`](../llm-agent-memory-postgres/postgres/relay.go)
- [`../llm-agent-memory-postgres/postgres/schema.go`](../llm-agent-memory-postgres/postgres/schema.go)

### 3.9 `llm-agent-memory-gateway`

The governance-oriented durable memory HTTP gateway. It provides:

- authoritative scope
- recall cache
- consistency semantics
- session lifecycle
- trace + metrics
- outbox-to-vector projection

Key files:

- [`../llm-agent-memory-gateway/internal/service/service.go`](../llm-agent-memory-gateway/internal/service/service.go)
- [`../llm-agent-memory-gateway/internal/service/hybrid_recaller.go`](../llm-agent-memory-gateway/internal/service/hybrid_recaller.go)
- [`../llm-agent-memory-gateway/internal/service/recall_cache.go`](../llm-agent-memory-gateway/internal/service/recall_cache.go)
- [`../llm-agent-memory-gateway/internal/transport/router.go`](../llm-agent-memory-gateway/internal/transport/router.go)

---

## 4. A 30-Minute Reading Path

### Step 1: understand the boundaries

Read:

- [`../README.md`](../README.md)

Goal:

- understand that this is not a single-repo product
- understand the responsibilities of the 9 subprojects

### Step 2: understand the core abstractions

Read:

- [`../llm-agent/doc.go`](../llm-agent/doc.go)
- [`../llm-agent/llm/chatmodel.go`](../llm-agent/llm/chatmodel.go)
- [`../llm-agent/agent_chatmodel.go`](../llm-agent/agent_chatmodel.go)
- [`../llm-agent/orchestrate/graph.go`](../llm-agent/orchestrate/graph.go)

Goal:

- understand `Agent`, `ChatModel`, and `StateGraph`
- understand how budget and other cross-cutting controls are attached

### Step 3: understand the RAG platform

Read:

- [`../llm-agent-rag/rag/system.go`](../llm-agent-rag/rag/system.go)
- [`../llm-agent-rag/rag/ask.go`](../llm-agent-rag/rag/ask.go)
- [`../llm-agent-rag/retrieve/retrieve.go`](../llm-agent-rag/retrieve/retrieve.go)
- [`../llm-agent-rag/store/store.go`](../llm-agent-rag/store/store.go)

Goal:

- understand the three answer paths behind `rag.System`
- understand the pluggable retrieval/store design

### Step 4: read the two main runtimes

Read:

- [`../llm-agent-customer-support/internal/app/app.go`](../llm-agent-customer-support/internal/app/app.go)
- [`../llm-agent-memory-gateway/internal/service/service.go`](../llm-agent-memory-gateway/internal/service/service.go)

Goal:

- understand how an application-style service is assembled
- understand how a governance-style service handles state and consistency

### Step 5: read the workflow runtime

Read:

- [`../llm-agent-flow/flow/ir.go`](../llm-agent-flow/flow/ir.go)
- [`../llm-agent-flow/flow/engine.go`](../llm-agent-flow/flow/engine.go)
- [`../llm-agent-flow/cmd/flowd/server/server.go`](../llm-agent-flow/cmd/flowd/server/server.go)

Goal:

- understand the difference between `flow` and `StateGraph`
- understand why `flowd` exists as a separate runtime service

---

## 5. `customer-support` Call Chain

### 5.1 Composition Root

The composition center is:

- [`../llm-agent-customer-support/internal/app/app.go`](../llm-agent-customer-support/internal/app/app.go)

Assembly order:

```text
config
  -> providers.NewChatModel
  -> providers.NewEmbedder
  -> sessionstore.Open(SQLite/Postgres)
  -> otel.NewTracerProvider
  -> otelmodel.Wrap(model)
  -> knowledgebase.New(embedder)
  -> supportflow.New(model, knowledge, sessions, guardrails)
  -> limits.New(...)
  -> guard.WrapAgent(agent)
  -> otelagent.Wrap(agent)
  -> httpapi.NewMux(...)
```

### 5.2 HTTP Layer

Entrypoint:

- [`../llm-agent-customer-support/internal/httpapi/httpapi.go`](../llm-agent-customer-support/internal/httpapi/httpapi.go)

`POST /chat`:

```text
decodeChatRequest
  -> withRequestSession
  -> preflightGuard
  -> agent.Run
  -> writeJSON(ChatResponse)
```

`POST /chat/stream`:

```text
decodeChatRequest
  -> withRequestSession
  -> preflightGuard
  -> agent.RunStream
  -> SSE step events
  -> SSE done/error
```

### 5.3 supportflow State Graph

Core file:

- [`../llm-agent-customer-support/internal/supportflow/supportflow.go`](../llm-agent-customer-support/internal/supportflow/supportflow.go)

State graph:

```text
load-session
  -> classify
     -> handover-human
     -> request-more-info
     -> self-service
```

Node responsibilities:

- `load-session`
  - read history from `sessionstore`
  - merge history with the current question
- `classify`
  - extract order ID
  - send `chargeback/fraud` to human handoff
  - request more info if no order ID is present
- `request-more-info`
  - return a direct "please provide your order ID" response
- `handover-human`
  - return a direct human-escalation response
- `self-service`
  - call the tool-enabled agent for refund-policy handling

### 5.4 toolAgent

Core file:

- [`../llm-agent-customer-support/internal/supportflow/toolagent.go`](../llm-agent-customer-support/internal/supportflow/toolagent.go)

Execution path:

```text
model.WithTools(registry.AsLLMTools())
  -> Generate(prompt)
  -> if no tool calls:
       return resp.Text
  -> else:
       AsyncRunner.Execute(tasks)
       build action/observation steps
       final answer = concatenated tool outputs
```

There is currently no second LLM round that summarizes tool outputs, so this
is a deliberately thin function-calling loop.

### 5.5 knowledgebase / limits / guardrails

Relevant files:

- [`../llm-agent-customer-support/internal/knowledgebase/knowledgebase.go`](../llm-agent-customer-support/internal/knowledgebase/knowledgebase.go)
- [`../llm-agent-customer-support/internal/limits/limits.go`](../llm-agent-customer-support/internal/limits/limits.go)
- [`../llm-agent-customer-support/internal/guardrails/guardrails.go`](../llm-agent-customer-support/internal/guardrails/guardrails.go)

Notes:

- `knowledgebase` builds a tiny `rag.System + InMemoryStore` and seeds it at
  startup with minimal refund-policy data.
- `limits` performs request-time checks before the run and tool-loop /
  cumulative-token checks after the run.
- `guardrails` does a lightweight prompt-injection scan and prefixes the
  system prompt with "retrieved content is untrusted" guidance.

---

## 6. `memory-gateway` Call Chain

### 6.1 Write Path

Entrypoint:

- [`../llm-agent-memory-gateway/internal/service/service.go`](../llm-agent-memory-gateway/internal/service/service.go)

Common write flow:

```text
transport/router
  -> readAuthoritativeScope(headers)
  -> service.Validate
  -> merge auth scope + body scope
  -> backend.WriteRecord / Patch / Pin / Disable / Delete
  -> invalidateScopeState(scope)
  -> return memory_id/version/status
```

`invalidateScopeState(scope)` is responsible for:

- recall-cache invalidation
- scope-version bump

### 6.2 Durable Truth Source

Underlying file:

- [`../llm-agent-memory-postgres/postgres/store.go`](../llm-agent-memory-postgres/postgres/store.go)

One `WriteRecord` transaction writes:

```text
memory_record
memory_event
outbox_event(status=pending)
idempotency snapshot
```

That makes durable memory much more than CRUD. It is current state,
event flow, outbound projection, and idempotency in one backend.

### 6.3 Recall Path

Main `RecallUnified` flow:

```text
validate query/top_k
  -> merge scope
  -> sessionRegistry.Get + validate session state
  -> scopeVersionStore.CurrentScopeVersion
  -> try recallCache.Lookup
  -> if miss:
       recaller.Recall(scope, query, topK)
       token budget filter
       emit trace + metrics
       cache fill
       return hits
```

### 6.4 Consistency Semantics

Cache file:

- [`../llm-agent-memory-gateway/internal/service/recall_cache.go`](../llm-agent-memory-gateway/internal/service/recall_cache.go)

Scope-version file:

- [`../llm-agent-memory-gateway/internal/service/scope_version_store.go`](../llm-agent-memory-gateway/internal/service/scope_version_store.go)

Three consistency modes:

- `strong`
  - bypass cache completely
- `eventual`
  - 30-second TTL
  - may return stale data when `allow_stale_cache=true`
- `bounded`
  - 5-second TTL
  - cached scope version must still match current
  - cached record versions must still match the truth source

### 6.5 Session Lifecycle

File:

- [`../llm-agent-memory-gateway/internal/service/session_registry.go`](../llm-agent-memory-gateway/internal/service/session_registry.go)

The gateway tracks:

- `active`
- `closed`
- `LastHeartbeatAt`
- `ClosedAt`

Recall requests are gated on:

- whether the session is already closed
- whether the session is idle-expired
- whether heartbeats are still allowed

### 6.6 Hybrid Recall

File:

- [`../llm-agent-memory-gateway/internal/service/hybrid_recaller.go`](../llm-agent-memory-gateway/internal/service/hybrid_recaller.go)

Flow:

```text
RecallCandidateSource[]
  -> candidate memory ids + scores
  -> merge by best score
  -> rank topK
  -> hydrator.HydrateRecords from truth source
  -> return records
```

Vector and lexical sources are candidate generators only. Final returned
records always come from truth-source hydration.

### 6.7 Relay and Vector Projection

Gateway-side publisher:

- [`../llm-agent-memory-gateway/internal/service/outbox_vector_publisher.go`](../llm-agent-memory-gateway/internal/service/outbox_vector_publisher.go)
- [`../llm-agent-memory-gateway/internal/service/vector_projector.go`](../llm-agent-memory-gateway/internal/service/vector_projector.go)

Backend relay:

- [`../llm-agent-memory-postgres/postgres/relay.go`](../llm-agent-memory-postgres/postgres/relay.go)

Pipeline:

```text
outbox_event(status=pending)
  -> Relay.ClaimBatch (lease + attempt_count++)
  -> OutboxVectorPublisher.Publish(msg)
  -> reread current record from truth source
  -> version match ? ProjectUpsert/ProjectRemove : stale skip
  -> Ack(sent | pending | failed)
```

This is a standard leased, at-least-once outbox projection pipeline.

---

## 7. Summary

This project is not one AI application. It is a complete Go-based LLM-agent
stack consisting of:

- an agent framework
- a RAG platform
- provider adapters
- an observability layer
- a workflow runtime
- a durable memory subsystem
- a reference application

If compressed into one sentence:

> `llm-agent` defines the abstractions, `llm-agent-rag` defines the
> knowledge-and-answering platform, `flow` defines the persisted workflow
> runtime, `memory-*` defines the durable-memory subsystem, and
> `customer-support` assembles those capabilities into a runnable service.

---

## Further Reading

- [`./architecture-and-sequence-diagrams.zh-CN.md`](./architecture-and-sequence-diagrams.zh-CN.md)
- [`./source-design-umbrella-root.zh-CN.md`](./source-design-umbrella-root.zh-CN.md)
- [`./source-design-llm-agent.zh-CN.md`](./source-design-llm-agent.zh-CN.md)
- [`./source-design-llm-agent-rag.zh-CN.md`](./source-design-llm-agent-rag.zh-CN.md)
- [`./source-design-llm-agent-flow.zh-CN.md`](./source-design-llm-agent-flow.zh-CN.md)
- [`./source-design-llm-agent-customer-support.zh-CN.md`](./source-design-llm-agent-customer-support.zh-CN.md)
- [`./multi-service-memory-architecture.zh-CN.md`](./multi-service-memory-architecture.zh-CN.md)
