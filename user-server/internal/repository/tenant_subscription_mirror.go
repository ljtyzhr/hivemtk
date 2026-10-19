package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
)

// TenantSubscriptionMirrorRepository 订阅镜像仓储（部署级单行）。
//
// 表无 tenant_id（003 修订 #4：sync 跑 SystemScope，带 tenant_id 必踩默认
// 租户串写坑）—— 读写直接走 DB，不经租户隔离 callback 的过滤列改写。
type TenantSubscriptionMirrorRepository interface {
	// Get 读单行；无行返回 (nil, nil)（fail-open 判定的输入）。
	Get(ctx context.Context) (*model.TenantSubscriptionMirror, error)
	// Upsert 幂等写单行（按 id=1 ON CONFLICT 更新）。
	Upsert(ctx context.Context, row *model.TenantSubscriptionMirror) error
}

type tenantSubscriptionMirrorRepo struct{ db *gorm.DB }

// NewTenantSubscriptionMirrorRepository 构造（按次取全局 DB，理由同 TenantModuleRepository）
func NewTenantSubscriptionMirrorRepository() TenantSubscriptionMirrorRepository {
	return &tenantSubscriptionMirrorRepo{db: db.GetDB()}
}

// NewTenantSubscriptionMirrorRepositoryWithDB 带显式 DB（测试注入）
func NewTenantSubscriptionMirrorRepositoryWithDB(gdb *gorm.DB) TenantSubscriptionMirrorRepository {
	return &tenantSubscriptionMirrorRepo{db: gdb}
}

func (r *tenantSubscriptionMirrorRepo) Get(ctx context.Context) (*model.TenantSubscriptionMirror, error) {
	if r.db == nil {
		return nil, errors.New("db is nil")
	}
	var row model.TenantSubscriptionMirror
	err := r.db.WithContext(ctx).Where("id = ?", model.MirrorRowID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *tenantSubscriptionMirrorRepo) Upsert(ctx context.Context, row *model.TenantSubscriptionMirror) error {
	if r.db == nil {
		return errors.New("db is nil")
	}
	if row == nil {
		return errors.New("row is nil")
	}
	row.ID = model.MirrorRowID
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"status", "current_period_end", "trial_expires_at", "price_cents", "fetched_at", "updated_at",
		}),
	}).Create(row).Error
}
