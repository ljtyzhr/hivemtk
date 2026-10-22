# Bug Fix: SSE 推送回调带上租户上下文，出站认领不再被拒

- **Slug**: bridge-sse-publisher-tenant-scope
- **Fixed**: 2026-10-22T20:50:00+08:00
- **Assessment**: ./assessment.md
- **Status**: applied

## Summary

`GlobalSSEPublisher` 回调签名加上 `ctx` 作为第一个参数，4 个调用点在起 goroutine 前用 `tenant.BackgroundWithTenant(ctx)` 派生（保留租户归属、丢弃请求的取消信号），bus 侧改用 `PublishCtx`。修完后 SSE 出站认领（UPDATE `message_hub`，租户表）不再被隔离守卫 fail-closed 拒绝，消息当场被认领并推送，不再靠 30s 超时重投兜底。

## Changes

| File | Change | Notes |
|------|--------|-------|
| `user-server/internal/service/bridge_outbound.go` | modified | `GlobalSSEPublisher` / `SetGlobalSSEPublisher` 签名加 `ctx context.Context`（首位）；调用点在起 goroutine 前 `pubCtx := tenant.BackgroundWithTenant(ctx)` 并传入 |
| `user-server/internal/service/inbox_ingress_outbound.go` | modified | 同上（`DeliverOutbound` 调用点） |
| `user-server/internal/service/inbox_ingress_persist.go` | modified | 同上（`persistHistoryMessage` 调用点） |
| `user-server/internal/service/webhook_outbound.go` | modified | 同上（`webhook_outbound` 调用点） |
| `user-server/internal/router/router.go` | modified | 注入实现由 `GlobalSSEBus.Publish(...)` 改为 `GlobalSSEBus.PublishCtx(ctx, ...)` |
| `user-server/internal/service/bridge_sse_publisher_tenant_test.go` | added test | 回归锁：回调收到的 ctx 已绑定租户；无租户时写库即被拒（不得回落默认租户） |

## Diff Highlights

```go
// 签名：租户从接口层就带上
var GlobalSSEPublisher func(ctx context.Context, channel, accountID string, hubID uint64, ...)

// 调用点：派生带租户的 ctx（保留归属、丢弃请求取消信号）
pubCtx := tenant.BackgroundWithTenant(ctx)
go func() {
    ...
    GlobalSSEPublisher(pubCtx, channel, accountID, uint64(h.ID), ...)
}()

// router 注入：走带 ctx 的 Publish
bridge.GlobalSSEBus.PublishCtx(ctx, bridge.BuildOutboundSSEEvent(...))
```

## Local Verification

```bash
export POSTGRES_TEST_HOST=127.0.0.1 POSTGRES_TEST_PORT=8202 \
       POSTGRES_TEST_PASSWORD="$(docker inspect mtk-postgres --format '{{range .Config.Env}}{{println .}}{{end}}' | grep '^POSTGRES_PASSWORD=' | cut -d= -f2- | head -1)"
```

| 检查 | 命令 | 结果 |
|------|------|------|
| 构建 | `cd user-server && go build ./...` | ✅ exit 0 |
| 新增回归锁 | `go test ./internal/service/ -run 'TestSSEScopedPublisher' -v` | ✅ 2 个用例 PASS |
| 相关回归 | `go test ./internal/service/ -run 'TestDeliverBridgeOutbound\|TestSSEScoped\|TestOfflineReplay\|TestLeadMining' -v` | ✅ **21 个用例全 PASS** |
| 回归有效性（去修复须红） | `git stash push -- bridge_outbound.go inbox_ingress_outbound.go inbox_ingress_persist.go webhook_outbound.go router.go` 后重跑 | ✅ **编译期即红**（回调签名不带 ctx，新测试传不进去）——比运行时红更强：接口形状本身现在强制租户传播 |
| Lint | `golangci-lint run ./internal/service/... ./internal/router/...` | ✅ **0 issues** |
| Vet | `go vet ./internal/service/... ./internal/router/...` | ✅ exit 0 |
| 线上部署 | 交叉编译 → 上传（旧二进制留档 `user-server.prev-sse-*`）→ `docker compose build user-server` → 重建容器 | ✅ 7 秒就绪，`/health` 全绿 |
| 线上行为 | 观察 SSE 连接与认领 | ✅ 扩展的 SSE 真实连上（douyin `last_event_id: 214`、xiaohongshu 同时在线）；**「出站认领报错」0 条**；**租户上下文报错 0 条**；出站行当场进 `inflight`（修复前是认领失败、靠超时重投） |

## Deviations from Assessment

无路径偏离。两处实现细节补充（assessment 未细化到这一层）：

1. **第二个回归用例的断言比 assessment 更强** —— 最初写成"回调收到未绑定的 ctx 时应报错"，实测发现 on 模式下**写库那一步就已经被拒**（`message_hub` create 需要租户上下文），回调根本不会被触发。已把断言改成这个更强的性质：**无租户 ⇒ 写库被拒 ⇒ 回调不触发 ⇒ 宁可不发也不落到默认租户**。

2. **本修复上线后仍偶发同一类报错，追出第二个根因并当轮修掉**（规则 2）：
   上一版修的是"回调签名带 ctx + 调用点派生租户 ctx"，但**平台级后台任务那条路仍是无租户**：
   延迟出站派发 `dispatchDueDelayedOutbound` 拿的是 `tenant.WithSystemScope(ctx)`，
   而 `tenant.BackgroundWithTenant` **只搬租户、不搬 SystemScope** ⇒ 派生出的 ctx 变成
   "无租户、无豁免" ⇒ 隔离 callback 一律 fail-closed 拒绝 ⇒ 认领（UPDATE `message_hub`）失败。
   修法（提交 `3a30bfe4`）：`BackgroundWithTenant` 在父 ctx 带 SystemScope 时，派生 ctx 也带。
   **丢豁免比不丢更坏**——调用方以为豁免还在，写库全被拒。
   回归锁：`user-server/internal/pkg/tenant/tenant_test.go` 的
   `TestBackgroundWithTenantPreservesSystemScope`（三种父 ctx：租户+豁免 / 仅豁免 / 普通）。

## 线上验证（最终）

```
需要租户上下文        0
出站认领报错           0
zh_rag                0
all providers FAILED  0
渠道快照读取失败        0
lead-mining 读配置失败  0
--- 正向读数
SSE 客户端连接         4
AI 回复生成(outbound)  5（10 分钟内）
延迟出站派发(H-3)      19（SystemScope 那条路正常跑）
```

## Risks

- 回调签名变更：仓内调用点只有 5 处（4 个 service 侧 + 1 个 router 注入），已全部同步；无其他消费者。
- 异步推送 ctx 由 `context.Background()` 派生（保留租户、丢弃取消），与本仓既有异步写库口径一致。
- 无 API 契约变更、无迁移、无前端改动。
- 行为变化：出站行从"认领失败 → 30s 超时重投"变为"当场认领 → 推送"，延迟减少一轮认领租约。

## Follow-up

已在线上验证（见上表最后两行）。本 bug 与前两个（`bridge-offline-replay-cron-tenant-scope`、`rag-zh-rag-config-missing-error`）同属"SaaS 下租户上下文缺失"这一类，建议后续把这一类做成一条常驻巡检（扫描仓内所有"写租户表却拿不到租户 ctx"的路径），避免第四处。
