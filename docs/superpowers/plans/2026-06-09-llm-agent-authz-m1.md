# llm-agent-authz M1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `llm-agent-authz` v0.1.0 — a shared, importable Go library providing org→scope tenancy, argon2id passwords, JWT access tokens, refresh-token sessions, and `Authenticate`/`RequireScopeRole` middleware, so `llm-agent-kb` (and later `llm-agent-studio`) can depend on a single, tested auth substrate.

**Architecture:** A standalone sibling repo (`github.com/costa92/llm-agent-authz`, gitignored from the umbrella). Layered, single-responsibility packages: `role` (pure RBAC algebra), `password` (argon2id), `token` (JWT), `store` (pgx-backed users/orgs/memberships/sessions + versioned migrations), `service` (login/refresh/logout orchestration), `httpapi` (handlers + middleware). Pure packages have no DB and are unit-tested directly; `store`/integration tests hit a live Postgres via `LLM_AGENT_AUTHZ_PG_URL` (skipped when unset), matching `llm-agent-memory-postgres` conventions.

**Tech Stack:** Go 1.26.0 · `github.com/jackc/pgx/v5` (pgxpool) · `github.com/golang-jwt/jwt/v5` · `golang.org/x/crypto/argon2` · stdlib `net/http`, `crypto/rand`, `crypto/sha256`, `crypto/subtle`.

**Spec:** `docs/superpowers/specs/2026-06-09-llm-agent-authz-design.md` (this plan implements its M1).

**Conventions verified in ecosystem:** Go 1.26.0; pgx/v5 v5.9.2 + pgxpool; migrations as Go-coded versioned `migrationGroup` bundles with a `Migrate(ctx)` method (see `llm-agent-memory-postgres/postgres/schema.go`); live-PG tests gated by an env DSN with `t.Skipf` (see `llm-agent-memory-postgres/postgres/schema_test.go`). Standalone sibling → Go commands need `GOWORK=off` unless added to the umbrella `go.work`.

---

## File Structure

```
llm-agent-authz/                      module github.com/costa92/llm-agent-authz
├── go.mod
├── role/role.go            role/role_test.go          # Role enum + Merge/AtLeast (pure)
├── password/password.go    password/password_test.go  # argon2id Hash/Verify (pure)
├── token/token.go          token/token_test.go        # JWT Issuer.Issue/Verify (pure)
├── store/schema.go                                     # versioned migrations + Migrate
├── store/store.go          store/store_test.go         # Store{pool}, New, test harness
├── store/users.go                                      # CreateUser/GetUserByEmail/CreateOrg
├── store/memberships.go                                # UpsertMembership/ResolveRole
├── store/sessions.go                                   # session CRUD + rotate/revoke
├── service/service.go      service/service_test.go     # Authenticator, Login/Refresh/Logout
├── httpapi/middleware.go                               # Authenticate, RequireScopeRole
├── httpapi/handlers.go     httpapi/httpapi_test.go     # /api/auth/* handlers + Mount
└── cmd/authz-migrate/main.go                           # standalone migrate command
```

Boundaries: `role`/`password`/`token` are pure (no imports of `store`); `store` imports `role`; `service` imports `store`+`token`+`password`; `httpapi` imports `service`+`store`+`token`+`role`. No cycles.

---

## Task 1: Repo bootstrap

**Files:**
- Create: `llm-agent-authz/go.mod`
- Create: `llm-agent-authz/doc.go`

- [ ] **Step 1: Create the repo directory and module**

Run (from the umbrella root):
```bash
mkdir -p llm-agent-authz && cd llm-agent-authz && GOWORK=off go mod init github.com/costa92/llm-agent-authz
```

- [ ] **Step 2: Pin Go version and add dependencies**

Edit `llm-agent-authz/go.mod` so the `go` line reads `go 1.26.0`, then run:
```bash
cd llm-agent-authz && GOWORK=off go get github.com/jackc/pgx/v5@v5.9.2 github.com/golang-jwt/jwt/v5@latest golang.org/x/crypto@latest
```
Expected: `go.mod` now requires pgx/v5, golang-jwt/jwt/v5, golang.org/x/crypto.

- [ ] **Step 3: Add a package doc file**

Create `llm-agent-authz/doc.go`:
```go
// Package authz is the umbrella module for llm-agent-authz: a shared,
// importable tenancy/auth library (org -> parameterized scope) providing
// argon2id passwords, JWT access tokens, refresh-token sessions, and
// Authenticate / RequireScopeRole HTTP middleware. It is a library, not a
// service: consuming services run its migrations, mount its handlers, and
// wrap routes with its middleware.
package authz
```

- [ ] **Step 4: Verify it builds**

Run: `cd llm-agent-authz && GOWORK=off go build ./...`
Expected: success, no output.

- [ ] **Step 5: Commit**

```bash
cd llm-agent-authz && git init && git add go.mod go.sum doc.go && \
git commit -m "chore: bootstrap llm-agent-authz module (go 1.26, pgx, jwt, x/crypto)"
```

---

## Task 2: Role algebra (`role` package, pure)

**Files:**
- Create: `llm-agent-authz/role/role.go`
- Test: `llm-agent-authz/role/role_test.go`

Implements the spec §2 merge rule: effective role = highest of org-level ⊕ scope-level, ordering `org_admin > admin > editor > viewer`.

- [ ] **Step 1: Write the failing test**

Create `role/role_test.go`:
```go
package role

import "testing"

func TestRankOrdering(t *testing.T) {
	if !(RoleOrgAdmin.Rank() > RoleAdmin.Rank() &&
		RoleAdmin.Rank() > RoleEditor.Rank() &&
		RoleEditor.Rank() > RoleViewer.Rank()) {
		t.Fatalf("rank ordering wrong: orgAdmin=%d admin=%d editor=%d viewer=%d",
			RoleOrgAdmin.Rank(), RoleAdmin.Rank(), RoleEditor.Rank(), RoleViewer.Rank())
	}
}

func TestMergeTakesHighest(t *testing.T) {
	if got := Merge(RoleViewer, RoleAdmin); got != RoleAdmin {
		t.Fatalf("Merge(viewer,admin)=%q, want admin", got)
	}
	if got := Merge(RoleEditor, RoleOrgAdmin); got != RoleOrgAdmin {
		t.Fatalf("Merge(editor,orgAdmin)=%q, want org_admin", got)
	}
	if got := Merge(); got != RoleNone {
		t.Fatalf("Merge()=%q, want none", got)
	}
	if got := Merge(RoleNone, RoleViewer); got != RoleViewer {
		t.Fatalf("Merge(none,viewer)=%q, want viewer", got)
	}
}

func TestAtLeast(t *testing.T) {
	if !RoleAdmin.AtLeast(RoleEditor) {
		t.Fatal("admin should satisfy editor minimum")
	}
	if RoleViewer.AtLeast(RoleEditor) {
		t.Fatal("viewer must NOT satisfy editor minimum")
	}
	if RoleNone.AtLeast(RoleViewer) {
		t.Fatal("none must NOT satisfy viewer minimum")
	}
}

func TestParseRejectsUnknown(t *testing.T) {
	if _, err := Parse("superuser"); err == nil {
		t.Fatal("Parse(superuser) should error")
	}
	r, err := Parse("editor")
	if err != nil || r != RoleEditor {
		t.Fatalf("Parse(editor)=%q,%v", r, err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd llm-agent-authz && GOWORK=off go test ./role/ -v`
