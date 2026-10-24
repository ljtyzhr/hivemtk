package controller

import (
	"net/http"
	"strconv"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// XianyuCardController 闲鱼卡片控制器
type XianyuCardController struct {
	service  service.XianyuCardService
	statsSvc service.XianyuCardStatsService
}

// NewXianyuCardController 创建闲鱼卡片控制器实例
// statsSvc 在构造时注入，避免 Controller 方法内直接访问数据库（架构违规）
func NewXianyuCardController(service service.XianyuCardService, statsSvc service.XianyuCardStatsService) *XianyuCardController {
	return &XianyuCardController{
		service:  service,
		statsSvc: statsSvc,
	}
}

// [AUTO] Create
// @Tags      xianyu-card
// @Router    /api/xianyu/create [post]
// Create 创建闲鱼卡片
func (c *XianyuCardController) Create(ctx *gin.Context) {
	var req dto.XianyuCardCreateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidParams, err.Error())
		return
	}

	card, err := c.service.Create(ctx.Request.Context(), &req)
	if HandleDBError(ctx, err, "创建闲鱼卡片") {
		return
	}

	response.Success(ctx, card, response.ErrSuccess)
}

// [AUTO] Update
// @Tags      xianyu-card
// @Router    /api/xianyu-card/{id} [put]
// Update 更新闲鱼卡片
func (c *XianyuCardController) Update(ctx *gin.Context) {
	var req dto.XianyuCardUpdateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidParams, err.Error())
		return
	}

	card, err := c.service.Update(ctx.Request.Context(), &req)
	if HandleDBError(ctx, err, "更新闲鱼卡片") {
		return
	}

	response.Success(ctx, card, response.ErrUpdateSuccess)
}

// [AUTO] Delete
// @Tags      xianyu-card
// @Router    /api/xianyu-card/{id} [delete]
// Delete 删除闲鱼卡片
func (c *XianyuCardController) Delete(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidIDFormat, err.Error())
		return
	}

	if HandleDBError(ctx, c.service.Delete(ctx.Request.Context(), uint(id)), "删除闲鱼卡片") {
		return
	}

	response.Success(ctx, gin.H{"id": id}, response.ErrDeleteSuccess)
}

// [AUTO] GetByID
// @Tags      xianyu-card
// @Router    /api/xianyu-card/{id} [get]
// GetByID 根据ID获取闲鱼卡片
func (c *XianyuCardController) GetByID(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidIDFormat, err.Error())
		return
	}

	card, err := c.service.GetByID(ctx.Request.Context(), uint(id))
	if HandleDBError(ctx, err, "获取闲鱼卡片") {
		return
	}

	response.Success(ctx, card, response.ErrGetSuccess)
}

// [AUTO] GetList
// @Tags      xianyu-card
// @Router    /api/xianyu-card/list [get]
// GetList 获取闲鱼卡片列表
func (c *XianyuCardController) GetList(ctx *gin.Context) {
	var req dto.XianyuCardListRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidParams, err.Error())
		return
	}

	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 10
	}

	cards, err := c.service.GetList(ctx.Request.Context(), &req)
	if err != nil {
		response.ErrorFromDB(ctx, err, response.ErrGetListFailed, err.Error())
		return
	}

	response.Success(ctx, cards, response.ErrGetSuccess)
}

// [AUTO] GenerateShortLink
// @Tags      xianyu-card
// @Router    /api/xianyu/{id}/generate-short-link [post]
// GenerateShortLink 生成短链
func (c *XianyuCardController) GenerateShortLink(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidIDFormat, err.Error())
		return
	}

	card, err := c.service.GetByIDWithRefresh(ctx.Request.Context(), uint(id))
	if HandleDBError(ctx, err, "获取闲鱼卡片") {
		return
	}

	response.Success(ctx, gin.H{
		"short_link_url": card.ShortLinkURL,
		"short_code":     card.ShortCode,
	}, response.ErrSuccess)
}

// [AUTO] SharePage
// @Tags      xianyu-card
// @Router    /share/xianyu/{id} [get]
// SharePage 渲染卡片分享页面（统一卡片聊天页，含联系客服按钮）
func (c *XianyuCardController) SharePage(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		ctx.String(http.StatusBadRequest, "无效的卡片ID")
		return
	}
	renderCardChatPage(ctx, c.service.GenerateCardChatPage, uint(id), buildBaseURL(ctx))
}
