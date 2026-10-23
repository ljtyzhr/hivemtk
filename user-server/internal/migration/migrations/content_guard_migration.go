// content_guard_migration.go 内容安全闸（ContentGuard）五张表的迁移。
//
// 版本号 v3.78.0：接在现网最新 v3.77.0 之后。**不复用旧版本号**——迁移注册表按
// 版本去重，复用一个已 completed 的版本会让本迁移被判定为"已执行"而静默跳过
// （user_blacklist_migration.go 顶部记过同一个坑）。
package migrations

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"hivemtk-user/internal/contentguard/model"
	"hivemtk-user/internal/migration"
)

// ContentGuardMigration 创建 guard_* 五张表。
type ContentGuardMigration struct {
	db *gorm.DB
}

// NewContentGuardMigration 构造迁移。
func NewContentGuardMigration(db *gorm.DB) *ContentGuardMigration {
	return &ContentGuardMigration{db: db}
}

// Version 版本号。
func (m *ContentGuardMigration) Version() string { return "v3.78.0" }

// Name 迁移名。
func (m *ContentGuardMigration) Name() string { return "创建内容安全闸 guard_* 表" }

// Description 描述。
func (m *ContentGuardMigration) Description() string {
	return "创建 guard_words / guard_policies / guard_tasks / guard_records / guard_sanctions，" +
		"支撑离线词库审查、按主体类型的策略、异步审查任务、审查结论留痕与冻结/黑名单处置"
}

// Up 建表（AutoMigrate 幂等；唯一索引由 tenancyidx 复合化为租户维度）。
func (m *ContentGuardMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	if err := m.db.WithContext(ctx).AutoMigrate(
		&model.GuardWord{},
		&model.GuardPolicy{},
		&model.GuardTask{},
		&model.GuardRecord{},
		&model.GuardSanction{},
	); err != nil {
		return fmt.Errorf("guard_* 建表失败: %w", err)
	}
	return nil
}

// Down 回滚（本仓策略：不自动 DROP 生产表，见 down_drop_policy.go）。
func (m *ContentGuardMigration) Down(ctx context.Context) error {
	declineTableDrop(m.Version(), "guard_words / guard_policies / guard_tasks / guard_records / guard_sanctions")
	return nil
}

var _ migration.Migration = (*ContentGuardMigration)(nil)
