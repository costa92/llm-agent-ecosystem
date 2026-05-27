package observability

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/costa92/llm-agent-memory-gateway/internal/service"
)

func TestMetrics_ExposeCounters(t *testing.T) {
	m := NewMetrics()
	m.AddRecallOrigin()
	m.AddRecallL1Hit()
	m.AddRecallStaleServed()
	m.AddOutboxProjected()
	m.AddOutboxStale()
	m.AddOutboxFailed()
	m.AddOutboxIgnored()

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	m.Handler().ServeHTTP(recorder, req)

	body := recorder.Body.String()
	for _, want := range []string{
		"recall_origin_total 1",
		"recall_l1_hit_total 1",
		"recall_stale_served_total 1",
		"recall_cache_fill_total 0",
		"recall_invalidation_total 0",
		"outbox_projection_projected_total 1",
		"outbox_projection_stale_total 1",
		"outbox_projection_failed_total 1",
		"outbox_projection_ignored_total 1",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics output missing %q\n%s", want, body)
		}
	}
}

func TestMetrics_OutboxObserverCountsStatuses(t *testing.T) {
	m := NewMetrics()
	observer := m.OutboxObserver()
	observer.ObserveProjection(context.Background(), service.OutboxProjectionObservation{Status: "projected"})
	observer.ObserveProjection(context.Background(), service.OutboxProjectionObservation{Status: "stale"})
	observer.ObserveProjection(context.Background(), service.OutboxProjectionObservation{Status: "failed"})
	observer.ObserveProjection(context.Background(), service.OutboxProjectionObservation{Status: "ignored"})

	snap := m.Snapshot()
	if snap.OutboxProjectionProjectedTotal != 1 || snap.OutboxProjectionStaleTotal != 1 || snap.OutboxProjectionFailedTotal != 1 || snap.OutboxProjectionIgnoredTotal != 1 {
		t.Fatalf("snapshot = %+v", snap)
	}
}

func TestMetrics_RecallObserverCountsCacheLevels(t *testing.T) {
	m := NewMetrics()
	observer := m.RecallObserver()
	observer.ObserveRecall(context.Background(), service.RecallObservation{CacheLevel: "l1_hit", StaleServed: false})
	observer.ObserveRecall(context.Background(), service.RecallObservation{CacheLevel: "l1_hit", StaleServed: true})
	observer.ObserveRecall(context.Background(), service.RecallObservation{CacheLevel: "l2_hit", StaleServed: false})
	observer.ObserveRecall(context.Background(), service.RecallObservation{CacheLevel: "origin", StaleServed: false})

	snap := m.Snapshot()
	if snap.RecallL1HitTotal != 2 || snap.RecallL2HitTotal != 1 || snap.RecallOriginTotal != 1 || snap.RecallStaleServedTotal != 1 {
		t.Fatalf("snapshot = %+v", snap)
	}
}

func TestMetrics_RecallCacheObserverCountsLifecycleEvents(t *testing.T) {
	m := NewMetrics()
	observer := m.RecallCacheObserver()
	observer.ObserveRecallCache(context.Background(), service.RecallCacheObservation{Action: "fill"})
	observer.ObserveRecallCache(context.Background(), service.RecallCacheObservation{Action: "invalidate"})

	snap := m.Snapshot()
	if snap.RecallCacheFillTotal != 1 || snap.RecallInvalidationTotal != 1 {
		t.Fatalf("snapshot = %+v", snap)
	}
}
