# Implementation Plan: D2 观测 + B2 渠道健康看板

**Feature**: `007-observability-and-channel-health` ｜ **Created**: 2026-10-24 ｜ **Spec**: `./spec.md`

## 交付物总览

| 层 | 新增 | 修改 | 牙齿 |
|---|---|---|---|
| repository | `internal/repository/observability_dashboard.go` | — | `go build` |
| service | `internal/service/observability_dashboard.go`、`observability_dashboard_test.go` | — | 纯单测（不连库） |
| controller | `internal/controller/observability_dashboard.go` | — | `go build` |
| router | `internal/router/observability_dashboard_routes.go` | `internal/router/router.go`（1 行接线） | `go build` |
| 前端 | `user-web/src/api/observabilityDashboard.js`、`user-web/src/views/observabilityDashboard/Index.vue`、`user-web/src/router/modules/observabilityDashboard.js` | `user-web/src/router/index.js`（白名单 +1） | 前端 build（主智能体接） |
| 规格 | `specs/007-*/{spec,plan,tasks,delivery}.md` | — | — |

## 后端分层设计

### 仓储（只读，五层 L5）

`ObservabilityDashboardRepository` 六方法，全部 `db.WithContext(ctx)`（租户隔离由既有 callback 盖章）：

- `LLMCost(ctx, since)` → 按 (provider, scenario) 聚合 `llm_routing_logs` 行
- `Delivery(ctx, since)` → 按 platform 聚合 `message_hub` 出站行（delivered/failed/inflight）
- `Funnel(ctx, since)` → `message_trace` 六节点 `COUNT(DISTINCT trace_id)`
- `ChannelAccounts(ctx, graceSeconds)` → `bridge_accounts` 按 channel 聚合
  accounts/online（online 用 `bridgeOnlineSQLPredicate` 同一条 predicate）
- `SelectorHealthRows(ctx)` → `[]model.BridgeSelectorHealth`（复用 B1-a 表）
- `DomainBanSummary(ctx)` → `domain_pool` 黑名单/总数

### 服务（口径层）

`Snapshot(ctx, window)` 聚合为一份 JSON；纯函数供单测：

- `windowDuration(window)`：24h/7d/30d，非法回落 7d
- `DeliveryRate(delivered, failed)`：分母 0 → 0，一位小数
- `OnlineRate(online, accounts)`：同上
- `FunnelConversion(prev, curr)`：同上
- `ClassifyChannelHealth(accounts, online, worstSelector, brokenFields)`：
  empty → banned → offline → degraded → healthy（顺序见 spec §3.6）
- `worstSelector(rows, channel)`：复用 `ClassifySelectorHealth`（B1-a 判定，不新写判据）

### controller / router

- `GET /api/observability/dashboard?window=7d`；`response.Success` 包装；带
  `@Tags observability-dashboard` / `@Router /api/observability/dashboard [get]` 注解
  （route_surface 门要求注解路径参数用花括号、并挂在真正服务该路由的函数上）。
- 路由组：`auth`（已挂 JWT），与 `monitor.RegisterRoutes` 同组、同一调用点之后。

## 前端设计

- 独立页面 `observabilityDashboard/Index.vue`：四区块（成本卡 + 送达率 + 漏斗 + 渠道健康表），
  数据全来自 `/api/observability/dashboard`；空态显式渲染「无数据」（成本 0 不是异常）。
- 独立路由模块 + 白名单 +1；**菜单树由后端 `menus` 表驱动**，本泳道不碰菜单库，
  挂载点写进 delivery（主智能体接菜单 + 路由白名单复核）。

## 边界与护栏

- 铁律：不跑真库测试（测试库被并发 lane 占用）。只跑纯单测 + `go build` + `gofmt -l` + `go vet`。
- 不新表 → 不动 `module_tables.go`；不碰 `CHANGELOG.md`、计划文档、payment、006、.github。
- 接线前必查 `git status --porcelain -- <file>`；router.go / index.js 当前 clean，写入前再复核一次，
  一旦被 lane 改动即跳过接线、在 delivery 写明挂载点。
