package model

import "time"

// BridgeSelectorHealth 桥接扩展的**选择器命中率**累计计数（B1-a）。
//
// 为什么需要它：桥接（用户机器上的浏览器扩展）靠 DOM 选择器读写目标平台页面。
// 平台改版一次，选择器就静默失效——消息不再被采集、回复发不出去，而后端看到的
// 只是「这个账号没消息了」，与「这个账号今天没客户」在指标上长得一模一样。
// 扩展侧无法自行判断「平台改版了」还是「今天没客户」，所以它只做一件事：
// **把「试了多少次、命中了多少次」如实上报**，判定交回服务端（阈值走参数中心）。
//
// 计数是**单调累计**的而不是每秒快照：扩展可能离线、可能被浏览器休眠，
// 快照会漏掉全部离线时段；累计值在服务端加总，任意时刻的命中率都可复算。
// 需要「近期」口径时按 last_report_at 判断新鲜度，而不是靠窗口切分。
//
// 归属：tenant_id 由隔离 callback 从 ctx 盖章；没有 user_id 字段——上报通道只认
// 实例级桥接凭证、不带用户身份，健康度是实例级事实，与「谁认领了这个账号」无关。
type BridgeSelectorHealth struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID uint `gorm:"primaryKey;autoIncrement" json:"id"`

	// Channel 渠道标识（douyin / xiaohongshu / xianyu / kuaishou / tiktok …），
	// 与 bridge_accounts.channel 同口径。
	Channel string `gorm:"type:varchar(32);not null;uniqueIndex:uk_bridge_sel_health" json:"channel"`

	// Field 选择器字段名，取值收敛在 service.selectorHealthFields（conversationList /
	// messageList / messageItem / text / input / send），与扩展侧 SELECTOR_FIELDS 同源。
	// 白名单而不是自由文本：字段名由上报方决定时，一行坏数据就能让这张表按渠道无限长。
	Field string `gorm:"type:varchar(32);not null;uniqueIndex:uk_bridge_sel_health" json:"field"`

	// Attempts 累计「尝试定位」次数；Hits 其中「拿到非空结果」的次数。
	// 两者的比值才是命中率——只报命中数时无法区分「一直好」与「一直没被调用」。
	Attempts uint64 `gorm:"not null;default:0" json:"attempts"`
	Hits     uint64 `gorm:"not null;default:0" json:"hits"`

	// FirstSeenAt 第一次上报时间（判断这条健康度是不是本轮新增的渠道）。
	FirstSeenAt time.Time `gorm:"not null" json:"first_seen_at"`

	// LastReportAt 最后一次上报时间。新鲜度判据：超过 N 天没更新说明扩展已经不在跑，
	// 此时「命中率 100%」是历史读数，不是现在的状态。
	LastReportAt time.Time `gorm:"index;not null" json:"last_report_at"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (BridgeSelectorHealth) TableName() string { return "bridge_selector_health" }
