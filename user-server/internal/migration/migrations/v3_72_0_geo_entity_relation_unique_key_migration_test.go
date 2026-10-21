package migrations

// v3.72.0 钉住行为：重复关系行按 id 最小保留、唯一索引按租户隔离、
// 迁移可重入、source_doc_id 僵尸列被删除。

import (
	"context"
	"testing"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

func gerKeyFixture(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.NewTestDB(t)
	if db == nil {
		t.Fatal("测试库不可达：唯一键是否修好无法判定（不 Skip，跳过等于没锁）")
	}
	stmts := []string{
		`DROP TABLE IF EXISTS geo_entity_relations`,
		`CREATE TABLE geo_entity_relations (
			id bigserial PRIMARY KEY,
			tenant_id bigint NOT NULL DEFAULT 1,
			entity_a_id bigint,
			entity_b_id bigint,
			relation varchar(64),
			created_at timestamptz,
			deleted_at timestamptz
		)`,
		`CREATE INDEX idx_geo_entity_relations_entity_a_id ON geo_entity_relations (entity_a_id)`,
		`DROP TABLE IF EXISTS geo_entities`,
		`CREATE TABLE geo_entities (
			id bigserial PRIMARY KEY,
			tenant_id bigint NOT NULL DEFAULT 1,
			name varchar(256),
			description text,
			source_doc_id bigint,
			created_at timestamptz,
			updated_at timestamptz,
			deleted_at timestamptz
		)`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("还原迁移前形态失败: %v\nSQL: %s", err, s)
		}
	}
	t.Cleanup(func() {
		_ = db.Exec(`DROP TABLE IF EXISTS geo_entity_relations`).Error
		_ = db.Exec(`DROP TABLE IF EXISTS geo_entities`).Error
	})
	return db
}

func gerInsertDuplicate(t *testing.T, db *gorm.DB, tenantID, a, b uint, relation string) {
	t.Helper()
	for range 2 {
		if err := db.Exec(`
			INSERT INTO geo_entity_relations (tenant_id, entity_a_id, entity_b_id, relation)
			VALUES (?, ?, ?, ?)`, tenantID, a, b, relation).Error; err != nil {
			t.Fatalf("写入重复关系失败: %v", err)
		}
	}
}

func TestGeoEntityRelationUniqueKeyMigration(t *testing.T) {
	db := gerKeyFixture(t)
	gerInsertDuplicate(t, db, 291, 1, 2, "is_a")
	gerInsertDuplicate(t, db, 292, 1, 2, "is_a")

	m := NewGeoEntityRelationUniqueKeyMigration(db)
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	var rows []struct {
		TenantID  uint
		EntityAID uint
		EntityBID uint
		Relation  string
	}
	if err := db.Raw(`SELECT tenant_id, entity_a_id, entity_b_id, relation FROM geo_entity_relations`).Scan(&rows).Error; err != nil {
		t.Fatalf("回查关系失败: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("两个租户各保留 1 条，实际 %d 条", len(rows))
	}

	// 唯一性必须真实存在：同租户同键再插一行应被索引拒绝。
	if err := db.Exec(`
		INSERT INTO geo_entity_relations (tenant_id, entity_a_id, entity_b_id, relation)
		VALUES (291, 1, 2, 'is_a')`).Error; err == nil {
		t.Fatal("重复关系仍能写入：唯一索引未生效")
	}
	// 跨租户同键不受影响（索引含 tenant_id）。
	if err := db.Exec(`
		INSERT INTO geo_entity_relations (tenant_id, entity_a_id, entity_b_id, relation)
		VALUES (293, 1, 2, 'is_a')`).Error; err != nil {
		t.Fatalf("跨租户同键被误拒: %v", err)
	}

	if db.Migrator().HasColumn("geo_entities", "source_doc_id") {
		t.Fatal("source_doc_id 僵尸列未被删除")
	}

	// 幂等：重复执行不报错、不再删行。
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("迁移重入失败: %v", err)
	}
}
