package service

import (
	"context"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/utils/logger"

	"github.com/lib/pq"
	"gorm.io/gorm"
)

// 开户赠送 provisioning（003 计费二期 T013）
//
// 需求：新商户注册即得「1 个租户级智能体 + 1 个起步资产包」，on/off 两态都做。
//
// 为什么克隆而不是改归属：default_sales 是全局种子行（tenant_id=1），
// 直接改它的 tenant_id 会把存量默认租户的智能体抢走 —— 只能克隆副本。
//
// 幂等键 = tenant_id + template_key：
//   - 智能体：(tenant_id, agent_code=default_sales) —— agent_code 唯一索引
//     经 v3.59 规范化后是 (tenant_id, agent_code)，跨租户各有一份合法；
//   - 资产包：(tenant_id, asset_id=starter-bundle) —— 同理。
//
// 写入口径：WithSystemScope + 行上显式 TenantID。注册流程的 ctx 没有租户
// 绑定（public 组），裸写会撞 fillCreateForTenant 的 fail-closed；SystemScope
// 是系统域写入的豁免通道，显式 TenantID 保证行归属正确（SystemScope 下
// callback 不填列，列值以 struct 为准）。
//
// 失败语义：**仅 Warn，绝不阻断注册** —— 赠送可以后补（重跑脚本/管理端），
// 把商户挡在开户门外不可逆。调用方无需检查返回值。

const (
	starterKitAgentCode   = "default_sales"
	starterKitTemplateKey = "starter-bundle"
)

// provisionTenantStarterKit 为新租户克隆起步智能体并创建内置起步资产包（幂等）。
// database 由调用方经仓储 GetDB 传入：service 禁止全局摸 db.GetDB（架构门 L4）。
func provisionTenantStarterKit(ctx context.Context, tenantID uint, database *gorm.DB) {
	if tenantID == 0 {
		return
	}
	if database == nil {
		logger.Warnf("[starter-kit] tenant=%d 无可用数据库句柄，跳过赠送", tenantID)
		return
	}
	// 注册/初始化 ctx 无租户绑定：走系统域豁免通道，行上显式 TenantID 定归属
	sysCtx := tenant.WithSystemScope(ctx)

	provisionStarterAgent(sysCtx, database, tenantID)
	provisionStarterBundle(sysCtx, database, tenantID)
}

// provisionStarterAgent 克隆全局 default_sales 为租户级副本
func provisionStarterAgent(sysCtx context.Context, db *gorm.DB, tenantID uint) {
	var existing int64
	if err := db.WithContext(sysCtx).Model(&model.AIAgent{}).
		Where("tenant_id = ? AND agent_code = ?", tenantID, starterKitAgentCode).
		Count(&existing).Error; err != nil {
		logger.Warnf("[starter-kit] tenant=%d 智能体幂等检查失败（跳过克隆，不阻断注册）: %v", tenantID, err)
		return
	}
	if existing > 0 {
		return // 已赠过：幂等键命中
	}

	// 全局种子行（tenant_id=1，migration v2.2.0 建）；找不到就 Warn 跳过——
	// 种子可后补，开户不能被一条 Warn 拖死
	var source model.AIAgent
	if err := db.WithContext(sysCtx).
		Where("tenant_id = ? AND agent_code = ?", uint(1), starterKitAgentCode).
		First(&source).Error; err != nil {
		logger.Warnf("[starter-kit] tenant=%d 未找到全局 %s 种子行（跳过克隆）: %v",
			tenantID, starterKitAgentCode, err)
		return
	}

	clone := source
	clone.ID = 0
	clone.TenantID = tenantID
	clone.CreatedAt = time.Time{}
	clone.UpdatedAt = time.Time{}
	clone.DeletedAt = gorm.DeletedAt{}
	clone.Name = "默认销售智能体"
	// ID 数组是种子租户的绑定，副本不能继承（否则跨租户引用种子行的资产）
	clone.RagProductIDs = pq.StringArray{}
	clone.FAQEntryIDs = pq.StringArray{}
	clone.SOPTemplateIDs = pq.StringArray{}
	clone.SOPIDs = pq.StringArray{}
	clone.ScriptLibraryIDs = pq.StringArray{}
	clone.ABExperimentIDs = pq.StringArray{}

	if err := db.WithContext(sysCtx).Create(&clone).Error; err != nil {
		logger.Warnf("[starter-kit] tenant=%d 智能体克隆失败（不阻断注册）: %v", tenantID, err)
		return
	}
	logger.Infof("[starter-kit] tenant=%d 已克隆起步智能体（agent_id=%d, code=%s）",
		tenantID, clone.ID, clone.AgentCode)
}

// provisionStarterBundle 创建内置起步资产包（欢迎语/常见问答/话术）
func provisionStarterBundle(sysCtx context.Context, db *gorm.DB, tenantID uint) {
	var existing int64
	if err := db.WithContext(sysCtx).Model(&model.AssetBundle{}).
		Where("tenant_id = ? AND asset_id = ?", tenantID, starterKitTemplateKey).
		Count(&existing).Error; err != nil {
		logger.Warnf("[starter-kit] tenant=%d 资产包幂等检查失败（跳过创建，不阻断注册）: %v", tenantID, err)
		return
	}
	if existing > 0 {
		return
	}

	bundle := &model.AssetBundle{
		TenantID:           tenantID,
		AssetID:            starterKitTemplateKey,
		Title:              "起步资产包",
		Description:        "开户赠送的内置起步模板：欢迎语 + 常见问答 + 销售话术，可直接挂载到智能体使用",
		Author:             "system",
		Version:            "1.0.0",
		Scope:              model.AssetBundleScopePrivate,
		Status:             model.AssetBundleStatusActive,
		Language:           "zh",
		Tags:               pq.StringArray{"starter", "内置赠送"},
		SupportedLanguages: pq.StringArray{"zh"},
		Messages: model.AssetBundleMessages{
			// 欢迎语
			{Role: "system", Content: "欢迎语：您好，我是您的智能客服，很高兴为您服务。请问有什么可以帮您？"},
			{Role: "system", Content: "服务承诺：我们将尽快回复您的咨询；如遇非工作时间，会在下一个工作日内优先处理您的留言。"},
			// 常见问答
			{Role: "user", Content: "你们是做什么的？"},
			{Role: "assistant", Content: "我们是一家专注于为客户解决问题的服务型企业，详细业务介绍可以参考我们的官网或直接咨询人工客服。"},
			{Role: "user", Content: "怎么收费？"},
			{Role: "assistant", Content: "具体收费取决于您选择的服务方案，我可以帮您转接人工客服获取详细报价。"},
			{Role: "user", Content: "人工客服什么时候在线？"},
			{Role: "assistant", Content: "人工客服的工作时间请以官网公示为准；非工作时间您可以留言，我们会尽快回复。"},
			// 销售话术
			{Role: "system", Content: "销售话术：先倾听客户诉求，再针对性介绍价值点；不夸大承诺，报价与交付细节引导至人工确认。"},
		},
		Examples: model.JSONArray{},
	}
	if err := db.WithContext(sysCtx).Create(bundle).Error; err != nil {
		logger.Warnf("[starter-kit] tenant=%d 起步资产包创建失败（不阻断注册）: %v", tenantID, err)
		return
	}
	logger.Infof("[starter-kit] tenant=%d 已创建起步资产包（asset_id=%s）", tenantID, starterKitTemplateKey)
}
