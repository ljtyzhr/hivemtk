// Package service T5 资产市场商店化 service（状态机收口 + 审核 + 评分举报 + 用量归因）。
//
// 设计：user-server/docs/architecture/design/T5-asset-store.md
// 双轨事实源：本地 asset_listings 为事实源，平台 contributor submit 保持兼容
// （本层只管本地状态机；平台转发走既有 asset_market 代理）。
package service

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
)

// AssetStoreService 商店化域服务
type AssetStoreService struct {
	db *gorm.DB
}

// NewAssetStoreService 构造
func NewAssetStoreService(db *gorm.DB) *AssetStoreService {
	return &AssetStoreService{db: db}
}

// AssetStoreSubmitLimit 7 天提审限额（Chrome Web Store 新账号限额对齐）
const AssetStoreSubmitLimit = 10

// CreateListing 创建上架单（draft）
func (s *AssetStoreService) CreateListing(tenantID, contributorID uint, assetCode, assetType, name, industry, summary string, policy, permissions []byte) (*model.AssetListing, error) {
	if assetCode == "" || assetType == "" || name == "" {
		return nil, errors.New("asset_code/asset_type/name 不能为空")
	}
	var cnt int64
	if err := s.db.Model(&model.AssetListing{}).Where("asset_code = ?", assetCode).Count(&cnt).Error; err != nil {
		return nil, err
	}
	if cnt > 0 {
		return nil, fmt.Errorf("asset_code %s 已存在", assetCode)
	}
	l := &model.AssetListing{
		TenantID:               tenantID,
		AssetCode:              assetCode,
		ContributorID:          int64(contributorID),
		AssetType:              assetType,
		Name:                   name,
		Industry:               industry,
		Summary:                summary,
		Status:                 model.AssetListingDraft,
		PolicyDeclaration:      policy,
		PermissionDeclarations: permissions,
	}
	if err := s.db.Create(l).Error; err != nil {
		return nil, err
	}
	return l, nil
}

// SubmitListing 提审：draft/rejected → submitted。
// 政策声明是提审前置必填（GPT Store attestation）；7 天提审限额拦截。
func (s *AssetStoreService) SubmitListing(tenantID uint, listingID int64) error {
	var l model.AssetListing
	if err := s.db.Where("id = ? AND tenant_id = ?", listingID, tenantID).First(&l).Error; err != nil {
		return fmt.Errorf("上架单不存在: %w", err)
	}
	if len(l.PolicyDeclaration) == 0 {
		return errors.New("提审前必须填写政策声明（数据收集项与用途）")
	}
	// 发布限额：该贡献者 7 天内提交数
	var weekCnt int64
	weekAgo := time.Now().AddDate(0, 0, -7)
	if err := s.db.Model(&model.AssetVersion{}).
		Where("tenant_id = ? AND created_at >= ?", tenantID, weekAgo).
		Where("listing_id IN (?)", s.db.Model(&model.AssetListing{}).Select("id").Where("contributor_id = ?", l.ContributorID)).
		Count(&weekCnt).Error; err != nil {
		return err
	}
	if weekCnt >= AssetStoreSubmitLimit {
		return fmt.Errorf("7 天内提审次数已达上限 %d，请稍后再试", AssetStoreSubmitLimit)
	}
	if !model.AssetListingTransitionAllowed(l.Status, model.AssetListingSubmitted) {
		return fmt.Errorf("状态 %s 不允许提审", l.Status)
	}
	// 版本行（每次提审一行，不可变）
	v := &model.AssetVersion{
		TenantID: tenantID,
		ListingID: l.ID,
		Version:  time.Now().Format("2006.01.02.150405"),
		Status:   model.AssetListingSubmitted,
	}
	if err := s.db.Create(v).Error; err != nil {
		return fmt.Errorf("创建版本记录失败: %w", err)
	}
	return s.db.Model(&l).Updates(map[string]any{
		"status":             model.AssetListingSubmitted,
		"reject_reason":      "",
		"current_version_id": v.ID,
	}).Error
}

