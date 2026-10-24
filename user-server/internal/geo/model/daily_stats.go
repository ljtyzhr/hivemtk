package model

import (
	"time"

	"gorm.io/gorm"
)

// GeoDailyStat GEO 每日统计聚合表
//
// 按日期 × 引擎 × 意图 三元组唯一，对 geo_probe_runs 做预聚合。
// 写入由 ProbeAnalyzer 定时任务完成，读取端直接查此表避免实时大聚合。
type GeoDailyStat struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID                       uint           `gorm:"primaryKey" json:"id"`
	Date                     string         `gorm:"column:stat_date;size:10;uniqueIndex:idx_date_engine_intent" json:"date"`
	Engine                   string         `gorm:"column:engine;size:32;uniqueIndex:idx_date_engine_intent" json:"engine"`
	Intent                   string         `gorm:"column:intent;size:64;uniqueIndex:idx_date_engine_intent" json:"intent"`
	FunnelStage              string         `gorm:"column:funnel_stage;size:20;index" json:"funnel_stage"`
	BrandMentionedCount      int            `gorm:"column:brand_mentioned_count" json:"brand_mentioned_count"`
	CompetitorMentionedCount int            `gorm:"column:competitor_mentioned_count" json:"competitor_mentioned_count"`
	CitationCount            int            `gorm:"column:citation_count" json:"citation_count"`
	NegativeCount            int            `gorm:"column:negative_count" json:"negative_count"`
	ProbeCount               int            `gorm:"column:probe_count;default:0" json:"probe_count"`
	CreatedAt                time.Time      `json:"created_at"`
	UpdatedAt                time.Time      `json:"updated_at"`
	DeletedAt                gorm.DeletedAt `gorm:"index" json:"-"`
}

func (GeoDailyStat) TableName() string { return "geo_daily_stats" }
