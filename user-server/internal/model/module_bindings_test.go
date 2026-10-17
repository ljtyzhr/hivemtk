package model

import (
	"sort"
	"strings"
	"testing"
)

// TestModuleMenuBindingsCoverAllMenuKeys 钉死「每个菜单节点都有归属」。
//
// 这是整个模块化改造的单点事实源：漏绑 = 那个页面永远不过闸（收费模块白送），
// 双绑（叶子节点）= 两个模块都声称拥有它，关掉任一个都会让它消失（用户投诉"我买了却看不到"）。
// 两种错都在人工维护 200+ 条映射时极易犯，故用测试兜底而不是靠 review。
//
// 绑定匹配按节点形态分两档：
//   - **精确绑定**（叶子节点，有 Path）：必须**恰好一个**归属——单点所有权；
//     免费模块不允许"混进"付费模块的入口。
//   - **前缀绑定**（目录节点，无 Path）：允许一个模块前缀覆盖一片——例如 reach
//     顶级目录下就有 6 个付费模块；前缀与"精确单模块"互斥，避免一个路径被两条
//     绑定同时命中。
//
// 任何菜单键都必须能被某条绑定覆盖，否则该菜单对所有模块档位都不可见。
func TestModuleMenuBindingsCoverAllMenuKeys(t *testing.T) {
	all := FlattenMenuKeys(SystemMenuTree)
	if len(all) == 0 {
		t.Fatal("SystemMenuTree 为空，测试前提不成立")
	}

	isDir := map[string]bool{}
	var markDirs func([]SystemMenuNode)
	markDirs = func(ns []SystemMenuNode) {
		for _, n := range ns {
			if n.Path == "" {
				isDir[n.Key] = true
			}
			markDirs(n.Children)
		}
	}
	markDirs(SystemMenuTree)

	owner := map[string][]string{} // menu_key → 归属模块列表
	for mk, keys := range ModuleMenuBindings {
		for _, k := range keys {
			owner[k] = append(owner[k], mk)
		}
	}

	var uncovered, duplicated []string
	for _, k := range all {
		if len(owner[k]) == 0 {
			uncovered = append(uncovered, k+" ("+nodeKind(isDir[k])+")")
			continue
		}
		// 只有叶子节点要求单点所有权
		if len(owner[k]) > 1 && !isDir[k] {
			duplicated = append(duplicated, k+" -> "+strings.Join(owner[k], ","))
		}
	}
	sort.Strings(uncovered)
	sort.Strings(duplicated)

	if len(uncovered) > 0 {
		t.Errorf("有 %d 个菜单节点未绑定模块（它们将永远不过闸）：\n  %s",
			len(uncovered), strings.Join(uncovered, "\n  "))
	}
	if len(duplicated) > 0 {
		t.Errorf("有 %d 个叶子菜单节点被多个模块绑定（关掉任一个都会让它消失）：\n  %s",
			len(duplicated), strings.Join(duplicated, "\n  "))
	}
}

func nodeKind(isDir bool) string {
	if isDir {
		return "目录"
	}
	return "叶子"
}

// TestModuleMenuBindingsNoUnknownKey 反向：绑定表里不得有注册表中不存在的 key
// （改名后忘了同步绑定表，这里会响）。
func TestModuleMenuBindingsNoUnknownKey(t *testing.T) {
	valid := map[string]bool{}
	for _, k := range FlattenMenuKeys(SystemMenuTree) {
		valid[k] = true
	}
	var unknown []string
	for _, keys := range ModuleMenuBindings {
		for _, k := range keys {
			if !valid[k] {
				unknown = append(unknown, k)
			}
		}
	}
	sort.Strings(unknown)
	if len(unknown) > 0 {
		t.Errorf("绑定表含注册表中不存在的菜单节点（改名/删除后未同步）：\n  %s",
			strings.Join(unknown, "\n  "))
	}
}

// TestModuleCatalogMetaConsistent 元数据表与绑定表必须互相认识。
func TestModuleCatalogMetaConsistent(t *testing.T) {
	metaKeys := map[string]bool{}
	for _, m := range ModuleCatalogMeta {
		if metaKeys[m.Key] {
			t.Errorf("ModuleCatalogMeta 出现重复 key: %s", m.Key)
		}
		metaKeys[m.Key] = true

		if m.Category != ModuleCategoryBase && m.Category != ModuleCategoryPaid {
			t.Errorf("%s 分类非法: %q", m.Key, m.Category)
		}
		if m.PriceCents < 0 {
			t.Errorf("%s 价格不能为负: %d", m.Key, m.PriceCents)
		}
		if m.Summary == "" {
			t.Errorf("%s 缺 Summary（市场卡片会显示空白）", m.Key)
		}
	}

	for mk := range ModuleMenuBindings {
		if !metaKeys[mk] {
			t.Errorf("绑定表里的模块 %s 在 ModuleCatalogMeta 中没有元数据", mk)
		}
	}
	for _, m := range ModuleCatalogMeta {
		if _, ok := ModuleMenuBindings[m.Key]; !ok {
			t.Errorf("模块 %s 在元数据里但绑定表里没有条目", m.Key)
		}
	}
}

