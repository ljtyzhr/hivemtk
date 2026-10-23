# Delivery Report: 主动触达合规策略（E1）

**Feature**: `005-reach-compliance-policy` ｜ **交付日期**: 2026-10-23 ｜ **Spec/Plan/Tasks**: 同目录

## 一句话

调研文档 P0 项 **E1（主动触达合规策略可配置）** 从零落地：策略以 JSON 存参数中心，判定器挂在
**两个外发出口**（同步发送管线新步骤 + proactive 唯一出口），判定结果落审计表，**未配置时零行为变更**。

## 交付物清单

| 类型 | 文件 | 行数/规模 | 说明 |
|---|---|---|---|
| 新增 | `user-server/internal/service/reach_compliance_policy.go` | ~700 | 策略解析/校验/规范化/判定/计数/全局单例 |
| 新增 | `user-server/internal/service/reach_compliance_policy_test.go` | 18 用例 | 解析/校验/六类判定/计数口径/缓存失效 |
| 新增 | `user-server/internal/service/reach_send_pipeline_compliance_policy_test.go` | 7 用例 | 步骤位置/nil 零影响/拒绝/延后/入队失败/计数 |
| 新增 | `user-server/internal/service/proactive_reach_compliance_test.go` | 6 用例 | proactive 出口放行/拒绝/顺序（授权先于合规） |
| 修改 | `internal/service/reach_send_pipeline.go` | +~30 | 步骤常量、默认序列、配置字段、哨兵错误 |
| 修改 | `internal/service/reach_send_pipeline_steps.go` | +~120 | `runCompliancePolicy`、`deferSend` 复用、`RecordSent` |
| 修改 | `internal/service/reach_send_pipeline_compliance.go` | +~60 | `recordDecision`、`recordComplianceDecision`、批量透传新列 |
| 修改 | `internal/service/proactive_reach.go` | +~80 | `enforceOutboundGates`、`enforceCompliance`、请求三字段、计数 defer |
| 修改 | `internal/service/config_param_seeds.go` | +7 | 种子 `reach/compliance_policy_json` |
| 修改 | `internal/model/reach_compliance_log.go` | +18 | 审计 5 列 |
| 修改 | `internal/app/reach_tool_wiring.go` | +8 | 注入全局判定器（与 proactive 共用同一份计数） |
| 修改 | `internal/service/reach_send_pipeline_test.go` | 2 处 | 步数断言 8→9（新步骤的必然结果） |
| 修改 | `.env-example` | +9 | 顺带补 `TRUSTED_PROXIES` 文档（见「顺带修复」） |
| 文档 | `specs/005-reach-compliance-policy/{spec,plan,tasks,delivery}.md` | — | spec-kit 产物 |
| 文档 | `CHANGELOG.md` | +9 | 未发布段新增条目 |

## 验证读数（本机实测，2026-10-23）

| 命令 | 结果 |
|---|---|
| `cd user-server && go build ./...` | ✅ 通过（无输出） |
| `go vet ./internal/service/ ./internal/app/` | ✅ 通过 |
| `go test ./internal/service/ -run 'Compliance\|SendPipeline' -count=1` | ✅ 全绿（含既有 send pipeline 全量） |
| `golangci-lint run ./internal/service/... ./internal/app/...` | ⚠️ 剩 1 条，来自**他人在途文件** `internal/service/inbox_ingress.go:771` 的 gofmt（本轮未改该文件，如实记录） |
| `make audit` | ✅ 全绿（含参数三道门：readpoints 227 条有读取点/0 未声明；defaults 196 条比对一致；keystyle 0 违规） |
| `python3 scripts/check-env-coverage.py` | ✅ 红 0（修 `TRUSTED_PROXIES` 之前为 1） |
| `go test ./internal/service/ -count=1`（全量） | ❌ 2571 例失败，**全部**为 `初始化进程级测试库失败: SASL auth ... SQLSTATE 28P01`；逐例核对：唯一非 DB 失败是 `TestDefaultSendPipelineConfig_AllComponents`（步数断言，已修） |

