# M6 Memory Gateway Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first runnable `llm-agent-memory-gateway` HTTP/service module that exposes the first-batch 7 memory endpoints on top of SDK abstractions and the Postgres backend.

**Architecture:** The gateway module remains the only place allowed to own HTTP routing, auth-derived tenant binding, request validation, error translation, runtime configuration, and service composition. The implementation should use stdlib `net/http`, compose a backend service layer around `llm-agent-memory-postgres`, and keep Postgres semantics behind gateway-owned request/response contracts rather than leaking backend rows or SQL concepts directly to handlers.

**Tech Stack:** Go 1.26.0, stdlib `net/http`, `httptest`, `encoding/json`, `context`, `log/slog`, `os`, `testing`; `github.com/costa92/llm-agent-memory/memory`; `github.com/costa92/llm-agent-memory-postgres/postgres`; existing workspace `go.work`.

---

## File Structure

| Path | Action | Responsibility |
|---|---|---|
| `llm-agent-memory-gateway/go.mod` | Modify | Add direct module dependencies on `llm-agent-memory` and `llm-agent-memory-postgres` if missing |
| `llm-agent-memory-gateway/README.md` | Modify | Document M6 gateway scope, env vars, run command, and first-batch endpoint list |
| `llm-agent-memory-gateway/doc.go` | Modify | Update package doc from skeleton wording to concrete HTTP gateway wording |
| `llm-agent-memory-gateway/cmd/memory-gateway/main.go` | Create | Binary entry point, env/config loading, backend composition, HTTP server startup |
| `llm-agent-memory-gateway/cmd/memory-gateway/main_test.go` | Create | Compile/config smoke tests for command wiring |
| `llm-agent-memory-gateway/internal/config/config.go` | Create | Runtime config struct, env parsing, defaults, validation |
| `llm-agent-memory-gateway/internal/config/config_test.go` | Create | TDD for env parsing and validation |
| `llm-agent-memory-gateway/internal/authz/scope.go` | Create | Auth-derived scope model, header extraction, override rules |
| `llm-agent-memory-gateway/internal/authz/scope_test.go` | Create | TDD for tenant/user/session binding and client-claim override behavior |
| `llm-agent-memory-gateway/internal/httpapi/types.go` | Create | Public JSON request/response structs for first-batch endpoints and error model |
| `llm-agent-memory-gateway/internal/httpapi/types_test.go` | Create | JSON contract and required-field tests |
| `llm-agent-memory-gateway/internal/httpapi/errors.go` | Create | Error codes, HTTP mapping helpers, response writer for uniform error payloads |
| `llm-agent-memory-gateway/internal/httpapi/errors_test.go` | Create | TDD for exact error payload/status mapping |
| `llm-agent-memory-gateway/internal/service/service.go` | Create | Gateway service interface and implementation composed over SDK/backend abstractions |
| `llm-agent-memory-gateway/internal/service/service_test.go` | Create | TDD for transport-independent orchestration and decision trace emission |
| `llm-agent-memory-gateway/internal/service/token_budget.go` | Create | Token-estimate helper for recall hits and trace metadata |
| `llm-agent-memory-gateway/internal/service/token_budget_test.go` | Create | TDD for stable `token_cost_estimate` behavior |
| `llm-agent-memory-gateway/internal/transport/router.go` | Create | `http.Handler` construction and route registration |
| `llm-agent-memory-gateway/internal/transport/router_test.go` | Create | TDD for route presence and method constraints |
| `llm-agent-memory-gateway/internal/transport/middleware.go` | Create | Request ID, JSON content-type enforcement, response headers |
| `llm-agent-memory-gateway/internal/transport/middleware_test.go` | Create | TDD for `X-Request-Id`, `X-Memory-Version`, `X-Consistency-Level` |
| `llm-agent-memory-gateway/internal/transport/recall_handler.go` | Create | `POST /memory/recall/unified` handler |
| `llm-agent-memory-gateway/internal/transport/write_handler.go` | Create | `POST /memory/write` handler |
| `llm-agent-memory-gateway/internal/transport/patch_handler.go` | Create | `PATCH /memory/items/{memory_id}` handler |
| `llm-agent-memory-gateway/internal/transport/pin_handler.go` | Create | `POST /memory/items/{memory_id}/pin` handler |
| `llm-agent-memory-gateway/internal/transport/disable_handler.go` | Create | `POST /memory/items/{memory_id}/disable` handler |
| `llm-agent-memory-gateway/internal/transport/delete_handler.go` | Create | `DELETE /memory/items/{memory_id}` handler |
| `llm-agent-memory-gateway/internal/transport/session_handler.go` | Create | `POST /memory/sessions/{session_id}/close` handler |
| `llm-agent-memory-gateway/internal/transport/handlers_test.go` | Create | Endpoint-level handler tests with `httptest` |
| `llm-agent-memory-gateway/internal/transport/smoke_test.go` | Create | End-to-end in-process smoke test: write → recall → pin → disable → delete → session close |

