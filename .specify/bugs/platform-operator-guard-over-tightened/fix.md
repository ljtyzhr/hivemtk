# Fix Report: 平台运营方守卫过度收紧——租户 admin 被误挡在自己的租户级管理端点外

- **Slug**: platform-operator-guard-over-tightened
- **Fixed**: 2026-10-21
- **Assessment**: [assessment.md](assessment.md)
- **Status**: applied

## Summary

第一轮修跨租户越权时把**整个** `systemAdmin` 路由组换成了平台运营方守卫。
方向对（越权堵住了），但**收紧过头**——那一组里混着操作**租户表**的端点，
租户 admin 本就该读写自己租户的这些数据，于是被一并打成 403：
站点配置、工具集成凭证、Agent 调参、OBS 连接、参数覆盖、SSO IdP、模块同步日志 —— 全废。

根因：**守卫按"路由组"施加，而权限边界实际按"数据域"划分。**
「是不是平台运营方」与「这个端点该不该被平台运营方守卫」是两个不同的问题。

修法：把 admin 档按**端点操作的数据落在哪张表**拆成两档。

## Changes

| 文件 | 改动 | 依据 |
|------|------|------|
| `user-server/internal/router/router.go` | admin 档拆成 `tenantAdmin`（普通 admin 守卫）与 `systemAdmin`（平台运营方守卫）；各 setup 函数按数据域分发 | assessment B1/B3/B4/B5 |
| `user-server/internal/router/system_routes.go` | `setupSystemRoutes` 改双入参 `(tenantAdmin, systemAdmin)`，端点按数据域分派 | assessment B2 |
| `user-server/internal/router/module_routes.go` | `setupModuleRoutes` 改四入参 `(public, auth, tenantAdmin, systemAdmin)`；`sync-logs` ⇒ 租户档、`uncovered-routes` ⇒ 平台档 | assessment B3 |
| `user-server/internal/router/platform_operator_scope_test.go` | **新增**门禁：两张归属表 + 重复注册断言 | assessment B6 |
| `scripts/audit_api_contract.py` | 前端契约审计的**按名推断根组表**补 `tenantAdmin` | 见下方「审计工具的连带修复」 |

### 审计工具的连带修复（同一根因的另一处体现）

`scripts/audit_api_contract.py` 判定"后端有没有这条路由"时，对**自由函数形参**的
根组前缀是**按变量名推断**的（`if var in ('auth', 'public', 'router', 'plainAuth',
'admin', 'systemAdmin'): return '/api'`）。

新增的 `tenantAdmin` 组不在那张表里 ⇒ 那批端点被判成"后端没有" ⇒
`make audit` 的 strict 档报 **UNMATCHED=11**（前端调了后端没有）：
`/api/system/config`、`/api/agent/tool-integrations`、`/api/agent/settings`、
`/api/obs/config*`、`/api/manage/config-params`、`/api/modules/sync-logs`。

**报的方向完全指错**——真相是"后端路由存在、审计脚本没认出新组名"，
而脚本说的是"前端调了后端没有"。不查清楚就会去改前端或加后端路由，越改越偏。

**修法**：把 `tenantAdmin` 加进那张表（装配处 `tenantAdmin := auth.Group("")`
⇒ 前缀同样是 `/api`），并把"任何新增组形参都必须登记"写进注释。

**这也是同一个根因的另一面**：**按名字约定的东西（组形参名、路由表、门禁清单）
都会在新增时漏登记**，而且漏登记时的报错方向往往指向别处。
⇒ 新增任何"按名识别"的东西时，要么登记进表，要么改成一等公民（显式前缀参数）。

## Diff Highlights

`internal/router/router.go`：

```go
// 两个 admin 档，按**数据域**分，不按"路由组"一刀切：
//
//   tenantAdmin —— 操作**租户表**（system_config / system_config_kv /
//     obs_config / tenant_config_params …）的管理端点。租户 admin 本就该
//     读写自己租户的这些数据（隔离 callback 已经把范围限死在本租户）。
//
//   systemAdmin —— 操作**全局表与实例运行态**的管理端点（tenants 租户清单 /
//     停用租户 / 备份恢复 / 重启 / 迁移记录 / 路由巡检）。
//
// 【为什么必须分两档】平台运营方守卫是"降敌意"方向（把本该能访问的人挡在外面），
// 一旦套在整个 admin 组上就会**误伤租户内管理功能**——第一轮就是这么把
// 「租户 admin 读自己的站点配置 / 工具集成 / Agent 设置 / OBS 配置 / 参数覆盖」
// 全部打成 403 的。分档判据是**端点操作的数据落在哪张表**，不是"挂在哪个组"。
tenantAdmin := auth.Group("")
tenantAdmin.Use(middleware.AdminAuthMiddleware())

systemAdmin := auth.Group("")
systemAdmin.Use(middleware.PlatformOperatorMiddleware())
setupSystemRoutes(tenantAdmin, systemAdmin)
```

`internal/router/system_routes.go`（按数据域分派）：

