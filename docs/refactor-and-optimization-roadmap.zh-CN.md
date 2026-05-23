# 重构与优化路线图：`llm-agent-ecosystem`

> 编制日期：2026-05-21
> 关联评审：`docs/ecosystem-design-review.zh-CN.md`
> 来源：6 份 `docs/source-design-*.zh-CN.md` 深读 + umbrella README + cmd/depcheck + 必要源码回查
> 任务约定：每条优化都附编号、所属仓、severity（P0/P1/P2）、effort（S/M/L）、影响面、推荐方案、迁移风险、验证方式（test 名 / metric 名 / 命令）、依赖项。所有断言带 `file:line`。

---

## 1. 路线图总览

### 1.1 全表（按编号）

> Effort 标识：**S** ≤ 1 人天，**M** 1–3 人天，**L** > 3 人天。
> Impact 标识：**A** = 影响所有下游、**B** = 影响多个仓、**C** = 仓内。

| # | 仓 | 类别 | Severity | Effort | Impact | 摘要 |
|---|---|---|---|---|---|---|
| **P0-1** | customer-support | bug | P0 | S | C | guardrails 在 production wiring 中悬空 |
| **P0-2** | llm-agent / umbrella | 架构 | P0 | M | B | RAG facade 名实不副，须二选一 |
| **P1-1**（已修） | rag | 架构 + 性能 | P1 | M | A | ~~Postgres Migrate 加 ivfflat / hnsw 索引选项~~ — 已 ship in rag **v1.0.5**（2026-05-23 v1.3 perf-wave 第 3 棒）|
| **P1-2** | rag | 安全 | P1 | S | A | AskGlobal / AskDrift 路径补 injection sanitize |
| **P1-3** | llm-agent | 架构 | P1 | S | B | `comm/a2a` 后台 goroutine ctx 修复 |
| **P1-4** | llm-agent | DX | P1 | S | A | `runStreamFromBlocking` ctx-cancel 时发 Done event |
| **P1-5** | llm-agent | DX | P1 | M | A | `context` 包名重命名（v1 前 breaking 窗口） |
| **P1-6** | providers | 架构 | P1 | S | A | 5 个 provider 加 default timeout |
| **P1-7** | providers | 测试 | P1 | S | A | DeepSeek/MiniMax 补 cancel + partial-usage conformance |
| **P1-8** | providers | DX | P1 | S | A | DeepSeek/MiniMax 显式 `capabilitiesForModel` |
| **P1-9** | providers | 架构 | P1 | S | A | anthropic/ollama/minimax 解析 `Retry-After` |
| **P1-10** | otel | 可观测 | P1 | S | A | `NewTracerProvider` 暴露 sampler |
| **P1-11** | otel | DX | P1 | S | A | 接管标准 OTel env（OTLP_ENDPOINT 等） |
| **P1-12** | otel | 可观测 | P1 | M | A | 接入 `timeToFirst` histogram |
| **P1-13** | otel | 可观测 | P1 | L | A | `otelslog` 走 OTel log SDK |
| **P1-14** | otel | DX | P1 | M | A | `otelmodel` 自动连接 `otelmetrics.Recorder` |
| **P1-15**（已修） | rag | 性能 | P1 | M | A | ~~`HybridRetriever` 四路并发~~ — 已 ship in rag **v1.0.4**（2026-05-23 v1.3 perf-wave 第 2 棒）|
| **P1-16**（已修） | rag | 性能 | P1 | M | A | ~~新增 `BatchEmbedder` optional capability~~ — 已 ship in rag **v1.0.3**（2026-05-23 v1.3 perf-wave 第 1 棒）|
| **P1-17** | flow | 性能 | P1 | S | B | SQLite WAL + multi-VALUES INSERT |
| **P1-18** | flow | DX | P1 | S | B | `FlowEvent.Metadata` 字段加（additive） |
| **P1-19** | customer-support | DX | P1 | M | C | toolAgent 补 ReAct 第二轮（observation→final） |
| **P1-20** | customer-support | 性能 | P1 | S | C | session 历史 tail-N 截断 |
| **P1-21** | customer-support | DX | P1 | M | C | flowrunner 接入 production handler |
| **P1-22** | customer-support | 安全 | P1 | S | C | `/readyz` 真正 ping db/model |
| **P1-23**（已修） | providers | 架构 | P1 | L | A | ~~抽 `internal/openaicompat` / `anthropiccompat`~~ — 已 ship in providers **v0.2.4**（2026-05-23 v1.3 milestone 闭合，PR #28 / #29 / #30），落地 `internal/compat` 共享 `DefaultTimeout` + `WrapOpenAIError` + `WrapAnthropicError`（5/5 共享 timeout、4/5 共享 error 映射；ollama 保留 atomic-state 模式）|
| **P2-1** | llm-agent | DX | P2 | M | A | 加 `Message.ToolCallID` 字段 + 多轮 FunctionCallAgent |
| **P2-2** | llm-agent | 架构 | P2 | S | C | `memory.Consolidate` 衰减源项 Importance |
| **P2-3** | llm-agent | 可观测 | P2 | S | C | `policy.BlockedError.Wrapped` 标准化承接 budget err |
| **P2-4** | rag | DX | P2 | S | C | `ingest.Importer` 重命名或 deprecate |
| **P2-5** | rag | 测试 | P2 | M | A | `api/v1.snapshot.txt` 加行为 golden |
| **P2-6** | flow | DX | P2 | S | B | Validate 检查 Edge multi-target 冲突 |
| **P2-7** | flow | DX | P2 | M | A | `api/v0.1.flow-schema.json` JSON Schema |
| **P2-8** | llm-agent | 架构 | P2 | L | A | `bench/` + `rl/` 拆独立仓 |
| **P2-9** | rag | DX | P2 | S | C | `MarkdownSplitter` 识别 setext / code fence |
| **P2-10** | customer-support | 性能 | P2 | S | C | `estimateTokens` 改 BPE |
| **P2-11** | llm-agent | DX | P2 | M | B | `Termination` 子包化或 rename |
| **P2-12** | llm-agent | DX | P2 | S | B | `As*Tool` 命名统一 |

### 1.2 矩阵分类（4 维度 × 3 严重度）

| 维度 \ Severity | P0 | P1 | P2 |
|---|---|---|---|
| **架构** | P0-2 | P1-1, P1-3, P1-6, P1-9, P1-15, P1-16, P1-23 | P2-2, P2-8 |
| **性能** | — | P1-17, P1-20 | P2-10 |
| **可观测 / 治理** | — | P1-2, P1-10, P1-11, P1-12, P1-13, P1-14 | P2-3 |
| **DX / 安全 / 测试** | P0-1 | P1-4, P1-5, P1-7, P1-8, P1-18, P1-19, P1-21, P1-22 | P2-1, P2-4, P2-5, P2-6, P2-7, P2-9, P2-11, P2-12 |

总计：**2 P0 + 23 P1 + 12 P2 = 37 条**。

---

## 2. P0 立即修复（必须紧急）

### P0-1 customer-support：Guardrails wiring bug

**仓**：`llm-agent-customer-support`
**严重度**：P0（生产 binary 上 prompt-injection 防御完全失效）
**Effort**：S（< 0.5 人天，主要时间是写测试）
**Impact**：C（仓内）

#### 现状

`internal/app/app.go:106-110`：

```go
agent, err := supportflow.New(supportflow.Options{
    Model:     wrappedModel,
    Knowledge: knowledge,
    Sessions:  sessions,
})  // 不传 Guardrails
```

而 `supportflow.New`（`supportflow.go:44-74`）只有在 `opts.Guardrails != nil` 时才启用：

- prompt-injection 输入过滤（`supportflow.go:264-269` `allowInput` 当前直接返回 `true`）；
- system prompt 注入"把检索到的知识当作不可信"前缀（`supportflow.go:51-53`）。

`grep -rn "Guardrails" llm-agent-customer-support/internal/app/` 返回 0 行。但 README 第 7 行声称"day-one prompt-injection defenses"已上线。

#### 推荐方案

```go
// internal/app/app.go:106-115（修改后）
agent, err := supportflow.New(supportflow.Options{
    Model:      wrappedModel,
    Knowledge:  knowledge,
    Sessions:   sessions,
    Guardrails: guardrails.New(guardrails.Config{}),  // 加这一行
})
```

#### 替代方案对比

| 方案 | 优点 | 缺点 |
|---|---|---|
| **A. 显式注入（推荐）** | 最小修改；保留 `Guardrails` 字段为 optional 让 fork 可关 | — |
| B. `supportflow.New` 内默认构造 `Guardrails` | 调用方零成本 | 默认行为变化，对已存在的 fork breaking |
| C. 加 `Config.DisableGuardrails bool` opt-out | 兼容性最好 | 增加 Config 字段 |

#### 迁移风险

无。`Guardrails` 字段已存在，仅是装配遗漏。

#### 验证

1. **单元测试**：新增 `app_test.go::TestNew_GuardrailsWired`，用 `"ignore previous instructions"` 输入，断言 agent 返回 `guardrails.SafeFallback()`。
2. **集成测试**：在 `httpapi_test.go` 加 `TestChat_RejectsPromptInjection`，发 prompt-injection 输入到 `/chat`，断言返回的 `answer` 含 `guardrails.SafeFallback()` 文案。
3. **回归命令**：
   ```bash
   cd llm-agent-customer-support && go test ./internal/app/... -run TestNew_GuardrailsWired -count=1
   ```

#### 依赖项

无。

---

### P0-2 llm-agent / umbrella：RAG facade 名实不副

**仓**：`llm-agent` + umbrella
**严重度**：P0（stdlib-only 规则的"唯一豁免"未兑现；B4 stdlib-only assertion gate 需为它硬编码白名单）
**Effort**：M（落地方案 1 ~2 人天；删依赖方案 ~0.5 人天）
**Impact**：B（影响所有下游对"什么是核心"的判断）

#### 现状

