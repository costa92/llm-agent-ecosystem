# 生态架构总览

> 文档版本：2026-07-06（roster 与依赖方向已刷新）
> 对应代码快照：调用链章节（§5–§6）为 2026-05-28；下方 roster 与依赖图为
> 2026-07-06 现状。
> 阅读目标：在 30 分钟内建立 `llm-agent-ecosystem` 的正确心智模型，并能继续沿源码主链深入。
> roster 与依赖图的权威来源是根 [`README.md`](../README.md)；本总览是配套注解。

---

## 1. 项目定位

`llm-agent-ecosystem` 不是单体应用，而是一个 **多仓伞形工作区**。根仓本身只负责：

- 工作区组织与跨仓脚本
- 生态级依赖方向和规则
- 文档导航与规划入口

真正的功能实现分布在约 20 个子项目中。本总览深读框架家族；完整 roster（含应用
仓）见根 [`README.md`](../README.md)：

```text
llm-agent-ecosystem/
├── llm-agent                     # 核心 agent 框架（只 require contract）
├── llm-agent-contract            # stdlib-only 的 LLM-provider 契约（ChatModel + capability）
├── llm-agent-rag                 # 独立 RAG SDK（main 上 additive v1.x）
├── llm-agent-providers           # 模型供应商适配层（只 require contract）
├── llm-agent-otel                # OTel 装饰器
├── llm-agent-customer-support    # 参考客服服务
├── llm-agent-flow                # 可序列化 DAG/Flow runtime（root）+ /v2 typed-graph 引擎
├── llm-agent-builtin             # 开箱即用的 agent Tool
├── llm-agent-policy              # ChatModel policy 装饰器
├── llm-agent-comm                # inter-agent 通信：base + A2A + MCP
├── llm-agent-memory              # durable memory 抽象（/v2 模块，contract-backed）
├── llm-agent-memory-contract     # backend-neutral 的 durable 契约
├── llm-agent-memory-postgres     # durable memory 的 Postgres 后端
├── llm-agent-memory-gateway      # durable memory 的 HTTP 网关
├── llm-agent-memory-worker       # 异步 consolidation worker
├── llm-agent-memory-client       # 网关的 stdlib-only HTTP 客户端
│
│   # 应用与案例服务（消费框架家族）
├── llm-agent-authz               # 可导入的多租户 authz 库
├── llm-agent-kb                  # 企业级 GraphRAG 知识库平台
├── llm-agent-studio              # AI Studio — 可视化工作流编排
└── llm-agent-console             # 统一运维控制台（HTTP-only BFF）
```

根仓入口可参考：

- [`../README.md`](../README.md)
- [`../Makefile`](../Makefile)
- [`../scripts/eco.sh`](../scripts/eco.sh)
- [`../scripts/workspace.sh`](../scripts/workspace.sh)

---

## 2. 生态分层

整个生态更适合按 4 层理解：

```text
生态协调层
  llm-agent-ecosystem(root)

契约层
  llm-agent-contract          (stdlib-only 的 LLM-provider seam)
  llm-agent-memory-contract   (stdlib-only 的 durable-memory seam)

核心能力层
  llm-agent                   (只 require contract)
  llm-agent-rag
  llm-agent-memory (/v2)      (contract-backed；不再依赖 core)

基础设施层
  llm-agent-providers
  llm-agent-otel
  llm-agent-flow (root + /v2)
  llm-agent-builtin / llm-agent-policy / llm-agent-comm
  llm-agent-memory-postgres
  llm-agent-memory-gateway
  llm-agent-memory-worker
  llm-agent-memory-client

应用层
  llm-agent-customer-support
  llm-agent-authz / llm-agent-kb / llm-agent-studio / llm-agent-console
```

### 2.1 依赖方向

当前实际依赖方向（对照各仓 `go.mod` 直接 require；权威依赖图见根
[`README.md`](../README.md) 的「Dependency direction」节）可压缩成：

