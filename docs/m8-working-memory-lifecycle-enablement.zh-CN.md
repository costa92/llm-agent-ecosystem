# M8 Working-Memory 生命周期：生产启用设计

> 文档版本：2026-06-02（v4 — 两轮评审 + D1-D8 拍板已锁定）
> 状态：**设计依据已定稿，可作为立项/实现依据**。演进：v1 初稿 → v2（round-1 Plan 内审 H1/H2/M3/M4/M5/L8）→ v3（round-2 Codex 外审 C1/C2/C3）→ v4（D1-D8 拍板）。**关键判断（拍板后）**：因 D3（契约下沉）+ D8（写栅栏）取重选项，这是**四仓 lockstep（contract+postgres+worker+gateway）、含契约版本 bump + 写 API 语义变更**的 milestone 量级协调工作，**非单仓激活**（见 §7 表、§6、§9）。建议分两段：步骤 0（postgres C1 修复）立即独立推进；步骤 1-7 正式立项。
> 关联：[`./superpowers/specs/2026-05-27-m8-umbrella-design.md`](./superpowers/specs/2026-05-27-m8-umbrella-design.md)（M8 拆分总纲，本文是其未单独立项的一块）、[`./memory-roadmap.zh-CN.md`](./memory-roadmap.zh-CN.md)、[`./memory-gateway-api-contract.zh-CN.md`](./memory-gateway-api-contract.zh-CN.md)

---

## 1. 一句话结论

**working-memory 生命周期的「晋升」一半已经在生产里跑（M8b worker）；缺的是「过期回收」一半——working 记录目前在生产中永不清理。** 把已经写好、测试齐全但未接线的 `DurableSessionCloser` 接进生产，就补齐了这一半的**主路径**。它的工作量是**激活已有机器**（一个适配器 + 一行接线 + 一个 metrics observer 实现），不是从零设计。CAP2 那对生命周期指标（`working_expired_total` / `working_dropped_before_use_total`）之所以现在是「死指标」，正因为它们度量的就是这个尚未接线的回收器。

> ⚠️ **边界 1（round-1 H1）孤儿会话不回收**：接线 `DurableSessionCloser` 只回收**被显式 `POST /sessions/{id}/close` 关闭**的会话。**从不显式关闭的会话（客户端崩溃/放弃/never-close）其 working 记录仍永久累积**——`SessionIdleTTL` 只用于**拒绝**对 idle 会话的访问（`service.go:820-848`），不触发关闭，全代码无任何 idle-session 清扫器。见 §7 D6。
>
> ✅ **边界 2（round-2 C2）→ D8 已决定加写栅栏修复**：现状 `validateSessionState` 只在 `RecallUnified`（`service.go:121`）和 `HeartbeatSession`（`678`）调用，`WriteMemory`/`PatchMemory`/`PinMemory`/`DeleteMemory`（`261/307/392/569`）**不检查会话状态**，故关闭后写入仍会新建 working 记录。**D8 拍板加栅栏**（步骤 5：closed→拒绝），使「关闭」成为写入终态。注意这是**写 API 语义变更**（见 §6 步骤 5、§7 D8）。
>
> 因此本工作落地后可宣称「彻底关闭**显式 `/close` 会话**的泄漏」（含写栅栏）；但**孤儿会话（边界 1，D6 划出）仍泄漏**，故仍不能宣称「彻底关闭所有泄漏」。

> ⚠️ 本文修正了此前基于记忆的判断「这是 M8 核心大块、需大量设计」。实际核对代码后：晋升逻辑、过期逻辑、schema、契约接口、单测**全部已存在**，剩余的是接线与可观测落地，规模为小到中等。

---

## 2. 现状核对（基于 `main`，2026-06-02 逐文件验证）

### 2.1 已建好的部分

