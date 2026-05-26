# 记忆子系统优化路线图：`llm-agent`

> 文档版本：2026-05-25
> 对应代码快照：2026-05-25
> 范围：`llm-agent/memory/` 与其和 `context/` 的集成边界
> 关联深读：`docs/source-design-llm-agent.zh-CN.md`

---

## 1. 背景与边界

当前 `llm-agent` 的记忆能力是一个 **进程内、SDK 风格、三层分级** 的子系
统，而不是一个独立的 memory service。

现状的几个关键事实：

- 三类记忆统一走 `Memory` 接口：`Working`、`Episodic`、`Semantic`
  （`llm-agent/memory/memory.go:24-75`）。
- 三类记忆共享同一个 `scoredStore`，底层是 `map[item] + map[vector]`
  的进程内索引（`llm-agent/memory/internal_score.go:14-25`）。
- `Manager` 负责路由、`SearchAll`、`ListAll`、`Consolidate`、`Forget`
  等协调动作（`llm-agent/memory/manager.go:12-35`
  `llm-agent/memory/manager.go:96-149`
  `llm-agent/memory/manager.go:184-243`）。
- `ScopedManager` 只在 CRUD / Search / List 路径上做 scope 约束；
  `Consolidate`、`Forget`、`StatsAll` 明确不 honor scope
  （`llm-agent/memory/scoped_manager.go:12-17`
  `llm-agent/memory/scoped_manager.go:128-144`）。
- `context.Builder` 只把记忆结果当作 `MemoryHits` 输入消费，并不拥有记忆
  本身（`llm-agent/context/builder.go:48-56`）。

因此，这份路线图的目标不是把当前实现“直接改造成分布式向量数据库”，而是：

1. 先补齐当前模型下最危险的正确性缺口。
2. 再改善召回质量、观测性和扩展性。
3. 最后在 `v1/v2` 窗口做真正的抽象与存储层重构。

---

## 2. 当前设计判断

### 2.1 当前设计的优点

- 接口小，核心模型清晰：`MemoryItem + Memory + Manager` 的主线很稳
  （`llm-agent/memory/memory.go:33-72`）。
- profile / pin / disable / scope 都叠加在 `Metadata` 上，不破坏原始
  接口（`llm-agent/memory/profile.go:3-54`
  `llm-agent/memory/scope.go:69-114`）。
- Snapshot 持久化是可插拔的，核心保持 stdlib-only
  （`llm-agent/memory/persistence.go:15-18`
  `llm-agent/memory/persistence.go:167-176`）。
- 记忆可以通过统一 tool surface 暴露给 agent，而不是要求上层直接依赖
  concrete type（`llm-agent/memory/tool.go:14-29`）。

### 2.2 当前设计的主要问题

- scope 不是真正的一等租户边界，只是 decorator 过滤器
  （`llm-agent/memory/scoped_manager.go:7-17`）。
- `Consolidate` 是 copy，不是 move，也没有 promote 去重标记，长期容易重复
  提升同一条记忆（`llm-agent/memory/manager.go:178-208`）。
- `SearchAll` 返回的是“按 kind 分桶的结果”，不是统一融合后的 recall
  结果（`llm-agent/memory/manager.go:96-115`）。
- `scoredStore` 用单把 `sync.Mutex`，并且 `snapshot()` 每次深拷贝 item +
  vector；`Episodic` 又无上限，扩展性上限很明确
  （`llm-agent/memory/internal_score.go:18-25`
  `llm-agent/memory/internal_score.go:121-137`
  `llm-agent/memory/episodic.go:10-18`）。
- `WorkingMemory.Add` 在 eviction 路径上会再次 embed probe，写放大明显
  （`llm-agent/memory/working.go:54-63`
  `llm-agent/memory/working.go:166-197`）。

---

## 3. 路线图原则

本路线图分成两类：

- **`v0.x` 可加法改动**：不破坏现有 exported API，不改变既有调用者的默认
  行为。
