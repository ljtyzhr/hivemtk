# Bug Fix: JourneySleepCron nil svc 检查

- **Slug**: `journey-sleep-cron-panic`
- **Fixed**: 2026-10-13
- **Assessment**: ./assessment.md
- **Status**: applied

## Summary

`main.go:468` 传入 `nil` svc 到 `NewJourneySleepCron`，`runOnce` 无 nil 检查直接调用
`c.svc.AutoDetectSleeping(ctx)` 导致 nil pointer dereference。

在 `runOnce` 中加 nil svc 防御检查，防止 panic。

## Changes

| File | Change | Notes |
|------|--------|-------|
| `internal/service/customer_journey.go` | modified（`runOnce` 方法，~line 635） | 加 `if c.svc == nil` 早退 + 错误日志 |

## Diff Highlights

```diff
 func (c *JourneySleepCron) runOnce(ctx context.Context) error {
+    if c.svc == nil {
+        logger.Ctx(ctx).Error().Msg("[JourneySleepCron] svc 为 nil，跳过沉睡检测")
+        return nil
+    }
     return c.svc.AutoDetectSleeping(ctx)
 }
```

## Local Verification

- Commit: `58107a08` — `fix(journey): JourneySleepCron.runOnce 加 nil svc 检查防止 panic`
- 集成测试：`ok hivemtk-user/test/integration 8.314s` ✅

## Follow-ups

- 考虑在 `NewJourneySleepCron` 构造函数中拒绝 nil svc（返回 error 或 panic），从源头防止误用
- `main.go:468` 为何传 nil svc 需确认是否为调试遗留
