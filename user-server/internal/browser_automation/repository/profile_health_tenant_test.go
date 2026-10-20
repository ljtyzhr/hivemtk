package repository

// 契约锁（实体审查 #9，TENANT_MODE=on）：熔断行是「每租户每平台」一行，不是「每平台」一行。
//
// 这张表的形状在 off 模式与 on 模式下不是同一件事，而它两边都「看起来正常」：
//   - 键位：模型给 `platform` 建的是**单列** uniqueIndex（`model/profile_health.go:18`），
//     on 模式启动时由 tenancyidx.Normalize 复合化成 (tenant_id, platform)。键位一旦退回单列，
//     症状不是报错而是闸门静默失效：第二个租户 MarkBlocked 撞 23505，而写侧只 Errorf 不判红
//     （`service/profile_fuse.go:25`），读侧又因为「B 永远建不出自己的行」而永远读到 not found
//     ——熔断对非默认租户等于不存在，风控特色整条形同虚设。
//   - 谓词：`GetByPlatform` 只按 platform 查、`MarkRecovered` 只按 platform 清，两处的租户遮挡
//     全在隔离 callback 上。遮挡失效时 A 能读到 B 的熔断原因（把别人的风控结论读进自己的
//     错误消息），或一次「恢复」把别的租户的闸门一起抬起。
//
// 种子刻意用**同一个** platform 常量：这张表的业务键就是平台名，不同平台名断言不出键位形状。

import (
	"context"
	"testing"

	"hivemtk-user/internal/browser_automation/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	profileTenantA = uint(18)
	profileTenantB = uint(39)
	// 平台名与去重/回执那两组测试的 "xiaohongshu" 同值不冲突：它们各自的表，行号互不引用。
	profilePlatformA = "xiaohongshu"
	profileReasonA   = "A 的风控结论：验证码升级"
	profileReasonB   = "B 的风控结论：设备指纹失效"
)

func profileHealthDB(t *testing.T) (*gorm.DB, context.Context, context.Context, BrowserProfileHealthRepository) {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })
	db := testutil.NewTestDB(t, &model.BrowserProfileHealth{})
	if db == nil {
		t.Fatal("测试库不可达：熔断行的键位形状判不了（不 Skip，跳过等于没锁）")
	}
	return db,
		tenant.WithTenantID(context.Background(), profileTenantA),
		tenant.WithTenantID(context.Background(), profileTenantB),
		NewBrowserProfileHealthRepositoryWithDB(db)
}

// profilePhysicalRows 全租户物理行数（system scope）：本表按平台唯一，
// 租户内计数看不出「键位不含 tenant」，只有跨租户总量才暴露它。
func profilePhysicalRows(t *testing.T, db *gorm.DB, platform string) int64 {
	t.Helper()
	var n int64
	ctx := tenant.WithSystemScope(context.Background())
	if err := db.WithContext(ctx).Model(&model.BrowserProfileHealth{}).
		Where("platform = ?", platform).Count(&n).Error; err != nil {
		t.Fatalf("统计物理熔断行失败: %v", err)
	}
	return n
}

// TestProfileHealthFuseIsPerTenant 同平台名在两租户各成立一行，且各自只读得到自己的结论。
func TestProfileHealthFuseIsPerTenant(t *testing.T) {
	db, ctxA, ctxB, repo := profileHealthDB(t)

	if err := repo.MarkBlocked(ctxA, profilePlatformA, profileReasonA); err != nil {
		t.Fatalf("租户 %d 落熔断失败: %v", profileTenantA, err)
	}

	// B 必须读不到 A 的熔断行：读到 = A 的风控结论进 B 的错误消息，
	// 且 B 的新任务会被 A 的封号拦死（跨租户拒绝服务）。
	if h, err := repo.GetByPlatform(ctxB, profilePlatformA); err != gorm.ErrRecordNotFound {
		if h != nil {
			t.Fatalf("租户 %d 读到了别人的熔断行（reason=%q）", profileTenantB, h.Reason)
		}
		t.Fatalf("租户 %d 未命中应返回 ErrRecordNotFound，实得 err=%v", profileTenantB, err)
	}

	// B 落自己的那行：键位不含 tenant 时这一步就是 23505，
	// 而生产写侧只记日志不判红 ⇒ 熔断对 B 静默失效，库面上看不出任何异常。
	if err := repo.MarkBlocked(ctxB, profilePlatformA, profileReasonB); err != nil {
		t.Fatalf("租户 %d 落自己的熔断行失败（键位大概率不含 tenant_id）: %v", profileTenantB, err)
	}
	if n := profilePhysicalRows(t, db, profilePlatformA); n != 2 {
		t.Errorf("同平台物理熔断行 got=%d want=2（每平台一行 ≠ 每租户每平台一行）", n)
	}

	for _, tc := range []struct {
		ctx      context.Context
		tenantID uint
		reason   string
	}{
		{ctxA, profileTenantA, profileReasonA},
		{ctxB, profileTenantB, profileReasonB},
	} {
		h, err := repo.GetByPlatform(tc.ctx, profilePlatformA)
		if err != nil {
			t.Fatalf("租户 %d 读回熔断行失败: %v", tc.tenantID, err)
		}
		if !h.Blocked || h.Reason != tc.reason || h.BlockedAt == nil {
			t.Errorf("租户 %d 读到的行 got=(blocked=%v reason=%q at=%v) 不是自己那条",
				tc.tenantID, h.Blocked, h.Reason, h.BlockedAt)
		}
		if h.TenantID != tc.tenantID {
			t.Errorf("租户 %d 读到的行 tenant_id=%d", tc.tenantID, h.TenantID)
		}
	}
}

