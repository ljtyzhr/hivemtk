# b2-recall-api 验证报告

## 验证内容
- 编译通过
- 单元测试通过
- 集成测试通过（企微 msgid 回填链路）

## 验证结果
- ✅ `go build ./...` 通过
- ✅ `go test ./internal/app/ -run 'TestPlatformMessageID_WeCom'` 通过 (0.820s)
- ✅ `go test ./internal/app/ -run 'TestMessageHubUpdateExtra_Integration'` 通过 (1.224s)
- ✅ `go test ./internal/router/ ./internal/middleware/ -count=1` 通过 (router 20.766s / middleware 3.037s)
- ✅ recall 测试更新：从 `ErrRecallNotImplemented`（stub）改为 `ErrIntegrationServiceNotConfigured`（service nil 哨兵）

## 测试更新
- `recall_channel_classification_test.go` — 子测试标题和断言已更新
  - 旧：`"平台撤回实现尚未接通，三家也报不支持"` → 期望 `ErrRecallNotSupported`
  - 新：`"平台撤回实现已接通，三家在 service 为 nil 时返回未配置"` → 期望 `ErrIntegrationServiceNotConfigured`
- `recall_wecom_msgid_test.go` — 新增 `TestPlatformMessageID_WeCom`（4 腿：有 msgid / nil / 空串 / 类型错误）
- `recall_wecom_update_extra_test.go` — 新增 `TestMessageHubUpdateExtra_Integration`（3 腿：写入读出 / 全量覆盖 / id=0 空操作）

## 后续行动
- 真平台撤回需要真实账号凭证（TG BotToken / 飞书 AppSecret / 企微 CorpSecret）
- 建议在 staging 环境做一次端到端撤回验证
