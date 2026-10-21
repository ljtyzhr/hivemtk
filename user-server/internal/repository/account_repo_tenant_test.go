// account_repo_tenant_test.go 账号仓储的租户读写契约。
//
// on 模式下查询会被租户回调自动拼上 tenant_id，这里把「跨租户读不到、列表只含本租户」
// 钉死，防止后续把隔离条件挪进业务层时悄悄漏掉。
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
	accTenantA = uint(71)
	accTenantB = uint(72)
)

func accSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.Account{})
	if db == nil {
		t.Fatal("测试库不可达：租户遮挡判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func accSeed(t *testing.T, db *gorm.DB, tenantID uint, id string) *model.Account {
	t.Helper()
	row := &model.Account{
		TenantID:   tenantID,
		ID:         id,
		TgName:     "tg-" + id,
		TgBotToken: "token-" + id,
	}
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).Create(row).Error; err != nil {
		t.Fatalf("预置租户 %d 的账号失败: %v", tenantID, err)
	}
	return row
}

func TestAccountRepository_GetByID_CrossTenantNotFound(t *testing.T) {
	db := accSetupDB(t)
	accSeed(t, db, accTenantA, "acc-tenant-a")
	theirs := accSeed(t, db, accTenantB, "acc-tenant-b")

	repo := NewAccountRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), accTenantA)

	if _, err := repo.GetByID(ctxA, theirs.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户 GetByID 期望 ErrRecordNotFound，实际: %v", err)
	}

	mine, err := repo.GetByID(ctxA, "acc-tenant-a")
	if err != nil {
		t.Fatalf("本租户 GetByID 失败: %v", err)
	}
	if mine.TenantID != accTenantA {
		t.Errorf("期望 tenant_id=%d，实际 %d", accTenantA, mine.TenantID)
	}
}

func TestAccountRepository_GetAccountList_TenantScoped(t *testing.T) {
	db := accSetupDB(t)
	accSeed(t, db, accTenantA, "acc-list-a")
	accSeed(t, db, accTenantB, "acc-list-b")

	repo := NewAccountRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), accTenantA)

	rows, err := repo.GetAccountList(ctxA)
	if err != nil {
		t.Fatalf("GetAccountList 失败: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("期望仅本租户 1 条，实际 %d 条", len(rows))
	}
	if rows[0].ID != "acc-list-a" {
		t.Errorf("期望 acc-list-a，实际 %s", rows[0].ID)
	}
}
