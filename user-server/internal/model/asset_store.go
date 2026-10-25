// Package model T5 资产市场商店化（见 user-server/docs/architecture/design/T5-asset-store.md）
//
// 五张新表：asset_listings（上架单主表）/ asset_versions（不可变版本）/
// asset_reports（举报）/ asset_ratings（评分）/ extension_usage_monthly（用量月账）。
// 建表登记：internal/service/asset_store_register.go init() 里
// db.RegisterExtraModels（与本仓 T4 先例一致：service→db 反向引用避开 model→db 成环）。
package model

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// 资产上架状态机取值（T5 §4.1）
const (
	AssetListingDraft        = "draft"
	AssetListingSubmitted    = "submitted"
	AssetListingAutoCheck    = "auto_check"
	AssetListingManualReview = "manual_review"
	AssetListingPublished    = "published"
	AssetListingRejected     = "rejected"
	AssetListingSuspended    = "suspended"
)

// assetListingTransitions 状态跃迁表（单点判定，对齐 kb_release.go 写法）
var assetListingTransitions = map[string]map[string]bool{
	AssetListingDraft:        {AssetListingSubmitted: true},
	AssetListingSubmitted:    {AssetListingAutoCheck: true, AssetListingRejected: true},
	AssetListingAutoCheck:    {AssetListingManualReview: true, AssetListingRejected: true},
	AssetListingManualReview: {AssetListingPublished: true, AssetListingRejected: true},
	AssetListingPublished:    {AssetListingSuspended: true},
	AssetListingRejected:     {AssetListingSubmitted: true},
	AssetListingSuspended:    {AssetListingManualReview: true, AssetListingDraft: true},
}

// AssetListingTransitionAllowed 上架单状态跃迁判定
func AssetListingTransitionAllowed(from, to string) bool {
	return assetListingTransitions[from][to]
}

// 举报分类与状态
const (
	AssetReportPolicyViolation = "policy_violation"
	AssetReportMalicious       = "malicious"
	AssetReportInfringement    = "infringement"
	AssetReportOther           = "other"

	AssetReportOpen      = "open"
	AssetReportConfirmed = "confirmed"
	AssetReportDismissed = "dismissed"
)

// 结算状态（T5 §7.2）
const (
	ExtensionSettlePending  = "pending"
	ExtensionSettleBilled   = "billed"
	ExtensionSettlePaid     = "paid"
	ExtensionSettleDisputed = "disputed"
)

// AssetListing 上架单（商店化主表）
type AssetListing struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID            int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	AssetCode     string `gorm:"size:64;uniqueIndex;not null" json:"asset_code"`
	ContributorID int64  `gorm:"index" json:"contributor_id"`
	AssetType     string `gorm:"size:32;index;not null" json:"asset_type"` // prompt/knowledge/tool/extension
	Name          string `gorm:"size:128;not null" json:"name"`
	Industry      string `gorm:"size:32" json:"industry"`
	Summary       string `gorm:"type:text" json:"summary"`
	Status        string `gorm:"size:16;index;not null;default:draft" json:"status"`
	// PolicyDeclaration 政策声明（GPT Store attestation 对齐）：数据收集项/用途/第三方 ToS/隐私政策链接
	PolicyDeclaration json.RawMessage `gorm:"type:jsonb" json:"policy_declaration,omitempty"`
	// PermissionDeclarations 扩展类：manifest 每条权限 → 用途说明（Chrome Web Store 对齐）
	PermissionDeclarations json.RawMessage `gorm:"type:jsonb" json:"permission_declarations,omitempty"`
	RejectReason           string          `gorm:"type:text" json:"reject_reason,omitempty"`
	ReviewedBy             int64           `json:"reviewed_by,omitempty"`
	ReviewedAt             *time.Time      `json:"reviewed_at,omitempty"`
	CurrentVersionID       int64           `json:"current_version_id,omitempty"`
	DownloadCount          int64           `gorm:"not null;default:0" json:"download_count"`
	RatingAvg              float64         `gorm:"type:numeric(3,2);not null;default:0" json:"rating_avg"`
	RatingCount            int             `gorm:"not null;default:0" json:"rating_count"`

	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (AssetListing) TableName() string { return "asset_listings" }

