# Current Project Analysis

## Overview

This repository is an umbrella workspace for the `llm-agent` ecosystem.
The root project is not a product runtime and does not own shared business
logic. Its role is to provide:

- a single local workspace for all ecosystem repositories
- shared conventions and dependency direction
- ecosystem-level planning and release coordination

The actual product and library functionality lives in five independent
subprojects:

- `llm-agent`
- `llm-agent-rag`
- `llm-agent-providers`
- `llm-agent-otel`
- `llm-agent-customer-support`

## Ecosystem Structure

### Root repository

Observed root responsibilities:

- top-level navigation and summary in `README.md`
- umbrella scope and boundaries in `PROJECT.md`
- ecosystem planning in `.planning/`
- workspace/bootstrap helpers in `Makefile`, `scripts/eco.sh`,
  `scripts/workspace.sh`

The root is intentionally thin. There is no app entrypoint, no shared API
server, and no central package that all code is implemented inside.

### Dependency direction

The implemented dependency direction is:

- `llm-agent-customer-support` depends on `llm-agent`,
  `llm-agent-providers`, and `llm-agent-otel`
- `llm-agent-otel` depends on `llm-agent` and `llm-agent-rag`
- `llm-agent-providers` depends on `llm-agent`
- `llm-agent` depends on `llm-agent-rag` only for the RAG facade
- `llm-agent-rag` is the lowest-level retrieval package in the stack

This creates a layered architecture:

1. foundation: `llm-agent-rag`
2. core agent abstraction: `llm-agent`
3. integration layers: `llm-agent-providers`, `llm-agent-otel`
4. reference application: `llm-agent-customer-support`

## Subproject Analysis

## `llm-agent`

### Role

`llm-agent` is the core Go agent framework. It provides the abstractions and
runtime patterns used by the rest of the ecosystem.

### Public capability surface

Based on `README.md`, `doc.go`, and package layout, the current framework
implements:

- five agent paradigms:
  - `SimpleAgent`
  - `ReActAgent`
  - `ReflectionAgent`
  - `PlanAndSolveAgent`
  - `FunctionCallAgent`
- tool abstraction and registry
- sequential and async tool execution
- message/model abstraction in `llm/`
- memory primitives
- context engineering helpers
- multi-agent orchestration
- lightweight communication protocols
- evaluation and benchmark helpers
- examples for runnable usage patterns

### Directory responsibilities

- `llm/`: model contracts, capabilities, request/response types, streaming,
  provider/model metadata, scripted test model
- `builtin/`: built-in tools such as calculator, search, note, terminal
- `memory/`: working, episodic, and semantic memory plus memory tools
- `context/`: context building, selection, compression, token-aware packing
- `comm/`: envelope and transports for HTTP, stdio, and in-memory usage
- `comm/mcp`, `comm/a2a`, `comm/anp`: experimental protocol-oriented
  communication layers
- `orchestrate/`: pipeline, fan-out/fan-in, round-robin, roleplay,
  state-graph orchestration
- `rl/`: evaluation and trainer-proxy abstractions for agentic RL workflows
- `bench/`: benchmark harnesses such as BFCL, GAIA, judge, and win-rate
- `budget/`: runtime budget and usage constraint helpers
- `examples/`: runnable example programs covering the main framework patterns

### Architectural reading

This package is not just an agent runner. It is a general-purpose agent
construction kit with four main concerns:

- model abstraction
- tool execution
- orchestration
- evaluation

That means the project is aimed more at framework consumers than at one
specific application.

### Testing and maturity signals

Strong signals:

- broad package-level tests across nearly every directory
- example tests for common usage patterns
- API snapshot and migration docs
- explicit rules around compatibility and stdlib-only constraints

Current maturity assessment:

- core framework surface is substantial and well-tested
- still evolving as a framework line rather than a frozen stable platform
- suitable for internal use, demos, and downstream composition

## `llm-agent-rag`

### Role

`llm-agent-rag` is the standalone retrieval-augmented generation SDK. It is
positioned as an independent lower-level library rather than a helper package
inside `llm-agent`.

### Public capability surface

From `README.md` and directory structure, it currently implements:

- document import from abstract sources
- deterministic splitting for documents and markdown
- embedding seam with default hash embedder
- vector-store seam with in-memory implementation
- PostgreSQL + pgvector backend
- retrieval and hybrid retrieval
- reranking
- token-budget-aware context packing
- prompt templating
- answer generation through caller-supplied model interfaces
- graph and route-aware retrieval features
- retrieval evaluation framework
- optional adapter back into `llm-agent`

