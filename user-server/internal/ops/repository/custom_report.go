package repository

import (
	"context"
	"errors"

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

// GetAll 获取所有报表列表。
//
// Count 与 Find 必须各起一条链：同一 *gorm.DB 上先 Count 再追加分页子句，
// GORM 会把分页子句叠进 Count 语句（判例 #133/#150）。分页夹取 1..200。
func (r *CustomReportRepository) GetAll(ctx context.Context, page, pageSize int) ([]*model.CustomReport, int64, error) {
	page, pageSize = clampPage(page, pageSize)
	var reports []*model.CustomReport
	var total int64

	base := func() *gorm.DB {
		return r.db.WithContext(ctx).Model(&model.CustomReport{})
	}
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := base().
		Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&reports).Error
	if err != nil {
		return nil, 0, err
	}

	return reports, total, nil
}

// GetByCreatorOrPublic 获取当前用户创建的或公开的报表列表（普通用户视角；
// Count/Find 拆链 + 夹取，理由同 GetAll）。
func (r *CustomReportRepository) GetByCreatorOrPublic(ctx context.Context, creatorID uint, page, pageSize int) ([]*model.CustomReport, int64, error) {
	page, pageSize = clampPage(page, pageSize)
	var reports []*model.CustomReport
	var total int64

	base := func() *gorm.DB {
		return r.db.WithContext(ctx).Model(&model.CustomReport{}).
			Where("created_by = ? OR is_public = ?", creatorID, true)
	}
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := base().
		Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&reports).Error
	if err != nil {
		return nil, 0, err
	}

	return reports, total, nil
}

// Update 整行回写报表。走 db.SaveWholeRow：裸 Save 命中 0 行会回退成
// `INSERT ... ON CONFLICT (id) DO UPDATE`（gorm@v1.31.2），把「编辑页读出后、
// 保存前被删掉」的报表按原 id 插回。
func (r *CustomReportRepository) Update(ctx context.Context, report *model.CustomReport) error {
	if report == nil || report.ID == 0 {
		return errors.New("custom report: 缺主键，不能整行回写")
	}
	return _db.SaveWholeRow(r.db.WithContext(ctx), report)
}

// Delete 删除报表。0 行 ⇒ ErrRecordNotFound。
func (r *CustomReportRepository) Delete(ctx context.Context, id uint) error {
	return _db.EnsureAffected(r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.CustomReport{}))
}

// GetPublicTemplates 获取公开报表模板（limit 兜底 200：模板是运营手工建的
// 配置型数据，行数可控，但无界 Find 的习惯不能留）。
func (r *CustomReportRepository) GetPublicTemplates(ctx context.Context) ([]*model.CustomReport, error) {
	var reports []*model.CustomReport
	err := r.db.WithContext(ctx).Where("is_public = ?", true).
		Order("created_at DESC").
		Limit(200).
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
