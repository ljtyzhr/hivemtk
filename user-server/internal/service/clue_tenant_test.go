// clue_tenant_test.go Register 的 (type, account) 去重必须在租户内生效（实体 #60 审计补充）。
//
// 去重从仓储搬到 service 层之后，「重复数据」的判据变成一次显式查询。这条用例钉住：
// 同租户重复被拒、另一租户同名账号照常入库 —— 判据漏掉租户谓词时，B 租户的
// 同一联系人会被 A 租户的行判成重复而整批丢失。
package service

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/repository"
)

const (
	clueSvcTenantA = uint(295)
	clueSvcTenantB = uint(296)
)

func TestClueService_RegisterDedupIsTenantScoped(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	database := testutil.NewTestDB(t, &model.Clue{})
	svc := NewClueService(repository.NewClueRepositoryWithDB(database), nil)
	ctxA := tenant.WithTenantID(context.Background(), clueSvcTenantA)
	ctxB := tenant.WithTenantID(context.Background(), clueSvcTenantB)

	clue := model.Clue{Type: 9, Account: "same-account", Name: "A 的线索"}
	if _, err := svc.Register(ctxA, clue); err != nil {
		t.Fatalf("租户 A 首次 Register 失败: %v", err)
	}
	if _, err := svc.Register(ctxA, clue); err == nil {
		t.Fatal("同租户重复 (type, account) 应被拒绝")
	}
	clueB := clue
	clueB.Name = "B 的线索"
	if _, err := svc.Register(ctxB, clueB); err != nil {
		t.Fatalf("另一租户的同名账号不应被判重复: %v", err)
	}

	var rows []model.Clue
	sys := tenant.WithSystemScope(context.Background())
	if err := database.WithContext(sys).Order("tenant_id").Find(&rows).Error; err != nil {
		t.Fatalf("系统作用域回读失败: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("两租户各应有 1 行，实际 %d", len(rows))
	}
	if rows[0].TenantID != clueSvcTenantA || rows[1].TenantID != clueSvcTenantB {
		t.Errorf("tenant_id 章不对: %d, %d", rows[0].TenantID, rows[1].TenantID)
	}
}

// TestClueService_RegisterAllowsEmptyAccount 空 account 不参与去重：
// 否则同类型下第二条起的所有空账号线索都会被判成「重复数据」丢掉。
func TestClueService_RegisterAllowsEmptyAccount(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	database := testutil.NewTestDB(t, &model.Clue{})
	svc := NewClueService(repository.NewClueRepositoryWithDB(database), nil)
	ctx := tenant.WithTenantID(context.Background(), clueSvcTenantA)

	for i := 0; i < 3; i++ {
		if _, err := svc.Register(ctx, model.Clue{Type: 1, Account: "", Name: "无账号"}); err != nil {
			t.Fatalf("第 %d 条空账号线索不应被判重复: %v", i+1, err)
		}
	}
	var n int64
	sys := tenant.WithSystemScope(context.Background())
	if err := database.WithContext(sys).Model(&model.Clue{}).Count(&n).Error; err != nil {
		t.Fatalf("计数失败: %v", err)
	}
	if n != 3 {
		t.Errorf("应入库 3 条，实际 %d", n)
	}
}
