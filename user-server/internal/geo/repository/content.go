package repository

import (
	"context"
	"time"

	"hivemtk-user/internal/geo/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// GeoArticleRepository GEO 文章仓储接口
type GeoArticleRepository interface {
	Create(ctx context.Context, article *model.GeoArticle) error
	GetByID(ctx context.Context, id string) (*model.GeoArticle, error)
	GetList(ctx context.Context, keyword, status string, page, limit int) ([]*model.GeoArticle, int64, error)
	Update(ctx context.Context, article *model.GeoArticle) error
	// Count 文章总数；deployedOnly 为 true 时只统计已部署（deployed_at 非空）
	Count(ctx context.Context, deployedOnly bool) (int64, error)
	// ListDeployedNeedingVerify 已部署但 since 之后没有更新过追踪的文章
	ListDeployedNeedingVerify(ctx context.Context, since time.Time, limit int) ([]*model.GeoArticle, error)
	// ListDeployed 全部已部署且有站点 URL 的文章（sitemap 全量生成用）
	ListDeployed(ctx context.Context) ([]*model.GeoArticle, error)
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

func (r *geoArticleRepo) Count(ctx context.Context, deployedOnly bool) (int64, error) {
	var n int64
	q := r.db.WithContext(ctx).Model(&model.GeoArticle{})
	if deployedOnly {
		q = q.Where("deployed_at IS NOT NULL")
	}
	err := q.Count(&n).Error
	return n, err
}

func (r *geoArticleRepo) ListDeployedNeedingVerify(ctx context.Context, since time.Time, limit int) ([]*model.GeoArticle, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	var list []*model.GeoArticle
	err := r.db.WithContext(ctx).
		Where("deployed_at IS NOT NULL AND site_url != ''").
		Where(`NOT EXISTS (
			SELECT 1 FROM geo_index_trackings t
			WHERE t.article_id = geo_articles.id AND t.updated_at > ?
		)`, since).
		Order("deployed_at DESC").
		Limit(limit).
		Find(&list).Error
	return list, err
}

func (r *geoArticleRepo) ListDeployed(ctx context.Context) ([]*model.GeoArticle, error) {
	var list []*model.GeoArticle
	err := r.db.WithContext(ctx).
		Where("deployed_at IS NOT NULL AND site_url != ''").
		Order("deployed_at DESC").
		Find(&list).Error
	return list, err
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

func (r *geoArticleRepo) Update(ctx context.Context, article *model.GeoArticle) error {
	// 不用 Save：已删除文章会被重新 INSERT，且命中 0 行时无法区分不存在。
	res := r.db.WithContext(ctx).Model(&model.GeoArticle{}).
		Where("id = ?", article.ID).
		Select("title", "content", "keyword", "model", "prompt", "word_count", "status",
			"score", "score_detail", "json_ld", "brand_name", "site_url", "site_path",
			"schema_json", "markdown_path", "deployed_at", "llms_included",
			"funnel_stage", "query_intent").
		Updates(article)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// GeoOptimizationRepository GEO 优化记录仓储接口
type GeoOptimizationRepository interface {
	Create(ctx context.Context, opt *model.GeoOptimization) error
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