| 能力 | 位置 | 状态 |
|---|---|---|
| working kind schema（CHECK 约束 + dedupe 索引表） | `llm-agent-memory-postgres/postgres/schema.go`（`HeadSchemaVersion=3`，`v3WorkingAndDedupeStatements`） | ✅ 生产已应用 |
| `Promote` / `ResolveDedupe` / `ListSessionWorking` | `llm-agent-memory-postgres/postgres/durable_ops.go:55/228/353` | ✅ |
| 契约接口 `Promoter` / `Deduper` / `RecordStore` / `AccessMarker` + kind 常量 + `NormalizeRecordKind` | `llm-agent-memory-contract/contract/durable.go:12-14,187-234` | ✅ |
| **晋升（异步、outbox 驱动）= M8b worker** | `llm-agent-memory-worker/internal/service/consolidation_publisher.go` | ✅ **已实现**（消费 `memory_created`/`memory_updated`，调 `ResolveDedupe`+`Promote`，有自己的 promote metrics） |
| **会话关闭生命周期（同步、HTTP 驱动）** | `llm-agent-memory-gateway/internal/service/durable_session_closer.go` | ✅ 逻辑+单测完整，❌ **生产未接线** |
| working kind 写入校验（CAP1） | gateway `service.go`（PR #3） | ✅ 已合并 |
| recall 命中标记 `MarkAccess`（CAP3） | gateway `service.go`（PR #4） | ✅ 已合并 |
| `WorkingLifecycleObserver` 接口 + nop 实现 | gateway `internal/service/working_lifecycle_observer.go` | ✅ 接口在，❌ 无真实 metrics 实现 |
| `Service.CloseSession` 派发（含 mode 校验） | gateway `service.go:603-657` | ✅ 已派发到 `sessionCloser`，mode 默认 `expire_working`、另支持 `promote_and_expire` |

### 2.2 缺口（生产未启用的全部）

1. **生产用的是 `noOpSessionCloser{}`**：`cmd/memory-gateway/main.go:140` 注入空实现，`NewDurableSessionCloser` 仅在测试构造。→ 会话关闭时**不做任何过期/晋升**。
2. **类型不匹配的适配器缺失**：gateway 的 `sessionWorkingStore` 接口（`durable_session_closer.go:22-27`）要求 `ListSessionWorking(...) ([]service.SessionWorkingRecord, error)`，但 `postgres.Store.ListSessionWorking` 返回的是 `[]postgres.SessionWorkingRecord`（同字段、不同类型）。`*postgres.Store` **不直接满足** gateway 接口，需要一个做类型转换的适配器。
3. **CAP2 生命周期指标不存在**：`internal/observability/metrics.go` 里**没有** `working_expired_total` / `working_dropped_before_use_total` 计数器（此前记忆误以为存在；实际只有接口与调用点）。需要新建计数器 + 一个把 `ObserveWorkingLifecycle` 映射到计数器的 `WorkingLifecycleObserver` 实现，并接到 `service.New(...)`。

### 2.3 两条晋升路径并存（核心认知）

代码里同时存在两条晋升路径，**晋升判定等价**（同源→合格映射：`user_saved` 必晋升、`agent_inferred` 当 `importance≥0.7`；同 dedupe-key 构造），但**触发与职责不同、互补不冲突**。

> **澄清（round-1 H2）**：两份代码**并非字节级一致**——函数/常量名不同（`shouldPromoteOnSessionClose`/`sessionClosePromoteImportanceThreshold` vs `shouldPromote`/`agentInferredImportanceThreshold`）、`Reason` 字符串不同（`session_close_*` vs worker 的对应值）、**幂等键盐值刻意不同**（closer 用 `"session_close_promote"`、worker 用 `"promote"`——两路径必须用不同幂等键，否则会互相冲销）。**真正必须锁步一致的只有三样**：①dedupe-key 构造（`sessionCloseDedupeKey` vs `dedupeKey`，当前值等价）②`0.7` 阈值 ③source→合格映射。D3（§7）针对的是这个**窄契约面**，不是「整份函数一致」。

| 维度 | M8b Worker（已生产） | 会话关闭器（未接线） |
|---|---|---|
| 触发 | 异步，消费 `memory_created`/`memory_updated` outbox | 同步，`POST /sessions/{id}/close` |
| 动作 | **只晋升**（working→episodic），从不删除 | `promote_and_expire`：晋升合格者 + **过期回收**其余；`expire_working`：全部过期 |
| 时机 | 写入后即时（高价值 working 记忆尽早固化） | 会话结束时统一清理 |
| 版本栅栏 | `msg.Version == current.Version` | `ExpectedVersion` + stale 容忍 |

**关键互补点**：worker **从不删除/过期** working 记录。**只有会话关闭器**提供过期回收。没有它，working-kind 记录在生产中**永久累积、永不清理**——这才是真正的生产缺口。`working_expired_total` / `working_dropped_before_use_total` 度量的正是这个回收器的工作量，所以在回收器接线前它们必然恒为零。

