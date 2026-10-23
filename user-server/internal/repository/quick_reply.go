package repository

import (
	"context"
	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// QuickReplyRepository 快捷回复仓库
type QuickReplyRepository struct {
	db *gorm.DB
}

// NewQuickReplyRepository 创建快捷回复仓库实例
func NewQuickReplyRepository() *QuickReplyRepository {
	return &QuickReplyRepository{
		db: _db.GetDB(),
	}
}

// Create 创建快捷回复
func (r *QuickReplyRepository) Create(ctx context.Context, reply *model.QuickReply) error {
	return r.db.WithContext(ctx).Create(reply).Error
}

// Update 整行回写一条快捷回复。
//
// 走 `db.SaveWholeRow`：裸 Save 命中 0 行时会回退成 upsert，等于把已被删掉的那条
// 复活成一条新行（还带着旧 id），接口却回 200。
func (r *QuickReplyRepository) Update(ctx context.Context, reply *model.QuickReply) error {
	return _db.SaveWholeRow(r.db.WithContext(ctx), reply)
}

// Delete 删除快捷回复；0 行 ⇒ 该条不存在（`db.EnsureAffected`），不是"删成功"。
func (r *QuickReplyRepository) Delete(ctx context.Context, id uint) error {
	return _db.EnsureAffected(r.db.WithContext(ctx).Delete(&model.QuickReply{}, id))
}

// GetByID 根据ID获取快捷回复
func (r *QuickReplyRepository) GetByID(ctx context.Context, id uint) (*model.QuickReply, error) {
	var reply model.QuickReply
	err := r.db.WithContext(ctx).First(&reply, id).Error
	if err != nil {
		return nil, err
	}
	return &reply, nil
}

func (r *QuickReplyRepository) GetByMerchant(ctx context.Context, category string) ([]*model.QuickReply, error) {
	var replies []*model.QuickReply
	query := r.db.WithContext(ctx).Where("is_public = ?", true)
	if category != "" {
		query = query.Where("category = ?", category)
	}
	err := query.Order("sort_order ASC").Find(&replies).Error
	return replies, err
}

// GetCategories 获取快捷回复分类列表
func (r *QuickReplyRepository) GetCategories(ctx context.Context) ([]string, error) {
	var categories []string
	err := r.db.WithContext(ctx).Model(&model.QuickReply{}).
		Where("is_public = ?", true).
		Distinct("category").Pluck("category", &categories).Error
	return categories, err
}
