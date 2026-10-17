# Phase 1 Contracts: API 契约

**Feature**: 001-saas-multi-tenant | **Date**: 2026-10-17

沿用项目响应契约：成功 `{"code":0,"data":{...},"message":"ok"}`；失败体 `code` 为 4 位业务码（4xxx），登记于 `internal/domain/errors/codes.go`；HTTP 传输语义走 `response.Error`（100-999）。

## 新增端点

### POST /api/tenant/register — 自助开户（SaaS 模式）

```json
// 请求
{ "name": "某某科技", "username": "admin", "email": "a@b.com", "password": "..." }
// 成功
{ "code": 0, "data": { "tenant_id": 42, "tenant_code": "t42xk", "token": "<JWT 含 tenant_id>" }, "message": "ok" }
// 失败（示例）
{ "code": 0, "data": null, "message": "" }  // HTTP 200 + 业务码 4101 参数错误 / 4102 用户名租户内重复 / 4103 邮箱全局重复 / 4104 注册未开放 / 4105 限流
```

- 事务原子：租户 + 管理员账号 + 默认配置一并建成或全回滚。
- 离线模式（TENANT_MODE=off）：恒 4104 注册未开放。
- 限流：同源 IP/指纹频率限制（复用既有 ratelimit 基建）。

### POST /api/auth/login — 登录（既有端点扩展）

- 请求新增可选字段 `email`（全局唯一登录通道）；`username` 登录保持兼容（单机模式不变；SaaS 模式下 username 登录如遇跨租户歧义则提示改用邮箱）。
- 响应 data 增加 `tenant_id`、`tenant_code`；JWT claims 增加 `tenant_id`。
- off 模式：响应不含租户字段语义差异（租户恒为默认租户），存量前端零适配。

### GET /api/tenant/info — 当前租户信息（登录态）

```json
{ "code": 0, "data": { "tenant_id": 42, "tenant_code": "t42xk", "name": "某某科技", "status": 1 } }
```

### GET /api/manage/tenants — 租户运营清单（平台运营，admin）

分页：`page/page_size/status/keyword`；返回 `{"list":[...], "total":N}`，含活跃度概览（成员数/最近活跃时间）。

### PATCH /api/manage/tenants/:id/status — 租户停用/启用（admin）

停用后：该租户登录受限、后台任务跳过；数据保留。

## 既有端点行为变更（协议不破坏）

| 端点族 | 变更 |
|--------|------|
| 全部受保护 API | on 模式 JWT 必须含 tenant_id claim（无则 401）；off 模式行为不变 |
| /api/webhook/:channel/:account_id | **协议零改动**；内部按 account → tenant 解析，解析失败 404 + 告警 |
| /api/chat/public/*（访客） | **协议零改动**；channel → tenant 解析；visitor token 签名携带租户 |
| bridge SSE/HTTP | **协议零改动**；X-Bridge-Token → bridge_account → tenant |
| WS/SSE | 握手鉴权不变；推送按连接绑定租户分桶 |

## 不新增的外部接口

- 平台端（hivemtk-platform）开户下发 API：本期不做（research D3），接口留扩展位。
- PG RLS：二期。
