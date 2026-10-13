# Bug Assessment: `event_tracker: AutoTagger.ProcessEvent recovered from panic`

- **Slug**: `autotagger-panic`
- **Date**: 2026-10-13
- **Source**: `/tmp/fulltest.log` 中 recover 捕获的 nil-pointer panic
- **Frequency**: 2 次
- **Verdict**: valid, defensive fix applied

## 现象

```
event_tracker: AutoTagger.ProcessEvent recovered from panic: runtime error: invalid memory address or nil pointer dereference
AutoTagger.ProcessEvent error: 客户不存在
```

panic 后紧跟 `ErrCustomerNotFound`，说明 `ProcessEvent` → `EvaluateAndTag` 路径中
有 nil dereference。

## 代码路径分析

`event_tracker.go:83` → `auto_tagger.go:216 ProcessEvent` → `auto_tagger.go:43 EvaluateAndTag`

### 可能的 nil dereference 点

1. **`buildCustomerDataSnapshot` (line 107→373)** — `customer` 已被 nil check 保护（line 48）
2. **`GetCustomerEventData(event)` (line 383)** — 需确认是否可能 nil dereference
3. **`s.loadAssignmentTimes` (line 77)** — `s.assignRepo` 已初始化
4. **`s.recordNewAssignments` (line 131)** — 需确认
5. **Repo 方法** — `custRepo.GetByID` / `tagRepo.ListAutoTags` 等已初始化

### 限制

应用层 recover 只捕获了 panic 消息，**无完整 Go 堆栈**。需进一步定位。

## 修复状态

- **Status**: 防御性修复 (commit `a748f970`)
- **修复内容**：`auto_tagger.go:399-407` 加 `event != nil` 检查
- **堆栈增强**：`event_tracker.go:80` recover 点已加 `debug.Stack()` 输出（commit `6c389c8e`）
- **精确根因**：待后续 panic 发生时通过堆栈输出确认

## 后续

- [ ] 等待 panic 复现时通过 `debug.Stack()` 确认精确 nil dereference 行
