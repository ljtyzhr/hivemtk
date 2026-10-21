package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
)

// AlertRuleRepository 告警规则仓储接口
type AlertRuleRepository interface {
	Create(ctx context.Context, r *model.AlertRule) error
	Update(ctx context.Context, r *model.AlertRule) error
	Delete(ctx context.Context, id uint) error
	GetByID(ctx context.Context, id uint) (*model.AlertRule, error)
	List(ctx context.Context, page, size int, enabledOnly bool) ([]*model.AlertRule, int64, error)
	ListEnabled(ctx context.Context) ([]*model.AlertRule, error)
	UpdateLastTriggered(ctx context.Context, id uint, t time.Time) error
	BatchUpdateStatus(ctx context.Context, ids []uint, enabled bool) error
}

type alertRuleRepo struct {
	db *gorm.DB
}

// NewAlertRuleRepository 构造
func NewAlertRuleRepository() AlertRuleRepository {
	return &alertRuleRepo{db: db.GetDB()}
}

func (r *alertRuleRepo) Create(ctx context.Context, m *model.AlertRule) error {
	return r.db.WithContext(ctx).Create(m).Error
}

// Update 更新告警规则
//
// 不用 Save：Save 命中 0 行会回退成 ON CONFLICT(裸主键) DO UPDATE，跨租户按主键
// Save 能把整行搬到调用方租户下（#14 真库探针复现）。这里显式 Omit 主键/租户/创建时间，
// 谓词只剩 WHERE id + 注入的 tenant_id，0 行就是 0 行；
// Select("*") 保证布尔/数值零值也会落库（Updates(struct) 默认跳过零值）。
func (r *alertRuleRepo) Update(ctx context.Context, m *model.AlertRule) error {
	res := r.db.WithContext(ctx).Model(&model.AlertRule{}).
		Where("id = ?", m.ID).
		Select("*").
		Omit("id", "tenant_id", "created_at").
		Updates(m)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *alertRuleRepo) Delete(ctx context.Context, id uint) error {
	res := r.db.WithContext(ctx).Delete(&model.AlertRule{}, id)
	if res.Error != nil {
		return fmt.Errorf("删除告警规则失败: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *alertRuleRepo) GetByID(ctx context.Context, id uint) (*model.AlertRule, error) {
	var m model.AlertRule
	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *alertRuleRepo) List(ctx context.Context, page, size int, enabledOnly bool) ([]*model.AlertRule, int64, error) {
	if page < 1 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	var list []*model.AlertRule
	var total int64
	q := r.db.WithContext(ctx).Model(&model.AlertRule{})
	if enabledOnly {
		q = q.Where("enabled = ?", true)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := q.Order("created_at DESC").
		Offset((page - 1) * size).
		Limit(size).
		Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func (r *alertRuleRepo) ListEnabled(ctx context.Context) ([]*model.AlertRule, error) {
	var list []*model.AlertRule
	if err := r.db.WithContext(ctx).
		Where("enabled = ?", true).
		Order("id ASC").
		Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (r *alertRuleRepo) UpdateLastTriggered(ctx context.Context, id uint, t time.Time) error {
	res := r.db.WithContext(ctx).Model(&model.AlertRule{}).
		Where("id = ?", id).
		Update("last_triggered_at", t)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *alertRuleRepo) BatchUpdateStatus(ctx context.Context, ids []uint, enabled bool) error {
	if len(ids) == 0 {
		return errors.New("ids 不能为空")
	}
	res := r.db.WithContext(ctx).Model(&model.AlertRule{}).
		Where("id IN ?", ids).
		Update("enabled", enabled)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

var _ AlertRuleRepository = (*alertRuleRepo)(nil)
