// entity_tenant_test.go GEO 实体与关系的租户契约。
//
// geo_entities / geo_entity_relations 是租户表：列表与关系图只含本租户；
// 关系的 (tenant_id, entity_a_id, entity_b_id, relation) 唯一索引保证
// 重复抽取不会写出重复边，且不同租户的同名实体关系互不冲突。
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
	getTenantA = uint(281)
	getTenantB = uint(282)
)

func getSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.GeoEntity{}, &model.GeoEntityRelation{})
	if db == nil {
		t.Fatal("测试库不可达：租户遮挡判不了（不 Skip，跳过等于没锁）")
	}
	// 唯一索引在生产由 v3.72.0 迁移创建；测试库每次 DropTable 重建，这里补上同形状索引。
	if err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_entity_rel
		ON geo_entity_relations (tenant_id, entity_a_id, entity_b_id, relation)`).Error; err != nil {
		t.Fatalf("建立关系唯一索引失败: %v", err)
	}
	return db
}

func TestGeoEntityRepository_ListTenantScopedAndLikeEscaped(t *testing.T) {
	db := getSetupDB(t)
	repo := &geoEntityRepo{db: db}
	ctxA := tenant.WithTenantID(context.Background(), getTenantA)
	ctxB := tenant.WithTenantID(context.Background(), getTenantB)

	if err := repo.Create(ctxA, &model.GeoEntity{Name: "alpha"}); err != nil {
		t.Fatalf("租户 A 创建实体失败: %v", err)
	}
	if err := repo.Create(ctxA, &model.GeoEntity{Name: "100%增长"}); err != nil {
		t.Fatalf("租户 A 创建含通配符实体失败: %v", err)
	}
	if err := repo.Create(ctxB, &model.GeoEntity{Name: "beta"}); err != nil {
		t.Fatalf("租户 B 创建实体失败: %v", err)
	}

	listA, totalA, err := repo.List(ctxA, "", "", 1, 20)
	if err != nil {
		t.Fatalf("租户 A 列表失败: %v", err)
	}
	if totalA != 2 || len(listA) != 2 {
		t.Fatalf("租户 A 列表应 2 条，实际 total=%d len=%d", totalA, len(listA))
	}
	for _, e := range listA {
		if e.TenantID != getTenantA {
			t.Fatalf("列表越界到租户 %d", e.TenantID)
		}
	}

	// % 是 LIKE 通配符：未转义时搜索 "%" 会命中全部行。
	_, wildcardTotal, err := repo.List(ctxA, "%", "", 1, 20)
	if err != nil {
		t.Fatalf("通配符搜索失败: %v", err)
	}
	if wildcardTotal != 1 {
		t.Fatalf("搜索 %% 应只命中名字里真含 %% 的 1 条，实际 %d", wildcardTotal)
	}
}

func TestGeoEntityRepository_UpsertRelationIdempotentAndTenantScoped(t *testing.T) {
	db := getSetupDB(t)
	repo := &geoEntityRepo{db: db}
	ctxA := tenant.WithTenantID(context.Background(), getTenantA)
	ctxB := tenant.WithTenantID(context.Background(), getTenantB)

	rel := &model.GeoEntityRelation{EntityAID: 1, EntityBID: 2, Relation: "is_a"}
	if err := repo.UpsertRelation(ctxA, rel); err != nil {
		t.Fatalf("租户 A 首次写入关系失败: %v", err)
	}
	if err := repo.UpsertRelation(ctxA, &model.GeoEntityRelation{EntityAID: 1, EntityBID: 2, Relation: "is_a"}); err != nil {
		t.Fatalf("租户 A 重复写入关系失败: %v", err)
	}
	if err := repo.UpsertRelation(ctxB, &model.GeoEntityRelation{EntityAID: 1, EntityBID: 2, Relation: "is_a"}); err != nil {
		t.Fatalf("租户 B 写入同名关系失败（唯一索引未按租户隔离）: %v", err)
	}

	var rows []model.GeoEntityRelation
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		Where("entity_a_id = ? AND entity_b_id = ? AND relation = ?", 1, 2, "is_a").
		Find(&rows).Error; err != nil {
		t.Fatalf("回查关系失败: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("两个租户各一行关系，实际 %d 行", len(rows))
	}
	countByTenant := map[uint]int{}
	for _, row := range rows {
		countByTenant[row.TenantID]++
	}
	if countByTenant[getTenantA] != 1 || countByTenant[getTenantB] != 1 {
		t.Fatalf("关系未按租户唯一: %+v", countByTenant)
	}

	graph, entities, err := repo.GetRelationGraph(ctxA, 1, 1)
	if err != nil {
		t.Fatalf("关系图查询失败: %v", err)
	}
	if len(graph) != 1 || len(entities) != 0 {
		t.Fatalf("租户 A 关系图应 1 条边，实际 edges=%d entities=%d", len(graph), len(entities))
	}
}
