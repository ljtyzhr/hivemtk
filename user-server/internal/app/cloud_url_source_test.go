package app

// A11（`未完成任务.md` §9）回归：云端版地址的**来源**可判定，且回落到厂商默认域名不能是静默的。
//
// 修前形态：`DefaultCloudURL = "https://work.xapptool.cn"`（厂商生产域名）是最后一级回落，
// 且 provider 的缺省参数也是同一个常量 ⇒ "没配"与"配了同一个值"在代码里完全无法区分。
// 一套自建 SaaS / 私有化部署因此会把前端「云端版入口」指向厂商站点，没有任何提示。

import (
	"context"
	"testing"

	"hivemtk-user/internal/config"
)

// clearCloudURLSources 清空三个来源（平台配置 / env / 参数中心），让断言只看到"内置默认"。
func clearCloudURLSources(t *testing.T) {
	t.Helper()
	prevCfg := config.PlatformCfg
	config.PlatformCfg = nil
	t.Cleanup(func() { config.PlatformCfg = prevCfg })

	t.Setenv("PLATFORM_CLOUD_URL", "")
	// provider 复位为"未装配"（与 WireURLsConfigParams 之前的启动早期一致）
	config.SetCloudURLProvider(func(ctx context.Context) string { return "" })
}

func TestCloudURLSource_BuiltinFallback(t *testing.T) {
	clearCloudURLSources(t)

	url, source := config.CloudURLSource(context.Background())
	if url != config.DefaultCloudURL {
		t.Fatalf("无任何配置时应回落到内置常量，实际 %q", url)
	}
	if source != config.CloudURLSourceBuiltin {
		t.Fatalf("来源应为 builtin（这是 A11 能打响声的唯一依据），实际 %q", source)
	}
}

func TestCloudURLSource_ExplicitSources(t *testing.T) {
	clearCloudURLSources(t)

	// 平台配置优先
	config.PlatformCfg = &config.PlatformConfig{CloudURL: "https://cloud.example.com"}
	if _, source := config.CloudURLSource(context.Background()); source != config.CloudURLSourcePlatformCfg {
		t.Fatalf("平台配置来源标签错误: %q", source)
	}
	config.PlatformCfg = nil

	// env 次之
	t.Setenv("PLATFORM_CLOUD_URL", "https://env.example.com")
	url, source := config.CloudURLSource(context.Background())
	if url != "https://env.example.com" || source != config.CloudURLSourceEnv {
		t.Fatalf("env 来源错误: (%q,%q)", url, source)
	}

	// 参数中心最后（且**缺省值必须是空串**：同一个常量当缺省值就等于把"没配"伪装成"配了"）
	t.Setenv("PLATFORM_CLOUD_URL", "")
	config.SetCloudURLProvider(func(ctx context.Context) string { return "" })
	if _, source := config.CloudURLSource(context.Background()); source != config.CloudURLSourceBuiltin {
		t.Fatalf("参数中心返回空串时应落到 builtin，实际 %q", source)
	}
	config.SetCloudURLProvider(func(ctx context.Context) string { return "https://param.example.com" })
	url, source = config.CloudURLSource(context.Background())
	if url != "https://param.example.com" || source != config.CloudURLSourceParamCenter {
		t.Fatalf("参数中心来源错误: (%q,%q)", url, source)
	}
}

// TestWarnIfCloudURLIsBuiltinDefault_DoesNotPanic 冒烟：提示函数只在"仍是厂商默认域名"时打日志，
// 三个来源都要能安全走一遍（日志内容无法在单测里断言，这里钉"不 panic + 不改写配置值"）。
func TestWarnIfCloudURLIsBuiltinDefault_DoesNotPanic(t *testing.T) {
	clearCloudURLSources(t)
	WarnIfCloudURLIsBuiltinDefault()

	config.SetCloudURLProvider(func(ctx context.Context) string { return "https://param.example.com" })
	WarnIfCloudURLIsBuiltinDefault()
	if got := config.CloudURL(); got != "https://param.example.com" {
		t.Fatalf("提示函数不得改写生效值，实际 %q", got)
	}

	t.Setenv("PLATFORM_CLOUD_URL", config.DefaultCloudURL) // 显式配置成同一域名 ⇒ 不提示
	WarnIfCloudURLIsBuiltinDefault()
}
