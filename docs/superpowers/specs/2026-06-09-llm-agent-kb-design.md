# llm-agent-kb 设计文档（企业知识库 GraphRAG 问答平台）

- 日期：2026-06-09
- 状态：设计已批准，待写实现计划
- 类型：新案例项目（准生产级，独立 sibling 仓）
- 相关：[[project_case-study-projects]]；姊妹项目 llm-agent-studio（后做）

## 1. 目标与定位

为 `llm-agent-ecosystem` 提供第一个**面向终端用户的全栈案例项目**：企业知识库 GraphRAG 问答平台。它充分串联 `llm-agent-rag` 的检索深度（向量 / 混合 / GraphRAG 全局问答 / 质量评估），并以独立产品形态（真实登录、组织/库多租户、文档治理、引用溯源、质量仪表盘）区别于已有的 `llm-agent-customer-support`（单库 RAG 客服后端，无独立前端）。

**非目标（v1 不做）**：扫描件 OCR、复杂表格抽取、真流式 token 输出、跨会话长期用户记忆、外部消息队列、K8s 清单、多 embedding 模型并存。

### 成功标准

一次端到端可验证流程：
登录 → 建组织/知识库 → 上传 PDF/DOCX/URL/粘贴文本 → 等待异步索引 `ready` → 提问（hybrid，返回带引用的答案）→ 全局问答（AskGlobal）→ 跑 eval → 看到 drift 报告。后端 `httptest`+真 pgvector，前端 Vitest+@testing-library，`docker compose up` <影响首次模型拉取的时间内起栈。

## 2. 已确定的范围决策（v1）

| 维度 | 决策 |
|---|---|
| 摄入来源 | 文件上传 PDF/Markdown/TXT + DOCX + 网页 URL 抓取 + 粘贴纯文本 |
| 租户/鉴权 | 组织(org)→知识库(kb) 层级 + 用户账号 RBAC（库级角色 admin/editor/viewer + org 级 org_admin）；由共享库 **`llm-agent-authz`** 提供登录/JWT/refresh/argon2id/OIDC 预留（见 authz spec + §8/§16） |
| 检索/问答 | 向量、Hybrid+rerank、GraphRAG `AskGlobal`/`AskDrift` 全部纳入 v1 |
| 质量评估 | eval/drift 仪表盘纳入 v1（retrieval / triad / global / drift） |
| 流式 | **v1 非流式**，一次返回 `Answer` + 前端打字机动画（保留 rag 完整 reflection/grader/citation 编排）。真流式放 M5 |
| embedding | **v1 全局单一 embedding 模型/维度**，所有 kb 共享一个 `rag.System`；kb 表仍存 `embedding_model/embedding_dim` 便于未来扩展 |
| PDF | **仅文本型 PDF**（纯 Go 库提取）；OCR/表格列为后续 |
| 技术栈 | 后端 Go（BFF+业务一体）；前端 React19/TS/Vite/Tailwind v4/shadcn/TanStack（独立选型，详见 §15 说明） |
| 可观测 | `llm-agent-otel`（`otelrag.Wrap` + `otelmodel.Wrap`） |

## 3. 总体架构

单 Go 二进制 `kbd` 同时承担 BFF（同源、服务端注入 auth/RBAC、SSE 透传）与业务服务（内嵌 `rag.System`）。kb 没有可代理的下游业务 API，业务逻辑就在本进程内调用 `rag.System`，采用 `customer-support`（已发版 v0.3.0，稳定参照）的「内嵌 rag + 自有 httpapi」形态。

> 注：图中 `/api/*/stream` 的 SSE **仅用于摄入索引进度**（§6）；问答答案为非流式（§7）。

```
Browser (React SPA, 单 origin)
  │  /api/* + /api/*/stream (SSE)
  ▼
kbd (Go 进程: httpapi[BFF+业务] · authz(import) · orgkb · ingest · retrieval · eval · ragsvc · limits · obs)
  │              │                         │
  ▼              ▼                         ▼
rag.System   ingest worker pool      otelrag.Observer → otel collector
(namespace=kb_id)  (Postgres 队列)
  │
  ▼
Postgres + pgvector
  · authz 表(auth_user/auth_org/auth_membership/auth_session, 由 authz 提供 migrations)
  · kb 业务表(knowledge_base/document/ingest_job/qa_session/qa_message/eval_run)
  · rag 自管表(chunks + graph + community, 由 postgres.Store.Migrate 建)
外部: llm-agent-providers (ChatModel + Embedder)
```

