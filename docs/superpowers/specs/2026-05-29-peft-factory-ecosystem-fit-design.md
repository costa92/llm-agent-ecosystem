# PEFT-Factory 与 llm-agent-ecosystem 生态适配设计

> Date: 2026-05-29
> Status: draft for review
> Scope: 生态级分析与推荐落点，不包含实现计划

## 1. 目标

这份文档回答三个问题：

1. `PEFT-Factory` 提供的核心能力是什么。
2. 这些能力与当前 `llm-agent-ecosystem` 的哪一层相关。
3. 如果未来要吸收这类能力，最合理的生态落点是什么。

本文档只做生态级分析，不设计具体 API，不拆实现任务，不直接引入新仓库。

## 2. 输入与约束

### 2.1 当前生态事实

当前根仓是 9 个子项目的 umbrella workspace，本身不承载训练或推理 runtime。已有能力集中在：

- `llm-agent`: agent framework、tool/runtime、orchestration、evaluation
- `llm-agent-rag`: RAG / GraphRAG、retrieval、generation、diagnostics、eval
- `llm-agent-providers`: OpenAI / Anthropic / Ollama / DeepSeek / MiniMax provider adapters
- `llm-agent-otel`: OpenTelemetry decorators
- `llm-agent-flow`: serializable workflow / DAG runtime
- `llm-agent-memory*`: durable memory SDK、Postgres backend、gateway
- `llm-agent-customer-support`: 参考业务服务

其中最重要的边界约束已经写入 [`llm-agent/rl/doc.go`](../../../llm-agent/rl/doc.go)：

- Go 侧提供评测与训练代理接口
- Go 核心不承载 `SFT / GRPO / PPO / LoRA / any gradient-based training`
- 推荐模式是 Python 训练、产出 checkpoint、再通过服务化接口接回 Go 生态

### 2.2 PEFT-Factory 事实

根据其 README、PyPI 包元数据和公开目录结构，`PEFT-Factory` 的核心特征是：

- 统一的训练入口：CLI + WebUI
- 统一的训练配置模板与实验输出目录约定
- 多种 PEFT method 支持
- Hugging Face PEFT / AdapterHub / 自定义方法扩展
- 数据集、示例配置、evaluation、tests 的完整训练项目结构

支持的方法包含但不限于：

- LoRA 与其变体
- Prefix Tuning
- Prompt Tuning
- P-Tuning / P-Tuning v2
- IA3
- Bottleneck / Parallel / Sequential Adapter
- BitFit
- SVFT

它的本质不是推理 SDK，而是一个训练侧平台。

## 3. 结论

推荐结论如下：

- `PEFT-Factory` 值得被当前生态吸收，但吸收的对象应是“训练侧平台能力”，不是某个单独 PEFT method。
- 当前 9 个子项目中，没有一个适合作为 `PEFT-Factory` 的直接归宿。
- 最合理的未来落点是新增独立训练子项目，可命名为 `llm-agent-train`。
- 该子项目应采用 `Go control plane + Python training backend` 架构。
- 当前 `llm-agent` 的边界不应被打破，尤其不能推翻 `rl` 包已经明确的“训练在 Python，Go 负责桥接、评测、接入”的原则。

## 4. 能力映射

### 4.1 训练入口层

`PEFT-Factory` 提供：

- CLI
- WebUI
- 训练任务启动入口
- 配置模板展开
- 输出目录规范

当前生态现状：

- 有 `make`、脚本、`workspace/build/test/up/down` 等工程入口
- 没有统一训练任务入口
- 没有训练产品面

结论：

这是当前生态的明显空白，但它属于训练控制层，不属于现有任何单一子项目的职责。

### 4.2 训练方法层

`PEFT-Factory` 提供：

- 多种 PEFT method 的统一抽象
- 依托 Hugging Face PEFT、AdapterHub 和自定义实现的训练后端

当前生态现状：

- 没有任何 PEFT method 实现
- 也没有 Python 训练执行子系统

结论：

这是当前生态的缺失能力，但不应直接进入 Go 核心仓库。它属于 Python 训练后端范畴。

### 4.3 数据与实验层

`PEFT-Factory` 提供：

- 数据集组织
- 示例配置
- 实验目录规范
- benchmark / evaluation 结构

当前生态现状：

- `llm-agent/rl` 有评测抽象
- `llm-agent-rag/eval` 有 RAG 评测能力
- `flow`、`otel`、`memory` 提供可编排、可观测、可持久化基础
- 但缺少面向训练实验的统一组织层

结论：

这是最值得对齐的部分之一。当前生态已有外围基础设施，但没有训练实验平台。

### 4.4 评测层

`PEFT-Factory` 提供：

- 训练后评测
- 方法比较
- 标准化实验结果组织

当前生态现状：

- `llm-agent/rl` 适合复用评测抽象思想
- `llm-agent-rag/eval` 适合提供任务评测参考
- 但两者都不是训练评测平台本身

结论：

当前生态在“评测思想”上有邻接能力，在“训练评测产品层”上仍然是空白。

### 4.5 产物接入层

`PEFT-Factory` 最终产出的是：

