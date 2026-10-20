// cron_tenant_test.go 钉住后台任务的租户注入通道：cron 交来的 ctx 不含租户，
// 而带 tenant_id 列的表在 on 模式下要求绑定租户，未绑定即被隔离守卫 fail-closed 拒绝。
package cron

import (
	"context"
	"errors"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	pkgdb "hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"github.com/google/uuid"
)

// withActiveTenants 把进程级租户模式切成 on 并注入活跃租户名单；
// 模式与枚举器都是进程级全局，用例结束成对还原，否则同包后续用例读到的扇出口径会被改写。
func withActiveTenants(t *testing.T, active ...uint) {
	t.Helper()
	prev := tenant.CurrentMode()
	tenant.SetMode(tenant.ModeOn)
	tenant.SetActiveTenantLoader(func(context.Context) ([]uint, error) { return active, nil })
	t.Cleanup(func() {
		tenant.SetActiveTenantLoader(nil)
		tenant.SetMode(prev)
	})
}

func TestFanoutTask_BindsEachActiveTenant(t *testing.T) {
	withActiveTenants(t, 7, 8)

	seen := []uint{}
	err := fanoutTask(context.Background(), func(ctx context.Context) error {
		id, ok := tenant.FromContext(ctx)
		if !ok {
			t.Errorf("任务体 ctx 未绑定租户，租户表读写会被守卫拒绝")
		}
		seen = append(seen, id)
		return nil
	})
	if err != nil {
		t.Fatalf("fanoutTask err=%v", err)
	}
	if len(seen) != 2 || seen[0] != 7 || seen[1] != 8 {
		t.Fatalf("应逐活跃租户各执行一次，实际 seen=%v want [7 8]", seen)
	}
}

// 全部租户失败时错误必须回到调用方：cron 据此出声，否则整轮任务静默空转。
func TestFanoutTask_AllTenantFailurePropagates(t *testing.T) {
	withActiveTenants(t, 7, 8)

	calls := 0
	err := fanoutTask(context.Background(), func(context.Context) error {
		calls++
		return errors.New("轮询锁不可用")
	})
	if err == nil {
		t.Fatal("全部租户失败必须把错误传回调用方")
	}
	if calls != 2 {
		t.Fatalf("任务体调用次数=%d want 2", calls)
	}
}

func TestFanoutTask_OffModeRunsOnceWithDefault(t *testing.T) {
	prev := tenant.CurrentMode()
	tenant.SetMode(tenant.ModeOff)
	t.Cleanup(func() { tenant.SetMode(prev) })

	seen := []uint{}
	if err := fanoutTask(context.Background(), func(ctx context.Context) error {
		id, _ := tenant.FromContext(ctx)
		seen = append(seen, id)
		return nil
	}); err != nil {
		t.Fatalf("fanoutTask err=%v", err)
	}
	if len(seen) != 1 || seen[0] != tenant.DefaultTenantID {
		t.Fatalf("off 模式应单默认租户一次，实际 seen=%v want [%d]", seen, tenant.DefaultTenantID)
	}
}

// 活码轮询走真实库与真实守卫：轮询出的二维码必须落在被注入的那个租户名下。
func TestLiveCodeRotateCron_RotateLandsInEachActiveTenant(t *testing.T) {
	withActiveTenants(t, 11)

	gdb := testutil.NewTestDB(t, &model.DomainPool{}, &model.LiveCode{}, &model.LiveCodeQR{})
	prevDB := pkgdb.GetDB()
	pkgdb.SetTestDB(gdb)
	t.Cleanup(func() { pkgdb.SetTestDB(prevDB) })

	ctx11 := tenant.WithTenantID(context.Background(), 11)
	domain := &model.DomainPool{Domain: "rot11." + uuid.NewString()[:8] + ".example.com", Port: 443, Status: 1}
	if err := gdb.WithContext(ctx11).Create(domain).Error; err != nil {
		t.Fatalf("seed 域名失败: %v", err)
	}
	code := &model.LiveCode{
		Name:            "轮询租户归属测试",
		ShortLink:       "rot-tenant-11",
		ShortDomainID:   domain.ID,
		EntryDomainID:   domain.ID,
		LandingDomainID: domain.ID,
		Status:          1,
	}
	if err := gdb.WithContext(ctx11).Create(code).Error; err != nil {
		t.Fatalf("seed 活码失败: %v", err)
	}

	LiveCodeRotateCron()

	var qrs []model.LiveCodeQR
	if err := gdb.WithContext(ctx11).Where("live_code_id = ?", code.ID).Find(&qrs).Error; err != nil {
		t.Fatalf("读回二维码失败（若为 [tenant] fail-closed 报错即扇出未生效）: %v", err)
	}
	if len(qrs) != 1 {
		t.Fatalf("二维码数=%d want 1（0 表示轮询查询被守卫拒绝）", len(qrs))
	}
	if qrs[0].TenantID != 11 {
		t.Errorf("二维码租户归属=%d want 11", qrs[0].TenantID)
	}
}

// 过期令牌清理走真实库与真实守卫：删除语句未绑定租户会被守卫拒绝，令牌原地不动。
func TestPasswordResetTokenCleanupCron_DeletesExpiredInEachActiveTenant(t *testing.T) {
	withActiveTenants(t, 11)

	gdb := testutil.NewTestDB(t, &model.PasswordResetToken{})
	prevDB := pkgdb.GetDB()
	pkgdb.SetTestDB(gdb)
	t.Cleanup(func() { pkgdb.SetTestDB(prevDB) })

	ctx11 := tenant.WithTenantID(context.Background(), 11)
	expired := &model.PasswordResetToken{
		UserID:    "u-rot-11",
		RawToken:  uuid.NewString(),
		ExpiresAt: time.Now().Add(-30 * 24 * time.Hour),
	}
	if err := gdb.WithContext(ctx11).Create(expired).Error; err != nil {
		t.Fatalf("seed 过期令牌失败: %v", err)
	}

	PasswordResetTokenCleanupCron()

	// 两张读数一起看才判得住"删除真发生过"：普通查询为 0 也可能是被租户条件筛掉，
	// 所以再带 deleted_at 读一次，要求行还在且作废时间已被写入。
	var visible int64
	if err := gdb.WithContext(ctx11).Model(&model.PasswordResetToken{}).
		Where("id = ?", expired.ID).Count(&visible).Error; err != nil {
		t.Fatalf("读回令牌失败（若为 [tenant] fail-closed 报错即扇出未生效）: %v", err)
	}
	if visible != 0 {
		t.Fatalf("过期令牌仍可见 count=%d want 0", visible)
	}
	var trashed []model.PasswordResetToken
	if err := gdb.WithContext(ctx11).Unscoped().Where("id = ?", expired.ID).Find(&trashed).Error; err != nil {
		t.Fatalf("带作废行读回失败: %v", err)
	}
	if len(trashed) != 1 {
		t.Fatalf("带作废行读回应为 1 条，实际 %d", len(trashed))
	}
	if trashed[0].DeletedAt.Time.IsZero() {
		t.Fatal("令牌未写入作废标记：删除语句根本没执行（守卫拒绝的典型形态）")
	}
}