进程/容器边界：`kbd` 单二进制（embed `web/dist`）；Postgres 单实例（pgvector）；otel collector 复用 `llm-agent-otel/compose/`；LLM/Embedding 为外部 provider（compose 可选 ollama 免 key）。

## 4. 仓库与模块布局

Go module：`github.com/costa92/llm-agent-kb`（独立 sibling 仓，自有 git 历史 + `.planning/`，从 umbrella gitignore；Go 命令需 `GOWORK=off`，见 [[project_console-gowork-off]]）。

```
llm-agent-kb/
├── cmd/kbd/main.go        # 装配: config→otel→pgxpool→providers→rag.System→httpapi→listen
├── internal/
│   ├── config/            # env 解析(对标 customer-support/internal/config)
│   ├── (authz 接线)       # import llm-agent-authz: mount /api/auth/*, Authenticate/RequireScopeRole 中间件 (不自建 JWT/argon2id/membership)
│   ├── orgkb/             # kb 资源领域服务(建/删/列举库) + Postgres 仓储; org/user/membership 由 authz 提供, 建库时写 creator admin membership
│   ├── ingest/            # 解析(PDF/DOCX/URL/MD/TXT)→ragingest.Document; 异步 worker; 进度
│   ├── retrieval/         # 把 rag.Ask/AskGlobal/AskDrift 封成业务用例 + 引用映射
│   ├── eval/              # 触发 RetrievalEvaluator/Triad/Global/Drift; 存 eval_run; drift 对比
│   ├── ragsvc/            # 唯一持有 rag 后端的单元; 定义窄接口 RagPort(Ask/AskGlobal/AskDrift/Import); providers 适配器(含 ragModelAdapter)
│   ├── storage/           # pgxpool + RegisterTypes + 业务表 migrations + rag Migrate
│   ├── limits/            # 限流/配额(对标 customer-support/internal/limits)
│   ├── httpapi/           # ServeMux 路由/handler/中间件链/SSE/embed web/dist
│   └── obs/               # otel TracerProvider 装配 + otelrag.Observer 接线
└── web/                   # React SPA: src/{app,features,components,lib}/，按 feature 分目录
```

**单一职责约束**：`ragsvc` 是唯一直接依赖 rag 后端的包，其它包只依赖其 `RagPort` 窄接口。**`RagPort` 由 `ragsvc` 的薄适配器实现，不是靠某个现成类型直接满足**——因 `*otelrag.Wrapper` 只暴露 `Ask/Import/Retrieve`、**不含** `AskGlobal/AskDrift/PrewarmCommunityReports`（otelrag.go:99/124/145）：故适配器把 `Ask/Import/Retrieve` 委托 `*otelrag.Wrapper`（自动出 span），把 `AskGlobal/AskDrift/Prewarm` 委托 `Wrapper.Inner()`(`*rag.System`) 并自建 span。删除能力亦在 `RagPort`（见 §16），由 `ragsvc` 直接持有的 `postgres.Store` 承载。`orgkb` 不碰向量；`ingest` 只产出 `ragingest.Document` 并驱动 `system.Import`，不直接写 chunks 表；`httpapi` 不含业务规则，只做编排+鉴权+SSE。

## 5. 数据模型

业务表与 rag 自管表共存于同一 Postgres。向量**不**自建表——复用 rag 的 `chunks` 表（pgvector，可选 IVFFlat/HNSW，`postgres.Config.VectorIndex`）。

**鉴权/租户表由 `llm-agent-authz` 拥有**（`auth_user`/`auth_org`/`auth_membership`/`auth_session`，见 authz spec）；kb 不再自建 org/user/membership。kb 的 `knowledge_base` 即 authz membership 的 `scope_kind="kb"` 所指资源。

