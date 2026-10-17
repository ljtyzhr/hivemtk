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
type moduleGateAdapter struct {
	gate *service.ModuleGateService
}

func (a moduleGateAdapter) IsEnabled(moduleKey string) bool {
	return a.gate.IsEnabled(context.Background(), moduleKey)
}

var _ = middleware.ModuleOwnerOf
