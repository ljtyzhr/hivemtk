package controller

import (
	"hivemtk-user/internal/pkg/utils/pagination"
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// WeComController 企业微信控制器
type WeComController struct {
	wecomService *service.WeComService
}

// NewWeComController 创建企业微信控制器实例
//
// 测试场景下传入 nil 时自动构造默认服务（依赖 dbUtil.GetDB()，
// 由 testutil.NewTestDB + db.SetTestDB 设置全局 DB）。
func NewWeComController(wecomService *service.WeComService) *WeComController {
	if wecomService == nil {
		wecomService = service.NewWeComService()
	}
	return &WeComController{
		wecomService: wecomService,
	}
}

// [AUTO] CreateAccount
// @Tags      we-com
// @Router    /api/wecom/accounts [post]
// CreateAccount 创建企业微信账号
func (c *WeComController) CreateAccount(ctx *gin.Context) {

	var req service.CreateAccountRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误："+err.Error())
		return
	}

	account, err := c.wecomService.CreateAccount(ctx.Request.Context(), &req)
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}

	// 出参脱敏：账号带着 corp_secret / agent_secret / access_token / callback_token /
	// encoding_aes_key，直接序列化就是把商户的企业微信应用密钥发给了前端。
	maskWeComAccount(account)
	response.Success(ctx, account, "创建成功")
}

// [AUTO] GetAccountList
// @Tags      we-com
// @Router    /api/wecom/accounts [get]
// GetAccountList 获取企业微信账号列表
func (c *WeComController) GetAccountList(ctx *gin.Context) {

	accounts, err := c.wecomService.GetAccountList(ctx.Request.Context())
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}

	maskWeComAccounts(accounts)
	response.Success(ctx, accounts, "获取成功")
}

// [AUTO] GetAccountByID
// @Tags      we-com
// @Router    /api/wecom/accounts/{id} [get]
// GetAccountByID 获取企业微信账号详情
func (c *WeComController) GetAccountByID(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的账号 ID")
		return
	}

	account, err := c.wecomService.GetAccountByID(ctx.Request.Context(), uint(id))
	if err != nil {
		response.Error(ctx, http.StatusNotFound, err.Error())
		return
	}

	maskWeComAccount(account)
	response.Success(ctx, account, "获取成功")
}

// [AUTO] UpdateAccount
// @Tags      we-com
// @Router    /api/wecom/accounts/{id} [put]
// UpdateAccount 更新企业微信账号
func (c *WeComController) UpdateAccount(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的账号 ID")
		return
	}

	var req service.CreateAccountRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误："+err.Error())
		return
	}

	account, err := c.wecomService.UpdateAccount(ctx.Request.Context(), uint(id), &req)
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}

	maskWeComAccount(account)
	response.Success(ctx, account, "更新成功")
}

// [AUTO] DeleteAccount
// @Tags      we-com
// @Router    /api/wecom/accounts/{id} [delete]
// DeleteAccount 删除企业微信账号
func (c *WeComController) DeleteAccount(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的账号 ID")
		return
	}

	if err := c.wecomService.DeleteAccount(ctx.Request.Context(), uint(id)); err != nil {
		if isNotFoundError(err) {
			response.Error(ctx, http.StatusNotFound, err.Error())
			return
		}
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}

	response.Success(ctx, nil, "删除成功")
}

// [AUTO] SyncCustomers
// @Tags      we-com
// @Router    /api/wecom/accounts/{id}/sync-customers [post]
// SyncCustomers 同步企业微信客户
func (c *WeComController) SyncCustomers(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的账号 ID")
		return
	}

	account, err := c.wecomService.GetAccountByID(ctx.Request.Context(), uint(id))
	if err != nil {
		response.Error(ctx, http.StatusNotFound, err.Error())
		return
	}

	count, err := c.wecomService.SyncCustomers(ctx.Request.Context(), account)
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}

	response.Success(ctx, gin.H{"count": count}, "同步成功")
}

// [AUTO] GetCustomerList
// @Tags      we-com
// @Router    /api/wecom/customers [get]
// GetCustomerList 获取企业微信客户列表
func (c *WeComController) GetCustomerList(ctx *gin.Context) {

	page, pageSize, err := pagination.Parse(ctx)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	customers, total, err := c.wecomService.GetCustomerList(ctx.Request.Context(), page, pageSize)
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"list":      customers,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "获取成功")
}

