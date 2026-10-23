# Feature Specification: 主动触达合规策略（地区×行业可配置）

**Feature Branch**: `005-reach-compliance-policy`

**Created**: 2026-10-23

**Status**: Approved

**Input**: 用户指令：「读取调研论证文档，提取改进清单，形成开发计划，完善计划，评估计划是否可执行，然后开始代码开发，交付」。
本 spec 对应调研文档《HiveMtk 深度调研论证与改进方向》P0 项 **E1 主动触达合规策略可配置**
（原文：「短信/邮件/社媒/Telegram/WhatsApp 主动推送是敏感能力，虽有 [COMPLIANCE] 提示与 DNC，
但缺地区/行业可配置合规策略」）。

## 现状调研结论（Argumentation）

对代码库的实际核验（2026-10-23，主 Agent 逐文件读源码）：

### 已有的合规相关能力（不重做）

| 环节 | 文件 | 现状 |
|------|------|------|
| 发送前合规提醒 | `internal/service/reach_send_pipeline_compliance.go` | `LogComplianceReminder` 每次外发打 `[COMPLIANCE]` WARN + 异步落库 `reach_compliance_log`（只有 channel/recipient 两列） |
| 全局退订（DNC） | `internal/service/do_not_contact.go`、`aiagent/agent/tooluse/decorator_dnc.go` | 跨渠道 `IsBlocked` 已接线到 permission 步骤与工具链 |
| 全渠道静默时段 | `internal/service/reach_send_pipeline_quiethours.go` | 参数中心 `reach.quiet_hours_start/_end`，全局唯一窗口（默认 22–8 CST），只作用于同步发送管线 |
| 单客户日触达上限 | `internal/service/reach_pipeline.go:193` | 参数中心 `reach.global_per_user_daily_limit`（默认 3），**只作用于批量管线**，同步管线不受此限 |
| 频控（GCRA/令牌桶） | `internal/service/reach_pipeline_ratelimit.go`、`reach_gcra_limiter.go` | 按渠道账号维度，无地区/行业维度 |
| 外发前授权闸门 | `internal/service/proactive_reach.go:243` `enforcePreSendApproval` | 全渠道唯一出口（工具/API/cron 三条路径共用），当前只判 W-1 白名单 |

### 真正的断点（本功能要补的）

1. **合规策略没有地区维度**：静默时段只有一组全局值，海外客户（如美国 TCPA 20:00–10:00）与国内（22:00–8:00）共用同一窗口，要么对海外违法，要么对国内过严。
2. **没有行业维度**：医美/金融/教育等行业对主动营销的同意要求、渠道限制都不同，现有代码无处表达，只能靠人工培训。
3. **同步发送管线无日上限**：`global_per_user_daily_limit` 只在批量管线生效，AI 工具单条外发（`reach.send`）与 proactive 按客户触达都不受任何日上限约束。
4. **同意（opt-in）不可表达**：`RequireOptIn` 这类「必须先有明确同意」的判据在代码里不存在，只有 DNC（事后退订）这一半。
5. **合规审计无判定语义**：`reach_compliance_log` 只有 channel/recipient，事后无法回答「这次为什么被拒/为什么延后」，也无地区/行业/规则域可查。
6. **策略只能改代码**：没有任何配置入口，运维改策略必须发版。

### 本功能不覆盖（明确边界）

- 不实现「自动从客户档案推导地区」——`model.Customer` 无 region/country 字段（已核验），地区来源为：请求显式入参 > 参数中心默认地区。客户档案补地区是另一张卡（涉及客户模型与导入链路）。
- 不改 DNC 与 W-1 白名单的既有语义，只在它们**之后**追加一层策略判定。
- 不覆盖浏览器桥接渠道的平台侧风控（那是 B1 的范围）。

## User Scenarios & Testing

### User Story 1 - 地区差异化静默时段（Priority: P0）

运营按目标市场配置合规策略：美国客户 20:00–10:00（当地时区以外按策略窗口）不得推送，国内维持 22:00–8:00。配置写进参数中心即生效，无需发版。

**Why this priority**：跨市场静默时段错误是可直接被投诉/处罚的合规事故，且当前代码结构上无法表达。

**Independent Test**：写入策略 JSON（`regions.US.quiet_hours_start=20/end=10`），以 `metadata.region=US` 在 CST 21:00 触发一次外发，同步管线应返回 `deferred=true` 且 `deferred_at` 为次日 10:00；同一时刻以 `region=CN` 外发应放行。

**Acceptance Scenarios**:

1. Given 策略配置 `US: quiet_hours 20–10`，When 21:00 CST 发往 `region=US` 的客户，Then 判定 `deferred`，原因码 `quiet_hours`，`send_at` 为当地下一次释放点。
2. Given 同一策略，When 11:00 CST 发往 `region=US`，Then 放行，原因码 `allowed`。