**为什么不会重复晋升（同一 memory_id）**：worker 已晋升的记录其 `kind` 已变为 `episodic`、`version` 已自增；`ListSessionWorking` 只返回 `kind=working`，自然不会再取到它。会话关闭器的 `Promote`/`ResolveDedupe` 又是幂等键 + 版本栅栏 + stale 容忍（`ErrVersionConflict`/`ErrNotFound` 视作「已处理」），二次尝试天然 no-op。

> ⚠️ **但「不同 memory_id、相同 dedupe_key」并发不安全（round-2 C1，正确性前置，见 §2.4）**。v2 曾断言 `FOR UPDATE` 串行化了并发 deduper——**这是错的**。下面 §2.4 单列。

---

### 2.4 正确性前置：`ResolveDedupe` first-writer 竞态（round-2 C1，HIGH）

**接线会话关闭器会放大一个 postgres 既有的并发缺陷，须先修。**

`postgres.Store.ResolveDedupe`（`durable_ops.go:235-251`）的逻辑是：`SELECT winner_memory_id ... FOR UPDATE`，若 `isNoRows` 则裸 `INSERT`。代码注释自称「FOR UPDATE 串行化并发 deduper」——**但 `FOR UPDATE` 只锁已存在的行；当 dedupe 行尚不存在时它锁不住任何东西**。两个并发事务（如 worker 处理记录 A 的 outbox 同时会话关闭处理记录 B，二者算出**相同 dedupe_key**）都看到空表 → 都走 `INSERT` → 后提交者撞主键 `(tenant_id, dedupe_key)` 唯一约束（`schema.go:302-308`）。该错误被包成 `"insert dedupe index: %w"`，**既不是 `ErrVersionConflict` 也不是 `ErrNotFound`**，因此 closer 的 `isSessionCloseStale`（`closer:194-196`）与 worker 都**不会**当 stale 吞掉 → 一次显式 close 直接报错失败、留下 active + 半清理状态。

> 这个竞态**当前已存在**（worker 是唯一调用者，但单 relay 串行降低了概率）；**接线会话关闭器引入第二个并发调用者，显著提高命中率**——所以它从「已有低概率 bug」升级为本工作的**硬前置**。

**修复方向**（postgres 仓）：M8 总纲 §4.3 step 1 本就规定 loser「应看到唯一约束冲突并读取 winner 行」——即 `ResolveDedupe` 应在 `INSERT` 上处理唯一冲突（`INSERT ... ON CONFLICT DO NOTHING` 后重查 winner，或捕获 unique violation → 重读 → 返回 `DedupeMergedExisting` 走既有 collision 分支）。**当前 postgres 实现未落实其自身 spec 的这一步**。这是一个独立的、可单独测试的 postgres 正确性修复（含并发回归测试），是会话关闭器接线的前置条件。

---

## 3. 生命周期语义

### 3.1 状态与迁移

```
                    写入(kind=working)
                          │
                          ▼
                   ┌─────────────┐
        recall ───▶│   WORKING   │◀─── 会话进行中
        命中标记    │ (会话内临时) │
      (MarkAccess) └─────────────┘
                     │    │    │
       ┌─────────────┘    │    └──────────────┐
       │ worker 即时晋升    │ 会话关闭          │ 会话关闭
       │ (memory_created/  │ promote_and_      │ expire_working
       │  updated 后,合格)  │ expire(合格者)    │ 或 promote 不合格
       ▼                   ▼                   ▼
  ┌──────────┐       ┌──────────┐        ┌──────────────┐
  │ EPISODIC │       │ EPISODIC │        │  EXPIRED     │
  │ (晋升固化)│       │ (晋升固化)│        │ (deleted=TRUE)│
  └──────────┘       └──────────┘        └──────────────┘
                                              │
                                  其中从未被 recall 命中过的
                                  (hit_count==0 || last_access_at==nil)
                                  额外计入 dropped_before_use
```

