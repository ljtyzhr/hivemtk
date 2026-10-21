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

	// 这条口挂在 public 组上，链上只有 OptionalAuthMiddleware：认得出身份就写进上下文，
	// 认不出也不拦。因此「谁能看到什么」只能在这里按身份切片，而不是靠状态码。
	if ctx.GetString("role") != "admin" {
		response.Success(ctx, publicSystemInfo(info), "获取系统信息成功")
		return
	}

	response.Success(ctx, info, "获取系统信息成功")
}

// publicSystemInfo 非超管（含匿名）可见的那一片字段。
//
// 留下的只有前端启动期需要的功能开关；被裁掉的是这台机器的画像——主机名（内网机器名/
// 容器命名常带项目与租户信息）、Go 版本（精确到 minor，直接可对漏洞库）、uptime（判定
// 重启窗口与实例新鲜度）、CPU/内存/磁盘占用与协程数（容量与负载侧写）。
// 判据留在此处而不是路由表，是因为这些字段同属一个 DTO，路由层无法只拦其中几个。
func publicSystemInfo(info *service.SystemInfo) gin.H {
	return gin.H{"platform_enabled": info.PlatformEnabled}
}

func (c *SystemInfoController) Health(ctx *gin.Context) {
	response.Success(ctx, gin.H{"status": "ok"}, "ok")
}

// 移除 SystemMenus（返回空数组的占位桩）。
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
