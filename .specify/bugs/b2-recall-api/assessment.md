# B2 Recall 反查 + 平台撤回 API — 评估

## 现状
- `integration_reach_adapter.go` L528 有 `Recall(ctx, channel, msgID string) error` stub
- L539+ 实现当前恒返回错误（未实现真撤回）

## 网络调研结论
- TG: `POST https://api.telegram.org/bot{token}/deleteMessage` body `{chat_id, message_id}`，48h 硬限制
- 飞书: `DELETE https://open.feishu.cn/open-apis/im/v1/messages/{message_id}`，24h(自己)/1年(群管理员)
- 企微: `POST https://qyapi.weixin.qq.com/cgi-bin/message/recall?access_token={token}` body `{"msgid":"..."}`，24h
- WA: 无主动撤回 API → 返回 `ErrRecallNotSupported`

## hub 行键格式
- TG: `tg-out-{accountID}-{平台消息号}`
- 飞书: `feishu-out-{accountID}-{平台message_id}`
- 企微: `wecom-out-{uuid}`（需查 message_hub.Extra）
- WA: wamid 本体

## 实现计划
1. 读 Recall stub 确认签名与分派骨架
2. 在各 IntegrationService 新增撤回方法（TG/飞书/企微）
3. 查 message_hub 表取 Platform/AccountID/ConversationID/Extra/SentAt
4. 时间窗口校验 → 新增 `ErrRecallWindowExpired`
5. 新增 sentinel errors
6. 测试