```text
llm-agent-customer-support
  -> llm-agent
  -> llm-agent-contract
  -> llm-agent-providers
  -> llm-agent-otel
  -> llm-agent-flow
  -> llm-agent-rag

llm-agent-otel
  -> llm-agent
  -> llm-agent-contract
  -> llm-agent-rag
  -> llm-agent-flow (+ /v2)

llm-agent-providers
  -> llm-agent-contract          (现在只 require contract；llm-agent 边已删)

llm-agent-flow
  -> llm-agent
  -> llm-agent-flow/v2 (+ llm-agent-contract，间接)

llm-agent
  -> llm-agent-contract          (唯一 require；core + contract = stdlib-only 闭包)

llm-agent-contract
  -> (无 —— 纯 stdlib)

llm-agent-rag
  -> llm-agent-contract          (可选的 adapter/llmagent 子包会另外拉 llm-agent)

llm-agent-memory (/v2)
  -> llm-agent-contract          (contract-backed；/v2 倒置已删掉 llm-agent 边)

llm-agent-memory-postgres
  -> llm-agent-memory-contract

llm-agent-memory-gateway
  -> llm-agent-memory-contract
  -> llm-agent-memory-postgres
  -> llm-agent-rag

llm-agent-memory-worker
  -> llm-agent-memory-contract
  -> llm-agent-memory-postgres
```

### 2.2 核心设计原则

- `llm-agent` 保持 stdlib-only：唯一 `require` 是 `llm-agent-contract`（本身
  stdlib-only），因此「core + contract」是一个 stdlib-only 闭包。无第三方依赖，
  也不再有 RAG facade 回接边（P0-2 于 2026-05-21 移除）。
- LLM-provider 契约（`ChatModel` + capability 接口）落在独立的
  `llm-agent-contract` 模块；providers、otel、rag、core 都对齐到它。
- 模型能力采用“最小接口 + 可选 capability”设计。
- OTel 一律通过 decorator 注入，不侵入核心抽象。
- `llm-agent-rag` 是 RAG 固定点，承担下游兼容性锚点角色。
- durable memory 采用 truth source + outbox + relay + projection 架构，memory
  集群已 contract-backed（`llm-agent-memory-contract`），`/v2` 倒置移除了对
  core 的旧依赖。
- `flow` 与 `StateGraph` 分工明确：前者是持久化 DAG runtime（root 模块），并额外
  提供带 streaming 与 checkpoint/resume 的 typed-graph `/v2` 引擎；后者是 core
  内的进程内状态机。

---

## 3. 各子项目分别实现了什么

### 3.1 `llm-agent`

核心 Go agent 框架，负责定义：

- `Agent` 抽象
- 五种 agent 范式
- 工具系统与工具执行
- `llm.ChatModel` 及相关能力接口
- `StateGraph`、pipeline、fan-out/fan-in 等控制流原语
- budget / policy 等横切治理能力

关键文件：

- [`../llm-agent/doc.go`](../llm-agent/doc.go)
- [`../llm-agent/llm/chatmodel.go`](../llm-agent/llm/chatmodel.go)
- [`../llm-agent/agent_chatmodel.go`](../llm-agent/agent_chatmodel.go)
- [`../llm-agent/orchestrate/graph.go`](../llm-agent/orchestrate/graph.go)

### 3.2 `llm-agent-rag`

独立 RAG SDK，负责定义：

- 文档 ingest
- embedding seam
- store seam
- retrieval 策略
- `rag.System`
- GraphRAG / AskGlobal / AskDrift
- diagnostics、trace、评测能力

关键文件：

- [`../llm-agent-rag/rag/system.go`](../llm-agent-rag/rag/system.go)
- [`../llm-agent-rag/rag/ask.go`](../llm-agent-rag/rag/ask.go)
- [`../llm-agent-rag/retrieve/retrieve.go`](../llm-agent-rag/retrieve/retrieve.go)
- [`../llm-agent-rag/store/store.go`](../llm-agent-rag/store/store.go)

### 3.3 `llm-agent-providers`

模型供应商适配层，负责把具体厂商 API 接入 `llm-agent-contract` 定义的
`ChatModel` / capability 抽象（唯一 require；早期的 `llm-agent` 边已删）。

当前覆盖：

