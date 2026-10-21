package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"hivemtk-user/internal/middleware"
	"hivemtk-user/internal/pkg/tenant"

	"github.com/gin-gonic/gin"
)

// TestTenantBindIfAuthenticated_BindsTenantWhenLoggedIn 钉住「公开端点登录则绑租户」。
//
// 线上缺陷（2026-10-22）：功能模块市场挂在 public 组（未登录可逛），
// 不挂 TenantMiddleware ⇒ request ctx 无租户 ⇒ ownedRows 拿不到租户直接返回空
// ⇒ 市场页对所有人都不显示「已拥有」标记（已付费的模块看着像没买）。
func TestTenantBindIfAuthenticated_BindsTenantWhenLoggedIn(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// 模拟 OptionalAuthMiddleware 已把租户 ID 写进 gin ctx
	r.Use(func(c *gin.Context) {
		c.Set(middleware.TenantContextKey, uint(42))
		c.Next()
	})
	r.Use(middleware.TenantBindIfAuthenticated())
	var gotID uint
	var gotOK bool
	r.GET("/market", func(c *gin.Context) {
		gotID, gotOK = tenant.FromContext(c.Request.Context())
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/market", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if !gotOK {
		t.Fatal("已登录请求必须在 request ctx 里绑定租户：否则公开市场接口拿不到租户，owned 判定恒空")
	}
	if gotID != 42 {
		t.Errorf("绑定的租户应为 42，实际 %d", gotID)
	}
}

// TestTenantBindIfAuthenticated_AnonymousStaysUnbound 钉住反向：匿名请求不绑租户
// （公开橱窗档不能因为中间件而拒绝匿名浏览）。
func TestTenantBindIfAuthenticated_AnonymousStaysUnbound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.TenantBindIfAuthenticated())
	var gotOK bool
	r.GET("/market", func(c *gin.Context) {
		_, gotOK = tenant.FromContext(c.Request.Context())
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/market", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("匿名请求应放行，实际 %d", w.Code)
	}
	if gotOK {
		t.Error("匿名请求不应绑定租户")
	}
}
