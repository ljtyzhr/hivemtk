package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"
)

// GlossaryRepository 术语表仓储。
type GlossaryRepository struct {
	db *gorm.DB
}

// NewGlossaryRepository 创建术语表仓库实例（绑定全局默认 DB）。
func NewGlossaryRepository() *GlossaryRepository {
	return &GlossaryRepository{db: _db.GetDB()}
}

// NewGlossaryRepositoryWithDB 创建指定 DB 的术语表仓库实例（用于依赖注入 / 测试）。
// db 为 nil 时回退全局 DB。
func NewGlossaryRepositoryWithDB(db *gorm.DB) *GlossaryRepository {
	if db == nil {
		return &GlossaryRepository{db: _db.GetDB()}
	}
	return &GlossaryRepository{db: db}
}

// SetDB 注入 db（与现有 repository 风格保持一致）。
func (r *GlossaryRepository) SetDB(_ context.Context, db *gorm.DB) {
	if db != nil {
		r.db = db
	}
}

// GetDB 返回内部 db（用于测试与子事务透传）。
func (r *GlossaryRepository) GetDB(ctx context.Context) *gorm.DB {
	if r.db == nil {
		return nil
	}
	return r.db.WithContext(ctx)
}

// Create 创建术语。
func (r *GlossaryRepository) Create(ctx context.Context, g *model.Glossary) error {
	if g == nil {
		return errors.New("glossary: nil entity")
	}
	now := time.Now()
	if g.CreatedAt.IsZero() {
		g.CreatedAt = now
	}
	g.UpdatedAt = now
	return r.db.WithContext(ctx).Create(g).Error
}

// Update 更新术语（按 ID 主键）。
//
// 不用 Save：Save 把**整个结构体**写回，包括本层从未赋值的 tenant_id（=0）。
// 后果有两处，都实测过：
//   - 离线单机（off）档：隔离 callback 不注入 ⇒ 行带着 tenant_id=0 落库 ⇒
//     那不属于任何租户，**默认租户自己也读不到**（实测：更新一次术语后
//     select tenant_id from glossaries where term_id=… 得到 0）。
//   - SaaS（on）档：撞「更新不可改写 tenant_id」闸门 ⇒ 整条更新 500。
//
// 改成「按主键定向更新 + 显式列出业务列」：tenant_id 由隔离 callback 负责
// （INSERT 填、UPDATE 不许改），updated_at 交给 GORM 的 autoUpdateTime。
func (r *GlossaryRepository) Update(ctx context.Context, g *model.Glossary) error {
	if g == nil {
		return errors.New("glossary: nil entity")
	}
	if g.ID == 0 {
		return errors.New("glossary: id required")
	}
	g.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Model(&model.Glossary{}).
		Where("id = ?", g.ID).
		Select("term_id", "category", "preserve", "translations", "pattern", "status").
		Updates(g).Error
}

// Delete 软删除（按 TermID）。Status 置 "disabled"，DeletedAt 置当前时间。
//
// 注意：本方法依赖 gorm.DeletedAt 字段；若 model.Glossary 未启用软删除，
// 调用方应改用物理删除（当前实现兼容两种 —— 优先 Save 整行）。
func (r *GlossaryRepository) Delete(ctx context.Context, termID string) error {
	if termID == "" {
		return errors.New("glossary: empty term_id")
	}
	return r.db.WithContext(ctx).
		Where("term_id = ?", termID).
		Delete(&model.Glossary{}).Error
}

// GetByTermID 按 TermID 查询单条术语。
func (r *GlossaryRepository) GetByTermID(ctx context.Context, termID string) (*model.Glossary, error) {
	if termID == "" {
		return nil, errors.New("glossary: empty term_id")
	}
	var g model.Glossary
	if err := r.db.WithContext(ctx).Where("term_id = ?", termID).First(&g).Error; err != nil {
		return nil, err
	}
	return &g, nil
}

// GetByID 按主键 ID 查询单条术语。
func (r *GlossaryRepository) GetByID(ctx context.Context, id int64) (*model.Glossary, error) {
	var g model.Glossary
	if err := r.db.WithContext(ctx).First(&g, id).Error; err != nil {
		return nil, err
	}
	return &g, nil
}

// ListActive 查询所有 status=active 的术语（按 CreatedAt 升序，保证缓存稳定）。
//
// service.GlossaryService.LoadByLang 通过此方法加载全量术语后构建视图。
func (r *GlossaryRepository) ListActive(ctx context.Context) ([]*model.Glossary, error) {
	var list []*model.Glossary
	err := r.db.WithContext(ctx).
		Where("status = ?", "active").
		Order("created_at ASC, id ASC").
		Find(&list).Error
	if err != nil {
		return nil, err
	}
	return list, nil
}

// ListByCategory 按分类查询 active 术语。
//
// category 取值：brand / sku / logistic / policy / other。
func (r *GlossaryRepository) ListByCategory(ctx context.Context, category string) ([]*model.Glossary, error) {
	if category == "" {
		return nil, errors.New("glossary: empty category")
	}
	var list []*model.Glossary
	err := r.db.WithContext(ctx).
		Where("status = ? AND category = ?", "active", category).
		Order("created_at ASC, id ASC").
		Find(&list).Error
	if err != nil {
		return nil, err
	}
	return list, nil
}

// ListAll 查询全部术语（含 disabled），用于管理后台分页查询。
//
// status 为空时不过滤；keyword 匹配 TermID（前缀）。
func (r *GlossaryRepository) ListAll(ctx context.Context, status, keyword string, page, pageSize int) ([]*model.Glossary, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	q := r.db.WithContext(ctx).Model(&model.Glossary{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if keyword != "" {
		q = q.Where("term_id LIKE ?", keyword+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []*model.Glossary
	if err := q.
		Order("created_at DESC, id DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}