// TestProfileHealthRecoverStaysInTenant A 的人工恢复只抬起 A 的闸门。
// 第二次 MarkBlocked 走的是 Save 分支（行已存在），这条腿顺带钉住「Save 带着整行回写
// tenant_id」不会被租户守卫改成 0——那会让行掉出本租户，比跨租户更难发现。
func TestProfileHealthRecoverStaysInTenant(t *testing.T) {
	db, ctxA, ctxB, repo := profileHealthDB(t)
	if err := repo.MarkBlocked(ctxA, profilePlatformA, profileReasonA); err != nil {
		t.Fatalf("A 落熔断失败: %v", err)
	}
	if err := repo.MarkBlocked(ctxB, profilePlatformA, profileReasonB); err != nil {
		t.Fatalf("B 落熔断失败: %v", err)
	}
	// A 再熔断一次（同租户同平台 ⇒ Save 分支）：物理行数不得增长。
	if err := repo.MarkBlocked(ctxA, profilePlatformA, "A 的第二次熔断"); err != nil {
		t.Fatalf("A 二次熔断（Save 分支）失败: %v", err)
	}
	if n := profilePhysicalRows(t, db, profilePlatformA); n != 2 {
		t.Errorf("A 重复熔断后物理行数 got=%d want=2（Save 变成 INSERT 就是键位又建了行）", n)
	}

	if err := repo.MarkRecovered(ctxA, profilePlatformA); err != nil {
		t.Fatalf("A 人工恢复失败: %v", err)
	}
	hA, err := repo.GetByPlatform(ctxA, profilePlatformA)
	if err != nil {
		t.Fatalf("A 恢复后读回失败: %v", err)
	}
	if hA.Blocked || hA.BlockedAt != nil || hA.Reason != "" {
		t.Errorf("A 恢复不彻底 got=(blocked=%v at=%v reason=%q)", hA.Blocked, hA.BlockedAt, hA.Reason)
	}
	if hA.TenantID != profileTenantA {
		t.Errorf("A 恢复后行的 tenant_id=%d want=%d：恢复把租户列改写了，行会掉出本租户", hA.TenantID, profileTenantA)
	}

	hB, err := repo.GetByPlatform(ctxB, profilePlatformA)
	if err != nil {
		t.Fatalf("B 读回失败: %v", err)
	}
	if !hB.Blocked || hB.Reason != profileReasonB {
		t.Errorf("B 的闸门被别人的恢复抬起 got=(blocked=%v reason=%q)", hB.Blocked, hB.Reason)
	}
}

// TestProfileHealthRejectsUnboundContext 三个方法在裸 ctx 上一律 fail-closed。
// 读侧放行 = 全平台熔断面跨租户串读；写侧放行 = 熔断行落进默认租户名下
// （列默认值 1），发起租户从此既看不到也恢复不了自己的闸门。
func TestProfileHealthRejectsUnboundContext(t *testing.T) {
	db, ctxA, _, repo := profileHealthDB(t)
	if err := repo.MarkBlocked(ctxA, profilePlatformA, profileReasonA); err != nil {
		t.Fatalf("A 落熔断失败: %v", err)
	}
	bare := context.Background()
	if _, err := repo.GetByPlatform(bare, profilePlatformA); err == nil {
		t.Error("裸 ctx 的 GetByPlatform 未被拒绝")
	}
	if err := repo.MarkBlocked(bare, "douyin", "无租户熔断"); err == nil {
		t.Error("裸 ctx 的 MarkBlocked 未被拒绝")
	}
	if err := repo.MarkRecovered(bare, profilePlatformA); err == nil {
		t.Error("裸 ctx 的 MarkRecovered 未被拒绝")
	}
	// 被拒的写入不能留下任何行（douyin 那条尤其：它会把熔断挂到默认租户名下）。
	if n := profilePhysicalRows(t, db, "douyin"); n != 0 {
		t.Errorf("裸 ctx 的 MarkBlocked 仍落了 %d 行", n)
	}
	h, err := repo.GetByPlatform(ctxA, profilePlatformA)
	if err != nil || !h.Blocked {
		t.Errorf("裸 ctx 的操作改变了 A 的行 got=(blocked=%v err=%v)", h != nil && h.Blocked, err)
	}
}
