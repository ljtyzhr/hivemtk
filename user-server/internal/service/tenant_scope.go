package service

import (
	"context"
	"fmt"

	"hivemtk-user/internal/pkg/tenant"
)

// tenantScopeID 返回进程内缓存/键空间应使用的租户标识。
//
// off 模式回落默认租户（历史单机行为）；on 模式下 ctx 没有租户时返回 0。
// 0 是一个独立命名空间：既不读默认租户的数据，也不会把两个租户混进同一格缓存。
func tenantScopeID(ctx context.Context) uint {
	if id, ok := tenant.FromContext(ctx); ok {
		return id
	}
	if !tenant.IsMultiTenant() {
		return tenant.DefaultTenantID
	}
	return 0
}

// tenantScopedKey 给进程内缓存键加租户前缀（on 模式缺租户时落在 t0 独立空间）。
func tenantScopedKey(ctx context.Context, key string) string {
	return fmt.Sprintf("t%d|%s", tenantScopeID(ctx), key)
}
