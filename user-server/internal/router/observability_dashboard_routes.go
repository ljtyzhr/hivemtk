package router

import (
	"hivemtk-user/internal/controller"
	"hivemtk-user/internal/repository"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RegisterObservabilityDashboardRoutes 注册统一看板端点（D2 + B2，specs/007）。
//
// 鉴权由**调用方**负责：rg 必须已挂载 JWTAuthMiddleware（与 monitor.RegisterRoutes
// 同一调用点、同一 auth 组）。本函数不自带鉴权中间件，切勿接到未受保护的路由组上——
// 看板返回的是租户内的成本/送达/渠道健康数据，匿名可读与当年 7 条 /api/monitor/*
// 匿名可读是同一种缺陷。
func RegisterObservabilityDashboardRoutes(rg *gin.RouterGroup, gormDB *gorm.DB) {
	ctrl := controller.NewObservabilityDashboardController(
		service.NewObservabilityDashboardService(repository.NewObservabilityDashboardRepository(gormDB)))
	rg.GET("/observability/dashboard", ctrl.Snapshot)
}
