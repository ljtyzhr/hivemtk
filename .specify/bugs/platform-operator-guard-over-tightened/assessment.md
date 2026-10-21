# Bug Assessment: 平台运营方守卫过度收紧——租户 admin 被误挡在自己的租户级管理端点外

- **Slug**: platform-operator-guard-over-tightened
- **Created**: 2026-10-21
- **Source**: 二次审核（对 `cross-tenant-admin-privilege-escalation` 修复的活体复验）
- **Verdict**: valid
- **Severity**: high

## Report (verbatim or summarized)

二次审核时用**真实租户令牌**复验上一轮的跨租户越权修复，发现修复方向对、但**收紧过头**：

上一轮把整个 `systemAdmin` 路由组换成了 `middleware.PlatformOperatorMiddleware()`。
跨租户越权确实堵住了（租户 admin 看不到/停用不了别的租户），但那一组里**混着操作
租户表的端点**——租户 admin 本就该读写自己租户的这些数据（隔离 callback 已把范围限死
在本租户），于是被一并打成 403。

## Symptom

`TENANT_MODE=on` 下，用**普通租户**的 admin 令牌访问**自己租户**的管理端点，全部 403：

| 端点 | 落库 | 期望 | 实际（修复后） |
|---|---|---|---|
| `GET /api/system/config` | `system_config`（**租户表**） | 200 | **403** |
| `GET /api/agent/tool-integrations` | `system_config_kv`（**租户表**） | 200 | **403** |
| `GET /api/agent/settings` | `system_config_kv`（**租户表**） | 200 | **403** |
| `GET /api/obs/config` | `obs_config`（**租户表**） | 200 | **403** |
| `GET /api/manage/config-params` | 读平台目录 + **本租户覆盖** | 200 | **403** |
| `GET /api/admin/config` | 进程静态配置 | 200 | **403** |
| `GET /api/modules/sync-logs` | `module_sync_logs`（**租户表**） | 200 | **403** |
| `GET /api/sso/config` | `system_config_kv` 的 `sso.providers`（**租户表**） | 200 | **403** |

即：**租户管理员在自己的租户里，除了业务功能之外，所有管理配置页面全废**。
站点名、工具集成凭证、Agent 调参、OBS 连接、参数覆盖、SSO IdP —— 一律读不到。

## Reproduction

前置：`TENANT_MODE=on`、真实 PG、本地 user-server（8204）、两个租户（首租户 id=1 作平台运营方，
租户 id=6 作探针租户）。

```bash
# 登录租户 6 的 admin，拿令牌
TOKEN=$(curl -s -X POST -H "Content-Type: application/json" \
  -d '{"username":"probe_tb_admin","password":"Probe@123456"}' \
  http://127.0.0.1:8204/api/auth/login | jq -r .data.token)

# 读**自己租户**的站点配置 ⇒ 403（期望 200）
curl -s -H "Authorization: Bearer $TOKEN" http://127.0.0.1:8204/api/system/config
# ⇒ {"code":403,"message":"权限不足，该操作仅平台运营方可执行"}
```

## Suspected Code Paths

- `user-server/internal/router/router.go` — `systemAdmin := auth.Group("")` +
  `middleware.PlatformOperatorMiddleware()`，然后 `setupSystemRoutes(systemAdmin)` /
  `setupModuleRoutes(nil, auth, systemAdmin)` / `setupConfigParamRoutes(auth, systemAdmin, …)` /
  `setupSSOConfigRoutes(systemAdmin, …)` / `setupDictionaryRoutes(auth, systemAdmin, …)`
  —— **整组换守卫**，没按端点操作的数据域分类。
- `user-server/internal/router/system_routes.go` — `setupSystemRoutes(auth)` 的单一入参
  形制，使"租户级端点"与"实例级端点"无法分档。

## Root Cause Hypothesis

**守卫按"路由组"施加，而权限边界实际按"数据域"划分。**

`admin` 角色在 SaaS 模式下承载两种语义（平台运营方 / 租户管理员），所以需要
`PlatformOperatorMiddleware` 这道更严的守卫——这点第一轮判对了。
但**判定"是不是平台运营方"和"这个端点该不该被平台运营方守卫"是两个不同的问题**：

| 端点操作的数据 | 表的域 | 该挂哪档 |
|---|---|---|
| `system_config` / `system_config_kv` / `obs_config` / `module_sync_logs` / `tenant_config_params` | **租户表**（有 `tenant_id` 列，隔离 callback 自动限域） | 租户档（普通 admin 守卫） |
| `tenants` / `backups` / `restore_records` / `migration_records` / 进程内路由注册表 | **全局表 / 实例运行态** | 平台运营方档 |

