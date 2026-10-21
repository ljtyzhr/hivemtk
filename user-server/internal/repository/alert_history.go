package repository

import (
	"context"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
)

// AlertHistoryRepository 告警历史仓储接口
type AlertHistoryRepository interface {
	Create(ctx context.Context, h *model.AlertHistory) error
	List(ctx context.Context, page, size int, ruleID uint, source string) ([]*model.AlertHistory, int64, error)
	ResolveFiring(ctx context.Context, ruleID uint, resolvedAt time.Time) error
	// CountFiring 统计所有未恢复（firing）告警数
	CountFiring(ctx context.Context) (int64, error)
	// ListFiring 列出未恢复（firing）告警（按触发时间倒序，limit<=0 时取前 20 条）
	ListFiring(ctx context.Context, limit int) ([]*model.AlertHistory, error)
	// UpdateNotify 回写通知结果（异步通知完成后补记）
	UpdateNotify(ctx context.Context, id uint, result string) error
}

type alertHistoryRepo struct {
	db *gorm.DB
}

// NewAlertHistoryRepository 构造
func NewAlertHistoryRepository() AlertHistoryRepository {
	return &alertHistoryRepo{db: db.GetDB()}
}

func (r *alertHistoryRepo) Create(ctx context.Context, h *model.AlertHistory) error {
	return r.db.WithContext(ctx).Create(h).Error
}

func (r *alertHistoryRepo) List(ctx context.Context, page, size int, ruleID uint, source string) ([]*model.AlertHistory, int64, error) {
	if page < 1 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	var list []*model.AlertHistory
	var total int64
	q := r.db.WithContext(ctx).Model(&model.AlertHistory{})
	if ruleID > 0 {
		q = q.Where("rule_id = ?", ruleID)
	}
	if source != "" {
		q = q.Where("source = ?", source)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := q.Order("triggered_at DESC").
		Offset((page - 1) * size).
		Limit(size).
		Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func (r *alertHistoryRepo) ResolveFiring(ctx context.Context, ruleID uint, resolvedAt time.Time) error {
	return r.db.WithContext(ctx).Model(&model.AlertHistory{}).
		Where("rule_id = ? AND status = ?", ruleID, model.AlertHistoryFiring).
		Updates(map[string]any{"status": model.AlertHistoryResolved, "resolved_at": resolvedAt}).
		Error
}

func (r *alertHistoryRepo) CountFiring(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.AlertHistory{}).
		Where("status = ?", model.AlertHistoryFiring).
		Count(&n).Error
	return n, err
}

func (r *alertHistoryRepo) ListFiring(ctx context.Context, limit int) ([]*model.AlertHistory, error) {
	if limit <= 0 {
		limit = 20
	}
	var list []*model.AlertHistory
	err := r.db.WithContext(ctx).Model(&model.AlertHistory{}).
		Where("status = ?", model.AlertHistoryFiring).
		Order("triggered_at DESC").
		Limit(limit).
		Find(&list).Error
	return list, err
}

// UpdateNotify 回写通知结果；0 行说明存量行不属于本租户或已不存在，不能静默成功。
func (r *alertHistoryRepo) UpdateNotify(ctx context.Context, id uint, result string) error {
	res := r.db.WithContext(ctx).Model(&model.AlertHistory{}).
		Where("id = ?", id).
		Update("notify_result", result)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

var _ AlertHistoryRepository = (*alertHistoryRepo)(nil)
