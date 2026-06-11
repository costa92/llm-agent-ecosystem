# AI Studio 技术设计文档（Multi-Agent 内容生产平台）

- 日期：2026-06-10
- 状态：设计待评审（brainstorm 已完成，逐段获用户确认）
- 类型：新案例项目（准生产级，独立 sibling 仓 `github.com/costa92/llm-agent-studio`）
- 上游：[[2026-06-10-llm-agent-studio-prd]]（AI Studio PRD V1.0，产品需求）；本文是把 PRD 接地到生态真实能力的**技术设计**
- 取代：[[2026-06-09-llm-agent-studio-design]]（旧「工作流可视化编排器」设计，已废弃为引擎/技术参考）
- 相关：[[project_case-study-projects]]；共享鉴权 `llm-agent-authz`（与 llm-agent-kb 同库，scope_kind 不同）

---

## 1. 目标与定位

为生态提供第二个**面向终端用户的全栈案例项目**：基于 Multi-Agent 的**内容生产平台**——用户给一句创意需求，AI 自动规划并执行 `剧本 → 分镜 → 图片素材`（二期加视频/音频），人工审核采纳后入资产库。

与旧「编排器」的根本差异：用户**不画 DAG**，跑的是一条**固定语义的内容生产流水线**（Planner→Script→Storyboard→Asset→Review）；生成任务**分钟级、异步、人工审核可能隔小时**——因此运行模型是**DB 驱动的持久 job 状态机**，而非进程内 flow run。

### 成功标准（端到端可验证）
登录 → 建项目（填创意 brief：内容类型/目标平台/风格）→ 运行 → Planner 产出 todo 图 → worker 异步执行 ScriptAgent/StoryboardAgent 产出剧本+分镜（SSE 时间线实时上色）→ AssetAgent 经 PromptBuilder+ImageGenerator 生成图片落 BlobStore → 资产 `pending_acceptance` → admin 采纳/退回/编辑Prompt重生成（新版本血缘）→ 资产库按标签/风格/项目检索 → 成本账本记录每次 provider 调用用量。后端 `httptest`+真 Postgres+MinIO，前端测试随 UI agent 的 UI-SPEC 落地。

## 2. 已确定的范围决策

| 维度 | 决策 |
|---|---|
| 范围 | **一期 + 二期**（一期=文本管线+图片+审核+资产库+PromptBuilder；二期=视频/音频/LoRA/数字人/剪辑，靠新增 generator 适配器加性扩展） |
| 编排引擎 | **DB 驱动的 todo/job 状态机**（复用 kb `ingest_job` worker 范式：`FOR UPDATE SKIP LOCKED`+租约+退避重试）。**不用** flow IR 引擎（那是旧编排器的运行模型，不匹配异步长任务+隔时审核） |
| Planner | **真 LLM 动态规划**（`agents.PlanAndSolveAgent`/`orchestrate.Supervisor` → 结构化 todo 图）+ 类型白名单/依赖校验 + 畸形回落默认管线 |
| 生成层 | **可插拔 `MediaGenerator` 统一接缝**：一期 Image（包 `contract/llm.ImageGenerator`）；二期 Video/Audio 各为新实现，管线/审核/库零改 |
| 资产存储 | **可插拔 `BlobStore`**：dev=本地FS、prod=S3 兼容；含 `SignedURL(key,ttl)`，资产内容**走签名 URL 直连**（不代理字节） |
| HITL | **DB 状态机**（`pending_acceptance→accepted/rejected`，退回→重生成新版本），**审核权限限 admin** |
| 鉴权/租户 | 复用 `llm-agent-authz`，`scope_kind="org"`，**Project 是 org 拥有的资源**；角色 `admin`/`editor`/`viewer`(+`org_admin`)；团队 workspace 层放三期 |
| 前端 | **由 UI 设计 agent 负责**（`gsd-ui-researcher` 出 UI-SPEC 设计契约 → UI 实现阶段），消费本文 API 契约；技术栈/视觉/UX 由 UI agent 定，**不预设、不移植 console** |
| 可观测 | `llm-agent-otel`（`otelmodel.Wrap`/`otelagent.Wrap`）；成本=provider `Usage` 落用量账本；Langfuse 走 OTLP collector 导出 |
| 持久化 | Postgres（准生产）；dev 可降级（沿用 kb 经验） |

