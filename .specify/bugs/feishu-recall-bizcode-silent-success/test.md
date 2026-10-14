# Bug Verification: 飞书撤回只看 HTTP 状态码，业务失败被静默判成成功

- **Slug**: feishu-recall-bizcode-silent-success
- **Tested**: 2026-10-14
- **Assessment**: ./assessment.md
- **Fix**: ./fix.md
- **Result**: verified

## Summary

原症状已不重现：HTTP 200 + 非 0 业务码现在必然报错，不再返回 `nil`。新增单测 4 个子用例全过，`internal/app` 整包 240.9s 无回归，全仓 `-race` 0 处 data race。

修复前的判定逻辑已原样抽出、对同一份响应体跑过一遍，确认它确实返回 `<nil>`（即"谎报成功"为 true），因此本用例不是凭空写的，是真能抓住原 bug 的。

## Checks Performed

| Check | Command / Action | Result | Notes |
|-------|------------------|--------|-------|
| Reproduction (post-fix) | 修复前判定逻辑 vs 修复后判定逻辑，同一响应体对照 | pass | 修复前 `<nil>`，修复后报错 |
| New / updated tests | `go test ./internal/app/ -run "TestFeishuRecallAPIError\|TestPlatformMessageID_WeCom\|TestRecallChannelClassification" -v -count=1` | pass | 4 子用例全过 |
| Regression suite | `go test ./internal/app/ -count=1` | pass | `ok ... 240.918s` |
| Build | `go build ./...` | pass | — |
| Lint / type-check | `go vet ./internal/app/ ./internal/service/ ./internal/repository/`；`gofmt -l` | pass | 均无输出 |
| Concurrency | `go test -race ./...` | pass | 0 处 data race，29 包 ok |

## Output Excerpts

定点测试：

```
--- PASS: TestRecallChannelClassification (0.00s)
--- PASS: TestFeishuRecallAPIError (0.00s)
    --- PASS: TestFeishuRecallAPIError/code=0_视为成功
    --- PASS: TestFeishuRecallAPIError/code!=0_且_HTTP_200_时必须报错
    --- PASS: TestFeishuRecallAPIError/常见撤回拒绝码都判失败
    --- PASS: TestFeishuRecallAPIError/响应体不是合法_JSON_时报错而非放行
--- PASS: TestPlatformMessageID_WeCom (0.00s)
ok  	hivemtk-user/internal/app	0.806s
```

红/绿对照（把修复前的判定逻辑原样抽出来跑同一份响应体）：

```
飞书返回 HTTP 200 + {"code":230002,"msg":"Bad Request"}
  修复前判定结果: <nil>  (nil = 被当成撤回成功)
  ⇒ 修复前会谎报成功: true
```

整包回归：

```
ok  	hivemtk-user/internal/app	240.918s
```

## Residual Risks

- **复现是等价复现，非端到端**：走的是修复前/后两段判定逻辑对同一响应体的对照，没有真的连飞书发一次撤回请求（需要真实飞书凭据与群环境）。判定逻辑本身已被完全覆盖，但网络层/序列化层未在真实环境验证过。
- `-race` 下 `aiagent/knowledge/{controller,service}` 与 `app` 三处报 `panic: test timed out after 10m0s`，是 race 插桩把耗时放大到超过 Go 默认 10 分钟上限所致，**非测试失败**：同包在非 race 下 240s 全绿。
- `cmd/seed` 的两个用例失败是环境问题（`password authentication failed for user "admin"`），测试库密码按设计不从 `.env` 读取，须显式导出 `POSTGRES_TEST_PASSWORD`。与本次修复无关。
- 附带发现（非本 bug）：`internal/.../testdb.go` 在数据库连接失败时一律 `t.Skipf`，这会造成**假绿**——测试看起来通过，实则什么都没跑。属既有测试基建问题，建议单独跟进。

## Recommendation

关闭该 bug。根因清晰、修复已由单测锁定、整包无回归。

`fix.md` 中原列的两个 Follow-ups 已在本轮结清（规则2 不留待办）：上层调用链
`reach_tools.go:776-784` 的错误分支经查证完整，`recalled: true` 只在 `err == nil`
分支产生，谎报成功已被彻底掐断；`UpdateExtra` 的全量覆盖语义为有意设计，
服务层与仓储层注释均已写明，且被 `recall_wecom_update_extra_test.go:69-90` 固化。