### User Story 2 - 行业渠道限制与同意要求（Priority: P0）

医美行业配置「仅允许企业微信 + 必须 opt-in」。未带同意的短信外发必须被拒，并在审计表中留痕（渠道、原因、行业、规则域）。

**Independent Test**：策略 `industries.medical.allowed_channels=["wecom"], require_opt_in=true`；发一条 `industry=medical, channel=sms, opt_in=false`，管线返回失败，错误串含 `compliance_policy_denied: channel_not_allowed`。

**Acceptance Scenarios**:

1. Given 医美仅允许企微，When 短信外发，Then 拒绝，原因 `channel_not_allowed`。
2. Given 医美仅允许企微 + 需 opt-in，When 企微外发但 `opt_in` 缺省，Then 拒绝，原因 `opt_in_required`。
3. Given 同上，When 企微外发且 `opt_in=true`，Then 放行。

### User Story 3 - 地区日上限（Priority: P1）

按地区限制同一客户的当日触达次数（如 `US: daily_limit=1`）。超限拒发并留痕，不消耗新的触达。

**Independent Test**：`daily_limit=1`；同一客户第二次外发被拒，原因 `daily_limit`，且计数只在**成功委派发送**后自增（被拒的请求不自增，否则拒发会烧掉配额、形成自锁）。

**Acceptance Scenarios**:

1. Given 上限 1 且当日已成功 1 次，When 再次外发，Then 拒绝，原因 `daily_limit`。
2. Given 一次被策略拒绝的外发，Then 当日计数不变。

### User Story 4 - 默认零行为变更（Priority: P0）

未配置策略（参数缺省/空串）时，系统行为与升级前逐字节一致：不新增拒绝、不新增延后、不多打日志。

**Independent Test**：不写 `reach.compliance_policy_json`，跑既有 `reach_send_pipeline_*` 全量用例，全绿；新步骤在 `StepResults` 里标记 `skipped`，`Success` 不受影响。

## Requirements

- **FR-1** 策略文档承载于参数中心 `reach/compliance_policy_json`（ValueType=json，写侧已由 `validateValue` 校验 JSON 语法）。
- **FR-2** 策略结构：`enabled`、`default_region`、`unknown_region_deny`、`regions{规则}`、`industries{规则}`；单条规则支持 `allowed_channels`（白名单，非空即生效）、`denied_channels`、`quiet_hours_start/_end`（0–23，CST 窗口语义与既有静默时段一致）、`daily_limit`（0=不限）、`require_opt_in`。
- **FR-3** 判定顺序：拒绝类（渠道拒绝/白名单外/缺同意）优先于延后类（静默时段）优先于限额类（日上限），一次判定只回一个原因码。
- **FR-4** 同步发送管线新增 `compliance_policy` 步骤，位置在 `permission`（含 DNC 与全局静默时段）之后、`rate_limit` 之前。
- **FR-5** proactive 同步外发路径（工具/API/cron 共用出口 `enforcePreSendApproval`）同样受策略约束，拒不延后（同步路径无延迟队列，返回 `reach_compliance_denied`）。
- **FR-6** 判定与拒绝必须落入 `reach_compliance_log`，新增 `decision`/`reason`/`region`/`industry`/`policy_scope` 五列（GORM AutoMigrate，模型已由 `RegisterExtraModels` 登记）。
- **FR-7** 策略文档损坏（JSON 非法/语义越界）时按「未启用」处理并 **ERROR 留痕一次**，不得静默吞掉。
- **FR-8** 日上限计数只在发送真正委派给渠道适配器后自增（`RecordSent`），被拒请求不计。
- **FR-9** 全部新增判据必须有单测覆盖；不写 DB 的用例不得依赖测试库。

## Key Entities

- **CompliancePolicy**（值对象，JSON 承载）：策略文档根。
- **ComplianceRule**：地区或行业的单条规则。
- **ComplianceDecision**：一次判定结果（allowed/deferred/send_at/reason/region/industry/policy_scope）。
- **ReachComplianceLog**（既有实体，扩 5 列）：合规留痕。

## Success Criteria

- **SC-1** 四类拒绝/延后原因码均有可复现的用例（`channel_denied`/`channel_not_allowed`/`opt_in_required`/`quiet_hours`/`daily_limit`/`region_unruled`）。
- **SC-2** 未配置策略时 `internal/service` 既有用例零回归。
- **SC-3** `go build ./...`、`go vet ./internal/service/ ./internal/app/`、`go test ./internal/service/ -run 'Compliance'` 全绿。
- **SC-4** `make audit` 中参数三道门（readpoints / defaults / keystyle）对新键放行。