// TransitionListing 状态跃迁（审核动作收口；to 状态合法性单点判定）。
// reviewerID/reason 供审核链路写入。
func (s *AssetStoreService) TransitionListing(tenantID uint, listingID int64, to string, reviewerID int64, reason string) error {
	var l model.AssetListing
	if err := s.db.Where("id = ? AND tenant_id = ?", listingID, tenantID).First(&l).Error; err != nil {
		return fmt.Errorf("上架单不存在: %w", err)
	}
	if !model.AssetListingTransitionAllowed(l.Status, to) {
		return fmt.Errorf("非法状态跃迁 %s → %s", l.Status, to)
	}
	now := time.Now()
	updates := map[string]any{
		"status":       to,
		"reviewed_by":  reviewerID,
		"reviewed_at":  now,
		"reject_reason": reason,
	}
	if to == model.AssetListingPublished {
		updates["reject_reason"] = ""
		// 版本级同步发布
		if l.CurrentVersionID > 0 {
			_ = s.db.Model(&model.AssetVersion{}).
				Where("id = ? AND tenant_id = ?", l.CurrentVersionID, tenantID).
				Updates(map[string]any{"status": model.AssetListingPublished, "published_at": now, "reviewed_by": reviewerID}).Error
		}
	}
	if to == model.AssetListingRejected && reason == "" {
		return errors.New("驳回必须填写理由")
	}
	return s.db.Model(&l).Updates(updates).Error
}

