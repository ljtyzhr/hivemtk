package repository

import (
	"context"
	"fmt"
	"time"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// SmsDeliveryAggregateRow 到达率基础聚合行
type SmsDeliveryAggregateRow struct {
	Total     int64
	Delivered int64
	Failed    int64
	Retryable int64
}

// SmsDeliveryCarrierStatRow 按运营商维度统计行
type SmsDeliveryCarrierStatRow struct {
	Provider  string
	Total     int64
	Delivered int64
	Failed    int64
}

// SmsDeliveryRepository 短信到达率追踪仓库接口
//
// 配套 service.SmsDeliveryTrackerService，封装 sms_number_portability_logs 与
// sms_delivery_statuses 两张表的所有 DB 操作。service 不再直接持有 *gorm.DB。
type SmsDeliveryRepository interface {
	CreatePortability(ctx context.Context, rec *model.SmsNumberPortabilityRecord) error
	LoadLatestPortability(ctx context.Context, limit int) ([]model.SmsNumberPortabilityRecord, error)
	ListPortability(ctx context.Context, phone string, page, limit int) ([]model.SmsNumberPortabilityRecord, int64, error)

	GetDeliveryAggregate(ctx context.Context, start, end time.Time) (*SmsDeliveryAggregateRow, error)
	CountBlacklisted(ctx context.Context, start, end time.Time) (int64, error)
	CountPortabilityFailure(ctx context.Context, start, end time.Time) (int64, error)
	GetCarrierStats(ctx context.Context, start, end time.Time) ([]SmsDeliveryCarrierStatRow, error)
}

type smsDeliveryRepo struct {
	db *gorm.DB
}

// NewSmsDeliveryRepositoryDefault 默认构造（全局句柄）。
func NewSmsDeliveryRepositoryDefault() SmsDeliveryRepository {
	return NewSmsDeliveryRepository(GetDB())
}

func NewSmsDeliveryRepository(db *gorm.DB) SmsDeliveryRepository {
	return &smsDeliveryRepo{db: db}
}

// errSmsDeliveryUnavailable 仓储句柄缺失。
//
// 与读口「没数据就返回空」不同：写口在句柄缺失时必须报错，否则调用方会把
// 写失败读成「已记录」，携号转网流水凭据被静默丢掉。
var errSmsDeliveryUnavailable = fmt.Errorf("sms delivery repository not initialized")

func (r *smsDeliveryRepo) CreatePortability(ctx context.Context, rec *model.SmsNumberPortabilityRecord) error {
	if r == nil || r.db == nil {
		return errSmsDeliveryUnavailable
	}
	if rec == nil {
		return fmt.Errorf("sms delivery repository: nil 不是写入对象")
	}
	return r.db.WithContext(ctx).Create(rec).Error
}

// LoadLatestPortability 取最近 N 条携号转网流水。
//
// limit 在仓储侧夹取：GORM 的 `Limit(0)` 等于**不加 limit**，`Limit(-1)` 同理
// 被忽略；本表是「检测到的手机号异动」流水，随发送量单调增长，一次误传 0 就是
// 整张表读进内存。
func (r *smsDeliveryRepo) LoadLatestPortability(ctx context.Context, limit int) ([]model.SmsNumberPortabilityRecord, error) {
	if r == nil || r.db == nil {
		return []model.SmsNumberPortabilityRecord{}, nil
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	rows := []model.SmsNumberPortabilityRecord{}
	if err := r.db.WithContext(ctx).Order("detected_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *smsDeliveryRepo) ListPortability(ctx context.Context, phone string, page, limit int) ([]model.SmsNumberPortabilityRecord, int64, error) {
	if r == nil || r.db == nil {
		return []model.SmsNumberPortabilityRecord{}, 0, nil
	}
	// page = 0 ⇒ offset 为负 ⇒ PG 报 "OFFSET must not be negative"；
	// limit = 0 ⇒ Limit(0) 等于整表（与 LoadLatestPortability 同一条）。
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}

	// Count 与 Find 必须各起一条 query（见 base()），不能在同一条链上先 Count 再 Find。
	base := func() *gorm.DB {
		q := r.db.WithContext(ctx).Model(&model.SmsNumberPortabilityRecord{})
		if phone != "" {
			q = q.Where("phone = ?", phone)
		}
		return q
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count portability: %w", err)
	}

	var rows []model.SmsNumberPortabilityRecord
	if err := base().Order("detected_at DESC").
		Offset((page - 1) * limit).
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("list portability: %w", err)
	}
	return rows, total, nil
}

func (r *smsDeliveryRepo) GetDeliveryAggregate(ctx context.Context, start, end time.Time) (*SmsDeliveryAggregateRow, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("sms delivery repo: db is nil")
	}
	var row SmsDeliveryAggregateRow
	if err := r.db.WithContext(ctx).
		Table("sms_delivery_statuses").
		Where("received_at >= ? AND received_at < ?", start, end).
		Select(`
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE status = 'delivered') AS delivered,
			COUNT(*) FILTER (WHERE status = 'failed') AS failed,
			COUNT(*) FILTER (WHERE status = 'retryable') AS retryable
		`).
		Scan(&row).Error; err != nil {
		return nil, fmt.Errorf("query delivery aggregate: %w", err)
	}
	return &row, nil
}

func (r *smsDeliveryRepo) CountBlacklisted(ctx context.Context, start, end time.Time) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	var n int64
	err := r.db.WithContext(ctx).
		Table("sms_delivery_statuses").
		Where("received_at >= ? AND received_at < ? AND error_code LIKE ?", start, end, "ERR_4002%").
		Count(&n).Error
	return n, err
}

func (r *smsDeliveryRepo) CountPortabilityFailure(ctx context.Context, start, end time.Time) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	var n int64
	err := r.db.WithContext(ctx).
		Table("sms_delivery_statuses").
		Where("received_at >= ? AND received_at < ? AND (error_code = 'ERR_4005' OR error_msg LIKE '%携号转网%')", start, end).
		Count(&n).Error
	return n, err
}

func (r *smsDeliveryRepo) GetCarrierStats(ctx context.Context, start, end time.Time) ([]SmsDeliveryCarrierStatRow, error) {
	if r == nil || r.db == nil {
		return []SmsDeliveryCarrierStatRow{}, nil
	}
	var rows []SmsDeliveryCarrierStatRow
	if err := r.db.WithContext(ctx).
		Table("sms_delivery_statuses").
		Where("received_at >= ? AND received_at < ?", start, end).
		Select(`
			COALESCE(provider, 'unknown') AS provider,
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE status = 'delivered') AS delivered,
			COUNT(*) FILTER (WHERE status = 'failed') AS failed
		`).
		Group("provider").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
