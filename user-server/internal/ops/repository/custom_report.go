package repository

import (
	"context"

	"hivemtk-user/internal/ops/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// CustomReportRepository 自定义报表仓库
type CustomReportRepository struct {
	db *gorm.DB
}

// NewCustomReportRepository 创建自定义报表仓库
func NewCustomReportRepository() *CustomReportRepository {
	return &CustomReportRepository{
		db: _db.GetDB(),
	}
}

// Create 创建报表
func (r *CustomReportRepository) Create(ctx context.Context, report *model.CustomReport) error {
	return r.db.WithContext(ctx).Create(report).Error
}

// GetByID 根据 ID 获取报表
func (r *CustomReportRepository) GetByID(ctx context.Context, id uint) (*model.CustomReport, error) {
	var report model.CustomReport
	err := r.db.WithContext(ctx).First(&report, id).Error
	if err != nil {
		return nil, err
	}
	return &report, nil
}

// GetAll 获取所有报表列表
func (r *CustomReportRepository) GetAll(ctx context.Context, page, pageSize int) ([]*model.CustomReport, int64, error) {
	var reports []*model.CustomReport
	var total int64

	offset := (page - 1) * pageSize

	q := r.db.WithContext(ctx).Model(&model.CustomReport{})
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := q.
		Order("created_at DESC").
		Limit(pageSize).Offset(offset).
		Find(&reports).Error
	if err != nil {
		return nil, 0, err
	}

	return reports, total, nil
}

// GetByCreatorOrPublic 获取当前用户创建的或公开的报表列表（普通用户视角）
func (r *CustomReportRepository) GetByCreatorOrPublic(ctx context.Context, creatorID uint, page, pageSize int) ([]*model.CustomReport, int64, error) {
	var reports []*model.CustomReport
	var total int64

	offset := (page - 1) * pageSize

	where := r.db.WithContext(ctx).Model(&model.CustomReport{}).Where("created_by = ? OR is_public = ?", creatorID, true)

	if err := where.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := where.
		Order("created_at DESC").
		Limit(pageSize).Offset(offset).
		Find(&reports).Error
	if err != nil {
		return nil, 0, err
	}

	return reports, total, nil
}

// GetByDataSource 根据数据源获取报表(排除公开模板)
func (r *CustomReportRepository) GetByDataSource(ctx context.Context, dataSource string) ([]*model.CustomReport, error) {
	var reports []*model.CustomReport
	err := r.db.WithContext(ctx).Where("data_source = ? AND is_public = ?", dataSource, false).
		Order("created_at DESC").
		Find(&reports).Error
	return reports, err
}

// Update 更新报表
func (r *CustomReportRepository) Update(ctx context.Context, report *model.CustomReport) error {
	return r.db.WithContext(ctx).Save(report).Error
}

// Delete 删除报表
func (r *CustomReportRepository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.CustomReport{}).Error
}

// GetPublicTemplates 获取公开报表模板
func (r *CustomReportRepository) GetPublicTemplates(ctx context.Context) ([]*model.CustomReport, error) {
	var reports []*model.CustomReport
	err := r.db.WithContext(ctx).Where("is_public = ?", true).
		Order("created_at DESC").
		Find(&reports).Error
	return reports, err
}

// UseTemplate 使用模板创建报表
func (r *CustomReportRepository) UseTemplate(ctx context.Context, templateID uint, createdBy uint) (*model.CustomReport, error) {
	var template model.CustomReport
	err := r.db.WithContext(ctx).First(&template, templateID).Error
	if err != nil {
		return nil, err
	}

	report := &model.CustomReport{
		Name:        template.Name,
		Description: template.Description,
		DataSource:  template.DataSource,
		Dimensions:  template.Dimensions,
		Metrics:     template.Metrics,
		Filters:     template.Filters,
		ChartType:   template.ChartType,
		ChartConfig: template.ChartConfig,
		IsPublic:    false,
		CreatedBy:   createdBy,
	}

	err = r.db.WithContext(ctx).Create(report).Error
	return report, err
}

// NewCustomReportRepositoryWithDB 创建指定数据库连接的自定义报表仓库实例（用于测试）
func NewCustomReportRepositoryWithDB(db *gorm.DB) *CustomReportRepository {
	return &CustomReportRepository{db: db}
}
