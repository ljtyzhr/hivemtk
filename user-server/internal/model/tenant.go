package model

import (
	"time"

	"gorm.io/gorm"
)

// TenantStatus 租户状态常量（specs/001-saas-multi-tenant data-model.md）
const (
	// TenantStatusActive 正常
	TenantStatusActive int16 = 1
	// TenantStatusSuspended 停用：登录受限、后台任务跳过，数据保留
	TenantStatusSuspended int16 = 2
)

// IsValidTenantStatus 校验租户状态取值
func IsValidTenantStatus(s int16) bool {
	return s == TenantStatusActive || s == TenantStatusSuspended
}

// Tenant 租户：SaaS 多租户模式的独立经营主体，几乎所有业务数据的归属根。
//
// 离线单机部署（TENANT_MODE=off）不创建租户行：全部数据归属
// tenant.DefaultTenantID，tenants 表存在但为空属于正常状态。
type Tenant struct {
	ID     uint   `gorm:"primaryKey" json:"id"`
	Code   string `gorm:"type:varchar(32);uniqueIndex;not null" json:"code"`
	Name   string `gorm:"type:varchar(100);not null" json:"name"`
	Status int16  `gorm:"type:smallint;not null;default:1" json:"status"`
	Plan   string `gorm:"type:varchar(32);default:'free'" json:"plan"`

	// ---- 开户登记（平台监管必需项）----
	CompanyName string `gorm:"type:varchar(200);not null;default:''" json:"company_name"`
	// LegalPerson 负责人姓名。平台履行监管责任时"能把人找到"的最小集合之一
	LegalPerson string `gorm:"type:varchar(64);not null;default:''" json:"legal_person"`
	Phone       string `gorm:"type:varchar(32);not null;default:''" json:"phone"`
	Email       string `gorm:"type:varchar(128);not null;default:''" json:"email"`
	// 选填但法律上通常需要，留空时平台端列表高亮提示补录
	LicenseNo string `gorm:"type:varchar(64)" json:"license_no"`
	Address   string `gorm:"type:varchar(255)" json:"address"`

	RegMode          string `gorm:"type:varchar(16);not null;default:'saas'" json:"reg_mode"`
	PlatformSync     string `gorm:"type:varchar(16);not null;default:'pending';index" json:"platform_sync"`
	PlatformSyncErr  string `gorm:"type:text" json:"platform_sync_error"`
	AgreementVersion string `gorm:"type:varchar(32);not null;default:''" json:"agreement_version"`
	// RegulatoryAck 用户是否已确认监管告知（留痕字段，管理端不可改）
	RegulatoryAck bool `gorm:"not null;default:false" json:"regulatory_ack"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (Tenant) TableName() string { return "tenants" }

// BeforeCreate GORM 钩子：兜底状态默认值
func (t *Tenant) BeforeCreate(tx *gorm.DB) error {
	if t.Status == 0 {
		t.Status = TenantStatusActive
	}
	return nil
}

// IsActive 租户是否正常（后台扇出/登录判定用）
func (t *Tenant) IsActive() bool { return t.Status == TenantStatusActive }

// ── 开户登记（平台监管责任）────────────────────────────────────────────
//
// 需求原文：「尤其是法律要求的平台端监管责任，租户开户必须收集，电话，
// 邮箱，公司名称，负责人等。注意离线独立部署情况」。
//
// 为什么在 Tenant 上加列而不是单开一张登记表：
// 租户本身就是经营主体，「公司名称 / 负责人」是它的自然属性；
// 而「是否已上报平台、上报失败原因、重试次数」是**同步状态**，属于另一类数据。
// 两者混在一张表里会让"改一下负责人"把同步状态一起写掉。
// 故：静态登记信息在 Tenant，动态同步状态由 platform 模块的日志表承载。

// 登记模式：SaaS 自助注册 / 离线独立部署
const (
	TenantRegModeSaaS    = "saas"
	TenantRegModeOffline = "offline"
)

// 登记同步态。skipped 不是错误 —— 它表示"离线部署，本来就没打算上报"。
const (
	TenantSyncPending = "pending"
	TenantSyncSynced  = "synced"
	TenantSyncFailed  = "failed"
	TenantSyncSkipped = "skipped"
)

// RegulatoryAck 监管告知确认位：用户勾选后置 true。
//
// 它是**留痕字段**，管理端不允许改写 —— 改了等于伪造用户同意过的内容。
type RegulatoryAck = bool
