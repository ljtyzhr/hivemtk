package controller

import (
	"hivemtk-user/internal/pkg/shared/service"

	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/system/install"

	"github.com/gin-gonic/gin"
)

type SystemInfoController struct {
	service *service.SystemStatsService
}

func NewSystemInfoController() *SystemInfoController {
	return &SystemInfoController{
		service: service.NewSystemStatsService(),
	}
}

// GetSystemInfo 获取系统信息
// @Summary 获取系统信息
// @Description 获取系统基本信息，包括版本、运行环境等
// @Tags 系统
// @Produce json
// @Success 200 {object} object{data=service.SystemInfo} "获取成功"
// @Router /api/system/info [get]
func (c *SystemInfoController) GetSystemInfo(ctx *gin.Context) {
	info, err := c.service.GetSystemInfo()
	if err != nil {
		response.ErrorFromDB(ctx, err, "获取系统信息失败")
		return
	}

	response.Success(ctx, info, "获取系统信息成功")
}

func (c *SystemInfoController) Health(ctx *gin.Context) {
	response.Success(ctx, gin.H{"status": "ok"}, "ok")
}

// 2026-10-12 移除 SystemMenus（返回空数组的占位桩）。
// 它原先被 frontend_aliases 注册为 GET /api/system/menus 的兼容别名，
// 菜单 RBAC 上线后该路径已由 menu_routes.go 的 AdminGetAllMenus 承担，
// 且桩返回的 [] 没有 tree 字段，前端 RoleList.vue 读不到任何节点。
// 保留一个无路由、无测试覆盖、语义已被取代的导出方法没有意义，
// 与配置治理里「没有消费点的东西要么接上要么删掉」是同一条标准。

// IsSystemInitialized 检查系统是否已完成安装（install.lock.initialized=true）。
// 供 router / init 流程调用，避免路由层直接依赖 install 包。
func IsSystemInitialized() bool {
	return install.GetStatus().Initialized
}
