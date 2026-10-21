package model

import "time"

// SystemConfigKV 租户级单条 KV 配置。
//
// 归属（SaaS 多租户）：本表存**租户业务配置**（智能体设置、工具集成凭证、
// 知识库连接器、报价模板指针、客服排班、意向识别、话术 A/B、热插拔资产包等）。
// 主键 (tenant_id, key)：同一键名可在每个租户各存一行，互不覆盖。
//
// 平台级键（桥接令牌 / MCP 令牌 / 订单 webhook 密钥 / 浏览器宿主令牌 /
// 安全规则 / 备份策略）已迁至 platform_config_kv（见该模型注释），它们由平台
// 统一持有，不随租户分域。口令策略不在其中：v3.65.0 明确把它留给各租户 admin
// 维护，属租户级安全设置（见 service/password_policy.go 的 PolicyKVKey）。
type SystemConfigKV struct {
	TenantID  uint      `gorm:"column:tenant_id;primaryKey;not null;default:1" json:"tenant_id"`
	Key       string    `gorm:"column:key;primaryKey;size:100" json:"key"`
	Value     string    `gorm:"column:value;type:text;not null" json:"value"`
	CreatedAt time.Time `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

// TableName 表名
func (SystemConfigKV) TableName() string {
	return "system_config_kv"
}
