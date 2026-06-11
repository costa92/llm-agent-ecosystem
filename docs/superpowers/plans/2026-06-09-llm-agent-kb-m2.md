# llm-agent-kb M2 (ingest expansion) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the shipped `llm-agent-kb` M1 backend (module published at **v0.1.0**, developed on `main` IN the kb repo) with the §13 **M2** scope: PDF / DOCX / URL parsing (with SSRF + upload validation, §16.3), an **asynchronous** Postgres-queue ingest worker (lease / backoff retry / stuck recovery / `dead` terminal / manual retry), per-document progress (status + phase, SSE), reimport/dedup via checksum short-circuit, and keyset-paginated document listing. M2 **replaces** M1's synchronous `Ingest` with an enqueue-then-worker flow: `POST /documents` now writes `document(pending)` + an `ingest_job` and returns **202 + documentId**; the worker drains the queue. The M1 §16.4 delete cascade and all M1 behavior (auth/ask/citations/kb CRUD) are preserved.

**Architecture:** Same single Go binary `kbd` (BFF + embedded `rag.System`). M2 grows the `ingest` package — split by responsibility into `ingest.go` (enqueue + makeDocument + checksum, retained), `worker.go` (the Postgres-queue worker pool: claim with `FOR UPDATE SKIP LOCKED` + `locked_until` lease, parse → Import → status transitions, backoff, stuck recovery, dead terminal), `parse.go` (the `source_type` dispatch, extended), `parse_pdf.go`, `parse_docx.go`, `parse_url.go` (the new parsers). A new **security-critical** package `internal/fetch` performs SSRF-safe outbound HTTP (scheme allowlist, DNS-resolve-then-validate-IP, dial-the-resolved-IP to defeat DNS rebinding, per-hop redirect re-validation, connect/read timeouts, max body, MIME allowlist). `internal/storage` gains the `ingest_job` table and a `document.phase` column. `internal/httpapi` gains `GET /documents` (paginated), the 202 enqueue path, `GET /documents/{docId}/progress` (SSE), and `POST /documents/{docId}/retry`. `cmd/kbd` starts the worker pool and drains it on shutdown. `ragsvc`/`orgkb`/`retrieval`/`limits`/`obs` are unchanged except where noted (orgkb gains nothing; the document-list keyset query lives in a small `ingest` repo method mirroring `orgkb.ListByOrg`).

Boundaries unchanged (spec §4): `ragsvc` is the ONLY package importing `rag`/`postgres`/`otelrag`. `ingest` depends only on `ragsvc.RagPort` + the pool + the new `internal/fetch`. `internal/fetch` depends on nothing kb-internal (pure stdlib + the readability/parser libs are NOT imported there — fetch returns raw bytes; the URL *parser* in `ingest/parse_url.go` owns readability). `httpapi` holds no business rules.

**Tech Stack:** Go 1.26.0 · `github.com/jackc/pgx/v5` v5.9.2 (pgxpool) · `github.com/costa92/llm-agent-authz v0.1.0` · `github.com/costa92/llm-agent-rag v1.11.0` · `github.com/costa92/llm-agent-contract v0.5.0` · `github.com/costa92/llm-agent-providers v0.7.0` · `github.com/costa92/llm-agent-otel v0.4.0` · **NEW** `github.com/ledongthuc/pdf v0.0.0-20250511090121-5959a4027728` (pure-Go PDF text extraction) · **NEW** `github.com/fumiama/go-docx v0.0.0-20250506085032-0c30fd09304b` (pure-Go OOXML/DOCX parse) · **NEW** `github.com/go-shiori/go-readability v0.0.0-20251205110129-5db1dc9836f0` (HTML→main-text extraction) · stdlib `net`, `net/http`, `net/url`, `crypto/sha256`, `context`, `time`.

**Spec:** `docs/superpowers/specs/2026-06-09-llm-agent-kb-design.md` (this plan implements its **M2**, §13). Scope authority: M2 list §13; async pipeline §6 (worker pool + Postgres queue `FOR UPDATE SKIP LOCKED`); SSRF/upload §16.3; data model §5 (`ingest_job` columns); endpoints §16.2 (documents list pagination, POST 202, progress SSE, retry). The M1 plan `docs/superpowers/plans/2026-06-09-llm-agent-kb-m1.md` is the style/gating/TDD-rhythm reference.

**Live base (read before editing — DO NOT reinvent M1):**
- `internal/ingest/ingest.go` — `SourceType` consts (markdown/txt/paste), `parse(st, raw) (string, error)` switch, `checksum(content) string` (returns `"sha256:"+hex`), `makeDocument(docID, kbID, st, title, content) ragingest.Document` (ID=SourceID=docID, Metadata{kb_id,source_type}, no doc_id), `Service{pool, rag}`, `New(pool, rag)`, `IngestInput{KBID,Namespace,Title,SourceType,Raw}`, `Result{DocumentID,Status,ChunkCount}`, the **synchronous** `Ingest` (M2 replaces this with `Enqueue`), `newID() string`.
- `internal/ingest/delete.go` — `DeleteDocument(ctx, namespace, documentID)` (List→RemoveGraphBySource→RemoveChunks→delete row) and `DeleteAllDocumentsForKB`. **M2 must keep these working with the new schema** (the document row delete is unchanged; add `ingest_job` rows are removed via `ON DELETE CASCADE`).
- `internal/storage/storage.go` — `businessMigrations` (knowledge_base + document, the **M1 document schema has NO `phase` column**), `Migrate` (rag Migrate → business migrations), `ensureVectorExtension` cold-start, `Open` with `AfterConnect=RegisterTypes`.
- `internal/httpapi/httpapi.go` — `NewMux`, the `Ingester` interface (M2 widens it), `chain(min, h)`, route table. `internal/httpapi/handlers.go` — `uploadHandler` (M1 sync, M2 makes it 202), `deleteDocHandler`, kb/org handlers, `maxUploadBytes = 10<<20`.
- `cmd/kbd/main.go` — `build(ctx, cfg)` assembly + graceful shutdown via `signal.NotifyContext` + `srv.Shutdown`. M2 starts/drains the worker here.
- `internal/config/config.go` — `Config`, `LoadFromLookup`, `envOr/envInt/envBool`. M2 adds worker + upload/SSRF fields.

**Verified external APIs (inspected at the pinned pseudo-versions in the on-disk module cache — DO NOT change these calls):**
- `ledongthuc/pdf`: `pdf.NewReader(f io.ReaderAt, size int64) (*pdf.Reader, error)`; `(*pdf.Reader).GetPlainText() (io.Reader, error)`. (Import path `github.com/ledongthuc/pdf`, package `pdf`.)
- `fumiama/go-docx`: `docx.Parse(reader io.ReaderAt, size int64) (*docx.Docx, error)`; `(*docx.Docx).Document.Body.Items []interface{}` whose elements are `*docx.Paragraph`; `(*docx.Paragraph).Children []interface{}` whose elements are `*docx.Run`; `(*docx.Run).Children []interface{}` whose elements are `*docx.Text`; `(*docx.Text).Text string`. (Import path `github.com/fumiama/go-docx`, package `docx`.)
- `go-shiori/go-readability`: `readability.FromReader(input io.Reader, pageURL *url.URL) (readability.Article, error)`; `Article{Title string; TextContent string; Content string; ...}`. We use `FromReader` (NOT `FromURL`) so `internal/fetch` owns the HTTP and SSRF guard. (Import path `github.com/go-shiori/go-readability`, package `readability`.)
- All three resolve via the Go proxy (verified `GOWORK=off go get …@<pseudo-version>` succeeds; replace-guard hook leaves non-`costa92` deps untouched).

**Conventions (carried from M1, re-verified):** Go commands need `GOWORK=off` (umbrella `go.work` excludes kb). DB-touching tests gate on `LLM_AGENT_KB_PG_URL` + `t.Skipf`. **Each gated test sets up its OWN fresh DB** (M1 lesson: gated tests are NOT co-runnable on a shared DB — drop the tables it owns at the top). Worker tests are **deterministic**: inject a clock and use short/zero leases + a `RunOnce`-style single-claim method; **no real `time.Sleep` in tests**. The live test DB MUST be pgvector-enabled, built from source into `postgres:16-alpine` (see "Gated test DB" below).

**Gated test DB (build pgvector from source, per constraints):**
```bash
docker run -d --name kb-m2-pg -e POSTGRES_PASSWORD=pw postgres:16-alpine
docker exec -u root kb-m2-pg sh -c 'apk add --no-cache build-base clang19 llvm19-dev git && cd /tmp && git clone --depth 1 --branch v0.8.0 https://github.com/pgvector/pgvector && cd pgvector && make OPTFLAGS="" install'
# connect via the container bridge IP:
IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' kb-m2-pg)
export LLM_AGENT_KB_PG_URL="postgres://postgres:pw@$IP:5432/postgres?sslmode=disable"
# teardown when done: docker rm -f kb-m2-pg
```
(`pgvector/pgvector:pg16` is also acceptable if reachable; the from-source build is the constraint-mandated fallback.)

---

## File Structure

```
llm-agent-kb/                          module github.com/costa92/llm-agent-kb (v0.1.0, M2 on main)
├── go.mod                                                    # + ledongthuc/pdf, fumiama/go-docx, go-shiori/go-readability
├── docker-compose.yml                                        # + otel collector service (M1 follow-up)
├── internal/
│   ├── config/config.go            (CHANGED)                 # + worker + upload/SSRF fields
│   │   config_test.go              (CHANGED)
│   ├── storage/storage.go          (CHANGED)                 # + ingest_job table + document.phase/content_bytes/content columns + indexes
│   │   storage_test.go             (CHANGED)
│   ├── fetch/fetch.go              (NEW)                     # SSRF-safe outbound HTTP
│   │   fetch_test.go               (NEW)                     # heavy IP-block matrix unit tests
│   ├── ingest/ingest.go            (CHANGED)                 # makeDocument/checksum kept; Ingest → Enqueue; + retry/list repo methods
│   │   ingest_test.go              (CHANGED)
│   │   parse.go                    (NEW, split from ingest)  # source_type dispatch (md/txt/paste + pdf/docx/url)
│   │   parse_test.go               (NEW)
│   │   parse_pdf.go                (NEW)
│   │   parse_pdf_test.go           (NEW)
│   │   parse_docx.go               (NEW)
│   │   parse_docx_test.go          (NEW)
│   │   parse_url.go                (NEW)
│   │   parse_url_test.go           (NEW)
│   │   worker.go                   (NEW)                     # Postgres-queue worker pool (claim/lease/retry/stuck/dead)
│   │   worker_test.go              (NEW)                     # deterministic (injected clock, RunOnce)
│   │   delete.go                   (unchanged)
│   ├── httpapi/httpapi.go          (CHANGED)                 # + GET /documents, progress SSE, retry routes; widen Ingester
│   │   handlers.go                 (CHANGED)                 # uploadHandler → 202 enqueue; + list/progress/retry handlers
│   │   httpapi_test.go             (CHANGED)
│   └── obs/obs.go                  (unchanged)
└── cmd/kbd/main.go                 (CHANGED)                 # start worker pool; drain on shutdown
    main_test.go                    (CHANGED)                 # e2e: async upload → poll ready → ask
```

---

## Task 1: Dependencies + config fields

**Files:**
- Change: `llm-agent-kb/go.mod`
- Change: `llm-agent-kb/internal/config/config.go`
- Change: `llm-agent-kb/internal/config/config_test.go`

Add the three parser deps and the M2 config knobs (worker pool size / poll interval / lease / max attempts / base backoff; upload max bytes / per-kb quota / parse timeout; fetch timeouts / max body).

- [ ] **Step 1: Add the new dependencies**

Run (from the kb repo root; M2 develops on `main` in the kb repo, so branch first):
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb && \
git checkout -b feat/m2-ingest-expansion && \
GOWORK=off go get \
  github.com/ledongthuc/pdf@v0.0.0-20250511090121-5959a4027728 \
  github.com/fumiama/go-docx@v0.0.0-20250506085032-0c30fd09304b \
  github.com/go-shiori/go-readability@v0.0.0-20251205110129-5db1dc9836f0
```
Expected: `go.mod` requires all three (plus their transitive deps: `go-shiori/dom`, `gogs/chardet`, `andybalholm/cascadia`, `araddon/dateparse`, `fumiama/imgsz`).

- [ ] **Step 2: Write the failing config test**

Edit `internal/config/config_test.go` — extend `TestLoadDefaults` and add a new test. Add these assertions to the END of `TestLoadDefaults` (before the closing brace):
```go
	if cfg.IngestWorkers != 2 {
		t.Fatalf("IngestWorkers=%d want 2", cfg.IngestWorkers)
	}
	if cfg.IngestPollInterval != 2*time.Second {
		t.Fatalf("IngestPollInterval=%v want 2s", cfg.IngestPollInterval)
	}
	if cfg.IngestLease != 60*time.Second {
		t.Fatalf("IngestLease=%v want 60s", cfg.IngestLease)
	}
	if cfg.IngestMaxAttempts != 5 {
		t.Fatalf("IngestMaxAttempts=%d want 5", cfg.IngestMaxAttempts)
	}
	if cfg.IngestBaseBackoff != 5*time.Second {
		t.Fatalf("IngestBaseBackoff=%v want 5s", cfg.IngestBaseBackoff)
	}
	if cfg.MaxUploadBytes != 10<<20 {
		t.Fatalf("MaxUploadBytes=%d want 10MiB", cfg.MaxUploadBytes)
	}
	if cfg.KBStorageQuotaBytes != 256<<20 {
		t.Fatalf("KBStorageQuotaBytes=%d want 256MiB", cfg.KBStorageQuotaBytes)
	}
	if cfg.ParseTimeout != 30*time.Second {
		t.Fatalf("ParseTimeout=%v want 30s", cfg.ParseTimeout)
	}
	if cfg.FetchTimeout != 15*time.Second {
		t.Fatalf("FetchTimeout=%v want 15s", cfg.FetchTimeout)
	}
	if cfg.FetchMaxBytes != 10<<20 {
		t.Fatalf("FetchMaxBytes=%d want 10MiB", cfg.FetchMaxBytes)
	}
```
Add `import "time"` to the test file if not present (it is needed now).

- [ ] **Step 3: Run test to verify it fails**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/config/ -run TestLoadDefaults -v`
Expected: FAIL to compile — `cfg.IngestWorkers` undefined (and the rest).

- [ ] **Step 4: Add the fields to `Config` and `LoadFromLookup`**

