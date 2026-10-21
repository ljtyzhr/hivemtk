package migrations

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// GeoEntityRelationUniqueKeyMigration v3.72.0：为 geo_entity_relations 建立
// (tenant_id, entity_a_id, entity_b_id, relation) 唯一索引，并删除
// geo_entities.source_doc_id 这个从未被赋值的僵尸列。
//
// 事实链：
//  1. `UpsertRelation` 原实现是 `FirstOrCreate`（SELECT 后 INSERT），并发抽取
//     同一文档时两次 SELECT 都可能未命中，于是同一关系写入两行，关系图出现重复边。
//     仓储已改为 ON CONFLICT DO NOTHING，需要与列集完全一致的唯一索引。
//  2. 存量库可能已有重复关系行，而 AutoMigrate 不做去重、且先于迁移执行；
//     因此唯一索引不在模型标签里声明，只由本迁移在去重后创建。
//  3. `source_doc_id` 全仓唯一赋值是常量 0（知识库文档主键是 varchar(36)，
//     与 uint 列类型不可能对上），且没有任何读取方 —— 僵尸列，一并删除。
//
// 幂等可重入。
type GeoEntityRelationUniqueKeyMigration struct {
	db *gorm.DB
}

var _ migration.Migration = (*GeoEntityRelationUniqueKeyMigration)(nil)

func NewGeoEntityRelationUniqueKeyMigration(db *gorm.DB) *GeoEntityRelationUniqueKeyMigration {
	return &GeoEntityRelationUniqueKeyMigration{db: db}
}

func (m *GeoEntityRelationUniqueKeyMigration) Version() string { return "v3.72.0" }

func (m *GeoEntityRelationUniqueKeyMigration) Name() string {
	return "geo_entity_relations 唯一键租户化并清理僵尸列"
}

func (m *GeoEntityRelationUniqueKeyMigration) Description() string {
	return "去重后建立 (tenant_id, entity_a_id, entity_b_id, relation) 唯一索引，保证关系幂等写入；删除 geo_entities.source_doc_id 僵尸列"
}

func (m *GeoEntityRelationUniqueKeyMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	db := m.db.WithContext(ctx)

	if db.Migrator().HasTable("geo_entity_relations") {
		if err := db.Transaction(func(tx *gorm.DB) error {
			// 同一关系只保留 id 最小的一行。
			if err := tx.Exec(`
				DELETE FROM geo_entity_relations a
				USING geo_entity_relations b
				WHERE a.id > b.id
				  AND a.tenant_id = b.tenant_id
				  AND a.entity_a_id = b.entity_a_id
				  AND a.entity_b_id = b.entity_b_id
				  AND a.relation = b.relation`).Error; err != nil {
				return fmt.Errorf("删除重复实体关系失败: %w", err)
			}
			if err := tx.Exec(`
				CREATE UNIQUE INDEX IF NOT EXISTS idx_entity_rel
				ON geo_entity_relations (tenant_id, entity_a_id, entity_b_id, relation)`).Error; err != nil {
				return fmt.Errorf("创建实体关系唯一索引失败: %w", err)
			}
			return nil
		}); err != nil {
			return err
		}
	}

	if db.Migrator().HasTable("geo_entities") && db.Migrator().HasColumn("geo_entities", "source_doc_id") {
		if err := db.Exec(`ALTER TABLE geo_entities DROP COLUMN source_doc_id`).Error; err != nil {
			return fmt.Errorf("删除 geo_entities.source_doc_id 失败: %w", err)
		}
	}
	return nil
}

// Down 不回滚：唯一索引与僵尸列清理都是正确的最终形状。
func (m *GeoEntityRelationUniqueKeyMigration) Down(ctx context.Context) error {
	return nil
}
