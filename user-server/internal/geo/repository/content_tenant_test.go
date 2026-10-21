// content_tenant_test.go GEO 文章与优化记录的租户契约。
//
// geo_articles / geo_optimizations 都是租户表：列表只含本租户，跨租户更新
// 必须报不存在，更新不得复活已删除文章。
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
	gctTenantA = uint(191)
	gctTenantB = uint(192)
)

func gctSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.GeoArticle{}, &model.GeoOptimization{})
	if db == nil {
		t.Fatal("测试库不可达：租户遮挡判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func gctSeedArticle(t *testing.T, db *gorm.DB, tenantID uint, title string) *model.GeoArticle {
	t.Helper()
	row := &model.GeoArticle{TenantID: tenantID, Title: title, Status: "draft"}
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).Create(row).Error; err != nil {
		t.Fatalf("预置租户 %d 文章失败: %v", tenantID, err)
	}
	return row
}

func TestGeoArticleRepository_CreateAndListTenantScoped(t *testing.T) {
	db := gctSetupDB(t)
	repo := &geoArticleRepo{db: db}
	ctxA := tenant.WithTenantID(context.Background(), gctTenantA)
	ctxB := tenant.WithTenantID(context.Background(), gctTenantB)

	a := &model.GeoArticle{Title: "a", Status: "draft"}
	if err := repo.Create(ctxA, a); err != nil {
		t.Fatalf("租户 A 创建文章失败: %v", err)
	}
	if err := repo.Create(ctxB, &model.GeoArticle{Title: "b", Status: "draft"}); err != nil {
		t.Fatalf("租户 B 创建文章失败: %v", err)
	}
	if a.TenantID != gctTenantA {
		t.Fatalf("创建未填租户: %d", a.TenantID)
	}

	list, total, err := repo.GetList(ctxA, "", "", 1, 20)
	if err != nil {
		t.Fatalf("GetList 失败: %v", err)
	}
	if total != 1 || len(list) != 1 || list[0].TenantID != gctTenantA {
		t.Fatalf("租户 A 文章列表越界: total=%d list=%+v", total, list)
	}
}

func TestGeoArticleRepository_UpdateCrossTenantNotFound(t *testing.T) {
	db := gctSetupDB(t)
	rowB := gctSeedArticle(t, db, gctTenantB, "b")

	repo := &geoArticleRepo{db: db}
	ctxA := tenant.WithTenantID(context.Background(), gctTenantA)

	rowB.Title = "hijacked"
	if err := repo.Update(ctxA, rowB); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户更新应报不存在，实际 %v", err)
	}
	var back model.GeoArticle
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).First(&back, "id = ?", rowB.ID).Error; err != nil {
		t.Fatalf("读回租户 B 文章失败: %v", err)
	}
	if back.Title == "hijacked" || back.TenantID != gctTenantB {
		t.Fatalf("跨租户更新改动了他人文章: %+v", back)
	}
}

func TestGeoArticleRepository_UpdateDeletedNotResurrected(t *testing.T) {
	db := gctSetupDB(t)
	row := gctSeedArticle(t, db, gctTenantA, "a")

	repo := &geoArticleRepo{db: db}
	ctxA := tenant.WithTenantID(context.Background(), gctTenantA)
	if err := db.WithContext(ctxA).Delete(&model.GeoArticle{}, "id = ?", row.ID).Error; err != nil {
		t.Fatalf("删除文章失败: %v", err)
	}

	row.Title = "resurrected"
	if err := repo.Update(ctxA, row); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("更新已删除文章应报不存在，实际 %v", err)
	}
	var active int64
	db.WithContext(tenant.WithSystemScope(context.Background())).Model(&model.GeoArticle{}).
		Where("id = ?", row.ID).Count(&active)
	if active != 0 {
		t.Fatalf("已删除文章被更新复活（活动行 %d）", active)
	}
}

func TestGeoOptimizationRepository_ListTenantScoped(t *testing.T) {
	db := gctSetupDB(t)
	repo := &geoOptimizationRepo{db: db}
	ctxA := tenant.WithTenantID(context.Background(), gctTenantA)
	ctxB := tenant.WithTenantID(context.Background(), gctTenantB)

	if err := repo.Create(ctxA, &model.GeoOptimization{ArticleID: "art-a", Model: "m"}); err != nil {
		t.Fatalf("租户 A 写优化记录失败: %v", err)
	}
	if err := repo.Create(ctxB, &model.GeoOptimization{ArticleID: "art-b", Model: "m"}); err != nil {
		t.Fatalf("租户 B 写优化记录失败: %v", err)
	}

	list, total, err := repo.GetList(ctxA, "", 1, 20)
	if err != nil {
		t.Fatalf("GetList 失败: %v", err)
	}
	if total != 1 || len(list) != 1 || list[0].ArticleID != "art-a" {
		t.Fatalf("租户 A 优化记录越界: total=%d list=%+v", total, list)
	}
}
