// tenant_orphan_row_repair_migration_test.go 孤儿行归位迁移的回归。
//
// 判据两条：
//
//	① 目标租户已有同一业务键的行 ⇒ 孤儿行是重复副本，删掉孤儿行、保留目标行
//	   （不是把孤儿行改成目标租户 —— 那样必然撞唯一键 23505）；
//	② 目标租户没有对应行 ⇒ 孤儿行是真的漏了归属，归位到默认租户 1。
//
// 幂等：跑第二遍不再有行可改，也不报错。
package migrations

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	opsmodel "hivemtk-user/internal/ops/model"
	"hivemtk-user/internal/pkg/testutil"
)

func TestTenantOrphanRowRepairMigration_RepairsOrphanRows(t *testing.T) {
	db := testutil.NewTestDB(t, &opsmodel.ChurnPrediction{}, &model.CustomerRFM{})
	if db == nil {
		t.Fatal("测试库不可达：孤儿行归位判不了（不 Skip，跳过等于没锁）")
	}
	// 孤儿行（tenant_id=0）**必须用原生 SQL 播种**：GORM 会把零值字段从 INSERT
	// 里省掉、让列默认值（1）生效，而且隔离 callback 在 Create 前还要回填
	// tenant_id —— 两条路都写不出 tenant_id=0 的行。系统作用域只豁免回填，
	// 不豁免列默认值。
	ctx := context.Background()
	seed := func(sql string, args ...any) {
		t.Helper()
		if err := db.WithContext(ctx).Exec(sql, args...).Error; err != nil {
			t.Fatalf("seed 失败: %v", err)
		}
	}

	// customer_rfm：一条有孪生行（重复副本），一条没有（真孤儿）。
	seed(`INSERT INTO customer_rfm (tenant_id, customer_id, composite_score, computed_at, created_at, updated_at)
	      VALUES (0, 'rfm-dup-cust', 10, now(), now(), now()),
	             (1, 'rfm-dup-cust', 99, now(), now(), now()),
	             (0, 'rfm-orphan-cust', 20, now(), now(), now())`)

	// churn_predictions：同样两条。
	seed(`INSERT INTO churn_predictions (tenant_id, user_id, churn_score, churn_risk, created_at, updated_at)
	      VALUES (0, 'churn-dup-user', 10, 'low', now(), now()),
	             (1, 'churn-dup-user', 99, 'critical', now(), now()),
	             (0, 'churn-orphan-user', 20, 'low', now(), now())`)

	m := NewTenantOrphanRowRepairMigration(db)
	if err := m.Up(ctx); err != nil {
		t.Fatalf("迁移执行失败: %v", err)
	}

	// 不该再有 tenant_id=0 的行。
	for _, table := range []string{"customer_rfm", "churn_predictions"} {
		var n int64
		row := db.WithContext(ctx).Raw("SELECT count(*) FROM " + table + " WHERE tenant_id = 0").Row()
		if err := row.Scan(&n); err != nil {
			t.Fatalf("统计 %s 孤儿行失败: %v", table, err)
		}
		if n != 0 {
			t.Errorf("%s 仍剩 %d 条 tenant_id=0 的孤儿行", table, n)
		}
	}

	// 孪生行：保留的是目标租户那条，且业务列没被动过。
	var keptTwin model.CustomerRFM
	if err := db.WithContext(ctx).Where("customer_id = ?", "rfm-dup-cust").First(&keptTwin).Error; err != nil {
		t.Fatalf("读回孪生行失败: %v", err)
	}
	if keptTwin.TenantID != 1 {
		t.Errorf("孪生行租户=%d want 1", keptTwin.TenantID)
	}
	if keptTwin.CompositeScore != 99 {
		t.Errorf("孪生行被覆盖了: score=%v want 99（孤儿副本不该改到目标行）", keptTwin.CompositeScore)
	}

	// 真孤儿：归位到默认租户 1。
	var moved model.CustomerRFM
	if err := db.WithContext(ctx).Where("customer_id = ?", "rfm-orphan-cust").First(&moved).Error; err != nil {
		t.Fatalf("读回归位行失败: %v", err)
	}
	if moved.TenantID != 1 {
		t.Errorf("孤儿行租户=%d want 1", moved.TenantID)
	}

	// 幂等：再跑一遍不报错也不改数。
	if err := m.Up(ctx); err != nil {
		t.Fatalf("迁移应幂等，第二遍却失败: %v", err)
	}
}
