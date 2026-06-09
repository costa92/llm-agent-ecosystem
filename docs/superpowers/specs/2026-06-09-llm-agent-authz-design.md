# llm-agent-authz 设计文档（共享租户/鉴权库）

- 日期：2026-06-09
- 状态：设计已批准，待写实现计划
- 类型：新共享库（独立 sibling 仓），由二轮评审 X1 决策催生
- 相关：[[project_case-study-projects]]；被 llm-agent-kb 与 llm-agent-studio 共同依赖（kb 是 first-mover）

## 1. 目标与定位

二轮评审发现 kb 与 studio 各自重复实现 org/user/membership/RBAC/JWT 且已发散（角色枚举、作用域层级、表名、refresh 处理不一致）。抽一个**可 import 的 Go 库**（**不是独立服务**）承载全部鉴权/多租户基座，两个案例项目 import 它、在各自单二进制内 mount 其 handler 与中间件。这样鉴权——最不该重复造、漂移即安全洞的部分——只实现一次。

**非目标**：不做独立 auth 服务/SSO 网关；v1 不做组织计费、SCIM、审计日志导出（预留扩展位）。

### 成功标准

kb 与 studio 各自 `import github.com/costa92/llm-agent-authz`，调用其 migrations 建表、mount `/api/auth/*` handler、用其中间件链完成"登录→JWT→作用域 membership 解析→RBAC 判定"，二者鉴权行为逐字一致，仅"二级资源作用域名"（kb=knowledge_base / studio=workspace）与各自资源表不同。

## 2. 核心设计：作用域参数化

两级租户层级统一为 `org → <scoped-resource>`，其中 `<scoped-resource>` 由消费方注入一个 **scope key**（kb 传 `"kb"`，studio 传 `"workspace"`）。authz 不认识 kb/workspace 的业务语义，只持有 `(org_id, user_id, scope_kind, scope_id, role)` 的 membership 模型。

**统一角色枚举（取代 kb 的 writer/reader 与 studio 的 editor/viewer）**：
- `admin` — 管理作用域成员 + 全部读写 + 敏感操作（HITL resume、删除资源、MCP stdio 启用等由消费方界定）
- `editor` — 创建/编辑/删除作用域内业务对象（kb 文档 / studio flow）、触发运行
- `viewer` — 只读
- 另有 `org_admin`（org 级 membership，`scope_id IS NULL`）覆盖该 org 下所有作用域的 admin 能力。

**角色合并规则（消除 kb 评审 K9 的越权歧义）**：有效角色 = `max(org 级角色, 该 scope 级角色)`，按 `org_admin > admin > editor > viewer` 取最高；跨 org 访问直接 403。该算法在 authz 中间件实现并附越权矩阵测试。

## 3. 数据模型（authz 拥有的表，提供 migrations）

- `auth_user(id, email UNIQUE, password_hash, created_at)` — argon2id
- `auth_org(id, name, created_at)`
- `auth_membership(org_id, user_id, scope_kind, scope_id NULLABLE, role, PRIMARY KEY(org_id,user_id,scope_kind,scope_id))` — `scope_id IS NULL` = org 级；`scope_kind ∈ {消费方注入}`
- `auth_session(id, user_id, refresh_hash, user_agent, expires_at, revoked_at, created_at)` — refresh token 服务端存储，支撑轮换/登出/吊销/logout-all

> 消费方的业务资源表（kb 的 `knowledge_base`、studio 的 `workspace`）仍归各自所有；authz 只通过 `auth_membership.scope_id` 弱引用其主键（不加跨库外键，由消费方保证一致性）。

## 4. 对外接口（库 API）

- **HTTP handler**（消费方 mount 到自己的 mux，前缀可配）：
  - `POST /api/auth/login` → `{access_token, expires_in}` + Set-Cookie httpOnly+SameSite=Strict refresh cookie
  - `POST /api/auth/refresh` → 轮换 refresh（旧 hash 失效，重用检测→吊销整链）+ 新 access；CSRF：refresh 走 cookie 时要求自定义头 `X-CSRF: 1`（double-submit）或仅接受同源
  - `POST /api/auth/logout` / `POST /api/auth/logout-all`
- **中间件**：`Authenticate`（解析+校验 access JWT，挂 `user_id` 到 ctx）→ `RequireScopeRole(scopeKind, minRole)`（从路由取 scope_id、查 membership、按 §2 合并规则判定，不足则 403）。
- **Authenticator 接口**：`Authenticate(ctx, creds) (userID, error)`；默认实现=邮箱+密码，预留 OIDC 实现位。
- **JWT claims**：`{sub: user_id, iat, exp}`，access TTL 默认 15m（可配）；refresh TTL 默认 30d。HS256，密钥来自 env。
- **领域服务**：`Orgs/Users/Memberships` CRUD（供消费方的 org/成员管理端点复用）。

## 5. 消费方接线（kb / studio 各做）

1. 启动跑 `authz.Migrate(ctx, pool)` 建 authz 表（与各自业务 migrations 并列）。
2. mount `/api/auth/*` handler。
3. 业务路由套 `Authenticate` + `RequireScopeRole("kb"|"workspace", role)`。
4. 建 kb/workspace 时在 `auth_membership` 写 creator 的 admin 行（scope_kind=自身、scope_id=新资源 id）。

## 6. 里程碑

- **M1（authz v0.1.0，先于 kb M1）**：user/org/membership/session 表 + login/refresh/logout + argon2id + JWT + `Authenticate`/`RequireScopeRole` 中间件 + 角色合并算法 + 越权矩阵测试。kb M1 直接依赖此 tag。
- **M2**：logout-all + refresh 重用检测/吊销链 + OIDC 适配位。studio 接入时复用，无需新增。

## 7. 风险/未决

- 作用域参数化要避免过度抽象——v1 只支持"两级（org + 单一 scope_kind）"，不做任意层级树。
- authz 是 first dependency：kb 不能在 authz v0.1.0 tag 前动工（排期耦合，已在 kb spec §13 标注）。
- 跨库无外键，删 org/用户时消费方须自行清理业务资源（在各自 spec 的删除级联里覆盖）。

## 8. 关键参考
- 现有生态无鉴权库可参照——greenfield。仅参照 `llm-agent-customer-support`（v0.3.0）`internal/config` 的 env 装配风格与 polyrepo sibling 约定（[[project_polyrepo-split]]、[[project_console-gowork-off]]）。
