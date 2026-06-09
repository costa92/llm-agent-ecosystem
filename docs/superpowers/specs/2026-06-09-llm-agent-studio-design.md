# llm-agent-studio 设计文档（多 Agent 工作流可视化编排器）

- 日期：2026-06-09
- 状态：设计已批准，待写实现计划
- 类型：新案例项目（准生产级，独立 sibling 仓）
- 相关：[[project_case-study-projects]]；姊妹项目 llm-agent-kb（同期）

## 1. 目标与定位

为 `llm-agent-ecosystem` 提供第二个**面向终端用户的全栈案例项目**：拖拽式可视化构建/运行/调试多 Agent 工作流的编排器。充分串联 `llm-agent-flow` 的 v1（JSON IR + 拓扑执行 + CEL 条件边 + replay）与 v2（typed graph + 人在回路 checkpoint/interrupt/resume），核心差异化是**真 DAG 画布 + HITL 审批 + 多类节点**（LLM/内置工具/RAG/A2A/MCP），而非 JSON 文本编辑。

**非目标（v1 不做）**：token 级流式与 HITL 同时满足（含 HITL 的 flow 接受节点级时间线）、v2 typed graph builder 路线（走 IR+NodeRegistry）、扫描件/复杂数据流式分支、K8s 清单、外部消息队列。

### 成功标准

端到端可验证：登录 → 进工作区 → 画布拖出含 LLM/工具/RAG/条件节点的 flow → 保存（IR 校验通过）→ 运行（SSE 时间线实时上色）→ replay 回放 → 画一条含 approval 节点的 flow → 运行到中断 → 审批弹窗提交 → resume 续跑至完成。后端 `httptest`+真 Postgres，前端 Vitest+@testing-library，`docker compose up` 起栈。

## 2. 已确定的范围决策（v1）

| 维度 | 决策 |
|---|---|
| 引擎 | v1（JSON IR/flowd/CEL/replay）+ v2（HITL checkpoint/interrupt/resume）**都用** |
| 节点调色板 | LLM 节点 + 内置工具(builtin) + RAG 检索 + A2A/MCP 远程技能 + 条件/分支(CEL) |
| 运行方式 | **进程内执行**（后端内嵌 v1/v2 runner，不反代独立 flowd） |
| 租户/鉴权 | 共享库 **`llm-agent-authz`**（与 kb 同一库，scope_kind="workspace"）：组织(org)→工作区(workspace) + RBAC（admin/editor/viewer + org_admin）+ JWT/refresh（见 §17） |
| 技术栈 | 后端 Go（BFF+业务一体）；前端 React19/TS/Vite/Tailwind v4/shadcn/TanStack + **@xyflow/react** 画布 |
| 持久化 | Postgres（准生产）；开发可降级 SQLite（flow 两套 store 均有 SQLite 实现） |
| 可观测 | `llm-agent-otel`（`otelflow.Wrap` v1 + `otelflow/v2.Wrap` v2） |

## 3. 关键 API 事实（已核对真实代码，决定可行性）

