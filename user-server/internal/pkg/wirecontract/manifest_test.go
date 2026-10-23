// manifest_test.go 把"租户端这一侧的跨端线上形状"钉成一份进 git 的清单（SaaS 审计 A10）。
//
// # 与平台端那份的关系
//
// 平台端是 wire 的**权威方**（它定义服务端接受什么），本仓是消费方。两份清单由
// `scripts/check-wire-contract.py` 按方向性规则比对：
//
//	租户侧发的 / 读的每个键，平台侧必须有；平台的 `binding:"required"` 键，租户侧必须发。
//
// # 清单的键为什么用**平台侧的类型名**
//
// 这份清单唯一用途就是跨仓比对，键用对面（平台 `platform-server/docs/contracts/
// cross-repo-wire.json`）的类型名，配对关系就是键相等 —— 不需要再维护一张"谁对谁"的
// 映射表（那张表迟早会漏一行，而漏一行的表现是"这条通道静默不再被判据覆盖"）。
// 每条登记项旁边注明了本仓对应的 Go 类型。
//
// 重新生成：`UPDATE_WIRE_MANIFEST=1 go test ./internal/pkg/wirecontract/`
package wirecontract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"hivemtk-user/internal/platform"
)

// tenantRoots 登记"跨端传递的结构"的**租户侧**类型。
//
// 只收真的跨端的四条通道（开户登记 / 心跳 / 订单 / 模块下发）。`ConfigParam` 这类各自
// 库内的同名模型刻意不收：两端独立库、两条演进节奏，共形状既不划算也没有失败面。
func tenantRoots() map[string]any {
	return map[string]any{
		// 开户登记（平台 controller.RegisterReq / controller.InstallReportRequest）
		"RegisterReq":          platform.RegisterMerchantReq{},
		"InstallReportRequest": platform.ReportInstallReq{},
		// 心跳（平台 service.HeartbeatRequest）
		"HeartbeatRequest": platform.ReportHeartbeatReq{},
		// 订单（平台 service.CreateOrderInput / CreateOrderResult / model.OrderItem /
		// service.EpayPayParams）
		"CreateOrderInput":  platform.CreateOrderInput{},
		"CreateOrderResult": platform.ModuleOrderResult{},
		"OrderItem":         platform.ModuleOrderItem{},
		"EpayPayParams":     platform.OrderPayment{},
		// 模块下发（平台 service.ModuleSyncSnapshot / MerchantModuleView /
		// SubscriptionView / model.ModuleOption）
		"ModuleSyncSnapshot": platform.ModuleSnapshot{},
		"MerchantModuleView": platform.ModuleCatalogRow{},
		"SubscriptionView":   platform.SubscriptionSnapshot{},
		"ModuleOption":       platform.ModuleOptionRow{},
	}
}

// tenantRenames 把**嵌套类型**的租户 Go 名改写成平台侧的规范名。
//
// 根类型在 tenantRoots() 里已经直接用平台名，这里补的是被反射自动发现的那一层：
// `platform.ModuleCatalogRow` 与平台的 `service.MerchantModuleView` 说的是同一个线上对象。
// 不写这张表，同一类型会在清单里出现两次（一次叫平台名、一次叫自家名），于是跨仓脚本
// 既看到"未登记的 struct"又看到"对面没有这个类型"——第一次跑就报了这个。
//
// 为什么不把映射留在跨仓脚本里：映射留在这里，两份清单就落进同一个命名空间，
// 配对退化成键相等，脚本里不再有"谁对谁"的表可以漏行。新出现且没登记的嵌套类型
// 会以自己的 Go 名留在清单里 ⇒ 对面查不到 ⇒ 红（见下一条测试与跨仓脚本）。
var tenantRenames = map[string]string{
	"ModuleCatalogRow":     "MerchantModuleView",
	"ModuleOptionRow":      "ModuleOption",
	"SubscriptionSnapshot": "SubscriptionView",
	"ModuleOrderItem":      "OrderItem",
	"OrderPayment":         "EpayPayParams",
}

