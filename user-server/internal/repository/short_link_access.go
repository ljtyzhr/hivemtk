package repository

import (
	"context"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/timeutil"
	"time"

	"gorm.io/gorm"
)

// ShortLinkAccessRepository 短链访问统计仓储接口
type ShortLinkAccessRepository interface {
	Create(ctx context.Context, access *model.ShortLinkAccess) error
	GetByID(ctx context.Context, id uint) (*model.ShortLinkAccess, error)
	GetByShortLinkID(ctx context.Context, shortLinkID uint, page, pageSize int) ([]*model.ShortLinkAccess, int64, error)
	GetDailyStatsByShortLinkID(ctx context.Context, shortLinkID uint, startDate, endDate time.Time) ([]map[string]any, error)
	GetDeviceTypeStatsByShortLinkID(ctx context.Context, shortLinkID uint, startDate, endDate time.Time) ([]map[string]any, error)
	GetAllDailyStats(ctx context.Context, startDate, endDate time.Time) ([]map[string]any, error)
	GetAllDeviceTypeStats(ctx context.Context, startDate, endDate time.Time) ([]map[string]any, error)
	GetAllShortLinksBasicStats(ctx context.Context, startDate, endDate time.Time) ([]map[string]any, error)
	DeleteByShortLinkID(ctx context.Context, shortLinkID uint) error
}

type shortLinkAccessRepository struct {
	db *gorm.DB
}

// NewShortLinkAccessRepository 创建短链访问统计仓储实例
func NewShortLinkAccessRepository(db *gorm.DB) ShortLinkAccessRepository {
	return &shortLinkAccessRepository{db: db}
}

func (r *shortLinkAccessRepository) Create(ctx context.Context, access *model.ShortLinkAccess) error {
	return r.db.WithContext(ctx).Create(access).Error
}

func (r *shortLinkAccessRepository) DeleteByShortLinkID(ctx context.Context, shortLinkID uint) error {
	return r.db.WithContext(ctx).Where("short_link_id = ?", shortLinkID).Delete(&model.ShortLinkAccess{}).Error
}

func (r *shortLinkAccessRepository) GetByID(ctx context.Context, id uint) (*model.ShortLinkAccess, error) {
	var access model.ShortLinkAccess
	err := r.db.WithContext(ctx).First(&access, id).Error
	if err != nil {
		return nil, err
	}
	return &access, nil
}

// GetByShortLinkID 分页列出某短链的访问流水。
//
// 两个夹取都有真实来源：`page = 0` ⇒ offset 为负 ⇒ PG 直接报
// "OFFSET must not be negative"；`pageSize = 0` ⇒ GORM 的 Limit(0) 等于**不加 limit**。
// 后者不是假想 —— `service/short_link.go` 的统计口正是 `GetByShortLinkID(ctx, id, 1, 0)`：
// 它只取 total、把行丢掉，旧代码因此每统计一次就把该链接的全部流水捞进内存再扔。
//
// Count 与 Find 各起一条 query：同一条链上先 Count 再 Find 靠的是 GORM 内部
// 子句还原侥幸能跑（判例 #133 / #150）。
func (r *shortLinkAccessRepository) GetByShortLinkID(ctx context.Context, shortLinkID uint, page, pageSize int) ([]*model.ShortLinkAccess, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}

	base := func() *gorm.DB {
		return r.db.WithContext(ctx).Model(&model.ShortLinkAccess{}).Where("short_link_id = ?", shortLinkID)
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var accesses []*model.ShortLinkAccess
	offset := (page - 1) * pageSize
	if err := base().Offset(offset).Limit(pageSize).Order("access_time DESC").Find(&accesses).Error; err != nil {
		return nil, 0, err
	}

	return accesses, total, nil
}

func (r *shortLinkAccessRepository) GetDailyStatsByShortLinkID(ctx context.Context, shortLinkID uint, startDate, endDate time.Time) ([]map[string]any, error) {
	query := r.db.WithContext(ctx).Model(&model.ShortLinkAccess{}).
		Select("DATE(access_time) as date, COUNT(*) as count").
		Where("short_link_id = ?", shortLinkID)

	if !startDate.IsZero() {
		query = query.Where("DATE(access_time) >= ?", timeutil.BusinessDate(startDate))
	}

	if !endDate.IsZero() {
		query = query.Where("DATE(access_time) <= ?", timeutil.BusinessDate(endDate))
	}

	var results []map[string]any
	err := query.Group("DATE(access_time)").Order("date").Scan(&results).Error
	return results, err
}

func (r *shortLinkAccessRepository) GetDeviceTypeStatsByShortLinkID(ctx context.Context, shortLinkID uint, startDate, endDate time.Time) ([]map[string]any, error) {
	query := r.db.WithContext(ctx).Model(&model.ShortLinkAccess{}).
		Select("device_type, COUNT(*) as count").
		Where("short_link_id = ?", shortLinkID)

	if !startDate.IsZero() {
		query = query.Where("DATE(access_time) >= ?", timeutil.BusinessDate(startDate))
	}

	if !endDate.IsZero() {
		query = query.Where("DATE(access_time) <= ?", timeutil.BusinessDate(endDate))
	}

	var results []map[string]any
	err := query.Group("device_type").Order("count DESC").Scan(&results).Error
	return results, err
}

func (r *shortLinkAccessRepository) GetAllDailyStats(ctx context.Context, startDate, endDate time.Time) ([]map[string]any, error) {
	query := r.db.WithContext(ctx).Model(&model.ShortLinkAccess{}).
		Select("DATE(access_time) as date, COUNT(*) as count")

	if !startDate.IsZero() {
		query = query.Where("DATE(access_time) >= ?", timeutil.BusinessDate(startDate))
	}

	if !endDate.IsZero() {
		query = query.Where("DATE(access_time) <= ?", timeutil.BusinessDate(endDate))
	}

	var results []map[string]any
	err := query.Group("DATE(access_time)").Order("date").Scan(&results).Error
	return results, err
}

func (r *shortLinkAccessRepository) GetAllDeviceTypeStats(ctx context.Context, startDate, endDate time.Time) ([]map[string]any, error) {
	query := r.db.WithContext(ctx).Model(&model.ShortLinkAccess{}).
		Select("device_type, COUNT(*) as count")

	if !startDate.IsZero() {
		query = query.Where("DATE(access_time) >= ?", timeutil.BusinessDate(startDate))
	}

	if !endDate.IsZero() {
		query = query.Where("DATE(access_time) <= ?", timeutil.BusinessDate(endDate))
	}

	var results []map[string]any
	err := query.Group("device_type").Order("count DESC").Scan(&results).Error
	return results, err
}

func (r *shortLinkAccessRepository) GetAllShortLinksBasicStats(ctx context.Context, startDate, endDate time.Time) ([]map[string]any, error) {
	query := r.db.WithContext(ctx).Table("short_links sl").
		Select("sl.id, sl.short_code, sl.original_url AS title, COUNT(sla.id) as access_count").
		Joins("LEFT JOIN short_link_accesses sla ON sl.id = sla.short_link_id")

	if !startDate.IsZero() {
		query = query.Where("DATE(sla.access_time) >= ? OR sla.access_time IS NULL", timeutil.BusinessDate(startDate))
	}

	if !endDate.IsZero() {
		query = query.Where("DATE(sla.access_time) <= ? OR sla.access_time IS NULL", timeutil.BusinessDate(endDate))
	}

	var results []map[string]any
	err := query.Group("sl.id, sl.short_code, sl.original_url").Order("access_count DESC").Scan(&results).Error
	return results, err
}
