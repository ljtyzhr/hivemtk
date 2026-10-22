# Test Report: 平台 provider 同步落库被多租户隔离 fail-closed 拦截

- **Slug**: platform-provider-sync-tenant-isolation
- **Tested**: 2026-10-22
- **Fix**: [fix.md](fix.md)
- **Assessment**: [assessment.md](assessment.md)
- **Result**: passed

## Environment

| 项 | 值 |
|---|---|
| 测试库 | `127.0.0.1:8202`（本地 `mtk-postgres` 容器）/ `user_db_test`（本次新建） |
| 口令 | `POSTGRES_TEST_PASSWORD` 取自 `docker exec mtk-postgres printenv POSTGRES_PASSWORD` |
| 串行 | `-p 1`（平台端各测试包共享同一个测试库，必须串行） |
| 被测 commit | `4f2eb433` |

## 复现（第 2 步：红必须先在场）

把守卫全部摘除（= 完整还原生产现场：调用方 ctx + 不写 TenantID + `ON CONFLICT (name)`），
`TestSavePlatformProviders_OnModeWithoutTenantCtx` 红，读数原文：

```
[llm-provider] 平台 provider 落库失败 platform-sensenova: [tenant] 表 llm_providers 需要租户上下文但 ctx 未绑定（create）；多租户模式下拒绝执行，请经中间件/TenantRunner/行自证提供租户
[llm-provider] 平台下发 provider 同步完成：0 条
    llm_platform_provider_sync_test.go:48: 应同步 1 条平台 provider，实际 0
--- FAIL: TestSavePlatformProviders_OnModeWithoutTenantCtx (5.01s)
```

红因文案与生产日志**逐字一致**（`需要租户上下文但 ctx 未绑定（create）`）。

## 修复后读数（第 5 步：只读不改源码）

命令：

```bash
POSTGRES_TEST_PORT=8202 POSTGRES_TEST_USER=admin POSTGRES_TEST_PASSWORD=<...> \
  go test -p 1 ./internal/service/ -run "TestSavePlatformProviders" -v
```

```
--- PASS: TestSavePlatformProviders_OnModeWithoutTenantCtx (7.15s)
--- PASS: TestSavePlatformProviders_ExplicitTenantIDSelfAsserts (0.22s)
--- PASS: TestSavePlatformProviders_OnConflictTargetMatchesIndex (0.12s)
--- PASS: TestSavePlatformProviders_KeepsMerchantAPIKey (0.12s)
PASS
ok  	hivemtk-user/internal/service	8.753s
```

四个用例各自钉住的不变量：

| 用例 | 钉住什么 |
|---|---|
| `OnModeWithoutTenantCtx` | 装配期（无租户 ctx）同步必须落库成功；行归属默认租户；`source=platform`；`plan_code` 保真 |
| `ExplicitTenantIDSelfAsserts` | 不写 `tenant_id` 的装配期写入必须被隔离 fail-closed 拒绝，且**红因文案**与生产一致；带显式 `tenant_id` 的行自证写入放行 |
| `OnConflictTargetMatchesIndex` | 冲突目标必须匹配 `(tenant_id, name)`（否则 42P10）；重复同步只有 1 行且元信息被更新 |
| `KeepsMerchantAPIKey` | 同名下发**不得清空**商户自带的 `api_key`（provider 从 platform 转回 self 的前提） |

## 守卫反向验证（第 4 步）

见 fix.md「Guard Reverse Verification」表。逐项摘除后的读数：

| 摘除的守卫 | 读数 |
|---|---|
| 全部三条 | **红**，原因为生产红因原文（见上「复现」段） |
| 仅 system scope | 绿（显式 tenant_id 走行自证放行） |
| 仅显式 tenant_id | 绿（system scope 是装配层豁免通道） |
| 仅复合冲突目标 | **红**，`ERROR: there is no unique or exclusion constraint matching the ON CONFLICT specification (SQLSTATE 42P10)` |

md5 双向对账：

| 对象 | md5 |
|---|---|
| 修复后工作区（最终态） | `85a9f8572cf56203ad4ecbc31e68702e` |
| HEAD blob（未改前） | `2e08039dc1f3048bf35cef758593f9dd` |
| 注码前基线（注码时记录的修复态） | `a50352a8d4dd40ae3435a67a4c6523ba` |

`a50352a8` → `85a9f857` 的差异**只有注释**（`diff <(grep -v "^//" A) <(grep -v "^//" B)` 输出为空）。

## 受影响包回归

```bash
go build ./...                    # BUILD_OK
go test -p 1 ./internal/service/ ./internal/aiagent/llm/ ./internal/pkg/tenancyidx/
```