// [AUTO] SyncGroups
// @Tags      we-com
// @Router    /api/wecom/accounts/{id}/sync-groups [post]
// SyncGroups 同步企业微信客户群
func (c *WeComController) SyncGroups(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的账号 ID")
		return
	}

	account, err := c.wecomService.GetAccountByID(ctx.Request.Context(), uint(id))
	if err != nil {
		response.Error(ctx, http.StatusNotFound, err.Error())
		return
	}

	count, err := c.wecomService.SyncGroups(ctx.Request.Context(), account)
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}

	response.Success(ctx, gin.H{"count": count}, "同步成功")
}

// [AUTO] GetGroupList
// @Tags      we-com
// @Router    /api/wecom/groups [get]
// GetGroupList 获取企业微信客户群列表
func (c *WeComController) GetGroupList(ctx *gin.Context) {

	page, pageSize, err := pagination.Parse(ctx)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	groups, total, err := c.wecomService.GetGroupList(ctx.Request.Context(), page, pageSize)
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"list":      groups,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "获取成功")
}

// [AUTO] SendMessage
// @Tags      we-com
// @Router    /api/wecom/accounts/{id}/send-message [post]
// SendMessage 发送企业微信消息
func (c *WeComController) SendMessage(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的账号 ID")
		return
	}

	account, err := c.wecomService.GetAccountByID(ctx.Request.Context(), uint(id))
	if err != nil {
		response.Error(ctx, http.StatusNotFound, err.Error())
		return
	}

	var req service.WeComSendMessageRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误："+err.Error())
		return
	}

	msgID, err := c.wecomService.SendMessage(ctx.Request.Context(), account, &req)
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}

	response.Success(ctx, gin.H{"msg_id": msgID}, "发送成功")
}

// [AUTO] RefreshAccount
// @Tags      we-com
// @Router    /api/wecom/accounts/{id}/refresh [post]
// RefreshAccount 刷新 access_token（清空本地缓存，强制向企业微信重新换取）
func (c *WeComController) RefreshAccount(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的账号 ID")
		return
	}

	account, err := c.wecomService.GetAccountByID(ctx.Request.Context(), uint(id))
	if err != nil {
		response.Error(ctx, http.StatusNotFound, err.Error())
		return
	}

	account.AccessToken = ""
	account.TokenExpires = time.Time{}
	if _, err := c.wecomService.GetAccessToken(ctx.Request.Context(), account); err != nil {
		response.ErrorFromDB(ctx, err, "刷新失败："+err.Error())
		return
	}

	account, err = c.wecomService.GetAccountByID(ctx.Request.Context(), uint(id))
	if err != nil {
		response.ErrorFromDB(ctx, err, "刷新后回查账号失败："+err.Error())
		return
	}
	maskWeComAccount(account)
	response.Success(ctx, account, "刷新成功")
}

// [AUTO] GetMessageList
// @Tags      we-com
// @Router    /api/wecom/messages [get]
// GetMessageList 获取企业微信消息列表
func (c *WeComController) GetMessageList(ctx *gin.Context) {

	page, pageSize, err := pagination.Parse(ctx)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	messages, total, err := c.wecomService.GetMessageList(ctx.Request.Context(), page, pageSize)
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"list":      messages,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "获取成功")
}

// [AUTO] GetTagList
// @Tags      we-com
// @Router    /api/wecom/tags [get]
// GetTagList 获取企业微信标签列表
func (c *WeComController) GetTagList(ctx *gin.Context) {

	tags, err := c.wecomService.GetTagList(ctx.Request.Context())
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}

	response.Success(ctx, tags, "获取成功")
}

// [AUTO] SyncTags
// @Tags      we-com
// @Router    /api/wecom/accounts/{id}/sync-tags [post]
// SyncTags 同步企业微信标签
func (c *WeComController) SyncTags(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的账号 ID")
		return
	}

	account, err := c.wecomService.GetAccountByID(ctx.Request.Context(), uint(id))
	if err != nil {
		response.Error(ctx, http.StatusNotFound, err.Error())
		return
	}

	count, err := c.wecomService.SyncTags(ctx.Request.Context(), account)
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}

	response.Success(ctx, gin.H{"count": count}, "同步成功")
}
