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
| 5 | [x] | `internal/browser_automation/model/cron.go` | `BrowserCronTrigger` | `browser_cron_triggers` | 1 | - → 已修 4.3（软删残留占键位） |
| 6 | [x] | `internal/browser_automation/model/llm_plan.go` | `BrowserLLMPlan` | `browser_llm_plans` | 1 | - → 已清 4.4（两个零调用越权读口） |
| 7 | [x] | `internal/browser_automation/model/outreach_dedupe.go` | `BrowserOutreachDedupe` | `browser_outreach_dedupe` | 1 | - → 已清 4.4（无参全局 db 构造口） |
| 8 | [x] | `internal/browser_automation/model/outreach_receipt.go` | `BrowserOutreachReceipt` | `browser_outreach_receipts` | 1 | - → 已修 4.3（归属子查询漏绑 ctx，on 模式 500）、已清 4.4（会话级读口 + 无参全局 db 构造口） |
| 9 | [x] | `internal/browser_automation/model/profile_health.go` | `BrowserProfileHealth` | `browser_profile_health` | 1 | - → 已同步 4.3 口径（「每平台一行」实为「每租户每平台一行」）并补三条 on 模式锁 |
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



### #5 `BrowserCronTrigger` / `browser_cron_triggers`（2026-10-21，已过四关，遗留 1 项决策）

四关读数：

- **4.1 五层**：六条路由（`router/browser_automation_routes.go:136-141`）→ `controller/cron.go`（薄层，只做 id 解析 + DTO 绑定 + 错误分类）→ `service/cron.go CronService`（唯一持有调度语义：`toSixField`/`ValidateCronExpr`/`applySchedule`/`registerWithUser`）→ `repository/cron.go`→ model；装配在 routes:30/69，`taskSvc.SetTriggerRemover(cronSvc)`（routes:72）是任务侧对触发器回收的**可选接口注入**，不反向依赖 controller。一处口径登记：`GetByID/ListByUser/Delete` 在 Repository 里 `JOIN browser_tasks ... AND user_id = ?`——这是本仓既定的**数据范围约束**（与 `BrowserTaskRepository`「除 `GetByIDAnyUser` 外全部强制 userID 过滤」同一条），不是业务判断，保持原位。
- **4.2 GORM**：仓储只有 `Create/First/Find/Save/Update/Updates/Delete + Joins`，无 `.Exec(`/`.Raw(`；`check-gorm-only.sh` 复验「业务包 0 处」。建表 DDL 在 `v3_37_0`（豁免目录），加列在 `v3_40_0`（`time_zone`）。
- **4.3 多租户**：表有 `tenant_id`（`model/cron.go:14`，真库 `tenant_id bigint not null default 1`），唯一键已由租户迁移降为 `(tenant_id, task_id)`。**ctx 来源五条全核过**：HTTP（`auth.Use(middleware.TenantMiddleware())`，`router/router.go:413`）✅；启动恢复 `RestoreAll` 走 `tenant.WithSystemScope`（全租户扫活行，平台级）✅；调度回调先 system scope 读、再 `tenant.OnlyTenant(cur.TenantID)` 把记账与 `RunTask` 回钉归属租户 ✅；任务删除/改类型级联 `RemoveByTask` 取请求 ctx ✅；反馈重试链路（#4 已修）不触本表 ✅。**本轮真缺陷在写侧不在读侧**：删除走软删，而 `(tenant_id, task_id)` 唯一键位不认软删——行还占着位，重名预检查 `GetByTaskID`（带 `deleted_at IS NULL`）读不到它于是放行，INSERT 撞约束 ⇒ 裸 error ⇒ 控制器 500。即「删掉再建一个」（用户改时间的常规走法）与「cron→非 cron→cron」两条路都必然失败。开发库实测：**53 行里 52 行是软删残留，50 条在世 cron 任务因此一个触发器都挂不上**；而 `RestoreAll` 只看活行，所以这些任务在界面上「配过定时」却永不执行——比报错更难发现。已修：`repository/cron.go Delete/DeleteByTaskID` 改 `Unscoped()` 硬删（触发行没有历史读家，执行事实记在 `browser_sessions`/`browser_command_log`），并补 `v3.67.0` 迁移摘除存量残留、把键位归还任务。
- **4.4 僵尸**：无。仓储十个方法逐个有消费者（`Create`/`Update`/`UpdateEnabled`/`Delete`/`GetByID`=HTTP 六口，`GetByTaskID`=重名检查+调度回调，`ListByUser`=列表，`ListAllEnabled`+`UpdateTimes`=恢复与记账，`DeleteByTaskID`=任务级联）；字段逐个有读写面（`TimeZone` 进 `toSixField` 前缀 `CRON_TZ=`，`NextRunAt/LastRunAt` 由 `UpdateTimes` 成对写并被列表读回）。`DeletedAt` 本轮**刻意保留**：改造后代码不再产生软删行，但没跑过 v3.67.0 的库（回滚、老实例灰度）里残留仍在——留着它让这些残留继续被读侧排除，否则 `RestoreAll` 会把已删的定时器重新武装起来。

