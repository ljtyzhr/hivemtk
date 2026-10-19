package repository

import (
	"context"
	"hivemtk-user/internal/geo/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// GeoArticleRepository GEO 文章仓储接口
type GeoArticleRepository interface {
	Create(ctx context.Context, article *model.GeoArticle) error
	GetByID(ctx context.Context, id string) (*model.GeoArticle, error)
	GetList(ctx context.Context, keyword, status string, page, limit int) ([]*model.GeoArticle, int64, error)
	Delete(ctx context.Context, id string) error
	Update(ctx context.Context, article *model.GeoArticle) error
}

type geoArticleRepo struct {
	db *gorm.DB
}

func NewGeoArticleRepository() GeoArticleRepository {
	return &geoArticleRepo{db: _db.GetDB()}
}

// NewGeoArticleRepositoryWithDB 创建指定数据库连接的实例（用于测试）
func NewGeoArticleRepositoryWithDB(db *gorm.DB) GeoArticleRepository {
	return &geoArticleRepo{db: db}
}

func (r *geoArticleRepo) Create(ctx context.Context, article *model.GeoArticle) error {
	return r.db.WithContext(ctx).Create(article).Error
}

func (r *geoArticleRepo) GetByID(ctx context.Context, id string) (*model.GeoArticle, error) {
	var article model.GeoArticle
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&article).Error
	return &article, err
}

func (r *geoArticleRepo) GetList(ctx context.Context, keyword, status string, page, limit int) ([]*model.GeoArticle, int64, error) {
	var articles []*model.GeoArticle
	var total int64
	offset := (page - 1) * limit

	query := r.db.WithContext(ctx).Model(&model.GeoArticle{})

	if keyword != "" {
		query = query.Where("keyword LIKE ?", "%"+escapeLike(keyword)+"%")
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&articles).Error
	return articles, total, err
}

func (r *geoArticleRepo) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.GeoArticle{}).Error
}

func (r *geoArticleRepo) Update(ctx context.Context, article *model.GeoArticle) error {
	return r.db.WithContext(ctx).Save(article).Error
}

// GeoOptimizationRepository GEO 优化记录仓储接口
type GeoOptimizationRepository interface {
	Create(ctx context.Context, opt *model.GeoOptimization) error
	GetByArticleID(ctx context.Context, articleID string) ([]*model.GeoOptimization, error)
	GetList(ctx context.Context, articleID string, page, limit int) ([]*model.GeoOptimization, int64, error)
}

type geoOptimizationRepo struct {
	db *gorm.DB
}

func NewGeoOptimizationRepository() GeoOptimizationRepository {
	return &geoOptimizationRepo{db: _db.GetDB()}
}

// NewGeoOptimizationRepositoryWithDB 创建指定数据库连接的实例（用于测试）
func NewGeoOptimizationRepositoryWithDB(db *gorm.DB) GeoOptimizationRepository {
	return &geoOptimizationRepo{db: db}
}

func (r *geoOptimizationRepo) Create(ctx context.Context, opt *model.GeoOptimization) error {
	return r.db.WithContext(ctx).Create(opt).Error
}

func (r *geoOptimizationRepo) GetByArticleID(ctx context.Context, articleID string) ([]*model.GeoOptimization, error) {
	var opts []*model.GeoOptimization
	err := r.db.WithContext(ctx).Where("article_id = ?", articleID).Order("created_at DESC").Find(&opts).Error
	return opts, err
}

func (r *geoOptimizationRepo) GetList(ctx context.Context, articleID string, page, limit int) ([]*model.GeoOptimization, int64, error) {
	var opts []*model.GeoOptimization
	var total int64
	offset := (page - 1) * limit

	query := r.db.WithContext(ctx).Model(&model.GeoOptimization{})

	if articleID != "" {
		query = query.Where("article_id = ?", articleID)
	}

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&opts).Error
	return opts, total, err
}