**业务表（kb 拥有，迁移由 `internal/storage` 管）**：
- `knowledge_base(id, org_id, name, namespace UNIQUE, embedding_model, embedding_dim, created_at)` — `namespace` 即 `ImportOptions.Namespace`/`SearchOptions.Namespace`，实现租户隔离；`org_id` 弱引用 `auth_org`
- `document(id, kb_id, title, source_type, source_ref, source_id, checksum, status, error, chunk_count, created_at)` — `status ∈ {pending,parsing,embedding,indexing,ready,failed}`；**`source_id`=稳定源标识（=document.id），写入 `ragingest.Document.SourceID` 驱动 `ReplaceSource` 替换与按源删除**；`checksum`=`ragingest.Document.Checksum`，仅用于"内容未变则跳过 Import"短路（不参与替换决策，见 §16）
- `ingest_job(id, document_id, state, attempts, next_run_at, locked_by, locked_until, idempotency_key, last_error, phase, updated_at)` — worker 租约队列（`FOR UPDATE SKIP LOCKED` + `locked_until` 租约，stuck-job 回收按 `locked_until` 过期重取；`dead` 为终态，见 §16）
- `qa_session(id, kb_id, user_id, title, created_at)` + `qa_message(id, session_id, role, content, citations_json, mode, created_at)` — 问答历史（kb 自存）
- `eval_run(id, kb_id, kind, dataset_name, metrics_json, drift_json, created_at)` — `kind ∈ {retrieval,global,drift,triad}`；`metrics_json` 存对应 evaluator 结果中的指标子结构（retrieval=`eval.Metrics`；triad/global 为各自 `*Result` 的指标部分，非裸 `Metrics`）；`drift_json`=`eval.DriftReport`

**rag 自管表（由 `postgres.Store.Migrate` 建）**：`chunks`（embedding 向量列 + 元数据 + 全文索引，同时实现 `store.LexicalSearcher` 支撑 hybrid）；graph / community 表（`store.GraphStore`/`store.CommunityStore`，供 GraphRAG）。

关系：org 1—N kb，kb 1—N document，document 1—N chunk（chunk 属 rag）。chunk 经 `namespace=kb.namespace` + `Chunk.Metadata{doc_id,kb_id}` 与业务表关联。

## 6. 摄入 pipeline

对外入口：`POST /api/kb/{kbId}/documents`（multipart 上传 / JSON 粘贴 / URL）。

1. **接收**：写 `document(status=pending)` + 入队 `ingest_job`，立即返回 202 + documentId（异步）。
2. **解析（worker 内）**，按 `source_type`：
   - PDF：纯 Go 库（`github.com/dslipak/pdf` 或 `ledongthuc/pdf`），仅文本型。
   - DOCX：纯 Go OOXML 解析（`github.com/fumiama/go-docx`）。
   - Markdown：走 rag `ingest.NewMarkdownSplitter` 保留 section/heading（供 citation 的 SectionPath）。TXT 直接读。
   - URL：`net/http` 抓取 + 正文抽取（`go-readability`/`html-to-markdown`）→ 当 Markdown 处理。
3. **chunk+embed+入库**：构造 `[]ragingest.Document`（`ID`=业务 document.id、**`SourceID`=document.id**(驱动 `ReplaceSource` 替换与按源删除，splitter.go:77-91 仅在 `SourceID!=""` 时生效)、`Title/Content/Checksum`、`Metadata{kb_id,source_type}`；**不放 doc_id**——citation 的 `DocID` 来自 `Chunk.DocID=doc.ID`，非 metadata），调 `system.Import(ctx, docs, ImportOptions{Namespace: kb.namespace, ReplaceSource: 重导})`。chunk/embed/写 pgvector 全由 rag 完成；embedder 走 `BatchEmbedder` 批量快路径。
4. **建 GraphRAG 社区**：rag 在 `Import` 持久化后自动抽实体图并检测社区（需配 `EntityExtractor`+`CommunityDetector` 且 store 实现 `CommunityStore`）。社区报告惰性生成；索引完成后调 `system.PrewarmCommunityReports(ctx, namespace)` 预热。组件：`graph.LLMEntityExtractor`、`graph.LouvainDetector`、`graph.LLMCommunitySummarizer`，可选 `graph.EmbeddingEntityResolver` 合并近重复实体。
5. **进度/失败重试**：worker 每阶段更新 `document.status`；失败写 `error`，按 `attempts` 退避重试（借鉴 rag `eval` 的瞬态分类）。前端轮询 `GET …/documents/{id}` 或 SSE `…/documents/{id}/progress`。

**编排**：进程内 worker pool + Postgres 作队列（`SELECT … FOR UPDATE SKIP LOCKED`），不引外部 MQ；队列在库，重启可恢复。

## 7. 检索 / 问答 API

