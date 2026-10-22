# Bug Assessment: 平台 provider 同步落库被多租户隔离 fail-closed 拦截

- **Slug**: platform-provider-sync-tenant-isolation
- **Created**: 2026-10-21T23:34:14Z
- **Source**: 生产日志实证（用户报修）
- **Verdict**: valid
- **Severity**: high

## Report (verbatim or summarized)

生产环境（`manageapi.xapptool.cn` / `workapi.xapptool.cn`）user-server 启动日志：

```
{"level":"warn","component":"user-server","message":"[llm-provider] 平台 provider 落库失败 platform-sensenova: [tenant] 表 llm_providers 需要租户上下文但 ctx 未绑定（create）；多租户模式下拒绝执行，请经中间件/TenantRunner/行自证提供租户"}
{"level":"info","component":"user-server","message":"[llm-provider] 平台下发 provider 同步完成：0 条"}
```

同一时刻平台端 `GET /merchant-api/llm/providers` 明确返回了 1 条下发记录
（`platform-sensenova`），而商户侧 `llm_providers` 表仍只有 2 条自带 Key 的
`source=self` 行，下发的平台 provider 始终没有落地。

## Symptom

`TENANT_MODE=on`（SaaS 多租户）下，user-server 启动期的「平台 provider 同步」
把平台下发的 provider 落库时，被多租户隔离 callback fail-closed 拒绝，
结果是**平台下发的 provider 永远同步不到商户端**；商户侧 `source=platform`
的 provider 不存在，平台垫资的 LLM 通道对所有租户不可用
（新商户开户赠送的 LLM token 因此无法实际消耗）。

期望：启动期平台级装配操作能走系统作用域豁免通道落库成功，
同时**不放宽**任何带请求 ctx 的 HTTP 路径的租户隔离。

## Reproduction

1. 部署 user-server，`TENANT_MODE=on`（生产实测：`PLATFORM_ENABLED=true`，
   租户表 `llm_providers` 已声明 `TenantID`，属隔离覆盖表）。
2. 平台端 `llm_provider_bindings` 存在 `deliver=true, enabled=true` 的下发记录
   （线上实测：`platform-sensenova`）。
3. 重启 user-server 容器 → 启动日志出现
   `[llm-provider] 平台 provider 落库失败 <name>: [tenant] 表 llm_providers 需要租户上下文但 ctx 未绑定（create）`
   且 `平台下发 provider 同步完成：0 条`。
4. 查商户库 `llm_providers`：平台下发的 provider 不存在。

生产实测读数（2026-10-22，`101.35.252.112`）：

```
平台端 llm_provider_bindings: platform-sensenova | deliver=t | enabled=t | plan_code=starter
商户端 llm_providers:        deepseek-flash-b(self) | default(self)   ← 无 platform-sensenova
启动日志:                     平台 provider 落库失败 platform-sensenova: [tenant] 表 llm_providers 需要租户上下文但 ctx 未绑定（create）
```

## Suspected Code Paths

- `user-server/internal/service/llm_platform_usage.go:115` — `syncPlatformProviders(ctx)`
  把**调用方传入的 ctx** 原样交给 `gormDB.WithContext(ctx)`（第 145 行）做 upsert；
  启动期调用方 `cmd/api/main.go:397` 传的是 `context.Background()`，ctx 无租户绑定。
- `user-server/internal/pkg/tenancyidx/tenant_callback.go:188` — `tenantGuardFail`
  产出报错原文 `[tenant] 表 %s 需要租户上下文但 ctx 未绑定（%s）`。
- `user-server/internal/pkg/tenancyidx/tenant_callback.go:203` — `scopeQueryForTenant`
  里 `if tenant.IsSystemScope(ctx) { return }` 是唯一豁免通道；`fillCreateForTenant`
  是 create 链的对应豁免点（同一 `IsSystemScope` 判定）。
- `user-server/internal/aiagent/llm/dispatcher_persistence.go:24` — `systemScopeCtx()`
  是**同类问题已修好的先例**：`LoadProvidersFromDB` / `LoadRoutesFromDB` 走
  `tenant.WithSystemScope(context.Background())`，并有回归测试
  `dispatcher_persistence_system_scope_test.go` 钉住。
