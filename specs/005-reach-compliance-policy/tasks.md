# Tasks: 主动触达合规策略（地区×行业可配置）

**Feature**: `005-reach-compliance-policy` ｜ 勾选即以「已验证」为准（无验证不算完成）

## Phase 1 判定器（`internal/service/reach_compliance_policy.go`，新建）

- [x] T001 定义策略值对象 `CompliancePolicy` / `ComplianceRule` / `ComplianceDecision` / `ComplianceInput`
- [x] T002 定义原因码常量（allowed / policy_disabled / channel_denied / channel_not_allowed / opt_in_required / quiet_hours / daily_limit / region_unruled）
- [x] T003 实现 `ParseCompliancePolicy(raw string) (CompliancePolicy, error)` + `ValidateCompliancePolicy`（窗口 0–23、日上限非负、渠道名去空格小写、规则表非 nil）
- [x] T004 实现规则判定 `decide`（地区×行业叠加、拒绝优先于延后、延后取最早释放点）
- [x] T005 实现 `ComplianceUsageCounter` 接口 + `MemoryComplianceUsageCounter`（CST 自然日、有界增长）
- [x] T006 实现 `ConfigCompliancePolicy`（`policy(ctx)` 读参数中心 + 原文级解析缓存 + 坏文档 ERROR 留痕一次）
- [x] T007 实现 `GlobalCompliancePolicy()` / `SetGlobalCompliancePolicyForTest()`（RWMutex，懒初始化）

## Phase 2 同步发送管线（`reach_send_pipeline*.go`）

- [x] T008 新增 `SendStepCompliancePolicy` 常量并插到 `DefaultSendPipelineSteps` 的 `permission` 之后
- [x] T009 `SendPipelineConfig` 增 `CompliancePolicy CompliancePolicyEvaluator` 字段；新增哨兵错误 `ErrSendComplianceDenied`
- [x] T010 实现 `runCompliancePolicy`（nil 判定器 ⇒ skipped；拒绝 ⇒ fail；延后 ⇒ 复用既有延迟队列）
- [x] T011 抽出 `deferSend` 复用于全局静默时段与策略静默时段（消除两处重复入队逻辑）
- [x] T012 `Send` 成功收尾处调用 `RecordSent`（仅成功委派后计数）

## Phase 3 proactive 出口（`proactive_reach.go`）

- [x] T013 `ReachSubject` 扩 `Region` / `Industry` / `OptIn`；`ProactiveReachRequest` 扩三个可选 JSON 字段
- [x] T014 新增哨兵 `ErrReachComplianceDenied`，在 `enforcePreSendApproval` 内（授权闸门之后）追加策略判定
- [x] T015 三个 `enforcePreSendApproval` 调用点透传 region/industry/opt_in

## Phase 4 审计与配置登记

- [x] T016 `model.ReachComplianceLog` 增 `decision`/`reason`/`region`/`industry`/`policy_scope` 五列
- [x] T017 `ComplianceAuditLogger.RecordDecision` + `copyComplianceBatch` 透传新列
- [x] T018 `config_param_seeds.go` 登记 `reach/compliance_policy_json`（json，触达组）

## Phase 5 装配与验证

- [x] T019 `internal/app/reach_tool_wiring.go` 把 `service.GlobalCompliancePolicy()` 注入管线配置
- [x] T020 单测：解析/校验/地区窗口/白名单/同意/日上限/未命中/零行为变更（`reach_compliance_policy_test.go`）
- [x] T021 单测：管线步骤拒绝、延后、跳过、计数（`reach_send_pipeline_compliance_policy_test.go`）
- [x] T022 单测：proactive 出口拒绝与放行（`proactive_reach_compliance_test.go`）
- [x] T023 跑 `go build ./...`、`go vet`、`go test ./internal/service/ -run Compliance`、参数三门
- [x] T024 回写交付报告（`delivery.md`）与 CHANGELOG
