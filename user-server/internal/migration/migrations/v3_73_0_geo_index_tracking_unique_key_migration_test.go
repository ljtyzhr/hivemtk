package migrations

// v3.73.0 钉住行为：重复收录追踪按 id 最大保留、唯一索引按租户隔离、迁移可重入。

import (
	"context"
	"testing"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

func gitKeyFixture(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.NewTestDB(t)
	if db == nil {
		t.Fatal("测试库不可达：唯一键是否修好无法判定（不 Skip，跳过等于没锁）")
	}
	stmts := []string{
		`DROP TABLE IF EXISTS geo_index_trackings`,
		`CREATE TABLE geo_index_trackings (
			id bigserial PRIMARY KEY,
			tenant_id bigint NOT NULL DEFAULT 1,
			article_id varchar(36),
			engine varchar(30),
			url varchar(512),
			keyword text,
			indexed boolean DEFAULT false,
			rank_position integer DEFAULT 0,
			ai_cited boolean DEFAULT false,
			ai_cite_count integer DEFAULT 0,
			last_checked timestamptz,
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
	t.Cleanup(func() { _ = db.Exec(`DROP TABLE IF EXISTS geo_index_trackings`).Error })
	return db
}

func TestGeoIndexTrackingUniqueKeyMigration(t *testing.T) {
	db := gitKeyFixture(t)
	for range 2 {
		if err := db.Exec(`
			INSERT INTO geo_index_trackings (tenant_id, article_id, engine, ai_cited)
			VALUES (311, 'a-1', 'kimi', false)`).Error; err != nil {
			t.Fatalf("写入重复追踪失败: %v", err)
		}
	}
	if err := db.Exec(`
		INSERT INTO geo_index_trackings (tenant_id, article_id, engine, ai_cited)
		VALUES (312, 'a-1', 'kimi', false)`).Error; err != nil {
		t.Fatalf("写入另一租户追踪失败: %v", err)
	}

	m := NewGeoIndexTrackingUniqueKeyMigration(db)
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	var n int64
	if err := db.Raw(`SELECT COUNT(*) FROM geo_index_trackings`).Scan(&n).Error; err != nil {
		t.Fatalf("回查失败: %v", err)
	}
	if n != 2 {
		t.Fatalf("两个租户各保留 1 行，实际 %d 行", n)
	}

	if err := db.Exec(`
		INSERT INTO geo_index_trackings (tenant_id, article_id, engine)
		VALUES (311, 'a-1', 'kimi')`).Error; err == nil {
		t.Fatal("同租户重复追踪仍能写入：唯一索引未生效")
	}
	if err := db.Exec(`
		INSERT INTO geo_index_trackings (tenant_id, article_id, engine)
		VALUES (313, 'a-1', 'kimi')`).Error; err != nil {
		t.Fatalf("跨租户同键被误拒: %v", err)
	}

	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("迁移重入失败: %v", err)
	}
}
