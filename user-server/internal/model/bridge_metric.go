package model

import "time"

// BridgeMetricRow bridge_metrics 落库行（追加式指标时间序列，供运维 SQL 巡检按
// (metric_name, ts) 查趋势）。
//
// 为什么在 model 包里：它原本声明在 `internal/pkg/metrics`，由 `pkg/metrics` 的 sink
// 直接 `db.Create(...)` 写库——模型越层 + 指标包持 DB 句柄写库两件事叠在一起，
// 于是"这个包能写哪些表"没有边界可言。搬到 model 后写入口收在
// `repository.BridgeMetricsRepository`，pkg/metrics 只负责采样与序列化。
//
// 关于 tenant_id：**没有**。它是指标时间序列，维度是 (metric_name, labels, ts)，
// 没有任何租户归属语义；按租户切分只会把同一张运维看板拆成 N 份。因此登记为租户豁免表
// （`tenant_column_batch_migration.go`），写入走系统域（见 sink）。
type BridgeMetricRow struct {
	ID         uint64    `gorm:"column:id;primaryKey" json:"-"`
	MetricName string    `gorm:"column:metric_name" json:"metric_name"`
	Labels     string    `gorm:"column:labels" json:"labels"`
	Value      float64   `gorm:"column:value" json:"value"`
	MetricType string    `gorm:"column:metric_type" json:"metric_type"`
	TS         time.Time `gorm:"column:ts" json:"ts"`
}

// TableName bridge_metrics 表名
func (BridgeMetricRow) TableName() string { return "bridge_metrics" }
