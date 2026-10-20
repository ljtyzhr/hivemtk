# user-server Model / 表 清单（全实体审计工作表）

> **用途**：本文件是「五层架构 + GORM 驱动 + SaaS 多租户」逐实体审计的唯一施工清单。
> 第三步每轮只从本表取**一个**实体，检查 4.1 五层 / 4.2 GORM / 4.3 多租户 / 4.4 僵尸代码，
> 处理完在该行「审计结论」列打勾，再自动进入下一个。**禁止用脚本批量改码**。

> **生成方式**（只读抽取，不改任何源码）：表名用 GORM 自己的 `schema.Parse` + `NamingStrategy`
> 求出（与 `internal/service/tenant_extra_models_test.go` 同一口径），不是猜的复数化；
> 「fresh 建表」列取自**当前 HEAD 新建一个空库跑一次真实 `db.AutoMigrate()`** 产出的 340 张表
> （探针 `user-server/tmp/probe/fresh`，gitignore 覆盖，不随仓交付）；
> 「dev 库」列取自开发库 `user_db`（366 张，含历史残留）。

---

## 一、判据速查（审计时逐条对）

| # | 判据 | 事实源 | 违反时的症状 |
|---|------|--------|-------------|
| 4.1 | Router→Controller→Service→Repository→Model 单向，禁止跨层 | `docs/architecture/GO_FIVE_LAYER_ARCHITECTURE.md`；`scripts/check-architecture.sh` | Controller 直访 Repository / Service 持 `*gorm.DB` / Repository 调 Service / Model 带业务方法 |
| 4.1b | 每域四文件齐全，DI 在 `main.go`，router 只做映射 | `CLAUDE.md` 五层节 | 路由内联闭包、缺 repository |
| 4.2 | 数据访问只走 GORM，业务包 `.Exec(`/`.Raw(` 归零 | `user-server/scripts/check-gorm-only.sh` | 裸 SQL 绕过 GORM / 绕过隔离 callback |
| 4.3 | 租户表判据 = **表上有无 `tenant_id` 列**（内省，无第二份豁免清单） | `internal/pkg/tenancyidx/tenancyidx.go:264-311`；`docs/architecture/SaaS_MULTI_TENANT.md` §三.2 | 漏加列 ⇒ 全量放行 ⇒ 跨租户读写 |
| 4.3b | 租户表唯一索引须复合化为 `(tenant_id, 原列...)`；`global_uniq_*` 前缀是刻意的跨租户全局唯一 | `internal/pkg/tenancyidx/tenancyidx.go:156-192` | 单列唯一 ⇒ 第二个租户插不进 / 跨租户撞名 |
| 4.3c | 待办/队列/追踪行写时必须带 `tenant_id`（行自证） | `docs/architecture/SaaS_MULTI_TENANT.md` §四 | 消费端读行恢复不出租户 ⇒ 落默认租户 |
| 4.3d | 写路径必须能填出 `tenant_id`（只读侧加 WHERE 不算隔离） | `docs/CONFIG_DB_DRIVEN_PLAN.md:1593` | 下一个租户覆盖上一个人的数据 |
| 4.4 | 僵尸代码：零生产 importer + 无动态注册（反射/字符串/embed/路由表/init） | 本仓既有判例见 `docs/architecture/adr/` | 僵尸包冒充「租户隔离的第二套机制」 |
| — | **建表登记**：有生产写入路径的表必须被**三处之一**登记 —— `db.allModels()` / `RegisterExtraModels` / **Go 版本化迁移**（`internal/migration/migrations/*.go` 里的 AutoMigrate，启动时由 `cmd/api/main.go:437` 的 `NewMigrationServiceDefault` 执行） | `internal/pkg/db/migrate.go:22-467`；`internal/migration/migrations/initial_schema.go` | 全新部署不建表 ⇒ 写入静默失败 |

---

## 二、实体清单

列含义：**fresh 建表** = 当前代码跑一次 AutoMigrate 会不会建出这张表；
**dev 库** = 开发库 `user_db` 里有没有这张表；
**登记** = `allModels()` / `RegisterExtraModels` / **Go 版本化迁移** / **未登记**。

> ⚠️ **「登记」列的口径订正（2026-10-21，实体 #1/#2 审计时发现）**：原口径只有
> `allModels()` 与 `RegisterExtraModels` 两处，**漏了 Go 版本化迁移**
> （`internal/migration/migrations/*.go` 里的 AutoMigrate，启动时执行）。
> 漏判的后果是**假红**：实体 #1 `LiveCodeClickLog` / #2 `QRCodeClickLog` 被标成
> 「未登记 / fresh 建表 ❌」，而实际上表由 `v3_20_live_code_click_logs_migration.go`
> 建出（已 register 进 `initial_schema.go:175`），全新部署照样建得出。
> 判定「有没有被建表」必须看**三处齐全与否**，不是只看 `allModels()`。