- **v1 IR**（`flow/ir.go`）：`Flow{Nodes []Node, Edges []Edge, Inputs []NamedPortRef, Outputs []NamedPortRef}`；`Node{ID,Type,Config json.RawMessage}`；`Edge{Source,Target PortRef, Condition string}`；`NamedPortRef{Name, PortRef{Node,Port}}`。**`Load` 用 `DisallowUnknownFields()`** —— 画布坐标等额外字段**不能进 IR**（核心约束，§5 缓解）。
- **v1 节点注册**（`flow/node.go`）：`NodeRegistry.Register(typ, NodeFactory)`；`NodeKind{Inputs/Outputs/Run}`，carrier=`string`。仅内置 `tool` 节点（`flow/tool_node.go` `RegisterToolNode`），LLM/RAG 须 studio 自建。
- **Tool 桥**（`flow/adapter_llmagent.go`）：`FromAgentTool(agents.Tool) flow.Tool` —— builtin/comm/rag 全部经此接入 v1。
- **CEL**（`flow/cond/cel/cel.go`）：`eval, _ := cel.NewEvaluator()` → `WithConditionEvaluator(eval)`（构造函数是 `NewEvaluator() (*Evaluator, error)`，**无** `cel.New()`）；`Edge.Condition` 求值环境 `CondEnv{Value string}`（仅源端口字符串）。
- **v1 引擎/事件**（`flow/engine.go`、`flow/event.go`）：`Compile/RunStream`(→`<-chan FlowEvent`)；事件 `flow_started/node_started/node_finished/node_skipped/flow_done/flow_err`。
- **v1 Store**（`flow/store/store.go`）：Flow CRUD + Run 生命周期 + RunEvent；`RunStatusSuspended`/`SuspendRun`/`ResumeToken` 为可选能力（type-assert）。
- **flowd HTTP 蓝本**（`cmd/flowd/server/server.go`）：`/flows` CRUD、`/flows/{id}/run[/stream]`(SSE)、`/runs/{id}/events`、`/runs/{id}/replay`、**`/flows/{id}/runs/{runID}/resume`**(HITL)、`req.Resumable` 分流双引擎、`appendV2Event` 合成事件、resume `status!=suspended→409` 防重。**这是 studio 后端可直接复刻的蓝本。**
- **v2 是 v1 超集**（`v2/README.md`）：同一 flow JSON 两引擎都能 Load，carrier v2=`any`。
- **v2 HITL**（`v2/flow/{interrupt,resume,checkpoint}.go`）：节点 `Run` 返回 `flow.Interrupt(InterruptRequest{Kind,Prompt,Schema})` + 实现 `InterruptCapable.CanInterrupt()`；`Compile(...,WithCheckpointStore(cs))`→`RunResumable(ctx,runID,inputs)`→`RunResult{Outputs, Suspended *Suspension{ResumeToken,NodeID,Request}}`；`Resume(ctx,runID,token,humanInput map[string]any)`；`Checkpoint` 全量游标，resume 用 `StructHash` 容忍仅 Config 变化。
- **v2 checkpoint 持久化**（`v2/flow/store/sqlite/checkpoint.go`）：`checkpoints(token,run_id,flow_id,interrupt_node,created_at,data_json)`，studio 逐列复刻 Postgres 版即可。
- **comm**（`a2a/client.go`、`mcp/client.go`）：`a2a.AsAgentTool(client,skill,prefix) agents.Tool`；`mcp.AsAgentTools(ctx,client,prefix) []agents.Tool`；transport `comm.NewHTTPTransport`/`NewStdioTransport`/`NewInMemoryTransport(handler Handler)`（inmem 构造需传 `Handler`）。均产出 `agents.Tool`→`FromAgentTool` 成节点。
- **builtin**：`NewCalculator()/NewNoteTool(ws)/NewMockSearch(...)/NewTerminalTool(cfg)`（terminal 默认禁用，需 `EnableUnsafeExecution`）。
- **otel**：`otelflow.Wrap`(v1)、`otelflow/v2.Wrap`(v2)。v2 `Wrap` 返回 `v2flow.Runner`，仅当 inner 满足 `ResumableRunner` 时透传 `RunResumable/Resume`——调用方须 `w.(v2flow.ResumableRunner)` 断言才能拿到（见 §9）。
- **无可参照的全栈前端**：生态内尚无已定型、可作为代码参照的全栈前端（`llm-agent-console` 未定型，明确**不**作为引用代码）。studio 的 `web/`（SSE 客户端、运行时间线 reducer、DAG 画布、各表单/表格）全部**全新构建**，不移植任何未定型仓库代码。后端参照仅限已发版的稳定仓库（见 §15）。

## 4. 总体架构

单 Go 二进制（BFF + 业务一体，内嵌 v1+v2 runner，**不反代独立 flowd**）+ React SPA（@xyflow 画布）+ Postgres + otel collector。独立 sibling 仓 `github.com/costa92/llm-agent-studio`。

```
Browser (React SPA): DAG画布(@xyflow) · 运行时间线(SSE) · HITL审批弹窗 · Replay
  │  HTTPS JWT, /api/* (REST + SSE)
  ▼
studio 后端 (单 Go 二进制)
  httpapi(authz.Authenticate→ws→RequireScopeRole) · authz(import) · orgws(workspace) · flowstore(版本化+双列IR/canvas)
  · engine: runner-v1(RunStream→SSE) + runner-v2(RunResumable/Resume) 共享 NodeRegistry + LRU cache
  · nodes(llm/builtin/rag/a2a/mcp/cond/approval) + schema · comm/rag/providers 客户端 · checkpoint(Postgres) · obs(otelflow.Wrap)
  │ pgx                   │ HTTP/stdio              │ OTLP
  ▼                       ▼                         ▼
Postgres(主存储)     远程 A2A skill / MCP server    otel collector
外部: LLM provider(via llm-agent-providers), RAG(rag.System)
```

