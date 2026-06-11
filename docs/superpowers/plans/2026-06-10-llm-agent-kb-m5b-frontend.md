# M5b Frontend React SPA Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a real, tested React 19 SPA in `llm-agent-kb/web/` that drives the kb backend — login (in-memory access token + auto-refresh), org/kb management, document upload with live SSE index progress, token-by-token streaming Q&A (vector/hybrid) plus non-stream global/drift, an eval/drift dashboard, and a sessions history — completing milestone M5 (tagged `v0.5.0` at close-out).

**Architecture:** Vite + React 19 + TypeScript SPA, feature-dir layout (`src/{app,components,features,lib,routes,test}/`). Tailwind CSS v4 via the `@tailwindcss/vite` plugin (no `tailwind.config.js`; theme lives in `src/index.css`). shadcn/ui (radix-nova preset) for primitives. **File-based** routing via `@tanstack/router-plugin` + `@tanstack/router-cli` (`tsr generate`). Server state via TanStack Query; SSE via `@microsoft/fetch-event-source` (POST + Bearer, NOT cached in Query). A single typed `apiFetch` wrapper injects the in-memory access token and does single-flight refresh-on-401. Tests are Vitest + @testing-library/react + jsdom with mocked `fetch` and a mocked SSE helper — NO live backend.

**Tech Stack (versions VERIFIED by actually scaffolding with the real toolchain on 2026-06-10):** Node v22.22.0, pnpm 10.30.3. react 19.2.6, vite 8.0.16, typescript 6.0.3, tailwindcss 4.3.0 + @tailwindcss/vite 4.3.0, @tanstack/react-router 1.170 + @tanstack/router-plugin 1.168 + @tanstack/router-cli 1.167, @tanstack/react-query 5.101, @microsoft/fetch-event-source 2.0.1, react-hook-form 7.78 + zod 4.4 + @hookform/resolvers 5.4, shadcn 4.11 (radix-nova), vitest 4.1.8 + @testing-library/react 16.3 + @testing-library/jest-dom 6.9 + jsdom 29. **This is a Node/npm toolchain — NOT GOWORK.** All work happens on branch `feat/m5a-streaming` (the M5a backend branch; the frontend completes M5 — `v0.5.0` is tagged at M5b close-out).

---

## ENVIRONMENT & TOOLCHAIN NOTES (read before Task 1)

- **Package manager: pnpm** (verified reachable, react@19 installs). All commands below use `pnpm`. The repo dir is `/home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb` (its OWN git repo, gitignored from the umbrella). The SPA lives in `web/` under it.
- **Branch:** stay on `feat/m5a-streaming`. Do NOT branch again. Confirm with `git -C <repo> branch --show-current` → `feat/m5a-streaming`.
- **TypeScript 6 gotcha (VERIFIED):** TS 6.0 deprecates `baseUrl` (emits `TS5101`). Do NOT add `baseUrl` to any tsconfig — the `@/*` path alias works with `paths` alone (paths resolve relative to the tsconfig file). This plan's tsconfig edits omit `baseUrl` deliberately.
- **TanStack route tree (VERIFIED):** the `@tanstack/router-plugin` generates `src/routeTree.gen.ts` at vite dev/build, but `tsc -b` runs FIRST and fails without it. Fix: install `@tanstack/router-cli` and run `tsr generate` before `tsc`/`vitest`. The `dev`/`build`/`test` scripts (Task 1) all prefix `tsr generate`. `src/routeTree.gen.ts` is generated — add it to `.gitignore`.
- **shadcn CLI (VERIFIED current behavior):** `pnpm dlx shadcn@latest init` is interactive and prompts for a "preset" even with `-y`. Use `init -b radix -p nova -y` for a non-interactive init. shadcn detects the existing Vite project, validates Tailwind v4 + the `@/` alias, writes `components.json` (style `radix-nova`), and rewrites `src/index.css` with the theme. `add` works the same: `pnpm dlx shadcn@latest add button ... -y`.
- **eslint:** Vite scaffolds a flat `eslint.config.js`. shadcn's generated `src/components/ui/*` files export both a component and a `*Variants` const → they trip `react-refresh/only-export-components`. The plan ignores `src/components/ui/**` and `src/routeTree.gen.ts` in eslint config (Task 1) so `pnpm lint` is green on hand-written code.

---

## THE BACKEND WIRE CONTRACT (verified against the real Go source — these are the exact shapes the SPA types must match)

Base URL in dev: the Vite proxy forwards `/api/*` → `http://localhost:8080`. Every authenticated request carries `Authorization: Bearer <access>`. List envelope is `{ items: T[], next_cursor: string }`.

**Auth** (`llm-agent-authz` httpapi, mounted at `/api/auth`):
- `POST /api/auth/login` body `{"Email": string, "Password": string}` (PascalCase — Go decodes into `struct{Email,Password string}`) → `200 {"access_token": string, "expires_in": number}` and a `Set-Cookie: authz_refresh=...; HttpOnly; Path=/api/auth`. On bad creds → `401`.
- `POST /api/auth/refresh` → reads the httpOnly cookie, **requires header `X-CSRF: 1`** (double-submit guard; without it → `403`) → `200 {"access_token", "expires_in"}` and a rotated cookie. On expired/invalid → `401`.
- `POST /api/auth/logout` → **requires `X-CSRF: 1`**, clears the cookie → `204`.
- The access token is kept IN MEMORY only (never localStorage). The refresh cookie is httpOnly and sent automatically by the browser (same-origin via the proxy).

**Orgs / KB:**
- `POST /api/orgs` body `{"name": string}` → `200 {"id", "name"}`. Any authenticated user; creator becomes org_admin.
- `POST /api/orgs/{org}/kbs` body `{"name": string, "embeddingModel"?: string, "embeddingDim"?: number}` → `200 {"id","orgId","name","namespace"}`.
- `GET /api/orgs/{org}/kbs?limit=&cursor=` → `200 {"items":[{"id","orgId","name","namespace"}], "next_cursor"}`.
- `GET /api/kb/{id}` → `200 {"id","orgId","name","namespace"}`. `DELETE /api/kb/{id}` (admin) → `204`.

**Documents:**
- `GET /api/kb/{id}/documents?limit=&cursor=` → `200 {"items": DocumentView[], "next_cursor"}` where `DocumentView = {id, title, sourceType, status, phase, error?, chunkCount}`.
- `POST /api/kb/{id}/documents` body `{"title", "sourceType":"paste"|"url"|"pdf"|"docx", "content", "url"?, "filename"?}` → `202 {"documentId", "status":"pending"}`. (For paste, `content` is the text. For url, set `url`. File types out of M5b's primary scope but the form supports the shape.)
- `GET /api/kb/{id}/documents/{docId}` → `200 {"id","status","phase","chunkCount","error"}`.
- `GET /api/kb/{id}/documents/{docId}/progress` → SSE; each frame is `data: {"status","phase","chunkCount","error"}` (no `event:` line for progress frames); terminal when `status` is `"ready"` or `"failed"`; an `event: error\ndata: {"error":"not found"}` frame on lookup failure.
- `POST /api/kb/{id}/documents/{docId}/retry` → `202 {"documentId","status":"pending"}`. `DELETE /api/kb/{id}/documents/{docId}` → `204`.

**Ask:**
- `POST /api/kb/{id}/ask` body `{"q","mode":"vector"|"hybrid","topK"?,"sessionId"?}` → `200 {"answer","citations":Citation[],"diagnostics":object,"sessionId"?}`.
- `POST /api/kb/{id}/ask/global` body `{"q","maxCommunities"?,"sessionId"?}` → same `200` shape.
- `POST /api/kb/{id}/ask/drift` body `{"q","maxCommunities"?,"rounds"?,"topK"?,"sessionId"?}` → same `200` shape.
- `POST /api/kb/{id}/ask/stream` body `{"q","mode":"vector"|"hybrid","topK"?,"sessionId"?}` → SSE. **Wire contract (from the M5a plan, confirmed):** zero+ `event: token\ndata: {"text": string}\n\n` frames, then exactly ONE terminal `event: done\ndata: {"citations":Citation[],"diagnostics":{"mode":string,"hitCount":number},"sessionId":string}\n\n` OR `event: error\ndata: {"error":string}\n\n`. Concatenating all `token.text` in arrival order yields the full answer. Pre-stream failures (bad body/mode) return HTTP 4xx before any SSE byte.
- `Citation = {chunkId, docId, title, sectionPath?: string[], score: number, snippet: string}` (lowerCamel; `sectionPath` omitted when empty).

**Sessions / Eval:**
- `GET /api/kb/{id}/sessions?limit=&cursor=` → `200 {"items":[{"id","title","createdAt"}], "next_cursor"}`.
- `GET /api/kb/{id}/sessions/{sid}` → `200 {"sessionId", "messages":[{"id","role","content","mode","createdAt","citations"?}]}` (`citations` is the raw `Citation[]` JSON when present).
- `POST /api/kb/{id}/eval/run` body `{"kind":"retrieval"|"triad"|"global"|"drift", "dataset": string /* inline JSONL */}` → `200 {"runId", "result": EvalResult}`.
- `GET /api/kb/{id}/eval/runs?limit=&cursor=` → `200 {"items":[{"id","kind","datasetName","createdAt","metrics":object,"drift"?:object}], "next_cursor"}`.
- `EvalResult = {kind, datasetName, retrieval?: {precisionAtK, recallAtK, mrr, groundingAtK, examples, topK}, generation?: {meanGroundedness, meanAnswerRelevance, examples}, drift?: {dataset, deltas: {name, prev: number|null, curr: number|null, delta: number|null, direction: "improved"|"regressed"|"unchanged"}[], histograms: {name, prev: number[], curr: number[], delta: number[], l1Distance: number}[], newExamples: string[], droppedExamples: string[]}}`.

---

## File Structure

The SPA is created under `web/`. After scaffolding, the tree (hand-written files only; `node_modules/`, `dist/`, `src/routeTree.gen.ts` are generated) is:

```
web/
├── package.json                  # scripts: dev/build/test/lint (Task 1)
├── vite.config.ts                # react + tailwindcss + tanstackRouter plugins, /api proxy, vitest config (Task 1)
├── tsconfig.json                 # @/* path alias (Task 1)
├── tsconfig.app.json             # @/* path alias (Task 1)
├── eslint.config.js              # ignores ui/** + routeTree.gen.ts (Task 1)
├── components.json               # shadcn config (Task 2, generated by init)
├── index.html
├── .gitignore                    # adds routeTree.gen.ts (Task 1)
├── README.md                     # web docs (Task 15)
└── src/
    ├── index.css                 # @import "tailwindcss" + shadcn theme (Tasks 1/2)
    ├── main.tsx                   # Router + QueryClient providers (Task 3)
    ├── routeTree.gen.ts          # GENERATED by tsr — do not edit, gitignored
    ├── test/
    │   ├── setup.ts              # jest-dom matchers (Task 1)
    │   └── helpers.ts            # mockFetch + mockSSE test helpers (Task 4)
    ├── lib/
    │   ├── utils.ts              # cn() (Task 2, written by shadcn)
    │   ├── types.ts             # API response types (Task 4)
    │   ├── apiClient.ts          # apiFetch wrapper + token store + refresh single-flight (Task 4)
    │   ├── apiClient.test.ts     # refresh-on-401 unit tests (Task 4)
    │   ├── sse.ts                # streamAsk() SSE helper (Task 5)
    │   └── sse.test.ts           # SSE frame-parsing unit tests (Task 5)
    ├── components/
    │   └── ui/                   # shadcn primitives (Task 2)
    ├── app/
    │   ├── auth.tsx             # AuthProvider + useAuth context (Task 6)
    │   ├── auth.test.tsx         # auth context test (Task 6)
    │   └── AppShell.tsx          # nav layout (Task 8)
    ├── features/
    │   ├── orgkb/
    │   │   ├── api.ts            # org/kb queries+mutations (Task 9)
    │   │   ├── KbListPage.tsx     # list + create org/kb (Task 9)
    │   │   └── KbListPage.test.tsx
    │   ├── documents/
    │   │   ├── api.ts            # document queries+mutations (Task 10)
    │   │   ├── UploadForm.tsx     # paste/url/file upload (Task 10)
    │   │   ├── UploadForm.test.tsx
    │   │   ├── DocStatusTable.tsx # list + status badge + retry/delete + live progress (Task 11)
    │   │   └── DocStatusTable.test.tsx
    │   ├── ask/
    │   │   ├── api.ts            # ask (non-stream) mutations (Task 12)
    │   │   ├── useAskStream.ts    # streaming hook over lib/sse (Task 12)
    │   │   ├── useAskStream.test.ts
    │   │   ├── AskPage.tsx        # ModeSwitch + input + AnswerPane wiring (Task 13)
    │   │   ├── AnswerPane.tsx     # streamed answer text (Task 13)
    │   │   ├── CitationList.tsx   # citations (Task 13)
    │   │   ├── DiagnosticsDrawer.tsx
    │   │   └── AskPage.test.tsx   # streaming render with mocked SSE (Task 13)
    │   ├── eval/
    │   │   ├── api.ts            # eval run/list (Task 14)
    │   │   ├── MetricCards.tsx    # precision/recall/MRR/grounding + triad (Task 14)
    │   │   ├── DriftReportTable.tsx
    │   │   ├── RunEvalDialog.tsx
    │   │   ├── EvalPage.tsx
    │   │   └── MetricCards.test.tsx
    │   └── sessions/
    │       ├── api.ts            # sessions list + transcript (Task 15)
    │       ├── SessionsPage.tsx
    │       └── SessionsPage.test.tsx
    └── routes/                    # TanStack file-based routes (Tasks 3/7/8/9/...)
        ├── __root.tsx
        ├── login.tsx
        ├── _authed.tsx           # protected layout (redirect to /login)
        ├── _authed/index.tsx     # "/" → KbListPage
        └── _authed/kb.$kbId/
            ├── documents.tsx
            ├── ask.tsx
            ├── eval.tsx
            └── sessions.tsx
```

Routing decision is justified in Task 3.

---

### Task 1: Scaffold the Vite project + verify build/test green

**Files:**
- Create: `web/` (Vite scaffold)
- Modify: `web/vite.config.ts`, `web/tsconfig.json`, `web/tsconfig.app.json`, `web/package.json`, `web/eslint.config.js`, `web/.gitignore`
- Create: `web/src/index.css`, `web/src/test/setup.ts`, `web/src/lib/utils.ts`, `web/src/routes/__root.tsx`, `web/src/routes/index.tsx`, `web/src/main.tsx`
- Delete: `web/src/App.tsx`, `web/src/App.css`

- [ ] **Step 1: Confirm the branch**

Run:
```bash
git -C /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb branch --show-current
```
Expected: `feat/m5a-streaming`. If not, STOP and check out that branch first.

- [ ] **Step 2: Scaffold Vite React-TS**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb && pnpm create vite@latest web --template react-ts
```
Expected: ends with `Done. Now run: cd web ...`. Creates `web/` with React 19 + TS 6 + Vite 8.

- [ ] **Step 3: Install base deps + all runtime/dev deps**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb/web && pnpm install && pnpm add tailwindcss @tailwindcss/vite @tanstack/react-router @tanstack/react-query @microsoft/fetch-event-source react-hook-form zod @hookform/resolvers class-variance-authority clsx tailwind-merge lucide-react && pnpm add -D @tanstack/router-plugin @tanstack/router-cli @tanstack/react-router-devtools vitest @testing-library/react @testing-library/user-event @testing-library/jest-dom jsdom @vitest/coverage-v8
```
Expected: both `pnpm add` commands end with `Done in ...`. (NOTE: the package is `jsdom`, NOT `@testing-library/jsdom` — that does not exist. The matcher package is `@testing-library/jest-dom`.)

- [ ] **Step 4: Write `vite.config.ts`**

Create `web/vite.config.ts`:
```ts
/// <reference types="vitest/config" />
import path from "node:path"
import { defineConfig } from "vite"
import react from "@vitejs/plugin-react"
import tailwindcss from "@tailwindcss/vite"
import { tanstackRouter } from "@tanstack/router-plugin/vite"

export default defineConfig({
  plugins: [
    tanstackRouter({ target: "react", autoCodeSplitting: true }),
    react(),
    tailwindcss(),
  ],
  resolve: { alias: { "@": path.resolve(__dirname, "./src") } },
  server: {
    proxy: { "/api": { target: "http://localhost:8080", changeOrigin: true } },
  },
  test: {
    globals: true,
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
  },
})
```

- [ ] **Step 5: Add the `@/*` alias to both tsconfigs (NO baseUrl)**

In `web/tsconfig.app.json`, add inside `compilerOptions` (right after the `"jsx": "react-jsx",` line):
```json
    "paths": { "@/*": ["./src/*"] },
```
In `web/tsconfig.json`, add a `compilerOptions` block before `"files"` (the file currently has only `files`/`references`):
```json
  "compilerOptions": {
    "paths": { "@/*": ["./src/*"] }
  },
```
DO NOT add `"baseUrl"` anywhere — TS 6 deprecates it and `tsc -b` errors with `TS5101`.

- [ ] **Step 6: Set the npm scripts**

In `web/package.json`, replace the `scripts` block with:
```json
  "scripts": {
    "dev": "tsr generate && vite",
    "build": "tsr generate && tsc -b && vite build",
    "preview": "vite preview",
    "lint": "eslint .",
    "test": "tsr generate && vitest run",
    "test:watch": "tsr generate && vitest"
  },
```

- [ ] **Step 7: Write the Tailwind v4 entry CSS, test setup, and cn() util**

Create `web/src/index.css`:
```css
@import "tailwindcss";
```
Create `web/src/test/setup.ts`:
```ts
import "@testing-library/jest-dom/vitest"
```
Create `web/src/lib/utils.ts`:
```ts
import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}
```

- [ ] **Step 8: Replace App with minimal file-based routes + providers**

Delete the scaffold files:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb/web && rm -f src/App.tsx src/App.css
```
Create `web/src/routes/__root.tsx`:
```tsx
import { createRootRoute, Outlet } from "@tanstack/react-router"

export const Route = createRootRoute({ component: () => <Outlet /> })
```
Create `web/src/routes/index.tsx`:
```tsx
import { createFileRoute } from "@tanstack/react-router"

export const Route = createFileRoute("/")({
  component: () => <div className="p-6 text-xl font-bold">llm-agent-kb</div>,
})
```
Create `web/src/main.tsx`:
```tsx
import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { RouterProvider, createRouter } from "@tanstack/react-router"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { routeTree } from "./routeTree.gen"
import "./index.css"

const router = createRouter({ routeTree })
declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router
  }
}

