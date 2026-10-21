package repository

import (
	"context"
	"time"

	"hivemtk-user/internal/geo/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// EngineCountRow 按引擎分组的计数行
type EngineCountRow struct {
	Engine string `gorm:"column:engine" json:"engine"`
	Count  int64  `gorm:"column:cnt" json:"count"`
}

// TrackRow 收录追踪明细行（Funnel 列表展示）
type TrackRow struct {
	ArticleID    string     `gorm:"column:article_id" json:"article_id"`
	URL          string     `gorm:"column:url" json:"url"`
	Engine       string     `gorm:"column:engine" json:"engine"`
	Keyword      string     `gorm:"column:keyword" json:"keyword"`
	Indexed      bool       `gorm:"column:indexed" json:"indexed"`
	RankPosition int        `gorm:"column:rank_position" json:"rank_position"`
	AICited      bool       `gorm:"column:ai_cited" json:"ai_cited"`
	AICiteCount  int        `gorm:"column:ai_cite_count" json:"ai_cite_count"`
	LastChecked  *time.Time `gorm:"column:last_checked" json:"last_checked"`
}

// GeoIndexTrackingRepository 收录 + AI 引用追踪仓储
type GeoIndexTrackingRepository interface {
	// UpsertCitation 按 (tenant_id, article_id, engine) 幂等写入一次探针结果
	UpsertCitation(ctx context.Context, t *model.GeoIndexTracking) error
	ListByArticle(ctx context.Context, articleID string) ([]*model.GeoIndexTracking, error)
	CountIndexedByEngine(ctx context.Context) ([]EngineCountRow, error)
	CountAICitedByEngine(ctx context.Context) ([]EngineCountRow, error)
	Recent(ctx context.Context, limit int) ([]TrackRow, error)
}

type geoIndexTrackingRepo struct {
	db *gorm.DB
}

func NewGeoIndexTrackingRepository() GeoIndexTrackingRepository {
	return &geoIndexTrackingRepo{db: _db.GetDB()}
}

func NewGeoIndexTrackingRepositoryWithDB(db *gorm.DB) GeoIndexTrackingRepository {
	return &geoIndexTrackingRepo{db: db}
}

// UpsertCitation 冲突键与 v3.73.0 的唯一索引 (tenant_id, article_id, engine) 一致。
// 原先 service 里「先 Updates，0 行再 Create」在并发验证下会写出同一文章同一引擎的多行。
func (r *geoIndexTrackingRepo) UpsertCitation(ctx context.Context, t *model.GeoIndexTracking) error {
	if t == nil {
		return nil
	}
	now := time.Now()
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "tenant_id"}, {Name: "article_id"}, {Name: "engine"}},
		DoUpdates: clause.Assignments(map[string]any{
			"url":           t.URL,
			"keyword":       t.Keyword,
			"ai_cited":      t.AICited,
			"ai_cite_count": t.AICiteCount,
			"last_checked":  t.LastChecked,
			"updated_at":    now,
		}),
	}).Create(t).Error
}

func (r *geoIndexTrackingRepo) ListByArticle(ctx context.Context, articleID string) ([]*model.GeoIndexTracking, error) {
	var list []*model.GeoIndexTracking
	err := r.db.WithContext(ctx).Where("article_id = ?", articleID).Find(&list).Error
	return list, err
}

func (r *geoIndexTrackingRepo) CountIndexedByEngine(ctx context.Context) ([]EngineCountRow, error) {
	var rows []EngineCountRow
	err := r.db.WithContext(ctx).Model(&model.GeoIndexTracking{}).
		Select("engine, COUNT(*) as cnt").
		Where("indexed = true").
		Group("engine").
		Scan(&rows).Error
	return rows, err
}

func (r *geoIndexTrackingRepo) CountAICitedByEngine(ctx context.Context) ([]EngineCountRow, error) {
	var rows []EngineCountRow
	err := r.db.WithContext(ctx).Model(&model.GeoIndexTracking{}).
		Select("engine, COUNT(*) as cnt").
		Where("ai_cited = true").
		Group("engine").
		Scan(&rows).Error
	return rows, err
}

func (r *geoIndexTrackingRepo) Recent(ctx context.Context, limit int) ([]TrackRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var rows []TrackRow
	err := r.db.WithContext(ctx).Model(&model.GeoIndexTracking{}).
		Order("updated_at DESC").
		Limit(limit).
		Scan(&rows).Error
	return rows, err
}
