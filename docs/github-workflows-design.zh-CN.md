# GitHub Workflows 工作流与设计说明

> 文档版本：2026-05-22
> 对应代码快照：2026-05-22

本文档说明 `llm-agent-ecosystem` 6 个子项目当前实际存在的 GitHub Actions workflows、它们的职责分层、触发链路、设计取舍，以及当前的主路径与兜底路径。

目标不是重复贴 YAML，而是回答 4 个问题：

1. 生态里现在到底有哪些 workflow。
2. 哪些 workflow 是主路径，哪些只是兜底或历史保留。
3. PR 从打开到合并、从合并到分支删除，实际是怎么流转的。
4. 各子仓库为什么存在少量差异。

## 1. 适用范围

本文档覆盖以下 6 个仓库：

- `llm-agent`
- `llm-agent-rag`
- `llm-agent-flow`
- `llm-agent-providers`
- `llm-agent-otel`
- `llm-agent-customer-support`

对应工作流文件位置：

- `llm-agent/.github/workflows/*`
- `llm-agent-rag/.github/workflows/*`
- `llm-agent-flow/.github/workflows/*`
- `llm-agent-providers/.github/workflows/*`
- `llm-agent-otel/.github/workflows/*`
- `llm-agent-customer-support/.github/workflows/*`

## 2. 设计目标

这套 workflows 设计同时服务 5 类目标：

1. 每仓库独立 CI 必须存在，确保各模块能单独构建、测试、发布。
2. owner 自己的 PR 不应再被 GitHub 内建 required review 卡死。
3. external PR 仍然必须由 `costa92` 审核当前 head 后才能合并。
4. release 分支不能带 `replace` 指令进入发布链路。
5. 多仓库体系里，核心仓 `llm-agent` 的变更必须能被下游联动验证。

## 3. 工作流总览

### 3.1 按仓库盘点

| Repo | Default Branch | Workflows |
|---|---|---|
| `llm-agent` | `main` | `pr-governance`, `delete-merged-branch`, `release-precheck`, `test`, `umbrella` |
| `llm-agent-rag` | `master` | `pr-governance`, `delete-merged-branch`, `release-precheck`, `test` |
| `llm-agent-flow` | `main` | `pr-governance`, `delete-merged-branch`, `test` |
| `llm-agent-providers` | `main` | `pr-governance`, `delete-merged-branch`, `release-precheck`, `test`, `nightly-ollama-live` |
| `llm-agent-otel` | `main` | `pr-governance`, `delete-merged-branch`, `release-precheck`, `test` |
| `llm-agent-customer-support` | `main` | `pr-governance`, `delete-merged-branch`, `release-precheck`, `ci` |

### 3.2 按职责分层

| 类别 | Workflow | 角色 | 当前地位 |
|---|---|---|---|
| PR 治理 | `pr-governance.yml` | author-sensitive merge gate + owner auto-merge + 合并后删分支 | 主路径 |
| PR / Push CI | `test.yml` 或 `ci` | 每仓库编译、测试、格式、构建校验 | 主路径 |
| Release 闸门 | `release-precheck.yml` | 拒绝 `release/**` 上的 `replace` 指令 | 主路径 |
| 生态联动验证 | `umbrella.yml` | 在 `llm-agent` PR 上验证跨仓兼容性 | 主路径，仅核心仓 |
| 夜间真实环境验证 | `nightly-ollama-live.yml` | `providers` 对真实 Ollama 容器做 conformance | 主路径，仅特定仓 |
| 合并后删分支 | `delete-merged-branch.yml` | push 到默认分支后尝试删除 merged branch | 兜底路径 |

## 4. 全局设计原则

### 4.1 治理与代码执行分离

`pr-governance.yml` 只处理治理判断，不 checkout PR 代码，不运行来自 PR 分支的脚本。它依赖 `pull_request_target` 读取 PR 元数据和 review 状态，然后决定：

- 是否直接通过 `governance`
- 是否请求 `costa92` review
- 是否为 owner PR 开启 auto-merge
- 是否在 merge 完成后删除同仓库 head branch

对应实现见：

- `llm-agent/.github/workflows/pr-governance.yml:3-15`
- `llm-agent/.github/workflows/pr-governance.yml:17-67`
- `llm-agent/.github/workflows/pr-governance.yml:69-133`