## 3. 生态能力盘点（已核对真实代码，决定可行性）

> 来源：Explore agent 实读生态源码。✅=可直接复用；⚠️=有约束；❌=生态缺失需自建。

- **✅ LLM 对话**（`llm-agent-contract/llm.ChatModel`）：`Generate/Stream/Info`；provider 经 `llm-agent-providers/{openai,anthropic,google,deepseek,kimi,minimax,...}` 的 `New(WithModel(...),WithAPIKey(...))` option 构造。能力按 `Info().Capabilities` 协商。
- **✅ 图像生成**（`llm-agent-contract/llm.ImageGenerator`）：`GenerateImage(ctx, ImageRequest) (ImageResponse, error)`；`ImageRequest{Prompt,N,Size,Quality,Format,Extra}`；返回 `GeneratedImage{Bytes/URL,MimeType,RevisedPrompt}`。实现：openai(dall-e/gpt-image)、google(imagen/gemini)、minimax(image-01)、volcengine(seedream)。**⚠️ 无 Flux/SDXL/Midjourney 适配器**（PRD 列了，须后续自建或裁掉）。
- **✅ Agent 框架**（`llm-agent` 核心 `agents`）：`SimpleAgent`/`ReActAgent`/`ReflectionAgent`/`PlanAndSolveAgent`/`FunctionCallAgent`，均实现 `Agent{Name,Run,RunStream}`，带 `Result.Trace`/`StepEvent`。
- **✅ 多 Agent 编排**（`llm-agent/orchestrate`）：`Pipeline`/`FanOutFanIn`/`Supervisor`/`StateGraph[S]` 等。Planner 可用 `PlanAndSolveAgent` 或 `Supervisor`。
- **⚠️ 结构化输出**（`llm.StructuredOutputs`）：接口已定义但**各 provider 尚未实现**。ScriptAgent/StoryboardAgent/Planner 需要 JSON 结构化输出 → **走 prompt 工程产 JSON + 容错解析**（或 `ToolCaller` 函数调用），不能依赖 provider 原生 structured output。（风险 R1）
- **✅ 可观测**（`llm-agent-otel`）：`otelmodel.Wrap(ChatModel)`、`otelagent.Wrap(Agent)`、`otelflow.Wrap`（本项目不用 flow）。Langfuse 无 SDK，靠 OTLP 导出。
- **✅ 鉴权**（`llm-agent-authz` v0.1.0）：`Authenticate`/`RequireScopeRole(scope_kind, minRole)` 中间件 + `/api/auth/*` + 角色合并代数；kb 已验证集成范式。
- **✅ worker 范式**（`llm-agent-kb/internal/ingest` + storage）：`ingest_job` 租约队列（`FOR UPDATE SKIP LOCKED`+DB-now() lease+退避+stuck-reclaim+幂等 dedup）——todo worker 直接照搬此范式。
- **❌ 视频生成**：生态无 `VideoGenerator`/任何 Runway/Kling/Veo 适配器 → 二期自建。
- **❌ 音频/TTS**：生态无 → 二期自建。
- **❌ 对象存储**：生态无 blob 抽象 → 自建 `BlobStore`（local FS + S3，外部 SDK minio-go/aws-sdk-go-v2）。
- **❌ 无可参照全栈前端**：`llm-agent-console` 未定型，明确**不**作参照。前端由 UI agent 全新设计。后端参照仅限已发版稳定仓（kb v0.5.0 的分层/worker/authz 接线、SSE 范式）。

## 4. 总体架构

单 Go 二进制 `studiod`（BFF+业务一体）+ React SPA（UI agent 设计）+ Postgres + BlobStore + otel collector。独立 sibling 仓 `github.com/costa92/llm-agent-studio`（自有 git + `.planning/`，从 umbrella gitignore；Go 命令需 `GOWORK=off`，见 [[project_console-gowork-off]]）。

