package config

import "testing"

// TestResolvePlatformEnabledDistinguishesUnknownFromOff 钉死三态区分。
//
// 背景（2026-10-20 多部署模式评估）：PLATFORM_ENABLED 拼错原本是**纯静默**的——
// 开关判假，链路不装配，而启动日志写的是「PLATFORM_ENABLED 未设置」。
// 运维明明配了开关却看不到任何联平台行为，日志还反过来告诉他"你没配"，
// 于是往完全错误的方向查。真正能解释"我配了却不生效"的原因（拼错）被日志盖住了。
//
// 本测试锁住 ResolvePlatformEnabled 的三态语义：
//   - 已知关闭（false/off/0/no/空）⇒ (false, true)
//   - 已知开启（true/on/1/yes）      ⇒ (true, true)
//   - 未知值（拼错）                 ⇒ (false, false) —— 保守关闭，但**如实说不知道**
//
// 注意：**不**要求未知值报错或拒绝启动。判真方向必须是"关"（既有
// TestPlatformEnabledAcceptsOnlyTrueLike 已锁），因为本地私有化部署不该因为
// 一个可选组件的开关拼错就起不来；也不该因为拼错就朝一个可能已下线的地址发请求。
func TestResolvePlatformEnabledDistinguishesUnknownFromOff(t *testing.T) {
	offCases := []string{"", "  ", "false", "FALSE", " False ", "0", "off", "OFF", "no", "NO"}
	for _, v := range offCases {
		enabled, known := ResolvePlatformEnabled(v)
		if enabled {
			t.Errorf("PLATFORM_ENABLED=%q 应判为关闭", v)
		}
		if !known {
			t.Errorf("PLATFORM_ENABLED=%q 是已知关闭值，known 应为 true（不能与拼错混为一谈）", v)
		}
	}

	onCases := []string{"true", "TRUE", " True ", "1", "on", "ON", "yes", "YES"}
	for _, v := range onCases {
		enabled, known := ResolvePlatformEnabled(v)
		if !enabled {
			t.Errorf("PLATFORM_ENABLED=%q 应判为开启", v)
		}
		if !known {
			t.Errorf("PLATFORM_ENABLED=%q 是已知开启值，known 应为 true", v)
		}
	}

	// 拼错：保守关闭（enabled=false），但 known=false 让调用方能说清真相
	typoCases := []string{"tru", "ture", "TRUEE", "1x", "enable", "enabled", "yes please", "on1"}
	for _, v := range typoCases {
		enabled, known := ResolvePlatformEnabled(v)
		if enabled {
			t.Errorf("PLATFORM_ENABLED=%q 拼错时不得判为开启（判真方向必须朝关）", v)
		}
		if known {
			t.Errorf("PLATFORM_ENABLED=%q 是拼错值，known 应为 false（拼错必须与明确关闭区分开）", v)
		}
	}
}

// TestPlatformEnabledMatchesResolvePlatformEnabled 保证布尔入口与三态入口
// 永远给出同一个判真结果——ResolvePlatformEnabled 是新增的，PlatformEnabled
// 是全仓 30+ 处调用的既有入口，两者分叉就会让日志与实际行为对不上。
func TestPlatformEnabledMatchesResolvePlatformEnabled(t *testing.T) {
	for _, v := range []string{"", "false", "true", "tru", "1", "off", "on", "yes", "no", "garbage"} {
		t.Setenv("PLATFORM_ENABLED", v)
		wantEnabled, _ := ResolvePlatformEnabled(v)
		if got := PlatformEnabled(); got != wantEnabled {
			t.Errorf("PLATFORM_ENABLED=%q：PlatformEnabled()=%v 与 ResolvePlatformEnabled()=%v 不一致",
				v, got, wantEnabled)
		}
	}
}