- checkpoint
- adapter
- merge/unmerge 相关训练结果
- 可用于推理部署的模型产物

当前生态现状：

- `llm-agent-providers` 负责接推理服务
- `llm-agent` 负责 agent runtime 消费模型
- `llm-agent-rag` 可消费训练后模型做检索问答验证

结论：

这部分是最适合与现有生态打通的接口面。训练系统应作为上游产物生成器，现有生态继续作为下游消费层。

## 5. 推荐落点

推荐未来新增独立子项目：

- 可命名为：`llm-agent-train`
- 定位：训练控制平面与训练产物接入层
- 生态角色：现有推理、RAG、memory、flow、otel 体系的上游补充

它不应替代现有任何子项目，而应补上当前生态中不存在的一层：

- 训练任务管理
- 训练实验管理
- 训练产物管理
- 训练评测调度
- 训练结果向现有推理生态的回流

## 6. 不推荐落点

### 6.1 不推荐放进 `llm-agent`

原因：

- `llm-agent` 是 agent framework，不是训练框架
- `rl` 包已经明确排除了 LoRA、SFT、PPO、GRPO 等梯度训练
- 将训练平台塞入 `llm-agent` 会破坏当前清晰边界

### 6.2 不推荐放进 `llm-agent-rag`

原因：

- `rag` 的职责中心是 retrieval、generation、GraphRAG、RAG eval
- 即便已有 eval/benchmark，也只是相邻能力
- 把训练平台放入 `rag` 会造成职责漂移

### 6.3 不推荐放进 `llm-agent-providers`

原因：

- `providers` 负责供应商 API 适配，不负责训练生命周期
- 它应消费训练结果，而不是承担训练编排和执行

### 6.4 不推荐放进根仓

原因：

- 根仓是生态协调层，不是 runtime 层
- 它适合承载路线图、分析、规划，不适合承载训练实现

## 7. Go 与 Python 的职责边界

推荐的架构不是“用 Go 重写 PEFT-Factory”，而是：

- Go 负责 control plane
- Python 负责 training backend

### 7.1 Go 适合承担的部分

- 训练任务创建、取消、重试、状态查询
- 训练规格定义与参数校验
- 训练流程编排
- checkpoint / adapter 元数据管理
- benchmark / evaluation 调度
- 训练结果注册与对外查询
- 与 `flow`、`otel`、`memory` 的集成
- 与现有推理生态的接入桥接

### 7.2 Python 必须承担的部分

- 基于 PyTorch 的实际训练执行
- PEFT method 实现
- Hugging Face `transformers` / `peft` / `trl` / `adapters` 集成
- GPU 张量计算、autograd、optimizer
- mixed precision、distributed training、显存优化

### 7.3 生态一致性结论

这种分层与当前生态边界一致：

- Go 继续保持 framework / runtime / orchestration / evaluation / serving integration 的优势
- Python 承担训练执行这一现实上依赖 Hugging Face 生态的部分

## 8. 与现有生态的高层接口关系

未来若新增 `llm-agent-train`，其与现有子项目的关系应是：

- 对 `llm-agent`：提供训练产物注册与评测结果回流，不把训练逻辑并入 agent runtime
- 对 `llm-agent-providers`：消费训练后部署出的推理 endpoint 或模型服务，而不是修改 provider 抽象
- 对 `llm-agent-rag`：支持训练后模型在 RAG 任务上的评测与比较
- 对 `llm-agent-flow`：可选地用作训练作业状态编排或任务流水线控制
- 对 `llm-agent-otel`：统一训练与评测过程的 observability
- 对 `llm-agent-memory*`：仅在需要实验记录、作业元数据或结果索引时产生弱耦合，不应让 memory 成为训练内核依赖

## 9. 非目标

本文档明确不主张：

- 在现有 `llm-agent` 仓库内直接实现 LoRA / SFT / PPO / GRPO
- 用 Go 直接重写 Hugging Face PEFT / TRL 训练内核
- 将 `PEFT-Factory` 的全部 Python 代码机械迁移进当前生态
- 在本阶段决定具体 API、数据库 schema 或任务编排细节

## 10. 决策摘要

一句话总结：

`PEFT-Factory` 对应的不是当前生态中某个子项目的功能空位，而是一个尚未存在的新层级。

因此推荐：

- 先在根仓保留本分析文档作为生态决策依据
- 后续若进入实施阶段，再围绕未来 `llm-agent-train` 编写独立 spec 与 implementation plan
- 架构原则固定为 `Go control plane + Python training backend`

## 11. 参考来源

- `PEFT-Factory` GitHub 仓库：<https://github.com/kinit-sk/PEFT-Factory>
- `PEFT-Factory` README：<https://github.com/kinit-sk/PEFT-Factory/blob/main/README.md>
- `PEFT-Factory` PyPI 包信息：<https://pypi.org/project/peftfactory/>
- 当前生态根仓 README：[`README.md`](../../README.md)
- 当前生态分析：[`docs/current-project-analysis.md`](../../current-project-analysis.md)
- 当前训练边界定义：[`llm-agent/rl/doc.go`](../../../llm-agent/rl/doc.go)
