// verification_tenant_test.go 验证结果与 API 成本记录的租户契约。
//
// geo_verify_results / geo_api_calls 都是租户表：明细查询与聚合统计都只能看到本租户。
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
	gvrTenantA = uint(377)
	gvrTenantB = uint(378)
)

func gvrSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.GeoVerifyResult{}, &model.GeoAPICall{})
	if db == nil {
		t.Fatal("测试库不可达：租户遮挡判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func TestGeoVerifyResultRepository_TenantScoped(t *testing.T) {
	db := gvrSetupDB(t)
	repo := NewGeoVerifyResultRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), gvrTenantA)
	ctxB := tenant.WithTenantID(context.Background(), gvrTenantB)

	resA := &model.GeoVerifyResult{
		ArticleID: "art-1", BrandName: "蜂巢", Model: "m1", Query: "q",
		BrandMentioned: true, MentionCount: 2,
	}
	if err := repo.Create(ctxA, resA); err != nil {
		t.Fatalf("租户 A 写入失败: %v", err)
	}
	if resA.TenantID != gvrTenantA {
		t.Fatalf("Create 未填租户: %d", resA.TenantID)
	}
	if err := repo.Create(ctxB, &model.GeoVerifyResult{
		ArticleID: "art-1", BrandName: "蜂巢", Model: "m2", Query: "q", BrandMentioned: false,
	}); err != nil {
		t.Fatalf("租户 B 写入失败: %v", err)
	}

	byArticleA, err := repo.GetByArticleID(ctxA, "art-1")
	if err != nil {
		t.Fatalf("租户 A 按文章查询失败: %v", err)
	}
	if len(byArticleA) != 1 || byArticleA[0].TenantID != gvrTenantA {
		t.Fatalf("租户 A 按文章查询越界: %+v", byArticleA)
	}

	byBrandA, err := repo.GetByBrandName(ctxA, "蜂巢")
	if err != nil {
		t.Fatalf("租户 A 按品牌查询失败: %v", err)
	}
	if len(byBrandA) != 1 || byBrandA[0].TenantID != gvrTenantA {
		t.Fatalf("租户 A 按品牌查询越界: %+v", byBrandA)
	}

	// 聚合统计只算本租户：A 只有 m1 一条。
	statsA, err := repo.GetStatistics(ctxA)
	if err != nil {
		t.Fatalf("租户 A 统计失败: %v", err)
	}
	if len(statsA) != 1 {
		t.Fatalf("租户 A 统计组数 = %d，期望 1: %+v", len(statsA), statsA)
	}
	if got := toString(statsA[0]["model"]); got != "m1" {
		t.Fatalf("租户 A 统计串租户: model=%q", got)
	}
	if got := toInt64(statsA[0]["total"]); got != 1 {
		t.Fatalf("租户 A 统计条数 = %d，期望 1", got)
	}
}

func TestGeoAPICallRepository_TenantScoped(t *testing.T) {
	db := gvrSetupDB(t)
	repo := NewGeoAPICallRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), gvrTenantA)
	ctxB := tenant.WithTenantID(context.Background(), gvrTenantB)

	if err := repo.Create(ctxA, &model.GeoAPICall{
		Provider: "openai", Model: "gpt", InputTokens: 10, OutputTokens: 5,
		CostUSD: 0.1, CostCNY: 0.7, Purpose: "a",
	}); err != nil {
		t.Fatalf("租户 A 写入失败: %v", err)
	}
	if err := repo.Create(ctxB, &model.GeoAPICall{
		Provider: "openai", Model: "gpt", InputTokens: 1000, OutputTokens: 500,
		CostUSD: 9, CostCNY: 63, Purpose: "b",
	}); err != nil {
		t.Fatalf("租户 B 写入失败: %v", err)
	}

	statsA, err := repo.GetCostStatistics(ctxA, "", "")
	if err != nil {
		t.Fatalf("租户 A 成本统计失败: %v", err)
	}
	if len(statsA) != 1 {
		t.Fatalf("租户 A 成本统计组数 = %d，期望 1: %+v", len(statsA), statsA)
	}
	if got := toInt64(statsA[0]["input_tokens"]); got != 10 {
		t.Fatalf("租户 A 输入 token 聚合越界: %d（期望 10，说明把租户 B 的算进来了）", got)
	}
	if got := toInt64(statsA[0]["call_count"]); got != 1 {
		t.Fatalf("租户 A 调用次数 = %d，期望 1", got)
	}
}

func toString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func toInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int32:
		return int64(n)
	case int:
		return int64(n)
	case float64:
		return int64(n)
	}
	return -1
}
