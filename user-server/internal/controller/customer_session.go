package controller

import (
	"errors"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// CustomerSessionController 客服会话控制器
type CustomerSessionController struct {
	sessionService *service.CustomerSessionService
}

// NewCustomerSessionController 创建客服会话控制器实例
func NewCustomerSessionController() *CustomerSessionController {
	return &CustomerSessionController{
		sessionService: service.NewCustomerSessionService(),
	}
}

func getUserIDFromContext(ctx *gin.Context) uint {
	if uid, exists := ctx.Get("user_id"); exists {
		switch v := uid.(type) {
		case uint:
			return v
		case float64:
			return uint(v)
		case int:
			return uint(v)
		case int64:
			return uint(v)
		}
	}
	return 0
}

// sessionScopeFromCtx 从登录态组装数据隔离身份（role + user_id）。
func sessionScopeFromCtx(ctx *gin.Context) service.SessionScope {
	return service.SessionScope{
		Role:   ctx.GetString("role"),
		UserID: getUserIDFromContext(ctx),
	}
}

// writeSessionScopedError 数据隔离错误映射：越权 403，其余交回通用错误口径
// （not found → 404，DB 错 → 500），与隔离前的响应形态保持一致。
func writeSessionScopedError(ctx *gin.Context, err error, notFoundMsg string) {
	if errors.Is(err, service.ErrSessionForbidden) {
		response.Error(ctx, http.StatusForbidden, err.Error())
		return
	}
	if !HandleServiceError(ctx, err) {
		response.Error(ctx, http.StatusNotFound, notFoundMsg)
	}
}

// @Summary      获取会话列表
// @Tags      customer-session
// @Router    /api/customer-sessions [get]
// GetSessions 获取会话列表
func (c *CustomerSessionController) GetSessions(ctx *gin.Context) {
	status := model.SessionStatus(ctx.Query("status"))
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "20"))

	sessions, total, err := c.sessionService.GetSessionsScoped(ctx.Request.Context(), status, page, pageSize, sessionScopeFromCtx(ctx))
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"list":      sessions,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "获取成功")
}

// @Summary      获取会话详情
// @Tags      customer-session
// @Router    /api/customer-sessions/{id} [get]
// GetSessionByID 获取会话详情
func (c *CustomerSessionController) GetSessionByID(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的会话ID")
		return
	}

	session, err := c.sessionService.GetSessionByIDScoped(ctx.Request.Context(), uint(id), sessionScopeFromCtx(ctx))
	if err != nil {
		writeSessionScopedError(ctx, err, err.Error())
		return
	}

	response.Success(ctx, session, "获取成功")
}

// @Summary      创建会话
// @Tags      customer-session
// @Router    /api/customer-sessions [post]
// CreateSession 创建会话
func (c *CustomerSessionController) CreateSession(ctx *gin.Context) {
	var req service.CreateSessionRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	session, err := c.sessionService.CreateSession(ctx.Request.Context(), &req)
	if HandleDBError(ctx, err, "创建会话") {
		return
	}

	response.Success(ctx, session, "创建成功")
}

// @Summary      获取会话消息
// @Tags      customer-session
// @Router    /api/customer-sessions/{id}/messages [get]
// GetMessages 获取会话消息
func (c *CustomerSessionController) GetMessages(ctx *gin.Context) {
	idStr := ctx.Param("id")
	if idStr == "" {
		idStr = ctx.Param("session_id")
	}

	sessionID := idStr
	if id, err := strconv.ParseUint(idStr, 10, 32); err == nil {
		session, err := c.sessionService.GetSessionByIDScoped(ctx.Request.Context(), uint(id), sessionScopeFromCtx(ctx))
		if err != nil {
			writeSessionScopedError(ctx, err, err.Error())
			return
		}
		sessionID = session.SessionID
	} else {
		// 入口给的是字符串 session_id：数字 id 那条路已挡，字符串这条路同样过可见性检查。
		if _, err := c.sessionService.GetSessionByIDStringScoped(ctx.Request.Context(), sessionID, sessionScopeFromCtx(ctx)); err != nil {
			writeSessionScopedError(ctx, err, err.Error())
			return
		}
	}

	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "50"))

	messages, total, err := c.sessionService.GetMessages(ctx.Request.Context(), sessionID, page, pageSize)
	if HandleServiceError(ctx, err) {
		return
	}

	response.Success(ctx, gin.H{
		"list":      messages,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "获取成功")
}

