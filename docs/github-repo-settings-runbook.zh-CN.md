# GitHub 仓库设置与 Branch Protection 运维清单

> 文档版本：2026-05-22
> 对应代码快照：2026-05-22

本文档不是解释 workflow 设计本身，而是给维护者一份可以直接核对的 GitHub 仓库设置清单，确保 GitHub 仓库侧配置和仓库内 YAML 设计一致。

它回答的是：

1. 每个仓库 GitHub Settings 里必须打开什么。
2. Branch protection / Rulesets 应该要求什么。
3. 哪些设置如果配错，会直接让自动合并或自动删分支失效。

## 1. 适用仓库

- `llm-agent`
- `llm-agent-rag`
- `llm-agent-flow`
- `llm-agent-providers`
- `llm-agent-otel`
- `llm-agent-customer-support`

## 2. 仓库级必需设置

所有 6 个仓库都应保持以下仓库级设置：

1. `Allow auto-merge = true`
2. `Automatically delete head branches = true`

原因：

- `Allow auto-merge` 是 `gh pr merge --auto --merge` 成功的前提。
- `Automatically delete head branches` 现在是安全网，不是主路径，但仍应开启。

## 3. 默认分支清单

| Repo | Default Branch |
|---|---|
| `llm-agent` | `main` |
| `llm-agent-rag` | `master` |
| `llm-agent-flow` | `main` |
| `llm-agent-providers` | `main` |
| `llm-agent-otel` | `main` |
| `llm-agent-customer-support` | `main` |

## 4. Branch Protection / Rulesets 最小要求

默认分支保护应至少满足：

1. Require status checks before merging
2. Required checks 中包含：
   - `go`
   - `governance`
3. 不再依赖 GitHub 内建 required approving review 作为主 merge gate

对于 `llm-agent-customer-support`，由于 CI 结构更重，通常还会看到：

- `format`
- `compose`
- `docker`

但治理层的关键要求仍是 `go` 与 `governance`。

## 5. 为什么不再把 GitHub approving review 设为主门禁

因为这套治理的目标是：

- owner PR 自动放行
- external PR 必须 owner 审核

GitHub 内建 required approving review 无法表达这条 author-sensitive 规则，所以真正的 merge gate 必须交给 `governance` check，而不是平台原生 review gate。

## 6. PR 治理链路要求

只要下面任一项配置错误，`pr-governance.yml` 就可能失效：

1. 仓库关闭了 `Allow auto-merge`
2. 默认分支 required checks 没有要求 `governance`
3. workflow 文件没有在默认分支上
4. 仓库管理员把 `pr-governance.yml` 的权限需求改弱

正确链路是：

```text
owner PR
  -> governance pass
  -> auto-merge-owner enables auto-merge
  -> PR merged
  -> same workflow deletes same-repo head branch
```

## 7. 合并后删分支链路要求

当前删分支设计分成两层：

### 主路径

- `pr-governance.yml` 内的 `auto-merge-owner` job
- 在 PR 真实 merged 后删除同仓库 head branch

### 兜底路径

- 仓库设置 `Automatically delete head branches`
- `delete-merged-branch.yml`

运维上应该把主路径视为：

- “workflow 负责删除”

而不是：

- “只靠 GitHub repo setting 删除”

## 8. 受保护分支与直推预期

对启用了受保护默认分支的仓库，直接 `git push origin main` 可能会被 GitHub 拒绝，并显示 required checks 未满足。

这不是异常，而是预期行为。正确流程是：

1. 推分支
2. 创建 PR
3. 让 `go` / `governance` 跑完
4. owner PR 进入 auto-merge
5. 合并后由 workflow 删除分支

## 9. 每仓库核对表

### `llm-agent`

- default branch: `main`
- required checks: `go`, `governance`
- special workflow: `umbrella.yml`

### `llm-agent-rag`

- default branch: `master`
- required checks: `go`, `governance`
- special CI behavior: adapter build tag + API snapshot gate

### `llm-agent-flow`

- default branch: `main`
- required checks: `go`, `governance`
- note: 当前未见 `release-precheck.yml`

### `llm-agent-providers`

- default branch: `main`
- required checks: `go`, `governance`
- special workflow: `nightly-ollama-live.yml`

### `llm-agent-otel`

- default branch: `main`
- required checks: `go`, `governance`

### `llm-agent-customer-support`

- default branch: `main`
- required checks 至少应覆盖：`go`, `governance`
- repo CI 还包含：`format`, `compose`, `docker`

## 10. 推荐巡检步骤

每次改 workflow 或 GitHub 规则后，建议做一次完整巡检：

1. 打开仓库 Settings，确认 `Allow auto-merge = true`
2. 确认 `Automatically delete head branches = true`
3. 打开默认分支 protection / ruleset，确认 required checks 仍包含 `go` 和 `governance`
4. 发一个 owner 测试 PR，确认：
   - `governance` 自动通过
   - auto-merge 被开启
   - merge 后同仓库分支被删掉
5. 发一个 external 测试 PR，确认：
   - 自动 request review 给 `costa92`
   - 未审批 current head 前 `governance` 为失败
   - 审批 current head 后 `governance` 变绿

## 11. 常见错误配置

1. 打开了 `go`，但忘了把 `governance` 设成 required check。
2. 还保留 GitHub required approving review，结果 owner PR 继续被平台卡住。
3. 关闭了 `Allow auto-merge`，导致 workflow 虽然执行但无法真正开启 auto-merge。
4. 把 `deleteBranchOnMerge` 当成唯一删除机制，忽略了主删除路径其实在 `pr-governance.yml`。
5. 在 PR 分支里修改 workflow 后，误以为当前这个 PR 会立刻使用新 workflow；但 `pull_request_target` 实际使用的是默认分支版本。

## 12. 一句话结论

GitHub 仓库设置层的职责很简单：

- 给 `pr-governance.yml` 提供可以工作的环境
- 不要再让平台原生 review gate 和自定义治理 gate 打架
- 把 repo setting 的自动删分支保留为安全网，而不是主路径

## 延伸阅读

- [`./github-workflows-design.zh-CN.md`](./github-workflows-design.zh-CN.md)
- [`../llm-agent/docs/PR-GOVERNANCE-OVERVIEW.md`](../llm-agent/docs/PR-GOVERNANCE-OVERVIEW.md)
- [`../llm-agent/docs/PR-GOVERNANCE-OPERATIONS.md`](../llm-agent/docs/PR-GOVERNANCE-OPERATIONS.md)
