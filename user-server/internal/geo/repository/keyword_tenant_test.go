// keyword_tenant_test.go 关键词与关键词分组的租户契约。
//
// geo_keywords / geo_keyword_groups 都是租户表：列表只含本租户；
// 跨租户删除必须报不存在；分组按 (tenant_id, name) 幂等 upsert，
// 重复写入更新而不是新增，软删除行可被重新启用。
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
	gkwTenantA = uint(331)
	gkwTenantB = uint(332)
)

func gkwSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.GeoKeyword{}, &model.GeoKeywordGroup{})
	if db == nil {
		t.Fatal("测试库不可达：租户遮挡判不了（不 Skip，跳过等于没锁）")
	}
	// 唯一索引在生产由 v3.74.0 迁移创建；测试库每次 DropTable 重建，这里补上同形状索引。
	if err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_keyword_group_tenant_name
		ON geo_keyword_groups (tenant_id, name)`).Error; err != nil {
		t.Fatalf("建立关键词分组唯一索引失败: %v", err)
	}
	return db
}

func TestGeoKeywordRepository_ListAndDeleteTenantScoped(t *testing.T) {
	db := gkwSetupDB(t)
	repo := NewGeoKeywordRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), gkwTenantA)
	ctxB := tenant.WithTenantID(context.Background(), gkwTenantB)

	kwA := &model.GeoKeyword{Keyword: "GEO优化", Source: "ai", Layer: "seed", Status: "active"}
	if err := repo.Create(ctxA, kwA); err != nil {
		t.Fatalf("租户 A 创建关键词失败: %v", err)
	}
	if err := repo.Create(ctxB, &model.GeoKeyword{Keyword: "GEO优化", Source: "ai", Layer: "seed", Status: "active"}); err != nil {
		t.Fatalf("租户 B 创建关键词失败: %v", err)
	}

	listA, totalA, err := repo.GetList(ctxA, "", "", "", "", "", "", 0, 0)
	if err != nil {
		t.Fatalf("租户 A 列表失败: %v", err)
	}
	if totalA != 1 || len(listA) != 1 || listA[0].TenantID != gkwTenantA {
		t.Fatalf("租户 A 关键词列表越界: total=%d list=%+v", totalA, listA)
	}

	// page/limit 非法值必须被夹取，而不是拼出负 OFFSET / LIMIT 0。
	if _, _, err := repo.GetList(ctxA, "", "", "", "", "", "", -3, -1); err != nil {
		t.Fatalf("非法分页参数应被夹取: %v", err)
	}

	if err := repo.Delete(ctxB, kwA.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户删除应报不存在，实际 err=%v", err)
	}
	if _, totalA, _ := repo.GetList(ctxA, "", "", "", "", "", "", 1, 10); totalA != 1 {
		t.Fatalf("跨租户删除不应删掉租户 A 的关键词，剩余 %d", totalA)
	}
}

func TestGeoKeywordGroupRepository_UpsertByNameIdempotentAndTenantScoped(t *testing.T) {
	db := gkwSetupDB(t)
	repo := NewGeoKeywordGroupRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), gkwTenantA)
	ctxB := tenant.WithTenantID(context.Background(), gkwTenantB)

	if _, err := repo.UpsertByName(ctxA, "对比类", []string{"a", "b"}); err != nil {
		t.Fatalf("租户 A 首次 upsert 失败: %v", err)
	}
	if _, err := repo.UpsertByName(ctxA, "对比类", []string{"a", "b", "c"}); err != nil {
		t.Fatalf("租户 A 重复 upsert 失败: %v", err)
	}
	if _, err := repo.UpsertByName(ctxB, "对比类", []string{"x"}); err != nil {
		t.Fatalf("租户 B 同名分组 upsert 失败（唯一索引未按租户隔离）: %v", err)
	}

	var rows []model.GeoKeywordGroup
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		Where("name = ?", "对比类").Find(&rows).Error; err != nil {
		t.Fatalf("回查分组失败: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("两个租户各一行分组，实际 %d 行", len(rows))
	}
	for _, row := range rows {
		if row.TenantID == gkwTenantA && (row.KeywordCount != 3 || row.KeywordList == "") {
			t.Fatalf("重复 upsert 未更新租户 A 的分组: %+v", row)
		}
	}

	// 软删除后再次 upsert 应让分组重新可用。
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		Model(&model.GeoKeywordGroup{}).
		Where("tenant_id = ? AND name = ?", gkwTenantA, "对比类").
		Update("deleted_at", time.Now()).Error; err != nil {
		t.Fatalf("软删除分组失败: %v", err)
	}
	if _, err := repo.UpsertByName(ctxA, "对比类", []string{"a"}); err != nil {
		t.Fatalf("软删除后 upsert 失败: %v", err)
	}
	var revived model.GeoKeywordGroup
	if err := db.WithContext(ctxA).Where("name = ?", "对比类").First(&revived).Error; err != nil {
		t.Fatalf("软删除后 upsert 未恢复分组: %v", err)
	}
	if revived.KeywordCount != 1 {
		t.Fatalf("恢复后的分组内容不新: %+v", revived)
	}
}
