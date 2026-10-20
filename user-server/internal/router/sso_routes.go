package router

import (
	"hivemtk-user/internal/controller"

	"github.com/gin-gonic/gin"
)

// setupSSORoutes 注册公开档的 SSO 三条入口。
//
// 控制器由 Setup 构造后传进来（与 liveCodeController 同一形状），因为 admin 档的
// 两条自配端点必须和它共用同一个 service 实例：租户 IdP 适配器缓存挂在实例上，
// 两份实例等于两份缓存，"保存后立刻生效"就只在其中一条路径上成立。
func setupSSORoutes(public *gin.RouterGroup, ssoCtrl *controller.SSOController) {
	public.GET("/sso/providers", ssoCtrl.ListProviders)
	public.GET("/sso/login/:provider", ssoCtrl.Login)
	public.GET("/sso/callback/:provider", ssoCtrl.Callback)
}

// setupSSOConfigRoutes 注册 admin 档的租户 IdP 自配端点（R5-4，SaaS 多租户）。
//
// 挂 admin 而不是普通登录档：这一格写的是**本租户**的 IdP 地址与 client_secret，
// 拿到它就等于能把全租户用户的登录流量导向自己的服务器。租户内的这一权限只给
// 管理档，与参数中心的租户覆盖写入口同档（见 config_param_routes.go）。
func setupSSOConfigRoutes(systemAdmin *gin.RouterGroup, ssoCtrl *controller.SSOController) {
	systemAdmin.GET("/sso/config", ssoCtrl.GetTenantProviders)
	systemAdmin.PUT("/sso/config", ssoCtrl.SaveTenantProviders)
}
