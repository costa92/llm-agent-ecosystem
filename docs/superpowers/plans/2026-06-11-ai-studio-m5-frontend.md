# AI Studio M5 — 前端 React SPA 实现计划

> **给执行 agent：** 必备子技能：用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务执行。所有步骤用 `- [ ]` 复选框追踪。
>
> **本计划是中文计划：** 散文/标题用中文，代码/标识符/命令用英文。每个 API 断言均锚定到真实 Go handler 的 `writeJSON`/路由/中间件。

**目标：** 在全新目录 `llm-agent-studio/web/` 里构建一个真实、可测试的 React 19 SPA，驱动 studio 后端（M1–M4 已落地的接口表）—— 登录（内存 access token + 自动刷新）、组织/项目管理、实时制片轨道（SSE 状态机）、剧本/分镜视图、HITL 审核看板、资产库、成本中心、模型配置、Prompt Builder —— 完成里程碑 M5（前端专属里程碑，**后端零改动**），close-out 时打 `v0.5.0`（与现 `main @ v0.4.0` 衔接）。

**架构：** Vite + React 19 + TypeScript SPA，feature-dir 布局（`src/{app,components,features,lib,routes,test}/`）。Tailwind v4 经 `@tailwindcss/vite` 插件（**无** `tailwind.config.js`，theme 写在 `src/index.css` 的 `@theme`）。shadcn/ui（radix-nova preset）出 primitives；制片轨道/pip/血缘等自建组件用 cva。**file-based** 路由经 `@tanstack/router-plugin` + `@tanstack/router-cli`（`tsr generate`）。服务端状态用 TanStack Query。SSE 用 `@microsoft/fetch-event-source`（**见下方 §SSE 决策——已从源码验证**）。单一 typed `apiFetch` 包装注入内存 access token 并做 single-flight refresh-on-401。测试 = Vitest + @testing-library/react + jsdom + @testing-library/jest-dom，mock `fetch` 与注入式 SSE client —— **无真实后端、无浏览器**。

**栈（沿用已验证的 kb M5b 栈；sandbox 可达 node v22.22 / pnpm 10.30 / npm 10.9）：** react 19 + typescript 6 + vite 8 + tailwindcss 4 + @tailwindcss/vite + @tanstack/react-router + @tanstack/router-plugin + @tanstack/router-cli + @tanstack/react-query + @microsoft/fetch-event-source + react-hook-form + zod + @hookform/resolvers + shadcn（radix-nova）+ sonner + class-variance-authority + clsx + tailwind-merge + lucide-react；dev：vitest + @testing-library/react + @testing-library/user-event + @testing-library/jest-dom + jsdom + @vitest/coverage-v8。**这是 Node/pnpm 工具链，不走 GOWORK。**

---

## 验证策略（先读，贯穿全计划）

- **每个任务结尾**，从 `llm-agent-studio/web/` 运行：`pnpm build`（含 `tsr generate` + `tsc -b` + `vite build`）、`pnpm test`（`vitest run`）、`pnpm lint`（`eslint .`）。三者全绿才算任务完成。
- **前端 "TDD" 范围**：对承载逻辑的单元（apiClient 的 refresh-on-401、SSE 制片轨道 reducer/状态机、role-gating、zod schema、keyset 分页累积）—— **先写 Vitest 测试再实现**。对纯展示视图 —— render/smoke 测试足矣。
- **明确不在 M5 范围**：真实浏览器/Playwright E2E（sandbox 无浏览器）。跨栈联调（dev server vs 活后端）记为**手动可选 smoke**，不进 CI、不作完成判据。
- **后端零改动**：M5 是前端专属里程碑。若发现微小 CORS/静态服务缺口，**只标记、不静默新增后端代码**（见 §已知缺口）。

---

## 已验证的关键事实（从真实源码，执行 agent 不必重新发现）

### AUTH 机制（从 `llm-agent-authz@v0.1.0` 源码验证 —— 决定 SSE 策略）

- **access token = `Authorization: Bearer <token>` 请求头**。`authz/httpapi/middleware.go` 的 `Authenticate(iss)` **只**读 `Authorization` 头的 `Bearer ` 前缀，`iss.Verify(tok)` 校验；**无任何 cookie 回退**给 access token。缺/错 token → `401`。
- **refresh token = httpOnly cookie `authz_refresh`**（`authz/httpapi/handlers.go`：`HttpOnly:true, Secure:true, SameSite:Strict, Path:"/api/auth"`）。
- **登录接口**（studiod `main.go` 把 `authzhttp.New(authService).Mount(mux, "/api/auth")`）：
  - `POST /api/auth/login` body `struct{ Email, Password string }` —— Go 默认 JSON 解码大小写不敏感，PascalCase `{"Email","Password"}` 与 lowerCamel `{"email","password"}` **都能解**（authz 测试用例就用 `{"email","password"}`）。本计划登录请求体用 `{"email","password"}`（与 authz 自带测试一致）。成功 → `200 {"access_token": string, "expires_in": number}` + `Set-Cookie: authz_refresh=...; HttpOnly`；凭据错 → `401`。
  - `POST /api/auth/refresh` —— **必须带请求头 `X-CSRF: 1`**（double-submit，缺则 `403`），读 httpOnly cookie（需 `credentials:"include"`）→ `200 {"access_token","expires_in"}` + 轮换 cookie；过期/无效 → `401`。
  - `POST /api/auth/logout` —— **必须带 `X-CSRF: 1`**，清 cookie → `204`。

### SSE 决策（**用 `@microsoft/fetch-event-source`，不用原生 `EventSource`**）

- 制片轨道实时流 `GET /api/projects/{id}/events/stream` 挂在 `proj(roleViewer, ...)` 下 = `Authenticate(d.Issuer)` 包裹（`httpapi.go:107`）—— 即**必须带 `Authorization: Bearer` 头**。
- 原生 `EventSource` **无法设置请求头**（只能带 cookie），而 studio 的 access token 不在 cookie 里（见上）—— 故原生 `EventSource` 不可用。
- **结论：用 `@microsoft/fetch-event-source`**（GET + `Authorization: Bearer` 头 + 自动重连）。这与 UI-spec §2 的推荐一致。`fetchEventSource` 默认在标签页隐藏时会断流 —— 设 `openWhenHidden: true`。
- **续传机制（已核对 `sse.go:51-66`）：服务端不发 `id:` 字段、不读 `Last-Event-ID` 请求头，每次（重）连都从 `after=0` 全量回放历史事件。** 故 NOT 依赖 `Last-Event-ID`；防重复渲染靠客户端 **按 `seq` 去重的 reducer**（Task 8）—— 重连全量回放的旧帧被 seq-dedup 吞掉，再无缝接实时帧。

