package middleware

import (
	"net/http"

	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/utils/response"

	"github.com/gin-gonic/gin"
)

// TenantContextKey gin context 中租户 ID 的键名（JWT 中间件写入，本中间件消费）
const TenantContextKey = "tenant_id"

// TenantMiddleware 租户上下文中间件（specs/001-saas-multi-tenant T009）。
//
// 位置：必须挂在 JWTAuthMiddleware 之后（消费其写入的 tenant_id claim）。
//
//   - off 模式（默认）：请求上下文绑定默认租户，行为等价旧版本；
//     历史 JWT（无 tenant_id claim）照常放行
//   - on 模式：tenant_id claim 缺失/为 0 ⇒ 401 fail-closed；
//     有效 claim 绑定进 request context，GORM 隔离 callback 从这里取租户
//
// 消费方式：后续链路经 c.Request.Context() 携带租户（db.WithContext 已贯通）。
func TenantMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !tenant.IsMultiTenant() {
			// 离线单机：显式绑定默认租户（与"无绑定"严格区分）
			c.Set(TenantContextKey, tenant.DefaultTenantID)
			c.Request = c.Request.WithContext(tenant.ContextWithDefault(c.Request.Context()))
			c.Next()
			return
		}

		raw, exists := c.Get(TenantContextKey)
		if !exists {
			response.Error(c, http.StatusUnauthorized, "缺少租户归属，请重新登录")
			c.Abort()
			return
		}
		id, ok := raw.(uint)
		if !ok || id == 0 {
			response.Error(c, http.StatusUnauthorized, "令牌缺少有效租户归属，请重新登录")
			c.Abort()
			return
		}
		c.Request = c.Request.WithContext(tenant.WithTenantID(c.Request.Context(), id))
		c.Next()
	}
}
