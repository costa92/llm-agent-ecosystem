package config

import (
	"os"
	"testing"
	"time"
)

func TestLoadFromEnv_Defaults(t *testing.T) {
	t.Setenv("LLM_AGENT_MEMORY_WORKER_PG_URL", "postgres://worker")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}
	if cfg.PostgresURL != "postgres://worker" {
		t.Fatalf("PostgresURL = %q, want postgres://worker", cfg.PostgresURL)
	}
	if cfg.PollInterval != time.Second {
		t.Fatalf("PollInterval = %v, want 1s", cfg.PollInterval)
	}
	if cfg.RelayLeaseTTL != 180*time.Second {
		t.Fatalf("RelayLeaseTTL = %v, want 180s", cfg.RelayLeaseTTL)
	}
	if cfg.RelayMaxAttempts != 5 {
		t.Fatalf("RelayMaxAttempts = %d, want 5", cfg.RelayMaxAttempts)
	}
	if cfg.RelayBatchSize != 100 {
		t.Fatalf("RelayBatchSize = %d, want 100", cfg.RelayBatchSize)
	}
}

func TestLoadFromEnv_FallsBackToSharedPGEnv(t *testing.T) {
	t.Setenv("LLM_AGENT_MEMORY_PG_URL", "postgres://shared")
	os.Unsetenv("LLM_AGENT_MEMORY_WORKER_PG_URL")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}
	if cfg.PostgresURL != "postgres://shared" {
		t.Fatalf("PostgresURL = %q, want postgres://shared", cfg.PostgresURL)
	}
}

func TestLoadFromEnv_RejectsInvalidValues(t *testing.T) {
	t.Setenv("LLM_AGENT_MEMORY_WORKER_PG_URL", "postgres://worker")
	t.Setenv("LLM_AGENT_MEMORY_WORKER_POLL_INTERVAL", "0s")

	if _, err := LoadFromEnv(); err == nil {
		t.Fatal("expected error")
	}
}
