# 子系统设计要点速记

## 1. `llm-agent`

- 角色：核心抽象层
- 关键词：`Agent`、`Tool`、`Registry`、`ChatModel`、`orchestrate`、`budget`、`policy`
- 设计重点：最小接口、能力协商、可组合编排、可流式追踪
- 适用场景：写 Go 代码直接组装 Agent 系统

## 2. `llm-agent-rag`

- 角色：知识与检索层
- 关键词：`ingest`、`embed`、`store`、`retrieve`、`rerank`、`pack`、`generate`
- 设计重点：seam-first、可替换 pipeline、GraphRAG、诊断信息完整
- 适用场景：构建独立 RAG 系统或作为 Agent 的知识层

## 3. `llm-agent-providers`

- 角色：模型供应商适配层
- 关键词：bound-model、capability matrix、stream mapping、tool calling
- 设计重点：统一 contract，下沉厂商差异
- 适用场景：把真实模型接入 `llm-agent-contract`（唯一 require；`llm-agent` 边已删）

## 4. `llm-agent-otel`

- 角色：观测层
- 关键词：decorator、span、gen_ai semconv、OTLP
- 设计重点：不侵入核心，通过 wrap 保留原始能力
- 适用场景：需要端到端 tracing/metrics 的服务

## 5. `llm-agent-flow`

- 角色：外部化流程编排层
- 关键词：Flow IR、DAG、Runner、Store、flowd、replay；`/v2` typed-graph（streaming、checkpoint/resume、Loop）
- 设计重点：可序列化、可审计、可回放、可服务化；root（v0.2.0）与 `/v2`（v2.2.0）模块并存
- 适用场景：需要把流程持久化、配置化、运维化

## 6. `llm-agent-customer-support`

- 角色：参考应用层
- 关键词：supportflow、sessionstore、limits、httpapi、flowrunner
- 设计重点：把生态能力拼成一个可运行产品
- 适用场景：对外演示、集成样例、启动模板

## 7. 其余框架子系统（速记）

- `llm-agent-contract`：stdlib-only 的 LLM-provider 契约（`ChatModel` + capability 接口、streaming、mocks）。core/providers/otel/rag 都对齐到它。
- `llm-agent-builtin`：开箱即用的 agent `Tool`（calculator/note/search/terminal），只 require contract。
- `llm-agent-policy`：`ChatModel` policy 装饰器（PII 脱敏、注入扫描、长度闸），只 require contract。
- `llm-agent-comm`：inter-agent 通信——transport/envelope base + A2A + MCP（ANP 留在 core），只 require contract。
- memory 家族：`llm-agent-memory`（durable 抽象 + manager，`/v2` contract-backed）、`llm-agent-memory-contract`（backend-neutral durable 契约）、`llm-agent-memory-postgres`（Postgres 后端 + outbox relay）、`llm-agent-memory-gateway`（HTTP 网关 + recall cache + session 生命周期）、`llm-agent-memory-worker`（异步 consolidation）、`llm-agent-memory-client`（stdlib-only HTTP 客户端）。

## 8. 应用与案例服务（消费框架家族）

- `llm-agent-authz`：可导入的多租户 authz 库（org→scope、argon2id、JWT、refresh session、middleware；无 sibling 依赖）。
- `llm-agent-kb`：企业级 GraphRAG 知识库问答平台（Go `kbd` + React SPA，消费 rag/authz/otel/providers/contract）。
- `llm-agent-studio`：AI Studio——多租户可视化工作流编排/内容生产平台（消费 llm-agent/authz/otel/providers/contract）。
- `llm-agent-console`：统一运维控制台——HTTP-only BFF / 反向代理（无 Go 模块边）。

## 9. 推荐选型原则

- 只想在 Go 代码里快速组装多 Agent：优先 `llm-agent/orchestrate`
- 需要把流程配置化、存储、回放：优先 `llm-agent-flow`
- 只需要模型抽象，不想引入重依赖：只用 `llm-agent`
- 需要真实模型：加 `llm-agent-providers`
- 需要知识库问答：加 `llm-agent-rag`
- 需要 tracing：加 `llm-agent-otel`
- 需要端到端参考实现：看 `llm-agent-customer-support`