| 对外 HTTP | rag 调用 | 关键参数 |
|---|---|---|
| `POST /api/kb/{kbId}/ask`（`mode=vector\|hybrid`） | `system.Ask(ctx, q, AskOptions{...})` | `Search.TopK`、`Search.EnableRerank`（hybrid 开/向量关）、可选 `EnableMQE/EnableHyDE/Reflection`、`MaxTotalTokens`(配额) |
| `POST /api/kb/{kbId}/ask/global` | `system.AskGlobal(ctx, q, GlobalOptions{Namespace, MaxCommunities})` | 全局社区 map-reduce |
| `POST /api/kb/{kbId}/ask/drift` | `system.AskDrift(ctx, q, DriftOptions{Namespace, MaxCommunities, Rounds, TopK})` | 全局 primer + 局部 follow-up |

- **vector vs hybrid**：同一 `Ask` 接口用请求体 `mode` 区分；默认 retriever 即 hybrid，`mode=vector` 关 lexical/rerank。
- **流式**：v1 非流式，一次返回 `Answer`，前端打字机动画。
- **citation**：映射 `Answer.Citations{ChunkID,DocID,Namespace,Title,SectionID,SectionPath,Score}`。对外 JSON：`{answer, citations:[{chunkId,docId,title,sectionPath,score,snippet}], diagnostics:{mode,...}}`；`snippet` 取 `Answer.Hits[].Chunk.Content` 裁剪；`diagnostics` 暴露 `Answer.Diagnostics`（含 Global/Drift/Reflection 子结构）。

## 8. RBAC 与鉴权

- 登录/JWT/refresh/argon2id/`Authenticator`(预留 OIDC) 全部由 `llm-agent-authz` 提供（kb 不自建）；前端存 access 于内存 + httpOnly+SameSite=Strict refresh cookie，refresh 轮换/重用检测/登出由 authz 的 `auth_session` 承载。
- 中间件链：`otel → authz.Authenticate(解析JWT,挂当前user) → authz.RequireScopeRole("kb", minRole)(按 org级⊕库级合并取最高) → limits → handler`。
- 校验点：写操作(上传/删除文档/建 kb)要求 `editor+`；读/问答要求 `viewer+`；成员管理/删库/敏感操作要求 `admin`。
- **租户隔离落到 rag**：vector/hybrid（`Ask`）路径用 `SearchOptions.SecurityFilters`（rag 保证不可绕过，options.go:19-24）+ `Namespace=kb.namespace` 双层。**注意 `GlobalOptions`/`DriftOptions` 不带 `SecurityFilters`（仅有 `Namespace` + 图参数）**——故 GraphRAG 全局/drift 的隔离**仅靠 namespace**（在 RBAC 通过之后）；v1 文档级 ACL 不经 global/drift 暴露，待 rag 上游支持再扩展。
- 鉴权（登录/JWT/RBAC/org/user/membership）由共享库 **`llm-agent-authz`** 承载，本节其余细节见该 spec 与 §16；kb 仅注入 scope_kind=`"kb"` 并在建库时写 creator 的 admin membership。角色统一 `admin/editor/viewer`（+ org 级 `org_admin`）。

## 9. eval / drift 仪表盘

- 触发：手动 `POST /api/kb/{kbId}/eval/run`（选 kind + 上传/选 JSONL 数据集）+ 可选定时（复用 worker）。
- 执行：retrieval=`eval.RetrievalEvaluator`（`Run` 返回 `RetrievalResult`，内含 `Metrics{PrecisionAtK,RecallAtK,MRR,GroundingAtK}`；数据集经 `eval.LoadJSONL`→`Dataset`）；答案质量=`eval.TriadEvaluator`(+`eval.LLMJudge`，返回 `TriadResult`)；GraphRAG=`eval.GlobalEvaluator`(→`GlobalEvalResult`)/`eval.DriftEvaluator`(→`DriftEvalResult`)；漂移=`eval.CompareBenchmarks(prev, curr eval.BenchmarkResult)`→`eval.DriftReport`（含 `MetricDelta`+`Direction`+`HistogramDelta`）。
- 存：`eval_run` 表（指标子结构序列化）；如需基准 JSONL，先把结果组装为 `eval.BenchmarkResult` 再 `eval.WriteBenchmarkJSONL(w io.Writer, r BenchmarkResult)` 落卷（调用方自备 Writer；drift 对比即吃两份 `BenchmarkResult`）。
- 前端：precision/recall/MRR/grounding 数值卡 + 历史折线；triad 三项；drift 用 `Direction` 标 improved/regressed/unchanged，`HistogramDelta` 画分布。

