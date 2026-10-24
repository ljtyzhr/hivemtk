package repository

import (
	"context"

	"gorm.io/gorm"

	"hivemtk-user/internal/ops/model"
	_db "hivemtk-user/internal/pkg/db"
)

// PerformanceTestRepository 性能压测结果仓储
type PerformanceTestRepository struct {
	db *gorm.DB
}

// NewPerformanceTestRepository 创建压测结果仓储实例
func NewPerformanceTestRepository() *PerformanceTestRepository {
	return &PerformanceTestRepository{db: _db.GetDB()}
}

// Create 创建压测记录
func (r *PerformanceTestRepository) Create(ctx context.Context, record *model.PerformanceTestResult) error {
	return r.db.WithContext(ctx).Create(record).Error
}

// UpdateFields 按主键更新指定字段。0 行 ⇒ ErrRecordNotFound：
// 压测任务后台回写时若记录已被删，静默成功会让这条压测永远停在 running 状态
// （无人知晓结果丢了）。
func (r *PerformanceTestRepository) UpdateFields(ctx context.Context, id uint, fields map[string]any) error {
	return _db.EnsureAffected(r.db.WithContext(ctx).
		Model(&model.PerformanceTestResult{}).
		Where("id = ?", id).
		Updates(fields))
}

// GetByID 根据 ID 获取压测结果
func (r *PerformanceTestRepository) GetByID(ctx context.Context, id uint) (*model.PerformanceTestResult, error) {
	var record model.PerformanceTestResult
	if err := r.db.WithContext(ctx).First(&record, id).Error; err != nil {
		return nil, err
	}
	return &record, nil
}

// List 分页查询压测历史（按 created_at DESC 排序；Count/Find 各起一条链 +
// 分页夹取 1..200，口径同 clampPage）。
func (r *PerformanceTestRepository) List(ctx context.Context, page, pageSize int) ([]*model.PerformanceTestResult, int64, error) {
	page, pageSize = clampPage(page, pageSize)
	var list []*model.PerformanceTestResult
	var total int64
	if err := r.db.WithContext(ctx).
		Model(&model.PerformanceTestResult{}).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := r.db.WithContext(ctx).
		Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&list).Error
	return list, total, err
}
