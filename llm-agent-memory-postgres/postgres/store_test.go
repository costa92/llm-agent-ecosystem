package postgres

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	corememory "github.com/costa92/llm-agent-memory/memory"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestEventTypeConstants_PromotedAndDedupeCollapsed(t *testing.T) {
	// Compile-time sanity check: both new constants must exist with the
	// canonical spec strings.
	if eventTypeMemoryPromoted != "memory_promoted" {
		t.Fatalf("eventTypeMemoryPromoted = %q, want memory_promoted", eventTypeMemoryPromoted)
	}
	if eventTypeMemoryDedupeCollapsed != "memory_dedupe_collapsed" {
		t.Fatalf("eventTypeMemoryDedupeCollapsed = %q, want memory_dedupe_collapsed", eventTypeMemoryDedupeCollapsed)
	}
}

func TestValidateEventType_AcceptsKnownTypes(t *testing.T) {
	for _, et := range []string{
		eventTypeMemoryCreated,
		eventTypeMemoryUpdated,
		eventTypeMemoryDeleted,
		eventTypeMemoryPinned,
		eventTypeMemoryUnpinned,
		eventTypeMemoryDisabled,
		eventTypeMemoryEnabled,
		eventTypeMemoryPromoted,
		eventTypeMemoryDedupeCollapsed,
	} {
		if err := validateEventType(et); err != nil {
			t.Errorf("validateEventType(%q) = %v, want nil", et, err)
		}
	}
}

func TestValidateEventType_RejectsTypo(t *testing.T) {
	err := validateEventType("memry_created") // deliberate typo
	if err == nil {
		t.Fatal("expected error for typo")
	}
	if !errors.Is(err, ErrInvalidEventType) {
		t.Fatalf("err = %v, want ErrInvalidEventType", err)
	}
}

func TestValidateEventType_RejectsEmpty(t *testing.T) {
	err := validateEventType("")
	if err == nil {
		t.Fatal("expected error for empty event type")
	}
	if !errors.Is(err, ErrInvalidEventType) {
		t.Fatalf("err = %v, want ErrInvalidEventType", err)
	}
}

func TestAppendEvent_RejectsInvalidEventType(t *testing.T) {
	// No DB required: validation happens before any pool call.
	s := &Store{}
	err := s.AppendEvent(nil, corememory.StoredEvent{ //nolint:staticcheck
		MemoryID:  "mem_1",
		TenantID:  "tenant_1",
		EventType: "memry_created", // deliberate typo
		Version:   1,
	})
	if err == nil {
		t.Fatal("expected error for invalid event type")
	}
	if !errors.Is(err, ErrInvalidEventType) {
		t.Fatalf("err = %v, want ErrInvalidEventType", err)
	}
}

func TestEnqueueOutbox_RejectsInvalidEventType(t *testing.T) {
	s := &Store{}
	err := s.EnqueueOutbox(nil, corememory.OutboxMessage{ //nolint:staticcheck
		MemoryID:  "mem_1",
		TenantID:  "tenant_1",
		EventID:   "evt_1",
		EventType: "memry_created",
		Version:   1,
	})
	if err == nil {
		t.Fatal("expected error for invalid event type")
	}
	if !errors.Is(err, ErrInvalidEventType) {
		t.Fatalf("err = %v, want ErrInvalidEventType", err)
	}
}

func TestMutateRecord_RejectsInvalidEventType(t *testing.T) {
	s := &Store{}
	_, err := s.mutateRecord(nil, mutationInput{ //nolint:staticcheck
		tenantID:        "tenant_1",
		memoryID:        "mem_1",
		expectedVersion: 1,
		eventType:       "memry_updated", // deliberate typo
		apply:           func(*corememory.MemoryRecord, time.Time) {},
	})
	if err == nil {
		t.Fatal("expected error for invalid event type")
	}
	if !errors.Is(err, ErrInvalidEventType) {
		t.Fatalf("err = %v, want ErrInvalidEventType", err)
	}
}

func TestPostgresSurface_Compiles(t *testing.T) {
	var (
		_ error = ErrVersionConflict
		_ error = ErrIdempotencyConflict
		_ error = ErrNotFound
		_ error = ErrSchemaVersionAhead
		_ error = ErrInvalidEventType
	)

	_, _ = New((*pgxpool.Pool)(nil), Config{})

	var (
		_ corememory.RecordStore      = (*Store)(nil)
		_ corememory.EventStore       = (*Store)(nil)
		_ corememory.IdempotencyStore = (*Store)(nil)
		_ corememory.Outbox           = (*Store)(nil)
	)
}

func TestNewRejectsBadInputs(t *testing.T) {
	if _, err := New(nil, Config{}); err == nil {
		t.Fatal("expected error for nil pool")
	}
	if _, err := New(&pgxpool.Pool{}, Config{TablePrefix: "bad-prefix"}); err == nil {
		t.Fatal("expected error for invalid table prefix")
	}
}

