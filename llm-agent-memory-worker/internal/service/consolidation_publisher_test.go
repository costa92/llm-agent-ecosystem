package service

import (
	"context"
	"errors"
	"testing"

	pgmemory "github.com/costa92/llm-agent-memory-postgres/postgres"
	corememory "github.com/costa92/llm-agent-memory-contract/contract"
)

type fakeConsolidationStore struct {
	record      corememory.MemoryRecord
	getErr      error
	getCalls    int
	promoteIn   corememory.PromoteRecordInput
	promoteRes  corememory.PromoteRecordResult
	promoteErr  error
	promoteCall int
	dedupeIn    corememory.ResolveDedupeInput
	dedupeRes   corememory.ResolveDedupeResult
	dedupeErr   error
	dedupeCall  int
}

func (f *fakeConsolidationStore) GetRecord(_ context.Context, _, _ string) (corememory.MemoryRecord, error) {
	f.getCalls++
	if f.getErr != nil {
		return corememory.MemoryRecord{}, f.getErr
	}
	return f.record, nil
}

func (f *fakeConsolidationStore) WriteRecord(context.Context, corememory.WriteRecordInput) (corememory.WriteRecordResult, error) {
	return corememory.WriteRecordResult{}, nil
}

func (f *fakeConsolidationStore) PatchRecord(context.Context, corememory.PatchRecordInput) (corememory.PatchRecordResult, error) {
	return corememory.PatchRecordResult{}, nil
}

func (f *fakeConsolidationStore) DeleteRecord(context.Context, corememory.DeleteRecordInput) (corememory.DeleteRecordResult, error) {
	return corememory.DeleteRecordResult{}, nil
}

func (f *fakeConsolidationStore) PinRecord(context.Context, corememory.PinRecordInput) (corememory.PinRecordResult, error) {
	return corememory.PinRecordResult{}, nil
}

func (f *fakeConsolidationStore) DisableRecord(context.Context, corememory.DisableRecordInput) (corememory.DisableRecordResult, error) {
	return corememory.DisableRecordResult{}, nil
}

func (f *fakeConsolidationStore) Promote(_ context.Context, in corememory.PromoteRecordInput) (corememory.PromoteRecordResult, error) {
	f.promoteCall++
	f.promoteIn = in
	if f.promoteErr != nil {
		return corememory.PromoteRecordResult{}, f.promoteErr
	}
	return f.promoteRes, nil
}

func (f *fakeConsolidationStore) ResolveDedupe(_ context.Context, in corememory.ResolveDedupeInput) (corememory.ResolveDedupeResult, error) {
	f.dedupeCall++
	f.dedupeIn = in
	if f.dedupeErr != nil {
		return corememory.ResolveDedupeResult{}, f.dedupeErr
	}
	return f.dedupeRes, nil
}

func TestConsolidationPublisher_PromotesEligibleWorkingCreate(t *testing.T) {
	store := &fakeConsolidationStore{
		record: corememory.MemoryRecord{
			MemoryID:              "mem_1",
			TenantID:              "tenant-a",
			UserID:                "user-a",
			ProjectID:             "project-a",
			Kind:                  corememory.RecordKindWorking,
			Source:                "agent_inferred",
			Category:              "preference",
			NormalizedContentHash: "hash-a",
			Importance:            0.8,
			Version:               1,
		},
		dedupeRes: corememory.ResolveDedupeResult{WinnerID: "mem_1", Action: corememory.DedupeNoCollision},
		promoteRes: corememory.PromoteRecordResult{
			MemoryID: "mem_1",
			Version:  2,
			Created:  true,
		},
	}
	metrics := NewMetrics()
	publisher := NewConsolidationPublisher(store, metrics)

	err := publisher.Publish(context.Background(), corememory.OutboxMessage{
		EventType: "memory_created",
		MemoryID:  "mem_1",
		TenantID:  "tenant-a",
		EventID:   "evt_1",
		Version:   1,
		Record: corememory.MemoryRecord{
			MemoryID: "mem_1",
			TenantID: "tenant-a",
			Kind:     corememory.RecordKindWorking,
			Version:  1,
		},
	})
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if store.getCalls != 1 {
		t.Fatalf("GetRecord calls = %d, want 1", store.getCalls)
	}
	if store.dedupeCall != 1 {
		t.Fatalf("ResolveDedupe calls = %d, want 1", store.dedupeCall)
	}
	if store.promoteCall != 1 {
		t.Fatalf("Promote calls = %d, want 1", store.promoteCall)
	}
	if store.promoteIn.ExpectedVersion != 1 {
		t.Fatalf("ExpectedVersion = %d, want 1", store.promoteIn.ExpectedVersion)
	}
	if store.promoteIn.SourceEventID != "evt_1" {
		t.Fatalf("SourceEventID = %q, want evt_1", store.promoteIn.SourceEventID)
	}
	if store.promoteIn.IdempotencyKey == "" {
		t.Fatal("IdempotencyKey is empty")
	}
	if store.dedupeIn.DedupeKey == "" {
		t.Fatal("DedupeKey is empty")
	}
	snap := metrics.Snapshot()
	bucket := TenantBucket("tenant-a")
	if got := snap.PromoteAttemptTotal[bucket]; got != 1 {
		t.Fatalf("promote_attempt_total[%s] = %d, want 1", bucket, got)
	}
	if got := snap.PromoteAcceptedTotal[bucket]; got != 1 {
		t.Fatalf("promote_accepted_total[%s] = %d, want 1", bucket, got)
	}
	if got := snap.WorkingPromotedTotal[bucket]; got != 1 {
		t.Fatalf("working_promoted_total[%s] = %d, want 1", bucket, got)
	}
}

