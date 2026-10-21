package middleware

import (
	"hivemtk-user/internal/pkg/tenant"

	"github.com/gin-gonic/gin"
)

// TenantBindIfAuthenticated 公开端点的「登录则绑租户」中间件。
//
// 背景（2026-10-22 线上实测）：功能模块市场是**公开橱窗档**（未登录也能逛，
// 见 module_routes.go 的 public 组注释），所以它不挂 TenantMiddleware ——
// 请求 ctx 里没有租户。于是：
//
//   - on 模式下 ownedRows（module_market.go）拿不到租户 ⇒ 直接返回空 map
//     ⇒ **市场页对所有人都不显示「已拥有」标记**，已付费的模块看着像没买；
//   - 公开档又不该强制登录（未登录浏览是获客面）。
//
// 本中间件的语义：带合法 JWT ⇒ 按令牌里的租户绑定 ctx；无 JWT / 令牌无效
// ⇒ 退化为匿名（不绑租户），公开浏览不受影响。
//
// 与 OptionalAuthMiddleware 的分工：那个只把租户 ID 写进 gin ctx
// （c.Set(TenantContextKey, ...)），不绑 request ctx —— GORM 的 tenancyidx
// 隔离回调读的是 request ctx。本中间件负责补上最后那一环。
//
// 用法：公开组上按需挂
//
//	public.GET("/module-market/list", middleware.TenantBindIfAuthenticated(), ctrl.List)
func TenantBindIfAuthenticated() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, exists := c.Get(TenantContextKey)
		if !exists {
			c.Next()
			return
		}
		id, ok := raw.(uint)
		if !ok || id == 0 {
			c.Next()
			return
		}
		// 已有租户绑定就不覆盖（公开租户短码提示可能已绑过）
		if _, bound := tenant.FromContext(c.Request.Context()); bound {
			c.Next()
			return
		}
		c.Request = c.Request.WithContext(tenant.OnlyTenant(c.Request.Context(), id))
		c.Next()
	}
}