### 4.2 每仓库 CI 独立存在

每个 repo 都有自己的 CI，不依赖 umbrella 才能发现基础构建错误。这样任何单仓 PR 都能在本仓库内完成最小闭环。

### 4.3 多仓联动验证只放在核心仓

只有 `llm-agent` 有 `umbrella.yml`，因为只有核心仓的 API 变化天然会向全生态辐射。把跨仓验证放在所有仓库上会显著放大 CI 成本，而且很多联动在下游仓库 PR 上并不需要每次重跑。

### 4.4 `deleteBranchOnMerge` 不是唯一依赖

仓库级 `deleteBranchOnMerge = true` 当前仍然开启，但最终主删除路径不是 GitHub 仓库设置本身，也不是独立 cleanup workflow，而是 `pr-governance.yml` 在 owner auto-merge 链路里直接删分支。

### 4.5 `GOWORK=off` 是默认 CI 约束

除 umbrella 外，所有 Go CI workflow 都显式设定 `GOWORK=off`，避免本地 workspace 行为污染 CI 结果。

示例：

- `llm-agent/.github/workflows/test.yml:13-15`
- `llm-agent-providers/.github/workflows/test.yml:16-18`
- `llm-agent-customer-support/.github/workflows/test.yml:16-18`

## 5. PR 治理主路径

### 5.1 触发器与权限

`pr-governance.yml` 统一监听：

- `pull_request_target`: `opened`, `reopened`, `synchronize`, `ready_for_review`
- `pull_request_review`: `submitted`, `dismissed`, `edited`

统一权限为：

- `contents: write`
- `pull-requests: write`

这是 owner auto-merge 成功所需的最低写权限组合。少了 `contents: write`，`gh pr merge --auto` 会因为 `enablePullRequestAutoMerge` 权限不足而失败。

见：

- `llm-agent/.github/workflows/pr-governance.yml:3-15`

### 5.2 `governance` job

`governance` job 的判断逻辑是：

```text
if draft:
  pass (informational)
elif author == costa92:
  pass
else:
  request review from costa92
  if latest costa92 APPROVED review targets current head SHA:
    pass
  else:
    fail
```

关键点：

- external PR 不是“只要审过一次就行”，而是必须审当前 head。
- `COMMENTED` review 不算审批。
- review request 是幂等容错的，`gh api ... requested_reviewers || true` 不会因为重复请求导致整条链路失败。

见：

- `llm-agent/.github/workflows/pr-governance.yml:21-67`

### 5.3 `auto-merge-owner` job

`auto-merge-owner` 只对 owner PR 生效，逻辑分 4 段：

1. Draft PR 直接跳过。
2. 非 owner PR 直接跳过，治理可能通过，但合并保持手动。
3. 若 auto-merge 尚未开启，则执行 `gh pr merge --auto --merge`。
4. 轮询 PR merged 状态；若 PR 已真实 merged，则删除同仓库 head branch。

关键设计：

- 先查 `autoMergeRequest != null`，保证幂等。
- 不再把主删除路径依赖在 `--delete-branch` 上。
- 删除前明确跳过 fork branch 和默认分支。

见：

- `llm-agent/.github/workflows/pr-governance.yml:69-133`

### 5.4 为什么分支删除内嵌在 `pr-governance.yml`

这次治理设计最终收敛到“合并与删分支同源完成”，原因是独立 cleanup workflow 在实际 auto-merge 链路里不够稳定。

也就是说，最终主路径是：

```text
owner PR opened
  -> governance pass
  -> auto-merge-owner enables auto-merge
  -> PR merged
  -> same workflow polls MERGED
  -> same workflow deletes same-repo head branch
```

而不是：

```text
owner PR merged
  -> expect some downstream workflow to be triggered later
  -> hope that workflow deletes the branch
```

## 6. `delete-merged-branch.yml` 的定位

所有 6 个仓库都保留了 `delete-merged-branch.yml`，它的触发器是默认分支 `push`，做法是：

1. 根据 merge commit 查关联 PR。
2. 找到 same-repo head branch。
3. 如果分支仍存在，则删除它。

对应实现见：

- `llm-agent/.github/workflows/delete-merged-branch.yml:3-15`
- `llm-agent/.github/workflows/delete-merged-branch.yml:17-82`

