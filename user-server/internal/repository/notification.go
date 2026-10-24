package repository

import (
	"context"
	"time"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type NotificationRepository struct {
	db *gorm.DB
}

func NewNotificationRepository() *NotificationRepository {
	return &NotificationRepository{db: _db.GetDB()}
}

// NewNotificationRepositoryWithDB 指定数据库连接构造（用于测试 / 服务层注入）
func NewNotificationRepositoryWithDB(db *gorm.DB) *NotificationRepository {
	return &NotificationRepository{db: db}
}

// SetDB 注入 db（用于测试）
func (r *NotificationRepository) SetDB(ctx context.Context, db *gorm.DB) {
	if db != nil {
		r.db = db
	}
}

type NotificationListQuery struct {
	UserID  uint
	Page    int
	Size    int
	Type    string
	Level   string
	IsRead  *bool
	Keyword string
}

type NotificationListResult struct {
	List  []model.Notification
	Total int64
}

// List 通知列表查询（user_id = 0 表示全体，与具体 userID 取并集）
func (r *NotificationRepository) List(ctx context.Context, q NotificationListQuery) (*NotificationListResult, error) {
	if r == nil || r.db == nil {
		return &NotificationListResult{}, nil
	}
	if q.Page < 1 {
		q.Page = 1
	}
	if q.Size < 1 || q.Size > 100 {
		q.Size = 20
	}

	tx := r.db.WithContext(ctx).Model(&model.Notification{}).
		Where("user_id = 0 OR user_id = ?", q.UserID)

	if q.Type != "" {
		tx = tx.Where("type = ?", q.Type)
	}
	if q.Level != "" {
		tx = tx.Where("level = ?", q.Level)
	}
	if q.IsRead != nil {
		tx = tx.Where("is_read = ?", *q.IsRead)
	}
	if q.Keyword != "" {
		like := "%" + q.Keyword + "%"
		tx = tx.Where("title ILIKE ? OR content ILIKE ?", like, like)
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, err
	}

	var list []model.Notification
	if err := tx.Order("created_at DESC").
		Offset((q.Page - 1) * q.Size).
		Limit(q.Size).
		Find(&list).Error; err != nil {
		return nil, err
	}

	return &NotificationListResult{List: list, Total: total}, nil
}

// UpsertFromPlatform 幂等写入一条平台下发的通知。
// 幂等靠 (tenant_id, source_msg_id) 部分唯一索引（WHERE source_msg_id > 0）；
// PG 的 ON CONFLICT 必须带上部分索引谓词才能推断用哪个唯一索引（只写列名会报 42P10）。
// 冲突时什么都不更新：平台消息正文是不可变快照，已展示给用户的行不应随平台改稿而变。
func (r *NotificationRepository) UpsertFromPlatform(ctx context.Context, n *model.Notification) (bool, error) {
	if r == nil || r.db == nil {
		return false, nil
	}
	res := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "tenant_id"},
			{Name: "source_msg_id"},
		},
		TargetWhere: clause.Where{Exprs: []clause.Expression{
			clause.Expr{SQL: "source_msg_id > 0"},
		}},
		DoNothing: true,
	}).Create(n)
	if res.Error != nil {
		return false, res.Error
	}
	// RowsAffected = 0 表示撞上已有行（已投递过），= 1 表示本次新写入。
	return res.RowsAffected > 0, nil
}

// MaxSourceMsgID 取本租户已投递的最大平台消息 ID（= 下一轮同步的游标）。
// 平台消息 ID 单调递增，本地行逐条对应已投递消息，取最大值即为正确游标，不另存以免漂移。
func (r *NotificationRepository) MaxSourceMsgID(ctx context.Context) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	var max *int64
	err := r.db.WithContext(ctx).Model(&model.Notification{}).
		Where("source_msg_id > 0").
		Select("MAX(source_msg_id)").
		Scan(&max).Error
	if err != nil {
		return 0, err
	}
	if max == nil {
		return 0, nil
	}
	return *max, nil
}

// ExistsInTenant 判断某条通知是否存在于当前租户（标记已读前的归属校验，不泄露跨租户存在性）。
func (r *NotificationRepository) ExistsInTenant(ctx context.Context, tenantID, id uint) (bool, error) {
	if r == nil || r.db == nil {
		return false, nil
	}
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Notification{}).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Count(&count).Error
	return count > 0, err
}

// IsPlatformNotice 判断某条通知是否是平台下发的消息。
func (r *NotificationRepository) IsPlatformNotice(ctx context.Context, id uint) (bool, error) {
	if r == nil || r.db == nil {
		return false, nil
	}
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Notification{}).
		Where("id = ? AND type = ?", id, model.NotificationTypePlatformNotice).
		Count(&count).Error
	return count > 0, err
}

// MarkReadByID 标记单条已读，返回受影响行数（用于权限校验）
func (r *NotificationRepository) MarkReadByID(ctx context.Context, userID, id uint) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	now := time.Now()
	res := r.db.WithContext(ctx).Model(&model.Notification{}).
		Where("id = ? AND (user_id = 0 OR user_id = ?)", id, userID).
		Updates(map[string]any{
			"is_read": true,
			"read_at": &now,
		})
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

// MarkAllRead 把该用户的本地业务通知全部标记已读，返回受影响条数。
// 平台消息不在这里处理：其已读由 notification_reads 回执表按人记录（service 层另走 readRepo），
// 若在此置 is_read 会破坏"一条消息 N 个人"的语义（一人点开全场清零）。
func (r *NotificationRepository) MarkAllRead(ctx context.Context, userID uint) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	now := time.Now()
	res := r.db.WithContext(ctx).Model(&model.Notification{}).
		Where("is_read = ? AND (user_id = 0 OR user_id = ?)", false, userID).
		Where("type <> ?", model.NotificationTypePlatformNotice).
		Updates(map[string]any{
			"is_read": true,
			"read_at": &now,
		})
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

// CountUnread 统计未读数（仅本地业务通知）。
// 平台消息的未读由 notification_reads 回执表按人记录，service 层另走回执口径；
// 两个口径都算会未读数翻倍。
func (r *NotificationRepository) CountUnread(ctx context.Context, userID uint) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Notification{}).
		Where("is_read = ? AND (user_id = 0 OR user_id = ?)", false, userID).
		Where("type <> ?", model.NotificationTypePlatformNotice).
		Count(&count).Error
	return count, err
}

func (r *NotificationRepository) Create(ctx context.Context, n *model.Notification) error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.WithContext(ctx).Create(n).Error
}

// CountAll 统计通知总数（用于 SeedIfEmpty 判定）
func (r *NotificationRepository) CountAll(ctx context.Context) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.Notification{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// CountByTypeLinkSince 统计 type=ntype 且 link=link 且 created_at >= since 的通知条数，
// 用于提醒去重（节拍器每轮都扫到同一批逾期行，不去重会反复刷通知）。
func (r *NotificationRepository) CountByTypeLinkSince(ctx context.Context, ntype, link string, since time.Time) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Notification{}).
		Where("type = ? AND link = ? AND created_at >= ?", ntype, link, since).
		Count(&count).Error
	return count, err
}
