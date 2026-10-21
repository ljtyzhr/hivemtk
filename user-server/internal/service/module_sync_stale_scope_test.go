package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"hivemtk-user/internal/config"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

// staleSyncStub 造一个"商户级"平台：active_keys 只含基础模块，不含付费模块。
//
// 这正是线上后台周期 runner 看到的形态 —— 它没有租户绑定，sync 请求带
// ?tenant_code=（空串），平台按商户级行下发名单。
func setupStaleScopeStub(t *testing.T) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/merchant-api/modules/sync":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "msg": "", "data": map[string]any{
				"merchant_key": "MKE2E",
				"active_keys":  []string{"core"},
				"menu_keys":    []string{},
				"catalog": []map[string]any{
					{"module_key": "core", "name": "core", "category": "base",
						"owned": true, "owned_status": "active", "owned_source": "default_base", "menu_keys": []string{}},
				},
			}})
		case "/merchant-api/orders":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "msg": "", "data": map[string]any{
				"total": 0, "list": []map[string]any{},
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	prev := config.PlatformCfg
	config.PlatformCfg = &config.PlatformConfig{APIURL: srv.URL, Secret: "test-secret"}
	t.Cleanup(func() { config.PlatformCfg = prev })
}

func setupStaleScopeDB(t *testing.T) *gorm.DB {
	t.Helper()
	database := testutil.NewTestDB(t,
		&model.Tenant{}, &model.TenantModule{}, &model.ModuleSyncLog{}, &model.ModuleOrder{})
	db.SetTestDB(database)
	t.Cleanup(func() {
		sysCtx := tenant.WithSystemScope(context.Background())
		for _, m := range []any{&model.Tenant{}, &model.TenantModule{}, &model.ModuleSyncLog{}, &model.ModuleOrder{}} {
			if err := database.Session(&gorm.Session{AllowGlobalUpdate: true}).WithContext(sysCtx).Delete(m).Error; err != nil {
				t.Logf("清理 %T 失败: %v", m, err)
			}
		}
	})
	return database
}

// TestSync_MerchantLevelRound_MustNotDisableTenantPaidModules 钉住「商户级那轮
// sync 不得把租户级已开通的付费模块标成 disabled」。
//
// 线上症状（2026-10-22）：m1 买过 reach-auto 并已开通（平台端 t578fub 名下
// reach-auto=active），用户端「我的模块」一度显示已开通，但市场页却显示未拥有。
// 根因：后台周期 runner 没有租户绑定 ⇒ sync 请求带 tenant_code=（空串）⇒ 平台
// 按**商户级** active_keys 下发（不含 reach-auto）⇒ 用户端 stale 判定
// （"本地存在但平台不再下发 ⇒ 标 disabled"）拿这份**商户级**名单去遍历
// **全租户**的行 ⇒ 把租户级已开通的付费模块误标为 disabled。
//
// 正确的判据：stale 判定只在**本轮同步覆盖到的租户**范围内做；商户级那轮
// 无权判定任何租户行的存废。
func TestSync_MerchantLevelRound_MustNotDisableTenantPaidModules(t *testing.T) {
	withTenantMode(t, tenant.ModeOn)
	t.Setenv("PLATFORM_ENABLED", "true")
	t.Setenv("PLATFORM_MERCHANT_KEY", "MKE2E")

	database := setupStaleScopeDB(t)
	setupStaleScopeStub(t)

	tn := &model.Tenant{Code: "tSTALE1", Name: "租户一"}
	if err := database.Create(tn).Error; err != nil {
		t.Fatalf("写入租户行: %v", err)
	}
	other := &model.Tenant{Code: "tSTALE2", Name: "租户二"}
	if err := database.Create(other).Error; err != nil {
		t.Fatalf("写入租户行: %v", err)
	}

	// 租户一已开通一个付费模块（平台端该租户名下有这行）
	tenantCtx := tenant.OnlyTenant(context.Background(), tn.ID)
	if err := database.WithContext(tenantCtx).Create(&model.TenantModule{
		ModuleKey:  "reach-auto",
		Status:     model.TenantModuleActive,
		Source:     model.TenantModuleSourcePurchase,
		PriceCents: 100,
	}).Error; err != nil {
		t.Fatalf("写入租户一已开通的付费模块: %v", err)
	}

	// 后台周期 runner：无租户绑定（=商户级那轮）
	svc := NewModuleCatalogService()
	if _, err := svc.Sync(context.Background()); err != nil {
		t.Fatalf("商户级 Sync 失败: %v", err)
	}

	var got model.TenantModule
	if err := database.WithContext(tenantCtx).
		Where("module_key = ?", "reach-auto").First(&got).Error; err != nil {
		t.Fatalf("回查租户一的模块行失败: %v", err)
	}
	if got.Status != model.TenantModuleActive {
		t.Errorf("商户级那轮 sync 不得把租户级已开通的付费模块标成 %s（实际 %s）："+
			"stale 判据必须按租户作用域，不能用商户级 active_keys 遍历全租户的行",
			model.TenantModuleDisabled, got.Status)
	}
}

