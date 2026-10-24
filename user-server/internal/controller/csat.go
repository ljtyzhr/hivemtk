// csat_controller.go CSAT 控制器（五层 L2）
package controller

import (
	"net/http"
	"strconv"

	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/reporting"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// CSATController 满意度控制器
type CSATController struct {
	svc *service.CSATService
}

// NewCSATController 构造
func NewCSATController() *CSATController {
	return &CSATController{svc: service.NewCSATService()}
}

// @Summary      评分已提交
// @Tags      c-s-a-t
// @Router    /api/customer-sessions/{id}/csat [post]
// Submit POST /api/customer-sessions/:id/csat {score, comment}
func (c *CSATController) Submit(ctx *gin.Context) {
	var req struct {
		Score   int    `json:"score" binding:"required"`
		Comment string `json:"comment"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误：score 必填(1-5)")
		return
	}
	survey, err := c.svc.Submit(ctx.Request.Context(), ctx.Param("id"), req.Score, req.Comment)
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, survey, "评分已提交")
}

// @Summary      调查已触发
// @Tags      c-s-a-t
// @Router    /api/customer-sessions/{id}/csat/trigger [post]
// Trigger POST /api/customer-sessions/:id/csat/trigger
func (c *CSATController) Trigger(ctx *gin.Context) {
	survey, err := c.svc.Trigger(ctx.Request.Context(), ctx.Param("id"), "manual")
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, survey, "调查已触发")
}

// @Summary      CSAT 满意度统计（可按时间窗）
// @Tags      c-s-a-t
// @Router    /api/csat/stats [get]
// Stats GET /api/csat/stats?window=month|week|7d|30d（缺省=全量）
func (c *CSATController) Stats(ctx *gin.Context) {
	stats, err := c.svc.Stats(ctx.Request.Context(), ctx.Query("window"))
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, stats, "ok")
}

// @Summary      CSAT 满意度趋势（按天）
// @Tags      c-s-a-t
// @Router    /api/csat/trend [get]
// Trend GET /api/csat/trend?days=30
func (c *CSATController) Trend(ctx *gin.Context) {
	days := queryInt(ctx, "days", reporting.DefaultWindowDays(ctx.Request.Context()))
	list, err := c.svc.Trend(ctx.Request.Context(), days)
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, gin.H{"list": list, "total": len(list)}, "ok")
}

// @Summary      查询差评会话列表
// @Tags      c-s-a-t
// @Router    /api/csat/negative [get]
// Negative GET /api/csat/negative
func (c *CSATController) Negative(ctx *gin.Context) {
	limit := queryInt(ctx, "limit", 50)
	list, threshold, err := c.svc.Negative(ctx.Request.Context(), limit)
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, gin.H{"list": list, "total": len(list), "threshold": threshold}, "ok")
}

// @Summary      获取 CSAT 评价模板
// @Tags      c-s-a-t
// @Router    /api/csat/template [get]
// GetTemplate GET /api/csat/template
func (c *CSATController) GetTemplate(ctx *gin.Context) {
	response.Success(ctx, c.svc.GetTemplate(ctx.Request.Context()), "ok")
}

// @Summary      模板已保存
// @Tags      c-s-a-t
// @Router    /api/csat/template [put]
// SaveTemplate PUT /api/csat/template
func (c *CSATController) SaveTemplate(ctx *gin.Context) {
	var req map[string]any
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误："+err.Error())
		return
	}
	// 防御空 body {} 把模板配置清空
	if len(req) == 0 {
		response.Error(ctx, http.StatusBadRequest, "模板内容不能为空")
		return
	}
	if err := c.svc.SaveTemplate(ctx.Request.Context(), req); HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, req, "模板已保存")
}

func queryInt(ctx *gin.Context, key string, def int) int {
	raw := ctx.Query(key)
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return n
}
