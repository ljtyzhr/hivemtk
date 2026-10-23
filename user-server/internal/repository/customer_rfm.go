package repository

import (
	"context"
	"errors"
	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"
	"time"

	"gorm.io/gorm"
)

// CustomerRFMRepository 客户 RFM 仓库
type CustomerRFMRepository interface {
	Upsert(ctx context.Context, rfm *model.CustomerRFM) error
	GetByCustomerID(ctx context.Context, customerID string) (*model.CustomerRFM, error)
	ListBySegment(ctx context.Context, segment string, page, pageSize int) ([]*model.CustomerRFM, int64, error)
	CountBySegment(ctx context.Context) (map[string]int64, error)
	DeleteByCustomerID(ctx context.Context, customerID string) error
}

type customerRFMRepo struct {
	db *gorm.DB
}

func NewCustomerRFMRepository() CustomerRFMRepository {
	return &customerRFMRepo{db: _db.GetDB()}
}

func NewCustomerRFMRepositoryWithDB(db *gorm.DB) CustomerRFMRepository {
	return &customerRFMRepo{db: db}
}

func (r *customerRFMRepo) Upsert(ctx context.Context, rfm *model.CustomerRFM) error {
	if rfm.CustomerID == "" {
		return errors.New("customer_id 不能为空")
	}
	var existing model.CustomerRFM
	err := r.db.WithContext(ctx).Where("customer_id = ?", rfm.CustomerID).First(&existing).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.db.WithContext(ctx).Create(rfm).Error
	}
	rfm.ID = existing.ID
	rfm.CreatedAt = existing.CreatedAt
	rfm.ComputedAt = time.Now()
	// 不用 Save：Save 会把整个结构体写回，包括本层从未赋值的 tenant_id（=0），
	// 那是「更新不可改写 tenant_id」闸门的直接触发点（on 模式整条 RFM 计算 500）。
	// tenant_id 由隔离 callback 在 INSERT 时填、在 UPDATE 时不允许改。
	// updated_at 不进 Select：交给 GORM 的 autoUpdateTime 填当前值，
	// 进了 Select 反而会把结构体里的零值写回去。
	res := r.db.WithContext(ctx).Model(&model.CustomerRFM{}).
		Where("id = ?", existing.ID).
		Select("customer_id", "unified_id",
			"recency_days", "frequency", "monetary_total", "avg_order_value",
			"r_score", "f_score", "m_score", "composite_score", "segment",
			"churn_risk_level", "churn_score", "last_active_at",
			"computed_at").
		Updates(rfm)
	return res.Error
}

func (r *customerRFMRepo) GetByCustomerID(ctx context.Context, customerID string) (*model.CustomerRFM, error) {
	var rfm model.CustomerRFM
	if err := r.db.WithContext(ctx).Where("customer_id = ?", customerID).First(&rfm).Error; err != nil {
		return nil, err
	}
	return &rfm, nil
}

func (r *customerRFMRepo) ListBySegment(ctx context.Context, segment string, page, pageSize int) ([]*model.CustomerRFM, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	var rfms []*model.CustomerRFM
	var total int64
	q := r.db.WithContext(ctx).Model(&model.CustomerRFM{})
	if segment != "" {
		q = q.Where("segment = ?", segment)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := q.Order("composite_score DESC, monetary_total DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&rfms).Error; err != nil {
		return nil, 0, err
	}
	return rfms, total, nil
}

func (r *customerRFMRepo) CountBySegment(ctx context.Context) (map[string]int64, error) {
	type row struct {
		Segment string
		Cnt     int64
	}
	var rows []row
	if err := r.db.WithContext(ctx).Model(&model.CustomerRFM{}).
		Select("segment, COUNT(*) as cnt").Group("segment").Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(rows))
	for _, r := range rows {
		out[r.Segment] = r.Cnt
	}
	return out, nil
}

func (r *customerRFMRepo) DeleteByCustomerID(ctx context.Context, customerID string) error {
	return r.db.WithContext(ctx).Where("customer_id = ?", customerID).Delete(&model.CustomerRFM{}).Error
}

// RecoveryQueueRepository 流失挽回队列仓库
type RecoveryQueueRepository interface {
	Create(ctx context.Context, item *model.RecoveryQueue) error
	// MarkRecovered 按主键白名单写「已挽回」三列（stage/recovered_at/recovery_value）；
	// 命中 0 行即记录不存在（已删或不属于当前租户），返回 gorm.ErrRecordNotFound。
	MarkRecovered(ctx context.Context, id uint64, value int64, at time.Time) error
	// DeferAttempt 只把下次可触达时间推后、不消耗 attempts；命中 0 行同上返回不存在。
	DeferAttempt(ctx context.Context, id uint64, next time.Time) error
	GetByID(ctx context.Context, id uint64) (*model.RecoveryQueue, error)
	GetActiveByCustomerID(ctx context.Context, customerID string) (*model.RecoveryQueue, error)
	ListByStage(ctx context.Context, stage string, page, pageSize int) ([]*model.RecoveryQueue, int64, error)
	ListReadyForAttempt(ctx context.Context, now time.Time, limit int) ([]*model.RecoveryQueue, error)
	MarkAttempt(ctx context.Context, id uint64, channel, result string, nextAt *time.Time) error
	MarkStage(ctx context.Context, id uint64, stage string) error
	CountByStage(ctx context.Context) (map[string]int64, error)
	Delete(ctx context.Context, id uint64) error
}