```
Browser (React SPA, UI agent 设计): 项目工作台 · 流水线时间线(SSE) · 分镜栅格 · 审核看板 · 资产库 · 成本/模型(admin)
  │  HTTPS JWT, /api/* (REST + SSE)
  ▼
studiod (单 Go 二进制)
  httpapi(authz.Authenticate→org→RequireScopeRole) · project · planner(LLM→todo图)
  · todos(依赖图 store) · worker(FOR UPDATE SKIP LOCKED 池，复用 kb 范式)
  · agents{Script,Storyboard,Asset,Review} · prompt(风格库)
  · generate(MediaGenerator: Image一期 / Video·Audio二期) · assets(库+版本+标签) · review(HITL)
  · blob(local/S3 + SignedURL) · models(配置) · cost(用量账本) · obs(otel) · storage(pgx+迁移)
  │ pgx          │ HTTPS(provider API)        │ S3/FS          │ OTLP
  ▼              ▼                            ▼               ▼
Postgres     LLM/图像/视频/音频 provider      BlobStore        otel collector
```

进程/容器边界：单容器=studiod（embed 或 nginx 托管前端静态资源）；Postgres 独立容器；MinIO（S3 兼容）独立容器（dev 可改本地FS 卷）；provider 为外部 HTTPS 服务。

## 5. 仓库与模块布局

```
llm-agent-studio/
├── cmd/studiod/main.go       # DI 装配: DB、authz、worker 池、generator registry、blob、otel、http server
├── internal/
│   ├── (authz 接线)          # import llm-agent-authz: mount /api/auth/*, Authenticate/RequireScopeRole(scope_kind="org")
│   ├── project/              # 项目 CRUD + 状态机(派生自 todos) + store
│   ├── planner/              # LLM 规划: PlanAndSolve/Supervisor → 结构化 todo 图 + 类型白名单/依赖校验 + 畸形回落
│   ├── todos/                # todo 图 store(依赖边) + 租约队列列(claim/lease/retry) — todo 即 job
│   ├── worker/               # worker 池(复用 kb ingest 范式): claim ready todo → dispatch by type → agent → 写工件 → 发 run_event
│   ├── agents/               # ScriptAgent/StoryboardAgent/AssetAgent/ReviewAgent(可选预审) — 每个执行一种 todo 类型
│   ├── prompt/               # PromptBuilder + 风格库(日漫/吉卜力/皮克斯/迪士尼/写实/赛博朋克/国风)
│   ├── generate/             # MediaGenerator 接缝 + registry; image/ 适配 contract/llm.ImageGenerator; video//audio/ 二期
│   ├── assets/               # 资产库: 元数据 store + 版本血缘 + 标签/风格/项目检索
│   ├── blob/                 # BlobStore 接口 + localfs/ + s3/(minio-go) + SignedURL
│   ├── review/               # HITL: accept/reject/regenerate(派生重生成 todo), admin-only
│   ├── models/               # model_configs CRUD + model-catalog
│   ├── cost/                 # generations 用量账本 + 聚合查询
│   ├── httpapi/              # mux、中间件(auth→org→rbac)、handlers、SSE writer
│   ├── storage/              # pgx 连接、迁移
│   └── obs/                  # otel TracerProvider + otelmodel/otelagent Wrap 装配
└── web/                      # React SPA — 由 UI agent 出 UI-SPEC 后实现(全新构建)
```

**单一职责**：`planner` 只产计划；`worker` 只调度；`agents` 各司一阶段；`generate` 是唯一对接外部生成 API 处；`blob` 唯一管字节；`assets` 唯一管资产元数据+检索+版本；`httpapi` 只编排+鉴权+SSE。

## 6. 数据模型（Postgres，studio 拥有）

> `auth_*`（user/org/membership/session）由 `llm-agent-authz` 拥有，studio 注入 `scope_kind="org"`，不自建。

**项目与规划**
- `projects(id, org_id, name, description, content_type, target_platform, style, status, created_by, created_at, updated_at)` — `status ∈ {draft,planning,running,review,completed,failed,canceled}`；`org_id` 弱引用 `auth_org`
- `plans(id, project_id, status, raw_plan_json, valid, fallback_used, created_at)` — Planner 一次规划产物（原始 LLM 计划 + 校验结果 + 是否回落）

