package migrations

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"
	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// InAppNotificationMigration v3.75.0：站内消息通知模块的表结构。
//
// 建两张表 + 给 notifications 加三列：
//   - notification_reads（新表）：平台消息的**按人**已读回执。
//     为什么必须有：notifications.is_read 是消息行上的单一布尔位，平台消息
//     一条对应 N 个用户，A 点已读会把 B 看到的同一条也变成已读 ⇒ 未读角标
//     被一个人清零。
//   - notifications.level / source_msg_id / source（加列）：
//     level 是消息级别（平台消息取平台原值）；source_msg_id 是来源平台消息
//     ID，兼作幂等键；source 区分平台下发与本地业务通知。
//
// 幂等键 = (tenant_id, source_msg_id) 的**部分唯一**索引
// （WHERE source_msg_id > 0）：普通唯一索引会让第二条本地业务通知撞键
// ——本地通知的 source_msg_id 恒为 0。
//
// 幂等可重入。
type InAppNotificationMigration struct {
	db *gorm.DB
}

var _ migration.Migration = (*InAppNotificationMigration)(nil)

// NewInAppNotificationMigration 创建迁移实例。
func NewInAppNotificationMigration(db *gorm.DB) *InAppNotificationMigration {
	return &InAppNotificationMigration{db: db}
}

func (m *InAppNotificationMigration) Version() string { return "v3.75.0" }

func (m *InAppNotificationMigration) Name() string {
	return "站内消息通知模块：回执表 + 通知扩列"
}

func (m *InAppNotificationMigration) Description() string {
	return "建 notification_reads 回执表（平台消息按人已读），notifications 加 level/source_msg_id/source 列与幂等部分唯一索引；幂等可重入"
}

func (m *InAppNotificationMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	db := m.db.WithContext(ctx)

	if err := db.AutoMigrate(&model.NotificationRead{}); err != nil {
		return fmt.Errorf("建 notification_reads 表失败: %w", err)
	}

	// notifications 的三列：AutoMigrate 对已有表做加列是安全的（ADD COLUMN IF NOT EXISTS
	// 语义），但既有行需要回填默认值，否则老行的 source 为空串、level 为空串，
	// 读面按 source 区分两类消息时会把它们漏掉。
	if err := db.AutoMigrate(&model.Notification{}); err != nil {
		return fmt.Errorf("扩列 notifications 失败: %w", err)
	}

	if err := db.Model(&model.Notification{}).
		Where("source = '' OR source IS NULL").
		Update("source", model.NotificationSourceLocal).Error; err != nil {
		return fmt.Errorf("回填 notifications.source 失败: %w", err)
	}
	if err := db.Model(&model.Notification{}).
		Where("level = '' OR level IS NULL").
		Update("level", model.NotificationLevelInfo).Error; err != nil {
		return fmt.Errorf("回填 notifications.level 失败: %w", err)
	}

	return nil
}

// Down 不回滚：回执表与三列都是本功能的正确最终形状，
// 回滚会让已投递的平台消息失去按人已读能力。
func (m *InAppNotificationMigration) Down(ctx context.Context) error {
	return nil
}