## 10. 前端视图清单

前端按 feature 分目录 + TanStack Router/Query；REST 走 TanStack Query，SSE 走 `@microsoft/fetch-event-source` 不入 Query 缓存（自建轻量 SSE 客户端）。

| 路由 | 视图 | 核心组件 |
|---|---|---|
| `/login` | 登录 | LoginForm(react-hook-form+zod) |
| `/orgs`、`/orgs/$orgId/kbs` | 组织/库管理 | KbTable、CreateKbDialog、MembersPanel(RBAC 分配) |
| `/kb/$kbId/documents` | 文档上传+索引进度 | Dropzone、UrlInput、PasteText、DocStatusTable(进度徽章) |
| `/kb/$kbId/ask` | 问答 | ModeSwitch(vector/hybrid/global/drift)、AnswerPane(打字机)、CitationList(跳转源+SectionPath 高亮)、DiagnosticsDrawer |
| `/kb/$kbId/graph` | GraphRAG 主题/社区 | CommunityList、CommunityReportCard、可选社区关系图 |
| `/kb/$kbId/eval` | eval/drift 仪表盘 | MetricCards、TrendChart、TriadPanel、DriftReportTable、RunEvalDialog |
| `/kb/$kbId/sessions` | 问答历史 | SessionList、SessionTranscript(复用 CitationList) |

每 feature 目录 `{Page.tsx, api/, components/, hooks/}`。

## 11. 准生产级横切

- 鉴权：JWT + RBAC 中间件 + rag `SecurityFilters` 双保险（§8）。
- 限流/配额：`internal/limits` 沿用 `customer-support/internal/limits` 的 Preflight/配额分层**思路**（非直接复用其 `Guard.WrapAgent`——那是包裹 `agents.Agent` 的，机制不同）；kb 的 token 配额改由 rag 自带 `AskOptions.MaxTotalTokens` 承载，超额返回 `*rag.BudgetExceededError`。
- 可观测：`obs` 用 otel `NewTracerProvider` 建 TP。`otelrag.Wrap(*rag.System, Config)` 返回 `*otelrag.Wrapper`（并行方法，只含 `Import/Retrieve/Ask`），由 `ragsvc.RagPort` 适配器统一（§4）：`Ask/Import/Retrieve` 经 Wrapper 自动出 span，`AskGlobal/AskDrift/Prewarm` 经 `Wrapper.Inner()` + `ragsvc` 自建 span（GraphRAG 路径的 span 是 kb 自行埋点，非 otelrag 提供）。token 用量经 `otelrag.MakeOnGenerateUsageHook` 接 `rag.Observer.OnGenerateUsage`；provider 调用 `otelmodel.Wrap`；httpapi `withTrace` 中间件。
- E2E：登录→建 org/kb→上传 PDF→等 ready→ask(hybrid)→断言 citation→ask/global→跑 eval→看 drift。后端 `httptest`+真 pgvector(docker)；前端 Vitest+@testing-library。
- docker-compose：postgres(pgvector) + otel collector(引 `llm-agent-otel/compose/`) + kbd +（可选 ollama）。启动跑业务 migrations + `ragStore.Migrate`。

## 12. 已知必须自建的胶水 / 风险

1. **`ragModelAdapter`（必须新写，~15 行）**：现成 `ChatModel→generate.Model` 适配器在 build-tagged `adapter/llmagent`（会拉 `llm-agent` 核心），kb 不该开该 tag。仿照 `customer-support` 的 `ragEmbedderAdapter` 写一个把 `llm.ChatModel.Generate(llm.Request)` 适配成 `generate.Model.Generate(generate.Request)` 并填 `generate.Usage` 的小适配器。低风险。
2. **锁定 rag 版本 ≥ v1.9.0**：让 `AskGlobal`/`AskDrift` 的 `MaxTotalTokens` 配额生效（customer-support 当前锁 v1.10.0，跟随）。
3. **embedding 维度与 kb 绑定**：`postgres.Config.Dimension` 建表后固定；v1 全局单 embedding 模型规避此问题；提供「重建索引」流程兜底。
4. **PDF/DOCX 解析质量**：v1 限文本型；扫描件 OCR、表格抽取后续里程碑。

## 13. 里程碑

> **前置依赖**：kb M1 依赖 `llm-agent-authz` v0.1.0 tag（先于 kb 动工，见 authz spec §6）。排期耦合：authz 不就绪则 kb M1 不能开工。

