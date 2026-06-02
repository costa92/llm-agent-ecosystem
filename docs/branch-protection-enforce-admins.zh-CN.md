# Branch Protection 与 `enforce_admins` 修改手册

> 文档版本：2026-06-02
> 适用范围：`costa92/llm-agent-*` 全部 13 个仓库的默认分支

这份文档专门解决一件事：**你想根据情况，调整某个仓库默认分支上「管理员（你自己）是否也受 branch protection 约束」这个开关（`enforce_admins`）。**

它回答：当前是什么状态、这个开关控制什么、在哪里改、怎么改、按什么标准选、改完怎么验证。

---

## 1. `enforce_admins` 是什么

它是 GitHub branch protection 里的一个布尔开关，控制 **仓库管理员（admin / owner）是否也必须遵守该分支的保护规则**。

| 值 | 对管理员（你）的影响 | 对非管理员的影响 |
|---|---|---|
| **`true`**（当前全仓默认） | 你**也**受约束：不能 `git push` 直推默认分支；PR 必须等 required checks（`go` / `governance`）全绿才能合并，不能强制提前合 | 一直受约束（与开关无关） |
| **`false`** | 你可以**绕过**保护：能直接 `git push` 默认分支、能在 checks 没绿时强制合并 PR | 一直受约束（与开关无关） |

一句话：`enforce_admins` 只决定**「你自己要不要守规矩」**。它不会放松对其他人的限制。

### 一个重要澄清（避免误解）

**`enforce_admins=true` 不会妨碍自动合并（auto-merge）。** `pr-governance.yml` 用 `GITHUB_TOKEN` 在 required checks 满足后触发的合并，属于**合法合并**，不是「绕过保护」，因此不受 `enforce_admins` 影响。

也就是说：**你不需要为了让 auto-merge 工作而把 `enforce_admins` 关成 `false`。** 当前全仓 `enforce_admins=true` + auto-merge 同时正常工作，已验证。

---

## 2. 当前配置快照（实测 2026-06-02）

全部 13 个仓库的默认分支当前都已开启保护，配置一致：

| 仓库 | 分支 | required checks | strict | **enforce_admins** | auto_merge | del_branch |
|---|---|---|---|---|---|---|
| `llm-agent-ecosystem` | main | `governance` | ✅ | **true** | ✅ | ✅ |
| `llm-agent` | main | `go`+`governance` | ✅ | **true** | ✅ | ✅ |
| `llm-agent-rag` | master | `go`+`governance` | ✅ | **true** | ✅ | ✅ |
| `llm-agent-otel` | main | `go`+`governance` | ✅ | **true** | ✅ | ✅ |
| `llm-agent-providers` | main | `go`+`governance` | ✅ | **true** | ✅ | ✅ |
| `llm-agent-customer-support` | main | `go`+`governance` | ✅ | **true** | ✅ | ✅ |
| `llm-agent-flow` | main | `go`+`governance` | ✅ | **true** | ✅ | ✅ |
| `llm-agent-memory` | main | `go`+`governance` | ✅ | **true** | ✅ | ✅ |
| `llm-agent-memory-contract` | main | `go`+`governance` | ✅ | **true** | ✅ | ✅ |
| `llm-agent-memory-gateway` | main | `go`+`governance` | ✅ | **true** | ✅ | ✅ |
| `llm-agent-memory-postgres` | main | `go`+`governance` | ✅ | **true** | ✅ | ✅ |
| `llm-agent-memory-worker` | main | `go`+`governance` | ✅ | **true** | ✅ | ✅ |
| `llm-agent-memory-client` | main | `go`+`governance` | ✅ | **true** | ✅ | ✅ |

> `umbrella`（`llm-agent-ecosystem`）的 required check 只设了轻量的 `governance`，**故意不含** `cross-repo-build`（它要构建全部 12 仓、约 15–30 分钟，设为 required 会让纯文档 PR 也等半小时）。

---

## 3. 该选 `true` 还是 `false`？（决策依据）

| 你的情况 / 诉求 | 推荐值 | 理由 |
|---|---|---|
| 想要质量门（CI 没绿不让合）+ 防孤儿 commit + 全仓行为一致 | **`true`**（当前默认） | required check 让 auto-merge 必须等 CI；补推会重置 checks，不会再出现「PR 秒合、补推变孤儿」 |
| 某个仓你需要频繁**直接 `git push` 默认分支**做快速实验/小修 | **`false`** | 保留对该仓 main 的直推能力；非管理员仍受 required check 约束 |
| 需要偶尔**强制合并**一个 CI 卡住但你确信没问题的 PR | **`false`**（或临时关→合→再开） | `true` 下管理员也无法 override required check |
| 不确定 | **保持 `true`** | 这是当前默认，也是 6 个核心 Go 仓长期运行的配置 |

> 「孤儿 commit」指：PR 开启后 owner PR 被秒合关闭，你随后补推到该分支的 commit 进不了 main、且静默无报错。`enforce_admins=true` + required check 是它的机制防线。详见旧 runbook 与 `pr-governance` 文档。