当前定位不是主路径，而是兜底路径：

- 如果 `pr-governance.yml` 已经删掉分支，这个 workflow 会读到 404 并正常退出。
- 如果 merge 完成时 `pr-governance.yml` 没赶上 merged 可见性窗口，这个 workflow 仍有机会在 push 到默认分支后补删。

设计取舍：

- 保留它可以提高鲁棒性。
- 但不再把它写成治理设计的唯一或主要删除机制。

## 7. 每仓库 CI 设计

### 7.1 `llm-agent` 的 `test.yml`

核心仓 CI 比普通仓多两层：

- 主模块 `go mod tidy` drift check
- `examples/` 子模块 `go mod tidy` drift check

然后再执行：

- `go vet`
- `go build`
- `go test`
- `examples` 的 `vet + build`

见：

- `llm-agent/.github/workflows/test.yml:16-60`

### 7.2 `llm-agent-rag` 的 `test.yml`

`rag` 的 CI 有两条独特约束：

1. 核心包不能直接 import `github.com/costa92/llm-agent`，只有 `adapter/` 允许。
2. 必须覆盖 build tag `llmagent` 下的适配器构建与测试。

此外它还有显式 API snapshot gate，用于把 `internal/apisnapshot` 的可见性拉到单独 step。

见：

- `llm-agent-rag/.github/workflows/test.yml:16-57`

### 7.3 `llm-agent-flow`、`llm-agent-providers`、`llm-agent-otel`

这 3 个仓库的 `test.yml` 目前是单 job 结构：

- `go mod tidy` drift check
- `go vet`
- `go build`
- `go test`

见：

- `llm-agent-flow/.github/workflows/test.yml:16-41`
- `llm-agent-providers/.github/workflows/test.yml:19-44`
- `llm-agent-otel/.github/workflows/test.yml:19-44`

### 7.4 `llm-agent-customer-support` 的 `ci`

`customer-support` 是最重的仓库，所以 CI 被拆成 4 个 job：

- `format`
- `go`
- `compose`
- `docker`

依赖关系：

- `go` 依赖 `format`
- `compose` 依赖 `format`
- `docker` 依赖 `go`

这意味着：

- 先挡住格式和 `go mod tidy` 漂移。
- 然后并行做 Go 校验与 compose 配置校验。
- 最后只在 Go 基础通过后再构建镜像。

见：

- `llm-agent-customer-support/.github/workflows/test.yml:19-90`

## 8. `release-precheck.yml`

`release-precheck.yml` 当前存在于：

- `llm-agent`
- `llm-agent-rag`
- `llm-agent-providers`
- `llm-agent-otel`
- `llm-agent-customer-support`

`llm-agent-flow` 当前没有这份 workflow。

统一逻辑是：

- 只在 `release/**` 的 `push` 和 `pull_request` 上触发。
- 解析 `go mod edit -json`。
- 若存在任意 `Replace` 条目，则直接失败。

见：

- `llm-agent/.github/workflows/release-precheck.yml:6-36`
- `llm-agent-providers/.github/workflows/release-precheck.yml:6-36`

设计目的：

- 本地开发允许 `replace`。
- 发布分支绝不允许把本地路径依赖带入 release。

## 9. `llm-agent` 专属的 `umbrella.yml`

`umbrella.yml` 是生态级联动验证 workflow，只存在于 `llm-agent`：

- checkout 当前 PR 的 `llm-agent`
- checkout 下游 `providers`、`otel`、`customer-support`、`rag`
- 校验 `scripts/workspace.sh` 在 4 个核心 checkout 中字节一致
- 跑 dependency currency gate
- 临时生成 `go.work`
- 用当前 PR 的 `llm-agent` 构建全部下游

见：

- `llm-agent/.github/workflows/umbrella.yml:14-122`

它解决的问题不是“单仓能不能过 CI”，而是“核心 API 改了以后，下游还能不能一起编译通过”。

当前限制：

- 它还没有把 `llm-agent-flow` 纳入 cross-repo build 列表。
- 也没有为所有仓库统一出 reusable workflow。

这两个点都是后续可以继续演进的方向，但不影响它作为当前主联动闸门的定位。

