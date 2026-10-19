package platform

import (
	"context"

	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/repository"
)

// TenantCodeFromContext 解析当前上下文租户的短码（平台交互用：LLM 钱包/流水
// query、资产购买 body、LLM 代理调用 query、下单 body）。口径与
// service.currentTenantCode（internal/service/module_catalog.go）**逐字一致**：
//   - ctx 无租户绑定（后台 runner、裸 Background）→ 空串；
//   - off 模式（TenantMiddleware 绑默认租户但 tenants 表无行）/ on 模式查不到
//     该 id 的行 → 空串；
//   - 查库失败（含 db 未就绪）→ 空串（短码是给平台的归属提示，取不到就走
//     平台旧行为兜底，绝不允许把钱包/购买/代理调用整条链路打挂）。
//
// 为什么放在 platform 而不是 service：aiagent（LLM 代理调用）与 service
// 两处都要这个口径，而 aiagent 是叶子包（不得 import repository/service）；
// platform 已经依赖 repository 与 tenant，放这里两边都能取且不会成环。
//
// tenants 表是全局表（无 tenant_id 列、不进隔离登记表），所以挂 SystemScope
// 的直查不受隔离 callback fail-closed 影响。
func TenantCodeFromContext(ctx context.Context) string {
	id, ok := tenant.FromContext(ctx)
	if !ok || db.GetDB() == nil {
		return ""
	}
	t, err := repository.NewTenantRepository().GetByID(tenant.WithSystemScope(ctx), id)
	if err != nil || t == nil {
		return ""
	}
	return t.Code
}
