// aftersale_repo_tenant_test.go 售后单仓储的租户契约。
//
// 售后单由 AI 客服工具 aftersale.create/query 写入与读取，ctx 来自 agent 运行时；
// 这里钉死 on 模式下「创建自动落本租户、跨租户读不到、按客户查询只含本租户」。
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
	asTenantA = uint(81)
	asTenantB = uint(82)
)

func asSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.AfterSale{})
	if db == nil {
		t.Fatal("测试库不可达：租户遮挡判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func asSeed(t *testing.T, db *gorm.DB, tenantID uint, orderID string) *model.AfterSale {
	t.Helper()
	row := &model.AfterSale{
		TenantID:      tenantID,
		Platform:      "taobao",
		OrderID:       orderID,
		CustomerPhone: "13800000000",
		Type:          "refund",
		Status:        "pending",
	}
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).Create(row).Error; err != nil {
		t.Fatalf("预置租户 %d 的售后单失败: %v", tenantID, err)
	}
	return row
}

func TestAfterSaleRepository_CreateFillsTenant(t *testing.T) {
	db := asSetupDB(t)
	repo := NewAfterSaleRepositoryWithDB(db)

	ctxA := tenant.WithTenantID(context.Background(), asTenantA)
	row := &model.AfterSale{Platform: "jd", OrderID: "JD-CREATE-A", Type: "refund", Status: "pending"}
	if err := repo.Create(ctxA, row); err != nil {
		t.Fatalf("Create 失败: %v", err)
	}
	if row.TenantID != asTenantA {
		t.Errorf("期望 tenant_id=%d，实际 %d", asTenantA, row.TenantID)
	}
}

// 跨租户回写必须按"不存在"处理：电商回推的状态只能落到本租户自己的售后单上。
func TestAfterSaleRepository_Update_CrossTenantNotFound(t *testing.T) {
	db := asSetupDB(t)
	theirs := asSeed(t, db, asTenantB, "AS-THEIRS-B")

	repo := NewAfterSaleRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), asTenantA)

	theirs.Status = "refunded"
	if err := repo.Update(ctxA, theirs); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户 Update 期望 ErrRecordNotFound，实际: %v", err)
	}

	// 本租户更新成功。
	mine := asSeed(t, db, asTenantA, "AS-MINE-A")
	mine.Status = "refunded"
	if err := repo.Update(ctxA, mine); err != nil {
		t.Fatalf("本租户 Update 失败: %v", err)
	}
}

func TestAfterSaleRepository_Query_OnlyOwnTenant(t *testing.T) {
	db := asSetupDB(t)
	asSeed(t, db, asTenantA, "AS-ORDER-A")
	asSeed(t, db, asTenantB, "AS-ORDER-B")

	repo := NewAfterSaleRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), asTenantA)

	byPhone, err := repo.ListByCustomer(ctxA, "13800000000")
	if err != nil {
		t.Fatalf("ListByCustomer 失败: %v", err)
	}
	if len(byPhone) != 1 || byPhone[0].OrderID != "AS-ORDER-A" {
		t.Fatalf("按客户查询应只含本租户 AS-ORDER-A，实际: %+v", byPhone)
	}

	byOrder, err := repo.ListByOrder(ctxA, "taobao", "AS-ORDER-B")
	if err != nil {
		t.Fatalf("ListByOrder 失败: %v", err)
	}
	if len(byOrder) != 0 {
		t.Fatalf("跨租户按订单查询应为空，实际 %d 条", len(byOrder))
	}
}
