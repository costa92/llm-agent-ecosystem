# llm-agent-memory-worker

Asynchronous consolidation worker for durable memory.

## Scope

This module owns:

- relay-driven outbox consumption for consolidation
- working to episodic promotion decisions
- durable dedupe before promotion
- worker process startup and runtime configuration

This module depends on:

- `github.com/costa92/llm-agent-memory`
- `github.com/costa92/llm-agent-memory-postgres`

## Runtime configuration

- `LLM_AGENT_MEMORY_WORKER_PG_URL` required
- `LLM_AGENT_MEMORY_WORKER_POLL_INTERVAL` optional, default `1s`
- `LLM_AGENT_MEMORY_WORKER_RELAY_LEASE_TTL` optional, default `180s`
- `LLM_AGENT_MEMORY_WORKER_RELAY_MAX_ATTEMPTS` optional, default `5`
- `LLM_AGENT_MEMORY_WORKER_RELAY_BATCH_SIZE` optional, default `100`

## Run

```bash
LLM_AGENT_MEMORY_WORKER_PG_URL=postgres://... GOWORK=off go run ./cmd/memory-worker
```