- OpenAI
- Anthropic
- Ollama
- DeepSeek
- MiniMax

关键文件：

- [`../llm-agent-providers/openai/openai.go`](../llm-agent-providers/openai/openai.go)
- [`../llm-agent-providers/anthropic/anthropic.go`](../llm-agent-providers/anthropic/anthropic.go)
- [`../llm-agent-providers/ollama/ollama.go`](../llm-agent-providers/ollama/ollama.go)
- [`../llm-agent-providers/internal/contract/contract.go`](../llm-agent-providers/internal/contract/contract.go)

### 3.4 `llm-agent-otel`

OpenTelemetry 装饰层，负责给：

- `llm.ChatModel`
- `agents.Agent`
- `rag.System`
- `flow.Runner`

注入 tracing、metrics 和日志桥接。

关键文件：

- [`../llm-agent-otel/otelmodel/otelmodel.go`](../llm-agent-otel/otelmodel/otelmodel.go)
- [`../llm-agent-otel/otelagent/otelagent.go`](../llm-agent-otel/otelagent/otelagent.go)
- [`../llm-agent-otel/otelrag/otelrag.go`](../llm-agent-otel/otelrag/otelrag.go)
- [`../llm-agent-otel/otelflow/otelflow.go`](../llm-agent-otel/otelflow/otelflow.go)

### 3.5 `llm-agent-customer-support`

生态中的参考业务应用，负责把 agent、provider、knowledge、session、guardrails、limits、OTel 装配成一个可运行客服服务。仓内已经有 `flowrunner` 作为对 `llm-agent-flow` 的 bridge，但当前公开 HTTP surface 仍未接入 `/flow/run` 之类的 flow 端点。

对外接口：

- `POST /chat`
- `POST /chat/stream`
- `GET /healthz`
- `GET /readyz`

关键文件：

- [`../llm-agent-customer-support/internal/app/app.go`](../llm-agent-customer-support/internal/app/app.go)
- [`../llm-agent-customer-support/internal/supportflow/supportflow.go`](../llm-agent-customer-support/internal/supportflow/supportflow.go)
- [`../llm-agent-customer-support/internal/httpapi/httpapi.go`](../llm-agent-customer-support/internal/httpapi/httpapi.go)

### 3.6 `llm-agent-flow`

可序列化 DAG/Flow runtime。稳定的 root 模块（v0.2.0）负责：

- JSON Flow IR
- DAG validate / compile / run
- topological-layer 并发执行
- event stream
- `flowd` 持久化 HTTP 服务

此外还提供 typed-graph `/v2` 引擎（v2.2.0），补齐 streaming、checkpoint/resume、
Loop 构造与 durable run 状态——作为独立 `/v2` 模块路径与 root 并存的后继 runtime。

关键文件：

- [`../llm-agent-flow/flow/ir.go`](../llm-agent-flow/flow/ir.go)
- [`../llm-agent-flow/flow/engine.go`](../llm-agent-flow/flow/engine.go)
- [`../llm-agent-flow/cmd/flowd/server/server.go`](../llm-agent-flow/cmd/flowd/server/server.go)

### 3.7 `llm-agent-memory`

durable memory 抽象 + manager 层，现已是 `/v2` 模块（v2.0.0）且 contract-backed：
只依赖 `llm-agent-contract`，不再依赖 `llm-agent` core（`/v2` 倒置已移除该边）。负责：

- capability-interface 化 manager
- unified search
- recall engine
- consolidator
- scoped lifecycle
- durable abstraction

关键文件：

- [`../llm-agent-memory/memory/manager.go`](../llm-agent-memory/memory/manager.go)
- [`../llm-agent-memory/memory/recall_engine.go`](../llm-agent-memory/memory/recall_engine.go)
- [`../llm-agent-memory/memory/consolidator.go`](../llm-agent-memory/memory/consolidator.go)

### 3.8 `llm-agent-memory-postgres`

durable memory 的 Postgres 后端，负责：

- schema/migration
- `memory_record`
- `memory_event`
- `outbox_event`
- idempotency
- leased relay

关键文件：

