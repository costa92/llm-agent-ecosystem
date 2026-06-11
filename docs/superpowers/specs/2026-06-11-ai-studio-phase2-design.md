# AI Studio 二期技术设计文档（M4：异步长任务引擎 + 视频/音频生成接缝）

- 日期：2026-06-11
- 状态：设计待评审（第二轮 codex 修复已应用，§15.3；待 Plan agent 复核后落 impl plan）
- 类型：准生产级里程碑设计（独立 sibling 仓 `github.com/costa92/llm-agent-studio`，main @ v0.3.0）
- 上游：[[2026-06-10-ai-studio-design]]（父设计文档，§13 R5 + §15 M4 强制要求二期「另写 spec + build-vs-buy」——**本文即该 spec**）；[[2026-06-10-llm-agent-studio-prd]]（PRD，模块7 视频/音频模型清单 + §8 二期范围）
- 范围决策（用户已选 **Option A**）：本里程碑只建 **异步长任务引擎 + 视频/音频 `MediaGenerator` 接缝 + fake 异步 generator（sandbox 内可活验）+ key-gated 真实 provider 适配器骨架（薄可插拔）**。**显式延后**：配音(dubbing) / 自动剪辑(ffmpeg) / 图片 LoRA / 数字人(digital human)——见 §10 非目标。
- 命令约束：所有 Go 命令需 `GOWORK=off`（见 [[project_console-gowork-off]]）；LSP 诊断为陈旧噪声，以 `GOWORK=off go build/test` 为准。

---

## 1. 背景与范围

### 1.1 为什么 M4 需要独立设计

父设计文档把生成层抽象为统一接缝 `MediaGenerator`（[[2026-06-10-ai-studio-design]] §7.2），并把视频/音频明确划为「二期，靠新增 generator 适配器加性扩展」，同时在 §13 R4/R5 标出两个高风险：

- **R4 异步长任务（中）**：视频生成分钟级 + 轮询，当前 worker 是「单次 dispatch 内同步执行」模型，分钟级任务会占满 worker 池、撞 `CallTimeout`、撞租约。
- **R5 二期生成全缺失（高）**：生态 `llm-agent-contract` / `llm-agent-providers` **零** 视频/音频/TTS 支持——已亲自核对（§3）。

父文档因此要求：二期落地前先出独立 spec + build-vs-buy。本文承担该职责，并把范围收敛到 Option A——**先把引擎与接缝做到准生产、可活验**，把真正烧钱/需外部凭据/需 ffmpeg 的部分留作 key-gated 骨架与后续里程碑。

### 1.2 范围边界（Option A）

| 维度 | M4 决策 |
|---|---|
| 核心 | **异步长任务引擎**：提交→轮询 的 todo 状态机 + 租约续约(heartbeat) + 按 kind 的并发隔离（§5） |
| 接缝 | `MediaGenerator` **加性扩展** 出异步形态（`AsyncGenerator` 可选接口 `Submit`/`Poll`），image 适配器**零改、仍单遍**（§4） |
| 资产类型 | 新增 `video` / `audio`（asset.type 本就是自由 TEXT，无白名单——§6.1） |
| 计费 | `pricing` 表加 `micros_per_second`；`generations.video_seconds` 已存在；音频按秒计（§6.3） |
| 可活验 | **fake 异步 generator**（确定性 submit→poll，零网络）——M4 在 sandbox 内活验的关键（§7） |
| 真实适配器 | Runway/Kling/Veo（视频）+ 一个 TTS（音频）的**适配器接口 + key-gated 注册骨架**，照搬 M3 `registerImageGenerators` 范式；真实 SaaS HTTP 接线 + 密钥 + ffmpeg **out-of-band / 延后**（§8） |
| 复用 | 全量复用 M3 已建范式：`pricing`+`RecordPriced`、otel span、配额/并发、SSRF-safe `internal/fetch`、SSE 白名单、`scope_kind="org"` RBAC、签名 URL（§9） |
| 非目标 | 配音 / 自动剪辑(ffmpeg) / LoRA / 数字人（§10） |

### 1.3 成功标准（端到端可验证，sandbox 内）

登录 → 建项目（`content_type` 含视频/音频意图）→ 运行 → Planner 产 todo 图（含 `asset` 节点，kind 经路由解析为 video/audio）→ worker 领 asset todo → **fake 异步 generator** 返回「已提交，job_id=X，待轮询」→ worker 把 todo 以 `next_run_at` 退避 **重排为 polling 态**（不阻塞单次 dispatch）→ 下一次/数次 dispatch 轮询 fake 直到 `ready` → 拉回字节落 BlobStore → 资产 `pending_acceptance` → admin 采纳 → 资产库按 `type=video/audio` 检索 → `generations` 账本按秒计费累计。后端 `httptest` + 真 Postgres + MinIO，全程**不调真实外部 API**（fake 覆盖引擎+接缝）。

---

## 2. 生态能力盘点（亲自核对，决定自建边界）

> 命令：`GOWORK=off go list -m -f '{{.Dir}}' github.com/costa92/llm-agent-contract` → `@v0.5.0`；`...llm-agent-providers` → `@v0.7.0`。逐目录 grep `video|audio|tts|speech|voice|VideoGenerat|AudioGenerat|TextToSpeech`。

- **❌ 视频生成**：`llm-agent-contract@v0.5.0/llm/` 仅 `chatmodel.go` / `image.go` / `stream.go` / `capabilities.go` 等——**无任何 video 契约**。`llm-agent-providers@v0.7.0`（anthropic/deepseek/google/kimi/minimax/ollama/openai/volcengine）grep `VideoGenerat|AudioGenerat|TextToSpeech|SpeechGenerat` **零命中**。
- **❌ 音频/TTS**：同上，零命中。
- **结论**：M4 **自建** 视频/音频接缝与适配器；真实 SaaS = 外部 HTTP（项目内 `internal/generate/video`、`internal/generate/audio` 各为新包，照搬 `internal/generate/image` 的加性扩展模板）。生态侧**不**新增契约（避免给 contract/providers 引入未验证的 video 表面——保持 studio 自包含，与 §3 image 路径一致）。
- **✅ 可复用**：`internal/generate`（接缝/registry/fake，`generate.go:34-38`）、`internal/worker`（租约队列，`worker.go`）、`internal/fetch`（SSRF-safe 拉取，`fetch.go:40` `New(Config{...})` + `fetch.go:89` `Get(ctx,url) ([]byte,string,error)`）、`internal/cost`（`pricing`+`RecordPriced`，`cost/store.go:132`）、`internal/assets`（自由 TEXT type，`assets/store.go`）、`internal/storage`（m1/m2/m3 迁移 slice，`storage.go:42/125/182`）。

---

## 3. 总体架构（M4 增量）

M4 不改变进程/容器边界（单 `studiod` + Postgres + BlobStore + otel collector，父文档 §4）。增量集中在 `internal/generate`（接缝扩展 + video/audio 包）与 `internal/worker`（异步引擎）：

```
worker 池(FOR UPDATE SKIP LOCKED + 租约)
  └─ runAsset(asset todo)
       ├─ 同步路径(image, 不变): MediaGenerator.Generate → 一遍出字节 → BlobStore → pending_acceptance
       └─ 异步路径(video/audio, NEW):
            phase=submit:  GetOrCreateForTodo(todoID) → 复用/建唯一 asset 行(崩溃幂等, B1)
                           AsyncGenerator.Submit(req, idemKey=hash(todoID)) → {ExternalJobID, 估算用量}
                           → 【单事务】SetSubmitted(external_job_id) + generations upsert + todo 重排
                             (status='ready', phase=poll, next_run_at=now()+pollBackoff)
                           ← 崩溃恢复: reclaim 发现 asset 已 submitted+有 external_job_id → 跳过 Submit, 直接续轮询
            phase=poll:    AsyncGenerator.Poll(jobID) → {Pending | Done(URL/Bytes) | Failed} | transient err
                           Pending/transient-err → next_run_at 退避重排 (poll_attempts++; 不调 SetAsyncFailed)
                           poll_attempts≥Max     → 终态: SetAsyncFailed + todo MarkFailed (立即, 不退避)
                           Done    → fetch.Get(URL) 拉回字节 → BlobStore(覆盖写) → pending_acceptance → UpdateGenerationByAssetTodo(按秒)
                           Failed  → 终态: SetAsyncFailed + todo MarkFailed (provider 明确报失败, 立即)
```

并发隔离：claim SQL 的全局 asset 并发上限（`worker.go:165-174`）扩展为 **按 kind 的子上限**，使分钟级 video 任务不饿死快速 image/script todo（§5.3）。

---

## 4. 生成抽象扩展（`MediaGenerator` 接缝，加性）

### 4.1 现状（必须不破坏）

`internal/generate/generate.go:9-38` 当前定义（逐字）：

```go
type GenRequest struct {
    Prompt  string
    N       int
    Size    string
    Quality string
    Format  string
}
type GenResult struct {
    Bytes      []byte
    URL        string
    MimeType   string
    Provider   string
    Model      string
    Tokens     int
    ImageCount int
    LatencyMS  int
}
type MediaGenerator interface {
    Kind() string // "image" | "video" | "audio"
    Generate(ctx context.Context, req GenRequest) (GenResult, error)
}
```

image 适配器 `internal/generate/image/image.go:48` 实现 `Generate` 为**单遍同步**（调 `ImageGenerator.GenerateImage` → 必要时 `puller.Get` 拉 URL → 返回 Bytes）。M4 **不得**改动这条路径。

### 4.2 决策：可选 `AsyncGenerator` 接口对（Submit/Poll），而非给 GenResult 加状态枚举

**选定形态（NEW）**：在 `generate` 包新增一个 **可选实现的接口**，异步 generator 同时实现 `MediaGenerator`（保持 registry/路由统一）与 `AsyncGenerator`：

```go
// NEW — internal/generate/generate.go
// GenRequest 新增可选字段(加性, image 适配器忽略即可):
//   DurationSeconds int    // 视频/音频时长诉求(计费 + provider 参数)
//   Voice           string // TTS 音色(audio)
// (放在 GenRequest 末尾, 现有 image 调用零改 — AssetAgent.RunWith 构造 GenRequest
//  只填 Prompt/N/Size, 见 agents/asset.go RunWith)

type SubmitResult struct {
    ExternalJobID string // provider 侧 job 句柄, 落 asset.external_job_id
    Provider      string
    Model         string
    EstSeconds    int    // 提交时已知的时长(计费预登记/估算; 真实秒数 Poll 完成时回填)
}

type PollStatus int
const (
    PollPending PollStatus = iota // job 仍在跑 → 退避重排
    PollDone                       // 完成 → result 带 URL/Bytes + 真实用量
    PollFailed                     // provider 侧失败 → todo fail
)

type PollResult struct {
    Status   PollStatus
    Result   GenResult // Status==PollDone 时填(URL 优先, 经 fetch 拉回)
    Err      string    // Status==PollFailed 时的 provider 错误
}

// AsyncGenerator 是长任务 generator 的可选接缝。Kind()=="video"|"audio" 的
// generator 实现它; image 不实现(worker 据此走同步路径)。
//
// idempotencyKey(B1, 第二轮): Submit 携带一个【确定性幂等键】(由 todoID 派生,
// 见 §5.2 崩溃幂等)。真实适配器【必须】把它转发为 provider 侧的
// client-token / idempotency header —— 这样 submit 之后、external_job_id 落库
// 之前进程崩溃, lease 过期 → 二次 Submit 时 provider 据同一 key 去重, 不会
// 重复建计费 job。fake 适配器【回显】该 key(用于断言透传)。
type AsyncGenerator interface {
    MediaGenerator                                              // 仍是 MediaGenerator(registry 统一)
    Submit(ctx context.Context, req GenRequest, idempotencyKey string) (SubmitResult, error)
    Poll(ctx context.Context, jobID string) (PollResult, error)
}
```

**为什么选 Submit/Poll 接口对，而非给 `GenResult` 加 `Status`/`ExternalJobID`**：

1. **不污染 image 路径**：image 适配器返回的 `GenResult` 永远是「完成态字节」，给它加一个它永不设置的 `Status` 字段会让每个 image 调用点都要判空，违反父文档「image 仍单遍」。接口对让 worker 用 **类型断言** `g.(generate.AsyncGenerator)` 一次性分流，image 适配器不需要任何改动。
2. **registry 零改**：`AsyncGenerator` 内嵌 `MediaGenerator`，`Registry.Register`/`Resolve`（`registry.go:24/38`）签名不变，仍按 `provider/model` 解析；worker 拿到 generator 后再断言是否异步。
3. **`Generate` 对异步 generator 的语义**：异步 generator 仍须实现 `Generate`（接口要求），其实现 = **「Submit 然后阻塞轮询到完成」** 的便捷封装（供非 worker 调用方/单元测试用）。但 **worker 不走 `Generate`**——worker 显式走 Submit/Poll 分阶段，以避免单次 dispatch 阻塞分钟级（§5.2）。fake 异步 generator 的 `Generate` 也这样实现，保证两种调用方式语义一致。

**驳回的备选**：(a) 给 `GenResult` 加 `Status int + ExternalJobID string`——污染 image，每个消费点要判空，否决。(b) 单独的顶层 `Submit/Poll` 方法挂在 `Registry` 上——把异步语义泄漏进 registry，且 image 也被迫有空实现，否决。

### 4.2b otel 包装层必须保留 AsyncGenerator 接缝（B1，**载重修复**）

