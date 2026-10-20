# Bug Assessment: 4 条 golangci-lint 存量红挡住提交总门禁

- **Slug**: golangci-lint-dead-code-and-swallowed-error
- **Created**: 2026-10-20
- **Source**: 自查（前置任务一第 3 段对抗性检查期间跑提交前总门禁 `make lint`）
- **Verdict**: valid
- **Severity**: medium

## Report (verbatim or summarized)

本轮为「前置任务一」第 3 段（对抗性检查、审核、测试整个 SaaS 多租户改造，发现问题立即修复）的延伸。提交前总门禁按 CLAUDE.md 规则 3.5/4.4 = `make audit` + `make lint` + `make vet` + `make test-go`，其中 `make lint` 在 user-server 跑 `golangci-lint run ./...`。该门在 HEAD 上即红 4 条，且 4 条全部落在本轮**未触碰**的已提交文件上（`git grep HEAD` 逐条核对过，HEAD 形态一模一样）⇒ 属仓内既有护栏债，不是本轮 SaaS 改造引入。

## Symptom

```
cd user-server && golangci-lint run ./...
```

输出 4 issues in 4 files：

1. `internal/middleware/rate_quota.go:240` — `unused`: `func (m *pathQuotaLimiterManager) stop() {`
2. `internal/service/customer_session_cron.go:95` — `errcheck`: `c.trigger(ctx)`
3. `internal/service/reach_send_pipeline_quiethours.go:34` — `unused`: `nextDayFirstSendHour = quietHoursEndHour`
4. `internal/bridge/sse.go:540` — `ineffassign`: `ctxReq := c.Request.Context()`

`make lint` 因此 rc≠0 ⇒ 总门禁不绿 ⇒ 不允许提交推送（CLAUDE.md 规则 1、规则 31）。

## Reproduction

```bash
cd user-server && golangci-lint run ./... ; echo "RC=$?"
# => 4 issues in 4 files, RC=1
```

## Suspected Code Paths

1. `user-server/internal/middleware/rate_quota.go:240` `func (m *pathQuotaLimiterManager) stop() { close(m.stopChan) }`
   - `stopChan` 由 `newPathQuotaLimiterManager`（:128-133）创建，`cleanupLoop`（:209，5 分钟 ticker，:216 `case <-m.stopChan:`）监听。
   - 生产面无调用方。唯一调用方是 `user-server/internal/middleware/rate_quota_middleware_test.go:22,28`（`resetPathQuotaManagerForTest` 换新管理器前停旧清理协程；Cleanup 再停一次；每次都先 `globalPathQuotaMgr = nil` ⇒ 不会重复 close 同一 channel）。
   - `user-server/.golangci.yml` 设 `run.tests: false` ⇒ 测试里的调用不算使用 ⇒ 判 unused。
   - 仓内已有 23 处同形先例（`//nolint:unused //// 仅被 *_test.go 引用，生产路径未用`），例：`internal/app/tool_circuit_breaker_wiring.go:151`、`internal/controller/dashboard_sse.go:243`、`internal/aiagent/rag/retrieval/rrf_fusion.go:87`、`internal/service/inbox_ingress.go:254`、`internal/channelbot/telegram/telegram.go:382`。
2. `user-server/internal/service/customer_session_cron.go:95` `c.trigger(ctx)`
   - 位于 `func (c *SessionTTLCron) tryTriggerWithDBWait(ctx context.Context)` 内。
   - 多租户分支（:89-93）走 `_, _ = tenant.Fanout(ctx, c.db, func(ctx context.Context, _ uint) (int64, error) { return c.trigger(ctx) })` 后 return；:95 是**非多租户分支**裸调用，丢弃 `(int64, error)`。
   - `trigger` 签名 `func (c *SessionTTLCron) trigger(ctx context.Context) (int64, error)`（:98，内部 `defer recover()` + `return c.sessionSvc.AutoCloseStaleSessions(ctx)`）。
3. `user-server/internal/service/reach_send_pipeline_quiethours.go:34` `nextDayFirstSendHour = quietHoursEndHour`
   - const 块原文：`const (` / `quietHoursStartHour = 22` / `quietHoursEndHour   = 8` / 空行 / `nextDayFirstSendHour = quietHoursEndHour` / `)`。
   - 静默窗口释放时间由 `nextQuietHoursRelease(t, endHour)` 按参数算，不引用该 const ⇒ 死常量。全仓唯一提及是 `user-server/internal/service/config_param_seeds.go:890` 的注释「nextDayFirstSend 恒等于 end（代码里 nextDayFirstSendHour = end），不单独设键。」
4. `user-server/internal/bridge/sse.go:540` `ctxReq := c.Request.Context()`
   - 540 到 572 之间只有入参校验（accountID / channel / IsBridgeChannel）与 `bindBridgeTenant`（`user-server/internal/bridge/tenant_binding.go:102-112`，内部 `rctx := c.Request.Context()` 并返回派生 bound），期间不读 ctxReq。
   - :572 `ctxReq = reqCtx`（`reqCtx, terr := bindBridgeTenant(c, channel, accountID)`）覆盖初值 ⇒ 初值从未被读 ⇒ 死赋值。
   - ctxReq 真正使用点：:576 `runtimeSSEHeartbeatInterval(ctxReq)`、:580 `runtimeSSEMaxStreamDuration(ctxReq)`、:600 `context.WithTimeout(ctxReq, maxStreamDuration)`。

