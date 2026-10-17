package repository

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/tenant"
)

// TenantFilter 租户运营清单过滤条件（contracts/api.md GET /api/manage/tenants）
type TenantFilter struct {
	Page     int
	PageSize int
	// Status 为 nil 表示不过滤状态
	Status  *int16
	Keyword string
}

// TenantActivity 租户活跃度概览（成员数与最近活跃时间）
type TenantActivity struct {
	MemberCount  int64      `json:"member_count"`
	LastActiveAt *time.Time `json:"last_active_at"`
}

// TenantWithActivity 租户行 + 活跃度概览（运营清单展示用）
type TenantWithActivity struct {
	*model.Tenant
	Activity TenantActivity `json:"activity"`
}

// TenantRepository 租户仓储接口（specs/001-saas-multi-tenant T029/T033）
//
// 说明：tenants 表在 tenancyidx 登记表中标记为全局表（豁免隔离），
// 因此租户本体的 CRUD 不需要租户上下文；跨租户聚合（成员数/最近活跃）
// 走 tenant.WithSystemScope 显式绕过隔离注入，属平台运营语义。
type TenantRepository interface {
	Create(ctx context.Context, t *model.Tenant) error
	GetByID(ctx context.Context, id uint) (*model.Tenant, error)
	GetByCode(ctx context.Context, code string) (*model.Tenant, error)
	UpdateStatus(ctx context.Context, id uint, status int16) error
	UpdateRegistration(ctx context.Context, id uint, fields map[string]any) error
	// List 运营清单：分页 + 状态/关键词过滤，每行附活跃度概览
	List(ctx context.Context, f TenantFilter) ([]TenantWithActivity, int64, error)

	// CreateWithAdmin 事务内建租户与首位管理员：全成或全回滚
	//
	// 仅承担原子写入编排（两个 INSERT 同事务），不做业务判断：
	// 重复校验、租户短码生成、状态默认值均由 Service 决定。
	// admin.TenantID 由调用方显式给出（开户事务内新租户 ID 尚不可知，
	// 故不走 callback 上下文填充路径，见 fillCreateForTenant 的显式值放行）。
	CreateWithAdmin(ctx context.Context, t *model.Tenant, admin *model.SystemUser) error
}

type tenantRepo struct {
	db *gorm.DB
}

// NewTenantRepository 构造
func NewTenantRepository() TenantRepository {
	return &tenantRepo{db: db.GetDB()}
}

func (r *tenantRepo) Create(ctx context.Context, t *model.Tenant) error {
	return r.db.WithContext(ctx).Create(t).Error
}

func (r *tenantRepo) GetByID(ctx context.Context, id uint) (*model.Tenant, error) {
	var t model.Tenant
	if err := r.db.WithContext(ctx).First(&t, id).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *tenantRepo) GetByCode(ctx context.Context, code string) (*model.Tenant, error) {
	var t model.Tenant
	if err := r.db.WithContext(ctx).Where("code = ?", code).First(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *tenantRepo) UpdateStatus(ctx context.Context, id uint, status int16) error {
	return r.db.WithContext(ctx).Model(&model.Tenant{}).Where("id = ?", id).
		Update("status", status).Error
}

// UpdateRegistration 局部更新开户登记字段
func (r *tenantRepo) UpdateRegistration(ctx context.Context, id uint, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	fields["updated_at"] = time.Now()
	return r.db.WithContext(ctx).Model(&model.Tenant{}).
		Where("id = ?", id).Updates(fields).Error
}

func (r *tenantRepo) CreateWithAdmin(ctx context.Context, t *model.Tenant, admin *model.SystemUser) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(t).Error; err != nil {
			return err
		}
		admin.TenantID = t.ID
		return tx.Create(admin).Error
	})
}

func (r *tenantRepo) List(ctx context.Context, f TenantFilter) ([]TenantWithActivity, int64, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 || f.PageSize > 200 {
		f.PageSize = 20
	}

	q := r.db.WithContext(ctx).Model(&model.Tenant{})
	if f.Status != nil {
		q = q.Where("status = ?", *f.Status)
	}
	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		// 模糊匹配企业名或短码（转义 LIKE 通配符，避免 % 注入放大匹配面）
		like := "%" + escapeLike(kw) + "%"
		q = q.Where("name ILIKE ? ESCAPE '\\' OR code ILIKE ? ESCAPE '\\'", like, like)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []*model.Tenant
	if err := q.Order("id DESC").
		Offset((f.Page - 1) * f.PageSize).
		Limit(f.PageSize).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	if len(rows) == 0 {
		return nil, total, nil
	}

	acts, err := r.activities(ctx, rows)
	if err != nil {
		return nil, 0, err
	}

	out := make([]TenantWithActivity, 0, len(rows))
	for _, row := range rows {
		out = append(out, TenantWithActivity{Tenant: row, Activity: acts[row.ID]})
	}
	return out, total, nil
}

// activities 批量取租户活跃度：一次分组聚合，避免逐租户 N+1
//
// 读的是其他租户的成员行，属平台运营视角——显式挂 SystemScope 绕过隔离注入。
func (r *tenantRepo) activities(ctx context.Context, rows []*model.Tenant) (map[uint]TenantActivity, error) {
	ids := make([]uint, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	out := make(map[uint]TenantActivity, len(rows))
	if len(ids) == 0 {
		return out, nil
	}

	type agg struct {
		TenantID     uint
		MemberCount  int64
		LastActiveAt *time.Time
	}
	var stats []agg
	sysCtx := tenant.WithSystemScope(ctx)
	if err := r.db.WithContext(sysCtx).Model(&model.SystemUser{}).
		Select("tenant_id, COUNT(*) AS member_count, MAX(last_login) AS last_active_at").
		Where("tenant_id IN ?", ids).
		Group("tenant_id").
		Scan(&stats).Error; err != nil {
		return nil, err
	}
	for _, s := range stats {
		out[s.TenantID] = TenantActivity{MemberCount: s.MemberCount, LastActiveAt: s.LastActiveAt}
	}
	return out, nil
}

// escapeLike 转义 LIKE 模式元字符，配合 ESCAPE '\' 使用
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}