- **`v1/v2` breaking change**：改造抽象边界、注入模型和底层存储结构。

执行顺序遵循三个原则：

1. 先 correctness，后 performance。
2. 先把“当前抽象下做得对”补齐，再讨论“换抽象”。
3. `Working` 可以继续保持进程内优化，`Episodic/Semantic` 再逐步外部化。

---

## 4. `v0.x` 可加法改动

### 4.1 Phase A：正确性与隔离补齐

#### A-1. 增加 scope-aware 生命周期接口

**问题**

`ScopedManager` 的 `Consolidate`、`Forget`、`StatsAll` 目前全部直接透传到底
 层 `Manager`，明确不按 scope 生效
（`llm-agent/memory/scoped_manager.go:128-144`）。

**建议**

- 保留现有方法语义不变。
- 新增：
  - `ConsolidateScoped(ctx, opts)`
  - `ForgetScoped(ctx, kind, opts)`
  - `StatsScoped(ctx)`
- 非零 scope 时，只作用于匹配 scope 的 item。

**收益**

- 不破坏旧调用方。
- 为多租户 / 多会话使用场景补上真正可依赖的 lifecycle 入口。

**风险**

- scope 过滤需要枚举底层 store，仍然会沿用当前 snapshot 成本。
- 旧调用方如果继续使用原方法，跨 scope 行为仍然存在，需要文档明确。

**验收标准**

- 新增单测覆盖：
  - 不同 `User/Project/Session` 之间不能互相 `Forget`。
  - scoped consolidate 不能把别的 scope 的 working item promote 到 episodic。
- 旧 API 行为保持不变。

#### A-2. 为 consolidate 增加 promote 去重标记

**问题**

当前 `Consolidate` 只按 `Importance` 和 `MinAge` 判断，然后 copy 到
 `Episodic`，不会修改源 item，也不会记录“已经 promote 过”
（`llm-agent/memory/manager.go:191-206`）。

**建议**

- 在 `Metadata` 中增加保留键，例如：
  - `_consolidated_at`
  - `_promoted_from`
  - `_promotion_count`
- 新增可选策略：
  - 仅提升一次
  - 冷却窗口后允许再次提升

**收益**

- 立刻降低 episodic 膨胀和重复 recall。
- 不改变 `MemoryItem` 结构，属于纯加法。

**风险**

- 需要定义 metadata 兼容语义，避免和调用方自定义 key 冲突。

**验收标准**

- 相同 working item 连续两次执行 consolidate，episodic 不再重复插入。
- restore / export / import 后 promote 元数据能正确 round-trip。

#### A-3. 增加统一召回接口 `SearchUnified`

**问题**

当前 `SearchAll` 只返回 `map[Kind][]SearchResult`，它更像调试接口，不是最终
 recall 接口（`llm-agent/memory/manager.go:96-115`）。

**建议**

- 新增 `SearchUnified(ctx, query, topK)`。
- 先保守实现：
  - fan-out 到三类 memory
  - 合并结果
  - 支持按 `ID + Content` 做基础去重
  - 输出统一排序后的 `[]SearchResult`

**收益**

- 给 `context.Builder` 提供更合理的上游输入。
- 避免每个业务方重复写 merge 逻辑。

**风险**

- 三类 memory 的 score 不是同一量纲，初版需要明确定义“只做启发式 merge”。

**验收标准**

- unified 结果数量 obey `topK`。
- 同一条内容在多个层级命中时，只返回一条合并结果。
- 旧的 `SearchAll` 保持可用。

### 4.2 Phase B：观测性与 write-path 优化

#### B-1. 为 memory 操作补观测点

**问题**

当前 memory 几乎没有自己的指标抽象。Add/Search/Consolidate/Forget 的耗时、
 命中数、丢弃数都不可见。

**建议**

