# Bug Assessment: 多租户改造遗留的陈旧测试夹具（平台/租户 KV 拆分 + MFA fail-closed）

- **Slug**: stale-test-fixtures-after-tenant-kv-split
- **Created**: 2026-10-20
- **Source**: 自查（HEAD 029ccf7a 与工作区全量 `go test ./...` 基线对比，3 个用例在 HEAD 即红）
- **Verdict**: valid
- **Severity**: medium

## Report (verbatim or summarized)

用户指令（2026-10-20）：「前置任务一：3 对抗性 检查、审核、测试 整个 saas多租户改造 发现问题立即修复」。

本轮对 SaaS 多租户改造做全量回归时，工作区仍红 3 个用例。用独立 worktree 检出 HEAD（029ccf7a）逐包对比确认：这 3 个用例**在 HEAD 上就红**，不是本轮改动引入。根因是同一类——多租户改造改了生产侧的表归属与失败语义，测试夹具没跟着改，于是用例红在根因上（缺表），而不是红在被测逻辑上。

## Symptom

`go test ./internal/controller ./internal/router` 失败三例：

1. `internal/controller/account_test.go:25 TestAuthController_Login_Success`
   `Expected status OK, got 401, body: {"code":"UNAUTHORIZED_2001",...}`
   日志：`MFA 状态查询失败: ERROR: relation "user_mfa" does not exist (SQLSTATE 42P01)`、
   `查询密码策略失败: ERROR: relation "system_config_kv" does not exist (SQLSTATE 42P01)`
2. `internal/router/order_webhook_routes_test.go TestOrderWebhook_EndToEndContract`
   `合法签名应 200，实际 503：{"code":"SERVICE_UNAVAILABLE_6003","message":"平台 tp2020027000 未配置回调密钥（platform_config_kv 键 order_webhook_secret_tp2020027000）"}`
3. `internal/router/order_webhook_routes_test.go TestOrderWebhook_LegacyPathAnouncesDeprecationAndPointsToLiveRoute`
   同上 503。

附带噪音（非断言失败）：登录风控 goroutine（`user-server/internal/controller/auth.go:86-104`）在测试收尾关闭 DB 之后写 `login_events` ⇒ 日志 `写入登录事件失败: sql: database is closed`（WRN 级）。

## Reproduction

```bash
cd user-server
PW="$(docker exec mtk-postgres printenv POSTGRES_PASSWORD)"
export POSTGRES_TEST_HOST=127.0.0.1 POSTGRES_TEST_PORT=8202 POSTGRES_TEST_USER=admin POSTGRES_TEST_PASSWORD="$PW"
go test ./internal/controller -run 'TestAuthController_Login_Success' -count=1
go test ./internal/router -run 'TestOrderWebhook_(EndToEndContract|LegacyPathAnounces)' -count=1
```

HEAD（029ccf7a）独立 worktree 上同样两命令，同样三例红 ⇒ 与本轮改动无关。

## Suspected Code Paths

**根因 A：登录夹具缺 MFA / 密码策略表**

- `user-server/internal/controller/account_test.go:20-32 setupTestControllerDB`：建表清单只有 `&model.SystemUser{}`、`&model.User{}`、`&model.Account{}`、`&model.DouyinCard{}`、`&model.ShortLink{}`、`&model.DomainPool{}`。
- `user-server/internal/service/auth.go:282 func (s *AuthService) loginWithUser(ctx context.Context, user *model.SystemUser) (*LoginResponse, error)`：`bindUserTenant` 之后调 `mfaSvc.IsMFAEnabled(ctx, user.ID)`，查询失败即 `logger.Errorf("MFA 状态查询失败: %v", err)` 并 fail-closed 拒登。
- `user-server/internal/service/password_policy.go:159-161`：`s.kvRepo.Get(ctx, PolicyKVKey)` 读 `system_config_kv`；失败 `logger.Errorf("查询密码策略失败: %v", err)` 后回落 `DefaultPasswordPolicy`（非致命，但掩盖根因）。
- 先例：HEAD 029ccf7a 已给 `user-server/internal/service/auth_test.go:15 setupAuthServiceTestDB` 补 `&model.UserMFA{}`/`&model.PasswordHistory{}`/`&model.SystemConfigKV{}`、给 `user-server/internal/service/tenant_test.go:29 setupTenantTestDB` 补 `&model.UserMFA{}`，**漏了 controller 包的 `setupTestControllerDB`**（不同包、不同夹具）。

**根因 B：订单回调夹具把平台级密钥写进租户级表**

- 生产读侧：`user-server/internal/middleware/webhook_signature.go:175` `secrets = repository.NewPlatformConfigKVRepository()`，读 `platform_config_kv`（`user-server/internal/model/platform_config_kv.go:29 TableName() = "platform_config_kv"`），键 `order_webhook_secret_<platform>`（`webhook_signature.go:72 webhookSecretKeyPrefix`，`:319 func WebhookSecretKeyFor(platform string) string`）。读失败或未配都 fail-closed ⇒ 503。
- 测试写侧：`user-server/internal/router/order_webhook_routes_test.go:46 func newOrderWebhookEngine(t *testing.T) (*gin.Engine, *gorm.DB)` 建表只列 `&model.SystemConfigKV{}`/`&model.ExternalOrder{}`/`&model.WebhookEvent{}`，缺 `&model.PlatformConfigKV{}`；`:71 func seedWebhookSecret(t *testing.T, database *gorm.DB, platform, secret string)` 写 `model.SystemConfigKV`（`system_config_kv` 表）。
- 契约漂移（不红但有害）：`user-server/internal/router/order_webhook_routes_test.go:373 repo := repository.NewSystemConfigKVRepository()`（`TestOrderWebhook_KVSecretKeyContract`）只断言键名格式、不断言表归属，钉的是已废弃的表。
- 拆分来源：`user-server/internal/migration/migrations/v3_65_0_platform_kv_tenant_split_migration.go:63` 把 `order_webhook_secret_%` 从 `system_config_kv` 迁到 `platform_config_kv`；该迁移由 8c25c8dc「feat(tenancy): R5 配置数据全量租户化」引入。

