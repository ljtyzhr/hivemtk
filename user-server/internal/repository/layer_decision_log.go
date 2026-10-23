package repository

import (
	"context"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

type LayerDecisionLogRepository struct {
	db *gorm.DB
}

func NewLayerDecisionLogRepository(db *gorm.DB) *LayerDecisionLogRepository {
	return &LayerDecisionLogRepository{db: db}
}

// Record 新增决策日志
func (r *LayerDecisionLogRepository) Record(ctx context.Context, log *model.LayerDecisionLog) error {
	return r.db.WithContext(ctx).Create(log).Error
}

// GetByTraceID 按 trace 查询所有相关日志
func (r *LayerDecisionLogRepository) GetByTraceID(ctx context.Context, traceID string) ([]model.LayerDecisionLog, error) {
	if traceID == "" {
		return nil, nil
	}
	var logs []model.LayerDecisionLog
	err := r.db.WithContext(ctx).
		Where("trace_id = ?", traceID).
		Order("created_at ASC, id ASC").
		Find(&logs).Error
	return logs, err
}

// Recent 查询最近 N 条 (按时间倒序)
func (r *LayerDecisionLogRepository) Recent(ctx context.Context, limit int) ([]model.LayerDecisionLog, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	var logs []model.LayerDecisionLog
	err := r.db.WithContext(ctx).
		Order("created_at DESC, id DESC").
		Limit(limit).
		Find(&logs).Error
	return logs, err
}
