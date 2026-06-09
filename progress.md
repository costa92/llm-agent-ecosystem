# 多轮功能支持审核进度

## 2026-06-08

- 初始化多轮审核记录文件。
- 当前已知工作区变更：`.planning/eino-gap-plans/README.md` 已修改，非本轮代码实现变更。
- Phase 1 完成：已按代码入口确认 9 个功能项的存在性状态。
- Phase 2 完成：已复核测试覆盖和成熟度边界。
- 目标测试结果：`llm-agent-contract` 下 `go test ./prompt` 通过。
- 目标测试限制：`llm-agent-flow/v2` 直接测试需要下载远端依赖，网络超时；`llm-agent-flow/cmd/flowd/server` 测试也因依赖下载失败未完成。
- Phase 3 完成：当前 `.planning/eino-gap-plans/README.md` 对前四项的“待补充”判断已不符合当前代码状态，建议改为已支持/有边界。
- Phase 4 完成：多轮审核结论已汇总。
- Phase 5 开始：实现 Workflow 字段级数据映射，范围收敛在 `llm-agent-flow/v2`。
- Phase 5 实现完成：`llm-agent-flow/v2` 新增 `Flow.Mappings`，支持从全局输入或节点输出端口选字段并组装目标节点输入。
- 已补充测试：input 字段组装、node output 字段激活、同一目标 port 多字段合并、缺失路径错误、混合 source 校验、mapping 参与环检测、resume humanInput mapping。
- 已通过测试：在 `llm-agent-flow/v2` 执行 `GOCACHE=/tmp/aics-go-build GOMODCACHE=/tmp/aics-go-mod go test ./flow ./flow/graph ./internal/apisnapshot -count=1` 通过。
- 独立 agent 审核后处理：补充空 path segment 的 Validate 静态校验与测试；补充未知 node-source port 不激活 target 的语义测试。
- 复测通过：`go test ./flow -run 'TestMapping' -count=1` 通过；`go test ./flow ./flow/graph ./internal/apisnapshot -count=1` 通过。
- Phase 6 开始并完成：实现统一 Agent 运行事件首版。
- 已新增：`llm-agent-contract/agents.RunEvent` / `RunEventKind`，`RunEventFromStepEvent`，`RunEventFromStreamEvent`。
- 已新增：`llm-agent-flow/v2/flow.RunEventFromFlowEvent`，保留 Flow interrupt/suspend 的 `ResumeToken` 与 payload。
- 已更新：`llm-agent/aliases.go` 重新导出统一事件类型、常量和转换函数。
- 已通过测试：`GOCACHE=/tmp/aics-go-build GOMODCACHE=/tmp/aics-go-mod go test ./...` 于 `llm-agent-contract` 通过。
- 已通过测试：`GOCACHE=/tmp/aics-go-build GOMODCACHE=/tmp/aics-go-mod go test ./... -run '^$'` 于 `llm-agent` 通过。
- 已通过测试：`GOCACHE=/tmp/aics-go-build GOMODCACHE=/tmp/aics-go-mod go test ./flow ./flow/graph ./internal/apisnapshot -count=1` 于 `llm-agent-flow/v2` 通过。
- Phase 7 开始并完成：实现统一 Callback / Aspect 首版。
- 已新增：`llm-agent-contract/agents.Callback` / `CallbackFunc`、`NoopCallback`、`EmitRunEvent`、`ChainCallbacks`。
- 已新增：`llm-agent.WrapAgent` / `ObserveAgent`，把 Agent `Run` / `RunStream` 镜像成统一 `RunEvent` 回调。
- 已验证：callback panic 会被 recover，不影响 Agent 主流程。
- 已通过测试：`GOCACHE=/tmp/aics-go-build GOMODCACHE=/tmp/aics-go-mod go test ./...` 于 `llm-agent-contract` 通过。
- 已通过测试：`GOCACHE=/tmp/aics-go-build GOMODCACHE=/tmp/aics-go-mod go test ./...` 于 `llm-agent` 通过。
- 已通过测试：`GOCACHE=/tmp/aics-go-build GOMODCACHE=/tmp/aics-go-mod go test ./flow ./flow/graph ./internal/apisnapshot -count=1` 于 `llm-agent-flow/v2` 通过。
- Phase 8 开始并完成：实现预置 Agent Pattern 产品化首版。
- 已新增：`llm-agent/patterns` catalog/factory。
- 已支持：Simple、ReAct、FunctionCall、PlanAndSolve、Reflection、Workspace 单 Agent preset。
- 已支持：Supervisor、FanOutFanIn、RoundRobin、RolePlay 多 Agent factory。
- 边界：Workspace 只使用调用方提供的 Registry，不默认启用 shell/terminal；未引入 `llm-agent-builtin` 作为核心依赖。
- 已通过测试：`GOCACHE=/tmp/aics-go-build GOMODCACHE=/tmp/aics-go-mod go test ./patterns -count=1` 于 `llm-agent` 通过。
- 已通过测试：`GOCACHE=/tmp/aics-go-build GOMODCACHE=/tmp/aics-go-mod go test ./...` 于 `llm-agent` 通过。
- Phase 9 开始并完成：实现 DevOps 可视化与调试体验首版。
- 已新增：`llm-agent-flow/cmd/flowd/server` 的 `GET /flows/{id}/debug` flow 拓扑调试视图。
- 已新增：`llm-agent-flow/cmd/flowd/server` 的 `GET /runs/{id}/debug` run 级 trace/debug 聚合视图。
- run debug 返回：run record、debug event 列表、节点状态聚合、timeline、`/runs/{id}/replay` path、suspended 摘要。
- 已补充测试：flow debug、run trace summary、failed run、skipped node、suspended run、missing run、malformed event payload。
- 已通过静态检查：`git diff --check` 于 `llm-agent-flow` 通过。
- 测试限制：`GOCACHE=/tmp/aics-go-build GOMODCACHE=/tmp/aics-go-mod go test ./cmd/flowd/server -run 'TestDebug' -count=1` 因网络无法解析/下载依赖失败。
- 授权后重试限制：`GOCACHE=/tmp/aics-go-build go test ./cmd/flowd/server -run 'TestDebug' -count=1` 仍因 `google.golang.org/genproto@v0.0.0-20240903143218-8af14fe29dc1` 从 `proxy.golang.org` 下载超时失败。

## 2026-06-09

- 继续执行 Phase 9 收尾验证。
- 已通过静态检查：`git diff --check` 于 `llm-agent-flow` 通过。
- 复测默认 `proxy.golang.org` 仍失败：`google.golang.org/genproto@v0.0.0-20240903143218-8af14fe29dc1` 下载超时。
- 已通过测试：`GOPROXY=https://goproxy.cn,direct GOCACHE=/tmp/aics-go-build go test ./cmd/flowd/server -run 'TestDebug' -count=1` 于 `llm-agent-flow` 通过。
- 已通过测试：`GOPROXY=https://goproxy.cn,direct GOCACHE=/tmp/aics-go-build go test ./cmd/flowd/server -count=1` 于 `llm-agent-flow` 通过。