### Directory responsibilities

- `ingest/`: documents, sources, import flow, splitters
- `embed/`: embedding contracts and default hash embedder
- `store/`: vector store contracts and in-memory implementation
- `store/storetest/`: backend conformance contract
- `postgres/`: PostgreSQL/pgvector storage backend
- `retrieve/`: ranked retrieval, multihop, graph-oriented retrieval
- `rerank/`: heuristic and HTTP-backed rerank behavior
- `pack/`: token-budget-aware context packing
- `prompt/`: prompt templates and prompt interfaces
- `generate/`: generation model seam
- `rag/`: orchestration layer exposing import, retrieve, and ask workflows
- `graph/`: graph extraction, pathing, community detection, summarization
- `tree/`: structured document tree primitives
- `eval/`: retrieval quality and grounding-related evaluation helpers
- `guard/`: injection/redaction utilities around retrieved content
- `obs/`: observation hooks
- `feedback/`: feedback capture structures
- `adapter/llmagent/`: optional integration layer for the core agent repo

### Architectural reading

This subproject has grown beyond basic RAG. It already contains:

- classic chunk retrieval
- graph-oriented retrieval
- route/path-aware retrieval
- evaluation and safety-adjacent helpers

So its real role is closer to a retrieval platform SDK than a simple utility
library.

### Testing and maturity signals

Strong signals:

- extensive tests across retrieval, graph, storage, and evaluation layers
- API snapshot file and compatibility docs
- explicit conformance tests for backends
- stable `v1.0` positioning in README

Current maturity assessment:

- most mature and stability-focused library in the ecosystem
- clearly intended as the compatibility anchor for downstream projects
- suitable as the lowest-level reusable data/retrieval foundation

## `llm-agent-providers`

### Role

`llm-agent-providers` is the provider adapter repository. It converts concrete
vendor APIs into the interfaces defined by `llm-agent/llm`.

### Public capability surface

Currently implemented providers:

- OpenAI
- Anthropic
- Ollama
- DeepSeek
- MiniMax

Across those providers, the repo implements combinations of:

- synchronous generation
- streaming generation
- native tool/function calling
- embeddings where the provider supports it
- model-aware capability exposure

### Directory responsibilities

- `openai/`: OpenAI adapter, request/response mapping, options, tests
- `anthropic/`: Anthropic adapter and mapping layer
- `ollama/`: Ollama adapter, embedding strategy, tool strategy
- `deepseek/`: DeepSeek adapter and regional options
- `minimax/`: MiniMax adapter and regional options
- `internal/contract/`: cross-provider contract/conformance coverage
- `scripts/`: fixture capture and workspace helper scripts

### Architectural reading

This repository acts as the compatibility boundary between the abstract agent
framework and real LLM vendors. The design emphasizes:

- per-provider isolation
- shared capability contracts
- model-aware truthfulness about supported features

It is intentionally not trying to hide every provider difference. The file
layout suggests the adapters preserve provider-specific behavior where needed
while still fitting a common interface.

### Testing and maturity signals

Strong signals:

- each provider has dedicated tests
- shared contract coverage exists in `internal/contract`
- fixture capture scripts suggest repeatable request/response verification

Current maturity assessment:

- focused and fairly modular integration layer
- mature enough for real adapter composition
- ongoing risk remains at the provider API boundary, not in repo structure

## `llm-agent-otel`

### Role

`llm-agent-otel` adds observability without violating the stdlib-only rule of
the core framework. It is an instrumentation wrapper layer.

### Public capability surface

The current observability surface includes:

- model instrumentation through `otelmodel`
- agent instrumentation through `otelagent`
- RAG instrumentation through `otelrag`
- low-cardinality metrics helpers
- slog bridge integration
- exporter wiring for OTLP HTTP and gRPC
- demo compose setup for local observability

### Directory responsibilities

- `otelmodel/`: wraps `llm.ChatModel` and preserves capability interfaces
- `otelagent/`: wraps `agents.Agent`
- `otelrag/`: wraps retrieval/RAG workflows
- `otelmetrics/`: metrics helpers and conventions
- `otelslog/`: slog handler bridge
- root exporter files: shared exporter creation and defaults
- `compose/`: local observability demo stack
- `cmd/tailprobe/`: small executable utility related to trace sampling/testing

### Architectural reading

This subproject is a clean decorator layer. It keeps observability concerns out
of the core framework and adds instrumentation where needed at the edges.

That separation is important because it preserves:

- stdlib-only guarantees in `llm-agent`
- optional adoption of telemetry
- independent release cadence for instrumentation concerns

