// asset_bundle_version_log_tenant_test.go 资产包版本日志的租户契约。
//
// 这张表只有 Create 一条活路径（读取端在 #36 审计中确认为零调用者并删除），
// 于是"租户对不对"只能靠写入后回查来证明：Create 必须由 ctx 盖章，
// 且两个租户对同一 asset_id 的版本轨迹互不可见。
package repository

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	abvlTenantA = uint(181)
	abvlTenantB = uint(182)
)

func abvlSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.AssetBundleVersionLog{})
	if db == nil {
		t.Fatal("测试库不可达：租户盖章判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func abvlNewLog(from, to string) *model.AssetBundleVersionLog {
	return &model.AssetBundleVersionLog{
		AssetID:    "abvl_shared_asset",
		FromVer:    from,
		ToVer:      to,
		ChangeNote: "租户隔离用例",
		Operator:   "tester",
	}
}

// TestAssetBundleVersionLogRepo_CreateStampsTenant 模型给的 TenantID 是默认值 0，
// 落库时必须由 ctx 改写成本租户，否则跨租户轨迹会混进默认租户。
func TestAssetBundleVersionLogRepo_CreateStampsTenant(t *testing.T) {
	db := abvlSetupDB(t)
	repo := NewAssetBundleVersionLogRepository(db)

	ctxA := tenant.WithTenantID(context.Background(), abvlTenantA)
	m := abvlNewLog("1.0.0", "1.1.0")
	if err := repo.Create(ctxA, m); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if m.TenantID != abvlTenantA {
		t.Fatalf("TenantID = %d, want %d（ctx 未盖章，行会落在默认租户）", m.TenantID, abvlTenantA)
	}

	var got model.AssetBundleVersionLog
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		First(&got, m.ID).Error; err != nil {
		t.Fatalf("回查: %v", err)
	}
	if got.TenantID != abvlTenantA {
		t.Errorf("库内 TenantID = %d, want %d", got.TenantID, abvlTenantA)
	}
}

// TestAssetBundleVersionLogRepo_SameAssetAcrossTenantsIsolated 两个租户可以各自
// 记录同一个 asset_id 的版本轨迹（该表没有租户级唯一索引，这是预期），
// 但租户内查询只见自己的行。
func TestAssetBundleVersionLogRepo_SameAssetAcrossTenantsIsolated(t *testing.T) {
	db := abvlSetupDB(t)
	repo := NewAssetBundleVersionLogRepository(db)

	ctxA := tenant.WithTenantID(context.Background(), abvlTenantA)
	ctxB := tenant.WithTenantID(context.Background(), abvlTenantB)

	if err := repo.Create(ctxA, abvlNewLog("1.0.0", "1.1.0")); err != nil {
		t.Fatalf("租户 A 写入: %v", err)
	}
	if err := repo.Create(ctxB, abvlNewLog("2.0.0", "2.1.0")); err != nil {
		t.Fatalf("租户 B 写入同一 asset_id: %v", err)
	}

	var rowsA []model.AssetBundleVersionLog
	if err := db.WithContext(ctxA).Where("asset_id = ?", "abvl_shared_asset").Find(&rowsA).Error; err != nil {
		t.Fatalf("租户 A 查询: %v", err)
	}
	if len(rowsA) != 1 || rowsA[0].ToVer != "1.1.0" || rowsA[0].TenantID != abvlTenantA {
		t.Fatalf("租户 A 看到了别人的轨迹: %+v", rowsA)
	}

	var rowsB []model.AssetBundleVersionLog
	if err := db.WithContext(ctxB).Where("asset_id = ?", "abvl_shared_asset").Find(&rowsB).Error; err != nil {
		t.Fatalf("租户 B 查询: %v", err)
	}
	if len(rowsB) != 1 || rowsB[0].ToVer != "2.1.0" || rowsB[0].TenantID != abvlTenantB {
		t.Fatalf("租户 B 看到了别人的轨迹: %+v", rowsB)
	}
}