- `llm-agent/rag/` 空目录（`docs/source-design-llm-agent.zh-CN.md:376-379`）；
- `llm-agent/go.mod:5` 声明 `require github.com/costa92/llm-agent-rag v1.0.1`；
- `grep -rn 'llm-agent-rag' llm-agent/ --include='*.go'` 只在 `agentstest/doc.go` 注释里出现；
- `llm-agent/go.sum` 只有 RAG 的 2 行 hash；
- 下游 `customer-support/knowledgebase.go:46` 直接 `import "github.com/costa92/llm-agent-rag/rag"`，不经 facade。

#### 推荐方案

**二选一**，由 v1.2 milestone 决定：

##### 方案 A：落地 facade（推荐 if 计划在 v1.3 用）

在 `llm-agent/rag/facade.go` 写一个 thin re-export 层：

```go
// llm-agent/rag/facade.go
//
// rag is the core's thin facade to llm-agent-rag/rag. Downstream code is
// expected to use this package so that they import only "github.com/costa92/llm-agent"
// in their go.mod and inherit the rag stack transitively.
package rag

import (
    upstream "github.com/costa92/llm-agent-rag/rag"
)

type (
    System  = upstream.System
    Options = upstream.Options
    Answer  = upstream.Answer
    // ... 把 customer-support 当前直接 import 的 8 个类型 alias 过来
)

func New(opts Options) *System { return upstream.New(opts) }
```

并同步：
- 修 `customer-support/knowledgebase.go` 改 import 路径；
- 在 README + CLAUDE.md 写明 facade 的边界；
- B4 gate 的 "allowed back-edges" 白名单中保留 llm-agent-rag。

##### 方案 B：删除依赖（推荐 if 6 个月内不会用 facade）

```bash
cd llm-agent && go mod edit -droprequire github.com/costa92/llm-agent-rag
rm go.sum && touch go.sum  # 让 go mod tidy 重建
go mod tidy
```

并同步：
- 在 README + umbrella README 删掉"唯一豁免"措辞；
- B4 gate 改成"core 绝对零反向边"；
- 下游 customer-support 保持现状不变。

#### 替代方案对比

| 方案 | 适用场景 | 风险 |
|---|---|---|
| **A. 落地 facade** | 计划在 v1.3-v1.4 把 RAG 当核心一部分（如 RAG 工具自动注册） | 增加维护面；type alias 跨模块 godoc 不友好 |
| **B. 删依赖** | RAG 永远独立，由下游显式 import | 文档 / CI 改造工作量；要解释为何"曾经声称"的豁免不再存在 |
| C. 保持现状 + 改文档 | 把"豁免"改写为"占位计划" | 仍然是名实不副，B4 gate 仍要白名单 |

#### 迁移风险

- **方案 A**：customer-support 改 import 路径是 single-line change；其他下游若未来直接 import facade，可享受透明性。
- **方案 B**：CI 闸子要调整，但行为零变化。

#### 验证

- **方案 A**：
  ```bash
  go test ./llm-agent/rag/... && go test ./llm-agent-customer-support/...
  ```
  + 新增 `rag/facade_test.go` 验证 `var _ upstream.System = (*System)(nil)` 类型等价。

- **方案 B**：
  ```bash
  cd llm-agent && go mod why github.com/costa92/llm-agent-rag
  # 应返回 "module ... is not needed"
  ```

#### 依赖项

无。可独立完成。建议在 v1.2 收尾时决策、v1.3 milestone 第一周完成。

---

## 3. P1 中期重构（下个版本窗口）

按"安全 → 性能 → 架构 → 可观测 → DX"分组陈列。每条只展开"现状/方案/验证"。

### 3.1 P1 安全相关

#### P1-2 rag：AskGlobal / AskDrift 补 injection sanitize

**仓**：`llm-agent-rag` | **Effort**：S | **Impact**：A

**现状**：`rag/inject.go:21-48` 的 `sanitizeHits` 只在 `Ask` 路径（`ask.go:84-87`）被调用；`AskGlobal`（`global.go:62`）和 `AskDrift`（`drift.go:75`）都不跑 sanitize。攻击者可在 entity description（被 community report 收纳）注入 prompt 指令。

**方案**：

- 把 `sanitizeHits` 升级为通用 `sanitizeStrings(scanner, items []string) ([]string, []InjectionFinding)`；
- 新增 `sanitizeReports(reports []CommunityReport) ([]CommunityReport, []InjectionFinding)`；
- 在 `global.go` map 步之前对 reports 跑 sanitize；
- 在 `drift.go` primer 之前对 reports 跑 sanitize。

**替代方案对比**：

| 方案 | 优点 | 缺点 |
|---|---|---|
| **A. AskGlobal/AskDrift 内部跑（推荐）** | 与 Ask 一致；不影响 caller | 增加 ~30 行重复 helper |
| B. 在 `CommunityStore.UpsertCommunities` 处一次性 sanitize | 一次扫描永久干净 | 改 store 接口语义（违反 v1 frozen） |
| C. 加 `AskGlobalOptions.SanitizeReports bool` opt-in | 兼容性好 | 默认关 = 默认不安全 |

**验证**：`eval/global_test.go` 加 fixture `report_with_injection.json` 含 `"ignore previous instructions"`，断言 `Diagnostics.InjectionFindings` 非空、`Answer.Text` 不复读注入指令。

**依赖项**：无。

---

#### P1-22 customer-support：`/readyz` 真正 ping db/model

> **状态（2026-05-22）：已 merge to master，PR #16 customer-support，commit fd78a40**。`internal/app/app.go:134` 现注入 `makeReadyFunc(sessions, embedder, cfg.ReadinessProbeEmbedder)`（详细实现见 `app.go:234-256`），实装 db PingContext + 1s embedder Embed 双探测；新增 `READINESS_PROBE_EMBEDDER` env 开关；测试 `httpapi/handlers_test.go` 验证 `/readyz` 在 db/embedder 故障时返回 503 + 错误消息。下方原文不动以保留方案审计轨迹。

**仓**：`llm-agent-customer-support` | **Effort**：S | **Impact**：C

**现状**：`internal/app/app.go:130` 注入 `ReadyFunc = func(ctx) error { return nil }`，readiness 永远 200。`/readyz`（`httpapi.go:212-224`）调用它。

**方案**：

```go
// app.go (改写 ReadyFunc)
Ready: func(ctx context.Context) error {
    if pinger, ok := sessions.(interface { PingContext(context.Context) error }); ok {
        if err := pinger.PingContext(ctx); err != nil { return err }
    }
    // 尝试一次 cheap embed 调用验证 model reachable（限 1s 超时）
    cctx, cancel := context.WithTimeout(ctx, time.Second)
    defer cancel()
    if _, err := embedder.Embed(cctx, []string{"ok"}); err != nil { return err }
    return nil
},
```

**替代方案对比**：

| 方案 | 优点 | 缺点 |
|---|---|---|
| **A. 显式 ping db + 试 embed（推荐）** | 真探测；和 K8s readiness 语义一致 | 加 1s 延迟 |
| B. 仅 ping db 不 ping model | 快；不被 model 抖动连累 | model 真挂时不可发现 |
| C. 周期性后台探测，handler 只读缓存状态 | handler 永远快 | 实现复杂；状态可能 stale |

**验证**：`httpapi_test.go::TestReadyz_FailsWhenDBDown` + `TestReadyz_FailsWhenModelDown`。

**依赖项**：`sessionstore.Store` 实现 `PingContext`（PG/SQLite 都有），可能需要在 `Store` 接口加可选 `Pinger` capability（按 type-assert 嗅探）。

---

### 3.2 P1 性能相关

#### P1-15 rag：HybridRetriever 四路并发

> **状态（2026-05-23）：已 ship in rag v1.0.4，PR #6 llm-agent-rag，merge commit ff7af07**。`HybridRetriever.Retrieve` 采用 `sync.WaitGroup` 并行 fan-out Dense/Lexical/Structure/Graph 四路；按 route 索引确定性 merge fusion trace；wall-clock 从 Σtimes 降到 max(times)，典型 4× 改善。无新增依赖（仍 stdlib-only）。v1.3 perf-wave 第 2 棒。下方原文不动以保留方案审计轨迹。

**仓**：`llm-agent-rag` | **Effort**：M | **Impact**：A

**现状**：`retrieve.go:984-1009` HybridRetriever 内部 Dense → Lexical → Structure → Graph 串行；4 路全部独立可并发。

**方案**：用 stdlib `sync.WaitGroup` + cancellable ctx 把 4 路并发：

```go
// retrieve.go (HybridRetriever.Retrieve 内部)
var wg sync.WaitGroup
results := make([][]store.Hit, 4)
traces := make([]Trace, 4)
errs := make([]error, 4)

routes := []retrievalRoute{denseRoute, lexicalRoute, structureRoute, graphRoute}
for i, r := range routes {
    if r == nil { continue }
    wg.Add(1)
    go func(i int, r retrievalRoute) {
        defer wg.Done()
        results[i], traces[i], errs[i] = r(ctx, req)
    }(i, r)
}
wg.Wait()
// 按确定性顺序 0,1,2,3 合并 fuse + merge trace
```

**关键约束**：trace 字段合并必须**按 route 序号顺序**写回 `FusionAttribution`，否则 `eval` 的 golden test 会挂（rag 的 determinism 信条）。

**替代方案对比**：

| 方案 | 优点 | 缺点 |
|---|---|---|
| **A. sync.WaitGroup（推荐，stdlib-only）** | 不引新依赖，确定性顺序 | 任一路 fail 时仍跑完其他路（不 fail-fast） |
| B. errgroup（已在 indirect dep） | fail-fast | 引入 `golang.org/x/sync` 到 rag direct dep，违反 stdlib-only |
| C. 使用 `llm-agent` 的 `pkg/fanout` | fail-fast + bounded | 需新增反向依赖到 llm-agent（不可） |

