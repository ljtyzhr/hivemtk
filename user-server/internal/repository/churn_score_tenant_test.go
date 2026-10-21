// churn_score_tenant_test.go churn_scores 的租户契约（实体 #59 审计补充）。
//
// 这张表的写入是一条显式指定冲突列的 upsert：`ON CONFLICT (tenant_id, customer_key)`。
// 冲突列一旦与库里的唯一键形状不一致，整批写入会直接报错（还好），但若唯一键仍是
// 单列 `customer_key`，B 租户的同名客户就会被 A 的行静默覆盖 —— 那是一条没有任何
// 报错的跨租户数据串用。本文件把三件事钉在真库上：
//  1. 唯一键已复合为 (tenant_id, customer_key)：两租户同名客户各存一行；
//  2. 同租户内重复 upsert 是更新而不是新增（customer_key 幂等），且只改自己的行；
//  3. 读取（ListByPAliveBelow / Count）只看本租户。
package repository

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	csTenantA = uint(291)
	csTenantB = uint(292)
)

func csSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.ChurnScore{})
	if db == nil {
		t.Fatal("测试库不可达：churn_scores 的租户契约判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func csRow(key string, pAlive float64) model.ChurnScore {
	return model.ChurnScore{
		CustomerKey:          key,
		X:                    3,
		Tx:                   20,
		TObs:                 40,
		PAlive:               pAlive,
		ExpectedPurchases30d: 0.5,
		Params:               `{"r":0.5}`,
		StatsCount:           1,
		ComputedAt:           time.Now(),
	}
}

func TestChurnScoreRepository_UpsertIsTenantScoped(t *testing.T) {
	db := csSetupDB(t)
	repo := NewChurnScoreRepository(db)
	ctxA := tenant.WithTenantID(context.Background(), csTenantA)
	ctxB := tenant.WithTenantID(context.Background(), csTenantB)
	sys := tenant.WithSystemScope(context.Background())

	if err := repo.UpsertBatch(ctxA, []model.ChurnScore{csRow("cust-1", 0.8)}); err != nil {
		t.Fatalf("租户 A 首次 upsert 失败: %v", err)
	}
	if err := repo.UpsertBatch(ctxB, []model.ChurnScore{csRow("cust-1", 0.1)}); err != nil {
		t.Fatalf("租户 B 同名客户应能独立落一行: %v", err)
	}

	var rows []model.ChurnScore
	if err := db.WithContext(sys).Order("tenant_id").Find(&rows).Error; err != nil {
		t.Fatalf("系统作用域回读失败: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("应有 2 行（每租户一条），实际 %d：唯一键未按租户复合化", len(rows))
	}
	if rows[0].TenantID != csTenantA || rows[1].TenantID != csTenantB {
		t.Fatalf("tenant_id 章不对: %d, %d", rows[0].TenantID, rows[1].TenantID)
	}

	// 同租户重复 upsert：更新而非新增，且不得动 B 的行。
	if err := repo.UpsertBatch(ctxA, []model.ChurnScore{csRow("cust-1", 0.05)}); err != nil {
		t.Fatalf("租户 A 二次 upsert 失败: %v", err)
	}
	if n, err := repo.Count(ctxA); err != nil || n != 1 {
		t.Fatalf("A 的行数应恒为 1（upsert 幂等）, got %d err=%v", n, err)
	}
	var aRow model.ChurnScore
	if err := db.WithContext(sys).Where("tenant_id = ? AND customer_key = ?", csTenantA, "cust-1").
		First(&aRow).Error; err != nil {
		t.Fatalf("回读 A 行失败: %v", err)
	}
	if aRow.PAlive != 0.05 {
		t.Errorf("A 行 p_alive=%v want 0.05（upsert 未更新冲突列）", aRow.PAlive)
	}
	var bRow model.ChurnScore
	if err := db.WithContext(sys).Where("tenant_id = ? AND customer_key = ?", csTenantB, "cust-1").
		First(&bRow).Error; err != nil {
		t.Fatalf("回读 B 行失败: %v", err)
	}
	if bRow.PAlive != 0.1 {
		t.Errorf("B 行被 A 的 upsert 改写: p_alive=%v want 0.1", bRow.PAlive)
	}

	// 读侧遮挡：A 只看得到自己那一行。
	listA, err := repo.ListByPAliveBelow(ctxA, 1.0, 100)
	if err != nil {
		t.Fatalf("ListByPAliveBelow(A): %v", err)
	}
	if len(listA) != 1 || listA[0].CustomerKey != "cust-1" {
		t.Fatalf("A 只该看见自己的 1 行，实际 %+v", listA)
	}
	if listA[0].TenantID != csTenantA {
		t.Errorf("读回的行不属于 A: tenant_id=%d", listA[0].TenantID)
	}
}

// TestChurnScoreRepository_RejectsEmptyCustomerKey 空 customer_key 是冲突列：
// 同租户内所有空键会互相覆盖成一行，且 upsert 不会报错。
func TestChurnScoreRepository_RejectsEmptyCustomerKey(t *testing.T) {
	db := csSetupDB(t)
	repo := NewChurnScoreRepository(db)
	ctxA := tenant.WithTenantID(context.Background(), csTenantA)

	if err := repo.UpsertBatch(ctxA, []model.ChurnScore{csRow("   ", 0.5)}); err == nil {
		t.Fatal("空 customer_key 应被拒绝")
	}
	var n int64
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		Model(&model.ChurnScore{}).Count(&n).Error; err != nil {
		t.Fatalf("计数失败: %v", err)
	}
	if n != 0 {
		t.Errorf("拒绝后不应落行, got %d", n)
	}
}
