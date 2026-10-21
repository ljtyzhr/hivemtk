package repository

import (
	"context"

	"hivemtk-user/internal/geo/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// GeoCompetitorRepository 竞品仓储
type GeoCompetitorRepository interface {
	List(ctx context.Context, status string) ([]*model.GeoCompetitor, error)
	GetByID(ctx context.Context, id uint) (*model.GeoCompetitor, error)
	Create(ctx context.Context, c *model.GeoCompetitor) error
	Update(ctx context.Context, c *model.GeoCompetitor) error
	Delete(ctx context.Context, id uint) error
	ListActive(ctx context.Context) ([]*model.GeoCompetitor, error)
}

type geoCompetitorRepo struct{ db *gorm.DB }

func NewGeoCompetitorRepository() GeoCompetitorRepository {
	return &geoCompetitorRepo{db: _db.GetDB()}
}

// NewGeoCompetitorRepositoryWithDB 显式注入 gormDB（路由装配用，测试可替换）
func NewGeoCompetitorRepositoryWithDB(db *gorm.DB) GeoCompetitorRepository {
	return &geoCompetitorRepo{db: db}
}

func (r *geoCompetitorRepo) List(ctx context.Context, status string) ([]*model.GeoCompetitor, error) {
	q := r.db.WithContext(ctx).Model(&model.GeoCompetitor{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var list []*model.GeoCompetitor
	err := q.Order("priority DESC, id ASC").Find(&list).Error
	return list, err
}

func (r *geoCompetitorRepo) GetByID(ctx context.Context, id uint) (*model.GeoCompetitor, error) {
	var c model.GeoCompetitor
	err := r.db.WithContext(ctx).First(&c, id).Error
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *geoCompetitorRepo) Create(ctx context.Context, c *model.GeoCompetitor) error {
	return r.db.WithContext(ctx).Create(c).Error
}

func (r *geoCompetitorRepo) Update(ctx context.Context, c *model.GeoCompetitor) error {
	// 不用 Save：Save 在目标行已被删除时会重新 INSERT，把已删竞品「复活」，
	// 且命中 0 行时无法区分跨租户/不存在。按主键定向更新并检查影响行数。
	res := r.db.WithContext(ctx).Model(&model.GeoCompetitor{}).
		Where("id = ?", c.ID).
		Select("name", "domain", "paths", "category", "priority", "status", "notes").
		Updates(c)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *geoCompetitorRepo) Delete(ctx context.Context, id uint) error {
	res := r.db.WithContext(ctx).Delete(&model.GeoCompetitor{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *geoCompetitorRepo) ListActive(ctx context.Context) ([]*model.GeoCompetitor, error) {
	return r.List(ctx, "active")
}