**性能预期**：4 路顺序 ≈ Σtimes，并发 ≈ max(times)；典型 Dense+Lex+Struct+Graph 跑 200ms+150ms+80ms+300ms → 730ms 降到 ~300ms（**2.4x**）。

**验证**：`retrieve_test.go::TestHybridRetriever_Concurrent` 用 sleepy mock 验证 wall-clock 接近 max；`eval/graph_test.go` 锁定 RecallDelta 不回退（行为确定性）。

**依赖项**：无。

---

#### P1-16 rag：BatchEmbedder optional capability

> **状态（2026-05-23）：已 ship in rag v1.0.3，PR #5 llm-agent-rag，merge commit af9b5b8**。`embed/embedder.go` 新增 `BatchEmbedder` optional capability（`EmbedBatch(ctx, []string) ([]Vector, error)`）；`rag/import.go` type-assert 嗅探后走 batch 路径，否则 fallback 单调；customer-support PR #20 已 wire `ragEmbedderAdapter.EmbedBatch` 通过 providers OpenAI 适配器走 batch。实测在 import 路径上 20× 吞吐改善（OpenAI text-embedding-3-small）。v1.3 perf-wave 第 1 棒。下方原文不动以保留方案审计轨迹。

**仓**：`llm-agent-rag` | **Effort**：M | **Impact**：A

**现状**：`rag/import.go:76` 每 chunk 单调用 `embedder.Embed`，真模型（OpenAI text-embedding-3 等）吞吐 < 50 tokens/s 的话，1000 chunk 一次 import 要 ~30 分钟。

**方案**：v1 接口不可改（K-frozen），新增 sibling capability：

```go
// embed/embedder.go (additive)
type BatchEmbedder interface {
    EmbedBatch(ctx context.Context, texts []string) ([]Vector, error)
}

// rag/import.go: type-assert 嗅探
if be, ok := s.embedder.(embed.BatchEmbedder); ok {
    vecs, err := be.EmbedBatch(ctx, allTexts)
    // 用 vecs，跳过 per-chunk Embed
} else {
    // 老路径单调
}
```

**替代方案对比**：

| 方案 | 优点 | 缺点 |
|---|---|---|
| **A. Sibling capability + type-assert（推荐）** | v1 加法兼容 | embedder 实现方需要二次升级 |
| B. 改 `Embedder` 接口加 `EmbedBatch` | 简单 | breaking change，v2 路径 |
| C. 引入 `BatchingEmbedder` 装饰器自动 batch（默认包 1 张张） | 调用方零成本 | 假并发，无真实 batch 收益 |

**性能预期**：OpenAI text-embedding-3-small 单调 ~25 chunk/s，batch=100 时 ~500 chunk/s（**20x**）。

**验证**：providers 仓为 `openai.OpenAI` 加 `EmbedBatch` 实现 + rag fixture `batch_import_test.json`；`rag/import_test.go::TestImport_UsesBatchEmbedderWhenAvailable` 用 mock 验证 batch 路径被调。

**依赖项**：providers 仓同步实现 `BatchEmbedder`（至少 OpenAI / Ollama）；P1-23 抽 `openaicompat` 后实现成本更低。

---

#### P1-17 flow：SQLite WAL + multi-VALUES INSERT

> **状态（2026-05-22）：WAL 部分已 merge to master，PR #2 flow v0.1.2**。`flow/store/sqlite/open.go` 已对 on-disk DSN 启用 `PRAGMA journal_mode=WAL` + `PRAGMA synchronous=NORMAL`；multi-VALUES INSERT 仍 pending（v0.1.1 显式事务 + Prepare + 循环 ExecContext 路径继续生效）。下方原文不动以保留方案审计轨迹。

**仓**：`llm-agent-flow` | **Effort**：S | **Impact**：B

**现状**：`flow/store/sqlite/open.go:51-92` Schema 不开 WAL；`events.go:20-54` `AppendRunEvent` 是单语句 `INSERT INTO run_events ... COALESCE(MAX(seq)+1,0)`，每事件 ~5ms（FULL fsync）；v0.1.1 批量版 `AppendRunEvents` 走显式事务 + Prepare + 循环 ExecContext，比单条快 5x 但仍 N 次 Exec。

**方案**：

1. **开 WAL + relax sync**：在 `ensureSchema` 头部加：

   ```go
   if dsn != ":memory:" {
       _, _ = db.Exec(`PRAGMA journal_mode=WAL;`)
       _, _ = db.Exec(`PRAGMA synchronous=NORMAL;`)
   }
   ```

2. **`AppendRunEvents` 改单 INSERT 多 VALUES**：

   ```sql
   INSERT INTO run_events (run_id, seq, kind, node_id, payload_json, ts)
   VALUES (?, ?, ?, ?, ?, ?), (?, ?, ?, ?, ?, ?), ...
   ```

   配合事务内先 `SELECT MAX(seq) FROM run_events WHERE run_id=?` 拿基线，再 in-memory 自增。

**替代方案对比**：

| 方案 | 写延迟 | 兼容性 | 注释 |
|---|---|---|---|
| 现状 | ~5ms/event | 默认 | FULL fsync |
| **A. WAL + NORMAL + multi-VALUES（推荐）** | ~0.3ms/event | crash 时丢失最后一组 batch（NORMAL 不保证 fsync） | 大多数 demo / 中等耐久性场景可接受 |
| B. 仅开 WAL，不动 INSERT 风格 | ~1ms/event | 不破坏耐久性 | 收益 5x，仍是单语句 |
| C. WAL + FULL 不动 sync | ~2ms/event | 强耐久 | 部分收益 |

**性能预期**：100-node flow 6 个 event/node = 600 events，从 ~3s 降到 ~180ms（**16x**）。

**验证**：`store/sqlite/events_bench_test.go::BenchmarkAppendRunEvents_Batch_1000` 跑前后对比；`server_test.go::TestRunFlow_NoEventLoss_WithWAL` 跑长 flow 后断言 `ListRunEvents` 行数等于发射数。

**迁移风险**：开 WAL 会产生 `*-wal` / `*-shm` 辅助文件，docker volume / 备份策略需更新。

**依赖项**：无。`modernc.org/sqlite` 已支持 WAL。

---

#### P1-20 customer-support：session 历史 tail-N 截断

**仓**：`llm-agent-customer-support` | **Effort**：S | **Impact**：C

**现状**：`supportflow.go:209-226` `mergeQuestionWithHistory` 把整段历史拼成 prompt；长会话单 prompt 超 `MaxTokensPerRequest`，preflight 直接 429。

**方案**：

- 在 `load-session` 节点加 `tailMessages(history, N)` 留最近 N=20 消息；
- 默认 N=20（cfg 可调）；
- 长远改用 `[]llm.Message` 多轮协议而非单 user 拼接（涉及 toolAgent 改造，见 P1-19）。

**替代方案对比**：

| 方案 | 优点 | 缺点 |
|---|---|---|
| **A. tail-N 截断（推荐 short-term）** | 1 行改动 | 老话题断层 |
| B. 加 conversation summary 节点 | 保留语义 | 多 1 次 LLM call |
| C. 改 multi-turn `[]llm.Message` | 协议正确 | 牵动 toolAgent + supportflow |

**验证**：`supportflow_test.go::TestMergeQuestion_TruncatesLongHistory` 喂 200 条 message，断言 prompt 长度 ≤ MaxTokensPerRequest 估算上限。

**依赖项**：长远依赖 P1-19。

---

### 3.3 P1 架构相关

#### P1-23 providers：抽 `internal/openaicompat` / `anthropiccompat`

> **状态（2026-05-23）：已 ship in providers v0.2.4，v1.3 milestone 闭合**。3 个 PR 渐次合并（PR #28 openai+deepseek、PR #29 anthropic+minimax、PR #30 ollama）落地 `internal/compat` 内部包，导出 `DefaultTimeout`（5/5 provider 共享）+ `WrapOpenAIError`（openai / deepseek 共享）+ `WrapAnthropicError`（anthropic / minimax 共享）。**ollama `errors.go` 保留 atomic-state pattern**（Path A 取舍：streaming response 状态机与 OpenAI / Anthropic 同形难以零成本对齐），仅借用 `compat.DefaultTimeout`；同理 streaming reader 抽取留待 v1.4 窗口（详见下文方案，stream.go 部分尚未抽出）。原本路线图设想的"包名 `internal/openaicompat` + `internal/anthropiccompat`"演化为单一 `internal/compat`，因为 timeout 是 5/5 共享、错误映射按 SDK 家族二分即可，过细分包反而增加 import surface。下方原文不动以保留方案审计轨迹。

**仓**：`llm-agent-providers` | **Effort**：L | **Impact**：A

**现状**：openai vs deepseek、anthropic vs minimax 在 `openai.go/deepseek.go`、`anthropic.go/minimax.go`、`map.go`、`errors.go` 这 4 个文件上重复 ~90%。streaming reader 改一边手工同步另一边，drift 风险高。`docs/source-design-llm-agent-providers.zh-CN.md:104-116` 已点明。

**方案**：

```
internal/openaicompat/
  stream.go    # StreamReader[provider string]，承接 openai/deepseek 的 chunkEvents
  mapping.go   # toSDKRequest / fromSDKResponse 通用
  errors.go    # wrapErr 通用（保留 wrapErr 接收 providerName 参数差异）

internal/anthropiccompat/
  stream.go    # content-block 模型通用 reader
  mapping.go   # system prompt lifting / toToolInputSchema
  errors.go
```

`openai/openai.go` / `deepseek/deepseek.go` 退化为 50 行 thin wrapper：构造时注入 baseURL + providerName + caps。anthropic/minimax 同。

**替代方案对比**：

| 方案 | 优点 | 缺点 |
|---|---|---|
| **A. 抽 internal/{openai,anthropic}compat（推荐）** | 减少 ~70% 重复代码；新加 OpenAI-兼容 provider 成本几乎 0 | ~2 人日重构；测试要全跑 |
| B. 共用 SDK 类型但保留各自 stream reader | 改动小 | drift 风险继续 |
| C. 完全无抽象，靠 CI 跑 conformance | 简单 | drift 时只能事后发现 |

