package app

import (
	"context"

	"hivemtk-user/internal/config"
	appsvc "hivemtk-user/internal/service"
)

// WireURLsConfigParams 注入 urls 组的 config 侧参数中心取值口
// （cloud_url / geo_site_base_url / public_base_url）。
//
// 收录标准：读取点在 internal/config（config 不能反向依赖 service），只能由装配层
// 注入 provider。urls.frontend_url 不在这里——它的读取点在 service 本体
// （password_reset.go 的 getFrontendBaseURL），走「读取函数自己调 GlobalConfigParam()」
// 的直连范式，不需要注入。
//
// 每条都写成显式闭包，且 GetString 与键名字面量必须在同一行：
// scripts/check-config-param-readpoints.py 认的是这个形状。
//
// 闭包 provider（不注入启动瞬间的值）：每次调用都现读参数中心，值改完 60s TTL 缓存
// 过期后即生效，不需要重启。ctx 原样透传，SaaS 模式绑定租户后读到本租户覆盖值；
// 平台级调用（无租户 ctx）回落平台默认值。
func WireURLsConfigParams() []string {
	cp := appsvc.GlobalConfigParam()
	wired := make([]string, 0, 3)

	config.SetCloudURLProvider(func(ctx context.Context) string {
		return cp.GetString(ctx, "urls", "cloud_url", config.DefaultCloudURL)
	})
	wired = append(wired, "urls.cloud_url")

	config.SetWebsiteBaseURLProvider(func(ctx context.Context) string {
		return cp.GetString(ctx, "urls", "geo_site_base_url", config.DefaultWebsiteBaseURL)
	})
	wired = append(wired, "urls.geo_site_base_url")

	config.SetPublicBaseURLProvider(func(ctx context.Context) string {
		return cp.GetString(ctx, "urls", "public_base_url", "")
	})
	wired = append(wired, "urls.public_base_url")

	return wired
}
