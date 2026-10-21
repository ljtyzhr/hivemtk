package model

import (
	"time"

	"github.com/google/uuid"

	"gorm.io/gorm"
)

// GeoIndexTracking 收录 + AI 引用追踪
//
// IndexTrackerService.VerifyArticleFull 写入。
// FunnelDashboard / IndexTracking 页面读取。
// engine 字段同时覆盖搜索引擎（baidu/google/bing）和 AI 引擎（doubao/wenxin/kimi/deepseek）。
//
// 业务唯一键 (tenant_id, article_id, engine) 由 v3.73.0 迁移建唯一索引（先去重再建）。
// 模型不写 uniqueIndex 标签：AutoMigrate 先于迁移执行且不去重，存量库有重复行时会阻断启动。
type GeoIndexTracking struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID        string `gorm:"type:varchar(36);primaryKey" json:"id"`
	ArticleID string `gorm:"type:varchar(36);index" json:"article_id"`
	Engine    string `gorm:"type:varchar(30);index" json:"engine"`
	// 'baidu' | 'google' | 'bing' | 'toutiao' | 'shenma' | '360' | 'doubao' | 'wenxin' | 'kimi' | 'deepseek'
	URL          string     `gorm:"type:varchar(512)" json:"url"`
	Keyword      string     `gorm:"type:text" json:"keyword"`
	Indexed      bool       `gorm:"default:false" json:"indexed"`
	RankPosition int        `json:"rank_position"`
	AICited      bool       `gorm:"default:false" json:"ai_cited"`
	AICiteCount  int        `gorm:"default:0" json:"ai_cite_count"`
	LastChecked  *time.Time `json:"last_checked"`

	CreatedAt time.Time      `gorm:"autoCreateTime;index" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (GeoIndexTracking) TableName() string { return "geo_index_trackings" }

func (t *GeoIndexTracking) BeforeCreate(tx *gorm.DB) error {
	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	return nil
}
