package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"hivemtk-user/internal/content/model"

	"gorm.io/gorm"
)

// ErrMaterialMissing 文案含「不存在」：出口层按 errhttp 的口径交 404，
// 而不是把隔离闸或 SQL 细节原样回显。
var ErrMaterialMissing = errors.New("素材不存在")

// ErrMaterialCategoryMissing 同上，分类腿。
var ErrMaterialCategoryMissing = errors.New("素材分类不存在")

// ErrMaterialCategoryNotEmpty 分类下仍有子分类或素材：这是业务校验，
// 由服务层包成 utils.ErrInvalidInput 交 400（改前是 Go 预查后伪造一个
// gorm.ErrForeignKeyViolated 上抛 ⇒ 出口 500，而 schema 上根本没有外键约束）。
var ErrMaterialCategoryNotEmpty = errors.New("素材分类下仍有子分类或素材")

// MaterialTypeStat 按类型聚合的一行素材统计。
type MaterialTypeStat struct {
	Type       string `gorm:"column:material_type"`
	Count      int64  `gorm:"column:row_count"`
	TotalSize  int64  `gorm:"column:total_size"`
	TotalUsage int64  `gorm:"column:total_usage"`
}

type MaterialRepository interface {
	Create(ctx context.Context, material *model.Material) error
	GetByID(ctx context.Context, id string) (*model.Material, error)
	GetList(ctx context.Context, licenseID string, categoryID string, materialType string, search string, page int, limit int) ([]*model.Material, int64, error)
	Update(ctx context.Context, material *model.Material) error
	Delete(ctx context.Context, id string) error
	IncrementUsage(ctx context.Context, id string) error
	GetByHash(ctx context.Context, hash string, licenseID string) (*model.Material, error)
	StatsByType(ctx context.Context, licenseID string) ([]MaterialTypeStat, error)
	CountCreatedSince(ctx context.Context, licenseID string, since time.Time) (int64, error)
	CountUsedSince(ctx context.Context, licenseID string, since time.Time) (int64, error)
}

type materialRepo struct {
	db *gorm.DB
}

// NewMaterialRepository 由装配点注入句柄：仓储不自己摸全局 db.GetDB()。
func NewMaterialRepository(db *gorm.DB) MaterialRepository {
	return &materialRepo{db: db}
}

func (r *materialRepo) Create(ctx context.Context, material *model.Material) error {
	return r.db.WithContext(ctx).Create(material).Error
}

// GetByID err 非空时返回 nil：改前回的是零值结构体指针，
// 调用方漏判 err 就拿到一个「ID 为空、URL 为空」的假素材。
func (r *materialRepo) GetByID(ctx context.Context, id string) (*model.Material, error) {
	var material model.Material
	if err := r.db.WithContext(ctx).Preload("Category").Where("id = ?", id).First(&material).Error; err != nil {
		return nil, err
	}
	return &material, nil
}

func (r *materialRepo) GetList(ctx context.Context, licenseID string, categoryID string, materialType string, search string, page int, limit int) ([]*model.Material, int64, error) {
	var materials []*model.Material
	var total int64
	page, limit = normalizeMaterialPaging(page, limit)

	cond := func() *gorm.DB {
		query := r.db.WithContext(ctx).Model(&model.Material{}).Where("license_id = ?", licenseID)
		if categoryID != "" {
			query = query.Where("category_id = ?", categoryID)
		}
		if materialType != "" {
			query = query.Where("type = ?", materialType)
		}
		if search != "" {
			// 通配符当字面量：改前把外部串直接拼进 %…%，
			// 搜 % 等于全库扫，搜 _ 也命中不该命中的行。
			like := "%" + escapeMaterialLike(search) + "%"
			query = query.Where("name LIKE ? OR tags LIKE ? OR description LIKE ?", like, like, like)
		}
		return query
	}

	if err := cond().Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := cond().Preload("Category").Offset((page - 1) * limit).Limit(limit).Order("created_at DESC").Find(&materials).Error
	return materials, total, err
}