新增回归 7 条：`service/cron_delete_keyslot_test.go`（删掉重建必须成功 + 物理行数归一 + 重建后条目在、`next_run_at` 有值；任务改走再改回同一条路）——修复前实测红在 `duplicate key value violates unique constraint "idx_browser_cron_triggers_task_id"`，修复后绿；`migration/migrations/v3_67_0_..._test.go`（on 模式 system scope 下两租户残留全摘、活行逐字不动且 `tenant_id` 不被改写、幂等二跑、摘完键位真能再插活行、表缺失时不报错、nil db fail-closed、v3.67.0 已注册）。`./internal/browser_automation/{service,controller,repository}` 与 `./internal/migration/migrations` 复跑全绿。

| # | 遗留项 | 现状证据 | 需决策的原因 |
|---|---|---|---|
| 1 | 同一张表上**两套重叠唯一约束与重复索引**：`uk_browser_cron_task UNIQUE (tenant_id, task_id)`（v3_37_0 手写 DDL 后经租户迁移改造）与 `idx_browser_cron_triggers_task_id UNIQUE (tenant_id, task_id)`（模型 tag `uniqueIndex`）；`enabled` 同样有 `idx_browser_cron_enabled` + `idx_browser_cron_triggers_enabled` | 真库 `\d browser_cron_triggers` 六个索引里有两对同列；测试库（只跑 AutoMigrate，不跑编号迁移）只有模型那一支 ⇒ 同一份代码在两种来源下索引形状不同 | 与 #1 第 1 条同因（手写 DDL 与 AutoMigrate 并存）。收口方向是删掉手写 DDL 那一支的重复索引、让模型 tag 成为唯一事实源；属 DDL 破坏性变更 + 迁移 Down 归属判定，需确认后再动 |

### #6 `BrowserLLMPlan` / `browser_llm_plans`（2026-10-21，已过四关，遗留 1 项决策）

四关读数：