**测试库说明**（按 CLAUDE.md 3.5）：本机 8202 端口可连，但 `hivemtk/.env` 的 `POSTGRES_PASSWORD`
与该实例实际口令不匹配（`psql` 直连同样 28P01），属**环境失败**，不是代码缺陷。
合规相关用例全部刻意不依赖测试库（判定器是纯逻辑、DNC 用桩），因此在无库环境可完整验证。

## 设计要点（为什么这么做）

1. **承载形态选参数中心 JSON**：写侧已有 JSON 语法校验、读侧已有 60s TTL 缓存与租户覆盖、变更已有审计。
   新造 `compliance_policies` 表要多 4 文件 + 1 迁移 + 前端两页 + 契约门，收益仅「结构化查询」，
   而策略是低频运维配置（个位数条数）。代价是语义校验只能靠代码，已由 `ValidateCompliancePolicy`
   在解析期承担并留 ERROR。
2. **判定器做全局单例**：`ProactiveReachService` 有 4 个构造点，逐个注入＝「漏一个就静默失效」。
   本仓外发闸门（`reach_gate_wiring.go`）与 WS 租户桶（A14）都踩过这个形状。判定点只有一处，装配点为零。
3. **判定与计数分离**：`Evaluate` 在发出前、`RecordSent` 在**成功委派后**。合在一起会让被拒的请求
   烧掉配额——连续几次拒发之后，该客户当天再也发不出去（自锁）。
4. **拒绝优先于延后**：被地区明令禁止的渠道，不该因为「现在是静默时段」进延迟队列然后 8 小时后照发。
5. **默认零行为变更**：未配置/`enabled=false` ⇒ `policy_disabled` 直通，且不写审计表（不给审计灌行）。
   这是能在生产灰度的前提。

## 已知边界（写清不做什么）

- 地区来源是**请求入参**（管线 `metadata.region` / proactive `region`）> 策略 `default_region`。
  `model.Customer` 没有 region 字段，客户档案补地区属另一张卡（涉及客户模型与导入链路）。
- 日上限计数是**进程内**（重启归零、多实例不共享）。接口已抽出（`ComplianceUsageCounter`），
  上云时换 Redis ZSET 实现即可，判定器不用改。
- 客户面覆盖：同步发送管线（AI 工具 `reach.send` 等）+ proactive 出口（工具/API/cron）。
  **不覆盖**：批量管线 `ReachPipelineService` 的独立频控链（它有 `global_per_user_daily_limit`）、
  SOP 节点直连渠道、浏览器桥接扩展本身。
- 静默窗口使用 CST 小时语义（与既有 R-4 一致），**不是**收件人当地时间。跨时区精确到当地时间
  需要客户时区数据，属后续卡（先在策略里用「按市场分地区」近似）。

## 顺带修复（规则 2：发现问题当轮修）

`make audit` 在 HEAD `b54c1e1b` 上即红：`check-env-coverage.py` 报
`UNDOCUMENTED TRUSTED_PROXIES <- user-server/internal/middleware/client_ip_guard.go`——
该中间件（同一 commit 落地）读 `TRUSTED_PROXIES`，但 `.env-example` / `DEPLOYMENT_GUIDE.md` / 基线
三处都没有它。已补进 `.env-example`（含「不配时来源 IP 会全部塌成反代 IP 且不报错」的说明）。
复跑：红 0。

## 下一轮建议（承接 `docs/开发计划-改进清单-2026-10-23.md` Wave 2）

A2-a 依赖策略门 → A2-b SBOM 产物断言 → A2-c WhatsApp 接口化 → A4-a 核心域分档覆盖率 → B1-a 选择器失效检测。
五项都是「判据型」改动，不碰业务语义，建议合成一个切片一次交付。
