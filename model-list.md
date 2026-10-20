# user-server Model 实体清单（任务一 · 五层架构 / GORM / 多租户 / 僵尸代码 逐实体审查）

- 范围：`user-server/` 下**全部声明 `TableName()` 的 model 文件**（排除 `*_test.go`）
- 基线 commit：`029ccf7a`（master，与 `gitee-upstream/master` / `upstream/master` 均 0/0）
- 规模：**205 个 model 文件 / 346 张表声明**
- 生成方式：只做「梳理与清洗」的索引；**逐实体四关审查（步骤 3）一律基于编辑器单条推进，禁止脚本批量**

## 判据（每个实体按此四条过）

| # | 关 | 判据 | 违规处置 |
|---|---|---|---|
| 4.1 | 五层架构 | `Router → Handler → Service → Repository → Model`，模型必须在 `internal/model/` 或 `internal/<域>/model/`；Handler 不得直连 Repository；Service 不得直接操作 DB；Repository 不得含业务判断 | 就地补齐/下沉/上移，不新增抽象 |
| 4.2 | GORM 驱动 | 该实体全链路不得出现裸 SQL：`.Raw(` / `.Exec(` / 手拼 `WHERE`；必须走 GORM 链式或结构化条件（门禁：`user-server/scripts/check-gorm-only.sh`） | 改写为 GORM，跑门禁复验 |
| 4.3 | 多租户约束 | 判据是**表上有没有 `tenant_id` 列**（内省登记表唯一口径，见 `docs/architecture/SaaS_MULTI_TENANT.md` §三）。业务数据表必须有；平台/实例级配置表必须没有并在 §三.5 对账。`on` 模式访问租户表缺 ctx ⇒ fail-closed | 加列 + 迁移补列归默认租户；或论证归属并登记文档 |
| 4.4 | 僵尸代码 | 结构体/字段/方法/整条链路无任何引用：需交叉验证 router 注册、service/repo 调用、前端 `user-web/src/api/*` 调用点、迁移与统计任务；**判僵尸前必须给出引用面证据**，不得凭"看着没人用"删 | 删除并 `go build`/`go test` 复验 |

## 状态与预检标记图例

状态列：`[ ]` 待审 / `[x]` 已过四关并测试绿 / `[~]` 需人工决策（记入文末「待决策」）

预检标记（索引阶段自动比对得出，**是待核实线索、不是结论**）：

- `T` 该文件内无 `TenantID` 字段 ⇒ 4.3 需论证归属
- `U` 声明了 `TableName()` 但**未出现在 `allModels()`** ⇒ 表可能不由 AutoMigrate 建（手写迁移/漏登记/死表）
- `N` 表名来自动态表达式或常量（`~` 前缀）⇒ 需展开确认真实表名
- `L` model 不在 `internal/model/` 或 `internal/<域>/model/` 目录 ⇒ 4.1 疑似越层

统计：`T` 9 · `U` 12 · `N` 101 · `L` 7 · 无标记 88

## 清单

