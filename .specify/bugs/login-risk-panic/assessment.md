# Bug Assessment: `[login-risk] panic recovered`

- **Slug**: `login-risk-panic`
- **Date**: 2026-10-13
- **Source**: `/tmp/fulltest.log` 中 recover 捕获的 nil-pointer panic
- **Frequency**: 1 次
- **Verdict**: valid, defensive fix applied

## 现象

```
[login-risk] panic recovered
panic="invalid memory address or nil pointer dereference"
```

## 代码路径分析

`auth.go:89` → `anomaly_login_detector.go:109 DetectAndAlert` → `login_risk.go:86 Evaluate`

### 可能的 nil dereference 点

1. **`s.resolveLocation(rctx, riskCtx.IP)` (line 97)** — 可能依赖外部服务返回 nil
2. **`s.countRecentFailures` (line 106)** — 数据库查询，一般安全
3. **`s.getPreviousLoginLocation` (line 116)** — 数据库查询
4. **`s.isDeviceFingerprintChanged` (line 130)** — 数据库查询
5. **`d.config` 字段** — `DefaultAnomalyLoginDetectorConfig()` 已初始化

### 限制

应用层 recover 只捕获了 panic 消息，**无完整 Go 堆栈**。需进一步定位。

## 修复状态

- **Status**: 防御性修复 (commit `a748f970`)
- **修复内容**：`login_risk.go:86-89` 加 `riskCtx == nil` 早退
- **堆栈增强**：`auth.go:83` recover 点已加 `debug.Stack()` 输出（commit `6c389c8e`）
- **精确根因**：待后续 panic 发生时通过堆栈输出确认

## 后续

- [ ] 等待 panic 复现时通过 `debug.Stack()` 确认精确 nil dereference 行