**不在修复范围（已核实）**

- `user-server/internal/controller/integration_order_webhook_test.go:28 setupOrderWebhookControllerDB` 也建 `&model.SystemConfigKV{}`，但该文件三个用例故意**不挂 guard**（验证"忘挂 guard 必须 403"），不读密钥 ⇒ 不需要 `platform_config_kv`。
- `user-server/internal/model/platform_config_kv.go` 顶部键空间注释把 `password_policy` 列进平台键清单，尾段又说它留在 `system_config_kv`；代码事实（`user-server/internal/service/password_policy.go:86`）是租户级。属注释自相矛盾，单独修（见下）。

## Root Cause Hypothesis

**根因 A（medium）**：夹具与被测链路的表依赖不同步。登录链对 MFA 状态是 fail-closed 语义——查不出状态就拒登，不把"查询失败"当"未启用"。夹具缺 `user_mfa` 时，42P01 被 fail-closed 翻译成 401「登录失败」，用例红在缺表而不是红在认证逻辑，定位成本被人为抬高。同一缺陷类 HEAD 已在 service 包修过，controller 包是漏项。

**根因 B（medium）**：`platform_config_kv` / `system_config_kv` 拆分后，测试夹具仍按旧表归属种数据。写侧种到 `system_config_kv` 而读侧读 `platform_config_kv`，用例 503；更糟的是 `TestOrderWebhook_KVSecretKeyContract` 用租户级仓储断言平台级密钥的键名契约，一旦有人照它写管理端写入路径，就会把密钥写回租户表，等于把已修好的拆分重新打破。

**共同特征**：生产代码正确，测试代码陈旧。修法是让夹具与契约用例回到生产事实源，不改任何生产逻辑。

## Proposed Remediation

### Preferred

两处夹具与一处契约断言对齐生产事实源（纯测试侧改动，零生产代码改动）：

| 文件 | 改动 |
|------|------|
| `user-server/internal/controller/account_test.go` | `setupTestControllerDB` 建表清单补 `&model.UserMFA{}`、`&model.PasswordHistory{}`、`&model.SystemConfigKV{}`、`&model.LoginEvent{}`，与 HEAD 029ccf7a 在 service 包的口径一致 |
| `user-server/internal/router/order_webhook_routes_test.go` | `newOrderWebhookEngine` 建表清单补 `&model.PlatformConfigKV{}`；`seedWebhookSecret` 改写 `model.PlatformConfigKV`（清理用的 Delete 同步改）；`TestOrderWebhook_KVSecretKeyContract` 改用 `repository.NewPlatformConfigKVRepository()`，并新增断言：密钥在平台表可读、在租户表读不到（把"拆分"钉成契约） |
| `user-server/internal/model/platform_config_kv.go` | 删除键空间注释里自相矛盾的 `password_policy` 平台条目（代码事实是租户级 `system_config_kv`），消除事实源漂移 |

`&model.LoginEvent{}` 的理由：登录链的风险检测写 `login_events`，缺表会让真实失败被 42P01 噪声淹没；加上它至少保证"表存在"，剩下的 `sql: database is closed` 是测试收尾竞态，属既有行为，不在本条修。

### Alternatives (optional)

- **改生产代码让登录链路容忍缺表**：**未采纳**。MFA fail-closed 是刻意的安全语义，测试库缺表不是生产场景；为绿灯松安全闸方向错误。
- **改生产代码让中间件回落读 `system_config_kv`**：**未采纳**。那会把平台级密钥重新绑到租户上下文，是多租户隔离的倒退。
- **只补建表、不改契约用例**：**未采纳**。`TestOrderWebhook_KVSecretKeyContract` 现在钉的是已废弃的表，留着就是给未来埋坑。

### Files likely to change

- `user-server/internal/controller/account_test.go`
- `user-server/internal/router/order_webhook_routes_test.go`
- `user-server/internal/model/platform_config_kv.go`（仅注释）

范围限定：只允许动上述三个文件；任何 `internal/service/`、`internal/middleware/`、`internal/repository/` 的生产逻辑都不许动。

### Tests to add or update

- `internal/controller`：既有 `TestAuthController_Login_Success` 修复后应转绿，即为回归证据（无需新增用例）。
- `internal/router`：`TestOrderWebhook_EndToEndContract` 与 `TestOrderWebhook_LegacyPathAnouncesDeprecationAndPointsToLiveRoute` 转绿；`TestOrderWebhook_KVSecretKeyContract` 内新增两条断言（平台表命中 / 租户表不命中）。

## Risks & Considerations

- 加建表会拖慢该包测试（每张表一次 AutoMigrate）。四张表增量可忽略。
- `system_config_kv` 是租户级表，`platform_config_kv` 无 `tenant_id`。测试环境 `TENANT_MODE` 默认 off（`user-server/internal/pkg/tenant/tenant.go:37 EnvMode`），租户闸不介入，两张表的写入都不会被租户回调改写。
- 契约用例新增的"租户表读不到"断言依赖 `SystemConfigKVRepository.Get` 缺键返回空串的既有语义，需在实现时确认，避免把语义假设写成断言。
- 本修法不改变任何生产行为，`make audit` / `make vet` / 迁移链不受影响。

## Open Questions

无。
