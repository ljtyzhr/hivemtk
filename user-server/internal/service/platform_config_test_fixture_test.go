package service

import (
	"os"
	"testing"

	"hivemtk-user/internal/config"
)

// withPlatformEndpointCfg 装一份平台配置并保证平台集成开关是开启态。
//
// 为什么开关必须一起开：平台地址的解析入口是 config.PlatformURL()，
// 而它在**关态恒返回空串**（"没接平台"是合法值）—— 只设 PlatformCfg.APIURL
// 而不开关，出站会拼出 "/merchant-api/xxx" 这种无协议的相对路径，
// 请求构造阶段就报 "unsupported protocol scheme"，而"请求没发出去"在
// 多数用例里表现为断言拿不到预期结果，很难一眼看出真因。
//
// 与 internal/platform 包的 withPlatformConfig、internal/controller 包的
// withPlatformCfg 同一口径（三处各自实现是因为它们分属不同包，
// 同包内共用一个即可）。
//
// 开关的置法是「**调用方已经设过就不动**」：测关态的用例（platform_disabled_test.go）
// 自己会 t.Setenv("PLATFORM_ENABLED", "false")，夹具无条件设 true 会把那条声明顶掉。
func withPlatformEndpointCfg(t *testing.T, cfg *config.PlatformConfig) {
	t.Helper()
	orig := config.PlatformCfg
	config.PlatformCfg = cfg
	t.Cleanup(func() { config.PlatformCfg = orig })
	if _, alreadySet := os.LookupEnv("PLATFORM_ENABLED"); !alreadySet {
		t.Setenv("PLATFORM_ENABLED", "true")
	}
}