## Open Decisions Locked For This Plan

- **Router:** stdlib `net/http`, not chi/gin. Reason: existing repo style favors low-dependency Go modules, and first-batch routing needs are simple enough to implement with explicit path parsing.
- **Auth scope transport:** use request headers in M6 for deterministic tests and local execution. Recommended headers:
  - `X-Tenant-Id`
  - `X-User-Id`
  - optional `X-Project-Id`
  - optional `X-Session-Id`
  Client-provided JSON scope remains accepted for shape parity, but service-auth scope is authoritative and overrides conflicting claims.
- **Persistence composition:** first implementation composes directly with `llm-agent-memory-postgres/postgres.Store`; gateway-owned service interfaces should be shaped so a future non-Postgres backend can slot in without rewriting handlers.
- **Recall implementation:** first M6 recall path may use a minimal in-process read strategy over the Postgres truth-source rather than a full vector index. The contract obligation is response shape, scope enforcement, consistency flag handling, and `token_cost_estimate`, not production-quality ranking.
- **Decision trace:** first-batch implementation emits structured logs or in-memory observer events from the service layer with the four required stages:
  - `recalled`
  - `selected`
  - `dropped`
  - `promote_decided`
  No persistence is required in M6.
- **Read-only mode:** implement as config flag in `internal/config`. Recall endpoints remain available; mutating endpoints return `503 read_only_mode`.

## Sequencing Rules

- **Strict TDD.** Every task starts from a failing test or failing command expectation.
- **Gateway-only ownership.** No edits to `llm-agent-memory` or `llm-agent-memory-postgres` unless a concrete bug blocks M6 composition.
- **Auth-derived scope wins.** Service logic must overwrite any conflicting client scope fields before calling backend code.
- **Error contract exactness matters.** The JSON error model and HTTP status mapping are part of the product surface, not a later polish item.
- **Endpoint rollout order:** config/auth/error primitives first, then service layer, then `write`/`patch`/`pin`/`disable`/`delete`, then `session close`, then `recall`, then smoke test/docs.
- **Consistency headers are mandatory.** `X-Request-Id` always; `X-Memory-Version` on single-record mutating success; `X-Consistency-Level` on recall and delete/session-close responses that declare a consistency mode.

## Task Plan

### Task 1: Declare gateway config and command surface

**Files:**
- Create: `llm-agent-memory-gateway/internal/config/config_test.go`
- Create: `llm-agent-memory-gateway/cmd/memory-gateway/main_test.go`
- Modify: `llm-agent-memory-gateway/go.mod`

- [ ] **Step 1: Write the failing config/command surface tests**

```go
package config

import "testing"

func TestConfigSurface_Compiles(t *testing.T) {
	var cfg Config
	_ = cfg.ListenAddr
	_ = cfg.PostgresURL
	_ = cfg.ReadOnly
}
```

```go
package main

import "testing"

func TestCommandPackageCompiles(t *testing.T) {}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-gateway
GOWORK=off GOCACHE=/tmp/go-build go test ./internal/config ./cmd/memory-gateway -count=1
```

Expected: compile failure for missing files/types.

- [ ] **Step 3: Implement minimal config and command scaffold**

Create:

```go
// llm-agent-memory-gateway/internal/config/config.go
package config

type Config struct {
	ListenAddr string
	PostgresURL string
	ReadOnly bool
}
```

```go
// llm-agent-memory-gateway/cmd/memory-gateway/main.go
package main

func main() {}
```

