package middleware

// ModuleRouteSupplement 手写补充的路径模式（**不在生成件里**）
//
// 生成件（module_route_patterns.go）覆盖 224 条「人工审过的能力面 → 模块」。
// 这里补的是**那张表天然覆盖不到**的几类：
//
//  1. 没有菜单页面的能力（llm-cloud 只有顶部余额条与套餐 Tab，
//     platform-link 只有状态条）—— 没有菜单就没有菜单键，
//     能力面表里自然也不会有它。
//  2. 不属于任何页面、纯控制面的接口（模块同步、订单查询）。
//
// 口径：**只加"必须拦"的，不加"不该拦"的**。
// 特别是下面刻意**没有**的三类，它们是买模块的入口，拦了就永远买不了：
//   /api/module-market/*   市场列表与下单
//   /api/modules/my        我的模块
//   /api/llm/*（除 wallet/plans）  商户自带 Key 的模型管理
//
// 生成规则：往这张表加条目时，请在同一处写一句"为什么生成件覆盖不到"，
// 否则三个月后没人分得清它为什么不生成。

var ModuleRouteSupplement = []ModuleRouteRule{
	// ---- llm-cloud：平台模型算力（计量型收费模块）----
	// 为什么不生成：llm-cloud 没有独立菜单页，入口是 LLM 页面的「平台套餐」Tab
	// 与顶部余额条，能力面表按"菜单页消费的口"收录，收不到它。
	// （/api/llm/llm-plans 曾在这里，但没有任何路由注册它 —— 前端调的、
	// 路由挂的都是 /api/llm/plans，死条目已删；下单 /api/llm/recharge 刻意
	// **不进本表**：它是购买入口，拦了就死锁。）
	{Pattern: "/api/llm/wallet", ModuleKey: "llm-cloud"},
	{Pattern: "/api/llm/wallet/*", ModuleKey: "llm-cloud"},
	{Pattern: "/api/llm/plans", ModuleKey: "llm-cloud"},

	// ---- platform-link：模块下发与我的模块 ----
	// 为什么不生成：纯控制面，无菜单页消费。
	{Pattern: "/api/modules/my", ModuleKey: "platform-link"},
	{Pattern: "/api/modules/sync", ModuleKey: "platform-link"},
	{Pattern: "/api/modules/sync-logs", ModuleKey: "platform-link"},
	{Pattern: "/api/manage/modules/*", ModuleKey: "platform-link"},
}

// ModuleRouteAll 生成件 + 补充件（匹配时按此顺序，补充件在前）
//
// 补充件在前是刻意的：它是人工判定"必须拦"的，语义比生成件的推导结果强。
// 两者不会真冲突（生成件里没有这批路径），先放谁都不改变结果 —— 但顺序写死
// 是为了让"以后真冲突了"时的行为可预期，而不是取决于 map 遍历顺序。
func ModuleRouteAll() []ModuleRouteRule {
	out := make([]ModuleRouteRule, 0, len(ModuleRouteSupplement)+len(ModuleRouteRules))
	out = append(out, ModuleRouteSupplement...)
	out = append(out, ModuleRouteRules...)
	return out
}
