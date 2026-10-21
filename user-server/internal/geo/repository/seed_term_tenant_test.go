// seed_term_tenant_test.go 种子词条的租户契约。
//
// geo_seed_terms 是租户表：同一词条在不同租户各存一份（唯一索引为
// (tenant_id, term)）；列表只含本租户；upsert 不得改写调用方传入的切片。
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
	gstmTenantA = uint(371)
	gstmTenantB = uint(372)
)

func gstmSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.GeoSeedTerm{})
	if db == nil {
		t.Fatal("测试库不可达：租户遮挡判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func TestGeoSeedTermRepository_TenantScoped(t *testing.T) {
	db := gstmSetupDB(t)
	repo := NewGeoSeedTermRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), gstmTenantA)
	ctxB := tenant.WithTenantID(context.Background(), gstmTenantB)

	termsA := []*model.GeoSeedTerm{
		{Term: "共享词", Category: model.SeedTermCategoryTechAttr, Active: true, Sort: 1},
		{Term: "A 专有词", Category: model.SeedTermCategoryCoreTerm, BusinessLine: "A 线", Active: true, Sort: 2},
	}
	n, err := repo.UpsertBatch(ctxA, termsA)
	if err != nil {
		t.Fatalf("租户 A 播种失败: %v", err)
	}
	if n != 2 {
		t.Fatalf("租户 A 播种条数 = %d，期望 2", n)
	}
	for _, term := range termsA {
		if term.TenantID != gstmTenantA {
			t.Fatalf("UpsertBatch 未逐行填租户: %q → %d", term.Term, term.TenantID)
		}
	}

	// 同名词条在租户 B 必须能独立存在（唯一索引按租户复合）。
	if _, err := repo.UpsertBatch(ctxB, []*model.GeoSeedTerm{
		{Term: "共享词", Category: model.SeedTermCategoryBrandCombo, Active: true},
	}); err != nil {
		t.Fatalf("租户 B 同名播种失败（唯一索引未按租户复合？）: %v", err)
	}

	listA, err := repo.ListActive(ctxA)
	if err != nil {
		t.Fatalf("租户 A 列表失败: %v", err)
	}
	if len(listA) != 2 {
		t.Fatalf("租户 A 词条数 = %d，期望 2", len(listA))
	}
	for _, row := range listA {
		if row.TenantID != gstmTenantA {
			t.Fatalf("租户 A 列表越界: %+v", row)
		}
	}

	// 幂等 upsert：同租户同词条再播一次只更新字段，不新增行。
	if _, err := repo.UpsertBatch(ctxA, []*model.GeoSeedTerm{
		{Term: "共享词", Category: model.SeedTermCategoryCoreTerm, BusinessLine: "B 线", Active: true, Sort: 9},
	}); err != nil {
		t.Fatalf("重复播种失败: %v", err)
	}
	listA, err = repo.ListActive(ctxA)
	if err != nil {
		t.Fatalf("重复播种后列表失败: %v", err)
	}
	if len(listA) != 2 {
		t.Fatalf("重复播种不应新增行: %d", len(listA))
	}
	found := false
	for _, row := range listA {
		if row.Term == "共享词" {
			found = true
			if row.Category != model.SeedTermCategoryCoreTerm || row.BusinessLine != "B 线" {
				t.Fatalf("upsert 未更新字段: %+v", row)
			}
		}
	}
	if !found {
		t.Fatal("重复播种后共享词丢失")
	}

	// 启用过滤：停用的词条不出现在 ListActive。
	if _, err := repo.UpsertBatch(ctxA, []*model.GeoSeedTerm{
		{Term: "停用词", Category: model.SeedTermCategoryTechAttr, Active: false},
	}); err != nil {
		t.Fatalf("播种停用词失败: %v", err)
	}
	listA, err = repo.ListActive(ctxA)
	if err != nil {
		t.Fatalf("播种停用词后列表失败: %v", err)
	}
	for _, row := range listA {
		if row.Term == "停用词" {
			t.Fatal("停用词不应出现在启用列表")
		}
	}
}

// TestGeoSeedTermRepository_UpsertBatch不改写入参 复用 terms[:0] 压紧会原地
// 改写调用方切片（nil 被后面的元素顶掉），调用方再读入参就是错行。
func TestGeoSeedTermRepository_UpsertBatch不改写入参(t *testing.T) {
	db := gstmSetupDB(t)
	repo := NewGeoSeedTermRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), gstmTenantA)

	terms := []*model.GeoSeedTerm{
		nil,
		{Term: "词条一", Category: model.SeedTermCategoryTechAttr, Active: true},
	}
	if _, err := repo.UpsertBatch(ctxA, terms); err != nil {
		t.Fatalf("播种失败: %v", err)
	}
	if terms[0] != nil {
		t.Fatalf("入参第 0 位被改写为 %+v", terms[0])
	}
	if terms[1] == nil || terms[1].Term != "词条一" {
		t.Fatalf("入参第 1 位被改写: %+v", terms[1])
	}
}
