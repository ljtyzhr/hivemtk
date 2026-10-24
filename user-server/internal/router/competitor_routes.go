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

	customToolCtrl := controller.NewCustomToolController()
	auth.POST("/custom-tools/import", customToolCtrl.Import)
	auth.GET("/custom-tools", customToolCtrl.List)
	auth.DELETE("/custom-tools/:id", customToolCtrl.Delete)

	agentAttrCtrl := controller.NewAgentAttributionController()
	auth.GET("/agent-attribution/performance", agentAttrCtrl.GetPerformance)

	dataExportCtrl := controller.NewDataExportController()
	auth.GET("/gdpr/export/:customer_id", dataExportCtrl.Export)
}