In `internal/config/config.go`, add to the `Config` struct (after the `MaxRequestsPerUserPerMinute` field):
```go
	// M2 ingest worker pool.
	IngestWorkers      int           // number of concurrent worker goroutines
	IngestPollInterval time.Duration // queue poll interval when idle
	IngestLease        time.Duration // job lease (locked_until = now + lease); stuck jobs reclaimed past it
	IngestMaxAttempts  int           // attempts before a job becomes 'dead'
	IngestBaseBackoff  time.Duration // base for exponential backoff (next_run_at = now + base*2^attempts)

	// M2 upload / parse safety (§16.3).
	MaxUploadBytes      int64         // http.MaxBytesReader cap per document body
	KBStorageQuotaBytes int64         // per-kb cumulative byte quota (sum of document content sizes)
	ParseTimeout        time.Duration // context deadline around PDF/DOCX parse (anti parse-bomb)

	// M2 SSRF-safe URL fetch (§16.3).
	FetchTimeout  time.Duration // connect+read deadline for outbound URL ingest
	FetchMaxBytes int64         // max response body bytes for outbound URL ingest
```
And in `LoadFromLookup`, add to the `cfg := Config{...}` literal (after `MaxRequestsPerUserPerMinute`):
```go
		IngestWorkers:       envInt(lookup, "INGEST_WORKERS", 2),
		IngestPollInterval:  time.Duration(envInt(lookup, "INGEST_POLL_INTERVAL_SECONDS", 2)) * time.Second,
		IngestLease:         time.Duration(envInt(lookup, "INGEST_LEASE_SECONDS", 60)) * time.Second,
		IngestMaxAttempts:   envInt(lookup, "INGEST_MAX_ATTEMPTS", 5),
		IngestBaseBackoff:   time.Duration(envInt(lookup, "INGEST_BASE_BACKOFF_SECONDS", 5)) * time.Second,
		MaxUploadBytes:      int64(envInt(lookup, "MAX_UPLOAD_BYTES", 10<<20)),
		KBStorageQuotaBytes: int64(envInt(lookup, "KB_STORAGE_QUOTA_BYTES", 256<<20)),
		ParseTimeout:        time.Duration(envInt(lookup, "PARSE_TIMEOUT_SECONDS", 30)) * time.Second,
		FetchTimeout:        time.Duration(envInt(lookup, "FETCH_TIMEOUT_SECONDS", 15)) * time.Second,
		FetchMaxBytes:       int64(envInt(lookup, "FETCH_MAX_BYTES", 10<<20)),
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/config/ -v`
Expected: PASS (all config tests).

- [ ] **Step 6: Commit**

```bash
cd llm-agent-kb && git add go.mod go.sum internal/config/ && \
git commit -m "feat(config): M2 ingest-worker + upload/parse + SSRF-fetch config knobs; pin pdf/docx/readability deps"
```

---

## Task 2: storage — `ingest_job` table + `document.phase` column

**Files:**
- Change: `llm-agent-kb/internal/storage/storage.go`
- Change: `llm-agent-kb/internal/storage/storage_test.go`

Per spec §5 `ingest_job(id, document_id, state, attempts, next_run_at, locked_by, locked_until, idempotency_key, last_error, phase, updated_at)`. Add `phase`, `content_bytes`, and `content BYTEA` columns to `document` (M1 lacked all three). `content` holds the raw uploaded bytes so the async worker can re-parse after the HTTP request returns — load-bearing for `worker.loadRaw` (Task 8). `ingest_job.document_id REFERENCES document(id) ON DELETE CASCADE` so the §16.4 delete-document cascade auto-removes the job row. `idempotency_key` is `UNIQUE` so an enqueue is naturally deduplicated.

- [ ] **Step 1: Write the failing test**

Add to `internal/storage/storage_test.go` (the gated `openTestStorage` helper already drops/migrates; extend the drop list and add a test). First, in `openTestStorage`, add `"ingest_job"` to the FRONT of the `DROP TABLE` slice (before `"document"`), so it reads:
```go
	for _, tbl := range []string{"ingest_job", "document", "knowledge_base", "chunks", "chunks_entities", "chunks_relations", "chunks_communities", "chunks_community_reports"} {
		_, _ = st.Pool().Exec(ctx, "DROP TABLE IF EXISTS "+tbl+" CASCADE")
	}
```
Then add:
```go
func TestMigrateCreatesIngestJobAndPhase(t *testing.T) {
	ctx := context.Background()
	st := openTestStorage(t, ctx)
	// ingest_job table is queryable with the §5 columns.
	if _, err := st.Pool().Exec(ctx,
		`INSERT INTO ingest_job (id, document_id, state, idempotency_key)
		 SELECT 'j1', NULL, 'pending', 'k1' WHERE false`); err != nil {
		t.Fatalf("ingest_job columns not as expected: %v", err)
	}
	var n int
	if err := st.Pool().QueryRow(ctx, `SELECT count(*) FROM ingest_job`).Scan(&n); err != nil {
		t.Fatalf("ingest_job not queryable: %v", err)
	}
	// document.phase column exists.
	if err := st.Pool().QueryRow(ctx, `SELECT count(phase) FROM document`).Scan(&n); err != nil {
		t.Fatalf("document.phase missing: %v", err)
	}
	// document.content (BYTEA) column exists — the async worker's loadRaw reads it.
	if err := st.Pool().QueryRow(ctx, `SELECT count(content) FROM document`).Scan(&n); err != nil {
		t.Fatalf("document.content missing: %v", err)
	}
	// idempotency_key is UNIQUE.
	var con int
	if err := st.Pool().QueryRow(ctx,
		`SELECT count(*) FROM pg_indexes WHERE tablename='ingest_job' AND indexdef ILIKE '%idempotency_key%'`).Scan(&con); err != nil {
		t.Fatalf("index introspection: %v", err)
	}
	if con == 0 {
		t.Fatal("ingest_job.idempotency_key has no unique index")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run (with the gated DB up):
```bash
cd llm-agent-kb && GOWORK=off go test ./internal/storage/ -run TestMigrateCreatesIngestJobAndPhase -v
```
Expected: FAIL — `column "phase" does not exist` / `relation "ingest_job" does not exist`.

- [ ] **Step 3: Add the migration statements**

In `internal/storage/storage.go`, extend `businessMigrations`. Add a `phase` column to the `document` table literal (after the `status` line):
```go
			status       TEXT NOT NULL DEFAULT 'pending',
			phase        TEXT NOT NULL DEFAULT '',
```
Then append these statements to the `businessMigrations` slice (after the `document_kb_idx` index):
```go
	`CREATE TABLE IF NOT EXISTS ingest_job (
		id              TEXT PRIMARY KEY,
		document_id     TEXT REFERENCES document(id) ON DELETE CASCADE,
		state           TEXT NOT NULL DEFAULT 'pending',
		attempts        INT  NOT NULL DEFAULT 0,
		next_run_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
		locked_by       TEXT NOT NULL DEFAULT '',
		locked_until    TIMESTAMPTZ,
		idempotency_key TEXT NOT NULL,
		last_error      TEXT NOT NULL DEFAULT '',
		phase           TEXT NOT NULL DEFAULT '',
		updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS ingest_job_idem_idx ON ingest_job (idempotency_key)`,
	// Claim query orders by next_run_at among claimable rows; index it.
	`CREATE INDEX IF NOT EXISTS ingest_job_claim_idx ON ingest_job (state, next_run_at)`,
	// document_kb_idx already exists in M1; add columns for quota accounting and
	// for the async worker's raw-content re-read (loadRaw reads document.content).
	`ALTER TABLE document ADD COLUMN IF NOT EXISTS content_bytes BIGINT NOT NULL DEFAULT 0`,
	// content holds the raw uploaded bytes so the async worker can re-parse after
	// the HTTP request returns (paste/file in content; url sources leave it empty).
	// Load-bearing: worker.loadRaw's `SELECT content` (Task 8) requires this column.
	`ALTER TABLE document ADD COLUMN IF NOT EXISTS content BYTEA`,
```
> Note: `ALTER TABLE … ADD COLUMN IF NOT EXISTS phase` is NOT needed because the `CREATE TABLE … document` statement uses `IF NOT EXISTS` and adds `phase` inline only on a fresh DB; **for an existing M1 DB the inline column is skipped**. So ALSO append an explicit `ALTER` for `phase` to cover the upgrade-in-place path:
```go
	`ALTER TABLE document ADD COLUMN IF NOT EXISTS phase TEXT NOT NULL DEFAULT ''`,
```
(Place all three `ALTER` statements AFTER the `ingest_job` create so the table exists; they are idempotent.)

- [ ] **Step 4: Run test to verify it passes**

Run (gated DB up):
```bash
cd llm-agent-kb && GOWORK=off go test ./internal/storage/ -v
```
Expected: PASS (existing migrate tests + `TestMigrateCreatesIngestJobAndPhase`). Without the DB, SKIP.

- [ ] **Step 5: Commit**

```bash
cd llm-agent-kb && git add internal/storage/ && \
git commit -m "feat(storage): ingest_job lease-queue table + document.phase/content_bytes/content columns (idempotent ALTERs for in-place M1 upgrade)"
```

---

## Task 3: `internal/fetch` — SSRF-safe outbound HTTP (security-critical, heavy unit tests)

**Files:**
- Create: `llm-agent-kb/internal/fetch/fetch.go`
- Test: `llm-agent-kb/internal/fetch/fetch_test.go`

Per §16.3: scheme allowlist (`http`/`https`); resolve DNS, reject any resolved IP that is private / loopback / link-local (incl. `169.254.169.254` metadata) / multicast / unspecified; **dial the resolved IP directly** via a custom `DialContext` (defeats DNS rebinding — the IP we validated is the IP we connect to); re-validate **every** redirect hop; connect+read timeouts; max body bytes (`io.LimitReader`); response Content-Type allowlist. The IP-block matrix is tested with an **injectable resolver** so no real DNS/network is needed.

- [ ] **Step 1: Write the failing test**

Create `internal/fetch/fetch_test.go`:
```go
package fetch

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestIsBlockedIP is the SSRF IP-block matrix: every non-public address class
// must be rejected; ordinary public IPs allowed.
func TestIsBlockedIP(t *testing.T) {
	cases := []struct {
		ip      string
		blocked bool
	}{
		{"127.0.0.1", true},         // loopback v4
		{"::1", true},               // loopback v6
		{"10.0.0.5", true},          // private A
		{"172.16.0.1", true},        // private B
		{"192.168.1.1", true},       // private C
		{"169.254.169.254", true},   // link-local / cloud metadata
		{"fe80::1", true},           // link-local v6
		{"0.0.0.0", true},           // unspecified v4
		{"::", true},                // unspecified v6
		{"224.0.0.1", true},         // multicast v4
		{"ff02::1", true},           // multicast v6
		{"100.64.0.1", true},        // CGNAT (RFC 6598)
		{"fc00::1", true},           // unique-local v6
		{"8.8.8.8", false},          // public
		{"1.1.1.1", false},          // public
		{"2606:4700:4700::1111", false}, // public v6
	}
	for _, c := range cases {
		ip := net.ParseIP(c.ip)
		if ip == nil {
			t.Fatalf("bad test IP %q", c.ip)
		}
		if got := isBlockedIP(ip); got != c.blocked {
			t.Errorf("isBlockedIP(%s)=%v want %v", c.ip, got, c.blocked)
		}
	}
}

func TestFetchRejectsNonHTTPScheme(t *testing.T) {
	f := New(Config{Timeout: time.Second, MaxBytes: 1 << 20, AllowedContentTypes: []string{"text/html"}})
	for _, u := range []string{"ftp://x/y", "file:///etc/passwd", "gopher://x", "data:text/html,hi"} {
		if _, _, err := f.Get(context.Background(), u); err == nil {
			t.Errorf("scheme %q should be rejected", u)
		}
	}
}

// TestFetchRejectsPrivateResolution uses an injected resolver that maps the
// host to a private IP — the fetch must refuse BEFORE dialing.
func TestFetchRejectsPrivateResolution(t *testing.T) {
	f := New(Config{
		Timeout: time.Second, MaxBytes: 1 << 20, AllowedContentTypes: []string{"text/html"},
		resolve: func(ctx context.Context, host string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("10.1.2.3")}, nil
		},
	})
	if _, _, err := f.Get(context.Background(), "http://intranet.evil/"); err == nil {
		t.Fatal("private resolution must be rejected")
	} else if !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("err=%v want a 'blocked' SSRF error", err)
	}
}

// TestFetchAllowsPublicAndCapsBody starts a local server, but injects a resolver
// that returns the server's (loopback) IP AND a dialer override so the
// loopback-block does not trip — proving the happy path: status, MIME allowlist,
// body cap. We bypass isBlockedIP via the test-only allowLoopback flag.
func TestFetchAllowsPublicAndCapsBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(strings.Repeat("A", 100)))
	}))
	defer srv.Close()

	f := New(Config{
		Timeout: 2 * time.Second, MaxBytes: 10, AllowedContentTypes: []string{"text/html"},
		allowLoopback: true, // test-only: permit the httptest loopback server
	})
	body, ct, err := f.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("content-type=%q", ct)
	}
	if len(body) != 10 {
		t.Fatalf("body len=%d want 10 (MaxBytes cap)", len(body))
	}
}

