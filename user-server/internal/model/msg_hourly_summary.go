package model

import "time"

type MsgHourlySummary struct {
	TenantID     uint      `gorm:"not null;default:1;index" json:"tenant_id"`
	HourBucket   time.Time `gorm:"type:timestamptz;not null;uniqueIndex:uni_mhs_bucket_dim,priority:1" json:"hour_bucket"`
	MerchantID   uint      `gorm:"not null;default:0;uniqueIndex:uni_mhs_bucket_dim,priority:2" json:"merchant_id"`
	Platform     string    `gorm:"type:varchar(30);not null;default:'';uniqueIndex:uni_mhs_bucket_dim,priority:3" json:"platform"`
	SessionCount int64     `gorm:"not null;default:0" json:"session_count"`
	AICount      int64     `gorm:"not null;default:0" json:"ai_count"`
	HumanCount   int64     `gorm:"not null;default:0" json:"human_count"`
	MessageCount int64     `gorm:"not null;default:0" json:"message_count"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (MsgHourlySummary) TableName() string { return "msg_hourly_summary" }

// AggregationWatermark 增量聚合水位线（D-3，trace_learning 式模式复用）。
// source 为数据源标识（当前仅 "message_hub"）；last_event_id 记录已消费的最大源表 id，
// 与增量 upsert 同事务推进，保证重启/重跑不丢不重。
// 主键为 (tenant_id, source)：水位线按租户推进，SaaS 模式下各租户消费互不越线
// （specs/001-saas-multi-tenant T008 冲突目标复合化的配套主键升级）。
type AggregationWatermark struct {
	TenantID uint `gorm:"primaryKey;not null;default:1" json:"tenant_id"`

	Source      string    `gorm:"type:varchar(60);primaryKey" json:"source"`
	LastEventID int64     `gorm:"not null;default:0" json:"last_event_id"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (AggregationWatermark) TableName() string { return "aggregation_watermarks" }

// SummarySourceMessageHub watermark source 常量：message_hub 小时汇总
const SummarySourceMessageHub = "message_hub"