// Update 只覆写本行可编辑列，归属列（tenant_id / license_id / user_id）与主键、创建时间一起排除。
//
// 不用 Save：Save 走 INSERT … ON CONFLICT (id) DO UPDATE，主键命中不到行时不是「更新 0 行」而是按
// 结构体快照把整行重新插入——读改写之间素材被别处删掉，一次「更新」就是复活已删的行，连它的
// license_id 一起写回。别租户的行被并进本租户也是同一条路（DO UPDATE 改的是归属列本身）。
//
// ⚠️ 隔离闸拦不住这条：guardOnConflictTenantAssignments 只拒「DO UPDATE 把 tenant_id 改成别的值」，
// 而 Save 生成的正是 tenant_id = excluded.tenant_id，在放行名单里（变异 M2 实测：on 模式全绿）。
// 所以归属只能靠「不用 Save」+ 服务层那条租户自证的读前置，不能指望 upsert 被闸拒。
func (r *materialRepo) Update(ctx context.Context, material *model.Material) error {
	res := r.db.WithContext(ctx).Model(material).
		Select("*").Omit("id", "created_at", "tenant_id", "license_id", "user_id").Updates(material)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrMaterialMissing
	}
	return nil
}

func (r *materialRepo) Delete(ctx context.Context, id string) error {
	res := r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.Material{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrMaterialMissing
	}
	return nil
}

// IncrementUsage 库内自增，不做读改写：usage_count 的并发计次只有
// usage_count = usage_count + 1 这一种写法不丢更新。命中 0 行如实交出缺失错误。
func (r *materialRepo) IncrementUsage(ctx context.Context, id string) error {
	res := r.db.WithContext(ctx).Model(&model.Material{}).Where("id = ?", id).Updates(map[string]any{
		"usage_count":  gorm.Expr("usage_count + 1"),
		"last_used_at": time.Now(),
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrMaterialMissing
	}
	return nil
}

func (r *materialRepo) GetByHash(ctx context.Context, hash string, licenseID string) (*model.Material, error) {
	var material model.Material
	if err := r.db.WithContext(ctx).Where("hash = ? AND license_id = ?", hash, licenseID).First(&material).Error; err != nil {
		return nil, err
	}
	return &material, nil
}

// StatsByType 按类型分桶聚合。统计口不再「取前 1000 行回 Go 里数」：
// 那条实现对第 1001 行起静默少报，总体积与总次数一起少。
func (r *materialRepo) StatsByType(ctx context.Context, licenseID string) ([]MaterialTypeStat, error) {
	var rows []MaterialTypeStat
	err := r.db.WithContext(ctx).Model(&model.Material{}).
		Where("license_id = ?", licenseID).
		Select("type AS material_type, COUNT(*) AS row_count, COALESCE(SUM(size), 0) AS total_size, COALESCE(SUM(usage_count), 0) AS total_usage").
		Group("type").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *materialRepo) CountCreatedSince(ctx context.Context, licenseID string, since time.Time) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.Material{}).
		Where("license_id = ? AND created_at >= ?", licenseID, since).
		Count(&n).Error
	return n, err
}

func (r *materialRepo) CountUsedSince(ctx context.Context, licenseID string, since time.Time) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.Material{}).
		Where("license_id = ? AND last_used_at >= ?", licenseID, since).
		Count(&n).Error
	return n, err
}

// normalizeMaterialPaging 夹紧分页参数。夹紧的实际危害在 limit：limit<=0 会被 GORM 原样
// 发成 LIMIT 0（响应恒空，运维以为「素材没了」），limit 过界则一次把整表拖进一条响应。
// page<=0 在 GORM 里被吞成「无偏移」（变异 M6 实测：不是 PG 报错），不夹紧也照样要修，
// 因为「page=0 回第 1 页」和「page=1 回第 1 页」该有同一个明确口径。
func normalizeMaterialPaging(page, limit int) (int, int) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	return page, limit
}

// escapeMaterialLike 转义 LIKE 模式串里的通配符与转义符本身。
// PG 与 MySQL 的 LIKE 默认转义符都是反斜杠，无需显式 ESCAPE 子句。
func escapeMaterialLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
