package contract

import (
	"encoding/json"
	"testing"
	"time"
)

// fixtureRecord is the canonical MemoryRecord used to pin the persisted JSON
// wire shape. durable.go carries NO json tags, so the wire keys equal the Go
// field names; any rename / pointer-vs-value change / type change will break
// this test and signal a Postgres-schema-breaking change.
func fixtureRecord() MemoryRecord {
	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	deletedAt := now.Add(time.Hour)
	lastAccess := now.Add(2 * time.Hour)
	return MemoryRecord{
		MemoryID:                "mem-1",
		TenantID:                "tenant-1",
		UserID:                  "user-1",
		ProjectID:               "proj-1",
		SessionID:               "sess-1",
		Kind:                    RecordKindSemantic,
		Source:                  "src",
		Category:                "cat",
		Content:                 "hello",
		NormalizedContentHash:   "hash",
		Tags:                    []string{"a", "b"},
		Importance:              0.5,
		Pinned:                  true,
		Disabled:                false,
		Deleted:                 false,
		Version:                 7,
		CreatedAt:               now,
		UpdatedAt:               now,
		DeletedAt:               &deletedAt,
		LastAccessAt:            &lastAccess,
		HitCount:                3,
		ConsolidatedFromEventID: "evt-9",
	}
}

func fixtureStoredEvent() StoredEvent {
	return StoredEvent{
		Version:        7,
		TenantID:       "tenant-1",
		MemoryID:       "mem-1",
		EventType:      "memory_written",
		IdempotencyKey: "idem-1",
		Record:         fixtureRecord(),
		Metadata:       map[string]any{"k": "v"},
	}
}

func fixtureOutboxMessage() OutboxMessage {
	return OutboxMessage{
		Version:   7,
		TenantID:  "tenant-1",
		MemoryID:  "mem-1",
		EventType: "memory_written",
		EventID:   "evt-1",
		Record:    fixtureRecord(),
		Metadata:  map[string]any{"k": "v"},
	}
}

func fixtureIdempotencyEntry() IdempotencyEntry {
	expires := time.Date(2024, 1, 9, 3, 4, 5, 0, time.UTC)
	return IdempotencyEntry{
		TenantID:       "tenant-1",
		IdempotencyKey: "idem-1",
		RequestHash:    "rh",
		MemoryID:       "mem-1",
		Response: WriteRecordResult{
			MemoryID: "mem-1",
			Version:  7,
			Created:  true,
			Record:   fixtureRecord(),
		},
		CreatedAt: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
		ExpiresAt: &expires,
	}
}

// Golden wire strings: captured from the byte-identical pre-move type. These
// pin the persisted Postgres JSON. A diff here means a DB-breaking change.
const goldenMemoryRecordJSON = `{"MemoryID":"mem-1","TenantID":"tenant-1","UserID":"user-1","ProjectID":"proj-1","SessionID":"sess-1","Kind":"semantic","Source":"src","Category":"cat","Content":"hello","NormalizedContentHash":"hash","Tags":["a","b"],"Importance":0.5,"Pinned":true,"Disabled":false,"Deleted":false,"Version":7,"CreatedAt":"2024-01-02T03:04:05Z","UpdatedAt":"2024-01-02T03:04:05Z","DeletedAt":"2024-01-02T04:04:05Z","LastAccessAt":"2024-01-02T05:04:05Z","HitCount":3,"ConsolidatedFromEventID":"evt-9"}`
const goldenStoredEventJSON = `{"Version":7,"TenantID":"tenant-1","MemoryID":"mem-1","EventType":"memory_written","IdempotencyKey":"idem-1","Record":{"MemoryID":"mem-1","TenantID":"tenant-1","UserID":"user-1","ProjectID":"proj-1","SessionID":"sess-1","Kind":"semantic","Source":"src","Category":"cat","Content":"hello","NormalizedContentHash":"hash","Tags":["a","b"],"Importance":0.5,"Pinned":true,"Disabled":false,"Deleted":false,"Version":7,"CreatedAt":"2024-01-02T03:04:05Z","UpdatedAt":"2024-01-02T03:04:05Z","DeletedAt":"2024-01-02T04:04:05Z","LastAccessAt":"2024-01-02T05:04:05Z","HitCount":3,"ConsolidatedFromEventID":"evt-9"},"Metadata":{"k":"v"}}`
const goldenOutboxMessageJSON = `{"Version":7,"TenantID":"tenant-1","MemoryID":"mem-1","EventType":"memory_written","EventID":"evt-1","Record":{"MemoryID":"mem-1","TenantID":"tenant-1","UserID":"user-1","ProjectID":"proj-1","SessionID":"sess-1","Kind":"semantic","Source":"src","Category":"cat","Content":"hello","NormalizedContentHash":"hash","Tags":["a","b"],"Importance":0.5,"Pinned":true,"Disabled":false,"Deleted":false,"Version":7,"CreatedAt":"2024-01-02T03:04:05Z","UpdatedAt":"2024-01-02T03:04:05Z","DeletedAt":"2024-01-02T04:04:05Z","LastAccessAt":"2024-01-02T05:04:05Z","HitCount":3,"ConsolidatedFromEventID":"evt-9"},"Metadata":{"k":"v"}}`
const goldenIdempotencyEntryJSON = `{"TenantID":"tenant-1","IdempotencyKey":"idem-1","RequestHash":"rh","MemoryID":"mem-1","Response":{"MemoryID":"mem-1","Version":7,"Created":true,"Record":{"MemoryID":"mem-1","TenantID":"tenant-1","UserID":"user-1","ProjectID":"proj-1","SessionID":"sess-1","Kind":"semantic","Source":"src","Category":"cat","Content":"hello","NormalizedContentHash":"hash","Tags":["a","b"],"Importance":0.5,"Pinned":true,"Disabled":false,"Deleted":false,"Version":7,"CreatedAt":"2024-01-02T03:04:05Z","UpdatedAt":"2024-01-02T03:04:05Z","DeletedAt":"2024-01-02T04:04:05Z","LastAccessAt":"2024-01-02T05:04:05Z","HitCount":3,"ConsolidatedFromEventID":"evt-9"}},"CreatedAt":"2024-01-02T03:04:05Z","ExpiresAt":"2024-01-09T03:04:05Z"}`

func TestGoldenWire_MemoryRecord(t *testing.T) {
	b, err := json.Marshal(fixtureRecord())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(b) != goldenMemoryRecordJSON {
		t.Fatalf("wire shape changed (Postgres-breaking!):\n got: %s\nwant: %s", b, goldenMemoryRecordJSON)
	}
	var back MemoryRecord
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
}

func TestGoldenWire_StoredEvent(t *testing.T) {
	b, err := json.Marshal(fixtureStoredEvent())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(b) != goldenStoredEventJSON {
		t.Fatalf("wire shape changed (Postgres-breaking!):\n got: %s\nwant: %s", b, goldenStoredEventJSON)
	}
}

func TestGoldenWire_OutboxMessage(t *testing.T) {
	b, err := json.Marshal(fixtureOutboxMessage())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(b) != goldenOutboxMessageJSON {
		t.Fatalf("wire shape changed (Postgres-breaking!):\n got: %s\nwant: %s", b, goldenOutboxMessageJSON)
	}
}

func TestGoldenWire_IdempotencyEntry(t *testing.T) {
	b, err := json.Marshal(fixtureIdempotencyEntry())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(b) != goldenIdempotencyEntryJSON {
		t.Fatalf("wire shape changed (Postgres-breaking!):\n got: %s\nwant: %s", b, goldenIdempotencyEntryJSON)
	}
}