- [`../llm-agent-memory-postgres/postgres/store.go`](../llm-agent-memory-postgres/postgres/store.go)
- [`../llm-agent-memory-postgres/postgres/relay.go`](../llm-agent-memory-postgres/postgres/relay.go)
- [`../llm-agent-memory-postgres/postgres/schema.go`](../llm-agent-memory-postgres/postgres/schema.go)

### 3.9 `llm-agent-memory-gateway`

durable memory 的治理型 HTTP 网关，负责：

- authoritative scope
- recall cache
- consistency 语义
- session lifecycle
- trace + metrics
- outbox vector projection

关键文件：

- [`../llm-agent-memory-gateway/internal/service/service.go`](../llm-agent-memory-gateway/internal/service/service.go)
- [`../llm-agent-memory-gateway/internal/service/hybrid_recaller.go`](../llm-agent-memory-gateway/internal/service/hybrid_recaller.go)
- [`../llm-agent-memory-gateway/internal/service/recall_cache.go`](../llm-agent-memory-gateway/internal/service/recall_cache.go)
- [`../llm-agent-memory-gateway/internal/transport/router.go`](../llm-agent-memory-gateway/internal/transport/router.go)

---

## 4. 30 分钟读码路径

### 第 1 步：先看边界

读：

- [`../README.md`](../README.md)

目标：

- 理解这不是单仓产品
- 知道约 20 个子项目的职责边界（roster 见根 README）

### 第 2 步：看核心抽象

读：

- [`../llm-agent/doc.go`](../llm-agent/doc.go)
- [`../llm-agent/llm/chatmodel.go`](../llm-agent/llm/chatmodel.go)
- [`../llm-agent/agent_chatmodel.go`](../llm-agent/agent_chatmodel.go)
- [`../llm-agent/orchestrate/graph.go`](../llm-agent/orchestrate/graph.go)

目标：

- 理解 Agent、ChatModel、StateGraph 这三个基础抽象
- 理解 budget 等横切能力如何挂接

### 第 3 步：看 RAG 平台

读：

- [`../llm-agent-rag/rag/system.go`](../llm-agent-rag/rag/system.go)
- [`../llm-agent-rag/rag/ask.go`](../llm-agent-rag/rag/ask.go)
- [`../llm-agent-rag/retrieve/retrieve.go`](../llm-agent-rag/retrieve/retrieve.go)
- [`../llm-agent-rag/store/store.go`](../llm-agent-rag/store/store.go)

目标：

- 理解 `rag.System` 的三条答题路径
- 理解 retrieval/store 的可插拔设计

### 第 4 步：看两个主运行时

读：

- [`../llm-agent-customer-support/internal/app/app.go`](../llm-agent-customer-support/internal/app/app.go)
- [`../llm-agent-memory-gateway/internal/service/service.go`](../llm-agent-memory-gateway/internal/service/service.go)

目标：

- 理解一个应用型服务怎么装配
- 理解一个治理型服务怎么处理状态和一致性

### 第 5 步：看 workflow runtime

读：

- [`../llm-agent-flow/flow/ir.go`](../llm-agent-flow/flow/ir.go)
- [`../llm-agent-flow/flow/engine.go`](../llm-agent-flow/flow/engine.go)
- [`../llm-agent-flow/cmd/flowd/server/server.go`](../llm-agent-flow/cmd/flowd/server/server.go)

目标：

- 理解 `flow` 和 `StateGraph` 的区别
- 理解 `flowd` 为什么是独立运行时

---

## 5. `customer-support` 调用链

### 5.1 总装入口

`customer-support` 的装配中心是：

- [`../llm-agent-customer-support/internal/app/app.go`](../llm-agent-customer-support/internal/app/app.go)

装配顺序：

```text
config
  -> providers.NewChatModel
  -> providers.NewEmbedder
  -> sessionstore.Open(SQLite/Postgres)
  -> otel.NewTracerProvider
  -> otelmodel.Wrap(model)
  -> knowledgebase.New(embedder)
  -> supportflow.New(model, knowledge, sessions, guardrails)
  -> limits.New(...)
  -> guard.WrapAgent(agent)
  -> otelagent.Wrap(agent)
  -> httpapi.NewMux(...)
```

