package repository

import (
	"context"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// AIToolAccountBindingRepository AI工具-账号绑定仓储
type AIToolAccountBindingRepository struct {
	db *gorm.DB
}

// NewAIToolAccountBindingRepository 创建AI工具-账号绑定仓储
func NewAIToolAccountBindingRepository(db *gorm.DB) *AIToolAccountBindingRepository {
	return &AIToolAccountBindingRepository{db: db}
}

// ListByTool 获取工具绑定的账号列表
func (r *AIToolAccountBindingRepository) ListByTool(ctx context.Context, toolName string) ([]model.AIToolAccountBinding, error) {
	var bindings []model.AIToolAccountBinding
	err := r.db.WithContext(ctx).Where("tool_name = ?", toolName).Find(&bindings).Error
	return bindings, err
}

// Bind 原子创建绑定。
//
// 主账号切换必须与写入同一事务：先清同工具同类型的主标记、再建行；
// 若分开执行，重复绑定撞唯一索引失败时会把原来的主账号清成"无主"半截状态。
func (r *AIToolAccountBindingRepository) Bind(ctx context.Context, binding *model.AIToolAccountBinding) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if binding.IsPrimary {
			if err := tx.Model(&model.AIToolAccountBinding{}).
				Where("tool_name = ? AND account_type = ?", binding.ToolName, binding.AccountType).
				Update("is_primary", false).Error; err != nil {
				return err
			}
		}
		return tx.Create(binding).Error
	})
}

// Delete 删除绑定关系
func (r *AIToolAccountBindingRepository) Delete(ctx context.Context, toolName, accountType, accountID string) error {
	res := r.db.WithContext(ctx).
		Where("tool_name = ? AND account_type = ? AND account_id = ?", toolName, accountType, accountID).
		Delete(&model.AIToolAccountBinding{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		// 跨租户与真不存在在租户谓词之前读数相同，静默成功会让界面以为已解绑
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ListByTools 批量获取多个工具的账号绑定（一次 IN 查询替代 N 次 ListByTool，消除列表页 N+1）。
func (r *AIToolAccountBindingRepository) ListByTools(ctx context.Context, toolNames []string) (map[string][]model.AIToolAccountBinding, error) {
	out := make(map[string][]model.AIToolAccountBinding, len(toolNames))
	if len(toolNames) == 0 {
		return out, nil
	}
	var bindings []model.AIToolAccountBinding
	if err := r.db.WithContext(ctx).Where("tool_name IN ?", toolNames).Find(&bindings).Error; err != nil {
		return nil, err
	}
	for _, b := range bindings {
		out[b.ToolName] = append(out[b.ToolName], b)
	}
	return out, nil
}
