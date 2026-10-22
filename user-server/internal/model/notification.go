package model

import "time"

// Notification 系统通知模型（商户端内置通知中心）
//
// 设计目标：
//   - 私域部署下，平台端下发的系统通知、版本公告、安全告警、租户自定义消息
//     全部落地到本表，供 user-web 通知中心 / 顶部铃铛 badge 使用
//   - 不依赖外部 IM，纯站内通知
//   - 平台端发布的消息经 notice_sync 拉取镜像到本表（保证 user-web 离线仍可读）
//
// 字段说明：
//   - UserID  = 0 表示全体用户（admin/manager 可见）
//   - Type    枚举: info/warning/error/success/announcement/platform_notice
//   - Link    可选外链（公告/详情跳转）
//   - Metadata 预留 JSON 字段，存扩展信息
//
// 两类通知的已读语义**不同**，读面必须分开处理：
//   - 本地业务通知（Type ∈ info/warning/error/success/announcement）：
//     受众是管理员/审核人，"任一人读过即算"是对的，已读位就是本行的 is_read。
//   - 平台消息（Type = platform_notice）：一条消息对 N 个用户，
//     is_read 是**消息行上的单一布尔位**，A 点已读会把 B 的那条也变已读
//     ⇒ 未读角标被一个人清零。故平台消息的已读改由 notification_reads
//     回执表按人记录，本行的 is_read 不参与判定。
type Notification struct {
	// TenantID 租户号。参与幂等唯一键，故 priority:1。
	TenantID uint `gorm:"not null;default:1;index;uniqueIndex:uq_notification_tenant_source_msg,priority:1,where:source_msg_id > 0" json:"tenant_id"`

	ID        uint       `json:"id" gorm:"primaryKey"`
	UserID    uint       `json:"user_id" gorm:"index;not null;default:0"`
	Type      string     `json:"type" gorm:"size:32;index;not null;default:'info'"`
	Title     string     `json:"title" gorm:"size:255;not null"`
	Content   string     `json:"content" gorm:"type:text"`
	Link      string     `json:"link" gorm:"size:512"`
	IsRead    bool       `json:"is_read" gorm:"index;not null;default:false"`
	ReadAt    *time.Time `json:"read_at,omitempty"`
	Metadata  string     `json:"metadata" gorm:"type:text"`
	CreatedAt time.Time  `json:"created_at" gorm:"index"`
	UpdatedAt time.Time  `json:"updated_at"`

	// Level 消息级别。平台下发消息取平台侧原值，本地业务通知恒 info。
	Level string `json:"level" gorm:"size:16;index;not null;default:'info'"`

	// SourceMsgID 来源平台消息 ID；0 = 本地产生。
	//
	// 幂等键 = (tenant_id, source_msg_id)：平台消息被重复投递时靠它去重。
	// 唯一索引是**部分唯一**（where source_msg_id > 0）——普通唯一索引会让
	// 第二条本地业务通知撞键，而本地通知的 source_msg_id 恒为 0。
	SourceMsgID int64 `json:"source_msg_id" gorm:"not null;default:0;index;uniqueIndex:uq_notification_tenant_source_msg,priority:2,where:source_msg_id > 0"`

	// Source 消息来源：platform（平台下发）/ local（本地业务通知）。
	Source string `json:"source" gorm:"size:16;index;not null;default:'local'"`
}

// TableName 表名
func (n *Notification) TableName() string {
	return "notifications"
}

// Notification 通知类型枚举
const (
	NotificationTypeInfo         = "info"
	NotificationTypeWarning      = "warning"
	NotificationTypeSuccess      = "success"
	NotificationTypeAnnouncement = "announcement"

	// NotificationTypePlatformNotice 平台下发的站内消息。
	//
	// 与上面四个枚举并列而非取代它们：那四个是本地业务通知（待办逾期、
	// 待归因坏例…）的类型；平台消息有自己的来源标记（Source=platform），
	// 类型列用来把两类在通知中心里区分开。
	NotificationTypePlatformNotice = "platform_notice"
)

// 通知来源枚举。
const (
	// NotificationSourcePlatform 平台下发。
	NotificationSourcePlatform = "platform"
	// NotificationSourceLocal 本地业务通知。
	NotificationSourceLocal = "local"
)

// 消息级别枚举。与平台侧 model.MessageLevel* 同名取值。
const (
	NotificationLevelInfo    = "info"
	NotificationLevelWarning = "warning"
	NotificationLevelError   = "error"
	NotificationLevelSuccess = "success"
)

// IsValidNotificationLevel 校验消息级别是否在枚举内。
func IsValidNotificationLevel(level string) bool {
	switch level {
	case NotificationLevelInfo, NotificationLevelWarning,
		NotificationLevelError, NotificationLevelSuccess:
		return true
	default:
		return false
	}
}
