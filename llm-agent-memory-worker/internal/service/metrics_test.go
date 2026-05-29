package service

import (
	"context"
	"strings"
	"testing"

	corememory "github.com/costa92/llm-agent-memory-contract/contract"
)

func TestMetrics_RecordPromoteAttemptAndAccepted(t *testing.T) {
	m := NewMetrics()
	m.RecordPromoteAttempt("tenant-a")
	m.RecordPromoteAccepted("tenant-a")
	m.RecordWorkingPromoted("tenant-a")

	snap := m.Snapshot()
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

func TestMetrics_RecordPromoteRejected(t *testing.T) {
	m := NewMetrics()
	m.RecordPromoteRejected("tenant-a")

	snap := m.Snapshot()
	bucket := TenantBucket("tenant-a")
	if got := snap.PromoteRejectedTotal[bucket]; got != 1 {
		t.Fatalf("promote_rejected_total[%s] = %d, want 1", bucket, got)
	}
}

func TestMetrics_HandlerExposesPromoteCounters(t *testing.T) {
	m := NewMetrics()
	m.RecordPromoteAttempt("tenant-a")
	m.RecordPromoteAccepted("tenant-a")
	m.RecordPromoteRejected("tenant-a")
	m.RecordWorkingPromoted("tenant-a")

	body := m.Render()
	bucket := TenantBucket("tenant-a")
	for _, want := range []string{
		`promote_attempt_total{tenant_bucket="` + bucket + `"} 1`,
		`promote_accepted_total{tenant_bucket="` + bucket + `"} 1`,
		`promote_rejected_total{tenant_bucket="` + bucket + `"} 1`,
		`working_promoted_total{tenant_bucket="` + bucket + `"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics output missing %q\n%s", want, body)
		}
	}
}

func TestConsolidationPublisher_RecordsRejectedWhenRuleSkipsPromotion(t *testing.T) {
	store := &fakeConsolidationStore{
		record: corememory.MemoryRecord{
			MemoryID:              "mem_1",
			TenantID:              "tenant-a",
			UserID:                "user-a",
			Kind:                  corememory.RecordKindWorking,
			Source:                "agent_inferred",
			Category:              "preference",
			NormalizedContentHash: "hash-a",
			Importance:            0.2,
			Version:               1,
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

	snap := metrics.Snapshot()
	bucket := TenantBucket("tenant-a")
	if got := snap.PromoteAttemptTotal[bucket]; got != 1 {
		t.Fatalf("promote_attempt_total[%s] = %d, want 1", bucket, got)
	}
	if got := snap.PromoteRejectedTotal[bucket]; got != 1 {
		t.Fatalf("promote_rejected_total[%s] = %d, want 1", bucket, got)
	}
}
