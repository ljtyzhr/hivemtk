package repository

import (
	"context"
	"errors"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// WhatsappTemplateRepository WhatsApp 消息模板仓库
type WhatsappTemplateRepository struct {
	db *gorm.DB
}

// NewWhatsappTemplateRepository 创建 WhatsApp 消息模板仓库
func NewWhatsappTemplateRepository() *WhatsappTemplateRepository {
	return &WhatsappTemplateRepository{db: _db.GetDB()}
}

// NewWhatsappTemplateRepositoryWithDB 创建指定数据库连接的实例（用于测试）
func NewWhatsappTemplateRepositoryWithDB(db *gorm.DB) *WhatsappTemplateRepository {
	return &WhatsappTemplateRepository{db: db}
}

// SetDB 注入 db（用于测试）
func (r *WhatsappTemplateRepository) SetDB(ctx context.Context, db *gorm.DB) {
	if db != nil {
		r.db = db
	}
}

// Create 创建模板
func (r *WhatsappTemplateRepository) Create(ctx context.Context, template *model.WhatsappMessageTemplate) error {
	return r.db.WithContext(ctx).Create(template).Error
}

// Save 整行回写模板。走 db.SaveWholeRow：裸 Save 命中 0 行会回退成 INSERT，
// 把「编辑页读出后、保存前被删掉」的模板按原 id 插回。
func (r *WhatsappTemplateRepository) Save(ctx context.Context, template *model.WhatsappMessageTemplate) error {
	if template == nil || template.ID == "" {
		return errors.New("whatsapp template: 缺主键，不能整行回写")
	}
	return _db.SaveWholeRow(r.db.WithContext(ctx), template)
}

// GetByID 按 ID 获取模板
func (r *WhatsappTemplateRepository) GetByID(ctx context.Context, id string) (*model.WhatsappMessageTemplate, error) {
	var template model.WhatsappMessageTemplate
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&template).Error
	if err != nil {
		return nil, err
	}
	return &template, nil
}

// ListByFilters 按条件列出模板（limit 夹取 1..200：模板表是配置表，
// 行数可控，但无界 Find 的习惯不能留）。
func (r *WhatsappTemplateRepository) ListByFilters(ctx context.Context, category string, isActive *bool) ([]*model.WhatsappMessageTemplate, error) {
	var templates []*model.WhatsappMessageTemplate
	query := r.db.WithContext(ctx).Model(&model.WhatsappMessageTemplate{}).Limit(200)
	if category != "" {
		query = query.Where("category = ?", category)
	}
	if isActive != nil {
		query = query.Where("is_active = ?", *isActive)
	}
	if err := query.Order("created_at DESC").Find(&templates).Error; err != nil {
		return nil, err
	}
	return templates, nil
}

// DeleteByID 按 ID 删除模板，返回受影响行数
func (r *WhatsappTemplateRepository) DeleteByID(ctx context.Context, id string) (int64, error) {
	result := r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.WhatsappMessageTemplate{})
	return result.RowsAffected, result.Error
}