- **WORKING**：会话内临时记忆。生产中由 gateway 写入路径接受（CAP1 已落地校验）。
- **→ EPISODIC（晋升）**：满足晋升规则（§4）的记录固化为长期 episodic 记忆。可由 worker 即时触发，或由会话关闭 `promote_and_expire` 触发。晋升通过 `consolidated_from_event_id` 记录来源 event（M8 总纲 §4.1 的 provenance 约定）。
- **→ EXPIRED（过期回收）**：会话关闭时，未被晋升的 working 记录被 `DeleteRecord`（软删 `deleted=TRUE`）。
- **dropped_before_use**：过期记录的子集——从未被 recall 命中过的（`hit_count==0 || last_access_at==nil`，见 `durable_session_closer.go:190-192`）。这是一个产品质量信号：写入的 working 记忆有多少根本没被用上就被回收了。

### 3.2 会话关闭的两种 mode（已实现，`service.go:617-622`）

| mode | 行为 | 默认 |
|---|---|---|
| `expire_working` | 过期回收会话内**全部** working 记录（不晋升） | ✅ 默认 |
| `promote_and_expire` | 先按规则晋升合格者，其余过期回收 | |

> 设计立场：默认 `expire_working` 是保守选择——「会话内临时记忆默认不外溢到长期存储」。需要固化时由调用方显式传 `promote_and_expire`，或依赖 worker 的即时晋升。**这与 worker 即时晋升并不矛盾**：worker 负责会话进行中高价值记忆的尽早固化，会话关闭器负责收尾清理。

---

## 4. 晋升规则（已实现，gateway 与 worker 一致）

来源：`durable_session_closer.go:143-152`（与 worker `consolidation_publisher.go:121-130` **判定等价**的规则——见 §2.3 H2 澄清，二者并非字节级一致，但源→合格映射与 0.7 阈值相同）。

| `source` | 晋升条件 |
|---|---|
| `user_saved` | **总是**晋升 |
| `agent_inferred` | 当 `importance >= 0.7`（`sessionClosePromoteImportanceThreshold`） |
| 其他（如 `system`） | 不晋升 |

晋升前流程（`promoteIfEligible`）：
1. 规则不合格、或无 `LatestEventID` → 跳过（不晋升，进入过期路径）。
2. `ResolveDedupe`：若与现有记录碰撞或自己不是 winner → 视为已处理（不重复晋升），返回 handled。
3. `Promote`：幂等键 = `sha256(tenant || memory_id || event_id || "session_close_promote")`，`ExpectedVersion` 版本栅栏，`Reason` 记录晋升理由（`session_close_user_saved_default` / `session_close_agent_inferred_importance_threshold`）。
4. stale（`ErrVersionConflict`/`ErrNotFound`）→ 视作并发已处理，吞掉不报错。

> **规则一致性是隐性契约**：gateway 与 worker 靠两份独立代码维持上述**窄契约面**（dedupe-key 构造 + 0.7 阈值 + 源→合格映射）一致。这是一个**待评审的脆弱点**（见 §7 开放决策 D3）——这三样若在一处改、另一处忘改，会导致同步/异步晋升结果分叉（dedupe-key 分叉尤其危险：两路径会判到不同 winner）。注意幂等键盐值与 `Reason` 字符串**本就该不同**，不在此契约面内。

---

## 5. 可观测契约

### 5.1 现状

- **Trace**：`Service.CloseSession` 已无条件发 `promote_decided`（`service.go:646-652`，载荷含 tenant/user/project/session/mode）。这是会话关闭已有的唯一生命周期 trace。
- **Metric**：**无**。`WorkingLifecycleObservation`（`working_lifecycle_observer.go:5-10`）只携带 `Expired` / `DroppedBeforeUse` 两个计数，且 observer 只在 `expired>0 || droppedBeforeUse>0` 时触发（`durable_session_closer.go:79`）。

### 5.2 待落地的指标（CAP2）

**标签策略（round-1 M3 重开）**：此前曾拍板「全局无 label」，但 round-1 核对发现这与代码库惯例不一致，**重新提交评审**（见 §7 D7）。事实：gateway `metrics.go` 中无 label 的计数器**只有运营失败类**（`storageCronFailures`/`TraceDropped`，注释明确「global to the cron tick/query」）；**所有数据面计数器**（`episodicDisabled`/`episodicDeleted`/`recallReturned`/`recallSelected`/embedding 类）**都带 `tenant_bucket`**；worker 的同类 `working_promoted_total` **也分桶**（`RecordWorkingPromoted(tenantID)`）；且 `WorkingLifecycleObservation` **已携带 `TenantID`**（无 label 设计会白白丢弃它）。`working_expired_total` / `working_dropped_before_use_total` 是**每租户数据面量信号**（`dropped_before_use` 还是产品质量信号），去掉 bucket 就无法做「promoted vs expired vs dropped」的每租户比值分析、且与 `working_promoted_total` 不对称。基数论据弱（是 bucket 不是裸 tenant_id，兄弟计数器都已付此成本）。**本文倾向改为按 `tenant_bucket` 分桶**，与全栈对齐。下表指标名不变：

