# Phase 5 交接：slim `llm-agent-memory` 为 contract alias shim

> 状态：**待你解除阻塞后应用。** Proposal 2 的 Phase 1–4 + 6 已完成并验证(见
> `feat/memory-contract-extraction` 分支)。Phase 5 不能由助手代做,原因如下。

## 为什么 Phase 5 必须由你来落地

`llm-agent-memory` 是一个**独立的 git 仓**(自带 `.git`,被 umbrella `.gitignore` 忽略),
当前有约 48 个**未提交的 WIP 改动**(`compat/` 已删、`manager.go` / `recall_engine.go` /
`sqlite_store.go` 等大量 engine 文件在改),且其 `GOWORK=off go vet` 当前是**红的**
(`observer_test.go` 报 `MemoryItem` 类型不匹配 —— 你的重构进行到一半)。

关键事实:**`memory/durable.go` 本身是你未跟踪的 WIP**(`git cat-file -e HEAD:memory/durable.go`
返回不存在 —— 它不在 HEAD,是你工作区里的新文件)。Phase 5 的做法是"删 durable.go、加 alias shim",
而删一个**未跟踪、无 git 备份**的文件 = 不可逆地销毁你的在改工作。因此助手停手,留给你。

## 解除阻塞的前置

在 `llm-agent-memory` 仓里,先把你的 WIP 落地到一个干净、可编译的基线:
```
cd llm-agent-memory
git add -A && git commit -m "wip: <你的重构描述>"   # 或 git stash,二选一
GOWORK=off go build ./... && GOWORK=off go vet ./... && GOWORK=off go test ./...   # 必须全绿
```
只有当 `durable.go` 是一个**已提交、可恢复**的文件、且模块自身测试绿了,Phase 5 才能安全做。

## Phase 5 应用步骤(前置满足后,一次完成)

### 1. 新建 `llm-agent-memory/memory/durable_shim.go`

内容如下(已按 contract 的真实导出符号生成,逐一对应;`NormalizeWriteDefaults` /
`SetWorkingDefault` 是 `MemoryRecord` 的方法,通过 `=` 别名自动提升,无需再导出):

```go
package memory

import contract "github.com/costa92/llm-agent-memory-contract/contract"

// Durable contract alias shim. The durable model was extracted verbatim into
// github.com/costa92/llm-agent-memory-contract/contract. These are true `=`
// type aliases so existing importers of llm-agent-memory/memory keep compiling
// for one release cycle.
//
// CAVEAT: aliases only unify types when the whole module graph resolves to a
// SINGLE llm-agent-memory-contract version. A mixed-version graph still breaks
// var-_ interface assertions and named-type slices/maps. Pin one contract
// version across all modules during the migration wave. This shim is a
// transition aid, not a mixed-version fix; plan its removal in a future major.

// Types.
type (
	MemoryRecord        = contract.MemoryRecord
	StoredEvent         = contract.StoredEvent
	OutboxMessage       = contract.OutboxMessage
	IdempotencyEntry    = contract.IdempotencyEntry
	WriteRecordInput    = contract.WriteRecordInput
	WriteRecordResult   = contract.WriteRecordResult
	PatchRecordInput    = contract.PatchRecordInput
	PatchRecordResult   = contract.PatchRecordResult
	DeleteRecordInput   = contract.DeleteRecordInput
	DeleteRecordResult  = contract.DeleteRecordResult
	PinRecordInput      = contract.PinRecordInput
	PinRecordResult     = contract.PinRecordResult
	DisableRecordInput  = contract.DisableRecordInput
	DisableRecordResult = contract.DisableRecordResult
	PromoteRecordInput  = contract.PromoteRecordInput
	PromoteRecordResult = contract.PromoteRecordResult
	DedupeAction        = contract.DedupeAction
	ResolveDedupeInput  = contract.ResolveDedupeInput
	ResolveDedupeResult = contract.ResolveDedupeResult
	MarkAccessInput     = contract.MarkAccessInput
	RecordStore         = contract.RecordStore
	Promoter            = contract.Promoter
	Deduper             = contract.Deduper
	AccessMarker        = contract.AccessMarker
	EventStore          = contract.EventStore
	IdempotencyStore    = contract.IdempotencyStore
	Outbox              = contract.Outbox
	MessagePublisher    = contract.MessagePublisher
)

// Constants.
const (
	RecordKindWorking                 = contract.RecordKindWorking
	RecordKindEpisodic                = contract.RecordKindEpisodic
	RecordKindSemantic                = contract.RecordKindSemantic
	DedupeCollapsedLoserIDMetadataKey = contract.DedupeCollapsedLoserIDMetadataKey
	DedupeNoCollision                 = contract.DedupeNoCollision
	DedupeMergedExisting              = contract.DedupeMergedExisting
	DedupeCollapsedByPin              = contract.DedupeCollapsedByPin
)

// Errors.
var ErrInvalidRecordKind = contract.ErrInvalidRecordKind

// Functions.
var NormalizeRecordKind = contract.NormalizeRecordKind
```

### 2. 删除 `llm-agent-memory/memory/durable.go`(及若有的 `durable_test.go`)
durable 的单元测试与 wire 守卫已随 Phase 1 搬入
`llm-agent-memory-contract/contract/`(`durable_test.go` + `golden_wire_test.go`),
此处删除不丢测试覆盖。

### 3. `llm-agent-memory/go.mod` 加 contract 依赖(保持 `v0.0.0` 占位 + 本地 replace 约定)
require 块加:
```
	github.com/costa92/llm-agent-memory-contract v0.0.0
```
文件末尾加:
```
replace github.com/costa92/llm-agent-memory-contract => ../llm-agent-memory-contract
```
不要 `go mod tidy` 把 `v0.0.0` 解析成远程版本(本仓沿用 umbrella 的占位+replace 约定)。

### 4. 验证(全绿才算完成)
```
cd llm-agent-memory
GOWORK=off go build ./...
GOWORK=off go vet ./...
GOWORK=off go test ./...
```
shim 是真 `=` 别名,引擎层/`core_adapters.go`/`types_alias.go` 里按本地名
(`MemoryRecord` 等)引用的代码会被 shim 自动满足,应无需改动。

### 5. 提交
```
git add memory/durable_shim.go go.mod
git rm memory/durable.go memory/durable_test.go   # 若 durable_test.go 存在
git commit -m "refactor(memory): replace durable.go with contract alias shim (one-cycle transition)"
```

## 验证助手已确认的事实(2026-05-31)
- 助手曾用此 shim 在你的工作区试做,`GOWORK=off go build ./...` **rc=0**(编译通过),
  随后**全部回退**,因发现 `durable.go` 是你的未跟踪 WIP,删除不可逆。你的 WIP 原样保留。
- 整条 contract 波次(contract / postgres / worker / gateway)在 `GOWORK=off` 下
  build+vet 全绿;依赖图:postgres+worker 已甩掉 `llm-agent`,gateway 经 rag 保留,
  三者均只依赖 `llm-agent-memory-contract`、旧 `llm-agent-memory` 依赖为 0。

## shim 的兼容性注意(来自两轮评审)
`type X = contract.X` 仅在**整个模块图解析到同一个 contract 版本**时才让类型统一。
混合版本图(一处 `@vX` 一处 `@vY`)仍会在 `var _` 接口断言、命名类型的 slice/map 处炸。
迁移波次里务必让所有模块 pin 同一 contract 版本。shim 是一个发布周期的过渡件,
后续应在某个 major 版本里移除(届时 `llm-agent-memory` 的消费方直接改 import 到 contract)。
