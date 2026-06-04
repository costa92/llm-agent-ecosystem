# 中文文档翻译术语表（llm-agent-ecosystem）

- **日期：** 2026-06-04
- **用途：** 为 `llm-agent-ecosystem` 这一「仓库套娃」（16 个兄弟仓、约 68 篇待翻译文档）固定常见英文术语的中文译法，保证跨仓翻译的术语一致性。这是后续所有翻译 agent 必须遵守的术语「法律」。
- **配套计划：** [`docs/superpowers/plans/2026-06-04-ecosystem-subrepo-chinese-docs.md`](../plans/2026-06-04-ecosystem-subrepo-chinese-docs.md)
- **翻译约定：** 英文 `X.md` 保留，新增配对的 `X.zh-CN.md`。

## 首要规则：永不翻译的内容

以下「标识符类」内容在任何语境下 **一律保留原文，绝不翻译、绝不音译**：

- **代码标识符 / API / 类型名 / 函数名 / 方法名 / 字段名 / 接口名** —— 例如 `ChatModel`、`Generate`、`Runner`、`HybridRetriever`、`Options.EntityExtractor`。
- **包名 / 模块名 / 仓库名** —— 例如 `agents`、`rag`、`graph`、`postgres`、`otelmodel`、`github.com/costa92/llm-agent`、`llm-agent-rag`。
- **环境变量 / 配置键** —— 例如 `LLM_AGENT_MEMORY_PG_URL`、`OTEL_EXPORTER_OTLP_ENDPOINT`、`GOWORK=off`。
- **HTTP 端点 / 路由 / 路径** —— 例如 `POST /memory/write`、`/healthz`。
- **URL、文件路径、命令行 / shell 命令** —— 例如 `go test ./...`、`./scripts/workspace.sh`。
- **版本号、Git 分支名 / tag 名** —— 例如 `v0.2.0`、`release/**`、`main`。
- **指标 / span 名 / 属性键** —— 例如 `gen_ai.*`、`rag.generate.tokens`、`recall_l1_hit_total`。

行文规范：术语首次出现时可用「中文（English）」括注；后续按本表统一使用中文；当某术语恰好是上面某个标识符（如包名 `policy`、`rag`），则保留原文。

## 术语表