**问题（亲核）**：worker 的同步/异步分流依赖类型断言 `routed.(generate.AsyncGenerator)`（§5.2），但注册时 generator 经 `obs.WrapGenerator` 包了一层 otel span——当前 `obs.WrapGenerator`（`obs/obs.go:51-56`）**无条件** 返回 `*tracedGenerator`（`obs/obs.go:58-81`），而 `tracedGenerator` **只实现 `Kind()`+`Generate()`**，不实现 `Submit/Poll`。于是被包装后的 video/audio generator **丢失了 `AsyncGenerator` 接口**，worker 的断言 `routed.(generate.AsyncGenerator)` **永远 false** → 异步 generator 被误路由进阻塞的同步 `Generate` 路径（撞 CallTimeout、占满 worker 池）。`registerImageGenerators`（`main.go:289-321`）已 `reg.Register(..., obs.WrapGenerator(...))`，video/audio 注册照搬此范式（§8.1），故包装层必然在断言之前介入——这是 **必修**，否则 §5 整套异步引擎在生产装配下根本不被触发。

**修复（NEW，`obs/obs.go`）**：`WrapGenerator` 在包装时做一次类型断言，**当内层 generator 实现 `AsyncGenerator` 时，返回一个同样实现 `AsyncGenerator` 的包装器**（`Submit`/`Poll` 委托给内层并各起 span），否则返回原来的 `*tracedGenerator`：

```go
// NEW — obs/obs.go: 包装时保留异步接缝
func WrapGenerator(g generate.MediaGenerator, tp trace.TracerProvider) generate.MediaGenerator {
    if tp == nil {
        return g // 既有: nil tp 原样返回
    }
    base := &tracedGenerator{inner: g, tracer: tp.Tracer("llm-agent-studio/generate")}
    if ag, ok := g.(generate.AsyncGenerator); ok {
        // 内层是异步 generator → 返回同样实现 AsyncGenerator 的包装器,
        // 否则类型断言 routed.(AsyncGenerator) 在 worker 侧失败, 异步被误走同步。
        return &tracedAsyncGenerator{tracedGenerator: base, inner: ag}
    }
    return base // image: 仅 Kind()+Generate()
}

// tracedAsyncGenerator 内嵌 tracedGenerator(继承 Kind()+Generate()),
// 额外实现 Submit/Poll(委托内层 AsyncGenerator + span)。
type tracedAsyncGenerator struct {
    *tracedGenerator
    inner generate.AsyncGenerator
}
func (t *tracedAsyncGenerator) Submit(ctx context.Context, req generate.GenRequest, idempotencyKey string) (generate.SubmitResult, error) {
    ctx, span := t.tracer.Start(ctx, "studio.generate.submit."+t.inner.Kind())
    defer span.End()
    res, err := t.inner.Submit(ctx, req, idempotencyKey) // B1: 透传幂等键
    span.SetAttributes(
        attribute.String("studio.provider", res.Provider),
        attribute.String("studio.model", res.Model),
        attribute.String("studio.external_job_id", res.ExternalJobID),
        attribute.Int("studio.est_seconds", res.EstSeconds),
    )
    if err != nil { span.RecordError(err); span.SetStatus(codes.Error, err.Error()) }
    return res, err
}
func (t *tracedAsyncGenerator) Poll(ctx context.Context, jobID string) (generate.PollResult, error) {
    ctx, span := t.tracer.Start(ctx, "studio.generate.poll."+t.inner.Kind())
    defer span.End()
    res, err := t.inner.Poll(ctx, jobID)
    span.SetAttributes(attribute.String("studio.external_job_id", jobID), attribute.Int("studio.poll_status", int(res.Status)))
    if err != nil { span.RecordError(err); span.SetStatus(codes.Error, err.Error()) }
    return res, err
}
```

- **不变量**：`tracedGenerator.Generate`（`obs/obs.go:65-81`）零改；`tracedAsyncGenerator` 内嵌它继承 `Kind()/Generate()`，只新增 `Submit/Poll`。
- **断言成立链**：`reg.Resolve(provider, model)` 返回的是 `WrapGenerator` 的产物——对 video/audio 即 `*tracedAsyncGenerator`，`routed.(generate.AsyncGenerator)` **真**；对 image 即 `*tracedGenerator`，断言 **假** → 走同步路径。这正是 §5.2 分流所依赖的。
- 与 §9.2 otel span 要求一致：submit/poll 各自起 `studio.generate.submit.<kind>` / `studio.generate.poll.<kind>` span（§9.2 据此更新）。
- **命名回归测试（第二轮确认）**：`WrapGenerator(fakeAsync).(generate.AsyncGenerator)` 断言【必须成立】——这是 §5 异步引擎在生产装配下被触发的最小回归点（B1 一旦回退，此断言立刻失败）。该测试列入 §13 验证矩阵与 M4a 验证项。本修复（包装时保留异步接缝）经第二轮独立 codex 复核【确认 sound】。

### 4.3 AssetAgent 适配（最小改动）

`internal/agents/asset.go` 的 `RunWith` 当前直接 `gen.Generate(...)`。M4 **保留** `RunWith`（同步路径，image 仍走它），新增 `agents.AssetSubmit/AssetPoll`（薄包装，把 `AssetInput` → `GenRequest` 后调 `AsyncGenerator.Submit/Poll`），由 worker 在异步路径调用。AssetAgent 仍**无 I/O**（不碰 DB/blob，purity 不变）。

---

## 5. 异步长任务引擎（M4 核心）

> 这是评审会按得最狠的一节。下面把「new 列 / new 状态 / claim-SQL 改动 / cancel 交互 / CallTimeout 共存」逐一钉死。

### 5.1 当前 worker 的同步约束（为何不能直接跑分钟级）

`worker.go` 关键事实（逐一引用）：

- **claim 一次性租约，无续约**：`worker.go:182-186` `UPDATE ... locked_until = now() + make_interval(secs => $3)`，`$3=Lease`（默认 120s，`config.go:90`）。`locked_until` 只在 claim 时设一次，**全程不再延长**（`process`/`runAsset` 中无任何 renew）。
- **CallTimeout < Lease 不变量**：`worker.go:208-213` `dctx = WithTimeout(ctx, CallTimeout)`；`config.go:122-125` 强制 `WorkerCallTimeout < WorkerLease`（默认 90s < 120s）。注释（`worker.go:196-198`）明说：CallTimeout 保证「hung 调用不会活过租约被二次领取」。
- **stuck-reclaim**：`worker.go:167-168` claim 同时领「`status='running'` 且 `locked_until < now()` 的过期租约」。
- **失败退避重排**：`worker.go:606-612` `fail` 把 todo 置回 `status='ready'` + `next_run_at=now()+backoff`（`backoff = BaseBackoff << (attempts-1)`）。

**矛盾**：分钟级视频若走同步 `Generate`，单次 dispatch 会阻塞数分钟 → 撞 90s CallTimeout（被判失败）；即便放大 CallTimeout，单次 dispatch 长期占用一个 worker（饿死其他 todo），且一旦 > 120s 租约过期会被 stuck-reclaim 二次领取（重复烧钱）。**所以必须把长任务拆成 submit→poll 多次短 dispatch**，并为「短而多次」的轮询配租约续约兜底。

### 5.2 决策：submit→poll 作为 todo 状态机（主），租约续约作为兜底（次）

**(a) submit→poll 状态机（主机制）**——把一次长生成拆成多次**短** dispatch：

- 第一次 dispatch（`phase=submit`）：`runAsset` 检测 generator 是 `AsyncGenerator` → **先取/建本 todo 的唯一 asset 行**（`GetOrCreateForTodo`，§5.2c B1）→ **先派生确定性幂等键** `idemKey = hash(todoID)`（崩溃后二次 Submit 仍是同一 key）→ 调 `Submit(req, idemKey)` → 拿到 `ExternalJobID` → **在一个 DB 事务里**提交三件事：`SetSubmitted(external_job_id)`（asset `generating → submitted`，写 `submitted_at`，§6.2）+ generations 账本 **预登记 upsert**（估算成本，`ON CONFLICT(asset_id,todo_id)`，§6.3/§9.3）+ **todo** 用 `next_run_at = now() + PollBackoff` 重排回 `status='ready'`（`phase=poll`、`poll_attempts=0`、`attempts=0`，重排 SQL 见 §5.5）。这次 dispatch **秒级返回**，立即释放 worker。**为何单事务**：三者分散提交时，进程在 Submit 返回后、SetSubmitted 落库前崩溃 → 外部 job 已被 provider 接受但 `external_job_id` 丢失 → reclaim 二次 Submit → 孤儿计费 job + 丢结果。把 {SetSubmitted + upsert + reschedule} 原子化，确保不会停在「provider 已接受、本地无记录」的半态（与 §5.2c 的崩溃恢复走查配套）。
- 后续 dispatch（`phase=poll`）：`runAsset` 读 `external_job_id` → 调 `Poll(jobID)`：
  - `PollPending`（job 仍在跑，**非错误**）→ `poll_attempts++`；若 `< MaxPollAttempts` 则再次 `next_run_at` 退避重排（指数退避，封顶 `MaxPollBackoff`）；否则【poll 预算耗尽】= **终态失败**（见下「终态分支」）。
  - **transient Poll 错误**（`Poll` 返回 `err != nil`，如网络抖动/provider 5xx）→ **不调 `SetAsyncFailed`**（asset 仍留 `submitted`，外部 job 还在跑）→ 消耗 `poll_attempts` 预算后退避重排（同 Pending 路径）；只在 `poll_attempts` 预算耗尽时才转终态（见下）。这避免一次网络抖动就把还在跑的外部 job 误判终态。
  - `PollDone` → 拿 `GenResult`（URL 优先）→ **经 `internal/fetch` 拉回字节**（§9.4）→ `BlobStore.Put`（同一确定性 key，可重复覆盖——§6 幂等不变量）→ `Assets.SetBlob(... 'pending_acceptance')`（from 守卫含 `submitted`，§6.2）→ `cost.UpdateGenerationByAssetTodo`（回填真实秒数/成本，§6.3/§9.3）→ 发 `asset_generated` SSE。
  - **终态失败分支（IMPORTANT 2，第二轮）** = `PollFailed`（provider 明确报失败）**或** `poll_attempts >= MaxPollAttempts`（预算耗尽）：调 `SetAsyncFailed`（asset `submitted/generating → failed`，§5.4）**并把 todo 立即强制终态 `failed`**（`Todos.MarkFailed` + 阻断后继 + 可能触发 `run_done`，与 `worker.go:585-604` 的 attempts 耗尽分支同收口）——**不走** `fail` 的「`attempts < MaxAttempts` 则退避重排」分支。**为何不能复用 `fail`**：`fail`（`worker.go:580-613`）在 `attempts < MaxAttempts` 时把 todo 重排回 `ready` 继续重试，但 asset 已被 `SetAsyncFailed` 置 `failed` → 出现「asset=failed 但 todo 仍在轮询重试」的撕裂态。终态失败必须让 asset 与 todo **同时**落 `failed`，不经 attempts-based 退避。

**(a-bis) runAsset→process 重排契约（I1，**必修**）**：`process`（`worker.go:233`）在 dispatch 无错返回后 **无条件** `MarkDone`，而 `MarkDone` 守卫 `status='running'`（`todos/store.go:91-92`）。但 **submit 阶段** 与 **poll-pending 阶段** 的 `runAsset` 已把 todo 自行重排为 `status='ready'`（不是终态）——若此时 `process` 仍调 `MarkDone`，守卫不命中 → 返回 `done=false` → `process` 误以为「todo 被 cancel」，进而调 `discardCanceledAsset`（`worker.go:249`），**把一个健康的 submitted 资产推成 canceled**。这是引擎正确性致命缺陷，必须显式拆开「重排」与「完成」两种成功语义。

**契约（NEW）**：`runAsset` 返回值新增一个「已自重排」信号——选 **哨兵错误** `errRescheduled`（包内 `var errRescheduled = errors.New("worker: todo rescheduled")`）或 `(outputRef string, rescheduled bool, err error)` 三返回值，本文选哨兵：

- submit 成功后 runAsset 自重排 todo 为 ready(poll) → 返回 `("", errRescheduled)`。
- poll-pending 重排为 ready → 同样返回 `("", errRescheduled)`。
- poll-done（真正完成）→ 返回 `("asset:<id>", nil)`（走原 MarkDone 完成路径）。
- 真失败 → 返回 `("", realErr)`（走原 fail 路径）。

`process` 在 dispatch 后 **先判 `errRescheduled`**：

```go
// process(): NEW —— 在 perr != nil 之前拦截 errRescheduled
if errors.Is(perr, errRescheduled) {
    // runAsset 已把 todo 自行重排回 ready(poll); 本次 dispatch 是一次合法的
    // 中间步, 既非完成也非失败 —— 跳过 MarkDone / discardCanceledAsset /
    // emitNewlyReady / RefreshStatus / AppendRunDone, 直接释放 worker。
    span.AddEvent("async.rescheduled")
    return
}
if perr != nil { /* …既有 fail 路径… */ }
// 仅 poll-done / 同步 image 落到这里, 走既有 MarkDone 完成路径。
```

