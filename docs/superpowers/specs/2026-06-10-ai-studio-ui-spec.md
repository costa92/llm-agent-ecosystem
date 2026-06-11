---
phase: ai-studio-frontend
slug: ai-studio-ui
status: draft
shadcn_initialized: false
preset: radix-nova (kb 生态先例, cssVariables, baseColor=neutral, icon=lucide)
created: 2026-06-10
---

# AI Studio — UI 设计契约（UI-SPEC）

> Multi-Agent 内容生产平台前端的视觉与交互契约。由 gsd-ui-researcher 出，gsd-ui-checker 校验。
>
> **权威输入（设计语言以此为准，不得另起炉灶）：**
> - 高保真原型：`docs/superpowers/specs/ai-studio-ui-prototype.html`（设计语言的真相源）
> - 后端契约：`docs/superpowers/specs/2026-06-10-ai-studio-design.md`（§9 API/SSE · §6 数据模型 · §8 RBAC · §10 资产签名 URL · §11 前端视图清单 · §15 里程碑）
>
> **本契约的职责**：把原型的设计语言**逐字形式化**为可实现的工程契约 —— 不重新设计。所有 token 从原型 `:root` 提取。

---

## 0. 范围与约束

- **全新 greenfield 前端**，落地于 `llm-agent-studio/web/`（仓内当前无 `web/`）。
- **不参照、不移植 `llm-agent-console`**（未定型）。唯一可参照的稳定生态先例是 `llm-agent-kb/web/`（仅借其技术栈接线，不借视觉）。
- **不画 DAG canvas**：本平台跑的是固定语义流水线（Planner→Script→Storyboard→Asset→Review），UI 是一条**实时制片轨道 + 资产网格**，不是节点编排画布。
- Go 命令需 `GOWORK=off`（sibling 仓从 umbrella go.work 排除）；前端构建独立，不受此影响。

---

## 1. Design System

| Property | Value |
|----------|-------|
| Tool | shadcn（UI 实现阶段 `npx shadcn init` 初始化；本契约定 token，初始化随实现落地） |
| Preset | `radix-nova`（沿用 kb 先例：`cssVariables:true`、`baseColor:neutral`、`rsc:false`、`tsx:true`），**但 CSS 变量全部覆写为原型暗色 token（见 §4）** |
| Component library | radix-ui（shadcn 默认底座） |
| Icon library | lucide-react |
| Font | Space Grotesk（display/标题）· JetBrains Mono（id/数据/数字）· Noto Sans SC（正文，默认 family） |

字体加载：`@fontsource-variable`（Space Grotesk + JetBrains Mono），Noto Sans SC 经 Google Fonts `display=swap`（原型用法）。CSS 变量 `--disp:"Space Grotesk","Noto Sans SC",sans-serif`、`--mono:"JetBrains Mono",monospace`。

> 主题为**纯暗色单主题**（原型无浅色态）。`next-themes` 可装但锁定 dark；不交付浅色变体。

---

## 2. 技术栈推荐（grounded recommendation）

| 层 | 选型 | 理由（基于原型需求 + kb 生态先例） |
|----|------|------|
| 框架 | **React 19 + TypeScript** | 生态先例（kb `web/`）；并发渲染利好 SSE 高频局部更新 |
| 构建 | **Vite 8 + `@tailwindcss/vite`** | 先例一致；Tailwind v4 原生 vite 插件，CSS-first token 配置 |
| 样式 | **Tailwind v4（CSS 变量模式）** | 原型已是 CSS 自定义属性体系，v4 的 `@theme` 直接吃 §4 token，零摩擦 |
| 组件 | **shadcn / radix-ui** | 可访问底座（drawer/dialog/checkbox/dropdown），徽标/按钮/卡片用 cva 自建以贴合原型语言 |
| 路由 | **TanStack Router**（file/code-based + 类型安全 search params） | 审核看板的 `?asset=`、库的过滤态需类型化 URL state；先例一致 |
| 数据 | **TanStack Query** | 缓存/失效/乐观更新（HITL 采纳即失效审核队列 + 资产库）；先例一致 |
| **SSE** | **`@microsoft/fetch-event-source`** | **关键**：原生 `EventSource` 不能带 `Authorization` header（JWT）；fetch-event-source 支持带 header 的 SSE + 自动重连 + `Last-Event-ID`，正好对接 §9 的 `GET /events/stream` + `run_events.seq` 回放。先例已用此库 |
| 表单/校验 | **react-hook-form + zod**（`@hookform/resolvers`） | 建项目表单、改 prompt 重生成表单、模型配置；DTO 用 zod schema 兜后端容错 JSON（§13 R1） |
| Toast | **sonner** | run_done / todo_failed / 采纳成功的非阻塞反馈 |
| 样式工具 | clsx + tailwind-merge + class-variance-authority + tw-animate-css | 先例一致；cva 驱动徽标/按钮 variant |

