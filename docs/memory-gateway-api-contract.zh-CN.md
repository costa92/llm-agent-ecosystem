# Memory Gateway API 契约草案

> 文档版本：2026-05-25
> 对应代码快照：2026-05-25
> 范围：多服务 memory 架构中的 `Memory Gateway / API Service`
> 关联文档：`multi-service-memory-architecture.zh-CN.md`

---

## 1. 目标

这份文档把多服务 memory 架构中的 `Memory Gateway` 约束，收敛成一组可实现的
HTTP/JSON API 契约。

设计目标：

- 为 `Agent Service` 提供统一的长期 memory 入口
- 隐藏 `Postgres / Vector Index / Redis / Worker` 的内部实现细节
- 把多租户、幂等、版本控制、删除语义、缓存一致性约束固化到接口层

本草案不覆盖：

- 内部数据库 schema
- outbox 表设计
- MQ topic 命名
- 向量库厂商特定参数

---

## 2. 总体约束

### 2.1 统一响应头

所有成功响应建议包含：

- `X-Request-Id`
- `X-Memory-Version`（单对象操作时）
- `X-Consistency-Level`（recall 场景）

### 2.2 统一错误模型

错误响应统一为：

```json
{
  "error": {
    "code": "memory_conflict",
    "message": "expected_version does not match current version",
    "request_id": "req_123",
    "retryable": false,
    "details": {
      "memory_id": "mem_123",
      "expected_version": 4,
      "current_version": 5
    }
  }
}
```

### 2.3 通用错误码

| code | HTTP | 含义 |
|---|---|---|
| `bad_request` | `400` | 参数不合法 |
| `unauthorized` | `401` | 未认证 |
| `forbidden` | `403` | scope / tenant 越权 |
| `not_found` | `404` | 资源不存在或对当前 scope 不可见 |
| `memory_conflict` | `409` | `expected_version` 不匹配 |
| `idempotency_conflict` | `409` | 同一 `idempotency_key` 语义不一致 |
| `rate_limited` | `429` | 限流 |
| `read_only_mode` | `503` | 当前只读，拒绝写入 |
| `upstream_unavailable` | `503` | DB / index / cache 关键依赖不可用 |

### 2.4 租户边界

所有请求都必须显式或隐式绑定：

- `tenant_id`
- `user_id`
- 视场景可带：
  - `project_id`
  - `session_id`

Gateway 必须以服务端认证结果为准，不能信任客户端自报的跨租户 scope。

#### 多租户安全硬约束

Memory recall 的越权风险高于普通 CRUD，因为：

- recall 是 fuzzy match
- vector search 是 approximate retrieval

因此必须明确：

1. 向量索引返回的只是候选，不是授权结果
2. tenant / user / project / disabled / deleted 的最终判定必须在 `Postgres`
   或等价真相源侧完成
3. 任何仅依赖向量库 metadata filter 就直接返回结果的实现，都不满足本契约

一句话约束：

- **tenant filtering 必须 DB side enforce**

---

## 3. 公共对象模型

## 3.1 MemoryRecord

```json
{
  "memory_id": "mem_123",
  "tenant_id": "tenant_a",
  "user_id": "user_1",
  "project_id": "proj_x",
  "session_id": "sess_9",
  "kind": "semantic",
  "source": "user_saved",
  "category": "project",
  "content": "User prefers concise technical answers.",
  "tags": ["style", "preference"],
  "importance": 0.92,
  "pinned": true,
  "disabled": false,
  "version": 7,
  "created_at": "2026-05-25T10:00:00Z",
  "updated_at": "2026-05-25T10:10:00Z"
}
```

### 3.2 Scope

```json
{
  "tenant_id": "tenant_a",
  "user_id": "user_1",
  "project_id": "proj_x",
  "session_id": "sess_9"
}
```

### 3.3 RecallHit

```json
{
  "memory_id": "mem_123",
  "kind": "episodic",
  "score": 0.91,
  "version": 7,
  "content": "User asked for PDF export twice this week.",
  "tags": ["feature", "behavior"],
  "source": "agent_inferred",
  "category": "project",
  "pinned": false,
  "disabled": false,
  "metadata": {
    "matched_by": "long_term_unified"
  }
}
```

---

## 4. Recall API

## 4.1 `POST /memory/recall/unified`

用途：

- 做长期层 unified recall
- 只覆盖 `episodic + semantic`
- 本地 `working` 由 agent 自己查，再在调用侧做最终 merge

### 请求