| # | 状态 | model 文件 | 结构体 | 表名 | tenant\_id | 预检 |
|---|---|---|---|---|---|---|
| 1 | [x] | `internal/model/trace_event.go`（原 `internal/aiagent/llm/trace_context.go`） | `TraceEvent` | `trace_events` | 1 | U、L → 已消除 |
| 2 | [x] | `internal/model/rag_answer_cache.go`（原 `internal/aiagent/rag/cache/answer_cache_model.go`） | `RAGAnswerCache` | `rag_answer_cache` | 1 | L → 已消除 |
| 3 | [x] | `internal/browser_automation/model/audit_digest.go` | `BrowserAuditDigest`, `BrowserAuditPruneRun` | `browser_audit_digests`, `browser_audit_prune_runs` | 2 | - → 已修 4.3 |
| 4 | [x] | `internal/browser_automation/model/command_log.go` | `BrowserCommandLog` | `browser_command_log` | 1 | - → 已修 4.3 |
| 5 | [ ] | `internal/browser_automation/model/cron.go` | `BrowserCronTrigger` | `browser_cron_triggers` | 1 | - |
| 6 | [ ] | `internal/browser_automation/model/llm_plan.go` | `BrowserLLMPlan` | `browser_llm_plans` | 1 | - |
| 7 | [ ] | `internal/browser_automation/model/outreach_dedupe.go` | `BrowserOutreachDedupe` | `browser_outreach_dedupe` | 1 | - |
| 8 | [ ] | `internal/browser_automation/model/outreach_receipt.go` | `BrowserOutreachReceipt` | `browser_outreach_receipts` | 1 | - |
| 9 | [ ] | `internal/browser_automation/model/profile_health.go` | `BrowserProfileHealth` | `browser_profile_health` | 1 | - |
| 10 | [ ] | `internal/browser_automation/model/session.go` | `BrowserSession` | `browser_sessions` | 1 | - |
| 11 | [ ] | `internal/browser_automation/model/step.go` | `BrowserStep` | `browser_steps` | 1 | - |
| 12 | [ ] | `internal/browser_automation/model/task.go` | `BrowserTask` | `browser_tasks` | 1 | - |
| 13 | [ ] | `internal/browser_automation/model/write_claim.go` | `BrowserWriteClaim` | `browser_write_claims` | 1 | - |
| 14 | [ ] | `internal/content/model/ai_content.go` | `AIGenerationRecord`, `PromptTemplate` | `~>`, `~>` | 2 | N |
| 15 | [ ] | `internal/content/model/batch_operation_history.go` | `BatchOperationHistory` | `~>` | 1 | N |
| 16 | [ ] | `internal/content/model/market_template.go` | `MarketTemplate`, `MarketTemplateDownload` | `~>`, `~>` | 2 | N |
| 17 | [ ] | `internal/content/model/marketing_flow.go` | `MarketingFlow`, `FlowExecution` | `~>`, `~>` | 2 | N |
| 18 | [ ] | `internal/content/model/material.go` | `Material`, `MaterialCategory` | `~>`, `~>` | 2 | N |
| 19 | [ ] | `internal/content/model/script_template.go` | `ScriptTemplate`, `ScriptCategory`, `ScriptRecommend` | `~>`, `~>`, `~>` | 3 | N |
| 20 | [ ] | `internal/geo/model/alert.go` | `GeoAlert` | `geo_alerts` | 1 | - |
| 21 | [ ] | `internal/geo/model/competitor.go` | `GeoCompetitor` | `geo_competitors` | 1 | - |
| 22 | [ ] | `internal/geo/model/config.go` | `GeoConfig`, `GeoPlatformAccount`, `GeoPublishRecord` | `~>`, `~>`, `~>` | 3 | N |
| 23 | [ ] | `internal/geo/model/content.go` | `GeoArticle`, `GeoOptimization` | `~>`, `~>` | 2 | N |
| 24 | [ ] | `internal/geo/model/crawler_visit.go` | `GeoCrawlerVisit` | `geo_crawler_visits` | 1 | - |
| 25 | [ ] | `internal/geo/model/daily_stats.go` | `GeoDailyStat` | `geo_daily_stats` | 1 | - |
| 26 | [ ] | `internal/geo/model/dict.go` | `GeoDict` | `~>` | 1 | N |
| 27 | [ ] | `internal/geo/model/entity.go` | `GeoEntity`, `GeoEntityRelation` | `geo_entities`, `geo_entity_relations` | 2 | - |
| 28 | [ ] | `internal/geo/model/index_tracking.go` | `GeoIndexTracking` | `geo_index_trackings` | 1 | - |
| 29 | [ ] | `internal/geo/model/job_run.go` | `GeoJobRun` | `geo_job_runs` | 1 | - |
| 30 | [ ] | `internal/geo/model/keyword.go` | `GeoKeyword`, `GeoKeywordGroup` | `~>`, `~>` | 2 | N |
| 31 | [ ] | `internal/geo/model/knowledge.go` | `GeoKnowledgeDocument` | `~>` | 1 | N |
| 32 | [ ] | `internal/geo/model/probe.go` | `GeoProbeRun` | `geo_probe_runs` | 1 | - |
| 33 | [ ] | `internal/geo/model/push_record.go` | `GeoPushRecord` | `geo_push_records` | 1 | - |
| 34 | [ ] | `internal/geo/model/pusher_config.go` | `GeoPusherConfig` | `geo_pusher_configs` | 1 | - |
| 35 | [ ] | `internal/geo/model/query_chain.go` | `GeoQueryChain`, `GeoContentTask` | `geo_query_chains`, `geo_content_tasks` | 2 | - |
| 36 | [ ] | `internal/geo/model/schema_template.go` | `GeoSchemaTemplate` | `geo_schema_templates` | 1 | - |
| 37 | [ ] | `internal/geo/model/seed_term.go` | `GeoSeedTerm` | `~>` | 1 | N |
| 38 | [ ] | `internal/geo/model/site.go` | `GeoSite` | `geo_sites` | 1 | - |
| 39 | [ ] | `internal/geo/model/source_catalog.go` | `GeoSourceCatalog` | `geo_source_catalogs` | 1 | - |
| 40 | [ ] | `internal/geo/model/verification.go` | `GeoVerifyResult`, `GeoAPICall` | `~>`, `~>` | 2 | N |
| 41 | [ ] | `internal/geo/model/workflow.go` | `GeoWorkflow`, `GeoWorkflowExecution`, `GeoWorkflowTemplate` | `~>`, `~>`, `~>` | 3 | N |
| 42 | [ ] | `internal/model/account.go` | `Account` | `~>` | 1 | N |
| 43 | [ ] | `internal/model/aftersale.go` | `AfterSale` | `after_sales` | 1 | - |
| 44 | [ ] | `internal/model/agent_kb_binding.go` | `AgentKBBinding` | `agent_kb_bindings` | 1 | - |
| 45 | [ ] | `internal/model/agent_status.go` | `AgentStatus` | `~>` | 1 | N |
| 46 | [ ] | `internal/model/ai_agent.go` | `AIAgent`, `ChannelAgentBinding`, `CustomerServiceAgent` | `~>`, `~>`, `~>` | 3 | N |
| 47 | [ ] | `internal/model/ai_sales_champion.go` | `MessageHub`, `IntentRecord`, `DialogueMemory`, `SOPAgent`, `SOPExecution`, `ReachPipeline`, `ReachJob`, `WeComAccountHealth`, `SalesIntentScore`, `ScriptLibrary`, `ScriptVersion`, `ScriptExposureLog`, `ObjectionTemplate`, `ConversionFunnel`, `SalesPersona`, `AISalesLog`, `InboxConversation`, `InboxAssignment` | `message_hub`, `intent_records`, `dialogue_memories`, `sop_agents`, `sop_executions`, `reach_pipelines`, `reach_jobs`, `wecom_account_health`, `sales_intent_scores`, `script_library`, `script_versions`, `script_exposure_logs`, `objection_templates`, `conversion_funnels`, `sales_personas`, `ai_sales_logs`, `inbox_conversations`, `inbox_assignments` | 18 | - |
| 48 | [ ] | `internal/model/ai_suggestion.go` | `AISuggestion` | `~>` | 1 | N |
| 49 | [ ] | `internal/model/ai_tool_config.go` | `AIToolConfig`, `AIToolAccountBinding` | `~>`, `~>` | 2 | N |
| 50 | [ ] | `internal/model/alert_rule.go` | `AlertRule`, `AlertHistory` | `~>`, `~>` | 2 | N |
| 51 | [ ] | `internal/model/approval_request.go` | `ApprovalRequest` | `approval_requests` | 1 | - |
| 52 | [ ] | `internal/model/asset_bundle.go` | `AssetBundle`, `AssetBundleVersionLog` | `asset_bundles`, `asset_bundle_version_logs` | 2 | - |
| 53 | [ ] | `internal/model/backup.go` | `Backup`, `RestoreRecord` | `~>`, `~>` | 0 | T、N |
| 54 | [ ] | `internal/model/bad_case.go` | `BadCase` | `bad_cases` | 1 | - |
| 55 | [ ] | `internal/model/bandit_reflux_log.go` | `BanditRefluxLog` | `bandit_reflux_log` | 1 | - |
| 56 | [ ] | `internal/model/bill.go` | `Bill` | `bills` | 1 | - |
| 57 | [ ] | `internal/model/bridge_account.go` | `BridgeAccount` | `bridge_accounts` | 1 | - |
| 58 | [ ] | `internal/model/chat_channel.go` | `ChatChannel` | `~>` | 1 | N |
| 59 | [ ] | `internal/model/churn_score.go` | `ChurnScore` | `churn_scores` | 1 | - |
| 60 | [ ] | `internal/model/clue.go` | `Clue` | `~>` | 1 | N |
| 61 | [ ] | `internal/model/clue_score.go` | `ClueScore`, `ClueEngagementEvent` | `clue_scores`, `clue_engagement_events` | 2 | - |
| 62 | [ ] | `internal/model/community.go` | `CommunityGroup`, `CommunityMember`, `CommunityMessage` | `~>`, `~>`, `~>` | 3 | N |
| 63 | [ ] | `internal/model/confidence.go` | `ConfidenceSignal`, `ConfidenceCalibration`, `HandoffDecisionRecord`, `ThresholdPolicy`, `ABTest`, `ABTestMetric` | `confidence_signals`, `confidence_calibrations`, `handoff_decisions`, `threshold_policies`, `ab_tests`, `ab_test_metrics` | 6 | - |
| 64 | [ ] | `internal/model/config_param.go` | `ConfigParam`, `ConfigParamAuditLog` | `config_params`, `config_param_audit_logs` | 0 | T |
| 65 | [ ] | `internal/model/config_param_group.go` | `ConfigParamGroup` | `config_param_groups` | 0 | T |
| 66 | [ ] | `internal/model/config_param_tenant.go` | `TenantConfigParam`, `TenantConfigParamAuditLog` | `tenant_config_params`, `tenant_config_param_audit_logs` | 2 | - |
| 67 | [ ] | `internal/model/cron_job_lease.go` | `CronJobLease` | `cron_job_leases` | 0 | T |
| 68 | [ ] | `internal/model/csat.go` | `CSATSurvey` | `csat_surveys` | 1 | - |
| 69 | [ ] | `internal/model/customer.go` | `Customer` | `~>` | 1 | N |
| 70 | [ ] | `internal/model/customer_channel.go` | `CustomerChannel` | `~>` | 1 | N |
| 71 | [ ] | `internal/model/customer_do_not_contact.go` | `CustomerDoNotContact` | `~>` | 1 | N |
| 72 | [ ] | `internal/model/customer_event.go` | `CustomerEvent` | `~>` | 1 | N |
| 73 | [ ] | `internal/model/customer_long_term_memory.go` | `CustomerLongTermMemory` | `customer_long_term_memory` | 1 | - |
| 74 | [ ] | `internal/model/customer_rfm.go` | `CustomerRFM`, `RecoveryQueue` | `customer_rfm`, `recovery_queue` | 2 | - |
| 75 | [ ] | `internal/model/customer_segment.go` | `CustomerSegment`, `Macro`, `SessionAISummary`, `WebhookSubscription`, `SavedView`, `ReportSubscription`, `AutomationRule`, `RulePendingExecution` | `customer_segments`, `macros`, `session_ai_summaries`, `webhook_subscriptions`, `saved_views`, `report_subscriptions`, `automation_rules`, `rule_pending_executions` | 8 | - |
| 76 | [ ] | `internal/model/customer_session.go` | `CustomerSession` | `~>` | 1 | N |
| 77 | [ ] | `internal/model/customer_tag.go` | `CustomerTag` | `~>` | 1 | N |
| 78 | [ ] | `internal/model/customer_tag_assignment.go` | `CustomerTagAssignment` | `~>` | 1 | N |
| 79 | [ ] | `internal/model/dialogue_eval_run.go` | `DialogueEvalRun` | `dialogue_eval_runs` | 1 | - |
| 80 | [ ] | `internal/model/dictionary.go` | `Dictionary`, `DictionaryTransition` | `dictionaries`, `dictionary_transitions` | 0 | T |
| 81 | [ ] | `internal/model/dingtalk_app_account.go` | `DingTalkAppAccount` | `dingtalk_app_accounts` | 1 | - |
| 82 | [ ] | `internal/model/domain_pool.go` | `DomainPool`, `DomainHealthLog`, `DomainBlacklist` | `~>`, `~>`, `~>` | 3 | N |
| 83 | [ ] | `internal/model/douyin_card.go` | `DouyinCard` | `~>` | 1 | N |
| 84 | [ ] | `internal/model/douyin_card_activity.go` | `DouyinCardActivity` | `~>` | 1 | N |
| 85 | [ ] | `internal/model/email_jobs.go` | `EmailJobs` | `~>` | 1 | N |
| 86 | [ ] | `internal/model/email_list.go` | `EmailList` | `~>` | 1 | N |
| 87 | [ ] | `internal/model/email_smtp.go` | `EmailSmtp` | `~>` | 1 | N |
| 88 | [ ] | `internal/model/email_tracking.go` | `EmailTrackingEvent`, `EmailJobMetric` | `~>`, `~>` | 2 | N |
| 89 | [ ] | `internal/model/email_unsubscribe.go` | `EmailUnsubscribe` | `~>` | 1 | N |
| 90 | [ ] | `internal/model/faq_entry.go` | `FAQEntry` | `faq_entries` | 1 | - |
| 91 | [ ] | `internal/model/feature_flag.go` | `FeatureFlag`, `FeatureFlagAuditLog`, `FeatureFlagEvalLog` | `feature_flags`, `feature_flag_audit_logs`, `feature_flag_eval_logs` | 3 | - |
| 92 | [ ] | `internal/model/feedback_learning.go` | `SalesChampionProfileSnapshot`, `SOPNodeTransition`, `OptimizationSuggestion` | `sales_champion_profile_snapshots`, `sop_node_transitions`, `optimization_suggestions` | 3 | - |
| 93 | [ ] | `internal/model/feedback_loop.go` | `FeedbackEvent`, `FeedbackSignal`, `ChampionDialogue`, `PromptCandidate`, `BanditArm`, `PromptABTest` | `feedback_events`, `feedback_signals`, `champion_dialogues`, `prompt_candidates`, `bandit_arms`, `prompt_ab_tests` | 6 | - |
| 94 | [ ] | `internal/model/feedback_record.go` | `FeedbackRecordORM` | `feedback_records` | 1 | - |
| 95 | [ ] | `internal/model/feishu.go` | `FeishuAccount`, `FeishuCustomer`, `FeishuMessage`, `TelegramAccount`, `WhatsAppCloudAccount`, `QQAccount` | `feishu_accounts`, `feishu_customers`, `feishu_messages`, `telegram_accounts`, `whatsapp_cloud_accounts`, `qq_accounts` | 6 | - |
| 96 | [ ] | `internal/model/glossary.go` | `Glossary` | `glossaries` | 1 | - |
| 97 | [ ] | `internal/model/human_task.go` | `HumanTask` | `human_tasks` | 1 | - |
| 98 | [ ] | `internal/model/humanize_score.go` | `HumanizeScore`, `HumanizeDimensionRecord`, `ChampionBaseline`, `ChampionPhrase`, `ABTestStat` | `humanize_scores`, `humanize_dimensions`, `champion_baselines`, `champion_phrases`, `ab_test_stats` | 5 | - |
| 99 | [ ] | `internal/model/integration.go` | `IntegrationAccount`, `SyncLog`, `ExternalCustomer`, `ExternalOrder`, `ExternalProduct`, `WebhookEvent` | `~>`, `~>`, `~>`, `~>`, `~>`, `~>` | 6 | N |
| 100 | [ ] | `internal/model/integration_template.go` | `IntegrationTemplate` | `integration_templates` | 1 | - |
| 101 | [ ] | `internal/model/intent_example.go` | `IntentExample` | `intent_examples` | 1 | - |
| 102 | [ ] | `internal/model/intent_log.go` | `IntentLog` | `intent_records` | 1 | U |
| 103 | [ ] | `internal/model/kb_connector.go` | `ExternalKBConnector`, `ExternalKBSyncItem` | `external_kb_connectors`, `external_kb_sync_items` | 2 | - |
| 104 | [ ] | `internal/model/kb_document.go` | `KBDocument` | `~>` | 1 | N |
| 105 | [ ] | `internal/model/kb_merchant.go` | `KnowledgeAPIToken`, `KnowledgeFeedback`, `ExternalImportJob` | `~>`, `~>`, `~>` | 3 | N |
| 106 | [ ] | `internal/model/kb_release.go` | `KBChangeRequest`, `KBRelease`, `KBChangeAuditLog` | `kb_change_requests`, `kb_releases`, `kb_change_audit_logs` | 3 | - |
| 107 | [ ] | `internal/model/kb_workspace.go` | `KnowledgeDocument`, `KnowledgeChunk`, `KnowledgeImportLog`, `KnowledgeSearchLog`, `KnowledgeOpenAPISource`, `HelpCenterTestRecord` | `~>`, `~>`, `~>`, `~>`, `~>`, `help_center_test_records` | 6 | N |
| 108 | [ ] | `internal/model/knowledge_base.go` | `KnowledgeBase` | `knowledge_bases` | 1 | - |
| 109 | [ ] | `internal/model/kuaishou_card_activity.go` | `KuaishouCardActivity` | `~>` | 1 | N |
| 110 | [ ] | `internal/model/layer_decision_log.go` | `LayerDecisionLog` | `layer_decision_logs` | 1 | - |
| 111 | [ ] | `internal/model/lead_mining_config.go` | `LeadMiningConfig` | `lead_mining_configs` | 1 | - |
| 112 | [ ] | `internal/model/learning_insight.go` | `LearningInsight` | `learning_insights` | 1 | - |
| 113 | [ ] | `internal/model/live_code.go` | `LiveCode` | `~>` | 1 | N |
| 114 | [ ] | `internal/model/live_code_click_log.go` | `LiveCodeClickLog`, `QRCodeClickLog` | `~>`, `~>` | 2 | U、N |
| 115 | [ ] | `internal/model/live_code_qr.go` | `LiveCodeQR` | `~>` | 1 | N |
| 116 | [ ] | `internal/model/live_code_qr_stat.go` | `LiveCodeQRStat` | `~>` | 1 | N |
| 117 | [ ] | `internal/model/llm_provider.go` | `LLMProvider` | `llm_providers` | 1 | - |
| 118 | [ ] | `internal/model/llm_routing_log.go` | `LLMRoutingLog` | `llm_routing_logs` | 1 | - |
| 119 | [ ] | `internal/model/llm_routing_rule.go` | `LLMRoutingRule` | `llm_routing_rules` | 1 | - |
| 120 | [ ] | `internal/model/local_asset.go` | `LocalAsset`, `LocalAssetData`, `LocalAssetSyncLog` | `local_assets`, `local_asset_data`, `local_asset_sync_log` | 3 | - |
| 121 | [ ] | `internal/model/login_event.go` | `LoginEvent` | `~>` | 1 | N |
| 122 | [ ] | `internal/model/low_quality_sample.go` | `LowQualitySample` | `low_quality_samples` | 1 | - |
| 123 | [ ] | `internal/model/mcp_credential.go` | `MCPCredential` | `mcp_credentials` | 1 | - |
| 124 | [ ] | `internal/model/memory.go` | `MemoryItem`, `SOPStateMemory`, `BusinessMemory` | `memory_items`, `sop_state_memories`, `business_memories` | 3 | - |
| 125 | [ ] | `internal/model/menu.go` | `RoleMenuPermission` | `~>` | 1 | N |
| 126 | [ ] | `internal/model/message.go` | `Message` | `~>` | 1 | N |
| 127 | [ ] | `internal/model/message_trace.go` | `MessageTrace` | `message_trace` | 1 | - |
| 128 | [ ] | `internal/model/msg_hourly_summary.go` | `MsgHourlySummary`, `AggregationWatermark` | `msg_hourly_summary`, `aggregation_watermarks` | 2 | - |
| 129 | [ ] | `internal/model/notification.go` | `Notification` | `~>` | 1 | N |
| 130 | [ ] | `internal/model/obs_config.go` | `ObsConfig` | `~>` | 1 | N |
| 131 | [ ] | `internal/model/operation_log.go` | `OperationLog` | `~>` | 1 | N |
| 132 | [ ] | `internal/model/opportunity.go` | `Opportunity` | `opportunities` | 1 | - |
| 133 | [ ] | `internal/model/order.go` | `Order` | `~>` | 1 | N |
| 134 | [ ] | `internal/model/order_draft.go` | `OrderDraft` | `order_drafts` | 1 | - |
| 135 | [ ] | `internal/model/password_history.go` | `PasswordHistory` | `~>` | 1 | N |
| 136 | [ ] | `internal/model/password_reset_token.go` | `PasswordResetToken` | `~>` | 1 | N |
| 137 | [ ] | `internal/model/payment.go` | `Payment` | `payments` | 1 | - |
| 138 | [ ] | `internal/model/platform_account_config.go` | `PlatformAccountConfig` | `~>` | 1 | N |
| 139 | [ ] | `internal/model/platform_config_kv.go` | `PlatformConfigKV` | `~>` | 0 | T、N |
| 140 | [ ] | `internal/model/quick_reply.go` | `QuickReply` | `~>` | 1 | N |
| 141 | [ ] | `internal/model/quick_reply_folder.go` | `QuickReplyFolder` | `quick_reply_folders` | 1 | - |
| 142 | [ ] | `internal/model/quote.go` | `Quote`, `QuoteLineItem` | `quotes`, `quote_line_items` | 2 | - |
| 143 | [ ] | `internal/model/rag_metrics.go` | `RagQueryLog`, `RagMetricsDaily` | `rag_query_logs`, `rag_metrics_daily` | 2 | - |
| 144 | [ ] | `internal/model/rag_product.go` | `RagProduct` | `~>` | 2 | N |
| 145 | [ ] | `internal/model/rag_recall_monitor_snapshot.go` | `RagRecallMonitorSnapshot` | `~>` | 1 | N |
| 146 | [ ] | `internal/model/reach_compliance_log.go` | `ReachComplianceLog` | `reach_compliance_log` | 1 | U |
| 147 | [ ] | `internal/model/reach_delayed_outbound.go` | `DelayedOutboundReply` | `reach_delayed_outbound` | 1 | U |
| 148 | [ ] | `internal/model/rfm_rule.go` | `RFMRule`, `UserRFM` | `~>`, `~>` | 2 | N |
| 149 | [ ] | `internal/model/sales_event.go` | `SalesEvent` | `sales_events` | 1 | - |
| 150 | [ ] | `internal/model/security_alert.go` | `SecurityAlert` | `~>` | 1 | N |
| 151 | [ ] | `internal/model/security_audit.go` | `SecurityAudit`, `SecurityAuditItem` | `security_audits`, `security_audit_items` | 2 | - |
| 152 | [ ] | `internal/model/session_message.go` | `SessionMessage` | `~>` | 1 | N |
| 153 | [ ] | `internal/model/session_tag.go` | `SessionTag` | `~>` | 1 | N |
| 154 | [ ] | `internal/model/short_link.go` | `ShortLink` | `~>` | 1 | N |
| 155 | [ ] | `internal/model/short_link_access.go` | `ShortLinkAccess` | `~>` | 1 | N |
| 156 | [ ] | `internal/model/sla.go` | `SLAPolicy`, `SLAViolation` | `sla_policies`, `sla_violations` | 2 | U |
| 157 | [ ] | `internal/model/sm_list.go` | `Smlist` | `~>` | 1 | N |
| 158 | [ ] | `internal/model/sms_number_portability.go` | `SmsNumberPortabilityRecord` | `~>` | 1 | N |
| 159 | [ ] | `internal/model/sms_tracking.go` | `SmsDeliveryStatus`, `SmsJobMetric` | `~>`, `~>` | 2 | U、N |
| 160 | [ ] | `internal/model/sms_unsubscribe.go` | `SmsUnsubscribe` | `~>` | 1 | N |
| 161 | [ ] | `internal/model/sop_executor.go` | `SOPExecEvent`, `SOPTimer`, `SOPOutbox` | `sop_exec_events`, `sop_timers`, `sop_outbox` | 3 | - |
| 162 | [ ] | `internal/model/sop_template.go` | `SOPTemplate` | `sop_templates` | 1 | - |
| 163 | [ ] | `internal/model/sso_identity.go` | `SSOIdentity` | `~>` | 1 | N |
| 164 | [ ] | `internal/model/stats.go` | `APILog`, `VisitLog`, `DailyStats`, `SystemMetrics` | `~>`, `~>`, `~>`, `~>` | 4 | N |
| 165 | [ ] | `internal/model/system_config.go` | `SystemConfig` | `~>` | 1 | N |
| 166 | [ ] | `internal/model/system_config_kv.go` | `SystemConfigKV` | `~>` | 1 | N |
| 167 | [ ] | `internal/model/system_user.go` | `SystemUser` | `~>` | 1 | N |
| 168 | [ ] | `internal/model/telegram_gate.go` | `TelegramGroupGate`, `TelegramGroupMember` | `telegram_group_gates`, `telegram_group_members` | 2 | - |
| 169 | [ ] | `internal/model/tenant.go` | `Tenant` | `tenants` | 0 | T |
| 170 | [ ] | `internal/model/tenant_module.go` | `TenantModule`, `ModuleSyncLog`, `ModuleOrder`, `LLMWallet`, `LLMWalletTxn`, `PlatformLLMUsageRecord` | `tenant_modules`, `module_sync_logs`, `module_orders`, `llm_wallets`, `llm_wallet_txns`, `platform_llm_usage_records` | 6 | - |
| 171 | [ ] | `internal/model/tenant_subscription_mirror.go` | `TenantSubscriptionMirror` | `tenant_subscription_mirror` | 1 | - |
| 172 | [ ] | `internal/model/tiktok_card.go` | `TikTokCard`, `TikTokCardActivity` | `~>`, `~>` | 2 | N |
| 173 | [ ] | `internal/model/tool_audit.go` | `ToolCallAudit` | `tool_call_audits` | 1 | - |
| 174 | [ ] | `internal/model/trace_eval.go` | `TraceEvalLog` | `trace_eval_log` | 1 | - |
| 175 | [ ] | `internal/model/unified_message.go` | `UnifiedMessage`, `UnifiedReply`, `PlatformAccount` | `~>`, `~>`, `~>` | 3 | N |
| 176 | [ ] | `internal/model/upgrade.go` | `UpgradeTask`, `MigrationRecord`, `MigrationCheckpoint` | `~>`, `~>`, `~>` | 1 | N |
| 177 | [ ] | `internal/model/user.go` | `User`, `UserTag` | `~>`, `~>` | 2 | N |
| 178 | [ ] | `internal/model/user_blacklist.go` | `UserBlacklist` | `~>` | 1 | N |
| 179 | [ ] | `internal/model/user_mfa.go` | `UserMFA` | `~>` | 1 | N |
| 180 | [ ] | `internal/model/web_vital.go` | `WebVitalRecord`, `RagEvalQuestion`, `RagEvalRun` | `web_vital_records`, `rag_eval_questions`, `rag_eval_runs` | 3 | - |
| 181 | [ ] | `internal/model/wechat_account.go` | `WechatAccount` | `~>` | 1 | N |
| 182 | [ ] | `internal/model/wechat_message.go` | `WechatMessage` | `~>` | 1 | N |
| 183 | [ ] | `internal/model/wecom.go` | `WeComAccount`, `WeComCustomer`, `WeComGroup`, `WeComGroupMember`, `WeComMessage`, `WeComTag` | `~>`, `~>`, `~>`, `~>`, `~>`, `~>` | 6 | N |
| 184 | [ ] | `internal/model/whatsapp_account.go` | `WhatsappAccount` | `~>` | 1 | N |
| 185 | [ ] | `internal/model/whatsapp_draft.go` | `WhatsappDraft` | `~>` | 1 | N |
| 186 | [ ] | `internal/model/whatsapp_group_message.go` | `WhatsappGroupMessage` | `~>` | 1 | N |
| 187 | [ ] | `internal/model/whatsapp_job.go` | `WhatsappJob`, `WhatsappJobDetail` | `~>`, `~>` | 2 | N |
| 188 | [ ] | `internal/model/whatsapp_message_queue.go` | `WhatsAppMessageQueue`, `WhatsAppQueueStatus` | `~>`, `~>` | 2 | N |
| 189 | [ ] | `internal/model/whatsapp_message_template.go` | `WhatsappMessageTemplate` | `~>` | 1 | N |
| 190 | [ ] | `internal/model/whatsapp_session.go` | `WhatsappSession` | `~>` | 1 | N |
| 191 | [ ] | `internal/model/workflow_orchestrator.go` | `WorkflowVersion`, `WorkflowExecution`, `WorkflowNodeExecution` | `workflow_versions`, `workflow_executions`, `workflow_node_executions` | 3 | - |
| 192 | [ ] | `internal/model/xianyu_card.go` | `XianyuCard` | `~>` | 1 | N |
| 193 | [ ] | `internal/model/xianyu_card_activity.go` | `XianyuCardActivity` | `~>` | 1 | N |
| 194 | [ ] | `internal/model/xiaohongshu_card.go` | `XiaohongshuCard` | `~>` | 1 | N |
| 195 | [ ] | `internal/model/xiaohongshu_card_activity.go` | `XiaohongshuCardActivity` | `~>` | 1 | N |
| 196 | [ ] | `internal/ops/model/ab_experiment.go` | `ABExperiment`, `ABVariant`, `ABConversionEvent`, `ABExperimentResult` | `~>`, `~>`, `~>`, `~>` | 4 | N |
| 197 | [ ] | `internal/ops/model/churn_prediction.go` | `ChurnPrediction`, `ChurnWarning`, `ChurnModelConfig`, `ChurnStatistics` | `~>`, `~>`, `~>`, `~>` | 4 | N |
| 198 | [ ] | `internal/ops/model/custom_report.go` | `CustomReport` | `~>` | 1 | N |
| 199 | [ ] | `internal/ops/model/dashboard_screen.go` | `DashboardScreen`, `DashboardWidget` | `~>`, `~>` | 2 | N |
| 200 | [ ] | `internal/ops/model/performance_test_result.go` | `PerformanceTestResult` | `performance_test_results` | 1 | - |
| 201 | [ ] | `internal/pkg/metrics/bridge_sink.go` | `BridgeMetricRow` | `bridge_metrics` | 0 | T、U、L |
| 202 | [ ] | `internal/repository/ab_exposure.go` | `AbExposure` | `ab_exposures` | 1 | U、L |
| 203 | [ ] | `internal/repository/email_account_repo.go` | `EmailAccountRow` | `email_accounts` | 0 | T、U、L |
| 204 | [ ] | `internal/repository/kb_document_chunk_repo.go` | `KBDocumentChunkRow` | `kb_document_chunks` | 1 | U、L |
| 205 | [ ] | `internal/service/email.go` | `EmailAccount` | `email_accounts` | 1 | U、L |

