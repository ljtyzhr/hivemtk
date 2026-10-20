package model

import "time"

// PlatformConfigKV 平台级单条 KV 配置（实例级，不参与租户分域）。
//
// 与 system_config_kv 的分工：那张表按 (tenant_id, key) 存**租户业务配置**；
// 本表存**平台自身运行所需的键**——它们描述的是这一套服务实例怎么被接入，
// 而不是某个租户怎么经营：
//   - bridge_ingest_token / bridge_ingest_token_prev（浏览器扩展接入）
//   - mcp_token（MCP 工具入口）
//   - order_webhook_secret_<platform>（各电商平台回调验签）
//   - browser_host_token（浏览器自动化宿主）
//   - security.rules（平台安全基线；safeprompt 词典为进程级全局状态）
//   - backup.strategy（整库备份运维策略）
//
// 口令强度策略（password_policy）属租户级安全设置，留在 system_config_kv。
//
// 键仍全局唯一（key 主键）：平台配置对全实例只有一份。
type PlatformConfigKV struct {
	Key       string    `gorm:"column:key;primaryKey;size:100" json:"key"`
	Value     string    `gorm:"column:value;type:text;not null" json:"value"`
	CreatedAt time.Time `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

// TableName 表名
func (PlatformConfigKV) TableName() string {
	return "platform_config_kv"
}
