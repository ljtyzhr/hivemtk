package controller

import (
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// [AUTO] AssignSession
// @Tags      customer-session
// @Router    /api/customer-sessions/assign [post]
// AssignSession 分配会话
func (c *CustomerSessionController) AssignSession(ctx *gin.Context) {
	var req service.AssignSessionRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	// 数据隔离：分配前先过可见性检查（未分配池/自己名下可操作）
	if _, err := c.sessionService.GetSessionByIDScoped(ctx.Request.Context(), req.SessionID, sessionScopeFromCtx(ctx)); err != nil {
		writeSessionScopedError(ctx, err, "会话不存在")
		return
	}

	if err := c.sessionService.AssignSession(ctx.Request.Context(), &req); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, nil, "分配成功")
}

// [AUTO] AutoAssignSession
// @Tags      customer-session
// @Router    /api/customer-sessions/{id}/auto-assign [post]
// AutoAssignSession 自动分配会话
func (c *CustomerSessionController) AutoAssignSession(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的会话ID")
		return
	}

	// 数据隔离：自动分配前先过可见性检查
	if _, err := c.sessionService.GetSessionByIDScoped(ctx.Request.Context(), uint(id), sessionScopeFromCtx(ctx)); err != nil {
		writeSessionScopedError(ctx, err, "会话不存在")
		return
	}

	if err := c.sessionService.AutoAssign(ctx.Request.Context(), uint(id)); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, nil, "分配成功")
}

// [AUTO] TransferSession
// @Tags      customer-session
// @Router    /api/customer-sessions/{id}/transfer [post]
// TransferSession 转接会话
func (c *CustomerSessionController) TransferSession(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的会话ID")
		return
	}

	var req struct {
		NewAgentID uint `json:"new_agent_id" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	// 数据隔离：转接前先过可见性检查（他人名下的会话不可转接）
	if _, err := c.sessionService.GetSessionByIDScoped(ctx.Request.Context(), uint(id), sessionScopeFromCtx(ctx)); err != nil {
		writeSessionScopedError(ctx, err, "会话不存在")
		return
	}

	if err := c.sessionService.TransferSession(ctx.Request.Context(), uint(id), req.NewAgentID); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, nil, "转接成功")
}