type recoveryQueueRepo struct {
	db *gorm.DB
}

func NewRecoveryQueueRepository() RecoveryQueueRepository {
	return &recoveryQueueRepo{db: _db.GetDB()}
}

func NewRecoveryQueueRepositoryWithDB(db *gorm.DB) RecoveryQueueRepository {
	return &recoveryQueueRepo{db: db}
}

func (r *recoveryQueueRepo) Create(ctx context.Context, item *model.RecoveryQueue) error {
	if item.CustomerID == "" {
		return errors.New("customer_id 不能为空")
	}
	existing, err := r.GetActiveByCustomerID(ctx, item.CustomerID)
	if err == nil && existing != nil {
		return errors.New("客户已在挽回队列中")
	}
	return r.db.WithContext(ctx).Create(item).Error
}

// MarkRecovered 标记已挽回。
//
// 只写三列而不是整行 Save：整行回写会把调用方手里那份快照的其它字段（含
// attempts/next_attempt_at 这类正在被 worker 改的列）一并覆回去，也会在记录已被
// 保留期清理、或 id 属于别的租户时静默返回成功——客户会一直显示"挽回中"。
func (r *recoveryQueueRepo) MarkRecovered(ctx context.Context, id uint64, value int64, at time.Time) error {
	res := r.db.WithContext(ctx).
		Model(&model.RecoveryQueue{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"stage":          model.RecoveryStageSucceed,
			"recovered_at":   at,
			"recovery_value": value,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// DeferAttempt 推后下次可触达时间（不消耗 attempts）
func (r *recoveryQueueRepo) DeferAttempt(ctx context.Context, id uint64, next time.Time) error {
	res := r.db.WithContext(ctx).
		Model(&model.RecoveryQueue{}).
		Where("id = ?", id).
		Update("next_attempt_at", next)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *recoveryQueueRepo) GetByID(ctx context.Context, id uint64) (*model.RecoveryQueue, error) {
	var item model.RecoveryQueue
	if err := r.db.WithContext(ctx).First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *recoveryQueueRepo) GetActiveByCustomerID(ctx context.Context, customerID string) (*model.RecoveryQueue, error) {
	var item model.RecoveryQueue
	if err := r.db.WithContext(ctx).Where("customer_id = ? AND stage IN ?", customerID,
		[]string{model.RecoveryStageQueued, model.RecoveryStageRunning}).First(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *recoveryQueueRepo) ListByStage(ctx context.Context, stage string, page, pageSize int) ([]*model.RecoveryQueue, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	var items []*model.RecoveryQueue
	var total int64
	q := r.db.WithContext(ctx).Model(&model.RecoveryQueue{})
	if stage != "" {
		q = q.Where("stage = ?", stage)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := q.Order("priority ASC, created_at ASC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *recoveryQueueRepo) ListReadyForAttempt(ctx context.Context, now time.Time, limit int) ([]*model.RecoveryQueue, error) {
	if limit < 1 || limit > 500 {
		limit = 50
	}
	var items []*model.RecoveryQueue
	err := r.db.WithContext(ctx).Where("stage = ? AND attempts < max_attempts AND (next_attempt_at IS NULL OR next_attempt_at <= ?)",
		model.RecoveryStageQueued, now).
		Order("priority ASC, next_attempt_at ASC NULLS FIRST").
		Limit(limit).Find(&items).Error
	return items, err
}

func (r *recoveryQueueRepo) MarkAttempt(ctx context.Context, id uint64, channel, result string, nextAt *time.Time) error {
	updates := map[string]any{
		"attempts":        gorm.Expr("attempts + 1"),
		"last_attempt_at": time.Now(),
		"last_channel":    channel,
		"last_result":     result,
	}
	if nextAt != nil {
		updates["next_attempt_at"] = *nextAt
	}
	return r.db.WithContext(ctx).Model(&model.RecoveryQueue{}).Where("id = ?", id).Updates(updates).Error
}

func (r *recoveryQueueRepo) MarkStage(ctx context.Context, id uint64, stage string) error {
	return r.db.WithContext(ctx).Model(&model.RecoveryQueue{}).Where("id = ?", id).Update("stage", stage).Error
}

func (r *recoveryQueueRepo) CountByStage(ctx context.Context) (map[string]int64, error) {
	type row struct {
		Stage string
		Cnt   int64
	}
	var rows []row
	if err := r.db.WithContext(ctx).Model(&model.RecoveryQueue{}).
		Select("stage, COUNT(*) as cnt").Group("stage").Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(rows))
	for _, r := range rows {
		out[r.Stage] = r.Cnt
	}
	return out, nil
}

func (r *recoveryQueueRepo) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&model.RecoveryQueue{}, id).Error
}
