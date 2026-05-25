# Memory Postgres + Outbox 表结构草案

> 文档版本：2026-05-25
> 对应代码快照：2026-05-25
> 范围：多服务 memory 架构中的 `Postgres` 真相源与 `Transactional Outbox`
> 关联文档：`multi-service-memory-architecture.zh-CN.md`
> 关联文档：`memory-gateway-api-contract.zh-CN.md`

---

## 1. 目标

这份文档把多服务 memory 方案中已经确定的 API/一致性约束，下沉成一套
`Postgres + Outbox` 的逻辑表结构草案。

目标：

- 支撑 `Memory Gateway` 的读写 API
- 支撑 optimistic concurrency control
- 支撑 `Embedding / Consolidation / Cache Invalidate` 的事件消费
- 支撑 delete/disable/pin/content update 的一致性与重放

本草案不覆盖：

- 具体 SQL migration 语法
- 分库分表策略
- 向量库的物理 schema

---

## 2. 设计原则

### 2.1 真相源原则

- `memory_record` 是当前有效状态的真相源
- `memory_event` 是变更事实流
- `outbox_event` 是可靠投递桥
- Redis / Vector Index 都不是最终真相源

### 2.2 版本原则

- 每条 memory 的业务状态由 `version` 单调递增保护
- 所有更新类操作都应建立在 `expected_version == current_version` 的前提下

### 2.3 软删优先

第一版建议：

- 业务层采用 soft delete / tombstone
- 后台异步清理向量索引和缓存
- 是否做物理删除交由归档/压缩流程处理

### 2.4 事件可重放

- worker 默认按“至少一次消费”处理
- 所有事件必须可重放、可幂等

---

## 3. 表概览

| 表名 | 职责 |
|---|---|
| `memory_record` | 当前有效 memory 状态 |
| `memory_event` | memory 的事实变更流 |
| `memory_idempotency` | 幂等键与首个结果绑定 |
| `outbox_event` | 可靠投递桥 |
| `memory_recall_invalidation` | recall/cache 失效辅助记录，可选 |

---

## 4. `memory_record`

## 4.1 职责

存储某条 memory 的**当前状态**。Gateway 读写、权限过滤、delete/disable/pin 等
最终都以这张表为准。

## 4.2 字段草案

| 字段 | 类型建议 | 说明 |
|---|---|---|
| `memory_id` | `text` / `uuid` | 主键 |
| `tenant_id` | `text` | 租户边界 |
| `user_id` | `text` | 用户边界 |
| `project_id` | `text null` | 项目边界 |
| `session_id` | `text null` | 原始会话来源 |
| `kind` | `text` | `semantic` / `episodic` |
| `source` | `text` | `user_saved` / `agent_inferred` / `system` |
| `category` | `text` | `user` / `feedback` / `project` / `reference` |
| `content` | `text` | 主内容 |
| `normalized_content_hash` | `text` | 用于去重 / merge |
| `tags` | `jsonb` | 标签数组 |
| `importance` | `double precision` | `[0,1]` |
| `pinned` | `boolean` | 是否固定保留 |
| `disabled` | `boolean` | 是否 recall 隐藏 |
| `deleted` | `boolean` | 是否 tombstone |
| `version` | `bigint` | 单调递增版本 |
| `created_at` | `timestamptz` | 创建时间 |
| `updated_at` | `timestamptz` | 最近变更时间 |
| `deleted_at` | `timestamptz null` | 软删时间 |
| `last_access_at` | `timestamptz null` | 访问统计字段 |
| `hit_count` | `bigint` | 非关键统计字段 |
| `consolidated_from_event_id` | `text null` | 若来自 promote，记录源事件 |

## 4.3 主键与唯一约束

建议：

- `PRIMARY KEY (memory_id)`

辅助唯一约束建议至少一条：

- `UNIQUE (tenant_id, memory_id)`

如果 `memory_id` 全局唯一，则第二条可选。

## 4.4 索引建议

建议至少包含：

- `(tenant_id, user_id, project_id, deleted, disabled)`
- `(tenant_id, user_id, category, deleted, disabled)`
- `(tenant_id, user_id, normalized_content_hash)`
- `(tenant_id, updated_at desc)`

如果需要按 pinned / source 高频过滤，可再加：

- `(tenant_id, user_id, pinned, deleted, disabled)`
- `(tenant_id, user_id, source, deleted, disabled)`

