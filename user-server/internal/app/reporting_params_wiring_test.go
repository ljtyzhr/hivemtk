package app

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/reporting"
	"hivemtk-user/internal/service"
)

var wantReportingWired = []string{
	"reporting.default_window_days",
}

func resetReportingProvider(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { reporting.SetDefaultWindowDaysProvider(nil) })
}

// 接线条数与键名对账：写漏一条只有启动日志能发现，这里把它变成测试红。
func TestWireReportingConfigParamsKeys(t *testing.T) {
	resetReportingProvider(t)
	got := WireReportingConfigParams()
	if len(got) != len(wantReportingWired) {
		t.Fatalf("接线条数 = %d, 期望 %d\ngot  = %v\nwant = %v", len(got), len(wantReportingWired), got, wantReportingWired)
	}
	for i, k := range got {
		if k != wantReportingWired[i] {
			t.Errorf("第 %d 条接线 = %q, 期望 %q", i, k, wantReportingWired[i])
		}
	}
}

// 接了线但种子没有对应参数行 = 每次读都 miss、永远回落兜底，运维在页面上还看不到旋钮。
func TestWireReportingConfigParamsKeysExistInSeed(t *testing.T) {
	index := map[string]bool{}
	for _, d := range service.DefaultParamDefs() {
		index[d.Group+"."+d.Key] = true
	}
	for _, k := range wantReportingWired {
		if !index[k] {
			t.Errorf("%q 在 DefaultParamDefs() 里不存在：接了线但没有参数行", k)
		}
	}
}

// 装配完之后，参数中心的值必须真的出现在读取点上（reporting.DefaultWindowDays）。
func TestWireReportingConfigParamsReachesReader(t *testing.T) {
	resetReportingProvider(t)
	service.SetGlobalForTest(nil)
	database := testutil.NewTestDBOrSkip(t, &model.ConfigParam{}, &model.ConfigParamAuditLog{})
	if database == nil {
		t.Skip("no DB")
	}
	if err := service.SeedConfigParams(context.Background()); err != nil {
		t.Fatalf("SeedConfigParams 失败：%v", err)
	}
	svc := service.NewConfigParamService()
	service.SetGlobalForTest(svc)
	t.Cleanup(func() { service.SetGlobalForTest(nil) })

	ctx := context.Background()
	if err := svc.UpdateValue(ctx, "reporting", "default_window_days", "7", 1); err != nil {
		t.Fatalf("改 reporting.default_window_days 失败：%v", err)
	}
	WireReportingConfigParams()
	if got := reporting.DefaultWindowDays(ctx); got != 7 {
		t.Errorf("DefaultWindowDays() = %d, 期望参数值 7", got)
	}
}

// 无库时接线不该 panic，读取点回落编译期兜底 30（行为零变更）。
func TestWireReportingConfigParamsWithoutDB(t *testing.T) {
	resetReportingProvider(t)
	service.SetGlobalForTest(nil)
	if got := WireReportingConfigParams(); len(got) != len(wantReportingWired) {
		t.Fatalf("无库时接线条数 = %d, 期望 %d", len(got), len(wantReportingWired))
	}
	if got := reporting.DefaultWindowDays(context.Background()); got != 30 {
		t.Errorf("无库时 DefaultWindowDays() = %d, 期望回落 30", got)
	}
}
