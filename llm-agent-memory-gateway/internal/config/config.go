package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

const (
	defaultListenAddr     = ":8080"
	defaultSessionIdleTTL = 30 * time.Minute
	defaultRecallMode     = "lexical"
	defaultVectorTable    = "memory_gateway_vectors"
	defaultVectorIndex    = "none"
	defaultVectorDim      = 32
	defaultOutboxPoll     = time.Second
	defaultOutboxBatch    = 100
)

type Config struct {
	ListenAddr         string
	PostgresURL        string
	ReadOnly           bool
	SessionIdleTTL     time.Duration
	RecallMode         string
	VectorEnabled      bool
	VectorTable        string
	VectorDimension    int
	VectorNamespace    string
	VectorIndex        string
	OutboxPollInterval time.Duration
	OutboxBatchSize    int

	// EmbeddingCostMicrosPerToken is the unit cost (in micro-units) applied to
	// the embedding_cost_total counter on every successful Embed call. The
	// gateway treats "tokens" as the whitespace-separated word count of the
	// embedded text (the rag/embed SDK does not return token counts at v1);
	// see internal/service/vector_projector.go for the count source.
	// Default 0 keeps the counter at zero for deployments that haven't wired
	// up a cost rate yet.
	EmbeddingCostMicrosPerToken uint64
}

func LoadFromEnv() (Config, error) {
	cfg := Config{
		ListenAddr:         defaultListenAddr,
		PostgresURL:        os.Getenv("LLM_AGENT_MEMORY_PG_URL"),
		SessionIdleTTL:     defaultSessionIdleTTL,
		RecallMode:         defaultRecallMode,
		VectorTable:        defaultVectorTable,
		VectorDimension:    defaultVectorDim,
		VectorIndex:        defaultVectorIndex,
		OutboxPollInterval: defaultOutboxPoll,
		OutboxBatchSize:    defaultOutboxBatch,
	}

	if listenAddr := os.Getenv("LLM_AGENT_MEMORY_GATEWAY_ADDR"); listenAddr != "" {
		cfg.ListenAddr = listenAddr
	}

	if cfg.PostgresURL == "" {
		return Config{}, errors.New("LLM_AGENT_MEMORY_PG_URL is required")
	}

	if readOnlyValue := os.Getenv("LLM_AGENT_MEMORY_GATEWAY_READ_ONLY"); readOnlyValue != "" {
		readOnly, err := strconv.ParseBool(readOnlyValue)
		if err != nil {
			return Config{}, fmt.Errorf("parse LLM_AGENT_MEMORY_GATEWAY_READ_ONLY: %w", err)
		}
		cfg.ReadOnly = readOnly
	}

	if ttlValue := os.Getenv("LLM_AGENT_MEMORY_GATEWAY_SESSION_IDLE_TTL"); ttlValue != "" {
		ttl, err := time.ParseDuration(ttlValue)
		if err != nil {
			return Config{}, fmt.Errorf("parse LLM_AGENT_MEMORY_GATEWAY_SESSION_IDLE_TTL: %w", err)
		}
		if ttl <= 0 {
			return Config{}, errors.New("LLM_AGENT_MEMORY_GATEWAY_SESSION_IDLE_TTL must be > 0")
		}
		cfg.SessionIdleTTL = ttl
	}

	if recallMode := os.Getenv("LLM_AGENT_MEMORY_GATEWAY_RECALL_MODE"); recallMode != "" {
		switch recallMode {
		case "lexical", "hybrid":
			cfg.RecallMode = recallMode
		default:
			return Config{}, fmt.Errorf("LLM_AGENT_MEMORY_GATEWAY_RECALL_MODE must be lexical or hybrid")
		}
	}

	if vectorEnabledValue := os.Getenv("LLM_AGENT_MEMORY_GATEWAY_VECTOR_ENABLED"); vectorEnabledValue != "" {
		vectorEnabled, err := strconv.ParseBool(vectorEnabledValue)
		if err != nil {
			return Config{}, fmt.Errorf("parse LLM_AGENT_MEMORY_GATEWAY_VECTOR_ENABLED: %w", err)
		}
		cfg.VectorEnabled = vectorEnabled
	}

	if vectorTable := os.Getenv("LLM_AGENT_MEMORY_GATEWAY_VECTOR_TABLE"); vectorTable != "" {
		cfg.VectorTable = vectorTable
	}

	if vectorDimensionValue := os.Getenv("LLM_AGENT_MEMORY_GATEWAY_VECTOR_DIMENSION"); vectorDimensionValue != "" {
		vectorDimension, err := strconv.Atoi(vectorDimensionValue)
		if err != nil {
			return Config{}, fmt.Errorf("parse LLM_AGENT_MEMORY_GATEWAY_VECTOR_DIMENSION: %w", err)
		}
		if vectorDimension <= 0 {
			return Config{}, errors.New("LLM_AGENT_MEMORY_GATEWAY_VECTOR_DIMENSION must be > 0")
		}
		cfg.VectorDimension = vectorDimension
	}

	if vectorNamespace := os.Getenv("LLM_AGENT_MEMORY_GATEWAY_VECTOR_NAMESPACE"); vectorNamespace != "" {
		cfg.VectorNamespace = vectorNamespace
	}

	if vectorIndex := os.Getenv("LLM_AGENT_MEMORY_GATEWAY_VECTOR_INDEX"); vectorIndex != "" {
		switch vectorIndex {
		case "none", "ivfflat", "hnsw":
			cfg.VectorIndex = vectorIndex
		default:
			return Config{}, fmt.Errorf("LLM_AGENT_MEMORY_GATEWAY_VECTOR_INDEX must be none, ivfflat, or hnsw")
		}
	}

	if outboxPollValue := os.Getenv("LLM_AGENT_MEMORY_GATEWAY_OUTBOX_POLL_INTERVAL"); outboxPollValue != "" {
		pollInterval, err := time.ParseDuration(outboxPollValue)
		if err != nil {
			return Config{}, fmt.Errorf("parse LLM_AGENT_MEMORY_GATEWAY_OUTBOX_POLL_INTERVAL: %w", err)
		}
		if pollInterval <= 0 {
			return Config{}, errors.New("LLM_AGENT_MEMORY_GATEWAY_OUTBOX_POLL_INTERVAL must be > 0")
		}
		cfg.OutboxPollInterval = pollInterval
	}

	if outboxBatchValue := os.Getenv("LLM_AGENT_MEMORY_GATEWAY_OUTBOX_BATCH_SIZE"); outboxBatchValue != "" {
		batchSize, err := strconv.Atoi(outboxBatchValue)
		if err != nil {
			return Config{}, fmt.Errorf("parse LLM_AGENT_MEMORY_GATEWAY_OUTBOX_BATCH_SIZE: %w", err)
		}
		if batchSize <= 0 {
			return Config{}, errors.New("LLM_AGENT_MEMORY_GATEWAY_OUTBOX_BATCH_SIZE must be > 0")
		}
		cfg.OutboxBatchSize = batchSize
	}

	if costValue := os.Getenv("LLM_AGENT_MEMORY_GATEWAY_EMBED_COST_MICROS"); costValue != "" {
		cost, err := strconv.ParseUint(costValue, 10, 64)
		if err != nil {
			return Config{}, fmt.Errorf("parse LLM_AGENT_MEMORY_GATEWAY_EMBED_COST_MICROS: %w", err)
		}
		cfg.EmbeddingCostMicrosPerToken = cost
	}

	return cfg, nil
}