- `user-server/cmd/api/main.go:397` — `service.SyncPlatformProviders(context.Background())`
  启动期调用点（无租户 ctx）。

## Root Cause Hypothesis

**高置信度**。`llm_providers` 声明了 `TenantID`，属多租户隔离覆盖表；
`fillCreateForTenant`（create 链）在 `TENANT_MODE=on` 且 ctx 无租户绑定时
fail-closed 报错。`syncPlatformProviders` 是**装配层/启动期**操作
（由 `cmd/api/main.go` 启动流程调用，不是 HTTP 请求路径），
却把调用方 ctx 原样透传给写库操作，没有像 `dispatcher_persistence.go`
那样走 `tenant.WithSystemScope` 这条唯一豁免通道，于是每次落库都被拒。

同一缺陷类在 dispatcher 路径已修（`systemScopeCtx()` + 回归测试），
本次是**同一类缺陷在 sync 路径上的遗漏**——两条路径都写 `llm_providers`，
只有一条被修过。

注：即使 ctx 带上某个租户 ID 也不对——平台下发的是**全局** provider 定义
（dispatcher 是全局单键 map，按 Name 索引），不能归属到单个租户；
`LoadProvidersFromDB` 也只装默认租户（tenant_id=1）以防跨租户 Key 泄漏。
所以正确修法是 system scope + 落默认租户，与 dispatcher 路径保持一致。

## Proposed Remediation

**Preferred**: `syncPlatformProviders` 的**写库操作**改走
`tenant.WithSystemScope(ctx)`，与 `dispatcher_persistence.go:systemScopeCtx()`
同一口径。

要点：
- 只放宽**这一个装配层写库点**，不动隔离 callback 本身；
  隔离闸对无租户 ctx 的裸查询仍然 fail-closed（要有断言钉住这一点）。
- 落库行必须显式归属默认租户（`tenant.DefaultTenantID`），
  与 `LoadProvidersFromDB` 的 `Where("tenant_id = ?", tenant.DefaultTenantID)` 一致；
  平台下发的是全局 provider 定义，归属单租户是错的。
- 保留「不动商户自己填的 api_key」语义（现有 OnConflict 列白名单已如此）。

**Alternatives** (optional):
- 在 `cmd/api/main.go` 调用点包一层 system scope：改动更小，但豁免语义散到调用方，
  下次有人换调用点会再踩；且 service 层仍可被别处以裸 ctx 调用。**不推荐**。
- 把 `llm_providers` 移出隔离覆盖表：会破坏「provider 定义按租户隔离」的能力，
  且与既有 `dispatcher_persistence_system_scope_test.go` 的断言冲突。**不推荐**。

**Files likely to change**:
- `user-server/internal/service/llm_platform_usage.go`
- `user-server/internal/service/llm_platform_usage_test.go`（新增回归测试，若无同名文件）

**Tests to add or update**:
- 回归钉死：`TENANT_MODE=on` + 无租户 ctx 的装配期同步必须落库成功，
  且落库行归属默认租户。
- 反向断言：隔离本身不得被放宽——无租户 ctx 的裸写仍须 fail-closed。
- 幂等/不覆盖断言：重复同步不产生重复行；商户自带的 `api_key` 不被平台下发覆盖。

## Risks & Considerations

- **安全**：放宽的是「装配层写库点」，不是全局豁免。必须用断言证明
  隔离闸本身仍然 fail-closed，否则等于开了一个跨租户写入口。
- **归属语义**：落默认租户（tenant_id=1）与 `LoadProvidersFromDB` 的读取过滤一致，
  否则会出现「写进去但读不出来」（provider 永远不可见）的新缺陷。
- **既有行**：`llm_providers` 里若已存在同名但归属别的租户的行，
  upsert 的冲突键是 `name`（全局唯一索引），需确认不会跨租户改写他人行。
- **无 API 破坏性**：不改接口、不改表结构、不涉及迁移。

## Open Questions

- 无。复现路径、红因原文、既有同类修法均已在生产日志与代码中确认。
