package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hivemtk-user/internal/model"

	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/utils/logger"
)

// TenantModuleRepository 本地已开通模块镜像仓储
type TenantModuleRepository interface {
	UpsertMany(ctx context.Context, rows []*model.TenantModule) error
	ListAll(ctx context.Context) ([]*model.TenantModule, error)
	// UsableCount 统计「当前 ctx 作用域内」某模块的可用行数（FR-5.6 运行时模块闸用）。
	// 跨租户并集语义由调用方挂 SystemScope 达成（任一租户开通判定，见
	// service.ModuleGateService.AnyTenantEnabled）—— 本方法只负责单次索引计数，
	// 不解释租户语义。
	UsableCount(ctx context.Context, moduleKey string, now time.Time) (int64, error)
	// Get 取单个模块镜像行；无行返回 (nil, nil)。
	//
	// 保留理由：全仓唯一调用方是 module_gate_test.go 对「离线兜底是否真把
	// geo 落成一行」的单行断言。它读的是**一行**，用 ListAll 去整表捞一行
	// 不是等价的最小读面（镜像表虽然小，但把"取一个键"写成"取全表再筛"
	// 会让将来镜像变大时的读放大埋进测试里）。这是本文件唯一被保留的
	// 测试独占读口。
	Get(ctx context.Context, moduleKey string) (*model.TenantModule, error)
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
//
// 双重保险（第一层是 moduleidx.Normalize 的启动期索引规范化）：
// ON CONFLICT 失败时退回「先查后写」。
//
// 为什么必须有这一层兜底：ON CONFLICT 依赖索引的**精确列集**，而
// AutoMigrate 不会删除形状已变的索引。任何一次模型演进改了索引列集，
// 老库就会留下旧形状的同名索引，PG 按名字选它 → 42P10。
//
// 而这里失败的后果不是"同步失败"这么轻：同步失败 → tenant_modules 恒空 →
// 模块闸落在「首启全放行」态 → **付费模块对所有人开放**，界面无任何异样。
// 也就是说：一个索引形状问题 = 商业模式整体静默失效。
// 慢路径的正确性不依赖任何索引形状，因此它是最后一道、也是唯一一道
// 不依赖数据库状态的安全网。
//
// tenant_id 的落值（on 模式核对过的行为，勿凭直觉改）：行来自平台下发载荷，
// 不带 tenant_id；调用方（ModuleCatalogService.Sync）在 on 模式挂的是
// SystemScope，隔离 callback 此时不填充 —— 由 GORM 自己补：列带 `default:1`
// → DefaultValueInterface=1 → 值为零时 GORM 把该默认值写进 INSERT 语句
// （gorm callbacks/create.go ConvertToCreateValues），行落默认租户 1，
// 且 ON CONFLICT 的 (tenant_id, module_key) 目标拿到的是同一个值。
// off 模式走的同一条路径，直通逐字节等价 —— 故这里不需要任何模式分支。
func (r *tenantModuleRepo) UpsertMany(ctx context.Context, rows []*model.TenantModule) error {
	if r == nil || r.db == nil {
		return errors.New("[module] tenant_modules 仓储未初始化（DB 未就绪）")
	}
	if len(rows) == 0 {
		return nil
	}
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		// tenant_id 在前是**仓库租户索引策略的产物**（tenancyidx 把租户表的唯一索引
		// 复合化为 (tenant_id, 原列)），不是这里能自己决定的形状。
		// 写成 (module_key) 或 (tenant_slot, module_key) 都会与库里实际的索引
		// 匹配不上 → 42P10 → 模块下发全量失败 → 闸恒放行。
		Columns: []clause.Column{{Name: "tenant_id"}, {Name: "module_key"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"status", "source", "category", "price_cents", "menu_keys",
			"activated_at", "expires_at", "updated_at",
		}),
	}).Create(&rows).Error
	if err == nil {
		return nil
	}
	// 42P10 = ON CONFLICT 匹配不上唯一约束 → 走慢路径
	if !isConflictTargetErr(err) {
		return err
	}
	logger.Warnf("[module] ON CONFLICT 不可用（索引形状与模型不符），退回先查后写: %v", err)
	return r.upsertSlow(ctx, rows)
}

