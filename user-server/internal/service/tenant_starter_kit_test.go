package service

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

// 开户赠送 provisioning 与租户隔离（003 T013/T014）
//
// 判据钉在「行落库形态 + 跨租户可见性」上：
//   - T013：注册后 ai_agents/asset_bundles 各有租户行；重复调用不重复创建（幂等）；
//   - T014：租户副本只在本租户可见 —— 换租户 ctx 查询必须为空
//     （fail-closed 教训：任何未走隔离注入的裸查询都会串数据）。

func setupStarterKitTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	database := testutil.NewTestDB(t,
		&model.Tenant{}, &model.SystemUser{}, &model.AIAgent{}, &model.AssetBundle{},
		&model.TenantModule{}, &model.ModuleSyncLog{}, &model.ModuleOrder{},
	)
	db.SetTestDB(database)
	return database
}

// TestProvisionTenantStarterKit_CreatesAndIdempotent 赠送创建 + 幂等
func TestProvisionTenantStarterKit_CreatesAndIdempotent(t *testing.T) {
	withTenantMode(t, tenant.ModeOn)
	database := setupStarterKitTestDB(t)
	ctx := context.Background()

	// 前置：全局 default_sales 种子（与迁移 v2.2.0 同形）
	sysCtx := tenant.WithSystemScope(ctx)
	seed := &model.AIAgent{
		TenantID:  1,
		AgentCode: starterKitAgentCode,
		Name:      "默认销售智能体",
		AgentType: string(model.AgentTypeSales),
		Persona:   "你是销售专家。",
		Status:    1,
	}
	if err := database.WithContext(sysCtx).Create(seed).Error; err != nil {
		t.Fatalf("种下全局 default_sales 失败: %v", err)
	}

	const tenantID = uint(42)
	provisionTenantStarterKit(ctx, tenantID, database)

	// 智能体：租户 42 有一份副本
	var agents []model.AIAgent
	if err := database.WithContext(sysCtx).
		Where("tenant_id = ? AND agent_code = ?", tenantID, starterKitAgentCode).
		Find(&agents).Error; err != nil {
		t.Fatalf("查询租户智能体失败: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("应恰有 1 份租户级智能体副本，实际 %d", len(agents))
	}
	if agents[0].Persona != seed.Persona {
		t.Errorf("副本应继承种子人设，实际 %q", agents[0].Persona)
	}

	// 资产包：租户 42 有一份 starter-bundle
	var bundles []model.AssetBundle
	if err := database.WithContext(sysCtx).
		Where("tenant_id = ? AND asset_id = ?", tenantID, starterKitTemplateKey).
		Find(&bundles).Error; err != nil {
		t.Fatalf("查询租户资产包失败: %v", err)
	}
	if len(bundles) != 1 {
		t.Fatalf("应恰有 1 个起步资产包，实际 %d", len(bundles))
	}
	if len(bundles[0].Messages) == 0 {
		t.Error("起步资产包 messages 不应为空")
	}

	// 幂等：重复调用不重复创建
	provisionTenantStarterKit(ctx, tenantID, database)
	provisionTenantStarterKit(ctx, tenantID, database)

	var agentCount, bundleCount int64
	if err := database.WithContext(sysCtx).Model(&model.AIAgent{}).
		Where("tenant_id = ? AND agent_code = ?", tenantID, starterKitAgentCode).
		Count(&agentCount).Error; err != nil {
		t.Fatalf("统计智能体失败: %v", err)
	}
	if agentCount != 1 {
		t.Errorf("重复 provisioning 后智能体应仍为 1，实际 %d", agentCount)
	}
	if err := database.WithContext(sysCtx).Model(&model.AssetBundle{}).
		Where("tenant_id = ? AND asset_id = ?", tenantID, starterKitTemplateKey).
		Count(&bundleCount).Error; err != nil {
		t.Fatalf("统计资产包失败: %v", err)
	}
	if bundleCount != 1 {
		t.Errorf("重复 provisioning 后资产包应仍为 1，实际 %d", bundleCount)
	}
}

// TestTenantIsolation_StarterKitAssets 跨租户不可见（T014 核心判据）
func TestTenantIsolation_StarterKitAssets(t *testing.T) {
	withTenantMode(t, tenant.ModeOn)
	database := setupStarterKitTestDB(t)
	ctx := context.Background()
	sysCtx := tenant.WithSystemScope(ctx)

	seed := &model.AIAgent{
		TenantID: 1, AgentCode: starterKitAgentCode, Name: "默认销售智能体",
		AgentType: string(model.AgentTypeSales), Persona: "你是销售专家。", Status: 1,
	}
	if err := database.WithContext(sysCtx).Create(seed).Error; err != nil {
		t.Fatalf("种下全局 default_sales 失败: %v", err)
	}

	provisionTenantStarterKit(ctx, 61, database)
	provisionTenantStarterKit(ctx, 62, database)

	// 本租户可见自己的副本
	ctx61 := tenant.WithTenantID(ctx, 61)
	var agents61 []model.AIAgent
	if err := database.WithContext(ctx61).
		Where("agent_code = ?", starterKitAgentCode).
		Find(&agents61).Error; err != nil {
		t.Fatalf("租户 61 查询智能体失败: %v", err)
	}
	if len(agents61) != 1 {
		t.Fatalf("租户 61 应只见自己的 1 份副本，实际 %d（隔离注入失效会看到 2 份）", len(agents61))
	}
	if agents61[0].TenantID != 61 {
		t.Errorf("租户 61 看到的行归属应为 61，实际 %d", agents61[0].TenantID)
	}

	// 换租户：租户 62 只见自己的副本，不见 61 的
	ctx62 := tenant.WithTenantID(ctx, 62)
	var agents62 []model.AIAgent
	if err := database.WithContext(ctx62).
		Where("agent_code = ?", starterKitAgentCode).
		Find(&agents62).Error; err != nil {
		t.Fatalf("租户 62 查询智能体失败: %v", err)
	}
	if len(agents62) != 1 {
		t.Fatalf("租户 62 应只见自己的 1 份副本，实际 %d", len(agents62))
	}
	if agents62[0].TenantID != 62 {
		t.Errorf("租户 62 看到的行归属应为 62，实际 %d", agents62[0].TenantID)
	}

	// 资产包同判据
	var bundles61 []model.AssetBundle
	if err := database.WithContext(ctx61).
		Where("asset_id = ?", starterKitTemplateKey).
		Find(&bundles61).Error; err != nil {
		t.Fatalf("租户 61 查询资产包失败: %v", err)
	}
	if len(bundles61) != 1 || bundles61[0].TenantID != 61 {
		t.Fatalf("租户 61 资产包可见性异常: %+v", bundles61)
	}
	var bundles62 []model.AssetBundle
	if err := database.WithContext(ctx62).
		Where("asset_id = ?", starterKitTemplateKey).
		Find(&bundles62).Error; err != nil {
		t.Fatalf("租户 62 查询资产包失败: %v", err)
	}
	if len(bundles62) != 1 || bundles62[0].TenantID != 62 {
		t.Fatalf("租户 62 资产包可见性异常（应只见自己那份）: %+v", bundles62)
	}
}

// TestProvisionTenantStarterKit_NoSeedStillSafe 种子缺失时不阻断（失败仅 Warn）
func TestProvisionTenantStarterKit_NoSeedStillSafe(t *testing.T) {
	withTenantMode(t, tenant.ModeOn)
	database := setupStarterKitTestDB(t)
	ctx := context.Background()

	// 不种 default_sales：provisioning 必须静默跳过克隆且不 panic
	provisionTenantStarterKit(ctx, 71, database)

	sysCtx := tenant.WithSystemScope(ctx)
	var agentCount, bundleCount int64
	_ = db.GetDB().WithContext(sysCtx).Model(&model.AIAgent{}).
		Where("tenant_id = ?", uint(71)).Count(&agentCount).Error
	_ = db.GetDB().WithContext(sysCtx).Model(&model.AssetBundle{}).
		Where("tenant_id = ?", uint(71)).Count(&bundleCount).Error
	if agentCount != 0 {
		t.Errorf("种子缺失时不应克隆出智能体，实际 %d", agentCount)
	}
	// 资产包不依赖种子，应照常创建
	if bundleCount != 1 {
		t.Errorf("起步资产包不依赖种子，应创建 1 个，实际 %d", bundleCount)
	}
}