- **4.1 五层**：这张表没有自己的 CRUD 路由，两条链路都齐：写侧 `service/brain.go`（三处 `planRepo.Create`：`recordAuxPlan:131` judge/summary、`recordJevPlan:150` jev_choice、`planRound:287` plan）→ `repository/llm_plan.go`；读侧 `GET /api/browser-automation/sessions/:id/export` → `controller/session.go:125` → `service/session.go:246 ListBySessionID` → 仓储。装配在 `router/browser_automation_routes.go:31`（建仓储）与 `:65`（`SetLLMPlanRepository` 注入会话服务）、`:169`（治理 goroutine）。模型在 `internal/browser_automation/model/`，无越层。一处口径登记：`SessionService` 用 setter 注入而非构造参数，是为可选装配（仓储缺位时导出仍出其余分量），与 #5 的 `SetTriggerRemover` 同一形状。
- **4.2 GORM**：仓储只有 `Create` + `Where/Order/Limit/Find` + `Model(...).Update`（`PruneSnapshotText` 的 `id IN (?)` 是 GORM 子查询而非拼串），无 `.Exec(`/`.Raw(`；本轮 `check-gorm-only.sh` 复验「业务包 0 处」。建表在 `v3_37_0`、`session_id/kind` 加列在 `v3_40_0`（均在豁免目录）。
- **4.3 多租户**：表有 `tenant_id`（`model/llm_plan.go:12`，`not null;default:1;index`），无跨租户唯一键。**写侧 ctx 来源五条全核过**：HTTP 触发的执行（`middleware.TenantMiddleware()`，`router/router.go:413`）✅；cron 触发先 system scope 读再 `tenant.OnlyTenant(cur.TenantID)`（`service/cron.go:204`）✅；反馈重试扫描逐租户扇出（`service/feedback_retry_tenant_test.go` 已锁）✅；MCP 浏览器工具 `tenant.BackgroundWithTenant(ctx)`（`aiagent/agent/tooluse/browser_tools.go:143`）✅；workflow `browser_task` 节点用 `context.WithoutCancel(ctx)`，Values 随租户一起保留（`internal/service/workflow_node_executors.go:199`）✅。**读侧**只有导出（请求 ctx），**治理侧** `PruneSnapshotText` 走 `tenant.Fanout`（`service/retention.go:115`，#3/#4 那批已修并锁）。本轮补齐仓储层第二道遮挡的契约锁：`repository/llm_plan_tenant_test.go` 三条（写侧填租户 + 裸 ctx 拒、同一 `session_id` 两租户各读各的、快照裁剪不跨租户）。**这条锁验过它会红**：临时把 `ListBySessionID` 换成 `WithSystemScope` 后三条断言全红（各读到 2 条 + 裸 ctx 未拒），换回后绿；`PruneSnapshotText` 同样做法证出「未绑 ctx 未拒 / 裁了 0 行 / 租户 35 的原文被别人裁掉」。
- **4.4 僵尸**：删掉 `GetByID(ctx, id)` 与 `ListByTaskID(ctx, taskID, limit)`（接口 + 实现）。引用面证据三条：① 删除后 `go build ./...` 全绿 ⇒ 编译面零调用方；② 这两个方法**没有任何 HTTP 读家**——`/api/browser-automation` 全域路由里不存在按 plan id 或按 task id 取 LLM 记录的端点（`grep` 路由表为空），前端 `user-web/src/api/browserAutomation.js` 只有 `sessions/:id/export` 一处消费本表，且 `views/browserAutomation/Monitor.vue:646` 是把整个导出载荷落盘成 JSON 文件，`llm_plans` 分量确实在消费面内；③ 形状上它们缺归属过滤（`Where("id = ?")` / `Where("task_id = ?")`，同域 `session.go`/`outreach_receipt.go` 一律带 `userID`），留着就是仓储面上摆两个越权读口。同轮把三处口径不实的注释改成四类（`plan/judge/summary/jev_choice`）：`model/llm_plan.go` 头注释与 `Kind` 字段、`service/brain.go:278`、`service/session.go:206`；并改掉 `SessionExportRow` 注释里「要快照原文走 llm_plans 查询端点」这句不存在的路（见遗留项 1）。

新增回归 3 条（`repository/llm_plan_tenant_test.go`，on 模式）。`./internal/browser_automation/...` 与 `./internal/migration/...` 全量复跑绿。

