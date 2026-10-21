// competitor_tenant_test.go 竞品清单的租户契约。
//
// geo_competitors 是租户表：写入自动落本租户，列表只含本租户，跨租户改/删
// 必须报不存在；更新不得把已删除的竞品重新插入。
package repository

import (
	"context"
	"errors"
	"testing"

	"hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	gcTenantA = uint(171)
	gcTenantB = uint(172)
)

func gcSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.GeoCompetitor{})
	if db == nil {
		t.Fatal("测试库不可达：租户遮挡判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func gcSeed(t *testing.T, db *gorm.DB, tenantID uint, domain string) *model.GeoCompetitor {
	t.Helper()
	row := &model.GeoCompetitor{
		TenantID: tenantID,
		Name:     "rival",
		Domain:   domain,
		Status:   "active",
		Priority: 5,
	}
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).Create(row).Error; err != nil {
		t.Fatalf("预置租户 %d 竞品失败: %v", tenantID, err)
	}
	return row
}

func TestGeoCompetitorRepository_CreateFillsTenant(t *testing.T) {
	db := gcSetupDB(t)
	repo := &geoCompetitorRepo{db: db}

	row := &model.GeoCompetitor{Name: "rival", Domain: "rival.example", Status: "active"}
	if err := repo.Create(tenant.WithTenantID(context.Background(), gcTenantA), row); err != nil {
		t.Fatalf("Create 失败: %v", err)
	}
	if row.TenantID != gcTenantA {
		t.Errorf("期望 tenant_id=%d，实际 %d", gcTenantA, row.TenantID)
	}
}

func TestGeoCompetitorRepository_ListTenantScoped(t *testing.T) {
	db := gcSetupDB(t)
	gcSeed(t, db, gcTenantA, "a.example")
	gcSeed(t, db, gcTenantB, "b.example")

	repo := &geoCompetitorRepo{db: db}
	list, err := repo.List(tenant.WithTenantID(context.Background(), gcTenantA), "")
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if len(list) != 1 || list[0].TenantID != gcTenantA {
		t.Fatalf("租户 A 应只见自己的 1 条竞品，实际 %+v", list)
	}
}

func TestGeoCompetitorRepository_UpdateCrossTenantNotFound(t *testing.T) {
	db := gcSetupDB(t)
	rowB := gcSeed(t, db, gcTenantB, "b.example")

	repo := &geoCompetitorRepo{db: db}
	ctxA := tenant.WithTenantID(context.Background(), gcTenantA)

	rowB.Name = "hijacked"
	if err := repo.Update(ctxA, rowB); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户更新应报记录不存在，实际 %v", err)
	}

	var back model.GeoCompetitor
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).First(&back, rowB.ID).Error; err != nil {
		t.Fatalf("读回租户 B 竞品失败: %v", err)
	}
	if back.Name == "hijacked" {
		t.Fatal("跨租户更新不应改动他人竞品")
	}
	if back.TenantID != gcTenantB {
		t.Fatalf("竞品归属被改写：期望 %d，实际 %d", gcTenantB, back.TenantID)
	}
}

func TestGeoCompetitorRepository_UpdateDeletedRowNotResurrected(t *testing.T) {
	db := gcSetupDB(t)
	rowA := gcSeed(t, db, gcTenantA, "a.example")

	repo := &geoCompetitorRepo{db: db}
	ctxA := tenant.WithTenantID(context.Background(), gcTenantA)
	if err := repo.Delete(ctxA, rowA.ID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}

	rowA.Name = "resurrected"
	if err := repo.Update(ctxA, rowA); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("更新已删除竞品应报记录不存在，实际 %v", err)
	}
	var active int64
	db.WithContext(tenant.WithSystemScope(context.Background())).Model(&model.GeoCompetitor{}).
		Where("id = ?", rowA.ID).Count(&active)
	if active != 0 {
		t.Fatalf("已删除竞品被更新操作复活（活动行 %d）", active)
	}
}

func TestGeoCompetitorRepository_DeleteCrossTenantNotFound(t *testing.T) {
	db := gcSetupDB(t)
	rowB := gcSeed(t, db, gcTenantB, "b.example")

	repo := &geoCompetitorRepo{db: db}
	ctxA := tenant.WithTenantID(context.Background(), gcTenantA)

	if err := repo.Delete(ctxA, rowB.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户删除应报记录不存在，实际 %v", err)
	}
	var cnt int64
	db.WithContext(tenant.WithSystemScope(context.Background())).Model(&model.GeoCompetitor{}).
		Where("id = ?", rowB.ID).Count(&cnt)
	if cnt != 1 {
		t.Fatalf("跨租户删除不应删掉他人竞品，实际剩余 %d 条", cnt)
	}
}
