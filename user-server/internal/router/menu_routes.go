package router

import (
	"hivemtk-user/internal/controller"
	"hivemtk-user/internal/middleware"

	"github.com/gin-gonic/gin"
)

// setupMenuRoutes 菜单权限路由
//
// 两块：
//  1. GET /api/menus —— 任意登录用户拿自己的可见菜单树（超管全量，其余按角色分配）
//  2. /api/system/*  —— 管理端注册表查询 + 角色菜单分配读写，全部 RequireAdminMiddleware
//
// 只做 URL→Controller 映射，业务判断在 service。
func setupMenuRoutes(auth *gin.RouterGroup) {
	ctrl := controller.NewMenuController()

	// 登录即可读自己的菜单（渲染导航 + 路由守卫的数据源）
	auth.GET("/menus", ctrl.GetMyMenus)

	adminSystem := auth.Group("/system", middleware.RequireAdminMiddleware())
	{
		adminSystem.GET("/menus", ctrl.AdminGetAllMenus)
	}

	adminRoles := auth.Group("/system/roles", middleware.RequireAdminMiddleware())
	{
		adminRoles.GET("/:code/menus", ctrl.AdminGetRoleMenus)
		adminRoles.PUT("/:code/menus", ctrl.AdminUpdateRoleMenus)
	}
}
