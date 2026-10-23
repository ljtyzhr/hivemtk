package service

import (
	"encoding/json"
	"strings"
	"testing"

	"hivemtk-user/internal/pkg/contract"
)

func TestNewSystemStatsService(t *testing.T) {
	svc := NewSystemStatsService()
	if svc == nil {
		t.Error("Expected non-nil SystemStatsService")
	}
}

func TestSystemStatsService_GetSystemInfo(t *testing.T) {
	svc := NewSystemStatsService()
	info, err := svc.GetSystemInfo()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("Expected non-nil SystemInfo")
	}
	if info.GoVersion == "" {
		t.Error("Expected non-empty GoVersion")
	}
	if info.Hostname == "" {
		t.Error("Expected non-empty Hostname")
	}
}

// TestSystemInfoCarriesPlatformEnabled 把"平台集成是否启用"透给前端，供其隐藏上架入口。
//
// 两个方向都要断言：只断言关态 false 的话，字段被写死成常量也是绿的。
// JSON 名必须是 platform_enabled——前端 Playground.vue 的 v-if 直接认这个名字。
func TestSystemInfoCarriesPlatformEnabled(t *testing.T) {
	svc := NewSystemStatsService()

	t.Setenv("PLATFORM_ENABLED", "false")
	off, err := svc.GetSystemInfo()
	if err != nil {
		t.Fatalf("GetSystemInfo: %v", err)
	}
	if off.PlatformEnabled {
		t.Fatal("关态 platform_enabled 必须为 false，否则前端会露出打不通的上架入口")
	}
	b, _ := json.Marshal(off)
	if !strings.Contains(string(b), `"platform_enabled":false`) {
		t.Fatalf("JSON 字段名必须是 platform_enabled，实际=%s", b)
	}

	t.Setenv("PLATFORM_ENABLED", "true")
	on, err := svc.GetSystemInfo()
	if err != nil {
		t.Fatalf("GetSystemInfo(开态): %v", err)
	}
	if !on.PlatformEnabled {
		t.Fatal("开态 platform_enabled 必须为 true，否则上架入口永久隐藏")
	}
}

// TestSystemInfoCarriesPlatformContractOnlyWhenRefused 把"平台要求本机升级"（A4 的 426）
// 透给管理员，且**没被拒过时整个字段不出现**。
//
// 两个方向都必须钉：
//
//  1. 被拒后 ⇒ 字段出现且带原因。只写日志不够：A4 的原始症状正是"失败被 logger.Warn
//     吞掉，表现为同步一直没生效"，管理端看不见就没人会去升级。
//  2. 没被拒时 ⇒ **JSON 里连键都不能有**。这是 `off` 模式（无平台）逐字节等价的硬约束：
//     多一个 `"platform_contract":null` 就是响应体变了，前端快照测试与契约对比都会红。
//     用 `omitempty` 不够 —— 指针为 nil 才真的省略，空结构体会被序列化成 `{}`。
func TestSystemInfoCarriesPlatformContractOnlyWhenRefused(t *testing.T) {
	svc := NewSystemStatsService()

	contract.ClearUpgradeRequired()
	t.Cleanup(contract.ClearUpgradeRequired)

	clean, err := svc.GetSystemInfo()
	if err != nil {
		t.Fatalf("GetSystemInfo: %v", err)
	}
	b, _ := json.Marshal(clean)
	if strings.Contains(string(b), "platform_contract") {
		t.Fatalf("从没被 426 拒过时，响应体里不该出现 platform_contract（off 模式必须逐字节等价）：%s", b)
	}

	contract.RecordUpgradeRequired("契约版本不兼容：平台最低支持 v2")

	refused, err := svc.GetSystemInfo()
	if err != nil {
		t.Fatalf("GetSystemInfo: %v", err)
	}
	if refused.PlatformContract == nil {
		t.Fatal("平台已回 426，system_info 里必须带上 platform_contract（否则管理员看不到要升级）")
	}
	b2, _ := json.Marshal(refused)
	if !strings.Contains(string(b2), `"platform_contract"`) ||
		!strings.Contains(string(b2), "平台最低支持 v2") {
		t.Fatalf("platform_contract 应带上平台给的原因，实际=%s", b2)
	}
}
