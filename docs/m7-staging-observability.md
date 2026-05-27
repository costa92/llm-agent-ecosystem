# M7 Staging Observability Queries

> Date: 2026-05-27
> Status: companion to the M7 ship. Use while M7 runs in staging/production to gather the data that informs M8 prioritization (see `docs/superpowers/specs/2026-05-27-m7-workers-and-validation-design.md` §4 deferred-to-M8 table).

The queries below are tied to the 10 validation counters + `trace_dropped_total` + `storage_cron_failures_total` exposed by `llm-agent-memory-gateway/internal/observability/metrics.go`, and the `memory_decision_trace` table migrated by `llm-agent-memory-postgres`.

Counter naming reminder:
- All 10 validation counters carry only `tenant_bucket` as label.
- `trace_dropped_total` carries only `reason` (`buffer_full` / `db_error` / `shutdown`).
- `storage_cron_failures_total` has no label.

---

## PromQL — for Grafana / Prometheus / VictoriaMetrics

### Q1. Embedding cost trend per tenant bucket (decides: M8 cost-attribution priority)

```promql
# Cost rate (micros/sec) per tenant bucket, last 1h
sum by (tenant_bucket) (
  rate(embedding_cost_total[5m])
)

# Total spend per bucket over 24h, in dollars (assumes micros = USD * 1e6)
sum by (tenant_bucket) (
  increase(embedding_cost_total[24h])
) / 1e6
```

**M8 trigger:** if any bucket consistently > $X/hour (set X per budget), Class B promote-tracking + per-tenant attribution rises in priority.

### Q2. Memory storage growth (decides: Working-tier + auto-forget urgency)

```promql
# Current bytes per bucket
max by (tenant_bucket) (memory_storage_bytes_total)

# Day-over-day growth rate (bytes/day)
delta(memory_storage_bytes_total[24h])

# Top 5 buckets by absolute size
topk(5, max by (tenant_bucket) (memory_storage_bytes_total))
```

**M8 trigger:** if any bucket crosses ~1GB or growth rate is > 100MB/day, Working-tier introduction + lifecycle policies become P0 not P1.

### Q3. Lifecycle activity — are users disabling/deleting at all?

```promql
# Disabled/deleted events per hour, last 24h
sum(rate(episodic_disabled_total[5m]))
sum(rate(episodic_deleted_total[5m]))

# By tenant bucket
sum by (tenant_bucket) (increase(episodic_disabled_total[24h]))
sum by (tenant_bucket) (increase(episodic_deleted_total[24h]))
```

**M8 trigger:** if both stay at 0 for ≥1 week of real traffic, users aren't curating memory at all → Consolidation Worker + auto-promotion priority **drops** (no manual control means automation needs to be cautious, not aggressive). If non-zero, the inverse: users care, automation should match their patterns.

### Q4. Trace pipeline health (decides: M8 sink design — async vs sync)

```promql
# Any drops at all?
sum by (reason) (rate(trace_dropped_total[5m]))

# Drop ratio: dropped per total emit (no direct emit counter, approximate via recall path)
sum(rate(trace_dropped_total[5m])) / sum(rate(recall_returned_total[5m]))
```

**M8 trigger:**
- `reason="buffer_full"` non-zero → increase `TraceSinkBufferSize` or move to sync writes with backpressure (revisit spec §5.2 + codex C-6).
- `reason="db_error"` non-zero → trace table contention; partition the table or move to a separate database.
- `reason="shutdown"` non-zero on most shutdowns → graceful drain budget too short; bump `TraceSinkShutdownTimeout`.

### Q5. Recall budget pressure (decides: typed RecallObserver ROI)

```promql
# Selection ratio per bucket — what fraction of recalled hits survive budget
sum by (tenant_bucket) (rate(recall_selected_total[5m])) /
sum by (tenant_bucket) (rate(recall_returned_total[5m]))

# Absolute drops per bucket per hour
sum by (tenant_bucket) (
  rate(recall_returned_total[5m]) - rate(recall_selected_total[5m])
) * 3600
```

**M8 trigger:** if selection ratio < 0.5 for any active bucket, token-budget is the bottleneck → reason-bucketed `recall_dropped_total` (deferred to M8) becomes high-value; typed RecallObserver + memory_id/request_id correlation jumps in priority.

### Q6. Storage cron health

```promql
# Failures per hour
sum(rate(storage_cron_failures_total[5m])) * 3600
```

**Trigger:** any non-zero means the periodic aggregate query is failing — check Postgres load + `LLM_AGENT_MEMORY_GATEWAY_STORAGE_INTERVAL`.

---

## SQL — `memory_decision_trace` direct queries

Run via `psql $LLM_AGENT_MEMORY_PG_URL`. Replace `<prefix>memory_decision_trace` with the actual prefixed table name (`memory_decision_trace` for empty TablePrefix, otherwise check `Config.TablePrefix`).

### S1. Reason value distribution (decides: when to freeze §13.4 enum in M8)

```sql
-- Top 20 reason strings in last 24h, with counts and stage breakdown
SELECT
  reason,
  stage,
  COUNT(*) AS rows
FROM memory_decision_trace
WHERE emitted_at >= NOW() - INTERVAL '24 hours'
GROUP BY reason, stage
ORDER BY rows DESC
LIMIT 20;
```