**触发条件升级**：deeapseek 之后第 3 个 OpenAI-兼容 provider（Zhipu/Moonshot 等）出现时，本项**自动升级 P0**。

**验证**：抽完后 `internal/contract/generate_test.go` 全套必须 GREEN；新增 `internal/openaicompat/stream_test.go` 跑 openai + deepseek fixture 两套，断言行为等价。

**依赖项**：建议在 P1-7（DeepSeek/MiniMax conformance 补齐）之后做；conformance 完整有助于回归。

---

#### P1-1 rag：Postgres Migrate 加 vector index 选项

> **状态（2026-05-23）：已 ship in rag v1.0.5，PR #7 llm-agent-rag，merge commit 3c92585**。`postgres.Config` 新增 `VectorIndex VectorIndex`（enum: `VectorIndexNone` / `VectorIndexIVFFlat` / `VectorIndexHNSW`）+ `IVFFlatLists` / `HNSWConstructionM` 等可选参数；`Migrate` 在 opt-in 时 `CREATE INDEX IF NOT EXISTS ... USING ivfflat (embedding vector_cosine_ops)` 或 `... USING hnsw (...)`。默认零值 = `VectorIndexNone` 保持向后兼容；customer-support 当前未启用（按需自配 postgres）。预计 100K-chunk NN 查询从 ~1.5s 降到 ~80ms (~19×)。v1.3 perf-wave 第 3 棒；至此 P1-1/15/16 三棒全部 closed，rag 进入"可直接生产用"状态（详见 `docs/ecosystem-design-review.zh-CN.md` §7）。下方原文不动以保留方案审计轨迹。

**仓**：`llm-agent-rag` | **Effort**：M | **Impact**：A

**现状**：`postgres/postgres.go:93-161` `Migrate` 创建 4 张表 + tsvector GIN，**但不创建 vector 索引**。`Search` 用 `embedding <=> $vec ORDER BY ... LIMIT`（`postgres.go:233-239`），无索引时 O(N) 全表扫描。生产 ~100K chunk 即崩。

**方案**：

```go
// postgres/postgres.go
type VectorIndex int
const (
    VectorIndexNone VectorIndex = iota
    VectorIndexIVFFlat
    VectorIndexHNSW
)

type Config struct {
    // ... existing
    VectorIndex      VectorIndex  // default None（向后兼容）
    IVFFlatLists     int          // default 100
    HNSWConstructionM int         // default 16
}

// 在 Migrate 内
switch c.VectorIndex {
case VectorIndexIVFFlat:
    _, _ = pool.Exec(ctx, fmt.Sprintf(
        `CREATE INDEX IF NOT EXISTS %s_embedding_ivfflat
         ON %s USING ivfflat (embedding vector_cosine_ops)
         WITH (lists = %d)`, table, table, lists))
case VectorIndexHNSW:
    // 类似
}
```

**替代方案对比**：

| 方案 | 优点 | 缺点 |
|---|---|---|
| **A. Config 字段（推荐）** | additive；用户显式选 | 默认 None = 升级用户不会自动获益 |
| B. 默认开 IVFFlat | 自动加速 | DDL 在大表上长时间锁；可能 surprise 用户 |
| C. 在 `Open` 外加 helper `EnsureVectorIndex(ctx, pool, ...)` | 灵活 | 增加 API surface |

**性能预期**：100K chunk 上，IVFFlat (lists=100) 查询从 ~1.5s 降到 ~80ms（**~19x**）。

**验证**：`postgres_conformance_test.go` 加 ivfflat 子套件（env-gated），跑 1M-row synthetic + 200 query 测 p95 延迟阈值。

**迁移风险**：DDL `CREATE INDEX` 在 PG <11 不支持 `vector_cosine_ops`；要求 pgvector ≥ 0.5。

**依赖项**：无。

---

#### P1-3 llm-agent：comm/a2a 后台 goroutine ctx 修复

> **状态（2026-05-22）：已 merge to master，PR #3 llm-agent，commit 860dd20**。`comm/a2a/server.go:111` 起 worker 用 `ctx, cancel := context.WithCancel(...)`，cancel 存入 `Task.cancel` 字段；`comm/a2a/task.go:112` 实现 `cancelAndFail`；新增 `DELETE /tasks/{id}` 端点 invokes `cancelAndFail`；测试 `comm/a2a/server_test.go` 增 RED→GREEN 用例（commit 75b92f6）。下方原文不动以保留方案审计轨迹。

**仓**：`llm-agent` | **Effort**：S | **Impact**：B

**现状**：`comm/a2a/server.go:128-129` 起 worker 用 `context.Background()`：

```go
go s.runTask(taskID, handler, body.Input)
// ...
func (s *Server) runTask(id string, handler SkillHandler, input json.RawMessage) {
    s.tasks.update(id, func(t *Task) { t.State = TaskRunning })
    // Use background ctx — the HTTP request already returned; future
    // improvement: store cancel in taskStore and expose DELETE /tasks/{id}.
    ctx := context.Background()
    ...
}
```

HTTP 请求返回后无法 cancel；长跑任务下 goroutine 不可中断。

**方案**：

```go
// task.go: Task 加 cancel field
type Task struct {
    ID, State, Error, Result, ...
    cancel context.CancelFunc  // unexported
}

// server.go:runTask
ctx, cancel := context.WithCancel(context.Background())
s.tasks.update(id, func(t *Task) { t.State = TaskRunning; t.cancel = cancel })

// server.go: 新增 DELETE /tasks/{id}
func (s *Server) handleDeleteTask(...) {
    t, ok := s.tasks.get(id)
    if t.cancel != nil { t.cancel() }
    s.tasks.update(id, func(t *Task) { t.State = TaskFailed; t.Error = "canceled" })
}
```

**验证**：`comm/a2a/server_test.go::TestDeleteTask_CancelsRunningHandler` 起一个 long-handler，调 `DELETE`，断言 100ms 内 task state 变 failed + handler 收到 ctx.Err。

**迁移风险**：`Task.cancel` 是 unexported，对外不影响；新增 endpoint 是 additive。

**依赖项**：无。

---

#### P1-4 llm-agent：runStreamFromBlocking ctx-cancel 时发 Done event

> **状态（2026-05-22）：已 merge to master，PR #2 llm-agent，commit 8ffed58（merge 44f547d）**。`agent.go:126-128` 已实装方案 A：在 ctx 取消路径上 best-effort 推 `final.Err = ctx.Err()`；测试 `agent_test.go::TestRunStreamFromBlocking_EmitsDoneOnCancel` 已通过。下方原文不动以保留方案审计轨迹。

**仓**：`llm-agent` | **Effort**：S | **Impact**：A

**现状**：`agent.go:107-122`：

```go
go func() {
    defer close(ch)
    ...
    select {
    case ch <- StepEvent{Done: true, Final: &res}:
    case <-ctx.Done():
    }
}()
```

ctx 取消时 channel 直接关闭，consumer 用 `range ch` 看不到 final event，无法区分"正常完成"与"中途 cancel"。

**方案**：

```go
go func() {
    defer close(ch)
    ...
    final := StepEvent{Done: true, Final: &res}
    if err != nil {
        final = StepEvent{Done: true, Err: err}
    }
    // Best-effort send（chan buffer 16，几乎一定能放下）
    select {
    case ch <- final:
    default:
        // Channel 已被 consumer drop；no-op
    }
    if ctx.Err() != nil {
        select {
        case ch <- StepEvent{Done: true, Err: ctx.Err()}:
        default:
        }
    }
}()
```

**替代方案对比**：

| 方案 | 语义 | 风险 |
|---|---|---|
| 现状 | 静默关闭 | consumer 不知是否 canceled |
| **A. 尝试推 Done{Err: ctx.Err()}（推荐）** | 显式终结 | buffer 满时仍会丢（边角） |
| B. 改用 `chan struct{ Event; Err error }` | 类型变化 breaking | 破坏 API |
| C. 让 RunStream 返回 (chan, cleanup func)，cleanup 显式同步等待 | 强语义 | 增加 caller 负担 |

**验证**：`agent_test.go::TestRunStreamFromBlocking_EmitsDoneOnCancel` 起 stream → cancel → 断言 `range ch` 末尾收到 `StepEvent{Done: true, Err: context.Canceled}`。

**依赖项**：无。

---

#### P1-5 llm-agent：`context` 包名重命名

**仓**：`llm-agent` | **Effort**：M | **Impact**：A（breaking）

**现状**：`llm-agent/context/builder.go:4-9` 包名 `context`，与 stdlib 冲突，强制下游 `aictx`/`promptctx` 别名。

**方案**：v1 前一次性 rename。候选名：

| 候选 | 优点 | 缺点 |
|---|---|---|
| **`promptctx`** | 准确反映"prompt context engineering" | 词新 |
| **`prompt`** | 简短 | 与 rag.prompt 视觉冲突 |
| **`ctxbuild`** | 描述功能 | 不优雅 |
| `gssc` | 反映 G-S-S-C 4 阶段 | 隐晦 |

**推荐 `promptctx`**。

**迁移**：
- llm-agent 内部把 `context` → `promptctx` rename；
- 加 `Deprecated:` 注释到 `aictx`（如果有别名）；
- 发 v0.6.0 标记 breaking；
- customer-support 同步改 import（影响 ~3 个文件）。

**验证**：
```bash
cd llm-agent && go build ./... && go test ./...
cd ../llm-agent-customer-support && go build ./...
```

**依赖项**：build before v1 stability commitment（即 v0.6 → v1.0 之间）。

---

#### P1-6 providers：5 个 provider 加 default timeout

