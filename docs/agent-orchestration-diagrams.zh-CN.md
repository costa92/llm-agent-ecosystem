# Agent 编排与记忆 Outbox 时序图

本文档用 Mermaid 图描绘 `llm-agent` 核心 Agent 范式、多 Agent 编排，以及持久化记忆 Outbox 子系统的运行机制，附实现位置（file:line）。

> 渲染说明：为兼容 VS Code 等较旧的 Mermaid 引擎，统一采用保守语法——子图用纯标题（不带引号标签）、节点标签用双引号、标签内不含 `<`/`>`、时序消息不含 `<br/>`。

## 一、ReAct 循环（双路径）

`react.go:82-199` — 构造时探测原生 tool-calling 能力，分两条路径。

```mermaid
flowchart TD
    Start(["Run input"]) --> Probe{"nativeToolCaller? react.go:63"}
    Probe -->|无能力| SP["Scratchpad 路径 react.go:93"]
    Probe -->|有能力| NT["原生 WithTools 路径 react.go:154"]

    subgraph 路径A_Scratchpad循环
        SP --> Gen["generateFromPrompt 含 scratchpad"]
        Gen --> Parse["parseReAct react.go:211"]
        Parse --> HasFinal{"有 Final?"}
        HasFinal -->|是| EmitF["StepFinal 返回"]
        HasFinal -->|否| Action["StepAction Tool+Args"]
        Action --> Exec["tools.Get.Execute"]
        Exec --> Obs["StepObservation"]
        Obs --> Append["追加 Observation 到 scratchpad"]
        Append --> Step{"步数未超 MaxSteps=8?"}
        Step -->|是| Gen
        Step -->|否| ErrMax["ErrMaxStepsExceeded"]
    end

    subgraph 路径B_原生单轮
        NT --> NGen["WithTools + Generate"]
        NGen --> HasTC{"有 ToolCalls?"}
        HasTC -->|否| NF["StepFinal resp.Text"]
        HasTC -->|是| Loop["逐个执行 ToolCall: StepAction+StepObservation"]
        Loop --> Join["拼接输出为答案"]
    end

    EmitF --> End(["Result"])
    NF --> End
    Join --> End
```

每次 `generateFromPrompt` 都经预算/策略收口（`agent_chatmodel.go:11-54`）。

## 二、Supervisor 调度循环（时序图）

`supervisor.go` — 本质是 `StateGraph[supervisorState]` 的 3 节点封装。

```mermaid
sequenceDiagram
    autonumber
    participant C as Caller
    participant S as Supervisor
    participant P as Planner
    participant W as Worker
    participant A as BuildAggregate

    C->>S: Run(ctx, task)

    loop 每轮 planNode supervisor.go:221
        S->>S: round++
        S->>P: Run(buildPlannerPrompt) 含历史与轮次
        P-->>S: planRes.Answer
        S->>S: ParseDispatch(answer)

        alt dispatch 为 nil 干净收尾
            Note over S: routeFromPlan 路由到 final
        else results 达到 MaxRounds
            Note over S: 优雅终止到 final 非错误
        else 继续 dispatchNode :245
            S->>W: Run(dispatch.Input)
            W-->>S: WorkerResult
            S->>S: results.append
            Note over S: 路由回 plan
        end
    end

    S->>A: BuildAggregate(results) finalNode :274 仅一次
    A-->>S: final.Answer
    S-->>C: Result Answer/Trace/Usage
```

**关键语义**：路由契约 `routeFromPlan`（`supervisor.go:289`）—— `dispatch==nil` 或 `len(results)>=MaxRounds` 都走 final 且**优雅**；内层 `WithMaxSteps(MaxRounds*3+4)` 保证 StateGraph 硬上限永不先触发。

## 三、StateGraph 状态流转（规划-执行-审查示例）

`graph.go:160-204` — 条件边优先于无条件边，允许成环。

```mermaid
stateDiagram-v2
    [*] --> plan
    plan --> execute
    execute --> cond
    cond --> review : st.Done 为 false
    cond --> done : st.Done 为 true
    review --> plan
    done --> [*]

    note right of execute
        每个节点 fn 接收 ctx 与 st 返回新 st 或 err
        节点错误带 节点名加步号
    end note
    note right of plan
        WithMaxSteps 默认100
        超限返回 ErrGraphMaxSteps
    end note
```

> 说明：`cond` 对应 `AddConditionalEdge`（条件边，优先于无条件边）；`done` 对应 `orchestrate.NodeEnd`。

