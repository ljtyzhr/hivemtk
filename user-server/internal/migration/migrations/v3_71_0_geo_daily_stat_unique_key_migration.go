package migrations

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// GeoDailyStatUniqueKeyMigration v3.71.0：为 geo_daily_stats 建立
// (tenant_id, stat_date, engine, intent) 唯一索引。
//
// 事实链：
//  1. 模型原标签写成 `index:idx_date_engine_intent,uniqueIndex`，GORM 把它解析成
//     「一个同名非唯一复合索引 + 一个单独的 uniqueIndex 设置」，实际建出来的是
//     **非唯一**索引（按仓内迁移重建的库验证：idx_date_engine_intent 无 UNIQUE）。
//  2. 开发库里存在手写的 `UNIQUE (tenant_id, stat_date, engine, intent)`，
//     但仓内没有任何迁移建它 ⇒ 全新部署与开发库漂移。
//  3. 后果：`Upsert` / `IncrementByEngine` 的 `ON CONFLICT (...)` 在全新库上报
//     `42P10 there is no unique or exclusion constraint matching`，每日聚合整条写不进去。
//
// 本迁移先按唯一键去重（把重复行的计数累加进 id 最大的那行，其余删除），再建唯一索引；
// 幂等可重入。模型标签已同步改为 `uniqueIndex:idx_date_engine_intent`。
type GeoDailyStatUniqueKeyMigration struct {
	db *gorm.DB
}

var _ migration.Migration = (*GeoDailyStatUniqueKeyMigration)(nil)

func NewGeoDailyStatUniqueKeyMigration(db *gorm.DB) *GeoDailyStatUniqueKeyMigration {
	return &GeoDailyStatUniqueKeyMigration{db: db}
}

func (m *GeoDailyStatUniqueKeyMigration) Version() string { return "v3.71.0" }

func (m *GeoDailyStatUniqueKeyMigration) Name() string {
	return "geo_daily_stats 唯一键租户化"
}

func (m *GeoDailyStatUniqueKeyMigration) Description() string {
	return "去重后建立 (tenant_id, stat_date, engine, intent) 唯一索引，修复 upsert 的 ON CONFLICT 无约束可匹配；幂等可重入"
}

func (m *GeoDailyStatUniqueKeyMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	db := m.db.WithContext(ctx)
	if !db.Migrator().HasTable("geo_daily_stats") {
		return nil
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`DROP INDEX IF EXISTS idx_date_engine_intent`).Error; err != nil {
			return fmt.Errorf("删除旧索引失败: %w", err)
		}

		// 重复行合并：计数相加进 id 最大的那行；funnel_stage 保留该行的值。
		if err := tx.Exec(`
			UPDATE geo_daily_stats a
			SET citation_count = s.citation_count,
			    brand_mentioned_count = s.brand_mentioned_count,
			    competitor_mentioned_count = s.competitor_mentioned_count,
			    negative_count = s.negative_count,
			    probe_count = s.probe_count
			FROM (
				SELECT tenant_id, stat_date, engine, intent, MAX(id) AS keep_id,
				       SUM(citation_count) AS citation_count,
				       SUM(brand_mentioned_count) AS brand_mentioned_count,
				       SUM(competitor_mentioned_count) AS competitor_mentioned_count,
				       SUM(negative_count) AS negative_count,
				       SUM(probe_count) AS probe_count
				FROM geo_daily_stats
				GROUP BY tenant_id, stat_date, engine, intent
				HAVING COUNT(*) > 1
			) s
			WHERE a.id = s.keep_id`).Error; err != nil {
			return fmt.Errorf("合并重复聚合行失败: %w", err)
		}
		if err := tx.Exec(`
			DELETE FROM geo_daily_stats a
			USING geo_daily_stats b
			WHERE a.id < b.id
			  AND a.tenant_id = b.tenant_id
			  AND a.stat_date = b.stat_date
			  AND a.engine = b.engine
			  AND a.intent = b.intent`).Error; err != nil {
			return fmt.Errorf("删除重复聚合行失败: %w", err)
		}

		if err := tx.Exec(`
			CREATE UNIQUE INDEX IF NOT EXISTS idx_date_engine_intent
			ON geo_daily_stats (tenant_id, stat_date, engine, intent)`).Error; err != nil {
			return fmt.Errorf("创建唯一索引失败: %w", err)
		}
		return nil
	})
}

// Down 不回滚：唯一键是聚合表的正确形状，回到非唯一索引会重新打开并发重复写面。
func (m *GeoDailyStatUniqueKeyMigration) Down(ctx context.Context) error {
	return nil
}
