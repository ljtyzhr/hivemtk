package repository

import (
	"context"
	"encoding/json"
	"time"

	"hivemtk-user/internal/geo/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GeoKeywordGroupRepository GEO 关键词分组（话题聚类结果）
type GeoKeywordGroupRepository interface {
	// UpsertByName 按 (tenant_id, name) upsert（同品牌聚类覆盖更新）
	UpsertByName(ctx context.Context, name string, keywords []string) (*model.GeoKeywordGroup, error)
}

type geoKeywordGroupRepo struct {
	db *gorm.DB
}

// NewGeoKeywordGroupRepository 构造 repository（使用默认 DB）
func NewGeoKeywordGroupRepository() GeoKeywordGroupRepository {
	return &geoKeywordGroupRepo{db: _db.GetDB()}
}

// NewGeoKeywordGroupRepositoryWithDB 构造 repository（使用指定 DB，用于路由注入和测试）
func NewGeoKeywordGroupRepositoryWithDB(db *gorm.DB) GeoKeywordGroupRepository {
	return &geoKeywordGroupRepo{db: db}
}

// UpsertByName 冲突键与 v3.74.0 的唯一索引 (tenant_id, name) 一致。
// 原先「SELECT 后 Save/Create」有两个问题：并发聚类会建出同名分组；
// Save 命中软删除行时会把它原地复活（且 resurrections 不刷新 updated_at 之外的语义）。
// 这里统一走 ON CONFLICT DO UPDATE，并把软删除标记清空（upsert 语义即「让该分组可用」）。
func (r *geoKeywordGroupRepo) UpsertByName(ctx context.Context, name string, keywords []string) (*model.GeoKeywordGroup, error) {
	kwJSON, err := json.Marshal(keywords)
	if err != nil {
		return nil, err
	}
	g := &model.GeoKeywordGroup{
		Name:         name,
		KeywordList:  string(kwJSON),
		KeywordCount: len(keywords),
	}
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "tenant_id"}, {Name: "name"}},
		DoUpdates: clause.Assignments(map[string]any{
			"keyword_list":  string(kwJSON),
			"keyword_count": len(keywords),
			"updated_at":    time.Now(),
			"deleted_at":    nil,
		}),
	}).Create(g).Error; err != nil {
		return nil, err
	}
	return g, nil
}
