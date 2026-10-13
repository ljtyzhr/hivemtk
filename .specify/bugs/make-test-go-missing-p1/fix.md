# Bug Fix: `make test-go` 补回 `-p 1 -timeout 900s`，与 CI 口径对齐

- **Slug**: `make-test-go-missing-p1`
- **Fixed**: 2026-10-13
- **Assessment**: ./assessment.md
- **Status**: applied

## Summary

`Makefile` 的 `test-go` 目标漏掉了 CI 早已明确的 `-p 1`（以及配套 `-timeout 900s`），
导致本地用 Go 默认并行度并发跑共享测试库的包，包间互相争抢测试库槽位/自增主键而集体挂起，
4 个重度依赖测试库的包在 10 分钟默认超时点同时 FAIL。
本次只改 `Makefile` 一行命令 + 补注释，把本地 gate 与 CI 拉齐。

## Changes

| File | Change | Notes |
|------|--------|-------|
| `Makefile` | modified（`test-go` 目标，446-461 行） | 命令由 `go test ./... -count=1` 改为 `go test ./... -count=1 -p 1 -timeout 900s`；新增 13 行注释说明 `-p 1` 为何是硬要求、与 CI 的对应行号、以及与 CI 的有意差异 |

未改动任何 Go 源码或测试文件——本次缺陷在**门禁命令**，不在被测代码（assessment 的对照实验已证明
单跑任一失败包都通过）。

## Diff Highlights

```diff
 test-go:
-	cd user-server && go test ./... -count=1
+	cd user-server && go test ./... -count=1 -p 1 -timeout 900s
```

改动位置 446 行；并行泳道在 `Makefile` 的已暂存 hunk 位于 540 行（`audit:` 目标），
`git diff --cached Makefile` 与 `git diff Makefile` 的 hunk 头分别为
`@@ -540,6 +540,8 @@` 与 `@@ -446,8 +446,19 @@`，**不重叠，不会冲突**。

## Tests Added or Updated

- 本轮未新增自动化断言。assessment 的「Tests to add or update」已说明：本仓没有针对
  Makefile 目标定义的现成静态断言测试，本轮以**实跑 `make test-go` 全绿**作为验证口径
  （见 Local Verification）。「`test-go` 必须含 `-p 1`」的防退化断言列入 Follow-ups。

## Local Verification

- Commands run:
  - `make test-go`（导出 `POSTGRES_TEST_HOST=127.0.0.1 POSTGRES_TEST_PORT=8202 POSTGRES_TEST_PASSWORD=<.env 值>`）
    → **进行中**，日志 `/tmp/fix-verify-test-go.log`（串行全量，耗时数分钟量级）。结论以
    `test.md` 记录的实际退出码与 FAIL 数为淮，此处不预先填结果。
- Manual checks:
  - `sed -n '449,463p' Makefile` → 确认新命令体与注释就位。
  - `grep -n "^test-go:" -A 4 Makefile` → 目标体只此一处，无重复定义覆盖。
  - CI 侧 `.github/workflows/user-server-ci.yml:566` → 确认 `-p 1` / `-timeout 900s` 取值来源。

## Deviations from Assessment

无。根因、修复方案、文件范围、`-short`/`-race` 的取舍均与 assessment 一致。

## Follow-ups

- 补一个轻量静态断言（挂 `scripts/` 与 `make audit` 同族）：`Makefile` 的 `test-go` 必须含 `-p 1`，
  防止再次退化；本轮未做，避免在别人 lane 有未提交改动的 `Makefile`/`scripts/` 上叠加范围。
- 在 `CLAUDE.md` 验证口径处补一句「手敲 `go test` 也不要并发跑多个依赖 testutil 的包」，
  让规则不只约束 `make test-go` 这一条路径。
- 后续可挖的真实 bug 候选（本轮未动，来自 `/tmp/fulltest.log` 中 recover 捕获的 nil-pointer）：
  `[login-risk] panic recovered`（1 次）、`event_tracker: AutoTagger.ProcessEvent recovered from panic`（2 次）、
  `[JourneySleepCron] 沉睡检测 panic`（1 次）。
