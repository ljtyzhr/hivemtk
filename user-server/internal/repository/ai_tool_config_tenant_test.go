// ai_tool_config_tenant_test.go AI 工具配置的租户契约。
//
// ai_tool_configs.tool_name 的 uniqueIndex 经 tenancyidx.Normalize 复合化为
// (tenant_id, tool_name)。这里钉住「跨租户同名工具可并存、读写只看本租户、
// 跨租户改状态不得假成功」。
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
	atcTenantA = uint(141)
	atcTenantB = uint(142)
)

func atcSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.AIToolConfig{})
	if db == nil {
		t.Fatal("测试库不可达：租户遮挡判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func atcSeed(t *testing.T, db *gorm.DB, tenantID uint, name string, enabled bool) *model.AIToolConfig {
	t.Helper()
	row := &model.AIToolConfig{
		TenantID:  tenantID,
		ToolName:  name,
		Category:  "customer",
		IsEnabled: enabled,
	}
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).Create(row).Error; err != nil {
		t.Fatalf("预置租户 %d 工具 %s 失败: %v", tenantID, name, err)
	}
	return row
}

func TestAIToolConfigRepository_SameToolNameAcrossTenants(t *testing.T) {
	db := atcSetupDB(t)
	seedA := atcSeed(t, db, atcTenantA, "shared.tool", true)
	seedB := atcSeed(t, db, atcTenantB, "shared.tool", true)
	if seedA.ID == seedB.ID {
		t.Fatal("两租户同名工具应是两条独立记录")
	}

	repo := NewAIToolConfigRepository(db)
	gotA, err := repo.GetByName(tenant.WithTenantID(context.Background(), atcTenantA), "shared.tool")
	if err != nil || gotA.ID != seedA.ID {
		t.Fatalf("租户 A 应读到自己的工具: err=%v row=%+v", err, gotA)
	}
	gotB, err := repo.GetByName(tenant.WithTenantID(context.Background(), atcTenantB), "shared.tool")
	if err != nil || gotB.ID != seedB.ID {
		t.Fatalf("租户 B 应读到自己的工具: err=%v row=%+v", err, gotB)
	}
}

func TestAIToolConfigRepository_ListTenantScoped(t *testing.T) {
	db := atcSetupDB(t)
	atcSeed(t, db, atcTenantA, "a.tool", true)
	atcSeed(t, db, atcTenantB, "b.tool", true)

	repo := NewAIToolConfigRepository(db)
	tools, total, err := repo.List(tenant.WithTenantID(context.Background(), atcTenantA), "", nil, 1, 20)
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if len(tools) != 1 || tools[0].ToolName != "a.tool" || total != 1 {
		t.Fatalf("期望仅本租户 a.tool，实际 tools=%+v total=%d", tools, total)
	}
}

func TestAIToolConfigRepository_UpdateStatusTenantScoped(t *testing.T) {
	db := atcSetupDB(t)
	seedB := atcSeed(t, db, atcTenantB, "b.status", true)
	repo := NewAIToolConfigRepository(db)
	ctxA := tenant.WithTenantID(context.Background(), atcTenantA)
	ctxB := tenant.WithTenantID(context.Background(), atcTenantB)

	if err := repo.UpdateStatus(ctxA, "b.status", false); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户 UpdateStatus 期望 ErrRecordNotFound，实际: %v", err)
	}

	var after model.AIToolConfig
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).First(&after, seedB.ID).Error; err != nil {
		t.Fatalf("回查租户 B 工具失败: %v", err)
	}
	if !after.IsEnabled {
		t.Fatal("租户 B 的启用状态被跨租户改写")
	}

	if err := repo.UpdateStatus(ctxB, "b.status", false); err != nil {
		t.Fatalf("本租户 UpdateStatus 失败: %v", err)
	}
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).First(&after, seedB.ID).Error; err != nil {
		t.Fatalf("回查租户 B 工具失败: %v", err)
	}
	if after.IsEnabled {
		t.Fatal("本租户 UpdateStatus 未落库")
	}
}

func TestAIToolConfigRepository_BatchUpdateStatusTenantScoped(t *testing.T) {
	db := atcSetupDB(t)
	seedB := atcSeed(t, db, atcTenantB, "b.batch", true)
	repo := NewAIToolConfigRepository(db)
	ctxA := tenant.WithTenantID(context.Background(), atcTenantA)

	if err := repo.BatchUpdateStatus(ctxA, []string{"b.batch"}, false); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户 BatchUpdateStatus 期望 ErrRecordNotFound，实际: %v", err)
	}

	var after model.AIToolConfig
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).First(&after, seedB.ID).Error; err != nil {
		t.Fatalf("回查租户 B 工具失败: %v", err)
	}
	if !after.IsEnabled {
		t.Fatal("租户 B 的启用状态被跨租户批量改写")
	}
}
