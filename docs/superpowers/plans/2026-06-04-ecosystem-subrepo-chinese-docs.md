# 全生态子仓库文档中文化 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 16 个子仓库的全部用户文档新建配对 `.zh-CN.md` 中文版，并先补全 5 处缺失/桩文档的英文内容。

**Architecture:** 方案 B——先产出共享术语表（L0）统一译名；再顺序补桩仓英文内容（L1）；然后每仓一个翻译任务并行成波（L2），引用术语表；每仓改动以独立分支+PR 提交、自动合并。

**Tech Stack:** Markdown、git、gh CLI；执行以子 agent 翻译/撰写为主，主线审查。

---

## 关键约定（所有任务共享）

**翻译规则（每个翻译 agent 必须遵守）：**
1. 英文原文 `X.md` 不动；新增 `X.zh-CN.md` 完整中文版。
2. 章节层级、标题顺序、表格、列表、代码块与原文 1:1。
3. 不翻译：代码块内容、命令、API/类型/函数/包名等标识符、URL、文件路径、版本号、分支/tag 名。
4. 保留原文头部元信息（如 `> Document version / Code snapshot`），值不变。
5. 强制引用术语表 `docs/superpowers/specs/2026-06-04-chinese-docs-glossary.md`，禁止同术语多译法。

**README 互链行**：仅 `README.md` ↔ `README.zh-CN.md` 顶部各加一行
`[English](./README.md) | [简体中文](./README.zh-CN.md)`
（docs/ 内文档不加切换行，保持平行文件风格；加互链行需同时补到英文 README 顶部）。

**每篇验证标准：**
- (a) 结构与英文版 1:1；(b) 无成段未译英文（标识符/URL/代码例外）；(c) 代码块/表格/链接/图片完好；(d) 术语与术语表一致。

**提交方式（每仓一次）：** 在该仓目录内 `git checkout -b docs/zh-cn` → 加文件 → commit → `git push -u origin docs/zh-cn` → `gh pr create`。受保护 main 的仓由 owner PR 自动合并、自动删分支。docs-only/新增文件不触 `go.mod`，CI 可过。

---

## Phase L0 — 术语表

### Task 0: 产出共享术语表

**Files:**
- Create: `docs/superpowers/specs/2026-06-04-chinese-docs-glossary.md`（umbrella 仓）

- [ ] **Step 1: 抽样核心文档提取高频术语**

读取以下代表性文档，提取反复出现的领域/工程术语：
`llm-agent/README.md`、`llm-agent-rag/README.md`、`llm-agent-rag/docs/graphrag.md`、`llm-agent-memory-gateway/README.md`、`llm-agent-flow/docs/architecture.md`、`llm-agent-contract/README.md`、`llm-agent-otel/README.md`。

- [ ] **Step 2: 写术语表**

格式：`| 英文 | 中文 | 备注 |`。至少覆盖（译法可在 Step1 抽样后微调，但须固定）：

| 英文 | 中文 | 备注 |
|---|---|---|
| agent | 智能体 | 生态名 `llm-agent` 等保留不译 |
| tool | 工具 | |
| provider | 提供方/Provider | 包名 `providers` 不译 |
| contract | 契约 | 包名/仓名不译 |
| gateway | 网关 | 仓名不译 |
| worker | 工作进程 | 仓名不译 |
| engine | 引擎 | |
| adapter | 适配器 | |
| seam | 接缝 | |
| umbrella | 伞形（工作区） | |
| retriever | 检索器 | |
| reranker | 重排器 | |
| embedding | 嵌入 | |
| chunk | 文本块 | |
| GraphRAG | GraphRAG | 保留不译 |
| reflection | 反思 | Self-RAG 语境 |
| working memory | 工作记忆 | |
| episodic memory | 情景记忆 | |
| outbox | 发件箱（outbox） | 模式名保留 |
| dedupe | 去重 | |
| guardrail | 护栏 | |
| policy | 策略 | 仓名 `policy` 不译 |
| supervisor | 督导器 | |
| observability | 可观测性 | |
| telemetry | 遥测 | |
| span / trace | span / 链路 | span 保留 |
| replace directive | replace 指令 | |
| conformance harness | 一致性测试套件 | |

- [ ] **Step 3: 自查**

确认每个术语给出了固定中文译法或明确"保留不译"；标识符类（包名/类型）标注保留。

- [ ] **Step 4: 提交（umbrella）**

```bash
cd /home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem
git checkout -b docs/zh-cn-glossary
git add docs/superpowers/specs/2026-06-04-chinese-docs-glossary.md
git commit -m "docs: add Chinese translation glossary for subrepo i18n"
git push -u origin docs/zh-cn-glossary
gh pr create --base main --head docs/zh-cn-glossary --title "docs: Chinese translation glossary" --body "Shared glossary for the ecosystem subrepo Chinese documentation effort."
```

