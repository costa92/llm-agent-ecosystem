# 全生态子仓库文档中文化设计

**日期：** 2026-06-04
**状态：** 已认可（待用户复审 spec）
**作者：** Brainstorming 协作产出
**范围：** `llm-agent-ecosystem` 下 16 个子仓库的用户文档中文化

---

## 1. 背景与目标

umbrella 根仓的 `docs/` 已有大量中文文档，约定为 `名.md`（英文）+ `名.zh-CN.md`（中文）配对。但**所有子仓库目前零 `.zh-CN.md`**，文档基本纯英文，且部分仓库文档严重缺失（桩 README 或无 README）。

**目标：** 为全部子仓库的用户文档补齐配对中文版，并先补全实质缺失的文档内容。完成后每个子仓库的关键文档都有中英对照。

**非目标：** 不改写英文原文；不翻译第三方 `node_modules`；不翻译 `.planning/` 一次性产物；不做与中文化无关的文档重构。

---

## 2. 范围清单（已锁定）

### 2.1 纳入翻译的文件类别

| 类别 | 数量 | 说明 |
|---|---|---|
| 用户文档（README + docs/ + 子包 README） | ~52 | 主入口与架构/教程/运维/兼容性等 |
| `CHANGELOG.md` | 11 | 全量翻译（含 rag 905 行等大文件） |
| `CLAUDE.md` | 2 | llm-agent、console |
| `docs/superpowers/` 规划文档 | 4 | rag ×2、providers ×2 |
| **合计** | **~69** | 均新建对应 `.zh-CN.md` |

> 精确文件清单在实现计划阶段由每仓 agent 逐仓 `find` 重新枚举确认，以防本设计编写后文件有增减。枚举命令统一排除 `node_modules`、`.planning`、`vendor`、`.git`、已存在的 `*.zh-CN.md`。

### 2.2 需先补英文内容再翻译（L1）

| 仓库 | 现状 | 补内容动作 |
|---|---|---|
| `llm-agent-console` | 无 README（仅 CLAUDE.md） | 读源码写实质 README.md |
| `llm-agent-memory-client` | 无任何 .md | 读源码写实质 README.md |
| `llm-agent-builtin` | 14 行桩 README | 扩充为实质 README |
| `llm-agent-comm` | 14 行桩 README | 扩充为实质 README |
| `llm-agent-policy` | 14 行桩 README | 扩充为实质 README |

补写的英文 README 至少覆盖：包定位（一句话职责）、安装/导入、最小用法示例、核心导出 API/接口、与生态其他仓的依赖关系。内容必须对照该仓 Go 源码核实，不臆造。

### 2.3 涉及仓库（16）

`llm-agent`、`llm-agent-contract`、`llm-agent-memory`、`llm-agent-memory-client`、`llm-agent-memory-contract`、`llm-agent-memory-gateway`、`llm-agent-memory-postgres`、`llm-agent-memory-worker`、`llm-agent-rag`、`llm-agent-flow`、`llm-agent-builtin`、`llm-agent-comm`、`llm-agent-policy`、`llm-agent-otel`、`llm-agent-providers`、`llm-agent-customer-support`

---

## 3. 翻译约定

跟随 umbrella 现有约定，并补充以下硬性规则，所有翻译 agent 必须遵守：

1. **配对而非替换**：英文 `X.md` 原文不动；新增 `X.zh-CN.md` 为完整中文版。
2. **结构 1:1**：中文版与英文版章节层级、标题顺序、表格、列表逐一对应。
3. **不翻译的内容**：代码块内容、命令、API 名、类型名/函数名/包名等标识符、URL、文件路径、版本号、git 分支/tag 名一律保持原样。
4. **保留元信息头**：英文原文若有 `> Document version / Code snapshot` 之类的头部，中文版照样保留（值不变）。
5. **README 互链**：每个 `README.md` 与 `README.zh-CN.md` 顶部加一行语言切换：
   - 英文版：`[English](./README.md) | [简体中文](./README.zh-CN.md)`
   - 中文版：同样一行，确保双向可达。
   - （仅 README 加互链行；docs/ 内文档保持 umbrella 现有的"平行文件、不加切换行"风格，避免大改原文。）
