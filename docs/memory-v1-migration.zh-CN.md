# `llm-agent-memory` v0.x → v1.0.0 迁移指南

> 文档版本：2026-05-26
> 适用范围：`github.com/costa92/llm-agent-memory v0.3.0` → `v1.0.0`
> 关联设计：`docs/superpowers/plans/2026-05-26-m4-phase-d-capability-interfaces.md`、
> `docs/memory-roadmap.zh-CN.md` §5.1（D-1 / D-2）。

## 1. 概览

`llm-agent-memory v1.0.0` 收敛了 Phase D 的两个 breaking change：

1. **D-1：`ManagerOptions` 从 concrete-type 转为 capability-interface。**
   新的 `memory.Options` / `memory.TierOptions` 接受
   `coremem.Memory` 接口值，所以 `coremem.WithSanitizer(...)` 包装出来的
   `Memory` 接口可以直接挂进 Manager —— 不再需要 cast。
2. **D-2：新增 `RecallEngine` 作为统一召回门面。** `Recall(ctx, query, opts)`
   是 v1 推荐的召回入口，tier 选择（working / episodic / semantic）成为内部细节。

核心仓 `github.com/costa92/llm-agent` 没有任何修改 —— v0.7.0 保持不变。
所有 break 都发生在 sibling 模块内部。

## 2. 破坏性变更清单

| # | 区域 | 变更 | 旧用法 | 新用法 |
|---|---|---|---|---|
| 1 | 构造 Manager | 推荐入口从 `coremem.NewManager(coremem.ManagerOptions{...})` 转为 `memory.NewManager(memory.Options{...})`。 | `coremem.NewManager(coremem.ManagerOptions{Working: w})` | `memory.NewManager(memory.Options{Working: memory.TierOptions{Memory: w, Lister: w, Exporter: w, Importer: w}})` |
| 2 | 召回入口 | 推荐入口从 `UnifiedSearcher.SearchUnified` / `ParallelSearcher.SearchAllParallel` 转为 `RecallEngine.Recall`。`UnifiedSearcher` 与 `ParallelSearcher` 仍在 v1.x 中可用但已 `// Deprecated:`。 | `uni.SearchUnified(ctx, q, topK)` | `eng.Recall(ctx, q, memory.RecallOptions{TopK: topK})` |

说明：除上述两项，v1.0.0 对外暴露的 M1/M2/M3 surface（`ScopedLifecycleManager`、
`Consolidator`、`PolicyEnforcingMemory`、`SQLiteStore` 等）API 完全不变。
你可以仅升级 sibling 依赖、什么也不改，旧测试照样能跑。

## 3. 升级配方

### 3.1 最小改动：让旧的 `*coremem.Manager` 走 v1 Manager

```go
// 旧代码：
coreMgr, _ := coremem.NewManager(coremem.ManagerOptions{
    Working:  w,
    Episodic: e,
    Semantic: s,
})

// 升级后（一行）：
import compat "github.com/costa92/llm-agent-memory/memory/compat"
mgr := compat.NewManagerFromCore(coreMgr)

// 下游代码（mgr.Add / mgr.Search / mgr.Consolidate ...）无需修改。
```

`compat.NewManagerFromCore` 在 v1.x 全周期内可用，将在 v2.0.0 移除。

### 3.2 字段级改动：直接构造 v1 Manager

```go
import (
    "github.com/costa92/llm-agent-memory/memory"
    coremem "github.com/costa92/llm-agent/memory"
)

w, _ := coremem.NewWorking(emb, coremem.WorkingOptions{})
e, _ := coremem.NewEpisodic(emb, coremem.EpisodicOptions{})
s, _ := coremem.NewSemantic(emb, coremem.SemanticOptions{})

mgr, _ := memory.NewManager(memory.Options{
    Working:  memory.TierOptions{Memory: w, Lister: w, Exporter: w, Importer: w},
    Episodic: memory.TierOptions{Memory: e, Lister: e, Exporter: e, Importer: e},
    Semantic: memory.TierOptions{Memory: s, Lister: s, Exporter: s, Importer: s},
    // 可选：若需要 Consolidate / Forget 调用，提供 CoreManager 兜底：
    CoreManager: nil,
})
```

注意：bundled `coremem.WorkingMemory` 等同时满足 `Memory`、`Lister`、
`Exporter`、`Importer` 四个接口，所以同一个对象可以填四个字段。

### 3.3 带 sanitizer 的写入路径（D-1 的主要价值）

