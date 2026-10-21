// daily_stats_tenant_test.go 每日聚合统计的租户与 upsert 契约。
//
// 唯一键是 Normalize 后的 (tenant_id, stat_date, engine, intent)：同租户同键覆盖/累加，
// 不同租户同键必须各存一行。
package repository

import (
	"context"
	"testing"

	"hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	gdsTenantA = uint(211)
	gdsTenantB = uint(212)
)

func gdsSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.GeoDailyStat{})
	if db == nil {
		t.Fatal("测试库不可达：租户遮挡判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func gdsCount(t *testing.T, db *gorm.DB) (int64, int64) {
	t.Helper()
	var a, b int64
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).Model(&model.GeoDailyStat{}).
		Where("tenant_id = ?", gdsTenantA).Count(&a).Error; err != nil {
		t.Fatalf("统计租户 A 行数失败: %v", err)
	}
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).Model(&model.GeoDailyStat{}).
		Where("tenant_id = ?", gdsTenantB).Count(&b).Error; err != nil {
		t.Fatalf("统计租户 B 行数失败: %v", err)
	}
	return a, b
}

func TestGeoDailyStatRepository_UpsertOverwritesWithinTenant(t *testing.T) {
	db := gdsSetupDB(t)
	repo := &geoDailyStatRepo{db: db}
	ctxA := tenant.WithTenantID(context.Background(), gdsTenantA)

	if err := repo.Upsert(ctxA, &model.GeoDailyStat{
		Date: "2026-10-22", Engine: "gpt", Intent: "buy", FunnelStage: "mofu",
		CitationCount: 3, ProbeCount: 10,
	}); err != nil {
		t.Fatalf("首次 Upsert 失败: %v", err)
	}
	if err := repo.Upsert(ctxA, &model.GeoDailyStat{
		Date: "2026-10-22", Engine: "gpt", Intent: "buy", FunnelStage: "bofu",
		CitationCount: 7, ProbeCount: 20,
	}); err != nil {
		t.Fatalf("二次 Upsert 失败: %v", err)
	}

	var rows []model.GeoDailyStat
	if err := db.WithContext(ctxA).Find(&rows).Error; err != nil {
		t.Fatalf("读回失败: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("同租户同唯一键应只有 1 行，实际 %d", len(rows))
	}
	if rows[0].CitationCount != 7 || rows[0].ProbeCount != 20 || rows[0].FunnelStage != "bofu" {
		t.Fatalf("冲突时未覆盖为最新值: %+v", rows[0])
	}
	if rows[0].TenantID != gdsTenantA {
		t.Fatalf("tenant_id 未填充: %d", rows[0].TenantID)
	}
}

func TestGeoDailyStatRepository_IncrementByEngineAccumulatesPerTenant(t *testing.T) {
	db := gdsSetupDB(t)
	repo := &geoDailyStatRepo{db: db}
	ctxA := tenant.WithTenantID(context.Background(), gdsTenantA)
	ctxB := tenant.WithTenantID(context.Background(), gdsTenantB)

	for i := 0; i < 2; i++ {
		if err := repo.IncrementByEngine(ctxA, "2026-10-22", "gpt", 2, 1); err != nil {
			t.Fatalf("租户 A 累加失败: %v", err)
		}
		if err := repo.IncrementByEngine(ctxB, "2026-10-22", "gpt", 5, 4); err != nil {
			t.Fatalf("租户 B 累加失败: %v", err)
		}
	}

	a, b := gdsCount(t, db)
	if a != 1 || b != 1 {
		t.Fatalf("两租户同唯一键应各存 1 行，实际 A=%d B=%d", a, b)
	}

	var rowA, rowB model.GeoDailyStat
	db.WithContext(tenant.WithSystemScope(context.Background())).Where("tenant_id = ?", gdsTenantA).First(&rowA)
	db.WithContext(tenant.WithSystemScope(context.Background())).Where("tenant_id = ?", gdsTenantB).First(&rowB)
	if rowA.CitationCount != 4 || rowA.BrandMentionedCount != 2 {
		t.Fatalf("租户 A 累加结果错误: %+v", rowA)
	}
	if rowB.CitationCount != 10 || rowB.BrandMentionedCount != 8 {
		t.Fatalf("租户 B 累加结果错误: %+v", rowB)
	}
}

func TestGeoDailyStatRepository_ListAndTrendTenantScoped(t *testing.T) {
	db := gdsSetupDB(t)
	repo := &geoDailyStatRepo{db: db}
	ctxA := tenant.WithTenantID(context.Background(), gdsTenantA)
	ctxB := tenant.WithTenantID(context.Background(), gdsTenantB)

	if err := repo.Upsert(ctxA, &model.GeoDailyStat{Date: "2026-10-22", Engine: "gpt", Intent: "buy", CitationCount: 1}); err != nil {
		t.Fatalf("租户 A 写入失败: %v", err)
	}
	if err := repo.Upsert(ctxB, &model.GeoDailyStat{Date: "2026-10-22", Engine: "gpt", Intent: "buy", CitationCount: 9}); err != nil {
		t.Fatalf("租户 B 写入失败: %v", err)
	}

	list, total, err := repo.List(ctxA, "", "", "", "", 1, 20)
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if total != 1 || len(list) != 1 || list[0].TenantID != gdsTenantA {
		t.Fatalf("租户 A 列表越界: total=%d list=%+v", total, list)
	}

	trend, err := repo.GetTrend(ctxA, "gpt", "", "", 30)
	if err != nil {
		t.Fatalf("GetTrend 失败: %v", err)
	}
	for _, s := range trend {
		if s.TenantID != gdsTenantA {
			t.Fatalf("趋势里混入他人数据: %+v", s)
		}
	}
}
