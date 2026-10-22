# Bug Verification: SSE 推送回调带上租户上下文

- **Slug**: bridge-sse-publisher-tenant-scope
- **Tested**: 2026-10-22T20:52:00+08:00
- **Assessment**: ./assessment.md
- **Fix**: ./fix.md
- **Result**: verified

## Summary

线上症状已消失：修复前每条 SSE 推送都打 `[SSE] 出站认领报错，放弃本次推送（行仍待办，超时后重投）`，修复后该类日志 0 条，出站行**当场**进 `inflight`（即被认领并推送），不再靠 30s 认领租约超时重投。扩展的 SSE 连接在修复后真实建立（douyin 与 xiaohongshu 两路都在线，douyin 从 `last_event_id: 214` 续传），说明这正是此前被"认领失败 ⇒ 放弃推送"掩盖掉的那条生产下行路径。

## Checks Performed

| Check | Command / Action | Result | Notes |
|-------|------------------|--------|-------|
| Reproduction (post-fix) | 线上 `docker logs` 观察 `[SSE] client connected` 与认领日志 | pass | 修复前每条回复一行 `出站认领报错`；修复后 **0 条** |
| Reproduction (post-fix) | 线上观察出站行状态 | pass | `id=237 outbound status=inflight` —— 当场被 SSE 认领（修复前是认领失败、行留在 pending 等超时重投） |
| Reproduction (post-fix) | 线上 `grep -c "需要租户上下文"` | pass | **0**（修复前：每条 SSE 推送 1 条） |
| New / updated tests | `go test ./internal/service/ -run 'TestSSEScopedPublisher' -v` | pass | 2 个用例 PASS |
| Regression suite | `go test ./internal/service/ -run 'TestDeliverBridgeOutbound\|TestSSEScoped\|TestOfflineReplay\|TestLeadMining' -v` | pass | 21 个用例全 PASS |
| 回归有效性（去修复须红） | `git stash push -- bridge_outbound.go inbox_ingress_outbound.go inbox_ingress_persist.go webhook_outbound.go router.go` 后重跑 | pass | **编译期即红**（`cannot use func(ctx context.Context, …) as func(channel string, …)`）——接口形状本身现在强制租户传播 |
| Lint / type-check | `golangci-lint run ./internal/service/... ./internal/router/...` / `go vet` | pass | 0 issues / exit 0 |
| 线上部署 | 交叉编译 → 上传（旧二进制留档 `user-server.prev-sse-*`）→ `docker compose build user-server` → 重建容器 | pass | 7 秒就绪，`/health` 全绿 |

## Output Excerpts

线上 SSE 连接（修复后，扩展真实在线）：

```
[SSE] client connected   channel=douyin account_id=MS4wLjAB… last_event_id=214
[SSE] subscribed to SSEBus  channel=douyin
[SSE] client connected   channel=xiaohongshu account_id=69c62722000000003203b0fe
[SSE] subscribed to SSEBus  channel=xiaohongshu
```

线上核对：

```
[SSE] 出站认领报错 条数：0
租户上下文报错 条数：0
outbound 行状态：inflight（当场认领，非超时重投）
```

## Residual Risks

- **`internal/service` 全包测试超时** —— 同另两个 bug 的 test.md：已用 stash 对照证明与本轮无关（原始树同样超时、两次挂的用例不同），属工作区在途的他人 lane 改动。
- 本次验证的 SSE 客户端是**用户自己的扩展**（真实 IP 171.222.186.162），验证覆盖的是"服务端认领 + 推送"这一段；扩展端是否把推送内容正确落到输入框、是否回执 ack，不在本次改动范围内（回执链路 `POST /api/bridge/outbox/ack` 走的是另一条已鉴权、已带租户的路径）。
- 线上 30 分钟窗口内未出现新的 `zh_rag` / LLM 全失败 / 回扫失败，三类读数均为 0。

## Recommendation

本 bug 已闭环。三处"SaaS 下租户上下文缺失"（回扫 cron、线索发掘 worker、SSE 推送回调）已全部修完并各自带回归锁。
**建议下一步做一条常驻巡检**：扫描仓内所有"写租户表却拿不到租户 ctx"的路径（`tenancyidx` 的 fail-closed 报错文案是现成的判据），把这一类一次性扫干净，避免第四处。