- **为何不能让 runAsset 自己 MarkDone**：MarkDone 同时解锁依赖（`todos/store.go:104-110`）+ process 后续 emitNewlyReady/RefreshStatus/run_done——这些只能在 **真正完成** 时发生；重排阶段触发它们会让下游 asset todo 提前 ready、项目状态误判完成。故重排路径必须 **完全跳过** 这一整段。
- **cancel 仍可达**：若 poll dispatch 时 todo 已被 `project.Cancel` 置 `canceled`（§5.4），runAsset 在重排 UPDATE（守卫 `status='running' AND locked_by=$worker`，见 §5.5）会 0 行命中 → runAsset 据此返回真错误（或检测到 0 行命中后返回非 `errRescheduled` 的取消信号），由 §5.4 的本地取消清理接管，**不** 误判为 errRescheduled。

**关键**：每次 poll dispatch 都很短（一次 HTTP 状态查询，远 < CallTimeout），所以 **CallTimeout/Lease 不变量原样成立，无需放大**。worker 池不被长任务占满——poll 之间 worker 自由处理别的 todo。

**(b) 租约续约 / heartbeat（兜底机制，闭合 M3 deferred gap）**——为「单次 dispatch 偶尔略长」（如 submit 阶段 provider 慢、或 poll 拉大文件）提供租约延长，避免误判 stuck：

- 新增 `Worker.renewLease(ctx, todoID)`：`UPDATE todos SET locked_until = now() + make_interval(secs => $lease) WHERE id=$1 AND locked_by=$2 AND status='running'`（**只续自己持有的、仍 running 的租约**——`locked_by` 守卫防止续约一个已被 cancel/reclaim 的行）。
- `process` 在 dispatch 前启一个 **心跳 goroutine**：每 `Lease/3` 调一次 `renewLease`，dispatch 返回时停。这让一次合法的长 dispatch（仍 < CallTimeout）不会因 `locked_until` 到期被另一 worker stuck-reclaim。
- **与 CallTimeout 的关系**：CallTimeout 仍是单次 dispatch 的硬上限（防真 hung）；续约只是在 `[0, CallTimeout]` 窗口内不断把 `locked_until` 推到未来，二者正交。**不变量更新**：父文档/M3 的「CallTimeout < Lease」可放宽为「续约周期 < Lease」，但 M4 **保守保留** `CallTimeout < Lease`（续约只是额外保险），以免引入新失败模式（遵循 CLAUDE.md 第2条「保留有用的边界检查」）。

> 为什么两者都要：状态机解决「分钟级不能塞进单次 dispatch」；续约解决「单次短 dispatch 也可能偶尔超租约」。状态机是主，续约是次。M3 注释（`worker.go:196-198`）已把「无租约续约」标为遗留——M4 在这里闭合。

#### 5.2c Submit 必须崩溃幂等，否则外部 provider 双提交（B1，第二轮，**必修**）

**问题（亲核）**：上面的 submit 序列若拆成「Submit → SetSubmitted → ledger → reschedule」四步分散提交，存在一个致命崩溃窗口：进程在 **provider 已接受 job（计费已起）但 `external_job_id` 尚未落库** 时崩溃 → todo 仍 `running`、`locked_until` 到期 → stuck-reclaim（`worker.go:167` claim 领过期 running todo）→ 二次进入 submit → **再调一次 Submit** → provider 侧产生第二个孤儿计费 job、第一个 job 的结果丢失。且 asset 行的创建（`createAsset`，`worker.go:428` 在 provider 之前建 `generating` 行）本身也非幂等——reclaim 会再建一行重复资产。三道防线一起上：

1. **确定性幂等键贯穿 Submit（provider 去重）**：
   - submit 前派生 `idemKey = hash(todoID)`（todoID 是稳定主键，崩溃前后不变；或落 `assets.async_request_id` 列持久化后读出，二者等价——本文取「由 todoID 派生」，零新列）。
   - 传入 `AsyncGenerator.Submit(ctx, req, idemKey)`（§4.2 签名已加该参）。
   - 真实适配器 **必须** 把 `idemKey` 转发为 provider 的 client-token / idempotency header（Runway/Kling/Veo 各家头名不同，适配器内映射；标 `// TODO(m5)` 的真实 HTTP 接线须保留此映射点）。fake 适配器 **回显** `idemKey`（供 §13 断言「同一 todo 二次 Submit 拿到同一 jobID」）。
   - 效果：即便本地没拦住二次 Submit，provider 凭同一 key 返回 **同一个 job**（而非新建），杜绝孤儿计费。

2. **asset 创建按 todo 幂等（本地去重）**：
   - 当前 `assets.Create`（`assets/store.go:106-126`）无脑 INSERT 新行，reclaim 会重复建资产。新增 **部分唯一索引** `CREATE UNIQUE INDEX IF NOT EXISTS assets_todo_uniq ON assets (todo_id) WHERE todo_id <> ''`（§6.2 m4Migrations）+ 新方法 `assets.GetOrCreateForTodo(ctx, in)`：先按 `todo_id` 查，命中则返回既有行（含其 `status`/`external_job_id`），未命中才 INSERT（靠唯一索引 + `ON CONFLICT (todo_id) WHERE todo_id <> '' DO NOTHING` + 回查兜住并发）。submit 阶段 runAsset 改用 `GetOrCreateForTodo` 而非 `createAsset`，reclaim 复用同一 asset 行。
     - **fan-out / regenerate 共存**：fan-out 的 asset todo `todo_id` 唯一 → 索引天然成立；regenerate 的 v2 asset 在 HTTP 时已建（`in.AssetID != ""`），仍走「填充既有行」分支（不经 GetOrCreateForTodo），其 `todo_id` 指向 regenerate todo——需确认 regenerate v2 asset 写入时也带正确 `todo_id`（否则唯一索引与 regenerate 冲突）；若 regenerate 既有逻辑令 v2 asset 的 `todo_id` 与原 asset 同值，则该部分索引需排除（impl 期核对 `review.Regenerate` 的 todo_id 赋值，必要时索引条件再收窄）。

3. **{SetSubmitted + ledger upsert + reschedule} 单事务（半态不可持久化）**：见上 submit 序列——三写在一个 `tx` 内提交，要么全成、要么全回滚，不存在「provider 已接受、本地无 external_job_id」的可持久化中间态。

**崩溃恢复走查（必须文档化）**：reclaim 二次进入 submit 阶段时——
- `GetOrCreateForTodo` 命中既有 asset 行；
- 若该 asset 已 `status='submitted'` 且 `external_job_id` 非空 → **跳过 Submit**，直接 **续轮询**（把 todo 重排为 `phase=poll`，不再调 Submit）；
- 若该 asset 仍 `generating`（半态被单事务回滚掉了，或首次 Submit 根本没成功）→ 重新 Submit，凭 `idemKey` provider 去重（拿回同一 job 或新建——provider 保证）。

这条走查与 §5.2 submit 序列、§6.2 `submitted` 态、§6.2 `assets_todo_uniq` 索引、§5.4 孤儿 reaper 联动闭合。

#### 5.2b heartbeat 作用域必须严格收口（I4，避免续约复活已重排的 poll todo）

心跳是 M3 deferred gap 的闭合，**保留**，但必须钉死作用域，否则它会把一个已被重排为 `ready` 的 poll todo 的 `locked_until` 续回未来，制造「ready 行却有活租约」的脏态。

**第二轮修正（二轮 I3「heartbeat 时序矛盾」，区别于一轮 I3「input_json.kind」）**：第一轮文本同时要求「心跳必须在 `runAsset` 内部重排 UPDATE **之前**停掉」与「心跳由 `process` 在 `runAsset` 返回后才停」——而重排发生在 `runAsset` **内部**，二者不可能同时成立（`process` 无法在它尚未拿回控制权的 `runAsset` 内部某一行之前停心跳）。第二轮 **取更简的 status-guard 方案**，放弃「重排前停」这条做不到的要求：

- **唯一约束改由 SQL 守卫承担**：重排 UPDATE 把 todo 一条语句改成 `status='ready'` 并同时清租约（`locked_by=''`, `locked_until=NULL`，§5.5 重排 SQL 已如此写）。`renewLease` 守卫 `WHERE id=$1 AND locked_by=$worker AND status='running'`——重排后该行已非 `running`（且 `locked_by` 已清空），**任何在重排之后到来的心跳一拍自然 0 行命中、no-op**。无需「在重排前精确停心跳」的不可达时序。
- **心跳作用域 = 整个 dispatch（含 runAsset）**：心跳 goroutine 由 `process` 在 dispatch 前启动、`defer`（cancel 专属 ctx + 等 goroutine 退出）在 `process` 拿回 `runAsset` 返回值后停掉。在「`runAsset` 内部已重排」到「`process` 停心跳」这一小段窗口里，即便心跳触发一次 `renewLease`，因双守卫不命中而 no-op——**不会** 复活已重排行的租约。这就是「靠 status-guard 而非靠时序」。
- **renewLease 双守卫（保留）**：`WHERE id=$1 AND locked_by=$worker AND status='running'`——`locked_by` 防止续约一个已被 reclaimer 重新领取（locked_by 已变）的行（消除与 stuck-reclaim 的 double-claim 窗口）；`status='running'` 防止续约一个已重排/已 cancel 的行。这两个守卫正是上面「重排后心跳自动 no-op」成立的依据。
- **落点**：心跳在 `process`（`worker.go:199-261`）内围绕 dispatch 段（`worker.go:208-225`）启停，`runAsset` 不感知心跳；`process` 的心跳 stop（`defer`）在 `runAsset` 返回后、判 `errRescheduled` 之前执行。**自洽性**：本节不再声称「重排前停心跳」，§5.2 submit/poll 重排路径也不需感知心跳——重排清租约 + renewLease 双守卫两者已闭合该竞态。

### 5.3 并发隔离：claim SQL 从全局 asset 上限改为按 kind 子上限

当前 claim 子查询（`worker.go:165-174`，逐字）：

```sql
SELECT id, project_id, type, attempts, input_json FROM todos
WHERE ((status='ready' AND next_run_at <= now())
   OR (status='running' AND locked_until IS NOT NULL AND locked_until < now()))
  AND (type <> 'asset' OR $1 <= 0
       OR (SELECT count(*) FROM todos
           WHERE type='asset' AND status='running' AND locked_until > now()) < $1)
ORDER BY next_run_at ASC
FOR UPDATE SKIP LOCKED
LIMIT 1
```

`$1 = MaxConcurrentGen`（全局 asset 并发软上限，注释已说明这是 READ COMMITTED 下可瞬时超 `Workers-1` 的**软**上限）。

**M4 改动（NEW）**：asset todo 需要按 **kind** 分别限流——分钟级 video 不该挤占 image 的并发额度。两种落地，本文选 **(B)**：

- (A) 在 todos 表加 `kind` 列，子查询按 `kind` 分组计数。**否决**：kind 在 fan-out 时才由 storyboard/路由决定，写一份冗余 kind 列增加一致性负担。
- **(B) 选定**：claim SQL 增加一个 **按 kind 的子上限** 参数，kind 经 `asset.type` 或 `input_json->>'kind'` 取得（asset todo 的 kind 在 fan-out 时已写入 `input_json`，§6.2）。子查询改为：

```sql
-- NEW: $1=MaxConcurrentGen(全局软上限, 保留), $2=MaxConcurrentVideo(video 子上限)
AND (type <> 'asset' OR $1 <= 0
     OR (SELECT count(*) FROM todos
         WHERE type='asset' AND status='running' AND locked_until > now()) < $1)
AND (type <> 'asset' OR (input_json->>'kind') IS DISTINCT FROM 'video' OR $2 <= 0
     OR (SELECT count(*) FROM todos
         WHERE type='asset' AND status='running' AND locked_until > now()
           AND (input_json->>'kind')='video') < $2)
```

**第二轮关键纠正（二轮 B2「并发上限不限外部在途」，区别于一轮 B2「submitted 搁浅」，**必修**）：上面这个基于 `todos.status='running'` 的计数只是【fetch/dispatch 上限】，绝不等于【外部在途 job 上限】。** submit 成功后 todo 立刻被重排为 `ready`（poll 阶段，§5.2），其间外部 job 在 provider 侧跑分钟级——而 `status='running'` 只在 submit/poll 那一瞬持续毫秒级。于是用上面的 SQL 限 `MaxConcurrentVideo` 只能限制「同时在本地发 HTTP 的瞬间数」，**根本不限制外部在途 job 数**：100 个 video 可以在几秒内全部 submit 出去（每个 submit 几毫秒，running 计数永远很小），外部并发计费失控。**必须区分两个语义不同的上限**：

- **(1) 提交准入上限（submit-admission cap，B2 真正要的）= 限外部在途 job**：按 **持久的在途资产** 计数——`assets.status='submitted' AND type=$kind`（+ 当前正在 submit 中的）。**submit 阶段的 claim/dispatch 在该计数 ≥ `MaxConcurrent<Kind>` 时，拒绝开始一个新的 submit**（不发起新的 Submit 调用）。因为 `submitted` 态横跨整个外部 job 生命周期（submit 落库 → poll-done 才离开），这个计数才真实反映「外部在途」。
  - **CRITICAL —— poll dispatch 永远可领，不受 submit 准入上限约束**：准入上限【只】作用于 **submit 这一转移**，**不**作用于 poll 重领。否则当 `submitted` 计数打满时，所有 poll dispatch 也被挡 → 已完成的外部 job 永远无法被 poll 拉回 → `submitted` 计数永不下降 → **死锁**。claim 必须能区分「这是一次 submit（asset 尚未 submitted，受准入上限）」与「这是一次 poll 重领（asset 已 submitted，无条件可领）」。落地：claim SQL 的 kind 子上限子句 **只在 asset 还没有对应 `submitted` 资产时** 才施加（即排除掉「该 todo 已有 submitted 资产 → 这是 poll 重领」的行），或在 runAsset 进入 submit 分支前用 `Assets.CountInFlightByKind(kind)` 做一次准入检查、poll 分支跳过该检查。本文取后者（更清晰）：准入检查放在 runAsset 的 **submit 分支入口**（poll 分支不查），计数源 `SELECT count(*) FROM assets WHERE status='submitted' AND type=$kind`；超限则 runAsset 不 Submit、把 todo 退避重排回 `ready`（`errRescheduled`，等额度释放后再试），**不**计 `attempts`。
