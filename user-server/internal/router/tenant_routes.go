package router

import (
	"hivemtk-user/internal/controller"

	"github.com/gin-gonic/gin"
)

// setupTenantRoutes 注册租户与自助开户路由（specs/001-saas-multi-tenant T030/T033）
//
// 三档挂载，权限语义各不相同：
//   - public：POST /api/tenant/register。SaaS 引流入口，登录前唯一可写端点；
//     off 模式由 Service 判 4104，注册页前端按 SaaS 显隐，无需在此处挡。
//   - plainAuth：GET /api/tenant/info。租户标识展示用，任何登录用户都要读。
//   - admin：GET /api/manage/tenants 与 PATCH .../status。跨租户可见的数据，
//     是平台运营动作（停用会影响该租户全部用户登录），必须硬超管守卫。
func setupTenantRoutes(public, plainAuth, admin *gin.RouterGroup) {
	ctrl := controller.NewTenantController()

	// 分组可能尚未装配（Setup 早期阶段只有 public）：逐档 nil 防护，
	// 鉴权档由调用方在 auth 组就绪后二次传入（setupTenantAuthedRoutes）。
	if public != nil {
		public.POST("/tenant/register", ctrl.Register)
		// 监管告知公开口已由 module_routes.go 注册（module 市场与注册页共用一份），
		// 此处不得重复挂载——gin 对同路径双注册直接 panic。
	}
	if plainAuth != nil {
		plainAuth.GET("/tenant/info", ctrl.Info)
		plainAuth.GET("/tenant/registration", ctrl.Registration)
	}
	if admin != nil {
		admin.GET("/manage/tenants", ctrl.ListTenants)
		admin.PATCH("/manage/tenants/:id/status", ctrl.UpdateTenantStatus)
		// 登记补同步：平台不可达时用户已经注册成功，只是登记没上报，
		// 运营可以在这里手动重试而不必让用户重新注册。
		admin.POST("/manage/tenants/:id/retry-registration-sync", ctrl.RetrySync)
	}
}
