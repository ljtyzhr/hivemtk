package repository

import (
	"context"
	"hivemtk-user/internal/geo/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// GeoVerifyResultRepository GEO 验证结果仓储接口
type GeoVerifyResultRepository interface {
	ListAllForSOV(ctx context.Context, intent string) ([]*model.GeoVerifyResult, error)
	Create(ctx context.Context, result *model.GeoVerifyResult) error
	GetByArticleID(ctx context.Context, articleID string) ([]*model.GeoVerifyResult, error)
	GetByBrandName(ctx context.Context, brandName string) ([]*model.GeoVerifyResult, error)
	GetList(ctx context.Context, articleID string, page, limit int) ([]*model.GeoVerifyResult, int64, error)
	GetStatistics(ctx context.Context) ([]map[string]any, error)
}

type geoVerifyResultRepo struct {
	db *gorm.DB
}

func NewGeoVerifyResultRepository() GeoVerifyResultRepository {
	return &geoVerifyResultRepo{db: _db.GetDB()}
}

// NewGeoVerifyResultRepositoryWithDB 创建指定数据库连接的实例（用于测试）
func NewGeoVerifyResultRepositoryWithDB(db *gorm.DB) GeoVerifyResultRepository {
	return &geoVerifyResultRepo{db: db}
}

func (r *geoVerifyResultRepo) Create(ctx context.Context, result *model.GeoVerifyResult) error {
	return r.db.WithContext(ctx).Create(result).Error
}

func (r *geoVerifyResultRepo) GetByArticleID(ctx context.Context, articleID string) ([]*model.GeoVerifyResult, error) {
	var results []*model.GeoVerifyResult
	err := r.db.WithContext(ctx).Where("article_id = ?", articleID).Order("created_at DESC").Find(&results).Error
	return results, err
}

func (r *geoVerifyResultRepo) GetByBrandName(ctx context.Context, brandName string) ([]*model.GeoVerifyResult, error) {
	var results []*model.GeoVerifyResult
	err := r.db.WithContext(ctx).Where("brand_name = ?", brandName).Order("created_at DESC").Find(&results).Error
	return results, err
}

func (r *geoVerifyResultRepo) GetList(ctx context.Context, articleID string, page, limit int) ([]*model.GeoVerifyResult, int64, error) {
	var results []*model.GeoVerifyResult
	var total int64
	offset := (page - 1) * limit

	query := r.db.WithContext(ctx).Model(&model.GeoVerifyResult{})

	if articleID != "" {
		query = query.Where("article_id = ?", articleID)
	}

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&results).Error
	return results, total, err
}

func (r *geoVerifyResultRepo) GetStatistics(ctx context.Context) ([]map[string]any, error) {
	var statistics []map[string]any
	err := r.db.WithContext(ctx).Table("geo_verify_results").
		Select("model AS model, COUNT(*) AS total, " +
			"SUM(CASE WHEN brand_mentioned = true THEN 1 ELSE 0 END) AS mentioned_total").
		Where("deleted_at IS NULL").
		Group("model").Order("model").
		Scan(&statistics).Error
	return statistics, err
}

// GeoAPICallRepository GEO API 调用记录仓储接口
type GeoAPICallRepository interface {
	Create(ctx context.Context, call *model.GeoAPICall) error
	GetList(ctx context.Context, provider, modelName string, page, limit int) ([]*model.GeoAPICall, int64, error)
	GetCostStatistics(ctx context.Context, startDate, endDate string) ([]map[string]any, error)
	GetByProvider(ctx context.Context, provider string) ([]*model.GeoAPICall, error)
}

type geoAPICallRepo struct {
	db *gorm.DB
}

func NewGeoAPICallRepository() GeoAPICallRepository {
	return &geoAPICallRepo{db: _db.GetDB()}
}

// NewGeoAPICallRepositoryWithDB 创建指定数据库连接的实例（用于测试）
func NewGeoAPICallRepositoryWithDB(db *gorm.DB) GeoAPICallRepository {
	return &geoAPICallRepo{db: db}
}

func (r *geoAPICallRepo) Create(ctx context.Context, call *model.GeoAPICall) error {
	return r.db.WithContext(ctx).Create(call).Error
}

func (r *geoAPICallRepo) GetList(ctx context.Context, provider, modelName string, page, limit int) ([]*model.GeoAPICall, int64, error) {
	var calls []*model.GeoAPICall
	var total int64
	offset := (page - 1) * limit

	query := r.db.WithContext(ctx).Model(&model.GeoAPICall{})

	if provider != "" {
		query = query.Where("provider = ?", provider)
	}
	if modelName != "" {
		query = query.Where("model = ?", modelName)
	}

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&calls).Error
	return calls, total, err
}

func (r *geoAPICallRepo) GetCostStatistics(ctx context.Context, startDate, endDate string) ([]map[string]any, error) {
	var statistics []map[string]any

	query := r.db.WithContext(ctx).Model(&model.GeoAPICall{}).
		Select("provider, model, COUNT(*) AS call_count, COALESCE(SUM(input_tokens),0) AS input_tokens, COALESCE(SUM(output_tokens),0) AS output_tokens, COALESCE(SUM(cost_usd),0) AS cost_usd, COALESCE(SUM(cost_cny),0) AS cost_cny").
		Group("provider, model").
		Order("provider, model")

	if startDate != "" {
		query = query.Where("created_at >= ?", startDate)
	}
	if endDate != "" {
		query = query.Where("created_at <= ?", endDate)
	}

	err := query.Scan(&statistics).Error
	return statistics, err
}

func (r *geoAPICallRepo) GetByProvider(ctx context.Context, provider string) ([]*model.GeoAPICall, error) {
	var calls []*model.GeoAPICall
	err := r.db.WithContext(ctx).Where("provider = ?", provider).Order("created_at DESC").Find(&calls).Error
	return calls, err
}

func (r *geoVerifyResultRepo) ListAllForSOV(ctx context.Context, intent string) ([]*model.GeoVerifyResult, error) {
	var rows []*model.GeoVerifyResult
	q := r.db.WithContext(ctx)
	if intent != "" {

		_ = intent
	}
	err := q.Find(&rows).Error
	return rows, err
}
