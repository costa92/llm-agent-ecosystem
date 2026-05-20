# 当前项目分析

## 概览

这个仓库是 `llm-agent` 生态的总工作区。根仓本身不是产品运行时，也
不承载统一的业务逻辑，它的职责主要是：

- 为整个生态提供统一的本地工作区
- 维护共享约定和依赖方向
- 承担跨仓规划与发布协同

真正的产品能力和库能力分布在 5 个独立子项目中：

- `llm-agent`
- `llm-agent-rag`
- `llm-agent-providers`
- `llm-agent-otel`
- `llm-agent-customer-support`

## 生态结构

### 根仓职责

从目录和文档来看，根仓当前主要负责：

- 在 `README.md` 中提供总览和导航
- 在 `PROJECT.md` 中定义边界和范围
- 在 `.planning/` 中维护生态级规划
- 通过 `Makefile`、`scripts/eco.sh`、`scripts/workspace.sh`
  提供工作区和 bootstrap 能力

根仓本身刻意保持轻量。它没有统一的应用入口、没有公共 API 服务，也
没有把所有实现揉进一个 monorepo 包结构中。

### 依赖方向

当前实际依赖关系是：

- `llm-agent-customer-support` 依赖 `llm-agent`、
  `llm-agent-providers` 和 `llm-agent-otel`
- `llm-agent-otel` 依赖 `llm-agent` 和 `llm-agent-rag`
- `llm-agent-providers` 依赖 `llm-agent`
- `llm-agent` 只通过 RAG facade 依赖 `llm-agent-rag`
- `llm-agent-rag` 位于整个栈的最底层

这形成了一个清晰的分层架构：

1. 基础检索层：`llm-agent-rag`
2. 核心 agent 抽象层：`llm-agent`
3. 集成层：`llm-agent-providers`、`llm-agent-otel`
4. 参考应用层：`llm-agent-customer-support`

## 子项目分析

## `llm-agent`

### 角色定位

`llm-agent` 是核心 Go agent 框架，负责提供整个生态中的抽象、运行模
型和编排能力。

### 对外能力

结合 `README.md`、`doc.go` 和目录结构，当前已经实现：

- 五类 agent 范式：
  - `SimpleAgent`
  - `ReActAgent`
  - `ReflectionAgent`
  - `PlanAndSolveAgent`
  - `FunctionCallAgent`
- 工具抽象与注册表
- 串行和异步工具执行
- `llm/` 下的模型抽象
- Memory 能力
- Context engineering 能力
- Multi-agent orchestration
- 轻量通信协议能力
- 评测与 benchmark 能力
- 可运行示例

### 目录职责

- `llm/`：模型契约、能力声明、请求响应类型、流式事件、模型元信息、
  脚本化测试模型
- `builtin/`：内置工具，如 calculator、search、note、terminal
- `memory/`：working、episodic、semantic memory 及相关工具
- `context/`：上下文构建、筛选、压缩、token 感知处理
- `comm/`：信封结构与 HTTP、stdio、in-memory 传输
- `comm/mcp`、`comm/a2a`、`comm/anp`：偏协议层的通信实现
- `orchestrate/`：pipeline、fan-out/fan-in、round-robin、roleplay、
  state-graph 等编排能力
- `rl/`：Agentic RL 相关评估和 trainer proxy 抽象
- `bench/`：BFCL、GAIA、judge、win-rate 等基准能力
- `budget/`：运行时预算和配额控制
- `examples/`：主要使用模式的 runnable 示例

### 架构判断

这个项目不只是一个“agent runner”，而是一个更完整的 agent 构建框架，
核心关注点包括：

- 模型抽象
- 工具执行
- 编排能力
- 评测能力

所以它更像一个面向框架使用者的基础设施项目，而不是单一业务应用。

### 测试与成熟度信号

较强信号：

- 几乎所有核心目录都有包级测试
- 有覆盖典型场景的 example tests
- 有 API snapshot 和迁移文档
- 有明确的 stdlib-only 和兼容性约束

当前成熟度判断：

- 核心框架能力已经比较完整，测试面也较广
- 仍处于持续演进中的框架阶段，不是完全冻结的平台
- 适合做下游组合、内部试用和参考实现

## `llm-agent-rag`

### 角色定位

`llm-agent-rag` 是独立的 RAG SDK。它不是 `llm-agent` 内部的一个小包，
而是被刻意拆成了单独的底层检索库。

### 对外能力

从 `README.md` 和目录结构判断，目前已经实现：

- 面向抽象 source 的文档导入
- 文档与 Markdown 的确定性切分
- embedding seam 和默认 hash embedder
- 向量存储 seam 与内存实现
- PostgreSQL + pgvector 后端
- 检索与混合检索
- rerank
- token budget 感知的上下文打包
- prompt template 能力
- 基于调用方模型接口的答案生成
- graph / route 感知的检索能力
- 检索评测框架
- 回接 `llm-agent` 的可选 adapter