6. **术语一致**：统一引用第 4 节术语表，禁止同一术语在不同文件出现不同译法。

---

## 4. 术语表（L0，先行产出）

执行第一步先产出共享术语表，落地为 `docs/superpowers/specs/2026-06-04-chinese-docs-glossary.md`，所有后续翻译 agent 强制引用。术语表由抽样各仓核心文档（README + 架构类 docs）后生成，至少覆盖以下高频术语，给出固定中文译法或"保留不译"标注：

- 结构性：agent、tool、provider、contract、gateway、worker、engine、adapter、seam（接缝）、umbrella（伞形）
- RAG/记忆域：retriever、reranker、embedding、chunk、graph/GraphRAG、reflection、working memory、episodic memory、outbox、dedupe
- 工程域：guardrail（护栏）、policy（策略）、supervisor、observability（可观测性）、telemetry、span/trace、replace directive、conformance harness（一致性测试套件）
- 保留不译（标识符）：所有 `package`/类型/函数名、`go.work`、`GOWORK=off`、`pgvector` 等

术语表格式：`| 英文 | 中文 | 备注（是否保留不译/上下文） |`

---

## 5. 执行分层（方案 B）

### L0 — 术语表
产出 `2026-06-04-chinese-docs-glossary.md`。主线审查通过后才进入 L1/L2。

### L1 — 补内容（顺序、谨慎）
对 2.2 的 5 个仓库：每仓一个 agent，读源码 → 写实质英文 README → 再出 `README.zh-CN.md`。**逐仓审查**英文内容是否与源码相符，再放行。此层是唯一有"创作"风险的部分，不并行、不跳审。

### L2 — 翻译并行（按波次）
对其余翻译型文件：**每仓一个 agent**，引用术语表，按仓内优先级翻译——
1. README + 用户文档（docs/、子包 README）
2. CHANGELOG / CLAUDE / superpowers

多仓可并行成波（一波若干仓），但**每波结束主线抽查**若干文件（结构 1:1、无残留英文段、代码块/链接完好）再开下一波。

### L3 — 收尾
- 若某仓有 docs 索引（如 README 的 Docs 章节），补中文文档链接。
- 各仓改动按"一仓一分支一 PR"提交（见第 6 节）。

---

## 6. 提交与集成

- **一仓一分支一 PR**：分支名 `docs/zh-cn`，仅含该仓新增的 `.zh-CN.md`（及 L1 仓的新英文 README）。
- docs-only 附加文件不触 `go.mod`，CI 的 go build/test/governance 检查可过；replace-guard pre-commit hook 对无 go.mod 改动无副作用。
- 遵循生态现有保护/自动合并流程：受保护 main 的仓走 PR → owner PR 自动合并 → 分支自动删除（参照 rag PR#28 的路径）。
- 每个 PR 独立，互不阻塞；某仓失败不影响其余仓。

---

## 7. 验证标准

每篇 `.zh-CN.md` 必须满足：
1. 章节结构与英文版 1:1 对应。
2. 无成段未翻译的英文（代码/标识符/URL 例外）。
3. 代码块、表格、链接、图片引用与原文一致且可渲染。
4. 术语译法与术语表一致。
5. README 互链行双向可达。

L1 仓额外验证：新英文 README 的 API/用法描述与源码核对无臆造。

---

## 8. 风险与对策

| 风险 | 对策 |
|---|---|
| 多 agent 术语漂移 | L0 术语表先行 + 强制引用 |
| 大文件（rag CHANGELOG 905 行）翻译质量/截断 | 单文件单 agent，必要时分段；验证时检查结构完整 |
| 桩仓补内容臆造 | L1 强制对照源码、逐仓审查、不并行 |
| 文件清单在编写后变动 | 实现阶段每仓重新 `find` 枚举核对 |
| 16 个 PR 管理 | 一仓一 PR、自动合并、失败隔离 |
