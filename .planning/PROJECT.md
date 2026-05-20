# llm-agent-ecosystem

Umbrella project for the `llm-agent` family.

## Mission

Provide a single workspace and coordination layer for the independent
`llm-agent` repositories.

## Subprojects

- `llm-agent`: core framework
- `llm-agent-rag`: RAG SDK
- `llm-agent-otel`: OpenTelemetry wrappers
- `llm-agent-providers`: provider adapters
- `llm-agent-customer-support`: reference application

## Boundaries

- The root does not own product code for any subproject.
- Each subproject keeps its own git history, CI, releases, and focused docs.
- The root owns navigation, shared conventions, and cross-repo coordination.

## Status

- **v1.0** — `llm-agent-rag` API stabilization — shipped 2026-05-21.
- **v1.1** — Ecosystem alignment — **shipped 2026-05-20**. 5/5 ECO
  requirements PASS; coordinated tag set verified end-to-end; umbrella
  dependency-currency CI gate live. Audit lives in
  `llm-agent/.planning/v1.1-MILESTONE-AUDIT.md`.
- **v1.2** — Core Capability Deepening — **in flight**. Core-feature
  milestone (budget / policy / orchestrate.Supervisor). Active phase:
  35 (CC-1 budget / cancellation context). Source of truth:
  `llm-agent/.planning/STATE.md`.
