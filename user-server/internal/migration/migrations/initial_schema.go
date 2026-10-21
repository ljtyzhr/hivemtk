package migrations

import (
	"context"
	"fmt"
	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// InitialSchemaMigration 初始版本迁移(原 V110Migration 重命名,语义化命名)
// 私域部署: 单租户,无 merchant_id 字段
type InitialSchemaMigration struct {
	db *gorm.DB
}

// NewInitialSchemaMigration 创建初始版本迁移
func NewInitialSchemaMigration(db *gorm.DB) *InitialSchemaMigration {
	return &InitialSchemaMigration{db: db}
}

// Version 返回版本号
func (m *InitialSchemaMigration) Version() string {
	return "v1.1.0"
}

// Name 返回迁移名称
func (m *InitialSchemaMigration) Name() string {
	return "初始版本迁移"
}

// Description 返回迁移描述
func (m *InitialSchemaMigration) Description() string {
	return "创建基础表结构"
}

// Up 执行升级
func (m *InitialSchemaMigration) Up(ctx context.Context) error {
	return nil
}

// Down 执行降级
func (m *InitialSchemaMigration) Down(ctx context.Context) error {
	return nil
}

var _ migration.Migration = (*InitialSchemaMigration)(nil)

// MarketingFlowSchemaMigration 营销自动化迁移(原 V130Migration 重命名)
type MarketingFlowSchemaMigration struct {
	db *gorm.DB
}

// NewMarketingFlowSchemaMigration 创建营销流程迁移
func NewMarketingFlowSchemaMigration(db *gorm.DB) *MarketingFlowSchemaMigration {
	return &MarketingFlowSchemaMigration{db: db}
}

// Version 返回版本号
func (m *MarketingFlowSchemaMigration) Version() string {
	return "v1.3.0"
}

// Name 返回迁移名称
func (m *MarketingFlowSchemaMigration) Name() string {
	return "营销自动化"
}

// Description 返回迁移描述
func (m *MarketingFlowSchemaMigration) Description() string {
	return "添加营销流程、触发器、动作表"
}

// Up 执行升级
func (m *MarketingFlowSchemaMigration) Up(ctx context.Context) error {
	m.db.Exec(`
		CREATE TABLE IF NOT EXISTS marketing_flows (
			id BIGSERIAL PRIMARY KEY,
			name VARCHAR(100),
			description VARCHAR(500),
			status VARCHAR(20) DEFAULT 'draft',
			trigger_type VARCHAR(20),
			trigger_config TEXT,
			flow_data TEXT,
			version INTEGER DEFAULT 1,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)

	m.db.Exec(`
		CREATE TABLE IF NOT EXISTS flow_executions (
			id BIGSERIAL PRIMARY KEY,
			flow_id BIGINT,
			trigger_id VARCHAR(50),
			user_id VARCHAR(50),
			status VARCHAR(20),
			current_node VARCHAR(50),
			execution_data TEXT,
			started_at TIMESTAMP,
			completed_at TIMESTAMP,
			error_message TEXT
		)
	`)

	m.db.Exec("CREATE INDEX IF NOT EXISTS idx_marketing_flows_status ON marketing_flows(status)")
	m.db.Exec("CREATE INDEX IF NOT EXISTS idx_flow_executions_flow ON flow_executions(flow_id)")
	m.db.Exec("CREATE INDEX IF NOT EXISTS idx_flow_executions_user ON flow_executions(user_id)")

	return nil
}

// Down 执行降级
func (m *MarketingFlowSchemaMigration) Down(ctx context.Context) error {
	declineTableDrop(m.Version(), "flow_executions", "marketing_flows")
	return nil
}

var _ migration.Migration = (*MarketingFlowSchemaMigration)(nil)

// RegisterMigrations 注册所有迁移
// 如果检测到重复版本号会 panic（开发期错误，必须立即修复）
func RegisterMigrations(registry *migration.MigrationRegistry, db *gorm.DB) {
	register := func(m migration.Migration) {
		if err := registry.Register(m); err != nil {
			panic(fmt.Sprintf("迁移注册失败: %v", err))
		}
	}

	register(NewInitialSchemaMigration(db))
	register(NewMarketingFlowSchemaMigration(db))
	register(NewUnmultitenantSchemaMigration(db))
	register(NewMerchantIDNullableMigration(db))
	register(NewWecomWebhookFieldsMigration(db))
	register(NewAIAgentSchemaMigration(db))
	register(NewAIAgentExtensionMigration(db))
	register(NewKnowledgeVectorMigration(db))
	register(NewRagHybridMigration(db))
	register(NewSOPExecutorMigration(db))
	register(NewConfidenceMigration(db))
	register(NewRagMonitoringMigration(db))
	register(NewAmountMoneyMigration(db))
	register(NewHumanizeEvaluatorMigration(db))
	register(NewFeedbackLoopMigration(db))
	register(NewAuthSecurityMigration(db))
	register(NewComplianceDEMigration(db))
	register(NewHP1Migration(db))
	register(NewLP1Migration(db))
	register(NewADomainP1Migration(db))
	register(NewShortLinkColumnsMigration(db))
	register(NewMP1Migration(db))
	register(NewLLMUsageRecordsMigration(db))
	register(NewAssetBundleMigration(db))
	register(NewLLMRoutingLogsMigration(db))
	register(NewLLMRoutingLogsExtendMigration(db))
	register(NewAgentAssetBindingMigration(db))
	register(NewUserBlacklistMigration(db))
	register(NewBridgeAccountMigration(db))
	register(NewWorkflowVisualOrchestratorMigration(db))
	register(NewMultilingualI18nMigration(db))
	register(NewMultilingualI18nP13Migration(db))
	register(NewKnowledgeWeightMigration(db))
	register(NewTelegramPollingLockMigration(db))
	register(NewAIPerfFAQSOPLayerMigration(db))
	register(NewAIAgentKBBindingMigration(db))
	register(NewKBUnificationMigration(db))
	register(NewRagSafetyAuditDropMigration(db))
	register(NewRagAlertsDropMigration(db))
	register(NewBridgeChannelNormalizeMigration(db))
	register(NewBridgeChannelUnifyV2Migration(db))
	register(NewBridgeChannelUnifyV3_18_1Migration(db))
	register(NewCustomerSessionUpdatedAtMigration(db))
	register(NewKnowledgeSearchLogProductOptionalMigration(db))
	register(NewDropCdpAutoReplyMigration(db))
	register(NewLiveCodeClickLogsMigration(db))
	register(NewBridgeMetricsMigration(db))
	register(NewCustomerIDStandardizeMigration(db))
	register(NewSoftDeleteMigration(db))
	register(NewEnumOptimizeMigration(db))
	register(NewIntentMergeMigration(db))
	register(NewSessionIDLengthMigration(db))
	register(NewAlertRuleMigration(db))
	register(NewUnifiedIDWidenMigration(db))
	register(NewCustomerOwnerAgentMigration(db))
	register(NewReachTablesMigration(db))
	register(NewSOPTimerSinkColumnsMigration(db))
	register(NewSOPHeatmapIndexMigration(db))
	register(NewEmailSmtpPasswordEncryptMigration(db))
	register(NewSOPExecutedNodesMigration(db))
	register(NewBanditRefluxLogMigration(db))
	register(NewEmbeddingSourceMigration(db))
	register(NewHandoffOutcomeMigration(db))
	register(NewAgentCheckpointMigration(db))
	register(NewChurnScoreMigration(db))
	register(NewTelegramGroupGateMigration(db))
	register(NewAdminPasswordGuardMigration(db))
	register(NewBrowserAutomationMigration(db))
	register(NewBrowserCommandLogMigration(db))
	register(NewBrowserTaskPlatformMigration(db))
	register(NewBrowserAuditRetryTzMigration(db))
	register(NewOperationLogEncryptFieldsMigration(db))
	register(NewBrowserRequireConfirmMigration(db))
	register(NewBrowserWriteLedgerConfirmBudgetMigration(db))
	register(NewBrowserCommandLogOkTriStateMigration(db))
	register(NewMessageHubOutboundPushBudgetMigration(db))
	register(NewBrowserAuditPruneCutoffSourceMigration(db))
	register(NewConfigParamGroupKeyUniqueMigration(db))
	register(NewBrowserTaskCopyTextMigration(db))
	register(NewBrowserProfileHealthMigration(db))
	register(NewBrowserOutreachDedupeMigration(db))
	register(NewBrowserTaskCampaignBudgetMigration(db))
	register(NewBrowserOutreachReceiptMigration(db))
	register(NewFailoverPolicyToConfigParamMigration(db))
	register(NewEmbeddingGlobalOverrideToConfigParamMigration(db))
	register(NewMenuGrantCeilingPruneMigration(db))
	register(NewClueGeoCaptureTypeSplitMigration(db))
	register(NewTenantTableMigration(db))
	register(NewTenantColumnBatchMigration(db))
	register(NewTenantUniqueIndexMigration(db))
	register(NewTenantGlobalEmailUniqueMigration(db))
	register(NewAggregationWatermarkPKMigration(db))
	// v3.62.0：LLM 输入/输出分列计价与 provider 来源（模块化改造 · 任务五）
	register(NewLLMSeparatePricingMigration(db))
	// v3.63.0：tenants 静默合规采集三列（003 计费二期 T010）
	register(NewTenantInitComplianceColumnsMigration(db))
	// v3.64.0：订阅镜像表（003 计费二期 T022）
	register(NewTenantSubscriptionMirrorMigration(db))
	// v3.65.0：system_config_kv 租户分域（平台键迁至 platform_config_kv，主键复合化）
	register(NewPlatformKVTenantSplitMigration(db))
	// v3.66.0：站点配置一租户一行；功能开关唯一键降为租户内
	register(NewTenantSiteConfigAndFeatureFlagMigration(db))
	// v3.67.0：摘除浏览器定时触发器的软删残留行（它们占着 (tenant_id, task_id) 唯一键位，
	// 让同一任务再也挂不上新触发器）
	register(NewBrowserCronTriggerTombstonePruneMigration(db))
	// v3.68.0：A/B 实验结果唯一键从 (tenant_id, experiment_id) 恢复为
	// (tenant_id, experiment_id, variant_id)——单列唯一会让多变异实验算不出第二条结果
	register(NewABExperimentResultUniqueKeyMigration(db))
	// v3.69.0：bandit_arms 唯一键从 (tenant_id, arm_key) 恢复为
	// (tenant_id, experiment_id, arm_key)——arm_key 只在实验内唯一
	register(NewBanditArmUniqueKeyMigration(db))
	// v3.70.0：channel_agent_bindings 唯一键加入 chat_id 维度——同一账号下的
	// telegram 群组要能各自建绑定，而 account 级默认仍只允许一条
	register(NewChannelBindingChatIDUniqueKeyMigration(db))
	// v3.71.0：geo_daily_stats 唯一键 (tenant_id, stat_date, engine, intent)——
	// 模型原标签建出的是非唯一索引，ON CONFLICT 在全新库上 42P10
	register(NewGeoDailyStatUniqueKeyMigration(db))
	// v3.72.0：geo_entity_relations 唯一键 (tenant_id, entity_a_id, entity_b_id, relation)
	// ——先按该键去重再建，修复并发抽取写重复关系边；顺带删除 source_doc_id 僵尸列
	register(NewGeoEntityRelationUniqueKeyMigration(db))
	// v3.73.0：geo_index_trackings 唯一键 (tenant_id, article_id, engine)——先按该键
	// 去重再建，保证验证结果幂等写入（原「先 UPDATE 再 INSERT」并发写重复行）
	register(NewGeoIndexTrackingUniqueKeyMigration(db))
}
