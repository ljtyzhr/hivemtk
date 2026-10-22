# Bug Assessment: 后台桥接回扫/线索发掘任务缺租户上下文，SaaS 模式下整轮静默空转

- **Slug**: bridge-offline-replay-cron-tenant-scope
- **Created**: 2026-10-22T19:30:00+08:00
- **Source**: 线上 user-server 日志（容器 `mtk-user-server`，101.35.252.112）
- **Verdict**: valid
- **Severity**: high

## Report (verbatim or summarized)

线上部署（`TENANT_MODE=on`，`SITE_TENANT_CODE=tz3rmdj`）的用户端容器，每 5 分钟稳定打出同一组告警（修复 CORS 之前）：

```
2026/10/22 18:55:00 hivemtk-user/internal/repository/bridge_offline_replay_repo.go:60
  [tenant] 表 bridge_accounts 需要租户上下文但 ctx 未绑定（query）；多租户模式下拒绝执行，请经中间件/TenantRunner/行自证提供租户
{"level":"warn","message":"[BridgeReplay] 渠道快照读取失败: [tenant] 表 bridge_accounts 需要租户上下文但 ctx 未绑定（query）；多租户模式下拒绝执行…"}
2026/10/22 18:55:00 hivemtk-user/internal/repository/bridge_offline_replay_repo.go:200
  [tenant] 表 message_hub 需要租户上下文但 ctx 未绑定（query）；多租户模式下拒绝执行…
```

同一时刻，桥接入站路径也稳定打出：

```
{"level":"warn","message":"[lead-mining] 读取配置失败: [tenant] 表 lead_mining_configs 需要租户上下文但 ctx 未绑定（query）；多租户模式下拒绝执行…"}
```

## Symptom

SaaS 多租户部署下，两个后台任务整轮被租户隔离守卫拒绝：
1. **离线消息回扫**（`bridge_offline_replay`，每 5 分钟）——`bridge_accounts` / `message_hub` 两张租户表的跨租户快照查询全部被拒 ⇒ 渠道在线/离线快照恒为空 ⇒ **延后出站永远不补投、孤儿出站永远不结算**。
2. **线索发掘**（`lead_mining`）——worker 读租户表 `lead_mining_configs` 被拒 ⇒ 配置恒读不到 ⇒ **线索发掘整条链路静默不跑**（无异常、无产出、只有一行 warn）。

## Reproduction

1. 线上 user-server 以 `TENANT_MODE=on` 启动（`hivemtk/user-server/.env`：`TENANT_MODE=on`）。
2. 观察容器日志 `docker logs mtk-user-server`：
   - 每 5 分钟一条 `[BridgeReplay] 渠道快照读取失败: [tenant] 表 bridge_accounts 需要租户上下文但 ctx 未绑定`；
   - 每条桥接入站消息一条 `[lead-mining] 读取配置失败: [tenant] 表 lead_mining_configs 需要租户上下文但 ctx 未绑定`。
3. 现状（2026-10-22 实测）：`bridge_accounts` / `message_hub` 两张表在 SaaS 租户下均无数据回流；`lead_mining_configs` 的读取恒失败。
4. off 模式（`TENANT_MODE=off`）不复现——隔离 callback 直通。

## Suspected Code Paths

- `user-server/internal/pkg/cron/cron.go:108-110` — `bridge_offline_replay` 任务用 cron 交来的裸 ctx 直接调 `RunOnce(ctx)`；同文件 `handoff_chain`（:116-125）、`churn`（:151+）、活码轮询（:182）、过期令牌清理（:194）都已走 `fanoutTask`，唯独这条没有。
- `user-server/internal/pkg/cron/cron.go:44-48` — `goTask` 用 `utils.SafeGo(context.Background(), …)` 起任务，ctx 天然不含租户。
- `user-server/internal/service/bridge_offline_replay.go:150-160`（`detectBridgeChannels`）、`:231-297`（`RunOnce`）— 跨租户快照查询 + 每租户补投 + 孤儿结算，全部依赖 ctx 带租户。
- `user-server/internal/repository/bridge_offline_replay_repo.go:51-62`（`ListBridgeAccounts`）、`:~200`（`message_hub` 查询）— 隔离 callback 在此 fail-closed。
- `user-server/internal/service/lead_mining.go:70` — `NewLeadMiningService` 用 `context.WithCancel(context.Background())` 起 worker，**ctx 不含租户**。
- `user-server/internal/service/lead_mining.go:187-204`（`worker`）、`:206-251`（`process`）、`:253-272`（`loadConfig`）— worker 把同一个无租户 ctx 传给 `process`；队列只带 `*model.MessageHub`，行自带的 `TenantID` 没有被用来重新绑定 ctx。
- `user-server/internal/pkg/tenant/fanout.go:44-84` — `Fanout` 逐活跃租户回调并 `WithTenantID(ctx, id)`，是本仓后台任务的标准口径。

