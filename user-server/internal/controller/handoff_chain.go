package controller

import (
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// HandoffChainController 管理端会话链路 + SLA + 自动化规则控制器
type HandoffChainController struct {
	chainSvc *service.SessionChainService
	ruleSvc  *service.RuleEngineService
}

// NewHandoffChainController 构造
func NewHandoffChainController() *HandoffChainController {
	return &HandoffChainController{
		chainSvc: service.NewSessionChainServiceFromGlobal(),
		ruleSvc:  service.NewRuleEngineServiceFromGlobal(),
	}
}

// [AUTO] GetAutoResolveConfig
// @Tags      handoff-chain
// @Router    /api/manage/session-chain/sla-config [get]
// GetAutoResolveConfig GET /api/manage/session-chain/sla-config
func (c *HandoffChainController) GetAutoResolveConfig(ctx *gin.Context) {
	cfg := c.chainSvc.GetAutoResolveConfig(ctx.Request.Context())
	response.Success(ctx, cfg, "ok")
}

// SaveAutoResolveConfig PUT /api/manage/session-chain/sla-config
func (c *HandoffChainController) SaveAutoResolveConfig(ctx *gin.Context) {
	var cfg service.AutoResolveConfig
	if err := ctx.ShouldBindJSON(&cfg); err != nil {
		response.Error(ctx, 400, "请求参数错误："+err.Error())
		return
	}
	if err := c.chainSvc.SaveAutoResolveConfig(ctx.Request.Context(), &cfg); HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, cfg, "SLA 配置已保存")
}

// ReopenOnInboundMessage POST /api/manage/session-chain/reopen
// body: {"session_id": "..."}
func (c *HandoffChainController) ReopenOnInboundMessage(ctx *gin.Context) {
	var req struct {
		SessionID string `json:"session_id" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, 400, "session_id 必填")
		return
	}
	reopened, err := c.chainSvc.ReopenOnInboundMessage(ctx.Request.Context(), req.SessionID)
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, gin.H{"session_id": req.SessionID, "reopened": reopened}, "ok")
}

// [AUTO] ListRules
// @Tags      handoff-chain
// @Router    /api/manage/rules [get]
// ListRules GET /api/manage/rules?event=
func (c *HandoffChainController) ListRules(ctx *gin.Context) {
	list, err := c.ruleSvc.List(ctx.Request.Context(), ctx.Query("event"))
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, gin.H{"list": list, "total": len(list)}, "ok")
}
