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
- 适用场景：把真实模型接入 `llm-agent`

## 4. `llm-agent-otel`

- 角色：观测层
- 关键词：decorator、span、gen_ai semconv、OTLP
- 设计重点：不侵入核心，通过 wrap 保留原始能力
- 适用场景：需要端到端 tracing/metrics 的服务

## 5. `llm-agent-flow`

- 角色：外部化流程编排层
- 关键词：Flow IR、DAG、Runner、Store、flowd、replay
- 设计重点：可序列化、可审计、可回放、可服务化
- 适用场景：需要把流程持久化、配置化、运维化

## 6. `llm-agent-customer-support`

- 角色：参考应用层
- 关键词：supportflow、sessionstore、limits、httpapi、flowrunner
- 设计重点：把生态能力拼成一个可运行产品
- 适用场景：对外演示、集成样例、启动模板

## 7. 推荐选型原则

- 只想在 Go 代码里快速组装多 Agent：优先 `llm-agent/orchestrate`
- 需要把流程配置化、存储、回放：优先 `llm-agent-flow`
- 只需要模型抽象，不想引入重依赖：只用 `llm-agent`
- 需要真实模型：加 `llm-agent-providers`
- 需要知识库问答：加 `llm-agent-rag`
- 需要 tracing：加 `llm-agent-otel`
- 需要端到端参考实现：看 `llm-agent-customer-support`