const queryClient = new QueryClient()

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  </StrictMode>,
)
```

- [ ] **Step 9: Ignore the generated route tree + ui/** in eslint and git**

In `web/eslint.config.js`, find the existing `globalIgnores([...])` or top-level ignores (the Vite flat config has `globalIgnores(['dist'])`). Replace that ignore list to include the generated tree and shadcn ui dir:
```js
  globalIgnores(['dist', 'src/routeTree.gen.ts', 'src/components/ui']),
```
(If the config uses `{ ignores: [...] }` form instead, add the same three entries to that array.)

Append to `web/.gitignore`:
```
src/routeTree.gen.ts
```

- [ ] **Step 10: Verify the production build passes**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb/web && pnpm build
```
Expected: `tsr generate` runs, then `tsc -b` (silent), then `vite build` prints `✓ built in ...` with `dist/assets/index-*.css` and `dist/assets/index-*.js`. NO `TS5101`/`TS2307` errors.

- [ ] **Step 11: Add a smoke test and verify Vitest passes**

Create `web/src/lib/utils.test.ts`:
```ts
import { describe, it, expect } from "vitest"
import { cn } from "./utils"

describe("cn", () => {
  it("merges and dedupes conflicting tailwind classes", () => {
    expect(cn("p-2", "p-4")).toBe("p-4")
    expect(cn("text-sm", "font-bold")).toBe("text-sm font-bold")
  })
})
```
Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb/web && pnpm test
```
Expected: `Test Files  1 passed (1)` / `Tests  1 passed (1)`.

- [ ] **Step 12: Commit**

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb && git add web/ && git commit -m "feat(web): scaffold Vite+React19+TS SPA with Tailwind v4, TanStack Router/Query, Vitest

File-based routing via tsr generate (run before tsc/vitest because the
plugin only emits routeTree.gen.ts at vite phase). /api dev proxy to the
Go backend on :8080. baseUrl omitted (TS6 deprecates it; paths alias
suffices). Build + Vitest verified green."
```

---

### Task 2: Initialize shadcn/ui + add the primitives the app uses

**Files:**
- Create: `web/components.json` (by `init`), `web/src/components/ui/*.tsx` (by `add`)
- Modify: `web/src/index.css` (rewritten by `init`), `web/src/lib/utils.ts` (rewritten by `init`, identical cn())

- [ ] **Step 1: Run the non-interactive shadcn init**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb/web && pnpm dlx shadcn@latest init -b radix -p nova -y
```
Expected: prints `Validating Tailwind CSS. Found v4.`, `Validating import alias.`, `Writing components.json.`, `Updating src/index.css`, `Project initialization completed.` It rewrites `src/index.css` (adds `@import "tw-animate-css"; @import "shadcn/tailwind.css";` + the `@theme` block + light/dark CSS vars) and confirms `src/lib/utils.ts`. (`-p nova` is required — `init` prompts for a preset even with `-y`; `nova` is the recommended Lucide/Geist default.)

- [ ] **Step 2: Add the UI primitives the app needs**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb/web && pnpm dlx shadcn@latest add button card input label dialog table badge textarea tabs sonner -y
```
Expected: `Created N files:` listing `src/components/ui/button.tsx`, `card.tsx`, `input.tsx`, `label.tsx`, `dialog.tsx`, `table.tsx`, `badge.tsx`, `textarea.tsx`, `tabs.tsx`, `sonner.tsx`.

- [ ] **Step 3: Verify build still passes (Tailwind v4 + shadcn + React 19 together)**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb/web && pnpm build
```
Expected: `✓ built in ...`; the CSS chunk grows (~35 kB, theme + font) and Geist `.woff2` assets appear. No type errors.

- [ ] **Step 4: Add a render test proving the toolchain (RTL + jsdom + shadcn) works**

Create `web/src/components/ui/button.smoke.test.tsx`:
```tsx
import { describe, it, expect } from "vitest"
import { render, screen } from "@testing-library/react"
import { Button } from "./button"

describe("shadcn Button", () => {
  it("renders its children as a button", () => {
    render(<Button>Save</Button>)
    expect(screen.getByRole("button", { name: "Save" })).toBeInTheDocument()
  })
})
```
Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb/web && pnpm test
```
Expected: `Test Files  2 passed (2)`.

- [ ] **Step 5: Commit**

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb && git add web/ && git commit -m "feat(web): shadcn/ui init (radix-nova) + primitives

init -b radix -p nova (preset required even with -y). Adds button/card/
input/label/dialog/table/badge/textarea/tabs/sonner. Build + RTL render
test verified green."
```

---

### Task 3: Routing model — file-based; root + index wiring

**ROUTING DECISION: file-based (TanStack `@tanstack/router-plugin` + `tsr generate`).**

**Justification:** the spec (§10) prescribes TanStack Router with a feature-dir structure and named routes like `/kb/$kbId/ask`. File-based routing makes that 1:1 — each route is a file under `src/routes/`, the plugin generates a fully-typed `routeTree.gen.ts`, and `$kbId` params are type-safe via `Route.useParams()` with zero manual route-tree maintenance. Code-based routing would require hand-writing and hand-linking a growing route tree (8+ routes across 5 features) — more boilerplate, more drift risk, and no benefit here. File-based is the documented default for Vite + the router plugin and is what I verified builds.

**Files:**
- Already created in Task 1: `web/src/routes/__root.tsx`, `web/src/routes/index.tsx`. This task only confirms the model and adds devtools (optional, dev-only) — no behavior change, so it folds into later tasks. **No separate commit; skip to Task 4.** (This task is documentation of the decision; it carries no code beyond Task 1.)

---

### Task 4: API types + apiFetch wrapper with single-flight refresh-on-401 (TDD)

**Files:**
- Create: `web/src/lib/types.ts`
- Create: `web/src/lib/apiClient.ts`
- Create: `web/src/lib/apiClient.test.ts`
- Create: `web/src/test/helpers.ts`

Design: `apiClient` holds the access token in a module-level variable (in memory only). `apiFetch(path, init)` injects `Authorization: Bearer <token>`; on a `401` it calls `refresh()` exactly once even under concurrent callers (single-flight via a shared promise), then retries the original request once. `refresh()` POSTs `/api/auth/refresh` with `X-CSRF: 1` and `credentials: "include"` (sends the httpOnly cookie). If refresh fails, the token is cleared and the error propagates (callers redirect to login).

- [ ] **Step 1: Write the API response types**

Create `web/src/lib/types.ts`:
```ts
// Mirrors the Go backend wire shapes (verified against the kb source).

export interface ListEnvelope<T> {
  items: T[]
  next_cursor: string
}

export interface LoginResponse {
  access_token: string
  expires_in: number
}

export interface Org {
  id: string
  name: string
}

export interface Kb {
  id: string
  orgId: string
  name: string
  namespace: string
}

export interface DocumentView {
  id: string
  title: string
  sourceType: string
  status: string
  phase: string
  error?: string
  chunkCount: number
}

export interface Citation {
  chunkId: string
  docId: string
  title: string
  sectionPath?: string[]
  score: number
  snippet: string
}

export interface AskResponse {
  answer: string
  citations: Citation[]
  diagnostics: Record<string, unknown>
  sessionId?: string
}

// SSE stream frame payloads (POST /ask/stream wire contract from M5a).
export interface StreamTokenData {
  text: string
}
export interface StreamDoneData {
  citations: Citation[]
  diagnostics: { mode: string; hitCount: number }
  sessionId: string
}
export interface StreamErrorData {
  error: string
}

export interface SessionRow {
  id: string
  title: string
  createdAt: string
}
export interface TranscriptMessage {
  id: string
  role: string
  content: string
  mode: string
  createdAt: string
  citations?: Citation[]
}

export interface RetrievalMetrics {
  precisionAtK: number
  recallAtK: number
  mrr: number
  groundingAtK: number
  examples: number
  topK: number
}
export interface GenerationMetrics {
  meanGroundedness: number
  meanAnswerRelevance: number
  examples: number
}
export type DriftDirection = "improved" | "regressed" | "unchanged"
export interface MetricDelta {
  name: string
  prev: number | null
  curr: number | null
  delta: number | null
  direction: DriftDirection
}
export interface DriftView {
  dataset: string
  deltas: MetricDelta[]
  histograms: {
    name: string
    prev: number[]
    curr: number[]
    delta: number[]
    l1Distance: number
  }[]
  newExamples: string[]
  droppedExamples: string[]
}
export interface EvalResult {
  kind: "retrieval" | "triad" | "global" | "drift"
  datasetName: string
  retrieval?: RetrievalMetrics
  generation?: GenerationMetrics
  drift?: DriftView
}
export interface EvalRunRow {
  id: string
  kind: string
  datasetName: string
  createdAt: string
  metrics: Record<string, unknown>
  drift?: DriftView
}
```

- [ ] **Step 2: Write the failing apiClient test**

Create `web/src/lib/apiClient.test.ts`:
```ts
import { describe, it, expect, beforeEach, vi } from "vitest"
import { apiFetch, setAccessToken, getAccessToken } from "./apiClient"

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

describe("apiFetch", () => {
  beforeEach(() => {
    setAccessToken(null)
    vi.restoreAllMocks()
  })

  it("injects the Authorization header from the in-memory token", async () => {
    setAccessToken("tok-1")
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ ok: true }))
    vi.stubGlobal("fetch", fetchMock)

    await apiFetch("/api/kb/x")

    const [, init] = fetchMock.mock.calls[0]
    expect((init.headers as Headers).get("Authorization")).toBe("Bearer tok-1")
  })

  it("refreshes once on 401 then retries the original request", async () => {
    setAccessToken("stale")
    const fetchMock = vi
      .fn()
      // 1st: original request → 401
      .mockResolvedValueOnce(new Response("unauthorized", { status: 401 }))
      // 2nd: refresh → new token
      .mockResolvedValueOnce(jsonResponse({ access_token: "fresh", expires_in: 900 }))
      // 3rd: retried original → 200
      .mockResolvedValueOnce(jsonResponse({ ok: true }))
    vi.stubGlobal("fetch", fetchMock)

    const res = await apiFetch("/api/kb/x")
    expect(res.status).toBe(200)
    expect(getAccessToken()).toBe("fresh")
    // refresh call carried the X-CSRF header + credentials
    const refreshInit = fetchMock.mock.calls[1][1]
    expect((refreshInit.headers as Headers).get("X-CSRF")).toBe("1")
    expect(refreshInit.credentials).toBe("include")
    // retried request used the fresh token
    const retryInit = fetchMock.mock.calls[2][1]
    expect((retryInit.headers as Headers).get("Authorization")).toBe("Bearer fresh")
  })

  it("single-flights refresh under concurrent 401s (refresh called once)", async () => {
    setAccessToken("stale")
    let refreshCount = 0
    const fetchMock = vi.fn((input: string) => {
      const url = String(input)
      if (url.endsWith("/api/auth/refresh")) {
        refreshCount++
        return Promise.resolve(jsonResponse({ access_token: "fresh", expires_in: 900 }))
      }
      // first hit per caller is 401 (stale), retry (fresh) is 200
      return Promise.resolve(
        getAccessTokenInternal() === "fresh"
          ? jsonResponse({ ok: true })
          : new Response("unauthorized", { status: 401 }),
      )
    })
    // helper to read the live token inside the mock
    function getAccessTokenInternal() {
      return getAccessToken()
    }
    vi.stubGlobal("fetch", fetchMock)

    const [a, b] = await Promise.all([apiFetch("/api/kb/a"), apiFetch("/api/kb/b")])
    expect(a.status).toBe(200)
    expect(b.status).toBe(200)
    expect(refreshCount).toBe(1)
  })

  it("clears the token and throws when refresh fails", async () => {
    setAccessToken("stale")
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(new Response("unauthorized", { status: 401 }))
      .mockResolvedValueOnce(new Response("unauthorized", { status: 401 })) // refresh fails
    vi.stubGlobal("fetch", fetchMock)

    await expect(apiFetch("/api/kb/x")).rejects.toThrow()
    expect(getAccessToken()).toBeNull()
  })
})
```

- [ ] **Step 3: Run the test to verify it fails**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb/web && pnpm vitest run src/lib/apiClient.test.ts
```
Expected: FAIL — `Failed to resolve import "./apiClient"` / `apiFetch is not a function`.