// TestModuleDependencyGraphAcyclic 依赖图必须无环，且依赖项都必须存在。
//
// 环会让开通时的闭包计算死循环 —— 而开通是一个运营手点一次的动作，
// 一次死循环就是平台端一个 goroutine 永久卡住，故在写入侧与种子侧都要拦。
func TestModuleDependencyGraphAcyclic(t *testing.T) {
	known := map[string]bool{}
	for _, m := range ModuleCatalogMeta {
		known[m.Key] = true
	}
	for _, m := range ModuleCatalogMeta {
		for _, dep := range m.DependsOn {
			if !known[dep] {
				t.Errorf("%s 依赖了不存在的模块 %s", m.Key, dep)
			}
			if dep == m.Key {
				t.Errorf("%s 依赖了自己", m.Key)
			}
		}
	}

	// 迭代式着色检测环（208 节点规模，递归也不爆栈，但迭代更稳）
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[string]int{}
	deps := map[string][]string{}
	for _, m := range ModuleCatalogMeta {
		deps[m.Key] = m.DependsOn
	}
	var stack []string
	var visit func(string) bool
	visit = func(k string) bool {
		switch color[k] {
		case gray:
			return true // 成环
		case black:
			return false
		}
		color[k] = gray
		stack = append(stack, k)
		for _, d := range deps[k] {
			if visit(d) {
				return true
			}
		}
		stack = stack[:len(stack)-1]
		color[k] = black
		return false
	}
	for _, m := range ModuleCatalogMeta {
		stack = stack[:0]
		if visit(m.Key) {
			t.Errorf("模块依赖存在环，路径: %s", strings.Join(stack, " -> "))
		}
	}
}

// TestBaseModuleCount 基础模块必须是 7 个，且 core/customer/inbox/knowledge 必须在内。
//
// 7 是 docs/architecture/MODULAR_MODULES.md 的承诺；基础模块增减会直接改变
// "免费用户能用什么"，属产品契约变更，必须改文档 + 改这个断言，不能顺手加。
func TestBaseModuleCount(t *testing.T) {
	base := BaseModuleKeys()
	if len(base) != 7 {
		t.Errorf("基础模块应为 7 个，实际 %d 个: %v", len(base), base)
	}
	for _, must := range []string{ModuleCore, ModuleCustomer, ModuleInbox, ModuleKnowledge, ModuleAsset, ModuleLLMSelf, ModulePlatformLink} {
		found := false
		for _, b := range base {
			if b == must {
				found = true
			}
		}
		if !found {
			t.Errorf("基础模块集合缺少 %s", must)
		}
	}
	if n := len(PaidModuleKeys()); n != 17 {
		t.Errorf("收费模块应为 17 个，实际 %d 个", n)
	}
}

// TestBaseModulesHaveNoPaidDependency 基础模块不得依赖收费模块 ——
// 那会让"免费用户"在开通闭包时被拖进付费模块。
func TestBaseModulesHaveNoPaidDependency(t *testing.T) {
	for _, m := range ModuleCatalogMeta {
		if m.Category != ModuleCategoryBase {
			continue
		}
		for _, dep := range m.DependsOn {
			if OwnerModuleCategory(dep) == ModuleCategoryPaid {
				t.Errorf("基础模块 %s 依赖了收费模块 %s", m.Key, dep)
			}
		}
	}
}

// TestFilterMenuTreeByModule 关掉一个模块，它的菜单子树必须整体消失。
func TestFilterMenuTreeByModule(t *testing.T) {
	enabled := func(mk string) bool { return mk != ModuleGEO }

	if len(FilterMenuTreeByModule(SystemMenuTree, func(string) bool { return true })) == 0 {
		t.Fatal("全开时菜单树不应为空")
	}
	cut := FilterMenuTreeByModule(SystemMenuTree, enabled)

	// geoTools 是 GEO 模块里的一个目录节点，关掉 GEO 后它必须不在结果里
	found := false
	var walk func([]SystemMenuNode)
	walk = func(ns []SystemMenuNode) {
		for _, n := range ns {
			if n.Key == "geoTools" || n.Key == "geoKeywordMining" {
				found = true
			}
			walk(n.Children)
		}
	}
	walk(cut)
	if found {
		t.Error("关闭 geo 模块后，geoTools / geoKeywordMining 仍出现在菜单树中")
	}

	// core 的节点必须还在
	coreAlive := false
	walk = func(ns []SystemMenuNode) {
		for _, n := range ns {
			if n.Key == "systemUser" {
				coreAlive = true
			}
			walk(n.Children)
		}
	}
	walk(cut)
	if !coreAlive {
		t.Error("core 模块节点（systemUser）不应被过滤掉")
	}
}

// TestFilterMenuTreeByModuleNilFilter 未注入判定函数时返回全树（离线首启不被卡死）。
func TestFilterMenuTreeByModuleNilFilter(t *testing.T) {
	got := FilterMenuTreeByModule(SystemMenuTree, nil)
	if len(got) != len(SystemMenuTree) {
		t.Errorf("未注入判定函数时应返回全树，实际 %d/%d", len(got), len(SystemMenuTree))
	}
}
