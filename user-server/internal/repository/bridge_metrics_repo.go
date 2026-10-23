// bridge_metrics_repo.go bridge_metrics 的写入口（运维指标时间序列）
package repository

import (
	"context"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// BridgeMetricsRepository bridge_metrics 追加式写入仓储。
//
// 这个仓储只有写、没有读：读侧是运维 SQL 巡检（按 (metric_name, ts) 查趋势），
// 应用不做查询。把它从 `pkg/metrics` 里搬出来的原因见 model/bridge_metric.go 的注释：
// 指标包原先自己持 DB 句柄 `db.Create(...)`，等于"任何 pkg 都能写任意表"。
type BridgeMetricsRepository struct {
	db *gorm.DB
}

// NewBridgeMetricsRepository 显式注入 DB（装配/测试用）
func NewBridgeMetricsRepository(db *gorm.DB) *BridgeMetricsRepository {
	return &BridgeMetricsRepository{db: db}
}

// WriteBridgeMetrics 批量追加一行一采样（时间序列，不 upsert）。
func (r *BridgeMetricsRepository) WriteBridgeMetrics(ctx context.Context, rows []model.BridgeMetricRow) error {
	if r == nil || r.db == nil {
		return gorm.ErrInvalidDB
	}
	if len(rows) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&rows).Error
}