**关键不对称**：平台运营方守卫是"**降敌意**"方向——它把本该能访问的人**挡在外面**。
套错档位的后果是**误伤合法功能**（租户 admin 读不到自己的配置）；
而漏套的后果是**漏放越权**（租户 admin 能停用别的租户）。
两个方向的代价不对称，但**都必须按数据域判**，不能按"这个路由组看起来像管理端"判。

**为什么常规测试没抓到**：守卫本身是对的、编译照过、既有单测照绿；
这类回归只在**拿真实租户令牌打活体端点**时才看得见。

## Proposed Remediation

**Preferred**（按数据域把 admin 档拆成两档）：

- **B1** `router.go` 拆两组：
  - `tenantAdmin := auth.Group("")` + `middleware.AdminAuthMiddleware()`（租户级管理端点）
  - `systemAdmin := auth.Group("")` + `middleware.PlatformOperatorMiddleware()`（平台运营方端点）
- **B2** `setupSystemRoutes(tenantAdmin, systemAdmin)` 双入参，按数据域分派端点：
  - 租户档：`/system/config`、`/agent/tool-integrations`、`/agent/settings`、`/obs/config*`、`/admin/config`
  - 平台运营方档：`/system/restart`、`/system/logs`、`/system/stats`、`/stats/system`、`/system/backup`、`/system/restore`
- **B3** `setupModuleRoutes(public, auth, tenantAdmin, systemAdmin)`：
  - `sync-logs`（`module_sync_logs` 租户表）⇒ 租户档
  - `uncovered-routes`（进程内路由注册表）⇒ 平台运营方档
- **B4** `setupConfigParamRoutes` / `setupSSOConfigRoutes` ⇒ 租户档
  （config-params 的写落在 `tenant_config_params` 本租户覆盖行，见
  `service/config_param.go` `UpdateValue`；SSO 落 `system_config_kv` 的 `sso.providers`）
- **B5** `setupDictionaryRoutes` ⇒ 平台运营方档
  （`dictionaries` / `dictionary_transitions` 是全局只读共享数据，
  改词条是全局治理动作；只读端点本来就在 plainAuth 档，不受影响）
- **B6** 新增常驻门禁 `internal/router/platform_operator_scope_test.go`：
  把"哪些端点归哪档"钉成两张表，并断言**同一端点不得同时注册在两档**。
  必须做**反向验证**（把端点挪错档位 ⇒ 测试必须红）。

**Alternatives**:

- **A. 守卫内按路径前缀放行租户级端点**：把"哪些路径是租户级"做成守卫里的白名单。
  ⇒ 否决：判据从"数据域"退化成"路径字符串"，端点一多必然漂移。
- **B. 引入独立的 `platform_admin` 角色**：语义最干净，但要改开户、角色枚举、签发、
  前端判断与存量迁移。⇒ 本轮不做（评估文档 §「Follow-ups」已记）。

**Files likely to change**:

- `user-server/internal/router/router.go`（拆两组）
- `user-server/internal/router/system_routes.go`（双入参 + 按数据域分派）
- `user-server/internal/router/module_routes.go`（双入参 + 按数据域分派）
- `user-server/internal/router/platform_operator_scope_test.go`（新增门禁）

**Tests to add or update**:

- `TestPlatformOperatorScopeEndpointOwnership`：逐端点断言归属档位。
- `TestPlatformOperatorScopeNoDuplicateRegistration`：断言同一端点不重复注册在两档。
- **反向验证**：把租户级端点挪到 `systemAdmin` 档 ⇒ 上两条必须红。

## Risks & Considerations

- **off 模式零回归**：`PlatformOperatorMiddleware` 在 off 模式恒真 ⇒ 离线单机全部
  admin 端点行为**逐字节不变**。
- **租户级写端点仍受隔离保护**：`/api/system/config` 等虽然租户 admin 可写，但写入
  由隔离 callback 限在本租户（实测写租户 6 的行，租户 5 的行不变）。
- **字典只读端点不动**：`/api/manage/dictionaries`（读）本来在 plainAuth 档，
  平台运营方与租户 admin 都能读（全局只读共享数据）；只有**写**端点收进平台档。
- **并发写入者**：本仓工作区有另一 agent 进程并发改同仓 ⇒ 提交前需重新核对 `git status`，
  且测试须在树稳定后重跑。

## Open Questions

- [无阻塞项。修法按数据域分档，方向由用户「平台超管=首个租户」的既定选择推导而来。]
