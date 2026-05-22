# GitHub Web UI 配置步骤清单

> 文档版本：2026-05-22
> 对应代码快照：2026-05-22

本文档给维护者一份可以直接照着点击的 GitHub Web UI 配置步骤，用来把仓库 Settings、Pull Requests、Rules / Branch protection 配置到和当前 workflows 设计一致。

和 [`github-repo-settings-runbook.zh-CN.md`](./github-repo-settings-runbook.zh-CN.md) 的区别是：

- `github-repo-settings-runbook.zh-CN.md` 讲“该配什么”
- 本文档讲“去哪里点、按什么顺序配”

## 1. 适用仓库

- `llm-agent`
- `llm-agent-rag`
- `llm-agent-flow`
- `llm-agent-providers`
- `llm-agent-otel`
- `llm-agent-customer-support`

## 2. 先确认默认分支

先确认每个仓库的默认分支名称，因为后面 rules 绑定时要用到：

| Repo | Default Branch |
|---|---|
| `llm-agent` | `main` |
| `llm-agent-rag` | `master` |
| `llm-agent-flow` | `main` |
| `llm-agent-providers` | `main` |
| `llm-agent-otel` | `main` |
| `llm-agent-customer-support` | `main` |

## 3. Repository Settings 页面需要打开的选项

### 3.1 打开仓库 Settings

点击路径：

1. 打开目标仓库主页
2. 顶部点击 `Settings`
3. 如果顶栏没有直接显示 `Settings`，先点下拉菜单，再点 `Settings`

## 3.2 打开 `Allow auto-merge`

点击路径：

1. 进入 `Settings`
2. 左侧保持在 `General`
3. 向下滚动到 `Pull Requests`
4. 勾选 `Allow auto-merge`

为什么必须开：

- 当前 `pr-governance.yml` 使用的是 `gh pr merge --auto --merge`
- 如果仓库级没开 `Allow auto-merge`，workflow 无法真正开启自动合并

## 3.3 打开 `Automatically delete head branches`

点击路径：

1. 仍在 `Settings > General`
2. 在 `Pull Requests` 区域
3. 勾选 `Automatically delete head branches`

为什么还要开：

- 当前主删除路径在 `pr-governance.yml`
- 但 repo setting 仍然是安全网

## 3.4 打开 `Allow merge commits`

点击路径：

1. 仍在 `Settings > General`
2. 在 `Pull Requests` 区域
3. 勾选 `Allow merge commits`

为什么这个容易漏：

- 当前治理 workflow 用的是 `gh pr merge --auto --merge`
- 这要求仓库允许 merge commit
- 如果仓库禁用了 merge commits，而 workflow 仍然用 `--merge`，owner PR 自动合并会失败

## 3.5 关于 `Allow squash merging` / `Allow rebase merging`

这两项不是当前治理链路的硬要求。

可以保持开启，也可以不开，但要注意：

- 当前自动合并脚本显式使用 `--merge`
- 所以真正必须保证的是 `Allow merge commits`

## 4. 配置默认分支保护

GitHub 现在常见有两种做法：

1. `Rules > Rulesets`
2. `Branches > Branch protection rules`

两种都能实现目标。若仓库已经在用 rulesets，优先继续用 rulesets；若仓库还在用传统 branch protection rule，也可以继续沿用。

## 5. 用 Rulesets 配置默认分支

### 5.1 进入 Rulesets

点击路径：

1. `Settings`
2. 左侧点击 `Rules`
3. 点击 `Rulesets`
4. 新建或编辑针对默认分支的 ruleset

### 5.2 选择目标分支

在 ruleset 里：

1. 选择 branch ruleset
2. target 选择默认分支
3. `llm-agent-rag` 选 `master`
4. 其他仓库选 `main`

### 5.3 开启 merge 约束

在 ruleset 里，至少保证：

1. 要求 pull request 才能进入默认分支
2. Require status checks

### 5.4 Required checks 里加入什么

最小要求：

- `go`
- `governance`

对于 `llm-agent-customer-support`，通常还应包含：

- `format`
- `compose`
- `docker`

### 5.5 不要把 GitHub approving review 设成主门禁

如果你的目标是当前这套 author-sensitive 治理，不要再把平台原生 “required approving review” 当成主 gate。

原因：

- owner PR 需要自动放行
- external PR 需要 owner 审核
- 这条规则是 `governance` check 表达的，不是 GitHub 内建 approval 规则表达的

### 5.6 Ruleset 启用状态

确认 ruleset 最终是：

- `Active`