// TestManifestUsesCanonicalNamesOnly 清单里不许出现"自家 Go 名"的键。
//
// 这条是本仓能独立发现"新加了跨端嵌套类型但没登记规范名"的地方：那种情况下
// 新类型会以自己的 Go 名进清单，而本仓 `go test` 就应该红，不必等到跨仓脚本
// （单仓 CI 跑不到跨仓脚本）。
func TestManifestUsesCanonicalNamesOnly(t *testing.T) {
	m := DescribeWithRenames(tenantRoots(), tenantRenames)
	known := map[string]bool{}
	for name := range tenantRoots() {
		known[name] = true
	}
	for _, canon := range tenantRenames {
		known[canon] = true
	}
	for name := range m.Structs {
		if !known[name] {
			t.Errorf("清单里出现未登记的键 %q —— 它要么是新加的跨端嵌套类型（请在 tenantRenames 里"+
				"写上它在平台侧叫什么），要么是登记错了。跨仓脚本靠键相等配对，留个自家名就等于"+
				"这条子树不在判据里", name)
		}
	}
}

// TestCrossRepoWireManifestMatchesCode 清单必须与活体 struct 一致。
func TestCrossRepoWireManifestMatchesCode(t *testing.T) {
	got := DescribeWithRenames(tenantRoots(), tenantRenames)
	raw, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatalf("序列化清单失败: %v", err)
	}
	raw = append(raw, '\n')

	path := filepath.Join("..", "..", "..", "docs", "contracts", "cross-repo-wire.json")
	if os.Getenv("UPDATE_WIRE_MANIFEST") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("创建目录失败: %v", err)
		}
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			t.Fatalf("写入清单失败: %v", err)
		}
		t.Logf("已重新生成 %s（%d 个结构）", path, len(got.Structs))
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读不到清单 %s（%v）：它必须进 git，否则跨仓比对无从执行", path, err)
	}
	if string(want) != string(raw) {
		t.Fatalf("租户侧线上形状与清单不一致 —— 改了跨端 struct 却没重新生成清单。\n"+
			"本地 `UPDATE_WIRE_MANIFEST=1 go test ./internal/pkg/wirecontract/` 之后，\n"+
			"还要跑 `python3 scripts/check-wire-contract.py` 确认与平台侧那份仍然对得上。\n"+
			"清单路径: %s\n期望(新):\n%s\n实际(旧):\n%s", path, raw, want)
	}
}

// TestManifestCoversTheFourBoundaries 四个边界都必须有登记项。
//
// 防"把某条通道从清单里删掉就绿了"：A10 的范围就是这四条，少一条该通道退回"靠人对齐"。
func TestManifestCoversTheFourBoundaries(t *testing.T) {
	roots := tenantRoots()
	for boundary, typ := range map[string]string{
		"开户登记-注册": "RegisterReq",
		"开户登记-安装": "InstallReportRequest",
		"心跳":      "HeartbeatRequest",
		"订单-请求":   "CreateOrderInput",
		"订单-响应":   "CreateOrderResult",
		"模块-下发":   "ModuleSyncSnapshot",
	} {
		if _, ok := roots[typ]; !ok {
			t.Errorf("边界「%s」的登记项 %s 不见了 —— 少一条该通道就退回「靠人对齐」", boundary, typ)
		}
	}
}

// TestManifestHasNoUnnamedJSONFields 任何字段都必须有明确的线上名。
//
// 没写 json tag 的字段在 wire 上叫 Go 名（大驼峰）。两端各写一套 struct 时，
// 一边写 `contact_email`、另一边漏 tag 叫 `ContactEmail`，两边都能编译，线上永远收不到值。
func TestManifestHasNoUnnamedJSONFields(t *testing.T) {
	m := DescribeWithRenames(tenantRoots(), tenantRenames)
	for name, st := range m.Structs {
		for _, f := range st.Fields {
			if f.JSON == "" {
				t.Errorf("%s 里有一个没有线上名的字段（没写 json tag）", name)
			}
			if f.JSON[0] >= 'A' && f.JSON[0] <= 'Z' {
				t.Errorf("%s.%s 的线上名是大驼峰（看起来是漏了 json tag）", name, f.JSON)
			}
		}
	}
}
