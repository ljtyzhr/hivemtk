// Package model — custom_tool.go P1-2：OpenAPI 声明式工具导入持久化。
package model

import "time"

// CustomTool 用户经 OpenAPI 导入的自定义工具 Provider。
// 凭据（AuthValue）经 dbencrypt 加密落库，不明文。
type CustomTool struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID uint `gorm:"primaryKey;autoIncrement" json:"id"`

	// Name Provider 名（业务唯一键，注册名再统一加 openapi_ 前缀）。
	Name        string `gorm:"type:varchar(128);uniqueIndex" json:"name"`
	Description string `gorm:"type:varchar(512)" json:"description"`
	BaseURL     string `gorm:"type:varchar(512)" json:"base_url"`
	AuthHeader  string `gorm:"type:varchar(128)" json:"auth_header,omitempty"`
	// AuthValueEnc 加密后的出站鉴权值（dbencrypt）。
	AuthValueEnc string `gorm:"type:text" json:"-"`
	// SchemaJSON OpenAPI 3.x 文档（工具参数推导源，重导入/重建用）。
	SchemaJSON string `gorm:"type:text" json:"schema_json"`
	TimeoutMS  int    `gorm:"default:10000" json:"timeout_ms"`
	ToolCount  int    `gorm:"default:0" json:"tool_count"`
	IsActive   bool   `gorm:"default:true" json:"is_active"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 表名。
func (CustomTool) TableName() string { return "custom_tools" }
