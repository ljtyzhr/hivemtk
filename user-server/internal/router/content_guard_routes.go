// content_guard_routes.go 内容安全闸（ContentGuard）的路由挂载点。
//
// 单独一个文件的理由与 bad_case_routes.go / human_task_routes.go 同一条：
// 让"这套端点挂在哪个组、用哪一档守卫"只有一处可查。内容审查的写端点（改词库、
// 冻结、解冻）决定"谁的内容能被放行"，挂错组（例如挂到 plainAuth）等于把
// 内容合规开关交给任何登录用户。
//
// 服务实例取自全局登记处（装配点 app.InitContentGuardRuntime，在 router.Setup 之前）。
// 取到 nil 不是错误：路由照挂，每个请求回 503 —— 本处的"关闸"就是不给它底座。
package router

import (
	"github.com/gin-gonic/gin"

	"hivemtk-user/internal/contentguard/controller"
	guardservice "hivemtk-user/internal/contentguard/service"
	"hivemtk-user/internal/pkg/utils/logger"
)

// setupContentGuardRoutes 在**已鉴权 + 租户 admin** 的组下挂 15 条端点
// （/api/manage/content-guard/*）。
//
// 守卫档位的判据按 CLAUDE.md 的"数据域"口径：guard_* 五张表都是**租户表**
// （tenant_id 由隔离 callback 限死在本租户），操作的是本租户自己的词库与处罚，
// 所以是 tenantAdmin 档，不是平台运营方（systemAdmin）档。
func setupContentGuardRoutes(admin *gin.RouterGroup) {
	rt := guardservice.GlobalRuntime()
	controller.NewGuardController(rt).RegisterRoutes(admin)
	if rt == nil || !rt.Available() {
		logger.Infof("[Router] content-guard 路由已挂载，但运行时未装配 ⇒ 全部端点回 503" +
			"（检查 app.InitContentGuardRuntime 是否在路由之前跑过）")
		return
	}
	logger.Infof("[Router] content-guard API 已连通（/api/manage/content-guard/*）")
}