## 附录 A：`allModels()` 登记但索引未捕获 `TableName()` 的结构体（13）

U 标记的反向核对：这些模型注册进建表清单，但 `TableName()` 声明未被同一正则捕获（多为依赖 GORM 默认复数命名）。逐实体审查时确认「实际表名 = 结构体复数」是否符合本仓命名约定。

`EmailDraft`, `EmailSend`, `KuaishouCard`, `RagMessage`, `RagSession`, `SmsAliyunConfig`, `SmsConfig`, `SmsDraft`, `SmsHuaweiConfig`, `SmsJob`, `SmsJobDetail`, `SmsRecord`, `SmsTencentConfig`

## 附录 B：声明 `TableName()` 但未进 `allModels()` 的结构体（14）

`AbExposure`, `BridgeMetricRow`, `DelayedOutboundReply`, `EmailAccount`, `EmailAccountRow`, `IntentLog`, `KBDocumentChunkRow`, `LiveCodeClickLog`, `QRCodeClickLog`, `ReachComplianceLog`, `SLAPolicy`, `SLAViolation`, `SmsJobMetric`, `TraceEvent`

每张此类表必须在 4.4 给出结论：**由哪一版迁移建表 / 是否死表 / 是否漏登记**。

## 附录 C：无 `tenant_id` 列的文件（T 标记，逐条论证归属）