进程/容器边界：单容器=studio 后端（内嵌双 runner，embed 或 nginx 托管前端静态资源）；Postgres 独立容器（dev 可降级 SQLite）；MCP stdio server 由后端 fork 子进程（`comm.NewStdioTransport`）；A2A/MCP-http 为外部服务。

## 5. v1 与 v2 如何共存（最关键取舍）

**核心事实**：v2 IR 是 v1 IR 严格超集，同一 flow JSON 两引擎都能解析，区别仅 carrier 与是否有 HITL。flowd 已示范在同一 flow、同一 run_events 表上按 `req.Resumable` 切引擎。

**方案：单一 IR + 单一画布，引擎按「flow 是否含 HITL 节点」编译期自动选择，不暴露给用户。**

1. 画布只产出一套 IR（v1 schema），用户不感知 v1/v2。
2. **编译期分诊**：保存 flow 时扫描节点类型集合——
   - 不含 HITL 节点 → `engine_kind=v1`，运行走 `runner-v1` `RunStream` 全程 SSE 流式（性能/流式最佳）。
   - 含 ≥1 HITL 节点 → `engine_kind=v2`，运行走 `runner-v2` `RunResumable`，可中断/resume；事件由后端合成写入同一 run_event 表 + 独立 SSE 通道（复刻 `appendV2Event`）。
3. **统一事件模型**：前端时间线只认 `store.RunEventKind` 字符串集（+ HITL `node_interrupted`/`flow_suspended`），不区分引擎。
4. **节点工厂双注册**：每类型同时在 v1 与 v2 `NodeRegistry` 注册；HITL（approval）节点**仅** v2 有。

取舍理由：只用 v2 会牺牲 RunStream token 流式；只用 v1 无法 HITL；两套 IR 等于自找迁移/画布分叉地狱。**桥接风险**：v1 `DisallowUnknownFields` 要求画布元数据旁存（§6）。

> 已知局限：含 HITL 的 flow（走 v2）当前**无法 token 级流式**（RunResumable 同步返回），只做节点级时间线。token 流式+HITL 共存放 M4 评估。

## 6. 数据模型（Postgres）

**鉴权/租户表由 `llm-agent-authz` 拥有**（`auth_user`/`auth_org`/`auth_membership`/`auth_session`）；studio 注入 `scope_kind="workspace"`，不自建 org/user/membership/session。角色统一 `admin/editor/viewer`(+ org 级 `org_admin`)。`workspace` 即 authz membership 的 `scope_kind="workspace"` 所指资源。

业务表（studio 拥有）：
- `workspaces(id, org_id, name, created_at)` — `org_id` 弱引用 `auth_org`；建 workspace 时在 `auth_membership` 写 creator 的 admin 行

flow/run（flowstore——**Postgres 重实现 `store.Store` 接口 + 扩展列**，叠加 workspace 隔离与版本化）：
- `flows(id, workspace_id, name, description, engine_kind, latest_version, created_by, created_at, updated_at)`
- `flow_versions(flow_id, version, ir_json BYTEA, canvas_json BYTEA, base_version, created_by, created_at)` — **`ir_json` 是纯净 flow IR**（能被 `flow.Load` 通过 `DisallowUnknownFields`）；**`canvas_json` 旁存画布坐标/视图/分组等所有非 IR 元数据**（绕开 §5 约束）；保存须带 `base_version`/`If-Match`，与 `latest_version` 不符则 **409**（并发编辑，见 §17）
- `flow_runs(id, flow_id, version, workspace_id, status, inputs JSONB, outputs JSONB, error, started_at, finished_at, resume_token, interrupt_node)` — `version` **pin 触发时的 flow_version**（run/resume 必须用此版本编译引擎，见 §17）；`status ∈ {running,suspended,done,error,canceled}`（补 `canceled`/超时终态）
- `run_events(run_id, seq, kind, node_id, payload JSONB, ts)` — v1 实时事件与 v2 合成事件统一落此表，replay 顺序回放
- `checkpoints(token PK, run_id, flow_id, interrupt_node, created_at, data_json JSONB)` — **逐列复刻 `v2/flow/store/sqlite/checkpoint.go`**（真实实现 `token == run_id`、一 run 一活 checkpoint，多级审批靠同 token 反复 Save/Resume）；`data_json = json.Marshal(v2flow.Checkpoint)`；studio 实现 `v2flow.CheckpointStore` 4 方法；**suspended run 超 TTL 置 `canceled`+删 checkpoint 的后台清扫**（防泄漏，见 §17）

