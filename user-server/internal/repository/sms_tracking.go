package repository

import (
	"context"
	"errors"
	"fmt"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/tenant"

	"gorm.io/gorm"
)

// SmsTrackingRepository 短信追踪仓库接口
type SmsTrackingRepository interface {
	CreateStatus(ctx context.Context, status *model.SmsDeliveryStatus) error
	UpdateStatus(ctx context.Context, status *model.SmsDeliveryStatus) error
	GetByMessageID(ctx context.Context, messageID string) (*model.SmsDeliveryStatus, error)
	// TenantIDByMessageID 系统作用域反查回执归属租户；跨租户同号时 ok=false（拒猜）
	TenantIDByMessageID(ctx context.Context, messageID string) (uint, bool, error)

	ListStatusesByJob(ctx context.Context, jobID string, page, limit int) ([]*model.SmsDeliveryStatus, int64, error)
	ListStatusesByPhone(ctx context.Context, phone string, page, limit int) ([]*model.SmsDeliveryStatus, int64, error)
	ListRetryableStatuses(ctx context.Context, limit int) ([]*model.SmsDeliveryStatus, error)
}

type smsTrackingRepo struct {
	db *gorm.DB
}

// NewSmsTrackingRepository 创建短信追踪仓库
//
// db 为 nil 时尝试从全局获取（_db.GetDB()）；若全局也未初始化（如测试场景未 SetTestDB），
// 则返回 db=nil 的 repo，所有方法走 nil 兜底分支返回错误或空结果，避免 panic。
// NewSmsTrackingRepositoryDefault 默认构造（全局句柄）。
func NewSmsTrackingRepositoryDefault() SmsTrackingRepository {
	return NewSmsTrackingRepository(GetDB())
}

func NewSmsTrackingRepository(db *gorm.DB) SmsTrackingRepository {
	if db == nil {
		db = _db.GetDB()
	}
	return &smsTrackingRepo{db: db}
}

var errNilDB = fmt.Errorf("sms tracking repo: db is nil")

// clampSmsStatusLimit 夹取「取最近 N 条」的 N：GORM 的 Limit(0) 等于**不加 limit**
// （整表），Limit(-1) 同样被忽略。本表是每次发送一行的回执流水，无界读的代价
// 是随发送量线性增长的内存。
func clampSmsStatusLimit(limit int) int {
	if limit < 1 {
		return 100
	}
	if limit > 500 {
		return 500
	}
	return limit
}

// clampSmsStatusPaging 夹取分页：page = 0 ⇒ offset 为负 ⇒ PG 报
// "OFFSET must not be negative"；limit = 0 ⇒ 不加 limit。
func clampSmsStatusPaging(page, limit int) (int, int) {
	if page < 1 {
		page = 1
	}
	return page, clampSmsStatusLimit(limit)
}

func (r *smsTrackingRepo) CreateStatus(ctx context.Context, status *model.SmsDeliveryStatus) error {
	if r == nil || r.db == nil {
		return errNilDB
	}
	return r.db.WithContext(ctx).Create(status).Error
}

// UpdateStatus 整行回写一条送达状态。
//
// 走 `db.SaveWholeRow`：裸 Save 命中 0 行会回退成 `INSERT ... ON CONFLICT (id)
// DO UPDATE` —— 把「读完待重试列表之后、写回之前被删掉」的回执行按原 id 插回去，
// 而重试计数（retry_count）带着旧值复活，于是这条短信会被永远重试下去。
func (r *smsTrackingRepo) UpdateStatus(ctx context.Context, status *model.SmsDeliveryStatus) error {
	if r == nil || r.db == nil {
		return errNilDB
	}
	if status == nil {
		return errors.New("sms tracking repo: nil 不是更新对象")
	}
	return _db.SaveWholeRow(r.db.WithContext(ctx), status)
}

func (r *smsTrackingRepo) GetByMessageID(ctx context.Context, messageID string) (*model.SmsDeliveryStatus, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	var status model.SmsDeliveryStatus
	err := r.db.WithContext(ctx).Where("message_id = ?", messageID).First(&status).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &status, nil
}

// TenantIDByMessageID 运营商回执没有租户线索，靠发送侧预落的 message_id 行自证归属。
// 唯一索引是 (tenant_id, message_id)：同一 message_id 若在多个租户下都有行，
// 无法判定本次回执属于谁 ⇒ 返回 ok=false，由调用方 fail-closed（绝不随手挑一个）。
func (r *smsTrackingRepo) TenantIDByMessageID(ctx context.Context, messageID string) (uint, bool, error) {
	if r == nil || r.db == nil {
		return 0, false, errNilDB
	}
	var ids []uint
	if err := r.db.WithContext(tenant.WithSystemScope(ctx)).
		Model(&model.SmsDeliveryStatus{}).
		Distinct("tenant_id").
		Where("message_id = ?", messageID).
		Limit(2).
		Pluck("tenant_id", &ids).Error; err != nil {
		return 0, false, err
	}
	if len(ids) != 1 || ids[0] == 0 {
		return 0, false, nil
	}
	return ids[0], true, nil
}

func (r *smsTrackingRepo) ListStatusesByJob(ctx context.Context, jobID string, page, limit int) ([]*model.SmsDeliveryStatus, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, nil
	}
	page, limit = clampSmsStatusPaging(page, limit)

	// Count 与 Find 必须各起一条 query（见 base()），不能在同一条链上先 Count 再 Find。
	base := func() *gorm.DB {
		return r.db.WithContext(ctx).Model(&model.SmsDeliveryStatus{}).Where("job_id = ?", jobID)
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var statuses []*model.SmsDeliveryStatus
	offset := (page - 1) * limit
	if err := base().Order("received_at DESC").Offset(offset).Limit(limit).Find(&statuses).Error; err != nil {
		return nil, 0, err
	}
	return statuses, total, nil
}

func (r *smsTrackingRepo) ListStatusesByPhone(ctx context.Context, phone string, page, limit int) ([]*model.SmsDeliveryStatus, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, nil
	}
	page, limit = clampSmsStatusPaging(page, limit)

	base := func() *gorm.DB {
		return r.db.WithContext(ctx).Model(&model.SmsDeliveryStatus{}).Where("phone = ?", phone)
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var statuses []*model.SmsDeliveryStatus
	offset := (page - 1) * limit
	if err := base().Order("received_at DESC").Offset(offset).Limit(limit).Find(&statuses).Error; err != nil {
		return nil, 0, err
	}
	return statuses, total, nil
}

func (r *smsTrackingRepo) ListRetryableStatuses(ctx context.Context, limit int) ([]*model.SmsDeliveryStatus, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	var statuses []*model.SmsDeliveryStatus
	err := r.db.WithContext(ctx).Where("is_retryable = ? AND status = ?",
		true, model.SmsStatusRetryable).
		Order("received_at ASC").
		Limit(clampSmsStatusLimit(limit)).
		Find(&statuses).Error
	return statuses, err
}
