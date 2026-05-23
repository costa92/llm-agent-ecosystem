# Umbrella Root 源码级设计说明

> 仓库路径：`/home/hellotalk/code/go/src/github.com/costa92/llm-agent-ecosystem/`
> 文档版本：2026-05-21
> 阅读对象：想搞清楚"为什么这里有 6 个子目录但根本身没有 `go.mod` 主模块"的开发者；进入跨仓 change 之前希望先理解 umbrella 协调机制的 reviewer / 运维。
> 文档约定：所有断言带 `file:line` 锚点。

---

## 目录

1. [Umbrella 的定位与非目标](#1-umbrella-的定位与非目标)
2. [目录布局解释](#2-目录布局解释)
3. [Makefile 设计](#3-makefile-设计)
4. [`cmd/depcheck` 工具](#4-cmddepcheck-工具)
5. [`go.work` 与 `GOWORK=off` 规则](#5-gowork-与-gowork-off-规则)
6. [B2 / B3 / B4 CI 闸子](#6-b2--b3--b4-ci-闸子)
7. [依赖方向与硬约束](#7-依赖方向与硬约束)
8. [协调式发布流程（Phase 33 模式）](#8-协调式发布流程phase-33-模式)
9. [`.planning/` 关系与 STATE / ROADMAP / REQUIREMENTS](#9-planning-关系与-state--roadmap--requirements)
10. [典型本地工作流](#10-典型本地工作流)
11. [遗留问题与改进点](#11-遗留问题与改进点)

---

## 1. Umbrella 的定位与非目标

### 1.1 定位（一句话）

> Umbrella 根仓**是协调点，不是 monorepo**。它只持有：(a) 跨仓导航 + 文档；(b) 跨仓 CI 闸子；(c) 跨仓本地开发脚本；(d) 一个 stdlib-only Go 工具 `cmd/depcheck` 用来回答"该 bump 谁了"。它**不持有产品代码**，每个 sibling 仓库各有自己的 `go.mod` / SemVer / git tag / GitHub repo。

证据：根目录只有 5 个文件 + 4 个目录（`README.md` / `PROJECT.md` / `Makefile` / `go.work` / `go.work.sum` + `cmd/` / `docs/` / `scripts/` / `.github/`）；6 个 sibling 各自是独立的 git clone（`scripts/eco.sh:6-13`）。

### 1.2 非目标（明示不做）

按 `README.md:67-89` 与 `PROJECT.md:13-16` 的清单：

| 不做 | 原因 |
|---|---|
| **不是 monorepo** | 每个 sibling 仓库各自 release，version、CI、issue tracker 独立；umbrella 抹掉这些只会增加耦合。 |
| **不是统一构建系统** | 没有 Bazel / Nx / pants 这种"top-level build graph"。`make build` 只是 `cd <repo> && go build ./...` 的薄 fan-out。 |
| **不持有 K8s / Helm / 部署清单** | 整个生态的常驻非目标（`README.md:80-82`）。部署样板在 `llm-agent-customer-support/compose/`，是 docker-compose，不是 K8s。 |
| **不持有产品代码** | `cmd/depcheck` 是**唯一**位于 umbrella 的 Go 程序，且仅服务于 CI 与运维。 |
| **不持有 sibling 仓库源码副本** | `make bootstrap` 实时 git clone 6 个独立 repo（`scripts/eco.sh:18-30, 59-76`）。 |
| **不解决跨仓 SemVer 冲突** | sibling 仓库各自选 tag；冲突由 PR review + `depcheck` 报 stale + 手动 re-tag wave 解决（§8）。 |

### 1.3 为什么不是 monorepo（设计哲学）

四条理由：

1. **核心 `llm-agent` 的 stdlib-only 价值**依赖"独立可读、独立审查"。把它放进 monorepo 后，下游 transitive deps 会自动出现在它的 `go.sum` 里（`go list -deps ./...` 会把整图拉满），破坏 *"读得完每一行"* 这个最大卖点（`source-design-llm-agent.zh-CN.md` §2 DP-1）。
2. **`llm-agent-rag` 是 fixed point**，v1.x API additive-only（`source-design-llm-agent-rag.zh-CN.md` §1.3）。它需要**独立**的 tag 流和 issue tracker，让兄弟仓库"向它对齐"而不是"和它一起 bump"。Monorepo 化的代价：每次 rag 的 patch 都得拖动整个 monorepo 的 CI。
3. **不同 sibling 的演进节奏差异极大**：`llm-agent-flow` 在 ~3 周内从 v0.0.1 → v0.1.1 走了 11 个 phase；同期 `llm-agent` 已到 v0.6.1；`llm-agent-rag` v1.x 锁 API 后仍在打 additive 性能补丁（v1.0.5，2026-05-23 v1.3 perf-wave）。强行同步 release wheel 会让快的等慢的。
4. **GitHub PR review 模型适合"小仓库精读"**而非"大 monorepo 改 5 个目录的 PR"。每个 sibling repo 是独立 review 单位，CODEOWNERS、issue labels、release notes 都 colocated。

非目标列表是**non-negotiable**：umbrella `README.md:67-89` 把这些规则上升为 keystone-level，由 CI 闸子强制（§6）。

---

## 2. 目录布局解释

### 2.1 顶层快照

```
llm-agent-ecosystem/
├── README.md                        # 导航文档（非产品文档），列依赖方向 + 规则
├── PROJECT.md                       # scope 声明（极简，~17 行）
├── Makefile                         # 8 个 target 的薄入口
├── go.work                          # 本地 workspace（gitignored 在每个 sibling，本仓 commit）
├── go.work.sum                      # 同上
├── cmd/
│   └── depcheck/                    # 唯一 Go 程序：stdlib-only 跨仓依赖巡检
│       └── main.go
├── docs/                            # 生态级文档（中英、源码级深读、roadmap、本文档）
├── scripts/
│   ├── eco.sh                       # 多仓 bootstrap/pull/status/build/test/up/down
│   ├── workspace.sh                 # 写 go.work 把 6 个 sibling 串成 workspace
│   └── stdlib-only-check.sh         # B4 闸子实现（核心 stdlib-only 三项断言）
├── .github/
│   └── workflows/
│       └── umbrella.yml             # 4 个 job：cross-repo-build / smoke / stdlib-only / depcheck artifact
├── .planning/                       # umbrella 自己的 phase/roadmap（若有）
├── llm-agent/                       # ↓ 以下是 6 个 sibling 仓库（git clone 实时存在）
├── llm-agent-rag/
├── llm-agent-otel/
├── llm-agent-providers/
├── llm-agent-flow/
└── llm-agent-customer-support/
```

`flow/` 目录在历史快照里曾经出现过，是 git status 在 phase 转移期遗留的旧名（`PROJECT.md` 只列了 5 个 sibling，未含 flow）；当前 canonical roster 以 `cmd/depcheck/main.go:32-39` 与 `README.md:31-39` 为准（6 个，含 flow）。

### 2.2 每个一级目录的角色

| 一级目录 | 类型 | 角色 | 是否被 CI/工具消费 |
|---|---|---|---|
| `cmd/depcheck/` | Go 程序源码 | 跨仓依赖巡检工具（§4） | 是 — umbrella CI B3 job 调用 |
| `docs/` | Markdown | 生态级文档（导航 / 源码级深读 / 架构图 / roadmap / 评审） | 否 — 人类阅读 |
| `scripts/` | bash | 多仓操作 + workspace + stdlib 检查的脚本实现 | 是 — Makefile 转发 + B4 调用 |
| `.github/workflows/` | YAML | umbrella 自己的 CI 流水（B2 / B3 / B4 + cross-repo-build） | 是 — GitHub Actions runner |
| `.planning/` | Markdown | umbrella 级别的 phase/roadmap/STATE（若存在） | 否 — 人类阅读 + AI 工作流引用 |
| `llm-agent/` 等 6 sibling | git clone | 子仓代码（不属于 umbrella；通过 `make bootstrap` 拉取） | 是 — CI 在 umbrella job 内 checkout 各 sibling |

注意：6 个 sibling 目录**不应该**被 commit 到 umbrella 仓 — 它们由 `make bootstrap` 实时 clone（`scripts/eco.sh:59-76` `bootstrap_repo` 函数）。

---

## 3. Makefile 设计

### 3.1 完整 target 列表

`Makefile` 共 8 个 phony target（`Makefile:5-40`）：

| Target | 命令 | 作用 |
|---|---|---|
| `help` | 输出 8 行说明 | 默认 target；纯文本帮助。 |
| `bootstrap` | `./scripts/eco.sh bootstrap $(TARGETS)` | 从 GitHub clone 缺失的 sibling 仓库。 |
| `workspace` | `./scripts/workspace.sh` | 写 `<root>/go.work` 把 6 sibling 接入本地 workspace。 |
| `pull` | `./scripts/eco.sh pull $(TARGETS)` | 对每个 sibling 跑 `git pull --ff-only`；缺失则 clone。 |
| `status` | `./scripts/eco.sh status $(TARGETS)` | 对每个 sibling 跑 `git status -sb`。 |
| `build` | `./scripts/eco.sh build $(TARGETS)` | 对每个 sibling 跑 `GOWORK=off go build ./...`。 |
| `test` | `./scripts/eco.sh test $(TARGETS)` | 对每个 sibling 跑 `GOWORK=off go test ./... -count=1`。 |
| `up` | `./scripts/eco.sh up $(TARGETS)` | 启动 launchable sibling（otel / customer-support）的 docker compose。 |
| `down` | `./scripts/eco.sh down $(TARGETS)` | 关停 docker compose。 |

### 3.2 `TARGETS=...` 用法

`Makefile:3` 默认 `TARGETS ?= all`。`scripts/eco.sh:39-49` `normalize_targets` 函数把 `all` 展开为完整 6 sibling list，否则按 `,` 拆分。例：

```bash
make build TARGETS=llm-agent,llm-agent-rag       # 只 build 这两个
make up TARGETS=llm-agent-customer-support       # 只起 customer-support compose
make down TARGETS=llm-agent-otel,llm-agent-customer-support  # 关两个
make test                                          # 等价 TARGETS=all
```

### 3.3 launchable vs library-only

`scripts/eco.sh:15-18, 32-37` 显式声明 `launchable_repos = (llm-agent-otel, llm-agent-customer-support)`：只有这两个有 `compose/compose.yaml`，可以 `make up`。其它 4 sibling 是 library-only — `make build` / `make test` 跑得了，但 `make up` 会因 `is_launchable` 返回 false 而拒绝（`scripts/eco.sh:158-167`）。

### 3.4 端口分配（避免 demo 共存冲突）

`scripts/eco.sh:91-106` 写死了端口环境变量：

- `customer-support` 用 `CS_APP_PORT=8080 / CS_GRAFANA_PORT=3000 / CS_OLLAMA_PORT=11434 / CS_OTEL_GRPC_PORT=4317 / CS_OTEL_HTTP_PORT=4318`
- `llm-agent-otel/compose` 用 `OTEL_DEMO_GRAFANA_PORT=3001 / OTEL_DEMO_OTLP_GRPC_PORT=4319 / OTEL_DEMO_OTLP_HTTP_PORT=4320`

两个 demo 可以并存（Grafana 3000 vs 3001）。这个分配是 umbrella 层的小协议；进一步细化由 sibling 各自的 `compose.yaml` 决定。

### 3.5 设计取舍：为什么 Makefile 这么薄

Makefile 总共 40 行，没有任何 recipe 逻辑 — 所有真实工作在 `scripts/eco.sh`。这种"Makefile 是入口、bash 脚本是实现"的分离是有意的：

- **跨平台维护性**：Makefile 在 BSD make vs GNU make 之间有微妙差异；把逻辑放 bash 让脚本对所有 POSIX 系统行为一致。
- **入口稳定**：`make build` 这个用户接口永远不变；bash 脚本可以随意 refactor。
- **可独立调用**：CI 直接调 `scripts/stdlib-only-check.sh` 而不通过 `make`，避免 GitHub Actions runner 上 `make` 行为差异。

---

## 4. `cmd/depcheck` 工具

### 4.1 这是什么

`cmd/depcheck/main.go`（437 行，单文件 stdlib-only）。它做三件事：

1. **解析每个 sibling 的 `go.mod`**，提取所有 `github.com/costa92/*` 的 `require` 行（`main.go:73-126` `parseGoMod / parseRequireLine`）。
2. **构建跨仓 DAG**，把"A 在它的 `go.mod` 里 require B"翻译成"A → B"边（`main.go:144-174` `buildDAG`）。
3. **拓扑排序输出 cascade 顺序 + 标记 stale pin**（`main.go:184-252` `topoSort`，`main.go:313-334` `detectStale`）。

### 4.2 它为什么需要存在

当 `llm-agent-rag` bump 到新 patch 时，需要先在 `llm-agent` 更新它的 require version，然后 `llm-agent-otel` 在它的 `go.mod` 里更新对 `llm-agent` 与 `llm-agent-rag` 的 require，**按拓扑顺序**逐 sibling bump。`depcheck` 把这个顺序机器化输出：

```
CASCADE ORDER (bump leaves first):
  1. llm-agent-rag                latest:v1.0.5   pins: (no in-ecosystem deps)
  2. llm-agent                    latest:v0.6.1   pins: (none — P0-2 dropped rag back-edge)
  3. llm-agent-providers          latest:v0.2.2   pins: llm-agent@v0.5.1
  4. llm-agent-flow               latest:v0.1.1   pins: llm-agent@v0.5.1
  5. llm-agent-otel               latest:v0.2.2   pins: llm-agent@v0.5.1, llm-agent-rag@v1.0.1, llm-agent-flow@v0.0.7
  6. llm-agent-customer-support   latest:v0.2.3   pins: llm-agent@v0.5.1, llm-agent-otel@v0.2.2, llm-agent-providers@v0.2.2, llm-agent-flow@v0.0.7, llm-agent-rag@v1.0.3

STALE PINS (2026-05-23 snapshot, v1.3 perf-wave 闭合后):
  - llm-agent-providers pins llm-agent@v0.5.1 (latest v0.6.1)
  - llm-agent-flow pins llm-agent@v0.5.1 (latest v0.6.1)
  - llm-agent-otel pins llm-agent@v0.5.1 (latest v0.6.1)
  - llm-agent-otel pins llm-agent-rag@v1.0.1 (latest v1.0.5)
  - llm-agent-otel pins llm-agent-flow@v0.0.7 (latest v0.1.1)
  - llm-agent-customer-support pins llm-agent@v0.5.1 (latest v0.6.1)
  - llm-agent-customer-support pins llm-agent-rag@v1.0.3 (latest v1.0.5)
  - llm-agent-customer-support pins llm-agent-flow@v0.0.7 (latest v0.1.1)
```

`STALE` 行触发 `os.Exit(1)`（`main.go:434-436`），所以 `depcheck` 本身可以做硬门 — 但 umbrella CI **故意只用 informational 模式**（`umbrella.yml:113-114` `|| true`），让运维通过 artifact 看到 stale 而不阻塞 PR。

### 4.3 K3 keystone 体现：拓扑 + 回滚 cascade（B3）

`depcheck` 的 `topoSort`（`main.go:184-252`）是修改版 Kahn 算法：

- 正常情况下按入度 0 → emit → 减少依赖者入度。
- **back-edge 处理**：算法对 cycle 不报死循环而是**选 cycle 中 out-degree 最小的节点先 emit**，alphabetical tie-break（`main.go:226-244`）。**当前态（2026-05-23）**：P0-2 已删 `llm-agent` → `llm-agent-rag` 反向边，cycle 路径不再触发；但算法仍保留兜底能力以容忍未来其他合法 back-edge 场景。

历史背景：v1.1 时期允许 `llm-agent → llm-agent-rag` 作为 facade 反向边；P0-2（2026-05-22，commit 6029565）采纳 drop-dependency 路径将其撤销，核心 `llm-agent` 现严格 stdlib-only。

### 4.4 调用约定

```bash
cd cmd/depcheck
GOWORK=off go run .            # 人类可读表
GOWORK=off go run . --json     # CI artifact 用的 JSON
GOWORK=off go run . --root /custom/path  # 显式 root
```

`findUmbrellaRoot`（`main.go:339-371`）会向上 walk 5 层找 `README.md + Makefile + 至少一个 sibling clone` 的目录，所以从任何 sibling 目录里跑也能定位到 umbrella root。

### 4.5 为什么 stdlib-only

按 `main.go:1-17` 包注释：`depcheck` 自身**禁止**引入任何第三方依赖（即使在 umbrella `cmd/` 下）。原因：
1. 它的运行环境是 CI runner / 运维 shell，不应该拉 dep tree。
2. 它需要在 sibling repo 处于"半 bump"状态时也能工作 — 此时 `go.mod` 可能有 inconsistent pin，stdlib 解析最稳。
3. 跨仓巡检工具的目标是 *最小依赖、最大确定性*。

---

## 5. `go.work` 与 `GOWORK=off` 规则

### 5.1 `go.work` 在 umbrella 的角色

`scripts/workspace.sh:1-23` 写一个 `<root>/go.work` 把所有 6 sibling 的本地 path 接入同一个 Go workspace。这样：

- 在 `llm-agent-providers/` 修改一行代码，`llm-agent-customer-support/` 立即 build 时用本地修改后的版本，无需 `replace`。
- 任意 `go test` 跨多个 sibling 的测试可以 stitch 起来跑。

`umbrella.yml:64-69` 要求 umbrella root **必须存在** `go.work` 文件（`test -f go.work`），所以 umbrella repo 本身的 `go.work` 是 commit 进版本控制的。

### 5.2 每个 sibling 的 `.gitignore`

按 `README.md:77-79`：每个 sibling 仓库的 `.gitignore` 都覆盖 `go.work` — 让本地 workspace 不污染 sibling repo 本身。如果开发者在 sibling 仓内运行 `go work init`（或 umbrella 的 `make workspace` 把 `go.work` 写到 umbrella root），sibling 仓内不会出现脏文件。

### 5.3 CI 的 `GOWORK=off`

`umbrella.yml:73, 80, 87, 94, 101` 都强制 `GOWORK=off`：让每个 sibling 在 CI 上**只**根据自己的 `go.mod` build，不依赖 workspace。这避免：

- "在 umbrella workspace 跑得过，单独 release 跑不过" 的灾难。
- 任何 sibling 在 release 前 push 一个错误 `go.work` 而被 cascade 影响。

`scripts/eco.sh:82` `run_go_cmd` 函数同样在 `make build` / `make test` 里强制 `GOWORK=off`（即使在本地，跑 `make build` 也不享受 workspace）。要享受 workspace，必须直接 `cd <sibling> && go build` — 这是有意的：让 `make build` 在本地与 CI 行为一致。

### 5.4 设计意图小结

| 场景 | `go.work` 状态 | `GOWORK` |
|---|---|---|
| umbrella root commit | 有，已 commit | — |
| sibling repo commit | `.gitignore` 覆盖，无 | — |
| 本地 `make build`/`make test` | umbrella root 的存在 | **强制 off** |
| 本地 `cd sibling && go build` | umbrella root 的会被自动发现 | on（默认） |
| CI（每个 sibling job） | 通过 `actions/checkout@v4 path=<sibling>` 单独 checkout 的目录，**没有** `go.work` | 显式 off 兜底 |

这种"sibling 各自 ignore，umbrella commit，CI 强制 off"三组合是当前生态最微妙的工程契约之一。它的硬保障来自 `INFRA-04` 规则（`README.md:74-76`）：tag 分支禁 `replace` 指令；以及 `umbrella.yml:64-69` 要求 root `go.work` 存在 — 任何破坏这个组合的 commit 都会被 CI 抓住。

---

## 6. B2 / B3 / B4 CI 闸子

### 6.1 闸子总览

`.github/workflows/umbrella.yml` 共 3 个 job：

| Job 名 | 闸子代号 | 路径 | 角色 |
|---|---|---|---|
| `cross-repo-build` | — | `umbrella.yml:14-103` | 5 sibling 单独 checkout，跑 `GOWORK=off go vet/build/test` |
| `cross-repo-build > "B3 — depcheck cascade tool"` step | **B3** | `umbrella.yml:105-121` | depcheck JSON artifact，**informational**，不 fail PR |
| `smoke` | **B2** | `umbrella.yml:123-191` | build `flowd` 二进制，启动后探 `/healthz` `/flows`，验证 binary 可启动 |
| `stdlib-only-gate` | **B4** | `umbrella.yml:193-214` | 跑 `scripts/stdlib-only-check.sh`，硬门 |

### 6.2 B2 — flowd 二进制冒烟门

`umbrella.yml:151-191`：

```bash
cd llm-agent-flow
GOWORK=off go build -o /tmp/flowd ./cmd/flowd
/tmp/flowd --addr 127.0.0.1:7861 --token= > /tmp/flowd.log 2>&1 &
sleep 2
curl -fsS http://127.0.0.1:7861/healthz
curl -fsS http://127.0.0.1:7861/flows  # auth-disabled when token=""
kill -TERM <pid>
```

**为什么需要**：flowd 是新引入的 binary（v0.0.2 + v0.0.8 之后），它的 `cmd/flowd/main.go` 启动顺序、port 绑定、authenticator 接入容易出 silent breakage。`flow/...` 包级别测试只 covers 接口，进程级冒烟不在范围内。B2 把"我能不能跑起来"做成 CI 一等公民。

### 6.3 B3 — depcheck cascade 工具门

`umbrella.yml:105-121`：

```bash
cd cmd/depcheck
GOWORK=off go vet ./...
GOWORK=off go test ./... -count=1
GOWORK=off go run . --json | tee depcheck.json || true
GOWORK=off go run . || true
```

注意两个 `|| true` — 让 B3 在 stale 时**不 fail**，只把 `depcheck.json` 作为 artifact 上传（`umbrella.yml:116-121`）。**设计取舍**：stale pin 不应该阻塞 sibling 的功能 PR — 它只意味着"该协调一次 bump wave 了"，是发布管理层面的信号而非合并层面的硬门。

如果运维想把它变成硬门，把 `|| true` 删掉即可；但当前共识是保留 informational。

### 6.4 B4 — 核心 stdlib-only 断言门

`umbrella.yml:193-214` 调 `scripts/stdlib-only-check.sh`，硬门（任一项 fail → PR fail）。

脚本（`scripts/stdlib-only-check.sh:38-167`）三项断言：

1. **Assertion 1**（`stdlib-only-check.sh:41-91`）：`llm-agent/go.mod` 的 direct require block 必须**恰好 0 条**（P0-2 已删 RAG 反向边；脚本 message: `expected ZERO direct requires in core go.mod`）。
2. **Assertion 2**（`stdlib-only-check.sh:94-132`）：`cd llm-agent && GOWORK=off go list -deps ./...` 输出里只能包含：
   - stdlib 路径（无 `.` 字符，如 `context`、`encoding/json`）；
   - `vendor/` 前缀（stdlib 内部 vendoring）；
   - `crypto/internal/...`（go 1.26 后 stdlib 内部伪版本路径）；
   - `github.com/costa92/llm-agent` 与 `github.com/costa92/llm-agent-rag` 任意子包；
   - `golang.org/x/...`（rag 的 transitive 允许）。

   其他任何条目都视为 leak。
3. **Assertion 3**（`stdlib-only-check.sh:135-166`）：`policy/` / `budget/` / `agentstest/` 三个 sub-package 必须**完全 zero external dep**。这是因为这三个 sub-package 设计上要可被任何上游单独引入（DP-1 的极致）。P0-2 之后整个 `llm-agent` 都已严格 stdlib-only，该断言现在与 Assertion 1+2 重叠，但保留作 sub-package 级别的精细兜底。

### 6.5 子仓自己的 CI vs umbrella CI

| 范围 | umbrella CI 做的 | sibling CI 做的 |
|---|---|---|
| 编译可过 | 每个 sibling `GOWORK=off go vet/build/test` | 同 |
| 跨仓集成 | B2 跑 flowd binary 冒烟 | 没有跨仓视角 |
| 拓扑健康 | B3 depcheck artifact | 不可见 |
| stdlib 边界 | B4 三项断言 | 各自的 codeowner 评审 |
| API surface 锁定 | 不做 | 各仓 `_test.go` 内 `apisnapshot` 对照 `api/v*.snapshot.txt` 锁定 |
| Conformance 套件 | 不做 | 各仓自己（如 `llm-agent-providers/internal/contract/`，`llm-agent-rag/store/storetest/`） |
| INFRA-04 禁 replace | 不做 | 各仓 release 流程 |
| 端到端模型联调 | 不做 | sibling 自己的 nightly / live 测试（如 `nightly-ollama-live.yml`） |

**分工原则**：umbrella 只看跨仓**整体属性**（依赖方向、stdlib 边界、binary 启动），不替任何 sibling 看自己的内部行为。这是为了让 umbrella CI 跑得快、报错明确、不需要跟着 sibling 演进。

---

## 7. 依赖方向与硬约束

`README.md:67-89` 列了 7 条规则。下面把每条展开 *为什么* —— 这部分是其他文档没充分展开的。

### 规则 1：核心 `llm-agent` 保持 stdlib-only

**为什么**：

- `llm-agent` 是生态的"读得完每一行"那个仓 — 任何下游服务都希望能审查它的全部代码。一旦它的 `go.sum` 出现 SDK，下游（每个 sibling、每个 customer service）的 `go.sum` 都被传染。
- 任何 *设计差异* 都会暴露：如果核心依赖 `openai-go`，那 `anthropic-sdk-go` 的 stream 语义差异就要进核心；如果核心依赖 `otel-go`，那核心就要跟 OTel SDK 的 spec drift 一起 release。这破坏 K1/K3 keystone。
- 它使核心可以放心地引入到任何受限环境（air-gapped、规范严格的 enterprise）。

**硬保障**：B4 闸子（§6.4）。

### 规则 2：tag 分支不允许 `replace`

**为什么**：`replace` 是 local-dev escape hatch，让本地修改不必先 push tag 就能被下游消费。在 tag 上保留 `replace` 意味着这个 release 实际上指向某个未发布的 ref —— 这是一个**幽灵 release**，会让任何尝试 `go get` 那个 tag 的人拿到 broken module。

**硬保障**：每个 sibling 自己的 INFRA-04 流程（不是 umbrella CI；umbrella 不 enforce sibling 的 release branch policy）。

### 规则 3：`go.work` 一律 `.gitignore`，CI `GOWORK=off`

**为什么**：见 §5.3。本质是隔离"本地 workspace 行为"与"release build 行为"。

### 规则 4：无 K8s / Helm

**为什么**：

- 整个 ecosystem 的设计目标是给开发者"组件库"，让他们自由部署到他们公司既有的 K8s / ECS / Lambda / on-prem。
- K8s 模板是 *deployment opinion*；这个 ecosystem 的 opinion 集中在 *API contracts*（K1/K2/K3）。
- `llm-agent-customer-support/compose/` 用 docker-compose 是因为 demo 必须能 `docker compose up` — 但仅限 demo。

### 规则 5：Capabilities per `(provider × model)`（K2）

**为什么**：见 `source-design-llm-agent-providers.zh-CN.md` §2.1。Ollama `llama2` 不支持 tool、`qwen3-coder` 输出 XML、`llama3.1` 输出 `<|python_tag|>` —— 按 provider-level 暴露 capability，agent 在运行时只能瞎试。Provider 构造时绑 model（`openai.New(openai.WithModel("gpt-4o"))`），`Info().Capabilities` 立刻反映那个 model 的真实状态。

### 规则 6：OTel as decorator wrapper（K3）

**为什么**：见 `source-design-llm-agent-otel.zh-CN.md` §2.1。Hook 模型会把可观测语义焊死在核心 API 上、引入 OTel 依赖、迫使核心库随 OTel semconv 演进。Decorator (`otelmodel.Wrap(inner) ChatModel`) 解决全部三个问题。

### 规则 7：StreamEvent 是 typed union（K1）

**为什么**：见架构图 §9 与 `source-design-llm-agent.zh-CN.md` §4.7。最小公分母 chunk 流会让 agent 层重做 reassembly；统一 `Kind` 枚举 + 稳定 `Index` 字段把这个负担收敛到 provider adapter，让 agent 层永远拿到 typed 事件。

---

## 8. 协调式发布流程（Phase 33 模式）

### 8.1 背景

v1.1 milestone（2026-05-20 关闭）的 Phase 33 "coordinated bump + re-tag wave" 是 ecosystem 的发布管理范式（`README.md:134-135`）。

### 8.2 触发条件

任一发生：

- `llm-agent-rag` 发布新 patch（fixed-point 升级）→ 整张图需要 cascade 更新对 rag 的 pin。
- `llm-agent` 核心新 feature → 至少 `llm-agent-otel`、`llm-agent-providers`、`llm-agent-customer-support` 需要 bump 对 core 的 pin。
- 某个 sibling 修了关键 bug，下游需要拉新版本。

### 8.3 流程（实际操作）

```
1. depcheck 确认 stale set
   $ cd cmd/depcheck && go run .
   STALE PINS:
     - llm-agent-otel pins llm-agent-flow@v0.0.7 (latest v0.1.1)
     - llm-agent-customer-support pins llm-agent-flow@v0.0.7 (latest v0.1.1)

2. 按 cascade 顺序逐 sibling 操作（拓扑：叶子→树根）
   For each repo in topo order:
     a. cd <repo>
     b. go get github.com/costa92/<dep>@<latest>
     c. go mod tidy
     d. go vet ./... && go test ./...
     e. git commit -am "chore(deps): bump <dep> to <ver>"
     f. git tag v<new>; git push --tags

3. umbrella commit: 更新 README.md 的 roster table 中 tag 列
   $ vim README.md  # 改对应 sibling 的 **vX.Y.Z** 到 latest
   $ git commit -am "docs(roster): bump after <cascade name>"

4. 触发 umbrella CI 跑 cross-repo-build + B2/B3/B4 验证整链一致
```

### 8.4 为什么 sibling 顺序很关键

按规则 5（K2 capability per-model）与 K1 streaming union，**接口契约**变化（即使是 additive）会让下游必须重 build / 重测。如果不按拓扑顺序：

- 假设先 bump `llm-agent-otel` 对 `llm-agent` v0.6.0 的 pin，但 v0.6.0 还没 release —— PR 拿到 broken module。
- 假设先 bump `llm-agent` 用了新 `llm-agent-rag` v1.0.3 的 symbol，但 v1.0.3 还在 unreleased — 同样断。

`depcheck` 输出的 topo 顺序保证：当前一项 emit 时，它的所有 in-ecosystem deps 都已 emit。

### 8.5 不是同步 release

umbrella 没有"统一发版"概念 — 每个 sibling 各自打 tag。Phase 33 是 *coordinated*：意思是"按拓扑、连续几个 PR、几天内做完"，但每个 PR 仍然是 sibling repo 自己的 release 流程。

这种设计让"小修小补"不必走 cascade — 只有真正影响 API 边界或 ecosystem-wide pin 的变更才走 Phase 33 模式。

---

## 9. `.planning/` 关系与 STATE / ROADMAP / REQUIREMENTS

### 9.1 两层 `.planning/`

按 `README.md:90-106`，存在**两层**：

- **Umbrella 层**：`<root>/.planning/`（若存在）—— 描述生态整体的 phase / milestone。当前 git 状态显示这个目录有内容（`.planning/PROJECT.md` 与 `.planning/README.md` 被 umbrella CI 检查存在，`umbrella.yml:65-68`），但具体文件未在本次源码深读范围内。
- **核心仓层**：`llm-agent/.planning/` —— 是**整个生态的 source of truth**（`README.md:96-106`）：
  - `llm-agent/.planning/PROJECT.md` — 项目定位、核心价值、硬规则
  - `llm-agent/.planning/STATE.md` — 当前 milestone + 活跃 phase
  - `llm-agent/.planning/ROADMAP.md` — 活跃 milestone 的 phase 计划
  - `llm-agent/.planning/REQUIREMENTS.md` — 当前 milestone 的需求 + traceability
  - `llm-agent/.planning/research/v1.1-ecosystem-alignment-SUMMARY.md` — 跨仓审计 + keystone KE-1…KE-7

### 9.2 为什么核心仓持有"生态级" planning

历史原因：`llm-agent` 是第一个仓库；早期所有 ecosystem-wide 决策（含 KE-1..KE-7）都在它的 `.planning/` 里讨论。即使后来 splitup 出 sibling 仓，规划文档没有迁出。

**好处**：单一真相之源，AI 工作流（claude-code 等）按一个固定路径找上下文。
**坏处**：sibling 仓 PR 时需要回去看核心仓的 `.planning/REQUIREMENTS.md` 看是否触发某条约束。这种"指向"关系在 `README.md:96-106` 显式声明。

### 9.3 sibling 仓自己的 `.planning/`

每个 sibling 也有自己的 `.planning/`，只承担自仓 milestone：例如 `llm-agent-providers/.planning/codebase/CONCERNS.md`、`llm-agent-flow/.planning/`、`llm-agent-customer-support/.planning/`。这些不是"生态级"，而是"sibling 内部 phase 跟踪"。

### 9.4 当前 milestone 状态（2026-05-21 快照）

按 `README.md:138-152`：

- **v1.0** — `llm-agent-rag` API stabilization — **shipped** 2026-05-21（v1.0.0 surface freeze + apisnapshot baseline）。
- **v1.3 perf-wave (rag)** — **shipped** 2026-05-23：P1-16 BatchEmbedder (v1.0.3) → P1-15 HybridRetriever 并发 (v1.0.4) → P1-1 pgvector index opt-in (v1.0.5)。三条都是 additive，v1 surface 不变。
- **v1.1** — Ecosystem alignment — **shipped** 2026-05-20。审计 PASS 5/5（`llm-agent/.planning/v1.1-MILESTONE-AUDIT.md`）。
- **v1.2** — Core Capability Deepening — **in flight**。主线是 `budget` / `policy` / `orchestrate.Supervisor` 三大核心能力。Phase 35 (budget/cancellation, CC-1) 在执行；Phases 36-38 计划 policy → supervisor → audit/close。

---

## 10. 典型本地工作流

### 10.1 首次进入 ecosystem

```bash
# 1) 克隆 umbrella
git clone https://github.com/costa92/llm-agent-ecosystem.git
cd llm-agent-ecosystem

# 2) bootstrap 6 个 sibling
make bootstrap                        # 实时 git clone

# 3) 写本地 workspace（可选，但推荐用于跨仓改）
make workspace                        # 写 <root>/go.work

# 4) 验证整链可 build
make build                            # GOWORK=off + 每个 sibling go build

# 5) 验证整链可 test
make test                             # GOWORK=off + 每个 sibling go test
```

### 10.2 跨仓修改（典型场景：给 `llm-agent` 加新方法、`llm-agent-customer-support` 立即使用）

```bash
# 0) 确保 workspace 已写
make workspace

# 1) 在 llm-agent 改代码
cd llm-agent
vim agents/agent.go                   # 加方法
go vet ./... && go test ./...

# 2) 在 customer-support 直接消费（workspace 已 stitch，无需 replace）
cd ../llm-agent-customer-support
vim internal/app/app.go               # 调用新方法
go vet ./... && go test ./...

# 3) 提 PR 时分两个 PR：
#    PR-A: llm-agent 改动（独立 review）
#    PR-B: llm-agent-customer-support 改动（PR-A merge + tag 后再开 PR-B）

# 4) PR-A merge 后，在 llm-agent 仓打 tag
cd ../llm-agent
git tag v0.5.2
git push --tags

# 5) 在 PR-B 里把 go.mod 的 pin 更新到 v0.5.2
cd ../llm-agent-customer-support
go get github.com/costa92/llm-agent@v0.5.2
go mod tidy
git commit -am "chore(deps): bump llm-agent to v0.5.2"
# 推 PR-B
```

### 10.3 启动 demo（customer-support）

```bash
# 起完整栈（ollama + otel-lgtm + grafana + app）
make up TARGETS=llm-agent-customer-support

# 验证
curl http://localhost:8080/healthz
curl -X POST http://localhost:8080/chat -d '{"message": "hello"}'
# Grafana: http://localhost:3000

# 关停
make down TARGETS=llm-agent-customer-support
```

### 10.4 排查 stale dep

```bash
cd cmd/depcheck
go run .                              # 看人类可读 cascade + stale
go run . --json | jq                  # 看 JSON
```

### 10.5 在 PR 推出前自检 CI 闸子

```bash
# 模拟 B4
ECOSYSTEM_ROOT=$PWD bash scripts/stdlib-only-check.sh

# 模拟 cross-repo-build
make build && make test

# 模拟 B2 (flowd smoke)
cd llm-agent-flow
GOWORK=off go build -o /tmp/flowd ./cmd/flowd
/tmp/flowd --addr 127.0.0.1:7861 --token= &
sleep 1
curl -fsS http://127.0.0.1:7861/healthz
kill %1
```

---

## 11. 遗留问题与改进点

按当前 source-design 深读发现，umbrella 层有以下值得改进的地方：

| # | 问题 | 影响 | 优先级 | 建议 |
|---|---|---|---|---|
| 1 | `PROJECT.md` 只列 5 sibling（`PROJECT.md:7-11`），未含 `llm-agent-flow` | 文档与 `README.md:31-39` / `cmd/depcheck/main.go:32-39` 的 6-sibling roster 不一致 | 中 | 把 `PROJECT.md` 第 7-11 行加上 `llm-agent-flow` |
| 2 | `Makefile` 没有"全栈起来测一个 turn"的 target | 跨仓集成测试只能手动 `make up + curl + make down` | 低 | 加 `make demo` target，自动 up → curl → down |
| 3 | B3 是 informational，stale pin 永远不阻塞 PR | 容易积累 stale；运维需要主动跑 `depcheck` 才能发现 | 中 | 给 B3 加一个"超过 N 个版本落后则 fail"的阈值 |
| 4 | umbrella CI 没有跨仓 *端到端* 闸子（如：跑 customer-support 启动 + 一次 chat round-trip） | binary 启动只验证了 flowd（B2），customer-support 没有同级别 smoke | 中 | 加 B5 customer-support smoke job |
| 5 | `cmd/depcheck` 不读 GitHub remote tag（只读 local `git tag`） | 如果运维没 `git fetch --tags`，"latest" 会偏旧 | 低 | 在 `loadRepoInfo` 内可选 `git fetch --tags` |
| 6 | `.planning/` 在 umbrella 与 `llm-agent` 各一份，但只有核心仓那份是 source of truth | 新人容易在 umbrella `.planning/` 找不到 milestone 信息 | 低 | 在 umbrella `.planning/README.md` 用一个明显的 redirect link 指向 `llm-agent/.planning/` |
| 7 | `scripts/eco.sh` 的端口分配（`:91-106`）写死，无法通过 ENV 覆盖 | 多 demo 共存时端口冲突难调 | 低 | 把端口默认值改成 `${CS_APP_PORT:-8080}` 等 fallback 形式 |
| 8 | `umbrella.yml:64-69` 只验证 root 文件存在，不验证 `go.work` 列了正确的 6 个 sibling | 如果 `go.work` 漏了某个 module，本地 workspace 行为异常但 CI 不报错 | 低 | 加一行 `grep -c "use ./llm-agent" go.work` 检查 |
| 9 | 没有"sibling repo schema check"（确保每个 sibling 至少有 `go.mod` / `README.md` / `.gitignore`） | 新 sibling 加入时 onboarding 全靠口口相传 | 低 | 加一个 `scripts/check-sibling-schema.sh` |
| 10 | umbrella 的 `cmd/depcheck/` 用 stdlib-only Go，但没有 `go.mod`（它是 umbrella 根的子目录而 root 无 go module） | 必须 `cd cmd/depcheck` 才能跑，对工具化使用不友好 | 低 | 给 `cmd/depcheck/` 一个独立 `go.mod`（依然 stdlib-only），让 `go install` 可直接安装 |

---

## 附录 A：关键文件清单（一目了然）

| 文件 | 行数 | 角色 |
|---|---|---|
| `README.md` | ~155 | 导航 + 依赖方向 + 规则 + roster |
| `PROJECT.md` | ~17 | 极简 scope 声明 |
| `Makefile` | 41 | 8 个 phony target |
| `cmd/depcheck/main.go` | 437 | stdlib-only 跨仓依赖巡检 |
| `scripts/eco.sh` | ~185 | bootstrap/pull/status/build/test/up/down 实现 |
| `scripts/workspace.sh` | 23 | 写 `<root>/go.work` |
| `scripts/stdlib-only-check.sh` | 173 | B4 闸子实现（3 项 fail-fast 断言） |
| `.github/workflows/umbrella.yml` | 214 | 4 个 CI job |
| `go.work` | — | 6-sibling workspace 引用 |

## 附录 B：与其他文档的指向

- 跨仓架构图与时序图：`./architecture-and-sequence-diagrams.zh-CN.md`
- 每个 sibling 的源码级深读：`./source-design-llm-agent*.zh-CN.md`（共 6 篇）
- 生态级设计评审：`./ecosystem-design-review.zh-CN.md`
- 重构与优化路线图：`./refactor-and-optimization-roadmap.zh-CN.md`
- 子系统设计要点速记：`./subsystems-design-notes.zh-CN.md`
- 当前项目分析（含历史快照）：`./current-project-analysis.md` (EN) / `./current-project-analysis.zh-CN.md` (zh)