func TestFetchRejectsDisallowedContentType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte("binary"))
	}))
	defer srv.Close()
	f := New(Config{Timeout: time.Second, MaxBytes: 1 << 20, AllowedContentTypes: []string{"text/html"}, allowLoopback: true})
	if _, _, err := f.Get(context.Background(), srv.URL); err == nil {
		t.Fatal("disallowed content-type must be rejected")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/fetch/ -v`
Expected: FAIL to compile — undefined `New`, `Config`, `isBlockedIP`, `(*Fetcher).Get`.

- [ ] **Step 3: Write the implementation**

Create `internal/fetch/fetch.go`:
```go
// Package fetch performs SSRF-safe outbound HTTP for URL document ingest
// (spec §16.3). It enforces a scheme allowlist (http/https), resolves DNS and
// rejects any non-public resolved IP, dials the resolved IP DIRECTLY to defeat
// DNS rebinding, re-validates every redirect hop, applies connect/read
// timeouts and a max body cap, and enforces a response Content-Type allowlist.
package fetch

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Config configures a Fetcher.
type Config struct {
	Timeout             time.Duration
	MaxBytes            int64
	AllowedContentTypes []string // matched as a prefix against the response media type

	// resolve overrides DNS resolution (tests inject a stub). nil → net.DefaultResolver.
	resolve func(ctx context.Context, host string) ([]net.IP, error)
	// allowLoopback permits loopback IPs (test-only; httptest servers are loopback).
	allowLoopback bool
}

// Fetcher fetches remote documents safely.
type Fetcher struct {
	cfg    Config
	client *http.Client
}

// New builds a Fetcher whose transport dials only validated, resolved IPs.
func New(cfg Config) *Fetcher {
	if cfg.resolve == nil {
		cfg.resolve = func(ctx context.Context, host string) ([]net.IP, error) {
			return net.DefaultResolver.LookupIP(ctx, "ip", host)
		}
	}
	f := &Fetcher{cfg: cfg}
	dialer := &net.Dialer{Timeout: cfg.Timeout}
	transport := &http.Transport{
		// DialContext receives the host:port the http client wants to reach.
		// We resolve the host ourselves, validate every candidate IP, and dial
		// the validated IP directly — so the IP we checked is the IP we connect
		// to (no TOCTOU / DNS-rebinding window).
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ip, err := f.resolveAndValidate(ctx, host)
			if err != nil {
				return nil, err
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		},
		TLSHandshakeTimeout:   cfg.Timeout,
		ResponseHeaderTimeout: cfg.Timeout,
		DisableKeepAlives:     true,
	}
	f.client = &http.Client{
		Timeout:   cfg.Timeout,
		Transport: transport,
		// Re-validate every redirect hop's scheme + host BEFORE following it.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("fetch: too many redirects")
			}
			if err := validateScheme(req.URL); err != nil {
				return err
			}
			// Resolution+IP validation happens again in DialContext on the new
			// connection; this rejects an obviously-bad scheme early.
			return nil
		},
	}
	return f
}

// Get fetches the URL and returns the (capped) body + response media type.
func (f *Fetcher) Get(ctx context.Context, rawURL string) ([]byte, string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, "", fmt.Errorf("fetch: parse url: %w", err)
	}
	if err := validateScheme(u); err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "llm-agent-kb/url-ingest")
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fetch: get %s: %w", rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("fetch: status %d for %s", resp.StatusCode, rawURL)
	}
	ct := resp.Header.Get("Content-Type")
	if !f.contentTypeAllowed(ct) {
		return nil, "", fmt.Errorf("fetch: content-type %q not allowed", ct)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, f.cfg.MaxBytes))
	if err != nil {
		return nil, "", fmt.Errorf("fetch: read body: %w", err)
	}
	return body, ct, nil
}

func (f *Fetcher) contentTypeAllowed(ct string) bool {
	media := strings.TrimSpace(strings.SplitN(ct, ";", 2)[0])
	for _, a := range f.cfg.AllowedContentTypes {
		if strings.HasPrefix(media, a) {
			return true
		}
	}
	return false
}

// resolveAndValidate resolves host to IPs and returns the first that passes the
// SSRF block check; if every candidate is blocked it errors.
func (f *Fetcher) resolveAndValidate(ctx context.Context, host string) (net.IP, error) {
	// A literal IP host is validated directly (no DNS).
	if literal := net.ParseIP(host); literal != nil {
		if f.blocked(literal) {
			return nil, fmt.Errorf("fetch: blocked IP %s", literal)
		}
		return literal, nil
	}
	ips, err := f.cfg.resolve(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("fetch: resolve %s: %w", host, err)
	}
	for _, ip := range ips {
		if !f.blocked(ip) {
			return ip, nil
		}
	}
	return nil, fmt.Errorf("fetch: all resolved IPs for %s are blocked", host)
}

func (f *Fetcher) blocked(ip net.IP) bool {
	if f.cfg.allowLoopback && ip.IsLoopback() {
		return false
	}
	return isBlockedIP(ip)
}

func validateScheme(u *url.URL) error {
	switch u.Scheme {
	case "http", "https":
		return nil
	default:
		return fmt.Errorf("fetch: scheme %q not allowed (http/https only)", u.Scheme)
	}
}

// cgnat is the RFC 6598 carrier-grade NAT range (100.64.0.0/10).
var cgnat = &net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}

// isBlockedIP returns true for any IP that is NOT a routable public address:
// loopback, private, link-local (incl. 169.254.169.254 metadata), multicast,
// unspecified, interface-local, and RFC 6598 CGNAT.
func isBlockedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsInterfaceLocalMulticast() {
		return true
	}
	if v4 := ip.To4(); v4 != nil && cgnat.Contains(v4) {
		return true
	}
	return false
}
```
> Note: `net.IP.IsPrivate` covers `10/8`, `172.16/12`, `192.168/16`, and `fc00::/7` (unique-local v6). `IsLinkLocalUnicast` covers `169.254/16` (incl. the `169.254.169.254` cloud-metadata address) and `fe80::/10`. CGNAT is added explicitly. The `allowLoopback` and `resolve` fields are unexported, so only same-package tests can set them — production callers (Task 6) construct `Config` with public fields only.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/fetch/ -v`
Expected: PASS (`TestIsBlockedIP` 16 cases, scheme/private/public/content-type tests). No DB, no network needed.

- [ ] **Step 5: Commit**

```bash
cd llm-agent-kb && git add internal/fetch/ && \
git commit -m "feat(fetch): SSRF-safe outbound HTTP — scheme allowlist + resolve-then-validate-IP + dial-resolved-IP (no DNS rebinding) + per-hop redirect check + body/MIME caps (§16.3)"
```

---

## Task 4: PDF parser (`parse_pdf.go`)

**Files:**
- Create: `llm-agent-kb/internal/ingest/parse_pdf.go`
- Test: `llm-agent-kb/internal/ingest/parse_pdf_test.go`

Pure-Go text extraction via `ledongthuc/pdf`. Input is the raw uploaded bytes; output is plain text (treated as text downstream like txt). A parse deadline is applied by the caller (worker) via context; the parse function itself accepts `[]byte` and is synchronous, so the worker wraps it with a timeout goroutine (Task 7). Here we keep it a pure byte→string transform.

- [ ] **Step 1: Write the failing test**

Create `internal/ingest/parse_pdf_test.go`. We generate a tiny valid one-page text PDF inline (a minimal hand-written PDF with a `Tj` text-showing operator) so the test needs no fixture file:
```go
package ingest

import (
	"strings"
	"testing"
)

// minimalPDF is a hand-written single-page PDF whose content stream shows the
// text "Hello PDF". It is the smallest structurally-valid PDF the ledongthuc
// reader can extract text from.
const minimalPDF = "%PDF-1.4\n" +
	"1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n" +
	"2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n" +
	"3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 612 792]/Contents 4 0 R/Resources<</Font<</F1 5 0 R>>>>>>endobj\n" +
	"4 0 obj<</Length 44>>stream\n" +
	"BT /F1 24 Tf 100 700 Td (Hello PDF) Tj ET\n" +
	"endstream endobj\n" +
	"5 0 obj<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>endobj\n" +
	"xref\n0 6\n0000000000 65535 f \n0000000009 00000 n \n0000000052 00000 n \n0000000101 00000 n \n0000000209 00000 n \n0000000300 00000 n \n" +
	"trailer<</Size 6/Root 1 0 R>>\nstartxref\n364\n%%EOF"

func TestParsePDFExtractsText(t *testing.T) {
	text, err := parsePDF([]byte(minimalPDF))
	if err != nil {
		t.Fatalf("parsePDF: %v", err)
	}
	if !strings.Contains(text, "Hello PDF") {
		t.Fatalf("extracted text=%q want to contain 'Hello PDF'", text)
	}
}

func TestParsePDFRejectsGarbage(t *testing.T) {
	if _, err := parsePDF([]byte("not a pdf at all")); err == nil {
		t.Fatal("garbage bytes should fail to parse as PDF")
	}
}
```
> Note: if the hand-written `xref` offsets cause the reader to reject the doc, the executor must regenerate the fixture with correct byte offsets (the `ledongthuc` reader is offset-sensitive). Robust fallback: commit a tiny real `testdata/hello.pdf` produced by any tool and `os.ReadFile` it instead — the assertion (`Contains "Hello PDF"`) is unchanged. Prefer the inline constant; fall back to `testdata/` only if the reader rejects it.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/ingest/ -run TestParsePDF -v`
Expected: FAIL to compile — undefined `parsePDF`.

- [ ] **Step 3: Write the implementation**

Create `internal/ingest/parse_pdf.go`:
```go
package ingest

import (
	"bytes"
	"fmt"
	"io"

	"github.com/ledongthuc/pdf"
)

// parsePDF extracts plain text from a text-based PDF (spec §6: text-only;
// scanned/OCR is out of scope). Pure-Go via ledongthuc/pdf.
func parsePDF(raw []byte) (string, error) {
	r, err := pdf.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return "", fmt.Errorf("ingest: open pdf: %w", err)
	}
	rc, err := r.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("ingest: pdf plain text: %w", err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, rc); err != nil {
		return "", fmt.Errorf("ingest: read pdf text: %w", err)
	}
	if buf.Len() == 0 {
		return "", fmt.Errorf("ingest: pdf produced no extractable text (scanned/image PDF unsupported)")
	}
	return buf.String(), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/ingest/ -run TestParsePDF -v`
Expected: PASS (2 tests). No DB needed.

- [ ] **Step 5: Commit**

```bash
cd llm-agent-kb && git add internal/ingest/parse_pdf.go internal/ingest/parse_pdf_test.go && \
git commit -m "feat(ingest): text-only PDF parser via ledongthuc/pdf (NewReader+GetPlainText)"
```

---

## Task 5: DOCX parser (`parse_docx.go`)

**Files:**
- Create: `llm-agent-kb/internal/ingest/parse_docx.go`
- Test: `llm-agent-kb/internal/ingest/parse_docx_test.go`

Pure-Go OOXML via `fumiama/go-docx`. We walk `Document.Body.Items` → `*Paragraph` → `Children` (`*Run`) → `Children` (`*Text`) and join paragraph texts with newlines. The test BUILDS a `.docx` in memory using the same library's writer (`docx.New()` + `AddParagraph().AddText()` + `WriteTo`) so no fixture file is needed.

- [ ] **Step 1: Write the failing test**

Create `internal/ingest/parse_docx_test.go`:
```go
package ingest

import (
	"bytes"
	"strings"
	"testing"

	docx "github.com/fumiama/go-docx"
)

// buildDocx returns a valid .docx byte stream with two paragraphs.
func buildDocx(t *testing.T) []byte {
	t.Helper()
	d := docx.New()
	d.AddParagraph().AddText("First paragraph about foxes.")
	d.AddParagraph().AddText("Second paragraph about dogs.")
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatalf("write docx: %v", err)
	}
	return buf.Bytes()
}

func TestParseDOCXExtractsParagraphs(t *testing.T) {
	raw := buildDocx(t)
	text, err := parseDOCX(raw)
	if err != nil {
		t.Fatalf("parseDOCX: %v", err)
	}
	if !strings.Contains(text, "First paragraph about foxes.") ||
		!strings.Contains(text, "Second paragraph about dogs.") {
		t.Fatalf("extracted text missing paragraphs: %q", text)
	}
}