### 5.2 HTTP 层

入口文件：

- [`../llm-agent-customer-support/internal/httpapi/httpapi.go`](../llm-agent-customer-support/internal/httpapi/httpapi.go)

`POST /chat` 路径：

```text
decodeChatRequest
  -> withRequestSession
  -> preflightGuard
  -> agent.Run
  -> writeJSON(ChatResponse)
```

`POST /chat/stream` 路径：

```text
decodeChatRequest
  -> withRequestSession
  -> preflightGuard
  -> agent.RunStream
  -> SSE step events
  -> SSE done/error
```

### 5.3 supportflow 状态图

核心文件：

- [`../llm-agent-customer-support/internal/supportflow/supportflow.go`](../llm-agent-customer-support/internal/supportflow/supportflow.go)

状态图：

```text
load-session
  -> classify
     -> handover-human
     -> request-more-info
     -> self-service
```

节点职责：

- `load-session`
  - 从 `sessionstore` 读取历史
  - 把历史和当前问题拼接成上下文
- `classify`
  - 提取订单号
  - `chargeback/fraud` 走人工
  - 缺少订单号走补信息
- `request-more-info`
  - 直接返回请求补充订单号
- `handover-human`
  - 直接转人工
- `self-service`
  - 调工具型 agent 处理退款政策问题

### 5.4 toolAgent

核心文件：

- [`../llm-agent-customer-support/internal/supportflow/toolagent.go`](../llm-agent-customer-support/internal/supportflow/toolagent.go)

调用链：

```text
model.WithTools(registry.AsLLMTools())
  -> Generate(prompt)
  -> if no tool calls:
       return resp.Text
  -> else:
       AsyncRunner.Execute(tasks)
       build action/observation steps
       final answer = concatenated tool outputs
```

这里当前没有“工具调用后再次回模型总结”的第二轮，因此它是一个很薄的 function-calling 流程。

### 5.5 knowledgebase / limits / guardrails

相关文件：

- [`../llm-agent-customer-support/internal/knowledgebase/knowledgebase.go`](../llm-agent-customer-support/internal/knowledgebase/knowledgebase.go)
- [`../llm-agent-customer-support/internal/limits/limits.go`](../llm-agent-customer-support/internal/limits/limits.go)
- [`../llm-agent-customer-support/internal/guardrails/guardrails.go`](../llm-agent-customer-support/internal/guardrails/guardrails.go)

说明：

- `knowledgebase` 用 `rag.System + InMemoryStore` 在启动时 seed 极小规模退款政策数据。
- `limits` 在请求前做限流、token 预算和 panic switch，在请求后做 tool loop 和累计 token 校验。
- `guardrails` 做简化版 prompt injection 检测，并对 system prompt 注入“检索内容不可信”的前缀。

---

## 6. `memory-gateway` 调用链

### 6.1 写入路径

入口文件：

- [`../llm-agent-memory-gateway/internal/service/service.go`](../llm-agent-memory-gateway/internal/service/service.go)

通用写路径：

```text
transport/router
  -> readAuthoritativeScope(headers)
  -> service.Validate
  -> merge auth scope + body scope
  -> backend.WriteRecord / Patch / Pin / Disable / Delete
  -> invalidateScopeState(scope)
  -> return memory_id/version/status
```

`invalidateScopeState(scope)` 的作用是：

- recall cache 失效
- scope version bump

### 6.2 durable truth source

底层文件：

- [`../llm-agent-memory-postgres/postgres/store.go`](../llm-agent-memory-postgres/postgres/store.go)

一次 `WriteRecord` 事务内会写：

```text
memory_record
memory_event
outbox_event(status=pending)
idempotency snapshot
```

这说明 durable memory 不是简单 CRUD，而是“当前状态 + 事件流 + 出站投影 + 幂等”的组合。

### 6.3 recall 路径

`RecallUnified` 主链：