| 指标 | 含义 | 来源 |
|---|---|---|
| `working_expired_total` | 会话关闭回收的 working 记录总数 | `WorkingLifecycleObservation.Expired` |
| `working_dropped_before_use_total` | 其中从未被 recall 命中即被回收的数量 | `WorkingLifecycleObservation.DroppedBeforeUse` |

落地方式（沿用 `metrics.go` 既有 atomic 计数器 + `Add*` 方法 + `*Observer()` 构造器模式）：
1. `Metrics` 结构体加两个 `atomic` 计数器 + `AddWorkingExpired()` / `AddWorkingDroppedBeforeUse()` + snapshot 暴露。
2. 新增 `Metrics.WorkingLifecycleObserver()` 返回 `service.WorkingLifecycleObserver` 实现，把 `ObserveWorkingLifecycle` 映射到上述计数器。
3. 经 `service.Config` 注入，main.go 用 `NewDurableSessionCloser(adapter, metrics.WorkingLifecycleObserver())`。

### 5.3 已知可观测缺口（待评审）

**晋升数未被观测，且 promote-only 关闭完全不发 observation**。`ObserveWorkingLifecycle` 只在 `expired>0 || droppedBeforeUse>0` 时触发（`durable_session_closer.go:79`）；会话关闭中被晋升的记录（`promoteIfEligible` 返回 `handled=true` 后 `continue`，`closer:62-64`）**不计入** observation——`WorkingLifecycleObservation` 没有 `Promoted` 字段。**后果**：一次 `promote_and_expire` 若晋升 5 条、过期 0 条，则 observer **一次都不触发**（不只是「晋升数缺失」，而是整条 observation 为空）。worker 侧有自己的 `RecordWorkingPromoted`，但那是**另一条代码路径**、不覆盖会话关闭路径的晋升。若要补，需同时：①扩展 observation 加 `Promoted int` + `working_promoted_total`（会话关闭路径）②**修改 `closer:79` 的触发条件**使 promote-only 关闭也发 observation（见 §7 决策 D2）。

---

## 6. 待启用工作分解

按依赖顺序，TDD 红→绿，每块原子提交：

0. **✅ 已完成【postgres 仓】修 `ResolveDedupe` first-writer 竞态**（round-2 C1，§2.4）：改为原子 `INSERT ... ON CONFLICT DO NOTHING RETURNING` 认领；冲突则重读 winner 走既有 collision 分支。并发回归测试（同 dedupe_key 双并发）5/5 红 → 8/8 绿。**postgres PR #2 已合入 main（merge `6793bac`）**。详细落地计划见 [`superpowers/plans/2026-06-02-m8-working-memory-lifecycle-implementation.md`](./superpowers/plans/2026-06-02-m8-working-memory-lifecycle-implementation.md)。

1. **【D3，contract 仓】单一事实源**：在 `llm-agent-memory-contract` 新增 `PromotionPolicy`（source→合格 + `0.7` 阈值）+ dedupe-key 构造函数（含 normalize）。契约**版本 bump + tag**（M8 总纲 §4.7 lockstep）。
2. **【D3 消费，worker + gateway】**：重构 worker `consolidation_publisher.go` 与 gateway `durable_session_closer.go` 改调 contract 共享函数，**删除两份重复的 `shouldPromote`/dedupe-key 逻辑**；两仓 go.mod 升级到新 contract tag（lockstep）。加跨仓行为一致性测试。
3. **【gateway】适配器**（约 30 行 + 单测）：新建 `sessionWorkingStore` 适配器，包裹 `*pgmemory.Store`，提供 `RecordStore`+`Promoter`+`Deduper`（直接转发），把 `ListSessionWorking` 的 `[]postgres.SessionWorkingRecord` 转为 `[]service.SessionWorkingRecord`。
4. **【gateway，D7】指标**：§5.2 三步——计数器 + observer 实现 + 注入；**复用 `service.TenantBucket` 按 `tenant_bucket` 分桶**、统一空值规则。先于接线落地。
5. **【gateway，D8】写栅栏**：`WriteMemory`/`PatchMemory`/`PinMemory`/`DeleteMemory` 加 `validateSessionState`（closed→拒绝）；**更新 `memory-gateway-api-contract.zh-CN.md`**（closed session 写入语义从静默成功改 4xx）；测试覆盖 closed→4xx 与迟到写入语义。
6. **【gateway】接线 closer**（1 行 + 集成测试）：`main.go:140` 的 `noOpSessionCloser{}` → `service.NewDurableSessionCloser(adapter, metrics.WorkingLifecycleObserver())`。依赖步骤 0/3/4（与步骤 2 的共享 policy）。集成测试覆盖两 mode + read-only 拒绝 + already-closed 重放。
7. **【gateway，D2】promote 可观测**：observation 加 `Promoted` + `working_promoted_total` + 改 `closer:79` 触发条件使 promote-only 关闭也发 observation。

