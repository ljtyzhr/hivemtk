package controller

import (
	"context"
	"strings"

	"hivemtk-user/internal/config"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// auth_register.go 自助注册 + 公开站点配置
//
// 背景：注册 API 曾整体下线（authorization_routes_gone_test 钉死 404），
// enable_register 沦为无消费点的死配置。本文件把自助注册接回来：
//   - POST /api/auth/register —— 受 system_config.enable_register 开关 gate，
//     关闭时 403；走 BruteForceGuard 防灌号
//   - GET  /api/public/config —— 未登录可取的最小公开配置（站点名/是否开放注册），
//     前端据此渲染注册入口
//
// 服务层复用 AuthService.Register（角色 user、唯一性/强度校验、注册即签发 JWT）。

// resolvePublicTenantCtx 公开入口的租户归属：站点配置与注册开关都是租户数据，
// 匿名请求没有 JWT 可依，只能由请求显式携带短码确定。
//
//   - off 模式：显式绑定默认租户，语义与旧版本等价；
//   - on 模式：`tenant_code`（query/body）或 X-Tenant-Code 头解析租户并钉死
//     绑定。解析不出来就返回错误——绝不在匿名请求上回落到默认租户，
//     否则 S 租户的公开页会读到默认租户的站点名/注册开关。
func resolvePublicTenantCtx(c *gin.Context, code string) (context.Context, error) {
	rctx := c.Request.Context()
	if !tenant.IsMultiTenant() {
		return tenant.ContextWithDefault(rctx), nil
	}
	code = strings.TrimSpace(code)
	if code == "" {
		code = strings.TrimSpace(c.GetHeader("X-Tenant-Code"))
	}
	id, err := service.NewTenantService().ResolvePublicTenantID(rctx, code)
	if err != nil {
		return rctx, err
	}
	return tenant.OnlyTenant(rctx, id), nil
}

// PublicConfig 未登录可取的最小站点配置
//
// @Summary      公开站点配置
// @Description  仅暴露站点名、注册开关、租户模式与云端版入口，不含任何敏感字段
// @Tags         Auth
// @Produce      json
// @Router       /api/public/config [get]
func (c *AuthController) PublicConfig(ctx *gin.Context) {
	// tenant_mode 与系统配置无关，两个分支都带上：前端据此决定是否渲染
	// 租户短码登录框（on 模式才显示），且读配置失败也不影响该判定。
	tenantMode := tenant.IsMultiTenant()
	// edition/cloud_url 同理（003 T003）：edition 是 TENANT_MODE 的语义映射
	// （前端不该猜开关名）；cloud_url 任何形态都要给可渲染值（离线回落占位常量）。
	edition := "opensource"
	if tenantMode {
		edition = "saas"
	}
	requestCtx, err := resolvePublicTenantCtx(ctx, ctx.Query("tenant_code"))
	if err != nil {
		// 租户未知（on 模式未带短码）按保守值返回：站点名空、注册关闭——
		// 前端隐藏入口而非误开放，也绝不拿默认租户的配置冒充本租户。
		response.Success(ctx, gin.H{
			"site_name": "", "enable_register": false, "tenant_mode": tenantMode,
			"edition": edition, "cloud_url": config.CloudURLFor(requestCtx),
		}, "ok")
		return
	}
	cfg, err := service.NewSystemConfigService().GetConfig(requestCtx)
	if err != nil {
		// 读失败按保守值返回：站点名空、注册关闭——前端隐藏入口而非误开放
		response.Success(ctx, gin.H{
			"site_name": "", "enable_register": false, "tenant_mode": tenantMode,
			"edition": edition, "cloud_url": config.CloudURLFor(requestCtx),
		}, "ok")
		return
	}
	response.Success(ctx, gin.H{
		"site_name":       cfg.Name,
		"enable_register": cfg.EnableRegister,
		"tenant_mode":     tenantMode,
		"edition":         edition,
		"cloud_url":       config.CloudURLFor(requestCtx),
	}, "ok")
}