```json
{
  "scope": {
    "tenant_id": "tenant_a",
    "user_id": "user_1",
    "project_id": "proj_x",
    "session_id": "sess_9"
  },
  "query": "export document as pdf",
  "top_k": 8,
  "token_budget": 1200,
  "memory_token_budget": 400,
  "consistency_level": "eventual",
  "allow_stale_cache": true,
  "debug": false
}
```

### 字段说明

| 字段 | 必填 | 说明 |
|---|---|---|
| `scope` | 是 | 多租户边界 |
| `query` | 是 | 用户查询 |
| `top_k` | 否 | 默认 `8`，上限建议 `50` |
| `token_budget` | 否 | 当前请求剩余总 token 预算 |
| `memory_token_budget` | 否 | 分配给 memory 片段的预算上限 |
| `consistency_level` | 否 | `eventual` / `bounded` / `strong` |
| `allow_stale_cache` | 否 | 是否允许 stale result cache |
| `debug` | 否 | 是否返回 trace 信息 |

补充约束：

- 若调用方提供 `token_budget` 或 `memory_token_budget`：
  - Gateway 不应只返回“相关但明显放不进 prompt”的长结果列表
  - 应优先返回更可能在预算内被选中的 hits
- 第一阶段不要求 Gateway 做精确 tokenizer 级别裁剪，但至少应支持：
  - 基于内容长度或预估 token 成本的保守过滤
  - 为每个 hit 返回可用于调用侧二次裁剪的 `token_cost_estimate`

### 一致性等级映射

| API 值 | 对应语义 |
|---|---|
| `eventual` | Level 1 最终一致 |
| `bounded` | Level 2 准强一致 |
| `strong` | Level 3 强一致 |

约束：

- `strong` 时，Gateway 不得返回 stale result cache
- `strong` 时，必要时可绕过 recall result cache 直接回源

### 响应

```json
{
  "hits": [
    {
      "memory_id": "mem_123",
      "kind": "semantic",
      "score": 0.95,
      "version": 7,
      "content": "User prefers concise technical answers.",
      "tags": ["style", "preference"],
      "source": "user_saved",
      "category": "project",
      "pinned": true,
      "disabled": false,
      "metadata": {
        "matched_by": "long_term_unified",
        "token_cost_estimate": 42
      }
    }
  ],
  "trace": {
    "cache_level": "l2_hit",
    "consistency_level": "eventual",
    "stale_served": false
  }
}
```

### 示例：带 token 预算的 recall

当调用方只希望给 memory 预留较小 prompt 预算时，请求可以显式带：

```json
{
  "scope": {
    "tenant_id": "tenant_a",
    "user_id": "user_1",
    "project_id": "proj_x",
    "session_id": "sess_9"
  },
  "query": "what do we already know about pdf export preferences",
  "top_k": 6,
  "token_budget": 900,
  "memory_token_budget": 180,
  "consistency_level": "eventual",
  "debug": true
}
```

示例响应：

```json
{
  "hits": [
    {
      "memory_id": "mem_101",
      "kind": "semantic",
      "score": 0.93,
      "version": 4,
      "content": "User prefers concise PDF export instructions.",
      "tags": ["pdf", "preference"],
      "source": "user_saved",
      "category": "project",
      "pinned": true,
      "disabled": false,
      "metadata": {
        "matched_by": "long_term_unified",
        "token_cost_estimate": 18
      }
    },
    {
      "memory_id": "mem_205",
      "kind": "episodic",
      "score": 0.88,
      "version": 2,
      "content": "User asked twice this week for export progress feedback.",
      "tags": ["pdf", "behavior"],
      "source": "agent_inferred",
      "category": "project",
      "pinned": false,
      "disabled": false,
      "metadata": {
        "matched_by": "long_term_unified",
        "token_cost_estimate": 24
      }
    }
  ],
  "trace": {
    "cache_level": "origin",
    "consistency_level": "eventual",
    "stale_served": false,
    "memory_token_budget": 180,
    "returned_token_estimate": 42
  }
}
```

这个例子表达的重点是：

- Gateway 不需要精确替代 prompt builder
- 但它至少要知道“返回一堆明显塞不进预算的长 memory”是低质量结果

### 错误语义

- `400`: 空 query / 非法 top_k
- `403`: scope 越权
- `429`: recall 被限流
- `503`: 主存储或关键依赖不可用

---

## 5. Write API

## 5.1 `POST /memory/write`

用途：

- 新增一条 memory
- 支持 `user_saved` 与 `agent_inferred`
- 强制携带 `idempotency_key`

### 请求

