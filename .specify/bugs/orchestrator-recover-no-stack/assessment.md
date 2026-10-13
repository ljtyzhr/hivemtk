# Bug Assessment: customer_orchestrator recover 点缺少 debug.Stack() 输出

- **Slug**: `orchestrator-recover-no-stack`
- **Date**: 2026-10-13
- **Source**: 代码扫描发现 `customer_orchestrator.go` 8 个 recover 点全部缺少堆栈输出
- **Verdict**: valid
- **Severity**: medium

## 现象

`customer_orchestrator.go` 中 8 个 `recover()` 点（行 94, 104, 128, 138, 154, 180, 193, 205, 217）
只记录 `logger.Errorf` 消息，**不输出 `debug.Stack()`**。

对比 `smart_cs_orchestrator.go` 的 12 个 recover 点，大部分已有 `debug.Stack()` 输出。

## 影响

当这些路径发生 panic 时：
1. 只能看到 `logger.Errorf` 的消息（如 "journey.Touch panic: %v"）
2. **无法追踪 panic 的具体调用栈和根因**
3. 与 `smart_cs_orchestrator.go` 的 recover 处理不一致

## 修复方案

为 `customer_orchestrator.go` 的 8 个 recover 点添加 `debug.Stack()` 输出，
与 `smart_cs_orchestrator.go` 保持一致。

## 后续

- [ ] 修复并验证