- [ ] **Step 4: Implement the apiClient**

Create `web/src/lib/apiClient.ts`:
```ts
// In-memory access token (never persisted to localStorage). The refresh token
// lives in an httpOnly cookie set by /api/auth/login and is sent automatically.
let accessToken: string | null = null
let refreshInFlight: Promise<string> | null = null

export function setAccessToken(t: string | null): void {
  accessToken = t
}
export function getAccessToken(): string | null {
  return accessToken
}

export class AuthError extends Error {}

// refresh POSTs /api/auth/refresh (cookie-driven). The backend requires the
// X-CSRF double-submit header and the httpOnly cookie (credentials:include).
// Single-flight: concurrent callers share one in-flight refresh promise.
async function refresh(): Promise<string> {
  if (refreshInFlight) return refreshInFlight
  refreshInFlight = (async () => {
    try {
      const res = await fetch("/api/auth/refresh", {
        method: "POST",
        headers: new Headers({ "X-CSRF": "1" }),
        credentials: "include",
      })
      if (!res.ok) throw new AuthError("refresh failed")
      const body = (await res.json()) as { access_token: string }
      setAccessToken(body.access_token)
      return body.access_token
    } finally {
      refreshInFlight = null
    }
  })()
  return refreshInFlight
}

function withAuth(init: RequestInit | undefined, token: string | null): RequestInit {
  const headers = new Headers(init?.headers)
  if (token) headers.set("Authorization", `Bearer ${token}`)
  return { ...init, headers, credentials: "include" }
}

// apiFetch injects the bearer token and transparently refreshes once on a 401,
// then retries the original request a single time. A failed refresh clears the
// token and throws AuthError (callers redirect to /login).
export async function apiFetch(path: string, init?: RequestInit): Promise<Response> {
  let res = await fetch(path, withAuth(init, accessToken))
  if (res.status !== 401) return res
  let fresh: string
  try {
    fresh = await refresh()
  } catch (e) {
    setAccessToken(null)
    throw e instanceof AuthError ? e : new AuthError(String(e))
  }
  res = await fetch(path, withAuth(init, fresh))
  return res
}

// apiJSON is the typed convenience wrapper used by feature api modules.
export async function apiJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await apiFetch(path, init)
  if (!res.ok) {
    const text = await res.text().catch(() => "")
    throw new Error(`${res.status} ${path}: ${text || res.statusText}`)
  }
  return (await res.json()) as T
}
```

- [ ] **Step 5: Write the shared test helpers (mockFetch + mockSSE)**

Create `web/src/test/helpers.ts`:
```ts
import { vi } from "vitest"

export function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

// installFetchRoutes stubs global fetch with a path→response map. Each value is
// either a Response or a function (path, init) => Response, called per request.
type RouteValue = Response | ((path: string, init?: RequestInit) => Response)
export function installFetchRoutes(routes: Record<string, RouteValue>) {
  const mock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    const path = String(input)
    for (const [pattern, value] of Object.entries(routes)) {
      if (path.includes(pattern)) {
        const r = typeof value === "function" ? value(path, init) : value
        return Promise.resolve(r)
      }
    }
    return Promise.resolve(new Response("no route", { status: 404 }))
  })
  vi.stubGlobal("fetch", mock)
  return mock
}
```

- [ ] **Step 6: Run the test to verify it passes**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb/web && pnpm vitest run src/lib/apiClient.test.ts
```
Expected: all 4 `apiFetch` tests PASS.

- [ ] **Step 7: Commit**

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb && git add web/src/lib/types.ts web/src/lib/apiClient.ts web/src/lib/apiClient.test.ts web/src/test/helpers.ts && git commit -m "feat(web): typed apiFetch with in-memory token + single-flight refresh-on-401

Access token in memory only; refresh hits /api/auth/refresh with the
X-CSRF header + credentials:include (httpOnly cookie). Concurrent 401s
share one refresh promise; failed refresh clears the token. Mirrors the
verified Go wire shapes in types.ts. Shared mockFetch test helper added."
```

---

### Task 5: SSE streaming helper over @microsoft/fetch-event-source (TDD)

**Files:**
- Create: `web/src/lib/sse.ts`
- Create: `web/src/lib/sse.test.ts`

Design: `streamAsk` POSTs to a stream endpoint with the Bearer token and parses `event: token|done|error` frames. It uses `fetchEventSource` (which supports POST + custom headers, unlike native `EventSource`). The `EventSourceMessage` has `{ id, event, data }`; we switch on `msg.event` and `JSON.parse(msg.data)`. To keep the parser unit-testable WITHOUT the network, `streamAsk` takes an injectable `client` (defaulting to `fetchEventSource`) with the same signature — the test passes a fake that synthesizes frames.

- [ ] **Step 1: Write the failing test**

Create `web/src/lib/sse.test.ts`:
```ts
import { describe, it, expect, vi } from "vitest"
import { streamAsk } from "./sse"
import type { StreamDoneData } from "./types"

// A fake fetchEventSource: replays a scripted list of {event,data} frames
// through the provided onmessage, then calls onclose.
function fakeClient(frames: { event: string; data: string }[]) {
  return vi.fn(async (_url: string, init: any) => {
    await init.onopen?.(new Response(null, { status: 200, headers: { "Content-Type": "text/event-stream" } }))
    for (const f of frames) init.onmessage?.({ id: "", event: f.event, data: f.data })
    init.onclose?.()
  })
}

describe("streamAsk", () => {
  it("forwards token deltas in order and resolves with the done payload", async () => {
    const client = fakeClient([
      { event: "token", data: JSON.stringify({ text: "hello " }) },
      { event: "token", data: JSON.stringify({ text: "world" }) },
      {
        event: "done",
        data: JSON.stringify({
          citations: [{ chunkId: "c1", docId: "d1", title: "Doc", score: 0.9, snippet: "s" }],
          diagnostics: { mode: "hybrid", hitCount: 1 },
          sessionId: "sid-1",
        }),
      },
    ])
    const tokens: string[] = []
    let done: StreamDoneData | undefined
    await streamAsk(
      "/api/kb/demo/ask/stream",
      { q: "fox", mode: "hybrid", topK: 5 },
      "tok",
      { onToken: (t) => tokens.push(t), onDone: (d) => (done = d) },
      client,
    )
    expect(tokens).toEqual(["hello ", "world"])
    expect(done?.sessionId).toBe("sid-1")
    expect(done?.citations[0].chunkId).toBe("c1")
    // the request body + bearer were passed through
    const init = (client.mock.calls[0][1]) as any
    expect(init.method).toBe("POST")
    expect((init.headers as Record<string, string>)["Authorization"]).toBe("Bearer tok")
    expect(JSON.parse(init.body).mode).toBe("hybrid")
  })

  it("invokes onError on an error frame", async () => {
    const client = fakeClient([{ event: "error", data: JSON.stringify({ error: "boom" }) }])
    let err = ""
    await streamAsk(
      "/api/kb/demo/ask/stream",
      { q: "x", mode: "vector" },
      "tok",
      { onToken: () => {}, onDone: () => {}, onError: (m) => (err = m) },
      client,
    )
    expect(err).toBe("boom")
  })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb/web && pnpm vitest run src/lib/sse.test.ts
```
Expected: FAIL — `Failed to resolve import "./sse"`.

- [ ] **Step 3: Implement the SSE helper**

Create `web/src/lib/sse.ts`:
```ts
import { fetchEventSource } from "@microsoft/fetch-event-source"
import type { StreamDoneData, StreamTokenData, StreamErrorData } from "./types"

export interface AskStreamBody {
  q: string
  mode: "vector" | "hybrid"
  topK?: number
  sessionId?: string
}

export interface StreamHandlers {
  onToken: (text: string) => void
  onDone: (done: StreamDoneData) => void
  onError?: (message: string) => void
}

// The injectable client signature (matches fetchEventSource). Tests pass a fake.
type EventSourceClient = (
  url: string,
  init: {
    method: string
    headers: Record<string, string>
    body: string
    signal?: AbortSignal
    openWhenHidden?: boolean
    onopen?: (res: Response) => Promise<void> | void
    onmessage?: (ev: { id: string; event: string; data: string }) => void
    onclose?: () => void
    onerror?: (err: unknown) => number | void
  },
) => Promise<void>

// streamAsk opens the POST SSE stream with the Bearer token and dispatches the
// token/done/error wire frames (M5a contract). Returns when the stream closes.
// `signal` lets callers abort (e.g. component unmount / new question).
export async function streamAsk(
  url: string,
  body: AskStreamBody,
  accessToken: string,
  handlers: StreamHandlers,
  client: EventSourceClient = fetchEventSource as unknown as EventSourceClient,
  signal?: AbortSignal,
): Promise<void> {
  await client(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${accessToken}`,
    },
    body: JSON.stringify(body),
    signal,
    openWhenHidden: true,
    onmessage(ev) {
      switch (ev.event) {
        case "token": {
          const d = JSON.parse(ev.data) as StreamTokenData
          handlers.onToken(d.text)
          break
        }
        case "done": {
          const d = JSON.parse(ev.data) as StreamDoneData
          handlers.onDone(d)
          break
        }
        case "error": {
          const d = JSON.parse(ev.data) as StreamErrorData
          handlers.onError?.(d.error)
          break
        }
      }
    },
  })
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb/web && pnpm vitest run src/lib/sse.test.ts
```
Expected: both `streamAsk` tests PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb && git add web/src/lib/sse.ts web/src/lib/sse.test.ts && git commit -m "feat(web): streamAsk SSE helper (POST+Bearer) over fetch-event-source

Parses the M5a token/done/error wire frames. fetchEventSource is
injectable so the parser is unit-tested without the network; default is
the real client. POST + Authorization header (native EventSource cannot
do either)."
```

---

### Task 6: Auth context (login/logout, token lifecycle) (TDD)

**Files:**
- Create: `web/src/app/auth.tsx`
- Create: `web/src/app/auth.test.tsx`

Design: `AuthProvider` exposes `{ isAuthenticated, login, logout }`. `login(email, password)` POSTs `/api/auth/login` with the PascalCase `{Email,Password}` body, stores the returned `access_token` via `setAccessToken`, and flips `isAuthenticated`. `logout` POSTs `/api/auth/logout` (with `X-CSRF: 1`) and clears the token. Token lives in `apiClient` (in memory); the context tracks a boolean mirror for rendering.

- [ ] **Step 1: Write the failing test**

Create `web/src/app/auth.test.tsx`:
```tsx
import { describe, it, expect, beforeEach, vi } from "vitest"
import { renderHook, act, waitFor } from "@testing-library/react"
import { AuthProvider, useAuth } from "./auth"
import { getAccessToken, setAccessToken } from "@/lib/apiClient"
import { installFetchRoutes, jsonResponse } from "@/test/helpers"

const wrapper = ({ children }: { children: React.ReactNode }) => <AuthProvider>{children}</AuthProvider>

describe("AuthProvider", () => {
  beforeEach(() => {
    setAccessToken(null)
    vi.restoreAllMocks()
  })

  it("starts unauthenticated", () => {
    installFetchRoutes({})
    const { result } = renderHook(() => useAuth(), { wrapper })
    expect(result.current.isAuthenticated).toBe(false)
  })

  it("login stores the access token and flips isAuthenticated", async () => {
    installFetchRoutes({
      "/api/auth/login": jsonResponse({ access_token: "tok-9", expires_in: 900 }),
    })
    const { result } = renderHook(() => useAuth(), { wrapper })
    await act(async () => {
      await result.current.login("a@x.com", "pw")
    })
    await waitFor(() => expect(result.current.isAuthenticated).toBe(true))
    expect(getAccessToken()).toBe("tok-9")
  })

  it("login throws on bad credentials and stays unauthenticated", async () => {
    installFetchRoutes({
      "/api/auth/login": new Response("invalid credentials", { status: 401 }),
    })
    const { result } = renderHook(() => useAuth(), { wrapper })
    await expect(
      act(async () => {
        await result.current.login("a@x.com", "bad")
      }),
    ).rejects.toThrow()
    expect(result.current.isAuthenticated).toBe(false)
  })

  it("logout clears the token", async () => {
    installFetchRoutes({
      "/api/auth/login": jsonResponse({ access_token: "tok-9", expires_in: 900 }),
      "/api/auth/logout": new Response(null, { status: 204 }),
    })
    const { result } = renderHook(() => useAuth(), { wrapper })
    await act(async () => {
      await result.current.login("a@x.com", "pw")
    })
    await act(async () => {
      await result.current.logout()
    })
    expect(getAccessToken()).toBeNull()
    expect(result.current.isAuthenticated).toBe(false)
  })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb/web && pnpm vitest run src/app/auth.test.tsx
```
Expected: FAIL — `Failed to resolve import "./auth"`.

- [ ] **Step 3: Implement the auth context**

Create `web/src/app/auth.tsx`:
```tsx
import { createContext, useContext, useState, useCallback, type ReactNode } from "react"
import { apiFetch, setAccessToken } from "@/lib/apiClient"
import type { LoginResponse } from "@/lib/types"

interface AuthValue {
  isAuthenticated: boolean
  login: (email: string, password: string) => Promise<void>
  logout: () => Promise<void>
}

const AuthContext = createContext<AuthValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [isAuthenticated, setIsAuthenticated] = useState(false)

  // login uses the PascalCase {Email,Password} body the Go authz handler decodes.
  const login = useCallback(async (email: string, password: string) => {
    const res = await fetch("/api/auth/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify({ Email: email, Password: password }),
    })
    if (!res.ok) throw new Error("invalid credentials")
    const body = (await res.json()) as LoginResponse
    setAccessToken(body.access_token)
    setIsAuthenticated(true)
  }, [])

  const logout = useCallback(async () => {
    try {
      await apiFetch("/api/auth/logout", {
        method: "POST",
        headers: new Headers({ "X-CSRF": "1" }),
      })
    } finally {
      setAccessToken(null)
      setIsAuthenticated(false)
    }
  }, [])

  return <AuthContext.Provider value={{ isAuthenticated, login, logout }}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthValue {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error("useAuth must be used within AuthProvider")
  return ctx
}
```