| 文件 | 结构体 | 表 | 论证结论 |
|---|---|---|---|
| `internal/model/backup.go` | Backup, RestoreRecord | 动态 | 待论证 |
| `internal/model/config_param.go` | ConfigParam, ConfigParamAuditLog | config_params, config_param_audit_logs | 平台级（§三.5 已登记） |
| `internal/model/config_param_group.go` | ConfigParamGroup | config_param_groups | 待论证 |
| `internal/model/cron_job_lease.go` | CronJobLease | cron_job_leases | 待论证 |
| `internal/model/dictionary.go` | Dictionary, DictionaryTransition | dictionaries, dictionary_transitions | 待论证 |
| `internal/model/platform_config_kv.go` | PlatformConfigKV | 动态 | 平台级（§三.5 已登记） |
| `internal/model/tenant.go` | Tenant | tenants | 租户主表 |
| `internal/pkg/metrics/bridge_sink.go` | BridgeMetricRow | bridge_metrics | 待论证（并 4.1：模型不在 model 目录） |
| `internal/repository/email_account_repo.go` | EmailAccountRow | email_accounts | 待论证（并 4.1：模型声明在 repository 层） |

## 待决策

（审查中产生的人工决策项记在这里）

### #1 `TraceEvent` / `trace_events`（2026-10-21，已过四关，遗留 3 项决策）

