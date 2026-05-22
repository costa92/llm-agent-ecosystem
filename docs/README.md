# Docs 索引

> 本目录是 `llm-agent-ecosystem` 的**生态级文档**入口。每个 sibling 仓库的本地文档（README、CHANGELOG、`.planning/`）保留在各自仓库内；本目录只收录跨仓视角与 ecosystem-wide 设计资料。
> 文档版本：2026-05-21

---

## 1. 阅读地图

### 1.1 推荐阅读顺序

| 阅读路径 | 适用读者 | 顺序 |
|---|---|---|
| **首读路径**（30 分钟内建立心智模型） | 新人 / reviewer | (1) [`current-project-analysis.zh-CN.md`](./current-project-analysis.zh-CN.md) → (2) [`architecture-and-sequence-diagrams.zh-CN.md`](./architecture-and-sequence-diagrams.zh-CN.md) → (3) [`source-design-umbrella-root.zh-CN.md`](./source-design-umbrella-root.zh-CN.md) |
| **深读路径**（按子系统逐仓深入） | 贡献者 / 维护者 | (1) [`source-design-llm-agent.zh-CN.md`](./source-design-llm-agent.zh-CN.md) → (2) [`source-design-llm-agent-rag.zh-CN.md`](./source-design-llm-agent-rag.zh-CN.md) → (3) [`source-design-llm-agent-providers.zh-CN.md`](./source-design-llm-agent-providers.zh-CN.md) → (4) [`source-design-llm-agent-otel.zh-CN.md`](./source-design-llm-agent-otel.zh-CN.md) → (5) [`source-design-llm-agent-flow.zh-CN.md`](./source-design-llm-agent-flow.zh-CN.md) → (6) [`source-design-llm-agent-customer-support.zh-CN.md`](./source-design-llm-agent-customer-support.zh-CN.md) |
| **评审路径**（找设计争议、风险点、改进点） | tech-lead / 架构师 | (1) [`ecosystem-design-review.zh-CN.md`](./ecosystem-design-review.zh-CN.md) → (2) [`subsystems-design-notes.zh-CN.md`](./subsystems-design-notes.zh-CN.md) → (3) `source-design-*` 各文档的 §8 / §9 优化与遗留章节 |
| **路线图路径**（理解 v1.2 → v1.3 → v2 走向） | 产品/项目管理 | (1) [`refactor-and-optimization-roadmap.zh-CN.md`](./refactor-and-optimization-roadmap.zh-CN.md) → (2) `llm-agent/.planning/STATE.md` + `ROADMAP.md`（核心仓 source of truth）|

### 1.2 文档拓扑

```
首读                    深读                       评审                  路线图
─────                  ─────                     ─────                ─────
current-project-       source-design-           ecosystem-           refactor-and-
  analysis (EN/zh)      llm-agent.zh-CN          design-review          optimization-
       │                source-design-           .zh-CN                 roadmap.zh-CN
       ▼                  llm-agent-rag                                   │
architecture-and-       source-design-          subsystems-              │
  sequence-diagrams       llm-agent-providers     design-notes           │
       │                source-design-            .zh-CN                 │
       ▼                  llm-agent-otel                                 │
source-design-          source-design-                                   │
  umbrella-root           llm-agent-flow                                 │
                        source-design-                                   │
                          llm-agent-customer-                            │
                          support                                        │
                                                                         │
                       （所有 source-design 的 §8/§9 → 评审 → 路线图）─┘
```

---

## 2. 完整文档清单（按主题）

### 2.1 项目概览

| 文档 | 角色 | 长度 |
|---|---|---|
| [`current-project-analysis.md`](./current-project-analysis.md) | 项目全景分析（英文） | — |
| [`current-project-analysis.zh-CN.md`](./current-project-analysis.zh-CN.md) | 项目全景分析（中文） | — |
| [`ecosystem-design-review.zh-CN.md`](./ecosystem-design-review.zh-CN.md) | 跨仓设计评审 + 代码实现差距分析 | — |

### 2.2 源码级设计（7 篇 source-design-\*）