- [ ] **Step 4: Wire AuthProvider into main.tsx**

In `web/src/main.tsx`, import the provider and wrap the router. Change the import block to add:
```tsx
import { AuthProvider } from "./app/auth"
```
And change the render tree so `AuthProvider` wraps `QueryClientProvider`:
```tsx
createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <AuthProvider>
      <QueryClientProvider client={queryClient}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    </AuthProvider>
  </StrictMode>,
)
```

- [ ] **Step 5: Run the test to verify it passes**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb/web && pnpm vitest run src/app/auth.test.tsx
```
Expected: all 4 `AuthProvider` tests PASS.

- [ ] **Step 6: Commit**

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb && git add web/src/app/auth.tsx web/src/app/auth.test.tsx web/src/main.tsx && git commit -m "feat(web): AuthProvider — in-memory token lifecycle

login POSTs the PascalCase {Email,Password} body; logout sends X-CSRF.
isAuthenticated mirrors token presence for routing. Wired above the
query + router providers."
```

---

### Task 7: Login route + form (TDD)

**Files:**
- Create: `web/src/routes/login.tsx`

Design: `/login` renders a react-hook-form + zod form (email/password). On submit it calls `useAuth().login`, then navigates to `/`. On failure it shows an inline error. The route component lives in the route file (small page; no separate feature dir needed for one form).

- [ ] **Step 1: Write the failing test**

Create `web/src/routes/login.test.tsx`:
```tsx
import { describe, it, expect, beforeEach, vi } from "vitest"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { AuthProvider } from "@/app/auth"
import { LoginForm } from "./login"
import { installFetchRoutes, jsonResponse } from "@/test/helpers"
import { setAccessToken } from "@/lib/apiClient"

const navigate = vi.fn()

function renderForm() {
  return render(
    <AuthProvider>
      <LoginForm onSuccess={navigate} />
    </AuthProvider>,
  )
}

describe("LoginForm", () => {
  beforeEach(() => {
    setAccessToken(null)
    navigate.mockReset()
    vi.restoreAllMocks()
  })

  it("logs in and calls onSuccess", async () => {
    installFetchRoutes({ "/api/auth/login": jsonResponse({ access_token: "t", expires_in: 900 }) })
    renderForm()
    await userEvent.type(screen.getByLabelText(/email/i), "a@x.com")
    await userEvent.type(screen.getByLabelText(/password/i), "pw")
    await userEvent.click(screen.getByRole("button", { name: /sign in/i }))
    await waitFor(() => expect(navigate).toHaveBeenCalled())
  })

  it("shows an error on bad credentials", async () => {
    installFetchRoutes({ "/api/auth/login": new Response("nope", { status: 401 }) })
    renderForm()
    await userEvent.type(screen.getByLabelText(/email/i), "a@x.com")
    await userEvent.type(screen.getByLabelText(/password/i), "bad")
    await userEvent.click(screen.getByRole("button", { name: /sign in/i }))
    expect(await screen.findByText(/invalid credentials/i)).toBeInTheDocument()
    expect(navigate).not.toHaveBeenCalled()
  })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb/web && pnpm vitest run src/routes/login.test.tsx
```
Expected: FAIL — `LoginForm` is not exported / module not found.

- [ ] **Step 3: Implement the login route + form**

Create `web/src/routes/login.tsx`:
```tsx
import { useState } from "react"
import { createFileRoute, useNavigate } from "@tanstack/react-router"
import { useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { useAuth } from "@/app/auth"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Card } from "@/components/ui/card"

const schema = z.object({
  email: z.string().email(),
  password: z.string().min(1),
})
type FormValues = z.infer<typeof schema>

// LoginForm is exported (sans router) so it can be unit-tested directly.
export function LoginForm({ onSuccess }: { onSuccess: () => void }) {
  const { login } = useAuth()
  const [serverError, setServerError] = useState<string | null>(null)
  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>({ resolver: zodResolver(schema) })

  const onSubmit = handleSubmit(async (values) => {
    setServerError(null)
    try {
      await login(values.email, values.password)
      onSuccess()
    } catch {
      setServerError("Invalid credentials")
    }
  })

  return (
    <div className="flex min-h-screen items-center justify-center p-4">
      <Card className="w-full max-w-sm p-6">
        <h1 className="mb-4 text-lg font-semibold">Sign in</h1>
        <form onSubmit={onSubmit} className="space-y-4">
          <div className="space-y-1">
            <Label htmlFor="email">Email</Label>
            <Input id="email" type="email" autoComplete="username" {...register("email")} />
            {errors.email && <p className="text-sm text-destructive">Enter a valid email</p>}
          </div>
          <div className="space-y-1">
            <Label htmlFor="password">Password</Label>
            <Input id="password" type="password" autoComplete="current-password" {...register("password")} />
            {errors.password && <p className="text-sm text-destructive">Password required</p>}
          </div>
          {serverError && <p className="text-sm text-destructive">{serverError}</p>}
          <Button type="submit" className="w-full" disabled={isSubmitting}>
            {isSubmitting ? "Signing in…" : "Sign in"}
          </Button>
        </form>
      </Card>
    </div>
  )
}

function LoginRoute() {
  const navigate = useNavigate()
  return <LoginForm onSuccess={() => navigate({ to: "/" })} />
}

export const Route = createFileRoute("/login")({ component: LoginRoute })
```

- [ ] **Step 4: Run the test to verify it passes**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb/web && pnpm vitest run src/routes/login.test.tsx
```
Expected: both `LoginForm` tests PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb && git add web/src/routes/login.tsx web/src/routes/login.test.tsx && git commit -m "feat(web): /login route — react-hook-form + zod, calls useAuth().login

LoginForm exported sans router for unit testing; success navigates to /,
bad creds show an inline error."
```

---

### Task 8: Protected layout route (redirect unauthenticated → /login) + app shell

**Files:**
- Create: `web/src/app/AppShell.tsx`
- Create: `web/src/routes/_authed.tsx`
- Modify: `web/src/routes/index.tsx` → move to `web/src/routes/_authed/index.tsx`

Design: TanStack file-based `_authed` is a pathless layout route. Its `beforeLoad` checks `isAuthenticated` (read from a router context value injected at `createRouter`) and `throw redirect({ to: "/login" })` when false. Children render inside `AppShell` (nav bar). We pass the auth state into the router via `context`.

- [ ] **Step 1: Add an auth-aware router context**

In `web/src/main.tsx`, change `createRouter` to accept a context and re-render on auth change. Replace the router creation + render with an `InnerApp` component that reads `useAuth()` and feeds it to the router:
```tsx
import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { RouterProvider, createRouter } from "@tanstack/react-router"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { routeTree } from "./routeTree.gen"
import { AuthProvider, useAuth } from "./app/auth"
import "./index.css"

const queryClient = new QueryClient()

const router = createRouter({
  routeTree,
  context: { isAuthenticated: false },
})
declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router
  }
}

function InnerApp() {
  const auth = useAuth()
  return <RouterProvider router={router} context={{ isAuthenticated: auth.isAuthenticated }} />
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <AuthProvider>
      <QueryClientProvider client={queryClient}>
        <InnerApp />
      </QueryClientProvider>
    </AuthProvider>
  </StrictMode>,
)
```
And update `web/src/routes/__root.tsx` to declare the context type:
```tsx
import { createRootRouteWithContext, Outlet } from "@tanstack/react-router"

interface RouterContext {
  isAuthenticated: boolean
}

export const Route = createRootRouteWithContext<RouterContext>()({
  component: () => <Outlet />,
})
```

- [ ] **Step 2: Build the app shell**

Create `web/src/app/AppShell.tsx`:
```tsx
import { Link, Outlet } from "@tanstack/react-router"
import { useAuth } from "./auth"
import { Button } from "@/components/ui/button"

// AppShell is the nav frame for authenticated routes. kbId-scoped links are
// rendered by the per-kb pages; the shell carries the top-level nav + logout.
export function AppShell() {
  const { logout } = useAuth()
  return (
    <div className="min-h-screen">
      <header className="flex items-center justify-between border-b px-6 py-3">
        <nav className="flex items-center gap-4">
          <Link to="/" className="font-semibold">
            llm-agent-kb
          </Link>
        </nav>
        <Button variant="outline" size="sm" onClick={() => void logout()}>
          Sign out
        </Button>
      </header>
      <main className="p-6">
        <Outlet />
      </main>
    </div>
  )
}
```

- [ ] **Step 3: Create the protected layout route**

Create `web/src/routes/_authed.tsx`:
```tsx
import { createFileRoute, redirect } from "@tanstack/react-router"
import { AppShell } from "@/app/AppShell"

export const Route = createFileRoute("/_authed")({
  beforeLoad: ({ context }) => {
    if (!context.isAuthenticated) {
      throw redirect({ to: "/login" })
    }
  },
  component: AppShell,
})
```

- [ ] **Step 4: Move the index route under the protected layout**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb/web && mkdir -p src/routes/_authed && git rm --quiet src/routes/index.tsx 2>/dev/null; rm -f src/routes/index.tsx
```
Create `web/src/routes/_authed/index.tsx` (placeholder until Task 9 fills it):
```tsx
import { createFileRoute } from "@tanstack/react-router"

export const Route = createFileRoute("/_authed/")({
  component: () => <div>Knowledge bases</div>,
})
```

- [ ] **Step 5: Verify the build regenerates the tree and compiles**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb/web && pnpm build
```
Expected: `tsr generate` rewrites `routeTree.gen.ts` to include `/login`, `/_authed`, `/_authed/`; `tsc -b` silent; `✓ built`.

- [ ] **Step 6: Verify the existing test suite is still green**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb/web && pnpm test
```
Expected: all prior test files still PASS (login/auth/apiClient/sse/utils/button).

- [ ] **Step 7: Commit**

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb && git add web/ && git commit -m "feat(web): protected _authed layout + app shell

Router context carries isAuthenticated; _authed.beforeLoad redirects to
/login when false. AppShell provides nav + sign-out. Index route moved
under _authed."
```

---

### Task 9: Org/KB list + create flow (TDD)

**Files:**
- Create: `web/src/features/orgkb/api.ts`
- Create: `web/src/features/orgkb/KbListPage.tsx`
- Create: `web/src/features/orgkb/KbListPage.test.tsx`
- Modify: `web/src/routes/_authed/index.tsx`

Design: the home page needs an org to list kbs (the backend lists kbs per org). The flow: a "Create organization" action (POST /api/orgs) sets the current org, then a kb table for that org with a "Create KB" dialog. We keep the selected org id in component state (v1 — no org switcher persistence). Queries via TanStack Query.

- [ ] **Step 1: Write the api module**

Create `web/src/features/orgkb/api.ts`:
```ts
import { apiJSON } from "@/lib/apiClient"
import type { Kb, ListEnvelope, Org } from "@/lib/types"

export function createOrg(name: string): Promise<Org> {
  return apiJSON<Org>("/api/orgs", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name }),
  })
}

export function listKbs(orgId: string): Promise<ListEnvelope<Kb>> {
  return apiJSON<ListEnvelope<Kb>>(`/api/orgs/${orgId}/kbs`)
}

export function createKb(orgId: string, name: string): Promise<Kb> {
  return apiJSON<Kb>(`/api/orgs/${orgId}/kbs`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name }),
  })
}
```

- [ ] **Step 2: Write the failing page test**

Create `web/src/features/orgkb/KbListPage.test.tsx`:
```tsx
import { describe, it, expect, beforeEach, vi } from "vitest"
import { render, screen, waitFor } from "@testing-library/react"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { KbListPage } from "./KbListPage"
import { installFetchRoutes, jsonResponse } from "@/test/helpers"
import { setAccessToken } from "@/lib/apiClient"

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <KbListPage initialOrgId="org-1" />
    </QueryClientProvider>,
  )
}

describe("KbListPage", () => {
  beforeEach(() => {
    setAccessToken("tok")
    vi.restoreAllMocks()
  })

  it("lists the kbs for the selected org", async () => {
    installFetchRoutes({
      "/api/orgs/org-1/kbs": jsonResponse({
        items: [
          { id: "kb-1", orgId: "org-1", name: "Handbook", namespace: "kb_kb-1" },
          { id: "kb-2", orgId: "org-1", name: "Runbooks", namespace: "kb_kb-2" },
        ],
        next_cursor: "",
      }),
    })
    renderPage()
    expect(await screen.findByText("Handbook")).toBeInTheDocument()
    expect(screen.getByText("Runbooks")).toBeInTheDocument()
  })

  it("shows an empty state when the org has no kbs", async () => {
    installFetchRoutes({
      "/api/orgs/org-1/kbs": jsonResponse({ items: [], next_cursor: "" }),
    })
    renderPage()
    await waitFor(() => expect(screen.getByText(/no knowledge bases/i)).toBeInTheDocument())
  })
})
```

- [ ] **Step 3: Run the test to verify it fails**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb/web && pnpm vitest run src/features/orgkb/KbListPage.test.tsx
```
Expected: FAIL — `KbListPage` not found.

- [ ] **Step 4: Implement the page**

Create `web/src/features/orgkb/KbListPage.tsx`:
```tsx
import { useState } from "react"
import { Link } from "@tanstack/react-router"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { createOrg, createKb, listKbs } from "./api"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
  DialogFooter,
} from "@/components/ui/dialog"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"