Expected：PR 创建成功，自动合并后术语表进入 umbrella main。

---

## Phase L1 — 补桩/空仓英文内容（顺序、对照源码、逐仓审查）

> 每个 L1 任务：先读该仓 Go 源码 → 写实质英文 README → 再出 `README.zh-CN.md`。英文 README 至少含：包定位（一句话职责）、安装/导入、最小用法示例、核心导出 API/接口、与生态其他仓的依赖关系。**严禁臆造**，所有 API/用法对照源码核实。主线审查英文内容后再放行中文。

### Task 1: llm-agent-builtin 补 README

**Files:**
- Modify: `llm-agent-builtin/README.md`（现 14 行桩 → 扩充）
- Create: `llm-agent-builtin/README.zh-CN.md`

- [ ] **Step 1:** 读源码确定职责：`ls llm-agent-builtin`，读其包根 `.go` 文件与 `go.mod`，确认它在生态中提供什么内置能力（builtin tools/components）。
- [ ] **Step 2:** 扩充 `README.md`：补包定位、`go get` 导入路径、最小用法、核心导出、依赖关系；顶部加互链行。
- [ ] **Step 3:** 写 `README.zh-CN.md`（遵守翻译规则 + 互链行）。
- [ ] **Step 4:** 验证：英文内容与源码相符；中文 1:1；两文件互链可达。
- [ ] **Step 5:** 提交（一仓一 PR，见关键约定）。分支 `docs/zh-cn`。

### Task 2: llm-agent-comm 补 README

**Files:**
- Modify: `llm-agent-comm/README.md`
- Create: `llm-agent-comm/README.zh-CN.md`

- [ ] **Step 1:** 读源码：comm 含 base + a2a + mcp（参照生态记忆），确认各子能力。
- [ ] **Step 2:** 扩充英文 README（定位/导入/用法/导出/依赖）+ 互链行。
- [ ] **Step 3:** 写 `README.zh-CN.md`。
- [ ] **Step 4:** 验证（同 Task 1 Step 4）。
- [ ] **Step 5:** 提交 PR（分支 `docs/zh-cn`）。

### Task 3: llm-agent-policy 补 README

**Files:**
- Modify: `llm-agent-policy/README.md`
- Create: `llm-agent-policy/README.zh-CN.md`

- [ ] **Step 1:** 读源码确认策略/护栏能力。
- [ ] **Step 2:** 扩充英文 README + 互链行。
- [ ] **Step 3:** 写 `README.zh-CN.md`。
- [ ] **Step 4:** 验证。
- [ ] **Step 5:** 提交 PR（分支 `docs/zh-cn`）。

### Task 4: llm-agent-memory-client 新建 README

**Files:**
- Create: `llm-agent-memory-client/README.md`
- Create: `llm-agent-memory-client/README.zh-CN.md`

- [ ] **Step 1:** 读源码：确认它是 memory gateway 的客户端 SDK，列出主要客户端 API。
- [ ] **Step 2:** 新建英文 README（定位/导入/连接配置/最小调用示例/核心 API/与 gateway 的关系）+ 互链行。
- [ ] **Step 3:** 写 `README.zh-CN.md`。
- [ ] **Step 4:** 验证。
- [ ] **Step 5:** 提交 PR（分支 `docs/zh-cn`）。

### Task 5: llm-agent-console 新建根 README

**Files:**
- Create: `llm-agent-console/README.md`
- Create: `llm-agent-console/README.zh-CN.md`

- [ ] **Step 1:** 读 `llm-agent-console/CLAUDE.md`（170L）与源码/`web/README.md`，确认 console 定位（含 web 前端）。注意该仓 Go 命令需 `GOWORK=off`（生态记忆）。
- [ ] **Step 2:** 新建根 `README.md`：项目定位、目录结构（后端 + `web/`）、本地运行（含 `GOWORK=off` 提示）、与生态关系 + 互链行。
- [ ] **Step 3:** 写 `README.zh-CN.md`。
- [ ] **Step 4:** 验证。
- [ ] **Step 5:** 提交 PR（分支 `docs/zh-cn`，本任务只含两个新建 README；`web/README.md`、`CLAUDE.md` 的翻译在 L2 Task 16 处理，同一分支追加亦可）。

---

## Phase L2 — 翻译并行（每仓一任务，引用术语表）

> 每个 L2 任务对该仓所有列出文件产出配对 `.zh-CN.md`，遵守翻译规则与验证标准，最后一仓一 PR（分支 `docs/zh-cn`；若该仓已在 L1 建过 `docs/zh-cn` 分支则在其上追加）。大文件（如 rag CHANGELOG 905L）由 agent 分段翻译，验证时检查结构完整无截断。**建议按波次执行**：每波若干仓，波末主线抽查再开下一波。