本轮已落地：模型从 `internal/aiagent/llm/trace_context.go` 迁入 `internal/model/trace_event.go`（4.1）；登记进 `allModels()` + `coreOwnedTables`（消除 U）；`fetchTraceSummaryStats` 补 `WithContext(ctx)` 并把吞掉的 error 上抛（4.3 跨租户聚合泄漏）；删掉零引用死代码 `PublishToolCall`/`PublishDBOp`/`TraceRecorder`/`TraceContext.SetMetadata·GetMetadata·Metadata`（4.4）；修 `SpanID()`/`ChildSpan()` 读写不加锁的数据竞争（`-race` 复现）。

| # | 遗留项 | 现状证据 | 需决策的原因 |
|---|---|---|---|
| 1 | `trace_events` 有**两个建表来源**：手写 DDL `internal/migration/migrations/m_p1_migration.go`（无 `tenant_id`）+ AutoMigrate（有 `tenant_id`、带 6 个 `idx_trace_events_*`） | 实测 AutoMigrate 走了 `SELECT count(*) FROM pg_indexes ... idx_trace_events_tenant_id` 的补列/补索引路径 | 是把 DDL 从迁移里摘掉交给 AutoMigrate，还是反过来把 AutoMigrate 条目撤成迁移管理；两者并存时列/索引形状随启动顺序变 |
| 2 | 写入侧 `trace_sink.go` 用 `Table("trace_events")` + `map[string]any` 批量插，绕开刚建的 `model.TraceEvent` | `traceEventToRow` 手工构造 map key，字段与结构体 tag 需人肉对齐 | 改成 `.Create(&[]model.TraceEvent{})` 后序列化器（`serializer:json`）接管 metadata；收益是单一事实源，代价是批插路径重写 |
| 3 | trace 域**缺 Service/Repository 两层** | `controller/trace.go` 直接调 `llm.QueryTrace`/`llm.ListRecentTraces`（Handler → 基础设施包） | 按五层铁律应补 `service/trace.go` + `repository/trace_repo.go`；但 llm 包持有查询实现，搬迁会牵动 `aiagent` 依赖方向，属重构而非审查修复 |