- [ ] **Step 4: Re-run focused tests**

Run the command from Step 2 and expect `PASS`.

- [ ] **Step 5: Commit**

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-gateway
git add go.mod internal/config/config.go internal/config/config_test.go cmd/memory-gateway/main.go cmd/memory-gateway/main_test.go
git commit -m "test(memory-gateway): declare config and command surface"
```

### Task 2: Parse env config and validate required runtime inputs

**Files:**
- Modify: `llm-agent-memory-gateway/internal/config/config.go`
- Modify: `llm-agent-memory-gateway/internal/config/config_test.go`

- [ ] **Step 1: Write failing env parsing tests**

Add tests covering:

```go
func TestLoadFromEnv_RequiresPostgresURL(t *testing.T) {}
func TestLoadFromEnv_DefaultsListenAddr(t *testing.T) {}
func TestLoadFromEnv_ReadOnlyFlag(t *testing.T) {}
```

Expected rules:

- `LLM_AGENT_MEMORY_PG_URL` required
- default `ListenAddr` is `:8080`
- `LLM_AGENT_MEMORY_GATEWAY_READ_ONLY=true` maps to `ReadOnly=true`

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-gateway
GOWORK=off GOCACHE=/tmp/go-build go test ./internal/config -run 'TestLoadFromEnv_' -count=1
```

- [ ] **Step 3: Implement env loading**

Add:

```go
func LoadFromEnv() (Config, error)
```

Environment variables:

- `LLM_AGENT_MEMORY_GATEWAY_ADDR`
- `LLM_AGENT_MEMORY_PG_URL`
- `LLM_AGENT_MEMORY_GATEWAY_READ_ONLY`

- [ ] **Step 4: Re-run focused tests**

Run the command from Step 2 and expect `PASS`.

- [ ] **Step 5: Commit**

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-gateway
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(memory-gateway): load gateway runtime config from env"
```

### Task 3: Lock auth-derived scope precedence

**Files:**
- Create: `llm-agent-memory-gateway/internal/authz/scope.go`
- Create: `llm-agent-memory-gateway/internal/authz/scope_test.go`

- [ ] **Step 1: Write failing scope precedence tests**

Cover:

- missing tenant header -> unauthorized/invalid scope error
- missing user header -> unauthorized/invalid scope error
- client JSON scope cannot override auth-derived tenant/user
- project/session may be filled from headers when absent in JSON

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-gateway
GOWORK=off GOCACHE=/tmp/go-build go test ./internal/authz -count=1
```

- [ ] **Step 3: Implement scope extraction**

Define:

```go
type Scope struct {
	TenantID  string
	UserID    string
	ProjectID string
	SessionID string
}

func ScopeFromHeaders(h http.Header) (Scope, error)
func MergeAuthoritativeScope(auth Scope, claimed Scope) Scope
```

- [ ] **Step 4: Re-run focused tests**

Run the command from Step 2 and expect `PASS`.

- [ ] **Step 5: Commit**

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-gateway
git add internal/authz/scope.go internal/authz/scope_test.go
git commit -m "feat(memory-gateway): enforce auth-derived scope precedence"
```

### Task 4: Lock the JSON error model and transport response helpers

**Files:**
- Create: `llm-agent-memory-gateway/internal/httpapi/errors.go`
- Create: `llm-agent-memory-gateway/internal/httpapi/errors_test.go`

- [ ] **Step 1: Write failing error payload tests**

Cover exact JSON for:

- `bad_request` -> 400
- `unauthorized` -> 401
- `forbidden` -> 403
- `not_found` -> 404
- `memory_conflict` -> 409
- `idempotency_conflict` -> 409
- `read_only_mode` -> 503

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-gateway
GOWORK=off GOCACHE=/tmp/go-build go test ./internal/httpapi -run 'Test(Error|WriteError)' -count=1
```

- [ ] **Step 3: Implement error helpers**

Define:

- `type ErrorResponse struct`
- `type ErrorBody struct`
- gateway-local sentinels or typed wrappers
- `WriteError(w http.ResponseWriter, requestID string, err error)`

- [ ] **Step 4: Re-run focused tests**

Run the command from Step 2 and expect `PASS`.