- **(2) fetch/dispatch 上限（原 claim-SQL 软上限，保留为独立第二道）= 限本地并发拉取**：上面基于 `todos.status='running' AND locked_until>now()` 的按 kind 子查询 **保留**，但语义重定位为「同时在本地占 worker 做 submit 或 poll-done 大文件拉回的瞬间并发」——主要服务 §9.4 的 OOM 天花板（限并发大文件拉回数）。它是 READ COMMITTED 下可瞬时超 `Workers-1` 的 **软** 上限（评审已接受 `worker.go:157-164` 的软上限论证）。
- **两个上限命名清晰区分**：`MaxConcurrentVideo`/`MaxConcurrentAudio`（§5.6）**既** 作为 submit-admission cap 的阈值（限在途 job），**也** 复用为 fetch cap 的阈值——同一旋钮、两处执行点（submit 入口的资产计数 + claim SQL 的 running 计数）；若运维需把「在途 job 上限」与「本地拉取并发」拆成不同值，可后续加独立 `MaxFetch<Kind>` 旋钮（M4 不拆，YAGNI，但文档化二者是不同执行点以便将来拆分）。0=不限（两处皆然）。
- **新方法**：`assets.CountInFlightByKind(ctx, kind string) (int, error)` = `SELECT count(*) FROM assets WHERE status='submitted' AND type=$1`——submit 准入计数源（部分依赖 `assets_status_idx`，`storage.go` 已有 status 索引，足够）。

**前置：`input_json->>'kind'` 今天根本没写（I3，**必修**）**：当前 storyboard fan-out（`worker.go:366-369`）写入的 `input_json` 只有 `{shotId, shotPrompt, style}`——**没有 `kind`**。`input_json->>'kind'` 取出 NULL，上面按 kind 子上限子查询 `IS DISTINCT FROM 'video'` 对 NULL 恒为 true（被排除出 video 计数），**子上限永不生效**。修复要求 fan-out **和** regenerate 路径都把 `kind` 写进 asset todo 的 `input_json`：

- **kind 来源（单一真相）**：与 `DefaultForOrg(kind)` 路由（§6.4）**同源**——fan-out 时由 storyboard 决策 / 项目 `content_type` 意图解析出每个 shot 的 kind（image/video/audio）。fan-out 处（`worker.go:366-369`）的 marshal 从 `map[string]string` 扩为含 `"kind"`：
  ```go
  // worker.go:366 fan-out, NEW: 写 kind + duration(媒体时长)
  input, _ := json.Marshal(map[string]any{
      "shotId": shotID, "shotPrompt": sh.Prompt, "style": projectStyle,
      "kind":     kind,          // image|video|audio, 与 DefaultForOrg(kind) 路由同源
      "duration": sh.Duration,   // 来自 shots.duration(storage.go:107, INT DEFAULT 0); 喂 EstSeconds/DurationSeconds
  })
  ```
- **DurationSeconds/EstSeconds 来源**：`shots.duration`（`storage.go:107` `duration INT NOT NULL DEFAULT 0`，StoryboardAgent 产出，已写入 shots 行 `worker.go:360`）→ 落 asset todo input 的 `duration` → runAsset 构造 `GenRequest.DurationSeconds`（§4.2 加性字段）→ Submit 时作为 `SubmitResult.EstSeconds`（§9.3 预登记成本估算）。这是「秒计费」与「估算预登记」唯一的时长真相。
- **regenerate 路径**：HITL regenerate 的 asset todo（`AddSingleReady`，`todos/store.go:187`）的 `input_json` 同样必须带 `kind`/`duration`，从被重生资产的 `assets.type` + 其 shot 的 `duration` 取——否则 regenerate 出的 video 既不被子上限计数、也拿不到时长计费。
- **不过度设计（YAGNI）**：claim 子查询的 kind 计数只在 **极小的 running 集**（`status='running' AND locked_until > now()`，并发上限量级 ≤ Workers）上跑 `(input_json->>'kind')='video'`，不需要给 `input_json->>'kind'` 建 JSONB 表达式索引——running 集太小，全扫即可。M4 **不** 加该索引（CLAUDE.md 第2条）。

### 5.4 cancel 交互

当前 `project.Cancel`（`project/store.go:168-185`）：把所有非终态 todo 置 `canceled` + 把 `status='generating'` 的 asset 扫成 `canceled`。`worker.discardCanceledAsset`（`worker.go:643-658`）在 `MarkDone` 发现 todo 已非 running 时，把 asset 从 `pending_acceptance`/`generating` 推到 `canceled`。

**M4 增量 —— `submitted` 态的每一条失败/取消转移都必须显式枚举（B2，**必修**）**

新中间态 `submitted`（§6.2）引入了一个隐患：**所有把 asset 推向终态的写都用 `SetBlob`，而 `SetBlob` 守卫死在 `WHERE status='generating'`（`assets/store.go:143`）**。这意味着 §6.2 happy-path「把 SetBlob from 守卫放宽到 `generating OR submitted`」**单独不够**——异步路径下 asset 在 `submitted` 态时遭遇的 **每一条失败/取消** 路径，若仍走只认 `generating` 的旧守卫，都会 **静默 no-op，把资产搁浅在 `submitted`**。逐一枚举今天会命中 submitted 资产、但守卫只认 generating 的写点：

| 失败/取消点 | 代码位置 | 当前守卫 | 在 submitted 态会怎样 |
|---|---|---|---|
| 配额 backstop 兜底（regenerate v2） | `worker.go:421` `SetBlob(...,"failed")` | `status='generating'` | submit 后再被配额拦截时 no-op → 搁浅 submitted |
| generator/poll 错误 | `worker.go:474` `SetBlob(...,"failed")` | `status='generating'` | poll 报错时 no-op → 搁浅 submitted |
| blob put 失败 | `worker.go:482` `SetBlob(...,"failed")` | `status='generating'` | poll-done 拉回后落 blob 失败时 no-op → 搁浅 submitted |
| poll-done 成功落地 | `worker.go:492` `SetBlob(...,"pending_acceptance")` | `status='generating'` | 正常完成 no-op → 搁浅 submitted（§6.2 happy-path 修的就是这条） |
| 项目取消扫描 | `project/store.go:182` | `status='generating'` | cancel 时 no-op → 搁浅 submitted |
| 取消中途丢弃 | `worker.go:648` discardCanceledAsset from-list | 无 `submitted` | 取消 race 时 no-op → 搁浅 submitted |

**修复（NEW，二选一，本文选 (1)）**：

1. **新增显式 `SetAsyncFailed(id)` 方法** + 放宽成功守卫：
   - `assets.SetAsyncFailed(ctx, id)`：`UPDATE assets SET status='failed' WHERE id=$1 AND status IN ('generating','submitted')`——异步路径的 **所有失败转移**（`worker.go:421`/`:474`/`:482` 在异步 kind 下）改调它；image 同步路径仍用旧 `SetBlob(...,"failed")`（from=generating），零改。
   - 成功转移：`SetBlob` 的 from 守卫放宽为 `status IN ('generating','submitted')`（§6.2），承载 `worker.go:492` 的 poll-done 完成。
2. （备选）直接把 `SetBlob` 的守卫整体放宽为 `status IN ('generating','submitted')`，并 **审计上表每一个 caller** 确认放宽后语义正确——更省方法但守卫语义变宽，需逐点复核。本文选 (1)，把异步失败语义独立成名，避免悄悄拓宽 image 路径的成功守卫。

**取消相关两条必改（与上表对应）**：

- `project.Cancel` 的 asset 扫描 **覆盖 submitted**：`UPDATE assets SET status='canceled' WHERE project_id=$1 AND status IN ('generating','submitted')`（`project/store.go:182`，`submitted`=已提交外部 job 但未完成）。
- `discardCanceledAsset`（`worker.go:643-648`）的 from 列表加 `submitted`：`for _, from := range []string{"pending_acceptance", "submitted", "generating"}`。

**本地取消（Q4 在 M4 必须解决的部分）**：

- cancel 后 worker 在下一次 poll dispatch **检测到 todo 已 `canceled`**（runAsset 的重排/读取 UPDATE 0 行命中，或显式查 todo 状态）→ **停止轮询**，不再重排，让 §5.4 上面的 submitted→canceled 扫描收口资产。这条本地取消语义（终态化 submitted 资产 + poll dispatch 检测到 canceled todo 即停）**必须在 M4 落地**（与 B2 的搁浅风险绑定），由 M4c 验证（§12）。
- **外部 job 取消（Q4 延后部分）**：真正调 `AsyncGenerator.Cancel(jobID)` 向 provider 发 HTTP 取消 **延后 M5**——M4 把 `Cancel` 作为 `AsyncGenerator` 的 **可选方法**，接口里给一个 no-op 默认（provider 不支持则 no-op）。**已提交的外部 job 可能仍在 provider 侧跑完并计费**——与 M3 已接受的「cancel 与 SetBlob race 时钱已花」语义一致（`worker.go:243-248`），文档化为已知窗口，不追求强一致。

**孤儿清理（M1）**：`submitted` 态有 provider 侧不返回（job 永久卡住）导致资产永久搁浅的风险，需一个 **孤儿 reaper**：周期性把 `submitted` 且 `submitted_at` 早于 TTL（如 `> 2 × MaxPollAttempts × MaxPollBackoff`）的资产终态化为 `failed`（理由：B2 搁浅风险 + `submitted_at` 列正好为此而设，§6.2）。本文把 reaper 列为 **M4 的一个小任务**（M4c，与本地取消同批），让 `submitted_at` 落到实处而非悬空。

### 5.5 CallTimeout 与多次 poll dispatch 共存

- 每次 poll/submit dispatch 仍受 `CallTimeout`（`worker.go:208-213`）约束——单次操作是一个 HTTP 调用，远短于 90s。
- **失败清理须活过 CallTimeout 到期**：沿用 M3 的 `cctx := context.WithoutCancel(ctx)`（`worker.go:446`）——poll 完成后写 asset 终态/账本（`SetBlob` / `SetAsyncFailed` / `UpdateGenerationByAssetTodo`）/SSE 都用 `cctx`，避免 CallTimeout 触发后静默 no-op（`worker.go:489-494` 的同款修复）。submit 阶段的预登记 upsert（§9.3）同样用 `cctx`。
- `MaxPollAttempts` × `PollBackoff` 给长任务一个 **总轮询预算**（如 60 次 × 退避封顶 30s ≈ 覆盖 30 分钟级视频）。**超预算 = 终态失败（IMPORTANT 2，第二轮）**：不走 `fail` 的 attempts-based 退避分支，而是直接 `SetAsyncFailed`（asset 终态 `failed`）+ `Todos.MarkFailed`（todo 立即终态 `failed` + 阻断后继 + 可能触发 `run_done`，与 `worker.go:585-604` 的 attempts 耗尽收口 **复用同一终态化代码**，但触发条件是 `poll_attempts >= MaxPollAttempts` 而非 `attempts >= MaxAttempts`）。**关键区别**：`poll_attempts` 耗尽 → asset 与 todo **同时** 落 `failed`（终态分支，§5.2）；普通 `attempts` 耗尽（claim 层面，如 submit 反复报错）仍走 `fail`。两者都终态，但 poll-budget 路径必须主动 `SetAsyncFailed` 以避免资产搁浅 `submitted`（B2）。

> **poll_attempts 与 attempts 的区别（关键，避免混淆）**：`todos.attempts`（`worker.go:181`）是 **claim 次数**，每次 dispatch +1，受 `MaxAttempts=3` 约束——但 submit→poll 会产生很多次合法 dispatch，**不能**让正常轮询耗尽 `attempts`。故 M4 **新增独立的 `poll_attempts` 列**，submit→poll 的重排走「成功重排」路径（不增 `attempts`、不进 `fail`），只有 provider 报错/poll 预算耗尽才进 `fail`（增 `attempts`）。这是 §6.2 新列的核心理由。

**致命细节（I6，**必修**）：`claim` 无条件 `c.attempts++`（`worker.go:181`）会反过来吃掉 poll_attempts 的隔离**。`claim()` 每次领取都 `c.attempts++` 再 `attempts=$4` 写回（`worker.go:181-186`），而 submit→poll 的「成功重排」把 todo 置回 `ready`，**下一次 claim 又会把 attempts +1**。于是哪怕 poll 走的是「成功重排」不进 fail，正常的第 3 次 poll claim 就把 `attempts` 顶到 `MaxAttempts=3`——`poll_attempts` 的预算（§5.6 默认 60）还没用就被 `attempts` 误杀。**修复**：submit→poll / poll-continue 的「成功重排」UPDATE **必须在同一条 SQL 里把 `attempts` 重置为 0**（`poll_attempts` 才是这条长任务的真实预算载体）：

