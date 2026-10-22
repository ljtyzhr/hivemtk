# Bug Assessment: SSE 推送回调不带租户上下文，出站认领被隔离守卫拒绝

- **Slug**: bridge-sse-publisher-tenant-scope
- **Created**: 2026-10-22T20:45:00+08:00
- **Source**: 线上 user-server 日志（容器 `mtk-user-server`，101.35.252.112）
- **Verdict**: valid
- **Severity**: high

## Report (verbatim or summarized)

线上部署（`TENANT_MODE=on`）的用户端容器，桥接回复就绪后稳定打出：

```
{"level":"error","error":"[tenant] 表 message_hub 需要租户上下文但 ctx 未绑定（update）；多租户模式下拒绝执行，请经中间件/TenantRunner/行自证提供租户",
 "hub_id":150,"channel":"douyin","time":"2026-10-22T20:32:44+08:00",
 "message":"[SSE] 出站认领报错，放弃本次推送（行仍待办，超时后重投）"}
```

## Symptom

SSE 下行推送前的"出站认领"每条都失败：服务端把行置 `inflight` 的那步 UPDATE 被租户隔离守卫 fail-closed 拒绝，于是**放弃本次推送**，改由 30s 认领租约到期后重投兜底。现象是"SSE 像是连着、消息像是推了，但每条都靠超时重投"，且每次多一条 ERROR 日志（每条回复一行）。

## Reproduction

1. 线上以 `TENANT_MODE=on` 部署，扩展装好并连上 SSE（`docker logs` 可见 `[SSE] client connected`）。
2. 任何一条 AI 回复产生 ⇒ `message_hub` 出现 `outbound` 行 ⇒ 总线 `Publish` ⇒ 认领那步报上面的 ERROR。
3. off 模式不复现（隔离 callback 直通）。

## Suspected Code Paths

- `user-server/internal/service/bridge_outbound.go:55` — `GlobalSSEPublisher` 的回调签名**没有 ctx**：
  `func(channel, accountID string, hubID uint64, convID, msgType, receiverID, content string, isAIReply bool, createdAt time.Time)`。
- `user-server/internal/service/bridge_outbound.go:126-144`、`user-server/internal/service/inbox_ingress_outbound.go:51-69`、`user-server/internal/service/inbox_ingress_persist.go:380-403`、`user-server/internal/service/webhook_outbound.go:879-895` — 4 个调用点都是 `go func(){ … GlobalSSEPublisher(…) }()`：**请求 ctx 在起 goroutine 时被整个丢掉**（goroutine 里的 ctx 还会随请求结束被取消）。
- `user-server/internal/bridge/sse.go:406-408` — `SSEBus.Publish(event)` 内部是 `b.PublishCtx(context.Background(), event)` ⇒ 认领那步拿到的是**无租户的 Background**。
- `user-server/internal/bridge/sse.go:383-388` — `claimer.ClaimOutboundForPush(ctx, hubID, timeout)` → `UPDATE message_hub`（租户表）⇒ 被隔离守卫 fail-closed 拒绝 ⇒ 记上面那条 ERROR 并放弃推送。
- `user-server/internal/router/router.go:658-674` — `SetGlobalSSEPublisher(...)` 的实现调的是 `bridge.GlobalSSEBus.Publish(...)`（无 ctx 那个版本）。
- `user-server/internal/pkg/tenant/tenant.go:149-155` — `tenant.BackgroundWithTenant(ctx)`：保留租户归属、丢掉父 ctx 的取消/截止，是本仓异步写库保持租户归属的既有做法（`account_domain.go:56`、`customer_session.go:450`、`customer_service_plus.go:514`）。

## Root Cause Hypothesis

**置信度：高。**

与本轮已修的两处（`bridge_offline_replay` 回扫 cron、线索发掘 worker）**同源**：SaaS 模式下凡是要写租户表的路径都必须带租户上下文，隔离守卫在 on 模式下一律 fail-closed 拒绝（绝不回落默认租户）。

这一处的特殊之处在于**租户在接口签名层就丢了**：`GlobalSSEPublisher` 是 service 包与 bridge 包之间的全局回调（service 不能 import bridge，避免循环依赖），签名里没有 ctx，调用点又在 `go func()` 里把请求 ctx 丢掉 ⇒ 从"业务 ctx"到"认领 SQL"这条链路上，**从来就没有过租户信息**。

后果比前两处更隐蔽：认领失败**不会丢消息**（行留在待办集合、30s 租约到期后重投），所以业务无感，只是"每条都慢一轮" + "每次一条 ERROR"。这解释了为什么它在别的问题被修好之后才浮出来——SSE 之前根本没有真实订阅者（账号未注册），认领压根没被调用过。

## Proposed Remediation

**Preferred**（沿用本仓既有口径，不引入新机制）：

1. `GlobalSSEPublisher` 回调签名**加 ctx 作为第一个参数**（`ctx context.Context, ...`），`SetGlobalSSEPublisher` 同步。
2. 4 个调用点在起 goroutine 前用 `tenant.BackgroundWithTenant(ctx)` 派生（保留租户归属、丢掉请求的取消信号——请求早结束、推送还没发出去时不能把推送一起取消），并把派生出的 ctx 传进回调。
3. `router.go` 的注入实现改用 `bridge.GlobalSSEBus.PublishCtx(ctx, …)`（而不是 `Publish`）。

**Alternatives**:
- 在 `claimForPush` 里按 hub 行反查租户再绑定：多一次查询，且与"账号行自证"的口径重复，不取。
- 给 `message_hub` 的认领开 SystemScope 豁免：等于把写隔离整体豁免掉，**不可取**。

**Files likely to change**:
- `user-server/internal/service/bridge_outbound.go`
- `user-server/internal/service/inbox_ingress_outbound.go`
- `user-server/internal/service/inbox_ingress_persist.go`
- `user-server/internal/service/webhook_outbound.go`
- `user-server/internal/router/router.go`

**Tests to add or update**:
- 断言 SSE 回调收到的 ctx **已绑定租户**（`tenant.FromContext` 能读出该租户）。
- 断言无租户的 ctx **不得被绑到默认租户**（on 模式下写库即被拒 ⇒ 回调根本不被触发）。
- 回归：`DeliverBridgeOutbound` / `offline replay` / `lead mining` 既有用例保持绿。

## Risks & Considerations

- **回调签名变更**：`GlobalSSEPublisher` 是包级全局函数变量，仓内调用点只有这 5 处（含 router 注入），已全部同步；无外部插件依赖该签名。
- 异步推送的 ctx 由 `context.Background()` 派生（保留租户、丢弃取消）：与本仓既有异步写库口径一致（`account_domain.go:56`）。
- 无 API 契约变更、无迁移、无前端改动。
- 行为变化：认领从"必然失败 → 靠超时重投"变成"当场认领并推送"，出站行的 `inflight → delivered` 链路变快 30s（认领租约）。

## Open Questions

- [无阻塞项]。
