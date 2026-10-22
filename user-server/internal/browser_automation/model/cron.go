package model

import (
	"time"

	"gorm.io/gorm"
)

// BrowserCronTrigger 定时触发器（调度由 internal/pkg/cron TaskManager 接管）
// CronExpr 存储 5 段表达式（前端展示习惯），注册到 TaskManager 前补秒段转 6 段。
// TimeZone（G20/v3.40.0）：IANA 时区名，注册时前缀 CRON_TZ=（robfig v3 原生支持），
// 空=沿用服务器本地时区（存量行为不变）。
type BrowserCronTrigger struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID       uint   `gorm:"primaryKey" json:"id"`
	TaskID   uint   `gorm:"column:task_id;uniqueIndex;not null" json:"task_id"`
	CronExpr string `gorm:"column:cron_expr;size:128;not null" json:"cron_expr"`           // "*/5 * * * *"
	TimeZone string `gorm:"column:time_zone;size:64;not null;default:''" json:"time_zone"` // "Asia/Shanghai"，空=服务器本地
	// Enabled 刻意**不声明** gorm `default:true`。
	//
	// GORM 对带 default 标签的字段，零值（false）会被当成"没赋值"从 INSERT 里剔除，
	// 交给列默认值兜底 —— 而列默认值是 true。于是「显式建一个停用状态的触发器」
	// （POST /api/browser-automation/cron 带 enabled:false）会落库成 enabled=true，
	// 且 service.applySchedule 读到内存里的 true 照样把它注册进调度器，
	// 于是这条用户明确要求"先别跑"的定时任务会在 09:00 准时开火。
	//
	// 缺省语义放在 service 层（CreateCronReq.Enabled 为 nil ⇒ 默认 true），
	// 那是唯一知道"用户到底传没传"的地方；模型层只负责把传下来的值原样落库。
	// 列上的 DEFAULT true 保留给裸 SQL 写入，不影响本条路径。
	Enabled   bool       `gorm:"column:enabled;index" json:"enabled"`
	NextRunAt *time.Time `gorm:"column:next_run_at" json:"next_run_at,omitempty"`
	LastRunAt *time.Time `gorm:"column:last_run_at" json:"last_run_at,omitempty"`

	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (BrowserCronTrigger) TableName() string { return "browser_cron_triggers" }
