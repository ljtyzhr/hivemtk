# Feature Specification: A1 巨型文件分层重构——executor.go 拆分（纯重构）

**Feature**: `010-executor-split`
**Created**: 2026-10-24
**Status**: Implemented
**Input**: 把 `user-server/internal/browser_automation/service/executor.go`（实测 1901 行）拆成多个同包文件，
按职责分段、消重复，**不改任何可观察行为、不改公开 API/函数签名**。

## 现状调研结论（Argumentation，逐行读码取证）

### 文件画像

| 度量 | 数值 |
|---|---|
| 总行数 | 1901 |
| 顶层函数 | 49 |
| 顶层类型 | 9（StepParams / StepRuntime / parsedStep / brainPlanner / Executor / PendingConfirmGate / confirmGate / ConfirmVerdict / confirmOutcome） |
| 顶层常量块 | 5（ConfirmVerdict、confirmOutcome、d7PreviewRunes、gateWaitFrame/gateConfirmFrame、commentSendAction/writeConfirmFrame、historyWindow） |
| 最大单函数 | dispatchStep（约 327 行，15 个 case） |
| 次大单函数 | executeBrain（约 271 行） |

### 职责分组（拆分依据）

| 行区间 | 职责 | 行数 | 去向 |
|---|---|---|---|
| 1–20 | package + import | 20 | 各文件头 |
| 22–133 | StepParams/StepRuntime/parsedStep/brainPlanner/Executor 结构体 | 112 | executor.go |
| 135–168 | NewExecutor 构造 | 34 | executor.go |
| 169–232 | finishStep / 6 个 Set* 注入 / appendCommandLog | 64 | executor.go |
| 234–260 | verdict / auditEventPayload | 27 | executor.go |
| 262–307 | 停止/中断注册表（SignalStop/registerStop/unregisterStop/stopFired/stopChFor） | 46 | executor_stop.go |
| 309–555 | D7 人工确认闸门（类型/挂起/放行/超时归因/审计帧） | 247 | executor_confirm.go |
| 557–562 | sendErrText | 6 | executor_helpers.go |
| 564–584 | cleanupSessionTab 收口回收 | 21 | executor_session.go |
| 586–617 | 错误谓词 isInjectTimeout/isSendGateReject/isNeverDispatched | 32 | executor_helpers.go |
| 619–757 | ExecuteSession 显式编排循环 | 139 | executor_session.go |
| 759–1029 | executeBrain Brain 模式循环 | 271 | executor_session.go |
| 1031–1247 | executeStepWithRetry（重试/双发闸/预算/派生写闸门） | 217 | executor_step.go |
| 1249–1338 | detectBlockedIfFatal 拦截页检测 | 90 | executor_blockdetect.go |
| 1340–1666 | dispatchStep 动作分发（15 case） | 327 | executor_dispatch.go |
| 1668–1729 | recordResultPayload / mergeExtract / finalizeComment | 62 | executor_dispatch.go |
| 1731–1901 | 纯函数与小型工具（buildStepParams/taskPlatformID/ParseSteps/睡眠/历史压缩/stepChangedPage/stepErrRetryable 等） | 171 | executor_helpers.go |

### 现有测试约束（拆分前必须摸清的硬约束）

该包存在一类**源码静态锁测试**：它们 `os.ReadFile("executor.go")` / `readSrc(t, "executor.go")`
直接读源码文本、锁「某段代码的相对次序 / 调用点个数」。这类测试在文件拆分后若不同步改
「文件定位」，会全部假红：

| 测试文件 | 锁的内容 | 拆分后落点 |
|---|---|---|
| executor_confirm_test.go `TestConfirmGatePrecedesIrreversibleSend` | prep→gate→send 次序、awaitConfirmGate×2、waitForConfirm×1、register→frame→wait、派生闸门先于命令帧 | confirm/step/dispatch 三文件 |
| post_comment_finalize_test.go（3 处） | prep→send→finalize 次序、finalize 只读、注入超时归因次序 | dispatch |
| brain_action_gate_b19h_test.go `b19hDispatchCases` | dispatchStep case 集合 == dto 白名单 | dispatch |
| executor_selfheal_test.go `TestSelfHealWiredSingleShot` | 自愈单发、注入超时先于自愈、finalize 禁提交 | dispatch |
| batch14_send_gate_b14_test.go `TestSendGateOrderedBeforeSentLedger` | send→闸门→落 sent 次序 | dispatch |
| timeouts_b8_test.go `TestBudgetDecoupleWiringLocks` | Brain 真钟期限、≥2 处 WithoutCancel、终态写预算 | executor.go + session |
| ledger_b16c_test.go（2 处） | 退避函数接线、sent→final 两次落账无早返 | step / dispatch |

### 已有纯单元测试覆盖（不重写）

拆分出的纯函数**多数已有直接测试**：`stepErrRetryable`/`stepChangedPage`/`buildStepParams`/
`appendHistoryBoundedFolds`/`recordResultPayload`/`humanizedDelay`/`isNeverDispatched`/
`isInjectTimeout`/`isSendGateReject`/`mergeExtract`/`stopChFor` 生命周期等。本卡**只补无直接测试的
纯函数**（见 Requirements FR-6），不重复造轮子。

## Requirements

- **FR-1** 纯重构：全部函数体、注释、常量、结构体字段**逐字节保留**；公开 API 与函数签名零改动。
- **FR-2** 拆成同包多文件（`executor_*.go`），职责边界如上表；每文件自带 package 子句与最小 import 集。
- **FR-3** 7 处静态源码锁测试的「文件定位」随拆分更新；**断言强度一字不减**，只改读哪个文件。
- **FR-4** `go build ./...`、`gofmt -l`、`go vet ./internal/browser_automation/...` 必须绿。
- **FR-5** 需要真库的测试不跑（本机 PG 口令与测试库不匹配）；以「编译通过 + FAIL 集合与基线一致」兜底。
- **FR-6** 为拆分出但无直接测试的纯函数补 6 个不连 DB 的单元用例：`taskPlatformID`、`sendErrText`、
  `lastN`、`historyActionOf`、`auditEventPayload`、`sleepInterruptible`。

## Success Criteria

- **SC-1** executor.go 从 1901 行降至 253 行；最大单文件 ≤ 448 行；文件数 1 → 8。
- **SC-2** 拆分前后函数内容逐字节一致（拼接一致性脚本校验通过）。
- **SC-3** 干净工作树（HEAD）下 `go build ./...` / `go vet` / `gofmt -l` / 测试文件编译全绿。
- **SC-4** service 包全量测试的 FAIL 名称集合与拆分前基线**完全一致**（135 个 DB 用例，其余全绿）。
- **SC-5** 48+ 个纯单元/静态用例显式全绿（含新增 6 个）。

## 边界（明确不做什么）

- 不新建子包（大量未导出类型 parsedStep/confirmGate/StepParams 跨文件共享，导出即扩大 API 面）。
- 不改任何函数实现（哪怕看到可优化点：本卡只做“搬”，不做“改”）。
- 不改 Executor 结构体字段、不改仓储接口、不改路由装配。
- 不清理 `internal/browser_automation/` 之外的任何文件（含 payment*.go、module_tables.go、scripts/、CHANGELOG、.github/workflows、specs/006）。
- 不动 docs 中引用 executor.go 行号的历史文档（行号偏移属文档性陈旧，另卡处理）。
