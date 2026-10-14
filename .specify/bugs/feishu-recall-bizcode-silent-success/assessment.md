# Bug Assessment: 飞书撤回只看 HTTP 状态码，业务失败被静默判成成功

- **Slug**: feishu-recall-bizcode-silent-success
- **Created**: 2026-10-14
- **Source**: 自查发现（无外部 issue）。审计 `user-server` 消息撤回链路时定位。
- **Verdict**: valid
- **Severity**: high

## Report

自查发现。`internal/app/integration_reach_adapter.go` 的 `recallFeishu` 只检查 HTTP 状态码，没有解析飞书响应体里的业务码 `code`，导致**撤回失败被静默判成成功**。

## Symptom

调用飞书撤回接口后，即使平台以业务码拒绝了这次撤回，`Recall` 也返回 `nil`，上层据此向操作者反馈"撤回成功"，但消息仍留在飞书会话里。预期是：平台拒绝了就要如实返回错误。

## Reproduction

1. 对一条飞书消息发起撤回（正常业务动作）。
2. 满足任一拒绝条件：消息已被撤回过、已超出可撤回时长、机器人不在该会话、`message_id` 非法。
3. 飞书返回 **HTTP 200**，响应体为 `{"code":230002,"msg":"Bad Request"}`。
4. 修复前：`recallFeishu` 的 `if resp.StatusCode != http.StatusOK` 不成立，直接走到 `return nil`。
5. 观察：调用方判定撤回成功。

## Suspected Code Paths

- `user-server/internal/app/integration_reach_adapter.go` — `recallFeishu`（约 L683）：**根因所在**，只判 `resp.StatusCode`
- `user-server/internal/app/integration_reach_adapter.go` — `recallWeCom`（约 L737-745）：对照组，**已**解析 `errcode` 并在非 0 时报错
- `user-server/internal/app/integration_reach_adapter.go` — `getFeishuTenantToken`（约 L808）：对照组，**已**判 `tokenResp.Code != 0`
- `user-server/internal/service/feishu.go:213` / `:321` / `:392`：对照组，三处**均**判 `Code != 0`

## Root Cause Hypothesis

飞书开放平台的成功与失败**都用 HTTP 200 表达**，业务成败只落在响应体的 `code` 字段上：

- 成功：`{"code":0,"msg":"success","data":{...}}`
- 失败：`{"code":230002,"msg":"Bad Request"}`（消息已撤回 / 超过可撤回时长 / 无权限 / `message_id` 非法等一律如此）

只判 `StatusCode` 时，`code=230002` 的失败响应会走完 if、直接 `return nil`。置信度：**high** —— 这不是推测，同文件另外两处飞书写法、以及 `internal/service/feishu.go` 的三处，**全都**判了业务码，`recallFeishu` 是全仓唯一一处没跟上这条既有口径的。方向明确，不是个人风格偏好。

## Proposed Remediation

**Preferred**：把"业务码 → error"这段抽成纯函数（可直接单测，不依赖网络），并在 `recallFeishu` 里接上。口径对齐 `recallWeCom` 与 `internal/service/feishu.go`。

1. 新增纯函数 `feishuRecallAPIError(body []byte) error`：`json.Unmarshal` 出 `{code, msg}`，`code != 0` 即返回 `fmt.Errorf("recallFeishu: 飞书 errcode=%d errmsg=%s", ...)`；解析失败也要报错（网关兜底回 HTML 页面时不能当作成功放行）。
2. `recallFeishu` 改为先 `io.ReadAll(resp.Body)`，再 `return feishuRecallAPIError(body)`。
3. 顺带修掉 `Recall` 上方那段**已与实现矛盾**的注释（L539/L546 写着"本服务没有任何渠道的撤回通道，恒返回错误"、"撤回本身仍未接"，而同文件的 `recallTelegram`/`recallFeishu`/`recallWeCom` 均已实现）。

**Alternatives**：
- 直接在 `recallFeishu` 里内联 `json.Unmarshal` + 判 `code`。改动更小，但这条判定就绑死在网络路径上，没法不起网络就单测——而"能否单测"恰是本 bug 的核心防线。不推荐。

**Files likely to change**:
- `user-server/internal/app/integration_reach_adapter.go`
- `user-server/internal/app/recall_feishu_bizcode_test.go`（新增）

**Tests to add or update**:
- `code = 0` → 必须成功
- `code != 0` 且 **HTTP 200** → 必须报错（这条正是钉住原 bug 的用例）
- 常见拒绝码（230013 无权限 / 230020 已撤回 / 230002 不存在）→ 都判失败
- 响应体非合法 JSON（如 `<html>502</html>`）→ 报错，不得放行

## Risks & Considerations

- **行为变化**：仅在飞书返回非 0 业务码时，撤回从"假成功"变为"如实报错"。成功路径（`code=0`）与 HTTP 非 200 路径行为不变。这是**收紧**，但对既有调用方而言，错误分支会真实被触发过一次——调用方需能处理 `Recall` 返回错误（需在修复后确认调用链上层的处理是完整的）。
- 无 API 契约变更、无迁移、无性能影响（多一次 `ReadAll`，可忽略）。
- 纯函数化是本次修复的关键收益：把网络 I/O 与业务判定分离，判定逻辑可脱离网络/DB 单测。

## Open Questions

- 无。拒绝条件、错误码语义均已在 `internal/service/feishu.go` 既有代码中得到印证。