### #2 `RAGAnswerCache` / `rag_answer_cache`（2026-10-21，已过四关，遗留 2 项决策）

本轮已落地：模型迁入 `internal/model/rag_answer_cache.go`，同包 `answer_cache_model.go` 更名 `answer_cache_types.go` 只留 DTO（4.1）；`allModels()` 改用 `model.RAGAnswerCache` 并删掉为取模型而存在的 `ragcachemodel` 别名导入（4.1 装配层反向依赖业务包）；补 `knowledgeOwnedTables` 归属登记；按 `store_pg.go` 头注释里一直没落地的 DDL 补 `(tenant_id, kb_id, prompt_version)` 复合索引（真库 EXPLAIN 由 `Index Cond: tenant_id` + Filter 升级为三列 Index Cond）；新增跨租户 `Delete` 隔离用例（`Table()+Delete(nil)` 形态经 callback 正确注入，无缺陷但原先零覆盖）。

| # | 遗留项 | 现状证据 | 需决策的原因 |
|---|---|---|---|
| 1 | 写读仍全部走 `Table("rag_answer_cache")` + `map[string]any`，`model.RAGAnswerCache` 只用于建表 | `store_pg.go` 四处 `.Table(...)`；`kb_canary_test.go`/`kb_cache_baseline_test.go` 里还有 `database.Raw("SELECT COUNT(*) FROM rag_answer_cache ...")` 的测试侧裸 SQL | 与 #1 第 2 条同因：向量列要 `?::vector` 强制转换，GORM 结构化写入需 `gorm.Expr`，改成类型化 Create 收益有限，先记债 |
| 2 | 库上多出一列 `updated_at TIMESTAMPTZ NOT NULL DEFAULT now()`，模型无 `UpdatedAt` 字段 | 真库 `\d rag_answer_cache`；该列不在任何 DDL 注释与模型里 | 是历史 AutoMigrate 形状残留（模型曾带 UpdatedAt）还是别的写入方在用？删列属破坏性变更，需确认 |

