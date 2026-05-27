package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	corememory "github.com/costa92/llm-agent-memory/memory"
)

func TestRelay_RunOnceWithoutPendingRows(t *testing.T) {
	r, err := NewRelay(&Store{}, &MemoryPublisher{}, 10)
	if err != nil {
		t.Fatalf("NewRelay: %v", err)
	}
	if r.batchSize != 10 {
		t.Fatalf("batchSize = %d, want 10", r.batchSize)
	}
}

func TestRelay_NewRejectsNilPublisher(t *testing.T) {
	_, err := NewRelay(&Store{}, nil, 1)
	if err == nil {
		t.Fatal("expected error for nil publisher")
	}
}

func TestMemoryPublisher_PublishStoresPayload(t *testing.T) {
	p := &MemoryPublisher{}
	payload := corememory.OutboxMessage{MemoryID: "mem_1", EventType: "memory_created", Version: 1}
	if err := p.Publish(context.Background(), payload); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if len(p.Events) != 1 || p.Events[0].MemoryID != payload.MemoryID {
		t.Fatalf("events = %+v", p.Events)
	}
}

func TestRelay_RunOnceMarksSentOnSuccess(t *testing.T) {
	ctx := context.Background()
	pool := openTestPool(t, ctx)

	prefix := fmt.Sprintf("m5_%d_relay_sent", time.Now().UnixNano())
	s, err := New(pool, Config{TablePrefix: prefix})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if _, err := s.WriteRecord(ctx, corememory.WriteRecordInput{
		TenantID:       "tenant_a",
		IdempotencyKey: "idem_relay_sent",
		RequestHash:    "hash_relay_sent",
		Record: corememory.MemoryRecord{
			UserID:                "user_a",
			Kind:                  "episodic",
			Source:                "user_saved",
			Category:              "project",
			Content:               "relay sent",
			NormalizedContentHash: "relay-hash-sent",
			Tags:                  []string{"relay"},
			Importance:            0.9,
		},
	}); err != nil {
		t.Fatalf("WriteRecord: %v", err)
	}

	publisher := &MemoryPublisher{}
	relay, err := NewRelay(s, publisher, 10)
	if err != nil {
		t.Fatalf("NewRelay: %v", err)
	}
	stats, err := relay.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if stats.Published != 1 || stats.Failed != 0 {
		t.Fatalf("stats = %+v", stats)
	}
	if len(publisher.Events) != 1 {
		t.Fatalf("published events = %d, want 1", len(publisher.Events))
	}
	assertOutboxStatus(t, ctx, pool, s.outboxTable(), "sent", 1)
}

func TestRelay_RunOnceMarksFailedOnPublisherError(t *testing.T) {
	ctx := context.Background()
	pool := openTestPool(t, ctx)

	prefix := fmt.Sprintf("m5_%d_relay_failed", time.Now().UnixNano())
	s, err := New(pool, Config{TablePrefix: prefix})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if _, err := s.WriteRecord(ctx, corememory.WriteRecordInput{
		TenantID:       "tenant_a",
		IdempotencyKey: "idem_relay_failed",
		RequestHash:    "hash_relay_failed",
		Record: corememory.MemoryRecord{
			UserID:                "user_a",
			Kind:                  "episodic",
			Source:                "user_saved",
			Category:              "project",
			Content:               "relay failed",
			NormalizedContentHash: "relay-hash-failed",
			Tags:                  []string{"relay"},
			Importance:            0.9,
		},
	}); err != nil {
		t.Fatalf("WriteRecord: %v", err)
	}

	publisher := &MemoryPublisher{Fail: true}
	relay, err := NewRelay(s, publisher, 10)
	if err != nil {
		t.Fatalf("NewRelay: %v", err)
	}
	stats, err := relay.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if stats.Published != 0 || stats.Failed != 1 {
		t.Fatalf("stats = %+v", stats)
	}
	assertOutboxStatus(t, ctx, pool, s.outboxTable(), outboxStatusPending, 1)
	assertOutboxAttemptCount(t, ctx, pool, s.outboxTable(), 1)
}

func TestRelay_RunOnceClaimsRowsBeforePublish(t *testing.T) {
	ctx := context.Background()
	pool := openTestPool(t, ctx)

	prefix := fmt.Sprintf("m5_%d_relay_claim", time.Now().UnixNano())
	s, err := New(pool, Config{TablePrefix: prefix})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if _, err := s.WriteRecord(ctx, corememory.WriteRecordInput{
		TenantID:       "tenant_a",
		IdempotencyKey: "idem_relay_claim",
		RequestHash:    "hash_relay_claim",
		Record: corememory.MemoryRecord{
			UserID:                "user_a",
			Kind:                  "episodic",
			Source:                "user_saved",
			Category:              "project",
			Content:               "relay claim",
			NormalizedContentHash: "relay-hash-claim",
			Tags:                  []string{"relay"},
			Importance:            0.9,
		},
	}); err != nil {
		t.Fatalf("WriteRecord: %v", err)
	}

	relay, err := NewRelay(s, &MemoryPublisher{}, 10)
	if err != nil {
		t.Fatalf("NewRelay: %v", err)
	}
	tx, claimed, err := relay.claimPending(ctx)
	if err != nil {
		t.Fatalf("claimPending: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if len(claimed) != 1 {
		t.Fatalf("claimed = %d, want 1", len(claimed))
	}

	secondTx, secondClaimed, err := relay.claimPending(ctx)
	if err != nil {
		t.Fatalf("second claimPending: %v", err)
	}
	defer secondTx.Rollback(ctx) //nolint:errcheck
	if len(secondClaimed) != 0 {
		t.Fatalf("second claimed = %d, want 0", len(secondClaimed))
	}
}

func assertOutboxStatus(t *testing.T, ctx context.Context, pool dbQueryer, table, wantStatus string, wantCount int) {
	t.Helper()
	var got int
	if err := pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE status = $1`, table),
		wantStatus,
	).Scan(&got); err != nil {
		t.Fatalf("outbox status count: %v", err)
	}
	if got != wantCount {
		t.Fatalf("status=%s count = %d, want %d", wantStatus, got, wantCount)
	}
}

func assertOutboxAttemptCount(t *testing.T, ctx context.Context, pool dbQueryer, table string, want int) {
	t.Helper()
	var got int
	if err := pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT attempt_count FROM %s LIMIT 1`, table),
	).Scan(&got); err != nil {
		t.Fatalf("outbox attempt_count: %v", err)
	}
	if got != want {
		t.Fatalf("attempt_count = %d, want %d", got, want)
	}
}
