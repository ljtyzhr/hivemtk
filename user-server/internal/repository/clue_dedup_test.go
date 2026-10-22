// clue_dedup_test.go 批量导入去重的边界（实体 #60 审计补充）。
//
// 去重键是 (type, account)。account 为空时这个键没有任何区分度：同类型的
// 第 2 条空账号线索会被判成「重复」而整条丢掉，而邮件列表等链路本来就存在
// 空账号线索。这里钉住空账号照常入库、非空账号同批/跨批仍按租户内去重。
package repository

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"
)

func TestClueRepository_BatchCreateWithDedup_KeepEmptyAccounts(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.Clue{})
	repo := NewClueRepositoryWithDB(db)
	ctx := tenant.WithTenantID(context.Background(), clueTenantA)

	batch := []*model.Clue{
		{Type: 1, Account: "", Name: "无账号 1"},
		{Type: 1, Account: "", Name: "无账号 2"},
		{Type: 1, Account: "acc-1", Name: "有账号 1"},
		{Type: 1, Account: "acc-1", Name: "有账号 1 重复"},
	}
	success, skipped, err := repo.BatchCreateWithDedup(ctx, batch)
	if err != nil {
		t.Fatalf("BatchCreateWithDedup failed: %v", err)
	}
	if success != 3 || skipped != 1 {
		t.Fatalf("success=%d skipped=%d want 3/1（空账号不判重，非空重复仍跳过）", success, skipped)
	}

	// 跨批：同名非空账号仍被跳过，空账号继续入库。
	success, skipped, err = repo.BatchCreateWithDedup(ctx, []*model.Clue{
		{Type: 1, Account: "acc-1", Name: "跨批重复"},
		{Type: 1, Account: "", Name: "无账号 3"},
	})
	if err != nil {
		t.Fatalf("第二批失败: %v", err)
	}
	if success != 1 || skipped != 1 {
		t.Fatalf("第二批 success=%d skipped=%d want 1/1", success, skipped)
	}

	var n int64
	sys := tenant.WithSystemScope(context.Background())
	if err := db.WithContext(sys).Model(&model.Clue{}).Where("type = ? AND account = ''", 1).Count(&n).Error; err != nil {
		t.Fatalf("计数失败: %v", err)
	}
	if n != 3 {
		t.Errorf("空账号线索应入库 3 条，实际 %d", n)
	}
}