### Wave A（小仓，快速验证流程）

### Task 6: llm-agent-contract

**Files:** Create `README.zh-CN.md`（README.md 88L）

- [ ] Step 1: 翻译 README.md → README.zh-CN.md（+互链行，补英文 README 顶部互链行）。
- [ ] Step 2: 验证（结构 1:1 / 无残留英文 / 术语一致）。
- [ ] Step 3: 提交 PR（分支 `docs/zh-cn`）。

### Task 7: llm-agent-memory

**Files:** Create `README.zh-CN.md`(44L)、`CHANGELOG.zh-CN.md`(180L)

- [ ] Step 1: 翻译 README.md（+互链行）。
- [ ] Step 2: 翻译 CHANGELOG.md。
- [ ] Step 3: 验证两篇。
- [ ] Step 4: 提交 PR（分支 `docs/zh-cn`）。

### Task 8: llm-agent-memory-contract

**Files:** Create `README.zh-CN.md`(38L)、`CHANGELOG.zh-CN.md`(45L)

- [ ] Step 1-2: 翻译两篇（README +互链行）。
- [ ] Step 3: 验证。
- [ ] Step 4: 提交 PR。

### Task 9: llm-agent-memory-postgres

**Files:** Create `README.zh-CN.md`(107L)、`CHANGELOG.zh-CN.md`(106L)

- [ ] Step 1-2: 翻译两篇（README +互链行）。
- [ ] Step 3: 验证。
- [ ] Step 4: 提交 PR。

### Task 10: llm-agent-memory-worker

**Files:** Create `README.zh-CN.md`(31L)、`CHANGELOG.zh-CN.md`(49L)

- [ ] Step 1-2: 翻译两篇（README +互链行）。
- [ ] Step 3: 验证。
- [ ] Step 4: 提交 PR。

### Task 11: llm-agent-otel

**Files:** Create `README.zh-CN.md`(258L)、`CHANGELOG.zh-CN.md`(50L)

- [ ] Step 1-2: 翻译两篇（README +互链行）。
- [ ] Step 3: 验证。
- [ ] Step 4: 提交 PR。

### Task 12: llm-agent-customer-support

**Files:** Create `README.zh-CN.md`(143L)、`docs/flowrunner.zh-CN.md`(166L)

- [ ] Step 1-2: 翻译两篇（README +互链行）。
- [ ] Step 3: 验证。
- [ ] Step 4: 提交 PR。

### Task 13: llm-agent-memory-gateway

**Files:** Create `README.zh-CN.md`(287L)、`CHANGELOG.zh-CN.md`(154L)

- [ ] Step 1-2: 翻译两篇（README +互链行）。
- [ ] Step 3: 验证。
- [ ] Step 4: 提交 PR。

### Wave B（多文档大仓）

### Task 14: llm-agent-flow

**Files:** Create 配对中文版——
`README.zh-CN.md`(292L)、`CHANGELOG.zh-CN.md`(535L)、`docs/architecture.zh-CN.md`(258L)、`docs/compatibility.zh-CN.md`(70L)、`docs/operations.zh-CN.md`(253L)、`docs/tutorial.zh-CN.md`(288L)

- [ ] Step 1: 翻译 README（+互链行）+ 4 篇 docs。
- [ ] Step 2: 翻译 CHANGELOG。
- [ ] Step 3: 验证全部 6 篇。
- [ ] Step 4: 提交 PR（分支 `docs/zh-cn`）。

### Task 15: llm-agent-providers

**Files:** Create——
`README.zh-CN.md`(146L)、`CHANGELOG.zh-CN.md`(189L)、`anthropic/README.zh-CN.md`(38L)、`deepseek/README.zh-CN.md`(37L)、`minimax/README.zh-CN.md`(37L)、`ollama/README.zh-CN.md`(30L)、`openai/README.zh-CN.md`(32L)、`docs/superpowers/plans/2026-05-13-deepseek-minimax-implementation.zh-CN.md`(451L)、`docs/superpowers/specs/2026-05-13-deepseek-minimax-design.zh-CN.md`(308L)

- [ ] Step 1: 翻译根 README（+互链行）+ 5 个子包 README（各 +互链行）。
- [ ] Step 2: 翻译 CHANGELOG + 2 篇 superpowers。
- [ ] Step 3: 验证全部 9 篇。
- [ ] Step 4: 提交 PR。

### Task 16: llm-agent-console（翻译剩余）

**Files:** Create `web/README.zh-CN.md`(73L)、`CLAUDE.zh-CN.md`(170L)

- [ ] Step 1: 翻译 web/README.md + CLAUDE.md。
- [ ] Step 2: 验证。
- [ ] Step 3: 提交（追加到 Task 5 的 `docs/zh-cn` 分支/PR，或独立 PR）。