// KbListPage takes an optional initialOrgId so tests can render the kb list
// without going through org creation. In the app, the route passes undefined
// and the user creates/selects an org first.
export function KbListPage({ initialOrgId }: { initialOrgId?: string }) {
  const qc = useQueryClient()
  const [orgId, setOrgId] = useState<string | undefined>(initialOrgId)
  const [orgName, setOrgName] = useState("")
  const [kbName, setKbName] = useState("")

  const orgMut = useMutation({
    mutationFn: () => createOrg(orgName),
    onSuccess: (org) => {
      setOrgId(org.id)
      setOrgName("")
    },
  })

  const kbsQuery = useQuery({
    queryKey: ["kbs", orgId],
    queryFn: () => listKbs(orgId!),
    enabled: !!orgId,
  })

  const kbMut = useMutation({
    mutationFn: () => createKb(orgId!, kbName),
    onSuccess: () => {
      setKbName("")
      void qc.invalidateQueries({ queryKey: ["kbs", orgId] })
    },
  })

  if (!orgId) {
    return (
      <div className="max-w-sm space-y-3">
        <h1 className="text-lg font-semibold">Create an organization</h1>
        <Input placeholder="Organization name" value={orgName} onChange={(e) => setOrgName(e.target.value)} />
        <Button disabled={!orgName || orgMut.isPending} onClick={() => orgMut.mutate()}>
          {orgMut.isPending ? "Creating…" : "Create organization"}
        </Button>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-lg font-semibold">Knowledge bases</h1>
        <Dialog>
          <DialogTrigger asChild>
            <Button>New KB</Button>
          </DialogTrigger>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Create knowledge base</DialogTitle>
            </DialogHeader>
            <Input placeholder="KB name" value={kbName} onChange={(e) => setKbName(e.target.value)} />
            <DialogFooter>
              <Button disabled={!kbName || kbMut.isPending} onClick={() => kbMut.mutate()}>
                {kbMut.isPending ? "Creating…" : "Create"}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>

      {kbsQuery.isLoading && <p className="text-sm text-muted-foreground">Loading…</p>}
      {kbsQuery.data && kbsQuery.data.items.length === 0 && (
        <p className="text-sm text-muted-foreground">No knowledge bases yet.</p>
      )}
      {kbsQuery.data && kbsQuery.data.items.length > 0 && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Namespace</TableHead>
              <TableHead className="text-right">Open</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {kbsQuery.data.items.map((kb) => (
              <TableRow key={kb.id}>
                <TableCell>{kb.name}</TableCell>
                <TableCell className="font-mono text-xs">{kb.namespace}</TableCell>
                <TableCell className="text-right">
                  <Link to="/kb/$kbId/documents" params={{ kbId: kb.id }} className="text-sm underline">
                    Documents
                  </Link>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </div>
  )
}
```

- [ ] **Step 5: Wire the route**

Replace `web/src/routes/_authed/index.tsx`:
```tsx
import { createFileRoute } from "@tanstack/react-router"
import { KbListPage } from "@/features/orgkb/KbListPage"

export const Route = createFileRoute("/_authed/")({
  component: () => <KbListPage />,
})
```

- [ ] **Step 6: Run the test to verify it passes**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb/web && pnpm vitest run src/features/orgkb/KbListPage.test.tsx
```
Expected: both tests PASS. (Note: the `/kb/$kbId/documents` Link is type-checked against the route tree at build, not in this RTL test which renders the page standalone — the `Link` still renders an anchor.)

- [ ] **Step 7: Commit**

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb && git add web/src/features/orgkb/ web/src/routes/_authed/index.tsx && git commit -m "feat(web): org/kb list + create flow

Create-org → list kbs for the org → create-kb dialog. TanStack Query
for the list with invalidation on create. List + empty-state tested."
```

---

### Task 10: Document upload form (paste/url/file) (TDD)

**Files:**
- Create: `web/src/features/documents/api.ts`
- Create: `web/src/features/documents/UploadForm.tsx`
- Create: `web/src/features/documents/UploadForm.test.tsx`

Design: `UploadForm` has a source-type selector (paste/url) plus an optional file input. Paste sends `{title, sourceType:"paste", content}`; url sends `{title, sourceType:"url", url}`; file reads the file as text into `content` with `sourceType` derived from extension and `filename` set. On submit → POST `/api/kb/{id}/documents` (202) → invalidate the documents query. (File handling is supported in the form shape; M5b primarily exercises paste/url in tests since file bytes need backend parsing.)

- [ ] **Step 1: Write the api module**

Create `web/src/features/documents/api.ts`:
```ts
import { apiFetch, apiJSON } from "@/lib/apiClient"
import type { DocumentView, ListEnvelope } from "@/lib/types"

export interface UploadInput {
  title: string
  sourceType: "paste" | "url" | "pdf" | "docx"
  content?: string
  url?: string
  filename?: string
}

export interface UploadAccepted {
  documentId: string
  status: string
}

export function uploadDocument(kbId: string, input: UploadInput): Promise<UploadAccepted> {
  return apiJSON<UploadAccepted>(`/api/kb/${kbId}/documents`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  })
}

export function listDocuments(kbId: string): Promise<ListEnvelope<DocumentView>> {
  return apiJSON<ListEnvelope<DocumentView>>(`/api/kb/${kbId}/documents`)
}

export function retryDocument(kbId: string, docId: string): Promise<UploadAccepted> {
  return apiJSON<UploadAccepted>(`/api/kb/${kbId}/documents/${docId}/retry`, { method: "POST" })
}

export async function deleteDocument(kbId: string, docId: string): Promise<void> {
  const res = await apiFetch(`/api/kb/${kbId}/documents/${docId}`, { method: "DELETE" })
  if (!res.ok && res.status !== 204) throw new Error(`delete failed: ${res.status}`)
}
```

- [ ] **Step 2: Write the failing test**

Create `web/src/features/documents/UploadForm.test.tsx`:
```tsx
import { describe, it, expect, beforeEach, vi } from "vitest"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { UploadForm } from "./UploadForm"
import { installFetchRoutes, jsonResponse } from "@/test/helpers"
import { setAccessToken } from "@/lib/apiClient"

function renderForm(onUploaded = vi.fn()) {
  const qc = new QueryClient()
  render(
    <QueryClientProvider client={qc}>
      <UploadForm kbId="kb-1" onUploaded={onUploaded} />
    </QueryClientProvider>,
  )
  return onUploaded
}

describe("UploadForm", () => {
  beforeEach(() => {
    setAccessToken("tok")
    vi.restoreAllMocks()
  })

  it("submits a paste document and calls onUploaded", async () => {
    const calls: { body: unknown }[] = []
    installFetchRoutes({
      "/api/kb/kb-1/documents": (_p, init) => {
        calls.push({ body: JSON.parse(String(init?.body)) })
        return jsonResponse({ documentId: "doc-9", status: "pending" }, 202)
      },
    })
    const onUploaded = renderForm()
    await userEvent.type(screen.getByLabelText(/title/i), "My Note")
    await userEvent.type(screen.getByLabelText(/content/i), "hello body")
    await userEvent.click(screen.getByRole("button", { name: /upload/i }))
    await waitFor(() => expect(onUploaded).toHaveBeenCalled())
    expect(calls[0].body).toMatchObject({ title: "My Note", sourceType: "paste", content: "hello body" })
  })
})
```

- [ ] **Step 3: Run the test to verify it fails**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb/web && pnpm vitest run src/features/documents/UploadForm.test.tsx
```
Expected: FAIL — `UploadForm` not found.

- [ ] **Step 4: Implement the upload form**

Create `web/src/features/documents/UploadForm.tsx`:
```tsx
import { useState } from "react"
import { useMutation } from "@tanstack/react-query"
import { uploadDocument, type UploadInput } from "./api"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs"

// UploadForm posts a document (paste text or URL) and notifies onUploaded so
// the parent can refetch the list. Paste/url cover the M5b-tested paths; the
// URL tab maps to sourceType:"url".
export function UploadForm({ kbId, onUploaded }: { kbId: string; onUploaded: () => void }) {
  const [title, setTitle] = useState("")
  const [content, setContent] = useState("")
  const [url, setUrl] = useState("")
  const [mode, setMode] = useState<"paste" | "url">("paste")

  const mut = useMutation({
    mutationFn: () => {
      const input: UploadInput =
        mode === "paste"
          ? { title, sourceType: "paste", content }
          : { title, sourceType: "url", url }
      return uploadDocument(kbId, input)
    },
    onSuccess: () => {
      setTitle("")
      setContent("")
      setUrl("")
      onUploaded()
    },
  })

  const canSubmit = title.length > 0 && (mode === "paste" ? content.length > 0 : url.length > 0)

  return (
    <div className="space-y-3 rounded-lg border p-4">
      <div className="space-y-1">
        <Label htmlFor="doc-title">Title</Label>
        <Input id="doc-title" value={title} onChange={(e) => setTitle(e.target.value)} />
      </div>
      <Tabs value={mode} onValueChange={(v) => setMode(v as "paste" | "url")}>
        <TabsList>
          <TabsTrigger value="paste">Paste text</TabsTrigger>
          <TabsTrigger value="url">URL</TabsTrigger>
        </TabsList>
        <TabsContent value="paste" className="space-y-1">
          <Label htmlFor="doc-content">Content</Label>
          <Textarea id="doc-content" rows={6} value={content} onChange={(e) => setContent(e.target.value)} />
        </TabsContent>
        <TabsContent value="url" className="space-y-1">
          <Label htmlFor="doc-url">URL</Label>
          <Input id="doc-url" type="url" value={url} onChange={(e) => setUrl(e.target.value)} />
        </TabsContent>
      </Tabs>
      {mut.isError && <p className="text-sm text-destructive">{(mut.error as Error).message}</p>}
      <Button disabled={!canSubmit || mut.isPending} onClick={() => mut.mutate()}>
        {mut.isPending ? "Uploading…" : "Upload"}
      </Button>
    </div>
  )
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb/web && pnpm vitest run src/features/documents/UploadForm.test.tsx
```
Expected: the paste-upload test PASSES.

- [ ] **Step 6: Commit**

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb && git add web/src/features/documents/api.ts web/src/features/documents/UploadForm.tsx web/src/features/documents/UploadForm.test.tsx && git commit -m "feat(web): document upload form (paste/url) + documents api

POST /api/kb/{id}/documents (202). Tabs select paste vs url source;
paste path tested (body shape asserted)."
```

---

### Task 11: Document list with status, live SSE progress, retry/delete + the documents route (TDD)

**Files:**
- Create: `web/src/features/documents/DocStatusTable.tsx`
- Create: `web/src/features/documents/DocStatusTable.test.tsx`
- Create: `web/src/routes/_authed/kb.$kbId/documents.tsx`

Design: `DocStatusTable` lists documents with a status `Badge` and Retry/Delete actions. Non-terminal docs (status not ready/failed) get a live progress subscription via the document `progress` SSE endpoint — but to keep the table test deterministic and network-free, the SSE subscription is encapsulated in an injectable `subscribeProgress` prop (default uses `streamAsk`-style fetchEventSource; the test omits it / passes a no-op). The list itself comes from TanStack Query.

- [ ] **Step 1: Write the failing test**

Create `web/src/features/documents/DocStatusTable.test.tsx`:
```tsx
import { describe, it, expect, beforeEach, vi } from "vitest"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { DocStatusTable } from "./DocStatusTable"
import { installFetchRoutes, jsonResponse } from "@/test/helpers"
import { setAccessToken } from "@/lib/apiClient"

function renderTable() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={qc}>
      <DocStatusTable kbId="kb-1" subscribeProgress={() => () => {}} />
    </QueryClientProvider>,
  )
}

describe("DocStatusTable", () => {
  beforeEach(() => {
    setAccessToken("tok")
    vi.restoreAllMocks()
  })

  it("renders documents with their status", async () => {
    installFetchRoutes({
      "/api/kb/kb-1/documents": jsonResponse({
        items: [
          { id: "d1", title: "Ready Doc", sourceType: "paste", status: "ready", phase: "done", chunkCount: 4 },
          { id: "d2", title: "Failed Doc", sourceType: "url", status: "failed", phase: "parse", chunkCount: 0, error: "boom" },
        ],
        next_cursor: "",
      }),
    })
    renderTable()
    expect(await screen.findByText("Ready Doc")).toBeInTheDocument()
    expect(screen.getByText("ready")).toBeInTheDocument()
    expect(screen.getByText("failed")).toBeInTheDocument()
  })

  it("shows a Retry button for failed docs and posts retry", async () => {
    let retried = false
    installFetchRoutes({
      "/api/kb/kb-1/documents/d2/retry": () => {
        retried = true
        return jsonResponse({ documentId: "d2", status: "pending" }, 202)
      },
      "/api/kb/kb-1/documents": jsonResponse({
        items: [{ id: "d2", title: "Failed Doc", sourceType: "url", status: "failed", phase: "parse", chunkCount: 0, error: "boom" }],
        next_cursor: "",
      }),
    })
    renderTable()
    await screen.findByText("Failed Doc")
    await userEvent.click(screen.getByRole("button", { name: /retry/i }))
    await waitFor(() => expect(retried).toBe(true))
  })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb/web && pnpm vitest run src/features/documents/DocStatusTable.test.tsx
```
Expected: FAIL — `DocStatusTable` not found.

- [ ] **Step 3: Implement the table + the progress subscriber**

Create `web/src/features/documents/DocStatusTable.tsx`:
```tsx
import { useEffect } from "react"
import { fetchEventSource } from "@microsoft/fetch-event-source"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { listDocuments, retryDocument, deleteDocument } from "./api"
import { getAccessToken } from "@/lib/apiClient"
import type { DocumentView } from "@/lib/types"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"

// SubscribeProgress opens the document progress SSE and calls onUpdate per
// frame; returns an unsubscribe. Injectable so tests pass a no-op.
export type SubscribeProgress = (
  kbId: string,
  docId: string,
  onUpdate: () => void,
) => () => void

const defaultSubscribe: SubscribeProgress = (kbId, docId, onUpdate) => {
  const ctrl = new AbortController()
  void fetchEventSource(`/api/kb/${kbId}/documents/${docId}/progress`, {
    method: "GET",
    headers: { Authorization: `Bearer ${getAccessToken() ?? ""}` },
    signal: ctrl.signal,
    openWhenHidden: true,
    onmessage() {
      onUpdate()
    },
  })
  return () => ctrl.abort()
}

function statusVariant(status: string): "default" | "secondary" | "destructive" {
  if (status === "ready") return "default"
  if (status === "failed") return "destructive"
  return "secondary"
}

export function DocStatusTable({
  kbId,
  subscribeProgress = defaultSubscribe,
}: {
  kbId: string
  subscribeProgress?: SubscribeProgress
}) {
  const qc = useQueryClient()
  const docsQuery = useQuery({
    queryKey: ["documents", kbId],
    queryFn: () => listDocuments(kbId),
  })

  const invalidate = () => void qc.invalidateQueries({ queryKey: ["documents", kbId] })

  const retryMut = useMutation({
    mutationFn: (docId: string) => retryDocument(kbId, docId),
    onSuccess: invalidate,
  })
  const deleteMut = useMutation({
    mutationFn: (docId: string) => deleteDocument(kbId, docId),
    onSuccess: invalidate,
  })

  // Subscribe to live progress for any non-terminal document; refetch on update.
  const items: DocumentView[] = docsQuery.data?.items ?? []
  useEffect(() => {
    const unsubs = items
      .filter((d) => d.status !== "ready" && d.status !== "failed")
      .map((d) => subscribeProgress(kbId, d.id, invalidate))
    return () => unsubs.forEach((u) => u())
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [kbId, items.map((d) => `${d.id}:${d.status}`).join(",")])

  if (docsQuery.isLoading) return <p className="text-sm text-muted-foreground">Loading…</p>
  if (items.length === 0) return <p className="text-sm text-muted-foreground">No documents yet.</p>

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Title</TableHead>
          <TableHead>Status</TableHead>
          <TableHead>Chunks</TableHead>
          <TableHead className="text-right">Actions</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {items.map((d) => (
          <TableRow key={d.id}>
            <TableCell>{d.title}</TableCell>
            <TableCell>
              <Badge variant={statusVariant(d.status)}>{d.status}</Badge>
            </TableCell>
            <TableCell>{d.chunkCount}</TableCell>
            <TableCell className="space-x-2 text-right">
              {d.status === "failed" && (
                <Button size="sm" variant="outline" onClick={() => retryMut.mutate(d.id)}>
                  Retry
                </Button>
              )}
              <Button size="sm" variant="ghost" onClick={() => deleteMut.mutate(d.id)}>
                Delete
              </Button>
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb/web && pnpm vitest run src/features/documents/DocStatusTable.test.tsx
```
Expected: both tests PASS.

- [ ] **Step 5: Create the documents route (upload + table)**

Create `web/src/routes/_authed/kb.$kbId/documents.tsx`:
```tsx
import { createFileRoute } from "@tanstack/react-router"
import { useQueryClient } from "@tanstack/react-query"
import { UploadForm } from "@/features/documents/UploadForm"
import { DocStatusTable } from "@/features/documents/DocStatusTable"
import { KbTabs } from "@/app/AppShell"

function DocumentsPage() {
  const { kbId } = Route.useParams()
  const qc = useQueryClient()
  return (
    <div className="space-y-6">
      <KbTabs kbId={kbId} />
      <UploadForm kbId={kbId} onUploaded={() => void qc.invalidateQueries({ queryKey: ["documents", kbId] })} />
      <DocStatusTable kbId={kbId} />
    </div>
  )
}

export const Route = createFileRoute("/_authed/kb/$kbId/documents")({ component: DocumentsPage })
```

- [ ] **Step 6: Add the KbTabs nav helper to AppShell**

In `web/src/app/AppShell.tsx`, append a `KbTabs` export (per-kb sub-nav reused by every kb route):
```tsx
export function KbTabs({ kbId }: { kbId: string }) {
  const linkCls = "rounded px-3 py-1.5 text-sm hover:bg-accent"
  return (
    <nav className="flex gap-1 border-b pb-2">
      <Link to="/kb/$kbId/documents" params={{ kbId }} className={linkCls}>
        Documents
      </Link>
      <Link to="/kb/$kbId/ask" params={{ kbId }} className={linkCls}>
        Ask
      </Link>
      <Link to="/kb/$kbId/eval" params={{ kbId }} className={linkCls}>
        Eval
      </Link>
      <Link to="/kb/$kbId/sessions" params={{ kbId }} className={linkCls}>
        Sessions
      </Link>
    </nav>
  )
}
```

- [ ] **Step 7: Build to regenerate the route tree + verify whole suite**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb/web && pnpm build && pnpm test
```
Expected: build `✓ built` (new `/kb/$kbId/documents` route compiled); all tests PASS.

- [ ] **Step 8: Commit**

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb && git add web/ && git commit -m "feat(web): documents view — list, status badge, live SSE progress, retry/delete

DocStatusTable subscribes to the per-doc progress SSE for non-terminal
docs (injectable for tests) and refetches on each frame. KbTabs sub-nav
added; documents route wires upload + table."
```

---

### Task 12: Ask non-stream api + streaming hook (TDD)

**Files:**
- Create: `web/src/features/ask/api.ts`
- Create: `web/src/features/ask/useAskStream.ts`
- Create: `web/src/features/ask/useAskStream.test.ts`

Design: `api.ts` has `askGlobal`/`askDrift` (non-stream — those have no stream endpoint) returning `AskResponse`. `useAskStream` is a hook driving the streaming endpoint via `lib/sse.streamAsk`: it exposes `{ answer, citations, diagnostics, isStreaming, run }`. `run(q, mode, topK)` resets state, calls `streamAsk`, appending tokens to `answer` and capturing the `done` payload. The hook accepts the `streamAsk` fn injected (default the real one) for testing.

- [ ] **Step 1: Write the non-stream api module**

Create `web/src/features/ask/api.ts`:
```ts
import { apiJSON } from "@/lib/apiClient"
import type { AskResponse } from "@/lib/types"

export function askGlobal(kbId: string, q: string, sessionId?: string): Promise<AskResponse> {
  return apiJSON<AskResponse>(`/api/kb/${kbId}/ask/global`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ q, sessionId }),
  })
}

export function askDrift(kbId: string, q: string, sessionId?: string): Promise<AskResponse> {
  return apiJSON<AskResponse>(`/api/kb/${kbId}/ask/drift`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ q, sessionId }),
  })
}
```

- [ ] **Step 2: Write the failing hook test**

Create `web/src/features/ask/useAskStream.test.ts`:
```ts
import { describe, it, expect, vi } from "vitest"
import { renderHook, act, waitFor } from "@testing-library/react"
import { useAskStream } from "./useAskStream"
import type { streamAsk as RealStreamAsk } from "@/lib/sse"
import { setAccessToken } from "@/lib/apiClient"

// fakeStreamAsk replays tokens then done through the handlers synchronously.
const fakeStreamAsk: typeof RealStreamAsk = async (_url, _body, _tok, handlers) => {
  handlers.onToken("hello ")
  handlers.onToken("world")
  handlers.onDone({
    citations: [{ chunkId: "c1", docId: "d1", title: "Doc", score: 0.9, snippet: "snip" }],
    diagnostics: { mode: "hybrid", hitCount: 1 },
    sessionId: "sid-1",
  })
}

describe("useAskStream", () => {
  it("accumulates tokens and captures the done payload", async () => {
    setAccessToken("tok")
    const { result } = renderHook(() => useAskStream("kb-1", fakeStreamAsk))
    await act(async () => {
      await result.current.run("fox", "hybrid", 5)
    })
    await waitFor(() => expect(result.current.answer).toBe("hello world"))
    expect(result.current.citations[0].chunkId).toBe("c1")
    expect(result.current.diagnostics?.mode).toBe("hybrid")
    expect(result.current.sessionId).toBe("sid-1")
    expect(result.current.isStreaming).toBe(false)
  })

  it("surfaces an error frame", async () => {
    setAccessToken("tok")
    const erroring: typeof RealStreamAsk = async (_u, _b, _t, h) => {
      h.onError?.("boom")
    }
    const { result } = renderHook(() => useAskStream("kb-1", erroring))
    await act(async () => {
      await result.current.run("x", "vector")
    })
    await waitFor(() => expect(result.current.error).toBe("boom"))
  })
})
```

- [ ] **Step 3: Run the test to verify it fails**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb/web && pnpm vitest run src/features/ask/useAskStream.test.ts
```
Expected: FAIL — `useAskStream` not found.

- [ ] **Step 4: Implement the hook**

Create `web/src/features/ask/useAskStream.ts`:
```ts
import { useCallback, useRef, useState } from "react"
import { streamAsk as realStreamAsk } from "@/lib/sse"
import { getAccessToken } from "@/lib/apiClient"
import type { Citation, StreamDoneData } from "@/lib/types"

interface AskStreamState {
  answer: string
  citations: Citation[]
  diagnostics: StreamDoneData["diagnostics"] | null
  sessionId: string | null
  isStreaming: boolean
  error: string | null
  run: (q: string, mode: "vector" | "hybrid", topK?: number) => Promise<void>
}

// useAskStream drives POST /ask/stream and accumulates the streamed answer.
// streamAsk is injectable (default = the real SSE helper) for unit testing.
export function useAskStream(
  kbId: string,
  streamAsk: typeof realStreamAsk = realStreamAsk,
): AskStreamState {
  const [answer, setAnswer] = useState("")
  const [citations, setCitations] = useState<Citation[]>([])
  const [diagnostics, setDiagnostics] = useState<StreamDoneData["diagnostics"] | null>(null)
  const [sessionId, setSessionId] = useState<string | null>(null)
  const [isStreaming, setIsStreaming] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const abortRef = useRef<AbortController | null>(null)

  const run = useCallback(
    async (q: string, mode: "vector" | "hybrid", topK?: number) => {
      abortRef.current?.abort()
      const ctrl = new AbortController()
      abortRef.current = ctrl
      setAnswer("")
      setCitations([])
      setDiagnostics(null)
      setSessionId(null)
      setError(null)
      setIsStreaming(true)
      try {
        await streamAsk(
          `/api/kb/${kbId}/ask/stream`,
          { q, mode, topK },
          getAccessToken() ?? "",
          {
            onToken: (t) => setAnswer((prev) => prev + t),
            onDone: (d) => {
              setCitations(d.citations)
              setDiagnostics(d.diagnostics)
              setSessionId(d.sessionId)
            },
            onError: (m) => setError(m),
          },
          undefined,
          ctrl.signal,
        )
      } finally {
        setIsStreaming(false)
      }
    },
    [kbId, streamAsk],
  )

  return { answer, citations, diagnostics, sessionId, isStreaming, error, run }
}
```
NOTE: the real `streamAsk` signature is `(url, body, token, handlers, client?, signal?)`. The injected fake in the test ignores the trailing `client`/`signal` args, which is valid.

- [ ] **Step 5: Run the test to verify it passes**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb/web && pnpm vitest run src/features/ask/useAskStream.test.ts
```
Expected: both tests PASS.

- [ ] **Step 6: Commit**

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb && git add web/src/features/ask/api.ts web/src/features/ask/useAskStream.ts web/src/features/ask/useAskStream.test.ts && git commit -m "feat(web): ask streaming hook + non-stream global/drift api

useAskStream accumulates token frames and captures the done payload
(citations/diagnostics/sessionId); streamAsk injectable for tests.
askGlobal/askDrift cover the non-streamed modes."
```

---

### Task 13: Ask page — ModeSwitch, AnswerPane, CitationList, DiagnosticsDrawer (TDD)

**Files:**
- Create: `web/src/features/ask/AnswerPane.tsx`
- Create: `web/src/features/ask/CitationList.tsx`
- Create: `web/src/features/ask/DiagnosticsDrawer.tsx`
- Create: `web/src/features/ask/AskPage.tsx`
- Create: `web/src/features/ask/AskPage.test.tsx`
- Create: `web/src/routes/_authed/kb.$kbId/ask.tsx`

Design: `AskPage` has a ModeSwitch (vector/hybrid/global/drift), a question input, and an answer area. For `vector`/`hybrid` it uses `useAskStream` (real streaming). For `global`/`drift` it calls the non-stream api and sets the answer once. `AnswerPane` shows the (streaming or final) answer; `CitationList` renders citations (title + sectionPath + snippet); `DiagnosticsDrawer` shows diagnostics JSON. The test injects a fake `streamAsk` to assert token-by-token rendering.

- [ ] **Step 1: Write the leaf components**

Create `web/src/features/ask/AnswerPane.tsx`:
```tsx
export function AnswerPane({ answer, isStreaming }: { answer: string; isStreaming: boolean }) {
  return (
    <div className="min-h-24 whitespace-pre-wrap rounded-lg border p-4" data-testid="answer-pane">
      {answer}
      {isStreaming && <span className="ml-0.5 animate-pulse">▋</span>}
    </div>
  )
}
```
Create `web/src/features/ask/CitationList.tsx`:
```tsx
import type { Citation } from "@/lib/types"

export function CitationList({ citations }: { citations: Citation[] }) {
  if (citations.length === 0) return null
  return (
    <ul className="space-y-2">
      {citations.map((c) => (
        <li key={c.chunkId} className="rounded-md border p-3 text-sm">
          <div className="font-medium">{c.title}</div>
          {c.sectionPath && c.sectionPath.length > 0 && (
            <div className="text-xs text-muted-foreground">{c.sectionPath.join(" › ")}</div>
          )}
          <p className="mt-1 text-muted-foreground">{c.snippet}</p>
          <div className="mt-1 text-xs">score {c.score.toFixed(3)}</div>
        </li>
      ))}
    </ul>
  )
}
```
Create `web/src/features/ask/DiagnosticsDrawer.tsx`:
```tsx
import { useState } from "react"
import { Button } from "@/components/ui/button"

export function DiagnosticsDrawer({ diagnostics }: { diagnostics: Record<string, unknown> | null }) {
  const [open, setOpen] = useState(false)
  if (!diagnostics) return null
  return (
    <div>
      <Button variant="ghost" size="sm" onClick={() => setOpen((o) => !o)}>
        {open ? "Hide" : "Show"} diagnostics
      </Button>
      {open && (
        <pre className="mt-2 overflow-x-auto rounded-md border bg-muted p-3 text-xs">
          {JSON.stringify(diagnostics, null, 2)}
        </pre>
      )}
    </div>
  )
}
```

- [ ] **Step 2: Write the failing AskPage test**

Create `web/src/features/ask/AskPage.test.tsx`:
```tsx
import { describe, it, expect, beforeEach, vi } from "vitest"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { AskPage } from "./AskPage"
import type { streamAsk as RealStreamAsk } from "@/lib/sse"
import { setAccessToken } from "@/lib/apiClient"

// streams two tokens then done — asserts the pane shows the concatenation.
const fakeStreamAsk: typeof RealStreamAsk = async (_u, _b, _t, h) => {
  h.onToken("The answer ")
  h.onToken("is 42.")
  h.onDone({
    citations: [{ chunkId: "c1", docId: "d1", title: "Guide", sectionPath: ["Intro"], score: 0.91, snippet: "the snippet" }],
    diagnostics: { mode: "hybrid", hitCount: 1 },
    sessionId: "sid-1",
  })
}

describe("AskPage", () => {
  beforeEach(() => {
    setAccessToken("tok")
    vi.restoreAllMocks()
  })

  it("streams the answer and renders citations for hybrid mode", async () => {
    render(<AskPage kbId="kb-1" streamAsk={fakeStreamAsk} />)
    await userEvent.type(screen.getByPlaceholderText(/ask/i), "what is the answer")
    await userEvent.click(screen.getByRole("button", { name: /^ask$/i }))
    await waitFor(() => expect(screen.getByTestId("answer-pane")).toHaveTextContent("The answer is 42."))
    expect(screen.getByText("Guide")).toBeInTheDocument()
    expect(screen.getByText(/Intro/)).toBeInTheDocument()
    expect(screen.getByText("the snippet")).toBeInTheDocument()
  })
})
```

- [ ] **Step 3: Run the test to verify it fails**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb/web && pnpm vitest run src/features/ask/AskPage.test.tsx
```
Expected: FAIL — `AskPage` not found.

- [ ] **Step 4: Implement AskPage**

Create `web/src/features/ask/AskPage.tsx`:
```tsx
import { useState } from "react"
import { useAskStream } from "./useAskStream"
import { askGlobal, askDrift } from "./api"
import { streamAsk as realStreamAsk } from "@/lib/sse"
import { AnswerPane } from "./AnswerPane"
import { CitationList } from "./CitationList"
import { DiagnosticsDrawer } from "./DiagnosticsDrawer"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import type { Citation } from "@/lib/types"

type Mode = "vector" | "hybrid" | "global" | "drift"

// AskPage drives streaming for vector/hybrid (token-by-token) and the
// non-stream endpoints for global/drift (no stream endpoint exists). streamAsk
// is injectable for tests.
export function AskPage({
  kbId,
  streamAsk = realStreamAsk,
}: {
  kbId: string
  streamAsk?: typeof realStreamAsk
}) {
  const [mode, setMode] = useState<Mode>("hybrid")
  const [q, setQ] = useState("")
  const stream = useAskStream(kbId, streamAsk)

  // Non-stream (global/drift) state.
  const [nsAnswer, setNsAnswer] = useState("")
  const [nsCitations, setNsCitations] = useState<Citation[]>([])
  const [nsDiagnostics, setNsDiagnostics] = useState<Record<string, unknown> | null>(null)
  const [nsLoading, setNsLoading] = useState(false)

  const streaming = mode === "vector" || mode === "hybrid"

  const onAsk = async () => {
    if (!q) return
    if (streaming) {
      await stream.run(q, mode, 5)
    } else {
      setNsLoading(true)
      setNsAnswer("")
      setNsCitations([])
      setNsDiagnostics(null)
      try {
        const res = mode === "global" ? await askGlobal(kbId, q) : await askDrift(kbId, q)
        setNsAnswer(res.answer)
        setNsCitations(res.citations)
        setNsDiagnostics(res.diagnostics)
      } finally {
        setNsLoading(false)
      }
    }
  }

  const answer = streaming ? stream.answer : nsAnswer
  const citations = streaming ? stream.citations : nsCitations
  const diagnostics = streaming ? stream.diagnostics : nsDiagnostics
  const busy = streaming ? stream.isStreaming : nsLoading

  return (
    <div className="space-y-4">
      <Tabs value={mode} onValueChange={(v) => setMode(v as Mode)}>
        <TabsList>
          <TabsTrigger value="vector">Vector</TabsTrigger>
          <TabsTrigger value="hybrid">Hybrid</TabsTrigger>
          <TabsTrigger value="global">Global</TabsTrigger>
          <TabsTrigger value="drift">Drift</TabsTrigger>
        </TabsList>
      </Tabs>
      <div className="flex gap-2">
        <Input
          placeholder="Ask a question…"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") void onAsk()
          }}
        />
        <Button onClick={() => void onAsk()} disabled={busy || !q}>
          {busy ? "Asking…" : "Ask"}
        </Button>
      </div>
      {stream.error && streaming && <p className="text-sm text-destructive">{stream.error}</p>}
      <AnswerPane answer={answer} isStreaming={streaming && stream.isStreaming} />
      <DiagnosticsDrawer diagnostics={diagnostics} />
      <CitationList citations={citations} />
    </div>
  )
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb/web && pnpm vitest run src/features/ask/AskPage.test.tsx
```
Expected: the streaming-render test PASSES (pane shows "The answer is 42.", citation Guide/Intro/the snippet visible).

- [ ] **Step 6: Wire the ask route**

Create `web/src/routes/_authed/kb.$kbId/ask.tsx`:
```tsx
import { createFileRoute } from "@tanstack/react-router"
import { AskPage } from "@/features/ask/AskPage"
import { KbTabs } from "@/app/AppShell"

function AskRoute() {
  const { kbId } = Route.useParams()
  return (
    <div className="space-y-6">
      <KbTabs kbId={kbId} />
      <AskPage kbId={kbId} />
    </div>
  )
}

export const Route = createFileRoute("/_authed/kb/$kbId/ask")({ component: AskRoute })
```

- [ ] **Step 7: Build + full suite**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb/web && pnpm build && pnpm test
```
Expected: build `✓ built` (ask route compiled); all tests PASS.

- [ ] **Step 8: Commit**

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb && git add web/src/features/ask/ web/src/routes/_authed/kb.\$kbId/ask.tsx && git commit -m "feat(web): ask view — streaming vector/hybrid + non-stream global/drift

ModeSwitch tabs; vector/hybrid stream token-by-token via useAskStream,
global/drift use the non-stream endpoints. AnswerPane + CitationList
(title/sectionPath/snippet) + DiagnosticsDrawer. Streaming render tested
with a mocked SSE."
```

---

### Task 14: Eval dashboard — MetricCards, DriftReportTable, RunEvalDialog (TDD)

**Files:**
- Create: `web/src/features/eval/api.ts`
- Create: `web/src/features/eval/MetricCards.tsx`
- Create: `web/src/features/eval/DriftReportTable.tsx`
- Create: `web/src/features/eval/RunEvalDialog.tsx`
- Create: `web/src/features/eval/EvalPage.tsx`
- Create: `web/src/features/eval/MetricCards.test.tsx`
- Create: `web/src/routes/_authed/kb.$kbId/eval.tsx`

Design: `MetricCards` renders retrieval metrics (precision/recall/MRR/grounding) and/or generation triad means from an `EvalResult`. `DriftReportTable` renders drift deltas with the Direction. `RunEvalDialog` posts an eval run (kind + JSONL textarea). `EvalPage` lists past runs and shows the latest result. Test asserts MetricCards renders from a fixture (no network).

- [ ] **Step 1: Write the api module**

Create `web/src/features/eval/api.ts`:
```ts
import { apiJSON } from "@/lib/apiClient"
import type { EvalResult, EvalRunRow, ListEnvelope } from "@/lib/types"

export interface RunEvalResponse {
  runId: string
  result: EvalResult
}

export function runEval(
  kbId: string,
  kind: "retrieval" | "triad" | "global" | "drift",
  dataset: string,
): Promise<RunEvalResponse> {
  return apiJSON<RunEvalResponse>(`/api/kb/${kbId}/eval/run`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ kind, dataset }),
  })
}