## Root Cause Hypothesis

**置信度：高。**

两个后台任务都拿到了"不带租户"的 ctx，而它们要访问的表（`bridge_accounts` / `message_hub` / `lead_mining_configs`）都带 `tenant_id` 列 ⇒ 被 `internal/pkg/tenancyidx` 的隔离 callback 判为 fail-closed（`[tenant] 表 X 需要租户上下文但 ctx 未绑定（query）`）。

- 回扫任务：cron 交来的 ctx 来自 `context.Background()`（`cron.go:46`），同文件其余租户表任务都已用 `fanoutTask` 补上租户，只有这条漏了。**这不是设计上的"跨租户扫描"**——该任务后续的写操作（补投 `reach_delayed_outbound`、孤儿结算 `message_hub`）本来就是逐租户语义，用 `SystemScope` 豁免反而会让写操作失去租户边界。正确口径是 `fanoutTask` 逐租户跑。
- 线索发掘：`MessageHub` 行自带 `TenantID`（`internal/model/ai_sales_champion.go:13`），但 `process()` 沿用的是 worker 启动时的无租户 ctx，导致读配置、读会话历史、写线索全部无租户上下文。本仓既有的做法是 `tenant.OnlyTenant(ctx, row.TenantID)`（见 `config_param.go:520`、`email_tracking.go:144` 等）。

## Proposed Remediation

**Preferred**（两处各按本仓既有口径改，不引入新机制）：

1. **回扫任务改走 `fanoutTask`**（`cron.go`）：把 `service.NewBridgeOfflineReplayService().RunOnce(ctx)` 包进 `fanoutTask(ctx, func(ctx) error { …; return nil })`，与同文件 `handoff_chain`（:116-125）的写法逐字对齐。逐租户跑之后：
   - 渠道快照查询在该租户 ctx 下只看到本租户的账号行（隔离 callback 正常放行）；
   - 补投与孤儿结算的写操作落在本租户，边界不丢。
   - 统计口径随之变成"逐租户累加"，需在 `ReplayStats` 的汇总处明确跨租户求和方式（与 `churn` 的 `tenant.Fanout` 计数口径一致）。
2. **线索发掘 worker 按行自证绑定租户**（`lead_mining.go`）：`process(ctx, hub)` 进入后用 `tenant.OnlyTenant(ctx, hub.TenantID)` 重新绑定（`hub.TenantID` 为 0 时保持原 ctx，由隔离 callback 继续 fail-closed，不回落默认租户）。

**Alternatives**:
- 回扫任务改用 `tenant.WithSystemScope`：可让跨租户快照一次读完，但后续写操作会失去租户边界（写隔离被整体豁免），**不可取**。
- 线索发掘改成每租户一个 worker：改动面大、收益相同，**不取**。

**Files likely to change**:
- `user-server/internal/pkg/cron/cron.go`
- `user-server/internal/service/bridge_offline_replay.go`（统计汇总口径，如需）
- `user-server/internal/service/lead_mining.go`

**Tests to add or update**:
- `user-server/internal/pkg/cron/cron_test.go`（或就近包）：断言 `bridge_offline_replay` 任务在 on 模式下经 `fanoutTask` 执行 —— 注入租户枚举器 + 假仓储，验证每租户各跑一次、单租户失败不阻断其余租户。
- `user-server/internal/service/lead_mining_test.go`：断言 `process()` 在 on 模式下用 `hub.TenantID` 绑定 ctx —— 断言读配置时 ctx 携带该租户（可用一个"记录 ctx 租户"的假仓储）。
- 回归：既有 `bridge_offline_replay_test.go` / `bridge_offline_orphan_test.go` / `lead_mining` 相关用例保持绿。

## Risks & Considerations

- **回扫任务行为变化**：改为逐租户后，单租户失败不再中断其余租户（`Fanout` 语义），日志形态从"一条跨租户快照失败"变为"逐租户"，运维看板读数口径需同步（`scanned/online/offline` 变为各租户之和）。
- **off 模式行为不变**：`Fanout` 在 off 模式退化为单次回调 + 绑默认租户（`fanout.go:48-50`），与历史行为逐字节等价。
- **无 API 契约变更**，无迁移，无前端改动。
- 线索发掘的 `process` 每次多一次 ctx 包装，队列吞吐无影响（每条消息一次）。

## Open Questions

- [无阻塞项]。回扫任务的统计口径改为跨租户求和后，`ReplayStats` 是否需要新增"按租户"的明细字段（便于排障）——本次按"求和 + 逐租户日志"实现，明细字段留作后续。