| # | 遗留项 | 现状证据 | 需决策的原因 |
|---|---|---|---|
| 1 | `snapshot` 与 `task_id` 两列**只写不读**：删掉 `ListByTaskID` 后，导出行（`SessionExportRow`）不带这两列，全仓再无任何代码读它们；而 `snapshot` 每行按 64 KiB 截断写入，G19 整套治理（`PruneSnapshotText` + 分批 + 语句计数锁）存在的唯一目的就是给这个没人读的列腾地方 | 写：`service/brain.go:280`（`truncate(snapshot, 64*1024)`）、`:276`（`TaskID`）。读：`SessionExport`（`service/session.go:244-255`）取 ID/Kind/Goal/Steps/Reasoning/Model/TokenIn/TokenOut/CreatedAt，无 Snapshot/TaskID；`PruneSnapshotText` 只在 `WHERE snapshot <> ''` 谓词里出现，不进投影 | 两条收口方向互斥且都有代价：给 `snapshot` 加读口（brain 决策回放/客服排障要的正是「当时喂给模型的那份快照」，代价是导出体积与脱敏面），还是承认它没人读、连同 G19 的裁剪与 `TaskID` 一起摘掉（省掉最大的一列，代价是再也不能复现任何一次 LLM 输入）。属产品取舍，不在审查修复的授权范围内 |

### #7 `BrowserOutreachDedupe` / `browser_outreach_dedupe`（2026-10-21，已过四关，无遗留决策）

四关读数：

- **4.1 五层**：这张表同样没有自己的 CRUD 路由，链路只有一条且各层齐：`service/executor.go:1079`（写步前）与 `:1545`（verified 后）→ `service/outreach_dedupe.go`（`resolveOutreachDedupe`/`checkOutreachDedupe`/`normalizeOutreachURL`/`recordOutreachDedupeSend`，去重策略全在这层：URL 归一化、键在检查点冻结、fail-open 放行）→ `repository/outreach_dedupe.go`（只有 `Where/First` 与 `Clauses(OnConflict)`，无业务判断）→ model。装配在 `router/browser_automation_routes.go:36` + `:53`（`SetOutreachDedupeRepository`，可选注入，nil 时跳过去重）。模型在 `internal/browser_automation/model/`。
- **4.2 GORM**：仓储无 `.Exec(`/`.Raw(`；`OnConflict{DoNothing}` 是 clause 结构体不是拼串。建表 DDL 在 `v3_50_0`（豁免目录），其 `Up` 的 `IF NOT EXISTS` 与模型标签同名同列，形状漂移由 #4.3 的规范化统一收口。
- **4.3 多租户**：表有 `tenant_id`（`model/outreach_dedupe.go:8`）。**键位形状实测**（不是推断）：真库 `user_db` 与测试槽库的 `pg_indexes` 都是 `uk_outreach_dedupe UNIQUE (tenant_id, platform, target_url, action, copy_hash)`——模型标签建的是四元键，第五列由 `tenancyidx.Normalize`（`internal/pkg/db/migrate.go:641`，与迁移 v3.59.0 同一实现，AutoMigrate 后执行、fail-closed 拒绝启动）升格。ctx：读写都发生在执行器的 `execCtx` 上，而 `utils.SafeGoDetached` 用的是 `context.WithoutCancel`（`internal/pkg/utils/recover.go:54`），只剥取消链、Values 随租户一起进 goroutine；四条派发入口（HTTP / cron `OnlyTenant` / 反馈重试扇出 / MCP·workflow）已在 #6 逐条核过。**这条形状此前只有 off 模式覆盖**（`outreach_dedupe_test.go` 三条 + `v3_50_0` 迁移测一条都跑在 `context.Background()`），补 `repository/outreach_dedupe_tenant_test.go` 三条 on 模式锁：同四元键跨租户各成立一条（B 读不到 A 的、且 B 插得进自己的，判据取物理行数）、同租户重复记录仍只一行（防「为补租户维度把键拆掉」的反向失效）、裸 ctx 两个方法都 fail-closed。**锁验过会红**：把 `uk_outreach_dedupe` 临时退回不含 `tenant_id` 的四列形状，物理行数腿即以 `got=1 want=2` 红——正是 `DoNothing` 静默吞掉 B 的行的形态（B 从此每次调度重发同一条内容，而库里看不出它本该被去重）；改回后绿。本表无保留期裁剪（行数≈历史发送数、每行几十字节，且「同一目标同一文案只触达一次」是设计意图而非疏漏），故不挂 G19。
- **4.4 僵尸**：删掉 `NewBrowserOutreachDedupeRepository()`（无参、内部取 `_db.GetDB()` 全局句柄）。引用面证据：全仓 `--include=*.{go,md}` 只命中它自己的定义与文档行，装配与测试一律走 `...WithDB`；删除后 `go build ./...` 绿、`_db` 导入随之移除。留着不只是死代码——它是本域仓储面上唯一一条「不接 db 也能摸库」的后门，与「DI 在装配层」相违。接口两个方法各有两家消费者（`FindOutreachHit`←`checkOutreachDedupe`，`RecordOutreachSend`←`recordOutreachDedupeSend`），字段 `Platform/TargetURL/Action/CopyHash` 进唯一键与查询、`TenantID` 由隔离 callback 填。`TaskID/SessionID` 与 #6 遗留项 1 同判据（整行读回但无人消费，回执↔去重行的对账走 `copy_hash`），不另开决策项。

