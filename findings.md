# 多轮功能支持审核发现

## Phase 1 - 功能存在性

| 功能项 | 支持状态 | 代码证据 | 发现 |
|---|---|---|---|
| Prompt / ChatTemplate 通用组件 | 已支持 | `llm-agent-contract/prompt/prompt.go` | 有 `Template`、`Requester`、`Spec`、`EngineBrace`、`EngineGoTemplate`、`FormatRequest`。 |
| 类型安全编排 | 已支持 | `llm-agent-flow/v2/flow/graph` | 有 `NewGraph[I,O]`、typed node builder、`Compile` reflect 类型检查、typed `Invoke`。 |
| 流式自动适配 | 已支持/基本支持 | `llm-agent-flow/v2/flow/graph/runnable.go`、`stream*.go`、`merge.go`、`copy.go` | 有 `StreamCapable`、linear stream、branch DAG、Copy DAG、stream merge/zip；复杂 fan-in 形态会降级。 |
| HITL / checkpoint / resume | 已支持 | `llm-agent-flow/v2/flow/checkpoint.go`、`resume.go`、`interrupt.go`、`cmd/flowd/server/server.go` | 有 checkpoint store、interrupt、suspension、`RunResumable`、`Resume`、flowd resume route。 |
| Workflow 字段级数据映射 | 未支持 | `llm-agent-flow/v2/flow/ir.go` | IR 只有 `NamedPortRef` 和 `Edge Source/Target/Condition`；未发现 `InputMapping`、`InputsFrom`、`jsonpath` 等通用字段映射能力。 |
| 统一 Agent 运行事件 | 部分支持 | `llm-agent-contract/agents/agent.go`、`llm-agent-flow/v2/flow/event.go`、`llm-agent-contract/llm/stream.go` | Agent `StepEvent`、Flow `Event`、LLM `StreamEvent` 分别存在，但未统一成跨层事件模型。 |
| 统一 Callback / Aspect 能力 | 部分支持 | `llm-agent-otel/*`、`llm-agent-policy/policy.go`、`llm-agent-rag/rag/observer.go` | 有 OTel wrapper、policy gate、RAG observer、Agent `OnStep`，但不是统一跨 model/tool/graph/agent 的 callback/aspect。 |
| 预置 Agent Pattern 产品化 | 部分支持 | `llm-agent/*.go`、`llm-agent/orchestrate/*.go`、`llm-agent-builtin/*.go` | 有 5 类 Agent、Supervisor/FanOut/RoundRobin/RolePlay、内置工具；缺少 DeepAgent 类成套产品化 pattern。 |
| DevOps 可视化与调试 | 部分支持 | `llm-agent-flow/cmd/flowd/server/server.go`、`llm-agent-flow/flow/store` | 有 flow CRUD、run history、events、replay；未发现可视化 UI/图编辑器/IDE 调试界面。 |

## Phase 2 - 边界与成熟度

| 功能项 | 成熟度判断 | 边界/风险 | 测试验证 |
|---|---|---|---|
| Prompt / ChatTemplate 通用组件 | 可用能力 | 作为 contract prompt 包存在，覆盖基础模板渲染和 request 转换。 | `llm-agent-contract`: `go test ./prompt` 通过。 |
| 类型安全编排 | 可用能力 | v2 typed graph 有较完整 builder、compile、typecheck、lowering 测试；子 module 当前未在 `go.work` 中直接纳入。 | 直接测试 `llm-agent-flow/v2` 受依赖下载/网络超时影响未完成。 |
| 流式自动适配 | 可用能力，但有形态边界 | linear、branch DAG、Copy DAG、stream merge/zip 有测试；parallel/combine fan-in、implicit tee、diamond 等复杂形态按测试预期降级，不是“任意图全自动流式”。 | 直接测试受同上网络问题影响；代码测试文件覆盖较密集。 |
| HITL / checkpoint / resume | 可用能力 | 有内存和 sqlite checkpoint、state checkpoint、struct hash、flowd resume 测试；flowd v2 resume route 仅在 `V2Registry` 和 `V2Checkpoints` 存在时注册。 | 直接测试受同上网络问题影响；服务端测试也因依赖下载失败未完成。 |
| Workflow 字段级数据映射 | 未形成能力 | 没有通用字段选择/字段组装机制；只能通过端口连接、显式 lambda/combine 实现个案。 | 检索未发现相关实现和测试。 |
| 统一 Agent 运行事件 | 局部能力 | Agent、Flow、LLM 分别有事件模型和测试；缺少跨层统一事件 envelope，因此不能标为已支持。 | Agent RunStream、Flow events、LLM StreamEvent 均有测试，但分散。 |
| 统一 Callback / Aspect 能力 | 局部能力 | OTel、Policy、RAG Observer、Agent `OnStep` 都有测试；缺少统一跨层 callback/aspect 注册与生命周期。 | policy/otel/rag observer 测试存在，覆盖局部 wrapper 行为。 |
| 预置 Agent Pattern 产品化 | 局部能力 | 5 类 Agent 和 orchestrate patterns 测试充足；但 DeepAgent 类“todo/sub-agent/filesystem/shell/middleware”成套产品化未形成。 | Agent/orchestrate/builtin 测试存在。 |
| DevOps 可视化与调试 | 局部能力 | flowd 有 run event/replay；customer-support 有 Grafana dashboard；但未发现 flow 图可视化编辑器、运行时调试 UI 或 IDE 调试器。 | server events/replay/metadata/resume 测试存在；Grafana dashboard 是观测面，不是交互调试器。 |

