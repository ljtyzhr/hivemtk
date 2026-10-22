# Bug Verification: 后台桥接回扫/线索发掘任务补齐租户上下文

- **Slug**: bridge-offline-replay-cron-tenant-scope
- **Tested**: 2026-10-22T20:20:00+08:00
- **Assessment**: ./assessment.md
- **Fix**: ./fix.md
- **Result**: verified

## Summary

线上症状已消失：修复前每 5 分钟一条 `[BridgeReplay] 渠道快照读取失败: [tenant] 表 bridge_accounts 需要租户上下文但 ctx 未绑定`，修复后同一时刻改为 `[BridgeReplay] 回扫完成: scanned=…`（任务真的跑起来了），且 tenant 18 那一轮读到 `scanned=2 online=1 offline=1`（本租户的两个账号行）；线索发掘读配置也从「被守卫拒绝」变为带租户过滤的正常查询。新增的 4 条回归锁在去掉修复后如期变红。

## Checks Performed

| Check | Command / Action | Result | Notes |
|-------|------------------|--------|-------|
| Reproduction (post-fix) | 线上 `docker logs mtk-user-server` 观察 20:15 那轮回扫 | pass | 修复前：`[BridgeReplay] 渠道快照读取失败: [tenant] 表 bridge_accounts 需要租户上下文但 ctx 未绑定`；修复后：`[BridgeReplay] 回扫完成: scanned=2 online=1 offline=1`（tenant 18 那一轮） |
| Reproduction (post-fix) | 线上探针 ingest 一条消息后观察 lead-mining 查询 | pass | 查询已带租户过滤：`SELECT * FROM "lead_mining_configs" WHERE id = 1 AND "lead_mining_configs"."tenant_id" = 18`（修复前是守卫直接拒绝） |
| New / updated tests | `go test ./internal/pkg/cron/ -run TestBridgeOfflineReplayCron -v` | pass | 2 个用例（逐租户 / off 模式退化） |
| New / updated tests | `go test ./internal/service/ -run TestLeadMiningProcess_ -v` | pass | 2 个用例（按行自证绑定 / 无 tenant_id 不回落） |
| Regression suite | `go test ./internal/pkg/cron/... -timeout 300s` | pass | `ok hivemtk-user/internal/pkg/cron 15.348s` |
| Regression suite | `go test ./internal/service/ -run 'TestOfflineReplay\|TestBridgeOffline\|TestLeadMining\|TestPartitionBridge\|TestMergeTags\|TestChannelEnabled' -timeout 300s -v` | pass | 17 个用例全 PASS |
| 回归有效性（去修复须红） | `git stash push -- lead_mining.go` 后重跑 `TestLeadMiningProcess_` | pass | 如期红：`读配置时 ctx 未绑定租户（隔离守卫会 fail-closed 拒绝，线索发掘整条链路静默不跑）` |
| Lint / type-check | `cd user-server && go build ./...` | pass | exit 0 |
| 线上部署 | 交叉编译 → 上传 `/www/wwwroot/hivemtk/user-server/user-server`（旧二进制留档 `user-server.prev-*`）→ `docker compose build user-server` → `docker compose up -d --force-recreate user-server` | pass | 7 秒就绪，`/health` 四项依赖全绿（database/redis ok、inference/embedding up） |
| 线上零残留 | `docker logs mtk-user-server --since 20m | grep -c "需要租户上下文"` | pass | **0**（修复前：每 5 分钟 1 条回扫 + 每条入站 1 条线索发掘） |

## Output Excerpts

线上 20:15 那轮回扫（**逐租户各一轮**，10 轮 = 10 个活跃租户）：

```
[BridgeReplay] 回扫完成: scanned=2 online=1 offline=0 grace=30s unreachable_channels=0 replayed=0 failed=0 duration=1ms   ← tenant 18
[BridgeReplay] 回扫完成: scanned=0 online=0 offline=0 grace=30s unreachable_channels=0 replayed=0 failed=0 duration=1ms   ← 其余 9 个租户（本来就无账号行 ⇒ 真值 0，不是被拒）
```

线上 lead-mining 查询（**已带租户过滤**）：

```
SELECT * FROM "lead_mining_configs" WHERE id = 1 AND "lead_mining_configs"."tenant_id" = 18 ORDER BY ... LIMIT 1
```

线上零残留核对：

```
近 20 分钟「需要租户上下文」出现次数：0
近 20 分钟 zh_rag 报错次数：0
```

## Residual Risks

- **`internal/service` 全包测试在本工作区超时（7 分钟，panic 指向 `TestRecordSOP_Number` / `TestAudience_SegmentSourceDiagnostics`）** —— 已用 stash 对照证明**与本轮改动无关**：把本轮三个文件 stash 掉后跑原始树同样超时，且两次挂的用例不同（用例间相互影响/顺序依赖的偶发挂起，属工作区在途改动）。两个用例单独跑都通过（22.4s / 31.0s）。按 CLAUDE.md §3.5 与规则 2，不为绿灯去改别人的在途文件。
- **统计口径变化**：回扫日志从「一条跨租户快照」变成「逐租户各一条」，`scanned/online/offline` 读数变为各租户之和（已在 `mergeReplayStats` 注释中写明）。运维看板若按旧口径读数需同步。
- 线上验证依赖真实租户数据（tenant 18 有 2 个账号行、1 个在线），覆盖了「有账号」这一侧；「某租户无账号」侧由单测 `TestBridgeOfflineReplayCron_OffModeRunsOnce` 与线上 9 个租户的 `scanned=0` 共同覆盖。

## Recommendation

本 bug 已闭环：线上症状消失、回归锁就位、零残留。后续若要进一步优化「回扫统计按租户出明细」，按新功能车道（规则 4）另立需求，不在本次范围内。
