package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"hivemtk-user/internal/pkg/tenant"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func setupTenantRouter(mode tenant.Mode) (*gin.Engine, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	tenant.SetMode(mode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		// 模拟 JWT 中间件：历史令牌场景不写 tenant_id；带 claim 场景写入
		if mode == tenant.ModeOn {
			// on 模式默认模拟"历史令牌无 claim"（用例自行覆写）
			c.Set(TenantContextKey, uint(0))
		}
		c.Next()
	}, TenantMiddleware())
	r.GET("/probe", func(c *gin.Context) {
		id, ok := tenant.FromContext(c.Request.Context())
		c.JSON(http.StatusOK, gin.H{"ok": ok, "ctx_tenant": id})
	})
	w := httptest.NewRecorder()
	return r, w
}

func TestTenantMiddleware_OffModeBindsDefault(t *testing.T) {
	r, _ := setupTenantRouter(tenant.ModeOff)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"ctx_tenant":1`)
}

func TestTenantMiddleware_OnModeRejectsMissingClaim(t *testing.T) {
	r, _ := setupTenantRouter(tenant.ModeOn)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/probe", nil))
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestTenantMiddleware_OnModeBindsClaim(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(TenantContextKey, uint(42))
		c.Next()
	}, TenantMiddleware())
	r.GET("/probe", func(c *gin.Context) {
		id, ok := tenant.FromContext(c.Request.Context())
		c.JSON(http.StatusOK, gin.H{"ctx_tenant": id, "ok": ok})
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/probe", nil))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"ctx_tenant":42`)
}