## Phase 3 - 文档适配

当前 `.planning/eino-gap-plans/README.md` 仍把以下能力列为“需要补充”：

- Prompt / ChatTemplate 通用组件
- 类型安全编排
- 流式自动适配
- HITL / checkpoint / resume

多轮代码审核显示，这四项当前已经有明确实现入口和测试痕迹，不应继续作为原始“缺口”列出。更合适的文档状态应为：

| 功能项 | 建议文档状态 | 原因 |
|---|---|---|
| Prompt / ChatTemplate 通用组件 | 移入“已支持能力” | `llm-agent-contract/prompt` 已提供通用组件。 |
| 类型安全编排 | 移入“已支持能力”，可注明 v2 typed graph 边界 | `llm-agent-flow/v2/flow/graph` 已提供 code-first typed graph。 |
| 流式自动适配 | 移入“已支持能力”，注明复杂图会降级 | 已支持 linear/branch DAG/copy DAG/merge/zip，不是任意图全自动。 |
| HITL / checkpoint / resume | 移入“已支持能力” | v2 flow 与 flowd 已有 resume 支持。 |
| Workflow 字段级数据映射 | 保留为“需要补充” | 没有通用字段映射能力。 |
| 统一 Agent 运行事件 | 保留为“部分支持，需统一” | Agent/Flow/LLM 事件分散。 |
| 统一 Callback / Aspect 能力 | 保留为“部分支持，需统一” | wrapper/observer/gate 分散。 |
| 预置 Agent Pattern 产品化 | 保留为“部分支持，需产品化” | 有 primitives，缺成套 pattern。 |
| DevOps 可视化与调试 | 保留为“部分支持，需调试体验” | 有 events/replay/Grafana dashboard，缺 graph 调试 UI。 |

建议优先级也应调整：

1. 第一优先级：Workflow 字段级数据映射、统一 Agent 运行事件。
2. 第二优先级：统一 Callback / Aspect、预置 Agent Pattern 产品化。
3. 第三优先级：DevOps 可视化与调试体验。
4. 已支持但可继续增强：Prompt、typed graph、streaming、HITL。

## Phase 5 - 字段级数据映射实现审核

| 检查项 | 结论 | 代码证据 |
|---|---|---|
| IR 是否只补功能字段 | 已补 `Flow.Mappings` 与 `Mapping/MappingSource/MappingTarget`，没有引入兼容性适配层 | `llm-agent-flow/v2/flow/ir.go` |
| 校验是否覆盖无效 mapping | 覆盖空 source、混用 input/node source、未知 node、空 port、自环，并把 node-source mapping 纳入 cycle 检测 | `llm-agent-flow/v2/flow/validate.go` |
| 调度是否把 mapping 当作依赖 | node-source mapping 纳入 indegree/outgoing；target 不会作为无前驱 entry 被提前激活 | `llm-agent-flow/v2/flow/engine.go` |
| 运行期是否支持字段组装 | 支持从 input 或节点输出选字段，写入目标 port 或目标 port 下的嵌套 map | `llm-agent-flow/v2/flow/mapping.go` |
| checkpoint/resume 是否一致 | suspend 前应用同层非 interrupt 节点 mapping；resume 后把 humanInput 作为 interrupt 节点输出再应用 mapping | `llm-agent-flow/v2/flow/engine.go`、`llm-agent-flow/v2/flow/resume.go` |
| resume cursor 是否受保护 | structural hash 纳入 mapping 的顺序、source、target 和 path | `llm-agent-flow/v2/flow/structhash.go` |
| 测试覆盖 | 覆盖 input 字段组装、node output 字段激活、同目标 port 多字段合并、缺失路径错误、混合 source 校验、mapping 参与环检测、resume humanInput mapping | `llm-agent-flow/v2/flow/mapping_test.go` |

