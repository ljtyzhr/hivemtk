package controller

import (
	"net/http"
	"strconv"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// ConfigParamController 动态阈值参数管理端 CRUD
//
// 权限层级：所有路由都挂在 AdminAuthMiddleware 下（router 层保证）。
// 路由映射见 config_param_routes.go。
type ConfigParamController struct {
	svc *service.ConfigParamService
}

func NewConfigParamController(svc *service.ConfigParamService) *ConfigParamController {
	return &ConfigParamController{svc: svc}
}

// List GET /api/manage/config-params
// 全部参数（分组按 group 字段排序）
//
// include_deprecated=1 时连已下架的旋钮一起返回（查历史值用）。默认不返回：
// 页面上挂着一个改了不生效的旋钮，比没有这个旋钮更糟。
func (c *ConfigParamController) List(ctx *gin.Context) {
	group := ctx.Query("group")
	includeDeprecated := ctx.Query("include_deprecated") == "1"
	var (
		list interface{}
		err  error
	)
	if group != "" {
		list, err = c.svc.ListByGroup(ctx.Request.Context(), group, includeDeprecated)
	} else {
		list, err = c.svc.List(ctx.Request.Context(), includeDeprecated)
	}
	if err != nil {
		logger.Warnf("[ConfigParam] List group=%s err=%v", group, err)
		ctx.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"code": 0, "data": list, "message": "ok"})
}

// ListGroups GET /api/manage/config-params/groups
// 分组元数据（中文名 / 说明 / 排序）。
//
// 前端原来硬编码了一份 29 组的 GROUP_LABELS，那份清单与 param_group 是两份事实：
// 新增一个组前端不知道中文名，页面上就显示 "inbox_sales" 这种英文 key，像出了 bug。
// 这里下发唯一事实源，前端只负责渲染。
func (c *ConfigParamController) ListGroups(ctx *gin.Context) {
	groups, err := c.svc.Groups(ctx.Request.Context())
	if err != nil {
		logger.Warnf("[ConfigParam] ListGroups err=%v", err)
		ctx.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	// nil slice 序列化成 null，前端 map/Object.entries 会炸；空集合必须是 []。
	if groups == nil {
		groups = []model.ConfigParamGroup{}
	}
	ctx.JSON(http.StatusOK, gin.H{"code": 0, "data": groups, "message": "ok"})
}

// Update PUT /api/manage/config-params/:group/:key
// 更新单个参数值（请求体 { "value": "xxx" }）
func (c *ConfigParamController) Update(ctx *gin.Context) {
	group := ctx.Param("group")
	key := ctx.Param("key")

	var body struct {
		Value string `json:"value" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusOK, gin.H{"code": 400, "message": "参数错误: " + err.Error()})
		return
	}
	actorID := c.currentUserID(ctx)
	if err := c.svc.UpdateValue(ctx.Request.Context(), group, key, body.Value, actorID); err != nil {
		logger.Warnf("[ConfigParam] Update %s.%s err=%v", group, key, err)
		ctx.JSON(http.StatusOK, gin.H{"code": 400, "message": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok"})
}

// ResetToDefault POST /api/manage/config-params/:group/:key/reset
// 重置单条为默认值
func (c *ConfigParamController) ResetToDefault(ctx *gin.Context) {
	group := ctx.Param("group")
	key := ctx.Param("key")
	actorID := c.currentUserID(ctx)
	if err := c.svc.ResetToDefault(ctx.Request.Context(), group, key, actorID); err != nil {
		logger.Warnf("[ConfigParam] ResetToDefault %s.%s err=%v", group, key, err)
		ctx.JSON(http.StatusOK, gin.H{"code": 400, "message": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok"})
}

// BulkResetGroup POST /api/manage/config-params/:group/reset
// 整组重置为默认值
func (c *ConfigParamController) BulkResetGroup(ctx *gin.Context) {
	group := ctx.Param("group")
	actorID := c.currentUserID(ctx)
	if err := c.svc.BulkResetGroup(ctx.Request.Context(), group, actorID); err != nil {
		logger.Warnf("[ConfigParam] BulkResetGroup %s err=%v", group, err)
		ctx.JSON(http.StatusOK, gin.H{"code": 400, "message": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok"})
}

// AuditLogs GET /api/manage/config-params/audit-logs?limit=100
// 变更日志（管理端只读，用于审计回溯）
func (c *ConfigParamController) AuditLogs(ctx *gin.Context) {
	limit, _ := strconv.Atoi(ctx.DefaultQuery("limit", "100"))
	logs, err := c.svc.AuditLogs(ctx.Request.Context(), limit)
	if err != nil {
		logger.Warnf("[ConfigParam] AuditLogs err=%v", err)
		ctx.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"code": 0, "data": logs, "message": "ok"})
}

func (c *ConfigParamController) currentUserID(ctx *gin.Context) uint {
	if id, exists := ctx.Get("user_id"); exists {
		switch v := id.(type) {
		case uint:
			return v
		case int:
			return uint(v)
		case string:
			n, _ := strconv.Atoi(v)
			return uint(n)
		case float64:
			return uint(v)
		}
	}
	return 0
}
