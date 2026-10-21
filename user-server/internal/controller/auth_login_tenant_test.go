package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// TestAuthController_Login_BindsTenantToRequestContext 钉住「登录成功后 request ctx
// 必须带租户绑定」这条契约。
//
// 生产事故（2026-10-21）：登录是公开路由（无 TenantMiddleware），租户由命中的用户行
// 携带，但 bindUserTenant 只在 service 内部改了局部 ctx，不回流到 c.Request.Context()。
// 于是 controller.recordLoginRiskAsync 从 request ctx 取租户必然落空（hasTenant=false），
// 登录风控整链（login_events 写入 / 最近失败次数 / 异常登录告警）在 on 模式下被
// 隔离闸 fail-closed 静默拒绝 —— 线上日志可见「需要租户上下文但 ctx 未绑定」。
func TestAuthController_Login_BindsTenantToRequestContext(t *testing.T) {
	orig := tenant.CurrentMode()
	tenant.SetMode(tenant.ModeOn)
	defer tenant.SetMode(orig)

	setupTestControllerDB(t)
	authCtrl := NewAuthController()
	router := setupGinEngine()

	// 用一个能在 handler 之后读 request ctx 的中间件，把登录后的 ctx 抓出来。
	// 必须先于路由注册：gin 的中间件只对注册之后添加的路由生效。
	var capturedTenant uint
	var capturedHas bool
	router.Use(func(c *gin.Context) {
		c.Next()
		capturedTenant, capturedHas = tenant.FromContext(c.Request.Context())
	})
	router.POST("/login", authCtrl.Login)

	// 登录要过租户停用拦截：租户行必须存在且为活跃
	if err := db.GetDB().Create(&model.Tenant{
		ID:     77,
		Code:   "t_risk77",
		Name:   "风控测试租户",
		Status: 1,
	}).Error; err != nil {
		t.Fatalf("Create tenant failed: %v", err)
	}

	admin := &model.SystemUser{
		Username: "risk_admin",
		Password: "Admin@123",
		Email:    "risk_admin@test.com",
		Role:     "admin",
		Status:   1,
		TenantID: 77,
	}
	if err := db.GetDB().Create(admin).Error; err != nil {
		t.Fatalf("Create admin failed: %v", err)
	}

	body, _ := json.Marshal(service.LoginRequest{
		Username: "risk_admin",
		Password: "Admin@123",
	})
	req, _ := http.NewRequest("POST", "/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
	if !capturedHas {
		t.Fatal("登录后 request ctx 必须绑定租户：登录风控链（login_events 等）依赖它，" +
			"缺失会导致 on 模式下被隔离闸 fail-closed 静默拒绝")
	}
	if capturedTenant != 77 {
		t.Errorf("绑定的租户应为 77, 实际 %d", capturedTenant)
	}
}
