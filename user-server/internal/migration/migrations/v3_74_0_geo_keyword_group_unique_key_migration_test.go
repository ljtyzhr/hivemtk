package migrations

// v3.74.0 钉住行为：重复分组保留「活跃优先、updated_at 最新」的一行、
// 唯一索引按租户隔离、迁移可重入。

import (
	"context"
	"testing"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

func gkgKeyFixture(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.NewTestDB(t)
	if db == nil {
		t.Fatal("测试库不可达：唯一键是否修好无法判定（不 Skip，跳过等于没锁）")
	}
	stmts := []string{
		`DROP TABLE IF EXISTS geo_keyword_groups`,
		`CREATE TABLE geo_keyword_groups (
			id varchar(36) PRIMARY KEY,
			tenant_id bigint NOT NULL DEFAULT 1,
			name varchar(200) NOT NULL,
			description text,
			keyword_count integer DEFAULT 0,
			keyword_list text,
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
	t.Cleanup(func() { _ = db.Exec(`DROP TABLE IF EXISTS geo_keyword_groups`).Error })
	return db
}

func TestGeoKeywordGroupUniqueKeyMigration(t *testing.T) {
	db := gkgKeyFixture(t)
	// 租户 A：一条已删除的旧行 + 一条活跃新行；租户 B：独立同名分组。
	stmts := []string{
		`INSERT INTO geo_keyword_groups (id, tenant_id, name, keyword_count, updated_at, deleted_at)
		 VALUES ('old', 341, '对比类', 1, now() - interval '1 day', now())`,
		`INSERT INTO geo_keyword_groups (id, tenant_id, name, keyword_count, updated_at)
		 VALUES ('new', 341, '对比类', 3, now())`,
		`INSERT INTO geo_keyword_groups (id, tenant_id, name, keyword_count, updated_at)
		 VALUES ('b1', 342, '对比类', 2, now())`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("预置分组失败: %v\nSQL: %s", err, s)
		}
	}

	m := NewGeoKeywordGroupUniqueKeyMigration(db)
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	var kept string
	if err := db.Raw(`SELECT id FROM geo_keyword_groups WHERE tenant_id = 341`).Scan(&kept).Error; err != nil {
		t.Fatalf("回查失败: %v", err)
	}
	if kept != "new" {
		t.Fatalf("应保留活跃且最新的分组，实际保留 %q", kept)
	}

	if err := db.Exec(`
		INSERT INTO geo_keyword_groups (id, tenant_id, name)
		VALUES ('dup', 341, '对比类')`).Error; err == nil {
		t.Fatal("同租户同名分组仍能写入：唯一索引未生效")
	}
	if err := db.Exec(`
		INSERT INTO geo_keyword_groups (id, tenant_id, name)
		VALUES ('b2', 343, '对比类')`).Error; err != nil {
		t.Fatalf("跨租户同名分组被误拒: %v", err)
	}

	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("迁移重入失败: %v", err)
	}
}
