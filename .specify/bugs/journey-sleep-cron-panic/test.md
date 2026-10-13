# Bug Test: JourneySleepCron nil svc 检查验证

- **Slug**: `journey-sleep-cron-panic`
- **Tested**: 2026-10-13
- **Fix**: ./fix.md
- **Result**: verified
- **Status**: verified

## Verification Commands

### 1. 静态确认

```bash
grep -n "svc == nil" internal/service/customer_journey.go
```

**结果**：确认 nil 检查已就位。

### 2. 集成测试

```bash
cd user-server && go test ./test/integration/ -v -count=1
```

**结果**：`ok hivemtk-user/test/integration 4.453s` ✅

## Test Results

| 检查项 | 结果 |
|--------|------|
| nil svc 检查代码就位 | ✅ |
| 集成测试通过 | ✅ |
| 无 panic 日志输出 | ✅ |

## Conclusion

修复有效：`runOnce` 在 svc 为 nil 时早退，不再 panic。