### studiod 端口 = `:8083`

`internal/config/config.go:94` `HTTP_ADDR` 默认 `:8083`；`Dockerfile` `EXPOSE 8083`。Vite dev proxy `/api` → `http://localhost:8083`。

---

## 后端线缆契约（逐条对照真实 Go handler 的 `writeJSON` —— SPA 的 TS 类型必须匹配）

> dev 基址：Vite proxy 转发 `/api/*` → `http://localhost:8083`。除登录/刷新外的请求都带 `Authorization: Bearer <access>`。RBAC 由后端 `RequireScopeRole`（scope_kind=`"org"`）强制，**前端按角色隐藏/禁用入口，仅作 UX，不是安全边界**。
> 列表信封不统一，逐条注明：项目/资产库用 `{items, next_cursor}`；其余多为 `{items}`。

**Org / 项目**（`handlers.go`）：
- `POST /api/orgs` body `{"name": string}` → `200 {"id","name"}`。任意已认证用户；创建者成 org_admin。（`createOrgHandler`）
- `POST /api/orgs/{org}/projects` body `{"name","brief","contentType","targetPlatform","style"}` → `200 Project`（**editor+**，`scoped(roleEditor, orgScope)`）。`name` 必填，否则 `400`。（`createProjectHandler`）
- `GET /api/orgs/{org}/projects?limit=&cursor=` → `200 {"items": Project[], "next_cursor": string}`（**viewer+**）。（`listProjectsHandler`）
- `GET /api/projects/{id}` → `200 Project`；不存在 → `404`（**viewer+**）。（`getProjectHandler`）
- `POST /api/projects/{id}/run` → `202 {"planId","valid","fallbackUsed"}`（**editor+**）。配额超限 → `429`；项目不存在 → `404`。（`runHandler`）
- `POST /api/projects/{id}/cancel` → `200 {"status":"canceled"}`（**editor+**）。（`cancelHandler`）
- `GET /api/projects/{id}/events?afterSeq=` → `200 {"items": Event[]}`（**viewer+**，回放，按 seq 分页，每次最多 200 行）。（`listEventsHandler`）
- `GET /api/projects/{id}/events/stream` → **SSE**（**viewer+**，见 §SSE 决策与下方帧契约）。（`streamEventsHandler`）
- `GET /api/projects/{id}/todos` → `200 {"items": object[]}`（**viewer+**）。（`todosHandler`）
- `GET /api/projects/{id}/script` → `200 <raw script JSON>`（**viewer+**，**注意：直接 `w.Write(content)`，不是 `{items}` 信封**）；未生成 → `404 "no script yet"`。（`scriptHandler`）
- `GET /api/projects/{id}/shots` → `200 {"items": object[]}`（**viewer+**）。（`shotsHandler`）
- `GET /api/projects/{id}/assets?status=` → `200 {"items": object[]}`（**viewer+**，项目维度资产）。（`projectAssetsHandler`）

**Project struct**（`project/store.go`）：`{id, orgId, name, description, contentType, targetPlatform, style, status, createdBy}`。`status ∈ {draft,planning,running,review,completed,failed,canceled}`（UI-spec §7.2）。**注意 `brief` 是 create 入参，落库读出时是 `description` 字段**。

**Event struct**（`events/store.go` + `sse.go`）：`GET /events` 列表元素 = `{seq, kind, todoId?, payload?}`；SSE 帧 `data:` 也是 `{seq, kind, todoId, payload}`，SSE 行的 `event:` 名 = `kind`（见白名单）。

**SSE 事件白名单**（`sse.go:22` 的 `sseEventNames`，**9 种** —— 比 UI-spec §6.1 列的 7 种多 `asset_prescreened`、`asset_submitted`）：
`planner_started · todo_ready · todo_started · todo_finished · todo_failed · asset_generated · asset_prescreened · asset_submitted · run_done`。未在白名单的 kind 以通用 `message` 事件流出（原 kind 仍在 payload）。`run_done` 是终止帧（服务端见到后关闭流）。
> UI 状态机至少要处理这 9 种命名事件 + `message` 兜底。`asset_submitted`（M4 异步提交）/`asset_prescreened`（M3 ReviewAgent 预筛）按 UI-spec §6 语义并入 pip/日志（见 Task 8）。

**Prompt**（`m2handlers.go`）：
- `GET /api/prompt-styles` → `200 {"styles": Style[]}`，`Style = {name, suffix}`（**auth-only**，`authOnly`）。真实 7 种 `name`：`日漫 / 吉卜力 / 皮克斯 / 迪士尼 / 写实 / 赛博朋克 / 国风`（`prompt/prompt.go`）。
- `POST /api/prompt/build` body `{"prompt","style"}` → `200 {"prompt": string}`（**auth-only**）；`prompt` 必填否则 `400`。

**HITL / 资产**（`m2handlers.go`）：
- `POST /api/assets/{id}/accept` → `200 {"id","status":"accepted"}`（**admin**，`asset(roleAdmin)`）；非 `pending_acceptance` → `409 "asset not pending_acceptance"`。
- `POST /api/assets/{id}/reject` → `200 {"id","status":"rejected"}`（**admin**）；非 pending → `409`。
- `POST /api/assets/{id}/regenerate` body `{"prompt"}` → `200 {"newAssetId","todoId","status":"generating"}`（**admin**）；非 pending → `409`；配额超限 → `429`。
- `GET /api/orgs/{org}/assets?project=&type=&status=&style=&tag=&limit=&cursor=` → `200 {"items": Asset[], "next_cursor": string}`（**viewer+**，keyset 分页，cursor=最后一个 asset id）。（`libraryHandler`）
- `GET /api/assets/{id}` → `200 {"asset": Asset, "versions": Asset[]}`（**viewer+**，含版本血缘）；不存在 → `404`。（`getAssetHandler`）
- `GET /api/assets/{id}/content` → `302` 跳转到签名 URL（短 TTL，`signedURLTTL = 10m`）；provider 托管的 URL-only 资产直接 302 到外链（**viewer+**）。（`assetContentHandler`）
- `GET /api/blob/{key...}?exp=&sig=` → **无 auth**（HMAC sig+exp 在 query 里把关，spec §10），校验通过后回源字节，带 `X-Content-Type-Options:nosniff` + `Content-Security-Policy:sandbox`。仅 localfs 模式挂载。