```sql
-- 成功重排(submit→poll 或 poll-pending→poll)的 UPDATE, NEW: attempts 归零 + poll_attempts 递增
UPDATE todos
SET status='ready', next_run_at=$2, attempts=0, poll_attempts=$3,
    locked_by='', locked_until=NULL, updated_at=now()
WHERE id=$1 AND locked_by=$worker AND status='running'
```

- `attempts=0` 让下一次 claim 的 `attempts++` 从 1 起算，正常轮询永不撞 `MaxAttempts`；只有 **真失败重排**（走 `worker.go:fail`）才保留 `attempts` 递增语义（poll 预算耗尽 / provider 报错的退避仍受 MaxAttempts 约束）。
- **明确**：不重置 attempts，则 `poll_attempts/attempts` 的分离形同虚设——这是引擎正确性不变量，必须有针对性测试（§11 R9）。
- 重排 UPDATE 的 `WHERE ... locked_by=$worker AND status='running'` 守卫同时承担 §5.2(a-bis) 的「cancel 检测」（被 cancel 的 todo 已非 running → 0 行命中 → runAsset 不返回 errRescheduled，转本地取消清理）。

### 5.6 新增 worker / config 旋钮（照搬 `config.go` durOf/intOf 严格解析）

`internal/config/config.go` 新增（沿用 `config.go:140-157` 的 `durOf`/`intOf` 严格解析，错误命名 env key）：

```go
PollBackoff        time.Duration // 异步轮询基础退避, 默认 5s   (env POLL_BACKOFF)
MaxPollBackoff     time.Duration // 轮询退避封顶, 默认 30s      (env MAX_POLL_BACKOFF)
MaxPollAttempts    int           // 单 asset 轮询次数预算, 默认 60 (env MAX_POLL_ATTEMPTS)
MaxConcurrentVideo int           // video asset-todo 子上限, 0=不限 (env MAX_CONCURRENT_VIDEO)
MaxConcurrentAudio int           // audio asset-todo 子上限, 0=不限 (env MAX_CONCURRENT_AUDIO)
```

`worker.Config` 对应新增 `PollBackoff/MaxPollBackoff/MaxPollAttempts/MaxConcurrentVideo/MaxConcurrentAudio`，在 `cmd/studiod/main.go` 的 worker 装配（`main.go:172-185`）处接线。

---

## 6. 数据模型（m4Migrations）

### 6.1 asset 类型 video/audio——无需迁移

`assets.type` 已是自由 TEXT（`storage.go:131` `type TEXT NOT NULL DEFAULT 'image'`，无 CHECK 白名单；`assets/store.go:111` 仅在空时默认 `'image'`）。`video`/`audio` 直接可写，**无 schema 改动**。库检索 `LibraryFilter.Type`（`assets/store.go:227`）已支持任意 type 过滤。

### 6.2 todos / assets 新列（m4Migrations，全部 idempotent，不破坏性 ALTER）

新增 `internal/storage/storage.go` 的 `m4Migrations` slice（与 `m1/m2/m3Migrations` 同模式，`storage.go:42/125/182`；`Migrate` 的 `all` 拼接加 `m4Migrations`，`storage.go:205`）：

```sql
-- todos: 异步轮询状态(§5.2/§5.5)
ALTER TABLE todos  ADD COLUMN IF NOT EXISTS poll_attempts INT NOT NULL DEFAULT 0;
-- assets: 外部 job 句柄 + 提交时间(可观测/孤儿清理)
ALTER TABLE assets ADD COLUMN IF NOT EXISTS external_job_id TEXT NOT NULL DEFAULT '';
ALTER TABLE assets ADD COLUMN IF NOT EXISTS submitted_at    TIMESTAMPTZ;          -- NULL=未提交
-- 索引: 按 external_job_id 反查(取消/对账), 部分索引避免空串膨胀
CREATE INDEX IF NOT EXISTS assets_extjob_idx ON assets (external_job_id) WHERE external_job_id <> '';
-- B1(第二轮): 每个 todo 至多一行 asset(崩溃 reclaim 复用同一行, 不重复建), 部分唯一索引避免空 todo_id 冲突
CREATE UNIQUE INDEX IF NOT EXISTS assets_todo_uniq ON assets (todo_id) WHERE todo_id <> '';
-- B3(第二轮): generations 异步账本去重升为 DB 不变量(submit-insert/poll-update 同一行),
-- 配合 UpsertSubmittedGeneration 的 ON CONFLICT(asset_id,todo_id); 部分索引排除空键(image 同步路径仍可空)
CREATE UNIQUE INDEX IF NOT EXISTS generations_asset_todo_uniq ON generations (asset_id, todo_id)
  WHERE asset_id <> '' AND todo_id <> '';
```

- **新 asset 中间态 `submitted`**：status 取值集扩为 `{generating, submitted, pending_acceptance, accepted, rejected, failed, canceled}`。`submitted` 介于「已建行」与「拉回完成」之间，对应外部 job 进行中。`assets/store.go` 新增方法：
  - `SetSubmitted(id, jobID)`：`generating → submitted`，写 `external_job_id` + `submitted_at=now()`。
  - **`GetOrCreateForTodo(in CreateInput) (Asset, error)`**（B1，第二轮）：按 `todo_id` 取既有 asset 行，未命中才插入（靠 `assets_todo_uniq` 部分唯一索引 + `ON CONFLICT (todo_id) WHERE todo_id <> '' DO NOTHING` + 回查兜并发）。submit 阶段 runAsset 改用它替代无脑 INSERT 的 `createAsset`（`assets/store.go:106-126` / `worker.go:428`），令崩溃 reclaim 复用同一行、不重复建资产。
  - **`CountInFlightByKind(kind string) (int, error)`**（B2 submit-admission，第二轮）：`SELECT count(*) FROM assets WHERE status='submitted' AND type=$1`——submit 准入上限的计数源（限外部在途 job，§5.3）。
  - **`SetAsyncFailed(id)`**：`UPDATE ... SET status='failed' WHERE id=$1 AND status IN ('generating','submitted')`——异步路径的所有失败转移走它（B2，§5.4），覆盖 submit 后才发生的失败。
  - 放宽 `SetBlob` 的 **成功** from 守卫（当前 `WHERE ... status='generating'`，`assets/store.go:143`）为 `status IN ('generating','submitted')`，承载 poll-done 完成（from=submitted）。**注意**：单放宽这条成功守卫 **不足以** 解决搁浅——§5.4 已枚举所有失败/取消转移必须各自处理（`SetAsyncFailed` + Cancel 扫描 + discard from-list），happy-path widening 只修「正常完成」一条。
- **`Blob.Put` 对同一确定性 key 必须可重复（幂等不变量，第二轮 minor）**：poll-done 落 blob 用确定性 key `assets/<project>/<asset>`（`worker.go:480` 既有形态，与 asset id 绑定，崩溃前后不变）。崩溃后 poll-done 重试会以 **同一 key** 再次 `Blob.Put` 同一字节——`Blob.Put` 必须是 **覆盖写（overwrite）语义**，重复 put 同 key 幂等、不报错、不产生第二份对象（MinIO/S3 PUT 天然覆盖；本不变量是对该行为的显式依赖声明）。这与 §6.3 generations upsert 的幂等、§5.2c Submit 的幂等共同构成「整条异步落地链崩溃可重放」的闭环。
- **kind 不入 todos 表列**：asset todo 的 kind 走 `input_json->>'kind'`（fan-out **与 regenerate** 时由 storyboard/路由写入，§5.3 claim SQL 直接读 JSONB），避免冗余列。**前提**：fan-out 当前根本不写 kind（`worker.go:366-369`），M4 必须补写（I3，§5.3）。
- `poll_attempts` 独立于 `attempts`（理由见 §5.5），且「成功重排」UPDATE 必须 `attempts=0`（I6，§5.5）。
- **`submitted_at` 的用途（M1）**：供 **孤儿 reaper** 判定——周期性把 `submitted` 且 `submitted_at` 早于 TTL 的资产终态化 `failed`（§5.4），防 provider 永不返回导致的永久搁浅。reaper 列为 M4c 小任务，使 `submitted_at` 落到实处（不悬空）。

### 6.3 计费：pricing 加 `micros_per_second`，按秒计

`generations.video_seconds`（`storage.go:158`）**已存在**（`INT DEFAULT 0`）。`cost.Generation.VideoSeconds`（`cost/store.go:29`）也已存在但当前未填。M4：

```sql
-- m4Migrations
ALTER TABLE pricing ADD COLUMN IF NOT EXISTS micros_per_second BIGINT NOT NULL DEFAULT 0;
-- 视频/音频 fake + 真实型号种价(占位, ops 可 SQL 调; fake 价用于活验账本断言)
INSERT INTO pricing (provider, model, kind, micros_per_second) VALUES
  ('fake',    'fake-video-async', 'video', 500000),   -- 0.5 USD/s 量级占位
  ('fake',    'fake-audio-async', 'audio',  50000),
  ('runway',  'gen-3',            'video', 500000),
  ('kling',   'kling-v1',         'video', 280000),
  ('google',  'veo-2',            'video', 500000),
  ('openai',  'tts-1',            'audio',  15000)
ON CONFLICT (provider, model) DO NOTHING;
```

- **音频时长（Q3，RESOLVED）**：复用 `generations.video_seconds` 列承载「媒体秒数」（视频帧时长 / 音频时长），**不加** `audio_seconds` 列（YAGNI——一个 asset 要么 video 要么 audio，一列足够）。SQL 列名 `video_seconds` 保持不变（历史命名，迁移成本无意义）；仅把 Go 侧字段语义注释调整为「媒体时长（秒）」——`cost.Generation.VideoSeconds`（`cost/store.go:29`）的注释改为 `// MediaSeconds: 媒体时长(秒), video=帧时长 audio=音频时长`。字段名是否改成 `MediaSeconds` 由 impl 决定（改名需同步所有 caller），最低要求是注释澄清语义。
- `cost.ComputeCostMicros`（`cost/store.go:124`）扩展为加上 `int64(seconds)*p.MicrosPerSecond`：
  ```go
  func ComputeCostMicros(p Price, imageCount, tokens, seconds int) int64 {
      return int64(imageCount)*p.MicrosPerImage + int64(tokens)*p.MicrosPer1kTokens/1000 + int64(seconds)*p.MicrosPerSecond
  }
  ```
  `cost.Price` 加 `MicrosPerSecond int64`；`PriceFor`（`cost/store.go:109`）SELECT 增列。
- **异步账本 = submit 预登记 + poll-done 回填（I2+I5，**必修**，见 §9.3；B3 第二轮升为 DB 不变量）**：异步路径下 `generations` 行 **在 submit 阶段就 upsert 一行**（估算成本 = `EstSeconds × micros_per_second`），poll-done 时 **UPDATE 同一行**（回填真实 `video_seconds`/`cost_micros`），**绝不二次插入**。第二轮把「`asset_id+todo_id` 去重」从【prose 约定】升为【DB 不变量】（§6.2 新增 `generations_asset_todo_uniq` 部分唯一索引），并据此规定两个新方法（**不**依赖 read-before-insert）：
  - **`UpsertSubmittedGeneration(ctx, g Generation) (id string, err error)`**（submit-insert）：
    ```sql
    INSERT INTO generations (id, project_id, asset_id, todo_id, kind, provider, model, prompt, video_seconds, cost_micros, ...)
    VALUES ($1, ..., $estSeconds, $estCostMicros, ...)
    ON CONFLICT (asset_id, todo_id) WHERE asset_id <> '' AND todo_id <> ''
      DO UPDATE SET id = generations.id   -- no-op 更新, 仅为 RETURNING 既有行 id
    RETURNING id
    ```
    崩溃后二次 submit 命中冲突 → 返回既有行 id、不新建 → 杜绝 I5 双插。这【就是】§6.3/§9.3 所说的「submit-insert」（同一机制，不是两套）。
  - **`UpdateGenerationByAssetTodo(ctx, assetID, todoID string, seconds int, costMicros int64) error`**（poll-done 回填）：`UPDATE generations SET video_seconds=$3, cost_micros=$4 WHERE asset_id=$1 AND todo_id=$2`——天然幂等（重复 UPDATE 同值），无需持有行 id。runAsset 用 `asset_id+todo_id` 定位，不在内存透传 id。
  - **image 同步路径零改**：仍「一次完成」直接 `RecordPriced`（`cost/store.go:132`，单次 insert），`Kind:"image"`、`VideoSeconds=0`。新唯一索引对 image 行同样部分覆盖（image 的 asset_id/todo_id 也非空），但 image 每 todo 只 insert 一次、无并发二插，索引对其无害（最坏一次 reclaim 重复 insert 被索引挡成约束错误——这与 image 同步路径既有「单 todo 单资产」语义一致，属安全收紧，非回归）。异步 video/audio 才走「Upsert / UpdateByAssetTodo」。
  - **幂等性（I5 闭合）**：崩溃重试落在 submit→poll 之间时，submit 的 `UpsertSubmittedGeneration` 凭 `generations_asset_todo_uniq` 命中冲突 → 不重复计费；poll-done 的 `UpdateGenerationByAssetTodo` 天然幂等。**这是 DB 强制的去重，不再是 read-before-insert 的竞态 prose。**
- `generations` 账本聚合（`cost/store.go` 的 `ByOrg`/`PerProjectByOrg`/`RecentByOrg`）**无需改 SQL**——已 `sum(cost_micros)`，按秒成本已并入 `cost_micros`；submit 时预登记的估算成本先入账、poll-done 时被真实值覆盖（聚合读到的始终是当前行的最新值）。UI 成本中心的「视频生成次数」可由 `kind` 维度聚合（PRD 模块13），属 UI 侧查询，非本 spec。

