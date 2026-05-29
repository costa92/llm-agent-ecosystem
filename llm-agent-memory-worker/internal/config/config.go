package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

const (
	defaultPollInterval = time.Second
	defaultRelayLeaseTTL = 180 * time.Second
	defaultRelayMaxAttempts = 5
	defaultRelayBatchSize = 100
)

type Config struct {
	PostgresURL  string
	PollInterval time.Duration
	RelayLeaseTTL time.Duration
	RelayMaxAttempts int
	RelayBatchSize int
}

func LoadFromEnv() (Config, error) {
	cfg := Config{
		PostgresURL: os.Getenv("LLM_AGENT_MEMORY_WORKER_PG_URL"),
		PollInterval: defaultPollInterval,
		RelayLeaseTTL: defaultRelayLeaseTTL,
		RelayMaxAttempts: defaultRelayMaxAttempts,
		RelayBatchSize: defaultRelayBatchSize,
	}
	if cfg.PostgresURL == "" {
		cfg.PostgresURL = os.Getenv("LLM_AGENT_MEMORY_PG_URL")
	}
	if cfg.PostgresURL == "" {
		return Config{}, errors.New("LLM_AGENT_MEMORY_WORKER_PG_URL is required")
	}

	if value := os.Getenv("LLM_AGENT_MEMORY_WORKER_POLL_INTERVAL"); value != "" {
		d, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, fmt.Errorf("parse LLM_AGENT_MEMORY_WORKER_POLL_INTERVAL: %w", err)
		}
		if d <= 0 {
			return Config{}, errors.New("LLM_AGENT_MEMORY_WORKER_POLL_INTERVAL must be > 0")
		}
		cfg.PollInterval = d
	}
	if value := os.Getenv("LLM_AGENT_MEMORY_WORKER_RELAY_LEASE_TTL"); value != "" {
		d, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, fmt.Errorf("parse LLM_AGENT_MEMORY_WORKER_RELAY_LEASE_TTL: %w", err)
		}
		if d <= 0 {
			return Config{}, errors.New("LLM_AGENT_MEMORY_WORKER_RELAY_LEASE_TTL must be > 0")
		}
		cfg.RelayLeaseTTL = d
	}
	if value := os.Getenv("LLM_AGENT_MEMORY_WORKER_RELAY_MAX_ATTEMPTS"); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil {
			return Config{}, fmt.Errorf("parse LLM_AGENT_MEMORY_WORKER_RELAY_MAX_ATTEMPTS: %w", err)
		}
		if n <= 0 {
			return Config{}, errors.New("LLM_AGENT_MEMORY_WORKER_RELAY_MAX_ATTEMPTS must be > 0")
		}
		cfg.RelayMaxAttempts = n
	}
	if value := os.Getenv("LLM_AGENT_MEMORY_WORKER_RELAY_BATCH_SIZE"); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil {
			return Config{}, fmt.Errorf("parse LLM_AGENT_MEMORY_WORKER_RELAY_BATCH_SIZE: %w", err)
		}
		if n <= 0 {
			return Config{}, errors.New("LLM_AGENT_MEMORY_WORKER_RELAY_BATCH_SIZE must be > 0")
		}
		cfg.RelayBatchSize = n
	}

	return cfg, nil
}