// TestSync_TenantRound_StillDisablesRevokedModule 正向用例：撤销语义不能被
// 上面那条「商户级无权判定」改没了 —— 租户自己的那轮同步里，平台不再下发
// 的模块仍必须被显式失效（否则「撤销」永远不生效）。
//
// 判据的完整形状：**按租户作用域**。商户级那轮不判；租户级那轮只判该租户。
func TestSync_TenantRound_StillDisablesRevokedModule(t *testing.T) {
	withTenantMode(t, tenant.ModeOn)
	t.Setenv("PLATFORM_ENABLED", "true")
	t.Setenv("PLATFORM_MERCHANT_KEY", "MKE2E")

	database := setupStaleScopeDB(t)
	setupTenantScopeStub(t, []string{"core"}) // 平台只下发 core

	tn := &model.Tenant{Code: "tSTALE3", Name: "租户三"}
	if err := database.Create(tn).Error; err != nil {
		t.Fatalf("写入租户行: %v", err)
	}
	tenantCtx := tenant.OnlyTenant(context.Background(), tn.ID)
	if err := database.WithContext(tenantCtx).Create(&model.TenantModule{
		ModuleKey:  "reach-auto",
		Status:     model.TenantModuleActive,
		Source:     model.TenantModuleSourcePurchase,
		PriceCents: 100,
	}).Error; err != nil {
		t.Fatalf("写入租户三已开通的付费模块: %v", err)
	}

	svc := NewModuleCatalogService()
	if _, err := svc.Sync(tenantCtx); err != nil {
		t.Fatalf("租户级 Sync 失败: %v", err)
	}

	var got model.TenantModule
	if err := database.WithContext(tenantCtx).
		Where("module_key = ?", "reach-auto").First(&got).Error; err != nil {
		t.Fatalf("回查失败: %v", err)
	}
	if got.Status != model.TenantModuleDisabled {
		t.Errorf("平台已不再下发的模块应被显式失效（%s），实际 %s：撤销语义不能被"+
			"「商户级无权判定」这条改没了", model.TenantModuleDisabled, got.Status)
	}
}

// setupTenantScopeStub 造一个按租户下发的平台桩（active_keys 可配）。
func setupTenantScopeStub(t *testing.T, activeKeys []string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/merchant-api/modules/sync":
			catalog := []map[string]any{}
			for _, k := range activeKeys {
				catalog = append(catalog, map[string]any{
					"module_key": k, "name": k, "category": "base",
					"owned": true, "owned_status": "active", "owned_source": "default_base",
					"menu_keys": []string{},
				})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "msg": "", "data": map[string]any{
				"merchant_key": "MKE2E",
				"active_keys":  activeKeys,
				"menu_keys":    []string{},
				"catalog":      catalog,
			}})
		case "/merchant-api/orders":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "msg": "", "data": map[string]any{
				"total": 0, "list": []map[string]any{},
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	prev := config.PlatformCfg
	config.PlatformCfg = &config.PlatformConfig{APIURL: srv.URL, Secret: "test-secret"}
	t.Cleanup(func() { config.PlatformCfg = prev })
}