**todo 图 = 作业队列（合并）**
- `todos(id, project_id, plan_id, type, status, agent, skill, depends_on TEXT[], input_json JSONB, output_ref, error, attempts, locked_by, locked_until, next_run_at, updated_at, created_at)`
  - `type ∈ {script,storyboard,asset,review}`；`status ∈ {pending,ready,running,done,failed,blocked,canceled}`
  - `depends_on`=前驱 todo id；worker 只领「依赖全 done」的 ready todo
  - 直接带租约列（`FOR UPDATE SKIP LOCKED`+lease+退避），不另起 job 表
  - 索引：`(project_id)`、claim 索引 `(status,next_run_at)`

**工件树**
- `scripts(id, project_id, todo_id, content_json JSONB, version, created_at)`
- `shots(id, project_id, script_id, todo_id, shot_no, camera, scene, action, prompt, duration, ordering, created_at)`
- `assets(id, project_id, shot_id NULL, todo_id, type, blob_key, url, prompt, style, provider, model, status, version, parent_asset_id NULL, tags TEXT[], created_at)`
  - `type ∈ {image,video,audio}`；`status ∈ {generating,pending_acceptance,accepted,rejected,failed}`
  - 重生成=新行（`parent_asset_id` 血缘 + `version+1`，不覆盖）；`tags` GIN 索引支持检索
  - 索引：`(project_id)`、库检索 `(org_id via project, ...)`、`tags` GIN

**横切**
- `generations(id, project_id, asset_id NULL, todo_id, kind, provider, model, prompt, tokens, image_count, video_seconds, cost_micros, latency_ms, created_at)` — 用量/成本账本，每次 provider 调用一行
- `model_configs(id, org_id, kind, provider, model, enabled, is_default, params_json, created_at)` — 模型管理；**API key 不在此**（服务端密钥）
- `run_events(project_id, seq BIGSERIAL, kind, todo_id, payload JSONB, ts)` — 流水线进度，SSE 实时 + 历史回放；`(project_id, seq)` 索引

## 7. Agent 层 + 生成抽象 + 执行流

### 7.1 Agent（每个执行一种 todo 类型）
- **Planner**（`planner/`）：`PlanAndSolveAgent`/`Supervisor` 产结构化 todo 图。**校验**：类型 ∈ 白名单、依赖无环、至少含 script。畸形/解析失败 → **回落默认管线**（script→storyboard→每 shot 一 asset），`plans.fallback_used=true`。
- **ScriptAgent**：项目 brief → `ChatModel` → `scripts`（故事/对白/人物/场景，JSON via prompt + 容错解析）
- **StoryboardAgent**：script → `ChatModel` → `shots`（camera/scene/action/prompt/duration）
- **AssetAgent**：shot →（PromptBuilder 注入风格）→ `MediaGenerator.Generate` → `BlobStore.Put` → `assets`(`pending_acceptance`) + 写 `generations`
- **ReviewAgent**（可选，M3）：LLM 审查（敏感/版权/违规）打 flag；**人工采纳是硬门禁**，自动预审仅辅助

### 7.2 生成抽象（二期加性扩展的关键）
```go
// 所有素材生成的统一接缝。AssetAgent 不区分 image/video/audio。
type MediaGenerator interface {
    Kind() string                                // "image" | "video" | "audio"
    Generate(ctx context.Context, req GenRequest) (GenResult, error)
}
// GenResult 带 Bytes 或外链 URL + Usage（落 generations 账本）
```
- 一期：`image/` 适配 `contract/llm.ImageGenerator`（openai/google/minimax/volcengine）
- 二期：`video/`(Runway/Kling/Veo)、`audio/`(TTS) 各为新 `MediaGenerator` 实现 + 新 asset type
- `registry`：按 `model_configs` 的 provider+model 解析到具体 generator
- **异步长任务**：视频「提交→轮询」。适配器内部封装轮询（worker 租约可续约）；或返回「外部 job 待轮询」让 todo 带 `next_run_at` 退避重排

