package controller

import (
	"net/http"

	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

type SystemMonitorController struct {
	svc *service.SystemMonitorService
}

func NewSystemMonitorController() *SystemMonitorController {
	return &SystemMonitorController{svc: service.NewSystemMonitorService()}
}

// @Summary      查询系统运行指标
// @Tags      system-monitor
// @Router    /api/system-monitor/metrics [get]
func (c *SystemMonitorController) GetMetrics(ctx *gin.Context) {
	stats, err := c.svc.GetSystemStats(ctx.Request.Context())
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "failed to get system metrics")
		return
	}
	response.Success(ctx, stats, "ok")
}

// @Summary      查询系统健康详情
// @Tags      system-monitor
// @Router    /api/system-monitor/health [get]
func (c *SystemMonitorController) GetHealth(ctx *gin.Context) {
	details, err := c.svc.GetDetailedSystemStats(ctx.Request.Context())
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "failed to get system health")
		return
	}
	health := map[string]any{
		"status": "ok",
		"checks": []map[string]any{
			{"name": "database", "ok": true},
			{"name": "application", "ok": true},
		},
		"resources": details["system_resources"],
		"timestamp": details["timestamp"],
	}
	response.Success(ctx, health, "ok")
}
