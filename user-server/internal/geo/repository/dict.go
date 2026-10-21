package repository

import (
	"context"
	"fmt"
	"time"

	"hivemtk-user/internal/geo/model"
	_db "hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/tenant"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GeoDictRepository GEO 通用配置字典
type GeoDictRepository interface {
	// GetAny 按类目+键取单条（含停用，用于区分“缺行”与“已停用”，避免停用被自动播种复活）
	GetAny(ctx context.Context, category, key string) (*model.GeoDict, error)
	// ListByCategory 按类目列出全部启用条目（sort 升序）
	ListByCategory(ctx context.Context, category string) ([]*model.GeoDict, error)
	// Upsert 按 (tenant_id, category, key) 幂等写入
	Upsert(ctx context.Context, d *model.GeoDict) error
	// Delete 按类目+键删除
	Delete(ctx context.Context, category, key string) error
}

type geoDictRepo struct {
	db *gorm.DB
}

// NewGeoDictRepository 构造 repository（使用默认 DB）
func NewGeoDictRepository() GeoDictRepository {
	return &geoDictRepo{db: _db.GetDB()}
}

// NewGeoDictRepositoryWithDB 构造 repository（使用指定 DB，用于路由注入和测试）
func NewGeoDictRepositoryWithDB(db *gorm.DB) GeoDictRepository {
	return &geoDictRepo{db: db}
}

// dbh 解析本次调用实际使用的 DB：构造时注入的优先；未注入（或注入时全局 DB
// 尚未就绪）则按调用时点重取全局，容忍 globalDict 首次触达早于 DB 初始化；
// 两者皆空返回 ErrInvalidDB，让上层 DictService 走既定的 fail-open 回缺省
// 而不是 panic（DictService.DictJSON 文档承诺"DB 故障 fail-open"）。
func (r *geoDictRepo) dbh(ctx context.Context) (*gorm.DB, error) {
	db := r.db
	if db == nil {
		db = _db.GetDB()
	}
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	return db.WithContext(ctx), nil
}

func (r *geoDictRepo) GetAny(ctx context.Context, category, key string) (*model.GeoDict, error) {
	db, err := r.dbh(ctx)
	if err != nil {
		return nil, err
	}
	var d model.GeoDict
	if err := db.Where("category = ? AND \"key\" = ?", category, key).First(&d).Error; err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *geoDictRepo) ListByCategory(ctx context.Context, category string) ([]*model.GeoDict, error) {
	db, err := r.dbh(ctx)
	if err != nil {
		return nil, err
	}
	var list []*model.GeoDict
	if err := db.Where("category = ? AND active = ?", category, true).Order("\"sort\" ASC, \"key\" ASC").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (r *geoDictRepo) Upsert(ctx context.Context, d *model.GeoDict) error {
	if d == nil {
		return nil
	}
	db, err := r.dbh(ctx)
	if err != nil {
		return err
	}
	// 原生 Exec 的历史原因（GORM 结构体 Create 的 default:true 零值吞写）对
	// map 形态 Create 不成立：map 绑定按值落列，false 就是 false（T012 清偿）。
	// BeforeCreate 在 Table+Create(map) 下不触发，显式补 UUID（与 model 钩子一致）。
	if d.ID == "" {
		d.ID = uuid.NewString()
	}
	// map 形态的 Create 拿不到模型 Schema，隔离 callback 无法填 tenant_id；
	// 必须显式取 ctx 绑定并写进列，否则 SaaS 下所有字典行都会落列默认值 1。
	tenantID, ok := tenant.FromContext(ctx)
	if !ok {
		if tenant.IsMultiTenant() {
			return fmt.Errorf("写入 geo_dicts 缺少租户上下文")
		}
		tenantID = tenant.DefaultTenantID
	}
	now := time.Now()
	return db.Table("geo_dicts").
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "tenant_id"}, {Name: "category"}, {Name: "key"}},
			DoUpdates: clause.Assignments(map[string]any{
				"value":      d.Value,
				"remark":     d.Remark,
				"active":     d.Active,
				"sort":       d.Sort,
				"updated_at": now,
			}),
		}).
		Create(map[string]any{
			"id":         d.ID,
			"tenant_id":  tenantID,
			"category":   d.Category,
			"key":        d.Key,
			"value":      d.Value,
			"remark":     d.Remark,
			"active":     d.Active,
			"sort":       d.Sort,
			"created_at": now,
			"updated_at": now,
		}).Error
}

func (r *geoDictRepo) Delete(ctx context.Context, category, key string) error {
	db, err := r.dbh(ctx)
	if err != nil {
		return err
	}
	res := db.Where("category = ? AND \"key\" = ?", category, key).Delete(&model.GeoDict{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