- 增加可选 observer / hook，不强绑 OTel。
- 最小可观测集：
  - `memory_add_total`
  - `memory_search_total`
  - `memory_search_hits`
  - `memory_consolidated_total`
  - `memory_forgotten_total`
  - `memory_snapshot_items`
  - `memory_snapshot_vectors_bytes`

**收益**

- 不改变核心 portability。
- 为后续性能优化提供数据基础。

**风险**

- 如果直接内嵌 metrics 依赖，会破坏当前“装饰器优先”的设计哲学。

**验收标准**

- 在不配置 observer 时零行为变化。
- 在启用 observer 时，Add/Search/Consolidate/Forget 均能产出事件。

#### B-2. 复用 Working eviction 的 query embedding

**问题**

`WorkingMemory.Add` 之后的 `evictIfOverCapacity` 会再次 embed probe 文本
（`llm-agent/memory/working.go:56-63`
 `llm-agent/memory/working.go:179-185`）。

**建议**

- 在 Add 路径里复用已生成的 embedding，避免 probe 文本再 embed 一次。
- 如果想避免改底层签名，可先把“最近一次 Add 的 vector”作为临时局部变量向下
  传递。

**收益**

- 对慢 embedder 或远程 embedder 直接降低写延迟。

**风险**

- 若实现不慎，容易把“写入 embedding”和“淘汰评分 embedding”耦合过深。

**验收标准**

- Add + eviction 的 embed 次数从 2 次降到 1 次。
- 行为测试保持一致：容量满时仍然淘汰最低分项。

#### B-3. 并行化 `SearchAll` / `ListAll`

**问题**

当前 `SearchAll` / `ListAll` 是顺序 fan-out
（`llm-agent/memory/manager.go:96-149`）。

**建议**

- 在不改返回结构的前提下并行查询三类 memory。
- 保持 fail-fast 语义与当前一致。

**收益**

- 是最小成本的延迟优化。
- 不需要先碰底层存储结构。

**风险**

- 底层仍然是单 mutex store，收益受限；但它至少能并行三个独立 memory。

**验收标准**

- API 行为和错误语义与串行版本一致。
- benchmark 能看到多 memory 查询的 wall time 下降。

### 4.3 Phase C：策略层与持久化能力补强

#### C-1. 抽象写入策略接口

**问题**

当前“记不记、记到哪一层、importance 给多少”主要靠调用方自己决定。
`Sanitizer` 只能做 Add-time reject / redact，不能表达完整 write policy
（`llm-agent/memory/policy_hook.go:8-24`
 `llm-agent/memory/policy_hook.go:67-79`）。

**建议**

- 新增显式策略接口，例如：
  - `Decide(ctx, input) -> {kind, importance, tags, keep}`
- 让上层业务可以把“remember / infer / ignore”的逻辑集中化。

**收益**

- 把 memory 从“存储组件”推进为“可治理的记忆入口”。

**风险**

- 若策略接口设计过大，会提前把业务 DSL 拉进核心仓。

**验收标准**

- 可以只通过策略接口完成：
  - user-saved memory 写入
  - agent-inferred memory 写入
  - reject / redact 决策

#### C-2. 丰富官方 SnapshotStore 实现

**问题**

当前核心只有 `FilesystemStore`
（`llm-agent/memory/persistence.go:178-247`）。

**建议**

- 核心保持 stdlib-only，不在本仓引入 DB 依赖。
- 在 sibling repo 提供官方 `SQLiteStore` 或 `PostgresStore` 参考实现。

**收益**

- 保留核心仓边界，同时给服务化接入方一个官方路径。

**风险**

- 如果过早把外部 store 语义写死，会影响后续 v2 外部索引化。

**验收标准**

- 至少提供一套非文件系统实现，支持 Save/Load/Delete/List。
- 可以和现有 `ImportAll` / `ExportAll` 无缝对接
  （`llm-agent/memory/manager.go:291-379`）。

---

## 5. `v1` 需要 breaking change 的改动

