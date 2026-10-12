// 能力面 ↔ 菜单 key 对照表：RoutePolicyGuard 用它把"这条接口只有该能力可用"
// 交给菜单授权来决定，而不是再维护一份和侧栏各写各的静态表。
//
// 语义：调用方（非超管）被授予 MenuKeys 中**任意一个**菜单节点才放行；
// 一个都没授 ⇒ 403。超管恒放行；库里 role_menu_permissions 整表为空时按
// DefaultMenuKeys 的出厂分配判（与 GET /api/menus 首屏看到的集合一致）。
//
// 入选口径（三关全过才进表，缺一就留在登录档）：
//  1. 消费该形状的菜单页在注册表里对 customer_service / staff 都不可见；
//  2. 没有任何"角色可见的菜单页"共用该形状；
//  3. 该形状不是登录即执行的壳层调用（store/layout/请求封装），也不是
//     菜单外路由页（详情页/嵌入页）在用——这两类一旦收口，打到的是
//     算不出归属角色的请求，只能先修前端归属再说。
//
// 生成命令：python3 scripts/menu-route-role-audit.py --emit-go user-server/internal/model/menu_capability_routes.go
// 事实源：菜单注册表 user-server/internal/model/menu.go + live 路由守卫档位导出 + user-web 调用图。
// 本文件不许手改：改了会在下一次跑生成命令时被覆盖，且
// TestRouteGuardChain_CapabilityRulesAreNotDeadLines 会逐条核对它命中的 live 路由。
package model

// MenuCapability 一条能力面规则。Pattern 按段匹配真实请求路径，
// `*` 吃掉恰好一段（与 middleware 里仅超管表用的是同一套段匹配）。
type MenuCapability struct {
	Pattern  string
	MenuKeys []string
}

