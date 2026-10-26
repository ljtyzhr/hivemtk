# 全链路业务监控追踪体系（设计 + 论证）

> 状态：已实现并部署运行态（2026-08-08 二次 redesign；2026-08-08 增强：自动采集 agent 多轮/多工具 + 全链路 UI 下钻 + 异步非阻塞解耦）。
> 设计定位：**围绕核心业务链路的生命周期监控**，而非系统监控（CPU/内存/Prometheus 等）。
> 后端 `user-server` 已重建，迁移自动建表 `message_trace`（节点 span 模型）并给 `message_hub` 加 `trace_id` 列。

## 1. 背景与痛点

此前排查一条消息（如 `884899` / 会话 `63bd5238...`）卡在下行队列，只能靠**临时拼 SQL** 还原时间线：
- `message_hub` 记录了入站/出站，但**没有把「同一轮 客户 inbound ↔ AI outbound」串起来的关联键**；
- `inbox_conversations` 漏同步、`pending` 永久孤儿、重复消息等问题**缺乏自动发现手段**；
- 没有按业务节点观测「每个环节入参/出参/耗时/是否符合预期」的能力，更没有按渠道拆分。

## 2. 设计目标（核心定位）

1. **业务生命周期监控，不是系统监控**：只盯核心链路「消息上报 → AI 处理 → 出站入队 → 收件箱同步 → 下行出库 → 送达确认」，不做基础设施指标。
2. **节点级可观测**：生命周期中**每个节点**都记录
   - **入参 Input**（请求 / 触发条件）
   - **出参 Output**（处理结果）
   - **响应时间 DurationMs**（本节点处理耗时）
   - **预期结果 Expected**（正常应得到什么）
   - **实际 / 异常结果 Status(ok|abnormal) + Abnormal**（偏离预期的异常详情）
   支持还原「某节点为什么没得到预期结果」。
3. **符合多渠道特性**：以 `Channel`（= `message_hub.Platform`：xiaohongshu / douyin / xianyu / telegram …）为维度，**按渠道 × 节点**聚合响应时间与异常率，定位某渠道特有的链路问题。
4. **零侵入**：埋点 best-effort，任何失败只记日志，**绝不阻断业务主链路**。
5. **可下钻**：概览 → 异常 → 按渠道节点健康 → 单会话/单轮生命周期时间线 → 节点明细。

## 3. 核心模型

### 3.1 生命周期节点（固定顺序，画链路图 + 算节点间时延）
| 顺序 | 节点 | 含义 |
|---|---|---|
| 1 | `ingest` 上报接入 | 桥接 / 私域 webhook 收到客户消息 |
| 2 | `ai_dispatch` AI 处理 | 编排器生成回复 / 决策（自动回复 / 转人工 / 接管） |
| 3 | `outbound_enqueue` 出站入队 | 写入 `message_hub`(outbound, status=pending) |
| 4 | `inbox_sync` 收件箱同步 | `UpsertFromHubMessage` 同步 `inbox_conversations` |
| 5 | `downlink_fetch` 下行出库 | 桥接 `GET /outbox` 拉取 pending 出站 |
| 6 | `delivered_ack` 送达确认 | 桥接 `POST /outbox/ack` 标记 delivered |

### 3.2 `message_trace` 节点 span 表（追加写、不可变）
每发生一个节点，追加一条记录，字段：
`trace_id` / `conversation_id` / `account_id` / `channel` / `node` / `node_order` / `direction` / `msg_id` /
`input`(text) / `output`(text) / `duration_ms` / `expected`(text) / `status`(ok|abnormal) / `abnormal`(text) / `error`(text) /
`parent_node` / `span_kind` / `turn_index` / `tool_name` / `agent_id` / `created_at`。
索引：`conversation_id` / `trace_id` / `channel` / `node` / `node_order` / `duration_ms` / `created_at` / `parent_node` / `span_kind` / `turn_index`。

