package repository

import (
	"context"

	"hivemtk-user/internal/geo/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GeoCitationSnapshotRepository A10 引用漂移快照仓储（UPSERT 语义，同 GeoDailyStat）。
type GeoCitationSnapshotRepository interface {
	Upsert(ctx context.Context, snap *model.GeoCitationSnapshot) error
	GetByDate(ctx context.Context, date string) (*model.GeoCitationSnapshot, error)
	ListSince(ctx context.Context, sinceDate string, limit int) ([]*model.GeoCitationSnapshot, error)
}

type geoCitationSnapshotRepo struct{ db *gorm.DB }

func NewGeoCitationSnapshotRepository() GeoCitationSnapshotRepository {
	return &geoCitationSnapshotRepo{db: _db.GetDB()}
}

func NewGeoCitationSnapshotRepositoryWithDB(db *gorm.DB) GeoCitationSnapshotRepository {
	return &geoCitationSnapshotRepo{db: db}
}

// Upsert 冲突目标 = 表上的唯一键（tenant_id, snapshot_date，迁移 v3.80.0 建）。
// 不用 FirstOrCreate：SELECT 后 INSERT 并发下撞 23505（同 daily_stats.go 的取舍）。
func (r *geoCitationSnapshotRepo) Upsert(ctx context.Context, snap *model.GeoCitationSnapshot) error {
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "tenant_id"}, {Name: "snapshot_date"}},
			DoUpdates: clause.Assignments(map[string]any{
				"url_counts":       snap.URLCounts,
				"total_citations":  snap.TotalCitations,
				"distinct_urls":    snap.DistinctURLs,
			}),
		}).
		Create(snap).Error
}

func (r *geoCitationSnapshotRepo) GetByDate(ctx context.Context, date string) (*model.GeoCitationSnapshot, error) {
	var snap model.GeoCitationSnapshot
	err := r.db.WithContext(ctx).Where("snapshot_date = ?", date).First(&snap).Error
	if err != nil {
		return nil, err
	}
	return &snap, nil
}

// ListSince 列出 snapshot_date >= sinceDate 的快照（升序，diff 用）。
func (r *geoCitationSnapshotRepo) ListSince(ctx context.Context, sinceDate string, limit int) ([]*model.GeoCitationSnapshot, error) {
	if limit <= 0 {
		limit = 90
	}
	var rows []*model.GeoCitationSnapshot
	err := r.db.WithContext(ctx).
		Where("snapshot_date >= ?", sinceDate).
		Order("snapshot_date ASC").Limit(limit).Find(&rows).Error
	return rows, err
}
