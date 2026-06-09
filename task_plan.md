# 多轮功能支持审核计划

## 目标

对当前项目中与 Eino 对比的功能项做多轮只读审核，判断代码是否已经支持、支持边界是否清楚、现有计划文档是否需要调整功能项状态。

## 约束

- 前四阶段只审核功能支持情况；用户后续明确要求开始执行写代码。
- 不做兼容性/API/IR 设计。
- 不回滚用户或既有变更。
- 结论必须有本地代码证据。

## 阶段

| 阶段 | 状态 | 内容 |
|---|---|---|
| Phase 1 | complete | 功能存在性与代码证据复核 |
| Phase 2 | complete | 边界、成熟度、测试覆盖复核 |
| Phase 3 | complete | 计划文档结论适配复核 |
| Phase 4 | complete | 汇总多轮审核结论 |
| Phase 5 | complete | 实现 Workflow 字段级数据映射 |
| Phase 6 | complete | 实现统一 Agent 运行事件首版 |
| Phase 7 | complete | 实现统一 Callback / Aspect 首版 |
| Phase 8 | complete | 实现预置 Agent Pattern 产品化首版 |
| Phase 9 | complete | 实现 DevOps 可视化与调试体验首版 |

## 待审核功能项

1. Prompt / ChatTemplate 通用组件
2. 类型安全编排
3. 流式自动适配
4. HITL / checkpoint / resume
5. Workflow 字段级数据映射
6. 统一 Agent 运行事件
7. 统一 Callback / Aspect 能力
8. 预置 Agent Pattern 产品化
9. DevOps 可视化与调试

## 当前实现目标

先实现第一优先级中的 `Workflow 字段级数据映射`：允许 flow 在运行时从全局输入或上游节点输出中选取字段，组装成目标节点输入端口的结构化值。

## Phase 5 完成范围

- `llm-agent-flow/v2` 的 `Flow` 支持 `Mappings`。
- mapping source 支持全局输入或上游节点输出端口，并支持字段路径选择。
- mapping target 支持写入目标节点输入端口，支持嵌套 map 组装。
- node-source mapping 参与拓扑排序、环检测、checkpoint structural hash。
- fresh run、suspend checkpoint 前、resume humanInput 注入后均会应用对应 mapping。

## Phase 6 完成范围

- `llm-agent-contract/agents` 新增 `RunEvent` / `RunEventKind` 统一运行事件 envelope。
- 提供 `RunEventFromStepEvent`，把现有 Agent `StepEvent` 转成统一事件。
- 提供 `RunEventFromStreamEvent`，把 LLM `StreamEvent` 转成统一事件。
- `llm-agent` 通过 alias 暴露新增类型、常量和转换函数。
- `llm-agent-flow/v2/flow` 新增 `RunEventFromFlowEvent`，把 Flow v2 `Event` 转成统一事件，并保留 interrupt/suspend 的 `ResumeToken` 与 payload。
- 未修改现有 `Agent.RunStream`、`llm.StreamReader`、`flow.Runner.RunStream` 签名。

## Phase 7 完成范围

- `llm-agent-contract/agents` 新增观察式 `Callback` / `CallbackFunc`。
- 提供 `NoopCallback`、`EmitRunEvent`、`ChainCallbacks`，并保留 `RunEventHandler` 命名别名。
- `EmitRunEvent` 会 recover callback panic，避免观测逻辑改变主流程。
- `llm-agent` 通过 alias 暴露 callback 类型和辅助函数。
- `llm-agent` 新增 `WrapAgent` / `ObserveAgent` 装饰器，把 Agent `Run` / `RunStream` 镜像成统一 `RunEvent` 回调。
- 未修改现有 Agent 接口、OTel wrapper、policy gate、RAG observer。

## Phase 8 完成范围

- `llm-agent/patterns` 新增产品化 pattern catalog / factory。
- 支持 `Simple`、`ReAct`、`FunctionCall`、`PlanAndSolve`、`Reflection`、`Workspace` 单 Agent preset。
- `Workspace` preset 使用调用方提供的 `Registry`，不默认引入 shell/filesystem 工具。
- 支持 `BuildSupervisor`、`BuildFanOutFanIn`、`BuildRoundRobin`、`BuildRolePlay` 多 Agent factory。
- 多 Agent factory 保留原有 orchestrate 专用 result，不强行包装成 `agents.Agent`。
- 未修改现有 Agent 和 orchestrate 执行语义。

## Phase 9 完成范围

- `llm-agent-flow/cmd/flowd/server` 新增 flow 级 `GET /flows/{id}/debug`。
- `GET /flows/{id}/debug` 返回 flow metadata、graph nodes/edges/inputs/outputs、最近 10 次 runs 和 raw JSON。
- `llm-agent-flow/cmd/flowd/server` 新增 run 级 `GET /runs/{id}/debug`。
- `GET /runs/{id}/debug` 返回 run record、原始事件的 debug 表示、节点状态聚合、timeline、replay path、suspended 摘要。
- run debug 仅复用 `GetRun` / `ListRunEvents`，不修改 `flow/store.Store` 接口，不改变 `/runs/{id}/events` 或 `/runs/{id}/replay` 语义。
- malformed event payload 不作为 500 处理；debug event 记录 `decode_error`，无效 payload 以 `raw_payload` 输出。
- 未实现前端 UI、图编辑器、IDE 插件，首版定位为 API 级调试视图。
