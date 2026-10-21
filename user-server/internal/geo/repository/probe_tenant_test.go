// probe_tenant_test.go 探针运行记录的租户契约。
//
// geo_probe_runs 是租户表：明细、引擎清单与时间窗口聚合都只能看到本租户。
package repository

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	gprTenantA = uint(361)
	gprTenantB = uint(362)
)

func gprSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.GeoProbeRun{})
	if db == nil {
		t.Fatal("测试库不可达：租户遮挡判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func TestGeoProbeRunRepository_TenantScoped(t *testing.T) {
	db := gprSetupDB(t)
	repo := NewGeoProbeRunRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), gprTenantA)
	ctxB := tenant.WithTenantID(context.Background(), gprTenantB)

	runA := &model.GeoProbeRun{Engine: "kimi", Query: "geo", BrandMentioned: true}
	if err := repo.Create(ctxA, runA); err != nil {
		t.Fatalf("租户 A 写入失败: %v", err)
	}
	if runA.TenantID != gprTenantA {
		t.Fatalf("Create 未填租户: %d", runA.TenantID)
	}
	if err := repo.Create(ctxB, &model.GeoProbeRun{Engine: "claude", Query: "geo"}); err != nil {
		t.Fatalf("租户 B 写入失败: %v", err)
	}

	recentA, err := repo.ListRecent(ctxA, 50)
	if err != nil {
		t.Fatalf("租户 A 最近记录失败: %v", err)
	}
	if len(recentA) != 1 || recentA[0].TenantID != gprTenantA {
		t.Fatalf("租户 A 最近记录越界: %+v", recentA)
	}

	listA, totalA, err := repo.ListByEngine(ctxA, "kimi", 1, 20)
	if err != nil {
		t.Fatalf("租户 A 按引擎查询失败: %v", err)
	}
	if totalA != 1 || len(listA) != 1 {
		t.Fatalf("租户 A 按引擎结果越界: total=%d len=%d", totalA, len(listA))
	}

	enginesA, err := repo.DistinctEngines(ctxA)
	if err != nil {
		t.Fatalf("租户 A 引擎清单失败: %v", err)
	}
	if len(enginesA) != 1 || enginesA[0] != "kimi" {
		t.Fatalf("租户 A 引擎清单越界: %+v", enginesA)
	}

	betweenA, err := repo.ListBetween(ctxA, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("租户 A 时间窗口查询失败: %v", err)
	}
	if len(betweenA) != 1 || betweenA[0].TenantID != gprTenantA {
		t.Fatalf("租户 A 时间窗口越界: %+v", betweenA)
	}
}
