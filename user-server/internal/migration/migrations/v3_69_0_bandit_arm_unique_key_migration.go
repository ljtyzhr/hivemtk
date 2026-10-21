package migrations

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// BanditArmUniqueKeyMigration v3.69.0：bandit_arms 唯一键从
// (tenant_id, arm_key) 恢复为 (tenant_id, experiment_id, arm_key)。
//
// 模型曾只给 ArmKey 标 uniqueIndex，AutoMigrate 落成「全库同租户 arm_key 唯一」。
// 而 arm_key 只在实验内唯一：prompt 迭代固定用 arm_0_original，SOP 克隆固定用
// arm_a_original/arm_b_variant。第二个实验一创建臂就撞 23505，多实验并行直接失效。
type BanditArmUniqueKeyMigration struct {
	db *gorm.DB
}

var _ migration.Migration = (*BanditArmUniqueKeyMigration)(nil)

// NewBanditArmUniqueKeyMigration 创建迁移实例
func NewBanditArmUniqueKeyMigration(db *gorm.DB) *BanditArmUniqueKeyMigration {
	return &BanditArmUniqueKeyMigration{db: db}
}

// Version 返回版本号
func (m *BanditArmUniqueKeyMigration) Version() string { return "v3.69.0" }

// Name 返回迁移名称
func (m *BanditArmUniqueKeyMigration) Name() string {
	return "bandit_arms 唯一键从 (tenant_id, arm_key) 恢复为 (tenant_id, experiment_id, arm_key)"
}

// Description 返回迁移描述
func (m *BanditArmUniqueKeyMigration) Description() string {
	return "删除按 arm_key 全局唯一的索引，改建实验内唯一键；幂等可重入"
}

func (m *BanditArmUniqueKeyMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	db := m.db.WithContext(ctx)
	if !db.Migrator().HasTable("bandit_arms") {
		return nil
	}

	return db.Transaction(func(tx *gorm.DB) error {
		// 旧模型标签建出的 (tenant_id, arm_key) 唯一索引；不删它，
		// 第二个实验复用 arm_0_original 就永远写不进来。
		if err := tx.Exec(`DROP INDEX IF EXISTS idx_bandit_exp_arm`).Error; err != nil {
			return fmt.Errorf("删除 arm_key 单列唯一索引失败: %w", err)
		}
		// 防御性去重：旧索引保证 (tenant_id, arm_key) 唯一，实验内本不该有重复；
		// 保留每个 (tenant_id, experiment_id, arm_key) 的最新一行。
		if err := tx.Exec(`
DELETE FROM bandit_arms a USING bandit_arms b
WHERE a.tenant_id = b.tenant_id
  AND a.experiment_id = b.experiment_id
  AND a.arm_key = b.arm_key
  AND a.id < b.id`).Error; err != nil {
			return fmt.Errorf("bandit_arms 重复行清理失败: %w", err)
		}
		if err := tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_bandit_exp_arm
ON bandit_arms (tenant_id, experiment_id, arm_key)`).Error; err != nil {
			return fmt.Errorf("创建实验内唯一索引失败: %w", err)
		}
		return nil
	})
}

// Down 回滚：唯一键收窄会因跨实验重复 arm_key 而失败或删数据，不做自动回滚。
func (m *BanditArmUniqueKeyMigration) Down(ctx context.Context) error {
	return nil
}
