package model

import "time"

// NotificationRead 通知已读回执（表 notification_reads）
//
// 为什么单独成表：notifications.is_read 是**消息行上的单一布尔位**。
// 平台消息一条对应 N 个用户（notifications.user_id = 0 表示广播），
// A 用户点已读会把 B 用户看到的同一条也变成已读 ⇒ 未读角标被一个人清零。
// 把"消息 × 人"的已读关系显式建模成一张表，是表达"按人独立已读"的最小结构。
//
// 本地业务通知**不走这张表**：它们的受众是管理员/审核人，
// "任一人读过即算"是对的（避免 N 个人各点一次），沿用行上的 is_read。
//
// 幂等键 = (tenant_id, notification_id, user_id)：同一用户对同一消息只有一条
// 回执，标记已读是 upsert，重复点不报错、不产生重复行。
type NotificationRead struct {
	// TenantID 租户号。回执同样按租户隔离。
	TenantID uint `gorm:"not null;default:1;index;uniqueIndex:uq_notification_read,priority:1" json:"tenant_id"`

	ID uint `json:"id" gorm:"primaryKey"`

	// NotificationID 指向 notifications.id。
	NotificationID uint `json:"notification_id" gorm:"not null;index;uniqueIndex:uq_notification_read,priority:2" json:"notification_id"`

	// UserID 谁读的。
	UserID uint `json:"user_id" gorm:"not null;index;uniqueIndex:uq_notification_read,priority:3" json:"user_id"`

	ReadAt time.Time `json:"read_at" gorm:"not null"`
}

// TableName 表名
func (n *NotificationRead) TableName() string {
	return "notification_reads"
}