## 7. 仓库与模块布局

Go module：`github.com/costa92/llm-agent-studio`（独立 sibling 仓，自有 git + `.planning/`，从 umbrella gitignore；Go 命令需 `GOWORK=off`，见 [[project_console-gowork-off]]；加入 go.work 可本地联调全生态）。

```
llm-agent-studio/
├── cmd/studio/main.go        # DI 装配: DB、registry、engine cache、otel、http server
├── internal/
│   ├── (authz 接线)         # import llm-agent-authz: mount /api/auth/*, Authenticate/RequireScopeRole(scope_kind="workspace")
│   ├── orgws/                # workspace 业务 + store(建/删/列举); org/user/membership 由 authz 提供
│   ├── flowstore/            # 扩展 flow CRUD: 版本化(base_version/409)、engine_kind、ir_json/canvas_json 双列(Postgres)
│   ├── enginev1/             # runner-v1: NodeRegistry 构建 + Compile + LRU cache(key=flow_id,version,kind) + RunStream
│   ├── enginev2/             # runner-v2: v2 NodeRegistry + RegisterCodec + WithCheckpointStore + RunResumable/Resume(按 pinned version)
│   ├── nodes/                # 节点工厂(llm/builtin/rag/a2a/mcp/cond/approval) + schema.go(JSON Schema, 含 v2Only 标记) + port codec 注册
│   ├── comm/                 # A2A/MCP 客户端持有 + AsAgentTool(s) 包装
│   ├── checkpoint/           # Postgres CheckpointStore(实现 v2flow.CheckpointStore)
│   ├── httpapi/              # mux、中间件(auth→ws→rbac)、handlers、SSE writer
│   ├── storage/              # pgx 连接、迁移、SQLite dev 降级
│   └── obs/                  # otel TracerProvider、otelflow.Wrap 装配
└── web/                      # React SPA（全新构建）+ @xyflow/react
    └── src/{app/routes, features/{auth,workspace,flows,canvas,runs,hitl}, lib/sse.ts, components/ui}
```

**单一职责**：`enginev1`/`enginev2` 隔离两套引擎；`nodes` 是唯一定义节点类型与 schema 的地方（v1/v2 双注册）；`flowstore` 唯一处理 IR/canvas 拆分；`httpapi` 只编排+鉴权+SSE。

## 8. 节点类型与注册

每类型 = ① v1+v2 `NodeRegistry.Register` ② `nodes/schema.go` 一份 JSON Schema（驱动前端表单）。底层调用经 `agents.Tool`→`flow.FromAgentTool` 统一桥（cond/approval 除外）。

| 画布节点 | flow type | 配置 schema | 运行期真实调用 |
|---|---|---|---|
| LLM | `llm` | `{provider,model,system,temperature,tools[]?}` | providers 构造 `llm.ChatModel`；挂 tools 则 `NewReActAgent`(llm-agent 仓)；输出 `text`，token usage 走 `MetadataAware` |
| 内置工具 | `tool` | `{tool:"calculator\|note\|search\|terminal",args}` | builtin 构造→`FromAgentTool`→复用 flow `RegisterToolNode`（config 已是 `{tool,args}`，零改） |
| RAG 检索 | `rag` | `{collection,top_k,mode:"retrieve\|ask"}` | 持久 `rag.System`；`Retrieve` 或 `Ask`；输出 `hits`/`answer` |
| A2A 远程技能 | `a2a` | `{endpoint,skill,auth?}` | `a2a.NewClient`→`a2a.AsAgentTool`→`FromAgentTool`；运行即 `ExecuteSkill` 轮询 |
| MCP 工具 | `mcp` | `{transport,endpoint\|command+args,tool}` | `comm.New*Transport`→`mcp.NewClient.Initialize`→`mcp.AsAgentTools`→`FromAgentTool` |
| 条件/分支 | （无独立节点） | 在 **Edge.Condition** 写 CEL | `WithConditionEvaluator(eval)`（`eval, _ := cel.NewEvaluator()`）；`CondEnv.Value`=源端口字符串 |
| 审批(HITL) | `approval`（**v2-only**） | `{kind:"approval\|input\|confirm_dangerous",prompt,schema?}` | 复刻 `cmd/flowd/v2nodes.go approvalNode`：`Run` 返回 `Interrupt(InterruptRequest)` + `CanInterrupt()`；resume humanInput 注入输出端口 `decision`（`Kind` 枚举见 `v2/flow/interrupt.go`：`approval`/`input`/`confirm_dangerous`） |

