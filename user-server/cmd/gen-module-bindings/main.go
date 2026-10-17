package main

// gen-module-bindings 从 hivemtk user-server 的 ModuleMenuBindings / ModuleCatalogMeta
// 导出 platform-server 的 seed JSON。
//
// 为什么需要这一步：菜单归属的事实源在 user-server（SystemMenuTree 在那边），
// 而平台端要做「模块包含哪些页面」的展示与下发，两端必须看到同一份数据。
// 直接手抄两份必然漂移，故由本脚本单向导出 + 两端各有一条测试钉死计数。
//
// 用法（工作区根目录）：
//   cd hivemtk/user-server && go run ./cmd/gen-module-bindings \
//     -out ../hivemtk-platform/platform-server/seed/module_menu_bindings.json
//
// 生成件禁止手改：改了会被下一次生成覆盖，且 platform-server 侧
// TestSeedModuleBindingsCoverage 会按 user-server 的节点总数核对而变红。

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"hivemtk-user/internal/model"
)

type binding struct {
	ModuleKey    string   `json:"module_key"`
	Name         string   `json:"name"`
	Category     string   `json:"category"`
	PriceCents   int64    `json:"price_cents"`
	BillingCycle string   `json:"billing_cycle"`
	Icon         string   `json:"icon"`
	Summary      string   `json:"summary"`
	DependsOn    []string `json:"depends_on"`
	MenuKeys     []string `json:"menu_keys"`
}

type payload struct {
	GeneratedBy  string    `json:"generated_by"`
	MenuKeyTotal int       `json:"menu_key_total"`
	ModuleTotal  int       `json:"module_total"`
	BaseTotal    int       `json:"base_module_total"`
	PaidTotal    int       `json:"paid_module_total"`
	MenuKeys     []string  `json:"menu_keys"`
	Bindings     []binding `json:"bindings"`
}

func main() {
	out := flag.String("out", "../hivemtk-platform/platform-server/seed/module_menu_bindings.json", "输出 JSON 路径")
	flag.Parse()

	allKeys := model.FlattenMenuKeys(model.SystemMenuTree)

	bindings := make([]binding, 0, len(model.ModuleCatalogMeta))
	for _, m := range model.ModuleCatalogMeta {
		keys := append([]string(nil), model.ModuleMenuBindings[m.Key]...)
		sort.Strings(keys)
		deps := append([]string(nil), m.DependsOn...)
		sort.Strings(deps)
		if deps == nil {
			deps = []string{}
		}
		bindings = append(bindings, binding{
			ModuleKey:    m.Key,
			Name:         m.Name,
			Category:     m.Category,
			PriceCents:   m.PriceCents,
			BillingCycle: m.BillingCycle,
			Icon:         m.Icon,
			Summary:      m.Summary,
			DependsOn:    deps,
			MenuKeys:     keys,
		})
	}

	p := payload{
		GeneratedBy:  "hivemtk/user-server/cmd/gen-module-bindings（勿手改）",
		MenuKeyTotal: len(allKeys),
		ModuleTotal:  len(model.ModuleCatalogMeta),
		BaseTotal:    len(model.BaseModuleKeys()),
		PaidTotal:    len(model.PaidModuleKeys()),
		MenuKeys:     allKeys,
		Bindings:     bindings,
	}

	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "创建输出目录失败: %v\n", err)
		os.Exit(1)
	}
	buf, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "序列化失败: %v\n", err)
		os.Exit(1)
	}
	buf = append(buf, '\n')
	if err := os.WriteFile(*out, buf, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "写入失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ 已生成 %s：%d 个模块（基础 %d / 收费 %d），覆盖 %d 个菜单节点\n",
		*out, p.ModuleTotal, p.BaseTotal, p.PaidTotal, p.MenuKeyTotal)
}