## 10. `llm-agent-providers` 专属的 `nightly-ollama-live.yml`

这是唯一一个夜间真实环境 conformance workflow，触发器只有：

- `schedule`
- `workflow_dispatch`

它显式不进入 PR CI，原因是：

- 真实 Ollama 容器测试耗时长
- 依赖缓存与 Docker 环境
- 适合作为定时健康检查，不适合作为每个 PR 的必跑项

执行内容：

- 缓存 Ollama 模型卷
- 确认 Docker 可用
- 只跑 `TestGenerate_Ollama_Live`

见：

- `llm-agent-providers/.github/workflows/nightly-ollama-live.yml:5-47`

## 11. 端到端状态机

### 11.1 Owner PR

```text
owner opens PR
  -> repo CI starts
  -> pr-governance / governance passes immediately
  -> pr-governance / auto-merge-owner enables auto-merge
  -> required checks all pass
  -> GitHub merges PR
  -> same pr-governance run polls MERGED
  -> same pr-governance run deletes same-repo head branch
  -> if branch still survives, delete-merged-branch may later delete it on default-branch push
```

### 11.2 External PR

```text
external contributor opens PR
  -> repo CI starts
  -> pr-governance requests costa92 review
  -> governance fails until costa92 approves current head
  -> after current-head approval, governance passes
  -> merge remains manual
  -> after merge, repo setting or delete-merged-branch may clean branch, but fork branches are skipped
```

## 12. 为什么现在还保留重复 YAML

当前每个仓库仍保留一份自己的 workflow 文件，而不是抽成 reusable workflow，原因是：

1. 现在的优先级是“先稳定治理与验证链路”。
2. 各仓库 CI 细节确实不同，短期内硬抽公共模板会把差异藏起来。
3. `pr-governance.yml` 虽然高度一致，但这次刚完成跨仓推广，先保持每仓可独立回滚更稳妥。

这意味着今天的设计重点是“行为一致”，而不是“YAML 去重”。

## 13. 当前已知限制

1. `delete-merged-branch.yml` 仍然存在于全部仓库，但已降级为兜底，不应再被视为主删除路径。
2. `umbrella.yml` 目前未纳入 `llm-agent-flow`。
3. `release-precheck.yml` 当前没有推广到 `llm-agent-flow`。
4. `pr-governance.yml` 里的 merged 轮询窗口是有限的；如果 GitHub merged 可见性超出窗口，主删除路径会让位给 repo 设置或兜底 workflow。
5. 目前没有统一 reusable workflow 层，跨仓修改时需要同步更新多份 YAML。

## 14. 推荐运维检查清单

1. 确认每仓库默认分支仍要求 `go` 与 `governance` 作为 required checks。
2. 确认每仓库 `allow_auto_merge = true`。
3. 确认每仓库 `deleteBranchOnMerge = true`。
4. 确认 `pr-governance.yml` 仍包含 `contents: write` 与 `pull-requests: write`。
5. 确认 owner PR 仍能自动合并并删除同仓库分支。
6. 确认 external PR 仍会自动请求 `costa92` review 且要求 current-head approval。
7. 确认 `llm-agent` 的 `umbrella.yml` 仍能拉起下游联动编译。
8. 确认 `release/**` 分支上的 `replace` 仍会被拒绝。

## 15. 一句话结论

这套 GitHub workflows 设计本质上是 3 层：

- 每仓库独立 CI 保证局部正确性。
- `pr-governance.yml` 保证 author-sensitive 合并治理，并承担 owner PR 合并后删分支的主路径。
- `llm-agent` 的 `umbrella.yml` 补上多仓库体系里单仓 CI 看不到的联动兼容性验证。

## 延伸阅读

- [`../llm-agent/docs/PR-GOVERNANCE-OVERVIEW.md`](../llm-agent/docs/PR-GOVERNANCE-OVERVIEW.md)
- [`../llm-agent/docs/PR-GOVERNANCE-RULES.md`](../llm-agent/docs/PR-GOVERNANCE-RULES.md)
- [`../llm-agent/docs/PR-GOVERNANCE-OPERATIONS.md`](../llm-agent/docs/PR-GOVERNANCE-OPERATIONS.md)
- [`./source-design-umbrella-root.zh-CN.md`](./source-design-umbrella-root.zh-CN.md)
