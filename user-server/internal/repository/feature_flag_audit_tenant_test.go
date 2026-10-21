// feature_flag_audit_tenant_test.go FeatureFlagAuditLog 的租户契约。
//
// 审计行是「谁改了哪个开关」的唯一凭据：写的时候必须落对租户（on 模式漏绑 ctx 要拒，
// 绝不能落默认租户），读的时候只能读到本租户的审计。两条都靠隔离 callback + 显式填充，
// 这里用真实库钉住。off 模式保留历史语义：显式落默认租户。
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
	ffTenantA = uint(71)
	ffTenantB = uint(72)
)

func ffSetupDB(t *testing.T, mode tenant.Mode) *gorm.DB {
	t.Helper()
	tenant.SetMode(mode)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.FeatureFlag{}, &model.FeatureFlagAuditLog{}, &model.FeatureFlagEvalLog{})
	if db == nil {
		t.Fatal("测试库不可达：审计行的租户归属判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func ffSeedFlag(t *testing.T, db *gorm.DB, tenantID uint, key string) *model.FeatureFlag {
	t.Helper()
	row := &model.FeatureFlag{TenantID: tenantID, Key: key, Name: key, RolloutPercentage: 100}
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).Create(row).Error; err != nil {
		t.Fatalf("预置租户 %d 的开关失败: %v", tenantID, err)
	}
	return row
}

func TestFeatureFlagAudit_WritesAndReadsAreTenantScoped(t *testing.T) {
	db := ffSetupDB(t, tenant.ModeOn)
	flagA := ffSeedFlag(t, db, ffTenantA, "ff_a")
	flagB := ffSeedFlag(t, db, ffTenantB, "ff_b")

	repo := NewFeatureFlagRepository(db)
	ctxA := tenant.WithTenantID(context.Background(), ffTenantA)
	ctxB := tenant.WithTenantID(context.Background(), ffTenantB)

	if err := repo.CreateAudit(ctxA, &model.FeatureFlagAuditLog{FlagID: flagA.ID, FlagKey: "ff_a", Action: "update"}); err != nil {
		t.Fatalf("租户 A 写审计失败: %v", err)
	}
	if err := repo.CreateAudit(ctxB, &model.FeatureFlagAuditLog{FlagID: flagB.ID, FlagKey: "ff_b", Action: "delete"}); err != nil {
		t.Fatalf("租户 B 写审计失败: %v", err)
	}

	// 行的租户归属必须与发起方一致，而不是列默认值 1。
	var physical model.FeatureFlagAuditLog
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		Where("flag_key = ?", "ff_a").First(&physical).Error; err != nil {
		t.Fatalf("物理读取审计行失败: %v", err)
	}
	if physical.TenantID != ffTenantA {
		t.Fatalf("审计行 tenant_id=%d，期望 %d", physical.TenantID, ffTenantA)
	}

	listA, err := repo.ListAudit(ctxA, flagA.ID, 100)
	if err != nil {
		t.Fatalf("租户 A 读审计失败: %v", err)
	}
	if len(listA) != 1 || listA[0].Action != "update" {
		t.Fatalf("租户 A 读到自己 flag 的审计 %+v，期望仅 1 条 update", listA)
	}

	// 拿 B 的 flagID 在 A 的 ctx 下查：租户条件把行挡掉，只能得到空集。
	crossA, err := repo.ListAudit(ctxA, flagB.ID, 100)
	if err != nil {
		t.Fatalf("跨租户读审计调用失败: %v", err)
	}
	if len(crossA) != 0 {
		t.Fatalf("租户 A 读到了租户 B 的审计 %+v ⇒ 过滤没生效", crossA)
	}
}

// TestFeatureFlagAudit_UnboundCtxIsRejected on 模式下漏绑租户的审计写入必须被拒：
// 放行会落默认租户 1，审计记到别人头上，且写入方拿不到任何失败信号。
func TestFeatureFlagAudit_UnboundCtxIsRejected(t *testing.T) {
	db := ffSetupDB(t, tenant.ModeOn)
	repo := NewFeatureFlagRepository(db)

	err := repo.CreateAudit(context.Background(), &model.FeatureFlagAuditLog{
		FlagID: 1, FlagKey: "ff_x", Action: "update",
	})
	if err == nil {
		t.Fatal("on 模式无租户 ctx 写审计成功 ⇒ 行会落默认租户")
	}
}

// TestFeatureFlagAudit_OffModeWritesDefaultTenant 离线单机没有租户概念：
// 显式落默认租户，不依赖列默认值，直连仓储的调用方也能看到真实归属。
func TestFeatureFlagAudit_OffModeWritesDefaultTenant(t *testing.T) {
	db := ffSetupDB(t, tenant.ModeOff)
	repo := NewFeatureFlagRepository(db)

	if err := repo.CreateAudit(context.Background(), &model.FeatureFlagAuditLog{
		FlagID: 1, FlagKey: "ff_off", Action: "create",
	}); err != nil {
		t.Fatalf("off 模式写审计失败: %v", err)
	}
	var physical model.FeatureFlagAuditLog
	if err := db.WithContext(context.Background()).Where("flag_key = ?", "ff_off").First(&physical).Error; err != nil {
		t.Fatalf("读取审计行失败: %v", err)
	}
	if physical.TenantID != tenant.DefaultTenantID {
		t.Fatalf("off 模式审计行 tenant_id=%d，期望默认租户 %d", physical.TenantID, tenant.DefaultTenantID)
	}
}
