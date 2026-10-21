package controller

import (
	"context"
	"strings"

	"hivemtk-user/internal/pkg/tenant"

	"github.com/gin-gonic/gin"
)

// publicWebhookTenantCtx 公开 webhook 的租户解析。
//
//   - off 模式：显式绑定默认租户，语义与旧版本等价；
//   - on 模式：只认显式 tenant_code(query) / X-Tenant-Code 头，传了但解析失败即报错；
//   - 没带线索时保持无绑定，交由服务层按行自证（如短信回执的 message_id）；
//     服务层也解析不出时由隔离 callback fail-closed，绝不回落默认租户。
func publicWebhookTenantCtx(c *gin.Context) (context.Context, error) {
	rctx := c.Request.Context()
	if !tenant.IsMultiTenant() {
		return tenant.ContextWithDefault(rctx), nil
	}
	hint := strings.TrimSpace(c.Query("tenant_code"))
	if hint == "" {
		hint = strings.TrimSpace(c.GetHeader("X-Tenant-Code"))
	}
	if hint == "" {
		return rctx, nil
	}
	return resolvePublicTenantCtx(c, hint)
}
