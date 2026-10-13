# Bug Fix: AutoTagger.ProcessEvent nil event guard

- **Slug**: `autotagger-panic`
- **Fixed**: 2026-10-13
- **Assessment**: ./assessment.md
- **Status**: applied (defensive)

## Summary

`AutoTagger.ProcessEvent` 中 `buildCustomerDataSnapshot` 遍历 events 时无 nil 检查，
当 event 对象为 nil 时可能触发 nil pointer dereference。

在 event 遍历循环中加 `event != nil` 防御检查。

## Changes

| File | Change | Notes |
|------|--------|-------|
| `internal/service/auto_tagger.go` | modified（`buildCustomerDataSnapshot`，~line 399-407） | 加 `event != nil` 检查 |

## Diff Highlights

```diff
 for _, event := range events {
+    if event == nil {
+        continue
+    }
     // ... process event
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