**Asset struct**（`assets/store.go`）：`{id, projectId, shotId, todoId, type, blobKey, url, prompt, style, provider, model, status, version, parentAssetId, tags[], prescreenScore, prescreenFlags[], prescreenNote, externalJobId}`。
> **注意：Asset 无 `signedUrl` 字段**（UI-spec §7.6 DTO 列了 `signedUrl`，但后端不下发）。要拿可显示的图，**走 `GET /api/assets/{id}/content`（302→签名 URL）**：直接把该 URL 作为 `<img src>`（浏览器自动跟 302），或在 SPA 内对 `/api/assets/{id}/content` 发请求拿 `Location`。M5 用前者（`<img src="/api/assets/{id}/content">` + 带 token 的问题见 Task 10 决策）。

**模型管理**（`m2handlers.go`，**注意 M3 handler 都在 `m2handlers.go`，无独立 `m3handlers.go`**）：
- `GET /api/model-catalog` → `200 {"catalog": CatalogEntry[]}`，`CatalogEntry = {provider, model, kind, label}`（**auth-only**）。真实 catalog 含 image（openai/google/minimax/volcengine）+ video（fake/runway/kling/google）+ audio（fake/openai）条目。
- `POST /api/orgs/{org}/model-configs` body `{"kind","provider","model","enabled","isDefault","params"}` → `200 ModelConfig`（**admin**）；`provider`+`model` 必填否则 `400`；含密钥型 param → `400 ErrSecretParam`。
- `GET /api/orgs/{org}/model-configs` → `200 {"items": ModelConfig[]}`（**admin**）。`ModelConfig = {id, orgId, kind, provider, model, enabled, isDefault, params?}`。**无 API key 字段 —— 密钥在服务端，永不下发。**

**成本中心**（`m2handlers.go`，全部 **admin**，时间范围 `?from=&to=` RFC3339，畸形 → `400`）：
- `GET /api/orgs/{org}/cost` → `200 Aggregate`，`Aggregate = {generations, tokens, imageCount, costMicros}`。（`orgCostHandler`）
- `GET /api/projects/{id}/cost` → `200 Aggregate`（**admin**）。（`projectCostHandler`）
- `GET /api/orgs/{org}/cost/projects` → `200 {"items": ProjectAggregate[]}`，`ProjectAggregate = {projectId, projectName, generations, tokens, imageCount, costMicros}`（内嵌 Aggregate 字段）。（`orgCostProjectsHandler`）
- `GET /api/orgs/{org}/generations?limit=` → `200 {"items": LedgerEntry[]}`，`LedgerEntry = {id, projectId, projectName, kind, provider, model, tokens, imageCount, costMicros, latencyMs, createdAt}`。（`orgGenerationsHandler`）

---

## 设计 token（从 `ai-studio-ui-prototype.html` 的 `:root` 逐字提取 —— 不重新设计）

```
--bg-base:#17191E; --bg-surface:#1F232A; --bg-raised:#272C34;
--line:#343A44; --text-1:#EDEEF0; --text-2:#9AA1AC; --text-3:#666D78;
--amber:#E8A33D; --script:#5C9BD6; --board:#9C7BDA; --asset:#E8A33D; --review:#4FB286; --danger:#E05F5B;
--mono:"JetBrains Mono",monospace; --disp:"Space Grotesk","Noto Sans SC",sans-serif;
```
- 字体：`<link>` 加载 `Space Grotesk:wght@500;700` + `JetBrains Mono:wght@400;600` + `Noto Sans SC:wght@400;500;600`（Google Fonts `display=swap`，原型用法）。body `font:13px/1.55 "Noto Sans SC"`。
- `focus-visible`：`outline:2px solid var(--amber); outline-offset:2px`（全局）。
- per-agent 语义色：S2 Script=`--script` 蓝 / S3 Storyboard=`--board` 紫 / S4 Asset=`--asset` 琥珀 / S5 Review=`--review` 绿。
- 纯暗色单主题（无浅色态）。Spacing/Typography/Radii 取值见 UI-spec §5（4 基数 spacing；正文锁 13px；display 30px/700；radii sm4/md8/lg10–12/full999）。

---

## 文件结构（hand-written；`node_modules/`、`dist/`、`src/routeTree.gen.ts` 为生成物）

```
web/
├── package.json / vite.config.ts / tsconfig.json / tsconfig.app.json / eslint.config.js / .gitignore / components.json / index.html / README.md
└── src/
    ├── index.css                 # @import "tailwindcss" + shadcn theme + 暗色 token 覆写 (Task 1/2/3)
    ├── main.tsx                   # AuthProvider > QueryClientProvider > RouterProvider (Task 4/6)
    ├── routeTree.gen.ts          # GENERATED by tsr — gitignored
    ├── test/{setup.ts, helpers.ts}        # jest-dom; mockFetch + 注入式 SSE 帧脚本 (Task 1/5)
    ├── lib/
    │   ├── utils.ts (cn)         # Task 2
    │   ├── types.ts              # 所有后端线缆类型 (Task 5)
    │   ├── apiClient.ts (+test)  # apiFetch + 内存 token + single-flight refresh-on-401 (Task 5)
    │   ├── sse.ts (+test)        # streamRunEvents() over fetch-event-source (Task 7)
    │   └── timeline.ts (+test)   # SSE→节点状态机 reducer (Task 8)
    ├── components/
    │   ├── ui/                   # shadcn primitives (Task 2)
    │   └── studio/               # 自建 cva 组件: Badge/Button/TimelineStage/PipGroup/SlateBar/AssetCard/SseIndicator/StatCard/BarRow/LineageTrail (Task 3/8/...)
    ├── app/
    │   ├── auth.tsx (+test)      # AuthProvider + useAuth + role (Task 6)
    │   ├── rbac.ts (+test)       # 角色判定 + 视图门禁 (Task 6)
    │   └── AppShell.tsx          # IconRail 导航 + 角色过滤入口 (Task 4)
    ├── features/
    │   ├── projects/             # 列表/建项目 (Task 9)
    │   ├── workflow/             # 工作台 + 制片轨道 + SSE + 剧本/分镜 (Task 10)
    │   ├── review/               # HITL 审核看板 (Task 11)
    │   ├── library/              # 资产库 (Task 12)
    │   ├── cost/                 # 成本中心 + 模型配置 (Task 13)
    │   └── prompt/               # Prompt Builder (Task 14)
    └── routes/                    # TanStack file-based 路由 (Task 4 起)
        ├── __root.tsx / login.tsx / _authed.tsx / _authed/index.tsx
        └── _authed/{orgs.$org.projects, projects.$id, projects.$id.script, projects.$id.storyboard,
                      orgs.$org.review, orgs.$org.assets, orgs.$org.cost, orgs.$org.model-configs}.tsx
```