- [ ] **Step 5: Commit**

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-gateway
git add internal/httpapi/errors.go internal/httpapi/errors_test.go
git commit -m "feat(memory-gateway): add gateway error contract helpers"
```

### Task 5: Define first-batch request/response JSON types

**Files:**
- Create: `llm-agent-memory-gateway/internal/httpapi/types.go`
- Create: `llm-agent-memory-gateway/internal/httpapi/types_test.go`

- [ ] **Step 1: Write failing JSON contract tests**

Cover request/response shapes for:

- recall unified
- write
- patch
- pin
- disable
- delete
- session close

Also assert presence of:

- `consistency_level`
- `expected_version`
- `idempotency_key`
- `token_cost_estimate`

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-gateway
GOWORK=off GOCACHE=/tmp/go-build go test ./internal/httpapi -run 'Test.*JSON' -count=1
```

- [ ] **Step 3: Implement transport types**

Create JSON structs matching the API contract, including:

- `ScopePayload`
- `RecallUnifiedRequest`
- `RecallHitResponse`
- `WriteMemoryRequest`
- `PatchMemoryRequest`
- `PinMemoryRequest`
- `DisableMemoryRequest`
- `DeleteMemoryRequest`
- `SessionCloseRequest`

- [ ] **Step 4: Re-run focused tests**

Run the command from Step 2 and expect `PASS`.

- [ ] **Step 5: Commit**

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-gateway
git add internal/httpapi/types.go internal/httpapi/types_test.go
git commit -m "feat(memory-gateway): define first-batch gateway JSON contracts"
```

### Task 6: Build the gateway service interface and trace seam

**Files:**
- Create: `llm-agent-memory-gateway/internal/service/service.go`
- Create: `llm-agent-memory-gateway/internal/service/service_test.go`

- [ ] **Step 1: Write failing service orchestration tests**

Cover:

- read-only mode blocks mutating calls
- service emits trace events for `recalled`, `selected`, `dropped`, `promote_decided`
- service applies authoritative scope before backend calls

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-gateway
GOWORK=off GOCACHE=/tmp/go-build go test ./internal/service -count=1
```

- [ ] **Step 3: Implement service layer skeleton**

Define:

- `type Service struct`
- `type TraceEmitter interface`
- `type DurableBackend interface`
- method stubs:
  - `RecallUnified`
  - `WriteMemory`
  - `PatchMemory`
  - `PinMemory`
  - `DisableMemory`
  - `DeleteMemory`
  - `CloseSession`

- [ ] **Step 4: Re-run focused tests**

Run the command from Step 2 and expect `PASS`.

- [ ] **Step 5: Commit**

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-gateway
git add internal/service/service.go internal/service/service_test.go
git commit -m "feat(memory-gateway): add service orchestration and trace seam"
```

### Task 7: Add token estimate helper for recall hits

**Files:**
- Create: `llm-agent-memory-gateway/internal/service/token_budget.go`
- Create: `llm-agent-memory-gateway/internal/service/token_budget_test.go`

- [ ] **Step 1: Write failing token estimate tests**

Cover:

- empty content -> zero/low estimate
- longer content -> larger estimate
- estimate is stable/deterministic

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-gateway
GOWORK=off GOCACHE=/tmp/go-build go test ./internal/service -run 'TestToken' -count=1
```

- [ ] **Step 3: Implement minimal deterministic estimator**

Use a simple rune/word-length heuristic. Do not introduce tokenizer dependencies.

- [ ] **Step 4: Re-run focused tests**

Run the command from Step 2 and expect `PASS`.

- [ ] **Step 5: Commit**

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-gateway
git add internal/service/token_budget.go internal/service/token_budget_test.go
git commit -m "feat(memory-gateway): add stable token cost estimate helper"
```

### Task 8: Register routes and middleware with stdlib `net/http`

**Files:**
- Create: `llm-agent-memory-gateway/internal/transport/router.go`
- Create: `llm-agent-memory-gateway/internal/transport/router_test.go`
- Create: `llm-agent-memory-gateway/internal/transport/middleware.go`
- Create: `llm-agent-memory-gateway/internal/transport/middleware_test.go`

- [ ] **Step 1: Write failing route and middleware tests**

Cover:

- all 7 first-batch routes registered
- wrong method returns 405/404 as designed
- request ID added when absent
- response headers written on success paths

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-gateway
GOWORK=off GOCACHE=/tmp/go-build go test ./internal/transport -run 'Test(Route|Middleware)' -count=1
```