**依赖与 lockstep**：步骤 0（postgres）独立、可先 tag。步骤 1（contract）→ 步骤 2（worker+gateway 消费）须 lockstep。步骤 3/4/5 互相独立（纯 gateway）。步骤 6 依赖 0/3/4（+2）。步骤 7 在 6 之后。**tag 顺序**：postgres → contract → (worker + gateway bump 依赖) → gateway 终态。

**规模评估**：中—大（拍板后再上修）。无 schema 变更，但**远非「纯激活」**——含：① postgres 并发正确性修复（C1）② **契约接口新增 + 版本 bump**（D3）③ worker + gateway 消费重构 ④ **写 API 语义变更**（D8）⑤ gateway 适配/可观测/接线。改动落在 **contract + postgres + worker + gateway 四仓**，需多次 lockstep tag。约 7 个工作块、跨 4 仓、含 2 处契约/API 级变更——**这已是 milestone 量级的协调工作，§9 据此重估**。

---

## 7. 决策（已拍板 2026-06-02）

> 两轮评审完成后由用户拍板。下表为结论，其后为各项 rationale。**注意：D3、D8 取了较重选项，使范围从「单仓激活」扩为「4 仓 lockstep + 契约版本 + 写 API 语义变更」——§6/§9 已据此重算。**

| 决策 | 结论 | 范围影响 |
|---|---|---|
| **D1** 默认 mode | **`expire_working`**（不改） | 无 |
| **D2** 会话关闭晋升可观测 | **补**（加 `Promoted` + `working_promoted_total` + 改 `closer:79` 触发条件） | gateway，小 |
| **D3** 防分叉 | **下沉到 contract 单一事实源**（`PromotionPolicy` + dedupe-key 构造） | **contract（新增）+ worker + gateway 消费，3 仓 lockstep，契约版本 bump** |
| **D4** 过期删除 | **软删**（硬删 GC 留独立运维项） | 无 |
| **D5** 部分失败恢复 | **文档化恢复路径 + 补失败路径 trace/测试** | gateway，小 |
| **D6** 孤儿会话 | **本轮显式划出**（不做 idle-reaper） | 无（§1 边界 1 保留） |
| **D7** 标签 | **按 `tenant_bucket` 分桶 + 统一空值规则** | gateway，小 |
| **D8** 写栅栏 | **加**（`WriteMemory` 等 closed→拒绝） | **gateway mutator + 写 API 语义变更（静默成功→4xx）+ api-contract 文档** |

各项 rationale：

