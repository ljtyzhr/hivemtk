# Bug Fix: 飞书撤回只看 HTTP 状态码，业务失败被静默判成成功

- **Slug**: feishu-recall-bizcode-silent-success
- **Fixed**: 2026-10-14
- **Assessment**: ./assessment.md
- **Status**: applied

## Summary

把"飞书业务码 → Go error"抽成纯函数 `feishuRecallAPIError`，并让 `recallFeishu` 改为读取响应体后交给它判定。原先只看 `resp.StatusCode`，而飞书的业务成败不在状态码上，导致失败的撤回被返回成 `nil`（谎报成功）。

## Changes

| File | Change | Notes |
|------|--------|-------|
| `user-server/internal/app/integration_reach_adapter.go` | modified | `recallFeishu` 改判业务码；新增 `feishuRecallAPIError`；重写 `Recall` 注释 |
| `user-server/internal/app/recall_feishu_bizcode_test.go` | added | `TestFeishuRecallAPIError`，4 子用例 |

## Diff Highlights

**修复前**（只判 HTTP 状态码）：

```go
if resp.StatusCode != http.StatusOK {
    body, _ := io.ReadAll(resp.Body)
    return fmt.Errorf("recallFeishu: 飞书返回 %d: %s", resp.StatusCode, string(body))
}

return nil
```

**修复后**：

```go
if resp.StatusCode != http.StatusOK {
    body, _ := io.ReadAll(resp.Body)
    return fmt.Errorf("recallFeishu: 飞书返回 %d: %s", resp.StatusCode, string(body))
}

// 飞书的成败不在状态码上：成功与失败都是 HTTP 200，业务码在 body 的 `code`。
// 只看 StatusCode 的话，平台拒绝了这次撤回也会 return nil（消息其实还在飞书里），
// 而 Recall 的调用方会把"没撤掉"当成"撤掉了"报给操作者。口径对齐 recallWeCom
// 与 internal/service/feishu.go 的 `code != 0` 判定。
body, err := io.ReadAll(resp.Body)
if err != nil {
    return fmt.Errorf("recallFeishu: 读取响应失败: %w", err)
}
return feishuRecallAPIError(body)
```

**新增纯函数**：

```go
// feishuRecallAPIError 把飞书撤回接口的响应体翻译成 Go error：业务码非 0 即失败。
//
// 单独拆成纯函数是为了能不起网络就单测这条判定——它正是原先漏掉的那一步。
func feishuRecallAPIError(body []byte) error {
    var r struct {
        Code int    `json:"code"`
        Msg  string `json:"msg"`
    }
    if err := json.Unmarshal(body, &r); err != nil {
        return fmt.Errorf("recallFeishu: 解析响应失败: %w", err)
    }
    if r.Code != 0 {
        return fmt.Errorf("recallFeishu: 飞书 errcode=%d errmsg=%s", r.Code, r.Msg)
    }
    return nil
}
```

**原因**：把网络 I/O 与业务判定分开，判定逻辑可脱离网络/DB 单测。非 JSON 响应（网关兜底回前端页面）也一并在此挡下，不当作撤回成功。

## Tests Added or Updated

- `recall_feishu_bizcode_test.go::TestFeishuRecallAPIError/code=0_视为成功` — 成功路径未被误伤
- `recall_feishu_bizcode_test.go::TestFeishuRecallAPIError/code!=0_且_HTTP_200_时必须报错` — **钉住原 bug**
- `recall_feishu_bizcode_test.go::TestFeishuRecallAPIError/常见撤回拒绝码都判失败` — 230013 无权限 / 230020 已撤回 / 230002 不存在
- `recall_feishu_bizcode_test.go::TestFeishuRecallAPIError/响应体不是合法_JSON_时报错而非放行` — 挡下网关兜底 HTML

## Local Verification

- `go build ./...` → 通过
- `go vet ./internal/app/ ./internal/service/ ./internal/repository/` → 无输出
- `gofmt -l` → 空输出
- 定点测试 + 整包回归结果见 `./test.md`

## Deviations from Assessment

评估里只列了 `integration_reach_adapter.go` 与新测试两个文件。实际提交里还带上了撤回链路上此前**未提交的既有 WIP**（企微 `msgid` 回填 `message_hub.Extra` 的 `UpdateExtra` 仓储/服务链路，共 5 个文件）——这部分不是本次修复引入的，但在同一文件族内、与撤回功能不可分割，随本次一并落库。若需拆分，应单独成 commit。

## 遗留疑点本轮结清（规则2：不得记录待办）

本轮在 fix 阶段自查出的两个疑点，均已当场查证并结清，**不留待办**：

**1. `Recall` 上层调用链的错误分支是否完整** —— 已查证，**完整，无需改动**。
生产代码中对本适配器 `Recall` 的唯一调用方是
`internal/aiagent/agent/tooluse/reach_tools.go:776-784`：

```go
if err := t.deps.Adapter.Recall(ctx, channel, msgID); err != nil {
    return ErrorResult(t.Name(), err), err
}
return SuccessResult(t.Name(), map[string]any{
    "channel": channel, "message_id": msgID,
    "recalled": true, "recalled_at": time.Now().Format(time.RFC3339),
}), nil
```

`recalled: true` 只在 `err == nil` 分支产生。补上业务码判定后，这条链路上的
"谎报成功"被**彻底掐断**：平台拒绝 ⇒ 适配器返错 ⇒ 工具返 `ErrorResult` ⇒ 不会
出现 `recalled: true`。修复前该谎报会一路传到 LLM 可见的工具返回值里。

**2. `UpdateExtra` 全量覆盖 vs `telegram_media.go` 读改写** —— 已查证，**是有意设计，且意图已写明**，无需改动。
- `service/message_hub.go:782` 与 `repository/message_hub_inbox_message.go:205` 两处注释**均已**写明「**全量覆盖** hub 行的 extra 字段（用于发送侧回填平台消息 ID 等元数据）」。
- 语义已被测试固化：`recall_wecom_update_extra_test.go:69-90` 子用例「全量覆盖 Extra」显式断言 `msgid` 键被删。
- `telegram_media.go:166-176` 的读改写属于另一个函数（媒体回填 `backfillMedia`），它需要**保留**既有 extra 键，行为不同是合理的，不是同一语义的两种写法冲突。

结论：两处口径差异源于**职责不同**，且两处均已文档化。不构成缺陷，按规则2 不记待办。