---

### Task 1：脚手架（Vite + TS + Tailwind v4 + 测试 harness + proxy）

**做什么：** 在仓内新建 `web/`，装栈，配 vite/tsconfig/eslint/scripts/proxy，跑通空壳 build+test+lint。

- [ ] **Step 1：确认仓与分支。** `git -C /home/.../llm-agent-studio status`；在 studio 仓开前端分支（如 `feat/m5-frontend`），不在 `main` 上直接改。
- [ ] **Step 2：脚手架。** `cd /home/.../llm-agent-studio && pnpm create vite@latest web --template react-ts`。
- [ ] **Step 3：装依赖。**
  ```bash
  cd web && pnpm install && \
  pnpm add tailwindcss @tailwindcss/vite @tanstack/react-router @tanstack/react-query @microsoft/fetch-event-source react-hook-form zod @hookform/resolvers class-variance-authority clsx tailwind-merge lucide-react sonner && \
  pnpm add -D @tanstack/router-plugin @tanstack/router-cli @tanstack/react-router-devtools vitest @testing-library/react @testing-library/user-event @testing-library/jest-dom jsdom @vitest/coverage-v8
  ```
  注意：测试 DOM 包是 `jsdom`，matcher 是 `@testing-library/jest-dom`；**没有** `@testing-library/jsdom` 这个包。