// @Summary      发送消息
// @Tags      customer-session
// @Router    /api/customer-sessions/{id}/messages [post]
// SendMessage 发送消息
func (c *CustomerSessionController) SendMessage(ctx *gin.Context) {
	var req service.SendMessageRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	if channelID, ok := ctx.Get("chat_channel_id"); ok {
		req.SenderType = "user"
		switch v := channelID.(type) {
		case uint:
			req.SenderID = strconv.FormatUint(uint64(v), 10)
		case string:
			req.SenderID = v
		default:
			req.SenderID = ""
		}
		req.SenderName = ""
	} else if uid := getUserIDFromContext(ctx); uid != 0 {
		req.SenderType = "agent"
		req.SenderID = strconv.FormatUint(uint64(uid), 10)
		if name, ok := ctx.Get("username"); ok {
			if s, ok := name.(string); ok {
				req.SenderName = s
			}
		}
	} else {
		response.Error(ctx, http.StatusUnauthorized, "未登录或无权访问", "missing auth context")
		return
	}

	if req.SenderType == "" {
		req.SenderType = "user"
	}

	if req.SessionID == "" {
		idStr := ctx.Param("id")
		if idStr == "" {
			response.Error(ctx, http.StatusBadRequest, "请求参数错误: session_id is required")
			return
		}
		req.SessionID = idStr
		if id, err := strconv.ParseUint(idStr, 10, 32); err == nil {
			session, err := c.sessionService.GetSessionByIDScoped(ctx.Request.Context(), uint(id), sessionScopeFromCtx(ctx))
			if err != nil {
				writeSessionScopedError(ctx, err, "会话不存在")
				return
			}
			req.SessionID = session.SessionID
		}
	}
	// 数据隔离：无论入口给的是数字 id 还是字符串 session_id，发消息前统一过可见性检查
	// （不可见 → 403；不存在 → 404，与服务层原口径一致）。
	if _, err := c.sessionService.GetSessionByIDStringScoped(ctx.Request.Context(), req.SessionID, sessionScopeFromCtx(ctx)); err != nil {
		writeSessionScopedError(ctx, err, "会话不存在")
		return
	}

	message, err := c.sessionService.SendMessage(ctx.Request.Context(), &req)
	if err != nil {
		if err.Error() == "会话不存在" {
			response.Error(ctx, http.StatusNotFound, err.Error())
			return
		}
		if strings.Contains(err.Error(), "不允许发送消息") {
			response.Error(ctx, http.StatusBadRequest, err.Error())
			return
		}
		HandleDBError(ctx, err, "发送消息")
		return
	}

	response.Success(ctx, message, "发送成功")
}

// @Summary      更新会话状态
// @Tags      customer-session
// @Router    /api/customer-sessions/{id}/status [put]
// UpdateSessionStatus 更新会话状态
func (c *CustomerSessionController) UpdateSessionStatus(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的会话ID")
		return
	}

	var req struct {
		Status model.SessionStatus `json:"status" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	// 数据隔离：改状态前先过可见性检查（不可见 → 403）
	if _, err := c.sessionService.GetSessionByIDScoped(ctx.Request.Context(), uint(id), sessionScopeFromCtx(ctx)); err != nil {
		writeSessionScopedError(ctx, err, "会话不存在")
		return
	}

	if err := c.sessionService.UpdateSessionStatus(ctx.Request.Context(), uint(id), req.Status); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, nil, "更新成功")
}

// @Summary      评价成功
// @Tags      customer-session
// @Router    /api/customer-sessions/{id}/rate [post]
// RateSession 评价会话
func (c *CustomerSessionController) RateSession(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的会话ID")
		return
	}

	var req struct {
		Rating  int    `json:"rating" binding:"required,min=1,max=5"`
		Comment string `json:"comment"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	// 数据隔离：评价前先过可见性检查
	if _, err := c.sessionService.GetSessionByIDScoped(ctx.Request.Context(), uint(id), sessionScopeFromCtx(ctx)); err != nil {
		writeSessionScopedError(ctx, err, "会话不存在")
		return
	}

	if err := c.sessionService.RateSession(ctx.Request.Context(), uint(id), req.Rating, req.Comment); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, nil, "评价成功")
}

// @Summary      标记成功
// @Tags      customer-session
// @Router    /api/customer-sessions/{id}/tags [post]
// TagSession 标记会话
func (c *CustomerSessionController) TagSession(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的会话ID")
		return
	}

	var req struct {
		Tags []string `json:"tags" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	// 数据隔离：标记前先过可见性检查
	if _, err := c.sessionService.GetSessionByIDScoped(ctx.Request.Context(), uint(id), sessionScopeFromCtx(ctx)); err != nil {
		writeSessionScopedError(ctx, err, "会话不存在")
		return
	}

	if err := c.sessionService.TagSession(ctx.Request.Context(), uint(id), req.Tags); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, nil, "标记成功")
}

// @Summary      获取待处理会话
// @Tags      customer-session
// @Router    /api/customer-sessions/pending [get]
// GetPendingSessions 获取待处理会话
func (c *CustomerSessionController) GetPendingSessions(ctx *gin.Context) {
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}

	sessions, err := c.sessionService.GetPendingSessionsScoped(ctx.Request.Context(), page, pageSize, sessionScopeFromCtx(ctx))
	if err != nil {
		response.ErrorFromDB(ctx, err, "获取待处理会话失败："+err.Error())
		return
	}

	total, _ := c.sessionService.CountPendingSessionsScoped(ctx.Request.Context(), sessionScopeFromCtx(ctx))

	response.Success(ctx, gin.H{
		"list":      sessions,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "获取成功")
}

// @Summary      关闭成功
// @Tags      customer-session
// @Router    /api/customer-sessions/{id}/close [post]
// CloseSession 关闭会话
func (c *CustomerSessionController) CloseSession(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的会话ID")
		return
	}

	// 数据隔离：关闭前先过可见性检查
	if _, err := c.sessionService.GetSessionByIDScoped(ctx.Request.Context(), uint(id), sessionScopeFromCtx(ctx)); err != nil {
		writeSessionScopedError(ctx, err, "会话不存在")
		return
	}

	if err := c.sessionService.UpdateSessionStatus(ctx.Request.Context(), uint(id), model.SessionStatusClosed); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, nil, "关闭成功")
}