Expected: FAIL — undefined `RoleOrgAdmin`, `Merge`, etc.

- [ ] **Step 3: Write the implementation**

Create `role/role.go`:
```go
// Package role defines the RBAC role enum and the org⊕scope merge algebra
// shared by all llm-agent-authz consumers. Pure: no I/O.
package role

import "fmt"

type Role string

const (
	RoleNone     Role = ""
	RoleViewer   Role = "viewer"
	RoleEditor   Role = "editor"
	RoleAdmin    Role = "admin"
	RoleOrgAdmin Role = "org_admin"
)

// Rank returns a total order; higher is more privileged.
func (r Role) Rank() int {
	switch r {
	case RoleOrgAdmin:
		return 4
	case RoleAdmin:
		return 3
	case RoleEditor:
		return 2
	case RoleViewer:
		return 1
	default:
		return 0
	}
}

// AtLeast reports whether r is at least as privileged as min.
func (r Role) AtLeast(min Role) bool { return r.Rank() >= min.Rank() && r.Rank() > 0 }

// Merge returns the highest-ranked role among the inputs (RoleNone if empty).
func Merge(roles ...Role) Role {
	best := RoleNone
	for _, r := range roles {
		if r.Rank() > best.Rank() {
			best = r
		}
	}
	return best
}

// Parse validates and converts a string to a Role.
func Parse(s string) (Role, error) {
	switch Role(s) {
	case RoleViewer, RoleEditor, RoleAdmin, RoleOrgAdmin:
		return Role(s), nil
	default:
		return RoleNone, fmt.Errorf("authz: unknown role %q", s)
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd llm-agent-authz && GOWORK=off go test ./role/ -v`
Expected: PASS (all 4 tests).

- [ ] **Step 5: Commit**

```bash
cd llm-agent-authz && git add role/ && git commit -m "feat(role): RBAC role enum + org⊕scope merge algebra"
```

---

## Task 3: Password hashing (`password` package, pure)

**Files:**
- Create: `llm-agent-authz/password/password.go`
- Test: `llm-agent-authz/password/password_test.go`

argon2id with a PHC-format encoded string (`$argon2id$v=19$m=...,t=...,p=...$salt$hash`), constant-time verify.

- [ ] **Step 1: Write the failing test**

Create `password/password_test.go`:
```go
package password

import (
	"strings"
	"testing"
)

func TestHashVerifyRoundTrip(t *testing.T) {
	enc, err := Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if !strings.HasPrefix(enc, "$argon2id$") {
		t.Fatalf("encoded form not PHC argon2id: %q", enc)
	}
	ok, err := Verify("correct horse battery staple", enc)
	if err != nil || !ok {
		t.Fatalf("Verify(correct)=%v,%v want true,nil", ok, err)
	}
	ok, err = Verify("wrong", enc)
	if err != nil || ok {
		t.Fatalf("Verify(wrong)=%v,%v want false,nil", ok, err)
	}
}

func TestHashIsSalted(t *testing.T) {
	a, _ := Hash("same")
	b, _ := Hash("same")
	if a == b {
		t.Fatal("two hashes of the same password must differ (random salt)")
	}
}

func TestVerifyRejectsMalformed(t *testing.T) {
	if _, err := Verify("x", "not-a-phc-string"); err == nil {
		t.Fatal("Verify of malformed encoded string must error")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd llm-agent-authz && GOWORK=off go test ./password/ -v`
Expected: FAIL — undefined `Hash`/`Verify`.

- [ ] **Step 3: Write the implementation**

Create `password/password.go`:
```go
// Package password provides argon2id password hashing with PHC-encoded output.
// Pure: no I/O beyond crypto/rand.
package password

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argonTime    = 1
	argonMemory  = 64 * 1024 // 64 MiB
	argonThreads = 4
	argonKeyLen  = 32
	saltLen      = 16
)

// Hash returns a PHC-format argon2id encoding of plain.
func Hash(plain string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(plain), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	b64 := base64.RawStdEncoding.EncodeToString
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads, b64(salt), b64(key)), nil
}

// Verify reports whether plain matches the PHC-encoded hash, in constant time.
func Verify(plain, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	// ["", "argon2id", "v=19", "m=...,t=...,p=...", salt, hash]
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errors.New("authz: malformed argon2id hash")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, err
	}
	var m uint32
	var t, p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false, err
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, err
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, err
	}
	got := argon2.IDKey([]byte(plain), salt, uint32(t), m, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd llm-agent-authz && GOWORK=off go test ./password/ -v`
Expected: PASS (all 3 tests).

- [ ] **Step 5: Commit**

```bash
cd llm-agent-authz && git add password/ && git commit -m "feat(password): argon2id hashing with PHC encoding"
```

---

## Task 4: JWT access tokens (`token` package, pure)

**Files:**
- Create: `llm-agent-authz/token/token.go`
- Test: `llm-agent-authz/token/token_test.go`

HS256, subject = user id, short TTL. Verify rejects expired and wrong-signature tokens.

- [ ] **Step 1: Write the failing test**

Create `token/token_test.go`:
```go
package token

import (
	"testing"
	"time"
)

func TestIssueVerifyRoundTrip(t *testing.T) {
	iss := NewIssuer([]byte("test-secret"), 15*time.Minute)
	tok, err := iss.Issue("user-123", time.Unix(1_000_000, 0))
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	uid, err := iss.VerifyAt(tok, time.Unix(1_000_100, 0))
	if err != nil || uid != "user-123" {
		t.Fatalf("VerifyAt=%q,%v want user-123,nil", uid, err)
	}
}

func TestVerifyRejectsExpired(t *testing.T) {
	iss := NewIssuer([]byte("test-secret"), 1*time.Minute)
	tok, _ := iss.Issue("u", time.Unix(1_000_000, 0))
	if _, err := iss.VerifyAt(tok, time.Unix(1_000_000+120, 0)); err == nil {
		t.Fatal("expired token must fail verification")
	}
}

func TestVerifyRejectsWrongSecret(t *testing.T) {
	tok, _ := NewIssuer([]byte("secret-a"), time.Minute).Issue("u", time.Unix(1_000_000, 0))
	if _, err := NewIssuer([]byte("secret-b"), time.Minute).VerifyAt(tok, time.Unix(1_000_001, 0)); err == nil {
		t.Fatal("token signed with different secret must fail")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd llm-agent-authz && GOWORK=off go test ./token/ -v`
Expected: FAIL — undefined `NewIssuer`.

- [ ] **Step 3: Write the implementation**

Create `token/token.go`:
```go
// Package token issues and verifies short-lived HS256 JWT access tokens.
// Pure: no I/O. Time is injectable for deterministic tests.
package token

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Issuer struct {
	secret    []byte
	accessTTL time.Duration
}

func NewIssuer(secret []byte, accessTTL time.Duration) *Issuer {
	return &Issuer{secret: secret, accessTTL: accessTTL}
}

// Issue mints an access token for userID, valid for accessTTL from now.
func (i *Issuer) Issue(userID string, now time.Time) (string, error) {
	claims := jwt.RegisteredClaims{
		Subject:   userID,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(i.accessTTL)),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(i.secret)
}

// VerifyAt validates the token as of `now` and returns the subject (user id).
func (i *Issuer) VerifyAt(tok string, now time.Time) (string, error) {
	parsed, err := jwt.Parse(tok, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("authz: unexpected signing method")
		}
		return i.secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithTimeFunc(func() time.Time { return now }))
	if err != nil {
		return "", err
	}
	sub, err := parsed.Claims.GetSubject()
	if err != nil || sub == "" {
		return "", errors.New("authz: token missing subject")
	}
	return sub, nil
}

// Verify validates as of the current wall clock.
func (i *Issuer) Verify(tok string) (string, error) { return i.VerifyAt(tok, time.Now()) }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd llm-agent-authz && GOWORK=off go test ./token/ -v`
Expected: PASS (all 3 tests).

