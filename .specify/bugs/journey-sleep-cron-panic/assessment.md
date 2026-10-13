# Bug Assessment: `[JourneySleepCron] 沉睡检测 panic`

- **Slug**: `journey-sleep-cron-panic`
- **Date**: 2026-10-13
- **Source**: `/tmp/fulltest.log` 中 recover 捕获的 nil-pointer panic
- **Frequency**: 1 次
- **Verdict**: valid, fixed
- **Status**: **已修复** (commit `58107a08`)

## 根因分析

**明确根因**：`main.go:468` 调用 `service.NewJourneySleepCron(nil)` 传入 `nil` svc，
而 `runOnce` 方法（`customer_journey.go:642`）直接调用 `c.svc.AutoDetectSleeping(ctx)` 
无 nil 检查，导致 nil pointer dereference。

### 证据链

1. `main.go:468`：`journeySleepCron := service.NewJourneySleepCron(nil)` — 传入 nil
2. `customer_journey.go:588-592`：`NewJourneySleepCron` 接受 nil 并存储到 `c.svc`
3. `customer_journey.go:642`：`return c.svc.AutoDetectSleeping(ctx)` — 无 nil 检查
4. 日志：`[JourneySleepCron] 沉睡检测 panic: runtime error: invalid memory address or nil pointer dereference`

## 修复方案

在 `runOnce` 中加 nil svc 检查：

```go
if c.svc == nil {
    logger.Ctx(ctx).Error().Msg("[JourneySleepCron] svc 为 nil，跳过沉睡检测")
    return nil
}
```

## 修复验证

- Commit: `58107a08` — `fix(journey): JourneySleepCron.runOnce 加 nil svc 检查防止 panic`
- 需要 `make test-go` 全量验证
