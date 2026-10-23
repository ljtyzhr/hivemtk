// feature_flag.go FeatureFlag 仓储（K2 五层 L4）
package repository

import (
	"context"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/tenant"
)

// FeatureFlagRepository FeatureFlag 仓储接口
type FeatureFlagRepository interface {
	Create(ctx context.Context, f *model.FeatureFlag) error
	Update(ctx context.Context, f *model.FeatureFlag) error
	Delete(ctx context.Context, id uint) error
	GetByID(ctx context.Context, id uint) (*model.FeatureFlag, error)
	GetByKey(ctx context.Context, key string) (*model.FeatureFlag, error)
	List(ctx context.Context, page, pageSize int) ([]model.FeatureFlag, int64, error)
	ListStale(ctx context.Context, olderThan time.Time) ([]model.FeatureFlag, error)
	CreateAudit(ctx context.Context, a *model.FeatureFlagAuditLog) error
	ListAudit(ctx context.Context, flagID uint, limit int) ([]model.FeatureFlagAuditLog, error)
	CreateEvalLog(ctx context.Context, e *model.FeatureFlagEvalLog) error
	ListEvalLogs(ctx context.Context, flagKey string, limit int) ([]model.FeatureFlagEvalLog, error)
	TouchEvaluated(ctx context.Context, id uint) error
}

type featureFlagRepo struct {
	db *gorm.DB
}

// NewFeatureFlagRepository 构造
func NewFeatureFlagRepository(db *gorm.DB) FeatureFlagRepository {
	return &featureFlagRepo{db: db}
}

// NewFeatureFlagRepositoryFromGlobal 便捷构造（内部调用 db.GetDB()）
func NewFeatureFlagRepositoryFromGlobal() FeatureFlagRepository {
	return NewFeatureFlagRepository(_db.GetDB())
}

func (r *featureFlagRepo) Create(ctx context.Context, f *model.FeatureFlag) error {
	if f.TenantID == 0 {
		f.TenantID = scopeTenantID(ctx)
	}
	return r.db.WithContext(ctx).Create(f).Error
}

// scopeTenantID 取本次写入的租户归属：有绑定用绑定；离线单机显式落默认租户
// （不依赖列默认值，测试与直连仓储的调用方都能看到真实归属）；on 模式无绑定
// 返回 0，由隔离 callback fail-closed —— 绝不静默写进默认租户。
func scopeTenantID(ctx context.Context) uint {
	if id, ok := tenant.FromContext(ctx); ok {
		return id
	}
	if !tenant.IsMultiTenant() {
		return tenant.DefaultTenantID
	}
	return 0
}

func (r *featureFlagRepo) Update(ctx context.Context, f *model.FeatureFlag) error {
	return saveWholeRow(r.db.WithContext(ctx), f)
}

func (r *featureFlagRepo) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&model.FeatureFlag{}, id).Error
}

func (r *featureFlagRepo) GetByID(ctx context.Context, id uint) (*model.FeatureFlag, error) {
	var f model.FeatureFlag
	if err := r.db.WithContext(ctx).First(&f, id).Error; err != nil {
		return nil, err
	}
	return &f, nil
}

func (r *featureFlagRepo) GetByKey(ctx context.Context, key string) (*model.FeatureFlag, error) {
	var f model.FeatureFlag
	if err := r.db.WithContext(ctx).Where("\"key\" = ?", key).First(&f).Error; err != nil {
		return nil, err
	}
	return &f, nil
}

func (r *featureFlagRepo) List(ctx context.Context, page, pageSize int) ([]model.FeatureFlag, int64, error) {
	var list []model.FeatureFlag
	var total int64
	q := r.db.WithContext(ctx).Model(&model.FeatureFlag{})
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 50
	}
	err := q.Order("updated_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error
	return list, total, err
}

func (r *featureFlagRepo) ListStale(ctx context.Context, olderThan time.Time) ([]model.FeatureFlag, error) {
	var list []model.FeatureFlag
	err := r.db.WithContext(ctx).
		Where("updated_at < ?", olderThan).
		Where("rollout_percentage IN (0, 100)").
		Order("updated_at ASC").
		Limit(100).
		Find(&list).Error
	return list, err
}

func (r *featureFlagRepo) CreateAudit(ctx context.Context, a *model.FeatureFlagAuditLog) error {
	if a.TenantID == 0 {
		a.TenantID = scopeTenantID(ctx)
	}
	return r.db.WithContext(ctx).Create(a).Error
}

func (r *featureFlagRepo) ListAudit(ctx context.Context, flagID uint, limit int) ([]model.FeatureFlagAuditLog, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var list []model.FeatureFlagAuditLog
	err := r.db.WithContext(ctx).
		Where("flag_id = ?", flagID).
		Order("created_at DESC").
		Limit(limit).
		Find(&list).Error
	return list, err
}

func (r *featureFlagRepo) CreateEvalLog(ctx context.Context, e *model.FeatureFlagEvalLog) error {
	if e.TenantID == 0 {
		e.TenantID = scopeTenantID(ctx)
	}
	return r.db.WithContext(ctx).Create(e).Error
}

func (r *featureFlagRepo) ListEvalLogs(ctx context.Context, flagKey string, limit int) ([]model.FeatureFlagEvalLog, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var list []model.FeatureFlagEvalLog
	err := r.db.WithContext(ctx).
		Where("flag_key = ?", flagKey).
		Order("created_at DESC").
		Limit(limit).
		Find(&list).Error
	return list, err
}

func (r *featureFlagRepo) TouchEvaluated(ctx context.Context, id uint) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&model.FeatureFlag{}).
		Where("id = ?", id).
		UpdateColumn("last_evaluated_at", &now).Error
}
