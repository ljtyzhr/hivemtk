package model

import "time"

// BrowserProfileHealth 单主 Profile 的平台级健康监护行。
// 主 Profile 复用是本项目风控特色（不做多账号矩阵）：每平台一行，
// blocked=true 即熔断——同平台新任务拒绝/排队，人工恢复后清标记。
// 熔断状态必须落库（Executor 进程级单例禁内存计数）。
//
// 「每平台一行」在 SaaS 下是**每租户每平台**一行：`platform` 建的是单列 uniqueIndex，
// on 模式启动时由 tenancyidx.Normalize 复合化为 (tenant_id, platform)（开发库 pg_indexes 实测）。
// 键位若退回单列，症状不是报错而是闸门静默失效——第二个租户 MarkBlocked 撞 23505，
// 写侧只记日志不判红，读侧永远读到 not found，熔断对非默认租户等于不存在。
// 锁在 repository/profile_health_tenant_test.go。
type BrowserProfileHealth struct {
	TenantID  uint       `gorm:"not null;default:1;index" json:"tenant_id"`
	ID        uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	Platform  string     `gorm:"column:platform;type:varchar(64);uniqueIndex;not null" json:"platform"`
	Blocked   bool       `gorm:"column:blocked;not null;default:false" json:"blocked"`
	Reason    string     `gorm:"column:reason;type:text;not null;default:''" json:"reason"`
	BlockedAt *time.Time `gorm:"column:blocked_at" json:"blocked_at"`
	UpdatedAt time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

// TableName 表名。
func (BrowserProfileHealth) TableName() string { return "browser_profile_health" }