### #3 `BrowserAuditDigest` / `BrowserAuditPruneRun`（2026-10-21，已过四关，无遗留决策）

四关读数：

- **4.1 五层**：链完整且无越层 —— `router/browser_automation_routes.go:125` → `controller/session.go:125 Export` → `service/session.go:223 SessionExport` → `repository/audit_digest.go BrowserAuditDigestRepository` → model；写侧 `service/retention.go` → `repository/command_log.go PruneBefore`。两表都在 `internal/browser_automation/model/` 域内目录，合规。
- **4.2 GORM**：`PruneBefore` 的库内 sha256 走 `Table(...).Select(...)` 的 GORM 链，无 `.Exec(`/`.Raw(`；`scripts/check-gorm-only.sh` 复验「业务包 0 处」。
- **4.3 多租户**：两表都有 `tenant_id`（`audit_digest.go:29/77`），但**调用方给的 ctx 是裸的** —— `router/browser_automation_routes.go:163` 用 `context.Background()` 起 bootstrap goroutine，`StartAuditRetention` → `pruneAuditOnce` 把它一路传给 `PruneBefore`/`PruneSnapshotText`。saas 形态下隔离 callback 对未绑租户的租户表访问 fail-closed ⇒ 每小时一条 `logger.Warnf`、**一行都不裁**：表现不是刷屏而是「90 天保留期在云上从未生效」，`browser_command_log` 无界增长（正是 G19 立项要防的事）。已修：两次裁剪各自过 `tenant.Fanout`（与仓内其余 31 处后台任务同口径），分开扇出以免一处故障带走另一处。
- **4.4 僵尸**：读侧有真消费者（`SessionExport` 把摘要带进审计包，`controller/session_export_a6_test.go` 覆盖），`allModels()` 建表登记齐全（`migrate.go:520-521`），迁移 `v3.46.0` 只加 `cutoff_source` 列不删对象 —— 无僵尸代码。