**前端表单 schema 驱动**：`GET /api/node-types` 返回 `[{type,label,inputs[],outputs[],configSchema, v2Only}]`；前端 react-hook-form + JSON Schema 渲染节点 inspector，免逐节点写死表单。`v2Only`（如 approval）让前端在纯 v1 上下文隐藏该节点，后端保存分诊时校验"含 v2Only 节点 ⇒ engine_kind=v2"，否则 422（防 §5 的 `unknown type`）。

> **carrier 与 JSON 形状**：v1 carrier=`string`、v2=`any`，而对外 API 用 JSON。studio 定义自有 DTO：节点输入/输出在 API 层统一为 JSON 值，进 v1 引擎前 stringify、出引擎后按节点声明的 output 类型反序列化；**v2 跨 suspend 的非 string 端口须 `flow.RegisterCodec[T]`**（见 §17，否则 `ErrNotCheckpointable`）。
>
> **MCP/A2A 节点安全**：MCP `transport:"stdio"` 默认禁用（`exec.Command` 任意命令=RCE 面）；启用需 admin + 服务端命令白名单，用户**不能**在 flow JSON 自由填 `command`，只能引用 admin 预置的 server。MCP http / A2A endpoint 受 §17 的 SSRF 白名单约束。对齐 builtin terminal 的禁用/白名单门禁范式。

## 9. 运行与事件流

- **普通运行(v1)**：`POST /api/workspaces/{ws}/flows/{id}/run/stream` → `enginev1` 取/编译 engine(`otelflow.Wrap`)→`RunStream` → 每 `FlowEvent` ① `AppendRunEvent` 入库 ② SSE 推前端（`X-Run-ID` 首帧前 set）。前端 `useRunStream` reducer 画时间线（自建）。
- **Replay**：`POST /api/.../runs/{runID}/replay` → `ListRunEvents` 顺序回放 SSE（复刻 `handleReplayRun`），前端复用同一 reducer。
- **HITL 完整回路(v2)**：
  1. 运行含 approval 的 flow（按 engine_kind 自动置 resumable）→ **走非流式 `POST .../run`**（flowd 的 `streamHandler` 对 `req.Resumable=true` 显式拒绝："resumable runs are not supported on the streaming endpoint"，`server.go:436`——故 v2 run 不挂 SSE `/run/stream`）→ `runResumableV2`：engine 经 `otelflow/v2.Wrap` 装饰后须断言回 `v2flow.ResumableRunner` 才能调 `RunResumable`，跑到 approval 返回 `Suspended{ResumeToken,NodeID,Request}`。
  2. 后端合成 `node_interrupted`+`flow_suspended` 入 `run_events`，`SuspendRun` 置 `status=suspended`+存 token/node，checkpoint 已写 `checkpoints` 表；响应 `{run_id, suspended:{node,prompt,schema}}`。
  3. 前端经 **`GET /api/.../runs/{runID}/events/stream`（专用 v2 实时事件 SSE 端点）或轮询 `…/events`** 收到 `flow_suspended`（**注意：v2 run 本身走非流式 `POST .../run`，实时事件靠这个独立 events SSE 通道而非 run 端点的流式响应**）→ 弹审批弹窗（按 `InterruptRequest.Kind` 渲染，`Schema` 驱动）。
  4. 审批人提交 → `POST /api/.../runs/{runID}/resume`，**body 仅含 `humanInput`（resume token 服务端取自 `flow_runs.resume_token`，client 不传）；server 按中断节点的 `InterruptRequest.Schema` 校验 humanInput 值域**（Schema 在引擎层是 advisory，studio 自行强制）→ 取 `flow_runs.version` 编译对应版本 v2 engine（见 §17）→ `engine.Resume` 续跑 → 完成 `flow_done`+`FinishRun`+`DeleteCheckpoint`；再遇中断则再次 suspended（多级审批靠同 token 反复 Resume）。
  5. 前端收到 `flow_done` 刷新结果。
