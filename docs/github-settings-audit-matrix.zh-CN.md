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
| `llm-agent` | `main` | `false` | `true` | `true` | `true` | `true` | 未对齐 |
| `llm-agent-rag` | `master` | `false` | `true` | `true` | `true` | `true` | 未对齐 |
| `llm-agent-flow` | `main` | `false` | `true` | `true` | `true` | `true` | 未对齐 |
| `llm-agent-providers` | `main` | `true` | `true` | `true` | `true` | `true` | 已对齐 |
| `llm-agent-otel` | `main` | `true` | `true` | `true` | `true` | `true` | 已对齐 |
| `llm-agent-customer-support` | `main` | `true` | `true` | `true` | `true` | `true` | 已对齐 |

### 3.2 默认分支保护矩阵

| Repo | Default Branch | 保护状态 | Required Checks | Enforce Admins | 结论 |
|---|---|---|---|---:|---|
| `llm-agent` | `main` | 无 protection | 无 | — | 未对齐 |
| `llm-agent-rag` | `master` | 无 protection | 无 | — | 未对齐 |
| `llm-agent-flow` | `main` | 无 protection | 无 | — | 未对齐 |
| `llm-agent-providers` | `main` | 有 protection | `go`, `governance` | `true` | 已对齐 |
| `llm-agent-otel` | `main` | 有 protection | `go`, `governance` | `true` | 已对齐 |
| `llm-agent-customer-support` | `main` | 有 protection | `go`, `governance` | `true` | 部分对齐 |

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

### 4.1 已经完整对齐的仓库

以下 3 个仓库线上设置与当前治理设计基本一致：

- `llm-agent-providers`
- `llm-agent-otel`
- `llm-agent-customer-support`

它们共同满足：

- `allow_auto_merge = true`
- `delete_branch_on_merge = true`
- 默认分支存在 protection
- required checks 包含 `go` 与 `governance`

### 4.2 当前未对齐的仓库

以下 3 个仓库当前还没有把 GitHub 仓库设置完全补齐：

- `llm-agent`
- `llm-agent-rag`
- `llm-agent-flow`

共同问题：

1. `allow_auto_merge = false`
2. 默认分支 protection 缺失

这意味着虽然仓库里已经有：

- `pr-governance.yml`
- `delete-merged-branch.yml`
- 基础 CI workflow

但 GitHub 仓库侧环境还不足以让完整治理链路真正稳定落地。

### 4.3 `llm-agent-customer-support` 的备注

`llm-agent-customer-support` 的默认分支 protection 当前 required checks 显示为：

- `go`
- `governance`

这与治理基线是一致的，但由于它的 CI 还会产出：

- `format`
- `compose`
- `docker`

所以它是“治理层已对齐”，但如果团队未来要把更完整的 CI 结果也升为必过项，还可以继续增强。

### 4.4 `llm-agent-flow` 的额外缺口

`llm-agent-flow` 当前除了 GitHub 仓库设置未对齐外，还有一个 workflow 级差异：

- 当前仓库里没有 `release-precheck.yml`

这不是 GitHub 仓库 settings 的问题，而是 repo 内 workflow 文件还没补齐。

## 5. 审计结论

### 5.1 总体状态

可以把当前 6 个仓库分成两组：

#### A 组：治理已完整落地

- `llm-agent-providers`
- `llm-agent-otel`
- `llm-agent-customer-support`

#### B 组：workflow 已推送，但 GitHub 仓库设置未补齐

- `llm-agent`
- `llm-agent-rag`
- `llm-agent-flow`

### 5.2 当前最重要的待修复项

优先级最高的 4 项：

1. 在 `llm-agent` 打开 `Allow auto-merge`
2. 在 `llm-agent-rag` 打开 `Allow auto-merge`
3. 在 `llm-agent-flow` 打开 `Allow auto-merge`
4. 为这 3 个仓库的默认分支补上 protection / ruleset，至少要求 `go` 与 `governance`

### 5.3 次级待修复项

1. 为 `llm-agent-flow` 补上 `release-precheck.yml`
2. 评估是否将 `llm-agent-customer-support` 的 `format` / `compose` / `docker` 也纳入 required checks

## 6. 推荐下一步

建议按下面顺序收口：

1. 先按 [`github-web-ui-setup-runbook.zh-CN.md`](./github-web-ui-setup-runbook.zh-CN.md) 补 `llm-agent`、`llm-agent-rag`、`llm-agent-flow` 的 GitHub 仓库设置
2. 再重新执行一次本审计矩阵
3. 若要完全统一治理面，再补 `llm-agent-flow` 的 `release-precheck.yml`

## 7. 一句话总结

当前不是所有仓库都“已经完全可用”。

准确说法是：

- 6 个仓库的 workflow 文件已经基本铺开
- 但只有 `providers`、`otel`、`customer-support` 这 3 个仓库的 GitHub 线上设置已经和治理设计真正对齐
- `llm-agent`、`llm-agent-rag`、`llm-agent-flow` 还需要补 GitHub 仓库级设置与默认分支保护

## 延伸阅读

- [`./github-workflows-design.zh-CN.md`](./github-workflows-design.zh-CN.md)
- [`./github-repo-settings-runbook.zh-CN.md`](./github-repo-settings-runbook.zh-CN.md)
- [`./github-web-ui-setup-runbook.zh-CN.md`](./github-web-ui-setup-runbook.zh-CN.md)
