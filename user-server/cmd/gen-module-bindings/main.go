package main

// gen-module-bindings 从 hivemtk user-server 的 ModuleMenuBindings / ModuleCatalogMeta
// 导出 platform-server 的 seed JSON。
//
// 为什么需要这一步：菜单归属的事实源在 user-server（SystemMenuTree 在那边），
// 而平台端要做「模块包含哪些页面」的展示与下发，两端必须看到同一份数据。
// 直接手抄两份必然漂移，故由本脚本单向导出 + 两端各有一条测试钉死计数。
//
// 用法（工作区根目录）：
//   go run ./cmd/gen-module-bindings     （在 hivemtk/user-server 下执行）
//
// 默认输出到 <工作区>/hivemtk-platform/platform-server/config/module_catalog_seed.json。
//
// ⚠️ 这条路径错了**两次**，两次命令都打印了"已生成"。记在这里免得第三次：
//   1. 写成 `../hivemtk-platform/…`。命令在 hivemtk/user-server 下执行，
//      `..` 只回到 hivemtk/，于是解析成 hivemtk/hivemtk-platform/…（多一层）。
//      正确是从 user-server 上两级：../../hivemtk-platform/…
//   2. 输出目录不能叫 seed/：仓库里 platform-server/seed 是一个未入库的构建
//      残留二进制（go build ./cmd/seed 的产物），同名会让路径解析失败。
//
// 因此 defaultOutPath 不写死相对路径，而是**向上找工作区根**（含
// hivemtk-platform 的那一层），并由 writeSeed 带写后存在性断言：
// 生成类命令的"成功输出"不是验证，落点才是。

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

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

// defaultOutPath 反推 platform-server 的绝对路径。
//
// ============================================================
// 为什么不用相对路径，也不数"上几级"
// ============================================================
// 这条路径错了三次，三次命令都打印了"已生成"：
//  1. `../hivemtk-platform/…`（从 user-server 起算 → 解析成 hivemtk/hivemtk-platform）
//  2. `../../hivemtk-platform/…`（数了两级 → 解析到工作区**上面**一层）
//  3. 按 CWD 往上数三级（go run 的 CWD 是包目录，三级只到主仓，仍差一级）
//
// 共同点：都在**猜层级**，而层级会因 checkout 位置、软链、执行目录而变。
// 改为**认标识**：向上找到同时含 `hivemtk` 与 `hivemtk-platform` 两个子目录的那一层，
// 也就是工作区根。找不到就报错退出 —— 绝不猜。
func defaultOutPath() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	dir := wd
	for i := 0; i < 8; i++ {
		if isWorkspaceRoot(dir) {
			return filepath.Join(dir, "hivemtk-platform", "platform-server", "config", "module_catalog_seed.json")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// isWorkspaceRoot 判该目录是否是工作区根（同时含两个仓目录）
func isWorkspaceRoot(dir string) bool {
	for _, child := range []string{"hivemtk", "hivemtk-platform"} {
		st, err := os.Stat(filepath.Join(dir, child))
		if err != nil || !st.IsDir() {
			return false
		}
	}
	return true
}

func main() {
	defaultOut := defaultOutPath()
	out := flag.String("out", defaultOut, "输出 JSON 路径")
	flag.Parse()
	if strings.TrimSpace(*out) == "" {
		fmt.Fprintf(os.Stderr,
			"❌ 找不到工作区根（向上 8 层内未同时发现 hivemtk/ 与 hivemtk-platform/）。\n"+
				"   请用 -out 显式指定目标路径，或确认 checkout 结构完整。\n")
		os.Exit(1)
	}

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