| 英文 | 中文 | 备注 |
|---|---|---|
| agent | 智能体 | 生态名 `llm-agent` 等、类型名 `agents.Agent` 保留不译 |
| multi-agent | 多智能体 | |
| tool | 工具 | 类型名 `Tool` / 包名 `tools` 不译 |
| tool call | 工具调用 | 类型名 `ToolCall` 不译 |
| function calling | 函数调用 | 范式名 FunctionCall 保留不译 |
| provider | 提供方 / Provider | 包名/仓名 `providers` 不译 |
| contract | 契约 | 包名/仓名 `contract`、`llm-agent-contract` 不译 |
| gateway | 网关 | 仓名 `llm-agent-memory-gateway` 不译 |
| worker | 工作进程 | 仓名/类型名不译 |
| engine | 引擎 | 类型名 `Engine` 不译 |
| runner | 执行器 | 接口名 `Runner` 不译 |
| adapter | 适配器 | 包名 `adapter` 不译 |
| seam | 接缝 | 指可替换的扩展点（如 embedder seam） |
| umbrella | 伞形（工作区） | umbrella repo 译「伞形仓库」 |
| ecosystem | 生态 | 生态名 `llm-agent-ecosystem` 保留不译 |
| polyrepo / multi-repo | 多仓 | |
| sibling repo | 兄弟仓 | sister repo 同译 |
| framework | 框架 | |
| module | 模块 | Go module / `go.mod` 语境保留 module |
| package | 包 | 具体包名不译 |
| stdlib-only | 仅标准库 | 指不引入第三方依赖 |
| dependency | 依赖 | `require` / `replace` 指令保留不译 |
| retriever | 检索器 | 类型名 `*Retriever` 不译 |
| retrieval | 检索 | |
| reranker | 重排器 | |
| rerank | 重排 | 包名 `rerank` 不译 |
| embedder | 嵌入器 | 类型名 `Embedder` / 包名 `embed` 不译 |
| embedding | 嵌入 | |
| chunk | 文本块 | 类型名 `Chunk` 不译 |
| chunking / split | 切块 / 切分 | splitter 译「切分器」 |
| ingest / import | 摄入 / 导入 | 包名 `ingest`、方法 `Import` 不译 |
| corpus | 语料 | |
| namespace | 命名空间 | 字段 `Namespace` 不译 |
| vector store | 向量存储 | 类型名 `Store` 不译 |
| pgvector | pgvector | 保留不译 |
| dense / lexical | 稠密 / 词法 | dense retrieval 译「稠密检索」，lexical 译「词法检索」 |
| hybrid retrieval | 混合检索 | |
| reciprocal rank fusion | 倒数排名融合（RRF） | |
| context packing | 上下文打包 | 包名 `pack` 不译 |
| prompt | 提示词 | 包名 `prompt`、字段 `Prompt` 不译 |
| prompt template | 提示词模板 | 类型名 `Template` 不译 |
| GraphRAG | GraphRAG | 保留不译 |
| knowledge graph | 知识图谱 | |
| entity | 实体 | |
| relation | 关系 | |
| entity extraction | 实体抽取 | 类型名 `EntityExtractor` 不译 |
| entity resolution | 实体消解 | 类型名 `EntityResolver` 不译；fuzzy resolution 译「模糊消解」 |
| community detection | 社区检测 | 类型名 `CommunityDetector` 不译 |
| community summary | 社区摘要 | 类型名 `CommunitySummarizer` / `CommunityReport` 不译 |
| traversal | 遍历 | |
| hop | 跳 | multi-hop 译「多跳」 |
| neighborhood | 邻域 | |
| provenance | 溯源 | provenance chunk 译「溯源文本块」 |
| global search | 全局检索 | 方法 `AskGlobal` 不译 |
| local search | 局部检索 | |
| DRIFT search | DRIFT 检索 | 方法 `AskDrift` 保留不译 |
| map-reduce | map-reduce | 保留不译 |
| reflection | 反思 | Self-RAG 语境；类型名 `ReflectionOptions` 不译 |
| Self-RAG | Self-RAG | 保留不译 |
| grounding / groundedness | 接地性 | grounded 译「有接地（依据）」 |
| query expansion | 查询扩展 | MQE / HyDE 保留不译 |
| recall (检索) | 召回 | recall@k 保留；指标名 `recall_*_total` 不译 |
| precision | 精确率 | precision@k 保留 |
| memory | 记忆 | 仓名 `llm-agent-memory` 不译 |
| working memory | 工作记忆 | 类型名 `WorkingMemory` 不译 |
| episodic memory | 情景记忆 | 类型名 `EpisodicMemory` 不译 |
| semantic memory | 语义记忆 | 类型名 `SemanticMemory` 不译 |
| durable memory | 持久记忆 | |
| recall (记忆) | 唤回 | 记忆语境下的 recall（如 `/memory/recall`）译「唤回」，与检索召回区分 |
| pin / unpin | 钉住 / 取消钉住 | |
| session | 会话 | |
| session lifecycle | 会话生命周期 | |
| heartbeat | 心跳 | |
| idle TTL / TTL | 空闲 TTL / TTL | TTL 保留不译 |
| tenant | 租户 | tenant binding 译「租户绑定」 |
| scope | 作用域 | 字段 `scope` 不译 |
| outbox | 发件箱（outbox） | 模式名保留 |
| relay | 中继 | 类型名 `Relay` 不译 |
| lease | 租约 | |
| projection | 投影 | outbox projection 译「发件箱投影」 |
| upsert | upsert（写入或更新） | 保留不译 |
| dedupe | 去重 | |
| consistency | 一致性 | `eventual` / `bounded` / `strong` 模式值保留不译 |
| stale | 陈旧 | serve stale 译「返回陈旧数据」 |
| cache hit / miss | 缓存命中 / 未命中 | |
| graceful shutdown | 优雅关闭 | |
| failover | 故障转移 | |
| guardrail | 护栏 | |
| guard | 防护 | 包名 `guard` 不译 |
| PII redaction | PII 脱敏 | PII 保留不译 |
| prompt injection | 提示词注入 | |
| policy | 策略 | 仓名/包名 `policy`、字段 `route policy` 中 policy 不译 |
| conformance harness | 一致性测试套件 | conformance contract 译「一致性契约」 |
| harness | 测试套件 | evaluation harness 译「评估套件」 |
| regression gate | 回归门禁 | |
| evaluation | 评估 | 包名 `eval` 不译 |
| benchmark | 基准测试 | 包名 `bench`、BFCL/GAIA 等保留不译 |
| deterministic | 确定性的 | |
| golden test | 黄金测试 | golden-testable 译「可黄金测试的」 |
| mock | 模拟（mock） | 类型名 `ScriptedLLM` / `ChatOnlyMock` 不译 |
| supervisor | 督导器 | 类型名 `Supervisor` 不译 |
| orchestration | 编排 | 包名 `orchestrate` 不译 |
| pipeline | 流水线 | 类型名 `Pipeline` 不译 |
| fan-out / fan-in | 扇出 / 扇入 | 类型名 `FanOutFanIn` 不译 |
| planner / aggregator | 规划器 / 汇聚器 | 角色名，字段名保留不译 |
| handoff | 交接 | |
| state graph | 状态图 | 类型名 `StateGraph` 不译 |
| node / edge | 节点 / 边 | 类型名 `Node` / `Edge` 不译 |
| topological sort | 拓扑排序 | |
| flow | 流程 | 仓名 `llm-agent-flow`、类型名 `Flow` 不译 |
| IR (intermediate representation) | 中间表示（IR） | |
| compile | 编译 | 方法 `Compile` 不译 |
| decorator | 装饰器 | decorator pattern 译「装饰器模式」 |
| wrapper / wrap | 包装器 / 包装 | 方法 `Wrap` 不译 |
| capability | 能力 | 类型名 `Capabilities` 不译 |
| capability negotiation | 能力协商 | |
| type assertion | 类型断言 | |
| streaming | 流式 | |
| stream event | 流事件 | 类型名 `StreamEvent` 不译 |
| typed union | 类型化联合 | |
| iterator | 迭代器 | |
| observability | 可观测性 | |
| telemetry | 遥测 | |
| OpenTelemetry / OTel | OpenTelemetry / OTel | 保留不译；仓名 `llm-agent-otel`、包名 `otel*` 不译 |
| span / trace | span / 链路 | span 保留不译；trace 译「链路」，decision trace 译「决策链路」 |
| tracer / tracer provider | 追踪器 / 追踪器提供者 | 类型名 `TracerProvider` 不译 |
| exporter | 导出器 | OTLP 保留不译 |
| sampler / sampling | 采样器 / 采样 | |
| meter / metrics | 计量器 / 指标 | 具体指标名不译 |
| counter / gauge | 计数器 / 仪表盘量 | |
| cardinality | 基数 | low-cardinality 译「低基数」 |
| semconv (semantic conventions) | 语义约定（semconv） | semconv 保留不译 |
| observer hook | 观察者钩子 | 类型名 `Observer` 不译 |
| hook | 钩子 | |
| token | token | 保留不译；token budget 译「token 预算」 |
| token accounting | token 计量 | |
| usage | 用量 | 类型名 `Usage` 不译 |
| build tag | 构建标签 | 具体 tag（如 `llmagent`）保留不译 |
| replace directive | replace 指令 | `replace` 保留不译 |
| require | require | Go 依赖指令保留不译 |
| pin (依赖) | 锚定 | sibling pin 译「兄弟仓锚定」 |
| bump | 版本提升 | bump wave 译「版本提升波次」 |
| semantic versioning / semver | 语义化版本 | semver 保留不译 |
| breaking change | 破坏性变更 | |
| backward compatible (BC) | 向后兼容（BC） | |
| additive-only | 仅增量 | |
| frozen surface | 冻结面 | frozen v1 public surface 译「冻结的 v1 公共面」 |
| API snapshot | API 快照 | 包名 `apisnapshot` 不译 |
| deprecation | 弃用 | |
| auto-merge | 自动合并 | |
| branch protection | 分支保护 | |
| status check | 状态检查 | |
| governance | 治理 | PR governance 译「PR 治理」 |
| keystone (decision) | 基石（决策） | 如 keystone K1，K1 等编号保留 |
| reference service | 参考服务 | |
| walking skeleton | 走通骨架 | |
| escape hatch | 逃生舱 | 指临时手段，如 replace 是本地开发逃生舱 |
| migration | 迁移 | schema migration 译「schema 迁移」，schema 保留不译 |
| best-effort | 尽力而为 | |
| opt-in / opt-out | 选择开启 / 选择关闭 | |
| no-op | 空操作 | |
| graceful degradation | 优雅降级 | |
| backoff | 退避 | exponential backoff 译「指数退避」 |
| CGO | CGO | 保留不译 |
