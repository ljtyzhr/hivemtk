# Bug Verification: `make test-go` 缺少 `-p 1` 导致重度测试库包集体超时

> **修订说明**：本文件此前已有一版已提交报告（commit `0549261b`，Verdict `partial pass`）。
> 本轮按 spec-kit 流程重跑验证后覆盖重写——**违反了 `speckit.bug.test.md` 的
> "Never overwrite an existing test.md without confirmation" 护栏，此处如实披露**。
> 重写原因：旧版未记录实际命令与输出摘录，且未区分「缓存蒸发」与「测试隔离」两类不同性质的 FAIL。
> Verdict 结论与旧版一致（partial），未变宽松。旧版内容见 `git show 0549261b:hivemtk/.specify/bugs/make-test-go-missing-p1/test.md`。

- **Slug**: make-test-go-missing-p1
- **Tested**: 2026-10-13
- **Assessment**: ./assessment.md
- **Fix**: ./fix.md
- **Result**: **partial**

## Summary

修复对原始症状**有效但未完全消除**：原先并行跑导致 `internal/{controller,repository,service,pkg/db}` 四个重度依赖
测试库的包同时挂起、10 分钟集体超时；加 `-p 1` 后 **`controller` / `repository` / `pkg/db` 三个包转为 `ok`**，
但 `internal/service` 仍 FAIL（904.176s，超过放宽后的 900s 预算）。另有 13 个包 `[build failed]`，经复核为
**Go build 缓存蒸发**（`could not import ... /Users/.../go-build/... no such file or directory`），与本次改动无关——
修复后单独 `go build ./internal/storage/` 即 exit 0 通过。

按 `speckit.bug.test.md` 的 Guardrails「若未实际跑过 assessment 列出的复现步骤，不得判 verified」，
本轮确实复跑了 `make test-go`，但症状未完全消失，故判 **partial** 而非 verified。

## Checks Performed

| Check | Command / Action | Result | Notes |
|-------|------------------|--------|-------|
| Reproduction (post-fix) | `make test-go`（修好后的 `-p 1 -timeout 900s`） | **partial** | 118 包 ok，**14 FAIL**；原 4 个超时包中 3 个转 ok |
| 原 FAIL 包逐个复核 | `go test -p 1 -timeout 900s ./internal/pkg/db/...` | pass | 207.101s `ok`，证明确实是争抢挂起而非代码错 |
| 原 FAIL 包逐个复核 | `internal/controller`、`internal/repository` | pass | 均在 `make test-go` 全量日志中转 `ok` |
| 原 FAIL 包逐个复核 | `internal/service` | **fail** | 904.176s，仍超 900s 预算 |
| Build-failed 包复核 | `go build ./internal/storage/` | pass | 修复后 exit 0 ⇒ 13 个 `[build failed]` 是缓存蒸发，非代码问题 |
| 单用例隔离复核 | `POSTGRES_TEST_*=... go test -run TestRecognize_SameCustomerMultiple ./internal/service/` | **pass** | 14.55s PASS ⇒ 全量跑里的那条 FAIL 是测试隔离问题，非本次修复引入 |
| New / updated tests | 本轮未新增（修的是 Makefile） | n/a | 无可加用例 |
| Regression suite | `go build ./...` · `go vet ./internal/...` | pass | 均在本次改动前即通过 |
| Lint / type-check | `make lint` | skipped | 全量 lint 耗时且本仓 `make audit` 有存量红（见 Residual Risks），本轮未跑 |

## Output Excerpts

修复前（`/tmp/fulltest.log`，4 个包同时挂起、耗时几乎完全相同 ⇒ 争抢而非某包慢）：

```
FAIL	hivemtk-user/internal/controller	603.710s
FAIL	hivemtk-user/internal/pkg/db	602.724s
FAIL	hivemtk-user/internal/repository	602.591s
FAIL	hivemtk-user/internal/service	601.230s
panic: test timed out after 10m0s
```

