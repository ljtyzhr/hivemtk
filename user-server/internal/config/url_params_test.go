package config

import (
	"context"
	"strconv"
	"testing"

	"hivemtk-user/internal/pkg/tenant"
)

// withEmptyURLProviders 把三个 URL provider 置为「参数中心无值」。
//
// 为什么不能靠 SetXxxProvider(nil) 复位：nil 按设计是空操作（防装配顺序把兜底顶掉），
// 于是装过真 provider 的用例会跨用例泄漏——website_base_test.go 那句「未设置覆盖变量时
// 应取默认基址」读到的就是上一条用例装的租户 provider。
func withEmptyURLProviders(t *testing.T) {
	t.Helper()
	empty := func(context.Context) string { return "" }
	SetCloudURLProvider(empty)
	SetWebsiteBaseURLProvider(empty)
	SetPublicBaseURLProvider(empty)
	t.Cleanup(func() {
		SetCloudURLProvider(empty)
		SetWebsiteBaseURLProvider(empty)
		SetPublicBaseURLProvider(empty)
	})
}

// TestCloudURLProviderInjection 注入 provider 之后 CloudURL 必须返回参数值；
// 这证明「运维改参数真的改变了对外展示的云端地址」，而不是只验证 setter 收下了闭包。
func TestCloudURLProviderInjection(t *testing.T) {
	withEmptyURLProviders(t)
	// 确保不受 env 影响。
	t.Setenv("PLATFORM_CLOUD_URL", "")
	SetCloudURLProvider(func(context.Context) string { return "https://param.example.com" })
	if got := CloudURL(); got != "https://param.example.com" {
		t.Errorf("CloudURL() = %q, 期望参数值 https://param.example.com", got)
	}
}

// TestCloudURLParamEmptyFallsToConst 参数返回空串时必须回落常量（行为零变更）。
func TestCloudURLParamEmptyFallsToConst(t *testing.T) {
	withEmptyURLProviders(t)
	t.Setenv("PLATFORM_CLOUD_URL", "")
	if got := CloudURL(); got != DefaultCloudURL {
		t.Errorf("CloudURL() = %q, 期望回落常量 %q", got, DefaultCloudURL)
	}
}

// TestCloudURLProviderNilIsNoOp 传 nil 是空操作：已装配的 provider 不能被顶掉，
// 否则装配顺序出错会把参数中心静默降级成常量兜底。
func TestCloudURLProviderNilIsNoOp(t *testing.T) {
	withEmptyURLProviders(t)
	t.Setenv("PLATFORM_CLOUD_URL", "")
	SetCloudURLProvider(func(context.Context) string { return "https://keep.example.com" })
	SetCloudURLProvider(nil)
	if got := CloudURL(); got != "https://keep.example.com" {
		t.Errorf("传 nil 后 CloudURL() = %q, 期望保留已装配的 https://keep.example.com", got)
	}
}

// TestWebsiteBaseURLProviderInjection 注入 provider 之后 WebsiteBaseURL 必须返回参数值，
// 且结尾 '/' 被裁掉（供调用方直接 + path）。
func TestWebsiteBaseURLProviderInjection(t *testing.T) {
	withEmptyURLProviders(t)
	t.Setenv("GEO_SITE_BASE_URL", "")
	SetWebsiteBaseURLProvider(func(context.Context) string { return "https://site.example.com/" })
	if got := WebsiteBaseURL(); got != "https://site.example.com" {
		t.Errorf("WebsiteBaseURL() = %q, 期望去掉结尾斜杠的 https://site.example.com", got)
	}
}

// TestWebsiteBaseURLParamEmptyFallsToConst 参数返回空串时必须回落常量（行为零变更）。
func TestWebsiteBaseURLParamEmptyFallsToConst(t *testing.T) {
	withEmptyURLProviders(t)
	t.Setenv("GEO_SITE_BASE_URL", "")
	if got := WebsiteBaseURL(); got != DefaultWebsiteBaseURL {
		t.Errorf("WebsiteBaseURL() = %q, 期望回落常量 %q", got, DefaultWebsiteBaseURL)
	}
}

