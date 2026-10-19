package app

import (
	"context"
	"testing"

	"hivemtk-user/internal/config"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/service"
)

var wantURLsWired = []string{
	"urls.cloud_url",
	"urls.geo_site_base_url",
}

func resetURLProviders(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { config.SetCloudURLProvider(nil) })
	t.Cleanup(func() { config.SetWebsiteBaseURLProvider(nil) })
}

func newURLParamFixture(t *testing.T) *service.ConfigParamService {
	t.Helper()
	resetURLProviders(t)
	database := testutil.NewTestDBOrSkip(t, &model.ConfigParam{}, &model.ConfigParamAuditLog{})
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

// 装配完之后，参数中心的值必须真的出现在读取点上（CloudURL / WebsiteBaseURL）。
func TestWireURLsConfigParamsReachesReader(t *testing.T) {
	svc := newURLParamFixture(t)
	ctx := context.Background()
	if err := svc.UpdateValue(ctx, "urls", "cloud_url", "https://changed.example.com", 1); err != nil {
		t.Fatalf("改 urls.cloud_url 失败：%v", err)
	}
	if err := svc.UpdateValue(ctx, "urls", "geo_site_base_url", "https://changed-site.example.com", 1); err != nil {
		t.Fatalf("改 urls.geo_site_base_url 失败：%v", err)
	}
	t.Setenv("PLATFORM_CLOUD_URL", "")
	t.Setenv("GEO_SITE_BASE_URL", "")
	WireURLsConfigParams()
	if got := config.CloudURL(); got != "https://changed.example.com" {
		t.Errorf("CloudURL() = %q, 期望参数值 https://changed.example.com", got)
	}
	if got := config.WebsiteBaseURL(); got != "https://changed-site.example.com" {
		t.Errorf("WebsiteBaseURL() = %q, 期望参数值 https://changed-site.example.com", got)
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
	if got := config.CloudURL(); got != config.DefaultCloudURL {
		t.Errorf("无库时 CloudURL() = %q, 期望回落常量 %q", got, config.DefaultCloudURL)
	}
	if got := config.WebsiteBaseURL(); got != config.DefaultWebsiteBaseURL {
		t.Errorf("无库时 WebsiteBaseURL() = %q, 期望回落常量 %q", got, config.DefaultWebsiteBaseURL)
	}
}
