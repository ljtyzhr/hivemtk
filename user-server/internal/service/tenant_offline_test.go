package service

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"
)

// 离线单机形态回归（specs/001-saas-multi-tenant US1/T018）：
// off 模式下隔离 callback 直通、数据归默认租户、行为与历史版本等价。
func TestTenantOfflineMode(t *testing.T) {
	tenant.SetMode(tenant.ModeOff)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	database := testutil.NewTestDB(t, &model.Tenant{}, &model.Customer{})

	ctx := context.Background()

	// ① off 模式写数据：tenant_id 走列默认值 1（callback 不注入、不强制 ctx）
	cust := &model.Customer{Name: "offline-user"}
	if err := database.WithContext(ctx).Create(cust).Error; err != nil {
		t.Fatalf("off 模式创建失败: %v", err)
	}
	if cust.TenantID != tenant.DefaultTenantID {
		t.Fatalf("off 模式 tenant_id 应为默认租户 1，实际 %d", cust.TenantID)
	}

	// ② off 模式读数据：无租户 ctx 照常可读（callback 直通）
	var n int64
	if err := database.WithContext(ctx).Model(&model.Customer{}).Count(&n).Error; err != nil {
		t.Fatalf("off 模式无租户 ctx 查询应直通: %v", err)
	}
	if n != 1 {
		t.Fatalf("off 模式计数 = %d，期望 1", n)
	}

	// ③ off 模式下注册服务拒绝（4104 语义在 Service 层）：离线实例不产生租户行
	var tenantRows int64
	if err := database.WithContext(ctx).Model(&model.Tenant{}).Count(&tenantRows).Error; err != nil {
		t.Fatal(err)
	}
	if tenantRows != 0 {
		t.Fatalf("off 模式下不应产生租户行: %d", tenantRows)
	}

	// ④ off 模式 Fanout 退化为单默认租户（与 T020 联动验证）
	ran := 0
	if _, err := tenant.Fanout(ctx, database, func(ctx context.Context, tenantID uint) (int64, error) {
		ran++
		if tenantID != tenant.DefaultTenantID {
			t.Errorf("off 模式扇出应绑默认租户，实际 %d", tenantID)
		}
		return 0, nil
	}); err != nil {
		t.Fatal(err)
	}
	if ran != 1 {
		t.Fatalf("off 模式扇出应恰好一次: %d", ran)
	}
}
