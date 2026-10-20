# Fix Report: 多租户改造遗留的陈旧测试夹具（平台/租户 KV 拆分 + MFA fail-closed）

- **Slug**: stale-test-fixtures-after-tenant-kv-split
- **Fixed**: 2026-10-20
- **Assessment**: [assessment.md](assessment.md)
- **Status**: applied

## Summary

3 个红用例在 HEAD（029ccf7a）上就红，根因是**测试夹具没跟上多租户改造的生产侧变更**，生产代码是对的：

- 登录链对 MFA 状态是 fail-closed 语义（查不出状态就拒登），controller 包夹具 `setupTestControllerDB` 缺 `user_mfa` / `password_history` / `system_config_kv` / `login_events` ⇒ 42P01 被翻译成 401。
- 回调密钥已随平台/租户 KV 拆分迁到 `platform_config_kv`，router 包夹具 `seedWebhookSecret` 仍往租户表 `system_config_kv` 种数据 ⇒ 中间件读不到 ⇒ 合法签名一律 503。

修法：把夹具与契约断言拉回生产事实源，**零生产代码改动**。

## Changes

| 文件 | 改动 |
|------|------|
| `user-server/internal/controller/account_test.go` | `setupTestControllerDB` 建表清单补 `&model.UserMFA{}`、`&model.PasswordHistory{}`、`&model.SystemConfigKV{}`、`&model.LoginEvent{}`（与 HEAD 在 service 包的口径一致） |
| `user-server/internal/router/order_webhook_routes_test.go` | `newOrderWebhookEngine` 建表清单补 `&model.PlatformConfigKV{}`；`seedWebhookSecret` 改写 `model.PlatformConfigKV`（Cleanup 的 Delete 同步改）；`TestOrderWebhook_KVSecretKeyContract` 改用 `repository.NewPlatformConfigKVRepository()`，并新增「平台键在租户表读不到」断言 |
| `user-server/internal/model/platform_config_kv.go` | 仅注释：删掉键空间清单里自相矛盾的 `password_policy` 平台条目，改为一句话说明它留在 `system_config_kv` |

三处都补了中文注释说明**为什么**（fail-closed 语义、平台/租户表归属），避免下次又被当成冗余删掉。

## Diff Highlights

`user-server/internal/controller/account_test.go`：

```diff
 		&model.DouyinCard{},
 		&model.ShortLink{},
 		&model.DomainPool{},
+		// 登录链路会读 MFA 状态与密码策略：任一查询失败即 fail-closed 拒登，
+		// 缺表会让用例红在 42P01 而不是红在认证逻辑上。
+		&model.UserMFA{},
+		&model.PasswordHistory{},
+		&model.SystemConfigKV{},
+		// 登录风险检测异步写登录事件，缺表会把真实失败淹没在 42P01 噪声里。
+		&model.LoginEvent{},
 	)
```

`user-server/internal/router/order_webhook_routes_test.go`：

```diff
 	row := model.SystemConfigKV{Key: key, Value: secret, CreatedAt: time.Now(), UpdatedAt: time.Now()}
+	// 种到平台表：中间件验签走 PlatformConfigKVRepository，写租户表等于没配。
+	row := model.PlatformConfigKV{Key: key, Value: secret, CreatedAt: time.Now(), UpdatedAt: time.Now()}
```

```diff
-	repo := repository.NewSystemConfigKVRepository()
+	// 回调密钥属平台级配置：读写都必须落 platform_config_kv，
+	// 用租户级仓储断言会把已完成的拆分重新打破。
+	repo := repository.NewPlatformConfigKVRepository()
```

```diff
+	// 表归属钉死：同一个键在租户表必须读不到，否则拆分随时可被写回旧表。
+	tenantRepo := repository.NewSystemConfigKVRepository()
+	if tenantHit, err := tenantRepo.Get(context.Background(), key); err != nil || tenantHit != "" {
+		t.Errorf("平台键泄漏到租户表：%q %v", tenantHit, err)
+	}
```

## Tests Added or Updated

- 未新增测试文件。
- `internal/router` 的 `TestOrderWebhook_KVSecretKeyContract` 内新增 1 条断言（平台键在租户表读不到），把「平台/租户 KV 拆分」从口头约定钉成可执行契约。
- 既有 `TestAuthController_Login_Success`、`TestOrderWebhook_EndToEndContract`、`TestOrderWebhook_LegacyPathAnouncesDeprecationAndPointsToLiveRoute` 由红转绿，即回归证据。

## Local Verification

```bash
cd user-server
PW="$(docker exec mtk-postgres printenv POSTGRES_PASSWORD)"
export POSTGRES_TEST_HOST=127.0.0.1 POSTGRES_TEST_PORT=8202 \
       POSTGRES_TEST_USER=admin POSTGRES_TEST_PASSWORD="$PW"
go test ./internal/controller -run 'TestAuthController_Login_Success' -count=1
go test ./internal/router -run 'TestOrderWebhook' -count=1 -v
```

结果：`ok hivemtk-user/internal/controller 1.404s`；router 8 个 `TestOrderWebhook*` 全部 `--- PASS`（`ok hivemtk-user/internal/router 3.280s`）。修复前 3 例红（401 / 503 / 503）。

`gofmt -l` 干净、`go build ./...` 通过。

## Deviations from Assessment

- assessment 建议的「Files likely to change」正是这 3 个文件，未越界。
- assessment 提到「`SystemConfigKVRepository.Get` 缺键返回空串的既有语义需在实现时确认」：已确认 —— `user-server/internal/repository/platform_config_kv.go` 的 `Get` 缺键返回 `("", nil)`，租户表侧同样语义，故新断言写成「命中则报错」而非「缺键必须报错」，避免把语义假设钉死。
- assessment 里 `&model.LoginEvent{}` 的理由（缺表淹没真实失败）保留，同时说明剩余的 `sql: database is closed` 属测试收尾竞态，不在本条修。

## Follow-ups

- 无。生产侧无管理端点写 `platform_config_kv`（`order_webhook_secret_*` 只能靠 SQL/脚本配置）属运维文档缺口，与本缺陷无关，不在本 slug 范围。
