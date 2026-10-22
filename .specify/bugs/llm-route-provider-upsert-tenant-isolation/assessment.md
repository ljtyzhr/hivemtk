# Bug Assessment: LLM 路由与 provider 的后台落库被多租户隔离拦截

- **Slug**: llm-route-provider-upsert-tenant-isolation
- **Created**: 2026-10-22T01:24:44Z
- **Source**: 生产日志实证
- **Verdict**: valid
- **Severity**: high

## Report (verbatim or summarized)

生产环境（`workapi.xapptool.cn`）在后台「LLM 路由」页改场景路由后，日志：

```
[LLM] UpsertRouteToDB 查询 scenario=intent_recognize 失败: [tenant] 表 llm_routing_rules 需要租户上下文但 ctx 未绑定（query）；多租户模式下拒绝执行，请经中间件/TenantRunner/行自证提供租户
[LLM] SetRouteWithAudit 路由落库失败 scenario=intent_recognize: [tenant] 表 llm_routing_rules 需要租户上下文但 ctx 未绑定（query）；多租户模式下拒绝执行，请经中间件/TenantRunner/行自证提供租户
[LLM] UpsertRouteToDB 查询 scenario=sop_reply 失败: ...
（8 个场景各一条）
```

同时改 provider 的质量分报：

```
{"code":400,"message":"provider 落库失败: [tenant] 表 llm_providers 需要租户上下文但 ctx 未绑定（query）；多租户模式下拒绝执行，请经中间件/TenantRunner/行自证提供租户"}
```

## Symptom

后台改 LLM 场景路由 / provider 配置时，**内存态生效但落库被隔离 fail-closed 拒绝**：
接口返回 200（路由）或 400（provider），但 `llm_routing_rules` / `llm_providers`
表里没有对应的变更 ⇒ **容器一重启，运营在后台配的一切全部丢失**。

期望：后台的路由 / provider 配置变更应当落库持久化，重启不丢；
同时隔离本身**不得被放宽**（带请求 ctx 的 HTTP 路径仍应 fail-closed）。

## Reproduction

1. `TENANT_MODE=on`（生产实测：`PLATFORM_ENABLED=true`，SaaS 多租户），
   `llm_routing_rules` 与 `llm_providers` 两表都有 `tenant_id` 列
   ⇒ 被 `tenancyidx` 自动登记为租户表（登记是**按库里有没有 tenant_id 列**自动发现的，
   不是硬编码清单）。
2. 在后台「LLM 路由」页改任一场景的主 provider / fallback，或改任一 provider 的
   质量分 / 启用状态并保存。
3. 观察：接口返回成功，但日志出现上面那两条 `[tenant] 表 ... 需要租户上下文但 ctx 未绑定`。
4. 查库：`SELECT count(*) FROM llm_routing_rules;` → **0 行**（生产实测），
   变更从未落库。

生产实测读数（2026-10-22，`101.35.252.112`）：

```
llm_routing_rules: 0 行（8 个场景的路由配置全部没落库）
llm_providers: platform-sensenova 的 quality_score 直接改库后为 0.96
  （走 API 改则报 400，内存态改了但落库失败）
```

## Suspected Code Paths

- `user-server/internal/aiagent/llm/dispatcher_persistence.go:83` —
  `UpsertProviderToDB` 用 `g := providerDB()`（**无 ctx**）做 `First` / `Create` / `Save`。
  `g` 上不带任何 ctx ⇒ 隔离 callback 取不到租户 ⇒ fail-closed。
- `user-server/internal/aiagent/llm/dispatcher_persistence.go:148` —
  `UpsertRouteToDB` 同样用无 ctx 的 `g` 做 `First` / `Create` / `Save`。
- `user-server/internal/aiagent/llm/dispatcher_persistence.go:188` —
  `DeleteRouteFromDB` 用无 ctx 的 `g` 做 `Delete`。
- `user-server/internal/aiagent/llm/dispatcher_persistence.go:201` —
  `DeleteProviderFromDB` 用无 ctx 的 `g` 做 `Delete`。
- `user-server/internal/aiagent/llm/dispatcher_persistence.go:31` —
  `systemScopeCtx()` 是**同文件内已有的正确做法**（`LoadProvidersFromDB` /
  `LoadRoutesFromDB` 走它），但只用在**读**路径上，四个**写**函数漏了。
