# Bug Test: 多租户改造遗留陈旧测试夹具的回归验证

- **Slug**: stale-test-fixtures-after-tenant-kv-split
- **Tested**: 2026-10-20
- **Fix**: [fix.md](fix.md)
- **Result**: verified
- **Status**: verified

## Verification Commands

### 1. 登录用例（根因 A：MFA fail-closed 缺表）

```bash
cd user-server
PW="$(docker exec mtk-postgres printenv POSTGRES_PASSWORD)"
export POSTGRES_TEST_HOST=127.0.0.1 POSTGRES_TEST_PORT=8202 \
       POSTGRES_TEST_USER=admin POSTGRES_TEST_PASSWORD="$PW"
go test ./internal/controller -run 'TestAuthController_Login_Success' -count=1
```

**修复前**：`FAIL` — `Expected status OK, got 401, body: {"code":"UNAUTHORIZED_2001",...}`，
日志 `MFA 状态查询失败: ERROR: relation "user_mfa" does not exist (SQLSTATE 42P01)`、
`查询密码策略失败: ERROR: relation "system_config_kv" does not exist (SQLSTATE 42P01)`。

**修复后**：`ok hivemtk-user/internal/controller 1.404s` ✅

### 2. 订单回调用例（根因 B：平台/租户 KV 拆分）

```bash
go test ./internal/router -run 'TestOrderWebhook' -count=1 -v
```

**修复前**：`TestOrderWebhook_EndToEndContract` 与 `TestOrderWebhook_LegacyPathAnouncesDeprecationAndPointsToLiveRoute` 红，
报 `合法签名应 200，实际 503：{"code":"SERVICE_UNAVAILABLE_6003","message":"平台 tp2020027000 未配置回调密钥（platform_config_kv 键 order_webhook_secret_tp2020027000）"}`。

**修复后**（8 例全 PASS）：

```
--- PASS: TestOrderWebhook_BothPathsRegistered (1.17s)
--- PASS: TestOrderWebhook_RegistrationDoesNotPanic (0.14s)
--- PASS: TestOrderWebhook_PublicPathSkipsAuthButVerifies (0.16s)
--- PASS: TestOrderWebhook_LegacyPathStillRequiresAuth (0.18s)
--- PASS: TestOrderWebhook_EndToEndContract (0.19s)
--- PASS: TestOrderWebhook_UnconfiguredPlatformFailsClosed (0.23s)
--- PASS: TestOrderWebhook_LegacyPathAnouncesDeprecationAndPointsToLiveRoute (0.19s)
--- PASS: TestOrderWebhook_KVSecretKeyContract (0.16s)
ok  	hivemtk-user/internal/router	3.259s
```

### 3. 静态确认：夹具与生产事实源对齐

```bash
cd user-server
grep -n "UserMFA\|PasswordHistory\|SystemConfigKV\|LoginEvent" internal/controller/account_test.go
grep -n "PlatformConfigKV\|SystemConfigKV" internal/router/order_webhook_routes_test.go
grep -n "password_policy" internal/model/platform_config_kv.go
```

**结果**：

| 位置 | 期望 | 实测 |
|------|------|------|
| `account_test.go` 建表清单 | 含 `UserMFA` / `PasswordHistory` / `SystemConfigKV` / `LoginEvent` | ✅ 全在 |
| `order_webhook_routes_test.go` 建表清单 | 含 `PlatformConfigKV` | ✅ 在 |
| `seedWebhookSecret` 写侧 | 写 `model.PlatformConfigKV` | ✅ 已改 |
| `TestOrderWebhook_KVSecretKeyContract` 读侧 | `repository.NewPlatformConfigKVRepository()` | ✅ 已改，并新增租户表不命中断言 |
| `platform_config_kv.go` 键空间注释 | 不再把 `password_policy` 列进平台键 | ✅ 已删（仅注释改动） |

### 4. 编译与格式

```bash
cd user-server && go build ./... && gofmt -l internal/controller internal/router internal/model
```

**结果**：`BUILD_OK`；`gofmt -l` 输出为空 ✅

## Test Results

| 检查项 | 结果 |
|--------|------|
| `TestAuthController_Login_Success` 转绿 | ✅ `ok 1.404s` |
| `TestOrderWebhook_EndToEndContract` 转绿 | ✅ PASS |
| `TestOrderWebhook_LegacyPathAnouncesDeprecationAndPointsToLiveRoute` 转绿 | ✅ PASS |
| `TestOrderWebhook_KVSecretKeyContract` 转绿 + 新增「平台键不落租户表」断言 | ✅ PASS |
| router 包 `TestOrderWebhook*` 8 例全 PASS | ✅ |
| 生产代码零改动（仅 3 个文件：2 个测试 + 1 个注释） | ✅ diff 核对 |
| `go build ./...` + `gofmt -l` | ✅ 干净 |

## Conclusion

3 个 HEAD 即红的用例全部转绿，且**没有改任何生产逻辑**：登录侧补齐 MFA / 密码策略 / 登录事件的建表清单，回调侧把密钥的写侧与读侧一起迁回平台表 `platform_config_kv`，并把「平台键不得落租户表」钉成可执行断言。生产侧「MFA fail-closed」「平台级密钥读平台表」两条语义原样保留，未为绿灯松安全闸。