新增回归 5 条：`repository/retention_tenant_fanout_test.go`（租户 11 的扫描只裁 11 的 3 行、32 的 5 行不动，摘要/留痕不跨租户且留痕带发起租户号；裸 ctx 被拒且一行不删）、`service/retention_tenant_fanout_test.go`（on 模式逐活跃租户各裁一次且 ctx 已绑、off 模式退化单默认租户、一处失败不带走另一处）。

### #4 `BrowserCommandLog` / `browser_command_log`（2026-10-21，已过四关，无遗留决策）

四关读数：

- **4.1 五层**：写侧唯一入口 `Executor.appendCommandLog`（`service/executor.go:216`）→ `repository/command_log.go Append`；读侧 `SessionService.CommandFlow·SessionExport`（`service/session.go:73/133/234`）→ `controller/session.go Export` → `router:125`。模型在域内 `model/` 目录，Executor 不直连 gorm，Repository 无业务判断 —— 无越层。
- **4.2 GORM**：仓储只有 `Create/Find/Delete` + `Table().Select()` 聚合，无 `.Exec(`/`.Raw(`；批摘要的 sha256 在库内算（G19 的口径：payload 从不过网络），仍走 GORM 链，门禁复验通过。
- **4.3 多租户**：`TenantID` 由隔离 callback 从 ctx 填，**写入路径的 ctx 来源有五条**，逐条核过：HTTP `controller/task.go:285`（请求 ctx，中间件已绑）✅、cron 触发 `service/cron.go:208`（`tenant.OnlyTenant(cur.TenantID)` 回钉）✅、workflow 节点（`workflow_dispatcher.go:370` 显式 `WithTenantID`）✅、MCP 工具（agent 请求 ctx）✅、**到期重试扫描 ❌**。缺陷：`StartRetryScanner` 由 bootstrap 的 `context.Background()` 起（`router/browser_automation_routes.go:162/168`），`scanDueRetries` 拿裸 ctx 调 `ClaimDueRetries`（`browser_tasks` 是租户表）⇒ on 模式 fail-closed，每分钟一行「重试扫描失败」，自动重试整条功能静默停摆；即使命中，`runRetry → RunTask` 往下写 session/命令日志时也取不到租户。已修：`scanDueRetries` 整轮过 `tenant.Fanout`，租户内认领、租户内触发（`scanDueRetriesForTenant` 为可测单元，归属门与「认领即清 next_retry_at」的语义不变）。
- **4.4 僵尸**：无。三个仓储方法都有消费者（`Append`=Executor、`ListBySessionID`=命令流查询+审计导出、`PruneBefore`=保留期治理）；字段逐个有读写面，含 `Ok *bool` 三态（v3.44.0 由 bool 改三态，`command_log_outcome_b20b_test.go` 钉住「下发帧不得自带 ✓」）与 `StepID`（进 `auditRowHashSQL` 的 COALESCE 指纹）。`llm_plans` 的快照裁剪是另一实体（#6），本轮未动。

新增回归 2 条：`service/feedback_retry_tenant_test.go`（on 模式两个租户各自的到期重试各被认领一次、runner 收到的 ctx 绑着自己租户；off 模式退化单默认租户且 ctx 非裸；认领后 `next_retry_at` 必须清空否则下一分钟双发）。修复前后各跑一次：改前 `got=map[]`（fail-closed 实证），改后绿。`./internal/browser_automation/...` 全包复跑全绿（controller/dto/platform/repository/service）。