### 5.1 Phase D：抽象边界拉正

#### D-1. `ManagerOptions` 从 concrete type 改为 capability interface

**问题**

当前 `ManagerOptions` 直接依赖 `*WorkingMemory`、`*EpisodicMemory`、
`*SemanticMemory`（`llm-agent/memory/manager.go:22-35`）。

这会带来两个问题：

- 装饰器无法自然嵌入，例如 `WithSanitizer` 返回的是 `Memory` 接口，不是
  concrete type（`llm-agent/memory/policy_hook.go:37-45`
  `llm-agent/memory/policy_hook.go:53-60`）。
- 外部 memory backend 很难直接挂入 Manager。

**建议**

- `v1` 时把 `ManagerOptions` 改成接口注入。
- capability 分层建议：
  - `Memory`
  - `Lister`
  - `Exporter`
  - `Importer`
  - 可选 `LifecycleMemory` 或同类接口

**收益**

- 真正打开 decorator 组合和 remote-backed memory 的入口。

**风险**

- 这是使用面很广的 breaking change，必须配迁移指南。

**验收标准**

- `WithSanitizer` 包装后的 memory 可直接放进 `Manager`。
- 外部实现只要满足接口即可挂接，不需要伪装成 core concrete type。

#### D-2. 把 recall 从“三类记忆并排”提升为统一抽象

**问题**

目前对上层暴露的是 `KindWorking/KindEpisodic/KindSemantic` 这三个物理层次
 （`llm-agent/memory/memory.go:24-31`）。

**建议**

- 在 `v1` 引入统一 recall facade，例如 `RecallEngine`。
- working / episodic / semantic 退为内部策略或 tier。

**收益**

- 上层不再需要知道“该搜哪一层”。
- 更符合最终产品语义：“召回我需要的记忆”，而不是“手动查三个桶”。

**风险**

- 如果保留旧 API，短期会出现双抽象共存。

**验收标准**

- `context.Builder` 的 memory 接入可以直接消费 recall 结果，而不是手工拼接
  `[]memory.SearchResult`（`llm-agent/context/builder.go:48-56`）。

---

## 6. `v2` 需要 breaking change 的改动

### 6.1 Phase E：存储与多租户模型重构

#### E-1. 替换 `scoredStore` 的并发与读模型

**问题**

当前 `scoredStore` 使用单把 `sync.Mutex`，并依赖全量 `snapshot()` 深拷贝
 提供读隔离（`llm-agent/memory/internal_score.go:18-25`
 `llm-agent/memory/internal_score.go:121-137`）。

**建议**

- `v2` 选择其一：
  - `RWMutex + 只读迭代视图`
  - immutable read view + CoW
  - 按 kind 或 shard 分片

**收益**

- 这是 episodic/semantic 走向大规模数据的前提。

**风险**

- 会改变读写一致性与返回值可变性假设。

**验收标准**

- 大量 episodic item 下，Search/Stats 不再产生 O(n) vector 深拷贝。
- 并发读性能明显提升。

#### E-2. 让 `Episodic/Semantic` 支持外部索引后端

**问题**

当前三类 memory 都默认绑定进程内 `scoredStore`
（`llm-agent/memory/working.go:18-20`
 `llm-agent/memory/episodic.go:16-18`
 `llm-agent/memory/semantic.go:17-19`）。

**建议**

- `Working` 继续偏本地、低延迟。
- `Episodic/Semantic` 可切换到 pgvector / Milvus / Qdrant 等外部索引。

**收益**

- 分层职责终于和物理部署形态对齐。

**风险**

- score normalization、分页语义、import/export 语义都会变复杂。

**验收标准**

- 同一个 `RecallEngine` 可以同时挂接本地 working 与远程 episodic/semantic。
- 对上层调用方保持尽量一致的召回接口。

#### E-3. 把 scope 从 decorator 升级为一等主键维度

**问题**

