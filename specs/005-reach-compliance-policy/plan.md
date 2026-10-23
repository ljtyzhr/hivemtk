# Implementation Plan: 主动触达合规策略（地区×行业可配置）

**Feature**: `005-reach-compliance-policy` ｜ **Created**: 2026-10-23 ｜ **Spec**: `./spec.md`

## 技术路线（一句话）

策略是**一份存进参数中心的 JSON 文档**，由 service 层一个「策略判定器」解析并给出
`ComplianceDecision`；判定器同时挂到两个外发出口（同步发送管线的独立步骤 + proactive 的
`enforcePreSendApproval`），判定结果落 `reach_compliance_log`。没有新表、没有新路由、没有新前端页面
（编辑走既有参数中心管理面）。

## 分层落点（五层架构核对）

| 层 | 是否改动 | 说明 |
|----|---------|------|
| Model | ✅ `internal/model/reach_compliance_log.go` 扩 5 列 | 纯数据结构，无外部依赖 |
| Repository | ❌ 不改 | `ReachComplianceLogRepository.CreateInBatches` 已能承载新列 |
| Service | ✅ 新增 `reach_compliance_policy.go`；改 `reach_send_pipeline.go`/`_steps.go`/`_compliance.go`/`proactive_reach.go`/`config_param_seeds.go` | 判定与编排都在 service；不直接持 `*gorm.DB` |
| Controller | ❌ 不改 | 策略读写复用既有参数中心接口（`/api/manage/config-params`），零新路由 |
| Router | ❌ 不改 | 同上 |
| App（装配） | ✅ `internal/app/reach_tool_wiring.go` 一处 | 把全局判定器注入同步管线配置 |

**为什么不做 Controller/前端**：策略是运维级配置，参数中心已有管理面、审计、租户覆盖、热失效缓存
（60s TTL）。新造一套 CRUD 会立刻产生「两处能改同一份策略」的裂脑，且要连带前端/i18n/契约门。

## 关键设计决策（含被否决的方案）

### D1 承载形态：参数中心 JSON 文档（否决：新表 + CRUD 接口）

- 采纳理由：写侧已有 JSON 语法校验（`validateValue` 的 `json` 分支）、读侧已有 60s TTL 缓存与租户覆盖、
  变更已有操作审计；无需迁移、无需前端。
- 被否决方案：`compliance_policies` 表 + CRUD。代价是多 4 个文件 + 1 条迁移 + 前端 2 个页面 + 契约门，
  收益只是「结构化查询」，而策略文档本身是低频运维配置（个位数条数）。
- 已知代价：JSON 文档的语义校验只能靠代码（`ValidateCompliancePolicy`），且无法按地区分权。
  后者在本产品「私有化单租户为主」的形态下不构成需求。

### D2 判定器形态：进程内全局单例（否决：四处构造点逐个注入）

`ProactiveReachService` 有 4 个构造点（`service_routes.go`、`quote_wiring.go`、`collection_wiring.go`、
`recovery_worker_wiring.go`）。逐个注入 = 「漏接一个就静默失效」，与本仓 A14/外发闸门踩过的坑同形。
因此判定器做成 `GlobalCompliancePolicy()`（与 `GlobalConfigParam()`/`GetGlobalQuietHoursQueue()` 同款），
**判定点只有一处（`enforcePreSendApproval`），装配点为零**。测试用 `SetGlobalCompliancePolicyForTest` 替换。

### D3 同步路径遇静默时段：直接拒绝（否决：也走延迟队列）

`MemoryQuietHoursQueue` 只在同步发送管线里注入，proactive 路径没有延迟队列，硬塞一个会让「拒发」
变成「静默丢消息」。因此同步路径对 `quiet_hours` 回 `reach_compliance_denied`（带原因码），
由上层队列/SOP 重排——上层本来就有退避重试语义。

### D4 日上限计数时机：委派成功后自增（否决：判定时自增）

判定时自增会让「被拒的请求烧掉配额」，连续几次拒发之后客户当天再也发不出去（自锁）。
故 `RecordSent` 只在管线判 `Success` 后调用；被拒/延后的路径不自增。

### D5 默认零行为变更：`enabled` 缺省 + 未配置 = 直通

`policy()` 对空串/解析失败/`enabled=false` 一律回 `Allowed=true, Reason=policy_disabled`，
同步管线该步骤留在 `Steps` 里但记 `skipped`。这样「是否启用」只有参数中心一处开关，
不存在「步骤没挂上」的第二种形态。

## 语义细则（实现判据）

- **地区解析**：`metadata.region`（同步管线）/ `ProactiveReachRequest.region` > 策略 `default_region` > 空。
  比较前统一大写去空格；行业统一小写。
- **规则叠加**：地区规则与行业规则**各自判各自**，任一拒绝即拒绝；多处延后取最早释放时间。
  任一规则命中即可，不要求同时命中。
- **未命中**：地区规则不存在且 `unknown_region_deny=true` ⇒ 拒绝（`region_unruled`）；
  `false`（默认）⇒ 放行并留痕 `region_unruled`（可观测「有多少流量落在无规则地区」）。
- **拒绝优先级**：`channel_denied` > `channel_not_allowed` > `opt_in_required` > `quiet_hours`（延后）> `daily_limit`。
- **静默窗口**：`start > end` 表示跨零点（与既有 `inQuietHoursWindow` 同口径），越界值（<0/>23）
  视为该规则未配置窗口（回落全局静默时段），不报错——避免一个手误把全渠道停机。
- **计数键**：`region|industry|channel|customer_id|day(CST)`；`customer_id` 缺省回落到 `recipient_id`。

## 实施阶段

| 阶段 | 内容 | 产物 |
|---|---|---|
| P1 | 判定器 + 策略解析 + 校验 + 计数（纯函数，无 DB） | `reach_compliance_policy.go` |
| P2 | 同步管线接线（步骤 + 配置字段 + 记录） | `reach_send_pipeline*.go` |
| P3 | proactive 出口接线（`enforcePreSendApproval`） | `proactive_reach.go` |
| P4 | 审计留痕扩列 + 种子登记 | `model/reach_compliance_log.go`、`config_param_seeds.go` |
| P5 | 装配与验证 | `internal/app/reach_tool_wiring.go`、测试、文档 |

## 验证口径（与 CLAUDE.md 3.5/4.4 同表）

| 改动范围 | 命令 |
|---|---|
| Go | `cd user-server && go build ./...` |
| Go service 包 | `go test ./internal/service/ -run 'Compliance\|SendPipeline' -count=1` |
| 跨包 | `go vet ./internal/service/ ./internal/app/` |
| 参数门 | `python3 scripts/check-config-param-readpoints.py` + `-defaults.py` + `-keystyle.py` |
| 提交前 | `make audit`（已知红除外，见交付报告） |

## 风险与回滚

| 风险 | 处置 |
|---|---|
| 策略文档语义写错（如 `quiet_hours=25`）导致误停机 | 越界值视为未配置窗口；`ValidateCompliancePolicy` 在解析时记 ERROR；用例钉住 |
| 日上限计数在进程重启后归零 | 与 `MemoryQuietHoursQueue`/`MemorySendAuditLogger` 同级语义，已在代码注释与本 spec 声明；量级上云再迁 Redis ZSET |
| 全局单例在 `-race` 下被测试并发改写 | 读写走 `sync.RWMutex`；生产只写一次（懒初始化） |
| 出现非预期拒绝（新策略上线） | 参数中心把 `enabled` 置 `false` 或整份清空 ⇒ 60s 内回落直通，无需回滚发版 |