- **D1 默认 mode 是否改？** 现默认 `expire_working`（会话临时记忆默认不外溢）。考虑到 worker 已即时晋升高价值记忆，会话关闭再默认只过期是自洽的。**建议保持 `expire_working`**，固化交给 worker + 显式 `promote_and_expire`。
- **D2 是否补「会话关闭晋升数」可观测？** §5.3 缺口。**建议补**：①加 `Promoted int` + `working_promoted_total`（会话关闭路径）②同时改 `closer:79` 触发条件，否则 promote-only 关闭整条 observation 为空。属可选第 4 步。
- **D3 晋升窄契约面如何防分叉？**（round-1 H2 重新界定）真正必须锁步的只有三样：**dedupe-key 构造 + 0.7 阈值 + source→合格映射**（**不是**整份函数，幂等键盐值/`Reason` 本就该不同）。其中 **dedupe-key 分叉最危险**（两路径判到不同 winner）。**待评审**：是否把这三样下沉到 `llm-agent-memory-contract` 做单一事实源（契约层提供 `PromotionPolicy` + dedupe-key 构造函数），还是接受双份 + 加跨仓一致性测试（类似 umbrella 的 regex-parity gate）。下沉涉契约版本，需 M8 总纲 §4.7 lockstep 评估。
- **D4 过期是软删还是别的？** 现实现 `DeleteRecord`→`deleted=TRUE`（软删）。软删后记录仍占行、不释放（与 D6 叠加：孤儿会话连软删都没有）。working 记忆是否需要后续硬删 GC？M8 总纲未覆盖。**建议**本轮维持软删，硬删 GC 留作独立运维议题。
- **D5 会话关闭部分失败的恢复语义。** `CloseSession` 先调 closer 再 `sessionRegistry.Close`（`service.go:636-641`）；closer 中途失败则会话留在 `active`，重试重跑 `ListSessionWorking`（记录更少）并重新晋升/过期。幂等键 + stale 容忍（`closer:102,120,135`）保证**无重复晋升/重复计数**。但 round-1 M4 补充两点须在实现计划写明并测试：①**半回收的 active 会话窗口**——失败到重试之间，部分记录已软删/晋升，客户端仍可向这个半清理会话读写；②**失败路径无 trace**——`promote_decided` 只在 closer + registry.Close **都成功**后才发（`service.go:646`），失败路径可观测盲区，需补失败计数/日志。
- **D6（round-1 H1，新增）孤儿会话回收。** 见 §1 边界：从不显式 `/close` 的会话其 working 记录永不回收，`SessionIdleTTL` 不触发关闭。**这是本工作之外、但与「关闭泄漏」直接相关的缺口**。**待评审**：本轮是否纳入一个 idle-session 清扫器（cron 扫 idle 会话 → 调 closer），还是显式划出范围、留作独立后续项。**建议**本轮显式划出（先把主路径接线 + 可观测落地），D6 单列后续，避免范围蔓延；但 §8 验收**不得**宣称「彻底关闭泄漏」。
- **D7（round-1 M3 + round-2 C3）生命周期计数器的标签策略。** 见 §5.2：此前「全局无 label」与代码库惯例不一致（所有数据面计数器 + worker 的 `working_promoted_total` 都带 `tenant_bucket`，observation 已携带 `TenantID`）。**建议改为按 `tenant_bucket` 分桶**与全栈对齐。**额外（C3）**：空租户分桶规则两仓不一致——gateway `tenantBucket("")=="unknown"`（`tenant_bucket.go:21`），worker `TenantBucket("")=="00"`（worker `metrics.go:102`）；非空租户两仓一致（同 fnv32a %32）。若分桶，须**统一空值规则**（建议 gateway 复用既有 `service.TenantBucket`，并在设计里声明这些路径 `tenant_id` 不应为空——authz 已要求 tenant，空值仅理论边界）。**待评审拍板**（推翻先前决策，需明确确认）。
- **D8（round-2 C2，新增）close 是否应栅栏后续写入？** 见 §1 边界 2。**待评审**：是否在 `WriteMemory` 等 mutator 加 `validateSessionState`（closed→拒绝），使「关闭」成为写入终态、回收可靠。**权衡**：加栅栏会改变写 API 语义（closed session 写入从「静默成功」变 4xx），可能影响现有客户端契约（见 `memory-gateway-api-contract`），且需考虑「关闭后合法的迟到写入」是否存在。**建议**：本轮**至少**在 §8 验收里明确「关闭后写入」的既有行为（写测试固化现状），是否加栅栏作为 D8 单独拍板——若不加，§1 的保守措辞必须保留。

---

## 8. 验收标准