### 目录职责

- `ingest/`：文档、source、导入流程、splitter
- `embed/`：embedding 抽象和默认 hash embedder
- `store/`：向量存储抽象和内存实现
- `store/storetest/`：后端一致性测试契约
- `postgres/`：PostgreSQL/pgvector 后端
- `retrieve/`：排序检索、多跳检索、图相关检索
- `rerank/`：启发式和 HTTP 模型驱动的 rerank
- `pack/`：token 预算感知的上下文打包
- `prompt/`：prompt 模板和接口
- `generate/`：生成模型 seam
- `rag/`：对外统一暴露 import、retrieve、ask 的编排层
- `graph/`：图抽取、路径、社区发现、摘要
- `tree/`：结构化文档树
- `eval/`：检索质量与 grounding 相关评测
- `guard/`：注入与脱敏相关能力
- `obs/`：观测钩子
- `feedback/`：反馈数据结构
- `adapter/llmagent/`：对 `llm-agent` 的可选适配层

### 架构判断

这个子项目已经明显不止是“基础 RAG”：

- 有经典 chunk retrieval
- 有 GraphRAG / route-aware retrieval
- 有评测和安全辅助能力

它更接近一个检索平台 SDK，而不是简单工具库。

### 测试与成熟度信号

较强信号：

- 检索、图、存储、评测等层都有较完整测试
- 有 API snapshot 和兼容性文档
- 有后端 conformance 测试
- README 明确以 `v1.0` 稳定线定位

当前成熟度判断：

- 是整个生态里最稳定、最强调兼容性的底层库
- 明显被设计成下游项目的兼容性锚点
- 适合作为统一的检索与知识层基础设施

## `llm-agent-providers`

### 角色定位

`llm-agent-providers` 负责模型供应商适配，把具体厂商 API 转成
`llm-agent/llm` 所需的统一接口。

### 对外能力

当前已经覆盖的 provider：

- OpenAI
- Anthropic
- Ollama
- DeepSeek
- MiniMax

这些 provider 组合实现了：

- 同步生成
- 流式生成
- 原生 tool/function calling
- provider 支持范围内的 embedding 能力
- 按模型真实暴露 capabilities

### 目录职责

- `openai/`：OpenAI 适配器、映射层、选项、测试
- `anthropic/`：Anthropic 适配器和映射层
- `ollama/`：Ollama 适配器、embedding 策略、tool 策略
- `deepseek/`：DeepSeek 适配器与区域配置
- `minimax/`：MiniMax 适配器与区域配置
- `internal/contract/`：跨 provider 的契约一致性测试
- `scripts/`：fixture 抓取和 workspace 辅助脚本

### 架构判断

这个仓库本质上是抽象框架与真实 LLM 厂商之间的兼容层，设计重点在于：

- 每个 provider 相互隔离
- 共享能力契约
- 按模型真实声明能力边界

它并没有强行抹平所有 provider 差异，而是尽量在统一接口下保留厂商特
性。

### 测试与成熟度信号

较强信号：

- 每个 provider 都有独立测试
- `internal/contract` 提供共享契约覆盖
- fixture 抓取脚本说明已有可重复验证思路

当前成熟度判断：

- 是一个边界清晰、模块化程度不错的集成层
- 已经适合做真实下游接入
- 主要风险更可能来自外部 provider API 演进，而不是仓内结构

## `llm-agent-otel`

### 角色定位

`llm-agent-otel` 负责在不破坏 core stdlib-only 约束的前提下，为整个
生态补充 OpenTelemetry 可观测性能力。

### 对外能力

当前能力面包括：

- 通过 `otelmodel` 对模型进行埋点
- 通过 `otelagent` 对 agent 进行埋点
- 通过 `otelrag` 对 RAG 流程埋点
- 低基数 metrics 辅助能力
- slog bridge
- OTLP HTTP 和 gRPC exporter 装配
- 本地观测 demo compose

### 目录职责

- `otelmodel/`：包装 `llm.ChatModel`，并尽量保留原能力接口
- `otelagent/`：包装 `agents.Agent`
- `otelrag/`：包装检索/RAG 流程
- `otelmetrics/`：metrics 辅助和约定
- `otelslog/`：slog handler bridge
- 根目录 exporter 文件：统一 exporter 构建与默认配置
- `compose/`：本地观测 demo 栈
- `cmd/tailprobe/`：与 trace 采样/测试相关的小工具

### 架构判断

这个子项目是一个干净的 decorator 层，把观测逻辑放在外围，而不是侵入
核心框架。这种拆分带来几个好处：

- 保留 `llm-agent` 的 stdlib-only 承诺
- 让 telemetry 变成可选能力
- 让观测层可以独立演进和发布

### 测试与成熟度信号

较强信号：