**M8 trigger:** the reason column is free-form in M7. Use this to catalog the actual values gateway emits before freezing the §13.4 enum. If you see typos or near-duplicates ("deferred" vs "deferred_promotion"), spec the canonical names from the data.

### S2. Stage flow per request (validates: trace pipeline coverage)

```sql
-- For a sample request_id, all stages in order
SELECT emitted_at, stage, reason, payload
FROM memory_decision_trace
WHERE request_id = '<paste-a-real-request-id>'
ORDER BY emitted_at;

-- Coverage check: which stages appear at all?
SELECT stage, COUNT(*) AS rows
FROM memory_decision_trace
WHERE emitted_at >= NOW() - INTERVAL '1 hour'
GROUP BY stage;
```

**Expected stages:** `recalled`, `selected`, `dropped`, `promote_decided`. Missing stages signal a gateway code path that doesn't yet emit traces.

### S3. Trace table growth (operator hygiene)

```sql
-- Approximate size
SELECT
  pg_size_pretty(pg_total_relation_size('memory_decision_trace')) AS total_size,
  pg_size_pretty(pg_relation_size('memory_decision_trace')) AS table_size,
  pg_size_pretty(pg_indexes_size('memory_decision_trace')) AS indexes_size,
  (SELECT COUNT(*) FROM memory_decision_trace) AS row_count;

-- Daily row volume
SELECT
  date_trunc('day', emitted_at) AS day,
  COUNT(*) AS rows
FROM memory_decision_trace
GROUP BY day
ORDER BY day DESC
LIMIT 14;
```

**Action:** `TraceRetentionEnabled` config flag exists but isn't wired (spec OD-5 / plan). When the table crosses your storage budget, run a manual retention:

```sql
-- Delete rows older than 30 days (adjust window per ops policy)
DELETE FROM memory_decision_trace
WHERE emitted_at < NOW() - INTERVAL '30 days';
```

Wire automation in M8 if manual becomes a chore.

### S4. Cross-tenant sanity check (privacy guarantee verification)

```sql
-- Any rows missing tenant_id? (Should be zero — DAL enforces NOT NULL)
SELECT COUNT(*) FROM memory_decision_trace WHERE tenant_id = '';

-- Tenant distribution (rough cardinality check)
SELECT tenant_id, COUNT(*) AS rows
FROM memory_decision_trace
WHERE emitted_at >= NOW() - INTERVAL '24 hours'
GROUP BY tenant_id
ORDER BY rows DESC
LIMIT 50;
```

**Expected:** every row has a non-empty `tenant_id`. The PromQL counters only show `tenant_bucket` (32 buckets); this SQL is the only way to see actual tenant_id distribution in production.

### S5. Reason → metric correlation spot-check

```sql
-- For 'dropped' stage, see which reasons appear — these inform the reason-bucketed
-- recall_dropped_total counter that's deferred to M8.
SELECT
  reason,
  COUNT(*) AS rows,
  COUNT(DISTINCT tenant_id) AS tenants
FROM memory_decision_trace
WHERE stage = 'dropped'
  AND emitted_at >= NOW() - INTERVAL '7 days'
GROUP BY reason
ORDER BY rows DESC;
```

**Use:** the dominant `dropped` reasons in your real traffic dictate the M8 reason enum's first-class values. Long-tail reasons can be merged into "other" in the frozen enum.

---

## Decision matrix — which queries inform which M8 questions

| M8 decision (per spec §4 / roadmap) | Primary signal | Source |
|---|---|---|
| Working-tier introduction urgency | Memory growth + storage cost | Q2 PromQL |
| Atomic promotion API priority | Lifecycle activity volume | Q3 PromQL |
| Consolidation Worker priority | Lifecycle activity + recall budget pressure | Q3, Q5 PromQL |
| Typed RecallObserver ROI | Recall budget pressure | Q5 PromQL |
| Reason-enum freeze contents | Actual emitted reasons | S1, S5 SQL |
| Trace sink async-vs-sync revisit | Drop counter | Q4 PromQL |
| `LastAccessAt` write path priority | Recall budget pressure | Q5 PromQL |
| Relay delivery hardening (codex C-1) | Not directly observable in M7 | — (need M8 worker telemetry first) |
| `vector_storage_bytes_total` real source | Not measured in M7 (returns 0) | — |

---

## Known limitations of this observability set (be honest with stakeholders)

1. **`vector_storage_bytes_total = 0`** — embeddings live in the RAG store, not this Postgres. Don't trend it; it's a placeholder until M8 wires the real source.
2. **`embedding_*` token counts are word-count proxies** — `len(strings.Fields(text))`, not real tokenizer counts. Calibrate `EmbeddingCostMicrosPerToken` against your actual provider invoice for accurate cost.
3. **Reason values are free-form** — S1 will catalog them; don't dashboard on specific values until M8 freezes the enum.
4. **No `request_id` in PromQL counters** — by design (cardinality). Use S2 SQL for per-request investigation.
5. **`promote_*` and `working_*` counters are missing entirely** — deferred to M8. Don't build dashboards expecting them.
