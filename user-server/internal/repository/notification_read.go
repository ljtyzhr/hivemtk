package repository

import (
	"context"
	"time"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// NotificationReadRepository 通知已读回执仓库。
type NotificationReadRepository struct {
	db *gorm.DB
}

// NewNotificationReadRepository 创建回执仓库（使用全局 db）。
func NewNotificationReadRepository() *NotificationReadRepository {
	return &NotificationReadRepository{db: _db.GetDB()}
}

// NewNotificationReadRepositoryWithDB 创建指定连接的回执仓库（测试注入）。
func NewNotificationReadRepositoryWithDB(db *gorm.DB) *NotificationReadRepository {
	return &NotificationReadRepository{db: db}
}

// SetDB 注入 db（用于测试）。
func (r *NotificationReadRepository) SetDB(ctx context.Context, db *gorm.DB) {
	if db != nil {
		r.db = db
	}
}

// MarkRead 写一条已读回执（幂等）。
//
// 幂等键 (tenant_id, notification_id, user_id) 命中已有行时只更新 read_at，
// 不产生重复行 —— 重复点"标记已读"是正常操作，不是错误。
func (r *NotificationReadRepository) MarkRead(ctx context.Context, tenantID, notificationID, userID uint) error {
	if r == nil || r.db == nil {
		return nil
	}
	row := &model.NotificationRead{
		TenantID:       tenantID,
		NotificationID: notificationID,
		UserID:         userID,
		ReadAt:         time.Now(),
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "tenant_id"},
			{Name: "notification_id"},
			{Name: "user_id"},
		},
		DoUpdates: clause.AssignmentColumns([]string{"read_at"}),
	}).Create(row).Error
}

// MarkAllRead 把该租户下、该用户的所有平台消息标记为已读，返回写入条数。
func (r *NotificationReadRepository) MarkAllRead(ctx context.Context, tenantID, userID uint) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	// 先取该用户还没读过的平台消息 id，再逐条 upsert。
	// 不直接 INSERT..SELECT ON CONFLICT：那样拿不到"本次新读了几条"的计数，
	// 而这个计数要回给前端当"全部标记已读"的反馈。
	var ids []uint
	err := r.db.WithContext(ctx).Model(&model.Notification{}).
		Where("tenant_id = ? AND type = ?", tenantID, model.NotificationTypePlatformNotice).
		Where("id NOT IN (?)",
			r.db.WithContext(ctx).Model(&model.NotificationRead{}).
				Select("notification_id").
				Where("tenant_id = ? AND user_id = ?", tenantID, userID)).
		Pluck("id", &ids).Error
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}

	rows := make([]model.NotificationRead, 0, len(ids))
	now := time.Now()
	for _, id := range ids {
		rows = append(rows, model.NotificationRead{
			TenantID:       tenantID,
			NotificationID: id,
			UserID:         userID,
			ReadAt:         now,
		})
	}
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "tenant_id"},
			{Name: "notification_id"},
			{Name: "user_id"},
		},
		DoUpdates: clause.AssignmentColumns([]string{"read_at"}),
	}).CreateInBatches(&rows, 200).Error; err != nil {
		return 0, err
	}
	return int64(len(ids)), nil
}

// ReadNotificationIDs 取该用户已读过的平台消息 id 集合（供列表 join 用）。
func (r *NotificationReadRepository) ReadNotificationIDs(ctx context.Context, tenantID, userID uint) (map[uint]bool, error) {
	out := make(map[uint]bool)
	if r == nil || r.db == nil {
		return out, nil
	}
	var ids []uint
	err := r.db.WithContext(ctx).Model(&model.NotificationRead{}).
		Where("tenant_id = ? AND user_id = ?", tenantID, userID).
		Pluck("notification_id", &ids).Error
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}

// CountUnread 统计该用户未读过的平台消息条数。
func (r *NotificationReadRepository) CountUnread(ctx context.Context, tenantID, userID uint) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Notification{}).
		Where("tenant_id = ? AND type = ?", tenantID, model.NotificationTypePlatformNotice).
		Where("id NOT IN (?)",
			r.db.WithContext(ctx).Model(&model.NotificationRead{}).
				Select("notification_id").
				Where("tenant_id = ? AND user_id = ?", tenantID, userID)).
		Count(&count).Error
	return count, err
}
