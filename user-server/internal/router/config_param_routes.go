package router

import (
	"hivemtk-user/internal/controller"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupConfigParamRoutes 注册配置参数中心的路由。
//
// admin 组是「已通过 AdminAuthMiddleware 的 auth 子组」：五个写/查全量的端点必须挂它。
// 2026-10-11 补：此前这五个端点直接挂在 auth 上，而 auth 只要求登录、不要求管理员
// ——任何普通登录用户都能改上传上限、改验签旁路、改锁 TTL。前端侧
// `router/modules/system.js` 的 ConfigParams 路由一直标了 requiresAdmin，
// 两端口径不一致，靠前端藏菜单挡着。改挂 admin 组把后端口径补齐。
//
// plainAuth 组只挂一条只读的 Runtime（GET /api/config/runtime），它返回白名单内的
// 组的值，供前端缓存。两条组的分界见 controller/config_param_runtime.go。
func setupConfigParamRoutes(plainAuth, admin *gin.RouterGroup, db *gorm.DB) {
	svc := service.GlobalConfigParam()
	ctrl := controller.NewConfigParamController(svc)

	plainAuth.GET("/config/runtime", ctrl.Runtime)

	admin.GET("/manage/config-params", ctrl.List)
	admin.GET("/manage/config-params/audit-logs", ctrl.AuditLogs)
	admin.PUT("/manage/config-params/:group/:key", ctrl.Update)
	admin.POST("/manage/config-params/:group/:key/reset", ctrl.ResetToDefault)
	admin.POST("/manage/config-params/:group/reset", ctrl.BulkResetGroup)
}
