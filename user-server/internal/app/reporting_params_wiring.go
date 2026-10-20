package app

import (
	"context"

	"hivemtk-user/internal/reporting"
	appsvc "hivemtk-user/internal/service"
)

// WireReportingConfigParams 注入 reporting 组参数读取器。
//
// reporting.default_window_days 收口全仓报表/列表入口在调用方没给 days 时的默认
// 窗口语义（原先十几处各自写死 30）。点位横跨 repository / controller / geo /
// knowledge / ops 等不能反向 import service 的包，故走 provider 注入。
//
// 每请求热读：provider 闭包不接 ctx 会丢请求级取消信息，这里接 ctx 直通
// （ConfigParamService 有 60s 内存缓存，热读成本可控）。
// GetInt 与键名字面量必须写在同一行：scripts/check-config-param-readpoints.py
// 认的就是这个形状。
func WireReportingConfigParams() []string {
	cp := appsvc.GlobalConfigParam()

	reporting.SetDefaultWindowDaysProvider(func(ctx context.Context) int {
		return cp.GetInt(ctx, "reporting", "default_window_days", reporting.DefaultWindowDaysValue)
	})

	return []string{"reporting.default_window_days"}
}
