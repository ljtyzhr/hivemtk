package router

import (
	"hivemtk-user/internal/pkg/utils"
	"hivemtk-user/internal/pkg/utils/response"

	"github.com/gin-gonic/gin"
)

// 已下线的 /api/knowledge 顶层域名（D-10 选 B）：老客户端此前拿到的是 NoRoute 的 404，
// 分不清"接口不存在"和"地址写错了"。这里补 410 Gone，让它们拿到明确的迁移指引。
//
// 为什么是逐条注册而不是 GET /knowledge/*path 的 catch-all：gin 1.12 的 radix tree
// 不允许 catch-all 与同前缀的静态子节点共存，而 auth 档里仍有两条 GET 别名
// （/api/knowledge/playground/presets、/api/knowledge/help-center/top，见
// frontend_aliases.go）。注册 catch-all 会让 Setup panic，或反过来让那两条别名注册
// 失败（TestFrontendAliases_NoFailedRegistration 会红），代价大于收益。因此只覆盖
// 老客户端实际会打的两个入口：域名根与历史搜索口。
const (
	legacyKnowledgeRootPath   = "/knowledge"
	legacyKnowledgeSearchPath = "/knowledge/search"

	// knowledgeMigrationTarget 是前端已迁移到的权威路径（规划 §2.2 契约第 1 条：
	// 不再有 /api/knowledge 顶层域名）。message 与 data.migrate_to 共用这一份字面量。
	knowledgeMigrationTarget = "/api/knowledge-bases/:id/items"

	legacyKnowledgeGoneMessage = "该接口已废弃，请改用 " + knowledgeMigrationTarget +
		"（/api/knowledge 顶层域名已下线，不再提供服务）"
)

// setupLegacyRoutes 挂两条只回 410 的墓碑路由。
//
// 410 是终点态：不读库、不读租户数据、不写任何状态，所以挂在 public 组匿名可达；
// 匿名面登记与判定见 route_policy_gate_test.go 的 anonymousAllowedRoutes。
func setupLegacyRoutes(public *gin.RouterGroup) {
	gone := func(c *gin.Context) {
		// 传强类型码而不是 http.StatusGone：int 路径的反向映射没有 410，会落
		// UNKNOWN_1000（"未知错误"），N-5 注册 LEGACY_GONE_4100 后两者等价，
		// 但强类型码不依赖反向映射表维持正确。
		response.Error(c, utils.ErrorCodeLegacyGone, legacyKnowledgeGoneMessage, gin.H{
			"migrate_to": knowledgeMigrationTarget,
		})
	}
	public.GET(legacyKnowledgeRootPath, gone)
	public.GET(legacyKnowledgeSearchPath, gone)
}