- [ ] **Step 4：`vite.config.ts`** —— plugins `[tanstackRouter({target:"react",autoCodeSplitting:true}), react(), tailwindcss()]`；`resolve.alias { "@": ./src }`；`server.proxy { "/api": { target: "http://localhost:8083", changeOrigin: true } }`（**端口 8083**）；`test { globals:true, environment:"jsdom", setupFiles:["./src/test/setup.ts"] }`。
- [ ] **Step 5：tsconfig 加 `@/*` 别名（绝不加 `baseUrl`）。** 在 `tsconfig.app.json` 与 `tsconfig.json` 的 `compilerOptions` 里加 `"paths": { "@/*": ["./src/*"] }`。**TS 6 弃用 `baseUrl`（`tsc -b` 报 `TS5101`）—— 仅用 `paths`。**
- [ ] **Step 6：scripts。** `dev:"tsr generate && vite"`、`build:"tsr generate && tsc -b && vite build"`、`test:"tsr generate && vitest run"`、`lint:"eslint ."`、`preview:"vite preview"`。（`router-plugin` 仅在 vite 阶段生成 `routeTree.gen.ts`，而 `tsc -b` 在它之前跑会失败 → 所有脚本前置 `tsr generate`。）
- [ ] **Step 7：`src/index.css`** = `@import "tailwindcss";`（shadcn token 在 Task 2/3 写入）；`src/test/setup.ts` = `import "@testing-library/jest-dom/vitest"`；`src/lib/utils.ts` = `cn()`。
- [ ] **Step 8：最小 file-based 路由 + providers。** 删 `src/App.tsx`/`App.css`；建 `src/routes/__root.tsx`（`createRootRoute` + `<Outlet/>`）、`src/routes/index.tsx`（占位）、`src/main.tsx`（`createRouter` + `QueryClientProvider` + `RouterProvider`，含 `declare module` 类型注册）。
- [ ] **Step 9：eslint + gitignore。** `eslint.config.js` 的 `globalIgnores` 加 `'dist'`、`'src/routeTree.gen.ts'`、`'src/components/ui'`（shadcn ui/* 导出 component+variants 触发 `react-refresh/only-export-components`）；`.gitignore` 追加 `src/routeTree.gen.ts`。
- [ ] **Step 10：建 `cn` smoke 测试**，跑 `pnpm build && pnpm test && pnpm lint` 三绿。
- [ ] **Step 11：commit** —— `feat(web): scaffold Vite+React19+TS SPA (tailwind v4, tanstack router/query, vitest)`。

**Verify（`web/`）：** `pnpm build`（无 TS5101/TS2307）、`pnpm test`（1 passed）、`pnpm lint`（clean）。

---

### Task 2：shadcn/ui init（radix-nova）+ primitives

- [ ] **Step 1：非交互 init。** `pnpm dlx shadcn@latest init -b radix -p nova -y`（preset 即便 `-y` 也要 `-b radix -p nova`；写 `components.json` style=`radix-nova`、baseColor=neutral、cssVariables、icon=lucide，重写 `src/index.css`）。
- [ ] **Step 2：add primitives。** `pnpm dlx shadcn@latest add button card input label dialog sheet checkbox dropdown-menu select table textarea badge skeleton sonner -y`（`sheet`=审核 Drawer 底座；`select`/`checkbox`=过滤；`skeleton`=骨架；`dropdown-menu`=时间范围/排序）。
- [ ] **Step 3：render smoke 测试**（shadcn Button 渲染）证明 RTL+jsdom+shadcn 链路通。
- [ ] **Step 4：build+test+lint 三绿；commit** —— `feat(web): shadcn/ui init (radix-nova) + primitives`。

**Verify：** `pnpm build`（CSS chunk 增大）、`pnpm test`、`pnpm lint`。

---

### Task 3：设计 token 覆写 + 字体 + 基础 shell 主题 + 自建组件骨架

**做什么：** 把原型 `:root` token 逐字写进 `@theme`，锁暗色单主题，建自建 cva 组件的空壳与 token 映射。

- [ ] **Step 1：`index.css` 覆写。** 在 shadcn 写入的 theme 后，用上方"设计 token"块的值覆写 CSS 变量（暗色 token、per-agent 色、`--disp`/`--mono`）；body `font:13px/1.55 "Noto Sans SC"`；全局 `focus-visible` outline=amber；`prefers-reduced-motion:reduce` 停 `slide`/`pulse`/`spin`/条纹（UI-spec §9）。`<html lang="zh-CN">`。
- [ ] **Step 2：字体 `<link>`** 写进 `index.html`（Space Grotesk + JetBrains Mono + Noto Sans SC，`display=swap`）。`next-themes` 不装（单暗色主题）。
- [ ] **Step 3：建自建组件（cva，token 映射，先做无状态的）：** `components/studio/Badge.tsx`（running/done/pending/rejected）、`Button.tsx`（amber/ghost/green/red + Kbd 槽）、`Kbd.tsx`、`StatCard.tsx`、`BarRow.tsx`、`WarnStrip.tsx`、`EventLog.tsx`、`SseIndicator.tsx`（connected/reconnecting/disconnected）。状态机相关的（TimelineStage/PipGroup/SlateBar/LineageTrail/AssetCard）留到 Task 8/11。每个组件配 render smoke 测试。
- [ ] **Step 4：build+test+lint 三绿；commit** —— `feat(web): dark-theme tokens from prototype + base cva components`。

**Verify：** `pnpm build`、`pnpm test`、`pnpm lint`。

---

### Task 4：apiClient + 路由模型 + AppShell（先把骨架接起来）

> 路由决策：**file-based**（`@tanstack/router-plugin` + `tsr generate`）。8+ 路由跨 6 个 feature，`$id`/`$org` 类型安全、`?asset=`/过滤态用 typed search params，file-based 1:1 映射、零手维护 route tree。

- [ ] **Step 1：建 `_authed.tsx` 受保护布局**（`beforeLoad` 检查 `getAccessToken()`，无则 `redirect("/login")`）+ `_authed/index.tsx`（`/` → 重定向到默认 org 项目列表或一个 org 选择页）。
- [ ] **Step 2：`app/AppShell.tsx`** —— 64px IconRail（logo + 项目/审核/资产/成本 + avatar），`on`=amber 12% 底。**入口按角色过滤**（审核/成本/模型配置仅 admin 显示，见 Task 6 rbac）。
- [ ] **Step 3：build+test+lint 三绿；commit** —— `feat(web): file-based routing model + AppShell nav`。

**Verify：** `pnpm build`、`pnpm test`、`pnpm lint`。

---

### Task 5：API 类型 + apiFetch（内存 token + single-flight refresh-on-401）（TDD）

**做什么：** 写 `lib/types.ts`（上方线缆契约的 TS 镜像）；TDD `lib/apiClient.ts`。

- [ ] **Step 1：`lib/types.ts`** —— `ListEnvelope<T>={items,next_cursor}`、`Project`、`Event`、`Asset`、`AssetDetail={asset,versions}`、`Style`、`CatalogEntry`、`ModelConfig`、`Aggregate`、`ProjectAggregate`、`LedgerEntry`、`LoginResponse={access_token,expires_in}`、SSE 帧 payload 类型（`{seq,kind,todoId,payload}`）。**字段名严格照上方 Go `json:` tag**（lowerCamel，如 `contentType`/`blobKey`/`costMicros`/`parentAssetId`）。
- [ ] **Step 2：写失败测试 `apiClient.test.ts`：**（a）从内存 token 注入 `Authorization: Bearer`；（b）401 → 刷新一次 → 重试原请求（断言 refresh 带 `X-CSRF:1` + `credentials:"include"`，重试带新 token）；（c）并发 401 single-flight（refresh 只调一次）；（d）刷新失败 → 清 token + 抛 `AuthError`。
- [ ] **Step 3：跑测试确认 FAIL。**
- [ ] **Step 4：实现 `apiClient.ts`** —— 模块级 `accessToken` 变量（**仅内存，不进 localStorage，防 XSS**）+ `setAccessToken`/`getAccessToken`；`refresh()` POST `/api/auth/refresh`（`X-CSRF:1` + `credentials:include`，single-flight 共享 promise）；`apiFetch(path, init)` 注入 Bearer，401 → refresh 一次 → 重试一次；`apiJSON<T>` typed 便捷包装（非 2xx 抛错）。建 `test/helpers.ts`（`jsonResponse` + `installFetchRoutes`）。
- [ ] **Step 5：跑测试确认 PASS；build+test+lint 三绿；commit** —— `feat(web): typed apiFetch with in-memory token + single-flight refresh-on-401`。

**Verify：** `pnpm build`、`pnpm test`、`pnpm lint`。

---

### Task 6：Auth context + 登录视图 + RBAC 角色门禁（TDD）

**做什么：** TDD `app/auth.tsx`（login/logout/token 生命周期 + 从 token 取角色）、`app/rbac.ts`（视图级门禁判定），实现登录视图。

- [ ] **Step 1：角色来源决策。** access token 是 authz JWT，前端**不解析 JWT 取角色**（角色是 per-(org,scope) 的，由后端 `ResolveRole` 决定）。前端角色门禁策略：**乐观显示 + 后端强制**。`rbac.ts` 暴露一个轻量 `useRole(org)` —— 通过对一个 admin-only 探针（如 `GET /api/orgs/{org}/model-configs`）的成功/`403` 推断当前用户在该 org 是否 admin，缓存进 Query。**非 admin 时隐藏审核/成本/模型配置入口；直访路由 → `403` → 重定向 + 文案"需要管理员权限"**。（这是 UX，不是安全边界。）TDD：mock `403`/`200` 验证 `useRole` 推断与入口隐藏。
- [ ] **Step 2：TDD `auth.tsx`** —— `AuthProvider {isAuthenticated, login, logout}`；`login(email,password)` POST `/api/auth/login` body `{email,password}` + `credentials:include`，存 `access_token` 进 `setAccessToken`，翻 `isAuthenticated`；`logout` POST `/api/auth/logout`（`X-CSRF:1`）后清 token。测试：起始未认证 / 登录存 token 翻标志 / 凭据错抛错且仍未认证 / 登出清 token。
- [ ] **Step 3：实现 `auth.tsx`，把 `AuthProvider` 包进 `main.tsx`（在 query+router 之上）。**
- [ ] **Step 4：登录视图 `routes/login.tsx`** —— rhf+zod（email/password），导出无路由的 `LoginForm` 便于单测；loading（按钮 spinner）/ error（凭据错→"邮箱或密码错误，请重试"）/ success→跳项目列表。TDD：登录成功调 `onSuccess`；凭据错显错且不跳。
- [ ] **Step 5：build+test+lint 三绿；commit** —— `feat(web): auth context + login view + role-gating`。

**Verify：** `pnpm build`、`pnpm test`、`pnpm lint`。

---

### Task 7：SSE 流客户端（GET + Bearer over fetch-event-source）（TDD）

**做什么：** TDD `lib/sse.ts` 的 `streamRunEvents()` —— 用 `@microsoft/fetch-event-source` 开 `GET /api/projects/{id}/events/stream`，带 `Authorization: Bearer`，解析 9 种命名事件 + `message` 兜底帧 + `signal` 取消。**续传不靠 `Last-Event-ID`**（服务端 `sse.go` 不发 `id:`、每次连都从 `after=0` 全量回放）—— 客户端按 `seq` 去重（Task 8 reducer）消化重连后的重复回放帧。

- [ ] **Step 1：写失败测试 `sse.test.ts`** —— 注入式 fake client（同 kb M5b 范式：脚本化 `{id,event,data}` 帧序列喂 `onmessage`，末了 `onclose`）。断言：（a）按序回调每个事件、解析 `{seq,kind,todoId,payload}`；（b）请求是 GET + `Authorization: Bearer tok` 头；（c）`run_done` 触发 onDone；（d）未知 kind 走 `message` 兜底回调（原 kind 在 payload）。
- [ ] **Step 2：跑测试确认 FAIL。**
- [ ] **Step 3：实现 `sse.ts`** —— `streamRunEvents(projectId, accessToken, handlers, client=fetchEventSource, signal?)`；GET + Bearer 头 + `openWhenHidden:true`；`onmessage` switch `ev.event`（9 种命名 + default→`message`），`JSON.parse(ev.data)`；`onopen`/`onerror`/`onclose` 暴露给上层做 SseIndicator 连接态（connected/reconnecting/disconnected）。**注意原生 EventSource 不可用的原因已在 §SSE 决策记录。**
- [ ] **Step 4：跑测试确认 PASS；build+test+lint 三绿；commit** —— `feat(web): streamRunEvents SSE client (GET+Bearer) over fetch-event-source`。

**Verify：** `pnpm build`、`pnpm test`、`pnpm lint`。

---

### Task 8：制片轨道状态机 reducer（SSE 事件 → 节点态）（TDD）

**做什么：** TDD `lib/timeline.ts` —— 把事件流（回放 + 实时）归约成轨道全态，是本里程碑最核心的逻辑单元。

- [ ] **Step 1：写失败测试 `timeline.test.ts`** —— 覆盖 UI-spec §6.1 全部转换：
  - `planner_started` → S1 running、SlateBar 显示、徽标"生产中"。
  - `todo_ready ×N`（按 `payload.type`）→ 对应 stage `blocked→pending`。
  - `todo_started(type=script/storyboard)` → 该 stage running（agent 色环）。
  - `todo_finished(type=script)` → S2 done + 连接线着蓝；`todo_finished(type=storyboard)` → S3 done + 着紫，且从 payload 取 shots 数 N 初始化 S4 PipGroup。
  - `todo_started(type=asset)`（每 shot）→ 对应 pip running；`asset_generated` → 对应 pip done（asset 色）+ `done/N` 计数；全部 done → S4 done、S5 Review pending。
  - `todo_failed(type=asset)` → 对应 pip failed；同 todoId 再来 `todo_started` → pip 回 running（重试范式）。`todo_failed`（阶段级，重试耗尽）→ stage failed、后继维持 blocked。
  - `asset_submitted`（M4 异步提交）→ pip 维持 running（仅日志）；`asset_prescreened`（M3 预筛）→ 日志 + pip 不变（不改审核态，审核仍走 HITL）。
  - `run_done` → SlateBar 隐藏、徽标"待审核·N"、success toast 信号。
  - `message` 兜底事件 → 仅追加日志，不改节点态。
  - **幂等/回放**：按 `seq` 去重，重复 seq 不重复渲染；回放后续接实时不丢态。
- [ ] **Step 2：跑测试确认 FAIL。**
- [ ] **Step 3：实现 `timeline.ts`**（纯函数 reducer `(state, event) => state`，固定阶段语义 S1–S5；S4 的 N 来自 storyboard payload）。
- [ ] **Step 4：跑测试确认 PASS；build+test+lint 三绿；commit** —— `feat(web): production-timeline reducer (SSE event state machine)`。

**Verify：** `pnpm build`、`pnpm test`、`pnpm lint`。

---

### Task 9：项目列表 + 建项目视图

**做什么：** `features/projects/` —— 列表（卡片 + status Badge）、建项目表单（rhf+zod）、风格下拉取自 `GET /api/prompt-styles`。

- [ ] **Step 1：`api.ts`** —— `useProjects(org)`（`GET /api/orgs/{org}/projects` → `{items,next_cursor}`）、`useCreateProject(org)`（`POST` body `{name,brief,contentType,targetPlatform,style}`）、`usePromptStyles()`（`GET /api/prompt-styles` → `{styles}`）。
- [ ] **Step 2：`ProjectListPage.tsx`** —— 卡片网格 + status→Badge variant 映射；loading 骨架；empty（"还没有项目" + "用一句创意需求开始你的第一支作品" + CTA "新建项目"）；error（"项目加载失败" + "重试"）。`新建项目` 按钮 **editor+ 才显示**（viewer 隐藏）。点击进工作台。
- [ ] **Step 3：建项目表单（Dialog + rhf+zod）** —— 名称/创意 brief(textarea)/内容类型/目标平台/风格(下拉取 styles 的 `name`)。成功后失效列表 Query。
- [ ] **Step 4：render/smoke 测试**（列表渲染、empty 态、CTA 角色隐藏、表单提交调 mutation）。
- [ ] **Step 5：build+test+lint 三绿；commit** —— `feat(web): project list + create view`。

**Verify：** `pnpm build`、`pnpm test`、`pnpm lint`。

---

### Task 10：项目工作台 + 制片轨道 + SSE 实时 + 剧本/分镜视图

**做什么：** `features/workflow/` —— 三栏工作台（左 brief/KV/WarnStrip/EventLog，中制片轨道，右选中工件预览）、接 Task 7 SSE + Task 8 reducer，含剧本视图与分镜栅格。

- [ ] **Step 1：`api.ts`** —— `useProject(id)`、`useEvents(id, afterSeq)`（回放 `GET /events` → `{items}`）、`useRun(id)`（`POST /run`）、`useCancel(id)`（`POST /cancel`）、`useScript(id)`（`GET /script`，**裸 JSON 非 `{items}`**，zod 容错解析兜畸形）、`useShots(id)`（`GET /shots` → `{items}`）、`useProjectAssets(id,status)`（`GET /assets` → `{items}`）。
- [ ] **Step 2：建状态机相关自建组件** —— `TimelineStage`、`PipGroup`、`SlateBar`、`LineageTrail`（血缘留 Task 11 用）。各配 render smoke。
- [ ] **Step 3：`WorkbenchPage.tsx`** —— 进入时先 `GET /events` 回放重建轨道全态（喂 Task 8 reducer），再 `streamRunEvents` 续接实时；**完成态项目（status∈{completed,review,failed,canceled}）只回放不开流**。`SseIndicator` 显连接态。WarnStrip：项目/plan 的 `fallbackUsed=true` 时常驻（来自 `POST /run` 返回的 `fallbackUsed` 或项目详情，**非 SSE**）。`运行`/`取消`/`重新运行` 按钮 **editor+** 才显示；viewer 只读。
- [ ] **Step 4：右栏工件预览** —— M1 文本（script/shot 文本）；含 asset 的（M2+）显示缩略图。**缩略图来源（决策）：** 走 `GET /api/assets/{id}/content`（302→签名 URL）。该端点需 Bearer auth，`<img src>` 不带 token —— 故 SPA 内对 `/api/assets/{id}/content` 用 `apiFetch` 拿到 `302 Location`（`redirect:"manual"` 读 `Location` 头），把签名 URL（无需 auth，HMAC 在 query）塞 `<img src>`；签名过期（图 onError）则重拉一次刷新（UI-spec §11 默认决策 3）。
- [ ] **Step 5：剧本视图 `routes/.../script.tsx`** —— zod 容错解析 `{故事/对白/人物/场景}`；loading/empty（"剧本尚未生成"）/error（"剧本数据异常，请重新运行剧本阶段"）。
- [ ] **Step 6：分镜栅格 `routes/.../storyboard.tsx`** —— `GET /shots` 栅格（auto-fill minmax 150–170px），shot 编号 + 镜头描述 + prompt 摘要；M2+ 每格挂 asset 缩略图（同 Step 4 取图方式）。empty（"分镜尚未拆解"）。
- [ ] **Step 7：smoke 测试**（工作台用 mock SSE client 注入帧序列，断言轨道渲染随事件推进；剧本 empty/error；分镜栅格渲染）。
- [ ] **Step 8：build+test+lint 三绿；commit** —— `feat(web): workbench + production timeline (SSE live) + script/storyboard views`。

**Verify：** `pnpm build`、`pnpm test`、`pnpm lint`。

---

### Task 11：审核看板（HITL accept/reject/regenerate，admin 门禁，版本血缘）

**做什么：** `features/review/` —— 左过滤+AssetCard 网格，右 Sheet/Drawer（hero 签名图 + KV + PromptBox + LineageTrail + actions），键盘流 A/R/E + ←→。

- [ ] **Step 1：`api.ts`** —— `useReviewQueue(org)`（`GET /api/orgs/{org}/assets?status=pending_acceptance&type=image` → `{items,next_cursor}`）、`useAsset(id)`（`GET /api/assets/{id}` → `{asset,versions}`）、HITL mutations `useAccept/useReject/useRegenerate`（admin）。
- [ ] **Step 2：建 `AssetCard.tsx`**（default/hover/sel=amber 描边，图走 content 端点取图方式）+ `LineageTrail.tsx`（normal/cur=amber）+ `PromptBox.tsx`。
- [ ] **Step 3：`ReviewBoardPage.tsx`** —— 左过滤 chips（项目/风格/类型）+ AssetCard 网格；右 Drawer（`?asset=` typed search param 控制开合）= hero（content 签名图）+ KV（类型/Shot/Provider·Model/version）+ PromptBox + LineageTrail（versions）+ actions。
- [ ] **Step 4：HITL actions（Drawer 底，admin-only）** —— `✓ 采纳[A]`(green)→`accept` 乐观移出队列、失效 review+library；`✗ 退回[R]`(red)→`reject`（可撤销 sonner toast，5s 内"撤销"，UI-spec §11 默认决策 1，不弹模态）；`✎ 改 Prompt 重生成[E]`(ghost)→打开 PromptBox 编辑表单(rhf)→`regenerate` body `{prompt}`。**防重：对非 pending 资产操作→后端 `409`→toast"该资产已被处理（{状态}）"。**
- [ ] **Step 5：键盘（UI-spec §9）** —— A/R/E（仅 admin 生效）+ ←→（切上/下一个待审，所有角色可浏览）；输入框聚焦时禁用快捷键。`<kbd>` 标注。
- [ ] **Step 6：RBAC** —— HITL 三动作 admin-only，actions 区对非 admin **隐藏**（非禁用灰显）；viewer/editor 可浏览网格与 Drawer 只读。loading/empty（"没有待审资产"/"所有素材都处理完了"）/error。
- [ ] **Step 7：TDD 承载逻辑** —— 键盘 dispatch（admin vs 非 admin 行为分支、输入聚焦禁用）+ 409 防重 toast；视图其余 render smoke（含 admin 隐藏 actions）。
- [ ] **Step 8：build+test+lint 三绿；commit** —— `feat(web): HITL review board (accept/reject/regenerate, admin-gated, lineage)`。

**Verify：** `pnpm build`、`pnpm test`、`pnpm lint`。

---

### Task 12：资产库（过滤 + keyset 分页 + 版本血缘）

**做什么：** `features/library/` —— 左 FilterRail，右网格 + "加载更多"（keyset 游标累积）。

- [ ] **Step 1：`api.ts`** —— `useLibrary(org, filter)`（`GET /api/orgs/{org}/assets?project=&type=&status=&style=&tag=&limit=&cursor=` → `{items,next_cursor}`），用 TanStack `useInfiniteQuery` 按 `next_cursor` 累积。typed search params 持有过滤态。
- [ ] **Step 2：TDD keyset 累积**（多页 `next_cursor` 串接、空 cursor 停、过滤变更重置）。
- [ ] **Step 3：`LibraryPage.tsx`** —— 左 FilterRail（类型[图片✓ / **视频/音频 disabled 标"二期"**]、状态[accepted/pending/rejected]、风格[7 种]、项目下拉）+ 顶部 tag 搜索；右网格（status Badge + version vtag，图走 content 取图方式）+ "加载更多"。**视频/音频资产卡（若库返回 type=video/audio）用对应 `<video controls>`/`<audio controls>` 播放器**（生成是后端驱动，前端只播放，不新增后端）。
- [ ] **Step 4：状态** —— loading 网格骨架 / empty（"没有匹配的资产"/"调整筛选条件试试"）/ error。viewer+ 只读。
- [ ] **Step 5：render smoke + build+test+lint 三绿；commit** —— `feat(web): asset library (filters + keyset pagination + version lineage)`。

**Verify：** `pnpm build`、`pnpm test`、`pnpm lint`。

---

### Task 13：成本中心 + 模型配置（均 admin-only）

**做什么：** `features/cost/` —— 成本中心（StatCard + BarRow + DataTable + 时间范围）与模型配置（分组表 + 编辑表单）。

- [ ] **Step 1：`api.ts`** —— `useOrgCost(org,from,to)`（`GET /api/orgs/{org}/cost` → `Aggregate`）、`useOrgCostProjects(org,from,to)`（`/cost/projects` → `{items: ProjectAggregate[]}`）、`useGenerations(org,limit)`（`/generations` → `{items: LedgerEntry[]}`）、`useModelCatalog()`（`/model-catalog` → `{catalog}`）、`useModelConfigs(org)`（`GET /model-configs` → `{items}`）、`useCreateModelConfig(org)`（`POST` body `{kind,provider,model,enabled,isDefault,params}`）。
- [ ] **Step 2：`CostCenterPage.tsx`** —— 3 StatCard（本月成本=costMicros 换算/生成次数=generations/Token 用量=tokens）+ 按项目 BarRow（条色轮换 agent 色，金额 mono）+ DataTable（generations 明细：时间/项目/provider·model/类型/用量/金额，`td.num` mono 右对齐）+ 时间范围 dropdown（近 30 天 ▾，转 RFC3339 `from/to`）。loading/empty（"暂无成本数据"）/error。
- [ ] **Step 3：`ModelConfigPage.tsx`** —— 按 kind（chat/image，video/audio 标"二期"）分组配置表 + 编辑表单（provider/model 下拉取自 catalog、enabled/isDefault 开关、params JSON 编辑）。**表单绝不含 API key 字段，文案明示密钥服务端管理。** `ErrSecretParam`(400) → toast。loading/empty（"尚未配置模型"+"添加第一个模型"）/error/保存成功 toast。
- [ ] **Step 4：RBAC** —— 两视图整体 admin-only；非 admin 导航不显示入口，直访路由 → 重定向 + "需要管理员权限"。TDD 角色门禁分支 + 时间范围→query 参数转换；其余 render smoke。
- [ ] **Step 5：build+test+lint 三绿；commit** —— `feat(web): cost center + model configs (admin-only)`。

**Verify：** `pnpm build`、`pnpm test`、`pnpm lint`。

---

### Task 14：Prompt Builder（风格选择 + build 预览）

**做什么：** `features/prompt/` —— 复用 `GET /prompt-styles`（建项目/重生成共用）+ `POST /prompt/build` 实时预览拼装后的 prompt。

- [ ] **Step 1：`api.ts`** —— `usePromptStyles()`（若 Task 9 已建则复用）、`useBuildPrompt()`（`POST /api/prompt/build` body `{prompt,style}` → `{prompt}`）。
- [ ] **Step 2：`PromptBuilder.tsx`** —— prompt textarea + 风格 chip 选择（7 种 `name`）+ PromptBox 预览（mono）。可作为建项目表单/审核重生成表单的内嵌组件。
- [ ] **Step 3：render smoke（输入+选风格→调 build→显预览）+ build+test+lint 三绿；commit** —— `feat(web): prompt builder (styles + build preview)`。

**Verify：** `pnpm build`、`pnpm test`、`pnpm lint`。

---

### Task 15：README + 全量验证 + .gitignore + 文档化已知缺口

- [ ] **Step 1：`web/README.md`** —— 栈、`pnpm dev/build/test/lint`、dev proxy → `:8083`、auth 机制（内存 access token + httpOnly refresh + X-CSRF）、SSE 用 fetch-event-source 的原因、"真实浏览器 E2E 不在 M5 范围"。
- [ ] **Step 2：根仓 README 加 `web/` 小节链接**（若存在 studio README）。
- [ ] **Step 3：确认 `.gitignore` 含 `web/node_modules`、`web/dist`、`web/src/routeTree.gen.ts`。**
- [ ] **Step 4：全量验证** —— `pnpm build && pnpm test && pnpm lint` 全绿，记录输出。
- [ ] **Step 5：可选手动 smoke（不进 CI）** —— 起活 studiod（`HTTP_ADDR=:8083` + PG + JWT_SECRET），`pnpm dev`，登录→建项目→运行→看轨道随 SSE 推进。记为手动步骤。
- [ ] **Step 6：文档化已知缺口（见下方 §已知缺口），commit** —— `docs(web): README + M5 verification + deferred items`。
- [ ] **Step 7：close-out** —— 提 PR；merge 后在 studio 仓打 `v0.5.0`。

**Verify：** `pnpm build`、`pnpm test`、`pnpm lint`（全绿）。

---

## 已知缺口与延期项（执行 agent 须遵守：只标记、不静默改后端）

1. **`Asset.signedUrl` 不存在。** UI-spec §7.6 的 DTO 列了 `signedUrl`，但 `assets.Asset` 无此字段。可显示图一律走 `GET /api/assets/{id}/content`（302→签名 URL），SPA 用 `apiFetch` + `redirect:"manual"` 读 `Location`（见 Task 10 Step 4）。**无需后端改动。**
2. **`GET /script` 是裸 JSON，不是 `{items}` 信封。** 单独处理（zod 容错解析）。其余多数列表是 `{items}`；仅项目列表与资产库是 `{items,next_cursor}`。
3. **SSE 白名单 9 种 > UI-spec 列的 7 种。** 多出 `asset_submitted`（M4 异步提交）/`asset_prescreened`（M3 预筛）—— 状态机已纳入（Task 8）。
4. **"运行历史" 无独立路由/端点。** 复用工作台 `GET /events` 回放 + 完成态只回放不开流（UI-spec §11 决策 6）。**符合现有后端，无缺口。**
5. **登录请求体大小写。** authz `struct{Email,Password}` Go JSON 大小写不敏感，`{email,password}` 与 `{Email,Password}` 都能解；本计划用 `{email,password}`（与 authz 自带测试一致）。
6. **CORS/静态服务：** dev 走 Vite proxy（同源），无 CORS 问题。**生产由 studiod 直接静态服务 `web/dist` 的能力当前后端未提供** —— 若需生产同源部署，是一处后端缺口，**本计划只标记，留待后续里程碑**（M5 不加后端代码）；当前可由反向代理/独立静态托管承担。
7. **真实浏览器/Playwright E2E** —— sandbox 无浏览器，不在 M5 范围；验证 = build + typecheck + vitest + lint。
8. **视频/音频生成** —— 后端 M4 已支持异步生成，前端只**展示/播放**库里返回的 video/audio 资产（`<video>`/`<audio>`），生成触发不在 M5 前端范围。库的视频/音频**过滤项**按原型标"二期" disabled。
