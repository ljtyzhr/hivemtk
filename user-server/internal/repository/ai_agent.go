package repository

import (
	"context"
	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

const (
	// aiAgentListLimit 智能体列表上限：管理端下拉/列表全量拉取，封顶防止无界扫描。
	aiAgentListLimit = 500
	// channelBindingListLimit 单渠道账号/单智能体的绑定行数上限：业务上远小于该值。
	channelBindingListLimit = 500
	// csAgentMountListLimit 单座席/单智能体的挂载行数上限。
	csAgentMountListLimit = 500
)

// AIAgentRepository AI 智能体仓库
type AIAgentRepository struct {
	db *gorm.DB
}

// NewAIAgentRepository 创建智能体仓库（绑定全局默认 DB，仅兼容存量测试）
// 生产装配一律用 NewAIAgentRepositoryWithDB 显式注入句柄。
func NewAIAgentRepository() *AIAgentRepository {
	return &AIAgentRepository{db: GetDB()}
}

// NewAIAgentRepositoryWithDB 创建指定数据库连接的智能体仓库
func NewAIAgentRepositoryWithDB(db *gorm.DB) *AIAgentRepository {
	return &AIAgentRepository{db: db}
}

// Create 创建智能体
func (r *AIAgentRepository) Create(ctx context.Context, agent *model.AIAgent) error {
	return r.db.WithContext(ctx).Create(agent).Error
}

// CountChannelBindingsByAgent 统计智能体被渠道账号绑定的数量
func (r *AIAgentRepository) CountChannelBindingsByAgent(ctx context.Context, agentID uint) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.ChannelAgentBinding{}).
		Where("agent_id = ?", agentID).Count(&n).Error
	return n, err
}

// CountServiceMountsByAgent 统计智能体被客服座席挂载的数量
func (r *AIAgentRepository) CountServiceMountsByAgent(ctx context.Context, agentID uint) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.CustomerServiceAgent{}).
		Where("ai_agent_id = ?", agentID).Count(&n).Error
	return n, err
}

