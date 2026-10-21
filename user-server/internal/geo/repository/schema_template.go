package repository

import (
	"context"
	"time"

	"hivemtk-user/internal/geo/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// GeoSchemaTemplateRepository Schema 模板库仓储
type GeoSchemaTemplateRepository interface {
	List(ctx context.Context, page, limit int) ([]*model.GeoSchemaTemplate, int64, error)
	GetByID(ctx context.Context, id string) (*model.GeoSchemaTemplate, error)
	Create(ctx context.Context, tpl *model.GeoSchemaTemplate) error
	Update(ctx context.Context, tpl *model.GeoSchemaTemplate) error
	Delete(ctx context.Context, id string) error
}

type geoSchemaTemplateRepo struct{ db *gorm.DB }

func NewGeoSchemaTemplateRepository() GeoSchemaTemplateRepository {
	return &geoSchemaTemplateRepo{db: _db.GetDB()}
}

// NewGeoSchemaTemplateRepositoryWithDB 创建指定数据库连接的实例（用于测试）
func NewGeoSchemaTemplateRepositoryWithDB(db *gorm.DB) GeoSchemaTemplateRepository {
	return &geoSchemaTemplateRepo{db: db}
}

func (r *geoSchemaTemplateRepo) List(ctx context.Context, page, limit int) ([]*model.GeoSchemaTemplate, int64, error) {
	var list []*model.GeoSchemaTemplate
	var total int64
	q := r.db.WithContext(ctx).Model(&model.GeoSchemaTemplate{})
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Order("page_type ASC, schema_type ASC").
		Offset((page - 1) * limit).Limit(limit).Find(&list).Error
	return list, total, err
}

func (r *geoSchemaTemplateRepo) GetByID(ctx context.Context, id string) (*model.GeoSchemaTemplate, error) {
	var tpl model.GeoSchemaTemplate
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&tpl).Error; err != nil {
		return nil, err
	}
	return &tpl, nil
}

func (r *geoSchemaTemplateRepo) Create(ctx context.Context, tpl *model.GeoSchemaTemplate) error {
	return r.db.WithContext(ctx).Create(tpl).Error
}

// Update 只改可编辑列（page_type/schema_type/template_json/active）。
func (r *geoSchemaTemplateRepo) Update(ctx context.Context, tpl *model.GeoSchemaTemplate) error {
	res := r.db.WithContext(ctx).Model(&model.GeoSchemaTemplate{}).
		Where("id = ?", tpl.ID).
		Updates(map[string]any{
			"page_type":     tpl.PageType,
			"schema_type":   tpl.SchemaType,
			"template_json": tpl.TemplateJSON,
			"active":        tpl.Active,
			"updated_at":    time.Now(),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *geoSchemaTemplateRepo) Delete(ctx context.Context, id string) error {
	res := r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.GeoSchemaTemplate{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
