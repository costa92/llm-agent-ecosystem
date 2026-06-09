# eino 对照：功能支持状态与待补充项

对照 cloudwego/eino 的核心能力，本项目不需要照搬组件数量。本文只记录**当前项目已经支持什么、还缺什么功能能力**，用于后续拆计划时参考。

> 审核结论更新：经多轮代码审核，Prompt / ChatTemplate、类型安全编排、流式自动适配、HITL / checkpoint / resume 已在当前代码中具备支持，不再作为原始“待补充功能”列出。

## 当前项目的相对优势

对照 eino，当前项目已经具备或更深入的能力：

- **RAG / GraphRAG 更深**：`llm-agent-rag` 已有导入、检索、GraphRAG、主动检索、诊断、评测、漂移分析与 Postgres/pgvector 后端。
- **Durable Memory 是独立体系**：`llm-agent-memory-*` 已拆出 contract、SDK、Postgres backend、HTTP gateway、worker、client，并有 outbox、session lifecycle、recall cache、promotion policy。
- **Provider 覆盖更贴近真实接入**：`llm-agent-providers` 已覆盖 OpenAI、Anthropic、Ollama、DeepSeek、MiniMax、Volcengine、Google、Kimi 等 provider surface。
- **Policy 与通信协议独立化**：`llm-agent-policy` 提供 capability-preserving guardrails；`llm-agent-comm` 提供 A2A/MCP/transport 抽象。
- **JSON DAG + flowd 是差异化资产**：`llm-agent-flow` 已有可序列化流程定义、持久化 run history、HTTP/SSE surface 和事件审计。

因此，本轮补齐重点不是增加更多组件名，而是让已有能力能通过更强的编排层组合起来。

## 已支持的能力

| 能力 | 当前项目支持 | 代码证据 | 边界 |
|---|---|---|---|
| **Prompt / ChatTemplate 通用组件** | 支持 system、few-shot、history、user turn 组合，支持 brace 与 `text/template` 两类模板引擎，并可输出 `llm.Request` | `llm-agent-contract/prompt` | 已是可用基础组件，后续只需按业务场景接入。 |
| **类型安全编排** | 支持 code-first typed graph、typed node、edge 类型检查、typed invoke | `llm-agent-flow/v2/flow/graph` | 代码内编排已支持；JSON DAG 仍是独立的可序列化前端。 |
| **流式自动适配** | 支持 linear stream、branch DAG、Copy DAG、stream merge/zip，并提供 stream-to-value concatenator | `llm-agent-flow/v2/flow/graph` | 不是任意复杂图全自动流式；parallel/combine fan-in、implicit tee、diamond 等复杂形态按当前测试预期降级。 |
| **HITL / checkpoint / resume** | 支持 interrupt、suspension、checkpoint store、`RunResumable`、`Resume`，flowd 支持 resume route | `llm-agent-flow/v2/flow`、`llm-agent-flow/cmd/flowd/server` | flowd resume route 依赖 v2 registry 与 checkpoint store 已配置。 |
| **Workflow 字段级数据映射** | 支持从全局输入或上游节点输出端口选字段，组装目标节点输入端口 | `llm-agent-flow/v2/flow` | 首版支持字符串 path、`map[string]any`/`map[string]string`/struct 字段读取，以及目标端口 map 组装。 |
| **统一 Agent 运行事件** | 支持 `RunEvent` 统一 envelope，可从 Agent `StepEvent`、LLM `StreamEvent`、Flow v2 `Event` 转换 | `llm-agent-contract/agents`、`llm-agent-flow/v2/flow` | 首版不替换现有 stream 接口；调用方需要统一事件时显式转换。 |
| **统一 Callback / Aspect 能力** | 支持观察式 `Callback`、callback chain，以及 Agent wrapper 镜像统一 `RunEvent` | `llm-agent-contract/agents`、`llm-agent` | 首版只做观测，不做阻断/改写；policy gate 仍负责安全拦截。 |
| **预置 Agent Pattern 产品化** | 支持 `patterns` catalog/factory，覆盖单 Agent preset 与多 Agent orchestration factory | `llm-agent/patterns` | 首版不新增执行引擎；Workspace 不默认启用 shell/terminal。 |
| **DevOps 可视化与调试** | 支持 flow 拓扑 debug JSON 与 run 级 trace/debug JSON，可查看节点输入输出、事件时间线、replay 入口与 suspended 摘要 | `llm-agent-flow/cmd/flowd/server` | 首版是 API 级调试视图，不包含前端 UI、图编辑器或 IDE 插件。 |

## 需要补充的功能

当前首轮对照项均已有功能落点。后续可继续增强的是交互体验与产品化程度，例如前端图调试 UI、图编辑器、IDE 插件等，但不再作为本轮原始功能缺口。

## 优先级判断

1. **本轮已补齐首版**：DevOps 可视化与调试体验。
2. **已支持但可继续增强**：Prompt / ChatTemplate、类型安全编排、流式自动适配、HITL / checkpoint / resume、Workflow 字段级数据映射、统一 Agent 运行事件、统一 Callback / Aspect、预置 Agent Pattern 产品化、DevOps 可视化与调试。

## 审核边界

- 本文只记录功能缺口，不写代码草案。
- 本文不讨论兼容性设计或实现路线。
- 本文不替代各子项目已有详细计划。
- 本文不要求为已支持能力重复立项。
