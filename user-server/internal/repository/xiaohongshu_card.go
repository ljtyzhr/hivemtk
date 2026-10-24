package repository

import (
	"context"
	"errors"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// XiaohongshuCardRepository 小红书卡片仓储接口
type XiaohongshuCardRepository interface {
	Create(ctx context.Context, card *model.XiaohongshuCard) (*model.XiaohongshuCard, error)
	Update(ctx context.Context, card *model.XiaohongshuCard) (*model.XiaohongshuCard, error)
	Delete(ctx context.Context, id uint) error
	GetByID(ctx context.Context, id uint) (*model.XiaohongshuCard, error)
	GetList(ctx context.Context, req CardListFilter) ([]model.XiaohongshuCard, int64, error)
	IncrementViewCount(ctx context.Context, id uint) (*model.XiaohongshuCard, error)
	CreateActivity(ctx context.Context, activity *model.XiaohongshuCardActivity) error
	UpdateShortLinkID(ctx context.Context, id uint, shortLinkID *uint) error
}

type xiaohongshuCardRepository struct {
	db *gorm.DB
}

// NewXiaohongshuCardRepository 创建小红书卡片仓储
func NewXiaohongshuCardRepository(db *gorm.DB) XiaohongshuCardRepository {
	return &xiaohongshuCardRepository{
		db: db,
	}
}

func (r *xiaohongshuCardRepository) Create(ctx context.Context, card *model.XiaohongshuCard) (*model.XiaohongshuCard, error) {
	if err := r.db.WithContext(ctx).Create(card).Error; err != nil {
		return nil, err
	}
	return card, nil
}

func (r *xiaohongshuCardRepository) Update(ctx context.Context, card *model.XiaohongshuCard) (*model.XiaohongshuCard, error) {
	if card == nil || card.ID == 0 {
		return nil, errors.New("xiaohongshu card: 缺主键，不能整行回写")
	}
	if err := _db.SaveWholeRow(r.db.WithContext(ctx), card); err != nil {
		return nil, err
	}
	return card, nil
}

func (r *xiaohongshuCardRepository) Delete(ctx context.Context, id uint) error {
	return _db.EnsureAffected(r.db.WithContext(ctx).Delete(&model.XiaohongshuCard{}, id))
}

func (r *xiaohongshuCardRepository) GetByID(ctx context.Context, id uint) (*model.XiaohongshuCard, error) {
	var card model.XiaohongshuCard
	if err := r.db.WithContext(ctx).First(&card, id).Error; err != nil {
		return nil, err
	}
	return &card, nil
}

func (r *xiaohongshuCardRepository) GetList(ctx context.Context, req CardListFilter) ([]model.XiaohongshuCard, int64, error) {
	var cards []model.XiaohongshuCard
	var total int64

	// 分页夹取（同 xianyu_card 口径）；分页子句必须只挂在 Find 链上——
	// 先挂 Offset/Limit 再 Count，total 会是本页行数而不是全表数。
	page, pageSize := req.Page, req.PageSize
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
		q := r.db.WithContext(ctx).Model(&model.XiaohongshuCard{})
		if req.Keyword != "" {
			q = q.Where("title LIKE ? OR description LIKE ? OR tags LIKE ?",
				"%"+req.Keyword+"%", "%"+req.Keyword+"%", "%"+req.Keyword+"%")
		}
		if req.IsActive != nil {
			q = q.Where("is_active = ?", *req.IsActive)
		}
		return q
	}

	if err := base().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := base().Order("created_at DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&cards).Error; err != nil {
		return nil, 0, err
	}

	return cards, total, nil
}

func (r *xiaohongshuCardRepository) IncrementViewCount(ctx context.Context, id uint) (*model.XiaohongshuCard, error) {
	var card model.XiaohongshuCard
	if err := r.db.WithContext(ctx).First(&card, id).Error; err != nil {
		return nil, err
	}

	// UpdateColumn 不触发 UpdatedAt 钩子（计数不是内容变更）；
	// 0 行由 First 已排除，无需 EnsureAffected。
	if err := r.db.WithContext(ctx).Model(&card).UpdateColumn("view_count", gorm.Expr("view_count + 1")).Error; err != nil {
		return nil, err
	}
	card.ViewCount++

	return &card, nil
}

func (r *xiaohongshuCardRepository) CreateActivity(ctx context.Context, activity *model.XiaohongshuCardActivity) error {
	return r.db.WithContext(ctx).Create(activity).Error
}

func (r *xiaohongshuCardRepository) UpdateShortLinkID(ctx context.Context, id uint, shortLinkID *uint) error {
	return r.db.WithContext(ctx).Model(&model.XiaohongshuCard{}).Where("id = ?", id).Update("short_link_id", shortLinkID).Error
}