- [ ] **Step 3: Implement router and middleware**

Build a stdlib `http.ServeMux`-based router plus helpers for:

- request ID generation/propagation
- JSON content-type
- optional memory-version / consistency-level headers

- [ ] **Step 4: Re-run focused tests**

Run the command from Step 2 and expect `PASS`.

- [ ] **Step 5: Commit**

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-gateway
git add internal/transport/router.go internal/transport/router_test.go internal/transport/middleware.go internal/transport/middleware_test.go
git commit -m "feat(memory-gateway): add stdlib router and response middleware"
```

### Task 9: Implement mutating handlers around the service layer

**Files:**
- Create: `llm-agent-memory-gateway/internal/transport/write_handler.go`
- Create: `llm-agent-memory-gateway/internal/transport/patch_handler.go`
- Create: `llm-agent-memory-gateway/internal/transport/pin_handler.go`
- Create: `llm-agent-memory-gateway/internal/transport/disable_handler.go`
- Create: `llm-agent-memory-gateway/internal/transport/delete_handler.go`
- Modify: `llm-agent-memory-gateway/internal/transport/handlers_test.go`

- [ ] **Step 1: Write failing handler tests for write/patch/pin/disable/delete**

Cover:

- malformed JSON -> `400 bad_request`
- missing auth scope -> `401`/`403`
- read-only mode -> `503 read_only_mode`
- successful mutation writes `X-Memory-Version`
- `expected_version` and `idempotency_key` required where contract says so

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-gateway
GOWORK=off GOCACHE=/tmp/go-build go test ./internal/transport -run 'Test(Write|Patch|Pin|Disable|Delete)Handler' -count=1
```

- [ ] **Step 3: Implement handlers**

Each handler must:

- decode JSON
- extract authoritative scope from headers
- call service
- write contract response
- set `X-Memory-Version` where applicable

- [ ] **Step 4: Re-run focused tests**

Run the command from Step 2 and expect `PASS`.

- [ ] **Step 5: Commit**

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-gateway
git add internal/transport/write_handler.go internal/transport/patch_handler.go internal/transport/pin_handler.go internal/transport/disable_handler.go internal/transport/delete_handler.go internal/transport/handlers_test.go
git commit -m "feat(memory-gateway): add first-batch mutating HTTP handlers"
```

### Task 10: Implement session-close handler

**Files:**
- Create: `llm-agent-memory-gateway/internal/transport/session_handler.go`
- Modify: `llm-agent-memory-gateway/internal/transport/handlers_test.go`

- [ ] **Step 1: Write failing session-close handler tests**

Cover:

- `POST /memory/sessions/{session_id}/close`
- scope mismatch rejected
- success response includes `status=closed`
- `X-Consistency-Level` written when request carries consistency/close semantics

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-gateway
GOWORK=off GOCACHE=/tmp/go-build go test ./internal/transport -run 'TestSessionCloseHandler' -count=1
```

- [ ] **Step 3: Implement handler**

Implement request decode, path parsing, scope merge, service call, and JSON response.

- [ ] **Step 4: Re-run focused tests**

Run the command from Step 2 and expect `PASS`.

- [ ] **Step 5: Commit**

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-gateway
git add internal/transport/session_handler.go internal/transport/handlers_test.go
git commit -m "feat(memory-gateway): add session close handler"
```

### Task 11: Implement recall-unified handler and minimal recall composition

**Files:**
- Create: `llm-agent-memory-gateway/internal/transport/recall_handler.go`
- Modify: `llm-agent-memory-gateway/internal/service/service.go`
- Modify: `llm-agent-memory-gateway/internal/service/service_test.go`
- Modify: `llm-agent-memory-gateway/internal/transport/handlers_test.go`

- [ ] **Step 1: Write failing recall tests**

Cover:

- successful `POST /memory/recall/unified`
- response includes `hits[]`
- each hit includes `token_cost_estimate`
- response includes `X-Consistency-Level`
- `strong` bypass path and `eventual` path are distinguishable in trace metadata

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-gateway
GOWORK=off GOCACHE=/tmp/go-build go test ./internal/service ./internal/transport -run 'TestRecall' -count=1
```