- `user-server/internal/pkg/tenancyidx/tenant_callback.go:188` —
  `tenantGuardFail` 产出报错原文 `[tenant] 表 %s 需要租户上下文但 ctx 未绑定（%s）`。

## Root Cause Hypothesis

**高置信度**。`llm_routing_rules` / `llm_providers` 都有 `tenant_id` 列，
被 `tenancyidx.InitTenantRegistry`（按 `information_schema` 内省自动发现）
登记为租户表。`TENANT_MODE=on` 时，隔离 callback 对「ctx 里没有租户绑定」的
写是 fail-closed 的。

这四个写库函数**没有把任何 ctx 传给 GORM**（`g` 是 `db.GetDB()` 裸句柄），
于是 callback 拿到的 `stmt.Context` 是 `context.Background()`，既没有租户绑定、
也不是 system scope ⇒ 每次都拒绝。

与同文件读路径的对照最能说明问题：`LoadProvidersFromDB` 走
`g.WithContext(systemScopeCtx())`（`dispatcher_persistence.go:42`）——
同一个文件、同一张表、读能过、写不过。

**为什么没更早暴露**：这四个函数此前只在**默认租户 / 单租户**下被调用过
（`TENANT_MODE=off` 时 callback 直通，不报错）；SaaS 多租户开启后，
后台的路由/provider 配置页一用就红。生产上 `llm_routing_rules` 至今 0 行，
说明这条路径在生产**从未成功落库过**。

## Proposed Remediation

**Preferred**: 四个写库函数统一改走 `g.WithContext(systemScopeCtx())`，
与同文件读路径同一口径。

要点：
- 只放宽这四个**平台级装配写库点**，不动隔离 callback 本身；
  隔离闸对无租户 ctx 的裸写仍然 fail-closed（要有断言钉住）。
- 写库行显式归属默认租户（`tenant.DefaultTenantID`），与
  `LoadProvidersFromDB` 的 `Where("tenant_id = ?", tenant.DefaultTenantID)`
  读取口径一致；否则会出现「写进去但读不出来」。
- `UpsertProviderToDB` 的 `Save` 要注意别把 `tenant_id` 写成 0
  （D2 缺陷：零值 Save 会把行移出本租户，见
  `.specify/bugs/saas-tenant-isolation-audit/`）。
- `DeleteProviderFromDB` / `DeleteRouteFromDB` 同样要走 system scope，
  否则后台「删除 provider」也是删不掉的。

**Alternatives** (optional):
- 把这两张表移出租户登记表：不推荐。dispatcher 的 provider 注册表与路由表
  是**全局单键 map**（按 Name / scenario 索引），本来就不分租户，
  但它们是租户表 ⇒ 需要的是"按默认租户写"而不是"不隔离"。
- 在 controller 层包 ctx：会让豁免语义散到调用方，且这四个函数还有别的调用点
  （`llm_routing.go` service 层）。**不推荐**。

**Files likely to change**:
- `user-server/internal/aiagent/llm/dispatcher_persistence.go`
- `user-server/internal/aiagent/llm/dispatcher_persistence_system_scope_test.go`（扩写既有回归测试）

**Tests to add or update**:
- 回归钉死：`TENANT_MODE=on` 下四个写库函数（Upsert/Delete × provider/route）
  必须落库成功，且行归属默认租户。
- 反向断言：隔离本身不得被放宽——无租户 ctx 的裸写仍须 fail-closed。
- 幂等断言：重复 upsert 不产生重复行。

## Risks & Considerations

- **安全**：放宽的是「平台级装配写库点」四个位置，不是全局豁免。
  必须用断言证明隔离闸本身仍然 fail-closed。
- **归属语义**：写默认租户（tenant_id=1）与 dispatcher 的读取过滤一致，
  否则 provider / 路由写进去但 dispatcher 读不出来。
- **D2 零值 Save**：`Save` 一个 `TenantID` 零值的结构体会把行移出本租户
  （既有审计卡 saas-tenant-isolation-audit 的 D2）。写库时必须显式带上
  `tenant_id = DefaultTenantID`，不能依赖列默认值。
- **无 API 破坏性**：不改接口、不改表结构、不涉及迁移。
- **并发**：这四个函数是后台低频操作，行锁/唯一键冲突不是新问题；
  但 upsert 走的是「先查后写」（非 `ON CONFLICT`），并发下可能撞唯一键
  —— 与修复前同构，不在本次范围。

## Open Questions

- 无。复现路径、红因原文、同文件内的正确先例均已在生产日志与代码中确认。