当前 scope 是通过 `context.Context` 传入，再由 `ScopedManager` 在结果侧过滤
 （`llm-agent/memory/scope.go:49-67`
 `llm-agent/memory/scoped_manager.go:41-125`）。

**建议**

- `v2` 把 scope 纳入正式存储键或 partition key。
- scope 不再是“查询时附加过滤器”，而是 item 的主索引维度之一。

**收益**

- 真正的多租户边界。
- 生命周期操作、导出导入、清理、统计都能天然按 scope 生效。

**风险**

- 这是最重的一类迁移，需要考虑 legacy unscoped data 的迁移路径。

**验收标准**

- 不存在“先查全局、再过滤 scope”的路径。
- 全部 lifecycle 操作天然支持 scope 约束。

#### E-4. 正规化 `MemoryItem` schema

**问题**

现在 `Source/Category/Pinned/Disabled/Scope` 都依赖 `Metadata map[string]any`
（`llm-agent/memory/memory.go:36-44`
 `llm-agent/memory/profile.go:45-54`
 `llm-agent/memory/scope.go:71-114`）。

**建议**

- `v2` 将稳定字段升级为一等 schema。
- `Metadata` 保留给真正开放扩展。

**收益**

- 类型安全、索引能力和外部后端映射都会更好。

**风险**

- 会直接影响 snapshot 结构与导入导出兼容性。

**验收标准**

- 常用字段不再依赖 `map[string]any` 的动态断言。
- Snapshot version 升级后有明确 migration 文档。

---

## 7. 分阶段风险总表

| 阶段 | 主要风险 | 风险级别 | 缓解方式 |
|---|---|---|---|
| Phase A | 新增 scoped lifecycle 但旧 API 仍保留，调用方可能继续误用旧方法 | 中 | 文档明确标注；tool 层优先切到 scoped 版本 |
| Phase B | 加观测点时破坏 portability 或引入额外依赖 | 中 | 只暴露 hook/interface，不直接绑具体 metrics SDK |
| Phase C | write policy 设计过大，变成业务规则引擎 | 中 | 先只覆盖 remember / infer / reject 三类基础决策 |
| Phase D | `ManagerOptions` breaking 面较广 | 高 | 配迁移指南、保留一版 compatibility shim |
| Phase E | 底层存储重构过重，影响 Snapshot / Scope / Score 三条主线 | 高 | 拆成多个小 milestone，先抽象后换引擎 |

---

## 8. 建议执行顺序

### 8.1 推荐落地次序

1. `Phase A`
2. `Phase B`
3. `Phase C`
4. `Phase D`
5. `Phase E`

### 8.2 原因

- `Phase A` 先补 correctness，尤其是 scope 边界和 consolidate 去重。
- `Phase B` 用最小成本拿到观测与写路径收益。
- `Phase C` 让记忆写入逻辑显式化，为后续外部化做准备。
- `Phase D` 处理抽象边界，这是所有 v2 存储演化的前置条件。
- `Phase E` 最后处理底层引擎和 schema，否则会过早复杂化。

---

## 9. 最小可执行版本

如果只允许做一轮短周期改进，建议只交付下面 5 项：

1. `ConsolidateScoped / ForgetScoped / StatsScoped`
2. consolidate 去重 metadata
3. `SearchUnified`
4. memory observer hooks
5. Working eviction embedding 复用

这 5 项都属于 `v0.x` 加法改动，且能同时改善：

- 多租户正确性
- episodic 膨胀
- 统一召回质量
- 可观测性
- 写入延迟

---

## 10. 验收清单

```
[ ] scoped lifecycle API 已补齐，且跨 scope 误删/误统计有测试覆盖
[ ] consolidate 已有去重或冷却策略，不再重复 promote 同一 item
[ ] unified recall 已存在，且可直接供 context 组装层消费
[ ] memory 关键操作已具备可选观测点
[ ] Working eviction 不再重复 embed probe
[ ] v1 breaking 方案已明确迁移路径
[ ] v2 存储重构前置抽象已收敛，而不是直接替换实现
```