### 3.3 关联键
- **`conversation_id`**：会话级串联（同一会话全部节点可见）。
- **`trace_id`**：轮次级串联（同一轮 客户 inbound ↔ AI outbound 共享）。
  - inbound 落库 `LinkInboundTraceID` 生成新 trace_id；
  - outbound 落库 `LinkOutboundTraceID` **优先复用同会话最近 inbound 的 trace_id**，无则新生成。
- **`channel`**：多渠道维度，监控按渠道聚合。

### 3.4 层级 span（agent 多轮 / 多工具）
一个生命周期节点（`span_kind=lifecycle`）下可挂 **agent 多轮**（`span_kind=agent_turn`）与 **多工具调用**（`span_kind=tool_call`），还原「一条消息 → AI 多轮 → 多工具」的完整调用树：
- `parent_node`：父节点名（`lifecycle` 节点为空；`agent_turn`/`tool_call` 均指向 `ai_dispatch`）。
- `span_kind`：`lifecycle` | `agent_turn` | `tool_call`。
- `turn_index`：agent 轮次序号（`tool_call` 继承其所属轮次，便于归并到对应 `agent_turn`）。
- `tool_name` / `agent_id`：工具名 / 智能体 ID（仅 `tool_call` / `agent_turn` 填充）。
- 排序键：`node_order ASC, turn_index ASC, id ASC` —— 保证子 span 自然排在所属 `ai_dispatch` 之后、下一个生命周期节点之前，UI 据此组装树。

## 4. 埋点边界（6 节点，最小稳定）

| 节点 | 代码位置 | 入参 / 出参 / 预期（要点） |
|---|---|---|
| `ingest` | `service/inbox_ingress.go` `persistMessage`（inbound 分支） | 入参：channel/account_id/conv_id/sender_id/content_len；出参：msg_id/status；预期：落库 + 收件箱同步 |
| `ai_dispatch` | `service/webhook.go` `runAIGeneration`（编排器调用前后计时） | 入参：channel/conv_id/content_len；出参：ai_replied/transferred/handler_type/reply_len；预期：生成回复并决策；异常：编排器失败 |
| `agent_turn` | `aiagent/agent/tooluse` 观察者（自动，零侵入） | 每次 LLM 推理轮次一条；入参：本轮 tool_calls 数；出参：耗时；`turn_index` 自增 |
| `outbound_enqueue` | `webhook.go` `sendOutbound`（AI 回复主路径）；`inbox_ingress.go` `DeliverOutbound`（人工代发） | 入参：channel/conv_id/content_len；出参：msg_id/status(pending\|failed)；预期：落库 pending；异常：目标不可达→failed |
| `inbox_sync` | `inbox_ingress.go` `persistMessage`（inbound 分支，事务内原子） | 入参：conv_id；出参：synced；预期：inbox_conversations 建立/更新（避免 sync_gap） |
| `downlink_fetch` | `bridge/handler_http.go` `GetBridgeOutbox`（两查询分支） | 入参：channel/account_id/msg_id；出参：fetched；**按 msg_id 去重，每消息仅记一次** |
| `delivered_ack` | `inbox_ingress.go` `AckOutboundDelivered` | 入参：channel/account_id/msg_id；出参：status=delivered；预期：pending→delivered |

> 统一收口：`persistMessage` 覆盖 inbound 全链路（ingest+inbox_sync）与人工/自言 outbound；`sendOutbound` 覆盖 AI 回复出站；`runAIGeneration` 覆盖 AI 处理；`GetBridgeOutbox` 覆盖下行出库；`AckOutboundDelivered` 覆盖送达。不触碰异步 AI 调度内部，避免重排管线。