// ReviewQueue 审核队列（平台后台）：submitted/auto_check/manual_review
func (s *AssetStoreService) ReviewQueue(assetType string, page, pageSize int) ([]model.AssetListing, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	q := s.db.Model(&model.AssetListing{}).
		Where("status IN ?", []string{model.AssetListingSubmitted, model.AssetListingAutoCheck, model.AssetListingManualReview})
	if assetType != "" {
		q = q.Where("asset_type = ?", assetType)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []model.AssetListing
	if err := q.Order("updated_at ASC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&out).Error; err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// Rate 评分（purchase 门槛：purchase_id 必须能对应到该用户在 local_assets 的购买记录）。
// 一人一评（唯一键 upsert 语义），写后重算 listing 评分冗余列。
func (s *AssetStoreService) Rate(tenantID uint, listingID, userID, purchaseID int64, score int, comment string) error {
	if score < 1 || score > 5 {
		return errors.New("评分必须在 1-5 之间")
	}
	// purchase 门槛：purchase_id 归属当前用户
	var pc int64
	if err := s.db.Model(&model.LocalAsset{}).
		Where("tenant_id = ? AND purchase_id = ? AND source = ?", tenantID, purchaseID, model.AssetSourcePurchased).
		Count(&pc).Error; err != nil {
		return err
	}
	if pc == 0 {
		return errors.New("仅购买者可评分")
	}
	r := &model.AssetRating{
		TenantID:   tenantID,
		ListingID:  listingID,
		UserID:     userID,
		PurchaseID: purchaseID,
		Score:      score,
		Comment:    comment,
		Status:     "visible",
	}
	// upsert：一人一评可更新
	if err := s.db.Where("listing_id = ? AND user_id = ? AND tenant_id = ?", listingID, userID, tenantID).
		FirstOrCreate(r).Error; err != nil {
		return err
	}
	if r.ID > 0 && (r.Score != score || r.Comment != comment) {
		if err := s.db.Model(r).Updates(map[string]any{"score": score, "comment": comment}).Error; err != nil {
			return err
		}
	}
	return s.recalcRating(tenantID, listingID)
}

func (s *AssetStoreService) recalcRating(tenantID uint, listingID int64) error {
	var avg struct {
		Avg float64 `json:"avg"`
		Cnt int64   `json:"cnt"`
	}
	if err := s.db.Model(&model.AssetRating{}).
		Select("COALESCE(AVG(score),0) AS avg, COUNT(*) AS cnt").
		Where("listing_id = ? AND tenant_id = ? AND status = ?", listingID, tenantID, "visible").
		Scan(&avg).Error; err != nil {
		return err
	}
	return s.db.Model(&model.AssetListing{}).Where("id = ? AND tenant_id = ?", listingID, tenantID).
		Updates(map[string]any{"rating_avg": avg.Avg, "rating_count": int(avg.Cnt)}).Error
}

// Report 举报（幂等：同 user+listing+category 存在未结单举报则去重）
func (s *AssetStoreService) Report(tenantID uint, listingID, reporterID int64, category, detail string) (*model.AssetReport, error) {
	validCats := map[string]bool{model.AssetReportPolicyViolation: true, model.AssetReportMalicious: true, model.AssetReportInfringement: true, model.AssetReportOther: true}
	if !validCats[category] {
		return nil, fmt.Errorf("非法举报分类: %s", category)
	}
	var cnt int64
	if err := s.db.Model(&model.AssetReport{}).
		Where("listing_id = ? AND reporter_id = ? AND category = ? AND status = ? AND tenant_id = ?", listingID, reporterID, category, model.AssetReportOpen, tenantID).
		Count(&cnt).Error; err != nil {
		return nil, err
	}
	if cnt > 0 {
		return nil, errors.New("已有同类型未处理举报")
	}
	r := &model.AssetReport{
		TenantID:   tenantID,
		ListingID:  listingID,
		ReporterID: reporterID,
		Category:   category,
		Detail:     detail,
		Status:     model.AssetReportOpen,
	}
	if err := s.db.Create(r).Error; err != nil {
		return nil, err
	}
	return r, nil
}

// HandleReport 举报处理：confirmed 触发 suspended；dismissed 结单。
func (s *AssetStoreService) HandleReport(tenantID uint, reportID int64, handlerID int64, confirm bool, reason string) error {
	var r model.AssetReport
	if err := s.db.Where("id = ? AND tenant_id = ?", reportID, tenantID).First(&r).Error; err != nil {
		return fmt.Errorf("举报不存在: %w", err)
	}
	if r.Status != model.AssetReportOpen {
		return fmt.Errorf("举报已处理（%s）", r.Status)
	}
	now := time.Now()
	status := model.AssetReportDismissed
	if confirm {
		status = model.AssetReportConfirmed
		if err := s.TransitionListing(tenantID, r.ListingID, model.AssetListingSuspended, handlerID, reason); err != nil {
			return fmt.Errorf("下架失败: %w", err)
		}
	}
	return s.db.Model(&r).Updates(map[string]any{"status": status, "handled_by": handlerID, "handled_at": now}).Error
}

// ReportUsage 用量归因上报：(listing_id, extension_key, period) 唯一键 upsert 累加。
// key 缺失落匿名桶（extension_key="<anonymous>"），月账可对账。
func (s *AssetStoreService) ReportUsage(tenantID uint, listingID int64, extensionKey, tenantCode string, usageDelta int64) error {
	if listingID <= 0 {
		return errors.New("listing_id 不能为空")
	}
	if extensionKey == "" {
		extensionKey = "<anonymous>"
	}
	if usageDelta <= 0 {
		return errors.New("usage_delta 必须为正数")
	}
	period := time.Now().Format("2006-01")
	var row model.ExtensionUsageMonthly
	err := s.db.Where("listing_id = ? AND extension_key = ? AND period = ? AND tenant_id = ?", listingID, extensionKey, period, tenantID).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = model.ExtensionUsageMonthly{
			TenantID: tenantID, ListingID: listingID, ExtensionKey: extensionKey,
			Period: period, TenantCode: tenantCode, UsageCount: usageDelta,
			SettleStatus: model.ExtensionSettlePending,
		}
		return s.db.Create(&row).Error
	}
	if err != nil {
		return err
	}
	return s.db.Model(&row).UpdateColumn("usage_count", gorm.Expr("usage_count + ?", usageDelta)).Error
}

// Attribution 月账查询（平台后台对账）
func (s *AssetStoreService) Attribution(period string, page, pageSize int) ([]model.ExtensionUsageMonthly, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 50
	}
	q := s.db.Model(&model.ExtensionUsageMonthly{})
	if period != "" {
		q = q.Where("period = ?", period)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []model.ExtensionUsageMonthly
	if err := q.Order("usage_count DESC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&out).Error; err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// ListReports 举报列表（平台后台）
func (s *AssetStoreService) ListReports(status string, page, pageSize int) ([]model.AssetReport, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	q := s.db.Model(&model.AssetReport{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []model.AssetReport
	if err := q.Order("id DESC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&out).Error; err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// Versions 版本列表（版本号/changelog/发布时间）
func (s *AssetStoreService) Versions(tenantID uint, listingID int64) ([]model.AssetVersion, error) {
	var out []model.AssetVersion
	err := s.db.Where("listing_id = ? AND tenant_id = ?", listingID, tenantID).
		Order("id DESC").Find(&out).Error
	return out, err
}