而不是 `Evaluate` 或 `Disabled`

## 6. 用 Branch Protection Rule 配置默认分支

如果仓库还没迁到 rulesets，可以走传统入口。

### 6.1 进入 Branch protection rules

点击路径：

1. `Settings`
2. 左侧点击 `Branches`
3. 找到 `Branch protection rules`
4. 点击 `Add rule` 或编辑现有规则

### 6.2 Branch name pattern

填写默认分支：

- `main`
- 或 `master`（仅 `llm-agent-rag`）

### 6.3 必须勾选的项

至少勾选：

1. `Require a pull request before merging`
2. `Require status checks to pass before merging`

然后在 required checks 列表里加入：

- `go`
- `governance`

对于 `llm-agent-customer-support`，再加入：

- `format`
- `compose`
- `docker`

### 6.4 不建议继续依赖的平台审批项

如果你还保留：

- `Require approvals`
- `Require approval of the most recent reviewable push`

就会和当前 `governance` 设计重叠甚至冲突。

如果仍强制平台原生 approval，owner PR 可能继续被 GitHub 自己卡住。

## 7. 每个仓库配完后的最小验收

### 7.1 owner PR 验收

发一个 owner 测试 PR，确认：

1. `governance` 自动通过
2. `auto-merge-owner` 被触发
3. auto-merge 被开启
4. PR 在 required checks 通过后自动合并
5. 合并后同仓库分支被删掉

### 7.2 external PR 验收

发一个 external 测试 PR，确认：

1. 自动 request review 给 `costa92`
2. `governance` 在未审批 current head 前失败
3. `costa92` 审批当前 head 后 `governance` 变绿
4. 合并仍保持手动

## 8. 当前仓库差异提醒

### `llm-agent`

- 除常规 CI 外，还有 `umbrella.yml`
- 这是核心仓的跨仓兼容性验证，不需要在其他仓库设置额外 UI 选项

### `llm-agent-rag`

- 默认分支是 `master`
- 配 rules 时最容易误填成 `main`

### `llm-agent-flow`

- 当前仓库里还没有 `release-precheck.yml`
- 所以 release 分支侧的 UI 配置不会自动多出这项 workflow check

### `llm-agent-providers`

- 有 `nightly-ollama-live.yml`
- 这是 schedule / 手动触发 workflow，不应被加进默认分支 required checks

### `llm-agent-customer-support`

- CI job 比其他仓库多
- 若把 `go`, `governance` 之外的 job 也设成 required，需要确保名称与 workflow 产出的 check name 完全一致

## 9. 最容易配错的 6 个点

1. 忘了打开 `Allow auto-merge`
2. 忘了打开 `Allow merge commits`
3. 忘了打开 `Automatically delete head branches`
4. required checks 里加了 `go`，但没加 `governance`
5. 还保留 GitHub 原生 required approval，导致 owner PR 继续被卡
6. 把 `nightly-ollama-live` 这种非 PR CI workflow 误加成 required check

## 10. 推荐配置顺序

建议按下面顺序配置，能减少来回返工：

1. 先确认默认分支名称
2. 打开 `Settings > General > Pull Requests`
3. 勾选：
   - `Allow auto-merge`
   - `Automatically delete head branches`
   - `Allow merge commits`
4. 再去 `Rules > Rulesets` 或 `Branches > Branch protection rules`
5. 配 required checks：
   - `go`
   - `governance`
   - 以及 repo-specific checks
6. 保存后发 owner 测试 PR 验证
7. 再发 external 测试 PR 验证

## 11. 后续统一提交约束

配置完成后，后续提交代码的标准流程应固定为：

1. 切分支
2. 提交代码
3. 推分支
4. 创建 PR
5. 等待 `go` / `governance`
6. 让 owner PR 自动合并，或让 external PR 经 `costa92` 审核后合并

也就是说，这份 Web UI 配置不是为了“偶尔自动化一下”，而是为了把后续所有代码提交都统一到同一套 PR 入口规则上。

## 12. 一句话结论

按 GitHub Web UI 配置这套仓库时，真正不能漏的 3 个仓库级开关是：

- `Allow auto-merge`
- `Automatically delete head branches`
- `Allow merge commits`

真正不能漏的默认分支 required checks 是：

- `go`
- `governance`

## 延伸阅读

- [`./github-repo-settings-runbook.zh-CN.md`](./github-repo-settings-runbook.zh-CN.md)
- [`./github-workflows-design.zh-CN.md`](./github-workflows-design.zh-CN.md)
- [`./github-workflows-design.md`](./github-workflows-design.md)