- [ ] **Step 5: Commit**

```bash
cd llm-agent-authz && git add token/ && git commit -m "feat(token): HS256 JWT issue/verify with injectable clock"
```

---

## Task 5: Store schema + migrations

**Files:**
- Create: `llm-agent-authz/store/schema.go`
- Create: `llm-agent-authz/store/store.go`
- Test: `llm-agent-authz/store/store_test.go`

Four tables + a `authz_schema_version` tracker, applied by `Migrate(ctx)`. Live-PG test gated by `LLM_AGENT_AUTHZ_PG_URL`.

- [ ] **Step 1: Write the failing test**

Create `store/store_test.go`:
```go
package store

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

const liveEnvVar = "LLM_AGENT_AUTHZ_PG_URL"

func openTestStore(t *testing.T, ctx context.Context) *Store {
	t.Helper()
	dsn := os.Getenv(liveEnvVar)
	if dsn == "" {
		t.Skipf("set %s to run live postgres tests", liveEnvVar)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	s := New(pool)
	// Clean slate for deterministic tests.
	for _, tbl := range []string{"auth_session", "auth_membership", "knowledge_base_placeholder", "auth_user", "auth_org"} {
		_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS "+tbl+" CASCADE")
	}
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS authz_schema_version")
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return s
}

func TestMigrateIsIdempotent(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t, ctx)
	if err := s.Migrate(ctx); err != nil { // second run must be a no-op
		t.Fatalf("second Migrate: %v", err)
	}
	var n int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM auth_user").Scan(&n); err != nil {
		t.Fatalf("auth_user not queryable after migrate: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd llm-agent-authz && GOWORK=off go test ./store/ -v`
Expected: FAIL to compile — undefined `New`, `Store`, `Migrate`.

- [ ] **Step 3: Write `store/store.go`**

```go
// Package store is the pgx-backed persistence layer for llm-agent-authz:
// users, orgs, memberships, and refresh sessions, plus versioned migrations.
package store

import "github.com/jackc/pgx/v5/pgxpool"

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }
```

- [ ] **Step 4: Write `store/schema.go`**

```go
package store

import (
	"context"
	"fmt"
)

// HeadSchemaVersion is the latest schema version this code migrates to.
const HeadSchemaVersion = 1

type migrationGroup struct {
	Version    int
	Statements []string
}

func migrations() []migrationGroup {
	return []migrationGroup{
		{Version: 1, Statements: []string{
			`CREATE TABLE IF NOT EXISTS auth_org (
				id          TEXT PRIMARY KEY,
				name        TEXT NOT NULL,
				created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
			)`,
			`CREATE TABLE IF NOT EXISTS auth_user (
				id            TEXT PRIMARY KEY,
				email         TEXT NOT NULL UNIQUE,
				password_hash TEXT NOT NULL,
				created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
			)`,
			// scope_id NULL = org-level membership. The COALESCE unique index
			// makes (org,user,scope_kind,scope_id) unique while treating NULL
			// scope_id as a single distinct value.
			`CREATE TABLE IF NOT EXISTS auth_membership (
				org_id     TEXT NOT NULL REFERENCES auth_org(id) ON DELETE CASCADE,
				user_id    TEXT NOT NULL REFERENCES auth_user(id) ON DELETE CASCADE,
				scope_kind TEXT NOT NULL,
				scope_id   TEXT,
				role       TEXT NOT NULL
			)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS auth_membership_uniq
				ON auth_membership (org_id, user_id, scope_kind, COALESCE(scope_id, ''))`,
			`CREATE TABLE IF NOT EXISTS auth_session (
				id           TEXT PRIMARY KEY,
				user_id      TEXT NOT NULL REFERENCES auth_user(id) ON DELETE CASCADE,
				refresh_hash TEXT NOT NULL UNIQUE,
				user_agent   TEXT NOT NULL DEFAULT '',
				expires_at   TIMESTAMPTZ NOT NULL,
				revoked_at   TIMESTAMPTZ,
				created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
			)`,
			`CREATE INDEX IF NOT EXISTS auth_session_user ON auth_session (user_id)`,
		}},
	}
}

// Migrate applies all migration groups not yet recorded, transactionally per group.
func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx,
		`CREATE TABLE IF NOT EXISTS authz_schema_version (version INT PRIMARY KEY)`); err != nil {
		return fmt.Errorf("authz: ensure version table: %w", err)
	}
	var current int
	if err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(max(version), 0) FROM authz_schema_version`).Scan(&current); err != nil {
		return fmt.Errorf("authz: read schema version: %w", err)
	}
	for _, g := range migrations() {
		if g.Version <= current {
			continue
		}
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return err
		}
		for _, stmt := range g.Statements {
			if _, err := tx.Exec(ctx, stmt); err != nil {
				_ = tx.Rollback(ctx)
				return fmt.Errorf("authz: migrate v%d: %w", g.Version, err)
			}
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO authz_schema_version(version) VALUES ($1)`, g.Version); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run (with a disposable Postgres):
```bash
docker run -d --rm --name authz-pg -e POSTGRES_PASSWORD=pw -p 55432:5432 postgres:16
export LLM_AGENT_AUTHZ_PG_URL='postgres://postgres:pw@localhost:55432/postgres'
cd llm-agent-authz && GOWORK=off go test ./store/ -run TestMigrate -v
```
Expected: PASS. (Without the env var the test SKIPs — also acceptable in CI lacking PG, but you MUST run it once locally with PG to prove it.)

- [ ] **Step 6: Commit**

```bash
cd llm-agent-authz && git add store/store.go store/schema.go store/store_test.go && \
git commit -m "feat(store): authz schema + versioned Migrate (org/user/membership/session)"
```

---

## Task 6: Users & orgs persistence

**Files:**
- Create: `llm-agent-authz/store/users.go`
- Modify: `llm-agent-authz/store/store_test.go` (append tests)

- [ ] **Step 1: Write the failing test (append to `store/store_test.go`)**

```go
func TestCreateAndGetUser(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t, ctx)
	id, err := s.CreateUser(ctx, "Alice@Example.com", "phc-hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	// Email is normalized to lowercase; lookup is case-insensitive.
	u, err := s.GetUserByEmail(ctx, "alice@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if u.ID != id || u.Email != "alice@example.com" || u.PasswordHash != "phc-hash" {
		t.Fatalf("user mismatch: %+v", u)
	}
}

func TestCreateUserDuplicateEmail(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t, ctx)
	if _, err := s.CreateUser(ctx, "dup@x.com", "h"); err != nil {
		t.Fatalf("first CreateUser: %v", err)
	}
	if _, err := s.CreateUser(ctx, "DUP@x.com", "h"); err == nil {
		t.Fatal("duplicate email (case-insensitive) must error")
	}
}