### 4.1 零侵入的 agent 多轮 / 多工具自动采集（观察者模式）
`agent_turn` 与 `tool_call` 子 span **不手写埋点**，而是由 `aiagent/agent/tooluse` 包暴露的可选 observer 钩子 `ToolTraceSink func(ctx, tracing.ToolTraceEvent)` 自动产生：
- 工具执行引擎在 `Execute`（单次工具调用）与 `DispatchByLLMToolCall`（每轮 LLM 工具编排）的出口调用 `ToolTraceSink`，把 `tool_call` / `agent_turn` 事件抛出。
- `tracing.ReportToolCall` 是钩子实现：把事件转为层级 `message_trace` 行（自动继承 context 中的 `Carrier` 渠道/账号/会话维度）。
- 接线在 `router` 启动处：`tooluse.ToolTraceSink = tracing.ReportToolCall`（一次，基础设施层），业务代码与工具调用点均无需改动。
- 轮次序号由 `tooluse` 基于 `trace_id` 自增计数器分配（`WithTurnIndex`/`GetTurnIndex`），`tool_call` 继承其所属轮次，UI 据此归并到对应 `agent_turn`。
- **依赖方向**：`tooluse → tracing`（单向，工具引擎依赖可观测性包），`tracing` **不反向依赖** `tooluse`（避免循环依赖），接线放置于同时导入两者的 `router` 层。

## 5. 监控聚合（`internal/monitor`，只读）

- **`HealthOverview`** 业务链路健康概览：近 1h 入/出站速率、pending 深度与最旧年龄、delivered/failed、同步缺口 `sync_gap_count`、卡住（可达 FAIL / 不可达 WARN 拆分）、异常节点数。
- **`NodeHealthByChannel`**（多渠道 × 节点）：每个 (channel, node) 的 总量 / 异常数 / **异常率** / **平均响应时间** / **P95 响应时间** / 最近异常时间。**直接回答"每渠道每节点的耗时与异常"**。
- **`LifecycleLatencyByChannel`**：按渠道统计端到端时延（ingest → delivered_ack）的 P50 / P95。
- **`Lifecycle`**：还原某会话 / 某轮的完整节点链，输出每个节点的 入参/出参/响应时间/**节点间时延**/预期/异常。
- **`TraceTree`**：任意一条消息 / 单轮对话的**完整链路明细**（生命周期节点 + agent 多轮 + 多工具调用，含入参/出参/耗时/预期/异常）。查询优先级：`trace_id` > `msg_id`（反查所属 trace）> `conversation_id`（取最近一轮）。
- **`Anomalies`**：业务链路异常（sync_gap / stuck_pending / unreachable / node_abnormal）。
- **`Traces`** / **`PurgeOld`**：轮次汇总 + 7 天归档。

## 6. 暴露方式（沿用 bridge 私域鉴权模型）

`/api/monitor/{health,anomalies,node-health,latency,lifecycle,traces,trace-tree}` 挂在 `InitGuard` 组（与 bridge ingest 同模型，**无需前端 JWT**，私域单租户部署）。所有端点返回 `{code:"SUCCESS", data:...}` 信封（与 user-server 全站一致）。
- `node-health` / `latency`：按渠道节点健康 + 端到端时延 JSON。
- `lifecycle?conversation_id=` / `?trace_id=`：单会话/单轮节点链（核心视图）。
- `trace-tree?trace_id=` / `?msg_id=` / `?conversation_id=`：完整链路明细（含 agent 多轮 / 多工具），前端据此渲染可展开调用树。
- **UI 位置（2026-08-08 迁移）**：追踪 UI 已从 user-server 的 `//go:embed` 自包含 HTML 仪表盘迁移到 **user-web 前端**，即 `user-web/src/views/system/TraceMonitor.vue`（「系统设置 → 链路追踪」菜单，路由 `/system/trace`），调用上述 JSON 接口渲染。user-server 不再内嵌任何 HTML，仅暴露数据接口。新增「消息链路明细」视图：输入 trace_id/msg_id/conversation_id 查看一条消息的完整链路，骨架为 6 生命周期节点，`ai_dispatch` 下展开 agent 多轮、每轮下展开多工具调用，每个 span 可点开看 入参/出参/耗时/预期/异常；异常节点红色高亮；点击链路列表/会话链路可一键下钻。
- `scripts/monitor-trace.sh`：CLI，默认 `127.0.0.1:8204`，支持传 `conversation_id` 打印节点链；已适配信封（`.data.*`）。

## 7. 论证：为何这样设计

