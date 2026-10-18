package service

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/platform"
	"hivemtk-user/internal/system/install"
)

// TestOfflineRegistrationWritesTenantRow 离线部署的负责人初始化必须落一条租户登记。
//
// 需求原文：「注意离线独立部署情况，**也要有负责人初始化**，也要收集信息」。
//
// 这一条是整轮审核里最容易漏的地方：注册页有公司名/负责人/电话/邮箱（做了），
// 而**离线用户根本不走注册** —— 他们装完系统看到的是 /setup 初始化向导。
// 那条路径当时把四项全标「选填」，于是需求唯一点名的人群，
// 恰恰是唯一完全绕过监管信息收集的人群。
//
// 判据放在"租户行真的写出来了"上，而不是"校验函数返回 nil"：
// 校验通过但没落库，等于留痕不存在。
func TestOfflineRegistrationWritesTenantRow(t *testing.T) {
	database := testutil.NewTestDB(t,
		&model.Tenant{}, &model.SystemUser{}, &model.TenantModule{},
		&model.ModuleSyncLog{}, &model.ModuleOrder{},
	)
	db.SetTestDB(database)

	// 清空，模拟全新安装
	for _, m := range []any{&model.Tenant{}, &model.SystemUser{}} {
		if err := database.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(m).Error; err != nil {
			t.Fatalf("清空 %T 失败: %v", m, err)
		}
	}

	ctx := context.Background()
	svc := NewAuthService()

	// 1) 四项缺一必被拒
	cases := []struct {
		name, company, person, phone, email string
		ack                                 bool
	}{
		{"缺公司名", "", "李四", "13900001111", "b@e.com", true},
		{"缺负责人", "某某科技", "", "13900001111", "b@e.com", true},
		{"缺电话", "某某科技", "李四", "", "b@e.com", true},
		{"缺邮箱", "某某科技", "李四", "13900001111", "", true},
		{"未确认告知", "某某科技", "李四", "13900001111", "b@e.com", false},
	}
	for _, c := range cases {
		if err := svc.ValidateOfflineRegistration(c.company, c.person, c.phone, c.email, c.ack); err == nil {
			t.Errorf("%s 应被拒", c.name)
		}
	}

	// 2) 完整信息应通过校验
	if err := svc.ValidateOfflineRegistration("某某科技有限公司", "李四", "13900001111", "boss@example.com", true); err != nil {
		t.Fatalf("完整信息应通过: %v", err)
	}

	// 3) 真正调用初始化，断言租户行落库
	err := svc.InitAdminWithRegistration(ctx,
		"offline_admin", "Admin@123456", "boss@example.com",
		"李四", "13900001111", "某某科技有限公司",
		"91330100MA2XXXXX1A", "杭州市", true, "2026-10-17.v1")
	if err != nil {
		t.Fatalf("离线初始化失败: %v", err)
	}

	var tn model.Tenant
	if err := database.Where("company_name = ?", "某某科技有限公司").First(&tn).Error; err != nil {
		t.Fatalf("未落租户登记行：%v（留痕不存在 = 需求未满足）", err)
	}
	if tn.LegalPerson != "李四" {
		t.Errorf("负责人姓名应为「李四」，实际 %q", tn.LegalPerson)
	}
	if tn.Phone != "13900001111" {
		t.Errorf("电话应为 13900001111，实际 %q", tn.Phone)
	}
	if tn.RegMode != model.TenantRegModeOffline {
		t.Errorf("reg_mode 应为 offline，实际 %q", tn.RegMode)
	}
	if tn.PlatformSync != model.TenantSyncSkipped {
		t.Errorf("离线部署的 platform_sync 应为 skipped（不是 failed —— "+
			"离线本来就没打算上报，标成失败会在平台端堆一批无法处理的红单），实际 %q", tn.PlatformSync)
	}
	if !tn.RegulatoryAck {
		t.Error("监管确认位必须为 true")
	}
	if tn.AgreementVersion == "" {
		t.Error("协议版本不能为空：它是「用户同意的是哪一版」的唯一凭据")
	}

	// 4) 负责人信息应回填到超管本人（登录后自己的资料里就能看到）
	var u model.SystemUser
	if err := database.Where("username = ?", "offline_admin").First(&u).Error; err != nil {
		t.Fatalf("超管未创建: %v", err)
	}
	if u.RealName != "李四" {
		t.Errorf("超管 real_name 应回填为「李四」，实际 %q", u.RealName)
	}
	if u.Phone != "13900001111" {
		t.Errorf("超管 phone 应回填，实际 %q", u.Phone)
	}
}

// TestRegistrationPayloadKeepsTwoIdentitiesDistinct 开户登记里两个标识必须各归其位。
//
// 事故形态（本轮发现）：两处上报都把 install_id 填成 merchant_key，
// 于是平台端 tenant_registrations.install_id 存的是商户密钥，
// 而 platform_installs 用的是真正 install.lock 里的 install_id ——
// 两张表之间**没有可连接的键**，"这个部署是谁 / 它在线吗 / 它登记了吗"
// 这三个问题在平台上答不出来。
//
// 这类混填在界面上、在单测里都看不出来：字段有值、请求成功、登记也有记录，
// 只是那个值对不上任何东西。所以判据必须是"两者不同且各自来自正确来源"。
func TestRegistrationPayloadKeepsTwoIdentitiesDistinct(t *testing.T) {
	lr, err := install.Load()
	if err != nil {
		t.Skipf("读不到 install.lock（%v），跳过", err)
	}
	if lr == nil || lr.InstallID == "" {
		t.Skip("install.lock 无 install_id，跳过")
	}

	got := currentInstallID()
	if got != lr.InstallID {
		t.Errorf("currentInstallID() 应返回 install.lock 的 install_id（%q），实际 %q",
			lr.InstallID, got)
	}

	// 关键断言：安装标识不得等于商户密钥。
	// 测试环境里 merchant_key 可能为空（未 InitSync），此时只验 install_id 有值。
	mk := platform.GetMerchantKey()
	if mk != "" && got == mk {
		t.Fatalf("install_id 与 merchant_key 相同（%q）：两个标识被混填，"+
			"平台端将无法把开户登记与心跳记录关联起来", got)
	}
	if got == "" {
		t.Error("install_id 不应为空：空值让平台端无法关联，且比错值更难排查")
	}
}
