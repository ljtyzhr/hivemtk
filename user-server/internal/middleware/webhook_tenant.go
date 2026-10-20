package middleware

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"

	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/utils/logger"
)

// 渠道回调是公网未认证入口：没有登录态，租户只能由 URL 自带的
// (channel, account_id) 自证。绑定动作放在组级，是因为同一个租户线索被
// 两种 handler 消费——通用漏斗 Receive 与各家 URL 验证挑战（wecom/feishu/
// whatsapp/dingtalk 的 GET），只在其中一处绑定的话，另一处在 on 模式下
// 必然被隔离 callback 打成"账号不存在"，而报文的签名本来是对的。
//
// 解析失败时**不绑定**而不是报错退出：下游读租户表会 fail-closed，控制器
// 那侧仍按既有 404/401 分支回绝，行为与本中间件在场前一致。

const webhookPathPrefix = "/api/webhook/"

// WebhookTenantResolver 由 (渠道段, 账号段) 反查归属租户，解析不出返回错误。
type WebhookTenantResolver func(ctx context.Context, channel, accountID string) (uint, error)

// webhookLiteralChannel 取 "/api/webhook/wecom/:account_id" 里的字面量渠道段。
//
// 通配段（":channel"）与运维端点（stats/health）返回空串 ⇒ 不绑定：前者由
// c.Param("channel") 直接给出，后两者压根没有渠道语义。
func webhookLiteralChannel(fullPath string) string {
	rest := strings.TrimPrefix(fullPath, webhookPathPrefix)
	if rest == fullPath {
		return ""
	}
	if i := strings.Index(rest, "/"); i >= 0 {
		rest = rest[:i]
	}
	if rest == "" || strings.HasPrefix(rest, ":") {
		return ""
	}
	switch rest {
	case "stats", "health", "healthz", "readyz":
		return ""
	}
	return rest
}

// WebhookTenantBinding 为 /api/webhook/* 的渠道回调绑定归属租户。
func WebhookTenantBinding(resolve WebhookTenantResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !tenant.IsMultiTenant() || resolve == nil {
			c.Next()
			return
		}
		channel := c.Param("channel")
		if channel == "" {
			channel = webhookLiteralChannel(c.FullPath())
		}
		if channel == "" {
			c.Next()
			return
		}
		accountID := c.Param("account_id")
		if _, bound := tenant.FromContext(c.Request.Context()); bound {
			c.Next()
			return
		}
		tenantID, err := resolve(c.Request.Context(), channel, accountID)
		if err != nil {
			logger.Warnf("[tenant] 渠道回调租户解析拒绝 channel=%s account=%s path=%s: %v",
				channel, accountID, c.FullPath(), err)
			c.Next()
			return
		}
		if tenantID == 0 {
			c.Next()
			return
		}
		// OnlyTenant：回调报文自带的租户是权威归属，不接受任何请求头/查询参数覆写。
		c.Request = c.Request.WithContext(tenant.OnlyTenant(c.Request.Context(), tenantID))
		c.Next()
	}
}
