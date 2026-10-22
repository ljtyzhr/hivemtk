# Fix Report: LLM 路由与 provider 的后台落库被多租户隔离 fail-closed 拦截

- **Slug**: llm-route-provider-upsert-tenant-isolation
- **Fixed**: 2026-10-22
- **Assessment**: [assessment.md](assessment.md)
- **Status**: applied

## Summary

修复 `TENANT_MODE=on` 下「后台改 LLM 场景路由 / provider 配置落库失败」的缺陷。
生产后果：接口返回成功（路由）或 400（provider），但**表里没有对应变更**
⇒ 容器一重启，运营在后台配的一切全部丢失。生产实测 `llm_routing_rules` 至今 **0 行**，
说明这条路径在生产**从未成功落库过**。

根因是四个写库函数用了**无 ctx 的裸句柄** `g := providerDB()`，隔离 callback
拿不到租户绑定 ⇒ fail-closed。同一文件的**读**路径
（`LoadProvidersFromDB` / `LoadRoutesFromDB`）走 `g.WithContext(systemScopeCtx())`
—— 同文件、同表、读能过、写不过。

## Changes

| 文件 | 改动 | 依据 |
|------|------|------|
| `user-server/internal/aiagent/llm/dispatcher_persistence.go` | 四个写库函数（`UpsertProviderToDB` / `UpsertRouteToDB` / `DeleteProviderFromDB` / `DeleteRouteFromDB`）改走 `g.WithContext(systemScopeCtx())`；provider 行与路由行显式写 `tenant_id = DefaultTenantID` | assessment「Proposed Remediation」 |
| `user-server/internal/aiagent/llm/dispatcher_persistence_write_system_scope_test.go` | 新增回归 2 例（写路径落库 / 删除路径落库） | assessment「Tests to add or update」 |

改动面 ⊆ assessment 的文件清单。

## Diff Highlights

```go
 func (d *Dispatcher) UpsertProviderToDB(pc ProviderConfig) error {
 	g := providerDB()
 	if g == nil {
 		return nil
 	}
+	g = g.WithContext(systemScopeCtx())
 	row := providerConfigToRow(pc)
+	row.TenantID = tenant.DefaultTenantID
 	row.UpdatedAt = time.Now()
```

```go
 func (d *Dispatcher) UpsertRouteToDB(r ScenarioRoute) error {
 	g := providerDB()
 	if g == nil {
 		return nil
 	}
+	g = g.WithContext(systemScopeCtx())
 	...
 	row := model.LLMRoutingRule{
+		TenantID:  tenant.DefaultTenantID,
 		Scenario:  string(r.Scenario),
```

显式写 `tenant_id` 的第二个理由（assessment「Risks」第 3 条）：
`Save` 一个 `TenantID` 零值的结构体时，GORM 的 `ConvertToAssignments` 仍会写
`tenant_id` 列，值 0 把行**移出本租户**（D2 缺陷，见
`.specify/bugs/saas-tenant-isolation-audit/`）。显式写 `DefaultTenantID`
才既不串租户、也不依赖列默认值。

## Guard Reverse Verification

守卫 = 每处写库的 `system scope` + 显式 `tenant_id`。

摘掉全部守卫（= 完整还原生产现场）后的读数：

```
dispatcher_persistence_write_system_scope_test.go:59: 装配层写 provider 必须在 system scope 下成功: [tenant] 表 llm_providers 需要租户上下文但 ctx 未绑定（query）；多租户模式下拒绝执行，请经中间件/TenantRunner/行自证提供租户
--- FAIL: TestProviderAndRouteWrites_OnModeWithoutTenantCtx (7.48s)
    dispatcher_persistence_write_system_scope_test.go:137: seed provider 失败: [tenant] 表 llm_providers 需要租户上下文但 ctx 未绑定（query）；多租户模式下拒绝执行，请经中间件/TenantRunner/行自证提供租户
--- FAIL: TestProviderAndRouteDeletes_OnModeWithoutTenantCtx (0.13s)
```

红因文案与生产日志**逐字一致**（`需要租户上下文但 ctx 未绑定（query）`）。

md5 双向对账（共享工作区口径）：

| 对象 | md5 |
|---|---|
| 修复后工作区（最终态） | `193d6bc4c0e7f4425f4f9b7d635e07f2` |
| 注码前基线（注码时记录的修复态） | `193d6bc4c0e7f4425f4f9b7d635e07f2` |
| HEAD blob（未改前） | `3697073cdc5897e1cfee115572ff1f35` |

## Affected Package Regression

```bash
go build ./...                                    # BUILD_OK
go test -p 1 ./internal/aiagent/llm/              # ok  22.904s
```

隔离闸未被放宽：测试首条断言「on 模式无租户 ctx 的裸写必须 fail-closed」仍然成立
（它在守卫摘除前后都通过）。

## Production Deployment

- 修复上线后需在后台重配一次场景路由（本次线上配置在修复前只进了内存、
  `llm_routing_rules` 0 行；修复上线后需重新提交一次才能落库）。
- 发版仍须交叉编译 + 重建镜像（本地 darwin/arm64 二进制在容器里会 exec format error）。