| # | 优先级 | 实体 | 表 | 模型文件 | tenant_id | 登记 | fresh 建表 | dev 库 | 审计结论 |
|---|--------|------|-----|----------|-----------|------|-----------|-------|----------|
| 1 | P0 | `LiveCodeClickLog` | `livecode_click_log` | `internal/model/live_code_click_log.go` | ✅ | Go 迁移 v3.20.0 | ✅ | ✅ | ✅ 4.1 五层齐全（router admin_routes.go:84 → controller/live_code.go:278 → service/live_code.go:214 → repository/live_code_click_log.go:34 → model）；4.2 全 GORM 无裸 SQL；4.3 tenant_id 齐 + ctx 全程透传（WithContext）→ 租户表自动隔离；4.4 有写入路径非僵尸。**原判「未登记」是误判**：表由 `internal/migration/migrations/v3_20_live_code_click_logs_migration.go` 的 AutoMigrate 建（已 register 到 initial_schema.go:175，cmd/api/main.go:437 启动时执行）⇒ fresh 部署也建得出。清单「登记」列口径须改为「allModels/RegisterExtraModels/**Go 迁移**」三选一 |
| 2 | P0 | `QRCodeClickLog` | `qr_code_click_log` | `internal/model/live_code_click_log.go` | ✅ | Go 迁移 v3.20.0 | ✅ | ✅ | ✅ 同 #1（同一迁移、同一 service 写路径）。**表名口径订正**：`v3_20_..._migration.go` 头部注释写的是 `live_code_click_logs` / `qr_code_click_logs`（复数带下划线），与 model 的 `TableName()` 实际值 `livecode_click_log` / `qr_code_click_log` 不符 ⇒ 注释事实源漂移，但 AutoMigrate 尊重 TableName ⇒ 实际建表正确（dev 库实测即此二表名） |
| 3 | P0 | `SmsJobMetric` | `sms_job_metrics` | `internal/model/sms_tracking.go` | ✅ | **未登记** | ❌ | ✅ | ☐ |
| 4 | P0 | `WechatAccount` | `wechat_accounts` | `internal/model/wechat_account.go` | ✅ | **未登记** | ❌ | ✅ | ☐ |
| 5 | P1 | `FeatureFlagAuditLog` | `feature_flag_audit_logs` | `internal/model/feature_flag.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 6 | P1 | `FeatureFlagEvalLog` | `feature_flag_eval_logs` | `internal/model/feature_flag.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 7 | P1 | `FeatureFlag` | `feature_flags` | `internal/model/feature_flag.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 8 | P1 | `SystemConfig` | `system_config` | `internal/model/system_config.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 9 | P1 | `SystemConfigKV` | `system_config_kv` | `internal/model/system_config_kv.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 10 | P1 | `TenantSubscriptionMirror` | `tenant_subscription_mirror` | `internal/model/tenant_subscription_mirror.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 11 | P2 | `ABConversionEvent` | `ab_conversion_events` | `internal/ops/model/ab_experiment.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 12 | P2 | `ABExperimentResult` | `ab_experiment_results` | `internal/ops/model/ab_experiment.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 13 | P2 | `ABExperiment` | `ab_experiments` | `internal/ops/model/ab_experiment.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 14 | P2 | `ABTestMetric` | `ab_test_metrics` | `internal/model/confidence.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 15 | P2 | `ABTestStat` | `ab_test_stats` | `internal/model/humanize_score.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 16 | P2 | `ABTest` | `ab_tests` | `internal/model/confidence.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 17 | P2 | `ABVariant` | `ab_variants` | `internal/ops/model/ab_experiment.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 18 | P2 | `Account` | `account` | `internal/model/account.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 19 | P2 | `AfterSale` | `after_sales` | `internal/model/aftersale.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 20 | P2 | `AgentKBBinding` | `agent_kb_bindings` | `internal/model/agent_kb_binding.go` | ✅ | allModels | ✅ | ✅ | ✅ 4.1/4.2/4.4；❌4.3 缺建表登记 → 已修：allModels() 补 &model.AgentKBBinding{}；UNIQUE(agent_id,kb_id) 已复合化为 (tenant_id,agent_id,kb_id) |
| 21 | P2 | `AgentStatus` | `agent_statuses` | `internal/model/agent_status.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 22 | P2 | `AggregationWatermark` | `aggregation_watermarks` | `internal/model/msg_hourly_summary.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 23 | P2 | `AIAgent` | `ai_agents` | `internal/model/ai_agent.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 24 | P2 | `AIGenerationRecord` | `ai_generation_records` | `internal/content/model/ai_content.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 25 | P2 | `AISalesLog` | `ai_sales_logs` | `internal/model/ai_sales_champion.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 26 | P2 | `AISuggestion` | `ai_suggestions` | `internal/model/ai_suggestion.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 27 | P2 | `AIToolAccountBinding` | `ai_tool_account_bindings` | `internal/model/ai_tool_config.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 28 | P2 | `AIToolConfig` | `ai_tool_configs` | `internal/model/ai_tool_config.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 29 | P2 | `AIToolStats` | `ai_tool_stats` | `internal/model/ai_tool_config.go` | — | **未登记** | ❌ | — | ☐ |
| 30 | P2 | `AlertHistory` | `alert_histories` | `internal/model/alert_rule.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 31 | P2 | `AlertRuleManager` | `alert_rule_managers` | `internal/model/alert_rule.go` | — | **未登记** | ❌ | — | ☐ |
| 32 | P2 | `AlertRule` | `alert_rules` | `internal/model/alert_rule.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 33 | P2 | `APILog` | `api_logs` | `internal/model/stats.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 34 | P2 | `ApprovalRequest` | `approval_requests` | `internal/model/approval_request.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 35 | P2 | `AssetBundleMessage` | `asset_bundle_messages` | `internal/model/asset_bundle.go` | — | **未登记** | ❌ | — | ☐ |
| 36 | P2 | `AssetBundleVersionLog` | `asset_bundle_version_logs` | `internal/model/asset_bundle.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 37 | P2 | `AssetBundle` | `asset_bundles` | `internal/model/asset_bundle.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 38 | P2 | `AutomationRule` | `automation_rules` | `internal/model/customer_segment.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 39 | P2 | `Backup` | `backups` | `internal/model/backup.go` | — | allModels | ✅ | ✅ | ☐ |
| 40 | P2 | `BadCase` | `bad_cases` | `internal/model/bad_case.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 41 | P2 | `BanditArm` | `bandit_arms` | `internal/model/feedback_loop.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 42 | P2 | `BanditRefluxLog` | `bandit_reflux_log` | `internal/model/bandit_reflux_log.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 43 | P2 | `BatchOperationHistory` | `batch_operation_histories` | `internal/content/model/batch_operation_history.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 44 | P2 | `Bill` | `bills` | `internal/model/bill.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 45 | P2 | `BridgeAccount` | `bridge_accounts` | `internal/model/bridge_account.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 46 | P2 | `BrowserAuditDigest` | `browser_audit_digests` | `internal/browser_automation/model/audit_digest.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 47 | P2 | `BrowserAuditPruneRun` | `browser_audit_prune_runs` | `internal/browser_automation/model/audit_digest.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 48 | P2 | `BrowserCommandLog` | `browser_command_log` | `internal/browser_automation/model/command_log.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 49 | P2 | `BrowserCronTrigger` | `browser_cron_triggers` | `internal/browser_automation/model/cron.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 50 | P2 | `BrowserLLMPlan` | `browser_llm_plans` | `internal/browser_automation/model/llm_plan.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 51 | P2 | `BrowserOutreachDedupe` | `browser_outreach_dedupe` | `internal/browser_automation/model/outreach_dedupe.go` | ✅ | allModels | ✅ | ✅ | ✅ 4.1/4.2/4.4；❌4.3 缺建表登记 → 已修：allModels() 补 &browsermodel.BrowserOutreachDedupe{}；写路径只 Warn ⇒ 去重层静默失效 |
| 52 | P2 | `BrowserOutreachReceipt` | `browser_outreach_receipts` | `internal/browser_automation/model/outreach_receipt.go` | ✅ | allModels | ✅ | ✅ | ✅ 4.1/4.2/4.4；❌4.3 缺建表登记 → 已修：allModels() 补 &browsermodel.BrowserOutreachReceipt{}；回执行写不进去 ⇒ 触达台账整层不存在 |
| 53 | P2 | `BrowserProfileHealth` | `browser_profile_health` | `internal/browser_automation/model/profile_health.go` | ✅ | allModels | ✅ | ✅ | ✅ 4.1/4.2/4.4；❌4.3 缺建表登记 → 已修：allModels() 补 &browsermodel.BrowserProfileHealth{}；熔断状态落不下去 ⇒ 风控闸永不关；platform 唯一索引已复合化 |
| 54 | P2 | `BrowserSession` | `browser_sessions` | `internal/browser_automation/model/session.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 55 | P2 | `BrowserStep` | `browser_steps` | `internal/browser_automation/model/step.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 56 | P2 | `BrowserTask` | `browser_tasks` | `internal/browser_automation/model/task.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 57 | P2 | `BrowserWriteClaim` | `browser_write_claims` | `internal/browser_automation/model/write_claim.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 58 | P2 | `BusinessMemory` | `business_memories` | `internal/model/memory.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 59 | P2 | `CardButton` | `card_buttons` | `internal/model/rich_card.go` | — | **未登记** | ❌ | — | ☐ |
| 60 | P2 | `ChampionBaseline` | `champion_baselines` | `internal/model/humanize_score.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 61 | P2 | `ChampionDialogue` | `champion_dialogues` | `internal/model/feedback_loop.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 62 | P2 | `ChampionPhrase` | `champion_phrases` | `internal/model/humanize_score.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 63 | P2 | `ChannelAgentBinding` | `channel_agent_bindings` | `internal/model/ai_agent.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 64 | P2 | `ChatChannel` | `chat_channels` | `internal/model/chat_channel.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 65 | P2 | `ChatInfo` | `chat_infos` | `internal/model/unified_message.go` | — | **未登记** | ❌ | — | ☐ |
| 66 | P2 | `ChurnModelConfig` | `churn_model_configs` | `internal/ops/model/churn_prediction.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 67 | P2 | `ChurnPrediction` | `churn_predictions` | `internal/ops/model/churn_prediction.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 68 | P2 | `ChurnScore` | `churn_scores` | `internal/model/churn_score.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 69 | P2 | `ChurnStatistics` | `churn_statistics` | `internal/ops/model/churn_prediction.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 70 | P2 | `ChurnWarning` | `churn_warnings` | `internal/ops/model/churn_prediction.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 71 | P2 | `ClueEngagementEvent` | `clue_engagement_events` | `internal/model/clue_score.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 72 | P2 | `ClueScore` | `clue_scores` | `internal/model/clue_score.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 73 | P2 | `Clue` | `clues` | `internal/model/clue.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 74 | P2 | `CommunityGroup` | `community_groups` | `internal/model/community.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 75 | P2 | `CommunityMember` | `community_members` | `internal/model/community.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 76 | P2 | `CommunityMessage` | `community_messages` | `internal/model/community.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 77 | P2 | `ConfidenceCalibration` | `confidence_calibrations` | `internal/model/confidence.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 78 | P2 | `ConfidenceSignal` | `confidence_signals` | `internal/model/confidence.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 79 | P2 | `ConfigParamAuditLog` | `config_param_audit_logs` | `internal/model/config_param.go` | — | allModels | ✅ | ✅ | ☐ |
| 80 | P2 | `ConfigParamGroup` | `config_param_groups` | `internal/model/config_param_group.go` | — | allModels | ✅ | ✅ | ☐ |
| 81 | P2 | `ConfigParam` | `config_params` | `internal/model/config_param.go` | — | allModels | ✅ | ✅ | ☐ |
| 82 | P2 | `ConversionFunnel` | `conversion_funnels` | `internal/model/ai_sales_champion.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 83 | P2 | `CronJobLease` | `cron_job_leases` | `internal/model/cron_job_lease.go` | — | allModels | ✅ | ✅ | ☐ |
| 84 | P2 | `CSATSurvey` | `csat_surveys` | `internal/model/csat.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 85 | P2 | `CustomReport` | `custom_reports` | `internal/ops/model/custom_report.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 86 | P2 | `CustomerChannel` | `customer_channels` | `internal/model/customer_channel.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 87 | P2 | `CustomerDoNotContact` | `customer_do_not_contact` | `internal/model/customer_do_not_contact.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 88 | P2 | `CustomerEvent` | `customer_events` | `internal/model/customer_event.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 89 | P2 | `CustomerLongTermMemory` | `customer_long_term_memory` | `internal/model/customer_long_term_memory.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 90 | P2 | `CustomerRFM` | `customer_rfm` | `internal/model/customer_rfm.go` | ✅ | allModels | ✅ | ✅ | ✅ 4.1/4.2/4.4；❌4.3 缺建表登记 → 已修：allModels() 补 &model.CustomerRFM{}；customer_id 唯一索引已复合化 |
| 91 | P2 | `CustomerSegment` | `customer_segments` | `internal/model/customer_segment.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 92 | P2 | `CustomerServiceAgent` | `customer_service_agents` | `internal/model/ai_agent.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 93 | P2 | `CustomerSession` | `customer_sessions` | `internal/model/customer_session.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 94 | P2 | `CustomerTagAssignment` | `customer_tag_assignments` | `internal/model/customer_tag_assignment.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 95 | P2 | `CustomerTag` | `customer_tags` | `internal/model/customer_tag.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 96 | P2 | `Customer` | `customers` | `internal/model/customer.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 97 | P2 | `DailyStats` | `daily_stats` | `internal/model/stats.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 98 | P2 | `DashboardScreen` | `dashboard_screens` | `internal/ops/model/dashboard_screen.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 99 | P2 | `DashboardWidget` | `dashboard_widgets` | `internal/ops/model/dashboard_screen.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 100 | P2 | `DataScopeRow` | `data_scope_rows` | `internal/model/data_scope.go` | — | **未登记** | ❌ | — | ☐ |
| 101 | P2 | `DialogueEvalRun` | `dialogue_eval_runs` | `internal/model/dialogue_eval_run.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 102 | P2 | `DialogueMemory` | `dialogue_memories` | `internal/model/ai_sales_champion.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 103 | P2 | `Dictionary` | `dictionaries` | `internal/model/dictionary.go` | — | allModels | ✅ | ✅ | ☐ |
| 104 | P2 | `DictionaryTransition` | `dictionary_transitions` | `internal/model/dictionary.go` | — | allModels | ✅ | ✅ | ☐ |
| 105 | P2 | `DingTalkAppAccount` | `dingtalk_app_accounts` | `internal/model/dingtalk_app_account.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 106 | P2 | `DomainBlacklist` | `domain_blacklist` | `internal/model/domain_pool.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 107 | P2 | `DomainHealthLog` | `domain_health_log` | `internal/model/domain_pool.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 108 | P2 | `DomainPool` | `domain_pool` | `internal/model/domain_pool.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 109 | P2 | `DouyinCardActivity` | `douyin_card_activities` | `internal/model/douyin_card_activity.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 110 | P2 | `DouyinCard` | `douyin_cards` | `internal/model/douyin_card.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 111 | P2 | `EmailDraft` | `email_drafts` | `internal/model/email_draft.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 112 | P2 | `EmailJobMetric` | `email_job_metrics` | `internal/model/email_tracking.go` | ✅ | allModels | ✅ | ✅ | ✅ 4.1/4.2/4.4；❌4.3 缺建表登记 → 已修：allModels() 补 &model.EmailJobMetric{}；job_id 唯一索引已复合化 |
| 113 | P2 | `EmailJobs` | `email_jobs` | `internal/model/email_jobs.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 114 | P2 | `EmailList` | `email_list` | `internal/model/email_list.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 115 | P2 | `EmailSend` | `email_sends` | `internal/model/email_send.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 116 | P2 | `EmailSmtp` | `email_smtp` | `internal/model/email_smtp.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 117 | P2 | `EmailTrackingEvent` | `email_tracking_events` | `internal/model/email_tracking.go` | ✅ | allModels | ✅ | ✅ | ✅ 4.1/4.2/4.4；❌4.3 缺建表登记 → 已修：allModels() 补 &model.EmailTrackingEvent{}；event_id 唯一索引已复合化 |
| 118 | P2 | `EmailUnsubscribe` | `email_unsubscribes` | `internal/model/email_unsubscribe.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 119 | P2 | `EmbeddingProviderConfig` | `embedding_provider_configs` | `internal/model/rag_product.go` | ✅ | **未登记** | ❌ | — | ☐ |
| 120 | P2 | `ExternalCustomer` | `external_customers` | `internal/model/integration.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 121 | P2 | `ExternalImportJob` | `external_import_jobs` | `internal/model/kb_merchant.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 122 | P2 | `ExternalKBConnector` | `external_kb_connectors` | `internal/model/kb_connector.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 123 | P2 | `ExternalKBSyncItem` | `external_kb_sync_items` | `internal/model/kb_connector.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 124 | P2 | `ExternalOrder` | `external_orders` | `internal/model/integration.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 125 | P2 | `ExternalProduct` | `external_products` | `internal/model/integration.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 126 | P2 | `FAQEntry` | `faq_entries` | `internal/model/faq_entry.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 127 | P2 | `FeedbackEvent` | `feedback_events` | `internal/model/feedback_loop.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 128 | P2 | `FeedbackRecordORM` | `feedback_records` | `internal/model/feedback_record.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 129 | P2 | `FeedbackSignal` | `feedback_signals` | `internal/model/feedback_loop.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 130 | P2 | `FeishuAccount` | `feishu_accounts` | `internal/model/feishu.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 131 | P2 | `FeishuCustomer` | `feishu_customers` | `internal/model/feishu.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 132 | P2 | `FeishuMessage` | `feishu_messages` | `internal/model/feishu.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 133 | P2 | `FlowExecution` | `flow_executions` | `internal/content/model/marketing_flow.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 134 | P2 | `GeoAlert` | `geo_alerts` | `internal/geo/model/alert.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 135 | P2 | `GeoAPICall` | `geo_api_calls` | `internal/geo/model/verification.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 136 | P2 | `GeoArticle` | `geo_articles` | `internal/geo/model/content.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 137 | P2 | `GeoCompetitor` | `geo_competitors` | `internal/geo/model/competitor.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 138 | P2 | `GeoConfig` | `geo_config` | `internal/geo/model/config.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 139 | P2 | `GeoContentTask` | `geo_content_tasks` | `internal/geo/model/query_chain.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 140 | P2 | `GeoCrawlerVisit` | `geo_crawler_visits` | `internal/geo/model/crawler_visit.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 141 | P2 | `GeoDailyStat` | `geo_daily_stats` | `internal/geo/model/daily_stats.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 142 | P2 | `GeoDict` | `geo_dicts` | `internal/geo/model/dict.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 143 | P2 | `GeoEntity` | `geo_entities` | `internal/geo/model/entity.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 144 | P2 | `GeoEntityRelation` | `geo_entity_relations` | `internal/geo/model/entity.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 145 | P2 | `GeoIndexTracking` | `geo_index_trackings` | `internal/geo/model/index_tracking.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 146 | P2 | `GeoJobRun` | `geo_job_runs` | `internal/geo/model/job_run.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 147 | P2 | `GeoKeywordGroup` | `geo_keyword_groups` | `internal/geo/model/keyword.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 148 | P2 | `GeoKeyword` | `geo_keywords` | `internal/geo/model/keyword.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 149 | P2 | `GeoKnowledgeDocument` | `geo_knowledge_documents` | `internal/geo/model/knowledge.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 150 | P2 | `GeoOptimization` | `geo_optimizations` | `internal/geo/model/content.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 151 | P2 | `GeoPlatformAccount` | `geo_platform_accounts` | `internal/geo/model/config.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 152 | P2 | `GeoProbeRun` | `geo_probe_runs` | `internal/geo/model/probe.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 153 | P2 | `GeoPublishRecord` | `geo_publish_records` | `internal/geo/model/config.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 154 | P2 | `GeoPushRecord` | `geo_push_records` | `internal/geo/model/push_record.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 155 | P2 | `GeoPusherConfig` | `geo_pusher_configs` | `internal/geo/model/pusher_config.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 156 | P2 | `GeoQueryChain` | `geo_query_chains` | `internal/geo/model/query_chain.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 157 | P2 | `GeoSchemaTemplate` | `geo_schema_templates` | `internal/geo/model/schema_template.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 158 | P2 | `GeoSeedTerm` | `geo_seed_terms` | `internal/geo/model/seed_term.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 159 | P2 | `GeoSite` | `geo_sites` | `internal/geo/model/site.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 160 | P2 | `GeoSourceCatalog` | `geo_source_catalogs` | `internal/geo/model/source_catalog.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 161 | P2 | `GeoVerifyResult` | `geo_verify_results` | `internal/geo/model/verification.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 162 | P2 | `GeoWorkflowExecution` | `geo_workflow_executions` | `internal/geo/model/workflow.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 163 | P2 | `GeoWorkflowTemplate` | `geo_workflow_templates` | `internal/geo/model/workflow.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 164 | P2 | `GeoWorkflow` | `geo_workflows` | `internal/geo/model/workflow.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 165 | P2 | `Glossary` | `glossaries` | `internal/model/glossary.go` | — | allModels | ✅ | ✅ | ✅ 4.1/4.2/4.4；❌4.3 三处 → 已修：①model 补 tenant_id（术语是租户业务数据，原为平台表=跨租户共享品牌词）②缓存键加租户维度（原键 glossary:lang:<lang> 跨租户复用）③InvalidateAll 改逐键删本租户（原调 cache.Clear=Redis FLUSHDB，一个租户保存术语掀掉整台实例缓存）④allModels() 补登记；新增 4 条回归用例 internal/service/translation/glossary_tenant_test.go |
| 166 | P2 | `HandoffDecisionRecord` | `handoff_decisions` | `internal/model/confidence.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 167 | P2 | `HelpCenterTestRecord` | `help_center_test_records` | `internal/model/kb_workspace.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 168 | P2 | `HumanTask` | `human_tasks` | `internal/model/human_task.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 169 | P2 | `HumanizeDimensionRecord` | `humanize_dimensions` | `internal/model/humanize_score.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 170 | P2 | `HumanizeScore` | `humanize_scores` | `internal/model/humanize_score.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 171 | P2 | `InboxAssignment` | `inbox_assignments` | `internal/model/ai_sales_champion.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 172 | P2 | `InboxConversation` | `inbox_conversations` | `internal/model/ai_sales_champion.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 173 | P2 | `IntegrationAccount` | `integration_accounts` | `internal/model/integration.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 174 | P2 | `IntegrationTemplate` | `integration_templates` | `internal/model/integration_template.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 175 | P2 | `IntentExample` | `intent_examples` | `internal/model/intent_example.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 176 | P2 | `IntentLog` | `intent_records` | `internal/model/intent_log.go` | ✅ | **未登记** | ✅ | ✅ | ☐ |
| 177 | P2 | `IntentRecord` | `intent_records` | `internal/model/ai_sales_champion.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 178 | P2 | `KBChangeAuditLog` | `kb_change_audit_logs` | `internal/model/kb_release.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 179 | P2 | `KBChangeRequest` | `kb_change_requests` | `internal/model/kb_release.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 180 | P2 | `KBDocument` | `kb_documents` | `internal/model/kb_document.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 181 | P2 | `KBRelease` | `kb_releases` | `internal/model/kb_release.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 182 | P2 | `KnowledgeAPIToken` | `knowledge_api_tokens` | `internal/model/kb_merchant.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 183 | P2 | `KnowledgeBase` | `knowledge_bases` | `internal/model/knowledge_base.go` | ✅ | allModels | ✅ | ✅ | ✅ 4.1/4.2/4.4；❌4.3 缺建表登记 → 已修：allModels() 补 &model.KnowledgeBase{}；UNIQUE(kb_code) 已复合化为 (tenant_id,kb_code)；RAG 答案缓存读路径也不再撞空表 |
| 184 | P2 | `KnowledgeChunk` | `knowledge_chunks` | `internal/model/kb_workspace.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 185 | P2 | `KnowledgeDocument` | `knowledge_documents` | `internal/model/kb_workspace.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 186 | P2 | `KnowledgeFeedback` | `knowledge_feedbacks` | `internal/model/kb_merchant.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 187 | P2 | `KnowledgeHit` | `knowledge_hits` | `internal/model/unified_message.go` | — | **未登记** | ❌ | — | ☐ |
| 188 | P2 | `KnowledgeImportLog` | `knowledge_import_logs` | `internal/model/kb_workspace.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 189 | P2 | `KnowledgeOpenAPISource` | `knowledge_openapi_sources` | `internal/model/kb_workspace.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 190 | P2 | `KnowledgeSearchLog` | `knowledge_search_logs` | `internal/model/kb_workspace.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 191 | P2 | `KuaishouCardActivity` | `kuaishou_card_activities` | `internal/model/kuaishou_card_activity.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 192 | P2 | `KuaishouCard` | `kuaishou_cards` | `internal/model/kuaishou_card.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 193 | P2 | `LayerDecisionLog` | `layer_decision_logs` | `internal/model/layer_decision_log.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 194 | P2 | `LeadMiningConfig` | `lead_mining_configs` | `internal/model/lead_mining_config.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 195 | P2 | `LearningInsight` | `learning_insights` | `internal/model/learning_insight.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 196 | P2 | `LiveCodeQRStat` | `live_code_qr_stats` | `internal/model/live_code_qr_stat.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 197 | P2 | `LiveCodeQR` | `live_code_qrs` | `internal/model/live_code_qr.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 198 | P2 | `LiveCode` | `live_codes` | `internal/model/live_code.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 199 | P2 | `LLMProviderConfig` | `llm_provider_configs` | `internal/model/rag_product.go` | — | **未登记** | ❌ | — | ☐ |
| 200 | P2 | `LLMProvider` | `llm_providers` | `internal/model/llm_provider.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 201 | P2 | `LLMRoutingLog` | `llm_routing_logs` | `internal/model/llm_routing_log.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 202 | P2 | `LLMRoutingRule` | `llm_routing_rules` | `internal/model/llm_routing_rule.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 203 | P2 | `LLMWalletTxn` | `llm_wallet_txns` | `internal/model/tenant_module.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 204 | P2 | `LLMWallet` | `llm_wallets` | `internal/model/tenant_module.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 205 | P2 | `LocalAssetData` | `local_asset_data` | `internal/model/local_asset.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 206 | P2 | `LocalAssetSyncLog` | `local_asset_sync_log` | `internal/model/local_asset.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 207 | P2 | `LocalAsset` | `local_assets` | `internal/model/local_asset.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 208 | P2 | `LoginEvent` | `login_events` | `internal/model/login_event.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 209 | P2 | `LowQualitySample` | `low_quality_samples` | `internal/model/low_quality_sample.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 210 | P2 | `Macro` | `macros` | `internal/model/customer_segment.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 211 | P2 | `MarketTemplateDownload` | `market_template_downloads` | `internal/content/model/market_template.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 212 | P2 | `MarketTemplate` | `market_templates` | `internal/content/model/market_template.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 213 | P2 | `MarketingFlow` | `marketing_flows` | `internal/content/model/marketing_flow.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 214 | P2 | `MaterialCategory` | `material_categories` | `internal/content/model/material.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 215 | P2 | `Material` | `materials` | `internal/content/model/material.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 216 | P2 | `MCPCredential` | `mcp_credentials` | `internal/model/mcp_credential.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 217 | P2 | `MemoryItem` | `memory_items` | `internal/model/memory.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 218 | P2 | `Message` | `message` | `internal/model/message.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 219 | P2 | `MessageEventHistoryItem` | `message_event_history_items` | `internal/model/message_event.go` | — | **未登记** | ❌ | — | ☐ |
| 220 | P2 | `MessageHub` | `message_hub` | `internal/model/ai_sales_champion.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 221 | P2 | `MessageQueryOptions` | `message_query_options` | `internal/model/unified_message.go` | — | **未登记** | ❌ | — | ☐ |
| 222 | P2 | `MessageTrace` | `message_trace` | `internal/model/message_trace.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 223 | P2 | `MigrationCheckpoint` | `migration_checkpoints` | `internal/model/upgrade.go` | — | allModels | ✅ | ✅ | ☐ |
| 224 | P2 | `MigrationRecord` | `migration_records` | `internal/model/upgrade.go` | — | allModels | ✅ | ✅ | ☐ |
| 225 | P2 | `ModuleOptionRow` | `module_option_rows` | `internal/model/module_bindings.go` | — | **未登记** | ❌ | — | ☐ |
| 226 | P2 | `ModuleOrder` | `module_orders` | `internal/model/tenant_module.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 227 | P2 | `ModuleSyncLog` | `module_sync_logs` | `internal/model/tenant_module.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 228 | P2 | `MsgHourlySummary` | `msg_hourly_summary` | `internal/model/msg_hourly_summary.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 229 | P2 | `Notification` | `notifications` | `internal/model/notification.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 230 | P2 | `ObjectionTemplate` | `objection_templates` | `internal/model/ai_sales_champion.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 231 | P2 | `ObsConfig` | `obs_config` | `internal/model/obs_config.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 232 | P2 | `OperationLog` | `operation_logs` | `internal/model/operation_log.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 233 | P2 | `Opportunity` | `opportunities` | `internal/model/opportunity.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 234 | P2 | `OptimizationSuggestion` | `optimization_suggestions` | `internal/model/feedback_learning.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 235 | P2 | `Order` | `order` | `internal/model/order.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 236 | P2 | `OrderDraft` | `order_drafts` | `internal/model/order_draft.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 237 | P2 | `PasswordHistory` | `password_history` | `internal/model/password_history.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 238 | P2 | `PasswordResetToken` | `password_reset_tokens` | `internal/model/password_reset_token.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 239 | P2 | `Payment` | `payments` | `internal/model/payment.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 240 | P2 | `PerformanceTestResult` | `performance_test_results` | `internal/ops/model/performance_test_result.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 241 | P2 | `PlatformAccountConfig` | `platform_account_configs` | `internal/model/platform_account_config.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 242 | P2 | `PlatformAccount` | `platform_accounts` | `internal/model/unified_message.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 243 | P2 | `PlatformConfigKV` | `platform_config_kv` | `internal/model/platform_config_kv.go` | — | allModels | ✅ | — | ☐ |
| 244 | P2 | `PlatformLLMUsageRecord` | `platform_llm_usage_records` | `internal/model/tenant_module.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 245 | P2 | `PlatformUser` | `platform_users` | `internal/model/unified_message.go` | — | **未登记** | ❌ | — | ☐ |
| 246 | P2 | `PromptABTest` | `prompt_ab_tests` | `internal/model/feedback_loop.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 247 | P2 | `PromptCandidate` | `prompt_candidates` | `internal/model/feedback_loop.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 248 | P2 | `PromptTemplate` | `prompt_templates` | `internal/content/model/ai_content.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 249 | P2 | `QQAccount` | `qq_accounts` | `internal/model/feishu.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 250 | P2 | `QueueStatus` | `queue_statuses` | `internal/model/whatsapp_queue.go` | — | **未登记** | ❌ | — | ☐ |
| 251 | P2 | `QueuedMessage` | `queued_messages` | `internal/model/whatsapp_queue.go` | — | **未登记** | ❌ | — | ☐ |
| 252 | P2 | `QuickReply` | `quick_replies` | `internal/model/quick_reply.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 253 | P2 | `QuickReplyFolder` | `quick_reply_folders` | `internal/model/quick_reply_folder.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 254 | P2 | `QuoteLineItem` | `quote_line_items` | `internal/model/quote.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 255 | P2 | `Quote` | `quotes` | `internal/model/quote.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 256 | P2 | `RagEvalQuestion` | `rag_eval_questions` | `internal/model/web_vital.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 257 | P2 | `RagEvalRun` | `rag_eval_runs` | `internal/model/web_vital.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 258 | P2 | `RagMessage` | `rag_messages` | `internal/model/rag_session.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 259 | P2 | `RagMetricsDaily` | `rag_metrics_daily` | `internal/model/rag_metrics.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 260 | P2 | `RagProduct` | `rag_products` | `internal/model/rag_product.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 261 | P2 | `RagQueryLog` | `rag_query_logs` | `internal/model/rag_metrics.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 262 | P2 | `RagRecallMonitorSnapshot` | `rag_recall_monitor_snapshots` | `internal/model/rag_recall_monitor_snapshot.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 263 | P2 | `RagSession` | `rag_sessions` | `internal/model/rag_session.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 264 | P2 | `RateValue` | `rate_values` | `internal/ops/model/ltc_rates.go` | — | **未登记** | ❌ | — | ☐ |
| 265 | P2 | `ReachComplianceLog` | `reach_compliance_log` | `internal/model/reach_compliance_log.go` | ✅ | ExtraModels | ✅ | ✅ | ☐ |
| 266 | P2 | `DelayedOutboundReply` | `reach_delayed_outbound` | `internal/model/reach_delayed_outbound.go` | ✅ | ExtraModels | ✅ | ✅ | ☐ |
| 267 | P2 | `ReachJob` | `reach_jobs` | `internal/model/ai_sales_champion.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 268 | P2 | `ReachPipeline` | `reach_pipelines` | `internal/model/ai_sales_champion.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 269 | P2 | `RecoveryQueue` | `recovery_queue` | `internal/model/customer_rfm.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 270 | P2 | `ReportDimension` | `report_dimensions` | `internal/ops/model/custom_report.go` | — | **未登记** | ❌ | — | ☐ |
| 271 | P2 | `ReportMetric` | `report_metrics` | `internal/ops/model/custom_report.go` | — | **未登记** | ❌ | — | ☐ |
| 272 | P2 | `ReportSubscription` | `report_subscriptions` | `internal/model/customer_segment.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 273 | P2 | `RerankProviderConfig` | `rerank_provider_configs` | `internal/model/rag_product.go` | — | **未登记** | ❌ | — | ☐ |
| 274 | P2 | `RestoreRecord` | `restore_records` | `internal/model/backup.go` | — | allModels | ✅ | ✅ | ☐ |
| 275 | P2 | `RFMRule` | `rfm_rules` | `internal/model/rfm_rule.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 276 | P2 | `RoleMenuPermission` | `role_menu_permissions` | `internal/model/menu.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 277 | P2 | `RulePendingExecution` | `rule_pending_executions` | `internal/model/customer_segment.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 278 | P2 | `SalesChampionProfileSnapshot` | `sales_champion_profile_snapshots` | `internal/model/feedback_learning.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 279 | P2 | `SalesEvent` | `sales_events` | `internal/model/sales_event.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 280 | P2 | `SalesIntentScore` | `sales_intent_scores` | `internal/model/ai_sales_champion.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 281 | P2 | `SalesPersona` | `sales_personas` | `internal/model/ai_sales_champion.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 282 | P2 | `SavedView` | `saved_views` | `internal/model/customer_segment.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 283 | P2 | `ScriptCategory` | `script_categories` | `internal/content/model/script_template.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 284 | P2 | `ScriptExposureLog` | `script_exposure_logs` | `internal/model/ai_sales_champion.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 285 | P2 | `ScriptLibrary` | `script_library` | `internal/model/ai_sales_champion.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 286 | P2 | `ScriptRecommend` | `script_recommendations` | `internal/content/model/script_template.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 287 | P2 | `ScriptTemplate` | `script_templates` | `internal/content/model/script_template.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 288 | P2 | `ScriptVersion` | `script_versions` | `internal/model/ai_sales_champion.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 289 | P2 | `SecurityAlert` | `security_alerts` | `internal/model/security_alert.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 290 | P2 | `SecurityAuditItem` | `security_audit_items` | `internal/model/security_audit.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 291 | P2 | `SecurityAudit` | `security_audits` | `internal/model/security_audit.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 292 | P2 | `SendOptions` | `send_options` | `internal/model/unified_message.go` | — | **未登记** | ❌ | — | ☐ |
| 293 | P2 | `SessionAISummary` | `session_ai_summaries` | `internal/model/customer_segment.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 294 | P2 | `SessionMessage` | `session_messages` | `internal/model/session_message.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 295 | P2 | `SessionTag` | `session_tags` | `internal/model/session_tag.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 296 | P2 | `ShortLinkAccess` | `short_link_accesses` | `internal/model/short_link_access.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 297 | P2 | `ShortLink` | `short_links` | `internal/model/short_link.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 298 | P2 | `SLAPolicy` | `sla_policies` | `internal/model/sla.go` | ✅ | **未登记** | ❌ | — | ☐ |
| 299 | P2 | `SLAViolation` | `sla_violations` | `internal/model/sla.go` | ✅ | **未登记** | ❌ | — | ☐ |
| 300 | P2 | `Smlist` | `smlist` | `internal/model/sm_list.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 301 | P2 | `SmsAliyunConfig` | `sms_aliyun_configs` | `internal/model/sms.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 302 | P2 | `SmsConfig` | `sms_configs` | `internal/model/sms.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 303 | P2 | `SmsDeliveryStatus` | `sms_delivery_statuses` | `internal/model/sms_tracking.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 304 | P2 | `SmsDraft` | `sms_drafts` | `internal/model/sms.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 305 | P2 | `SmsHuaweiConfig` | `sms_huawei_configs` | `internal/model/sms.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 306 | P2 | `SmsJobDetail` | `sms_job_details` | `internal/model/sms.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 307 | P2 | `SmsJob` | `sms_jobs` | `internal/model/sms.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 308 | P2 | `SmsNumberPortabilityRecord` | `sms_number_portability_logs` | `internal/model/sms_number_portability.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 309 | P2 | `SmsRecord` | `sms_records` | `internal/model/sms.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 310 | P2 | `SmsTencentConfig` | `sms_tencent_configs` | `internal/model/sms.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 311 | P2 | `SmsUnsubscribe` | `sms_unsubscribes` | `internal/model/sms_unsubscribe.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 312 | P2 | `SOPAgent` | `sop_agents` | `internal/model/ai_sales_champion.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 313 | P2 | `SOPExecEvent` | `sop_exec_events` | `internal/model/sop_executor.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 314 | P2 | `SOPExecution` | `sop_executions` | `internal/model/ai_sales_champion.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 315 | P2 | `SOPNodeTransition` | `sop_node_transitions` | `internal/model/feedback_learning.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 316 | P2 | `SOPOutbox` | `sop_outbox` | `internal/model/sop_executor.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 317 | P2 | `SOPStateMemory` | `sop_state_memories` | `internal/model/memory.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 318 | P2 | `SOPTemplate` | `sop_templates` | `internal/model/sop_template.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 319 | P2 | `SOPTimer` | `sop_timers` | `internal/model/sop_executor.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 320 | P2 | `SSOIdentity` | `sso_identities` | `internal/model/sso_identity.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 321 | P2 | `SupportMetrics` | `support_metrics` | `internal/model/support_metrics.go` | — | **未登记** | ❌ | — | ☐ |
| 322 | P2 | `SyncLog` | `sync_logs` | `internal/model/integration.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 323 | P2 | `SystemMetrics` | `system_metrics` | `internal/model/stats.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 324 | P2 | `SystemRole` | `system_roles` | `internal/model/role.go` | — | **未登记** | ❌ | — | ☐ |
| 325 | P2 | `SystemUser` | `system_users` | `internal/model/system_user.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 326 | P2 | `TelegramAccount` | `telegram_accounts` | `internal/model/feishu.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 327 | P2 | `TelegramGroupGate` | `telegram_group_gates` | `internal/model/telegram_gate.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 328 | P2 | `TelegramGroupMember` | `telegram_group_members` | `internal/model/telegram_gate.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 329 | P2 | `TenantConfigParamAuditLog` | `tenant_config_param_audit_logs` | `internal/model/config_param_tenant.go` | ✅ | allModels | ✅ | — | ☐ |
| 330 | P2 | `TenantConfigParam` | `tenant_config_params` | `internal/model/config_param_tenant.go` | ✅ | allModels | ✅ | — | ☐ |
| 331 | P2 | `TenantModule` | `tenant_modules` | `internal/model/tenant_module.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 332 | P2 | `Tenant` | `tenants` | `internal/model/tenant.go` | — | allModels | ✅ | ✅ | ☐ |
| 333 | P2 | `ThresholdPolicy` | `threshold_policies` | `internal/model/confidence.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 334 | P2 | `TikTokCardActivity` | `tiktok_card_activities` | `internal/model/tiktok_card.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 335 | P2 | `TikTokCard` | `tiktok_cards` | `internal/model/tiktok_card.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 336 | P2 | `ToolCallAudit` | `tool_call_audits` | `internal/model/tool_audit.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 337 | P2 | `TraceEvalLog` | `trace_eval_log` | `internal/model/trace_eval.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 338 | P2 | `UnifiedMessage` | `unified_messages` | `internal/model/unified_message.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 339 | P2 | `UnifiedReply` | `unified_replies` | `internal/model/unified_message.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 340 | P2 | `UpgradeTask` | `upgrade_tasks` | `internal/model/upgrade.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 341 | P2 | `User` | `user` | `internal/model/user.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 342 | P2 | `UserBlacklist` | `user_blacklist` | `internal/model/user_blacklist.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 343 | P2 | `UserMFA` | `user_mfa` | `internal/model/user_mfa.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 344 | P2 | `UserRFM` | `user_rfms` | `internal/model/rfm_rule.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 345 | P2 | `UserTag` | `user_tags` | `internal/model/user.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 346 | P2 | `VisitLog` | `visit_logs` | `internal/model/stats.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 347 | P2 | `WebVitalRecord` | `web_vital_records` | `internal/model/web_vital.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 348 | P2 | `WebhookEvent` | `webhook_events` | `internal/model/integration.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 349 | P2 | `WebhookSubscription` | `webhook_subscriptions` | `internal/model/customer_segment.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 350 | P2 | `WechatMessage` | `wechat_messages` | `internal/model/wechat_message.go` | ✅ | **未登记** | ❌ | — | ☐ |
| 351 | P2 | `WeComAccountHealth` | `wecom_account_health` | `internal/model/ai_sales_champion.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 352 | P2 | `WeComAccount` | `wecom_accounts` | `internal/model/wecom.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 353 | P2 | `WeComCustomer` | `wecom_customers` | `internal/model/wecom.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 354 | P2 | `WeComGroupMember` | `wecom_group_members` | `internal/model/wecom.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 355 | P2 | `WeComGroup` | `wecom_groups` | `internal/model/wecom.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 356 | P2 | `WeComMessage` | `wecom_messages` | `internal/model/wecom.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 357 | P2 | `WeComTag` | `wecom_tags` | `internal/model/wecom.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 358 | P2 | `WhatsappAccount` | `whatsapp_accounts` | `internal/model/whatsapp_account.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 359 | P2 | `WhatsAppCloudAccount` | `whatsapp_cloud_accounts` | `internal/model/feishu.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 360 | P2 | `WhatsappDraft` | `whatsapp_drafts` | `internal/model/whatsapp_draft.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 361 | P2 | `WhatsappGroupMessage` | `whatsapp_group_messages` | `internal/model/whatsapp_group_message.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 362 | P2 | `WhatsappJobDetail` | `whatsapp_job_details` | `internal/model/whatsapp_job.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 363 | P2 | `WhatsappJob` | `whatsapp_jobs` | `internal/model/whatsapp_job.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 364 | P2 | `WhatsAppMessageQueue` | `whatsapp_message_queues` | `internal/model/whatsapp_message_queue.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 365 | P2 | `WhatsappMessageTemplate` | `whatsapp_message_templates` | `internal/model/whatsapp_message_template.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 366 | P2 | `WhatsAppQueueStatus` | `whatsapp_queue_statuses` | `internal/model/whatsapp_message_queue.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 367 | P2 | `WhatsappSession` | `whatsapp_sessions` | `internal/model/whatsapp_session.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 368 | P2 | `WorkflowExecution` | `workflow_executions` | `internal/model/workflow_orchestrator.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 369 | P2 | `WorkflowNodeExecution` | `workflow_node_executions` | `internal/model/workflow_orchestrator.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 370 | P2 | `WorkflowVersion` | `workflow_versions` | `internal/model/workflow_orchestrator.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 371 | P2 | `XianyuCardActivity` | `xianyu_card_activities` | `internal/model/xianyu_card_activity.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 372 | P2 | `XianyuCard` | `xianyu_cards` | `internal/model/xianyu_card.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 373 | P2 | `XiaohongshuCardActivity` | `xiaohongshu_card_activities` | `internal/model/xiaohongshu_card_activity.go` | ✅ | allModels | ✅ | ✅ | ☐ |
| 374 | P2 | `XiaohongshuCard` | `xiaohongshu_cards` | `internal/model/xiaohongshu_card.go` | ✅ | allModels | ✅ | ✅ | ☐ |

---

## 三、平台级（无 `tenant_id`）表——逐张核对「为什么可以不带」

> 唯一判据是表上有没有 `tenant_id` 列，所以「为什么这张表例外」必须逐张有理由，
> 不能靠「它看起来是平台表」。带 ❓ 的表是本轮重点。

| 表 | dev 库 | 归属 | 理由 | 结论 |
|----|-------|------|------|------|
| `backups` | ✅ | 平台 | 待填 | ☐ |
| `config_param_audit_logs` | ✅ | 平台 | 待填 | ☐ |
| `config_param_groups` | ✅ | 平台 | 待填 | ☐ |
| `config_params` | ✅ | 平台 | 待填 | ☐ |
| `cron_job_leases` | ✅ | 平台 | 待填 | ☐ |
| `dictionaries` | ✅ | 平台 | 待填 | ☐ |
| `dictionary_transitions` | ✅ | 平台 | 待填 | ☐ |
| `feature_flag_audit_logs` | ✅ | 平台 | 待填 | ☐ |
| `feature_flag_eval_logs` | ✅ | 平台 | 待填 | ☐ |
| `feature_flags` | ✅ | 平台 | 待填 | ☐ |
| `migration_checkpoints` | ✅ | 平台 | 待填 | ☐ |
| `migration_records` | ✅ | 平台 | 待填 | ☐ |
| `restore_records` | ✅ | 平台 | 待填 | ☐ |
| `system_config` | ✅ | 平台 | 待填 | ☐ |
| `system_config_kv` | ✅ | 平台 | 待填 | ☐ |
| `tenant_subscription_mirror` | ✅ | 平台 | 待填 | ☐ |
| `tenants` | ✅ | 平台 | 待填 | ☐ |

---

## 四、审计重点名单（自动从上面两张表派生）

### 4-A 有 model 但**建表登记缺失**（fresh 建表=❌ 且 登记=未登记）

> 这类实体在全新部署上**表根本不存在**，写入路径若吞掉 error 就是「功能静默失效」。

| # | 实体 | 表 | 模型文件 | dev 库有表 |
|---|------|-----|----------|-----------|
| 1 | `LiveCodeClickLog` | `livecode_click_log` | `internal/model/live_code_click_log.go` | ✅ |
| 2 | `QRCodeClickLog` | `qr_code_click_log` | `internal/model/live_code_click_log.go` | ✅ |
| 3 | `SmsJobMetric` | `sms_job_metrics` | `internal/model/sms_tracking.go` | ✅ |
| 4 | `WechatAccount` | `wechat_accounts` | `internal/model/wechat_account.go` | ✅ |

### 4-B 实体在 dev 库存在、但 GORM 模型算不出这张表（历史残留 / 迁移建表）

| 表 | dev 库 | 是否有 model | 处置 |
|----|-------|-------------|------|
| `ab_exposures` | ✅ | ❌ 无 | ☐ |
| `agent_checkpoints` | ✅ | ❌ 无 | ☐ |
| `bridge_metrics` | ✅ | ❌ 无 | ☐ |
| `email_accounts` | ✅ | ❌ 无 | ☐ |
| `email_lists` | ✅ | ❌ 无 | ☐ |
| `email_smtps` | ✅ | ❌ 无 | ☐ |
| `embedding_cache` | ✅ | ❌ 无 | ☐ |
| `fts_test` | ✅ | ❌ 无 | ☐ |
| `intent_logs` | ✅ | ❌ 无 | ☐ |
| `kb_document_chunks` | ✅ | ❌ 无 | ☐ |
| `knowledge_embeddings` | ✅ | ❌ 无 | ☐ |
| `llm_routing_audit` | ✅ | ❌ 无 | ☐ |
| `llm_usage_records` | ✅ | ❌ 无 | ☐ |
| `merchants` | ✅ | ❌ 无 | ☐ |
| `obs_configs` | ✅ | ❌ 无 | ☐ |
| `provider_health` | ✅ | ❌ 无 | ☐ |
| `query_rewrite_cache` | ✅ | ❌ 无 | ☐ |
| `rag_answer_cache` | ✅ | ❌ 无 | ☐ |
| `script_recommends` | ✅ | ❌ 无 | ☐ |
| `system_kv_config` | ✅ | ❌ 无 | ☐ |
| `trace_events` | ✅ | ❌ 无 | ☐ |

### 4-C **租户属性漂移**：dev 库与 fresh 建表对同一张表判断不同

> 判据是「表上有无 `tenant_id` 列」。同一张表两边不一致 = 存量库还没被 R5 租户化，
> 或者反过来 fresh 建表把它当成了租户表而存量库没有 —— 两种都要查清。

| 表 | fresh 建表 | dev 库 | 差异 | 结论 |
|----|-----------|-------|------|------|
| `feature_flag_audit_logs` | TENANT | PLATFORM | ⚠️ 归属不一致 | ☐ |
| `feature_flag_eval_logs` | TENANT | PLATFORM | ⚠️ 归属不一致 | ☐ |
| `feature_flags` | TENANT | PLATFORM | ⚠️ 归属不一致 | ☐ |
| `system_config` | TENANT | PLATFORM | ⚠️ 归属不一致 | ☐ |
| `system_config_kv` | TENANT | PLATFORM | ⚠️ 归属不一致 | ☐ |
| `tenant_subscription_mirror` | TENANT | PLATFORM | ⚠️ 归属不一致 | ☐ |

### 4-D dev 库有、但 fresh 建表**不建**的表（存量残留 / 未登记 / 迁移建表）

| 表 | dev 库 tenant 属性 | fresh | 是否有 model | 结论 |
|----|------------------|-------|-------------|------|
| `agent_checkpoints` | TENANT | ❌ 不建 | ❌ | ☐ |
| `bridge_metrics` | TENANT | ❌ 不建 | ❌ | ☐ |
| `email_lists` | TENANT | ❌ 不建 | ❌ | ☐ |
| `email_smtps` | TENANT | ❌ 不建 | ❌ | ☐ |
| `embedding_cache` | TENANT | ❌ 不建 | ❌ | ☐ |
| `fts_test` | TENANT | ❌ 不建 | ❌ | ☐ |
| `intent_logs` | TENANT | ❌ 不建 | ❌ | ☐ |
| `knowledge_embeddings` | TENANT | ❌ 不建 | ❌ | ☐ |
| `livecode_click_log` | TENANT | ❌ 不建 | ✅ | ☐ |
| `llm_routing_audit` | TENANT | ❌ 不建 | ❌ | ☐ |
| `llm_usage_records` | TENANT | ❌ 不建 | ❌ | ☐ |
| `merchants` | TENANT | ❌ 不建 | ❌ | ☐ |
| `obs_configs` | TENANT | ❌ 不建 | ❌ | ☐ |
| `provider_health` | TENANT | ❌ 不建 | ❌ | ☐ |
| `qr_code_click_log` | TENANT | ❌ 不建 | ✅ | ☐ |
| `query_rewrite_cache` | TENANT | ❌ 不建 | ❌ | ☐ |
| `script_recommends` | TENANT | ❌ 不建 | ❌ | ☐ |
| `sms_job_metrics` | TENANT | ❌ 不建 | ✅ | ☐ |
| `system_kv_config` | TENANT | ❌ 不建 | ❌ | ☐ |
| `wechat_accounts` | TENANT | ❌ 不建 | ✅ | ☐ |

---

## 五、审计进度

| 轮次 | 实体 | 4.1 五层 | 4.2 GORM | 4.3 多租户 | 4.4 僵尸 | 处置 |
|------|------|---------|---------|-----------|---------|------|
| 1 | AgentKBBinding | ✅ | ✅ | ❌ 缺建表登记 | ✅ | 已修：allModels() 补登记 |
| 2 | KnowledgeBase | ✅ | ✅ | ❌ 缺建表登记 | ✅ | 已修：allModels() 补登记（同时修好 RAG 答案缓存的读路径） |
| 3 | BrowserOutreachDedupe | ✅ | ✅ | ❌ 缺建表登记 | ✅ | 已修：allModels() 补登记 |
| 4 | BrowserOutreachReceipt | ✅ | ✅ | ❌ 缺建表登记 | ✅ | 已修：allModels() 补登记 |
| 5 | BrowserProfileHealth | ✅ | ✅ | ❌ 缺建表登记 | ✅ | 已修：allModels() 补登记 |
| 6 | CustomerRFM | ✅ | ✅ | ❌ 缺建表登记 | ✅ | 已修：allModels() 补登记 |
| 7 | EmailTrackingEvent | ✅ | ✅ | ❌ 缺建表登记 | ✅ | 已修：allModels() 补登记 |
| 8 | EmailJobMetric | ✅ | ✅ | ❌ 缺建表登记 | ✅ | 已修：allModels() 补登记 |
| 9 | Glossary | ✅ | ✅ | ❌ 无 tenant_id + 缓存键无租户 + FLUSHDB | ✅ | 已修：model 补 tenant_id、缓存键加租户、失效改逐键删（不再 FLUSHDB）、allModels() 补登记；新增 4 条回归用例 |

（每轮一行；处置写「无问题」或「已修：xxx」。整表清空且 §二 全部打勾 = 第一轮完成。）