### 7.3 worker 执行流（复用 kb ingest 范式）
1. `POST /run` → Planner 写 `plans`+`todos`（script=ready，余 blocked）
2. worker 池领 ready todo（`SKIP LOCKED`+lease）→ 按 type 派 Agent
3. 成功：写工件行 + todo `done` + 发 `run_event` → 依赖满足者转 ready
4. 失败：退避重试；耗尽 → `failed` + 阻断后继
5. 项目状态派生：`planning→running→review`(资产待审)→`completed`(全采纳)
6. **取消/超时/卡死**：`cancel` 置 canceled；lease 过期 stuck-reclaim；项目级 `ctx` 超时

### 7.4 HITL（DB 状态机）
- 资产落 `pending_acceptance` → **admin** 操作：
  - **采纳** → `accepted`
  - **退回** → `rejected`
  - **编辑Prompt重生成** → 派生新 asset-type todo（带编辑后 prompt，`parent_asset_id` 血缘）→ worker 再跑
- 全程异步持久、扛重启——选 DB 状态机而非进程内 checkpoint 的核心理由
- **防重**：accept/reject 对非 `pending_acceptance` 资产 → 409

## 8. 鉴权 / RBAC

- JWT/登录/refresh/argon2id 全由 `llm-agent-authz` 提供（studio 不自建）。
- org 上下文：`/api/orgs/{org}/...` 套 `Authenticate → RequireScopeRole("org", minRole)`；跨 org→403。
- 校验点：`viewer`=读项目/工件/资产/库；`editor`=+建/编辑/删项目、触发 run；`admin`=+成员管理、**HITL 采纳/退回/重生成**、模型配置、成本中心。
- 归属隔离：project/asset/todo 均经 project→org 归属，**所有按 id 直查强制校验 org 归属**（仅凭 id 不可跨 org 读）。
- 下游凭据隔离：provider key / S3 凭据只存服务端配置，永不下发浏览器（BFF）。资产经**短 TTL 签名 URL** 直连，不暴露 BlobStore 凭据。

## 9. HTTP API 契约 + SSE

> 全挂 `/api`；列表 cursor 分页。

- **鉴权**：`POST /api/auth/{login,refresh,logout,logout-all}`
- **组织/成员**：`POST /api/orgs`（创建者 org_admin）、`GET/POST/DELETE /api/orgs/{org}/memberships`（admin）
- **项目**：`GET/POST /api/orgs/{org}/projects`（editor+ 建）、`GET/PUT/DELETE /api/projects/{id}`、`POST /api/projects/{id}/run`（editor+，重跑=重规划）、`POST /api/projects/{id}/cancel`、`GET /api/projects/{id}/events`（分页）、`GET /api/projects/{id}/events/stream`（**SSE 时间线**）
- **工件（viewer+）**：`GET /api/projects/{id}/{todos,script,shots,assets}`（assets 按 status/type/shot 过滤）
- **HITL（admin）**：`POST /api/assets/{id}/{accept,reject,regenerate}`（regenerate body=编辑 prompt/params）
- **资产库**：`GET /api/orgs/{org}/assets`（标签/风格/项目/类型过滤 + keyset 分页）、`GET /api/assets/{id}`（含版本血缘）；资产内容经响应里的短 TTL `signedUrl` 直连（`GET /api/assets/{id}/content` 302 到签名 URL）
- **Prompt Builder**：`GET /api/prompt-styles`、`POST /api/prompt/build`（预览增强 prompt）
- **模型管理（admin）**：`GET/POST /api/orgs/{org}/model-configs`、`GET /api/model-catalog`
- **成本中心（admin）**：`GET /api/orgs/{org}/cost`、`GET /api/projects/{id}/cost`（按项目/时间聚合）

**SSE 时间线事件**（复刻 kb progress SSE）：`planner_started`→`todo_ready`→`todo_started`→`todo_finished`→`asset_generated`(待审)→`todo_failed`→`run_done`；每事件带 `todo_id/type/payload`。`run_events` 同时落库支持断线重连/历史回放。

## 10. 资产存储（BlobStore）

```go
type BlobStore interface {
    Put(ctx, key string, r io.Reader, contentType string) error
    SignedURL(ctx, key string, ttl time.Duration) (string, error)
    Delete(ctx, key string) error
}
```
- `s3/`：minio-go / aws-sdk-go-v2；`SignedURL`=presigned GET。
- `localfs/`（dev）：字节落卷；`SignedURL`=后端签发带 `sig+exp` 的回源 URL（`GET /api/blob/{key}?sig=&exp=`，独立校验 handler），无凭据外泄。
- provider 直返外链的资产（volcengine/minimax）：**默认拉回落 BlobStore**（统一寻址+版本+生命周期）；可配只存 URL。

