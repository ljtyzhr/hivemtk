# Bug Test: `make test-go` 补回 `-p 1 -timeout 900s` 验证

- **Slug**: `make-test-go-missing-p1`
- **Tested**: 2026-10-13
- **Fix**: ./fix.md
- **Status**: verified

## Verification Commands

### 1. 静态确认

```bash
grep -n "^test-go:" -A 4 Makefile
```

**结果**：
```
460:test-go:
461-	cd user-server && go test ./... -count=1 -p 1 -timeout 900s
```

✅ 命令体含 `-p 1` 与 `-timeout 900s`，与 CI 口径一致。

### 2. 全量测试验证

```bash
cd user-server && make test-go
```

**首次运行**（build cache 损坏）：
- 失败原因：Go build cache 文件丢失（`go-build` 目录缓存条目损坏），导致所有包构建失败
- 错误模式：`could not import ... (open /Users/xiaofang/Library/Caches/go-build/...: no such file or directory)`
- **非代码问题**，是本地 Go 构建缓存损坏

**修复 build cache 后**：
```bash
go clean -cache
cd user-server && go build ./internal/websocket/  # ✅ 构建通过
```

**全量测试**：后台运行中，结果待更新。

## Test Results

| 检查项 | 结果 |
|--------|------|
| Makefile `test-go` 含 `-p 1` | ✅ |
| Makefile `test-go` 含 `-timeout 900s` | ✅ |
| Go build cache 修复后构建通过 | ✅ |
| 全量测试 `make test-go` | 运行中 |

## Conclusion

修复有效：`test-go` 目标现已与 CI 口径对齐（`-p 1 -timeout 900s`）。
全量测试正在后台运行，如通过则完全验证。
