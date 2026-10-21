// asset_bundle_tenant_test.go 资产包的租户契约。
//
// 这张表同时压着三件事：
//  1. asset_id 的 uniqueIndex 会被 tenancyidx.Normalize 复合成 (tenant_id, asset_id)，
//     于是两个租户可以各自持有同一 AssetID（租户初始化套件就依赖这一点）；
//  2. Update 只按列白名单回写，绝不能把行搬到别的租户、也不能清掉 DTO 不携带的列；
//  3. 跨租户的 Update / SoftDelete 必须报 NotFound，而不是静默 200。
package repository

import (
	"context"
	"errors"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	abTenantA = uint(191)
	abTenantB = uint(192)
	abAssetID = "ab_shared_asset"
)

func abSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.AssetBundle{})
	if db == nil {
		t.Fatal("测试库不可达：资产包租户契约判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func abNewBundle(tenantID uint, title string) *model.AssetBundle {
	return &model.AssetBundle{
		TenantID: tenantID,
		AssetID:  abAssetID,
		Title:    title,
		Version:  "1.0.0",
		Status:   model.AssetBundleStatusDraft,
		Messages: model.AssetBundleMessages{{Role: "system", Content: "你是助手"}},
		Examples: model.JSONArray{map[string]any{"q": "示例"}},
	}
}

// TestAssetBundleRepo_SameAssetIDAcrossTenantsAndIsolation 证明 uniqueIndex 已复合化：
// 租户 B 能建出与租户 A 同名的 AssetID，而两边的读取与存在性判断互不串台。
func TestAssetBundleRepo_SameAssetIDAcrossTenantsAndIsolation(t *testing.T) {
	db := abSetupDB(t)
	repo := NewAssetBundleRepository(db)
	ctxA := tenant.WithTenantID(context.Background(), abTenantA)
	ctxB := tenant.WithTenantID(context.Background(), abTenantB)

	a := abNewBundle(0, "租户A的包")
	if err := repo.Create(ctxA, a); err != nil {
		t.Fatalf("租户 A 创建: %v", err)
	}
	if a.TenantID != abTenantA {
		t.Fatalf("租户 A TenantID = %d, want %d", a.TenantID, abTenantA)
	}

	// 服务层重复检查也必须按租户：A 已占用同名 AssetID，B 仍应被判为"不存在"。
	existsForB, err := repo.ExistsByAssetID(ctxB, abAssetID)
	if err != nil {
		t.Fatalf("租户 B ExistsByAssetID: %v", err)
	}
	if existsForB {
		t.Fatal("租户 B 看到了租户 A 的 AssetID ⇒ 存在性检查漏了租户条件")
	}

	b := abNewBundle(0, "租户B的包")
	if err := repo.Create(ctxB, b); err != nil {
		t.Fatalf("租户 B 用同一 AssetID 创建失败（uniqueIndex 未复合化？）: %v", err)
	}
	if b.TenantID != abTenantB {
		t.Fatalf("租户 B TenantID = %d, want %d", b.TenantID, abTenantB)
	}

	gotA, err := repo.FindByAssetID(ctxA, abAssetID)
	if err != nil {
		t.Fatalf("租户 A FindByAssetID: %v", err)
	}
	if gotA.Title != "租户A的包" || gotA.TenantID != abTenantA {
		t.Fatalf("租户 A 读到了别人的包: %+v", gotA)
	}
	gotB, err := repo.FindByAssetID(ctxB, abAssetID)
	if err != nil {
		t.Fatalf("租户 B FindByAssetID: %v", err)
	}
	if gotB.Title != "租户B的包" || gotB.TenantID != abTenantB {
		t.Fatalf("租户 B 读到了别人的包: %+v", gotB)
	}
}

// TestAssetBundleRepo_UpdateWhitelistKeepsIdentityAndSeedColumns 把 HTTP 更新路径
// 的真实形状喂给仓储：Identity 为空、计数为零、examples 为零。白名单回写后，
// 只有可编辑列变化；tenant_id / asset_id / examples 原样保留。
func TestAssetBundleRepo_UpdateWhitelistKeepsIdentityAndSeedColumns(t *testing.T) {
	db := abSetupDB(t)
	repo := NewAssetBundleRepository(db)
	ctxA := tenant.WithTenantID(context.Background(), abTenantA)

	seed := abNewBundle(0, "原标题")
	if err := repo.Create(ctxA, seed); err != nil {
		t.Fatalf("创建: %v", err)
	}
	seed.UseCount = 7
	if err := db.WithContext(ctxA).Model(&model.AssetBundle{}).
		Where("id = ?", seed.ID).UpdateColumn("use_count", 7).Error; err != nil {
		t.Fatalf("预置 use_count: %v", err)
	}

	// HTTP Update 构造出来的形状：只带可编辑列。messages 由 service 在空值时
	// 从旧行继承（service/asset_bundle.go UpdateBundle），这里照同一形状喂给仓储。
	shaped := &model.AssetBundle{
		ID:       seed.ID,
		Title:    "新标题",
		Status:   model.AssetBundleStatusActive,
		Version:  "1.1.0",
		Messages: seed.Messages,
	}
	if err := repo.Update(ctxA, shaped); err != nil {
		t.Fatalf("更新: %v", err)
	}

	var got model.AssetBundle
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		First(&got, seed.ID).Error; err != nil {
		t.Fatalf("回查: %v", err)
	}
	if got.Title != "新标题" || got.Status != model.AssetBundleStatusActive || got.Version != "1.1.0" {
		t.Errorf("可编辑列未写回: %+v", got)
	}
	if got.TenantID != abTenantA {
		t.Errorf("TenantID 被改成 %d ⇒ Save 式整行覆盖把行搬了租户", got.TenantID)
	}
	if got.AssetID != abAssetID {
		t.Errorf("AssetID 被改成 %q ⇒ 白名单漏了身份列", got.AssetID)
	}
	if len(got.Examples) != 1 {
		t.Errorf("Examples 被清空: %+v ⇒ DTO 不携带的列不该进更新白名单", got.Examples)
	}
	if got.UseCount != 7 {
		t.Errorf("UseCount = %d, want 7 ⇒ 更新路径覆盖了计数列", got.UseCount)
	}
}

// TestAssetBundleRepo_CrossTenantUpdateAndDeleteNotFound 跨租户写必须报
// ErrRecordNotFound（控制层据此回 404），而不是静默 200。
func TestAssetBundleRepo_CrossTenantUpdateAndDeleteNotFound(t *testing.T) {
	db := abSetupDB(t)
	repo := NewAssetBundleRepository(db)
	ctxA := tenant.WithTenantID(context.Background(), abTenantA)
	ctxB := tenant.WithTenantID(context.Background(), abTenantB)

	seed := abNewBundle(0, "租户A的包")
	if err := repo.Create(ctxA, seed); err != nil {
		t.Fatalf("创建: %v", err)
	}

	err := repo.Update(ctxB, &model.AssetBundle{ID: seed.ID, Title: "越权改名"})
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户 Update err = %v, want ErrRecordNotFound", err)
	}
	if err := repo.SoftDelete(ctxB, seed.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户 SoftDelete err = %v, want ErrRecordNotFound", err)
	}

	got, err := repo.FindByID(ctxA, seed.ID)
	if err != nil {
		t.Fatalf("租户 A 回查: %v", err)
	}
	if got.Title != "租户A的包" {
		t.Errorf("跨租户 Update 仍然改到了行: %+v", got)
	}
}
