package app

import (
	"context"
	"testing"

	"hivemtk-user/internal/config"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/service"
)

var wantURLsWired = []string{
	"urls.cloud_url",
	"urls.geo_site_base_url",
	"urls.public_base_url",
}

// resetURLProviders 复位三个 config 侧 provider。
// 不能传 nil：SetXxxProvider(nil) 按设计是空操作（防装配顺序顶掉兜底），
// 装过真 provider 的用例会跨用例泄漏。
func resetURLProviders(t *testing.T) {
	t.Helper()
	empty := func(context.Context) string { return "" }
	config.SetCloudURLProvider(empty)
	config.SetWebsiteBaseURLProvider(empty)
	config.SetPublicBaseURLProvider(empty)
	t.Cleanup(func() {
		config.SetCloudURLProvider(empty)
		config.SetWebsiteBaseURLProvider(empty)
		config.SetPublicBaseURLProvider(empty)
	})
}

func newURLParamFixture(t *testing.T) *service.ConfigParamService {
	t.Helper()
	resetURLProviders(t)
	database := testutil.NewTestDBOrSkip(t,
		&model.ConfigParam{},
		&model.ConfigParamAuditLog{},
		&model.TenantConfigParam{},
		&model.TenantConfigParamAuditLog{},
	)
	if database == nil {
		t.Skip("no DB")
	}
	if err := service.SeedConfigParams(context.Background(), database); err != nil {
		t.Fatalf("SeedConfigParams 失败：%v", err)
	}
	svc := service.NewConfigParamService(database)
	service.SetGlobalForTest(svc)
	t.Cleanup(func() { service.SetGlobalForTest(nil) })
	return svc
}

func TestWireURLsConfigParamsKeys(t *testing.T) {
	resetURLProviders(t)
	got := WireURLsConfigParams()
	if len(got) != len(wantURLsWired) {
		t.Fatalf("接线条数 = %d, 期望 %d\ngot  = %v\nwant = %v", len(got), len(wantURLsWired), got, wantURLsWired)
	}
	for i, k := range got {
		if k != wantURLsWired[i] {
			t.Errorf("第 %d 条接线 = %q, 期望 %q", i, k, wantURLsWired[i])
		}
	}
}

func TestWiredURLsKeysExistInSeed(t *testing.T) {
	index := map[string]bool{}
	for _, d := range service.DefaultParamDefs() {
		index[d.Group+"."+d.Key] = true
	}
	for _, k := range wantURLsWired {
		if !index[k] {
			t.Errorf("%q 在 DefaultParamDefs() 里不存在：接了线但没有参数行", k)
		}
	}
}

// 装配完之后，参数中心的值必须真的出现在读取点上（CloudURL / WebsiteBaseURL / GetPublicBaseURL）。
func TestWireURLsConfigParamsReachesReader(t *testing.T) {
	svc := newURLParamFixture(t)
	ctx := context.Background()
	if err := svc.UpdateValue(ctx, "urls", "cloud_url", "https://changed.example.com", 1); err != nil {
		t.Fatalf("改 urls.cloud_url 失败：%v", err)
	}
	if err := svc.UpdateValue(ctx, "urls", "geo_site_base_url", "https://changed-site.example.com", 1); err != nil {
		t.Fatalf("改 urls.geo_site_base_url 失败：%v", err)
	}
	if err := svc.UpdateValue(ctx, "urls", "public_base_url", "https://changed-public.example.com", 1); err != nil {
		t.Fatalf("改 urls.public_base_url 失败：%v", err)
	}
	t.Setenv("PLATFORM_CLOUD_URL", "")
	t.Setenv("GEO_SITE_BASE_URL", "")
	t.Setenv("PUBLIC_BASE_URL", "")
	old := config.GetAppConfig()
	config.SetAppConfig(&config.AppConfig{})
	t.Cleanup(func() { config.SetAppConfig(&old) })
	WireURLsConfigParams()
	if got := config.CloudURL(); got != "https://changed.example.com" {
		t.Errorf("CloudURL() = %q, 期望参数值 https://changed.example.com", got)
	}
	if got := config.WebsiteBaseURL(); got != "https://changed-site.example.com" {
		t.Errorf("WebsiteBaseURL() = %q, 期望参数值 https://changed-site.example.com", got)
	}
	if got := config.GetPublicBaseURL(); got != "https://changed-public.example.com" {
		t.Errorf("GetPublicBaseURL() = %q, 期望参数值 https://changed-public.example.com", got)
	}
}

