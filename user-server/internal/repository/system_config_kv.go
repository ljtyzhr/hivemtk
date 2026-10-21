package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/tenant"
)

// SystemConfigKVRepository 租户级 KV 配置仓储接口。
//
// 租户归属口径（与隔离 callback 互为冗余，故意显式写出）：
//   - on 模式：ctx 没有租户绑定时一律返回 ErrKVTenantContextMissing，绝不回落
//     默认租户——回落会把"漏绑中间件"变成静默写进/读错租户 1 的数据
//   - off 模式：显式使用默认租户（1），与旧版本行为逐字节等价
type SystemConfigKVRepository interface {
	// Available 报告底层 DB 是否可用（nil 探测收敛到 repository 层）。
	Available() bool
	Get(ctx context.Context, key string) (string, error)

	Upsert(ctx context.Context, key, value string) (string, error)

	EnsureTable(ctx context.Context) error
}

// ErrKVStoreUnavailable 未装配数据库句柄时返回。
//
// 为什么要有它而不是安静地当成"没配过"：nil 句柄只可能来自装配顺序不对
// （构造那一刻 db.GetDB() 还是 nil），而"没配过"是一个合法的业务状态、
// 两者在 Get 里都表现为空值。合成一种返回，运维改了配置不生效时无从查起。
var ErrKVStoreUnavailable = errors.New("system_config_kv: 未装配数据库句柄")

// ErrKVTenantContextMissing on 模式下读写租户 KV 却没有租户上下文。
//
// 独立哨兵（不复用参数中心的同名语义）：调用方多为"读不到就回落默认值"的
// 分支，必须能把"库坏了"与"ctx 漏了租户"分开观测——后者是装配缺陷，
// 静默回落会在 SaaS 上表现为"某租户配置永远不生效"。
var ErrKVTenantContextMissing = errors.New("system_config_kv: 多租户模式读写需要租户上下文")

type systemConfigKVRepo struct {
	db *gorm.DB
}

// NewSystemConfigKVRepository 构造。
//
// 注意它捕获的是**调用那一刻**的 db.GetDB()：装配顺序不对时拿到的是 nil。
// 拿到 nil 不再是崩，而是三个方法一律返回 ErrKVStoreUnavailable（调用侧本来
// 就都有"读不到就回落默认值"的分支，所以改成 error 对它们是可观测的、不是新增行为）。
// 需要句柄确定性的装配点用下面那个。
func NewSystemConfigKVRepository() SystemConfigKVRepository {
	return &systemConfigKVRepo{db: db.GetDB()}
}

// NewSystemConfigKVRepositoryWithDB 用显式句柄构造（与商机/报价/审批各仓储的 WithDB 同一口径）。
//
// 存在的理由是装配点的句柄一致性：报价竖的模板与话术指针读的是这张表，装配函数手里已经
// 有一把 db，再走全局句柄就等于"版本行读 A 库、模板读 B 库"这种只在多库环境才暴露的错。
func NewSystemConfigKVRepositoryWithDB(gormDB *gorm.DB) SystemConfigKVRepository {
	return &systemConfigKVRepo{db: gormDB}
}

// Available 报告底层 DB 是否可用。
func (r *systemConfigKVRepo) Available() bool {
	return r != nil && r.db != nil
}

// kvTenantIDFor 解析本次读写应归属的租户。
func kvTenantIDFor(ctx context.Context) (uint, error) {
	if id, ok := tenant.FromContext(ctx); ok {
		return id, nil
	}
	if !tenant.IsMultiTenant() {
		return tenant.DefaultTenantID, nil
	}
	return 0, ErrKVTenantContextMissing
}

func (r *systemConfigKVRepo) Get(ctx context.Context, key string) (string, error) {
	if !r.Available() {
		return "", ErrKVStoreUnavailable
	}
	tenantID, err := kvTenantIDFor(ctx)
	if err != nil {
		return "", err
	}
	var row model.SystemConfigKV
	err = r.db.WithContext(ctx).Where("tenant_id = ? AND key = ?", tenantID, key).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	return row.Value, nil
}

func (r *systemConfigKVRepo) Upsert(ctx context.Context, key, value string) (string, error) {
	if !r.Available() {
		return "", ErrKVStoreUnavailable
	}
	tenantID, err := kvTenantIDFor(ctx)
	if err != nil {
		return "", err
	}
	now := time.Now()
	row := model.SystemConfigKV{
		TenantID:  tenantID,
		Key:       key,
		Value:     value,
		CreatedAt: now,
		UpdatedAt: now,
	}
	upsert := func() error {
		return r.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "tenant_id"}, {Name: "key"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"value":      value,
				"updated_at": now,
			}),
		}).Create(&row).Error
	}
	err = upsert()
	if err != nil {
		if ensureErr := r.EnsureTable(ctx); ensureErr != nil {
			return "", err
		}
		err = upsert()
		if err != nil {
			return "", err
		}
	}
	return value, nil
}

func (r *systemConfigKVRepo) EnsureTable(ctx context.Context) error {
	if !r.Available() {
		return ErrKVStoreUnavailable
	}
	// 原 CREATE TABLE 裸 DDL（T012 清偿）：模型已在 allModels 登记，AutoMigrate 幂等建表
	return r.db.WithContext(ctx).AutoMigrate(&model.SystemConfigKV{})
}

var _ SystemConfigKVRepository = (*systemConfigKVRepo)(nil)
