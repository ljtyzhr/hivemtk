// index_tracking_tenant_test.go 收录追踪的租户契约与幂等写入。
//
// geo_index_trackings 是租户表：同一文章同一引擎在租户内只能有一行，
// 重复验证必须更新而不是新增；不同租户的同名键互不影响。
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
	gtiTenantA = uint(301)
	gtiTenantB = uint(302)
)

func gtiSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.GeoIndexTracking{}, &model.GeoArticle{})
	if db == nil {
		t.Fatal("测试库不可达：租户遮挡判不了（不 Skip，跳过等于没锁）")
	}
	// 唯一索引在生产由 v3.73.0 迁移创建；测试库每次 DropTable 重建，这里补上同形状索引。
	if err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_tracking_article_engine
		ON geo_index_trackings (tenant_id, article_id, engine)`).Error; err != nil {
		t.Fatalf("建立收录追踪唯一索引失败: %v", err)
	}
	return db
}

func gtiTracking(engine string, cited bool, count int) *model.GeoIndexTracking {
	now := time.Now()
	return &model.GeoIndexTracking{
		ArticleID:   "article-1",
		Engine:      engine,
		URL:         "https://example.com/a",
		Keyword:     "geo",
		AICited:     cited,
		AICiteCount: count,
		LastChecked: &now,
	}
}

func TestGeoIndexTrackingRepository_UpsertCitationIdempotentAndTenantScoped(t *testing.T) {
	db := gtiSetupDB(t)
	repo := NewGeoIndexTrackingRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), gtiTenantA)
	ctxB := tenant.WithTenantID(context.Background(), gtiTenantB)

	if err := repo.UpsertCitation(ctxA, gtiTracking("kimi", false, 0)); err != nil {
		t.Fatalf("租户 A 首次写入失败: %v", err)
	}
	if err := repo.UpsertCitation(ctxA, gtiTracking("kimi", true, 3)); err != nil {
		t.Fatalf("租户 A 重复写入失败: %v", err)
	}
	if err := repo.UpsertCitation(ctxB, gtiTracking("kimi", true, 1)); err != nil {
		t.Fatalf("租户 B 写入同名键失败（唯一索引未按租户隔离）: %v", err)
	}

	listA, err := repo.ListByArticle(ctxA, "article-1")
	if err != nil {
		t.Fatalf("租户 A 回查失败: %v", err)
	}
	if len(listA) != 1 {
		t.Fatalf("同租户重复验证应只有 1 行，实际 %d 行", len(listA))
	}
	if !listA[0].AICited || listA[0].AICiteCount != 3 {
		t.Fatalf("重复写入未更新为最新探针结果: %+v", listA[0])
	}
	if listA[0].TenantID != gtiTenantA {
		t.Fatalf("写入未填租户: %d", listA[0].TenantID)
	}

	counts, err := repo.CountAICitedByEngine(ctxA)
	if err != nil {
		t.Fatalf("租户 A 引擎统计失败: %v", err)
	}
	if len(counts) != 1 || counts[0].Engine != "kimi" || counts[0].Count != 1 {
		t.Fatalf("租户 A 引擎统计越界: %+v", counts)
	}

	recentA, err := repo.Recent(ctxA, 10)
	if err != nil {
		t.Fatalf("租户 A 最近明细失败: %v", err)
	}
	if len(recentA) != 1 {
		t.Fatalf("租户 A 最近明细越界: %d 行", len(recentA))
	}
}