// MenuCapabilities 生成件：共 114 条。
var MenuCapabilities = []MenuCapability{
	{Pattern: "/api/ai/sales-cockpit", MenuKeys: []string{"opsOverview", "salesCockpit"}},
	{Pattern: "/api/analytics/ai-performance", MenuKeys: []string{"automationCenter"}},
	{Pattern: "/api/analytics/persona/staffs", MenuKeys: []string{"personaList"}},
	{Pattern: "/api/analytics/persona/staffs/*", MenuKeys: []string{"personaList"}},
	{Pattern: "/api/customer-oneid/*/identities", MenuKeys: []string{"oneidConflicts", "oneidList"}},
	{Pattern: "/api/customer/oneid/conflicts", MenuKeys: []string{"oneidConflicts", "oneidList"}},
	{Pattern: "/api/customer/oneid/conflicts/*/resolve", MenuKeys: []string{"oneidConflicts", "oneidList"}},
	{Pattern: "/api/customer/oneid/list", MenuKeys: []string{"oneidConflicts", "oneidList"}},
	{Pattern: "/api/customer/oneid/merge", MenuKeys: []string{"oneidConflicts", "oneidList"}},
	{Pattern: "/api/customer/oneid/resolve", MenuKeys: []string{"oneidConflicts", "oneidList"}},
	{Pattern: "/api/customer/oneid/stats", MenuKeys: []string{"oneidConflicts", "oneidList"}},
	{Pattern: "/api/feature-flags", MenuKeys: []string{"systemFeatureFlags"}},
	{Pattern: "/api/feature-flags/*", MenuKeys: []string{"systemFeatureFlags"}},
	{Pattern: "/api/feature-flags/stale", MenuKeys: []string{"systemFeatureFlags"}},
	{Pattern: "/api/glossaries", MenuKeys: []string{"glossary"}},
	{Pattern: "/api/glossaries/*", MenuKeys: []string{"glossary"}},
	{Pattern: "/api/glossaries/validate", MenuKeys: []string{"glossary"}},
	{Pattern: "/api/i18n/stats", MenuKeys: []string{"i18nDashboard"}},
	{Pattern: "/api/i18n/stats/cache", MenuKeys: []string{"i18nDashboard"}},
	{Pattern: "/api/i18n/stats/glossary", MenuKeys: []string{"i18nDashboard"}},
	{Pattern: "/api/i18n/stats/lang-dist", MenuKeys: []string{"i18nDashboard"}},
	{Pattern: "/api/i18n/stats/latency", MenuKeys: []string{"i18nDashboard"}},
	{Pattern: "/api/i18n/stats/quality", MenuKeys: []string{"i18nDashboard"}},
	{Pattern: "/api/knowledge-merchant/batch/import", MenuKeys: []string{"knowledgeBatchImport", "knowledgeExternal", "knowledgeFeedbacks", "knowledgePlayground", "knowledgeTokens"}},
	{Pattern: "/api/knowledge-merchant/batch/upload", MenuKeys: []string{"knowledgeBatchImport", "knowledgeExternal", "knowledgeFeedbacks", "knowledgePlayground", "knowledgeTokens"}},
	{Pattern: "/api/knowledge-merchant/chunks/*", MenuKeys: []string{"knowledgeBatchImport", "knowledgeExternal", "knowledgeFeedbacks", "knowledgePlayground", "knowledgeTokens"}},
	{Pattern: "/api/knowledge-merchant/chunks/*/split", MenuKeys: []string{"knowledgeBatchImport", "knowledgeExternal", "knowledgeFeedbacks", "knowledgePlayground", "knowledgeTokens"}},
	{Pattern: "/api/knowledge-merchant/documents/*/chunks", MenuKeys: []string{"knowledgeBatchImport", "knowledgeExternal", "knowledgeFeedbacks", "knowledgePlayground", "knowledgeTokens"}},
	{Pattern: "/api/knowledge-merchant/external/jobs", MenuKeys: []string{"knowledgeBatchImport", "knowledgeExternal", "knowledgeFeedbacks", "knowledgePlayground", "knowledgeTokens"}},
	{Pattern: "/api/knowledge-merchant/feedback", MenuKeys: []string{"knowledgeBatchImport", "knowledgeExternal", "knowledgeFeedbacks", "knowledgePlayground", "knowledgeTokens"}},
	{Pattern: "/api/knowledge-merchant/feedbacks", MenuKeys: []string{"knowledgeBatchImport", "knowledgeExternal", "knowledgeFeedbacks", "knowledgePlayground", "knowledgeTokens"}},
	{Pattern: "/api/knowledge-merchant/playground", MenuKeys: []string{"knowledgeBatchImport", "knowledgeExternal", "knowledgeFeedbacks", "knowledgePlayground", "knowledgeTokens"}},
	{Pattern: "/api/knowledge-merchant/tokens", MenuKeys: []string{"knowledgeBatchImport", "knowledgeExternal", "knowledgeFeedbacks", "knowledgePlayground", "knowledgeTokens"}},
	{Pattern: "/api/knowledge-merchant/tokens/*/revoke", MenuKeys: []string{"knowledgeBatchImport", "knowledgeExternal", "knowledgeFeedbacks", "knowledgePlayground", "knowledgeTokens"}},
	{Pattern: "/api/knowledge/connectors", MenuKeys: []string{"knowledgeConnectors"}},
	{Pattern: "/api/knowledge/connectors/*", MenuKeys: []string{"knowledgeConnectors"}},
	{Pattern: "/api/knowledge/connectors/*/pull", MenuKeys: []string{"knowledgeConnectors"}},
	{Pattern: "/api/knowledge/connectors/*/test", MenuKeys: []string{"knowledgeConnectors"}},
	{Pattern: "/api/knowledge/documents", MenuKeys: []string{"knowledgeManagement", "knowledgeOpenAPI", "knowledgeStatistics"}},
	{Pattern: "/api/knowledge/documents/*", MenuKeys: []string{"knowledgeManagement", "knowledgeOpenAPI", "knowledgeStatistics"}},
	{Pattern: "/api/knowledge/documents/*/chunks", MenuKeys: []string{"knowledgeManagement", "knowledgeOpenAPI", "knowledgeStatistics"}},
	{Pattern: "/api/knowledge/documents/*/progress", MenuKeys: []string{"knowledgeManagement", "knowledgeOpenAPI", "knowledgeStatistics"}},
	{Pattern: "/api/knowledge/documents/*/public-visibility", MenuKeys: []string{"knowledgeManagement"}},
	{Pattern: "/api/knowledge/documents/*/reindex", MenuKeys: []string{"knowledgeManagement", "knowledgeOpenAPI", "knowledgeStatistics"}},
	{Pattern: "/api/knowledge/import-logs", MenuKeys: []string{"knowledgeManagement", "knowledgeOpenAPI", "knowledgeStatistics"}},
	{Pattern: "/api/knowledge/import/text", MenuKeys: []string{"knowledgeManagement", "knowledgeOpenAPI", "knowledgeStatistics"}},
	{Pattern: "/api/knowledge/import/upload", MenuKeys: []string{"knowledgeManagement", "knowledgeOpenAPI", "knowledgeStatistics"}},
	{Pattern: "/api/knowledge/import/url", MenuKeys: []string{"knowledgeManagement", "knowledgeOpenAPI", "knowledgeStatistics"}},
	{Pattern: "/api/knowledge/openapi/sources", MenuKeys: []string{"knowledgeManagement", "knowledgeOpenAPI", "knowledgeStatistics"}},
	{Pattern: "/api/knowledge/openapi/sources/*", MenuKeys: []string{"knowledgeManagement", "knowledgeOpenAPI", "knowledgeStatistics"}},
	{Pattern: "/api/knowledge/openapi/sources/*/sync", MenuKeys: []string{"knowledgeManagement", "knowledgeOpenAPI", "knowledgeStatistics"}},
	{Pattern: "/api/knowledge/openapi/sources/*/test", MenuKeys: []string{"knowledgeManagement", "knowledgeOpenAPI", "knowledgeStatistics"}},
	{Pattern: "/api/knowledge/openapi/sources/*/toggle", MenuKeys: []string{"knowledgeManagement", "knowledgeOpenAPI", "knowledgeStatistics"}},
	{Pattern: "/api/knowledge/products/*/overview", MenuKeys: []string{"knowledgeManagement", "knowledgeOpenAPI", "knowledgeStatistics"}},
	{Pattern: "/api/knowledge/products/*/rebuild-index", MenuKeys: []string{"knowledgeManagement", "knowledgeOpenAPI", "knowledgeStatistics"}},
	{Pattern: "/api/knowledge/search", MenuKeys: []string{"knowledgeManagement", "knowledgeOpenAPI", "knowledgeStatistics"}},
	{Pattern: "/api/knowledge/stats/documents", MenuKeys: []string{"knowledgeManagement", "knowledgeOpenAPI", "knowledgeStatistics"}},
	{Pattern: "/api/knowledge/stats/imports", MenuKeys: []string{"knowledgeManagement", "knowledgeOpenAPI", "knowledgeStatistics"}},
	{Pattern: "/api/knowledge/stats/openapi", MenuKeys: []string{"knowledgeManagement", "knowledgeOpenAPI", "knowledgeStatistics"}},
	{Pattern: "/api/knowledge/stats/overview", MenuKeys: []string{"knowledgeManagement", "knowledgeOpenAPI", "knowledgeStatistics"}},
	{Pattern: "/api/knowledge/stats/searches", MenuKeys: []string{"knowledgeManagement", "knowledgeOpenAPI", "knowledgeStatistics"}},
	{Pattern: "/api/ltc-rates", MenuKeys: []string{"salesCockpit"}},
	{Pattern: "/api/marketing-flows", MenuKeys: []string{"marketingFlow"}},
	{Pattern: "/api/marketing-flows/*", MenuKeys: []string{"marketingFlow"}},
	{Pattern: "/api/marketing-flows/*/executions", MenuKeys: []string{"marketingFlow"}},
	{Pattern: "/api/marketing-flows/*/stats", MenuKeys: []string{"marketingFlow"}},
	{Pattern: "/api/material/*", MenuKeys: []string{"systemMaterialLibrary"}},
	{Pattern: "/api/material/*/usage", MenuKeys: []string{"systemMaterialLibrary"}},
	{Pattern: "/api/material/categories", MenuKeys: []string{"systemMaterialLibrary"}},
	{Pattern: "/api/material/categories/*", MenuKeys: []string{"systemMaterialLibrary"}},
	{Pattern: "/api/material/list", MenuKeys: []string{"systemMaterialLibrary"}},
	{Pattern: "/api/material/selector", MenuKeys: []string{"systemMaterialLibrary"}},
	{Pattern: "/api/material/stats", MenuKeys: []string{"systemMaterialLibrary"}},
	{Pattern: "/api/material/upload", MenuKeys: []string{"systemMaterialLibrary"}},
	{Pattern: "/api/monitor/alerts/unread", MenuKeys: []string{"opsOverview"}},
	{Pattern: "/api/monitor/anomalies", MenuKeys: []string{"opsOverview", "salesCockpit", "systemTrace"}},
	{Pattern: "/api/monitor/health", MenuKeys: []string{"opsOverview", "salesCockpit", "systemTrace"}},
	{Pattern: "/api/monitor/knowledge-weights", MenuKeys: []string{"systemTrace"}},
	{Pattern: "/api/monitor/latency", MenuKeys: []string{"systemTrace"}},
	{Pattern: "/api/monitor/lifecycle", MenuKeys: []string{"systemTrace"}},
	{Pattern: "/api/monitor/node-health", MenuKeys: []string{"opsOverview", "salesCockpit", "systemTrace"}},
	{Pattern: "/api/monitor/trace-eval/logs", MenuKeys: []string{"systemTrace"}},
	{Pattern: "/api/monitor/trace-tree", MenuKeys: []string{"systemTrace"}},
	{Pattern: "/api/monitor/traces", MenuKeys: []string{"systemTrace"}},
	{Pattern: "/api/objection/categories", MenuKeys: []string{"objectionList"}},
	{Pattern: "/api/objection/classify", MenuKeys: []string{"objectionList"}},
	{Pattern: "/api/objection/handle", MenuKeys: []string{"objectionList"}},
	{Pattern: "/api/objection/usage", MenuKeys: []string{"objectionList"}},
	{Pattern: "/api/office-hours", MenuKeys: []string{"automationCenter"}},
	{Pattern: "/api/platform-accounts", MenuKeys: []string{"platformAccount"}},
	{Pattern: "/api/platform-accounts/*", MenuKeys: []string{"platformAccount"}},
	{Pattern: "/api/platform-accounts/*/status", MenuKeys: []string{"platformAccount"}},
	{Pattern: "/api/platform-accounts/platforms", MenuKeys: []string{"platformAccount"}},
	{Pattern: "/api/rag/eval/diff", MenuKeys: []string{"knowledgeRagEval"}},
	{Pattern: "/api/rag/eval/latest", MenuKeys: []string{"knowledgeRagEval"}},
	{Pattern: "/api/rag/eval/run", MenuKeys: []string{"knowledgeRagEval"}},
	{Pattern: "/api/rag/eval/runs", MenuKeys: []string{"knowledgeRagEval"}},
	{Pattern: "/api/rag/eval/upload", MenuKeys: []string{"knowledgeRagEval"}},
	{Pattern: "/api/rag/recall/collect", MenuKeys: []string{"systemTrace"}},
	{Pattern: "/api/rag/recall/low-recall", MenuKeys: []string{"systemTrace"}},
	{Pattern: "/api/rag/recall/metrics", MenuKeys: []string{"systemTrace"}},
	{Pattern: "/api/rag/recall/snapshot", MenuKeys: []string{"systemTrace"}},
	{Pattern: "/api/rag/recall/snapshots", MenuKeys: []string{"systemTrace"}},
	{Pattern: "/api/rag/recall/start", MenuKeys: []string{"systemTrace"}},
	{Pattern: "/api/rag/recall/stop", MenuKeys: []string{"systemTrace"}},
	{Pattern: "/api/reach/jobs", MenuKeys: []string{"reachPipeline"}},
	{Pattern: "/api/reach/jobs/*", MenuKeys: []string{"reachPipeline"}},
	{Pattern: "/api/reach/pipelines", MenuKeys: []string{"reachPipeline"}},
	{Pattern: "/api/reach/stats", MenuKeys: []string{"reachPipeline"}},
	{Pattern: "/api/report-subscriptions", MenuKeys: []string{"automationCenter"}},
	{Pattern: "/api/security/audit/*", MenuKeys: []string{"securityAudit"}},
	{Pattern: "/api/security/audit/list", MenuKeys: []string{"securityAudit"}},
	{Pattern: "/api/unified-messages", MenuKeys: []string{"unifiedMessage"}},
	{Pattern: "/api/unified-messages/*", MenuKeys: []string{"unifiedMessage"}},
}