export function listRuns(kbId: string): Promise<ListEnvelope<EvalRunRow>> {
  return apiJSON<ListEnvelope<EvalRunRow>>(`/api/kb/${kbId}/eval/runs`)
}
```

- [ ] **Step 2: Write the MetricCards + DriftReportTable components**

Create `web/src/features/eval/MetricCards.tsx`:
```tsx
import type { EvalResult } from "@/lib/types"
import { Card } from "@/components/ui/card"

function Metric({ label, value }: { label: string; value: number }) {
  return (
    <Card className="p-4">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="text-2xl font-semibold tabular-nums">{value.toFixed(3)}</div>
    </Card>
  )
}

// MetricCards renders whichever metric leg the EvalResult carries: retrieval
// (precision/recall/MRR/grounding) and/or the generation triad means.
export function MetricCards({ result }: { result: EvalResult }) {
  return (
    <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
      {result.retrieval && (
        <>
          <Metric label="Precision@K" value={result.retrieval.precisionAtK} />
          <Metric label="Recall@K" value={result.retrieval.recallAtK} />
          <Metric label="MRR" value={result.retrieval.mrr} />
          <Metric label="Grounding@K" value={result.retrieval.groundingAtK} />
        </>
      )}
      {result.generation && (
        <>
          <Metric label="Mean Groundedness" value={result.generation.meanGroundedness} />
          <Metric label="Mean Answer Relevance" value={result.generation.meanAnswerRelevance} />
        </>
      )}
    </div>
  )
}
```
Create `web/src/features/eval/DriftReportTable.tsx`:
```tsx
import type { DriftView } from "@/lib/types"
import { Badge } from "@/components/ui/badge"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"

