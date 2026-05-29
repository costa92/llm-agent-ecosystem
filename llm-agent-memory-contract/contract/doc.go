// Package contract defines the backend-neutral durable memory contract shared
// by llm-agent-memory-postgres, llm-agent-memory-worker, and
// llm-agent-memory-gateway.
//
// STABILITY POLICY (read before editing):
//
//   - This is the highest-stability API in the ecosystem. MemoryRecord,
//     StoredEvent, OutboxMessage, and IdempotencyEntry are serialized with
//     the standard encoding/json (NO json tags) directly into Postgres. The
//     wire keys therefore equal the Go field names.
//   - Renaming a field, changing a field between value and pointer form,
//     changing a field's type, or adding a json tag is a DATABASE MIGRATION,
//     not a code change. Such changes require a major version bump and a
//     migration plan.
//   - Additive, optional fields appended at the end (still tag-free) are a
//     minor version bump.
//   - The interfaces (RecordStore, Promoter, Deduper, AccessMarker,
//     EventStore, IdempotencyStore, Outbox, MessagePublisher) are consumed by
//     three runtime modules; adding a method is a breaking change for
//     implementers and requires a major version bump.
//   - SemVer is enforced per-module via the git tag
//     llm-agent-memory-contract/vX.Y.Z. The golden_wire_test.go guard MUST
//     stay green; a red golden test means you are about to break persisted
//     data.
package contract
