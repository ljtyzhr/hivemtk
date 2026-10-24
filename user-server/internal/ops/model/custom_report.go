package model

import (
	"time"
)

// CustomReport 自定义报表模型
type CustomReport struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Name        string    `gorm:"type:varchar(100);not null" json:"name"`
	Description string    `gorm:"type:varchar(500)" json:"description"`
	DataSource  string    `gorm:"type:varchar(50);not null" json:"data_source"`
	Dimensions  string    `gorm:"type:text" json:"dimensions"`
	Metrics     string    `gorm:"type:text" json:"metrics"`
	Filters     string    `gorm:"type:text" json:"filters"`
	ChartType   string    `gorm:"type:varchar(20)" json:"chart_type"`
	ChartConfig string    `gorm:"type:text" json:"chart_config"`
	IsPublic    bool      `gorm:"default:false" json:"is_public"`
	CreatedBy   uint      `json:"created_by"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (CustomReport) TableName() string {
	return "custom_reports"
}

// ReportDimension 报表维度
type ReportDimension struct {
	Field    string `json:"field"`
	Label    string `json:"label"`
	DataType string `json:"data_type"`
	GroupBy  bool   `json:"group_by"`
}

// ReportMetric 报表指标
type ReportMetric struct {
	Field    string `json:"field"`
	Label    string `json:"label"`
	AggFunc  string `json:"agg_func"`
	DataType string `json:"data_type"`
}

// ReportFilter 报表筛选条件
type ReportFilter struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    any    `json:"value"`
}

// ReportData 报表数据
type ReportData struct {
	Dimensions []string         `json:"dimensions"`
	Metrics    []string         `json:"metrics"`
	Data       []map[string]any `json:"data"`
	Total      int64            `json:"total"`
}
