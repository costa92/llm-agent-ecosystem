package service

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

type MetricsSnapshot struct {
	PromoteAttemptTotal  map[string]uint64
	PromoteAcceptedTotal map[string]uint64
	PromoteRejectedTotal map[string]uint64
	WorkingPromotedTotal map[string]uint64
}

type Metrics struct {
	mu              sync.RWMutex
	promoteAttempt  map[string]*atomic.Uint64
	promoteAccepted map[string]*atomic.Uint64
	promoteRejected map[string]*atomic.Uint64
	workingPromoted map[string]*atomic.Uint64
}

func NewMetrics() *Metrics {
	return &Metrics{
		promoteAttempt:  make(map[string]*atomic.Uint64),
		promoteAccepted: make(map[string]*atomic.Uint64),
		promoteRejected: make(map[string]*atomic.Uint64),
		workingPromoted: make(map[string]*atomic.Uint64),
	}
}

func (m *Metrics) RecordPromoteAttempt(tenantID string)  { m.addBucket(m.promoteAttempt, TenantBucket(tenantID), 1) }
func (m *Metrics) RecordPromoteAccepted(tenantID string) { m.addBucket(m.promoteAccepted, TenantBucket(tenantID), 1) }
func (m *Metrics) RecordPromoteRejected(tenantID string) { m.addBucket(m.promoteRejected, TenantBucket(tenantID), 1) }
func (m *Metrics) RecordWorkingPromoted(tenantID string) { m.addBucket(m.workingPromoted, TenantBucket(tenantID), 1) }

func (m *Metrics) Snapshot() MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return MetricsSnapshot{
		PromoteAttemptTotal:  copyBuckets(m.promoteAttempt),
		PromoteAcceptedTotal: copyBuckets(m.promoteAccepted),
		PromoteRejectedTotal: copyBuckets(m.promoteRejected),
		WorkingPromotedTotal: copyBuckets(m.workingPromoted),
	}
}

func (m *Metrics) Render() string {
	snap := m.Snapshot()
	lines := make([]string, 0, 8)
	lines = appendBucketLines(lines, "promote_attempt_total", snap.PromoteAttemptTotal)
	lines = appendBucketLines(lines, "promote_accepted_total", snap.PromoteAcceptedTotal)
	lines = appendBucketLines(lines, "promote_rejected_total", snap.PromoteRejectedTotal)
	lines = appendBucketLines(lines, "working_promoted_total", snap.WorkingPromotedTotal)
	return strings.Join(lines, "\n")
}

func (m *Metrics) addBucket(buckets map[string]*atomic.Uint64, key string, delta uint64) {
	m.mu.RLock()
	v, ok := buckets[key]
	m.mu.RUnlock()
	if ok {
		v.Add(delta)
		return
	}
	m.mu.Lock()
	if v, ok = buckets[key]; !ok {
		v = new(atomic.Uint64)
		buckets[key] = v
	}
	m.mu.Unlock()
	v.Add(delta)
}

func copyBuckets(buckets map[string]*atomic.Uint64) map[string]uint64 {
	out := make(map[string]uint64, len(buckets))
	for k, v := range buckets {
		out[k] = v.Load()
	}
	return out
}

func appendBucketLines(lines []string, name string, buckets map[string]uint64) []string {
	if len(buckets) == 0 {
		return lines
	}
	keys := make([]string, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		lines = append(lines, fmt.Sprintf(`%s{tenant_bucket=%q} %d`, name, k, buckets[k]))
	}
	return lines
}

func TenantBucket(tenantID string) string {
	if tenantID == "" {
		return "00"
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(tenantID))
	return fmt.Sprintf("%02d", h.Sum32()%32)
}
