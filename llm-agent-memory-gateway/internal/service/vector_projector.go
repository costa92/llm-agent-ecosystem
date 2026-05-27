package service

import (
	"context"
	"fmt"

	"github.com/costa92/llm-agent-memory-gateway/internal/authz"
	corememory "github.com/costa92/llm-agent-memory/memory"
	ragembed "github.com/costa92/llm-agent-rag/embed"
	ragstore "github.com/costa92/llm-agent-rag/store"
)

type VectorProjector interface {
	ProjectUpsert(ctx context.Context, scope authz.Scope, record corememory.MemoryRecord) error
	ProjectRemove(ctx context.Context, scope authz.Scope, memoryID string) error
}

type RAGVectorProjector struct {
	embedder  ragembed.Embedder
	store     ragstore.Store
	namespace string
}

func NewRAGVectorProjector(embedder ragembed.Embedder, store ragstore.Store, namespace string) *RAGVectorProjector {
	return &RAGVectorProjector{
		embedder:  embedder,
		store:     store,
		namespace: namespace,
	}
}

func (p *RAGVectorProjector) ProjectUpsert(ctx context.Context, scope authz.Scope, record corememory.MemoryRecord) error {
	if p.embedder == nil {
		return fmt.Errorf("memory-gateway/service: rag vector projector requires an embedder")
	}
	if p.store == nil {
		return fmt.Errorf("memory-gateway/service: rag vector projector requires a store")
	}

	vec, err := p.embedder.Embed(ctx, record.Content)
	if err != nil {
		return fmt.Errorf("memory-gateway/service: embed memory record: %w", err)
	}
	chunk := ragstore.StoredChunk{
		ID:        record.MemoryID,
		Namespace: resolveVectorNamespace(p.namespace, scope),
		DocID:     record.MemoryID,
		Title:     record.Category,
		Content:   record.Content,
		Vector:    vec,
		Metadata:  vectorChunkMetadata(record),
	}
	return p.store.Upsert(ctx, []ragstore.StoredChunk{chunk})
}

func (p *RAGVectorProjector) ProjectRemove(ctx context.Context, _ authz.Scope, memoryID string) error {
	if p.store == nil {
		return fmt.Errorf("memory-gateway/service: rag vector projector requires a store")
	}
	if memoryID == "" {
		return nil
	}
	if err := p.store.Remove(ctx, memoryID); err != nil && err != ragstore.ErrNotFound {
		return err
	}
	return nil
}

func vectorChunkMetadata(record corememory.MemoryRecord) map[string]any {
	return map[string]any{
		"tenant_id":  record.TenantID,
		"user_id":    record.UserID,
		"project_id": record.ProjectID,
		"session_id": record.SessionID,
		"kind":       record.Kind,
		"source":     record.Source,
		"category":   record.Category,
		"version":    record.Version,
	}
}