func TestGetUserMissing(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t, ctx)
	if _, err := s.GetUserByEmail(ctx, "nobody@x.com"); err != ErrNotFound {
		t.Fatalf("missing user err=%v, want ErrNotFound", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd llm-agent-authz && GOWORK=off go test ./store/ -run 'TestCreate|TestGetUser' -v`
Expected: FAIL — undefined `CreateUser`, `GetUserByEmail`, `User`, `ErrNotFound`.

- [ ] **Step 3: Write `store/users.go`**

```go
package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

// ErrNotFound is returned when a lookup matches no row.
var ErrNotFound = errors.New("authz: not found")

type User struct {
	ID           string
	Email        string
	PasswordHash string
}

// newID returns a random 128-bit hex id (no external uuid dep needed).
func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func normalizeEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

func (s *Store) CreateUser(ctx context.Context, email, passwordHash string) (string, error) {
	id := newID()
	_, err := s.pool.Exec(ctx,
		`INSERT INTO auth_user (id, email, password_hash) VALUES ($1, $2, $3)`,
		id, normalizeEmail(email), passwordHash)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (User, error) {
	var u User
	err := s.pool.QueryRow(ctx,
		`SELECT id, email, password_hash FROM auth_user WHERE email = $1`,
		normalizeEmail(email)).Scan(&u.ID, &u.Email, &u.PasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

func (s *Store) CreateOrg(ctx context.Context, name string) (string, error) {
	id := newID()
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO auth_org (id, name) VALUES ($1, $2)`, id, name); err != nil {
		return "", err
	}
	return id, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run (PG env set): `cd llm-agent-authz && GOWORK=off go test ./store/ -run 'TestCreate|TestGetUser' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd llm-agent-authz && git add store/users.go store/store_test.go && \
git commit -m "feat(store): user + org creation, case-insensitive email lookup"
```

---

## Task 7: Memberships + role resolution

**Files:**
- Create: `llm-agent-authz/store/memberships.go`
- Modify: `llm-agent-authz/store/store_test.go` (append tests — the overreach matrix)

`ResolveRole` returns the merged effective role for (user, org, scope) per spec §2.

- [ ] **Step 1: Write the failing test (append to `store/store_test.go`)**

```go
import "github.com/costa92/llm-agent-authz/role"  // add to the import block at top of file

func seedUserOrg(t *testing.T, ctx context.Context, s *Store) (uid, oid string) {
	t.Helper()
	uid, err := s.CreateUser(ctx, newID()+"@x.com", "h")
	if err != nil {
		t.Fatal(err)
	}
	oid, err = s.CreateOrg(ctx, "Acme")
	if err != nil {
		t.Fatal(err)
	}
	return uid, oid
}

func TestResolveRoleMergesOrgAndScope(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t, ctx)
	uid, oid := seedUserOrg(t, ctx, s)
	// org-level editor + scope-level viewer => effective editor (highest wins).
	if err := s.UpsertMembership(ctx, oid, uid, "kb", nil, role.RoleEditor); err != nil {
		t.Fatal(err)
	}
	kb := "kb-1"
	if err := s.UpsertMembership(ctx, oid, uid, "kb", &kb, role.RoleViewer); err != nil {
		t.Fatal(err)
	}
	got, err := s.ResolveRole(ctx, uid, oid, "kb", "kb-1")
	if err != nil || got != role.RoleEditor {
		t.Fatalf("ResolveRole=%q,%v want editor", got, err)
	}
}

func TestResolveRoleNoMembershipIsNone(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t, ctx)
	uid, oid := seedUserOrg(t, ctx, s)
	got, err := s.ResolveRole(ctx, uid, oid, "kb", "kb-1")
	if err != nil {
		t.Fatal(err)
	}
	if got != role.RoleNone {
		t.Fatalf("no membership should resolve to none, got %q", got)
	}
}

func TestResolveRoleScopeIsolated(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t, ctx)
	uid, oid := seedUserOrg(t, ctx, s)
	kbA := "kb-A"
	if err := s.UpsertMembership(ctx, oid, uid, "kb", &kbA, role.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	// admin on kb-A must NOT leak to kb-B.
	got, _ := s.ResolveRole(ctx, uid, oid, "kb", "kb-B")
	if got != role.RoleNone {
		t.Fatalf("admin on kb-A leaked to kb-B: %q", got)
	}
}

func TestUpsertMembershipReplacesRole(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t, ctx)
	uid, oid := seedUserOrg(t, ctx, s)
	kb := "kb-1"
	_ = s.UpsertMembership(ctx, oid, uid, "kb", &kb, role.RoleViewer)
	if err := s.UpsertMembership(ctx, oid, uid, "kb", &kb, role.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	got, _ := s.ResolveRole(ctx, uid, oid, "kb", "kb-1")
	if got != role.RoleAdmin {
		t.Fatalf("upsert should replace role, got %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd llm-agent-authz && GOWORK=off go test ./store/ -run TestResolveRole -v`
Expected: FAIL — undefined `UpsertMembership`, `ResolveRole`.

- [ ] **Step 3: Write `store/memberships.go`**

```go
package store

import (
	"context"

	"github.com/costa92/llm-agent-authz/role"
)

// UpsertMembership inserts or replaces a membership row. scopeID nil = org-level.
func (s *Store) UpsertMembership(ctx context.Context, orgID, userID, scopeKind string, scopeID *string, r role.Role) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO auth_membership (org_id, user_id, scope_kind, scope_id, role)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (org_id, user_id, scope_kind, COALESCE(scope_id, ''))
		 DO UPDATE SET role = EXCLUDED.role`,
		orgID, userID, scopeKind, scopeID, string(r))
	return err
}

// ResolveRole returns the merged effective role for (user, org, scope):
// the highest of the org-level row (scope_id IS NULL) and the scope-level row.
// Returns RoleNone (nil error) when the user has no membership in the org.
func (s *Store) ResolveRole(ctx context.Context, userID, orgID, scopeKind, scopeID string) (role.Role, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT role FROM auth_membership
		 WHERE user_id = $1 AND org_id = $2 AND scope_kind = $3
		   AND (scope_id IS NULL OR scope_id = $4)`,
		userID, orgID, scopeKind, scopeID)
	if err != nil {
		return role.RoleNone, err
	}
	defer rows.Close()
	var found []role.Role
	for rows.Next() {
		var rs string
		if err := rows.Scan(&rs); err != nil {
			return role.RoleNone, err
		}
		found = append(found, role.Role(rs))
	}
	if err := rows.Err(); err != nil {
		return role.RoleNone, err
	}
	return role.Merge(found...), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run (PG env set): `cd llm-agent-authz && GOWORK=off go test ./store/ -run TestResolveRole -v && GOWORK=off go test ./store/ -run TestUpsert -v`
Expected: PASS (3 ResolveRole + 1 Upsert).

- [ ] **Step 5: Commit**

```bash
cd llm-agent-authz && git add store/memberships.go store/store_test.go && \
git commit -m "feat(store): membership upsert + org⊕scope role resolution (overreach matrix tested)"
```

---

## Task 8: Sessions (refresh tokens) persistence

**Files:**
- Create: `llm-agent-authz/store/sessions.go`
- Modify: `llm-agent-authz/store/store_test.go` (append tests)

- [ ] **Step 1: Write the failing test (append to `store/store_test.go`)**

```go
import "time" // add to import block if not present

func TestSessionLifecycle(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t, ctx)
	uid, _ := seedUserOrg(t, ctx, s)
	exp := time.Now().Add(time.Hour)
	if err := s.CreateSession(ctx, uid, "hash-1", "agent", exp); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	sess, err := s.SessionByHash(ctx, "hash-1")
	if err != nil || sess.UserID != uid || sess.RevokedAt != nil {
		t.Fatalf("SessionByHash=%+v,%v", sess, err)
	}
	// Rotate: old hash invalid, new hash valid.
	if err := s.RotateSession(ctx, "hash-1", "hash-2", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("RotateSession: %v", err)
	}
	if _, err := s.SessionByHash(ctx, "hash-1"); err != ErrNotFound {
		t.Fatalf("old hash after rotate err=%v, want ErrNotFound", err)
	}
	if _, err := s.SessionByHash(ctx, "hash-2"); err != nil {
		t.Fatalf("new hash after rotate: %v", err)
	}
}

func TestRevokeAllForUser(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t, ctx)
	uid, _ := seedUserOrg(t, ctx, s)
	exp := time.Now().Add(time.Hour)
	_ = s.CreateSession(ctx, uid, "h-a", "", exp)
	_ = s.CreateSession(ctx, uid, "h-b", "", exp)
	if err := s.RevokeAllForUser(ctx, uid); err != nil {
		t.Fatalf("RevokeAllForUser: %v", err)
	}
	if _, err := s.SessionByHash(ctx, "h-a"); err != ErrNotFound {
		t.Fatalf("h-a should be gone, err=%v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd llm-agent-authz && GOWORK=off go test ./store/ -run 'TestSession|TestRevoke' -v`
Expected: FAIL — undefined session methods.

- [ ] **Step 3: Write `store/sessions.go`**

```go
package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type Session struct {
	ID        string
	UserID    string
	ExpiresAt time.Time
	RevokedAt *time.Time
}

func (s *Store) CreateSession(ctx context.Context, userID, refreshHash, userAgent string, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO auth_session (id, user_id, refresh_hash, user_agent, expires_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		newID(), userID, refreshHash, userAgent, expiresAt)
	return err
}

// SessionByHash returns a live (not revoked, not expired) session for the hash.
func (s *Store) SessionByHash(ctx context.Context, refreshHash string) (Session, error) {
	var sess Session
	err := s.pool.QueryRow(ctx,
		`SELECT id, user_id, expires_at, revoked_at FROM auth_session
		 WHERE refresh_hash = $1 AND revoked_at IS NULL AND expires_at > now()`,
		refreshHash).Scan(&sess.ID, &sess.UserID, &sess.ExpiresAt, &sess.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	return sess, err
}

// RotateSession atomically revokes oldHash and inserts newHash for the same user.
// Returns ErrNotFound if oldHash is not a live session (reuse/replay → caller revokes chain).
func (s *Store) RotateSession(ctx context.Context, oldHash, newHash string, newExpiry time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var userID string
	err = tx.QueryRow(ctx,
		`UPDATE auth_session SET revoked_at = now()
		 WHERE refresh_hash = $1 AND revoked_at IS NULL AND expires_at > now()
		 RETURNING user_id`, oldHash).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx,
		`INSERT INTO auth_session (id, user_id, refresh_hash, expires_at)
		 VALUES ($1, $2, $3, $4)`, newID(), userID, newHash, newExpiry); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) RevokeSession(ctx context.Context, refreshHash string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE auth_session SET revoked_at = now() WHERE refresh_hash = $1 AND revoked_at IS NULL`,
		refreshHash)
	return err
}

func (s *Store) RevokeAllForUser(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE auth_session SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`,
		userID)
	return err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run (PG env set): `cd llm-agent-authz && GOWORK=off go test ./store/ -run 'TestSession|TestRevoke' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd llm-agent-authz && git add store/sessions.go store/store_test.go && \
git commit -m "feat(store): refresh sessions — create/lookup/rotate/revoke"
```

---

## Task 9: Service — login/refresh/logout orchestration

**Files:**
- Create: `llm-agent-authz/service/service.go`
- Test: `llm-agent-authz/service/service_test.go`

The service wires store+password+token. Refresh tokens are opaque random strings; only their sha256 hash is stored. Refresh rotation detects reuse (a revoked/unknown token → revoke all the user's sessions is deferred to M2; M1: reject with error).

- [ ] **Step 1: Write the failing test**

The service needs only a narrow slice of the store, so the test uses a fake implementing a `Store` interface — no DB required.

Create `service/service_test.go`:
```go
package service

import (
	"context"
	"testing"
	"time"

	"github.com/costa92/llm-agent-authz/password"
	"github.com/costa92/llm-agent-authz/store"
	"github.com/costa92/llm-agent-authz/token"
)

// fakeStore implements the Store interface in-memory.
type fakeStore struct {
	users    map[string]store.User // by email
	sessions map[string]string     // refresh_hash -> userID (live only)
}

func newFakeStore() *fakeStore {
	return &fakeStore{users: map[string]store.User{}, sessions: map[string]string{}}
}
func (f *fakeStore) GetUserByEmail(_ context.Context, email string) (store.User, error) {
	u, ok := f.users[email]
	if !ok {
		return store.User{}, store.ErrNotFound
	}
	return u, nil
}
func (f *fakeStore) CreateSession(_ context.Context, userID, hash, _ string, _ time.Time) error {
	f.sessions[hash] = userID
	return nil
}
func (f *fakeStore) SessionByHash(_ context.Context, hash string) (store.Session, error) {
	uid, ok := f.sessions[hash]
	if !ok {
		return store.Session{}, store.ErrNotFound
	}
	return store.Session{UserID: uid, ExpiresAt: time.Now().Add(time.Hour)}, nil
}
func (f *fakeStore) RotateSession(_ context.Context, oldHash, newHash string, _ time.Time) error {
	uid, ok := f.sessions[oldHash]
	if !ok {
		return store.ErrNotFound
	}
	delete(f.sessions, oldHash)
	f.sessions[newHash] = uid
	return nil
}
func (f *fakeStore) RevokeSession(_ context.Context, hash string) error {
	delete(f.sessions, hash)
	return nil
}
func (f *fakeStore) RevokeAllForUser(_ context.Context, userID string) error {
	for h, u := range f.sessions {
		if u == userID {
			delete(f.sessions, h)
		}
	}
	return nil
}

func newSvc(t *testing.T) (*Service, *fakeStore) {
	t.Helper()
	fs := newFakeStore()
	hash, _ := password.Hash("pw")
	fs.users["alice@x.com"] = store.User{ID: "u1", Email: "alice@x.com", PasswordHash: hash}
	svc := New(fs, token.NewIssuer([]byte("sec"), 15*time.Minute), 30*24*time.Hour)
	return svc, fs
}

func TestLoginSuccess(t *testing.T) {
	svc, _ := newSvc(t)
	res, err := svc.Login(context.Background(), "alice@x.com", "pw", "agent")
	if err != nil || res.AccessToken == "" || res.RefreshToken == "" {
		t.Fatalf("Login=%+v,%v", res, err)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	svc, _ := newSvc(t)
	if _, err := svc.Login(context.Background(), "alice@x.com", "nope", ""); err != ErrInvalidCredentials {
		t.Fatalf("err=%v want ErrInvalidCredentials", err)
	}
}

func TestLoginUnknownUser(t *testing.T) {
	svc, _ := newSvc(t)
	// Must NOT distinguish unknown-user from wrong-password (no enumeration).
	if _, err := svc.Login(context.Background(), "ghost@x.com", "pw", ""); err != ErrInvalidCredentials {
		t.Fatalf("err=%v want ErrInvalidCredentials", err)
	}
}

func TestRefreshRotates(t *testing.T) {
	svc, _ := newSvc(t)
	first, _ := svc.Login(context.Background(), "alice@x.com", "pw", "")
	second, err := svc.Refresh(context.Background(), first.RefreshToken)
	if err != nil || second.RefreshToken == first.RefreshToken {
		t.Fatalf("Refresh did not rotate: %+v,%v", second, err)
	}
	// Old refresh token must no longer work.
	if _, err := svc.Refresh(context.Background(), first.RefreshToken); err == nil {
		t.Fatal("reused old refresh token must fail")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd llm-agent-authz && GOWORK=off go test ./service/ -v`
Expected: FAIL — undefined `New`, `Service`, `ErrInvalidCredentials`.

- [ ] **Step 3: Write `service/service.go`**

```go
// Package service orchestrates login/refresh/logout over store+password+token.
package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/costa92/llm-agent-authz/password"
	"github.com/costa92/llm-agent-authz/store"
	"github.com/costa92/llm-agent-authz/token"
)

// ErrInvalidCredentials is returned for both unknown user and wrong password
// (no account enumeration).
var ErrInvalidCredentials = errors.New("authz: invalid credentials")

// Store is the persistence slice the service needs (satisfied by *store.Store).
type Store interface {
	GetUserByEmail(ctx context.Context, email string) (store.User, error)
	CreateSession(ctx context.Context, userID, refreshHash, userAgent string, expiresAt time.Time) error
	SessionByHash(ctx context.Context, refreshHash string) (store.Session, error)
	RotateSession(ctx context.Context, oldHash, newHash string, newExpiry time.Time) error
	RevokeSession(ctx context.Context, refreshHash string) error
	RevokeAllForUser(ctx context.Context, userID string) error
}

type Service struct {
	store      Store
	issuer     *token.Issuer
	refreshTTL time.Duration
}

func New(s Store, issuer *token.Issuer, refreshTTL time.Duration) *Service {
	return &Service{store: s, issuer: issuer, refreshTTL: refreshTTL}
}

// LoginResult carries the freshly minted tokens. RefreshToken is the opaque
// secret to set as an httpOnly cookie; only its hash is persisted.
type LoginResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int // access token TTL seconds
}

func newRefreshToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func hashToken(t string) string {
	sum := sha256.Sum256([]byte(t))
	return hex.EncodeToString(sum[:])
}

func (s *Service) Login(ctx context.Context, email, plain, userAgent string) (LoginResult, error) {
	u, err := s.store.GetUserByEmail(ctx, email)
	if errors.Is(err, store.ErrNotFound) {
		return LoginResult{}, ErrInvalidCredentials
	}
	if err != nil {
		return LoginResult{}, err
	}
	ok, err := password.Verify(plain, u.PasswordHash)
	if err != nil || !ok {
		return LoginResult{}, ErrInvalidCredentials
	}
	return s.issue(ctx, u.ID, userAgent)
}

func (s *Service) issue(ctx context.Context, userID, userAgent string) (LoginResult, error) {
	now := time.Now()
	access, err := s.issuer.Issue(userID, now)
	if err != nil {
		return LoginResult{}, err
	}
	refresh := newRefreshToken()
	if err := s.store.CreateSession(ctx, userID, hashToken(refresh), userAgent, now.Add(s.refreshTTL)); err != nil {
		return LoginResult{}, err
	}
	return LoginResult{AccessToken: access, RefreshToken: refresh, ExpiresIn: 900}, nil
}

// Refresh rotates the refresh token (old becomes invalid) and mints a new access token.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (LoginResult, error) {
	sess, err := s.store.SessionByHash(ctx, hashToken(refreshToken))
	if errors.Is(err, store.ErrNotFound) {
		return LoginResult{}, ErrInvalidCredentials
	}
	if err != nil {
		return LoginResult{}, err
	}
	now := time.Now()
	access, err := s.issuer.Issue(sess.UserID, now)
	if err != nil {
		return LoginResult{}, err
	}
	newRefresh := newRefreshToken()
	if err := s.store.RotateSession(ctx, hashToken(refreshToken), hashToken(newRefresh), now.Add(s.refreshTTL)); err != nil {
		return LoginResult{}, err
	}
	return LoginResult{AccessToken: access, RefreshToken: newRefresh, ExpiresIn: 900}, nil
}

func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	return s.store.RevokeSession(ctx, hashToken(refreshToken))
}

func (s *Service) LogoutAll(ctx context.Context, userID string) error {
	return s.store.RevokeAllForUser(ctx, userID)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd llm-agent-authz && GOWORK=off go test ./service/ -v`
Expected: PASS (5 tests; no DB needed — uses fakeStore).

- [ ] **Step 5: Commit**

```bash
cd llm-agent-authz && git add service/ && \
git commit -m "feat(service): login/refresh(rotation)/logout, no account enumeration"
```

---

## Task 10: HTTP middleware — Authenticate + RequireScopeRole

**Files:**
- Create: `llm-agent-authz/httpapi/middleware.go`
- Test: `llm-agent-authz/httpapi/httpapi_test.go`

- [ ] **Step 1: Write the failing test**

Create `httpapi/httpapi_test.go`:
```go
package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/costa92/llm-agent-authz/role"
	"github.com/costa92/llm-agent-authz/token"
)

// roleResolver fake for middleware tests.
type fakeResolver struct{ roles map[string]role.Role } // key: scopeID

func (f fakeResolver) ResolveRole(_ context.Context, _, _, _, scopeID string) (role.Role, error) {
	return f.roles[scopeID], nil
}

func TestAuthenticateRejectsMissingToken(t *testing.T) {
	iss := token.NewIssuer([]byte("s"), time.Minute)
	h := Authenticate(iss)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not run without token")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code=%d want 401", rec.Code)
	}
}

func TestAuthenticatePassesUserID(t *testing.T) {
	iss := token.NewIssuer([]byte("s"), time.Minute)
	tok, _ := iss.Issue("u1", time.Now())
	var seen string
	h := Authenticate(iss)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = UserID(r.Context())
	}))
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	h.ServeHTTP(httptest.NewRecorder(), req)
	if seen != "u1" {
		t.Fatalf("UserID in ctx=%q want u1", seen)
	}
}

