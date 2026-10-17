package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hivemtk-user/internal/model"

	"hivemtk-user/internal/pkg/db"
)

// TenantModuleRepository 本地已开通模块镜像仓储
type TenantModuleRepository interface {
	UpsertMany(ctx context.Context, rows []*model.TenantModule) error
	ListAll(ctx context.Context) ([]*model.TenantModule, error)
	ListActiveKeys(ctx context.Context, now time.Time) ([]string, error)
	Get(ctx context.Context, moduleKey string) (*model.TenantModule, error)
	DeleteAll(ctx context.Context) error
	CreateLog(ctx context.Context, l *model.ModuleSyncLog) error
	ListLogs(ctx context.Context, limit int) ([]*model.ModuleSyncLog, error)
}

type tenantModuleRepo struct{ db *gorm.DB }

// NewTenantModuleRepository 构造租户模块仓储
func NewTenantModuleRepository() TenantModuleRepository {
	return &tenantModuleRepo{db: db.GetDB()}
}

// NewTenantModuleRepositoryWithDB 带显式 DB（测试注入）
func NewTenantModuleRepositoryWithDB(gdb *gorm.DB) TenantModuleRepository {
	return &tenantModuleRepo{db: gdb}
}

// UpsertMany 幂等落本地镜像
func (r *tenantModuleRepo) UpsertMany(ctx context.Context, rows []*model.TenantModule) error {
	if len(rows) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "tenant_slot"}, {Name: "module_key"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"status", "source", "category", "price_cents", "menu_keys",
			"activated_at", "expires_at", "updated_at",
		}),
	}).Create(&rows).Error
}

func (r *tenantModuleRepo) ListAll(ctx context.Context) ([]*model.TenantModule, error) {
	var out []*model.TenantModule
	err := r.db.WithContext(ctx).Order("module_key ASC").Find(&out).Error
	return out, err
}

func (r *tenantModuleRepo) ListActiveKeys(ctx context.Context, now time.Time) ([]string, error) {
	rows, err := r.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, m := range rows {
		if m.IsUsable(now) {
			out = append(out, m.ModuleKey)
		}
	}
	return out, nil
}

func (r *tenantModuleRepo) Get(ctx context.Context, moduleKey string) (*model.TenantModule, error) {
	var m model.TenantModule
	err := r.db.WithContext(ctx).Where("module_key = ?", moduleKey).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *tenantModuleRepo) DeleteAll(ctx context.Context) error {
	return r.db.WithContext(ctx).Where("1 = 1").Delete(&model.TenantModule{}).Error
}

func (r *tenantModuleRepo) CreateLog(ctx context.Context, l *model.ModuleSyncLog) error {
	return r.db.WithContext(ctx).Create(l).Error
}

func (r *tenantModuleRepo) ListLogs(ctx context.Context, limit int) ([]*model.ModuleSyncLog, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	var out []*model.ModuleSyncLog
	err := r.db.WithContext(ctx).Order("created_at DESC").Limit(limit).Find(&out).Error
	return out, err
}

// MenuKeysOf 解析镜像行的菜单节点列表
func MenuKeysOf(m *model.TenantModule) []string {
	if m == nil || m.MenuKeys == "" {
		return nil
	}
	var keys []string
	if err := json.Unmarshal([]byte(m.MenuKeys), &keys); err != nil {
		return nil
	}
	return keys
}

// ---------------------------------------------------------- 订单镜像

// ModuleOrderRepository 本地订单镜像仓储（只读缓存，无状态机）
type ModuleOrderRepository interface {
	Upsert(ctx context.Context, o *model.ModuleOrder) error
	GetByOrderNo(ctx context.Context, orderNo string) (*model.ModuleOrder, error)
	GetByClientOrderNo(ctx context.Context, clientOrderNo string) (*model.ModuleOrder, error)
	List(ctx context.Context, page, pageSize int) ([]*model.ModuleOrder, int64, error)
}

type moduleOrderRepo struct{ db *gorm.DB }

func NewModuleOrderRepository() ModuleOrderRepository {
	return &moduleOrderRepo{db: db.GetDB()}
}

func NewModuleOrderRepositoryWithDB(gdb *gorm.DB) ModuleOrderRepository {
	return &moduleOrderRepo{db: gdb}
}

func (r *moduleOrderRepo) Upsert(ctx context.Context, o *model.ModuleOrder) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "order_no"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"status", "total_amount_cents", "currency", "items",
			"payment_guide", "paid_at", "updated_at",
		}),
	}).Create(o).Error
}

func (r *moduleOrderRepo) GetByOrderNo(ctx context.Context, orderNo string) (*model.ModuleOrder, error) {
	var o model.ModuleOrder
	err := r.db.WithContext(ctx).Where("order_no = ?", orderNo).First(&o).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (r *moduleOrderRepo) GetByClientOrderNo(ctx context.Context, clientOrderNo string) (*model.ModuleOrder, error) {
	var o model.ModuleOrder
	err := r.db.WithContext(ctx).Where("client_order_no = ?", clientOrderNo).First(&o).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (r *moduleOrderRepo) List(ctx context.Context, page, pageSize int) ([]*model.ModuleOrder, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	var total int64
	q := r.db.WithContext(ctx).Model(&model.ModuleOrder{})
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []*model.ModuleOrder
	err := q.Order("created_at DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&out).Error
	return out, total, err
}
