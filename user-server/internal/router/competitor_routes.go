package router

import (
	"hivemtk-user/internal/controller"
	"hivemtk-user/internal/repository"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func setupCompetitorFeatureRoutes(auth *gin.RouterGroup, gormDB *gorm.DB) {

	coPilotCtrl := controller.NewAgentCoPilotController()
	auth.GET("/agent/co-pilot/config", coPilotCtrl.GetConfig)
	auth.PUT("/agent/co-pilot/config", coPilotCtrl.SaveConfig)
	auth.POST("/agent/co-pilot/evaluate", coPilotCtrl.Evaluate)

	smartRouterCtrl := controller.NewSmartRouterController(service.NewSmartRouterWithRepos(
		repository.NewAgentStatusRepositoryWithDB(gormDB),
	))
	auth.POST("/smart-router/select", smartRouterCtrl.SelectAgent)

	ragEvalCtrl := controller.NewRagEvalController()
	auth.POST("/rag-eval/run", ragEvalCtrl.Run)
	auth.GET("/rag-eval/runs", ragEvalCtrl.List)
	auth.GET("/rag-eval/runs/:id", ragEvalCtrl.Get)

	dialogueEvalCtrl := controller.NewDialogueEvalController()
	auth.POST("/dialogue-eval/run", dialogueEvalCtrl.Run)
	auth.GET("/dialogue-eval/runs", dialogueEvalCtrl.List)
	auth.GET("/dialogue-eval/runs/:id", dialogueEvalCtrl.Get)
	// CSV 评测集（P0-1 残点 5）：模板 → 导入 → 列表 → 导出（字节对账）
	auth.GET("/dialogue-eval/sets/template", dialogueEvalCtrl.Template)
	auth.POST("/dialogue-eval/sets/import", dialogueEvalCtrl.ImportSet)
	auth.GET("/dialogue-eval/sets", dialogueEvalCtrl.ListSets)
	auth.GET("/dialogue-eval/sets/:id/export", dialogueEvalCtrl.ExportSet)

	mcpClientCtrl := controller.NewMCPClientController()
	auth.GET("/mcp-servers", mcpClientCtrl.List)
	auth.POST("/mcp-servers", mcpClientCtrl.Create)
	auth.DELETE("/mcp-servers/:id", mcpClientCtrl.Delete)
	auth.PUT("/mcp-servers/:id/enabled", mcpClientCtrl.SetEnabled)
	auth.GET("/mcp-servers/:id/test", mcpClientCtrl.Test)

	customToolCtrl := controller.NewCustomToolController()
	auth.POST("/custom-tools/import", customToolCtrl.Import)
	auth.GET("/custom-tools", customToolCtrl.List)
	auth.DELETE("/custom-tools/:id", customToolCtrl.Delete)

	agentAttrCtrl := controller.NewAgentAttributionController()
	auth.GET("/agent-attribution/performance", agentAttrCtrl.GetPerformance)

	dataExportCtrl := controller.NewDataExportController()
	auth.GET("/gdpr/export/:customer_id", dataExportCtrl.Export)
}