边界记录：

- input-source mapping 当前要求调用方必须提供对应 input key；缺失会报错。
- node-source mapping 如果 source port 没有产生值则跳过，语义与普通 edge 的 source port 缺失处理保持接近。
- target path 只能写入 `map[string]any`；如果目标端口已有非 map 值再写子路径，会报错。

独立 agent 审核处理：

- 已修复：`Mapping.Source.Path` / `Mapping.Target.Path` 的空 segment 现在会在 `Validate` 阶段被拒绝。
- 已固化测试：未知 node-source port 不激活 target，保持当前 edge-like 的“无源值不传播”语义。
- 未扩展：没有新增 mapping edge state 或 resolved port 静态校验，避免把本次功能补齐扩大成兼容性/诊断体系改造。

## Phase 6 - 统一 Agent 运行事件实现审核

| 检查项 | 结论 | 代码证据 |
|---|---|---|
| 统一事件落点 | 已落在 `llm-agent-contract/agents`，由 agent contract 作为跨层事件 envelope 出口 | `llm-agent-contract/agents/run_event.go` |
| 是否替换现有接口 | 未替换 `StepEvent`、`llm.StreamEvent`、`flow.Event`，只新增转换函数 | `RunEventFromStepEvent`、`RunEventFromStreamEvent`、`flow.RunEventFromFlowEvent` |
| Agent 事件转换 | step、done、error、cancel error 可转成 `RunEvent` | `llm-agent-contract/agents/run_event_test.go` |
| LLM 事件转换 | text/thinking delta、tool call、model done 可转成 `RunEvent`，保留原始 `llm.StreamEvent` | `llm-agent-contract/agents/run_event_test.go` |
| Flow 事件转换 | flow start/node start/node done/skipped/error/interrupt/suspended 可转成 `RunEvent` | `llm-agent-flow/v2/flow/run_event.go`、`run_event_test.go` |
| HITL 信息保留 | Flow interrupt/suspend 保留 `ResumeToken` 和 `InterruptRequest` payload | `llm-agent-flow/v2/flow/run_event_test.go` |
| 上游暴露 | `llm-agent` alias 暴露 `RunEvent`、`RunEventKind`、常量和转换函数 | `llm-agent/aliases.go` |

边界记录：

- 这是统一事件 envelope 首版，不是统一 Callback / Aspect 框架。
- 没有修改任何现有 stream 接口签名；调用方需要统一事件时显式调用转换函数。
- flowd/customer-support SSE 尚未切换到统一事件输出，避免扩大本阶段变更面。

## Phase 7 - 统一 Callback / Aspect 实现审核

| 检查项 | 结论 | 代码证据 |
|---|---|---|
| Callback 落点 | 已落在 `llm-agent-contract/agents`，基于 Phase 6 的 `RunEvent` | `llm-agent-contract/agents/callback.go` |
| 是否改现有接口 | 未修改 Agent/LLM/Flow 运行接口，只提供观察式 callback | `Callback`、`EmitRunEvent`、`WrapAgent` |
| 组合能力 | 支持 `NoopCallback`、函数适配、链式 callback，并跳过 nil | `callback.go`、`callback_test.go` |
| callback 故障隔离 | `EmitRunEvent` recover panic，callback 不改变主流程 | `llm-agent-contract/agents/callback_test.go` |
| Agent 装饰器 | `WrapAgent` / `ObserveAgent` 可镜像 `Run` 与 `RunStream` 的统一运行事件 | `llm-agent/callback.go` |
| 事件透传 | `WrapAgent.RunStream` 保留原始 `StepEvent` 输出，同时触发 callback | `llm-agent/callback_test.go` |
| 同步运行 | `WrapAgent.Run` 保留原始 `Result` / error，同时从 trace 与 terminal 结果触发 callback | `llm-agent/callback.go`、`callback_test.go` |
| 上游暴露 | `llm-agent` alias 暴露 callback 类型与辅助函数 | `llm-agent/aliases.go` |

边界记录：