> 与 kb 的差异点（本项目新增）：**实时制片轨道**（SSE→节点态机）+ **资产网格/抽屉 HITL**（键盘流）。这两者不需要 DAG 库（react-flow 等），用 CSS grid + 受控状态即可（原型已证明）。明确**不引入** react-flow / d3 / 任何画布库。

---

## 3. 组件清单（Component Inventory）

> 全部 variant 从原型 class 提取。cva 实现，token 见 §4。

| 组件 | 原型来源 | Variants / 状态 | 备注 |
|------|---------|----------------|------|
| `Badge`（状态徽标） | `.badge` + dot | `running`(amber,脉冲dot) / `done`(green) / `pending`(amber) / `rejected`(danger) | dot 6px；running 的 dot `pulse 1.4s`（reduced-motion 停） |
| `Button` | `.btn` | `amber`(主CTA,#1a1408字) / `ghost`(描边) / `green`(采纳) / `red`(退回) | 7px×16px，radius 8px，含 `<kbd>` 快捷键槽位 |
| `Kbd` | `kbd` | — | mono 10px，raised 背景，键盘提示 |
| `Chip / ChipSelect`（过滤/下拉触发） | `.chip-sel` | default / active | 也作搜索 input 外观 |
| `TimelineStage`（制片轨道节点） | `.stage`+`.node` | 节点态：`done`/`running`(虚线 ring 旋转)/`pending`/`blocked`/`failed`；连接线 `.linked` 着 agent 色 | 见 §6 状态机 |
| `StageChip`（节点态小标签） | `.tchip` | `t-done`/`t-run`(amber条纹)/`t-pend`/`t-block`(虚线)/`t-fail` | |
| `PipGroup`（并行 asset pip 组） | `.fan`+`.pip` | pip: idle/`running`(条纹)/`done`(asset色填充)/`failed` | N=shot 数；带 `done/N 完成` 计数 |
| `SlateBar`（场记板运行条） | `.slate-bar` | 显示=运行中 / 隐藏=结束 | amber 斜条纹 `slide 1s`（reduced-motion 停，降级为静态条） |
| `WarnStrip`（回落告警） | `.warn-strip` | — | `fallback_used` 时显示 |
| `EventLog`（事件日志） | `.log-line` | append-only | mono，时间戳 em + 事件名 |
| `AssetCard` | `.acard` | default / `hover`(上移2px) / `sel`(amber描边) | 图 + cap（标题/状态徽标 + `vtag` 版本号） |
| `Drawer`（审核详情） | `.drawer` | open（受 `?asset=` 控制） | hero + kv + prompt-box + 版本血缘 + actions |
| `LineageTrail`（版本血缘） | `.lineage`+`.lin-node` | node: normal / `cur`(amber) | `v1 已退回 → v2 当前` |
| `PromptBox` | `.prompt-box` | — | mono 11px，展示/可编辑（重生成） |
| `KVRow`（键值行） | `.kv` / `.meta-row` | — | 工件元数据 |
| `BriefCard` | `.brief-card` | — | 创意 brief 展示 |
| `StatCard` | `.stat` | — | 成本中心：lab + num（display 30px，small 单位） |
| `BarRow`（成本条） | `.bar-row` | — | 130px 标签 / 进度条 / mono 金额；条色按项目轮换 agent 色 |
| `DataTable` | `table` | — | 成本明细；`td.num` mono 右对齐 |
| `FilterRail` | `.fil-rail`+`.fcheck` | checkbox(accent amber) / `disabled`(二期) | 资产库左轨 |
| `SseIndicator`（连接指示） | `.sse-dot` | connected(green脉冲) / reconnecting(amber) / disconnected(text-3) | 见 §6.3 |
| `IconRail`（左导航） | `.rail`+`.nav-btn` | `on`(amber 12%底) / hover | logo + 项目/审核/资产/成本 + avatar |
| `PageHead` | `.page-head` | — | crumb + title + badge + toolbar-right；底部可挂 SlateBar |

---

## 4. Color（从原型 `:root` 逐字提取）

> 暗色 + amber 主色 + **per-agent 配色**。60/30/10 映射如下。

| Role | Value | Usage |
|------|-------|-------|
| Dominant (60%) | `--bg-base #17191E` | 页面底 / body |
| 表面 surface | `--bg-surface #1F232A` | 卡片 / 导航轨 / 抽屉 |
| 抬升 raised | `--bg-raised #272C34` | hover 态 / kbd / chip |
| Secondary (30%) | `--line #343A44` | 分隔线 / 描边 / 默认节点环 |
| Accent (10%) | `--amber #E8A33D` | **见下方保留清单** |
| Destructive | `--danger #E05F5B` | 退回/失败/拒绝 |

文本：`--text-1 #EDEEF0`（主）/ `--text-2 #9AA1AC`（次）/ `--text-3 #666D78`（弱/标签）。

**Per-agent 语义色（数据可视化专用，非通用强调）：**

| 变量 | 值 | 绑定 agent / 用途 |
|------|----|------|
| `--script` | `#5C9BD6` 蓝 | ScriptAgent / S2 节点与连接线 |
| `--board` | `#9C7BDA` 紫 | StoryboardAgent / S3 节点 |
| `--asset` | `#E8A33D` 琥珀（=amber） | AssetAgent / S4 / pip 填充 |
| `--review` | `#4FB286` 绿 | ReviewAgent / S5 / done 态 / SSE 已连接 |

**Accent（amber）保留给（显式清单，绝不"所有交互元素"）：**
- 主 CTA 按钮（`btn-amber`：运行/重新运行/建项目）
- 当前选中态：导航 `on`、AssetCard `sel` 描边、血缘 `cur` 节点
- 运行中信号：SlateBar、running 徽标 dot、running 节点环、pip running 条纹
- `focus-visible` outline（`2px solid var(--amber)`）
- `pending_acceptance` 状态文字 + 回落告警条
- AssetAgent/asset 类工件的语义色（恰为 amber）

> 对比度说明见 §9。`--text-3 #666D78` 仅用于非正文（标签/时间戳/装饰），不承载需阅读的关键信息。

---

## 5. Spacing / Typography / Radii

### Spacing（原型以 4 基数为主，登记如下）

| Token | Value | Usage |
|-------|-------|-------|
| xs | 4px | icon gap、徽标内距 |
| sm | 8px | 紧凑间距、pip 间距(7≈8)、按钮间距 |
| md | 16px | 默认元素间距、轨道 stage gap |
| lg | 24px | 列内边距(18≈) / page-head 横向(24) |
| xl | 32px | 布局大间距 |
| 2xl | 48px | 主区块分隔 |
| 3xl | 64px | 左导航轨宽（64px 固定） |

**Exceptions（原型实测，登记为允许偏差）：**
- 导航按钮 `nav-btn` 44×44px（触控目标，**保留 ≥44px**）
- pip 14×14px、node 28×28px（数据可视化固定尺寸，不走 spacing scale）
- page-head 纵向 16px、列 padding 18px（原型值，归入 md 区间，实现取最近 token 或保留原值，二选一须全局一致）

### Typography（暗色界面，3 档尺寸 + 2 字重为主）

| Role | Size | Weight | Line Height | Family |
|------|------|--------|-------------|--------|
| Display（统计数字） | 30px | 700 | 1.2 | Space Grotesk |
| Heading（页标题） | 17px | 600 | 1.2 | Noto Sans SC |
| Body（正文/控件） | 13px | 400 | 1.55 | Noto Sans SC |
| Label/Meta（标签/弱文本） | 11px | 500/600 | 1.2 | Noto Sans SC / 区段标题 letter-spacing .08em |
| Data/Mono（id·数字·prompt） | 10–11px | 400/600 | 1.6 | JetBrains Mono |

> 字重收敛为 **400（regular）+ 600（semibold）** 两档主用；700 仅用于 display 数字与 logo（装饰例外）。正文基线 13px/1.55 为原型 body 值，**全站正文锁定 13px**（不在 14–16 间游移）。

### Radii

| Token | Value | Usage |
|-------|-------|-------|
| sm | 4px | kbd、pip、lin-node |
| md | 8px | 按钮、chip、warn/log 容器 |
| lg | 10–12px | 卡片、stat、drawer hero、缩略图 |
| full | 999px | 徽标 pill、dot、avatar |

---

## 6. 实时制片轨道契约（核心）

### 6.1 SSE 事件 → 节点状态机（§9 事件名 → §3 TimelineStage 态）

后端 SSE 事件（§9）：`planner_started → todo_ready → todo_started → todo_finished → asset_generated → todo_failed → run_done`，每事件带 `todo_id / type / payload`。

| SSE 事件 | UI 转换 |
|---------|---------|
| `planner_started` | S1 节点 → `running`；SlateBar 显示；项目徽标 → `生产中`；日志追加 |
| `todo_ready ×N` | 对应 type 的 stage 由 `blocked` → `pending`；日志 `todo_ready ×N` |
| `todo_started`（按 `type`） | 该 stage 节点 → `running`（虚线 ring 旋转，agent 色环）；StageChip → `t-run` |
| `todo_finished`（type=script/storyboard） | 该 stage → `done`（agent 色填充，✓）；其连接线 `.linked` 着 agent 色；后继依赖满足者 `blocked→pending` |
| `todo_started`（type=asset，每 shot 一个） | S4 PipGroup 中对应 pip → `running`（条纹） |
| `asset_generated`（每 shot） | 对应 pip → `done`（asset 色填充）；`done/N 完成` 计数 +1；日志 `asset_generated 待审` |
| `todo_failed`（type=asset） | 对应 pip → `failed`（danger）；日志 `todo_failed {id} · 退避重试`；worker 重试时该 pip 回 `running`（原型第6个 pip 失败重试范式） |
| `todo_failed`（阶段级，重试耗尽） | 该 stage 节点 → `failed`；阻断后继 stage 维持 `blocked` |
| 全部 asset pip done | S4 → `done`；S5 Review → `pending`（StageChip `待人工审核`） |
| `run_done` | SlateBar 隐藏；项目徽标 → `待审核 · N`（N=pending 资产数）；sonner 成功 toast |
| `fallback_used`（来自 plan，非 SSE 而是 `GET /events` 或项目详情字段 `plans.fallback_used`） | 左栏 WarnStrip 显示「Planner 输出畸形，已回落默认管线」 |

固定阶段语义：**S1 Planner（amber/无 agent 色，规划态）· S2 Script（蓝）· S3 Storyboard（紫）· S4 Asset×N（琥珀 pip 组）· S5 Review（绿，admin 门禁）**。S4 的 N 来自 `todo_finished(storyboard)` 的 payload（shots 数）。

### 6.2 重连 / 历史回放

- 进入工作台：先 `GET /api/projects/{id}/events`（分页，`run_events` 历史）**重建当前轨道全态**，再开 `GET /api/projects/{id}/events/stream` 续接实时。
- 断线：`@microsoft/fetch-event-source` 自动重连，携 `Last-Event-ID`（=`run_events.seq`）从断点续传，不重复渲染已处理 seq。
- 完成态项目（status=completed/review/failed）：只 `GET /events` 回放，**不开 stream**（无活跃 run）。

### 6.3 SSE 连接指示（SseIndicator）

| 状态 | 视觉 | 触发 |
|------|------|------|
| connected | green dot 脉冲 + 「实时连接」 | stream open |
| reconnecting | amber dot + 「重连中」 | fetch-event-source onerror 重试期 |
| disconnected | text-3 灰 + 「已断开」 | 重试耗尽 / 完成态不开流时隐藏 |

### 6.4 fallback_used 告警条

`plans.fallback_used=true` → 左栏常驻 WarnStrip（amber）：`⚠ Planner 输出畸形，已回落默认管线（fallback_used）`。非阻断，仅告知。

---

## 7. 视图级契约（View-by-View）

> 路由用 TanStack Router。org 上下文：列表类挂 `/orgs/{org}/...`，按 id 详情挂 `/projects/{id}` 等（与 §9 路径一致）。RBAC 由后端强制（§8），前端按角色**隐藏/禁用**入口（不作安全边界，仅 UX）。

### 7.1 登录 `/login`（M1）

- **API**：`POST /api/auth/login`（+ `refresh`/`logout`）。
- **DTO**：`{email,password}` → `{accessToken, refreshToken, user{role,...}}`。
- **状态**：loading（按钮 spinner）/ error（凭据错→「邮箱或密码错误，请重试」）/ success→跳项目列表。
- **RBAC**：无（公开）。token 存内存 + refresh，所有 API 与 SSE 带 `Authorization: Bearer`。

### 7.2 项目列表 / 建项目 `/orgs/{org}/projects`（M1）

- **API**：`GET /api/orgs/{org}/projects`（cursor 分页）；`POST`（editor+ 建）。
- **DTO**：`Project{id,name,description,content_type,target_platform,style,status,created_at}`；`status ∈ {draft,planning,running,review,completed,failed,canceled}`。
- **建项目表单**（rhf+zod）：名称 · 创意 brief（textarea）· 内容类型 · 目标平台 · 风格（下拉，取自 `GET /api/prompt-styles`：日漫/吉卜力/皮克斯/迪士尼/写实/赛博朋克/国风）。
- **状态**：
  - loading：卡片骨架屏
  - empty：「还没有项目」+ 「用一句创意需求开始你的第一支作品」+ 主 CTA `新建项目`
  - error：「项目加载失败」+ 「重试」按钮
- **RBAC**：viewer 可见列表，`新建项目` 按钮 editor+ 才显示（viewer 隐藏）。
- 项目卡片显示状态徽标（status→Badge variant 映射）；点击进工作台。

### 7.3 项目工作台 `/projects/{id}`（M1 文本管线，M2 起含 asset pip + 预览缩略图）

- **API**：`GET /api/projects/{id}`、`GET /api/projects/{id}/events`（回放）、`GET /api/projects/{id}/events/stream`（SSE）、`GET /api/projects/{id}/{todos,script,shots,assets}`；`POST /run`（editor+，重跑=重规划）、`POST /cancel`。
- **DTO**：Project + `Todo{id,type,status,agent,depends_on,attempts,...}` + `Plan{fallback_used,valid}` + `RunEvent{seq,kind,todo_id,payload,ts}` + 右栏选中工件（script/shot/asset 之一）。
- **布局**（三栏，原型 `250px 1fr 330px`）：
  - 左：BriefCard + 项目信息 KV（内容类型/平台/风格/plan id）+ WarnStrip（fallback）+ EventLog。
  - 中：制片轨道（§6）。`max-width:560px` 居中。
  - 右：选中节点工件预览 —— **缩略图（M2，asset 签名 URL）** + KV（类型/Shot/Provider·Model/耗时·attempts）+ PromptBox + 「在分镜视图中打开」。M1（仅文本）右栏展示 script/shot 文本工件，无缩略图。
- **实时**：§6 全套。`重新运行` 按钮触发 `POST /run` 后清空轨道重跑（原型 `resetPipeline`）。`取消` → `POST /cancel`。
- **状态**：loading（轨道骨架）/ planning（S1 running，余 blocked）/ failed（节点 danger + 重试入口）/ empty（draft 未运行：「项目尚未运行」+ `运行` CTA editor+）。
- **RBAC**：viewer 只读（`运行`/`取消`/`重新运行` 隐藏）；editor+ 可运行。

### 7.4 剧本视图 `/projects/{id}/script`（M1）

- **API**：`GET /api/projects/{id}/script`。
- **DTO**：`Script{content_json{故事/对白/人物/场景}, version}`（§13 R1：容错解析，zod schema 兜畸形 JSON）。
- **状态**：loading 骨架 / empty（「剧本尚未生成」，S2 未完成时）/ error（解析失败→「剧本数据异常，请重新运行剧本阶段」）。
- **RBAC**：viewer+ 只读。

### 7.5 分镜栅格 `/projects/{id}/storyboard`（M1 文本，M2 起每 shot 挂 asset 缩略图）

- **API**：`GET /api/projects/{id}/shots`（M2 起合并 `?...assets`）。
- **DTO**：`Shot{shot_no,camera,scene,action,prompt,duration,ordering}` + 关联 asset（M2）。
- **布局**：栅格（auto-fill minmax 150–170px），每格 shot 编号 + 镜头描述 + prompt 摘要（M2 加缩略图）。
- **状态**：loading 骨架 / empty（「分镜尚未拆解」）/ error。
- **RBAC**：viewer+ 只读。

### 7.6 审核看板 `/orgs/{org}/review`（M2）

- **API**：`GET /api/orgs/{org}/assets?status=pending_acceptance&type=image&...`；`GET /api/assets/{id}`（含版本血缘）；HITL：`POST /api/assets/{id}/{accept,reject,regenerate}`（**admin**）；`GET /api/assets/{id}/content`（302 签名 URL，§10）。
- **DTO**：`Asset{id,shot_id,type,blob_key,url,signedUrl,prompt,style,provider,model,status,version,parent_asset_id,tags}`。
- **布局**（`1fr 460px`）：左=过滤 chips（项目/风格/类型）+ AssetCard 网格（auto-fill minmax 150px，sel=amber 描边）；右=Drawer（hero 签名 URL 图 + KV + PromptBox + LineageTrail + actions）。
- **HITL actions**（Drawer 底部）：`✓ 采纳 [A]`(green) · `✗ 退回 [R]`(red) · `✎ 改 Prompt 重生成 [E]`(ghost)。
  - 采纳 → `accept`，乐观更新移出待审队列，TanStack Query 失效 review + 资产库。
  - 退回 → `reject`。
  - 重生成 → 打开 PromptBox 编辑表单（rhf），`regenerate` body=`{prompt, params}`；派生新版本（`parent_asset_id` 血缘，version+1），新 asset-type todo 入队。
  - 防重：对非 `pending_acceptance` 资产操作 → 后端 409 → toast「该资产已被处理（{当前状态}）」。
- **实时**：可订阅项目 stream，`asset_generated` 时新资产进入待审网格（badge 计数 +1）。
- **状态**：loading 骨架 / empty（「没有待审资产」+「所有素材都处理完了」）/ error。
- **RBAC**：**HITL 三动作 admin-only**。viewer/editor 可浏览网格与 Drawer（只读），actions 区对非 admin **隐藏**（不是禁用灰显——避免误以为可点）。
- **键盘**：见 §9。

### 7.7 资产库 `/orgs/{org}/assets`（M2）

- **API**：`GET /api/orgs/{org}/assets`（标签/风格/项目/类型过滤 + keyset 分页）；`GET /api/assets/{id}`。
- **DTO**：Asset（同上）+ 版本血缘。
- **布局**（`190px 1fr`）：左 FilterRail（类型[图片✓ / **视频(二期) disabled**]、状态[accepted/pending/rejected]、风格[国风/赛博朋克/吉卜力/皮克斯...]、项目下拉）；右网格（status Badge + vtag 版本号）+「加载更多」（keyset 游标）。顶部标签搜索 input。
- **状态**：loading（网格骨架）/ empty（无过滤结果：「没有匹配的资产」+「调整筛选条件试试」）/ error。
- **RBAC**：viewer+ 只读浏览。
- **M2/M3-only 标记**：视频过滤项 `disabled` 且标「二期」（原型已示，对应 M4 视频生成）。

### 7.8 成本中心 `/orgs/{org}/cost`（M3，**admin-only**）

- **API**：`GET /api/orgs/{org}/cost`、`GET /api/projects/{id}/cost`（按项目/时间聚合）。
- **DTO**：聚合 `{monthCost, generationCount, tokenUsage}` + 按项目 `{project,costMicros}[]` + 明细 `Generation{ts,project,provider,model,kind,usage,cost_micros}[]`。
- **布局**：3 StatCard（本月成本/生成次数/Token 用量）+ 按项目成本 BarRow（条色轮换 agent 色）+ DataTable（时间/项目/provider·model/类型/用量/金额，mono 右对齐数字）+ 时间范围 chip（近 30 天 ▾）。
- **状态**：loading 骨架 / empty（「暂无成本数据」）/ error。
- **RBAC**：**整个视图 admin-only**；非 admin 导航不显示「成本」入口，直访路由 → 重定向 + 「需要管理员权限」。

### 7.9 模型配置 `/orgs/{org}/model-configs`（M3，**admin-only**）

- **API**：`GET/POST /api/orgs/{org}/model-configs`；`GET /api/model-catalog`。
- **DTO**：`ModelConfig{id,kind,provider,model,enabled,is_default,params_json}`（**API key 不在此，服务端密钥，永不下发**）+ catalog 可选项。
- **布局**：按 kind（chat/image）分组的配置表 + 编辑表单（provider/model 下拉来自 catalog、enabled/is_default 开关、params JSON 编辑）。
- **状态**：loading / empty（「尚未配置模型」+「添加第一个模型」）/ error / 保存成功 toast。
- **RBAC**：**admin-only**（同 7.8）。
- **安全提示**：表单**绝不**含 API key 字段；UI 文案明示密钥在服务端管理。

---

## 8. Copywriting Contract

| Element | Copy |
|---------|------|
| Primary CTA（项目列表） | `新建项目` |
| Primary CTA（工作台） | `运行` / 已跑过为 `重新运行` |
| Empty state heading（项目列表） | `还没有项目` |
| Empty state body（项目列表） | `用一句创意需求开始你的第一支作品` |
| Empty state（审核看板） | `没有待审资产` / `所有素材都处理完了` |
| Empty state（资产库） | `没有匹配的资产` / `调整筛选条件试试` |
| Empty state（工作台/draft） | `项目尚未运行` + CTA `运行` |
| Error state（通用列表） | `加载失败` + `重试` |
| Error state（剧本解析） | `剧本数据异常，请重新运行剧本阶段` |
| Error state（管线失败节点） | `{阶段}失败 · 已重试 {attempts} 次` + `重新运行` |
| Warn（回落） | `⚠ Planner 输出畸形，已回落默认管线（fallback_used）` |
| 权限拒绝 | `需要管理员权限` |
| Destructive — 退回资产 | `退回此资产？退回后将标记为 rejected，可改 Prompt 重新生成。`（Drawer 内 R，原型为直接操作；建议轻确认或可撤销 toast，二选一须全站一致——见 §11 默认决策） |
| Destructive — 取消运行 | `取消当前运行？已生成的工件会保留，未完成的任务将中止。` 确认按钮 `取消运行` |
| Destructive — 删除项目 | `删除项目「{name}」？此操作不可撤销，项目下所有工件与资产将一并移除。` 确认按钮 `删除` |

---

## 9. Accessibility

- **键盘快捷键（审核看板）**：`A` 采纳 · `R` 退回 · `E` 改 Prompt 重生成 · `←/→` 切换上/下一个待审资产。仅 admin 时 A/R/E 生效（非 admin 仅 ←→ 浏览）。快捷键在输入框聚焦时禁用（避免误触）。Drawer 内 `<kbd>` 标注每个动作。
- **focus-visible**：全局 `outline:2px solid var(--amber); outline-offset:2px`（原型已定），所有按钮/链接/卡片可键盘聚焦。AssetCard 为 `<button>`，Tab 可达。
- **prefers-reduced-motion: reduce**：停用 SlateBar `slide`、running dot `pulse`、running 节点 ring `spin`、pip 条纹动画（原型 media query 已声明）；节点态改用纯色/边框区分（不依赖动画传达 running）。
- **色彩对比**：
  - 正文 `--text-1 #EDEEF0` on `--bg-base #17191E` ≈ 14:1（AAA）。
  - 次要 `--text-2 #9AA1AC` on bg ≈ 6.5:1（AA 正文 / AAA 大字）。
  - `--text-3 #666D78` ≈ 3.5:1 —— **仅限非正文**（标签/时间戳/装饰/占位），不承载关键可读信息。
  - amber 主 CTA：深字 `#1a1408` on `#E8A33D` 高对比，达标。
- **状态不单靠颜色**：节点态同时用形状/图标（✓ done、ring running、虚线 blocked、danger 填充 failed）+ 文字 StageChip（done/running/pending/blocked/failed），色盲可辨。Badge 同时有 dot + 文字。
- **签名 URL 图片**：`<img>` 带描述性 `alt`（shot 描述，如「#03 中景·推 茶馆内黄昏」）。
- **语言**：`<html lang="zh-CN">`（原型已定）。

---

## 10. Phasing（视图 → 里程碑映射）

> 后端 §15 里程碑：M1 文本管线（已建骨架）· M2 图片+HITL+资产库（PRD 一期完成线）· M3 准生产横切（成本/模型/可观测）· M4 二期视频音频。前端按批跟进。

| 视图 | 里程碑 | 备注 |
|------|--------|------|
| 登录 | M1 | authz |
| 项目列表 / 建项目 | M1 | |
| 项目工作台（SSE 制片轨道，文本管线 S1–S3） | M1 | 右栏工件预览=文本（无缩略图） |
| 剧本视图 | M1 | |
| 分镜栅格（文本） | M1 | |
| 运行历史（`GET /events` 回放） | M1 | 工作台内置回放，非独立路由 |
| 工作台 S4 Asset pip 组 + 右栏缩略图 | M2 | 依赖图片生成 |
| 审核看板 / HITL（accept/reject/regenerate + 版本血缘） | M2 | admin |
| 资产库（过滤 + 网格 + 加载更多） | M2 | |
| Prompt Builder（建项目风格选择 + 重生成 prompt 编辑） | M2 | `GET /prompt-styles`、`POST /prompt/build` |
| 成本中心 | M3 | admin |
| 模型配置 | M3 | admin |
| **视频过滤项 / 视频 asset 卡** | **M4（UI 标「二期」disabled）** | 原型已示禁用态，本期不实现交互 |

**M2/M3-only UI 在 M1 的处理**：导航轨「审核/资产/成本」入口在对应里程碑前可隐藏或显示「即将上线」占位；视频相关控件全程标「二期」`disabled`（原型范式）。

---

## 11. 默认决策（两文档均未明确，记录合理默认而非追问）

1. **退回（reject）确认强度**：原型为 Drawer 内直接操作（无模态）。**默认**：reject 走**可撤销 toast**（sonner，5s 内「撤销」），不弹模态——保持原型的快速审核流（A/R/E 键盘连击）。删除项目/取消运行才用模态确认（§8）。
2. **token 存储**：access token 存内存（非 localStorage，防 XSS），refresh 经 httpOnly cookie 或内存 + 静默刷新——具体由后端 authz cookie 策略定；前端默认假设 Bearer header + 内存 access + `/refresh` 续期（kb 范式）。
3. **签名 URL 刷新**：资产 `signedUrl` 短 TTL（§10）。**默认**：图片加载失败（403 过期）时自动重拉 `GET /api/assets/{id}/content`（302）刷新；长开抽屉超 TTL 同理。
4. **正文字号**：锁 13px（原型 body 值），不在 14–16 间游移；故 §5 typography 仅 3 主档（30/17/13）+ 标签 11px。
5. **shadcn 初始化时机**：本契约不执行 `npx shadcn init`（无 `web/`），由 UI 实现阶段首步初始化并按 §4 覆写 CSS 变量。
6. **运行历史**：不单设路由，复用工作台 `GET /events` 回放 + 完成态项目只回放不开流（§6.2）。

---

## 12. Registry Safety

| Registry | Blocks Used | Safety Gate |
|----------|-------------|-------------|
| shadcn official | button, badge, dialog, drawer/sheet, checkbox, dropdown-menu, input, textarea, select, table, skeleton, sonner | not required |
| 第三方 | none（未声明任何第三方 registry） | not applicable |

> 自建组件（TimelineStage / PipGroup / SlateBar / LineageTrail / AssetCard / StatCard / BarRow / SseIndicator 等）以 cva + Tailwind 实现，贴合原型设计语言，不来自任何 registry。

---

## 13. Checker Sign-Off

- [ ] Dimension 1 Copywriting: PASS
- [ ] Dimension 2 Visuals: PASS
- [ ] Dimension 3 Color: PASS
- [ ] Dimension 4 Typography: PASS
- [ ] Dimension 5 Spacing: PASS
- [ ] Dimension 6 Registry Safety: PASS

**Approval:** pending