## Root Cause Hypothesis

4 条成因各不相同，共同点是「仓内既有的护栏式清理（unused 存量 65→0、ineffassign 存量 26→0、errcheck 存量 354→0）在后续新增/改造代码时没有同步收口」：

- 第 1 条：清理协程停止方法只在 `*_test.go` 被调用，而 lint 的 `run.tests: false` 不看测试引用 ⇒ 命中 unused。仓内既定处置方式是加 `//nolint:unused` 并写明「仅被 *_test.go 引用，生产路径未用」。
- 第 2 条：多租户改造把 cron 切到 `tenant.Fanout` 之后，非多租户分支的裸调用没跟着处理 error ⇒ 真吞错（`.golangci.yml` R4 要求计数/状态类失败记日志且不因此改变控制流）。
- 第 3 条：静默窗口结束小时参数化（`nextQuietHoursRelease(t, endHour)`）之后，旧的 `nextDayFirstSendHour` 别名常量失去引用。
- 第 4 条：SSE 接入租户绑定（`bindBridgeTenant`）之后，`ctxReq` 的旧初值被新绑定 ctx 覆盖，形成死赋值。

## Proposed Remediation

### Preferred

逐条按 `user-server/.golangci.yml` 的 R1–R9 判定处理（注释明确要求「不做机械 `_ =` 包裹」），每条都保持生产语义不变：

1. `rate_quota.go:240` —— 在 `stop()` 上加 `//nolint:unused //// 仅被 *_test.go 引用，生产路径未用`，与仓内 23 处先例同形同措辞。生产路径确实不调用，进程退出即回收清理协程，不是泄漏。
2. `bridge/sse.go` —— 删 :540 初值，:572 改 `ctxReq := reqCtx`。`bindBridgeTenant` 已用 request ctx 派生租户绑定后的 ctx，删初值不改变 :576/:580/:600 拿到的 ctx 语义。`context` 包 import 仍需保留（:199/:407/:411/:500/:520/:800 仍在用）。
3. `reach_send_pipeline_quiethours.go` —— 删死常量 `nextDayFirstSendHour`；同步 `config_param_seeds.go:890` 注释里对 `nextDayFirstSendHour` 标识符的指认，改为「nextDayFirstSend 恒等于 end（同一参数）」，避免注释指向已不存在的标识符。`config_param_seeds.go:895` 的 Description 文案描述的是产品语义（次日首发小时恒等于结束小时），保持不动。
4. `customer_session_cron.go:95` —— `if _, err := c.trigger(ctx); err != nil { logger.Errorf(...) }`，按 R4 记日志、不改变控制流（不提前 return、不影响下一轮调度）。同文件 logger 已 import，措辞沿用 `[session_ttl_cron]` 前缀。

### Alternatives

- 对第 1、3 条直接删除而不加 nolint：会破坏 `rate_quota_middleware_test.go` 里真实存在的 `stop()` 调用，且与仓内 23 处先例不符 ⇒ 否。
- 对第 4 条加 `//nolint:ineffassign`：掩盖真死代码，与「unused/ineffassign 存量归零后钉死为护栏」的口径相反 ⇒ 否。
- 对第 2 条写 `_ = c.trigger(ctx)`：违反 `.golangci.yml` 注释里「errcheck 不做机械 `_ =` 包裹」⇒ 否。
- 改 `.golangci.yml` 加 excludes / 降级规则：为绿灯改护栏配置，与仓内口径相反 ⇒ 否。

### Files likely to change

- `user-server/internal/middleware/rate_quota.go`
- `user-server/internal/bridge/sse.go`
- `user-server/internal/service/reach_send_pipeline_quiethours.go`
- `user-server/internal/service/config_param_seeds.go`
- `user-server/internal/service/customer_session_cron.go`

### Tests to add or update

不新增用例。回归手段：

- `cd user-server && golangci-lint run ./...` ⇒ 0 issues（这是本 slug 的直接验收口径）
- `cd user-server && go build ./...` ⇒ 通过
- `cd user-server && go vet ./...` ⇒ 通过
- `go test ./internal/middleware/... ./internal/bridge/... ./internal/service/... -count=1`（真实 PostgreSQL，运行时取容器口令）⇒ 通过

## Risks & Considerations

- 删 `sse.go:540` 后必须确认 :576/:580/:600 仍拿得到**租户绑定后**的 ctx（来源 :572 `ctxReq := reqCtx`），否则会把租户绑定 ctx 退化成裸 request ctx ⇒ 跨租户读风险。本 slug 只做等价替换，不改取值来源。
- `rate_quota.go` 的 nolint 注释沿用仓内统一措辞，不新造风格。
- 5 个文件都可能被别的 lane 同时在改（工作区存在并行在途修改）⇒ 提交前必须核对 `git diff` 与本 slug 改动无交叉，若有交叉以本 slug 的最小改动为准并如实记录。
- 4 条在 HEAD 即红，修复后 `make lint` 由红转绿属于仓内护栏回归，不改变任何运行时行为。

## Open Questions

无。本 slug 不需要用户决策：4 条的修法都由仓内既有先例与 `.golangci.yml` 注释定死。