新增回归 3 条（`repository/outreach_dedupe_tenant_test.go`）。`./internal/browser_automation/repository` 全量复跑绿（含既有 3 条 dedupe 用例与 `v3_50_0` 迁移测）。

### #8 `BrowserOutreachReceipt` / `browser_outreach_receipts`（2026-10-21，已过四关，遗留 1 项决策）

四关读数：

- **4.1 五层**：一条读链、一条写链，各层齐。读：`user-web/src/views/browserAutomation/Detail.vue:447` → `api/browserAutomation.js:102` → `GET /api/browser-automation/tasks/:id/receipts`（`internal/router/browser_automation_routes.go:128`）→ `controller/task.go:375 ListReceipts` → `service/outreach_receipt.go:209 ListOutreachReceipts` → `repository/outreach_receipt.go ListByTaskID`。写：`service/executor.go:1531`（verified 分支）→ `enqueueOutreachReceipt`（只挂进 `pendingReceipts[session.ID]`，不当场落库）→ 会话收口 `captureSessionReceipts`（拍验收截图 → 落库 → 回收 tab，顺序固定）→ `RecordOutreachReceipt`。装配在 `router/browser_automation_routes.go:37` 建仓储，`:59`（Executor 写侧）与 `:62`（TaskService 读侧）分别注入。模型在 `internal/browser_automation/model/`，无越层；`Executor`/`TaskService` 走 setter 注入（可选装配，nil 时读侧返回空、写侧短路），与 #5/#6 同形状。
- **4.2 GORM**：仓储只有 `Create` 与 `Model/Where/Order/Limit/Find`，无 `.Exec(`/`.Raw(`；本轮 `check-gorm-only.sh` 复验「业务包 0 处」。建表 DDL 在 `v3_50_0`（豁免目录）。
- **4.3 多租户**：表有 `tenant_id`（`model/outreach_receipt.go:23`），**刻意无唯一键**——这是它与去重表的核心形状差异（4.4 的变异③就是给这条差异上的锁）。写侧 ctx 与 #6/#7 同源（`execCtx` 经 `context.WithoutCancel` 保留租户 Values）。**本轮查出真缺陷（在读侧）**：`ListByTaskID` 的归属过滤子查询建在 `r.db.Model(...)` 上，即无租户上下文的全局句柄；而 `userID` 在唯一的生产调用点恒非零（`controller/task.go:380` 传 `ctx.GetUint("user_id")`）。on 模式下租户 callback 对这条子查询 fail-closed ⇒ 子会话 SQL 建为空 ⇒ 渲染成 `task_id IN ()` ⇒ PG 42601，`GET /tasks/:id/receipts` 在 SaaS 模式下整条必然 500、任务详情的验收表格永远拿不到数据；off 模式直通，既有覆盖（`outreach_receipt_test.go` 三条全跑 `context.Background()`）看不出异常。修复：子查询改 `r.db.WithContext(ctx)`（同 `repository/live_code_qr.go:139`、`knowledge_base.go:131` 的写法），顺带让归属子查询本身也受租户谓词遮挡。**补 `repository/outreach_receipt_tenant_test.go` 三条 on 模式锁**：① 写侧填发起租户 + 裸 ctx 被拒且不留行（物理计数判据）；② 同一 `task_id` 在两租户各有一行时各读各的（断言取「1 行且是本租户」而非「0 行」——B 本该有自己的那行，要防的是谓词失效时多出的第 2 行），含 `userID` 过滤两条腿（B 拿 A 的 user_id 读为空、A 用自己的读到 1 行）与裸 ctx 拒绝；③ 同四元组重复落库必须 3 行全留，另一租户写同四元组既不吞别人的也不被吞（物理行数 4）。**三条锁逐条验过会红**：(a) 子查询退回 `r.db.Model(...)` ⇒ `带 userID 的读取失败: SQLSTATE 42601`，即修复前的生产形态；(b) 外层读换成 `WithSystemScope` ⇒ 「各读到 2 行」「裸 ctx 未被拒」「B 的写入把 A 的行数改成 4」四条断言同红；(c) 给四元组加 `uniqueIndex`（经 `tenancyidx.Normalize` 升为租户内五列键）⇒ 同租户第 2 次落回执即 23505，即「回执表退化成闸门、重复交付证据被静默吞掉」的形态。三处变异全部改回，`./internal/browser_automation/{repository,service,controller}` 复跑全绿。
- **4.4 僵尸**：删掉 `ListBySessionID(ctx, sessionID, userID)`（接口 + 实现）与 `NewBrowserOutreachReceiptRepository()`（无参取 `_db.GetDB()` 全局句柄，与 #7 同判据，删除后 `_db` 导入一并移除）。引用面证据三条：① 删除后 `go build ./...` 全绿 ⇒ 编译面零调用方；② 路由表里回执只有一个端点（`grep -rn "receipts" internal/router/` 单命中 `tasks/:id/receipts`），`SessionExport` 的返回元组不含回执，前端两个消费点分别指向任务详情与 sessions/export，无会话级回执调用；③ `docs/superpowers/plans/2026-09-30-outreach-p0.md` Chunk5 定的展示面就是任务详情（「这批触达发出去了什么」，跨 session 的 cron 活动没有单个会话视角——这条理由也写在 `controller/task.go:373` 的注释里）。日后若真要会话级回执，正确做法是把它加进 `SessionService.SessionExport` 的返回元组（少一个值调用方编译不过），而不是再摆一个没人读的独立方法。字段面：`Platform/Action/TargetURL/CopySnapshot/ScreenshotURL/Verified/CreatedAt` 逐个进 `Detail.vue:187-259` 的表格列；`SessionID`/`CopyHash` 见遗留项 1。同轮校正两处不实注释：`model/outreach_receipt.go` 头注释把去重键写成「四元组」而漏掉 `tenant_id` 前导列；`repository/outreach_receipt.go` 上残留的 `receiptQuery` 旧函数名与「经 session_id 落归属」这句已不成立的说明。

