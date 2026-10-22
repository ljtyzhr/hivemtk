package repository

import (
	"context"
	"errors"
	"time"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/tenant"

	"gorm.io/gorm"
)

// EmailTrackingRepository 邮件追踪仓库接口
type EmailTrackingRepository interface {
	CreateEvent(ctx context.Context, event *model.EmailTrackingEvent) error
	EventExists(ctx context.Context, eventID string) (bool, error)
	ListEventsByJob(ctx context.Context, jobID string, page, limit int) ([]*model.EmailTrackingEvent, int64, error)
	CountEventsByJob(ctx context.Context, jobID, eventType string) (int64, error)
	CountUniqueEmailsByJob(ctx context.Context, jobID, eventType string) (int64, error)
	ListEventsByRange(ctx context.Context, start, end time.Time) ([]*model.EmailTrackingEvent, error)
	CountEventsByRange(ctx context.Context, start, end time.Time, eventType string) (int64, error)

	GetJobMetric(ctx context.Context, jobID string) (*model.EmailJobMetric, error)
	UpsertJobMetric(ctx context.Context, metric *model.EmailJobMetric) error
	ListJobMetricsByRange(ctx context.Context, start, end time.Time) ([]*model.EmailJobMetric, error)

	// TenantIDByRecipient 系统作用域按收件人反查发信租户；
	// 同一收件人被多个租户发过信时 ok=false（拒猜）
	TenantIDByRecipient(ctx context.Context, email string) (uint, bool, error)
}

type emailTrackingRepo struct {
	db *gorm.DB
}

// NewEmailTrackingRepository 创建邮件追踪仓库
func NewEmailTrackingRepository(db *gorm.DB) EmailTrackingRepository {
	if db == nil {
		db = _db.GetDB()
	}
	return &emailTrackingRepo{db: db}
}

// TenantIDByRecipient 供应商回执没有登录态，收件人 + 发信台账可自证租户。
// 同一收件人若出现在多个租户的 email_sends 里，无法判定这封信属于谁 ⇒ ok=false，
// 由调用方 fail-closed（要求 webhook 显式携带 tenant_code）。
func (r *emailTrackingRepo) TenantIDByRecipient(ctx context.Context, email string) (uint, bool, error) {
	if r == nil || r.db == nil {
		return 0, false, errors.New("email tracking repository 未初始化（db is nil）")
	}
	var ids []uint
	if err := r.db.WithContext(tenant.WithSystemScope(ctx)).
		Model(&model.EmailSend{}).
		Distinct("tenant_id").
		Where("to = ?", email).
		Limit(2).
		Pluck("tenant_id", &ids).Error; err != nil {
		return 0, false, err
	}
	if len(ids) != 1 || ids[0] == 0 {
		return 0, false, nil
	}
	return ids[0], true, nil
}

func (r *emailTrackingRepo) CreateEvent(ctx context.Context, event *model.EmailTrackingEvent) error {
	if r == nil || r.db == nil {
		return errors.New("email tracking repository 未初始化（db is nil）")
	}
	return r.db.WithContext(ctx).Create(event).Error
}

func (r *emailTrackingRepo) EventExists(ctx context.Context, eventID string) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("email tracking repository 未初始化（db is nil）")
	}
	var count int64
	err := r.db.WithContext(ctx).Model(&model.EmailTrackingEvent{}).Where("event_id = ?", eventID).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *emailTrackingRepo) ListEventsByJob(ctx context.Context, jobID string, page, limit int) ([]*model.EmailTrackingEvent, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, errors.New("email tracking repository 未初始化（db is nil）")
	}
	var events []*model.EmailTrackingEvent
	var total int64

	query := r.db.WithContext(ctx).Model(&model.EmailTrackingEvent{}).Where("job_id = ?", jobID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := query.Order("timestamp DESC").Offset(offset).Limit(limit).Find(&events).Error; err != nil {
		return nil, 0, err
	}
	return events, total, nil
}

func (r *emailTrackingRepo) CountEventsByJob(ctx context.Context, jobID, eventType string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("email tracking repository 未初始化（db is nil）")
	}
	var count int64
	query := r.db.WithContext(ctx).Model(&model.EmailTrackingEvent{}).Where("job_id = ?", jobID)
	if eventType != "" {
		query = query.Where("event_type = ?", eventType)
	}
	err := query.Count(&count).Error
	return count, err
}

func (r *emailTrackingRepo) CountUniqueEmailsByJob(ctx context.Context, jobID, eventType string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("email tracking repository 未初始化（db is nil）")
	}
	var count int64
	err := r.db.WithContext(ctx).Model(&model.EmailTrackingEvent{}).
		Where("job_id = ? AND event_type = ?", jobID, eventType).
		Distinct("email").Count(&count).Error
	return count, err
}

func (r *emailTrackingRepo) ListEventsByRange(ctx context.Context, start, end time.Time) ([]*model.EmailTrackingEvent, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("email tracking repository 未初始化（db is nil）")
	}
	var events []*model.EmailTrackingEvent
	err := r.db.WithContext(ctx).Where("timestamp BETWEEN ? AND ?", start, end).Find(&events).Error
	return events, err
}

func (r *emailTrackingRepo) CountEventsByRange(ctx context.Context, start, end time.Time, eventType string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("email tracking repository 未初始化（db is nil）")
	}
	var count int64
	query := r.db.WithContext(ctx).Model(&model.EmailTrackingEvent{}).Where("timestamp BETWEEN ? AND ?", start, end)
	if eventType != "" {
		query = query.Where("event_type = ?", eventType)
	}
	err := query.Count(&count).Error
	return count, err
}

func (r *emailTrackingRepo) GetJobMetric(ctx context.Context, jobID string) (*model.EmailJobMetric, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("email tracking repository 未初始化（db is nil）")
	}
	var metric model.EmailJobMetric
	err := r.db.WithContext(ctx).Where("job_id = ?", jobID).First(&metric).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &metric, nil
}

func (r *emailTrackingRepo) UpsertJobMetric(ctx context.Context, metric *model.EmailJobMetric) error {
	if r == nil || r.db == nil {
		return errors.New("email tracking repository 未初始化（db is nil）")
	}
	existing, err := r.GetJobMetric(ctx, metric.JobID)
	if err != nil {
		return err
	}
	if existing != nil {
		metric.ID = existing.ID
		// Save 写全部字段（含零值），必须带回库里的 tenant_id：
		// 调用方传进来的是零值 model（tenant_id=0），直接 Save 等于把租户改写成 0，
		// 多租户模式下 tenant 回调会当场拒掉（"更新不可改写 tenant_id"）。
		metric.TenantID = existing.TenantID
		return r.db.WithContext(ctx).Save(metric).Error
	}
	return r.db.WithContext(ctx).Create(metric).Error
}

func (r *emailTrackingRepo) ListJobMetricsByRange(ctx context.Context, start, end time.Time) ([]*model.EmailJobMetric, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("email tracking repository 未初始化（db is nil）")
	}
	var metrics []*model.EmailJobMetric
	err := r.db.WithContext(ctx).Where("updated_at BETWEEN ? AND ?", start, end).Find(&metrics).Error
	return metrics, err
}