```json
{
  "idempotency_key": "idem_abc_123",
  "scope": {
    "tenant_id": "tenant_a",
    "user_id": "user_1",
    "project_id": "proj_x",
    "session_id": "sess_9"
  },
  "record": {
    "kind": "semantic",
    "source": "user_saved",
    "category": "project",
    "content": "User wants API contract drafts before DB schema.",
    "tags": ["workflow", "preference"],
    "importance": 0.95,
    "pinned": true
  }
}
```

### 写入语义

- `user_saved`
  - 必须先成功写入共享真相源，才能返回成功
- `agent_inferred`
  - 可接受后续异步 embedding / consolidation

### 响应

```json
{
  "memory": {
    "memory_id": "mem_123",
    "version": 1,
    "status": "saved"
  }
}
```

### 状态值

| status | 含义 |
|---|---|
| `saved` | 已成功落共享真相源 |
| `save_pending` | 请求方当前只拿到“本地暂存/待确认”语义，不得向用户宣称已永久记住 |
| `save_failed` | 本次写入未被共享真相源确认，调用方只能把它当作本地短期上下文 |

约束：

- `user_saved`
  - 第一版默认只返回：
    - `saved`
    - 或错误
  - 不建议对终端用户暴露 `save_pending`
- `agent_inferred`
  - 可在调用侧短暂处于 `save_pending`
  - 但若 Gateway 已明确拒绝或重试预算耗尽，应收敛为 `save_failed`
- 被策略拒绝、权限拒绝、参数错误等情况：
  - 统一通过错误响应表达
  - 不建议再复用 `status=rejected`

### 错误语义

- `409 idempotency_conflict`
  - 同一 `idempotency_key` 对应不同 payload
- `503 read_only_mode`
  - 当前只读，禁止写

## 5.2 `POST /memory/write/batch`

用途：

- 批量写入多条 memory
- 适合回放、导入、批量同步

请求结构与单条写入一致，只是 `record` 变成 `records[]`。  
第一版可限制：

- 同一批次必须共享同一个 `scope`
- 每条记录仍需独立的 `idempotency_key`

---

## 6. Manage API

## 6.1 `POST /memory/items/{memory_id}/pin`

### 请求

```json
{
  "scope": {
    "tenant_id": "tenant_a",
    "user_id": "user_1",
    "project_id": "proj_x"
  },
  "expected_version": 7
}
```

### 响应

```json
{
  "memory_id": "mem_123",
  "version": 8,
  "pinned": true
}
```

### 约束

- 必须做 optimistic concurrency control
- `expected_version != current_version` 时返回 `409`

## 6.2 `POST /memory/items/{memory_id}/unpin`

与 `pin` 相同，只是目标状态改为 `false`。

## 6.3 `POST /memory/items/{memory_id}/disable`

用途：

- 将 memory 从 recall 中隐藏
- 不一定物理删除

### 响应

```json
{
  "memory_id": "mem_123",
  "version": 9,
  "disabled": true
}
```

## 6.4 `POST /memory/items/{memory_id}/enable`

与 `disable` 对称。

## 6.5 `PATCH /memory/items/{memory_id}`

用途：

- 更新可编辑字段，如：
  - `content`
  - `tags`
  - `importance`
  - `category`

### 请求

```json
{
  "scope": {
    "tenant_id": "tenant_a",
    "user_id": "user_1",
    "project_id": "proj_x"
  },
  "expected_version": 9,
  "patch": {
    "content": "User wants API contract drafts before table schema.",
    "importance": 0.97
  }
}
```

### 错误语义

- `409 memory_conflict`
  - 版本冲突

---

## 7. Forget / Delete API

## 7.1 `POST /memory/forget`

用途：

- 根据策略批量 forget
- 可按 scope / category / kind / query 定位

### 请求

```json
{
  "scope": {
    "tenant_id": "tenant_a",
    "user_id": "user_1",
    "project_id": "proj_x"
  },
  "strategy": {
    "type": "query_match",
    "query": "pdf export preference"
  },
  "consistency_level": "strong"
}
```

### 响应

```json
{
  "forgotten_count": 2,
  "consistency_level": "strong"
}
```

### 约束

- `strong` 时：
  - 先更新 `Postgres`
  - 同步清理相关 result cache
  - 再返回成功

## 7.2 `DELETE /memory/items/{memory_id}`

用途：

- 删除单条 memory

### 请求参数

- 路径：`memory_id`
- body：