| # | 遗留项 | 现状证据 | 需决策的原因 |
|---|---|---|---|
| 1 | `session_id` 与 `copy_hash` 两列**只写不读**：删掉 `ListBySessionID` 后 `session_id` 不再出现在任何 WHERE 里，`copy_hash` 全仓无读家（`grep -rn "\.CopyHash"` 业务侧只命中去重层自己构造键的那行）。模型头注释里「`copy_hash` 与去重行同键，保证回执↔去重行可 join：『有回执无去重行』= 落库失败」这条对账**只存在于注释**，没有任何代码或 SQL 跑过它 | 写：`service/outreach_receipt.go:143`（`SessionID: session.ID`）、`:147`（`CopyHash: p.copyHash`）。读：`ListByTaskID` 的投影是整行（两列随 JSON 返回），但 `Detail.vue` 的表格列不含它们；无按 `session_id`/`copy_hash` 的检索、统计或对账语句 | 两个方向都有代价：把「有回执无去重行 / 有去重行无回执」这条对账做成真读口（它原本是给「落库失败 vs 收取失败」这个区分兜底的，正是本表立项理由），还是承认它只是设计意图、连同 `session_id` 的会话归属线索一起按 #6 遗留项 1 的口径处置。`session_id` 还是「这张截图来自哪次会话」在库里唯一的挂钩，摘掉会让回执与 `browser_sessions.final_screenshot_url` 的关系退化为仅靠 `task_id` 归组，属产品/审计取舍 |