```text
validate query/top_k
  -> merge scope
  -> sessionRegistry.Get + validate session state
  -> scopeVersionStore.CurrentScopeVersion
  -> try recallCache.Lookup
  -> if miss:
       recaller.Recall(scope, query, topK)
       token budget filter
       emit trace + metrics
       cache fill
       return hits
```

### 6.4 consistency 语义

缓存文件：

- [`../llm-agent-memory-gateway/internal/service/recall_cache.go`](../llm-agent-memory-gateway/internal/service/recall_cache.go)

scope version 文件：

- [`../llm-agent-memory-gateway/internal/service/scope_version_store.go`](../llm-agent-memory-gateway/internal/service/scope_version_store.go)

三种 consistency：

- `strong`
  - 完全绕过缓存
- `eventual`
  - 30 秒 TTL
  - 可选 stale 返回
- `bounded`
  - 5 秒 TTL
  - scope version 必须匹配
  - record version 也必须仍与 truth source 一致

### 6.5 session lifecycle

文件：

- [`../llm-agent-memory-gateway/internal/service/session_registry.go`](../llm-agent-memory-gateway/internal/service/session_registry.go)

网关用 session registry 维护：

- `active`
- `closed`
- `LastHeartbeatAt`
- `ClosedAt`

召回前会校验：

- 是否已关闭
- 是否 idle 超时
- 是否允许 heartbeat 刷新

### 6.6 hybrid recall

文件：

- [`../llm-agent-memory-gateway/internal/service/hybrid_recaller.go`](../llm-agent-memory-gateway/internal/service/hybrid_recaller.go)

流程：

```text
RecallCandidateSource[]
  -> candidate memory ids + scores
  -> merge by best score
  -> rank topK
  -> hydrator.HydrateRecords from truth source
  -> return records
```

向量源和词法源都只是候选源，最终返回内容必须经过 truth source hydrate。

### 6.7 relay 和 vector projection

网关侧 publisher：

- [`../llm-agent-memory-gateway/internal/service/outbox_vector_publisher.go`](../llm-agent-memory-gateway/internal/service/outbox_vector_publisher.go)
- [`../llm-agent-memory-gateway/internal/service/vector_projector.go`](../llm-agent-memory-gateway/internal/service/vector_projector.go)

后端 relay：

- [`../llm-agent-memory-postgres/postgres/relay.go`](../llm-agent-memory-postgres/postgres/relay.go)

链路：

```text
outbox_event(status=pending)
  -> Relay.ClaimBatch (lease + attempt_count++)
  -> OutboxVectorPublisher.Publish(msg)
  -> reread current record from truth source
  -> version match ? ProjectUpsert/ProjectRemove : stale skip
  -> Ack(sent | pending | failed)
```

这是一条标准的 at-least-once leased outbox 投影链路。

---

## 7. 总结

这个项目的本质不是一个单独的 AI 应用，而是一整套 Go 语言 LLM Agent 技术栈：

- agent framework
- RAG platform
- provider adapters
- observability layer
- workflow runtime
- durable memory subsystem
- reference application

如果把全项目压缩成一句话：

> `llm-agent` 定义抽象，`llm-agent-rag` 定义知识和回答平台，`flow` 定义可持久化工作流运行时，`memory-*` 定义 durable memory 子系统，`customer-support` 则把这些能力装配成一个可运行参考服务。

---

## 延伸阅读

- [`./architecture-and-sequence-diagrams.zh-CN.md`](./architecture-and-sequence-diagrams.zh-CN.md)
- [`./source-design-umbrella-root.zh-CN.md`](./source-design-umbrella-root.zh-CN.md)
- [`./source-design-llm-agent.zh-CN.md`](./source-design-llm-agent.zh-CN.md)
- [`./source-design-llm-agent-rag.zh-CN.md`](./source-design-llm-agent-rag.zh-CN.md)
- [`./source-design-llm-agent-flow.zh-CN.md`](./source-design-llm-agent-flow.zh-CN.md)
- [`./source-design-llm-agent-customer-support.zh-CN.md`](./source-design-llm-agent-customer-support.zh-CN.md)
- [`./multi-service-memory-architecture.zh-CN.md`](./multi-service-memory-architecture.zh-CN.md)
