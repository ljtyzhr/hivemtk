package model

import (
	"time"
)

// ============================================================
// 租户订阅镜像（003 · ADR-M8 修订 #4）
//
// 部署级**单行**（无 tenant_id）：sync 跑在 SystemScope，带 tenant_id 必踩
// 默认租户串写坑（002 模块镜像 purchaseTenants 同款问题）。本仓单商户单部署，
// 订阅只有「本部署这一份」，行主键恒为 1。
//
// status 列只是判定缓存/展示列；权威 = CurrentStatus() 对
// current_period_end / trial_expires_at 的惰性比较（与平台端同款，修订 #2：
// 锁判定本地化，与平台可达性无关；fetched_at 只用于 Warn，不参与锁决策）。
// ============================================================

// SubscriptionStatus 订阅状态（与平台端枚举同名同义）
const (
	SubscriptionStatusTrial   = "trial"   // 体验期
	SubscriptionStatusActive  = "active"  // 已付费生效
	SubscriptionStatusExpired = "expired" // 已到期（业务面锁定）
)

// TenantSubscriptionMirror 订阅镜像（每租户一行，id = tenant_id）。
//
// 多租户修正（003 生产联调）：001 迁移给全表都加了 tenant_id 列且 tenancyidx
// 对本表生效注入 —— 旧"部署级单行 id=1"设计在第二个租户出现时读写全错
// （写撞主键、读 WHERE id=1 AND tenant_id=N 恒空）。改为 id=tenant_id 每租户
// 一行；off 模式回落 DefaultTenantID=1，行为与旧版一致。
type TenantSubscriptionMirror struct {
	// ID 行锚 = tenant_id（on 模式），off 模式恒 1。
	// 不加 default tag：显式迁移 DDL 已带 DEFAULT 1，模型再声明会与
	// AutoMigrate 撞出 "multiple default values"（42601）。
	ID uint `gorm:"primaryKey" json:"id"`

	// TenantID 所属租户（001 迁移已加列；显式建模让读写与 tenancyidx 注入同值）
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	// Status trial|active|expired —— **缓存列**，权威 = CurrentStatus() 惰性判定
	Status string `gorm:"type:varchar(16);not null;default:''" json:"status"`

	CurrentPeriodEnd *time.Time `json:"current_period_end"`
	TrialExpiresAt   *time.Time `json:"trial_expires_at"`

	// PriceCents 平台下发的月价快照（分），续费墙展示用
	PriceCents int64 `gorm:"type:bigint;not null;default:0" json:"price_cents"`

	// FetchedAt 最近一次从平台同步到本行的时间。
	// 只用于可观测（>1h 记 Warn），**不参与锁决策**（修订 #2：堵死「断网 1h 白嫖」）。
	FetchedAt time.Time `json:"fetched_at"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (TenantSubscriptionMirror) TableName() string { return "tenant_subscription_mirror" }

// HasPeriodData 镜像是否携带任何可用于判定的到期数据。
// false = 从未同步到订阅段（fail-open 放行，修订 #2 唯一的 fail-open 情形）。
func (m *TenantSubscriptionMirror) HasPeriodData() bool {
	if m == nil {
		return false
	}
	return m.CurrentPeriodEnd != nil || m.TrialExpiresAt != nil
}

// CurrentStatus 惰性判定（与平台 TenantSubscription.CurrentStatus 同款逻辑，独立实现）。
//
// 优先级：current_period_end 在未来 → active（已付费用户不再显示体验期，
// 即使 trial 期还没过完）；否则 trial_expires_at 在未来 → trial；否则 expired。
func (m *TenantSubscriptionMirror) CurrentStatus(now time.Time) string {
	if m == nil {
		return SubscriptionStatusExpired
	}
	if m.CurrentPeriodEnd != nil && m.CurrentPeriodEnd.After(now) {
		return SubscriptionStatusActive
	}
	if m.TrialExpiresAt != nil && m.TrialExpiresAt.After(now) {
		return SubscriptionStatusTrial
	}
	return SubscriptionStatusExpired
}
