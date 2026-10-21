package repository

import (
	"context"
	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"
	"time"

	"gorm.io/gorm"
)

// AISuggestionRepository AI建议仓库
type AISuggestionRepository struct {
	db *gorm.DB
}

// NewAISuggestionRepository 创建AI建议仓库实例
func NewAISuggestionRepository() *AISuggestionRepository {
	return &AISuggestionRepository{
		db: _db.GetDB(),
	}
}

// NewAISuggestionRepositoryWithDB 创建指定数据库连接AI建议仓库实例
func NewAISuggestionRepositoryWithDB(db *gorm.DB) *AISuggestionRepository {
	return &AISuggestionRepository{db: db}
}

// Create 创建AI建议
func (r *AISuggestionRepository) Create(ctx context.Context, suggestion *model.AISuggestion) error {
	return r.db.WithContext(ctx).Create(suggestion).Error
}

// GetBySessionID 获取会话的AI建议列表
func (r *AISuggestionRepository) GetBySessionID(ctx context.Context, sessionID string) ([]*model.AISuggestion, error) {
	var suggestions []*model.AISuggestion
	err := r.db.WithContext(ctx).Where("session_id = ?", sessionID).Order("created_at DESC").Limit(10).Find(&suggestions).Error
	return suggestions, err
}

// MarkAsUsed 标记建议已使用
//
// 命中 0 行必须报错：GORM 对 0 行 UPDATE 只回 Error==nil，而「不属于本租户」
// 与「id 不存在」在 after-clauses 前是完全相同的读数——静默成功会让界面显示
// 「已采用」，库里一个字节都没变。
func (r *AISuggestionRepository) MarkAsUsed(ctx context.Context, id uint, agentID uint) error {
	now := time.Now()
	res := r.db.WithContext(ctx).Model(&model.AISuggestion{}).Where("id = ?", id).Updates(map[string]any{
		"is_used": true,
		"used_by": agentID,
		"used_at": &now,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
