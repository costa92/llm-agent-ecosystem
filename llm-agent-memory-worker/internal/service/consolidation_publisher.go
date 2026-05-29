package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	pgmemory "github.com/costa92/llm-agent-memory-postgres/postgres"
	corememory "github.com/costa92/llm-agent-memory-contract/contract"
)

const agentInferredImportanceThreshold = 0.7

type consolidationStore interface {
	corememory.RecordStore
	corememory.Promoter
	corememory.Deduper
}

type ConsolidationPublisher struct {
	store   consolidationStore
	metrics *Metrics
}

func NewConsolidationPublisher(store consolidationStore, metrics ...*Metrics) *ConsolidationPublisher {
	var m *Metrics
	if len(metrics) > 0 {
		m = metrics[0]
	}
	return &ConsolidationPublisher{store: store, metrics: m}
}

func (p *ConsolidationPublisher) Publish(ctx context.Context, msg corememory.OutboxMessage) error {
	if p == nil || p.store == nil {
		return nil
	}
	if !isConsolidationEvent(msg.EventType) {
		return nil
	}
	if msg.Record.Kind != "" && msg.Record.Kind != corememory.RecordKindWorking {
		return nil
	}

	current, err := p.store.GetRecord(ctx, msg.TenantID, msg.MemoryID)
	if err != nil {
		if errors.Is(err, pgmemory.ErrNotFound) {
			return nil
		}
		return err
	}
	if current.Kind != corememory.RecordKindWorking || current.Deleted || current.Disabled {
		return nil
	}
	if current.Version != msg.Version {
		return nil
	}
	if p.metrics != nil {
		p.metrics.RecordPromoteAttempt(current.TenantID)
	}
	if !shouldPromote(current) {
		if p.metrics != nil {
			p.metrics.RecordPromoteRejected(current.TenantID)
		}
		return nil
	}

	dedupeResult, err := p.store.ResolveDedupe(ctx, corememory.ResolveDedupeInput{
		TenantID:  current.TenantID,
		DedupeKey: dedupeKey(current),
		Candidate: current,
	})
	if err != nil {
		if errors.Is(err, pgmemory.ErrVersionConflict) || errors.Is(err, pgmemory.ErrNotFound) {
			return nil
		}
		return err
	}
	if dedupeResult.Action != corememory.DedupeNoCollision || dedupeResult.WinnerID != current.MemoryID {
		if p.metrics != nil {
			p.metrics.RecordPromoteRejected(current.TenantID)
		}
		return nil
	}

	_, err = p.store.Promote(ctx, corememory.PromoteRecordInput{
		TenantID:        current.TenantID,
		MemoryID:        current.MemoryID,
		ExpectedVersion: current.Version,
		SourceEventID:   msg.EventID,
		IdempotencyKey:  promotionIdempotencyKey(current.TenantID, current.MemoryID, msg.EventID),
		Reason:          promoteReason(current),
	})
	if err != nil {
		if errors.Is(err, pgmemory.ErrVersionConflict) || errors.Is(err, pgmemory.ErrNotFound) {
			if p.metrics != nil {
				p.metrics.RecordPromoteRejected(current.TenantID)
			}
			return nil
		}
		return err
	}
	if p.metrics != nil {
		p.metrics.RecordPromoteAccepted(current.TenantID)
		p.metrics.RecordWorkingPromoted(current.TenantID)
	}
	return nil
}

func isConsolidationEvent(eventType string) bool {
	switch eventType {
	case "memory_created", "memory_updated":
		return true
	default:
		return false
	}
}

func shouldPromote(record corememory.MemoryRecord) bool {
	switch record.Source {
	case "user_saved":
		return true
	case "agent_inferred":
		return record.Importance >= agentInferredImportanceThreshold
	default:
		return false
	}
}

func dedupeKey(record corememory.MemoryRecord) string {
	return hashParts(
		record.TenantID,
		record.UserID,
		record.Category,
		record.ProjectID,
		normalizedDedupeContent(record),
	)
}

func normalizedDedupeContent(record corememory.MemoryRecord) string {
	if strings.TrimSpace(record.NormalizedContentHash) != "" {
		return strings.TrimSpace(record.NormalizedContentHash)
	}
	return collapseWhitespace(strings.ToLower(record.Content))
}

func promotionIdempotencyKey(tenantID, memoryID, eventID string) string {
	return hashParts(tenantID, memoryID, eventID, "promote")
}

func promoteReason(record corememory.MemoryRecord) string {
	if record.Source == "user_saved" {
		return "user_saved_default"
	}
	if record.Source == "agent_inferred" && record.Importance >= agentInferredImportanceThreshold {
		return "agent_inferred_importance_threshold"
	}
	return "worker_default_rule"
}

func hashParts(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}

var _ interface{ Publish(context.Context, corememory.OutboxMessage) error } = (*ConsolidationPublisher)(nil)

func (p *ConsolidationPublisher) String() string {
	if p == nil {
		return "ConsolidationPublisher(<nil>)"
	}
	return fmt.Sprintf("ConsolidationPublisher(store=%T)", p.store)
}
