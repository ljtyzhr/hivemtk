# Bug Fix: LoginRiskService.Evaluate riskCtx nil guard

- **Slug**: `login-risk-panic`
- **Fixed**: 2026-10-13
- **Assessment**: ./assessment.md
- **Status**: applied (defensive)

## Summary

`LoginRiskService.Evaluate` (line 86) 无 `riskCtx` nil 检查，直接访问 `riskCtx.LoginAt`
导致 nil pointer dereference。

在 `Evaluate` 方法入口加 `riskCtx == nil` 早退防御检查。

## Changes

| File | Change | Notes |
|------|--------|-------|
| `internal/service/login_risk.go` | modified（`Evaluate` 方法，~line 86-89） | 加 `riskCtx == nil` 早退 |

## Diff Highlights

```diff
 func (s *LoginRiskService) Evaluate(ctx context.Context, riskCtx *RiskContext) (*RiskResult, error) {
+    if riskCtx == nil {
+        return nil, ErrRiskContextNil
+    }
     // ... evaluate logic
 }
```

## Local Verification

- Commit: `a748f970` — `fix(panic): LoginRiskService riskCtx nil + AutoTagger event nil guard`
- Commit: `6c3898e` — `chore(panic): 三个 recover 点加 debug.Stack() 输出完整堆栈`
- 集成测试：`ok hivemtk-user/test/integration 8.314s` ✅

## Deviations from Assessment

assessment 中的后续项「获取完整 panic 堆栈」已通过 `debug.Stack()` 输出实现。
当前修复为防御性 nil guard，精确根因需后续堆栈输出确认。

## Follow-ups

- 等待下次测试运行时 `debug.Stack()` 输出，确认精确 nil dereference 行
- 如堆栈指向其他位置，需进一步修复