读数：

```
ok  	hivemtk-user/internal/aiagent/llm	29.501s
ok  	hivemtk-user/internal/pkg/tenancyidx	(cached)
FAIL	hivemtk-user/internal/service	601.106s
--- FAIL: TestModuleSyncExemptsGiftRowsFromStaleDisable (0.32s)
    module_gate_tenant_test.go:231: purchase 行应照常被 stale-disable：approval 状态应为 disabled，实际 "active"
--- FAIL: TestModuleSyncProvisionsFreeModulesForRequestingTenant (0.26s)
    module_gate_tenant_test.go:491: 赠送行来源应为 gift，实际 "purchase"
    module_gate_tenant_test.go:499: 租户 18 未下单却拿到了收费模块 browser-automation（source=purchase）
    module_gate_tenant_test.go:507: 默认租户应保留商户级 owned 的 browser-automation 行
    module_gate_tenant_test.go:517: 未下单的收费模块不应放行
```

**这两条是既有红，不是本次引入。** 归属判定：在干净 worktree
（`git worktree add /tmp/hivemtk-baseline-check HEAD --detach`，HEAD=`c2ed9674`）
上跑同样两个用例，**同样红、同样文案**：

```
--- FAIL: TestModuleSyncExemptsGiftRowsFromStaleDisable (14.09s)
    module_gate_tenant_test.go:231: purchase 行应照常被 stale-disable：approval 状态应为 disabled，实际 "active"
--- FAIL: TestModuleSyncProvisionsFreeModulesForRequestingTenant (0.36s)
    module_gate_tenant_test.go:491: 赠送行来源应为 gift，实际 "purchase"
```

判据：本次改动只碰 `llm_platform_usage.go`（provider 同步落库）与新增测试文件；
这两条红在 `module_gate_tenant_test.go`（模块闸/赠送行），与 provider 同步无关，
且在**未含本次改动的基线上同样复现** ⇒ 不在本次修复范围，另开卡处理。

## 生产验证

平台端（已生效，2026-10-22 实测）：

- `llm_provider_bindings` 新增 `platform-sensenova`
  （`base_url=https://token.sensenova.cn/v1`、`model=deepseek-v4-flash`、
  `api_type=openai`、`vendor=sensenova`、`plan_code=starter`、
  `deliver=true`、`enabled=true`）。
- `GET /merchant-api/llm/providers` 返回该条（**不含 API Key**）：
  `{"provider_name":"platform-sensenova",...,"source":"platform"}`。
- 代理转发实测通：`POST /llm-api/v1/chat/completions?provider=platform-sensenova&tenant_code=<租户>`
  → 200，厂商返回标准 OpenAI 格式 `usage`。
- 计量与扣费实测：`llm_usage_records` 落 `plan_code=starter`、
  `prompt_tokens=2283`、`input_cost_cents=2`（整数分向下取整：2283/1000=2.28→2）、
  `output_cost_cents=0`（64 token 不足 1k 粒度）；钱包余额同步减少
  （`input_tokens_remaining` 9999824→9997514、`total_consumed_input` 176→2315）。
- 发放前的赠送额度可实际消耗（赠送余额 10M/2M，调用后余额下降）⇒
  「赠送 token 无法使用」这一阻断点已解除。

**待发版后复核**：商户侧 `llm_providers` 出现 `platform-sensenova` 行
（同步在 user-server 启动期执行，需本修复上线后才生效）。

## Residual Risks

- 本次修复**未发版到线上**（user-server 侧）。平台侧 provider 绑定已上线，
  商户侧同步仍被隔离拦截，直到本修复的二进制上线并重启 user-server。
  ⇒ 线上此刻平台垫资通道**仍不可用**，商户看到的仍是「同步 0 条」。
- 发版须交叉编译 + 重建镜像（本地 darwin/arm64 二进制在容器里会 exec format error；
  只 `docker compose up -d` 不改镜像时会跑旧二进制）。
- `TestModuleSyncExemptsGiftRowsFromStaleDisable` /
  `TestModuleSyncProvisionsFreeModulesForRequestingTenant` 两条既有红
  属模块闸/赠送行口径（`source` 应为 gift 实为 purchase），不在本次范围，
  但会让 `make audit` 与 CI 变红 —— 需另开卡。
- 平台端 provider 的 `api_key` 在平台库**明文存储**（平台库无加密层，
  与商户端 `enc:v1:` 密文不同）。本次沿用现状未改加密策略；
  若后续要加密平台侧密钥，需另评估 `llm_provider_bindings.api_key` 的迁移。