func TestParseDOCXRejectsGarbage(t *testing.T) {
	if _, err := parseDOCX([]byte("PK\x03\x04 but not really a docx")); err == nil {
		t.Fatal("garbage should fail to parse as docx")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/ingest/ -run TestParseDOCX -v`
Expected: FAIL to compile — undefined `parseDOCX`.

- [ ] **Step 3: Write the implementation**

Create `internal/ingest/parse_docx.go`:
```go
package ingest

import (
	"bytes"
	"fmt"
	"strings"

	docx "github.com/fumiama/go-docx"
)

// parseDOCX extracts plain text from an OOXML .docx. Pure-Go via fumiama/go-docx.
// It walks Body.Items (paragraphs) → Run children → Text children, joining
// paragraphs with newlines. Tables/images are skipped (text-only, spec §6).
func parseDOCX(raw []byte) (string, error) {
	doc, err := docx.Parse(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return "", fmt.Errorf("ingest: parse docx: %w", err)
	}
	var paragraphs []string
	for _, item := range doc.Document.Body.Items {
		p, ok := item.(*docx.Paragraph)
		if !ok {
			continue
		}
		var sb strings.Builder
		for _, child := range p.Children {
			run, ok := child.(*docx.Run)
			if !ok {
				continue
			}
			for _, rc := range run.Children {
				if txt, ok := rc.(*docx.Text); ok {
					sb.WriteString(txt.Text)
				}
			}
		}
		if line := strings.TrimSpace(sb.String()); line != "" {
			paragraphs = append(paragraphs, line)
		}
	}
	if len(paragraphs) == 0 {
		return "", fmt.Errorf("ingest: docx produced no extractable text")
	}
	return strings.Join(paragraphs, "\n\n"), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/ingest/ -run TestParseDOCX -v`
Expected: PASS (2 tests). No DB needed.

- [ ] **Step 5: Commit**

```bash
cd llm-agent-kb && git add internal/ingest/parse_docx.go internal/ingest/parse_docx_test.go && \
git commit -m "feat(ingest): text-only DOCX parser via fumiama/go-docx (walk Body→Paragraph→Run→Text)"
```

---

## Task 6: URL parser (`parse_url.go`) — SSRF fetch + readability → text

**Files:**
- Create: `llm-agent-kb/internal/ingest/parse_url.go`
- Test: `llm-agent-kb/internal/ingest/parse_url_test.go`

The URL source parses to text in two steps: (1) `internal/fetch.Fetcher.Get` retrieves the HTML safely (SSRF guard), (2) `go-readability.FromReader` extracts the main article text. The extracted `TextContent` is treated as text downstream (the rag splitter handles structure during Import). `parseURL` takes a `*fetch.Fetcher` so the worker can inject a configured one and tests can inject a loopback-allowed one.

- [ ] **Step 1: Write the failing test**

Create `internal/ingest/parse_url_test.go`:
```go
package ingest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/costa92/llm-agent-kb/internal/fetch"
)

func TestParseURLExtractsArticleText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head><title>Foxes</title></head><body>
			<article><h1>The Fox</h1><p>The quick brown fox jumps over the lazy dog. ` +
			strings.Repeat("This is a long article body so readability keeps it. ", 20) +
			`</p></article></body></html>`))
	}))
	defer srv.Close()

	f := fetch.NewLoopbackForTest(time.Second, 1<<20, []string{"text/html"})
	text, title, err := parseURL(context.Background(), f, srv.URL)
	if err != nil {
		t.Fatalf("parseURL: %v", err)
	}
	if !strings.Contains(text, "quick brown fox") {
		t.Fatalf("text missing article body: %q", text)
	}
	if title == "" {
		t.Fatalf("title not extracted")
	}
}

func TestParseURLPropagatesFetchError(t *testing.T) {
	f := fetch.NewLoopbackForTest(time.Second, 1<<20, []string{"text/html"})
	if _, _, err := parseURL(context.Background(), f, "http://10.0.0.1/"); err == nil {
		t.Fatal("private URL must error through parseURL")
	}
}
```
> This test needs a small test-only constructor `fetch.NewLoopbackForTest` that builds a Fetcher with `allowLoopback:true` (the field is unexported). Add it in Step 3.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/ingest/ -run TestParseURL -v`
Expected: FAIL to compile — undefined `parseURL`, `fetch.NewLoopbackForTest`.

- [ ] **Step 3: Write the implementation**

First add the test-only constructor to `internal/fetch/fetch.go` (it lives in the prod package but is only used by tests; it sets the unexported flag for the loopback httptest servers — production code never calls it). Append:
```go
// NewLoopbackForTest builds a Fetcher that permits loopback IPs, for tests that
// must reach an httptest server. NOT for production use.
func NewLoopbackForTest(timeout time.Duration, maxBytes int64, allowed []string) *Fetcher {
	return New(Config{Timeout: timeout, MaxBytes: maxBytes, AllowedContentTypes: allowed, allowLoopback: true})
}
```
Then create `internal/ingest/parse_url.go`:
```go
package ingest

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"strings"

	readability "github.com/go-shiori/go-readability"

	"github.com/costa92/llm-agent-kb/internal/fetch"
)

// parseURL fetches an HTML page through the SSRF-safe Fetcher and extracts the
// main article text via go-readability. The extracted text is treated as plain
// text downstream (the rag splitter handles structure during Import). Returns
// (text, title, error).
func parseURL(ctx context.Context, f *fetch.Fetcher, rawURL string) (string, string, error) {
	body, _, err := f.Get(ctx, rawURL)
	if err != nil {
		return "", "", err // fetch already wraps with SSRF/transport context
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", fmt.Errorf("ingest: parse url: %w", err)
	}
	art, err := readability.FromReader(bytes.NewReader(body), u)
	if err != nil {
		return "", "", fmt.Errorf("ingest: readability extract %s: %w", rawURL, err)
	}
	text := strings.TrimSpace(art.TextContent)
	if text == "" {
		return "", "", fmt.Errorf("ingest: no readable content extracted from %s", rawURL)
	}
	return text, strings.TrimSpace(art.Title), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/ingest/ -run TestParseURL -v && GOWORK=off go test ./internal/fetch/ -v`
Expected: PASS (parseURL 2 tests; fetch tests still pass). No DB needed.

- [ ] **Step 5: Commit**

```bash
cd llm-agent-kb && git add internal/ingest/parse_url.go internal/ingest/parse_url_test.go internal/fetch/fetch.go && \
git commit -m "feat(ingest): URL parser — SSRF-safe fetch + go-readability article extraction"
```

---

## Task 7: parse dispatch + upload validation helpers (`parse.go`)

**Files:**
- Create: `llm-agent-kb/internal/ingest/parse.go` (moves + extends the `parse` switch out of `ingest.go`)
- Change: `llm-agent-kb/internal/ingest/ingest.go` (delete the old `parse` + `SourceType` consts that move)
- Test: `llm-agent-kb/internal/ingest/parse_test.go`

Extend the `source_type` set with `pdf`/`docx`/`url`, and provide a single dispatch the worker calls: `parseSource(ctx, deps, st, raw, sourceRef) (text, title, error)`. PDF/DOCX dispatch to Tasks 4/5; URL dispatches to Task 6 using a `*fetch.Fetcher`; md/txt/paste keep the M1 behavior. Also add the exported upload-validation helper `ValidateUpload(filename string, size, maxBytes int64) error` (extension allowlist + size cap — the size cap is also enforced earlier by `MaxBytesReader`, re-checked here). This is the FINAL signature; the httpapi `uploadHandler` (Task 9) calls it as-is — Task 9 does NOT re-signature it.

- [ ] **Step 1: Write the failing test**

Create `internal/ingest/parse_test.go`:
```go
package ingest

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/costa92/llm-agent-kb/internal/fetch"
)

func TestParseSourceTextTypes(t *testing.T) {
	deps := parseDeps{fetcher: nil, parseTimeout: time.Second}
	for _, st := range []SourceType{SourceTypeMarkdown, SourceTypeTXT, SourceTypePaste} {
		text, _, err := parseSource(context.Background(), deps, st, []byte("# hi\nbody"), "")
		if err != nil {
			t.Fatalf("%s: %v", st, err)
		}
		if !strings.Contains(text, "body") {
			t.Fatalf("%s text=%q", st, text)
		}
	}
}

func TestParseSourcePDF(t *testing.T) {
	deps := parseDeps{parseTimeout: 5 * time.Second}
	text, _, err := parseSource(context.Background(), deps, SourceTypePDF, []byte(minimalPDF), "")
	if err != nil {
		t.Fatalf("pdf: %v", err)
	}
	if !strings.Contains(text, "Hello PDF") {
		t.Fatalf("pdf text=%q", text)
	}
}

func TestParseSourceUnsupported(t *testing.T) {
	if _, _, err := parseSource(context.Background(), parseDeps{}, SourceType("bogus"), nil, ""); err == nil {
		t.Fatal("unknown source type must error")
	}
}

func TestValidateUpload(t *testing.T) {
	// allowed
	if err := ValidateUpload("report.pdf", 100, 1<<20); err != nil {
		t.Fatalf("pdf should be allowed: %v", err)
	}
	if err := ValidateUpload("notes.md", 100, 1<<20); err != nil {
		t.Fatalf("md should be allowed: %v", err)
	}
	// disallowed extension
	if err := ValidateUpload("evil.exe", 100, 1<<20); err == nil {
		t.Fatal("exe must be rejected")
	}
	// over size
	if err := ValidateUpload("big.pdf", 2<<20, 1<<20); err == nil {
		t.Fatal("oversize must be rejected")
	}
}

func TestParseTimeoutFires(t *testing.T) {
	// A canceled context makes the PDF parse return a deadline error.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	deps := parseDeps{parseTimeout: time.Hour}
	if _, _, err := parseSource(ctx, deps, SourceTypePDF, []byte(minimalPDF), ""); err == nil {
		t.Fatal("canceled context should abort parse")
	}
}

var _ = fetch.New // keep import even if fetcher unused in some subtests
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/ingest/ -run 'TestParseSource|TestValidateUpload|TestParseTimeout' -v`
Expected: FAIL to compile — undefined `parseDeps`, `parseSource`, `SourceTypePDF`, `ValidateUpload`.

- [ ] **Step 3: Write the implementation**

In `internal/ingest/ingest.go`, **remove** the M1 `SourceType` consts block and the `parse` function (they move to `parse.go`). Keep `checksum`, `makeDocument`, `Service`, `New`, `IngestInput`, `Result`, `newID`.

Create `internal/ingest/parse.go`:
```go
package ingest

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/costa92/llm-agent-kb/internal/fetch"
)

// SourceType is the accepted document source set (M1 text + M2 pdf/docx/url).
type SourceType string

const (
	SourceTypeMarkdown SourceType = "markdown"
	SourceTypeTXT      SourceType = "txt"
	SourceTypePaste    SourceType = "paste"
	SourceTypePDF      SourceType = "pdf"
	SourceTypeDOCX     SourceType = "docx"
	SourceTypeURL      SourceType = "url"
)

// parseDeps carries the runtime collaborators a parse needs.
type parseDeps struct {
	fetcher      *fetch.Fetcher // for SourceTypeURL
	parseTimeout time.Duration  // deadline around PDF/DOCX parse (anti parse-bomb)
}

// parseSource converts a source's raw bytes (or, for URL, the URL in sourceRef)
// into (text, title, error). md/txt/paste are read as text; pdf/docx are parsed
// under a parse-timeout; url is fetched (SSRF-safe) + readability-extracted.
func parseSource(ctx context.Context, deps parseDeps, st SourceType, raw []byte, sourceRef string) (string, string, error) {
	switch st {
	case SourceTypeMarkdown, SourceTypeTXT, SourceTypePaste:
		return string(raw), "", nil
	case SourceTypePDF:
		return withParseTimeout(ctx, deps.parseTimeout, func() (string, error) { return parsePDF(raw) })
	case SourceTypeDOCX:
		return withParseTimeout(ctx, deps.parseTimeout, func() (string, error) { return parseDOCX(raw) })
	case SourceTypeURL:
		if deps.fetcher == nil {
			return "", "", fmt.Errorf("ingest: url source requires a fetcher")
		}
		return parseURL(ctx, deps.fetcher, sourceRef)
	default:
		return "", "", fmt.Errorf("ingest: unsupported source_type %q", st)
	}
}

// withParseTimeout runs a synchronous CPU-bound parse with a deadline. If the
// context is already done OR the timeout elapses, it returns the context error
// (the parse goroutine is abandoned — acceptable for a bounded text extract).
func withParseTimeout(ctx context.Context, timeout time.Duration, fn func() (string, error)) (string, string, error) {
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	if timeout <= 0 {
		text, err := fn()
		return text, "", err
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	type res struct {
		text string
		err  error
	}
	ch := make(chan res, 1)
	go func() { t, e := fn(); ch <- res{t, e} }()
	select {
	case <-cctx.Done():
		return "", "", fmt.Errorf("ingest: parse timed out: %w", cctx.Err())
	case r := <-ch:
		return r.text, "", r.err
	}
}

// allowedUpload maps the content-type+extension allowlist (§16.3): pdf/md/txt/docx.
var allowedExtensions = map[string]bool{
	".pdf": true, ".md": true, ".markdown": true, ".txt": true, ".docx": true,
}

// ValidateUpload enforces the §16.3 upload allowlist: the filename extension
// must be in the allowlist and size must be within maxBytes. (Content-type is
// not checked — clients lie; the extension governs, and the actual parse in
// Task 4/5 is the real validator.) Exported because the httpapi uploadHandler
// (Task 9, another package) calls it; this is the FINAL signature used everywhere.
func ValidateUpload(filename string, size, maxBytes int64) error {
	if size > maxBytes {
		return fmt.Errorf("ingest: upload %d bytes exceeds max %d", size, maxBytes)
	}
	ext := strings.ToLower(filepath.Ext(filename))
	if !allowedExtensions[ext] {
		return fmt.Errorf("ingest: extension %q not allowed (pdf/md/txt/docx only)", ext)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/ingest/ -run 'TestParse|TestValidate' -v && GOWORK=off go build ./...`
Expected: PASS; build clean (the old `parse`/`SourceType` removal from `ingest.go` must not break `makeDocument`, which still references `SourceType`).

- [ ] **Step 5: Commit**

```bash
cd llm-agent-kb && git add internal/ingest/parse.go internal/ingest/parse_test.go internal/ingest/ingest.go && \
git commit -m "feat(ingest): parse dispatch (md/txt/paste + pdf/docx/url) with parse-timeout + upload validation (§16.3); move SourceType out of ingest.go"
```

---

## Task 8: async worker — claim/lease/retry/stuck/dead (`worker.go`), deterministic

**Files:**
- Create: `llm-agent-kb/internal/ingest/worker.go`
- Test: `llm-agent-kb/internal/ingest/worker_test.go`
- Change: `llm-agent-kb/internal/ingest/ingest.go` (add `Enqueue`, the document-list keyset method, the retry method)

The heart of M2. `Enqueue` (replaces sync `Ingest`) writes `document(pending)` + `ingest_job(pending)` in one tx and returns the documentId. A `Worker` claims a due job with `SELECT … FOR UPDATE SKIP LOCKED` (atomically setting `state='running'`, `locked_by`, `locked_until=now()+lease`, bumping `attempts`), parses, Imports, and on success sets `document.status='ready'` + `ingest_job.state='done'`; on failure it either reschedules (`next_run_at=now()+backoff`, `state='pending'`) or, past `maxAttempts`, marks `state='dead'` + `document.status='failed'`. Stuck recovery: the claim query treats a `running` row whose `locked_until < now()` as claimable. A clock is injected for determinism; `RunOnce` claims+processes exactly one job (no sleeps in tests).

- [ ] **Step 1: Write the failing test**

Create `internal/ingest/worker_test.go`:
```go
package ingest

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/costa92/llm-agent-contract/llm"
	ragingest "github.com/costa92/llm-agent-rag/ingest"
	ragpostgres "github.com/costa92/llm-agent-rag/postgres"

	"github.com/costa92/llm-agent-kb/internal/ragsvc"
)

const liveEnvVar = "LLM_AGENT_KB_PG_URL"

// freshWorkerDB sets up its OWN database (M1 lesson: gated tests are not
// co-runnable on a shared DB). It drops everything it owns, migrates, and
// returns a pool + a ragsvc wired to a scripted embedder.
func freshWorkerDB(t *testing.T, ctx context.Context) (*pgxpool.Pool, *ragsvc.Service, *Service, *clock) {
	t.Helper()
	dsn := os.Getenv(liveEnvVar)
	if dsn == "" {
		t.Skipf("set %s (pgvector) to run worker tests", liveEnvVar)
	}
	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	poolCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		return ragpostgres.RegisterTypes(ctx, conn)
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	for _, tbl := range []string{"ingest_job", "document", "knowledge_base", "chunks", "chunks_entities", "chunks_relations", "chunks_communities", "chunks_community_reports"} {
		_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS "+tbl+" CASCADE")
	}
	chunkStore, err := ragpostgres.New(pool, ragpostgres.Config{Dimension: 8})
	if err != nil {
		t.Fatal(err)
	}
	if err := chunkStore.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range businessMigrationsForTest {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("migrate %q: %v", stmt, err)
		}
	}
	// seed a kb row (FK target) — namespace ns1.
	if _, err := pool.Exec(ctx, `INSERT INTO knowledge_base (id, org_id, name, namespace) VALUES ('kb1','o1','KB','ns1')`); err != nil {
		t.Fatal(err)
	}
	model := llm.NewScriptedLLM(llm.WithResponses(llm.Response{Text: "ok", Usage: llm.Usage{TotalTokens: 1}}))
	embedder := llm.NewScriptedLLM(llm.WithEmbedDimensions(8))
	rag := ragsvc.New(ragsvc.Deps{Model: model, Embedder: embedder, RagStore: chunkStore, ChunkStore: chunkStore})
	clk := &clock{now: time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)}
	svc := New(pool, rag)
	return pool, rag, svc, clk
}

func docStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool, docID string) (string, string) {
	t.Helper()
	var status, phase string
	if err := pool.QueryRow(ctx, `SELECT status, phase FROM document WHERE id=$1`, docID).Scan(&status, &phase); err != nil {
		t.Fatalf("doc status: %v", err)
	}
	return status, phase
}

func jobState(t *testing.T, ctx context.Context, pool *pgxpool.Pool, docID string) (string, int) {
	t.Helper()
	var state string
	var attempts int
	if err := pool.QueryRow(ctx, `SELECT state, attempts FROM ingest_job WHERE document_id=$1`, docID).Scan(&state, &attempts); err != nil {
		t.Fatalf("job state: %v", err)
	}
	return state, attempts
}

func TestEnqueueWritesDocumentAndJob(t *testing.T) {
	ctx := context.Background()
	pool, _, svc, _ := freshWorkerDB(t, ctx)
	docID, err := svc.Enqueue(ctx, IngestInput{KBID: "kb1", Namespace: "ns1", Title: "T", SourceType: SourceTypePaste, Raw: []byte("the quick brown fox")})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if status, _ := docStatus(t, ctx, pool, docID); status != "pending" {
		t.Fatalf("status=%q want pending", status)
	}
	if state, _ := jobState(t, ctx, pool, docID); state != "pending" {
		t.Fatalf("job state=%q want pending", state)
	}
}