## 4.5 状态约束

推荐 check 约束：

- `importance >= 0 and importance <= 1`
- `kind in ('semantic','episodic')`
- `source in ('user_saved','agent_inferred','system')`

业务语义约束：

- `deleted = true` 时，不应再出现在 recall 结果里
- `disabled = true` 时，不应再出现在 recall 结果里
- `deleted = true` 不等于物理删除

---

## 5. `memory_event`

## 5.1 职责

记录 memory 的事实变更流。worker 不应直接从 `memory_record` 推断历史，而应消费
事件。

## 5.2 字段草案

| 字段 | 类型建议 | 说明 |
|---|---|---|
| `event_id` | `text` / `uuid` | 主键 |
| `memory_id` | `text` | 关联 memory |
| `tenant_id` | `text` | 租户边界 |
| `event_type` | `text` | 事件类型 |
| `version` | `bigint` | 该事件对应的目标版本 |
| `idempotency_key` | `text null` | 触发事件的幂等键 |
| `payload` | `jsonb` | 事件负载 |
| `created_at` | `timestamptz` | 事件时间 |

## 5.3 事件类型建议

第一版建议最少支持：

- `memory_created`
- `memory_updated`
- `memory_deleted`
- `memory_disabled`
- `memory_enabled`
- `memory_pinned`
- `memory_unpinned`
- `memory_promoted_to_episodic`

可选扩展：

- `memory_embedding_requested`
- `memory_embedding_applied`
- `memory_recall_invalidated`

## 5.4 索引建议

- `(memory_id, version)`
- `(tenant_id, created_at desc)`
- `(event_type, created_at desc)`

## 5.5 唯一性建议

推荐：

- `UNIQUE (memory_id, version, event_type)`

这不是绝对必要，但有助于压住重复写事件。

---

## 6. `memory_idempotency`

## 6.1 职责

把 `idempotency_key` 绑定到第一次成功处理的请求语义，避免重复创建或冲突重放。

## 6.2 字段草案

| 字段 | 类型建议 | 说明 |
|---|---|---|
| `tenant_id` | `text` | 租户边界 |
| `idempotency_key` | `text` | 幂等键 |
| `request_hash` | `text` | 请求体哈希 |
| `memory_id` | `text null` | 首次结果绑定的 memory |
| `response_snapshot` | `jsonb` | 首次成功响应快照 |
| `created_at` | `timestamptz` | 首次写入时间 |
| `expires_at` | `timestamptz null` | 可选过期时间 |

## 6.3 主键建议

- `PRIMARY KEY (tenant_id, idempotency_key)`

## 6.4 业务规则

- 同 `tenant_id + idempotency_key + request_hash`
  - 返回首个成功结果
- 同 `tenant_id + idempotency_key` 但 `request_hash` 不同
  - 返回 `409 idempotency_conflict`

---

## 7. `outbox_event`

## 7.1 职责

作为业务事务与消息系统之间的可靠桥梁。

## 7.2 字段草案

| 字段 | 类型建议 | 说明 |
|---|---|---|
| `outbox_id` | `text` / `uuid` | 主键 |
| `aggregate_type` | `text` | 固定为 `memory` |
| `aggregate_id` | `text` | 即 `memory_id` |
| `tenant_id` | `text` | 租户边界 |
| `event_id` | `text` | 对应 `memory_event.event_id` |
| `event_type` | `text` | 便于 relay 路由 |
| `payload` | `jsonb` | 发布到 MQ 的事件体 |
| `status` | `text` | `pending` / `sent` / `failed` |
| `attempt_count` | `integer` | 发布重试次数 |
| `created_at` | `timestamptz` | 写入时间 |
| `sent_at` | `timestamptz null` | 成功发布时间 |
| `last_error` | `text null` | 最后一次发布错误 |

## 7.3 索引建议

- `(status, created_at)`
- `(aggregate_id, created_at)`
- `(event_id)`

## 7.4 事务规则

以下 3 张表必须在同一数据库事务中落库：

1. `memory_record`
2. `memory_event`
3. `outbox_event`

否则会破坏之前文档里定义的 Transactional Outbox 约束。

## 7.5 Relay 规则

relay 进程建议：

1. 扫描 `status = 'pending'`
2. 发布到 MQ
3. 成功后更新：
   - `status = 'sent'`
   - `sent_at = now()`
4. 失败则：
   - `attempt_count += 1`
   - `last_error` 更新

