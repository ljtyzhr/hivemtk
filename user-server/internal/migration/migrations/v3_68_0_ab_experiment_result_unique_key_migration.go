package migrations

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// ABExperimentResultUniqueKeyMigration v3.68.0：A/B 实验结果唯一键恢复为
// (tenant_id, experiment_id, variant_id)。
//
// 模型曾把 ExperimentID 标成单列 uniqueIndex，AutoMigrate 在多租户库上落成
// (tenant_id, experiment_id) 唯一索引。实验结果的粒度是「每个变体一行」
// （CalculateResults 逐变体 Upsert），单列唯一会让第二个变体撞 23505，
// 多变异实验的结果计算整体失败。008_ab_test.sql 的原始设计就是
// (experiment_id, variant_id)，这里按租户化口径把它补回来。
type ABExperimentResultUniqueKeyMigration struct {
	db *gorm.DB
}

var _ migration.Migration = (*ABExperimentResultUniqueKeyMigration)(nil)

// NewABExperimentResultUniqueKeyMigration 创建迁移实例
func NewABExperimentResultUniqueKeyMigration(db *gorm.DB) *ABExperimentResultUniqueKeyMigration {
	return &ABExperimentResultUniqueKeyMigration{db: db}
}

// Version 返回版本号
func (m *ABExperimentResultUniqueKeyMigration) Version() string { return "v3.68.0" }

// Name 返回迁移名称
func (m *ABExperimentResultUniqueKeyMigration) Name() string {
	return "A/B 实验结果唯一键恢复为 (tenant_id, experiment_id, variant_id)"
}

// Description 返回迁移描述
func (m *ABExperimentResultUniqueKeyMigration) Description() string {
	return "删除误建的 (tenant_id, experiment_id) 唯一索引，改回每变体一行的复合唯一键；幂等可重入"
}

func (m *ABExperimentResultUniqueKeyMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	db := m.db.WithContext(ctx)
	if !db.Migrator().HasTable("ab_experiment_results") {
		return nil
	}

	return db.Transaction(func(tx *gorm.DB) error {
		// 旧模型标签建出的单列唯一索引；不删它，多变异实验永远只能落第一条结果。
		if err := tx.Exec(`DROP INDEX IF EXISTS idx_ab_experiment_results_experiment_id`).Error; err != nil {
			return fmt.Errorf("删除单列唯一索引失败: %w", err)
		}
		// 防御性去重：旧索引已保证每实验至多一行，正常不会命中；保留最新一行。
		if err := tx.Exec(`
DELETE FROM ab_experiment_results a USING ab_experiment_results b
WHERE a.tenant_id = b.tenant_id
  AND a.experiment_id = b.experiment_id
  AND a.variant_id = b.variant_id
  AND a.id < b.id`).Error; err != nil {
			return fmt.Errorf("ab_experiment_results 重复行清理失败: %w", err)
		}
		if err := tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_ab_experiment_results_exp_variant
ON ab_experiment_results (tenant_id, experiment_id, variant_id)`).Error; err != nil {
			return fmt.Errorf("创建复合唯一索引失败: %w", err)
		}
		return nil
	})
}

// Down 回滚：唯一键收窄会丢多变异结果，不做自动回滚。
func (m *ABExperimentResultUniqueKeyMigration) Down(ctx context.Context) error {
	return nil
}