- **M1 基座**：依赖 authz(登录/JWT/RBAC) + 建/删/列举 kb + 上传(MD/TXT)+**粘贴纯文本** + `Import`(含 `SourceID` 约定) + `Ask`(**vector+hybrid 双模**) + citation + **单文档删除(级联清理, §16)** + **基础 per-user 限流** + docker-compose + otel。
- **M2 摄入扩展**：PDF/DOCX/URL 解析(**含 SSRF/上传校验, §16**) + 异步 worker(租约/重试/stuck 回收/手动重试) + 进度 + 重导/去重 + 列表分页。
- **M3 GraphRAG**：EntityExtractor/Louvain/Summarizer 接线 + `AskGlobal`/`AskDrift` + 社区视图 + prewarm。
- **M4 质量**：eval(retrieval/triad/global/drift) + 仪表盘 + 配额/限流硬化 + E2E。
- **M5（可选）**：真流式 / 评估接 `llm-agent-memory-client` 做跨会话记忆。

## 14. 关键参考文件（实现期）

- `llm-agent-rag/rag/system.go`（`rag.System` 门面、`Answer`/`Citation`/`Diagnostics`、`New`）
- `llm-agent-rag/rag/options.go`（`Options`/`AskOptions`/`SearchOptions`/`GlobalOptions`/`DriftOptions`）
- `llm-agent-rag/postgres/postgres.go`（pgvector `Store`、`New`/`Migrate`/`RegisterTypes`/`Config`）
- `llm-agent-customer-support`（v0.3.0）`internal/knowledgebase`（**仅** `ragEmbedderAdapter` 写法可参照——该文件用 `InMemoryStore` 且未设 Model，**不**涉及 pgvector/答案路径）+ `internal/{httpapi,limits,config}` 分层
- `llm-agent-otel/otelrag`（`Wrap`→`*Wrapper`/`Observer`/`MakeOnGenerateUsageHook`）
- **无既有样板（greenfield）**：`postgres.Store.Migrate`+pgvector 接线、答案路径（`Ask/AskGlobal/AskDrift`）、`ragModelAdapter`、otelrag 接线，生态内均无现成参照，与前端同属全新构建。

## 15. 前端栈与"无前端参照"说明

生态内目前**没有已定型、可作为代码参照的全栈前端**（`llm-agent-console` 尚未定型，明确**不**作为引用代码）。因此 kb 的 `web/` 是**全新构建**（greenfield）：
- 技术栈 React19/TS/Vite/Tailwind v4/shadcn/TanStack 是**基于自身需求的独立选型**（成熟、React19 兼容、社区活跃），不依赖任何现有仓库代码。
- SSE 客户端、feature 目录结构、表格/表单组件均自建，不移植任何未定型仓库的代码。
- 后端参照仅限**已发版的稳定仓库**：`llm-agent-customer-support`（v0.3.0）的 `internal/{httpapi,limits,config}` 分层 + `knowledgebase` 的 embedder adapter 写法，以及 `llm-agent-rag`/`llm-agent-otel` 的公共 API。postgres+pgvector 与答案路径接线为 greenfield（见 §14）。

## 16. 二轮评审修订（缺口闭合）

本节为二轮评审（`.planning/.specs-review-round2.md`）后的权威补订，覆盖前文未尽的端点、安全、删除、分页等。

### 16.1 共享鉴权依赖
鉴权/租户基座迁至 `llm-agent-authz`（spec 同目录）。kb import 它，注入 `scope_kind="kb"`；org/user/membership/session 表与 `/api/auth/*` 端点由 authz 提供；角色统一 `admin/editor/viewer`(+`org_admin`)，合并规则=org级⊕库级取最高。

