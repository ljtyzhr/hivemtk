package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
)

// GET /api/public/config 必须带 reg_mode（003 T012 的注册口径映射）。
//
// 为什么单独钉住：`scripts/api_verify_full.py` 的 B1.3 就在断言这个键，而修复前
// 实测响应体是 `{cloud_url, edition, enable_register, site_name, tenant_mode}` ——
// 没有 reg_mode。前端只能用 tenant_mode 布尔值自己推断注册口径，一旦推断与后端
// 不一致，云端实例上就会出现"离线部署"文案。
//
// 本用例走的是**配置读失败**的保守分支（测试库没建 system_config 表）：
// 两条分支同样要带这个键，保守分支是最容易漏改的那条。
func TestPublicConfig_CarriesRegMode(t *testing.T) {
	orig := tenant.CurrentMode()
	tenant.SetMode(tenant.ModeOff)
	defer tenant.SetMode(orig)

	setupTestControllerDB(t)

	authCtrl := NewAuthController()
	router := setupGinEngine()
	router.GET("/public/config", authCtrl.PublicConfig)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/public/config", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("状态码 %d，body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Edition    string `json:"edition"`
			RegMode    string `json:"reg_mode"`
			TenantMode bool   `json:"tenant_mode"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v body=%s", err, w.Body.String())
	}
	if resp.Data.RegMode == "" {
		t.Fatalf("响应缺 reg_mode：body=%s", w.Body.String())
	}
	if resp.Data.RegMode != model.TenantRegModeOffline {
		t.Errorf("TENANT_MODE=off 时 reg_mode 应为 %q，实际 %q", model.TenantRegModeOffline, resp.Data.RegMode)
	}
}
