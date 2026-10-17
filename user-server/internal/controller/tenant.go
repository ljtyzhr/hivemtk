package controller

import (
	"errors"
	"net/http"
	"strconv"

	bizerr "hivemtk-user/internal/domain/errors"
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// 租户与自助开户端点（specs/001-saas-multi-tenant T029/T030/T033）
//
// 端点集（contracts/api.md）：
//   - POST  /api/tenant/register          公开：SaaS 模式自助开户（off 恒 4104）
//   - GET   /api/tenant/info              登录态：当前租户信息
//   - GET   /api/manage/tenants           平台运营：租户清单（admin）
//   - PATCH /api/manage/tenants/:id/status 平台运营：停用/启用（admin）

// TenantController 租户控制器
type TenantController struct {
	tenantService *service.TenantService
}

// NewTenantController 创建租户控制器
func NewTenantController() *TenantController {
	return &TenantController{tenantService: service.NewTenantService()}
}

// tenantFail 业务错误转换：BizError → 业务码响应体（HTTP 恒 200）
func tenantFail(c *gin.Context, err error) {
	var be *bizerr.BizError
	if errors.As(err, &be) {
		response.ErrorWithBusinessCode(c, be.Code, be.Message, gin.H{})
		return
	}
	response.ErrorWithBusinessCode(c, bizerr.CodeInternal, err.Error(), gin.H{})
}

// Register 自助开户
//
// 公开端点：off 模式由 Service 判 4104；限流在 Service 层按来源键计数（4105）。
func (c *TenantController) Register(ctx *gin.Context) {
	var req service.RegisterTenantRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidParams, err.Error())
		return
	}
	resp, err := c.tenantService.RegisterTenant(ctx.Request.Context(), req, ctx.ClientIP())
	if err != nil {
		tenantFail(ctx, err)
		return
	}
	response.Success(ctx, resp, "注册成功")
}

// Info 当前租户信息
func (c *TenantController) Info(ctx *gin.Context) {
	info, err := c.tenantService.GetTenantInfo(ctx.Request.Context())
	if err != nil {
		tenantFail(ctx, err)
		return
	}
	response.Success(ctx, info, "ok")
}

// ListTenants 租户运营清单（分页 + 状态/关键词过滤）
func (c *TenantController) ListTenants(ctx *gin.Context) {
	var q service.TenantFilter
	if v := ctx.Query("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			q.Page = n
		}
	}
	if v := ctx.Query("page_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			q.PageSize = n
		}
	}
	if v := ctx.Query("status"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 16); err == nil {
			st := int16(n)
			q.Status = &st
		}
	}
	q.Keyword = ctx.Query("keyword")

	rows, total, err := c.tenantService.ListTenants(ctx.Request.Context(), q)
	if err != nil {
		tenantFail(ctx, err)
		return
	}
	if rows == nil {
		rows = []service.TenantWithActivity{}
	}
	response.Success(ctx, gin.H{"list": rows, "total": total}, "ok")
}

// UpdateTenantStatus 租户停用/启用
func (c *TenantController) UpdateTenantStatus(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidParams, "非法的租户 ID")
		return
	}
	var body struct {
		Status int16 `json:"status"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidParams, err.Error())
		return
	}
	if err := c.tenantService.UpdateTenantStatus(ctx.Request.Context(), uint(id), body.Status); err != nil {
		tenantFail(ctx, err)
		return
	}
	response.Success(ctx, gin.H{"tenant_id": id, "status": body.Status}, "ok")
}

// RegulatoryNotice 开户监管告知（未登录可取）
//
// 需求原文提到「法律要求的平台端监管责任」与「注意离线独立部署情况」，
// 因此这个端点在两条路径下都必须可用：
//   - 平台可达：从平台端取（两端同源，避免文案漂移）
//   - 离线部署：本地常量兜底，否则离线用户根本没法开户
//
// 挂在 public 档而不是 tenant/info 旁边的原因：注册页在登录**之前**就要展示
// 这段文本并要求勾选，那时用户还没有任何身份凭证。
func (c *TenantController) RegulatoryNotice(ctx *gin.Context) {
	view := service.NewTenantService().RegulatoryNotice(ctx.Request.Context())
	response.Success(ctx, gin.H{
		"notice":            view.Notice,
		"agreement_version": view.AgreementVersion,
		"required_fields":   view.RequiredFields,
	}, "ok")
}

// Registration 登记状态（登录后查：是否已上报平台 / 失败原因）
func (c *TenantController) Registration(ctx *gin.Context) {
	info, err := c.tenantService.RegistrationInfo(ctx.Request.Context())
	if err != nil {
		tenantFail(ctx, err)
		return
	}
	response.Success(ctx, info, "ok")
}

// RetrySync 补同步（超管）
func (c *TenantController) RetrySync(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidParams, "非法的租户 ID")
		return
	}
	if err := c.tenantService.RetryRegistrationSync(ctx.Request.Context(), uint(id)); err != nil {
		tenantFail(ctx, err)
		return
	}
	response.Success(ctx, nil, "已触发补同步")
}