### 16.2 HTTP 端点目录（补全 K4）
请求/响应均 JSON；列表统一 cursor 分页 `?limit=&cursor=` → `{items, next_cursor}`。
- 鉴权（authz 提供）：`POST /api/auth/{login,refresh,logout,logout-all}`
- 组织/库：`GET/POST /api/orgs`、`GET /api/orgs/{org}/kbs`、`POST /api/orgs/{org}/kbs`、`GET /api/kb/{id}`、`DELETE /api/kb/{id}`（admin，级联见 16.4）
- 成员：`GET /api/kb/{id}/memberships`、`POST /api/kb/{id}/memberships`、`DELETE /api/kb/{id}/memberships/{userId}`（admin）
- 文档：`GET /api/kb/{id}/documents`（分页）、`POST /api/kb/{id}/documents`（上传/URL/粘贴，202）、`GET /api/kb/{id}/documents/{docId}`、`DELETE /api/kb/{id}/documents/{docId}`、`POST /api/kb/{id}/documents/{docId}/retry`（重触发 failed/dead）、`GET /api/kb/{id}/documents/{docId}/progress`（SSE 索引进度）
- 问答：`POST /api/kb/{id}/ask`（body `{q, mode:"vector"|"hybrid", topK?, options?}`，`options` 仅暴露服务端允许的 rag 开关：`enableRerank/enableMQE/enableHyDE/reflection`，其余服务端固定）、`POST /api/kb/{id}/ask/global`（`{q, maxCommunities?}`）、`POST /api/kb/{id}/ask/drift`（`{q, maxCommunities?, rounds?, topK?}`）。响应 `{answer, citations[], diagnostics}`（`answer` 映射 `Answer.Text`）。
- 会话/eval：`GET /api/kb/{id}/sessions`（分页）、`GET /api/kb/{id}/sessions/{sid}`、`POST /api/kb/{id}/eval/run`、`GET /api/kb/{id}/eval/runs`（分页）。

### 16.3 出站/上传安全（补全 K3）
- **URL 摄入 SSRF 防护**：限 `http/https`；DNS 解析后校验解析到的 IP 不属私网/环回/链路本地(169.254/.../元数据段)，并**用该 IP 直连**防 DNS rebinding；禁止重定向到内网（每跳复检）；连接/读取超时 + 最大字节数 + 响应 MIME 白名单。
- **上传校验**：`http.MaxBytesReader` 限单文件大小；content-type/扩展名白名单（pdf/md/txt/docx）；每 kb 存储配额；解析前对 PDF/DOCX 设解析超时防"解析炸弹"。

### 16.4 删除与级联（补全 K2，订正 API 契约）
`*rag.System` 无导出的按源删除——`ragsvc` 直接持有 `postgres.Store`。**严格照搬 rag 自身 `Import(ReplaceSource)` 的 reconcile 顺序**（rag/import.go:58-78,179-186）：

- **删文档**（事务）：
  1. `ids := Store.List(ns, Filter{source_id})` — 先收集待删 chunk 的 ID（`RemoveByFilter` 只回计数、不回 ID，postgres.go:427）。
  2. `Store.RemoveGraphBySource(ctx, ns, ids)` — **入参是 chunkIDs `[]string`，不是 source**（postgres/graph.go:83）；必须在删 chunks 之前，否则 ID 取不到。
  3. `Store.RemoveByFilter(ns, Filter{source_id})` — 删 chunks。
  4. 删业务行 `document`。
  > `source_id` 是 chunk metadata 的 JSON 键（splitter.go:12 `MetadataSourceIDKey`），过滤走 `metadata @> $n`（postgres.go:555）；海量库建 GIN 索引。
- **社区无法按源精细清理**：`postgres/community.go` 无导出的按源删除（社区由 Louvain 全图重算，`UpsertCommunities` 内部"全 namespace 删后重插"）。删源后**标记该 namespace 社区需重建**：重跑社区检测 + `UpsertCommunities`（覆盖全 namespace）再 `PrewarmCommunityReports`，由后台任务批量做（非每删一文档即重算）。
- **删 kb**：批量删其全部文档（同上 1-4），触发一次该 namespace 社区重建，再删 `knowledge_base` 行 + authz 中该 scope 的 membership。
- **更简替代**：纯"重导替换"场景直接用 `system.Import(ReplaceSource:true)`（rag 内部已按上述顺序 reconcile），仅"真删除"才下沉到 Store 层手工三步。

### 16.5 PII / 注入防护（补全 K9）
启用 rag 的注入/PII 防护（`sanitizeHits`/`InjectionFinding`，ask.go），findings 计入 `Answer.Diagnostics` 并在前端 DiagnosticsDrawer 可见；上传文本入库前可选经同一 guard。

### 16.6 仍为 greenfield（无样板）
`ragModelAdapter`、`RagPort` 适配器（含 global/drift 自埋 span + 删除）、postgres+pgvector 接线、SSRF 防护、ingest 租约队列——生态内均无现成参照，与前端同属全新构建。