function directionVariant(d: string): "default" | "secondary" | "destructive" {
  if (d === "improved") return "default"
  if (d === "regressed") return "destructive"
  return "secondary"
}

function fmt(v: number | null): string {
  return v === null ? "—" : v.toFixed(3)
}

export function DriftReportTable({ drift }: { drift: DriftView }) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Metric</TableHead>
          <TableHead>Prev</TableHead>
          <TableHead>Curr</TableHead>
          <TableHead>Delta</TableHead>
          <TableHead>Direction</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {drift.deltas.map((d) => (
          <TableRow key={d.name}>
            <TableCell>{d.name}</TableCell>
            <TableCell className="tabular-nums">{fmt(d.prev)}</TableCell>
            <TableCell className="tabular-nums">{fmt(d.curr)}</TableCell>
            <TableCell className="tabular-nums">{fmt(d.delta)}</TableCell>
            <TableCell>
              <Badge variant={directionVariant(d.direction)}>{d.direction}</Badge>
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
```

- [ ] **Step 3: Write the failing MetricCards test**

Create `web/src/features/eval/MetricCards.test.tsx`:
```tsx
import { describe, it, expect } from "vitest"
import { render, screen } from "@testing-library/react"
import { MetricCards } from "./MetricCards"
import { DriftReportTable } from "./DriftReportTable"
import type { EvalResult, DriftView } from "@/lib/types"

describe("MetricCards", () => {
  it("renders retrieval metrics from a fixture", () => {
    const result: EvalResult = {
      kind: "retrieval",
      datasetName: "ds",
      retrieval: { precisionAtK: 0.8, recallAtK: 0.6, mrr: 0.75, groundingAtK: 0.9, examples: 10, topK: 5 },
    }
    render(<MetricCards result={result} />)
    expect(screen.getByText("Precision@K")).toBeInTheDocument()
    expect(screen.getByText("0.800")).toBeInTheDocument()
    expect(screen.getByText("0.750")).toBeInTheDocument() // MRR
  })

  it("renders triad generation means", () => {
    const result: EvalResult = {
      kind: "triad",
      datasetName: "ds",
      generation: { meanGroundedness: 0.85, meanAnswerRelevance: 0.92, examples: 5 },
    }
    render(<MetricCards result={result} />)
    expect(screen.getByText("Mean Groundedness")).toBeInTheDocument()
    expect(screen.getByText("0.850")).toBeInTheDocument()
  })
})

describe("DriftReportTable", () => {
  it("renders directions from a drift fixture", () => {
    const drift: DriftView = {
      dataset: "ds",
      deltas: [
        { name: "MRR", prev: 0.5, curr: 0.7, delta: 0.2, direction: "improved" },
        { name: "Precision", prev: 0.8, curr: 0.6, delta: -0.2, direction: "regressed" },
      ],
      histograms: [],
      newExamples: [],
      droppedExamples: [],
    }
    render(<DriftReportTable drift={drift} />)
    expect(screen.getByText("improved")).toBeInTheDocument()
    expect(screen.getByText("regressed")).toBeInTheDocument()
  })
})
```

- [ ] **Step 4: Run the test to verify it fails**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb/web && pnpm vitest run src/features/eval/MetricCards.test.tsx
```
Expected: FAIL — modules not found.

- [ ] **Step 5: Implement RunEvalDialog + EvalPage**

Create `web/src/features/eval/RunEvalDialog.tsx`:
```tsx
import { useState } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { runEval, type RunEvalResponse } from "./api"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
  DialogFooter,
} from "@/components/ui/dialog"

type Kind = "retrieval" | "triad" | "global" | "drift"

export function RunEvalDialog({ kbId, onComplete }: { kbId: string; onComplete: (r: RunEvalResponse) => void }) {
  const qc = useQueryClient()
  const [kind, setKind] = useState<Kind>("retrieval")
  const [dataset, setDataset] = useState("")

  const mut = useMutation({
    mutationFn: () => runEval(kbId, kind, dataset),
    onSuccess: (r) => {
      onComplete(r)
      void qc.invalidateQueries({ queryKey: ["eval-runs", kbId] })
    },
  })

  return (
    <Dialog>
      <DialogTrigger asChild>
        <Button>Run eval</Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Run evaluation</DialogTitle>
        </DialogHeader>
        <Tabs value={kind} onValueChange={(v) => setKind(v as Kind)}>
          <TabsList>
            <TabsTrigger value="retrieval">Retrieval</TabsTrigger>
            <TabsTrigger value="triad">Triad</TabsTrigger>
            <TabsTrigger value="global">Global</TabsTrigger>
            <TabsTrigger value="drift">Drift</TabsTrigger>
          </TabsList>
        </Tabs>
        <Textarea
          rows={8}
          placeholder='Dataset JSONL — one example per line, e.g. {"question":"...","answer":"..."}'
          value={dataset}
          onChange={(e) => setDataset(e.target.value)}
        />
        {mut.isError && <p className="text-sm text-destructive">{(mut.error as Error).message}</p>}
        <DialogFooter>
          <Button disabled={!dataset || mut.isPending} onClick={() => mut.mutate()}>
            {mut.isPending ? "Running…" : "Run"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
```
Create `web/src/features/eval/EvalPage.tsx`:
```tsx
import { useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { listRuns, type RunEvalResponse } from "./api"
import { MetricCards } from "./MetricCards"
import { DriftReportTable } from "./DriftReportTable"
import { RunEvalDialog } from "./RunEvalDialog"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"

export function EvalPage({ kbId }: { kbId: string }) {
  const [latest, setLatest] = useState<RunEvalResponse | null>(null)
  const runsQuery = useQuery({ queryKey: ["eval-runs", kbId], queryFn: () => listRuns(kbId) })

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-lg font-semibold">Evaluation</h1>
        <RunEvalDialog kbId={kbId} onComplete={setLatest} />
      </div>

      {latest && (
        <div className="space-y-4">
          <MetricCards result={latest.result} />
          {latest.result.drift && <DriftReportTable drift={latest.result.drift} />}
        </div>
      )}

      <div>
        <h2 className="mb-2 text-sm font-medium text-muted-foreground">Past runs</h2>
        {runsQuery.data && runsQuery.data.items.length === 0 && (
          <p className="text-sm text-muted-foreground">No runs yet.</p>
        )}
        {runsQuery.data && runsQuery.data.items.length > 0 && (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Kind</TableHead>
                <TableHead>Dataset</TableHead>
                <TableHead>Created</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {runsQuery.data.items.map((r) => (
                <TableRow key={r.id}>
                  <TableCell>{r.kind}</TableCell>
                  <TableCell>{r.datasetName}</TableCell>
                  <TableCell className="text-xs">{r.createdAt}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </div>
    </div>
  )
}
```

- [ ] **Step 6: Run the test to verify it passes**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb/web && pnpm vitest run src/features/eval/MetricCards.test.tsx
```
Expected: all 3 tests PASS.

- [ ] **Step 7: Wire the eval route**

Create `web/src/routes/_authed/kb.$kbId/eval.tsx`:
```tsx
import { createFileRoute } from "@tanstack/react-router"
import { EvalPage } from "@/features/eval/EvalPage"
import { KbTabs } from "@/app/AppShell"

function EvalRoute() {
  const { kbId } = Route.useParams()
  return (
    <div className="space-y-6">
      <KbTabs kbId={kbId} />
      <EvalPage kbId={kbId} />
    </div>
  )
}

export const Route = createFileRoute("/_authed/kb/$kbId/eval")({ component: EvalRoute })
```

- [ ] **Step 8: Build + full suite**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb/web && pnpm build && pnpm test
```
Expected: build `✓ built`; all tests PASS.

- [ ] **Step 9: Commit**

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb && git add web/src/features/eval/ web/src/routes/_authed/kb.\$kbId/eval.tsx && git commit -m "feat(web): eval dashboard — MetricCards, DriftReportTable, RunEvalDialog

MetricCards renders retrieval (precision/recall/MRR/grounding) + triad
means; DriftReportTable shows deltas with improved/regressed/unchanged
direction badges (null deltas → —). RunEvalDialog posts kind+JSONL.
Cards + drift tested from fixtures."
```

---

### Task 15: Sessions history + README + final verification

**Files:**
- Create: `web/src/features/sessions/api.ts`
- Create: `web/src/features/sessions/SessionsPage.tsx`
- Create: `web/src/features/sessions/SessionsPage.test.tsx`
- Create: `web/src/routes/_authed/kb.$kbId/sessions.tsx`
- Create: `web/README.md`
- Modify: `README.md` (repo root)

Design: `SessionsPage` lists sessions; clicking one loads its transcript (messages with role/content, reusing `CitationList` for assistant message citations). Lighter feature per the brief.

- [ ] **Step 1: Write the sessions api**

Create `web/src/features/sessions/api.ts`:
```ts
import { apiJSON } from "@/lib/apiClient"
import type { Citation, ListEnvelope, SessionRow, TranscriptMessage } from "@/lib/types"

interface TranscriptResponse {
  sessionId: string
  messages: TranscriptMessage[]
}

export function listSessions(kbId: string): Promise<ListEnvelope<SessionRow>> {
  return apiJSON<ListEnvelope<SessionRow>>(`/api/kb/${kbId}/sessions`)
}

export async function getTranscript(kbId: string, sid: string): Promise<TranscriptMessage[]> {
  const res = await apiJSON<TranscriptResponse>(`/api/kb/${kbId}/sessions/${sid}`)
  return res.messages
}

export type { Citation }
```

- [ ] **Step 2: Write the failing test**

Create `web/src/features/sessions/SessionsPage.test.tsx`:
```tsx
import { describe, it, expect, beforeEach, vi } from "vitest"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { SessionsPage } from "./SessionsPage"
import { installFetchRoutes, jsonResponse } from "@/test/helpers"
import { setAccessToken } from "@/lib/apiClient"

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={qc}>
      <SessionsPage kbId="kb-1" />
    </QueryClientProvider>,
  )
}

describe("SessionsPage", () => {
  beforeEach(() => {
    setAccessToken("tok")
    vi.restoreAllMocks()
  })

  it("lists sessions and loads a transcript on click", async () => {
    installFetchRoutes({
      "/api/kb/kb-1/sessions/s1": jsonResponse({
        sessionId: "s1",
        messages: [
          { id: "m1", role: "user", content: "hi", mode: "hybrid", createdAt: "2026-06-10" },
          { id: "m2", role: "assistant", content: "hello there", mode: "hybrid", createdAt: "2026-06-10" },
        ],
      }),
      "/api/kb/kb-1/sessions": jsonResponse({
        items: [{ id: "s1", title: "First chat", createdAt: "2026-06-10" }],
        next_cursor: "",
      }),
    })
    renderPage()
    await userEvent.click(await screen.findByText("First chat"))
    await waitFor(() => expect(screen.getByText("hello there")).toBeInTheDocument())
  })
})
```

- [ ] **Step 3: Run the test to verify it fails**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb/web && pnpm vitest run src/features/sessions/SessionsPage.test.tsx
```
Expected: FAIL — `SessionsPage` not found.

- [ ] **Step 4: Implement SessionsPage**

Create `web/src/features/sessions/SessionsPage.tsx`:
```tsx
import { useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { listSessions, getTranscript } from "./api"
import { CitationList } from "@/features/ask/CitationList"
import { Button } from "@/components/ui/button"

export function SessionsPage({ kbId }: { kbId: string }) {
  const [activeSid, setActiveSid] = useState<string | null>(null)
  const sessionsQuery = useQuery({ queryKey: ["sessions", kbId], queryFn: () => listSessions(kbId) })
  const transcriptQuery = useQuery({
    queryKey: ["transcript", kbId, activeSid],
    queryFn: () => getTranscript(kbId, activeSid!),
    enabled: !!activeSid,
  })

  return (
    <div className="grid grid-cols-3 gap-6">
      <div className="space-y-1">
        <h2 className="mb-2 text-sm font-medium text-muted-foreground">Sessions</h2>
        {sessionsQuery.data?.items.length === 0 && (
          <p className="text-sm text-muted-foreground">No sessions yet.</p>
        )}
        {sessionsQuery.data?.items.map((s) => (
          <Button
            key={s.id}
            variant={activeSid === s.id ? "secondary" : "ghost"}
            className="w-full justify-start"
            onClick={() => setActiveSid(s.id)}
          >
            {s.title || s.id}
          </Button>
        ))}
      </div>
      <div className="col-span-2 space-y-4">
        {!activeSid && <p className="text-sm text-muted-foreground">Select a session.</p>}
        {transcriptQuery.data?.map((m) => (
          <div key={m.id} className="rounded-lg border p-3">
            <div className="mb-1 text-xs font-medium uppercase text-muted-foreground">{m.role}</div>
            <div className="whitespace-pre-wrap text-sm">{m.content}</div>
            {m.citations && m.citations.length > 0 && (
              <div className="mt-2">
                <CitationList citations={m.citations} />
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  )
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb/web && pnpm vitest run src/features/sessions/SessionsPage.test.tsx
```
Expected: PASS.

- [ ] **Step 6: Wire the sessions route**

Create `web/src/routes/_authed/kb.$kbId/sessions.tsx`:
```tsx
import { createFileRoute } from "@tanstack/react-router"
import { SessionsPage } from "@/features/sessions/SessionsPage"
import { KbTabs } from "@/app/AppShell"

function SessionsRoute() {
  const { kbId } = Route.useParams()
  return (
    <div className="space-y-6">
      <KbTabs kbId={kbId} />
      <SessionsPage kbId={kbId} />
    </div>
  )
}

export const Route = createFileRoute("/_authed/kb/$kbId/sessions")({ component: SessionsRoute })
```

- [ ] **Step 7: Write web/README.md**

Create `web/README.md`:
```markdown
# llm-agent-kb web

React 19 + TypeScript SPA for the llm-agent-kb backend (GraphRAG Q&A platform).

## Stack

- Vite 8 + React 19 + TypeScript 6
- Tailwind CSS v4 (via `@tailwindcss/vite`; theme in `src/index.css`, no `tailwind.config.js`)
- shadcn/ui (radix-nova) primitives in `src/components/ui/`
- TanStack Router (file-based, `tsr generate`) + TanStack Query
- SSE via `@microsoft/fetch-event-source` (POST + Bearer) — `src/lib/sse.ts`
- Tests: Vitest + @testing-library/react + jsdom (mocked fetch + SSE; no live backend)

## Develop

```bash
pnpm install
pnpm dev      # tsr generate + vite; proxies /api → http://localhost:8080
```

Start the Go backend (`kbd`) on :8080 separately so the dev proxy can reach it.

## Build & test

```bash
pnpm build    # tsr generate + tsc -b + vite build → dist/
pnpm test     # vitest run (all unit/component tests)
pnpm lint     # eslint
```

## Layout

`src/{app,components,features,lib,routes,test}/`. Features are self-contained dirs
(`api.ts`, page + component `.tsx`, tests). The typed API client (`src/lib/apiClient.ts`)
keeps the access token in memory and refreshes on 401; response types live in
`src/lib/types.ts`. The SSE wire contract (token/done/error) matches the backend
`POST /api/kb/{id}/ask/stream` endpoint.
```

- [ ] **Step 8: Mention the SPA in the repo root README**

In `README.md` (repo root), the Architecture paragraph currently ends with `... Tenancy/auth comes from the imported llm-agent-authz library.` Add a new line after the Architecture paragraph:
```markdown

The React SPA lives in `web/` (Vite + React 19 + TypeScript + Tailwind v4 + shadcn/ui + TanStack Router/Query). It calls the REST + SSE endpoints above; see `web/README.md`. Dev: `cd web && pnpm install && pnpm dev` (proxies `/api` to the backend on :8080).
```

- [ ] **Step 9: Full build + full test + lint (the M5b gate)**

Run:
```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb/web && pnpm build && pnpm test && pnpm lint
```
Expected: `✓ built`; `Test Files  N passed` / `Tests  M passed` (every test file green); `pnpm lint` exits 0 (ui/** and routeTree.gen.ts ignored; hand-written code clean).

- [ ] **Step 10: Commit**

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/llm-agent-kb && git add web/ README.md && git commit -m "feat(web): sessions history + web README + root README SPA note

SessionsPage lists sessions and loads transcripts (reuses CitationList).
web/README documents the stack + commands; root README points at web/.
Full build + test + lint green — M5b SPA complete."
```

---

## Self-Review

**1. Spec §10 view coverage:**
- `/login` → Task 7 (LoginForm, react-hook-form + zod). ✅
- `/orgs`, `/orgs/$orgId/kbs` (KbTable, CreateKbDialog) → Task 9 (KbListPage: create-org + list + create-kb dialog). Members/RBAC panel is OUT of M5b scope (the brief's scope list 5 specifies "create-org + create-kb flow"; the backend `memberships` endpoints are not in the brief's endpoint list). Noted as a deliberate scope cut.
- `/kb/$kbId/documents` (Dropzone, UrlInput, PasteText, DocStatusTable) → Tasks 10–11 (UploadForm paste/url + DocStatusTable with status badge, live SSE progress, retry/delete). File upload shape supported in `UploadInput`; file-byte parsing is backend-side. ✅
- `/kb/$kbId/ask` (ModeSwitch, AnswerPane typewriter, CitationList + sectionPath, DiagnosticsDrawer) → Task 13. Streaming for vector/hybrid; non-stream for global/drift. ✅
- `/kb/$kbId/graph` (communities) → OUT of M5b scope (the brief's scope list 1–10 does not include the graph view; backend community endpoints exist but the brief omits them). Deliberate cut, stated here.
- `/kb/$kbId/eval` (MetricCards, DriftReportTable, RunEvalDialog) → Task 14. TrendChart/TriadPanel from §10 reduced to MetricCards (triad means included) per the brief's scope 8. ✅
- `/kb/$kbId/sessions` (SessionList, SessionTranscript reusing CitationList) → Task 15. ✅

**Brief scope 1–10 coverage:** (1) scaffold+Tailwind v4+shadcn+TanStack+Vitest+proxy → Tasks 1–2 (verified building). (2) api client + auth refresh single-flight + SSE helper + auth store + tests → Tasks 4–6. (3) login view → Task 7. (4) app shell + protected routes → Task 8. (5) org/kb list+create → Task 9. (6) documents upload+list+status+SSE progress+retry+delete → Tasks 10–11. (7) ask streaming + ModeSwitch + AnswerPane + CitationList + DiagnosticsDrawer → Tasks 12–13. (8) eval dashboard MetricCards + DriftReportTable + RunEvalDialog + runs table → Task 14. (9) sessions history → Task 15. (10) final build+test+lint + web README + root README → Task 15. ✅

**2. Placeholder scan:** No TBD/TODO/"add styling here"/"implement later"/"similar to Task N". Every component is complete real `.tsx`/`.ts`; every command has expected output; every test has full code. The one `eslint-disable` line in DocStatusTable is intentional (stable effect dep on a derived key string), not a placeholder. ✅

**3. Type consistency (the load-bearing check):**
- `Citation {chunkId, docId, title, sectionPath?, score, snippet}` defined once in `lib/types.ts` (Task 4); reused by `sse.ts`, `useAskStream`, `AskPage`, `CitationList`, sessions, eval — never redefined. Matches the Go `retrieval.Citation` json tags. ✅
- SSE frame types `StreamTokenData {text}`, `StreamDoneData {citations, diagnostics:{mode,hitCount}, sessionId}`, `StreamErrorData {error}` defined in `lib/types.ts` (Task 4), consumed identically in `sse.ts` (Task 5), `useAskStream` (Task 12), `AskPage` (Task 13). Match the M5a wire contract exactly. ✅
- `streamAsk(url, body, accessToken, handlers, client?, signal?)` signature defined in Task 5; `useAskStream` injects a `typeof realStreamAsk` (Task 12) and `AskPage` accepts `streamAsk?: typeof realStreamAsk` (Task 13) — one signature, threaded consistently. The fakes in tests match by ignoring trailing optional args. ✅
- `EvalResult`/`RetrievalMetrics`/`GenerationMetrics`/`DriftView`/`MetricDelta` in `lib/types.ts` mirror the Go `eval.EvalResult` json (`precisionAtK/recallAtK/mrr/groundingAtK/examples/topK`, `meanGroundedness/meanAnswerRelevance`, `deltas[].direction`, `prev/curr/delta` nullable). `MetricCards`/`DriftReportTable`/`RunEvalDialog`/`EvalPage` (Task 14) consume them consistently; `fmt(null)→"—"` handles the JSON-null deltas. ✅
- `apiFetch`/`apiJSON`/`setAccessToken`/`getAccessToken` (Task 4) used uniformly by every feature `api.ts` and the auth context. Auth body is PascalCase `{Email,Password}` (Task 6/7) per the Go authz decoder; refresh/logout carry `X-CSRF: 1` (verified in the authz handler source). ✅
- TanStack route params: `Route.useParams()` returns `{kbId}` typed against the file route names `/_authed/kb/$kbId/{documents,ask,eval,sessions}`; `<Link to="/kb/$kbId/...">` params match. (The verified scaffold confirmed the plugin generates these from the route files.) ✅

**Risks / gotchas found and mitigated (grounded in the actual scaffold I ran):**
1. **TS 6 `baseUrl` deprecation (TS5101)** — verified failing; the plan omits `baseUrl` everywhere and uses `paths` alone. This is the single most likely trap for an engineer following stale shadcn docs (which still show `baseUrl`).
2. **Route tree generation ordering** — verified `tsc -b` runs before the vite plugin emits `routeTree.gen.ts`, breaking `pnpm build`. Mitigated by `@tanstack/router-cli` + prefixing every script with `tsr generate`, and gitignoring the generated file.
3. **shadcn CLI changed** — `init` now requires a `-p <preset>` (e.g. `nova`) and uses `-b radix`; `--base-color` no longer exists. Verified non-interactive `init -b radix -p nova -y` works and detects Tailwind v4 + the `@/` alias.
4. **shadcn ui files trip eslint `react-refresh/only-export-components`** (they export a `*Variants` const alongside the component) — verified; mitigated by eslint-ignoring `src/components/ui/**`.
5. **`@testing-library/jsdom` does not exist** — the correct packages are `jsdom` (env) + `@testing-library/jest-dom` (matchers); the install command uses those.
6. **Refresh CSRF** — the authz `/refresh` + `/logout` require header `X-CSRF: 1` (double-submit) or return 403; the apiClient + auth context set it. Confirmed in the authz source, not assumed.
7. **Stream vs non-stream citation divergence** — per M5a, `/ask/stream` citations come from raw hits and may differ in count/order from `/ask`; the SPA only relies on the field shape (same `Citation`), which holds. No code depends on count parity.

Compatibility verdict: **Tailwind v4 + shadcn (radix-nova) + React 19 + TanStack Router/Query + Vitest 4 all build and test green together** — verified end-to-end by scaffolding the real project with the live toolchain (Node 22.22, pnpm 10.30) before writing this plan, including a production `pnpm build` and RTL render test.
```

