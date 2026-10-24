# Implementation Plan: executor.go 拆分（纯重构）

**Feature**: `010-executor-split` ｜ **Created**: 2026-10-24 ｜ **Spec**: `./spec.md`

## 技术路线（一句话）

把 1901 行的 `executor.go` 按职责切为 8 个同包文件：核心类型与构造留在 `executor.go`，
停止注册表、D7 确认闸门、Session 生命周期、单步执行、动作分发、拦截检测、纯函数工具各居一文件；
函数体逐字节搬运（锚点行区间抽取 + 拼接一致性校验），并把 7 处「读 executor.go 源码」的静态锁测试
改到拆分后的文件上。

## 拆分映射

| 新文件 | 内容 | 行数 |
|---|---|---|
| executor.go | StepParams/StepRuntime/parsedStep/brainPlanner/Executor 结构体、NewExecutor、finishStep、6 个 Set*、appendCommandLog、verdict、auditEventPayload | 253 |
| executor_stop.go | SignalStop/registerStop/unregisterStop/stopFired/stopChFor | 49 |
| executor_confirm.go | PendingConfirmGate/confirmGate/ConfirmVerdict/confirmOutcome/闸门常量、SignalConfirm/ConfirmPending/PendingGate/registerConfirmGate/waitForConfirm/gateDecision/awaitConfirmGate | 258 |
| executor_session.go | cleanupSessionTab、ExecuteSession、executeBrain | 448 |
| executor_step.go | executeStepWithRetry（重试/退避/双发闸/预算/派生写闸门） | 232 |
| executor_blockdetect.go | detectBlockedIfFatal | 103 |
| executor_dispatch.go | dispatchStep、recordResultPayload、mergeExtract、finalizeComment | 406 |
| executor_helpers.go | sendErrText、isInjectTimeout/isSendGateReject/isNeverDispatched、buildStepParams、taskPlatformID、ParseSteps、sleepInterruptible、humanizedDelay、lastN、historyWindow/appendHistoryBounded/historyActionOf/buildFoldHeader、stepChangedPage、stepErrRetryable | 229 |

合计 1978 行（含每文件 package/import 头；函数内容与原文件逐字节一致）。

## 关键设计决策（含被否决方案）

### D1 同包多文件（否决：新建子包）

- 采纳理由：`parsedStep`、`confirmGate`、`StepParams`、`confirmOutcome` 等大量**未导出**类型跨函数共享；
  拆子包必须导出它们或引入接口，等于为“文件变小”扩大公开 API 面，违背「纯重构」前提。
- 被否决方案：`service/executor/` 子包。代价是公开 API 扩散 + 循环依赖风险 + 装配层改动。

### D2 粒度 8 文件（否决：只拆成 4 个）

- 采纳理由：executeBrain（271）+ ExecuteSession（139）+ cleanupSessionTab 合成 session 文件后 448 行，
  是「不再增长」的上限；dispatchStep（327）+ finalize 合成 dispatch 后 406 行。若只拆 4 文件，
  单个文件仍在 500+ 行，没有解除「巨型文件」的根本压力。
- 被否决方案：4 文件粗拆（核心/session/step/dispatch+helpers）。

### D3 函数零改动（否决：顺手优化）

- 采纳理由：本轮唯一可验证手段是「逐字节一致」。任何“顺手”改动（哪怕局部变量重命名）都会破坏
  可验证性，并可能与并发 lane 的改动产生语义冲突。
- 被否决方案：搬运时顺手消重复/换变量名。

### D4 静态源码锁测试：改「文件定位」不改「断言」

- 采纳理由：源码静态锁锁的是**语义不变式**（次序、调用点个数），不是“函数必须住在哪个文件”。
  拆分必然改变文件边界，同步改 readSrc/os.ReadFile 的路径即可；断言原文保留，锁的强度不变。
- 关键实现：`TestConfirmGatePrecedesIrreversibleSend` 跨 3 个文件锁次序，按
  `executor.go + executor_confirm.go + executor_step.go + executor_dispatch.go` 固定顺序拼接，
  `LastIndex(awaitConfirmGate)` 仍落在 post_comment 调用点，与拆分前语义一致。

### D5 补测而非重测

- 采纳理由：可拆纯函数多数已有测试（见 spec）。只为**无直接测试**的 6 个纯函数补用例，
  落在一个新文件 `executor_split_test.go`，不连 DB。
- 被否决方案：为每个拆分函数各写一遍测试（大量重复、无信息增量）。

## 实施阶段

| 阶段 | 内容 | 产物 |
|---|---|---|
| P1 | 取证：逐行读 executor.go，产出函数/类型清单与职责分组 | spec.md §现状调研 |
| P2 | 锚点行区间抽取 + 拼接一致性校验脚本（一次性验证工具，不落库） | /tmp 脚本 |
| P3 | 生成 8 文件 + 最小 import 集，gofmt | 8 个 executor*.go |
| P4 | 补测 `executor_split_test.go`（6 用例） | 新测试文件 |
| P5 | 适配 7 处静态源码锁测试的文件定位 | 7 个测试文件各 1–2 行 |
| P6 | 干净工作树验证：build/vet/gofmt/测试编译/全量 FAIL 集合比对 | 见 delivery |

## 验证口径

| 改动范围 | 命令 |
|---|---|
| Go 全量 | `cd user-server && go build ./...` |
| 包内静态检查 | `go vet ./internal/browser_automation/...` |
| 格式 | `gofmt -l internal/browser_automation/`（空输出） |
| 测试编译 | `go test ./internal/browser_automation/... -run '^$'` |
| 纯单元/静态用例 | `go test ./internal/browser_automation/service/ -run '<纯函数/静态锁 regex>'` |
| 全量回归 | `go test ./internal/browser_automation/service/` 后与基线 FAIL 集合 diff |

## 风险与回滚

| 风险 | 处置 |
|---|---|
| 搬运时丢行/串行 | 抽取脚本内置「拼接一致性校验」：8 段按原顺序拼回 == 原文件，不等即退出 |
| 静态锁测试假红 | P5 统一改文件定位；跑 48+ 纯单元/静态用例确认全绿 |
| import 缺/多 | 每个新文件最小 import 集；`go build` 兜底，未用 import 由 gofmt/编译报错逼出 |
| 并发 lane 改到本目录 | 每次动手前 `git status --porcelain -- <dir>` 复检；本卡只动 executor*.go 与相关测试 |
| 主树 build 被外部 WIP 阻断 | 用 `git worktree add`（HEAD 干净树）复制本目录做隔离验证，不触碰任何 lane 文件 |