| 文档 | 范围 | 长度（行）| 关键章节 |
|---|---|---|---|
| [`source-design-umbrella-root.zh-CN.md`](./source-design-umbrella-root.zh-CN.md) | umbrella 根仓（非代码协调点） | ~600 | Makefile / depcheck / B2-B4 / 协调式发布 |
| [`source-design-llm-agent.zh-CN.md`](./source-design-llm-agent.zh-CN.md) | 核心抽象层（stdlib-only） | 1023 | agents / llm / orchestrate / memory / budget / policy / comm |
| [`source-design-llm-agent-rag.zh-CN.md`](./source-design-llm-agent-rag.zh-CN.md) | RAG / GraphRAG SDK（fixed point） | 1050 | ingest / retrieve / pack / Ask 三路径 / GraphRAG |
| [`source-design-llm-agent-providers.zh-CN.md`](./source-design-llm-agent-providers.zh-CN.md) | 5 家厂商适配器 | 1054 | K1/K2 合规 / contract fixtures / 5 provider 深读 |
| [`source-design-llm-agent-otel.zh-CN.md`](./source-design-llm-agent-otel.zh-CN.md) | OTel decorator wrappers（K3） | 880 | 8-wrapper capability matrix / GenAI semconv / 双 opt-in 闸门 |
| [`source-design-llm-agent-flow.zh-CN.md`](./source-design-llm-agent-flow.zh-CN.md) | Flow IR + DAG Engine + flowd | 1026 | IR / Engine 分层并行 / SQLite per-event / Runner 缝合点 |
| [`source-design-llm-agent-customer-support.zh-CN.md`](./source-design-llm-agent-customer-support.zh-CN.md) | 端到端参考服务 | 703 | App 装配根 / supportflow StateGraph / limits / guardrails 缺口 |

### 2.3 架构图与时序

| 文档 | 内容 |
|---|---|
| [`architecture-and-sequence-diagrams.zh-CN.md`](./architecture-and-sequence-diagrams.zh-CN.md) | 10 张架构图 + 7 张关键时序图（mermaid），全部带 `file:line` 锚点 |

### 2.4 路线图

| 文档 | 内容 |
|---|---|
| [`refactor-and-optimization-roadmap.zh-CN.md`](./refactor-and-optimization-roadmap.zh-CN.md) | 重构计划与短/中/长期优化路线（v1.2 → v1.3 → v2） |

### 2.5 CI / Workflows 设计

| 文档 | 内容 |
|---|---|
| [`github-workflows-design.zh-CN.md`](./github-workflows-design.zh-CN.md) | 6 个子项目的 GitHub Actions 工作流盘点、主路径、兜底路径与治理设计 |
| [`github-workflows-design.md`](./github-workflows-design.md) | English version of the workflows design guide |
| [`github-repo-settings-runbook.zh-CN.md`](./github-repo-settings-runbook.zh-CN.md) | GitHub 仓库设置、branch protection、auto-merge 与自动删分支的运维清单 |

### 2.6 速记 / 笔记

| 文档 | 内容 |
|---|---|
| [`subsystems-design-notes.zh-CN.md`](./subsystems-design-notes.zh-CN.md) | 跨子系统设计要点速记（非完整深读，但收录决策原文与 link） |

---

## 3. 文档约定

1. **中英分文件**：英文与中文文档**独立存在**，不在同一 markdown 里混排。中文文档以 `*.zh-CN.md` 为后缀；英文文档不加后缀（如 `current-project-analysis.md`）。
2. **`file:line` 锚点强制**：所有 source-design 与架构文档的断言**必须**附 `file:line`（如 `agent.go:13-21`），让读者一键跳转源码核对。
3. **优先 mermaid**：架构图、时序图、状态机统一用 mermaid 表达，避免外链图片随时间失效；mermaid 语法保持兼容 GitHub render（不用未稳定的 syntax）。
4. **不放运行配置**：环境变量、`.env.example`、Helm values、docker-compose 完整 yaml 等都**不**放本目录；它们属于各自 sibling 仓库的 `compose/` 或 `internal/config/`。
5. **每篇文档自带版本日期**：开头标"文档版本：YYYY-MM-DD" + "对应代码快照：YYYY-MM-DD"，提示行号引用可能漂移。
6. **不在文档内嵌入 secret 或 token 样例**：即使示意值，也用 `<your-key>` 占位符。
7. **每个文档结尾或顶部提供"延伸阅读"**：通过 relative link 指向同目录其他相关文档。

---

## 4. 如何贡献文档

### 4.1 必须更新文档的场景

PR review 时，**必须**同步本目录文档的场景：

