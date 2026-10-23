// email_account_repo_tenant_test.go email_accounts 的租户契约。
//
// 这张表存的是 SMTP 凭据与发信配额：读侧漏过滤 = 拿别的租户的账号发信（既有泄凭据、
// 又有配额串账），写侧漏填充 = 配额计数打到别人的行上。仓储此前**零租户覆盖**，
// 而它的两个读口（GetByID / ListActiveBelowQuota）正是发信前的选号逻辑。
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
	eaTenantA = uint(95)
	eaTenantB = uint(96)
)

func eaSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.EmailAccount{})
	if db == nil {
		t.Fatal("测试库不可达：租户隔离判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func eaSeed(t *testing.T, db *gorm.DB, tenantID uint, name string) *model.EmailAccount {
	t.Helper()
	row := &model.EmailAccount{
		TenantID:   tenantID,
		Name:       name,
		Host:       "smtp.example.com",
		Port:       465,
		FromAddr:   name + "@example.com",
		UseSSL:     true,
		DailyQuota: 100,
		Status:     "active",
	}
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).Create(row).Error; err != nil {
		t.Fatalf("预置租户 %d 的邮件账号失败: %v", tenantID, err)
	}
	return row
}

// TestEmailAccountRepository_ReadsAreTenantScoped 选号逻辑只看得见本租户的账号。
func TestEmailAccountRepository_ReadsAreTenantScoped(t *testing.T) {
	db := eaSetupDB(t)
	mine := eaSeed(t, db, eaTenantA, "a-smtp")
	theirs := eaSeed(t, db, eaTenantB, "b-smtp")

	repo := NewEmailAccountRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), eaTenantA)

	list, err := repo.ListActiveBelowQuota(ctxA)
	if err != nil {
		t.Fatalf("ListActiveBelowQuota 失败: %v", err)
	}
	if len(list) != 1 || list[0].ID != mine.ID {
		t.Fatalf("租户 A 选到 %d 个账号（%+v），期望只有自己的 %d", len(list), list, mine.ID)
	}

	if _, err := repo.GetByID(ctxA, theirs.ID); err == nil {
		t.Fatal("租户 A 用 ID 取到了租户 B 的邮件账号 ⇒ 可用他人凭据发信")
	}
}

// TestEmailAccountRepository_QuotaIncrementStaysInTenant 配额自增不能打到别人的行上。
func TestEmailAccountRepository_QuotaIncrementStaysInTenant(t *testing.T) {
	db := eaSetupDB(t)
	mine := eaSeed(t, db, eaTenantA, "a-smtp")
	theirs := eaSeed(t, db, eaTenantB, "b-smtp")

	repo := NewEmailAccountRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), eaTenantA)

	// 故意用对方的 ID：写侧过滤没生效的话，B 的配额会被 A 的发送记账。
	if err := repo.IncDailyUsed(ctxA, theirs.ID); err != nil {
		t.Fatalf("IncDailyUsed 失败: %v", err)
	}
	if err := repo.IncDailyUsed(ctxA, mine.ID); err != nil {
		t.Fatalf("IncDailyUsed（本租户）失败: %v", err)
	}

	var a, b model.EmailAccount
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).First(&a, mine.ID).Error; err != nil {
		t.Fatalf("物理读取租户 A 账号失败: %v", err)
	}
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).First(&b, theirs.ID).Error; err != nil {
		t.Fatalf("物理读取租户 B 账号失败: %v", err)
	}
	if a.DailyUsed != 1 {
		t.Fatalf("租户 A 自己的配额应 +1，实际 %d", a.DailyUsed)
	}
	if b.DailyUsed != 0 {
		t.Fatalf("租户 B 的配额被租户 A 的发送改了：daily_used=%d", b.DailyUsed)
	}
}

// TestEmailAccountRepository_UnboundCtxIsRejected on 模式无租户 ctx 必须 fail-closed。
func TestEmailAccountRepository_UnboundCtxIsRejected(t *testing.T) {
	db := eaSetupDB(t)
	eaSeed(t, db, eaTenantA, "a-smtp")
	repo := NewEmailAccountRepositoryWithDB(db)

	if _, err := repo.ListActiveBelowQuota(context.Background()); err == nil {
		t.Fatal("无租户 ctx 选到了账号 ⇒ 会跨租户发信")
	}
}
