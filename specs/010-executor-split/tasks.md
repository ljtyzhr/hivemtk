# Tasks: executor.go 拆分（纯重构）

**Feature**: `010-executor-split` ｜ 勾选即以「已验证」为准（无验证不算完成）

## Phase 1 取证

- [x] T001 逐行读 executor.go（1901 行），产出 49 函数 / 9 类型 / 常量块清单
- [x] T002 按职责分组并标注行区间（核心 / stop / confirm / session / step / blockdetect / dispatch / helpers）
- [x] T003 盘点源码静态锁测试：7 处 readSrc/os.ReadFile("executor.go") 的锁面与拆分后落点

## Phase 2 拆分生成

- [x] T004 写锚点行区间抽取脚本（不依赖行号，锚点漂移即报错）
- [x] T005 内置拼接一致性校验（8 段按原顺序拼回 == 原文件），通过后生成 8 文件
- [x] T006 为每文件配最小 import 集 + package 子句 + 一行文件说明注释
- [x] T007 gofmt -w 全部 executor*.go，`gofmt -l` 空输出

## Phase 3 编译与补测

- [x] T008 `go build ./...`（干净工作树）通过
- [x] T009 新增 `executor_split_test.go`：taskPlatformID / sendErrText / lastN / historyActionOf / auditEventPayload / sleepInterruptible 六用例，不连 DB
- [x] T010 `go vet ./internal/browser_automation/...` 通过
- [x] T011 `go test ./internal/browser_automation/... -run '^$'` 全包测试文件编译通过

## Phase 4 静态锁测试适配

- [x] T012 post_comment_finalize_test.go：3 处 executor.go → executor_dispatch.go
- [x] T013 brain_action_gate_b19h_test.go：os.ReadFile("executor.go") → executor_dispatch.go
- [x] T014 executor_selfheal_test.go：executor.go → executor_dispatch.go
- [x] T015 batch14_send_gate_b14_test.go：executor.go → executor_dispatch.go
- [x] T016 executor_confirm_test.go：拼接 executor.go+confirm+step+dispatch（次序语义不变）
- [x] T017 timeouts_b8_test.go：executor.go + executor_session.go（deadline / WithoutCancel×2 / sessionFinalWriteBudget）
- [x] T018 ledger_b16c_test.go：retryBackoff → executor_step.go；sent→final 无早返 → executor_dispatch.go

## Phase 5 验证与交付

- [x] T019 纯单元/静态用例 48+ 显式全绿（含新增 6 用例）
- [x] T020 service 包全量测试 FAIL 名称集合与基线 diff == 空（135 个 DB 用例，其余全绿）
- [x] T021 回写 delivery.md（拆前拆后数据、命令与结果、保守保留、决策点、遗留项）