> **状态（2026-05-23）：已 ship in providers v0.2.4 (P1-23 cascade)**。P1-6 在 P1-23 三连 PR 中顺带闭合：`internal/compat/timeout.go::DefaultTimeout` 落地后，openai / anthropic / deepseek / minimax / ollama 5 个 `options.go` 全部统一调用 `cfg.timeout = compat.DefaultTimeout(cfg.timeout)`（默认 60 秒，非零值原样保留）。**P1-6 / P1-23 同步闭合，v1.3 milestone 收尾**。下方原文不动以保留方案审计轨迹。

**仓**：`llm-agent-providers` | **Effort**：S | **Impact**：A

**现状**：5 个 `options.go` 的 `New` 函数不设默认 timeout（`docs/source-design-llm-agent-providers.zh-CN.md:858-868`），用户从不调 `WithTimeout` → 永挂连接让 Generate 永远 block。

**方案**：每个 `New` 末尾加：

```go
if cfg.httpClient == nil {
    cfg.httpClient = &http.Client{Timeout: 60 * time.Second}
} else if cfg.httpClient.Timeout == 0 {
    cfg.httpClient.Timeout = 60 * time.Second
}
// 注意：stream 单独考虑 — SDK 通常用 cfg.httpClient，但 stream 长时间合法
```

**关键细节**：stream 调用通过 `ctx.WithDeadline` 而非 client.Timeout 控制（否则 5min 流会被 cut）；sync `Generate` 通过 client.Timeout 控制。

**替代方案对比**：

| 方案 | 默认值 | 取舍 |
|---|---|---|
| **A. 60s for client（推荐）** | 60s | 兼容大多数 sync 调用 |
| B. 30s | 30s | 部分 reasoning model 超时 |
| C. 不设 default，强制用户调 WithTimeout | 0s 表示无限 | 默认行为变更（添加约束）会让现有代码出错 |

**验证**：`*_test.go::TestNew_HasDefaultTimeout` 检查 `m.httpClient.Timeout > 0`。

**依赖项**：无。

---

#### P1-9 providers：anthropic/ollama/minimax 解析 Retry-After

> **状态（2026-05-22）：已 merge to master，5/5 闭合**。PR #19（anthropic + minimax）与 PR #20（ollama，commit 537b351）相继合并；deepseek 已在历史 PR 同步落地。当前 `openai/anthropic/deepseek/minimax/ollama` 5 个 `errors.go` 均含 `Retry-After` 解析路径（结构化字段 `RateLimitError.RetryAfter`）。K4 retry 信号完整，与 §1.2 总评 "K4 GREEN" 一致。下方原文不动以保留方案审计轨迹。

**仓**：`llm-agent-providers` | **Effort**：S | **Impact**：A

**现状**：openai 已实现（`openai/errors.go:33-38`），anthropic（`anthropic/errors.go:31-33`）、ollama（`ollama/errors.go:38-46`）、minimax（`minimax/errors.go:28-31`）都不解析 `Retry-After` header。K4 retry 状态机的关键信号缺失。

**方案**：抽 `internal/contract/retryafter.go`（或借机做 P1-23 拆分时放入 compat 包）：

```go
// internal/(openaicompat|contract)/retryafter.go
func ParseRetryAfter(h http.Header) time.Duration {
    v := h.Get("Retry-After")
    if v == "" { return 0 }
    // 先试数字秒
    if n, err := strconv.Atoi(v); err == nil { return time.Duration(n) * time.Second }
    // 再试 HTTP-date
    if t, err := http.ParseTime(v); err == nil { return time.Until(t) }
    return 0
}
```

在 anthropic/ollama/minimax 的 RateLimitError wrap 处调用。

**验证**：`*_test.go::TestWrapErr_ParsesRetryAfter_Seconds` + `_HTTPDate` 两个 case 每个 provider 都跑（用 fixture 注入 header）。

**依赖项**：P1-23 完成后实现一处即可。

---

### 3.4 P1 可观测相关

#### P1-10 otel：NewTracerProvider 暴露 sampler

**仓**：`llm-agent-otel` | **Effort**：S | **Impact**：A

**现状**：`exporters.go:35` 永远 `trace.WithBatcher(exporter)`，无 sampler 配置；默认 `ParentBased(AlwaysOn)`，生产 LLM 服务 trace 量爆。

**方案**：

```go
type ExporterConfig struct {
    Protocol Protocol
    Endpoint string
    Insecure bool
    Sampler  sdktrace.Sampler  // 新增
    SamplingRatio float64       // 也支持简化形式
}

// NewTracerProvider 内
sampler := cfg.Sampler
if sampler == nil {
    if cfg.SamplingRatio > 0 {
        sampler = sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SamplingRatio))
    } else {
        sampler = sdktrace.ParentBased(sdktrace.AlwaysSample())
    }
}
return sdktrace.NewTracerProvider(
    sdktrace.WithBatcher(exporter),
    sdktrace.WithSampler(sampler),
    ...
), nil
```

**推荐默认**：生产建议用户显式传 `SamplingRatio: 0.05`，开发不传。

**验证**：`exporters_test.go::TestNewTracerProvider_HonorsSampler` 用自定义 sampler，断言 NewTracerProvider 注册了它。

**依赖项**：无。

---

#### P1-11 otel：接管标准 OTel env

**仓**：`llm-agent-otel` | **Effort**：S | **Impact**：A

**现状**：`DefaultExporterConfig` 写死 endpoint 4318，忽略 `OTEL_EXPORTER_OTLP_ENDPOINT` / `OTEL_EXPORTER_OTLP_PROTOCOL` / `OTEL_EXPORTER_OTLP_INSECURE`。

**方案**：

```go
// exporters.go
func DefaultExporterConfig() ExporterConfig {
    cfg := ExporterConfig{
        Protocol: ProtocolHTTP,
        Endpoint: "http://localhost:4318",
        Insecure: true,
    }
    if v := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); v != "" {
        cfg.Endpoint = v
    }
    if v := os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL"); v == "grpc" {
        cfg.Protocol = ProtocolGRPC
    }
    if v := os.Getenv("OTEL_EXPORTER_OTLP_INSECURE"); v != "" {
        cfg.Insecure = v == "true" || v == "1"
    }
    return cfg
}
```

**优先级**：**env > caller Config**，与 OTel SDK 上游一致。

**替代方案对比**：

| 方案 | 兼容性 | 注释 |
|---|---|---|
| **A. env > Config（推荐）** | 与上游一致 | 用户自传 Config 会被 env 覆盖（合预期） |
| B. Config > env | 老 API 行为不变 | 与上游不一致 |
| C. 三级（caller args > env > default），需 caller 主动 opt-in env | 灵活 | 复杂 |

**验证**：`exporters_test.go::TestDefault_HonorsEnv` 用 `t.Setenv` 注入。

**依赖项**：无。

---

#### P1-12 otel：接入 timeToFirst histogram

**仓**：`llm-agent-otel` | **Effort**：M | **Impact**：A

**现状**：`otelmetrics/otelmetrics.go:19-26` 已建 `timeToFirst` instrument（`gen_ai.client.operation.time_to_first_chunk`），但 `otelmodel/otelmodel.go:76-147` 的 streamReader 只发 `gen_ai.first_token` span event，**没有 record 这个 histogram**。在 Prometheus/Mimir 里 first-token 完全不可聚合。

**方案**：

```go
// otelmodel.go: streamReader 加 start time + recorder reference
type streamReader struct {
    inner llm.StreamReader
    span  trace.Span
    started time.Time
    recorder *otelmetrics.Recorder  // 通过 Config 注入
    sawContent, closed bool
}

func (r *streamReader) Next() (llm.StreamEvent, error) {
    ev, err := r.inner.Next()
    if !r.sawContent && err == nil && ev.Kind != llm.EventDone {
        r.span.AddEvent(otelroot.EventFirstToken)
        if r.recorder != nil {
            r.recorder.RecordTimeToFirst(r.ctx, time.Since(r.started), attrs...)
        }
        r.sawContent = true
    }
    ...
}
```

并暴露 `otelmodel.Config{TracerProvider, Recorder *otelmetrics.Recorder}`。

**替代方案对比**：

| 方案 | 优点 | 缺点 |
|---|---|---|
| **A. 在 streamReader 内自动 record（推荐）** | DX 好，零额外配置 | otelmodel.Config 加字段 |
| B. 暴露 `OnFirstChunk func(time.Duration)` 回调 | 灵活 | caller 自己 record |
| C. 让 otelmetrics.Recorder 实现 Observer 接口 | 解耦 | 增加抽象层 |

**验证**：`otelmodel_test.go::TestStream_RecordsTimeToFirstHistogram` 用 mock recorder 验证 RecordTimeToFirst 被调一次。

**依赖项**：与 P1-14（Recorder 自动接入）合并实现更经济。

---

#### P1-13 otel：otelslog 走 OTel log SDK

**仓**：`llm-agent-otel` | **Effort**：L | **Impact**：A

**现状**：`otelslog/otelslog.go:27-36` 仅追加 trace_id/span_id 到 slog record，没有走 `go.opentelemetry.io/otel/log` 通道。不能形成 trace + log + metric 三联在同一 OTLP backend。

**方案**：新增 `otelslog/bridge.go`，与现有 `Handler` 并存：

```go
// otelslog/bridge.go
import "go.opentelemetry.io/otel/log"

type Bridge struct {
    logger log.Logger
}
func NewBridge(lp log.LoggerProvider) *Bridge {
    return &Bridge{logger: lp.Logger("github.com/costa92/llm-agent-otel/otelslog")}
}
// 实现 slog.Handler，把每条 slog.Record 转 log.Record 写到 OTel log
```

老的 `Handler` 保留为"轻量关联"模式（不走 OTel log SDK，仅 trace_id 注入）。

**替代方案对比**：

| 方案 | 复杂度 | 兼容性 |
|---|---|---|
| **A. 新增 Bridge，老 Handler 保留（推荐）** | 中 | 老用户零变化 |
| B. 替换老 Handler | 低 | 老用户需要 OTel log SDK 依赖 |
| C. 给 Handler 加 `Mode = Annotate \| Bridge` 切换 | 中 | API 复杂 |