// GetByID 根据 ID 获取
func (r *AIAgentRepository) GetByID(ctx context.Context, id uint) (*model.AIAgent, error) {
	var a model.AIAgent
	if err := r.db.WithContext(ctx).First(&a, id).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

// GetByCode 根据 agent_code 获取
func (r *AIAgentRepository) GetByCode(ctx context.Context, code string) (*model.AIAgent, error) {
	var a model.AIAgent
	if err := r.db.WithContext(ctx).Where("agent_code = ?", code).First(&a).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

// List 列表查询（支持类型/状态/关键字筛选）
func (r *AIAgentRepository) List(ctx context.Context, agentType string, status int, keyword string) ([]*model.AIAgent, error) {
	q := r.db.WithContext(ctx).Model(&model.AIAgent{})
	if agentType != "" {
		q = q.Where("agent_type = ?", agentType)
	}
	if status >= 0 {
		q = q.Where("status = ?", status)
	}
	if keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where("name LIKE ? OR agent_code LIKE ? OR description LIKE ?", like, like, like)
	}
	var list []*model.AIAgent
	if err := q.Order("id DESC").Limit(aiAgentListLimit).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// ListEnabled 获取所有启用的智能体
func (r *AIAgentRepository) ListEnabled(ctx context.Context) ([]*model.AIAgent, error) {
	var list []*model.AIAgent
	if err := r.db.WithContext(ctx).Where("status = ?", 1).Order("id DESC").
		Limit(aiAgentListLimit).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// Update 更新智能体
//
// 不用 Save：Save 的 UPDATE 命中 0 行时会回退成 ON CONFLICT(裸主键) DO UPDATE，
// 而租户闸对 excluded.tenant_id 是放行的 —— 跨租户按主键 Save 会把整行搬到
// 调用方租户下（#14 真库探针复现）。这里用显式列 map：谓词只剩 WHERE id +
// 注入的 tenant_id，0 行就是 0 行；同时 map 不做零值裁剪，显式 false/0 能落库。
// 四个 *bool 能力开关为 nil 时跳过该列（保留原值），不解引用、不写 NULL。
func (r *AIAgentRepository) Update(ctx context.Context, agent *model.AIAgent) error {
	updates := map[string]any{
		"agent_code": agent.AgentCode, "name": agent.Name, "description": agent.Description,
		"avatar": agent.Avatar, "agent_type": agent.AgentType, "agent_mode": agent.AgentMode,
		"persona": agent.Persona, "system_prompt": agent.SystemPrompt, "greeting": agent.Greeting,
		"rag_product_ids": agent.RagProductIDs, "faq_entry_ids": agent.FAQEntryIDs,
		"sop_template_ids": agent.SOPTemplateIDs, "sop_ids": agent.SOPIDs,
		"script_library_ids": agent.ScriptLibraryIDs, "ab_experiment_ids": agent.ABExperimentIDs,
		"asset_bundle_id": agent.AssetBundleID,
		"llm_model":       agent.LLMModel, "llm_api_key": agent.LLMProviderConfig.APIKey,
		"llm_base_url": agent.LLMProviderConfig.BaseURL, "llm_api_type": agent.LLMProviderConfig.APIType,
		"llm_model_detail": agent.LLMProviderConfig.Model, "llm_max_retries": agent.LLMProviderConfig.MaxRetries,
		"llm_request_timeout": agent.LLMProviderConfig.RequestTimeout,
		"temperature":         agent.Temperature, "max_tokens": agent.MaxTokens, "top_p": agent.TopP,
		"frequency_penalty": agent.FrequencyPenalty, "presence_penalty": agent.PresencePenalty,
		"internal_language": agent.InternalLanguage, "target_language": agent.TargetLanguage,
		"enable_content_audit": agent.EnableContentAudit, "rag_top_k": agent.RAGTopK,
		"confidence_threshold": agent.ConfidenceThreshold, "max_ai_consecutive": agent.MaxAIConsecutive,
		"status": agent.Status, "version": agent.Version,
	}
	if agent.EnableRAG != nil {
		updates["enable_rag"] = *agent.EnableRAG
	}
	if agent.EnableScriptMatch != nil {
		updates["enable_script_match"] = *agent.EnableScriptMatch
	}
	if agent.EnableHumanizePolish != nil {
		updates["enable_humanize_polish"] = *agent.EnableHumanizePolish
	}
	if agent.EnablePlaybook != nil {
		updates["enable_playbook"] = *agent.EnablePlaybook
	}

	res := r.db.WithContext(ctx).Model(&model.AIAgent{}).
		Where("id = ?", agent.ID).
		Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// UpdateStatus 更新状态
func (r *AIAgentRepository) UpdateStatus(ctx context.Context, id uint, status int) error {
	result := r.db.WithContext(ctx).Model(&model.AIAgent{}).Where("id = ?", id).Update("status", status)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// Delete 删除智能体
// 注意：关联的 channel_agent_bindings 和 customer_service_agents 由数据库外键 ON DELETE CASCADE 自动级联删除
func (r *AIAgentRepository) Delete(ctx context.Context, id uint) error {
	result := r.db.WithContext(ctx).Delete(&model.AIAgent{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ChannelAgentBindingRepository 渠道绑定仓库
type ChannelAgentBindingRepository struct {
	db *gorm.DB
}

// NewChannelAgentBindingRepository 创建渠道绑定仓库（绑定全局默认 DB，仅兼容存量测试）
func NewChannelAgentBindingRepository() *ChannelAgentBindingRepository {
	return &ChannelAgentBindingRepository{db: GetDB()}
}

// NewChannelAgentBindingRepositoryWithDB 创建指定数据库连接的渠道绑定仓库
func NewChannelAgentBindingRepositoryWithDB(db *gorm.DB) *ChannelAgentBindingRepository {
	return &ChannelAgentBindingRepository{db: db}
}

// Create 创建绑定
//
// IsPrimary/Enabled 是 *bool：nil 落 DB 默认 true，显式 false 真写 false（GORM 对
// 非 nil 指针不做零值替换）。
func (r *ChannelAgentBindingRepository) Create(ctx context.Context, b *model.ChannelAgentBinding) error {
	return r.db.WithContext(ctx).Create(b).Error
}

// ReplacePrimaryBinding 事务替换渠道账号主绑定: 清除旧主绑定 + 创建新主绑定
//
// 任一步骤失败整体回滚; 与 channel_agent_bindings 的 idx_channel_binding_unique
// 唯一键 (tenant_id, channel_type, account_id, COALESCE(chat_id,”)) 配合,
// 即使并发也只有一条主绑定能落地。
func (r *ChannelAgentBindingRepository) ReplacePrimaryBinding(ctx context.Context, channelType, accountID string, agentID uint) (*model.ChannelAgentBinding, error) {
	var created *model.ChannelAgentBinding
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tmpRepo := NewChannelAgentBindingRepositoryWithDB(tx)
		if err := tmpRepo.ClearPrimaryByChannelAccount(ctx, channelType, accountID); err != nil {
			return err
		}
		primary, enabled := true, true
		b := &model.ChannelAgentBinding{
			ChannelType: channelType,
			AccountID:   accountID,
			AgentID:     agentID,
			IsPrimary:   &primary,
			Enabled:     &enabled,
		}
		if err := tmpRepo.Create(ctx, b); err != nil {
			return err
		}
		created = b
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// GetByID 根据 ID 获取
func (r *ChannelAgentBindingRepository) GetByID(ctx context.Context, id uint) (*model.ChannelAgentBinding, error) {
	var b model.ChannelAgentBinding
	if err := r.db.WithContext(ctx).First(&b, id).Error; err != nil {
		return nil, err
	}
	return &b, nil
}

// ListByChannelAccount 按渠道+账号ID查询所有绑定
func (r *ChannelAgentBindingRepository) ListByChannelAccount(ctx context.Context, channelType, accountID string) ([]*model.ChannelAgentBinding, error) {
	var list []*model.ChannelAgentBinding
	if err := r.db.WithContext(ctx).Where("channel_type = ? AND account_id = ?", channelType, accountID).
		Order("is_primary DESC, id DESC").Limit(channelBindingListLimit).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// GetPrimaryByChannelAccount 按渠道+账号ID查询主绑定
// 返回 nil, nil 表示未绑定（不视为错误）
func (r *ChannelAgentBindingRepository) GetPrimaryByChannelAccount(ctx context.Context, channelType, accountID string) (*model.ChannelAgentBinding, error) {
	var b model.ChannelAgentBinding
	err := r.db.WithContext(ctx).Where("channel_type = ? AND account_id = ? AND is_primary = ? AND enabled = ?",
		channelType, accountID, true, true).First(&b).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &b, nil
}

// ListByAgentID 反查智能体被哪些渠道账号使用
func (r *ChannelAgentBindingRepository) ListByAgentID(ctx context.Context, agentID uint) ([]*model.ChannelAgentBinding, error) {
	var list []*model.ChannelAgentBinding
	if err := r.db.WithContext(ctx).Where("agent_id = ?", agentID).Order("id DESC").
		Limit(channelBindingListLimit).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// ResolveBinding 按优先级路由查找渠道+账号+会话的最佳绑定
//
// 匹配优先级（从高到低）：
//  1. chat_id 精确匹配  + priority DESC（最具体）
//  2. chat_id IS NULL（account 级默认）+ priority DESC
//  3. is_primary=true 兜底
//
// 返回 nil, nil 表示未绑定
//
// 实现策略：两次参数化查询（先精确后默认），避免 OR 条件导致 Seq Scan + 消除 SQL 注入风险。
func (r *ChannelAgentBindingRepository) ResolveBinding(ctx context.Context, channelType, accountID, chatID string) (*model.ChannelAgentBinding, error) {
	// Step 1: chat_id 精确匹配 + 高 priority
	if chatID != "" {
		var b model.ChannelAgentBinding
		err := r.db.WithContext(ctx).
			Where("channel_type = ? AND account_id = ? AND chat_id = ? AND enabled = ?",
				channelType, accountID, chatID, true).
			Order("priority DESC").
			Order("is_primary DESC").
			Order("id DESC").
			Limit(1).
			First(&b).Error
		if err == nil {
			return &b, nil
		}
		if err != gorm.ErrRecordNotFound {
			return nil, err
		}
	}

	// Step 2: chat_id IS NULL 的 account 级默认绑定
	{
		var b model.ChannelAgentBinding
		err := r.db.WithContext(ctx).
			Where("channel_type = ? AND account_id = ? AND chat_id IS NULL AND enabled = ?",
				channelType, accountID, true).
			Order("priority DESC").
			Order("is_primary DESC").
			Order("id DESC").
			Limit(1).
			First(&b).Error
		if err == nil {
			return &b, nil
		}
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
}

// GetBindingByChatID 按渠道+账号+chat_id 精确查询（不含默认绑定）
func (r *ChannelAgentBindingRepository) GetBindingByChatID(ctx context.Context, channelType, accountID, chatID string) (*model.ChannelAgentBinding, error) {
	var b model.ChannelAgentBinding
	err := r.db.WithContext(ctx).Where(
		"channel_type = ? AND account_id = ? AND chat_id = ? AND enabled = ?",
		channelType, accountID, chatID, true,
	).First(&b).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &b, nil
}

// Update 更新绑定
//
// 不用 Save（0 行命中会退化成按裸主键 ON CONFLICT，跨租户能把整行搬走）；用列 map
// 保留零值语义：is_primary/enabled 为 nil 时跳过（保留原值），显式 false 落库。
func (r *ChannelAgentBindingRepository) Update(ctx context.Context, b *model.ChannelAgentBinding) error {
	updates := map[string]any{
		"channel_type": b.ChannelType,
		"account_id":   b.AccountID,
		"chat_id":      b.ChatID,
		"chat_type":    b.ChatType,
		"priority":     b.Priority,
		"agent_id":     b.AgentID,
	}
	if b.IsPrimary != nil {
		updates["is_primary"] = *b.IsPrimary
	}
	if b.Enabled != nil {
		updates["enabled"] = *b.Enabled
	}
	res := r.db.WithContext(ctx).Model(&model.ChannelAgentBinding{}).
		Where("id = ?", b.ID).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// Delete 删除绑定
func (r *ChannelAgentBindingRepository) Delete(ctx context.Context, id uint) error {
	res := r.db.WithContext(ctx).Delete(&model.ChannelAgentBinding{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ClearPrimaryByChannelAccount 清除指定渠道账号下的所有主绑定标记
// 用于切换主智能体时先清空再设置
func (r *ChannelAgentBindingRepository) ClearPrimaryByChannelAccount(ctx context.Context, channelType, accountID string) error {
	return r.db.WithContext(ctx).Model(&model.ChannelAgentBinding{}).
		Where("channel_type = ? AND account_id = ?", channelType, accountID).
		Update("is_primary", false).Error
}

// DeleteByChannelAccount 删除指定渠道账号下的所有绑定
func (r *ChannelAgentBindingRepository) DeleteByChannelAccount(ctx context.Context, channelType, accountID string) error {
	return r.db.WithContext(ctx).Where("channel_type = ? AND account_id = ?", channelType, accountID).
		Delete(&model.ChannelAgentBinding{}).Error
}

// CustomerServiceAgentRepository 客服挂载仓库
type CustomerServiceAgentRepository struct {
	db *gorm.DB
}

// NewCustomerServiceAgentRepository 创建客服挂载仓库（绑定全局默认 DB，仅兼容存量测试）
func NewCustomerServiceAgentRepository() *CustomerServiceAgentRepository {
	return &CustomerServiceAgentRepository{db: GetDB()}
}

// NewCustomerServiceAgentRepositoryWithDB 创建指定数据库连接的客服挂载仓库
func NewCustomerServiceAgentRepositoryWithDB(db *gorm.DB) *CustomerServiceAgentRepository {
	return &CustomerServiceAgentRepository{db: db}
}

// Create 创建挂载
func (r *CustomerServiceAgentRepository) Create(ctx context.Context, c *model.CustomerServiceAgent) error {
	return r.db.WithContext(ctx).Create(c).Error
}

// GetByID 根据 ID 获取
func (r *CustomerServiceAgentRepository) GetByID(ctx context.Context, id uint) (*model.CustomerServiceAgent, error) {
	var c model.CustomerServiceAgent
	if err := r.db.WithContext(ctx).First(&c, id).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// ListByAgentStatusID 按客服座席ID查询所有挂载
func (r *CustomerServiceAgentRepository) ListByAgentStatusID(ctx context.Context, agentStatusID uint) ([]*model.CustomerServiceAgent, error) {
	var list []*model.CustomerServiceAgent
	if err := r.db.WithContext(ctx).Where("agent_status_id = ?", agentStatusID).
		Order("is_primary DESC, id DESC").Limit(csAgentMountListLimit).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// GetPrimaryByAgentStatusID 按座席ID查询主挂载
// 返回 nil, nil 表示未挂载
func (r *CustomerServiceAgentRepository) GetPrimaryByAgentStatusID(ctx context.Context, agentStatusID uint) (*model.CustomerServiceAgent, error) {
	var c model.CustomerServiceAgent
	err := r.db.WithContext(ctx).Where("agent_status_id = ? AND is_primary = ? AND enabled = ?",
		agentStatusID, true, true).First(&c).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

// ListByAIAgentID 反查智能体被哪些客服使用
func (r *CustomerServiceAgentRepository) ListByAIAgentID(ctx context.Context, aiAgentID uint) ([]*model.CustomerServiceAgent, error) {
	var list []*model.CustomerServiceAgent
	if err := r.db.WithContext(ctx).Where("ai_agent_id = ?", aiAgentID).Order("id DESC").
		Limit(csAgentMountListLimit).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// Update 更新挂载（列 map：nil 的 is_primary/enabled 保留原值，显式 false 落库）
func (r *CustomerServiceAgentRepository) Update(ctx context.Context, c *model.CustomerServiceAgent) error {
	updates := map[string]any{
		"agent_status_id": c.AgentStatusID,
		"ai_agent_id":     c.AIAgentID,
	}
	if c.IsPrimary != nil {
		updates["is_primary"] = *c.IsPrimary
	}
	if c.Enabled != nil {
		updates["enabled"] = *c.Enabled
	}
	res := r.db.WithContext(ctx).Model(&model.CustomerServiceAgent{}).
		Where("id = ?", c.ID).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// Delete 删除挂载
func (r *CustomerServiceAgentRepository) Delete(ctx context.Context, id uint) error {
	res := r.db.WithContext(ctx).Delete(&model.CustomerServiceAgent{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ClearPrimaryByAgentStatusID 清除指定座席下所有主挂载标记
func (r *CustomerServiceAgentRepository) ClearPrimaryByAgentStatusID(ctx context.Context, agentStatusID uint) error {
	return r.db.WithContext(ctx).Model(&model.CustomerServiceAgent{}).
		Where("agent_status_id = ?", agentStatusID).
		Update("is_primary", false).Error
}

// DeleteByAgentStatusID 删除指定座席下的所有挂载
func (r *CustomerServiceAgentRepository) DeleteByAgentStatusID(ctx context.Context, agentStatusID uint) error {
	return r.db.WithContext(ctx).Where("agent_status_id = ?", agentStatusID).
		Delete(&model.CustomerServiceAgent{}).Error
}
