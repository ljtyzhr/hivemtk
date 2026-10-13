# Bug Fix: customer_orchestrator recover 点添加 debug.Stack() 输出

- **Slug**: `orchestrator-recover-no-stack`
- **Fixed**: 2026-10-13
- **Assessment**: ./assessment.md
- **Status**: applied

## Summary

`customer_orchestrator.go` 中 8 个 `recover()` 点（行 95, 105, 129, 139, 155, 181, 194, 206, 218）
只记录 `logger.Errorf` 消息，不输出 `debug.Stack()`。

本次为所有 8 个 recover 点添加 `debug.Stack()` 输出，与 `smart_cs_orchestrator.go` 保持一致。

## Changes

| File | Change | Notes |
|------|--------|-------|
| `user-server/internal/service/customer_orchestrator.go` | modified | 添加 `runtime/debug` import；8 个 recover 点添加 `debug.Stack()` 输出 |

## Diff Highlights

```diff
 import (
 	"context"
 	"fmt"
+	"runtime/debug"
 	"sync"
```

```diff
-				logger.Errorf("orchestrator.OnCustomerCreated: journey.Touch panic: %v", r)
+				logger.Errorf("orchestrator.OnCustomerCreated: journey.Touch panic: %v\n%s", r, debug.Stack())
```

（8 处相同模式）

## Tests Added or Updated

本轮未新增自动化断言。以 `go build` + 集成测试作为验证口径。

## Local Verification

- `go build ./internal/service/` — 待验证
- 集成测试 — 待运行

## Deviations from Assessment

无。修复方案与 assessment 一致。

## Follow-ups

无。
