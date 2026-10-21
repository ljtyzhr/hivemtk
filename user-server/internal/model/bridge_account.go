package model

import "time"

// BridgeAccount 桥接账号（抖音/小红书/TikTok 网页私信扩展账号）
//
// 由扩展上报（HTTP ingest）时自动注册/更新；agent_id 用于 AI 路由
// （与 channel_agent_bindings 联动）。user_id 的 0 是常态而非缺失：
// 上报通道只认实例级桥接凭证、不带用户身份，账号归属落在租户上；
// 非零表示被某个用户显式认领，此时只对本人可见/可用。
type BridgeAccount struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID          uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID      uint       `gorm:"index;not null;default:0" json:"user_id"`
	Channel     string     `gorm:"type:varchar(32);not null;uniqueIndex:uk_bridge_ch_acc" json:"channel"`
	AccountID   string     `gorm:"type:varchar(64);not null;uniqueIndex:uk_bridge_ch_acc" json:"account_id"`
	AccountName string     `gorm:"type:varchar(128);not null;default:''" json:"account_name"`
	AgentID     uint       `gorm:"index;not null;default:0" json:"agent_id"`
	Status      string     `gorm:"type:varchar(16);not null;default:'offline'" json:"status"`
	LastSyncAt  *time.Time `json:"last_sync_at"`
	CreatedAt   time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

func (BridgeAccount) TableName() string { return "bridge_accounts" }