func TestRequireScopeRoleForbidsInsufficient(t *testing.T) {
	res := fakeResolver{roles: map[string]role.Role{"kb-1": role.RoleViewer}}
	mw := RequireScopeRole(res, "kb", role.RoleEditor, func(r *http.Request) (string, string) {
		return "org-1", r.PathValue("kb")
	})
	h := withUser("u1", mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not run for viewer when editor required")
	})))
	mux := http.NewServeMux()
	mux.Handle("/kb/{kb}", h)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/kb/kb-1", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("code=%d want 403", rec.Code)
	}
}

func TestRequireScopeRoleAllowsSufficient(t *testing.T) {
	res := fakeResolver{roles: map[string]role.Role{"kb-1": role.RoleAdmin}}
	mw := RequireScopeRole(res, "kb", role.RoleEditor, func(r *http.Request) (string, string) {
		return "org-1", r.PathValue("kb")
	})
	ran := false
	h := withUser("u1", mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { ran = true })))
	mux := http.NewServeMux()
	mux.Handle("/kb/{kb}", h)
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/kb/kb-1", nil))
	if !ran {
		t.Fatal("handler should run for admin when editor required")
	}
}

// withUser injects a user id into the context (test helper mimicking Authenticate).
func withUser(uid string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxUserKey{}, uid)))
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd llm-agent-authz && GOWORK=off go test ./httpapi/ -v`
Expected: FAIL — undefined `Authenticate`, `UserID`, `RequireScopeRole`, `ctxUserKey`.

- [ ] **Step 3: Write `httpapi/middleware.go`**

```go
// Package httpapi exposes auth HTTP handlers and middleware for consumers to mount.
package httpapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/costa92/llm-agent-authz/role"
	"github.com/costa92/llm-agent-authz/token"
)