- **防重**：`status!=suspended→409` 守卫，防 resume 重放双触发副作用（直接采用 flowd 既有逻辑）。

## 10. 鉴权 / RBAC

- JWT/登录/refresh/argon2id/`Authenticator`(预留 OIDC) 全部由 `llm-agent-authz` 提供（studio 不自建）。
- 工作区上下文：路由前缀 `/api/workspaces/{ws}/...`，套 `authz.Authenticate → authz.RequireScopeRole("workspace", minRole)`（按 org级⊕workspace级合并取最高）；无成员/跨 org→403。
- 校验点（工作区级）：`viewer`=读 flows/runs/events/replay/画布只读；`editor`=+创建/编辑/删除 flow、触发 run；`admin`=+管理成员、HITL resume/审批、**启用 MCP stdio 节点**（见 §17 安全）。审批权限可做 flow 级可配，敏感流程限 admin。
- flow/run 归属：均带 `workspace_id`，**所有查询（含按 runID 直查的 resume/replay/events）强制 `WHERE workspace_id=:ws`** 防跨租户越权（仅凭 runID 不可跨 ws 读事件）。
- 下游凭据隔离：provider key / A2A token 只存后端配置，永不下发浏览器（遵循「剥离入站 auth + 服务端注入下游 auth」的 BFF 原则）。

## 11. 前端视图清单

| 路由 | 视图 | 核心组件/库 |
|---|---|---|
| `/login` | 登录 | react-hook-form |
| `/` | 工作区选择/管理 | WorkspaceSwitcher、成员管理(admin) |
| `/ws/$ws/flows` | flow 列表 | `@tanstack/react-table` FlowsTable（自建） |
| `/ws/$ws/flows/$id` | **DAG 画布编辑器** | **`@xyflow/react`** 画布；自定义 node 组件；节点 inspector(JSON-Schema 驱动)；边上 CEL 编辑器；保存拆 `ir_json`/`canvas_json` |
| `/ws/$ws/flows/$id/run/$runId` | 运行时间线 | TimelineView + NodeStatusList（自建 reducer + SSE 客户端） |
| 同上(suspended) | **HITL 审批** | ApprovalDialog：按 `InterruptRequest{Kind,Prompt,Schema}` 渲染→resume |
| `/ws/$ws/flows/$id/runs` | 运行历史 | RunsHistory 表（可点进 replay） |
| 同上 `?replay=1` | replay 回放 | 复用 TimelineView + replay SSE + 回放进度条 |

**DAG 库选型**：`@xyflow/react`（React Flow 12，React19 兼容、内置缩放/连线/自定义节点/minimap），真画布（而非 JSON 文本编辑）是 studio 的核心差异化。

## 12. 准生产级横切

- 鉴权：JWT + 工作区 RBAC（§10）；下游凭据服务端隔离。
- 运行隔离/配额：每 run `ctx` 超时+cancel；`WithMaxNodeConcurrency` 限层内并发；workspace 级并发 run 上限 + token bucket 速率限制；terminal 节点默认禁用，启用需 admin+白名单+沙箱目录。
- 可观测：`obs` 统一 otel `NewTracerProvider`；v1 `otelflow.Wrap`、v2 `otelflow/v2.Wrap`；run_id/workspace_id 作 span 属性。
- E2E：①auth+RBAC 越权矩阵；②flow CRUD+版本化往返(ir_json 过 `DisallowUnknownFields`)；③v1 run→SSE 时间线→replay；④v2 HITL run→suspend→resume→done 全回路 + 409 防重；⑤每类节点集成测试（A2A/MCP 用 `comm.NewInMemoryTransport(handler)` + 本地 server，免起进程）；⑥checkpoint 持久化 + StructHash 容忍 config 改动。
- docker-compose：`studio`(后端+静态前端) + `postgres` + `otel-collector`(+ 可选 jaeger/tempo)；dev 可去 postgres 改 SQLite 卷。

## 13. 风险与未决点

