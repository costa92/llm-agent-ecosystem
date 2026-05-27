package observability

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/costa92/llm-agent-memory-gateway/internal/service"
)

type Snapshot struct {
	RecallL1HitTotal               int64
	RecallL2HitTotal               int64
	RecallOriginTotal              int64
	RecallStaleServedTotal         int64
	RecallCacheFillTotal           int64
	RecallInvalidationTotal        int64
	OutboxProjectionProjectedTotal int64
	OutboxProjectionStaleTotal     int64
	OutboxProjectionFailedTotal    int64
	OutboxProjectionIgnoredTotal   int64
}

type Metrics struct {
	recallL1Hit        atomic.Int64
	recallL2Hit        atomic.Int64
	recallOrigin       atomic.Int64
	recallStaleServed  atomic.Int64
	recallCacheFill    atomic.Int64
	recallInvalidation atomic.Int64

	outboxProjected atomic.Int64
	outboxStale     atomic.Int64
	outboxFailed    atomic.Int64
	outboxIgnored   atomic.Int64
}

func NewMetrics() *Metrics { return &Metrics{} }

func (m *Metrics) AddRecallL1Hit()        { m.recallL1Hit.Add(1) }
func (m *Metrics) AddRecallL2Hit()        { m.recallL2Hit.Add(1) }
func (m *Metrics) AddRecallOrigin()       { m.recallOrigin.Add(1) }
func (m *Metrics) AddRecallStaleServed()  { m.recallStaleServed.Add(1) }
func (m *Metrics) AddRecallCacheFill()    { m.recallCacheFill.Add(1) }
func (m *Metrics) AddRecallInvalidation() { m.recallInvalidation.Add(1) }
func (m *Metrics) AddOutboxProjected()    { m.outboxProjected.Add(1) }
func (m *Metrics) AddOutboxStale()        { m.outboxStale.Add(1) }
func (m *Metrics) AddOutboxFailed()       { m.outboxFailed.Add(1) }
func (m *Metrics) AddOutboxIgnored()      { m.outboxIgnored.Add(1) }

func (m *Metrics) Snapshot() Snapshot {
	return Snapshot{
		RecallL1HitTotal:               m.recallL1Hit.Load(),
		RecallL2HitTotal:               m.recallL2Hit.Load(),
		RecallOriginTotal:              m.recallOrigin.Load(),
		RecallStaleServedTotal:         m.recallStaleServed.Load(),
		RecallCacheFillTotal:           m.recallCacheFill.Load(),
		RecallInvalidationTotal:        m.recallInvalidation.Load(),
		OutboxProjectionProjectedTotal: m.outboxProjected.Load(),
		OutboxProjectionStaleTotal:     m.outboxStale.Load(),
		OutboxProjectionFailedTotal:    m.outboxFailed.Load(),
		OutboxProjectionIgnoredTotal:   m.outboxIgnored.Load(),
	}
}

func (m *Metrics) TraceEmitter() service.TraceEmitter {
	return traceMetricsEmitter{metrics: m}
}

func (m *Metrics) RecallObserver() service.RecallObserver {
	return recallMetricsObserver{metrics: m}
}

func (m *Metrics) RecallCacheObserver() service.RecallCacheObserver {
	return recallCacheMetricsObserver{metrics: m}
}

func (m *Metrics) OutboxObserver() service.OutboxProjectionObserver {
	return outboxMetricsObserver{metrics: m}
}

func (m *Metrics) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		snap := m.Snapshot()
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = fmt.Fprint(w, strings.Join([]string{
			fmt.Sprintf("recall_l1_hit_total %d", snap.RecallL1HitTotal),
			fmt.Sprintf("recall_l2_hit_total %d", snap.RecallL2HitTotal),
			fmt.Sprintf("recall_origin_total %d", snap.RecallOriginTotal),
			fmt.Sprintf("recall_stale_served_total %d", snap.RecallStaleServedTotal),
			fmt.Sprintf("recall_cache_fill_total %d", snap.RecallCacheFillTotal),
			fmt.Sprintf("recall_invalidation_total %d", snap.RecallInvalidationTotal),
			fmt.Sprintf("outbox_projection_projected_total %d", snap.OutboxProjectionProjectedTotal),
			fmt.Sprintf("outbox_projection_stale_total %d", snap.OutboxProjectionStaleTotal),
			fmt.Sprintf("outbox_projection_failed_total %d", snap.OutboxProjectionFailedTotal),
			fmt.Sprintf("outbox_projection_ignored_total %d", snap.OutboxProjectionIgnoredTotal),
		}, "\n"))
	})
}

type traceMetricsEmitter struct {
	metrics *Metrics
}

func (e traceMetricsEmitter) Emit(_ context.Context, stage string, fields map[string]any) {
	if e.metrics == nil {
		return
	}
	_ = stage
	_ = fields
}

type recallMetricsObserver struct {
	metrics *Metrics
}

func (o recallMetricsObserver) ObserveRecall(_ context.Context, obs service.RecallObservation) {
	if o.metrics == nil {
		return
	}
	switch obs.CacheLevel {
	case "l1_hit":
		o.metrics.AddRecallL1Hit()
	case "l2_hit":
		o.metrics.AddRecallL2Hit()
	case "origin":
		o.metrics.AddRecallOrigin()
	}
	if obs.StaleServed {
		o.metrics.AddRecallStaleServed()
	}
}

type outboxMetricsObserver struct {
	metrics *Metrics
}

type recallCacheMetricsObserver struct {
	metrics *Metrics
}

func (o recallCacheMetricsObserver) ObserveRecallCache(_ context.Context, obs service.RecallCacheObservation) {
	if o.metrics == nil {
		return
	}
	switch obs.Action {
	case "fill":
		o.metrics.AddRecallCacheFill()
	case "invalidate":
		o.metrics.AddRecallInvalidation()
	}
}

func (o outboxMetricsObserver) ObserveProjection(_ context.Context, obs service.OutboxProjectionObservation) {
	if o.metrics == nil {
		return
	}
	switch obs.Status {
	case "projected":
		o.metrics.AddOutboxProjected()
	case "stale":
		o.metrics.AddOutboxStale()
	case "failed":
		o.metrics.AddOutboxFailed()
	case "ignored":
		o.metrics.AddOutboxIgnored()
	}
}

type composedTraceEmitter struct {
	emitters []service.TraceEmitter
}

func ComposeTraceEmitters(emitters ...service.TraceEmitter) service.TraceEmitter {
	filtered := make([]service.TraceEmitter, 0, len(emitters))
	for _, emitter := range emitters {
		if emitter != nil {
			filtered = append(filtered, emitter)
		}
	}
	return composedTraceEmitter{emitters: filtered}
}

func (e composedTraceEmitter) Emit(ctx context.Context, stage string, fields map[string]any) {
	for _, emitter := range e.emitters {
		emitter.Emit(ctx, stage, fields)
	}
}