func TestWorkerRunOnceProcessesToReady(t *testing.T) {
	ctx := context.Background()
	pool, rag, svc, clk := freshWorkerDB(t, ctx)
	docID, _ := svc.Enqueue(ctx, IngestInput{KBID: "kb1", Namespace: "ns1", Title: "T", SourceType: SourceTypePaste, Raw: []byte("the quick brown fox jumps over the lazy dog repeatedly")})

	w := NewWorker(WorkerConfig{Pool: pool, Rag: rag, WorkerID: "w1", Lease: time.Minute, MaxAttempts: 5, BaseBackoff: time.Second, Clock: clk.Now})
	claimed, err := w.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if !claimed {
		t.Fatal("RunOnce should have claimed the pending job")
	}
	if status, _ := docStatus(t, ctx, pool, docID); status != "ready" {
		t.Fatalf("status=%q want ready", status)
	}
	if state, _ := jobState(t, ctx, pool, docID); state != "done" {
		t.Fatalf("job state=%q want done", state)
	}
	var chunks int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM chunks WHERE namespace='ns1'`).Scan(&chunks); err != nil {
		t.Fatal(err)
	}
	if chunks == 0 {
		t.Fatal("no chunks imported")
	}
}

func TestWorkerRunOnceEmptyQueue(t *testing.T) {
	ctx := context.Background()
	pool, rag, _, clk := freshWorkerDB(t, ctx)
	w := NewWorker(WorkerConfig{Pool: pool, Rag: rag, WorkerID: "w1", Lease: time.Minute, MaxAttempts: 5, BaseBackoff: time.Second, Clock: clk.Now})
	claimed, err := w.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if claimed {
		t.Fatal("empty queue should claim nothing")
	}
}

// TestWorkerRetryThenDead drives a job whose parse always fails (unsupported
// source type injected directly into the row) through attempts→dead, advancing
// the injected clock past each backoff so the job is due again.
func TestWorkerRetryThenDead(t *testing.T) {
	ctx := context.Background()
	pool, rag, svc, clk := freshWorkerDB(t, ctx)
	// Enqueue a normal job then corrupt its source_type to force parse failure.
	docID, _ := svc.Enqueue(ctx, IngestInput{KBID: "kb1", Namespace: "ns1", Title: "T", SourceType: SourceTypePaste, Raw: []byte("x")})
	if _, err := pool.Exec(ctx, `UPDATE document SET source_type='bogus' WHERE id=$1`, docID); err != nil {
		t.Fatal(err)
	}
	w := NewWorker(WorkerConfig{Pool: pool, Rag: rag, WorkerID: "w1", Lease: time.Minute, MaxAttempts: 3, BaseBackoff: time.Second, Clock: clk.Now})
	for i := 0; i < 3; i++ {
		claimed, err := w.RunOnce(ctx)
		if err != nil {
			t.Fatalf("RunOnce attempt %d: %v", i, err)
		}
		if !claimed {
			t.Fatalf("attempt %d should have claimed (job due)", i)
		}
		// advance clock past the scheduled backoff so the retry is due.
		clk.Advance(time.Hour)
	}
	state, attempts := jobState(t, ctx, pool, docID)
	if state != "dead" {
		t.Fatalf("job state=%q want dead after maxAttempts", state)
	}
	if attempts != 3 {
		t.Fatalf("attempts=%d want 3", attempts)
	}
	if status, _ := docStatus(t, ctx, pool, docID); status != "failed" {
		t.Fatalf("doc status=%q want failed", status)
	}
}

// TestWorkerReclaimsStuckLease proves a 'running' job whose lease expired is
// reclaimable by another worker.
func TestWorkerReclaimsStuckLease(t *testing.T) {
	ctx := context.Background()
	pool, rag, svc, clk := freshWorkerDB(t, ctx)
	docID, _ := svc.Enqueue(ctx, IngestInput{KBID: "kb1", Namespace: "ns1", Title: "T", SourceType: SourceTypePaste, Raw: []byte("the quick brown fox jumps")})
	// Simulate worker A crashing mid-job: mark running with an EXPIRED lease.
	expired := clk.Now().Add(-time.Minute)
	if _, err := pool.Exec(ctx, `UPDATE ingest_job SET state='running', locked_by='dead-worker', locked_until=$2, attempts=1 WHERE document_id=$1`, docID, expired); err != nil {
		t.Fatal(err)
	}
	wB := NewWorker(WorkerConfig{Pool: pool, Rag: rag, WorkerID: "wB", Lease: time.Minute, MaxAttempts: 5, BaseBackoff: time.Second, Clock: clk.Now})
	claimed, err := wB.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if !claimed {
		t.Fatal("worker B should reclaim the stuck (expired-lease) job")
	}
	if status, _ := docStatus(t, ctx, pool, docID); status != "ready" {
		t.Fatalf("status=%q want ready after reclaim", status)
	}
}

// clock is an injectable monotonic-ish clock for deterministic worker tests.
type clock struct{ now time.Time }

func (c *clock) Now() time.Time      { return c.now }
func (c *clock) Advance(d time.Duration) { c.now = c.now.Add(d) }

// businessMigrationsForTest mirrors storage.businessMigrations (the worker test
// builds its own DB without importing storage to avoid an import cycle risk).
var businessMigrationsForTest = []string{
	`CREATE TABLE IF NOT EXISTS knowledge_base (id TEXT PRIMARY KEY, org_id TEXT NOT NULL, name TEXT NOT NULL, namespace TEXT NOT NULL UNIQUE, embedding_model TEXT NOT NULL DEFAULT '', embedding_dim INT NOT NULL DEFAULT 0, created_at TIMESTAMPTZ NOT NULL DEFAULT now())`,
	`CREATE TABLE IF NOT EXISTS document (id TEXT PRIMARY KEY, kb_id TEXT NOT NULL REFERENCES knowledge_base(id) ON DELETE CASCADE, title TEXT NOT NULL, source_type TEXT NOT NULL, source_ref TEXT NOT NULL DEFAULT '', source_id TEXT NOT NULL, checksum TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'pending', phase TEXT NOT NULL DEFAULT '', error TEXT NOT NULL DEFAULT '', chunk_count INT NOT NULL DEFAULT 0, content_bytes BIGINT NOT NULL DEFAULT 0, content BYTEA, created_at TIMESTAMPTZ NOT NULL DEFAULT now())`,
	`CREATE TABLE IF NOT EXISTS ingest_job (id TEXT PRIMARY KEY, document_id TEXT REFERENCES document(id) ON DELETE CASCADE, state TEXT NOT NULL DEFAULT 'pending', attempts INT NOT NULL DEFAULT 0, next_run_at TIMESTAMPTZ NOT NULL DEFAULT now(), locked_by TEXT NOT NULL DEFAULT '', locked_until TIMESTAMPTZ, idempotency_key TEXT NOT NULL, last_error TEXT NOT NULL DEFAULT '', phase TEXT NOT NULL DEFAULT '', updated_at TIMESTAMPTZ NOT NULL DEFAULT now())`,
	`CREATE UNIQUE INDEX IF NOT EXISTS ingest_job_idem_idx ON ingest_job (idempotency_key)`,
}
```
> Note: the worker uses the DB's `now()` for the *atomic claim* `FOR UPDATE SKIP LOCKED` comparison (so claim/reclaim is decided server-side, race-free), but uses the **injected clock** for computing `next_run_at` backoff and `locked_until`. The stuck-lease test sets an expired `locked_until` so the claim's `locked_until < now()` branch fires regardless of the injected clock. This keeps the *reclaim decision* deterministic against the DB clock while keeping *backoff scheduling* testable.

- [ ] **Step 2: Run test to verify it fails**

Run (gated DB up): `cd llm-agent-kb && GOWORK=off go test ./internal/ingest/ -run 'TestEnqueue|TestWorker' -v`
Expected: FAIL to compile — undefined `Enqueue`, `NewWorker`, `WorkerConfig`, `(*Worker).RunOnce`.

- [ ] **Step 3: Write the implementation**

Add `Enqueue` and the keyset/retry repo methods to `internal/ingest/ingest.go`. Replace the synchronous `Ingest` method with `Enqueue` (the M1 `Ingest` is removed — `httpapi` switches to `Enqueue` in Task 9). Add to `ingest.go`:
```go
// Enqueue writes document(pending) + ingest_job(pending) in one tx and returns
// the documentId. The worker (worker.go) drains the queue asynchronously.
// Replaces M1's synchronous Ingest (spec §6: 202 + documentId).
func (s *Service) Enqueue(ctx context.Context, in IngestInput) (string, error) {
	docID := newID()
	// For URL sources, the URL lives in SourceRef and Raw is empty; for file/paste
	// the content is in Raw. content_bytes drives the per-kb quota.
	cs := ""
	if in.SourceType != SourceTypeURL {
		cs = checksum(string(in.Raw))
	}
	// Reimport/dedup short-circuit (spec §6, deferred from M1): if a ready
	// document with the same (kb_id, source_ref OR title) already has this exact
	// checksum, skip enqueue and return the existing id. Same-source key = the
	// (kb_id, source_ref) pair for url/file, else (kb_id, title) for paste.
	// Best-effort only: this read runs OUTSIDE the enqueue tx, so two concurrent
	// identical uploads may both miss the check and both enqueue. Acceptable for
	// M2 (worker uses ReplaceSource:true; the duplicate just re-indexes the same
	// source) — not a transactional guarantee.
	if cs != "" {
		var existing string
		err := s.pool.QueryRow(ctx,
			`SELECT id FROM document
			 WHERE kb_id=$1 AND checksum=$2 AND status='ready'
			 AND ((source_ref<>'' AND source_ref=$3) OR (source_ref='' AND title=$4))
			 LIMIT 1`, in.KBID, cs, in.SourceRef, in.Title).Scan(&existing)
		if err == nil && existing != "" {
			return existing, nil // identical content already indexed — no-op
		}
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx,
		`INSERT INTO document (id, kb_id, title, source_type, source_ref, source_id, checksum, status, content_bytes, content)
		 VALUES ($1,$2,$3,$4,$5,$1,$6,'pending',$7,$8)`,
		docID, in.KBID, in.Title, string(in.SourceType), in.SourceRef, cs, int64(len(in.Raw)), in.Raw); err != nil {
		return "", fmt.Errorf("ingest: insert document: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO ingest_job (id, document_id, state, idempotency_key)
		 VALUES ($1,$2,'pending',$3)`,
		newID(), docID, docID); err != nil {
		return "", fmt.Errorf("ingest: insert job: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return docID, nil
}

// KBContentBytes returns the cumulative content_bytes for a kb (quota accounting).
func (s *Service) KBContentBytes(ctx context.Context, kbID string) (int64, error) {
	var n int64
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(sum(content_bytes),0) FROM document WHERE kb_id=$1`, kbID).Scan(&n)
	return n, err
}

// DocumentView is a row of the documents list.
type DocumentView struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	SourceType string `json:"sourceType"`
	Status     string `json:"status"`
	Phase      string `json:"phase"`
	Error      string `json:"error,omitempty"`
	ChunkCount int    `json:"chunkCount"`
}

// ListDocuments returns up to limit documents for a kb, keyset-paginated by id
// (mirrors orgkb.ListByOrg). Empty cursor starts from the beginning.
func (s *Service) ListDocuments(ctx context.Context, kbID string, limit int, cursor string) ([]DocumentView, string, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, title, source_type, status, phase, error, chunk_count
		 FROM document WHERE kb_id=$1 AND id>$2 ORDER BY id ASC LIMIT $3`,
		kbID, cursor, limit)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	var out []DocumentView
	for rows.Next() {
		var d DocumentView
		if err := rows.Scan(&d.ID, &d.Title, &d.SourceType, &d.Status, &d.Phase, &d.Error, &d.ChunkCount); err != nil {
			return nil, "", err
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(out) == limit {
		next = out[len(out)-1].ID
	}
	return out, next, nil
}

// Retry re-enqueues a failed/dead document's job (spec §16.2 POST .../retry):
// resets the job to pending+due-now with attempts cleared, and the document to
// pending. Errors if no job exists for the document.
func (s *Service) Retry(ctx context.Context, kbID, docID string) error {
	// Job-state vocabulary (ingest_job.state): pending / running / done / dead.
	// 'failed' is a document.status, NOT a job state; the worker's fail() only
	// produces 'dead' (terminal) or 'pending' (backoff retry). Only a 'dead' job
	// is manually retryable — a 'done' job already indexed successfully.
	tag, err := s.pool.Exec(ctx,
		`UPDATE ingest_job j SET state='pending', attempts=0, next_run_at=now(), locked_by='', locked_until=NULL, last_error=''
		 FROM document d
		 WHERE j.document_id=d.id AND d.id=$1 AND d.kb_id=$2 AND j.state = 'dead'`,
		docID, kbID)
	if err != nil {
		return fmt.Errorf("ingest: retry: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("ingest: no retryable job for document %s", docID)
	}
	_, err = s.pool.Exec(ctx, `UPDATE document SET status='pending', error='', phase='' WHERE id=$1`, docID)
	return err
}
```
Add `SourceRef string` to `IngestInput` in `ingest.go` (after `Title`):
```go
	SourceRef  string // url for SourceTypeURL; original filename for files
```

Now create `internal/ingest/worker.go`:
```go
package ingest

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	ragingest "github.com/costa92/llm-agent-rag/ingest"

	"github.com/costa92/llm-agent-kb/internal/fetch"
	"github.com/costa92/llm-agent-kb/internal/ragsvc"
)

// WorkerConfig configures a Worker.
type WorkerConfig struct {
	Pool         *pgxpool.Pool
	Rag          ragsvc.RagPort
	Fetcher      *fetch.Fetcher // for url sources; may be nil if url ingest disabled
	WorkerID     string
	Lease        time.Duration
	MaxAttempts  int
	BaseBackoff  time.Duration
	ParseTimeout time.Duration
	Clock        func() time.Time // injected for deterministic tests; nil → time.Now
	Logger       *slog.Logger     // nil → slog.Default()
}

// Worker drains the ingest_job queue.
type Worker struct {
	cfg WorkerConfig
}

// NewWorker builds a Worker.
func NewWorker(cfg WorkerConfig) *Worker {
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 5
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Worker{cfg: cfg}
}

// claimed describes a job atomically claimed for processing.
type claimed struct {
	jobID      string
	docID      string
	attempts   int
	kbID       string
	namespace  string
	title      string
	sourceType SourceType
	sourceRef  string
	checksum   string
	raw        []byte
}

// RunOnce claims (if any due/stuck job exists) and processes exactly one job.
// Returns (true, nil) if a job was claimed+processed, (false, nil) if the queue
// is empty. Deterministic for tests (no sleeps).
func (w *Worker) RunOnce(ctx context.Context) (bool, error) {
	c, ok, err := w.claim(ctx)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	w.process(ctx, c)
	return true, nil
}

// Run loops RunOnce, sleeping pollInterval when the queue is empty, until ctx is
// canceled (graceful drain). Production entrypoint (cmd/kbd).
func (w *Worker) Run(ctx context.Context, pollInterval time.Duration) {
	for {
		if ctx.Err() != nil {
			return
		}
		claimed, err := w.RunOnce(ctx)
		if err != nil {
			// transient DB error — log it (otherwise a persistent claim failure
			// is an invisible hot-idle) and back off one poll interval.
			w.cfg.Logger.Error("ingest worker claim failed", "worker", w.cfg.WorkerID, "err", err)
			claimed = false
		}
		if !claimed {
			select {
			case <-ctx.Done():
				return
			case <-time.After(pollInterval):
			}
		}
	}
}

// claim atomically selects one claimable job (pending+due OR running with an
// expired lease) using FOR UPDATE SKIP LOCKED, marks it running with a fresh
// lease, bumps attempts, and loads the document fields needed to process it.
// The lease (locked_until) and the staleness comparison use the DB clock
// (now()) so concurrent workers cannot double-claim; backoff scheduling on
// failure uses the injected clock.
func (w *Worker) claim(ctx context.Context) (claimed, bool, error) {
	tx, err := w.cfg.Pool.Begin(ctx)
	if err != nil {
		return claimed{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	lease := int(w.cfg.Lease / time.Second)
	if lease <= 0 {
		lease = 60
	}
	var jobID, docID string
	var attempts int
	row := tx.QueryRow(ctx, `
		SELECT id, document_id, attempts FROM ingest_job
		WHERE (state='pending' AND next_run_at <= now())
		   OR (state='running' AND locked_until IS NOT NULL AND locked_until < now())
		ORDER BY next_run_at ASC
		FOR UPDATE SKIP LOCKED
		LIMIT 1`)
	if err := row.Scan(&jobID, &docID, &attempts); err != nil {
		if err == pgx.ErrNoRows {
			return claimed{}, false, nil
		}
		return claimed{}, false, err
	}
	attempts++
	if _, err := tx.Exec(ctx, `
		UPDATE ingest_job
		SET state='running', locked_by=$2, locked_until = now() + ($3 || ' seconds')::interval,
		    attempts=$4, phase='parsing', updated_at=now()
		WHERE id=$1`, jobID, w.cfg.WorkerID, lease, attempts); err != nil {
		return claimed{}, false, err
	}
	c := claimed{jobID: jobID, docID: docID, attempts: attempts}
	if err := tx.QueryRow(ctx, `
		SELECT d.kb_id, kb.namespace, d.title, d.source_type, d.source_ref, d.checksum, d.content_bytes
		FROM document d JOIN knowledge_base kb ON kb.id=d.kb_id WHERE d.id=$1`, docID).
		Scan(&c.kbID, &c.namespace, &c.title, (*string)(&c.sourceType), &c.sourceRef, &c.checksum, new(int64)); err != nil {
		return claimed{}, false, err
	}
	// Mark the document parsing.
	if _, err := tx.Exec(ctx, `UPDATE document SET status='parsing', phase='parsing' WHERE id=$1`, docID); err != nil {
		return claimed{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return claimed{}, false, err
	}
	// The raw content for non-url sources is NOT stored in the DB in this design;
	// paste/file content is held only long enough to enqueue. For M2 we re-read
	// it from the document content store. SIMPLIFICATION: we store the raw bytes
	// inline so the worker can re-parse — see note below.
	c.raw = w.loadRaw(ctx, docID)
	return c, true, nil
}

// loadRaw retrieves the raw bytes to parse. Content is stored in the document
// row's content column (added below). For url sources raw is empty (the URL is
// in sourceRef).
func (w *Worker) loadRaw(ctx context.Context, docID string) []byte {
	var raw []byte
	_ = w.cfg.Pool.QueryRow(ctx, `SELECT content FROM document WHERE id=$1`, docID).Scan(&raw)
	return raw
}

// process parses + imports the claimed job and transitions state. On failure it
// reschedules with backoff or marks dead past MaxAttempts.
func (w *Worker) process(ctx context.Context, c claimed) {
	deps := parseDeps{fetcher: w.cfg.Fetcher, parseTimeout: w.cfg.ParseTimeout}
	text, _, perr := parseSource(ctx, deps, c.sourceType, c.raw, c.sourceRef)
	if perr == nil {
		w.setPhase(ctx, c.docID, c.jobID, "indexing")
		doc := makeDocument(c.docID, c.kbID, c.sourceType, c.title, text)
		var res ragingest.ImportResult
		res, perr = w.cfg.Rag.Import(ctx, []ragingest.Document{doc}, ragingest.ImportOptions{
			Namespace: c.namespace, ReplaceSource: true,
		})
		if perr == nil {
			// Persist checksum so the dedup short-circuit (Enqueue) can match;
			// null out content so the raw bytes are not retained after indexing
			// (content_bytes still drives the quota; content is only needed for
			// the async parse/retry window).
			_, _ = w.cfg.Pool.Exec(ctx,
				`UPDATE document SET status='ready', phase='ready', chunk_count=$2, checksum=$3, error='', content=NULL WHERE id=$1`,
				c.docID, res.Chunks, checksum(text))
			_, _ = w.cfg.Pool.Exec(ctx,
				`UPDATE ingest_job SET state='done', phase='ready', locked_by='', locked_until=NULL, updated_at=now() WHERE id=$1`,
				c.jobID)
			return
		}
	}
	w.fail(ctx, c, perr)
}

func (w *Worker) setPhase(ctx context.Context, docID, jobID, phase string) {
	_, _ = w.cfg.Pool.Exec(ctx, `UPDATE document SET phase=$2 WHERE id=$1`, docID, phase)
	_, _ = w.cfg.Pool.Exec(ctx, `UPDATE ingest_job SET phase=$2, updated_at=now() WHERE id=$1`, jobID, phase)
}

// fail either reschedules the job with exponential backoff (attempts < max) or
// marks it dead (and the document failed). Backoff uses the injected clock.
func (w *Worker) fail(ctx context.Context, c claimed, cause error) {
	msg := "unknown error"
	if cause != nil {
		msg = cause.Error()
	}
	if c.attempts >= w.cfg.MaxAttempts {
		_, _ = w.cfg.Pool.Exec(ctx,
			`UPDATE ingest_job SET state='dead', last_error=$2, locked_by='', locked_until=NULL, updated_at=now() WHERE id=$1`,
			c.jobID, msg)
		_, _ = w.cfg.Pool.Exec(ctx,
			`UPDATE document SET status='failed', phase='failed', error=$2 WHERE id=$1`, c.docID, msg)
		return
	}
	backoff := w.cfg.BaseBackoff * (1 << (c.attempts - 1)) // base * 2^(attempts-1)
	nextRun := w.cfg.Clock().Add(backoff)
	_, _ = w.cfg.Pool.Exec(ctx,
		`UPDATE ingest_job SET state='pending', next_run_at=$2, last_error=$3, locked_by='', locked_until=NULL, updated_at=now() WHERE id=$1`,
		c.jobID, nextRun, msg)
	_, _ = w.cfg.Pool.Exec(ctx,
		`UPDATE document SET status='pending', phase='retry-scheduled', error=$2 WHERE id=$1`, c.docID, msg)
}

```
> **Design note (raw content storage):** `Enqueue`/the worker need the raw bytes to parse *later* (async). The M2 approach stores the uploaded bytes in the `document.content BYTEA` column (added by Task 2's migration + `businessMigrationsForTest`) written at enqueue time and read by `loadRaw`; the worker re-parses from it. The `Enqueue` INSERT above already writes the `content` column = `in.Raw`. (For URL sources `content` is empty; the URL is in `source_ref`.)

- [ ] **Step 4: Run test to verify it passes**

Run (gated DB up): `cd llm-agent-kb && GOWORK=off go test ./internal/ingest/ -v`
Expected: PASS — `TestEnqueueWritesDocumentAndJob`, `TestWorkerRunOnceProcessesToReady`, `TestWorkerRunOnceEmptyQueue`, `TestWorkerRetryThenDead`, `TestWorkerReclaimsStuckLease`, plus the pure parse tests. Without the DB, the worker tests SKIP and the parse tests still PASS. Also run `GOWORK=off go vet ./internal/ingest/` (clean — `worker.go` imports only `context`/`log/slog`/`time`/pgx/pgxpool/ragingest/fetch/ragsvc, all used).

- [ ] **Step 5: Commit**

```bash
cd llm-agent-kb && git add internal/ingest/ && \
git commit -m "feat(ingest): async worker (FOR UPDATE SKIP LOCKED claim + lease/stuck-reclaim + backoff retry + dead terminal) + Enqueue/ListDocuments/Retry/dedup short-circuit + document.content column"
```

---

## Task 9: httpapi — 202 enqueue + documents list + progress SSE + retry

**Files:**
- Change: `llm-agent-kb/internal/httpapi/httpapi.go` (widen `Ingester`, add `kbGetter`, add routes)
- Change: `llm-agent-kb/internal/httpapi/handlers.go` (uploadHandler→202; add list/progress/retry handlers)
- Change: `llm-agent-kb/internal/httpapi/httpapi_test.go` (DB-free behavioral tests for the upload rejection paths via a fake `kbGetter` + fake `Ingester`)

`POST /documents` now returns **202 + {documentId, status:"pending"}** (calls `Enqueue`, applies upload validation + per-kb quota). `GET /documents?limit=&cursor=` returns `{items, next_cursor}` (viewer+). `GET /documents/{docId}/progress` streams SSE status updates (viewer+). `POST /documents/{docId}/retry` re-enqueues (editor+). The handler tests call `uploadHandler` directly (it is package-internal) with a fake `kbGetter` + fake `Ingester`, so they need no DB and no network — validation/quota happen before any enqueue.

- [ ] **Step 1: Write the failing test**

Add to `internal/httpapi/httpapi_test.go` a fake Ingester, a fake kbGetter, and the rejection-path tests. First define the fakes near the top of the test file (if an M1 fake exists, extend it):
```go
type fakeIngester struct {
	enqueued ingest.IngestInput
	docs     []ingest.DocumentView
	retried  string
	usedBytes int64 // KBContentBytes returns this (for the quota test)
	enqErr   error  // optional Enqueue error
}

func (f *fakeIngester) Enqueue(ctx context.Context, in ingest.IngestInput) (string, error) {
	if f.enqErr != nil {
		return "", f.enqErr
	}
	f.enqueued = in
	return "doc-123", nil
}
func (f *fakeIngester) DeleteDocument(ctx context.Context, ns, id string) error { return nil }
func (f *fakeIngester) DeleteAllDocumentsForKB(ctx context.Context, ns, kbID string) error { return nil }
func (f *fakeIngester) ListDocuments(ctx context.Context, kbID string, limit int, cursor string) ([]ingest.DocumentView, string, error) {
	return f.docs, "", nil
}
func (f *fakeIngester) KBContentBytes(ctx context.Context, kbID string) (int64, error) { return f.usedBytes, nil }
func (f *fakeIngester) Retry(ctx context.Context, kbID, docID string) error { f.retried = docID; return nil }

var _ Ingester = (*fakeIngester)(nil)

// fakeKBGetter satisfies the kbGetter interface (Step 3) so uploadHandler can be
// driven without a DB. It returns a fixed kb (or ErrNotFound if id is "missing").
type fakeKBGetter struct{}

func (fakeKBGetter) Get(ctx context.Context, id string) (orgkb.KB, error) {
	if id == "missing" {
		return orgkb.KB{}, orgkb.ErrNotFound
	}
	return orgkb.KB{ID: id, Namespace: "ns_" + id}, nil
}

var _ kbGetter = fakeKBGetter{}

// newUploadRequest builds a *http.Request carrying {id} as a path value (the
// handler reads r.PathValue("id")) and the JSON upload body.
func newUploadRequest(kbID, jsonBody string) *http.Request {
	r := httptest.NewRequest("POST", "/api/kb/"+kbID+"/documents", strings.NewReader(jsonBody))
	r.SetPathValue("id", kbID)
	return r
}

// TestUploadRejectsDisallowedExtension: a pdf/docx upload whose filename has a
// disallowed extension is rejected with 415 before any enqueue.
func TestUploadRejectsDisallowedExtension(t *testing.T) {
	ing := &fakeIngester{}
	h := uploadHandler(fakeKBGetter{}, ing, 10<<20, 256<<20)
	rec := httptest.NewRecorder()
	h(rec, newUploadRequest("kb1", `{"title":"x","sourceType":"pdf","filename":"evil.exe","content":"AAAA"}`))
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("code=%d want 415", rec.Code)
	}
	if ing.enqueued.KBID != "" {
		t.Fatal("enqueue must not be called on a rejected upload")
	}
}

// TestUploadRejectsOverQuota: when used+incoming exceeds the kb quota, the
// handler returns 507 Insufficient Storage and does not enqueue.
func TestUploadRejectsOverQuota(t *testing.T) {
	ing := &fakeIngester{usedBytes: 256 << 20} // already at quota
	h := uploadHandler(fakeKBGetter{}, ing, 10<<20, 256<<20)
	rec := httptest.NewRecorder()
	h(rec, newUploadRequest("kb1", `{"title":"x","sourceType":"paste","content":"more bytes"}`))
	if rec.Code != http.StatusInsufficientStorage {
		t.Fatalf("code=%d want 507", rec.Code)
	}
	if ing.enqueued.KBID != "" {
		t.Fatal("enqueue must not be called when over quota")
	}
}

// TestUploadAcceptsPaste: a valid paste upload returns 202 + documentId and
// passes the parsed input to Enqueue.
func TestUploadAcceptsPaste(t *testing.T) {
	ing := &fakeIngester{}
	h := uploadHandler(fakeKBGetter{}, ing, 10<<20, 256<<20)
	rec := httptest.NewRecorder()
	h(rec, newUploadRequest("kb1", `{"title":"Doc","sourceType":"paste","content":"hello"}`))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("code=%d want 202", rec.Code)
	}
	if ing.enqueued.Title != "Doc" || string(ing.enqueued.Raw) != "hello" {
		t.Fatalf("enqueue got %+v", ing.enqueued)
	}
}
```
> Note: `uploadHandler` is package-internal so the same-package test calls it directly — no auth/mux needed for these focused checks (RBAC is exercised by the gated Task 10 e2e). The `kbGetter` interface (Step 3) is what makes the repo lookup DB-free here; `*orgkb.Repo` satisfies it in production.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/httpapi/ -v`
Expected: FAIL to compile — undefined `kbGetter`; `*fakeIngester` does not satisfy `Ingester` (interface not yet widened); `uploadHandler` arity wrong (no `kbGetter`/quota args yet).

- [ ] **Step 3: Write the implementation**

In `internal/httpapi/httpapi.go`, widen the `Ingester` interface (replace `Ingest` with `Enqueue` + add the new methods) and add a narrow `kbGetter` so the M2 document handlers can be unit-tested DB-free (`*orgkb.Repo` satisfies it):
```go
type Ingester interface {
	Enqueue(ctx context.Context, in ingest.IngestInput) (string, error)
	DeleteDocument(ctx context.Context, namespace, documentID string) error
	DeleteAllDocumentsForKB(ctx context.Context, namespace, kbID string) error
	ListDocuments(ctx context.Context, kbID string, limit int, cursor string) ([]ingest.DocumentView, string, error)
	KBContentBytes(ctx context.Context, kbID string) (int64, error)
	Retry(ctx context.Context, kbID, docID string) error
}

// kbGetter is the slice of *orgkb.Repo the document handlers need (just Get).
// Narrowing to an interface lets the handler tests inject a DB-free fake.
type kbGetter interface {
	Get(ctx context.Context, id string) (orgkb.KB, error)
}
```
Add the new routes inside the `if d.Ingester != nil && d.KBRepo != nil {` block in `NewMux` (and pass the quota into the upload handler via a new Deps field):
```go
		mux.Handle("POST /api/kb/{id}/documents", chain(authzrole.RoleEditor, uploadHandler(d.KBRepo, d.Ingester, d.MaxUploadBytes, d.KBStorageQuotaBytes)))
		mux.Handle("GET /api/kb/{id}/documents", chain(authzrole.RoleViewer, listDocsHandler(d.KBRepo, d.Ingester)))
		mux.Handle("GET /api/kb/{id}/documents/{docId}/progress", chain(authzrole.RoleViewer, progressHandler(d.KBRepo, d.DocStatus)))
		mux.Handle("POST /api/kb/{id}/documents/{docId}/retry", chain(authzrole.RoleEditor, retryHandler(d.KBRepo, d.Ingester)))
		mux.Handle("DELETE /api/kb/{id}/documents/{docId}", chain(authzrole.RoleEditor, deleteDocHandler(d.KBRepo, d.Ingester)))
```
Add to `Deps`:
```go
	MaxUploadBytes      int64
	KBStorageQuotaBytes int64
	DocStatus           DocStatusReader // reads document status for SSE; satisfied by *ingest.Service
```
And a small interface for SSE polling (so the SSE handler does not need the full Ingester):
```go
// DocStatusReader reads a document's status+phase for progress streaming.
type DocStatusReader interface {
	DocumentStatus(ctx context.Context, kbID, docID string) (status, phase string, chunkCount int, errMsg string, err error)
}
```
Add `DocumentStatus` to `internal/ingest/ingest.go`:
```go
// DocumentStatus reads a single document's status/phase for progress SSE.
func (s *Service) DocumentStatus(ctx context.Context, kbID, docID string) (string, string, int, string, error) {
	var status, phase, errMsg string
	var cc int
	err := s.pool.QueryRow(ctx,
		`SELECT status, phase, chunk_count, error FROM document WHERE id=$1 AND kb_id=$2`,
		docID, kbID).Scan(&status, &phase, &cc, &errMsg)
	return status, phase, cc, errMsg, err
}
```
In `internal/httpapi/handlers.go`, rewrite `uploadHandler` to enqueue (202) with validation + quota, and add the new handlers:
```go
func uploadHandler(repo kbGetter, ing Ingester, maxUpload, quota int64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		kb, err := repo.Get(r.Context(), r.PathValue("id"))
		if errors.Is(err, orgkb.ErrNotFound) {
			http.Error(w, "kb not found", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		var req struct {
			Title      string `json:"title"`
			SourceType string `json:"sourceType"`
			Content    string `json:"content"`
			URL        string `json:"url"`      // for sourceType=url
			Filename   string `json:"filename"` // for file uploads (extension allowlist)
		}
		body := http.MaxBytesReader(w, r.Body, maxUpload)
		raw, err := io.ReadAll(body)
		if err != nil {
			http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
			return
		}
		if err := json.Unmarshal(raw, &req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		st := ingest.SourceType(req.SourceType)
		content := []byte(req.Content)
		sourceRef := ""
		switch st {
		case ingest.SourceTypeURL:
			sourceRef = req.URL
			if sourceRef == "" {
				http.Error(w, "url required for sourceType=url", http.StatusBadRequest)
				return
			}
		case ingest.SourceTypePDF, ingest.SourceTypeDOCX:
			// File bytes arrive base64-decoded by the client into Content; validate
			// the declared filename's extension + size.
			if err := ingest.ValidateUpload(req.Filename, int64(len(content)), maxUpload); err != nil {
				http.Error(w, err.Error(), http.StatusUnsupportedMediaType)
				return
			}
			sourceRef = req.Filename
		}
		// Per-kb storage quota (§16.3).
		if used, err := ing.KBContentBytes(r.Context(), kb.ID); err == nil && used+int64(len(content)) > quota {
			http.Error(w, "kb storage quota exceeded", http.StatusInsufficientStorage)
			return
		}
		docID, err := ing.Enqueue(r.Context(), ingest.IngestInput{
			KBID: kb.ID, Namespace: kb.Namespace, Title: req.Title,
			SourceType: st, SourceRef: sourceRef, Raw: content,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"documentId": docID, "status": "pending"})
	}
}

func listDocsHandler(repo kbGetter, ing Ingester) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		kb, err := repo.Get(r.Context(), r.PathValue("id"))
		if errors.Is(err, orgkb.ErrNotFound) {
			http.Error(w, "kb not found", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		limit := 0
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				limit = n
			}
		}
		items, next, err := ing.ListDocuments(r.Context(), kb.ID, limit, r.URL.Query().Get("cursor"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		out := make([]ingest.DocumentView, 0, len(items))
		out = append(out, items...)
		writeJSON(w, http.StatusOK, map[string]any{"items": out, "next_cursor": next})
	}
}

func retryHandler(repo kbGetter, ing Ingester) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		kb, err := repo.Get(r.Context(), r.PathValue("id"))
		if errors.Is(err, orgkb.ErrNotFound) {
			http.Error(w, "kb not found", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if err := ing.Retry(r.Context(), kb.ID, r.PathValue("docId")); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"documentId": r.PathValue("docId"), "status": "pending"})
	}
}

// progressHandler streams document status as Server-Sent Events until the
// document reaches a terminal state (ready/failed) or the client disconnects.
func progressHandler(repo kbGetter, reader DocStatusReader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		kb, err := repo.Get(r.Context(), r.PathValue("id"))
		if errors.Is(err, orgkb.ErrNotFound) {
			http.Error(w, "kb not found", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		docID := r.PathValue("docId")
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		emit := func() (terminal bool) {
			status, phase, cc, errMsg, err := reader.DocumentStatus(r.Context(), kb.ID, docID)
			if err != nil {
				_, _ = io.WriteString(w, "event: error\ndata: {\"error\":\"not found\"}\n\n")
				flusher.Flush()
				return true
			}
			payload, _ := json.Marshal(map[string]any{"status": status, "phase": phase, "chunkCount": cc, "error": errMsg})
			_, _ = io.WriteString(w, "data: "+string(payload)+"\n\n")
			flusher.Flush()
			return status == "ready" || status == "failed"
		}
		if emit() {
			return
		}
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				if emit() {
					return
				}
			}
		}
	}
}
```
`ingest.ValidateUpload(filename string, size, maxBytes int64) error` already exists (defined exported with this final signature in Task 7's `parse.go`); the `uploadHandler` above calls it as-is. Do NOT re-signature it or rewrite its test here — Task 9 only USES it.
Add the needed imports to `handlers.go`: `"time"` (for the SSE ticker). Add `"github.com/costa92/llm-agent-kb/internal/ingest"` to `httpapi_test.go` (the fakes reference `ingest.IngestInput`/`ingest.DocumentView`); `context`/`net/http`/`httptest`/`strings`/`orgkb` are already imported by the M1 test file.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd llm-agent-kb && GOWORK=off go test ./internal/httpapi/ ./internal/ingest/ -v && GOWORK=off go build ./...`
Expected: PASS — httpapi compiles, `var _ Ingester = (*fakeIngester)(nil)` and `var _ kbGetter = fakeKBGetter{}` hold, and the three upload tests (415 disallowed-extension, 507 over-quota, 202 accept) pass DB-free; ingest parse tests still pass with `ValidateUpload`. Build clean.

- [ ] **Step 5: Commit**

```bash
cd llm-agent-kb && git add internal/httpapi/ internal/ingest/ && \
git commit -m "feat(httpapi): POST /documents → 202 enqueue (upload validation + per-kb quota) + GET /documents (keyset page) + progress SSE + retry; widen Ingester; export ValidateUpload + DocumentStatus"
```

---

## Task 10: cmd/kbd — start + drain worker pool; async e2e smoke test

**Files:**
- Change: `llm-agent-kb/cmd/kbd/main.go`
- Change: `llm-agent-kb/cmd/kbd/main_test.go`

Wire a `*fetch.Fetcher` + start `cfg.IngestWorkers` `Worker.Run` goroutines in `build`; drain them on shutdown by canceling a worker context BEFORE `srv.Shutdown`. The e2e test uploads a paste doc (now 202), polls `GET /documents` until `ready`, then asks and asserts a citation — proving the async pipeline end-to-end.

- [ ] **Step 1: Write the failing test**

Update `cmd/kbd/main_test.go`. Add `"ingest_job"` to the FRONT of `cleanDB`'s drop list. Replace the M1 synchronous upload+ask block (steps 4-5) with the async flow:
```go
	// 4. POST /api/kb/{id}/documents (paste) → 202 + documentId.
	code, body = do("POST", "/api/kb/"+kbID+"/documents", token,
		`{"title":"Doc","sourceType":"paste","content":"the quick brown fox jumps over the lazy dog repeatedly"}`)
	if code != http.StatusAccepted {
		t.Fatalf("upload code=%d body=%v want 202", code, body)
	}
	docID, _ := body["documentId"].(string)
	if docID == "" {
		t.Fatalf("upload returned no documentId: %v", body)
	}

	// 4b. Poll GET /api/kb/{id}/documents until the doc is ready (worker drains async).
	// The existing `do` closure already decodes JSON, so it handles the list
	// envelope {items, next_cursor} directly — no separate helper needed.
	ready := false
	for i := 0; i < 50; i++ {
		code, listBody := do("GET", "/api/kb/"+kbID+"/documents", token, "")
		if code == http.StatusOK && hasReadyDoc(listBody, docID) {
			ready = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ready {
		t.Fatalf("document %s never reached ready", docID)
	}

	// 5. POST /api/kb/{id}/ask (hybrid) → answer + ≥1 citation.
	code, body = do("POST", "/api/kb/"+kbID+"/ask", token, `{"q":"fox","mode":"hybrid","topK":5}`)
	if code != http.StatusOK {
		t.Fatalf("ask code=%d body=%v want 200", code, body)
	}
	cites, _ := body["citations"].([]any)
	if len(cites) == 0 {
		t.Fatalf("ask returned 0 citations: %v", body)
	}
```
Add the `hasReadyDoc` package-level helper (and the `"time"` import for the poll sleep):
```go
func hasReadyDoc(listBody map[string]any, docID string) bool {
	items, _ := listBody["items"].([]any)
	for _, it := range items {
		m, _ := it.(map[string]any)
		if m["id"] == docID && m["status"] == "ready" {
			return true
		}
	}
	return false
}
```
The list polling reuses the existing `do` closure with a GET and empty body (it already decodes the JSON envelope), so no separate `doList` helper is needed.

- [ ] **Step 2: Run test to verify it fails**

Run (gated DB up): `cd llm-agent-kb && GOWORK=off go test ./cmd/kbd/ -v`
Expected: FAIL — upload returns 202 but no worker is running, so the doc never reaches `ready` (the poll times out) — proving the worker must be started in `build`.

- [ ] **Step 3: Wire the worker pool in `build`**

In `cmd/kbd/main.go`, add the fetcher + worker startup. Change `build` to also return a worker-stop function (fold it into `cleanup`). Add imports `"sync"`, `"time"`, `"github.com/costa92/llm-agent-kb/internal/fetch"`. After `ingestSvc := ingest.New(...)`:
```go
	fetcher := fetch.New(fetch.Config{
		Timeout:             cfg.FetchTimeout,
		MaxBytes:            cfg.FetchMaxBytes,
		AllowedContentTypes: []string{"text/html", "application/xhtml+xml", "text/plain"},
	})

	workerCtx, stopWorkers := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	for i := 0; i < cfg.IngestWorkers; i++ {
		w := ingest.NewWorker(ingest.WorkerConfig{
			Pool: st.Pool(), Rag: rag, Fetcher: fetcher,
			WorkerID:     fmt.Sprintf("kbd-%d", i),
			Lease:        cfg.IngestLease,
			MaxAttempts:  cfg.IngestMaxAttempts,
			BaseBackoff:  cfg.IngestBaseBackoff,
			ParseTimeout: cfg.ParseTimeout,
		})
		wg.Add(1)
		go func() { defer wg.Done(); w.Run(workerCtx, cfg.IngestPollInterval) }()
	}
```
Pass the new Deps into `NewMux`:
```go
	mux := httpapi.NewMux(httpapi.Deps{
		Issuer:              issuer,
		AuthHandlers:        authHandlers,
		RoleResolver:        az,
		OrgLookup:           kbRepo,
		Asker:               retrievalSvc,
		Ingester:            ingestSvc,
		KBRepo:              kbRepo,
		PerUserLimit:        cfg.MaxRequestsPerUserPerMinute,
		MaxUploadBytes:      cfg.MaxUploadBytes,
		KBStorageQuotaBytes: cfg.KBStorageQuotaBytes,
		DocStatus:           ingestSvc,
	})
```
And extend `cleanup` to drain workers FIRST:
```go
	cleanup := func() {
		stopWorkers()   // signal workers to stop claiming
		wg.Wait()       // drain in-flight jobs (lease protects un-drained ones)
		_ = tp.Shutdown(ctx)
		st.Close()
	}
```
Add the `"fmt"` import. (The smoke test drives `build` directly and calls `cleanup`, so workers start when the handler is built and stop on `defer cleanup()`.)

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
# bring up the from-source pgvector DB (see "Gated test DB" above), export LLM_AGENT_KB_PG_URL
cd llm-agent-kb && GOWORK=off go build ./... && GOWORK=off go vet ./...
GOWORK=off go test ./... && echo ALL_GREEN
```
Expected: build+vet clean; with PG up, `ALL_GREEN` — the async upload reaches `ready` via the live worker, ask returns a citation. Pure suites (config/fetch/ingest-parse/httpapi) pass without PG; storage/worker/e2e SKIP without PG.

- [ ] **Step 5: Commit**

```bash
cd llm-agent-kb && git add cmd/ && \
git commit -m "feat(cmd): start ingest worker pool + SSRF fetcher in build; drain workers on shutdown; async e2e smoke (upload 202 → poll ready → ask)"
```

---

## Task 11: docker-compose — add otel collector (M1 follow-up)

**Files:**
- Change: `llm-agent-kb/docker-compose.yml`

Fold in the otel collector service (M1 review follow-up) so traces have a sink. Reference the ecosystem `llm-agent-otel/compose/` collector config.

- [ ] **Step 1: Add the collector service**

In `docker-compose.yml`, add an `otel-collector` service and point `kbd` at it. Add under `services:`:
```yaml
  otel-collector:
    image: otel/opentelemetry-collector-contrib:latest
    command: ["--config=/etc/otelcol/config.yaml"]
    volumes:
      - ./deploy/otel-collector.yaml:/etc/otelcol/config.yaml:ro
    ports:
      - "4317:4317"   # OTLP gRPC
      - "4318:4318"   # OTLP HTTP
```
Add `otel-collector` to `kbd.depends_on` and set `kbd`'s `OTEL_EXPORTER_OTLP_ENDPOINT: "http://otel-collector:4318"`.

- [ ] **Step 2: Add the collector config**

Create `deploy/otel-collector.yaml` (minimal logging exporter so traces are visible in `docker compose logs otel-collector`):
```yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318
processors:
  batch: {}
exporters:
  debug:
    verbosity: detailed
service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [batch]
      exporters: [debug]
```

- [ ] **Step 3: Verify compose config parses**

Run: `cd llm-agent-kb && docker compose config >/dev/null && echo COMPOSE_OK`
Expected: `COMPOSE_OK` (no YAML/schema errors).

- [ ] **Step 4: Commit**

```bash
cd llm-agent-kb && git add docker-compose.yml deploy/otel-collector.yaml && \
git commit -m "ops(compose): add otel collector (OTLP gRPC/HTTP + debug exporter) and point kbd at it (M1 follow-up)"
```

---

## Task 12: Final verification + push

- [ ] **Step 1: Full build + vet + pure suites**

```bash
cd llm-agent-kb && GOWORK=off go build ./... && GOWORK=off go vet ./... && \
GOWORK=off go test ./internal/config/ ./internal/fetch/ ./internal/ingest/ -run 'TestParse|TestValidate|TestIsBlocked|TestFetch|TestLoad' && echo PURE_GREEN
```
Expected: build+vet clean; `PURE_GREEN`.

- [ ] **Step 2: Full gated suite on the from-source pgvector DB**

```bash
docker run -d --name kb-m2-pg -e POSTGRES_PASSWORD=pw postgres:16-alpine
docker exec -u root kb-m2-pg sh -c 'apk add --no-cache build-base clang19 llvm19-dev git && cd /tmp && git clone --depth 1 --branch v0.8.0 https://github.com/pgvector/pgvector && cd pgvector && make OPTFLAGS="" install'
IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' kb-m2-pg)
export LLM_AGENT_KB_PG_URL="postgres://postgres:pw@$IP:5432/postgres?sslmode=disable"
cd llm-agent-kb && GOWORK=off go test ./... && echo ALL_GREEN
docker rm -f kb-m2-pg
```
Expected: `ALL_GREEN` — every gated suite (storage / worker / cmd e2e) passes; pure suites pass.

- [ ] **Step 3: Push the branch + open PR**

```bash
cd llm-agent-kb && git push -u origin feat/m2-ingest-expansion
gh pr create --title "M2: ingest expansion (PDF/DOCX/URL + async worker + SSRF/upload + progress/retry/dedup/pagination)" \
  --body "Implements §13 M2 + §16.3. See docs/superpowers/plans/2026-06-09-llm-agent-kb-m2.md."
```
> Note: the replace-guard pre-commit hook strips any local `replace` and pins published tags; the three new deps are non-`costa92` and are left untouched. If `gh pr edit/view` fails (token lacks `read:org`), edit the PR via `gh api -X PATCH repos/.../pulls/N`.

---

## Self-Review

**Spec §13 M2 coverage:**
- **PDF parsing** → Task 4 `parsePDF` via `ledongthuc/pdf` (`NewReader`+`GetPlainText`), verified API; dispatched in Task 7 `parseSource` under a parse-timeout ✓
- **DOCX parsing** → Task 5 `parseDOCX` via `fumiama/go-docx` (`Parse` + Body→Paragraph→Run→Text walk), verified API ✓
- **URL parsing** → Task 6 `parseURL` = `internal/fetch` SSRF-safe Get + `go-readability.FromReader` → text; verified API ✓
- **SSRF protection (§16.3)** → Task 3 `internal/fetch`: scheme allowlist (`validateScheme`); resolve-then-validate-IP (`resolveAndValidate`); **dial the resolved IP** via custom `DialContext` (no DNS rebinding); per-hop redirect re-validation (`CheckRedirect` + DialContext re-runs on each new conn); connect/read timeouts; `io.LimitReader` body cap; Content-Type allowlist. Heavy IP-matrix unit test (16 classes incl. loopback/private/link-local/`169.254.169.254`/multicast/unspecified/CGNAT/unique-local v6 rejected; public v4/v6 allowed) via injected resolver ✓
- **Upload validation (§16.3)** → `http.MaxBytesReader` cap (uploadHandler); extension allowlist pdf/md/txt/docx (`ValidateUpload`); per-kb storage quota (`KBContentBytes` + 507 on over-quota); parse timeout (`withParseTimeout` around PDF/DOCX) ✓
- **Async worker (replaces M1 sync ingest)** → `ingest_job` table (Task 2, §5 columns); `POST /documents` writes document(pending)+job, returns **202+documentId** (Task 9 `uploadHandler` + Task 8 `Enqueue`); in-process worker pool claims with `SELECT … FOR UPDATE SKIP LOCKED` + `locked_until` lease (Task 8 `claim`); parse→Import→status transitions (`process`); exponential backoff retry (`fail`: `next_run_at = clock()+base*2^(attempts-1)`); stuck recovery (claim's `state='running' AND locked_until<now()` branch, tested by `TestWorkerReclaimsStuckLease`); `dead` terminal past `MaxAttempts` (`TestWorkerRetryThenDead`); manual retry `POST .../retry` editor+ (Task 9 `retryHandler` + Task 8 `Retry`); worker started + drained in `cmd/kbd` (Task 10 `stopWorkers`+`wg.Wait`). §16.4 delete cascade preserved — `ingest_job.document_id REFERENCES document ON DELETE CASCADE`, M1 `delete.go` unchanged. Worker tests deterministic: injected `Clock` for backoff scheduling, DB `now()` for race-free claim, `RunOnce` (no real sleeps); each gated test builds its OWN fresh DB ✓
- **Progress** → worker updates `document.status`+`phase` (`setPhase`, `process`); `GET /documents/{docId}/progress` SSE viewer+ (Task 9 `progressHandler`, terminal on ready/failed) ✓
- **Reimport/dedup** → checksum short-circuit in `Enqueue` (same-source key = `(kb_id, source_ref)` for url/file else `(kb_id, title)`; identical `sha256` checksum on a `ready` doc → return existing id, skip re-Import); else worker Imports with `ReplaceSource:true`; checksum re-persisted post-Import so future dedup matches ✓
- **Documents list pagination** → `GET /documents?limit=&cursor=` → `{items, next_cursor}` keyset by id, mirroring `orgkb.ListByOrg` (Task 8 `ListDocuments`, Task 9 `listDocsHandler`) ✓
- **otel collector** → Task 11 (M1 follow-up folded in) ✓

**§16.3 bullet-by-bullet:** scheme allowlist ✓; DNS-resolved-IP private/loopback/link-local/metadata block + dial-resolved-IP (rebinding) ✓; redirect re-validation per hop ✓; connect/read timeouts ✓; max body bytes ✓; response MIME allowlist ✓; MaxBytesReader ✓; content-type/extension allowlist ✓; per-kb quota ✓; parse timeout ✓.

**Grounded in live code (not assumed):** extends M1 `ingest.makeDocument`/`checksum`/`newID`/`Service`/`IngestInput`/`Result`, `storage.businessMigrations`/`Migrate`, `httpapi.NewMux`/`Ingester`/`Deps`/`chain`, `cmd/kbd.build` graceful-shutdown shape, `ragsvc.RagPort.Import` (`ImportResult{Chunks}`), `orgkb.ListByOrg` keyset pattern. External APIs inspected at the pinned pseudo-versions in the module cache (PDF/DOCX/readability signatures shown above are exact). M1 sync `Ingest` is intentionally replaced by `Enqueue`; `delete.go` and ask/citation/kb-CRUD untouched.

**Decisions made (ambiguities resolved):**
1. **PDF lib = `ledongthuc/pdf`** (over `dslipak/pdf`): both resolve via proxy, but `ledongthuc` exposes a clean `NewReader(io.ReaderAt,size)+GetPlainText()` byte-stream API (no temp file), verified in cache. `dslipak` only has semver `v0.0.2` but the same author-forked API; `ledongthuc` is the more-maintained upstream.
2. **Raw content storage** = new `document.content BYTEA` column (created by Task 2's migration + `businessMigrationsForTest`) written at enqueue, read by the worker (`loadRaw`), and nulled after `status='ready'` so raw bytes are not retained. Async parsing happens after the HTTP request returns, so the bytes must be persisted; storing inline in the document row is the simplest M2 design (no object store).
3. **Claim race-safety** uses the DB `now()` (server-side, race-free across workers) for the lease/staleness comparison; the **injected clock** is used only for backoff `next_run_at`. This keeps reclaim deterministic against the DB while making backoff testable without sleeps.
4. **Dedup same-source key** = `(kb_id, source_ref)` when a source_ref exists (url/file), else `(kb_id, title)` (paste). Only a `ready` doc with an identical checksum short-circuits; this is the §6 "content未变则跳过 Import" rule, never affecting `ReplaceSource`.
5. **SSE over polling**: `GET .../progress` is SSE per §16.2; the `cmd` e2e test polls `GET /documents` (simpler, deterministic) rather than consuming SSE — SSE correctness is a focused concern, the async pipeline is what the e2e proves.
6. **httpapi unit tests stay DB-free** via a narrow `kbGetter` interface (just `Get`, satisfied by `*orgkb.Repo`) + a fake `Ingester`: the upload-rejection paths (415 disallowed extension, 507 over-quota) and the 202 happy path are tested behaviorally by calling `uploadHandler` directly (no auth/mux/DB). The full RBAC-gated 202/list/retry/progress flow is additionally covered by the gated `cmd/kbd` e2e.

**New deps pinned (verified resolvable via proxy):** `github.com/ledongthuc/pdf v0.0.0-20250511090121-5959a4027728`; `github.com/fumiama/go-docx v0.0.0-20250506085032-0c30fd09304b`; `github.com/go-shiori/go-readability v0.0.0-20251205110129-5db1dc9836f0`.

**Gating:** all DB-touching suites use `LLM_AGENT_KB_PG_URL`+`t.Skipf`; each gated test sets up its OWN fresh DB (drops its tables, migrates) — not co-runnable on a shared DB by design. Pure suites (config/fetch/ingest-parse/httpapi-compile) run always. Live DB is pgvector built from source into `postgres:16-alpine` per constraints.

**Placeholder scan:** every code step shows actual, compiling code. No placeholders, no TBDs: the `cmd` e2e poll reuses the existing `do` closure (no `doList` stub); Task 9's upload-rejection tests (415/507) are full DB-free behavioral tests against a fake `kbGetter` + fake `Ingester`; the `leaseSeconds` helper is removed (claim uses the inline `($3 || ' seconds')` cast, so `worker.go` carries no unused symbols).

**Boundaries:** `ragsvc` remains the sole importer of rag/postgres/otelrag; `ingest` imports only `ragsvc.RagPort` + pool + `internal/fetch`; `internal/fetch` imports no kb-internal package; `httpapi` holds no business rules (validation lives in `ingest.ValidateUpload`; quota check calls `ing.KBContentBytes`). No import cycles.