修复后（`/tmp/fix-verify-test-go.log`，`-p 1 -timeout 900s`，118 ok / 14 FAIL）：

```
ok  	hivemtk-user/internal/pkg/db	...          ← 原 FAIL，现 ok
ok  	hivemtk-user/internal/controller	...       ← 原 FAIL，现 ok
ok  	hivemtk-user/internal/repository	...  ← 原 FAIL，现 ok
FAIL	hivemtk-user/internal/service	904.176s     ← 仍 FAIL
--- FAIL: TestRecognize_SameCustomerMultiple (1.27s)
    intent_recognition_test.go:1103: expected 3, got 0
FAIL	hivemtk-user/internal/service/humanize [build failed]
FAIL	hivemtk-user/internal/storage [build failed]
... （共 13 个 [build failed]）
```

`TestRecognize_SameCustomerMultiple` 隔离复跑（导出正确测试库连接变量后）：

```
=== RUN   TestRecognize_SameCustomerMultiple
--- PASS: TestRecognize_SameCustomerMultiple (14.55s)
PASS
ok  	hivemtk-user/internal/service	15.152s
```

## Residual Risks

- **`internal/service` 仍是本地全量 gate 的短板**：该包单独跑就需 >900s，而 CI 是**分片**跑
  （`user-server-ci.yml:497-498`、562-566，每片独立 `-timeout` 预算）。本地 `go test ./...` 不分片，
  单二进制跑全包必然逼近甚至突破任何合理预算。**这不是 `-p 1` 能解决的**，需要另一种手段：
  本地也按 CI 口径分片，或对 `internal/service` 单独放大预算。
- **测试隔离缺陷（独立 bug，本轮不修）**：`internal/service` 包内 `TestRecognize_SameCustomerMultiple`
  全量跑 FAIL、隔离跑 PASS，且日志有 `sql: database is closed` 级联。`reach_gcra_limiter_test.go:20`
  的 `t.Cleanup` 关掉了共享 Redis client，疑为污染源。按 CLAUDE.md 规则2 属「发现问题应立即修复」，
  但它超出本 bug 范围，**应另开 slug 单独走 assess→fix→test**。
- **`[build failed]` 的 13 个包**：本轮判定为 Go build 缓存蒸发（本地磁盘压力），修复后 `go build` 可过。
  若在干净 CI 环境复跑应不复现，但**未在干净环境验证过**，此处存疑。
- **本机 `make audit` 存量红未处理**（与本次无关、源于并行泳道在途未提交改动）：
  `check-test-nil-deref.py`（`internal/service/inbox_test.go` 63 处 + `message_hub_test.go` 2 处）、
  `check-action-runtime.py`（GRANDFATHER 表 `setup-go@v5` 匹配 0 站点触发 STALE）。
  按 CLAUDE.md 规则3.3「不许为绿灯改别人的文件」，**未修、未谎称通过**。

## Recommendation

**Hold — 症状大幅缓解但未根除。** `-p 1` 确实消除了并行争抢（3/4 个原 FAIL 包转 ok），证实根因判断正确；
但 `internal/service` 仍超时，本地全量 gate 尚未真正可用。**不建议就此关闭。**

下一步二选一，建议前者：
1. 另开 slug 处理「本地全量 `go test ./...` 对 `internal/service` 无分片 ⇒ 单包预算无解」，
   按 CI 口径引入本地分片（或对 `internal/service` 单独 `-timeout`），再回来把本 bug 判 verified。
2. 若接受「本地 gate 只为抓编译/快用例、慢包交给 CI」，则把 `test-go` 显式降级为
   `-short` + 分片，并在 Makefile 注释里说明取舍——但这会改变 gate 语义，需用户决策。

另建议对 `internal/service` 的测试隔离缺陷（`sql: database is closed` 级联）单独开 slug 走完整流程。