## 四、流式与治理的统一收口（贯穿全部范式）

```mermaid
flowchart LR
    subgraph Agent或编排器
      Loop["内部阻塞循环"]
    end
    Loop -.闭包.-> RSF["runStreamFromBlocking agent.go:103"]
    RSF --> Goroutine["goroutine 推送"]
    Goroutine --> Ch["StepEvent chan buffer=16"]
    Ch --> Term{"终止优先级 err 先于 ctxErr 先于 Final"}

    Loop --> GFP["generateFromPrompt agent_chatmodel.go:11"]
    GFP --> B1["预扣 Charge Calls=1"]
    B1 --> Pol["policy.Wrap PreGenerate"]
    Pol --> M["model.Generate"]
    M --> Pol2["PostGenerate"]
    Pol2 --> B2["后扣 Charge Tokens"]
```

## 五、记忆 Outbox 端到端时序图

事务性 Outbox 跨 4 个仓库：contract（契约）/ postgres（后端）/ gateway（写入触发）/ worker（消费晋升）。

```mermaid
sequenceDiagram
    autonumber
    participant U as Caller
    participant G as Gateway
    participant DB as Postgres
    participant R as Relay
    participant Wk as WorkerPublisher

    Note over G,DB: 写入阶段 单事务原子写三表
    U->>G: WriteMemory(record, idempotency_key)
    G->>DB: WriteRecord store.go:59
    activate DB
    Note over DB: BEGIN
    DB->>DB: 幂等检查 命中则重放
    DB->>DB: INSERT memory_record
    DB->>DB: INSERT memory_event
    DB->>DB: INSERT outbox_event status=pending :149
    DB->>DB: INSERT memory_idempotency 快照
    Note over DB: COMMIT
    deactivate DB
    DB-->>G: WriteRecordResult
    G-->>U: memory_id 与 version

    Note over R,DB: 中继阶段 租约式领取 FOR UPDATE SKIP LOCKED
    loop RunOnce relay.go:203
        R->>DB: ClaimBatch relay.go:104
        DB-->>R: 返回 pending 或租约过期行 并置 processing 与 attempt++
        R->>Wk: Publish(OutboxMessage)
        activate Wk
        Wk->>Wk: 过滤 created/updated 且 Kind=working
        Wk->>DB: GetRecord 重读当前态 陈旧守卫
        Wk->>Wk: PromotionEligible 门槛判定
        Wk->>DB: ResolveDedupe 原子裁决赢家
        alt 赢家
            Wk->>DB: Promote kind=episodic 版本+1 幂等
        else 输家或不合格
            Wk->>Wk: 记拒绝指标
        end
        Wk-->>R: 投递结果
        deactivate Wk
        alt 成功
            R->>DB: Ack status=sent 清租约 :255
        else 失败 attempt 未达 Max
            R->>DB: Ack status=pending 待重投 :301
        else 失败 attempt 达 Max
            R->>DB: Ack status=failed 待运维 :281
        end
    end
```

**容错保证**：

| 机制 | 实现 | 作用 |
|---|---|---|
| 原子性 | record 与 outbox 同事务 `store.go:59-188` | 杜绝孤儿事件 |
| 并发安全 | `FOR UPDATE SKIP LOCKED` + 租约 `relay.go:104` | 多 worker 无锁竞争 |
| 崩溃恢复 | `lease_expires_at < NOW()` 回收 | 租约到期自动重投 |
| 幂等 | 确定性键 sha256(tenant、memory、eventid、promote) | 重投不重复晋升 |
| 运维兜底 | `RequeueFailed` `store.go:433` | failed 行重置 pending |
| 晋升一致 | 共享 `PromotionEligible` `promotion.go:22` | gateway/worker 判定相同 |

晋升门槛：`Source="user_saved"` 永远合格；`Source="agent_inferred"` 仅当 `Importance >= 0.7`。

## 编排模式选择速查

| 场景 | 模式 | 文件 |
|---|---|---|
| 线性 A→B→C 无分支 | Pipeline | `pipeline.go:25` |
| 规划→并行专家→汇总 | FanOutFanIn | `fanout.go:50` |
| 规划→派发循环（显式轮次） | Supervisor | `supervisor.go:48` |
| 多 Agent 涌现式协作 | RoundRobinChat | `roundrobin.go:28` |
| 任务分解+显式完成信号 | RolePlay | `roleplay.go:28` |
| 显式分支/循环/可观测状态 | StateGraph[S] | `graph.go:29` |
