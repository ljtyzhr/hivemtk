# Bug Assessment: 跨租户管理端点越权（租户 admin 可操作其他租户与实例级端点）

- **Slug**: cross-tenant-admin-privilege-escalation
- **Created**: 2026-10-21
- **Source**: 本地 2×2 部署矩阵实跑（前置任务三形态③ `TENANT_MODE=on` + `PLATFORM_ENABLED=false`）打出的对抗性发现
- **Verdict**: valid
- **Severity**: high

## Report (verbatim or summarized)

用户指令（m00003 原文要点）：

> 前置任务二 2 对抗性 检查、审核、测试 多部署模式兼容 发现问题立即修复
> 前置任务三 请在本地 使用线上开发板平台端域名 完成本地 user-server|user-web 1saas版本 地启动 初始化

在形态③（`TENANT_MODE=on` + `PLATFORM_ENABLED=false`）本地起真实 `user-server` 后，
自助开户拿到两个租户，用**租户 B 的 admin 令牌**打**跨租户管理端点**，发现两条越权。

## Symptom

`TENANT_MODE=on` 下，**任意租户**的 `admin` 角色账号都能：

1. **读** `GET /api/manage/tenants` ⇒ 拿到**全部租户**清单（tenant_id / code / 公司名 / 负责人 / 电话 / 邮箱 / 地址 / 开户登记全字段）；
2. **写** `PATCH /api/manage/tenants/:id/status` ⇒ **停用其他租户**（实测把租户 5 停用成功，返回 `{"status":2,"tenant_id":5}`）；
3. 调**实例级**端点：`POST /api/system/restart`（重启整个服务）、`GET /api/system/backup`、`GET /api/admin/config`、`GET /api/agent/tool-integrations` 全部 200。

预期：`tenants` 是**全局表**（无 `tenant_id` 列 ⇒ 隔离 callback 不注入条件 ⇒ 全量放行），
它的运营端点按 spec FR-022 只该给**平台运营方**；租户 admin 不该看到/停用别的租户，
更不该重启实例、拉取备份、读实例级配置。

## Reproduction

前置：`TENANT_MODE=on`、`PLATFORM_ENABLED=false`、真实 PG（`user_db`）、本地 `user-server` 监听 8204。

```bash
# 1) 开户拿租户 A（token_A）
curl -s -X POST -H "Content-Type: application/json" -d '{
  "name":"探针租户A","username":"probe_a","password":"Probe@123456",
  "email":"probe_a@example.com","phone":"13800000003","legal_person":"A负责人",
  "company_name":"A公司","address":"某地","regulatory_ack":true}' \
  http://127.0.0.1:8204/api/tenant/register
# ⇒ {"code":0,"data":{"tenant_id":5,"tenant_code":"tjvdvvu","token":"..."}}

# 2) 再开一个租户 B（token_B）—— 同样自助开户，role=admin
#    token_B claim: {"user_id":41,"username":"probe_tb_admin","role":"admin","tenant_id":6,...}

# 3) 用**租户 B** 的令牌读跨租户清单
curl -s -H "Authorization: Bearer $token_B" http://127.0.0.1:8204/api/manage/tenants
# ⇒ total=6，list 里含 id=1..6 全部租户（含别家公司名/负责人/电话/邮箱/地址）

# 4) 用**租户 B** 的令牌停用**租户 A**
curl -s -X PATCH -H "Authorization: Bearer $token_B" -H "Content-Type: application/json" \
  -d '{"status":2}' http://127.0.0.1:8204/api/manage/tenants/5/status
# ⇒ {"code":0,"data":{"status":2,"tenant_id":5}}   ← 租户 A 被停用
```

## Suspected Code Paths

- `user-server/internal/router/tenant_routes.go:9` — 路由注释写明"跨租户可见的数据，是平台运营动作（停用会影响该租户全部用户登录），**必须硬超管守卫**"，但传入的 `admin` 组只挂了 `AdminAuthMiddleware()`。
- `user-server/internal/router/router.go:541-547` — `systemAdmin := auth.Group("")` + `middleware.AdminAuthMiddleware()`，然后 `setupTenantRoutes(nil, auth, systemAdmin)`。
- `user-server/internal/middleware/jwt.go:119` — `AdminAuthMiddleware` 只判 `role != "admin"` ⇒ 403。而**租户 admin 的 role 正是 `admin`**（`internal/service/tenant.go` 开户时建 admin，`model.SystemUserRoleAdmin = "admin"`）⇒ 守卫形同虚设。
- `user-server/internal/repository/tenant.go:119` — `tenantRepo.List` 直接 `r.db.WithContext(ctx).Model(&model.Tenant{})`，**无任何租户过滤**；`tenants` 是全局表（无 `tenant_id` 列）⇒ 隔离 callback 不注入条件 ⇒ 全量返回。
- `user-server/internal/service/tenant.go:265` — `UpdateTenantStatus` 用 `GetByID` 拿到就改，**不校验调用者是不是该租户**。

## Root Cause Hypothesis

**「admin」这一个角色在 SaaS 模式下承载了两种语义**，而守卫只认角色名：

| 语义 | 角色值 | 该有的权限边界 | 现状 |
|---|---|---|---|
| **平台运营方**（跨租户） | `admin` | 看全部租户、停用租户、实例级操作 | 与租户 admin **不可区分** |
| **租户管理员**（租户内） | `admin` | 只见自己租户的数据 | **拿到了平台运营方的权限** |