- ✅ 新增 sibling 仓库 → 更新 `source-design-umbrella-root.zh-CN.md` §2 / §7 / `cmd/depcheck/main.go:32-39` 的 roster
- ✅ 改动 K1 / K2 / K3 / K4 / KC-1 / CC-1 / CC-2 任一 keystone 的实现 → 更新对应 `source-design-*.zh-CN.md` 的设计思想章节
- ✅ 加新 Agent 范式 / 新 Tool / 新装饰器类型 → 更新 `architecture-and-sequence-diagrams.zh-CN.md` 的对应图
- ✅ 改动跨仓依赖方向 → 同步更新 `README.md`（生态根）+ `source-design-umbrella-root.zh-CN.md` §7 + 架构图 §1
- ✅ 引入 / 删除 CI 闸子 → 更新 `source-design-umbrella-root.zh-CN.md` §6 + 架构图 §10
- ✅ 改动 `Makefile` target 或 `scripts/` → 更新 `source-design-umbrella-root.zh-CN.md` §3 / §10
- ✅ 改动 `.github/workflows/*`、branch protection 约定、PR 自动合并 / 自动删分支链路 → 更新 `github-workflows-design.zh-CN.md`

### 4.2 PR 检查清单（针对文档贡献）

```
[ ] 我在本次代码变更中触及了 KEY 接口 / KEY 决策
[ ] 我已经更新所有相关 source-design-*.zh-CN.md 段落
[ ] 我已经核对 file:line 引用与最新代码一致
[ ] 我已经更新 architecture-and-sequence-diagrams.zh-CN.md 对应图（如适用）
[ ] 我在 docs/README.md（本文件）的清单中检查文档是否要新增条目
[ ] 我所有 mermaid 块通过 GitHub render 测试
[ ] 我的中文文档以 `.zh-CN.md` 结尾、英文不加后缀
[ ] 我没有在文档内嵌入 secret / token / 长 yaml
```

### 4.3 编辑工具建议

- mermaid 编辑可用 https://mermaid.live 预览，避免推 GitHub 后 render 失败
- file:line 行号可以用 `git log -L <start>,<end>:<file>` 追踪历史，确保引用稳定
- 中文术语保持与生态约定一致（"装饰器"而非"装饰器模式"，"chokepoint" 保持英文，"keystone" 保持英文）

---

## 5. 维护节奏

| 节奏 | 触发 | 负责人 | 范围 |
|---|---|---|---|
| **每个 sibling milestone close** | sibling 进入 vX.Y.0 release | sibling repo 的 maintainer | 更新对应 source-design 文档 + roster 中 tag 列 |
| **每个 ecosystem-wide milestone close**（如 v1.1） | umbrella `STATE.md` 标记 milestone shipped | umbrella maintainer | 更新 `current-project-analysis.*` + `ecosystem-design-review.zh-CN.md` + `refactor-and-optimization-roadmap.zh-CN.md` |
| **每次 Phase 33 cascade bump wave** | depcheck 输出 cleanup | 发布协调人 | 更新 `README.md`（roster tag）+ `source-design-umbrella-root.zh-CN.md` §8 中的 wave 列表（若加新 wave） |
| **每次 keystone 决策更新（KE-*, KC-*）** | `llm-agent/.planning/research/` 有新决策文档 | 提案人 | 更新本目录所有引用该 keystone 的 source-design 文档段落 |
| **每季度 documentation health check** | 季度初 | 文档负责人 | 全量 `file:line` 锚点核对，移除已失效链接，更新 mermaid 与最新代码同步 |

---

## 6. 与其他文档体系的关系

| 体系 | 路径 | 关系 |
|---|---|---|
| **umbrella 根 README** | [`/README.md`](../README.md) | 导航 + 规则 + roster；本目录是它的"docs/" 子目录 |
| **umbrella `.planning/`**（若有） | `/.planning/` | umbrella 层 phase / roadmap；本目录指向它而不收纳 |
| **核心仓 `.planning/`** | `/llm-agent/.planning/` | 生态 source of truth（含 PROJECT.md / STATE.md / ROADMAP.md / REQUIREMENTS.md / KE-*）；本目录的 source-design 文档**对照**它，但**不重复**它的内容 |
| **sibling 仓 README** | 各 sibling repo 的 `README.md` | 各仓 surface area 文档；本目录的 source-design 是它的"深读层" |
| **sibling 仓 CHANGELOG** | 各 sibling repo 的 `CHANGELOG.md` | 版本变更详情；本目录引用关键变更，不重复 |
| **sibling 仓 `.planning/codebase/*`** | 如 `llm-agent-providers/.planning/codebase/CONCERNS.md` | sibling 内部审计；本目录的 source-design 引用其中关键 finding |

---

> 本目录最后维护：2026-05-21（v1.1 close + v1.2 in flight + 6 篇 source-design 深读完工）。下次维护触发点：v1.2 milestone close 或任一 sibling 进入新 minor。