func TestConsolidationPublisher_IgnoresNonWorkingRecords(t *testing.T) {
	store := &fakeConsolidationStore{}
	publisher := NewConsolidationPublisher(store)

	err := publisher.Publish(context.Background(), corememory.OutboxMessage{
		EventType: "memory_created",
		MemoryID:  "mem_1",
		TenantID:  "tenant-a",
		EventID:   "evt_1",
		Version:   1,
		Record: corememory.MemoryRecord{
			MemoryID: "mem_1",
			TenantID: "tenant-a",
			Kind:     corememory.RecordKindEpisodic,
			Version:  1,
		},
	})
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if store.getCalls != 0 || store.dedupeCall != 0 || store.promoteCall != 0 {
		t.Fatalf("unexpected calls: get=%d dedupe=%d promote=%d", store.getCalls, store.dedupeCall, store.promoteCall)
	}
}

func TestConsolidationPublisher_IgnoresStaleWorkingCreate(t *testing.T) {
	store := &fakeConsolidationStore{
		record: corememory.MemoryRecord{
			MemoryID: "mem_1",
			TenantID: "tenant-a",
			Kind:     corememory.RecordKindWorking,
			Version:  2,
		},
	}
	publisher := NewConsolidationPublisher(store)

	err := publisher.Publish(context.Background(), corememory.OutboxMessage{
		EventType: "memory_created",
		MemoryID:  "mem_1",
		TenantID:  "tenant-a",
		EventID:   "evt_1",
		Version:   1,
		Record: corememory.MemoryRecord{
			MemoryID: "mem_1",
			TenantID: "tenant-a",
			Kind:     corememory.RecordKindWorking,
			Version:  1,
		},
	})
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if store.dedupeCall != 0 || store.promoteCall != 0 {
		t.Fatalf("unexpected calls: dedupe=%d promote=%d", store.dedupeCall, store.promoteCall)
	}
}

func TestConsolidationPublisher_StopsAfterDedupeCollision(t *testing.T) {
	store := &fakeConsolidationStore{
		record: corememory.MemoryRecord{
			MemoryID:              "mem_1",
			TenantID:              "tenant-a",
			UserID:                "user-a",
			Kind:                  corememory.RecordKindWorking,
			Source:                "user_saved",
			Category:              "preference",
			NormalizedContentHash: "hash-a",
			Version:               1,
		},
		dedupeRes: corememory.ResolveDedupeResult{WinnerID: "mem_existing", Action: corememory.DedupeMergedExisting},
	}
	publisher := NewConsolidationPublisher(store)

	err := publisher.Publish(context.Background(), corememory.OutboxMessage{
		EventType: "memory_created",
		MemoryID:  "mem_1",
		TenantID:  "tenant-a",
		EventID:   "evt_1",
		Version:   1,
		Record: corememory.MemoryRecord{
			MemoryID: "mem_1",
			TenantID: "tenant-a",
			Kind:     corememory.RecordKindWorking,
			Version:  1,
		},
	})
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if store.promoteCall != 0 {
		t.Fatalf("Promote calls = %d, want 0", store.promoteCall)
	}
}

func TestConsolidationPublisher_TreatsVersionConflictAsStale(t *testing.T) {
	store := &fakeConsolidationStore{
		record: corememory.MemoryRecord{
			MemoryID:              "mem_1",
			TenantID:              "tenant-a",
			UserID:                "user-a",
			Kind:                  corememory.RecordKindWorking,
			Source:                "user_saved",
			Category:              "preference",
			NormalizedContentHash: "hash-a",
			Version:               1,
		},
		dedupeRes:  corememory.ResolveDedupeResult{WinnerID: "mem_1", Action: corememory.DedupeNoCollision},
		promoteErr: pgmemory.ErrVersionConflict,
	}
	publisher := NewConsolidationPublisher(store)

	err := publisher.Publish(context.Background(), corememory.OutboxMessage{
		EventType: "memory_created",
		MemoryID:  "mem_1",
		TenantID:  "tenant-a",
		EventID:   "evt_1",
		Version:   1,
		Record: corememory.MemoryRecord{
			MemoryID: "mem_1",
			TenantID: "tenant-a",
			Kind:     corememory.RecordKindWorking,
			Version:  1,
		},
	})
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
}

func TestConsolidationPublisher_PropagatesUnexpectedErrors(t *testing.T) {
	wantErr := errors.New("boom")
	store := &fakeConsolidationStore{
		record: corememory.MemoryRecord{
			MemoryID:              "mem_1",
			TenantID:              "tenant-a",
			UserID:                "user-a",
			Kind:                  corememory.RecordKindWorking,
			Source:                "user_saved",
			Category:              "preference",
			NormalizedContentHash: "hash-a",
			Version:               1,
		},
		dedupeRes: corememory.ResolveDedupeResult{WinnerID: "mem_1", Action: corememory.DedupeNoCollision},
		getErr:    wantErr,
	}
	publisher := NewConsolidationPublisher(store)

	err := publisher.Publish(context.Background(), corememory.OutboxMessage{
		EventType: "memory_created",
		MemoryID:  "mem_1",
		TenantID:  "tenant-a",
		EventID:   "evt_1",
		Version:   1,
		Record: corememory.MemoryRecord{
			MemoryID: "mem_1",
			TenantID: "tenant-a",
			Kind:     corememory.RecordKindWorking,
			Version:  1,
		},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}
