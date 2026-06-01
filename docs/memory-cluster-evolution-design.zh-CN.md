# 记忆能力从本地到集群的演进设计（含两轮评审）

> 文档版本：2026-05-29
> 阅读对象：考虑把 `llm-agent` 的本地记忆扩展到集群、或想厘清 `llm-agent` 与 `llm-agent-memory` 边界的开发者 / reviewer。
> 文档约定：关键断言带 `file:line` 锚点。本文结论经过两轮评审（Plan 架构师 + codex 对抗式复核）。
> 状态：**设计已收敛，尚未实施。** Proposal 2 待实施，Proposal 1 待 gateway 补齐幂等后再做。

---

## 目录

1. [背景与问题](#1-背景与问题)
2. [两个 SDK 的现状关系（已核实）](#2-两个-sdk-的现状关系已核实)
3. [北极星约束](#3-北极星约束)
4. [Proposal 1：本地 → 集群客户端](#4-proposal-1本地--集群客户端)
5. [Proposal 2：抽出 durable 契约模块](#5-proposal-2抽出-durable-契约模块)
6. [两轮评审发现](#6-两轮评审发现)
7. [最终裁决与时序](#7-最终裁决与时序)
8. [遗留问题](#8-遗留问题)

---

## 1. 背景与问题

`llm-agent` 当前的记忆是**进程内本地记忆**（`llm-agent/memory`：Working/Episodic/Semantic 三引擎 + 基于快照的持久化）。问题：

- 当需要把记忆扩展到**集群 / 共享**时，是否要依赖 `llm-agent-memory` 的设计？
- `llm-agent` 与 `llm-agent-memory` 能否合并 / 互相替换，避免维护两套？

核心约束（用户设想）：两者都是 SDK，**互不 import**，但可由消费方组合使用。

---

## 2. 两个 SDK 的现状关系（已核实）

### 2.1 依赖方向：单向

- `llm-agent` 核心**不 import** `llm-agent-memory`；仅反向依赖存在。
- `llm-agent` 核心几乎不用自己的记忆：唯一引用在 `llm-agent/context/builder.go:53`（`MemoryHits []memory.SearchResult` 一个字段），`agent.go` 不用记忆。
- `llm-agent-memory` 重度依赖 core：`llm-agent-memory/memory/types_alias.go` 把 `Kind/Stats/Embedder/Scope/...` 别名到 `coremem.*`；`core_adapters.go` 提供可选 `AdaptCoreMemory` 桥。

### 2.2 引擎已各自独立

`llm-agent-memory` 的引擎（`engine_working.go`、`engine_episodic.go`、`engine_semantic.go`、`engine_scored_store.go`）运行时只用标准库，**没有运行时 `coremem.New*` 调用**（唯一一处在 `manager.go:63` 是注释示例）。即两套引擎在运行时互不调用，是**有意镜像**（都实现 §6.3 打分）。

### 2.3 集群能力的真正归属

| 层 | 模块 | 角色 |
|---|---|---|
| 本地引擎 | `llm-agent/memory`、`llm-agent-memory/memory` 引擎层 | 进程内、快照持久化，**无集群接缝** |
| 集群契约（端口） | `llm-agent-memory/memory/durable.go` | `MemoryRecord` + `RecordStore/EventStore/Outbox/MessagePublisher/Promoter/Deduper/AccessMarker/IdempotencyStore` |
| 集群适配器 | `llm-agent-memory-postgres` | 用 Postgres+pgvector 落地契约：`var _ corememory.RecordStore = (*Store)(nil)`（`postgres/store.go:57-63`）|
| 集群服务 | `llm-agent-memory-gateway` | 多租户 HTTP 服务、session、recall cache、authz |
| 异步 | `llm-agent-memory-worker` | relay / consolidation |

**核心结论：core 在结构上做不到集群**——它只有 `Memory/Lister/Exporter/Importer/SnapshotStore/Sanitizer` 接口，没有 `VectorBackend`、没有 `MemoryRecord`、没有事件/outbox/幂等概念。集群能力 = `llm-agent-memory` 的 durable 契约层 + postgres/gateway 适配器。**不能靠"扩大 core"走向集群**（会摧毁 core 的 stdlib-only 定位）。

---

## 3. 北极星约束

1. `llm-agent` 与 `llm-agent-memory` 都是 SDK，**互不 import**，由消费方组合。
2. 尽量避免维护两套发散的同逻辑。
3. `llm-agent` core 保持本地 + stdlib-only；集群记忆是独立的一层。

---

## 4. Proposal 1：本地 → 集群客户端

### 4.1 形态

新建消费边缘模块 `llm-agent-memory-client`，提供 `GatewayMemory`，通过 HTTP 访问 gateway。本地引擎退化为 L1 缓存，Postgres 为 L2 权威源。agent 在应用边缘通过 adapter 接入。

### 4.2 评审后的修正（重要）

- **不要实现 core `memory.Memory` 接口**（`llm-agent/memory/memory.go:64`）。7 个方法有 3 个实现不了：`Update(id, fn func(*MemoryItem))` 闭包过不了 HTTP；`Get(id)`/`Stats()` gateway 无端点（`router.go:21-30` 确认无 GET-by-id / Stats / List / scope-version 端点）。
- **客户端 DTO 用 gateway 的 `httpapi` 类型，不是 durable 契约 DTO。** 二者是不同 schema：`httpapi/types.go` 的 PATCH 用**指针字段**区分"字段缺省 vs 显式零值"（`types.go:80-91`），`PatchRecordInput` 表达不了。
- **客户端必须自己注入 `X-Tenant-Id` / `X-User-Id` 头 + 管 credential 生命周期。** gateway 每个路由都强制鉴权 scope（`authz/scope.go:18-30`），且用头部权威覆盖 body 里的 tenant/user（`service.go:599-605`）。这不是可选 plumbing。
- **L1 只缓存 recall 响应、绝不本地重排**：本地引擎与 gateway hybrid+pgvector 是两个不同 ranker，本地重排必然漂移。但 gateway 已有服务端 recall 缓存（`recall_cache.go`，带 `bounded`/`eventual` 一致性 + scopeVersion），请求体已有 `ConsistencyLevel`/`AllowStaleCache`——**优先用服务端缓存，不要在客户端重造一致性逻辑**。
- 若提供 core 兼容，仅给一个**显式有损**的 `AsCoreMemory()`：只实现 Search（`context/builder.go:48-56` / `select.go:110-117` 仅消费 `Item.Content/CreatedAt/ID/Tags` + `Score`），其余 `ErrUnsupported`。命名要避免暗示可替换 core `Memory`。

### 4.3 阻塞项

写操作幂等洞（见 §8）必须先在 gateway 补齐，否则客户端对 PATCH/pin/disable/DELETE 超时后无法安全重试。

---

## 5. Proposal 2：抽出 durable 契约模块

### 5.1 方案

新建 stdlib-only 模块 `llm-agent-memory-contract`，把 `durable.go` 整体搬入。它定义 `MemoryRecord` + 所有 `*Input/*Result` DTO + 8 个接口 + `RecordKind*` 常量。

### 5.2 为什么干净

- `durable.go` 只 import 标准库（`context/errors/fmt/strings/time`，`durable.go:1-9`），零引用引擎/别名/coremem，`Kind` 字段是裸 `string`。
- 引擎层↔durable 双向零耦合（穷举确认；codex 复核："did not find evidence" of dangling refs）。
- 三个卫星仓只用契约符号、零引擎类型 → import 路径替换是机械的。

### 5.3 评审后的修正（重要）

- **durable.go 是已持久化的 JSON schema，不只是 DTO。** `MemoryRecord/StoredEvent/OutboxMessage/IdempotencyEntry` 用默认 `encoding/json` 字段名直接序列化进 Postgres（`postgres/store.go:131-196, 368-438`；`store_test.go:251-287` 显式 JSON round-trip）。→ 抽出后该模块是**最高稳定性 API**：改字段名/指针形态/`time.Time` 处理 = DB 迁移。需独立 owner + 兼容策略。
- **抽 contract 后 gateway 仍保留 `llm-agent` 依赖**：gateway 经 `llm-agent-rag`（`gateway/go.mod` require rag）传递依赖 `llm-agent`。只有 postgres / worker 能彻底甩掉。
- **alias-shim 不是真正的跨版本兼容**：`type X = contract.X` 只在整图解析到同一 contract 版本时有效；混合图（`@vX` vs `@vY`）下 `var _` 断言与命名类型 slice/map 仍会炸。
- **go.work + replace 掩盖版本 skew**：本地全绿 ≠ tag 发布后下游可编译（`go.work` + 各 `go.mod` 的 replace）。

### 5.4 发布波次

```
1. 建 + tag  llm-agent-memory-contract v0.1.0（搬 durable.go）
2. postgres → 改 import + go.mod，tag（甩掉 llm-agent）
3. worker   → 改 import + go.mod，tag（甩掉 llm-agent）
4. gateway  → 改 import + go.mod，tag（仍保留 llm-agent via rag）
5. llm-agent-memory → 删 durable.go body，留 alias shim 一个周期，tag
```
**前提：捆绑 release-matrix CI**——脱离 go.work/replace、用真实 tag 测每个模块，否则迁移会"假绿"。

---

## 6. 两轮评审发现

### 6.1 第一轮（Plan 架构师）

- Proposal 1 用 core `Memory` 是漏抽象（3/7 方法实现不了）→ 改用专用客户端接口。
- Proposal 1 的客户端缓存与 gateway `recall_cache.go` 重复 → 去掉。
- Proposal 2 干净；应**先做**，因为它让客户端可只依赖 stdlib-only 契约模块。
- 迁移风险：contract 版本 skew 在 `var _` 断言处。
- （本文已修正其"三个卫星都甩掉 llm-agent"的不准确：gateway 经 rag 保留。）

### 6.2 第二轮（codex 对抗式，126k tokens / high）

- 🔴 "客户端复用 contract DTO" 错——gateway wire schema 不同（指针 PATCH 语义）。
- 🔴 除 write 外所有写操作无幂等键、且无 GET 核对 → 超时重试不安全（真 bug，见 §8）。
- 🔴 durable.go 是持久化 JSON schema → 稳定性责任被低估。
- 🟡 alias-shim 不解决混合版本图。
- 战略：契约模块是最高稳定性 API（3 个运行时模块 + DB JSON 双重依赖），需更严的 owner/兼容策略 + release-matrix 测试。

---

## 7. 最终裁决与时序

| 项 | 裁决 |
|---|---|
| 合并/替换两个 SDK | ❌ 不可。它们解决不同层问题（core=本地引擎，扩展=持久/分布式契约），且互不 import 的约束下只能靠"复制"或"第三方共享模块"共享代码 |
| Proposal 2（抽 contract） | ✅ 先做，但**捆绑 release-matrix CI**，并把契约模块当**持久化 schema 契约**管理（独立 owner + 兼容策略） |
| Proposal 1（集群客户端） | ⚠️ 推迟。重构为：不实现 core `Memory`；基于 gateway `httpapi` 类型；客户端自管鉴权头/credential；依赖 gateway 先补幂等 |

**走集群的落地路径**：不要碰 `llm-agent`。起 gateway + postgres 作为集群记忆服务；本地 agent 用 `llm-agent`（或扩展层引擎）做 L1；应用边缘用 adapter 把 gateway 包成 agent 可用的记忆接口（基于 httpapi 类型，非 core `Memory`）。

---

## 8. 遗留问题

**gateway 写操作幂等缺口（独立于本设计的真 bug）**：仅 `POST /memory/write` 有 `idempotency_key`（`httpapi/types.go:54-58`，`service.go:295-330`）；`PATCH/pin/unpin/disable/enable/DELETE/close/heartbeat` 无幂等 token（`types.go:80-149`），且无 `GET /memory/items/{id}` 在超时后核对结果（`router.go:21-30`）。任何 HTTP 客户端在请求已发出后超时都无法安全决定是否重试。建议独立开 issue 修复，作为 Proposal 1 的前置。