### 6.4 model_configs / catalog

`models.DefaultForOrg(ctx, orgID, kind)`（`models/store.go:180`）**已按 kind 参数化**——但 worker 当前**硬编码** `"image"`（`worker.go:458` `DefaultForOrg(ctx, orgID, "image")`）。M4：worker 在 fan-out 时已知 asset 的 kind（来自 `input_json->>'kind'`，§5.3 I3 写入，与路由同源），按该 kind 调 `DefaultForOrg(ctx, orgID, kind)`，让 video/audio 走各自的 org 默认 model_config。`models.Catalog()`（`models/store.go:68`）追加 video/audio 条目（fake + 真实型号），`kind` 字段已存在。

**catalog 按 kind 过滤（M3）**：`registerImageGenerators`（`main.go:289-321`）当前 **遍历整个 `Catalog()`** 并 `switch e.Provider`，对未知 provider 走 `default: continue`（`main.go:314-315`）。一旦 catalog 含 video/audio 条目，image 注册器会把 video provider（如 runway/kling）也喂进这个 switch——今天靠 `default:continue` **无害**（runway 不在 image 的 case 列表里，被跳过），但语义脏：image 注册器不该看见 video 条目。M4 指定 **清晰的按 kind 过滤**——三个注册器各自只处理本 kind 的 catalog 条目：
```go
// registerImageGenerators / registerVideoGenerators / registerAudioGenerators
for _, e := range models.Catalog() {
    if e.Kind != "image" { continue }  // video/audio 注册器同理过滤各自 kind
    // …既有 switch e.Provider…
}
```
这样 image 注册器只迭代 `kind=="image"`、video 只迭代 `kind=="video"`、audio 只迭代 `kind=="audio"`，互不串扰（依赖 `models.Catalog()` 条目的 `Kind` 字段，已存在）。

---

## 7. fake 异步 generator（sandbox 活验的关键）

新增 `internal/generate/fake_async.go`（与现有 `internal/generate/fake.go` 同包，`fake.go:10` 的 image fake 是范本）：

```go
// NEW — FakeAsync 是 video/audio 的确定性异步 generator: 零网络, 确定性
// submit→poll, 可控「需几次 poll 才 Done」, 供 sandbox 活验异步引擎 + 接缝。
type FakeAsync struct {
    mu          sync.Mutex
    kind        string                 // "video" | "audio"
    pollsToDone int                    // 需轮询几次才返回 PollDone (确定性)
    jobs        map[string]int         // jobID → 已 poll 次数
    result      GenResult              // Done 时返回(URL 或 Bytes; 活验可指向 httptest 服务)
}
func NewFakeAsync(kind string, pollsToDone int, result GenResult) *FakeAsync { ... }
func (f *FakeAsync) Kind() string { return f.kind }
func (f *FakeAsync) Submit(ctx, req, idemKey) (SubmitResult, error)  // 生成确定性 jobID(由 idemKey 派生→同 key 二次 submit 返同 jobID, B1 断言), EstSeconds=req.DurationSeconds; 回显 idemKey
func (f *FakeAsync) Poll(ctx, jobID) (PollResult, error)    // 第 < pollsToDone 次→Pending; 第 pollsToDone 次→Done(result)
func (f *FakeAsync) Generate(ctx, req) (GenResult, error)   // Submit+忙轮询到 Done(便捷; worker 不走这条)
```

- **确定性**：`pollsToDone=2` 意味着活验里 worker 第一次 dispatch=submit、第二次=Pending重排、第三次=Done——精确断言「submit→poll→poll 完成」三段。
- **零网络**：Done 的 `result.URL` 在 e2e 里指向一个 `httptest` 起的本地服务（返回固定字节 + `video/mp4`），让 §9.4 的 SSRF-safe 拉回路径在 sandbox 内真实跑通（用 `fetch.NewLoopbackForTest`，`fetch.go:84`，允许 loopback）。
- **注册**：经 `cmd/studiod/main.go` 的 `registryHook`（`main.go:153-155`，e2e seam）注入，键 `fake/fake-video-async`、`fake/fake-audio-async`，并 `models.Catalog()` 含同名条目，使 org 可把默认 video model 配成 fake → 走异步引擎全程。

---

## 8. key-gated 真实适配器（骨架 + build-vs-buy）

### 8.1 适配器接口与注册（骨架，照搬 M3 key-gated 范式）

真实视频/音频 SaaS 各为 `internal/generate/video/<provider>.go` / `internal/generate/audio/<provider>.go`，实现 `generate.AsyncGenerator`（video）或 `generate.MediaGenerator`+可选 `AsyncGenerator`（audio：短 TTS 可同步，长合成异步）。注册照搬 `registerImageGenerators`（`main.go:289-323`）的 **key-gated** 范式——**仅当对应 env key 存在才注册真实适配器**：

```go
// NEW — main.go: registerVideoGenerators / registerAudioGenerators
// 遍历 catalog 但按 kind 过滤(M3, §6.4): video 注册器只处理 e.Kind=="video"、
// audio 只处理 e.Kind=="audio"; 仅 provider 有 key 时注册真实适配器;
// 无 key → 该 provider/model 解析回落到 registry 默认(或保持未注册)。
// 与 registerImageGenerators(main.go:289) 完全同构(后者也加 e.Kind=="image" 过滤)。
// 注册时同样经 obs.WrapGenerator —— 它会保留 AsyncGenerator 接缝(§4.2b B1)。
RunwayAPIKey / KlingAPIKey / VeoAPIKey(走 GoogleAPIKey) / TTSAPIKey  // config.go 新增, 同 OpenAIAPIKey 模式(config.go:100-103)
```

`AsyncGenerator` 接口在 M4 含 **可选 `Cancel(ctx, jobID) error` 方法**（Q4）：M4 只给一个 no-op 默认实现（或独立可选接口 `Canceler`，断言后调），真正向 provider 发 HTTP 取消的实现 **延后 M5**（标 `// TODO(m5): real cancel HTTP`）。本地取消（停轮询 + 终态化 submitted 资产）在 M4 已由 §5.4 解决，不依赖外部 Cancel。

- **真实 SaaS HTTP 接线 + 密钥 + ffmpeg：out-of-band / 延后**。M4 只交付：接口、key-gated 注册骨架、**编译通过 + 对 fake 的集成测试**。真实适配器体内的 HTTP（Runway/Kling/Veo 各家 submit/poll REST 形态不同）标 `// TODO(m5): real HTTP wiring` 并以「未配 key → 不注册 → 不被解析」保证不影响活验。这遵循 CLAUDE.md 第2条（不为未验证的外部集成写臆测代码）。
- 适配器**必须**复用 `internal/fetch` 拉回结果 URL（§9.4），且 Submit/Poll 的 provider 鉴权头走服务端 env（永不下发浏览器，父文档 §8 下游凭据隔离）。

### 8.2 build-vs-buy（父文档 §13 R5 / §15 M4 强制）

| 能力 | 方案 | 形态 | 集成 shape | 大致成本/特点（量级，非报价） | M4 取舍 |
|---|---|---|---|---|---|
| 视频生成 | **Runway Gen-3** | SaaS REST，submit→poll | async 适配器（Submit 返 jobID，Poll 查状态，Done 给 URL→fetch 拉回） | 按秒/按生成计，分钟级延迟 | **骨架**（key-gated） |
| 视频生成 | **Kling（可灵）** | SaaS REST，submit→poll | 同上 | 国内可达，价低，分钟级 | **骨架** |
| 视频生成 | **Google Veo** | SaaS（走 google provider 家族），submit→poll | 同上；key 复用 `GoogleAPIKey` | 高质量，配额受限 | **骨架** |
| 音频/TTS | **OpenAI tts-1 / 类 TTS** | SaaS REST，短文本可**同步**返字节 | 短 TTS 走同步 `Generate`（不实现 AsyncGenerator）；长合成走 async | 按字符/秒，秒级 | **骨架**（短 TTS 可优先真接） |
| 背景音乐/音效 | — | — | — | — | **延后**（PRD 模块7 列出，M4 不接缝） |
| 自动剪辑 | **ffmpeg（自建/外部）** | 本地进程 / 外部服务 | 非 generator——是后处理流水线 | 计算密集，需二进制 | **延后**（§10） |

**buy 而非 build 的理由**：视频/音频底模训练与 GPU 推理远超本案例项目范围（这是「准生产案例」非「自研模型」）；所有候选都是「submit→poll 的外部 HTTP」，恰好验证 §5 异步引擎的设计——故 M4 价值在**引擎 + 接缝**，真实 provider 是薄壳。

---

## 9. 准生产横切（全量复用 M3 范式）

1. **计费**：`pricing` + 计费 chokepoint（`cost/store.go:132`），M4 加按秒维度（§6.3）。**异步路径不是 poll-done 才写 generations**——而是 **submit 预登记 upsert + poll-done UPDATE 回填**（I2+I5+B3，§6.3/§9.3）：submit `UpsertSubmittedGeneration` 写一行（估算成本，DB 唯一索引去重）、poll-done `UpdateGenerationByAssetTodo` 回填真实 `VideoSeconds`/`cost_micros`（`Kind:"video"/"audio"`），绝不二次 insert。image 同步路径仍单次 `RecordPriced`（一次 insert）。
2. **otel（含 B1 必修）**：`obs.WrapGenerator`（`main.go:145/320`）包真实+fake generator——**但当前实现会剥掉 `AsyncGenerator` 接口**（返回只实现 `Kind()+Generate()` 的 `*tracedGenerator`，`obs/obs.go:58-81`），使 worker 的 `routed.(AsyncGenerator)` 断言恒假、异步被误走同步。**修复见 §4.2b**：`WrapGenerator` 在内层实现 `AsyncGenerator` 时返回同样实现它的 `tracedAsyncGenerator`。Submit/Poll 各自起 span（`studio.generate.submit.<kind>` / `studio.generate.poll.<kind>`），带 `studio.provider/model/external_job_id/poll_status` 属性，与 `worker.go:200-204` 的现有 span 同风格。**这是 §5 异步引擎在生产装配下被触发的前提**。
3. **配额/并发（I2+I5，**必修**：submit-insert/poll-update 账本同时治好「在途欠计 + 崩溃双计」）**：org rolling-24h 生成配额 `GenQuota`（`worker.go:414-426` 的 backstop）对 video/audio **同样生效**。`CountByOrgSince`（`cost/store.go:254`）数的是 `generations` 行——**问题**：若 generations 行只在 poll-done（`RecordPriced`）才写，则从 submit 到 poll-done 的整个在途窗口里 `CountByOrgSince` **欠计**（并发 submit 全部通过配额检查，配额被击穿），且崩溃-重试落在 poll 中段会 **二次 insert 重复计费**（I5）。
   - **解法（与 §6.3 同一个机制）**：generations 行在 **submit 时就 upsert**（`UpsertSubmittedGeneration`，估算成本），poll-done **UPDATE 同一行**（`UpdateGenerationByAssetTodo`，回填真实秒数/成本），**绝不二次 insert**。这一举两得：
     - `CountByOrgSince` 现在 **把在途行也数进去** → submit 阶段的配额 backstop 看到真实的「已提交+已完成」总数，并发 submit 不再击穿配额；
     - submit 的 upsert 凭 `generations_asset_todo_uniq`（§6.2 DB 唯一索引）去重 + poll-done 的 UPDATE 幂等 → 崩溃重试 **不双计**（I5 闭合）。
   - 配额 backstop 仍在 **submit 阶段** 做（poll 阶段不再查配额，避免每次 poll 被当成新生成）；但「一次生成 = 一行 generations」的不变量从 poll-done 提前到 submit。Q2 据此 **RESOLVED**（见 §15 决策表）。并发隔离见 §5.3。
   - **第二轮（二轮 I1「配额竞态」，区别于一轮 I1「errRescheduled」，**必修**）：submit-insert 仍未消除并行 worker 的【配额竞态】。** submit 阶段是「先 `CountByOrgSince` 数 n，再 upsert」的 count-then-act（`worker.go:414` 的 backstop 形态）：两个 worker 可同时读到 `n=99`、都判定未超 `GenQuota=100`、都 upsert → 越额。generations 这笔是 **billing-sensitive**（视频按秒计费，越额=多烧钱），故 **采硬配额**（option a，非软上限）：
     - **用事务级 advisory lock 串行化 org 准入**：把 submit 阶段的 {`CountByOrgSince` + `UpsertSubmittedGeneration`} 包进一个事务，事务内先 `SELECT pg_advisory_xact_lock(hashtext($orgID))` —— 同 org 的并发 submit 被串行化，count 与 insert 之间不再有别的 worker 插队，配额成为 **硬上限**（无越额）。事务提交时 advisory lock 自动释放（xact 级，无需手动 unlock）。
     - **复用 M3 既有范式**：这正是 M3 `run_done` 已用的 `pg_advisory_xact_lock` 模式（避免重复 run_done），M4 把同一范式套到 org 配额准入，无新机制。
     - **与按 kind 并发上限（§5.3 软上限）的区别**：§5.3 的并发隔离是 **软** 上限（可瞬时越 `Workers-1`，仅做吞吐限流，非 billing 不变量）；本条 org 生成配额是 **硬** 上限（advisory-lock 串行化，billing 不允许越额）。两者性质不同、刻意不同处理——软的限并发、硬的限计费总额。
     - **落点**：runAsset submit 分支的配额 backstop 段（`worker.go:414-426` 在异步 kind 下）改为「open tx → advisory_xact_lock(org) → CountByOrgSince → 超额则回滚返错 / 未超则 UpsertSubmittedGeneration + SetSubmitted + reschedule（即 §5.2 的单事务，advisory lock 就在这同一个事务里）→ commit」。即 B1 的「{SetSubmitted+upsert+reschedule} 单事务」与 I1 的「advisory-lock 串行配额」**是同一个事务**，天然合并。
