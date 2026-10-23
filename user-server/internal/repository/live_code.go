package repository

import (
	"context"
	"hivemtk-user/internal/model"
	"time"

	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// LiveCodeRepository 活码仓储接口
type LiveCodeRepository interface {
	Create(ctx context.Context, liveCode *model.LiveCode) error
	Update(ctx context.Context, liveCode *model.LiveCode) error
	Delete(ctx context.Context, id string) error
	GetAvailableLiveCodes(ctx context.Context) ([]*model.LiveCode, error)
	GetByID(ctx context.Context, id string) (*model.LiveCode, error)
	GetByShortLink(ctx context.Context, shortLink string) (*model.LiveCode, error)
	GetList(ctx context.Context, page, pageSize int, name, status string) ([]*model.LiveCode, int64, error)
	IncrementClicks(ctx context.Context, id string) error
}

type liveCodeRepository struct {
	db *gorm.DB
}

// NewLiveCodeRepository 创建活码仓储实例
func NewLiveCodeRepository(db *gorm.DB) LiveCodeRepository {
	return &liveCodeRepository{db: db}
}

func (r *liveCodeRepository) Create(ctx context.Context, liveCode *model.LiveCode) error {
	return r.db.WithContext(ctx).Create(liveCode).Error
}

func (r *liveCodeRepository) Update(ctx context.Context, liveCode *model.LiveCode) error {
	return _db.SaveWholeRow(r.db.WithContext(ctx), liveCode)
}

func (r *liveCodeRepository) Delete(ctx context.Context, id string) error {
	// ShortLink 带唯一索引，软删行会占用短链导致同名重建撞键，保持物理删除；
	// 命中 0 行即活码不存在（已删/别租户），不能回"删除成功"。
	return _db.EnsureAffected(r.db.WithContext(ctx).Unscoped().Where("id = ?", id).Delete(&model.LiveCode{}))
}

func (r *liveCodeRepository) GetAvailableLiveCodes(ctx context.Context) ([]*model.LiveCode, error) {
	var liveCodes []*model.LiveCode
	now := time.Now()

	err := r.db.WithContext(ctx).Where("status = ? AND created_at > ?", 1, now.AddDate(0, 0, -7)).
		Find(&liveCodes).Error
	return liveCodes, err
}

func (r *liveCodeRepository) IncrementClicks(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Model(&model.LiveCode{}).
		Where("id = ?", id).
		UpdateColumns(map[string]interface{}{
			"total_clicks": gorm.Expr("total_clicks + 1"),
			"daily_clicks": gorm.Expr("daily_clicks + 1"),
		}).Error
}

func (r *liveCodeRepository) GetByID(ctx context.Context, id string) (*model.LiveCode, error) {
	var liveCode model.LiveCode
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&liveCode).Error
	if err != nil {
		return nil, err
	}

	if liveCode.ShortDomainID > 0 {
		var shortDomain model.DomainPool
		if err := r.db.WithContext(ctx).Where("id = ?", liveCode.ShortDomainID).First(&shortDomain).Error; err == nil {
			liveCode.ShortDomain = &shortDomain
		}
	}

	if liveCode.EntryDomainID > 0 {
		var entryDomain model.DomainPool
		if err := r.db.WithContext(ctx).Where("id = ?", liveCode.EntryDomainID).First(&entryDomain).Error; err == nil {
			liveCode.EntryDomain = &entryDomain
		}
	}

	if liveCode.LandingDomainID > 0 {
		var landingDomain model.DomainPool
		if err := r.db.WithContext(ctx).Where("id = ?", liveCode.LandingDomainID).First(&landingDomain).Error; err == nil {
			liveCode.LandingDomain = &landingDomain
		}
	}

	return &liveCode, nil
}

func (r *liveCodeRepository) GetByShortLink(ctx context.Context, shortLink string) (*model.LiveCode, error) {
	var liveCode model.LiveCode
	err := r.db.WithContext(ctx).Where("short_link = ?", shortLink).First(&liveCode).Error
	if err != nil {
		return nil, err
	}

	if liveCode.ShortDomainID > 0 {
		var shortDomain model.DomainPool
		if err := r.db.WithContext(ctx).Where("id = ?", liveCode.ShortDomainID).First(&shortDomain).Error; err == nil {
			liveCode.ShortDomain = &shortDomain
		}
	}

	if liveCode.EntryDomainID > 0 {
		var entryDomain model.DomainPool
		if err := r.db.WithContext(ctx).Where("id = ?", liveCode.EntryDomainID).First(&entryDomain).Error; err == nil {
			liveCode.EntryDomain = &entryDomain
		}
	}

	if liveCode.LandingDomainID > 0 {
		var landingDomain model.DomainPool
		if err := r.db.WithContext(ctx).Where("id = ?", liveCode.LandingDomainID).First(&landingDomain).Error; err == nil {
			liveCode.LandingDomain = &landingDomain
		}
	}

	return &liveCode, nil
}

func (r *liveCodeRepository) GetList(ctx context.Context, page, pageSize int, name, status string) ([]*model.LiveCode, int64, error) {
	// page/pageSize 在此夹取：`(page-1)*pageSize` 在 page<=0 时给出负 offset
	// （PG 直接报错），pageSize 无上界时一条 `page_size=100000` 就把整表读进内存。
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	var liveCodes []*model.LiveCode
	var total int64

	query := r.db.WithContext(ctx).Model(&model.LiveCode{})

	if name != "" {
		query = query.Where("name LIKE ?", "%"+name+"%")
	}

	if status != "" {
		if status == "1" {
			query = query.Where("status = ?", 1)
		} else if status == "0" {
			query = query.Where("status = ?", 0)
		}
	}

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err = query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&liveCodes).Error
	if err != nil {
		return nil, 0, err
	}

	for _, liveCode := range liveCodes {
		if liveCode.ShortDomainID > 0 {
			var shortDomain model.DomainPool
			if err := r.db.WithContext(ctx).Where("id = ?", liveCode.ShortDomainID).First(&shortDomain).Error; err == nil {
				liveCode.ShortDomain = &shortDomain
			}
		}

		if liveCode.EntryDomainID > 0 {
			var entryDomain model.DomainPool
			if err := r.db.WithContext(ctx).Where("id = ?", liveCode.EntryDomainID).First(&entryDomain).Error; err == nil {
				liveCode.EntryDomain = &entryDomain
			}
		}

		if liveCode.LandingDomainID > 0 {
			var landingDomain model.DomainPool
			if err := r.db.WithContext(ctx).Where("id = ?", liveCode.LandingDomainID).First(&landingDomain).Error; err == nil {
				liveCode.LandingDomain = &landingDomain
			}
		}
	}

	return liveCodes, total, nil
}
