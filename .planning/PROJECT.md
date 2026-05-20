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
