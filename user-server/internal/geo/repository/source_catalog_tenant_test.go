// source_catalog_tenant_test.go 信源目录的租户契约。
//
// geo_source_catalogs 是租户表：同一 URL 在不同租户各存一份（唯一索引为
// (tenant_id, source_url)）；爬虫登记只刷新时间与非空字段，不得清空已标注等级。
package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	gscTenantA = uint(375)
	gscTenantB = uint(376)
)

func gscSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.GeoSourceCatalog{})
	if db == nil {
		t.Fatal("测试库不可达：租户遮挡判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func TestGeoSourceCatalogRepository_TenantScoped(t *testing.T) {
	db := gscSetupDB(t)
	repo := NewGeoSourceCatalogRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), gscTenantA)
	ctxB := tenant.WithTenantID(context.Background(), gscTenantB)

	itemA := &model.GeoSourceCatalog{
		SourceURL: "https://news.example/a", Domain: "news.example",
		Level: "A", Category: "央媒", Verified: true, LastChecked: time.Now(),
	}
	if err := repo.Upsert(ctxA, itemA); err != nil {
		t.Fatalf("租户 A upsert 失败: %v", err)
	}
	if itemA.TenantID != gscTenantA {
		t.Fatalf("Upsert 未填租户: %d", itemA.TenantID)
	}

	// 同 URL 在租户 B 是独立条目。
	if err := repo.Upsert(ctxB, &model.GeoSourceCatalog{
		SourceURL: "https://news.example/a", Domain: "news.example", Verified: true,
	}); err != nil {
		t.Fatalf("租户 B 同 URL upsert 失败（唯一索引未按租户复合？）: %v", err)
	}

	// 爬虫登记：只带 URL/domain 的 upsert 不得清空已标注等级。
	if err := repo.Upsert(ctxA, &model.GeoSourceCatalog{
		SourceURL: "https://news.example/a", Domain: "news.example", Verified: true,
	}); err != nil {
		t.Fatalf("重复登记失败: %v", err)
	}
	gotA, err := repo.GetByURL(ctxA, "https://news.example/a")
	if err != nil {
		t.Fatalf("回查租户 A 条目失败: %v", err)
	}
	if gotA.Level != "A" || gotA.Category != "央媒" {
		t.Fatalf("重复登记清空了已有标注: level=%q category=%q", gotA.Level, gotA.Category)
	}

	// 跨租户读到的是自己那份（level 为空）。
	gotB, err := repo.GetByURL(ctxB, "https://news.example/a")
	if err != nil {
		t.Fatalf("回查租户 B 条目失败: %v", err)
	}
	if gotB.TenantID != gscTenantB || gotB.Level != "" {
		t.Fatalf("跨租户 GetByURL 越界: %+v", gotB)
	}

	seedsA, err := repo.ListSeedURLs(ctxA)
	if err != nil {
		t.Fatalf("租户 A 种子列表失败: %v", err)
	}
	if len(seedsA) != 1 || seedsA[0].TenantID != gscTenantA {
		t.Fatalf("租户 A 种子列表越界: %+v", seedsA)
	}

	// 探测时间回写只落本租户；不存在的 id 必须报不存在。
	if err := repo.UpdateLastChecked(ctxA, itemA.ID, time.Now()); err != nil {
		t.Fatalf("回写探测时间失败: %v", err)
	}
	if err := repo.UpdateLastChecked(ctxB, itemA.ID, time.Now()); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户回写应报不存在，实际 err=%v", err)
	}
	if err := repo.UpdateLastChecked(ctxA, 999999, time.Now()); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("不存在的 id 应报不存在，实际 err=%v", err)
	}

	// 无租户 ctx 不允许写租户表。
	if err := repo.Upsert(context.Background(), &model.GeoSourceCatalog{
		SourceURL: "https://x.example/", Domain: "x.example", Verified: true,
	}); err == nil {
		t.Fatal("无租户 ctx upsert 应报错，实际成功")
	}
}

// TestGeoSourceCatalogRepository_移除default后未验证仍可落库 与 #37 同型：
// Verified 声明 default:true 时显式 false 会被省略、落库变 true。
func TestGeoSourceCatalogRepository_未验证值不被列默认值覆盖(t *testing.T) {
	db := gscSetupDB(t)
	repo := NewGeoSourceCatalogRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), gscTenantA)

	if err := repo.Upsert(ctxA, &model.GeoSourceCatalog{
		SourceURL: "https://draft.example/", Domain: "draft.example", Verified: false,
	}); err != nil {
		t.Fatalf("upsert 失败: %v", err)
	}
	got, err := repo.GetByURL(ctxA, "https://draft.example/")
	if err != nil {
		t.Fatalf("回查失败: %v", err)
	}
	if got.Verified {
		t.Fatal("显式 verified=false 落库变成了 true")
	}
	seeds, err := repo.ListSeedURLs(ctxA)
	if err != nil {
		t.Fatalf("种子列表失败: %v", err)
	}
	if len(seeds) != 0 {
		t.Fatalf("未验证条目不应进种子集: %+v", seeds)
	}
}
