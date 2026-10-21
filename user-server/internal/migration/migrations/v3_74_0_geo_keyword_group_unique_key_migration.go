package migrations

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// GeoKeywordGroupUniqueKeyMigration v3.74.0：为 geo_keyword_groups 建立
// (tenant_id, name) 唯一索引。
//
// 事实链：`UpsertByName` 原为「先 SELECT，命中则 Save，否则 Create」——
//   - 并发聚类时两个请求都可能 SELECT 未命中，各建一个同名分组；
//   - Save 命中软删除行时会把它复活，且无法区分「不存在」与「已删除」。
//
// 仓储已改为 ON CONFLICT (tenant_id, name) DO UPDATE。存量的重复分组会与唯一
// 索引冲突，而 AutoMigrate 先于迁移执行且不做去重，所以索引只在本迁移里创建：
// 每个 (tenant_id, name) 只保留「活跃优先、updated_at 最新、id 最大」的一行。
//
// 幂等可重入。
type GeoKeywordGroupUniqueKeyMigration struct {
	db *gorm.DB
}

var _ migration.Migration = (*GeoKeywordGroupUniqueKeyMigration)(nil)

func NewGeoKeywordGroupUniqueKeyMigration(db *gorm.DB) *GeoKeywordGroupUniqueKeyMigration {
	return &GeoKeywordGroupUniqueKeyMigration{db: db}
}

func (m *GeoKeywordGroupUniqueKeyMigration) Version() string { return "v3.74.0" }

func (m *GeoKeywordGroupUniqueKeyMigration) Name() string {
	return "geo_keyword_groups 唯一键租户化"
}

func (m *GeoKeywordGroupUniqueKeyMigration) Description() string {
	return "去重后建立 (tenant_id, name) 唯一索引，保证话题聚类分组幂等写入；幂等可重入"
}

func (m *GeoKeywordGroupUniqueKeyMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	db := m.db.WithContext(ctx)
	if !db.Migrator().HasTable("geo_keyword_groups") {
		return nil
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`
			DELETE FROM geo_keyword_groups a
			USING (
				SELECT id FROM (
					SELECT id,
					       ROW_NUMBER() OVER (
					           PARTITION BY tenant_id, name
					           ORDER BY (deleted_at IS NULL) DESC, updated_at DESC NULLS LAST, id DESC
					       ) AS rn
					FROM geo_keyword_groups
				) ranked
				WHERE ranked.rn > 1
			) dup
			WHERE a.id = dup.id`).Error; err != nil {
			return fmt.Errorf("删除重复关键词分组失败: %w", err)
		}

		if err := tx.Exec(`
			CREATE UNIQUE INDEX IF NOT EXISTS idx_keyword_group_tenant_name
			ON geo_keyword_groups (tenant_id, name)`).Error; err != nil {
			return fmt.Errorf("创建关键词分组唯一索引失败: %w", err)
		}
		return nil
	})
}

// Down 不回滚：唯一索引是关键词分组的正确最终形状。
func (m *GeoKeywordGroupUniqueKeyMigration) Down(ctx context.Context) error {
	return nil
}
