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
	ID        uint      `gorm:"primaryKey" json:"id"`
	Code      string    `gorm:"type:varchar(32);uniqueIndex;not null" json:"code"`
	Name      string    `gorm:"type:varchar(100);not null" json:"name"`
	Status    int16     `gorm:"type:smallint;not null;default:1" json:"status"`
	Plan      string    `gorm:"type:varchar(32);default:'free'" json:"plan"`
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