## 11. 与多服务落地的优先级映射

这份路线图聚焦当前 `llm-agent/memory` SDK 本体，但如果目标是走向多服务
memory 平台，建议把需求再按上线优先级拆成三层：

### 11.1 P0：必须先做

- `Scope` 升级为正式 `tenant/user/project/session` 维度
- 长期写入口统一化：
  - `Memory Gateway`
  - `Postgres` 真相源
  - `Transactional Outbox`
- `idempotency_key`
- `expected_version`
- `WorkingMemory` 与长期层职责切分
- 最小验证指标：
  - recall 是否进入 prompt
  - promote 是否被后续再次命中
  - working 生命周期是否过短或过长
  - embedding 成本是否可接受
- token-aware 贯通：
  - recall 返回结果要能被 prompt budget 约束
- session 生命周期闭环：
  - close / expire / promote-and-expire
- decision trace 基础字段：
  - recalled
  - selected
  - dropped
  - promote accepted/rejected
  - drop_reason
  - promote_reason

### 11.2 P1：功能上线前补齐

- `Embedding / Consolidation / Index Sync` worker
- consolidate 判重前两层：
  - `memory_id`
  - `tenant_id + user_id + category + normalized_content_hash`
- 失败状态机
- 简单衰减模型：
  - recency
  - recalled-again / not-recalled
  - pinned / user_saved 降速衰减

说明：

- `singleflight`
- L1/L2 recall cache

这些都不是“验证记忆是否有用”的前置条件。若当前阶段还无法证明 recall quality
和 promote 策略有效，应避免先把复杂缓存层做满。

### 11.3 P2：灰度后增强

- 向量相似判重
- 更复杂的 merge / rerank
- `singleflight` 防 recall 热点击穿
- L1/L2 recall cache
- 提前刷新热 key
- 更强的 `Level 2 / Level 3` 一致性优化
- learned salience / temporal rerank 模型

### 11.4 Ready-to-Code 最小集

如果现在进入实现阶段，最小闭环建议限定为：

1. `POST /memory/write`
2. `POST /memory/recall/unified`
3. `PATCH /memory/items/{memory_id}`
4. `POST /memory/items/{memory_id}/pin`
5. `POST /memory/items/{memory_id}/disable`
6. `DELETE /memory/items/{memory_id}`
7. `memory_record + memory_event + outbox_event`
8. `Consolidation Worker` 幂等 promote
9. 跨租户隔离测试
10. Recall / Promote / 生命周期 / 成本 四类验证指标

这部分的详细约束以 [`multi-service-memory-architecture.zh-CN.md`](./multi-service-memory-architecture.zh-CN.md) 和 [`memory-gateway-api-contract.zh-CN.md`](./memory-gateway-api-contract.zh-CN.md) 为准。

最小验证口径建议固定为：

- Recall Quality：
  - returned
  - selected
  - helpful
- Promote：
  - attempt
  - accepted
  - recalled-again
- Decision Trace：
  - drop_reason
  - promote_reason
- 生命周期：
  - working expired
  - dropped before use
  - stale hit
- 成本：
  - embedding requests
  - embedding cost
  - storage bytes

补充约束：

- 不要把“最优 prompt 记忆选择”当作 v1 基础设施完成条件
- v1 先解决：
  - tenant 安全
  - promote 幂等
  - recall 可解释性
  - embedding 成本可控
  - 记忆是否真的对 prompt 有用
  - token budget 是否真正约束了 recall 输出

---

## 延伸阅读

- [`source-design-llm-agent.zh-CN.md`](./source-design-llm-agent.zh-CN.md)
- [`current-project-analysis.zh-CN.md`](./current-project-analysis.zh-CN.md)
- [`refactor-and-optimization-roadmap.zh-CN.md`](./refactor-and-optimization-roadmap.zh-CN.md)
