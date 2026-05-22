# GitHub 仓库设置审计矩阵

> 文档版本：2026-05-22
> 审计时间：2026-05-22
> 审计方式：GitHub REST / CLI 实时查询

本文档记录 6 个子项目当前 GitHub 仓库设置与保护规则的**实际线上状态**，并与“目标配置”做对照。

它不是设计文档，也不是操作手册，而是一份审计矩阵：

- 哪些仓库已经对齐
- 哪些仓库还没对齐
- 具体差异是什么

## 1. 审计范围

- `costa92/llm-agent`
- `costa92/llm-agent-rag`
- `costa92/llm-agent-flow`
- `costa92/llm-agent-providers`
- `costa92/llm-agent-otel`
- `costa92/llm-agent-customer-support`

## 2. 目标基线

### 2.1 仓库级目标设置

所有仓库目标上都应满足：

1. `allow_auto_merge = true`
2. `delete_branch_on_merge = true`
3. `allow_merge_commit = true`

### 2.2 默认分支保护目标

默认分支保护目标应至少满足：

1. 默认分支存在 protection / ruleset
2. required checks 至少包含：
   - `go`
   - `governance`

对于 `llm-agent-customer-support`，通常还需要额外 required checks：

- `format`
- `compose`
- `docker`

但治理层基线仍是 `go + governance`。

## 3. 实时审计结果

### 3.1 仓库级设置矩阵

| Repo | Default Branch | Allow Auto-Merge | Delete Branch On Merge | Allow Merge Commit | Allow Squash | Allow Rebase | 结论 |
|---|---|---:|---:|---:|---:|---:|---|
| `llm-agent` | `main` | `true` | `true` | `true` | `true` | `true` | 已对齐 |
| `llm-agent-rag` | `master` | `true` | `true` | `true` | `true` | `true` | 已对齐 |
| `llm-agent-flow` | `main` | `true` | `true` | `true` | `true` | `true` | 已对齐 |
| `llm-agent-providers` | `main` | `true` | `true` | `true` | `true` | `true` | 已对齐 |
| `llm-agent-otel` | `main` | `true` | `true` | `true` | `true` | `true` | 已对齐 |
| `llm-agent-customer-support` | `main` | `true` | `true` | `true` | `true` | `true` | 已对齐 |

### 3.2 默认分支保护矩阵

| Repo | Default Branch | 保护状态 | Required Checks | Enforce Admins | 结论 |
|---|---|---|---|---:|---|
| `llm-agent` | `main` | 有 protection | `go`, `governance` | `true` | 已对齐 |
| `llm-agent-rag` | `master` | 有 protection | `go`, `governance` | `true` | 已对齐 |
| `llm-agent-flow` | `main` | 有 protection | `go`, `governance` | `true` | 已对齐 |
| `llm-agent-providers` | `main` | 有 protection | `go`, `governance` | `true` | 已对齐 |
| `llm-agent-otel` | `main` | 有 protection | `go`, `governance` | `true` | 已对齐 |
| `llm-agent-customer-support` | `main` | 有 protection | `go`, `governance` | `true` | 基本对齐 |

### 3.3 workflow 文件矩阵

| Repo | `pr-governance` | `delete-merged-branch` | `release-precheck` | `test/ci` | 特殊 workflow |
|---|---:|---:|---:|---:|---|
| `llm-agent` | 是 | 是 | 是 | 是 | `umbrella.yml` |
| `llm-agent-rag` | 是 | 是 | 是 | 是 | — |
| `llm-agent-flow` | 是 | 是 | 否 | 是 | — |
| `llm-agent-providers` | 是 | 是 | 是 | 是 | `nightly-ollama-live.yml` |
| `llm-agent-otel` | 是 | 是 | 是 | 是 | — |
| `llm-agent-customer-support` | 是 | 是 | 是 | 是 | CI 名称为 `ci` |

## 4. 关键发现

### 4.1 已经完成核心对齐的仓库

当前 6 个仓库都已经完成核心治理配置对齐：

- `llm-agent`
- `llm-agent-rag`
- `llm-agent-flow`
- `llm-agent-providers`
- `llm-agent-otel`
- `llm-agent-customer-support`

它们共同满足：

- `allow_auto_merge = true`
- `delete_branch_on_merge = true`
- 默认分支存在 protection
- required checks 包含 `go` 与 `governance`

### 4.2 本次补齐的线上配置

这次已实际补齐以下 3 个仓库的 GitHub 线上设置：

- `llm-agent`
- `llm-agent-rag`
- `llm-agent-flow`

补齐内容包括：

1. 打开 `allow_auto_merge`
2. 为默认分支添加 protection
3. required checks 统一为：
   - `go`
   - `governance`
4. `enforce_admins = true`

### 4.3 `llm-agent-customer-support` 的备注

`llm-agent-customer-support` 的默认分支 protection 当前 required checks 显示为：

- `go`
- `governance`

这与治理基线是一致的，但由于它的 CI 还会产出：

- `format`
- `compose`
- `docker`

所以它是“治理层已对齐”，但如果团队未来要把更完整的 CI 结果也升为必过项，还可以继续增强。

另外，它当前是 6 个仓库里唯一一个：

- `allow_force_pushes = true`

这不影响 `go + governance` 主治理链路，但从一致性角度看，后续可评估是否也收紧到 `false`。

### 4.4 `llm-agent-flow` 的额外缺口

`llm-agent-flow` 当前除了 GitHub 仓库设置未对齐外，还有一个 workflow 级差异：

- 当前仓库里没有 `release-precheck.yml`

这不是 GitHub 仓库 settings 的问题，而是 repo 内 workflow 文件还没补齐。

## 5. 审计结论

### 5.1 总体状态

当前 6 个仓库已经全部完成核心治理落地：

- 仓库级 `allow_auto_merge = true`
- 仓库级 `delete_branch_on_merge = true`
- 默认分支 protection 存在
- required checks 为 `go + governance`

### 5.2 当前最重要的待修复项

目前剩余的不是核心阻塞项，而是一致性增强项：

1. 为 `llm-agent-flow` 补上 `release-precheck.yml`
2. 评估是否将 `llm-agent-customer-support` 的 `format` / `compose` / `docker` 也纳入 required checks
3. 评估是否将 `llm-agent-customer-support` 的 `allow_force_pushes` 从 `true` 收紧到 `false`

### 5.3 次级待修复项

1. 周期性复核 6 个仓库 settings 是否仍保持一致
2. 若后续新增仓库，按同样矩阵补齐治理面

## 6. 推荐下一步

建议按下面顺序收口：

1. 重新跑一次 owner PR 验证，确认 `llm-agent`、`llm-agent-rag`、`llm-agent-flow` 现在也能稳定 auto-merge
2. 为 `llm-agent-flow` 补上 `release-precheck.yml`
3. 评估是否继续统一 `customer-support` 的 protection 细节

## 7. 一句话总结

当前 6 个仓库的核心 GitHub 治理设置已经全部对齐。

现在的剩余工作，不再是“能不能用”，而是“是否要继续把一些细节也统一到完全一致”。

## 延伸阅读

- [`./github-workflows-design.zh-CN.md`](./github-workflows-design.zh-CN.md)
- [`./github-repo-settings-runbook.zh-CN.md`](./github-repo-settings-runbook.zh-CN.md)
- [`./github-web-ui-setup-runbook.zh-CN.md`](./github-web-ui-setup-runbook.zh-CN.md)