```go
// v0.7 - 不能直接挂入 ManagerOptions，被 docs/policy_hook.go 的 LIMITATION
// 标注阻断了：
// wrapped := coremem.WithSanitizer(w, redactor)  // coremem.Memory 接口
// opts.Working = wrapped                         // ❌ 类型不匹配

// v1.0.0 - 直接挂：
wrapped := coremem.WithSanitizer(w, redactor) // coremem.Memory 接口
opts.Working = memory.TierOptions{Memory: wrapped}
```

### 3.4 召回入口切换

```go
// v0.x：
uni, _ := memory.NewUnifiedSearcher(coreMgr)
results, _ := uni.SearchUnified(ctx, "query", 10)

// v1.0.0：
eng, _ := memory.NewRecallEngine(mgr)
recall, _ := eng.Recall(ctx, "query", memory.RecallOptions{TopK: 10})
results := recall.Results
```

`RecallOptions.Tiers`、`RecallOptions.Budgets`、`RecallOptions.IncludeProvenance`
是 v1 新增的能力。详见 `recall_engine.go` 的 godoc。

## 4. 兼容窗口

| 组件 | v1.x 状态 | 计划移除时间 |
|---|---|---|
| `memory/compat` 子包 | 可用，全部入口带 `// Deprecated:` | v2.0.0 |
| `UnifiedSearcher` / `ParallelSearcher` | 可用，类型 doc 带 `// Deprecated:` | v2.0.0 |
| `coremem.ManagerOptions` | 核心仓不归本路线图管，无变化 | n/a |

在 v2.0.0 之前，所有 v0.x 调用方都可以选择 *不迁移*，sibling 升级到 v1.x
不会破坏现有调用。

## 5. 与 `context.Builder` 的衔接

`coremem.context.Builder.BuildInput.MemoryHits` 字段仍是 `[]coremem.SearchResult`
（核心仓 v0.7 没动）。从 v1 `Recall` 喂数据给 Builder 的一行：

```go
out, _ := eng.Recall(ctx, query, memory.RecallOptions{TopK: 10})
build := builder.Build(context.BuildInput{
    UserQuery:  query,
    MemoryHits: out.Results, // 直接对接
    // ...
})
```

M6（Memory Gateway）以及核心仓 Builder 演化是后续 milestone 的话题；
本指南仅说明 v1.0.0 的接入面。

## 6. 退出标准对照

下表对应 `docs/superpowers/plans/2026-05-25-llm-agent-memory-roadmap.md`
M4 行的 exit criteria：

| # | 标准 | 实现位置 | 测试位置 |
|---|---|---|---|
| 1 | `ManagerOptions` 字段为接口（Memory/Lister/Exporter/Importer/可选 Lifecycle） | `memory/manager.go` — `TierOptions` struct | `memory/manager_test.go` — `TestManager_TierOptions_FieldsAreCapabilityInterfaces` |
| 2 | `WithSanitizer` 包装的 memory 不需 cast 即可装入 Manager | `memory/manager.go` — `TierOptions.Memory coremem.Memory` | `memory/manager_test.go` — `TestManager_WithSanitizerWrappedMemory_InstallsWithoutCast` |
| 3 | `RecallEngine.Recall(ctx, query, opts)` 暴露统一召回门面 | `memory/recall_engine.go` — `RecallEngine.Recall` | `memory/recall_engine_test.go` — `TestRecallEngine_Recall_ParityWithUnifiedSearcher`、`TestRecallEngine_Recall_TierMask_Working_OmitsOtherTiers`、`TestRecallEngine_Recall_PerTierBudget_CapsCandidates`、`TestRecallEngine_OverWithSanitizerWrappedManager_NoCast` |
| 4 | 迁移指南文档列出每一项 break | `docs/memory-v1-migration.zh-CN.md`（本文） | — |
| 5 | `memory/compat/` 子包提供旧 ManagerOptions 构造器 | `memory/compat/compat.go` — `NewManagerFromCore`、`NewManagerFromLegacyOptions`、`LegacyOptions` | `memory/compat/compat_test.go` 三个测试 |
| 6 | M1–M3 测试全部适配，无无理由删除 | 无现有测试删除；新 D-1/D-2 测试落在新文件 | `memory/manager_test.go` + `memory/recall_engine_test.go` + `memory/compat/compat_test.go` |

## 7. 相关文档

- 主路线图：`docs/superpowers/plans/2026-05-25-llm-agent-memory-roadmap.md`
- 详细计划：`docs/superpowers/plans/2026-05-26-m4-phase-d-capability-interfaces.md`
- 设计依据：`docs/memory-roadmap.zh-CN.md` §5.1（Phase D）
- 上一里程碑（M3）的同结构计划：`docs/superpowers/plans/2026-05-26-m3-phase-c-policy-and-stores.md`
