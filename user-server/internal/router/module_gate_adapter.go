package router

import (
	"context"

	"hivemtk-user/internal/middleware"
	"hivemtk-user/internal/service"
)

// moduleGateAdapter 把服务层的闸接到中间件的窄接口上。
//
// 为什么要有这一层：middleware 不能 import service（service 依赖大量业务包，
// 反向依赖会成环且把整个 service 层拖进中间件的依赖面）。这里在 router 包
// 做一次转接 —— router 本来就是装配层，依赖 service 是它的本职。
//
// ctx 直通不加工：TenantMiddleware 已把调用方租户写进 request ctx，
// 中间件把它传到这里，这里原样递给 service —— 闸按 ctx 解析租户、取**该租户**
// 的缓存条目判定。任何一层换成 context.Background() 都会让 on 模式下非默认
// 租户的请求按默认租户视图判定：该放的被拦、该拦的被放（判定张冠李戴）。
type moduleGateAdapter struct {
	gate *service.ModuleGateService
}

func (a moduleGateAdapter) IsEnabled(ctx context.Context, moduleKey string) bool {
	return a.gate.IsEnabled(ctx, moduleKey)
}

var _ = middleware.ModuleOwnerOf