1. **v1 IR `DisallowUnknownFields`（高，已缓解）**：画布坐标绝不进 IR。方案：`flow_versions` 拆 `ir_json`/`canvas_json` 双列（§6）；保存时 `flow.Load`+`Validate` 校验纯净 IR，CI 回归守护。
2. **v1/v2 事件不对称（中）**：v1 真流式、v2 同步合成；含 HITL 的 flow 接受节点级时间线。token 流式+HITL 共存放 M4。
3. **v2 组件节点需自建（中，最大胶水）**：v2 `cmd/flowd/v2nodes.go` 仅注册 `passthrough`/`approval`；LLM/tool/RAG 等的 v2 工厂须 studio 自建（v2 `Deps` 为空 struct，靠闭包注入 ChatModel/Tool），对可中断节点实现 `InterruptCapable`。**走 IR+NodeRegistry 路线，不用 v2 typed graph builder。**
4. **CEL 仅见 `Value string`（低-中）**：复杂路由需把判据塞进上游单一输出字符串。已知限制。
5. **CheckpointStore 自实现 Postgres 版（低）**：接口仅 4 方法、schema 已给定，逐列照搬。
6. **IR schema 稳定性（中）**：pin flow 到 v0.1.x，CI 跑 apisnapshot 兼容，升级前评估。
7. **A2A 异步轮询延迟（低）**：长任务阻塞节点，节点层加超时；异步占位放 M3。

## 14. 里程碑

> **前置依赖**：studio 复用 `llm-agent-authz`（kb 已先行落地的 tag）；无需新建鉴权。

- **M1（v1 骨架）**：依赖 authz(登录/JWT/workspace RBAC) + flowstore(双列, `engine_kind` 恒置 v1) + enginev1 `RunStream`→SSE + 画布(@xyflow)读写 + tool/llm/cond 节点 + 时间线 + replay + **并发编辑 base_version/409**。**画布按 `/node-types` 的 `v2Only` 过滤——M1 不暴露 approval**（分诊函数 M3 接管）。
- **M2（节点全集）**：rag/a2a/mcp 节点（**MCP stdio 安全门禁随本里程碑同期落地, §17, 不拖到 M4**）+ `node-types` schema 端点(含 v2Only)驱动表单 + builtin 全量 + 运行历史/配额/otel + MCP 子进程 `defer Close` 回收。
- **M3（HITL）**：enginev2 + Postgres CheckpointStore + approval 节点 + suspend/resume 回路 + 审批弹窗 + 引擎分诊 + 409 防重 + E2E。
- **M4（准生产）**：docker-compose、限流/隔离、安全审查、token 流式与 HITL 共存评估。

## 15. 关键参考文件（实现期）

- `llm-agent-flow/flow/ir.go`（v1 IR 契约 + `DisallowUnknownFields` 约束）
- `llm-agent-flow/cmd/flowd/server/server.go`（双引擎调度、SSE、replay、HITL resume 可复刻蓝本）
- `llm-agent-flow/v2/flow/resume.go` + `v2/flow/checkpoint.go`（HITL RunResumable/Resume/CheckpointStore 真实 API）
- `llm-agent-customer-support/internal/flowrunner/flowrunner.go`（v0.3.0，稳定参照：进程内嵌 runner + otelflow.Wrap + 自定义节点注册范式）

## 16. 前端栈与"无前端参照"说明

生态内目前**没有已定型、可作为代码参照的全栈前端**（`llm-agent-console` 尚未定型，明确**不**作为引用代码）。studio 的 `web/` 是**全新构建**（greenfield）：
- 技术栈 React19/TS/Vite/Tailwind v4/shadcn/TanStack + `@xyflow/react` 是**基于自身需求的独立选型**，不依赖任何现有仓库代码。
- DAG 画布、SSE 客户端、运行时间线 reducer、各表单/表格组件均自建，不移植任何未定型仓库的代码。
- 后端参照仅限**已发版的稳定仓库**：`llm-agent-flow`（v0.1.4，含 `cmd/flowd/server` 双引擎 + HITL 蓝本、v1/v2 公共 API）、`llm-agent-customer-support`（v0.3.0 的 flowrunner 内嵌范式）、`llm-agent-comm`/`-builtin`/`-rag`/`-otel` 的公共 API。

## 17. 二轮评审修订（缺口闭合）

本节为二轮评审（`.planning/.specs-review-round2.md`）后的权威补订。

### 17.1 共享鉴权依赖（X1）
鉴权/租户基座迁至 `llm-agent-authz`（同目录 spec）。studio import 它、注入 `scope_kind="workspace"`；org/user/membership/session 与 `/api/auth/*` 由 authz 提供；角色统一 `admin/editor/viewer`(+`org_admin`)，与 kb 完全同构（仅 scope_kind 不同）。studio 复用 kb 已落地的 authz tag，不新建鉴权。