---

## 8. `memory_recall_invalidation`（可选）

## 8.1 职责

这张表不是必须，但在 recall cache 较复杂时有价值：  
显式记录哪些 memory/version 变化需要驱动 recall cache 失效。

## 8.2 字段草案

| 字段 | 类型建议 | 说明 |
|---|---|---|
| `invalidation_id` | `text` / `uuid` | 主键 |
| `tenant_id` | `text` | 租户边界 |
| `memory_id` | `text` | 关联 memory |
| `new_version` | `bigint` | 变化后的版本 |
| `reason` | `text` | `delete` / `disable` / `pin` / `content_update` ... |
| `created_at` | `timestamptz` | 记录时间 |
| `processed_at` | `timestamptz null` | 是否已被 cache invalidation worker 处理 |

## 8.3 适用性

如果系统只依赖 `memory_event + outbox_event` 做 invalidation，这张表可以不建。  
如果系统需要：

- 回看哪些 cache invalidation 被漏处理
- 对 invalidation 做单独补偿
- 按 reason 分析 recall cache 抖动

则建议加上。

第一阶段如果目标是先验证 recall quality / promote / lifecycle / cost，而不是先做复
杂缓存层，则建议：

- 默认不建这张表
- 先依赖 `memory_event + outbox_event`
- 等确认 recall result cache 确实值得长期保留后，再考虑引入更细粒度的
  invalidation 辅助模型

---

## 9. 典型写事务草图

## 9.1 `POST /memory/write`

事务内动作：

1. 校验 `(tenant_id, idempotency_key)` 是否已存在
2. 写 `memory_record(version = 1)`
3. 写 `memory_event(type = memory_created, version = 1)`
4. 写 `outbox_event(status = pending)`
5. 写 `memory_idempotency`

## 9.2 `PATCH /memory/items/{memory_id}`

事务内动作：

1. 读取当前 `memory_record.version`
2. 校验 `expected_version == current_version`
3. 更新 `memory_record(version = current_version + 1)`
4. 写 `memory_event(type = memory_updated, version = new_version)`
5. 写 `outbox_event(status = pending)`

## 9.3 `DELETE /memory/items/{memory_id}`

事务内动作：

1. 校验 `expected_version`
2. 更新：
   - `deleted = true`
   - `deleted_at = now()`
   - `version = current_version + 1`
3. 写 `memory_event(type = memory_deleted, version = new_version)`
4. 写 `outbox_event(status = pending)`

---

## 10. 并发与冲突策略

### 10.1 默认策略

采用 optimistic concurrency control：

- 更新请求必须带 `expected_version`
- 更新 SQL 必须满足：
  - `where memory_id = ? and version = expected_version`

若更新行数为 0：

- 返回 `409 memory_conflict`

### 10.2 允许 LWW 的字段

只建议对非关键统计字段允许更宽松的冲突策略，例如：

- `hit_count`
- `last_access_at`

以下字段不应默认 LWW：

- `content`
- `source`
- `category`
- `pinned`
- `disabled`
- `deleted`

---

## 11. 最小索引集建议

如果只允许先做一版，最小索引集建议：

### `memory_record`

- `pk(memory_id)`
- `idx_record_scope(tenant_id, user_id, project_id, deleted, disabled)`
- `idx_record_hash(tenant_id, user_id, normalized_content_hash)`

### `memory_event`

- `pk(event_id)`
- `idx_event_memory(memory_id, version)`
- `idx_event_created(tenant_id, created_at desc)`

### `memory_idempotency`

- `pk(tenant_id, idempotency_key)`

### `outbox_event`

- `pk(outbox_id)`
- `idx_outbox_status(status, created_at)`

---

## 12. 不变量清单

实现时必须满足：

1. `memory_record.version` 单调递增
2. `memory_event.version` 与目标状态版本一致
3. `outbox_event` 与业务状态同事务提交
4. `deleted = true` 的 memory 不再出现在 recall 中
5. `disabled = true` 的 memory 不再出现在 recall 中
6. 同一 `idempotency_key` 不允许承载不同语义
7. Worker 消费旧事件时不得覆盖新版本状态

---

## 延伸阅读

- [`memory-gateway-api-contract.zh-CN.md`](./memory-gateway-api-contract.zh-CN.md)
- [`multi-service-memory-architecture.zh-CN.md`](./multi-service-memory-architecture.zh-CN.md)
