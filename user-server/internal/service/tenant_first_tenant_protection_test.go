package service

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"
)

// 首租户（平台运营方租户）不允许被停用的回归。
//
// 背景（2026-10-21 跨租户越权修复）：平台运营方 = 首个租户
// （tenants 表 ID 最小，见 middleware.PlatformOperatorMiddleware）。
// 若首租户可以被停用 ⇒ 平台运营方自己登录受限 ⇒ 读不到自己的租户 ⇒
// **没有任何人能再把它启用**（停用/启用都只有平台运营方能操作）——死锁。
//
// 所以防护落在 service 层的状态变更处，而不是等登录失败后再救。
//
// 关键断言：**不仅 HTTP 语义要拒，库内状态也必须没变**。
// 只断言错误码的话，"拒了但顺手改了库"这类假绿会漏过去。

func TestUpdateTenantStatusRejectsDisablingFirstTenant(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	database := testutil.NewTestDB(t, &model.Tenant{})

	// 建两个租户：ID 小的是首租户（平台运营方），ID 大的是普通租户。
	first := &model.Tenant{Code: "tfirst01", Name: "平台运营方", Status: model.TenantStatusActive}
	second := &model.Tenant{Code: "tsecond1", Name: "普通租户", Status: model.TenantStatusActive}
	if err := database.WithContext(context.Background()).Create(first).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.WithContext(context.Background()).Create(second).Error; err != nil {
		t.Fatal(err)
	}
	if second.ID <= first.ID {
		t.Fatalf("夹具前提不成立：第二个租户 ID(%d) 必须大于第一个(%d)", second.ID, first.ID)
	}

	svc := NewTenantService()

	// ① 首租户不允许停用
	if err := svc.UpdateTenantStatus(context.Background(), first.ID, model.TenantStatusSuspended); err == nil {
		t.Fatal("首租户是平台运营方租户，停用它必须被拒绝")
	}

	// ② 库内状态必须未变（不能只断言错误码）
	var afterFirst model.Tenant
	if err := database.WithContext(context.Background()).First(&afterFirst, first.ID).Error; err != nil {
		t.Fatal(err)
	}
	if afterFirst.Status != model.TenantStatusActive {
		t.Fatalf("首租户被拒绝后状态必须仍是 active，实际=%d", afterFirst.Status)
	}

	// ③ 普通租户可正常停用（别把功能一起关掉）
	if err := svc.UpdateTenantStatus(context.Background(), second.ID, model.TenantStatusSuspended); err != nil {
		t.Fatalf("普通租户停用应成功，实际=%v", err)
	}
	var afterSecond model.Tenant
	if err := database.WithContext(context.Background()).First(&afterSecond, second.ID).Error; err != nil {
		t.Fatal(err)
	}
	if afterSecond.Status != model.TenantStatusSuspended {
		t.Fatalf("普通租户应已停用，实际 status=%d", afterSecond.Status)
	}
}

func TestUpdateTenantStatusAllowsReenablingFirstTenant(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	database := testutil.NewTestDB(t, &model.Tenant{})

	first := &model.Tenant{Code: "treenabl1", Name: "平台运营方", Status: model.TenantStatusSuspended}
	if err := database.WithContext(context.Background()).Create(first).Error; err != nil {
		t.Fatal(err)
	}

	svc := NewTenantService()
	// 恢复（启用）首租户必须允许 —— 否则一旦被停用就再也救不回来。
	if err := svc.UpdateTenantStatus(context.Background(), first.ID, model.TenantStatusActive); err != nil {
		t.Fatalf("首租户重新启用必须允许，实际=%v", err)
	}
	var after model.Tenant
	if err := database.WithContext(context.Background()).First(&after, first.ID).Error; err != nil {
		t.Fatal(err)
	}
	if after.Status != model.TenantStatusActive {
		t.Fatalf("首租户应已启用，实际 status=%d", after.Status)
	}
}