## 11. 前端（交给 UI 设计 agent）

前端**不在本 spec 详设**：由 UI 设计 agent（`gsd-ui-researcher`）出独立 **UI-SPEC 设计契约**（视图/组件/交互/视觉/技术栈），再进 UI 实现阶段。本 spec 只提供 API 契约（§9）作为 UI 的消费面。

**权威视觉/UX 参考（UI agent 必须参考）**：`docs/superpowers/specs/ai-studio-ui-prototype.html`（高保真静态原型 + 模拟 SSE 推进）。UI agent 须同时参考该原型与本 spec，沿用其设计语言与信息架构，不得另起炉灶：
- **4 屏 + 左侧图标导航**：项目工作台 / 审核看板 / 资产库 / 成本中心。
- **设计语言**：深色（`--bg-base:#17191E`）+ amber 主色（`#E8A33D`）；**按 agent 配色**（script 蓝 `#5C9BD6` / storyboard 紫 `#9C7BDA` / asset 琥珀 `#E8A33D` / review 绿 `#4FB286`）；字体 Space Grotesk（标题）/ JetBrains Mono（id/数据）/ Noto Sans SC（正文）；「**制片/场记板**」电影制作隐喻（运行条 slate-bar 动画 + 制片轨道 timeline）。含 `prefers-reduced-motion` 降级。
- **项目工作台**三栏：左=创意 Brief + 项目信息 + `fallback_used` 告警条 + 事件日志；中=**制片轨道**（S1 Planner→S2 Script→S3 Storyboard→S4 Asset×N shots 并行 pip 组→S5 Review，SSE 实时上色、节点态 done/running/blocked/failed）；右=选中节点工件预览（缩略图 + provider/model + 耗时/attempts + prompt）。
- **审核看板**：资产网格 + 详情抽屉（hero + prompt + **版本血缘** v1退回→v2当前 + 采纳`A`/退回`R`/改Prompt重生成`E` + 键盘快捷键 + ←→ 切换）。
- **资产库**：左侧过滤轨（类型/状态/风格/项目，视频标「二期」disabled）+ 资产网格（状态徽标 + 版本号）+ 加载更多。
- **成本中心**：统计卡（本月成本/生成次数/Token 用量）+ 按项目成本条 + 用量明细表（时间/项目/provider·model/类型/用量/金额）。

需 UI 覆盖的视图：登录、项目列表/建项目、上述 4 屏、剧本视图、分镜栅格、模型配置(admin)。约束：不移植 `llm-agent-console`（未定型）。

## 12. 准生产级横切

- 鉴权：JWT + org RBAC（§8）；下游凭据服务端隔离；资产签名 URL。
- 运行隔离/配额：每 run/todo `ctx` 超时+cancel；worker 池并发上限；org 级并发 run 上限 + token bucket 速率限制（复用 kb limits 范式）；生成调用配额（防成本失控）。
- 安全：任何**用户输入 URL**（如外链素材引用）经 SSRF 防护（复刻 kb `internal/fetch` 的 resolve-validate-dial 反 rebinding）；上传校验（若允许参考图上传）；provider/S3 密钥仅服务端。
- 可观测：`obs` 统一 otel；`otelmodel.Wrap` 包所有 ChatModel/ImageGenerator 调用、`otelagent.Wrap` 包 Agent；project_id/todo_id 作 span 属性；成本账本双写（span + `generations` 表）。
- E2E：①auth+RBAC 越权矩阵；②项目 CRUD+run 触发；③文本管线 run→SSE 时间线（M1）；④图片管线→pending_acceptance→accept/reject/regenerate 版本血缘→库检索（M2）；⑤Planner 畸形回落默认管线；⑥成本账本累计（M3）；⑦generator 接缝（mock generator 注入，免真调外部 API）。
- docker-compose：`studiod` + `postgres` + `minio` + `otel-collector`(+ 可选 grafana/tempo)；dev 可去 minio 改本地FS 卷。