4. **SSRF-safe 拉取（Q1：流式延后 M5，M4 靠硬上限 + 并发上限兜 OOM）**：video/audio 结果 URL **必须**经 `internal/fetch`（`fetch.go:40` `New` + `:89` `Get`）拉回——新建 video/audio 专用 fetcher，`AllowedContentTypes: []string{"video/", "audio/", "application/octet-stream"}`。复用 image 适配器 `image.go:32-40` 的 `Puller` 接缝模式（nil → 默认 SSRF-safe fetcher；测试注入 loopback fetcher）。
   - **OOM 天花板（Q1，RESOLVED-as-DEFER）**：`fetch.Get` 当前 `io.ReadAll` 全量入内存（`fetch.go:116`），视频可达数百 MB。**M4 不做流式拉回到 blob**（属独立增强，延后 M5）；M4 仅靠两道边界把 OOM 风险压到可接受：
     1. video/audio fetcher `MaxBytes` **设硬上限 512MB**——超限即拒（拉回失败 → `SetAsyncFailed`，不 OOM）；
     2. 依赖 `MaxConcurrentVideo`（§5.3）限制并发大文件拉回数 → 内存峰值上界 ≈ `MaxConcurrentVideo × 512MB`，运维据此配 worker 内存。
   - 这道天花板 **文档化为已知限制**：单文件 > 512MB 在 M4 无法处理（需 M5 流式）；并发上限与内存的乘积关系是运维容量规划的输入。
5. **SSE 白名单**：`internal/httpapi/sse.go:22-31` 的 `sseEventNames` 是闭集白名单（防伪造 kind）。M4 新增异步事件**必须加进白名单**，否则降级为 `message`（`sse.go:62-64`）：
   ```go
   "asset_submitted":  true,  // 外部 job 已提交(异步, UI 显示"生成中…轮询") —— M4 发
   // "asset_polling" 不在 M4 发射范围(M2 DEFER, 见下)
   ```
   - **`asset_submitted`（M4 发）**：submit 阶段发一次，UI 据此把该资产标记为「生成中…轮询」。
   - **`asset_polling`（M2，**定为 DEFER**，不在 M4 发射）**：每次 poll-pending 发一条进度事件，会让分钟级 video（数十次 poll）刷出几十条噪声事件，无实际信息量——`asset_submitted`（开始）+ `asset_generated`（完成）两个端点已足够表达异步生命周期。M4 **不发** `asset_polling`。白名单层面 **仅** 保留对该事件名的前向兼容认知（即未来若要发，加进白名单即可），但 M4 **不把它加进白名单、不在任何地方发射**——避免引入未被消费的事件。
   - `asset_generated`（`sse.go:28`）复用为「拉回完成、待审」终点事件——与 image 路径一致，UI 时间线无需区分同步/异步终点。
6. **RBAC / 归属**：`scope_kind="org"` + admin/viewer/editor 门禁（父文档 §8）**零改**——video/audio asset 经 `assets.OrgIDForAsset`（`assets/store.go:271`，project join）做归属校验，HITL `accept/reject/regenerate` 路由（`httpapi.go` `/api/assets/{id}/...`）对 video/audio 资产一视同仁（admin-only）。
7. **签名 URL 直连**：video/audio 资产同样经短 TTL 签名 URL 直连（父文档 §10）——大文件尤其不能代理字节。`BlobStore.SignedURL` 不变。

---

## 10. 非目标 / 延后（显式，含「出局理由 + 重启条件」）

| 项 | 为何 M4 出局 | 重启需要什么 |
|---|---|---|
| **配音（dubbing）** | 是「TTS + 时间轴对齐 + 混音」的组合流水线，依赖音频接缝先稳 + 剪辑能力 | M4 音频接缝落地后，新里程碑：对齐 + 混音逻辑 |
| **自动剪辑（ffmpeg）** | 非 generator——是后处理（拼接/转码/字幕烧录），需 ffmpeg 二进制 + 计算密集 worker，与「submit→poll 外部 SaaS」模型不同 | 独立「后处理流水线」设计：ffmpeg sidecar/进程池 + 新 todo type `edit` |
| **图片 LoRA** | 是 image 路径的训练/微调扩展（自定义风格权重），需训练编排 + 权重存储，超「调用现成 generator」 | image generator 支持 LoRA 参数透传 + 权重 BlobStore + 训练 job 类型 |
| **数字人（digital human）** | 父文档 §15 已定「评估后或拆独立里程碑」；是视频+音频+口型+驱动的复合系统 | 视频/音频接缝稳定后单独评估，大概率独立里程碑 |

共同原则：M4 **只**交付能被 fake 活验、且对真实 provider 是薄壳的部分；任何需要新计算范式（训练、ffmpeg）或复合编排（配音、数字人）的能力都延后，避免给「准生产案例」引入未验证的重型子系统（CLAUDE.md 第2条）。

---

## 11. 风险（扩展父文档 R4/R5/R6）

1. **R4'（异步引擎，原 R4 收口）**：submit→poll 状态机 + 租约续约 + 按 kind 并发隔离已设计（§5）。残余风险：poll 预算（`MaxPollAttempts × backoff`）需覆盖最慢 provider；外部 job 在 cancel 后仍计费（已知窗口，§5.4）。缓解：预算可配 + 文档化 best-effort cancel。
2. **R5'（真实 provider 缺失，原 R5）**：M4 只交付骨架 + fake 活验；真实 HTTP/密钥/ffmpeg out-of-band。残余：真实适配器上线前无法端到端验真。缓解：fake 与真实走**同一 `AsyncGenerator` 接口**，真实适配器上线时只换实现、引擎不动；真实适配器交付时配 integration test（对 fake）+ 编译验证。
3. **R6'（成本失控，原 R6 放大）**：视频按秒计费，单次成本远高于图片。残余：分钟级视频 + fan-out 可能瞬时高额。缓解：submit 阶段配额 backstop（§9.3）+ 按 kind 并发上限（§5.3）+ `pricing` 按秒入账实时累计。**新增建议**：org 级「视频生成日上限」独立旋钮（比通用 GenQuota 更严），列为 M4 可选加固。
4. **R9（NEW，poll_attempts/attempts 混淆）**：若误用 `attempts` 计轮询次数，正常轮询会撞 `MaxAttempts=3` 被误判失败。缓解：独立 `poll_attempts` 列（§5.5/§6.2），submit→poll 重排走「成功重排」不增 `attempts`——这是引擎正确性的核心不变量，必须有针对性测试。
5. **R10（NEW，大文件拉回内存）**：视频文件可达数百 MB，`fetch.Get` 当前 `io.ReadAll`（全量入内存，`fetch.go:116`）。残余：多并发大文件拉回可能 OOM。缓解：M4 video fetcher `MaxBytes` 设硬上限（512MB）+ 按 kind 并发上限收窄；**长期**（延后）需流式拉回到 BlobStore（`fetch` 现为内存模型，改流式属独立增强，不在 M4）。

---

## 12. 里程碑分解（M4 子阶段，映射未来 impl plan 任务形态）

> 后端主线；UI 由 UI agent 跟进（资产库视频/音频过滤、时间线异步态）。每子阶段一组 TDD 任务（mirrors kb M1-M3 plan 形态：先写复现/断言测试再实现）。

- **M4a — 接缝 + fake + 数据模型**：`generate` 加 `AsyncGenerator`（`Submit(ctx,req,idemKey)`，含可选 `Cancel`）/`SubmitResult`/`PollResult`/`PollStatus` + `GenRequest` 加性字段（`DurationSeconds`/`Voice`）；`FakeAsync`（回显 idemKey、同 key 返同 jobID）；**`obs.WrapGenerator` 保留 AsyncGenerator 接缝 + 透传 idemKey（B1，`tracedAsyncGenerator`）**；`m4Migrations`（poll_attempts/external_job_id/submitted_at/pricing.micros_per_second + **`assets_todo_uniq`**（B1）+ **`generations_asset_todo_uniq`**（B3）+ 种价）；`assets.SetSubmitted` + **`GetOrCreateForTodo`**（B1）+ **`CountInFlightByKind`**（B2）+ **`SetAsyncFailed`**（B2）+ `SetBlob` 成功 from 放宽；`cost` 按秒计费 + **`UpsertSubmittedGeneration` / `UpdateGenerationByAssetTodo`**（I2/I5/B3）；`Generation.VideoSeconds` 注释改「媒体秒数」（Q3）。**验证**：fake submit→poll 单测；**`obs.WrapGenerator(fakeAsync).(AsyncGenerator)` 断言成立单测（B1 回归）**；**同一 idemKey 二次 Submit 返同 jobID（B1 回归）**；迁移 idempotent + 两个唯一索引拒重复（B1/B3 回归）；按秒计费 + upsert/poll-update 幂等单测。
- **M4b — 异步引擎**：worker 异步路径（runAsset 分流 submit/poll + 重排）；**submit 用 `GetOrCreateForTodo` + 派生 idemKey + {SetSubmitted+upsert+reschedule} 单事务（B1）**；**崩溃恢复走查：reclaim 见 submitted+external_job_id → 跳过 Submit 续轮询（B1）**；**runAsset→process `errRescheduled` 重排契约（I1）**；**「成功重排」UPDATE 重置 `attempts=0`（I6）** + `poll_attempts` 语义；**fan-out + regenerate 写 `input_json.kind`/`duration`（I3）**；**generations 行 submit `UpsertSubmittedGeneration` + poll-done `UpdateGenerationByAssetTodo` 回填（I2/I5/B3）**；**org 配额 advisory-lock 硬串行（I1，与 submit 单事务合并）**；**submit-admission 上限按 `CountInFlightByKind` 限外部在途 job + poll 重领不受限（B2）**；租约续约 heartbeat（`renewLease`，**作用域绑定 dispatch ctx + 双守卫 + status-guard 自洽**，I3/I4）；claim SQL 的 fetch 软上限（读 `input_json->>'kind'`，无 JSONB 索引）；`DefaultForOrg(kind)` 参数化（去掉 `worker.go:458` 硬编码 "image"）；**终态失败分支：`PollFailed`/poll 预算耗尽 → `SetAsyncFailed`+`MarkFailed` 立即终态，不走 attempts 退避（IMPORTANT 2）**；config 旋钮。**验证（活验主线）**：注入 FakeAsync(pollsToDone=2) → 真 Postgres，断言 submit→poll→poll→pending_acceptance + 健康资产**不被 discardCanceledAsset 误杀**（I1 回归）；连发 ≥3 次正常 poll 不撞 MaxAttempts（I6 回归）；submit-admission 上限挡新 submit、poll 仍可领（B2 无死锁回归）；并发隔离（依赖 input_json.kind 真有值——I3 回归）；poll 预算耗尽 → asset+todo 同时 failed（IMPORTANT 2 回归）；并行 worker 配额不越额（I1 advisory-lock 回归）；submit 在途的 generations 行被 `CountByOrgSince` 计入（I2 回归）+ 崩溃重试不双计/不双提交（I5/B1 回归）。
- **M4c — SSRF 拉回 + SSE + cancel + 孤儿清理 + otel**：video/audio fetcher（content-type/MaxBytes=512MB，Q1）+ loopback test 拉真字节（`Blob.Put` 覆盖写幂等，minor）；**SSE 白名单仅加 `asset_submitted`（`asset_polling` 不加、不发——M2 DEFER）**；`project.Cancel` 覆盖 submitted（B2）+ **本地取消（poll dispatch 检测 canceled todo 即停 + submitted→canceled，Q4）**；**孤儿 reaper（submitted + submitted_at 过 TTL → failed，M1）**；otel submit/poll span。**验证**：httptest 服务 + loopback fetcher 端到端拉回落 blob + 重复 poll-done put 同 key 幂等（minor 回归）；SSE 时间线含 `asset_submitted`/`asset_generated`（无 `asset_polling`）；cancel 中途 submitted 资产终态化 + poll 停止；reaper 终态化卡死的 submitted。
- **M4d — key-gated 真实适配器骨架 + build-vs-buy**：`registerVideoGenerators/registerAudioGenerators`（key-gated，**按 kind 过滤 catalog**——M3；image 注册器同步加 `kind=="image"` 过滤）；Runway/Kling/Veo/TTS 适配器骨架（接口实现 + `// TODO(m5) real HTTP` + `Cancel` no-op）；config 加各 provider key；catalog 加 video/audio 条目。**验证**：编译通过；无 key→不注册（单测）；image 注册器不触碰 video/audio 条目（M3 回归）；真实适配器对 fake 的接口契约测试。

---

## 13. 验证策略（sandbox 现实）