### 17.2 v2 port codec 注册（S1，HITL 关键）
v2 carrier=`any`，`encodePortValue` 对未注册 codec 的类型返回 `ErrNotCheckpointable`（codec.go:50），init 只内置 `string`（codec.go:80）。含 RAG/工具（结构化输出）+ approval 的 v2 flow 在 suspend 时会崩。**修法**：`nodes/` 在 v2 注册每个节点时，对其非 string 输出类型调 `flow.RegisterCodec[T](name)`；或强制所有跨节点 carrier 收敛为 `json.RawMessage` 并注册之。E2E 必含"RAG节点 + approval 的 flow suspend→resume→done"。

### 17.3 版本钉死的 resume / engine 缓存（S2）
flowd 蓝本 `engineForV2(rec.FlowID)` 无版本维度，新 IR 缺中断节点→`interrupt node not found`（resume.go:144），与 studio 版本化冲突。**修法**：engine LRU cache key = `(flow_id, version, engine_kind)`；`POST run` pin 当时 `latest_version` 写 `flow_runs.version`；resume 读 `flow_runs.version`→取 `flow_versions.ir_json`→编译**该版本** v2 engine→`Resume`。`otelflow/v2.Wrap` 后须 `w.(v2flow.ResumableRunner)` 断言才能拿 `RunResumable/Resume`。

### 17.4 出站/执行安全（S3）
- **MCP stdio = 任意命令 RCE**：默认禁用；启用需 admin + 服务端命令白名单（用户不能在 flow JSON 填 `command`，只引用 admin 预置 server）；或 v1 只支持 MCP http transport。对齐 builtin terminal 的 `ErrTerminalDisabled` 门禁范式。子进程 `defer transport.Close()` 回收，僵尸防护 + 并发 run 连带限制 fork 数。
- **MCP http / A2A endpoint SSRF**：限 http/https + DNS 解析后校私网/环回/链路本地 IP（防 rebinding）+ 超时 + 大小上限。
- **resume 注入**：humanInput 键须 ∈ 中断节点输出端口且**经 Schema 校验值域**（§9.4）；resume 权限 ≥ flow 配置的审批角色。

### 17.5 HTTP 端点目录（补全契约）
列表统一 cursor 分页。
- 鉴权（authz）：`POST /api/auth/{login,refresh,logout,logout-all}`
- 工作区/成员：`GET/POST /api/orgs/{org}/workspaces`、`GET /api/workspaces/{ws}`、成员 `GET/POST/DELETE /api/workspaces/{ws}/memberships`（admin）
- flow：`GET/POST /api/workspaces/{ws}/flows`、`GET/PUT/DELETE /api/workspaces/{ws}/flows/{id}`（PUT 带 `base_version`，冲突 409）、`GET …/flows/{id}/versions`
- 运行：`POST …/flows/{id}/run`（非流式，body pin version；含 v2Only→走 v2）、`POST …/flows/{id}/run/stream`（**仅非 HITL 的 v1 flow**，SSE）、`GET …/runs/{runId}`、`GET …/runs/{runId}/events`（分页）、`GET …/runs/{runId}/events/stream`（SSE 实时，v1+v2 通用）、`POST …/runs/{runId}/replay`（SSE）、`POST …/runs/{runId}/resume`（HITL，body=humanInput）、`POST …/runs/{runId}/cancel`
- 元数据：`GET /api/node-types`（含 `v2Only`）

### 17.6 状态/清理/漂移
- `flow_runs.status` 补 `canceled`（取消/超时/suspended-TTL 过期统一终态）+ `flow_err` 载原因。
- suspended run TTL 后台清扫：置 `canceled` + `DeleteCheckpoint`，防 checkpoint 表泄漏。
- node-types 单一真相源：v1 注册 / v2 注册 / JSON Schema / `/node-types` 由 `nodes/` 同一注册中心生成，CI 校验无漂移。

### 17.7 仍为 greenfield / 兼容提示
v2 全节点工厂（`Deps struct{}` 靠闭包注入 ChatModel/Tool）、port codec、版本化 engine cache、MCP 门禁、SSRF——均无现成样板。`flow.FromAgentTool` 跨 import 路径靠 `llm-agent-contract/agents` 别名兼容（adapter_llmagent.go:7-15），实现期注意 module 对齐。
