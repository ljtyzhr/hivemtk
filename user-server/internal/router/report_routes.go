// report_routes.go LLM 观测报表路由（T3）。
//
// 设计文档：docs/architecture/design/T3-observability-reports.md
// 读侧走 auth 组（登录态），租户维度由 GORM 隔离 callback 强制（表带 tenant_id 列）。
// 聚合定时任务：每 5 分钟增量（窗口 10 分钟覆盖乱序），每天 00:10 重算前一天；
// advisory lock + 幂等 upsert 保证多实例安全。
package router

import (
	"context"

	"hivemtk-user/internal/controller"
	"hivemtk-user/internal/pkg/cron"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// SetupReportRoutes LLM 报表读侧 + 聚合任务注册
func SetupReportRoutes(auth *gin.RouterGroup, gormDB *gorm.DB) {
	reportSvc := service.NewReportService(gormDB)
	reportCtrl := controller.NewReportController(reportSvc)

	auth.GET("/reports/llm/overview", reportCtrl.GetOverview)
	auth.GET("/reports/llm/timeseries", reportCtrl.GetTimeseries)
	auth.GET("/reports/llm/breakdown", reportCtrl.GetBreakdown)
	auth.GET("/reports/llm/errors", reportCtrl.GetErrors)

	// 聚合任务（TaskManager 自带 SkipIfStillRunning + Recover；advisory lock 防多实例）
	mgr := cron.GetTaskManager()
	_, _ = mgr.AddTask("0 */5 * * * *", func() {
		_ = reportSvc.AggregateRecent(context.Background())
	})
	_, _ = mgr.AddTask("0 10 0 * * *", func() {
		_ = reportSvc.RunDailyRecalc(context.Background())
	})
}
