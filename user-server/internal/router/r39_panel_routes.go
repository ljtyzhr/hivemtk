// r39_panel_routes.go R39 收编三面板中真正缺能力的两只：kb-templates（K7）与
// dashboard-templates（K8）。K6（/api/bots）经复算不落此地：mount-agent 已有
// /api/channel-agent-bindings 全 CRUD，跨渠道视图已有 /api/channels/overview，
// register/webhook 本就按渠道分账（qq/telegram/whatsapp/feishu/dingtalk/wecom 各自
// 路由族）——再造一个别名聚合面只会让同一份状态有两个写入口。
//
// 分档判据与 dictionary_routes.go 同口径："端点操作的数据落在哪张表"：
//   - 读 / rate / kb-apply 挂登录档（apply 的主效果是建本租户 KB，与
//     POST /api/knowledge-bases 同档——那条本来就是登录档）；
//   - 模板写 / publish / clone 挂租户 admin（clone 会落一张真实大屏，
//     与 POST /api/dashboards 同档；模板本身是本租户的复用资产）。
package router

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"hivemtk-user/internal/controller"
	"hivemtk-user/internal/middleware"
	opssvc "hivemtk-user/internal/ops/service"
	"hivemtk-user/internal/repository"
	"hivemtk-user/internal/service"
)

func setupR39PanelRoutes(auth *gin.RouterGroup, db *gorm.DB) {
	kbTplSvc := service.NewKbTemplateService(
		repository.NewKbTemplateRepository(),
		service.NewKnowledgeBaseServiceWithRepos(
			repository.NewKnowledgeBaseRepository(db),
			repository.NewAgentKBBindingRepository(db),
			repository.NewKBItemBindingRepository(db)))
	kbTpl := controller.NewKbTemplateController(kbTplSvc)

	auth.GET("/kb-templates", kbTpl.ListKbTemplates)
	auth.GET("/kb-templates/:id", kbTpl.GetKbTemplate)
	auth.POST("/kb-templates/:id/apply", kbTpl.ApplyKbTemplate)
	auth.POST("/kb-templates/:id/rate", kbTpl.RateKbTemplate)

	kbAdmin := auth.Group("/kb-templates", middleware.AdminAuthMiddleware())
	kbAdmin.POST("", kbTpl.CreateKbTemplate)
	kbAdmin.PUT("/:id", kbTpl.UpdateKbTemplate)
	kbAdmin.DELETE("/:id", kbTpl.DeleteKbTemplate)

	dashTplSvc := service.NewDashboardTemplateService(
		repository.NewDashboardTemplateRepository(),
		opssvc.NewDashboardScreenService())
	dashTpl := controller.NewDashboardTemplateController(dashTplSvc)

	auth.GET("/dashboard-templates", dashTpl.ListDashboardTemplates)
	auth.GET("/dashboard-templates/:id", dashTpl.GetDashboardTemplate)
	auth.POST("/dashboard-templates/:id/rate", dashTpl.RateDashboardTemplate)

	dashAdmin := auth.Group("/dashboard-templates", middleware.AdminAuthMiddleware())
	dashAdmin.POST("", dashTpl.CreateDashboardTemplate)
	dashAdmin.PUT("/:id", dashTpl.UpdateDashboardTemplate)
	dashAdmin.DELETE("/:id", dashTpl.DeleteDashboardTemplate)
	dashAdmin.POST("/:id/publish", dashTpl.PublishDashboardTemplate)
	dashAdmin.POST("/publish-from-screen", dashTpl.PublishScreenAsTemplate)
	dashAdmin.POST("/:id/clone", dashTpl.CloneDashboardTemplate)
}
