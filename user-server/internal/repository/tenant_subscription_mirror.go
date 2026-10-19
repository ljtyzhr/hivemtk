package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/tenant"
)

// TenantSubscriptionMirrorRepository 订阅镜像仓储（每租户一行，id=tenant_id）。
//
// 多租户修正（003 生产联调）：001 迁移给全表加了 tenant_id 且 tenancyidx 注入
// 生效 —— 旧行锚 id=1 在第二个租户出现后读写全错。行锚改为 ctx 租户 id，
// off 模式回落 DefaultTenantID=1 与旧版逐字节一致。
type TenantSubscriptionMirrorRepository interface {
	// Get 读当前租户单行；无行返回 (nil, nil)（fail-open 判定的输入）。
	Get(ctx context.Context) (*model.TenantSubscriptionMirror, error)
	// Upsert 幂等写当前租户单行（按 id=tenant_id ON CONFLICT 更新）。
	Upsert(ctx context.Context, row *model.TenantSubscriptionMirror) error
}

// mirrorTid 行锚租户：ctx 绑定优先，无绑定（off 模式 / SystemScope）回落默认租户。
func mirrorTid(ctx context.Context) uint {
	if id, ok := tenant.FromContext(ctx); ok && id > 0 {
		return id
	}
	return tenant.DefaultTenantID
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
	err := r.db.WithContext(ctx).Where("id = ?", mirrorTid(ctx)).First(&row).Error
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
	tid := mirrorTid(ctx)
	row.ID = tid
	row.TenantID = tid
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"status", "current_period_end", "trial_expires_at", "price_cents", "fetched_at", "updated_at",
		}),
	}).Create(row).Error
}
