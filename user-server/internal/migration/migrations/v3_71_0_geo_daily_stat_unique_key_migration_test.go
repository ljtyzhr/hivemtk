package migrations

// v3.71.0 钉住的是**行为**：迁移后同租户同键只能有一行、重复行的计数被累加、
// 不同租户同键仍能各存一行；只断言索引在会放过「索引存在但列不对」。

import (
	"context"
	"testing"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	gdsKeyTenantA = uint(261)
	gdsKeyTenantB = uint(262)
)

func gdsKeyFixture(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.NewTestDB(t)
	if db == nil {
		t.Fatal("测试库不可达：唯一键是否修好无法判定（不 Skip，跳过等于没锁）")
	}
	// 还原迁移前的形态：非唯一索引 + 同租户重复行。
	stmts := []string{
		`DROP TABLE IF EXISTS geo_daily_stats`,
		`CREATE TABLE geo_daily_stats (
			id bigserial PRIMARY KEY,
			tenant_id bigint NOT NULL DEFAULT 1,
			stat_date varchar(10),
			engine varchar(32),
			intent varchar(64),
			funnel_stage varchar(20),
			brand_mentioned_count integer DEFAULT 0,
			competitor_mentioned_count integer DEFAULT 0,
			citation_count integer DEFAULT 0,
			negative_count integer DEFAULT 0,
			probe_count integer DEFAULT 0,
			created_at timestamptz,
			updated_at timestamptz,
			deleted_at timestamptz
		)`,
		`CREATE INDEX idx_date_engine_intent ON geo_daily_stats (stat_date, engine, intent)`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("还原迁移前形态失败: %v\nSQL: %s", err, s)
		}
	}
	t.Cleanup(func() { _ = db.Exec(`DROP TABLE IF EXISTS geo_daily_stats`).Error })
	return db
}

func gdsKeyInsert(t *testing.T, db *gorm.DB, tenantID uint, citations, brand int) uint {
	t.Helper()
	var id uint
	if err := db.Raw(`
		INSERT INTO geo_daily_stats
			(tenant_id, stat_date, engine, intent, funnel_stage, citation_count, brand_mentioned_count)
		VALUES (?, '2026-10-22', 'gpt', 'buy', 'mofu', ?, ?)
		RETURNING id`, tenantID, citations, brand).Scan(&id).Error; err != nil {
		t.Fatalf("预置重复行失败: %v", err)
	}
	return id
}

func TestGeoDailyStatUniqueKeyMigration_DedupesAndCreatesUniqueIndex(t *testing.T) {
	db := gdsKeyFixture(t)
	gdsKeyInsert(t, db, gdsKeyTenantA, 2, 1)
	keepID := gdsKeyInsert(t, db, gdsKeyTenantA, 3, 4)
	gdsKeyInsert(t, db, gdsKeyTenantB, 9, 9)

	mig := NewGeoDailyStatUniqueKeyMigration(db)
	if err := mig.Up(context.Background()); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	var rows []struct {
		ID                  uint
		TenantID            uint
		CitationCount       int
		BrandMentionedCount int
	}
	if err := db.Raw(`SELECT id, tenant_id, citation_count, brand_mentioned_count
		FROM geo_daily_stats ORDER BY tenant_id`).Scan(&rows).Error; err != nil {
		t.Fatalf("读回失败: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("去重后应剩 2 行（每租户一行），实际 %d", len(rows))
	}
	if rows[0].TenantID != gdsKeyTenantA || rows[0].ID != keepID ||
		rows[0].CitationCount != 5 || rows[0].BrandMentionedCount != 5 {
		t.Fatalf("租户 A 重复行未合并进保留行: %+v (keepID=%d)", rows[0], keepID)
	}
	if rows[1].TenantID != gdsKeyTenantB || rows[1].CitationCount != 9 {
		t.Fatalf("租户 B 行被误改: %+v", rows[1])
	}

	// 唯一索引必须真的生效：同租户同键再插一行应当失败。
	if err := db.Exec(`INSERT INTO geo_daily_stats
		(tenant_id, stat_date, engine, intent) VALUES (?, '2026-10-22', 'gpt', 'buy')`,
		gdsKeyTenantA).Error; err == nil {
		t.Fatal("唯一索引未生效：同租户同键仍可插入")
	}
	// 不同租户同键必须仍可插入。
	if err := db.Exec(`INSERT INTO geo_daily_stats
		(tenant_id, stat_date, engine, intent) VALUES (?, '2026-10-22', 'gpt', 'buy')`,
		uint(263)).Error; err != nil {
		t.Fatalf("跨租户同键应可共存，实际 %v", err)
	}
}

func TestGeoDailyStatUniqueKeyMigration_Idempotent(t *testing.T) {
	db := gdsKeyFixture(t)
	gdsKeyInsert(t, db, gdsKeyTenantA, 1, 1)

	mig := NewGeoDailyStatUniqueKeyMigration(db)
	for i := 0; i < 2; i++ {
		if err := mig.Up(context.Background()); err != nil {
			t.Fatalf("第 %d 次迁移失败: %v", i+1, err)
		}
	}
	var n int64
	if err := db.Raw(`SELECT COUNT(*) FROM geo_daily_stats`).Scan(&n).Error; err != nil {
		t.Fatalf("计数失败: %v", err)
	}
	if n != 1 {
		t.Fatalf("幂等重入后应仍为 1 行，实际 %d", n)
	}
}