### Task 17: llm-agent-rag

**Files:** Create——
`README.zh-CN.md`(288L)、`CHANGELOG.zh-CN.md`(905L)、`docs/api-audit-v1.0.zh-CN.md`(569L)、`docs/backend-selection.zh-CN.md`(130L)、`docs/compatibility.zh-CN.md`(187L)、`docs/core-compatibility.zh-CN.md`(124L)、`docs/graphrag.zh-CN.md`(572L)、`docs/production-deployment.zh-CN.md`(231L)、`docs/v2-rfc.zh-CN.md`(487L)、`docs/superpowers/plans/2026-05-23-self-rag-reflection.zh-CN.md`(872L)、`docs/superpowers/specs/2026-05-23-self-rag-reflection-design.zh-CN.md`(295L)

- [ ] Step 1: 翻译 README（+互链行）+ 7 篇 docs/ 用户文档。
- [ ] Step 2: 翻译 CHANGELOG（905L，分段、防截断）+ 2 篇 superpowers。
- [ ] Step 3: 验证全部 11 篇（重点查 CHANGELOG 结构完整）。
- [ ] Step 4: 提交 PR。

### Task 18: llm-agent（最大仓，22 篇）

**Files:** Create 以下全部配对中文版——
README(283L,+互链行)、CHANGELOG(535L)、CLAUDE(71L)、DEPRECATIONS(59L)、PROVIDER_AUTHORING(275L)、
`docs/`：2026-05-13-rag-sdk-migration-status(139L)、2026-05-13-standalone-rag-sdk-design(460L)、2026-05-13-standalone-rag-sdk-implementation-plan(493L)、2026-05-14-rag-production-enhancement-plan(852L)、2026-05-14-rag-release-verification(180L)、migration-v0.2-to-v0.3(106L)、PR-GOVERNANCE-OPERATIONS(348L)、PR-GOVERNANCE-OVERVIEW(65L)、PR-GOVERNANCE-PROJECTS(138L)、PR-GOVERNANCE-RULES(212L)、SUPERVISOR(83L)、
`examples/`：README(74L)、06-budget/README(78L)、07-policy/README(80L)、08-supervisor/README(38L)、09-ollama/README(76L)、10-ollama-tools/README(72L)

- [ ] Step 1: 翻译 README（+互链行）+ 顶层文档（CLAUDE/DEPRECATIONS/PROVIDER_AUTHORING）。
- [ ] Step 2: 翻译 `docs/` 全部 11 篇。
- [ ] Step 3: 翻译 `examples/` 6 篇 README（各 +互链行）+ CHANGELOG。
- [ ] Step 4: 验证全部 22 篇。
- [ ] Step 5: 提交 PR（分支 `docs/zh-cn`）。

> 注：llm-agent 文件多，执行时可在该仓内分多次 agent 调用（按 Step 分组），最终汇总到同一 `docs/zh-cn` 分支再开 PR。

---

## Phase L3 — 收尾

### Task 19: 文档索引补中文链接 + 整体核验

**Files:** Modify 各仓 README 的 "Docs/文档" 章节（若有指向 docs/ 的列表）

- [ ] Step 1: 检查含 docs 索引的仓（rag/flow/llm-agent/providers/customer-support），在其 README.zh-CN.md 的文档列表补对应 `.zh-CN.md` 链接。
- [ ] Step 2: 全生态核验：`for d in <16仓>; do find $d -name '*.zh-CN.md' | wc -l; done`，与计划清单数量比对，列出任何缺漏。
- [ ] Step 3: 抽查 5 篇随机中文文档渲染（标题/表格/代码块/链接）。
- [ ] Step 4: 若 Step1 有改动，并入对应仓的 `docs/zh-cn` PR。

---

## Self-Review（计划自查）

- **Spec 覆盖**：L0 术语表↔spec§4；L1 五仓补内容↔spec§2.2；L2 全文件翻译↔spec§2.1（68 文件经 Task6–18 逐仓覆盖）；翻译约定↔spec§3 已提炼为"关键约定"；提交方式↔spec§6；验证↔spec§7。无遗漏。
- **占位符**：无 TBD/TODO；每仓文件清单为精确枚举结果（含行数）。
- **一致性**：术语表文件名在 L0 产出、在"关键约定"被引用，路径一致（`docs/superpowers/specs/2026-06-04-chinese-docs-glossary.md`）；分支名统一 `docs/zh-cn`（umbrella 术语表用 `docs/zh-cn-glossary` 区分）。
- **数量核对**：L1 涉及 builtin/comm/policy/memory-client/console 5 仓；L2 Task6–18 覆盖其余翻译；console 的非 README 文件在 Task16 兜底。总计 68 现存文件 + 5 处新建/补写。
