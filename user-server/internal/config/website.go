package config

import (
	"os"
	"strings"
)

// websiteBaseURLProvider 是 urls.geo_site_base_url 参数中心取值口（R2 参数化）。
// 默认返回空串 = 未装配，WebsiteBaseURL() 会继续回落到 env/常量，等价于接线前行为。
//
// 做成 provider 而不是在这里调 config_param service：config 不能反向依赖 service
// （service 层 import 了 config，反向会成环），装配点在 internal/app/url_params_wiring.go。
var websiteBaseURLProvider = func() string { return "" }

// SetWebsiteBaseURLProvider 注入 urls.geo_site_base_url 参数中心取值口。传 nil 是空操作而不是复位。
func SetWebsiteBaseURLProvider(fn func() string) {
	if fn != nil {
		websiteBaseURLProvider = fn
	}
}

// WebsiteBaseURL 官网基址（运行期唯一出口，拼接站内路径都走这里）。
//
// 覆盖：GEO_SITE_BASE_URL > 参数中心 urls.geo_site_base_url > DefaultWebsiteBaseURL。
// 结尾恒不带 '/'，供调用方直接 + "/features" 而不出 "//features"。
func WebsiteBaseURL() string {
	if v := strings.TrimSpace(os.Getenv("GEO_SITE_BASE_URL")); v != "" {
		return strings.TrimRight(v, "/")
	}
	if v := strings.TrimSpace(websiteBaseURLProvider()); v != "" {
		return strings.TrimRight(v, "/")
	}
	return strings.TrimRight(DefaultWebsiteBaseURL, "/")
}

// IsSelfSiteURL 判断一个 URL 是否指向本系统自己的官网。
//
// 判据是"基址 + 路径边界"而不是 host：GitHub Pages 项目页形态下，同一 host 上
// 还住着无数别人的项目（xiaofang142.github.io/someone-else/），只比 host 会把它们
// 全算成自家；而 "hivemtkevil" 这类同前缀不同路径又会躲过裸前缀匹配。
func IsSelfSiteURL(rawURL string) bool {
	base := WebsiteBaseURL()
	if base == "" || rawURL == "" {
		return false
	}
	return rawURL == base || strings.HasPrefix(rawURL, base+"/")
}