type ctxUserKey struct{}

// UserID returns the authenticated user id stored by Authenticate, or "".
func UserID(ctx context.Context) string {
	if v, ok := ctx.Value(ctxUserKey{}).(string); ok {
		return v
	}
	return ""
}

// Authenticate validates the Bearer access token and stores the user id in ctx.
func Authenticate(iss *token.Issuer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authz := r.Header.Get("Authorization")
			tok, ok := strings.CutPrefix(authz, "Bearer ")
			if !ok || tok == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			uid, err := iss.Verify(tok)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxUserKey{}, uid)))
		})
	}
}

// RoleResolver is satisfied by *store.Store.
type RoleResolver interface {
	ResolveRole(ctx context.Context, userID, orgID, scopeKind, scopeID string) (role.Role, error)
}

// ScopeFromRequest extracts (orgID, scopeID) from the request (e.g. path values).
type ScopeFromRequest func(r *http.Request) (orgID, scopeID string)

// RequireScopeRole enforces that the authenticated user has at least `min` role
// in the (scopeKind, scopeID) resolved from the request. 401 if unauthenticated,
// 403 if insufficient.
func RequireScopeRole(res RoleResolver, scopeKind string, min role.Role, scope ScopeFromRequest) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			uid := UserID(r.Context())
			if uid == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			orgID, scopeID := scope(r)
			eff, err := res.ResolveRole(r.Context(), uid, orgID, scopeKind, scopeID)
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			if !eff.AtLeast(min) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd llm-agent-authz && GOWORK=off go test ./httpapi/ -v`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
cd llm-agent-authz && git add httpapi/middleware.go httpapi/httpapi_test.go && \
git commit -m "feat(httpapi): Authenticate + RequireScopeRole middleware"
```