- [ ] **Step 3: Implement minimal recall**

First version may:

- use Postgres truth-source reads as the authoritative visible set
- return a simple ordered result set
- attach `token_cost_estimate`
- emit decision-trace events

Do not add vector index dependencies in this task.

- [ ] **Step 4: Re-run focused tests**

Run the command from Step 2 and expect `PASS`.

- [ ] **Step 5: Commit**

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-gateway
git add internal/transport/recall_handler.go internal/service/service.go internal/service/service_test.go internal/transport/handlers_test.go
git commit -m "feat(memory-gateway): add unified recall endpoint and trace output"
```

### Task 12: Wire the real command composition and in-process smoke test

**Files:**
- Modify: `llm-agent-memory-gateway/cmd/memory-gateway/main.go`
- Modify: `llm-agent-memory-gateway/cmd/memory-gateway/main_test.go`
- Create: `llm-agent-memory-gateway/internal/transport/smoke_test.go`

- [ ] **Step 1: Write failing command/smoke tests**

Cover:

- command fails clearly when config missing
- command can build handler stack from config
- in-process smoke test:
  - write
  - recall
  - pin
  - disable
  - delete
  - session close

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-gateway
GOWORK=off GOCACHE=/tmp/go-build go test ./cmd/memory-gateway ./internal/transport -run 'Test(Command|Smoke)' -count=1
```

- [ ] **Step 3: Implement composition**

`main.go` should:

- call `config.LoadFromEnv()`
- open pgx pool from `LLM_AGENT_MEMORY_PG_URL`
- construct Postgres store
- run migrations or fail fast if configured to require ready schema
- build service + router
- start `http.Server`

- [ ] **Step 4: Re-run focused tests**

Run the command from Step 2 and expect `PASS`.

- [ ] **Step 5: Commit**

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-gateway
git add cmd/memory-gateway/main.go cmd/memory-gateway/main_test.go internal/transport/smoke_test.go
git commit -m "feat(memory-gateway): wire gateway command and smoke coverage"
```

### Task 13: Update gateway docs and verify the module

**Files:**
- Modify: `llm-agent-memory-gateway/README.md`
- Modify: `llm-agent-memory-gateway/doc.go`

- [ ] **Step 1: Write failing doc-presence tests or assertions if useful**

If using tests:

```go
func TestREADME_MentionsFirstBatchEndpoints(t *testing.T) {}
```

Otherwise skip straight to implementation in Step 3 and rely on verification commands.

- [ ] **Step 2: Run any focused doc test if added**

Run the relevant `go test` command, or mark this step not applicable if no doc test is introduced.

- [ ] **Step 3: Update docs**

README should include:

- module purpose
- env vars
- run command
- first-batch endpoints
- testing commands

`doc.go` should describe the package as the HTTP/service composition layer rather than a skeleton.

- [ ] **Step 4: Run full module verification**

Run:

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-gateway
GOWORK=off GOCACHE=/tmp/go-build go test ./... -count=1
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-memory-gateway
git add README.md doc.go
git commit -m "docs(memory-gateway): document first-batch gateway module"
```

## Self-Review

- M6 first-batch 7 endpoints are all covered by Tasks 9–11.
- Auth-derived tenant scope and DB-side enforcement posture are covered by Tasks 3, 6, 9, 10, and 11.
- Error model and required headers are covered by Tasks 4, 8, 9, 10, and 11.
- `consistency_level`, `token_cost_estimate`, and decision trace are covered by Tasks 5, 6, 7, and 11.
- Command startup and smoke coverage are covered by Task 12.
- No SDK or Postgres-backend boundary violation is introduced by the plan itself.

Plan complete and saved to `docs/superpowers/plans/2026-05-26-m6-memory-gateway.md`. Two execution options:

1. Subagent-Driven (recommended) - I dispatch a fresh subagent per task, review between tasks, fast iteration

2. Inline Execution - Execute tasks in this session using executing-plans, batch execution with checkpoints

Which approach?