**验证**：`bridge_test.go` 用 mock LoggerProvider 验证 log Record 转 OTel record。

**依赖项**：go.opentelemetry.io/otel/log 已是 stable（v0.x ≥ 0.5）。

---

#### P1-14 otel：otelmodel 自动连接 Recorder

**仓**：`llm-agent-otel` | **Effort**：M | **Impact**：A

**现状**：`otelmetrics.Recorder` 与 `otelmodel.wrapper` 完全解耦（`docs/source-design-llm-agent-otel.zh-CN.md:726-727`），用户必须手动 `RecordTokenUsage` / `RecordDuration`，否则两个 instrument（token usage、operation duration）永远没人发。

**方案**：扩 `otelmodel.Config`：

```go
type Config struct {
    TracerProvider trace.TracerProvider
    MeterProvider  metric.MeterProvider  // 新增；nil 则不 record
    // 或更简单：直接接受 Recorder
    Recorder *otelmetrics.Recorder
}
```

在 `Generate` 与 `streamReader.end()` 末尾自动 record token usage + duration。

**与 P1-12 合并**：一并在 `Config.Recorder` 注入。

**验证**：`otelmodel_test.go::TestGenerate_RecordsMetricsAutomatically` 用 mock recorder，跑 Generate 后断言 3 个 instrument 都被调。

**依赖项**：与 P1-12 同步。

---

### 3.5 P1 DX 相关

#### P1-7 providers：DeepSeek/MiniMax 补 cancel + partial-usage conformance

> **状态（2026-05-22）：已 merge to master，PR #17 providers**。`internal/contract/generate_test.go` 的 providers 列表已含 `deepseek` 和 `minimax`；fixtures `testdata/deepseek/stream_cancel.json` + `testdata/minimax/stream_cancel.json` + `*_stream_partial_usage_error.json` 已落地；K2/K4 conformance 矩阵 5/5 闭合。下方原文不动以保留方案审计轨迹。

**仓**：`llm-agent-providers` | **Effort**：S | **Impact**：A

**现状**：`internal/contract/generate_test.go:281-375` 的 `TestStream_CancelMidStream_Conformance` / `TestStream_PartialUsageOnError_Conformance` 只跑 openai/anthropic/ollama，deepseek/minimax 没在 fixture 矩阵里。

**方案**：在 `generate_test.go` 的 `providers` 列表加 `deepseek`、`minimax`；写 2 个 fixture：

- `testdata/deepseek/stream_cancel.json` + `testdata/minimax/stream_cancel.json`
- `testdata/deepseek/stream_partial_usage_error.json` + minimax 同

每个 fixture 描述：mock server 写 1 chunk 后 hang 或注入错误。

**验证**：
```bash
cd llm-agent-providers && go test ./internal/contract/... -run TestStream_CancelMidStream_Conformance -count=1
```
新增 deepseek / minimax 路径都 PASS。

**依赖项**：无。

---

#### P1-8 providers：DeepSeek/MiniMax 显式 capabilitiesForModel

> **状态（2026-05-22）：已 merge to master，PR #18 providers，commits 5f619dd（deepseek）+ 4484ac0（minimax）**。`deepseek/capabilities.go` + `minimax/capabilities.go` 已落地 `capabilitiesForModel(model string) llm.Capabilities`；`options.go:93` 已改为 `Capabilities: capabilitiesForModel(cfg.model)`；测试 `*_test.go::TestCapabilities_ReflectModel` 已通过。K2 keystone GREEN（详见 ecosystem-design-review §2.2 / §3 keystone 表）。下方原文不动以保留方案审计轨迹。

**仓**：`llm-agent-providers` | **Effort**：S | **Impact**：A

**现状**：`deepseek/options.go:93-98` + `minimax/options.go:93-98` 硬编码 caps，不看 model。违反 K2 精神（`docs/source-design-llm-agent-providers.zh-CN.md:587`）。

**方案**：

```go
// deepseek/capabilities.go
func capabilitiesForModel(model string) llm.Capabilities {
    switch model {
    case "deepseek-chat", "deepseek-coder":
        return llm.Capabilities{Tools: true}
    case "deepseek-reasoner":
        // 假设 reasoner 不支持 tool（待验证）
        return llm.Capabilities{Tools: false}
    default:
        return llm.Capabilities{Tools: true}  // 保底
    }
}
```

即便所有 case 当前同字面，强制未来加 model 时显式 switch。

**验证**：`*_test.go::TestCapabilities_ReflectModel` 用每个 model 名构造，断言 caps 反映 model。

**依赖项**：无。

---

#### P1-18 flow：FlowEvent.Metadata 字段加

> **状态（2026-05-22）：已 merge to master，PR #3 flow v0.1.3，commits 77e5be1 + 373abf6 + e637e21 + 2157b1f**。`flow/event.go` 已加 `Metadata map[string]string`（additive，不破 apisnapshot）；新增 `MetadataAware` optional capability；引擎 `propagate MetadataAware metadata to NodeFinished events`；flowd SSE payload 同步 propagate；CHANGELOG bumped v0.1.3。下方原文不动以保留方案审计轨迹。

**仓**：`llm-agent-flow` | **Effort**：S | **Impact**：B

**现状**：`flow/event.go:14-34` 的 `FlowEvent` 不含 Metadata 字段，Tool 副作用（HTTP status code / exec exit code / token 用量）无法进事件流（`docs/source-design-llm-agent-flow.zh-CN.md:926-930`）。

**方案**（v0.1 additive）：

```go
type FlowEvent struct {
    Kind     FlowEventKind
    FlowID, NodeID string
    Input, Output, Outputs map[string]string
    Metadata map[string]string  // 新增；可由 NodeKind 在 Run 返回时填充
    Err      error
}

// node.go: NodeKind 接口可选支持 metadata 返回
type MetadataAware interface {
    NodeKind
    Run(ctx context.Context, input map[string]string) (output map[string]string, meta map[string]string, err error)
}
```

引擎 type-assert 判断 node 是否实现 MetadataAware，若是则把 meta 写入 NodeFinished 事件。

**验证**：`flow/engine_test.go::TestEngine_EmitsMetadataForMetaAwareNodes`。

**依赖项**：无。也是 P1-21 flowrunner 接入 production 时 OTel span attribute 的源数据。

---

#### P1-19 customer-support：toolAgent 补 ReAct 第二轮

**仓**：`llm-agent-customer-support` | **Effort**：M | **Impact**：C

**现状**：`toolagent.go:57-133` 单趟：LLM call → execute tools → 拼接输出当 final answer，没有 observation→final synthesis。客服话术粗糙。

**方案**：

- 选项 A：直接复用 `llm-agent` 核心的 `agents.NewReactAgent`（前提：v0.5 稳定足够）；
- 选项 B：在 toolAgent 内加一次 observation 后的 LLM call：

  ```go
  // toolagent.go: 执行完工具后
  observation := fmt.Sprintf("Tool outputs:\n%s", joinedOutputs)
  finalResp, err := o.model.Generate(ctx, llm.Request{
      Messages: []llm.Message{
          {Role: "system", Content: "Synthesize a final customer-friendly answer based on the tool outputs."},
          {Role: "user", Content: input},
          {Role: "assistant", Content: observation},
      },
  })
  return finalResp.Text, nil
  ```

**推荐 B**（更可控、不强依赖 core 演进）。

**替代方案对比**：

| 方案 | LLM 调用次数 | 客服质量 | 复杂度 |
|---|---|---|---|
| 现状 | 1 | 低 | 低 |
| **B. 加 synthesis（推荐）** | 2 | 中-高 | 中 |
| A. 完整 ReAct | 2-N（直到 final） | 高 | 高（要 react agent 核心稳） |

**验证**：`toolagent_test.go::TestRun_SynthesizesFinalAnswer` 用 ScriptedLLM 编脚本：(1) 返 ToolCall → (2) 收 tool output → (3) 返润色后 answer。

**依赖项**：可独立完成；与 P1-20（session 历史截断）配合更优。

---

#### P1-21 customer-support：flowrunner 接入 production handler

**仓**：`llm-agent-customer-support` | **Effort**：M | **Impact**：C

**现状**：`internal/flowrunner/*` 测试完备，但 `cmd/server/main.go` 与 `internal/app/app.go` 不构造它，`httpapi.go` 没有 `/flow/run` 端点。文档 `docs/flowrunner.md:95-110` 给出的接入代码引用了不存在的 `App.TracerProvider()` / `App.Tools()` getter。

**方案**：

1. `app.go` 加 getter：
   ```go
   func (a *App) TracerProvider() trace.TracerProvider { return a.tp }
   func (a *App) Tools() []agents.Tool { return a.tools }  // 暴露 registry 内容
   ```
2. `app.go` 装配可选 `*flowrunner.Runner`：
   ```go
   if cfg.EnableFlowRunner {
       runner, err := flowrunner.New(flowrunner.Config{
           TracerProvider: tp,
           Tools:          tools,
           ...
       })
       handlers.FlowRunner = runner
   }
   ```
3. `httpapi.go` 加 `POST /flow/run` 与 `POST /flow/run/stream`，请求体含 flow JSON + inputs。

**验证**：`httpapi_test.go::TestFlowRun_Success` + `TestFlowRunStream_EmitsEvents` 用 echo_chain flow JSON。

**依赖项**：无。`docs/flowrunner.md` 同步更新示例代码。

---

## 4. P2 长期演进（v1.3+）

### P2-1 llm-agent：Message.ToolCallID + 多轮 FunctionCallAgent

**仓**：`llm-agent` | **Effort**：M | **Impact**：A

**现状**：`function_call.go:18-19` 注释明示因 `llm.Message` 无 `ToolCallID` 字段，FunctionCallAgent 只能单轮。多轮 native function calling 用不了。

**方案**：v1 加法兼容地给 `llm.Message` 加字段：

```go
// llm/types.go
type Message struct {
    Role    string
    Content string
    Name    string  // 已有
    ToolCallID string  // 新增；仅 role="tool" 时填
}
```

