# b2-recall-api 修复记录

## 实现内容
实现了三个平台的主动撤回 API 调用：

### 1. recallTelegram — Telegram deleteMessage
- 从 TelegramAccount 表获取 BotToken
- 从 hub key `tg-out-{accountID}-{平台消息号}` 解析平台消息 ID
- 调用 `POST https://api.telegram.org/bot{token}/deleteMessage`
- Body: `{"chat_id": ..., "message_id": ...}`

### 2. recallFeishu — 飞书撤回消息
- 从 FeishuAccount 表获取 AppID/AppSecret
- 获取 tenant_access_token（带缓存，过期前 300s 刷新）
- 从 hub key `feishu-out-{accountID}-{平台message_id}` 解析平台消息 ID
- 调用 `DELETE https://open.feishu.cn/open-apis/im/v1/messages/{message_id}`
- Headers: `Authorization: Bearer {token}`

### 3. recallWeCom — 企微撤回消息
- 从 WeComAccount 表获取 CorpID/CorpSecret
- 获取 access_token（带缓存，过期前 300s 刷新）
- 从 `hub.Extra["msgid"]` 获取平台消息 ID
- 调用 `POST https://qyapi.weixin.qq.com/cgi-bin/message/recall?access_token={token}`
- Body: `{"msgid": ...}`
- 检查 errcode != 0 时报错

### 辅助函数
- `platformMessageID(hub)` — 从 hub 行反查平台消息标识
- `getFeishuTenantToken(ctx, acc)` — 飞书 token 获取（带缓存）
- `getWeComAccessToken(ctx, acc)` — 企微 token 获取（带缓存）

## 错误处理
- service 为 nil → `ErrIntegrationServiceNotConfigured`
- hub key 格式错误 → 返回格式错误
- 平台 API 返回非 200 → 返回状态码和响应体
- 企微 errcode != 0 → 返回 errcode 和 errmsg

### 4. 企微发送侧改造 — 回填平台 msgid 到 hub Extra
- `internal/service/wecom_integration.go` — SendMessage 后把 `result.MsgID` 写入 hub 行 `Extra["msgid"]`
- `internal/service/message_hub.go` — 新增 `UpdateExtra(ctx, id, extra)` 方法
- `internal/repository/message_hub_inbox_message.go` — 新增 `UpdateExtra(ctx, id, extra)` repo 方法
- 旧消息无此字段 → 撤回返回 `"wecom hub extra missing msgid"`（预期行为，已注释说明）

## 影响范围
- `internal/app/integration_reach_adapter.go` — 新增 import 和 5 个函数
- `internal/service/wecom_integration.go` — 企微发送侧回填 msgid
- `internal/service/message_hub.go` — 新增 UpdateExtra 方法
- `internal/repository/message_hub_inbox_message.go` — 新增 UpdateExtra repo 方法
- `internal/app/recall_channel_classification_test.go` — 更新测试断言
- `internal/app/recall_wecom_msgid_test.go` — 新增 platformMessageID 企微分支测试
- `internal/app/recall_wecom_update_extra_test.go` — 新增 UpdateExtra 集成测试
