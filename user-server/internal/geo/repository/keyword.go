package repository

import (
	"context"
	"strings"

	"hivemtk-user/internal/geo/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// GeoKeywordRepository GEO 关键词仓储接口
type GeoKeywordRepository interface {
	Create(ctx context.Context, keyword *model.GeoKeyword) error
	BatchCreate(ctx context.Context, keywords []*model.GeoKeyword) error
	GetByID(ctx context.Context, id string) (*model.GeoKeyword, error)
	GetList(ctx context.Context, search, category, source, cluster, status, layer string, page, limit int) ([]*model.GeoKeyword, int64, error)
	Delete(ctx context.Context, id string) error
	GetByCluster(ctx context.Context, cluster string) ([]*model.GeoKeyword, error)
	GetStatistics(ctx context.Context) ([]map[string]any, error)
	// CountByLayerStatus 统计指定 layer + status 的关键词数
	CountByLayerStatus(ctx context.Context, layer, status string) (int64, error)
}

type geoKeywordRepo struct {
	db *gorm.DB
}

func NewGeoKeywordRepository() GeoKeywordRepository {
	return &geoKeywordRepo{db: _db.GetDB()}
}

// NewGeoKeywordRepositoryWithDB 创建指定数据库连接的实例（用于测试）
func NewGeoKeywordRepositoryWithDB(db *gorm.DB) GeoKeywordRepository {
	return &geoKeywordRepo{db: db}
}

func (r *geoKeywordRepo) Create(ctx context.Context, keyword *model.GeoKeyword) error {
	return r.db.WithContext(ctx).Create(keyword).Error
}

func (r *geoKeywordRepo) BatchCreate(ctx context.Context, keywords []*model.GeoKeyword) error {
	if len(keywords) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).CreateInBatches(keywords, 100).Error
}

func (r *geoKeywordRepo) GetByID(ctx context.Context, id string) (*model.GeoKeyword, error) {
	var keyword model.GeoKeyword
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&keyword).Error
	return &keyword, err
}

func (r *geoKeywordRepo) GetList(ctx context.Context, search, category, source, cluster, status, layer string, page, limit int) ([]*model.GeoKeyword, int64, error) {
	var keywords []*model.GeoKeyword
	var total int64
	offset := (page - 1) * limit

	query := r.db.WithContext(ctx).Model(&model.GeoKeyword{})

	if search != "" {
		query = query.Where("keyword LIKE ?", "%"+escapeLike(search)+"%")
	}
	if category != "" {
		query = query.Where("category = ?", category)
	}
	if source != "" {
		query = query.Where("source = ?", source)
	}
	if cluster != "" {
		query = query.Where("cluster = ?", cluster)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if layer != "" {
		query = query.Where("layer = ?", layer)
	}

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&keywords).Error
	return keywords, total, err
}

func (r *geoKeywordRepo) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.GeoKeyword{}).Error
}

func (r *geoKeywordRepo) GetByCluster(ctx context.Context, cluster string) ([]*model.GeoKeyword, error) {
	var keywords []*model.GeoKeyword
	err := r.db.WithContext(ctx).Where("cluster = ?", cluster).Order("created_at DESC").Find(&keywords).Error
	return keywords, err
}

func (r *geoKeywordRepo) GetStatistics(ctx context.Context) ([]map[string]any, error) {
	var statistics []map[string]any
	err := r.db.WithContext(ctx).Table("geo_keywords").
		Select("source AS source, COUNT(*) AS total").
		Where("deleted_at IS NULL").
		Group("source").Order("source").
		Scan(&statistics).Error
	return statistics, err
}

func (r *geoKeywordRepo) CountByLayerStatus(ctx context.Context, layer, status string) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.GeoKeyword{}).
		Where("layer = ? AND status = ?", layer, status).
		Count(&n).Error
	return n, err
}
