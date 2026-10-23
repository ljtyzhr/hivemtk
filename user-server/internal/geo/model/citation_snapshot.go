package model

import (
	"time"

	"gorm.io/gorm"
)

// GeoCitationSnapshot A10 引用漂移快照（2026-10-24）：
// 每业务日一行，存当日聚合的 URL 引用计数（json）。定时任务写入，
// 报表读两次快照做 diff（增删/排名变化）——没有快照就没有"漂移"可言，
// 引用是探针流水，跑过就散在 geo_probe_runs.citations jsonb 里没法回看。
//
// 形状说明（同 index_tracking.go 的警告）：unique 键不用 uniqueIndex 标签——
// AutoMigrate 对它先到先得，多模型同键会静默复用。唯一性由迁移 v3.80.0 建。
type GeoCitationSnapshot struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID uint `gorm:"primaryKey" json:"id"`
	// SnapshotDate 业务日（Asia/Shanghai，timeutil 口径），YYYY-MM-DD。
	SnapshotDate string `gorm:"column:snapshot_date;size:10;index" json:"snapshot_date"`
	// DomainCount URL→引用次数的 JSON：{"https://a.com/x":3,...}（text 而非 jsonb：
	// 只整行读写，不需要 jsonb 索引能力）。
	URLCounts string `gorm:"column:url_counts;type:text" json:"url_counts"`
	// TotalCitations 当日引用总数（写死在行上，diff 时省一次全 JSON 解析对账）。
	TotalCitations int `gorm:"column:total_citations" json:"total_citations"`
	// DistinctURLs 当日不同 URL 数。
	DistinctURLs int `gorm:"column:distinct_urls" json:"distinct_urls"`

	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (GeoCitationSnapshot) TableName() string { return "geo_citation_snapshots" }
