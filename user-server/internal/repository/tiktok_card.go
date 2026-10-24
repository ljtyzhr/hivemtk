package repository

import (
	"context"
	"errors"
	"time"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// TikTokCardRepository TikTok 卡片仓储接口
type TikTokCardRepository interface {
	Create(ctx context.Context, card *model.TikTokCard) (*model.TikTokCard, error)
	Update(ctx context.Context, card *model.TikTokCard) (*model.TikTokCard, error)
	Delete(ctx context.Context, id uint) error
	GetByID(ctx context.Context, id uint) (*model.TikTokCard, error)
	GetList(ctx context.Context, req CardListFilter) ([]model.TikTokCard, int64, error)
	IncrementViewCount(ctx context.Context, id uint) error
	CreateActivity(ctx context.Context, activity *model.TikTokCardActivity) error
	GetOverallStats(ctx context.Context) (int64, int64, int64, []model.TikTokCard, error)
	GetCardStats(ctx context.Context, id uint, days int) (*model.TikTokCard, []model.TikTokCardActivity, error)
	CountDailyView(ctx context.Context, day string) (int64, error)
	CountCardDailyView(ctx context.Context, cardID uint, day string) (int64, error)
	ListRecentActivities(ctx context.Context, limit int) ([]model.TikTokCardActivity, error)
}

type tiktokCardRepository struct {
	db *gorm.DB
}

// NewTikTokCardRepository 创建 TikTok 卡片仓储实例
func NewTikTokCardRepository(db *gorm.DB) TikTokCardRepository {
	return &tiktokCardRepository{db: db}
}

func (r *tiktokCardRepository) Create(ctx context.Context, card *model.TikTokCard) (*model.TikTokCard, error) {
	if err := r.db.WithContext(ctx).Create(card).Error; err != nil {
		return nil, err
	}
	return card, nil
}

// Update 整行回写一张卡片。
//
// 走 db.SaveWholeRow：裸 Save 命中 0 行会回退成 `INSERT ... ON CONFLICT (id)
// DO UPDATE`（gorm@v1.31.2），把一张「管理端读出来之后、保存之前被删掉的」
// 卡片按原 id 插回去 —— 而它的短码可能已被别的卡片占用。
func (r *tiktokCardRepository) Update(ctx context.Context, card *model.TikTokCard) (*model.TikTokCard, error) {
	if card == nil {
		return nil, errors.New("tiktok card: nil 不是更新对象")
	}
	if err := _db.SaveWholeRow(r.db.WithContext(ctx), card); err != nil {
		return nil, err
	}
	return card, nil
}

// Delete 删除一张卡片。0 行 ⇒ ErrRecordNotFound（删一条已经被删的卡片不该回成功）。
func (r *tiktokCardRepository) Delete(ctx context.Context, id uint) error {
	return _db.EnsureAffected(r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.TikTokCard{}))
}

func (r *tiktokCardRepository) GetByID(ctx context.Context, id uint) (*model.TikTokCard, error) {
	var card model.TikTokCard
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&card).Error; err != nil {
		return nil, err
	}
	return &card, nil
}

func (r *tiktokCardRepository) GetList(ctx context.Context, req CardListFilter) ([]model.TikTokCard, int64, error) {
	var cards []model.TikTokCard
	var total int64

	// 分页夹取：`pageSize = 0` ⇒ GORM 的 Limit(0) 等于**不加 limit**；无上限 ⇒
	// `page_size=100000` 一次把整张卡片表读进内存（controller 侧用的是裸
	// strconv.Atoi，不是 pagination.Parse，所以仓储层必须自己设防）。
	page := 1
	pageSize := 20
	if req.Page > 0 {
		page = req.Page
	}
	if req.PageSize > 0 {
		pageSize = req.PageSize
	}
	if pageSize > 200 {
		pageSize = 200
	}

	// Count 与 Find 各起一条 query：同一条链上先 Count 再 Find 会就地改 Statement
	// （判例 #133 / #150）。
	base := func() *gorm.DB {
		q := r.db.WithContext(ctx).Model(&model.TikTokCard{})
		if req.Keyword != "" {
			like := "%" + req.Keyword + "%"
			q = q.Where("title LIKE ? OR description LIKE ? OR tags LIKE ?", like, like, like)
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

func (r *tiktokCardRepository) IncrementViewCount(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Model(&model.TikTokCard{}).
		Where("id = ?", id).
		UpdateColumn("view_count", gorm.Expr("view_count + 1")).Error
}

func (r *tiktokCardRepository) CreateActivity(ctx context.Context, activity *model.TikTokCardActivity) error {
	return r.db.WithContext(ctx).Create(activity).Error
}

func (r *tiktokCardRepository) GetOverallStats(ctx context.Context) (int64, int64, int64, []model.TikTokCard, error) {
	var totalCards, activeCards, totalViews int64

	if err := r.db.WithContext(ctx).Model(&model.TikTokCard{}).Count(&totalCards).Error; err != nil {
		return 0, 0, 0, nil, err
	}
	if err := r.db.WithContext(ctx).Model(&model.TikTokCard{}).Where("is_active = ?", true).Count(&activeCards).Error; err != nil {
		return 0, 0, 0, nil, err
	}
	if err := r.db.WithContext(ctx).Model(&model.TikTokCard{}).Select("COALESCE(SUM(view_count), 0)").Row().Scan(&totalViews); err != nil {
		return 0, 0, 0, nil, err
	}

	var popular []model.TikTokCard
	if err := r.db.WithContext(ctx).Order("view_count DESC").Limit(5).Find(&popular).Error; err != nil {
		return 0, 0, 0, nil, err
	}

	return totalCards, activeCards, totalViews, popular, nil
}

func (r *tiktokCardRepository) GetCardStats(ctx context.Context, id uint, days int) (*model.TikTokCard, []model.TikTokCardActivity, error) {
	var card model.TikTokCard
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&card).Error; err != nil {
		return nil, nil, err
	}
	var activities []model.TikTokCardActivity
	if days <= 0 {
		days = 7
	}
	since := time.Now().AddDate(0, 0, -days)
	if err := r.db.WithContext(ctx).Where("card_id = ? AND created_at >= ?", id, since).Order("created_at DESC").Limit(50).Find(&activities).Error; err != nil {
		return nil, nil, err
	}
	return &card, activities, nil
}

func (r *tiktokCardRepository) CountDailyView(ctx context.Context, day string) (int64, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&model.TikTokCardActivity{}).
		Where("activity_type = ? AND DATE(created_at) = ?", "view", day).
		Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

func (r *tiktokCardRepository) CountCardDailyView(ctx context.Context, cardID uint, day string) (int64, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&model.TikTokCardActivity{}).
		Where("card_id = ? AND activity_type = ? AND DATE(created_at) = ?", cardID, "view", day).
		Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

func (r *tiktokCardRepository) ListRecentActivities(ctx context.Context, limit int) ([]model.TikTokCardActivity, error) {
	// `limit = 0` ⇒ GORM 的 Limit(0) 等于不加 limit（活动流水只增不减）。
	if limit < 1 {
		limit = 10
	}
	if limit > 200 {
		limit = 200
	}
	var list []model.TikTokCardActivity
	if err := r.db.WithContext(ctx).Order("created_at DESC").Limit(limit).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}
