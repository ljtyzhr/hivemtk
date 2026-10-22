package service

import (
	"context"
	"strings"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"

	"gorm.io/gorm"
)

// 平台级参数的「写平台默认值」通道。
//
// 背景：SaaS 模式下 UpdateValue 见到请求 ctx 带着租户绑定，就一律写
// tenant_config_params 覆盖行 —— 这对「按租户分叉的参数」是对的，但对
// **平台级参数**（platform.api_host：这台部署连哪个平台，是部署级事实）
// 就成了死路：平台运营方自己也是一个租户，于是「改这台实例连哪个平台」
// 这件事没有任何人能做。
//
// UpdatePlatformDefault 就是补这条通道。判据放在 service 层而不是 controller 层：
// 「这一键是不是平台级」是 config_param_tenant_scope.go 里的事实，
// 放 service ⇒ 任何调用方都绕不过去。

// seedPlatformHostParam 种一条 string 类型的 platform.api_host。
func seedPlatformHostParam(t *testing.T, db *gorm.DB, def string) {
	t.Helper()
	if err := db.Create(&model.ConfigParam{
		Group:        "platform",
		Key:          "api_host",
		Name:         "平台端 API 地址",
		Description:  "平台端 API 地址",
		ValueType:    "string",
		Value:        def,
		DefaultValue: def,
	}).Error; err != nil {
		t.Fatalf("seed platform.api_host 失败: %v", err)
	}
}

// TestUpdatePlatformDefaultWritesBaseRowInSaaSMode 是本组的主判据：
// 带租户 ctx 的写入必须落在**平台默认行**上，不是租户覆盖行。
func TestUpdatePlatformDefaultWritesBaseRowInSaaSMode(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })
	svc, db := setupTenantParamFixture(t)
	seedPlatformHostParam(t, db, "http://old.example.com")

	ctxA := tenantCtx(41)
	if err := svc.UpdatePlatformDefault(ctxA, "platform", "api_host", "https://new.example.com", 7); err != nil {
		t.Fatalf("平台默认值写入被拒: %v", err)
	}

	// 平台默认行必须是新值
	var base model.ConfigParam
	if err := db.Where("param_group = ? AND key = ?", "platform", "api_host").First(&base).Error; err != nil {
		t.Fatal(err)
	}
	if base.Value != "https://new.example.com" {
		t.Fatalf("平台默认行应写入新值，实际=%q", base.Value)
	}

	// 且**不得**留下租户覆盖行：覆盖行对部署级参数无意义，
	// 留着只会让"参数页显示的值"与"实际生效的值"对不上。
	var n int64
	if err := db.WithContext(ctxA).Model(&model.TenantConfigParam{}).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("平台默认值写入不得留下租户覆盖行，实际 %d 行", n)
	}
}

// TestUpdatePlatformDefaultRejectsTenantScopedParams 钉住守卫方向：
// 这条通道**只**放行平台级参数。若让它能写任意键，租户就能通过它改写
// 全体租户的默认值 —— 那正是 UpdateValue 刻意不让人做的事。
func TestUpdatePlatformDefaultRejectsTenantScopedParams(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })
	svc, db := setupTenantParamFixture(t)
	seedBaseParam(t, db, "limits", "max_items", "10")

	err := svc.UpdatePlatformDefault(tenantCtx(41), "limits", "max_items", "999", 7)
	if err == nil {
		t.Fatal("非平台级参数必须被平台默认值通道拒下")
	}
	if !strings.Contains(err.Error(), "不是平台级参数") {
		t.Fatalf("报错必须说清原因，实际=%v", err)
	}

	// 值必须没变
	var row model.ConfigParam
	if err := db.Where("param_group = ? AND key = ?", "limits", "max_items").First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.Value != "10" {
		t.Fatalf("被拒的写入不得改值，实际=%q", row.Value)
	}
}

// TestUpdatePlatformDefaultRejectsUnknownParam 未知键必须报错而不是静默建行。
func TestUpdatePlatformDefaultRejectsUnknownParam(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })
	svc, db := setupTenantParamFixture(t)
	_ = db

	if err := svc.UpdatePlatformDefault(context.Background(), "platform", "not_exist", "x", 1); err == nil {
		t.Fatal("未知键必须报错")
	}
}

// TestPlatformHostIsPlatformScoped 钉住 platform.api_host 的档位判定。
//
// 它必须落在 platformScopedParams（平台级）而不是默认档：
// 默认档意味着租户能写覆盖行，而覆盖行对这个键不生效 ——
// 页面回「保存成功」而地址没变。
func TestPlatformHostIsPlatformScoped(t *testing.T) {
	scope, reason := paramTenantScope("platform", "api_host")
	if scope != tenantScopePlatform {
		t.Fatalf("platform.api_host 档位应为 %s，实际=%q", tenantScopePlatform, scope)
	}
	if strings.TrimSpace(reason) == "" {
		t.Fatal("平台级档位必须带原因（管理端要把它显示给用户）")
	}
}

// TestUpdatePlatformDefaultInvalidatesTenantCache 平台默认值改了之后，
// 各租户的缓存必须一起失效。
//
// 不这么做会出现「平台默认改了但某租户还读到旧值」—— 租户覆盖行里可能留着
// 旧值的拷贝，缓存不失效就等于旧值还在生效。
func TestUpdatePlatformDefaultInvalidatesTenantCache(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })
	svc, db := setupTenantParamFixture(t)
	seedPlatformHostParam(t, db, "http://old.example.com")

	ctxA := tenantCtx(41)
	// 先读一次，把值灌进缓存
	if got := svc.GetString(ctxA, "platform", "api_host", ""); got != "http://old.example.com" {
		t.Fatalf("首次读取应得默认值，实际=%q", got)
	}
	// 写平台默认值
	if err := svc.UpdatePlatformDefault(ctxA, "platform", "api_host", "https://new.example.com", 7); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	// 再读必须拿到新值
	if got := svc.GetString(ctxA, "platform", "api_host", ""); got != "https://new.example.com" {
		t.Fatalf("平台默认值改动后读取应得新值，实际=%q（缓存没失效）", got)
	}
}