- **业务生命周期 > 系统监控**：用户要的是"从消息上报到下行出库"这条核心链路的可追溯，节点级 I/O + 响应时间 + 预期/异常，而非主机指标。模型以「节点 span」承载，天然契合。
- **节点 span > 阶段事件**：相对初版（只有 stage+status），节点 span 额外记录入参/出参/响应时间/预期/异常，使"某节点为什么没得到预期结果"可被直接回答，是真正的可观测性。
- **多渠道维度**：`Channel` 字段 + 按 (channel,node) 聚合，让"某渠道特有的慢/异常"一眼可见，契合本系统多渠道事实源。
- **`trace_id` 复用而非新建**：AI 回复本质是「对上一条客户消息的应答」，复用 inbound 的 trace_id 最贴合业务语义，无需改造异步 AI 管线。
- **best-effort 埋点**：监控故障绝不应拖垮业务。`RecordNode` 吞掉错误、走独立 `message_trace` 表、不锁业务表；`downlink_fetch` 按 msg_id 去重避免轮询膨胀。
- **异步非阻塞 sink（追踪绝不阻塞业务）**：业务侧埋点（`RecordNode` / `Span.End`）仅把 `*model.MessageTrace` 投递进带缓冲 channel（`sinkBuffer=8192`），由独立后台 goroutine 每 300ms 或满 256 条批量 `CreateInBatches` 落库；缓冲满则**主动丢弃并计数**（背压），调用方零等待。进程启动时 `tracing.Init(db)` 启动 worker。
- **context 载体透传 + 流式 Span API（优雅、低侵入）**：`tracing.Carrier` 随 `context.Context` 透传会话/渠道/账号维度，`tracing.Start(ctx, node).Input(...).Expected(...).End(output, err)` 一行即一个节点埋点，耗时自动统计、异常自动标记，业务代码无需拼 `NodeSpan`、无需关心落库细节。
- **观察者模式解耦（利用 Go 装饰器/钩子特性，减少入侵耦合）**：agent 多轮/多工具调用经 `tooluse` 包既有装饰器链统一出口，仅抛出一个 `ToolTraceSink` 事件，由 `tracing` 在 router 层接线消费。工具引擎与业务代码**均不手写追踪埋点**，`tracing` 与 `tooluse` 保持单向依赖、无循环引用。
- **复用 `InitGuard` 鉴权**：与 bridge 端点一致，契合项目「私域单租户」既定约束，不引入新鉴权面。
- **聚合只读 + 索引 + 归档**：异常/健康检测全部 WHERE+聚合，毫秒级可高频轮询；`PurgeOld` 防表无限增长。

## 8. 已验证成效

- 全部新端点返回真实聚合数据；迁移自动建表 + 加列成功；单测（连真实库）覆盖 `LinkOutboundTraceID` 复用、`RecordNode` 写入、`Lifecycle`/`NodeHealthByChannel`/`HealthOverview`/`Traces`/`PurgeOld`。
- **自动发现 `sync_gap` 同步缺口**（部署首日 3→4），含活跃会话 `63bd5238...`（884899 会话）与 `69c62722...`——此前只能手工 SQL 才发现。
- `downlink_fetch` 去重修复：原来卡住 pending 被反复轮询、每次写一条节点（实测膨胀到 247 条）；改为每 msg_id 仅记一次后稳定。
- 真实业务路径已命中：`63bd5238...` 会话已有 `downlink_fetch` 节点，证明埋点接入真实链路。

## 9. 后续可扩展

- 告警推送（Webhook/钉钉）接入 `anomalies` 与 `NodeHealthByChannel` 异常率阈值。
- 对 `sync_gap` 触发自动补同步（`UpsertFromHubMessage`）的修复任务，闭环数据完整性缺口。
- `ai_dispatch` 耗时已支持细分：现已自动采集 `agent_turn`（每轮）与 `tool_call`（每工具）子 span，UI 可逐层下钻；可进一步在 `tool_call` 内按阶段（参数校验 / 执行 / 重试）细分耗时。
- `trace-tree` 可叠加时间泳道图 / 火焰图，直观展示 agent 多轮与多工具并行的耗时分布。