// R5-3：SaaS 模式下租户覆盖必须真的走到 config 读取口——接线时用 context.Background()
// 拍平过一次，这条判据就会一直绿着坏（A 改了参数，B 与平台默认都不受影响，但读不到 A）。
func TestWireURLsConfigParamsTenantOverride(t *testing.T) {
	svc := newURLParamFixture(t)
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	ctxA := tenant.WithTenantID(context.Background(), 71)
	ctxB := tenant.WithTenantID(context.Background(), 72)
	if err := svc.UpdateValue(ctxA, "urls", "cloud_url", "https://a.example.com", 1); err != nil {
		t.Fatalf("A 覆盖 urls.cloud_url 失败：%v", err)
	}
	if err := svc.UpdateValue(ctxA, "urls", "geo_site_base_url", "https://a-site.example.com", 1); err != nil {
		t.Fatalf("A 覆盖 urls.geo_site_base_url 失败：%v", err)
	}
	if err := svc.UpdateValue(ctxA, "urls", "public_base_url", "a-chat.example.com", 1); err != nil {
		t.Fatalf("A 覆盖 urls.public_base_url 失败：%v", err)
	}

	t.Setenv("PLATFORM_CLOUD_URL", "")
	t.Setenv("GEO_SITE_BASE_URL", "")
	t.Setenv("PUBLIC_BASE_URL", "")
	old := config.GetAppConfig()
	config.SetAppConfig(&config.AppConfig{})
	t.Cleanup(func() { config.SetAppConfig(&old) })
	WireURLsConfigParams()

	if got := config.CloudURLFor(ctxA); got != "https://a.example.com" {
		t.Errorf("A 的云端地址 = %q, 期望自己的覆盖值 https://a.example.com", got)
	}
	if got := config.WebsiteBaseURLFor(ctxA); got != "https://a-site.example.com" {
		t.Errorf("A 的官网基址 = %q, 期望自己的覆盖值 https://a-site.example.com", got)
	}
	// public_base_url 走 NormalizePublicBaseURL：无 scheme 补 https
	if got := config.GetPublicBaseURLFor(ctxA); got != "https://a-chat.example.com" {
		t.Errorf("A 的公网基址 = %q, 期望 https://a-chat.example.com", got)
	}
	// B 未覆盖 ⇒ 读平台默认（cloud/site 有兜底值，public 平台默认留空）
	if got := config.CloudURLFor(ctxB); got != "https://work.xapptool.cn" {
		t.Errorf("B 不得读到 A 的覆盖值，期望平台默认，实际 %q", got)
	}
	if got := config.GetPublicBaseURLFor(ctxB); got != "" {
		t.Errorf("B 未覆盖时公网基址应为空（调用方回退请求 Host），实际 %q", got)
	}
	// 无租户 ctx（平台级调用）同样读平台默认
	if got := config.CloudURL(); got != "https://work.xapptool.cn" {
		t.Errorf("无租户 ctx 应读平台默认，实际 %q", got)
	}
}

// 无库时接线不该 panic，也不该依赖库是否可用（provider 仍被装上，只是读到兜底）。
func TestWireURLsConfigParamsWithoutDB(t *testing.T) {
	resetURLProviders(t)
	service.SetGlobalForTest(nil)
	if got := WireURLsConfigParams(); len(got) != len(wantURLsWired) {
		t.Fatalf("无库时接线条数 = %d, 期望 %d", len(got), len(wantURLsWired))
	}
	t.Setenv("PLATFORM_CLOUD_URL", "")
	t.Setenv("GEO_SITE_BASE_URL", "")
	t.Setenv("PUBLIC_BASE_URL", "")
	old := config.GetAppConfig()
	config.SetAppConfig(&config.AppConfig{})
	t.Cleanup(func() { config.SetAppConfig(&old) })
	if got := config.CloudURL(); got != config.DefaultCloudURL {
		t.Errorf("无库时 CloudURL() = %q, 期望回落常量 %q", got, config.DefaultCloudURL)
	}
	if got := config.WebsiteBaseURL(); got != config.DefaultWebsiteBaseURL {
		t.Errorf("无库时 WebsiteBaseURL() = %q, 期望回落常量 %q", got, config.DefaultWebsiteBaseURL)
	}
	if got := config.GetPublicBaseURL(); got != "" {
		t.Errorf("无库时 GetPublicBaseURL() = %q, 期望空串（调用方回退请求 Host）", got)
	}
}
