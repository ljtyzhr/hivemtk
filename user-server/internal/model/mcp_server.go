package model

import "time"

// MCPServer 外部 MCP server 接入配置（P1-3 客户端侧配置面）。
//
// 为什么落表而不是 env：MCP server 是运营随时增删的接入项（和 OpenAPI 工具集同理），
// env 改一次要重启一次；表里一行 + 启动/保存时发现即可。APIKey 经 dbencrypt 加密。
type MCPServer struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID uint `gorm:"primaryKey;autoIncrement" json:"id"`

	// Name server 标识（业务唯一键；工具名前缀 mcp_<name>_）
	Name string `gorm:"type:varchar(64);uniqueIndex" json:"name"`
	// Endpoint JSON-RPC HTTP(S) 端点
	Endpoint string `gorm:"type:varchar(512);not null" json:"endpoint"`
	// APIKeyEnc 可选 Bearer（dbencrypt 密文）
	APIKeyEnc string `gorm:"type:text" json:"-"`

	Enabled         bool `gorm:"default:true" json:"enabled"`
	TimeoutSeconds  int  `gorm:"default:30" json:"timeout_seconds"`
	FailLimit       int  `gorm:"default:3" json:"fail_limit"`
	CooldownSeconds int  `gorm:"default:60" json:"cooldown_seconds"`

	// 最近一次发现结果（运维观测面；失败不影响其他 server）
	ToolCount    int        `gorm:"default:0" json:"tool_count"`
	LastStatus   string     `gorm:"type:varchar(16);default:''" json:"last_status"`
	LastError    string     `gorm:"type:varchar(512);default:''" json:"last_error"`
	LastSyncedAt *time.Time `json:"last_synced_at"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 表名。
func (MCPServer) TableName() string { return "mcp_servers" }
