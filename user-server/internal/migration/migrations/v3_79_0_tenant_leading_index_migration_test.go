package migrations

// v3.79.0（SaaS 审计 A6）钉住行为：
//  1. 缺 tenant_id 前导索引的表会被补上（且补的是**前导**列，不是 `(id, tenant_id)`）；
//  2. 已有 tenant_id 索引的表不动（幂等，不重复建）；
//  3. 表或列不存在时跳过而不是报错（全新部署尚未建表）；
//  4. Up 之后复核：真库里查得到才算，语言层面的"我建了"不算；
//  5. Down 只删本迁移建的索引。

import (
	"context"
	"testing"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

func leadingIdxFixture(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.NewTestDB(t)
	if db == nil {
		t.Fatal("测试库不可达：索引是否补齐无法判定（不 Skip，跳过等于没锁）")
	}
	stmts := []string{
		`DROP TABLE IF EXISTS a6_probe_gap`,
		`DROP TABLE IF EXISTS a6_probe_done`,
		`DROP TABLE IF EXISTS a6_probe_nocol`,
		// 缺口形态：有 tenant_id 列、索引族里只有 (id) 和 (created_at)
		`CREATE TABLE a6_probe_gap (
			id bigserial PRIMARY KEY,
			tenant_id bigint NOT NULL DEFAULT 1,
			created_at timestamptz
		)`,
		`CREATE INDEX a6_probe_gap_created_at ON a6_probe_gap (created_at)`,
		// 已就绪形态：tenant_id 已是某个复合索引的前导列
		`CREATE TABLE a6_probe_done (
			id bigserial PRIMARY KEY,
			tenant_id bigint NOT NULL DEFAULT 1
		)`,
		`CREATE INDEX a6_probe_done_tenant_first ON a6_probe_done (tenant_id, id)`,
		// 非租户表：没有 tenant_id 列
		`CREATE TABLE a6_probe_nocol (id bigserial PRIMARY KEY)`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("构造夹具失败: %v\nSQL: %s", err, s)
		}
	}
	t.Cleanup(func() {
		for _, tb := range []string{"a6_probe_gap", "a6_probe_done", "a6_probe_nocol"} {
			_ = db.Exec(`DROP TABLE IF EXISTS ` + tb).Error
		}
	})
	return db
}

func TestTenantLeadingIndexMigration(t *testing.T) {
	db := leadingIdxFixture(t)
	ctx := context.Background()

	m := NewTenantLeadingIndexMigration(db).withTables("a6_probe_gap", "a6_probe_done", "a6_probe_nocol", "a6_probe_absent")
	if err := m.Up(ctx); err != nil {
		t.Fatalf("Up 失败: %v", err)
	}

	// 1) 缺口表补上了 tenant_id 前导索引
	if has, err := hasLeadingTenantIndex(db, "a6_probe_gap"); err != nil || !has {
		t.Fatalf("a6_probe_gap 应当被补上 tenant_id 前导索引，has=%v err=%v", has, err)
	}
	// 2) 已就绪表不重复建：原索引还在，且没有多出一个 idx_a6_probe_done_tenant_id
	if has, err := hasLeadingTenantIndex(db, "a6_probe_done"); err != nil || !has {
		t.Fatalf("a6_probe_done 原本就有前导索引，has=%v err=%v", has, err)
	}
	var dup int64
	if err := db.Raw(`SELECT COUNT(1) FROM pg_indexes WHERE tablename = 'a6_probe_done' AND indexname = 'idx_a6_probe_done_tenant_id'`).Scan(&dup).Error; err != nil {
		t.Fatalf("查重失败: %v", err)
	}
	if dup != 0 {
		t.Fatalf("已有 tenant_id 前导索引的表不该再建一条，实际多建了 %d 条", dup)
	}

	// 3) 可重入：再跑一次不报错
	if err := m.Up(ctx); err != nil {
		t.Fatalf("第二次 Up 应当幂等，实际: %v", err)
	}

	// 4) Down 只删本迁移建的索引，不动别人的
	if err := m.Down(ctx); err != nil {
		t.Fatalf("Down 失败: %v", err)
	}
	if has, _ := hasLeadingTenantIndex(db, "a6_probe_gap"); has {
		t.Fatalf("Down 后 a6_probe_gap 的 tenant_id 索引应当被删掉")
	}
	if has, _ := hasLeadingTenantIndex(db, "a6_probe_done"); !has {
		t.Fatalf("Down 不得动别人建的索引（a6_probe_done_tenant_first）")
	}
}

// TestTenantLeadingIndexMigration_RealGapListIsClean 真库对账：清单里的 11 张表在
// 迁移执行后必须全部有 tenant_id 前导索引；同时钉住"清单本身没有形同虚设的项"。
func TestTenantLeadingIndexMigration_RealGapListIsClean(t *testing.T) {
	db := testutil.NewTestDB(t)
	if db == nil {
		t.Fatal("测试库不可达：真库缺口是否归零无法判定")
	}
	ctx := context.Background()
	if err := NewTenantLeadingIndexMigration(db).Up(ctx); err != nil {
		t.Fatalf("Up 失败: %v", err)
	}
	// 反"空转绿"闸：全新测试库里这 11 张表可能一张都没有，那样下面的循环什么都没验证。
	// 现造两张**同名探针表**（只有 id + tenant_id），让清单驱动的代码路径在同一份判据上真跑一遍。
	createdByProbe := map[string]bool{}
	for i, table := range tenantLeadingIndexTables {
		if i >= 2 {
			break
		}
		if db.Migrator().HasTable(table) {
			continue
		}
		stmt := `CREATE TABLE ` + table + ` (id bigserial PRIMARY KEY, tenant_id bigint NOT NULL DEFAULT 1)`
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("造探针表 %s 失败: %v", table, err)
		}
		createdByProbe[table] = true
	}
	if len(createdByProbe) > 0 {
		defer func() {
			for table := range createdByProbe {
				_ = db.Exec(`DROP TABLE IF EXISTS ` + table).Error
			}
		}()
		if err := NewTenantLeadingIndexMigration(db).Up(ctx); err != nil {
			t.Fatalf("对探针表重新 Up 失败: %v", err)
		}
	}

	verified := 0
	for _, table := range tenantLeadingIndexTables {
		if !db.Migrator().HasTable(table) {
			continue // 本环境没有这张表：交由建表迁移负责
		}
		if !db.Migrator().HasColumn(table, "tenant_id") {
			t.Fatalf("%s 在清单里却没有 tenant_id 列：清单过期了（表改名/删表）", table)
		}
		has, err := hasLeadingTenantIndex(db, table)
		if err != nil {
			t.Fatalf("查 %s 索引失败: %v", table, err)
		}
		if !has {
			t.Fatalf("%s 迁移后仍缺 tenant_id 前导索引", table)
		}
		verified++
	}
	if verified == 0 {
		t.Fatalf("一张租户表都没验到：本用例会退化成空转绿")
	}
	t.Logf("真库对账：%d 张清单表均有 tenant_id 前导索引", verified)
}