---

## 4. 在哪里改 / 怎么改

### 方式 A：`gh` CLI（推荐——最精确，只动 `enforce_admins`，不碰 required checks）

GitHub 为 `enforce_admins` 提供了**专用子资源**，可以单独切换而不影响保护规则的其它字段：

```bash
# 读取当前值
gh api repos/costa92/<repo>/branches/<branch>/protection/enforce_admins --jq '.enabled'

# 关闭（true → false：允许管理员绕过保护）
gh api -X DELETE repos/costa92/<repo>/branches/<branch>/protection/enforce_admins

# 开启（false → true：管理员也受约束）
gh api -X POST   repos/costa92/<repo>/branches/<branch>/protection/enforce_admins
```

把 `<repo>` 换成仓名、`<branch>` 换成默认分支（`llm-agent-rag` 是 `master`，其余是 `main`）。例如关闭 `llm-agent-memory` 的：

```bash
gh api -X DELETE repos/costa92/llm-agent-memory/branches/main/protection/enforce_admins
```

> ⚠️ **不要用** `gh api -X PUT .../protection` 来切这一个开关——`PUT` 会**整体替换**保护配置，漏写任何字段（如 `required_status_checks`）都会把 required checks 一并清空。专用子资源 `POST`/`DELETE` 不会有这个风险。

### 方式 B：GitHub 网页 UI

1. 打开仓库 → **Settings** → 左侧 **Branches**（或 **Rules → Rulesets**，取决于该仓用的是哪种）。
2. 找到默认分支的 **Branch protection rule** → **Edit**。
3. 找到这一项并勾选 / 取消勾选：
   - 新版措辞：**"Do not allow bypassing the above settings"**
   - 旧版措辞：**"Include administrators"**
   - **勾选 = `enforce_admins=true`**；取消勾选 = `false`。
4. **Save changes**。

---

## 5. 批量修改（一次改多个仓）

例如想把全部 6 个 memory 仓的 `enforce_admins` 关掉：

```bash
for r in llm-agent-memory llm-agent-memory-contract llm-agent-memory-gateway \
         llm-agent-memory-postgres llm-agent-memory-worker llm-agent-memory-client; do
  gh api -X DELETE "repos/costa92/$r/branches/main/protection/enforce_admins" \
    && echo "$r: enforce_admins -> false"
done
```

反向（开启）把 `-X DELETE` 换成 `-X POST` 即可。

---

## 6. 改完怎么验证

```bash
# 单仓
gh api repos/costa92/<repo>/branches/<branch>/protection/enforce_admins --jq '.enabled'

# 全 13 仓巡检（rag 是 master，其余 main）
for r in llm-agent-ecosystem llm-agent llm-agent-otel llm-agent-providers \
         llm-agent-customer-support llm-agent-flow llm-agent-memory \
         llm-agent-memory-contract llm-agent-memory-gateway \
         llm-agent-memory-postgres llm-agent-memory-worker llm-agent-memory-client; do
  printf "%-30s enforce_admins=%s\n" "$r" \
    "$(gh api repos/costa92/$r/branches/main/protection/enforce_admins --jq '.enabled' 2>/dev/null)"
done
printf "%-30s enforce_admins=%s\n" "llm-agent-rag" \
  "$(gh api repos/costa92/llm-agent-rag/branches/master/protection/enforce_admins --jq '.enabled')"
```

关掉后想确认「直推 main 是否真的放开了」：在该仓本地 `git push origin main` 一个无害提交即可（`true` 时会被拒，`false` 时会成功）。

---

## 7. 注意事项

1. **需要 admin 权限的 token。** `gh` 当前登录账号必须是仓库管理员，否则 `POST`/`DELETE` 会 403。
2. **`required_status_checks` 的 check 名必须真实存在**（`go` = `test.yml` 的 job 名，`governance` = `pr-governance.yml` 的 job 名）。若用 `PUT` 重配保护时填了不存在的 check 名，PR 会**永久卡死**等不到该 check。改 `enforce_admins` 用专用子资源就不碰这个。
3. **关掉 `enforce_admins` 不会关掉对外部贡献者的约束**——它只放开管理员自己。
4. **`enforce_admins` 与 auto-merge 无关**（见 §1 的澄清），不要为了 auto-merge 去动它。
5. 完全移除某分支的保护（慎用）：`gh api -X DELETE repos/costa92/<repo>/branches/<branch>/protection`。

---

## 延伸阅读

- [`./github-repo-settings-runbook.zh-CN.md`](./github-repo-settings-runbook.zh-CN.md) — 仓库设置核对清单（注：其 2026-05-22 快照只覆盖 6 个 Go 仓；截至本文档，全 13 仓均已开启保护）
- [`./github-workflows-design.zh-CN.md`](./github-workflows-design.zh-CN.md) — workflow 设计
- [`../llm-agent/docs/PR-GOVERNANCE-OPERATIONS.md`](../llm-agent/docs/PR-GOVERNANCE-OPERATIONS.md) — PR 治理链路运维
