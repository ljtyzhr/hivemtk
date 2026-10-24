package repository

import (
	"context"
	"hivemtk-user/internal/model"
	"time"

	"gorm.io/gorm"
)

// RagRecallMonitorTableName 监控快照表名（与 service 层常量保持一致，避免循环引用）。
const RagRecallMonitorTableName = "rag_recall_monitor_snapshots"

type RagRecallAggRow struct {
	Total         int64
	AvgRecall     float64
	AvgPrecision  float64
	AvgLatency    float64
	AvgSimilarity float64
	ZeroHit       int64
	LowRecall     int64
	Top1Hit       int64
}

type RagRecallMonitorRepository interface {
	AggregateRecallLogs(ctx context.Context, start, end time.Time) (*RagRecallAggRow, error)
	PluckP95Latency(ctx context.Context, start, end time.Time, offset int) (int64, error)
	CreateSnapshot(ctx context.Context, row map[string]any) error
	ListSnapshots(ctx context.Context, limit int) ([]map[string]any, error)
	EnsureSchema(ctx context.Context) error
}

type ragRecallMonitorRepo struct {
	db *gorm.DB
}

// NewRagRecallMonitorRepository db 为 nil 时返回 nil 接口，service 以 repo == nil 守卫。
func NewRagRecallMonitorRepository(db *gorm.DB) RagRecallMonitorRepository {
	if db == nil {
		return nil
	}
	return &ragRecallMonitorRepo{db: db}
}

func (r *ragRecallMonitorRepo) AggregateRecallLogs(ctx context.Context, start, end time.Time) (*RagRecallAggRow, error) {
	row := &RagRecallAggRow{}
	if err := r.db.WithContext(ctx).
		Table("rag_query_logs").
		Where("created_at >= ? AND created_at < ?", start, end).
		Select(`
			COUNT(*) AS total,
			COALESCE(AVG(recall), 0) AS avg_recall,
			COALESCE(AVG(precision), 0) AS avg_precision,
			COALESCE(AVG(latency_ms), 0) AS avg_latency,
			COALESCE(AVG(top_similarity), 0) AS avg_similarity,
			COUNT(*) FILTER (WHERE retrieved_count = 0) AS zero_hit,
			COUNT(*) FILTER (WHERE recall < 0.3) AS low_recall,
			COUNT(*) FILTER (WHERE hit_in_top1 = true) AS top1_hit
		`).
		Scan(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

func (r *ragRecallMonitorRepo) PluckP95Latency(ctx context.Context, start, end time.Time, offset int) (int64, error) {
	var p95 int64
	if err := r.db.WithContext(ctx).
		Table("rag_query_logs").
		Where("created_at >= ? AND created_at < ?", start, end).
		Order("latency_ms ASC").
		Offset(offset).
		Limit(1).
		Pluck("latency_ms", &p95).Error; err != nil {
		return 0, err
	}
	return p95, nil
}

func (r *ragRecallMonitorRepo) CreateSnapshot(ctx context.Context, row map[string]any) error {
	return r.db.WithContext(ctx).Table(RagRecallMonitorTableName).Create(row).Error
}

func (r *ragRecallMonitorRepo) ListSnapshots(ctx context.Context, limit int) ([]map[string]any, error) {
	// 本层自行夹取：GORM 的 Limit(0) 等于不加 limit（整表），不能依赖调用方兜底。
	if limit < 1 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	var rows []map[string]any
	if err := r.db.WithContext(ctx).
		Table(RagRecallMonitorTableName).
		Order("window_start DESC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *ragRecallMonitorRepo) EnsureSchema(ctx context.Context) error {
	// AutoMigrate 幂等建表
	return r.db.WithContext(ctx).AutoMigrate(&model.RagRecallMonitorSnapshot{})
}

var _ RagRecallMonitorRepository = (*ragRecallMonitorRepo)(nil)
