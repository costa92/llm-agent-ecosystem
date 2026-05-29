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

	pgmemory "github.com/costa92/llm-agent-memory-postgres/postgres"
)

type fakeRelayRunner struct {
	stats []pgmemory.RunStats
	err   error
	calls int
}

func (f *fakeRelayRunner) RunOnce(context.Context) (pgmemory.RunStats, error) {
	f.calls++
	if f.err != nil {
		return pgmemory.RunStats{}, f.err
	}
	if len(f.stats) == 0 {
		return pgmemory.RunStats{}, nil
	}
	idx := f.calls - 1
	if idx >= len(f.stats) {
		idx = len(f.stats) - 1
	}
	return f.stats[idx], nil
}

func TestRunRelayLoop_LogsNonZeroStats(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := slog.New(slog.NewTextHandler(buf, nil))
	runner := &fakeRelayRunner{stats: []pgmemory.RunStats{{Published: 1, Failed: 2, LeaseLost: 3}}}
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- runRelayLoop(ctx, logger, 5*time.Millisecond, runner)
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()
	err := <-done
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	out := buf.String()
	if out == "" || !containsAll(out, "memory worker relay tick", "published=1", "failed=2", "lease_lost=3") {
		t.Fatalf("log output = %q", out)
	}
}

func TestRunRelayLoop_ReturnsRunnerError(t *testing.T) {
	wantErr := errors.New("boom")
	err := runRelayLoop(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)), time.Millisecond, &fakeRelayRunner{err: wantErr})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
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
