package migrations

import (
	"context"
	"fmt"

	"hivemtk-user/internal/config"
	"hivemtk-user/internal/migration"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/utils/logger"

	"gorm.io/gorm"
)

// AIAgentSchemaMigration 多 AI 智能体架构迁移
type AIAgentSchemaMigration struct {
	db *gorm.DB
}

// NewAIAgentSchemaMigration 创建迁移实例
func NewAIAgentSchemaMigration(db *gorm.DB) *AIAgentSchemaMigration {
	return &AIAgentSchemaMigration{db: db}
}

// Version 返回版本号
func (m *AIAgentSchemaMigration) Version() string {
	return "v2.2.0"
}

// Name 返回迁移名称
func (m *AIAgentSchemaMigration) Name() string {
	return "多 AI 智能体架构"
}

// Description 返回迁移描述
func (m *AIAgentSchemaMigration) Description() string {
	return "创建 ai_agents / channel_agent_bindings / customer_service_agents 三张表，支持多 AI 智能体、渠道绑定、客服挂载"
}

// Up 执行迁移
func (m *AIAgentSchemaMigration) Up(ctx context.Context) error {
	if err := m.db.AutoMigrate(&model.AIAgent{}); err != nil {
		return fmt.Errorf("migrate ai_agents failed: %w", err)
	}

	if err := m.db.AutoMigrate(&model.ChannelAgentBinding{}); err != nil {
		return fmt.Errorf("migrate channel_agent_bindings failed: %w", err)
	}

	if err := m.db.AutoMigrate(&model.CustomerServiceAgent{}); err != nil {
		return fmt.Errorf("migrate customer_service_agents failed: %w", err)
	}

	var count int64
	// 种子写入挂 SystemScope（003 T014 复核）：迁移在 on 模式跑时无租户绑定，
	// 裸查/裸写 ai_agents 会被隔离 callback fail-closed（count 恒 0、Create 报错
	// 仅 Warn）——fresh on 安装从此没有 default_sales 种子，T013 的租户级克隆
	// 也就无源可克。SystemScope 是系统域写入的豁免通道；行上显式 TenantID=1。
	sysCtx := tenant.WithSystemScope(ctx)
	if err := m.db.WithContext(sysCtx).Model(&model.AIAgent{}).Count(&count).Error; err != nil {
		logger.Errorf("[migration] count default ai_agent warning: %v", err)
	}
	if count == 0 {
		defaultAgent := &model.AIAgent{
			TenantID:    1,
			AgentCode:   "default_sales",
			Name:        "默认销售智能体",
			Description: "系统初始化时创建的默认销售智能体，可编辑或删除",
			AgentType:   string(model.AgentTypeSales),
			Persona:     "你是一位资深销售专家，擅长用温和、专业的语气帮助客户解决问题。回复简洁、亲切、不超过 80 字。",
			LLMModel:    config.DefaultLLMModel(),
			Status:      1,
		}
		if err := m.db.WithContext(sysCtx).Create(defaultAgent).Error; err != nil {
			logger.Errorf("[migration] create default ai_agent warning: %v", err)
		}
	}

	return nil
}

// Down 回滚
func (m *AIAgentSchemaMigration) Down(ctx context.Context) error {
	declineTableDrop(m.Version(), "customer_service_agents", "channel_agent_bindings", "ai_agents")
	return nil
}

var _ migration.Migration = (*AIAgentSchemaMigration)(nil)
