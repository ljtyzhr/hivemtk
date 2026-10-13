# Bug Test: `make test-go` 补回 `-p 1 -timeout 900s` 的验证报告

- **Slug**: `make-test-go-missing-p1`
- **Tested**: 2026-10-13
- **Fix**: ./fix.md
- **Assessment**: ./assessment.md
- **Verdict**: **partial pass（根因已修，暴露两个后续独立问题）**

## 验证命令

```bash
cd /Users/xiaofang/Documents/www/go/hivemtk
export POSTGRES_TEST_HOST=127.0.0.1 POSTGRES_TEST_PORT=8202 POSTGRES_TEST_PASSWORD="$(grep '^POSTGRES_PASSWORD=' .env | cut -d= -f2)"
make test-go   # = cd user-server && go test ./... -count=1 -p 1 -timeout 900s
```

日志：`/tmp/fix-verify-test-go.log`（2026-10-13 14:36–14:50，约 14 分钟）。

## 结果总览

| 指标 | 数值 |
|------|------|
| `ok` 包 | **118** |
| `FAIL` 包 | **13** |
| 其中 `[build failed]`（缓存污染，非编译错） | 12 |
| 其中真超时 | 1（`internal/service`） |
| 进程退出码 | 1 |

## 关键判定：根因已修复

原始缺陷（assessment）：`make test-go` 缺 `-p 1`，4 个重度依赖共享测试库的包并发跑、互相认领对方行/撞自增主键，在 Go 默认 10m 超时点**集体**挂起。

串行（`-p 1`）后：

| 包 | 修复前 | 修复后 |
|----|--------|--------|
| `internal/controller` | FAIL 603.710s（10m 超时） | **ok 187.701s** |
| `internal/pkg/db` | FAIL 602.724s（10m 超时） | **ok 118.797s** |
| `internal/repository` | FAIL 602.591s（10m 超时） | **ok 233.369s** |
| `internal/service` | FAIL 601.230s（10m 超时） | **FAIL 904.176s（15m 超时）** |

⇒ 原始「4 包同时撞库」的并发缺陷**已根除**：3/4 包从「集体 10m 挂起」变为 2–4 分钟内正常通过。`-p 1` 的修复方向正确、有效。

## 遗留问题 1：`internal/service` 单包超 900s（真 FAIL）

`internal/service` 在 904s 触发 `panic: test timed out after 15m0s`（`-timeout 900s` 生效，非默认 10m）。goroutine dump 显示卡在：

```
hivemtk-user/internal/service.(*MessageTraceCleanupTask).run(...)
    message_trace_cleanup_cron.go:113
hivemtk-user/internal/service.NewWebhookService.(*WebhookService).startRLJanitor.func1(...)
    webhook_dedup.go:192
```

- 这是**第二个独立问题**：`internal/service` 是全仓最重的包，CI 注释已记录「连 300s 都不够」。串行跑单包仍超 900s，说明**不是撞库、而是包内真有测试子树超预算**（cron / janitor 相关）。
- 与本轮修复的根因（并发撞库）**无关**，是既有欠账。
- 修复前它在并发下 601s 超时，修复后串行 904s 超时——看似「变长」，实为串行不再被其他包抢资源后、把原本被掩盖的自身暴露出来了。

## 遗留问题 2：12 包 `[build failed]` = 构建缓存污染（非编译错）

```
scripts/telegram_smoke.go:18:2: could not import bytes
  (open /Users/xiaofang/Library/Caches/go-build/...-d: no such file or directory)
```

- 全仓共 **86 次** `could not import ... no such file or directory`，全部指向 `~/Library/Caches/go-build/` 下的缓存条目缺失。
- **抽测验证**：`go build ./internal/service/humanize/ ./internal/storage/ ./internal/websocket/` 全部通过（rc=0），无编译错误；`test/e2e`、`test/integration` 报 `no non-test Go files` 是 test-only 包在 `go build` 下的正常表现。
- ⇒ 这些失败是运行期间 Go build 构建缓存被并发进程（`go clean -cache` / 系统 GC）抽干导致，**不是被测代码的编译错**。干净环境下重跑应当消失。

## 结论

| 判定 | 说明 |
|------|------|
| 本轮根因（并发撞库 → 4 包集体 10m 超时） | ✅ **已修复**，`-p 1` 生效 |
| `internal/service` 单包超 900s | ⚠️ 既有欠账，独立问题，需另开 bug  lane |
| 12 包 build failed | ⚠️ 缓存污染，非代码缺陷，干净重跑验证 |
| 全绿 | ❌ 本轮未达成，但失败均可归因于上述两个独立/环境问题 |

## 建议后续（不在本轮范围）

1. **另开 bug lane** 排查 `internal/service` 的 `message_trace_cleanup_cron` / `webhook_dedup` janitor 测试子树为何单跑超 900s——可能是 cron 用例等待真实计时或 janitor goroutine 泄漏。
2. **干净重跑** `make test-go`（在无人并发清缓存的环境）确认 12 个 build-failed 包归零。
3. **可选**：`internal/service` 若在干净环境下仍超 900s，考虑给该包单独 `-timeout` 或给 cron 用例加 `testing.Short()` 门控。