```json
{
  "scope": {
    "tenant_id": "tenant_a",
    "user_id": "user_1",
    "project_id": "proj_x"
  },
  "expected_version": 9,
  "consistency_level": "strong"
}
```

### 响应

```json
{
  "memory_id": "mem_123",
  "deleted": true,
  "version": 10
}
```

## 7.3 `POST /memory/delete-by-scope`

用途：

- 大范围删除，如用户注销、项目归档

第一版建议限制为后台管理或内部工具接口，不对普通 agent 暴露。

---

## 8. Session API

仅有 `session_id` 字段还不够。若要让 working memory 真正可控，Gateway 或调用侧协
议至少需要显式的 session 生命周期语义。

## 8.1 `POST /memory/sessions/{session_id}/close`

用途：

- 显式结束一个会话
- 触发该 session 下本地 working 的清理或过期流程

### 请求

```json
{
  "scope": {
    "tenant_id": "tenant_a",
    "user_id": "user_1",
    "project_id": "proj_x",
    "session_id": "sess_9"
  },
  "mode": "expire_working"
}
```

### `mode` 建议值

- `expire_working`
  - 会话结束后让 working 尽快过期
- `promote_and_expire`
  - 先按 promote 规则评估，再清理 working

### 响应

```json
{
  "session_id": "sess_9",
  "status": "closed"
}
```

## 8.2 `POST /memory/sessions/{session_id}/heartbeat`

用途：

- 显式续期长会话 working 生命周期

第一版若不想暴露 API，也至少应在调用协议或 SDK 里有等价语义。

---

## 9. Read-Only 与降级语义

## 8.1 只读模式

当 Gateway 进入 `read_only_mode`：

- `Recall API` 仍可服务
- `Write / Manage / Forget / Delete` 必须返回：

```json
{
  "error": {
    "code": "read_only_mode",
    "message": "memory gateway is temporarily read-only",
    "request_id": "req_123",
    "retryable": true
  }
}
```

## 8.2 Recall 降级

当长期 recall 依赖故障时：

- `eventual`
  - 可退化到最近一次可用 recall cache
- `bounded`
  - 可退化，但必须满足 invalidation / version 约束
- `strong`
  - 不得返回 stale result cache
  - 只能回源或返回失败

### 一致性等级默认用法

若调用方没有更强需求，建议默认绑定如下场景：

| 场景 | 推荐等级 | 原因 |
|---|---|---|
| Agent 组装 prompt 的普通 recall | `eventual` | 速度优先，可接受秒级陈旧 |
| 用户界面展示项目/偏好类记忆 | `bounded` | 需要控制短暂陈旧，但不必每次强回源 |
| `delete/forget` 后的确认读取、审计/合规读取 | `strong` | 不允许 stale result cache |

补充说明：

- `bounded/strong` 的完整缓存一致性优化不应成为第一阶段阻塞项
- 如果当前目标是验证 recall quality，而不是压榨极限性能：
  - 第一阶段可优先保证 `eventual` 可用
  - `delete/forget` 相关路径保持 `strong`
  - 其他复杂缓存语义可在验证通过后再补齐

---

## 10. 版本与幂等约束

## 9.1 `idempotency_key`

所有写入请求必须携带 `idempotency_key`。

规则：

- 同 key 同 payload：返回相同结果或语义等价结果
- 同 key 不同 payload：返回 `409 idempotency_conflict`

## 9.2 `expected_version`

所有修改类请求建议携带 `expected_version`：

- `pin/unpin`
- `disable/enable`
- `patch`
- `delete`

缺失时，第一版建议按“必须显式提供”处理，而不是默默走 LWW。

---

## 11. 最小实现优先级

第一批必须落地：

1. `POST /memory/recall/unified`
2. `POST /memory/write`
3. `PATCH /memory/items/{memory_id}`
4. `POST /memory/items/{memory_id}/pin`
5. `POST /memory/items/{memory_id}/disable`
6. `DELETE /memory/items/{memory_id}`
7. `POST /memory/sessions/{session_id}/close`

补充要求：

- 第一批实现应至少支持把 memory decision trace 以结构化日志或事件形式输出，
  覆盖：
  - `recalled`
  - `selected`
  - `dropped`
  - `promote_decided`

第二批再补：

1. `POST /memory/write/batch`
2. `POST /memory/forget`
3. `POST /memory/delete-by-scope`

---

## 延伸阅读

- [`multi-service-memory-architecture.zh-CN.md`](./multi-service-memory-architecture.zh-CN.md)
- [`memory-roadmap.zh-CN.md`](./memory-roadmap.zh-CN.md)
