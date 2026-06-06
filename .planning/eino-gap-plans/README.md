# eino 对照：三件基础能力补全计划（索引）

对照 cloudwego/eino 的核心设计支柱，本项目缺三件"基础能力"。这里是三份计划的索引，以及把它们钉在一起的**跨计划契约**与**实施顺序**。

> 背景：本项目相对 eino 已在多 Agent（A2A/MCP）、RAG（GraphRAG/主动检索/评测）、记忆分层、policy gates 守护、Provider 覆盖上领先；差距集中在 eino 的编排支柱——**类型安全编排 + 流式自动适配 + Prompt 模板组件 + HITL checkpoint**。

## 三份计划

| 计划 | 文件 | 仓库 | 规模 |
|---|---|---|---|
| 1. ChatTemplate 组件 | [`../chat-template/PLAN.md`](../chat-template/PLAN.md) | `llm-agent-contract`（新 `prompt/` 包） | 小 |
| 2+3. **flow v2 协同设计**（typed Graph + checkpoint/HITL 同生） | [`../flow-v2/PLAN.md`](../flow-v2/PLAN.md) ⭐ | `llm-agent-flow/v2`（新模块路径，引擎重设） | 大（分阶段） |
| ↳ 原 flow-graph（已并入 flow-v2） | [`../flow-graph/PLAN.md`](../flow-graph/PLAN.md) | — | superseded |
| ↳ 原 flow-checkpoint（已并入 flow-v2） | [`../flow-checkpoint/PLAN.md`](../flow-checkpoint/PLAN.md) | — | superseded |

> **flow-graph 与 flow-checkpoint 已合并为 [`flow-v2/PLAN.md`](../flow-v2/PLAN.md)**——两轮审核证明 typed Graph 不能在现有引擎上加法泛化（独立包调不到 unexported `runAny`、`Runner`/`FlowEvent` 是冻结 string 面），且 checkpoint 序列化与 typed 数据面深度耦合。v2 走新模块路径 `llm-agent-flow/v2`（`compatibility.md:42-44` 的破坏性变更出口），v0.1 冻结保留、JSON DAG 作为 v2 一等前端保留。两份原计划保留作设计参考。

## 跨计划契约（已拍板，三份计划据此对齐）

1. **CT-1 — ChatTemplate 输出类型**：`prompt.Template.Format` 基接口返回 `[]llm.Message`；可选 `prompt.Requester.FormatRequest` 返回 `llm.Request`。flow-graph 的 **Template 节点消费 `prompt.Requester`**（`FormatRequest → llm.Request`），下游 ChatModel 节点直接拿到 `llm.Request`。
2. **CP-1 — checkpoint 快照边界**：checkpoint 在 **layer 边界**快照（绝不在流飞行中）。typed/`any` 数据面与 `StreamReader` 在 v1 **不可 checkpoint**，不可序列化的图返回 `ErrNotCheckpointable`。
3. **序列化约束（flow-graph → flow-checkpoint）**：flow-graph 引入的类型化数据面会让快照变难。约束 flow-graph：图状态应 **JSON 可序列化 by construction**，并暴露编译期 `Checkpointable()/Serializable() bool`；避免 gob + 全局类型注册（跨模块脆弱）。
4. **依赖时效**：contract 当前最新 tag = **v0.4.0**；chat-template 发 **v0.5.0**（新增 `prompt/` 包）。flow-graph 的 Template 节点依赖 contract v0.5.0 已发布。

## 实施顺序（两轮审核后修订，2026-06-05）

两轮审核（Plan agent + codex）把"flow-graph 共用现有引擎"定为最高风险假设并证伪（独立包调不到 unexported `runAny`、`Runner`/`FlowEvent` 是冻结 string 面）。用户拍板：**flow-graph 走 v2 引擎重设，与 checkpoint 联合设计**。原"chat-template → checkpoint先行 → flow-graph 分阶段"的顺序**作废**。

```
chat-template (contract v0.5.0, 独立先行)     ← 只 contract 叶子包；agent/RAG 集成移出本里程碑
        │  无下游耦合，可独立落地
        ▼
[先决修复] contract: 修 AccumulateStream EOF 判断 (errors.Is) ; 统一 Tool 面
        │
        ▼
flow v2  =  flow-graph  +  flow-checkpoint  （一个协同工程，major bump）
   一次性联合设计：typed 数据载体 · Runner/decorator · FlowEvent 形状 ·
   store 语义 · checkpoint 序列化 · flowd 集成
```

理由：
- **chat-template 先行且独立**——最小、零破坏；但**不含** agent/RAG 集成（核心仓用具体 `SimpleOptions` 结构体、ReAct 用 fmt.Sprintf 拼 prompt，集成是跨 agent 范式的多步工作，独立里程碑）。
- **flow-graph 与 checkpoint 合并为 flow v2**——typed 值的序列化、`StreamReader` 不可快照、layer 边界快照、RunID 所有权、`FinishRun` suspended 转移、事件/Runner/store 的破坏性演进，全部互相牵连，必须一次设计。**不并行做 checkpoint 和 any-引擎**（codex 明确警告）。
- **不再有 "string-map 引擎上先跑 HITL" 的独立 MVP**——它依赖的 string 数据面正是 v2 要替换的；先做会与 v2 互相拉扯。

## 各计划当前判定（2-round review）

| 计划 | 判定 | 去向 |
|---|---|---|
| chat-template | APPROVE-WITH-CHANGES | 独立落地，应用 R1-R6 修订 |
| flow-graph | NEEDS-REWORK | 并入 flow v2 协同设计 |
| flow-checkpoint | NEEDS-REWORK | 并入 flow v2 协同设计 |

## 共同原则（karpathy 准则）

- 三份计划全部**向后兼容、纯加法**：新包/新方法/新可选接口，不动 frozen API（`NodeKind`/`Runner`/`Store`/contract 三包导出）。
- 每个任务带**可验证目标**（TDD：先写失败测试再实现）。
- 明确 **MVP 边界**与**开放问题**，不静默选边——见各计划末尾 open questions。