// AssetVersion 版本（不可变，每次提审一行）
type AssetVersion struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID         int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	ListingID  int64  `gorm:"index;not null" json:"listing_id"`
	Version    string `gorm:"size:32;not null" json:"version"`
	Changelog  string `gorm:"type:text" json:"changelog,omitempty"`
	BundleHash string `gorm:"size:128" json:"bundle_hash,omitempty"`
	BundleRef  string `gorm:"size:256" json:"bundle_ref,omitempty"`
	Status     string `gorm:"size:16;not null;default:draft" json:"status"`
	// AutoCheckResult 自动检查逐项结果
	AutoCheckResult json.RawMessage `gorm:"type:jsonb" json:"auto_check_result,omitempty"`
	RejectReason    string          `gorm:"type:text" json:"reject_reason,omitempty"`
	ReviewedBy      int64           `json:"reviewed_by,omitempty"`
	ReviewedAt      *time.Time      `json:"reviewed_at,omitempty"`
	PublishedAt     *time.Time      `json:"published_at,omitempty"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (AssetVersion) TableName() string { return "asset_versions" }

// AssetReport 举报
type AssetReport struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID         int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	ListingID  int64      `gorm:"index;not null" json:"listing_id"`
	VersionID  int64      `json:"version_id,omitempty"`
	ReporterID int64      `gorm:"index" json:"reporter_id"`
	Category   string     `gorm:"size:32;not null" json:"category"`
	Detail     string     `gorm:"type:text" json:"detail,omitempty"`
	Status     string     `gorm:"size:16;index;not null;default:open" json:"status"`
	HandledBy  int64      `json:"handled_by,omitempty"`
	HandledAt  *time.Time `json:"handled_at,omitempty"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (AssetReport) TableName() string { return "asset_reports" }

// AssetRating 评分（purchase 门槛，一人一评可更新）
type AssetRating struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID         int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	ListingID  int64  `gorm:"uniqueIndex:uk_asset_rating_listing_user,priority:1;not null" json:"listing_id"`
	UserID     int64  `gorm:"uniqueIndex:uk_asset_rating_listing_user,priority:2;not null" json:"user_id"`
	PurchaseID int64  `json:"purchase_id,omitempty"`
	Score      int    `gorm:"not null" json:"score"` // 1~5
	Comment    string `gorm:"type:text" json:"comment,omitempty"`
	Status     string `gorm:"size:16;not null;default:visible" json:"status"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (AssetRating) TableName() string { return "asset_ratings" }

// ExtensionUsageMonthly 扩展用量归因月账
type ExtensionUsageMonthly struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID           int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	ListingID    int64  `gorm:"uniqueIndex:uk_ext_usage_dims,priority:1;not null" json:"listing_id"`
	VersionID    int64  `json:"version_id,omitempty"`
	ExtensionKey string `gorm:"size:128;uniqueIndex:uk_ext_usage_dims,priority:2;not null" json:"extension_key"`
	// Period 'YYYY-MM'
	Period       string  `gorm:"size:7;uniqueIndex:uk_ext_usage_dims,priority:3;not null" json:"period"`
	TenantCode   string  `gorm:"size:32" json:"tenant_code,omitempty"`
	UsageCount   int64   `gorm:"not null;default:0" json:"usage_count"`
	TokensIn     int64   `gorm:"not null;default:0" json:"tokens_in"`
	TokensOut    int64   `gorm:"not null;default:0" json:"tokens_out"`
	RevenueShare float64 `gorm:"type:numeric(12,4);not null;default:0" json:"revenue_share"`
	SettleStatus string  `gorm:"size:16;not null;default:pending" json:"settle_status"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (ExtensionUsageMonthly) TableName() string { return "extension_usage_monthly" }