// tenantProvider 模拟参数中心的租户覆盖：ctx 绑定租户返回本租户值，未绑定返回平台默认。
func tenantProvider(marker string) func(context.Context) string {
	return func(ctx context.Context) string {
		if id, ok := tenant.FromContext(ctx); ok {
			return "https://" + marker + "-t" + strconv.FormatUint(uint64(id), 10) + ".example.com"
		}
		return "https://" + marker + "-platform.example.com"
	}
}

// TestURLGettersForwardCallerContext R5-3 的核心判据：三个 URL 读取口必须把调用方 ctx
// 原样透传给 provider，绑定哪个租户就返回哪个租户的值——
// 否则「参数中心按租户覆盖」在 config 这一层就被 context.Background() 拍平成平台默认。
func TestURLGettersForwardCallerContext(t *testing.T) {
	withEmptyURLProviders(t)
	t.Setenv("PLATFORM_CLOUD_URL", "")
	t.Setenv("GEO_SITE_BASE_URL", "")
	t.Setenv("PUBLIC_BASE_URL", "")
	old := GetAppConfig()
	SetAppConfig(&AppConfig{})
	t.Cleanup(func() { SetAppConfig(&old) })

	ctxA := tenant.WithTenantID(context.Background(), 71)
	ctxB := tenant.WithTenantID(context.Background(), 72)

	SetCloudURLProvider(tenantProvider("cloud"))
	SetWebsiteBaseURLProvider(tenantProvider("site"))
	SetPublicBaseURLProvider(tenantProvider("public"))

	for _, tc := range []struct {
		name   string
		marker string
		a, b   string
		plain  string
	}{
		{"urls.cloud_url", "cloud", CloudURLFor(ctxA), CloudURLFor(ctxB), CloudURL()},
		{"urls.geo_site_base_url", "site", WebsiteBaseURLFor(ctxA), WebsiteBaseURLFor(ctxB), WebsiteBaseURL()},
		{"urls.public_base_url", "public", GetPublicBaseURLFor(ctxA), GetPublicBaseURLFor(ctxB), GetPublicBaseURL()},
	} {
		if tc.a == tc.b {
			t.Errorf("%s: 租户 A/B 读到同一个值 %q，调用方 ctx 没透传", tc.name, tc.a)
		}
		if want := "https://" + tc.marker + "-t71.example.com"; tc.a != want {
			t.Errorf("%s: 租户 A 应读自己的覆盖值 %q，实际 %q", tc.name, want, tc.a)
		}
		if want := "https://" + tc.marker + "-platform.example.com"; tc.plain != want {
			t.Errorf("%s: 无租户 ctx 应读平台默认 %q，实际 %q", tc.name, want, tc.plain)
		}
	}
}

// TestIsSelfSiteURLForUsesCallerTenantBase IsSelfSiteURLFor 必须按 ctx 解析基址：
// 否则自家站判定永远拿平台默认基址，租户自定义官网基址下的链接会被误判成"别人的站"。
func TestIsSelfSiteURLForUsesCallerTenantBase(t *testing.T) {
	withEmptyURLProviders(t)
	t.Setenv("GEO_SITE_BASE_URL", "")
	SetWebsiteBaseURLProvider(tenantProvider("site"))

	ctxA := tenant.WithTenantID(context.Background(), 81)
	if !IsSelfSiteURLFor(ctxA, "https://site-t81.example.com/features") {
		t.Error("租户 A 自己基址下的页面必须判为本站")
	}
	if IsSelfSiteURL("https://site-t81.example.com/features") {
		t.Error("无租户 ctx 读平台默认基址，不得把 A 的站判成自家")
	}
}