- 每个 wrapper 包都有对应测试
- exporter 和语义约定辅助能力也有测试
- 带有端到端可观察的本地 demo 栈

当前成熟度判断：

- 是一个边界非常清晰的基础设施组件
- 对本地 demo 和集成环境都比较实用
- 对已有 OTel 体系的场景具备较高接入价值

## `llm-agent-customer-support`

### 角色定位

`llm-agent-customer-support` 是整个生态里的参考应用，用来展示框架、
provider、RAG 和 observability 如何在一个服务中组合起来。

### 对外能力

目前对外暴露的服务能力包括：

- HTTP chat API
- SSE chat streaming
- 健康检查和 readiness probe
- 聊天模型与 embedding 模型独立切换
- 基于 RAG 的客服知识检索
- 基于 state-graph 的客服分流
- tool 驱动的工作流
- SQLite 或 Postgres 的会话持久化
- 运行时限额与 panic switch
- prompt injection 和不可信检索内容防护
- 带 Grafana 的本地 OpenTelemetry 观测栈

### 目录职责

- `cmd/server/`：服务入口和进程生命周期
- `internal/app/`：组合根，负责启动装配、模型和 session 初始化
- `internal/config/`：环境变量解析和运行配置
- `internal/providers/`：chat/embedding provider factory
- `internal/httpapi/`：REST/SSE 传输层和响应行为
- `internal/knowledgebase/`：种子知识库处理
- `internal/supportflow/`：分流、路由、工具、客服编排逻辑
- `internal/sessionstore/`：持久会话抽象和实现
- `internal/limits/`：限额、预算、panic-switch 控制
- `internal/guardrails/`：可疑输入处理与 prompt 安全策略
- `compose/`：本地 demo 运行栈
- `dashboards/`：Grafana 仪表盘资产

### 架构判断

这是整个生态里最接近最终产品形态的仓库，也是各底层组件的集成验证场。

它体现的是一个比较标准的服务结构：

- 配置层
- 组合根
- 传输层
- 领域流程层
- 持久化层
- 运行安全层

不过它的业务范围是刻意收窄的，主要聚焦客服分流和客服问答，因此更适
合作为参考实现，而不是通用 SaaS 产品。

### 测试与成熟度信号

较强信号：

- app wiring、config、HTTP API、guardrails、limits、knowledgebase、
  session store、support flow 都有测试
- 有 docker-compose demo 栈和 dashboard 资产
- README 明确写出了 day-one guardrails

当前成熟度判断：

- 是生态中最能体现“完整组合”的仓库
- 服务形态已经比较生产化，但 hardening 仍偏 demo/reference 级别
- 最适合拿来理解“整个生态该如何落地使用”

## 跨项目结论

### 当前已经落地的能力

从生态视角看，这个项目当前已经具备：

- 可复用的 Go agent 框架
- 独立的 RAG 平台 SDK
- 多模型 provider 适配层
- 可选的 OpenTelemetry 可观测性层
- 一个可运行的客服参考服务

这已经是一个真实的分层系统，不是停留在概念阶段的仓库。

### 当前架构的优化方向

整个代码库在持续优化这些目标：

- 按仓库隔离关注点
- 将基础设施依赖做成可选项
- 用抽象降低对单一厂商的绑定
- 通过 workspace 工具支持多仓本地联调
- 保持 core 轻量，把集成能力外移

### 各层成熟度判断

- 最稳定：`llm-agent-rag`
- 最基础：`llm-agent`
- 集成复杂度最高：`llm-agent-providers`
- 最偏基础设施：`llm-agent-otel`
- 最像产品：`llm-agent-customer-support`

### 当前结构上可见的主要限制

- 根仓本身不是统一产品运行时
- 整个生态依赖多仓协同，而不是单仓开发模型
- 某些能力仍明确处于 demo 或 pre-release 语境
- 运行时 hardening 最明显地体现在参考应用中，不代表整个栈都已同等成熟

## 建议阅读顺序

对新维护者或评估者，更高效的阅读顺序是：

1. 根仓 `README.md`
2. `llm-agent/README.md`
3. `llm-agent-rag/README.md`
4. `llm-agent-customer-support/README.md`
5. `llm-agent-providers/README.md`
6. `llm-agent-otel/README.md`

这个顺序对应的是理解栈的自然路径：先看框架，再看检索，再看参考应用，
最后看外部集成和可观测性。

## 总结

当前项目实现的不是单一产品，而是一整套用于构建、接入、观测 Go 版
LLM agents 的生态。

它的核心价值在于把下面几层拼成了一套完整方案：

- 轻量 agent 框架
- 独立检索平台
- provider 适配层
- observability 包装层
- 能证明整体可落地的参考应用

从当前目录结构、文档和测试分布来看，这个生态已经具备较强的功能完整
性和明确的分层设计，其中 `llm-agent-customer-support` 是最具体的端到
端落地示例。
