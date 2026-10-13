package app

import (
	"testing"

	contentctrl "hivemtk-user/internal/content/controller"
	"hivemtk-user/internal/model"
	appsvc "hivemtk-user/internal/service"

	"hivemtk-user/internal/pkg/testutil"

	"github.com/stretchr/testify/require"
)

var wantFrontendUIWired = []string{
	"frontend_ui.material_max_size_mb",
}

func TestWireFrontendUIConfigParamsKeys(t *testing.T) {
	got := WireFrontendUIConfigParams()
	require.Equal(t, wantFrontendUIWired, got, "接线键名必须逐条相等且顺序稳定")
}

func TestWiredFrontendUIKeysExistInSeed(t *testing.T) {
	inSeed := map[string]bool{}
	for _, d := range appsvc.DefaultParamDefs() {
		inSeed[d.Group+"."+d.Key] = true
	}
	for _, k := range wantFrontendUIWired {
		require.True(t, inSeed[k], "%s 不在 DefaultParamDefs 里，接线指向了不存在的参数", k)
	}
}

// TestWireFrontendUIConfigParamsReachesReader 证明参数中心的值真的走到素材库上传口。
//
// 断言打在 Probe 上而不是只打「Wire 返回了几条」：后者在键名写错组时也能全绿
// （Get* 读不到行就回兜底，静默且无声）。
func TestWireFrontendUIConfigParamsReachesReader(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.ConfigParam{}, &model.ConfigParamAuditLog{})
	require.NoError(t, appsvc.SeedConfigParams(t.Context(), db))
	appsvc.SetGlobalForTest(appsvc.NewConfigParamService(db))
	// provider 里捕获的是这个 service 实例；置回 nil 让残留闭包回落到无库 stub，
	// 而不是继续读一条已被本用例改过的行去喂下一个用例。
	t.Cleanup(func() { appsvc.SetGlobalForTest(nil) })

	cp := appsvc.GlobalConfigParam()
	require.NoError(t, cp.UpdateValue(t.Context(), "frontend_ui", "material_max_size_mb", "25", 1))

	WireFrontendUIConfigParams()

	maxBytes, maxMB := contentctrl.ProbeMaterialMaxUploadSize()
	require.Equal(t, 25, maxMB, "参数中心写 25，读取口没跟上")
	require.Equal(t, int64(25*1024*1024), maxBytes)
}

func TestWireFrontendUIConfigParamsWithoutDB(t *testing.T) {
	appsvc.SetGlobalForTest(nil)
	t.Cleanup(func() { appsvc.SetGlobalForTest(nil) })

	got := WireFrontendUIConfigParams()
	require.Len(t, got, 1, "无库时仍应报告接了 1 条，只是取值回落兜底")

	maxBytes, maxMB := contentctrl.ProbeMaterialMaxUploadSize()
	require.Equal(t, contentctrl.DefaultMaterialMaxSizeMB, maxMB)
	require.Equal(t, int64(contentctrl.DefaultMaterialMaxSizeMB*1024*1024), maxBytes,
		"兜底必须等于接线前写死的 10MB，否则「读不到库」会变成一次静默放宽")
}