更新所有 5 个 provider 的 mapping 把它带过去；FunctionCallAgent 内部循环：tool result → assistant message → next LLM call。

**风险**：触及 v0.4 锁定的 4 个 validated 类型之一。需先在 SUMMARY 立项 "additive-only field add"。

**验证**：providers conformance 加 multi-turn tool fixture；`function_call_test.go::TestRun_MultiTurnFunctionCalling`。

---

### P2-7 flow：JSON Schema for IR

**仓**：`llm-agent-flow` | **Effort**：M | **Impact**：A

**现状**：`docs/source-design-llm-agent-flow.zh-CN.md:935-942` 提议但未实现。100-node flow 手写易错；IR 字段名重命名无 CI 守门。

**方案**：

1. 写 `flow/api/v0.1.flow-schema.json`（手写或用 `gojsonschema` 反推 from Go struct）；
2. 在 `flow/v01_schema_test.go` 校验若干 valid / invalid fixture；
3. README + docs 引用，便于 IDE 用（VS Code 的 `json.schemas` 配置）；
4. v1 SemVer 文档承诺"字段名锁定"。

**验证**：`schema_test.go::TestSchema_ValidatesValidIRs` + `_RejectsInvalidIRs`。

---

### P2-8 llm-agent：bench/rl 拆独立仓

**仓**：`llm-agent` | **Effort**：L | **Impact**：A

**现状**：`bench/` + `rl/` 与 agent 框架耦合弱（只依赖 `agents.Agent` + `llm.ChatModel`），受 stdlib-only 约束反受其累 —— bench 想引入 statistics 库不能。

**方案**：

- 建 `llm-agent-bench` 仓（依赖 llm-agent）；
- 建 `llm-agent-rl-eval` 仓（依赖 llm-agent）；
- 核心 `llm-agent/bench/` 与 `llm-agent/rl/` 给 deprecated alias，v0.7 移除；
- 拆完 core 从 ~12K LOC 降到 ~10K，stdlib-only 约束更专注。

**验证**：build + test pass；CI cascade 加 2 个新 repo。

---

### P2-11 / P2-12 llm-agent：Termination 子包化 + As*Tool 命名统一

合并讨论：v1 前最后的 breaking 窗口可以一并做这两件事。

- `Termination` 抽到 `orchestrate/termination/` 子包或 rename 为 `ChatTermination` —— 反映它只用于 RoundRobin/RolePlay；
- `memory.AsTool` / `comm/{mcp,a2a,anp}.AsAgentTool(s)` 统一为 `AsTool(s)` 复数/单数明确。

**Effort**：M | **Impact**：B | **触发条件**：v1 release prep。

---

## 5. 4 维度详细分析与替代方案对比（汇总）

### 5.1 架构维度

跨条目的核心问题：**"加 method 不行（v1 frozen），改类型不行（validated type），但要扩展 = 怎么做？"**。

生态当前的范式是 **"sibling capability + type assertion"**（§4.2 评审）。一致执行得很好。

但有几个**典型未规范**情况：

1. `Message.ToolCallID` 加字段（P2-1）—— 给 validated type 加 field 是否可接受？  
   **观点（署名）**：可接受。Go 加 struct field 在 binary level 是兼容的（callers' 已构造的零值结构体在反序列化时新字段为零值），只要 v1 文档明示"字段是 additive"。
2. `flow.Store` 接口加 `AppendRunEvents` 方法（P1-17 关联）—— v0.1.1 已用 type-assert 绕过。  
   **观点**：这条范式应被文档为 "v1.x 通用扩展模式"。
3. RAG facade（P0-2）—— 反向边的"豁免"如何成立？  
   **观点**：要么真用、要么删；处于 "声明但不用" 的状态最糟。

### 5.2 性能维度

按"投入产出比"排：

| 优化 | 改动量 | 收益倍数 | ROI |
|---|---|---|---|
| P1-17 flow SQLite WAL + multi-VALUES | S | 5-16x | ★★★★★ |
| P1-16 rag BatchEmbedder | M | 5-20x（真模型） | ★★★★★ |
| P1-15 rag Hybrid 并发化 | M | 2-4x | ★★★★ |
| P1-1 rag pgvector index | M | 19x（大表） | ★★★★ |
| P1-12/14 otel metrics 接入 | M | 可观测增加（非直接性能） | ★★★ |
| P1-20 customer-support 历史截断 | S | 防超 budget | ★★★ |

通用原则：**先解锁瓶颈（embed batch + index），再做并发（hybrid）**。

### 5.3 可观测维度

OTel 在生态里覆盖 **trace 全面 / metric 半连 / log 仅关联**。补齐顺序建议：

1. P1-10/11 sampler + env 接管（防生产爆量）—— 最基础；
2. P1-12/14 metric 接入（让 dashboard 真有数据）；
3. P1-13 log SDK 桥（三联完整）。

详细对比表见路线图条目本身。

### 5.4 DX 维度

DX 缺口集中三类：

1. **包名 / 命名不统一**（P1-5 context、P2-11 Termination、P2-12 As*Tool）—— v1 前一次性 rename；
2. **测试覆盖缺角**（P1-7 deepseek/minimax conformance）—— 单独条目处理；
3. **文档与代码漂移**（P0-1 guardrails、P1-21 flowrunner、`docs/flowrunner.md` 引用不存在的 getter）—— 加 CI "docs-claims" 测试（见 §7）。

---

## 6. 迁移与发布序列建议

### 6.1 v1.2 收尾窗口（2-3 周）

只做 P0：