- **可活验（sandbox 内，真 Postgres + MinIO + httptest，零外部 API）**：异步引擎全程（submit→poll→拉回→pending_acceptance→accept→库检索）、按 kind 并发隔离、cancel 中途、poll 预算耗尽、按秒计费账本、SSE 异步事件——**全部经 `FakeAsync` + loopback fetcher 覆盖**。这是 M4 「live-verifiable here」的核心保证。
- **第二轮新增载重回归（必须各有针对性测试）**：
  - **B1 接缝保留**：`WrapGenerator(fakeAsync).(generate.AsyncGenerator)` 断言成立（包装层未剥异步接缝）。
  - **B1 崩溃幂等**：同一 `idemKey` 二次 `Submit` 返同 jobID（FakeAsync）；模拟「submit 后崩溃（不落 external_job_id）→ reclaim」→ 不产生第二个 job、`GetOrCreateForTodo` 复用同一 asset 行、reclaim 见 submitted+job_id 跳过 Submit 续轮询。
  - **B2 在途上限 + 无死锁**：`submitted` 计数打满时新 submit 被挡、而 poll 重领不受限（已完成 job 仍能 drain）；`assets_todo_uniq` 拒重复建。
  - **B3 账本去重**：`generations_asset_todo_uniq` 拒重复 insert；`UpsertSubmittedGeneration` 二次调返同行、不双计。
  - **I1 配额硬上限**：并行 worker 并发 submit 同 org 不越 `GenQuota`（advisory-lock 串行）。
  - **IMPORTANT 2 终态分支**：`PollFailed` / poll 预算耗尽 → asset 与 todo **同时** `failed`、不经 attempts 退避重试；transient poll 网络错不调 `SetAsyncFailed`、消耗 poll_attempts 后重排。
  - **minor blob 幂等**：同一 key 重复 `Blob.Put` 不报错、不产生第二份对象。
- **真实 SaaS 不可达**：Runway/Kling/Veo/TTS 在 sandbox 内**不可达**（外部 HTTPS + 需密钥）。真实适配器以 **接口契约测试（对 fake）+ 编译验证** 覆盖；真实 HTTP 接线（含 idemKey→client-token header 映射）+ 密钥 + ffmpeg 为 out-of-band，标 TODO 留 M5。
- **回归**：M1-M3 既有 e2e（auth/RBAC 越权、文本管线、图片管线、HITL 版本血缘、畸形回落、成本账本、generator 接缝）**全绿不退化**——image 同步路径零改是硬约束（§4.1）。

---

## 14. 参考文件（实现期，全部已实读）

- `internal/generate/generate.go:9-38`（`GenRequest`/`GenResult`/`MediaGenerator`——异步接缝加性扩展点）
- `internal/obs/obs.go:51-81`（`WrapGenerator`/`tracedGenerator`——**B1 必修**：当前剥掉 AsyncGenerator，须加 `tracedAsyncGenerator` 保留接缝）
- `internal/generate/registry.go:24/38`（`Register`/`Resolve`——零改，generator 解析后断言 AsyncGenerator）
- `internal/generate/image/image.go:32-82`（image 适配器——单遍同步模板 + `Puller`/SSRF 拉回模板）
- `internal/generate/fake.go:10-44`（image fake——`FakeAsync` 范本）
- `internal/worker/worker.go`（**异步引擎落点**：claim `:165-193`、**stuck-reclaim 领过期 running todo `:167`（B1 二次 submit 入口）**、CallTimeout `:208-213`、runAsset `:397-513`、**配额 backstop count-then-act `:414`（I1 advisory-lock）**、**createAsset 无脑 insert `:428`/`:517`（B1 GetOrCreateForTodo 替换）**、DefaultForOrg 硬编码 image `:458`、fail 退避 `:580-613`、cancel/discard `:643-658`、WithoutCancel `:446`）
- `internal/todos/store.go:83-218`（`MarkDone`（`:91-92` running 守卫——I1 重排不能走它）/`MarkFailed`（IMPORTANT 2 终态分支复用）/`AddDynamic`（`:164-177` fan-out input_json——I3 写 kind）/`cancelDependents`——重排与终态）
- `internal/models/store.go:180-192`（`DefaultForOrg(kind)`——已参数化，worker 待去硬编码）；`models.Catalog()`（`:68`，`Kind` 字段——M3 按 kind 过滤注册器）
- `internal/assets/store.go:100-126`（`Create` 无脑 INSERT——B1 据此加 `GetOrCreateForTodo` + `assets_todo_uniq`）`:141-157`（`SetBlob` from 守卫 `:143`——B2 成功放宽 + 新 `SetAsyncFailed` + `CountInFlightByKind`（B2）/ `TransitionStatus`——discard 加 submitted）
- `internal/storage/storage.go:42/97-110/125/182/205`（m1/m2/m3 迁移 slice + `shots.duration`（`:107`，I3 时长源）+ generations 表（含 `generations_project_idx`，唯一索引缺位——B3）+ assets 表（`assets_status_idx`——B2 CountInFlightByKind 复用）+ `Migrate` 拼接——加 m4Migrations）
- `internal/cost/store.go:29`（`Generation.VideoSeconds`——Q3 注释改媒体秒数）`:54`（`Record` 用 `newID()` 随机 id 无脑 insert——B3 据此知去重必须靠唯一索引而非随机 id）`:102-141`（`Price`/`PriceFor`/`ComputeCostMicros`/`RecordPriced`——加按秒 + 新 `UpsertSubmittedGeneration`/`UpdateGenerationByAssetTodo`，I2/I5/B3）；`:254`（`CountByOrgSince` 配额源——I2 须计在途行）
- `internal/httpapi/sse.go:22-31`（`sseEventNames` 白名单——加异步事件）
- `internal/config/config.go:36-103/140-157`（M3 旋钮 + key-gated key + `durOf`/`intOf`——加异步/并发/video-audio key）
- `cmd/studiod/main.go:139-155/289-323`（registry 装配 + `registerImageGenerators` key-gated 范式 + `registryHook` e2e seam——加 register{Video,Audio}Generators）
- `internal/project/store.go:168-185`（`Cancel`——扫 submitted 态）
- `internal/fetch/fetch.go:40/84/89/116`（`New`/`NewLoopbackForTest`/`Get`/`io.ReadAll`——video/audio 拉回 + Q1 OOM 天花板）
- 父：[[2026-06-10-ai-studio-design]] §7.2/§13 R4-R6/§15 M4；[[2026-06-10-llm-agent-studio-prd]] 模块7/§8 二期。

---

## 15. 决策记录 / 开放问题收口（第一轮 + 第二轮架构评审）

> §15.1/§15.2 记录第一轮 findings；§15.3 记录第二轮独立 codex 评审的 findings 收口结果。每条标 finding 编号 + 落点小节。

### 15.1 必修 findings（第一轮，已在对应小节落地）

| # | 问题（一句话） | 落点 | 决策 |
|---|---|---|---|
| **B1** | `obs.WrapGenerator` 剥掉 `AsyncGenerator` 接口 → worker 断言恒假、异步被误走同步 | §4.2b / §9.2 | `WrapGenerator` 在内层实现 AsyncGenerator 时返回 `tracedAsyncGenerator`（Submit/Poll 委托 + span），否则 `tracedGenerator` |
| **B2** | 失败/取消路径把资产搁浅在新 `submitted` 态（SetBlob 守卫只认 generating） | §5.4 / §6.2 | 新增 `SetAsyncFailed(from IN generating,submitted)`；成功 from 放宽；Cancel 扫描 + discard from-list 加 submitted；逐条枚举所有失败转移 |
| **I1** | runAsset 重排后 process 仍 MarkDone → done=false → discardCanceledAsset 误杀健康资产 | §5.2(a-bis) | runAsset 返回哨兵 `errRescheduled`，process 拦截后跳过 MarkDone/discard/emitNewlyReady/RefreshStatus/run_done |
| **I2+I5** | 配额在途欠计 + 崩溃双计 | §6.3 / §9.3 | generations 行 submit `UpsertSubmittedGeneration` + poll-done `UpdateGenerationByAssetTodo` 回填，绝不二次 insert；`asset_id+todo_id` 去重（第二轮 B3 升为 DB 唯一索引）→ CountByOrgSince 计在途 + 幂等 |
| **I3+M4** | `input_json->>'kind'`（及 EstSeconds/DurationSeconds）今天根本没写 → 子上限永不生效 | §5.3 / §6.2 | fan-out + regenerate 写 `kind`/`duration`（与 DefaultForOrg 路由同源；duration 源 `shots.duration`）；不加 JSONB 索引（YAGNI） |
| **I4** | 心跳可能复活已重排的 poll todo 的租约 | §5.2b | 心跳作用域绑 dispatch ctx；renewLease 双守卫 `locked_by=$worker AND status='running'`（第二轮：弃「重排前停」不可达时序，改靠 status-guard，重排清租约后心跳自然 no-op） |
| **I6** | `claim` 无条件 `attempts++` → 正常轮询 3 次撞 MaxAttempts | §5.5 | 「成功重排」UPDATE 同条 SQL `attempts=0`，poll_attempts 才是真预算 |
| **M1** | submitted_at 悬空 + submitted 永久搁浅风险 | §5.4 / §6.2 | submitted_at 喂孤儿 reaper（submitted 过 TTL → failed），reaper 列为 M4c 小任务 |
| **M2** | `asset_polling` SSE 事件 | §9.5 | **定为 DEFER**：M4 不发、不加白名单；仅前向兼容认知 |
| **M3** | image 注册器迭代整个 Catalog（含 video 条目） | §6.4 / §8.1 | 三注册器各按 `e.Kind` 过滤，互不串扰 |

### 15.2 开放问题收口

| Q | 问题 | 决策 | 落点 |
|---|---|---|---|
| **Q1** | R10 大文件拉回 OOM | **DEFER 流式到 M5**；M4 设 `MaxBytes=512MB` 硬上限 + 靠 `MaxConcurrentVideo` 限并发，内存天花板 ≈ `MaxConcurrentVideo × 512MB`，文档化 | §9.4 / §11 R10 |
| **Q2** | 配额计量时机（submit vs poll-done） | **RESOLVED by I2**：submit `UpsertSubmittedGeneration` + poll-done UPDATE，配额 backstop 在 submit 阶段（第二轮 I1 加 advisory-lock 硬串行），CountByOrgSince 自然计入在途 | §9.3 |
| **Q3** | 音频时长是否加 `audio_seconds` 列 | **RESOLVED**：复用 `video_seconds` 承载「媒体秒数」，不加列；Go 字段语义注释改 MediaSeconds | §6.3 |
| **Q4** | 外部 cancel | 真正向 provider 发 HTTP 取消 **DEFER M5**（M4 给 `AsyncGenerator.Cancel` 可选 no-op）；但**本地取消**（poll 检测 canceled todo 即停 + submitted→canceled 终态化）**M4 必须解决**（绑 B2） | §5.4 / §8.1 |

### 15.3 第二轮 findings（独立 codex 评审，已在对应小节落地）

| # | 问题（一句话） | 落点 | 决策 |
|---|---|---|---|
| **B1（二轮）** | Submit 非崩溃幂等 → provider 双提交（孤儿计费 + 丢结果）；asset 创建也非幂等 | §3 / §4.2 / §5.2 / §5.2c / §6.2 | Submit 签名加确定性 `idemKey=hash(todoID)`（真适配器转发为 client-token，fake 回显）；`assets_todo_uniq` 部分唯一索引 + `GetOrCreateForTodo`；{SetSubmitted+upsert+reschedule} 单事务；崩溃恢复走查（reclaim 见 submitted+job_id 跳过 Submit） |
| **B2（二轮）** | 按 kind 并发上限只限本地瞬时 HTTP，不限外部在途 job（submit 后 todo 转 ready）→ 可瞬间 submit 100 个 video | §5.3 / §6.2 | submit-admission 上限按 `CountInFlightByKind`（`assets.status='submitted'`）限外部在途 job；**poll 重领不受该上限约束**（否则 drain 死锁）；原 claim-SQL running 计数重定位为独立的 fetch/拉取软上限，两上限命名/执行点分明 |
| **B3（二轮）** | 账本 `asset_id+todo_id` 去重只是 prose，无 DB 约束（Record 用随机 id 无脑 insert） | §6.2 / §6.3 / §9.3 | `generations_asset_todo_uniq` 部分唯一索引；`UpsertSubmittedGeneration`（`ON CONFLICT...RETURNING id`）+ `UpdateGenerationByAssetTodo`；不依赖 read-before-insert |
| **I1（二轮）** | submit-insert 配额仍 count-then-act 竞态（两 worker 同见 n=99 都插） | §9.3 | billing-sensitive → **硬配额**：submit 事务内 `pg_advisory_xact_lock(hashtext(orgID))` 串行 org 准入（复用 M3 run_done advisory-lock 范式）；与 §5.3 软并发上限性质刻意不同 |
| **I2（二轮）** | 终态 provider 失败被错误组合进通用 attempts 退避（asset=failed 但 todo 仍重试） | §3 / §5.2 / §5.5 | 区分 transient（不调 SetAsyncFailed，消耗 poll_attempts 重排）vs 终态（`PollFailed`/poll 预算耗尽 → `SetAsyncFailed`+`MarkFailed` 立即终态，不走 attempts 退避） |
| **I3（二轮）** | heartbeat 时序自相矛盾（「重排前停」与「runAsset 返回后停」不可同时成立） | §5.2b | 取更简 status-guard 方案：弃「重排前停」；重排清租约 + renewLease 双守卫令重排后心跳自动 no-op；§5.2b 自洽 |
| **minor（二轮）** | WrapGenerator 异步保留需命名测试；Blob.Put 重复 key 幂等需声明 | §4.2b / §13 / §6.2 | 加 `WrapGenerator(fakeAsync).(AsyncGenerator)` 命名回归测试（确认 sound）；声明 `Blob.Put` 同 key 覆盖写幂等不变量 |
