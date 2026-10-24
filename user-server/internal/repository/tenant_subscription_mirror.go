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
// off 模式回落 DefaultTenantID=1 与旧版一致。
type TenantSubscriptionMirrorRepository interface {
	// Get 读当前租户单行；无行返回 (nil, nil)（fail-open 判定的输入）。
	Get(ctx context.Context) (*model.TenantSubscriptionMirror, error)
	// Upsert 幂等写当前租户单行（按 id=tenant_id ON CONFLICT 更新）。
	Upsert(ctx context.Context, row *model.TenantSubscriptionMirror) error
}

// ErrMirrorTenantContextMissing on 模式下既无租户绑定、又不在系统作用域。
//
// 后台同步/启动期走 SystemScope（单部署刷新默认租户那一行），租户请求走绑定；
// 只剩「漏绑中间件」这一种情况——此时回落默认租户会静默读写租户 1 的订阅，
// 表现为某些租户的续费墙永不出现，所以必须报错而不是 fail-open。
var ErrMirrorTenantContextMissing = errors.New("tenant_subscription_mirror: 多租户模式需要租户上下文或系统作用域")

// ErrMirrorStoreUnavailable 仓储句柄缺失（装配错，不是「没数据」）。
//
// 与「查不到行」严格分开：Get 的「无行」回 (nil, nil)（fail-open 判定的输入），
// 而句柄缺失必须报错 —— 合成一种的话，续费墙在装配错误的部署里会稳定地
// 「查不到订阅」⇒ 永久放行，且日志里没有任何异常。
var ErrMirrorStoreUnavailable = errors.New("tenant_subscription_mirror: 仓储未初始化（DB 未就绪）")

// mirrorTid 行锚租户：ctx 绑定优先（SystemScope 不剥 value，租户内手动同步仍落本租户）；
// 无绑定仅允许 SystemScope 与 off 模式回落默认租户。
func mirrorTid(ctx context.Context) (uint, error) {
	if id, ok := tenant.FromContext(ctx); ok && id > 0 {
		return id, nil
	}
	if tenant.IsSystemScope(ctx) {
		return tenant.DefaultTenantID, nil
	}
	if !tenant.IsMultiTenant() {
		return tenant.DefaultTenantID, nil
	}
	return 0, ErrMirrorTenantContextMissing
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
	if r == nil || r.db == nil {
		return nil, ErrMirrorStoreUnavailable
	}
	tid, err := mirrorTid(ctx)
	if err != nil {
		return nil, err
	}
	var row model.TenantSubscriptionMirror
	err = r.db.WithContext(ctx).Where("id = ?", tid).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *tenantSubscriptionMirrorRepo) Upsert(ctx context.Context, row *model.TenantSubscriptionMirror) error {
	if r == nil || r.db == nil {
		return ErrMirrorStoreUnavailable
	}
	if row == nil {
		return errors.New("tenant_subscription_mirror: nil 不是写入对象")
	}
	tid, err := mirrorTid(ctx)
	if err != nil {
		return err
	}
	row.ID = tid
	row.TenantID = tid
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"status", "current_period_end", "trial_expires_at", "price_cents", "fetched_at", "updated_at",
		}),
	}).Create(row).Error
}