## 13. 风险与未决点

1. **R1 结构化输出未实现（中，影响 M1）**：provider 无原生 structured output → Script/Storyboard/Planner 走 prompt 工程产 JSON + 容错解析（或 `ToolCaller`）。须健壮解析 + 重试 + schema 校验。E2E 必含畸形 JSON 回落。
2. **R2 LLM 规划不稳定（中）**：Planner 输出可能漏阶段/造非法类型 → §7.1 白名单校验 + 默认管线回落兜底。
3. **R3 图像 provider 不全（低-中）**：生态仅 openai/google/minimax/volcengine，**无 Flux/SDXL/Midjourney**。MVP 用现有四家；其余按需自建适配器或从 PRD 裁掉。
4. **R4 异步长任务（中，影响 M4）**：视频生成分钟级+轮询。worker 租约续约 / todo 退避重排两套机制；防长任务占满 worker 池（独立池或并发配额）。
5. **R5 二期生成全缺失（高，M4 范围）**：视频/音频/数字人/剪辑生态零支持，全是外部 SaaS/ffmpeg 集成。本 spec 只保证 `MediaGenerator` 接缝可加性扩展；二期落地另写 spec + build-vs-buy。
6. **R6 成本失控（中）**：图片/视频生成真实计费。须 org 级生成配额 + 速率限制 + 成本账本实时累计 + 超额熔断。
7. **R7 内容安全/合规（中）**：生成内容可能违规/侵权。ReviewAgent 自动预审(M3) + 人工硬门禁；敏感内容审计日志。
8. **R8 BlobStore 生命周期（低）**：被拒/孤儿资产清理；版本无限增长 → 保留策略 + 后台清扫（类比 kb checkpoint TTL）。

## 14. 关键参考文件（实现期）

- `llm-agent-kb/internal/ingest/`（v0.2.0，**worker 租约队列范式**：claim/lease/retry/stuck-reclaim/幂等——todo worker 照搬）
- `llm-agent-kb/internal/httpapi/` + `cmd/kbd/`（authz 接线、SSE progress、分层、E2E 范式，**最近的稳定全栈后端参照**）
- `llm-agent-contract/llm/image.go`（`ImageGenerator` 契约）+ `llm-agent-providers/{openai,google,minimax,volcengine}/image.go`（实现）
- `llm-agent/agents/{plan_solve.go,react.go}` + `llm-agent/orchestrate/`（Planner 候选）
- `llm-agent-otel/{otelmodel,otelagent}`（可观测 Wrap）
- `llm-agent-authz`（v0.1.0，鉴权库）

## 15. 里程碑

> 后端为主线；前端由 UI agent 出 UI-SPEC + 实现阶段，按 M1/M2/M3 分批跟进。

- **M1 — 骨架 + 文本管线**：authz 接线 + org/project CRUD + storage/迁移 + Planner(LLM→todo图+校验/回落) + todos store + worker 池 + ScriptAgent + StoryboardAgent（纯文本）+ run SSE 时间线 + 项目状态机 + otel 基线。E2E：brief→run→产出 script+shots、畸形计划回落。
- **M2 — 图片生成 + HITL + 资产库（= PRD 一期完成线）**：BlobStore(local+S3+SignedURL) + MediaGenerator/ImageGenerator + PromptBuilder + 风格库 + AssetAgent + HITL accept/reject/regenerate(admin) + 资产版本血缘 + 资产库检索 + model_configs + generations 账本。E2E：图片管线→待审→采纳/退回/重生成→库检索。
- **M3 — 准生产横切**：成本中心聚合 + 模型管理面 + 完整可观测 + 限流/配额/并发上限 + ReviewAgent 自动预审 + docker-compose(postgres+minio+otel) + 安全加固(SSRF/密钥审计) + E2E 加固。
- **M4 — 二期：视频/音频生成**（落地前另写二期 spec + build-vs-buy）：VideoGenerator(Runway/Kling/Veo) + Audio/TTS + 新 asset type + 异步轮询 + 配音 + 自动剪辑(ffmpeg/外部) + 图片 LoRA；数字人评估后或拆独立里程碑。
