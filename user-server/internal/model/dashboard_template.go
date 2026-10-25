// dashboard_template.go 数据大屏模板（R39-K8：大屏复用）。
package model

import (
	"time"

	"gorm.io/gorm"
)

// DashboardTemplate 数据大屏模板。两条来源：从既有大屏发布（PublishFromScreen，
// Layout/Widgets 落发布时刻快照）或手工创建；clone 用快照一键复制出真实大屏
// （dashboard_screens 行 + dashboard_widgets 行）。
type DashboardTemplate struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID          uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	Name        string `gorm:"type:varchar(100);not null" json:"name"`
	Description string `gorm:"type:text" json:"description"`
	// Layout/Theme/Widgets 是发布时刻的大屏快照，clone 时整包落到新大屏。
	// Widgets 是 dashboard_widgets 行（去掉 id/screen_id）的 JSON 数组快照。
	Layout  string `gorm:"type:text" json:"layout"`
	Theme   string `gorm:"type:varchar(20);default:dark" json:"theme"`
	Widgets string `gorm:"type:text" json:"widgets"`
	// Status draft/published：draft 不出现在 clone 候选里。
	Status         string         `gorm:"type:varchar(16);not null;default:draft" json:"status"`
	SourceScreenID uint           `gorm:"index" json:"source_screen_id"`
	Rating         float64        `gorm:"type:decimal(3,2);default:0" json:"rating"`
	RatingCount    int            `gorm:"default:0" json:"rating_count"`
	CloneCount     int            `gorm:"default:0" json:"clone_count"`
	CreatedBy      uint           `json:"created_by"`
	CreatedAt      time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (DashboardTemplate) TableName() string { return "dashboard_templates" }

const (
	DashboardTemplateStatusDraft     = "draft"
	DashboardTemplateStatusPublished = "published"
)

// DashboardTemplateWidget 模板里的单条 widget 快照（clone 时逐条还原成
// dashboard_widgets 行；字段与 ops 侧 DashboardWidget 对齐，去掉 id/screen_id）。
type DashboardTemplateWidget struct {
	WidgetType string         `json:"widget_type"`
	Title      string         `json:"title"`
	Config     map[string]any `json:"config"`
	DataSource string         `json:"data_source"`
	X          int            `json:"x"`
	Y          int            `json:"y"`
	Width      int            `json:"width"`
	Height     int            `json:"height"`
	SortOrder  int            `json:"sort_order"`
}
