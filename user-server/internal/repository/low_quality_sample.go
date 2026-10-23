package repository

import (
	"context"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// LowQualitySampleRepository 低质样本仓储（拟人度域 管理面读模型）
//
// 写入由 HumanizeLowQualitySampleCollector 负责，本仓储仅提供管理面列表查询，
// 命名按业务域（low_quality_sample），不按管理角色或优先级。
type LowQualitySampleRepository struct {
	db *gorm.DB
}

// NewLowQualitySampleRepository 构造（无参，内部取库句柄）
func NewLowQualitySampleRepository() *LowQualitySampleRepository {
	return &LowQualitySampleRepository{db: db.GetDB()}
}

// List 低质样本分页列表
func (r *LowQualitySampleRepository) List(ctx context.Context, sampleType string, page, pageSize int) ([]model.LowQualitySample, int64, error) {
	// 夹取分页：page ≤ 0 时 `(page-1)*pageSize` 是负 offset（PG 直接报错），
	// pageSize 无上界时 page_size=100000 一条就能把整表读进内存。
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	q := r.db.WithContext(ctx).Model(&model.LowQualitySample{})
	if sampleType != "" {
		q = q.Where("sample_type = ?", sampleType)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []model.LowQualitySample
	if err := q.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}