- 这是观察式 Callback / Aspect 首版，不提供阻断、改写、重试等 policy 能力。
- OTel、policy、RAG observer 暂不改，避免把局部 wrapper 强行迁移到统一框架。
- callback 是同步调用；需要异步或缓冲时由调用方自行包装 callback。

## Phase 8 - 预置 Agent Pattern 产品化实现审核

| 检查项 | 结论 | 代码证据 |
|---|---|---|
| 产品化落点 | 已新增 `llm-agent/patterns` 作为 catalog/factory 薄层 | `llm-agent/patterns` |
| 单 Agent preset | 支持 Simple、ReAct、FunctionCall、PlanAndSolve、Reflection、Workspace | `patterns/patterns.go` |
| Catalog 能力 | `Catalog` 提供稳定顺序、描述和 capability | `patterns/patterns_test.go` |
| Callback 组合 | `Build` 支持可选 `Callback`，用 `WrapAgent` 包装 | `patterns/patterns.go`、`patterns_test.go` |
| Workspace 安全边界 | Workspace 只使用调用方提供的 `Registry`，不默认加入 terminal/shell/filesystem 工具 | `patterns/patterns.go` |
| 多 Agent factory | 支持 Supervisor、FanOutFanIn、RoundRobin、RolePlay factory | `patterns/orchestrate.go` |
| 结果形态 | RoundRobin/RolePlay 保留专用 result；未强行适配为 `agents.Agent` | `patterns/orchestrate_test.go` |
| 默认解析/聚合 | Supervisor 提供 `ParseDispatchLine`、`JoinWorkerResults` 默认能力 | `patterns/orchestrate.go`、`orchestrate_test.go` |
| 测试覆盖 | 覆盖 catalog、单 Agent 构造、workspace registry、多 Agent factory、错误边界 | `patterns/*_test.go` |

边界记录：

- 首版只是产品化入口，不新增 Agent 执行引擎。
- 没有引入 `llm-agent-builtin` 作为核心依赖；工具仍由调用方显式注册。
- 没有默认开启 shell/terminal 能力。

## Phase 9 - DevOps 可视化与调试体验实现审核

| 检查项 | 结论 | 代码证据 |
|---|---|---|
| Flow 级调试视图 | 已新增 `GET /flows/{id}/debug`，返回 flow metadata、graph、最近 runs、raw JSON | `llm-agent-flow/cmd/flowd/server/server.go`、`debug.go` |
| Run 级调试视图 | 已新增 `GET /runs/{id}/debug`，返回 run、events、nodes、timeline、replay、suspended | `llm-agent-flow/cmd/flowd/server/server.go`、`debug.go` |
| 是否改 store 主接口 | 未修改 `flow/store.Store`，只复用 `GetRun` / `ListRunEvents` / `ListRuns` | `llm-agent-flow/flow/store/store.go` 未变更 |
| 是否改 events/replay 语义 | 未修改 `/runs/{id}/events` 和 `/runs/{id}/replay` handler | `cmd/flowd/server/server.go` |
| 节点状态聚合 | 从 `node_started`、`node_finished`、`node_skipped`、`node_interrupted`、`flow_suspended`、`flow_err` 推导节点状态与 seq | `cmd/flowd/server/debug.go` |
| malformed payload | decode 失败不导致 debug 500；事件输出 `decode_error` 与 `raw_payload` | `cmd/flowd/server/debug.go`、`server_debug_test.go` |
| 测试覆盖 | 补充 flow debug、run trace、failed、skipped、suspended、missing run、malformed payload 测试 | `cmd/flowd/server/server_debug_test.go` |

边界记录：

- 首版是 API 级调试视图，不包含前端 UI、图编辑器或 IDE 插件。
- run debug 是派生视图，不保证 v2 resumable 路径拥有完整 node started/finished 细粒度事件；当前 v2 resumable 只能展示 synthesized coarse trace。
- `GET /flows/{id}/debug` 当前按 v0.1 `flow.Flow` 投影 graph；v2 额外字段如 mappings 不在本轮做兼容性投影。
- 目标测试受网络依赖下载限制未跑通：`google.golang.org/genproto@v0.0.0-20240903143218-8af14fe29dc1` 从 `proxy.golang.org` 下载超时。
- 2026-06-09 复测：切换 `GOPROXY=https://goproxy.cn,direct` 后，`go test ./cmd/flowd/server -run 'TestDebug' -count=1` 与 `go test ./cmd/flowd/server -count=1` 均通过。
