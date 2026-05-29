# llm-agent-memory-contract

Stdlib-only, backend-neutral durable-memory contract for the llm-agent
ecosystem. Extracted verbatim from `llm-agent-memory/memory/durable.go`.

## What it contains

- `MemoryRecord` and the persisted aggregates `StoredEvent`, `OutboxMessage`,
  `IdempotencyEntry`.
- All `*Input` / `*Result` DTOs and `DedupeAction`.
- The 8 storage-port interfaces: `RecordStore`, `Promoter`, `Deduper`,
  `AccessMarker`, `EventStore`, `IdempotencyStore`, `Outbox`,
  `MessagePublisher`.
- `RecordKind*` / `Dedupe*` constants, `ErrInvalidRecordKind`, and the
  `NormalizeRecordKind` / `NormalizeWriteDefaults` / `SetWorkingDefault`
  helpers.

## Stability contract

This module is a **persisted JSON schema**, not just a DTO package. The four
aggregate types are marshaled with the default `encoding/json` (no tags)
straight into Postgres, so **wire keys equal Go field names**.

Do NOT, without a major version bump and a DB migration plan:

- rename a field;
- change a field between value and pointer form;
- change a field's type;
- add a `json` tag.

`golden_wire_test.go` pins the exact wire bytes. A red golden test means the
change would corrupt previously-persisted rows.

## Versioning

Independent module; tagged as `llm-agent-memory-contract/vX.Y.Z`. Consumers
must pin the SAME contract version during a coordinated release wave (the
alias shim in `llm-agent-memory` does NOT make mixed-version graphs safe).