### #9 `BrowserProfileHealth` / `browser_profile_health`（2026-10-21，已过四关，遗留 2 项决策）

四关读数：

- **4.1 五层**：写读各一条链，齐。写：`service/executor.go`（`detectBlockedIfFatal` 命中封号）→ `service/profile_fuse.go:19 recordProfileBlocked` → `repository/profile_health.go:40 MarkBlocked`，另落一帧 `block_fused` 审计到 `browser_command_log`。门：`service/task.go:467`（`RunTask` 启动前的熔断门，命中则 `stateConflict` 拒绝执行，并把 `h.Reason` 拼进给用户的错误消息）。人工出口：`POST /api/browser-automation/admin/profile/recover`（`router/browser_automation_routes.go:154`，`baAdmin` 组带 `middleware.AdminAuthMiddleware()`）→ `controller/task.go:356 RecoverProfile` → `service/task.go:513 RecoverProfile` → `MarkRecovered`。装配在 `router/browser_automation_routes.go:35` 建仓储，`:50`（Executor 写侧）与 `:61`（TaskService 门+恢复）分别注入；两处注释都写明「漏装 = 熔断面静默关闭」。模型在 `internal/browser_automation/model/`，无越层；setter 注入与 #5–#8 同形状（仓储缺位时功能降级而非 panic）。
- **4.2 GORM**：仓储只有 `Where/First/Create/Save/Model.Updates`，无 `.Exec(`/`.Raw(`；本轮 `check-gorm-only.sh` 复验「业务包 0 处」。建表 DDL 在 `v3_49_0`（豁免目录，`CREATE TABLE IF NOT EXISTS`），同时该实体已登记进 `allModels()`（`internal/pkg/db/migrate.go:551`，登记理由与该处注释一致：漏登记 ⇒ 全新部署没有这张表 ⇒ 熔断落不下去，而写侧只 Errorf 不判红 ⇒ 风控闸在全新部署上永不关）。
- **4.3 多租户**：表有 `tenant_id`（`model/profile_health.go:16`）。**键位形状实测**（不是推断，与 #7 同法）：开发库 `pg_indexes` 里两个唯一索引都已是 `(tenant_id, platform)`——`browser_profile_health_platform_key`（v3_49_0 手写 DDL 的 `platform VARCHAR(64) NOT NULL UNIQUE` 升格而来）与 `idx_browser_profile_health_platform`（模型标签）；`migrate.go:549-550` 的登记注释也已把「每平台一行」明确为「每租户每平台一行」，本轮把模型头注释与仓储接口注释的口径同步过来（原文只说「每平台一行」，在 SaaS 下是不实的：照字面实现就是全实例共享一个熔断状态）。ctx 三条腿：HTTP 门与恢复走请求 ctx（`middleware.TenantMiddleware()`，`router/router.go:413`）✅；熔断写入走 Executor 的 `execCtx`（`utils.SafeGoDetached` 用 `context.WithoutCancel`，Values 随租户进 goroutine，`internal/pkg/utils/recover.go:54`）✅；cron 起跑先 `tenant.OnlyTenant(cur.TenantID)`（#5/#6 已逐条核过）✅；`AdminAuthMiddleware` 不设 system scope（只置 role/user_id），所以管理员恢复的也只是自己租户那一行，不存在「一次恢复抬起全部租户的闸门」。**补 `repository/profile_health_tenant_test.go` 三条 on 模式锁**：① 同平台名在两租户各成立一行（B 读 A 的行必须 `ErrRecordNotFound`、B 插得进自己的、物理行数=2）；② A 的 `MarkRecovered` 只清 A 的（含 `MarkBlocked` 的 Save 分支重复熔断不新增行、恢复后 `tenant_id` 不被改写）；③ 三个方法在裸 ctx 一律 fail-closed 且不留行。**三条锁逐条验过会红**：(a) 把 `platform` 的唯一索引改名成 `global_uniq_*` 前缀（Normalize 对该前缀刻意豁免 ⇒ 得到真正的跨租户全局键）⇒ B 落熔断即 23505，两条用例同红；(b) `GetByPlatform` 换 `WithSystemScope` ⇒ B 读到 A 的熔断原因、裸 ctx 未被拒，且 B 的 `MarkBlocked` 转而撞上门禁「更新不可改写 tenant_id」——写侧守卫正好兜住读侧漏绑，这条副产物一并记下；(c) `MarkRecovered` 换 `WithSystemScope` ⇒ 「B 的闸门被别人的恢复抬起」+「裸 ctx 的 MarkRecovered 未被拒绝」同红。三处变异全部改回（`grep -rn MUT-` 无残留，`git diff` 对本实体只有注释增减、代码零改动）。
- **4.4 僵尸**：无。三个方法逐个有消费者（`GetByPlatform`=`RunTask` 门，`MarkBlocked`=`recordProfileBlocked`，`MarkRecovered`=`RecoverProfile`）；无第四构造口（只有 `...WithDB`，与 #7/#8 收口后的形状一致，本域仓储已无「不接 db 也能摸库」的后门）。字段面 `Platform`/`Blocked`/`Reason` 都在门的判断与错误消息里被读；`BlockedAt`/`UpdatedAt` 只写不读，与熔断路径完全没有读端点这一件事同源，见遗留项 1。