1. 生产 `cmd/memory-gateway` 二进制中，**显式 `/close`** 的会话其 `CloseSession` 真正执行过期/晋升（不再是 no-op）；集成测试覆盖 `expire_working` 与 `promote_and_expire` 两条 mode。**孤儿会话回收不在本轮验收内**（D6，显式划出）——验收**不得**宣称「彻底关闭泄漏」，只声明「关闭显式 `/close` 路径的泄漏」。
2. `working_expired_total` / `working_dropped_before_use_total` 在 `/metrics` 暴露，且在有 working 记录被回收时非零（端到端验证「死指标转活」）；标签按 D7 结论实现。
3. 已晋升记录不被会话关闭二次晋升（幂等性测试）；worker 与会话关闭器对同一记录无冲突（含 TOCTOU：worker 在 closer list 与 promote 之间晋升 → `ExpectedVersion` 栅栏吞掉）。
3b. **（步骤 0 前置）`ResolveDedupe` 并发回归**：两个并发调用、相同 `dedupe_key`、空 dedupe 表 → 一方成为 winner、另一方走 collision 分支返回，**无 unique-violation 报错冒泡**（round-2 C1）。
3c. **关闭后写入的既有行为被测试固化**（round-2 C2）：向已 `closed` 会话写入——记录当前行为（成功并新建 working 记录 / 或若 D8 决定加栅栏则 4xx）；§1 措辞与该测试结论一致。
4. **既有保护回归**（round-1 L8）：read-only gateway 下 `CloseSession` 经 `ensureWritable` 拒绝、closer 不跑；already-closed 会话重放走短路（`service.go:627-635`）不二次晋升/不重复发 trace——两者各有测试。
5. `GOWORK=off go build ./... && go vet ./... && go test ./...` 在 gateway 仓全绿。
6. D1–D7 在评审中各有明确落地（采纳/拒绝/延后），无静默遗漏。

---

## 9. 下一步建议

**拍板结果改变了路线判断。** D3（契约下沉）+ D8（写栅栏）都取了重选项，使工作从「双仓激活 + 一处修复」升级为**四仓 lockstep（contract + postgres + worker + gateway）、含契约版本 bump 与写 API 语义变更**的协调工作（§6 七个工作块、§7 表）。这已越过「轻量直接实现」的阈值。

- **修正建议：分两段推进，不一次性 plan-execute。**
  - **第一段（立即、无需更多决策）：步骤 0 — postgres C1 修复。** 它是独立正确性 bug、不依赖任何 D 决策，可作为单独 PR 立即合入并 tag。**这也修复了一个当前生产已存在的并发隐患**（不止是本特性的前置）。
  - **第二段（D3+D8 升级后的主体）：步骤 1-7。** 因含契约版本 + 4 仓 lockstep + 写 API 语义变更，**建议按 M8 子里程碑规格的两轮评审范式正式立项**——要么走一份完整实现计划（参照 `docs/superpowers/specs/` 既有 M8 sub-spec 形态 + 两轮评审），要么 `/gsd-new-project` 为 memory 集群 bootstrap GSD 后正式排期。理由：契约/API 级变更 + 多仓 lockstep 正是「需里程碑历史 + 多阶段规划」的场景，此时 GSD/正式规格**不再是过度设计**。
- **不建议**：把步骤 1-7 当「轻量计划」一把梭——D8 改写 API 语义、D3 动契约版本，任一出错都是跨仓回滚。
- **两轮评审 ✅ + D1–D8 已拍板 ✅ + 步骤 0 已落地 ✅**（postgres PR #2 merged）。**实现计划已成文**：[`superpowers/plans/2026-06-02-m8-working-memory-lifecycle-implementation.md`](./superpowers/plans/2026-06-02-m8-working-memory-lifecycle-implementation.md)（步骤 1-7 的 per-step 文件/测试/lockstep tag 序列）。步骤 1-7 待执行（4 仓 lockstep，建议逐步评审、一次一个 PR）。
- 若评审中 D3 决定下沉到契约层，则范围扩大到 contract + postgres + worker + gateway 四仓 lockstep，届时再评估是否值得正式排期。
- 若 D6 决定本轮纳入 idle-session 清扫器，则工作从「单仓激活」升级为「含后台清扫的子特性」，规模与排期需重估。

---

## 关联文档

- [`./superpowers/specs/2026-05-27-m8-umbrella-design.md`](./superpowers/specs/2026-05-27-m8-umbrella-design.md) — M8 拆分总纲（本文是其 worker 异步晋升之外、未单独立项的「会话同步生命周期」一块）
- [`./memory-roadmap.zh-CN.md`](./memory-roadmap.zh-CN.md) — memory 里程碑路线
- [`./memory-gateway-api-contract.zh-CN.md`](./memory-gateway-api-contract.zh-CN.md) — gateway API 契约（含 `POST /sessions/{id}/close`）
