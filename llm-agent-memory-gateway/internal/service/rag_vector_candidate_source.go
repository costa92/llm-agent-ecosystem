package service

import (
	"context"
	"fmt"

	"github.com/costa92/llm-agent-memory-gateway/internal/authz"
	pgmemory "github.com/costa92/llm-agent-memory-postgres/postgres"
	ragembed "github.com/costa92/llm-agent-rag/embed"
	ragstore "github.com/costa92/llm-agent-rag/store"
)

type RAGStoreVectorCandidateSource struct {
	embedder  ragembed.Embedder
	store     ragstore.Store
	namespace string
}

func NewRAGStoreVectorCandidateSource(embedder ragembed.Embedder, store ragstore.Store, namespace string) *RAGStoreVectorCandidateSource {
	return &RAGStoreVectorCandidateSource{
		embedder:  embedder,
		store:     store,
		namespace: namespace,
	}
}

func (s *RAGStoreVectorCandidateSource) Store() ragstore.Store {
	return s.store
}

func (s *RAGStoreVectorCandidateSource) RecallCandidates(ctx context.Context, scope authz.Scope, query string, topK int) ([]RecallCandidate, error) {
	if s.embedder == nil {
		return nil, fmt.Errorf("memory-gateway/service: rag vector candidate source requires an embedder")
	}
	if s.store == nil {
		return nil, fmt.Errorf("memory-gateway/service: rag vector candidate source requires a store")
	}
	if topK <= 0 {
		topK = 8
	}

	vec, err := s.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("memory-gateway/service: embed recall query: %w", err)
	}
	hits, err := s.store.Search(ctx, ragstore.Query{
		Namespace:       resolveVectorNamespace(s.namespace, scope),
		Vector:          vec,
		TopK:            topK,
		SecurityFilters: vectorSecurityFilters(scope),
	})
	if err != nil {
		if err == ragstore.ErrNotFound {
			return nil, pgmemory.ErrNotFound
		}
		return nil, err
	}
	if len(hits) == 0 {
		return nil, pgmemory.ErrNotFound
	}

	candidates := make([]RecallCandidate, 0, len(hits))
	for _, hit := range hits {
		if hit.Chunk.ID == "" {
			continue
		}
		candidates = append(candidates, RecallCandidate{
			MemoryID: hit.Chunk.ID,
			Score:    hit.Score,
		})
	}
	if len(candidates) == 0 {
		return nil, pgmemory.ErrNotFound
	}
	return candidates, nil
}

func resolveVectorNamespace(configured string, scope authz.Scope) string {
	if configured != "" {
		return configured
	}
	return scope.TenantID
}

func vectorSecurityFilters(scope authz.Scope) ragstore.Filter {
	filters := ragstore.Filter{
		"tenant_id": scope.TenantID,
		"user_id":   scope.UserID,
	}
	if scope.ProjectID != "" {
		filters["project_id"] = scope.ProjectID
	}
	if scope.SessionID != "" {
		filters["session_id"] = scope.SessionID
	}
	return filters
}
