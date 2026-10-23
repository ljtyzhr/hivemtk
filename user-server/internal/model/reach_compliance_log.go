// reach_compliance_log.go 合规审计日志模型与批量落库仓储（五层 L5）
package model

import (
	"time"
)

// ReachComplianceLog 合规提醒审计日志（表 reach_compliance_log）
type ReachComplianceLog struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Channel     string    `gorm:"type:varchar(30);index" json:"channel"`
	RecipientID string    `gorm:"type:varchar(128)" json:"recipient_id"`
	CreatedAt   time.Time `gorm:"autoCreateTime;index" json:"created_at"`

	// Decision E1：策略判定动作 allow/deny/defer。
	// 空串表示「本行只记了发送提醒」（升级前的历史行形态），不是缺失。
	Decision string `gorm:"type:varchar(16);index" json:"decision,omitempty"`
	// Reason E1：原因码（channel_denied / channel_not_allowed / opt_in_required /
	// quiet_hours / daily_limit / region_unruled / allowed / policy_disabled）。
	Reason string `gorm:"type:varchar(48);index" json:"reason,omitempty"`
	// Region / Industry E1：判定所依据的地区（大写）与行业（小写）。
	Region   string `gorm:"type:varchar(32);index" json:"region,omitempty"`
	Industry string `gorm:"type:varchar(32);index" json:"industry,omitempty"`
	// PolicyScope E1：命中的规则域（region:US / industry:medical），排障直接定位到规则。
	PolicyScope string `gorm:"type:varchar(64)" json:"policy_scope,omitempty"`
}

// TableName 指定表名
func (ReachComplianceLog) TableName() string { return "reach_compliance_log" }
