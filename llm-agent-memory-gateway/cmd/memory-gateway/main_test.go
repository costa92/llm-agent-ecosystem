package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/costa92/llm-agent-memory-gateway/internal/config"
	pgmemory "github.com/costa92/llm-agent-memory-postgres/postgres"
	corememory "github.com/costa92/llm-agent-memory/memory"
	ragpg "github.com/costa92/llm-agent-rag/postgres"
)

type fakeMigrator struct {
	calls int
	err   error
}

func (f *fakeMigrator) Migrate(context.Context) error {
	f.calls++
	return f.err
}

func TestCommandPackageCompiles(t *testing.T) {}

func TestRunMigrations_InvokesMigrator(t *testing.T) {
	migrator := &fakeMigrator{}

	if err := runMigrations(context.Background(), migrator); err != nil {
		t.Fatalf("runMigrations() error = %v", err)
	}
	if migrator.calls != 1 {
		t.Fatalf("calls = %d, want 1", migrator.calls)
	}
}

func TestRunMigrations_PropagatesError(t *testing.T) {
	migrator := &fakeMigrator{err: errors.New("boom")}

	if err := runMigrations(context.Background(), migrator); err == nil {
		t.Fatal("expected error")
	}
}

type fakeRecordStore struct{}

func (fakeRecordStore) GetRecord(context.Context, string, string) (corememory.MemoryRecord, error) {
	return corememory.MemoryRecord{}, nil
}

func (fakeRecordStore) WriteRecord(context.Context, corememory.WriteRecordInput) (corememory.WriteRecordResult, error) {
	return corememory.WriteRecordResult{}, nil
}

func (fakeRecordStore) PatchRecord(context.Context, corememory.PatchRecordInput) (corememory.PatchRecordResult, error) {
	return corememory.PatchRecordResult{}, nil
}

func (fakeRecordStore) DeleteRecord(context.Context, corememory.DeleteRecordInput) (corememory.DeleteRecordResult, error) {
	return corememory.DeleteRecordResult{}, nil
}

func (fakeRecordStore) PinRecord(context.Context, corememory.PinRecordInput) (corememory.PinRecordResult, error) {
	return corememory.PinRecordResult{}, nil
}

func (fakeRecordStore) DisableRecord(context.Context, corememory.DisableRecordInput) (corememory.DisableRecordResult, error) {
	return corememory.DisableRecordResult{}, nil
}

func TestBuildRecallBackend_SupportsConfiguredModes(t *testing.T) {
	for _, mode := range []string{"lexical", "hybrid"} {
		t.Run(mode, func(t *testing.T) {
			backend := buildRecallBackend(nil, fakeRecordStore{}, config.Config{RecallMode: mode}, nil)
			if backend == nil {
				t.Fatal("backend is nil")
			}
		})
	}
}

func TestBuildVectorCandidateSource_DisabledReturnsNullSource(t *testing.T) {
	source, cleanup, err := buildVectorCandidateSource(context.Background(), config.Config{VectorEnabled: false}, nil)
	if err != nil {
		t.Fatalf("buildVectorCandidateSource() error = %v", err)
	}
	if source == nil {
		t.Fatal("source is nil")
	}
	if cleanup != nil {
		t.Fatal("cleanup should be nil when vector is disabled")
	}
}

func TestParseVectorIndex(t *testing.T) {
	if got := parseVectorIndex("none"); got != ragpg.VectorIndexNone {
		t.Fatalf("parseVectorIndex(none) = %v", got)
	}
	if got := parseVectorIndex("ivfflat"); got != ragpg.VectorIndexIVFFlat {
		t.Fatalf("parseVectorIndex(ivfflat) = %v", got)
	}
	if got := parseVectorIndex("hnsw"); got != ragpg.VectorIndexHNSW {
		t.Fatalf("parseVectorIndex(hnsw) = %v", got)
	}
}

type fakeRelayRunner struct {
	calls int
	stats pgmemory.RunStats
	err   error
}

func (f *fakeRelayRunner) RunOnce(context.Context) (pgmemory.RunStats, error) {
	f.calls++
	return f.stats, f.err
}

func TestStartOutboxRelayWorker_RunsRelayUntilCanceled(t *testing.T) {
	runner := &fakeRelayRunner{}
	ctx, cancel := context.WithCancel(context.Background())
	stop := startOutboxRelayWorker(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)), 5*time.Millisecond, runner)

	time.Sleep(20 * time.Millisecond)
	cancel()
	stop()

	if runner.calls == 0 {
		t.Fatal("expected relay worker to call RunOnce at least once")
	}
}

func TestStartOutboxRelayWorker_LogsRelayStats(t *testing.T) {
	buf := &bytes.Buffer{}
	runner := &fakeRelayRunner{stats: pgmemory.RunStats{Published: 2, Failed: 1}}
	ctx, cancel := context.WithCancel(context.Background())
	stop := startOutboxRelayWorker(ctx, slog.New(slog.NewTextHandler(buf, nil)), 5*time.Millisecond, runner)

	time.Sleep(10 * time.Millisecond)
	cancel()
	stop()

	if got := buf.String(); got == "" || !containsAll(got, "outbox relay tick", "published=2", "failed=1") {
		t.Fatalf("log output = %q", got)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(s, part) {
			return false
		}
	}
	return true
}
