package service

// on 模式初始化向导回归（P0：TENANT_MODE=on 下 POST /api/system/init-admin 400）。
//
// 根因：public 组没有 TenantMiddleware（见 router.go 的 public 分组），初始化时
// ctx 无租户绑定 → system_users 的 INSERT 撞 fillCreateForTenant 的 fail-closed →
// "创建超管失败: [tenant] 表 system_users 需要租户上下文…"。
//
// 修复形态（与 SaaS 开户 CreateWithAdmin 同款）：先落 tenants 行拿到真实租户 ID，
// 再把 ID 显式写进超管行 —— 非零显式值走"行自证"放行路径，不依赖 ctx 绑定。
//
// 判据钉在"落库形态"上而不是"没报错"：超管 TenantID 必须非 0 且**等于**本次
// 初始化新建的那条 tenants 行的 ID（不是碰巧等于默认租户 1 就算过）。

import (
	"context"
	"testing"

	"gorm.io/gorm"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/tenant"
)

// TestInitAdminWithRegistrationOnModeBindsTenant on 模式初始化：超管必须挂在新租户上
func TestInitAdminWithRegistrationOnModeBindsTenant(t *testing.T) {
	withTenantMode(t, tenant.ModeOn)
	setupTenantTestDB(t) // Drop+Migrate Tenant/SystemUser；同时注册隔离 callback 与租户登记表
	pinFreshLockDir(t)   // 全新安装态（锁进临时目录，不污染 TestMain 的共享 lock）

	// 与 public 组一致：裸请求 ctx，无租户绑定
	ctx := context.Background()
	database := db.GetDB()
	svc := NewAuthService()

	// 前置自证：本连接上的隔离 callback 必须真的在拦（无租户绑定的写 fail-closed）。
	// 若这里能写进去，说明登记表/callback 没生效 —— 本用例会在修复前也绿 = 假绿。
	probe := &model.SystemUser{Username: "probe_should_fail_closed", Password: "x",
		Role: model.SystemUserRoleAdmin, Status: 1}
	if err := database.WithContext(ctx).Create(probe).Error; err == nil {
		t.Fatal("无租户绑定的 system_users 写入未被 fail-closed：on 模式隔离未生效，本用例测不到 P0")
	}

	if err := svc.InitAdminWithRegistration(ctx,
		"onmode_admin", "Admin@123456", "onmode@example.com",
		"张三", "13800000000", "某某科技有限公司",
		"91330100MA2XXXXX1A", "杭州市", true, "2026-10-17.v1",
		"198.51.100.7", "OnModeTest/1.0"); err != nil {
		t.Fatalf("on 模式初始化应成功（修复前此处 fail-closed，控制层 400）: %v", err)
	}

	// 读取口径：系统入口（无租户上下文）→ SystemScope
	sysCtx := tenant.WithSystemScope(ctx)

	var admin model.SystemUser
	if err := database.WithContext(sysCtx).
		Where("username = ?", "onmode_admin").First(&admin).Error; err != nil {
		t.Fatalf("超管未落库: %v", err)
	}
	if admin.TenantID == 0 {
		t.Fatal("超管 TenantID=0：on 模式下行没有租户归属，后续所有按租户隔离的写入都会落空")
	}

	// admin.TenantID 必须指向一条**真实存在**的 tenants 行（= 本次初始化建的登记行）
	var tn model.Tenant
	if err := database.WithContext(sysCtx).First(&tn, admin.TenantID).Error; err != nil {
		t.Fatalf("admin.TenantID=%d 找不到对应 tenants 行（应等于初始化时新建的那条登记行）: %v",
			admin.TenantID, err)
	}
	// 003 T012：on 模式的 /setup 初始化是云端 SaaS 形态开户，reg_mode 实报 saas
	if tn.RegMode != model.TenantRegModeSaaS || tn.CompanyName != "某某科技有限公司" {
		t.Errorf("租户登记行内容不符: reg_mode=%q company=%q（on 模式应为 saas）", tn.RegMode, tn.CompanyName)
	}
	// 003 T010：静默采集列在 on 模式初始化路径同样落库
	if tn.InitIP != "198.51.100.7" || tn.InitUA == "" || tn.InitDeviceFingerprint == "" {
		t.Errorf("静默采集三列未落库: init_ip=%q init_ua=%q fp=%q", tn.InitIP, tn.InitUA, tn.InitDeviceFingerprint)
	}

	// 顺序重构不得重复建登记行：本次初始化恰好一条
	var n int64
	if err := database.WithContext(sysCtx).Model(&model.Tenant{}).Count(&n).Error; err != nil {
		t.Fatalf("统计租户登记行失败: %v", err)
	}
	if n != 1 {
		t.Errorf("应恰好 1 条租户登记行，实际 %d（顺序重构引入了重复建行）", n)
	}

	// 负责人信息仍应回填到超管本人（离线登记需求 L10）
	if admin.RealName != "张三" || admin.Phone != "13800000000" {
		t.Errorf("负责人回填缺失: real_name=%q phone=%q", admin.RealName, admin.Phone)
	}

	// 收尾：清掉本用例落的行。显式 SystemScope：此刻 mode 仍是 on
	// （withTenantMode 的还原在 LIFO 之后才跑），无绑定删除会被 fail-closed。
	t.Cleanup(func() {
		for _, m := range []any{&model.Tenant{}, &model.SystemUser{}} {
			if err := database.Session(&gorm.Session{AllowGlobalUpdate: true}).
				WithContext(tenant.WithSystemScope(context.Background())).
				Delete(m).Error; err != nil {
				t.Logf("清理 %T 失败: %v", m, err)
			}
		}
	})
}