| # | 遗留项 | 现状证据 | 需决策的原因 |
|---|---|---|---|
| 1 | **熔断面只有「写」和「门」，没有「读」**：全域没有任何端点或页面能列出「哪些平台正处于熔断」，人工恢复口 `POST /admin/profile/recover` 要求调用方**先知道平台名**才能清；`BlockedAt`/`UpdatedAt` 两列因此全仓无读家 | 路由：`grep -rn "profile" internal/router/*.go` 只有 `:154` 这一条 POST，没有 GET；前端：`grep -rn "profile" user-web/src/api/ user-web/src/views/browserAutomation/` 零命中；`BlockedAt` 的引用只落在 `repository/profile_health.go` 的写入与本域测试里 | 补一个「熔断面」读端点（admin 组，按租户列 blocked 行 + 时间）是这本台账一路在避免的「加功能」，而它的替代方案（承认熔断原因只在 `RunTask` 被拒时随错误消息露面一次）等于接受：运维不能在熔断发生时看到它，只能等用户报「任务起不来」。属产品/运维取舍 |
| 2 | 同一张表上**两个同列唯一索引**：`browser_profile_health_platform_key`（v3_49_0 手写 DDL 的列内 `UNIQUE`）与 `idx_browser_profile_health_platform`（模型 `uniqueIndex`），升格后都是 `(tenant_id, platform)` | 开发库 `pg_indexes` 四条记录里两条唯一索引同列；fresh 库（只跑 AutoMigrate，迁移的 `CREATE TABLE IF NOT EXISTS` 落空）只有模型那一支 ⇒ 同一份代码在「迁移建表」与「AutoMigrate 建表」两种来源下索引形状不同 | 与 #5 遗留项 1、#1 第 1 条同因（手写 DDL 与 AutoMigrate 并存）。收口方向是删掉手写 DDL 那一支的重复索引、让模型标签成为唯一事实源；属 DDL 破坏性变更 + 迁移 Down 归属判定，需确认后再动 |