---

## Task 11: HTTP handlers — /api/auth/* + Mount

**Files:**
- Create: `llm-agent-authz/httpapi/handlers.go`
- Modify: `llm-agent-authz/httpapi/httpapi_test.go` (append handler tests)

login/refresh/logout/logout-all. Refresh token is delivered as an httpOnly+SameSite=Strict cookie; refresh/logout read it from the cookie and require a CSRF header (`X-CSRF: 1`, double-submit) since they are cookie-driven.

- [ ] **Step 1: Write the failing test (append to `httpapi/httpapi_test.go`)**

```go
import (
	"encoding/json"      // add to import block
	"strings"            // add to import block

	"github.com/costa92/llm-agent-authz/service"
)

// loginFunc/refreshFunc let the handler test stub the service without a DB.
type fakeAuth struct {
	loginRes  service.LoginResult
	loginErr  error
	refreshRes service.LoginResult
	refreshErr error
	loggedOut  bool
}

func (f *fakeAuth) Login(_ context.Context, email, pw, ua string) (service.LoginResult, error) {
	return f.loginRes, f.loginErr
}
func (f *fakeAuth) Refresh(_ context.Context, rt string) (service.LoginResult, error) {
	return f.refreshRes, f.refreshErr
}
func (f *fakeAuth) Logout(_ context.Context, rt string) error { f.loggedOut = true; return nil }
func (f *fakeAuth) LogoutAll(_ context.Context, uid string) error { return nil }

func TestLoginHandlerSetsCookieAndReturnsAccess(t *testing.T) {
	fa := &fakeAuth{loginRes: service.LoginResult{AccessToken: "acc", RefreshToken: "ref", ExpiresIn: 900}}
	h := New(fa)
	mux := http.NewServeMux()
	h.Mount(mux, "/api/auth")
	body := strings.NewReader(`{"email":"a@x.com","password":"pw"}`)
	req := httptest.NewRequest("POST", "/api/auth/login", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body)
	}
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out["access_token"] != "acc" {
		t.Fatalf("access_token=%v", out["access_token"])
	}
	if !strings.Contains(rec.Header().Get("Set-Cookie"), "HttpOnly") {
		t.Fatalf("refresh cookie not httpOnly: %q", rec.Header().Get("Set-Cookie"))
	}
}

func TestLoginHandlerInvalidCreds401(t *testing.T) {
	fa := &fakeAuth{loginErr: service.ErrInvalidCredentials}
	h := New(fa)
	mux := http.NewServeMux()
	h.Mount(mux, "/api/auth")
	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"email":"a@x.com","password":"x"}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code=%d want 401", rec.Code)
	}
}

func TestRefreshRequiresCSRFHeader(t *testing.T) {
	fa := &fakeAuth{refreshRes: service.LoginResult{AccessToken: "acc2", RefreshToken: "ref2", ExpiresIn: 900}}
	h := New(fa)
	mux := http.NewServeMux()
	h.Mount(mux, "/api/auth")
	req := httptest.NewRequest("POST", "/api/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "ref"})
	// No X-CSRF header → 403.
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("refresh without CSRF code=%d want 403", rec.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd llm-agent-authz && GOWORK=off go test ./httpapi/ -run 'TestLoginHandler|TestRefreshRequires' -v`
Expected: FAIL — undefined `New`, `Handlers.Mount`, `refreshCookieName`.

- [ ] **Step 3: Write `httpapi/handlers.go`**

