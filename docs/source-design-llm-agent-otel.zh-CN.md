# `llm-agent-otel` 子项目源码设计文档（中文版）

> 文档版本：2026-05-21  
> 范围：`llm-agent-otel/` 全部 25 个 `.go` 源文件（约 2.9K 行，含测试）  
> 阅读姿态：源码级深读 + 系统级设计点评；所有断言均带 `file:line` 锚点

---

## 0. 文档导航

1. [概述与定位](#1-概述与定位)
2. [设计思想（K3 keystone）](#2-设计思想k3-keystone)
3. [核心 wrapper 设计](#3-核心-wrapper-设计)
4. [Exporters / Metrics / Slog](#4-exporters--metrics--slog)
5. [GenAI semconv 与 attribute 命名](#5-genai-semconv-与-attribute-命名)
6. [`cmd` / `compose` 运行栈](#6-cmd--compose-运行栈)
7. [关键时序图](#7-关键时序图)
8. [设计优化与替代方案](#8-设计优化与替代方案)
9. [遗留与未来方向](#9-遗留与未来方向)

---

## 1. 概述与定位

`llm-agent-otel` 是 4-repo umbrella 中的「装饰器层」(decorator layer)，按 umbrella README 定义：

> `llm-agent-otel/  # capability-preserving OpenTelemetry wrappers`
> — `README.md:23`

它解决的是一组明确的工程问题：

| 维度 | 说明 |
|------|------|
| **核心目标** | 给 `llm.ChatModel` / `agents.Agent` / `flow.Runner` / `rag.System` 增加 OTel spans/metrics/logs，**不动核心仓库源码**。 |
| **关键不变式** | 核心 `llm-agent` 仓库坚持 stdlib-only；OTel SDK 不允许进入核心。本仓承担「重」依赖的容纳工作。 |
| **隔离方式** | `Wrap(inner) -> outer` 的纯结构装饰，inner 完全无感知；wrapper 自己实现接口 + 委托 inner。 |
| **能力保真** | 包装后的对象**不能损失** `ToolCaller` / `Embedder` / `StructuredOutputs` 等可选能力（capability-preserving）。 |
| **opt-in 闸门** | 默认输出**最小**信号面；`gen_ai.*` 属性与 prompt 内容捕获均靠 env opt-in。 |
| **导出器** | 自带 OTLP HTTP + gRPC 两种 exporter wiring，默认 HTTP `localhost:4318`。 |
| **演示栈** | `compose/compose.yaml` 拉起 `grafana/otel-lgtm` 单容器 LGTM 全家桶。 |

依赖关系（依据 `go.mod`）：

```
github.com/costa92/llm-agent-otel
  ├── github.com/costa92/llm-agent v0.5.1
  ├── github.com/costa92/llm-agent-rag v1.0.1
  ├── github.com/costa92/llm-agent-flow v0.0.7
  └── go.opentelemetry.io/otel v1.43.0  + exporters/sdk/metric/trace
```
— `go.mod:1-16`

注意：核心 `llm-agent` **不依赖**本仓；依赖箭头永远是「应用层依赖 otel 装饰器，装饰器依赖核心」，从而保住核心的 stdlib-only。

---

## 2. 设计思想（K3 keystone）

### 2.1 K3：「OTel attaches as decorator wrappers, never hooks」

umbrella 顶层 README 把这条规则上升为 keystone：

> 6. **OTel attaches as decorator wrappers, never hooks** —
>    `otelmodel.Wrap(inner) ChatModel`. (Keystone K3.)
> — `README.md:84-85`

这是一条「设计风格」+「依赖约束」叠加的硬约束，涵义可拆为四层：

1. **依赖方向硬约束**：核心库内不引入 `go.opentelemetry.io/otel`，更不引入「callback / hook 注册中心」之类需要侵入核心的概念。OTel 永远在外层装饰器仓库里，调用方向**单向**：装饰器 → 核心。
2. **API 形态约束**：装饰器以 **`Wrap(inner) outer`** 的纯函数姿态出现，不通过 `WithObserver(...)` / `RegisterHook(...)` 这类可变态注入。结果是：
   - 包装是**编译期可见**的（grep 全工程 `Wrap` 就能枚举所有 instrumentation 点）；
   - 包装位置由**调用方**自由选择，框架内部不预埋切点；
   - 包装的 telemetry 与未包装的核心**二进制级一致**（核心代码路径不变）。
3. **能力保真**：装饰器必须**保留**被包装对象的所有可选接口（`ToolCaller` / `Embedder` / `StructuredOutputs`），否则下游 `agents` 层就会因 `_, ok := model.(llm.ToolCaller); ok == false` 而退化能力——这就违反了「装饰器透明」语义。
4. **stdlib-only 友好**：装饰器仓允许 OTel SDK 这类「重」依赖；核心仓不允许。这条规则的真正成本由 `llm-agent-otel` 承担——下面 §3 的 8 个 wrapper 子结构、`semconv_gen_ai.go` 的 env opt-in 等都是它的副作用。

### 2.2 hook 模式被舍弃的原因

理论上，OTel 可以用「在 `llm.ChatModel.Generate` 里调用 `lifecycle hooks`」实现：

```
// 反例（核心库内的伪代码）
type Hooks struct { Before, After func(...) }
func (m *model) Generate(...) {
    if m.hooks.Before != nil { m.hooks.Before(...) }
    resp, err := m.doGenerate(...)
    if m.hooks.After != nil { m.hooks.After(..., resp, err) }
}
```

K3 拒绝这条路径，**至少**有以下理由：

- **侵入**：核心库必须新增 `Hooks` 字段、构造函数选项、文档约定。所有 provider 都要遵守同一 hook 协议，否则 trace 不全。
- **风险面**：hook 可能抛 panic、阻塞、修改 request——一行下游代码就能让核心库失稳。
- **依赖泄漏**：如果 hook 想直接生成 OTel span，hook 实现就引入了 OTel 类型；即使做成「抽象 hook 类型 + 适配器」，核心也得维护一个不被使用的抽象层。
- **可观测性维护**：升级 GenAI semconv 时，需要改核心库（多仓联动）。装饰器化后只改一仓即可。

### 2.3 与 stdlib-only 的关系

核心 `llm-agent` 是 stdlib-only 的（umbrella 顶层 keystone）。OTel SDK 是允许的依赖**但**只能存在于本仓。`go.mod` 明确：核心依赖在 `require` 第一行，OTel SDK 全套在第二段——只有外层装饰器才碰得到 (`go.mod:5,9-15`)。

---

## 3. 核心 wrapper 设计

### 3.1 `otelmodel.Wrap` —— 8 子结构的 capability 矩阵

`otelmodel/otelmodel.go` 是整个仓库设计最精巧的部分。其原因在于 `llm.ChatModel` 有 3 个互相独立的可选接口（`ToolCaller` / `Embedder` / `StructuredOutputs`），形成 2³ = 8 种能力组合。如果只写一个 wrapper：

```go
type wrapper struct { inner llm.ChatModel }
// Implements ChatModel, plus optionally ToolCaller/Embedder/StructuredOutputs
```

则 `_, ok := wrapped.(llm.ToolCaller)` 永远为 true（即使 inner 不支持），下游 `agents` 会错误地以为模型可用工具调用。Go 的接口断言机制要求**实际结构体类型**对应**真实方法集**——所以必须为每种 inner 能力组合各造一个独立结构体类型。

**实现拆解**（`otelmodel/otelmodel.go:14-49`）：

```go
type wrapper struct { inner; tp; tracer }      // ChatModel only

func Wrap(model llm.ChatModel, opts ...Config) llm.ChatModel {
    base := &wrapper{...}
    // 通过逐级类型断言决定返回哪一个具体结构
    if tc, ok := model.(llm.ToolCaller); ok {
        if emb, ok := model.(llm.Embedder); ok {
            if so, ok := model.(llm.StructuredOutputs); ok {
                return &toolEmbedSchemaWrapper{...}   // 7. tool+emb+schema
            }
            return &toolEmbedWrapper{...}             // 6. tool+emb
        }
        if so, ok := model.(llm.StructuredOutputs); ok {
            return &toolSchemaWrapper{...}            // 5. tool+schema
        }
        return &toolWrapper{...}                      // 4. tool
    }
    if emb, ok := model.(llm.Embedder); ok {
        if so, ok := model.(llm.StructuredOutputs); ok {
            return &embedSchemaWrapper{...}           // 3. emb+schema
        }
        return &embedWrapper{...}                     // 2. emb
    }
    if so, ok := model.(llm.StructuredOutputs); ok {
        return &schemaWrapper{...}                    // 1. schema
    }
    return base                                       // 0. ChatModel only
}
```

文末的接口断言锁定（`otelmodel/otelmodel.go:300-321`）：

```go
var (
    _ llm.ChatModel         = (*wrapper)(nil)
    _ llm.ChatModel         = (*toolWrapper)(nil)
    _ llm.ToolCaller        = (*toolWrapper)(nil)
    ...
    _ llm.ChatModel         = (*toolEmbedSchemaWrapper)(nil)
    _ llm.ToolCaller        = (*toolEmbedSchemaWrapper)(nil)
    _ llm.Embedder          = (*toolEmbedSchemaWrapper)(nil)
    _ llm.StructuredOutputs = (*toolEmbedSchemaWrapper)(nil)
)
```

测试守住这条不变式（`otelmodel/otelmodel_test.go:22-41`）：

```go
func TestWrap_PreservesCapabilities(t *testing.T) {
    model := llm.NewScriptedLLM(...,
        llm.WithCapabilities(llm.Capabilities{Tools: true, Embeddings: true, StructuredOutputs: true}))
    wrapped := Wrap(model, cfg)
    if _, ok := wrapped.(llm.ToolCaller); !ok { t.Fatal(...) }
    if _, ok := wrapped.(llm.Embedder); !ok { t.Fatal(...) }
    if _, ok := wrapped.(llm.StructuredOutputs); !ok { t.Fatal(...) }
}
```

### 3.2 span 生命周期：`Generate` vs `Stream`

**Generate 路径**（同步，`otelmodel/otelmodel.go:51-74`）：

1. `tracer.Start(ctx, "chat "+w.inner.Info().Model)` —— span 名以 model 名结尾，符合 GenAI semconv「`gen_ai.operation.name SP gen_ai.request.model`」shape。
2. `defer span.End()` 单出口。
3. 写入入参属性 `gen_ai.system` + `gen_ai.request.model`（仅在 `SemconvEnabled()` 真时才落地，见 §5）。
4. 调用 inner，失败时 `span.RecordError(err)` + `span.SetStatus(codes.Error, err.Error())`。
5. 成功后补 `gen_ai.usage.{input,output}_tokens`、`gen_ai.usage.source`、`gen_ai.response.finish_reason`。

**Stream 路径**（异步，`otelmodel/otelmodel.go:76-147`）：

1. 启动 span，**不 defer End**——因为 stream 由读取者驱动结束。
2. 包装 `llm.StreamReader` 为本仓 `streamReader{inner, span, sawContent, closed}`。
3. `Next()` 内：
   - 首个非 `EventDone` 事件触发 `span.AddEvent("gen_ai.first_token")` —— 用 span event 取代「Time To First Token」直方图，更省心，但牺牲了易聚合性（见 §8）。
   - `EventDone` 且 `Usage != nil` 时补写 usage，调 `end()` 关闭 span。
4. `Close()` 兜底 `end()`，`closed` 标位幂等。
5. 错误路径上 `RecordError + SetStatus` 后立即 `end()`，避免泄漏 span。

测试覆盖（`otelmodel/otelmodel_test.go:77-122`）确保 stream 单 span + 单 first-token event。

### 3.3 `WithTools` / `WithSchema` 的「装饰传递」

关键设计点：`llm.ToolCaller.WithTools(tools)` 返回的是**一个新的 ChatModel**（绑定了工具的）。如果不重新包装，下游就丢了 telemetry。`wrapper.wrap` 方法（`otelmodel/otelmodel.go:98-100`）专门做这件事：

```go
func (w *wrapper) wrap(next llm.ChatModel) llm.ChatModel {
    return Wrap(next, Config{TracerProvider: w.tp})
}
```

每个含 `WithTools` / `WithSchema` 的子 wrapper 都会调用 `w.wrap(next)` 重新分类（`otelmodel/otelmodel.go:155-298`）。例如：

```go
func (w *toolWrapper) WithTools(tools []llm.Tool) (llm.ToolCaller, error) {
    next, err := w.toolCaller.WithTools(tools)
    if err != nil { return nil, err }
    wrapped := w.wrap(next)
    tc, _ := wrapped.(llm.ToolCaller)
    return tc, nil
}
```

— `otelmodel/otelmodel.go:154-162`

值得注意的细节：`Embed` 在 `toolEmbedWrapper` / `toolEmbedSchemaWrapper` 等组合体里**没有重新分类**，而是直接构造 `&embedWrapper{wrapper: w.wrapper, embedder: w.embedder}` 来委托——因为 `Embed` 不产生新的模型对象（`otelmodel/otelmodel.go:221-223`）。

### 3.4 `otelagent.Wrap` —— 流式步骤 → span tree

`otelagent/otelagent.go` 把 `agents.Agent` 的 ReAct 步骤流变成树形 span：

- 根 span：`invoke_agent <Name>` 带 `agent.name` 属性（`otelagent/otelagent.go:36-38`）。
- 内部 `runBuilder.consume(step)`（`otelagent/otelagent.go:122-143`）按 `StepKind` 翻译：

| StepKind | 动作 |
|---|---|
| `StepThought` / `StepPlan` / `StepReflection` | `closeToolSpan` + `startChatSpan` |
| `StepFinal` | 同上 + 写 `answer` + `closeChatSpan` |
| `StepAction` | `closeChatSpan` + `closeToolSpan` + 开 `execute_tool <name>` |
| `StepObservation` | `closeToolSpan` |

`startChatSpan` 用单调递增的 `agent.llm_call.index` 区分多次 LLM 调用（`otelagent/otelagent.go:145-152`）。

设计巧思：**整个 wrapper 不修改 `Run` 的返回值结构**，只把流式事件**镜像**到 OTel span 树。`Run` 内部通过 `inner.RunStream` 拿事件流，自己 reconstruct `Result`（`otelagent/otelagent.go:35-71`）。这意味着即使 inner 实现了直接的 `Run`（非流式），wrapper 也会绕道用 `RunStream`——一种「为了 telemetry 而绕路」的取舍，下游同步用户感知不到。

**`RunStream` 路径**（`otelagent/otelagent.go:73-110`）则透传事件、**异步**镜像 span，关闭逻辑由 `defer close(out); defer span.End()` 把控。

ReAct 测试（`otelagent/otelagent_test.go:74-120`）期望典型形状：

```
invoke_agent react
├── chat                       (LLM call #1: Thought→Action)
├── execute_tool calc          (Tool exec)
└── chat                       (LLM call #2: Thought→Final)
```

并由 `assertParentChildCount(t, spans, "invoke_agent react", "chat", 2)` 锁定子计数。

### 3.5 `otelflow.Wrap` —— flow 节点 → span tree

`otelflow/otelflow.go` 包装 `llm-agent-flow/flow.Runner`，是这套 wrapper 模板里**最完整**的一个，因为：

- 它演示了**两种**根 span 名（`flow.run` / `flow.run.stream`）；
- 它实现了 `NodeStarted` / `NodeFinished` / `NodeSkipped` / `FlowDone` / `FlowErr` 全套事件 → span 映射；
- 它处理了上下文取消、未关闭 span 的兜底关闭、流式转发的非阻塞性。

`flowIdentifier` 接口（`otelflow/otelflow.go:28-31`）做了「鸭子类型 + escape hatch」组合：

```go
type flowIdentifier interface {
    FlowID() string
    FlowName() string
}
```

`Engine` 满足此接口；若用户传了非 `Engine` 的自定义 `flow.Runner`，可用 `Config.FlowID` 覆盖。`Wrap` 在初始化时把 id/name 缓存到结构体里（`otelflow/otelflow.go:38-55`）。

**`RunStream` 的 goroutine 边界**（`otelflow/otelflow.go:99-152`）：

- 内 chan → 外 chan 的复制由 goroutine 完成；
- 外 chan 在 inner 关闭后才 close，保证消费者看到完整事件流；
- `nodeSpans map[string]trace.Span` 跟踪开放的 child span；
- `defer` 块在 goroutine 退出时关闭所有遗留 span，并标 `Error` 状态——这是优雅处理 ctx 取消的标准做法（`otelflow/otelflow.go:123-128`）。

`NodeSkipped` 的处理别具一格：开启 + 立即关闭一个**零时长 span**，并标 `flow.node.skipped=true`（`otelflow/otelflow.go:183-192`）。doc.go 解释：

> Skipped nodes (CEL guard evaluated false) produce a zero-duration child span with the `flow.node.skipped` attribute set to true so the trace makes the topology explicit instead of silently omitting the node.
> — `otelflow/doc.go:13-18`

这个决策值得点赞：拓扑显式 > 计数省略。

### 3.6 `otelrag.Wrap` —— RAG 操作 + Observer 双形态

`otelrag/otelrag.go` 提供两种集成方式：

**形态 A：包装器（Wrapper struct）**

`Wrapper.Import / Retrieve / Ask` 三个公共方法，每个都：
1. 开 `rag.import` / `rag.retrieve` / `rag.ask` span；
2. 写 namespace / top_k 等入参属性；
3. 委托 inner；
4. 错误时 `RecordError + SetStatus(Error)`；
5. 成功后写出 hit count / route policy 等出参属性；
6. 调 `instruments.recordOp(...)` 写 RED 指标。

`Ask` 多做一件事——把 `ans.Diagnostics.Metrics.Tokens` 翻译成 `rag.tokens` 计数器（`otelrag/otelrag.go:152-157`）。

**形态 B：Observer（rag.Observer 注入到 rag.System）**

`Observer(cfg ...Config) rag.Observer` 返回三个回调，每个回调把 trace 转成**当前活跃 span 的 event**（`otelrag/otelrag.go:184-232`）。

> 当用户已经在外层有自己的 span（比如 HTTP handler），又不想再多一层 wrapper，那么注入 Observer 即可在外层 span 上加 event。

这是对 K3 的**良性破例**：Observer 不是「核心 hook」（rag 模块自己设计了 Observer 接口，与 OTel 解耦），otelrag 只是顺手实现了一个 OTel-aware Observer 实例——核心 rag 库照样无 OTel 依赖。

### 3.7 RAG metrics: `instruments` 与 noop 降级

`otelrag/metrics.go:42-71` 构造 4 个 instrument：

| 名称 | 类型 | 用途 |
|---|---|---|
| `rag.requests` | Int64Counter | RED：请求计数 |
| `rag.errors` | Int64Counter | RED：错误计数 |
| `rag.operation.duration` | Float64Histogram | RED：操作 + 阶段时延（ms） |
| `rag.tokens` | Int64Counter | 成本：token 计数（按 `prompt`/`completion` 切分） |

任一 instrument 构造失败时静默替换为 noop（`otelrag/metrics.go:53-69`）——「never fails」契约。这条契约让 `Wrap` 不需要返回 `error`，把错误处理推到「telemetry 端」而非「业务端」。

`recordOp` 不仅记主时长，还展开 `stages []obs.StageTiming` 逐 stage 入直方图，把 RAG 内部「retrieval/rerank/generate」阶段细分透出。

---

## 4. Exporters / Metrics / Slog

### 4.1 OTLP Exporter 默认值

`exporters.go:22-28`：

```go
func DefaultExporterConfig() ExporterConfig {
    return ExporterConfig{
        Protocol: ProtocolHTTP,
        Endpoint: "http://localhost:4318",
        Insecure: true,
    }
}
```

`newSpanExporter` 按 `Protocol` 字段切分到两个文件：

- `exporters_http.go:12-21` —— `otlptracehttp.New` + `trimHTTPPrefix`（剥 `http://`/`https://`，再 fallback 到 `url.Parse`）。
- `exporters_grpc.go:12-21` —— 镜像逻辑，调 `otlptracegrpc.New`。

两个 `trim*` 函数语义重复（`exporters_http.go:23-31` 和 `exporters_grpc.go:23-31`），属于设计上的微小冗余。理想做法是抽到 `exporters.go` 共享，见 §8.4。

`NewTracerProvider` 永远把 exporter 包成 `trace.WithBatcher(exporter)`（`exporters.go:35`）——这是合理默认（性能优先），但**没有暴露 sampler 选项**，详见 §8.2。

测试守住 4 件事（`exporters_test.go:11-69`）：默认 HTTP 4318、gRPC opt-in、`NewTracerProvider` 不返回 nil、README 必须包含若干关键词。

### 4.2 `otelmetrics.Recorder` —— GenAI 客户端指标

`otelmetrics/otelmetrics.go:19-26` 五件套：

| 字段 | OTel 名 | 类别 |
|---|---|---|
| `tokenUsage` | `gen_ai.client.token.usage` | Counter |
| `opDuration` | `gen_ai.client.operation.duration` | Histogram |
| `timeToFirst` | `gen_ai.client.operation.time_to_first_chunk` | Histogram |
| `agentIterations` | `agent.iterations` | Counter |
| `toolInvocations` | `agent.tool.invocations` | Counter |

`MessageAttributes(input, output string)` 配合 `ContentCaptureEnabled` 闸门 + `RedactText` 脱敏后才输出 `gen_ai.input.messages` / `gen_ai.output.messages`（`otelmetrics/otelmetrics.go:83-91`）。

**关键设计：基数控制**

`filterMetricAttrs`（`otelmetrics/otelmetrics.go:113-130`）白名单：

```go
case otelroot.AttrSystem, otelroot.AttrRequestModel, otelroot.AttrOperation,
     otelroot.AttrErrorType, otelroot.AttrFinishReason, otelroot.AttrServerAddr:
    out = append(out, kv)
```

`user.id` / `session.id` 等高基数属性**禁止**进 metric attribute set——测试 `TestCardinality_UserIDsDoNotExplodeMetricAttributes`（`otelmetrics/otelmetrics_test.go:43-67`）灌入 1000 个 user/session 组合，断言 metric attribute combinations ≤ 50。这是非常正确的基数防爆设计。

### 4.3 `otelslog.Handler` —— slog → trace 关联

`otelslog/otelslog.go:27-36`：

```go
func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
    sc := trace.SpanContextFromContext(ctx)
    if sc.IsValid() {
        r.AddAttrs(
            slog.String("trace_id", sc.TraceID().String()),
            slog.String("span_id", sc.SpanID().String()),
        )
    }
    return h.next.Handle(ctx, r)
}
```

逻辑极简：从 ctx 取活跃 SpanContext，把 trace_id/span_id 注入 slog record。`WithAttrs` / `WithGroup` 透传 next handler 以保持 slog 链路语义（`otelslog/otelslog.go:38-44`）。

未做：`severity_number` 翻译、OTel `LogRecord` 协议输出、log-bridge SDK 接入。这是个**极轻量的关联桥**，不是完整的 log signal pipeline。详见 §8.3。

---

## 5. GenAI semconv 与 attribute 命名

### 5.1 属性键集中定义

`semconv_gen_ai.go:9-30` 是仓库的**单一真理源**：

```go
const (
    AttrSystem         = "gen_ai.system"
    AttrRequestModel   = "gen_ai.request.model"
    AttrUsageInput     = "gen_ai.usage.input_tokens"
    AttrUsageOutput    = "gen_ai.usage.output_tokens"
    AttrUsageSource    = "gen_ai.usage.source"
    AttrFinishReason   = "gen_ai.response.finish_reason"
    AttrOperation      = "gen_ai.operation.name"
    AttrServerAddr     = "server.address"
    AttrErrorType      = "error.type"
    AttrUserID         = "user.id"
    AttrSessionID      = "session.id"
    AttrInputMessages  = "gen_ai.input.messages"
    AttrOutputMessages = "gen_ai.output.messages"

    EventFirstToken = "gen_ai.first_token"

    MetricClientTokenUsage        = "gen_ai.client.token.usage"
    MetricClientOperationDuration = "gen_ai.client.operation.duration"
    MetricClientOperationTTFT     = "gen_ai.client.operation.time_to_first_chunk"
    MetricAgentIterations         = "agent.iterations"
    MetricAgentToolInvocations    = "agent.tool.invocations"
)
```

字段命名跟随 OpenTelemetry 的 **gen_ai semconv (experimental)** 规范——`gen_ai.system`、`gen_ai.request.model`、`gen_ai.usage.input_tokens` 等都是上游标准条目。`agent.iterations` / `agent.tool.invocations` 不在上游标准内（agent 概念在 semconv 中尚未稳定），保持自有命名但与上游同前缀风格一致。

`otelmodel/semconv_gen_ai.go` 只是个 3 行 placeholder（仅含 `instrumentationName` 常量）；真正的属性键都在 root 包（`otelmodel/semconv_gen_ai.go:1-3`），子包通过 `otelroot.AttrXxx` 引用。

### 5.2 双重 opt-in 闸门

```go
func SemconvEnabled() bool {
    return strings.Contains(os.Getenv(semconvOptInEnv), semconvOptInValue)
}
func ContentCaptureEnabled() bool {
    v := strings.TrimSpace(strings.ToLower(os.Getenv(contentCaptureEnv)))
    return v == "1" || v == "true" || v == "yes" || v == "on"
}
```
— `semconv_gen_ai.go:37-44`

| 环境变量 | 默认 | 控制对象 |
|---|---|---|
| `OTEL_SEMCONV_STABILITY_OPT_IN=gen_ai_latest_experimental` | off | 所有 `gen_ai.*` 属性写入 |
| `OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT=true/1/yes/on` | off | prompt/response 文本捕获 |

`otelmodel/otelmodel.go:323-328` 的 `setSemconvAttrs` 用 `SemconvEnabled` 做门：

```go
func setSemconvAttrs(span trace.Span, attrs ...attribute.KeyValue) {
    if !otelroot.SemconvEnabled() { return }
    span.SetAttributes(attrs...)
}
```

设计含义：**默认部署**只看到 span 名 + 状态，看不到任何 `gen_ai.*` 标签——符合「保守默认 + 显式 opt-in」的隐私/合规倾向。这与 OTel 上游建议一致。

### 5.3 内容捕获 + 脱敏

`semconv_gen_ai.go:46-58`：

```go
var (
    emailPattern = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
    keyPattern   = regexp.MustCompile(`\b(?:sk|api[_-]?key)[-_A-Za-z0-9]*\b`)
)
func RedactText(s string) string {
    s = emailPattern.ReplaceAllString(s, "[REDACTED_EMAIL]")
    s = keyPattern.ReplaceAllString(s, "[REDACTED_SECRET]")
    return s
}
```

两条简单正则覆盖 email + sk/api-key 类 token。注意：

- 只在 `otelmetrics.MessageAttributes` 中被调用（`otelmetrics/otelmetrics.go:87-89`）；
- **otelmodel 当前没有把 prompt/response 文本写入 span**——只写 token 计数 + finish reason。这是一种**主动的最小化暴露**。
- 如未来要在 span 上加 `gen_ai.input.messages`，应统一走 `RedactText`，避免在 attribute 层再次出现「裸文本」。

`semconv_gen_ai_test.go:33-43` 用「`api key sk-1234567890 and email test@example.com`」做端到端验证。

---

## 6. `cmd` / `compose` 运行栈

仓库没有 `cmd/` 目录；唯一可执行入口在 `compose/demo/main.go`（46 行）。流程：

1. `NewTracerProvider(ctx, DefaultExporterConfig())` —— OTLP HTTP 默认端点。
2. `llm.NewScriptedLLM(...)` 造一个固定回复 `"hello from demo"` 的 mock model。
3. `otelmodel.Wrap(model, ...)` 装饰 model。
4. `agents.NewSimpleAgent(wrappedModel, ...)` 造 agent。
5. `otelagent.Wrap(agent, ...)` 装饰 agent。
6. `otelslog.NewHandler(slog.NewJSONHandler(...))` 装饰 slog。
7. `wrappedAgent.Run(ctx, "hello")` 触发一次完整端到端 trace。
8. `time.Sleep(2 * time.Second)` 等批量 exporter flush。

— `compose/demo/main.go:17-46`

`compose/compose.yaml:1-21` 把 demo 接到 `grafana/otel-lgtm` 单容器全家桶：

```yaml
services:
  otel-lgtm:
    image: grafana/otel-lgtm:latest
    ports: ["3000:3000", "4317:4317", "4318:4318"]
  demo:
    image: golang:1.26
    environment:
      OTEL_EXPORTER_OTLP_ENDPOINT: http://otel-lgtm:4318
      OTEL_SEMCONV_STABILITY_OPT_IN: gen_ai_latest_experimental
    command: > sh -lc "cd compose/demo && go run ."
```

值得注意的 DX 折点：

- **port 可参数化**：`${OTEL_DEMO_GRAFANA_PORT:-3000}` 允许多 demo 共存（`compose/compose.yaml:5-7`）。
- **env opt-in 默认开**：演示场景特意把 `OTEL_SEMCONV_STABILITY_OPT_IN` 打开（`compose/compose.yaml:16`），让 LGTM UI 上能直接看到 `gen_ai.*` 标签。
- **go 1.26 镜像**：与 `go.mod` 中 `go 1.26.0` 对齐（`go.mod:3`），但生产部署应注意 `golang:1.26` 是 dev 镜像，体积较大。

`scripts/workspace.sh` 处理多仓 local dev——为 4 个 sibling clone 写 `../go.work`，是 INFRA-06 跨仓迭代模式的一部分（`scripts/workspace.sh:1-41`）。CI 默认 `GOWORK=off`（umbrella README L77）。

---

## 7. 关键时序图

### 7.1 `ChatModel.Generate` 在 wrapper 中的 span 生命周期

```mermaid
sequenceDiagram
    autonumber
    participant App as Caller
    participant W as otelmodel.wrapper
    participant Tr as Tracer
    participant Span
    participant Inner as llm.ChatModel

    App->>W: Generate(ctx, req)
    W->>Tr: Start(ctx, "chat "+Info().Model)
    Tr-->>W: ctx, span
    W->>Span: setSemconvAttrs(gen_ai.system, gen_ai.request.model)
    Note over Span: 仅在 SemconvEnabled() 真时写入
    W->>Inner: Generate(ctx, req)
    alt success
        Inner-->>W: resp, nil
        W->>Span: setSemconvAttrs(usage.input/output, source, finish_reason)
        W-->>App: resp, nil
    else error
        Inner-->>W: resp, err
        W->>Span: RecordError(err); SetStatus(Error)
        W-->>App: resp, err
    end
    W->>Span: End() (defer)
```

### 7.2 `ChatModel.Stream` 与 `gen_ai.first_token` event

```mermaid
sequenceDiagram
    autonumber
    participant App as Caller
    participant W as otelmodel.wrapper
    participant SR as streamReader
    participant Span
    participant Inner as llm.ChatModel.Stream

    App->>W: Stream(ctx, req)
    W->>Span: Start("chat "+Model); setSemconvAttrs(...)
    W->>Inner: Stream(ctx, req)
    Inner-->>W: inner-reader
    W-->>App: streamReader{inner, span}
    loop until EOF / Done
        App->>SR: Next()
        SR->>Inner: Next()
        alt first non-Done chunk
            SR->>Span: AddEvent("gen_ai.first_token")
        end
        alt EventDone with Usage
            SR->>Span: setSemconvAttrs(usage, finish_reason)
            SR->>Span: End() (via end())
        else io.EOF
            SR->>Span: End() (via end())
        else error
            SR->>Span: RecordError; SetStatus(Error); End()
        end
        SR-->>App: event/err
    end
    App->>SR: Close()
    SR->>Span: End() (idempotent via end())
```

### 7.3 `agents.Agent.Run`（ReAct）的 span tree

```mermaid
sequenceDiagram
    autonumber
    participant App as Caller
    participant AW as otelagent.wrapper
    participant Root as invoke_agent
    participant Chat as chat span
    participant Tool as execute_tool span
    participant Inner as agents.Agent.RunStream

    App->>AW: Run(ctx, input)
    AW->>Root: Start("invoke_agent <name>")
    AW->>Inner: RunStream(ctx, input)
    loop event ev
        alt StepThought / Plan / Reflection
            AW->>Chat: Start("chat") (lazy)
        else StepAction
            AW->>Chat: End()
            AW->>Tool: Start("execute_tool <tool>")
        else StepObservation
            AW->>Tool: End()
        else StepFinal
            AW->>Chat: End() (capture answer)
        else Done w/ Err
            AW->>Root: RecordError; SetStatus(Error)
        end
    end
    AW->>Root: End() (defer)
    AW-->>App: Result
```

### 7.4 `flow.Runner.RunStream` 的根 + 节点 span 关系

```mermaid
sequenceDiagram
    autonumber
    participant App
    participant FW as otelflow.wrapper
    participant Root as flow.run.stream
    participant Inner as flow.Runner.RunStream
    participant Node as flow.node <id>

    App->>FW: RunStream(ctx, inputs)
    FW->>Root: Start with AttrFlowID/Name/InputCount
    FW->>Inner: RunStream(ctx, inputs)
    Inner-->>FW: inner-chan
    FW->>FW: spawn goroutine
    par goroutine
        loop ev in inner-chan
            alt NodeStarted
                FW->>Node: Start with AttrNodeID
            else NodeFinished
                FW->>Node: SetStatus(Ok/Error); End()
            else NodeSkipped
                FW->>Node: Start+End (AttrNodeSkipped=true)
            else FlowErr
                FW->>Root: record terminalErr
            end
            FW-->>App: forward ev (outer-chan)
        end
        FW->>Root: SetStatus(Ok/Error); End()
    end
    App-->>FW: range outer-chan
```

---

## 8. 设计优化与替代方案

### 8.1 架构维度

**当前权衡**

- 8 个具体 wrapper 类型（`otelmodel`）：**显式 capability 类型，零运行时反射**，但带来代码重复 + Go 1.18 之前难以用泛型重构。
- `otelagent` 强制走 `RunStream`：哪怕调用方用 `Run`，wrapper 也内部 stream 化。这给所有 inner agent 增加了 stream chan 开销。

**优化建议**

1. **能力位掩码 + 反射式 Wrap（替代 8 类型）**  
   引入 `type Caps uint8` 位掩码与一个 `wrapper` 结构体携带 `caps`，按需在 `Wrap` 中通过 `reflect.Type` + `reflect.StructTag` 或 hand-rolled 接口表生成代理。对 hot path（`Generate`）改动为零；只在 `WithTools` / `WithSchema` 等冷调用时走分支。可以将 8 个具体结构压回 1~2 个，但牺牲了「`var _ Interface = (*X)(nil)`」编译期断言的清晰度——**保守建议是维持现状**。
2. **可选「不强制 stream」的 agent wrapper 路径**  
   为 `otelagent.wrapper.Run` 增加快速路径：当 inner 是 `*SimpleAgent` 等非 ReAct 实现且无 step 流时，直接调用 inner.Run 并只产 1 个 `invoke_agent` span + 1 个 `chat` span。`Config.Mode = Auto|StreamMirror|Direct` 提供旋钮。
3. **暴露 Wrap 选项的 functional options 形式**  
   当前 `Config` 是 plain struct；扩展属性（采样、resource、attribute middleware）需要往 struct 加字段。改为 `Wrap(inner, WithTracerProvider(tp), WithSampler(...), WithAttributes(...))` 可保持二进制兼容。但这是「品味取舍」——可作为 v1 大版本切换时考虑。
4. **统一 `Observer` 形态做为 K3 的二级备选**  
   `otelrag.Observer` 已经示范了「不包 wrapper，注入回调拿到 trace 元数据」的二次接入点。可以推广到 model / agent / flow，让用户在「外层有 span 但不想多嵌一层」时也能拿 telemetry。

### 8.2 性能维度

**当前现实**

- 每次 `Generate` 创建 1 个 span + 4~6 个属性，端到端 < 5µs 是典型量级（在 `BatchSpanProcessor` 下）。
- `BatchSpanProcessor` 是 `NewTracerProvider` 唯一注册的 processor（`exporters.go:35`），其默认 batch size = 512、timeout = 5s——对大量短生命周期 model call 友好。
- **采样器**：未显式配置 → 使用 OTel SDK 默认 `ParentBased(AlwaysOn)`。生产中可能爆量。

**优化建议**

1. **暴露采样配置**  
   `ExporterConfig` 增加 `Sampler trace.Sampler` 或常见预设 `SamplingRatio float64`，把 `NewTracerProvider` 注册成 `WithSampler(...)`。建议默认 `ParentBased(TraceIDRatioBased(0.05))` 用于生产，0% 用于开发（让 OTLP collector 来做尾部采样）。
2. **stream first_token 改 histogram**  
   现版本 first-token 用 `span.AddEvent`，在 trace 视图直观，但在 Prometheus/Mimir 里**不可聚合**。应额外发 `gen_ai.client.operation.time_to_first_chunk` histogram（`otelmetrics` 已经预留了 instrument，但 `otelmodel.streamReader` 尚未调用它）。建议把两条信号一起发：event 给 trace、histogram 给 metric。
3. **`agents.runBuilder.trace` 切片预分配**  
   `make(..., 0, 8)` 用了 cap=8，对长 ReAct 链可能 grow 几次。可暴露 `Config.ExpectedSteps int` 让用户调高。属于「微优化」。
4. **GenAI semconv 属性写入合并**  
   `otelmodel.wrapper.Generate` 在 span 开启后先写两条属性、调用后再写四条，触发两次 `setAttributes`。可以收集到一个 `[]attribute.KeyValue` 再一次性 `SetAttributes`，减少锁竞争（OTel SDK 内部用 mutex）。

### 8.3 可观测性维度

**当前覆盖度**

| 信号 | model | agent | flow | rag |
|---|---|---|---|---|
| Trace（span） | ✓ | ✓ | ✓ | ✓ |
| Metrics | 仅 `otelmetrics.Recorder`（独立 API） | — | — | ✓（requests/errors/duration/tokens） |
| Logs | `otelslog` 关联 | — | — | — |
| Events | `gen_ai.first_token` | — | — | rag.Observer 模式 |

**优化建议**

1. **`otelmodel` 自动调用 `otelmetrics.Recorder`**  
   目前 `Recorder` 与 `wrapper` 没有耦合点——用户必须手动 `RecordTokenUsage` / `RecordDuration`。可以让 `otelmodel.Config` 接受 `MeterProvider` 或 `Recorder *otelmetrics.Recorder`，在 `Generate` 内完成「span + metric + log」三联发。Cost：API surface 扩大。Benefit：DX 接近 zero-config。
2. **GenAI semconv 上游标准 chasing**  
   semconv 仍处 experimental，`gen_ai.usage.source` / `gen_ai.response.finish_reason` 是本仓自添加（非上游严格定义）。建议建立**版本对齐 CI**：每次 OTel semconv 主版本升级，跑一遍属性差异对照。
3. **完整 slog → OTel LogRecord 桥**  
   当前 `otelslog.Handler` 只追加 trace_id/span_id，没有走 `go.opentelemetry.io/otel/log` 通道。如要满足「单一 backend 同时收 trace + log + metric」的合规需求，需替换为 `otelslog.NewHandler` 的「log SDK 桥版本」。建议新增 `otelslog/bridge.go` 与现有 `Handler` 并存。
4. **`agent.iterations` / `agent.tool.invocations` 在 `otelagent` 中自动发**  
   类似 8.3.1：将 `agent.Usage.LLMCalls` 自动翻译成 `agent.iterations` counter，否则这两个 instrument 永远没人用。
5. **RAG span event + Observer 二选一造成歧义**  
   `Wrapper` 自己开 span，但如果用户也注入了 `Observer`，会在**外层** span 上 AddEvent，造成「同一信息出现在两个 span 上」。文档应明确「Wrapper 与 Observer 不同时使用；若同时使用，Observer 的 event 会落在 wrapper 自创建的 span 上」——查 `otelrag/otelrag.go:194,205,217` 用 `trace.SpanFromContext(ctx)` 确实会落到 wrapper 当前 span。该行为可写明，避免误用。

### 8.4 DX 维度

**当前 DX**

- `Wrap(inner, Config{TracerProvider: tp})` —— 极简，但 `Config{}` 零值会 fall back 到 noop tracer provider（`otelmodel/config.go:9-14` / `otelagent/config.go:9-14` / `otelflow/config.go:18-23` / `otelrag/metrics.go:32-37`），开发者不易察觉「忘传 provider 等于关闭 telemetry」。
- `DefaultExporterConfig` 默认 4318 + insecure，与 grafana/otel-lgtm 默认对齐——上手友好。
- compose demo 一键起栈。

**优化建议**

1. **provider 缺失时发 stderr 警告**  
   `Config.tracerProvider()` 在 `c.TracerProvider == nil` 时除返回 noop 外，可在第一次 Wrap 时打一行 `fmt.Fprintln(os.Stderr, "[otel] WARN: noop tracer provider; spans will be discarded")`。可由 `OTEL_LOG_LEVEL=quiet` 关闭。
2. **统一 `trimEndpoint` helper**  
   `exporters_http.go:23-31` 与 `exporters_grpc.go:23-31` 重复实现 endpoint 规范化。应在 `exporters.go` 抽出 `trimEndpoint(string) string` 共用。
3. **OTLP env 自动接管**  
   OTel SDK 本来支持 `OTEL_EXPORTER_OTLP_ENDPOINT` 等 env。当前 `DefaultExporterConfig` 写死 endpoint，**忽略**了 env。建议改为「env > config > default」三级优先，与上游一致。
4. **`Config.tracerProvider()` 暴露成包级函数**  
   `func ResolveProvider(p trace.TracerProvider) trace.TracerProvider` 让用户也能用一行 fallback。
5. **`otelflow.Wrap` 对非 `Engine` 的失败兜底**  
   当 `Wrap` 拿到非 Engine 又未提供 `Config.FlowID` 时，根 span 名退化为 `"flow.run"`（无 id 后缀）。doc 写明了这件事，但 wrapper 可以在 Run 入口 `span.SetAttributes(attribute.String("flow.id", "unknown"))` 显式标注，避免 trace 上看不出来。
6. **`compose/demo` 增加 RAG / flow 例子**  
   当前 demo 只走 model + agent。可加 `compose/demo-flow/`、`compose/demo-rag/` 分别演示 otelflow/otelrag，让 Grafana 上一眼看到三种 span 形状。

---

## 9. 遗留与未来方向

### 9.1 GenAI semconv 演进

OTel `gen_ai.*` 仍在 experimental。近期可能变动：

- `gen_ai.usage.source`（本仓自加）可能不会进上游标准，需要别名兼容；
- `gen_ai.input.messages` / `gen_ai.output.messages` 上游正在讨论 LogRecord 化（而非 span attribute）。届时 `otelmetrics.MessageAttributes` 需迁移到 `otelslog` 的日志桥；
- `agent.iterations` / `agent.tool.invocations` 可能纳入 `gen_ai.agent.*` 前缀。

建议：**冻结一个内部 v1 命名空间** (`otelroot.AttrX` 别名机制)，让上游变动只需改一个文件。

### 9.2 Profiling 与 CDP 接入

仓库当前**不**集成 OTel Profiling（pprof → OTLP profiles）。如果未来 LLM agent 服务遇到「token 不爆但延迟无解」类问题，需要：

- 在 `cmd` 或 demo 入口集成 `runtime/pprof` + OTLP profiles exporter；
- 把 trace 与 profile 通过 `service.name` + `service.instance.id` 关联；
- Grafana Pyroscope 接 LGTM 栈是现成路径。

### 9.3 多导出器并行

目前 `NewTracerProvider` 只挂一个 `Batcher`。在某些金融/合规场景，希望同时发到 SaaS + 自建 OTLP。建议：

- `ExporterConfig` 变为 `[]ExporterConfig`；
- `NewTracerProvider` 内部对每个 cfg 各注册一个 `Batcher`；
- 或采用 `tracehttp` 转发 + downstream OTLP collector 做 fan-out。

### 9.4 与 `llm-agent-providers` 的解耦验证

K3 的硬约束之一是「核心 + provider 不依赖 otel」。建议在 CI 引入一条 grep 守卫：

```bash
go list -deps -f '{{ .ImportPath }}' ./... | grep -E 'opentelemetry' && exit 1 || true
```

只在核心 `llm-agent` 与 `llm-agent-providers` 上跑——只要这两仓的依赖图里出现了 OTel，CI 即 fail。目前已存在 stdlib-only assertion gate（umbrella 最近 commit `bc970bc` 引入了「B4 stdlib-only assertion gate」），但范围需要确认覆盖到「无 OTel」这一条额外约束。

### 9.5 wrapper 完整性盘点

| 抽象 | 是否已 wrap | 文件 | 覆盖度评分 |
|---|---|---|---|
| `llm.ChatModel` | ✓ | `otelmodel/otelmodel.go` | A（含 8-能力矩阵、stream first-token、错误状态） |
| `llm.ToolCaller` | ✓ | 同上（`toolWrapper` 等） | A |
| `llm.Embedder` | ✓ | 同上（`embedWrapper` 等） | B+（无 dimension/embedding-count 指标） |
| `llm.StructuredOutputs` | ✓ | 同上（`schemaWrapper` 等） | B（只透传，未记 schema 属性） |
| `agents.Agent` | ✓ | `otelagent/otelagent.go` | A-（强制 stream 路径，性能轻损） |
| `flow.Runner` | ✓ | `otelflow/otelflow.go` | A（含 NodeSkipped/取消兜底） |
| `*rag.System` | ✓ | `otelrag/otelrag.go` | A（Wrapper + Observer 双形态） |
| `slog.Handler` | ✓ | `otelslog/otelslog.go` | C+（仅 trace_id 关联，未对接 OTel log SDK） |
| `metrics.Recorder` | ✓（自助 API） | `otelmetrics/otelmetrics.go` | B+（基数防爆 + 内容捕获脱敏） |

> 评分主观，目的是让维护者一眼看到哪些点最需要补强。

### 9.6 测试覆盖盘点

| 包 | test 行数 | 关键断言 |
|---|---|---|
| 根 (`exporters_test.go` + `semconv_gen_ai_test.go`) | 76 + 48 | exporter 默认值、env opt-in 闸门、redact 行为、README 关键词 |
| `otelmodel` | 180 | capability 保留、单 span + first_token event、错误状态、WithTools 重新包装 |
| `otelagent` | 224 | invoke_agent + chat 父子关系、ReAct 4-span tree、错误流 |
| `otelflow` | 292 | flow.run/flow.run.stream 双名、节点 span、Skipped 零时长 span、FlowErr 标 |
| `otelrag` | 272 | Import/Retrieve/Ask span、Observer event、RED/Tokens metrics、no-op 安全 |
| `otelmetrics` | 192 | 三个 instrument 触发、≤50 attribute 组合（基数）、ContentCapture on/off |
| `otelslog` | 90 | trace_id/span_id 注入、gen_ai field 透传、WithAttrs/WithGroup 合成 |

**总测试行 ≈ 1374 行**，与「源码 1562 行」的比值是 **0.88**——非常健康的测试密度。

---

## 附录 A：文件清单（按行数排序）

| 文件 | 行数 | 角色 |
|---|---|---|
| `otelmodel/otelmodel.go` | 328 | ChatModel 8 能力 wrapper 全集 |
| `otelflow/otelflow_test.go` | 292 | flow wrapper 行为测试 |
| `otelrag/otelrag_test.go` | 272 | rag wrapper + metrics + observer 测试 |
| `otelrag/otelrag.go` | 232 | RAG Wrapper 与 Observer 实现 |
| `otelagent/otelagent_test.go` | 224 | agent wrapper 行为测试 |
| `otelflow/otelflow.go` | 197 | flow wrapper 主体 |
| `otelmetrics/otelmetrics_test.go` | 192 | 基数 + 指标测试 |
| `otelagent/otelagent.go` | 183 | agent wrapper 主体 |
| `otelmodel/otelmodel_test.go` | 180 | ChatModel wrapper 行为测试 |
| `otelmetrics/otelmetrics.go` | 130 | 客户端指标 Recorder |
| `otelrag/metrics.go` | 113 | RAG RED + tokens instrument |
| `otelslog/otelslog_test.go` | 90 | slog 桥测试 |
| `exporters_test.go` | 76 | exporter + README 守护 |
| `exporters.go` | 62 | TracerProvider/Exporter 路由 |
| `semconv_gen_ai.go` | 58 | 属性键 + 双 opt-in + redact |
| `semconv_gen_ai_test.go` | 48 | opt-in 测试 |
| `otelslog/otelslog.go` | 46 | slog handler 主体 |
| `compose/demo/main.go` | 46 | 一体化 demo |
| `otelflow/doc.go` | 38 | otelflow 包文档 |
| `otelflow/config.go` | 36 | flow attribute keys + Config |
| `exporters_http.go` | 31 | OTLP HTTP 路径 |
| `exporters_grpc.go` | 31 | OTLP gRPC 路径 |
| `otelmodel/config.go` | 14 | otelmodel.Config |
| `otelagent/config.go` | 14 | otelagent.Config |
| `otelmodel/semconv_gen_ai.go` | 3 | instrumentation 名常量 |

**合计：约 2936 行**（与 `wc -l` 输出一致）。

---

## 附录 B：常用 grep 路径

| 查找意图 | 命令 |
|---|---|
| 所有 wrap 入口 | `grep -rn "^func Wrap" llm-agent-otel/` |
| 所有 OTel attribute key | `grep -n "Attr[A-Z]" llm-agent-otel/semconv_gen_ai.go llm-agent-otel/otelflow/config.go llm-agent-otel/otelrag/otelrag.go` |
| 所有 metric name | `grep -n "Metric[A-Z]" llm-agent-otel/semconv_gen_ai.go llm-agent-otel/otelrag/metrics.go` |
| span 命名 | `grep -rn "tracer.Start" llm-agent-otel/` |
| env opt-in 闸门 | `grep -n "SemconvEnabled\|ContentCaptureEnabled" llm-agent-otel/` |
| capability 接口断言锁 | `otelmodel/otelmodel.go:300-321` |

---

**文档结束。**  
若需更新，请同时更新本文件与 `docs/source-design-umbrella-root.zh-CN.md`、`docs/subsystems-design-notes.zh-CN.md` 中对 K3 的引用。