### Testing and maturity signals

Strong signals:

- tests for each wrapper package
- tests for exporter setup and semantic-convention helpers
- explicit local demo stack for end-to-end inspection

Current maturity assessment:

- clear and bounded infrastructure component
- pragmatic for local demos and integration environments
- likely stable enough for adoption where OTel is already a standard

## `llm-agent-customer-support`

### Role

`llm-agent-customer-support` is the ecosystem's reference application. It shows
how the framework, providers, RAG, and observability pieces fit together in a
single service.

### Public capability surface

The currently visible service surface includes:

- HTTP chat API
- SSE chat streaming
- health and readiness probes
- provider-selectable chat and embeddings
- RAG-backed support knowledge lookup
- state-graph-based support triage
- tool-backed workflows
- session persistence with SQLite or Postgres
- runtime limits and panic switch
- prompt-injection and untrusted-content guardrails
- OpenTelemetry instrumentation with a local Grafana stack

### Directory responsibilities

- `cmd/server/`: application entrypoint and process lifecycle
- `internal/app/`: composition root, startup wiring, model/session/bootstrap
- `internal/config/`: environment parsing and runtime configuration
- `internal/providers/`: chat/embedding provider factory seam
- `internal/httpapi/`: REST/SSE transport handlers and response behavior
- `internal/knowledgebase/`: seed knowledge-base handling
- `internal/supportflow/`: triage, routing, support tools, orchestration logic
- `internal/sessionstore/`: durable session abstraction and implementations
- `internal/limits/`: runtime caps, quotas, and panic-switch handling
- `internal/guardrails/`: suspicious-input handling and prompt-safety policy
- `compose/`: local demo runtime stack
- `dashboards/`: Grafana dashboard assets

### Architectural reading

This repo is the only part of the ecosystem that behaves like an end-user
service. It is the integration proving ground for the lower-level libraries.

Its structure shows a conventional service composition:

- configuration
- composition root
- transport layer
- domain flow
- persistence
- operational safety

The business domain is intentionally narrow: support triage and support-answer
generation. That makes it a reference implementation more than a general SaaS
product.

### Testing and maturity signals

Strong signals:

- tests for app wiring, config, HTTP API, guardrails, limits, knowledgebase,
  session store, and support flow
- docker-compose demo stack and dashboard assets
- explicit day-one operational guardrails documented in README

Current maturity assessment:

- strongest demonstration of real ecosystem composition
- production-oriented in shape, but still demo/reference-oriented in hardening
- best repository to inspect for "how the ecosystem is meant to be used"

## Cross-Project Conclusions

### What is already implemented

At the ecosystem level, the current project already delivers:

- a reusable Go agent framework
- a standalone RAG platform SDK
- multiple model-provider adapters
- optional OpenTelemetry instrumentation
- a runnable reference customer-support service

This is a real layered system, not just a concept repo.

### What the architecture optimizes for

The codebase consistently optimizes for:

- separation of concerns by repository
- optionality of infrastructure dependencies
- testable abstractions over vendor lock-in
- local development across multiple repos via workspace tooling
- keeping the core framework lightweight while pushing integrations outward

### Maturity by layer

- most stable: `llm-agent-rag`
- most foundational: `llm-agent`
- most integration-heavy: `llm-agent-providers`
- most infrastructure-oriented: `llm-agent-otel`
- most product-like: `llm-agent-customer-support`

### Main limitations visible from the current structure

- the root repo itself is not a unified product runtime
- the ecosystem depends on multi-repo coordination rather than a monorepo model
- some surfaces remain explicitly demo or pre-release oriented
- operational hardening is strongest in the reference app docs, not necessarily
  guaranteed across the entire stack

## Recommended Reading Order

For a new maintainer or evaluator, the most effective reading order is:

1. root `README.md`
2. `llm-agent/README.md`
3. `llm-agent-rag/README.md`
4. `llm-agent-customer-support/README.md`
5. `llm-agent-providers/README.md`
6. `llm-agent-otel/README.md`

That order mirrors the conceptual stack: framework first, retrieval second,
reference application third, integrations afterward.

## Bottom Line

The current project implements an ecosystem for building and observing Go-based
LLM agents rather than a single product. The core value is the combination of:

- a lightweight agent framework
- a separate retrieval platform
- provider adapters
- observability wrappers
- a reference application that proves the pieces fit together

From the current directory structure and repository contents, this ecosystem is
already functionally rich and intentionally layered, with the customer-support
service serving as the concrete end-to-end demonstration of the architecture.
