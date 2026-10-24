# Tasks: D2 观测 + B2 渠道健康看板

**Feature**: `007-observability-and-channel-health` ｜ 勾选即以「已验证」为准

## Phase 1 · 调研取证（spec §2）

- [x] T001 读 `obs_config*.go`：确认为 OBS 对象存储，非观测配置（spec §2.1 记偏差）
- [x] T002 读 `internal/monitor/`：HealthOverview/Anomalies/NodeHealthByChannel/Lifecycle/
      LifecycleLatencyByChannel 现状与缺口（spec §2.1 表）
- [x] T003 读 `bridge_selector_health` 四件（model/repository/service/controller）：
      判定面已收敛在服务端（spec §2.2），本泳道只消费
- [x] T004 读 `bridge_online_predicate.go` + `config_param.go`：渠道在线正确口径
      （status<>'offline' + last_sync_at 宽限窗口），spec §2.4
- [x] T005 盘点数据源：llm_routing_logs / message_hub / message_trace / bridge_accounts /
      domain_pool（spec §2.5）

## Phase 2 · 后端实现

- [x] T006 `internal/repository/observability_dashboard.go`：只读仓储六方法
- [x] T007 `internal/service/observability_dashboard.go`：Snapshot + 口径纯函数 +
      ClassifyChannelHealth（复用 ClassifySelectorHealth）
- [x] T008 `internal/controller/observability_dashboard.go`：GET 端点 + @Router/@Tags 注解
- [x] T009 `internal/router/observability_dashboard_routes.go` + `router.go` 一行接线（写前查 git status）

## Phase 3 · 纯单测

- [x] T010 `internal/service/observability_dashboard_test.go`：
      DeliveryRate / OnlineRate / FunnelConversion 分母 0；ClassifyChannelHealth 全状态序；
      windowDuration 合法/非法/边界；Snapshot 聚合（fake repo）字段正确性

## Phase 4 · 前端

- [x] T011 `user-web/src/api/observabilityDashboard.js`
- [x] T012 `user-web/src/views/observabilityDashboard/Index.vue`（空态渲染，成本 0 非异常）
- [x] T013 `user-web/src/router/modules/observabilityDashboard.js` + `index.js` 白名单 +1（写前查 git status）

## Phase 5 · 验证与交付

- [x] T014 `cd user-server && go build ./...` 绿
- [x] T015 `gofmt -l` 对新增文件干净
- [x] T016 `go vet ./internal/service ./internal/repository ./internal/controller ./internal/router` 绿
- [x] T017 `go test ./internal/service -run Observability -count=1` 绿（纯单测，不连库）
- [x] T018 交付四件套：spec/plan/tasks/delivery（delivery 含改动清单、命令结果、决策点、接线建议）