```go
// 判据是"端点写读落到哪张表"：
//   system_config / system_config_kv / obs_config 都是**租户表**（有 tenant_id 列，
//   由隔离 callback 自动限域）⇒ 租户档；
//   backups / restore_records 是**全局表**，进程级操作没有租户概念 ⇒ 平台运营方档。
tenantAdmin.GET("/system/config", systemConfigCtrl.GetConfig)
...
systemAdmin.POST("/system/restart", systemOpsCtrl.RestartServer)
systemAdmin.GET("/system/stats", systemOpsCtrl.GetSystemStats)
systemAdmin.GET("/system/backup", systemOpsCtrl.GetBackupList)
```

## Tests Added or Updated

- `internal/router/platform_operator_scope_test.go`（2 例）：
  - `TestPlatformOperatorScopeEndpointOwnership`：逐端点断言归属档位
    （租户级端点不得在平台档、平台级端点不得在租户档）。
  - `TestPlatformOperatorScopeNoDuplicateRegistration`：同一端点不得同时注册在两档。
- 未改任何既有测试。

## Local Verification

环境：真实 PG（容器 `mtk-postgres`，`127.0.0.1:8202`，库 `user_db`），
`TENANT_MODE=on` + `PLATFORM_ENABLED=false`，真实 user-server 监听 8204。

### 1. 回归修复：租户 admin 恢复访问自己的租户级端点

租户 6 的 admin 令牌：

```
GET /api/system/config            → 200 ✅（修复前 403）
GET /api/agent/tool-integrations  → 200 ✅（修复前 403）
GET /api/agent/settings           → 200 ✅（修复前 403）
GET /api/obs/config               → 200 ✅（修复前 403）
GET /api/manage/config-params     → 200 ✅（修复前 403）
GET /api/manage/config-params/groups → 200 ✅（修复前 403）
GET /api/admin/config             → 200 ✅（修复前 403）
GET /api/modules/sync-logs        → 200 ✅（修复前 403）
GET /api/sso/config               → 200 ✅
```

### 2. 越权面仍然堵着

同一令牌：

```
GET  /api/manage/tenants          → 403 ✅
PATCH /api/manage/tenants/5/status（停用别的租户）→ 403 ✅
GET  /api/system/stats            → 403 ✅
GET  /api/system/backup           → 403 ✅
GET  /api/modules/uncovered-routes→ 403 ✅
POST /api/system/restart          → 403 ✅
PUT  /api/manage/dictionaries/:parent/items → 403 ✅
```

平台运营方（首租户 tenant_id=1）正向路径不受影响：

```
GET  /api/manage/tenants          → 200，total=6（全部租户可见 ✅）
PUT  /api/manage/dictionaries/:parent/items → 200 ✅
GET  /api/sso/config               → 200 ✅
```

### 3. 租户级写仍然受隔离保护

`POST /api/system/config`（租户 6 的 admin 改自己的站点名）⇒ 200，响应 `tenant_id=6`；
库内核对 `system_config`：**租户 5 的行没变**、只有租户 6 的行被改 ⇒
「租户 admin 可写自己那份」与「写不到别人那份」同时成立。

### 4. 门禁单测（2 例）

```bash
go test ./internal/router/... -run 'TestPlatformOperatorScope' -count=1 -v
```

**结果**：2 例全 PASS ✅

### 5. **反向验证**（门禁必须能红）

把 5 个租户级端点临时挪到 `systemAdmin` 档后重跑：

```
--- FAIL: TestPlatformOperatorScopeEndpointOwnership
    租户级端点 /api/system/config 误挂在平台运营方档 ⇒ 租户 admin 会被 403
    租户级端点 /api/agent/tool-integrations 误挂在平台运营方档 ⇒ 租户 admin 会被 403
    租户级端点 /api/agent/settings 误挂在平台运营方档 ⇒ 租户 admin 会被 403
    租户级端点 /api/obs/config 误挂在平台运营方档 ⇒ 租户 admin 会被 403
    租户级端点 /api/admin/config 误挂在平台运营方档 ⇒ 租户 admin 会被 403
--- FAIL: TestPlatformOperatorScopeNoDuplicateRegistration
```

恢复修复后重新变绿 ✅ —— **门禁确实有牙**（能红的门禁才算门禁）。

## Deviations from Assessment

- assessment B2 写的是「`setupSystemRoutes(tenantAdmin, systemAdmin)` 双入参」，
  实际实现按其意图落为「双入参 + 按数据域分派端点」，与 B2 一致。
- assessment 提到「`setupDictionaryRoutes` ⇒ 平台运营方档」：
  实测**只读**端点 `/api/manage/dictionaries` 本来就在 `plainAuth` 档（历史设计，
  全局只读共享数据，平台运营方与租户 admin 都能读），**未改动**；
  只有**写**端点（items / transitions）收进平台运营方档。

## Follow-ups

- **独立 `platform_admin` 角色**未做（语义最干净，改动面大）——
  评估文档 `cross-tenant-admin-privilege-escalation` 的 Follow-ups 已记。
- 本轮把"端点归属"钉成了**测试里的两张表**，但它与 `docs/architecture/DEPLOYMENT_MODES_EVALUATION_2026-10.md`
  §2 的行为契约表是**两份真相**。后续若要收口，应让门禁直接读那份文档的表格
  （或反过来由表格生成门禁），避免一处改了另一处没改。