```go
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/costa92/llm-agent-authz/service"
)

const refreshCookieName = "authz_refresh"

// AuthService is the slice of service.Service the handlers need (fake-able in tests).
type AuthService interface {
	Login(ctx context.Context, email, password, userAgent string) (service.LoginResult, error)
	Refresh(ctx context.Context, refreshToken string) (service.LoginResult, error)
	Logout(ctx context.Context, refreshToken string) error
	LogoutAll(ctx context.Context, userID string) error
}

type Handlers struct{ svc AuthService }

func New(svc AuthService) *Handlers { return &Handlers{svc: svc} }

// Mount registers the auth routes under prefix (e.g. "/api/auth") on mux.
func (h *Handlers) Mount(mux *http.ServeMux, prefix string) {
	mux.HandleFunc("POST "+prefix+"/login", h.login)
	mux.HandleFunc("POST "+prefix+"/refresh", h.refresh)
	mux.HandleFunc("POST "+prefix+"/logout", h.logout)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func setRefreshCookie(w http.ResponseWriter, value string) {
	http.SetCookie(w, &http.Cookie{
		Name: refreshCookieName, Value: value, Path: "/api/auth",
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode,
	})
}

func (h *Handlers) login(w http.ResponseWriter, r *http.Request) {
	var req struct{ Email, Password string }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	res, err := h.svc.Login(r.Context(), req.Email, req.Password, r.UserAgent())
	if errors.Is(err, service.ErrInvalidCredentials) {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	setRefreshCookie(w, res.RefreshToken)
	writeJSON(w, http.StatusOK, map[string]any{"access_token": res.AccessToken, "expires_in": res.ExpiresIn})
}

// requireCSRF enforces a double-submit header on cookie-driven endpoints.
func requireCSRF(r *http.Request) bool { return r.Header.Get("X-CSRF") == "1" }

func (h *Handlers) refresh(w http.ResponseWriter, r *http.Request) {
	if !requireCSRF(r) {
		http.Error(w, "missing csrf header", http.StatusForbidden)
		return
	}
	c, err := r.Cookie(refreshCookieName)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	res, err := h.svc.Refresh(r.Context(), c.Value)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	setRefreshCookie(w, res.RefreshToken)
	writeJSON(w, http.StatusOK, map[string]any{"access_token": res.AccessToken, "expires_in": res.ExpiresIn})
}

func (h *Handlers) logout(w http.ResponseWriter, r *http.Request) {
	if !requireCSRF(r) {
		http.Error(w, "missing csrf header", http.StatusForbidden)
		return
	}
	if c, err := r.Cookie(refreshCookieName); err == nil {
		_ = h.svc.Logout(r.Context(), c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: refreshCookieName, Value: "", Path: "/api/auth", MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd llm-agent-authz && GOWORK=off go test ./httpapi/ -v`
Expected: PASS (all middleware + handler tests).

- [ ] **Step 5: Commit**

```bash
cd llm-agent-authz && git add httpapi/handlers.go httpapi/httpapi_test.go && \
git commit -m "feat(httpapi): /api/auth login/refresh/logout handlers with httpOnly cookie + CSRF"
```

---

## Task 12: Migrate command + full build/test gate

**Files:**
- Create: `llm-agent-authz/cmd/authz-migrate/main.go`

- [ ] **Step 1: Write `cmd/authz-migrate/main.go`**

```go
// Command authz-migrate applies the llm-agent-authz schema to the database in
// LLM_AGENT_AUTHZ_PG_URL (or --dsn).
package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/costa92/llm-agent-authz/store"
)

func main() {
	dsn := flag.String("dsn", os.Getenv("LLM_AGENT_AUTHZ_PG_URL"), "Postgres DSN")
	flag.Parse()
	if *dsn == "" {
		log.Fatal("authz-migrate: set LLM_AGENT_AUTHZ_PG_URL or pass --dsn")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *dsn)
	if err != nil {
		log.Fatalf("authz-migrate: connect: %v", err)
	}
	defer pool.Close()
	if err := store.New(pool).Migrate(ctx); err != nil {
		log.Fatalf("authz-migrate: migrate: %v", err)
	}
	log.Printf("authz-migrate: schema at head version %d", store.HeadSchemaVersion)
}
```

- [ ] **Step 2: Build everything and run the full unit suite**

Run:
```bash
cd llm-agent-authz && GOWORK=off go build ./... && GOWORK=off go vet ./... && GOWORK=off go test ./... -v
```
Expected: build+vet clean; pure-package tests PASS; store tests SKIP if `LLM_AGENT_AUTHZ_PG_URL` unset, PASS if set. Run once WITH a live PG to prove store tests green.

- [ ] **Step 3: Commit**

```bash
cd llm-agent-authz && git add cmd/ && git commit -m "feat(cmd): authz-migrate command"
```

---

## Task 13: README + tag v0.1.0

**Files:**
- Create: `llm-agent-authz/README.md`

- [ ] **Step 1: Write `README.md`**

```markdown
# llm-agent-authz

Shared, importable tenancy/auth library for the llm-agent ecosystem: org → parameterized scope, argon2id passwords, HS256 JWT access tokens, refresh-token sessions, and `Authenticate` / `RequireScopeRole` HTTP middleware. A **library, not a service** — consumers run its migrations, mount its handlers, and wrap routes with its middleware.

## Quick start (consumer)

```go
st := store.New(pool)
if err := st.Migrate(ctx); err != nil { /* ... */ }
iss := token.NewIssuer([]byte(secret), 15*time.Minute)
svc := service.New(st, iss, 30*24*time.Hour)

mux := http.NewServeMux()
httpapi.New(svc).Mount(mux, "/api/auth")

// Protect a kb route: require editor on the {kb} path scope.
guard := httpapi.RequireScopeRole(st, "kb", role.RoleEditor,
    func(r *http.Request) (string, string) { return orgFromCtx(r), r.PathValue("kb") })
mux.Handle("PUT /api/kb/{kb}/documents", httpapi.Authenticate(iss)(guard(docHandler)))
```

## Roles

`org_admin > admin > editor > viewer`. Effective role = highest of org-level (scope_id NULL) and scope-level membership. Cross-org access is denied (no membership → `RoleNone` → 403).

## Tests

Pure packages (`role`, `password`, `token`, `service`, `httpapi`) run with `go test ./...`. Store/integration tests need a Postgres DSN in `LLM_AGENT_AUTHZ_PG_URL` (skipped otherwise).

## Note

Standalone sibling repo of the llm-agent ecosystem; run Go commands with `GOWORK=off` unless added to the umbrella `go.work`.
```

- [ ] **Step 2: Final verification**

Run: `cd llm-agent-authz && GOWORK=off go build ./... && GOWORK=off go test ./... && echo OK`
Expected: `OK` (with PG env set, all green).

- [ ] **Step 3: Commit and tag**

```bash
cd llm-agent-authz && git add README.md && git commit -m "docs: README for llm-agent-authz"
git tag v0.1.0
```
(Pushing to GitHub + creating the remote repo is a separate, user-authorized step — do not push without confirmation.)

---

## Self-Review

**Spec coverage (authz spec §6 M1):**
- user/org/membership/session tables + Migrate → Tasks 5–8 ✓
- argon2id → Task 3 ✓
- JWT → Task 4 ✓
- login/refresh/logout → Tasks 9, 11 ✓
- Authenticate / RequireScopeRole middleware → Task 10 ✓
- role merge algorithm → Tasks 2, 7 ✓
- overreach matrix tests → Task 7 (scope-isolated, cross-scope none, merge) + Task 10 (403/allow) ✓
- logout-all + refresh reuse-chain revocation → spec marks these M2; M1 includes `LogoutAll` service method (Task 9) and rejects reused refresh tokens (Task 9 test), full reuse-chain revocation deferred to M2 per spec §6 ✓

**Placeholder scan:** No TBD/TODO; every code step has complete code. ✓

**Type consistency:** `Role`/`Merge`/`AtLeast` (Task 2) used identically in Tasks 7/10; `store.User`/`store.Session`/`ErrNotFound` (Tasks 6/8) used in Task 9 `Store` interface; `token.Issuer.Issue(userID, now)` signature consistent across Tasks 4/9/10; `service.LoginResult` fields (`AccessToken`/`RefreshToken`/`ExpiresIn`) consistent across Tasks 9/11; `refreshCookieName` defined Task 11, used in its tests. ✓

**Note on cross-repo:** This plan delivers the `llm-agent-authz` v0.1.0 tag that `llm-agent-kb` M1 depends on. kb/studio plans are written separately after this tag exists.
