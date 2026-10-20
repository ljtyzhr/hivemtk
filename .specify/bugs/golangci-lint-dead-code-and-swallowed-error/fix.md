# Fix Report: 4 条 golangci-lint 存量红清零（死代码 / 吞错 / 无效赋值）

- **Slug**: golangci-lint-dead-code-and-swallowed-error
- **Fixed**: 2026-10-21
- **Assessment**: [assessment.md](assessment.md)
- **Status**: applied

## Summary

`make lint`（= `golangci-lint run ./...`）在 HEAD `029ccf7a` 上即红 4 条，4 条全部落在**本轮未触碰的已提交文件**上 ⇒ 属仓内既有护栏债。这 4 条挡住提交前总门禁（CLAUDE.md 规则 1、规则 3.5/4.4），不修则任务一~四的任何一次提交都过不了门禁。

本轮按 `user-server/.golangci.yml` 注释里既定的 R1–R9 判定逐条处置（该注释明确要求「**不做机械 `_ =` 包裹**」——那会把「确实该处理却被丢弃」的错误连同可安全忽略的 Close 一并掩盖）。4 条全部**不改变任何运行时行为**，其中 3 条是死代码/无效赋值的物理删除，1 条是真实吞错的补日志。

## Changes

| 文件 | 改动 | 说明 |
|------|------|------|
| `user-server/internal/middleware/rate_quota.go` | modified | `stop()` 加 `//nolint:unused //// 仅被 *_test.go 引用，生产路径未用`（+1 行注释） |
| `user-server/internal/bridge/sse.go` | modified | 删 `:540` 死赋值 `ctxReq := c.Request.Context()`；`:572` 改 `ctxReq := reqCtx`（-2/+1） |
| `user-server/internal/service/reach_send_pipeline_quiethours.go` | modified | 删死常量 `nextDayFirstSendHour`（-2 行） |
| `user-server/internal/service/config_param_seeds.go` | modified | 注释里对已删标识符 `nextDayFirstSendHour` 的指认改为「同一参数」 |
| `user-server/internal/service/customer_session_cron.go` | modified | 非多租户分支裸调用 `c.trigger(ctx)` → 记日志（+2 行） |

合计 **5 files changed, 6 insertions(+), 8 deletions(-)**。

## Diff Highlights

### 1. `customer_session_cron.go` — 唯一的真吞错（errcheck）

```go
-	c.trigger(ctx)
+	if _, err := c.trigger(ctx); err != nil {
+		logger.Errorf("[session_ttl_cron] 会话 TTL 清扫失败: %v", err)
+	}
```

多租户分支（`:89-93`）的 `tenant.Fanout` 内部已按 R4 逐租户 `logger.Warnf` 出声，但**非多租户分支**这条路径原先把 `(int64, error)` 整个丢掉 ⇒ 一次会话 TTL 清扫失败在日志里**一行痕迹都没有**。按 R4「记日志、不因此改变控制流」处置：不提前 return、不影响下一轮 1h ticker 调度。`logger` 本文件已 import，措辞沿用既有的 `[session_ttl_cron]` 前缀。

### 2. `sse.go` — 死赋值（ineffassign），**租户语义敏感**

```go
-	ctxReq := c.Request.Context()
...
-	ctxReq = reqCtx
+	ctxReq := reqCtx
```

`:540` 的初值在 `:540–:572` 之间从未被读（该区间只有入参校验与 `bindBridgeTenant`），`:572` 的 `ctxReq = reqCtx` 整体覆盖它 ⇒ 死赋值。

**租户风险已核对**：`bindBridgeTenant`（`internal/bridge/tenant_binding.go:102-112`）内部 `rctx := c.Request.Context()` 并返回**派生了租户绑定**的 ctx；`reqCtx` 即该返回值。删掉初值后，`:576 runtimeSSEHeartbeatInterval(ctxReq)`、`:580 runtimeSSEMaxStreamDuration(ctxReq)`、`:600 context.WithTimeout(ctxReq, maxStreamDuration)` 三处拿到的仍是**租户绑定后**的 ctx，与改动前完全一致 —— 不会把租户绑定 ctx 退化成裸 request ctx。`context` 包 import 仍需保留（文件内多处仍在用）。

### 3. `rate_quota.go` — 仅测试引用的停止方法（unused）