- P0-1 guardrails wiring fix（0.5 人天）**(done, 2026-05-22, PR #11 customer-support commit 9171a0a)**
- P0-2 RAG facade 决策与落地（2 人天）**(done, 2026-05-22, drop-dependency 路径, commit 6029565)**

### 6.2 v1.3 milestone（4-6 周）

主题：**"Streaming 完备性 + 可观测三联"**。

> **进度盘点（2026-05-23 EOD — v1.3 milestone closed）**：
> - **first wave 完成 + P1-6 / P1-23 闭合**：P1-3 / P1-4 / P1-22 + P1-9（5/5 闭合）+ **P1-6 providers default timeout（done, 随 P1-23 三连 PR providers v0.2.4 同步落地）** + **P1-23 internal/compat 抽取（done, PR #28 / #29 / #30 providers v0.2.4 — 5/5 共享 `DefaultTimeout`、4/5 共享 `WrapOpenAIError` / `WrapAnthropicError`；ollama 保留 atomic-state 模式）**。
> - **fourth wave 部分完成**：P1-7（PR #17 providers）+ P1-8（PR #18 providers）+ P1-18（PR #3 flow v0.1.3）+ **D3 flow MetadataAwareTool（done, flow v0.1.4，PR #8 flow — `toolNode` 实现 `MetadataAware`，built-in `http` / `exec` 工具实现 `MetadataAwareTool` sibling capability）**。剩 P1-19 / P1-20 / P1-21。
> - **third wave perf 棒（rag）3/3 闭合（2026-05-23 v1.3 perf-wave）**：P1-16 已 ship in rag v1.0.3（PR #5，merge af9b5b8）→ P1-15 已 ship in rag v1.0.4（PR #6，merge ff7af07）→ P1-1 已 ship in rag v1.0.5（PR #7，merge 3c92585）。customer-support 已 repin 到 rag v1.0.5（PR #21 customer-support，commit 85b7ad7）摘取 P1-15 + P1-1 收益；并通过 T5 test-pin SSE cancel 契约（`httpapi` PR commits d4ddf9f / db0174d）。
> - **third wave 部分完成（flow 部分）**：P1-17 WAL 已 merge（PR #2 flow v0.1.2）；multi-VALUES INSERT 仍 pending。
> - **second wave 未启动**：P1-10 / P1-11 / P1-12 / P1-14 待 v1.4 启动。
> - 同步：P0-1（commit 9171a0a）+ P0-2（commit 6029565）已在 v1.2 收尾完成（详见 §6.1）。

第一波（架构修复）：
- P1-3 a2a goroutine ctx **(done, PR #3 llm-agent)**
- P1-4 RunStream ctx-cancel Done **(done, PR #2 llm-agent)**
- P1-6 providers default timeout **(done, providers v0.2.4, 随 P1-23 三连 PR 同步, 2026-05-23)**
- P1-9 Retry-After 解析（5 个 provider 闭合）**(done, PR #19 + PR #20 providers, 5/5)**
- P1-22 customer-support readyz 真探测 **(done, PR #16 customer-support)**
- P1-23 providers internal/compat 抽取 **(done, providers v0.2.4, PR #28 + #29 + #30, 2026-05-23 — 原计划 v1.4，提前落地)**

第二波（可观测）：
- P1-10 sampler 暴露
- P1-11 OTel env 接管
- P1-12 + P1-14 timeToFirst histogram + Recorder 自动接

第三波（性能）：
- P1-17 flow SQLite WAL **(WAL done, PR #2 flow v0.1.2; multi-VALUES INSERT pending)**
- P1-1 rag pgvector index **(done, rag v1.0.5, PR #7 merge 3c92585, 2026-05-23)**
- P1-16 rag BatchEmbedder **(done, rag v1.0.3, PR #5 merge af9b5b8, 2026-05-23)**
- P1-15 rag Hybrid 并发 **(done, rag v1.0.4, PR #6 merge ff7af07, 2026-05-23)**

第四波（DX 与测试覆盖）：
- P1-7 / P1-8 deepseek/minimax conformance + capabilitiesForModel **(done, PR #17 + PR #18 providers)**
- P1-18 FlowEvent.Metadata **(done, PR #3 flow v0.1.3)**
- P1-19 toolAgent ReAct 第二轮
- P1-20 session 历史截断
- P1-21 flowrunner 接入

### 6.3 v1.4 milestone（6-8 周）

主题：**"v1 stability + breaking 清理"**。

- P1-5 context 包改名（breaking，v1.0 前的最后机会）
- P1-13 otelslog 走 OTel log SDK
- ~~P1-23 providers internal/openaicompat 抽取~~ — **已提前 ship in providers v0.2.4**（2026-05-23 v1.3 milestone 闭合）；v1.4 仅剩 streaming reader 抽取（ollama Path B 评估）作为后续 follow-up
- P2-2 P2-3 P2-4 P2-6 P2-9 P2-10 内部 bug fix

### 6.4 v2 / v1.x 分叉（可选）

- P2-1 Message.ToolCallID（若决定走 additive，在 v1.x；若 breaking，在 /v2）
- P2-7 flow JSON Schema（additive）
- P2-8 bench/rl 拆仓
- P2-11 P2-12 命名统一（v2 path）

---

## 7. CI 闸子增强建议

| 增强 | 触发条件 | 实现 |
|---|---|---|
| **G1：docs-claims test** | 每个 PR | 自定义 lint：扫 README 中 "day-one defense" / "wraps with OTel" 之类承诺，要求 production 代码路径 grep 到对应 wiring | 防 P0-1 类 README ≠ wiring 漂移 |
| **G2：api/v1 行为 golden** | rag 每个 PR | 在 `apisnapshot_test.go` 之外加 `golden_test.go`：固定输入跑 `WeightedPathRanker` / `LouvainDetector` / `HybridRetriever` 等，输出 diff 已 commit 黄金文件 | 防默认值改变（如 `Resolution` 0.5→0.6）无 CI 失败 |
| **G3：stdlib-only with explicit allowlist** | 每个 PR | B4 gate 已存在；维护"允许的反向边"显式白名单（仅 llm-agent ← llm-agent-rag），任何新加白名单需 SUMMARY 立项 | 防 phantom 反向边累积 |
| **G4：ParseRetryAfter unit test** | providers PR | unit test 锁定 ParseRetryAfter 5 个 case（数字秒、HTTP-date、空、非法、过期日期） | P1-9 落地后必须 |
| **G5：provider conformance 完整性** | providers PR | `internal/contract/generate_test.go` 中跨 provider 矩阵不允许出现 `t.Skip` 或缺失 fixture；CI 强制 5 个 provider 都跑 5 个核心 conformance | P1-7 |
| **G6：gen_ai semconv signature drift** | otel PR | 锁定 `semconv_gen_ai.go` 的 23 个常量；任何重命名 / 删除走 SUMMARY，确保 dashboard PromQL 不失效 | 防 semconv experimental 升级误伤生产 |
| **G7：flow IR JSON Schema check** | flow PR | `flow/api/v0.1.flow-schema.json` 落地后，所有 example flow.json 必须 schema-validate 过 | P2-7 落地后 |
| **G8：no implicit context.Background in goroutine** | core / cs PR | 自定义 lint：检查 `go func()` 内首行 `ctx := context.Background()` 模式，要求注释解释或显式 cancel pair | 防 P1-3 类 ctx 泄露复发 |
| **G9：generateFromPrompt budget chokepoint guard** | core PR | 加 vet/lint：任何新加 `model.Generate(...)` 调用点必须走 `generateFromPrompt` 包装，或在 SUMMARY 明示豁免 | 保 CC-1 keystone |

---

## 8. 验证 / 回归测试矩阵

按本路线图条目交叉测试名 / metric 名 / 命令：

| 路线图条目 | 引入的新测试 | 命令 |
|---|---|---|
| P0-1 | `TestNew_GuardrailsWired` + `TestChat_RejectsPromptInjection` | `go test ./internal/app/... ./internal/httpapi/...` |
| P0-2 A | `rag/facade_test.go` (type alias 等价) | `go test ./rag/...` |
| P0-2 B | `go mod why github.com/costa92/llm-agent-rag` 返回 "not needed" | shell command |
| P1-1 | `postgres_conformance_test.go::TestSearch_WithIVFFlat` | `LLM_AGENT_RAG_PG_URL=... go test ./postgres/...` |
| P1-2 | `eval/global_test.go::TestSanitizeReports_DropsInjection` | `go test ./eval/...` |
| P1-3 | `comm/a2a/server_test.go::TestDeleteTask_CancelsRunningHandler` | `go test ./comm/a2a/...` |
| P1-4 | `agent_test.go::TestRunStreamFromBlocking_EmitsDoneOnCancel` | `go test ./...` |
| P1-6 | `*_test.go::TestNew_HasDefaultTimeout` | `go test ./...` |
| P1-7 | `internal/contract/generate_test.go` 全 5 provider PASS | `go test ./internal/contract/...` |
| P1-9 | `internal/openaicompat/retryafter_test.go::TestParseRetryAfter*` | `go test ./internal/openaicompat/...` |
| P1-10 | `exporters_test.go::TestNewTracerProvider_HonorsSampler` | `go test ./...` |
| P1-11 | `exporters_test.go::TestDefault_HonorsEnv` | `go test ./...` |
| P1-12 + P1-14 | `otelmodel_test.go::TestStream_RecordsTimeToFirstHistogram` | `go test ./otelmodel/...` |
| P1-15 | wall-clock benchmark + golden trace | `go test ./retrieve/... -bench BenchmarkHybridRetriever` |
| P1-16 | `rag/import_test.go::TestImport_UsesBatchEmbedderWhenAvailable` | `go test ./...` |
| P1-17 | `store/sqlite/events_bench_test.go::BenchmarkAppendRunEvents_Batch_1000` | `go test ./store/sqlite/ -bench .` |
| P1-19 | `toolagent_test.go::TestRun_SynthesizesFinalAnswer` | `go test ./internal/supportflow/...` |
| P1-21 | `httpapi_test.go::TestFlowRun_Success` | `go test ./internal/httpapi/...` |
| P1-22 | `httpapi_test.go::TestReadyz_FailsWhenDBDown` | `go test ./...` |
| P1-23 | conformance 全套 GREEN + `internal/openaicompat/stream_test.go` | `go test ./internal/...` |

**关键 metric 名（落地后应能在 dashboard 看到非零）**：
- `gen_ai.client.operation.time_to_first_chunk`（P1-12）
- `gen_ai.client.operation.duration`（P1-14）
- `gen_ai.client.token.usage`（P1-14）
- `rag.requests` / `rag.errors` / `rag.operation.duration` / `rag.tokens`（已存在）
- `flow_node_duration_seconds`（P2 候补，O3 条目）

---

## 9. 附：每个仓的局部 backlog（按仓分组）

### 9.1 `llm-agent`

- P0-2 RAG facade 落地或删除
- P1-3 comm/a2a goroutine ctx
- P1-4 RunStream ctx-cancel Done event
- P1-5 context 包重命名（v1 前 breaking 窗口）
- P2-1 Message.ToolCallID + 多轮 FunctionCallAgent
- P2-2 memory.Consolidate 衰减源项 Importance
- P2-3 policy.BlockedError 承接 budget err
- P2-8 bench/rl 拆独立仓
- P2-11 Termination 子包化或 rename
- P2-12 As*Tool 命名统一

### 9.2 `llm-agent-rag`

- ~~P1-1 Postgres Migrate 加 vector index 选项~~ **(done, rag v1.0.5, PR #7, 2026-05-23)**
- P1-2 AskGlobal/AskDrift injection sanitize
- ~~P1-15 HybridRetriever 四路并发~~ **(done, rag v1.0.4, PR #6, 2026-05-23)**
- ~~P1-16 BatchEmbedder optional capability~~ **(done, rag v1.0.3, PR #5, 2026-05-23)**
- P2-4 ingest.Importer 命名收敛或 deprecate
- P2-5 api/v1.snapshot.txt 加行为 golden
- P2-9 MarkdownSplitter 识别 setext + code fence

### 9.3 `llm-agent-providers`

- ~~P1-6 5 个 provider 默认 timeout~~ **(done, providers v0.2.4, 随 P1-23 三连 PR 同步落地，2026-05-23)**
- ~~P1-7 DeepSeek/MiniMax conformance 补齐（cancel + partial-usage）~~ **(done, PR #17 providers, 2026-05-22)**
- ~~P1-8 DeepSeek/MiniMax 显式 capabilitiesForModel~~ **(done, PR #18 providers, 2026-05-22)**
- ~~P1-9 anthropic/ollama/minimax 解析 Retry-After~~ **(done, PR #19 + PR #20 providers, 2026-05-22)**
- ~~P1-23 抽 internal/openaicompat / anthropiccompat~~ **(done, providers v0.2.4, PR #28 + #29 + #30 落地 `internal/compat` 单包, 2026-05-23)**
- 文档：openai/anthropic doc.go 重写 Phase-1 残留
- 文档：README 版本号过期

### 9.4 `llm-agent-otel`

- P1-10 NewTracerProvider 暴露 sampler
- P1-11 接管 OTel 标准 env
- P1-12 streamReader 接入 timeToFirst histogram
- P1-13 otelslog 走 OTel log SDK
- P1-14 otelmodel 自动连接 Recorder
- 文档：明确 Wrapper 与 Observer 不同时使用的语义

### 9.5 `llm-agent-flow`

- P1-17 SQLite WAL + multi-VALUES INSERT
- P1-18 FlowEvent.Metadata 字段
- P2-6 Validate 检查 Edge multi-target 冲突
- P2-7 JSON Schema 落地
- 文档：YAML 加载器作为 v0.2.x 候补

### 9.6 `llm-agent-customer-support`

- **P0-1 guardrails wiring fix（最紧急）**
- P1-19 toolAgent 补 ReAct 第二轮
- P1-20 session 历史 tail-N
- P1-21 flowrunner 接入 production
- P1-22 /readyz 真探测
- P2-10 estimateTokens 改 BPE
- DX：加 `compose/.env.example`
- DX：加 `cmd/server-mock` 用 ScriptedLLM

---

*路线图完成于 2026-05-21。本文条目可直接转 GitHub Issue —— 每条都已有编号、severity、effort、影响面、推荐方案、验证方式。*