// upsertSlow 先查后写的兜底路径（不依赖任何唯一索引）
func (r *tenantModuleRepo) upsertSlow(ctx context.Context, rows []*model.TenantModule) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, row := range rows {
			var existing model.TenantModule
			err := tx.Where("module_key = ?", row.ModuleKey).First(&existing).Error
			switch {
			case err == nil:
				existing.Status = row.Status
				existing.Source = row.Source
				existing.Category = row.Category
				existing.PriceCents = row.PriceCents
				existing.MenuKeys = row.MenuKeys
				existing.ActivatedAt = row.ActivatedAt
				existing.ExpiresAt = row.ExpiresAt
				existing.UpdatedAt = time.Now()
				// 走 db.SaveWholeRow 而不是裸 Save：这条 UPDATE 命中 0 行说明
				// 镜像行在上面的 First 之后被删掉了（并发的重建镜像/手工清理），
				// 裸 Save 会回退成 INSERT 把它按原 id 插回来 —— 而这条慢路径
				// 存在的全部意义就是「不依赖库里的索引形状把镜像写对」，顺手
				// 造一条幽灵行只会让下一次同步多一条对不上的记录。
				if err := db.SaveWholeRow(tx, &existing); err != nil {
					return err
				}
			case errors.Is(err, gorm.ErrRecordNotFound):
				if err := tx.Create(row).Error; err != nil {
					return err
				}
			default:
				return err
			}
		}
		return nil
	})
}

// isConflictTargetErr 判定是否为 ON CONFLICT 目标不匹配（42P10）
func isConflictTargetErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "there is no unique or exclusion constraint") ||
		strings.Contains(msg, "42p10")
}

// ListAll 列出本地模块镜像全量行（不分页：这是"每模块一行"的表，
// 数量由平台模块目录决定，几十行；截断会让某些模块从管理页上消失）。
func (r *tenantModuleRepo) ListAll(ctx context.Context) ([]*model.TenantModule, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("[module] tenant_modules 仓储未初始化（DB 未就绪）")
	}
	var out []*model.TenantModule
	err := r.db.WithContext(ctx).Order("module_key ASC").Find(&out).Error
	return out, err
}

// UsableCount 统计某模块的可用行数（status=active 且未过期），语义与
// model.TenantModuleUsable 逐条对齐 —— 两处口径必须一起改（勿只改一侧）。
//
// nil-db 守卫：FR-5.6 的后台判定会在测试/装配早期拿到未初始化的连接，
// 这里宁可报错让上层 fail-open，也不能让 nil *gorm.DB 直接 panic 击穿
// 定时任务循环（见 service/module_runtime_filter.go 的 fail-open 教义）。
func (r *tenantModuleRepo) UsableCount(ctx context.Context, moduleKey string, now time.Time) (int64, error) {
	if moduleKey == "" {
		return 0, nil
	}
	if r == nil || r.db == nil {
		return 0, errors.New("[module] tenant_modules 仓储未初始化（DB 未就绪）")
	}
	var n int64
	err := r.db.WithContext(ctx).Model(&model.TenantModule{}).
		Where("module_key = ?", moduleKey).
		Where("status = ?", model.TenantModuleActive).
		Where("expires_at IS NULL OR expires_at > ?", now).
		Count(&n).Error
	if err != nil {
		return 0, err
	}
	return n, nil
}

func (r *tenantModuleRepo) Get(ctx context.Context, moduleKey string) (*model.TenantModule, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("[module] tenant_modules 仓储未初始化（DB 未就绪）")
	}
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

func (r *tenantModuleRepo) CreateLog(ctx context.Context, l *model.ModuleSyncLog) error {
	if r == nil || r.db == nil {
		return errors.New("[module] tenant_modules 仓储未初始化（DB 未就绪）")
	}
	if l == nil {
		return errors.New("[module] 同步日志: nil 不是写入对象")
	}
	return r.db.WithContext(ctx).Create(l).Error
}

func (r *tenantModuleRepo) ListLogs(ctx context.Context, limit int) ([]*model.ModuleSyncLog, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("[module] tenant_modules 仓储未初始化（DB 未就绪）")
	}
	// limit 夹取在仓储侧：GORM 的 `Limit(0)` 等于**不加 limit**，而同步日志是
	// 每次同步都追加的流水（心跳周期级），一次误传 0 就把整张表读进内存。
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
	if r == nil || r.db == nil {
		return errors.New("[module] module_orders 仓储未初始化（DB 未就绪）")
	}
	if o == nil {
		return errors.New("[module] 订单镜像: nil 不是写入对象")
	}
	// ID 是 varchar 主键：镜像落库不赋 ID 会插入空串主键，第二单必撞 pkey
	// （003 活体联调发现）。用 order_no 充当镜像主键，天然幂等。
	if strings.TrimSpace(o.ID) == "" {
		o.ID = o.OrderNo
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "tenant_id"}, {Name: "order_no"}},
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
	// Count 与 Find 各起一条 query：同一条链上先 Count 再 Find 会就地改 Statement
	// （判例 #133 / #150）。
	base := func() *gorm.DB {
		return r.db.WithContext(ctx).Model(&model.ModuleOrder{})
	}
	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []*model.ModuleOrder
	err := base().Order("created_at DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&out).Error
	return out, total, err
}
