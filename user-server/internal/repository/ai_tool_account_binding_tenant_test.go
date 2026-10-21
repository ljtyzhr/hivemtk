// ai_tool_account_binding_tenant_test.go AI 工具账号绑定的租户契约。
//
// idx_tool_account 的唯一索引经 tenancyidx.Normalize 复合化为
// (tenant_id, tool_name, account_type, account_id)。这里钉住「绑定自动落本租户、
// 同三元组两租户可并存且列表互不可见、跨租户删除不可见、主账号切换原子」。
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
	atbTenantA = uint(131)
	atbTenantB = uint(132)
)

func atbSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.AIToolAccountBinding{})
	if db == nil {
		t.Fatal("测试库不可达：租户遮挡判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func atbBind(t *testing.T, db *gorm.DB, tenantID uint, tool, accountType, accountID string, primary bool) *model.AIToolAccountBinding {
	t.Helper()
	row := &model.AIToolAccountBinding{
		ToolName:    tool,
		AccountType: accountType,
		AccountID:   accountID,
		IsPrimary:   primary,
		Enabled:     true,
	}
	if err := NewAIToolAccountBindingRepository(db).Bind(
		tenant.WithTenantID(context.Background(), tenantID), row,
	); err != nil {
		t.Fatalf("租户 %d 绑定 %s/%s/%s 失败: %v", tenantID, tool, accountType, accountID, err)
	}
	return row
}

func TestAIToolAccountBindingRepository_BindFillsTenant(t *testing.T) {
	db := atbSetupDB(t)
	row := atbBind(t, db, atbTenantA, "reach", "wechat", "wx-1", false)
	if row.TenantID != atbTenantA {
		t.Errorf("期望 tenant_id=%d，实际 %d", atbTenantA, row.TenantID)
	}
}

func TestAIToolAccountBindingRepository_SameBindingAcrossTenants(t *testing.T) {
	db := atbSetupDB(t)
	atbBind(t, db, atbTenantA, "reach", "wechat", "shared", false)
	atbBind(t, db, atbTenantB, "reach", "wechat", "shared", false)

	repo := NewAIToolAccountBindingRepository(db)
	listA, err := repo.ListByTool(tenant.WithTenantID(context.Background(), atbTenantA), "reach")
	if err != nil {
		t.Fatalf("租户 A ListByTool 失败: %v", err)
	}
	if len(listA) != 1 || listA[0].TenantID != atbTenantA {
		t.Fatalf("租户 A 应只见自己的 1 条绑定，实际: %+v", listA)
	}

	listB, err := repo.ListByTool(tenant.WithTenantID(context.Background(), atbTenantB), "reach")
	if err != nil {
		t.Fatalf("租户 B ListByTool 失败: %v", err)
	}
	if len(listB) != 1 || listB[0].TenantID != atbTenantB {
		t.Fatalf("租户 B 应只见自己的 1 条绑定，实际: %+v", listB)
	}
	if listA[0].ID == listB[0].ID {
		t.Fatal("两租户同三元组应是两条独立记录")
	}
}

func TestAIToolAccountBindingRepository_CrossTenantDeleteBlocked(t *testing.T) {
	db := atbSetupDB(t)
	seedB := atbBind(t, db, atbTenantB, "reach", "wechat", "b-only", false)
	repo := NewAIToolAccountBindingRepository(db)

	err := repo.Delete(tenant.WithTenantID(context.Background(), atbTenantA), "reach", "wechat", "b-only")
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户 Delete 期望 ErrRecordNotFound，实际: %v", err)
	}

	var count int64
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		Model(&model.AIToolAccountBinding{}).Where("id = ?", seedB.ID).Count(&count).Error; err != nil {
		t.Fatalf("回查租户 B 绑定失败: %v", err)
	}
	if count != 1 {
		t.Fatalf("租户 B 的绑定被跨租户删除，剩余 %d 条", count)
	}
}

func TestAIToolAccountBindingRepository_PrimarySwitchAtomic(t *testing.T) {
	db := atbSetupDB(t)
	atbBind(t, db, atbTenantA, "reach", "wechat", "acc-1", true)
	second := atbBind(t, db, atbTenantA, "reach", "wechat", "acc-2", true)

	repo := NewAIToolAccountBindingRepository(db)
	ctx := tenant.WithTenantID(context.Background(), atbTenantA)

	list, err := repo.ListByTool(ctx, "reach")
	if err != nil {
		t.Fatalf("ListByTool 失败: %v", err)
	}
	primaryCount := 0
	for _, row := range list {
		if row.IsPrimary {
			primaryCount++
			if row.AccountID != "acc-2" {
				t.Fatalf("主账号应是后绑的 acc-2，实际 %s", row.AccountID)
			}
		}
	}
	if primaryCount != 1 {
		t.Fatalf("同工具同类型应只有 1 个主账号，实际 %d", primaryCount)
	}

	// 重复绑定撞唯一索引：事务回滚，原主账号不能被清成"无主"半截状态
	dup := &model.AIToolAccountBinding{
		ToolName:    "reach",
		AccountType: "wechat",
		AccountID:   "acc-2",
		IsPrimary:   true,
		Enabled:     true,
	}
	if err := repo.Bind(ctx, dup); err == nil {
		t.Fatal("重复绑定应撞唯一索引报错")
	}
	var after model.AIToolAccountBinding
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).First(&after, second.ID).Error; err != nil {
		t.Fatalf("回查主账号失败: %v", err)
	}
	if !after.IsPrimary {
		t.Fatal("重复绑定失败后原主账号被清空：事务没有回滚")
	}
}
