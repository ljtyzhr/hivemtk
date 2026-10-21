package repository

import (
	"context"
	"time"

	"hivemtk-user/internal/geo/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GeoSourceCatalogRepository 信源目录仓储
type GeoSourceCatalogRepository interface {
	// Upsert 按 (tenant_id, source_url) 原子 upsert；空值字段不覆盖存量（见实现注释）
	Upsert(ctx context.Context, item *model.GeoSourceCatalog) error
	GetByURL(ctx context.Context, url string) (*model.GeoSourceCatalog, error)
	FindByDomain(ctx context.Context, domain string) ([]*model.GeoSourceCatalog, error)
	ListSeedURLs(ctx context.Context) ([]*model.GeoSourceCatalog, error)
	UpdateLastChecked(ctx context.Context, id uint, t time.Time) error
}

type geoSourceCatalogRepo struct{ db *gorm.DB }

func NewGeoSourceCatalogRepository() GeoSourceCatalogRepository {
	return &geoSourceCatalogRepo{db: _db.GetDB()}
}

func NewGeoSourceCatalogRepositoryWithDB(db *gorm.DB) GeoSourceCatalogRepository {
	return &geoSourceCatalogRepo{db: db}
}

// Upsert 一次 SQL 完成 upsert（旧写法 SELECT-then-INSERT，并发下会撞唯一索引报错）。
//
// 冲突时只覆盖**非空**字段：爬虫访问只带 URL 与 domain，若把 level/category/
// description 一起写回，会把人工/AI 整理好的 A/B/C/D 等级清空。
func (r *geoSourceCatalogRepo) Upsert(ctx context.Context, item *model.GeoSourceCatalog) error {
	updates := map[string]any{
		"verified":   item.Verified,
		"updated_at": time.Now(),
	}
	if item.Domain != "" {
		updates["domain"] = item.Domain
	}
	if item.Level != "" {
		updates["level"] = item.Level
	}
	if item.Category != "" {
		updates["category"] = item.Category
	}
	if item.Description != "" {
		updates["description"] = item.Description
	}
	if !item.LastChecked.IsZero() {
		updates["last_checked"] = item.LastChecked
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "source_url"}},
		DoUpdates: clause.Assignments(updates),
	}).Create(item).Error
}

func (r *geoSourceCatalogRepo) GetByURL(ctx context.Context, url string) (*model.GeoSourceCatalog, error) {
	var m model.GeoSourceCatalog
	err := r.db.WithContext(ctx).Where("source_url = ?", url).First(&m).Error
	return &m, err
}

func (r *geoSourceCatalogRepo) FindByDomain(ctx context.Context, domain string) ([]*model.GeoSourceCatalog, error) {
	var list []*model.GeoSourceCatalog
	err := r.db.WithContext(ctx).
		Where("domain = ? AND verified = true", domain).
		Order("level ASC").
		Find(&list).Error
	return list, err
}

func (r *geoSourceCatalogRepo) ListSeedURLs(ctx context.Context) ([]*model.GeoSourceCatalog, error) {
	var list []*model.GeoSourceCatalog
	err := r.db.WithContext(ctx).
		Where("verified = true").
		Order("level ASC, updated_at DESC").
		Find(&list).Error
	return list, err
}

// UpdateLastChecked 刷新探测时间；0 行说明条目不存在（含跨租户），不能当作成功。
func (r *geoSourceCatalogRepo) UpdateLastChecked(ctx context.Context, id uint, t time.Time) error {
	res := r.db.WithContext(ctx).Model(&model.GeoSourceCatalog{}).
		Where("id = ?", id).Update("last_checked", t)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
