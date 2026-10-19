package config

import "testing"

// TestCloudURLProviderInjection 注入 provider 之后 CloudURL 必须返回参数值；
// 这证明「运维改参数真的改变了对外展示的云端地址」，而不是只验证 setter 收下了闭包。
func TestCloudURLProviderInjection(t *testing.T) {
	t.Cleanup(func() { SetCloudURLProvider(nil) })
	// 确保不受 env 影响。
	t.Setenv("PLATFORM_CLOUD_URL", "")
	SetCloudURLProvider(func() string { return "https://param.example.com" })
	if got := CloudURL(); got != "https://param.example.com" {
		t.Errorf("CloudURL() = %q, 期望参数值 https://param.example.com", got)
	}
}

// TestCloudURLParamEmptyFallsToConst 参数返回空串时必须回落常量（行为零变更）。
func TestCloudURLParamEmptyFallsToConst(t *testing.T) {
	t.Cleanup(func() { SetCloudURLProvider(nil) })
	t.Setenv("PLATFORM_CLOUD_URL", "")
	SetCloudURLProvider(func() string { return "" })
	if got := CloudURL(); got != DefaultCloudURL {
		t.Errorf("CloudURL() = %q, 期望回落常量 %q", got, DefaultCloudURL)
	}
}

// TestCloudURLProviderDefaultIsConst 未注入 provider（默认）时回落常量。
func TestCloudURLProviderDefaultIsConst(t *testing.T) {
	t.Cleanup(func() { SetCloudURLProvider(nil) })
	SetCloudURLProvider(nil)
	t.Setenv("PLATFORM_CLOUD_URL", "")
	if got := CloudURL(); got != DefaultCloudURL {
		t.Errorf("CloudURL() = %q, 期望默认回落常量 %q", got, DefaultCloudURL)
	}
}

// TestWebsiteBaseURLProviderInjection 注入 provider 之后 WebsiteBaseURL 必须返回参数值，
// 且结尾 '/' 被裁掉（供调用方直接 + path）。
func TestWebsiteBaseURLProviderInjection(t *testing.T) {
	t.Cleanup(func() { SetWebsiteBaseURLProvider(nil) })
	t.Setenv("GEO_SITE_BASE_URL", "")
	SetWebsiteBaseURLProvider(func() string { return "https://site.example.com/" })
	if got := WebsiteBaseURL(); got != "https://site.example.com" {
		t.Errorf("WebsiteBaseURL() = %q, 期望去掉结尾斜杠的 https://site.example.com", got)
	}
}

// TestWebsiteBaseURLParamEmptyFallsToConst 参数返回空串时必须回落常量（行为零变更）。
func TestWebsiteBaseURLParamEmptyFallsToConst(t *testing.T) {
	t.Cleanup(func() { SetWebsiteBaseURLProvider(nil) })
	t.Setenv("GEO_SITE_BASE_URL", "")
	SetWebsiteBaseURLProvider(func() string { return "" })
	if got := WebsiteBaseURL(); got != DefaultWebsiteBaseURL {
		t.Errorf("WebsiteBaseURL() = %q, 期望回落常量 %q", got, DefaultWebsiteBaseURL)
	}
}