spec FR-022 明确要求"多租户模式下 MUST 提供租户维度的运营可见性（租户清单、状态、活跃度）**供平台运营方管理使用**"，
而 Key Entities 又限定"租户用户（SystemUser）…**保留既有角色与数据范围体系（其语义被限定在租户边界内）**"。
⇒ 语义边界是清楚的，**缺的只是把"平台运营方"这个身份在代码里表达出来并被守卫认到**。

**行级隔离机制本身没有破**（这一条要分清，否则会误修）：
- `GET /api/system/config` 租户 B 读到的是 `{"tenant_id":6,"site_name":"探针租户B"}` ⇒ 租户表隔离**生效**；
- `GET /api/system/stats` 的 `total_users=1`（全库 21 个用户，租户 6 只有 1 个）⇒ 统计**已隔离**；
- 泄露面**只在全局表**（15 张无 `tenant_id` 的表，`tenants` 是其中最要命的一张）
  **与实例级操作**（restart / backup / restore / 实例配置）上。

## Proposed Remediation

**Preferred**（用户已选定方向：平台超管 = 首个租户）：

- **B1 引入"平台超管"判定**（`internal/pkg/tenant` 或 `internal/middleware`）：
  `IsPlatformOperator(ctx) bool` —— 判据 = **调用者所属租户是本实例的首个租户**（`tenants` 表最小 ID）。
  - `on` 模式：首个租户 = 平台运营方租户（setup 向导建的第一个租户，或最早自助开户的那个）。
  - `off` 模式：**恒真**（单机形态只有一个租户，行为与历史版本逐字节一致，零回归）。
- **B2 把 `systemAdmin` 组的守卫从"角色判定"换成"平台超管判定"**：
  新增 `middleware.PlatformOperatorMiddleware()`（角色 + 平台超管双条件），
  `router.go` 的 `systemAdmin` 组改挂它。`AdminAuthMiddleware` 保留给**租户内**的管理端点。
- **B3 回归用例**：
  - 租户 admin 调 `/api/manage/tenants` ⇒ 403；
  - 租户 admin 调 `PATCH /api/manage/tenants/:id/status` ⇒ 403（且**目标租户不被停用**——要断言库内状态未变，不能只断言 HTTP 码）；
  - 平台超管（首租户 admin）调同端点 ⇒ 200；
  - `off` 模式全量 admin 端点 ⇒ 行为与历史一致（不回归）。

**Alternatives**:

- **A. 引入独立角色 `platform_admin`**：语义最干净，但要改开户流程、角色枚举、登录签发、前端角色判断，**改动面大**且存量账号要迁移 ⇒ 本轮不做，留作后续。
- **B. 在 `tenants` 上加 `tenant_id`**：把全局表改成租户表 —— **语义错误**（租户表不可能是别的租户的租户表），且会让"看全部租户"这个运营需求无解 ⇒ 否决。
- **C. 只在 controller 层过滤**：治标，端点一多就漏 ⇒ 否决。

**Files likely to change**:

- `user-server/internal/middleware/platform_operator.go`（新建）
- `user-server/internal/router/router.go`（`systemAdmin` 组换守卫）
- `user-server/internal/pkg/tenant/platform_operator.go` 或 `user-server/internal/repository/tenant.go`（首租户判定 + 缓存）
- `user-server/internal/middleware/platform_operator_test.go`（新建，回归）
- `user-server/internal/router/router_tenant_operator_test.go`（新建，路由级回归）

**Tests to add or update**:

- 上面 B3 的 4 条（尤其"租户 admin 停用别的租户"要**断言库内状态未变**）。

## Risks & Considerations

- **off 模式零回归**：`IsPlatformOperator` 在 off 模式恒真 ⇒ 离线单机所有 admin 端点行为**逐字节不变**（这是本仓硬要求，见 CLAUDE.md 五层架构与双形态口径）。
- **首租户判定的稳定性**：`tenants` 表最小 ID 在正常运营下不变（租户只增不删；停用是软删，不改 ID）。但**若首租户被停用**，平台超管会被锁在门外 ⇒ 需要一条"首租户停用后仍可运营"的逃生路径，或在停用首租户时**拒绝**该操作。建议：B2 的 `PATCH .../status` 对**首租户**额外加保护（不允许停用首租户），从根上避免这个锁死。
- **`off` 模式 `tenants` 表可能为空**（历史数据没建租户行）⇒ 首租户判定拿不到 ID ⇒ off 模式恒真这条前提让它不受影响；on 模式 `tenants` 必有行（开户才建）。
- **不要改 `AdminAuthMiddleware` 本身**：它被大量租户内管理端点复用（`/api/system/config` 等），改成平台超管判定会让**所有租户内运营功能全废**。只换 `systemAdmin` 组这一处的挂载。
- **既有 lint 债**：`make lint` 的 4 条红（`customer_session_cron.go` errcheck、`bridge/sse.go:540` ineffassign、`rate_quota.go` unused、`reach_send_pipeline_quiethours.go` unused）在 HEAD 上一模一样存在 ⇒ 不属本任务范围。
- **并发写入者**：本仓工作区在本轮有**另一个 agent 进程**并发改 `internal/ops/**`（A/B 实验 ctx 化重构）⇒ 改动必须避开 `internal/ops/**`，提交前需重新核对 `git status`。

## Open Questions

- [无阻塞项。修法方向已由用户确认（平台超管 = 首个租户）。]