`stop()` 的唯一调用方是 `rate_quota_middleware_test.go:22,28`（`resetPathQuotaManagerForTest` 换新管理器前停旧清理协程）。`.golangci.yml` 设 `run.tests: false` ⇒ 测试里的引用不算使用 ⇒ 判 unused。

按**仓内 23 处同形先例**的既定处置加 `//nolint:unused`（例：`internal/app/tool_circuit_breaker_wiring.go:151`、`internal/controller/dashboard_sse.go:243`、`internal/aiagent/rag/retrieval/rrf_fusion.go:87`），措辞与先例逐字同形。**不能直接删**：会破坏测试里真实存在的调用。生产路径确实不调用（进程退出即回收清理协程，不是泄漏）。

### 4. `reach_send_pipeline_quiethours.go` — 死常量（unused）

`nextDayFirstSendHour = quietHoursEndHour` 在静默窗口结束小时参数化（`nextQuietHoursRelease(t, endHour)` 按参数算）之后已无引用 ⇒ 物理删除。全仓唯一提及是 `config_param_seeds.go:890` 的注释，故同步改注释（指认已不存在的标识符 = 事实源漂移）。`config_param_seeds.go:895` 的 Description 描述的是**产品语义**（次日首发小时恒等于结束小时），保持不动。

## Tests Added or Updated

不新增用例。理由：4 条都不是逻辑缺陷，**没有可断言的行为变化**（3 条删死代码、1 条把静默失败变成有日志的失败）⇒ 新增用例会只能断言「代码长这样」，不产生回归保护力。回归手段是既有门 + 既有用例：

- `golangci-lint run ./...` ⇒ **0 issues**（本 slug 的直接验收口径）
- `go build ./...` / `go vet ./...` / `gofmt -l` ⇒ 干净
- `go test ./internal/middleware/... ./internal/bridge/... ./internal/service/...` ⇒ 全绿

## Local Verification

**基线（改前，HEAD `029ccf7a` 形态）**：

```
cd user-server && golangci-lint run ./... ; echo "RC=$?"
```
```
internal/service/customer_session_cron.go:95:11: Error return value of `c.trigger` is not checked (errcheck)
internal/bridge/sse.go:540:2: ineffectual assignment to ctxReq (ineffassign)
internal/middleware/rate_quota.go:240:35: func (*pathQuotaLimiterManager).stop is unused (unused)
internal/service/reach_send_pipeline_quiethours.go:34:2: const nextDayFirstSendHour is unused (unused)
4 issues:
LINT_EXIT=1
```

**改后**：

```
internal/middleware/rate_quota.go:240:35: func (*pathQuotaLimiterManager).stop is unused (unused)   ← 已被 nolint 豁免
0 issues.
LINT_EXIT=0
```

`go build ./...` ⇒ 通过（无输出）。
`gofmt -l` 对 5 个文件 ⇒ 输出为空。

**守卫反向验证**（fixbug 闭环第 4 步）：

```bash
git stash push -- <4 个被改文件>   # 退回 HEAD 形态
cd user-server && golangci-lint run ./...
```
输出**逐字复现**改前那 4 条原始红因（见上「基线」块，含相同的文件:行号与规则名）⇒ 证明这 4 条红确实由本轮改动消除，不是被环境或配置变化掩盖。

`git stash pop` 后对 4 个文件逐个 `diff -q` 与注码前另存的副本比对 ⇒ 全部 `IDENTICAL`（还原无损）。

## Deviations from Assessment

无。5 个文件与 assessment 的「Files likely to change」完全一致，未超范围。

## Follow-ups

- 本 slug 只解 4 条 lint 红。**另一个假绿门不在本 slug 范围**：`scripts/check-architecture.sh` 的「上下文透传检查」只扫 `internal/repository`，不扫 `internal/ops/repository` 与 `internal/content/repository` ⇒ 它报「所有 Repository 导出方法都含 ctx」是假绿，实测有 61 个方法没接 ctx（含 `internal/ops/repository/churn_prediction.go` 11 个、`custom_report.go` 6 个、`internal/content/repository/` 下 23 个）。后果：SaaS(on) 模式下这些租户表被隔离 callback 判「ctx 未绑定」而 500（已实测 `GET /api/scripts` 在 8204 返回 `INTERNAL_ERROR_6002`、8304 正常 200）。这属于**任务一 4.3** 的范围，另行处理。
- 提交前总门禁仍需 `make audit` + `make vet` + `make test-go` 三道在本地跑绿。