func TestPostgresPackage_DoesNotDefineSDKTypeAliases(t *testing.T) {
	t.Helper()

	path := filepath.Join(".", "models.go")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	file, err := parser.ParseFile(token.NewFileSet(), path, src, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Assign == 0 {
				continue
			}
			t.Fatalf("compatibility alias forbidden in %s: %s", path, typeSpec.Name.Name)
		}
	}
}

func TestPostgresJSON_MemoryRecordRoundTrip(t *testing.T) {
	now := time.Unix(1712345678, 0).UTC()
	deletedAt := now.Add(5 * time.Minute)
	lastAccessAt := now.Add(10 * time.Minute)

	want := corememory.MemoryRecord{
		MemoryID:                "mem_123",
		TenantID:                "tenant_123",
		UserID:                  "user_123",
		ProjectID:               "proj_123",
		SessionID:               "sess_123",
		Kind:                    "episodic",
		Source:                  "user_saved",
		Category:                "fact",
		Content:                 "hello",
		NormalizedContentHash:   "hash_123",
		Tags:                    []string{"alpha", "beta"},
		Importance:              0.8,
		Pinned:                  true,
		Disabled:                false,
		Deleted:                 true,
		Version:                 7,
		CreatedAt:               now,
		UpdatedAt:               now,
		DeletedAt:               &deletedAt,
		LastAccessAt:            &lastAccessAt,
		HitCount:                9,
		ConsolidatedFromEventID: "evt_ancestor",
	}

	got, err := roundTripJSON(t, want)
	if err != nil {
		t.Fatalf("roundTripJSON() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestPostgresJSON_EventOutboxAndIdempotencyRoundTrip(t *testing.T) {
	now := time.Unix(1712349999, 0).UTC()

	record := corememory.MemoryRecord{
		MemoryID:              "mem_456",
		TenantID:              "tenant_456",
		UserID:                "user_456",
		Kind:                  "semantic",
		Source:                "agent_inferred",
		Category:              "summary",
		Content:               "content",
		NormalizedContentHash: "hash_456",
		Tags:                  []string{"tag"},
		Importance:            0.5,
		Version:               11,
		CreatedAt:             now,
		UpdatedAt:             now,
	}

	eventWant := corememory.StoredEvent{
		Version:        11,
		TenantID:       "tenant_456",
		MemoryID:       "mem_456",
		EventType:      "memory.record_written",
		IdempotencyKey: "idem_456",
		Record:         record,
	}
	eventGot, err := roundTripJSON(t, eventWant)
	if err != nil {
		t.Fatalf("roundTripJSON(event) error = %v", err)
	}
	if !reflect.DeepEqual(eventGot, eventWant) {
		t.Fatalf("event round trip mismatch\n got: %#v\nwant: %#v", eventGot, eventWant)
	}

	outboxWant := corememory.OutboxMessage{
		Version:   11,
		TenantID:  "tenant_456",
		MemoryID:  "mem_456",
		EventType: "memory.record_written",
		EventID:   "evt_456",
		Record:    record,
	}
	outboxGot, err := roundTripJSON(t, outboxWant)
	if err != nil {
		t.Fatalf("roundTripJSON(outbox) error = %v", err)
	}
	if !reflect.DeepEqual(outboxGot, outboxWant) {
		t.Fatalf("outbox round trip mismatch\n got: %#v\nwant: %#v", outboxGot, outboxWant)
	}

	idempotencyWant := corememory.IdempotencyEntry{
		TenantID:       "tenant_456",
		IdempotencyKey: "idem_456",
		RequestHash:    "req_hash_456",
		MemoryID:       "mem_456",
		Response: corememory.WriteRecordResult{
			MemoryID: "mem_456",
			Version:  11,
			Created:  true,
			Record:   record,
		},
		CreatedAt: now,
		ExpiresAt: ptrTime(now.Add(time.Hour)),
	}
	idempotencyGot, err := roundTripJSON(t, idempotencyWant)
	if err != nil {
		t.Fatalf("roundTripJSON(idempotency) error = %v", err)
	}
	if !reflect.DeepEqual(idempotencyGot, idempotencyWant) {
		t.Fatalf("idempotency round trip mismatch\n got: %#v\nwant: %#v", idempotencyGot, idempotencyWant)
	}
}

func roundTripJSON[T any](t *testing.T, want T) (T, error) {
	t.Helper()

	raw, err := encodeJSON(want)
	if err != nil {
		var zero T
		return zero, err
	}

	var got T
	if err := decodeJSON(raw, &got); err != nil {
		var zero T
		return zero, err
	}
	return got, nil
}

func ptrTime(v time.Time) *time.Time {
	return &v
}
